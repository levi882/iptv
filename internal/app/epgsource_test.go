package app

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestSelectEPGSourceKeepsFreshPrimary(t *testing.T) {
	now := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)
	reads := 0
	selected, err := selectEPGSource(context.Background(), []string{"primary", "fallback"}, now, time.UTC, func(_ context.Context, source string) ([]byte, error) {
		reads++
		return testXMLTV("20260722190000 +0000"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected.URL != "primary" || !selected.Fresh || reads != 1 {
		t.Fatalf("selected=%#v reads=%d", selected, reads)
	}
}

func TestSelectEPGSourceUsesFreshFallbackForExpiredPrimary(t *testing.T) {
	now := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)
	selected, err := selectEPGSource(context.Background(), []string{"primary", "fallback"}, now, time.UTC, func(_ context.Context, source string) ([]byte, error) {
		if source == "primary" {
			return testXMLTV("20260721235900 +0000"), nil
		}
		return testXMLTV("20260723190000 +0000"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected.URL != "fallback" || !selected.Fresh || len(selected.Notes) != 1 {
		t.Fatalf("selected=%#v", selected)
	}
}

func TestSelectEPGSourceUsesFreshestStaleGuide(t *testing.T) {
	now := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)
	selected, err := selectEPGSource(context.Background(), []string{"older", "newer"}, now, time.UTC, func(_ context.Context, source string) ([]byte, error) {
		if source == "older" {
			return testXMLTV("20260720190000 +0000"), nil
		}
		return testXMLTV("20260721190000 +0000"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected.URL != "newer" || selected.Fresh {
		t.Fatalf("selected=%#v", selected)
	}
}

func TestSelectEPGSourceSkipsDownloadAndParseFailures(t *testing.T) {
	now := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)
	selected, err := selectEPGSource(context.Background(), []string{"download-error", "bad-xml", "fallback"}, now, time.UTC, func(_ context.Context, source string) ([]byte, error) {
		switch source {
		case "download-error":
			return nil, fmt.Errorf("offline")
		case "bad-xml":
			return []byte(`<tv>`), nil
		default:
			return testXMLTV("20260723190000 +0000"), nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected.URL != "fallback" {
		t.Fatalf("selected=%#v", selected)
	}
}

func testXMLTV(stop string) []byte {
	return []byte(`<tv><programme start="20260719000000 +0000" stop="` + stop + `" channel="one"/></tv>`)
}
