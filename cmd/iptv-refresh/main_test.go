package main

import (
	"os"
	"path/filepath"
	"testing"
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
			envPath := filepath.Join(dir, "hb.env")
			content := "IFACE=eth-env\nHB_BIND_INTERFACE=" + test.bind + "\n"
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
