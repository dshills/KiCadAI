package transactions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
)

func TestApplyBuildsSimpleProject(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":10},"pins":[{"number":"1","x_mm":-2.54},{"number":"2","x_mm":2.54}]},
	  {"op":"assign_footprint","ref":"R1","footprint_id":"Resistor_SMD:R_0805_2012Metric"},
	  {"op":"place_footprint","ref":"R1","at":{"x_mm":20,"y_mm":20}},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: output})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	if len(result.Artifacts) != 4 {
		t.Fatalf("expected artifacts, got %#v", result.Artifacts)
	}
	for _, name := range []string{"demo.kicad_pro", "demo.kicad_sch", "demo.kicad_pcb", ".kicadai/manifest.json"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
}

func TestApplySetsBoardOutline(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"set_board_outline","board":{"width_mm":50,"height_mm":30}},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: output})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	pcbData, err := os.ReadFile(filepath.Join(output, "demo.kicad_pcb"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(pcbData), `(layer "Edge.Cuts")`); got != 4 {
		t.Fatalf("Edge.Cuts drawing count = %d, want 4\n%s", got, pcbData)
	}
}

func TestApplyRequiresCreateProject(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"write_project"}]}`)
	result := Apply(tx, ApplyOptions{OutputDir: t.TempDir()})
	if len(result.Issues) == 0 || result.Issues[0].Path != "operations[0].op" {
		t.Fatalf("expected create_project issue: %#v", result.Issues)
	}
}

func TestApplyRejectsLateCreateProject(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"write_project"},{"op":"create_project","name":"demo"}]}`)
	result := Apply(tx, ApplyOptions{OutputDir: t.TempDir()})
	if len(result.Issues) == 0 || result.Issues[0].Path != "operations[0].op" {
		t.Fatalf("expected first create_project issue: %#v", result.Issues)
	}
}

func TestApplyRejectsWriteProjectOutputOverride(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"create_project","name":"demo"},{"op":"write_project","output_dir":"elsewhere"}]}`)
	result := Apply(tx, ApplyOptions{OutputDir: t.TempDir()})
	if len(result.Issues) == 0 || result.Issues[0].Path != "operations[1]" {
		t.Fatalf("expected output override issue: %#v", result.Issues)
	}
}

func TestDeterministicDesignUUIDDependsOnSeed(t *testing.T) {
	a := deterministicDesignUUID("demo", "a")
	b := deterministicDesignUUID("demo", "b")
	if a == b || a == "" || b == "" {
		t.Fatalf("unexpected deterministic UUIDs: %q %q", a, b)
	}
}

func TestApplyStopsOnOperationFailure(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"create_project","name":"demo"},{"op":"assign_footprint","ref":"R1","footprint_id":"Device:R"}]}`)
	result := Apply(tx, ApplyOptions{OutputDir: t.TempDir()})
	if len(result.Issues) == 0 || result.Issues[0].Path != "operations[1]" {
		t.Fatalf("expected operation index issue: %#v", result.Issues)
	}
}

func TestApplyImportedAddsSymbolAndAssignsFootprint(t *testing.T) {
	dir := writeImportedApplyProject(t, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
)`, `(kicad_pcb
  (version 20260206)
  (generator "pcbnew")
  (paper "A4")
  (layers (0 "F.Cu" signal) (25 "Edge.Cuts" user))
  (setup)
  (gr_line (start 0 0) (end 10 0) (layer "Edge.Cuts") (uuid "33333333-3333-5333-8333-333333333333"))
)`)
	tx := mustParse(t, `{"operations":[
	  {"op":"add_symbol","ref":"R1","value":"10k","library_id":"Device:R","at":{"x_mm":10,"y_mm":10},"pins":[{"number":"1"},{"number":"2"}]},
	  {"op":"assign_footprint","ref":"R1","footprint_id":"Resistor_SMD:R_0805_2012Metric"},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: dir})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	data, err := os.ReadFile(filepath.Join(dir, "demo.kicad_sch"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"R1"`, `"10k"`, `"Footprint"`, `"Resistor_SMD:R_0805_2012Metric"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("schematic missing %q:\n%s", want, text)
		}
	}
}

