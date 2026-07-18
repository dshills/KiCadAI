package pcb

import (
	"bytes"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

func TestReadPCBWrittenByWriter(t *testing.T) {
	board, err := LEDIndicatorPCB(LEDIndicatorInput{
		Name:     "reader_demo",
		DesignID: "11111111-1111-5111-8111-111111111111",
		Seed:     "reader_demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	attributeFootprintUUID := board.Footprints[0].UUID
	board.Footprints[0].Attributes = []string{"smd", "exclude_from_bom", "exclude_from_pos_files"}
	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatal(err)
	}
	read, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if read.Version == "" || read.Generator == "" || len(read.Layers) == 0 || len(read.Footprints) == 0 || len(read.Tracks) == 0 {
		t.Fatalf("unexpected read PCB: %#v", read)
	}
	if read.Footprints[0].LibraryID == "" || read.Footprints[0].Reference == "" {
		t.Fatalf("footprint metadata not read: %#v", read.Footprints[0])
	}
	if read.Footprints[0].Raw == "" || len(read.Footprints[0].Pads) == 0 || read.Footprints[0].Pads[0].Raw == "" {
		t.Fatalf("footprint raw nodes not preserved: %#v", read.Footprints[0])
	}
	var attributeFootprint *Footprint
	for index := range read.Footprints {
		if read.Footprints[index].UUID == attributeFootprintUUID {
			attributeFootprint = &read.Footprints[index]
			break
		}
	}
	if attributeFootprint == nil {
		t.Fatalf("attribute footprint %q not read", attributeFootprintUUID)
	}
	if got := strings.Join(attributeFootprint.Attributes, ","); got != "smd,exclude_from_pos_files,exclude_from_bom" {
		t.Fatalf("footprint attributes = %q, want KiCad-canonical placement and BOM exclusions", got)
	}
}

func TestReadPCBPreservesUnsupportedNode(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (paper "A4")`,
		`  (layers (0 "F.Cu" signal) (2 "B.Cu" signal))`,
		`  (setup)`,
		`  (future_constraint (uuid "22222222-2222-5222-8222-222222222222"))`,
		`)`,
	}, "\n")
	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Preserved) != 1 {
		t.Fatalf("preserved = %d, want 1", len(read.Preserved))
	}
	if read.Preserved[0].Family != "future_constraint" || !strings.Contains(read.Preserved[0].Raw, "future_constraint") {
		t.Fatalf("unexpected preserved node: %#v", read.Preserved[0])
	}
}

func TestReadPCBRejectsInvalidCoordinates(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (paper "A4")`,
		`  (layers (0 "F.Cu" signal) (2 "B.Cu" signal))`,
		`  (setup)`,
		`  (segment (start nope 1) (end 2 1) (width 0.25) (layer "F.Cu") (net "A") (uuid "22222222-2222-5222-8222-222222222222"))`,
		`)`,
	}, "\n")
	_, err := Read([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "coordinate") {
		t.Fatalf("expected coordinate error, got %v", err)
	}
}

func TestReadPCBRejectsMissingCoordinates(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (paper "A4")`,
		`  (layers (0 "F.Cu" signal) (2 "B.Cu" signal))`,
		`  (setup)`,
		`  (segment (start 1) (end 2 1) (width 0.25) (layer "F.Cu") (net "A") (uuid "22222222-2222-5222-8222-222222222222"))`,
		`)`,
	}, "\n")
	_, err := Read([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "requires x and y") {
		t.Fatalf("expected missing coordinate error, got %v", err)
	}
}

func TestReadWritePCBPreservesViaGeometryAndNet(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (generator_version "10.0.0")`,
		`  (paper "A4")`,
		`  (layers (0 "F.Cu" signal) (2 "B.Cu" signal))`,
		`  (setup)`,
		`  (net 0 "")`,
		`  (net 1 "GND")`,
		`  (via (at 10 10) (size 0.8) (drill 0.4) (layers "F.Cu" "B.Cu") (net "GND") (uuid "22222222-2222-5222-8222-222222222222"))`,
		`)`,
	}, "\n")
	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Vias) != 1 || read.Vias[0].Size != kicadfiles.MM(0.8) || read.Vias[0].Drill != kicadfiles.MM(0.4) || read.Vias[0].NetCode != 1 {
		t.Fatalf("unexpected via: %#v", read.Vias)
	}
	var buf bytes.Buffer
	if err := Write(&buf, read); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{`(size 0.8)`, `(drill 0.4)`, `"F.Cu"`, `"B.Cu"`, `(net 1)`} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %s:\n%s", want, output)
		}
	}
}

