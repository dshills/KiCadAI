package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/simmodel"
)

func TestPowerTransferRequirementIncludesUnityGainBuffers(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "powered_lowpass.json")
	unity := 1.0
	for index := range requirement.Requirements.BehavioralRequirements {
		if requirement.Requirements.BehavioralRequirements[index].Metric == "voltage_gain" {
			requirement.Requirements.BehavioralRequirements[index].Min = &unity
		}
	}
	requirement.Requirements.BehavioralRequirements = append(
		requirement.Requirements.BehavioralRequirements,
		BehavioralAssertion{Metric: "output_current"},
	)
	if !topologyRequiresPowerTransfer(requirement) {
		t.Fatal("unity-gain load-driving requirement did not select power-transfer topology search")
	}
}

func TestPrimitiveTopologySearchIsBoundedDeterministicAndProviderIndependent(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "powered_lowpass.json")
	inventory := testSearchInventory()
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 600
	policy.MaxGeneratedGraphs = 6_000
	policy.MaxPrimitiveInstances = 6
	policy.MaxInternalNodes = 2
	policy.MaxRetainedCandidates = 6

	first := SearchPrimitiveTopologies(context.Background(), requirement, inventory, policy)
	if first.Status != TopologySearchCandidates || len(first.Candidates) < 2 || len(first.Candidates) > policy.MaxRetainedCandidates {
		t.Fatalf("first search status/candidates = %s/%d, consumption=%#v issues=%#v", first.Status, len(first.Candidates), first.Consumption, first.Issues)
	}
	if first.RequirementHash == "" || first.InventoryHash != inventory.Hash ||
		first.Consumption.ExpandedStates <= 0 ||
		first.Consumption.GeneratedGraphs <= 0 ||
		first.Consumption.CompleteGraphs < len(first.Candidates) {
		t.Fatalf("search identity/consumption = %#v", first.Consumption)
	}
	dominanceRecorded := false
	for _, rejection := range first.Rejections {
		if rejection.Code == "dominated_topology" && rejection.Count > 0 && len(rejection.Samples) > 0 {
			dominanceRecorded = true
		}
	}
	if !dominanceRecorded && first.Consumption.ExpandedStates > 1 {
		t.Fatal("multi-state search did not record a canonical dominance proof")
	}
	topologies := map[string]bool{}
	for _, candidate := range first.Candidates {
		if candidate.Fingerprint == "" || candidate.TopologyHash == "" || topologies[candidate.TopologyHash] {
			t.Fatalf("candidate identity is empty or duplicate: %#v", candidate)
		}
		topologies[candidate.TopologyHash] = true
		if issues := ValidateCompleteGraph(candidate.Graph, inventory, GraphLimits{
			MaxPrimitiveInstances: policy.MaxPrimitiveInstances,
			MaxInternalNodes:      policy.MaxInternalNodes,
		}); len(issues) != 0 {
			t.Fatalf("candidate %s is not complete: %#v", candidate.Fingerprint, issues)
		}
		if len(candidate.Operations) == 0 {
			t.Fatalf("candidate %s lacks operation evidence", candidate.Fingerprint)
		}
		for index, operation := range candidate.Operations {
			if operation.Number != index+1 || operation.BeforeHash == "" || operation.AfterHash == "" ||
				(operation.Kind != "add_primitive" &&
					operation.Kind != "add_internal_node" &&
					operation.Kind != "redirect_terminal") {
				t.Fatalf("candidate %s invalid operation: %#v", candidate.Fingerprint, operation)
			}
		}
	}

	second := SearchPrimitiveTopologies(context.Background(), requirement, inventory, policy)
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("repeated search differs:\n%s\n%s", firstJSON, secondJSON)
	}

	reordered := inventory
	reordered.Primitives = append([]PrimitiveCandidate(nil), inventory.Primitives...)
	slices.Reverse(reordered.Primitives)
	third := SearchPrimitiveTopologies(context.Background(), requirement, reordered, policy)
	thirdJSON, err := json.Marshal(third)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, thirdJSON) {
		t.Fatalf("search depends on inventory order:\n%s\n%s", firstJSON, thirdJSON)
	}

	lower := strings.ToLower(string(firstJSON))
	for _, prohibited := range []string{"provider_id", "expansion_id", "sallen", "schmitt", "common_emitter"} {
		if strings.Contains(lower, prohibited) {
			t.Fatalf("search evidence contains pre-authored identity %q", prohibited)
		}
	}
}

func TestRepresentativePrimitivePrefersRequiredElectrothermalEvidence(t *testing.T) {
	required := map[string]bool{simmodel.AnalysisElectrothermal: true}
	model := func(parameters ...simmodel.NamedValue) PrimitiveModelContract {
		return PrimitiveModelContract{
			ModelID:         simmodel.PrimitiveComparatorOpenCollectorV1,
			Parameters:      parameters,
			AllowedAnalyses: []string{simmodel.AnalysisTransient},
		}
	}
	withoutThermalEvidence := PrimitiveCandidate{
		Key:     "comparator.alpha",
		AreaMM2: 1,
		Models: []PrimitiveModelContract{model(
			simmodel.NamedValue{Name: "max_temperature_c", Value: 125},
		)},
	}
	withThermalEvidence := PrimitiveCandidate{
		Key:     "comparator.zeta",
		AreaMM2: 2,
		Models: []PrimitiveModelContract{model(
			simmodel.NamedValue{Name: "max_temperature_c", Value: 125},
			simmodel.NamedValue{Name: "junction_to_ambient_c_per_w", Value: 100},
		)},
	}
	if compareRepresentativePrimitives(withThermalEvidence, withoutThermalEvidence, required) >= 0 {
		t.Fatal("electrothermal representative ranking did not retain the primitive with complete thermal evidence")
	}
	if compareRepresentativePrimitives(withoutThermalEvidence, withThermalEvidence, map[string]bool{simmodel.AnalysisTransient: true}) >= 0 {
		t.Fatal("non-electrothermal representative ranking did not preserve deterministic area ordering")
	}
}

