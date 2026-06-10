package roundtrip

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactWorkspaceCleansUnpreservedArtifacts(t *testing.T) {
	workspace, cleanup, err := NewArtifactWorkspace("fixture", Options{})
	if err != nil {
		t.Fatalf("NewArtifactWorkspace returned error: %v", err)
	}
	root := workspace.Root
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("workspace missing: %v", err)
	}

	cleanup()
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("workspace still exists or stat failed unexpectedly: %v", err)
	}
}

func TestArtifactWorkspacePreservesRequestedArtifacts(t *testing.T) {
	base := t.TempDir()
	workspace, cleanup, err := NewArtifactWorkspace("fixture", Options{KeepArtifacts: true, ArtifactDir: base})
	if err != nil {
		t.Fatalf("NewArtifactWorkspace returned error: %v", err)
	}
	cleanup()
	if _, err := os.Stat(workspace.Root); err != nil {
		t.Fatalf("workspace not preserved: %v", err)
	}
	resolvedBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatalf("resolve base: %v", err)
	}
	if !strings.HasPrefix(workspace.Root, resolvedBase+string(filepath.Separator)) {
		t.Fatalf("workspace = %q, want under %q", workspace.Root, resolvedBase)
	}
}

func TestArtifactWorkspacePathRejectsTraversal(t *testing.T) {
	workspace, cleanup, err := NewArtifactWorkspace("fixture", Options{})
	if err != nil {
		t.Fatalf("NewArtifactWorkspace returned error: %v", err)
	}
	defer cleanup()

	if _, err := workspace.Path("..", "escape"); err == nil {
		t.Fatal("expected traversal error")
	}
	if _, err := workspace.Path(filepath.Join(string(filepath.Separator), "tmp", "escape")); err == nil {
		t.Fatal("expected absolute path error")
	}
}

func TestArtifactWorkspaceCopyInputAllowsExternalSource(t *testing.T) {
	src := filepath.Join(t.TempDir(), "source.kicad_pcb")
	writeArtifactTestFile(t, src, "pcb")
	workspace, cleanup, err := NewArtifactWorkspace("fixture", Options{})
	if err != nil {
		t.Fatalf("NewArtifactWorkspace returned error: %v", err)
	}
	defer cleanup()

	dst, err := workspace.CopyInput(src)
	if err != nil {
		t.Fatalf("CopyInput returned error: %v", err)
	}
	if !pathWithinRoot(workspace.Root, dst) {
		t.Fatalf("copy destination escaped workspace: %s", dst)
	}
	contents, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read copy: %v", err)
	}
	if string(contents) != "pcb" {
		t.Fatalf("copy contents = %q", contents)
	}
}

func TestArtifactWorkspaceCopyInputDisambiguatesDuplicateNames(t *testing.T) {
	dir := t.TempDir()
	srcA := filepath.Join(dir, "a", "source.kicad_pcb")
	srcB := filepath.Join(dir, "b", "source.kicad_pcb")
	if err := os.MkdirAll(filepath.Dir(srcA), 0o755); err != nil {
		t.Fatalf("mkdir source a: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(srcB), 0o755); err != nil {
		t.Fatalf("mkdir source b: %v", err)
	}
	writeArtifactTestFile(t, srcA, "a")
	writeArtifactTestFile(t, srcB, "b")
	workspace, cleanup, err := NewArtifactWorkspace("fixture", Options{})
	if err != nil {
		t.Fatalf("NewArtifactWorkspace returned error: %v", err)
	}
	defer cleanup()

	dstA, err := workspace.CopyInput(srcA)
	if err != nil {
		t.Fatalf("CopyInput A returned error: %v", err)
	}
	dstB, err := workspace.CopyInput(srcB)
	if err != nil {
		t.Fatalf("CopyInput B returned error: %v", err)
	}
	if dstA == dstB {
		t.Fatalf("duplicate destinations: %s", dstA)
	}
}

func writeArtifactTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write artifact test file: %v", err)
	}
}

func TestArtifactWorkspaceWriteText(t *testing.T) {
	workspace, cleanup, err := NewArtifactWorkspace("fixture", Options{})
	if err != nil {
		t.Fatalf("NewArtifactWorkspace returned error: %v", err)
	}
	defer cleanup()

	path, err := workspace.WriteText("diffs/output.diff", "diff")
	if err != nil {
		t.Fatalf("WriteText returned error: %v", err)
	}
	if !pathWithinRoot(workspace.Root, path) {
		t.Fatalf("write destination escaped workspace: %s", path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written artifact: %v", err)
	}
	if string(contents) != "diff" {
		t.Fatalf("artifact contents = %q", contents)
	}
}
