package playlist

import (
	"bytes"
	"compress/gzip"
	"testing"
	"time"
)

func TestInspectEPGCoverage(t *testing.T) {
	raw := []byte(`<?xml version="1.0"?><tv>
<programme start="20260722180000 +0800" stop="20260722183000 +0800" channel="one"/>
<programme start="20260723010000 +0800" channel="one"/>
</tv>`)
	coverage, err := InspectEPGCoverage(raw, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 7, 23, 1, 0, 0, 0, time.FixedZone("+0800", 8*60*60))
	if coverage.Programmes != 2 || !coverage.Latest.Equal(want) {
		t.Fatalf("coverage = %#v, want programmes=2 latest=%s", coverage, want)
	}
}

func TestInspectEPGCoverageReadsGzipAndUsesDefaultLocation(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	raw := []byte(`<tv><programme start="202607221800" stop="202607221900" channel="one"/></tv>`)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	coverage, err := InspectEPGCoverage(compressed.Bytes(), location)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 7, 22, 19, 0, 0, 0, location)
	if coverage.Programmes != 1 || !coverage.Latest.Equal(want) {
		t.Fatalf("coverage = %#v, want programmes=1 latest=%s", coverage, want)
	}
}

func TestInspectEPGCoverageRejectsUnusableXMLTV(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(`<tv></tv>`),
		[]byte(`<tv><programme start="invalid"/></tv>`),
		[]byte(`<tv><programme>`),
	} {
		if _, err := InspectEPGCoverage(raw, time.UTC); err == nil {
			t.Fatalf("InspectEPGCoverage(%q) succeeded", raw)
		}
	}
}