func TestBehaviorScoringRecognizesGenericBufferedTimeConstant(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "powered_lowpass.json")
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	representatives := topologyRepresentatives(requirement, inventory)
	byKind := map[string]PrimitiveCandidate{}
	for _, primitive := range representatives {
		byKind[primitive.Kind] = primitive
	}
	opamp, opampFound := byKind["opamp"]
	resistor, resistorFound := byKind["resistor"]
	capacitor, capacitorFound := byKind["capacitor"]
	if !opampFound || !resistorFound || !capacitorFound {
		t.Fatalf("missing generic primitives: opamp=%t resistor=%t capacitor=%t",
			opampFound, resistorFound, capacitorFound)
	}
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	beforeInternal := graph
	graph = AddInternalNode(graph, "internal")
	internal := requireAddedNodeID(t, beforeInternal, graph)
	preferredPlacements := primitivePlacements(requirement, graph, opamp, maxPrimitivePlacementsPerKind)
	foundPreferredPlacement := false
	for _, placement := range preferredPlacements {
		terminals := map[string]string{}
		for _, connection := range placement {
			terminals[connection.Terminal] = connection.Node
		}
		if terminals["IN_MINUS"] == internal &&
			terminals["IN_PLUS"] == "port_ground" &&
			terminals["OUT"] == "port_output" {
			foundPreferredPlacement = true
		}
	}
	if !foundPreferredPlacement {
		t.Fatalf("generic endpoint preference omitted closed-loop placement: %#v", preferredPlacements)
	}
	graph = AddPrimitive(graph, opamp, nil, []TerminalConnection{
		{Terminal: "IN_MINUS", Node: internal},
		{Terminal: "IN_PLUS", Node: "port_ground"},
		{Terminal: "OUT", Node: "port_output"},
		{Terminal: "V_MINUS", Node: "port_ground"},
		{Terminal: "V_PLUS", Node: "port_power"},
	})
	reactivePlacements := primitivePlacements(
		requirement,
		graph,
		capacitor,
		maxPrimitivePlacementsPerKind,
	)
	foundFeedbackPlacement := false
	for _, placement := range reactivePlacements {
		if topologyPlacementBridgesActiveFeedback(graph, placement) {
			foundFeedbackPlacement = true
			break
		}
	}
	if !foundFeedbackPlacement {
		t.Fatalf(
			"generic passive placement omitted active feedback branch: %#v",
			reactivePlacements,
		)
	}
	incidental := CloneGraph(graph)
	incidental = AddPrimitive(
		incidental,
		resistor,
		seedPrimitiveValue(resistor),
		[]TerminalConnection{
			{Terminal: "A", Node: internal},
			{Terminal: "B", Node: "port_output"},
		},
	)
	incidental = AddPrimitive(
		incidental,
		capacitor,
		seedPrimitiveValue(capacitor),
		[]TerminalConnection{
			{Terminal: "A", Node: internal},
			{Terminal: "B", Node: "port_power"},
		},
	)
	incidental = AddPrimitive(
		incidental,
		resistor,
		seedPrimitiveValue(resistor),
		[]TerminalConnection{
			{Terminal: "A", Node: "port_input"},
			{Terminal: "B", Node: internal},
		},
	)
	if gap := topologyBehaviorGap(
		requirement,
		incidental,
		primitiveInventoryByKey(inventory),
	); gap == 0 {
		t.Fatal("rail-attached reactance falsely satisfied high-frequency attenuation")
	}
	graph = AddPrimitive(graph, resistor, seedPrimitiveValue(resistor), []TerminalConnection{
		{Terminal: "A", Node: internal},
		{Terminal: "B", Node: "port_output"},
	})
	graph = AddPrimitive(graph, capacitor, seedPrimitiveValue(capacitor), []TerminalConnection{
		{Terminal: "A", Node: internal},
		{Terminal: "B", Node: "port_output"},
	})
	graph = AddPrimitive(graph, resistor, seedPrimitiveValue(resistor), []TerminalConnection{
		{Terminal: "A", Node: "port_input"},
		{Terminal: "B", Node: internal},
	})
	if completeIssues := ValidateCompleteGraph(graph, inventory, GraphLimits{
		MaxPrimitiveInstances: DefaultPolicy().MaxPrimitiveInstances,
		MaxInternalNodes:      DefaultPolicy().MaxInternalNodes,
	}); len(completeIssues) != 0 {
		t.Fatalf("generic closed-loop time constant is incomplete: %#v", completeIssues)
	}
	if gap := topologyBehaviorGap(requirement, graph, primitiveInventoryByKey(inventory)); gap != 0 {
		t.Fatalf("generic closed-loop time constant behavior gap = %d", gap)
	}
}

func TestAnalogRelationshipSeedsTreatBandwidthAsFrequencyObligation(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "powered_lowpass.json")
	for index := range requirement.Requirements.BehavioralRequirements {
		if requirement.Requirements.BehavioralRequirements[index].Metric == "cutoff_frequency" {
			requirement.Requirements.BehavioralRequirements[index].Metric = "bandwidth"
		}
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	initialGraph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	initialHash, err := GraphHash(initialGraph)
	if err != nil {
		t.Fatal(err)
	}
	initialTopology, err := TopologyHash(initialGraph)
	if err != nil {
		t.Fatal(err)
	}
	inventoryByKey := primitiveInventoryByKey(inventory)
	initial := topologySearchState{
		graph: initialGraph, hash: initialHash, topology: initialTopology,
		score: scoreTopologyGraph(requirement, initialGraph, inventoryByKey, initialHash),
	}
	policy := DefaultPolicy()
	candidates, _, rejections := topologyAnalogRelationshipSeeds(
		context.Background(), requirement, inventory,
		topologyRepresentatives(requirement, inventory), inventoryByKey,
		GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes},
		policy, initial,
	)
	if len(candidates) == 0 {
		t.Fatalf("bandwidth relationship produced no candidates: %#v", rejections)
	}
	foundClosedLoop := false
	for _, candidate := range candidates {
		if topologyGraphHasAnalogFeedback(candidate.Graph) && candidate.Score.BehaviorGap == 0 {
			foundClosedLoop = true
			break
		}
	}
	if !foundClosedLoop {
		t.Fatalf("bandwidth relationship lacks a complete negative-feedback candidate: %#v", candidates)
	}

	shifted := requirement
	shifted.Requirements.Ports = append([]Port(nil), requirement.Requirements.Ports...)
	for index := range shifted.Requirements.Ports {
		if shifted.Requirements.Ports[index].ID != "output" {
			continue
		}
		minimum, nominal, maximum := 0.0, 6.0, 12.0
		shifted.Requirements.Ports[index].Electrical.MinVoltageV = &minimum
		shifted.Requirements.Ports[index].Electrical.NominalVoltageV = &nominal
		shifted.Requirements.Ports[index].Electrical.MaxVoltageV = &maximum
	}
	shiftedInitialGraph, issues := InitialGraph(shifted)
	if len(issues) != 0 {
		t.Fatalf("shifted initial graph issues: %#v", issues)
	}
	shiftedInitialHash, err := GraphHash(shiftedInitialGraph)
	if err != nil {
		t.Fatal(err)
	}
	shiftedInitialTopology, err := TopologyHash(shiftedInitialGraph)
	if err != nil {
		t.Fatal(err)
	}
	shiftedInitial := topologySearchState{
		graph: shiftedInitialGraph, hash: shiftedInitialHash, topology: shiftedInitialTopology,
		score: scoreTopologyGraph(shifted, shiftedInitialGraph, inventoryByKey, shiftedInitialHash),
	}
	shiftedCandidates, _, shiftedRejections := topologyAnalogRelationshipSeeds(
		context.Background(), shifted, inventory,
		topologyRepresentatives(shifted, inventory), inventoryByKey,
		GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes},
		policy, shiftedInitial,
	)
	var biased *TopologyCandidate
	for index := range shiftedCandidates {
		candidate := &shiftedCandidates[index]
		for _, instance := range candidate.Graph.Instances {
			for _, scale := range deriveAnalogTransferTopologyScales(
				shifted, candidate.Graph, instance, inventoryByKey,
			) {
				if strings.HasPrefix(scale.ID, "topology:affine_transfer:") {
					biased = candidate
					break
				}
			}
			if biased != nil {
				break
			}
		}
	}
	if biased == nil {
		t.Fatalf("bipolar-to-unipolar behavior lacks a rail-biased affine transfer: %#v", shiftedRejections)
	}
	scales := map[string]float64{}
	for _, instance := range biased.Graph.Instances {
		for _, scale := range deriveAnalogTransferTopologyScales(shifted, biased.Graph, instance, inventoryByKey) {
			scales[scale.ID] = scale.ValueSI
		}
	}
	for _, id := range []string{
		"topology:affine_transfer:input",
		"topology:affine_transfer:bias_lower",
		"topology:affine_transfer:bias_upper",
		"topology:affine_transfer:bias_bypass",
	} {
		if scales[id] <= 0 {
			t.Fatalf("biased affine transfer lacks %s scale: %#v", id, scales)
		}
	}
	feedback := scales["topology:affine_transfer:feedback"] +
		scales["topology:affine_transfer:series_feedback_input_side"] +
		scales["topology:affine_transfer:series_feedback_output_side"]
	if feedback <= 0 {
		t.Fatalf("biased affine transfer lacks direct or composed feedback scales: %#v", scales)
	}
	shiftedGain := 0.0
	for _, assertion := range shifted.Requirements.BehavioralRequirements {
		if assertion.Metric == "voltage_gain" {
			shiftedGain = math.Max(shiftedGain, assertionTarget(assertion))
		}
	}
	if analogInvertingSeriesFeedbackRequired(shifted, inventoryByKey, shiftedGain) {
		if scales["topology:affine_transfer:feedback"] != 0 ||
			scales["topology:affine_transfer:series_feedback_input_side"] <= 0 ||
			scales["topology:affine_transfer:series_feedback_output_side"] <= 0 {
			t.Fatalf("sparse catalog did not produce bounded series feedback: %#v", scales)
		}
	}
}

