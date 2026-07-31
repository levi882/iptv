package playlist

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	OperatorEPGGenerator = "iptv-refresh/operator"
	maxOperatorEPGBytes  = 128 << 20
)

type OperatorChannel struct {
	ID     string
	Name   string
	Number string
}

type OperatorProgramme struct {
	ID        string
	ChannelID string
	Title     string
	Start     time.Time
	Stop      time.Time
}

type OperatorGuide struct {
	Channels   []OperatorChannel
	Programmes []OperatorProgramme
}

type operatorXMLTV struct {
	XMLName           xml.Name               `xml:"tv"`
	GeneratorInfoName string                 `xml:"generator-info-name,attr,omitempty"`
	SourceInfoName    string                 `xml:"source-info-name,attr,omitempty"`
	Channels          []operatorXMLChannel   `xml:"channel"`
	Programmes        []operatorXMLProgramme `xml:"programme"`
}

type operatorXMLChannel struct {
	ID           string            `xml:"id,attr"`
	DisplayNames []operatorXMLText `xml:"display-name"`
}

type operatorXMLProgramme struct {
	Start      string              `xml:"start,attr"`
	Stop       string              `xml:"stop,attr"`
	Channel    string              `xml:"channel,attr"`
	CatchupID  string              `xml:"catchup-id,attr,omitempty"`
	Titles     []operatorXMLText   `xml:"title"`
	EpisodeNum *operatorXMLEpisode `xml:"episode-num,omitempty"`
}

type operatorXMLText struct {
	Lang  string `xml:"lang,attr,omitempty"`
	Value string `xml:",chardata"`
}

type operatorXMLEpisode struct {
	System string `xml:"system,attr,omitempty"`
	Value  string `xml:",chardata"`
}

func normalizeOperatorGuide(guide OperatorGuide) OperatorGuide {
	channels := make([]OperatorChannel, 0, len(guide.Channels))
	channelIDs := map[string]bool{}
	for _, channel := range guide.Channels {
		channel.ID = strings.TrimSpace(channel.ID)
		channel.Name = strings.TrimSpace(channel.Name)
		channel.Number = strings.TrimSpace(channel.Number)
		if channel.ID == "" || channel.Name == "" || channelIDs[channel.ID] {
			continue
		}
		channelIDs[channel.ID] = true
		channels = append(channels, channel)
	}
	sort.SliceStable(channels, func(i, j int) bool {
		a, aErr := strconv.Atoi(channels[i].Number)
		b, bErr := strconv.Atoi(channels[j].Number)
		switch {
		case aErr == nil && bErr == nil && a != b:
			return a < b
		case aErr == nil && bErr != nil:
			return true
		case aErr != nil && bErr == nil:
			return false
		case channels[i].Number != channels[j].Number:
			return channels[i].Number < channels[j].Number
		default:
			return channels[i].ID < channels[j].ID
		}
	})

	order := make(map[string]int, len(channels))
	for index, channel := range channels {
		order[channel.ID] = index
	}
	programmes := make([]OperatorProgramme, 0, len(guide.Programmes))
	seen := map[string]bool{}
	for _, programme := range guide.Programmes {
		programme.ID = strings.TrimSpace(programme.ID)
		programme.ChannelID = strings.TrimSpace(programme.ChannelID)
		programme.Title = strings.TrimSpace(programme.Title)
		if !channelIDs[programme.ChannelID] || programme.Title == "" || programme.Start.IsZero() || !programme.Stop.After(programme.Start) {
			continue
		}
		key := programme.ChannelID + "\x00" + programme.Start.Format(time.RFC3339Nano) + "\x00" + programme.Stop.Format(time.RFC3339Nano) + "\x00" + programme.Title
		if seen[key] {
			continue
		}
		seen[key] = true
		programmes = append(programmes, programme)
	}
	sort.SliceStable(programmes, func(i, j int) bool {
		a, b := order[programmes[i].ChannelID], order[programmes[j].ChannelID]
		if a != b {
			return a < b
		}
		if !programmes[i].Start.Equal(programmes[j].Start) {
			return programmes[i].Start.Before(programmes[j].Start)
		}
		if !programmes[i].Stop.Equal(programmes[j].Stop) {
			return programmes[i].Stop.Before(programmes[j].Stop)
		}
		return programmes[i].Title < programmes[j].Title
	})
	return OperatorGuide{Channels: channels, Programmes: programmes}
}

func RenderOperatorEPG(guide OperatorGuide) ([]byte, error) {
	guide = normalizeOperatorGuide(guide)
	document := operatorXMLTV{GeneratorInfoName: OperatorEPGGenerator, SourceInfoName: "operator portal"}
	for _, channel := range guide.Channels {
		xmlChannel := operatorXMLChannel{ID: channel.ID, DisplayNames: []operatorXMLText{{Lang: "zh", Value: channel.Name}}}
		document.Channels = append(document.Channels, xmlChannel)
	}
	for _, programme := range guide.Programmes {
		xmlProgramme := operatorXMLProgramme{
			Start:     programme.Start.Format("20060102150405 -0700"),
			Stop:      programme.Stop.Format("20060102150405 -0700"),
			Channel:   programme.ChannelID,
			CatchupID: programme.ID,
			Titles:    []operatorXMLText{{Lang: "zh", Value: programme.Title}},
		}
		if programme.ID != "" {
			xmlProgramme.EpisodeNum = &operatorXMLEpisode{System: "operator", Value: programme.ID}
		}
		document.Programmes = append(document.Programmes, xmlProgramme)
	}
	raw, err := xml.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("render operator XMLTV: %w", err)
	}
	return append(append([]byte(xml.Header), raw...), '\n'), nil
}

