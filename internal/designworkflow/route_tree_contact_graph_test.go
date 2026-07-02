package designworkflow

import (
	"encoding/json"
	"testing"

	"kicadai/internal/transactions"
)

func TestSummarizeRouteTreeContactGraphCountsCompleteAndPartialGroups(t *testing.T) {
	targets := InterBlockContactEvidence{Targets: []InterBlockContactTarget{
		routeTreeGraphTarget("SIG", "J1", "1", 0, 0),
		routeTreeGraphTarget("SIG", "U1", "1", 10, 0),
		routeTreeGraphTarget("GND", "J1", "2", 0, 5),
		routeTreeGraphTarget("GND", "U1", "2", 10, 5),
	}}
	operations := []transactions.Operation{
		mustRouteTreeAccessRouteOperation(t, "SIG", []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 5, YMM: 0}}),
		mustRouteTreeAccessRouteOperation(t, "SIG", []transactions.Point{{XMM: 5, YMM: 0}, {XMM: 10, YMM: 0}}),
		mustRouteTreeAccessRouteOperation(t, "GND", []transactions.Point{{XMM: 0, YMM: 5}, {XMM: 2, YMM: 5}}),
	}

	summary := SummarizeRouteTreeContactGraph(targets, operations, nil)
	if summary.RequiredEndpoints != 4 || summary.ProvenEndpoints != 3 {
		t.Fatalf("summary = %#v, want 4 required and 3 proven", summary)
	}
	if summary.CompleteGroups != 1 || summary.PartialGroups != 1 || summary.BlockedGroups != 0 {
		t.Fatalf("summary = %#v, want one complete and one partial group", summary)
	}
}

func TestRouteTreeContactGraphSummaryJSONStable(t *testing.T) {
	summary := RouteTreeContactGraphSummary{
		Nets:              []string{"GND", "SIG"},
		RequiredEndpoints: 4,
		ProvenEndpoints:   3,
		Components:        2,
		CompleteGroups:    1,
		PartialGroups:     1,
		BlockedGroups:     0,
		SameNetMerges:     1,
		LocalRouteMerges:  2,
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"nets":["GND","SIG"],"required_endpoints":4,"proven_endpoints":3,"components":2,"complete_groups":1,"partial_groups":1,"blocked_groups":0,"same_net_merges":1,"local_route_merges":2}`
	if string(data) != want {
		t.Fatalf("summary JSON = %q, want %q", data, want)
	}
}

func routeTreeGraphTarget(net string, ref string, pad string, x float64, y float64) InterBlockContactTarget {
	return InterBlockContactTarget{
		NetName:    net,
		Kind:       InterBlockContactTargetPad,
		EndpointID: ref + "." + pad,
		Ref:        ref,
		Pad:        pad,
		Point:      transactions.Point{XMM: x, YMM: y},
		Layer:      "F.Cu",
		Confidence: InterBlockContactConfidenceHigh,
	}
}
