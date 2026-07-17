package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hb.env")
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
