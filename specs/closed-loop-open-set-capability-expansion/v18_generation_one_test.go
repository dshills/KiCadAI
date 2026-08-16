package closedloopopensetcontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

type v18GenerationOneReport struct {
	Hash          string `json:"hash"`
	CaseCount     int    `json:"case_count"`
	OutcomeCounts []struct {
		Outcome string `json:"outcome"`
		Count   int    `json:"count"`
	} `json:"outcome_counts"`
	Cases []struct {
		Case struct {
			ID                   string   `json:"id"`
			Outcome              string   `json:"outcome"`
			SatisfiedObligations []string `json:"satisfied_obligations"`
		} `json:"case"`
		Gates        map[string]bool `json:"gates"`
		ReplaySHA256 []string        `json:"replay_sha256"`
		Promotions   []struct {
			RunSHA256       string `json:"run_sha256"`
			ProjectSHA256   string `json:"project_sha256"`
			InstalledKiCad  bool   `json:"installed_kicad"`
			ReplayIdentical bool   `json:"replay_identical"`
		} `json:"promotions"`
	} `json:"cases"`
}

type v18GenerationOneComparison struct {
	Schema                  string         `json:"schema"`
	Version                 int            `json:"version"`
	SourceCommit            string         `json:"source_commit"`
	BaselineReport          string         `json:"baseline_report"`
	BaselineReportSHA256    string         `json:"baseline_report_sha256"`
	BaselineHash            string         `json:"baseline_hash"`
	GenerationReport        string         `json:"generation_report"`
	GenerationReportSHA256  string         `json:"generation_report_sha256"`
	GenerationHash          string         `json:"generation_hash"`
	CaseCount               int            `json:"case_count"`
	ReplaysPerCase          int            `json:"replays_per_case"`
	BaselineOutcomeCounts   map[string]int `json:"baseline_outcome_counts"`
	GenerationOutcomeCounts map[string]int `json:"generation_outcome_counts"`
	NewlyPassing            []struct {
		CaseID                 string `json:"case_id"`
		Before                 string `json:"before"`
		After                  string `json:"after"`
		ReplaySHA256           string `json:"replay_sha256"`
		PromotionRunSHA256     string `json:"promotion_run_sha256"`
		PromotionProjectSHA256 string `json:"promotion_project_sha256"`
	} `json:"newly_passing"`
	OtherOutcomeTransitions         []any `json:"other_outcome_transitions"`
	LostSatisfiedObligations        int   `json:"lost_satisfied_obligations"`
	NoBaselinePassRegression        bool  `json:"no_baseline_pass_regression"`
	UnsafeEvidencePreserved         bool  `json:"unsafe_evidence_preserved"`
	TypedObligationNonRegression    bool  `json:"typed_obligation_non_regression"`
	DeterministicReplayComplete     bool  `json:"deterministic_replay_complete"`
	InstalledKiCadPromotionComplete bool  `json:"installed_kicad_promotion_complete"`
	AllPhysicalGatesPassed          bool  `json:"all_physical_gates_passed"`
	Accepted                        bool  `json:"accepted"`
}

