package amplifiers

import (
	"slices"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/routing"
)

func TestValidatePCBConstraintEvidenceAcceptsAmplifierRoutingIntent(t *testing.T) {
	registry := blocks.NewBuiltinRegistry()
	definition, ok := registry.GetBlock("opamp_gain_stage")
	if !ok {
		t.Fatal("opamp_gain_stage block missing")
	}
	quality := routing.BuildQualityReport(amplifierRoutingEvidenceRequest(), routing.Result{
		Status: routing.StatusRouted,
		Routes: []routing.Route{
			{Net: "AUDIO_IN", Status: routing.RouteStatusRouted, Segments: []routing.Segment{{Net: "AUDIO_IN", Layer: "F.Cu", Start: routing.Point{XMM: 1, YMM: 1}, End: routing.Point{XMM: 4, YMM: 1}, WidthMM: 0.25}}},
			{Net: "FEEDBACK", Status: routing.RouteStatusRouted, Segments: []routing.Segment{{Net: "FEEDBACK", Layer: "F.Cu", Start: routing.Point{XMM: 3, YMM: 3}, End: routing.Point{XMM: 5, YMM: 3}, WidthMM: 0.25}}},
			{Net: "HP_OUT", Status: routing.RouteStatusRouted, Segments: []routing.Segment{{Net: "HP_OUT", Layer: "F.Cu", Start: routing.Point{XMM: 5, YMM: 5}, End: routing.Point{XMM: 10, YMM: 5}, WidthMM: 0.6}}},
			{Net: "VCC", Status: routing.RouteStatusRouted, Segments: []routing.Segment{{Net: "VCC", Layer: "F.Cu", Start: routing.Point{XMM: 2, YMM: 5}, End: routing.Point{XMM: 4, YMM: 5}, WidthMM: 0.3}}},
			{Net: "GND", Status: routing.RouteStatusRouted, Segments: []routing.Segment{{Net: "GND", Layer: "F.Cu", Start: routing.Point{XMM: 2, YMM: 6}, End: routing.Point{XMM: 4, YMM: 6}, WidthMM: 0.3}}},
		},
	})

	evidence := ValidatePCBConstraintEvidence(definition.PCBRealization, quality)
	if !evidence.OK() {
		t.Fatalf("expected complete amplifier PCB evidence: %#v", evidence)
	}
}

func TestValidatePCBConstraintEvidenceReportsMissingAndIncompleteRoutes(t *testing.T) {
	quality := routing.BuildQualityReport(amplifierRoutingEvidenceRequest(), routing.Result{
		Status: routing.StatusPartial,
		Routes: []routing.Route{
			{Net: "AUDIO_IN", Status: routing.RouteStatusRouted, Segments: []routing.Segment{{Net: "AUDIO_IN", Layer: "F.Cu", Start: routing.Point{XMM: 1, YMM: 1}, End: routing.Point{XMM: 4, YMM: 1}, WidthMM: 0.25}}},
			{Net: "FEEDBACK", Status: routing.RouteStatusRouted, Segments: []routing.Segment{{Net: "FEEDBACK", Layer: "F.Cu", Start: routing.Point{XMM: 3, YMM: 3}, End: routing.Point{XMM: 5, YMM: 3}, WidthMM: 0.25}}},
			{Net: "HP_OUT", Status: routing.RouteStatusFailed},
			{Net: "VCC", Status: routing.RouteStatusRouted, Segments: []routing.Segment{{Net: "VCC", Layer: "F.Cu", Start: routing.Point{XMM: 2, YMM: 5}, End: routing.Point{XMM: 4, YMM: 5}, WidthMM: 0.3}}},
			{Net: "GND", Status: routing.RouteStatusRouted, Segments: []routing.Segment{{Net: "GND", Layer: "F.Cu", Start: routing.Point{XMM: 2, YMM: 6}, End: routing.Point{XMM: 4, YMM: 6}, WidthMM: 0.3}}},
		},
	})

	evidence := ValidatePCBConstraintEvidence(&blocks.PCBRealization{}, quality)
	if evidence.OK() {
		t.Fatal("expected missing constraints and incomplete high-current route")
	}
	for _, id := range OpAmpPCBConstraintIDs() {
		if !slices.Contains(evidence.MissingConstraints, id) {
			t.Fatalf("missing constraints = %#v, want %s", evidence.MissingConstraints, id)
		}
	}
	if !slices.Contains(evidence.IncompleteRoutes, "HP_OUT") {
		t.Fatalf("incomplete routes = %#v, want HP_OUT", evidence.IncompleteRoutes)
	}
	if len(evidence.Blockers) == 0 {
		t.Fatalf("expected blocker text: %#v", evidence)
	}
}

