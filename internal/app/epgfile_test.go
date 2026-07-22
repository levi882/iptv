package app

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"
)

func TestEPGBytesForPathCompressesPlainXMLForGzipPath(t *testing.T) {
	raw := []byte(`<?xml version="1.0"?><tv></tv>`)
	got, err := epgBytesForPath(raw, "/www/iptv_epg/e1.xml.gz")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 2 || got[0] != 0x1f || got[1] != 0x8b {
		t.Fatalf("EPG is not gzip data: %x", got[:min(len(got), 8)])
	}
	reader, err := gzip.NewReader(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decompressed, raw) {
		t.Fatalf("decompressed EPG = %q, want %q", decompressed, raw)
	}
}

func TestEPGBytesForPathDecompressesGzipForXMLPath(t *testing.T) {
	raw := []byte(`<?xml version="1.0"?><tv></tv>`)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := epgBytesForPath(compressed.Bytes(), "/www/iptv_epg/e1.xml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("plain EPG = %q, want %q", got, raw)
	}
}

func TestEPGBytesForPathPreservesMatchingEncoding(t *testing.T) {
	raw := []byte(`<tv></tv>`)
	got, err := epgBytesForPath(raw, "/www/iptv_epg/e1.xml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("EPG = %q, want %q", got, raw)
	}
}
