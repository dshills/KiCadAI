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

func TestVerifiedTransferUSB4125PowerOnlyPads(t *testing.T) {
	pads, ok := verifiedTransferPadSpecs("Connector_USB:USB_C_Receptacle_GCT_USB4125-xx-x_6P_TopMnt_Horizontal", map[string]string{
		"A5": "CC1",
		"SH": "SHIELD",
	})
	if !ok {
		t.Fatal("missing USB-C GCT transfer template")
	}
	if len(pads) != 10 {
		t.Fatalf("pad count = %d, want 10", len(pads))
	}
	want := map[string]int{"A5": 1, "SH": 4}
	for _, pad := range pads {
		if count, ok := want[pad.Name]; ok {
			net := "SHIELD"
			if pad.Name == "A5" {
				net = "CC1"
			}
			if pad.Net == nil || *pad.Net != net {
				t.Fatalf("pad %s net = %#v, want %s in %#v", pad.Name, pad.Net, net, pads)
			}
			if count == 1 {
				delete(want, pad.Name)
			} else {
				want[pad.Name] = count - 1
			}
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing pads: %#v in %#v", want, pads)
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

func TestFromDesignReportsMultiUnitLibraryIDConflicts(t *testing.T) {
	design := transferFixtureDesign()
	duplicate := design.Schematic.Symbols[1]
	duplicate.LibraryID = "Connector:Other"
	design.Schematic.Symbols = append(design.Schematic.Symbols, duplicate)
	index := transferFixtureLibraryIndex()

	result := FromDesign(design, Options{LibraryIndex: &index})
	found := false
	for _, issue := range result.Issues {
		if issue.Path == "schematic.symbols[2].lib_id" {
			found = true
		}
	}
	if !found {
		t.Fatalf("library conflict issue not found: %#v", result.Issues)
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

func TestFromDesignPlacesLoadedChildSheetSymbols(t *testing.T) {
	design := transferFixtureDesign()
	design.Schematic.Sheets = []schematic.Sheet{{Name: "Child", Filename: "child.kicad_sch"}}
	child := transferFixtureDesign().Schematic
	child.Filename = "child.kicad_sch"
	child.Symbols[0].Reference = "J4"
	child.Symbols[0].Value = "CHILD_OUT"
	child.Symbols[0].Properties = []schematic.Property{{Name: "Reference", Value: "J4"}, {Name: "Value", Value: "CHILD_OUT"}, {Name: "Footprint", Value: "Connector_Test:TH_1x02"}}
	child.Symbols[1].Reference = "J3"
	child.Symbols[1].Value = "CHILD_IN"
	child.Symbols[1].Properties = []schematic.Property{{Name: "Reference", Value: "J3"}, {Name: "Value", Value: "CHILD_IN"}, {Name: "Footprint", Value: "Connector_Test:TH_1x02"}}
	child.Labels = []schematic.Label{{Text: "CHILD_A", Position: kicadfiles.Point{X: kicadfiles.MM(25), Y: kicadfiles.MM(10)}}}
	design.SheetFiles = []*schematic.SchematicFile{child}
	index := transferFixtureLibraryIndex()

	result := FromDesign(design, Options{LibraryIndex: &index})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if result.SymbolCount != 4 || result.AssignedCount != 4 || result.PlacedCount != 4 || result.NetHintCount != 4 {
		t.Fatalf("result = %#v", result)
	}
	var childPlace transactions.PlaceFootprintOperation
	if err := json.Unmarshal(result.Transaction.Operations[2].Raw, &childPlace); err != nil {
		t.Fatal(err)
	}
	if childPlace.Ref != "J3" || len(childPlace.Pads) != 2 || childPlace.Pads[0].Net == nil || *childPlace.Pads[0].Net != "Child/CHILD_A" {
		t.Fatalf("child placement = %#v", childPlace)
	}
}

func TestFromDesignKeepsGlobalLabelsUnscopedAcrossSheetFiles(t *testing.T) {
	design := transferFixtureDesign()
	design.Schematic.Symbols = nil
	design.Schematic.Wires = nil
	design.Schematic.Labels = nil
	child := transferFixtureDesign().Schematic
	child.Filename = "child.kicad_sch"
	child.Symbols = child.Symbols[:1]
	child.Symbols[0].Reference = "J1"
	child.Symbols[0].Properties = []schematic.Property{{Name: "Reference", Value: "J1"}, {Name: "Value", Value: "IN"}, {Name: "Footprint", Value: "Connector_Test:TH_1x02"}}
	child.Wires = []schematic.Wire{{Points: []kicadfiles.Point{{X: kicadfiles.MM(40), Y: kicadfiles.MM(10)}, {X: kicadfiles.MM(25), Y: kicadfiles.MM(10)}}}}
	child.Labels = []schematic.Label{{Kind: schematic.LabelGlobal, Text: "GLOBAL_A", Position: kicadfiles.Point{X: kicadfiles.MM(25), Y: kicadfiles.MM(10)}}}
	design.SheetFiles = []*schematic.SchematicFile{child}
	index := transferFixtureLibraryIndex()

	result := FromDesign(design, Options{LibraryIndex: &index})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	var place transactions.PlaceFootprintOperation
	if err := json.Unmarshal(result.Transaction.Operations[0].Raw, &place); err != nil {
		t.Fatal(err)
	}
	if len(place.Pads) != 2 || place.Pads[0].Net == nil || *place.Pads[0].Net != "GLOBAL_A" {
		t.Fatalf("global net hint = %#v", place.Pads)
	}
}

func TestFromDesignRejectsMultiInstantiatedSheetFiles(t *testing.T) {
	design := transferFixtureDesign()
	design.Schematic.Sheets = []schematic.Sheet{
		{Filename: "child.kicad_sch"},
		{Filename: "./child.kicad_sch"},
	}
	child := transferFixtureDesign().Schematic
	child.Filename = "child.kicad_sch"
	design.SheetFiles = []*schematic.SchematicFile{child}

	result := FromDesign(design, Options{LibraryIndex: &libraryresolver.LibraryIndex{}})
	if len(result.Issues) != 1 || result.Issues[0].Path != "schematic.sheets.child.kicad_sch" {
		t.Fatalf("multi-instance issue not reported: %#v", result.Issues)
	}
	if len(result.Transaction.Operations) != 0 {
		t.Fatalf("operations should not be emitted for unsupported multi-instance sheets: %#v", result.Transaction.Operations)
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

func TestFromDesignWithoutLibraryIndexUsesVerifiedTemplatesForNetHints(t *testing.T) {
	trueValue := true
	pin := func(x, y float64) kicadfiles.Point {
		return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)}
	}
	design := kicaddesign.Design{
		Name: "verified_templates",
		Schematic: &schematic.SchematicFile{
			Symbols: []schematic.SchematicSymbol{
				{
					LibraryID:  "Device:R",
					Reference:  "R1",
					Value:      "1k",
					Position:   pin(10, 10),
					OnBoard:    &trueValue,
					Properties: []schematic.Property{{Name: "Reference", Value: "R1"}, {Name: "Value", Value: "1k"}, {Name: "Footprint", Value: "Resistor_SMD:R_0805_2012Metric"}},
				},
				{
					LibraryID:  "Device:R",
					Reference:  "R2",
					Value:      "1k",
					Position:   pin(30, 10),
					OnBoard:    &trueValue,
					Properties: []schematic.Property{{Name: "Reference", Value: "R2"}, {Name: "Value", Value: "1k"}, {Name: "Footprint", Value: "Resistor_SMD:R_0805_2012Metric"}},
				},
			},
			Wires:  []schematic.Wire{{Points: []kicadfiles.Point{pin(10, 13.81), pin(30, 13.81)}}},
			Labels: []schematic.Label{{Text: "SIG", Position: pin(20, 13.81)}},
		},
	}

	result := FromDesign(design, Options{})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if !result.RequiresLibraries {
		t.Fatalf("RequiresLibraries = false, want synthetic no-index evidence")
	}
	if result.AssignedCount != 2 || result.PlacedCount != 2 || result.NetHintCount != 2 {
		t.Fatalf("result = %#v", result)
	}
	var place transactions.PlaceFootprintOperation
	if err := json.Unmarshal(result.Transaction.Operations[0].Raw, &place); err != nil {
		t.Fatal(err)
	}
	if len(place.Pads) != 2 || place.Pads[0].Net == nil || *place.Pads[0].Net != "SIG" {
		t.Fatalf("place pads = %#v, want verified template pad net hint", place.Pads)
	}
}

func TestVerifiedTransferPinHeaderPadsAreCentered(t *testing.T) {
	pads, ok := verifiedTransferPadSpecs("Connector_PinHeader_2.54mm:PinHeader_1x04_P2.54mm_Vertical", nil)
	if !ok || len(pads) != 4 {
		t.Fatalf("pads = %#v ok=%v", pads, ok)
	}
	if pads[0].YMM != -3.81 || pads[3].YMM != 3.81 {
		t.Fatalf("pin header pad origins = %#v, want centered 1x04 row", pads)
	}
}

func TestVerifiedTransferSOIC8PadsProvideNetHints(t *testing.T) {
	pads, ok := verifiedTransferPadSpecs("Package_SO:SOIC-8_3.9x4.9mm_P1.27mm", map[string]string{"3": "SDA", "4": "SCL"})
	if !ok || len(pads) != 8 {
		t.Fatalf("pads = %#v ok=%v, want SOIC-8 verified transfer pads", pads, ok)
	}
	if pads[0].WidthMM != 1.55 || pads[0].HeightMM != 0.6 {
		t.Fatalf("SOIC pad size = %.2fx%.2f, want 1.55x0.60", pads[0].WidthMM, pads[0].HeightMM)
	}
	if pads[2].Name != "3" || pads[2].Net == nil || *pads[2].Net != "SDA" {
		t.Fatalf("pad 3 = %#v, want SDA net hint", pads[2])
	}
	if pads[7].Name != "5" {
		t.Fatalf("right row order pads = %#v, want KiCad SOIC descending right-side pad names", pads)
	}
}

func TestVerifiedTransferBMP280AndAP2112PadsProvideNetHints(t *testing.T) {
	sot, ok := verifiedTransferPadSpecs("Package_TO_SOT_SMD:SOT-23-5", map[string]string{"1": "VIN", "5": "VOUT"})
	if !ok || len(sot) != 5 || sot[0].Net == nil || *sot[0].Net != "VIN" || sot[4].Net == nil || *sot[4].Net != "VOUT" {
		t.Fatalf("SOT-23-5 pads = %#v", sot)
	}
	if sot[0].XMM != -1.1375 || sot[0].YMM != -0.95 || sot[4].XMM != 1.1375 || sot[4].YMM != -0.95 {
		t.Fatalf("SOT-23-5 geometry = %#v", sot)
	}

	lga, ok := verifiedTransferPadSpecs("Package_LGA:Bosch_LGA-8_2x2.5mm_P0.65mm_ClockwisePinNumbering", map[string]string{"1": "GND", "3": "SDA", "4": "SCL", "8": "VCC"})
	if !ok || len(lga) != 8 || lga[0].Net == nil || *lga[0].Net != "GND" || lga[7].Net == nil || *lga[7].Net != "VCC" {
		t.Fatalf("BMP280 LGA-8 pads = %#v", lga)
	}
	if lga[2].XMM != 0.325 || lga[2].YMM != -0.8 || lga[7].XMM != -0.975 || lga[7].YMM != 0.8 {
		t.Fatalf("BMP280 LGA-8 geometry = %#v", lga)
	}
}

func TestVerifiedTransferSOT223PreservesDuplicateOutputPads(t *testing.T) {
	pads, ok := verifiedTransferPadSpecs("Package_TO_SOT_SMD:SOT-223-3_TabPin2", map[string]string{"1": "GND", "2": "VOUT", "3": "VIN"})
	if !ok || len(pads) != 4 {
		t.Fatalf("SOT-223 pads = %#v ok=%v, want four verified copper shapes", pads, ok)
	}
	pinTwoCount := 0
	for _, pad := range pads {
		if pad.Name != "2" {
			continue
		}
		pinTwoCount++
		if pad.Net == nil || *pad.Net != "VOUT" {
			t.Fatalf("SOT-223 pad 2 = %#v, want VOUT net hint", pad)
		}
	}
	if pinTwoCount != 2 || pads[3].WidthMM != 3.8 || pads[3].HeightMM != 2.4 {
		t.Fatalf("SOT-223 duplicate tab geometry = %#v", pads)
	}
}

func TestFromDesignFallsBackToVerifiedTemplatesWhenIndexMissesFootprint(t *testing.T) {
	trueValue := true
	point := func(x, y float64) kicadfiles.Point {
		return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)}
	}
	design := kicaddesign.Design{
		Name: "index_miss",
		Schematic: &schematic.SchematicFile{
			Symbols: []schematic.SchematicSymbol{{
				LibraryID:  "Device:R",
				Reference:  "R1",
				Value:      "1k",
				Position:   point(10, 10),
				OnBoard:    &trueValue,
				Properties: []schematic.Property{{Name: "Reference", Value: "R1"}, {Name: "Value", Value: "1k"}, {Name: "Footprint", Value: "Resistor_SMD:R_0805_2012Metric"}},
			}},
			Labels: []schematic.Label{{Text: "SIG", Position: point(10, 13.81)}},
		},
	}
	index := libraryresolver.LibraryIndex{}

	result := FromDesign(design, Options{LibraryIndex: &index})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if result.RequiresLibraries {
		t.Fatalf("RequiresLibraries = true")
	}
	if result.NetHintCount != 1 {
		t.Fatalf("result = %#v, want verified-template net hint after resolver miss", result)
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
