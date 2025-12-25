package store

import (
	"context"

	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/model"
)

type EvaluationStore interface {
	Save(ctx context.Context, ev model.BidEvaluation) error
	GetLatest(ctx context.Context, workID string) (*model.BidEvaluation, error)
}

