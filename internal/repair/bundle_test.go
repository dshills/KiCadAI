package repair

import (
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestParseBundleLoadsValidBundle(t *testing.T) {
	bundle := parseBundle(t, `{
	  "schema":"kicadai.repair.bundle.v1",
	  "project_root":"out/demo",
	  "project_name":"demo",
	  "generated":true,
	  "transaction":{"operations":[{"op":"create_project","name":"demo"},{"op":"write_project"}]},
	  "stage_issues":[{"stage":"validation","issues":[{"code":"MISSING_FOOTPRINT","severity":"error","message":"missing","refs":["R1"],"nets":["SIG"]}]}]
	}`)
	if bundle.Schema != BundleSchemaV1 || !bundle.Generated || bundle.Transaction == nil {
		t.Fatalf("bundle = %#v", bundle)
	}
	if got := BundleIssues(bundle); len(got) != 1 || got[0].Refs[0] != "R1" || got[0].Nets[0] != "SIG" {
		t.Fatalf("issues = %#v", got)
	}
}

func TestSaveBundleRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repair.json")
	tx := transactions.Transaction{Operations: []transactions.Operation{
		mustRepairOperation(t, transactions.OpCreateProject, transactions.CreateProjectOperation{Op: transactions.OpCreateProject, Name: "demo"}, ""),
		mustRepairOperation(t, transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject}, ""),
	}}
	err := SaveBundle(path, Bundle{
		Schema:      BundleSchemaV1,
		ProjectRoot: filepath.Join("out", "demo"),
		Generated:   true,
		Transaction: &tx,
		StageIssues: []StageIssues{{Stage: "validation", Issues: []reports.Issue{{Code: reports.CodeMissingBoardOutline, Message: "missing outline"}}}},
	})
	if err != nil {
		t.Fatalf("SaveBundle returned error: %v", err)
	}
	loaded, err := LoadBundle(path)
	if err != nil {
		t.Fatalf("LoadBundle returned error: %v", err)
	}
	if loaded.ProjectRoot != "out/demo" || loaded.Transaction == nil || len(loaded.StageIssues) != 1 {
		t.Fatalf("loaded = %#v", loaded)
	}
}

func TestParseBundleRejectsUnsupportedSchema(t *testing.T) {
	if _, err := ParseBundle([]byte(`{"schema":"kicadai.repair.bundle.v2"}`)); err == nil {
		t.Fatal("expected unsupported schema error")
	}
}

func TestParseBundleRejectsMissingSchema(t *testing.T) {
	if _, err := ParseBundle([]byte(`{"generated":true}`)); err == nil {
		t.Fatal("expected missing schema error")
	}
}

func TestParseBundleRejectsInvalidTransaction(t *testing.T) {
	if _, err := ParseBundle([]byte(`{"schema":"kicadai.repair.bundle.v1","transaction":{"operations":[]}}`)); err == nil {
		t.Fatal("expected invalid transaction error")
	}
}

func TestParseBundleRejectsParentTraversalProjectRoot(t *testing.T) {
	if _, err := ParseBundle([]byte(`{"schema":"kicadai.repair.bundle.v1","project_root":"../outside"}`)); err == nil {
		t.Fatal("expected project_root traversal error")
	}
}

func parseBundle(t *testing.T, input string) Bundle {
	t.Helper()
	bundle, err := ParseBundle([]byte(input))
	if err != nil {
		t.Fatalf("ParseBundle returned error: %v", err)
	}
	return bundle
}

func TestLoadBundleReportsReadErrors(t *testing.T) {
	if _, err := LoadBundle(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected read error")
	}
}

func TestSaveBundleReportsWriteErrors(t *testing.T) {
	err := SaveBundle(filepath.Join(t.TempDir(), "missing", "repair.json"), Bundle{Schema: BundleSchemaV1})
	if err == nil || os.IsExist(err) {
		t.Fatalf("expected write error, got %v", err)
	}
}
