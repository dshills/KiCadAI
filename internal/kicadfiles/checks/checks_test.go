package checks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunERCWithRunnerParsesViolationReportExitOne(t *testing.T) {
	dir := t.TempDir()
	schematic := filepath.Join(dir, "demo.kicad_sch")
	writeCheckTestFile(t, schematic, "(kicad_sch)")
	runner := fakeRunner{
		version: "10.0.3",
		run: func(_ context.Context, _ string, _ string, args ...string) CommandResult {
			report := argAfter(args, "--output")
			if err := os.WriteFile(report, []byte(sampleERCReport), 0o644); err != nil {
				t.Fatalf("write report: %v", err)
			}
			return CommandResult{Args: args, ExitCode: 1, Duration: time.Millisecond, Err: fmt.Errorf("violations")}
		},
	}
	result, err := RunERCWithRunner(context.Background(), runner, KiCadCLI{Path: "/bin/kicad-cli"}, schematic, Options{KeepArtifacts: true, ArtifactDir: t.TempDir()})
	if err != nil {
		t.Fatalf("RunERCWithRunner() error = %v", err)
	}
	if result.Status != CheckStatusFail {
		t.Fatalf("status = %s, want fail", result.Status)
	}
	if len(result.Findings) != 2 {
		t.Fatalf("findings = %d, want 2", len(result.Findings))
	}
	if len(result.ContextWarnings) != 1 || result.ProjectContext != ProjectContextStandalone {
		t.Fatalf("expected standalone warning: %#v", result)
	}
}

func TestRunDRCWithRunnerMissingReportIsToolError(t *testing.T) {
	dir := t.TempDir()
	board := filepath.Join(dir, "demo.kicad_pcb")
	writeCheckTestFile(t, board, "(kicad_pcb)")
	runner := fakeRunner{
		version: "10.0.3",
		run: func(_ context.Context, _ string, _ string, args ...string) CommandResult {
			return CommandResult{Args: args, ExitCode: 2, Duration: time.Millisecond, Err: fmt.Errorf("crash")}
		},
	}
	result, err := RunDRCWithRunner(context.Background(), runner, KiCadCLI{Path: "/bin/kicad-cli"}, board, Options{})
	if err == nil {
		t.Fatal("expected tool error")
	}
	if result.Status != CheckStatusError {
		t.Fatalf("status = %s, want error", result.Status)
	}
}

func TestRunDRCProjectCopiesContext(t *testing.T) {
	dir := t.TempDir()
	writeCheckTestFile(t, filepath.Join(dir, "demo.kicad_pro"), "{}")
	writeCheckTestFile(t, filepath.Join(dir, "demo.kicad_pcb"), "(kicad_pcb)")
	writeCheckTestFile(t, filepath.Join(dir, "demo.kicad_sch"), "(kicad_sch)")
	writeCheckTestFile(t, filepath.Join(dir, "sym-lib-table"), "(sym_lib_table)")
	runner := fakeRunner{
		version: "10.0.3",
		run: func(_ context.Context, workingDir string, _ string, args ...string) CommandResult {
			input := args[len(args)-1]
			if !strings.Contains(filepath.ToSlash(input), "kicadai-check-drc") || filepath.Dir(input) != workingDir {
				t.Fatalf("input = %q workingDir = %q, want copied artifact input", input, workingDir)
			}
			report := argAfter(args, "--output")
			if err := os.WriteFile(report, []byte(`{"coordinate_units":"mm","violations":[]}`), 0o644); err != nil {
				t.Fatalf("write report: %v", err)
			}
			return CommandResult{Args: args, ExitCode: 0, Duration: time.Millisecond}
		},
	}
	result, err := RunDRCWithRunner(context.Background(), runner, KiCadCLI{Path: "/bin/kicad-cli"}, dir, Options{KeepArtifacts: true, ArtifactDir: t.TempDir()})
	if err != nil {
		t.Fatalf("RunDRCWithRunner() error = %v", err)
	}
	if result.Status != CheckStatusPass || result.ProjectContext != ProjectContextFull {
		t.Fatalf("unexpected result: %#v", result)
	}
}

type fakeRunner struct {
	version string
	run     func(context.Context, string, string, ...string) CommandResult
}

func (runner fakeRunner) Run(ctx context.Context, workingDir string, path string, args ...string) CommandResult {
	return runner.run(ctx, workingDir, path, args...)
}

func (runner fakeRunner) Version(context.Context, string) (string, error) {
	return runner.version, nil
}

func argAfter(args []string, name string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func writeCheckTestFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
