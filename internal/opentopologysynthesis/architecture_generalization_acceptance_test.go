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

	runTwice := func(t *testing.T, file string) (SynthesisRun, SynthesisRun) {
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
		return first, second
	}

	passed := 0
	for _, entry := range manifest.DesignCases {
		entry := entry
		t.Run(entry.ID, func(t *testing.T) {
			first, _ := runTwice(t, entry.RequirementFile)
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
			for _, candidate := range first.Candidates {
				if len(candidate.Evaluations) != 0 {
					evaluatedTopologies[candidate.TopologyHash] = true
					evaluatedActiveStructures[candidate.ActiveStructureHash] = true
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
			if entry.ID == "protected_programmable_current_output" {
				if len(evaluatedActiveStructures) < 2 || len(first.Report.Selected.Ranking.Alternatives) < 2 {
					t.Fatalf("protected current active structures/ranking = %v/%#v", evaluatedActiveStructures, first.Report.Selected.Ranking)
				}
				for index, alternative := range first.Report.Selected.Ranking.Alternatives {
					if alternative.ActiveStructureHash == "" || alternative.PhysicalHash == "" ||
						alternative.Disposition == "" || alternative.Reason == "" ||
						(index == 0) != alternative.Selected {
						t.Fatalf("protected current ranking alternative[%d] = %#v", index, alternative)
					}
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
			first, _ := runTwice(t, entry.RequirementFile)
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
