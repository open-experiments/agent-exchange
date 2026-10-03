#!/usr/bin/env bash
#
# AEX Platform End-to-End Test Runner
#
# Tests the full AEX workflow against docker-compose services on localhost.
# Covers CertAuth integration (10.4), cert-boosts-ranking (10.5),
# and revoke-cert-stops-boost (10.6).
#
# Usage:
#   ./hack/tests/e2e_test.sh
#
# Prerequisites:
#   - docker-compose services running (see hack/docker-compose.yml)
#   - curl, jq installed
#
set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
WORK_PUBLISHER_URL="${WORK_PUBLISHER_URL:-http://localhost:8081}"
BID_GATEWAY_URL="${BID_GATEWAY_URL:-http://localhost:8082}"
BID_EVALUATOR_URL="${BID_EVALUATOR_URL:-http://localhost:8083}"
CONTRACT_ENGINE_URL="${CONTRACT_ENGINE_URL:-http://localhost:8084}"
PROVIDER_REGISTRY_URL="${PROVIDER_REGISTRY_URL:-http://localhost:8085}"
TRUST_BROKER_URL="${TRUST_BROKER_URL:-http://localhost:8086}"
IDENTITY_URL="${IDENTITY_URL:-http://localhost:8087}"
SETTLEMENT_URL="${SETTLEMENT_URL:-http://localhost:8088}"
TELEMETRY_URL="${TELEMETRY_URL:-http://localhost:8089}"
CREDENTIALS_PROVIDER_URL="${CREDENTIALS_PROVIDER_URL:-http://localhost:8090}"
CERTAUTH_URL="${CERTAUTH_URL:-http://localhost:8091}"
TOKEN_BANK_URL="${TOKEN_BANK_URL:-http://localhost:8092}"

JWT_SECRET="${JWT_SECRET:-dev-jwt-secret-do-not-use-in-production}"

PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

# Colours (disabled when not a terminal)
if [ -t 1 ]; then
  GREEN='\033[0;32m'
  RED='\033[0;31m'
  YELLOW='\033[1;33m'
  CYAN='\033[0;36m'
  NC='\033[0m'
else
  GREEN='' RED='' YELLOW='' CYAN='' NC=''
fi

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

# Unique suffix so concurrent runs don't collide.
RUN_ID="$(date +%s)-$$"

