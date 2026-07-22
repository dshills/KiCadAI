package architecturesearch

import (
	"slices"
	"testing"
)

func TestValidatePowerTreeTopologyProvesUniqueAcyclicSources(t *testing.T) {
	requirement := powerTreeRequirement(false)
	selections := powerTreeSelections()
	checks, validation := validatePowerTreeTopology(requirement, selections)
	if validation != nil {
		t.Fatal(validation)
	}
	if !slices.ContainsFunc(checks, func(check GlobalCheck) bool { return check.Path == "candidate.power_tree.acyclic" }) {
		t.Fatalf("power-tree checks lack acyclic proof: %#v", checks)
	}
	slices.Reverse(selections)
	replayed, validation := validatePowerTreeTopology(requirement, selections)
	if validation != nil || !slices.EqualFunc(checks, replayed, func(left, right GlobalCheck) bool {
		return left.Code == right.Code && left.Path == right.Path && left.Message == right.Message
	}) {
		t.Fatalf("power-tree proof changed with selection order:\nfirst=%#v\nreplayed=%#v\nvalidation=%v", checks, replayed, validation)
	}
}

func TestValidatePowerTreeTopologyRejectsMissingAndAmbiguousSources(t *testing.T) {
	requirement := powerTreeRequirement(false)
	_, validation := validatePowerTreeTopology(requirement, nil)
	if validation == nil || validation.Code != CodePowerRailSourceMissing {
		t.Fatalf("missing rail producer validation = %#v", validation)
	}
	selections := powerTreeSelections()
	selections = append(selections, selections[0])
	_, validation = validatePowerTreeTopology(requirement, selections)
	if validation == nil || validation.Code != CodePowerRailSourceAmbiguous {
		t.Fatalf("ambiguous rail producer validation = %#v", validation)
	}
}

func TestValidatePowerTreeTopologyRejectsCyclesDeterministically(t *testing.T) {
	requirement := powerTreeRequirement(true)
	selections := powerTreeSelections()
	_, first := validatePowerTreeTopology(requirement, selections)
	if first == nil || first.Code != CodePowerRailCycle || first.Message != "power rail dependency cycle: rail_a -> rail_b -> rail_a" {
		t.Fatalf("cycle validation = %#v", first)
	}
	slices.Reverse(requirement.Requirements.Domains)
	slices.Reverse(requirement.Requirements.Objectives)
	slices.Reverse(selections)
	_, replayed := validatePowerTreeTopology(requirement, selections)
	if replayed == nil || replayed.Code != first.Code || replayed.Message != first.Message {
		t.Fatalf("cycle diagnostic changed with input order: first=%#v replayed=%#v", first, replayed)
	}
}

func powerTreeRequirement(cyclic bool) Requirement {
	inputA := Binding{Role: "input", Port: "external_power"}
	if cyclic {
		inputA = Binding{Role: "input", Signal: "rail_b_signal", Direction: "sink"}
	}
	return Requirement{Schema: SchemaIDV3, Version: VersionV3, Requirements: Requirements{
		Domains: []Domain{
			{ID: "external", Kind: "supply", NominalVoltageV: 12, Source: "external"},
			{ID: "rail_a", Kind: "supply", NominalVoltageV: 5, Source: "rail_a_signal"},
			{ID: "rail_b", Kind: "supply", NominalVoltageV: 3.3, Source: "rail_b_signal"},
		},
		Ports: []Port{{ID: "external_power", Kind: "power", Direction: "source", Domain: "external"}},
		Signals: []Signal{
			{ID: "rail_a_signal", Kind: "power", Domain: "rail_a"},
			{ID: "rail_b_signal", Kind: "power", Domain: "rail_b"},
		},
		Objectives: []Objective{
			{ID: "make_a", Capability: "voltage_regulation", Bindings: []Binding{inputA, {Role: "output", Signal: "rail_a_signal", Direction: "source"}}},
			{ID: "make_b", Capability: "voltage_regulation", Bindings: []Binding{{Role: "input", Signal: "rail_a_signal", Direction: "sink"}, {Role: "output", Signal: "rail_b_signal", Direction: "source"}}},
		},
	}}
}

func powerTreeSelections() []FragmentSelection {
	return []FragmentSelection{
		{ObligationPath: "objective:make_a", Ports: []RoleContract{{Role: "output", Anchor: signalAnchor("rail_a_signal"), Contract: PortContract{Kind: "power", Direction: "source", Domain: "rail_a"}}}},
		{ObligationPath: "objective:make_b", Ports: []RoleContract{{Role: "output", Anchor: signalAnchor("rail_b_signal"), Contract: PortContract{Kind: "power", Direction: "source", Domain: "rail_b"}}}},
	}
}
