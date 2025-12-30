package service

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/parlakisik/agent-exchange/aex-telemetry/internal/model"
	"github.com/parlakisik/agent-exchange/aex-telemetry/internal/store"
)

type Service struct {
	store *store.MemoryStore
}

func New(s *store.MemoryStore) *Service {
	return &Service{store: s}
}

// HandleIngestLogs handles POST /v1/logs
func (svc *Service) HandleIngestLogs(w http.ResponseWriter, r *http.Request) {
	var entries []model.LogEntry
	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		// Try single entry
		var entry model.LogEntry
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		entries = []model.LogEntry{entry}
	}

	for _, entry := range entries {
		if err := svc.store.AddLog(entry); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to store log")
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"accepted": len(entries),
	})
}

// HandleQueryLogs handles GET /v1/logs
func (svc *Service) HandleQueryLogs(w http.ResponseWriter, r *http.Request) {
	query := model.LogQuery{
		Service: r.URL.Query().Get("service"),
		Level:   r.URL.Query().Get("level"),
		Search:  r.URL.Query().Get("search"),
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			query.Limit = limit
		}
	}

	if startStr := r.URL.Query().Get("start_time"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			query.StartTime = t
		}
	}

	if endStr := r.URL.Query().Get("end_time"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			query.EndTime = t
		}
	}

	logs, err := svc.store.QueryLogs(query)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"logs":  logs,
		"count": len(logs),
	})
}

// HandleIngestMetrics handles POST /v1/metrics
func (svc *Service) HandleIngestMetrics(w http.ResponseWriter, r *http.Request) {
	var entries []model.MetricEntry
	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	for _, entry := range entries {
		if err := svc.store.AddMetric(entry); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to store metric")
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"accepted": len(entries),
	})
}

// HandleQueryMetrics handles GET /v1/metrics
func (svc *Service) HandleQueryMetrics(w http.ResponseWriter, r *http.Request) {
	query := model.MetricQuery{
		Name:    r.URL.Query().Get("name"),
		Service: r.URL.Query().Get("service"),
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			query.Limit = limit
		}
	}

	if startStr := r.URL.Query().Get("start_time"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			query.StartTime = t
		}
	}

	if endStr := r.URL.Query().Get("end_time"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			query.EndTime = t
		}
	}

	metrics, err := svc.store.QueryMetrics(query)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"metrics": metrics,
		"count":   len(metrics),
	})
}

// HandleIngestSpans handles POST /v1/spans
func (svc *Service) HandleIngestSpans(w http.ResponseWriter, r *http.Request) {
	var spans []model.TraceSpan
	if err := json.NewDecoder(r.Body).Decode(&spans); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	for _, span := range spans {
		if err := svc.store.AddSpan(span); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to store span")
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"accepted": len(spans),
	})
}

// HandleGetTrace handles GET /v1/traces/{trace_id}
func (svc *Service) HandleGetTrace(w http.ResponseWriter, r *http.Request) {
	// Extract trace_id from path
	traceID := r.PathValue("trace_id")
	if traceID == "" {
		respondError(w, http.StatusBadRequest, "trace_id required")
		return
	}

	spans, err := svc.store.GetTraceSpans(traceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"trace_id": traceID,
		"spans":    spans,
		"count":    len(spans),
	})
}

// HandleGetStats handles GET /v1/stats
func (svc *Service) HandleGetStats(w http.ResponseWriter, r *http.Request) {
	stats := svc.store.GetStats()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}

func respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    status,
			"message": message,
		},
	})
}

