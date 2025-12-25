package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// RequestBuilder helps build HTTP requests with fluent API
type RequestBuilder struct {
	method  string
	baseURL string
	path    string
	query   url.Values
	headers map[string]string
	body    interface{}
	ctx     context.Context
}

// NewRequest creates a new request builder
func NewRequest(method, baseURL string) *RequestBuilder {
	return &RequestBuilder{
		method:  method,
		baseURL: baseURL,
		query:   make(url.Values),
		headers: make(map[string]string),
		ctx:     context.Background(),
	}
}

// Path sets the URL path
func (b *RequestBuilder) Path(path string) *RequestBuilder {
	b.path = path
	return b
}

// Query adds a query parameter
func (b *RequestBuilder) Query(key, value string) *RequestBuilder {
	b.query.Add(key, value)
	return b
}

// Header adds a header
func (b *RequestBuilder) Header(key, value string) *RequestBuilder {
	b.headers[key] = value
	return b
}

// JSON sets the request body as JSON
func (b *RequestBuilder) JSON(body interface{}) *RequestBuilder {
	b.body = body
	b.headers["Content-Type"] = "application/json"
	return b
}

// Context sets the context
func (b *RequestBuilder) Context(ctx context.Context) *RequestBuilder {
	b.ctx = ctx
	return b
}

// Build creates the HTTP request
func (b *RequestBuilder) Build() (*http.Request, error) {
	// Build URL
	u, err := url.Parse(b.baseURL + b.path)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}

	if len(b.query) > 0 {
		u.RawQuery = b.query.Encode()
	}

	// Build body
	var bodyReader io.Reader
	if b.body != nil {
		encoded, err := json.Marshal(b.body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(encoded)
	}

	// Create request
	req, err := http.NewRequestWithContext(b.ctx, b.method, u.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Add headers
	for k, v := range b.headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

// Execute builds and executes the request using the provided client
func (b *RequestBuilder) Execute(client *Client) (*http.Response, error) {
	req, err := b.Build()
	if err != nil {
		return nil, err
	}
	return client.Do(b.ctx, req)
}

// ExecuteJSON builds, executes, and decodes JSON response
func (b *RequestBuilder) ExecuteJSON(client *Client, result interface{}) error {
	resp, err := b.Execute(client)
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

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}