func TestTopologyObligationsDerivePrimitiveThresholdNetworks(t *testing.T) {
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	for _, test := range []struct {
		fixture          string
		decisionStages   int
		resistorBranches int
	}{
		{fixture: "hysteretic_detector.json", decisionStages: 1, resistorBranches: 4},
		{fixture: "voltage_window_monitor.json", decisionStages: 2, resistorBranches: 4},
	} {
		requirement := testOpenTopologyRequirement(t, test.fixture)
		sequences := topologyObligationKindSequences(
			requirement,
			topologyRepresentatives(requirement, inventory),
			128,
		)
		if len(sequences) == 0 {
			t.Fatalf("%s produced no primitive obligation sequences", test.fixture)
		}
		for _, sequence := range sequences {
			decisions, resistors := 0, 0
			for _, primitive := range sequence {
				if primitive.Kind == "comparator" || primitive.Kind == "opamp" {
					decisions++
				}
				if primitive.Kind == "resistor" {
					resistors++
				}
			}
			if decisions != test.decisionStages || resistors != test.resistorBranches {
				t.Fatalf(
					"%s obligation counts = decisions:%d resistors:%d; want %d/%d",
					test.fixture,
					decisions,
					resistors,
					test.decisionStages,
					test.resistorBranches,
				)
			}
		}
	}
}

func TestDecisionRelationshipSeedsConstructBoundedPrimitiveGraph(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "hysteretic_detector.json")
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	initial, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	hash, err := GraphHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	topology, err := TopologyHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	byKey := primitiveInventoryByKey(inventory)
	state := topologySearchState{
		graph:    initial,
		hash:     hash,
		topology: topology,
		score:    scoreTopologyGraph(requirement, initial, byKey, hash),
	}
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 32
	policy.MaxGeneratedGraphs = 256
	candidates, consumption, rejections := topologyRelationshipSeeds(
		context.Background(),
		requirement,
		inventory,
		topologyRepresentatives(requirement, inventory),
		byKey,
		GraphLimits{
			MaxPrimitiveInstances: policy.MaxPrimitiveInstances,
			MaxInternalNodes:      policy.MaxInternalNodes,
		},
		policy,
		state,
	)
	if len(candidates) == 0 {
		t.Fatalf(
			"decision relationship construction produced no candidates: consumption=%#v rejections=%#v",
			consumption,
			rejections,
		)
	}
	foundRailDerived := false
	foundStableReference := false
	for _, candidate := range candidates {
		if candidate.Score.BehaviorGap != 0 {
			t.Fatalf("unexpected relationship candidate: score=%#v graph=%s", candidate.Score, testGraphTopologySummary(candidate.Graph))
		}
		if candidate.Score.InternalNodeCount == 2 &&
			len(candidate.Graph.Instances) == 5 {
			foundRailDerived = true
		}
		if candidate.Score.InternalNodeCount == 4 &&
			len(candidate.Graph.Instances) == 8 {
			for _, instance := range candidate.Graph.Instances {
				if instance.Kind == "reference_diode" {
					foundStableReference = true
				}
			}
		}
		if len(candidate.Operations) !=
			candidate.Score.InternalNodeCount+len(candidate.Graph.Instances) {
			t.Fatalf(
				"relationship operation count = %d; want %d",
				len(candidate.Operations),
				candidate.Score.InternalNodeCount+len(candidate.Graph.Instances),
			)
		}
		for index, operation := range candidate.Operations {
			if operation.Number != index+1 ||
				operation.BeforeHash == "" ||
				operation.AfterHash == "" {
				t.Fatalf("invalid relationship operation %d: %#v", index, operation)
			}
		}
	}
	if !foundRailDerived || !foundStableReference {
		t.Fatalf(
			"relationship candidates rail/stable = %t/%t; candidates=%d consumption=%#v rejections=%#v",
			foundRailDerived,
			foundStableReference,
			len(candidates),
			consumption,
			rejections,
		)
	}
}

func TestConditionalTransferRelationshipSeedsConstructBoundedPrimitiveGraph(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "audio_mute.json")
	relationships := topologyConditionalTransferRelationships(requirement)
	if len(relationships) != 1 ||
		relationships[0].input != "audio_input" ||
		relationships[0].output != "audio_output" ||
		relationships[0].control != "mute_control" {
		t.Fatalf("conditional transfer relationships = %#v", relationships)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	initial, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	hash, err := GraphHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	topology, err := TopologyHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	byKey := primitiveInventoryByKey(inventory)
	state := topologySearchState{
		graph:    initial,
		hash:     hash,
		topology: topology,
		score:    scoreTopologyGraph(requirement, initial, byKey, hash),
	}
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 32
	policy.MaxGeneratedGraphs = 256
	candidates, consumption, rejections := topologyConditionalTransferRelationshipSeeds(
		context.Background(),
		requirement,
		inventory,
		topologyRepresentatives(requirement, inventory),
		byKey,
		GraphLimits{
			MaxPrimitiveInstances: policy.MaxPrimitiveInstances,
			MaxInternalNodes:      policy.MaxInternalNodes,
		},
		policy,
		state,
	)
	if len(candidates) == 0 {
		t.Fatalf(
			"conditional transfer relationship produced no candidates: consumption=%#v rejections=%#v",
			consumption,
			rejections,
		)
	}
	for _, candidate := range candidates {
		counts := map[string]int{}
		for _, instance := range candidate.Graph.Instances {
			counts[instance.Kind]++
		}
		if candidate.Score.BehaviorGap != 0 ||
			candidate.Score.InternalNodeCount != 2 ||
			len(candidate.Graph.Instances) != 7 ||
			counts["opamp"] != 1 ||
			counts["n_channel_mosfet"] != 1 ||
			counts["resistor"] != 3 ||
			counts["capacitor"] != 2 {
			t.Fatalf(
				"unexpected conditional transfer relationship: score=%#v counts=%#v graph=%s",
				candidate.Score,
				counts,
				testGraphTopologySummary(candidate.Graph),
			)
		}
		if len(candidate.Operations) != 9 {
			t.Fatalf("conditional transfer operations = %d; want 9", len(candidate.Operations))
		}
		plan := BuildValueSearchPlan(
			requirement,
			candidate.Graph,
			inventory,
			policy,
		)
		if plan.Status != ValuePlanReady {
			t.Fatalf(
				"conditional transfer value plan = %s: rejections=%#v issues=%#v",
				plan.Status,
				plan.Rejections,
				plan.Issues,
			)
		}
		foundBias, foundCoupling := false, false
		for _, domain := range plan.Domains {
			for _, scale := range domain.AnalyticScales {
				foundBias = foundBias ||
					(scale.Kind == "resistance" && testValueSIEqual(scale.ValueSI, 47_000))
				foundCoupling = foundCoupling ||
					(scale.Kind == "capacitance" && testValueSIEqual(scale.ValueSI, 4.7e-6))
			}
		}
		if !foundBias || !foundCoupling {
			t.Fatalf(
				"conditional transfer scales bias/coupling = %t/%t: %#v",
				foundBias,
				foundCoupling,
				plan.Domains,
			)
		}
	}
}

func TestControlledSwitchRelationshipSeedsConstructBoundedPrimitiveGraph(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "ground_referenced_load_control.json")
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	initial, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	hash, err := GraphHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	topology, err := TopologyHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	byKey := primitiveInventoryByKey(inventory)
	state := topologySearchState{
		graph:    initial,
		hash:     hash,
		topology: topology,
		score:    scoreTopologyGraph(requirement, initial, byKey, hash),
	}
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 32
	policy.MaxGeneratedGraphs = 256
	candidates, consumption, rejections := topologyControlledSwitchRelationshipSeeds(
		context.Background(),
		requirement,
		inventory,
		topologyRepresentatives(requirement, inventory),
		byKey,
		GraphLimits{
			MaxPrimitiveInstances: policy.MaxPrimitiveInstances,
			MaxInternalNodes:      policy.MaxInternalNodes,
		},
		policy,
		state,
	)
	if len(candidates) == 0 {
		t.Fatalf(
			"controlled relationship construction produced no candidates: consumption=%#v rejections=%#v",
			consumption,
			rejections,
		)
	}
	for _, candidate := range candidates {
		counts := map[string]int{}
		for _, instance := range candidate.Graph.Instances {
			counts[instance.Kind]++
		}
		if candidate.Score.BehaviorGap != 0 ||
			candidate.Score.InternalNodeCount != 2 ||
			len(candidate.Graph.Instances) != 7 ||
			counts["comparator"] != 1 ||
			counts["n_channel_mosfet"] != 1 ||
			counts["resistor"] != 4 ||
			counts["signal_diode"] != 1 {
			t.Fatalf(
				"unexpected controlled relationship: score=%#v counts=%#v graph=%s",
				candidate.Score,
				counts,
				testGraphTopologySummary(candidate.Graph),
			)
		}
		if len(candidate.Operations) != 9 {
			t.Fatalf("controlled relationship operations = %d; want 9", len(candidate.Operations))
		}
		plan := BuildValueSearchPlan(
			requirement,
			candidate.Graph,
			inventory,
			policy,
		)
		if plan.Status != ValuePlanReady {
			t.Fatalf(
				"controlled relationship value plan = %s: rejections=%#v issues=%#v",
				plan.Status,
				plan.Rejections,
				plan.Issues,
			)
		}
		for _, domain := range plan.Domains {
			foundAnchor := false
			for _, scale := range domain.AnalyticScales {
				if domain.PrimitiveKind == "resistor" &&
					scale.Kind == "resistance" &&
					scale.SourceKind == "candidate_topology" &&
					scale.Priority == 1 &&
					scale.ValueSI > 0 &&
					!math.IsInf(scale.ValueSI, 0) &&
					!math.IsNaN(scale.ValueSI) {
					foundAnchor = true
				}
			}
			if domain.PrimitiveKind == "resistor" && !foundAnchor {
				t.Fatalf("interface domain lacks bounded analytic scale: %#v", domain)
			}
		}
	}
}

