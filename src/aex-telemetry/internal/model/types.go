package model

import "time"

// LogEntry represents a log entry from a service
type LogEntry struct {
	ID        string            `json:"id"`
	Timestamp time.Time         `json:"timestamp"`
	Level     string            `json:"level"`
	Service   string            `json:"service"`
	Message   string            `json:"message"`
	Fields    map[string]any    `json:"fields,omitempty"`
	TraceID   string            `json:"trace_id,omitempty"`
	SpanID    string            `json:"span_id,omitempty"`
}

// MetricEntry represents a metric data point
type MetricEntry struct {
	ID        string            `json:"id"`
	Timestamp time.Time         `json:"timestamp"`
	Name      string            `json:"name"`
	Type      MetricType        `json:"type"`
	Value     float64           `json:"value"`
	Service   string            `json:"service"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
)

// TraceSpan represents a span in a distributed trace
type TraceSpan struct {
	TraceID      string            `json:"trace_id"`
	SpanID       string            `json:"span_id"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	Service      string            `json:"service"`
	Operation    string            `json:"operation"`
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time"`
	DurationMs   int64             `json:"duration_ms"`
	Status       string            `json:"status"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

// LogQuery represents parameters for querying logs
type LogQuery struct {
	Service   string    `json:"service,omitempty"`
	Level     string    `json:"level,omitempty"`
	StartTime time.Time `json:"start_time,omitempty"`
	EndTime   time.Time `json:"end_time,omitempty"`
	Search    string    `json:"search,omitempty"`
	Limit     int       `json:"limit,omitempty"`
}

// MetricQuery represents parameters for querying metrics
type MetricQuery struct {
	Name      string    `json:"name,omitempty"`
	Service   string    `json:"service,omitempty"`
	StartTime time.Time `json:"start_time,omitempty"`
	EndTime   time.Time `json:"end_time,omitempty"`
	Limit     int       `json:"limit,omitempty"`
}

