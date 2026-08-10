package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"testing"
)

func TestDeclarativeBinaryDecisionGroupsAreSemanticAndOrderIndependent(t *testing.T) {
	requirement := declarativeProviderRequirement()
	first := declarativeBinaryDecisionGroups(Normalize(requirement))
	if len(first) != 1 || len(first[0]) != 2 ||
		first[0][0].excitation != "command_a" || first[0][1].excitation != "command_b" ||
		first[0][0].observation != "state" || first[0][1].observation != "state" {
		t.Fatalf("decision groups = %#v", first)
	}
	reverseRequirement(&requirement)
	second := declarativeBinaryDecisionGroups(Normalize(requirement))
	if !slices.EqualFunc(first[0], second[0], func(left, right declarativeBinaryDecision) bool {
		return left == right
	}) {
		t.Fatalf("decision grouping depends on contract order: %#v != %#v", first, second)
	}

	requirement = declarativeProviderRequirement()
	requirement.Requirements.BehavioralRequirements = requirement.Requirements.BehavioralRequirements[:1]
	if groups := declarativeBinaryDecisionGroups(Normalize(requirement)); len(groups) != 0 {
		t.Fatalf("one-sided decision contract produced a complete group: %#v", groups)
	}
}

func TestConvergentBinaryDecisionProviderBuildsBoundedReplayableGraph(t *testing.T) {
	requirement := Normalize(declarativeProviderRequirement())
	inventory := declarativeProviderInventory()
	first, firstConsumption, firstRejections, initialHash := runDeclarativeProvider(t, requirement, inventory, GraphLimits{
		MaxPrimitiveInstances: 12,
		MaxInternalNodes:      4,
	})
	if len(first) != 1 || len(firstRejections) != 0 ||
		firstConsumption.ExpandedStates != 1 || firstConsumption.CompleteGraphs != 1 ||
		firstConsumption.GeneratedGraphs == 0 || firstConsumption.BudgetExhausted {
		t.Fatalf("provider result candidates=%d consumption=%#v rejections=%#v", len(first), firstConsumption, firstRejections)
	}
	candidate := first[0]
	counts := map[string]int{}
	for _, instance := range candidate.Graph.Instances {
		counts[instance.Kind]++
	}
	if counts["npn_bjt"] != 1 || counts["p_channel_mosfet"] != 1 ||
		counts["signal_diode"] != 1 || counts["resistor"] != 4 {
		t.Fatalf("provider instance counts = %#v", counts)
	}
	if len(candidate.Operations) != len(candidate.Graph.Instances)+2 {
		t.Fatalf("operation count = %d, want %d", len(candidate.Operations), len(candidate.Graph.Instances)+2)
	}
	previous := initialHash
	for index, operation := range candidate.Operations {
		if operation.Number != index+1 || operation.BeforeHash != previous || operation.AfterHash == "" {
			t.Fatalf("operation %d breaks replay chain: %#v", index, operation)
		}
		previous = operation.AfterHash
	}
	if previous != candidate.Fingerprint {
		t.Fatalf("operation replay terminus = %q, want %q", previous, candidate.Fingerprint)
	}
	if issues := ValidateCompleteGraph(candidate.Graph, inventory, GraphLimits{MaxPrimitiveInstances: 12, MaxInternalNodes: 4}); len(issues) != 0 {
		t.Fatalf("provider graph is incomplete: %#v", issues)
	}
	valuePlan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, DefaultPolicy())
	if valuePlan.Status != ValuePlanReady {
		t.Fatalf("provider value plan = %s issues=%#v", valuePlan.Status, valuePlan.Issues)
	}
	instanceByID := map[string]GraphInstance{}
	for _, instance := range candidate.Graph.Instances {
		instanceByID[instance.ID] = instance
	}
	for _, domain := range valuePlan.Domains {
		instance := instanceByID[domain.InstanceID]
		if instance.Kind != "resistor" {
			continue
		}
		if len(domain.Candidates) == 0 || domain.Candidates[0].ValueSI == nil || instance.ValueSI == nil ||
			*domain.Candidates[0].ValueSI != *instance.ValueSI {
			t.Fatalf("construction value was not first for %s: instance=%#v domain=%#v", domain.InstanceID, instance, domain)
		}
	}

	reorderedRequirement := declarativeProviderRequirement()
	reverseRequirement(&reorderedRequirement)
	reorderedInventory := inventory
	reorderedInventory.Primitives = append([]PrimitiveCandidate(nil), inventory.Primitives...)
	slices.Reverse(reorderedInventory.Primitives)
	second, secondConsumption, secondRejections, _ := runDeclarativeProvider(
		t, Normalize(reorderedRequirement), reorderedInventory,
		GraphLimits{MaxPrimitiveInstances: 12, MaxInternalNodes: 4},
	)
	firstJSON, err := json.Marshal(struct {
		Candidates  []TopologyCandidate
		Consumption Consumption
		Rejections  map[string][]string
	}{first, firstConsumption, firstRejections})
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(struct {
		Candidates  []TopologyCandidate
		Consumption Consumption
		Rejections  map[string][]string
	}{second, secondConsumption, secondRejections})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("provider output depends on requirement or inventory order:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestConvergentBinaryDecisionProviderFailsClosedAtGenericBoundaries(t *testing.T) {
	requirement := Normalize(declarativeProviderRequirement())
	inventory := declarativeProviderInventory()
	candidates, _, rejections, _ := runDeclarativeProvider(
		t, requirement, inventory, GraphLimits{MaxPrimitiveInstances: 6, MaxInternalNodes: 4},
	)
	if len(candidates) != 0 || len(rejections["graph_limit"]) == 0 {
		t.Fatalf("bounded provider candidates=%d rejections=%#v", len(candidates), rejections)
	}

	ambiguous := declarativeProviderRequirement()
	ambiguous.Requirements.Ports = append(ambiguous.Requirements.Ports, Port{
		ID: "output_supply_b", Kind: "power", Direction: "sink", Domain: "output_supply",
		Electrical: Electrical{MinVoltageV: providerFloat(3), NominalVoltageV: providerFloat(3.3), MaxVoltageV: providerFloat(3.6)},
	})
	candidates, _, rejections, _ = runDeclarativeProvider(
		t, Normalize(ambiguous), inventory, GraphLimits{MaxPrimitiveInstances: 12, MaxInternalNodes: 4},
	)
	if len(candidates) != 0 || len(rejections["relationship_gap"]) == 0 {
		t.Fatalf("ambiguous supply candidates=%d rejections=%#v", len(candidates), rejections)
	}

	for _, missingKind := range []string{"p_channel_mosfet", "signal_diode"} {
		missing := inventory
		missing.Primitives = slices.DeleteFunc(append([]PrimitiveCandidate(nil), inventory.Primitives...), func(candidate PrimitiveCandidate) bool {
			return candidate.Kind == missingKind
		})
		candidates, _, rejections, _ = runDeclarativeProvider(
			t, requirement, missing, GraphLimits{MaxPrimitiveInstances: 12, MaxInternalNodes: 4},
		)
		if len(candidates) != 0 || len(rejections["relationship_gap"]) == 0 {
			t.Fatalf("missing %s evidence candidates=%d rejections=%#v", missingKind, len(candidates), rejections)
		}
	}
}