func TestApplyImportedDuplicateRefsGetUniqueUUIDs(t *testing.T) {
	dir := writeImportedApplyProject(t, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
)`, `(kicad_pcb (version 20260206) (generator "pcbnew") (paper "A4") (layers (0 "F.Cu" signal)) (setup))`)
	tx := mustParse(t, `{"operations":[
	  {"op":"add_symbol","ref":"U1","unit":1,"library_id":"Device:R","at":{"x_mm":10,"y_mm":10}},
	  {"op":"add_symbol","ref":"U1","unit":2,"library_id":"Device:R","at":{"x_mm":20,"y_mm":10}},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: dir})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	data, err := os.ReadFile(filepath.Join(dir, "demo.kicad_sch"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]struct{}{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, `(uuid "`) {
			continue
		}
		uuid := strings.TrimSuffix(strings.TrimPrefix(line, `(uuid "`), `")`)
		if _, ok := seen[uuid]; ok {
			t.Fatalf("duplicate uuid %s in:\n%s", uuid, data)
		}
		seen[uuid] = struct{}{}
	}
}

func TestAssignImportedFootprintUpdatesAllMatchingUnits(t *testing.T) {
	file := &schematic.SchematicFile{Symbols: []schematic.SchematicSymbol{
		{Reference: "U1", Value: "LM358A"},
		{Reference: "U1", Value: "LM358B"},
		{Reference: "R1", Value: "10k"},
	}}
	if err := assignImportedFootprint(file, "U1", "Package_SO:SOIC-8"); err != nil {
		t.Fatal(err)
	}
	for i, symbol := range file.Symbols[:2] {
		found := false
		for _, property := range symbol.Properties {
			if property.Name == "Footprint" && property.Value == "Package_SO:SOIC-8" {
				found = true
			}
		}
		if !found {
			t.Fatalf("symbol %d missing footprint property: %#v", i, symbol.Properties)
		}
	}
	if len(file.Symbols[2].Properties) != 0 {
		t.Fatalf("unmatched symbol was modified: %#v", file.Symbols[2].Properties)
	}
}

