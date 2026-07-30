package designworkflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/capabilitygate"
)

func TestCapabilityAssessmentClassifiesPromotedBlockSupported(t *testing.T) {
	request := Request{
		Name:   "supported",
		Blocks: []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	assessment, err := assessBlockRequestCapability(context.Background(), blocks.NewBuiltinRegistry(), request, false)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Classification != capabilitygate.ClassificationSupported {
		t.Fatalf("classification = %q gaps=%#v risks=%#v", assessment.Classification, assessment.Gaps, assessment.Risks)
	}
	if assessment.Hash == "" || !assessment.FabricationReadyEligible {
		t.Fatalf("supported assessment missing reproducible eligibility: %#v", assessment)
	}
}

func TestCapabilityAssessmentRequiresOptInForUnpromotedBlock(t *testing.T) {
	definition, ok := blocks.NewBuiltinRegistry().GetBlock("led_indicator")
	if !ok {
		t.Fatal("missing led_indicator definition")
	}
	definition.ID = "unpromoted_indicator"
	registry := blocks.NewRegistry([]blocks.BlockDefinition{definition})
	request := Request{
		Name:   "experimental",
		Blocks: []BlockInstanceSpec{{ID: "status", BlockID: definition.ID}},
	}

	assessment, err := assessBlockRequestCapability(context.Background(), registry, request, false)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Classification != capabilitygate.ClassificationExperimental {
		t.Fatalf("classification = %q, want experimental: %#v", assessment.Classification, assessment)
	}
	issues := capabilityGateIssues(assessment)
	if len(issues) != 1 || issues[0].Code != CodeCapabilityExperimentalOptIn {
		t.Fatalf("gate issues = %#v", issues)
	}

	optedIn, err := assessBlockRequestCapability(context.Background(), registry, request, true)
	if err != nil {
		t.Fatal(err)
	}
	if issues := capabilityGateIssues(optedIn); len(issues) != 0 {
		t.Fatalf("opted-in gate issues = %#v", issues)
	}
	result := BuildWorkflowResult(ProjectSummary{Name: request.Name}, AcceptanceFabricationCandidate, []StageResult{
		{Name: StageFabricationReady, Status: StageStatusOK},
	})
	result = applyWorkflowCapability(result, optedIn)
	if result.Acceptance.FabricationReady || result.Acceptance.Achieved == AcceptanceFabricationCandidate {
		t.Fatalf("experimental result received fabrication-ready status: %#v", result.Acceptance)
	}
}

func TestCapabilityAssessmentFailsClosedWhenBlockEvidenceCannotLoad(t *testing.T) {
	t.Setenv("KICADAI_BLOCK_VERIFICATION_ROOT", filepath.Join(t.TempDir(), "missing"))
	definition, ok := blocks.NewBuiltinRegistry().GetBlock("led_indicator")
	if !ok {
		t.Fatal("missing led_indicator definition")
	}
	definition.ID = "evidence_load_failure_indicator"
	registry := blocks.NewRegistry([]blocks.BlockDefinition{definition})
	request := Request{Name: "evidence-load-failure", Blocks: []BlockInstanceSpec{{ID: "status", BlockID: definition.ID}}}

	assessment, err := assessBlockRequestCapability(context.Background(), registry, request, false)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Classification != capabilitygate.ClassificationUnsupported {
		t.Fatalf("classification = %q, want unsupported: %#v", assessment.Classification, assessment)
	}
	if len(assessment.Gaps) == 0 || assessment.Gaps[0].Reason == "" {
		t.Fatalf("assessment did not retain evidence-load failure: %#v", assessment)
	}
	if len(assessment.Evidence) == 0 || assessment.Evidence[0].Description == "" {
		t.Fatalf("assessment did not retain typed evidence-load diagnostic: %#v", assessment)
	}
}

func TestCreateRefusesUnsupportedRequestBeforeFilesystemMutation(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "must-not-exist")
	request := Request{
		Name:   "unsupported",
		Blocks: []BlockInstanceSpec{{ID: "mystery", BlockID: "not_registered"}},
	}
	result := Create(context.Background(), request, CreateOptions{
		OutputDir: outputDir, BlockRegistry: blocks.NewBuiltinRegistry(),
	})
	if result.Capability == nil || result.Capability.Classification != capabilitygate.ClassificationUnsupported {
		t.Fatalf("capability = %#v", result.Capability)
	}
	if len(result.Stages) == 0 || result.Stages[0].Name != StageCapabilityAssessment || result.Stages[0].Status != StageStatusBlocked {
		t.Fatalf("stages = %#v", result.Stages)
	}
	if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
		t.Fatalf("capability refusal mutated output path: stat err=%v", err)
	}
}

func TestWorkflowCapabilityOnlyDecreasesAcrossCheckpoints(t *testing.T) {
	digest, err := capabilitygate.Digest("promoted")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := capabilitygate.Assess(capabilitygate.Input{
		Stage: "architecture_selection",
		Requirements: []capabilitygate.Requirement{{
			Kind: capabilitygate.RequirementArchitecture, ID: "gain_stage", EvidenceIDs: []string{"architecture"},
		}},
		Evidence: []capabilitygate.Evidence{{
			ID: "architecture", Kind: "promotion", Status: capabilitygate.EvidenceVerified,
			Source: "promotion://gain_stage", Digest: digest,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "failed"}, AcceptanceFabricationCandidate, []StageResult{
		{Name: StageRouting, Status: StageStatusBlocked},
		{Name: StageFabricationReady, Status: StageStatusOK},
	})
	result = applyWorkflowCapability(result, initial)
	if result.Capability == nil || result.Capability.Classification != capabilitygate.ClassificationUnsupported {
		t.Fatalf("downstream failure did not downgrade capability: %#v", result.Capability)
	}
	if result.Acceptance.FabricationReady {
		t.Fatalf("downgraded workflow retained fabrication-ready status: %#v", result.Acceptance)
	}
}