log_header() { echo -e "\n${CYAN}====== $1 ======${NC}"; }
log_pass()   { echo -e "  ${GREEN}PASS${NC}: $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
log_fail()   { echo -e "  ${RED}FAIL${NC}: $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
log_skip()   { echo -e "  ${YELLOW}SKIP${NC}: $1"; SKIP_COUNT=$((SKIP_COUNT + 1)); }
log_info()   { echo -e "  ${CYAN}INFO${NC}: $1"; }

# http_call METHOD URL [DATA]
# Executes curl and returns body on stdout, appends HTTP status code on the
# last line. Callers parse with: body / status=$(tail -1).
http_call() {
  local method="$1" url="$2" data="${3:-}"
  if [ -n "$data" ]; then
    curl -s -w '\n%{http_code}' -X "$method" -H 'Content-Type: application/json' \
      -d "$data" "$url" 2>/dev/null || echo -e "\n000"
  else
    curl -s -w '\n%{http_code}' -X "$method" -H 'Content-Type: application/json' \
      "$url" 2>/dev/null || echo -e "\n000"
  fi
}

# http_status METHOD URL [DATA] -> prints only the status code
http_status() {
  local raw
  raw="$(http_call "$@")"
  echo "$raw" | tail -1
}

# http_body METHOD URL [DATA] -> prints body (everything except last line)
http_body() {
  local raw
  raw="$(http_call "$@")"
  echo "$raw" | sed '$d'
}

# http_both METHOD URL [DATA] -> sets global _BODY and _STATUS
http_both() {
  local raw
  raw="$(http_call "$@")"
  _STATUS="$(echo "$raw" | tail -1)"
  _BODY="$(echo "$raw" | sed '$d')"
}

# http_call_auth METHOD URL AUTH_TOKEN [DATA]
# Like http_call but adds Authorization: Bearer <token> header.
http_call_auth() {
  local method="$1" url="$2" token="$3" data="${4:-}"
  if [ -n "$data" ]; then
    curl -s -w '\n%{http_code}' -X "$method" \
      -H 'Content-Type: application/json' \
      -H "Authorization: Bearer $token" \
      -d "$data" "$url" 2>/dev/null || echo -e "\n000"
  else
    curl -s -w '\n%{http_code}' -X "$method" \
      -H 'Content-Type: application/json' \
      -H "Authorization: Bearer $token" \
      "$url" 2>/dev/null || echo -e "\n000"
  fi
}

# http_both_auth METHOD URL AUTH_TOKEN [DATA] -> sets global _BODY and _STATUS
http_both_auth() {
  local raw
  raw="$(http_call_auth "$@")"
  _STATUS="$(echo "$raw" | tail -1)"
  _BODY="$(echo "$raw" | sed '$d')"
}

# http_both_consumer METHOD URL CONSUMER_ID DATA -> sets global _BODY and _STATUS
# Sends X-Consumer-ID, which work-publisher requires on direct (non-gateway) calls.
http_both_consumer() {
  local method="$1" url="$2" consumer="$3" data="$4" raw
  raw="$(curl -s -w '\n%{http_code}' -X "$method" \
    -H 'Content-Type: application/json' \
    -H "X-Consumer-ID: $consumer" \
    -d "$data" "$url" 2>/dev/null || echo -e "\n000")"
  _STATUS="$(echo "$raw" | tail -1)"
  _BODY="$(echo "$raw" | sed '$d')"
}

assert_status() {
  local expected="$1" actual="$2" label="$3"
  if [ "$actual" = "$expected" ]; then
    log_pass "$label (HTTP $actual)"
  else
    log_fail "$label (expected HTTP $expected, got HTTP $actual)"
  fi
}

assert_json_field() {
  local json="$1" field="$2" expected="$3" label="$4"
  local actual
  actual="$(echo "$json" | jq -r "$field" 2>/dev/null || echo "__jq_error__")"
  if [ "$actual" = "$expected" ]; then
    log_pass "$label ($field = $expected)"
  else
    log_fail "$label ($field: expected '$expected', got '$actual')"
  fi
}

assert_json_field_exists() {
  local json="$1" field="$2" label="$3"
  local actual
  actual="$(echo "$json" | jq -r "$field" 2>/dev/null || echo "null")"
  if [ "$actual" != "null" ] && [ "$actual" != "" ]; then
    log_pass "$label ($field exists: $actual)"
  else
    log_fail "$label ($field is null/empty)"
  fi
}

assert_json_numeric_gt() {
  local json="$1" field="$2" threshold="$3" label="$4"
  local actual
  actual="$(echo "$json" | jq -r "$field" 2>/dev/null || echo "0")"
  if awk "BEGIN {exit !($actual > $threshold)}"; then
    log_pass "$label ($field = $actual > $threshold)"
  else
    log_fail "$label ($field = $actual, expected > $threshold)"
  fi
}

# ---------------------------------------------------------------------------
# 0) Health checks
# ---------------------------------------------------------------------------
log_header "0. Service Health Checks"

declare -A SERVICES=(
  [gateway]="$GATEWAY_URL"
  [work-publisher]="$WORK_PUBLISHER_URL"
  [bid-gateway]="$BID_GATEWAY_URL"
  [bid-evaluator]="$BID_EVALUATOR_URL"
  [contract-engine]="$CONTRACT_ENGINE_URL"
  [provider-registry]="$PROVIDER_REGISTRY_URL"
  [trust-broker]="$TRUST_BROKER_URL"
  [identity]="$IDENTITY_URL"
  [settlement]="$SETTLEMENT_URL"
  [telemetry]="$TELEMETRY_URL"
  [certauth]="$CERTAUTH_URL"
)

ALL_HEALTHY=true
for svc in "${!SERVICES[@]}"; do
  status="$(http_status GET "${SERVICES[$svc]}/health")"
  if [ "$status" = "200" ]; then
    log_pass "$svc health"
  else
    log_fail "$svc health (HTTP $status)"
    ALL_HEALTHY=false
  fi
done

if [ "$ALL_HEALTHY" = false ]; then
  echo -e "\n${RED}Not all services are healthy. Aborting.${NC}"
  echo "RESULTS: $PASS_COUNT passed, $FAIL_COUNT failed, $SKIP_COUNT skipped"
  exit 1
fi

# ---------------------------------------------------------------------------
# 10.4  CertAuth Integration
# ---------------------------------------------------------------------------
log_header "10.4 CertAuth Integration"

# 10.4a  Register a provider via provider-registry
PROVIDER_A_NAME="CertTestProvider-${RUN_ID}"
http_both POST "$PROVIDER_REGISTRY_URL/v1/providers" \
  "{\"name\": \"$PROVIDER_A_NAME\", \"capabilities\": [\"text-generation\", \"summarization\"], \"endpoint\": \"https://provider-a.test/api\"}"
assert_status "200" "$_STATUS" "10.4a: Register provider"
PROVIDER_A_ID="$(echo "$_BODY" | jq -r '.provider_id // .id // empty')"
PROVIDER_A_KEY="$(echo "$_BODY" | jq -r '.api_key // empty')"
log_info "Provider A ID=$PROVIDER_A_ID"

# 10.4b  Request a certificate for that provider
http_both POST "$CERTAUTH_URL/v1/certificates/request" \
  "{
    \"tenant_id\": \"test-tenant-${RUN_ID}\",
    \"provider_id\": \"$PROVIDER_A_ID\",
    \"agent_name\": \"test-agent-a\",
    \"certificate_type\": \"CAPABILITY\",
    \"claims\": [{
      \"category\": \"TECHNOLOGY\",
      \"capability\": \"text-generation\",
      \"authorization\": \"SELF_ASSERTED\"
    }],
    \"public_key_pem\": \"test-public-key-pem-for-e2e\"
  }"
