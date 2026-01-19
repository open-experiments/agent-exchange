package store

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/parlakisik/agent-exchange/aex-telemetry/internal/model"
)

type MemoryStore struct {
	mu             sync.RWMutex
	logs           []model.LogEntry
	metrics        []model.MetricEntry
	spans          []model.TraceSpan
	maxLogEntries  int
	maxMetricItems int
}

func NewMemoryStore(maxLogEntries, maxMetricItems int) *MemoryStore {
	return &MemoryStore{
		logs:           make([]model.LogEntry, 0),
		metrics:        make([]model.MetricEntry, 0),
		spans:          make([]model.TraceSpan, 0),
		maxLogEntries:  maxLogEntries,
		maxMetricItems: maxMetricItems,
	}
}

func (s *MemoryStore) AddLog(entry model.LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry.ID == "" {
		entry.ID = generateID()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// Evict oldest if at capacity
	if len(s.logs) >= s.maxLogEntries {
		s.logs = s.logs[1:]
	}

	s.logs = append(s.logs, entry)
	return nil
}

func (s *MemoryStore) QueryLogs(query model.LogQuery) ([]model.LogEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []model.LogEntry
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}

	// Iterate in reverse (newest first)
	for i := len(s.logs) - 1; i >= 0 && len(results) < limit; i-- {
		entry := s.logs[i]

		// Apply filters
		if query.Service != "" && entry.Service != query.Service {
			continue
		}
		if query.Level != "" && entry.Level != query.Level {
			continue
		}
		if !query.StartTime.IsZero() && entry.Timestamp.Before(query.StartTime) {
			continue
		}
		if !query.EndTime.IsZero() && entry.Timestamp.After(query.EndTime) {
			continue
		}
		if query.Search != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(query.Search)) {
			continue
		}

		results = append(results, entry)
	}

	return results, nil
}

func (s *MemoryStore) AddMetric(entry model.MetricEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry.ID == "" {
		entry.ID = generateID()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// Evict oldest if at capacity
	if len(s.metrics) >= s.maxMetricItems {
		s.metrics = s.metrics[1:]
	}

	s.metrics = append(s.metrics, entry)
	return nil
}

func (s *MemoryStore) QueryMetrics(query model.MetricQuery) ([]model.MetricEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []model.MetricEntry
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}

	for i := len(s.metrics) - 1; i >= 0 && len(results) < limit; i-- {
		entry := s.metrics[i]

		if query.Name != "" && entry.Name != query.Name {
			continue
		}
		if query.Service != "" && entry.Service != query.Service {
			continue
		}
		if !query.StartTime.IsZero() && entry.Timestamp.Before(query.StartTime) {
			continue
		}
		if !query.EndTime.IsZero() && entry.Timestamp.After(query.EndTime) {
			continue
		}

		results = append(results, entry)
	}

	return results, nil
}

func (s *MemoryStore) AddSpan(span model.TraceSpan) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.spans) >= s.maxMetricItems {
		s.spans = s.spans[1:]
	}

	s.spans = append(s.spans, span)
	return nil
}

func (s *MemoryStore) GetTraceSpans(traceID string) ([]model.TraceSpan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []model.TraceSpan
	for _, span := range s.spans {
		if span.TraceID == traceID {
			results = append(results, span)
		}
	}
	return results, nil
}

func (s *MemoryStore) GetStats() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]any{
		"log_count":    len(s.logs),
		"metric_count": len(s.metrics),
		"span_count":   len(s.spans),
		"max_logs":     s.maxLogEntries,
		"max_metrics":  s.maxMetricItems,
	}
}

func generateID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
