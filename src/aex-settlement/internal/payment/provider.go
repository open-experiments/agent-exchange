package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/model"
)

// ProviderClient handles communication with payment provider agents
type ProviderClient struct {
	httpClient *http.Client
	providers  []PaymentProviderConfig
}

// PaymentProviderConfig represents a configured payment provider
type PaymentProviderConfig struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Endpoint    string `json:"endpoint"`
	Description string `json:"description"`
}

// A2ARequest represents an A2A JSON-RPC request
type A2ARequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	ID      string      `json:"id"`
	Params  interface{} `json:"params"`
}

// A2AResponse represents an A2A JSON-RPC response
type A2AResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *A2AError       `json:"error,omitempty"`
}

// A2AError represents an A2A error
type A2AError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// BidRequest represents the bid request sent to payment providers
type BidRequest struct {
	Action       string  `json:"action"`
	Amount       float64 `json:"amount"`
	WorkCategory string  `json:"work_category"`
	Currency     string  `json:"currency"`
}

// BidResponse represents a bid response from a payment provider
type BidResponse struct {
	Action string `json:"action"`
	Bid    struct {
		ProviderID            string   `json:"provider_id"`
		ProviderName          string   `json:"provider_name"`
		BaseFeePercent        float64  `json:"base_fee_percent"`
		RewardPercent         float64  `json:"reward_percent"`
		NetFeePercent         float64  `json:"net_fee_percent"`
		ProcessingTimeSeconds int      `json:"processing_time_seconds"`
		SupportedMethods      []string `json:"supported_methods"`
		FraudProtection       string   `json:"fraud_protection"`
	} `json:"bid"`
}

// NewProviderClient creates a new payment provider client
func NewProviderClient() *ProviderClient {
	// Load payment providers from environment or use defaults
	providers := loadPaymentProviders()

	return &ProviderClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		providers: providers,
	}
}

