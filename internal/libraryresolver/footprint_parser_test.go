package libraryresolver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestIndexFootprintsParsesSMDFootprint(t *testing.T) {
	root := t.TempDir()
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(footprints, "Resistor_SMD.pretty", "R_0805_2012Metric.kicad_mod"), resistor0805Footprint())

	inventory := Discover(LibraryRoots{FootprintsRoot: footprints})
	records, issues := IndexFootprints(inventory)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	record, ok := records["Resistor_SMD:R_0805_2012Metric"]
	if !ok {
		t.Fatalf("missing footprint in %#v", records)
	}
	if record.Description != "Resistor SMD 0805" {
		t.Fatalf("description = %q", record.Description)
	}
	if len(record.Attributes) != 1 || record.Attributes[0] != "smd" {
		t.Fatalf("attributes = %#v", record.Attributes)
	}
	if record.Properties["ki_locked"] != "no" {
		t.Fatalf("properties = %#v", record.Properties)
	}
	if len(record.Pads) != 2 {
		t.Fatalf("pads = %#v", record.Pads)
	}
	if record.Pads[0].Name != "1" || record.Pads[0].Type != "smd" || record.Pads[0].Shape != "roundrect" {
		t.Fatalf("pad 1 = %#v", record.Pads[0])
	}
	if record.Pads[0].RoundRectR != 0.25 {
		t.Fatalf("roundrect ratio = %v", record.Pads[0].RoundRectR)
	}
	if !record.GraphicsSummary.HasCourtyard || !record.GraphicsSummary.HasFabOutline || !record.GraphicsSummary.HasSilk {
		t.Fatalf("graphics summary = %#v", record.GraphicsSummary)
	}
	if record.GraphicsSummary.CircleCount != 1 {
		t.Fatalf("circle count = %d", record.GraphicsSummary.CircleCount)
	}
	if len(record.Models) != 1 || record.Models[0] != "${KICAD9_3DMODEL_DIR}/Resistor_SMD.3dshapes/R_0805.wrl" {
		t.Fatalf("models = %#v", record.Models)
	}
	if record.BoundingBox.Min.X >= record.BoundingBox.Max.X || record.BoundingBox.Min.Y >= record.BoundingBox.Max.Y {
		t.Fatalf("bounding box = %#v", record.BoundingBox)
	}
}

func TestIndexFootprintsParsesThroughHoleFootprint(t *testing.T) {
	root := t.TempDir()
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(footprints, "Connector_PinHeader_2.54mm.pretty", "PinHeader_1x02_P2.54mm.kicad_mod"), pinHeaderFootprint())

	inventory := Discover(LibraryRoots{FootprintsRoot: footprints})
	records, issues := IndexFootprints(inventory)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	record := records["Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm"]
	if len(record.Pads) != 2 {
		t.Fatalf("pads = %#v", record.Pads)
	}
	if record.Pads[0].Type != "thru_hole" || record.Pads[0].Drill != kicadfiles.MM(1.0) {
		t.Fatalf("pad 1 = %#v", record.Pads[0])
	}
}

func TestIndexFootprintsParsesNestedOvalDrill(t *testing.T) {
	root := t.TempDir()
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(footprints, "Connector.pretty", "Slot.kicad_mod"), `
(footprint "Slot"
  (pad "1" thru_hole oval (at 0 0) (size 1.7 2.4) (drill (oval 0.5 1.0)) (layers "*.Cu" "*.Mask"))
)`)

	inventory := Discover(LibraryRoots{FootprintsRoot: footprints})
	records, issues := IndexFootprints(inventory)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if records["Connector:Slot"].Pads[0].Drill != kicadfiles.MM(0.5) {
		t.Fatalf("pad = %#v", records["Connector:Slot"].Pads[0])
	}
}

func TestResolveFootprint(t *testing.T) {
	index := LibraryIndex{Footprints: map[string]FootprintRecord{
		"Resistor_SMD:R_0805": {FootprintID: "Resistor_SMD:R_0805", Name: "R_0805"},
	}}
	record, ok := ResolveFootprint(index, "Resistor_SMD:R_0805")
	if !ok || record.Name != "R_0805" {
		t.Fatalf("ResolveFootprint = %#v/%v", record, ok)
	}
}

