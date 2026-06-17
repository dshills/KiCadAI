package verification

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
)

func TestLoadSuiteDiscoversBuiltInCorpus(t *testing.T) {
	manifests, issues := LoadSuite(filepath.Join("..", "testdata", "verification"))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	got := manifestIDs(manifests)
	want := []string{
		"connector_breakout_4pin",
		"i2c_sensor_pullups",
		"led_indicator_default",
		"mcu_minimal_basic",
		"opamp_gain_stage_noninverting",
		"usb_c_power_5v_sink",
		"voltage_regulator_3v3",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ids = %#v, want %#v", got, want)
	}
}

func TestEveryBuiltinBlockHasManifest(t *testing.T) {
	registry := blocks.NewBuiltinRegistry()
	manifests, issues := LoadSuite(filepath.Join("..", "testdata", "verification"))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	covered := map[string]struct{}{}
	for _, manifest := range manifests {
		covered[manifest.BlockID] = struct{}{}
	}
	for _, summary := range registry.ListBlocks() {
		if _, ok := covered[summary.ID]; !ok {
			t.Fatalf("missing verification manifest for built-in block %s", summary.ID)
		}
	}
}

func TestBuiltInManifestsValidateAndRun(t *testing.T) {
	registry := blocks.NewBuiltinRegistry()
	manifests, issues := LoadSuite(filepath.Join("..", "testdata", "verification"))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	for _, manifest := range manifests {
		t.Run(manifest.ID, func(t *testing.T) {
			if issues := ValidateManifest(manifest, registry); len(issues) != 0 {
				t.Fatalf("validate issues = %#v", issues)
			}
			result := RunCase(context.Background(), manifest, RunOptions{Registry: registry})
			if result.Status == StatusBlocked || reports.HasBlockingIssue(result.Issues) {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestDiscoverManifestPathsSorted(t *testing.T) {
	paths, issues := DiscoverManifestPaths(filepath.Join("..", "testdata", "verification"))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if !slices.IsSorted(paths) {
		t.Fatalf("paths are not sorted: %#v", paths)
	}
}

func manifestIDs(manifests []Manifest) []string {
	ids := make([]string, 0, len(manifests))
	for _, manifest := range manifests {
		ids = append(ids, manifest.ID)
	}
	return ids
}