func TestReadWritePCBPreservesTrackArcsAndOvalDrills(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (generator_version "10.0.0")`,
		`  (paper "A4")`,
		`  (layers (0 "F.Cu" signal) (2 "B.Cu" signal))`,
		`  (setup)`,
		`  (net 0 "")`,
		`  (net 1 "SLOT")`,
		`  (footprint "Connector:Slot"`,
		`    (layer "F.Cu")`,
		`    (uuid "11111111-1111-5111-8111-111111111111")`,
		`    (at 10 10 0)`,
		`    (property "Reference" "J1" (at 10 8 0) (layer "F.SilkS") (uuid "22222222-2222-5222-8222-222222222222"))`,
		`    (property "Value" "Slot" (at 10 12 0) (layer "F.Fab") (uuid "33333333-3333-5333-8333-333333333333"))`,
		`    (path "/11111111-1111-5111-8111-111111111111")`,
		`    (pad "1" thru_hole oval (at 0 0) (size 1.4 2.0) (drill oval 0.6 1.0) (layers "*.Cu" "*.Mask") (net 1 "SLOT") (uuid "44444444-4444-5444-8444-444444444444"))`,
		`  )`,
		`  (arc (start 20 20) (mid 22 18) (end 24 20) (width 0.25) (layer "F.Cu") (net 1) (uuid "55555555-5555-5555-8555-555555555555"))`,
		`)`,
	}, "\n")
	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(read.TrackArcs) != 1 {
		t.Fatalf("track arcs = %#v", read.TrackArcs)
	}
	pad := read.Footprints[0].Pads[0]
	if pad.DrillShape != "oval" || pad.DrillSize != (kicadfiles.Point{X: kicadfiles.MM(0.6), Y: kicadfiles.MM(1.0)}) {
		t.Fatalf("unexpected pad drill: %#v", pad)
	}
	var buf bytes.Buffer
	if err := Write(&buf, read); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{"(arc", "(mid 22 18)", "(drill oval 0.6 1)"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %s:\n%s", want, output)
		}
	}
}

func TestReadWritePCBPreservesNonLineDrawings(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (generator_version "10.0.0")`,
		`  (paper "A4")`,
		`  (layers (0 "F.Cu" signal) (2 "B.Cu" signal) (17 "Dwgs.User" user))`,
		`  (setup)`,
		`  (gr_rect (start 0 0) (end 10 10) (stroke (width 0.1) (type solid)) (fill none) (layer "Dwgs.User") (uuid "22222222-2222-5222-8222-222222222222"))`,
		`  (gr_circle (center 20 20) (end 21 20) (stroke (width 0.1) (type solid)) (fill none) (layer "Dwgs.User") (uuid "33333333-3333-5333-8333-333333333333"))`,
		`  (gr_arc (start 30 30) (mid 31 31) (end 32 30) (stroke (width 0.1) (type solid)) (fill none) (layer "Dwgs.User") (uuid "44444444-4444-5444-8444-444444444444"))`,
		`  (gr_poly (pts (xy 40 40) (xy 42 40) (xy 42 42)) (stroke (width 0.1) (type solid)) (fill none) (layer "Dwgs.User") (uuid "55555555-5555-5555-8555-555555555555"))`,
		`  (gr_text "note" (at 50 50 0) (layer "Dwgs.User") (uuid "66666666-6666-5666-8666-666666666666") (effects (font (size 1 1))))`,
		`)`,
	}, "\n")
	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Drawings) != 5 || read.Drawings[0].Rect == nil || read.Drawings[1].Circle == nil || read.Drawings[2].Arc == nil || read.Drawings[3].Poly == nil || read.Drawings[4].Text == nil {
		t.Fatalf("unexpected drawings: %#v", read.Drawings)
	}
	var buf bytes.Buffer
	if err := Write(&buf, read); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{"(gr_rect", "(gr_circle", "(gr_arc", "(gr_poly", "(gr_text"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %s:\n%s", want, output)
		}
	}
}

func TestReadWritePCBPreservesZoneNetAndPolygon(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (generator_version "10.0.0")`,
		`  (paper "A4")`,
		`  (layers (0 "F.Cu" signal) (2 "B.Cu" signal))`,
		`  (setup)`,
		`  (net 0 "")`,
		`  (net 1 "GND")`,
		`  (zone`,
		`    (net 1)`,
		`    (net_name "GND")`,
		`    (layers "F.Cu")`,
		`    (uuid "22222222-2222-5222-8222-222222222222")`,
		`    (hatch edge 0.5)`,
		`    (connect_pads yes (clearance 0.2))`,
		`    (min_thickness 0.25)`,
		`    (fill (thermal_gap 0.5) (thermal_bridge_width 0.5) (island_removal_mode 0) (island_area_min 0))`,
		`    (polygon (pts (xy 0 0) (xy 10 0) (xy 10 10) (xy 0 0)))`,
		`  )`,
		`)`,
	}, "\n")
	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Zones) != 1 || read.Zones[0].NetCode != 1 || read.Zones[0].NetName != "GND" || len(read.Zones[0].Polygons) != 1 {
		t.Fatalf("unexpected zones: %#v", read.Zones)
	}
	var buf bytes.Buffer
	if err := Write(&buf, read); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{`(net 1)`, `(net_name "GND")`, `(layers "F.Cu")`, "(polygon"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %s:\n%s", want, output)
		}
	}
}

func TestReadPCBNetRefHandlesNumericOnlyNet(t *testing.T) {
	node, err := sexpr.Parse([]byte(`(net 1)`))
	if err != nil {
		t.Fatal(err)
	}
	code, name := readPCBNetRef(node)
	if code != 1 || name != "" {
		t.Fatalf("net ref = (%d, %q), want (1, \"\")", code, name)
	}
}

func TestReadPadDrillHandlesOvalDrill(t *testing.T) {
	node, err := sexpr.Parse([]byte(`(drill oval 0.5 0.8)`))
	if err != nil {
		t.Fatal(err)
	}
	drill, shape, size := readPadDrill(node)
	if drill != kicadfiles.MM(0.5) || shape != "oval" || size != (kicadfiles.Point{X: kicadfiles.MM(0.5), Y: kicadfiles.MM(0.8)}) {
		t.Fatalf("drill = %s shape = %q size = %#v", kicadfiles.ToMMString(drill), shape, size)
	}
}

func TestReadPadDrillRejectsZeroOvalDimension(t *testing.T) {
	node, err := sexpr.Parse([]byte(`(drill oval 0.5 0)`))
	if err != nil {
		t.Fatal(err)
	}
	drill, shape, size := readPadDrill(node)
	if drill != 0 || shape != "oval" || size.Y != 0 {
		t.Fatalf("drill = %s shape = %q size = %#v", kicadfiles.ToMMString(drill), shape, size)
	}
}
