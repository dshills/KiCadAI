package schematicir

import (
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
	"kicadai/internal/schematiclayout"
	"kicadai/internal/transactions"
)

func TestVectorBusUsesCalibratedConnectionAnchors(t *testing.T) {
	document := loadExampleDocument(t, "vector_bus.json")
	tx, issues := ToProjectTransaction(document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("vector bus transaction issues: %#v", issues)
	}

	wantY := map[string]float64{"1": -2.54, "2": 0, "3": 2.54, "4": 5.08}
	seen := map[string]bool{}
	for _, symbol := range decodeOperations[transactions.AddSymbolOperation](t, tx, transactions.OpAddSymbol) {
		if symbol.Ref != "J1" && symbol.Ref != "J2" {
			continue
		}
		seen[symbol.Ref] = true
		if len(symbol.Pins) != len(wantY) {
			t.Fatalf("%s pins = %#v", symbol.Ref, symbol.Pins)
		}
		for _, pin := range symbol.Pins {
			expectedY, ok := wantY[pin.Number]
			if !ok {
				t.Fatalf("%s has unexpected pin %s: %#v", symbol.Ref, pin.Number, symbol.Pins)
			}
			if !vectorBusMMEqual(pin.XMM, -5.08) || !vectorBusMMEqual(pin.YMM, expectedY) || !pin.ExplicitOffset {
				t.Fatalf("%s pin %s = %#v, want calibrated explicit anchor (-5.08, %.2f)", symbol.Ref, pin.Number, pin, expectedY)
			}
		}
	}
	if !seen["J1"] || !seen["J2"] {
		t.Fatalf("vector bus connector operations = %#v, want J1 and J2", seen)
	}

	repeated, repeatedIssues := ToProjectTransaction(document)
	if reports.HasBlockingIssue(repeatedIssues) {
		t.Fatalf("repeated vector bus transaction issues: %#v", repeatedIssues)
	}
	if !reflect.DeepEqual(tx, repeated) {
		t.Fatal("vector bus transaction changed across repeated generation")
	}
}

func TestVectorBusRejectsRawLibraryPinOffsetAsConnectionAnchor(t *testing.T) {
	document := loadExampleDocument(t, "vector_bus.json")
	staleRawY := 2.54
	issuePath := ""
	for componentIndex := range document.Circuit.Components {
		component := &document.Circuit.Components[componentIndex]
		if component.Ref != "J1" {
			continue
		}
		for pinIndex := range component.Pins {
			if component.Pins[pinIndex].Number == "1" {
				component.Pins[pinIndex].OffsetYMM = &staleRawY
				issuePath = fmt.Sprintf("circuit.components[%d].pins[%d]", componentIndex, pinIndex)
				break
			}
		}
	}
	if issuePath == "" {
		t.Fatal("vector bus fixture does not contain J1 pin 1")
	}

	_, issues := ToProjectTransaction(document)
	for _, issue := range issues {
		if issue.Path == issuePath && issue.Code == reports.CodeInvalidArgument && strings.Contains(issue.Message, "conflicts with the KiCad-validated connection anchor") {
			return
		}
	}
	t.Fatalf("stale raw pin offset did not fail closed: %#v", issues)
}

func TestVectorBusConnectionAnchorFollowsSymbolTransform(t *testing.T) {
	document := loadExampleDocument(t, "vector_bus.json")
	document.Layout.Placements = []Placement{{Target: "input", Group: "input_group", Orientation: OrientationRotated180}}
	state, issues := newAdapterState(document, nil)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("transformed vector bus adapter state issues: %#v", issues)
	}
	origin := state.pointsByID["input"]
	anchor, ok := state.portEndpointAnchor("input.1")
	if !ok {
		t.Fatal("transformed input.1 anchor was not resolved")
	}
	if !vectorBusMMEqual(anchor.XMM, origin.XMM+5.08) || !vectorBusMMEqual(anchor.YMM, origin.YMM+2.54) {
		t.Fatalf("transformed input.1 anchor = %#v from origin %#v, want (+5.08,+2.54)", anchor, origin)
	}
	if got := schematic.TransformConnectionAnchor(kicadfiles.Point{X: kicadfiles.MM(-5.08), Y: kicadfiles.MM(-2.54)}, 180, ""); got != (kicadfiles.Point{X: kicadfiles.MM(5.08), Y: kicadfiles.MM(2.54)}) {
		t.Fatalf("KiCad connection-anchor transform = %#v", got)
	}
}

func vectorBusMMEqual(left, right float64) bool {
	return math.Abs(left-right) <= 1e-9
}

