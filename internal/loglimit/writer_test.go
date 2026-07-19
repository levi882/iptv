package loglimit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSize(t *testing.T) {
	tests := map[string]int64{
		"1k": 1 << 10, "2KB": 2 << 10,
		"3m": 3 << 20, "4MB": 4 << 20,
		"100MB": 100 << 20,
	}
	for value, want := range tests {
		got, err := ParseSize(value)
		if err != nil || got != want {
			t.Fatalf("ParseSize(%q) = %d, %v; want %d", value, got, err, want)
		}
	}
	for _, value := range []string{"", "1", "0M", "101M", "1G", "1GB", "1T", "abc"} {
		if _, err := ParseSize(value); err == nil {
			t.Fatalf("ParseSize(%q) unexpectedly succeeded", value)
		}
	}
}

func TestWriterKeepsNewestDataWithinSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "iptv-refresh.log")
	writer, err := New(path, MinBytes)
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 100; index++ {
		if _, err := fmt.Fprintf(writer, "line-%03d %s\n", index, strings.Repeat("x", 24)); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(raw)) > MinBytes || strings.Contains(string(raw), "line-001") || !strings.Contains(string(raw), "line-100") {
		t.Fatalf("unexpected bounded log size/content: size=%d content=%q", len(raw), raw)
	}
}

func TestWriterHonorsExternalClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "iptv-refresh.log")
	writer, err := New(path, MinBytes)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = writer.Write([]byte("old-line\n"))
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, _ = writer.Write([]byte("new-line\n"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "new-line\n" {
		t.Fatalf("cleared log was resurrected: %q", raw)
	}
}
