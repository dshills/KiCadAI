package schematicir

import (
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/transactions"
)

func TestSchematicIRExamples(t *testing.T) {
	cases := []struct {
		name          string
		file          string
		minSymbols    int
		minConnects   int
		minFootprints int
		minBuses      int
		minBusEntries int
	}{
		{name: "LED indicator", file: "led_indicator.json", minSymbols: 3, minConnects: 3, minFootprints: 3},
		{name: "USB-C LED indicator", file: "usb_c_led_indicator.json", minSymbols: 5, minConnects: 7, minFootprints: 5},
		{name: "I2C sensor 3.3V regulator", file: "i2c_sensor_3v3_regulator.json", minSymbols: 11, minConnects: 20, minFootprints: 11},
		{name: "Resolver-backed external connector", file: "external_connector_indicator.json", minSymbols: 2, minConnects: 1, minFootprints: 0},
		{name: "Vector bus", file: "vector_bus.json", minSymbols: 2, minConnects: 0, minFootprints: 0, minBuses: 1, minBusEntries: 8},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testSchematicIRExample(t, tc.file, tc.minSymbols, tc.minConnects, tc.minFootprints, tc.minBuses, tc.minBusEntries)
		})
	}
}

func TestEducationalSchematicExamples(t *testing.T) {
	directories := []string{
		"01_dc_voltage_source",
		"02_bjt_current_source",
		"03_differential_amplifier",
		"04_rc_low_pass_filter",
		"05_voltage_divider",
	}
	for _, directory := range directories {
		t.Run(directory, func(t *testing.T) {
			root := filepath.Join("..", "..", "examples", "educational", directory)
			file, err := os.Open(filepath.Join(root, "source.json"))
			if err != nil {
				t.Fatalf("open educational example: %v", err)
			}
			document, issues := DecodeStrict(file)
			if closeErr := file.Close(); closeErr != nil {
				t.Fatalf("close educational example: %v", closeErr)
			}
			if len(issues) != 0 {
				t.Fatalf("decode issues: %+v", issues)
			}
			if issues := Validate(document); len(issues) != 0 {
				t.Fatalf("validation issues: %+v", issues)
			}
			normalized := NormalizeLayoutIntent(document)
			if len(normalized.Layout.Placements) != len(normalized.Circuit.Components) {
				t.Fatalf("placements = %d, components = %d", len(normalized.Layout.Placements), len(normalized.Circuit.Components))
			}
			if len(normalized.Circuit.Nets) == 0 {
				t.Fatal("educational example must contain connected nets")
			}
			schematics, err := filepath.Glob(filepath.Join(root, "generated", "*.kicad_sch"))
			if err != nil || len(schematics) != 1 {
				t.Fatalf("generated schematics = %v, err = %v", schematics, err)
			}
			projects, err := filepath.Glob(filepath.Join(root, "generated", "*.kicad_pro"))
			if err != nil || len(projects) != 1 {
				t.Fatalf("generated projects = %v, err = %v", projects, err)
			}
		})
	}
}

func testSchematicIRExample(t *testing.T, fileName string, minSymbols int, minConnects int, minFootprints int, minBuses int, minBusEntries int) {
	t.Helper()
	path := filepath.Join("..", "..", "examples", "schematic-ir", fileName)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open example: %v", err)
	}
	document, issues := DecodeStrict(file)
	if closeErr := file.Close(); closeErr != nil {
		t.Fatalf("close example: %v", closeErr)
	}
	if len(issues) != 0 {
		t.Fatalf("decode issues: %+v", issues)
	}
	if issues := Validate(document); len(issues) != 0 {
		t.Fatalf("validation issues: %+v", issues)
	}
	normalized := NormalizeLayoutIntent(document)
	if len(normalized.Layout.Groups) == 0 {
		t.Fatal("expected normalized layout groups")
	}
	if len(normalized.Layout.Placements) != len(normalized.Circuit.Components) {
		t.Fatalf("expected placement for every component, got %d for %d components", len(normalized.Layout.Placements), len(normalized.Circuit.Components))
	}

	tx, adapterIssues := ToTransaction(document)
	if len(adapterIssues) != 0 {
		t.Fatalf("adapter issues: %+v", adapterIssues)
	}
	if result := transactions.Validate(tx); len(result.Issues) != 0 {
		t.Fatalf("transaction validation issues: %+v", result.Issues)
	}
	if got := exampleOperationCount(tx, transactions.OpAddSymbol); got < minSymbols {
		t.Fatalf("add_symbol count = %d, want >= %d", got, minSymbols)
	}
	if got := exampleOperationCount(tx, transactions.OpConnect); got < minConnects {
		t.Fatalf("connect count = %d, want >= %d", got, minConnects)
	}
	if got := exampleOperationCount(tx, transactions.OpAssignFootprint); got < minFootprints {
		t.Fatalf("assign_footprint count = %d, want >= %d", got, minFootprints)
	}
	if got := exampleOperationCount(tx, transactions.OpAddBus); got < minBuses {
		t.Fatalf("add_bus count = %d, want >= %d", got, minBuses)
	}
	if got := exampleOperationCount(tx, transactions.OpAddBusEntry); got < minBusEntries {
		t.Fatalf("add_bus_entry count = %d, want >= %d", got, minBusEntries)
	}
}

func exampleOperationCount(tx transactions.Transaction, kind transactions.OperationKind) int {
	count := 0
	for _, operation := range tx.Operations {
		if operation.Op == kind {
			count++
		}
	}
	return count
}
