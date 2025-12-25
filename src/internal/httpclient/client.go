package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Client is a wrapper around http.Client with retry logic and better error handling
type Client struct {
	httpClient  *http.Client
	retryConfig RetryConfig
	serviceName string
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

// NewClient creates a new HTTP client with default settings
func NewClient(serviceName string, timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		retryConfig: DefaultRetryConfig(),
		serviceName: serviceName,
	}
}

// NewClientWithRetry creates a new HTTP client with custom retry config
func NewClientWithRetry(serviceName string, timeout time.Duration, retryConfig RetryConfig) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		retryConfig: retryConfig,
		serviceName: serviceName,
	}
}

// Do executes an HTTP request with retry logic
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	var lastErr error
	backoff := c.retryConfig.InitialBackoff

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

		resp, err := c.httpClient.Do(req.WithContext(ctx))
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		// Check if status is retryable
		if c.isRetryableStatus(resp.StatusCode) {
			resp.Body.Close()
			lastErr = fmt.Errorf("retryable status code: %d", resp.StatusCode)
			continue
		}

		// Success or non-retryable error
		return resp, nil
	}

	return nil, fmt.Errorf("max retries exceeded for %s: %w", req.URL.String(), lastErr)
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
		pr, pw := io.Pipe()
		go func() {
			defer pw.Close()
			pw.Write(encoded)
		}()
		bodyReader = pr
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
