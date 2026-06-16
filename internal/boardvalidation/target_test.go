package boardvalidation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveTargetDirectBoard(t *testing.T) {
	dir := t.TempDir()
	board := writeTestFile(t, dir, "direct.kicad_pcb")
	target, err := ResolveTarget(board)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if target.BoardPath != filepath.ToSlash(board) {
		t.Fatalf("BoardPath = %q, want %q", target.BoardPath, filepath.ToSlash(board))
	}
	if target.ProjectPath != "" {
		t.Fatalf("ProjectPath = %q, want empty", target.ProjectPath)
	}
	if target.ProjectDir != filepath.ToSlash(dir) {
		t.Fatalf("ProjectDir = %q, want %q", target.ProjectDir, filepath.ToSlash(dir))
	}
}

func TestResolveTargetProjectDirectoryMatchingBoard(t *testing.T) {
	dir := t.TempDir()
	project := writeTestFile(t, dir, "demo.kicad_pro")
	board := writeTestFile(t, dir, "demo.kicad_pcb")
	writeTestFile(t, dir, "other.kicad_pcb")
	target, err := ResolveTarget(dir)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if target.ProjectPath != filepath.ToSlash(project) {
		t.Fatalf("ProjectPath = %q, want %q", target.ProjectPath, filepath.ToSlash(project))
	}
	if target.BoardPath != filepath.ToSlash(board) {
		t.Fatalf("BoardPath = %q, want %q", target.BoardPath, filepath.ToSlash(board))
	}
}

func TestResolveTargetProjectDirectorySingleFallbackBoard(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "demo.kicad_pro")
	board := writeTestFile(t, dir, "board.kicad_pcb")
	target, err := ResolveTarget(dir)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if target.BoardPath != filepath.ToSlash(board) {
		t.Fatalf("BoardPath = %q, want %q", target.BoardPath, filepath.ToSlash(board))
	}
}

func TestResolveTargetProjectDirectoryMatchingBoardCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "Demo.kicad_pro")
	board := writeTestFile(t, dir, "demo.kicad_pcb")
	writeTestFile(t, dir, "other.kicad_pcb")
	target, err := ResolveTarget(dir)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if target.BoardPath != filepath.ToSlash(board) {
		t.Fatalf("BoardPath = %q, want %q", target.BoardPath, filepath.ToSlash(board))
	}
}

func TestResolveTargetProjectFile(t *testing.T) {
	dir := t.TempDir()
	project := writeTestFile(t, dir, "demo.kicad_pro")
	board := writeTestFile(t, dir, "demo.kicad_pcb")
	target, err := ResolveTarget(project)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if target.InputPath != filepath.ToSlash(project) {
		t.Fatalf("InputPath = %q, want %q", target.InputPath, filepath.ToSlash(project))
	}
	if target.ProjectPath != filepath.ToSlash(project) {
		t.Fatalf("ProjectPath = %q, want %q", target.ProjectPath, filepath.ToSlash(project))
	}
	if target.BoardPath != filepath.ToSlash(board) {
		t.Fatalf("BoardPath = %q, want %q", target.BoardPath, filepath.ToSlash(board))
	}
}

func TestResolveTargetProjectDirectoryErrors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string)
		wantErr string
	}{
		{
			name:    "missing project",
			setup:   func(t *testing.T, dir string) { writeTestFile(t, dir, "demo.kicad_pcb") },
			wantErr: "no .kicad_pro file found",
		},
		{
			name: "multiple projects",
			setup: func(t *testing.T, dir string) {
				writeTestFile(t, dir, "a.kicad_pro")
				writeTestFile(t, dir, "b.kicad_pro")
				writeTestFile(t, dir, "a.kicad_pcb")
			},
			wantErr: "multiple .kicad_pro files found",
		},
		{
			name:    "missing board",
			setup:   func(t *testing.T, dir string) { writeTestFile(t, dir, "demo.kicad_pro") },
			wantErr: "no .kicad_pcb file found",
		},
		{
			name: "ambiguous board",
			setup: func(t *testing.T, dir string) {
				writeTestFile(t, dir, "demo.kicad_pro")
				writeTestFile(t, dir, "one.kicad_pcb")
				writeTestFile(t, dir, "two.kicad_pcb")
			},
			wantErr: "ambiguous .kicad_pcb files",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			_, err := ResolveTarget(dir)
			if err == nil {
				t.Fatalf("ResolveTarget() error = nil, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ResolveTarget() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestResolveTargetRejectsUnsupportedFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTestFile(t, dir, "demo.txt")
	_, err := ResolveTarget(path)
	if err == nil {
		t.Fatal("ResolveTarget() error = nil, want unsupported file error")
	}
	if !strings.Contains(err.Error(), "target must be a .kicad_pcb file") {
		t.Fatalf("ResolveTarget() error = %q", err.Error())
	}
}

func writeTestFile(t *testing.T, dir string, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("abs %s: %v", path, err)
	}
	return absolute
}
