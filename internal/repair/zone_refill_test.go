package repair

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type fakeZoneRefillRunner struct {
	called     bool
	result     ZoneRefillRunResult
	err        error
	seenCLI    string
	seenTarget string
}

func (runner *fakeZoneRefillRunner) RefillZones(_ context.Context, cli checks.KiCadCLI, target string, _ ZoneRefillOptions) (ZoneRefillRunResult, error) {
	runner.called = true
	runner.seenCLI = cli.Path
	runner.seenTarget = target
	return runner.result, runner.err
}

type fakeCheckRunner struct {
	result checks.CommandResult
}

func (runner fakeCheckRunner) Run(_ context.Context, workingDir string, path string, args ...string) checks.CommandResult {
	result := runner.result
	result.WorkingDir = workingDir
	result.Path = path
	result.Args = append([]string(nil), args...)
	return result
}

func (fakeCheckRunner) Version(context.Context, string) (string, error) {
	return "fake", nil
}

type sequenceCheckRunner struct {
	results                []checks.CommandResult
	calls                  int
	writeReportOnCall      int
	writeEmptyReportOnCall int
}

func (runner *sequenceCheckRunner) Run(_ context.Context, workingDir string, path string, args ...string) checks.CommandResult {
	runner.calls++
	if len(runner.results) == 0 {
		return checks.CommandResult{
			WorkingDir: workingDir,
			Path:       path,
			Args:       append([]string(nil), args...),
			ExitCode:   -1,
			Err:        errors.New("sequence check runner has no results"),
		}
	}
	if runner.calls == runner.writeReportOnCall {
		for index, arg := range args {
			if arg == "--output" && index+1 < len(args) {
				_ = os.WriteFile(args[index+1], []byte("{}\n"), 0o644)
				break
			}
		}
	}
	if runner.calls == runner.writeEmptyReportOnCall {
		for index, arg := range args {
			if arg == "--output" && index+1 < len(args) {
				_ = os.WriteFile(args[index+1], nil, 0o644)
				break
			}
		}
	}
	resultIndex := runner.calls - 1
	if resultIndex >= len(runner.results) {
		resultIndex = len(runner.results) - 1
	}
	result := runner.results[resultIndex]
	result.WorkingDir = workingDir
	result.Path = path
	result.Args = append([]string(nil), args...)
	return result
}

func (*sequenceCheckRunner) Version(context.Context, string) (string, error) {
	return "fake", nil
}

func TestRunZoneRefillDefaultPolicySkipsWithoutCallingRunner(t *testing.T) {
	tx := transactions.Transaction{}
	target := Target{Generated: true, Transaction: &tx}
	runner := &fakeZoneRefillRunner{}
	result := RunZoneRefill(context.Background(), target, t.TempDir(), ZoneRefillOptions{}, runner)
	if !result.Skipped || result.Ran || runner.called {
		t.Fatalf("result = %#v, runner called = %v", result, runner.called)
	}
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
}

func TestRunZoneRefillRequestedMissingCLIBlocks(t *testing.T) {
	tx := transactions.Transaction{}
	target := Target{Generated: true, Transaction: &tx}
	runner := &fakeZoneRefillRunner{}
	result := RunZoneRefill(context.Background(), target, t.TempDir(), ZoneRefillOptions{
		Policy:   ZoneRefillBeforeValidation,
		KiCadCLI: filepath.Join(t.TempDir(), "missing-kicad-cli"),
	}, runner)
	if runner.called {
		t.Fatalf("runner called despite missing CLI")
	}
	if len(result.Issues) != 1 || result.Issues[0].Code != reports.CodeSkippedExternalTool || !result.Issues[0].Blocking() {
		t.Fatalf("issues = %#v", result.Issues)
	}
}

func TestRunZoneRefillRequestedImportedTargetBlocks(t *testing.T) {
	cli := fakeExecutable(t)
	runner := &fakeZoneRefillRunner{}
	result := RunZoneRefill(context.Background(), Target{}, t.TempDir(), ZoneRefillOptions{
		Policy:   ZoneRefillBeforeValidation,
		KiCadCLI: cli,
	}, runner)
	if runner.called {
		t.Fatalf("runner called for imported target")
	}
	if len(result.Issues) != 1 || result.Issues[0].Code != reports.CodePreservationConflict || !result.Issues[0].Blocking() {
		t.Fatalf("issues = %#v", result.Issues)
	}
}

