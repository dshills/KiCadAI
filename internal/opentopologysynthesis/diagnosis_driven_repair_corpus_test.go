package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestProtectedCurrentDriverRepairTraceReplaysAndFailsClosedPrecisely(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, filepath.Join(
		architectureGeneralizationCorpusRoot(), "protected_programmable_current_output.json",
	))))
	if len(issues) != 0 {
		t.Fatalf("requirement issues: %#v", issues)
	}
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 2_000
	policy.MaxGeneratedGraphs = 50_000
	policy.MaxRetainedCandidates = 16
	policy.MaxValueTrials = 64
	policy.MaxTopologyRepairs = 16
	policy.MaxCandidateSimulations = 4_096
	policy.MaxCornerEvaluations = 16_384
	first := Synthesize(context.Background(), requirement, inventory, environment, policy)
	second := Synthesize(context.Background(), requirement, inventory, environment, policy)
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if first.Hash == "" || first.Hash != second.Hash || !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("protected current-driver repair replay differs")
	}
	if first.Report.Status == StatusPassed {
		if first.Physical == nil || first.Physical.Status != PhysicalLoweringReady {
			t.Fatalf("passing repaired current driver lacks physical lowering: %#v", first.Physical)
		}
		return
	}
	if first.Physical != nil {
		t.Fatal("failed repair emitted a physical current-driver project")
	}
	traced := false
	causal := false
	for _, candidate := range first.Candidates {
		if candidate.Repair == nil {
			continue
		}
		trace := candidate.Repair.Trace
		if trace.Hash == "" || trace.Budget != policy.MaxTopologyRepairs || trace.Consumed > trace.Budget || len(trace.Diagnostics) == 0 || len(trace.Proposals) == 0 || len(trace.Outcomes) == 0 {
			t.Fatalf("incomplete current-driver repair trace: %#v", trace)
		}
		for _, proposal := range trace.Proposals {
			if proposal.ID == "" || proposal.DiagnosticHash == "" || proposal.ReenterStage == "" || proposal.ExpectedEffect == "" {
				t.Fatalf("incomplete current-driver proposal: %#v", proposal)
			}
		}
		if len(candidate.Repair.CausalAnalyses) == 0 {
			t.Fatal("current-driver repair lacks simulation-guided causal analyses")
		}
		for _, analysis := range candidate.Repair.CausalAnalyses {
			if err := validateCausalRepairAnalysis(analysis); err != nil {
				t.Fatalf("invalid current-driver causal analysis: %v", err)
			}
			if len(analysis.Trials) == 0 {
				t.Fatal("current-driver causal analysis lacks perturbation trials")
			}
			causal = true
		}
		traced = true
	}
	if !traced {
		t.Fatal("current-driver failure lacks diagnosis-driven repair evidence")
	}
	if !causal {
		t.Fatal("current-driver failure lacks complete causal repair evidence")
	}
}

func TestElectricalRepairDiagnosticCategoriesCoverBoundedTaxonomy(t *testing.T) {
	tests := []struct {
		diagnosis Diagnosis
		want      string
	}{
		{Diagnosis{Code: diagnosisNonconvergent}, "bias_or_reference_access"},
		{Diagnosis{Code: diagnosisUnstable}, "feedback_or_compensation"},
		{Diagnosis{Code: diagnosisAssertionAboveMaximum, Analysis: "dc_sweep"}, "value_domain_or_feedback"},
		{Diagnosis{Code: diagnosisAssertionBelowMinimum, Analysis: "electrothermal"}, "rating_thermal_or_soa"},
		{Diagnosis{Code: diagnosisThermalUnavailable}, "thermal_evidence"},
		{Diagnosis{Code: diagnosisModelUnavailable}, "model_evidence"},
	}
	for _, test := range tests {
		if got := electricalRepairCategory(test.diagnosis); got != test.want {
			t.Errorf("category for %#v = %q, want %q", test.diagnosis, got, test.want)
		}
	}
}
