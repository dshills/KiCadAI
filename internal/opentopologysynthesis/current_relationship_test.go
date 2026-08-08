package opentopologysynthesis

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestRegulatedCurrentRelationshipsDeriveIndependentControls(t *testing.T) {
	tests := []struct {
		file       string
		direction  string
		activation string
		fault      string
	}{
		{"fault_protected_low_side_current_sink.json", "sink", "enable", "fault"},
		{"startup_safe_high_side_current_source.json", "source", "permit", "fault"},
		{"../architecture_generalization_corpus/protected_programmable_current_output.json", "source", "", ""},
		{"../multi_stage_ood_corpus/enabled_current_regulation.json", "source", "enable", ""},
	}
	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
				t, filepath.Join(protectedCurrentOutputCorpusRoot(), test.file),
			)))
			if len(issues) != 0 {
				t.Fatalf("requirement decode issues: %#v", issues)
			}
			relationships := regulatedCurrentRelationships(requirement)
			if len(relationships) != 1 {
				t.Fatalf("regulated-current relationships = %#v", relationships)
			}
			got := relationships[0]
			if got.direction != test.direction || got.activation != test.activation || got.fault != test.fault {
				t.Fatalf("regulated-current relationship = %#v", got)
			}
		})
	}
}

func TestHighSideCurrentRelationshipProducesMateriallyDistinctDriveArchitectures(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(architectureGeneralizationCorpusRoot(), "protected_programmable_current_output.json"),
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
	policy.MaxExpandedStates = 16
	policy.MaxGeneratedGraphs = 128
	candidates, consumption, rejections := topologyHighSideTransconductanceRelationshipSeeds(
		context.Background(), requirement, inventory,
		topologyRepresentatives(requirement, inventory), byKey,
		GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes},
		policy, state,
	)
	if len(candidates) != 2 {
		t.Fatalf("high-side current architectures = %d, consumption=%#v rejections=%#v", len(candidates), consumption, rejections)
	}
	activeStructures := map[string]bool{}
	direct, buffered := false, false
	for _, candidate := range candidates {
		activeHash, err := ActiveStructureHash(candidate.Graph)
		if err != nil {
			t.Fatal(err)
		}
		if activeHash == "" || activeStructures[activeHash] {
			t.Fatalf("active structure hash = %q, existing=%v", activeHash, activeStructures)
		}
		activeStructures[activeHash] = true
		npnCount := 0
		for _, instance := range candidate.Graph.Instances {
			if instance.Kind == "npn_bjt" {
				npnCount++
			}
		}
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
		if plan.Status != ValuePlanReady {
			t.Fatalf("current architecture value plan = %#v", plan)
		}
		switch npnCount {
		case 0:
			direct = true
		case 1:
			buffered = true
			scaleFound := false
			for _, domain := range plan.Domains {
				for _, scale := range domain.AnalyticScales {
					scaleFound = scaleFound || strings.HasPrefix(scale.ID, "topology:buffered_pass_device_drive:")
				}
			}
			if !scaleFound {
				t.Fatal("buffered current architecture lacks derived drive-current resistance")
			}
		default:
			t.Fatalf("unexpected buffered-driver count %d", npnCount)
		}
	}
	if !direct || !buffered {
		t.Fatalf("direct/buffered current architectures = %t/%t", direct, buffered)
	}
}

