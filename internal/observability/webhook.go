package observability

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/config"
)

// WebhookConfig is the subset of config needed by the webhook dispatcher.
type WebhookConfig struct {
	Enabled bool
	URLs    []string
	Secret  string
	Events  []string
}

// WebhookDispatcher delivers events to configured HTTP endpoints.
type WebhookDispatcher struct {
	urls    []string
	secret  string
	events  map[string]bool
	all     bool
	client  *http.Client
}

// NewWebhookDispatcher creates a dispatcher for the given config.
func NewWebhookDispatcher(cfg config.WebhookConfig) *WebhookDispatcher {
	all := false
	events := make(map[string]bool)
	for _, e := range cfg.Events {
		if e == "*" {
			all = true
			continue
		}
		events[e] = true
	}
	if len(events) == 0 && !all {
		all = true
	}
	return &WebhookDispatcher{
		urls:   cfg.URLs,
		secret: cfg.Secret,
		events: events,
		all:    all,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Handle implements EventHandler.
func (w *WebhookDispatcher) Handle(event Event) {
	if !w.all && !w.events[event.Type] {
		return
	}
	if len(w.urls) == 0 {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"id":        newEventID(event),
		"type":      event.Type,
		"timestamp": event.Timestamp.UTC().Format(time.RFC3339),
		"payload":   event.Payload,
	})
	if err != nil {
		return
	}
	for _, url := range w.urls {
		go w.deliver(url, event.Type, payload)
	}
}

func (w *WebhookDispatcher) deliver(url, eventType string, payload []byte) {
	backoff := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	var lastErr error
	for attempt := 0; attempt <= len(backoff); attempt++ {
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			lastErr = err
			break
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-BPMN-Event-Type", eventType)
		req.Header.Set("X-BPMN-Event-Id", newEventIDFromPayload(payload))
		if w.secret != "" {
			mac := hmac.New(sha256.New, []byte(w.secret))
			mac.Write(payload)
			req.Header.Set("X-BPMN-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		}
		resp, err := w.client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return
			}
			lastErr = fmt.Errorf("http %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		if attempt < len(backoff) {
			time.Sleep(backoff[attempt])
		}
	}
	// Dead letter: log only (no DB table to keep scope small).
	fmt.Printf("webhook delivery failed: url=%s event=%s error=%v\n", url, eventType, lastErr)
}

func newEventID(event Event) string {
	return fmt.Sprintf("%s-%d", event.Type, event.Timestamp.UnixNano())
}

func newEventIDFromPayload(payload []byte) string {
	// Best effort: parse id back.
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err == nil {
		if id, ok := m["id"].(string); ok {
			return id
		}
	}
	return ""
}
