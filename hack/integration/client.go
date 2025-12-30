package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ServiceURLs contains the URLs for all services
type ServiceURLs struct {
	Gateway          string
	WorkPublisher    string
	BidGateway       string
	BidEvaluator     string
	ContractEngine   string
	ProviderRegistry string
	TrustBroker      string
	Identity         string
	Settlement       string
	Telemetry        string
}

// DefaultLocalURLs returns URLs for local docker-compose setup
func DefaultLocalURLs() ServiceURLs {
	return ServiceURLs{
		Gateway:          "http://localhost:8080",
		WorkPublisher:    "http://localhost:8081",
		BidGateway:       "http://localhost:8082",
		BidEvaluator:     "http://localhost:8083",
		ContractEngine:   "http://localhost:8084",
		ProviderRegistry: "http://localhost:8085",
		TrustBroker:      "http://localhost:8086",
		Identity:         "http://localhost:8087",
		Settlement:       "http://localhost:8088",
		Telemetry:        "http://localhost:8089",
	}
}

// Client is the integration test client
type Client struct {
	urls   ServiceURLs
	http   *http.Client
	apiKey string
}

// NewClient creates a new integration test client
func NewClient(urls ServiceURLs) *Client {
	return &Client{
		urls: urls,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetAPIKey sets the API key for authenticated requests
func (c *Client) SetAPIKey(key string) {
	c.apiKey = key
}

// Request makes an HTTP request
func (c *Client) Request(ctx context.Context, method, url string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	return c.http.Do(req)
}

// JSON makes a request and decodes the JSON response
func (c *Client) JSON(ctx context.Context, method, url string, body, result any) error {
	resp, err := c.Request(ctx, method, url, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

// HealthCheck checks if a service is healthy
func (c *Client) HealthCheck(ctx context.Context, url string) error {
	resp, err := c.Request(ctx, http.MethodGet, url+"/health", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: %d", resp.StatusCode)
	}
	return nil
}

// WaitForServices waits for all services to be healthy
func (c *Client) WaitForServices(ctx context.Context, timeout time.Duration) error {
	services := map[string]string{
		"work-publisher":    c.urls.WorkPublisher,
		"bid-gateway":       c.urls.BidGateway,
		"bid-evaluator":     c.urls.BidEvaluator,
		"contract-engine":   c.urls.ContractEngine,
		"provider-registry": c.urls.ProviderRegistry,
		"trust-broker":      c.urls.TrustBroker,
		"identity":          c.urls.Identity,
		"settlement":        c.urls.Settlement,
	}

	deadline := time.Now().Add(timeout)
	for name, url := range services {
		for {
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for %s", name)
			}

			err := c.HealthCheck(ctx, url)
			if err == nil {
				break
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1 * time.Second):
			}
		}
	}
	return nil
}

// Work Publisher API

type WorkSpec struct {
	ID            string         `json:"work_id,omitempty"`
	Category      string         `json:"category"`
	Description   string         `json:"description"`
	Payload       map[string]any `json:"payload,omitempty"`
	Constraints   *Constraints   `json:"constraints,omitempty"`
	Budget        *Budget        `json:"budget,omitempty"`
	ConsumerID    string         `json:"consumer_id,omitempty"`
	BidWindowMs   int64          `json:"bid_window_ms,omitempty"`
	Status        string         `json:"status,omitempty"`
}

type Constraints struct {
	MaxLatencyMs *int64 `json:"max_latency_ms,omitempty"`
}

type Budget struct {
	MaxPrice    float64 `json:"max_price"`
	BidStrategy string  `json:"bid_strategy,omitempty"`
}

func (c *Client) SubmitWork(ctx context.Context, work *WorkSpec) (*WorkSpec, error) {
	var result WorkSpec
	err := c.JSON(ctx, http.MethodPost, c.urls.WorkPublisher+"/v1/work", work, &result)
	return &result, err
}

func (c *Client) GetWork(ctx context.Context, workID string) (*WorkSpec, error) {
	var result WorkSpec
	err := c.JSON(ctx, http.MethodGet, c.urls.WorkPublisher+"/v1/work/"+workID, nil, &result)
	return &result, err
}

// Provider Registry API

type Provider struct {
	ID           string            `json:"provider_id,omitempty"`
	Name         string            `json:"name"`
	Capabilities []string          `json:"capabilities"`
	Endpoint     string            `json:"endpoint"`
	Status       string            `json:"status,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	APIKey       string            `json:"api_key,omitempty"`
	APISecret    string            `json:"api_secret,omitempty"`
}

type Subscription struct {
	ID         string   `json:"subscription_id,omitempty"`
	ProviderID string   `json:"provider_id"`
	Categories []string `json:"categories"`
	Status     string   `json:"status,omitempty"`
}

func (c *Client) RegisterProvider(ctx context.Context, provider *Provider) (*Provider, error) {
	var result Provider
	err := c.JSON(ctx, http.MethodPost, c.urls.ProviderRegistry+"/v1/providers", provider, &result)
	return &result, err
}

func (c *Client) CreateSubscription(ctx context.Context, sub *Subscription) (*Subscription, error) {
	var result Subscription
	err := c.JSON(ctx, http.MethodPost, c.urls.ProviderRegistry+"/v1/subscriptions", sub, &result)
	return &result, err
}

// Bid Gateway API

type Bid struct {
	BidID            string            `json:"bid_id,omitempty"`
	WorkID           string            `json:"work_id"`
	ProviderID       string            `json:"provider_id,omitempty"`
	Price            float64           `json:"price"`
	PriceBreakdown   map[string]float64 `json:"price_breakdown,omitempty"`
	Confidence       float64           `json:"confidence,omitempty"`
	Approach         string            `json:"approach,omitempty"`
	EstimatedLatency int64             `json:"estimated_latency,omitempty"`
	MVPSample        string            `json:"mvp_sample,omitempty"`
	SLA              *SLA              `json:"sla,omitempty"`
	A2AEndpoint      string            `json:"a2a_endpoint"`
	ExpiresAt        string            `json:"expires_at"`
	ReceivedAt       string            `json:"received_at,omitempty"`
	Status           string            `json:"status,omitempty"`
}

type SLA struct {
	MaxLatencyMs int     `json:"max_latency_ms"`
	Availability float64 `json:"availability"`
}

type BidResponse struct {
	BidID      string `json:"bid_id"`
	WorkID     string `json:"work_id"`
	Status     string `json:"status"`
	ReceivedAt string `json:"received_at"`
}

// SubmitBidWithAuth submits a bid using provider API key authentication
func (c *Client) SubmitBidWithAuth(ctx context.Context, providerAPIKey string, bid *Bid) (*BidResponse, error) {
	var bodyReader io.Reader
	data, err := json.Marshal(bid)
	if err != nil {
		return nil, err
	}
	bodyReader = bytes.NewReader(data)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.urls.BidGateway+"/v1/bids", bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+providerAPIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result BidResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SubmitBid is deprecated - use SubmitBidWithAuth for proper authentication
func (c *Client) SubmitBid(ctx context.Context, bid *Bid) (*Bid, error) {
	var result Bid
	err := c.JSON(ctx, http.MethodPost, c.urls.BidGateway+"/v1/bids", bid, &result)
	return &result, err
}

// Bid Evaluator API

type EvaluationRequest struct {
	WorkID   string               `json:"work_id"`
	Strategy string               `json:"strategy,omitempty"`
	Budget   *EvaluationBudget    `json:"budget,omitempty"`
}

type EvaluationBudget struct {
	MaxPrice    float64 `json:"max_price"`
	BidStrategy string  `json:"bid_strategy,omitempty"`
}

type EvaluationResult struct {
	ID               string           `json:"evaluation_id"`
	WorkID           string           `json:"work_id"`
	TotalBids        int              `json:"total_bids"`
	ValidBids        int              `json:"valid_bids"`
	RankedBids       []RankedBid      `json:"ranked_bids"`
	DisqualifiedBids []DisqualifiedBid `json:"disqualified_bids,omitempty"`
	EvaluatedAt      string           `json:"evaluated_at"`
}

type RankedBid struct {
	Rank       int           `json:"rank"`
	BidID      string        `json:"bid_id"`
	ProviderID string        `json:"provider_id"`
	Score      float64       `json:"total_score"`
	Scores     BidScoreDetail `json:"scores"`
	Price      float64       `json:"-"` // Not directly from API, computed if needed
}

type BidScoreDetail struct {
	Price      float64 `json:"price"`
	Trust      float64 `json:"trust"`
	Confidence float64 `json:"confidence"`
	MVPSample  float64 `json:"mvp_sample"`
	SLA        float64 `json:"sla"`
}

type DisqualifiedBid struct {
	BidID  string `json:"bid_id"`
	Reason string `json:"reason"`
}

func (c *Client) EvaluateBids(ctx context.Context, req *EvaluationRequest) (*EvaluationResult, error) {
	var result EvaluationResult
	err := c.JSON(ctx, http.MethodPost, c.urls.BidEvaluator+"/internal/v1/evaluate", req, &result)
	return &result, err
}

// Contract Engine API

type AwardRequest struct {
	BidID     string `json:"bid_id,omitempty"`
	AutoAward bool   `json:"auto_award,omitempty"`
}

type AwardResponse struct {
	ContractID       string  `json:"contract_id"`
	WorkID           string  `json:"work_id"`
	ProviderID       string  `json:"provider_id"`
	AgreedPrice      float64 `json:"agreed_price"`
	Status           string  `json:"status"`
	ProviderEndpoint string  `json:"provider_endpoint"`
	ExecutionToken   string  `json:"execution_token"`
	ExpiresAt        string  `json:"expires_at"`
	AwardedAt        string  `json:"awarded_at"`
}

type Contract struct {
	ContractID       string         `json:"contract_id,omitempty"`
	WorkID           string         `json:"work_id"`
	ConsumerID       string         `json:"consumer_id"`
	ProviderID       string         `json:"provider_id"`
	BidID            string         `json:"bid_id"`
	AgreedPrice      float64        `json:"agreed_price"`
	ProviderEndpoint string         `json:"provider_endpoint"`
	ExecutionToken   string         `json:"execution_token"`
	ConsumerToken    string         `json:"consumer_token"`
	Status           string         `json:"status,omitempty"`
	ExpiresAt        string         `json:"expires_at"`
	AwardedAt        string         `json:"awarded_at"`
	StartedAt        string         `json:"started_at,omitempty"`
	CompletedAt      string         `json:"completed_at,omitempty"`
	FailedAt         string         `json:"failed_at,omitempty"`
	Outcome          map[string]any `json:"outcome,omitempty"`
	FailureReason    string         `json:"failure_reason,omitempty"`
}

type ProgressRequest struct {
	Status  string `json:"status"`
	Percent *int   `json:"percent,omitempty"`
	Message string `json:"message,omitempty"`
}

type CompleteRequest struct {
	Success        bool           `json:"success"`
	ResultSummary  string         `json:"result_summary"`
	Metrics        map[string]any `json:"metrics,omitempty"`
	ResultLocation string         `json:"result_location,omitempty"`
}

type FailRequest struct {
	Reason     string `json:"reason"`
	Message    string `json:"message"`
	ReportedBy string `json:"reported_by"` // "provider" or "consumer"
}

func (c *Client) AwardContract(ctx context.Context, workID string, req *AwardRequest) (*AwardResponse, error) {
	var result AwardResponse
	err := c.JSON(ctx, http.MethodPost, c.urls.ContractEngine+"/v1/work/"+workID+"/award", req, &result)
	return &result, err
}

// UpdateProgressWithToken updates contract progress using execution token
func (c *Client) UpdateProgressWithToken(ctx context.Context, contractID, executionToken string, req *ProgressRequest) error {
	var bodyReader io.Reader
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	bodyReader = bytes.NewReader(data)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.urls.ContractEngine+"/v1/contracts/"+contractID+"/progress", bodyReader)
	if err != nil {
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+executionToken)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

// CompleteContractWithToken completes a contract using execution token
func (c *Client) CompleteContractWithToken(ctx context.Context, contractID, executionToken string, req *CompleteRequest) error {
	var bodyReader io.Reader
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	bodyReader = bytes.NewReader(data)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.urls.ContractEngine+"/v1/contracts/"+contractID+"/complete", bodyReader)
	if err != nil {
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+executionToken)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

// FailContractWithToken marks a contract as failed
func (c *Client) FailContractWithToken(ctx context.Context, contractID, token string, req *FailRequest) error {
	var bodyReader io.Reader
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	bodyReader = bytes.NewReader(data)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.urls.ContractEngine+"/v1/contracts/"+contractID+"/fail", bodyReader)
	if err != nil {
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

func (c *Client) GetContract(ctx context.Context, contractID string) (*Contract, error) {
	var result Contract
	err := c.JSON(ctx, http.MethodGet, c.urls.ContractEngine+"/v1/contracts/"+contractID, nil, &result)
	return &result, err
}

// Deprecated methods kept for compatibility
func (c *Client) UpdateProgress(ctx context.Context, req *ProgressRequest) error {
	return fmt.Errorf("use UpdateProgressWithToken instead")
}

func (c *Client) CompleteContract(ctx context.Context, req *CompleteRequest) (*Contract, error) {
	return nil, fmt.Errorf("use CompleteContractWithToken instead")
}

// Settlement API

type SettlementRequest struct {
	ContractID string  `json:"contract_id"`
	ConsumerID string  `json:"consumer_id"`
	ProviderID string  `json:"provider_id"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
}

type DepositRequest struct {
	TenantID string `json:"tenant_id"`
	Amount   string `json:"amount"`
}

type Balance struct {
	TenantID string  `json:"tenant_id"`
	Balance  float64 `json:"balance,string"`
	Currency string  `json:"currency"`
}

type Transaction struct {
	ID        string  `json:"id"`
	TenantID  string  `json:"tenant_id"`
	Type      string  `json:"type"`
	Amount    float64 `json:"amount,string"`
	Balance   float64 `json:"balance,string"`
	Reference string  `json:"reference,omitempty"`
}

func (c *Client) Deposit(ctx context.Context, req *DepositRequest) error {
	return c.JSON(ctx, http.MethodPost, c.urls.Settlement+"/v1/deposits", req, nil)
}

func (c *Client) GetBalance(ctx context.Context, tenantID string) (*Balance, error) {
	var result Balance
	err := c.JSON(ctx, http.MethodGet, c.urls.Settlement+"/v1/balance?tenant_id="+tenantID, nil, &result)
	return &result, err
}

func (c *Client) SettleContract(ctx context.Context, req *SettlementRequest) error {
	return c.JSON(ctx, http.MethodPost, c.urls.Settlement+"/internal/v1/settle", req, nil)
}

func (c *Client) GetTransactions(ctx context.Context, tenantID string) ([]Transaction, error) {
	var result struct {
		Transactions []Transaction `json:"transactions"`
	}
	err := c.JSON(ctx, http.MethodGet, c.urls.Settlement+"/v1/usage/transactions?tenant_id="+tenantID, nil, &result)
	return result.Transactions, err
}

// Trust Broker API

type TrustScore struct {
	ProviderID string  `json:"provider_id"`
	Score      float64 `json:"score"`
	Tier       string  `json:"tier"`
}

func (c *Client) GetTrustScore(ctx context.Context, providerID string) (*TrustScore, error) {
	var result TrustScore
	err := c.JSON(ctx, http.MethodGet, c.urls.TrustBroker+"/v1/providers/"+providerID+"/trust", nil, &result)
	return &result, err
}

// Identity API

type Tenant struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Status string `json:"status,omitempty"`
}

type APIKey struct {
	ID       string   `json:"id,omitempty"`
	Key      string   `json:"key,omitempty"`
	TenantID string   `json:"tenant_id"`
	Name     string   `json:"name"`
	Scopes   []string `json:"scopes"`
	Status   string   `json:"status,omitempty"`
}

func (c *Client) CreateTenant(ctx context.Context, tenant *Tenant) (*Tenant, error) {
	var result Tenant
	err := c.JSON(ctx, http.MethodPost, c.urls.Identity+"/v1/tenants", tenant, &result)
	return &result, err
}

func (c *Client) CreateAPIKey(ctx context.Context, key *APIKey) (*APIKey, error) {
	var result APIKey
	err := c.JSON(ctx, http.MethodPost, c.urls.Identity+"/v1/tenants/"+key.TenantID+"/api-keys", key, &result)
	return &result, err
}

func (c *Client) GetTenant(ctx context.Context, tenantID string) (*Tenant, error) {
	var result Tenant
	err := c.JSON(ctx, http.MethodGet, c.urls.Identity+"/v1/tenants/"+tenantID, nil, &result)
	return &result, err
}

func (c *Client) ListAPIKeys(ctx context.Context, tenantID string) ([]APIKey, error) {
	var result []APIKey
	err := c.JSON(ctx, http.MethodGet, c.urls.Identity+"/v1/tenants/"+tenantID+"/api-keys", nil, &result)
	return result, err
}

// Provider Registry extended API

func (c *Client) GetProvider(ctx context.Context, providerID string) (*Provider, error) {
	var result Provider
	err := c.JSON(ctx, http.MethodGet, c.urls.ProviderRegistry+"/v1/providers/"+providerID, nil, &result)
	return &result, err
}

func (c *Client) ListSubscriptions(ctx context.Context, providerID string) ([]Subscription, error) {
	var result struct {
		Subscriptions []Subscription `json:"subscriptions"`
	}
	err := c.JSON(ctx, http.MethodGet, c.urls.ProviderRegistry+"/v1/subscriptions?provider_id="+providerID, nil, &result)
	return result.Subscriptions, err
}

// Work Publisher extended API

func (c *Client) CancelWork(ctx context.Context, workID string) (*WorkSpec, error) {
	var result WorkSpec
	err := c.JSON(ctx, http.MethodPost, c.urls.WorkPublisher+"/v1/work/"+workID+"/cancel", nil, &result)
	return &result, err
}

// Settlement extended API

func (c *Client) GetUsage(ctx context.Context, tenantID string) (map[string]any, error) {
	var result map[string]any
	err := c.JSON(ctx, http.MethodGet, c.urls.Settlement+"/v1/usage?tenant_id="+tenantID, nil, &result)
	return result, err
}

// Telemetry API

type LogEntry struct {
	ID        string         `json:"id,omitempty"`
	Timestamp string         `json:"timestamp,omitempty"`
	Level     string         `json:"level"`
	Service   string         `json:"service"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields,omitempty"`
	TraceID   string         `json:"trace_id,omitempty"`
}

type MetricEntry struct {
	ID        string            `json:"id,omitempty"`
	Timestamp string            `json:"timestamp,omitempty"`
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Value     float64           `json:"value"`
	Service   string            `json:"service"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type TraceSpan struct {
	TraceID      string            `json:"trace_id"`
	SpanID       string            `json:"span_id"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	Service      string            `json:"service"`
	Operation    string            `json:"operation"`
	StartTime    string            `json:"start_time"`
	EndTime      string            `json:"end_time"`
	DurationMs   int64             `json:"duration_ms"`
	Status       string            `json:"status"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

func (c *Client) IngestLogs(ctx context.Context, logs []LogEntry) (int, error) {
	var result struct {
		Accepted int `json:"accepted"`
	}
	err := c.JSON(ctx, http.MethodPost, c.urls.Telemetry+"/v1/logs", logs, &result)
	return result.Accepted, err
}

func (c *Client) QueryLogs(ctx context.Context, service, level string, limit int) ([]LogEntry, error) {
	url := fmt.Sprintf("%s/v1/logs?limit=%d", c.urls.Telemetry, limit)
	if service != "" {
		url += "&service=" + service
	}
	if level != "" {
		url += "&level=" + level
	}
	var result struct {
		Logs  []LogEntry `json:"logs"`
		Count int        `json:"count"`
	}
	err := c.JSON(ctx, http.MethodGet, url, nil, &result)
	return result.Logs, err
}

func (c *Client) IngestMetrics(ctx context.Context, metrics []MetricEntry) (int, error) {
	var result struct {
		Accepted int `json:"accepted"`
	}
	err := c.JSON(ctx, http.MethodPost, c.urls.Telemetry+"/v1/metrics", metrics, &result)
	return result.Accepted, err
}

func (c *Client) QueryMetrics(ctx context.Context, name, service string, limit int) ([]MetricEntry, error) {
	url := fmt.Sprintf("%s/v1/metrics?limit=%d", c.urls.Telemetry, limit)
	if name != "" {
		url += "&name=" + name
	}
	if service != "" {
		url += "&service=" + service
	}
	var result struct {
		Metrics []MetricEntry `json:"metrics"`
		Count   int           `json:"count"`
	}
	err := c.JSON(ctx, http.MethodGet, url, nil, &result)
	return result.Metrics, err
}

func (c *Client) IngestSpans(ctx context.Context, spans []TraceSpan) (int, error) {
	var result struct {
		Accepted int `json:"accepted"`
	}
	err := c.JSON(ctx, http.MethodPost, c.urls.Telemetry+"/v1/spans", spans, &result)
	return result.Accepted, err
}

func (c *Client) GetTrace(ctx context.Context, traceID string) ([]TraceSpan, error) {
	var result struct {
		TraceID string      `json:"trace_id"`
		Spans   []TraceSpan `json:"spans"`
		Count   int         `json:"count"`
	}
	err := c.JSON(ctx, http.MethodGet, c.urls.Telemetry+"/v1/traces/"+traceID, nil, &result)
	return result.Spans, err
}

func (c *Client) GetTelemetryStats(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := c.JSON(ctx, http.MethodGet, c.urls.Telemetry+"/v1/stats", nil, &result)
	return result, err
}

// Trust Broker extended API

type OutcomeRequest struct {
	ProviderID string `json:"provider_id"`
	ContractID string `json:"contract_id"`
	Outcome    string `json:"outcome"`
}

func (c *Client) RecordOutcome(ctx context.Context, req *OutcomeRequest) error {
	return c.JSON(ctx, http.MethodPost, c.urls.TrustBroker+"/internal/v1/outcomes", req, nil)
}

// Gateway API

func (c *Client) GetGatewayInfo(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := c.JSON(ctx, http.MethodGet, c.urls.Gateway+"/v1/info", nil, &result)
	return result, err
}

// RequestWithStatus makes a request and returns the status code
func (c *Client) RequestWithStatus(ctx context.Context, method, url string, body any) (int, []byte, error) {
	resp, err := c.Request(ctx, method, url, body)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	
	bodyBytes, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, bodyBytes, nil
}

