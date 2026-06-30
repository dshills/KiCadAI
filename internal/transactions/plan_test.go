package transactions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/libraryresolver"
	"kicadai/internal/preservation"
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
	if !strings.HasPrefix(plan.Operations[1].ID, "op-add-symbol-ref-r1-") || !strings.HasPrefix(plan.Operations[2].ID, "op-connect-ref-r1-") {
		t.Fatalf("unexpected operation ids: %#v", plan.Operations)
	}
	if !plan.Operations[3].WillWrite || len(plan.Operations[3].Artifacts) != 3 {
		t.Fatalf("write artifacts missing: %#v", plan.Operations[3])
	}
}

func TestPlanOperationIDsUseNetWhenNoRef(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"route","net_name":"I2C SDA","points":[{"x_mm":0,"y_mm":0},{"x_mm":1,"y_mm":1}]}]}`)
	plan := PlanTransaction(t.TempDir(), tx)
	if len(plan.Operations) != 1 || !strings.HasPrefix(plan.Operations[0].ID, "op-route-net-i2c-sda-") {
		t.Fatalf("unexpected operation ids: %#v", plan.Operations)
	}
}

func TestPlanOperationIDIsJSONVisible(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"write_project"}]}`)
	plan := PlanTransaction(t.TempDir(), tx)
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) || !strings.Contains(string(data), `"id":"op-write-project-`) {
		t.Fatalf("operation id missing from json: %s", string(data))
	}
}

func TestPlanOperationIDsAreContentStableAcrossReorder(t *testing.T) {
	first := mustParse(t, `{"operations":[
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":0,"y_mm":0}},
	  {"op":"add_symbol","ref":"C1","library_id":"Device:C","at":{"x_mm":1,"y_mm":0}}
	]}`)
	second := mustParse(t, `{"operations":[
	  {"op":"add_symbol","ref":"C1","library_id":"Device:C","at":{"x_mm":1,"y_mm":0}},
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":0,"y_mm":0}}
	]}`)
	firstPlan := PlanTransaction(t.TempDir(), first)
	secondPlan := PlanTransaction(t.TempDir(), second)
	if firstPlan.Operations[0].ID != secondPlan.Operations[1].ID || firstPlan.Operations[1].ID != secondPlan.Operations[0].ID {
		t.Fatalf("operation ids changed across reorder: first=%#v second=%#v", firstPlan.Operations, secondPlan.Operations)
	}
}

func TestPlanOperationIDsCanonicalizeJSONFormatting(t *testing.T) {
	compact := mustParse(t, `{"operations":[{"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":0,"y_mm":0}}]}`)
	spaced := mustParse(t, `{
	  "operations": [
	    {
	      "op": "add_symbol",
	      "ref": "R1",
	      "library_id": "Device:R",
	      "at": {"x_mm": 0, "y_mm": 0}
	    }
	  ]
	}`)
	compactPlan := PlanTransaction(t.TempDir(), compact)
	spacedPlan := PlanTransaction(t.TempDir(), spaced)
	if compactPlan.Operations[0].ID != spacedPlan.Operations[0].ID {
		t.Fatalf("operation ids differ by JSON formatting: compact=%#v spaced=%#v", compactPlan.Operations, spacedPlan.Operations)
	}
}

func TestPlanOperationIDsCanonicalizeJSONKeyOrder(t *testing.T) {
	first := mustParse(t, `{"operations":[{"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":0,"y_mm":0}}]}`)
	second := mustParse(t, `{"operations":[{"at":{"y_mm":0,"x_mm":0},"library_id":"Device:R","ref":"R1","op":"add_symbol"}]}`)
	firstPlan := PlanTransaction(t.TempDir(), first)
	secondPlan := PlanTransaction(t.TempDir(), second)
	if firstPlan.Operations[0].ID != secondPlan.Operations[0].ID {
		t.Fatalf("operation ids differ by JSON key order: first=%#v second=%#v", firstPlan.Operations, secondPlan.Operations)
	}
}

