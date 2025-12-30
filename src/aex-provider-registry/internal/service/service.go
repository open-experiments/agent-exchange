package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/aex-provider-registry/internal/model"
	"github.com/parlakisik/agent-exchange/aex-provider-registry/internal/store"
)

type Service struct {
	store store.Store
}

func New(st store.Store) *Service {
	return &Service{store: st}
}

func (s *Service) HandleRegisterProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req model.ProviderRegistrationRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if err := validateHTTPSURL(req.Endpoint); err != nil {
		http.Error(w, "endpoint must be a valid https URL", http.StatusBadRequest)
		return
	}
	if req.BidWebhook != "" {
		if err := validateHTTPSURL(req.BidWebhook); err != nil {
			http.Error(w, "bid_webhook must be a valid https URL", http.StatusBadRequest)
			return
		}
	}

	apiKey := generateToken("aex_pk_live_")
	apiSecret := generateToken("aex_sk_live_")
	keyHash := sha256Hex(apiKey)
	secretHash := sha256Hex(apiSecret)

	now := time.Now().UTC()
	p := model.Provider{
		ProviderID:    generateToken("prov_"),
		Name:          req.Name,
		Description:   req.Description,
		Endpoint:      req.Endpoint,
		BidWebhook:    req.BidWebhook,
		Capabilities:  req.Capabilities,
		ContactEmail:  req.ContactEmail,
		Metadata:      req.Metadata,
		APIKeyHash:    keyHash,
		APISecretHash: secretHash,
		Status:        model.ProviderStatusActive, // Option A: keep it usable immediately for local dev
		TrustScore:    0.3,
		TrustTier:     model.TrustTierUnverified,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.store.CreateProvider(ctx, p); err != nil {
		http.Error(w, "failed to create provider", http.StatusInternalServerError)
		return
	}

	resp := model.ProviderRegistrationResponse{
		ProviderID: p.ProviderID,
		APIKey:     apiKey,
		APISecret:  apiSecret,
		Status:     p.Status,
		TrustTier:  p.TrustTier,
		CreatedAt:  p.CreatedAt,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Service) HandleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req model.SubscriptionRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.ProviderID = strings.TrimSpace(req.ProviderID)
	if req.ProviderID == "" {
		http.Error(w, "provider_id is required", http.StatusBadRequest)
		return
	}
	if len(req.Categories) == 0 {
		http.Error(w, "categories is required", http.StatusBadRequest)
		return
	}
	p, err := s.store.GetProvider(ctx, req.ProviderID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if p == nil {
		http.Error(w, "provider not found", http.StatusNotFound)
		return
	}

	if req.Delivery.Method == "webhook" && req.Delivery.WebhookURL != "" {
		if err := validateHTTPSURL(req.Delivery.WebhookURL); err != nil {
			http.Error(w, "delivery.webhook_url must be a valid https URL", http.StatusBadRequest)
			return
		}
	}

	now := time.Now().UTC()
	sub := model.Subscription{
		SubscriptionID: generateToken("sub_"),
		ProviderID:     req.ProviderID,
		Categories:     req.Categories,
		Filters:        req.Filters,
		Delivery:       req.Delivery,
		Status:         "ACTIVE",
		CreatedAt:      now,
	}
	if err := s.store.CreateSubscription(ctx, sub); err != nil {
		http.Error(w, "failed to create subscription", http.StatusInternalServerError)
		return
	}

	resp := model.SubscriptionResponse{
		SubscriptionID: sub.SubscriptionID,
		ProviderID:     sub.ProviderID,
		Categories:     sub.Categories,
		Status:         sub.Status,
		CreatedAt:      sub.CreatedAt,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Service) HandleGetProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract provider_id from path: /v1/providers/{provider_id}
	providerID := strings.TrimPrefix(r.URL.Path, "/v1/providers/")
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		http.Error(w, "provider_id is required", http.StatusBadRequest)
		return
	}

	p, err := s.store.GetProvider(ctx, providerID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if p == nil {
		http.Error(w, "provider not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"provider_id":  p.ProviderID,
		"name":         p.Name,
		"endpoint":     p.Endpoint,
		"status":       p.Status,
		"trust_score":  p.TrustScore,
		"trust_tier":   p.TrustTier,
		"capabilities": p.Capabilities,
		"created_at":   p.CreatedAt,
		"updated_at":   p.UpdatedAt,
	})
}

func (s *Service) HandleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	providerID := strings.TrimSpace(r.URL.Query().Get("provider_id"))

	subs, err := s.store.ListSubscriptions(ctx)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	result := make([]model.Subscription, 0)
	for _, sub := range subs {
		if providerID != "" && sub.ProviderID != providerID {
			continue
		}
		result = append(result, sub)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"subscriptions": result,
		"total":         len(result),
	})
}

func (s *Service) HandleInternalSubscribed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	if category == "" {
		http.Error(w, "category is required", http.StatusBadRequest)
		return
	}

	subs, err := s.store.ListSubscriptions(ctx)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	providerIDs := make([]string, 0)
	type subHit struct {
		providerID string
		webhookURL string
	}
	hits := make([]subHit, 0)

	for _, sub := range subs {
		if sub.Status != "ACTIVE" {
			continue
		}
		matched := false
		for _, pat := range sub.Categories {
			ok, err := path.Match(pat, category)
			if err == nil && ok {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}

		webhookURL := ""
		if sub.Delivery.Method == "webhook" && sub.Delivery.WebhookURL != "" {
			webhookURL = sub.Delivery.WebhookURL
		}
		providerIDs = append(providerIDs, sub.ProviderID)
		hits = append(hits, subHit{providerID: sub.ProviderID, webhookURL: webhookURL})
	}

	providers, err := s.store.ListProviders(ctx, providerIDs)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	byID := map[string]model.Provider{}
	for _, p := range providers {
		byID[p.ProviderID] = p
	}

	outProviders := make([]map[string]any, 0)
	for _, h := range hits {
		p, ok := byID[h.providerID]
		if !ok {
			continue
		}
		if p.Status != model.ProviderStatusActive {
			continue
		}
		webhookURL := h.webhookURL
		if webhookURL == "" {
			webhookURL = p.BidWebhook
		}
		outProviders = append(outProviders, map[string]any{
			"provider_id": h.providerID,
			"webhook_url": webhookURL,
			"trust_score": p.TrustScore,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"category":  category,
		"providers": outProviders,
	})
}

func validateHTTPSURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errors.New("empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" {
		return errors.New("scheme must be https")
	}
	if u.Host == "" {
		return errors.New("missing host")
	}
	return nil
}

func decodeJSON(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	defer r.Body.Close()
	return json.Unmarshal(body, v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func generateToken(prefix string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:8])
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (s *Service) HandleValidateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	apiKey := strings.TrimSpace(r.URL.Query().Get("api_key"))
	if apiKey == "" {
		http.Error(w, "api_key is required", http.StatusBadRequest)
		return
	}

	keyHash := sha256Hex(apiKey)

	provider, err := s.store.GetProviderByAPIKeyHash(ctx, keyHash)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if provider == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid":       false,
			"provider_id": "",
			"status":      "",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":       provider.Status == model.ProviderStatusActive,
		"provider_id": provider.ProviderID,
		"status":      provider.Status,
	})
}


