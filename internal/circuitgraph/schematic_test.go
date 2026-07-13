package circuitgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/evaluate"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
	"kicadai/internal/transactions"
)

var updateCircuitGraphGolden = flag.Bool("update", false, "update circuit graph golden files")

func TestToSchematicIRCheckedInExamples(t *testing.T) {
	catalog := loadGraphCatalog(t)
	resolver := NewResolver(ResolveOptions{Catalog: catalog, CatalogID: "checked-in"})
	for _, name := range []string{"rc_filter.json", "transistor_switch.json", "usb_c_led_indicator_protected.json", "usb_c_bmp280_breakout.json"} {
		t.Run(name, func(t *testing.T) {
			graph := loadGraphExample(t, name)
			resolved, issues := resolver.Resolve(context.Background(), graph)
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("resolve issues = %#v", issues)
			}
			document, issues := ToSchematicIR(resolved)
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("schematic lowering issues = %#v", issues)
			}
			if validation := schematicir.Validate(document); reports.HasBlockingIssue(validation) {
				t.Fatalf("schematic IR validation = %#v", validation)
			}
			index := schematicTestLibraryIndex(resolved)
			first, issues := schematicir.ToTransactionWithLibraryIndex(document, &index)
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("transaction issues = %#v", issues)
			}
			second, issues := schematicir.ToTransactionWithLibraryIndex(document, &index)
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("second transaction issues = %#v", issues)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatal("schematic transaction is not deterministic")
			}
			assertCircuitGraphGolden(t, name, document)
		})
	}
}

func TestToSchematicIRPreservesNoConnectAndProvenance(t *testing.T) {
	graph := loadGraphExample(t, "usb_c_bmp280_breakout.json")
	for index := range graph.Components {
		if graph.Components[index].ID == "regulator" {
			graph.Components[index].Properties = append(graph.Components[index].Properties,
				Property{Name: "MPN", Value: "spoofed"})
		}
	}
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), graph)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	document, issues := ToSchematicIR(resolved)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("lowering issues = %#v", issues)
	}
	var regulator schematicir.Component
	for _, component := range document.Circuit.Components {
		if component.ID == "regulator" {
			regulator = component
			break
		}
	}
	if regulator.Properties["KiCadAI Component ID"] != "regulator.linear.ap2112k_3v3.sot23_5" || regulator.Properties["KiCadAI Resolution Hash"] == "" || regulator.Properties["MPN"] != "AP2112K-3.3" {
		t.Fatalf("regulator provenance = %#v", regulator.Properties)
	}
	foundNC := false
	for _, pin := range regulator.Pins {
		foundNC = foundNC || (pin.Number == "4" && pin.NoConnect)
	}
	if !foundNC {
		t.Fatalf("regulator pins = %#v", regulator.Pins)
	}
}

func TestToSchematicIRLowersPowerFlagsAsSchematicOnlySymbols(t *testing.T) {
	graph := loadGraphExample(t, "usb_c_bmp280_breakout.json")
	graph.PowerFlags = []PowerFlag{{Net: "VBUS_RAW"}, {Net: "GND"}}
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), graph)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	document, issues := ToSchematicIR(resolved)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("lowering issues = %#v", issues)
	}
	if got, want := len(document.Circuit.Components), len(resolved.Components)+2; got != want {
		t.Fatalf("schematic component count = %d, want %d", got, want)
	}
	wantFlags := map[string]struct {
		ref  string
		role schematicir.ComponentRole
	}{
		powerFlagComponentID("GND"):      {ref: "#FLG01", role: schematicir.ComponentRoleGroundSymbol},
		powerFlagComponentID("VBUS_RAW"): {ref: "#FLG02", role: schematicir.ComponentRolePowerSymbol},
	}
	for _, component := range document.Circuit.Components {
		want, exists := wantFlags[component.ID]
		if !exists {
			continue
		}
		if component.Ref != want.ref || component.Role != want.role || component.Symbol != "power:PWR_FLAG" || component.Footprint != "" || len(component.Pins) != 1 || component.Pins[0].Number != "1" {
			t.Fatalf("power flag %s = %#v", component.ID, component)
		}
		delete(wantFlags, component.ID)
	}
	if len(wantFlags) != 0 {
		t.Fatalf("missing power flags = %#v", wantFlags)
	}
	for _, net := range document.Circuit.Nets {
		if net.Name != "GND" && net.Name != "VBUS_RAW" {
			continue
		}
		wantEndpoint := schematicir.EndpointRef(powerFlagComponentID(net.Name) + ".1")
		if !slices.Contains(net.Connect, wantEndpoint) {
			t.Fatalf("net %s endpoints = %#v, want %s", net.Name, net.Connect, wantEndpoint)
		}
	}
	index := schematicTestLibraryIndex(resolved)
	first, issues := schematicir.ToTransactionWithLibraryIndex(document, &index)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("transaction issues = %#v", issues)
	}
	second, issues := schematicir.ToTransactionWithLibraryIndex(document, &index)
	if reports.HasBlockingIssue(issues) || !reflect.DeepEqual(first, second) {
		t.Fatalf("transaction is not deterministic: issues=%#v", issues)
	}
}

