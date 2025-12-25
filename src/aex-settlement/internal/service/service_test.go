package service

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestCalculateCost(t *testing.T) {
	tests := []struct {
		name               string
		agreedPrice        string
		wantAgreedPrice    string
		wantPlatformFee    string
		wantProviderPayout string
	}{
		{
			name:               "standard price",
			agreedPrice:        "100.00",
			wantAgreedPrice:    "100",
			wantPlatformFee:    "15",
			wantProviderPayout: "85",
		},
		{
			name:               "small price",
			agreedPrice:        "10.00",
			wantAgreedPrice:    "10",
			wantPlatformFee:    "1.5",
			wantProviderPayout: "8.5",
		},
		{
			name:               "large price",
			agreedPrice:        "1000.00",
			wantAgreedPrice:    "1000",
			wantPlatformFee:    "150",
			wantProviderPayout: "850",
		},
		{
			name:               "price with decimals",
			agreedPrice:        "123.45",
			wantAgreedPrice:    "123.45",
			wantPlatformFee:    "18.5175",
			wantProviderPayout: "104.9325",
		},
		{
			name:               "zero price",
			agreedPrice:        "0.00",
			wantAgreedPrice:    "0",
			wantPlatformFee:    "0",
			wantProviderPayout: "0",
		},
		{
			name:               "price requiring rounding",
			agreedPrice:        "99.99",
			wantAgreedPrice:    "99.99",
			wantPlatformFee:    "14.9985",
			wantProviderPayout: "84.9915",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create service with nil store since we're only testing calculateCost
			svc := &Service{}

			agreedPrice, err := decimal.NewFromString(tt.agreedPrice)
			if err != nil {
				t.Fatalf("Invalid test agreedPrice: %v", err)
			}

			breakdown := svc.calculateCost(agreedPrice)

			if breakdown.AgreedPrice != tt.wantAgreedPrice {
				t.Errorf("calculateCost() agreedPrice = %v, want %v", breakdown.AgreedPrice, tt.wantAgreedPrice)
			}

			if breakdown.PlatformFee != tt.wantPlatformFee {
				t.Errorf("calculateCost() platformFee = %v, want %v", breakdown.PlatformFee, tt.wantPlatformFee)
			}

			if breakdown.ProviderPayout != tt.wantProviderPayout {
				t.Errorf("calculateCost() providerPayout = %v, want %v", breakdown.ProviderPayout, tt.wantProviderPayout)
			}
		})
	}
}

func TestPlatformFeeRate(t *testing.T) {
	// Verify platform fee rate is 15%
	expectedRate := decimal.RequireFromString("0.15")

	if !PlatformFeeRate.Equal(expectedRate) {
		t.Errorf("PlatformFeeRate = %v, want %v (15%%)", PlatformFeeRate, expectedRate)
	}
}

func TestCostBreakdownConsistency(t *testing.T) {
	svc := &Service{}

	tests := []string{"100.00", "50.00", "1.00", "999.99", "0.01"}

	for _, priceStr := range tests {
		t.Run("price_"+priceStr, func(t *testing.T) {
			agreedPrice, _ := decimal.NewFromString(priceStr)
			breakdown := svc.calculateCost(agreedPrice)

			// Parse breakdown values back to decimals
			agreed, _ := decimal.NewFromString(breakdown.AgreedPrice)
			platformFee, _ := decimal.NewFromString(breakdown.PlatformFee)
			providerPayout, _ := decimal.NewFromString(breakdown.ProviderPayout)

			// Verify: agreedPrice = platformFee + providerPayout
			sum := platformFee.Add(providerPayout)

			if !sum.Equal(agreed) {
				t.Errorf("Cost breakdown inconsistent: %v + %v = %v, want %v",
					platformFee, providerPayout, sum, agreed)
			}

			// Verify platform fee is 15% of agreed price
			expectedFee := agreed.Mul(PlatformFeeRate).Round(6)
			if !platformFee.Equal(expectedFee) {
				t.Errorf("Platform fee = %v, want %v (15%% of %v)",
					platformFee, expectedFee, agreed)
			}
		})
	}
}
