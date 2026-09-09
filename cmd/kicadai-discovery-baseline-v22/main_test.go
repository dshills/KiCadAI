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
	if evaluatorVersion != 22 {
		t.Fatalf("V22 evaluator version = %d", evaluatorVersion)
	}
	err := run(context.Background(), []string{"--repository-root", t.TempDir()}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--working-root and --report") {
		t.Fatalf("missing output error = %v", err)
	}
}

func TestNormalizeOptionsUsesV22ManifestAndRejectsExistingReport(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "report.json")
	if err := writeAtomicNoReplace(report, []byte("{}"), false); err != nil {
		t.Fatal(err)
	}
	opts := options{repositoryRoot: root, workingRoot: filepath.Join(root, "work"), reportPath: report, timeout: time.Second}
	if err := normalizeOptions(&opts); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing report error = %v", err)
	}
	if !strings.HasSuffix(opts.evaluatorManifest, filepath.Join("post-topology-electrical-blockers", "public-evaluation-v22", "EVALUATOR.sha256")) {
		t.Fatalf("V22 evaluator manifest = %q", opts.evaluatorManifest)
	}
	if !strings.HasSuffix(opts.selectedPopulation, filepath.Join("post-topology-electrical-blockers", "V22_PUBLIC_POPULATION.json")) {
		t.Fatalf("V22 selected population = %q", opts.selectedPopulation)
	}
}

func TestLoadV22SelectedCaseIDsRequiresFrozenShape(t *testing.T) {
	path := filepath.Join("..", "..", "specs", "post-topology-electrical-blockers", "V22_PUBLIC_POPULATION.json")
	ids, err := loadSelectedCaseIDs(path, selectedPopulationSHA256, "kicadai.public-electrical-population.v22", 22, 4)
	if err != nil || len(ids) != 4 {
		t.Fatalf("selected population = %v, %v", ids, err)
	}
	invalid := filepath.Join(t.TempDir(), "population.json")
	if err := os.WriteFile(invalid, []byte(`{"schema":"kicadai.public-causal-topology-population.v22","version":21,"selected_cases":[{"id":"same"},{"id":"same"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSelectedCaseIDs(invalid, selectedPopulationSHA256, "kicadai.public-electrical-population.v22", 22, 4); err == nil {
		t.Fatal("invalid selected population was accepted")
	}
}

func TestWriteAtomicNoReplaceRefusesReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	if err := writeAtomicNoReplace(path, []byte("first"), false); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomicNoReplace(path, []byte("second"), false); err == nil {
		t.Fatal("V22 report replacement succeeded")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first" {
		t.Fatalf("V22 report changed: %q, %v", data, err)
	}
}
