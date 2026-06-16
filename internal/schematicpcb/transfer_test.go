package schematicpcb

import (
	"encoding/json"
	"math"
	"testing"

	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/transactions"
)

func TestFromDesignGeneratesPlacementTransactionWithNetHints(t *testing.T) {
	design := transferFixtureDesign()
	index := transferFixtureLibraryIndex()

	result := FromDesign(design, Options{LibraryIndex: &index, OriginXMM: 30, OriginYMM: 40, Columns: 1})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if result.SymbolCount != 2 || result.AssignedCount != 2 || result.PlacedCount != 2 || result.NetHintCount != 2 {
		t.Fatalf("counts = %#v", result)
	}
	if len(result.Transaction.Operations) != 3 {
		t.Fatalf("operations = %#v", result.Transaction.Operations)
	}
	var first transactions.PlaceFootprintOperation
	if err := json.Unmarshal(result.Transaction.Operations[0].Raw, &first); err != nil {
		t.Fatal(err)
	}
	if first.Ref != "J1" || first.FootprintID != "Connector_Test:TH_1x02" || first.At.XMM != 30 || first.At.YMM != 40 {
		t.Fatalf("first placement = %#v", first)
	}
	if len(first.Pads) != 2 || first.Pads[0].Net == nil || *first.Pads[0].Net != "BUS_A" {
		t.Fatalf("first pads = %#v", first.Pads)
	}
	var second transactions.PlaceFootprintOperation
	if err := json.Unmarshal(result.Transaction.Operations[1].Raw, &second); err != nil {
		t.Fatal(err)
	}
	if second.Ref != "J2" || second.Pads[0].Net == nil || *second.Pads[0].Net != "BUS_A" {
		t.Fatalf("second placement = %#v", second)
	}
	if result.Transaction.Operations[2].Op != transactions.OpWriteProject {
		t.Fatalf("last operation = %#v", result.Transaction.Operations[2])
	}
}

func TestFromDesignGeneratesFallbackNetHintsForUnlabeledConnections(t *testing.T) {
	design := transferFixtureDesign()
	design.Schematic.Labels = nil
	index := transferFixtureLibraryIndex()

	result := FromDesign(design, Options{LibraryIndex: &index})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	var first transactions.PlaceFootprintOperation
	if err := json.Unmarshal(result.Transaction.Operations[0].Raw, &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Pads) != 2 || first.Pads[0].Net == nil || *first.Pads[0].Net != "Net-(J1-Pad1)" {
		t.Fatalf("first pads = %#v", first.Pads)
	}
}

func TestFromDesignIncludesHiddenPinsInNetHints(t *testing.T) {
	design := transferFixtureDesign()
	index := transferFixtureLibraryIndex()
	pins := index.Symbols["Connector:Conn_01x02"].Pins
	pins[0].Hidden = true
	record := index.Symbols["Connector:Conn_01x02"]
	record.Pins = pins
	index.Symbols["Connector:Conn_01x02"] = record

	result := FromDesign(design, Options{LibraryIndex: &index})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	var first transactions.PlaceFootprintOperation
	if err := json.Unmarshal(result.Transaction.Operations[0].Raw, &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Pads) != 2 || first.Pads[0].Net == nil || *first.Pads[0].Net != "BUS_A" {
		t.Fatalf("hidden J1 pin 1 net hint missing: %#v", first.Pads)
	}
}

func TestFromDesignGroupsMultiUnitSymbolsByReference(t *testing.T) {
	design := transferFixtureDesign()
	duplicate := design.Schematic.Symbols[1]
	duplicate.Unit = 2
	duplicate.Position = kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(20)}
	design.Schematic.Symbols = append(design.Schematic.Symbols, duplicate)
	index := transferFixtureLibraryIndex()

	result := FromDesign(design, Options{LibraryIndex: &index})
	if result.SymbolCount != 3 || result.AssignedCount != 2 || result.PlacedCount != 2 {
		t.Fatalf("result = %#v", result)
	}
	for _, issue := range result.Issues {
		if issue.Code == "DUPLICATE_REFERENCE" {
			t.Fatalf("unexpected duplicate reference issue: %#v", result.Issues)
		}
	}
}

