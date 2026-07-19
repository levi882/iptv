package capture

import (
	"path/filepath"
	"strings"
	"testing"

	"iptv/internal/config"
)

func TestParse(t *testing.T) {
	raw := []byte("POST /GetUserToken UserID=user1&Authenticator=aabb&citycode=027\nHost: 5.6.7.9:4338\nUser-Agent: Actual-STB/1.0\nPOST http://1.2.3.4:8080/iptvepg/function/funcportalauth.jsp\nSTBID=abcd&stbinfo=eeff&easip=1.2.3.4&networkid=9&stbtype=B860AV2.1%2DT&prmid=vendor%2D1&drmsupplier=drm%2Bvendor&UserToken=token-1")
	env := Parse(raw, config.Env{}, "5.6.7.8")
	if !Complete(env) || env["HB_AUTHENTICATOR"] != "AABB" || env["HB_STBID"] != "ABCD" || env["HB_USER_TOKEN"] != "token-1" || env["HB_EPG_ENTRY"] != "http://1.2.3.4:8080" || env["HB_TOKEN_SERVER"] != "http://5.6.7.9:4338" || env["HB_STB_TYPE"] != "B860AV2.1-T" || env["HB_PRMID"] != "vendor-1" || env["HB_DRM_SUPPLIER"] != "drm+vendor" || env["HB_USER_AGENT"] != "Actual-STB/1.0" {
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
	if capturedEnough(nil, "121.60.255.37") {
		t.Fatal("fallback-only credentials must not stop capture early")
	}
	if capturedEnough([]byte("UserID=new&Authenticator=dd&UserToken=new-token"), "121.60.255.37") {
		t.Fatal("partial fresh login must not stop capture")
	}
	if !capturedEnough([]byte("UserID=new&UserToken=new-token&STBID=BB&stbinfo=CC"), "121.60.255.37") {
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
				"HB_STBID":   "OLD-STB",
				"HB_STBINFO": "OLD-INFO",
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
