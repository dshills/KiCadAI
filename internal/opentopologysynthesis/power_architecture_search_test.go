package opentopologysynthesis

import (
	"bytes"
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/simmodel"
)

func TestBehaviorDrivenPowerTransferGrammarEmitsDistinctArchitectures(t *testing.T) {
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	tests := []struct {
		file                 string
		minimumTopologies    int
		requiredKinds        []string
		minimumArchitectures int
		requiredScaleIDs     []string
	}{
		{
			file:              "continuous_conduction_audio_stage.json",
			minimumTopologies: 2,
			requiredKinds: []string{
				"capacitor", "npn_bjt", "opamp", "reference_diode", "resistor",
			},
			requiredScaleIDs: []string{
				"topology:power_transfer:collector_feedback_bias",
				"topology:power_transfer:emitter_degeneration",
				"topology:power_transfer:input_coupling",
				"topology:power_transfer:output_coupling",
				"topology:power_transfer:standing_current_load",
			},
		},
		{
			file:              "efficient_audio_power_stage.json",
			minimumTopologies: 2,
			requiredKinds: []string{
				"capacitor", "n_channel_mosfet", "npn_bjt", "opamp",
				"p_channel_mosfet", "pnp_bjt", "resistor", "signal_diode",
			},
			minimumArchitectures: 2,
			requiredScaleIDs: []string{
				"topology:power_transfer:bias_chain_lower",
				"topology:power_transfer:feedback_gain",
				"topology:power_transfer:feedback_reference",
				"topology:power_transfer:output_degeneration",
			},
		},
	}
	for _, test := range tests {
		t.Run(strings.TrimSuffix(test.file, ".json"), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(architectureCorpusRoot(), test.file))
			if err != nil {
				t.Fatal(err)
			}
			requirement, issues := DecodeStrict(bytes.NewReader(data))
			if len(issues) != 0 {
				t.Fatalf("decode issues: %#v", issues)
			}
			initial, issues := InitialGraph(requirement)
			if len(issues) != 0 {
				t.Fatalf("initial graph issues: %#v", issues)
			}
			graphHash, err := GraphHash(initial)
			if err != nil {
				t.Fatal(err)
			}
			topologyHash, err := TopologyHash(initial)
			if err != nil {
				t.Fatal(err)
			}
			byKey := primitiveInventoryByKey(inventory)
			policy := DefaultPolicy()
			policy.MaxExpandedStates = 64
			policy.MaxGeneratedGraphs = 512
			candidates, consumption, rejections := topologyPowerTransferRelationshipSeeds(
				context.Background(),
				requirement,
				inventory,
				topologyRepresentatives(requirement, inventory),
				byKey,
				GraphLimits{
					MaxPrimitiveInstances: minPositive(
						policy.MaxPrimitiveInstances,
						requirement.Requirements.Constraints.MaxComponents,
					),
					MaxInternalNodes: policy.MaxInternalNodes,
				},
				policy,
				topologySearchState{
					graph:    initial,
					hash:     graphHash,
					topology: topologyHash,
					score:    scoreTopologyGraph(requirement, initial, byKey, graphHash),
				},
			)
			if len(candidates) < test.minimumTopologies {
				t.Fatalf(
					"power-transfer topologies = %d, want at least %d: consumption=%#v rejections=%#v",
					len(candidates), test.minimumTopologies, consumption, rejections,
				)
			}
			architectures := map[string]bool{}
			allKinds := map[string]bool{}
			analyticScales := map[string]AnalyticScale{}
			readyValuePlans := 0
			for _, candidate := range candidates {
				activeKinds := []string{}
				for _, instance := range candidate.Graph.Instances {
					allKinds[instance.Kind] = true
					if topologyActiveKind(instance.Kind) {
						activeKinds = append(activeKinds, instance.Kind)
					}
				}
				slices.Sort(activeKinds)
				architectures[strings.Join(activeKinds, "+")] = true
				if candidate.Score.BehaviorGap != 0 {
					t.Fatalf("candidate %s behavior gap = %d", candidate.TopologyHash, candidate.Score.BehaviorGap)
				}
				plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
				if plan.Status == ValuePlanReady {
					readyValuePlans++
					for _, domain := range plan.Domains {
						for _, scale := range domain.AnalyticScales {
							analyticScales[scale.ID] = scale
						}
					}
				}
			}
			for _, kind := range test.requiredKinds {
				if !allKinds[kind] {
					t.Errorf("generated candidates lack required primitive kind %s: kinds=%v architectures=%#v", kind, slices.Sorted(maps.Keys(allKinds)), architectures)
				}
			}
			if test.minimumArchitectures != 0 && len(architectures) < test.minimumArchitectures {
				t.Errorf("active architecture signatures = %d, want at least %d: %#v", len(architectures), test.minimumArchitectures, architectures)
			}
			if readyValuePlans == 0 {
				t.Fatal("no generated architecture has a ready value-search plan")
			}
			for _, scaleID := range test.requiredScaleIDs {
				scale, found := analyticScales[scaleID]
				if !found {
					t.Errorf("missing equation-derived analytic scale %s; found=%v", scaleID, slices.Sorted(maps.Keys(analyticScales)))
					continue
				}
				if scale.ValueSI <= 0 || scale.Derivation == "" || scale.SourceKind != "candidate_topology" {
					t.Errorf("invalid equation-derived scale %s: %#v", scaleID, scale)
				}
			}
		})
	}
}

