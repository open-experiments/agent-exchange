package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
)

// RouteScopes maps "METHOD /path/prefix" to the scope a key or token must
// hold to use it. Matching is by method and longest path prefix; the wildcard
// method "*" matches any method. A grant of "*" covers every scope.
//
// The map is fail closed: a route with no entry requires "*", so a key issued
// with custom scopes can reach only the routes the map names. Keys created
// with the default scopes (["*"]) are unaffected.
type RouteScopes map[string]string

// DefaultRouteScopes is the scope map for the AEX API surface. Reads need
// "read"; writes need the scope of the resource they change.
func DefaultRouteScopes() RouteScopes {
	return RouteScopes{
		"GET /v1/":               "read",
		"POST /v1/work":          "work:submit",
		"PUT /v1/work":           "work:submit",
		"DELETE /v1/work":        "work:submit",
		"POST /v1/bids":          "bids:submit",
		"POST /v1/contracts":     "contracts:write",
		"PUT /v1/contracts":      "contracts:write",
		"POST /v1/providers":     "providers:write",
		"PUT /v1/providers":      "providers:write",
		"DELETE /v1/providers":   "providers:write",
		"POST /v1/subscriptions": "providers:write",
		"POST /v1/capabilities":  "providers:write",
		"POST /v1/deposits":      "billing:write",
		"POST /v1/usage":         "billing:write",
		"POST /v1/tenants":       "tenants:write",
		"PUT /v1/tenants":        "tenants:write",
		"DELETE /v1/tenants":     "tenants:write",
		"POST /v1/certificates":  "certs:issue",
		"POST /v1/reputation":    "certs:issue",
		// Tool calls proxied to a provider's toolgate. Without an entry here
		// the route falls through to the "*" default, so the only key that
		// could reach a tool was one granted "*" -- no narrowing at all.
		//
		// This scope authorizes reaching the tool surface, nothing more. The
		// gate behind it enforces a scope PER TOOL from its own policy, and
		// those names come from a different vocabulary (payments:send,
		// invoices:read). A working least-privilege key therefore carries
		// both: "tools:invoke" to pass this check, plus the per-tool scopes
		// the provider's policy grants. A key holding only "tools:invoke"
		// passes here and is denied every individual tool downstream.
		"POST /v1/tools": "tools:invoke",
	}
}

// LoadRouteScopes returns the default map with the entries of a JSON file
// ({"METHOD /prefix": "scope"}) merged over it. An empty path returns the
// defaults.
func LoadRouteScopes(path string) (RouteScopes, error) {
	rs := DefaultRouteScopes()
	if path == "" {
		return rs, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("route scopes: read %s: %w", path, err)
	}
	var extra map[string]string
	if err := json.Unmarshal(b, &extra); err != nil {
		return nil, fmt.Errorf("route scopes: parse %s: %w", path, err)
	}
	for k, v := range extra {
		rs[k] = v
	}
	return rs, nil
}

// Required returns the scope a request needs, or "*" when no entry matches.
func (rs RouteScopes) Required(method, path string) string {
	best, bestLen := "*", -1
	for key, scope := range rs {
		m, prefix, ok := strings.Cut(key, " ")
		if !ok || prefix == "" {
			// An entry with an empty path ("POST ") would match every path and
			// outrank the fail-closed default, silently opening the whole
			// unmapped surface. Operators author these files by hand.
			continue
		}
		if m != "*" && m != method {
			continue
		}
		if matchPrefix(path, prefix) && len(prefix) > bestLen {
			best, bestLen = scope, len(prefix)
		}
	}
	return best
}

// matchPrefix reports whether path is prefix or sits under it, matching only
// on whole path segments. A plain strings.HasPrefix would let a sibling route
// inherit a shorter route's scope: "POST /v1/work" would cover a later
// "/v1/workflows", and "/v1/tools/list_invoices" would cover
// "/v1/tools/list_invoices_and_delete". The failure is silent
// under-protection, not a 403, so it has to be ruled out here.
func matchPrefix(path, prefix string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	if len(path) == len(prefix) || strings.HasSuffix(prefix, "/") {
		return true
	}
	return path[len(prefix)] == '/'
}

// Keys returns the map's entries sorted, for logs and docs.
func (rs RouteScopes) Keys() []string {
	keys := make([]string, 0, len(rs))
	for k := range rs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ScopeDecision is what the scope check decided for one request. Logging
// writes it into the request log line, so the line carries the scope consulted
// and the decision, the two fields a scope gate's record must hold to be
// checkable.
type ScopeDecision struct {
	Required string
	Granted  []string
	Allowed  bool
	Checked  bool
}

// RequireScope enforces the route scope map against the scopes Auth placed in
// the context. It must run after Auth. A request whose grant lacks the
// required scope gets 403 insufficient_scope with the scope named.
func RequireScope(rs RouteScopes) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			required := rs.Required(r.Method, r.URL.Path)
			granted := GetRoles(r.Context())
			allowed := false
			for _, s := range granted {
				if s == "*" || s == required {
					allowed = true
					break
				}
			}
			if ri := GetRequestInfo(r.Context()); ri != nil {
				ri.Scope = ScopeDecision{Required: required, Granted: granted, Allowed: allowed, Checked: true}
			}
			if !allowed {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":           "insufficient_scope",
						"message":        fmt.Sprintf("this route requires scope %q", required),
						"required_scope": required,
						"request_id":     GetRequestID(r.Context()),
					},
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