func TestIndexFootprintsAllowsDuplicatePadNames(t *testing.T) {
	root := t.TempDir()
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(footprints, "Test.pretty", "DupPads.kicad_mod"), duplicatePadFootprint())

	inventory := Discover(LibraryRoots{FootprintsRoot: footprints})
	records, issues := IndexFootprints(inventory)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(records["Test:Dup"].Pads) != 2 {
		t.Fatalf("pads = %#v", records["Test:Dup"].Pads)
	}
}

func TestIndexFootprintsHandlesEmptyListFields(t *testing.T) {
	root := t.TempDir()
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(footprints, "Test.pretty", "EmptyLists.kicad_mod"), `
(footprint "EmptyLists"
  (attr)
  (pad "1" smd rect (at 0 0) (size 1 1) (layers))
)`)

	inventory := Discover(LibraryRoots{FootprintsRoot: footprints})
	records, issues := IndexFootprints(inventory)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(records["Test:EmptyLists"].Pads[0].Layers) != 0 {
		t.Fatalf("layers = %#v", records["Test:EmptyLists"].Pads[0].Layers)
	}
}

func TestIndexFootprintsCircleBoundingBox(t *testing.T) {
	root := t.TempDir()
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(footprints, "Test.pretty", "Circle.kicad_mod"), `
(footprint "Circle"
  (fp_circle (center 0 0) (end 0 1.5) (layer "F.Fab"))
)`)

	inventory := Discover(LibraryRoots{FootprintsRoot: footprints})
	records, issues := IndexFootprints(inventory)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	box := records["Test:Circle"].BoundingBox
	if box.Min.X != kicadfiles.MM(-1.5) || box.Max.X != kicadfiles.MM(1.5) || box.Min.Y != kicadfiles.MM(-1.5) || box.Max.Y != kicadfiles.MM(1.5) {
		t.Fatalf("bounding box = %#v", box)
	}
}

func TestIndexFootprintsSkipsMissingGraphicPointsInBoundingBox(t *testing.T) {
	root := t.TempDir()
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(footprints, "Test.pretty", "MissingGraphicPoints.kicad_mod"), `
(footprint "MissingGraphicPoints"
  (fp_line (layer "F.Fab"))
  (pad "1" smd rect (at 10 10) (size 2 2) (layers "F.Cu"))
)`)

	inventory := Discover(LibraryRoots{FootprintsRoot: footprints})
	records, issues := IndexFootprints(inventory)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	box := records["Test:MissingGraphicPoints"].BoundingBox
	if box.Min.X != kicadfiles.MM(9) || box.Min.Y != kicadfiles.MM(9) || box.Max.X != kicadfiles.MM(11) || box.Max.Y != kicadfiles.MM(11) {
		t.Fatalf("bounding box = %#v", box)
	}
}

func TestIndexFootprintsTextContributesToSummaryAndBounds(t *testing.T) {
	root := t.TempDir()
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(footprints, "Test.pretty", "TextOnly.kicad_mod"), `
(footprint "TextOnly"
  (fp_text reference "REF**" (at 3 4 0) (layer "F.SilkS"))
)`)

	inventory := Discover(LibraryRoots{FootprintsRoot: footprints})
	records, issues := IndexFootprints(inventory)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	record := records["Test:TextOnly"]
	if !record.GraphicsSummary.HasSilk || record.GraphicsSummary.TextCount != 1 {
		t.Fatalf("summary = %#v", record.GraphicsSummary)
	}
	if record.BoundingBox.Min.X != kicadfiles.MM(3) || record.BoundingBox.Min.Y != kicadfiles.MM(4) {
		t.Fatalf("bounding box = %#v", record.BoundingBox)
	}
}

func TestIndexFootprintsOddInternalUnitPadBoundingBox(t *testing.T) {
	var bounds footprintBounds
	bounds.includePad(FootprintPad{
		Position: kicadfiles.Point{X: 0, Y: 0},
		Size:     kicadfiles.Point{X: 5, Y: 7},
	})
	box := bounds.box()
	if box.Min.X != -2 || box.Max.X != 3 || box.Min.Y != -3 || box.Max.Y != 4 {
		t.Fatalf("bounding box = %#v", box)
	}
}

