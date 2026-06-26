package rationale

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/intentdraft"
	"kicadai/internal/intentplanner"
	"kicadai/internal/reports"
)

func TestBuildFromPlanMinimalDeterministic(t *testing.T) {
	request := intentplanner.Request{
		Version:    intentplanner.RequestVersion,
		Name:       "sensor",
		Kind:       intentplanner.IntentSensorNode,
		Acceptance: designworkflow.AcceptanceConnectivity,
		Functions:  []intentplanner.FunctionIntent{{Kind: "sensor", Family: "i2c_sensor"}},
		Interfaces: []intentplanner.InterfaceIntent{{Kind: "i2c"}},
	}
	plan := intentplanner.Plan(request)
	report := BuildFromPlan(plan, SourceSummary{Mode: "request", Path: "request.json"})
	if report.Schema != Schema {
		t.Fatalf("schema = %q", report.Schema)
	}
	if report.Status == "" {
		t.Fatalf("status was empty")
	}
	first, err := MarshalJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("marshal output is not deterministic")
	}
	if !strings.Contains(string(first), `"schema": "kicadai.design.rationale.v1"`) {
		t.Fatalf("missing schema in JSON: %s", first)
	}
}

func TestBuildFromDraftAndPlanBlockingClarification(t *testing.T) {
	draft := intentdraft.Draft("make a battery sensor board", intentdraft.Options{})
	plan := intentplanner.Plan(draft.Request)
	report := BuildFromDraftAndPlan(draft, plan, SourceSummary{Mode: "text"})
	if report.Status != StatusNeedsClarification {
		t.Fatalf("status = %q, want %q", report.Status, StatusNeedsClarification)
	}
	if len(report.Clarifications) == 0 {
		t.Fatalf("expected clarifications")
	}
	found := false
	for _, limit := range report.KnownLimits {
		if limit.Category == "validation_blocked" || limit.Category == "unsupported_intent" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected clarification known limit, got %#v", report.KnownLimits)
	}
}

func TestBuildWithWorkflowSummary(t *testing.T) {
	issue := reports.Issue{
		Code:       reports.CodeMissingFootprint,
		Severity:   reports.SeverityError,
		Path:       "blocks.u1",
		Message:    "missing footprint",
		Suggestion: "assign a verified footprint",
	}
	workflow := designworkflow.BuildWorkflowResult(
		designworkflow.ProjectSummary{Name: "demo"},
		designworkflow.AcceptanceERCDRC,
		[]designworkflow.StageResult{
			designworkflow.NewStageResult(designworkflow.StageSchematic, nil),
			designworkflow.NewStageResult(designworkflow.StageComponentSelection, []reports.Issue{issue}),
		},
	)
	report := Build(BuildOptions{
		Source:   SourceSummary{Mode: "request"},
		Workflow: &workflow,
	})
	if report.Status != StatusBlocked {
		t.Fatalf("status = %q", report.Status)
	}
	if report.Validation.BlockingCount == 0 {
		t.Fatalf("expected blocking count")
	}
	if len(report.NextActions) == 0 {
		t.Fatalf("expected next actions")
	}
}

func TestLoadFromTarget(t *testing.T) {
	dir := t.TempDir()
	meta := filepath.Join(dir, MetadataDirName)
	if err := os.MkdirAll(meta, 0o755); err != nil {
		t.Fatal(err)
	}
	request := intentplanner.Request{Version: intentplanner.RequestVersion, Name: "demo", Kind: intentplanner.IntentBreakout}
	plan := intentplanner.Plan(request)
	writeJSON(t, filepath.Join(meta, "intent-plan.json"), plan)
	result := LoadFromTarget(dir)
	if reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	if result.Report.Source.Mode != "target" {
		t.Fatalf("source mode = %q", result.Report.Source.Mode)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
