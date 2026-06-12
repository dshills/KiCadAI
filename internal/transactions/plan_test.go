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
		if issue.Path == "operations[0].op" && issue.Severity == reports.SeverityBlocked {
			found = true
		}
	}
	if !found || plan.Operations[0].Supported {
		t.Fatalf("expected create_project blocked for existing project: %#v", plan)
	}
}

func TestPlanExistingProjectAllowsSafeAddSymbol(t *testing.T) {
	dir := writeExistingProject(t, `(kicad_sch (version 20260306) (generator "kicadai"))`, `(kicad_pcb (version 20260206) (generator "pcbnew") (layers (0 "F.Cu" signal)))`)
	tx := mustParse(t, `{"operations":[{"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":10}}]}`)
	plan := PlanTransaction(dir, tx)
	if len(plan.Issues) != 0 || !plan.Operations[0].Supported {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestPlanExistingProjectBlocksUnsupportedRawContent(t *testing.T) {
	dir := writeExistingProject(t, `(kicad_sch (version 20260306) (generator "kicadai") (rule_area (uuid "22222222-2222-5222-8222-222222222222")))`, `(kicad_pcb (version 20260206) (generator "pcbnew") (layers (0 "F.Cu" signal)))`)
	tx := mustParse(t, `{"operations":[{"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":10}}]}`)
	plan := PlanTransaction(dir, tx)
	if len(plan.Issues) == 0 || plan.Issues[0].Code != reports.CodePreservationConflict {
		t.Fatalf("expected preservation conflict: %#v", plan.Issues)
	}
}

func TestPlanExistingProjectBlocksUnsafeRemove(t *testing.T) {
	dir := writeExistingProject(t, `(kicad_sch (version 20260306) (generator "kicadai") (symbol (property "Reference" "R1") (uuid "11111111-1111-5111-8111-111111111111")))`, `(kicad_pcb (version 20260206) (generator "pcbnew") (layers (0 "F.Cu" signal)))`)
	tx := mustParse(t, `{"operations":[{"op":"remove_symbol","ref":"R1"}]}`)
	plan := PlanTransaction(dir, tx)
	found := false
	for _, issue := range plan.Issues {
		if issue.Code == reports.CodeUnsafeRemove {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unsafe remove issue: %#v", plan.Issues)
	}
}

func TestPlanExistingProjectBlocksAmbiguousReference(t *testing.T) {
	dir := writeExistingProject(t, `(kicad_sch (version 20260306) (generator "kicadai")
	  (symbol (property "Reference" "R1") (uuid "11111111-1111-5111-8111-111111111111"))
	  (symbol (property "Reference" "R1") (uuid "22222222-2222-5222-8222-222222222222"))
	)`, `(kicad_pcb (version 20260206) (generator "pcbnew") (layers (0 "F.Cu" signal)))`)
	tx := mustParse(t, `{"operations":[{"op":"assign_footprint","ref":"R1","footprint_id":"Device:R"}]}`)
	plan := PlanTransaction(dir, tx)
	if len(plan.Issues) == 0 || plan.Issues[0].Code != reports.CodeAmbiguousReference {
		t.Fatalf("expected ambiguous reference issue: %#v", plan.Issues)
	}
}

func TestPlanExistingProjectBlocksMissingReference(t *testing.T) {
	dir := writeExistingProject(t, `(kicad_sch (version 20260306) (generator "kicadai"))`, `(kicad_pcb (version 20260206) (generator "pcbnew") (layers (0 "F.Cu" signal)))`)
	tx := mustParse(t, `{"operations":[{"op":"assign_footprint","ref":"R404","footprint_id":"Device:R"}]}`)
	plan := PlanTransaction(dir, tx)
	if len(plan.Issues) == 0 || plan.Issues[0].Code != reports.CodeInvalidArgument {
		t.Fatalf("expected missing reference issue: %#v", plan.Issues)
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

func writeExistingProject(t *testing.T, schematicContents string, pcbContents string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.kicad_pro"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo.kicad_sch"), []byte(schematicContents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo.kicad_pcb"), []byte(pcbContents), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
