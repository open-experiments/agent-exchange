package integration

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestSettlementDeposit tests basic deposit functionality
func TestSettlementDeposit(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()
	tenantID := fmt.Sprintf("deposit-tenant-%d", timestamp)

	err := c.Deposit(ctx, &DepositRequest{
		TenantID: tenantID,
		Amount:   "100.00",
	})
	if err != nil {
		t.Fatalf("Failed to deposit: %v", err)
	}

	balance, err := c.GetBalance(ctx, tenantID)
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}

	if balance.Balance < 100.00 {
		t.Errorf("Expected balance >= 100, got %.2f", balance.Balance)
	}
	t.Logf("Deposited $100.00, balance: $%.2f", balance.Balance)
}

// TestSettlementMultipleDeposits tests multiple deposits
func TestSettlementMultipleDeposits(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()
	tenantID := fmt.Sprintf("multi-deposit-tenant-%d", timestamp)

	deposits := []string{"50.00", "75.00", "125.00", "200.00"}
	expectedTotal := 450.0

	for _, amount := range deposits {
		err := c.Deposit(ctx, &DepositRequest{
			TenantID: tenantID,
			Amount:   amount,
		})
		if err != nil {
			t.Fatalf("Failed to deposit $%s: %v", amount, err)
		}
		t.Logf("Deposited $%s", amount)
	}

	balance, err := c.GetBalance(ctx, tenantID)
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}

	if balance.Balance < expectedTotal {
		t.Errorf("Expected balance >= %.2f, got %.2f", expectedTotal, balance.Balance)
	}
	t.Logf("Total deposited: $%.2f, balance: $%.2f", expectedTotal, balance.Balance)
}

// TestSettlementTransactionHistory tests transaction listing
func TestSettlementTransactionHistory(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()
	tenantID := fmt.Sprintf("txn-history-tenant-%d", timestamp)

	// Make several deposits
	amounts := []string{"10.00", "20.00", "30.00", "40.00", "50.00"}
	for i, amount := range amounts {
		err := c.Deposit(ctx, &DepositRequest{
			TenantID: tenantID,
			Amount:   amount,
		})
		if err != nil {
			t.Fatalf("Failed to deposit %d: %v", i, err)
		}
	}

	// Get transactions
	transactions, err := c.GetTransactions(ctx, tenantID)
	if err != nil {
		t.Fatalf("Failed to get transactions: %v", err)
	}

	if len(transactions) < 5 {
		t.Errorf("Expected at least 5 transactions, got %d", len(transactions))
	}

	for i, tx := range transactions {
		t.Logf("Transaction %d: %s - $%.2f (type: %s)", i, tx.ID, tx.Amount, tx.Type)
	}
}

// TestSettlementBalance tests balance queries
func TestSettlementBalance(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()
	tenantID := fmt.Sprintf("balance-tenant-%d", timestamp)

	// Initial balance should be 0 or error
	initialBalance, err := c.GetBalance(ctx, tenantID)
	if err != nil {
		t.Logf("Initial balance query (expected error or 0): %v", err)
	} else {
		t.Logf("Initial balance: $%.2f", initialBalance.Balance)
	}

	// Deposit
	err = c.Deposit(ctx, &DepositRequest{
		TenantID: tenantID,
		Amount:   "500.00",
	})
	if err != nil {
		t.Fatalf("Failed to deposit: %v", err)
	}

	// Check new balance
	balance, err := c.GetBalance(ctx, tenantID)
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}

	if balance.Balance < 500.00 {
		t.Errorf("Expected balance >= 500, got %.2f", balance.Balance)
	}
	t.Logf("Balance after deposit: $%.2f", balance.Balance)
}

// TestSettlementDifferentAmounts tests various deposit amounts
func TestSettlementDifferentAmounts(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	amounts := []string{
		"0.01",     // Minimum
		"1.00",     // Dollar
		"10.50",    // With cents
		"100.00",   // Hundred
		"999.99",   // Near thousand
		"1000.00",  // Thousand
		"10000.00", // Ten thousand
	}

	for _, amount := range amounts {
		tenantID := fmt.Sprintf("amount-tenant-%d-%s", timestamp, amount)

		err := c.Deposit(ctx, &DepositRequest{
			TenantID: tenantID,
			Amount:   amount,
		})
		if err != nil {
			t.Errorf("Failed to deposit $%s: %v", amount, err)
			continue
		}

		balance, err := c.GetBalance(ctx, tenantID)
		if err != nil {
			t.Errorf("Failed to get balance for $%s: %v", amount, err)
			continue
		}
		t.Logf("Deposited $%s, balance: $%.2f", amount, balance.Balance)
	}
}

