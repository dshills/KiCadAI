package opentopologysynthesis

import (
	"testing"

	"kicadai/internal/circuitgraph"
)

func TestPhysicalSchematicIntentLeavesCoreRanksTopologyDerived(t *testing.T) {
	graph := CandidateGraph{
		Nodes: []GraphNode{
			{ID: "vin", Scope: "external", Role: "input"},
			{ID: "sense", Scope: "internal", Role: "feedback"},
			{ID: "out", Scope: "external", Role: "output"},
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
		groupByComponent[placement.Component] = placement.Group
		nearByComponent[placement.Component] = placement.Near
		if placement.Mirror != "" {
			t.Fatalf("core mirror was fixed before topology layout: %#v", placement)
		}
	}
	for _, component := range []string{"controller", "pass", "sense_resistor"} {
		if groupByComponent[component] == "synthesized_core" || nearByComponent[component] != "" {
			t.Fatalf("%s retained an arbitrary one-rank group/near chain: groups=%#v near=%#v", component, groupByComponent, nearByComponent)
		}
	}
	for _, group := range intent.Groups {
		if group.ID == "synthesized_core" {
			t.Fatalf("synthesized core still forces one fixed graph rank: %#v", group)
		}
		if group.ID == "external_outputs" && group.Rank != 4 {
			t.Fatalf("output boundary rank = %d, want canonical boundary rank 4", group.Rank)
		}
	}
	if intent.Rules.MinComponentSpacingMM > 12.7 {
		t.Fatalf("synthesized component spacing is not compact: %v", intent.Rules.MinComponentSpacingMM)
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