func TestValidateVectorBusLayout(t *testing.T) {
	document := validLEDDocument()
	document.Circuit.Buses = []Bus{{
		ID:   "data_bus",
		Name: "DATA[0..1]",
		Members: []BusMember{
			{Net: "LED_A", Label: "DATA1"},
			{Net: "VIN", Label: "DATA0"},
		},
	}}
	document.Layout.Buses = []BusLayout{{
		Bus:    "data_bus",
		Points: []LayoutPoint{{XMM: 50, YMM: 80}, {XMM: 100, YMM: 80}},
		Entries: []BusEntryLayout{
			{Member: "LED_A", Endpoint: "r_limit.2", At: LayoutPoint{XMM: 75, YMM: 80}, Size: LayoutPoint{XMM: 2.54, YMM: 2.54}},
			{Member: "LED_A", Endpoint: "led.1", At: LayoutPoint{XMM: 80, YMM: 80}, Size: LayoutPoint{XMM: 2.54, YMM: 2.54}},
			{Member: "VIN", Endpoint: "vin.1", At: LayoutPoint{XMM: 60, YMM: 80}, Size: LayoutPoint{XMM: 2.54, YMM: 2.54}},
			{Member: "VIN", Endpoint: "r_limit.1", At: LayoutPoint{XMM: 65, YMM: 80}, Size: LayoutPoint{XMM: 2.54, YMM: 2.54}},
		},
	}}

	if issues := Validate(document); len(issues) != 0 {
		t.Fatalf("valid vector bus produced issues: %#v", issues)
	}
	normalized := Normalize(document)
	if got := normalized.Circuit.Buses[0].Members[0].Net; got != "LED_A" {
		t.Fatalf("normalized bus members start with %q, want LED_A", got)
	}
	if got := normalized.Layout.Buses[0].Entries[0].Member; got != "LED_A" {
		t.Fatalf("normalized bus entries start with %q, want LED_A", got)
	}
}

func TestValidateVectorBusRejectsAmbiguousGeometry(t *testing.T) {
	document := validLEDDocument()
	document.Circuit.Buses = []Bus{{
		ID:      "data_bus",
		Name:    "DATA[0..1]",
		Members: []BusMember{{Net: "MISSING", Label: "DATA0"}},
	}}
	document.Layout.Buses = []BusLayout{{
		Bus:    "data_bus",
		Points: []LayoutPoint{{XMM: 50, YMM: 80}, {XMM: 60, YMM: 90}},
		Entries: []BusEntryLayout{{
			Member:   "MISSING",
			Endpoint: "vin.1",
			At:       LayoutPoint{XMM: 55, YMM: 85},
			Size:     LayoutPoint{XMM: 0, YMM: 2.54},
		}},
	}}

	issues := Validate(document)
	for _, want := range []string{
		"circuit.buses[0].members[0].net",
		"layout.buses[0].points[1]",
		"layout.buses[0].entries[0].size",
		"layout.buses[0].entries[0].at",
	} {
		if !schematicIRIssueContains(issues, want) {
			t.Fatalf("missing issue at %s: %#v", want, issues)
		}
	}
}

func TestNormalizeVectorBusIsDeterministic(t *testing.T) {
	document := validLEDDocument()
	document.Circuit.Buses = []Bus{
		{ID: "z_bus", Name: "Z", Members: []BusMember{{Net: "VIN", Label: "Z"}}},
		{ID: "a_bus", Name: "A", Members: []BusMember{{Net: "LED_OUT", Label: "A"}}},
	}
	document.Layout.Buses = []BusLayout{
		{Bus: "z_bus", Points: []LayoutPoint{{XMM: 1, YMM: 2}, {XMM: 1, YMM: 4}}, Entries: []BusEntryLayout{{Member: "VIN", Endpoint: "vin.1", At: LayoutPoint{XMM: 1, YMM: 2}, Size: LayoutPoint{XMM: 1, YMM: 1}}}},
		{Bus: "a_bus", Points: []LayoutPoint{{XMM: 2, YMM: 2}, {XMM: 2, YMM: 4}}, Entries: []BusEntryLayout{{Member: "LED_OUT", Endpoint: "r_limit.2", At: LayoutPoint{XMM: 2, YMM: 2}, Size: LayoutPoint{XMM: 1, YMM: 1}}}},
	}
	left := Normalize(document)
	right := Normalize(document)
	if !reflect.DeepEqual(left.Circuit.Buses, right.Circuit.Buses) || !reflect.DeepEqual(left.Layout.Buses, right.Layout.Buses) {
		t.Fatalf("normalization is not deterministic: left=%#v right=%#v", left, right)
	}
}

