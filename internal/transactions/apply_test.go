package transactions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/project"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/manifest"
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
	if len(result.Artifacts) != 7 {
		t.Fatalf("expected artifacts, got %#v", result.Artifacts)
	}
	for _, name := range []string{
		"demo.kicad_pro",
		"demo.kicad_sch",
		"demo.kicad_pcb",
		"fp-lib-table",
		"footprints/Resistor_SMD.pretty/R_0805_2012Metric.kicad_mod",
		".kicadai/manifest.json",
		".kicadai/transaction.json",
	} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
	generatedManifest, status, err := manifest.Read(output)
	if err != nil {
		t.Fatal(err)
	}
	if status.Stale {
		t.Fatalf("manifest status = %#v, want fresh", status)
	}
	if generatedManifest.Provenance == nil || generatedManifest.Provenance.Hash == "" {
		t.Fatalf("manifest provenance = %#v", generatedManifest.Provenance)
	}
	if got := generatedManifest.FileHashes[transactionProvenancePath]; got == "" || got != generatedManifest.Provenance.Hash {
		t.Fatalf("transaction provenance hash = %q manifest provenance = %#v", got, generatedManifest.Provenance)
	}
	provenanceData, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(transactionProvenancePath)))
	if err != nil {
		t.Fatal(err)
	}
	var readProvenance transactionProvenance
	if err := json.Unmarshal(provenanceData, &readProvenance); err != nil {
		t.Fatal(err)
	}
	if readProvenance.OperationCount != len(tx.Operations) {
		t.Fatalf("operation count = %d, want %d", readProvenance.OperationCount, len(tx.Operations))
	}
}

func TestApplyWritesCreateProjectTextVariables(t *testing.T) {
	output := filepath.Join(t.TempDir(), "metadata")
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"metadata","text_variables":{"board_finish":"ENIG","fabrication_notes":"Lead-free assembly."}},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: output})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	written, err := project.ReadFile(filepath.Join(output, "metadata.kicad_pro"))
	if err != nil {
		t.Fatal(err)
	}
	if written.TextVariables["board_finish"] != "ENIG" || written.TextVariables["fabrication_notes"] != "Lead-free assembly." {
		t.Fatalf("text variables = %#v", written.TextVariables)
	}
}

func TestApplyWritesRequestedFourLayerStack(t *testing.T) {
	output := filepath.Join(t.TempDir(), "four-layer")
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"four-layer"},
	  {"op":"route","net_name":"SIG","layer":"In1.Cu","points":[{"x_mm":5,"y_mm":5},{"x_mm":8,"y_mm":5}],"vias":[{"at":{"x_mm":8,"y_mm":5},"diameter_mm":0.6,"drill_mm":0.3,"layers":["In1.Cu","In2.Cu"]}]},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: output, CopperLayers: 4})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	board, err := pcb.ReadFile(filepath.Join(output, "four-layer.kicad_pcb"))
	if err != nil {
		t.Fatal(err)
	}
	var copper []kicadfiles.BoardLayer
	for _, layer := range board.Layers {
		if layer.Kind == "signal" {
			copper = append(copper, layer.Name)
		}
	}
	want := []kicadfiles.BoardLayer{
		kicadfiles.LayerFCu,
		kicadfiles.BoardLayer("In1.Cu"),
		kicadfiles.BoardLayer("In2.Cu"),
		kicadfiles.LayerBCu,
	}
	if !slices.Equal(copper, want) {
		t.Fatalf("copper layers = %#v, want %#v", copper, want)
	}
	if len(board.Tracks) != 1 || board.Tracks[0].Layer != kicadfiles.BoardLayer("In1.Cu") {
		t.Fatalf("inner-layer tracks = %#v", board.Tracks)
	}
	if len(board.Vias) != 1 || !slices.Equal(board.Vias[0].Layers, []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu}) {
		t.Fatalf("inner-layer vias = %#v", board.Vias)
	}
}

func TestApplyPreservesMirroredSymbolConnectionAnchors(t *testing.T) {
	output := filepath.Join(t.TempDir(), "mirrored")
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"mirrored"},
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":20,"y_mm":30},"rotation_deg":90,"mirror":"x","pins":[{"number":"1","x_mm":0,"y_mm":3.81},{"number":"2","x_mm":0,"y_mm":-3.81}]},
	  {"op":"write_project","schematic_only":true}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: output})
	if len(result.Issues) != 0 {
		t.Fatalf("apply issues: %#v", result.Issues)
	}
	file, err := schematic.ReadFile(filepath.Join(output, "mirrored.kicad_sch"))
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Symbols) != 1 {
		t.Fatalf("symbols = %#v", file.Symbols)
	}
	symbol := file.Symbols[0]
	if symbol.Mirror != schematic.SymbolMirrorX || symbol.Rotation != 90 {
		t.Fatalf("symbol transform = mirror:%q rotation:%v", symbol.Mirror, symbol.Rotation)
	}
	pins, ok := schematic.EmbeddedSymbolConnectionPinOffsets("Device:R")
	if !ok || len(pins) == 0 {
		t.Fatal("Device:R connection anchors missing")
	}
	offset := pins[0].Offset
	want := kicadfiles.Point{X: symbol.Position.X, Y: symbol.Position.Y}
	transformed := schematic.TransformConnectionAnchor(offset, symbol.Rotation, symbol.Mirror)
	want.X += transformed.X
	want.Y += transformed.Y
	if len(symbol.PinAnchors) == 0 || symbol.PinAnchors[0] != want {
		t.Fatalf("pin 1 anchor = %#v, want %#v", symbol.PinAnchors, want)
	}
}

