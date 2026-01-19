package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parlakisik/agent-exchange/aex-telemetry/internal/httpapi"
	"github.com/parlakisik/agent-exchange/aex-telemetry/internal/model"
	"github.com/parlakisik/agent-exchange/aex-telemetry/internal/service"
	"github.com/parlakisik/agent-exchange/aex-telemetry/internal/store"
)

func setupTestServer() *httptest.Server {
	memStore := store.NewMemoryStore(1000, 1000)
	svc := service.New(memStore)
	router := httpapi.NewRouter(svc)
	return httptest.NewServer(router)
}

func TestHealthEndpoint(t *testing.T) {
	ts := setupTestServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestIngestAndQueryLogs(t *testing.T) {
	ts := setupTestServer()
	defer ts.Close()

	// Ingest logs
	logs := []model.LogEntry{
		{
			Level:   "info",
			Service: "test-service",
			Message: "Test log message 1",
		},
		{
			Level:   "error",
			Service: "test-service",
			Message: "Test error message",
		},
		{
			Level:   "info",
			Service: "other-service",
			Message: "Other service log",
		},
	}

	body, _ := json.Marshal(logs)
	resp, err := http.Post(ts.URL+"/v1/logs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	// Query all logs
	resp, err = http.Get(ts.URL + "/v1/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Logs  []model.LogEntry `json:"logs"`
		Count int              `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result.Count != 3 {
		t.Fatalf("expected 3 logs, got %d", result.Count)
	}

	// Query by service
	resp, err = http.Get(ts.URL + "/v1/logs?service=test-service")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result.Count != 2 {
		t.Fatalf("expected 2 logs for test-service, got %d", result.Count)
	}

	// Query by level
	resp, err = http.Get(ts.URL + "/v1/logs?level=error")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result.Count != 1 {
		t.Fatalf("expected 1 error log, got %d", result.Count)
	}
}

func TestIngestAndQueryMetrics(t *testing.T) {
	ts := setupTestServer()
	defer ts.Close()

	// Ingest metrics
	metrics := []model.MetricEntry{
		{
			Name:    "requests_total",
			Type:    model.MetricTypeCounter,
			Value:   100,
			Service: "test-service",
			Labels:  map[string]string{"status": "200"},
		},
		{
			Name:    "requests_total",
			Type:    model.MetricTypeCounter,
			Value:   5,
			Service: "test-service",
			Labels:  map[string]string{"status": "500"},
		},
		{
			Name:    "latency_ms",
			Type:    model.MetricTypeGauge,
			Value:   45.5,
			Service: "test-service",
		},
	}

	body, _ := json.Marshal(metrics)
	resp, err := http.Post(ts.URL+"/v1/metrics", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	// Query all metrics
	resp, err = http.Get(ts.URL + "/v1/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Metrics []model.MetricEntry `json:"metrics"`
		Count   int                 `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result.Count != 3 {
		t.Fatalf("expected 3 metrics, got %d", result.Count)
	}

	// Query by name
	resp, err = http.Get(ts.URL + "/v1/metrics?name=requests_total")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result.Count != 2 {
		t.Fatalf("expected 2 metrics named requests_total, got %d", result.Count)
	}
}

func TestIngestAndQuerySpans(t *testing.T) {
	ts := setupTestServer()
	defer ts.Close()

	traceID := "trace-12345"

	// Ingest spans
	spans := []model.TraceSpan{
		{
			TraceID:    traceID,
			SpanID:     "span-1",
			Service:    "gateway",
			Operation:  "HTTP POST /v1/work",
			StartTime:  time.Now().Add(-100 * time.Millisecond),
			EndTime:    time.Now(),
			DurationMs: 100,
			Status:     "OK",
		},
		{
			TraceID:      traceID,
			SpanID:       "span-2",
			ParentSpanID: "span-1",
			Service:      "work-publisher",
			Operation:    "CreateWork",
			StartTime:    time.Now().Add(-80 * time.Millisecond),
			EndTime:      time.Now().Add(-10 * time.Millisecond),
			DurationMs:   70,
			Status:       "OK",
		},
	}

	body, _ := json.Marshal(spans)
	resp, err := http.Post(ts.URL+"/v1/spans", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	// Query trace
	resp, err = http.Get(ts.URL + "/v1/traces/" + traceID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		TraceID string            `json:"trace_id"`
		Spans   []model.TraceSpan `json:"spans"`
		Count   int               `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result.Count != 2 {
		t.Fatalf("expected 2 spans, got %d", result.Count)
	}
	if result.TraceID != traceID {
		t.Fatalf("expected trace_id=%s, got %s", traceID, result.TraceID)
	}
}

func TestGetStats(t *testing.T) {
	ts := setupTestServer()
	defer ts.Close()

	// Add some data first
	logs := []model.LogEntry{{Level: "info", Message: "test"}}
	body, _ := json.Marshal(logs)
	resp, _ := http.Post(ts.URL+"/v1/logs", "application/json", bytes.NewReader(body))
	_ = resp.Body.Close()

	// Get stats
	resp, err := http.Get(ts.URL + "/v1/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var stats map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}

	if stats["log_count"].(float64) != 1 {
		t.Fatalf("expected log_count=1, got %v", stats["log_count"])
	}
}
