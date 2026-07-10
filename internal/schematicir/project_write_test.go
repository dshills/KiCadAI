package schematicir

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"kicadai/internal/evaluate"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/kicadfiles/sexpr"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/schematiclayout"
	"kicadai/internal/transactions"
)

func TestSchematicIRWritesReadableProject(t *testing.T) {
	tests := []struct {
		name        string
		fileName    string
		projectName string
	}{
		{name: "LED indicator", fileName: "led_indicator.json", projectName: "led_indicator"},
		{name: "USB-C LED indicator", fileName: "usb_c_led_indicator.json", projectName: "usb_c_led_indicator"},
		{name: "I2C sensor regulator", fileName: "i2c_sensor_3v3_regulator.json", projectName: "i2c_sensor_3v3_regulator"},
		{name: "Vector bus", fileName: "vector_bus.json", projectName: "vector_bus"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testSchematicIRWritesReadableProject(t, tc.fileName, tc.projectName)
		})
	}
}

func TestSchematicIRWritesGlobalPortLabel(t *testing.T) {
	document := loadExampleDocument(t, "led_indicator.json")
	document.Circuit.Ports = []Port{{Name: "VIN_EXT", Direction: PortDirectionInput, Net: "VIN", Side: SideLeft}}
	tx, issues := ToProjectTransaction(document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("transaction issues: %+v", issues)
	}
	outputDir := filepath.Join(t.TempDir(), "port_fixture")
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir, Overwrite: true})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("apply issues: %+v", apply.Issues)
	}
	path := filepath.Join(outputDir, "led_indicator.kicad_sch")
	generated, err := schematic.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated schematic: %v", err)
	}
	found := false
	for _, label := range generated.Labels {
		if label.Text == "VIN_EXT" && label.Kind == schematic.LabelGlobal {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("generated schematic missing VIN_EXT global label: %#v", generated.Labels)
	}
	report, err := evaluate.Schematic(path)
	if err != nil {
		t.Fatalf("evaluate generated schematic: %v", err)
	}
	if check := schematicIRCheckByName(report.Checks, "schematic_validation"); check.Status != evaluate.CheckPassed {
		t.Fatalf("schematic_validation check = %#v", check)
	}
}

func TestSchematicIRWritesOversizedProjectAsHierarchy(t *testing.T) {
	document := loadExampleDocument(t, "led_indicator.json")
	document.Policy.Acceptance = AcceptanceReadable
	for index := 0; index < 80; index++ {
		document.Circuit.Components = append(document.Circuit.Components, Component{
			ID:        fmt.Sprintf("extra_%d", index),
			Ref:       fmt.Sprintf("R%d", index+10),
			Role:      ComponentRoleResistor,
			Symbol:    "Device:R",
			Value:     "10k",
			Footprint: "Resistor_SMD:R_0603_1608Metric",
			Pins:      []Pin{{Number: "1", Role: PinRoleOutput}, {Number: "2", Role: PinRoleInput}},
		})
	}
	for index := 1; index < 80; index++ {
		document.Circuit.Nets = append(document.Circuit.Nets, Net{
			Name:    fmt.Sprintf("EXTRA_%d", index),
			Role:    NetRoleSignal,
			Connect: []EndpointRef{EndpointRef(fmt.Sprintf("extra_%d.1", index-1)), EndpointRef(fmt.Sprintf("extra_%d.2", index))},
		})
	}
	tx, issues := ToProjectTransaction(document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("oversized transaction issues: %+v", issues)
	}
	var writeOp transactions.WriteProjectOperation
	if err := json.Unmarshal(tx.Operations[len(tx.Operations)-1].Raw, &writeOp); err != nil {
		t.Fatal(err)
	}
	if writeOp.Hierarchy == nil || len(writeOp.Hierarchy.Sheets) < 2 {
		t.Fatalf("missing hierarchy payload: %#v raw=%s", writeOp, tx.Operations[len(tx.Operations)-1].Raw)
	}
	outputDir := filepath.Join(t.TempDir(), "oversized")
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir, Overwrite: true})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("oversized apply issues: %+v", apply.Issues)
	}
	read, err := kicaddesign.ReadProjectDirectory(outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if read.Schematic == nil || len(read.Schematic.Sheets) < 2 || len(read.SheetFiles) < 2 {
		t.Fatalf("hierarchy was not written: root=%#v children=%d", read.Schematic, len(read.SheetFiles))
	}
	rootReport, err := evaluate.Schematic(filepath.Join(outputDir, "led_indicator.kicad_sch"))
	if err != nil {
		t.Fatalf("root schematic evaluation: %v", err)
	}
	for _, name := range []string{"schematic_validation", "schematic_electrical"} {
		if check := schematicIRCheckByName(rootReport.Checks, name); check.Status != evaluate.CheckPassed {
			t.Fatalf("root %s check = %#v", name, check)
		}
	}
	for _, child := range read.SheetFiles {
		if err := schematic.Validate(*child); err != nil {
			t.Fatalf("child %s validation: %v", child.Filename, err)
		}
		childReport, err := evaluate.Schematic(filepath.Join(outputDir, child.Filename))
		if err != nil {
			t.Fatalf("child %s evaluation: %v", child.Filename, err)
		}
		for _, name := range []string{"schematic_validation", "schematic_electrical"} {
			if check := schematicIRCheckByName(childReport.Checks, name); check.Status != evaluate.CheckPassed {
				t.Fatalf("child %s %s check = %#v", child.Filename, name, check)
			}
		}
		request, layoutResult := schematiclayout.AdaptSchematic(child)
		layoutResult = schematiclayout.Validate(layoutResult, request)
		readability := schematiclayout.BuildReport(layoutResult, schematiclayout.ProfileStandard)
		unexpectedOverlap := false
		for code, count := range readability.OverlapCounts {
			if code != "text_symbol_overlap" && count > 0 {
				unexpectedOverlap = true
			}
		}
		if !readability.Passed || readability.ErrorCount != 0 || unexpectedOverlap {
			t.Fatalf("child %s readability: %#v diagnostics=%#v", child.Filename, readability, layoutResult.Diagnostics)
		}
	}
}

