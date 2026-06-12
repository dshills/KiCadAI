package transactions

import (
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/reports"
)

func TestPlanSupportedGeneratedProject(t *testing.T) {
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":0,"y_mm":0}},
	  {"op":"connect","from":{"ref":"R1","pin":"1"},"to":{"ref":"J1","pin":"1"},"net_name":"SIG"},
	  {"op":"write_project"}
	]}`)
	plan := PlanTransaction(filepath.Join(t.TempDir(), "out"), tx)
	if len(plan.Issues) != 0 || len(plan.Operations) != 4 {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	if !plan.Operations[1].Supported || len(plan.Operations[1].Refs) != 1 || plan.Operations[1].Refs[0] != "R1" {
		t.Fatalf("unexpected add symbol plan: %#v", plan.Operations[1])
	}
	if !plan.Operations[3].WillWrite || len(plan.Operations[3].Artifacts) != 3 {
		t.Fatalf("write artifacts missing: %#v", plan.Operations[3])
	}
}

func TestPlanUnsupportedRemoval(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"remove_symbol","ref":"R1"}]}`)
	plan := PlanTransaction(t.TempDir(), tx)
	if len(plan.Issues) == 0 || plan.Operations[0].Supported {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestPlanExistingProjectBlocked(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.kicad_pro"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	tx := mustParse(t, `{"operations":[{"op":"create_project","name":"demo"}]}`)
	plan := PlanTransaction(dir, tx)
	found := false
	for _, issue := range plan.Issues {
		if issue.Path == "transaction.target" && issue.Severity == reports.SeverityBlocked {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected existing project blocked issue: %#v", plan.Issues)
	}
}

func TestPlanReportsDecodeIssue(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"place_footprint","ref":42}]}`)
	plan := PlanTransaction(t.TempDir(), tx)
	found := false
	for _, issue := range plan.Issues {
		if issue.Path == "operations[0]" && issue.Code == reports.CodeInvalidArgument {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected decode issue: %#v", plan.Issues)
	}
}

func TestPlanOperationIndexesPreserved(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"write_project"},{"op":"remove_symbol","ref":"R1"}]}`)
	plan := PlanTransaction(t.TempDir(), tx)
	if plan.Operations[1].Index != 1 || plan.Issues[0].Path != "operations[1].op" {
		t.Fatalf("indexes not preserved: %#v", plan)
	}
}
