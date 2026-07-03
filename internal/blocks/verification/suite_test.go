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
		"canned_oscillator_default",
		"class_ab_output_stage_headphone",
		"connector_breakout_4pin",
		"crystal_oscillator_default",
		"dc_blocking_capacitor_220uf",
		"esd_protection_5v",
		"headphone_output_protection_32ohm",
		"i2c_sensor_pullups",
		"led_indicator_default",
		"mcu_minimal_basic",
		"opamp_gain_stage_noninverting",
		"reset_programming_header_isp",
		"reverse_polarity_schottky",
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

func TestBuiltInKiCadCorpusSmokeCases(t *testing.T) {
	manifests, issues := LoadSuite(filepath.Join("..", "testdata", "verification"))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	selected, summary := SelectKiCadCorpusManifests(manifests, KiCadCorpusOptions{
		Enabled: true,
		Tiers:   []KiCadCorpusTier{KiCadCorpusTierSmoke},
	})
	got := manifestIDs(selected)
	want := []string{"connector_breakout_4pin", "led_indicator_default"}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("smoke corpus = %#v, want %#v summary=%#v", got, want, summary)
	}
	if summary.SelectedCount != 2 || summary.CountsByTier[KiCadCorpusTierSmoke] != 2 {
		t.Fatalf("summary = %#v", summary)
	}
	for _, manifest := range selected {
		if manifest.Expected.KiCadCorpus.Readiness != KiCadCorpusReadinessCandidate {
			t.Fatalf("%s readiness = %s", manifest.ID, manifest.Expected.KiCadCorpus.Readiness)
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

func TestBuiltInBlocksDeclareConsistentRequiredRoutes(t *testing.T) {
	registry := blocks.NewBuiltinRegistry()
	for _, summary := range registry.ListBlocks() {
		t.Run(summary.ID, func(t *testing.T) {
			definition, ok := registry.GetBlock(summary.ID)
			if !ok {
				t.Fatalf("missing block %s", summary.ID)
			}
			if definition.PCBRealization == nil {
				return
			}
			defined := make(map[string]struct{}, len(definition.PCBRealization.LocalRoutes))
			for _, route := range definition.PCBRealization.LocalRoutes {
				if route.ID == "" {
					t.Errorf("local route with empty ID: %#v", route)
					continue
				}
				if _, exists := defined[route.ID]; exists {
					t.Errorf("duplicate local route ID %s", route.ID)
				}
				defined[route.ID] = struct{}{}
			}
			for _, required := range definition.PCBRealization.Validation.RequiredRoutes {
				if _, ok := defined[required]; !ok {
					t.Errorf("required route %s has no local route definition", required)
				}
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
