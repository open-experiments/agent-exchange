package integration

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestTelemetryLogIngestion tests log ingestion
func TestTelemetryLogIngestion(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	// Check if telemetry is available
	if err := c.HealthCheck(ctx, c.urls.Telemetry); err != nil {
		t.Skipf("Telemetry service not available: %v", err)
	}

	timestamp := time.Now().UnixNano()

	logs := []LogEntry{
		{
			Level:   "INFO",
			Service: fmt.Sprintf("test-service-%d", timestamp),
			Message: "Test log message 1",
			Fields: map[string]any{
				"request_id": "req-123",
				"user_id":    "user-456",
			},
		},
		{
			Level:   "WARN",
			Service: fmt.Sprintf("test-service-%d", timestamp),
			Message: "Test warning message",
			Fields: map[string]any{
				"warning_code": "W001",
			},
		},
		{
			Level:   "ERROR",
			Service: fmt.Sprintf("test-service-%d", timestamp),
			Message: "Test error message",
			Fields: map[string]any{
				"error_code": "E001",
				"stack":      "line 42",
			},
		},
	}

	accepted, err := c.IngestLogs(ctx, logs)
	if err != nil {
		t.Fatalf("Failed to ingest logs: %v", err)
	}

	if accepted != len(logs) {
		t.Errorf("Expected %d logs accepted, got %d", len(logs), accepted)
	}
	t.Logf("Ingested %d logs", accepted)
}

// TestTelemetryLogQuery tests log querying
func TestTelemetryLogQuery(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Telemetry); err != nil {
		t.Skipf("Telemetry service not available: %v", err)
	}

	timestamp := time.Now().UnixNano()
	serviceName := fmt.Sprintf("query-service-%d", timestamp)

	// Ingest logs
	logs := []LogEntry{
		{Level: "INFO", Service: serviceName, Message: "Query test 1"},
		{Level: "INFO", Service: serviceName, Message: "Query test 2"},
		{Level: "ERROR", Service: serviceName, Message: "Query test error"},
	}
	_, err := c.IngestLogs(ctx, logs)
	if err != nil {
		t.Fatalf("Failed to ingest logs: %v", err)
	}

	// Query all logs for service
	queriedLogs, err := c.QueryLogs(ctx, serviceName, "", 100)
	if err != nil {
		t.Fatalf("Failed to query logs: %v", err)
	}
	t.Logf("Queried %d logs for service %s", len(queriedLogs), serviceName)

	// Query only ERROR logs
	errorLogs, err := c.QueryLogs(ctx, serviceName, "ERROR", 100)
	if err != nil {
		t.Fatalf("Failed to query error logs: %v", err)
	}
	t.Logf("Queried %d ERROR logs", len(errorLogs))
}

// TestTelemetryMetricIngestion tests metric ingestion
func TestTelemetryMetricIngestion(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Telemetry); err != nil {
		t.Skipf("Telemetry service not available: %v", err)
	}

	timestamp := time.Now().UnixNano()
	serviceName := fmt.Sprintf("metric-service-%d", timestamp)

	metrics := []MetricEntry{
		{
			Name:    "http_request_duration_ms",
			Type:    "histogram",
			Value:   125.5,
			Service: serviceName,
			Labels: map[string]string{
				"method":      "GET",
				"status_code": "200",
			},
		},
		{
			Name:    "http_request_count",
			Type:    "counter",
			Value:   1,
			Service: serviceName,
			Labels: map[string]string{
				"method":      "POST",
				"status_code": "201",
			},
		},
		{
			Name:    "memory_usage_bytes",
			Type:    "gauge",
			Value:   524288000,
			Service: serviceName,
			Labels: map[string]string{
				"instance": "pod-1",
			},
		},
	}

	accepted, err := c.IngestMetrics(ctx, metrics)
	if err != nil {
		t.Fatalf("Failed to ingest metrics: %v", err)
	}

	if accepted != len(metrics) {
		t.Errorf("Expected %d metrics accepted, got %d", len(metrics), accepted)
	}
	t.Logf("Ingested %d metrics", accepted)
}

// TestTelemetryMetricQuery tests metric querying
func TestTelemetryMetricQuery(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Telemetry); err != nil {
		t.Skipf("Telemetry service not available: %v", err)
	}

	timestamp := time.Now().UnixNano()
	serviceName := fmt.Sprintf("metric-query-service-%d", timestamp)

	// Ingest metrics
	metrics := []MetricEntry{
		{Name: "test_counter", Type: "counter", Value: 100, Service: serviceName},
		{Name: "test_counter", Type: "counter", Value: 150, Service: serviceName},
		{Name: "test_gauge", Type: "gauge", Value: 42.5, Service: serviceName},
	}
	_, err := c.IngestMetrics(ctx, metrics)
	if err != nil {
		t.Fatalf("Failed to ingest metrics: %v", err)
	}

	// Query metrics by name
	queriedMetrics, err := c.QueryMetrics(ctx, "test_counter", serviceName, 100)
	if err != nil {
		t.Fatalf("Failed to query metrics: %v", err)
	}
	t.Logf("Queried %d test_counter metrics", len(queriedMetrics))

	// Query all metrics for service
	allMetrics, err := c.QueryMetrics(ctx, "", serviceName, 100)
	if err != nil {
		t.Fatalf("Failed to query all metrics: %v", err)
	}
	t.Logf("Queried %d total metrics for service", len(allMetrics))
}

