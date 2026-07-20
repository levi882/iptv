package capture

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"iptv/internal/atomicfile"
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
	written   uint64
	truncated bool
}

func newSafeBuffer(limit int) *safeBuffer {
	return &safeBuffer{data: make([]byte, limit)}
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	written := len(p)
	b.written += uint64(written)
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

// TotalWritten counts every byte ever written, including data that has
// rotated out of the ring, so pollers can cheaply detect new traffic.
func (b *safeBuffer) TotalWritten() uint64 {
	b.Lock()
	defer b.Unlock()
	return b.written
}

var patterns = map[string]*regexp.Regexp{
	"PROVIDER_USER_ID":         regexp.MustCompile(`UserID=([^&\s]+)`),
	"PROVIDER_AUTHENTICATOR":   regexp.MustCompile(`(?i)Authenticator=([0-9a-f]+)`),
	"PROVIDER_STBID":           regexp.MustCompile(`(?i)STBID=([0-9a-f]+)`),
	"PROVIDER_STBINFO":         regexp.MustCompile(`(?i)stbinfo=([0-9a-f]+)`),
	"PROVIDER_USER_TOKEN":      regexp.MustCompile(`UserToken=([_A-Za-z0-9-]+)`),
	"PROVIDER_STB_TYPE":        regexp.MustCompile(`(?i)stbtype=([^&\s]+)`),
	"PROVIDER_PRMID":           regexp.MustCompile(`(?i)prmid=([^&\s]*)`),
	"PROVIDER_DRM_SUPPLIER":    regexp.MustCompile(`(?i)drmsupplier=([^&\s]*)`),
	"PROVIDER_CITYCODE":        regexp.MustCompile(`citycode=([0-9]+)`),
	"PROVIDER_NETWORKID":       regexp.MustCompile(`networkid=([0-9]+)`),
	"PROVIDER_EASIP":           regexp.MustCompile(`easip=([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)`),
	"PROVIDER_PLATFORM_ORIGIN": regexp.MustCompile(`(https?://[A-Za-z0-9.-]+(?::[0-9]+)?)/iptvepg/platform/index\.jsp`),
	"PROVIDER_EPG_ENTRY":       regexp.MustCompile(`(https?://[A-Za-z0-9.-]+(?::[0-9]+)?)/iptvepg/function/(?:index|funcportalauth|frameset_builder)\.jsp`),
	"PROVIDER_USER_AGENT":      regexp.MustCompile(`(?im)^User-Agent:[ \t]*([^\r\n]+)`),
}

var configTokenPattern = regexp.MustCompile(`CTCSetConfig\('UserToken','([_A-Za-z0-9-]+)'`)
var channelTokenPattern = regexp.MustCompile(`GetChannelList\?UserToken=([_A-Za-z0-9-]+)`)
var tokenServerPattern = regexp.MustCompile(`(?i)(?:Host:\s*|https?://)([A-Za-z0-9.-]+):4338`)

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
	fallback = fallback.NormalizeProviderKeys()
	out := config.Env{}
	for key, re := range patterns {
		out[key] = lastMatch(re, raw)
	}
	for _, key := range []string{"PROVIDER_STB_TYPE", "PROVIDER_PRMID", "PROVIDER_DRM_SUPPLIER"} {
		out[key] = decodedFormValue(out[key])
	}
	if out["PROVIDER_USER_TOKEN"] == "" {
		out["PROVIDER_USER_TOKEN"] = lastMatch(configTokenPattern, raw)
	}
	if out["PROVIDER_USER_TOKEN"] == "" {
		out["PROVIDER_USER_TOKEN"] = lastMatch(channelTokenPattern, raw)
	}
	for key, value := range fallback {
		if out[key] == "" {
			out[key] = value
		}
	}
	out["PROVIDER_AUTHENTICATOR"] = strings.ToUpper(out["PROVIDER_AUTHENTICATOR"])
	out["PROVIDER_STBID"] = strings.ToUpper(out["PROVIDER_STBID"])
	out["PROVIDER_STBINFO"] = strings.ToUpper(out["PROVIDER_STBINFO"])
	out["PROVIDER_USER_TOKEN"] = strings.Map(func(r rune) rune {
		if r == '_' || r == '-' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			return r
		}
		return -1
	}, out["PROVIDER_USER_TOKEN"])
	if capturedTokenHost := lastMatch(tokenServerPattern, raw); capturedTokenHost != "" {
		tokenHost = capturedTokenHost
	}
	if tokenHost != "" && !strings.EqualFold(tokenHost, "auto") {
		out["PROVIDER_TOKEN_SERVER"] = "http://" + tokenHost + ":4338"
	}
	return out
}

