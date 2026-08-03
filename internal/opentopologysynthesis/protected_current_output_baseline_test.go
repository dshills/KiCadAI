package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	measureProtectedCurrentOutputBaselineEnv = "KICADAI_MEASURE_PROTECTED_CURRENT_OUTPUT_BASELINE"
	protectedCurrentOutputCaseEnv            = "KICADAI_PROTECTED_CURRENT_OUTPUT_CASE"
	protectedCurrentOutputBaselineHash       = "bfb57000d685abbef2a868e23fe0cc1b33846566b182f2ea4771ff294850eea4"
)

type protectedCurrentOutputBaselineReport struct {
	Schema         string                               `json:"schema"`
	Version        int                                  `json:"version"`
	BaseCommit     string                               `json:"base_commit"`
	ManifestSHA256 string                               `json:"manifest_sha256"`
	EngineState    string                               `json:"engine_state"`
	Policy         Policy                               `json:"policy"`
	Cases          []protectedCurrentOutputBaselineCase `json:"cases"`
}

type protectedCurrentOutputBaselineCase struct {
	ID                    string                                    `json:"id"`
	RequirementSHA256     string                                    `json:"requirement_sha256"`
	SynthesisHash         string                                    `json:"synthesis_hash"`
	Status                Status                                    `json:"status"`
	StopReason            StopReason                                `json:"stop_reason"`
	DeepestStage          string                                    `json:"deepest_stage"`
	CandidateCount        int                                       `json:"candidate_count"`
	ReadyValuePlans       int                                       `json:"ready_value_plans"`
	SimulationEvaluations int                                       `json:"simulation_evaluations"`
	PassingEvaluations    int                                       `json:"passing_evaluations"`
	RepairCandidates      int                                       `json:"repair_candidates"`
	PhysicalAttempts      int                                       `json:"physical_attempts"`
	Diagnoses             []protectedCurrentOutputBaselineDiagnosis `json:"diagnoses"`
}

type protectedCurrentOutputDiagnosisKey struct {
	Code          string
	RequirementID string
	Analysis      string
	OperatingCase string
}

type protectedCurrentOutputBaselineDiagnosis struct {
	Code            string `json:"code"`
	RequirementID   string `json:"requirement_id"`
	Analysis        string `json:"analysis"`
	OperatingCorner string `json:"operating_corner"`
	Count           int    `json:"count"`
}

func TestProtectedCurrentOutputBaselineIsFrozenBeforeProductionChanges(t *testing.T) {
	path := filepath.Join("..", "..", "specs", "generic-protected-current-output-synthesis", "BASELINE_REPORT.json")
	data := mustRead(t, path)
	if got := frozenHash(data); got != protectedCurrentOutputBaselineHash {
		t.Fatalf("baseline sha256 = %s, want %s", got, protectedCurrentOutputBaselineHash)
	}
	sidecarPath := filepath.Join(filepath.Dir(path), "BASELINE_REPORT.sha256")
	if sidecar := string(bytes.TrimSpace(mustRead(t, sidecarPath))); sidecar != protectedCurrentOutputBaselineHash+"  BASELINE_REPORT.json" {
		t.Fatalf("baseline checksum sidecar = %q", sidecar)
	}
	var report protectedCurrentOutputBaselineReport
	decodeFrozenStrict(t, data, &report)
	if report.Schema != "kicadai.protected-current-output-baseline.v1" {
		t.Fatalf("baseline schema = %q", report.Schema)
	}
	if report.Version != 1 {
		t.Fatalf("baseline version = %d", report.Version)
	}
	if report.BaseCommit != protectedCurrentOutputCorpusBaseCommit {
		t.Fatalf("baseline base commit = %q", report.BaseCommit)
	}
	if report.ManifestSHA256 != protectedCurrentOutputCorpusManifestHash {
		t.Fatalf("baseline manifest sha256 = %q", report.ManifestSHA256)
	}
	if report.EngineState != "untouched" {
		t.Fatalf("baseline engine state = %q", report.EngineState)
	}
	if report.Policy != protectedCurrentOutputSynthesisPolicy() {
		t.Fatalf("baseline policy = %#v", report.Policy)
	}
	if len(report.Cases) != protectedCurrentOutputCorpusCaseCount {
		t.Fatalf("baseline cases = %d", len(report.Cases))
	}
	previousID := ""
	for _, entry := range report.Cases {
		if entry.ID <= previousID || entry.Status == StatusPassed || entry.StopReason == StopPassed ||
			entry.SynthesisHash == "" || entry.DeepestStage != "simulation" ||
			entry.CandidateCount == 0 || entry.ReadyValuePlans == 0 ||
			entry.SimulationEvaluations == 0 || entry.PassingEvaluations != 0 ||
			entry.PhysicalAttempts != 0 || len(entry.Diagnoses) == 0 {
			t.Fatalf("baseline case = %#v", entry)
		}
		previousID = entry.ID
	}
}

