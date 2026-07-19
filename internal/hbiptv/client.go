package hbiptv

import (
	"context"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"iptv/internal/netbind"
	"iptv/internal/redact"

	"golang.org/x/text/encoding/simplifiedchinese"
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
	STBType       string
	PRMID         string
	DRMSupplier   string
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

type portalSession struct {
	Host    string
	Referer string
}

var (
	userTokenRE   = regexp.MustCompile(`(?i)UserToken=([^\s&<]+)`)
	configTokenRE = regexp.MustCompile(`CTCSetConfig\('UserToken','([^']+)'\)`)
	errCodeRE     = regexp.MustCompile(`(?i)errcode\s*=\s*(\d+)`)
	epgRedirectRE = regexp.MustCompile(`(?i)(https?://[a-z0-9.-]+(?::[0-9]+)?/iptvepg/function/index\.jsp(?:\?[^"'<>[:space:]]*)?)`)
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
	data, err = decodeResponseBody(data, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := redact.Sensitive(strings.TrimSpace(string(data[:min(len(data), 300)])))
		return nil, fmt.Errorf("HTTP %s: %s", resp.Status, body)
	}
	return data, nil
}

func decodeResponseBody(data []byte, contentType string) ([]byte, error) {
	charset := ""
	if _, parameters, err := mime.ParseMediaType(contentType); err == nil {
		charset = strings.ToLower(strings.TrimSpace(parameters["charset"]))
	}
	var decoderName string
	var decoded []byte
	var err error
	switch charset {
	case "gbk", "gb2312", "gb_2312-80", "x-gbk", "cp936", "windows-936":
		decoderName = "GBK"
		decoded, err = simplifiedchinese.GBK.NewDecoder().Bytes(data)
	case "gb18030":
		decoderName = "GB18030"
		decoded, err = simplifiedchinese.GB18030.NewDecoder().Bytes(data)
	default:
		if utf8.Valid(data) {
			return data, nil
		}
		// Some provider nodes omit the charset even though their portal pages
		// are GBK/GB18030. GB18030 is a superset and is safe for this fallback.
		decoderName = "GB18030"
		decoded, err = simplifiedchinese.GB18030.NewDecoder().Bytes(data)
	}
	if err != nil {
		return nil, fmt.Errorf("decode provider response as %s: %w", decoderName, err)
	}
	if !utf8.Valid(decoded) {
		return nil, fmt.Errorf("decode provider response as %s: invalid UTF-8 result", decoderName)
	}
	return decoded, nil
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

func origin(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("invalid EPG session URL")
	}
	return u.Scheme + "://" + u.Host, nil
}

func pageRedirect(data []byte) string {
	// Some set-top box pages escape URL slashes for JavaScript and HTML-encode
	// query separators. Normalize both forms before following the exact URL.
	normalized := strings.ReplaceAll(string(data), `\/`, "/")
	match := epgRedirectRE.FindStringSubmatch(normalized)
	if len(match) < 2 {
		return ""
	}
	return html.UnescapeString(match[1])
}

func (c *Client) getSessionPage(ctx context.Context, endpoint string) ([]byte, string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	finalURL := resp.Request.URL.String()
	data, err := readResponse(resp)
	return data, finalURL, err
}

func (c *Client) initSession(ctx context.Context, entry, token string, creds Credentials) (portalSession, error) {
	u, err := url.Parse(strings.TrimRight(entry, "/") + "/iptvepg/function/index.jsp")
	if err != nil {
		return portalSession{}, err
	}
	q := u.Query()
	q.Set("UserToken", token)
	q.Set("UserID", creds.UserID)
	q.Set("STBID", creds.STBID)
	q.Set("LastTermno", "")
	q.Set("easip", c.config.EASIP)
	q.Set("networkid", c.config.NetworkID)
	u.RawQuery = q.Encode()
	current := u.String()
	seen := map[string]bool{}
	for range 4 {
		if seen[current] {
			return portalSession{}, fmt.Errorf("EPG session page redirect loop")
		}
		seen[current] = true
		data, finalURL, err := c.getSessionPage(ctx, current)
		if err != nil {
			return portalSession{}, err
		}
		host, err := origin(finalURL)
		if err != nil {
			return portalSession{}, err
		}
		next := pageRedirect(data)
		if next == "" || next == finalURL || seen[next] {
			return portalSession{Host: host, Referer: finalURL}, nil
		}
		// ZTE portals often return a 200 JavaScript page rather than an HTTP
		// redirect. Follow that exact URL so the load-balanced host can create
		// its own cookies before funcportalauth.jsp is submitted.
		current = next
	}
	return portalSession{}, fmt.Errorf("too many EPG session page redirects")
}

func (c *Client) portalAuth(ctx context.Context, session portalSession, token string, creds Credentials) error {
	stbType := creds.STBType
	if stbType == "" {
		stbType = "B860AV1.1-T2"
	}
	form := url.Values{
		"UserToken": {token}, "UserID": {creds.UserID}, "STBID": {creds.STBID},
		"stbinfo": {creds.STBInfo}, "prmid": {creds.PRMID}, "easip": {c.config.EASIP},
		"networkid": {c.config.NetworkID}, "stbtype": {stbType}, "drmsupplier": {creds.DRMSupplier},
	}
	req, err := c.newRequest(ctx, http.MethodPost, session.Host+"/iptvepg/function/funcportalauth.jsp", form)
	if err != nil {
		return err
	}
	req.Header.Set("Origin", session.Host)
	req.Header.Set("Referer", session.Referer)
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
		session, err := c.initSession(ctx, entry, token, creds)
		stage := "initialize session"
		if err == nil {
			stage = "authenticate portal"
			err = c.portalAuth(ctx, session, token, creds)
		}
		if err == nil {
			stage = "fetch channel list"
			var frameset string
			frameset, err = c.frameset(ctx, session.Host)
			if err == nil {
				return Result{Frameset: frameset, EPGHost: session.Host, Token: token}, nil
			}
		}
		lastErr = fmt.Errorf("EPG entry %s: %s: %s", redact.Sensitive(entry), stage, redact.Sensitive(err.Error()))
	}
	return Result{}, fmt.Errorf("all EPG entries failed: %w", lastErr)
}