func TestFlybackDiodeSelectionPrioritizesDeclaredThermalHeadroom(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, filepath.Join(
		nonlinearSwitchingCorpusRoot(), "controlled_pulse_power_stage.json",
	))))
	if len(issues) != 0 {
		t.Fatalf("requirement issues = %#v", issues)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	selected := topologyFlybackDiodePrimitive(requirement, inventory)
	if selected.Key == "" {
		t.Fatal("flyback diode selection returned no reviewed primitive")
	}
	requiredCurrent := requirementMaximumMetric(requirement, "peak_current")
	thermalExcess := math.MaxFloat64
	for _, model := range selected.Models {
		if model.ModelID != simmodel.PrimitiveDiodeShockleyV1 {
			continue
		}
		parameters := map[string]float64{}
		for _, parameter := range model.Parameters {
			parameters[parameter.Name] = parameter.Value
		}
		forwardVoltage := parameters["emission_coefficient"] * 8.617333262e-5 *
			parameters["junction_temperature_k"] * math.Log1p(requiredCurrent/parameters["saturation_current_a"])
		thermalExcess, _ = topologyFlybackDiodeThermalScore(requirement, model, requiredCurrent, forwardVoltage)
	}
	if thermalExcess != 0 {
		t.Fatalf("selected flyback diode %s has %.12g C conservative thermal excess", selected.Key, thermalExcess)
	}
}

func TestControlledSwitchFlybackUsesLoadSupplyInsteadOfAuxiliaryDriveRail(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, filepath.Join(
		nonlinearSwitchingCorpusRoot(), "controlled_pulse_power_stage.json",
	))))
	if len(issues) != 0 {
		t.Fatalf("requirement issues = %#v", issues)
	}
	auxiliaryDomain := requirement.Requirements.Domains[1]
	auxiliaryDomain.ID = "auxiliary_drive"
	auxiliaryDomain.MinVoltageV = graphFloat(7.5)
	auxiliaryDomain.NominalVoltageV = graphFloat(8)
	auxiliaryDomain.MaxVoltageV = graphFloat(8.5)
	auxiliaryDomain.MaxCurrentA = graphFloat(.1)
	requirement.Requirements.Domains = append(requirement.Requirements.Domains, auxiliaryDomain)
	auxiliaryPort := requirement.Requirements.Ports[1]
	auxiliaryPort.ID = "auxiliary_drive_power"
	auxiliaryPort.Domain = auxiliaryDomain.ID
	requirement.Requirements.Ports = append(requirement.Requirements.Ports, auxiliaryPort)

	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("build three-rail controlled-switch graph: %#v", issues)
	}
	supplies := topologyNodesByRole(graph, "supply")
	loadSupply, found := topologyControlledSwitchLoadSupply(requirement, graph, "port_load_output", supplies)
	if !found || loadSupply != "port_load_power" {
		t.Fatalf("controlled-switch load supply = %q found=%t, want port_load_power rather than auxiliary drive rail; supplies=%v", loadSupply, found, supplies)
	}
}

func TestTransconductanceRelationshipSeedsConstructBoundedPrimitiveGraph(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "adjustable_current_output.json")
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	initial, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	hash, err := GraphHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	topology, err := TopologyHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	byKey := primitiveInventoryByKey(inventory)
	state := topologySearchState{
		graph:    initial,
		hash:     hash,
		topology: topology,
		score:    scoreTopologyGraph(requirement, initial, byKey, hash),
	}
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 32
	policy.MaxGeneratedGraphs = 256
	candidates, consumption, rejections := topologyTransconductanceRelationshipSeeds(
		context.Background(),
		requirement,
		inventory,
		topologyRepresentatives(requirement, inventory),
		byKey,
		GraphLimits{
			MaxPrimitiveInstances: policy.MaxPrimitiveInstances,
			MaxInternalNodes:      policy.MaxInternalNodes,
		},
		policy,
		state,
	)
	if len(candidates) == 0 {
		t.Fatalf(
			"transconductance relationship produced no candidates: consumption=%#v rejections=%#v",
			consumption,
			rejections,
		)
	}
	for _, candidate := range candidates {
		counts := map[string]int{}
		for _, instance := range candidate.Graph.Instances {
			counts[instance.Kind]++
		}
		if candidate.Score.BehaviorGap != 0 ||
			candidate.Score.InternalNodeCount != 8 ||
			len(candidate.Graph.Instances) != 13 ||
			counts["opamp"] != 2 ||
			counts["pnp_bjt"] != 2 ||
			counts["resistor"] != 9 {
			t.Fatalf(
				"unexpected transconductance relationship: score=%#v counts=%#v graph=%s",
				candidate.Score,
				counts,
				testGraphTopologySummary(candidate.Graph),
			)
		}
		if len(candidate.Operations) != 21 {
			t.Fatalf("transconductance relationship operations = %d; want 21", len(candidate.Operations))
		}
		plan := BuildValueSearchPlan(
			requirement,
			candidate.Graph,
			inventory,
			policy,
		)
		if plan.Status != ValuePlanReady {
			t.Fatalf(
				"transconductance value plan = %s: rejections=%#v issues=%#v",
				plan.Status,
				plan.Rejections,
				plan.Issues,
			)
		}
		foundSense, foundRatio := false, false
		for _, domain := range plan.Domains {
			for _, scale := range domain.AnalyticScales {
				foundSense = foundSense ||
					(scale.Kind == "resistance" && testValueSIEqual(scale.ValueSI, 10))
				foundRatio = foundRatio ||
					(scale.Kind == "resistance" && testValueSIEqual(scale.ValueSI, 10_000))
			}
		}
		if !foundSense || !foundRatio {
			t.Fatalf(
				"transconductance value plan sense/ratio scales = %t/%t: %#v",
				foundSense,
				foundRatio,
				plan.Domains,
			)
		}
	}
}

