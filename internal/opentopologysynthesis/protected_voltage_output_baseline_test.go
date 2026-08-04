package opentopologysynthesis

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

const (
	measureProtectedVoltageOutputBaselineEnv = "KICADAI_MEASURE_PROTECTED_VOLTAGE_OUTPUT_BASELINE"
	protectedVoltageOutputCaseEnv            = "KICADAI_PROTECTED_VOLTAGE_OUTPUT_CASE"
	protectedVoltageOutputBaselineHash       = "6f36bef95dfcd4e961f6be81486b4ad88412388eb52725b02b49a8e6bd205fbf"
	protectedVoltageStageArchitecture        = "architecture_generation"
	protectedVoltageStageValueSearch         = "value_search"
	protectedVoltageStageSimulation          = "simulation"
	protectedVoltageStagePhysicalLowering    = "physical_lowering"
)

type protectedVoltageOutputBaselineReport struct {
	Schema         string                               `json:"schema"`
	Version        int                                  `json:"version"`
	BaseCommit     string                               `json:"base_commit"`
	ManifestSHA256 string                               `json:"manifest_sha256"`
	EngineState    string                               `json:"engine_state"`
	Policy         Policy                               `json:"policy"`
	Cases          []protectedVoltageOutputBaselineCase `json:"cases"`
}

type protectedVoltageOutputBaselineCase struct {
	ID                    string                                    `json:"id"`
	RequirementSHA256     string                                    `json:"requirement_sha256"`
	SynthesisHash         string                                    `json:"synthesis_hash"`
	Status                Status                                    `json:"status"`
	StopReason            StopReason                                `json:"stop_reason"`
	DeepestStage          string                                    `json:"deepest_stage"`
	SearchConsumption     Consumption                               `json:"search_consumption"`
	SearchRejections      []protectedVoltageOutputBaselineRejection `json:"search_rejections"`
	CandidateCount        int                                       `json:"candidate_count"`
	ReadyValuePlans       int                                       `json:"ready_value_plans"`
	SimulationEvaluations int                                       `json:"simulation_evaluations"`
	PassingEvaluations    int                                       `json:"passing_evaluations"`
	RepairCandidates      int                                       `json:"repair_candidates"`
	PhysicalAttempts      int                                       `json:"physical_attempts"`
	Diagnoses             []protectedVoltageOutputBaselineDiagnosis `json:"diagnoses"`
}

type protectedVoltageOutputBaselineRejection struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
}

type protectedVoltageOutputDiagnosisKey struct {
	Code            string
	RequirementID   string
	Analysis        string
	OperatingCorner string
}

type protectedVoltageOutputBaselineDiagnosis struct {
	Code            string `json:"code"`
	RequirementID   string `json:"requirement_id"`
	Analysis        string `json:"analysis"`
	OperatingCorner string `json:"operating_corner"`
	Count           int    `json:"count"`
}

func TestProtectedVoltageOutputBaselineIsFrozenBeforeProductionChanges(t *testing.T) {
	path := filepath.Join("..", "..", "specs", "generic-protected-voltage-output-synthesis", "BASELINE_REPORT.json")
	data := mustRead(t, path)
	if got := frozenHash(data); got != protectedVoltageOutputBaselineHash {
		t.Fatalf("baseline sha256 = %s, want %s", got, protectedVoltageOutputBaselineHash)
	}
	sidecarPath := filepath.Join(filepath.Dir(path), "BASELINE_REPORT.sha256")
	if sidecar := string(bytes.TrimSpace(mustRead(t, sidecarPath))); sidecar != protectedVoltageOutputBaselineHash+"  BASELINE_REPORT.json" {
		t.Fatalf("baseline checksum sidecar = %q", sidecar)
	}
	var report protectedVoltageOutputBaselineReport
	decodeFrozenStrict(t, data, &report)
	if report.Schema != "kicadai.protected-voltage-output-baseline.v1" {
		t.Fatalf("baseline schema = %q", report.Schema)
	}
	if report.Version != 1 {
		t.Fatalf("baseline version = %d", report.Version)
	}
	if report.BaseCommit != protectedVoltageOutputCorpusBaseCommit {
		t.Fatalf("baseline base commit = %q", report.BaseCommit)
	}
	if report.ManifestSHA256 != protectedVoltageOutputCorpusManifestHash {
		t.Fatalf("baseline manifest sha256 = %q", report.ManifestSHA256)
	}
	if report.EngineState != "untouched" {
		t.Fatalf("baseline engine state = %q", report.EngineState)
	}
	if report.Policy != protectedVoltageOutputBaselinePolicy() {
		t.Fatalf("baseline policy = %#v", report.Policy)
	}
	if len(report.Cases) != protectedVoltageOutputCorpusCaseCount {
		t.Fatalf("baseline case count = %d", len(report.Cases))
	}
	previousID := ""
	for _, entry := range report.Cases {
		if entry.ID <= previousID || entry.Status == StatusPassed || entry.StopReason == StopPassed ||
			entry.SynthesisHash == "" || entry.PhysicalAttempts != 0 ||
			(len(entry.SearchRejections) == 0 && len(entry.Diagnoses) == 0) {
			t.Fatalf("baseline case = %#v", entry)
		}
		previousID = entry.ID
	}
}

