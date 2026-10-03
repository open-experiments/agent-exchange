package store

import (
	"context"
	"errors"

	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/model"
)

// ErrWorkNotFound is returned when no work exists with the requested ID.
// Callers must distinguish it from other store errors: only this one means
// "the work does not exist".
var ErrWorkNotFound = errors.New("work not found")

// ErrVersionConflict is returned when a conditional update fails due to
// a concurrent modification (optimistic concurrency control).
var ErrVersionConflict = errors.New("version conflict")

// WorkStore defines the interface for work persistence
type WorkStore interface {
	SaveWork(ctx context.Context, work model.WorkSpec) error
	GetWork(ctx context.Context, workID string) (model.WorkSpec, error)
	// UpdateWork persists the work. If the store supports optimistic concurrency,
	// it checks work.Version matches the stored version and returns
	// ErrVersionConflict on mismatch.
	UpdateWork(ctx context.Context, work model.WorkSpec) error
	ListWork(ctx context.Context, consumerID string, limit int) ([]model.WorkSpec, error)
	Close() error
}
