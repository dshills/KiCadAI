package blocks

import (
	"slices"
	"strings"
	"testing"
)

func TestBuiltinInventoryIncludesRoadmapFamilies(t *testing.T) {
	inventory := NewBuiltinRegistry().Inventory()
	got := make([]string, 0, len(inventory.Families))
	for _, family := range inventory.Families {
		got = append(got, family.ID)
	}
	want := []string{
		"amplifier_bias_network",
		"amplifier_gain_stage",
		"amplifier_input_buffer",
		"amplifier_output_protection",
		"amplifier_power_entry",
		"amplifier_stability_network",
		"amplifier_supply_decoupling",
		"canned_oscillator",
		"class_a_output_stage",
		"class_ab_output_pair",
		"class_ab_output_stage",
		"connector_breakout",
		"crystal_oscillator",
		"esd_protection",
		"headphone_output_connector",
		"i2c_sensor",
		"led_indicator",
		"mcu_minimal",
		"opamp_gain_stage",
		"reset_programming_header",
		"reverse_polarity_protection",
		"speaker_output_connector",
		"usb_c_power",
		"voltage_regulator",
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("families = %#v, want %#v", got, want)
	}
}

func TestAmplifierFamilyInventoryMatchesVerifiedBlockPlan(t *testing.T) {
	inventory := NewBuiltinRegistry().Inventory()
	want := map[string]string{
		"amplifier_input_buffer":      "input",
		"amplifier_gain_stage":        "opamp_gain_stage",
		"amplifier_bias_network":      "quiescent",
		"class_ab_output_pair":        "SOA",
		"amplifier_output_protection": "headphone_output_protection",
		"amplifier_supply_decoupling": "decoupling",
		"headphone_output_connector":  "TRS",
		"speaker_output_connector":    "high-current",
	}
	for id, gapFragment := range want {
		family, ok := inventoryFamily(inventory, id)
		if !ok {
			t.Fatalf("missing verified amplifier family inventory entry %s", id)
		}
		if family.Implemented {
			t.Fatalf("%s should be explicit planned/unsupported entry until its contract is implemented: %#v", id, family)
		}
		if family.Readiness != BlockReadinessUnsupported {
			t.Fatalf("%s readiness = %q", id, family.Readiness)
		}
		if !inventoryGapsContain(family.Gaps, gapFragment) {
			t.Fatalf("%s gaps = %#v, want fragment %q", id, family.Gaps, gapFragment)
		}
	}
}

func TestAmplifierInventoryDeclaresUnsupportedGaps(t *testing.T) {
	inventory := NewBuiltinRegistry().Inventory()
	ids := unsupportedAmplifierRoadmapIDs()
	if len(ids) == 0 {
		t.Fatal("no amplifier roadmap IDs found")
	}
	for _, id := range ids {
		family, ok := inventoryFamily(inventory, id)
		if !ok {
			t.Fatalf("missing amplifier inventory family %s", id)
		}
		if family.Implemented {
			t.Fatalf("%s should be explicit unsupported gap, got implemented family %#v", id, family)
		}
		if family.Readiness != BlockReadinessUnsupported {
			t.Fatalf("%s readiness = %q", id, family.Readiness)
		}
		if len(family.Gaps) == 0 {
			t.Fatalf("%s missing unsupported gap detail", id)
		}
	}
}

func inventoryGapsContain(gaps []string, fragment string) bool {
	for _, gap := range gaps {
		if strings.Contains(gap, fragment) {
			return true
		}
	}
	return false
}

func unsupportedAmplifierRoadmapIDs() []string {
	implemented := map[string]bool{}
	for _, summary := range NewBuiltinRegistry().ListBlocks() {
		implemented[summary.ID] = true
	}
	var ids []string
	for _, family := range roadmapBlockFamilies {
		if len(family.Gaps) == 0 {
			continue
		}
		if implemented[family.ID] {
			continue
		}
		if slices.Contains(family.Tags, "amplifier") {
			ids = append(ids, family.ID)
		}
	}
	slices.Sort(ids)
	return ids
}

func TestOpAmpGainStageInventoryCarriesAnalogRules(t *testing.T) {
	inventory := NewBuiltinRegistry().Inventory()
	opamp, ok := inventoryFamily(inventory, "opamp_gain_stage")
	if !ok {
		t.Fatalf("missing opamp inventory")
	}
	if opamp.Readiness != BlockReadinessPartial {
		t.Fatalf("opamp readiness = %q", opamp.Readiness)
	}
	if !slices.Contains(opamp.ExportedPorts, "IN") || !slices.Contains(opamp.ExportedPorts, "OUT") {
		t.Fatalf("opamp ports = %#v", opamp.ExportedPorts)
	}
	if !slices.Contains(opamp.ElectricalRules, opampFeedbackProximityRuleID) {
		t.Fatalf("opamp electrical rules = %#v", opamp.ElectricalRules)
	}
	if len(opamp.PCBRules) == 0 {
		t.Fatalf("opamp missing PCB rules: %#v", opamp)
	}
}

func inventoryFamily(inventory BlockLibraryInventory, id string) (BlockFamilyInventory, bool) {
	for _, family := range inventory.Families {
		if family.ID == id {
			return family, true
		}
	}
	return BlockFamilyInventory{}, false
}

func TestBuiltinInventorySummarizesImplementedBlockRules(t *testing.T) {
	inventory := NewBuiltinRegistry().Inventory()
	var led BlockFamilyInventory
	for _, family := range inventory.Families {
		if family.ID == "led_indicator" {
			led = family
			break
		}
	}
	if !led.Implemented {
		t.Fatalf("led inventory missing: %#v", inventory.Families)
	}
	if led.Readiness != BlockReadinessPartial {
		t.Fatalf("led readiness = %q", led.Readiness)
	}
	if !slices.Contains(led.RequiredRoles, "led") || !slices.Contains(led.RequiredRoles, "resistor") {
		t.Fatalf("led roles = %#v", led.RequiredRoles)
	}
	if !slices.Contains(led.ExportedPorts, "IN") || !slices.Contains(led.ExportedPorts, "GND") {
		t.Fatalf("led ports = %#v", led.ExportedPorts)
	}
	if len(led.PCBRules) == 0 {
		t.Fatalf("led missing PCB rules: %#v", led)
	}
	if len(led.ElectricalRules) == 0 {
		t.Fatalf("led missing electrical rules: %#v", led)
	}
}
