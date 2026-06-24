package blocks

import (
	"slices"
	"testing"
)

func TestBuiltinInventoryIncludesRoadmapFamilies(t *testing.T) {
	inventory := NewBuiltinRegistry().Inventory()
	got := make([]string, 0, len(inventory.Families))
	for _, family := range inventory.Families {
		got = append(got, family.ID)
	}
	want := []string{
		"canned_oscillator",
		"connector_breakout",
		"crystal_oscillator",
		"esd_protection",
		"i2c_sensor",
		"led_indicator",
		"mcu_minimal",
		"opamp_gain_stage",
		"reset_programming_header",
		"reverse_polarity_protection",
		"usb_c_power",
		"voltage_regulator",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("families = %#v, want %#v", got, want)
	}
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
