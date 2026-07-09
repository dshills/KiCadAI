package schematicir

import (
	"encoding/json"
	"testing"

	"kicadai/internal/transactions"
)

func TestToTransactionLEDIndicator(t *testing.T) {
	tx, issues := ToTransaction(validLEDDocument())
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	if tx.Name != "LED1" || tx.Project != "LED1" {
		t.Fatalf("unexpected transaction identity: name=%q project=%q", tx.Name, tx.Project)
	}
	if got := countOperations(tx, transactions.OpCreateProject); got != 1 {
		t.Fatalf("expected one create_project operation, got %d", got)
	}
	if got := countOperations(tx, transactions.OpAddSymbol); got != 3 {
		t.Fatalf("expected three add_symbol operations, got %d", got)
	}
	if got := countOperations(tx, transactions.OpConnect); got != 3 {
		t.Fatalf("expected three connect operations, got %d", got)
	}
	symbols := decodeOperations[transactions.AddSymbolOperation](t, tx, transactions.OpAddSymbol)
	for _, symbol := range symbols {
		if symbol.At.XMM == 0 && symbol.At.YMM == 0 {
			t.Fatalf("symbol %s was not assigned a deterministic placement", symbol.Ref)
		}
	}
	result := transactions.Validate(tx)
	if len(result.Issues) != 0 {
		t.Fatalf("transaction validation issues: %+v", result.Issues)
	}

	connects := decodeOperations[transactions.ConnectOperation](t, tx, transactions.OpConnect)
	expected := map[string]bool{
		"VIN:J1.1->R1.1":   false,
		"LED_A:R1.2->D1.1": false,
		"GND:D1.2->J1.2":   false,
	}
	for _, connect := range connects {
		key := connect.NetName + ":" + connect.From.Ref + "." + connect.From.Pin + "->" + connect.To.Ref + "." + connect.To.Pin
		if _, ok := expected[key]; !ok {
			t.Fatalf("unexpected connect operation %s", key)
		}
		expected[key] = true
	}
	for key, seen := range expected {
		if !seen {
			t.Fatalf("missing connect operation %s", key)
		}
	}
}

func TestToTransactionAssignsMissingReferencesWhenAllowed(t *testing.T) {
	doc := validLEDDocument()
	for index := range doc.Circuit.Components {
		doc.Circuit.Components[index].Ref = ""
	}
	doc.Circuit.Components[2].Ref = "R1"

	tx, issues := ToTransaction(doc)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	symbols := decodeOperations[transactions.AddSymbolOperation](t, tx, transactions.OpAddSymbol)
	if got, want := symbols[0].Ref, "J1"; got != want {
		t.Fatalf("first generated ref = %q, want %q", got, want)
	}
	if got, want := symbols[1].Ref, "R2"; got != want {
		t.Fatalf("second generated ref = %q, want %q", got, want)
	}
	if got, want := symbols[2].Ref, "R1"; got != want {
		t.Fatalf("third generated ref = %q, want %q", got, want)
	}
}

func TestToTransactionRejectsMissingReferenceWhenRepairDisabled(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[0].Ref = ""
	doc.Policy.Repair.AllowRefAssignment = false

	_, issues := ToTransaction(doc)
	if len(issues) == 0 {
		t.Fatal("expected issue for missing reference")
	}
}

func TestToTransactionRejectsDuplicateExplicitReferences(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[1].Ref = "D1"

	tx, issues := ToTransaction(doc)
	if len(issues) == 0 {
		t.Fatal("expected issue for duplicate reference")
	}
	if len(tx.Operations) != 0 {
		t.Fatalf("expected no operations for duplicate reference, got %d", len(tx.Operations))
	}
}

func TestToTransactionAssignsFootprintsAndProperties(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[1].Footprint = "Resistor_SMD:R_0603_1608Metric"
	doc.Circuit.Components[1].Properties = map[string]string{
		"Tolerance": "1%",
		"MPN":       "RC0603FR-071KL",
	}

	tx, issues := ToTransaction(doc)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	assigns := decodeOperations[transactions.AssignFootprintOperation](t, tx, transactions.OpAssignFootprint)
	if len(assigns) != 1 {
		t.Fatalf("expected one assign_footprint operation, got %d", len(assigns))
	}
	if assigns[0].Ref != "R1" || assigns[0].FootprintID != "Resistor_SMD:R_0603_1608Metric" {
		t.Fatalf("unexpected footprint assignment: %+v", assigns[0])
	}
	symbols := decodeOperations[transactions.AddSymbolOperation](t, tx, transactions.OpAddSymbol)
	if len(symbols[1].Properties) != 3 {
		t.Fatalf("expected three properties, got %+v", symbols[1].Properties)
	}
	if symbols[1].Properties[0].Name != "Footprint" || symbols[1].Properties[1].Name != "MPN" || symbols[1].Properties[2].Name != "Tolerance" {
		t.Fatalf("properties are not sorted deterministically: %+v", symbols[1].Properties)
	}
	footprint := symbols[1].Properties[0]
	if footprint.Name != "Footprint" || footprint.Value != "Resistor_SMD:R_0603_1608Metric" || !footprint.Hidden {
		t.Fatalf("footprint was not emitted as a hidden symbol property: %+v", footprint)
	}
}

