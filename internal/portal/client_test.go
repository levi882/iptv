package portal

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"iptv/internal/playlist"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestFetchFlow(t *testing.T) {
	steps := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		steps = append(steps, r.URL.Path)
		switch r.URL.Path {
		case "/GetUserToken":
			fmt.Fprint(w, "UserToken=test-token")
		case "/iptvepg/function/index.jsp":
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "session"})
			fmt.Fprint(w, "ok")
		case "/iptvepg/function/funcportalauth.jsp":
			if cookie, err := r.Cookie("JSESSIONID"); err != nil || cookie.Value != "session" {
				http.Error(w, "missing session", http.StatusForbidden)
				return
			}
			fmt.Fprint(w, "ok")
		case "/iptvepg/function/frameset_builder.jsp":
			fmt.Fprint(w, "jsSetConfig('Channel','ChannelName=\"One\"');")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(Config{TokenServer: server.URL, PlatformOrigin: server.URL, EPGEntry: server.URL, EASIP: "127.0.0.1", NetworkID: "1", Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Fetch(context.Background(), Credentials{UserID: "u", STBID: "s", Authenticator: "a", STBInfo: "i"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Token != "test-token" || result.EPGHost != server.URL || !strings.Contains(result.Frameset, "jsSetConfig") {
		t.Fatalf("unexpected result: %#v", result)
	}
	want := []string{"/GetUserToken", "/iptvepg/function/index.jsp", "/iptvepg/function/funcportalauth.jsp", "/iptvepg/function/frameset_builder.jsp"}
	if fmt.Sprint(steps) != fmt.Sprint(want) {
		t.Fatalf("steps = %v, want %v", steps, want)
	}
}

func TestFetchDecodesGBKChannelNames(t *testing.T) {
	frameset := `jsSetConfig('Channel','ChannelName="示例卫视" ChannelURL="igmp://239.1.1.1:1234"');`
	gbkFrameset, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(frameset))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/iptvepg/function/index.jsp", "/iptvepg/function/funcportalauth.jsp":
			w.Header().Set("Content-Type", "text/html;charset=GBK")
			fmt.Fprint(w, "ok")
		case "/iptvepg/function/frameset_builder.jsp":
			w.Header().Set("Content-Type", "text/html;charset=GBK")
			_, _ = w.Write(gbkFrameset)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(Config{EPGEntry: server.URL, EASIP: "127.0.0.1", NetworkID: "1", Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Fetch(context.Background(), Credentials{UserID: "u", STBID: "s", STBInfo: "i", UserToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Frameset, "示例卫视") || !utf8.ValidString(result.Frameset) {
		t.Fatalf("frameset was not converted to UTF-8: %q", result.Frameset)
	}
	channels := playlist.ParseChannels(result.Frameset, playlist.URLSelectParams{Mode: "auto"}, "none", "超高清", "高清", "标清")
	rows, _, _ := playlist.ChannelsToRows(channels)
	logos := map[string]string{playlist.NormalizeName("示例卫视"): "http://logo/example.png"}
	if len(rows) != 1 || playlist.AttachLogos(rows, logos, .65) != 1 || rows[0].LogoURL == "" {
		t.Fatalf("decoded channel did not recover logo matching: channels=%#v rows=%#v", channels, rows)
	}
}

func TestDecodeResponseBodyFallsBackToGB18030(t *testing.T) {
	raw, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte("广东珠江频道"))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeResponseBody(raw, "text/html")
	if err != nil || string(decoded) != "广东珠江频道" {
		t.Fatalf("GB18030 fallback = %q, %v", decoded, err)
	}
	utf8Body := []byte("示例卫视")
	decoded, err = decodeResponseBody(utf8Body, "text/html; charset=utf-8")
	if err != nil || string(decoded) != string(utf8Body) {
		t.Fatalf("UTF-8 response changed: %q, %v", decoded, err)
	}
}

func TestInitSessionFollowsPageRedirectAndKeepsSession(t *testing.T) {
	balanced := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("loadbalanced") != "1" || r.URL.Query().Get("region") != "test" {
			http.Error(w, "missing redirect query", http.StatusBadRequest)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "balanced-session"})
		fmt.Fprint(w, "ok")
	}))
	defer balanced.Close()
	entry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<script>window.location="%s/iptvepg/function/index.jsp?loadbalanced=1&amp;region=test"</script>`, balanced.URL)
	}))
	defer entry.Close()

	client, err := New(Config{EPGEntry: entry.URL, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.initSession(context.Background(), entry.URL, "token", Credentials{UserID: "u", STBID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if session.Host != balanced.URL || !strings.Contains(session.Referer, "loadbalanced=1&region=test") {
		t.Fatalf("session = %#v", session)
	}
	balancedURL, _ := url.Parse(session.Referer)
	cookies := client.http.Jar.Cookies(balancedURL)
	if len(cookies) != 1 || cookies[0].Name != "JSESSIONID" || cookies[0].Value != "balanced-session" {
		t.Fatalf("balanced cookies = %#v", cookies)
	}
}

func TestFetchFollowsLoadBalancedSessionBeforePortalAuth(t *testing.T) {
	steps := []string{}
	balanced := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		steps = append(steps, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/iptvepg/function/index.jsp":
			if r.URL.Query().Get("loadbalanced") != "1" {
				http.Error(w, "missing load-balanced marker", http.StatusBadRequest)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "balanced-session"})
			fmt.Fprint(w, "ready")
		case "/iptvepg/function/funcportalauth.jsp":
			if cookie, err := r.Cookie("JSESSIONID"); err != nil || cookie.Value != "balanced-session" {
				http.Error(w, "missing balanced session", http.StatusForbidden)
				return
			}
			if !strings.Contains(r.Referer(), "loadbalanced=1") {
				http.Error(w, "missing balanced referer", http.StatusForbidden)
				return
			}
			if err := r.ParseForm(); err != nil || r.Form.Get("stbtype") != "B860AV2.1-T" || r.Form.Get("prmid") != "vendor-1" || r.Form.Get("drmsupplier") != "drm-vendor" {
				http.Error(w, "missing captured STB parameters", http.StatusForbidden)
				return
			}
			fmt.Fprint(w, "ok")
		case "/iptvepg/function/frameset_builder.jsp":
			fmt.Fprint(w, `jsSetConfig('Channel','ChannelName="One"');`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer balanced.Close()
	entry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		steps = append(steps, r.Method+" "+r.URL.Path)
		fmt.Fprintf(w, `<script>window.location="%s/iptvepg/function/index.jsp?loadbalanced=1"</script>`, balanced.URL)
	}))
	defer entry.Close()

	client, err := New(Config{EPGEntry: entry.URL, EASIP: "127.0.0.1", NetworkID: "1", Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Fetch(context.Background(), Credentials{UserID: "u", STBID: "s", STBInfo: "i", UserToken: "token", STBType: "B860AV2.1-T", PRMID: "vendor-1", DRMSupplier: "drm-vendor"})
	if err != nil {
		t.Fatal(err)
	}
	if result.EPGHost != balanced.URL || !strings.Contains(result.Frameset, "jsSetConfig") {
		t.Fatalf("unexpected result: %#v", result)
	}
	want := []string{
		"GET /iptvepg/function/index.jsp",
		"GET /iptvepg/function/index.jsp",
		"POST /iptvepg/function/funcportalauth.jsp",
		"POST /iptvepg/function/frameset_builder.jsp",
	}
	if fmt.Sprint(steps) != fmt.Sprint(want) {
		t.Fatalf("steps = %v, want %v", steps, want)
	}
}

func TestFetchRefreshesExpiredSavedToken(t *testing.T) {
	tokenCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/GetUserToken":
			tokenCalls++
			fmt.Fprint(w, "UserToken=fresh-token")
		case "/iptvepg/function/index.jsp":
			fmt.Fprint(w, "ok")
		case "/iptvepg/function/funcportalauth.jsp":
			if err := r.ParseForm(); err != nil || r.Form.Get("UserToken") != "fresh-token" {
				fmt.Fprint(w, "errcode = 401")
				return
			}
			fmt.Fprint(w, "ok")
		case "/iptvepg/function/frameset_builder.jsp":
			fmt.Fprint(w, `jsSetConfig('Channel','ChannelName="One"');`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(Config{TokenServer: server.URL, PlatformOrigin: server.URL, EPGEntry: server.URL, EASIP: "127.0.0.1", NetworkID: "1", Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Fetch(context.Background(), Credentials{UserID: "u", STBID: "s", Authenticator: "a", STBInfo: "i", UserToken: "expired-token"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Token != "fresh-token" || tokenCalls != 1 {
		t.Fatalf("token refresh fallback: result=%#v tokenCalls=%d", result, tokenCalls)
	}
}

func TestFetchDoesNotRefreshSavedTokenAfterTransientFailure(t *testing.T) {
	tokenCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/GetUserToken" {
			tokenCalls++
		}
		http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := New(Config{TokenServer: server.URL, PlatformOrigin: server.URL, EPGEntry: server.URL, EASIP: "127.0.0.1", NetworkID: "1", Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Fetch(context.Background(), Credentials{UserID: "u", STBID: "s", Authenticator: "single-use", STBInfo: "i", UserToken: "saved-token"})
	if err == nil {
		t.Fatal("Fetch succeeded unexpectedly")
	}
	if tokenCalls != 0 {
		t.Fatalf("GetUserToken was called %d times after a transient failure", tokenCalls)
	}
}

func TestAuthenticationErrorClassification(t *testing.T) {
	for _, err := range []error{
		&responseError{statusCode: http.StatusUnauthorized},
		&responseError{statusCode: http.StatusForbidden},
		&providerAuthError{code: "401"},
	} {
		if !isAuthenticationError(err) {
			t.Fatalf("authentication error was not classified: %T", err)
		}
	}
	if isAuthenticationError(&responseError{statusCode: http.StatusServiceUnavailable}) {
		t.Fatal("transient HTTP failure was classified as authentication failure")
	}
	if err := authenticationError([]byte("errcode=500")); err != nil {
		t.Fatalf("non-auth provider error was classified as authentication failure: %v", err)
	}
}

func TestFetchWithoutAuthenticatorDoesNotRetryExpiredToken(t *testing.T) {
	tokenCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/GetUserToken" {
			tokenCalls++
		}
		http.Error(w, "errcode = 401", http.StatusForbidden)
	}))
	defer server.Close()

	client, err := New(Config{TokenServer: server.URL, PlatformOrigin: server.URL, EPGEntry: server.URL, EASIP: "127.0.0.1", NetworkID: "1", Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Fetch(context.Background(), Credentials{UserID: "u", STBID: "s", STBInfo: "i", UserToken: "expired-token"})
	if err == nil {
		t.Fatal("Fetch succeeded unexpectedly")
	}
	if tokenCalls != 0 {
		t.Fatalf("GetUserToken was called %d times without an Authenticator", tokenCalls)
	}
}

func TestFetchRedactsCredentialsAndReportsStage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/iptvepg/function/index.jsp" {
			http.Error(w, "UserToken=reflected-secret&UserID=reflected-user", http.StatusForbidden)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := New(Config{EPGEntry: server.URL, EASIP: "127.0.0.1", NetworkID: "1", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Fetch(context.Background(), Credentials{UserID: "subscriber-secret", STBID: "stb-secret", STBInfo: "info", UserToken: "token-secret"})
	if err == nil {
		t.Fatal("Fetch succeeded unexpectedly")
	}
	message := err.Error()
	for _, secret := range []string{"subscriber-secret", "stb-secret", "token-secret", "reflected-secret", "reflected-user"} {
		if strings.Contains(message, secret) {
			t.Fatalf("error contains %q: %s", secret, message)
		}
	}
	if !strings.Contains(message, "initialize session") || !strings.Contains(message, "[redacted]") {
		t.Fatalf("error lacks safe stage information: %s", message)
	}
}
