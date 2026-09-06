package middleware

import "context"

// RequestInfo is a per-request holder that the outermost middleware (Logging)
// places in the context so inner middleware can report what they did:
// RequestID fills the id, Auth the tenant, RequireScope the scope decision.
// Context values only flow inward, so without the holder the log line could
// not carry them.
type RequestInfo struct {
	RequestID string
	TenantID  string
	Scope     ScopeDecision
}

const requestInfoKey contextKey = "request_info"

// WithRequestInfo attaches a fresh holder to the context.
func WithRequestInfo(ctx context.Context) (context.Context, *RequestInfo) {
	ri := &RequestInfo{}
	return context.WithValue(ctx, requestInfoKey, ri), ri
}

// GetRequestInfo returns the request's holder, or nil outside the middleware stack.
func GetRequestInfo(ctx context.Context) *RequestInfo {
	if ri, ok := ctx.Value(requestInfoKey).(*RequestInfo); ok {
		return ri
	}
	return nil
}
