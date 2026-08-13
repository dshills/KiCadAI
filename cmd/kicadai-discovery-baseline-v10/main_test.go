package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRequiresFreshWorkingAndReportRoots(t *testing.T) {
	err := run(context.Background(), []string{"--repository-root", t.TempDir()}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--working-root and --report") {
		t.Fatalf("missing output error = %v", err)
	}
}

func TestNormalizeOptionsRejectsExistingReport(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "report.json")
	if err := writeAtomicNoReplace(report, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	opts := options{repositoryRoot: root, workingRoot: filepath.Join(root, "work"), reportPath: report, timeout: 1}
	if err := normalizeOptions(&opts); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing report error = %v", err)
	}
}
