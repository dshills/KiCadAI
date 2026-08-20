package closedloopopensetcontract

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

type v19GenerationZeroRetirement struct {
	Schema                       string         `json:"schema"`
	Version                      int            `json:"version"`
	Stage                        string         `json:"stage"`
	SourceCommit                 string         `json:"source_commit"`
	EvaluatorFreezeCommit        string         `json:"evaluator_freeze_commit"`
	ContractManifestSHA256       string         `json:"contract_manifest_sha256"`
	EvaluatorManifestSHA256      string         `json:"evaluator_manifest_sha256"`
	CorpusManifestSHA256         string         `json:"corpus_manifest_sha256"`
	CorpusChecksumsSHA256        string         `json:"corpus_checksums_sha256"`
	EnvironmentSHA256            string         `json:"environment_sha256"`
	GenerationReport             string         `json:"generation_report"`
	GenerationReportSHA256       string         `json:"generation_report_sha256"`
	GenerationHash               string         `json:"generation_hash"`
	V18BaselineReport            string         `json:"v18_baseline_report"`
	V18BaselineReportSHA256      string         `json:"v18_baseline_report_sha256"`
	V18BaselineHash              string         `json:"v18_baseline_hash"`
	RequiredCaseCount            int            `json:"required_case_count"`
	CompletedCaseCount           int            `json:"completed_case_count"`
	ReplaysPerCase               int            `json:"replays_per_case"`
	ParallelCaseLimit            int            `json:"parallel_case_limit"`
	OutcomeCounts                map[string]int `json:"outcome_counts"`
	V18BaselinePassCount         int            `json:"v18_baseline_pass_count"`
	ChangedOutcomeCount          int            `json:"changed_outcome_count"`
	NoBaselinePassRegression     bool           `json:"no_baseline_pass_regression"`
	FiveOfFiveAdvancementGateMet bool           `json:"five_of_five_advancement_gate_met"`
	DeterministicReplayComplete  bool           `json:"deterministic_replay_complete"`
	CorrectionRunUsed            bool           `json:"correction_run_used"`
	HeldOutAccessSurface         bool           `json:"held_out_access_surface"`
	HeldOutKeyOpened             bool           `json:"held_out_key_opened"`
	Accepted                     bool           `json:"accepted"`
	Reason                       string         `json:"reason"`
	TerminalState                string         `json:"terminal_state"`
}

