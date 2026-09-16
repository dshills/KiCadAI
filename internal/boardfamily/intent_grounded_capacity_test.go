package boardfamily

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestGroundedDenseInventoryRetainsEveryOccurrence(t *testing.T) {
	// A mandatory per-quantity inventory must not inherit the legacy 64-fact
	// representation ceiling: that would make every >64-quantity input fail.
	prompt := "Use BMP280 standard with total bus capacitance " + strings.Repeat("100 pF, ", 128)
	quantities := map[string][]map[string]any{}
	for i := 0; i < 128; i++ {
		quantities[fmt.Sprintf("q%d", i)] = []map[string]any{groundedNumber("total_bus_capacitance_pf", "requested")}
	}
	raw := groundedRaw(t, []map[string]any{ownedFact("sensor", "BMP280", "requested", "c0"), ownedFact("profile", "standard", "requested", "c0")}, quantities)
	d := checkGrounded(t, prompt, raw, "supported")
	if d.Configuration.TotalBusCapacitancePF != 100 {
		t.Fatal("numeric constraint changed")
	}
	compiled, err := CompileGroundedEvidence(prompt, raw)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct{ Facts []json.RawMessage }
	if err := json.Unmarshal(compiled, &wire); err != nil || len(wire.Facts) != 130 {
		t.Fatal("quantity occurrences lost", err)
	}
	// Legacy public contracts must keep rejecting more than 64 facts.
	if _, err := DecodeConnectionEvidenceIntent(prompt, compiled); err == nil {
		t.Fatal("legacy connection contract widened")
	}
	old, err := json.Marshal(map[string]any{"version": OwnedEvidenceVersion, "facts": wire.Facts})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileOwnedEvidenceIntent(prompt, old); err == nil {
		t.Fatal("legacy owned contract widened")
	}
}

func TestGroundedMaximumAssertionInventoryAndRequirementLimit(t *testing.T) {
	prompt := "Use BMP280 with a supply of " + strings.Repeat("3.3 V, ", 128)
	quantities := map[string][]map[string]any{}
	for i := range 128 {
		quantities[fmt.Sprintf("q%d", i)] = []map[string]any{groundedNumber("supply_min_v", "requested"), groundedNumber("supply_max_v", "requested")}
	}
	requirements := make([]map[string]any, 64)
	for i := range requirements {
		requirements[i] = ownedFact("sensor", "BMP280", "requested", "c0")
	}
	raw := groundedRaw(t, requirements, quantities)
	checkGrounded(t, prompt, raw, "supported")
	compiled, err := CompileGroundedEvidence(prompt, raw)
	var wire struct{ Facts []json.RawMessage }
	if err != nil || json.Unmarshal(compiled, &wire) != nil || len(wire.Facts) != groundedMaxAssertions {
		t.Fatal("maximum assertion inventory lost", err, len(wire.Facts))
	}
	// This is a representation boundary fixture, not model output or a claim
	// that the unchanged 1600-token generation cap can emit this inventory.
	requirements = append(requirements, requirements[0])
	if _, err := CompileGroundedEvidence(prompt, groundedRaw(t, requirements, quantities)); err == nil {
		t.Fatal("more than 64 ordinary assertions accepted")
	}
}

func TestGroundedQuantitySchemaMatchesClassificationMultiplicity(t *testing.T) {
	prompt := "Use BMP280 with 3.3 V input."
	schema, err := GroundedEvidenceSchema(prompt)
	if err != nil {
		t.Fatal(err)
	}
	number := groundedNumber("supply_min_v", "requested")
	other := map[string]any{"kind": "other", "detail": "Explicit supply requirement", "state": "requested", "context": []string{}}
	for _, entries := range [][]map[string]any{{number, other}, {other, other}} {
		raw := groundedRaw(t, nil, map[string][]map[string]any{"q0": entries})
		if checkGroundedSchema(t, schema, raw) == nil {
			t.Fatal("schema permits a classification combination rejected by the compiler")
		}
		if _, err := CompileGroundedEvidence(prompt, raw); err == nil {
			t.Fatal("invalid classification multiplicity accepted")
		}
	}
}
