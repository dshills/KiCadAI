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

const verifyArchitectureGeneralizationEnv = "KICADAI_VERIFY_ARCHITECTURE_GENERALIZATION"

func TestArchitectureGeneralizationAcceptance(t *testing.T) {
	if os.Getenv(verifyArchitectureGeneralizationEnv) != "1" {
		t.Skip("set " + verifyArchitectureGeneralizationEnv + "=1 to run the frozen architecture-generalization acceptance corpus")
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

	runTwice := func(t *testing.T, file string) (SynthesisRun, SynthesisRun, Requirement) {
		t.Helper()
		requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
			t,
			filepath.Join(architectureGeneralizationCorpusRoot(), file),
		)))
		if len(issues) != 0 {
			t.Fatalf("requirement decode issues: %#v", issues)
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
			t.Fatalf("synthesis replay differs first=%s second=%s", first.Hash, second.Hash)
		}
		return first, second, requirement
	}

	passed := 0
	for _, entry := range manifest.DesignCases {
		entry := entry
		t.Run(entry.ID, func(t *testing.T) {
			first, _, requirement := runTwice(t, entry.RequirementFile)
			if first.Report.Status != StatusPassed {
				if first.Physical != nil || len(first.Report.Diagnostics) == 0 {
					t.Fatalf("non-pass design did not fail closed: status=%s physical=%t diagnostics=%#v", first.Report.Status, first.Physical != nil, first.Report.Diagnostics)
				}
				t.Logf("stable unsupported design hash=%s status=%s diagnostics=%#v", first.Hash, first.Report.Status, first.Report.Diagnostics)
				return
			}
			if first.Physical == nil || first.Physical.Status != PhysicalLoweringReady ||
				first.Report.Selected == nil || first.SelectedTrial == nil {
				t.Fatalf("passing design lacks selected physical evidence: physical=%#v selected=%#v", first.Physical, first.Report.Selected)
			}
			evaluatedTopologies := map[string]bool{}
			evaluatedActiveStructures := map[string]bool{}
			selectedPlanFound := false
			rankedFingerprints := map[string]bool{}
			for _, alternative := range first.Report.Selected.Ranking.Alternatives {
				rankedFingerprints[alternative.Fingerprint] = true
			}
			rejectionsByFingerprint := map[string]SelectionRejection{}
			for _, rejection := range first.Report.Selected.Ranking.Rejections {
				rejectionsByFingerprint[rejection.Fingerprint] = rejection
			}
			for _, candidate := range first.Candidates {
				if candidate.ValuePlan.Status == ValuePlanReady && len(candidate.Evaluations) == 0 {
					t.Fatalf("value-ready candidate %s was not simulated", candidate.Fingerprint)
				}
				if len(candidate.Evaluations) != 0 {
					evaluatedTopologies[candidate.TopologyHash] = true
					evaluatedActiveStructures[candidate.ActiveStructureHash] = true
				}
				for _, evaluation := range candidate.Evaluations {
					if evaluation.Hash == "" {
						t.Fatalf("candidate %s has unhashed simulation evidence", candidate.Fingerprint)
					}
					if evaluation.Status == SimulationEvaluationPassed {
						assertEvaluationCoversRequirementAnalyses(t, requirement, evaluation)
					}
				}
				if !rankedFingerprints[candidate.Fingerprint] {
					rejection, found := rejectionsByFingerprint[candidate.Fingerprint]
					if !found || rejection.Stage == "" || len(rejection.Codes) == 0 {
						t.Fatalf("candidate %s lacks explicit deterministic rejection evidence: %#v", candidate.Fingerprint, rejection)
					}
				}
				if candidate.Fingerprint == first.Report.Selected.Fingerprint {
					selectedPlanFound = true
					assertValueTrialHasExplainableComponents(t, candidate.ValuePlan, *first.SelectedTrial)
				}
			}
			delete(evaluatedActiveStructures, "")
			if len(evaluatedTopologies) < 2 {
				t.Fatalf("trusted simulation evaluated %d topology hashes, want at least 2", len(evaluatedTopologies))
			}
			if !selectedPlanFound || strings.TrimSpace(first.Report.Selected.SelectionSummary) == "" ||
				strings.TrimSpace(first.Report.Selected.Ranking.Policy) == "" {
				t.Fatalf("selected architecture lacks ranked explainable evidence: %#v", first.Report.Selected)
			}
			requireActiveDiversity := entry.ID == "protected_programmable_current_output" ||
				entry.ID == "regulated_low_voltage_output" ||
				entry.ID == "selective_midband_transfer"
			if requireActiveDiversity {
				if len(evaluatedActiveStructures) < 2 || len(first.Report.Selected.Ranking.Alternatives) < 2 {
					t.Fatalf("%s active structures/ranking = %v/%#v", entry.ID, evaluatedActiveStructures, first.Report.Selected.Ranking)
				}
				rankedActiveStructures := map[string]bool{}
				for index, alternative := range first.Report.Selected.Ranking.Alternatives {
					if alternative.ActiveStructureHash == "" || alternative.PhysicalHash == "" ||
						alternative.Disposition == "" || alternative.Reason == "" ||
						alternative.Rank != index+1 || alternative.ComponentCount <= 0 ||
						(index == 0) != alternative.Selected {
						t.Fatalf("%s ranking alternative[%d] = %#v", entry.ID, index, alternative)
					}
					rankedActiveStructures[alternative.ActiveStructureHash] = true
				}
				if len(rankedActiveStructures) < 2 {
					t.Fatalf("%s ranked physically-ready active structures=%v, want at least 2", entry.ID, rankedActiveStructures)
				}
			}
			passed++
		})
	}
	if passed < 5 {
		t.Fatalf("architecture-generalization designs passed=%d, want at least 5 of 6", passed)
	}

	for _, entry := range manifest.AdversarialCases {
		entry := entry
		t.Run(entry.ID, func(t *testing.T) {
			first, _, _ := runTwice(t, entry.RequirementFile)
			if first.Report.Status == StatusPassed || first.Physical != nil || len(first.Report.Diagnostics) == 0 {
				t.Fatalf("adversarial case did not fail closed: status=%s physical=%t diagnostics=%#v", first.Report.Status, first.Physical != nil, first.Report.Diagnostics)
			}
			for _, diagnostic := range first.Report.Diagnostics {
				if strings.TrimSpace(string(diagnostic.Code)) == "" || strings.TrimSpace(diagnostic.Message) == "" {
					t.Fatalf("adversarial diagnostic is not actionable: %#v", diagnostic)
				}
			}
		})
	}
}