func TestToSchematicIRRejectsPowerFlagIDCollision(t *testing.T) {
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), loadGraphExample(t, "usb_c_bmp280_breakout.json"))
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	resolved.Source.PowerFlags = []PowerFlag{{Net: "GND"}}
	resolved.Components[0].Instance.ID = powerFlagComponentID("GND")
	_, issues = ToSchematicIR(resolved)
	assertGraphIssueCode(t, issues, CodeSchematicLowering)
}

func TestToSchematicIRRejectsDuplicatePowerFlagAfterResolution(t *testing.T) {
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), loadGraphExample(t, "usb_c_bmp280_breakout.json"))
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	resolved.Source.PowerFlags = []PowerFlag{{Net: "GND"}, {Net: "GND"}}
	_, issues = ToSchematicIR(resolved)
	assertGraphIssueCode(t, issues, CodePowerFlagInvalid)
}

func TestToSchematicIRPreservesMultiUnitReference(t *testing.T) {
	resolved, issues := NewResolver(ResolveOptions{Catalog: minimalResolvedCatalog()}).Resolve(context.Background(), minimalResolvedDocument())
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	document, issues := ToSchematicIR(resolved)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("lowering issues = %#v", issues)
	}
	if len(document.Circuit.Components) != 4 {
		t.Fatalf("component count = %d", len(document.Circuit.Components))
	}
	refsByGraphComponent := map[string]string{}
	for _, component := range document.Circuit.Components {
		graphID := component.ID[:2]
		if prior := refsByGraphComponent[graphID]; prior != "" && prior != component.Ref {
			t.Fatalf("multi-unit refs differ: %q != %q", prior, component.Ref)
		}
		refsByGraphComponent[graphID] = component.Ref
	}
}

func TestToSchematicIRRejectsGeneratedUnitIDCollision(t *testing.T) {
	resolved, issues := NewResolver(ResolveOptions{Catalog: minimalResolvedCatalog()}).Resolve(context.Background(), minimalResolvedDocument())
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	resolved.Components[1].Instance.ID = "u1_u1"
	resolved.Source.Components[1].ID = "u1_u1"
	resolved.Components[1].Functions = resolved.Components[1].Functions[:1]
	document, issues := ToSchematicIR(resolved)
	assertGraphIssueCode(t, issues, CodeSchematicLowering)
	if document.Schema != "" || len(document.Circuit.Components) != 0 {
		t.Fatalf("failed lowering returned partial document: %#v", document)
	}
}

func TestToSchematicIRRejectsUnprovenResolution(t *testing.T) {
	_, issues := ToSchematicIR(ResolvedDocument{Source: minimalResolvedDocument()})
	assertGraphIssueCode(t, issues, CodeSchematicLowering)
}

func TestSchematicPinRoleRecognizesGroundNames(t *testing.T) {
	for _, name := range []string{"GND", "VSS", "AGND", "USB_GND"} {
		if got := schematicPinRole(ResolvedFunction{Function: name, Electrical: "passive"}); got != schematicir.PinRoleGround {
			t.Fatalf("pin role for %s = %s", name, got)
		}
	}
	if got := schematicPinRole(ResolvedFunction{Function: "VEE", Electrical: "power_in"}); got != schematicir.PinRolePower {
		t.Fatalf("pin role for VEE = %s", got)
	}
}

func TestCompareSchematicPinNumbersUsesNumericOrder(t *testing.T) {
	if compareSchematicPinNumbers("2", "10") >= 0 || compareSchematicPinNumbers("A9", "B1") >= 0 {
		t.Fatal("pin comparator did not preserve numeric and lexical ordering")
	}
}

func TestToSchematicIRRejectsEmptyResolvedSymbolID(t *testing.T) {
	resolved, issues := NewResolver(ResolveOptions{Catalog: minimalResolvedCatalog()}).Resolve(context.Background(), minimalResolvedDocument())
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	resolved.Components[0].Functions[0].SymbolID = ""
	_, issues = ToSchematicIR(resolved)
	assertGraphIssueCode(t, issues, CodeSchematicLowering)
}

func TestToSchematicIRRejectsConflictingFunctionsOnOneSymbolPin(t *testing.T) {
	resolved, issues := NewResolver(ResolveOptions{Catalog: minimalResolvedCatalog()}).Resolve(context.Background(), minimalResolvedDocument())
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	conflict := resolved.Components[0].Functions[0]
	conflict.Function = "CONFLICT"
	conflict.Pad = "99"
	resolved.Components[0].Functions = append(resolved.Components[0].Functions, conflict)
	_, issues = ToSchematicIR(resolved)
	assertGraphIssueCode(t, issues, CodeSchematicLowering)
}

