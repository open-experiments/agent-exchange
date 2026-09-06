# Tool-Call Gate (`internal/toolgate`)

AEX authorizes at the gateway. An API key carries scopes, a route needs a scope, and every request is allowed or refused on that match and logged with its method, path, status, latency and tenant. That decision is made on the name of the thing being called and never on the values passed to it. After a contract is awarded AEX steps aside, and tool access at the MCP layer is the provider's business. Nothing in the platform decides, or records, an individual tool call.

The tool-call gate closes that gap on the provider side. It sits in front of a provider agent's tool handlers, decides each call on scope and then on argument values, and emits an audit artifact per call into the `TOOLCALL` JetStream stream, hash-chained per provider, so that a reader who was not present can reconstruct what was requested, what was decided, on which rule, for whom, and what happened.

## Why a name check is not enough

A scope gate lets a payment through when the tool `send_payment` is in scope, whatever the amount and whoever the recipient. Its record of a payment redirected to an attacker's account is identical, field for field, to its record of a legitimate payment, because it never looked at the values and therefore never wrote them down. The gate's tests pin this down: in `ModeScope` the records of the legitimate payment (`C04`) and the redirected one (`C06`) match on every field but time and hash; in `ModeFull` the redirected payment is refused on rule `P3-recipient` and the record says so.

The study behind the component compared three configurations over the same twelve calls (see `hack/toolgate-harness`): a plain framework log line, a scope gate, and a value gate, scored on the five auditability dimensions of Nian et al. (2026), *Auditable agents* (arXiv:2604.05485). Only the value gate scores on every dimension; the plain log leaves two of nine fields an approver could ask for, the scope gate six, the value gate nine.

## What the gate does

1. **Scope.** The tool's required scope (from the policy's `tool_scopes`) must be in the session's `granted_scopes`. Unknown tools fail closed. A caller's `X-Scopes` intersects with the policy's grant and can only narrow it, never widen it: the header is caller-controlled, so `*` there leaves the policy's grant as it stands rather than granting everything.
2. **Rules.** In `ModeFull`, argument-value rules are evaluated in policy order and the first that fires decides: `ceiling` (numeric maximum), `allowlist`, `lookup` (the value must match what a table maps another argument to, such as the vendor account on file for this invoice), `sensitive_field` (escalate to a person), `suffix` (email domains), `prefix` (storage destinations). A rule's `effect` is `deny` or `escalate`.
3. **Execution.** Allowed calls run through the caller's executor; refused calls do not run; escalated calls are held until `Resolve` records a human decision under the approver's identity.
4. **Artifact.** One record per decision, chained to the previous record for the provider.
5. **Events.** `toolcall.requested`, `toolcall.decided`, then `toolcall.executed`, `toolcall.refused` or `toolcall.escalated`; a resolved hold adds `toolcall.approved` (or `toolcall.refused`). The AEX `events.Publisher` satisfies the gate's `Publisher` interface, so records land in JetStream with the standard envelope.

## The artifact

| Field | Meaning |
|---|---|
| `ts` | when the decision was made |
| `agent_id`, `principal`, `session_id` | who acted, for whom, in which session |
| `tool`, `args` | what was requested (`args` only in `ModeFull`, by design) |
| `decision`, `rule`, `scope` | allow, deny or escalate; the rule that decided; the scope consulted |
| `approval`, `approver` | automatic, pending-human, human-approved or human-rejected, and who |
| `outcome` | executed, refused, held, or the executor's error |
| `integrity`, `prev_hash`, `hash` | hash-chained; `VerifyChain` detects an edited or removed record |
| `tenant_id`, `contract_id`, `cert_id`, `trace_id` | joins to the tenant, the contract, the agent's ACA certificate, and the gateway's `X-Request-ID` |

`VerifyChain(records)` walks a provider's records and reports the first one whose hash or predecessor does not match. `Score(records, ruleCount)` computes the five auditability dimensions for any record set, so a deployment can measure what its records are worth.

## Using it

