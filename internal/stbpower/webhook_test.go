package stbpower

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebhookTrigger(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload["action"] != "power_on_for_credential_capture" || payload["source"] != "iptv-refresh" {
			http.Error(w, "unexpected payload", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := (Webhook{URL: server.URL, Timeout: time.Second}).Trigger(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWebhookRejectsUnsafeAndFailedRequests(t *testing.T) {
	for _, value := range []string{"", "file:///tmp/hook", "http://user:pass@example.invalid/hook"} {
		if err := (Webhook{URL: value}).Trigger(context.Background()); err == nil {
			t.Fatalf("unsafe webhook URL %q was accepted", value)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "failed", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	err := (Webhook{URL: server.URL}).Trigger(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("unexpected failed webhook result: %v", err)
	}

	secretServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	secretURL := secretServer.URL + "/api/webhook/secret-webhook-id"
	secretServer.Close()
	err = (Webhook{URL: secretURL, Timeout: 100 * time.Millisecond}).Trigger(context.Background())
	if err == nil || strings.Contains(err.Error(), "secret-webhook-id") {
		t.Fatalf("connection error exposed webhook ID: %v", err)
	}
}
