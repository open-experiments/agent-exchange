package store

import (
	"context"

	"github.com/parlakisik/agent-exchange/aex-contract-engine/internal/model"
)

type ContractStore interface {
	Save(ctx context.Context, c model.Contract) error
	Get(ctx context.Context, contractID string) (*model.Contract, error)
	Update(ctx context.Context, c model.Contract) error
}



