package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunRequiresFreshWorkingAndReportRoots(t *testing.T) {
	if evaluatorVersion != 23 {
		t.Fatalf("V23 evaluator version = %d", evaluatorVersion)
	}
	err := run(context.Background(), []string{"--repository-root", t.TempDir()}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--working-root and --report") {
		t.Fatalf("missing output error = %v", err)
	}
}

func TestNormalizeOptionsUsesV23ManifestAndRejectsExistingReport(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "report.json")
	if err := writeAtomicNoReplace(report, []byte("{}"), false); err != nil {
		t.Fatal(err)
	}
	opts := options{repositoryRoot: root, workingRoot: filepath.Join(root, "work"), reportPath: report, timeout: time.Second}
	if err := normalizeOptions(&opts); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing report error = %v", err)
	}
	if !strings.HasSuffix(opts.evaluatorManifest, filepath.Join("electrical-repair-followup-v23", "public-evaluation-v23", "EVALUATOR.sha256")) {
		t.Fatalf("V23 evaluator manifest = %q", opts.evaluatorManifest)
	}
	if !strings.HasSuffix(opts.selectedPopulation, filepath.Join("electrical-repair-followup-v23", "V23_PUBLIC_POPULATION.json")) {
		t.Fatalf("V23 selected population = %q", opts.selectedPopulation)
	}
}

func TestLoadV23SelectedCaseIDsRequiresFrozenShape(t *testing.T) {
	path := filepath.Join("..", "..", "specs", "electrical-repair-followup-v23", "V23_PUBLIC_POPULATION.json")
	ids, err := loadSelectedCaseIDs(path, selectedPopulationSHA256, "kicadai.public-electrical-population.v23", 23, 4)
	if err != nil || len(ids) != 4 {
		t.Fatalf("selected population = %v, %v", ids, err)
	}
	invalid := filepath.Join(t.TempDir(), "population.json")
	if err := os.WriteFile(invalid, []byte(`{"schema":"kicadai.public-causal-topology-population.v23","version":21,"selected_cases":[{"id":"same"},{"id":"same"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSelectedCaseIDs(invalid, selectedPopulationSHA256, "kicadai.public-electrical-population.v23", 23, 4); err == nil {
		t.Fatal("invalid selected population was accepted")
	}
}

func TestWriteAtomicNoReplaceRefusesReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	if err := writeAtomicNoReplace(path, []byte("first"), false); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomicNoReplace(path, []byte("second"), false); err == nil {
		t.Fatal("V23 report replacement succeeded")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first" {
		t.Fatalf("V23 report changed: %q, %v", data, err)
	}
}

func TestLoadSelectedCaseIDsReportsRequestedVersion(t *testing.T) {
	for _, version := range []int{21, 23} {
		_, err := loadSelectedCaseIDs(filepath.Join(t.TempDir(), "missing.json"), "", "", version, 4)
		if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("open V%d selected", version)) {
			t.Fatalf("version %d diagnostic = %v", version, err)
		}
	}
}