func TestToTransactionEmitsVectorBusMemberWires(t *testing.T) {
	document := validLEDDocument()
	document.Policy.Acceptance = AcceptanceReadable
	document.Circuit.Buses = []Bus{{
		ID:   "data_bus",
		Name: "DATA[0..1]",
		Members: []BusMember{
			{Net: "LED_A", Label: "DATA[0]"},
			{Net: "VIN", Label: "DATA[1]"},
		},
	}}
	document.Layout.Buses = []BusLayout{{
		Bus:    "data_bus",
		Points: []LayoutPoint{{XMM: 110, YMM: 80}, {XMM: 180, YMM: 80}},
		Entries: []BusEntryLayout{
			{Member: "LED_A", Endpoint: "r_limit.2", At: LayoutPoint{XMM: 153.67, YMM: 80}, Size: LayoutPoint{XMM: 2.54, YMM: 2.54}},
			{Member: "LED_A", Endpoint: "led.1", At: LayoutPoint{XMM: 170.18, YMM: 80}, Size: LayoutPoint{XMM: 2.54, YMM: 2.54}},
			{Member: "VIN", Endpoint: "vin.1", At: LayoutPoint{XMM: 114.3, YMM: 80}, Size: LayoutPoint{XMM: 2.54, YMM: 2.54}},
			{Member: "VIN", Endpoint: "r_limit.1", At: LayoutPoint{XMM: 135.89, YMM: 80}, Size: LayoutPoint{XMM: 2.54, YMM: 2.54}},
		},
	}}

	tx, issues := ToTransaction(document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("vector bus adapter issues: %#v", issues)
	}
	if countOperations(tx, transactions.OpAddBus) != 1 || countOperations(tx, transactions.OpAddBusEntry) != 4 || countOperations(tx, transactions.OpAddSchematicWire) != 8 {
		t.Fatalf("vector bus operations: buses=%d entries=%d wires=%d", countOperations(tx, transactions.OpAddBus), countOperations(tx, transactions.OpAddBusEntry), countOperations(tx, transactions.OpAddSchematicWire))
	}
	if countOperations(tx, transactions.OpConnect) != 1 {
		t.Fatalf("non-bus scalar connection count = %d, want one GND connection", countOperations(tx, transactions.OpConnect))
	}
	if validation := transactions.Validate(tx); reports.HasBlockingIssue(validation.Issues) {
		t.Fatalf("vector bus transaction validation issues: %#v", validation.Issues)
	}
}

func TestSchematicIRWritesAndReadsVectorBus(t *testing.T) {
	document := loadExampleDocument(t, "vector_bus.json")
	tx, issues := ToProjectTransaction(document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("vector bus project transaction issues: %#v", issues)
	}
	output := filepath.Join(t.TempDir(), "vector_bus")
	result := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output, Overwrite: true})
	if reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("vector bus apply issues: %#v", result.Issues)
	}
	path := filepath.Join(output, "vector_bus.kicad_sch")
	file, err := schematic.ReadFile(path)
	if err != nil {
		t.Fatalf("read vector bus schematic: %v", err)
	}
	if len(file.Buses) != 1 || len(file.BusEntries) != 8 || len(file.Wires) < 8 {
		t.Fatalf("vector bus readback counts: buses=%d entries=%d wires=%d labels=%d", len(file.Buses), len(file.BusEntries), len(file.Wires), len(file.Labels))
	}
	labels := map[string]int{}
	for _, label := range file.Labels {
		labels[label.Text]++
	}
	if labels["DATA0"] != 4 || labels["DATA1"] != 4 || labels["DATA2"] != 4 || labels["DATA3"] != 4 || labels["DATA[0..3]"] != 1 {
		t.Fatalf("vector bus member labels = %#v", labels)
	}
	request, layoutResult := schematiclayout.AdaptSchematic(&file)
	layoutResult = schematiclayout.Validate(layoutResult, request)
	readability := schematiclayout.BuildReport(layoutResult, schematiclayout.ProfileStandard)
	if !readability.Passed || readability.WarningCount != 0 {
		t.Fatalf("vector bus readability = %#v diagnostics=%#v", readability, layoutResult.Diagnostics)
	}
}

func TestSchematicBusLabelUsesKiCadBusSyntax(t *testing.T) {
	tests := []struct {
		name string
		bus  Bus
		want string
	}{
		{
			name: "explicit vector",
			bus:  Bus{Name: "DATA[0..3]", Members: []BusMember{{Net: "DATA0", Label: "DATA0"}}},
			want: "DATA[0..3]",
		},
		{
			name: "ordinary scalar members",
			bus:  Bus{Name: "I2C", Members: []BusMember{{Net: "SCL", Label: "SCL"}, {Net: "SDA", Label: "SDA"}}},
			want: "{SCL SDA}",
		},
		{
			name: "named group members",
			bus:  Bus{Name: "USB", Members: []BusMember{{Net: "USB.DP", Label: "USB.DP"}, {Net: "USB.DM", Label: "USB.DM"}}},
			want: "USB{DP DM}",
		},
		{
			name: "explicit group",
			bus:  Bus{Name: "{SCL SDA}", Members: []BusMember{{Net: "SCL", Label: "SCL"}, {Net: "SDA", Label: "SDA"}}},
			want: "{SCL SDA}",
		},
		{
			name: "legacy name without members",
			bus:  Bus{Name: "LEGACY"},
			want: "LEGACY",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := schematicBusLabel(test.bus); got != test.want {
				t.Fatalf("schematicBusLabel(%#v) = %q, want %q", test.bus, got, test.want)
			}
		})
	}
}
