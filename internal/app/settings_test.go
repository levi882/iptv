package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPackagedSettings(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	settings, _, err := LoadSettings(repo, filepath.Join(repo, "openwrt", "files", "provider.env"))
	if err != nil {
		t.Fatal(err)
	}
	if settings.OutputFormat != "m3u" || settings.Mode != "auto" || settings.R2HIGMPPath != "udp" || !settings.LocalLogoCache || settings.CatchupPlayseek != "{(b)YmdHMS}-{(e)YmdHMS}" || settings.CaptureDump != "" || settings.RefreshTimeout.Seconds() != 300 || settings.STBType != "auto" || settings.UserAgent != "auto" || settings.EPGURL != "http://epg.51zmt.top:8000/e.xml.gz" || settings.R2HBaseURL != "auto" || settings.XTvgURL != "auto" || settings.LocalLogoURLBase != "auto" {
		t.Fatalf("packaged config mismatch: %#v", settings)
	}
}

func TestLegacyEPGDefaultIsCorrected(t *testing.T) {
	const legacy = "http://epg.51zmt.top:8000/e1.xml.gz"
	if got := normalizeEPGURL(legacy); got != "http://epg.51zmt.top:8000/e.xml.gz" {
		t.Fatalf("legacy EPG URL = %q", got)
	}
	const custom = "https://example.test/e1.xml.gz"
	if got := normalizeEPGURL(custom); got != custom {
		t.Fatalf("custom EPG URL changed to %q", got)
	}
}

func TestProviderBindInterfaceModes(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		want     string
		explicit bool
	}{
		{name: "unset", want: "eth-test"},
		{name: "auto", value: "auto", want: "eth-test"},
		{name: "none", value: "none", explicit: true},
		{name: "off", value: "off", explicit: true},
		{name: "specific", value: "eth-provider", want: "eth-provider", explicit: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			envPath := filepath.Join(dir, "provider.env")
			content := "IFACE=eth-test\nPROVIDER_BIND_INTERFACE=" + test.value + "\n"
			if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}

			settings, _, err := LoadSettings(dir, envPath)
			if err != nil {
				t.Fatal(err)
			}
			if settings.BindInterface != test.want || settings.BindInterfaceExplicit != test.explicit {
				t.Fatalf("bind interface = %q explicit=%v, want %q explicit=%v", settings.BindInterface, settings.BindInterfaceExplicit, test.want, test.explicit)
			}
		})
	}
}