// TestTelemetrySpanIngestion tests span ingestion
func TestTelemetrySpanIngestion(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Telemetry); err != nil {
		t.Skipf("Telemetry service not available: %v", err)
	}

	timestamp := time.Now().UnixNano()
	traceID := fmt.Sprintf("trace-%d", timestamp)
	serviceName := fmt.Sprintf("span-service-%d", timestamp)

	startTime := time.Now()
	spans := []TraceSpan{
		{
			TraceID:    traceID,
			SpanID:     "span-001",
			Service:    serviceName,
			Operation:  "HTTP GET /api/work",
			StartTime:  startTime.Format(time.RFC3339Nano),
			EndTime:    startTime.Add(100 * time.Millisecond).Format(time.RFC3339Nano),
			DurationMs: 100,
			Status:     "OK",
			Attributes: map[string]string{
				"http.method": "GET",
				"http.url":    "/api/work",
			},
		},
		{
			TraceID:      traceID,
			SpanID:       "span-002",
			ParentSpanID: "span-001",
			Service:      serviceName,
			Operation:    "MongoDB Query",
			StartTime:    startTime.Add(10 * time.Millisecond).Format(time.RFC3339Nano),
			EndTime:      startTime.Add(50 * time.Millisecond).Format(time.RFC3339Nano),
			DurationMs:   40,
			Status:       "OK",
			Attributes: map[string]string{
				"db.type":      "mongodb",
				"db.statement": "findOne",
			},
		},
	}

	accepted, err := c.IngestSpans(ctx, spans)
	if err != nil {
		t.Fatalf("Failed to ingest spans: %v", err)
	}

	if accepted != len(spans) {
		t.Errorf("Expected %d spans accepted, got %d", len(spans), accepted)
	}
	t.Logf("Ingested %d spans for trace %s", accepted, traceID)
}

// TestTelemetryTraceRetrieval tests trace retrieval
func TestTelemetryTraceRetrieval(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Telemetry); err != nil {
		t.Skipf("Telemetry service not available: %v", err)
	}

	timestamp := time.Now().UnixNano()
	traceID := fmt.Sprintf("retrieve-trace-%d", timestamp)
	serviceName := fmt.Sprintf("trace-service-%d", timestamp)

	// Ingest a complete trace
	startTime := time.Now()
	spans := []TraceSpan{
		{
			TraceID:    traceID,
			SpanID:     "root-span",
			Service:    serviceName,
			Operation:  "ProcessRequest",
			StartTime:  startTime.Format(time.RFC3339Nano),
			EndTime:    startTime.Add(200 * time.Millisecond).Format(time.RFC3339Nano),
			DurationMs: 200,
			Status:     "OK",
		},
		{
			TraceID:      traceID,
			SpanID:       "child-span-1",
			ParentSpanID: "root-span",
			Service:      serviceName,
			Operation:    "ValidateInput",
			StartTime:    startTime.Add(5 * time.Millisecond).Format(time.RFC3339Nano),
			EndTime:      startTime.Add(20 * time.Millisecond).Format(time.RFC3339Nano),
			DurationMs:   15,
			Status:       "OK",
		},
		{
			TraceID:      traceID,
			SpanID:       "child-span-2",
			ParentSpanID: "root-span",
			Service:      serviceName,
			Operation:    "ExecuteLogic",
			StartTime:    startTime.Add(25 * time.Millisecond).Format(time.RFC3339Nano),
			EndTime:      startTime.Add(180 * time.Millisecond).Format(time.RFC3339Nano),
			DurationMs:   155,
			Status:       "OK",
		},
	}

	_, err := c.IngestSpans(ctx, spans)
	if err != nil {
		t.Fatalf("Failed to ingest spans: %v", err)
	}

	// Retrieve trace
	retrievedSpans, err := c.GetTrace(ctx, traceID)
	if err != nil {
		t.Fatalf("Failed to retrieve trace: %v", err)
	}

	if len(retrievedSpans) != len(spans) {
		t.Errorf("Expected %d spans, got %d", len(spans), len(retrievedSpans))
	}

	for _, span := range retrievedSpans {
		t.Logf("Span: %s (%s) - %dms", span.SpanID, span.Operation, span.DurationMs)
	}
}

// TestTelemetryStats tests stats endpoint
func TestTelemetryStats(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Telemetry); err != nil {
		t.Skipf("Telemetry service not available: %v", err)
	}

	stats, err := c.GetTelemetryStats(ctx)
	if err != nil {
		t.Logf("Stats endpoint not available: %v", err)
		return
	}

	t.Logf("Telemetry stats: %v", stats)
}