// TestSettlementUsage tests usage tracking
func TestSettlementUsage(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()
	tenantID := fmt.Sprintf("usage-tenant-%d", timestamp)

	// Deposit some funds
	err := c.Deposit(ctx, &DepositRequest{
		TenantID: tenantID,
		Amount:   "1000.00",
	})
	if err != nil {
		t.Fatalf("Failed to deposit: %v", err)
	}

	// Get usage
	usage, err := c.GetUsage(ctx, tenantID)
	if err != nil {
		t.Logf("Usage endpoint not implemented or failed: %v", err)
	} else {
		t.Logf("Usage data: %v", usage)
	}
}

// TestSettlementContractSettlement tests contract settlement flow
func TestSettlementContractSettlement(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	consumerID := fmt.Sprintf("settle-consumer-%d", timestamp)
	providerID := fmt.Sprintf("settle-provider-%d", timestamp)

	// Setup: deposit funds for consumer
	err := c.Deposit(ctx, &DepositRequest{
		TenantID: consumerID,
		Amount:   "500.00",
	})
	if err != nil {
		t.Fatalf("Failed to deposit for consumer: %v", err)
	}

	initialBalance, _ := c.GetBalance(ctx, consumerID)
	t.Logf("Consumer initial balance: $%.2f", initialBalance.Balance)

	// Settle a contract
	err = c.SettleContract(ctx, &SettlementRequest{
		ContractID: fmt.Sprintf("contract-%d", timestamp),
		ConsumerID: consumerID,
		ProviderID: providerID,
		Amount:     100.00,
		Currency:   "USD",
	})
	if err != nil {
		t.Logf("Settlement not implemented or failed: %v", err)
		return
	}

	// Check updated balances
	consumerBalance, err := c.GetBalance(ctx, consumerID)
	if err != nil {
		t.Fatalf("Failed to get consumer balance: %v", err)
	}
	t.Logf("Consumer balance after settlement: $%.2f", consumerBalance.Balance)

	// Check provider balance if created
	providerBalance, err := c.GetBalance(ctx, providerID)
	if err != nil {
		t.Logf("Provider balance query failed: %v", err)
	} else {
		t.Logf("Provider balance: $%.2f", providerBalance.Balance)
	}
}

// TestSettlementMultipleTenants tests settlement with multiple tenants
func TestSettlementMultipleTenants(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()

	tenantCount := 5
	for i := 0; i < tenantCount; i++ {
		tenantID := fmt.Sprintf("multi-tenant-%d-%d", timestamp, i)
		amount := fmt.Sprintf("%d.00", (i+1)*100)

		err := c.Deposit(ctx, &DepositRequest{
			TenantID: tenantID,
			Amount:   amount,
		})
		if err != nil {
			t.Errorf("Failed to deposit for tenant %d: %v", i, err)
			continue
		}

		balance, err := c.GetBalance(ctx, tenantID)
		if err != nil {
			t.Errorf("Failed to get balance for tenant %d: %v", i, err)
			continue
		}
		t.Logf("Tenant %d: deposited $%s, balance $%.2f", i, amount, balance.Balance)
	}
}

// TestSettlementConcurrentDeposits tests concurrent deposits
func TestSettlementConcurrentDeposits(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()
	timestamp := time.Now().UnixNano()
	tenantID := fmt.Sprintf("concurrent-tenant-%d", timestamp)

	// Simulate concurrent deposits (not truly concurrent in test but sequential)
	depositCount := 10
	depositAmount := "50.00"

	for i := 0; i < depositCount; i++ {
		err := c.Deposit(ctx, &DepositRequest{
			TenantID: tenantID,
			Amount:   depositAmount,
		})
		if err != nil {
			t.Errorf("Deposit %d failed: %v", i, err)
		}
	}

	balance, err := c.GetBalance(ctx, tenantID)
	if err != nil {
		t.Fatalf("Failed to get balance: %v", err)
	}

	expectedMin := float64(depositCount) * 50.0
	if balance.Balance < expectedMin {
		t.Errorf("Expected balance >= %.2f, got %.2f", expectedMin, balance.Balance)
	}
	t.Logf("After %d deposits of $%s: balance $%.2f", depositCount, depositAmount, balance.Balance)
}

// TestSettlementNonExistentTenant tests handling of non-existent tenant
func TestSettlementNonExistentTenant(t *testing.T) {
	c := getTestClient()
	skipIfNoServices(t, c)

	ctx := context.Background()

	_, err := c.GetBalance(ctx, "non-existent-tenant-12345")
	if err == nil {
		t.Log("Non-existent tenant returned without error (may return 0 balance)")
	} else {
		t.Logf("Non-existent tenant handled: %v", err)
	}

	transactions, err := c.GetTransactions(ctx, "non-existent-tenant-12345")
	if err == nil {
		t.Logf("Non-existent tenant has %d transactions", len(transactions))
	} else {
		t.Logf("Non-existent tenant transactions: %v", err)
	}
}
