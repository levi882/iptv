package playlist

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
)

type EPGName struct {
	ID        string
	Canonical string
}

type xmlTV struct {
	Channels []xmlChannel `xml:"channel"`
}

type xmlChannel struct {
	ID           string   `xml:"id,attr"`
	DisplayNames []string `xml:"display-name"`
}

func ParseEPG(raw []byte) (map[string]EPGName, error) {
	if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		decompressed, err := io.ReadAll(zr)
		closeErr := zr.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		raw = decompressed
	}
	var document xmlTV
	if err := xml.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("parse XMLTV: %w", err)
	}
	out := map[string]EPGName{}
	for _, channel := range document.Channels {
		canonical := ""
		for _, name := range channel.DisplayNames {
			if normalized := NormalizeName(name); normalized != "" {
				if canonical == "" {
					canonical = name
				}
				if _, exists := out[normalized]; !exists {
					out[normalized] = EPGName{ID: channel.ID, Canonical: canonical}
				}
			}
		}
	}
	return out, nil
}

func AttachEPG(rows []Row, names map[string]EPGName, replaceName bool) (mapped, replaced int) {
	for i := range rows {
		entry, ok := names[NormalizeName(rows[i].Name)]
		if !ok {
			continue
		}
		mapped++
		rows[i].EPGID = entry.ID
		rows[i].EPGName = entry.Canonical
		if replaceName && entry.Canonical != "" && rows[i].Name != entry.Canonical {
			rows[i].Name = entry.Canonical
			replaced++
		}
	}
	return mapped, replaced
}
