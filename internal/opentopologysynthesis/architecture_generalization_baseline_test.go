package opentopologysynthesis

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	measureGeneralizationBaselineEnv = "KICADAI_MEASURE_ARCHITECTURE_GENERALIZATION_BASELINE"
	generalizationBaselineSchema     = "kicadai.architecture-generalization-baseline.v1"
	generalizationBaselineReportHash = "77aea2c19ad22976fd3722a14d5384365c91cf69fee878bfbdfca31cb91229cc"
)

type generalizationMeasuredBaselineCase struct {
	ID                  string                                `json:"id"`
	Kind                string                                `json:"kind"`
	RequirementSHA256   string                                `json:"requirement_sha256"`
	RequirementHash     string                                `json:"requirement_hash"`
	Status              Status                                `json:"status"`
	StopReason          StopReason                            `json:"stop_reason"`
	StoppedAt           string                                `json:"stopped_at"`
	Code                string                                `json:"code"`
	ProviderBypass      bool                                  `json:"provider_bypass"`
	TopologyCandidates  int                                   `json:"topology_candidates"`
	EvaluatedCandidates int                                   `json:"evaluated_candidates"`
	PhysicalReady       int                                   `json:"physical_ready"`
	FailureCounts       []generalizationBaselineFailureCount  `json:"failure_counts"`
	FailureDetails      []generalizationBaselineFailureDetail `json:"failure_details,omitempty"`
	Consumption         Consumption                           `json:"consumption"`
	EvidenceHash        string                                `json:"evidence_hash"`
	ReplayIdentical     bool                                  `json:"replay_identical"`
}

type generalizationBaselineFailureCount struct {
	Stage string `json:"stage"`
	Code  string `json:"code"`
	Count int    `json:"count"`
}

type generalizationBaselineFailureDetail struct {
	RequirementID string    `json:"requirement_id"`
	Code          string    `json:"code"`
	Count         int       `json:"count"`
	Actuals       []float64 `json:"actuals,omitempty"`
	Messages      []string  `json:"messages,omitempty"`
}

type generalizationFrozenBaselineReport struct {
	Schema         string                               `json:"schema"`
	Version        int                                  `json:"version"`
	BaseCommit     string                               `json:"base_commit"`
	ManifestSHA256 string                               `json:"manifest_sha256"`
	MeasuredAt     string                               `json:"measured_at"`
	EngineState    string                               `json:"engine_state"`
	Policy         generalizationFrozenBaselinePolicy   `json:"policy"`
	Summary        generalizationFrozenBaselineSummary  `json:"summary"`
	Cases          []generalizationMeasuredBaselineCase `json:"cases"`
}

type generalizationFrozenBaselinePolicy struct {
	MaxExpandedStates       int `json:"max_expanded_states"`
	MaxGeneratedGraphs      int `json:"max_generated_graphs"`
	MaxRetainedCandidates   int `json:"max_retained_candidates"`
	MaxValueTrials          int `json:"max_value_trials"`
	MaxTopologyRepairs      int `json:"max_topology_repairs"`
	MaxCandidateSimulations int `json:"max_candidate_simulations"`
	MaxCornerEvaluations    int `json:"max_corner_evaluations"`
}

type generalizationFrozenBaselineSummary struct {
	DesignTotal           int `json:"design_total"`
	DesignPasses          int `json:"design_passes"`
	DesignFailClosed      int `json:"design_fail_closed"`
	AdversarialTotal      int `json:"adversarial_total"`
	AdversarialFailClosed int `json:"adversarial_fail_closed"`
	ReplayIdentical       int `json:"replay_identical"`
}

