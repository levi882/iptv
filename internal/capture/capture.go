package capture

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
}

type safeBuffer struct {
	sync.Mutex
	bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.Write(p)
}

func (b *safeBuffer) BytesCopy() []byte {
	b.Lock()
	defer b.Unlock()
	return append([]byte(nil), b.Buffer.Bytes()...)
}

var patterns = map[string]*regexp.Regexp{
	"HB_USER_ID":         regexp.MustCompile(`UserID=([^&\s]+)`),
	"HB_AUTHENTICATOR":   regexp.MustCompile(`(?i)Authenticator=([0-9a-f]+)`),
	"HB_STBID":           regexp.MustCompile(`(?i)STBID=([0-9a-f]+)`),
	"HB_STBINFO":         regexp.MustCompile(`(?i)stbinfo=([0-9a-f]+)`),
	"HB_USER_TOKEN":      regexp.MustCompile(`UserToken=([_A-Za-z0-9-]+)`),
	"HB_CITYCODE":        regexp.MustCompile(`citycode=([0-9]+)`),
	"HB_NETWORKID":       regexp.MustCompile(`networkid=([0-9]+)`),
	"HB_EASIP":           regexp.MustCompile(`easip=([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)`),
	"HB_PLATFORM_ORIGIN": regexp.MustCompile(`(http://[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+:8080)/iptvepg/platform/index\.jsp`),
	"HB_EPG_ENTRY":       regexp.MustCompile(`(http://[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+:8080)/iptvepg/function/(?:index|funcportalauth|frameset_builder)\.jsp`),
}

var configTokenPattern = regexp.MustCompile(`CTCSetConfig\('UserToken','([_A-Za-z0-9-]+)'`)
var channelTokenPattern = regexp.MustCompile(`GetChannelList\?UserToken=([_A-Za-z0-9-]+)`)

func lastMatch(re *regexp.Regexp, raw []byte) string {
	matches := re.FindAllSubmatch(raw, -1)
	if len(matches) == 0 || len(matches[len(matches)-1]) < 2 {
		return ""
	}
	return string(matches[len(matches)-1][1])
}

func Parse(raw []byte, fallback config.Env, tokenHost string) config.Env {
	out := config.Env{}
	for key, re := range patterns {
		out[key] = lastMatch(re, raw)
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

func capturedEnough(raw []byte, fallback config.Env, tokenHost string) bool {
	fresh := Parse(raw, config.Env{}, tokenHost)
	return fresh["HB_USER_ID"] != "" && (fresh["HB_AUTHENTICATOR"] != "" || fresh["HB_USER_TOKEN"] != "") &&
		(fresh["HB_STBID"] != "" || fallback["HB_STBID"] != "") &&
		(fresh["HB_STBINFO"] != "" || fallback["HB_STBINFO"] != "")
}

func Save(path string, env config.Env) error {
	keys := []string{"HB_USER_ID", "HB_STBID", "HB_AUTHENTICATOR", "HB_STBINFO", "HB_USER_TOKEN", "HB_TOKEN_SERVER", "HB_EPG_ENTRY", "HB_EASIP", "HB_NETWORKID", "HB_CITYCODE", "HB_PLATFORM_ORIGIN"}
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
	if opts.TokenHost == "" {
		opts.TokenHost = "121.60.255.37"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 180 * time.Second
	}
	if _, err := exec.LookPath(opts.TCPDump); err != nil {
		return nil, fmt.Errorf("tcpdump not found: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	filter := fmt.Sprintf("tcp and ((host %s and port 4338) or port 8080)", opts.TokenHost)
	cmd := exec.CommandContext(ctx, opts.TCPDump, "-i", opts.Interface, "-s0", "-A", filter)
	var output safeBuffer
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if capturedEnough(output.BytesCopy(), opts.Fallback, opts.TokenHost) {
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
			return finish(raw, opts)
		case err := <-done:
			raw := output.BytesCopy()
			if err != nil && len(raw) == 0 {
				return nil, err
			}
			return finish(raw, opts)
		}
	}
}

func finish(raw []byte, opts Options) (config.Env, error) {
	if opts.DumpPath != "" {
		if err := os.MkdirAll(filepath.Dir(opts.DumpPath), 0o755); err == nil {
			_ = os.WriteFile(opts.DumpPath, raw, 0o600)
		}
	}
	env := Parse(raw, opts.Fallback, opts.TokenHost)
	if env["HB_USER_ID"] == "" || (env["HB_AUTHENTICATOR"] == "" && env["HB_USER_TOKEN"] == "") {
		return nil, fmt.Errorf("missing UserID and Authenticator/UserToken; ensure the STB logged in during capture")
	}
	if opts.OutputPath != "" {
		if err := Save(opts.OutputPath, env); err != nil {
			return nil, err
		}
	}
	return env, nil
}
