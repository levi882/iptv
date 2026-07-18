package hbiptv

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestInitSessionDiscoversPageRedirectHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<script>window.location="http://121.60.163.238:8080/iptvepg/function/index.jsp?loadbalanced=1"</script>`)
	}))
	defer server.Close()

	client, err := New(Config{EPGEntry: server.URL, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	host, err := client.initSession(context.Background(), server.URL, "token", Credentials{UserID: "u", STBID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if host != "http://121.60.163.238:8080" {
		t.Fatalf("host = %q", host)
	}
}