func TestHighSideCurrentRelationshipSupportsActivationWithoutIndependentFault(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), "enabled_current_regulation.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	composition, found := currentSenseDifferentialComposition(
		context.Background(), requirement, inventory, 1/requirementTransconductance(requirement),
	)
	if !found || math.Abs(composition.effectiveResistance-2) > 1e-12 ||
		composition.shuntCount != 1 || composition.shuntResistance != 0.01 ||
		!strings.Contains(composition.shunt.Key, "wslt2512") {
		t.Fatalf("activation-only current-sense composition = %#v found=%t maximum_shunt=%.12g",
			composition, found, regulatedCurrentMaximumShuntResistance(requirement))
	}
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
	candidates, consumption, rejections := topologyHighSideTransconductanceRelationshipSeeds(
		context.Background(), requirement, inventory,
		topologyRepresentatives(requirement, inventory), byKey,
		GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes},
		policy, state,
	)
	if len(candidates) == 0 {
		t.Fatalf("activation-only high-side current relationship produced no candidates: consumption=%#v rejections=%#v", consumption, rejections)
	}
	for _, candidate := range candidates {
		counts := map[string]int{}
		passPrimitive := PrimitiveCandidate{}
		passEmitter := ""
		switchedSupply := ""
		for _, instance := range candidate.Graph.Instances {
			counts[instance.Kind]++
			if instance.Kind == "pnp_bjt" {
				passPrimitive = byKey[instance.PrimitiveKey]
				passEmitter = topologyTerminalNodes(instance)["EMITTER"]
			}
			if instance.Kind == "p_channel_mosfet" {
				switchedSupply = topologyTerminalNodes(instance)["DRAIN"]
			}
		}
		if candidate.Score.BehaviorGap != 0 || counts["p_channel_mosfet"] != 1 ||
			counts["npn_bjt"] == 0 || counts["pnp_bjt"] == 0 || counts["opamp"] < 2 {
			t.Fatalf("activation-only high-side graph score=%#v counts=%v topology=%s", candidate.Score, counts, testGraphTopologySummary(candidate.Graph))
		}
		if want := topologyHighSidePassDeviceCount(requirement, passPrimitive); counts["pnp_bjt"] != want {
			t.Fatalf("activation-only parallel pass count=%d, want thermal-envelope count %d", counts["pnp_bjt"], want)
		} else if want == 1 && passEmitter != switchedSupply {
			t.Fatalf("single pass device retains an unnecessary sharing ballast: emitter=%s switched_supply=%s", passEmitter, switchedSupply)
		}
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
		if plan.Status != ValuePlanReady {
			t.Fatalf("activation-only high-side value plan = %#v", plan)
		}
		instanceValues := map[string]float64{}
		for _, instance := range candidate.Graph.Instances {
			if instance.ValueSI != nil {
				instanceValues[instance.ID] = *instance.ValueSI
			}
		}
		differentialScales, bufferedDriveScales := 0, 0
		for _, domain := range plan.Domains {
			for _, scale := range domain.AnalyticScales {
				switch {
				case strings.HasPrefix(scale.ID, "topology:differential_observation:"):
					instanceID := strings.TrimPrefix(scale.ID, "topology:differential_observation:")
					if scale.ValueSI != instanceValues[instanceID] {
						t.Fatalf("differential scale %s=%g, selected graph value=%g", scale.ID, scale.ValueSI, instanceValues[instanceID])
					}
					differentialScales++
				case strings.HasPrefix(scale.ID, "topology:buffered_pass_device_drive:"):
					if scale.ValueSI <= 0 || scale.ValueSI >= 10_000 || len(domain.Candidates) == 0 ||
						domain.Candidates[0].ValueSI == nil || *domain.Candidates[0].ValueSI >= 10_000 {
						t.Fatalf("activation-only buffered drive scale=%g candidates=%#v", scale.ValueSI, domain.Candidates)
					}
					bufferedDriveScales++
				}
			}
		}
		if differentialScales < 4 || bufferedDriveScales != 1 {
			t.Fatalf("activation-only differential/buffered scales = %d/%d", differentialScales, bufferedDriveScales)
		}
	}
}

func TestHighSidePassDeviceCountRejectsThermallyUnboundedParallelBank(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), "enabled_current_regulation.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	maximumJunction := 70.001
	for index := range requirement.Requirements.BehavioralRequirements {
		assertion := &requirement.Requirements.BehavioralRequirements[index]
		if assertion.Metric == "junction_temperature" {
			assertion.Max = &maximumJunction
		}
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	for _, primitive := range inventory.Primitives {
		if primitive.Kind == "pnp_bjt" && primitiveHasThermalEvidence(primitive) {
			if got := topologyHighSidePassDeviceCount(requirement, primitive); got != 0 {
				t.Fatalf("thermally infeasible parallel pass count = %d, want fail-closed zero", got)
			}
			return
		}
	}
	t.Fatal("test inventory contains no thermal PNP pass device")
}

