package store

import (
	"context"

	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/model"
)

// WorkStore defines the interface for work persistence
type WorkStore interface {
	SaveWork(ctx context.Context, work model.WorkSpec) error
	GetWork(ctx context.Context, workID string) (model.WorkSpec, error)
	UpdateWork(ctx context.Context, work model.WorkSpec) error
	ListWork(ctx context.Context, consumerID string, limit int) ([]model.WorkSpec, error)
	Close() error
}
