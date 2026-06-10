package roundtrip

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEnabledFromEnv(t *testing.T) {
	t.Setenv(envRunKiCadCLI, "")
	if EnabledFromEnv() {
		t.Fatal("EnabledFromEnv enabled without opt-in")
	}

	t.Setenv(envRunKiCadCLI, "true")
	if EnabledFromEnv() {
		t.Fatal("EnabledFromEnv enabled for non-1 value")
	}

	t.Setenv(envRunKiCadCLI, "1")
	if !EnabledFromEnv() {
		t.Fatal("EnabledFromEnv disabled for 1")
	}
}

func TestOptionsFromEnv(t *testing.T) {
	t.Setenv(envKeepArtifacts, "1")
	t.Setenv(envArtifactDir, "/tmp/kicadai-roundtrip")
	t.Setenv(envTimeout, "2m")

	opts := OptionsFromEnv()
	if !opts.KeepArtifacts {
		t.Fatal("KeepArtifacts = false, want true")
	}
	if opts.ArtifactDir != "/tmp/kicadai-roundtrip" {
		t.Fatalf("ArtifactDir = %q", opts.ArtifactDir)
	}
	if opts.Timeout != 2*time.Minute {
		t.Fatalf("Timeout = %s", opts.Timeout)
	}
}

func TestOptionsFromEnvDefaultsTimeout(t *testing.T) {
	t.Setenv(envTimeout, "not-a-duration")

	opts := OptionsFromEnv()
	if opts.Timeout != defaultTimeout {
		t.Fatalf("Timeout = %s, want %s", opts.Timeout, defaultTimeout)
	}
}

func TestDiscoverCLIHonerConfiguredPath(t *testing.T) {
	path := fakeExecutable(t, "kicad-cli")
	t.Setenv(envKiCadCLI, path)

	cli, err := DiscoverCLI()
	if err != nil {
		t.Fatalf("DiscoverCLI returned error: %v", err)
	}
	if cli.Path != path {
		t.Fatalf("Path = %q, want %q", cli.Path, path)
	}
}

func TestDiscoverCLIRejectsMissingConfiguredPath(t *testing.T) {
	t.Setenv(envKiCadCLI, filepath.Join(t.TempDir(), "missing-kicad-cli"))

	_, err := DiscoverCLI()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestKiCadCLIVersion(t *testing.T) {
	path := fakeCommand(t, "kicad-cli", "10.0.0", "", 0)
	cli := KiCadCLI{Path: path}

	version, err := cli.Version(context.Background())
	if err != nil {
		t.Fatalf("Version returned error: %v", err)
	}
	if version != "10.0.0" {
		t.Fatalf("Version = %q", version)
	}
}

func TestKiCadCLIVersionErrorIncludesPath(t *testing.T) {
	path := fakeCommand(t, "kicad-cli", "", "nope", 3)
	cli := KiCadCLI{Path: path}

	_, err := cli.Version(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got == "" || !containsAll(got, path, "nope") {
		t.Fatalf("error = %q", got)
	}
}

func TestNormalizeText(t *testing.T) {
	input := "a  \r\nb\t\r\n\r\n"
	want := "a\nb\n"

	if got := NormalizeText(input); got != want {
		t.Fatalf("NormalizeText = %q, want %q", got, want)
	}
}

func TestNormalizeTextPreservesSemanticLayerChanges(t *testing.T) {
	a := NormalizeText("(layers\n  (1 \"F.Mask\" user)\n)\n")
	b := NormalizeText("(layers\n  (39 \"F.Mask\" user)\n)\n")

	if a == b {
		t.Fatal("normalization hid semantic layer-number change")
	}
}

func TestCompareFilesEqualAfterNormalization(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "original.kicad_pcb")
	roundTripped := filepath.Join(dir, "roundtripped.kicad_pcb")
	writeTestFile(t, original, "a  \r\nb\r\n")
	writeTestFile(t, roundTripped, "a\nb\n")

	result, err := CompareFiles(original, roundTripped, Options{})
	if err != nil {
		t.Fatalf("CompareFiles returned error: %v", err)
	}
	if !result.Equal {
		t.Fatalf("Equal = false, differences = %#v", result.Differences)
	}
}

func TestCompareFilesReportsNormalizedDifference(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "original.kicad_pcb")
	roundTripped := filepath.Join(dir, "roundtripped.kicad_pcb")
	writeTestFile(t, original, "(layers\n  (1 \"F.Mask\" user)\n)\n")
	writeTestFile(t, roundTripped, "(layers\n  (39 \"F.Mask\" user)\n)\n")

	result, err := CompareFiles(original, roundTripped, Options{})
	if err != nil {
		t.Fatalf("CompareFiles returned error: %v", err)
	}
	if result.Equal {
		t.Fatal("Equal = true, want false")
	}
	if len(result.Differences) == 0 {
		t.Fatal("expected differences")
	}
	if !strings.Contains(result.Differences[0].Message, "F.Mask") {
		t.Fatalf("difference message = %q", result.Differences[0].Message)
	}
}

