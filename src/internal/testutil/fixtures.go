package testutil

import (
	"time"
)

// WorkSpecFixture represents a test work specification
type WorkSpecFixture struct {
	ID          string
	ConsumerID  string
	Category    string
	Description string
	Budget      BudgetFixture
	CreatedAt   time.Time
}

// BudgetFixture represents a test budget
type BudgetFixture struct {
	MaxPrice    float64
	BidStrategy string
}

// NewWorkSpecFixture creates a default work spec for testing
func NewWorkSpecFixture() WorkSpecFixture {
	return WorkSpecFixture{
		ID:          "work_test_001",
		ConsumerID:  "tenant_test001",
		Category:    "general",
		Description: "Test work specification",
		Budget: BudgetFixture{
			MaxPrice:    100.0,
			BidStrategy: "balanced",
		},
		CreatedAt: time.Now().UTC(),
	}
}

// WithID sets the work spec ID
func (w WorkSpecFixture) WithID(id string) WorkSpecFixture {
	w.ID = id
	return w
}

// WithConsumerID sets the consumer ID
func (w WorkSpecFixture) WithConsumerID(consumerID string) WorkSpecFixture {
	w.ConsumerID = consumerID
	return w
}

// WithCategory sets the category
func (w WorkSpecFixture) WithCategory(category string) WorkSpecFixture {
	w.Category = category
	return w
}

// WithMaxPrice sets the max price
func (w WorkSpecFixture) WithMaxPrice(price float64) WorkSpecFixture {
	w.Budget.MaxPrice = price
	return w
}

// BidFixture represents a test bid
type BidFixture struct {
	ID         string
	WorkID     string
	ProviderID string
	AgentID    string
	Price      float64
	ReceivedAt time.Time
}

// NewBidFixture creates a default bid for testing
func NewBidFixture() BidFixture {
	return BidFixture{
		ID:         "bid_test_001",
		WorkID:     "work_test_001",
		ProviderID: "provider_test_001",
		AgentID:    "agent_test_001",
		Price:      85.0,
		ReceivedAt: time.Now().UTC(),
	}
}

// WithWorkID sets the work ID
func (b BidFixture) WithWorkID(workID string) BidFixture {
	b.WorkID = workID
	return b
}

// WithPrice sets the bid price
func (b BidFixture) WithPrice(price float64) BidFixture {
	b.Price = price
	return b
}

// WithProviderID sets the provider ID
func (b BidFixture) WithProviderID(providerID string) BidFixture {
	b.ProviderID = providerID
	return b
}

// ContractFixture represents a test contract
type ContractFixture struct {
	ID          string
	WorkID      string
	ConsumerID  string
	ProviderID  string
	AgentID     string
	AgreedPrice string
	Status      string
	CreatedAt   time.Time
}

// NewContractFixture creates a default contract for testing
func NewContractFixture() ContractFixture {
	return ContractFixture{
		ID:          "contract_test_001",
		WorkID:      "work_test_001",
		ConsumerID:  "tenant_test001",
		ProviderID:  "provider_test_001",
		AgentID:     "agent_test_001",
		AgreedPrice: "85.00",
		Status:      "active",
		CreatedAt:   time.Now().UTC(),
	}
}

// WithAgreedPrice sets the agreed price
func (c ContractFixture) WithAgreedPrice(price string) ContractFixture {
	c.AgreedPrice = price
	return c
}

// WithStatus sets the contract status
func (c ContractFixture) WithStatus(status string) ContractFixture {
	c.Status = status
	return c
}

// TenantFixture represents a test tenant
type TenantFixture struct {
	ID         string
	ExternalID string
	Name       string
	Type       string
	Balance    string
}

// NewConsumerFixture creates a default consumer tenant for testing
func NewConsumerFixture() TenantFixture {
	return TenantFixture{
		ID:         "tenant_test001",
		ExternalID: "test-consumer-001",
		Name:       "Test Consumer",
		Type:       "REQUESTOR",
		Balance:    "1000.00",
	}
}

// NewProviderFixture creates a default provider tenant for testing
func NewProviderFixture() TenantFixture {
	return TenantFixture{
		ID:         "provider_test_001",
		ExternalID: "test-provider-001",
		Name:       "Test Provider",
		Type:       "PROVIDER",
		Balance:    "0.00",
	}
}

// WithBalance sets the tenant balance
func (t TenantFixture) WithBalance(balance string) TenantFixture {
	t.Balance = balance
	return t
}

// ExecutionFixture represents a test execution
type ExecutionFixture struct {
	ID             string
	WorkID         string
	ContractID     string
	ConsumerID     string
	ProviderID     string
	AgreedPrice    string
	PlatformFee    string
	ProviderPayout string
	Success        bool
}

// NewExecutionFixture creates a default execution for testing
func NewExecutionFixture() ExecutionFixture {
	return ExecutionFixture{
		ID:             "exec_test_001",
		WorkID:         "work_test_001",
		ContractID:     "contract_test_001",
		ConsumerID:     "tenant_test001",
		ProviderID:     "provider_test_001",
		AgreedPrice:    "100.00",
		PlatformFee:    "15.00",
		ProviderPayout: "85.00",
		Success:        true,
	}
}

// WithAgreedPrice sets the agreed price and recalculates fees
func (e ExecutionFixture) WithAgreedPrice(price string) ExecutionFixture {
	e.AgreedPrice = price
	// Note: Caller should recalculate platform fee and payout if needed
	return e
}

// WithSuccess sets the success status
func (e ExecutionFixture) WithSuccess(success bool) ExecutionFixture {
	e.Success = success
	return e
}
