package repair

import (
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/inspect"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestHydrateGeneratedTargetWithBundleIsMutable(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "demo.kicad_pro"), "{}")
	bundle := generatedBundle(t, root)
	target := HydrateTarget(root, HydrateOptions{Bundle: &bundle, InspectProject: stubInspect(root, inspect.ProjectSummary{Root: root})})
	if !target.Mutable || !target.Generated || target.Kind != TargetProjectDir {
		t.Fatalf("target = %#v", target)
	}
}

func TestHydrateMissingBundleBlocksMutation(t *testing.T) {
	root := t.TempDir()
	target := HydrateTarget(root, HydrateOptions{InspectProject: stubInspect(root, inspect.ProjectSummary{Root: root})})
	if target.Mutable || !reports.HasBlockingIssue(target.Issues) {
		t.Fatalf("target = %#v", target)
	}
}

func TestHydrateUnsupportedContentBlocksMutation(t *testing.T) {
	root := t.TempDir()
	bundle := generatedBundle(t, root)
	target := HydrateTarget(root, HydrateOptions{
		Bundle: &bundle,
		InspectProject: stubInspect(root, inspect.ProjectSummary{
			Root:        root,
			Unsupported: []inspect.UnsupportedNode{{Kind: "future", Count: 1}},
		}),
	})
	if target.Mutable {
		t.Fatalf("target should not be mutable: %#v", target)
	}
	assertIssueCode(t, target.Issues, reports.CodePreservationConflict)
}

func TestHydrateDirectSchematicTargetIsInspectableButNotMutableWithoutProvenance(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "demo.kicad_sch")
	writeFile(t, path, "(kicad_sch)")
	target := HydrateTarget(path, HydrateOptions{InspectProject: stubInspect(root, inspect.ProjectSummary{Root: root})})
	if target.Kind != TargetSchematic || target.Mutable {
		t.Fatalf("target = %#v", target)
	}
}

func TestHydrateMissingTargetReturnsIssue(t *testing.T) {
	target := HydrateTarget(filepath.Join(t.TempDir(), "missing"), HydrateOptions{})
	if len(target.Issues) == 0 || target.Issues[0].Code != reports.CodeMissingFile {
		t.Fatalf("target = %#v", target)
	}
}

func generatedBundle(t *testing.T, root string) Bundle {
	t.Helper()
	tx := transactions.Transaction{Operations: []transactions.Operation{
		mustRepairOperation(t, transactions.OpCreateProject, transactions.CreateProjectOperation{Op: transactions.OpCreateProject, Name: "demo"}, ""),
		mustRepairOperation(t, transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject}, ""),
	}}
	return Bundle{Schema: BundleSchemaV1, ProjectRoot: root, ProjectName: "demo", Generated: true, Transaction: &tx}
}

func stubInspect(root string, summary inspect.ProjectSummary) func(string) (inspect.ProjectSummary, error) {
	return func(path string) (inspect.ProjectSummary, error) {
		if filepath.Clean(path) != filepath.Clean(root) {
			return inspect.ProjectSummary{}, os.ErrNotExist
		}
		return summary, nil
	}
}

func writeFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertIssueCode(t *testing.T, issues []reports.Issue, code reports.Code) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("missing issue code %s in %#v", code, issues)
}
