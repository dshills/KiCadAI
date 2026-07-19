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