func TestPlanOperationIDsDisambiguateDuplicateContent(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"write_project"},{"op":"write_project"}]}`)
	plan := PlanTransaction(t.TempDir(), tx)
	if len(plan.Operations) != 2 || plan.Operations[0].ID == plan.Operations[1].ID || !strings.HasSuffix(plan.Operations[1].ID, "-n1") {
		t.Fatalf("duplicate operation ids not disambiguated: %#v", plan.Operations)
	}
}

func TestUniquePlannedOperationIDAvoidsGeneratedSuffixCollision(t *testing.T) {
	seen := map[string]struct{}{}
	counts := map[string]int{}
	if got := uniquePlannedOperationID("op-write-project", seen, counts); got != "op-write-project" {
		t.Fatalf("first id = %q", got)
	}
	if got := uniquePlannedOperationID("op-write-project-n1", seen, counts); got != "op-write-project-n1" {
		t.Fatalf("natural suffixed id = %q", got)
	}
	if got := uniquePlannedOperationID("op-write-project", seen, counts); got != "op-write-project-n2" {
		t.Fatalf("collision-safe duplicate id = %q", got)
	}
}

func TestPlanUnsupportedRemoval(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"remove_symbol","ref":"R1"}]}`)
	plan := PlanTransaction(t.TempDir(), tx)
	if len(plan.Issues) == 0 || plan.Operations[0].Supported {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	if plan.Issues[0].OperationID == "" || plan.Issues[0].OperationID != plan.Operations[0].ID {
		t.Fatalf("unsupported issue missing operation id: issue=%#v operation=%#v", plan.Issues[0], plan.Operations[0])
	}
}