func TestApplyImportedPlacesFootprintAndRoute(t *testing.T) {
	dir := writeImportedApplyProject(t, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (symbol (lib_id "Connector:Conn_01x01") (property "Reference" "J1") (uuid "22222222-2222-5222-8222-222222222222"))
)`, `(kicad_pcb
  (version 20260206)
  (generator "pcbnew")
  (paper "A4")
  (layers (0 "F.Cu" signal) (25 "Edge.Cuts" user))
  (setup)
  (gr_line (start 0 0) (end 10 0) (layer "Edge.Cuts") (uuid "33333333-3333-5333-8333-333333333333"))
)`)
	tx := mustParse(t, `{"operations":[
	  {"op":"place_footprint","ref":"J1","footprint_id":"Connector_PinHeader_2.54mm:PinHeader_1x01_P2.54mm_Vertical","at":{"x_mm":5,"y_mm":5},"pads":[{"name":"1","type":"smd","shape":"roundrect","width_mm":1.2,"height_mm":1.4,"net":"SIG"}]},
	  {"op":"route","net_name":"SIG","points":[{"x_mm":5,"y_mm":5},{"x_mm":8,"y_mm":5}]},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: dir})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	data, err := os.ReadFile(filepath.Join(dir, "demo.kicad_pcb"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"J1"`, `(size 1.2 1.4)`, `(net "SIG")`, `(segment`} {
		if !strings.Contains(text, want) {
			t.Fatalf("PCB missing %q:\n%s", want, text)
		}
	}
}

func TestApplyImportedBlocksUnsafeMutationWithoutWriting(t *testing.T) {
	dir := writeImportedApplyProject(t, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (rule_area (uuid "22222222-2222-5222-8222-222222222222"))
)`, `(kicad_pcb (version 20260206) (generator "pcbnew") (paper "A4") (layers (0 "F.Cu" signal)) (setup))`)
	schematicPath := filepath.Join(dir, "demo.kicad_sch")
	before, err := os.ReadFile(schematicPath)
	if err != nil {
		t.Fatal(err)
	}
	tx := mustParse(t, `{"operations":[
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":10}},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: dir})
	if len(result.Issues) == 0 || result.Issues[0].Code != reports.CodePreservationConflict {
		t.Fatalf("expected preservation conflict: %#v", result.Issues)
	}
	after, err := os.ReadFile(schematicPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("blocked apply changed schematic:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestApplyImportedRejectsMutationAfterWriteProject(t *testing.T) {
	dir := writeImportedApplyProject(t, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
)`, `(kicad_pcb (version 20260206) (generator "pcbnew") (paper "A4") (layers (0 "F.Cu" signal)) (setup))`)
	schematicPath := filepath.Join(dir, "demo.kicad_sch")
	before, err := os.ReadFile(schematicPath)
	if err != nil {
		t.Fatal(err)
	}
	tx := mustParse(t, `{"operations":[
	  {"op":"write_project"},
	  {"op":"add_symbol","ref":"R1","value":"10k","library_id":"Device:R","at":{"x_mm":10,"y_mm":10}}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: dir})
	if len(result.Issues) == 0 || result.Issues[0].Code != reports.CodeInvalidArgument {
		t.Fatalf("expected invalid argument: %#v", result.Issues)
	}
	after, err := os.ReadFile(schematicPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("rejected apply changed schematic:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestApplyImportedRejectsExistingApplyLock(t *testing.T) {
	dir := writeImportedApplyProject(t, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
)`, `(kicad_pcb (version 20260206) (generator "pcbnew") (paper "A4") (layers (0 "F.Cu" signal)) (setup))`)
	if err := os.WriteFile(filepath.Join(dir, ".kicadai.apply.lock"), []byte("pid=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tx := mustParse(t, `{"operations":[
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":10}},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: dir})
	if len(result.Issues) == 0 || !strings.Contains(result.Issues[0].Message, "apply lock already exists") {
		t.Fatalf("expected lock issue: %#v", result.Issues)
	}
}

func TestApplyImportedRemovesStaleApplyLock(t *testing.T) {
	dir := writeImportedApplyProject(t, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
)`, `(kicad_pcb (version 20260206) (generator "pcbnew") (paper "A4") (layers (0 "F.Cu" signal)) (setup))`)
	if err := os.WriteFile(filepath.Join(dir, applyLockFileName), []byte("pid=999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tx := mustParse(t, `{"operations":[
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":10}},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: dir})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	if _, err := os.Stat(filepath.Join(dir, applyLockFileName)); !os.IsNotExist(err) {
		t.Fatalf("lock file was not cleaned up: %v", err)
	}
}

func TestWriteAtomicPreservesExistingPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.kicad_sch")
	if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(path, func(f *os.File) error {
		_, err := f.WriteString("new")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("permissions = %v, want 0640", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("contents = %q", data)
	}
}

func TestUpsertImportedFootprintPreservesExistingModeledFields(t *testing.T) {
	generator, err := kicadfiles.NewDeterministicIDGenerator("11111111-1111-5111-8111-111111111111", "test")
	if err != nil {
		t.Fatal(err)
	}
	netName := "SIG"
	board := &pcb.PCBFile{Footprints: []pcb.Footprint{{
		UUID:        "22222222-2222-5222-8222-222222222222",
		Path:        "/J1",
		LibraryID:   "Connector:Existing",
		Reference:   "J1",
		Value:       "Header",
		Description: "preserve me",
		Tags:        "existing",
		Position:    kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(2)},
		Layer:       kicadfiles.LayerFCu,
		Models:      []pcb.Model3D{{Path: "${KICAD9_3DMODEL_DIR}/Connector.step"}},
		Pads: []pcb.Pad{
			{Name: "1", Type: "smd", Shape: "rect", Size: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)}, Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu}},
			{Name: "1", Type: "smd", Shape: "rect", Size: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)}, Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu}},
			{Name: "2", Type: "smd", Shape: "rect", Size: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)}, Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu}},
		},
	}}}
	upsertImportedFootprint(board, generator, PlaceFootprintOperation{
		Ref:         "J1",
		FootprintID: "Connector:NewShouldNotOverwrite",
		At:          Point{XMM: 5, YMM: 6},
		Pads:        []PadSpec{{Name: "1", Net: &netName}},
	})
	if len(board.Footprints) != 1 {
		t.Fatalf("footprints = %d, want 1", len(board.Footprints))
	}
	got := board.Footprints[0]
	if got.Description != "preserve me" || got.Tags != "existing" || len(got.Models) != 1 || got.LibraryID != "Connector:Existing" {
		t.Fatalf("existing metadata not preserved: %#v", got)
	}
	if len(got.Pads) != 3 || got.Pads[0].NetName != "SIG" || got.Pads[1].NetName != "SIG" || got.Pads[2].NetName != "" {
		t.Fatalf("pads not updated selectively: %#v", got.Pads)
	}
	if got.Position != (kicadfiles.Point{X: kicadfiles.MM(5), Y: kicadfiles.MM(6)}) {
		t.Fatalf("position = %#v", got.Position)
	}
	upsertImportedFootprint(board, generator, PlaceFootprintOperation{
		Ref:         "J1",
		FootprintID: "Connector:Existing",
		At:          Point{XMM: 7, YMM: 8},
		Pads:        []PadSpec{{Name: "1"}},
	})
	if board.Footprints[0].Pads[0].NetName != "SIG" {
		t.Fatalf("omitted pad net cleared existing net: %#v", board.Footprints[0].Pads[0])
	}
}

