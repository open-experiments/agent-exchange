// Package toolgate authorizes individual agent tool calls and records an
// audit artifact for each one.
//
// AEX authorizes at the gateway: an API key carries scopes, and a request
// either has the scope its route needs or it does not. That decision is made
// per request, on the tool's name, and never on the values the agent passes.
// Tool access at the MCP layer is provider-managed, so once a contract is
// awarded nothing in AEX decides or records an individual tool call.
//
// This package closes that gap on the provider side. A Gate sits in front of a
// provider agent's tool handlers. For every call it decides allow, deny or
// escalate, first on scope and then on argument-value rules, and it emits an
// artifact from which a reader who was not present can reconstruct what was
// requested, what was decided, on which rule, for whom, and what happened.
// Artifacts are hash-chained per provider and published as toolcall.* events
// so they land in the TOOLCALL JetStream stream beside the CERTIFICATE stream.
//
// The nine fields of the artifact (timestamp, agent identity, tool name,
// argument values, decision, rule or scope, approval status, outcome, record
// integrity) and the five auditability scores in Score follow the harness
// study that motivated the component; see docs/TOOLGATE.md.
package toolgate
