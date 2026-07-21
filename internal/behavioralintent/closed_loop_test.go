package behavioralintent

import (
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/closedloopsynthesis"
)

func TestApplyClosedLoopEvidenceKeepsOnlyHashBoundPassExecutable(t *testing.T) {
	ready := Result{
		Status: StatusReady, Requirement: validBehavioralRequirement(t),
		Architecture: &ArchitectureEvidence{Status: architecturesearch.SearchSelected, RequirementHash: testCapabilitySHA256, RegistryHash: testCapabilitySHA256, CatalogHash: testCapabilitySHA256},
		Coverage:     []CoverageRecord{{StatementID: "statement_001", Disposition: DispositionCompiled, Rationale: "behavior", References: []Reference{{Kind: "requirement", ID: "filter"}}}},
	}
	pass := closedloopsynthesis.Report{
		Schema: closedloopsynthesis.ReportSchema, PolicyVersion: closedloopsynthesis.PolicyVersion,
		RequirementHash: testCapabilitySHA256, RegistryHash: testCapabilitySHA256, CatalogHash: testCapabilitySHA256,
		FormulaLibraryHash: testCapabilitySHA256, ModelRegistryHash: testCapabilitySHA256,
		Status: "pass", StopReason: closedloopsynthesis.StopPassed, Selected: &closedloopsynthesis.SelectedResult{}, SelectedCircuitHash: testCapabilitySHA256,
	}
	qualified := ApplyClosedLoopEvidence(ready, pass)
	if qualified.Status != StatusReady || qualified.Requirement == nil || qualified.ClosedLoop == nil {
		t.Fatalf("qualified = %#v", qualified)
	}

	failure := pass
	failure.Status = "failed"
	failure.StopReason = closedloopsynthesis.StopModelTrustFailed
	failure.Selected = nil
	failure.SelectedCircuitHash = ""
	unsupported := ApplyClosedLoopEvidence(ready, failure)
	if unsupported.Status != StatusUnsupported || unsupported.Requirement != nil || len(unsupported.CapabilityGaps) != 1 || unsupported.Coverage[0].Disposition != DispositionCapabilityGap {
		t.Fatalf("unsupported = %#v", unsupported)
	}
	repeated := ApplyClosedLoopEvidence(ready, failure)
	if repeated.CapabilityGaps[0].ID != unsupported.CapabilityGaps[0].ID {
		t.Fatalf("gap changed: first=%#v second=%#v", unsupported.CapabilityGaps, repeated.CapabilityGaps)
	}
}

func TestApplyClosedLoopEvidenceRejectsMismatchedHashes(t *testing.T) {
	ready := Result{
		Status: StatusReady, Requirement: validBehavioralRequirement(t),
		Architecture: &ArchitectureEvidence{Status: architecturesearch.SearchSelected, RequirementHash: testCapabilitySHA256, RegistryHash: testCapabilitySHA256, CatalogHash: testCapabilitySHA256},
	}
	report := closedloopsynthesis.Report{
		Schema: closedloopsynthesis.ReportSchema, PolicyVersion: closedloopsynthesis.PolicyVersion,
		RequirementHash: testCapabilitySHA256, RegistryHash: testCapabilitySHA256, CatalogHash: testCapabilitySHA256,
		FormulaLibraryHash: testCapabilitySHA256, ModelRegistryHash: testCapabilitySHA256,
		Status: "pass", StopReason: closedloopsynthesis.StopPassed, Selected: &closedloopsynthesis.SelectedResult{}, SelectedCircuitHash: testCapabilitySHA256,
	}
	report.CatalogHash = "mismatch"
	result := ApplyClosedLoopEvidence(ready, report)
	if result.Status != StatusUnsupported || result.Requirement != nil {
		t.Fatalf("mismatched evidence = %#v", result)
	}
}
