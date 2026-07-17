package app

import (
	"path/filepath"
	"testing"
)

func TestLoadPackagedSettings(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	settings, _, err := LoadSettings(repo, filepath.Join(repo, "openwrt", "files", "hb.env"))
	if err != nil {
		t.Fatal(err)
	}
	if settings.OutputFormat != "m3u" || settings.Mode != "auto" || settings.R2HIGMPPath != "udp" || !settings.LocalLogoCache || settings.CatchupPlayseek != "{(b)YmdHMS}-{(e)YmdHMS}" {
		t.Fatalf("packaged config mismatch: %#v", settings)
	}
}
