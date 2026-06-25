package designworkflow

import (
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestResolveAnchorBindingsBindsNearestMatchingEndpoint(t *testing.T) {
	fragments := testAnchorFragments("esd_protection", blocks.RealizedPCBEntryAnchor{
		ID: "signal_entry", Port: "SIGNAL", NetName: "SIG", Placement: blocks.RelativePlacement{XMM: 10, YMM: 10, Layer: "F.Cu"},
	})
	endpoints := []PhysicalEndpoint{
		testConnectorEndpoint("J1", "1", "SIG", 11, 10),
	}

	summary := ResolveAnchorBindings(fragments, endpoints, AnchorBindingOptions{})

	if summary.Total != 1 || summary.Bound != 1 || summary.BlockingIssues != 0 {
		t.Fatalf("summary = %#v", summary)
	}
	binding := summary.Bindings[0]
	if binding.EndpointRef != "J1" || binding.EndpointPad != "1" || binding.Status != AnchorBindingStatusBound || !binding.Required {
		t.Fatalf("binding = %#v", binding)
	}
}

func TestResolveAnchorBindingsReportsMissingEndpoint(t *testing.T) {
	fragments := testAnchorFragments("esd_protection", blocks.RealizedPCBEntryAnchor{
		ID: "raw_input", Port: "VIN_RAW", NetName: "VIN", Placement: blocks.RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"},
	})

	summary := ResolveAnchorBindings(fragments, nil, AnchorBindingOptions{})

	if summary.Bound != 0 || summary.Unbound != 1 || summary.BlockingIssues != 0 || summary.ErrorIssues != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if len(summary.Issues) != 1 || summary.Issues[0].Category != AnchorBindingIssueMissingEndpoint {
		t.Fatalf("issues = %#v", summary.Issues)
	}
}

func TestResolveAnchorBindingsReportsNetMismatch(t *testing.T) {
	fragments := testAnchorFragments("reverse_polarity_protection", blocks.RealizedPCBEntryAnchor{
		ID: "raw_input", Port: "VIN_RAW", NetName: "VIN", Placement: blocks.RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"},
	})
	endpoints := []PhysicalEndpoint{testConnectorEndpoint("J1", "1", "VCC", 0, 0)}

	summary := ResolveAnchorBindings(fragments, endpoints, AnchorBindingOptions{})

	if summary.Invalid != 1 || summary.ErrorIssues != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.Issues[0].Category != AnchorBindingIssueNetMismatch {
		t.Fatalf("issues = %#v", summary.Issues)
	}
}

func TestResolveAnchorBindingsIgnoresFarNetMismatch(t *testing.T) {
	fragments := testAnchorFragments("esd_protection", blocks.RealizedPCBEntryAnchor{
		ID: "signal_entry", Port: "SIGNAL", NetName: "SIG", Placement: blocks.RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"},
	})
	endpoints := []PhysicalEndpoint{testConnectorEndpoint("J1", "1", "OTHER", 50, 0)}

	summary := ResolveAnchorBindings(fragments, endpoints, AnchorBindingOptions{})

	if summary.Unbound != 1 || len(summary.Issues) != 1 || summary.Issues[0].Category != AnchorBindingIssueMissingEndpoint {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestResolveAnchorBindingsReportsAmbiguousEndpoints(t *testing.T) {
	fragments := testAnchorFragments("esd_protection", blocks.RealizedPCBEntryAnchor{
		ID: "signal_entry", Port: "SIGNAL", NetName: "SIG", Placement: blocks.RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"},
	})
	endpoints := []PhysicalEndpoint{
		testConnectorEndpoint("J1", "1", "SIG", 1, 0),
		testConnectorEndpoint("J2", "1", "SIG", 1, 0),
	}

	summary := ResolveAnchorBindings(fragments, endpoints, AnchorBindingOptions{})

	if summary.Ambiguous != 1 || summary.ErrorIssues != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.Issues[0].Category != AnchorBindingIssueAmbiguousEndpoint {
		t.Fatalf("issues = %#v", summary.Issues)
	}
}

func TestResolveAnchorBindingsSelectsEquivalentSameConnectorEndpoint(t *testing.T) {
	fragments := testAnchorFragments("esd_protection", blocks.RealizedPCBEntryAnchor{
		ID: "ground_return", Port: "GND", NetName: "GND", Placement: blocks.RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"},
	})
	endpoints := []PhysicalEndpoint{
		testConnectorEndpoint("J1", "1", "GND", 3, 0),
		testConnectorEndpoint("J1", "2", "GND", 1, 0),
	}

	summary := ResolveAnchorBindings(fragments, endpoints, AnchorBindingOptions{})

	if summary.Bound != 1 || summary.InfoIssues != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	binding := summary.Bindings[0]
	if binding.EndpointPad != "2" || len(binding.EquivalentEndpointIDs) != 1 {
		t.Fatalf("binding = %#v", binding)
	}
}

func TestResolveProtectionAnchorDoesNotBindInternalComponentPad(t *testing.T) {
	fragments := testAnchorFragments("esd_protection", blocks.RealizedPCBEntryAnchor{
		ID: "signal_entry", Port: "SIGNAL", NetName: "SIG", Placement: blocks.RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"},
	})
	internal := testPhysicalEndpoint("D1", "1", "SIG", 0.5, 0)
	internal.Roles = []string{"tvs"}

	summary := ResolveAnchorBindings(fragments, []PhysicalEndpoint{internal}, AnchorBindingOptions{})

	if summary.Bound != 0 || summary.Unbound != 1 || summary.Issues[0].Category != AnchorBindingIssueMissingEndpoint {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestResolveReversePolarityAnchorBindsConnectorPad(t *testing.T) {
	fragments := testAnchorFragments("reverse_polarity_protection", blocks.RealizedPCBEntryAnchor{
		ID: "raw_input", Port: "VIN_RAW", NetName: "VIN_RAW", Placement: blocks.RelativePlacement{XMM: 2, YMM: 0, Layer: "F.Cu"},
	})
	endpoints := []PhysicalEndpoint{testConnectorEndpoint("J_PWR", "1", "VIN_RAW", 2.5, 0)}

	summary := ResolveAnchorBindings(fragments, endpoints, AnchorBindingOptions{})

	if summary.Bound != 1 || summary.Bindings[0].EndpointRef != "J_PWR" || !summary.Bindings[0].Required {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestResolveReversePolarityAnchorBindsFootprintWithExternalRole(t *testing.T) {
	fragments := testAnchorFragments("reverse_polarity_protection", blocks.RealizedPCBEntryAnchor{
		ID: "raw_input", Port: "VIN_RAW", NetName: "VIN_RAW", Placement: blocks.RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"},
	})
	endpoint := testPhysicalEndpoint("TP1", "1", "VIN_RAW", 0, 0)
	endpoint.Roles = []string{"power_entry"}

	summary := ResolveAnchorBindings(fragments, []PhysicalEndpoint{endpoint}, AnchorBindingOptions{})

	if summary.Bound != 1 || summary.Bindings[0].EndpointRef != "TP1" {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestResolveProtectionAnchorBindsBoardEdgeEndpoint(t *testing.T) {
	fragments := testAnchorFragments("esd_protection", blocks.RealizedPCBEntryAnchor{
		ID: "signal_entry", Port: "SIGNAL", NetName: "SIG", Placement: blocks.RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"},
	})
	endpoints := []PhysicalEndpoint{{
		ID:         "edge_sig",
		Kind:       PhysicalEndpointBoardEdgePoint,
		NetName:    "SIG",
		Layers:     []string{"F.Cu"},
		Roles:      []string{"edge", "signal"},
		Point:      &transactions.Point{XMM: 0, YMM: 0},
		Source:     physicalEndpointSourceExternalRequest,
		Confidence: PhysicalEndpointConfidenceHigh,
	}}

	summary := ResolveAnchorBindings(fragments, endpoints, AnchorBindingOptions{})

	if summary.Bound != 1 || summary.Bindings[0].EndpointKind != PhysicalEndpointBoardEdgePoint {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestResolveReversePolarityAnchorBindsImportedMechanicalEndpoint(t *testing.T) {
	fragments := testAnchorFragments("reverse_polarity_protection", blocks.RealizedPCBEntryAnchor{
		ID: "raw_input", Port: "VIN_RAW", NetName: "VIN_RAW", Placement: blocks.RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"},
	})
	endpoints := []PhysicalEndpoint{{
		ID:      "mech_vin",
		Kind:    PhysicalEndpointImportedMechanicalPoint,
		NetName: "VIN_RAW",
		Layers:  []string{"F.Cu"},
		Roles:   []string{"power_entry"},
		Point:   &transactions.Point{XMM: 0, YMM: 0},
	}}

	summary := ResolveAnchorBindings(fragments, endpoints, AnchorBindingOptions{})

	if summary.Bound != 1 || summary.Bindings[0].EndpointKind != PhysicalEndpointImportedMechanicalPoint {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestResolveProtectionAnchorRejectsImportedMechanicalWithoutExternalRole(t *testing.T) {
	fragments := testAnchorFragments("esd_protection", blocks.RealizedPCBEntryAnchor{
		ID: "signal_entry", Port: "SIGNAL", NetName: "SIG", Placement: blocks.RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"},
	})
	endpoints := []PhysicalEndpoint{{
		ID:      "mech",
		Kind:    PhysicalEndpointImportedMechanicalPoint,
		NetName: "SIG",
		Layers:  []string{"F.Cu"},
		Roles:   []string{"internal_fixture"},
		Point:   &transactions.Point{XMM: 0, YMM: 0},
	}}

	summary := ResolveAnchorBindings(fragments, endpoints, AnchorBindingOptions{})

	if summary.Bound != 0 || summary.Issues[0].Category != AnchorBindingIssueMissingEndpoint {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestResolveProtectionAnchorRejectsImportedMechanicalWithOnlyGenericSignalRole(t *testing.T) {
	fragments := testAnchorFragments("esd_protection", blocks.RealizedPCBEntryAnchor{
		ID: "signal_entry", Port: "SIGNAL", NetName: "SIG", Placement: blocks.RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"},
	})
	endpoints := []PhysicalEndpoint{{
		ID:      "mech_signal",
		Kind:    PhysicalEndpointImportedMechanicalPoint,
		NetName: "SIG",
		Layers:  []string{"F.Cu"},
		Roles:   []string{"signal"},
		Point:   &transactions.Point{XMM: 0, YMM: 0},
	}}

	summary := ResolveAnchorBindings(fragments, endpoints, AnchorBindingOptions{})

	if summary.Bound != 0 || summary.Issues[0].Category != AnchorBindingIssueMissingEndpoint {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestResolveProtectionAnchorAllowsImportedMechanicalConnectorRef(t *testing.T) {
	fragments := testAnchorFragments("esd_protection", blocks.RealizedPCBEntryAnchor{
		ID: "signal_entry", Port: "SIGNAL", NetName: "SIG", Placement: blocks.RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"},
	})
	endpoints := []PhysicalEndpoint{{
		ID:      "mech_j1",
		Kind:    PhysicalEndpointImportedMechanicalPoint,
		Ref:     "J_MECH",
		NetName: "SIG",
		Layers:  []string{"F.Cu"},
		Point:   &transactions.Point{XMM: 0, YMM: 0},
	}}

	summary := ResolveAnchorBindings(fragments, endpoints, AnchorBindingOptions{})

	if summary.Bound != 1 || summary.Bindings[0].EndpointID != "mech_j1" {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestResolveAnchorBindingsAllowsExplicitNetlessMatch(t *testing.T) {
	fragments := testAnchorFragments("connector_breakout", blocks.RealizedPCBEntryAnchor{
		ID: "mech", Port: "MECH", Placement: blocks.RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"},
	})
	endpoints := []PhysicalEndpoint{testPhysicalEndpoint("J1", "MP", "", 0, 0)}

	summary := ResolveAnchorBindings(fragments, endpoints, AnchorBindingOptions{})

	if summary.Bound != 1 || summary.ErrorIssues != 0 {
		t.Fatalf("summary = %#v", summary)
	}
}

func testAnchorFragments(blockID string, anchors ...blocks.RealizedPCBEntryAnchor) PCBFragmentResult {
	return PCBFragmentResult{Fragments: []BlockFragment{{
		InstanceID:  "inst1",
		BlockID:     blockID,
		Realization: blocks.BlockPCBRealizationResult{EntryAnchors: anchors},
	}}}
}

func testPhysicalEndpoint(ref string, pad string, net string, x float64, y float64) PhysicalEndpoint {
	return PhysicalEndpoint{
		ID:         physicalEndpointID(PhysicalEndpointFootprintPad, ref, pad),
		Kind:       PhysicalEndpointFootprintPad,
		Ref:        ref,
		Pad:        pad,
		NetName:    net,
		Layers:     []string{"F.Cu"},
		Point:      &transactions.Point{XMM: x, YMM: y},
		Source:     physicalEndpointSourcePlacementPad,
		Confidence: PhysicalEndpointConfidenceHigh,
	}
}

func testConnectorEndpoint(ref string, pad string, net string, x float64, y float64) PhysicalEndpoint {
	endpoint := testPhysicalEndpoint(ref, pad, net, x, y)
	endpoint.Roles = []string{"connector"}
	return endpoint
}

func TestRequiredAnchorBindingIssueSeverityUsesRequiredFlag(t *testing.T) {
	if got := RequiredAnchorBindingIssueSeverity(true, AnchorBindingPolicyOptional, AnchorBindingStatusUnbound, AnchorRouteStatusSkipped); got != reports.SeverityError {
		t.Fatalf("severity = %s, want error", got)
	}
}

func TestPhysicalEndpointGridQueriesNearbyCells(t *testing.T) {
	index := newPhysicalEndpointGrid([]PhysicalEndpoint{
		testPhysicalEndpoint("J1", "1", "SIG", 1, 1),
		testPhysicalEndpoint("J2", "1", "SIG", 100, 100),
	}, 10)

	nearby := index.Near(transactions.Point{XMM: 0, YMM: 0})

	if len(nearby) != 1 || nearby[0].Ref != "J1" {
		t.Fatalf("nearby = %#v, want only J1", nearby)
	}
}