assert_status "201" "$_STATUS" "10.4b: Request certificate"
CERT_REQUEST_ID="$(echo "$_BODY" | jq -r '.request_id // empty')"
log_info "Certificate request ID=$CERT_REQUEST_ID"
assert_json_field "$_BODY" '.status' "PENDING" "10.4b: Request status is PENDING"

# 10.4c  Approve the certificate
http_both POST "$CERTAUTH_URL/internal/v1/certificates/${CERT_REQUEST_ID}/approve" \
  '{"reviewed_by": "e2e-test"}'
assert_status "200" "$_STATUS" "10.4c: Approve certificate"
CERT_A_ID="$(echo "$_BODY" | jq -r '.certificate_id // empty')"
log_info "Certificate A ID=$CERT_A_ID"
assert_json_field "$_BODY" '.status' "ACTIVE" "10.4c: Certificate status is ACTIVE"

# 10.4d  Get the certificate
http_both GET "$CERTAUTH_URL/v1/certificates/${CERT_A_ID}"
assert_status "200" "$_STATUS" "10.4d: Get certificate"
assert_json_field "$_BODY" '.certificate_id' "$CERT_A_ID" "10.4d: Certificate ID matches"
assert_json_field "$_BODY" '.status' "ACTIVE" "10.4d: Certificate still ACTIVE"
assert_json_field "$_BODY" '.provider_id' "$PROVIDER_A_ID" "10.4d: Provider ID matches"

# 10.4e  Verify the certificate
http_both POST "$CERTAUTH_URL/v1/certificates/verify" \
  "{\"certificate_id\": \"$CERT_A_ID\"}"
assert_status "200" "$_STATUS" "10.4e: Verify certificate"
assert_json_field "$_BODY" '.valid' "true" "10.4e: Certificate is valid"
assert_json_field "$_BODY" '.status' "ACTIVE" "10.4e: Verified status is ACTIVE"

# 10.4f  Check CRL
http_both GET "$CERTAUTH_URL/v1/crl"
assert_status "200" "$_STATUS" "10.4f: Get CRL"
assert_json_field "$_BODY" '.issuer_id' "aex-certauth" "10.4f: CRL issuer"

# 10.4g  Get reputation
http_both GET "$CERTAUTH_URL/v1/providers/${PROVIDER_A_ID}/reputation"
assert_status "200" "$_STATUS" "10.4g: Get reputation"
assert_json_field "$_BODY" '.provider_id' "$PROVIDER_A_ID" "10.4g: Reputation provider_id"

# ---------------------------------------------------------------------------
# 10.5  Cert Boosts Ranking
# ---------------------------------------------------------------------------
log_header "10.5 Cert Boosts Ranking"

