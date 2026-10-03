package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/sony/gobreaker"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// ErrCircuitOpen is returned when the circuit breaker is open and requests
// are being rejected to protect the downstream service.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// serverError signals a 5xx response to the circuit breaker without hiding
// the response from the caller.
type serverError struct {
	statusCode int
}

func (e *serverError) Error() string {
	return fmt.Sprintf("server error: %d", e.statusCode)
}

// CircuitBreakerConfig defines circuit breaker behavior
type CircuitBreakerConfig struct {
	// MaxConsecutiveFailures is the number of consecutive failures before the
	// circuit breaker trips to the open state. Default: 5.
	MaxConsecutiveFailures uint32
	// OpenDuration is how long the breaker stays open before transitioning to
	// half-open. Default: 10s.
	OpenDuration time.Duration
}

// DefaultCircuitBreakerConfig returns sensible defaults for the circuit breaker
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxConsecutiveFailures: 5,
		OpenDuration:           10 * time.Second,
	}
}

// Client is a wrapper around http.Client with retry logic, circuit breaker, and better error handling
type Client struct {
	httpClient  *http.Client
	retryConfig RetryConfig
	serviceName string
	breaker     *gobreaker.CircuitBreaker
}

// RetryConfig defines retry behavior
type RetryConfig struct {
	MaxRetries        int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	RetryableStatuses []int
}

// DefaultRetryConfig returns sensible defaults for retries
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		RetryableStatuses: []int{
			http.StatusRequestTimeout,
			http.StatusTooManyRequests,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout,
		},
	}
}

// newBreaker creates a gobreaker.CircuitBreaker for the given service name and config.
func newBreaker(serviceName string, cfg CircuitBreakerConfig) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:    serviceName,
		Timeout: cfg.OpenDuration,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cfg.MaxConsecutiveFailures
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			slog.Warn("circuit breaker state change",
				"service", name,
				"from", from.String(),
				"to", to.String(),
			)
		},
	})
}

// NewClient creates a new HTTP client with default settings
func NewClient(serviceName string, timeout time.Duration) *Client {
	cbCfg := DefaultCircuitBreakerConfig()
	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		retryConfig: DefaultRetryConfig(),
		serviceName: serviceName,
		breaker:     newBreaker(serviceName, cbCfg),
	}
}

// Do executes an HTTP request with circuit breaker and retry logic.
// If the circuit breaker is open, it returns ErrCircuitOpen immediately
// without attempting the request.
//
// A child span is created for each outgoing call, and the current trace
// context is injected into the request headers so downstream services can
// continue the trace.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	tracer := otel.Tracer("httpclient")
	spanName := fmt.Sprintf("HTTP %s %s", req.Method, c.serviceName)
	ctx, span := tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.HTTPMethodKey.String(req.Method),
			semconv.HTTPTargetKey.String(req.URL.String()),
			attribute.String("peer.service", c.serviceName),
		),
	)
	defer span.End()

	// Inject trace context into outgoing request headers
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	result, err := c.breaker.Execute(func() (interface{}, error) {
		resp, err := c.doWithRetry(ctx, req)
		if err != nil {
			return nil, err
		}
		// Only count 5xx as circuit breaker failures.
		// 4xx errors are client mistakes and must NOT trip the breaker.
		if resp.StatusCode >= 500 {
			return resp, &serverError{statusCode: resp.StatusCode}
		}
		return resp, nil
	})
	if err != nil {
		// Map gobreaker's sentinel error to our own ErrCircuitOpen so callers
		// don't need to depend on the gobreaker package.
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			span.RecordError(err)
			return nil, fmt.Errorf("%s: %w", c.serviceName, ErrCircuitOpen)
		}
		// For 5xx server errors the response is still returned alongside the
		// error so the breaker counts the failure. Extract the response.
		var se *serverError
		if errors.As(err, &se) && result != nil {
			resp := result.(*http.Response)
			span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
			return resp, nil
		}
		span.RecordError(err)
		return nil, err
	}

	resp := result.(*http.Response)
	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	return resp, nil
}

