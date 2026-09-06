package middleware

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	status      int
	size        int
	wroteHeader bool
}

func (rw *responseWriter) WriteHeader(status int) {
	if rw.wroteHeader {
		return
	}
	rw.wroteHeader = true
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

// Logging writes one structured line per request through the default slog
// handler (JSON in main). The line carries the request id, the tenant, the
// scope the route required and whether the grant covered it, so the gateway
// log is a scope gate's record: who asked, for what, what was consulted, what
// was decided, and what came back.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx, ri := WithRequestInfo(r.Context())
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r.WithContext(ctx))

		attrs := []any{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", wrapped.status),
			slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000.0),
			slog.String("request_id", ri.RequestID),
			slog.String("tenant_id", ri.TenantID),
			slog.Int("size", wrapped.size),
		}
		if tool, ok := strings.CutPrefix(r.URL.Path, "/v1/tools/"); ok && tool != "" {
			attrs = append(attrs, slog.String("tool", strings.SplitN(tool, "/", 2)[0]))
		}
		if ri.Scope.Checked {
			decision := "deny"
			if ri.Scope.Allowed {
				decision = "allow"
			}
			attrs = append(attrs, slog.String("scope_required", ri.Scope.Required), slog.String("scope_decision", decision))
		}
		slog.Info("request", attrs...)
	})
}
