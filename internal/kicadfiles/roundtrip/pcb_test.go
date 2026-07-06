package roundtrip

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestRoundTripPCBCopiesBeforeUpgrade(t *testing.T) {
	dir := t.TempDir()
	board := filepath.Join(dir, "board.kicad_pcb")
	writeTestFile(t, board, "(kicad_pcb)\n")
	logPath := filepath.Join(dir, "cli.log")
	cli := KiCadCLI{Path: fakeKiCadUpgradeCLI(t, logPath, 0)}
	artifactDir := filepath.Join(dir, "artifacts")

	result, err := RoundTripPCB(context.Background(), cli, board, Options{KeepArtifacts: true, ArtifactDir: artifactDir})
	if err != nil {
		t.Fatalf("RoundTripPCB returned error: %v", err)
	}
	if !result.Equal {
		t.Fatalf("Equal = false, differences = %#v", result.Differences)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake cli log: %v", err)
	}
	log := string(logBytes)
	if strings.Contains(log, board) {
		t.Fatalf("KiCad CLI was run against original fixture, log = %q", log)
	}
	if !strings.Contains(log, result.RoundTrippedPath) {
		t.Fatalf("KiCad CLI did not receive copied fixture, log = %q", log)
	}
}

func TestRoundTripPCBDoesNotReportArtifactsWhenNotKept(t *testing.T) {
	dir := t.TempDir()
	board := filepath.Join(dir, "board.kicad_pcb")
	writeTestFile(t, board, "(kicad_pcb)\n")
	cli := KiCadCLI{Path: fakeKiCadUpgradeCLI(t, filepath.Join(dir, "cli.log"), 0)}

	result, err := RoundTripPCB(context.Background(), cli, board, Options{ArtifactDir: filepath.Join(dir, "artifacts")})
	if err != nil {
		t.Fatalf("RoundTripPCB returned error: %v", err)
	}
	if result.RawDiffPath != "" || result.NormalizedDiffPath != "" || result.SummaryPath != "" {
		t.Fatalf("artifact paths should be empty when not kept: %#v", result)
	}
	if _, err := os.Stat(result.RoundTrippedPath); !os.IsNotExist(err) {
		t.Fatalf("round-tripped workspace file should be cleaned up, stat err = %v", err)
	}
}

func TestRoundTripPCBReportsArtifactsWhenKept(t *testing.T) {
	dir := t.TempDir()
	board := filepath.Join(dir, "board.kicad_pcb")
	writeTestFile(t, board, "(kicad_pcb)\n")
	cli := KiCadCLI{Path: fakeKiCadUpgradeCLI(t, filepath.Join(dir, "cli.log"), 0)}

	result, err := RoundTripPCB(context.Background(), cli, board, Options{KeepArtifacts: true, ArtifactDir: filepath.Join(dir, "artifacts")})
	if err != nil {
		t.Fatalf("RoundTripPCB returned error: %v", err)
	}
	if result.RawDiffPath == "" || result.NormalizedDiffPath == "" || result.SummaryPath == "" {
		t.Fatalf("artifact paths not set when kept: %#v", result)
	}
	for _, path := range []string{result.RawDiffPath, result.NormalizedDiffPath, result.SummaryPath, result.RoundTrippedPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected retained artifact %s: %v", path, err)
		}
	}
}

func TestComparePCBFilesTreatsKiCad10NameOnlyNetsAsEquivalent(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "original.kicad_pcb")
	roundTripped := filepath.Join(dir, "roundtripped.kicad_pcb")
	writeTestFile(t, original, `(kicad_pcb
  (version 20260206)
  (net 0 "")
  (net 1 "SIG")
  (footprint "Test:Part"
    (pad "1" smd rect (at 0 0) (size 1 1) (layers "F.Cu") (net 1 "SIG"))
  )
  (segment (start 0 0) (end 1 0) (width 0.25) (layer "F.Cu") (net 1))
)`)
	writeTestFile(t, roundTripped, `(kicad_pcb
  (version 20260206)
  (footprint "Test:Part"
    (pad "1" smd rect (at 0 0) (size 1 1) (layers "F.Cu") (net "SIG"))
  )
  (segment (start 0 0) (end 1 0) (width 0.25) (layer "F.Cu") (net "SIG"))
)`)

	result, err := ComparePCBFiles(original, roundTripped, Options{})
	if err != nil {
		t.Fatalf("ComparePCBFiles returned error: %v", err)
	}
	if !result.Equal {
		t.Fatalf("Equal = false, differences = %#v", result.Differences)
	}
}

func TestEquivalentPCBSummariesIgnoresOnlyNetTableCount(t *testing.T) {
	original := Summary{Sections: map[string]int{"net": 2, "footprint": 1, "segment": 1}}
	roundTripped := Summary{Sections: map[string]int{"footprint": 1, "segment": 1}}

	if !equivalentPCBSummaries(original, roundTripped) {
		t.Fatal("expected summaries to match after omitting net table")
	}
	roundTripped.Sections["segment"] = 2
	if equivalentPCBSummaries(original, roundTripped) {
		t.Fatal("unexpected match with non-net section change")
	}
}

func TestRoundTripPCBReportsKiCadFailure(t *testing.T) {
	dir := t.TempDir()
	board := filepath.Join(dir, "board.kicad_pcb")
	writeTestFile(t, board, "(kicad_pcb)\n")
	cli := KiCadCLI{Path: fakeKiCadUpgradeCLI(t, filepath.Join(dir, "cli.log"), 3)}

	result, err := RoundTripPCB(context.Background(), cli, board, Options{ArtifactDir: filepath.Join(dir, "artifacts")})
	if err == nil {
		t.Fatal("expected error")
	}
	if result.ExitCode != 3 {
		t.Fatalf("ExitCode = %d, want 3", result.ExitCode)
	}
	if !strings.Contains(err.Error(), "upgrade failed") {
		t.Fatalf("error = %v", err)
	}
}

func fakeKiCadUpgradeCLI(t *testing.T, logPath string, upgradeExit int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kicad-cli")
	var body string
	if runtime.GOOS == "windows" {
		path += ".bat"
		body = "@echo off\r\n" +
			"if \"%1\"==\"--version\" echo 10.0.0& exit /b 0\r\n" +
			"echo %* > \"" + logPath + "\"\r\n" +
			"if not \"" + strconv.Itoa(upgradeExit) + "\"==\"0\" echo failed 1>&2\r\n" +
			"exit /b " + strconv.Itoa(upgradeExit) + "\r\n"
	} else {
		body = "#!/bin/sh\n" +
			"if [ \"$1\" = \"--version\" ]; then echo 10.0.0; exit 0; fi\n" +
			"printf '%s\\n' \"$*\" > '" + logPath + "'\n"
		if upgradeExit != 0 {
			body += "printf '%s\\n' failed >&2\n"
		}
		body += "exit " + strconv.Itoa(upgradeExit) + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake KiCad CLI: %v", err)
	}
	return path
}
