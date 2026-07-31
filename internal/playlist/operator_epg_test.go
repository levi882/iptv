package playlist

import (
	"strings"
	"testing"
	"time"
)

func TestOperatorEPGRoundTripSortsAndMapsByProviderID(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	guide := OperatorGuide{
		Channels: []OperatorChannel{
			{ID: "channel-20", Name: "二十频道", Number: "20"},
			{ID: "channel-2", Name: "二频道", Number: "2"},
		},
		Programmes: []OperatorProgramme{
			{ID: "p2", ChannelID: "channel-2", Title: "第二档", Start: time.Date(2026, 7, 31, 1, 0, 0, 0, location), Stop: time.Date(2026, 7, 31, 2, 0, 0, 0, location)},
			{ID: "p1", ChannelID: "channel-2", Title: "第一档", Start: time.Date(2026, 7, 31, 0, 0, 0, 0, location), Stop: time.Date(2026, 7, 31, 1, 0, 0, 0, location)},
		},
	}
	raw, err := RenderOperatorEPG(guide)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `catchup-id="p1"`) {
		t.Fatalf("operator XMLTV does not preserve catch-up IDs:\n%s", text)
	}
	if strings.Index(text, `channel id="channel-2"`) > strings.Index(text, `channel id="channel-20"`) || strings.Index(text, "第一档") > strings.Index(text, "第二档") {
		t.Fatalf("operator XMLTV was not sorted:\n%s", text)
	}
	parsed, recognized, err := ParseOperatorEPG(raw, location)
	if err != nil || !recognized {
		t.Fatalf("ParseOperatorEPG recognized=%v err=%v", recognized, err)
	}
	if len(parsed.Channels) != 2 || len(parsed.Programmes) != 2 || parsed.Programmes[0].ID != "p1" {
		t.Fatalf("round trip = %#v", parsed)
	}
	rows := []Row{{ProviderID: "channel-2", Name: "旧名称"}, {ProviderID: "missing", Name: "无节目单"}}
	if mapped := AttachOperatorEPG(rows, parsed); mapped != 1 || rows[0].EPGID != "channel-2" || rows[0].EPGName != "二频道" || rows[1].EPGID != "" {
		t.Fatalf("mapped=%d rows=%#v", mapped, rows)
	}
}

func TestMergeOperatorEPGKeepsHistoryAndReplacesToday(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, location)
	channel := OperatorChannel{ID: "one", Name: "一套", Number: "1"}
	programme := func(id string, start time.Time) OperatorProgramme {
		return OperatorProgramme{ID: id, ChannelID: "one", Title: id, Start: start, Stop: start.Add(time.Hour)}
	}
	existing := OperatorGuide{Channels: []OperatorChannel{channel}, Programmes: []OperatorProgramme{
		programme("stale", time.Date(2026, 7, 23, 12, 0, 0, 0, location)),
		programme("history", time.Date(2026, 7, 30, 12, 0, 0, 0, location)),
		programme("old-today", time.Date(2026, 7, 31, 1, 0, 0, 0, location)),
	}}
	fresh := OperatorGuide{Channels: []OperatorChannel{channel}, Programmes: []OperatorProgramme{
		programme("new-today", time.Date(2026, 7, 31, 1, 0, 0, 0, location)),
	}}
	merged := MergeOperatorEPG(existing, fresh, now, 7)
	if len(merged.Programmes) != 2 || merged.Programmes[0].ID != "history" || merged.Programmes[1].ID != "new-today" {
		t.Fatalf("merged programmes = %#v", merged.Programmes)
	}
}

func TestParseOperatorEPGRejectsExternalGuide(t *testing.T) {
	guide, recognized, err := ParseOperatorEPG([]byte(`<tv generator-info-name="someone-else"><channel id="one"/></tv>`), time.UTC)
	if err != nil || recognized || len(guide.Channels) != 0 {
		t.Fatalf("external guide = %#v recognized=%v err=%v", guide, recognized, err)
	}
}
