package designworkflow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
)

func TestExplicitPlacementSeedPolicyAndLegacyFallback(t *testing.T) {
	circuit := ExplicitCircuitSpec{ResolutionHash: strings.Repeat("a", 64), GenerationHash: strings.Repeat("b", 64)}
	if explicitPlacementSeed(circuit) != circuit.GenerationHash {
		t.Fatal("legacy generation seed changed")
	}
	circuit.GenerationHash = ""
	if explicitPlacementSeed(circuit) != circuit.ResolutionHash {
		t.Fatal("legacy resolution fallback changed")
	}
	circuit.PlacementSeed = &ExplicitPlacementSeedSpec{Policy: ExplicitPlacementSeedAnnotationIndependentV1, Hash: strings.Repeat("c", 64)}
	circuit.Schematic.Layout = schematicir.Layout{NativeProfile: "annotation-v2"}
	if issues := validateExplicitPlacementSeed(circuit); len(issues) > 0 {
		t.Fatal(issues)
	}
	if explicitPlacementSeed(circuit) != circuit.PlacementSeed.Hash {
		t.Fatal("explicit seed ignored")
	}
	b, err := json.Marshal(circuit)
	if err != nil {
		t.Fatal(err)
	}
	var replay ExplicitCircuitSpec
	if err := json.Unmarshal(b, &replay); err != nil {
		t.Fatal(err)
	}
	if explicitPlacementSeed(replay) != explicitPlacementSeed(circuit) {
		t.Fatal("JSON replay lost placement identity")
	}
	clone := cloneExplicitCircuit(&circuit)
	clone.PlacementSeed.Hash = "changed"
	if circuit.PlacementSeed.Hash == "changed" {
		t.Fatal("clone aliases placement seed")
	}
	for _, change := range []func(*ExplicitCircuitSpec){
		func(c *ExplicitCircuitSpec) { c.PlacementSeed.Policy = "future-v2" },
		func(c *ExplicitCircuitSpec) { c.PlacementSeed.Hash = "short" },
		func(c *ExplicitCircuitSpec) { c.Schematic.Layout.NativeProfile = "" },
	} {
		bad := cloneExplicitCircuit(&circuit)
		change(bad)
		if !reports.HasBlockingIssue(validateExplicitPlacementSeed(*bad)) {
			t.Fatal("invalid seed accepted")
		}
		result := PlaceExplicitCircuit(context.Background(), Request{ExplicitCircuit: bad}, PlacementOptions{})
		if !reports.HasBlockingIssue(result.Stage.Issues) {
			t.Fatal("direct placement bypasses seed policy")
		}
	}
}
