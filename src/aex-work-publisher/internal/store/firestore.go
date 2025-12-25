package store

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/model"
	"google.golang.org/api/iterator"
)

type FirestoreStore struct {
	client     *firestore.Client
	collection string
}

func NewFirestoreStore(projectID, collection string) (*FirestoreStore, error) {
	client, err := firestore.NewClient(context.Background(), projectID)
	if err != nil {
		return nil, fmt.Errorf("firestore client: %w", err)
	}
	return &FirestoreStore{
		client:     client,
		collection: collection,
	}, nil
}

func (s *FirestoreStore) SaveWork(ctx context.Context, work model.WorkSpec) error {
	_, err := s.client.Collection(s.collection).Doc(work.ID).Set(ctx, work)
	if err != nil {
		return fmt.Errorf("save work: %w", err)
	}
	return nil
}

func (s *FirestoreStore) GetWork(ctx context.Context, workID string) (model.WorkSpec, error) {
	doc, err := s.client.Collection(s.collection).Doc(workID).Get(ctx)
	if err != nil {
		return model.WorkSpec{}, fmt.Errorf("get work: %w", err)
	}

	var work model.WorkSpec
	if err := doc.DataTo(&work); err != nil {
		return model.WorkSpec{}, fmt.Errorf("decode work: %w", err)
	}
	return work, nil
}

func (s *FirestoreStore) UpdateWork(ctx context.Context, work model.WorkSpec) error {
	_, err := s.client.Collection(s.collection).Doc(work.ID).Set(ctx, work)
	if err != nil {
		return fmt.Errorf("update work: %w", err)
	}
	return nil
}

func (s *FirestoreStore) ListWork(ctx context.Context, consumerID string, limit int) ([]model.WorkSpec, error) {
	query := s.client.Collection(s.collection).
		Where("consumer_id", "==", consumerID).
		OrderBy("created_at", firestore.Desc).
		Limit(limit)

	iter := query.Documents(ctx)
	defer iter.Stop()

	var works []model.WorkSpec
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("iterate works: %w", err)
		}

		var work model.WorkSpec
		if err := doc.DataTo(&work); err != nil {
			return nil, fmt.Errorf("decode work: %w", err)
		}
		works = append(works, work)
	}

	return works, nil
}

func (s *FirestoreStore) Close() error {
	return s.client.Close()
}
