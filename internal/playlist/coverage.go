package playlist

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"
)

const maxExpandedCoverageBytes = 128 << 20

// EPGCoverage summarizes the time range that can be determined from an XMLTV
// document. Latest is the latest valid programme stop time, or its start time
// when no valid stop time is present.
type EPGCoverage struct {
	Programmes int
	Latest     time.Time
}

// InspectEPGCoverage validates an XMLTV document and finds its latest
// programme time without retaining the full document in memory.
func InspectEPGCoverage(raw []byte, defaultLocation *time.Location) (EPGCoverage, error) {
	if defaultLocation == nil {
		defaultLocation = time.Local
	}
	var input io.Reader = bytes.NewReader(raw)
	var compressed *gzip.Reader
	if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		var err error
		compressed, err = gzip.NewReader(input)
		if err != nil {
			return EPGCoverage{}, fmt.Errorf("open compressed XMLTV: %w", err)
		}
		defer compressed.Close()
		input = compressed
	}

	limited := &io.LimitedReader{R: input, N: maxExpandedCoverageBytes + 1}
	decoder := xml.NewDecoder(limited)
	coverage := EPGCoverage{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			if limited.N <= 0 {
				return EPGCoverage{}, fmt.Errorf("expanded XMLTV exceeds %d MiB", maxExpandedCoverageBytes>>20)
			}
			return EPGCoverage{}, fmt.Errorf("parse XMLTV coverage: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "programme" {
			continue
		}
		coverage.Programmes++
		var latest time.Time
		for _, attr := range start.Attr {
			if attr.Name.Local != "start" && attr.Name.Local != "stop" {
				continue
			}
			value, err := parseXMLTVTime(attr.Value, defaultLocation)
			if err == nil && value.After(latest) {
				latest = value
			}
		}
		if latest.After(coverage.Latest) {
			coverage.Latest = latest
		}
	}
	if limited.N <= 0 {
		return EPGCoverage{}, fmt.Errorf("expanded XMLTV exceeds %d MiB", maxExpandedCoverageBytes>>20)
	}
	if coverage.Programmes == 0 {
		return EPGCoverage{}, fmt.Errorf("XMLTV contains no programmes")
	}
	if coverage.Latest.IsZero() {
		return EPGCoverage{}, fmt.Errorf("XMLTV contains no valid programme times")
	}
	return coverage, nil
}

func parseXMLTVTime(value string, defaultLocation *time.Location) (time.Time, error) {
	fields := strings.Fields(value)
	if len(fields) == 0 || len(fields[0]) < 8 {
		return time.Time{}, fmt.Errorf("invalid XMLTV time %q", value)
	}
	stamp := fields[0]
	layout := "20060102"
	switch {
	case len(stamp) >= 14:
		stamp = stamp[:14]
		layout = "20060102150405"
	case len(stamp) >= 12:
		stamp = stamp[:12]
		layout = "200601021504"
	case len(stamp) >= 10:
		stamp = stamp[:10]
		layout = "2006010215"
	default:
		stamp = stamp[:8]
	}
	if len(fields) > 1 {
		return time.Parse(layout+" -0700", stamp+" "+fields[1])
	}
	return time.ParseInLocation(layout, stamp, defaultLocation)
}
