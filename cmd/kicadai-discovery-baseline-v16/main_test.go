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

func TestNormalizeOptionsUsesV16ManifestAndRejectsExistingReport(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "report.json")
	if err := writeAtomicNoReplace(report, []byte("{}"), false); err != nil {
		t.Fatal(err)
	}
	opts := options{repositoryRoot: root, workingRoot: filepath.Join(root, "work"), reportPath: report, timeout: time.Second}
	if err := normalizeOptions(&opts); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing report error = %v", err)
	}
	if !strings.HasSuffix(opts.evaluatorManifest, filepath.Join("closed-loop-open-set-capability-expansion", "V16_EVALUATOR.sha256")) {
		t.Fatalf("V16 evaluator manifest = %q", opts.evaluatorManifest)
	}
}

func TestWriteAtomicNoReplaceRefusesReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	if err := writeAtomicNoReplace(path, []byte("first"), false); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomicNoReplace(path, []byte("second"), false); err == nil {
		t.Fatal("V16 report replacement succeeded")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first" {
		t.Fatalf("V16 report changed: %q, %v", data, err)
	}
}
