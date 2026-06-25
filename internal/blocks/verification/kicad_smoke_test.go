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