func TestVersionNineteenGenerationZeroIsPermanentlyRetired(t *testing.T) {
	directory := v7ContractDirectory(t)
	repositoryRoot := v18GenerationOneRepositoryRoot(t, directory)
	v8VerifyManifest(t, directory, "V19_GENERATION_ZERO_RETIREMENT.sha256")

	var retirement v19GenerationZeroRetirement
	decodeV11Strict(t, v7ReadFile(t, filepath.Join(directory, "V19_GENERATION_ZERO_RETIREMENT.json")), &retirement)
	if retirement.Schema != "kicadai.closed-loop-open-set-generation-zero-retirement.v19" {
		t.Errorf("V19 retirement schema = %q", retirement.Schema)
	}
	if retirement.Version != 19 {
		t.Errorf("V19 retirement version = %d", retirement.Version)
	}
	if retirement.Stage != "public_generation_zero" {
		t.Errorf("V19 retirement stage = %q", retirement.Stage)
	}
	if retirement.RequiredCaseCount != 24 {
		t.Errorf("V19 required case count = %d", retirement.RequiredCaseCount)
	}
	if retirement.CompletedCaseCount != 24 {
		t.Errorf("V19 completed case count = %d", retirement.CompletedCaseCount)
	}
	if retirement.ReplaysPerCase != 2 {
		t.Errorf("V19 replays per case = %d", retirement.ReplaysPerCase)
	}
	if retirement.ParallelCaseLimit != 1 {
		t.Errorf("V19 parallel case limit = %d", retirement.ParallelCaseLimit)
	}
	if retirement.V18BaselinePassCount != 1 {
		t.Errorf("V19 recorded V18 pass count = %d", retirement.V18BaselinePassCount)
	}
	if retirement.ChangedOutcomeCount != 5 {
		t.Errorf("V19 changed outcome count = %d", retirement.ChangedOutcomeCount)
	}
	if retirement.NoBaselinePassRegression {
		t.Error("V19 incorrectly claims no baseline pass regression")
	}
	if retirement.FiveOfFiveAdvancementGateMet {
		t.Error("V19 incorrectly claims the advancement gate passed")
	}
	if !retirement.DeterministicReplayComplete {
		t.Error("V19 retirement lacks complete deterministic replay")
	}
	if retirement.CorrectionRunUsed {
		t.Error("V19 retirement incorrectly records a correction run")
	}
	if retirement.HeldOutAccessSurface || retirement.HeldOutKeyOpened {
		t.Error("V19 retirement crossed the public-only evaluation boundary")
	}
	if retirement.Accepted {
		t.Error("V19 retirement is incorrectly marked accepted")
	}
	if retirement.Reason != "public_advancement_and_preservation_gates_failed" {
		t.Errorf("V19 retirement reason = %q", retirement.Reason)
	}
	if retirement.TerminalState != "permanently_retired" {
		t.Errorf("V19 terminal state = %q", retirement.TerminalState)
	}
	if retirement.ContractManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, "V19_CONTRACT.sha256")) ||
		retirement.EvaluatorManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, "V19_EVALUATOR.sha256")) {
		t.Fatal("V19 retirement does not bind the frozen contract and evaluator manifests")
	}

	generationPath := filepath.Join(repositoryRoot, filepath.FromSlash(retirement.GenerationReport))
	baselinePath := filepath.Join(repositoryRoot, filepath.FromSlash(retirement.V18BaselineReport))
	v8VerifyManifest(t, filepath.Dir(generationPath), "report.sha256")
	if v7FileSHA256(t, generationPath) != retirement.GenerationReportSHA256 ||
		v7FileSHA256(t, baselinePath) != retirement.V18BaselineReportSHA256 {
		t.Fatal("V19 retirement report byte identity does not reproduce")
	}

	var baseline, generation v18GenerationOneReport
	if err := json.Unmarshal(v7ReadFile(t, baselinePath), &baseline); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(v7ReadFile(t, generationPath), &generation); err != nil {
		t.Fatal(err)
	}
	if baseline.Hash != retirement.V18BaselineHash || generation.Hash != retirement.GenerationHash ||
		baseline.CaseCount != retirement.RequiredCaseCount || generation.CaseCount != retirement.RequiredCaseCount ||
		len(baseline.Cases) != retirement.RequiredCaseCount || len(generation.Cases) != retirement.RequiredCaseCount {
		t.Fatal("V19 retirement report identity or case count is invalid")
	}

	remainingOutcomes := make(map[string]int, len(retirement.OutcomeCounts))
	for outcome, count := range retirement.OutcomeCounts {
		remainingOutcomes[outcome] = count
	}
	for _, outcome := range generation.OutcomeCounts {
		want, ok := remainingOutcomes[outcome.Outcome]
		if !ok {
			t.Errorf("V19 report contains unexpected outcome category %q", outcome.Outcome)
			continue
		}
		if want != outcome.Count {
			t.Errorf("V19 outcome %q count = %d, want %d", outcome.Outcome, outcome.Count, want)
		}
		delete(remainingOutcomes, outcome.Outcome)
	}
	if len(remainingOutcomes) != 0 || retirement.OutcomeCounts["pass"] != 0 {
		t.Fatalf("V19 retirement outcome counts are incomplete or admit a pass: %v", remainingOutcomes)
	}

	baselineByID := make(map[string]string, len(baseline.Cases))
	for _, result := range baseline.Cases {
		baselineByID[result.Case.ID] = result.Case.Outcome
	}
	changed := 0
	regressedPass := false
	for _, result := range generation.Cases {
		before, ok := baselineByID[result.Case.ID]
		if !ok {
			t.Errorf("V19 case %q is absent from the V18 baseline", result.Case.ID)
			continue
		}
		if len(result.ReplaySHA256) != retirement.ReplaysPerCase {
			t.Errorf("V19 case %q has %d replays, want %d", result.Case.ID, len(result.ReplaySHA256), retirement.ReplaysPerCase)
		}
		for replay := 1; replay < len(result.ReplaySHA256); replay++ {
			if result.ReplaySHA256[replay] != result.ReplaySHA256[0] {
				t.Errorf("V19 case %q replay %d is not deterministic", result.Case.ID, replay+1)
			}
		}
		if before != result.Case.Outcome {
			changed++
		}
		if before == "pass" && result.Case.Outcome != "pass" {
			regressedPass = true
		}
	}
	if changed != retirement.ChangedOutcomeCount || !regressedPass {
		t.Fatalf("V19 changed outcomes = %d and regressed pass = %t", changed, regressedPass)
	}
}
