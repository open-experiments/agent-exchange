package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/model"
	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/service"
	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/store"
)

const validWorkBody = `{"category":"general","description":"Test work","budget":{"max_price":100}}`

// failingStore is a WorkStore whose reads fail with a non-not-found error,
// standing in for a database outage.
type failingStore struct {
	*store.MemoryStore
	err error
}

func (s failingStore) GetWork(context.Context, string) (model.WorkSpec, error) {
	return model.WorkSpec{}, s.err
}

func newTestRouter(st store.WorkStore) http.Handler {
	return NewRouter(service.New(st, "", nil))
}

func doRequest(t *testing.T, h http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response %q: %v", rec.Body.String(), err)
	}
	return resp.Error.Code
}

func TestSubmitWorkRequiresConsumerIdentity(t *testing.T) {
	st := store.NewMemoryStore()
	h := newTestRouter(st)

	for _, headers := range []map[string]string{
		nil,
		{"X-Consumer-ID": ""},
		{"X-Tenant-ID": "   "},
	} {
		rec := doRequest(t, h, http.MethodPost, "/v1/work", validWorkBody, headers)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("headers %v: status = %d, want %d (body %s)", headers, rec.Code, http.StatusUnauthorized, rec.Body.String())
		}
		if got := errorCode(t, rec); got != "CONSUMER_ID_REQUIRED" {
			t.Errorf("headers %v: error code = %q, want CONSUMER_ID_REQUIRED", headers, got)
		}
	}

	works, err := st.ListWork(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(works) != 0 {
		t.Errorf("rejected submissions were stored: %d work items", len(works))
	}
}

func TestSubmitWorkStoresConsumerIdentity(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"consumer header", map[string]string{"X-Consumer-ID": "tenant_direct"}, "tenant_direct"},
		{"gateway tenant header", map[string]string{"X-Tenant-ID": "tenant_gateway"}, "tenant_gateway"},
		{
			"gateway tenant wins over client-supplied consumer",
			map[string]string{"X-Tenant-ID": "tenant_gateway", "X-Consumer-ID": "tenant_spoofed"},
			"tenant_gateway",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := store.NewMemoryStore()
			h := newTestRouter(st)

			rec := doRequest(t, h, http.MethodPost, "/v1/work", validWorkBody, tt.headers)
			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusCreated, rec.Body.String())
			}
			var resp model.WorkResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			work, err := st.GetWork(context.Background(), resp.WorkID)
			if err != nil {
				t.Fatalf("GetWork: %v", err)
			}
			if work.ConsumerID != tt.want {
				t.Errorf("stored consumer_id = %q, want %q", work.ConsumerID, tt.want)
			}
		})
	}
}

func TestSubmitWorkInvalidSpecIsBadRequest(t *testing.T) {
	h := newTestRouter(store.NewMemoryStore())

	rec := doRequest(t, h, http.MethodPost, "/v1/work", `{}`, map[string]string{"X-Consumer-ID": "tenant_001"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if got := errorCode(t, rec); got != "VALIDATION_ERROR" {
		t.Errorf("error code = %q, want VALIDATION_ERROR", got)
	}
}

func TestGetWorkNotFound(t *testing.T) {
	h := newTestRouter(store.NewMemoryStore())

	rec := doRequest(t, h, http.MethodGet, "/v1/work/work_missing", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if got := errorCode(t, rec); got != "NOT_FOUND" {
		t.Errorf("error code = %q, want NOT_FOUND", got)
	}
}

func TestGetWorkStoreErrorIsInternal(t *testing.T) {
	st := failingStore{MemoryStore: store.NewMemoryStore(), err: errors.New("connection refused")}
	h := newTestRouter(st)

	rec := doRequest(t, h, http.MethodGet, "/v1/work/work_any", "", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if got := errorCode(t, rec); got != "INTERNAL_ERROR" {
		t.Errorf("error code = %q, want INTERNAL_ERROR", got)
	}
}

func TestCancelWorkRequiresConsumerIdentity(t *testing.T) {
	st := store.NewMemoryStore()
	h := newTestRouter(st)

	rec := doRequest(t, h, http.MethodPost, "/v1/work", validWorkBody, map[string]string{"X-Consumer-ID": "tenant_001"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var resp model.WorkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	rec = doRequest(t, h, http.MethodPost, "/v1/work/"+resp.WorkID+"/cancel", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("cancel status = %d, want %d (body %s)", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	work, err := st.GetWork(context.Background(), resp.WorkID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if work.State != model.WorkStateOpen {
		t.Errorf("work state = %s, want %s", work.State, model.WorkStateOpen)
	}
}

func TestCancelWorkStoreErrorIsInternal(t *testing.T) {
	st := failingStore{MemoryStore: store.NewMemoryStore(), err: errors.New("connection refused")}
	h := newTestRouter(st)

	rec := doRequest(t, h, http.MethodPost, "/v1/work/work_any/cancel", "", map[string]string{"X-Consumer-ID": "tenant_001"})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}
