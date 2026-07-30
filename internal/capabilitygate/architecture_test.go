package capabilitygate

import (
	"testing"

	"kicadai/internal/architecturesearch"
)

func TestAssessArchitectureUsesTypedSearchEvidence(t *testing.T) {
	requirement := architecturesearch.Requirement{
		Schema: architecturesearch.SchemaID, Version: architecturesearch.Version,
		Project: architecturesearch.Project{Name: "test"},
		Requirements: architecturesearch.Requirements{
			Domains: []architecturesearch.Domain{{ID: "vcc", Kind: "supply", NominalVoltageV: 5, Source: "external"}},
		},
		Acceptance: architecturesearch.Acceptance{RequireERC: true, RequireStrictDRC: true},
	}
	search := architecturesearch.SearchResult{
		Status: architecturesearch.SearchSelected, RequirementHash: testHash("requirement"),
		RegistryHash: testHash("registry"), CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formula"),
		Selected: &architecturesearch.CandidateResult{
			Fingerprint: testHash("candidate"),
			Selections: []architecturesearch.FragmentSelection{{
				ObligationPath: "objectives.regulate", Capability: "voltage_regulation",
				ProviderID: "catalog_function_fragments", ProviderRevision: "1.0.0",
				Evidence: architecturesearch.ContractEvidence{Confidence: architecturesearch.EvidenceVerified},
				Components: []architecturesearch.SelectedComponent{{
					InstanceID: "regulator", CatalogID: "regulator", VariantID: "sot_223",
					Evidence: architecturesearch.EvidenceVerified,
				}},
			}},
		},
	}
	assessment, err := AssessArchitecture(requirement, search, false)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Classification != ClassificationSupported || !assessment.FabricationReadyEligible {
		t.Fatalf("assessment=%#v, want supported", assessment)
	}
	if !hasAssessmentRequirement(assessment, RequirementArchitecture, "voltage_regulation") ||
		!hasAssessmentRequirement(assessment, RequirementComponent, "regulator:sot_223") ||
		!hasAssessmentRequirement(assessment, RequirementVerification, "strict_drc") {
		t.Fatalf("missing derived requirements: %#v", assessment.Requirements)
	}
}

func TestAssessArchitectureFailsClosedForUnsupportedSearch(t *testing.T) {
	requirement := architecturesearch.Requirement{
		Schema: architecturesearch.SchemaID, Version: architecturesearch.Version,
		Project: architecturesearch.Project{Name: "test"},
		Requirements: architecturesearch.Requirements{
			Domains: []architecturesearch.Domain{{ID: "gnd", Kind: "reference"}},
		},
	}
	assessment, err := AssessArchitecture(requirement, architecturesearch.SearchResult{Status: architecturesearch.SearchUnsupported}, false)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Classification != ClassificationUnsupported || len(assessment.Gaps) == 0 {
		t.Fatalf("assessment=%#v, want structured unsupported result", assessment)
	}
}

func TestAssessArchitectureKeepsInferredComponentExperimental(t *testing.T) {
	requirement := architecturesearch.Requirement{
		Schema: architecturesearch.SchemaID, Version: architecturesearch.Version,
		Project: architecturesearch.Project{Name: "test"},
		Requirements: architecturesearch.Requirements{
			Domains: []architecturesearch.Domain{{ID: "signal", Kind: "analog"}},
		},
	}
	search := architecturesearch.SearchResult{
		Status: architecturesearch.SearchSelected, CatalogHash: testHash("catalog"),
		Selected: &architecturesearch.CandidateResult{
			Selections: []architecturesearch.FragmentSelection{{
				ObligationPath: "objectives.condition", Capability: "signal_conditioning",
				ProviderID: "catalog_function_fragments",
				Metrics:    architecturesearch.ExpansionMetrics{UnprovenNonSafety: 1},
				Components: []architecturesearch.SelectedComponent{{
					InstanceID: "bias", CatalogID: "resistor.generic.0603",
					VariantID: "0603", Evidence: architecturesearch.EvidenceRuleInferred,
				}},
			}},
		},
	}
	assessment, err := AssessArchitecture(requirement, search, false)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Classification != ClassificationExperimental || assessment.FabricationReadyEligible {
		t.Fatalf("classification=%q gaps=%#v", assessment.Classification, assessment.Gaps)
	}
	var inferredSelection, inferredConfidence bool
	for _, evidence := range assessment.Evidence {
		if evidence.Kind == "catalog_component" && evidence.Status == EvidenceInferred && !evidence.Advisory {
			inferredSelection = true
		}
		if evidence.Kind == "catalog_component_confidence" && evidence.Status == EvidenceInferred && evidence.Advisory {
			inferredConfidence = true
		}
	}
	if !inferredSelection || !inferredConfidence {
		t.Fatalf("selection/confidence distinction missing: %#v", assessment.Evidence)
	}
	if len(assessment.Risks) < 2 {
		t.Fatalf("inferred risks missing: %#v", assessment.Risks)
	}
}

func hasAssessmentRequirement(assessment Assessment, kind RequirementKind, id string) bool {
	for _, requirement := range assessment.Requirements {
		if requirement.Kind == kind && requirement.ID == id {
			return true
		}
	}
	return false
}

func testHash(value string) string {
	digest, _ := Digest(value)
	return digest
}

func TestStableCapabilityIDIsUnambiguous(t *testing.T) {
	if stableCapabilityID("a:b", "c") == stableCapabilityID("a", "b:c") {
		t.Fatal("length-prefixed capability IDs collided")
	}
}
