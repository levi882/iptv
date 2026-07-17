package source

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReaderCachesAndFallsBackToStale(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		fmt.Fprint(w, "payload")
	}))
	reader := Reader{CacheDir: t.TempDir(), UseCache: true, TTL: time.Hour}
	for i := range 2 {
		data, err := reader.Read(context.Background(), server.URL)
		if err != nil || string(data) != "payload" {
			t.Fatalf("read %d: data=%q err=%v", i, data, err)
		}
	}
	if requests != 1 {
		t.Fatalf("requests=%d, want 1", requests)
	}
	server.Close()
	reader.TTL = time.Nanosecond
	time.Sleep(time.Millisecond)
	data, err := reader.Read(context.Background(), server.URL)
	if err != nil || string(data) != "payload" {
		t.Fatalf("stale fallback: data=%q err=%v", data, err)
	}
}
