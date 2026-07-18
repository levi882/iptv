package hbiptv

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"iptv/internal/netbind"
	"iptv/internal/redact"
)

const DefaultUserAgent = "B700-V2A|Mozilla|5.0|ztebw(Chrome)|1.2.0;Resolution(PAL,720p,1080i) AppleWebKit/535.7 (KHTML, like Gecko) Chrome/16.0.912.63 Safari/535.7"

type Config struct {
	TokenServer    string
	PlatformOrigin string
	EPGEntry       string
	EPGFallbacks   []string
	EASIP          string
	NetworkID      string
	CityCode       string
	UserAgent      string
	BindInterface  string
	BindSourceIP   string
	Timeout        time.Duration
}

type Credentials struct {
	UserID        string
	STBID         string
	Authenticator string
	STBInfo       string
	UserToken     string
}

type Result struct {
	Frameset string
	EPGHost  string
	Token    string
}

type Client struct {
	config Config
	http   *http.Client
}

var (
	userTokenRE   = regexp.MustCompile(`(?i)UserToken=([^\s&<]+)`)
	configTokenRE = regexp.MustCompile(`CTCSetConfig\('UserToken','([^']+)'\)`)
	errCodeRE     = regexp.MustCompile(`(?i)errcode\s*=\s*(\d+)`)
	epgHostRE     = regexp.MustCompile(`(?i)(https?://[a-z0-9.-]+(?::[0-9]+)?)/iptvepg`)
)

func New(config Config) (*Client, error) {
	if config.Timeout <= 0 {
		config.Timeout = 20 * time.Second
	}
	if config.UserAgent == "" {
		config.UserAgent = DefaultUserAgent
	}
	dialer, err := netbind.Dialer(config.BindInterface, config.BindSourceIP, config.Timeout)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialer.DialContext
	transport.ResponseHeaderTimeout = config.Timeout
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Client{
		config: config,
		http: &http.Client{
			Transport: transport,
			Jar:       jar,
			Timeout:   config.Timeout,
		},
	}, nil
}

func (c *Client) resetCookies() error {
	jar, err := cookiejar.New(nil)
	if err == nil {
		c.http.Jar = jar
	}
	return err
}

func (c *Client) newRequest(ctx context.Context, method, endpoint string, form url.Values) (*http.Request, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.config.UserAgent)
	req.Header.Set("Accept-Language", "zh-cn")
	req.Header.Set("Accept-Charset", "utf-8, iso-8859-1, utf-16, *;q=0.7")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return req, nil
}

func readResponse(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := redact.Sensitive(strings.TrimSpace(string(data[:min(len(data), 300)])))
		return nil, fmt.Errorf("HTTP %s: %s", resp.Status, body)
	}
	return data, nil
}

func (c *Client) userToken(ctx context.Context, creds Credentials) (string, error) {
	form := url.Values{"UserID": {creds.UserID}, "Authenticator": {creds.Authenticator}}
	if c.config.CityCode != "" {
		form.Set("citycode", c.config.CityCode)
	}
	endpoint := strings.TrimRight(c.config.TokenServer, "/") + "/GetUserToken"
	req, err := c.newRequest(ctx, http.MethodPost, endpoint, form)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Referer", strings.TrimRight(c.config.PlatformOrigin, "/")+"/iptvepg/platform/index.jsp?UserID="+url.QueryEscape(creds.UserID)+"&Action=Login&FCCSupport=1")
	req.Header.Set("Origin", strings.TrimRight(c.config.PlatformOrigin, "/"))
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	data, err := readResponse(resp)
	if err != nil {
		return "", err
	}
	if match := userTokenRE.FindSubmatch(data); match != nil {
		return strings.TrimSpace(string(match[1])), nil
	}
	for _, cookie := range resp.Cookies() {
		if strings.EqualFold(cookie.Name, "UserToken") && cookie.Value != "" {
			return cookie.Value, nil
		}
	}
	if match := configTokenRE.FindSubmatch(data); match != nil {
		return strings.TrimSpace(string(match[1])), nil
	}
	if match := errCodeRE.FindSubmatch(data); match != nil {
		return "", fmt.Errorf("UserToken not found, provider errcode=%s", match[1])
	}
	return "", fmt.Errorf("UserToken not found in provider response")
}