func TestApplyWritesNativeVectorBusOperations(t *testing.T) {
	output := filepath.Join(t.TempDir(), "bus_demo")
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"bus_demo"},
	  {"op":"add_bus","points":[{"x_mm":20,"y_mm":50},{"x_mm":80,"y_mm":50}]},
	  {"op":"add_bus_entry","at":{"x_mm":40,"y_mm":50},"size":{"x_mm":2.54,"y_mm":2.54}},
	  {"op":"add_schematic_wire","net_name":"DATA0","points":[{"x_mm":10,"y_mm":52.54},{"x_mm":42.54,"y_mm":52.54}],"label":"DATA0","label_at":{"x_mm":42.54,"y_mm":52.54}},
	  {"op":"write_project","schematic_only":true}
	]}`)
	validation := Validate(tx)
	if reports.HasBlockingIssue(validation.Issues) {
		t.Fatalf("transaction validation issues: %#v", validation.Issues)
	}
	result := Apply(tx, ApplyOptions{OutputDir: output, Overwrite: true})
	if reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("apply issues: %#v", result.Issues)
	}
	file, err := schematic.ReadFile(filepath.Join(output, "bus_demo.kicad_sch"))
	if err != nil {
		t.Fatalf("read schematic: %v", err)
	}
	if len(file.Buses) != 1 || len(file.BusEntries) != 1 || len(file.Wires) != 1 || len(file.Labels) != 1 {
		t.Fatalf("readback vector bus geometry = buses:%d entries:%d wires:%d labels:%d", len(file.Buses), len(file.BusEntries), len(file.Wires), len(file.Labels))
	}
}

func TestApplyPreservesMultiUnitSymbolsAndConnections(t *testing.T) {
	output := filepath.Join(t.TempDir(), "dual")
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"dual"},
	  {"op":"add_symbol","ref":"U1","unit":1,"library_id":"Device:R","at":{"x_mm":20,"y_mm":30},"pins":[{"number":"1","x_mm":-5},{"number":"2","x_mm":5}]},
	  {"op":"add_symbol","ref":"U1","unit":2,"library_id":"Device:R","at":{"x_mm":40,"y_mm":30},"pins":[{"number":"1","x_mm":-5},{"number":"2","x_mm":5}]},
	  {"op":"connect","from":{"ref":"U1","unit":1,"pin":"2"},"to":{"ref":"U1","unit":2,"pin":"1"},"net_name":"UNIT_LINK"},
	  {"op":"write_project","schematic_only":true}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: output})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	file, err := schematic.ReadFile(filepath.Join(output, "dual.kicad_sch"))
	if err != nil {
		t.Fatal(err)
	}
	units := map[int]bool{}
	for _, symbol := range file.Symbols {
		units[symbol.Unit] = true
	}
	if len(file.Symbols) != 2 || !units[1] || !units[2] {
		t.Fatalf("symbols = %#v, want two units", file.Symbols)
	}
	if len(file.Wires) == 0 {
		t.Fatal("multi-unit connection did not emit schematic wire")
	}
}

func TestWriteImportedProjectDoesNotPartiallyReplaceOnRenderFailure(t *testing.T) {
	root := t.TempDir()
	base := "demo"
	schematicPath := filepath.Join(root, base+".kicad_sch")
	pcbPath := filepath.Join(root, base+".kicad_pcb")
	oldSchematic := []byte("old schematic")
	oldPCB := []byte("old pcb")
	if err := os.WriteFile(schematicPath, oldSchematic, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pcbPath, oldPCB, 0o644); err != nil {
		t.Fatal(err)
	}

	design := kicaddesign.Design{
		Schematic: &schematic.SchematicFile{
			Version:          kicadfiles.KiCadSchematicFormatV20260306,
			Generator:        "eeschema",
			GeneratorVersion: "10.0",
			UUID:             kicadfiles.UUID("11111111-1111-5111-8111-111111111111"),
			Paper:            kicadfiles.Paper{Name: "A4"},
		},
		PCB: &pcb.PCBFile{
			Version:          kicadfiles.KiCadPCBFormatV20260206,
			Generator:        "pcbnew",
			GeneratorVersion: "10.0",
			General:          pcb.DefaultGeneral(),
			Paper:            kicadfiles.Paper{Name: "A4"},
			Layers:           pcb.DefaultTwoLayerStack(),
			Setup:            pcb.DefaultSetup(),
			Nets:             []pcb.Net{{Code: 0, Name: ""}, {Code: 1, Name: "A"}},
			Vias: []pcb.Via{{
				UUID:    kicadfiles.UUID("22222222-2222-5222-8222-222222222222"),
				NetCode: 1,
				NetName: "A",
				Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
			}},
		},
	}

	if _, err := writeImportedProject(root, base, design, true, true); err == nil {
		t.Fatal("expected PCB render failure")
	}
	if got, err := os.ReadFile(schematicPath); err != nil || string(got) != string(oldSchematic) {
		t.Fatalf("schematic changed: %q err=%v", got, err)
	}
	if got, err := os.ReadFile(pcbPath); err != nil || string(got) != string(oldPCB) {
		t.Fatalf("pcb changed: %q err=%v", got, err)
	}
}

func TestApplyUsesResolverFootprintPadsForGeneratedPlacement(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	index := applyResolverFixture()
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"add_symbol","ref":"J1","library_id":"Connector:Conn_01x02","at":{"x_mm":10,"y_mm":10},"pins":[{"number":"1"},{"number":"2"}]},
	  {"op":"assign_footprint","ref":"J1","footprint_id":"Connector_Test:TH_1x02"},
	  {"op":"place_footprint","ref":"J1","footprint_id":"Connector_Test:TH_1x02","at":{"x_mm":20,"y_mm":20}},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: output, LibraryIndex: &index})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	data, err := os.ReadFile(filepath.Join(output, "demo.kicad_pcb"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		`thru_hole`,
		`(drill 0.8)`,
		`"*.Cu"`,
		`"*.Mask"`,
		`(descr "test through-hole connector")`,
		`(property "ki_description" "test through-hole connector")`,
		`(fp_text`,
		`"TEST"`,
		`(fp_line`,
		`(fp_curve`,
		`(model`,
		`${KICAD9_3DMODEL_DIR}/Connector_Test.3dshapes/TH_1x02.wrl`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated PCB missing %q:\n%s", want, text)
		}
	}
}

func TestApplyCanPreserveTransactionFootprintGeometryWithResolver(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	index := applyResolverFixture()
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"add_symbol","ref":"J1","library_id":"Connector:Conn_01x02","at":{"x_mm":10,"y_mm":10},"pins":[{"number":"1"},{"number":"2"}]},
	  {"op":"assign_footprint","ref":"J1","footprint_id":"Connector_Test:TH_1x02"},
	  {"op":"place_footprint","ref":"J1","footprint_id":"Connector_Test:TH_1x02","at":{"x_mm":20,"y_mm":20},"pads":[
	    {"name":"1","type":"smd","shape":"rect","x_mm":-1,"y_mm":0,"size_x_mm":1,"size_y_mm":1},
	    {"name":"2","type":"smd","shape":"rect","x_mm":1,"y_mm":0,"size_x_mm":1,"size_y_mm":1}
	  ]},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: output, LibraryIndex: &index, PreserveFootprintGeometry: true})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	board, err := pcb.ReadFile(filepath.Join(output, "demo.kicad_pcb"))
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Footprints) != 1 || len(board.Footprints[0].Pads) != 2 {
		t.Fatalf("unexpected footprints: %#v", board.Footprints)
	}
	for _, pad := range board.Footprints[0].Pads {
		if pad.Type != "smd" || pad.Shape != "rect" || pad.Drill != 0 {
			t.Fatalf("transaction footprint geometry was not preserved: %#v", board.Footprints[0].Pads)
		}
	}
	for _, marker := range []string{"(fp_text", "(fp_line", "(fp_curve", "(model"} {
		if !strings.Contains(board.Footprints[0].Raw, marker) {
			t.Fatalf("resolver non-pad footprint geometry missing %q:\n%s", marker, board.Footprints[0].Raw)
		}
	}
}

func TestApplyUsesResolverSymbolPinsForGeneratedSchematic(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	index := applyResolverFixture()
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":10}},
	  {"op":"assign_footprint","ref":"R1","footprint_id":"Resistor_Test:R_0603"},
	  {"op":"place_footprint","ref":"R1","footprint_id":"Resistor_Test:R_0603","at":{"x_mm":20,"y_mm":20}},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: output, LibraryIndex: &index})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	file, err := schematic.ReadFile(filepath.Join(output, "demo.kicad_sch"))
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Symbols) != 1 || len(file.Symbols[0].Pins) != 2 {
		t.Fatalf("resolver pins not instantiated: %#v", file.Symbols)
	}
}

func TestApplyWritesAddSymbolProperties(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":10},"properties":[
	    {"name":"KiCadAI Component ID","value":"resistor.generic.0805","hidden":true,"show_name":false,"do_not_autoplace":true},
	    {"name":"Manufacturer","value":"Yageo","hidden":true,"at":{"x_mm":11,"y_mm":12},"rotation_deg":90}
	  ]},
	  {"op":"assign_footprint","ref":"R1","footprint_id":"Resistor_SMD:R_0805_2012Metric"},
	  {"op":"place_footprint","ref":"R1","at":{"x_mm":20,"y_mm":20}},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: output})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	file, err := schematic.ReadFile(filepath.Join(output, "demo.kicad_sch"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(output, "demo.kicad_sch"))
	if err != nil {
		t.Fatal(err)
	}
	properties := propertyValues(file.Symbols[0].Properties)
	if properties["KiCadAI Component ID"] != "resistor.generic.0805" || properties["Manufacturer"] != "Yageo" {
		t.Fatalf("properties not written: %#v", file.Symbols[0].Properties)
	}
	for _, property := range file.Symbols[0].Properties {
		if property.Name == "Manufacturer" && property.Position.X != kicadfiles.MM(11) {
			t.Fatalf("manufacturer property position not preserved: %#v", property)
		}
	}
	text := string(data)
	if !strings.Contains(text, `"Manufacturer"`) || !strings.Contains(text, `"Yageo"`) || !strings.Contains(text, `(at 11 12 90)`) || !strings.Contains(text, `(hide yes)`) {
		t.Fatalf("written schematic missing property flags:\n%s", text)
	}
}

func TestApplyUsesLastDuplicateAddSymbolProperty(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":10},"properties":[
	    {"name":"Manufacturer","value":"First","hidden":true},
	    {"name":"manufacturer","value":"Last","hidden":true}
	  ]},
	  {"op":"assign_footprint","ref":"R1","footprint_id":"Resistor_SMD:R_0805_2012Metric"},
	  {"op":"place_footprint","ref":"R1","at":{"x_mm":20,"y_mm":20}},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: output})
	if len(result.Issues) != 1 || result.Issues[0].Severity != reports.SeverityWarning {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	file, err := schematic.ReadFile(filepath.Join(output, "demo.kicad_sch"))
	if err != nil {
		t.Fatal(err)
	}
	properties := propertyValues(file.Symbols[0].Properties)
	if properties["Manufacturer"] != "Last" {
		t.Fatalf("duplicate property was not resolved with last value: %#v", file.Symbols[0].Properties)
	}
}

func TestSchematicPropertiesFromPayloadOffsetsVisibleProperties(t *testing.T) {
	properties := schematicPropertiesFromPayload([]SymbolProperty{
		{Name: "KiCadAI Component ID", Value: "resistor.generic.0805", Hidden: true},
		{Name: "Assembly Note", Value: "visible"},
	}, point(10, 20), 180, 2)
	if len(properties) != 2 {
		t.Fatalf("properties = %#v", properties)
	}
	if properties[0].Position != point(10, 20) {
		t.Fatalf("hidden property position = %#v, want symbol position", properties[0].Position)
	}
	if properties[0].Rotation != 180 || properties[1].Rotation != 180 {
		t.Fatalf("property rotations = %#v, want default symbol rotation", properties)
	}
	wantVisible := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(20 + 2.54*3)}
	if properties[1].Position != wantVisible {
		t.Fatalf("visible property position = %#v, want %#v", properties[1].Position, wantVisible)
	}
}

func TestApplyBlocksResolverMultiUnitSymbol(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	index := applyResolverFixture()
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"add_symbol","ref":"U1","unit":1,"library_id":"Amplifier:DUAL","at":{"x_mm":10,"y_mm":10}},
	  {"op":"add_symbol","ref":"U1","unit":2,"library_id":"Amplifier:DUAL","at":{"x_mm":30,"y_mm":10}},
	  {"op":"write_project","schematic_only":true}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: output, LibraryIndex: &index})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected multi-unit issues: %#v", result.Issues)
	}
	file, err := schematic.ReadFile(filepath.Join(output, "demo.kicad_sch"))
	if err != nil {
		t.Fatalf("read multi-unit schematic: %v", err)
	}
	units := map[int]bool{}
	for _, symbol := range file.Symbols {
		if len(symbol.Pins) != 2 {
			t.Fatalf("multi-unit symbol pins = %#v, want two pins", symbol)
		}
		units[symbol.Unit] = true
	}
	if len(file.Symbols) != 2 || !units[1] || !units[2] {
		t.Fatalf("multi-unit symbols = %#v, want units 1 and 2 with two pins each", file.Symbols)
	}
}

func TestApplyBlocksResolverDuplicatePinSymbol(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	index := applyResolverFixture()
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"add_symbol","ref":"U1","library_id":"Device:STACKED","at":{"x_mm":10,"y_mm":10}},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: output, LibraryIndex: &index})
	if len(result.Issues) == 0 || !strings.Contains(result.Issues[0].Message, "duplicate pin number") {
		t.Fatalf("expected duplicate pin issue: %#v", result.Issues)
	}
}

func TestApplyBlocksMissingResolverSymbolWhenPinsOmitted(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	index := applyResolverFixture()
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"add_symbol","ref":"X1","library_id":"Device:Missing","at":{"x_mm":10,"y_mm":10}},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: output, LibraryIndex: &index})
	if len(result.Issues) == 0 || !strings.Contains(result.Issues[0].Message, "symbol library record not found") {
		t.Fatalf("expected missing symbol issue: %#v", result.Issues)
	}
}

func TestUpsertImportedFootprintUsesResolverRecord(t *testing.T) {
	index := applyResolverFixture()
	generator, err := kicadfiles.NewDeterministicIDGenerator("11111111-1111-5111-8111-111111111111", "resolver")
	if err != nil {
		t.Fatal(err)
	}
	board := &pcb.PCBFile{}
	if err := upsertImportedFootprintWithLibrary(board, generator, PlaceFootprintOperation{
		Ref:         "J1",
		FootprintID: "Connector_Test:TH_1x02",
		At:          Point{XMM: 5, YMM: 5},
	}, &index); err != nil {
		t.Fatal(err)
	}
	if len(board.Footprints) != 1 || len(board.Footprints[0].Pads) != 2 {
		t.Fatalf("unexpected footprint: %#v", board.Footprints)
	}
	if board.Footprints[0].Pads[0].PinFunction != "A" || board.Footprints[0].Pads[0].Drill != kicadfiles.MM(0.8) {
		t.Fatalf("resolver pad metadata missing: %#v", board.Footprints[0].Pads[0])
	}
	if len(board.Footprints[0].Graphics) != 2 {
		t.Fatalf("resolver graphics missing: %#v", board.Footprints[0].Graphics)
	}
	if graphic := pcb.Drawing(board.Footprints[0].Graphics[0]); graphic.Line == nil || graphic.Layer != kicadfiles.LayerFSilkS {
		t.Fatalf("resolver graphic not mapped: %#v", graphic)
	}
	if graphic := pcb.Drawing(board.Footprints[0].Graphics[1]); graphic.Curve == nil || graphic.Layer != kicadfiles.LayerFFab {
		t.Fatalf("resolver curve not mapped: %#v", graphic)
	}
}

func TestUpsertImportedFootprintAppliesNetOnlyPadsToResolverGeometry(t *testing.T) {
	index := applyResolverFixture()
	board := &pcb.PCBFile{}
	netA, netB := "VCC", "GND"
	if err := upsertImportedFootprintWithLibrary(board, mustGenerator(t), PlaceFootprintOperation{
		Ref: "J1", FootprintID: "Connector_Test:TH_1x02", At: Point{XMM: 5, YMM: 5},
		Pads: []PadSpec{{Name: "1", Net: &netA}, {Name: "2", Net: &netB}},
	}, &index); err != nil {
		t.Fatal(err)
	}
	if len(board.Footprints) != 1 || len(board.Footprints[0].Pads) != 2 {
		t.Fatalf("unexpected footprint: %#v", board.Footprints)
	}
	if board.Footprints[0].Pads[0].Drill == 0 || board.Footprints[0].Pads[1].Position == (kicadfiles.Point{}) {
		t.Fatalf("resolver geometry was not preserved: %#v", board.Footprints[0].Pads)
	}
	if board.Footprints[0].Pads[0].NetName != netA || board.Footprints[0].Pads[1].NetName != netB {
		t.Fatalf("explicit nets were not applied: %#v", board.Footprints[0].Pads)
	}
}

func TestUpsertImportedFootprintRejectsUnknownNetOnlyPad(t *testing.T) {
	index := applyResolverFixture()
	board := &pcb.PCBFile{}
	net := "VCC"
	err := upsertImportedFootprintWithLibrary(board, mustGenerator(t), PlaceFootprintOperation{
		Ref: "J1", FootprintID: "Connector_Test:TH_1x02", Pads: []PadSpec{{Name: "99", Net: &net}},
	}, &index)
	if err == nil || !strings.Contains(err.Error(), "no pad 99") {
		t.Fatalf("expected missing-pad error, got %v", err)
	}
}

func TestImportedMetadataPropertiesExcludeReservedFootprintProperties(t *testing.T) {
	properties := importedMetadataProperties(map[string]string{
		"Reference": "REF**", "Value": "R", "Datasheet": "https://example.test", "Description": "resistor", "KiLib_Generator": "test",
	})
	if len(properties) != 1 || properties[0].Name != "KiLib_Generator" || properties[0].Value != "test" {
		t.Fatalf("metadata properties = %#v", properties)
	}
}

func TestImportedFootprintPropertiesExcludeDefaultProperties(t *testing.T) {
	properties := footprintPropertiesFromRecord([]libraryresolver.FootprintProperty{
		{Name: "Reference", Value: "REF**"},
		{Name: "value", Value: "R"},
		{Name: " Datasheet ", Value: "https://example.test"},
		{Name: "DESCRIPTION", Value: "resistor"},
		{Name: "KiLib_Generator", Value: "test"},
	}, kicadfiles.LayerFCu)
	if len(properties) != 1 || properties[0].Name != "KiLib_Generator" || properties[0].Value != "test" {
		t.Fatalf("footprint properties = %#v", properties)
	}
}

func TestResolverFootprintBottomPlacementMapsFrontLayers(t *testing.T) {
	index := applyResolverFixture()
	record := index.Footprints["Resistor_Test:R_0603"]
	specs := footprintRecordPadSpecs(record, kicadfiles.LayerBCu)
	if len(specs) != 2 || !containsBoardLayer(specs[0].Layers, kicadfiles.LayerBCu) || !containsBoardLayer(specs[0].Layers, kicadfiles.LayerBMask) {
		t.Fatalf("bottom layers not remapped: %#v", specs)
	}
	footprint := importedFootprintFromRecord(mustGenerator(t), PlaceFootprintOperation{Ref: "R1", FootprintID: record.FootprintID, Layer: string(kicadfiles.LayerBCu), HideDefaultFootprintText: true}, record)
	if len(footprint.Properties) < 2 || footprint.Properties[0].Layer != kicadfiles.LayerBSilkS {
		t.Fatalf("property layers not remapped: %#v", footprint.Properties)
	}
	for _, property := range footprint.Properties[:2] {
		if !property.Hide {
			t.Fatalf("default imported property %s should be hidden to avoid generated silkscreen DRC noise: %#v", property.Name, property)
		}
	}
	if len(footprint.Texts) == 0 || footprint.Texts[0].Layer != kicadfiles.LayerBSilkS {
		t.Fatalf("text layers not remapped: %#v", footprint.Texts)
	}
	if len(footprint.Graphics) == 0 {
		t.Fatalf("graphics not populated: %#v", footprint.Graphics)
	}
	if graphic := pcb.Drawing(footprint.Graphics[0]); graphic.Layer != kicadfiles.LayerBFab {
		t.Fatalf("graphic layers not remapped: %#v", graphic)
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
	if result.Issues[0].OperationID == "" || result.Issues[0].OperationID != result.Plan.Operations[0].ID {
		t.Fatalf("expected create_project issue operation id: issue=%#v plan=%#v", result.Issues[0], result.Plan.Operations)
	}
}

func TestApplyGeneratedOverwriteDoesNotRequireImportedApproval(t *testing.T) {
	output := t.TempDir()
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"write_project","overwrite":true}
	]}`)
	first := Apply(tx, ApplyOptions{OutputDir: output, Overwrite: true})
	if len(first.Issues) != 0 {
		t.Fatalf("initial apply issues: %#v", first.Issues)
	}
	second := Apply(tx, ApplyOptions{OutputDir: output, Overwrite: true})
	if len(second.Issues) != 0 {
		t.Fatalf("overwrite apply should not require imported approval: %#v", second.Issues)
	}
}

func TestApplyGeneratedOverwriteBlocksHandAuthoredProject(t *testing.T) {
	output := writeImportedApplyProject(t, `(kicad_sch
  (version 20260306)
  (generator "eeschema")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
)`, `(kicad_pcb (version 20260206) (generator "pcbnew") (paper "A4") (layers (0 "F.Cu" signal)) (setup))`)
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"write_project","overwrite":true}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: output, Overwrite: true})
	if len(result.Issues) == 0 || result.Issues[0].Code != reports.CodePreservationConflict {
		t.Fatalf("expected generated overwrite preservation conflict: %#v", result.Issues)
	}
}