func TestArchitectureGeneralizationUntouchedBaselineIsFrozen(t *testing.T) {
	root := filepath.Join("..", "..", "specs", "architecture-generalization-corpus")
	data := mustRead(t, filepath.Join(root, "BASELINE_REPORT.json"))
	if got := frozenHash(data); got != generalizationBaselineReportHash {
		t.Fatalf("baseline report sha256 = %s, want %s", got, generalizationBaselineReportHash)
	}
	if sidecar := strings.TrimSpace(string(mustRead(t, filepath.Join(root, "BASELINE_REPORT.sha256")))); sidecar != generalizationBaselineReportHash+"  BASELINE_REPORT.json" {
		t.Fatalf("baseline checksum sidecar = %q", sidecar)
	}
	var report generalizationFrozenBaselineReport
	decodeFrozenStrict(t, data, &report)
	if report.Schema != generalizationBaselineSchema || report.Version != 1 ||
		report.BaseCommit != generalizationCorpusBaseCommit ||
		report.ManifestSHA256 != generalizationCorpusManifestHash ||
		report.EngineState != "untouched" || strings.TrimSpace(report.MeasuredAt) == "" {
		t.Fatalf("baseline identity = %#v", report)
	}
	wantPolicy := generalizationFrozenBaselinePolicy{2000, 50000, 16, 64, 16, 4096, 16384}
	if report.Policy != wantPolicy {
		t.Fatalf("baseline policy = %#v, want %#v", report.Policy, wantPolicy)
	}
	wantSummary := generalizationFrozenBaselineSummary{6, 0, 6, 4, 4, 10}
	if report.Summary != wantSummary {
		t.Fatalf("baseline summary = %#v, want %#v", report.Summary, wantSummary)
	}

	var manifest generalizationCorpusManifest
	decodeFrozenStrict(t, mustRead(t, filepath.Join(architectureGeneralizationCorpusRoot(), "manifest.json")), &manifest)
	want := make(map[string]struct{ kind, sha string }, len(report.Cases))
	for _, entry := range manifest.DesignCases {
		want[entry.ID] = struct{ kind, sha string }{"design", entry.RequirementSHA256}
	}
	for _, entry := range manifest.AdversarialCases {
		want[entry.ID] = struct{ kind, sha string }{"adversarial", entry.RequirementSHA256}
	}
	if len(report.Cases) != len(want) {
		t.Fatalf("baseline cases = %d, want %d", len(report.Cases), len(want))
	}
	previousID := ""
	for _, entry := range report.Cases {
		if entry.ID <= previousID {
			t.Fatalf("baseline cases are not strictly sorted: %q after %q", entry.ID, previousID)
		}
		previousID = entry.ID
		expected, ok := want[entry.ID]
		if !ok || entry.Kind != expected.kind || entry.RequirementSHA256 != expected.sha {
			t.Fatalf("baseline case identity %s = %#v", entry.ID, entry)
		}
		delete(want, entry.ID)
		if entry.Status == StatusPassed || !entry.ProviderBypass || entry.PhysicalReady != 0 ||
			!entry.ReplayIdentical || strings.TrimSpace(entry.RequirementHash) == "" ||
			strings.TrimSpace(entry.EvidenceHash) == "" || len(entry.FailureCounts) == 0 {
			t.Fatalf("baseline case did not fail closed deterministically: %#v", entry)
		}
		if !slices.IsSortedFunc(entry.FailureCounts, func(left, right generalizationBaselineFailureCount) int {
			return cmp.Or(cmp.Compare(left.Stage, right.Stage), cmp.Compare(left.Code, right.Code))
		}) {
			t.Fatalf("%s failure counts are not sorted: %#v", entry.ID, entry.FailureCounts)
		}
	}
	if len(want) != 0 {
		t.Fatalf("baseline omitted manifest cases: %v", want)
	}
}

