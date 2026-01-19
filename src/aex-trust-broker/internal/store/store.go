package store

import (
	"context"

	"github.com/parlakisik/agent-exchange/aex-trust-broker/internal/model"
)

type Store interface {
	UpsertTrustRecord(ctx context.Context, rec model.TrustRecord) error
	GetTrustRecord(ctx context.Context, providerID string) (*model.TrustRecord, error)

	SaveOutcome(ctx context.Context, out model.ContractOutcome) error
	ListOutcomes(ctx context.Context, providerID string, limit int) ([]model.ContractOutcome, error)
}