func TestToSchematicIRLayoutDoesNotDependOnProjectName(t *testing.T) {
	resolver := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"})
	base := loadGraphExample(t, "rc_filter.json")
	renamed := Normalize(base)
	renamed.Project.Name = "renamed_without_fixture_identity"
	left := lowerGraphForTest(t, resolver, base)
	right := lowerGraphForTest(t, resolver, renamed)
	left.Metadata = schematicir.Metadata{}
	right.Metadata = schematicir.Metadata{}
	for index := range left.Circuit.Components {
		delete(left.Circuit.Components[index].Properties, "KiCadAI Resolution Hash")
		delete(right.Circuit.Components[index].Properties, "KiCadAI Resolution Hash")
	}
	if !reflect.DeepEqual(left.Circuit, right.Circuit) || !reflect.DeepEqual(left.Layout, right.Layout) {
		t.Fatal("schematic projection depends on project name")
	}
}

func TestToSchematicIRWritesAndReadsGenericRCProject(t *testing.T) {
	resolver := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"})
	document := lowerGraphForTest(t, resolver, loadGraphExample(t, "rc_filter.json"))
	tx, issues := schematicir.ToProjectTransaction(document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("project transaction issues = %#v", issues)
	}
	outputDir := filepath.Join(t.TempDir(), document.Metadata.Name)
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir, Overwrite: true})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("apply issues = %#v", apply.Issues)
	}
	schematicPath := filepath.Join(outputDir, document.Metadata.Name+".kicad_sch")
	if _, err := schematic.ReadFile(schematicPath); err != nil {
		t.Fatalf("read generated schematic: %v", err)
	}
	report, err := evaluate.Schematic(schematicPath)
	if err != nil {
		t.Fatalf("evaluate generated schematic: %v", err)
	}
	for _, checkName := range []string{"schematic_validation", "schematic_electrical"} {
		found := false
		for _, check := range report.Checks {
			if check.Name == checkName {
				found = true
				if check.Status != evaluate.CheckPassed {
					t.Fatalf("%s = %#v", checkName, check)
				}
			}
		}
		if !found {
			t.Fatalf("missing evaluation check %s", checkName)
		}
	}
}

func lowerGraphForTest(t *testing.T, resolver *Resolver, graph Document) schematicir.Document {
	t.Helper()
	resolved, issues := resolver.Resolve(context.Background(), graph)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	document, issues := ToSchematicIR(resolved)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("lowering issues = %#v", issues)
	}
	return document
}

func assertCircuitGraphGolden(t *testing.T, sourceName string, value any) {
	t.Helper()
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	path := filepath.Join("testdata", "golden", "schematic_"+sourceName)
	if *updateCircuitGraphGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, body) {
		t.Fatalf("golden mismatch\nwant:\n%s\ngot:\n%s", want, body)
	}
}

func schematicTestLibraryIndex(resolved ResolvedDocument) libraryresolver.LibraryIndex {
	index := libraryresolver.LibraryIndex{
		Symbols: map[string]libraryresolver.SymbolRecord{}, Footprints: map[string]libraryresolver.FootprintRecord{},
	}
	type pinKey struct {
		symbol string
		unit   int
		pin    string
	}
	seenPins := map[pinKey]struct{}{}
	seenPads := map[string]map[string]struct{}{}
	for _, component := range resolved.Components {
		for pinIndex, function := range component.Functions {
			key := pinKey{symbol: function.SymbolID, unit: function.Unit, pin: function.SymbolPin}
			if _, exists := seenPins[key]; !exists {
				record := index.Symbols[function.SymbolID]
				record.LibraryID = function.SymbolID
				record.Pins = append(record.Pins, libraryresolver.SymbolPin{
					Number: function.SymbolPin, Name: function.Function, Unit: function.Unit,
					Position: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(float64(pinIndex) * 2.54)}, Orientation: "0",
				})
				index.Symbols[function.SymbolID] = record
				seenPins[key] = struct{}{}
			}
			if seenPads[component.FootprintID] == nil {
				seenPads[component.FootprintID] = map[string]struct{}{}
			}
			if _, exists := seenPads[component.FootprintID][function.Pad]; !exists {
				record := index.Footprints[component.FootprintID]
				record.FootprintID = component.FootprintID
				record.Pads = append(record.Pads, libraryresolver.FootprintPad{Name: function.Pad})
				index.Footprints[component.FootprintID] = record
				seenPads[component.FootprintID][function.Pad] = struct{}{}
			}
		}
	}
	return index
}
