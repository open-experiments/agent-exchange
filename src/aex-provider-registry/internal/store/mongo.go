package store

import (
	"context"
	"time"

	"github.com/parlakisik/agent-exchange/aex-provider-registry/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoStore struct {
	providers   *mongo.Collection
	subs        *mongo.Collection
	agentCards  *mongo.Collection
	skillIndex  *mongo.Collection
}

func NewMongoStore(client *mongo.Client, dbName, providersColl, subsColl string) *MongoStore {
	db := client.Database(dbName)
	return &MongoStore{
		providers:   db.Collection(providersColl),
		subs:        db.Collection(subsColl),
		agentCards:  db.Collection("agent_cards"),
		skillIndex:  db.Collection("skill_index"),
	}
}

func (s *MongoStore) EnsureIndexes(ctx context.Context) error {
	_, err := s.providers.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "provider_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}
	_, err = s.providers.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "api_key_hash", Value: 1}},
		Options: options.Index().SetSparse(true),
	})
	if err != nil {
		return err
	}
	_, err = s.subs.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "subscription_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}
	// A2A indexes
	_, err = s.agentCards.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "provider_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}
	_, err = s.skillIndex.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "tags", Value: 1}},
	})
	if err != nil {
		return err
	}
	_, err = s.skillIndex.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "provider_id", Value: 1}},
	})
	return err
}

func (s *MongoStore) CreateProvider(ctx context.Context, p model.Provider) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.providers.InsertOne(ctx, p)
	return err
}

