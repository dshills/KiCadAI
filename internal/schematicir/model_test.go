package schematicir

import (
	"encoding/json"
	"testing"
)

func TestNewDocumentDefaults(t *testing.T) {
	doc := NewDocument()

	if doc.Schema != SchemaID {
		t.Fatalf("schema = %q, want %q", doc.Schema, SchemaID)
	}
	if doc.Version != Version {
		t.Fatalf("version = %d, want %d", doc.Version, Version)
	}
	if doc.Metadata.Paper != "A4" {
		t.Fatalf("paper = %q, want A4", doc.Metadata.Paper)
	}
	if doc.Layout.Flow != FlowLeftToRight {
		t.Fatalf("flow = %q, want %q", doc.Layout.Flow, FlowLeftToRight)
	}
	if doc.Layout.Origin != OriginCentered {
		t.Fatalf("origin = %q, want %q", doc.Layout.Origin, OriginCentered)
	}
	if doc.Layout.Lanes.Power != LanePositionTop {
		t.Fatalf("power lane = %q, want %q", doc.Layout.Lanes.Power, LanePositionTop)
	}
	if doc.Layout.Lanes.Ground != LanePositionBottom {
		t.Fatalf("ground lane = %q, want %q", doc.Layout.Lanes.Ground, LanePositionBottom)
	}
	if doc.Layout.Lanes.Signals != LanePositionMiddle {
		t.Fatalf("signal lane = %q, want %q", doc.Layout.Lanes.Signals, LanePositionMiddle)
	}
	if doc.Layout.Rules.MinGroupSpacingMM == nil || *doc.Layout.Rules.MinGroupSpacingMM != DefaultMinGroupSpacingMM {
		t.Fatalf("group spacing = %v, want %v", doc.Layout.Rules.MinGroupSpacingMM, DefaultMinGroupSpacingMM)
	}
	if doc.Layout.Rules.MinComponentSpacingMM == nil || *doc.Layout.Rules.MinComponentSpacingMM != DefaultMinComponentSpacingMM {
		t.Fatalf("component spacing = %v, want %v", doc.Layout.Rules.MinComponentSpacingMM, DefaultMinComponentSpacingMM)
	}
	if doc.Policy.Acceptance != AcceptanceStructural {
		t.Fatalf("acceptance = %q, want %q", doc.Policy.Acceptance, AcceptanceStructural)
	}
	if doc.Layout.Rules.PositivePowerTop == nil || !*doc.Layout.Rules.PositivePowerTop {
		t.Fatal("positive power top should default to true")
	}
	if !doc.Policy.Repair.AllowRefAssignment {
		t.Fatal("ref assignment should default to allowed")
	}
}

func TestDocumentJSONTags(t *testing.T) {
	doc := NewDocument()
	doc.Metadata.Name = "LED1"
	doc.Circuit.Components = []Component{{
		ID:     "r_limit",
		Ref:    "R1",
		Role:   ComponentRoleCurrentLimiter,
		Symbol: "Device:R",
		Value:  "1k",
		Pins: []Pin{
			{Number: "1", Role: PinRoleInput},
			{Number: "2", Role: PinRoleOutput},
		},
	}}
	doc.Circuit.Nets = []Net{{
		Name:    "LED_A",
		Role:    NetRoleSignal,
		Connect: []EndpointRef{"r_limit.2", "led.1"},
	}}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal document: %v", err)
	}

	var decoded Document
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal document: %v", err)
	}
	if decoded.Metadata.Name != "LED1" {
		t.Fatalf("metadata name = %q, want LED1", decoded.Metadata.Name)
	}
	if got := decoded.Circuit.Components[0].Role; got != ComponentRoleCurrentLimiter {
		t.Fatalf("component role = %q, want %q", got, ComponentRoleCurrentLimiter)
	}
	if got := decoded.Circuit.Components[0].Pins[0].Role; got != PinRoleInput {
		t.Fatalf("pin role = %q, want %q", got, PinRoleInput)
	}
	if got := decoded.Circuit.Nets[0].Role; got != NetRoleSignal {
		t.Fatalf("net role = %q, want %q", got, NetRoleSignal)
	}
}

func TestLayoutRuleFalseValuesMarshal(t *testing.T) {
	doc := NewDocument()
	doc.Layout.Rules.PositivePowerTop = boolPtr(false)
	doc.Layout.Rules.GroundBottom = boolPtr(false)
	doc.Layout.Rules.PreferLabelsForLongNets = boolPtr(false)
	doc.Layout.Rules.AvoidWireCrossings = boolPtr(false)
	doc.Layout.Rules.MinGroupSpacingMM = floatPtr(0)
	doc.Layout.Rules.MinComponentSpacingMM = floatPtr(0)

	data, err := json.Marshal(doc.Layout.Rules)
	if err != nil {
		t.Fatalf("marshal layout rules: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal layout rules: %v", err)
	}
	for _, key := range []string{
		"positive_power_top",
		"ground_bottom",
		"prefer_labels_for_long_nets",
		"avoid_wire_crossings",
	} {
		value, ok := raw[key]
		if !ok {
			t.Fatalf("expected %q to be present in marshaled JSON", key)
		}
		if value != false {
			t.Fatalf("%s = %v, want false", key, value)
		}
	}
	for _, key := range []string{
		"min_group_spacing_mm",
		"min_component_spacing_mm",
	} {
		value, ok := raw[key]
		if !ok {
			t.Fatalf("expected %q to be present in marshaled JSON", key)
		}
		if value != float64(0) {
			t.Fatalf("%s = %v, want 0", key, value)
		}
	}
}

func TestEndpointRefSplit(t *testing.T) {
	componentID, pinSelector, ok := EndpointRef("sensor.1.1").Split()
	if !ok {
		t.Fatal("expected endpoint to split")
	}
	if componentID != "sensor" {
		t.Fatalf("componentID = %q, want sensor", componentID)
	}
	if pinSelector != "1.1" {
		t.Fatalf("pinSelector = %q, want 1.1", pinSelector)
	}

	for _, value := range []EndpointRef{"missing_dot", ".1", "sensor."} {
		if _, _, ok := value.Split(); ok {
			t.Fatalf("expected %q to fail endpoint split", value)
		}
	}
}

func TestValidComponentID(t *testing.T) {
	for _, value := range []string{"sensor", "Sensor_1", "U1-power"} {
		if !ValidComponentID(value) {
			t.Fatalf("expected %q to be valid", value)
		}
	}
	for _, value := range []string{"", "sensor.1", "-sensor", "sensor space"} {
		if ValidComponentID(value) {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}