func TestAnnotatePlanIssueOperationIDsUsesSliceIndex(t *testing.T) {
	plan := Plan{
		Operations: []PlannedOperation{
			{ID: "op-first", Index: 10},
			{ID: "op-second", Index: 10},
		},
		Issues: []reports.Issue{{Path: "operations[1].ref"}},
	}
	annotatePlanIssueOperationIDs(&plan)
	if plan.Issues[0].OperationID != "op-second" {
		t.Fatalf("operation id = %q, want op-second", plan.Issues[0].OperationID)
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
	if plan.Preservation == nil || len(plan.Preservation.OperationReviews) != 1 ||
		plan.Preservation.OperationReviews[0].Mutability != preservation.MutabilitySafeAdd ||
		plan.Preservation.OperationReviews[0].Status != preservation.StatusClean {
		t.Fatalf("unexpected preservation review: %#v", plan.Preservation)
	}
}

func TestPlanExistingProjectBlocksUnsupportedRawContent(t *testing.T) {
	dir := writeExistingProject(t, `(kicad_sch (version 20260306) (generator "kicadai") (rule_area (uuid "22222222-2222-5222-8222-222222222222")))`, `(kicad_pcb (version 20260206) (generator "pcbnew") (layers (0 "F.Cu" signal)))`)
	tx := mustParse(t, `{"operations":[{"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":10}}]}`)
	plan := PlanTransaction(dir, tx)
	if len(plan.Issues) == 0 || plan.Issues[0].Code != reports.CodePreservationConflict {
		t.Fatalf("expected preservation conflict: %#v", plan.Issues)
	}
	if plan.Preservation == nil || len(plan.Preservation.OperationReviews) != 1 ||
		plan.Preservation.OperationReviews[0].Mutability != preservation.MutabilityUnsafe ||
		plan.Preservation.OperationReviews[0].Status != preservation.StatusBlocked {
		t.Fatalf("unexpected preservation review: %#v", plan.Preservation)
	}
}

func TestPlanExistingProjectBlocksAddSymbolExistingReference(t *testing.T) {
	dir := writeExistingProject(t, `(kicad_sch (version 20260306) (generator "kicadai")
	  (symbol (property "Reference" "R1") (uuid "11111111-1111-5111-8111-111111111111"))
	)`, `(kicad_pcb (version 20260206) (generator "pcbnew") (layers (0 "F.Cu" signal)))`)
	tx := mustParse(t, `{"operations":[{"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":10}}]}`)
	plan := PlanTransaction(dir, tx)
	if len(plan.Issues) == 0 || plan.Issues[0].Code != reports.CodeDuplicateReference {
		t.Fatalf("expected duplicate reference issue: %#v", plan.Issues)
	}
	if plan.Preservation == nil || plan.Preservation.OperationReviews[0].Mutability != preservation.MutabilityUnsafe {
		t.Fatalf("expected unsafe preservation review: %#v", plan.Preservation)
	}
}

func TestPlanExistingProjectBlocksDuplicateAddSymbolWithoutUnit(t *testing.T) {
	dir := writeExistingProject(t, `(kicad_sch (version 20260306) (generator "kicadai"))`, `(kicad_pcb (version 20260206) (generator "pcbnew") (layers (0 "F.Cu" signal)))`)
	tx := mustParse(t, `{"operations":[
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":10}},
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":20,"y_mm":10}}
	]}`)
	plan := PlanTransaction(dir, tx)
	if len(plan.Issues) == 0 || plan.Issues[0].Code != reports.CodeDuplicateReference {
		t.Fatalf("expected duplicate transaction reference issue: %#v", plan.Issues)
	}
	if plan.Preservation == nil || plan.Preservation.OperationReviews[1].Mutability != preservation.MutabilityUnsafe {
		t.Fatalf("expected unsafe second operation review: %#v", plan.Preservation)
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

func TestPlanWithResolverReportsBadSymbolID(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"add_symbol","ref":"R1","library_id":"Device:Missing","at":{"x_mm":0,"y_mm":0}}]}`)
	index := transactionResolverFixture()
	plan := PlanTransactionWithOptions(t.TempDir(), tx, PlanOptions{LibraryIndex: &index})
	if len(plan.Issues) == 0 || plan.Issues[0].Code != reports.CodeMissingFile || plan.Issues[0].Path != "operations[0].library_id" {
		t.Fatalf("expected bad symbol issue: %#v", plan.Issues)
	}
	if plan.Issues[0].OperationID != plan.Operations[0].ID {
		t.Fatalf("library issue missing operation id: issue=%#v operation=%#v", plan.Issues[0], plan.Operations[0])
	}
}

func TestPlanWithResolverReportsBadFootprintID(t *testing.T) {
	tx := mustParse(t, `{"operations":[
	  {"op":"assign_footprint","ref":"R1","footprint_id":"Missing:Nope"},
	  {"op":"place_footprint","ref":"R1","footprint_id":"Missing:Nope","at":{"x_mm":0,"y_mm":0}}
	]}`)
	index := transactionResolverFixture()
	plan := PlanTransactionWithOptions(t.TempDir(), tx, PlanOptions{LibraryIndex: &index})
	if len(plan.Issues) != 2 || plan.Issues[0].Path != "operations[0].footprint_id" || plan.Issues[1].Path != "operations[1].footprint_id" {
		t.Fatalf("expected bad footprint issues: %#v", plan.Issues)
	}
}

func TestPlanWithResolverAcceptsValidLibraries(t *testing.T) {
	tx := mustParse(t, `{"operations":[
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":0,"y_mm":0}},
	  {"op":"assign_footprint","ref":"R1","footprint_id":"Resistor_SMD:R_0805_2012Metric"},
	  {"op":"place_footprint","ref":"R1","footprint_id":"Resistor_SMD:R_0805_2012Metric","at":{"x_mm":0,"y_mm":0}}
	]}`)
	index := transactionResolverFixture()
	plan := PlanTransactionWithOptions(t.TempDir(), tx, PlanOptions{LibraryIndex: &index})
	if len(plan.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", plan.Issues)
	}
	if plan.Operations[0].Capability == "" || plan.Operations[2].Capability == "" {
		t.Fatalf("expected resolver suggestions/capabilities: %#v", plan.Operations)
	}
}

func TestPlanWithRequiredResolverWithoutIndexWarns(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":0,"y_mm":0}}]}`)
	plan := PlanTransactionWithOptions(t.TempDir(), tx, PlanOptions{RequireLibraryValidation: true})
	if len(plan.Issues) != 1 || plan.Issues[0].Severity != reports.SeverityWarning {
		t.Fatalf("expected missing resolver warning: %#v", plan.Issues)
	}
}

func transactionResolverFixture() libraryresolver.LibraryIndex {
	return libraryresolver.LibraryIndex{
		Symbols: map[string]libraryresolver.SymbolRecord{
			"Device:R": {
				LibraryID:       "Device:R",
				LibraryNickname: "Device",
				Name:            "R",
				Description:     "Resistor",
				Keywords:        []string{"resistor"},
				FootprintFilter: []string{"R_*", "Resistor_*"},
				Pins: []libraryresolver.SymbolPin{
					{Number: "1", Electrical: "passive"},
					{Number: "2", Electrical: "passive"},
				},
			},
		},
		Footprints: map[string]libraryresolver.FootprintRecord{
			"Resistor_SMD:R_0805_2012Metric": {
				FootprintID:     "Resistor_SMD:R_0805_2012Metric",
				LibraryNickname: "Resistor_SMD",
				Name:            "R_0805_2012Metric",
				Description:     "resistor",
				Tags:            []string{"resistor"},
				Pads: []libraryresolver.FootprintPad{
					{Name: "1"},
					{Name: "2"},
				},
			},
		},
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