func protectedVoltageOutputBaselinePolicy() Policy {
	policy := protectedVoltageOutputSynthesisPolicy()
	policy.MaxPrimitiveInstances = 20
	policy.MaxInternalNodes = 24
	return policy
}

func TestMeasureProtectedVoltageOutputBaseline(t *testing.T) {
	if os.Getenv(measureProtectedVoltageOutputBaselineEnv) != "1" {
		t.Skip("set " + measureProtectedVoltageOutputBaselineEnv + "=1 to measure the frozen pre-implementation baseline")
	}
	root := protectedVoltageOutputCorpusRoot()
	var manifest protectedVoltageOutputCorpusManifest
	decodeFrozenStrict(t, mustRead(t, filepath.Join(root, "manifest.json")), &manifest)
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := protectedVoltageOutputSynthesisPolicy()
	report := protectedVoltageOutputBaselineReport{
		Schema:         "kicadai.protected-voltage-output-baseline.v1",
		Version:        1,
		BaseCommit:     protectedVoltageOutputCorpusBaseCommit,
		ManifestSHA256: protectedVoltageOutputCorpusManifestHash,
		EngineState:    "untouched",
		Policy:         policy,
	}
	target := os.Getenv(protectedVoltageOutputCaseEnv)
	matchedTarget := target == ""
	for _, entry := range manifest.Cases {
		if target != "" && target != entry.ID {
			continue
		}
		matchedTarget = true
		requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, filepath.Join(root, entry.RequirementFile))))
		if len(issues) != 0 {
			t.Fatalf("%s requirement issues: %#v", entry.ID, issues)
		}
		first := runProtectedVoltageOutputBaselineSynthesis(t, requirement, inventory, environment, policy)
		second := runProtectedVoltageOutputBaselineSynthesis(t, requirement, inventory, environment, policy)
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
		report.Cases = append(report.Cases, summarizeProtectedVoltageOutputBaseline(t, entry, first))
	}
	if !matchedTarget {
		t.Fatalf("unknown protected voltage-output baseline case %q", target)
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if target != "" {
		t.Logf("case-filtered diagnostic output only; do not replace the full frozen baseline:\n%s", encoded)
		return
	}
	t.Logf("frozen protected-voltage-output baseline:\n%s", encoded)
}

func runProtectedVoltageOutputBaselineSynthesis(
	t *testing.T,
	requirement Requirement,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	policy Policy,
) SynthesisRun {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return Synthesize(ctx, requirement, inventory, environment, policy)
}

