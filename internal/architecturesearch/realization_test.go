package architecturesearch

import (
	"encoding/json"
	"testing"
)

func TestFragmentRealizationNormalizesSemanticConnections(t *testing.T) {
	payload, err := MarshalFragmentRealization(FragmentRealization{
		Capability: " logic_level_translation ",
		Instances: []RealizationInstance{
			{ID: "translator", CatalogID: "translator.example", Usage: "level_translator"},
			{ID: "pullup", CatalogID: "resistor.example", Usage: "bus_pullup"},
		},
		PortBindings: []RealizationPortBinding{
			{Role: "side_a", Lane: "scl", Instance: "translator", Function: "a2"},
			{Role: "side_a", Lane: "sda", Instance: "translator", Function: "a1"},
		},
		Connections: []RealizationConnection{{
			ID: "side_a_sda", Role: "open_drain_bus",
			Endpoints: []RealizationEndpoint{{Instance: "pullup", Function: "b"}, {Instance: "translator", Function: "a1"}},
		}},
	})
	if err != nil {
		t.Fatalf("MarshalFragmentRealization() error = %v", err)
	}
	realization, err := DecodeFragmentRealization(payload)
	if err != nil {
		t.Fatalf("DecodeFragmentRealization() error = %v", err)
	}
	if realization.Schema != FragmentRealizationSchema || realization.PortBindings[0].Lane != "scl" || len(realization.Connections) != 1 {
		t.Fatalf("realization = %#v", realization)
	}
	if got := realization.Instances[1].RequiredFunctions; len(got) != 2 || got[0] != "A1" || got[1] != "A2" {
		t.Fatalf("translator required functions = %#v", got)
	}
}

func TestFragmentRealizationRejectsDuplicateEndpointNets(t *testing.T) {
	realization := FragmentRealization{
		Schema: FragmentRealizationSchema, Capability: "filter",
		Instances:    []RealizationInstance{{ID: "r1", CatalogID: "resistor.example", Usage: "filter", RequiredFunctions: []string{"A", "B"}}},
		PortBindings: []RealizationPortBinding{{Role: "input", Instance: "r1", Function: "A"}},
		Connections: []RealizationConnection{
			{ID: "one", Role: "signal", Endpoints: []RealizationEndpoint{{Instance: "r1", Function: "B"}, {Instance: "r1", Function: "A"}}},
			{ID: "two", Role: "signal", Endpoints: []RealizationEndpoint{{Instance: "r1", Function: "B"}, {Instance: "r1", Function: "A"}}},
		},
	}
	payload, _ := json.Marshal(realization)
	if _, err := DecodeFragmentRealization(payload); err == nil {
		t.Fatal("DecodeFragmentRealization() accepted an endpoint assigned to two semantic nets")
	}
}

func TestFragmentRealizationNormalizesSeriesTransition(t *testing.T) {
	payload, err := MarshalFragmentRealization(FragmentRealization{
		Capability: " current_sensing ",
		Instances: []RealizationInstance{
			{ID: "shunt", CatalogID: "resistor.example", Usage: "current_sense"},
			{ID: "amplifier", CatalogID: "amplifier.example", Usage: "current_sense_amplifier"},
		},
		PortBindings: []RealizationPortBinding{{Role: "measurement", Instance: "amplifier", Function: "out"}},
		SeriesTransitions: []RealizationSeriesTransition{{
			Role: " power ", Input: RealizationEndpoint{Instance: " shunt ", Function: "a"}, Output: RealizationEndpoint{Instance: "shunt", Function: " b "},
		}},
	})
	if err != nil {
		t.Fatalf("MarshalFragmentRealization() error = %v", err)
	}
	realization, err := DecodeFragmentRealization(payload)
	if err != nil {
		t.Fatalf("DecodeFragmentRealization() error = %v", err)
	}
	if len(realization.SeriesTransitions) != 1 || realization.SeriesTransitions[0].Role != "power" || realization.SeriesTransitions[0].Input.Function != "A" || realization.SeriesTransitions[0].Output.Function != "B" {
		t.Fatalf("series transitions = %#v", realization.SeriesTransitions)
	}
	var shuntFunctions []string
	for _, instance := range realization.Instances {
		if instance.ID == "shunt" {
			shuntFunctions = instance.RequiredFunctions
		}
	}
	if len(shuntFunctions) != 2 || shuntFunctions[0] != "A" || shuntFunctions[1] != "B" {
		t.Fatalf("shunt required functions = %#v", shuntFunctions)
	}
}

func TestFragmentRealizationRejectsDuplicateSeriesRole(t *testing.T) {
	realization := FragmentRealization{
		Schema: FragmentRealizationSchema, Capability: "current_sensing",
		Instances:         []RealizationInstance{{ID: "shunt", CatalogID: "resistor.example", Usage: "current_sense", RequiredFunctions: []string{"A", "B"}}},
		PortBindings:      []RealizationPortBinding{{Role: "power", Instance: "shunt", Function: "A"}},
		SeriesTransitions: []RealizationSeriesTransition{{Role: "power", Input: RealizationEndpoint{Instance: "shunt", Function: "A"}, Output: RealizationEndpoint{Instance: "shunt", Function: "B"}}},
	}
	payload, _ := json.Marshal(realization)
	if _, err := DecodeFragmentRealization(payload); err == nil {
		t.Fatal("DecodeFragmentRealization() accepted duplicate direct and series role bindings")
	}
}
