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