func TestLowSideTransconductanceRelationshipBuildsRegulatedProtectedPath(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(protectedCurrentOutputCorpusRoot(), "fault_protected_low_side_current_sink.json"),
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
	composition := currentSenseSeriesComposition(context.Background(), requirement, inventory, 1/requirementTransconductance(requirement), 4)
	if len(composition) != 2 || *composition[0].ValueDomain.Nominal != 10 ||
		*composition[1].ValueDomain.Nominal != 10 {
		t.Fatalf("current-sense series composition = %#v, want 10+10 ohm", composition)
	}
	state := topologySearchState{
		graph: initial, hash: hash, topology: topology,
		score: scoreTopologyGraph(requirement, initial, byKey, hash),
	}
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 32
	policy.MaxGeneratedGraphs = 256
	candidates, consumption, rejections := topologyLowSideTransconductanceRelationshipSeeds(
		context.Background(), requirement, inventory,
		topologyRepresentatives(requirement, inventory), byKey,
		GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes},
		policy, state,
	)
	if len(candidates) == 0 {
		t.Fatalf("low-side regulated-current relationship produced no candidates: consumption=%#v rejections=%#v", consumption, rejections)
	}
	for _, candidate := range candidates {
		counts := map[string]int{}
		for _, instance := range candidate.Graph.Instances {
			counts[instance.Kind]++
			if instance.Kind == "n_channel_mosfet" {
				minimumHigh, maximumHigh, found := requirementControlHighVoltageRange(requirement, "enable")
				if !found || !primitiveHasClampedMOSFETGateDrive(byKey[instance.PrimitiveKey], minimumHigh, maximumHigh) {
					t.Fatalf("enable switch lacks guaranteed gate-drive margin: %s", instance.PrimitiveKey)
				}
			}
		}
		if candidate.Score.BehaviorGap != 0 || counts["opamp"] != 1 || counts["npn_bjt"] != 2 ||
			counts["pnp_bjt"] != 1 || counts["n_channel_mosfet"] != 1 || counts["resistor"] != 7 {
			t.Fatalf("low-side regulated-current graph score=%#v counts=%v topology=%s",
				candidate.Score, counts, testGraphTopologySummary(candidate.Graph))
		}
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
		if plan.Status != ValuePlanReady {
			t.Fatalf("low-side value plan = %#v", plan)
		}
		senseScales, controlScales := 0, 0
		for _, domain := range plan.Domains {
			for _, scale := range domain.AnalyticScales {
				switch {
				case strings.HasPrefix(scale.ID, "topology:low_side_current_sense:") && scale.ValueSI == 10:
					senseScales++
				case strings.HasPrefix(scale.ID, "topology:protected_current_control:") && scale.ValueSI == 10_000:
					controlScales++
				}
			}
		}
		if senseScales != 2 || controlScales != 5 {
			t.Fatalf("role-aware current value scales: sense=%d control=%d", senseScales, controlScales)
		}
	}
}