func TestUpsertImportedFootprintUsesValueProperty(t *testing.T) {
	generator, err := kicadfiles.NewDeterministicIDGenerator("11111111-1111-5111-8111-111111111111", "test")
	if err != nil {
		t.Fatal(err)
	}
	board := &pcb.PCBFile{}
	upsertImportedFootprint(board, generator, PlaceFootprintOperation{
		Ref:         "R1",
		FootprintID: "Resistor_SMD:R_0805_2012Metric",
		Value:       "10k",
		At:          Point{XMM: 1, YMM: 2},
	})
	if len(board.Footprints) != 1 {
		t.Fatalf("footprints = %d, want 1", len(board.Footprints))
	}
	got := board.Footprints[0]
	if got.Value != "10k" {
		t.Fatalf("value = %q, want 10k", got.Value)
	}
	for _, property := range got.Properties {
		if property.Name == "Value" && property.Value == "10k" {
			return
		}
	}
	t.Fatalf("value property not set from footprint value: %#v", got.Properties)
}

func TestUpsertImportedFootprintUsesPlacementSidePadLayers(t *testing.T) {
	generator, err := kicadfiles.NewDeterministicIDGenerator("11111111-1111-5111-8111-111111111111", "test")
	if err != nil {
		t.Fatal(err)
	}
	board := &pcb.PCBFile{}
	upsertImportedFootprint(board, generator, PlaceFootprintOperation{
		Ref:         "U1",
		FootprintID: "Package_SO:SOIC-8",
		Layer:       string(kicadfiles.LayerBCu),
		At:          Point{XMM: 1, YMM: 2},
		Pads:        []PadSpec{{Name: "1", Type: "smd"}},
	})
	if len(board.Footprints) != 1 || len(board.Footprints[0].Pads) != 1 {
		t.Fatalf("unexpected footprint: %#v", board.Footprints)
	}
	want := []kicadfiles.BoardLayer{kicadfiles.LayerBCu, kicadfiles.LayerBMask}
	if fmt.Sprint(board.Footprints[0].Pads[0].Layers) != fmt.Sprint(want) {
		t.Fatalf("layers = %#v, want %#v", board.Footprints[0].Pads[0].Layers, want)
	}
}

func writeImportedApplyProject(t *testing.T, schematicContents string, pcbContents string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.kicad_pro"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo.kicad_sch"), []byte(schematicContents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo.kicad_pcb"), []byte(pcbContents), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
