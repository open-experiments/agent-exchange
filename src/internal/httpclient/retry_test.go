package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// bodyRecorder is a test server that records the body of every request it
// receives and delegates the response to respond, which is given the 1-based
// attempt number.
type bodyRecorder struct {
	mu      sync.Mutex
	bodies  []string
	respond func(w http.ResponseWriter, r *http.Request, attempt int)
}

func (b *bodyRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, _ := io.ReadAll(r.Body)
	b.mu.Lock()
	b.bodies = append(b.bodies, string(data))
	attempt := len(b.bodies)
	b.mu.Unlock()
	b.respond(w, r, attempt)
}

func (b *bodyRecorder) recorded() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.bodies...)
}

// newFastRetryClient returns a client whose backoff is short enough for tests.
func newFastRetryClient(timeout time.Duration) *Client {
	c := NewClient("test", timeout)
	c.retryConfig.InitialBackoff = time.Millisecond
	c.retryConfig.MaxBackoff = 5 * time.Millisecond
	return c
}

func failFirstWith(status int) func(http.ResponseWriter, *http.Request, int) {
	return func(w http.ResponseWriter, _ *http.Request, attempt int) {
		if attempt == 1 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("try again"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}

func assertBodies(t *testing.T, got []string, want string, minAttempts int) {
	t.Helper()
	if len(got) < minAttempts {
		t.Fatalf("server saw %d attempts, want at least %d", len(got), minAttempts)
	}
	for i, body := range got {
		if body != want {
			t.Errorf("attempt %d body = %q, want %q", i+1, body, want)
		}
	}
}

func TestRetry_PostReplaysBody(t *testing.T) {
	rec := &bodyRecorder{respond: failFirstWith(http.StatusServiceUnavailable)}
	server := httptest.NewServer(rec)
	defer server.Close()

	client := newFastRetryClient(5 * time.Second)
	resp, err := client.Post(context.Background(), server.URL, map[string]string{"name": "alpha"})
	if err != nil {
		t.Fatalf("Post() error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Post() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	got := rec.recorded()
	if len(got) != 2 {
		t.Fatalf("server saw %d attempts, want 2", len(got))
	}
	assertBodies(t, got, `{"name":"alpha"}`, 2)
}

func TestRetry_PutReplaysBody(t *testing.T) {
	rec := &bodyRecorder{respond: failFirstWith(http.StatusBadGateway)}
	server := httptest.NewServer(rec)
	defer server.Close()

	client := newFastRetryClient(5 * time.Second)
	resp, err := client.Put(context.Background(), server.URL, map[string]int{"n": 7})
	if err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Put() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	assertBodies(t, rec.recorded(), `{"n":7}`, 2)
}

func TestRetry_BuilderReplaysBody(t *testing.T) {
	rec := &bodyRecorder{respond: failFirstWith(http.StatusServiceUnavailable)}
	server := httptest.NewServer(rec)
	defer server.Close()

	client := newFastRetryClient(5 * time.Second)
	var result struct {
		OK bool `json:"ok"`
	}
	err := NewRequest(http.MethodPost, server.URL).
		Path("/items").
		JSON(map[string]string{"id": "x1"}).
		ExecuteJSON(client, &result)
	if err != nil {
		t.Fatalf("ExecuteJSON() error: %v", err)
	}
	if !result.OK {
		t.Error("ExecuteJSON() did not decode the success response")
	}
	assertBodies(t, rec.recorded(), `{"id":"x1"}`, 2)
}

func TestBuild_SetsGetBody(t *testing.T) {
	req, err := NewRequest(http.MethodPost, "http://example.com").
		JSON(map[string]string{"k": "v"}).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if req.GetBody == nil {
		t.Fatal("Build() request has a body but no GetBody")
	}
	body, err := req.GetBody()
	if err != nil {
		t.Fatalf("GetBody() error: %v", err)
	}
	data, _ := io.ReadAll(body)
	if string(data) != `{"k":"v"}` {
		t.Errorf("GetBody() = %q, want %q", data, `{"k":"v"}`)
	}
}

// opaqueReader hides the concrete reader type so http.NewRequest cannot set
// GetBody.
type opaqueReader struct{ r io.Reader }

func (o opaqueReader) Read(p []byte) (int, error) { return o.r.Read(p) }

func TestRetry_BodyWithoutGetBodyIsNotRetried(t *testing.T) {
	rec := &bodyRecorder{respond: func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}}
	server := httptest.NewServer(rec)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL, opaqueReader{strings.NewReader("payload")})
	if err != nil {
		t.Fatalf("NewRequest() error: %v", err)
	}
	if req.GetBody != nil {
		t.Fatal("test setup: expected request without GetBody")
	}

	client := newFastRetryClient(5 * time.Second)
	resp, err := client.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do() error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Do() status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	got := rec.recorded()
	if len(got) != 1 {
		t.Fatalf("server saw %d attempts, want 1", len(got))
	}
	assertBodies(t, got, "payload", 1)
}

func TestRetry_BodyWithoutGetBodyTransportErrorIsNotRetried(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		mu.Lock()
		attempts++
		mu.Unlock()
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL, opaqueReader{strings.NewReader("payload")})
	if err != nil {
		t.Fatalf("NewRequest() error: %v", err)
	}

	client := newFastRetryClient(5 * time.Second)
	if _, err := client.Do(context.Background(), req); err == nil {
		t.Fatal("Do() expected transport error, got nil")
	}

	mu.Lock()
	defer mu.Unlock()
	if attempts != 1 {
		t.Errorf("server saw %d attempts, want 1", attempts)
	}
}

func TestRetry_TimeoutReplaysBody(t *testing.T) {
	rec := &bodyRecorder{respond: func(w http.ResponseWriter, r *http.Request, attempt int) {
		if attempt == 1 {
			select {
			case <-time.After(2 * time.Second):
			case <-r.Context().Done():
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}}
	server := httptest.NewServer(rec)
	defer server.Close()

	client := newFastRetryClient(200 * time.Millisecond)
	resp, err := client.Post(context.Background(), server.URL, map[string]string{"slow": "yes"})
	if err != nil {
		t.Fatalf("Post() error after timeout retry: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Post() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	assertBodies(t, rec.recorded(), `{"slow":"yes"}`, 2)
}

func TestRetry_TransportErrorReplaysBody(t *testing.T) {
	rec := &bodyRecorder{respond: func(w http.ResponseWriter, _ *http.Request, attempt int) {
		if attempt == 1 {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				conn.Close()
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}}
	server := httptest.NewServer(rec)
	defer server.Close()

	client := newFastRetryClient(5 * time.Second)
	resp, err := client.Post(context.Background(), server.URL, map[string]string{"drop": "conn"})
	if err != nil {
		t.Fatalf("Post() error after transport retry: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Post() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	assertBodies(t, rec.recorded(), `{"drop":"conn"}`, 2)
}

func TestRetry_GetWithoutBodyStillRetries(t *testing.T) {
	rec := &bodyRecorder{respond: failFirstWith(http.StatusServiceUnavailable)}
	server := httptest.NewServer(rec)
	defer server.Close()

	client := newFastRetryClient(5 * time.Second)
	resp, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Get() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	assertBodies(t, rec.recorded(), "", 2)
}