func TestSchematicIRWritesResolverBackedExternalSymbolProject(t *testing.T) {
	document := loadExampleDocument(t, "external_connector_indicator.json")
	for index := range document.Circuit.Components {
		for pin := range document.Circuit.Components[index].Pins {
			document.Circuit.Components[index].Pins[pin].OffsetXMM = nil
			document.Circuit.Components[index].Pins[pin].OffsetYMM = nil
		}
	}
	index, loadIssues := libraryresolver.Load(context.Background(), libraryresolver.LibraryRoots{
		SymbolsRoot: filepath.Join("testdata", "symbols"),
	}, libraryresolver.LoadOptions{})
	if reports.HasBlockingIssue(loadIssues) {
		t.Fatalf("fixture library issues: %+v", loadIssues)
	}
	tx, issues := ToProjectTransactionWithLibraryIndex(document, &index)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("project transaction issues: %+v", issues)
	}
	outputDir := filepath.Join(t.TempDir(), "external_connector_indicator")
	apply := transactions.Apply(tx, transactions.ApplyOptions{
		OutputDir:    outputDir,
		Overwrite:    true,
		LibraryIndex: &index,
	})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("apply issues: %+v", apply.Issues)
	}
	schematicPath := filepath.Join(outputDir, "external_connector_indicator.kicad_sch")
	generated, err := schematic.ReadFile(schematicPath)
	if err != nil {
		t.Fatalf("read generated schematic: %v", err)
	}
	var connector *schematic.SchematicSymbol
	for index := range generated.Symbols {
		if generated.Symbols[index].Reference == "J1" {
			connector = &generated.Symbols[index]
			break
		}
	}
	if connector == nil || connector.BodyBounds == nil {
		t.Fatalf("resolver-backed connector geometry missing: %#v", connector)
	}
	if len(connector.PinAnchors) != 4 || connector.PinAnchors[0] == connector.PinAnchors[1] {
		t.Fatalf("resolver-backed connector pin anchors = %#v", connector.PinAnchors)
	}
	if _, ok := schematic.EmbeddedSymbolTemplate("Connector_Generic:Conn_02x02_Odd_Even"); ok {
		t.Fatal("fixture symbol unexpectedly became a built-in template")
	}
	if len(generated.LibSymbols) == 0 || len(generated.LibSymbols[0].Body) == 0 {
		t.Fatalf("resolver-backed embedded body missing: %#v", generated.LibSymbols)
	}
	report, err := evaluate.Schematic(schematicPath)
	if err != nil {
		t.Fatalf("evaluate generated schematic: %v", err)
	}
	for _, name := range []string{"schematic_validation", "schematic_electrical"} {
		if check := schematicIRCheckByName(report.Checks, name); check.Status != evaluate.CheckPassed {
			t.Fatalf("%s check = %#v", name, check)
		}
	}
	request, layoutResult := schematiclayout.AdaptSchematic(&generated)
	layoutResult = schematiclayout.Validate(layoutResult, request)
	readability := schematiclayout.BuildReport(layoutResult, schematiclayout.ProfileStandard)
	if !readability.Passed || readability.ErrorCount != 0 || readability.WarningCount != 0 || len(readability.OverlapCounts) != 0 {
		t.Fatalf("external symbol readability failed: %#v diagnostics=%#v", readability, layoutResult.Diagnostics)
	}
}

