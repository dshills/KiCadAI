package designworkflow

import (
	"encoding/json"
	"testing"

	"kicadai/internal/transactions"
)

func TestBuildRouteTreeEndpointAccessIncludesPadAndLocalRouteAnchors(t *testing.T) {
	targets := InterBlockContactEvidence{Targets: []InterBlockContactTarget{{
		NetName:        "SDA",
		Kind:           InterBlockContactTargetPad,
		EndpointID:     "U1.1",
		Ref:            "U1",
		Pad:            "1",
		Point:          transactions.Point{XMM: 10, YMM: 5},
		Layer:          "F.Cu",
		GeometrySource: "placed_pad",
	}}}
	localRoute := mustRouteTreeAccessRouteOperation(t, "SDA", []transactions.Point{{XMM: 10, YMM: 5}, {XMM: 12, YMM: 5}})

	access := BuildRouteTreeEndpointAccess(targets, []transactions.Operation{localRoute})
	summary := SummarizeRouteTreeEndpointAccess(access)
	if summary.PadAccess != 1 || summary.LocalRouteAnchors != 2 || summary.AccessPoints != 3 {
		t.Fatalf("summary = %#v, access=%#v", summary, access)
	}
	if !stringSliceContains(summary.Nets, "SDA") || !stringSliceContains(summary.Refs, "U1") {
		t.Fatalf("summary = %#v, want SDA/U1 evidence", summary)
	}
}

func TestBuildRouteTreeEndpointAccessReturnsDecodeIssues(t *testing.T) {
	_, issues := BuildRouteTreeEndpointAccessWithIssues(InterBlockContactEvidence{}, []transactions.Operation{{
		Op:  transactions.OpRoute,
		Net: "SDA",
		Raw: []byte(`{"op":"route","net_name":"SDA","points":`),
	}})
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want malformed route decode issue", issues)
	}
}

func TestRouteTreeEndpointAccessSummaryJSONStable(t *testing.T) {
	summary := RouteTreeEndpointAccessSummary{
		AccessPoints:      3,
		PadAccess:         1,
		LocalRouteAnchors: 2,
		SameNetCopper:     0,
		ExternalAnchors:   0,
		Nets:              []string{"SDA"},
		Refs:              []string{"U1"},
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"access_points":3,"pad_access":1,"local_route_anchors":2,"same_net_copper":0,"external_anchors":0,"nets":["SDA"],"refs":["U1"]}`
	if string(data) != want {
		t.Fatalf("summary JSON = %q, want %q", data, want)
	}
}

func mustRouteTreeAccessRouteOperation(t *testing.T, net string, points []transactions.Point) transactions.Operation {
	t.Helper()
	payload := transactions.RouteOperation{Op: transactions.OpRoute, NetName: net, Layer: "F.Cu", Points: points}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return transactions.Operation{Op: transactions.OpRoute, Net: net, Raw: raw}
}
