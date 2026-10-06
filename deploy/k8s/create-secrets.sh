#!/bin/bash
# Create or update the aex-secrets Secret (and the mongodb-keyfile Secret)
# that the Kustomize manifests in deploy/k8s reference.
#
# The manifests never ship Secret values: every overlay expects these
# Secrets to exist before 'kubectl apply -k'. This script is the one command
# that provides them, and it is safe to re-run:
#
#   * A value passed in the environment always wins.
#   * Otherwise the value already stored in the cluster is kept, so re-running
#     never rotates JWT_SECRET or the MongoDB password behind your back.
#   * Missing values (and known placeholders such as REPLACE_ME, root/root or
#     dev-api-key) are replaced with freshly generated random values.
#   * ANTHROPIC_API_KEY and AEX_API_KEY cannot be generated; they are left
#     empty with a warning until you provide them.
#
# Secret values are passed to kubectl through files in a private temp
# directory, never on the command line, and are never printed.
#
# Usage:
#   ./deploy/k8s/create-secrets.sh [--namespace aex] [--dry-run]
#
# Examples:
#   # Local kind/minikube cluster: generate everything
#   ./deploy/k8s/create-secrets.sh
#
#   # Provide the values you already have
#   ANTHROPIC_API_KEY=sk-ant-... ./deploy/k8s/create-secrets.sh
#
#   # Use an external MongoDB instead of the in-cluster StatefulSet
#   MONGO_URI='mongodb+srv://user:pass@cluster.example.net/aex' ./deploy/k8s/create-secrets.sh
#
# Keys written to aex-secrets:
#   JWT_SECRET         HS256 key the gateway uses to verify JWTs (>= 32 bytes, random)
#   JWT_SIGNING_KEY    signing key passed to aex-identity (>= 32 bytes, random)
#   WEBHOOK_SECRET     HMAC key for webhook payload signatures (>= 32 bytes, random)
#   MONGO_USERNAME     MongoDB root user (default: aex-admin)
#   MONGO_PASSWORD     MongoDB root password (random)
#   MONGO_URI          connection string built from the two values above
#   AEX_API_KEY        API key the demo agents present to the gateway
#   ANTHROPIC_API_KEY  Anthropic API key for the code review agents
#   API_KEY_SALT       optional, kept/written only when provided

set -euo pipefail

NAMESPACE="aex"
SECRET_NAME="aex-secrets"
KEYFILE_SECRET_NAME="mongodb-keyfile"
DRY_RUN=false
MIN_SECRET_BYTES=32

usage() {
    sed -n '2,/^set -euo pipefail/p' "$0" | sed -e '$d' -e 's/^# \{0,1\}//'
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -n|--namespace) NAMESPACE="$2"; shift 2 ;;
        --dry-run)      DRY_RUN=true; shift ;;
        -h|--help)      usage; exit 0 ;;
        *)              echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
    esac
done

if ! command -v kubectl &> /dev/null; then
    echo "Error: kubectl is required" >&2
    exit 1
fi

# ------------------------------------------------------------
# Helpers
# ------------------------------------------------------------

random_base64() {
    head -c "$1" /dev/urandom | base64 | tr -d '\n'
}

random_hex() {
    head -c "$1" /dev/urandom | od -An -tx1 | tr -d ' \n'
}

# Values that have shipped as defaults or placeholders in this repository
# (or are obvious stand-ins) and must never be used as real credentials.
is_placeholder() {
    local lower
    lower=$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')
    case "$lower" in
        ""|replace_me|replace-me|changeme|change-me|change_me|placeholder|placeholder-update-me|\
        root|admin|password|secret|dev-api-key|test-api-key|"<"*">"|your-*|*do-not-use-in-production*)
            return 0 ;;
    esac
    return 1
}