func TestSchematicIRWritesResolverBackedMultiUnitProject(t *testing.T) {
	document := multiUnitFixtureDocument()
	index, loadIssues := libraryresolver.Load(context.Background(), libraryresolver.LibraryRoots{
		SymbolsRoot: filepath.Join("testdata", "symbols"),
	}, libraryresolver.LoadOptions{})
	if reports.HasBlockingIssue(loadIssues) {
		t.Fatalf("fixture library issues: %+v", loadIssues)
	}
	tx, issues := ToProjectTransactionWithLibraryIndex(document, &index)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("project transaction issues: %+v", issues)
	}
	outputDir := filepath.Join(t.TempDir(), "multi_unit_fixture")
	apply := transactions.Apply(tx, transactions.ApplyOptions{
		OutputDir:    outputDir,
		Overwrite:    true,
		LibraryIndex: &index,
	})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("apply issues: %+v", apply.Issues)
	}
	path := filepath.Join(outputDir, "multi_unit_fixture.kicad_sch")
	generated, err := schematic.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated schematic: %v", err)
	}
	var units []int
	for _, symbol := range generated.Symbols {
		if symbol.Reference == "U1" {
			units = append(units, symbol.Unit)
			if symbol.BodyBounds == nil || len(symbol.PinAnchors) != 2 {
				t.Fatalf("multi-unit symbol geometry = %#v", symbol)
			}
		}
	}
	sort.Ints(units)
	if !reflect.DeepEqual(units, []int{1, 2}) {
		t.Fatalf("U1 units = %#v, want [1 2]", units)
	}
	report, err := evaluate.Schematic(path)
	if err != nil {
		t.Fatalf("evaluate generated schematic: %v", err)
	}
	for _, name := range []string{"schematic_validation", "schematic_electrical"} {
		if check := schematicIRCheckByName(report.Checks, name); check.Status != evaluate.CheckPassed {
			t.Fatalf("%s check = %#v", name, check)
		}
	}
	request, layoutResult := schematiclayout.AdaptSchematic(&generated)
	layoutResult = schematiclayout.Validate(layoutResult, request)
	readability := schematiclayout.BuildReport(layoutResult, schematiclayout.ProfileStandard)
	if !readability.Passed || readability.WarningCount != 0 || len(readability.OverlapCounts) != 0 {
		t.Fatalf("multi-unit readability failed: %#v diagnostics=%#v", readability, layoutResult.Diagnostics)
	}
}

