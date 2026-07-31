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
	if settings.OutputFormat != "m3u" || settings.Mode != "auto" || settings.R2HIGMPPath != "udp" || !settings.LocalLogoCache || settings.CatchupPlayseek != "{(b)YmdHMS}-{(e)YmdHMS}" || settings.CaptureDump != "" || settings.RefreshTimeout.Seconds() != 300 || settings.STBType != "auto" || settings.UserAgent != "auto" || settings.GuideTemplate != "frame226" || settings.GuideHistoryDays != 7 || settings.ProviderCatchupURL != "auto" || settings.LogoMatchSource != defaultLogoMatchSource || settings.R2HBaseURL != "auto" || settings.XTvgURL != "auto" || settings.LocalLogoURLBase != "auto" {
		t.Fatalf("packaged config mismatch: %#v", settings)
	}
}

func TestReleasedLogoDefaultIsMigrated(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "provider.env")
	content := "LOGO_MATCH_SOURCE=https://live.fanmingming.com/tv/m3u/index.m3u\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, _, err := LoadSettings(dir, envPath)
	if err != nil {
		t.Fatal(err)
	}
	if settings.LogoMatchSource != defaultLogoMatchSource {
		t.Fatalf("logo source = %q", settings.LogoMatchSource)
	}
}

func TestProviderGuideSettingsAreValidated(t *testing.T) {
	for _, content := range []string{
		"PROVIDER_EPG_TEMPLATE=../frame226\n",
		"PROVIDER_EPG_HISTORY_DAYS=8\n",
		"PROVIDER_CATCHUP_URL=rtsp://invalid.example.test/replay\n",
	} {
		dir := t.TempDir()
		envPath := filepath.Join(dir, "provider.env")
		if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadSettings(dir, envPath); err == nil {
			t.Fatalf("invalid provider guide settings accepted: %q", content)
		}
	}
}

func TestGitHubTokenIsLoaded(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "provider.env")
	if err := os.WriteFile(envPath, []byte("GITHUB_TOKEN=github-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, _, err := LoadSettings(dir, envPath)
	if err != nil {
		t.Fatal(err)
	}
	if settings.GitHubToken != "github-secret" {
		t.Fatalf("GitHub token was not loaded")
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
