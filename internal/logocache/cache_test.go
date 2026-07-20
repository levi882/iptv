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

func TestRewriteDownloadsConcurrently(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-" + r.URL.Path))
	}))
	defer server.Close()
	dir := t.TempDir()
	playlistPath := filepath.Join(dir, "playlist.m3u")
	var content strings.Builder
	content.WriteString("#EXTM3U\n")
	const logos = 8
	for i := 0; i < logos; i++ {
		fmt.Fprintf(&content, "#EXTINF:-1 tvg-logo=\"%s/logo-%d\",Channel %d\nhttp://stream/%d\n", server.URL, i, i, i)
	}
	if err := os.WriteFile(playlistPath, []byte(content.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	result, err := Rewrite(context.Background(), playlistPath, filepath.Join(dir, "logos"), "http://router/iptv_logo", 5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if result.Downloaded != logos || result.Failed != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	// Serial downloads would need logos*50ms; allow generous scheduling slack
	// while still proving overlap.
	if serial := logos * 50 * time.Millisecond; elapsed >= serial {
		t.Fatalf("downloads did not overlap: %v elapsed for %d logos", elapsed, logos)
	}
	raw, _ := os.ReadFile(playlistPath)
	if strings.Contains(string(raw), server.URL) {
		t.Fatalf("playlist still references the origin server: %s", raw)
	}
}

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