func protectedVoltageOutputSynthesisPolicy() Policy {
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

func summarizeProtectedVoltageOutputBaseline(
	t *testing.T,
	entry protectedVoltageOutputCorpusCase,
	run SynthesisRun,
) protectedVoltageOutputBaselineCase {
	t.Helper()
	result := protectedVoltageOutputBaselineCase{
		ID:                entry.ID,
		RequirementSHA256: entry.RequirementSHA256,
		SynthesisHash:     run.Hash,
		Status:            run.Report.Status,
		StopReason:        run.Report.StopReason,
		DeepestStage:      protectedVoltageStageArchitecture,
		SearchConsumption: run.Search.Consumption,
	}
	if len(run.Search.Rejections) != 0 {
		result.SearchRejections = make([]protectedVoltageOutputBaselineRejection, 0, len(run.Search.Rejections))
	}
	for _, rejection := range run.Search.Rejections {
		result.SearchRejections = append(result.SearchRejections, protectedVoltageOutputBaselineRejection{
			Code: rejection.Code, Count: rejection.Count,
		})
	}
	slices.SortFunc(result.SearchRejections, func(left, right protectedVoltageOutputBaselineRejection) int {
		return cmp.Or(cmp.Compare(left.Code, right.Code), cmp.Compare(left.Count, right.Count))
	})
	diagnosisCounts := map[protectedVoltageOutputDiagnosisKey]int{}
	for _, candidate := range run.Candidates {
		if candidate.ValuePlan.Status == ValuePlanReady {
			result.ReadyValuePlans++
			advanceProtectedVoltageOutputStage(t, &result.DeepestStage, protectedVoltageStageValueSearch)
		}
		result.SimulationEvaluations += len(candidate.Evaluations)
		if len(candidate.Evaluations) != 0 {
			advanceProtectedVoltageOutputStage(t, &result.DeepestStage, protectedVoltageStageSimulation)
		}
		for _, evaluation := range candidate.Evaluations {
			if evaluation.Status == SimulationEvaluationPassed {
				result.PassingEvaluations++
				advanceProtectedVoltageOutputStage(t, &result.DeepestStage, protectedVoltageStagePhysicalLowering)
			}
			addProtectedVoltageOutputDiagnoses(diagnosisCounts, evaluation.Diagnoses)
		}
		if candidate.Repair != nil {
			result.RepairCandidates++
			for _, attempt := range candidate.Repair.Attempts {
				addProtectedVoltageOutputDiagnoses(diagnosisCounts, attempt.Evaluation.Diagnoses)
			}
		}
		result.PhysicalAttempts += len(candidate.Physical)
	}
	result.CandidateCount = len(run.Candidates)
	result.Diagnoses = make([]protectedVoltageOutputBaselineDiagnosis, 0, len(diagnosisCounts))
	for key, count := range diagnosisCounts {
		result.Diagnoses = append(result.Diagnoses, protectedVoltageOutputBaselineDiagnosis{
			Code: key.Code, RequirementID: key.RequirementID, Analysis: key.Analysis,
			OperatingCorner: key.OperatingCorner, Count: count,
		})
	}
	slices.SortFunc(result.Diagnoses, func(left, right protectedVoltageOutputBaselineDiagnosis) int {
		return cmp.Or(
			cmp.Compare(left.Code, right.Code),
			cmp.Compare(left.RequirementID, right.RequirementID),
			cmp.Compare(left.Analysis, right.Analysis),
			cmp.Compare(left.OperatingCorner, right.OperatingCorner),
			cmp.Compare(left.Count, right.Count),
		)
	})
	return result
}

func addProtectedVoltageOutputDiagnoses(counts map[protectedVoltageOutputDiagnosisKey]int, diagnoses []Diagnosis) {
	for _, diagnosis := range diagnoses {
		counts[protectedVoltageOutputDiagnosisKey{
			Code: string(diagnosis.Code), RequirementID: diagnosis.RequirementID,
			Analysis: diagnosis.Analysis, OperatingCorner: diagnosis.OperatingCase,
		}]++
	}
}

func advanceProtectedVoltageOutputStage(t *testing.T, current *string, candidate string) {
	t.Helper()
	rank := func(stage string) int {
		switch stage {
		case protectedVoltageStageArchitecture:
			return 1
		case protectedVoltageStageValueSearch:
			return 2
		case protectedVoltageStageSimulation:
			return 3
		case protectedVoltageStagePhysicalLowering:
			return 4
		default:
			t.Fatalf("unknown protected-voltage-output synthesis stage: %s", stage)
			return 0
		}
	}
	if rank(candidate) > rank(*current) {
		*current = candidate
	}
}