func TestTransconductanceRelationshipConditionsTransientCommand(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), "illumination_proportional_power_control.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement issues = %#v", issues)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	initial, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("initial graph issues: %#v", graphIssues)
	}
	hash, err := GraphHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	topology, err := TopologyHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	byKey := primitiveInventoryByKey(inventory)
	state := topologySearchState{
		graph: initial, hash: hash, topology: topology,
		score: scoreTopologyGraph(requirement, initial, byKey, hash),
	}
	policy := multiStageOODPromotionPolicy()
	candidates, consumption, rejections := topologyTransconductanceRelationshipSeeds(
		context.Background(),
		requirement,
		inventory,
		topologyRepresentatives(requirement, inventory),
		byKey,
		GraphLimits{
			MaxPrimitiveInstances: policy.MaxPrimitiveInstances,
			MaxInternalNodes:      policy.MaxInternalNodes,
		},
		policy,
		state,
	)
	if len(candidates) == 0 {
		t.Fatalf(
			"transient transconductance relationship produced no candidates: consumption=%#v rejections=%#v",
			consumption,
			rejections,
		)
	}
	for _, candidate := range candidates {
		if candidate.Score.BehaviorGap != 0 {
			continue
		}
		nodes := map[string]GraphNode{}
		for _, node := range candidate.Graph.Nodes {
			nodes[node.ID] = node
		}
		conditionedNode, resistorID, capacitorID := "", "", ""
		for _, capacitor := range candidate.Graph.Instances {
			if capacitor.Kind != "capacitor" || len(capacitor.Terminals) != 2 {
				continue
			}
			first, second := capacitor.Terminals[0].Node, capacitor.Terminals[1].Node
			if nodes[first].Role == "reference" && nodes[second].Scope == "internal" {
				conditionedNode, capacitorID = second, capacitor.ID
			} else if nodes[second].Role == "reference" && nodes[first].Scope == "internal" {
				conditionedNode, capacitorID = first, capacitor.ID
			}
		}
		for _, resistor := range candidate.Graph.Instances {
			if resistor.Kind != "resistor" || len(resistor.Terminals) != 2 {
				continue
			}
			first, second := resistor.Terminals[0].Node, resistor.Terminals[1].Node
			if first == conditionedNode && nodes[second].Scope == "external" && nodes[second].Role == "input" ||
				second == conditionedNode && nodes[first].Scope == "external" && nodes[first].Role == "input" {
				resistorID = resistor.ID
				break
			}
		}
		if conditionedNode == "" || resistorID == "" || capacitorID == "" {
			continue
		}
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
		if plan.Status != ValuePlanReady {
			t.Fatalf("conditioned transconductance value plan = %s: %#v", plan.Status, plan.Issues)
		}
		found := map[string]bool{}
		for _, domain := range plan.Domains {
			for _, scale := range domain.AnalyticScales {
				if scale.Priority == 1 && strings.HasPrefix(scale.ID, "topology:conditioned_transconductance_input:") {
					found[domain.InstanceID] = true
				}
			}
		}
		if !found[resistorID] || !found[capacitorID] {
			t.Fatalf("conditioned transconductance scales missing resistor=%t capacitor=%t: %#v", found[resistorID], found[capacitorID], plan.Domains)
		}
		return
	}
	t.Fatalf("no zero-gap transient transconductance relationship found: %#v", candidates)
}

func TestTransimpedanceRelationshipSeedsConstructBoundedPrimitiveGraph(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(architectureGeneralizationCorpusRoot(), "low_current_voltage_converter.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	initial, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("initial graph issues: %#v", graphIssues)
	}
	hash, err := GraphHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	topology, err := TopologyHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	byKey := primitiveInventoryByKey(inventory)
	state := topologySearchState{
		graph: initial, hash: hash, topology: topology,
		score: scoreTopologyGraph(requirement, initial, byKey, hash),
	}
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 32
	policy.MaxGeneratedGraphs = 256
	candidates, consumption, rejections := topologyTransimpedanceRelationshipSeeds(
		context.Background(), requirement, inventory,
		topologyRepresentatives(requirement, inventory), byKey,
		GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes},
		policy, state,
	)
	if len(candidates) < 1 {
		t.Fatalf("transimpedance relationship produced no candidates: consumption=%#v rejections=%#v", consumption, rejections)
	}
	for _, candidate := range candidates {
		if candidate.Score.BehaviorGap != 0 || internalNodeCount(candidate.Graph) > 1 {
			t.Fatalf("unexpected transimpedance score=%#v graph=%s", candidate.Score, testGraphTopologySummary(candidate.Graph))
		}
		counts := map[string]int{}
		for _, instance := range candidate.Graph.Instances {
			counts[instance.Kind]++
		}
		if counts["opamp"] != 1 || counts["resistor"] < 1 || counts["resistor"] > 2 || counts["capacitor"] > 1 {
			t.Fatalf("unexpected transimpedance primitive counts=%#v graph=%s", counts, testGraphTopologySummary(candidate.Graph))
		}
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
		if plan.Status != ValuePlanReady {
			t.Fatalf("transimpedance value plan=%s rejections=%#v issues=%#v", plan.Status, plan.Rejections, plan.Issues)
		}
		foundFeedbackScale := false
		seriesScaleTotal := 0.0
		for _, domain := range plan.Domains {
			for _, scale := range domain.AnalyticScales {
				foundFeedbackScale = foundFeedbackScale ||
					(domain.PrimitiveKind == "resistor" &&
						(scale.ID == "topology:current_to_voltage:feedback_resistance" && testValueSIEqual(scale.ValueSI, 100_000)))
				if domain.PrimitiveKind == "resistor" && scale.ID == "topology:current_to_voltage:series_feedback_resistance" {
					seriesScaleTotal += scale.ValueSI
				}
			}
		}
		foundFeedbackScale = foundFeedbackScale || math.Abs(seriesScaleTotal-100_000) <= 5_000
		if !foundFeedbackScale {
			t.Fatalf("transimpedance value plan lacks 100 kohm feedback derivation: %#v", plan.Domains)
		}
	}
}

func TestFullWaveRelationshipSeedsConstructBoundedPrimitiveGraph(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(architectureGeneralizationCorpusRoot(), "low_level_full_wave_transfer.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	initial, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("initial graph issues: %#v", graphIssues)
	}
	hash, err := GraphHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	topology, err := TopologyHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	byKey := primitiveInventoryByKey(inventory)
	state := topologySearchState{
		graph: initial, hash: hash, topology: topology,
		score: scoreTopologyGraph(requirement, initial, byKey, hash),
	}
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 32
	policy.MaxGeneratedGraphs = 256
	candidates, consumption, rejections := topologyFullWaveRelationshipSeeds(
		context.Background(), requirement, inventory,
		topologyRepresentatives(requirement, inventory), byKey,
		GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes},
		policy, state,
	)
	if len(candidates) != 2 {
		t.Fatalf("full-wave relationship produced %d candidates: consumption=%#v rejections=%#v", len(candidates), consumption, rejections)
	}
	foundCompensation := false
	for _, candidate := range candidates {
		if candidate.Score.BehaviorGap != 0 || internalNodeCount(candidate.Graph) != 7 {
			t.Fatalf("unexpected full-wave score=%#v graph=%s", candidate.Score, testGraphTopologySummary(candidate.Graph))
		}
		counts := map[string]int{}
		for _, instance := range candidate.Graph.Instances {
			counts[instance.Kind]++
		}
		if counts["opamp"] != 2 || counts["signal_diode"] != 2 || counts["resistor"] != 9 || counts["capacitor"] > 1 {
			t.Fatalf("unexpected full-wave primitive counts=%#v graph=%s", counts, testGraphTopologySummary(candidate.Graph))
		}
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
		if plan.Status != ValuePlanReady {
			t.Fatalf("full-wave value plan=%s rejections=%#v issues=%#v", plan.Status, plan.Rejections, plan.Issues)
		}
		values := map[float64]int{}
		for _, domain := range plan.Domains {
			for _, scale := range domain.AnalyticScales {
				if domain.PrimitiveKind == "resistor" && strings.HasPrefix(scale.ID, "topology:full_wave_ratio:") {
					values[scale.ValueSI]++
				}
				if domain.PrimitiveKind == "capacitor" && strings.HasPrefix(scale.ID, "topology:full_wave_compensation:") && testValueSIEqual(scale.ValueSI, 10e-12) {
					foundCompensation = true
				}
			}
		}
		if values[169_000] != 2 || values[47_000] != 6 || values[1_000] != 1 {
			t.Fatalf("unexpected full-wave resistor derivations=%#v", values)
		}
	}
	if !foundCompensation {
		t.Fatal("full-wave relationships lack a bounded feedback-compensation alternative")
	}
}

