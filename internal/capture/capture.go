package capture

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"iptv/internal/config"
)

type Options struct {
	Interface  string
	Timeout    time.Duration
	OutputPath string
	DumpPath   string
	TokenHost  string
	TCPDump    string
	Fallback   config.Env
	OnReady    func(context.Context) error
	ReadyDelay time.Duration
}

const (
	maxCaptureBytes    = 4 << 20
	maxDiagnosticBytes = 64 << 10
)

type safeBuffer struct {
	sync.Mutex
	data      []byte
	start     int
	size      int
	truncated bool
}

func newSafeBuffer(limit int) *safeBuffer {
	return &safeBuffer{data: make([]byte, limit)}
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	written := len(p)
	if len(b.data) == 0 || written == 0 {
		return written, nil
	}
	if len(p) >= len(b.data) {
		copy(b.data, p[len(p)-len(b.data):])
		b.start = 0
		b.size = len(b.data)
		b.truncated = true
		return written, nil
	}
	if overflow := b.size + len(p) - len(b.data); overflow > 0 {
		b.start = (b.start + overflow) % len(b.data)
		b.size -= overflow
		b.truncated = true
	}
	end := (b.start + b.size) % len(b.data)
	first := min(len(p), len(b.data)-end)
	copy(b.data[end:], p[:first])
	copy(b.data, p[first:])
	b.size += len(p)
	return written, nil
}

func (b *safeBuffer) BytesCopy() []byte {
	b.Lock()
	defer b.Unlock()
	result := make([]byte, b.size)
	first := min(b.size, len(b.data)-b.start)
	copy(result, b.data[b.start:b.start+first])
	copy(result[first:], b.data[:b.size-first])
	return result
}

func (b *safeBuffer) Truncated() bool {
	b.Lock()
	defer b.Unlock()
	return b.truncated
}

var patterns = map[string]*regexp.Regexp{
	"HB_USER_ID":         regexp.MustCompile(`UserID=([^&\s]+)`),
	"HB_AUTHENTICATOR":   regexp.MustCompile(`(?i)Authenticator=([0-9a-f]+)`),
	"HB_STBID":           regexp.MustCompile(`(?i)STBID=([0-9a-f]+)`),
	"HB_STBINFO":         regexp.MustCompile(`(?i)stbinfo=([0-9a-f]+)`),
	"HB_USER_TOKEN":      regexp.MustCompile(`UserToken=([_A-Za-z0-9-]+)`),
	"HB_STB_TYPE":        regexp.MustCompile(`(?i)stbtype=([^&\s]+)`),
	"HB_PRMID":           regexp.MustCompile(`(?i)prmid=([^&\s]*)`),
	"HB_DRM_SUPPLIER":    regexp.MustCompile(`(?i)drmsupplier=([^&\s]*)`),
	"HB_CITYCODE":        regexp.MustCompile(`citycode=([0-9]+)`),
	"HB_NETWORKID":       regexp.MustCompile(`networkid=([0-9]+)`),
	"HB_EASIP":           regexp.MustCompile(`easip=([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)`),
	"HB_PLATFORM_ORIGIN": regexp.MustCompile(`(http://[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+:8080)/iptvepg/platform/index\.jsp`),
	"HB_EPG_ENTRY":       regexp.MustCompile(`(http://[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+:8080)/iptvepg/function/(?:index|funcportalauth|frameset_builder)\.jsp`),
	"HB_USER_AGENT":      regexp.MustCompile(`(?im)^User-Agent:[ \t]*([^\r\n]+)`),
}

var configTokenPattern = regexp.MustCompile(`CTCSetConfig\('UserToken','([_A-Za-z0-9-]+)'`)
var channelTokenPattern = regexp.MustCompile(`GetChannelList\?UserToken=([_A-Za-z0-9-]+)`)
var tokenServerPattern = regexp.MustCompile(`(?i)(?:Host:\s*|https?://)([0-9]+(?:\.[0-9]+){3}):4338`)

