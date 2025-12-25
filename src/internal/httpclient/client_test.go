package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client := NewClient("test-service", 10*time.Second)

	if client == nil {
		t.Fatal("NewClient() returned nil")
	}

	if client.serviceName != "test-service" {
		t.Errorf("NewClient() serviceName = %v, want test-service", client.serviceName)
	}

	if client.httpClient == nil {
		t.Error("NewClient() did not initialize httpClient")
	}

	if client.httpClient.Timeout != 10*time.Second {
		t.Errorf("NewClient() timeout = %v, want %v", client.httpClient.Timeout, 10*time.Second)
	}
}

func TestNewRequest(t *testing.T) {
	req := NewRequest("GET", "http://example.com")

	if req == nil {
		t.Fatal("NewRequest() returned nil")
	}

	if req.method != "GET" {
		t.Errorf("NewRequest() method = %v, want GET", req.method)
	}

	if req.baseURL != "http://example.com" {
		t.Errorf("NewRequest() baseURL = %v, want http://example.com", req.baseURL)
	}

	if req.headers == nil {
		t.Error("NewRequest() did not initialize headers map")
	}

	if req.query == nil {
		t.Error("NewRequest() did not initialize query")
	}
}

func TestRequestBuilder_Path(t *testing.T) {
	req := NewRequest("GET", "http://example.com").
		Path("/api/v1/resource")

	if req.path != "/api/v1/resource" {
		t.Errorf("Path() = %v, want /api/v1/resource", req.path)
	}
}

func TestRequestBuilder_Query(t *testing.T) {
	req := NewRequest("GET", "http://example.com").
		Query("limit", "10").
		Query("offset", "20")

	if req.query.Get("limit") != "10" {
		t.Errorf("Query() limit = %v, want 10", req.query.Get("limit"))
	}

	if req.query.Get("offset") != "20" {
		t.Errorf("Query() offset = %v, want 20", req.query.Get("offset"))
	}
}

func TestRequestBuilder_Header(t *testing.T) {
	req := NewRequest("GET", "http://example.com").
		Header("Content-Type", "application/json").
		Header("X-Custom-Header", "value")

	if len(req.headers) != 2 {
		t.Errorf("Header() count = %v, want 2", len(req.headers))
	}

	if req.headers["Content-Type"] != "application/json" {
		t.Errorf("Header() Content-Type = %v, want application/json", req.headers["Content-Type"])
	}
}

func TestRequestBuilder_JSON(t *testing.T) {
	data := map[string]interface{}{
		"name":  "test",
		"count": 42,
	}

	req := NewRequest("POST", "http://example.com").
		JSON(data)

	if req.body == nil {
		t.Fatal("JSON() did not set body")
	}

	// Verify Content-Type header was set
	if req.headers["Content-Type"] != "application/json" {
		t.Errorf("JSON() Content-Type = %v, want application/json", req.headers["Content-Type"])
	}
}

func TestRequestBuilder_Context(t *testing.T) {
	ctx := context.Background()
	req := NewRequest("GET", "http://example.com").
		Context(ctx)

	if req.ctx != ctx {
		t.Error("Context() did not set context correctly")
	}
}

func TestExecute_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer server.Close()

	client := NewClient("test", 5*time.Second)
	req := NewRequest("GET", server.URL)

	resp, err := req.Execute(client)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Execute() status = %v, want %v", resp.StatusCode, http.StatusOK)
	}
}

func TestExecuteJSON_Success(t *testing.T) {
	responseData := map[string]interface{}{
		"id":   "123",
		"name": "test",
	}

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(responseData)
	}))
	defer server.Close()

	client := NewClient("test", 5*time.Second)
	req := NewRequest("GET", server.URL)

	var result map[string]interface{}
	err := req.ExecuteJSON(client, &result)
	if err != nil {
		t.Fatalf("ExecuteJSON() error: %v", err)
	}

	if result["id"] != "123" {
		t.Errorf("ExecuteJSON() id = %v, want 123", result["id"])
	}

	if result["name"] != "test" {
		t.Errorf("ExecuteJSON() name = %v, want test", result["name"])
	}
}

