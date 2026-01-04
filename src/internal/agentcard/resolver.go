package agentcard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// WellKnownPath is the standard A2A agent card path
	WellKnownPath = "/.well-known/agent-card.json"
	// DefaultCacheTTL is how long to cache agent cards
	DefaultCacheTTL = 5 * time.Minute
	// DefaultTimeout for HTTP requests
	DefaultTimeout = 10 * time.Second
)

// Resolver fetches and caches A2A Agent Cards
type Resolver struct {
	httpClient *http.Client
	cache      map[string]*cacheEntry
	cacheMu    sync.RWMutex
	cacheTTL   time.Duration
}

type cacheEntry struct {
	card      *ResolvedAgentCard
	expiresAt time.Time
}

// ResolverOption configures the Resolver
type ResolverOption func(*Resolver)

// WithCacheTTL sets the cache TTL
func WithCacheTTL(ttl time.Duration) ResolverOption {
	return func(r *Resolver) {
		r.cacheTTL = ttl
	}
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(client *http.Client) ResolverOption {
	return func(r *Resolver) {
		r.httpClient = client
	}
}

// NewResolver creates a new Agent Card resolver
func NewResolver(opts ...ResolverOption) *Resolver {
	r := &Resolver{
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		cache:    make(map[string]*cacheEntry),
		cacheTTL: DefaultCacheTTL,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Resolve fetches the agent card from the given base URL
func (r *Resolver) Resolve(ctx context.Context, baseURL string) (*ResolvedAgentCard, error) {
	// Normalize URL
	agentCardURL, err := r.buildAgentCardURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	// Check cache
	if cached := r.getCached(agentCardURL); cached != nil {
		slog.DebugContext(ctx, "agent card cache hit", "url", agentCardURL)
		return cached, nil
	}

	// Fetch from remote
	slog.InfoContext(ctx, "fetching agent card", "url", agentCardURL)
	card, err := r.fetch(ctx, agentCardURL)
	if err != nil {
		return nil, err
	}

	// Cache the result
	r.setCache(agentCardURL, card)

	return card, nil
}

// ResolveMultiple fetches agent cards from multiple URLs concurrently
func (r *Resolver) ResolveMultiple(ctx context.Context, baseURLs []string) ([]*ResolvedAgentCard, []error) {
	results := make([]*ResolvedAgentCard, len(baseURLs))
	errors := make([]error, len(baseURLs))

	var wg sync.WaitGroup
	for i, baseURL := range baseURLs {
		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()
			card, err := r.Resolve(ctx, url)
			if err != nil {
				errors[idx] = err
			} else {
				results[idx] = card
			}
		}(i, baseURL)
	}
	wg.Wait()

	return results, errors
}

// ExtractSkills extracts all skills from an agent card for indexing
func (r *Resolver) ExtractSkills(card *ResolvedAgentCard, providerID string) []SkillIndex {
	skills := make([]SkillIndex, 0, len(card.Skills))

	for _, skill := range card.Skills {
		skills = append(skills, SkillIndex{
			SkillID:     skill.ID,
			SkillName:   skill.Name,
			Description: skill.Description,
			Tags:        skill.Tags,
			ProviderID:  providerID,
			AgentName:   card.Name,
			AgentURL:    card.URL,
			A2AEndpoint: r.deriveA2AEndpoint(card.SourceURL),
		})
	}

	return skills
}

// Validate checks if an agent card is valid and well-formed
func (r *Resolver) Validate(card *AgentCard) error {
	if card.Name == "" {
		return fmt.Errorf("agent card missing required field: name")
	}
	if card.URL == "" {
		return fmt.Errorf("agent card missing required field: url")
	}
	if len(card.Skills) == 0 {
		return fmt.Errorf("agent card must have at least one skill")
	}

	for i, skill := range card.Skills {
		if skill.ID == "" {
			return fmt.Errorf("skill %d missing required field: id", i)
		}
		if skill.Name == "" {
			return fmt.Errorf("skill %d missing required field: name", i)
		}
	}

	return nil
}

// InvalidateCache removes a cached agent card
func (r *Resolver) InvalidateCache(baseURL string) {
	agentCardURL, err := r.buildAgentCardURL(baseURL)
	if err != nil {
		return
	}

	r.cacheMu.Lock()
	delete(r.cache, agentCardURL)
	r.cacheMu.Unlock()
}

// ClearCache removes all cached agent cards
func (r *Resolver) ClearCache() {
	r.cacheMu.Lock()
	r.cache = make(map[string]*cacheEntry)
	r.cacheMu.Unlock()
}

func (r *Resolver) buildAgentCardURL(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", fmt.Errorf("empty base URL")
	}

	// If already points to agent-card.json, use as-is
	if strings.HasSuffix(baseURL, "/agent-card.json") {
		return baseURL, nil
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	// Ensure HTTPS in production
	if u.Scheme == "" {
		u.Scheme = "https"
	}

	// Add well-known path
	u.Path = strings.TrimSuffix(u.Path, "/") + WellKnownPath

	return u.String(), nil
}

func (r *Resolver) fetch(ctx context.Context, agentCardURL string) (*ResolvedAgentCard, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, agentCardURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AEX-AgentCardResolver/1.0")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch agent card: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("agent card fetch failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var card AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return nil, fmt.Errorf("decode agent card: %w", err)
	}

	if err := r.Validate(&card); err != nil {
		return nil, fmt.Errorf("invalid agent card: %w", err)
	}

	now := time.Now()
	resolved := &ResolvedAgentCard{
		AgentCard:  card,
		SourceURL:  agentCardURL,
		ResolvedAt: now,
		ValidUntil: now.Add(r.cacheTTL),
	}

	return resolved, nil
}

func (r *Resolver) getCached(url string) *ResolvedAgentCard {
	r.cacheMu.RLock()
	entry, ok := r.cache[url]
	r.cacheMu.RUnlock()

	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}

	return entry.card
}

func (r *Resolver) setCache(url string, card *ResolvedAgentCard) {
	r.cacheMu.Lock()
	r.cache[url] = &cacheEntry{
		card:      card,
		expiresAt: time.Now().Add(r.cacheTTL),
	}
	r.cacheMu.Unlock()
}

func (r *Resolver) deriveA2AEndpoint(agentCardURL string) string {
	// The A2A endpoint is typically at the same host, path /a2a
	u, err := url.Parse(agentCardURL)
	if err != nil {
		return ""
	}
	u.Path = "/a2a"
	return u.String()
}