func TestMeasureProtectedCurrentOutputBaseline(t *testing.T) {
	if os.Getenv(measureProtectedCurrentOutputBaselineEnv) != "1" {
		t.Skip("set " + measureProtectedCurrentOutputBaselineEnv + "=1 to measure the frozen pre-implementation baseline")
	}
	root := protectedCurrentOutputCorpusRoot()
	var manifest protectedCurrentOutputCorpusManifest
	decodeFrozenStrict(t, mustRead(t, filepath.Join(root, "manifest.json")), &manifest)
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := protectedCurrentOutputSynthesisPolicy()
	report := protectedCurrentOutputBaselineReport{
		Schema:         "kicadai.protected-current-output-baseline.v1",
		Version:        1,
		BaseCommit:     protectedCurrentOutputCorpusBaseCommit,
		ManifestSHA256: protectedCurrentOutputCorpusManifestHash,
		EngineState:    "untouched",
		Policy:         policy,
	}
	for _, entry := range manifest.Cases {
		if target := os.Getenv(protectedCurrentOutputCaseEnv); target != "" && target != entry.ID {
			continue
		}
		requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, filepath.Clean(filepath.Join(root, entry.RequirementFile)))))
		if len(issues) != 0 {
			t.Fatalf("%s requirement issues: %#v", entry.ID, issues)
		}
		first := Synthesize(context.Background(), requirement, inventory, environment, policy)
		second := Synthesize(context.Background(), requirement, inventory, environment, policy)
		firstJSON, err := json.Marshal(first)
		if err != nil {
			t.Fatal(err)
		}
		secondJSON, err := json.Marshal(second)
		if err != nil {
			t.Fatal(err)
		}
		if first.Hash == "" || first.Hash != second.Hash || !bytes.Equal(firstJSON, secondJSON) {
			t.Fatalf("%s baseline replay differs", entry.ID)
		}
		report.Cases = append(report.Cases, summarizeProtectedCurrentOutputBaseline(entry, first))
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("frozen protected-current-output baseline:\n%s", encoded)
}

func protectedCurrentOutputSynthesisPolicy() Policy {
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 2_000
	policy.MaxGeneratedGraphs = 50_000
	policy.MaxRetainedCandidates = 16
	policy.MaxValueTrials = 64
	policy.MaxTopologyRepairs = 16
	policy.MaxCandidateSimulations = 4_096
	policy.MaxCornerEvaluations = 16_384
	return policy
}

func summarizeProtectedCurrentOutputBaseline(
	entry protectedCurrentOutputCorpusCase,
	run SynthesisRun,
) protectedCurrentOutputBaselineCase {
	result := protectedCurrentOutputBaselineCase{
		ID:                entry.ID,
		RequirementSHA256: entry.RequirementSHA256,
		SynthesisHash:     run.Hash,
		Status:            run.Report.Status,
		StopReason:        run.Report.StopReason,
		CandidateCount:    len(run.Candidates),
		DeepestStage:      "architecture_generation",
	}
	diagnosisCounts := map[protectedCurrentOutputDiagnosisKey]int{}
	for _, candidate := range run.Candidates {
		if candidate.ValuePlan.Status == ValuePlanReady {
			result.ReadyValuePlans++
		}
		result.SimulationEvaluations += len(candidate.Evaluations)
		if len(candidate.Evaluations) != 0 {
			advanceProtectedCurrentOutputStage(&result.DeepestStage, "simulation")
		}
		for _, evaluation := range candidate.Evaluations {
			if evaluation.Status == SimulationEvaluationPassed {
				result.PassingEvaluations++
				advanceProtectedCurrentOutputStage(&result.DeepestStage, "physical_lowering")
			}
			addProtectedCurrentOutputDiagnoses(diagnosisCounts, evaluation.Diagnoses)
		}
		if candidate.Repair != nil {
			result.RepairCandidates++
			for _, attempt := range candidate.Repair.Attempts {
				addProtectedCurrentOutputDiagnoses(diagnosisCounts, attempt.Evaluation.Diagnoses)
			}
		}
		result.PhysicalAttempts += len(candidate.Physical)
	}
	for key, count := range diagnosisCounts {
		result.Diagnoses = append(result.Diagnoses, protectedCurrentOutputBaselineDiagnosis{
			Code:            key.Code,
			RequirementID:   key.RequirementID,
			Analysis:        key.Analysis,
			OperatingCorner: key.OperatingCase,
			Count:           count,
		})
	}
	slices.SortFunc(result.Diagnoses, func(left, right protectedCurrentOutputBaselineDiagnosis) int {
		leftKey := left.Code + "\x00" + left.RequirementID + "\x00" + left.Analysis + "\x00" + left.OperatingCorner + "\x00" + strconv.Itoa(left.Count)
		rightKey := right.Code + "\x00" + right.RequirementID + "\x00" + right.Analysis + "\x00" + right.OperatingCorner + "\x00" + strconv.Itoa(right.Count)
		return strings.Compare(leftKey, rightKey)
	})
	return result
}

func addProtectedCurrentOutputDiagnoses(
	counts map[protectedCurrentOutputDiagnosisKey]int,
	diagnoses []Diagnosis,
) {
	for _, diagnosis := range diagnoses {
		key := protectedCurrentOutputDiagnosisKey{
			Code:          string(diagnosis.Code),
			RequirementID: diagnosis.RequirementID,
			Analysis:      diagnosis.Analysis,
			OperatingCase: diagnosis.OperatingCase,
		}
		counts[key]++
	}
}

func advanceProtectedCurrentOutputStage(current *string, candidate string) {
	rank := func(stage string) int {
		switch stage {
		case "architecture_generation":
			return 1
		case "simulation":
			return 2
		case "physical_lowering":
			return 3
		default:
			panic("unknown protected-current-output synthesis stage: " + stage)
		}
	}
	if rank(candidate) > rank(*current) {
		*current = candidate
	}
}