func TestApplyGeneratedOverwriteRejectsExistingApplyLock(t *testing.T) {
	output := t.TempDir()
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"write_project","overwrite":true}
	]}`)
	first := Apply(tx, ApplyOptions{OutputDir: output, Overwrite: true})
	if len(first.Issues) != 0 {
		t.Fatalf("initial apply issues: %#v", first.Issues)
	}
	if err := os.WriteFile(filepath.Join(output, applyLockFileName), []byte("pid=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second := Apply(tx, ApplyOptions{OutputDir: output, Overwrite: true})
	if len(second.Issues) == 0 || !strings.Contains(second.Issues[0].Message, "apply lock already exists") {
		t.Fatalf("expected lock issue: %#v", second.Issues)
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
	if result.Issues[0].OperationID == "" || result.Issues[0].OperationID != result.Plan.Operations[1].ID {
		t.Fatalf("expected failing operation id: issue=%#v plan=%#v", result.Issues[0], result.Plan.Operations)
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
	result := Apply(tx, ApplyOptions{OutputDir: dir, AllowImportedMutation: true})
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

func TestApplyImportedRequiresExplicitApproval(t *testing.T) {
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
	  {"op":"add_symbol","ref":"R1","value":"10k","library_id":"Device:R","at":{"x_mm":10,"y_mm":10}},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: dir})
	if len(result.Issues) == 0 || result.Issues[0].Code != reports.CodePreservationConflict ||
		!strings.Contains(result.Issues[0].Message, "disabled by default") {
		t.Fatalf("expected default imported apply gate: %#v", result.Issues)
	}
	after, err := os.ReadFile(schematicPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("blocked imported apply changed schematic:\nbefore=%s\nafter=%s", before, after)
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
	result := Apply(tx, ApplyOptions{OutputDir: dir, AllowImportedMutation: true})
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
	result := Apply(tx, ApplyOptions{OutputDir: dir, AllowImportedMutation: true})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	data, err := os.ReadFile(filepath.Join(dir, "demo.kicad_pcb"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"J1"`, `(size 1.2 1.4)`, `(net 1 "SIG")`, `(segment`} {
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
	result := Apply(tx, ApplyOptions{OutputDir: dir, AllowImportedMutation: true})
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
	result := Apply(tx, ApplyOptions{OutputDir: dir, AllowImportedMutation: true})
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
	result := Apply(tx, ApplyOptions{OutputDir: dir, AllowImportedMutation: true})
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
	result := Apply(tx, ApplyOptions{OutputDir: dir, AllowImportedMutation: true})
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
	if len(got.Pads) != 1 || got.Pads[0].NetName != "SIG" {
		t.Fatalf("pads not reconciled to desired set: %#v", got.Pads)
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

func TestUpsertImportedFootprintRemovesStalePadsAndPreservesUUID(t *testing.T) {
	generator, err := kicadfiles.NewDeterministicIDGenerator("11111111-1111-5111-8111-111111111111", "test")
	if err != nil {
		t.Fatal(err)
	}
	board := &pcb.PCBFile{Footprints: []pcb.Footprint{{
		Reference: "J1",
		Layer:     kicadfiles.LayerFCu,
		Pads: []pcb.Pad{
			{UUID: "pad-1", Name: "1", Type: "smd", Shape: "rect", Size: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)}},
			{UUID: "pad-2", Name: "2", Type: "smd", Shape: "rect", Size: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)}},
		},
	}}}

	upsertImportedFootprint(board, generator, PlaceFootprintOperation{
		Ref:  "J1",
		At:   Point{XMM: 5, YMM: 6},
		Pads: []PadSpec{{Name: "1", XMM: 1, YMM: 2, RotationDeg: 90}},
	})

	pads := board.Footprints[0].Pads
	if len(pads) != 1 {
		t.Fatalf("pads = %#v, want only replacement pad", pads)
	}
	if pads[0].UUID != "pad-1" {
		t.Fatalf("pad UUID = %q, want stable pad-1", pads[0].UUID)
	}
	if pads[0].Position != (kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(2)}) {
		t.Fatalf("pad position = %#v, want updated position", pads[0].Position)
	}
	if pads[0].Rotation != 90 {
		t.Fatalf("pad rotation = %v, want 90", pads[0].Rotation)
	}
}

