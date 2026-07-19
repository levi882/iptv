package stbpower

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const DefaultTimeout = 10 * time.Second

type Webhook struct {
	URL     string
	Timeout time.Duration
	Client  *http.Client
}

func (w Webhook) Trigger(ctx context.Context) error {
	parsed, err := url.Parse(w.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return fmt.Errorf("HA webhook must be an HTTP or HTTPS URL without embedded credentials")
	}
	payload, err := json.Marshal(map[string]string{
		"action": "power_on_for_credential_capture",
		"source": "iptv-refresh",
	})
	if err != nil {
		return fmt.Errorf("encode HA webhook request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create HA webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "iptv-refresh/ha-webhook")
	client := w.Client
	if client == nil {
		timeout := w.Timeout
		if timeout <= 0 {
			timeout = DefaultTimeout
		}
		client = &http.Client{
			Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	response, err := client.Do(req)
	if err != nil {
		if requestError, ok := err.(*url.Error); ok {
			err = requestError.Err
		}
		return fmt.Errorf("call HA webhook: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("HA webhook returned HTTP %d", response.StatusCode)
	}
	return nil
}