func TestCurrentLimitedSwitchRelationshipBuildsFeedbackProtectedPath(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), "inductive_load_current_control.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	relationships := currentLimitedSwitchRelationships(requirement)
	if len(relationships) != 1 || relationships[0].control != "pulse_command" ||
		relationships[0].output != "load_current" || math.Abs(relationships[0].targetCurrent-.955) > 1e-12 ||
		relationships[0].onVoltageLimit != .5 {
		t.Fatalf("current-limited relationships = %#v", relationships)
	}
	inventory, environment := testHeldOutSynthesisEnvironment(t)
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
	candidates, consumption, rejections := topologyCurrentLimitedSwitchRelationshipSeeds(
		context.Background(), requirement, inventory,
		topologyRepresentatives(requirement, inventory), byKey,
		GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes},
		policy, state,
	)
	if len(candidates) == 0 {
		t.Fatalf("current-limited switch produced no candidates: consumption=%#v rejections=%#v", consumption, rejections)
	}
	for _, candidate := range candidates {
		counts := map[string]int{}
		gateDecisions := map[string]int{}
		for _, instance := range candidate.Graph.Instances {
			counts[instance.Kind]++
			if instance.Kind == "comparator" {
				gateDecisions[topologyTerminalNodes(instance)["OUT"]]++
			}
		}
		sharedGate := false
		for _, count := range gateDecisions {
			sharedGate = sharedGate || count == 2
		}
		if candidate.Score.BehaviorGap != 0 || counts["comparator"] != 2 ||
			counts["n_channel_mosfet"] != 1 || counts["resistor"] != 13 ||
			counts["signal_diode"] != 1 || !sharedGate {
			t.Fatalf("current-limited graph score=%#v counts=%v topology=%s",
				candidate.Score, counts, testGraphTopologySummary(candidate.Graph))
		}
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
		if plan.Status != ValuePlanReady {
			t.Fatalf("current-limited value plan = %#v", plan)
		}
		roleScales, hysteresisScales, shuntScale := 0, 0, false
		for _, domain := range plan.Domains {
			for _, scale := range domain.AnalyticScales {
				if strings.HasPrefix(scale.ID, "topology:current_limited_switch_role:") {
					roleScales++
					shuntScale = shuntScale || scale.ValueSI == .22
				}
				if strings.HasPrefix(scale.ID, "topology:current_limited_switch_hysteresis:") {
					hysteresisScales++
				}
			}
		}
		if roleScales != 6 || hysteresisScales != 5 || !shuntScale {
			t.Fatalf("current-limited analytic scales: roles=%d hysteresis=%d shunt=%t plan=%#v",
				roleScales, hysteresisScales, shuntScale, plan)
		}
		enumeration := EnumerateValueTrials(plan, 1)
		if len(enumeration.Trials) != 1 {
			t.Fatalf("current-limited first value trial = %#v", enumeration)
		}
		applied, applyErr := ApplyValueTrial(candidate.Graph, enumeration.Trials[0], inventory)
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		for _, assertion := range requirement.Requirements.BehavioralRequirements {
			for _, operatingCase := range requirement.Requirements.OperatingCases {
				if !slices.Contains(assertion.OperatingCases, operatingCase.ID) {
					continue
				}
				for _, corner := range operatingCaseCorners(operatingCase) {
					attempt, diagnoses := evaluateAssertionCorner(
						requirement,
						assertion,
						operatingCase,
						corner,
						applied,
						inventory,
						environment,
					)
					if len(diagnoses) != 0 {
						t.Fatalf("current-limited first-trial %s %s/%s attempt=%#v diagnoses=%#v",
							assertion.ID, operatingCase.ID, corner.ID, attempt, diagnoses)
					}
				}
			}
		}
	}
}

func TestCurrentLimitedSwitchRelationshipIsRetainedByDefaultSearch(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), "inductive_load_current_control.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	search := SearchPrimitiveTopologies(
		context.Background(), requirement, inventory, DefaultPolicy(),
	)
	if search.Status != TopologySearchCandidates {
		t.Fatalf("default search status=%s issues=%#v rejections=%#v", search.Status, search.Issues, search.Rejections)
	}
	regulatorIndex := -1
	for index, candidate := range search.Candidates {
		output := externalRelationshipNode(candidate.Graph, "load_current")
		references := topologyNodesByRole(candidate.Graph, "reference")
		if output != "" && len(references) != 0 &&
			topologyGraphHasLowSideCurrentRegulation(candidate.Graph, output, references[0]) {
			regulatorIndex = index
			break
		}
	}
	if regulatorIndex < 0 {
		t.Fatalf("default search omitted behavior-compatible current regulation: candidates=%d", len(search.Candidates))
	}
	order := synthesisCandidateEvaluationOrder(requirement, inventory, search.Candidates)
	if len(order) == 0 || order[0] != regulatorIndex {
		t.Fatalf("behavior-compatible current regulator evaluated at index %d through order %v", regulatorIndex, order)
	}
}