// doWithRetry executes an HTTP request with retry logic (called inside the circuit breaker).
//
// Each attempt sends a clone of req. A request body is consumed by the first
// attempt, so retries obtain a fresh copy through req.GetBody. A request that
// has a body but no GetBody cannot be replayed and is sent exactly once: its
// first error or response is returned as-is instead of retrying with an empty
// body.
func (c *Client) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	var lastErr error
	backoff := c.retryConfig.InitialBackoff
	hasBody := req.Body != nil && req.Body != http.NoBody
	replayable := !hasBody || req.GetBody != nil

	for attempt := 0; attempt <= c.retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			slog.DebugContext(ctx, "retrying request",
				"service", c.serviceName,
				"attempt", attempt,
				"method", req.Method,
				"url", req.URL.String(),
				"backoff", backoff,
			)

			select {
			case <-time.After(backoff):
				// Continue with retry
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			// Exponential backoff
			backoff *= 2
			if backoff > c.retryConfig.MaxBackoff {
				backoff = c.retryConfig.MaxBackoff
			}
		}

		attemptReq := req.Clone(ctx)
		if attempt > 0 && hasBody {
			body, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("rewind request body: %w", err)
			}
			attemptReq.Body = body
		}

		resp, err := c.httpClient.Do(attemptReq)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			if !replayable {
				return nil, lastErr
			}
			continue
		}

		// Check if status is retryable
		if c.isRetryableStatus(resp.StatusCode) {
			if !replayable {
				return resp, nil
			}
			drainAndClose(resp.Body)
			lastErr = fmt.Errorf("retryable status code: %d", resp.StatusCode)
			continue
		}

		// Success or non-retryable error
		return resp, nil
	}

	return nil, fmt.Errorf("max retries exceeded for %s: %w", req.URL.String(), lastErr)
}

// maxDrainBytes bounds how much of a discarded response body is read so the
// underlying connection can be reused without stalling on a large payload.
const maxDrainBytes = 64 << 10

// drainAndClose discards the rest of a response body and closes it so the
// connection returns to the pool before the next attempt.
func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxDrainBytes))
	_ = body.Close()
}

// Get performs a GET request
func (c *Client) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	return c.Do(ctx, req)
}

// Post performs a POST request with JSON body
func (c *Client) Post(ctx context.Context, url string, body interface{}) (*http.Response, error) {
	return c.doJSON(ctx, http.MethodPost, url, body)
}

// Put performs a PUT request with JSON body
func (c *Client) Put(ctx context.Context, url string, body interface{}) (*http.Response, error) {
	return c.doJSON(ctx, http.MethodPut, url, body)
}

// Delete performs a DELETE request
func (c *Client) Delete(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	return c.Do(ctx, req)
}

// GetJSON performs a GET request and decodes JSON response
func (c *Client) GetJSON(ctx context.Context, url string, result interface{}) error {
	resp, err := c.Get(ctx, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       body,
		}
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

// PostJSON performs a POST request with JSON body and decodes JSON response
func (c *Client) PostJSON(ctx context.Context, url string, body interface{}, result interface{}) error {
	resp, err := c.Post(ctx, url, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       bodyBytes,
		}
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

// doJSON performs a request with JSON body
func (c *Client) doJSON(ctx context.Context, method, url string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode body: %w", err)
		}
		// A bytes.Reader lets http.NewRequest set GetBody so retries can
		// replay the body.
		bodyReader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.Do(ctx, req)
}

func (c *Client) isRetryableStatus(statusCode int) bool {
	for _, s := range c.retryConfig.RetryableStatuses {
		if s == statusCode {
			return true
		}
	}
	return false
}

// HTTPError represents an HTTP error response
type HTTPError struct {
	StatusCode int
	Status     string
	Body       []byte
}

func (e *HTTPError) Error() string {
	if len(e.Body) > 0 {
		return fmt.Sprintf("HTTP %d: %s", e.StatusCode, string(e.Body))
	}
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Status)
}
