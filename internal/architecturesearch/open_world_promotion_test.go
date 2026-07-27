package architecturesearch

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestOpenWorldPromotedRequirementsSelectGenericArchitectures(t *testing.T) {
	registry, registryIssues := NewCatalogRegistry(loadArchitectureCatalog(t))
	if len(registryIssues) != 0 {
		t.Fatal(registryIssues)
	}
	tests := []struct {
		file  string
		usage string
	}{
		{"heldout_analog_clock_fanout.json", "standalone_clock_source"},
		{"heldout_sensor_push_pull_translation.json", "push_pull_level_translator"},
		{"heldout_mcu_translated_debug.json", "programming_header"},
		{"heldout_mixed_signal_functional_isolation.json", "push_pull_functional_isolation"},
		{"heldout_power_protected_isolated_12v.json", "protected_isolated_power_stage"},
	}
	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			path := filepath.Join("testdata", "open_world_capability_promotion", test.file)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			requirement, issues := DecodeStrict(bytes.NewReader(data))
			if len(issues) != 0 {
				t.Fatalf("decode issues = %#v", issues)
			}
			result := Search(context.Background(), requirement, registry, SearchOptions{CatalogHash: registry.Hash()})
			if result.Status != SearchSelected || result.Selected == nil {
				t.Fatalf("search status = %s, issues=%#v rejections=%#v", result.Status, result.Issues, result.Rejections)
			}
			found := false
			for _, selection := range result.Selected.Selections {
				realization, err := DecodeFragmentRealization(selection.Payload)
				if err != nil {
					t.Fatal(err)
				}
				found = found || slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
					return instance.Usage == test.usage
				})
			}
			if !found {
				t.Fatalf("selected architecture lacks usage %q: %#v", test.usage, result.Selected.Selections)
			}
		})
	}
}