```go
policy, err := toolgate.LoadPolicy("policy.json")
gate, err := toolgate.New(policy, toolgate.WithPublisher(events.NewPublisherWithNATS("my-provider", nc)))

artifact, err := gate.Authorize(ctx, toolgate.Call{
    ID: reqID, Tool: "send_payment", Args: args,
    Principal: principal, AgentID: agentID, SessionID: sessionID,
    TenantID: tenantID, ContractID: contractID, CertID: certID, TraceID: traceID,
}, func(ctx context.Context, c toolgate.Call) (string, error) {
    return payments.Send(ctx, c.Args) // runs only when allowed
})
switch {
case errors.Is(err, toolgate.ErrDenied):    // refused; artifact says which rule
case errors.Is(err, toolgate.ErrEscalated): // held; artifact.Hash is the approval handle
}
```

A policy file names the provider, the granted scopes, the scope each tool needs, the rules, and any lookup tables. `src/internal/toolgate/testdata/policy.json` is a complete example for an accounts-payable assistant.

## The service form: `aex-toolgate`

`src/aex-toolgate` is the gate as an authorizing reverse proxy a provider runs in front of its tool endpoint, for providers whose tools are not written in Go (a Python MCP server, a playbook runner) or that want one gate for many tools.

```
agent  ->  aex-gateway  ->  aex-toolgate  ->  provider tools
           (API key,        (scope, then        (execute)
            route scope,     argument rules,
            X-Scopes)        artifact, events)
```

| Endpoint | What it does |
|---|---|
| `POST /v1/tools/{tool}` | body: the argument values as a JSON object. The gate decides; an allowed call is forwarded to `UPSTREAM_URL` + `UPSTREAM_PREFIX` + `/{tool}` and the tool's answer is returned with `X-Toolgate-Decision`, `X-Toolgate-Rule` and `X-Toolgate-Hash`. A refused call answers 403 with the rule and the message; a held call answers 202 with the hold hash. |
| `POST /v1/holds/{hash}` | **operator only.** Body `{"approver": "...", "approved": true}`. Settles a held call: runs the tool when approved, appends the human decision to the chain, publishes `toolcall.approved` or `toolcall.refused`. |
| `GET /v1/records`, `GET /v1/records/verify` | **operator only.** The provider's chain and its verification. |
| `GET /health`, `GET /ready` | probes |

Identity headers: `X-Tenant-ID` and `X-Request-ID` (the gateway sets both; the request id becomes the artifact's `trace_id`, the join key between the gateway's line and the gate's record), `X-Scopes` (the gateway forwards the validated scopes; they intersect with the policy's `granted_scopes` and can only narrow the grant), `X-AEX-Agent-ID`, `X-AEX-Principal`, `X-AEX-Session-ID`, `X-AEX-Contract-ID`, `X-AEX-Cert-ID`, `X-AEX-Call-ID`.

**Trust boundary.** The identity headers are attribution, not authentication: they are whatever the caller sent. The gate is designed to sit behind the gateway, which sets them, and it never lets a header widen an authorization decision. The operator endpoints are the exception and are not part of that model at all — settling a held call and reading the record chain are for the provider's operator, not for the agent being gated (an agent that can settle its own hold defeats the `escalate` effect, and one that can read the chain reads every other call's argument values). Both require `Authorization: Bearer $OPERATOR_TOKEN`, and with no token configured they are refused rather than open. A shared operator token authorizes the action; the `approver` in the body is the attribution claim recorded alongside it. Per-approver credentials are the next step.

Configuration: `PORT` (8090), `POLICY_FILE` (**required**; the gate refuses to start without a provider policy rather than fall back to a fixture), `UPSTREAM_URL`, `UPSTREAM_PREFIX` (`/tools`), `MODE` (`full` or `scope`), `NATS_URL` (optional; with it every event goes to the TOOLCALL stream), `UPSTREAM_TIMEOUT_SECONDS`, `OPERATOR_TOKEN` (required to use the operator endpoints), `EXPOSE_ARGS` (`true` serves full argument values on `GET /v1/records`; off by default because in `full` mode an artifact carries payment amounts and account numbers verbatim).