func (c *Client) initSession(ctx context.Context, entry, token string, creds Credentials) (string, error) {
	u, err := url.Parse(strings.TrimRight(entry, "/") + "/iptvepg/function/index.jsp")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("UserToken", token)
	q.Set("UserID", creds.UserID)
	q.Set("STBID", creds.STBID)
	q.Set("LastTermno", "")
	q.Set("easip", c.config.EASIP)
	q.Set("networkid", c.config.NetworkID)
	u.RawQuery = q.Encode()
	req, err := c.newRequest(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	data, err := readResponse(resp)
	if err != nil {
		return "", err
	}
	finalURL := resp.Request.URL.String()
	if before, _, ok := strings.Cut(finalURL, "/iptvepg"); ok {
		if before != strings.TrimRight(entry, "/") {
			return before, nil
		}
	}
	// ZTE portals commonly return a 200 HTML/JavaScript page which points the
	// STB at a load-balanced EPG host instead of issuing an HTTP redirect.
	if match := epgHostRE.FindSubmatch(data); match != nil {
		return strings.TrimRight(string(match[1]), "/"), nil
	}
	return strings.TrimRight(entry, "/"), nil
}

func (c *Client) portalAuth(ctx context.Context, host, token string, creds Credentials) error {
	form := url.Values{
		"UserToken": {token}, "UserID": {creds.UserID}, "STBID": {creds.STBID},
		"stbinfo": {creds.STBInfo}, "prmid": {""}, "easip": {c.config.EASIP},
		"networkid": {c.config.NetworkID}, "stbtype": {"B860AV1.1-T2"}, "drmsupplier": {""},
	}
	req, err := c.newRequest(ctx, http.MethodPost, host+"/iptvepg/function/funcportalauth.jsp", form)
	if err != nil {
		return err
	}
	req.Header.Set("Origin", host)
	req.Header.Set("Referer", host+"/iptvepg/function/index.jsp?loadbalanced=1&UserIP=&UserID="+url.QueryEscape(creds.UserID)+"&UserToken="+url.QueryEscape(token)+"&STBID="+url.QueryEscape(creds.STBID)+"&LastTermno=&easip="+url.QueryEscape(c.config.EASIP)+"&networkid="+url.QueryEscape(c.config.NetworkID))
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	_, err = readResponse(resp)
	return err
}

func (c *Client) frameset(ctx context.Context, host string) (string, error) {
	form := url.Values{"MAIN_WIN_SRC": {"/iptvepg/frame234/portal.jsp"}, "NEED_UPDATE_STB": {"1"}, "BUILD_ACTION": {"FRAMESET_BUILDER"}, "hdmistatus": {""}}
	req, err := c.newRequest(ctx, http.MethodPost, host+"/iptvepg/function/frameset_builder.jsp", form)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	data, err := readResponse(resp)
	return string(data), err
}

func uniqueEntries(primary string, fallbacks []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, entry := range append([]string{primary}, fallbacks...) {
		entry = strings.TrimRight(strings.TrimSpace(entry), "/")
		if entry != "" && !seen[entry] {
			seen[entry] = true
			result = append(result, entry)
		}
	}
	return result
}

func (c *Client) Fetch(ctx context.Context, creds Credentials) (Result, error) {
	if creds.UserID == "" || creds.STBID == "" || creds.STBInfo == "" {
		return Result{}, fmt.Errorf("UserID, STBID and STBInfo are required")
	}
	token := strings.TrimSpace(creds.UserToken)
	if token == "" {
		if creds.Authenticator == "" {
			return Result{}, fmt.Errorf("Authenticator is required when UserToken is empty")
		}
		var err error
		token, err = c.userToken(ctx, creds)
		if err != nil {
			return Result{}, fmt.Errorf("get user token: %w", err)
		}
	}
	var lastErr error
	for _, entry := range uniqueEntries(c.config.EPGEntry, c.config.EPGFallbacks) {
		if err := c.resetCookies(); err != nil {
			return Result{}, err
		}
		host, err := c.initSession(ctx, entry, token, creds)
		stage := "initialize session"
		if err == nil {
			stage = "authenticate portal"
			err = c.portalAuth(ctx, host, token, creds)
		}
		if err == nil {
			stage = "fetch channel list"
			var frameset string
			frameset, err = c.frameset(ctx, host)
			if err == nil {
				return Result{Frameset: frameset, EPGHost: host, Token: token}, nil
			}
		}
		lastErr = fmt.Errorf("EPG entry %s: %s: %s", redact.Sensitive(entry), stage, redact.Sensitive(err.Error()))
	}
	return Result{}, fmt.Errorf("all EPG entries failed: %w", lastErr)
}
