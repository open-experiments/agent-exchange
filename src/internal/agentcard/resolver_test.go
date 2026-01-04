package agentcard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolver_Resolve(t *testing.T) {
	// Create a test server that serves an agent card
	agentCard := AgentCard{
		Name:        "Test Legal Agent",
		Description: "A test agent for legal tasks",
		URL:         "https://example.com/agent",
		Version:     "1.0.0",
		Provider: &Provider{
			Organization: "Test Corp",
			URL:          "https://testcorp.com",
		},
		Capabilities: Capabilities{
			Streaming: true,
		},
		Skills: []Skill{
			{
				ID:          "contract_review",
				Name:        "Contract Review",
				Description: "Review contracts for risks",
				Tags:        []string{"legal", "contracts"},
			},
			{
				ID:          "legal_research",
				Name:        "Legal Research",
				Description: "Research legal precedents",
				Tags:        []string{"legal", "research"},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/agent-card.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agentCard)
	}))
	defer server.Close()

	resolver := NewResolver(WithCacheTTL(1 * time.Minute))

	// Test successful resolution
	card, err := resolver.Resolve(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if card.Name != "Test Legal Agent" {
		t.Errorf("expected name 'Test Legal Agent', got %q", card.Name)
	}

	if len(card.Skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(card.Skills))
	}

	if card.Skills[0].ID != "contract_review" {
		t.Errorf("expected first skill ID 'contract_review', got %q", card.Skills[0].ID)
	}
}

func TestResolver_ResolveWithCache(t *testing.T) {
	callCount := 0
	agentCard := AgentCard{
		Name:    "Cached Agent",
		URL:     "https://example.com/agent",
		Version: "1.0.0",
		Skills: []Skill{
			{ID: "skill1", Name: "Skill 1"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agentCard)
	}))
	defer server.Close()

	resolver := NewResolver(WithCacheTTL(1 * time.Hour))

	// First call - should hit the server
	_, err := resolver.Resolve(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 server call, got %d", callCount)
	}

	// Second call - should use cache
	_, err = resolver.Resolve(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected still 1 server call (cache hit), got %d", callCount)
	}
}

func TestResolver_ResolveMultiple(t *testing.T) {
	createServer := func(name string) *httptest.Server {
		agentCard := AgentCard{
			Name:    name,
			URL:     "https://example.com/" + name,
			Version: "1.0.0",
			Skills: []Skill{
				{ID: "skill1", Name: "Skill 1"},
			},
		}
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(agentCard)
		}))
	}

	server1 := createServer("Agent1")
	server2 := createServer("Agent2")
	defer server1.Close()
	defer server2.Close()

	resolver := NewResolver()

	cards, errors := resolver.ResolveMultiple(context.Background(), []string{
		server1.URL,
		server2.URL,
	})

	for i, err := range errors {
		if err != nil {
			t.Errorf("resolve %d failed: %v", i, err)
		}
	}

	if len(cards) != 2 {
		t.Errorf("expected 2 cards, got %d", len(cards))
	}

	if cards[0].Name != "Agent1" {
		t.Errorf("expected first card name 'Agent1', got %q", cards[0].Name)
	}

	if cards[1].Name != "Agent2" {
		t.Errorf("expected second card name 'Agent2', got %q", cards[1].Name)
	}
}

