package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const maxControlResponse = 2 << 20

func controlCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("control action is required: health, status, refresh, or playlist")
	}
	action := args[0]
	set := flag.NewFlagSet("control "+action, flag.ContinueOnError)
	host := set.String("host", "127.0.0.1", "service host")
	port := set.Int("port", defaultServicePort, "service port")
	tokenFile := set.String("token-file", "/etc/iptv-refresh/token", "API token file")
	iface := set.String("iface", "", "IPTV capture interface")
	if err := set.Parse(args[1:]); err != nil {
		return err
	}
	if set.NArg() != 0 {
		return fmt.Errorf("unexpected control arguments: %s", strings.Join(set.Args(), " "))
	}
	if *port < 1 || *port > 65535 {
		return fmt.Errorf("invalid service port %d", *port)
	}

	clientHost := *host
	switch clientHost {
	case "", "0.0.0.0", "::", "[::]":
		clientHost = "127.0.0.1"
	}
	baseURL := "http://" + net.JoinHostPort(clientHost, strconv.Itoa(*port))

	token := ""
	if action == "refresh" || action == "playlist" {
		raw, err := os.ReadFile(*tokenFile)
		if err != nil {
			return fmt.Errorf("read token file: %w", err)
		}
		token = strings.TrimSpace(string(raw))
		if token == "" || token == "change-me" {
			return errors.New("API token is missing or still uses the default value")
		}
	}

	client := &http.Client{Timeout: 15 * time.Second}
	body, err := controlRequest(client, baseURL, action, token, *iface)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(body)
	if err == nil && len(body) > 0 && body[len(body)-1] != '\n' {
		_, err = fmt.Fprintln(os.Stdout)
	}
	return err
}

func controlRequest(client *http.Client, baseURL, action, token, iface string) ([]byte, error) {
	if client == nil {
		client = http.DefaultClient
	}
	method := http.MethodGet
	path := ""
	switch action {
	case "health":
		path = "/healthz"
	case "status":
		path = "/status"
	case "refresh":
		method = http.MethodPost
		path = "/refresh"
	case "playlist":
		path = "/playlist"
	default:
		return nil, fmt.Errorf("unknown control action %q", action)
	}

	endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + path)
	if err != nil {
		return nil, fmt.Errorf("build control URL: %w", err)
	}
	if action == "refresh" && iface != "" {
		query := endpoint.Query()
		query.Set("iface", iface)
		endpoint.RawQuery = query.Encode()
	}
	var requestBody io.Reader
	if method == http.MethodPost {
		requestBody = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, endpoint.String(), requestBody)
	if err != nil {
		return nil, fmt.Errorf("create control request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call IPTV refresh service: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxControlResponse+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read control response: %w", err)
	}
	if len(body) > maxControlResponse {
		return nil, fmt.Errorf("control response exceeds %d bytes", maxControlResponse)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("service returned HTTP %s: %s", resp.Status, message)
	}
	return body, nil
}
