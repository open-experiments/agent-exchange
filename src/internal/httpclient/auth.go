package httpclient

import (
	"context"
	"net/http"
)

// AuthProvider adds authentication to requests
type AuthProvider interface {
	Apply(req *http.Request) error
}

// BearerTokenAuth adds Bearer token authentication
type BearerTokenAuth struct {
	Token string
}

func (a *BearerTokenAuth) Apply(req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+a.Token)
	return nil
}

// APIKeyAuth adds API key authentication
type APIKeyAuth struct {
	Header string
	Key    string
}

func (a *APIKeyAuth) Apply(req *http.Request) error {
	req.Header.Set(a.Header, a.Key)
	return nil
}

// BasicAuth adds HTTP Basic authentication
type BasicAuth struct {
	Username string
	Password string
}

func (a *BasicAuth) Apply(req *http.Request) error {
	req.SetBasicAuth(a.Username, a.Password)
	return nil
}

// ClientWithAuth wraps a client with authentication
type ClientWithAuth struct {
	*Client
	auth AuthProvider
}

// NewClientWithAuth creates a client that applies authentication to all requests
func NewClientWithAuth(client *Client, auth AuthProvider) *ClientWithAuth {
	return &ClientWithAuth{
		Client: client,
		auth:   auth,
	}
}

// Do executes a request with authentication
func (c *ClientWithAuth) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if err := c.auth.Apply(req); err != nil {
		return nil, err
	}
	return c.Client.Do(ctx, req)
}