func TestPowerTransferCandidatesReachTrustedSimulation(t *testing.T) {
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	for _, file := range []string{
		"continuous_conduction_audio_stage.json",
		"efficient_audio_power_stage.json",
	} {
		t.Run(strings.TrimSuffix(file, ".json"), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(architectureCorpusRoot(), file))
			if err != nil {
				t.Fatal(err)
			}
			requirement, issues := DecodeStrict(bytes.NewReader(data))
			if len(issues) != 0 {
				t.Fatalf("decode issues: %#v", issues)
			}
			search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, DefaultPolicy())
			if len(search.Candidates) == 0 {
				t.Fatalf("no retained candidates: status=%s issues=%#v rejections=%#v", search.Status, search.Issues, search.Rejections)
			}
			var selected *TopologyCandidate
			for index := range search.Candidates {
				hasNPN := false
				hasPNP := false
				npnCount, pnpCount := 0, 0
				followerEmitters := map[string]bool{}
				for _, instance := range search.Candidates[index].Graph.Instances {
					hasNPN = hasNPN || instance.Kind == "npn_bjt"
					hasPNP = hasPNP || instance.Kind == "pnp_bjt"
					if instance.Kind == "npn_bjt" {
						npnCount++
						terminals := topologyTerminalNodes(instance)
						if terminals["COLLECTOR"] == "port_power" {
							followerEmitters[terminals["EMITTER"]] = true
						}
					}
					if instance.Kind == "pnp_bjt" {
						pnpCount++
					}
				}
				hasFollowerOutput := false
				for _, instance := range search.Candidates[index].Graph.Instances {
					if instance.Kind != "capacitor" || len(instance.Terminals) != 2 {
						continue
					}
					left, right := instance.Terminals[0].Node, instance.Terminals[1].Node
					hasFollowerOutput = hasFollowerOutput ||
						(followerEmitters[left] && right == "port_speaker_output") ||
						(followerEmitters[right] && left == "port_speaker_output")
				}
				if file == "continuous_conduction_audio_stage.json" && !hasFollowerOutput {
					continue
				}
				if !hasNPN || (file == "efficient_audio_power_stage.json" &&
					(!hasPNP || npnCount < 4 || pnpCount < 4)) {
					continue
				}
				if selected == nil ||
					(file == "continuous_conduction_audio_stage.json" &&
						len(search.Candidates[index].Graph.Instances) > len(selected.Graph.Instances)) ||
					(file == "efficient_audio_power_stage.json" &&
						len(search.Candidates[index].Graph.Instances) < len(selected.Graph.Instances)) {
					selected = &search.Candidates[index]
				}
			}
			if selected == nil {
				t.Fatalf("no bipolar power candidate among %d retained topologies: %#v", len(search.Candidates), search.Rejections)
			}
			if file == "continuous_conduction_audio_stage.json" {
				stableReferences, zeners := 0, 0
				byKey := primitiveInventoryByKey(inventory)
				for _, instance := range selected.Graph.Instances {
					primitive := byKey[instance.PrimitiveKey]
					for _, model := range primitive.Models {
						switch model.ModelID {
						case simmodel.PrimitiveShuntVoltageReferenceV1:
							stableReferences++
						case simmodel.PrimitiveUnidirectionalZenerV1:
							zeners++
						}
					}
				}
				if stableReferences == 0 || zeners != 0 {
					t.Fatalf("controlled-current reference models: shunt=%d zener=%d", stableReferences, zeners)
				}
			}
			policy := DefaultPolicy()
			plan := BuildValueSearchPlan(requirement, selected.Graph, inventory, policy)
			if plan.Status != ValuePlanReady {
				t.Fatalf("value plan status=%s issues=%#v rejections=%#v", plan.Status, plan.Issues, plan.Rejections)
			}
			if file == "continuous_conduction_audio_stage.json" {
				requiredScales := map[string]bool{
					"topology:power_transfer:controlled_sink_sense":           false,
					"topology:power_transfer:controlled_sink_reference_bias":  false,
					"topology:power_transfer:controlled_sink_reference_upper": false,
				}
				lowerReferenceScales := 0
				for _, domain := range plan.Domains {
					for _, scale := range domain.AnalyticScales {
						if _, required := requiredScales[scale.ID]; required {
							requiredScales[scale.ID] = true
						}
						if strings.HasPrefix(scale.ID, "topology:power_transfer:controlled_sink_reference_lower_") {
							lowerReferenceScales++
						}
					}
				}
				for scaleID, found := range requiredScales {
					if !found {
						t.Fatalf("controlled-current architecture lacks analytic scale %q", scaleID)
					}
				}
				if lowerReferenceScales == 0 {
					t.Fatal("controlled-current architecture lacks a lower-reference scale")
				}
			}
			trials := EnumerateValueTrials(plan, 1).Trials
			if len(trials) != 1 {
				t.Fatalf("first analytic trial count = %d", len(trials))
			}
			assertValueTrialHasExplainableComponents(t, plan, trials[0])
			appliedGraph, err := ApplyValueTrial(selected.Graph, trials[0], inventory)
			if err != nil {
				t.Fatal(err)
			}
			evaluation := EvaluateCandidate(
				context.Background(), requirement, appliedGraph, nil,
				inventory, environment, policy,
			)
			if len(evaluation.Attempts) == 0 {
				t.Fatalf("trusted simulation produced no attempts: %#v", evaluation)
			}
			for _, attempt := range evaluation.Attempts {
				for _, diagnostic := range attempt.Diagnostics {
					if diagnostic.Code == diagnosisMetricUnsupported {
						t.Errorf("new architecture metric is unsupported: %#v", diagnostic)
					}
				}
			}
			if evaluation.Status != SimulationEvaluationPassed {
				t.Fatalf(
					"%s analytic trial status=%s attempts=%d values=%s diagnoses=%#v graph=%#v",
					file, evaluation.Status, len(evaluation.Attempts), testValueTrialSummary(plan, 0), evaluation.Diagnoses, selected.Graph,
				)
			}
			assertEvaluationCoversRequirementAnalyses(t, requirement, evaluation)
			if file == "continuous_conduction_audio_stage.json" {
				unsafeTrial, found := controlledSinkLowSenseAlternative(plan, trials[0])
				if !found {
					t.Fatal("controlled-current value plan lacks a lower catalog-valid sense-resistor alternative")
				}
				unsafeGraph, err := ApplyValueTrial(selected.Graph, unsafeTrial, inventory)
				if err != nil {
					t.Fatal(err)
				}
				unsafeEvaluation := EvaluateCandidate(
					context.Background(), requirement, unsafeGraph, nil,
					inventory, environment, policy,
				)
				if unsafeEvaluation.Status == SimulationEvaluationPassed {
					t.Fatal("lower catalog-valid current-sense resistance unexpectedly passed the Class A safety envelope")
				}
				safetyRejected := false
				for _, diagnosis := range unsafeEvaluation.Diagnoses {
					switch diagnosis.RequirementID {
					case "standing_current", "safe_temperature", "safe_operating_area":
						safetyRejected = true
					}
				}
				if !safetyRejected {
					t.Fatalf("unsafe Class A alternative was not rejected by bias/thermal/SOA evidence: %#v", unsafeEvaluation.Diagnoses)
				}
			}
			physical := LowerPassingCandidate(
				context.Background(), requirement, appliedGraph, evaluation, inventory, environment,
			)
			if physical.Status != PhysicalLoweringReady || physical.Hash == "" {
				t.Fatalf("%s physical lowering status=%s issues=%#v", file, physical.Status, physical.Issues)
			}
		})
	}
}