func TestExecuteJSON_ErrorResponse(t *testing.T) {
	// Create test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer server.Close()

	client := NewClient("test", 5*time.Second)
	req := NewRequest("GET", server.URL)

	var result map[string]interface{}
	err := req.ExecuteJSON(client, &result)
	if err == nil {
		t.Error("ExecuteJSON() expected error for 404 response, got nil")
	}

	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("ExecuteJSON() error type = %T, want *HTTPError", err)
	}

	if httpErr.StatusCode != http.StatusNotFound {
		t.Errorf("HTTPError StatusCode = %v, want %v", httpErr.StatusCode, http.StatusNotFound)
	}
}

func TestRetry_Success(t *testing.T) {
	attempts := 0

	// Create test server that succeeds on second attempt
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer server.Close()

	client := NewClient("test", 5*time.Second)
	req := NewRequest("GET", server.URL)

	resp, err := req.Execute(client)
	if err != nil {
		t.Fatalf("Execute() error after retry: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Execute() status = %v, want %v", resp.StatusCode, http.StatusOK)
	}

	if attempts != 2 {
		t.Errorf("Execute() attempts = %v, want 2", attempts)
	}
}

func TestRetry_MaxAttempts(t *testing.T) {
	attempts := 0

	// Create test server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient("test", 5*time.Second)
	req := NewRequest("GET", server.URL)

	_, err := req.Execute(client)
	if err == nil {
		t.Error("Execute() expected error after max retries, got nil")
	}

	// Should attempt maxRetries + 1 times (initial + retries)
	expectedAttempts := 4 // Initial + 3 retries
	if attempts != expectedAttempts {
		t.Errorf("Execute() attempts = %v, want %v", attempts, expectedAttempts)
	}
}

func TestBuild(t *testing.T) {
	tests := []struct {
		name    string
		builder *RequestBuilder
		wantErr bool
	}{
		{
			name:    "simple GET",
			builder: NewRequest("GET", "http://example.com"),
			wantErr: false,
		},
		{
			name:    "with path",
			builder: NewRequest("GET", "http://example.com").Path("/api/v1/resource"),
			wantErr: false,
		},
		{
			name:    "with query params",
			builder: NewRequest("GET", "http://example.com").Query("limit", "10"),
			wantErr: false,
		},
		{
			name:    "with JSON body",
			builder: NewRequest("POST", "http://example.com").JSON(map[string]string{"key": "value"}),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := tt.builder.Build()

			if tt.wantErr {
				if err == nil {
					t.Error("Build() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Build() unexpected error: %v", err)
			}

			if req == nil {
				t.Error("Build() returned nil request")
			}
		})
	}
}

func TestHTTPMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != method {
					t.Errorf("Server received method = %v, want %v", r.Method, method)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client := NewClient("test", 5*time.Second)
			req := NewRequest(method, server.URL)

			_, err := req.Execute(client)
			if err != nil {
				t.Errorf("Execute(%s) error: %v", method, err)
			}
		})
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxRetries != 3 {
		t.Errorf("DefaultRetryConfig() MaxRetries = %v, want 3", config.MaxRetries)
	}

	if config.InitialBackoff != 100*time.Millisecond {
		t.Errorf("DefaultRetryConfig() InitialBackoff = %v, want 100ms", config.InitialBackoff)
	}

	if config.MaxBackoff != 5*time.Second {
		t.Errorf("DefaultRetryConfig() MaxBackoff = %v, want 5s", config.MaxBackoff)
	}

	if len(config.RetryableStatuses) == 0 {
		t.Error("DefaultRetryConfig() RetryableStatuses is empty")
	}
}
