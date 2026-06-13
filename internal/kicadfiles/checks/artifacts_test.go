package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactWorkspaceCleansByDefault(t *testing.T) {
	workspace, cleanup, err := NewArtifactWorkspace("erc", Options{})
	if err != nil {
		t.Fatalf("NewArtifactWorkspace() error = %v", err)
	}
	root := workspace.Root
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("workspace missing: %v", err)
	}
	cleanup()
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("workspace still exists or unexpected stat error: %v", err)
	}
}

func TestArtifactWorkspacePreservesWhenRequested(t *testing.T) {
	base := t.TempDir()
	workspace, cleanup, err := NewArtifactWorkspace("drc", Options{KeepArtifacts: true, ArtifactDir: base})
	if err != nil {
		t.Fatalf("NewArtifactWorkspace() error = %v", err)
	}
	cleanup()
	if _, err := os.Stat(workspace.Root); err != nil {
		t.Fatalf("workspace not preserved: %v", err)
	}
	if !strings.HasPrefix(workspace.Root, base+string(filepath.Separator)) {
		t.Fatalf("workspace root = %q, want under %q", workspace.Root, base)
	}
}

func TestArtifactWorkspacePathRejectsTraversal(t *testing.T) {
	workspace, cleanup, err := NewArtifactWorkspace("check", Options{})
	if err != nil {
		t.Fatalf("NewArtifactWorkspace() error = %v", err)
	}
	defer cleanup()
	if _, err := workspace.Path("..", "escape"); err == nil {
		t.Fatal("expected traversal error")
	}
	if _, err := workspace.Path(filepath.Join(string(filepath.Separator), "tmp", "escape")); err == nil {
		t.Fatal("expected absolute path error")
	}
}

func TestArtifactWorkspaceCopyFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "input.kicad_sch")
	if err := os.WriteFile(src, []byte("schematic"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	workspace, cleanup, err := NewArtifactWorkspace("erc", Options{})
	if err != nil {
		t.Fatalf("NewArtifactWorkspace() error = %v", err)
	}
	defer cleanup()
	dst, err := workspace.CopyFile(src, "project/input.kicad_sch")
	if err != nil {
		t.Fatalf("CopyFile() error = %v", err)
	}
	if !pathWithinRoot(workspace.Root, dst) {
		t.Fatalf("copy escaped workspace: %s", dst)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read copy: %v", err)
	}
	if string(data) != "schematic" {
		t.Fatalf("copy contents = %q", data)
	}
}