// TestTelemetryLogLevels tests different log levels
func TestTelemetryLogLevels(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Telemetry); err != nil {
		t.Skipf("Telemetry service not available: %v", err)
	}

	timestamp := time.Now().UnixNano()
	serviceName := fmt.Sprintf("level-service-%d", timestamp)

	levels := []string{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"}

	for _, level := range levels {
		logs := []LogEntry{
			{
				Level:   level,
				Service: serviceName,
				Message: fmt.Sprintf("Test %s message", level),
			},
		}

		_, err := c.IngestLogs(ctx, logs)
		if err != nil {
			t.Errorf("Failed to ingest %s log: %v", level, err)
		} else {
			t.Logf("Ingested %s log", level)
		}
	}
}

// TestTelemetryMetricTypes tests different metric types
func TestTelemetryMetricTypes(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Telemetry); err != nil {
		t.Skipf("Telemetry service not available: %v", err)
	}

	timestamp := time.Now().UnixNano()
	serviceName := fmt.Sprintf("metric-type-service-%d", timestamp)

	metricTypes := []struct {
		name  string
		mtype string
		value float64
	}{
		{"counter_metric", "counter", 100},
		{"gauge_metric", "gauge", 42.5},
		{"histogram_metric", "histogram", 0.25},
		{"summary_metric", "summary", 0.95},
	}

	for _, mt := range metricTypes {
		metrics := []MetricEntry{
			{
				Name:    mt.name,
				Type:    mt.mtype,
				Value:   mt.value,
				Service: serviceName,
			},
		}

		_, err := c.IngestMetrics(ctx, metrics)
		if err != nil {
			t.Errorf("Failed to ingest %s metric: %v", mt.mtype, err)
		} else {
			t.Logf("Ingested %s metric: %s = %.2f", mt.mtype, mt.name, mt.value)
		}
	}
}

// TestTelemetryBulkIngestion tests bulk data ingestion
func TestTelemetryBulkIngestion(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Telemetry); err != nil {
		t.Skipf("Telemetry service not available: %v", err)
	}

	timestamp := time.Now().UnixNano()
	serviceName := fmt.Sprintf("bulk-service-%d", timestamp)

	// Bulk logs
	logs := make([]LogEntry, 100)
	for i := 0; i < 100; i++ {
		logs[i] = LogEntry{
			Level:   "INFO",
			Service: serviceName,
			Message: fmt.Sprintf("Bulk log message %d", i),
		}
	}

	accepted, err := c.IngestLogs(ctx, logs)
	if err != nil {
		t.Fatalf("Failed to ingest bulk logs: %v", err)
	}
	t.Logf("Bulk ingested %d logs", accepted)

	// Bulk metrics
	metrics := make([]MetricEntry, 50)
	for i := 0; i < 50; i++ {
		metrics[i] = MetricEntry{
			Name:    fmt.Sprintf("bulk_metric_%d", i%10),
			Type:    "gauge",
			Value:   float64(i) * 1.5,
			Service: serviceName,
		}
	}

	accepted, err = c.IngestMetrics(ctx, metrics)
	if err != nil {
		t.Fatalf("Failed to ingest bulk metrics: %v", err)
	}
	t.Logf("Bulk ingested %d metrics", accepted)
}

// TestTelemetryDistributedTrace tests distributed tracing across services
func TestTelemetryDistributedTrace(t *testing.T) {
	c := getTestClient()
	ctx := context.Background()

	if err := c.HealthCheck(ctx, c.urls.Telemetry); err != nil {
		t.Skipf("Telemetry service not available: %v", err)
	}

	timestamp := time.Now().UnixNano()
	traceID := fmt.Sprintf("distributed-trace-%d", timestamp)

	services := []string{"gateway", "work-publisher", "bid-gateway", "contract-engine"}
	startTime := time.Now()

	spans := make([]TraceSpan, len(services))
	for i, svc := range services {
		parentSpanID := ""
		if i > 0 {
			parentSpanID = fmt.Sprintf("span-%d", i-1)
		}

		spanStart := startTime.Add(time.Duration(i*50) * time.Millisecond)
		spans[i] = TraceSpan{
			TraceID:      traceID,
			SpanID:       fmt.Sprintf("span-%d", i),
			ParentSpanID: parentSpanID,
			Service:      svc,
			Operation:    fmt.Sprintf("%s.ProcessRequest", svc),
			StartTime:    spanStart.Format(time.RFC3339Nano),
			EndTime:      spanStart.Add(40 * time.Millisecond).Format(time.RFC3339Nano),
			DurationMs:   40,
			Status:       "OK",
		}
	}

	accepted, err := c.IngestSpans(ctx, spans)
	if err != nil {
		t.Fatalf("Failed to ingest distributed trace: %v", err)
	}
	t.Logf("Ingested distributed trace with %d spans across %d services", accepted, len(services))

	// Retrieve and verify
	retrievedSpans, err := c.GetTrace(ctx, traceID)
	if err != nil {
		t.Fatalf("Failed to retrieve distributed trace: %v", err)
	}
	t.Logf("Retrieved %d spans for distributed trace", len(retrievedSpans))
}