func TestIndexFootprintsRotatedPadBoundingBox(t *testing.T) {
	var bounds footprintBounds
	bounds.includePad(FootprintPad{
		Position: kicadfiles.Point{X: 0, Y: 0},
		Size:     kicadfiles.Point{X: kicadfiles.MM(2), Y: kicadfiles.MM(2)},
		Rotation: 45,
	})
	box := bounds.box()
	want := kicadfiles.IU(1414214)
	if box.Min.X != -want || box.Max.X != want || box.Min.Y != -want || box.Max.Y != want {
		t.Fatalf("bounding box = %#v", box)
	}
}

func TestIndexFootprintsRotatedCirclePadBoundingBox(t *testing.T) {
	var bounds footprintBounds
	bounds.includePad(FootprintPad{
		Shape:    "circle",
		Position: kicadfiles.Point{X: 0, Y: 0},
		Size:     kicadfiles.Point{X: kicadfiles.MM(2), Y: kicadfiles.MM(2)},
		Rotation: 45,
	})
	box := bounds.box()
	if box.Min.X != kicadfiles.MM(-1) || box.Max.X != kicadfiles.MM(1) || box.Min.Y != kicadfiles.MM(-1) || box.Max.Y != kicadfiles.MM(1) {
		t.Fatalf("bounding box = %#v", box)
	}
}

func TestIndexFootprintsRotatedOvalPadBoundingBox(t *testing.T) {
	var bounds footprintBounds
	bounds.includePad(FootprintPad{
		Shape:    "oval",
		Position: kicadfiles.Point{X: 0, Y: 0},
		Size:     kicadfiles.Point{X: kicadfiles.MM(4), Y: kicadfiles.MM(2)},
		Rotation: 90,
	})
	box := bounds.box()
	if box.Min.X != kicadfiles.MM(-1) || box.Max.X != kicadfiles.MM(1) || box.Min.Y != kicadfiles.MM(-2) || box.Max.Y != kicadfiles.MM(2) {
		t.Fatalf("bounding box = %#v", box)
	}
}

func TestIndexFootprintsArcBoundingBoxUsesCircumcircle(t *testing.T) {
	var bounds footprintBounds
	bounds.includeArc(
		kicadfiles.Point{X: kicadfiles.MM(-1), Y: 0},
		kicadfiles.Point{X: 0, Y: kicadfiles.MM(1)},
		kicadfiles.Point{X: kicadfiles.MM(1), Y: 0},
	)
	box := bounds.box()
	if box.Min.X != kicadfiles.MM(-1) || box.Max.X != kicadfiles.MM(1) || box.Min.Y != 0 || box.Max.Y != kicadfiles.MM(1) {
		t.Fatalf("bounding box = %#v", box)
	}
}

func TestIndexFootprintsMalformedFileDiagnostic(t *testing.T) {
	root := t.TempDir()
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(footprints, "Test.pretty", "Bad.kicad_mod"), "(footprint")

	inventory := Discover(LibraryRoots{FootprintsRoot: footprints})
	records, issues := IndexFootprints(inventory)
	if len(records) != 0 {
		t.Fatalf("records = %#v", records)
	}
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "unterminated") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestIndexFootprintsInvalidRootDiagnostic(t *testing.T) {
	root := t.TempDir()
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(footprints, "Test.pretty", "Bad.kicad_mod"), "(not_footprint)")

	inventory := Discover(LibraryRoots{FootprintsRoot: footprints})
	_, issues := IndexFootprints(inventory)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "expected footprint root") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestIndexFootprintsMalformedNumericDiagnostic(t *testing.T) {
	root := t.TempDir()
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(footprints, "Test.pretty", "BadNumber.kicad_mod"), `
(footprint "BadNumber"
  (pad "1" smd rect (at nope 0) (size 1 1) (layers "F.Cu"))
)`)

	inventory := Discover(LibraryRoots{FootprintsRoot: footprints})
	_, issues := IndexFootprints(inventory)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "at requires numeric x and y coordinates") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestIndexFootprintsMalformedPolygonPointDiagnostic(t *testing.T) {
	root := t.TempDir()
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(footprints, "Test.pretty", "BadPoly.kicad_mod"), `
(footprint "BadPoly"
  (fp_poly (pts (xy nope 0) (xy 1 1)) (layer "F.Fab"))
)`)

	inventory := Discover(LibraryRoots{FootprintsRoot: footprints})
	records, issues := IndexFootprints(inventory)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "xy requires numeric x and y coordinates") {
		t.Fatalf("issues = %#v", issues)
	}
	box := records["Test:BadPoly"].BoundingBox
	if box.Min.X != kicadfiles.MM(1) || box.Min.Y != kicadfiles.MM(1) {
		t.Fatalf("bounding box = %#v", box)
	}
}

