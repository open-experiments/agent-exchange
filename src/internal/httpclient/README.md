# HTTP Client Utilities

Shared HTTP client package with retry logic, authentication, and request building helpers.

## Features

- **Automatic retries** with exponential backoff
- **Configurable timeouts** per client
- **Authentication support** (Bearer token, API key, Basic auth)
- **Fluent request builder** API
- **JSON encoding/decoding** helpers
- **Structured error types**

## Usage

### Basic Client

```go
import (
    "context"
    "time"
    "github.com/parlakisik/agent-exchange/internal/httpclient"
)

// Create a client with 10 second timeout
client := httpclient.NewClient("my-service", 10*time.Second)

// Simple GET request
resp, err := client.Get(ctx, "https://api.example.com/resource")

// GET with JSON response
var result MyStruct
err := client.GetJSON(ctx, "https://api.example.com/resource", &result)

// POST with JSON body and response
body := MyRequest{Field: "value"}
var response MyResponse
err := client.PostJSON(ctx, "https://api.example.com/resource", body, &response)
```

### Request Builder

```go
// Build complex requests with fluent API
var result MyStruct
err := httpclient.NewRequest("GET", "https://api.example.com").
    Path("/users").
    Query("limit", "10").
    Query("offset", "0").
    Header("X-Custom-Header", "value").
    Context(ctx).
    ExecuteJSON(client, &result)

// POST with JSON body
body := MyRequest{Field: "value"}
var response MyResponse
err := httpclient.NewRequest("POST", "https://api.example.com").
    Path("/users").
    JSON(body).
    Context(ctx).
    ExecuteJSON(client, &response)
```

### Authentication

```go
// Bearer token
auth := &httpclient.BearerTokenAuth{Token: "your-token"}
authClient := httpclient.NewClientWithAuth(client, auth)

// API key
auth := &httpclient.APIKeyAuth{
    Header: "X-API-Key",
    Key:    "your-api-key",
}
authClient := httpclient.NewClientWithAuth(client, auth)

// Basic auth
auth := &httpclient.BasicAuth{
    Username: "user",
    Password: "pass",
}
authClient := httpclient.NewClientWithAuth(client, auth)
```

### Custom Retry Configuration

```go
retryConfig := httpclient.RetryConfig{
    MaxRetries:     5,
    InitialBackoff: 200 * time.Millisecond,
    MaxBackoff:     10 * time.Second,
    RetryableStatuses: []int{
        http.StatusRequestTimeout,
        http.StatusTooManyRequests,
        http.StatusServiceUnavailable,
    },
}

client := httpclient.NewClientWithRetry("my-service", 10*time.Second, retryConfig)
```

## Error Handling

The client returns `*httpclient.HTTPError` for non-2xx responses:

```go
resp, err := client.Get(ctx, url)
if err != nil {
    if httpErr, ok := err.(*httpclient.HTTPError); ok {
        log.Printf("HTTP %d: %s", httpErr.StatusCode, string(httpErr.Body))
    }
    return err
}
```

## Retry Behavior

By default, the client retries on:
- Network errors
- HTTP 408 (Request Timeout)
- HTTP 429 (Too Many Requests)
- HTTP 502 (Bad Gateway)
- HTTP 503 (Service Unavailable)
- HTTP 504 (Gateway Timeout)

Retries use exponential backoff starting at 100ms, doubling each retry, up to a maximum of 5 seconds.

## Service Client Example

```go
package clients

import (
    "context"
    "time"
    "github.com/parlakisik/agent-exchange/internal/httpclient"
)

type MyServiceClient struct {
    baseURL string
    client  *httpclient.Client
}

func NewMyServiceClient(baseURL string) *MyServiceClient {
    return &MyServiceClient{
        baseURL: baseURL,
        client:  httpclient.NewClient("my-service", 10*time.Second),
    }
}

func (c *MyServiceClient) GetResource(ctx context.Context, id string) (*Resource, error) {
    var result Resource
    err := httpclient.NewRequest("GET", c.baseURL).
        Path("/resources/" + id).
        Context(ctx).
        ExecuteJSON(c.client, &result)
    return &result, err
}

func (c *MyServiceClient) CreateResource(ctx context.Context, req CreateRequest) (*Resource, error) {
    var result Resource
    err := httpclient.NewRequest("POST", c.baseURL).
        Path("/resources").
        JSON(req).
        Context(ctx).
        ExecuteJSON(c.client, &result)
    return &result, err
}
```
