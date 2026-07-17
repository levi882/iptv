package logocache

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRewriteDownloadsAndReusesLogo(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-data"))
	}))
	defer server.Close()
	dir := t.TempDir()
	playlistPath := filepath.Join(dir, "playlist.m3u")
	logoDir := filepath.Join(dir, "logos")
	content := fmt.Sprintf("#EXTM3U\n#EXTINF:-1 tvg-logo=\"%s/logo\",One\nhttp://stream\n", server.URL)
	if err := os.WriteFile(playlistPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Rewrite(context.Background(), playlistPath, logoDir, "http://router/iptv_logo", 2*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Downloaded != 1 || result.Reused != 0 || result.Failed != 0 || requests != 1 {
		t.Fatalf("unexpected first result: %#v requests=%d", result, requests)
	}
	raw, _ := os.ReadFile(playlistPath)
	if !strings.Contains(string(raw), `tvg-logo="http://router/iptv_logo/`) || !strings.Contains(string(raw), `.png"`) {
		t.Fatalf("playlist was not rewritten: %s", raw)
	}
	// Restore the external URL and ensure the cached file is reused.
	if err := os.WriteFile(playlistPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err = Rewrite(context.Background(), playlistPath, logoDir, "http://router/iptv_logo", 2*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Reused != 1 || result.Downloaded != 0 || requests != 1 {
		t.Fatalf("unexpected reuse result: %#v requests=%d", result, requests)
	}
}
