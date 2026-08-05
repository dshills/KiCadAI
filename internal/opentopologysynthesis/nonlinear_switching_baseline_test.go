package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const measureNonlinearSwitchingBaselineEnv = "KICADAI_MEASURE_NONLINEAR_SWITCHING_BASELINE"

const nonlinearSwitchingBaselineReportHash = "c8369ebe5dfcd76f99b75a4230ff82ac7724dc1ce5e134241642db1efdbcac53"

type nonlinearSwitchingBaselineReport struct {
	Schema         string                           `json:"schema"`
	Version        int                              `json:"version"`
	BaseCommit     string                           `json:"base_commit"`
	ManifestSHA256 string                           `json:"manifest_sha256"`
	MeasuredAt     string                           `json:"measured_at"`
	Cases          []nonlinearSwitchingBaselineCase `json:"cases"`
}

type nonlinearSwitchingBaselineCase struct {
	ID                  string      `json:"id"`
	Kind                string      `json:"kind"`
	RequirementSHA256   string      `json:"requirement_sha256"`
	RequirementHash     string      `json:"requirement_hash"`
	Status              Status      `json:"status"`
	StopReason          StopReason  `json:"stop_reason"`
	FirstCode           string      `json:"first_code,omitempty"`
	TopologyCandidates  int         `json:"topology_candidates"`
	EvaluatedCandidates int         `json:"evaluated_candidates"`
	PhysicalReady       int         `json:"physical_ready"`
	Consumption         Consumption `json:"consumption"`
	EvidenceHash        string      `json:"evidence_hash"`
	ReplayIdentical     bool        `json:"replay_identical"`
}

func TestNonlinearSwitchingUntouchedBaselineIsFrozen(t *testing.T) {
	path := filepath.Join("..", "..", "specs", "nonlinear-switching-architecture-synthesis", "BASELINE_REPORT.json")
	data := mustRead(t, path)
	if got := frozenHash(data); got != nonlinearSwitchingBaselineReportHash {
		t.Fatalf("baseline report sha256 = %s, want %s", got, nonlinearSwitchingBaselineReportHash)
	}
	if sidecar := strings.TrimSpace(string(mustRead(t, strings.TrimSuffix(path, ".json")+".sha256"))); sidecar != nonlinearSwitchingBaselineReportHash+"  BASELINE_REPORT.json" {
		t.Fatalf("baseline checksum sidecar = %q", sidecar)
	}
	var report nonlinearSwitchingBaselineReport
	decodeFrozenStrict(t, data, &report)
	if report.Schema != "kicadai.nonlinear-switching-baseline.v1" || report.Version != 1 ||
		report.BaseCommit != nonlinearSwitchingCorpusBaseCommit ||
		report.ManifestSHA256 != nonlinearSwitchingCorpusManifestHash || len(report.Cases) != 8 {
		t.Fatalf("baseline identity = %#v", report)
	}
	want := map[string]Status{
		"autonomous_square_wave_source":                 StatusInvalid,
		"bipolar_magnitude_transfer":                    StatusPassed,
		"bounded_bipolar_transfer":                      StatusExhausted,
		"controlled_pulse_power_stage":                  StatusInvalid,
		"efficient_step_down_power":                     StatusInvalid,
		"adversarial_excessive_controlled_pulse_stress": StatusInvalid,
		"adversarial_excessive_step_down_stress":        StatusInvalid,
		"adversarial_ultrafast_power_conversion":        StatusInvalid,
	}
	for _, entry := range report.Cases {
		if want[entry.ID] != entry.Status || !entry.ReplayIdentical || entry.RequirementHash == "" || entry.EvidenceHash == "" {
			t.Fatalf("baseline case %s = %#v", entry.ID, entry)
		}
		delete(want, entry.ID)
	}
	if len(want) != 0 {
		t.Fatalf("baseline omitted cases %v", want)
	}
}

func TestMeasureNonlinearSwitchingUntouchedBaseline(t *testing.T) {
	if os.Getenv(measureNonlinearSwitchingBaselineEnv) != "1" {
		t.Skip("set " + measureNonlinearSwitchingBaselineEnv + "=1 to measure the frozen untouched baseline")
	}
	var manifest nonlinearSwitchingCorpusManifest
	decodeFrozenStrict(t, mustRead(t, filepath.Join(nonlinearSwitchingCorpusRoot(), "manifest.json")), &manifest)
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 2_000
	policy.MaxGeneratedGraphs = 50_000
	policy.MaxRetainedCandidates = 16
	policy.MaxValueTrials = 64
	policy.MaxTopologyRepairs = 16
	policy.MaxCandidateSimulations = 4_096
	policy.MaxCornerEvaluations = 16_384

	type baselineInput struct{ kind, id, file, sha string }
	inputs := make([]baselineInput, 0, nonlinearSwitchingDesignCaseCount+nonlinearSwitchingAdversarialCount)
	for _, entry := range manifest.DesignCases {
		inputs = append(inputs, baselineInput{"design", entry.ID, entry.RequirementFile, entry.RequirementSHA256})
	}
	for _, entry := range manifest.AdversarialCases {
		inputs = append(inputs, baselineInput{"adversarial", entry.ID, entry.RequirementFile, entry.RequirementSHA256})
	}

	report := nonlinearSwitchingBaselineReport{
		Schema: "kicadai.nonlinear-switching-baseline.v1", Version: 1,
		BaseCommit: nonlinearSwitchingCorpusBaseCommit, ManifestSHA256: nonlinearSwitchingCorpusManifestHash,
		MeasuredAt: "2026-08-05", Cases: make([]nonlinearSwitchingBaselineCase, 0, len(inputs)),
	}
	for _, input := range inputs {
		var requirement Requirement
		decodeFrozenStrict(t, mustRead(t, filepath.Join(nonlinearSwitchingCorpusRoot(), input.file)), &requirement)
		first := Synthesize(context.Background(), requirement, inventory, environment, policy)
		second := Synthesize(context.Background(), requirement, inventory, environment, policy)
		firstJSON, _ := json.Marshal(first)
		secondJSON, _ := json.Marshal(second)
		requirementHash, _ := CanonicalHash(Normalize(requirement))
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
		code := ""
		if len(first.Report.Diagnostics) != 0 {
			code = string(first.Report.Diagnostics[0].Code)
		}
		report.Cases = append(report.Cases, nonlinearSwitchingBaselineCase{
			ID: input.id, Kind: input.kind, RequirementSHA256: input.sha,
			RequirementHash: requirementHash, Status: first.Report.Status, StopReason: first.Report.StopReason,
			FirstCode: code, TopologyCandidates: len(first.Search.Candidates), EvaluatedCandidates: evaluated,
			PhysicalReady: physicalReady, Consumption: first.Report.Consumption,
			EvidenceHash: first.Hash, ReplayIdentical: bytes.Equal(firstJSON, secondJSON),
		})
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("NONLINEAR_SWITCHING_BASELINE %s", encoded)
}
