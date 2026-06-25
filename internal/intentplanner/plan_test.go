package intentplanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
)

func TestNewPlanBlocksInvalidRequest(t *testing.T) {
	plan := NewPlan(Request{Version: "0.1.0", Name: "bad", Kind: IntentKind("nope")})
	if plan.Status != PlanStatusBlocked {
		t.Fatalf("status = %s, want blocked; plan=%#v", plan.Status, plan)
	}
	if plan.Score != 0 {
		t.Fatalf("score = %d, want 0", plan.Score)
	}
	if len(plan.Issues) == 0 {
		t.Fatalf("issues missing")
	}
}

func TestNormalizePlanSortsAndInitializesCollections(t *testing.T) {
	plan := NormalizePlan(PlanResult{
		Requirements: []RequirementRecord{
			{ID: "b", Path: "b"},
			{ID: "a", Path: "a"},
		},
		SelectedBlocks: []SelectedBlockRecord{
			{InstanceID: "z", BlockID: "led_indicator"},
			{InstanceID: "a", BlockID: "connector_breakout"},
		},
		Assumptions: []PlanNote{{ID: "z"}, {ID: "a"}},
		Issues: []reports.Issue{
			{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: "z", Message: "z"},
			{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: "a", Message: "a"},
		},
	})
	if plan.Schema != PlanSchema {
		t.Fatalf("schema = %q", plan.Schema)
	}
	if plan.Requirements[0].ID != "a" || plan.SelectedBlocks[0].InstanceID != "a" || plan.Assumptions[0].ID != "a" || plan.Issues[0].Path != "a" {
		t.Fatalf("plan not sorted: %#v", plan)
	}
	if plan.Connections == nil || plan.Artifacts == nil || plan.KnownGaps == nil {
		t.Fatalf("collections should be initialized: %#v", plan)
	}
	if plan.Status != PlanStatusPartial {
		t.Fatalf("status = %s, want partial for warning issue", plan.Status)
	}
}

func TestMarshalPlanJSONStable(t *testing.T) {
	plan := PlanResult{
		Intent: PlanIntentSummary{Name: "demo", Kind: IntentBreakout},
		Requirements: []RequirementRecord{
			{ID: "req.power", Path: "power", Type: "power", Strength: StrengthRequired},
		},
	}
	first, err := MarshalPlanJSON(plan)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalPlanJSON(plan)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("marshal unstable:\n%s\n---\n%s", first, second)
	}
	if !strings.Contains(string(first), `"schema": "kicadai.intent.plan.v1"`) {
		t.Fatalf("schema missing from %s", first)
	}
}

func TestWriteArtifactsWritesPlanAndBlocksOverwrite(t *testing.T) {
	root := t.TempDir()
	plan := NormalizePlan(PlanResult{Intent: PlanIntentSummary{Name: "demo", Kind: IntentBreakout}})
	written, issues := WriteArtifacts(plan, ArtifactOptions{OutputDir: root})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if _, err := os.Stat(filepath.Join(root, "intent-plan.json")); err != nil {
		t.Fatalf("intent-plan.json missing: %v", err)
	}
	if len(written.Artifacts) != 1 || written.Artifacts[0].Path != "intent-plan.json" {
		t.Fatalf("artifacts = %#v", written.Artifacts)
	}
	_, issues = WriteArtifacts(plan, ArtifactOptions{OutputDir: root})
	if len(issues) == 0 {
		t.Fatalf("expected overwrite issue")
	}
}

func TestWriteArtifactsWritesGeneratedRequest(t *testing.T) {
	root := t.TempDir()
	workflowRequest := designworkflow.Request{
		Version: designworkflow.RequestVersion,
		Name:    "demo",
		Board:   designworkflow.BoardSpec{WidthMM: 10, HeightMM: 10, Layers: 2},
	}
	plan := NormalizePlan(PlanResult{
		Intent:           PlanIntentSummary{Name: "demo", Kind: IntentBreakout},
		GeneratedRequest: &workflowRequest,
	})
	written, issues := WriteArtifacts(plan, ArtifactOptions{OutputDir: root})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	data, err := os.ReadFile(filepath.Join(root, "generated-request.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["name"] != "demo" {
		t.Fatalf("generated request = %#v", decoded)
	}
	if len(written.Artifacts) != 2 {
		t.Fatalf("artifacts = %#v", written.Artifacts)
	}
}
