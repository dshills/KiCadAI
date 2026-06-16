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
