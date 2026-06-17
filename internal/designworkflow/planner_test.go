package designworkflow

import (
	"context"
	"strings"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
)

func TestPlanBlocksComposesExplicitRequest(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "header", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []any{"SIG", "GND"}}},
			{ID: "status", BlockID: "led_indicator"},
		},
		Connections: []ConnectionSpec{{From: "header.SIG", To: "status.IN", NetAlias: "STATUS"}},
	}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("PlanBlocks issues = %#v", result.Stage.Issues)
	}
	if result.Stage.Status != StageStatusOK || len(result.Output.Operations) == 0 {
		t.Fatalf("result = %#v", result)
	}
	if result.Composition.ProjectName != "status_board" {
		t.Fatalf("project name = %q", result.Composition.ProjectName)
	}
	evidence, ok := result.Stage.Summary["block_evidence"].([]BlockEvidenceSummary)
	if !ok || len(evidence) != 2 {
		t.Fatalf("block evidence = %#v", result.Stage.Summary["block_evidence"])
	}
	if evidence[1].BlockID != "led_indicator" || evidence[1].EvidenceLevel == "" {
		t.Fatalf("LED evidence = %#v", evidence[1])
	}
}

func TestPlanBlocksWarnsForMissingBlockEvidence(t *testing.T) {
	registry := testDesignRegistry{definition: testBlockDefinition("custom_block", blocks.VerificationStructural)}
	request := Request{
		Version: RequestVersion,
		Name:    "custom",
		Board:   BoardSpec{WidthMM: 10, HeightMM: 10, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "custom", BlockID: "custom_block"}},
	}
	result := PlanBlocks(context.Background(), registry, request)
	if reports.HasBlockingIssue(result.Stage.Issues) || !containsIssueMessage(result.Stage.Issues, "no built-in verification evidence") {
		t.Fatalf("stage = %#v", result.Stage)
	}
}

func TestPlanBlocksBlocksFabricationClaimWithoutStrongEvidence(t *testing.T) {
	registry := testDesignRegistry{definition: testBlockDefinition("custom_fab", blocks.VerificationERCDRCVerified)}
	request := Request{
		Version:    RequestVersion,
		Name:       "custom",
		Board:      BoardSpec{WidthMM: 10, HeightMM: 10, Layers: 2},
		Blocks:     []BlockInstanceSpec{{ID: "custom", BlockID: "custom_fab"}},
		Validation: ValidationSpec{Acceptance: AcceptanceFabricationCandidate},
	}
	result := PlanBlocks(context.Background(), registry, request)
	if !reports.HasBlockingIssue(result.Stage.Issues) || !containsIssueMessage(result.Stage.Issues, "fabrication readiness claim lacks") {
		t.Fatalf("stage = %#v", result.Stage)
	}
}

func TestPlanBlocksReportsUnknownBlock(t *testing.T) {
	request := validRequest()
	request.Blocks[0].BlockID = "missing"
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	if !reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("issues = %#v, want blocking", result.Stage.Issues)
	}
	assertIssueCode(t, result.Stage.Issues, reports.CodeMissingFile)
}

func TestPlanBlocksReportsUnknownPortFromComposition(t *testing.T) {
	request := validRequest()
	request.Connections[0].From = "sensor.NOPE"
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	if !reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("issues = %#v, want blocking", result.Stage.Issues)
	}
	if !containsIssueMessage(result.Stage.Issues, "unknown port sensor.NOPE") {
		t.Fatalf("issues = %#v", result.Stage.Issues)
	}
}

func TestPlanBlocksReportsConflictingVoltageDomains(t *testing.T) {
	request := validRequest()
	request.Blocks = []BlockInstanceSpec{
		{ID: "reg_a", BlockID: "voltage_regulator", Params: map[string]any{"output_voltage": "3.3V"}},
		{ID: "reg_b", BlockID: "voltage_regulator", Params: map[string]any{"output_voltage": "1.8V"}},
	}
	request.Connections = []ConnectionSpec{{From: "reg_a.VOUT", To: "reg_b.VOUT"}}
	result := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	if !reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("issues = %#v, want blocking", result.Stage.Issues)
	}
	if !containsIssueMessage(result.Stage.Issues, "conflicting voltage domains") {
		t.Fatalf("issues = %#v", result.Stage.Issues)
	}
}

func assertIssueCode(t *testing.T, issues []reports.Issue, code reports.Code) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("missing issue code %q in %#v", code, issues)
}

func containsIssueMessage(issues []reports.Issue, text string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.Message, text) {
			return true
		}
	}
	return false
}

type testDesignRegistry struct {
	definition blocks.BlockDefinition
}

func (registry testDesignRegistry) ListBlocks() []blocks.BlockSummary {
	return []blocks.BlockSummary{testBlockSummary(registry.definition)}
}

func (registry testDesignRegistry) GetBlock(id string) (blocks.BlockDefinition, bool) {
	return registry.definition, id == registry.definition.ID
}

func (registry testDesignRegistry) ValidateDefinition(definition blocks.BlockDefinition) []reports.Issue {
	return nil
}

func (registry testDesignRegistry) ValidateRequest(request blocks.BlockRequest) []reports.Issue {
	if request.BlockID != registry.definition.ID {
		return []reports.Issue{{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Message: "missing"}}
	}
	return nil
}

func (registry testDesignRegistry) Instantiate(ctx context.Context, request blocks.BlockRequest) (blocks.BlockOutput, []reports.Issue) {
	return blocks.BlockOutput{
		Definition: testBlockSummary(registry.definition),
		Instance: blocks.BlockInstance{
			BlockID:    request.BlockID,
			InstanceID: request.InstanceID,
			Params:     request.Params,
		},
	}, nil
}

func testBlockSummary(definition blocks.BlockDefinition) blocks.BlockSummary {
	return blocks.BlockSummary{
		ID:                definition.ID,
		Name:              definition.Name,
		Description:       definition.Description,
		Version:           definition.Version,
		Category:          definition.Category,
		VerificationLevel: definition.Verification.Level,
	}
}

func testBlockDefinition(id string, level blocks.VerificationLevel) blocks.BlockDefinition {
	return blocks.BlockDefinition{
		ID:          id,
		Name:        id,
		Version:     "0.1.0",
		Category:    "test",
		Description: "test block",
		Verification: blocks.VerificationRecord{
			Level: level,
		},
	}
}