func lastMatch(re *regexp.Regexp, raw []byte) string {
	matches := re.FindAllSubmatch(raw, -1)
	if len(matches) == 0 || len(matches[len(matches)-1]) < 2 {
		return ""
	}
	return string(matches[len(matches)-1][1])
}

func decodedFormValue(value string) string {
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return value
	}
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == 0 {
			return -1
		}
		return r
	}, decoded)
}

func Parse(raw []byte, fallback config.Env, tokenHost string) config.Env {
	out := config.Env{}
	for key, re := range patterns {
		out[key] = lastMatch(re, raw)
	}
	for _, key := range []string{"HB_STB_TYPE", "HB_PRMID", "HB_DRM_SUPPLIER"} {
		out[key] = decodedFormValue(out[key])
	}
	if out["HB_USER_TOKEN"] == "" {
		out["HB_USER_TOKEN"] = lastMatch(configTokenPattern, raw)
	}
	if out["HB_USER_TOKEN"] == "" {
		out["HB_USER_TOKEN"] = lastMatch(channelTokenPattern, raw)
	}
	for key, value := range fallback {
		if out[key] == "" {
			out[key] = value
		}
	}
	out["HB_AUTHENTICATOR"] = strings.ToUpper(out["HB_AUTHENTICATOR"])
	out["HB_STBID"] = strings.ToUpper(out["HB_STBID"])
	out["HB_STBINFO"] = strings.ToUpper(out["HB_STBINFO"])
	out["HB_USER_TOKEN"] = strings.Map(func(r rune) rune {
		if r == '_' || r == '-' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			return r
		}
		return -1
	}, out["HB_USER_TOKEN"])
	if capturedTokenHost := lastMatch(tokenServerPattern, raw); capturedTokenHost != "" {
		tokenHost = capturedTokenHost
	}
	if tokenHost == "" {
		tokenHost = "121.60.255.37"
	}
	out["HB_TOKEN_SERVER"] = "http://" + tokenHost + ":4338"
	if out["HB_EPG_ENTRY"] == "" {
		out["HB_EPG_ENTRY"] = "http://121.60.255.4:8080"
	}
	if out["HB_EASIP"] == "" {
		out["HB_EASIP"] = "121.60.255.4"
	}
	if out["HB_NETWORKID"] == "" {
		out["HB_NETWORKID"] = "1"
	}
	if out["HB_PLATFORM_ORIGIN"] == "" {
		out["HB_PLATFORM_ORIGIN"] = "http://121.60.255.6:8080"
	}
	return out
}

func Complete(env config.Env) bool {
	return env["HB_USER_ID"] != "" && (env["HB_AUTHENTICATOR"] != "" || env["HB_USER_TOKEN"] != "") && env["HB_STBID"] != "" && env["HB_STBINFO"] != ""
}

func capturedEnough(raw []byte, tokenHost string) bool {
	fresh := Parse(raw, config.Env{}, tokenHost)
	// Authenticator is commonly single-use and has already been consumed by
	// the STB before its portal request appears. Wait for the fresh UserToken
	// and both device fields so one capture cannot mix a new login with cached
	// credentials from a different session or set-top box.
	return fresh["HB_USER_ID"] != "" && fresh["HB_USER_TOKEN"] != "" &&
		fresh["HB_STBID"] != "" && fresh["HB_STBINFO"] != ""
}

