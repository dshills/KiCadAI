package designworkflow

import (
	"encoding/json"
	"testing"

	"kicadai/internal/transactions"
)

func TestAddAnchorBindingRoutesCreatesRouteOperation(t *testing.T) {
	anchor := transactions.Point{XMM: 1, YMM: 2}
	endpoint := transactions.Point{XMM: 3, YMM: 4}
	summary := SummarizeAnchorBindings([]AnchorBinding{{
		ID:              "inst1.signal_entry",
		BlockInstanceID: "inst1",
		AnchorID:        "signal_entry",
		AnchorNetName:   "SIG",
		AnchorPoint:     &anchor,
		AnchorLayers:    []string{"F.Cu"},
		EndpointID:      "footprint_pad:J1:1:abcd1234",
		EndpointRef:     "J1",
		EndpointPad:     "1",
		EndpointNetName: "SIG",
		EndpointPoint:   &endpoint,
		EndpointLayers:  []string{"F.Cu"},
		Status:          AnchorBindingStatusBound,
		Policy:          AnchorBindingPolicyRequired,
		Required:        true,
		RouteStatus:     AnchorRouteStatusSkipped,
	}}, nil)

	routed, operations := AddAnchorBindingRoutes(summary, AnchorBindingRouteOptions{WidthMM: 0.25})

	if len(operations) != 1 {
		t.Fatalf("operations = %#v, want one", operations)
	}
	if routed.Bindings[0].RouteStatus != AnchorRouteStatusRouted || routed.Routed != 1 {
		t.Fatalf("routed summary = %#v", routed)
	}
	var payload transactions.RouteOperation
	if err := json.Unmarshal(operations[0].Raw, &payload); err != nil {
		t.Fatalf("unmarshal route = %v", err)
	}
	if payload.NetName != "SIG" || payload.Layer != "F.Cu" || payload.WidthMM != 0.25 || len(payload.Points) != 2 {
		t.Fatalf("route payload = %#v", payload)
	}
	if payload.Points[0].XMM != 3 || payload.Points[1].XMM != 1 {
		t.Fatalf("route points = %#v", payload.Points)
	}
}

func TestAddAnchorBindingRoutesReportsMissingCoordinates(t *testing.T) {
	summary := SummarizeAnchorBindings([]AnchorBinding{{
		ID:              "inst1.signal_entry",
		BlockInstanceID: "inst1",
		AnchorID:        "signal_entry",
		AnchorNetName:   "SIG",
		Status:          AnchorBindingStatusBound,
		Policy:          AnchorBindingPolicyRequired,
		Required:        true,
		RouteStatus:     AnchorRouteStatusSkipped,
	}}, nil)

	routed, operations := AddAnchorBindingRoutes(summary, AnchorBindingRouteOptions{})

	if len(operations) != 0 {
		t.Fatalf("operations = %#v, want none", operations)
	}
	if routed.NotRoutable != 1 || routed.ErrorIssues != 1 || routed.Issues[0].Category != AnchorBindingIssueMissingEndpointPoint {
		t.Fatalf("routed summary = %#v", routed)
	}
}

func TestAddAnchorBindingRoutesSkipsAlreadyRoutedBinding(t *testing.T) {
	anchor := transactions.Point{XMM: 1, YMM: 2}
	endpoint := transactions.Point{XMM: 3, YMM: 4}
	summary := SummarizeAnchorBindings([]AnchorBinding{{
		ID:              "inst1.signal_entry",
		BlockInstanceID: "inst1",
		AnchorID:        "signal_entry",
		AnchorNetName:   "SIG",
		AnchorPoint:     &anchor,
		EndpointNetName: "SIG",
		EndpointPoint:   &endpoint,
		Status:          AnchorBindingStatusBound,
		RouteStatus:     AnchorRouteStatusRouted,
	}}, nil)

	routed, operations := AddAnchorBindingRoutes(summary, AnchorBindingRouteOptions{})

	if len(operations) != 0 {
		t.Fatalf("operations = %#v, want none", operations)
	}
	if routed.Routed != 1 {
		t.Fatalf("routed summary = %#v", routed)
	}
}

func TestAddAnchorBindingRoutesRejectsNetMismatch(t *testing.T) {
	anchor := transactions.Point{XMM: 1, YMM: 2}
	endpoint := transactions.Point{XMM: 3, YMM: 4}
	summary := SummarizeAnchorBindings([]AnchorBinding{{
		ID:              "inst1.signal_entry",
		BlockInstanceID: "inst1",
		AnchorID:        "signal_entry",
		AnchorNetName:   "SIG_A",
		AnchorPoint:     &anchor,
		EndpointID:      "footprint_pad:J1:1:abcd1234",
		EndpointNetName: "SIG_B",
		EndpointPoint:   &endpoint,
		Status:          AnchorBindingStatusBound,
		Policy:          AnchorBindingPolicyRequired,
		Required:        true,
		RouteStatus:     AnchorRouteStatusSkipped,
	}}, nil)

	routed, operations := AddAnchorBindingRoutes(summary, AnchorBindingRouteOptions{})

	if len(operations) != 0 {
		t.Fatalf("operations = %#v, want none", operations)
	}
	if routed.NotRoutable != 1 || routed.ErrorIssues != 1 || routed.Issues[0].Category != AnchorBindingIssueNetMismatch {
		t.Fatalf("routed summary = %#v", routed)
	}
}
