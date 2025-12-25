package events

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Publisher handles event publishing via HTTP (Phase A) or Pub/Sub (future)
type Publisher struct {
	source     string
	httpClient *http.Client
	endpoints  map[string]string // eventType -> webhook URL
}

// NewPublisher creates a new event publisher
func NewPublisher(source string) *Publisher {
	return &Publisher{
		source: source,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		endpoints: make(map[string]string),
	}
}

// RegisterEndpoint registers a webhook endpoint for an event type
func (p *Publisher) RegisterEndpoint(eventType, webhookURL string) {
	p.endpoints[eventType] = webhookURL
}

// Publish publishes an event (HTTP webhook for now, Pub/Sub later)
func (p *Publisher) Publish(ctx context.Context, eventType string, data map[string]any) error {
	envelope := Envelope{
		EventID:        generateEventID(),
		EventType:      eventType,
		SchemaVersion:  "1.0",
		IdempotencyKey: fmt.Sprintf("%s_%s_%d", eventType, data["work_id"], time.Now().Unix()),
		Timestamp:      time.Now().UTC(),
		Source:         p.source,
		Data:           data,
	}

	if tenantID, ok := data["tenant_id"].(string); ok {
		envelope.TenantID = tenantID
	}

	// For now, just log the event (HTTP webhooks can be added later)
	slog.InfoContext(ctx, "event_published",
		"event_id", envelope.EventID,
		"event_type", envelope.EventType,
		"source", envelope.Source,
	)

	// If webhook endpoint registered, send HTTP POST
	if webhookURL, ok := p.endpoints[eventType]; ok {
		return p.sendWebhook(ctx, webhookURL, envelope)
	}

	// In the future, this will publish to Pub/Sub
	return nil
}

func (p *Publisher) sendWebhook(ctx context.Context, url string, envelope Envelope) error {
	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-ID", envelope.EventID)
	req.Header.Set("X-Event-Type", envelope.EventType)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		slog.WarnContext(ctx, "webhook_failed",
			"url", url,
			"event_type", envelope.EventType,
			"error", err,
		)
		return nil // Don't fail on webhook errors
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		slog.WarnContext(ctx, "webhook_error",
			"url", url,
			"event_type", envelope.EventType,
			"status", resp.StatusCode,
		)
	}

	return nil
}

func generateEventID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "evt_" + hex.EncodeToString(b[:])
}
