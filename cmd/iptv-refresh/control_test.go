package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestControlRequest(t *testing.T) {
	t.Parallel()
	refreshQueries := make(chan string, 2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			if r.Method != http.MethodGet || r.Header.Get("Authorization") != "" {
				t.Errorf("unexpected status request: method=%s authorization=%q", r.Method, r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/refresh":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer secret" || r.URL.Query().Get("iface") != "ethX.Y" {
				t.Errorf("unexpected refresh request: method=%s authorization=%q query=%q", r.Method, r.Header.Get("Authorization"), r.URL.RawQuery)
			}
			refreshQueries <- r.URL.RawQuery
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"ok":true,"msg":"started"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	body, err := controlRequest(server.Client(), server.URL, "status", "", "", false)
	if err != nil || string(body) != `{"ok":true}` {
		t.Fatalf("status response = %q, %v", body, err)
	}
	body, err = controlRequest(server.Client(), server.URL, "refresh", "secret", "ethX.Y", false)
	if err != nil || !strings.Contains(string(body), `"started"`) {
		t.Fatalf("refresh response = %q, %v", body, err)
	}
	if query := <-refreshQueries; strings.Contains(query, "capture=") {
		t.Fatalf("normal refresh unexpectedly captured credentials: %q", query)
	}
	body, err = controlRequest(server.Client(), server.URL, "refresh", "secret", "ethX.Y", true)
	if err != nil || !strings.Contains(string(body), `"started"`) {
		t.Fatalf("capture refresh response = %q, %v", body, err)
	}
	if query := <-refreshQueries; !strings.Contains(query, "capture=1") {
		t.Fatalf("capture refresh query = %q", query)
	}
}

func TestControlRequestRejectsUnknownActionAndHTTPError(t *testing.T) {
	t.Parallel()
	if _, err := controlRequest(nil, "http://127.0.0.1", "invalid", "", "", false); err == nil {
		t.Fatal("unknown action was accepted")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "denied", http.StatusForbidden)
	}))
	t.Cleanup(server.Close)
	if _, err := controlRequest(server.Client(), server.URL, "playlist", "secret", "", false); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("HTTP error = %v", err)
	}
}