func TestValidatePCBConstraintEvidenceRequiresAnalogAndHighCurrentClasses(t *testing.T) {
	request := amplifierRoutingEvidenceRequest()
	request.Nets = []routing.Net{{Name: "SIG", Role: routing.NetSignal, Class: "signal", Endpoints: []routing.Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "U1", Pin: "3"}}}}
	quality := routing.BuildQualityReport(request, routing.Result{
		Status: routing.StatusRouted,
		Routes: []routing.Route{{Net: "SIG", Status: routing.RouteStatusRouted, Segments: []routing.Segment{{Net: "SIG", Layer: "F.Cu", Start: routing.Point{XMM: 1, YMM: 1}, End: routing.Point{XMM: 4, YMM: 1}, WidthMM: 0.25}}}},
	})

	evidence := ValidatePCBConstraintEvidence(&blocks.PCBRealization{Constraints: constraintsForTest(OpAmpPCBConstraintIDs())}, quality)
	for _, id := range []string{"analog_input", "feedback", "high_current_output", "power_supply"} {
		if !slices.Contains(evidence.MissingNetEvidence, id) {
			t.Fatalf("missing net evidence = %#v, want %s", evidence.MissingNetEvidence, id)
		}
	}
}

func TestValidatePCBConstraintEvidenceUsesSpecificAnalogInputTokens(t *testing.T) {
	request := amplifierRoutingEvidenceRequest()
	request.Nets = []routing.Net{
		{Name: "GAIN", Role: routing.NetAnalog, Class: "signal", Endpoints: []routing.Endpoint{{Ref: "R1", Pin: "1"}, {Ref: "U1", Pin: "4"}}},
		{Name: "FEEDBACK_IN", Class: "analog_input_feedback", Endpoints: []routing.Endpoint{{Ref: "R2", Pin: "1"}, {Ref: "U1", Pin: "4"}}},
		{Name: "HP_OUT", Role: routing.NetHighCurrent, Class: "headphone_output", Endpoints: []routing.Endpoint{{Ref: "R3", Pin: "2"}, {Ref: "J2", Pin: "1"}}},
		{Name: "(VCC+5V)", Role: routing.NetPower, Class: "supply", Endpoints: []routing.Endpoint{{Ref: "C1", Pin: "1"}, {Ref: "U1", Pin: "5"}}},
	}
	quality := routing.BuildQualityReport(request, routing.Result{
		Status: routing.StatusRouted,
		Routes: []routing.Route{
			{Net: "GAIN", Status: routing.RouteStatusRouted, Segments: []routing.Segment{{Net: "GAIN", Layer: "F.Cu", Start: routing.Point{XMM: 1, YMM: 1}, End: routing.Point{XMM: 4, YMM: 1}, WidthMM: 0.25}}},
			{Net: "FEEDBACK_IN", Status: routing.RouteStatusRouted, Segments: []routing.Segment{{Net: "FEEDBACK_IN", Layer: "F.Cu", Start: routing.Point{XMM: 2, YMM: 2}, End: routing.Point{XMM: 5, YMM: 2}, WidthMM: 0.25}}},
			{Net: "HP_OUT", Status: routing.RouteStatusRouted, Segments: []routing.Segment{{Net: "HP_OUT", Layer: "F.Cu", Start: routing.Point{XMM: 5, YMM: 5}, End: routing.Point{XMM: 10, YMM: 5}, WidthMM: 0.6}}},
			{Net: "(VCC+5V)", Status: routing.RouteStatusRouted, Segments: []routing.Segment{{Net: "(VCC+5V)", Layer: "F.Cu", Start: routing.Point{XMM: 2, YMM: 5}, End: routing.Point{XMM: 4, YMM: 5}, WidthMM: 0.3}}},
		},
	})

	evidence := ValidatePCBConstraintEvidence(&blocks.PCBRealization{Constraints: constraintsForTest(OpAmpPCBConstraintIDs())}, quality)
	if len(evidence.MissingNetEvidence) != 0 {
		t.Fatalf("missing net evidence = %#v, want none", evidence.MissingNetEvidence)
	}
}

func TestHasEvidencePhraseRejectsEmptyPhrase(t *testing.T) {
	if hasEvidencePhrase("audio_in", "") {
		t.Fatal("empty phrase must not match")
	}
}

func amplifierRoutingEvidenceRequest() routing.Request {
	return routing.Request{
		Nets: []routing.Net{
			{Name: "AUDIO_IN", Role: routing.NetAnalog, Class: "analog_input", Endpoints: []routing.Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "U1", Pin: "3"}}},
			{Name: "FEEDBACK", Role: routing.NetAnalog, Class: "feedback_loop", Endpoints: []routing.Endpoint{{Ref: "U1", Pin: "4"}, {Ref: "R1", Pin: "2"}}},
			{Name: "HP_OUT", Role: routing.NetHighCurrent, Class: "headphone_output", Endpoints: []routing.Endpoint{{Ref: "R2", Pin: "2"}, {Ref: "J2", Pin: "1"}}},
			{Name: "VCC", Role: routing.NetPower, Class: "supply", Endpoints: []routing.Endpoint{{Ref: "C1", Pin: "1"}, {Ref: "U1", Pin: "5"}}},
			{Name: "GND", Role: routing.NetGround, Class: "supply", Endpoints: []routing.Endpoint{{Ref: "C1", Pin: "2"}, {Ref: "U1", Pin: "2"}}},
		},
	}
}

func constraintsForTest(ids []string) []blocks.PCBConstraint {
	constraints := make([]blocks.PCBConstraint, 0, len(ids))
	for _, id := range ids {
		constraints = append(constraints, blocks.PCBConstraint{ID: id, Kind: "test"})
	}
	return constraints
}