func TestRunZoneRefillFakeSuccessRecordsEvidence(t *testing.T) {
	tx := transactions.Transaction{}
	target := Target{Generated: true, Transaction: &tx}
	cli := fakeExecutable(t)
	project := t.TempDir()
	artifact := reports.Artifact{Kind: reports.ArtifactValidationReport, Path: filepath.ToSlash(filepath.Join(project, "zone.json")), Description: "zone refill"}
	runner := &fakeZoneRefillRunner{result: ZoneRefillRunResult{
		Command:   []string{cli, "pcb", "fill-zones"},
		Artifacts: []reports.Artifact{artifact},
	}}
	result := RunZoneRefill(context.Background(), target, project, ZoneRefillOptions{
		Policy:   ZoneRefillAfterRepairValidation,
		KiCadCLI: cli,
	}, runner)
	if !runner.called || !result.Ran || result.Skipped {
		t.Fatalf("result = %#v, runner called = %v", result, runner.called)
	}
	if runner.seenCLI != cli || runner.seenTarget != filepath.ToSlash(project) {
		t.Fatalf("runner saw cli=%q target=%q", runner.seenCLI, runner.seenTarget)
	}
	if len(result.Issues) != 0 || len(result.Artifacts) != 1 || result.Artifacts[0].Path != artifact.Path {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunZoneRefillFailureBlocks(t *testing.T) {
	tx := transactions.Transaction{}
	target := Target{Generated: true, Transaction: &tx}
	cli := fakeExecutable(t)
	runner := &fakeZoneRefillRunner{err: fmt.Errorf("zone refill failed")}
	result := RunZoneRefill(context.Background(), target, t.TempDir(), ZoneRefillOptions{
		Policy:   ZoneRefillBeforeValidation,
		KiCadCLI: cli,
	}, runner)
	if !runner.called {
		t.Fatalf("runner was not called")
	}
	if len(result.Issues) != 1 || result.Issues[0].Code != reports.CodeKiCadCLIFailed || !result.Issues[0].Blocking() || !strings.Contains(result.Issues[0].Message, "zone refill failed") {
		t.Fatalf("issues = %#v", result.Issues)
	}
}

func TestKiCadZoneRefillRunnerAllowsDRCViolationExitCode(t *testing.T) {
	project := t.TempDir()
	pcb := filepath.Join(project, "demo.kicad_pcb")
	if err := os.WriteFile(pcb, []byte("(kicad_pcb)\n"), 0o644); err != nil {
		t.Fatalf("write pcb: %v", err)
	}
	runner := KiCadZoneRefillRunner{Runner: fakeCheckRunner{result: checks.CommandResult{
		ExitCode: 1,
		Err:      errors.New("exit status 1"),
	}}}
	result, err := runner.RefillZones(context.Background(), checks.KiCadCLI{Path: "/bin/kicad-cli"}, project, ZoneRefillOptions{KeepArtifacts: true, ArtifactDir: t.TempDir()})
	if err != nil {
		t.Fatalf("RefillZones() error = %v", err)
	}
	if len(result.Artifacts) != 1 || !strings.Contains(strings.Join(result.Command, " "), "--refill-zones") || !strings.Contains(strings.Join(result.Command, " "), "--save-board") {
		t.Fatalf("result = %#v", result)
	}
}

func TestKiCadZoneRefillRunnerReportsRepeatedNoOutputCrash(t *testing.T) {
	project := t.TempDir()
	pcb := filepath.Join(project, "demo.kicad_pcb")
	if err := os.WriteFile(pcb, []byte("(kicad_pcb)\n"), 0o644); err != nil {
		t.Fatalf("write pcb: %v", err)
	}
	checkRunner := &sequenceCheckRunner{results: []checks.CommandResult{{
		ExitCode: -1,
		Err:      signaledProcessError(t),
	}}}
	runner := KiCadZoneRefillRunner{Runner: checkRunner}
	_, err := runner.RefillZones(context.Background(), checks.KiCadCLI{Path: "/bin/kicad-cli"}, project, ZoneRefillOptions{})
	if err == nil || !strings.Contains(err.Error(), "signal: abort trap") {
		t.Fatalf("RefillZones() error = %v", err)
	}
	if checkRunner.calls != 2 {
		t.Fatalf("runner calls = %d, want 2", checkRunner.calls)
	}
}

func TestKiCadZoneRefillRunnerRetriesNoOutputCrashOnce(t *testing.T) {
	project := t.TempDir()
	pcb := filepath.Join(project, "demo.kicad_pcb")
	if err := os.WriteFile(pcb, []byte("(kicad_pcb)\n"), 0o644); err != nil {
		t.Fatalf("write pcb: %v", err)
	}
	checkRunner := &sequenceCheckRunner{results: []checks.CommandResult{{
		ExitCode: -1,
		Err:      signaledProcessError(t),
	}, {ExitCode: 0}}}
	runner := KiCadZoneRefillRunner{Runner: checkRunner}
	if _, err := runner.RefillZones(context.Background(), checks.KiCadCLI{Path: "/bin/kicad-cli"}, project, ZoneRefillOptions{}); err != nil {
		t.Fatalf("RefillZones() error = %v", err)
	}
	if checkRunner.calls != 2 {
		t.Fatalf("runner calls = %d, want 2", checkRunner.calls)
	}
}

func TestKiCadZoneRefillRunnerRetriesCrashWithEmptyReport(t *testing.T) {
	project := t.TempDir()
	pcb := filepath.Join(project, "demo.kicad_pcb")
	if err := os.WriteFile(pcb, []byte("(kicad_pcb)\n"), 0o644); err != nil {
		t.Fatalf("write pcb: %v", err)
	}
	checkRunner := &sequenceCheckRunner{
		results: []checks.CommandResult{{
			ExitCode: -1,
			Err:      signaledProcessError(t),
		}, {ExitCode: 0}},
		writeEmptyReportOnCall: 1,
	}
	runner := KiCadZoneRefillRunner{Runner: checkRunner}
	if _, err := runner.RefillZones(context.Background(), checks.KiCadCLI{Path: "/bin/kicad-cli"}, project, ZoneRefillOptions{}); err != nil {
		t.Fatalf("RefillZones() error = %v", err)
	}
	if checkRunner.calls != 2 {
		t.Fatalf("runner calls = %d, want 2", checkRunner.calls)
	}
}

func TestKiCadZoneRefillRunnerDoesNotRetrySubstantiveFailure(t *testing.T) {
	tests := []struct {
		name              string
		result            checks.CommandResult
		writeReportOnCall int
	}{
		{
			name: "stderr",
			result: checks.CommandResult{
				ExitCode: -1,
				Stderr:   "fatal error",
				Err:      signaledProcessError(t),
			},
		},
		{
			name: "report",
			result: checks.CommandResult{
				ExitCode: -1,
				Err:      signaledProcessError(t),
			},
			writeReportOnCall: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project := t.TempDir()
			pcb := filepath.Join(project, "demo.kicad_pcb")
			if err := os.WriteFile(pcb, []byte("(kicad_pcb)\n"), 0o644); err != nil {
				t.Fatalf("write pcb: %v", err)
			}
			checkRunner := &sequenceCheckRunner{
				results:           []checks.CommandResult{test.result},
				writeReportOnCall: test.writeReportOnCall,
			}
			runner := KiCadZoneRefillRunner{Runner: checkRunner}
			if _, err := runner.RefillZones(context.Background(), checks.KiCadCLI{Path: "/bin/kicad-cli"}, project, ZoneRefillOptions{}); err == nil {
				t.Fatal("RefillZones() error = nil")
			}
			if checkRunner.calls != 1 {
				t.Fatalf("runner calls = %d, want 1", checkRunner.calls)
			}
		})
	}
}

func TestKiCadZoneRefillRunnerDoesNotRetryLaunchFailure(t *testing.T) {
	project := t.TempDir()
	pcb := filepath.Join(project, "demo.kicad_pcb")
	if err := os.WriteFile(pcb, []byte("(kicad_pcb)\n"), 0o644); err != nil {
		t.Fatalf("write pcb: %v", err)
	}
	checkRunner := &sequenceCheckRunner{results: []checks.CommandResult{{
		ExitCode: -1,
		Err:      errors.New("executable file not found"),
	}}}
	runner := KiCadZoneRefillRunner{Runner: checkRunner}
	if _, err := runner.RefillZones(context.Background(), checks.KiCadCLI{Path: "/bin/kicad-cli"}, project, ZoneRefillOptions{}); err == nil {
		t.Fatal("RefillZones() error = nil")
	}
	if checkRunner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", checkRunner.calls)
	}
}