func assertEvaluationCoversRequirementAnalyses(t *testing.T, requirement Requirement, evaluation SimulationEvaluation) {
	t.Helper()
	type requirementCase struct {
		requirementID string
		operatingCase string
		analysis      string
	}
	wanted := map[requirementCase]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		for _, operatingCase := range assertion.OperatingCases {
			wanted[requirementCase{
				requirementID: assertion.ID,
				operatingCase: operatingCase,
				analysis:      assertion.Analysis,
			}] = false
		}
	}
	for _, attempt := range evaluation.Attempts {
		key := requirementCase{
			requirementID: attempt.RequirementID,
			operatingCase: attempt.OperatingCase,
			analysis:      attempt.Analysis,
		}
		if _, required := wanted[key]; !required {
			continue
		}
		if attempt.Status != SimulationEvaluationPassed || !attempt.AssertionPass ||
			attempt.Report == nil || strings.TrimSpace(attempt.PlanHash) == "" ||
			strings.TrimSpace(attempt.WorkflowModel) == "" || strings.TrimSpace(attempt.ReportHash) == "" {
			t.Fatalf("required trusted simulation evidence is incomplete for %#v: %#v", key, attempt)
		}
		wanted[key] = true
	}
	for key, covered := range wanted {
		if !covered {
			t.Errorf("required analysis/case was not covered: %#v", key)
		}
	}
}

