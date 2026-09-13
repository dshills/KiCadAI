package project

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestSchematicNetClassDefaultsUseMilsAndRoundTrip(t *testing.T) {
	p := minimalProject()
	p.NetClasses[0].WireWidth = kicadfiles.MM(.1524)
	p.NetClasses[0].BusWidth = kicadfiles.MM(.3048)
	p.NetClasses[0].HasLineStyle = true
	var b bytes.Buffer
	if err := Write(&b, p); err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Settings struct {
			Classes []struct {
				Wire  float64 `json:"wire_width"`
				Bus   float64 `json:"bus_width"`
				Style *int    `json:"line_style"`
			} `json:"classes"`
		} `json:"net_settings"`
	}
	if err := json.Unmarshal(b.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	c := raw.Settings.Classes[0]
	if c.Wire != 6 || c.Bus != 12 || c.Style == nil || *c.Style != 0 {
		t.Fatalf("native mil defaults: %+v", c)
	}
	r, err := Read(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.NetClasses, p.NetClasses) {
		t.Fatalf("readback: %+v", r.NetClasses)
	}
	p.NetClasses[0].WireWidth = -1
	if err := Validate(p); err == nil {
		t.Fatal("negative schematic width accepted")
	}
}