func TestDefaultSearchCurrentLimitedFirstTrialIsPhysicallyReady(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), "inductive_load_current_control.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := DefaultPolicy()
	search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, policy)
	order := synthesisCandidateEvaluationOrder(requirement, inventory, search.Candidates)
	if search.Status != TopologySearchCandidates || len(order) == 0 {
		t.Fatalf("default search status=%s candidates=%d issues=%#v", search.Status, len(search.Candidates), search.Issues)
	}
	candidate := search.Candidates[order[0]]
	plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
	enumeration := EnumerateValueTrials(plan, 1)
	if plan.Status != ValuePlanReady || len(enumeration.Trials) != 1 {
		t.Fatalf("first candidate value plan=%#v enumeration=%#v", plan, enumeration)
	}
	graph, err := ApplyValueTrial(candidate.Graph, enumeration.Trials[0], inventory)
	if err != nil {
		t.Fatal(err)
	}
	evaluation := EvaluateCandidate(
		context.Background(), requirement, graph, nil, inventory, environment, policy,
	)
	if evaluation.Status != SimulationEvaluationPassed {
		t.Fatalf("default-search first trial status=%s consumption=%#v issues=%#v diagnoses=%#v",
			evaluation.Status, evaluation.Consumption, evaluation.Issues, evaluation.Diagnoses)
	}
	physical := LowerPassingCandidate(
		context.Background(), requirement, graph, evaluation, inventory, environment,
	)
	if physical.Status != PhysicalLoweringReady {
		t.Fatalf("default-search first trial physical status=%s issues=%#v", physical.Status, physical.Issues)
	}
}

func TestCurrentSenseCompositionHonorsCancellation(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(protectedCurrentOutputCorpusRoot(), "fault_protected_low_side_current_sink.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if composition := currentSenseSeriesComposition(ctx, requirement, inventory, 20, 4); len(composition) != 0 {
		t.Fatalf("canceled series composition = %#v, want none", composition)
	}
	if composition, found := currentSenseDifferentialComposition(ctx, requirement, inventory, 20); found {
		t.Fatalf("canceled differential composition = %#v, want none", composition)
	}
	for name, target := range map[string]float64{"zero": 0, "infinite": math.Inf(1), "nan": math.NaN()} {
		t.Run(name, func(t *testing.T) {
			if composition := currentSenseSeriesComposition(context.Background(), requirement, inventory, target, 4); len(composition) != 0 {
				t.Fatalf("invalid-target series composition = %#v, want none", composition)
			}
			if composition, found := currentSenseDifferentialComposition(context.Background(), requirement, inventory, target); found {
				t.Fatalf("invalid-target differential composition = %#v, want none", composition)
			}
		})
	}
}