func Save(path string, env config.Env) error {
	keys := []string{
		"HB_USER_ID", "HB_STBID", "HB_AUTHENTICATOR", "HB_STBINFO", "HB_USER_TOKEN",
		"HB_STB_TYPE", "HB_PRMID", "HB_DRM_SUPPLIER", "HB_USER_AGENT",
		"HB_TOKEN_SERVER", "HB_EPG_ENTRY", "HB_EASIP", "HB_NETWORKID", "HB_CITYCODE", "HB_PLATFORM_ORIGIN",
	}
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s=%s\n", key, env[key])
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".creds-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := io.WriteString(tmp, b.String()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func Run(ctx context.Context, opts Options) (config.Env, error) {
	if opts.TCPDump == "" {
		opts.TCPDump = "tcpdump"
	}
	if opts.Interface == "" {
		opts.Interface = "any"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 180 * time.Second
	}
	if _, err := exec.LookPath(opts.TCPDump); err != nil {
		return nil, fmt.Errorf("tcpdump not found: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	// The provider may move GetUserToken between adjacent :4338 hosts. Capture
	// the service port itself and let Parse discover the host from the request.
	filter := "tcp and (port 4338 or port 8080)"
	cmd := exec.CommandContext(ctx, opts.TCPDump, "-i", opts.Interface, "-s0", "-A", filter)
	output := newSafeBuffer(maxCaptureBytes)
	diagnostics := newSafeBuffer(maxDiagnosticBytes)
	cmd.Stdout = output
	cmd.Stderr = diagnostics
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if opts.OnReady != nil {
		delay := opts.ReadyDelay
		if delay <= 0 {
			delay = time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case err := <-done:
			timer.Stop()
			detail := strings.TrimSpace(string(diagnostics.BytesCopy()))
			if detail != "" {
				return nil, fmt.Errorf("tcpdump stopped before capture became ready: %v: %s", err, detail)
			}
			return nil, fmt.Errorf("tcpdump stopped before capture became ready: %v", err)
		case <-ctx.Done():
			timer.Stop()
			_ = cmd.Process.Kill()
			<-done
			return nil, ctx.Err()
		}
		if err := opts.OnReady(ctx); err != nil {
			cancel()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				_ = cmd.Process.Kill()
				<-done
			}
			return nil, fmt.Errorf("power on STB after capture startup: %w", err)
		}
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if capturedEnough(output.BytesCopy(), opts.TokenHost) {
				cancel()
			}
		case <-ctx.Done():
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				_ = cmd.Process.Kill()
				<-done
			}
			raw := output.BytesCopy()
			return finish(raw, output.Truncated(), opts)
		case err := <-done:
			raw := output.BytesCopy()
			if err != nil && len(raw) == 0 {
				detail := strings.TrimSpace(string(diagnostics.BytesCopy()))
				if detail != "" {
					return nil, fmt.Errorf("tcpdump failed: %w: %s", err, detail)
				}
				return nil, fmt.Errorf("tcpdump failed: %w", err)
			}
			return finish(raw, output.Truncated(), opts)
		}
	}
}

func finish(raw []byte, truncated bool, opts Options) (config.Env, error) {
	if opts.DumpPath != "" {
		if err := os.MkdirAll(filepath.Dir(opts.DumpPath), 0o755); err != nil {
			return nil, fmt.Errorf("create capture dump directory: %w", err)
		}
		if err := os.WriteFile(opts.DumpPath, raw, 0o600); err != nil {
			return nil, fmt.Errorf("write capture dump: %w", err)
		}
	}
	fresh := Parse(raw, config.Env{}, opts.TokenHost)
	if !capturedEnough(raw, opts.TokenHost) {
		if truncated {
			return nil, fmt.Errorf("credential capture exceeded the %d MiB safety limit before a complete login was found; reduce unrelated port 8080 traffic and retry", maxCaptureBytes>>20)
		}
		missing := []string{}
		for _, key := range []string{"HB_USER_ID", "HB_USER_TOKEN", "HB_STBID", "HB_STBINFO"} {
			if fresh[key] == "" {
				missing = append(missing, strings.TrimPrefix(key, "HB_"))
			}
		}
		return nil, fmt.Errorf("incomplete STB login capture (missing %s); start capture before cold-booting the STB and wait for portal login to finish", strings.Join(missing, ", "))
	}
	env := Parse(raw, opts.Fallback, opts.TokenHost)
	if opts.OutputPath != "" {
		if err := Save(opts.OutputPath, env); err != nil {
			return nil, err
		}
	}
	return env, nil
}
