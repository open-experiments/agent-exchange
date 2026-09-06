package middleware

import "testing"

// A route prefix must match on whole path segments. Plain HasPrefix let a
// sibling inherit a shorter route's scope, which is silent under-protection:
// the request succeeds under the wrong scope rather than being refused.
func TestRequiredMatchesWholeSegmentsOnly(t *testing.T) {
	rs := RouteScopes{
		"POST /v1/work":                "work:submit",
		"POST /v1/tools/list_invoices": "invoices:read",
	}
	cases := []struct {
		path string
		want string
	}{
		{"/v1/work", "work:submit"},
		{"/v1/work/123", "work:submit"},
		{"/v1/workflows", "*"},
		{"/v1/workflows/run", "*"},
		{"/v1/tools/list_invoices", "invoices:read"},
		{"/v1/tools/list_invoices_and_delete", "*"},
		{"/v1/tools/list_invoices/page/2", "invoices:read"},
	}
	for _, c := range cases {
		if got := rs.Required("POST", c.path); got != c.want {
			t.Errorf("Required(POST, %q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// A prefix written with a trailing slash keeps matching everything under it.
func TestRequiredHonoursTrailingSlashPrefixes(t *testing.T) {
	rs := RouteScopes{"GET /v1/": "read"}
	for _, p := range []string{"/v1/", "/v1/work", "/v1/anything/at/all"} {
		if got := rs.Required("GET", p); got != "read" {
			t.Errorf("Required(GET, %q) = %q, want read", p, got)
		}
	}
}
