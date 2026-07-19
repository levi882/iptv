package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.env")
	content := "# comment\nexport A=plain\nB=\"hello world\"\nC='{(b)YmdHMS}-{(e)YmdHMS}'\nFLAG=1\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	env, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if env["A"] != "plain" || env["B"] != "hello world" || env["C"] != "{(b)YmdHMS}-{(e)YmdHMS}" || !env.Bool("FLAG", false) {
		t.Fatalf("unexpected env: %#v", env)
	}
}

func TestNormalizeProviderKeys(t *testing.T) {
	env := Env{
		"HB_USER_ID":       "legacy-user",
		"HB_EPG_ENTRY":     "http://legacy.invalid",
		"PROVIDER_USER_ID": "current-user",
	}.NormalizeProviderKeys()
	if env["PROVIDER_USER_ID"] != "current-user" {
		t.Fatalf("canonical value was overwritten: %#v", env)
	}
	if env["PROVIDER_EPG_ENTRY"] != "http://legacy.invalid" {
		t.Fatalf("legacy value was not normalized: %#v", env)
	}
}