func TestWindowRelationshipSeedsRetainBypassAlternative(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(architectureGeneralizationCorpusRoot(), "dual_threshold_window_indicator.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	initial, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("initial graph issues: %#v", graphIssues)
	}
	hash, err := GraphHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	topology, err := TopologyHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	byKey := primitiveInventoryByKey(inventory)
	state := topologySearchState{
		graph: initial, hash: hash, topology: topology,
		score: scoreTopologyGraph(requirement, initial, byKey, hash),
	}
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 32
	policy.MaxGeneratedGraphs = 256
	candidates, consumption, rejections := topologyWindowRelationshipSeeds(
		context.Background(), requirement, inventory,
		topologyRepresentatives(requirement, inventory), byKey,
		GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes},
		policy, state,
	)
	if len(candidates) != 2 {
		t.Fatalf("window relationships=%d consumption=%#v rejections=%#v", len(candidates), consumption, rejections)
	}
	foundBypass := false
	for _, candidate := range candidates {
		if candidate.Score.BehaviorGap != 0 || internalNodeCount(candidate.Graph) != 8 {
			t.Fatalf("unexpected window score=%#v graph=%s", candidate.Score, testGraphTopologySummary(candidate.Graph))
		}
		counts := map[string]int{}
		for _, instance := range candidate.Graph.Instances {
			counts[instance.Kind]++
		}
		if counts["comparator"] != 3 || counts["opamp"] != 2 || counts["reference_diode"] != 1 || counts["resistor"] != 9 || counts["capacitor"] > 1 {
			t.Fatalf("unexpected window primitive counts=%#v graph=%s", counts, testGraphTopologySummary(candidate.Graph))
		}
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
		if plan.Status != ValuePlanReady {
			t.Fatalf("window value plan=%s rejections=%#v issues=%#v", plan.Status, plan.Rejections, plan.Issues)
		}
		for _, domain := range plan.Domains {
			for _, scale := range domain.AnalyticScales {
				if strings.HasPrefix(scale.ID, "topology:window_supply_bypass:") && testValueSIEqual(scale.ValueSI, 100e-9) {
					foundBypass = true
				}
			}
		}
	}
	if !foundBypass {
		t.Fatal("window relationships lack a bounded local-bypass alternative")
	}
}

func TestWindowRelationshipSeedsRejectMissingRailRoles(t *testing.T) {
	requirement := testMultiBranchAnalogRequirement(t, "outside_window_supply_guard.json")
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	initial, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	policy := DefaultPolicy()
	byKey := primitiveInventoryByKey(inventory)
	for _, missingRole := range []string{"supply", "reference"} {
		t.Run(missingRole, func(t *testing.T) {
			graph := initial
			graph.Nodes = append([]GraphNode(nil), initial.Nodes...)
			for index := range graph.Nodes {
				if graph.Nodes[index].Role == missingRole {
					graph.Nodes[index].Role = "internal"
				}
			}
			candidates, consumption, rejections := topologyWindowRelationshipSeeds(
				context.Background(), requirement, inventory,
				topologyRepresentatives(requirement, inventory), byKey,
				GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes},
				policy, topologySearchState{graph: graph},
			)
			if len(candidates) != 0 || consumption != (Consumption{}) || len(rejections["relationship_gap"]) != 1 {
				t.Fatalf("missing %s candidates=%d consumption=%#v rejections=%#v", missingRole, len(candidates), consumption, rejections)
			}
		})
	}
}

func TestWindowRelationshipSeedsAttenuateBelowReferenceThreshold(t *testing.T) {
	requirement := testMultiBranchAnalogRequirement(t, "outside_window_supply_guard.json")
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, DefaultPolicy())
	for _, candidate := range search.Candidates {
		if graphContainsReferenceAttenuationBuffer(candidate.Graph, "port_ground") {
			return
		}
	}
	t.Fatal("below-reference window threshold lacks a buffered reference attenuator")
}

func graphContainsReferenceAttenuationBuffer(graph CandidateGraph, ground string) bool {
	referenceNodes := map[string]bool{}
	for _, instance := range graph.Instances {
		if instance.Kind == "reference_diode" {
			referenceNodes[topologyTerminalNodes(instance)["CATHODE"]] = true
		}
	}
	for _, instance := range graph.Instances {
		if instance.Kind != "opamp" {
			continue
		}
		terminals := topologyTerminalNodes(instance)
		tap := terminals["IN_PLUS"]
		if tap == "" || !graphHasResistorBetween(graph, terminals["IN_MINUS"], terminals["OUT"]) {
			continue
		}
		referenceBranch, groundBranch := false, false
		for _, passive := range graph.Instances {
			if passive.Kind != "resistor" {
				continue
			}
			nodes := topologyTerminalNodes(passive)
			other := ""
			switch {
			case nodes["A"] == tap:
				other = nodes["B"]
			case nodes["B"] == tap:
				other = nodes["A"]
			}
			referenceBranch = referenceBranch || referenceNodes[other]
			groundBranch = groundBranch || other == ground
		}
		if referenceBranch && groundBranch {
			return true
		}
	}
	return false
}

func graphHasResistorBetween(graph CandidateGraph, left, right string) bool {
	for _, instance := range graph.Instances {
		if instance.Kind != "resistor" {
			continue
		}
		nodes := topologyTerminalNodes(instance)
		if (nodes["A"] == left && nodes["B"] == right) || (nodes["A"] == right && nodes["B"] == left) {
			return true
		}
	}
	return false
}

func TestRegulatedVoltageRelationshipSeedsRetainBiasAlternative(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(architectureGeneralizationCorpusRoot(), "regulated_low_voltage_output.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	initial, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("initial graph issues: %#v", graphIssues)
	}
	hash, err := GraphHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	topology, err := TopologyHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	byKey := primitiveInventoryByKey(inventory)
	state := topologySearchState{
		graph: initial, hash: hash, topology: topology,
		score: scoreTopologyGraph(requirement, initial, byKey, hash),
	}
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 32
	policy.MaxGeneratedGraphs = 256
	candidates, consumption, rejections := topologyRegulatedVoltageRelationshipSeeds(
		context.Background(), requirement, inventory,
		topologyRepresentatives(requirement, inventory), byKey,
		GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes},
		policy, state,
	)
	if len(candidates) != 4 {
		t.Fatalf("regulated-voltage relationships=%d consumption=%#v rejections=%#v", len(candidates), consumption, rejections)
	}
	foundBleeder := false
	foundBufferedDrive := false
	driveStageCounts := map[int]bool{}
	activeStructures := map[string]bool{}
	for _, candidate := range candidates {
		if candidate.Score.BehaviorGap != 0 {
			t.Fatalf("unexpected regulated-voltage score=%#v graph=%s", candidate.Score, testGraphTopologySummary(candidate.Graph))
		}
		driveStages := 0
		for _, instance := range candidate.Graph.Instances {
			if instance.Kind == "pnp_bjt" {
				driveStages++
			}
		}
		driveStageCounts[driveStages] = true
		activeHash, err := ActiveStructureHash(candidate.Graph)
		if err != nil {
			t.Fatal(err)
		}
		activeStructures[activeHash] = true
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
		if plan.Status != ValuePlanReady {
			t.Fatalf("regulated-voltage value plan=%s rejections=%#v issues=%#v", plan.Status, plan.Rejections, plan.Issues)
		}
		for _, domain := range plan.Domains {
			for _, scale := range domain.AnalyticScales {
				if strings.HasPrefix(scale.ID, "topology:regulated_pass_bleeder:") && testValueSIEqual(scale.ValueSI, 10_000) {
					foundBleeder = true
				}
				if strings.HasPrefix(scale.ID, "topology:regulated_buffer_drive:") && testValueSIEqual(scale.ValueSI, 100) {
					foundBufferedDrive = true
				}
			}
		}
	}
	if !foundBleeder {
		t.Fatal("regulated-voltage relationships lack an emitter-referenced pass-device bias alternative")
	}
	if !foundBufferedDrive || !driveStageCounts[0] || !driveStageCounts[1] || len(activeStructures) < 2 {
		t.Fatalf("regulated-voltage relationships lack materially distinct direct/buffered drive: buffered_scale=%t drive_stage_counts=%v active_structures=%v", foundBufferedDrive, driveStageCounts, activeStructures)
	}

	outputOnly := requirement
	outputOnly.Requirements.BehavioralRequirements = slices.DeleteFunc(
		slices.Clone(requirement.Requirements.BehavioralRequirements),
		func(assertion BehavioralAssertion) bool { return assertion.Metric != "output_voltage" },
	)
	state.score = scoreTopologyGraph(outputOnly, initial, byKey, hash)
	outputOnlyCandidates, outputOnlyConsumption, outputOnlyRejections := topologyRegulatedVoltageRelationshipSeeds(
		context.Background(), outputOnly, inventory,
		topologyRepresentatives(outputOnly, inventory), byKey,
		GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes},
		policy, state,
	)
	if len(outputOnlyCandidates) != 4 {
		t.Fatalf("output-only regulated-voltage relationships=%d consumption=%#v rejections=%#v", len(outputOnlyCandidates), outputOnlyConsumption, outputOnlyRejections)
	}
}

