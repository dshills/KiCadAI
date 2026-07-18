package blocks

import (
	"context"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestSpeakerOutputProtectionProvesWindowAndMuteCorners(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock(speakerOutputProtectionID)
	if !ok {
		t.Fatal("missing speaker output protection block")
	}
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: speakerOutputProtectionID, InstanceID: "protect"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v params=%#v", issues, output.Instance.Params)
	}
	for _, key := range []string{"positive_trip_min_v", "positive_trip_max_v", "negative_trip_min_v", "negative_trip_max_v", "engagement_delay_min_s", "engagement_delay_max_s", "release_time_max_s"} {
		value, ok := output.Instance.Params[key].(float64)
		if !ok || !finiteSpeakerValue(value) {
			t.Fatalf("%s = %#v", key, output.Instance.Params[key])
		}
	}
	if output.Instance.Params["positive_trip_max_v"].(float64) > 1.5 || output.Instance.Params["negative_trip_max_v"].(float64) > 1.5 {
		t.Fatalf("trip corners = %#v", output.Instance.Params)
	}
	if output.Instance.Params["engagement_delay_min_s"].(float64) < 1 || output.Instance.Params["engagement_delay_max_s"].(float64) > 4 || output.Instance.Params["release_time_max_s"].(float64) > 0.05 {
		t.Fatalf("timing corners = %#v", output.Instance.Params)
	}
	if validation := transactions.Validate(transactions.Transaction{Operations: output.Operations}); len(validation.Issues) != 0 {
		t.Fatalf("transaction issues = %#v", validation.Issues)
	}
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{OriginXMM: 10, OriginYMM: 20})
	if reports.HasBlockingIssue(realized.Issues) {
		t.Fatalf("PCB realization issues = %#v", realized.Issues)
	}
	for _, route := range []string{"raw_contact", "speaker_contact", "fault_wire_or", "coil_clamp", "supply_loss"} {
		if !realizedRouteExists(realized, route) {
			t.Fatalf("missing realized route %s", route)
		}
	}
}

func TestSpeakerOutputProtectionFailsClosedAtUnsafeCorners(t *testing.T) {
	registry := NewBuiltinRegistry()
	for _, params := range []map[string]any{
		{"dc_trip_threshold": "2V"},
		{"resistor_tolerance_percent": 10.0},
		{"delay_capacitance": "100uF"},
		{"release_resistance": "10kΩ"},
		{"relay_component_id": ""},
		{"maximum_load_current": "5A"},
	} {
		output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: speakerOutputProtectionID, InstanceID: "unsafe", Params: params})
		if !reports.HasBlockingIssue(issues) || len(output.Operations) != 0 {
			t.Fatalf("params=%#v issues=%#v operations=%d", params, issues, len(output.Operations))
		}
	}
}

func TestSpeakerTripToleranceEnumerationWidensWithTolerance(t *testing.T) {
	narrow := speakerTripToleranceRange(12, 47000, 10000, 11700, 10000, 0.001, true)
	wide := speakerTripToleranceRange(12, 47000, 10000, 11700, 10000, 0.05, true)
	if wide[0] >= narrow[0] || wide[1] <= narrow[1] {
		t.Fatalf("narrow=%#v wide=%#v", narrow, wide)
	}
}

func TestNearestResistanceUsesVerifiedNominalSet(t *testing.T) {
	nominals := speakerPrecisionResistorNominals()
	if got := nearestResistance(12100, nominals); got != 11700 {
		t.Fatalf("tie selection = %g, want lower deterministic nominal", got)
	}
	if got := nearestResistance(12410, nominals); got != 12500 {
		t.Fatalf("nearest selection = %g, want 12500", got)
	}
}