func signaledProcessError(t *testing.T) error {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("signal-based process termination is not portable to Windows")
	}
	err := exec.Command("/bin/sh", "-c", "kill -ABRT $$").Run()
	if err == nil {
		t.Fatal("signaled process returned no error")
	}
	return err
}

func TestDiscoverZoneRefillPCBRejectsAmbiguousDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha.kicad_pcb", "beta.kicad_pcb"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("(kicad_pcb)\n"), 0o644); err != nil {
			t.Fatalf("write pcb: %v", err)
		}
	}
	if _, err := discoverZoneRefillPCB(dir); err == nil || !strings.Contains(err.Error(), "multiple .kicad_pcb") {
		t.Fatalf("discoverZoneRefillPCB() error = %v", err)
	}
}

func TestDiscoverZoneRefillPCBIgnoresAutosaveFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "_autosave-demo.kicad_pcb"), []byte("(kicad_pcb)\n"), 0o644); err != nil {
		t.Fatalf("write autosave: %v", err)
	}
	if _, err := discoverZoneRefillPCB(dir); err == nil || !strings.Contains(err.Error(), "no .kicad_pcb") {
		t.Fatalf("discoverZoneRefillPCB() error = %v", err)
	}
}

func fakeExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kicad-cli")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
	return path
}