func expandedOperatorEPG(raw []byte) ([]byte, error) {
	if len(raw) < 2 || raw[0] != 0x1f || raw[1] != 0x8b {
		return raw, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("open operator XMLTV: %w", err)
	}
	expanded, readErr := io.ReadAll(io.LimitReader(reader, maxOperatorEPGBytes+1))
	closeErr := reader.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read operator XMLTV: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close operator XMLTV: %w", closeErr)
	}
	if len(expanded) > maxOperatorEPGBytes {
		return nil, fmt.Errorf("operator XMLTV exceeds %d MiB", maxOperatorEPGBytes>>20)
	}
	return expanded, nil
}

func ParseOperatorEPG(raw []byte, location *time.Location) (OperatorGuide, bool, error) {
	if len(raw) == 0 {
		return OperatorGuide{}, false, nil
	}
	expanded, err := expandedOperatorEPG(raw)
	if err != nil {
		return OperatorGuide{}, false, err
	}
	var document operatorXMLTV
	if err := xml.Unmarshal(expanded, &document); err != nil {
		return OperatorGuide{}, false, fmt.Errorf("parse operator XMLTV: %w", err)
	}
	if document.GeneratorInfoName != OperatorEPGGenerator {
		return OperatorGuide{}, false, nil
	}
	if location == nil {
		location = time.Local
	}
	guide := OperatorGuide{}
	for _, channel := range document.Channels {
		name := ""
		for _, displayName := range channel.DisplayNames {
			if value := strings.TrimSpace(displayName.Value); value != "" {
				name = value
				break
			}
		}
		guide.Channels = append(guide.Channels, OperatorChannel{ID: channel.ID, Name: name})
	}
	for _, programme := range document.Programmes {
		start, err := parseOperatorXMLTVTime(programme.Start, location)
		if err != nil {
			return OperatorGuide{}, false, err
		}
		stop, err := parseOperatorXMLTVTime(programme.Stop, location)
		if err != nil {
			return OperatorGuide{}, false, err
		}
		title := ""
		for _, item := range programme.Titles {
			if value := strings.TrimSpace(item.Value); value != "" {
				title = value
				break
			}
		}
		id := ""
		if programme.EpisodeNum != nil && programme.EpisodeNum.System == "operator" {
			id = programme.EpisodeNum.Value
		}
		if id == "" {
			id = programme.CatchupID
		}
		guide.Programmes = append(guide.Programmes, OperatorProgramme{ID: id, ChannelID: programme.Channel, Title: title, Start: start, Stop: stop})
	}
	return normalizeOperatorGuide(guide), true, nil
}

func parseOperatorXMLTVTime(value string, location *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"20060102150405 -0700", "20060102150405", "200601021504"} {
		var parsed time.Time
		var err error
		if strings.Contains(layout, "-0700") {
			parsed, err = time.Parse(layout, value)
		} else {
			parsed, err = time.ParseInLocation(layout, value, location)
		}
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid operator XMLTV time %q", value)
}

func MergeOperatorEPG(existing, fresh OperatorGuide, now time.Time, historyDays int) OperatorGuide {
	if historyDays < 0 {
		historyDays = 0
	}
	location := now.Location()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	cutoff := dayStart.AddDate(0, 0, -historyDays)
	currentChannels := map[string]bool{}
	for _, channel := range fresh.Channels {
		currentChannels[channel.ID] = true
	}
	merged := OperatorGuide{Channels: append([]OperatorChannel(nil), fresh.Channels...)}
	for _, programme := range existing.Programmes {
		if currentChannels[programme.ChannelID] && programme.Stop.After(cutoff) && programme.Start.Before(dayStart) {
			merged.Programmes = append(merged.Programmes, programme)
		}
	}
	for _, programme := range fresh.Programmes {
		if currentChannels[programme.ChannelID] && programme.Stop.After(cutoff) {
			merged.Programmes = append(merged.Programmes, programme)
		}
	}
	return normalizeOperatorGuide(merged)
}

func AttachOperatorEPG(rows []Row, guide OperatorGuide) int {
	channels := make(map[string]OperatorChannel, len(guide.Channels))
	for _, channel := range guide.Channels {
		channels[channel.ID] = channel
	}
	mapped := 0
	for index := range rows {
		channel, ok := channels[rows[index].ProviderID]
		if !ok {
			continue
		}
		rows[index].EPGID = channel.ID
		rows[index].EPGName = channel.Name
		mapped++
	}
	return mapped
}