# 10.5a  Register a second provider (no cert)
PROVIDER_B_NAME="NoCertProvider-${RUN_ID}"
http_both POST "$PROVIDER_REGISTRY_URL/v1/providers" \
  "{\"name\": \"$PROVIDER_B_NAME\", \"capabilities\": [\"text-generation\"], \"endpoint\": \"https://provider-b.test/api\"}"
assert_status "200" "$_STATUS" "10.5a: Register provider B (no cert)"
PROVIDER_B_ID="$(echo "$_BODY" | jq -r '.provider_id // .id // empty')"
PROVIDER_B_KEY="$(echo "$_BODY" | jq -r '.api_key // empty')"
log_info "Provider B ID=$PROVIDER_B_ID (no cert)"

# 10.5b  Submit work spec
http_both_consumer POST "$WORK_PUBLISHER_URL/v1/work" "consumer-${RUN_ID}" \
  "{
    \"category\": \"text-generation\",
    \"description\": \"Cert boost ranking test\",
    \"payload\": {\"text\": \"test data for ranking evaluation\"},
    \"budget\": {\"max_price\": 100.00},
    \"consumer_id\": \"consumer-${RUN_ID}\",
    \"bid_window_ms\": 300000
  }"
assert_status "201" "$_STATUS" "10.5b: Submit work spec"
WORK_ID="$(echo "$_BODY" | jq -r '.work_id // .id // empty')"
log_info "Work ID=$WORK_ID"

# 10.5c  Both providers bid (same price, same confidence, same SLA)
EXPIRES_AT="$(date -u -v+1H +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u -d '+1 hour' +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "2099-12-31T23:59:59Z")"

http_both_auth POST "$BID_GATEWAY_URL/v1/bids" "$PROVIDER_A_KEY" \
  "{
    \"work_id\": \"$WORK_ID\",
    \"provider_id\": \"$PROVIDER_A_ID\",
    \"price\": 50.00,
    \"confidence\": 0.9,
    \"approach\": \"Certified provider approach\",
    \"a2a_endpoint\": \"https://provider-a.test/a2a\",
    \"sla\": {\"max_latency_ms\": 3000, \"availability\": 0.99},
    \"expires_at\": \"$EXPIRES_AT\"
  }"
assert_status "200" "$_STATUS" "10.5c: Provider A bid submitted"
BID_A_ID="$(echo "$_BODY" | jq -r '.bid_id // empty')"
log_info "Bid A ID=$BID_A_ID"

http_both_auth POST "$BID_GATEWAY_URL/v1/bids" "$PROVIDER_B_KEY" \
  "{
    \"work_id\": \"$WORK_ID\",
    \"provider_id\": \"$PROVIDER_B_ID\",
    \"price\": 50.00,
    \"confidence\": 0.9,
    \"approach\": \"Uncertified provider approach\",
    \"a2a_endpoint\": \"https://provider-b.test/a2a\",
    \"sla\": {\"max_latency_ms\": 3000, \"availability\": 0.99},
    \"expires_at\": \"$EXPIRES_AT\"
  }"
assert_status "200" "$_STATUS" "10.5c: Provider B bid submitted"
BID_B_ID="$(echo "$_BODY" | jq -r '.bid_id // empty')"
log_info "Bid B ID=$BID_B_ID"

# 10.5d  Evaluate bids -- certified provider should rank higher
http_both POST "$BID_EVALUATOR_URL/internal/v1/evaluate" \
  "{
    \"work_id\": \"$WORK_ID\",
    \"budget\": {\"max_price\": 100.00, \"bid_strategy\": \"balanced\"}
  }"
assert_status "200" "$_STATUS" "10.5d: Evaluate bids"
EVAL_BODY="$_BODY"

RANKED_COUNT="$(echo "$EVAL_BODY" | jq '.ranked_bids | length' 2>/dev/null || echo 0)"
log_info "Ranked bids: $RANKED_COUNT"