func TestSchematicIRWritesResolverBackedInheritedSymbolProject(t *testing.T) {
	document := inheritedSymbolFixtureDocument()
	index, loadIssues := libraryresolver.Load(context.Background(), libraryresolver.LibraryRoots{
		SymbolsRoot: filepath.Join("testdata", "symbols"),
	}, libraryresolver.LoadOptions{})
	if reports.HasBlockingIssue(loadIssues) {
		t.Fatalf("fixture library issues: %+v", loadIssues)
	}
	derived, ok := index.Symbols["Amplifier:Derived"]
	if !ok || !derived.Inherited || strings.Contains(derived.Raw, `(extends "Base")`) {
		t.Fatalf("inherited resolver record = %#v", derived)
	}
	tx, issues := ToProjectTransactionWithLibraryIndex(document, &index)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("project transaction issues: %+v", issues)
	}
	outputDir := filepath.Join(t.TempDir(), "inherited_symbol_fixture")
	apply := transactions.Apply(tx, transactions.ApplyOptions{
		OutputDir:    outputDir,
		Overwrite:    true,
		LibraryIndex: &index,
	})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("apply issues: %+v", apply.Issues)
	}
	path := filepath.Join(outputDir, "inherited_symbol_fixture.kicad_sch")
	generated, err := schematic.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated schematic: %v", err)
	}
	for _, embedded := range generated.LibSymbols {
		if embedded.LibraryID != "Amplifier:Derived" {
			continue
		}
		rendered, renderErr := sexpr.Format(embedded.Body)
		if renderErr != nil {
			t.Fatalf("render inherited embedded body: %v", renderErr)
		}
		if strings.Contains(rendered, `(extends "Base")`) || !strings.Contains(rendered, "(rectangle") || !strings.Contains(rendered, "(pin") {
			t.Fatalf("embedded inherited body was not materialized: %s", rendered)
		}
		return
	}
	t.Fatal("generated schematic did not contain Amplifier:Derived embedded symbol")
}

func inheritedSymbolFixtureDocument() Document {
	document := *NewDocument()
	document.Metadata.Name = "inherited_symbol_fixture"
	document.Metadata.Title = "Inherited symbol fixture"
	document.Circuit.Components = []Component{
		{ID: "u1", Ref: "U1", Role: ComponentRoleIC, Symbol: "Amplifier:Derived", Value: "Derived", Pins: []Pin{{Number: "1"}, {Number: "2"}}},
	}
	document.Circuit.Nets = []Net{{Name: "LINK", Role: NetRoleSignal, Connect: []EndpointRef{"u1.1", "u1.2"}}}
	return document
}

func multiUnitFixtureDocument() Document {
	document := *NewDocument()
	document.Metadata.Name = "multi_unit_fixture"
	document.Metadata.Title = "Multi-unit schematic fixture"
	document.Metadata.Description = "Resolver-backed multi-unit IR fixture."
	document.Layout.Rules.PreferLabelsForLongNets = boolPtr(false)
	document.Policy.Acceptance = AcceptanceReadable
	document.Layout.Groups = []Group{
		{ID: "connector", Role: GroupRoleConnectorStage, Members: []string{"j1"}, Rank: 0},
		{ID: "unit_a", Role: GroupRoleProcessingStage, Members: []string{"u1a"}, Rank: 1},
		{ID: "unit_b", Role: GroupRoleProcessingStage, Members: []string{"u1b"}, Rank: 2},
	}
	document.Circuit.Components = []Component{
		{ID: "u1a", Ref: "U1", Unit: "1", Role: ComponentRoleIC, Symbol: "Amplifier:DUAL", Value: "DUAL", Pins: []Pin{{Number: "1"}, {Number: "2"}}},
		{ID: "u1b", Ref: "U1", Unit: "2", Role: ComponentRoleIC, Symbol: "Amplifier:DUAL", Value: "DUAL", Pins: []Pin{{Number: "1"}, {Number: "2"}}},
		{ID: "j1", Ref: "J1", Role: ComponentRoleInputConnector, Symbol: "Connector_Generic:Conn_02x02_Odd_Even", Value: "IO", Pins: []Pin{{Number: "1"}, {Number: "2"}, {Number: "3"}, {Number: "4"}}},
	}
	document.Circuit.Nets = []Net{
		{Name: "IN", Role: NetRoleSignal, Connect: []EndpointRef{"j1.1", "u1a.1"}, UseLabel: boolPtr(true)},
		{Name: "LINK", Role: NetRoleSignal, Connect: []EndpointRef{"u1a.2", "u1b.1"}, UseLabel: boolPtr(true)},
		{Name: "OUT", Role: NetRoleSignal, Connect: []EndpointRef{"u1b.2", "j1.2"}, UseLabel: boolPtr(true)},
		{Name: "NC3", Role: NetRoleNoConnect, Connect: []EndpointRef{"j1.3"}},
		{Name: "NC4", Role: NetRoleNoConnect, Connect: []EndpointRef{"j1.4"}},
	}
	return document
}