func TestUpsertImportedFootprintUsesValueProperty(t *testing.T) {
	generator, err := kicadfiles.NewDeterministicIDGenerator("11111111-1111-5111-8111-111111111111", "test")
	if err != nil {
		t.Fatal(err)
	}
	board := &pcb.PCBFile{}
	upsertImportedFootprint(board, generator, PlaceFootprintOperation{
		Ref:                      "R1",
		FootprintID:              "Resistor_SMD:R_0805_2012Metric",
		Value:                    "10k",
		At:                       Point{XMM: 1, YMM: 2},
		HideDefaultFootprintText: true,
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
			if !property.Hide {
				t.Fatalf("value property should be hidden to avoid generated silkscreen DRC noise: %#v", property)
			}
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

func TestUpsertImportedFootprintUpdatesExistingPadLayersWhenSideChanges(t *testing.T) {
	generator, err := kicadfiles.NewDeterministicIDGenerator("11111111-1111-5111-8111-111111111111", "test")
	if err != nil {
		t.Fatal(err)
	}
	board := &pcb.PCBFile{Footprints: []pcb.Footprint{{
		Reference: "U1",
		Layer:     kicadfiles.LayerFCu,
		Pads: []pcb.Pad{{
			UUID:   "pad-1",
			Name:   "1",
			Type:   "smd",
			Shape:  "rect",
			Size:   kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)},
			Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFMask},
		}},
	}}}

	upsertImportedFootprint(board, generator, PlaceFootprintOperation{
		Ref:   "U1",
		Layer: string(kicadfiles.LayerBCu),
		At:    Point{XMM: 1, YMM: 2},
		Pads:  []PadSpec{{Name: "1"}},
	})

	want := []kicadfiles.BoardLayer{kicadfiles.LayerBCu, kicadfiles.LayerBMask}
	if fmt.Sprint(board.Footprints[0].Pads[0].Layers) != fmt.Sprint(want) {
		t.Fatalf("layers = %#v, want %#v", board.Footprints[0].Pads[0].Layers, want)
	}
}

