package playlist

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	channelRE = regexp.MustCompile(`(?s)jsSetConfig\('Channel','(.+?)'\);`)
	fieldRE   = regexp.MustCompile(`([A-Za-z0-9_]+)="([^"]*)"`)
)

func ParseChannels(text string, p URLSelectParams, lineRule, lineUHD, lineHD, lineSD string) []Channel {
	matches := channelRE.FindAllStringSubmatch(text, -1)
	rows := make([]Channel, 0, len(matches))
	for _, match := range matches {
		fields := map[string]string{}
		for _, field := range fieldRE.FindAllStringSubmatch(match[1], -1) {
			fields[field[1]] = field[2]
		}
		name := strings.TrimSpace(fields["ChannelName"])
		if name == "" {
			continue
		}
		timeshiftURL := strings.TrimSpace(fields["TimeShiftURL"])
		rawURL := SelectURL(
			strings.TrimSpace(fields["ChannelURL"]),
			timeshiftURL,
			strings.TrimSpace(fields["ChannelSDP"]),
			strings.TrimSpace(fields["ChannelFCCIP"]),
			strings.TrimSpace(fields["ChannelFCCPort"]),
			p,
		)
		rawURL = ApplyLineTag(rawURL, name, lineRule, lineUHD, lineHD, lineSD)
		if rawURL == "" {
			continue
		}
		seconds, _ := strconv.Atoi(strings.TrimSpace(fields["TimeShiftLength"]))
		days := 0
		if fields["TimeShift"] == "1" && timeshiftURL != "" {
			days = 1
			if seconds > 0 {
				days = (seconds + 86399) / 86400
			}
		}
		rows = append(rows, Channel{
			Name:            name,
			URL:             rawURL,
			UserChannelID:   strings.TrimSpace(fields["UserChannelID"]),
			TimeShiftURL:    timeshiftURL,
			TimeShiftDays:   days,
			TimeShiftLength: seconds,
		})
	}
	return rows
}

func SortChannels(channels []Channel, sortBy string) {
	if sortBy != "user_channel_id" {
		return
	}
	sort.SliceStable(channels, func(i, j int) bool {
		a, aErr := strconv.Atoi(channels[i].UserChannelID)
		b, bErr := strconv.Atoi(channels[j].UserChannelID)
		if aErr != nil && bErr != nil {
			return false
		}
		if aErr != nil {
			return false
		}
		if bErr != nil {
			return true
		}
		return a < b
	})
}