func assertValueTrialHasExplainableComponents(t *testing.T, plan ValueSearchPlan, trial ValueTrial) {
	t.Helper()
	if len(trial.Selections) == 0 {
		t.Fatal("selected value trial has no component selections")
	}
	for _, selection := range trial.Selections {
		matched := false
		for _, domain := range plan.Domains {
			if domain.InstanceID != selection.InstanceID {
				continue
			}
			for _, candidate := range domain.Candidates {
				if candidate.Hash != selection.CandidateHash {
					continue
				}
				matched = true
				if candidate.PrimitiveKey != selection.PrimitiveKey ||
					candidate.Rank <= 0 || strings.TrimSpace(candidate.Derivation) == "" ||
					strings.TrimSpace(candidate.CatalogEvidence) == "" {
					t.Fatalf("component selection %s lacks explainable catalog/value evidence: %#v", selection.InstanceID, candidate)
				}
				break
			}
		}
		if !matched {
			t.Fatalf("component selection %s candidate %s is not bound to its value plan", selection.InstanceID, selection.CandidateHash)
		}
	}
}

func controlledSinkLowSenseAlternative(plan ValueSearchPlan, baseline ValueTrial) (ValueTrial, bool) {
	trial := ValueTrial{Number: baseline.Number, Hash: baseline.Hash}
	trial.Selections = append([]ValueTrialSelection(nil), baseline.Selections...)
	for _, domain := range plan.Domains {
		isSense := false
		for _, scale := range domain.AnalyticScales {
			if scale.ID == "topology:power_transfer:controlled_sink_sense" {
				isSense = true
				break
			}
		}
		if !isSense {
			continue
		}
		var lowest *ComponentValueCandidate
		for index := range domain.Candidates {
			candidate := &domain.Candidates[index]
			if candidate.ValueSI == nil || *candidate.ValueSI <= 0 ||
				(lowest != nil && *candidate.ValueSI >= *lowest.ValueSI) {
				continue
			}
			lowest = candidate
		}
		if lowest == nil {
			return ValueTrial{}, false
		}
		for index := range trial.Selections {
			if trial.Selections[index].InstanceID != domain.InstanceID {
				continue
			}
			baselineValue := trial.Selections[index].ValueSI
			if baselineValue == nil || *lowest.ValueSI >= *baselineValue {
				return ValueTrial{}, false
			}
			trial.Selections[index] = ValueTrialSelection{
				InstanceID:    domain.InstanceID,
				PrimitiveKey:  lowest.PrimitiveKey,
				ValueSI:       cloneInventoryFloat(lowest.ValueSI),
				CandidateHash: lowest.Hash,
			}
			trial.Hash = hashJSON(trial.Selections)
			return trial, true
		}
	}
	return ValueTrial{}, false
}
