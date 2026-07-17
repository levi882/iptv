package playlist

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var attrRECache sync.Map
var tvgIDAttrRE = regexp.MustCompile(`tvg-id="[^"]*"`)
var tvgLogoAttrRE = regexp.MustCompile(`tvg-logo="[^"]*"`)

func setAttr(extinf, attr, value string, overwrite bool) string {
	if value == "" {
		return extinf
	}
	cached, ok := attrRECache.Load(attr)
	var re *regexp.Regexp
	if !ok {
		re = regexp.MustCompile(regexp.QuoteMeta(attr) + `="[^"]*"`)
		cached, _ = attrRECache.LoadOrStore(attr, re)
	}
	re = cached.(*regexp.Regexp)
	replacement := attr + `="` + value + `"`
	left, right, hasComma := strings.Cut(extinf, ",")
	if re.MatchString(left) {
		if overwrite {
			left = re.ReplaceAllString(left, replacement)
		}
	} else {
		left += " " + replacement
	}
	if hasComma {
		return left + "," + right
	}
	return left
}

func resolveGroup(name, tvgName string, groups map[string]string) string {
	for _, candidate := range []string{tvgName, name} {
		if group := groups[NormalizeName(candidate)]; group != "" {
			return group
		}
	}
	return InferGroupTitle(name, tvgName)
}

func RenderM3U(rows []Row, opts RenderOptions) string {
	header := "#EXTM3U"
	if opts.XTvgURL != "" {
		header += ` x-tvg-url="` + opts.XTvgURL + `"`
	}
	catchupType := opts.CatchupType
	if catchupType == "" {
		catchupType = "shift"
	}
	lines := []string{header}
	for _, row := range rows {
		tvgName := cleanTVGName(row.EPGName)
		if tvgName == "" {
			tvgName = cleanTVGName(row.Name)
		}
		displayName := row.Name
		if opts.DisplayNameMode == "tvg_name" && tvgName != "" {
			displayName = tvgName
		}
		group := resolveGroup(row.Name, tvgName, opts.GroupNames)
		timeshift, hasTimeshift := opts.TimeShiftLength[row.Name]
		catchup, hasCatchup := opts.Catchup[row.Name]
		catchupAttrs := ""
		if hasCatchup {
			catchupAttrs = fmt.Sprintf(` catchup="%s" catchup-days="%d" catchup-source="%s"`, catchupType, catchup.Days, catchup.Source)
		}

		if row.Ref != nil && row.Ref.EXTINF != "" {
			ext := row.Ref.EXTINF
			if tvgIDAttrRE.MatchString(ext) {
				ext = tvgIDAttrRE.ReplaceAllString(ext, `tvg-id="`+row.EPGID+`"`)
			} else if row.EPGID != "" {
				ext = setAttr(ext, "tvg-id", row.EPGID, true)
			} else {
				ext = strings.Replace(ext, "#EXTINF:-1", `#EXTINF:-1 tvg-id=""`, 1)
			}
			if tvgLogoAttrRE.MatchString(ext) {
				if row.LogoURL != "" {
					ext = tvgLogoAttrRE.ReplaceAllString(ext, `tvg-logo="`+row.LogoURL+`"`)
				}
			} else if row.LogoURL != "" {
				ext = setAttr(ext, "tvg-logo", row.LogoURL, true)
			} else {
				ext = strings.Replace(ext, "#EXTINF:-1", `#EXTINF:-1 tvg-logo=""`, 1)
			}
			ext = setAttr(ext, "tvg-name", tvgName, false)
			ext = setAttr(ext, "group-title", group, false)
			if hasTimeshift {
				ext = setAttr(ext, "x-r2h-timeshift-length", strconv.Itoa(timeshift), true)
			}
			if hasCatchup && !strings.Contains(ext, `catchup=`) {
				if pos := strings.LastIndex(ext, ","); pos >= 0 {
					ext = ext[:pos] + catchupAttrs + ext[pos:]
				}
			}
			if left, _, ok := strings.Cut(ext, ","); ok {
				ext = left + "," + displayName
			}
			lines = append(lines, ext)
			lines = append(lines, row.Ref.Options...)
		} else {
			extra := ""
			if hasTimeshift {
				extra = fmt.Sprintf(` x-r2h-timeshift-length="%d"`, timeshift)
			}
			lines = append(lines, fmt.Sprintf(`#EXTINF:-1 tvg-id="%s" tvg-name="%s" tvg-logo="%s" group-title="%s"%s%s,%s`, row.EPGID, tvgName, row.LogoURL, group, extra, catchupAttrs, displayName))
		}
		lines = append(lines, row.URL)
	}
	return strings.Join(lines, "\n") + "\n"
}

func RenderTXT(rows []Row) string {
	var b strings.Builder
	for _, row := range rows {
		b.WriteString(row.Name)
		b.WriteByte(',')
		b.WriteString(row.URL)
		b.WriteByte('\n')
	}
	return b.String()
}

func ChannelsToRows(channels []Channel) ([]Row, map[string]Catchup, map[string]int) {
	rows := make([]Row, 0, len(channels))
	catchup := map[string]Catchup{}
	lengths := map[string]int{}
	seen := map[string]bool{}
	for _, channel := range channels {
		key := channel.Name + "\x00" + channel.URL
		if seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, Row{Name: channel.Name, URL: channel.URL})
		if channel.TimeShiftLength > 0 {
			lengths[channel.Name] = channel.TimeShiftLength
		}
		if channel.TimeShiftDays > 0 {
			catchup[channel.Name] = Catchup{Source: channel.TimeShiftURL, Days: channel.TimeShiftDays}
		}
	}
	return rows, catchup, lengths
}
