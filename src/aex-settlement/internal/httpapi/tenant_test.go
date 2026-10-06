package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/store"
)

// tenantTestRouter serves a store where tenants alice and bob both hold a
// balance, so a response's tenant_id shows whose data was read.
func tenantTestRouter(t *testing.T) (http.Handler, store.SettlementStore) {
	t.Helper()
	st := store.NewMemoryStore()
	for _, tenant := range []string{"alice", "bob"} {
		if _, err := st.IncrementBalance(context.Background(), tenant, 10000, "USD"); err != nil {
			t.Fatal(err)
		}
	}
	return newTestRouter(t, st), st
}

func doTenantRequest(t *testing.T, h http.Handler, method, target, headerTenant, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if headerTenant != "" {
		req.Header.Set("X-Tenant-ID", headerTenant)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp
}

func TestTenantScopedReads(t *testing.T) {
	cases := []struct {
		name         string
		query        string
		headerTenant string
		wantCode     int
		wantErrCode  string
		wantTenant   string
	}{
		{"header only", "", "alice", http.StatusOK, "", "alice"},
		{"header and matching param", "?tenant_id=alice", "alice", http.StatusOK, "", "alice"},
		{"header and mismatching param", "?tenant_id=bob", "alice", http.StatusForbidden, "TENANT_MISMATCH", ""},
		{"no header uses param", "?tenant_id=bob", "", http.StatusOK, "", "bob"},
		{"neither", "", "", http.StatusBadRequest, "TENANT_ID_REQUIRED", ""},
	}
	for _, path := range []string{"/v1/balance", "/v1/usage", "/v1/usage/transactions"} {
		for _, c := range cases {
			t.Run(path+"/"+c.name, func(t *testing.T) {
				h, _ := tenantTestRouter(t)
				code, resp := doTenantRequest(t, h, http.MethodGet, path+c.query, c.headerTenant, "")
				if code != c.wantCode {
					t.Fatalf("status = %d, want %d (%v)", code, c.wantCode, resp)
				}
				if c.wantErrCode != "" && errorCode(resp) != c.wantErrCode {
					t.Fatalf("error code = %q, want %q", errorCode(resp), c.wantErrCode)
				}
				// The transactions response carries no tenant_id.
				if c.wantTenant != "" && path != "/v1/usage/transactions" && resp["tenant_id"] != c.wantTenant {
					t.Fatalf("tenant_id = %v, want %q", resp["tenant_id"], c.wantTenant)
				}
			})
		}
	}
}

func TestTransactionsUseAuthenticatedTenant(t *testing.T) {
	h, _ := tenantTestRouter(t)
	if code, resp := doTenantRequest(t, h, http.MethodPost, "/v1/deposits", "", `{"tenant_id":"bob","amount":"5.00"}`); code != http.StatusCreated {
		t.Fatalf("deposit: %d %v", code, resp)
	}

	code, resp := doTenantRequest(t, h, http.MethodGet, "/v1/usage/transactions", "alice", "")
	if code != http.StatusOK || resp["count"] != float64(0) {
		t.Fatalf("alice sees bob's transactions: %d %v", code, resp)
	}
	code, resp = doTenantRequest(t, h, http.MethodGet, "/v1/usage/transactions", "bob", "")
	if code != http.StatusOK || resp["count"] != float64(1) {
		t.Fatalf("bob transactions: %d %v, want count 1", code, resp)
	}
}

func TestDepositTenant(t *testing.T) {
	cases := []struct {
		name         string
		body         string
		headerTenant string
		wantCode     int
		wantErrCode  string
		wantTenant   string
	}{
		{"header only", `{"amount":"5.00"}`, "alice", http.StatusCreated, "", "alice"},
		{"header and matching body", `{"tenant_id":"alice","amount":"5.00"}`, "alice", http.StatusCreated, "", "alice"},
		{"header and mismatching body", `{"tenant_id":"bob","amount":"5.00"}`, "alice", http.StatusForbidden, "TENANT_MISMATCH", ""},
		{"no header uses body", `{"tenant_id":"bob","amount":"5.00"}`, "", http.StatusCreated, "", "bob"},
		{"no header no tenant", `{"amount":"5.00"}`, "", http.StatusBadRequest, "BAD_REQUEST", ""},
		{"header without amount", `{"tenant_id":"alice"}`, "alice", http.StatusBadRequest, "BAD_REQUEST", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, st := tenantTestRouter(t)
			code, resp := doTenantRequest(t, h, http.MethodPost, "/v1/deposits", c.headerTenant, c.body)
			if code != c.wantCode {
				t.Fatalf("status = %d, want %d (%v)", code, c.wantCode, resp)
			}
			if c.wantErrCode != "" && errorCode(resp) != c.wantErrCode {
				t.Fatalf("error code = %q, want %q", errorCode(resp), c.wantErrCode)
			}
			if c.wantTenant != "" && resp["tenant_id"] != c.wantTenant {
				t.Fatalf("tenant_id = %v, want %q", resp["tenant_id"], c.wantTenant)
			}

			// Only the tenant the deposit was for gains money.
			for _, tenant := range []string{"alice", "bob"} {
				bal, err := st.GetBalance(context.Background(), tenant)
				if err != nil {
					t.Fatal(err)
				}
				want := int64(10000)
				if tenant == c.wantTenant {
					want += 500
				}
				if bal.Balance != want {
					t.Errorf("%s balance = %d, want %d", tenant, bal.Balance, want)
				}
			}
		})
	}
}
