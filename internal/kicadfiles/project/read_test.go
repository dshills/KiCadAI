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

func TestReadProjectRejectsMalformedModeledSections(t *testing.T) {
	_, err := Read([]byte(`{"sheets":{}}`))
	if err == nil {
		t.Fatal("expected sheets error")
	}
}
