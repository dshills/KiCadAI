package fabrication

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizePlotRequestRejectsOutputEscape(t *testing.T) {
	root := t.TempDir()
	request := PlotRequest{
		ProjectRoot: root,
		PCBPath:     filepath.Join(root, "demo.kicad_pcb"),
		PackageDir:  filepath.Join(root, "fabrication"),
		GerberDir:   filepath.Join(filepath.Dir(root), "gerbers"),
		DrillDir:    filepath.Join(root, "fabrication", "drill"),
	}
	_, issues := normalizePlotRequest(request)
	if !hasIssuePath(issues, "gerber_dir") {
		t.Fatalf("issues = %#v, want gerber_dir containment issue", issues)
	}
}

func TestPackageRelativePathRejectsEscape(t *testing.T) {
	root := t.TempDir()
	packageDir := filepath.Join(root, "fabrication")
	_, err := packageRelativePath(packageDir, filepath.Join(root, "outside.gbr"))
	if err == nil {
		t.Fatalf("packageRelativePath accepted escaped path")
	}
}

func TestPackageRelativePathNormalizesInsidePath(t *testing.T) {
	root := t.TempDir()
	packageDir := filepath.Join(root, "fabrication")
	got, err := packageRelativePath(packageDir, filepath.Join(packageDir, "gerbers", "demo-F_Cu.gbr"))
	if err != nil {
		t.Fatalf("packageRelativePath returned error: %v", err)
	}
	if got != "gerbers/demo-F_Cu.gbr" {
		t.Fatalf("relative path = %q, want gerbers/demo-F_Cu.gbr", got)
	}
}

func TestFakePlotRunnerReturnsDeterministicEvidence(t *testing.T) {
	runner := &FakePlotRunner{Evidence: []PlotCommandEvidence{{
		GeneratedPaths: []string{"gerbers/demo-F_Cu.gbr"},
	}}}
	command := PlotCommand{Kind: PlotKindGerber, Argv: []string{"kicad-cli", "pcb", "export"}, OutputDir: "gerbers"}
	evidence := runner.RunPlotCommand(context.Background(), command)
	if evidence.Kind != PlotKindGerber {
		t.Fatalf("kind = %q, want %q", evidence.Kind, PlotKindGerber)
	}
	if len(evidence.Argv) != 3 {
		t.Fatalf("argv = %#v, want command argv copied", evidence.Argv)
	}
	if len(evidence.GeneratedPaths) != 1 || evidence.GeneratedPaths[0] != "gerbers/demo-F_Cu.gbr" {
		t.Fatalf("generated paths = %#v", evidence.GeneratedPaths)
	}
	if len(runner.Calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.Calls))
	}
}

func TestFakePlotRunnerSurfacesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &FakePlotRunner{}
	evidence := runner.RunPlotCommand(ctx, PlotCommand{Kind: PlotKindDrill, Argv: []string{"kicad-cli"}})
	if evidence.ExitCode != -1 {
		t.Fatalf("exit code = %d, want -1", evidence.ExitCode)
	}
	if evidence.StderrSnippet == "" {
		t.Fatalf("stderr snippet is empty")
	}
}

func TestExecPlotRunnerRejectsNonKiCadExecutable(t *testing.T) {
	evidence := ExecPlotRunner{}.RunPlotCommand(context.Background(), PlotCommand{
		Kind:      PlotKindGerber,
		Argv:      []string{"sh", "-c", "echo bad"},
		OutputDir: t.TempDir(),
	})
	if evidence.ExitCode != -1 {
		t.Fatalf("exit code = %d, want -1", evidence.ExitCode)
	}
	if evidence.StderrSnippet == "" {
		t.Fatalf("stderr snippet is empty")
	}
}

func TestPlotCommandWorkingDirUsesExistingParent(t *testing.T) {
	root := t.TempDir()
	got := plotCommandWorkingDir(filepath.Join(root, "missing-child"))
	if got != root {
		t.Fatalf("working dir = %q, want %q", got, root)
	}
}

func TestCommandSnippetIsRuneSafe(t *testing.T) {
	input := ""
	for range 600 {
		input += "ø"
	}
	got := commandSnippet(input)
	if len([]rune(got)) != 500 {
		t.Fatalf("snippet rune length = %d, want 500", len([]rune(got)))
	}
}

func TestLimitedBufferCapsStoredOutput(t *testing.T) {
	buffer := &limitedBuffer{Limit: 4}
	n, err := buffer.Write([]byte("abcdef"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != 6 {
		t.Fatalf("Write count = %d, want 6", n)
	}
	if got := buffer.String(); got != "abcd\n[output truncated]" {
		t.Fatalf("buffer = %q, want truncation marker", got)
	}
}

func TestLimitedBufferReturnsValidUTF8AfterByteLimit(t *testing.T) {
	buffer := &limitedBuffer{Limit: 1}
	_, _ = buffer.Write([]byte("ø"))
	if got := buffer.String(); got != "\n[output truncated]" {
		t.Fatalf("buffer = %q, want invalid rune removed with truncation marker", got)
	}
}

func TestJoinCommandErrorPreservesStderr(t *testing.T) {
	got := joinCommandError("KiCad failed\n", "exit status 1")
	want := "KiCad failed\nexecution error: exit status 1"
	if got != want {
		t.Fatalf("joined error = %q, want %q", got, want)
	}
}

func TestScanGeneratedPlotPathsReturnsSortedFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "b.gbr"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "a.drl"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths, err := scanPlotPaths(root)
	if err != nil {
		t.Fatalf("scanPlotPaths returned error: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %#v, want 2 files", paths)
	}
	if filepath.Base(paths[0]) != "b.gbr" || filepath.Base(paths[1]) != "a.drl" {
		t.Fatalf("paths = %#v, want sorted absolute paths", paths)
	}
}

func TestChangedPlotPathsIncludesNewAndModifiedFiles(t *testing.T) {
	before := map[string]plotFileState{
		"/tmp/out/existing.gbr": {Size: 1, ModUnix: 1},
		"/tmp/out/same.gbr":     {Size: 1, ModUnix: 1},
	}
	after := map[string]plotFileState{
		"/tmp/out/existing.gbr": {Size: 2, ModUnix: 2},
		"/tmp/out/same.gbr":     {Size: 1, ModUnix: 1},
		"/tmp/out/new.drl":      {Size: 1, ModUnix: 1},
	}
	got := changedPlotPaths(before, after)
	want := []string{"/tmp/out/existing.gbr", "/tmp/out/new.drl"}
	if len(got) != len(want) {
		t.Fatalf("changed paths = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("changed paths = %#v, want %#v", got, want)
		}
	}
}

func TestChangedPlotPathsExcludesUnchangedFiles(t *testing.T) {
	before := map[string]plotFileState{"/tmp/out/existing.gbr": {Size: 1, ModUnix: 1}}
	after := map[string]plotFileState{"/tmp/out/existing.gbr": {Size: 1, ModUnix: 1}, "/tmp/out/new.drl": {Size: 1, ModUnix: 1}}
	got := changedPlotPaths(before, after)
	if len(got) != 1 || got[0] != "/tmp/out/new.drl" {
		t.Fatalf("new paths = %#v, want only new.drl", got)
	}
}

func TestScanPlotPathsRejectsFileOutputPath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "not-a-dir")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := scanPlotPaths(path); err == nil {
		t.Fatalf("scanPlotPaths accepted file output path")
	}
}
