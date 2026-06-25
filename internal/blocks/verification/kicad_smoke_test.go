package verification

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kicadai/internal/blocks"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
)

func TestOptionalKiCadBlockSmoke(t *testing.T) {
	if os.Getenv(checks.EnvRunKiCadCLI) != "1" {
		t.Skipf("set %s=1 to run local KiCad block smoke verification", checks.EnvRunKiCadCLI)
	}
	cli, err := checks.DiscoverCLI("")
	if err != nil {
		t.Fatalf("discover kicad-cli: %v", err)
	}
	registry := blocks.NewBuiltinRegistry()
	for _, caseID := range []string{
		"esd_protection_5v",
		"reverse_polarity_schottky",
		"crystal_oscillator_default",
	} {
		t.Run(caseID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			manifest, issues := LoadManifest(filepath.Join("..", "testdata", "verification", caseID, "manifest.json"))
			if len(issues) != 0 {
				t.Fatalf("load issues = %#v", issues)
			}
			result := RunCase(ctx, manifest, RunOptions{
				Registry:  registry,
				OutputDir: filepath.Join(t.TempDir(), "out"),
				Overwrite: true,
				KiCadCLI:  cli.Path,
			})
			stage, ok := findStage(result.Stages, "erc_drc")
			if !ok {
				t.Fatalf("missing ERC/DRC stage: %#v", result.Stages)
			}
			if stage.Status == StatusSkipped {
				t.Fatalf("KiCad smoke stage skipped with configured CLI: %#v", stage)
			}
			if result.Status != StatusPass || reports.HasBlockingIssue(result.Issues) {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestOptionalKiCadBlockCorpusSmoke(t *testing.T) {
	if os.Getenv(checks.EnvRunKiCadCLI) != "1" {
		t.Skipf("set %s=1 to run local KiCad block corpus smoke verification", checks.EnvRunKiCadCLI)
	}
	cli, err := checks.DiscoverCLI("")
	if err != nil {
		t.Fatalf("discover kicad-cli: %v", err)
	}
	manifests, issues := LoadSuite(filepath.Join("..", "testdata", "verification"))
	if len(issues) != 0 {
		t.Fatalf("load suite issues = %#v", issues)
	}
	corpusOpts := KiCadCorpusOptions{
		Enabled: true,
		Tiers:   []KiCadCorpusTier{KiCadCorpusTierSmoke},
	}
	selected, summary := SelectKiCadCorpusManifests(manifests, corpusOpts)
	if len(selected) == 0 {
		t.Fatalf("no smoke corpus manifests selected: %#v", summary)
	}
	for _, manifest := range selected {
		t.Run(manifest.ID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			registry := blocks.NewBuiltinRegistry()
			result := RunCase(ctx, manifest, RunOptions{
				Registry:    registry,
				OutputDir:   t.TempDir(),
				Overwrite:   true,
				KiCadCorpus: corpusOpts,
				KiCadCLI:    cli.Path,
			})
			stage, ok := findStage(result.Stages, "erc_drc")
			if !ok {
				t.Fatalf("missing ERC/DRC stage: %#v", result.Stages)
			}
			if stage.Status == StatusSkipped {
				t.Fatalf("KiCad corpus smoke stage skipped with configured CLI: %#v", stage)
			}
			if result.KiCadCorpus == nil {
				t.Fatalf("missing corpus result: %#v", result)
			}
			if result.KiCadCorpus.Status == KiCadCorpusResultBlocked || reports.HasBlockingIssue(result.Issues) {
				t.Fatalf("case %s blocked: corpus=%#v issues=%#v stages=%#v", manifest.ID, result.KiCadCorpus, result.Issues, result.Stages)
			}
		})
	}
}
