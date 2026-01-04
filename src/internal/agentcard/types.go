package agentcard

import "time"

// AgentCard represents the A2A Agent Card structure
// Based on https://google.github.io/A2A/specification/
type AgentCard struct {
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	URL               string            `json:"url"`
	Provider          *Provider         `json:"provider,omitempty"`
	Version           string            `json:"version"`
	DocumentationURL  string            `json:"documentationUrl,omitempty"`
	Capabilities      Capabilities      `json:"capabilities"`
	Authentication    *Authentication   `json:"authentication,omitempty"`
	DefaultInputModes []string          `json:"defaultInputModes,omitempty"`
	DefaultOutputModes []string         `json:"defaultOutputModes,omitempty"`
	Skills            []Skill           `json:"skills"`
}

// Provider represents agent provider information
type Provider struct {
	Organization string `json:"organization"`
	URL          string `json:"url,omitempty"`
}

// Capabilities represents agent capabilities
type Capabilities struct {
	Streaming              bool `json:"streaming,omitempty"`
	PushNotifications      bool `json:"pushNotifications,omitempty"`
	StateTransitionHistory bool `json:"stateTransitionHistory,omitempty"`
}

// Authentication represents authentication requirements
type Authentication struct {
	Schemes []string `json:"schemes"`
	Credentials string `json:"credentials,omitempty"`
}

// Skill represents an agent skill/capability
type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Examples    []string `json:"examples,omitempty"`
	InputModes  []string `json:"inputModes,omitempty"`
	OutputModes []string `json:"outputModes,omitempty"`
}

// ResolvedAgentCard is the internal representation with additional metadata
type ResolvedAgentCard struct {
	AgentCard
	SourceURL   string    `json:"source_url"`
	ResolvedAt  time.Time `json:"resolved_at"`
	ValidUntil  time.Time `json:"valid_until"`
	ProviderID  string    `json:"provider_id,omitempty"`
}

// SkillIndex represents searchable skill information
type SkillIndex struct {
	SkillID     string   `json:"skill_id"`
	SkillName   string   `json:"skill_name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	ProviderID  string   `json:"provider_id"`
	AgentName   string   `json:"agent_name"`
	AgentURL    string   `json:"agent_url"`
	A2AEndpoint string   `json:"a2a_endpoint"`
}
