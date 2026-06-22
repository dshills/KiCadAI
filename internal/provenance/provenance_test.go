package provenance

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestSerializesDeterministically(t *testing.T) {
	tx := testTransaction(t)
	provenance := New("demo", tx, "test")
	first, err := Marshal(provenance)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Marshal(provenance)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("provenance serialization is not deterministic")
	}
	if !strings.Contains(string(first), Schema) {
		t.Fatalf("schema missing from provenance:\n%s", first)
	}
	if provenance.OperationCount != len(tx.Operations) || len(provenance.OperationSummaries) != len(tx.Operations) {
		t.Fatalf("operation metadata = %#v", provenance)
	}
}

func TestValidation(t *testing.T) {
	tx := testTransaction(t)
	cases := []struct {
		name     string
		mutate   func(*TransactionProvenance)
		wantPath string
		wantCode reports.Code
	}{
		{name: "missing schema", mutate: func(provenance *TransactionProvenance) { provenance.Schema = "" }, wantPath: "provenance.schema", wantCode: reports.CodeInvalidArgument},
		{name: "unsupported schema", mutate: func(provenance *TransactionProvenance) { provenance.Schema = "made.up" }, wantPath: "provenance.schema", wantCode: reports.CodeInvalidArgument},
		{name: "missing project", mutate: func(provenance *TransactionProvenance) { provenance.ProjectName = "" }, wantPath: "provenance.project_name", wantCode: reports.CodeInvalidArgument},
		{name: "missing generator version", mutate: func(provenance *TransactionProvenance) { provenance.GeneratorVersion = "" }, wantPath: "provenance.generator_version", wantCode: reports.CodeInvalidArgument},
		{name: "project mismatch", mutate: func(provenance *TransactionProvenance) { provenance.ProjectName = "other" }, wantPath: "provenance.project_name", wantCode: reports.CodeInvalidArgument},
		{name: "count mismatch", mutate: func(provenance *TransactionProvenance) { provenance.OperationCount++ }, wantPath: "provenance.operation_count", wantCode: reports.CodeInvalidArgument},
		{name: "missing summaries", mutate: func(provenance *TransactionProvenance) { provenance.OperationSummaries = nil }, wantPath: "provenance.operation_summaries", wantCode: reports.CodeInvalidArgument},
		{name: "summary mismatch", mutate: func(provenance *TransactionProvenance) { provenance.OperationSummaries[0].Op = "write_project" }, wantPath: "provenance.operation_summaries[0].op", wantCode: reports.CodeInvalidArgument},
		{
			name: "invalid transaction",
			mutate: func(provenance *TransactionProvenance) {
				provenance.Transaction.Operations = nil
				provenance.OperationCount = 0
			},
			wantPath: "provenance.transaction.operations",
			wantCode: reports.CodeInvalidArgument,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provenance := New("demo", tx, "test")
			tc.mutate(&provenance)
			issues := Validate(provenance)
			if !hasIssue(issues, tc.wantCode, tc.wantPath) {
				t.Fatalf("issues = %#v, want %s at %s", issues, tc.wantCode, tc.wantPath)
			}
		})
	}
}

func TestWriteRead(t *testing.T) {
	root := t.TempDir()
	tx := testTransaction(t)
	provenance := New("demo", tx, "test")
	artifact, err := Write(root, provenance)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Path == "" {
		t.Fatalf("artifact missing path")
	}
	if artifact.Path != RelativePath {
		t.Fatalf("artifact path = %q, want %q", artifact.Path, RelativePath)
	}
	read, issues, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if read.ProjectName != "demo" || read.OperationCount != len(tx.Operations) {
		t.Fatalf("read provenance = %#v", read)
	}
}

func TestAbsPathRejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	if _, err := AbsPath(root, filepath.Join("..", "transaction.json")); err == nil {
		t.Fatal("expected parent traversal rejection")
	}
	if _, err := AbsPath(root, filepath.Join(root, ".kicadai", "transaction.json")); err == nil {
		t.Fatal("expected absolute path rejection")
	}
	path, err := AbsPath(root, RelativePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, root) {
		t.Fatalf("path %q not inside root %q", path, root)
	}
}

func testTransaction(t *testing.T) transactions.Transaction {
	t.Helper()
	tx, err := transactions.Parse([]byte(`{"name":"demo","project":"demo","operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"write_project"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func hasIssue(issues []reports.Issue, code reports.Code, path string) bool {
	for _, issue := range issues {
		if issue.Code == code && issue.Path == path {
			return true
		}
	}
	return false
}