func TestCurrentSenseSeriesParallelCompositionRealizesBoundedIntermediateValue(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(protectedCurrentOutputCorpusRoot(), "fault_protected_low_side_current_sink.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	network, found := currentSenseSeriesParallelComposition(context.Background(), requirement, inventory, .33, 3)
	partCount := 0
	parallelSegment := false
	for _, segment := range network.segments {
		partCount += len(segment)
		parallelSegment = parallelSegment || len(segment) > 1
	}
	if !found || math.Abs(network.effectiveResistance-.33) > 1e-12 || partCount != 3 || !parallelSegment {
		t.Fatalf("series-parallel current-sense network = %#v found=%t", network, found)
	}
}

func TestCurrentSenseSeriesParallelCompositionPrefersSafeSideOfTarget(t *testing.T) {
	inventory := PrimitiveInventory{Primitives: []PrimitiveCandidate{
		{Key: "resistor.9", Kind: "resistor", ValueDomain: &PrimitiveValueDomain{Nominal: floatPointer(9)}},
		{Key: "resistor.12", Kind: "resistor", ValueDomain: &PrimitiveValueDomain{Nominal: floatPointer(12)}},
	}}
	network, found := currentSenseSeriesParallelCompositionWithin(
		context.Background(), Requirement{}, inventory, 10, 1, 8, 15,
	)
	if !found || network.effectiveResistance != 12 {
		t.Fatalf("safe-side current-sense network = %#v found=%t", network, found)
	}
}

func TestHighSideTransconductanceRelationshipBuildsStartupSafeFaultDominantPath(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(protectedCurrentOutputCorpusRoot(), "startup_safe_high_side_current_source.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	composition, found := currentSenseDifferentialComposition(
		context.Background(), requirement, inventory, 1/requirementTransconductance(requirement),
	)
	if !found || math.Abs(composition.effectiveResistance-12.5) > 1e-12 ||
		composition.shuntCount != 1 || composition.shuntResistance != 10 ||
		composition.inputCount != 1 || composition.feedbackCount != 1 ||
		*composition.shunt.ValueDomain.Nominal != 10 ||
		*composition.input.ValueDomain.Nominal != 10_000 ||
		*composition.feedback.ValueDomain.Nominal != 12_500 {
		t.Fatalf("high-side current-sense differential composition = %#v", composition)
	}
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
	policy.MaxGeneratedGraphs = 512
	candidates, consumption, rejections := topologyHighSideTransconductanceRelationshipSeeds(
		context.Background(), requirement, inventory,
		topologyRepresentatives(requirement, inventory), byKey,
		GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes},
		policy, state,
	)
	if len(candidates) != 2 {
		t.Fatalf("high-side regulated-current relationship produced no candidates: consumption=%#v rejections=%#v", consumption, rejections)
	}
	relationships := regulatedCurrentRelationships(requirement)
	if len(relationships) != 1 {
		t.Fatalf("high-side regulated-current relationships = %#v", relationships)
	}
	inputConditioning := transconductanceInputConditioningRequired(requirement, relationships[0])
	directFound, bufferedFound := false, false
	activeStructures := map[string]bool{}
	for _, candidate := range candidates {
		counts := map[string]int{}
		passPrimitive := PrimitiveCandidate{}
		for _, instance := range candidate.Graph.Instances {
			counts[instance.Kind]++
			if instance.Kind == "pnp_bjt" {
				passPrimitive = byKey[instance.PrimitiveKey]
			}
		}
		bufferedDrive := counts["npn_bjt"] == 3
		wantPassDevices := topologyHighSidePassDeviceCount(requirement, passPrimitive)
		wantResistors := 11
		wantCapacitors := 0
		if wantPassDevices > 1 {
			wantResistors += wantPassDevices
		}
		wantInstances, wantNPN := 5+wantPassDevices+wantResistors, 2
		if inputConditioning {
			wantInstances += 2
			wantResistors++
			wantCapacitors = 1
		}
		if bufferedDrive {
			wantInstances, wantNPN, wantResistors = wantInstances+2, 3, wantResistors+1
			bufferedFound = true
		} else {
			directFound = true
		}
		activeHash, err := ActiveStructureHash(candidate.Graph)
		if err != nil || activeHash == "" || activeStructures[activeHash] {
			t.Fatalf("high-side active structure = %q err=%v existing=%v", activeHash, err, activeStructures)
		}
		activeStructures[activeHash] = true
		if candidate.Score.BehaviorGap != 0 || len(candidate.Graph.Instances) != wantInstances ||
			counts["opamp"] != 2 || counts["pnp_bjt"] != wantPassDevices || counts["npn_bjt"] != wantNPN ||
			counts["p_channel_mosfet"] != 1 || counts["resistor"] != wantResistors ||
			counts["capacitor"] != wantCapacitors {
			t.Fatalf("high-side regulated-current graph score=%#v counts=%v topology=%s",
				candidate.Score, counts, testGraphTopologySummary(candidate.Graph))
		}
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
		if plan.Status != ValuePlanReady {
			t.Fatalf("high-side value plan = %#v", plan)
		}
		sense, differentialInput, differentialFeedback := 0, 0, 0
		drive, bufferedDriveScale, bias, control := 0, 0, 0, 0
		for _, domain := range plan.Domains {
			for _, scale := range domain.AnalyticScales {
				switch {
				case strings.HasPrefix(scale.ID, "topology:current_sense_impedance:") && scale.ValueSI == 10:
					sense++
				case strings.HasPrefix(scale.ID, "topology:differential_observation:") && scale.ValueSI == 10_000:
					differentialInput++
				case strings.HasPrefix(scale.ID, "topology:differential_observation:") && scale.ValueSI == 12_500:
					differentialFeedback++
				case strings.HasPrefix(scale.ID, "topology:pass_device_drive:"):
					want := (minimumTransconductanceSupplyVoltage(requirement) - 1.0) *
						primitiveMinimumForwardBeta(passPrimitive) /
						(2.0 * requiredTransconductanceOutputCurrent(requirement))
					if math.Abs(scale.ValueSI-want) > 1e-12 || len(domain.Candidates) == 0 ||
						domain.Candidates[0].ValueSI == nil || *domain.Candidates[0].ValueSI <= 0 {
						t.Fatalf("derived high-side base drive scale=%g candidates=%#v", scale.ValueSI, domain.Candidates)
					}
					drive++
				case strings.HasPrefix(scale.ID, "topology:buffered_pass_device_drive:"):
					if scale.ValueSI <= 0 || len(domain.Candidates) == 0 ||
						domain.Candidates[0].ValueSI == nil || *domain.Candidates[0].ValueSI <= 0 {
						t.Fatalf("derived buffered high-side drive scale=%g candidates=%#v", scale.ValueSI, domain.Candidates)
					}
					bufferedDriveScale++
				case strings.HasPrefix(scale.ID, "topology:pass_device_bias:") && scale.ValueSI == 10_000:
					bias++
				case strings.HasPrefix(scale.ID, "topology:protected_current_control:") && scale.ValueSI == 10_000:
					control++
				}
			}
		}
		wantDrive, wantBufferedDrive, wantBias := 1, 0, 0
		if bufferedDrive {
			wantDrive, wantBufferedDrive, wantBias = 0, 1, 1
		}
		if sense != 1 || differentialInput != 2 || differentialFeedback != 2 ||
			drive != wantDrive || bufferedDriveScale != wantBufferedDrive || bias != wantBias || control != 5 {
			t.Fatalf("role-aware high-side scales: sense=%d input=%d feedback=%d drive=%d buffered=%d bias=%d control=%d",
				sense, differentialInput, differentialFeedback, drive, bufferedDriveScale, bias, control)
		}
		if raw := os.Getenv("KICADAI_OPEN_TOPOLOGY_DIAGNOSTIC_TRIALS"); raw != "" {
			maximum, err := strconv.Atoi(raw)
			if err != nil || maximum <= 0 {
				t.Fatalf("invalid KICADAI_OPEN_TOPOLOGY_DIAGNOSTIC_TRIALS=%q", raw)
			}
			trials := EnumerateValueTrials(plan, maximum)
			assertion := requirement.Requirements.BehavioralRequirements[0]
			operatingCase := OperatingCase{}
			for _, candidateCase := range requirement.Requirements.OperatingCases {
				if len(assertion.OperatingCases) != 0 && candidateCase.ID == assertion.OperatingCases[0] {
					operatingCase = candidateCase
					break
				}
			}
			if operatingCase.ID == "" {
				t.Fatal("command-accuracy operating case not found")
			}
			corner := operatingCaseCorners(operatingCase)[0]
			for index, trial := range trials.Trials {
				trialGraph, err := ApplyValueTrial(candidate.Graph, trial, inventory)
				if err != nil {
					t.Fatal(err)
				}
				attempt, diagnoses := evaluateAssertionCorner(
					requirement, assertion, operatingCase, corner, trialGraph, inventory, environment,
				)
				actual := "missing"
				if attempt.Actual != nil {
					actual = strconv.FormatFloat(*attempt.Actual, 'g', -1, 64)
				}
				t.Logf("trial=%d actual=%s status=%s values=%s diagnoses=%#v",
					index, actual, attempt.Status, testValueTrialSummary(plan, index), diagnoses)
				if target := os.Getenv("KICADAI_OPEN_TOPOLOGY_DIAGNOSTIC_FULL_TRIAL"); target == strconv.Itoa(index) {
					evaluation := EvaluateCandidate(
						context.Background(), requirement, trialGraph, nil, inventory, environment, DefaultPolicy(),
					)
					t.Logf("full trial=%d status=%s attempts=%#v diagnoses=%#v issues=%#v",
						index, evaluation.Status, evaluation.Attempts, evaluation.Diagnoses, evaluation.Issues)
				}
			}
		}
	}
	if !directFound || !bufferedFound {
		t.Fatalf("high-side direct/buffered architectures = %t/%t", directFound, bufferedFound)
	}
}