func Complete(env config.Env) bool {
	env = env.NormalizeProviderKeys()
	return env["PROVIDER_USER_ID"] != "" && (env["PROVIDER_AUTHENTICATOR"] != "" || env["PROVIDER_USER_TOKEN"] != "") && env["PROVIDER_STBID"] != "" && env["PROVIDER_STBINFO"] != ""
}

func capturedEnough(raw []byte, tokenHost string) bool {
	fresh := Parse(raw, config.Env{}, tokenHost)
	// Authenticator is commonly single-use and has already been consumed by
	// the STB before its portal request appears. Wait for the fresh UserToken
	// and both device fields so one capture cannot mix a new login with cached
	// credentials from a different session or set-top box.
	return fresh["PROVIDER_USER_ID"] != "" && fresh["PROVIDER_USER_TOKEN"] != "" &&
		fresh["PROVIDER_STBID"] != "" && fresh["PROVIDER_STBINFO"] != "" &&
		fresh["PROVIDER_TOKEN_SERVER"] != "" && fresh["PROVIDER_PLATFORM_ORIGIN"] != "" &&
		fresh["PROVIDER_EPG_ENTRY"] != "" && fresh["PROVIDER_EASIP"] != "" &&
		fresh["PROVIDER_NETWORKID"] != "" && fresh["PROVIDER_STB_TYPE"] != ""
}

func Save(path string, env config.Env) error {
	keys := []string{
		"PROVIDER_USER_ID", "PROVIDER_STBID", "PROVIDER_AUTHENTICATOR", "PROVIDER_STBINFO", "PROVIDER_USER_TOKEN",
		"PROVIDER_STB_TYPE", "PROVIDER_PRMID", "PROVIDER_DRM_SUPPLIER", "PROVIDER_USER_AGENT",
		"PROVIDER_TOKEN_SERVER", "PROVIDER_EPG_ENTRY", "PROVIDER_EASIP", "PROVIDER_NETWORKID", "PROVIDER_CITYCODE", "PROVIDER_PLATFORM_ORIGIN",
	}
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s=%s\n", key, env[key])
	}
	// Captured credentials need a cold STB boot to recover, so they must
	// survive a router power cut; atomicfile fsyncs before the rename.
	return atomicfile.Write(path, []byte(b.String()), 0o600)
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
	var parsedAt uint64
	for {
		select {
		case <-ticker.C:
			// Re-running ~15 regexes over a 4 MiB ring every tick is wasteful
			// on router CPUs; skip when tcpdump produced no new bytes.
			if total := output.TotalWritten(); total > parsedAt {
				parsedAt = total
				if capturedEnough(output.BytesCopy(), opts.TokenHost) {
					cancel()
				}
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
		if err := atomicfile.Write(opts.DumpPath, raw, 0o600); err != nil {
			return nil, fmt.Errorf("write capture dump: %w", err)
		}
	}
	fresh := Parse(raw, config.Env{}, opts.TokenHost)
	if !capturedEnough(raw, opts.TokenHost) {
		if truncated {
			return nil, fmt.Errorf("credential capture exceeded the %d MiB safety limit before a complete login was found; reduce unrelated port 8080 traffic and retry", maxCaptureBytes>>20)
		}
		missing := []string{}
		for _, key := range []string{"PROVIDER_USER_ID", "PROVIDER_USER_TOKEN", "PROVIDER_STBID", "PROVIDER_STBINFO", "PROVIDER_TOKEN_SERVER", "PROVIDER_PLATFORM_ORIGIN", "PROVIDER_EPG_ENTRY", "PROVIDER_EASIP", "PROVIDER_NETWORKID", "PROVIDER_STB_TYPE"} {
			if fresh[key] == "" {
				missing = append(missing, strings.TrimPrefix(key, "PROVIDER_"))
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
