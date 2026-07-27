package capabilityevaluation

import (
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/behavioralintent"
)

func TestCaseFromPipelinePreservesCompilerBlockers(t *testing.T) {
	meta := CorpusCase{ID: "case_001", Domain: DomainMCU, SafetyImpact: SafetyRelevant}
	clarification, err := CaseFromPipeline(meta, behavioralintent.Result{
		Status: behavioralintent.StatusNeedsClarification,
		Uncertainties: []behavioralintent.Uncertainty{{
			ID: "uncertainty_001", Kind: "debug_voltage", ResolvedBy: "clarification_001",
		}},
		Clarifications: []behavioralintent.Clarification{{
			ID: "clarification_001", Path: "requirements.voltage",
			WhyNeeded: "debug voltage bounds are required",
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if clarification.Outcome != OutcomeNeedsClarification ||
		len(clarification.Observations) != 1 ||
		clarification.Observations[0].Capability != "debug_voltage" {
		t.Fatalf("clarification evidence = %#v", clarification)
	}

	unsupported, err := CaseFromPipeline(meta, behavioralintent.Result{
		Status: behavioralintent.StatusUnsupported,
		CapabilityGaps: []behavioralintent.CapabilityGap{{
			Capability: "debug_load_qualification", Path: "requirements.debug",
			Reason:           "trusted load evidence is unavailable",
			RequiredEvidence: []string{"reviewed debug loading model"},
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if unsupported.Outcome != OutcomeUnsupported ||
		unsupported.Observations[0].Capability != "debug_load_qualification" {
		t.Fatalf("unsupported evidence = %#v", unsupported)
	}
}

func TestCaseFromPipelineMapsArchitectureTerminalOutcomes(t *testing.T) {
	meta := CorpusCase{ID: "case_001", Domain: DomainDigital, SafetyImpact: SafetyReviewRequired}
	compilation := behavioralintent.Result{Status: behavioralintent.StatusReady}
	tests := []struct {
		name           string
		searchStatus   architecturesearch.SearchStatus
		coverageStatus architecturesearch.CoverageStatus
		outcome        Outcome
		code           string
	}{
		{"ready", architecturesearch.SearchSelected, architecturesearch.CoverageSelected, OutcomeReady, ""},
		{"unsupported", architecturesearch.SearchUnsupported, architecturesearch.CoverageUnsupported, OutcomeUnsupported, CodeCapabilityUnsupported},
		{"ambiguous", architecturesearch.SearchAmbiguous, architecturesearch.CoverageAmbiguous, OutcomeAmbiguous, CodeCapabilityAmbiguous},
		{"exhausted", architecturesearch.SearchExhausted, architecturesearch.CoverageBudgetExhausted, OutcomeBudgetExhausted, CodeSearchBudgetExhausted},
		{"failed", architecturesearch.SearchFailed, architecturesearch.CoverageRejected, OutcomeUnsupported, CodeArchitectureSearchFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			search := architecturesearch.SearchResult{
				Status: test.searchStatus,
				Coverage: &architecturesearch.CapabilityCoverage{Records: []architecturesearch.CapabilityCoverageRecord{{
					Path: "requirements.clock", Capability: "clock_fanout_loading", Status: test.coverageStatus,
				}}},
			}
			if test.searchStatus == architecturesearch.SearchSelected {
				search.Selected = &architecturesearch.CandidateResult{}
			}
			result, err := CaseFromPipeline(meta, compilation, &search)
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome != test.outcome {
				t.Fatalf("outcome = %q, want %q", result.Outcome, test.outcome)
			}
			if test.outcome == OutcomeReady {
				if len(result.Observations) != 0 {
					t.Fatalf("ready observations = %#v", result.Observations)
				}
			} else if len(result.Observations) != 1 || result.Observations[0].Code != test.code {
				t.Fatalf("blocking observations = %#v", result.Observations)
			}
		})
	}
}

func TestEvaluateCorpusBindsMembershipAndMetadata(t *testing.T) {
	corpus := testCorpus(CorpusDiscovery, "discovery")
	cases := make([]CaseResult, 0, len(corpus.Cases))
	for _, current := range corpus.Cases {
		cases = append(cases, CaseResult{
			ID: current.ID, Domain: current.Domain, SafetyImpact: current.SafetyImpact,
			Outcome: OutcomeReady, Observations: []Observation{},
		})
	}
	report, err := EvaluateCorpus(corpus, cases, ImpactRegistry{Version: "registry_v1"}, DefaultRankingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	wantHash, err := CorpusSHA256(corpus)
	if err != nil {
		t.Fatal(err)
	}
	if report.CorpusRole != CorpusDiscovery || report.CorpusSHA256 != wantHash {
		t.Fatalf("corpus binding = %q/%q", report.CorpusRole, report.CorpusSHA256)
	}

	cases[0].Domain = DomainPower
	if _, err := EvaluateCorpus(corpus, cases, ImpactRegistry{Version: "registry_v1"}, DefaultRankingPolicy()); err == nil {
		t.Fatal("expected corpus metadata mismatch")
	}
}