func TestMeasureArchitectureGeneralizationUntouchedBaseline(t *testing.T) {
	if os.Getenv(measureGeneralizationBaselineEnv) != "1" {
		t.Skip("set " + measureGeneralizationBaselineEnv + "=1 to measure the frozen untouched baseline")
	}
	var manifest generalizationCorpusManifest
	decodeFrozenStrict(t, mustRead(t, filepath.Join(architectureGeneralizationCorpusRoot(), "manifest.json")), &manifest)
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 2_000
	policy.MaxGeneratedGraphs = 50_000
	policy.MaxRetainedCandidates = 16
	policy.MaxValueTrials = 64
	policy.MaxTopologyRepairs = 16
	policy.MaxCandidateSimulations = 4_096
	policy.MaxCornerEvaluations = 16_384

	type baselineInput struct {
		kind, id, file, sha string
	}
	inputs := make([]baselineInput, 0, len(manifest.DesignCases)+len(manifest.AdversarialCases))
	for _, entry := range manifest.DesignCases {
		inputs = append(inputs, baselineInput{"design", entry.ID, entry.RequirementFile, entry.RequirementSHA256})
	}
	for _, entry := range manifest.AdversarialCases {
		inputs = append(inputs, baselineInput{"adversarial", entry.ID, entry.RequirementFile, entry.RequirementSHA256})
	}
	for _, input := range inputs {
		input := input
		t.Run(input.id, func(t *testing.T) {
			data := mustRead(t, filepath.Join(architectureGeneralizationCorpusRoot(), input.file))
			requirement, issues := DecodeStrict(bytes.NewReader(data))
			if len(issues) != 0 {
				t.Fatalf("decode issues: %#v", issues)
			}
			first := Synthesize(context.Background(), requirement, inventory, environment, policy)
			second := Synthesize(context.Background(), requirement, inventory, environment, policy)
			firstJSON, _ := json.Marshal(first)
			secondJSON, _ := json.Marshal(second)
			requirementHash, _ := CanonicalHash(requirement)
			evaluated, physicalReady := 0, 0
			for _, candidate := range first.Candidates {
				if len(candidate.Evaluations) != 0 {
					evaluated++
				}
				for _, physical := range candidate.Physical {
					if physical.Status == PhysicalLoweringReady {
						physicalReady++
					}
				}
			}
			if os.Getenv("KICADAI_OPEN_TOPOLOGY_DIAGNOSTIC") == "1" {
				bestPenalty := math.Inf(1)
				bestCandidate, bestEvaluation := -1, -1
				for candidateIndex, candidate := range first.Candidates {
					for evaluationIndex, evaluation := range candidate.Evaluations {
						if penalty := simulationEvaluationPenalty(evaluation); penalty < bestPenalty {
							bestPenalty = penalty
							bestCandidate = candidateIndex
							bestEvaluation = evaluationIndex
						}
					}
				}
				if bestCandidate >= 0 {
					candidate := first.Candidates[bestCandidate]
					if len(candidate.Evaluations) != 0 {
						t.Logf(
							"GENERALIZATION_FIRST values=%s diagnoses=%#v",
							testValueTrialSummary(candidate.ValuePlan, 0),
							candidate.Evaluations[0].Diagnoses,
						)
					}
					t.Logf(
						"GENERALIZATION_BEST candidate=%d evaluation=%d penalty=%g values=%s diagnoses=%#v topology=%s",
						bestCandidate,
						bestEvaluation,
						bestPenalty,
						testValueTrialSummary(candidate.ValuePlan, bestEvaluation),
						candidate.Evaluations[bestEvaluation].Diagnoses,
						testGraphTopologySummary(first.Search.Candidates[bestCandidate].Graph),
					)
				}
			}
			code := ""
			if len(first.Report.Diagnostics) != 0 {
				code = string(first.Report.Diagnostics[0].Code)
			}
			result := generalizationMeasuredBaselineCase{
				ID: input.id, Kind: input.kind,
				RequirementSHA256: input.sha, RequirementHash: requirementHash,
				Status: first.Report.Status, StopReason: first.Report.StopReason,
				StoppedAt: generalizationBaselineStopStage(first), Code: code,
				ProviderBypass: true, TopologyCandidates: len(first.Search.Candidates),
				EvaluatedCandidates: evaluated, PhysicalReady: physicalReady,
				FailureCounts:  generalizationBaselineFailureCounts(first),
				FailureDetails: generalizationBaselineFailureDetails(first),
				Consumption:    first.Report.Consumption, EvidenceHash: first.Hash,
				ReplayIdentical: bytes.Equal(firstJSON, secondJSON),
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("GENERALIZATION_BASELINE %s", encoded)
		})
	}
}

func generalizationBaselineFailureDetails(run SynthesisRun) []generalizationBaselineFailureDetail {
	const maxDiagnosticSamples = 12

	type key struct{ requirementID, code string }
	counts := map[key]int{}
	actuals := map[key][]float64{}
	messages := map[key][]string{}
	for _, candidate := range run.Candidates {
		for _, evaluation := range candidate.Evaluations {
			for _, diagnosis := range evaluation.Diagnoses {
				item := key{diagnosis.RequirementID, diagnosis.Code}
				counts[item]++
				if diagnosis.Actual != nil {
					actuals[item] = append(actuals[item], *diagnosis.Actual)
				}
				if diagnosis.Message != "" {
					messages[item] = append(messages[item], diagnosis.Message)
				}
			}
		}
	}
	result := make([]generalizationBaselineFailureDetail, 0, len(counts))
	for item, count := range counts {
		values := actuals[item]
		slices.Sort(values)
		values = slices.Compact(values)
		if len(values) > maxDiagnosticSamples {
			values = append(slices.Clone(values[:maxDiagnosticSamples/2]), values[len(values)-maxDiagnosticSamples/2:]...)
		}
		itemMessages := messages[item]
		slices.Sort(itemMessages)
		itemMessages = slices.Compact(itemMessages)
		if len(itemMessages) > maxDiagnosticSamples {
			itemMessages = itemMessages[:maxDiagnosticSamples]
		}
		result = append(result, generalizationBaselineFailureDetail{RequirementID: item.requirementID, Code: item.code, Count: count, Actuals: values, Messages: itemMessages})
	}
	slices.SortFunc(result, func(left, right generalizationBaselineFailureDetail) int {
		return cmp.Or(cmp.Compare(left.RequirementID, right.RequirementID), cmp.Compare(left.Code, right.Code))
	})
	return result
}

func generalizationBaselineFailureCounts(run SynthesisRun) []generalizationBaselineFailureCount {
	type key struct{ stage, code string }
	counts := map[key]int{}
	for _, rejection := range run.Search.Rejections {
		counts[key{"architecture_generation", rejection.Code}] += rejection.Count
	}
	for _, candidate := range run.Candidates {
		for _, rejection := range candidate.ValuePlan.Rejections {
			counts[key{"equation_sizing", rejection.Code}] += rejection.Count
		}
		for _, evaluation := range candidate.Evaluations {
			for _, diagnosis := range evaluation.Diagnoses {
				counts[key{"simulation", diagnosis.Code}]++
			}
		}
	}
	result := make([]generalizationBaselineFailureCount, 0, len(counts))
	for item, count := range counts {
		result = append(result, generalizationBaselineFailureCount{Stage: item.stage, Code: item.code, Count: count})
	}
	slices.SortFunc(result, func(left, right generalizationBaselineFailureCount) int {
		return cmp.Or(cmp.Compare(left.Stage, right.Stage), cmp.Compare(left.Code, right.Code))
	})
	return result
}

func generalizationBaselineStopStage(run SynthesisRun) string {
	if len(run.Search.Candidates) == 0 {
		return "architecture_generation"
	}
	hasReadyPlan := false
	hasEvaluation := false
	hasPhysical := false
	for _, candidate := range run.Candidates {
		hasReadyPlan = hasReadyPlan || candidate.ValuePlan.Status == ValuePlanReady
		hasEvaluation = hasEvaluation || len(candidate.Evaluations) != 0
		hasPhysical = hasPhysical || len(candidate.Physical) != 0
	}
	switch {
	case !hasReadyPlan:
		return "equation_sizing"
	case !hasEvaluation:
		return "simulation"
	case !hasPhysical:
		return "simulation"
	case run.Report.Status != StatusPassed:
		return "safety_rejection"
	default:
		return "lowering"
	}
}
