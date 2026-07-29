package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestReaderCachesAndFallsBackToStale(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		fmt.Fprint(w, "payload")
	}))
	reader := Reader{CacheDir: t.TempDir(), UseCache: true, TTL: time.Hour}
	for i := range 2 {
		data, err := reader.Read(context.Background(), server.URL)
		if err != nil || string(data) != "payload" {
			t.Fatalf("read %d: data=%q err=%v", i, data, err)
		}
	}
	if requests != 1 {
		t.Fatalf("requests=%d, want 1", requests)
	}
	server.Close()
	reader.TTL = time.Nanosecond
	time.Sleep(time.Millisecond)
	data, err := reader.Read(context.Background(), server.URL)
	if err != nil || string(data) != "payload" {
		t.Fatalf("stale fallback: data=%q err=%v", data, err)
	}
}

func TestReaderAuthenticatesOnlyHTTPSGitHubAPIRequests(t *testing.T) {
	const token = "github-secret"
	requests := map[string]http.Header{}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests[req.URL.String()] = req.Header.Clone()
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("payload")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}
	reader := Reader{Client: client, GitHubToken: token}

	urls := []string{
		"https://api.github.com/repos/example/project/contents/tv",
		"http://api.github.com/repos/example/project/contents/tv",
		"https://api.github.com.example.test/repos/example/project/contents/tv",
		"https://example.test/source",
	}
	for _, sourceURL := range urls {
		if _, err := reader.Read(context.Background(), sourceURL); err != nil {
			t.Fatalf("read %s: %v", sourceURL, err)
		}
	}

	githubHeaders := requests[urls[0]]
	if got := githubHeaders.Get("Authorization"); got != "Bearer "+token {
		t.Fatalf("GitHub Authorization = %q", got)
	}
	if got := githubHeaders.Get("Accept"); got != "application/vnd.github+json" {
		t.Fatalf("GitHub Accept = %q", got)
	}
	if got := githubHeaders.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
		t.Fatalf("GitHub API version = %q", got)
	}
	for _, sourceURL := range urls[1:] {
		if got := requests[sourceURL].Get("Authorization"); got != "" {
			t.Fatalf("Authorization sent to %s", sourceURL)
		}
	}
}

func TestReaderStripsGitHubAuthenticationOnUnsafeRedirect(t *testing.T) {
	var requests []*http.Request
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.Clone(req.Context()))
		statusCode := http.StatusFound
		status := "302 Found"
		header := http.Header{"Location": []string{"http://api.github.com/redirected"}}
		body := ""
		if len(requests) == 2 {
			statusCode = http.StatusOK
			status = "200 OK"
			header = make(http.Header)
			body = "payload"
		}
		return &http.Response{
			StatusCode: statusCode,
			Status:     status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     header,
			Request:    req,
		}, nil
	})}
	reader := Reader{Client: client, GitHubToken: "github-secret"}

	if _, err := reader.Read(context.Background(), "https://api.github.com/source"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	if got := requests[0].Header.Get("Authorization"); got == "" {
		t.Fatal("initial GitHub request was not authenticated")
	}
	if got := requests[1].Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization leaked on redirect: %q", got)
	}
	if got := requests[1].Header.Get("X-GitHub-Api-Version"); got != "" {
		t.Fatalf("GitHub API version leaked on redirect: %q", got)
	}
}