func TestVersionEighteenGenerationOneIsStrictlyImprovingAndPhysicallyPromoted(t *testing.T) {
	directory := v7ContractDirectory(t)
	repositoryRoot := v18GenerationOneRepositoryRoot(t, directory)
	v8VerifyManifest(t, directory, "V18_GENERATION_ONE_COMPARISON.sha256")
	var comparison v18GenerationOneComparison
	decodeV11Strict(t, v7ReadFile(t, filepath.Join(directory, "V18_GENERATION_ONE_COMPARISON.json")), &comparison)
	baselinePath := filepath.Join(repositoryRoot, filepath.FromSlash(comparison.BaselineReport))
	generationPath := filepath.Join(repositoryRoot, filepath.FromSlash(comparison.GenerationReport))
	v8VerifyManifest(t, filepath.Dir(generationPath), "report.sha256")

	var baseline, generation v18GenerationOneReport
	if err := json.Unmarshal(v7ReadFile(t, baselinePath), &baseline); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(v7ReadFile(t, generationPath), &generation); err != nil {
		t.Fatal(err)
	}
	if comparison.Schema != "kicadai.closed-loop-open-set-generation-one-comparison.v18" || comparison.Version != 18 ||
		comparison.SourceCommit == "" || comparison.ReplaysPerCase != 2 || !comparison.Accepted ||
		!comparison.NoBaselinePassRegression || !comparison.UnsafeEvidencePreserved ||
		!comparison.TypedObligationNonRegression || !comparison.DeterministicReplayComplete ||
		!comparison.InstalledKiCadPromotionComplete || !comparison.AllPhysicalGatesPassed ||
		comparison.LostSatisfiedObligations != 0 || len(comparison.OtherOutcomeTransitions) != 0 {
		t.Fatalf("invalid V18 generation-one comparison: %+v", comparison)
	}
	if baseline.Hash != comparison.BaselineHash || generation.Hash != comparison.GenerationHash ||
		v7FileSHA256(t, baselinePath) != comparison.BaselineReportSHA256 ||
		v7FileSHA256(t, generationPath) != comparison.GenerationReportSHA256 ||
		baseline.CaseCount != comparison.CaseCount || generation.CaseCount != comparison.CaseCount ||
		len(baseline.Cases) != comparison.CaseCount || len(generation.Cases) != comparison.CaseCount {
		t.Fatalf("unexpected V18 report identity or case count")
	}
	wantOutcomes := comparison.GenerationOutcomeCounts
	for _, outcome := range generation.OutcomeCounts {
		if wantOutcomes[outcome.Outcome] != outcome.Count {
			t.Fatalf("V18 outcome %q count = %d, want %d", outcome.Outcome, outcome.Count, wantOutcomes[outcome.Outcome])
		}
		delete(wantOutcomes, outcome.Outcome)
	}
	if len(wantOutcomes) != 0 {
		t.Fatalf("V18 report is missing outcome counts: %v", wantOutcomes)
	}

	baselineByID := make(map[string]struct {
		Outcome     string
		Obligations []string
	}, len(baseline.Cases))
	for _, result := range baseline.Cases {
		baselineByID[result.Case.ID] = struct {
			Outcome     string
			Obligations []string
		}{result.Case.Outcome, result.Case.SatisfiedObligations}
	}
	transitions := 0
	if len(comparison.NewlyPassing) != 1 {
		t.Fatalf("V18 newly passing count = %d, want 1", len(comparison.NewlyPassing))
	}
	newPass := comparison.NewlyPassing[0]
	for _, result := range generation.Cases {
		before, ok := baselineByID[result.Case.ID]
		if !ok {
			t.Fatalf("generation case %q is absent from baseline", result.Case.ID)
		}
		for _, obligation := range before.Obligations {
			if !slices.Contains(result.Case.SatisfiedObligations, obligation) {
				t.Fatalf("case %q lost satisfied obligation %q", result.Case.ID, obligation)
			}
		}
		if before.Outcome == "pass" && result.Case.Outcome != "pass" {
			t.Fatalf("baseline pass %q regressed to %q", result.Case.ID, result.Case.Outcome)
		}
		if before.Outcome == "unsafe" && result.Case.Outcome == "pass" {
			t.Fatalf("unsafe baseline case %q became pass", result.Case.ID)
		}
		if before.Outcome != result.Case.Outcome {
			transitions++
			if result.Case.ID != newPass.CaseID || before.Outcome != newPass.Before || result.Case.Outcome != newPass.After {
				t.Fatalf("unexpected V18 transition for %q: %s -> %s", result.Case.ID, before.Outcome, result.Case.Outcome)
			}
		}
	}
	if transitions != 1 {
		t.Fatalf("V18 transition count = %d, want 1", transitions)
	}

	for _, result := range generation.Cases {
		if result.Case.ID != newPass.CaseID {
			continue
		}
		if len(result.ReplaySHA256) != comparison.ReplaysPerCase || result.ReplaySHA256[0] != newPass.ReplaySHA256 ||
			result.ReplaySHA256[0] != result.ReplaySHA256[1] || len(result.Promotions) != comparison.ReplaysPerCase {
			t.Fatalf("V18 passing case lacks two identical replays and promotions")
		}
		requiredGates := []string{
			"primitive_only", "topology_search", "simulation", "all_corners", "model_provenance",
			"closed_loop_evidence", "complete_routing", "connectivity", "writer_correctness",
			"round_trip_zero_diff", "erc", "strict_drc", "deterministic_replay", "fail_closed",
		}
		if len(result.Gates) != len(requiredGates) {
			t.Fatalf("V18 passing case gate count = %d, want %d", len(result.Gates), len(requiredGates))
		}
		for _, gate := range requiredGates {
			if passed, ok := result.Gates[gate]; !ok || !passed {
				t.Fatalf("V18 passing case gate %q missing or failed", gate)
			}
		}
		for _, promotion := range result.Promotions {
			if !promotion.InstalledKiCad || !promotion.ReplayIdentical ||
				promotion.RunSHA256 != newPass.PromotionRunSHA256 ||
				promotion.ProjectSHA256 != newPass.PromotionProjectSHA256 {
				t.Fatalf("V18 installed-KiCad promotion evidence is invalid")
			}
		}
		return
	}
	t.Fatal("V18 passing case is absent")
}

func v18GenerationOneRepositoryRoot(t *testing.T, start string) string {
	t.Helper()
	for directory := filepath.Clean(start); ; directory = filepath.Dir(directory) {
		if info, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil && !info.IsDir() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatalf("repository root not found from %q", start)
		}
	}
}