func TestIndexFootprintsOversizedFileDiagnostic(t *testing.T) {
	root := t.TempDir()
	footprints := filepath.Join(root, "footprints")
	path := filepath.Join(footprints, "Test.pretty", "Huge.kicad_mod")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxFootprintLibraryBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	inventory := Discover(LibraryRoots{FootprintsRoot: footprints})
	_, issues := IndexFootprints(inventory)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "exceeds 64 MiB parser limit") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestIndexFootprintsDuplicateIDDiagnostic(t *testing.T) {
	root := t.TempDir()
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(footprints, "Test.pretty", "Dup.kicad_mod"), duplicatePadFootprint())
	mustWrite(t, filepath.Join(footprints, "nested", "Test.pretty", "Dup.kicad_mod"), duplicatePadFootprint())

	inventory := Discover(LibraryRoots{FootprintsRoot: footprints})
	_, issues := IndexFootprints(inventory)
	found := false
	for _, issue := range issues {
		if strings.Contains(issue.Message, "duplicate footprint ID Test:Dup") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected duplicate footprint ID diagnostic: %#v", issues)
	}
}

func TestIndexFootprintsGoldenRecordJSON(t *testing.T) {
	root := t.TempDir()
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(footprints, "Resistor_SMD.pretty", "R_0805_2012Metric.kicad_mod"), resistor0805Footprint())

	inventory := Discover(LibraryRoots{FootprintsRoot: footprints})
	records, issues := IndexFootprints(inventory)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	data, err := json.MarshalIndent(records["Resistor_SMD:R_0805_2012Metric"], "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"footprint_id": "Resistor_SMD:R_0805_2012Metric"`, `"name": "1"`, `"has_courtyard": true`} {
		if !strings.Contains(text, want) {
			t.Fatalf("golden JSON missing %s:\n%s", want, text)
		}
	}
}

func resistor0805Footprint() string {
	return `
(footprint "R_0805_2012Metric"
  (version 20240108)
  (generator "kicadai-test")
  (descr "Resistor SMD 0805")
  (tags "resistor 0805")
  (attr smd)
  (property "ki_locked" "no")
  (fp_text reference "REF**" (at 0 -1.8 0) (layer "F.SilkS"))
  (fp_text value "R_0805_2012Metric" (at 0 1.8 0) (layer "F.Fab"))
  (fp_line (start -1.8 -0.9) (end 1.8 -0.9) (layer "F.CrtYd"))
  (fp_line (start -1.0 -0.5) (end 1.0 -0.5) (layer "F.Fab"))
  (fp_line (start -1.0 0.5) (end 1.0 0.5) (layer "F.SilkS"))
  (fp_circle (center 0 0) (end 0 0.6) (layer "F.Fab"))
  (pad "1" smd roundrect (at -0.95 0) (size 1.0 1.2) (layers "F.Cu" "F.Paste" "F.Mask") (roundrect_rratio 0.25) (pinfunction "1") (pintype "passive"))
  (pad "2" smd roundrect (at 0.95 0) (size 1.0 1.2) (layers "F.Cu" "F.Paste" "F.Mask") (roundrect_rratio 0.25) (pinfunction "2") (pintype "passive"))
  (model "${KICAD9_3DMODEL_DIR}/Resistor_SMD.3dshapes/R_0805.wrl")
)`
}

func pinHeaderFootprint() string {
	return `
(footprint "PinHeader_1x02_P2.54mm"
  (version 20240108)
  (generator "kicadai-test")
  (descr "Through hole pin header")
  (tags "pin header")
  (attr through_hole)
  (pad "1" thru_hole rect (at 0 0) (size 1.7 1.7) (drill 1.0) (layers "*.Cu" "*.Mask"))
  (pad "2" thru_hole oval (at 0 2.54) (size 1.7 1.7) (drill 1.0) (layers "*.Cu" "*.Mask"))
)`
}

func duplicatePadFootprint() string {
	return `
(footprint "Dup"
  (version 20240108)
  (generator "kicadai-test")
  (pad "1" smd rect (at 0 0) (size 1 1) (layers "F.Cu"))
  (pad "1" smd rect (at 2 0) (size 1 1) (layers "F.Cu"))
)`
}
