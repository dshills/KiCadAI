package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/modelprovenance"
)

func TestFrozenHeldOutCorpusProducesBoundedTopologyCandidates(t *testing.T) {
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 2_000
	policy.MaxGeneratedGraphs = 50_000
	policy.MaxRetainedCandidates = 16
	for _, name := range testHeldOutRequirementNames() {
		t.Run(name, func(t *testing.T) {
			requirement := testOpenTopologyRequirement(t, name)
			result := SearchPrimitiveTopologies(context.Background(), requirement, inventory, policy)
			if os.Getenv("KICADAI_OPEN_TOPOLOGY_DIAGNOSTIC") == "1" {
				t.Logf("inventory primitives=%d search=%#v", len(inventory.Primitives), result.Consumption)
				for index, candidate := range result.Candidates {
					t.Logf("topology[%d] score=%#v %s", index, candidate.Score, testGraphTopologySummary(candidate.Graph))
				}
			}
			if name == "powered_lowpass.json" {
				foundClosedLoopRC := false
				for _, candidate := range result.Candidates {
					if testGraphHasClosedLoopRC(candidate.Graph) {
						foundClosedLoopRC = true
						break
					}
				}
				if !foundClosedLoopRC {
					t.Fatal("bounded generic search omitted the closed-loop active time-constant structure")
				}
			}
			if len(result.Candidates) == 0 {
				t.Fatalf("bounded search produced no candidates: status=%s issues=%#v rejections=%#v", result.Status, result.Issues, result.Rejections)
			}
		})
	}
}

func testGraphTopologySummary(graph CandidateGraph) string {
	instances := make([]string, 0, len(graph.Instances))
	for _, instance := range graph.Instances {
		terminals := make([]string, 0, len(instance.Terminals))
		for _, terminal := range instance.Terminals {
			terminals = append(terminals, terminal.Terminal+"="+terminal.Node)
		}
		instances = append(instances, fmt.Sprintf("%s[%s](%s)", instance.Kind, instance.PrimitiveKey, strings.Join(terminals, ",")))
	}
	return strings.Join(instances, " ")
}