func runDeclarativeProvider(
	t *testing.T,
	requirement Requirement,
	inventory PrimitiveInventory,
	limits GraphLimits,
) ([]TopologyCandidate, Consumption, map[string][]string, string) {
	t.Helper()
	initial, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	initialHash, err := GraphHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	initialTopology, err := TopologyHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	byKey := primitiveInventoryByKey(inventory)
	state := topologySearchState{
		graph: initial, hash: initialHash, topology: initialTopology,
		score: scoreTopologyGraph(requirement, initial, byKey, initialHash),
	}
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 8
	policy.MaxGeneratedGraphs = 64
	result, consumption, rejections := topologyConvergentBinaryDecisionSeeds(
		context.Background(), requirement, inventory, topologyRepresentatives(requirement, inventory),
		byKey, limits, policy, state,
	)
	return result, consumption, rejections, initialHash
}

func declarativeProviderRequirement() Requirement {
	return Requirement{
		Schema: RequirementSchema, Version: RequirementVersion,
		Project: Project{Name: "convergent_logic", Title: "Convergent logic", Description: "Two bounded commands drive one state."},
		Requirements: Requirements{
			Domains: []Domain{
				{ID: "reference", Kind: "reference", Source: "external", MinVoltageV: providerFloat(0), NominalVoltageV: providerFloat(0), MaxVoltageV: providerFloat(0)},
				{ID: "input_supply", Kind: "supply", Source: "external", MinVoltageV: providerFloat(1.7), NominalVoltageV: providerFloat(1.8), MaxVoltageV: providerFloat(1.9)},
				{ID: "output_supply", Kind: "supply", Source: "external", MinVoltageV: providerFloat(3), NominalVoltageV: providerFloat(3.3), MaxVoltageV: providerFloat(3.6)},
			},
			Ports: []Port{
				{ID: "command_a", Kind: "digital", Direction: "sink", Domain: "input_supply", Electrical: Electrical{MinVoltageV: providerFloat(0), NominalVoltageV: providerFloat(.9), MaxVoltageV: providerFloat(1.9), DefaultState: "low"}},
				{ID: "command_b", Kind: "digital", Direction: "sink", Domain: "input_supply", Electrical: Electrical{MinVoltageV: providerFloat(0), NominalVoltageV: providerFloat(.9), MaxVoltageV: providerFloat(1.9), DefaultState: "low"}},
				{ID: "state", Kind: "digital", Direction: "source", Domain: "output_supply", Electrical: Electrical{MinVoltageV: providerFloat(0), NominalVoltageV: providerFloat(1.65), MaxVoltageV: providerFloat(3.6)}},
				{ID: "input_supply_in", Kind: "power", Direction: "sink", Domain: "input_supply", Electrical: Electrical{MinVoltageV: providerFloat(1.7), NominalVoltageV: providerFloat(1.8), MaxVoltageV: providerFloat(1.9)}},
				{ID: "output_supply_in", Kind: "power", Direction: "sink", Domain: "output_supply", Electrical: Electrical{MinVoltageV: providerFloat(3), NominalVoltageV: providerFloat(3.3), MaxVoltageV: providerFloat(3.6)}},
			},
			OperatingCases: []OperatingCase{{
				ID: "bounded",
				Conditions: []OperatingCondition{
					{Axis: "supply_voltage", Target: "input_supply", Min: 1.7, Max: 1.9, Unit: "V"},
					{Axis: "supply_voltage", Target: "output_supply", Min: 3, Max: 3.6, Unit: "V"},
				},
			}},
			BehavioralRequirements: []BehavioralAssertion{
				{ID: "state_high", Metric: "output_high_voltage", Analysis: "dc_operating_point", Excitation: &Observation{Kind: "port", ID: "command_a"}, Observation: Observation{Kind: "port", ID: "state"}, Min: providerFloat(2.7), Max: providerFloat(3.6), Unit: "V", OperatingCases: []string{"bounded"}, Critical: true},
				{ID: "state_low", Metric: "output_low_voltage", Analysis: "dc_operating_point", Excitation: &Observation{Kind: "port", ID: "command_b"}, Observation: Observation{Kind: "port", ID: "state"}, Min: providerFloat(0), Max: providerFloat(.3), Unit: "V", OperatingCases: []string{"bounded"}, Critical: true},
			},
			Constraints: BoardLimits{MaxComponents: 12, MaxWidthMM: 40, MaxHeightMM: 30},
		},
		Acceptance: Acceptance{
			RequirePrimitiveOnly: true, RequireTopologySearch: true, RequireSimulation: true,
			RequireAllCorners: true, RequireModelProvenance: true, RequireClosedLoopEvidence: true,
			RequireCompleteRouting: true, RequireConnectivity: true, RequireWriterCorrectness: true,
			RequireRoundTripZeroDiff: true, RequireERC: true, RequireStrictDRC: true,
			RequireDeterministicReplay: true, RequireFailClosed: true,
		},
	}
}

func declarativeProviderInventory() PrimitiveInventory {
	inventory := testSearchInventory()
	fixedResistor := func(key string, value float64) PrimitiveCandidate {
		primitive := testPrimitive(key, "resistor", []string{"A", "B"}, &PrimitiveValueDomain{
			Kind: "resistance", Unit: "ohm", Minimum: providerFloat(value), Nominal: providerFloat(value), Maximum: providerFloat(value),
		})
		return primitive
	}
	inventory.Primitives = append(inventory.Primitives,
		fixedResistor("resistor|2k2", 2_200),
		fixedResistor("resistor|10k", 10_000),
		fixedResistor("resistor|100k", 100_000),
		testPrimitive("npn|sot23", "npn_bjt", []string{"BASE", "COLLECTOR", "EMITTER"}, nil),
		testPrimitive("pmos|sot23", "p_channel_mosfet", []string{"GATE", "DRAIN", "SOURCE"}, nil),
	)
	return inventory
}

func providerFloat(value float64) *float64 { return &value }
