package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestRefreshRequiresPostAndAuthorizationHeader(t *testing.T) {
	manager := NewManager(app.Runner{}, app.Settings{})
	handler := Handler(Config{Token: "secret", Manager: manager})

	request := func(method, target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		return recorder
	}
	if recorder := request(http.MethodGet, "/refresh?token=secret"); recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET refresh status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder := request(http.MethodPost, "/refresh?token=secret"); recorder.Code != http.StatusForbidden {
		t.Fatalf("query-token refresh status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestRefreshModesAndFailedRunPreservesLastReport(t *testing.T) {
	settingsSeen := make(chan app.Settings, 2)
	manager := NewManager(app.Runner{}, app.Settings{Interface: "default", BindInterface: "default"})
	manager.run = func(_ context.Context, settings app.Settings) (app.Report, error) {
		settingsSeen <- settings
		if settings.SkipCapture {
			return app.Report{Channels: 42, OutputPath: "/tmp/playlist.m3u"}, nil
		}
		return app.Report{}, errors.New("capture timed out")
	}
	handler := Handler(Config{Token: "secret", Manager: manager})

	request := func(target string) {
		req := httptest.NewRequest(http.MethodPost, target, nil)
		req.Header.Set("Authorization", "Bearer secret")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("refresh %s status = %d, body = %s", target, recorder.Code, recorder.Body.String())
		}
	}

	request("/refresh?iface=iptv0")
	first := <-settingsSeen
	if !first.SkipCapture || first.Interface != "iptv0" || first.BindInterface != "iptv0" {
		t.Fatalf("saved-credential settings = %+v", first)
	}
	waitForManager(t, manager)
	if status := manager.Status(); status.Report == nil || status.Report.Channels != 42 || status.LastError != "" {
		t.Fatalf("successful status = %+v", status)
	}

	request("/refresh?iface=iptv1&capture=1")
	second := <-settingsSeen
	if second.SkipCapture || second.Interface != "iptv1" || second.BindInterface != "iptv1" {
		t.Fatalf("capture settings = %+v", second)
	}
	waitForManager(t, manager)
	status := manager.Status()
	if status.Report == nil || status.Report.Channels != 42 || status.LastError != "capture timed out" {
		t.Fatalf("failed status did not preserve report: %+v", status)
	}
}

func TestFailedRunRedactsStatusError(t *testing.T) {
	manager := NewManager(app.Runner{}, app.Settings{})
	manager.run = func(context.Context, app.Settings) (app.Report, error) {
		return app.Report{}, errors.New("request failed: UserToken=status-secret&UserID=user-secret")
	}
	if err := manager.Trigger(""); err != nil {
		t.Fatal(err)
	}
	waitForManager(t, manager)
	message := manager.Status().LastError
	if strings.Contains(message, "status-secret") || strings.Contains(message, "user-secret") || !strings.Contains(message, "[redacted]") {
		t.Fatalf("status error was not redacted: %s", message)
	}
}

func waitForManager(t *testing.T, manager *Manager) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for manager.Status().Running && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if manager.Status().Running {
		t.Fatal("manager did not finish")
	}
}

func TestRefreshRejectsInvalidCaptureMode(t *testing.T) {
	manager := NewManager(app.Runner{}, app.Settings{})
	handler := Handler(Config{Token: "secret", Manager: manager})
	req := httptest.NewRequest(http.MethodPost, "/refresh?capture=sometimes", nil)
	req.Header.Set("Authorization", "Bearer secret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid capture status = %d, body = %s", recorder.Code, recorder.Body.String())
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
