package opentopologysynthesis

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
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
	directFound, bufferedFound := false, false
	activeStructures := map[string]bool{}
	for _, candidate := range candidates {
		counts := map[string]int{}
		for _, instance := range candidate.Graph.Instances {
			counts[instance.Kind]++
		}
		bufferedDrive := counts["npn_bjt"] == 3
		wantInstances, wantNPN, wantResistors := 20, 2, 13
		if bufferedDrive {
			wantInstances, wantNPN, wantResistors = 22, 3, 14
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
			counts["opamp"] != 2 || counts["pnp_bjt"] != 2 || counts["npn_bjt"] != wantNPN ||
			counts["p_channel_mosfet"] != 1 || counts["resistor"] != wantResistors {
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
					want := (9.0 - 1.0) * 40.0 / (2.0 * .084)
					if math.Abs(scale.ValueSI-want) > 1e-12 || len(domain.Candidates) == 0 ||
						domain.Candidates[0].ValueSI == nil || *domain.Candidates[0].ValueSI != 2_000 {
						t.Fatalf("derived high-side base drive scale=%g candidates=%#v", scale.ValueSI, domain.Candidates)
					}
					drive++
				case strings.HasPrefix(scale.ID, "topology:buffered_pass_device_drive:") && scale.ValueSI == 10_000:
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
