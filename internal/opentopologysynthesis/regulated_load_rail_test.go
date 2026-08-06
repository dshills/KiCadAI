package opentopologysynthesis

import (
	"math"
	"testing"

	"kicadai/internal/simmodel"
)

func TestLoadRailEnvelopeTreatsReferencesAsOneSourceBoundary(t *testing.T) {
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	converter := PrimitiveCandidate{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind == "isolated_converter" &&
			primitiveHasModel(primitive, simmodel.PrimitiveProtectedIsolatedConverterV1) &&
			primitiveModelParameter(
				primitive,
				simmodel.PrimitiveProtectedIsolatedConverterV1,
				"output_voltage_v",
			) == 12 {
			converter = primitive
			break
		}
	}
	if converter.Key == "" {
		t.Fatal("test inventory lacks a reviewed 12 V isolated converter")
	}
	branchResistance, referenceTieResistance := 10.0, 1.0
	graph := CandidateGraph{
		Nodes: []GraphNode{
			{ID: "reference_a", Scope: "external", Role: "reference"},
			{ID: "reference_b", Scope: "external", Role: "reference"},
			{ID: "branch_a", Scope: "internal", Role: "signal"},
			{ID: "branch_b", Scope: "internal", Role: "signal"},
			{ID: "rail", Scope: "internal", Role: "supply"},
		},
		Instances: []GraphInstance{
			{
				ID: "reference_tie", Kind: "resistor", ValueSI: &referenceTieResistance,
				Terminals: topologyTwoTerminalPlacement("reference_a", "reference_b"),
			},
			{
				ID: "converter_a", Kind: "isolated_converter", PrimitiveKey: converter.Key,
				Terminals: []TerminalConnection{
					{Terminal: "VOUT_MINUS", Node: "reference_a"},
					{Terminal: "VOUT_PLUS", Node: "branch_a"},
				},
			},
			{
				ID: "ballast_a", Kind: "resistor", ValueSI: &branchResistance,
				Terminals: topologyTwoTerminalPlacement("branch_a", "rail"),
			},
			{
				ID: "converter_b", Kind: "isolated_converter", PrimitiveKey: converter.Key,
				Terminals: []TerminalConnection{
					{Terminal: "VOUT_MINUS", Node: "reference_b"},
					{Terminal: "VOUT_PLUS", Node: "branch_b"},
				},
			},
			{
				ID: "ballast_b", Kind: "resistor", ValueSI: &branchResistance,
				Terminals: topologyTwoTerminalPlacement("branch_b", "rail"),
			},
		},
	}
	minimum, maximum, resistance, bounded := topologyLoadRailEnvelope(
		Requirement{}, graph, primitiveInventoryByKey(inventory), "rail",
	)
	if !bounded || minimum != 12 || maximum != 12 || math.Abs(resistance-5) > 1e-12 {
		t.Fatalf(
			"reference-connected parallel rail counted a physical branch more than once: min=%g max=%g resistance=%g bounded=%t",
			minimum, maximum, resistance, bounded,
		)
	}
}