func applyResolverFixture() libraryresolver.LibraryIndex {
	return libraryresolver.LibraryIndex{
		Symbols: map[string]libraryresolver.SymbolRecord{
			"Device:R": {
				LibraryID: "Device:R",
				Name:      "R",
				Pins: []libraryresolver.SymbolPin{
					{Number: "1", Electrical: "passive", Position: kicadfiles.Point{X: kicadfiles.MM(-2.54)}},
					{Number: "2", Electrical: "passive", Position: kicadfiles.Point{X: kicadfiles.MM(2.54)}},
				},
			},
			"Connector:Conn_01x02": {
				LibraryID: "Connector:Conn_01x02",
				Name:      "Conn_01x02",
				Pins: []libraryresolver.SymbolPin{
					{Number: "1", Electrical: "passive", Position: kicadfiles.Point{Y: kicadfiles.MM(-1.27)}},
					{Number: "2", Electrical: "passive", Position: kicadfiles.Point{Y: kicadfiles.MM(1.27)}},
				},
			},
			"Amplifier:DUAL": {
				LibraryID: "Amplifier:DUAL",
				Name:      "DUAL",
				Units:     []libraryresolver.SymbolUnit{{Unit: 1}, {Unit: 2}},
				Pins: []libraryresolver.SymbolPin{
					{Number: "1", Electrical: "input"},
					{Number: "2", Electrical: "output"},
				},
			},
			"Device:STACKED": {
				LibraryID: "Device:STACKED",
				Name:      "STACKED",
				Pins: []libraryresolver.SymbolPin{
					{Number: "1", Electrical: "passive"},
					{Number: "1", Electrical: "passive"},
				},
			},
		},
		Footprints: map[string]libraryresolver.FootprintRecord{
			"Connector_Test:TH_1x02": {
				FootprintID:     "Connector_Test:TH_1x02",
				LibraryNickname: "Connector_Test",
				Name:            "TH_1x02",
				Description:     "test through-hole connector",
				Attributes:      []string{"through_hole"},
				Properties:      map[string]string{"ki_description": "test through-hole connector"},
				Pads: []libraryresolver.FootprintPad{
					{Name: "1", Type: "thru_hole", Shape: "circle", Position: kicadfiles.Point{}, Size: kicadfiles.Point{X: kicadfiles.MM(1.6), Y: kicadfiles.MM(1.6)}, Drill: kicadfiles.MM(0.8), Layers: []kicadfiles.BoardLayer{kicadfiles.LayerAllCu, kicadfiles.LayerAllMask}, PinFunction: "A", PinType: "passive"},
					{Name: "2", Type: "thru_hole", Shape: "circle", Position: kicadfiles.Point{Y: kicadfiles.MM(2.54)}, Size: kicadfiles.Point{X: kicadfiles.MM(1.6), Y: kicadfiles.MM(1.6)}, Drill: kicadfiles.MM(0.8), Layers: []kicadfiles.BoardLayer{kicadfiles.LayerAllCu, kicadfiles.LayerAllMask}, PinFunction: "B", PinType: "passive"},
				},
				Texts: []libraryresolver.FootprintText{{Kind: "user", Text: "TEST", Layer: string(kicadfiles.LayerFSilkS)}},
				Graphics: []libraryresolver.FootprintGraphic{
					{
						Kind:  "line",
						Layer: string(kicadfiles.LayerFSilkS),
						Width: kicadfiles.MM(0.12),
						Start: &kicadfiles.Point{},
						End:   &kicadfiles.Point{X: kicadfiles.MM(1)},
					},
					{
						Kind:   "curve",
						Layer:  string(kicadfiles.LayerFFab),
						Points: []kicadfiles.Point{{}, {X: kicadfiles.MM(1)}, {X: kicadfiles.MM(1), Y: kicadfiles.MM(1)}, {Y: kicadfiles.MM(1)}},
					},
				},
				Models: []string{"${KICAD9_3DMODEL_DIR}/Connector_Test.3dshapes/TH_1x02.wrl"},
			},
			"Resistor_Test:R_0603": {
				FootprintID:     "Resistor_Test:R_0603",
				LibraryNickname: "Resistor_Test",
				Name:            "R_0603",
				Attributes:      []string{"smd"},
				Pads: []libraryresolver.FootprintPad{
					{Name: "1", Type: "smd", Shape: "roundrect", Size: kicadfiles.Point{X: kicadfiles.MM(0.8), Y: kicadfiles.MM(0.9)}, Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFMask}},
					{Name: "2", Type: "smd", Shape: "roundrect", Position: kicadfiles.Point{X: kicadfiles.MM(1.6)}, Size: kicadfiles.Point{X: kicadfiles.MM(0.8), Y: kicadfiles.MM(0.9)}, Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFMask}},
				},
				Texts: []libraryresolver.FootprintText{{Kind: "user", Text: "R", Layer: string(kicadfiles.LayerFSilkS)}},
				Graphics: []libraryresolver.FootprintGraphic{
					{
						Kind:  "line",
						Layer: string(kicadfiles.LayerFSilkS),
						Width: kicadfiles.MM(0.12),
						Start: &kicadfiles.Point{},
						End:   &kicadfiles.Point{X: kicadfiles.MM(1)},
					},
					{
						Kind:   "curve",
						Layer:  string(kicadfiles.LayerFFab),
						Points: []kicadfiles.Point{{}, {X: kicadfiles.MM(1)}, {X: kicadfiles.MM(1), Y: kicadfiles.MM(1)}, {Y: kicadfiles.MM(1)}},
					},
				},
			},
		},
	}
}

func mustGenerator(t *testing.T) kicadfiles.IDGenerator {
	t.Helper()
	generator, err := kicadfiles.NewDeterministicIDGenerator("11111111-1111-5111-8111-111111111111", "test")
	if err != nil {
		t.Fatal(err)
	}
	return generator
}

func containsBoardLayer(layers []kicadfiles.BoardLayer, want kicadfiles.BoardLayer) bool {
	for _, layer := range layers {
		if layer == want {
			return true
		}
	}
	return false
}

func propertyValues(properties []schematic.Property) map[string]string {
	values := map[string]string{}
	for _, property := range properties {
		values[property.Name] = property.Value
	}
	return values
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
