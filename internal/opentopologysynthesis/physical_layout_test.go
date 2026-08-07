package opentopologysynthesis

import (
	"fmt"
	"testing"

	"kicadai/internal/circuitgraph"
)

func TestPhysicalSchematicIntentUsesTopologyDerivedCoreRanks(t *testing.T) {
	graph := CandidateGraph{
		Nodes: []GraphNode{
			{ID: "vin", SemanticID: "vin", Scope: "external", Role: "input"},
			{ID: "sense", Scope: "internal", Role: "feedback"},
			{ID: "out", SemanticID: "out", Scope: "external", Role: "output"},
		},
		Instances: []GraphInstance{
			{ID: "controller", Kind: "opamp"},
			{ID: "pass", Kind: "nmos"},
			{ID: "sense_resistor", Kind: "resistor"},
		},
	}
	intent := physicalSchematicIntent(graph)
	groupByComponent := map[string]string{}
	nearByComponent := map[string]string{}
	for _, placement := range intent.Placements {
		if _, exists := groupByComponent[placement.Component]; exists {
			t.Fatalf("duplicate placement for %s: %#v", placement.Component, intent.Placements)
		}
		groupByComponent[placement.Component] = placement.Group
		nearByComponent[placement.Component] = placement.Near
		if placement.Mirror != "" {
			t.Fatalf("core mirror was fixed before topology layout: %#v", placement)
		}
	}
	ranks := physicalTopologyRanks(graph)
	for _, component := range []string{"controller", "pass", "sense_resistor"} {
		wantGroup := fmt.Sprintf("topology_rank_%03d", ranks[component])
		if groupByComponent[component] != wantGroup || nearByComponent[component] != "" {
			t.Fatalf("%s topology group = %q, near = %q; want %q and no named near chain", component, groupByComponent[component], nearByComponent[component], wantGroup)
		}
	}
	groupRanks := map[string]int{}
	for _, group := range intent.Groups {
		groupRanks[group.ID] = group.Rank
	}
	for _, component := range []string{"controller", "pass", "sense_resistor"} {
		group := groupByComponent[component]
		if groupRanks[group] != ranks[component] {
			t.Fatalf("%s group rank = %d, want topology rank %d", component, groupRanks[group], ranks[component])
		}
	}
	if groupByComponent["interface_out"] != "external_outputs" || groupRanks["external_outputs"] != 4 {
		t.Fatalf("output connector boundary placement = %#v, ranks=%#v", groupByComponent, groupRanks)
	}
	if intent.Rules.MinComponentSpacingMM > 10.16 {
		t.Fatalf("synthesized component spacing is not compact: %v", intent.Rules.MinComponentSpacingMM)
	}
	if intent.Rules.PreferLabelsForLongNets == nil || *intent.Rules.PreferLabelsForLongNets {
		t.Fatal("synthesized local nets should prefer continuous conductors")
	}
	if intent.Rules.MaxAuxiliaryPerRank != 2 {
		t.Fatalf("auxiliary components per rank = %d, want 2", intent.Rules.MaxAuxiliaryPerRank)
	}
	if !intent.Rules.ReserveTitleBlock {
		t.Fatal("synthesized schematics must reserve the standard title block")
	}
	if !intent.Rules.OrientEndpointLabels {
		t.Fatal("synthesized endpoint labels must face away from component bodies")
	}
	if intent.Hierarchy.Mode != "auto" || intent.Hierarchy.MaxComponentsPerSheet != 3 {
		t.Fatalf("functional hierarchy policy = %#v, want automatic grouping with the largest derived stage kept intact", intent.Hierarchy)
	}
}

func TestPhysicalEngineeringValueUsesReadableSIPrefixes(t *testing.T) {
	tests := []struct {
		value float64
		unit  string
		want  string
	}{
		{909000, "Ohm", "909k"},
		{0.22, "ohm", "220m"},
		{15e-9, "F", "15nF"},
		{220e-6, "F", "220uF"},
		{2.2e6, "Hz", "2.2MHz"},
		{1e-18, "F", "1e-18F"},
	}
	for _, test := range tests {
		if got := physicalEngineeringValue(test.value, test.unit); got != test.want {
			t.Errorf("physicalEngineeringValue(%g, %q) = %q, want %q", test.value, test.unit, got, test.want)
		}
	}
}

