package capabilitygate

import (
	"bytes"
	"testing"
)

func TestAssessmentClassifiesSupportedExperimentalAndUnsupported(t *testing.T) {
	verifiedDigest, err := Digest(map[string]string{"evidence": "verified"})
	if err != nil {
		t.Fatal(err)
	}
	baseRequirement := Requirement{Kind: RequirementArchitecture, ID: "voltage_regulation", EvidenceIDs: []string{"architecture"}}
	tests := []struct {
		name        string
		evidence    Evidence
		gaps        []Gap
		want        Classification
		fabrication bool
	}{
		{
			name:     "supported",
			evidence: Evidence{ID: "architecture", Kind: "promotion", Status: EvidenceVerified, Source: "registry://voltage_regulation", Digest: verifiedDigest},
			want:     ClassificationSupported, fabrication: true,
		},
		{
			name:     "experimental",
			evidence: Evidence{ID: "architecture", Kind: "provider", Status: EvidenceInferred, Source: "registry://voltage_regulation"},
			want:     ClassificationExperimental,
		},
		{
			name:     "unsupported",
			evidence: Evidence{ID: "architecture", Kind: "provider", Status: EvidenceMissing},
			gaps:     []Gap{{Code: "CAPABILITY_MISSING", Kind: RequirementArchitecture, ID: "voltage_regulation", Reason: "no provider"}},
			want:     ClassificationUnsupported,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assessment, err := Assess(Input{
				Stage: "architecture_selection", Requirements: []Requirement{baseRequirement},
				Evidence: []Evidence{test.evidence}, Gaps: test.gaps,
			})
			if err != nil {
				t.Fatal(err)
			}
			if assessment.Classification != test.want || assessment.FabricationReadyEligible != test.fabrication {
				t.Fatalf("assessment=%#v, want classification=%q fabrication=%v", assessment, test.want, test.fabrication)
			}
		})
	}
}

func TestAssessmentRequiresReproducibleVerifiedEvidence(t *testing.T) {
	_, err := Assess(Input{
		Stage:        "initial",
		Requirements: []Requirement{{Kind: RequirementComponent, ID: "opamp", EvidenceIDs: []string{"component"}}},
		Evidence:     []Evidence{{ID: "component", Kind: "catalog", Status: EvidenceVerified, Source: "catalog://opamp"}},
	})
	if err == nil {
		t.Fatal("expected verified evidence without digest to fail")
	}
}

func TestAssessmentRequiresUnlinkedInferenceToBeExplicitlyAdvisory(t *testing.T) {
	digest, err := Digest("verified")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Assess(Input{
		Stage: "architecture_selection",
		Requirements: []Requirement{{
			Kind: RequirementArchitecture, ID: "amplifier", EvidenceIDs: []string{"selection"},
		}},
		Evidence: []Evidence{
			{ID: "selection", Kind: "selection", Status: EvidenceVerified, Source: "provider://amplifier", Digest: digest},
			{ID: "confidence", Kind: "confidence", Status: EvidenceInferred, Source: "rule://amplifier"},
		},
	})
	if err == nil {
		t.Fatal("expected unlinked inferred evidence without advisory marker to fail")
	}
}

func TestDigestCanonicalizesJSONRepresentations(t *testing.T) {
	first := map[string]any{}
	first["b"] = []any{true, "value"}
	first["a"] = 1
	second := map[string]any{}
	second["a"] = 1
	second["b"] = []any{true, "value"}
	fromFirst, err := Digest(first)
	if err != nil {
		t.Fatal(err)
	}
	fromSecond, err := Digest(second)
	if err != nil {
		t.Fatal(err)
	}
	if fromFirst != fromSecond {
		t.Fatalf("canonical digests differ: first=%s second=%s", fromFirst, fromSecond)
	}
}

func TestAssessmentReplayIsByteIdentical(t *testing.T) {
	digest, err := Digest([]string{"verified", "block"})
	if err != nil {
		t.Fatal(err)
	}
	input := Input{
		Stage:             "block_planning",
		ExperimentalOptIn: true,
		Requirements: []Requirement{
			{Kind: RequirementPhysical, ID: "pcb_realization", EvidenceIDs: []string{"block"}},
			{Kind: RequirementArchitecture, ID: "amplifier", EvidenceIDs: []string{"block"}},
		},
		Evidence: []Evidence{{ID: "block", Kind: "block_verification", Status: EvidenceVerified, Source: "block://amplifier", Digest: digest}},
		Risks:    []Risk{{Code: "REVIEW_POWER", Stage: "block_planning", Summary: "review thermal margin"}},
	}
	first, err := Assess(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Assess(Input{
		Stage: input.Stage, ExperimentalOptIn: input.ExperimentalOptIn,
		Requirements: []Requirement{input.Requirements[1], input.Requirements[0]},
		Evidence:     append([]Evidence(nil), input.Evidence...), Risks: append([]Risk(nil), input.Risks...),
	})
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := first.MarshalJSONStable()
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := second.MarshalJSONStable()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("assessment replay differs\nfirst:  %s\nsecond: %s", firstJSON, secondJSON)
	}
}

func TestAssessmentReassessmentCannotUpgradeConfidence(t *testing.T) {
	initial, err := Assess(Input{
		Stage:        "architecture_selection",
		Requirements: []Requirement{{Kind: RequirementArchitecture, ID: "gain_stage", EvidenceIDs: []string{"architecture"}}},
		Evidence:     []Evidence{{ID: "architecture", Kind: "provider", Status: EvidenceInferred, Source: "provider://gain_stage"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := Digest("verified later")
	if err != nil {
		t.Fatal(err)
	}
	next, err := Reassess(initial, CheckpointInput{
		Stage:        "simulation",
		Requirements: []Requirement{{Kind: RequirementModel, ID: "gain_model", EvidenceIDs: []string{"simulation"}}},
		Evidence:     []Evidence{{ID: "simulation", Kind: "simulation", Status: EvidenceVerified, Source: "simulation://gain_model", Digest: digest}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.Classification != ClassificationExperimental || next.FabricationReadyEligible {
		t.Fatalf("reassessment upgraded confidence: %#v", next)
	}
	failed, err := Reassess(next, CheckpointInput{
		Stage: "routing",
		Gaps:  []Gap{{Code: "ROUTING_FAILED", Kind: RequirementPhysical, ID: "complete_routing", Stage: "routing", Reason: "required route is incomplete"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed.Classification != ClassificationUnsupported {
		t.Fatalf("failed reassessment classification=%q, want unsupported", failed.Classification)
	}
}
