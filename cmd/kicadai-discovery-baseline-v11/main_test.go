package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunRequiresFreshWorkingAndReportRoots(t *testing.T) {
	err := run(context.Background(), []string{"--repository-root", t.TempDir()}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--working-root and --report") {
		t.Fatalf("missing output error = %v", err)
	}
}

func TestNormalizeOptionsUsesV11ManifestAndRejectsExistingReport(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "report.json")
	if err := writeAtomicNoReplace(report, []byte("{}"), false); err != nil {
		t.Fatal(err)
	}
	opts := options{repositoryRoot: root, workingRoot: filepath.Join(root, "work"), reportPath: report, timeout: time.Second}
	if err := normalizeOptions(&opts); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing report error = %v", err)
	}
	if !strings.HasSuffix(opts.evaluatorManifest, filepath.Join("closed-loop-open-set-capability-expansion", "V11_EVALUATOR.sha256")) {
		t.Fatalf("V11 evaluator manifest = %q", opts.evaluatorManifest)
	}
}

func TestWriteAtomicNoReplaceRefusesReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	if err := writeAtomicNoReplace(path, []byte("first"), false); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomicNoReplace(path, []byte("second"), false); err == nil {
		t.Fatal("V11 report replacement succeeded")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first" {
		t.Fatalf("V11 report changed: %q, %v", data, err)
	}
	linePath := filepath.Join(t.TempDir(), "line.json")
	if err := writeAtomicNoReplace(linePath, []byte("line"), true); err != nil {
		t.Fatal(err)
	}
	line, err := os.ReadFile(linePath)
	if err != nil || string(line) != "line\n" {
		t.Fatalf("V11 report newline = %q, %v", line, err)
	}
}
