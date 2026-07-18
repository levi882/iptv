package redact

import (
	"strings"
	"testing"
)

func TestSensitive(t *testing.T) {
	input := `Get "http://provider/iptvepg/function/index.jsp?UserToken=secret-token&UserID=user-1&STBID=stb-1": timeout
HB_AUTHENTICATOR=abcdef
CTCSetConfig('UserToken','second-secret')`
	got := Sensitive(input)
	for _, secret := range []string{"secret-token", "user-1", "stb-1", "abcdef", "second-secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted output contains %q: %s", secret, got)
		}
	}
	if !strings.Contains(got, "provider") || !strings.Contains(got, "[redacted]") {
		t.Fatalf("redacted output lost useful context: %s", got)
	}
}