**The record chain is in memory and process-local.** A restart begins a new chain at genesis, and `VerifyChain` reports `ok` over any self-consistent chain, so a fresh chain and a truncated one look alike. `GET /v1/records/verify` therefore reports `chain_id`, which changes on every restart, and `publish_failures`, which is non-zero when an event could not reach JetStream and the durable copy is incomplete. The durable record is the stream, not the gate's memory; persisting the chain and anchoring it externally is still open.

## What changed in the gateway

Wiring the real gateway in front of the gate surfaced three defects, fixed with tests in `hack/tests/gateway_http_test.go`:

1. `httpapi.NewRouter` built an empty in-memory API-key table, so the identity-service validation path existed but was never used and every API key was refused. The router now validates against `IDENTITY_URL` (`API_KEY_VALIDATOR=identity`, the default); `API_KEY_VALIDATOR=memory` keeps the empty table for tests.
2. Scopes were validated, cached and placed in the request context, and nothing checked them against a route. `middleware.RequireScope` now enforces a route scope map after authentication: `DefaultRouteScopes` (reads need `read`, writes need the scope of the resource) merged with `ROUTE_SCOPES_FILE` (`{"METHOD /prefix": "scope"}`); a grant of `*` covers everything; an unmapped route requires `*` (fail closed). A refused request gets `403 insufficient_scope` with `required_scope` named.
3. The gateway expected `{"valid": true, ...}` from `/internal/v1/apikeys/validate` and the identity service never sent `valid`. The identity service now says `valid: true`, and the gateway accepts a 200 with a tenant whether or not the field is present.

Also: the request log line is a JSON record through slog carrying `request_id`, `tenant_id`, `scope_required` and `scope_decision` (a per-request holder lets inner middleware report to the outer logger; before, the line's request id and tenant were always empty); `TOOLS_URL` mounts `/v1/tools/` for a provider's tool endpoint; the proxy forwards the validated scopes as `X-Scopes`.

## Reproducing the comparison

```bash
cd hack/toolgate-harness
go run .                       # runs 1 and 2a: records, decision matrix and scores in ./out
go run . -nats nats://localhost:4222   # run 2b: the same, published to the TOOLCALL stream
```

Run 2c, the live path, needs the services up (`aex-identity` with MongoDB, `aex-gateway` with `TOOLS_URL`, `aex-toolgate`, and the `hack/ap-tools` mock provider), then:

```bash
cd hack/toolgate-client
go run . run -config B -calls ../../src/internal/toolgate/testdata/calls.json \
  -target http://localhost:8080/v1/tools -identity http://localhost:8087 -out out
go run . run -config C ... -records http://localhost:8090 -approve-held -out out   # gateway TOOLS_URL pointed at the gate
go run . score -calls ... -policy ... -ap-log ap-tools.log -gw-log gateway-B.log -gw-tenant <id> \
  -records out/artifacts_C.jsonl -gw-log-c gateway-C.log -gw-tenant-c <id> -out out/scored
```

The client creates a tenant and an API key through the identity service, replays the calls with the study's identity headers, and scores each configuration from its own record: the provider's log line (A), the gateway's request line (B), the gate's artifact (C). On an OpenShift lab cluster the decisions matched runs 1, 2a and 2b call for call; the gateway's lines for a legitimate and a redirected payment were identical apart from time and request id, and the gate refused the redirected one on `P3-recipient` with the values in the record.

## Status and roadmap

Library, harness, the service form and the gateway fixes are in. Next: policy storage in MongoDB with a per-provider policy API, an approval view on the dashboard that calls `/v1/holds/{hash}`, a `toolcall.escalated` consumer that opens the approval item, and the settlement link so an auditor can walk from a ledger entry back to the decision that allowed the payment. This is the governance piece of the Phase B roadmap.
