package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iptv/internal/app"
)

func TestHandlerAuthAndHealth(t *testing.T) {
	manager := NewManager(app.Runner{}, app.Settings{})
	handler := Handler(Config{Token: "secret", AllowedIPs: map[string]bool{"192.0.2.1": true}, Manager: manager})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("health status = %d", recorder.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body["ok"] != true {
		t.Fatalf("health body = %s", recorder.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/refresh?token=wrong", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("refresh status = %d", recorder.Code)
	}
}

func TestPlaylistRequiresTokenAndServesM3U(t *testing.T) {
	path := filepath.Join(t.TempDir(), "playlist.m3u")
	if err := os.WriteFile(path, []byte("#EXTM3U\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(app.Runner{}, app.Settings{})
	handler := Handler(Config{Token: "secret", AllowedIPs: map[string]bool{"192.0.2.1": true}, PlaylistPath: path, Manager: manager})
	req := httptest.NewRequest(http.MethodGet, "/playlist", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	req.Header.Set("Authorization", "Bearer secret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "#EXTM3U") {
		t.Fatalf("playlist response: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}