func TestBandpassRelationshipSeedsConstructBoundedPrimitiveGraph(t *testing.T) {
	data := mustRead(t, filepath.Join(architectureGeneralizationCorpusRoot(), "selective_midband_transfer.json"))
	requirement, issues := DecodeStrict(bytes.NewReader(data))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	implicitExcitation := requirement
	implicitExcitation.Requirements.BehavioralRequirements = slices.Clone(requirement.Requirements.BehavioralRequirements)
	for index := range implicitExcitation.Requirements.BehavioralRequirements {
		assertion := &implicitExcitation.Requirements.BehavioralRequirements[index]
		if slices.Contains([]string{"voltage_gain", "voltage_gain_at_frequency"}, assertion.Metric) {
			assertion.Excitation = nil
		}
	}
	implicitEnvelope, implicitOK := topologyBandpassBehaviorEnvelope(implicitExcitation)
	if !implicitOK || implicitEnvelope.input.Kind != "port" || implicitEnvelope.input.ID == "" {
		t.Fatalf("unique analog input did not provide deterministic implicit bandpass excitation: %#v", implicitEnvelope)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	initial, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("initial graph issues: %#v", graphIssues)
	}
	hash, err := GraphHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	topology, err := TopologyHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	byKey := primitiveInventoryByKey(inventory)
	state := topologySearchState{graph: initial, hash: hash, topology: topology, score: scoreTopologyGraph(requirement, initial, byKey, hash)}
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 32
	policy.MaxGeneratedGraphs = 256
	candidates, consumption, rejections := topologyBandpassRelationshipSeeds(
		context.Background(), requirement, inventory, topologyRepresentatives(requirement, inventory), byKey,
		GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes}, policy, state,
	)
	if len(candidates) < 2 {
		t.Fatalf("bandpass relationships=%d consumption=%#v rejections=%#v", len(candidates), consumption, rejections)
	}
	foundCascade := false
	for _, candidate := range candidates {
		counts := map[string]int{}
		for _, instance := range candidate.Graph.Instances {
			counts[instance.Kind]++
		}
		if candidate.Score.BehaviorGap != 0 {
			t.Fatalf("unexpected bandpass relationship score=%#v graph=%s", candidate.Score, testGraphTopologySummary(candidate.Graph))
		}
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
		if plan.Status != ValuePlanReady {
			t.Fatalf("bandpass value plan=%s rejections=%#v issues=%#v", plan.Status, plan.Rejections, plan.Issues)
		}
		foundLower, foundUpper := false, false
		for _, domain := range plan.Domains {
			for _, scale := range domain.AnalyticScales {
				foundLower = foundLower || strings.Contains(scale.ID, "topology:bracketed_passband:lower_corner_")
				foundUpper = foundUpper || strings.Contains(scale.ID, "topology:bracketed_passband:upper_corner_")
			}
		}
		if counts["opamp"] == 4 && counts["resistor"] == 4 && counts["capacitor"] == 4 {
			if !foundLower || !foundUpper {
				t.Fatalf("cascaded bandpass value plan lacks derived corners lower=%t upper=%t", foundLower, foundUpper)
			}
			foundCascade = true
		}
	}
	if !foundCascade {
		t.Fatalf("bandpass relationships lack second-order lower/upper cascade: %#v", candidates)
	}
}

func TestBehaviorScoringRejectsDegenerateDecisionFeedback(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "hysteretic_detector.json")
	if polarity := topologyDecisionPolarity(requirement); polarity != 1 {
		t.Fatalf("bounded low-input/low-output decision polarity = %d; want non-inverting", polarity)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	byKind := map[string]PrimitiveCandidate{}
	for _, primitive := range topologyRepresentatives(requirement, inventory) {
		byKind[primitive.Kind] = primitive
	}
	comparator, comparatorFound := byKind["comparator"]
	resistor, resistorFound := byKind["resistor"]
	if !comparatorFound || !resistorFound {
		t.Fatalf("missing decision primitives: comparator=%t resistor=%t", comparatorFound, resistorFound)
	}
	initial, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	preferred := AddInternalNode(initial, "internal")
	reference := requireAddedNodeID(t, initial, preferred)
	beforeSignal := preferred
	preferred = AddInternalNode(preferred, "internal")
	signal := requireAddedNodeID(t, beforeSignal, preferred)
	foundTwoNodeDecisionPlacement := false
	for _, placement := range primitivePlacements(requirement, preferred, comparator, maxPrimitivePlacementsPerKind) {
		terminals := map[string]string{}
		for _, connection := range placement {
			terminals[connection.Terminal] = connection.Node
		}
		if terminals["IN_MINUS"] == reference &&
			terminals["IN_PLUS"] == signal &&
			terminals["OUT"] == "port_indication" {
			foundTwoNodeDecisionPlacement = true
			break
		}
	}
	if !foundTwoNodeDecisionPlacement {
		t.Fatal("generic placement ordering omitted a two-node decision stage")
	}
	preferred = AddPrimitive(preferred, comparator, nil, []TerminalConnection{
		{Terminal: "IN_MINUS", Node: reference},
		{Terminal: "IN_PLUS", Node: signal},
		{Terminal: "OUT", Node: "port_indication"},
		{Terminal: "V_MINUS", Node: "port_ground"},
		{Terminal: "V_PLUS", Node: "port_power"},
	})
	desiredPassiveEdges := map[string]bool{
		reference + "|port_ground":  false,
		reference + "|port_power":   false,
		signal + "|port_input":      false,
		signal + "|port_indication": false,
	}
	for _, placement := range primitivePlacements(requirement, preferred, resistor, maxPrimitivePlacementsPerKind) {
		left, right := placement[0].Node, placement[1].Node
		if left > right {
			left, right = right, left
		}
		key := left + "|" + right
		if _, found := desiredPassiveEdges[key]; found {
			desiredPassiveEdges[key] = true
		}
	}
	for edge, found := range desiredPassiveEdges {
		if !found {
			t.Fatalf("generic placement ordering omitted decision-network edge %s", edge)
		}
	}
	for _, nodes := range [][2]string{
		{reference, "port_ground"},
		{reference, "port_power"},
		{signal, "port_input"},
		{signal, "port_indication"},
	} {
		preferred = AddPrimitive(preferred, resistor, seedPrimitiveValue(resistor), []TerminalConnection{
			{Terminal: "A", Node: nodes[0]},
			{Terminal: "B", Node: nodes[1]},
		})
	}
	if gap := topologyDecisionStageGap(preferred, 1, true, 1); gap != 0 {
		t.Fatalf("rail-referenced decision-stage gap = %d nodes=%#v graph=%s", gap, preferred.Nodes, testGraphTopologySummary(preferred))
	}
	if gap := topologyBehaviorGap(
		requirement,
		preferred,
		primitiveInventoryByKey(inventory),
	); gap != 0 {
		t.Fatalf("rail-referenced decision feedback behavior gap = %d", gap)
	}

	degenerate := AddPrimitive(initial, comparator, nil, []TerminalConnection{
		{Terminal: "IN_MINUS", Node: "port_indication"},
		{Terminal: "IN_PLUS", Node: "port_indication"},
		{Terminal: "OUT", Node: "port_indication"},
		{Terminal: "V_MINUS", Node: "port_ground"},
		{Terminal: "V_PLUS", Node: "port_power"},
	})
	for _, nodes := range [][2]string{
		{"port_input", "port_power"},
		{"port_indication", "port_input"},
		{"port_ground", "port_power"},
	} {
		degenerate = AddPrimitive(degenerate, resistor, seedPrimitiveValue(resistor), []TerminalConnection{
			{Terminal: "A", Node: nodes[0]},
			{Terminal: "B", Node: nodes[1]},
		})
	}
	if gap := topologyBehaviorGap(
		requirement,
		degenerate,
		primitiveInventoryByKey(inventory),
	); gap == 0 {
		t.Fatal("shorted decision terminals falsely satisfied threshold behavior")
	}

	railTied := AddPrimitive(initial, comparator, nil, []TerminalConnection{
		{Terminal: "IN_MINUS", Node: "port_ground"},
		{Terminal: "IN_PLUS", Node: "port_input"},
		{Terminal: "OUT", Node: "port_indication"},
		{Terminal: "V_MINUS", Node: "port_ground"},
		{Terminal: "V_PLUS", Node: "port_power"},
	})
	for _, nodes := range [][2]string{
		{"port_indication", "port_power"},
		{"port_ground", "port_indication"},
		{"port_ground", "port_input"},
	} {
		railTied = AddPrimitive(railTied, resistor, seedPrimitiveValue(resistor), []TerminalConnection{
			{Terminal: "A", Node: nodes[0]},
			{Terminal: "B", Node: nodes[1]},
		})
	}
	if gap := topologyBehaviorGap(
		requirement,
		railTied,
		primitiveInventoryByKey(inventory),
	); gap == 0 {
		t.Fatal("rail-tied decision input falsely satisfied intermediate threshold behavior")
	}
}