func TestSchematicIRWritesAdversarialTopologyProjects(t *testing.T) {
	tests := []struct {
		name string
		doc  Document
	}{
		{name: "feedback cycle", doc: adversarialCycleDocument()},
		{name: "high fanout", doc: adversarialFanoutDocument()},
		{name: "disconnected islands", doc: adversarialIslandsDocument()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testSchematicIRTopologyProject(t, tc.doc)
		})
	}
}

func TestSchematicIRWritesGeneratedArbitraryTopologyCorpus(t *testing.T) {
	for _, seed := range []int64{7, 19, 43} {
		t.Run(fmt.Sprintf("seed_%d", seed), func(t *testing.T) {
			document := generatedArbitraryTopologyDocument(seed)
			tx, issues := ToProjectTransaction(document)
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("transaction issues: %+v", issues)
			}
			outputDir := filepath.Join(t.TempDir(), document.Metadata.Name)
			apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir, Overwrite: true})
			if reports.HasBlockingIssue(apply.Issues) {
				t.Fatalf("apply issues: %+v", apply.Issues)
			}
			read, err := kicaddesign.ReadProjectDirectory(outputDir)
			if err != nil {
				t.Fatalf("read generated project: %v", err)
			}
			paths := []string{filepath.Join(outputDir, document.Metadata.Name+".kicad_sch")}
			for _, child := range read.SheetFiles {
				paths = append(paths, filepath.Join(outputDir, child.Filename))
			}
			for _, path := range paths {
				file, readErr := schematic.ReadFile(path)
				if readErr != nil {
					t.Fatalf("read %s: %v", path, readErr)
				}
				if file.Paper.Name != "A3" {
					t.Fatalf("%s paper = %q, want the layout-selected A3 page", path, file.Paper.Name)
				}
				request, result := schematiclayout.AdaptSchematic(&file)
				result = schematiclayout.Validate(result, request)
				readability := schematiclayout.BuildReport(result, schematiclayout.ProfileStrict)
				if !readability.Passed {
					t.Fatalf("%s readability: %#v diagnostics=%#v", path, readability, result.Diagnostics)
				}
			}
		})
	}
}

func generatedArbitraryTopologyDocument(seed int64) Document {
	random := rand.New(rand.NewSource(seed))
	document := *NewDocument()
	document.Metadata.Name = fmt.Sprintf("generated_topology_%d", seed)
	document.Metadata.Title = "Generated arbitrary topology fixture"
	document.Metadata.Description = "Deterministic high-fanout cyclic schematic corpus fixture."
	document.Policy.Acceptance = AcceptanceReadable
	document.Layout.Rules.PreferLabelsForLongNets = boolPtr(true)
	for index := 0; index < 12; index++ {
		id := fmt.Sprintf("r%d", index+1)
		document.Circuit.Components = append(document.Circuit.Components, Component{
			ID: id, Ref: fmt.Sprintf("R%d", index+1), Role: ComponentRoleResistor, Symbol: "Device:R", Value: "10k",
			Body: &BodyGeometry{MinXMM: -1.016, MinYMM: -2.54, MaxXMM: 1.016, MaxYMM: 2.54},
			Pins: []Pin{{Number: "1"}, {Number: "2"}},
		})
	}
	for netIndex := 0; netIndex < 3; netIndex++ {
		net := Net{Name: fmt.Sprintf("FABRIC_%d", netIndex+1), Role: NetRoleSignal, UseLabel: boolPtr(true)}
		for componentIndex := 0; componentIndex < len(document.Circuit.Components); componentIndex++ {
			if componentIndex%3 == netIndex {
				net.Connect = append(net.Connect, EndpointRef(fmt.Sprintf("r%d.1", componentIndex+1)))
			}
			if (componentIndex+1)%3 == netIndex {
				net.Connect = append(net.Connect, EndpointRef(fmt.Sprintf("r%d.2", componentIndex+1)))
			}
		}
		random.Shuffle(len(net.Connect), func(i, j int) { net.Connect[i], net.Connect[j] = net.Connect[j], net.Connect[i] })
		document.Circuit.Nets = append(document.Circuit.Nets, net)
	}
	return document
}