func TestToTransactionPreservesGenericFootprintPropertyWithoutExplicitFootprint(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[1].Footprint = ""
	doc.Circuit.Components[1].Properties = map[string]string{
		"Footprint": "Resistor_SMD:R_0603_1608Metric",
	}

	tx, issues := ToTransaction(doc)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	symbols := decodeOperations[transactions.AddSymbolOperation](t, tx, transactions.OpAddSymbol)
	if len(symbols[1].Properties) != 1 {
		t.Fatalf("expected one property, got %+v", symbols[1].Properties)
	}
	footprint := symbols[1].Properties[0]
	if footprint.Name != "Footprint" || footprint.Value != "Resistor_SMD:R_0603_1608Metric" || footprint.Hidden {
		t.Fatalf("generic footprint property was not preserved: %+v", footprint)
	}
}

func TestToTransactionDeduplicatesPropertyNamesCaseInsensitively(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[1].Properties = map[string]string{
		"MPN": "first",
		"mpn": "duplicate",
	}

	tx, issues := ToTransaction(doc)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	symbols := decodeOperations[transactions.AddSymbolOperation](t, tx, transactions.OpAddSymbol)
	if len(symbols[1].Properties) != 1 {
		t.Fatalf("expected one property after case-insensitive dedupe, got %+v", symbols[1].Properties)
	}
	if symbols[1].Properties[0].Name != "MPN" || symbols[1].Properties[0].Value != "first" {
		t.Fatalf("unexpected deduped property: %+v", symbols[1].Properties[0])
	}
}

func TestToTransactionNoConnectNet(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[0].Pins = append(doc.Circuit.Components[0].Pins, Pin{Number: "3"})
	doc.Circuit.Nets = append(doc.Circuit.Nets, Net{
		Name:    "NC_SPARE",
		Role:    NetRoleNoConnect,
		Connect: []EndpointRef{"vin.3"},
	})

	tx, issues := ToTransaction(doc)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	noConnects := decodeOperations[transactions.AddNoConnectOperation](t, tx, transactions.OpAddNoConnect)
	if len(noConnects) != 1 {
		t.Fatalf("expected one no-connect operation, got %d", len(noConnects))
	}
	if noConnects[0].Endpoint.Ref != "J1" || noConnects[0].Endpoint.Pin != "3" {
		t.Fatalf("unexpected no-connect endpoint: %+v", noConnects[0].Endpoint)
	}
}

func TestToTransactionRejectsInvalidIR(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Nets[0].Connect[0] = "missing.1"

	tx, issues := ToTransaction(doc)
	if len(issues) == 0 {
		t.Fatal("expected validation issue")
	}
	if len(tx.Operations) != 0 {
		t.Fatalf("expected no operations for invalid IR, got %d", len(tx.Operations))
	}
}

func TestToTransactionRejectsInvalidUnit(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[1].Unit = "A"

	tx, issues := ToTransaction(doc)
	if len(issues) == 0 {
		t.Fatal("expected issue for invalid unit")
	}
	if len(tx.Operations) != 0 {
		t.Fatalf("expected no operations for invalid unit, got %d", len(tx.Operations))
	}
}

func countOperations(tx transactions.Transaction, kind transactions.OperationKind) int {
	count := 0
	for _, operation := range tx.Operations {
		if operation.Op == kind {
			count++
		}
	}
	return count
}

func decodeOperations[T any](t *testing.T, tx transactions.Transaction, kind transactions.OperationKind) []T {
	t.Helper()
	var out []T
	for _, operation := range tx.Operations {
		if operation.Op != kind {
			continue
		}
		var payload T
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatalf("decode %s: %v", kind, err)
		}
		out = append(out, payload)
	}
	return out
}