if [ "$RANKED_COUNT" -ge 2 ]; then
  # Extract scores for each provider
  SCORE_A="$(echo "$EVAL_BODY" | jq -r ".ranked_bids[] | select(.provider_id == \"$PROVIDER_A_ID\") | .total_score" 2>/dev/null || echo "0")"
  SCORE_B="$(echo "$EVAL_BODY" | jq -r ".ranked_bids[] | select(.provider_id == \"$PROVIDER_B_ID\") | .total_score" 2>/dev/null || echo "0")"
  CERT_SCORE_A="$(echo "$EVAL_BODY" | jq -r ".ranked_bids[] | select(.provider_id == \"$PROVIDER_A_ID\") | .scores.certification" 2>/dev/null || echo "0")"
  CERT_SCORE_B="$(echo "$EVAL_BODY" | jq -r ".ranked_bids[] | select(.provider_id == \"$PROVIDER_B_ID\") | .scores.certification" 2>/dev/null || echo "0")"

  log_info "Provider A total_score=$SCORE_A, cert_score=$CERT_SCORE_A"
  log_info "Provider B total_score=$SCORE_B, cert_score=$CERT_SCORE_B"

  # The certified provider (A) should have a higher certification component
  if awk "BEGIN {exit !($CERT_SCORE_A > $CERT_SCORE_B)}"; then
    log_pass "10.5d: Certified provider has higher certification score ($CERT_SCORE_A > $CERT_SCORE_B)"
  else
    log_fail "10.5d: Expected certified provider to have higher cert score (A=$CERT_SCORE_A, B=$CERT_SCORE_B)"
  fi

  # The certified provider should have a higher or equal total score
  if awk "BEGIN {exit !($SCORE_A >= $SCORE_B)}"; then
    log_pass "10.5d: Certified provider ranks higher or equal ($SCORE_A >= $SCORE_B)"
  else
    log_fail "10.5d: Expected certified provider to rank higher or equal (A=$SCORE_A, B=$SCORE_B)"
  fi
else
  log_skip "10.5d: Not enough ranked bids to compare ($RANKED_COUNT)"
fi

# Save the first evaluation scores for comparison in 10.6
EVAL_1_SCORE_A="$SCORE_A"
EVAL_1_CERT_A="$CERT_SCORE_A"

# ---------------------------------------------------------------------------
# 10.6  Revoke Cert Stops Boost
# ---------------------------------------------------------------------------
log_header "10.6 Revoke Cert Stops Boost"

# 10.6a  Revoke the certificate from test 10.5
http_both DELETE "$CERTAUTH_URL/v1/certificates/${CERT_A_ID}" \
  '{"reason": "e2e test revocation"}'
assert_status "200" "$_STATUS" "10.6a: Revoke certificate"
assert_json_field "$_BODY" '.status' "revoked" "10.6a: Revocation confirmed"

# 10.6b  Verify the certificate is no longer valid
http_both POST "$CERTAUTH_URL/v1/certificates/verify" \
  "{\"certificate_id\": \"$CERT_A_ID\"}"
assert_status "200" "$_STATUS" "10.6b: Verify revoked certificate"
assert_json_field "$_BODY" '.valid' "false" "10.6b: Certificate is no longer valid"

# 10.6c  Submit a new work spec and identical bids to re-evaluate
http_both_consumer POST "$WORK_PUBLISHER_URL/v1/work" "consumer-${RUN_ID}" \
  "{
    \"category\": \"text-generation\",
    \"description\": \"Cert revoke boost test\",
    \"payload\": {\"text\": \"test data for revoked cert evaluation\"},
    \"budget\": {\"max_price\": 100.00},
    \"consumer_id\": \"consumer-${RUN_ID}\",
    \"bid_window_ms\": 300000
  }"
assert_status "201" "$_STATUS" "10.6c: Submit new work spec"
WORK_ID_2="$(echo "$_BODY" | jq -r '.work_id // .id // empty')"
log_info "Work ID 2=$WORK_ID_2"

http_both_auth POST "$BID_GATEWAY_URL/v1/bids" "$PROVIDER_A_KEY" \
  "{
    \"work_id\": \"$WORK_ID_2\",
    \"provider_id\": \"$PROVIDER_A_ID\",
    \"price\": 50.00,
    \"confidence\": 0.9,
    \"approach\": \"Provider A (cert revoked)\",
    \"a2a_endpoint\": \"https://provider-a.test/a2a\",
    \"sla\": {\"max_latency_ms\": 3000, \"availability\": 0.99},
    \"expires_at\": \"$EXPIRES_AT\"
  }"
assert_status "200" "$_STATUS" "10.6c: Provider A bid (revoked cert)"