func TestSchematicIRWritesLongLabelStressProject(t *testing.T) {
	document := longLabelStressDocument()
	tx, issues := ToProjectTransaction(document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("transaction issues: %+v", issues)
	}
	outputDir := filepath.Join(t.TempDir(), "long_label_stress")
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir, Overwrite: true})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("apply issues: %+v", apply.Issues)
	}
	path := filepath.Join(outputDir, "long_label_stress.kicad_sch")
	generated, err := schematic.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated schematic: %v", err)
	}
	report, err := evaluate.Schematic(path)
	if err != nil {
		t.Fatalf("evaluate generated schematic: %v", err)
	}
	for _, name := range []string{"schematic_validation", "schematic_electrical"} {
		if check := schematicIRCheckByName(report.Checks, name); check.Status != evaluate.CheckPassed {
			t.Fatalf("%s check = %#v", name, check)
		}
	}
	request, layoutResult := schematiclayout.AdaptSchematic(&generated)
	layoutResult = schematiclayout.Validate(layoutResult, request)
	readability := schematiclayout.BuildReport(layoutResult, schematiclayout.ProfileStandard)
	if !readability.Passed || readability.WarningCount != 0 || len(readability.OverlapCounts) != 0 {
		t.Fatalf("long-label readability failed: %#v diagnostics=%#v", readability, layoutResult.Diagnostics)
	}
}

func longLabelStressDocument() Document {
	document := *NewDocument()
	document.Metadata.Name = "long_label_stress"
	document.Metadata.Title = "Long label stress fixture"
	document.Metadata.Description = "Long-label cyclic schematic IR fixture."
	document.Layout.Rules.PreferLabelsForLongNets = boolPtr(true)
	document.Layout.Rules.MinGroupSpacingMM = floatPtr(16)
	document.Layout.Rules.MinComponentSpacingMM = floatPtr(9)
	document.Policy.Acceptance = AcceptanceReadable
	document.Layout.Groups = make([]Group, 0, 6)
	for index := 1; index <= 6; index++ {
		id := fmt.Sprintf("r%d", index)
		document.Layout.Groups = append(document.Layout.Groups, Group{ID: "stage_" + id, Role: GroupRoleProcessingStage, Members: []string{id}, Rank: index - 1})
		document.Circuit.Components = append(document.Circuit.Components, Component{
			ID: id, Ref: fmt.Sprintf("R%d", index), Role: ComponentRoleResistor, Symbol: "Device:R",
			Value: "100k", Pins: []Pin{{Number: "1"}, {Number: "2"}},
		})
	}
	for index := 1; index <= 6; index++ {
		next := index%6 + 1
		document.Circuit.Nets = append(document.Circuit.Nets, Net{
			Name: fmt.Sprintf("LONG_ANALOG_SIGNAL_STAGE_%02d_TO_STAGE_%02d", index, next), Role: NetRoleSignal,
			Connect:  []EndpointRef{EndpointRef(fmt.Sprintf("r%d.%d", index, 1)), EndpointRef(fmt.Sprintf("r%d.%d", next, 2))},
			UseLabel: boolPtr(true),
		})
	}
	return document
}

func testSchematicIRTopologyProject(t *testing.T, document Document) {
	t.Helper()
	tx, issues := ToProjectTransaction(document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("transaction issues: %+v", issues)
	}
	outputDir := filepath.Join(t.TempDir(), document.Metadata.Name)
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir, Overwrite: true})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("apply issues: %+v", apply.Issues)
	}
	path := filepath.Join(outputDir, document.Metadata.Name+".kicad_sch")
	generated, err := schematic.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated schematic: %v", err)
	}
	report, err := evaluate.Schematic(path)
	if err != nil {
		t.Fatalf("evaluate generated schematic: %v", err)
	}
	for _, name := range []string{"schematic_validation", "schematic_electrical"} {
		if check := schematicIRCheckByName(report.Checks, name); check.Status != evaluate.CheckPassed {
			t.Fatalf("%s check = %#v", name, check)
		}
	}
	request, layoutResult := schematiclayout.AdaptSchematic(&generated)
	layoutResult = schematiclayout.Validate(layoutResult, request)
	readability := schematiclayout.BuildReport(layoutResult, schematiclayout.ProfileStandard)
	if !readability.Passed || readability.ErrorCount != 0 || readability.WarningCount != 0 || len(readability.OverlapCounts) != 0 {
		t.Fatalf("topology readability failed: %#v diagnostics=%#v", readability, layoutResult.Diagnostics)
	}
}

