package designapi

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/schematiclayout"
)

func TestBuilderWritesGeneratedSchematicHierarchy(t *testing.T) {
	builder, err := New(Options{
		Name:     "hierarchy_demo",
		DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed:     "hierarchy_demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range []struct {
		ref string
		x   float64
	}{
		{ref: "R1", x: 30},
		{ref: "R2", x: 300},
	} {
		options := SymbolOptions{
			Reference: symbol.ref,
			Role:      "resistor",
			Value:     "10k",
			LibraryID: "Device:R",
			Position:  kicadfiles.Point{X: kicadfiles.MM(symbol.x), Y: kicadfiles.MM(50)},
			Pins: []PinSpec{
				{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54)}},
				{Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(2.54)}},
			},
		}
		if symbol.ref == "R1" {
			options.Rotation = 90
		}
		if _, err := builder.AddSymbol(options); err != nil {
			t.Fatal(err)
		}
	}
	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "R2", Pin: "1"}, "LONG_NET"); err != nil {
		t.Fatal(err)
	}
	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "1"}, Endpoint{Reference: "R2", Pin: "2"}, "PWR"); err != nil {
		t.Fatal(err)
	}
	if err := builder.SetSchematicHierarchy(SchematicHierarchy{
		Sheets: []SchematicSheet{
			{ID: "left", Name: "Left", Filename: "sch/left.kicad_sch", References: []string{"R1"}},
			{ID: "right", Name: "Right", Filename: "sch/right.kicad_sch", References: []string{"R2"}},
		},
		CrossSheetNets: []SchematicCrossSheetNet{{
			Name:      "LONG_NET",
			Endpoints: []Endpoint{{Reference: "R1", Pin: "2"}, {Reference: "R2", Pin: "1"}},
		}, {
			Name:        "PWR",
			GlobalScope: true,
			Endpoints:   []Endpoint{{Reference: "R1", Pin: "1"}, {Reference: "R2", Pin: "2"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(t.TempDir(), "hierarchy_demo")
	if _, err := builder.WriteSchematicProject(root, kicaddesign.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	read, err := kicaddesign.ReadProjectDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if read.Schematic == nil || len(read.Schematic.Sheets) != 2 {
		t.Fatalf("root sheets = %#v", read.Schematic)
	}
	if len(read.SheetFiles) != 2 {
		t.Fatalf("child sheets = %#v", read.SheetFiles)
	}
	if len(read.Schematic.SheetInstances) != 1 || read.Schematic.SheetInstances[0].Path != "/" {
		t.Fatalf("root sheet instances = %#v, want only the root path", read.Schematic.SheetInstances)
	}
	sheetUUIDByFilename := make(map[string]kicadfiles.UUID, len(read.Schematic.Sheets))
	orientedHierarchicalLabel := false
	for index, sheet := range read.Schematic.Sheets {
		wantPath := "/" + string(read.Schematic.UUID)
		wantPage := strconv.Itoa(index + 2)
		if len(sheet.Instances) != 1 || sheet.Instances[0].Project != "" || sheet.Instances[0].Path != wantPath || sheet.Instances[0].Page != wantPage {
			t.Fatalf("root sheet %s instances = %#v, want empty project path %s page %s", sheet.Filename, sheet.Instances, wantPath, wantPage)
		}
		if len(sheet.Pins) != 1 || sheet.Pins[0].Text != "LONG_NET" || sheet.Pins[0].Kind != schematic.SheetPinPassive {
			t.Fatalf("root sheet %s pins = %#v, want one explicit LONG_NET interface", sheet.Filename, sheet.Pins)
		}
		sheetUUIDByFilename[sheet.Filename] = sheet.UUID
	}
	for _, child := range read.SheetFiles {
		if len(child.Symbols) != 1 {
			t.Fatalf("child %s symbols = %#v", child.Filename, child.Symbols)
		}
		if child.SheetInstances != nil {
			t.Fatalf("child %s sheet instances = %#v, want none", child.Filename, child.SheetInstances)
		}
		if !child.OmitRootSheetInstances {
			t.Fatalf("child %s did not preserve root sheet instance omission", child.Filename)
		}
		sheetUUID, ok := sheetUUIDByFilename[child.Filename]
		if !ok {
			t.Fatalf("child %s has no matching root sheet", child.Filename)
		}
		instance := child.Symbols[0].Instances
		wantInstancePath := "/" + string(read.Schematic.UUID) + "/" + string(sheetUUID)
		if len(instance) != 1 || instance[0].Project != "hierarchy_demo" || instance[0].Path != wantInstancePath {
			t.Fatalf("child %s symbol instances = %#v, want path %s", child.Filename, instance, wantInstancePath)
		}
		hierarchicalLabels := 0
		globalPowerLabels := 0
		connectedHierarchicalLabel := false
		for _, label := range child.Labels {
			if label.Text == "LONG_NET" && label.Kind == schematic.LabelHierarchical {
				hierarchicalLabels++
				if label.Rotation != 0 {
					orientedHierarchicalLabel = true
				}
				wantRight := label.Rotation == 180 || label.Rotation == 270
				if gotRight := containsFold(label.Justify, "right"); gotRight != wantRight {
					t.Fatalf("child %s global label orientation = rotation %v justify %#v, want right=%t", child.Filename, label.Rotation, label.Justify, wantRight)
				}
				for _, wire := range child.Wires {
					if len(wire.Points) >= 2 && (wire.Points[0] == label.Position || wire.Points[len(wire.Points)-1] == label.Position) {
						connectedHierarchicalLabel = true
						break
					}
				}
			}
			if label.Text == "PWR" && label.Kind == schematic.LabelGlobal {
				globalPowerLabels++
			}
		}
		if hierarchicalLabels != 1 || globalPowerLabels != 1 {
			t.Fatalf("child %s labels = %#v", child.Filename, child.Labels)
		}
		for _, label := range child.Labels {
			if label.Kind != schematic.LabelGlobal {
				continue
			}
			if len(label.Fields) != 1 || label.Fields[0].Name != "Intersheetrefs" || label.Fields[0].Value != "${INTERSHEET_REFS}" || !label.Fields[0].Hidden || label.Fields[0].Position != label.Position {
				t.Fatalf("child %s global label %s intersheet field = %#v", child.Filename, label.Text, label.Fields)
			}
		}
		if !connectedHierarchicalLabel {
			t.Fatalf("child %s hierarchical label was not moved onto a connecting wire: labels=%#v wires=%#v", child.Filename, child.Labels, child.Wires)
		}
		request, result := schematiclayout.AdaptSchematic(child)
		result = schematiclayout.Validate(result, request)
		readability := schematiclayout.BuildReport(result, schematiclayout.ProfileStandard)
		if !readability.Passed || readability.ErrorCount != 0 {
			t.Fatalf("child %s transformed-symbol readability = %#v diagnostics=%#v", child.Filename, readability, result.Diagnostics)
		}
		for _, code := range []string{"wire_symbol_overlap", "wire_pin_overlap", "label_overlap"} {
			if readability.OverlapCounts[code] != 0 {
				t.Fatalf("child %s %s count = %d, report=%#v", child.Filename, code, readability.OverlapCounts[code], readability)
			}
		}
	}
	if !orientedHierarchicalLabel {
		t.Fatal("generated hierarchy did not preserve any outward-oriented hierarchical label")
	}
	for _, child := range read.SheetFiles {
		for _, symbol := range child.Symbols {
			if symbol.Reference == "R1" && symbol.Rotation != 90 {
				t.Fatalf("transformed hierarchy symbol rotation = %v, want 90", symbol.Rotation)
			}
		}
	}
}

func TestHierarchyRootSheetStepXReservesFacingLabelWidths(t *testing.T) {
	sheets := []SchematicSheet{{ID: "left"}, {ID: "right"}}
	interfaces := map[string][]hierarchySheetInterface{
		"left":  {{Name: "INTERNAL_001", Side: hierarchyInterfaceRight}},
		"right": {{Name: "SUPPORT_PRIMITIVE_000_RT", Side: hierarchyInterfaceLeft}},
	}
	got := hierarchyRootSheetStepX(sheets, interfaces, 2)
	want := hierarchySheetWidth + 2*hierarchyInterfaceStubMM +
		float64(len("INTERNAL_001")+len("SUPPORT_PRIMITIVE_000_RT"))*hierarchyDefaultTextCharacterMM +
		hierarchyInterfacePitchMM
	if got != want {
		t.Fatalf("hierarchy sheet step = %.2f mm, want %.2f mm", got, want)
	}
	labelChannel := got - hierarchySheetWidth - 2*hierarchyInterfaceStubMM
	labelWidths := want - hierarchySheetWidth - 2*hierarchyInterfaceStubMM - hierarchyInterfacePitchMM
	if labelChannel-labelWidths+1e-9 < hierarchyInterfacePitchMM {
		t.Fatalf("hierarchy label channel = %.2f mm for %.2f mm text, want at least %.2f mm clearance", labelChannel, labelWidths, hierarchyInterfacePitchMM)
	}
}

func TestHierarchyRootSheetStepXReservesUnpairedLabelWidths(t *testing.T) {
	const longUnpairedLabel = "UNPAIRED_INTERFACE_LABEL_THAT_EXTENDS_ACROSS_THE_CHANNEL"
	sheets := []SchematicSheet{{ID: "left"}, {ID: "right"}}
	interfaces := map[string][]hierarchySheetInterface{
		"left": {
			{Name: "SHORT", Side: hierarchyInterfaceRight},
			{Name: longUnpairedLabel, Side: hierarchyInterfaceRight},
		},
		"right": {{Name: "OTHER", Side: hierarchyInterfaceLeft}},
	}
	got := hierarchyRootSheetStepX(sheets, interfaces, 2)
	want := hierarchySheetWidth + 2*hierarchyInterfaceStubMM +
		float64(len(longUnpairedLabel))*hierarchyDefaultTextCharacterMM + hierarchyInterfacePitchMM
	if got != want {
		t.Fatalf("hierarchy sheet step with unpaired label = %.2f mm, want %.2f mm", got, want)
	}
}

func TestHierarchyBindsFootprintsToDeterministicSymbolInstancePaths(t *testing.T) {
	builder, err := New(Options{
		Name: "hierarchy_paths", Seed: "hierarchy_paths",
		DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
	})
	if err != nil {
		t.Fatal(err)
	}
	for index, reference := range []string{"R1", "R2"} {
		_, err := builder.AddSymbol(SymbolOptions{
			Reference: reference, LibraryID: "Device:R", Value: "10k",
			Position: kicadfiles.Point{X: kicadfiles.MM(float64(20 + index*20)), Y: kicadfiles.MM(20)},
			Pins:     []PinSpec{{Number: "1"}, {Number: "2"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := builder.AssignFootprint(reference, "Test:R"); err != nil {
			t.Fatal(err)
		}
		if _, err := builder.PlaceFootprint(reference, PlaceFootprintOptions{
			Position: kicadfiles.Point{X: kicadfiles.MM(float64(10 + index*10)), Y: kicadfiles.MM(10)},
		}); err != nil {
			t.Fatal(err)
		}
		flat := builder.footprint(reference)
		state, stateErr := builder.symbolState(reference)
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		symbolUUID := builder.design.Schematic.Symbols[state.symbolIndex].UUID
		if flat == nil || flat.Path != "/"+string(symbolUUID) {
			t.Fatalf("flat footprint %s path = %#v, want symbol UUID path", reference, flat)
		}
	}
	if err := builder.SetSchematicHierarchy(SchematicHierarchy{Sheets: []SchematicSheet{
		{ID: "left", Name: "Left", References: []string{"R1"}},
		{ID: "right", Name: "Right", References: []string{"R2"}},
	}}); err != nil {
		t.Fatal(err)
	}
	design := builder.Design()
	if err := builder.applySchematicHierarchy(&design); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, child := range design.SheetFiles {
		for _, symbol := range child.Symbols {
			paths[symbol.Reference] = strings.TrimRight(symbol.Instances[0].Path, "/") + "/" + string(symbol.UUID)
		}
	}
	for _, footprint := range design.PCB.Footprints {
		if footprint.Path != paths[footprint.Reference] {
			t.Fatalf("footprint %s path = %q, want %q", footprint.Reference, footprint.Path, paths[footprint.Reference])
		}
	}
}

func TestBuilderWritesUnitAwareGeneratedHierarchy(t *testing.T) {
	builder, err := New(Options{
		Name:     "unit_hierarchy_demo",
		DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed:     "unit_hierarchy_demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	for unit, x := range map[int]float64{1: 30, 2: 300} {
		if _, err := builder.AddSymbol(SymbolOptions{
			Reference: "U1",
			Unit:      unit,
			Value:     "DUAL",
			LibraryID: "Device:R",
			Position:  kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(50)},
			Pins: []PinSpec{
				{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54)}},
				{Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(2.54)}},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := builder.Connect(Endpoint{Reference: "U1", Unit: 1, Pin: "2"}, Endpoint{Reference: "U1", Unit: 2, Pin: "1"}, "UNIT_NET"); err != nil {
		t.Fatal(err)
	}
	if err := builder.SetSchematicHierarchy(SchematicHierarchy{
		Sheets: []SchematicSheet{
			{ID: "unit-a", Name: "Unit A", Filename: "sch/unit-a.kicad_sch", Symbols: []SchematicSymbolRef{{Reference: "U1", Unit: 1}}},
			{ID: "unit-b", Name: "Unit B", Filename: "sch/unit-b.kicad_sch", Symbols: []SchematicSymbolRef{{Reference: "U1", Unit: 2}}},
		},
		CrossSheetNets: []SchematicCrossSheetNet{{
			Name:      "UNIT_NET",
			Endpoints: []Endpoint{{Reference: "U1", Unit: 1, Pin: "2"}, {Reference: "U1", Unit: 2, Pin: "1"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "unit_hierarchy_demo")
	if _, err := builder.WriteSchematicProject(root, kicaddesign.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	read, err := kicaddesign.ReadProjectDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.SheetFiles) != 2 {
		t.Fatalf("child sheets = %#v", read.SheetFiles)
	}
	for _, sheet := range read.Schematic.Sheets {
		if len(sheet.Pins) != 1 || sheet.Pins[0].Text != "UNIT_NET" {
			t.Fatalf("root sheet %s pins = %#v, want UNIT_NET", sheet.Filename, sheet.Pins)
		}
	}
	seenUnits := map[int]bool{}
	for _, child := range read.SheetFiles {
		if len(child.Symbols) != 1 {
			t.Fatalf("child %s symbols = %#v", child.Filename, child.Symbols)
		}
		seenUnits[child.Symbols[0].Unit] = true
		foundLabel := false
		for _, label := range child.Labels {
			if label.Text == "UNIT_NET" && label.Kind == schematic.LabelHierarchical {
				foundLabel = true
			}
		}
		if !foundLabel {
			t.Fatalf("child %s missing unit-aware hierarchical label", child.Filename)
		}
	}
	if !seenUnits[1] || !seenUnits[2] {
		t.Fatalf("child units = %#v, want 1 and 2", seenUnits)
	}
}

func TestNoConnectsForSheetUsesActualPinAnchors(t *testing.T) {
	anchor := kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(50)}
	symbols := []schematic.SchematicSymbol{{Reference: "J1", PinAnchors: []kicadfiles.Point{anchor}}}
	noConnects := []schematic.NoConnect{
		{UUID: "connected", Position: anchor},
		{UUID: "nearby_but_not_pin", Position: kicadfiles.Point{X: kicadfiles.MM(42), Y: kicadfiles.MM(50)}},
	}
	selected := noConnectsForSheet(noConnects, symbols, map[kicadfiles.UUID]struct{}{})
	if len(selected) != 1 || selected[0].UUID != "connected" {
		t.Fatalf("selected no-connects = %#v, want only the pin-anchor marker", selected)
	}
}

func TestHierarchySymbolBodyRecoversEmbeddedWriterGeometry(t *testing.T) {
	body, ok := hierarchySymbolBody(nil, schematic.SchematicSymbol{LibraryID: "Device:LED", Unit: 1, BodyStyle: 1})
	if !ok {
		t.Fatal("expected embedded Device:LED body geometry")
	}
	want := schematiclayout.Rect{
		MinX: -kicadfiles.MM(4.572), MinY: -kicadfiles.MM(2.286),
		MaxX: kicadfiles.MM(1.27), MaxY: kicadfiles.MM(1.27),
	}
	if body != want {
		t.Fatalf("embedded hierarchy body = %#v, want %#v", body, want)
	}
}

func TestTranslateSchematicMovesEveryPositionedConnectivityPrimitive(t *testing.T) {
	start := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(20)}
	delta := kicadfiles.Point{X: kicadfiles.MM(5), Y: kicadfiles.MM(-3)}
	file := &schematic.SchematicFile{
		Wires:      []schematic.Wire{{Points: []kicadfiles.Point{start}}},
		Buses:      []schematic.Bus{{Points: []kicadfiles.Point{start}}},
		Polylines:  []schematic.Polyline{{Points: []kicadfiles.Point{start}}},
		BusEntries: []schematic.BusEntry{{Position: start}},
		Texts:      []schematic.Text{{Position: start}},
		Labels: []schematic.Label{{
			Position: start,
			Fields:   []schematic.Field{{Position: start}},
		}},
		Junctions:  []schematic.Junction{{Position: start}},
		NoConnects: []schematic.NoConnect{{Position: start}},
		Sheets: []schematic.Sheet{{
			Position:   start,
			Properties: []schematic.Property{{Position: start}},
			Pins:       []schematic.SheetPin{{Position: start}},
		}},
	}

	translateSchematic(file, delta)
	want := kicadfiles.Point{X: start.X + delta.X, Y: start.Y + delta.Y}
	for name, got := range map[string]kicadfiles.Point{
		"wire":           file.Wires[0].Points[0],
		"bus":            file.Buses[0].Points[0],
		"polyline":       file.Polylines[0].Points[0],
		"bus entry":      file.BusEntries[0].Position,
		"text":           file.Texts[0].Position,
		"label":          file.Labels[0].Position,
		"label field":    file.Labels[0].Fields[0].Position,
		"junction":       file.Junctions[0].Position,
		"no-connect":     file.NoConnects[0].Position,
		"sheet":          file.Sheets[0].Position,
		"sheet property": file.Sheets[0].Properties[0].Position,
		"sheet pin":      file.Sheets[0].Pins[0].Position,
	} {
		if got != want {
			t.Fatalf("translated %s = %#v, want %#v", name, got, want)
		}
	}
}
