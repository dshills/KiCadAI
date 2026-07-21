package behavioralintent

import (
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/reports"
)

func TestApplySearchEvidenceKeepsOnlySelectedRequirementExecutable(t *testing.T) {
	ready := Result{Status: StatusReady, Requirement: validBehavioralRequirement(t), Coverage: []CoverageRecord{{StatementID: "statement_001", Disposition: DispositionCompiled, Rationale: "behavior", References: []Reference{{Kind: "requirement", ID: "filter"}}}}}
	selected := ApplySearchEvidence(ready, architecturesearch.SearchResult{Status: architecturesearch.SearchSelected, Selected: &architecturesearch.CandidateResult{}, RequirementHash: "requirement", RegistryHash: "registry"})
	if selected.Status != StatusReady || selected.Requirement == nil || selected.Architecture == nil {
		t.Fatalf("selected result = %#v", selected)
	}
	unsupportedSearch := architecturesearch.SearchResult{
		Status: architecturesearch.SearchUnsupported, RequirementHash: "requirement", RegistryHash: "registry",
		Coverage: &architecturesearch.CapabilityCoverage{Records: []architecturesearch.CapabilityCoverageRecord{{Path: "requirements.objectives[0]", Capability: "frequency_filter", Status: architecturesearch.CoverageUnsupported}}},
	}
	unsupported := ApplySearchEvidence(ready, unsupportedSearch)
	if unsupported.Status != StatusUnsupported || unsupported.Requirement != nil || len(unsupported.CapabilityGaps) != 1 || unsupported.Coverage[0].Disposition != DispositionCapabilityGap {
		t.Fatalf("unsupported result = %#v", unsupported)
	}
	repeated := ApplySearchEvidence(ready, unsupportedSearch)
	if repeated.CapabilityGaps[0].ID != unsupported.CapabilityGaps[0].ID {
		t.Fatalf("gap id changed: %#v %#v", unsupported.CapabilityGaps, repeated.CapabilityGaps)
	}
}

func TestApplySearchEvidenceUsesStableMCURejectionCapability(t *testing.T) {
	ready := Result{Status: StatusReady, Requirement: validBehavioralRequirement(t), Coverage: []CoverageRecord{{StatementID: "statement_001", Disposition: DispositionCompiled, Rationale: "behavior", References: []Reference{{Kind: "requirement", ID: "filter"}}}}}
	path := "requirements.participants[0]"
	search := architecturesearch.SearchResult{
		Status:   architecturesearch.SearchUnsupported,
		Coverage: &architecturesearch.CapabilityCoverage{Records: []architecturesearch.CapabilityCoverageRecord{{Path: path, Capability: "programmable_controller", Status: architecturesearch.CoverageRejected}}},
		Rejections: []architecturesearch.RejectionSummary{{
			Code: reports.Code("MCU_PIN_ASSIGNMENT_IMPOSSIBLE"), Count: 1,
			Samples: []architecturesearch.ExpansionRejection{{Code: reports.Code("MCU_PIN_ASSIGNMENT_IMPOSSIBLE"), Path: path}},
		}},
	}
	result := ApplySearchEvidence(ready, search)
	if result.Status != StatusUnsupported || len(result.CapabilityGaps) != 1 || result.CapabilityGaps[0].ID != "mcu_pin_assignment_impossible" || result.CapabilityGaps[0].Capability != "mcu_pin_assignment_impossible" {
		t.Fatalf("MCU capability gap = %#v", result.CapabilityGaps)
	}
}

func TestApplySearchEvidenceTurnsAmbiguityIntoOneBehavioralQuestion(t *testing.T) {
	ready := Result{Status: StatusReady, Requirement: validBehavioralRequirement(t), Coverage: []CoverageRecord{{StatementID: "statement_001", Disposition: DispositionCompiled, Rationale: "behavior", References: []Reference{{Kind: "requirement", ID: "filter"}}}}}
	result := ApplySearchEvidence(ready, architecturesearch.SearchResult{Status: architecturesearch.SearchAmbiguous, RequirementHash: "requirement", RegistryHash: "registry"})
	if result.Status != StatusNeedsClarification || result.Requirement != nil || len(result.Clarifications) != 1 || len(result.Uncertainties) != 1 || result.Coverage[0].Disposition != DispositionClarification {
		t.Fatalf("ambiguous result = %#v", result)
	}
}
