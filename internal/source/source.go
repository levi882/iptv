package source

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Reader struct {
	Client    *http.Client
	CacheDir  string
	TTL       time.Duration
	UseCache  bool
	UserAgent string
}

func (r Reader) Read(ctx context.Context, source string) ([]byte, error) {
	if !strings.HasPrefix(source, "http://") && !strings.HasPrefix(source, "https://") {
		return os.ReadFile(source)
	}
	cachePath := r.cachePath(source)
	if r.UseCache {
		if data, ok := readFresh(cachePath, r.ttl()); ok {
			return data, nil
		}
	}
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	ua := r.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0"
	}
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	if err != nil {
		if r.UseCache {
			if data, readErr := os.ReadFile(cachePath); readErr == nil {
				return data, nil
			}
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: HTTP %s", source, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if r.UseCache && cachePath != "" {
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err == nil {
			_ = atomicWrite(cachePath, data, 0o644)
		}
	}
	return data, nil
}

func (r Reader) ttl() time.Duration {
	if r.TTL <= 0 {
		return 24 * time.Hour
	}
	return r.TTL
}

func (r Reader) cachePath(value string) string {
	if r.CacheDir == "" {
		return ""
	}
	sum := md5.Sum([]byte(value))
	return filepath.Join(r.CacheDir, hex.EncodeToString(sum[:]))
}

func readFresh(path string, ttl time.Duration) ([]byte, bool) {
	if path == "" {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > ttl {
		return nil, false
	}
	data, err := os.ReadFile(path)
	return data, err == nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".iptv-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