func TestPrimitiveTopologySearchStopsOnBudgetsCancellationAndMissingInventory(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "powered_lowpass.json")
	inventory := testSearchInventory()

	tiny := DefaultPolicy()
	tiny.MaxExpandedStates = 1
	tiny.MaxGeneratedGraphs = 1
	tiny.MaxPrimitiveInstances = 3
	tiny.MaxInternalNodes = 1
	tiny.MaxRetainedCandidates = 2
	exhausted := SearchPrimitiveTopologies(context.Background(), requirement, inventory, tiny)
	if exhausted.Status != TopologySearchExhausted ||
		!exhausted.Consumption.BudgetExhausted ||
		len(exhausted.Issues) != 1 ||
		exhausted.Issues[0].Code != CodeSearchExhausted {
		t.Fatalf("budget result = status=%s consumption=%#v issues=%#v", exhausted.Status, exhausted.Consumption, exhausted.Issues)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := SearchPrimitiveTopologies(ctx, requirement, inventory, DefaultPolicy())
	if canceled.Status != TopologySearchCanceled ||
		len(canceled.Issues) != 1 ||
		canceled.Issues[0].Code != CodeCanceled {
		t.Fatalf("canceled result = status=%s issues=%#v", canceled.Status, canceled.Issues)
	}

	unsupported := SearchPrimitiveTopologies(context.Background(), requirement, PrimitiveInventory{}, DefaultPolicy())
	if unsupported.Status != TopologySearchUnsupported ||
		len(unsupported.Issues) != 1 ||
		unsupported.Issues[0].Code != CodePrimitiveUnavailable {
		t.Fatalf("unsupported result = status=%s issues=%#v", unsupported.Status, unsupported.Issues)
	}
}

func TestGenericPlacementGenerationUsesTerminalRoles(t *testing.T) {
	graph := CandidateGraph{
		Schema:  CandidateGraphSchema,
		Version: CandidateGraphVersion,
		Nodes: []GraphNode{
			{ID: "ground", Scope: "external", SemanticKind: "port", SemanticID: "ground", Domain: "ground", Role: "reference"},
			{ID: "input", Scope: "external", SemanticKind: "port", SemanticID: "input", Domain: "ground", Role: "input"},
			{ID: "output", Scope: "external", SemanticKind: "port", SemanticID: "output", Domain: "ground", Role: "output"},
			{ID: "supply", Scope: "external", SemanticKind: "port", SemanticID: "power", Domain: "supply", Role: "supply"},
		},
	}
	inventory := testSearchInventory()
	var opamp PrimitiveCandidate
	for _, primitive := range inventory.Primitives {
		if primitive.Kind == "opamp" {
			opamp = primitive
			break
		}
	}
	placements := primitivePlacements(Requirement{}, graph, opamp, 128)
	if len(placements) == 0 {
		t.Fatal("generic op-amp terminal placement produced no candidates")
	}
	for _, placement := range placements {
		bindings := map[string]string{}
		for _, connection := range placement {
			bindings[connection.Terminal] = connection.Node
		}
		if bindings["V_PLUS"] != "supply" || bindings["V_MINUS"] != "ground" || bindings["OUT"] != "output" {
			t.Fatalf("terminal role contract produced invalid placement: %#v", placement)
		}
	}
}

func TestGenericPlacementPreservesDeclaredSupplyOrder(t *testing.T) {
	data := mustRead(t, filepath.Join(architectureGeneralizationCorpusRoot(), "low_level_full_wave_transfer.json"))
	requirement, issues := DecodeStrict(bytes.NewReader(data))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	graph, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("initial graph issues: %#v", graphIssues)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	var opamp PrimitiveCandidate
	for _, primitive := range topologyRepresentatives(requirement, inventory) {
		if primitive.Kind == "opamp" {
			opamp = primitive
			break
		}
	}
	placements := primitivePlacements(requirement, graph, opamp, 256)
	if len(placements) == 0 {
		t.Fatal("dual-supply op-amp produced no valid placement")
	}
	for _, placement := range placements {
		if !validPrimitiveSupplyOrder(requirement, graph, placement) {
			t.Fatalf("placement reverses declared supply order: %#v", placement)
		}
	}
}

func requireAddedNodeID(
	t *testing.T,
	before CandidateGraph,
	after CandidateGraph,
) string {
	t.Helper()
	known := make(map[string]bool, len(before.Nodes))
	for _, node := range before.Nodes {
		known[node.ID] = true
	}
	added := ""
	for _, node := range after.Nodes {
		if known[node.ID] {
			continue
		}
		if added != "" {
			t.Fatalf("AddInternalNode added multiple nodes: %q and %q", added, node.ID)
		}
		added = node.ID
	}
	if len(after.Nodes) != len(before.Nodes)+1 || added == "" {
		t.Fatalf(
			"AddInternalNode node delta = %d with added node %q; want one",
			len(after.Nodes)-len(before.Nodes),
			added,
		)
	}
	return added
}

func testValueSIEqual(actual, expected float64) bool {
	tolerance := math.Max(math.Abs(expected)*1e-12, 1e-18)
	return math.Abs(actual-expected) <= tolerance
}

func TestPowerTransferSeriesFeedbackRequiredForSparseCatalogGap(t *testing.T) {
	minimum, maximum := 18.0, 22.0
	requirement := Requirement{Requirements: Requirements{BehavioralRequirements: []BehavioralAssertion{{
		ID: "gain", Metric: "voltage_gain", Analysis: "ac_sweep", Min: &minimum, Max: &maximum,
	}}}}
	fixedResistor := func(key string, value float64) PrimitiveCandidate {
		return PrimitiveCandidate{
			Key: key, Kind: "resistor",
			ValueDomain: &PrimitiveValueDomain{Kind: "resistance", Unit: "ohm", Minimum: &value, Nominal: &value, Maximum: &value},
			Models:      []PrimitiveModelContract{{AllowedAnalyses: []string{"ac_sweep"}}},
		}
	}
	inventory := map[string]PrimitiveCandidate{
		"10k":  fixedResistor("10k", 10_000),
		"169k": fixedResistor("169k", 169_000),
	}
	if !powerTransferSeriesFeedbackRequired(requirement, inventory) {
		t.Fatal("sparse catalog gap below the bounded gain did not request series feedback composition")
	}
	inventory["190k"] = fixedResistor("190k", 190_000)
	if powerTransferSeriesFeedbackRequired(requirement, inventory) {
		t.Fatal("direct catalog feedback value inside the bounded gain unnecessarily requested composition")
	}
}

func testOpenTopologyRequirement(t *testing.T, file string) Requirement {
	t.Helper()
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, filepath.Join(frozenCorpusRoot(), file))))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	return requirement
}

func testSearchInventory() PrimitiveInventory {
	inventory := testGraphInventory()
	inventory.Hash = strings.Repeat("b", 64)
	inventory.CatalogHash = strings.Repeat("c", 64)
	inventory.ModelRegistryHash = strings.Repeat("d", 64)
	inventory.PrimitiveRegistry = strings.Repeat("e", 64)
	return inventory
}