func TestFrozenHeldOutCorpusSimulationPromotion(t *testing.T) {
	if os.Getenv("KICADAI_OPEN_TOPOLOGY_PROMOTION") != "1" {
		t.Skip("set KICADAI_OPEN_TOPOLOGY_PROMOTION=1 to run the bounded held-out synthesis lane")
	}
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 2_000
	policy.MaxGeneratedGraphs = 50_000
	policy.MaxRetainedCandidates = 16
	policy.MaxValueTrials = 256
	policy.MaxTopologyRepairs = 32
	policy.MaxCandidateSimulations = 50_000
	policy.MaxCornerEvaluations = 200_000
	passed := 0
	graphChangingRepairs := 0
	multipleTopologyCases := 0
	selectedAfterFailure := 0
	activeFamilies := map[string]bool{}
	for _, name := range testHeldOutRequirementNames() {
		t.Run(name, func(t *testing.T) {
			requirement := testOpenTopologyRequirement(t, name)
			run := Synthesize(
				context.Background(),
				requirement,
				inventory,
				environment,
				policy,
			)
			replay := Synthesize(
				context.Background(),
				requirement,
				inventory,
				environment,
				policy,
			)
			runJSON, err := json.Marshal(run)
			if err != nil {
				t.Fatal(err)
			}
			replayJSON, err := json.Marshal(replay)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(runJSON, replayJSON) {
				t.Fatal("held-out synthesis replay is not byte-identical")
			}
			assertSynthesisConsumptionMatchesEvidence(t, run)
			t.Logf(
				"status=%s stop=%s selected=%t consumption=%#v diagnostics=%#v",
				run.Report.Status,
				run.Report.StopReason,
				run.Report.Selected != nil,
				run.Report.Consumption,
				run.Report.Diagnostics,
			)
			if os.Getenv("KICADAI_OPEN_TOPOLOGY_DIAGNOSTIC") == "1" {
				bestPenalty := math.Inf(1)
				bestCandidate, bestEvaluation := -1, -1
				for candidateIndex, candidate := range run.Candidates {
					candidatePenalty := math.Inf(1)
					candidateEvaluation := -1
					for evaluationIndex, evaluation := range candidate.Evaluations {
						penalty := simulationEvaluationPenalty(evaluation)
						if penalty < candidatePenalty {
							candidatePenalty = penalty
							candidateEvaluation = evaluationIndex
						}
					}
					bestDiagnoses := []Diagnosis(nil)
					if candidateEvaluation >= 0 {
						bestDiagnoses = candidate.Evaluations[candidateEvaluation].Diagnoses
					}
					t.Logf(
						"candidate=%d score=%#v evaluations=%d best=%g/%d values=%s diagnoses=%#v topology=%s",
						candidateIndex,
						run.Search.Candidates[candidateIndex].Score,
						len(candidate.Evaluations),
						candidatePenalty,
						candidateEvaluation,
						testValueTrialSummary(candidate.ValuePlan, candidateEvaluation),
						bestDiagnoses,
						testGraphTopologySummary(run.Search.Candidates[candidateIndex].Graph),
					)
					if candidateIndex < 5 && len(candidate.Evaluations) == 0 {
						t.Logf(
							"candidate=%d unevaluated topology=%s plan_status=%s rejections=%#v issues=%#v",
							candidateIndex,
							testGraphTopologySummary(run.Search.Candidates[candidateIndex].Graph),
							candidate.ValuePlan.Status,
							candidate.ValuePlan.Rejections,
							candidate.ValuePlan.Issues,
						)
					}
					for evaluationIndex, evaluation := range candidate.Evaluations {
						penalty := simulationEvaluationPenalty(evaluation)
						if penalty < bestPenalty {
							bestPenalty = penalty
							bestCandidate = candidateIndex
							bestEvaluation = evaluationIndex
						}
					}
				}
				if bestCandidate >= 0 {
					candidate := run.Candidates[bestCandidate]
					evaluation := candidate.Evaluations[bestEvaluation]
					trials := EnumerateValueTrials(
						candidate.ValuePlan,
						len(candidate.Evaluations),
					).Trials
					var trial *ValueTrial
					if bestEvaluation < len(trials) {
						trial = &trials[bestEvaluation]
					}
					t.Logf(
						"best candidate=%d evaluation=%d penalty=%g topology=%s trial=%#v diagnoses=%#v",
						bestCandidate,
						bestEvaluation,
						bestPenalty,
						testGraphTopologySummary(run.Search.Candidates[bestCandidate].Graph),
						trial,
						evaluation.Diagnoses,
					)
				}
			}
			if run.Report.Status == StatusPassed {
				passed++
				if run.SelectedGraph != nil {
					for _, instance := range run.SelectedGraph.Instances {
						switch instance.Kind {
						case "opamp":
							activeFamilies["opamp"] = true
						case "comparator":
							activeFamilies["comparator"] = true
						case "npn_bjt", "pnp_bjt":
							activeFamilies["bjt"] = true
						case "n_channel_mosfet", "p_channel_mosfet":
							activeFamilies["mosfet"] = true
						case "fixed_voltage_regulator",
							"adjustable_voltage_regulator":
							activeFamilies["regulator"] = true
						}
					}
				}
				if len(run.Search.Candidates) >= 2 {
					multipleTopologyCases++
				}
				selectedIndex := -1
				for index, candidate := range run.Search.Candidates {
					if run.Report.Selected != nil &&
						candidate.Fingerprint == run.Report.Selected.Fingerprint {
						selectedIndex = index
						break
					}
				}
				if selectedIndex >= 0 {
					attempts := run.Report.Candidates[selectedIndex].Attempts
					for _, attempt := range attempts[:max(0, len(attempts)-1)] {
						if attempt.Status == StatusFailed {
							selectedAfterFailure++
							break
						}
					}
					if run.SelectedRepair != nil &&
						run.SelectedRepair.Selected != nil &&
						run.Report.Selected.TopologyHash !=
							run.Search.Candidates[selectedIndex].TopologyHash {
						graphChangingRepairs++
					}
				}
			}
		})
	}
	if passed < 6 {
		t.Fatalf("held-out synthesis passes = %d, want at least 6 of 8", passed)
	}
	if graphChangingRepairs < 1 {
		t.Fatal("held-out synthesis produced no selected graph-changing repair")
	}
	if multipleTopologyCases < 2 {
		t.Fatalf(
			"held-out synthesis cases with multiple retained topologies = %d, want at least 2",
			multipleTopologyCases,
		)
	}
	if selectedAfterFailure < 2 {
		t.Fatalf(
			"held-out synthesis selections after a failed simulation = %d, want at least 2",
			selectedAfterFailure,
		)
	}
	if len(activeFamilies) < 3 {
		t.Fatalf(
			"held-out passing active-device families = %d, want at least 3: %#v",
			len(activeFamilies),
			activeFamilies,
		)
	}
}