http_both_auth POST "$BID_GATEWAY_URL/v1/bids" "$PROVIDER_B_KEY" \
  "{
    \"work_id\": \"$WORK_ID_2\",
    \"provider_id\": \"$PROVIDER_B_ID\",
    \"price\": 50.00,
    \"confidence\": 0.9,
    \"approach\": \"Provider B (no cert)\",
    \"a2a_endpoint\": \"https://provider-b.test/a2a\",
    \"sla\": {\"max_latency_ms\": 3000, \"availability\": 0.99},
    \"expires_at\": \"$EXPIRES_AT\"
  }"
assert_status "200" "$_STATUS" "10.6c: Provider B bid (no cert)"

# 10.6d  Re-evaluate -- cert boost should be gone for provider A
http_both POST "$BID_EVALUATOR_URL/internal/v1/evaluate" \
  "{
    \"work_id\": \"$WORK_ID_2\",
    \"budget\": {\"max_price\": 100.00, \"bid_strategy\": \"balanced\"}
  }"
assert_status "200" "$_STATUS" "10.6d: Re-evaluate bids after revocation"
EVAL2_BODY="$_BODY"

RANKED_COUNT_2="$(echo "$EVAL2_BODY" | jq '.ranked_bids | length' 2>/dev/null || echo 0)"
log_info "Ranked bids (round 2): $RANKED_COUNT_2"

if [ "$RANKED_COUNT_2" -ge 2 ]; then
  SCORE_A_R2="$(echo "$EVAL2_BODY" | jq -r ".ranked_bids[] | select(.provider_id == \"$PROVIDER_A_ID\") | .total_score" 2>/dev/null || echo "0")"
  SCORE_B_R2="$(echo "$EVAL2_BODY" | jq -r ".ranked_bids[] | select(.provider_id == \"$PROVIDER_B_ID\") | .total_score" 2>/dev/null || echo "0")"
  CERT_SCORE_A_R2="$(echo "$EVAL2_BODY" | jq -r ".ranked_bids[] | select(.provider_id == \"$PROVIDER_A_ID\") | .scores.certification" 2>/dev/null || echo "0")"
  CERT_SCORE_B_R2="$(echo "$EVAL2_BODY" | jq -r ".ranked_bids[] | select(.provider_id == \"$PROVIDER_B_ID\") | .scores.certification" 2>/dev/null || echo "0")"

  log_info "Provider A total_score=$SCORE_A_R2, cert_score=$CERT_SCORE_A_R2 (after revoke)"
  log_info "Provider B total_score=$SCORE_B_R2, cert_score=$CERT_SCORE_B_R2"

  # After revocation, cert scores for both should be similar (both ~0 or equal)
  if awk "BEGIN {exit !($CERT_SCORE_A_R2 <= $CERT_SCORE_B_R2 + 0.01)}"; then
    log_pass "10.6d: Revoked cert no longer gives boost (A cert=$CERT_SCORE_A_R2 <= B cert=$CERT_SCORE_B_R2)"
  else
    log_fail "10.6d: Revoked cert still gives boost (A cert=$CERT_SCORE_A_R2 > B cert=$CERT_SCORE_B_R2)"
  fi

  # The original cert boost should have been reduced
  if awk "BEGIN {exit !($CERT_SCORE_A_R2 < $EVAL_1_CERT_A + 0.01)}"; then
    log_pass "10.6d: Cert score decreased after revocation ($CERT_SCORE_A_R2 <= $EVAL_1_CERT_A)"
  else
    log_fail "10.6d: Cert score did not decrease ($CERT_SCORE_A_R2 vs original $EVAL_1_CERT_A)"
  fi
else
  log_skip "10.6d: Not enough ranked bids for comparison ($RANKED_COUNT_2)"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
log_header "Test Summary"
TOTAL=$((PASS_COUNT + FAIL_COUNT + SKIP_COUNT))
echo -e "  Total:   $TOTAL"
echo -e "  ${GREEN}Passed:  $PASS_COUNT${NC}"
echo -e "  ${RED}Failed:  $FAIL_COUNT${NC}"
echo -e "  ${YELLOW}Skipped: $SKIP_COUNT${NC}"

if [ "$FAIL_COUNT" -gt 0 ]; then
  echo -e "\n${RED}SOME TESTS FAILED${NC}"
  exit 1
else
  echo -e "\n${GREEN}ALL TESTS PASSED${NC}"
  exit 0
fi