// loadPaymentProviders loads payment provider configs
func loadPaymentProviders() []PaymentProviderConfig {
	// Check for environment variable with provider URLs
	providerURLs := os.Getenv("PAYMENT_PROVIDER_URLS")
	if providerURLs != "" {
		var providers []PaymentProviderConfig
		if err := json.Unmarshal([]byte(providerURLs), &providers); err == nil {
			return providers
		}
	}

	// Default demo payment providers
	return []PaymentProviderConfig{
		{
			ID:          "legalpay",
			Name:        "LegalPay",
			Endpoint:    getEnvOrDefault("LEGALPAY_URL", "http://payment-legalpay:8200"),
			Description: "General legal payment processor",
		},
		{
			ID:          "contractpay",
			Name:        "ContractPay",
			Endpoint:    getEnvOrDefault("CONTRACTPAY_URL", "http://payment-contractpay:8201"),
			Description: "Contract specialist with cashback",
		},
		{
			ID:          "compliancepay",
			Name:        "CompliancePay",
			Endpoint:    getEnvOrDefault("COMPLIANCEPAY_URL", "http://payment-compliancepay:8202"),
			Description: "Compliance specialist with advanced security",
		},
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// GetPaymentBids requests bids from all payment providers
func (c *ProviderClient) GetPaymentBids(ctx context.Context, req model.PaymentBidRequest) ([]model.PaymentProviderBid, error) {
	var bids []model.PaymentProviderBid

	for _, provider := range c.providers {
		bid, err := c.requestBid(ctx, provider, req)
		if err != nil {
			slog.WarnContext(ctx, "failed to get bid from payment provider",
				"provider_id", provider.ID,
				"error", err,
			)
			continue
		}
		bids = append(bids, bid)
	}

	if len(bids) == 0 {
		// Return a default bid if no providers responded
		bids = append(bids, getDefaultBid(req.WorkCategory))
	}

	return bids, nil
}

// requestBid sends a bid request to a specific payment provider
func (c *ProviderClient) requestBid(ctx context.Context, provider PaymentProviderConfig, req model.PaymentBidRequest) (model.PaymentProviderBid, error) {
	// Build A2A request
	bidReq := BidRequest{
		Action:       "bid",
		Amount:       req.Amount,
		WorkCategory: req.WorkCategory,
		Currency:     req.Currency,
	}

	reqBody, err := json.Marshal(bidReq)
	if err != nil {
		return model.PaymentProviderBid{}, fmt.Errorf("marshal request: %w", err)
	}

	a2aReq := A2ARequest{
		JSONRPC: "2.0",
		Method:  "message/send",
		ID:      fmt.Sprintf("bid-%s-%d", provider.ID, time.Now().UnixNano()),
		Params: map[string]interface{}{
			"message": map[string]interface{}{
				"role": "user",
				"parts": []map[string]interface{}{
					{"type": "text", "text": string(reqBody)},
				},
			},
		},
	}

	a2aBody, err := json.Marshal(a2aReq)
	if err != nil {
		return model.PaymentProviderBid{}, fmt.Errorf("marshal a2a request: %w", err)
	}

	// Send request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", provider.Endpoint+"/a2a", bytes.NewReader(a2aBody))
	if err != nil {
		return model.PaymentProviderBid{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return model.PaymentProviderBid{}, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return model.PaymentProviderBid{}, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return model.PaymentProviderBid{}, fmt.Errorf("read response: %w", err)
	}

	var a2aResp A2AResponse
	if err := json.Unmarshal(body, &a2aResp); err != nil {
		return model.PaymentProviderBid{}, fmt.Errorf("unmarshal a2a response: %w", err)
	}

	if a2aResp.Error != nil {
		return model.PaymentProviderBid{}, fmt.Errorf("a2a error: %s", a2aResp.Error.Message)
	}

	// Extract bid from result
	var result struct {
		History []struct {
			Role  string `json:"role"`
			Parts []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"history"`
	}
	if err := json.Unmarshal(a2aResp.Result, &result); err != nil {
		return model.PaymentProviderBid{}, fmt.Errorf("unmarshal result: %w", err)
	}

	// Find the agent's response
	for _, msg := range result.History {
		if msg.Role == "agent" {
			for _, part := range msg.Parts {
				if part.Type == "text" {
					var bidResp BidResponse
					if err := json.Unmarshal([]byte(part.Text), &bidResp); err != nil {
						continue
					}

					return model.PaymentProviderBid{
						ProviderID:            bidResp.Bid.ProviderID,
						ProviderName:          bidResp.Bid.ProviderName,
						BaseFeePercent:        bidResp.Bid.BaseFeePercent,
						RewardPercent:         bidResp.Bid.RewardPercent,
						NetFeePercent:         bidResp.Bid.NetFeePercent,
						ProcessingTimeSeconds: bidResp.Bid.ProcessingTimeSeconds,
						SupportedMethods:      bidResp.Bid.SupportedMethods,
						FraudProtection:       bidResp.Bid.FraudProtection,
					}, nil
				}
			}
		}
	}

	return model.PaymentProviderBid{}, fmt.Errorf("no bid found in response")
}

// SelectBestProvider selects the best payment provider based on net fee
func (c *ProviderClient) SelectBestProvider(bids []model.PaymentProviderBid, strategy string) model.PaymentProviderSelection {
	if len(bids) == 0 {
		return model.PaymentProviderSelection{}
	}

	// Sort by strategy
	switch strategy {
	case "fastest":
		sort.Slice(bids, func(i, j int) bool {
			return bids[i].ProcessingTimeSeconds < bids[j].ProcessingTimeSeconds
		})
	case "most_secure":
		// Rank fraud protection levels
		securityRank := map[string]int{"advanced": 4, "standard": 3, "basic": 2, "none": 1}
		sort.Slice(bids, func(i, j int) bool {
			return securityRank[bids[i].FraudProtection] > securityRank[bids[j].FraudProtection]
		})
	default: // lowest_fee
		sort.Slice(bids, func(i, j int) bool {
			return bids[i].NetFeePercent < bids[j].NetFeePercent
		})
		strategy = "lowest_fee"
	}

	return model.PaymentProviderSelection{
		SelectedProvider: bids[0],
		AllBids:          bids,
		SelectionReason:  strategy,
	}
}

// getDefaultBid returns a default bid when no providers are available
func getDefaultBid(workCategory string) model.PaymentProviderBid {
	// Determine reward based on category
	reward := 1.0 // default
	if strings.Contains(workCategory, "contract") {
		reward = 1.5
	} else if strings.Contains(workCategory, "compliance") {
		reward = 2.0
	}

	return model.PaymentProviderBid{
		ProviderID:            "default",
		ProviderName:          "AEX Internal",
		BaseFeePercent:        2.0,
		RewardPercent:         reward,
		NetFeePercent:         2.0 - reward,
		ProcessingTimeSeconds: 1,
		SupportedMethods:      []string{"aex_balance"},
		FraudProtection:       "basic",
	}
}