func TestFromDesignUsesKiCadSchematicRotationForPinAnchors(t *testing.T) {
	design := transferFixtureDesign()
	design.Schematic.Symbols[1].Rotation = 90
	design.Schematic.Wires = []schematic.Wire{
		{Points: []kicadfiles.Point{{X: kicadfiles.MM(12.54), Y: kicadfiles.MM(10)}, {X: kicadfiles.MM(40), Y: kicadfiles.MM(10)}}},
	}
	index := transferFixtureLibraryIndex()

	result := FromDesign(design, Options{LibraryIndex: &index})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	var first transactions.PlaceFootprintOperation
	if err := json.Unmarshal(result.Transaction.Operations[0].Raw, &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Pads) != 2 || first.Pads[1].Net == nil || *first.Pads[1].Net != "BUS_A" {
		t.Fatalf("rotated J1 pin 2 net hint missing: %#v", first.Pads)
	}
}

func TestFromDesignUsesKiCadSchematicMirrorForPinAnchors(t *testing.T) {
	design := transferFixtureDesign()
	design.Schematic.Symbols[1].Mirror = schematic.SymbolMirrorY
	design.Schematic.Wires = []schematic.Wire{
		{Points: []kicadfiles.Point{{X: kicadfiles.MM(10), Y: kicadfiles.MM(7.46)}, {X: kicadfiles.MM(40), Y: kicadfiles.MM(10)}}},
	}
	design.Schematic.Labels = []schematic.Label{{Text: "MIRROR_A", Position: kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(7.46)}}}
	index := transferFixtureLibraryIndex()

	result := FromDesign(design, Options{LibraryIndex: &index})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	var first transactions.PlaceFootprintOperation
	if err := json.Unmarshal(result.Transaction.Operations[0].Raw, &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Pads) != 2 || first.Pads[1].Net == nil || *first.Pads[1].Net != "MIRROR_A" {
		t.Fatalf("mirrored J1 pin 2 net hint missing: %#v", first.Pads)
	}
}

func TestFromDesignRejectsHierarchicalSchematics(t *testing.T) {
	design := transferFixtureDesign()
	design.Schematic.Sheets = []schematic.Sheet{{Filename: "child.kicad_sch"}}

	result := FromDesign(design, Options{LibraryIndex: &libraryresolver.LibraryIndex{}})
	if len(result.Issues) != 1 || result.Issues[0].Path != "schematic.sheets" {
		t.Fatalf("hierarchy issue not reported: %#v", result.Issues)
	}
	if len(result.Transaction.Operations) != 0 {
		t.Fatalf("operations should not be emitted for unsupported hierarchy: %#v", result.Transaction.Operations)
	}
}

func TestFromDesignWithoutLibraryIndexStillPlacesAssignedFootprints(t *testing.T) {
	result := FromDesign(transferFixtureDesign(), Options{})
	if !result.RequiresLibraries {
		t.Fatalf("RequiresLibraries = false")
	}
	if result.AssignedCount != 2 || result.PlacedCount != 2 || result.NetHintCount != 0 {
		t.Fatalf("result = %#v", result)
	}
	var place transactions.PlaceFootprintOperation
	if err := json.Unmarshal(result.Transaction.Operations[0].Raw, &place); err != nil {
		t.Fatal(err)
	}
	if len(place.Pads) != 0 {
		t.Fatalf("pads = %#v", place.Pads)
	}
}