func TestResolver_ExtractSkills(t *testing.T) {
	resolver := NewResolver()

	card := &ResolvedAgentCard{
		AgentCard: AgentCard{
			Name: "Legal Agent",
			URL:  "https://legal.example.com/agent",
			Skills: []Skill{
				{
					ID:          "contract_review",
					Name:        "Contract Review",
					Description: "Review contracts",
					Tags:        []string{"legal", "contracts"},
				},
				{
					ID:          "compliance",
					Name:        "Compliance Check",
					Description: "Check compliance",
					Tags:        []string{"legal", "compliance"},
				},
			},
		},
		SourceURL: "https://legal.example.com/.well-known/agent-card.json",
	}

	skills := resolver.ExtractSkills(card, "prov_123")

	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	if skills[0].SkillID != "contract_review" {
		t.Errorf("expected skill ID 'contract_review', got %q", skills[0].SkillID)
	}

	if skills[0].ProviderID != "prov_123" {
		t.Errorf("expected provider ID 'prov_123', got %q", skills[0].ProviderID)
	}

	if skills[0].A2AEndpoint != "https://legal.example.com/a2a" {
		t.Errorf("expected A2A endpoint 'https://legal.example.com/a2a', got %q", skills[0].A2AEndpoint)
	}

	if len(skills[0].Tags) != 2 || skills[0].Tags[0] != "legal" {
		t.Errorf("expected tags [legal, contracts], got %v", skills[0].Tags)
	}
}

func TestResolver_Validate(t *testing.T) {
	resolver := NewResolver()

	tests := []struct {
		name    string
		card    AgentCard
		wantErr bool
	}{
		{
			name: "valid card",
			card: AgentCard{
				Name:    "Agent",
				URL:     "https://example.com",
				Version: "1.0.0",
				Skills: []Skill{
					{ID: "skill1", Name: "Skill 1"},
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			card: AgentCard{
				URL:     "https://example.com",
				Version: "1.0.0",
				Skills: []Skill{
					{ID: "skill1", Name: "Skill 1"},
				},
			},
			wantErr: true,
		},
		{
			name: "missing url",
			card: AgentCard{
				Name:    "Agent",
				Version: "1.0.0",
				Skills: []Skill{
					{ID: "skill1", Name: "Skill 1"},
				},
			},
			wantErr: true,
		},
		{
			name: "no skills",
			card: AgentCard{
				Name:    "Agent",
				URL:     "https://example.com",
				Version: "1.0.0",
				Skills:  []Skill{},
			},
			wantErr: true,
		},
		{
			name: "skill missing id",
			card: AgentCard{
				Name:    "Agent",
				URL:     "https://example.com",
				Version: "1.0.0",
				Skills: []Skill{
					{Name: "Skill 1"},
				},
			},
			wantErr: true,
		},
		{
			name: "skill missing name",
			card: AgentCard{
				Name:    "Agent",
				URL:     "https://example.com",
				Version: "1.0.0",
				Skills: []Skill{
					{ID: "skill1"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := resolver.Validate(&tt.card)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolver_InvalidateCache(t *testing.T) {
	callCount := 0
	agentCard := AgentCard{
		Name:    "Agent",
		URL:     "https://example.com/agent",
		Version: "1.0.0",
		Skills: []Skill{
			{ID: "skill1", Name: "Skill 1"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agentCard)
	}))
	defer server.Close()

	resolver := NewResolver(WithCacheTTL(1 * time.Hour))

	// First call
	_, _ = resolver.Resolve(context.Background(), server.URL)
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Invalidate cache
	resolver.InvalidateCache(server.URL)

	// Second call - should hit server again
	_, _ = resolver.Resolve(context.Background(), server.URL)
	if callCount != 2 {
		t.Errorf("expected 2 calls after cache invalidation, got %d", callCount)
	}
}

func TestResolver_BuildAgentCardURL(t *testing.T) {
	resolver := NewResolver()

	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{
			input:    "https://example.com",
			expected: "https://example.com/.well-known/agent-card.json",
			wantErr:  false,
		},
		{
			input:    "https://example.com/",
			expected: "https://example.com/.well-known/agent-card.json",
			wantErr:  false,
		},
		{
			input:    "https://example.com/agents/legal",
			expected: "https://example.com/agents/legal/.well-known/agent-card.json",
			wantErr:  false,
		},
		{
			input:    "example.com",
			expected: "https://example.com/.well-known/agent-card.json",
			wantErr:  false,
		},
		{
			input:    "https://example.com/.well-known/agent-card.json",
			expected: "https://example.com/.well-known/agent-card.json",
			wantErr:  false,
		},
		{
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := resolver.buildAgentCardURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildAgentCardURL(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("buildAgentCardURL(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
