package events

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewPublisher(t *testing.T) {
	pub := NewPublisher("test-service")

	if pub == nil {
		t.Fatal("NewPublisher() returned nil")
	}

	if pub.source != "test-service" {
		t.Errorf("NewPublisher() source = %v, want test-service", pub.source)
	}

	if pub.httpClient == nil {
		t.Error("NewPublisher() did not initialize httpClient")
	}

	if pub.endpoints == nil {
		t.Error("NewPublisher() did not initialize endpoints map")
	}
}

func TestPublish_NoWebhook(t *testing.T) {
	pub := NewPublisher("test-service")
	ctx := context.Background()

	data := map[string]any{
		"work_id":     "work_123",
		"consumer_id": "tenant_001",
		"category":    "general",
	}

	// Should not error even without webhook registered
	err := pub.Publish(ctx, EventWorkSubmitted, data)
	if err != nil {
		t.Errorf("Publish() without webhook error: %v", err)
	}
}

func TestPublish_WithWebhook(t *testing.T) {
	receivedEvent := false
	var receivedEnvelope Envelope

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedEvent = true

		// Verify headers
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Missing Content-Type header")
		}
		if r.Header.Get("X-Event-Type") == "" {
			t.Errorf("Missing X-Event-Type header")
		}

		// Parse body
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedEnvelope)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pub := NewPublisher("test-service")
	pub.RegisterEndpoint(EventWorkSubmitted, server.URL)

	ctx := context.Background()
	data := map[string]any{
		"work_id":     "work_123",
		"consumer_id": "tenant_001",
	}

	err := pub.Publish(ctx, EventWorkSubmitted, data)
	if err != nil {
		t.Fatalf("Publish() with webhook error: %v", err)
	}

	if !receivedEvent {
		t.Error("Webhook was not called")
	}

	if receivedEnvelope.EventType != EventWorkSubmitted {
		t.Errorf("Envelope EventType = %v, want %v", receivedEnvelope.EventType, EventWorkSubmitted)
	}

	if receivedEnvelope.Source != "test-service" {
		t.Errorf("Envelope Source = %v, want test-service", receivedEnvelope.Source)
	}

	if receivedEnvelope.Data["work_id"] != "work_123" {
		t.Errorf("Envelope Data work_id = %v, want work_123", receivedEnvelope.Data["work_id"])
	}
}

func TestPublish_WebhookFailure(t *testing.T) {
	// Create test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	pub := NewPublisher("test-service")
	pub.RegisterEndpoint(EventWorkSubmitted, server.URL)

	ctx := context.Background()
	data := map[string]any{
		"work_id": "work_123",
	}

	// Should not error even if webhook fails (logged only)
	err := pub.Publish(ctx, EventWorkSubmitted, data)
	if err != nil {
		t.Errorf("Publish() should not error on webhook failure, got: %v", err)
	}
}

func TestRegisterEndpoint(t *testing.T) {
	pub := NewPublisher("test-service")

	pub.RegisterEndpoint(EventWorkSubmitted, "http://example.com/webhook")

	if pub.endpoints[EventWorkSubmitted] != "http://example.com/webhook" {
		t.Errorf("RegisterEndpoint() did not register endpoint correctly")
	}
}

func TestPublish_AllEventTypes(t *testing.T) {
	eventTypes := []string{
		EventWorkSubmitted,
		EventWorkBidWindowClosed,
		EventWorkCancelled,
		EventBidSubmitted,
		EventBidsEvaluated,
		EventContractAwarded,
		EventContractCompleted,
		EventContractFailed,
		EventSettlementCompleted,
	}

	pub := NewPublisher("test-service")
	ctx := context.Background()

	for _, eventType := range eventTypes {
		t.Run(eventType, func(t *testing.T) {
			data := map[string]any{
				"test_key": "test_value",
			}

			err := pub.Publish(ctx, eventType, data)
			if err != nil {
				t.Errorf("Publish(%s) error: %v", eventType, err)
			}
		})
	}
}

func TestEnvelope_TenantID(t *testing.T) {
	var receivedEnvelope Envelope

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedEnvelope)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pub := NewPublisher("test-service")
	pub.RegisterEndpoint(EventWorkSubmitted, server.URL)

	ctx := context.Background()
	data := map[string]any{
		"work_id":   "work_123",
		"tenant_id": "tenant_001",
	}

	err := pub.Publish(ctx, EventWorkSubmitted, data)
	if err != nil {
		t.Fatalf("Publish() error: %v", err)
	}

	if receivedEnvelope.TenantID != "tenant_001" {
		t.Errorf("Envelope TenantID = %v, want tenant_001", receivedEnvelope.TenantID)
	}
}

func TestEnvelope_Structure(t *testing.T) {
	var receivedEnvelope Envelope

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedEnvelope)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pub := NewPublisher("test-service")
	pub.RegisterEndpoint(EventWorkSubmitted, server.URL)

	ctx := context.Background()
	data := map[string]any{
		"work_id": "work_123",
	}

	err := pub.Publish(ctx, EventWorkSubmitted, data)
	if err != nil {
		t.Fatalf("Publish() error: %v", err)
	}

	// Verify envelope structure
	if receivedEnvelope.EventID == "" {
		t.Error("Envelope EventID is empty")
	}

	if receivedEnvelope.EventType != EventWorkSubmitted {
		t.Errorf("Envelope EventType = %v, want %v", receivedEnvelope.EventType, EventWorkSubmitted)
	}

	if receivedEnvelope.SchemaVersion != "1.0" {
		t.Errorf("Envelope SchemaVersion = %v, want 1.0", receivedEnvelope.SchemaVersion)
	}

	if receivedEnvelope.Source != "test-service" {
		t.Errorf("Envelope Source = %v, want test-service", receivedEnvelope.Source)
	}

	if receivedEnvelope.Timestamp.IsZero() {
		t.Error("Envelope Timestamp is zero")
	}

	if receivedEnvelope.IdempotencyKey == "" {
		t.Error("Envelope IdempotencyKey is empty")
	}

	if receivedEnvelope.Data == nil {
		t.Error("Envelope Data is nil")
	}
}

func TestGenerateEventID(t *testing.T) {
	// Generate multiple IDs and verify they're unique
	ids := make(map[string]bool)

	for i := 0; i < 100; i++ {
		id := generateEventID()

		if id == "" {
			t.Error("generateEventID() returned empty string")
		}

		if len(id) < 5 {
			t.Errorf("generateEventID() returned short ID: %v", id)
		}

		if ids[id] {
			t.Errorf("generateEventID() generated duplicate ID: %v", id)
		}

		ids[id] = true
	}
}
