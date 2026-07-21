package architecturesearch

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNeutralMCUSynthesisCorpusSelectsAndReplaysDeterministically(t *testing.T) {
	wants := map[string]string{
		"five_volt_serial_controller.json":            "mcu.microchip.atmega328p_a.tqfp32",
		"wireless_sensor_controller.json":             "mcu.espressif.esp32_wroom_32e",
		"debuggable_mixed_peripheral_controller.json": "mcu.st.stm32g031k8t6.lqfp32",
	}
	root := filepath.Join("testdata", "mcu_synthesis_corpus")
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(wants) {
		t.Fatalf("MCU corpus entries = %d, want %d", len(entries), len(wants))
	}
	for _, entry := range entries {
		name := entry.Name()
		want, exists := wants[name]
		if !exists || entry.IsDir() {
			t.Fatalf("unexpected MCU corpus entry %q", name)
		}
		t.Run(name, func(t *testing.T) {
			contents, readErr := os.ReadFile(filepath.Join(root, name))
			if readErr != nil {
				t.Fatal(readErr)
			}
			requirement, decodeIssues := DecodeStrict(bytes.NewReader(contents))
			if len(decodeIssues) != 0 {
				t.Fatalf("decode issues = %#v", decodeIssues)
			}
			obligations, obligationIssues := initialSearchObligations(requirement, EvidenceRuleInferred)
			if len(obligationIssues) != 0 {
				t.Fatalf("obligation issues = %#v", obligationIssues)
			}
			var controller searchObligation
			for _, obligation := range obligations {
				if obligation.Capability == "programmable_controller" {
					controller = obligation
				}
			}
			if controller.Capability == "" {
				t.Fatal("controller obligation is missing")
			}
			request := providerRequestFor(controller, requirement.Requirements.Constraints)
			first, expandErr := provider.Expand(context.Background(), request)
			if expandErr != nil || len(first) == 0 {
				t.Fatalf("expand = %#v, %v", first, expandErr)
			}
			if len(first[0].Components) == 0 || first[0].Components[0].CatalogID != want {
				t.Fatalf("selected components = %#v, want controller %s", first[0].Components, want)
			}
			firstJSON, marshalErr := json.Marshal(first)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			second, secondErr := provider.Expand(context.Background(), request)
			if secondErr != nil {
				t.Fatal(secondErr)
			}
			secondJSON, _ := json.Marshal(second)
			if !bytes.Equal(firstJSON, secondJSON) {
				t.Fatal("MCU search replay differs")
			}
			search := Search(context.Background(), requirement, mustMCUCorpusRegistry(t), SearchOptions{})
			if search.Status != SearchSelected || search.Selected == nil {
				t.Fatalf("architecture search = %#v", search)
			}
			selected := false
			for _, selection := range search.Selected.Selections {
				for _, component := range selection.Components {
					selected = selected || component.CatalogID == want
				}
			}
			if !selected {
				t.Fatalf("selected architecture does not contain %s: %#v", want, search.Selected.Selections)
			}
		})
	}
}

func mustMCUCorpusRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, issues := NewCatalogRegistry(loadArchitectureCatalog(t))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	return registry
}