func (s *MongoStore) GetProvider(ctx context.Context, providerID string) (*model.Provider, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res := s.providers.FindOne(ctx, bson.M{"provider_id": providerID})
	if res.Err() == mongo.ErrNoDocuments {
		return nil, nil
	}
	if res.Err() != nil {
		return nil, res.Err()
	}
	var p model.Provider
	if err := res.Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *MongoStore) GetProviderByAPIKeyHash(ctx context.Context, apiKeyHash string) (*model.Provider, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res := s.providers.FindOne(ctx, bson.M{"api_key_hash": apiKeyHash})
	if res.Err() == mongo.ErrNoDocuments {
		return nil, nil
	}
	if res.Err() != nil {
		return nil, res.Err()
	}
	var p model.Provider
	if err := res.Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *MongoStore) GetProviderByName(ctx context.Context, name string) (*model.Provider, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res := s.providers.FindOne(ctx, bson.M{"name": name})
	if res.Err() == mongo.ErrNoDocuments {
		return nil, nil
	}
	if res.Err() != nil {
		return nil, res.Err()
	}
	var p model.Provider
	if err := res.Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *MongoStore) ListProviders(ctx context.Context, providerIDs []string) ([]model.Provider, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cur, err := s.providers.Find(ctx, bson.M{"provider_id": bson.M{"$in": providerIDs}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []model.Provider
	for cur.Next(ctx) {
		var p model.Provider
		if err := cur.Decode(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *MongoStore) CreateSubscription(ctx context.Context, sub model.Subscription) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.subs.InsertOne(ctx, sub)
	return err
}

func (s *MongoStore) ListSubscriptions(ctx context.Context) ([]model.Subscription, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cur, err := s.subs.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []model.Subscription
	for cur.Next(ctx) {
		var sub model.Subscription
		if err := cur.Decode(&sub); err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *MongoStore) ListAllProviders(ctx context.Context) ([]model.Provider, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cur, err := s.providers.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []model.Provider
	for cur.Next(ctx) {
		var p model.Provider
		if err := cur.Decode(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, cur.Err()
}

func (s *MongoStore) UpdateProvider(ctx context.Context, p model.Provider) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.providers.UpdateOne(ctx,
		bson.M{"provider_id": p.ProviderID},
		bson.M{"$set": p},
	)
	return err
}

// agentCardDoc wraps AgentCard with provider info for storage
type agentCardDoc struct {
	ProviderID  string          `bson:"provider_id"`
	A2AEndpoint string          `bson:"a2a_endpoint"`
	AgentCard   model.AgentCard `bson:"agent_card"`
	UpdatedAt   time.Time       `bson:"updated_at"`
}

func (s *MongoStore) SaveAgentCard(ctx context.Context, providerID string, card model.AgentCard, a2aEndpoint string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	doc := agentCardDoc{
		ProviderID:  providerID,
		A2AEndpoint: a2aEndpoint,
		AgentCard:   card,
		UpdatedAt:   time.Now(),
	}

	_, err := s.agentCards.UpdateOne(ctx,
		bson.M{"provider_id": providerID},
		bson.M{"$set": doc},
		options.Update().SetUpsert(true),
	)
	return err
}

func (s *MongoStore) GetProviderWithA2A(ctx context.Context, providerID string) (*model.ProviderWithA2A, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Get provider
	res := s.providers.FindOne(ctx, bson.M{"provider_id": providerID})
	if res.Err() == mongo.ErrNoDocuments {
		return nil, nil
	}
	if res.Err() != nil {
		return nil, res.Err()
	}
	var p model.Provider
	if err := res.Decode(&p); err != nil {
		return nil, err
	}

	result := &model.ProviderWithA2A{Provider: p}

	// Get agent card
	cardRes := s.agentCards.FindOne(ctx, bson.M{"provider_id": providerID})
	if cardRes.Err() == nil {
		var doc agentCardDoc
		if err := cardRes.Decode(&doc); err == nil {
			result.AgentCard = &doc.AgentCard
			result.A2AEndpoint = doc.A2AEndpoint
		}
	}

	return result, nil
}

func (s *MongoStore) IndexSkills(ctx context.Context, providerID string, skills []model.SkillIndex) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Delete existing skills for this provider
	_, err := s.skillIndex.DeleteMany(ctx, bson.M{"provider_id": providerID})
	if err != nil {
		return err
	}

	if len(skills) == 0 {
		return nil
	}

	// Insert new skills
	now := time.Now()
	docs := make([]interface{}, len(skills))
	for i, skill := range skills {
		skill.CreatedAt = now
		docs[i] = skill
	}

	_, err = s.skillIndex.InsertMany(ctx, docs)
	return err
}

func (s *MongoStore) SearchBySkillTags(ctx context.Context, tags []string, minTrust float64, limit int) ([]model.ProviderSearchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 100
	}

	// Group by provider
	providerSkills := make(map[string][]model.SkillIndex)
	providerTags := make(map[string]map[string]bool)

	// First, try skill_index collection
	cur, err := s.skillIndex.Find(ctx, bson.M{"tags": bson.M{"$in": tags}})
	if err == nil {
		defer cur.Close(ctx)
		for cur.Next(ctx) {
			var skill model.SkillIndex
			if err := cur.Decode(&skill); err != nil {
				continue
			}
			providerSkills[skill.ProviderID] = append(providerSkills[skill.ProviderID], skill)
			if providerTags[skill.ProviderID] == nil {
				providerTags[skill.ProviderID] = make(map[string]bool)
			}
			for _, tag := range skill.Tags {
				for _, searchTag := range tags {
					if tag == searchTag {
						providerTags[skill.ProviderID][tag] = true
					}
				}
			}
		}
	}

	// Also search providers by capabilities (fallback for simple registration)
	provCur, err := s.providers.Find(ctx, bson.M{
		"capabilities": bson.M{"$in": tags},
		"status":       model.ProviderStatusActive,
	})
	if err == nil {
		defer provCur.Close(ctx)
		for provCur.Next(ctx) {
			var p model.Provider
			if err := provCur.Decode(&p); err != nil {
				continue
			}
			// Add to results if not already from skill_index
			if _, exists := providerSkills[p.ProviderID]; !exists {
				providerSkills[p.ProviderID] = nil // mark as from capabilities
				providerTags[p.ProviderID] = make(map[string]bool)
				for _, cap := range p.Capabilities {
					for _, searchTag := range tags {
						if cap == searchTag {
							providerTags[p.ProviderID][cap] = true
						}
					}
				}
			}
		}
	}

	// Get provider details
	providerIDs := make([]string, 0, len(providerSkills))
	for pid := range providerSkills {
		providerIDs = append(providerIDs, pid)
	}

	providers, err := s.ListProviders(ctx, providerIDs)
	if err != nil {
		return nil, err
	}

	providerMap := make(map[string]model.Provider)
	for _, p := range providers {
		providerMap[p.ProviderID] = p
	}

	// Get A2A endpoints
	cardCur, err := s.agentCards.Find(ctx, bson.M{"provider_id": bson.M{"$in": providerIDs}})
	if err != nil {
		return nil, err
	}
	defer cardCur.Close(ctx)

	a2aEndpoints := make(map[string]string)
	for cardCur.Next(ctx) {
		var doc agentCardDoc
		if err := cardCur.Decode(&doc); err == nil {
			a2aEndpoints[doc.ProviderID] = doc.A2AEndpoint
		}
	}

	// Build results
	results := make([]model.ProviderSearchResult, 0)
	for providerID, skills := range providerSkills {
		p, ok := providerMap[providerID]
		if !ok || p.Status != model.ProviderStatusActive {
			continue
		}
		if p.TrustScore < minTrust {
			continue
		}

		matchedTags := make([]string, 0)
		for tag := range providerTags[providerID] {
			matchedTags = append(matchedTags, tag)
		}

		skillIDs := make([]string, 0, len(skills))
		for _, skill := range skills {
			skillIDs = append(skillIDs, skill.SkillID)
		}

		results = append(results, model.ProviderSearchResult{
			ProviderID:  p.ProviderID,
			Name:        p.Name,
			Description: p.Description,
			Endpoint:    p.Endpoint,
			A2AEndpoint: a2aEndpoints[providerID],
			TrustScore:  p.TrustScore,
			TrustTier:   string(p.TrustTier),
			Skills:      skillIDs,
			MatchedTags: matchedTags,
		})

		if len(results) >= limit {
			break
		}
	}

	return results, nil
}
