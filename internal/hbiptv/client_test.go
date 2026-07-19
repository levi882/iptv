package hbiptv

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFetchFlow(t *testing.T) {
	steps := []string{}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