existing_value() {
    if [[ "$DRY_RUN" == "true" ]] || [[ "$SECRET_EXISTS" != "true" ]]; then
        return 0
    fi
    local encoded
    encoded=$(kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" -o "jsonpath={.data.$1}" 2>/dev/null) || true
    if [[ -n "$encoded" ]]; then
        printf '%s' "$encoded" | base64 --decode
    fi
}

TMP_DIR=$(mktemp -d)
chmod 700 "$TMP_DIR"
trap 'rm -rf "$TMP_DIR"' EXIT

SECRET_ARGS=()
SUMMARY=()

write_key() {
    local key="$1" value="$2" source="$3"
    printf '%s' "$value" > "$TMP_DIR/$key"
    SECRET_ARGS+=("--from-file=$key=$TMP_DIR/$key")
    SUMMARY+=("$(printf '  %-18s %s' "$key" "$source")")
}

# resolve_random KEY BYTES
# Sets RESOLVED_VALUE / RESOLVED_SOURCE for a key that may be generated.
resolve_random() {
    local key="$1" bytes="$2"
    local env_value="${!key:-}"
    local current
    if [[ -n "$env_value" ]]; then
        if is_placeholder "$env_value" || [[ ${#env_value} -lt $MIN_SECRET_BYTES ]]; then
            echo "Error: $key from the environment is a placeholder or shorter than $MIN_SECRET_BYTES bytes." >&2
            echo "       Generate one with: openssl rand -base64 48" >&2
            exit 1
        fi
        RESOLVED_VALUE="$env_value"
        RESOLVED_SOURCE="from environment"
        return
    fi
    current=$(existing_value "$key")
    if [[ -n "$current" ]] && ! is_placeholder "$current" && [[ ${#current} -ge $MIN_SECRET_BYTES ]]; then
        RESOLVED_VALUE="$current"
        RESOLVED_SOURCE="kept existing value"
        return
    fi
    RESOLVED_VALUE=$(random_base64 "$bytes")
    if [[ -n "$current" ]]; then
        RESOLVED_SOURCE="REGENERATED (existing value was a placeholder or too short)"
    else
        RESOLVED_SOURCE="generated"
    fi
}

# resolve_provided KEY
# For values that cannot be generated (third-party API keys).
resolve_provided() {
    local key="$1"
    local env_value="${!key:-}"
    local current
    if [[ -n "$env_value" ]]; then
        RESOLVED_VALUE="$env_value"
        RESOLVED_SOURCE="from environment"
        return
    fi
    current=$(existing_value "$key")
    if [[ -n "$current" ]] && ! is_placeholder "$current"; then
        RESOLVED_VALUE="$current"
        RESOLVED_SOURCE="kept existing value"
        return
    fi
    RESOLVED_VALUE=""
    RESOLVED_SOURCE="EMPTY - set $key and re-run this script"
}

# ------------------------------------------------------------
# Resolve values
# ------------------------------------------------------------

SECRET_EXISTS=false
if [[ "$DRY_RUN" != "true" ]]; then
    kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f - > /dev/null
    if kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" &> /dev/null; then
        SECRET_EXISTS=true
    fi
fi

for key in JWT_SECRET JWT_SIGNING_KEY WEBHOOK_SECRET; do
    resolve_random "$key" 48
    write_key "$key" "$RESOLVED_VALUE" "$RESOLVED_SOURCE"
done

# MongoDB credentials. MongoDB only reads MONGO_INITDB_ROOT_* when its data
# volume is first initialised, so the stored password must stay stable.
mongo_user_source="kept existing value"
mongo_user="${MONGO_USERNAME:-}"
if [[ -n "$mongo_user" ]]; then
    mongo_user_source="from environment"
else
    mongo_user=$(existing_value MONGO_USERNAME)
    if [[ -z "$mongo_user" ]]; then
        mongo_user="aex-admin"
        mongo_user_source="default"
    fi
fi

previous_mongo_password=$(existing_value MONGO_PASSWORD)
mongo_password_source="kept existing value"
mongo_password="${MONGO_PASSWORD:-}"
if [[ -n "$mongo_password" ]]; then
    if is_placeholder "$mongo_password"; then
        echo "Error: MONGO_PASSWORD from the environment is a placeholder." >&2
        exit 1
    fi
    mongo_password_source="from environment"
elif [[ -n "$previous_mongo_password" ]] && ! is_placeholder "$previous_mongo_password"; then
    mongo_password="$previous_mongo_password"
else
    # Hex keeps the password URI-safe inside MONGO_URI.
    mongo_password=$(random_hex 24)
    if [[ -n "$previous_mongo_password" ]]; then
        mongo_password_source="REGENERATED (existing value was a placeholder)"
    else
        mongo_password_source="generated"
    fi
fi

mongo_uri="${MONGO_URI:-}"
mongo_uri_source="from environment"
if [[ -z "$mongo_uri" ]]; then
    mongo_uri=$(existing_value MONGO_URI)
    mongo_uri_source="kept existing value"
    # Rebuild only a URI this script owns: empty, the old root:root
    # placeholder, or one pointing at the in-cluster MongoDB. An operator's
    # external URI (managed MongoDB) is kept as is.
    if [[ -z "$mongo_uri" ]] || [[ "$mongo_uri" == *"root:root@"* ]] \
        || { [[ "$mongo_uri" == *"@mongodb.${NAMESPACE}.svc"* ]] \
            && { [[ "$mongo_password_source" != "kept existing value" ]] \
                || [[ "$mongo_user_source" == "from environment" ]]; }; }; then
        mongo_uri="mongodb://${mongo_user}:${mongo_password}@mongodb.${NAMESPACE}.svc.cluster.local:27017/?authSource=admin"
        mongo_uri_source="derived from MONGO_USERNAME/MONGO_PASSWORD"
    fi
fi

write_key MONGO_USERNAME "$mongo_user" "$mongo_user_source"
write_key MONGO_PASSWORD "$mongo_password" "$mongo_password_source"
write_key MONGO_URI "$mongo_uri" "$mongo_uri_source"

for key in AEX_API_KEY ANTHROPIC_API_KEY; do
    resolve_provided "$key"
    write_key "$key" "$RESOLVED_VALUE" "$RESOLVED_SOURCE"
done

# Optional keys: written only when provided or already present.
api_key_salt="${API_KEY_SALT:-}"
if [[ -n "$api_key_salt" ]]; then
    write_key API_KEY_SALT "$api_key_salt" "from environment"
else
    api_key_salt=$(existing_value API_KEY_SALT)
    if [[ -n "$api_key_salt" ]]; then
        write_key API_KEY_SALT "$api_key_salt" "kept existing value"
    fi
fi

# ------------------------------------------------------------
# Apply
# ------------------------------------------------------------

echo "Secret $NAMESPACE/$SECRET_NAME:"
printf '%s\n' "${SUMMARY[@]}"

if [[ "$DRY_RUN" == "true" ]]; then
    echo "Dry run: nothing applied."
    exit 0
fi

kubectl create secret generic "$SECRET_NAME" \
    --namespace "$NAMESPACE" \
    "${SECRET_ARGS[@]}" \
    --dry-run=client -o yaml \
    | kubectl label --local -f - -o yaml \
        app.kubernetes.io/part-of=agent-exchange \
        app.kubernetes.io/component=secrets \
    | kubectl apply -f - > /dev/null
echo "Applied $NAMESPACE/$SECRET_NAME"

# The MongoDB replica set keyfile must never change once the StatefulSet has
# started, so it is created only when missing.
if kubectl get secret "$KEYFILE_SECRET_NAME" -n "$NAMESPACE" &> /dev/null; then
    echo "Kept existing $NAMESPACE/$KEYFILE_SECRET_NAME"
else
    random_base64 756 > "$TMP_DIR/keyfile"
    kubectl create secret generic "$KEYFILE_SECRET_NAME" \
        --namespace "$NAMESPACE" \
        --from-file=keyfile="$TMP_DIR/keyfile" \
        --dry-run=client -o yaml \
        | kubectl label --local -f - -o yaml \
            app.kubernetes.io/part-of=agent-exchange \
            app.kubernetes.io/component=database \
        | kubectl apply -f - > /dev/null
    echo "Created $NAMESPACE/$KEYFILE_SECRET_NAME"
fi

if [[ "$mongo_password_source" == REGENERATED* ]] || \
   { [[ "$mongo_password_source" == "from environment" ]] && [[ -n "$previous_mongo_password" ]] \
     && [[ "$previous_mongo_password" != "$mongo_password" ]]; }; then
    echo ""
    echo "WARNING: MONGO_PASSWORD changed. MongoDB only reads its root credentials when"
    echo "the data volume is first initialised, so an existing database still accepts"
    echo "the old password. Change it inside MongoDB (or recreate the mongodb PVC) and"
    echo "then restart the AEX services:"
    echo "  kubectl exec -n $NAMESPACE -it mongodb-0 -- mongosh -u <old-user> -p --authenticationDatabase admin \\"
    echo "    --eval 'db.getSiblingDB(\"admin\").changeUserPassword(\"$mongo_user\", passwordPrompt())'"
fi

for key in AEX_API_KEY ANTHROPIC_API_KEY; do
    if [[ ! -s "$TMP_DIR/$key" ]]; then
        echo "WARNING: $key is empty. Re-run with $key=... set once you have it."
    fi
done
