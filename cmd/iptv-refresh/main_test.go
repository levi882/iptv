package main

import (
	"os"
	"path/filepath"
	"testing"

	"iptv/internal/app"
)

func TestLoadWithInterfaceOverride(t *testing.T) {
	tests := []struct {
		name     string
		bind     string
		wantBind string
	}{
		{name: "automatic", bind: "auto", wantBind: "eth-uci"},
		{name: "routing table", bind: "none", wantBind: ""},
		{name: "specific", bind: "eth-provider", wantBind: "eth-provider"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			envPath := filepath.Join(dir, "provider.env")
			content := "IFACE=eth-env\nPROVIDER_BIND_INTERFACE=" + test.bind + "\n"
			if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}

			settings, err := loadWithOverrides(dir, envPath, "", "eth-uci")
			if err != nil {
				t.Fatal(err)
			}
			if settings.Interface != "eth-uci" || settings.BindInterface != test.wantBind {
				t.Fatalf("interfaces = capture %q bind %q, want capture eth-uci bind %q", settings.Interface, settings.BindInterface, test.wantBind)
			}
		})
	}
}

func TestApplyProviderInterfaceOverride(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		explicit bool
		initial  string
		want     string
		wantMode bool
	}{
		{name: "automatic", value: "auto", initial: "capture0", want: "capture0"},
		{name: "routing table", value: "none", initial: "capture0", wantMode: true},
		{name: "specific", value: "pppoe-iptv", initial: "capture0", want: "pppoe-iptv", wantMode: true},
		{name: "environment wins", value: "pppoe-uci", explicit: true, initial: "pppoe-env", want: "pppoe-env", wantMode: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings := app.Settings{Interface: "capture0", BindInterface: test.initial, BindInterfaceExplicit: test.explicit}
			applyProviderInterfaceOverride(&settings, test.value)
			if settings.BindInterface != test.want || settings.BindInterfaceExplicit != test.wantMode {
				t.Fatalf("provider interface = %q explicit=%v, want %q explicit=%v", settings.BindInterface, settings.BindInterfaceExplicit, test.want, test.wantMode)
			}
		})
	}
}
