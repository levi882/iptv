package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"iptv/internal/playlist"
	"iptv/internal/redact"
)

const defaultGuideWorkers = 4

type GuideOptions struct {
	Template string
	Now      time.Time
	Days     int
	Workers  int
}

type guideChannelResponse struct {
	TotalSize       int               `json:"totalSize"`
	ChannelDataList []guideAPIChannel `json:"channelDataList"`
}

type guideAPIChannel struct {
	Name   string `json:"channelName"`
	ID     string `json:"channelID"`
	Number string `json:"channelIndex"`
}

type guideProgrammeResponse struct {
	TotalSize         int                 `json:"totalSize"`
	ChannelPrevueList []guideAPIProgramme `json:"channelPrevueList"`
}

type guideAPIProgramme struct {
	ID        string `json:"prevuecode"`
	Title     string `json:"prevueName"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

type guideSecondAuthResponse struct {
	Token string `json:"iptvToken"`
	Code  string `json:"respCode"`
}

type guideCatchupResponse struct {
	PlayURL string `json:"playUrl"`
}

var (
	guideTemplateRE  = regexp.MustCompile(`^frame[0-9]+$`)
	secondAuthHostRE = regexp.MustCompile(`(?i)\bipport\s*=\s*["'](https?://[^"']+)["']`)
)

func (c *Client) getJSON(ctx context.Context, endpoint, referer string, output any) error {
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/xml, text/html, application/xhtml+xml, image/png, text/plain, */*;q=0.8")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	raw, err := readResponse(resp)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, output); err != nil {
		return fmt.Errorf("decode provider JSON: %w", err)
	}
	return nil
}

func (c *Client) authorizeGuide(ctx context.Context, host, userID string) error {
	authPage := strings.TrimRight(host, "/") + "/iptvepg/frame234/authBySecond.jsp"
	raw, finalURL, err := c.getSessionPage(ctx, authPage)
	if err != nil {
		return fmt.Errorf("fetch secondary authentication page: %w", err)
	}
	match := secondAuthHostRE.FindSubmatch(raw)
	if len(match) < 2 {
		return fmt.Errorf("secondary authentication service not found")
	}
	authHost, err := url.Parse(strings.TrimRight(string(match[1]), "/"))
	if err != nil || (authHost.Scheme != "http" && authHost.Scheme != "https") || authHost.Host == "" {
		return fmt.Errorf("invalid secondary authentication service")
	}
	authHost.Path = strings.TrimRight(authHost.Path, "/") + "/authZX/" + url.PathEscape(userID)
	var result guideSecondAuthResponse
	if err := c.getJSON(ctx, authHost.String(), finalURL, &result); err != nil {
		return fmt.Errorf("request secondary authentication: %w", err)
	}
	if result.Code != "" && result.Code != "00000" {
		return fmt.Errorf("secondary authentication rejected, code=%s", redact.Sensitive(result.Code))
	}
	if strings.TrimSpace(result.Token) == "" {
		return fmt.Errorf("secondary authentication returned no IPTV token")
	}
	portalURL, err := url.Parse(strings.TrimRight(host, "/") + "/")
	if err != nil {
		return fmt.Errorf("invalid guide host")
	}
	c.http.Jar.SetCookies(portalURL, []*http.Cookie{{Name: "iptvToken", Value: result.Token, Path: "/"}})
	return nil
}

// AuthorizeCatchup obtains the secondary IPTV token used by both schedule and
// TVOD endpoints. It is separate from FetchGuide so a long-running service can
// establish a replay session without downloading every channel schedule.
func (c *Client) AuthorizeCatchup(ctx context.Context, host, userID string) error {
	return c.authorizeGuide(ctx, host, userID)
}

func guideEndpoint(host, template, name string, query url.Values) string {
	endpoint := strings.TrimRight(host, "/") + "/iptvepg/" + template + "/publicPage/datajsp/" + name
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	return endpoint
}

// FetchCatchupURL exchanges the stable programme and channel identifiers from
// the operator guide for a short-lived RTSP TVOD URL.
func (c *Client) FetchCatchupURL(ctx context.Context, host, template, programmeID, channelID string) (string, error) {
	template = strings.TrimSpace(template)
	programmeID = strings.TrimSpace(programmeID)
	channelID = strings.TrimSpace(channelID)
	if !guideTemplateRE.MatchString(template) {
		return "", fmt.Errorf("invalid provider guide template %q", template)
	}
	if programmeID == "" || channelID == "" {
		return "", fmt.Errorf("provider programme and channel IDs are required")
	}
	query := url.Values{
		"programCode": {programmeID}, "channelID": {channelID},
		"isJson": {"-1"}, "isAjax": {"1"},
	}
	referer := strings.TrimRight(host, "/") + "/iptvepg/" + template + "/publicPage/channelBack/channelBack.jsp"
	var response guideCatchupResponse
	if err := c.getJSON(ctx, guideEndpoint(host, template, "getTVODPlayURL.jsp", query), referer, &response); err != nil {
		return "", fmt.Errorf("fetch operator TVOD URL: %w", err)
	}
	playURL := strings.TrimSpace(response.PlayURL)
	parsed, err := url.Parse(playURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "rtsp") || parsed.Host == "" {
		return "", fmt.Errorf("provider returned an invalid TVOD URL")
	}
	return playURL, nil
}

func (c *Client) fetchGuideChannels(ctx context.Context, host, template, referer string) ([]playlist.OperatorChannel, error) {
	query := url.Values{
		"categoryCode": {"0900"}, "pageIndex": {"1"}, "pageSize": {"999"},
		"isJson": {"-1"}, "isAjax": {"1"},
	}
	var response guideChannelResponse
	if err := c.getJSON(ctx, guideEndpoint(host, template, "channelToLiveFullScreen.jsp", query), referer, &response); err != nil {
		return nil, err
	}
	channels := make([]playlist.OperatorChannel, 0, len(response.ChannelDataList))
	seen := map[string]bool{}
	for _, item := range response.ChannelDataList {
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		if item.ID == "" || item.Name == "" || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		channels = append(channels, playlist.OperatorChannel{ID: item.ID, Name: item.Name, Number: strings.TrimSpace(item.Number)})
	}
	if len(channels) == 0 {
		return nil, fmt.Errorf("provider returned no guide channels")
	}
	if response.TotalSize > 0 && len(channels) != response.TotalSize {
		return nil, fmt.Errorf("provider guide channel list is incomplete: got %d of %d", len(channels), response.TotalSize)
	}
	return channels, nil
}

func (c *Client) fetchGuideProgrammes(ctx context.Context, host, template, referer, channelID string, day time.Time, firstDate int) ([]playlist.OperatorProgramme, error) {
	query := url.Values{
		"channelID": {channelID}, "curdate": {day.Format("20060102")}, "isFristDate": {fmt.Sprint(firstDate)},
		"pageIndex": {"1"}, "pageSize": {"999"}, "isJson": {"-1"}, "isAjax": {"1"},
	}
	var response guideProgrammeResponse
	if err := c.getJSON(ctx, guideEndpoint(host, template, "prevueList.jsp", query), referer, &response); err != nil {
		return nil, err
	}
	programmes := make([]playlist.OperatorProgramme, 0, len(response.ChannelPrevueList))
	for _, item := range response.ChannelPrevueList {
		start, err := time.ParseInLocation("2006.01.02 15:04:05", strings.TrimSpace(item.StartTime), day.Location())
		if err != nil {
			return nil, fmt.Errorf("invalid programme start time: %w", err)
		}
		stop, err := time.ParseInLocation("2006.01.02 15:04:05", strings.TrimSpace(item.EndTime), day.Location())
		if err != nil {
			return nil, fmt.Errorf("invalid programme stop time: %w", err)
		}
		if strings.TrimSpace(item.Title) == "" || !stop.After(start) {
			continue
		}
		programmes = append(programmes, playlist.OperatorProgramme{
			ID: strings.TrimSpace(item.ID), ChannelID: channelID, Title: strings.TrimSpace(item.Title), Start: start, Stop: stop,
		})
	}
	if response.TotalSize > 0 && len(programmes) != response.TotalSize {
		return nil, fmt.Errorf("provider programme list is incomplete: got %d of %d", len(programmes), response.TotalSize)
	}
	return programmes, nil
}

func (c *Client) FetchGuide(ctx context.Context, host, userID string, options GuideOptions) (playlist.OperatorGuide, error) {
	template := strings.TrimSpace(options.Template)
	if !guideTemplateRE.MatchString(template) {
		return playlist.OperatorGuide{}, fmt.Errorf("invalid provider guide template %q", template)
	}
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	if options.Days <= 0 || options.Days > 8 {
		return playlist.OperatorGuide{}, fmt.Errorf("provider guide days must be between 1 and 8")
	}
	workers := options.Workers
	if workers <= 0 {
		workers = defaultGuideWorkers
	}
	if err := c.authorizeGuide(ctx, host, userID); err != nil {
		return playlist.OperatorGuide{}, err
	}
	referer := strings.TrimRight(host, "/") + "/iptvepg/" + template + "/publicPage/index.jsp"
	channels, err := c.fetchGuideChannels(ctx, host, template, referer)
	if err != nil {
		return playlist.OperatorGuide{}, fmt.Errorf("fetch operator guide channels: %w", err)
	}

	type job struct {
		channelID string
		day       time.Time
		firstDate int
	}
	type result struct {
		programmes []playlist.OperatorProgramme
		err        error
	}
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan job)
	results := make(chan result)
	var wait sync.WaitGroup
	if workers > len(channels) {
		workers = len(channels)
	}
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for item := range jobs {
				programmes, err := c.fetchGuideProgrammes(workerCtx, host, template, referer, item.channelID, item.day, item.firstDate)
				select {
				case results <- result{programmes: programmes, err: err}:
				case <-workerCtx.Done():
					return
				}
				if err != nil {
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		dayStart := time.Date(options.Now.Year(), options.Now.Month(), options.Now.Day(), 0, 0, 0, 0, options.Now.Location())
		for _, channel := range channels {
			for offset := 0; offset < options.Days; offset++ {
				select {
				case jobs <- job{channelID: channel.ID, day: dayStart.AddDate(0, 0, -offset), firstDate: 7 - offset}:
				case <-workerCtx.Done():
					return
				}
			}
		}
	}()
	go func() {
		wait.Wait()
		close(results)
	}()

	guide := playlist.OperatorGuide{Channels: channels}
	var firstErr error
	for item := range results {
		if item.err != nil {
			if firstErr == nil {
				firstErr = item.err
				cancel()
			}
			continue
		}
		guide.Programmes = append(guide.Programmes, item.programmes...)
	}
	if firstErr != nil {
		return playlist.OperatorGuide{}, fmt.Errorf("fetch operator programmes: %w", firstErr)
	}
	if err := ctx.Err(); err != nil {
		return playlist.OperatorGuide{}, err
	}
	if len(guide.Programmes) == 0 {
		return playlist.OperatorGuide{}, fmt.Errorf("provider returned no programmes")
	}
	return guide, nil
}