func TestCompareFilesWritesArtifacts(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "original.kicad_pcb")
	roundTripped := filepath.Join(dir, "roundtripped.kicad_pcb")
	artifactDir := filepath.Join(dir, "artifacts")
	writeTestFile(t, original, "a\n")
	writeTestFile(t, roundTripped, "b\n")

	result, err := CompareFiles(original, roundTripped, Options{KeepArtifacts: true, ArtifactDir: artifactDir})
	if err != nil {
		t.Fatalf("CompareFiles returned error: %v", err)
	}
	if result.RawDiffPath == "" || result.NormalizedDiffPath == "" {
		t.Fatalf("diff paths not set: %#v", result)
	}
	if _, err := os.Stat(result.RawDiffPath); err != nil {
		t.Fatalf("raw diff missing: %v", err)
	}
	if _, err := os.Stat(result.NormalizedDiffPath); err != nil {
		t.Fatalf("normalized diff missing: %v", err)
	}
}

func TestUnifiedDiffHandlesInsertedLines(t *testing.T) {
	diff := unifiedDiff("a", "b", "one\ntwo\n", "one\ninserted\ntwo\n")

	if !strings.Contains(diff, "+inserted\n") {
		t.Fatalf("inserted line missing from diff:\n%s", diff)
	}
	if strings.Count(diff, "-two\n") != 0 {
		t.Fatalf("unchanged shifted line marked as removed:\n%s", diff)
	}
}

func TestUnifiedDiffUsesCompactFallbackForLargeInputs(t *testing.T) {
	var aLines, bLines []string
	for i := 0; i < 2100; i++ {
		aLines = append(aLines, "same\n")
		bLines = append(bLines, "same\n")
	}
	aLines = append(aLines, "original\n")
	bLines = append(bLines, "roundtripped\n")
	for i := 0; i < 2100; i++ {
		aLines = append(aLines, "tail\n")
		bLines = append(bLines, "tail\n")
	}

	ops, startLine := compactDiffLineOps(aLines, bLines)
	var got strings.Builder
	for _, op := range ops {
		got.WriteString(op.line)
	}
	if startLine <= 1 {
		t.Fatalf("startLine = %d, want context near changed lines", startLine)
	}
	if !strings.Contains(got.String(), "large diff omitted") {
		t.Fatalf("compact fallback marker missing:\n%s", got.String())
	}
}

func TestUnifiedDiffTerminatesLinesWithoutFinalNewline(t *testing.T) {
	diff := unifiedDiff("a", "b", "old", "new")

	if strings.Contains(diff, "-old+new") {
		t.Fatalf("diff lines were concatenated:\n%s", diff)
	}
	if !strings.Contains(diff, "-old\n") || !strings.Contains(diff, "+new\n") {
		t.Fatalf("diff did not contain terminated changed lines:\n%s", diff)
	}
}

func fakeExecutable(t *testing.T, name string) string {
	t.Helper()
	return fakeCommand(t, name, "", "", 0)
}

func fakeCommand(t *testing.T, name, stdout, stderr string, exitCode int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	var body string
	if runtime.GOOS == "windows" {
		path += ".bat"
		body = "@echo off\r\n"
		if stdout != "" {
			body += "echo " + stdout + "\r\n"
		}
		if stderr != "" {
			body += "echo " + stderr + " 1>&2\r\n"
		}
		body += "exit /b " + strconv.Itoa(exitCode) + "\r\n"
	} else {
		body = "#!/bin/sh\n"
		if stdout != "" {
			body += "printf '%s\\n' '" + stdout + "'\n"
		}
		if stderr != "" {
			body += "printf '%s\\n' '" + stderr + "' >&2\n"
		}
		body += "exit " + strconv.Itoa(exitCode) + "\n"
	}
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write fake script: %v", err)
	}
	return path
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}

func containsAll(s string, values ...string) bool {
	for _, value := range values {
		if !strings.Contains(s, value) {
			return false
		}
	}
	return true
}
