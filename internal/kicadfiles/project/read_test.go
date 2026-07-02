package project

import (
	"bytes"
	"encoding/json"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestReadProjectWrittenByWriter(t *testing.T) {
	project := ProjectFile{
		Name:          "reader_demo",
		DesignID:      "11111111-1111-5111-8111-111111111111",
		FormatVersion: kicadfiles.KiCadFormatV20260306,
		Generator:     "kicadai",
		PageSettings:  PageSettings{Paper: kicadfiles.Paper{Name: "A4"}},
		NetClasses: []NetClass{{
			Name:        "Default",
			Clearance:   kicadfiles.MM(0.2),
			TrackWidth:  kicadfiles.MM(0.25),
			ViaDiameter: kicadfiles.MM(0.8),
			ViaDrill:    kicadfiles.MM(0.4),
		}},
		Sheets: []Sheet{{UUID: "11111111-1111-5111-8111-111111111111", Name: "Root"}},
	}
	var buf bytes.Buffer
	if err := Write(&buf, project); err != nil {
		t.Fatal(err)
	}
	read, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(read.NetClasses) != 1 || len(read.Sheets) != 1 {
		t.Fatalf("unexpected read project: %#v", read)
	}
}

func TestReadProjectPreservesUnknownTopLevelJSON(t *testing.T) {
	data := []byte(`{"meta":{"version":1},"future":{"enabled":true},"sheets":[]}`)
	read, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Preserved) != 1 || !json.Valid(read.Preserved["future"]) {
		t.Fatalf("unexpected preserved JSON: %#v", read.Preserved)
	}
}

func TestReadWriteProjectPreservesModeledSectionJSON(t *testing.T) {
	data := []byte(`{
  "board":{"design_settings":{"defaults":true},"visible_elements":"ffff"},
  "erc":{"erc_exclusions":[{"symbol":"U1"}]},
  "meta":{"version":1},
  "net_settings":{"classes":[{"name":"Default","clearance":0.2,"track_width":0.25,"via_diameter":0.8,"via_drill":0.4}],"meta":"keep"},
  "pcbnew":{"last_paths":{"gbr":"gerbers"}},
  "sheets":[["11111111-1111-5111-8111-111111111111","Root"]],
  "text_variables":{}
}`)
	read, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	read.Name = "preserved_project"
	read.DesignID = "22222222-2222-5222-8222-222222222222"
	read.FormatVersion = kicadfiles.KiCadFormatV20260306
	var buf bytes.Buffer
	if err := Write(&buf, read); err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for section, key := range map[string]string{
		"board":        "visible_elements",
		"erc":          "erc_exclusions",
		"net_settings": "meta",
		"pcbnew":       "last_paths",
	} {
		var sectionDocument map[string]json.RawMessage
		if err := json.Unmarshal(out[section], &sectionDocument); err != nil {
			t.Fatalf("decode %s: %v\n%s", section, err, buf.String())
		}
		if len(sectionDocument[key]) == 0 {
			t.Fatalf("missing preserved %s.%s in %s", section, key, buf.String())
		}
	}
}

func TestReadProjectRejectsMalformedModeledSections(t *testing.T) {
	_, err := Read([]byte(`{"sheets":{}}`))
	if err == nil {
		t.Fatal("expected sheets error")
	}
}
