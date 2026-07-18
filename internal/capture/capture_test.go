package capture

import (
	"strings"
	"testing"

	"iptv/internal/config"
)

func TestParse(t *testing.T) {
	raw := []byte("POST /GetUserToken UserID=user1&Authenticator=aabb&citycode=027\nHost: 5.6.7.9:4338\nGET http://1.2.3.4:8080/iptvepg/function/index.jsp?STBID=abcd&stbinfo=eeff&easip=1.2.3.4&networkid=9\nCTCSetConfig('UserToken','token-1')")
	env := Parse(raw, config.Env{}, "5.6.7.8")
	if !Complete(env) || env["HB_AUTHENTICATOR"] != "AABB" || env["HB_STBID"] != "ABCD" || env["HB_USER_TOKEN"] != "token-1" || env["HB_EPG_ENTRY"] != "http://1.2.3.4:8080" || env["HB_TOKEN_SERVER"] != "http://5.6.7.9:4338" {
		t.Fatalf("unexpected capture: %#v", env)
	}
	for key, value := range env {
		if strings.ContainsAny(value, "\r\n") {
			t.Fatalf("unsafe value %s=%q", key, value)
		}
	}
}

func TestParseTokenServerFallback(t *testing.T) {
	env := Parse(nil, config.Env{}, "5.6.7.8")
	if env["HB_TOKEN_SERVER"] != "http://5.6.7.8:4338" {
		t.Fatalf("token server = %q", env["HB_TOKEN_SERVER"])
	}
}

func TestCapturedEnoughRequiresFreshLogin(t *testing.T) {
	fallback := config.Env{"HB_USER_ID": "old", "HB_AUTHENTICATOR": "AA", "HB_STBID": "BB", "HB_STBINFO": "CC"}
	if capturedEnough(nil, fallback, "121.60.255.37") {
		t.Fatal("fallback-only credentials must not stop capture early")
	}
	if !capturedEnough([]byte("UserID=new&Authenticator=dd"), fallback, "121.60.255.37") {
		t.Fatal("fresh login plus cached device fields should stop capture")
	}
}

func TestSafeBufferRetainsBoundedTail(t *testing.T) {
	buffer := newSafeBuffer(5)
	_, _ = buffer.Write([]byte("abc"))
	_, _ = buffer.Write([]byte("def"))
	if got := string(buffer.BytesCopy()); got != "bcdef" {
		t.Fatalf("buffer tail = %q, want bcdef", got)
	}
	if !buffer.Truncated() {
		t.Fatal("buffer did not report truncation")
	}
}
