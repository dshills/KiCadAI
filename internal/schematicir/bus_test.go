package schematicir

import (
	"path/filepath"
	"reflect"
	"testing"

	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
	"kicadai/internal/schematiclayout"
	"kicadai/internal/transactions"
)

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
	if labels["DATA[0]"] != 4 || labels["DATA[1]"] != 4 || labels["DATA[2]"] != 4 || labels["DATA[3]"] != 4 {
		t.Fatalf("vector bus member labels = %#v", labels)
	}
	request, layoutResult := schematiclayout.AdaptSchematic(&file)
	layoutResult = schematiclayout.Validate(layoutResult, request)
	readability := schematiclayout.BuildReport(layoutResult, schematiclayout.ProfileStandard)
	if !readability.Passed || readability.WarningCount != 0 {
		t.Fatalf("vector bus readability = %#v diagnostics=%#v", readability, layoutResult.Diagnostics)
	}
}
