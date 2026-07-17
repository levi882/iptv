package logocache

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"iptv/internal/playlist"
)

type Result struct {
	Downloaded int
	Reused     int
	Failed     int
}

var logoRE = regexp.MustCompile(`(?i)tvg-logo="([^"]*)"`)

var knownExtensions = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true, ".svg": true}

func hashURL(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func extension(value, contentType string) string {
	u, _ := url.Parse(value)
	ext := strings.ToLower(filepath.Ext(u.Path))
	if knownExtensions[ext] {
		return ext
	}
	if exts, _ := mime.ExtensionsByType(strings.Split(contentType, ";")[0]); len(exts) > 0 && knownExtensions[exts[0]] {
		return exts[0]
	}
	return ".img"
}

func download(ctx context.Context, client *http.Client, value, directory, userAgent string) (string, error) {
	var lastErr error
	for _, candidate := range playlist.LogoSourceCandidates(value) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", userAgent)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 || len(data) == 0 {
			lastErr = fmt.Errorf("download %s: HTTP %s: %v", candidate, resp.Status, readErr)
			continue
		}
		filename := hashURL(value) + extension(value, resp.Header.Get("Content-Type"))
		if err := os.WriteFile(filepath.Join(directory, filename), data, 0o644); err != nil {
			return "", err
		}
		return filename, nil
	}
	return "", lastErr
}

func Rewrite(ctx context.Context, playlistPath, directory, publicBase string, timeout time.Duration, userAgent string) (Result, error) {
	var result Result
	raw, err := os.ReadFile(playlistPath)
	if err != nil {
		return result, err
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return result, err
	}
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if userAgent == "" {
		userAgent = "Mozilla/5.0"
	}
	urls := []string{}
	seen := map[string]bool{}
	for _, match := range logoRE.FindAllSubmatch(raw, -1) {
		value := strings.TrimSpace(string(match[1]))
		if value != "" && !seen[value] {
			seen[value] = true
			urls = append(urls, value)
		}
	}
	sort.Strings(urls)
	mapping := map[string]string{}
	client := &http.Client{Timeout: timeout}
	publicBase = strings.TrimRight(publicBase, "/")
	for _, value := range urls {
		if (!strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://")) || strings.HasPrefix(value, publicBase+"/") {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(directory, hashURL(value)+".*"))
		filename := ""
		if len(matches) > 0 {
			filename = filepath.Base(matches[0])
			result.Reused++
		} else {
			filename, err = download(ctx, client, value, directory, userAgent)
			if err != nil {
				result.Failed++
				continue
			}
			result.Downloaded++
		}
		mapping[value] = publicBase + "/" + filename
	}
	if len(mapping) > 0 {
		raw = logoRE.ReplaceAllFunc(raw, func(match []byte) []byte {
			parts := logoRE.FindSubmatch(match)
			value := strings.TrimSpace(string(parts[1]))
			if replacement := mapping[value]; replacement != "" {
				return []byte(`tvg-logo="` + replacement + `"`)
			}
			return match
		})
		if err := os.WriteFile(playlistPath, raw, 0o644); err != nil {
			return result, err
		}
	}
	return result, nil
}