func testValueTrialSummary(plan ValueSearchPlan, index int) string {
	if index < 0 {
		return ""
	}
	trials := EnumerateValueTrials(plan, index+1).Trials
	if index >= len(trials) {
		return ""
	}
	values := []string{}
	for _, selection := range trials[index].Selections {
		value := "fixed"
		if selection.ValueSI != nil {
			value = fmt.Sprintf("%.12g", *selection.ValueSI)
		}
		values = append(values, selection.InstanceID+"="+value+"@"+selection.PrimitiveKey)
	}
	return strings.Join(values, ",")
}

func testHeldOutRequirementNames() []string {
	return []string{
		"adjustable_current_output.json",
		"adjustable_voltage_regulation.json",
		"audio_mute.json",
		"ground_referenced_load_control.json",
		"hysteretic_detector.json",
		"powered_lowpass.json",
		"sensor_conditioner.json",
		"voltage_window_monitor.json",
	}
}

func testHeldOutSynthesisEnvironment(
	t *testing.T,
) (PrimitiveInventory, SimulationEnvironment) {
	t.Helper()
	catalog, err := components.LoadCatalog(
		context.Background(),
		components.LoadOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	registry, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatalf("model provenance diagnostics: %#v", diagnostics)
	}
	catalogHash := circuitgraph.NewResolver(
		circuitgraph.ResolveOptions{Catalog: catalog},
	).CatalogHash()
	inventory, issues := BuildPrimitiveInventory(catalog, catalogHash, registry)
	if len(issues) != 0 {
		t.Fatalf("primitive inventory issues: %#v", issues)
	}
	return inventory, SimulationEnvironment{
		Catalog:       catalog,
		CatalogHash:   catalogHash,
		ModelRegistry: registry,
	}
}

func testGraphHasClosedLoopRC(graph CandidateGraph) bool {
	nodeByID := map[string]GraphNode{}
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	between := func(kind, left, right string) bool {
		for _, instance := range graph.Instances {
			if instance.Kind != kind || len(instance.Terminals) != 2 {
				continue
			}
			first := instance.Terminals[0].Node
			second := instance.Terminals[1].Node
			if (first == left && second == right) ||
				(first == right && second == left) {
				return true
			}
		}
		return false
	}
	for _, instance := range graph.Instances {
		if instance.Kind != "opamp" {
			continue
		}
		nodes := topologyTerminalNodes(instance)
		output := nodes["OUT"]
		positive := nodes["IN_PLUS"]
		negative := nodes["IN_MINUS"]
		if nodeByID[output].Role != "output" {
			continue
		}
		nonInverting :=
			nodeByID[positive].Scope == "internal" &&
				nodeByID[negative].Scope == "internal" &&
				between("resistor", "port_input", positive) &&
				between("capacitor", positive, "port_ground") &&
				between("resistor", negative, output) &&
				between("resistor", negative, "port_ground")
		inverting :=
			nodeByID[negative].Scope == "internal" &&
				nodeByID[positive].Role == "reference" &&
				between("resistor", "port_input", negative) &&
				between("resistor", negative, output) &&
				between("capacitor", negative, output)
		if nonInverting || inverting {
			return true
		}
	}
	return false
}