func adversarialCycleDocument() Document {
	document := adversarialDocument("ir_feedback_cycle")
	for index := 1; index <= 3; index++ {
		document.Circuit.Components = append(document.Circuit.Components, resistorComponent(index))
	}
	document.Circuit.Nets = []Net{
		{Name: "N12", Role: NetRoleSignal, Connect: []EndpointRef{"r1.2", "r2.1"}},
		{Name: "N23", Role: NetRoleSignal, Connect: []EndpointRef{"r2.2", "r3.1"}},
		{Name: "N31", Role: NetRoleFeedback, Connect: []EndpointRef{"r3.2", "r1.1"}},
	}
	return document
}

func adversarialFanoutDocument() Document {
	document := adversarialDocument("ir_high_fanout")
	document.Layout.Placements = []Placement{{Target: "source", Orientation: OrientationRotated180}}
	document.Circuit.Components = append(document.Circuit.Components, Component{
		ID: "source", Ref: "J1", Role: ComponentRoleInputConnector, Symbol: "Connector_Generic:Conn_01x04", Value: "BUS",
		Body: &BodyGeometry{MinXMM: -3.81, MinYMM: -7.62, MaxXMM: 3.81, MaxYMM: 7.62},
		Pins: []Pin{{Number: "1"}, {Number: "2"}, {Number: "3"}, {Number: "4"}},
	})
	for index := 1; index <= 4; index++ {
		document.Circuit.Components = append(document.Circuit.Components, resistorComponent(index))
	}
	document.Circuit.Nets = []Net{
		{Name: "BUS", Role: NetRoleSignal, Connect: []EndpointRef{"source.1", "r1.1", "r2.1", "r3.1", "r4.1"}, Label: "BUS", UseLabel: boolPtr(false)},
		{Name: "NC_SOURCE_2", Role: NetRoleNoConnect, Connect: []EndpointRef{"source.2"}},
		{Name: "NC_SOURCE_3", Role: NetRoleNoConnect, Connect: []EndpointRef{"source.3"}},
		{Name: "NC_SOURCE_4", Role: NetRoleNoConnect, Connect: []EndpointRef{"source.4"}},
		{Name: "NC_R1_2", Role: NetRoleNoConnect, Connect: []EndpointRef{"r1.2"}},
		{Name: "NC_R2_2", Role: NetRoleNoConnect, Connect: []EndpointRef{"r2.2"}},
		{Name: "NC_R3_2", Role: NetRoleNoConnect, Connect: []EndpointRef{"r3.2"}},
		{Name: "NC_R4_2", Role: NetRoleNoConnect, Connect: []EndpointRef{"r4.2"}},
	}
	return document
}

func adversarialIslandsDocument() Document {
	document := adversarialDocument("ir_disconnected_islands")
	for index := 1; index <= 4; index++ {
		document.Circuit.Components = append(document.Circuit.Components, resistorComponent(index))
	}
	document.Circuit.Nets = []Net{
		{Name: "ISLAND_A", Role: NetRoleSignal, Connect: []EndpointRef{"r1.1", "r2.1"}, UseLabel: boolPtr(false)},
		{Name: "ISLAND_A_RETURN", Role: NetRoleSignal, Connect: []EndpointRef{"r1.2", "r2.2"}, UseLabel: boolPtr(false)},
		{Name: "ISLAND_B", Role: NetRoleSignal, Connect: []EndpointRef{"r3.1", "r4.1"}, UseLabel: boolPtr(false)},
		{Name: "ISLAND_B_RETURN", Role: NetRoleSignal, Connect: []EndpointRef{"r3.2", "r4.2"}, UseLabel: boolPtr(false)},
	}
	return document
}

func adversarialDocument(name string) Document {
	document := *NewDocument()
	document.Metadata.Name = name
	document.Metadata.Title = name
	document.Metadata.Description = "Adversarial schematic IR topology fixture."
	document.Layout.Rules.PreferLabelsForLongNets = boolPtr(false)
	document.Layout.Rules.MinGroupSpacingMM = floatPtr(18)
	document.Layout.Rules.MinComponentSpacingMM = floatPtr(10)
	document.Policy.Acceptance = AcceptanceReadable
	return document
}