func TestFromDesignReportsMissingFootprints(t *testing.T) {
	design := transferFixtureDesign()
	design.Schematic.Symbols = append(design.Schematic.Symbols, schematic.SchematicSymbol{
		LibraryID:  "Device:R",
		Reference:  "R1",
		Value:      "10k",
		Position:   kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(10)},
		Properties: []schematic.Property{{Name: "Reference", Value: "R1"}},
	})

	result := FromDesign(design, Options{LibraryIndex: &libraryresolver.LibraryIndex{}})
	if result.SymbolCount != 3 || result.AssignedCount != 2 {
		t.Fatalf("counts = %#v", result)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Code == "MISSING_FOOTPRINT" && issue.Path == "symbols.R1.Footprint" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing footprint issue not found: %#v", result.Issues)
	}
}

func TestFromDesignReportsOperationSerializationFailures(t *testing.T) {
	design := transferFixtureDesign()
	index := transferFixtureLibraryIndex()

	result := FromDesign(design, Options{LibraryIndex: &index, OriginXMM: math.NaN()})
	if result.PlacedCount != 0 {
		t.Fatalf("PlacedCount = %d, want 0", result.PlacedCount)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Code == "VALIDATION_FAILED" && issue.Path == "transaction.place_footprint.J1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("serialization issue not found: %#v", result.Issues)
	}
}

func transferFixtureDesign() kicaddesign.Design {
	trueValue := true
	return kicaddesign.Design{
		Name: "demo",
		Schematic: &schematic.SchematicFile{
			Symbols: []schematic.SchematicSymbol{
				{
					LibraryID:  "Connector:Conn_01x02",
					Reference:  "J2",
					Value:      "OUT",
					Position:   kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(10)},
					OnBoard:    &trueValue,
					Properties: []schematic.Property{{Name: "Reference", Value: "J2"}, {Name: "Value", Value: "OUT"}, {Name: "Footprint", Value: "Connector_Test:TH_1x02"}},
				},
				{
					LibraryID:  "Connector:Conn_01x02",
					Reference:  "J1",
					Value:      "IN",
					Position:   kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
					OnBoard:    &trueValue,
					Properties: []schematic.Property{{Name: "Reference", Value: "J1"}, {Name: "Value", Value: "IN"}, {Name: "Footprint", Value: "Connector_Test:TH_1x02"}},
				},
			},
			Wires: []schematic.Wire{
				{Points: []kicadfiles.Point{{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}, {X: kicadfiles.MM(40), Y: kicadfiles.MM(10)}}},
			},
			Labels: []schematic.Label{{Text: "BUS_A", Position: kicadfiles.Point{X: kicadfiles.MM(25), Y: kicadfiles.MM(10)}}},
		},
	}
}

func transferFixtureLibraryIndex() libraryresolver.LibraryIndex {
	return libraryresolver.LibraryIndex{
		Symbols: map[string]libraryresolver.SymbolRecord{
			"Connector:Conn_01x02": {
				LibraryID: "Connector:Conn_01x02",
				Pins: []libraryresolver.SymbolPin{
					{Number: "1", Unit: 1, BodyStyle: 1, Position: kicadfiles.Point{}},
					{Number: "2", Unit: 1, BodyStyle: 1, Position: kicadfiles.Point{Y: kicadfiles.MM(2.54)}},
				},
			},
		},
		Footprints: map[string]libraryresolver.FootprintRecord{
			"Connector_Test:TH_1x02": {
				FootprintID: "Connector_Test:TH_1x02",
				Pads: []libraryresolver.FootprintPad{
					{Name: "1", Type: "thru_hole", Shape: "circle", Size: kicadfiles.Point{X: kicadfiles.MM(1.6), Y: kicadfiles.MM(1.6)}, Drill: kicadfiles.MM(0.8)},
					{Name: "2", Type: "thru_hole", Shape: "circle", Position: kicadfiles.Point{Y: kicadfiles.MM(2.54)}, Size: kicadfiles.Point{X: kicadfiles.MM(1.6), Y: kicadfiles.MM(1.6)}, Drill: kicadfiles.MM(0.8)},
				},
			},
		},
	}
}
