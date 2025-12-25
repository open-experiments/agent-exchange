package model

import "time"

type TenantType string

const (
	TenantTypeRequestor TenantType = "REQUESTOR"
	TenantTypeProvider  TenantType = "PROVIDER"
	TenantTypeBoth      TenantType = "BOTH"
)

type TenantStatus string

const (
	TenantStatusPending    TenantStatus = "PENDING"
	TenantStatusActive     TenantStatus = "ACTIVE"
	TenantStatusSuspended  TenantStatus = "SUSPENDED"
	TenantStatusTerminated TenantStatus = "TERMINATED"
)

type Quotas struct {
	RequestsPerMinute   int   `json:"requests_per_minute" bson:"requests_per_minute"`
	RequestsPerDay      int   `json:"requests_per_day" bson:"requests_per_day"`
	MaxAgents           int   `json:"max_agents" bson:"max_agents"`
	MaxConcurrentTasks  int   `json:"max_concurrent_tasks" bson:"max_concurrent_tasks"`
	MaxTaskPayloadBytes int64 `json:"max_task_payload_bytes" bson:"max_task_payload_bytes"`
}

type Tenant struct {
	ID               string         `json:"id" bson:"id"`
	ExternalID       string         `json:"external_id" bson:"external_id"`
	Name             string         `json:"name" bson:"name"`
	Type             TenantType     `json:"type" bson:"type"`
	Status           TenantStatus   `json:"status" bson:"status"`
	ContactEmail     string         `json:"contact_email" bson:"contact_email"`
	BillingEmail     string         `json:"billing_email" bson:"billing_email"`
	Quotas           Quotas         `json:"quotas" bson:"quotas"`
	Metadata         map[string]any `json:"metadata" bson:"metadata"`
	CreatedAt        time.Time      `json:"created_at" bson:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at" bson:"updated_at"`
	SuspendedAt      *time.Time     `json:"suspended_at,omitempty" bson:"suspended_at,omitempty"`
	SuspensionReason *string        `json:"suspension_reason,omitempty" bson:"suspension_reason,omitempty"`
}

type APIKeyStatus string

const (
	APIKeyStatusActive  APIKeyStatus = "ACTIVE"
	APIKeyStatusRevoked APIKeyStatus = "REVOKED"
	APIKeyStatusExpired APIKeyStatus = "EXPIRED"
)

type APIKey struct {
	ID         string       `json:"id" bson:"id"`
	TenantID   string       `json:"tenant_id" bson:"tenant_id"`
	Name       string       `json:"name" bson:"name"`
	KeyHash    string       `json:"-" bson:"key_hash"`
	Prefix     string       `json:"prefix" bson:"prefix"`
	Scopes     []string     `json:"scopes" bson:"scopes"`
	Status     APIKeyStatus `json:"status" bson:"status"`
	CreatedAt  time.Time    `json:"created_at" bson:"created_at"`
	ExpiresAt  *time.Time   `json:"expires_at,omitempty" bson:"expires_at,omitempty"`
	LastUsedAt *time.Time   `json:"last_used_at,omitempty" bson:"last_used_at,omitempty"`
	RevokedAt  *time.Time   `json:"revoked_at,omitempty" bson:"revoked_at,omitempty"`
}

type CreateTenantRequest struct {
	Name         string         `json:"name"`
	Type         TenantType     `json:"type"`
	ContactEmail string         `json:"contact_email"`
	BillingEmail string         `json:"billing_email"`
	Metadata     map[string]any `json:"metadata"`
}

type CreateTenantResponse struct {
	ID         string       `json:"id"`
	ExternalID string       `json:"external_id"`
	Name       string       `json:"name"`
	Type       TenantType   `json:"type"`
	Status     TenantStatus `json:"status"`
	CreatedAt  time.Time    `json:"created_at"`
	APIKey     struct {
		ID     string `json:"id"`
		Key    string `json:"key"`
		Prefix string `json:"prefix"`
	} `json:"api_key"`
	Quotas Quotas `json:"quotas"`
}

type CreateAPIKeyRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type CreateAPIKeyResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Key        string     `json:"key"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type ValidateAPIKeyRequest struct {
	APIKey string `json:"api_key"`
}

type ValidateAPIKeyResponse struct {
	TenantID     string       `json:"tenant_id"`
	TenantStatus TenantStatus `json:"tenant_status"`
	Scopes       []string     `json:"scopes"`
	Quotas       Quotas       `json:"quotas"`
}