func TestPhysicalSchematicValueKindsIncludeReferencesAndOscillators(t *testing.T) {
	for _, kind := range []string{"resistance", "capacitance", "inductance", "voltage", "frequency"} {
		if !physicalSchematicValueKind(kind) {
			t.Fatalf("physical schematic value kind %q was omitted", kind)
		}
	}
	if physicalSchematicValueKind("current_rating") {
		t.Fatal("rating-only quantity was rendered as a component value")
	}
}

func TestPhysicalPassiveOrientationsFollowRailAndSignalTopology(t *testing.T) {
	graph := CandidateGraph{
		Nodes: []GraphNode{
			{ID: "supply", Scope: "external", Role: "supply"},
			{ID: "signal_a", Scope: "internal", Role: "signal"},
			{ID: "signal_b", Scope: "internal", Role: "signal"},
			{ID: "reference", Scope: "external", Role: "reference"},
		},
		Instances: []GraphInstance{
			{
				ID: "forward_path", Kind: "resistor",
				Terminals: []TerminalConnection{{Terminal: "A", Node: "signal_a"}, {Terminal: "B", Node: "signal_b"}},
			},
			{
				ID: "upper_rail_branch", Kind: "resistor",
				Terminals: []TerminalConnection{{Terminal: "A", Node: "supply"}, {Terminal: "B", Node: "signal_a"}},
			},
			{
				ID: "lower_rail_branch", Kind: "capacitor",
				Terminals: []TerminalConnection{{Terminal: "A", Node: "signal_b"}, {Terminal: "B", Node: "reference"}},
			},
			{
				ID: "controller", Kind: "opamp",
				Terminals: []TerminalConnection{{Terminal: "IN_PLUS", Node: "signal_a"}, {Terminal: "OUT", Node: "signal_b"}},
			},
		},
	}

	orientations := physicalPassiveOrientations(graph)
	if orientations["forward_path"] != "rotated_90" {
		t.Fatalf("forward-path orientation = %q, want horizontal", orientations["forward_path"])
	}
	for _, component := range []string{"upper_rail_branch", "lower_rail_branch"} {
		if orientations[component] != "normal" {
			t.Fatalf("%s orientation = %q, want vertical rail branch", component, orientations[component])
		}
	}
	if _, exists := orientations["controller"]; exists {
		t.Fatalf("active device received passive orientation: %#v", orientations)
	}

	intent := physicalSchematicIntent(graph)
	byComponent := map[string]string{}
	for _, placement := range intent.Placements {
		byComponent[placement.Component] = placement.Orientation
	}
	for component, want := range orientations {
		if byComponent[component] != want {
			t.Fatalf("%s intent orientation = %q, want %q", component, byComponent[component], want)
		}
	}
}

func TestPhysicalTopologyRanksFollowBoundaryDistances(t *testing.T) {
	graph := CandidateGraph{
		Nodes: []GraphNode{
			{ID: "input", Scope: "external", Role: "input"},
			{ID: "first", Scope: "internal", Role: "signal"},
			{ID: "second", Scope: "internal", Role: "signal"},
			{ID: "output", Scope: "external", Role: "output"},
		},
		Instances: []GraphInstance{
			{ID: "early", Kind: "resistor", Terminals: []TerminalConnection{{Terminal: "A", Node: "input"}, {Terminal: "B", Node: "first"}}},
			{ID: "middle", Kind: "resistor", Terminals: []TerminalConnection{{Terminal: "A", Node: "first"}, {Terminal: "B", Node: "second"}}},
			{ID: "late", Kind: "resistor", Terminals: []TerminalConnection{{Terminal: "A", Node: "second"}, {Terminal: "B", Node: "output"}}},
		},
	}
	ranks := physicalTopologyRanks(graph)
	if !(ranks["early"] < ranks["middle"] && ranks["middle"] < ranks["late"]) {
		t.Fatalf("topology ranks = %#v, want monotonic boundary flow", ranks)
	}
}

