package capture

import (
	"path/filepath"
	"strings"
	"testing"

	"iptv/internal/config"
)

func TestParse(t *testing.T) {
	raw := []byte("POST /GetUserToken UserID=user1&Authenticator=aabb&citycode=027\nHost: 203.0.113.9:4338\nUser-Agent: Actual-STB/1.0\nGET http://198.51.100.6:8080/iptvepg/platform/index.jsp\nPOST http://198.51.100.4:8080/iptvepg/function/funcportalauth.jsp\nSTBID=abcd&stbinfo=eeff&easip=198.51.100.4&networkid=9&stbtype=B860AV2.1%2DT&prmid=vendor%2D1&drmsupplier=drm%2Bvendor&UserToken=token-1")
	env := Parse(raw, config.Env{}, "203.0.113.8")
	if !Complete(env) || env["PROVIDER_AUTHENTICATOR"] != "AABB" || env["PROVIDER_STBID"] != "ABCD" || env["PROVIDER_USER_TOKEN"] != "token-1" || env["PROVIDER_EPG_ENTRY"] != "http://198.51.100.4:8080" || env["PROVIDER_TOKEN_SERVER"] != "http://203.0.113.9:4338" || env["PROVIDER_STB_TYPE"] != "B860AV2.1-T" || env["PROVIDER_PRMID"] != "vendor-1" || env["PROVIDER_DRM_SUPPLIER"] != "drm+vendor" || env["PROVIDER_USER_AGENT"] != "Actual-STB/1.0" {
		t.Fatalf("unexpected capture: %#v", env)
	}
	for key, value := range env {
		if strings.ContainsAny(value, "\r\n") {
			t.Fatalf("unsafe value %s=%q", key, value)
		}
	}
}

func TestParseTokenServerFallback(t *testing.T) {
	env := Parse(nil, config.Env{}, "203.0.113.8")
	if env["PROVIDER_TOKEN_SERVER"] != "http://203.0.113.8:4338" {
		t.Fatalf("token server = %q", env["PROVIDER_TOKEN_SERVER"])
	}
}

func TestCapturedEnoughRequiresFreshLogin(t *testing.T) {
	if capturedEnough(nil, "") {
		t.Fatal("fallback-only credentials must not stop capture early")
	}
	if capturedEnough([]byte("UserID=new&Authenticator=dd&UserToken=new-token"), "") {
		t.Fatal("partial fresh login must not stop capture")
	}
	complete := []byte("Host: 203.0.113.9:4338\nUserID=new&UserToken=new-token&STBID=BB&stbinfo=CC&stbtype=Demo-STB&easip=198.51.100.4&networkid=1\nGET http://198.51.100.6:8080/iptvepg/platform/index.jsp\nGET http://198.51.100.4:8080/iptvepg/function/index.jsp")
	if !capturedEnough(complete, "") {
		t.Fatal("complete fresh portal login should stop capture")
	}
}

func TestFinishRejectsFreshLoginMixedWithCachedDeviceFields(t *testing.T) {
	output := filepath.Join(t.TempDir(), "credentials.env")
	_, err := finish(
		[]byte("UserID=new&Authenticator=dd&UserToken=new-token"),
		false,
		Options{
			OutputPath: output,
			Fallback: config.Env{
				"PROVIDER_STBID":   "OLD-STB",
				"PROVIDER_STBINFO": "OLD-INFO",
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "STBID, STBINFO") {
		t.Fatalf("unexpected mixed-capture result: %v", err)
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