func resistorComponent(index int) Component {
	id := fmt.Sprintf("r%d", index)
	return Component{ID: id, Ref: fmt.Sprintf("R%d", index), Role: ComponentRoleResistor, Symbol: "Device:R", Value: "10k", Body: &BodyGeometry{MinXMM: -1.016, MinYMM: -2.54, MaxXMM: 1.016, MaxYMM: 2.54}, Pins: []Pin{{Number: "1"}, {Number: "2"}}}
}

func testSchematicIRWritesReadableProject(t *testing.T, fileName string, projectName string) {
	t.Helper()
	document := loadExampleDocument(t, fileName)
	tx, issues := ToProjectTransaction(document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("project transaction issues: %+v", issues)
	}
	if tx.Operations[len(tx.Operations)-1].Op != transactions.OpWriteProject {
		t.Fatalf("last operation = %s, want %s", tx.Operations[len(tx.Operations)-1].Op, transactions.OpWriteProject)
	}
	var writeOp transactions.WriteProjectOperation
	if err := json.Unmarshal(tx.Operations[len(tx.Operations)-1].Raw, &writeOp); err != nil {
		t.Fatalf("decode write_project operation: %v", err)
	}
	if !writeOp.SchematicOnly {
		t.Fatalf("write_project schematic_only = false")
	}

	outputDir := filepath.Join(t.TempDir(), projectName)
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir, Overwrite: true})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("apply issues: %+v", apply.Issues)
	}
	projectPath := filepath.Join(outputDir, projectName+".kicad_pro")
	schematicPath := filepath.Join(outputDir, projectName+".kicad_sch")
	for _, path := range []string{projectPath, schematicPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected generated file %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outputDir, projectName+".kicad_pcb")); err == nil {
		t.Fatal("schematic IR project write emitted a PCB file")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat generated PCB file: %v", err)
	}

	report, err := evaluate.Schematic(schematicPath)
	if err != nil {
		t.Fatalf("evaluate generated schematic: %v", err)
	}
	if check := schematicIRCheckByName(report.Checks, "schematic_validation"); check.Status != evaluate.CheckPassed {
		t.Fatalf("schematic_validation check = %#v", check)
	}
	if check := schematicIRCheckByName(report.Checks, "schematic_electrical"); check.Status != evaluate.CheckPassed {
		t.Fatalf("schematic_electrical check = %#v", check)
	}

	generated, err := schematic.ReadFile(schematicPath)
	if err != nil {
		t.Fatalf("read generated schematic: %v", err)
	}
	request, layoutResult := schematiclayout.AdaptSchematic(&generated)
	layoutResult = schematiclayout.Validate(layoutResult, request)
	readability := schematiclayout.BuildReport(layoutResult, schematiclayout.ProfileStandard)
	if !readability.Passed {
		t.Fatalf("readability report failed: %#v diagnostics=%#v", readability, layoutResult.Diagnostics)
	}
	if readability.DiagonalWireCount != 0 || readability.ErrorCount != 0 {
		t.Fatalf("unexpected readability counts: %#v diagnostics=%#v", readability, layoutResult.Diagnostics)
	}
	if readability.WarningCount != 0 || len(readability.OverlapCounts) != 0 {
		t.Fatalf("generated schematic has readability warnings: %#v diagnostics=%#v", readability, layoutResult.Diagnostics)
	}
}

func loadExampleDocument(t *testing.T, name string) Document {
	t.Helper()
	path := filepath.Join("..", "..", "examples", "schematic-ir", name)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open example %s: %v", name, err)
	}
	defer file.Close()
	document, issues := DecodeStrict(file)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("decode example %s issues: %+v", name, issues)
	}
	if issues := Validate(document); reports.HasBlockingIssue(issues) {
		t.Fatalf("validate example %s issues: %+v", name, issues)
	}
	return document
}

func schematicIRCheckByName(checks []evaluate.CheckResult, name string) evaluate.CheckResult {
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	return evaluate.CheckResult{}
}