func TestPhysicalTopologyNetRolesRecognizesPassiveFeedbackReturn(t *testing.T) {
	graph := CandidateGraph{
		Nodes: []GraphNode{
			{ID: "drive", Scope: "internal", Role: "signal"},
			{ID: "sense", Scope: "internal", Role: "signal"},
			{ID: "output", Scope: "external", Role: "output"},
			{ID: "setpoint", Scope: "external", Role: "input"},
		},
		Instances: []GraphInstance{
			{
				ID: "controller", Kind: "opamp",
				Terminals: []TerminalConnection{
					{Terminal: "IN_PLUS", Node: "setpoint"},
					{Terminal: "IN_MINUS", Node: "sense"},
					{Terminal: "OUT", Node: "drive"},
				},
			},
			{
				ID: "feedback_a", Kind: "resistor",
				Terminals: []TerminalConnection{
					{Terminal: "A", Node: "drive"},
					{Terminal: "B", Node: "output"},
				},
			},
			{
				ID: "feedback_b", Kind: "resistor",
				Terminals: []TerminalConnection{
					{Terminal: "A", Node: "output"},
					{Terminal: "B", Node: "sense"},
				},
			},
		},
	}

	roles := physicalTopologyNetRoles(graph)
	if roles["sense"] != circuitgraph.NetRoleFeedback {
		t.Fatalf("sense role = %q, want %q", roles["sense"], circuitgraph.NetRoleFeedback)
	}
	if roles["setpoint"] != "" {
		t.Fatalf("setpoint was misclassified as feedback: %q", roles["setpoint"])
	}
}

func TestPhysicalTopologyNetRolesRetainsParallelFeedbackAndPrunesBiasBranch(t *testing.T) {
	graph := CandidateGraph{
		Nodes: []GraphNode{
			{ID: "sense", Scope: "internal", Role: "signal"},
			{ID: "upper", Scope: "internal", Role: "signal"},
			{ID: "lower", Scope: "internal", Role: "signal"},
			{ID: "bias", Scope: "internal", Role: "signal"},
			{ID: "output", Scope: "external", Role: "output"},
		},
		Instances: []GraphInstance{
			{ID: "controller", Kind: "opamp", Terminals: []TerminalConnection{{Terminal: "IN_MINUS", Node: "sense"}, {Terminal: "OUT", Node: "output"}}},
			{ID: "upper_a", Kind: "resistor", Terminals: []TerminalConnection{{Terminal: "A", Node: "sense"}, {Terminal: "B", Node: "upper"}}},
			{ID: "upper_b", Kind: "capacitor", Terminals: []TerminalConnection{{Terminal: "A", Node: "upper"}, {Terminal: "B", Node: "output"}}},
			{ID: "lower_a", Kind: "capacitor", Terminals: []TerminalConnection{{Terminal: "A", Node: "sense"}, {Terminal: "B", Node: "lower"}}},
			{ID: "lower_b", Kind: "resistor", Terminals: []TerminalConnection{{Terminal: "A", Node: "lower"}, {Terminal: "B", Node: "output"}}},
			{ID: "bias_branch", Kind: "resistor", Terminals: []TerminalConnection{{Terminal: "A", Node: "upper"}, {Terminal: "B", Node: "bias"}}},
		},
	}

	roles := physicalTopologyNetRoles(graph)
	for _, node := range []string{"sense", "upper", "lower"} {
		if roles[node] != circuitgraph.NetRoleFeedback {
			t.Fatalf("%s role = %q, want feedback", node, roles[node])
		}
	}
	for _, node := range []string{"output", "bias"} {
		if roles[node] != "" {
			t.Fatalf("%s was incorrectly classified as feedback: %q", node, roles[node])
		}
	}
}
