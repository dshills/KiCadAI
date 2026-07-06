package designworkflow

import (
	"encoding/json"
	"slices"
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
	if len(summary.Groups) != 2 {
		t.Fatalf("summary groups = %#v, want two sorted per-net groups", summary.Groups)
	}
	if summary.Groups[0].NetName != "GND" || summary.Groups[0].Status != RouteTreeContactGraphGroupPartial || !slices.Equal(summary.Groups[0].MissingEndpointIDs, []string{"U1.2"}) {
		t.Fatalf("summary groups = %#v, want sorted per-net partial evidence", summary.Groups)
	}
}

func TestSummarizeRouteTreeContactGraphCountsBranchToPadContact(t *testing.T) {
	targets := InterBlockContactEvidence{Targets: []InterBlockContactTarget{
		routeTreeGraphTarget("SIG", "J1", "1", 0, 0),
		routeTreeGraphTarget("SIG", "U1", "1", 10, 0),
	}}
	operations := []transactions.Operation{
		mustRouteTreeAccessRouteOperation(t, "SIG", []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 10, YMM: 0}}),
	}

	summary := SummarizeRouteTreeContactGraph(targets, operations, nil)
	if summary.ProvenEndpoints != 2 || summary.CompleteGroups != 1 || summary.PartialGroups != 0 {
		t.Fatalf("summary = %#v, want branch-to-pad complete group", summary)
	}
}

func TestSummarizeRouteTreeContactGraphCountsSegmentInteriorContact(t *testing.T) {
	targets := InterBlockContactEvidence{Targets: []InterBlockContactTarget{
		routeTreeGraphTarget("SIG", "J1", "1", 5, 0),
		routeTreeGraphTarget("SIG", "U1", "1", 10, 0),
	}}
	operations := []transactions.Operation{
		mustRouteTreeAccessRouteOperation(t, "SIG", []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 10, YMM: 0}}),
	}

	summary := SummarizeRouteTreeContactGraph(targets, operations, nil)
	if summary.ProvenEndpoints != 2 || summary.CompleteGroups != 1 || summary.Components != 1 {
		t.Fatalf("summary = %#v, want segment-interior contact to complete same-net group", summary)
	}
}

func TestSummarizeRouteTreeContactGraphMergesAllMatchingSegmentsAtInteriorJunction(t *testing.T) {
	targets := InterBlockContactEvidence{Targets: []InterBlockContactTarget{
		routeTreeGraphTarget("SIG", "J1", "1", 0, 0),
		routeTreeGraphTarget("SIG", "U1", "1", 10, 0),
		routeTreeGraphTarget("SIG", "U2", "1", 5, 0),
		routeTreeGraphTarget("SIG", "U3", "1", 5, 5),
	}}
	operations := []transactions.Operation{
		mustRouteTreeAccessRouteOperation(t, "SIG", []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 10, YMM: 0}}),
		mustRouteTreeAccessRouteOperation(t, "SIG", []transactions.Point{{XMM: 5, YMM: -5}, {XMM: 5, YMM: 5}}),
	}

	summary := SummarizeRouteTreeContactGraph(targets, operations, nil)
	if summary.ProvenEndpoints != 4 || summary.CompleteGroups != 1 || summary.Components != 1 {
		t.Fatalf("summary = %#v, want same-net interior junction to merge all matching segments", summary)
	}
}

func TestSummarizeRouteTreeContactGraphMergesCrossingSameNetSegmentsWithoutJunctionTarget(t *testing.T) {
	targets := InterBlockContactEvidence{Targets: []InterBlockContactTarget{
		routeTreeGraphTarget("SIG", "J1", "1", 0, 0),
		routeTreeGraphTarget("SIG", "U1", "1", 10, 0),
		routeTreeGraphTarget("SIG", "U2", "1", 5, -5),
		routeTreeGraphTarget("SIG", "U3", "1", 5, 5),
	}}
	operations := []transactions.Operation{
		mustRouteTreeAccessRouteOperation(t, "SIG", []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 10, YMM: 0}}),
		mustRouteTreeAccessRouteOperation(t, "SIG", []transactions.Point{{XMM: 5, YMM: -5}, {XMM: 5, YMM: 5}}),
	}

	summary := SummarizeRouteTreeContactGraph(targets, operations, nil)
	if summary.ProvenEndpoints != 4 || summary.CompleteGroups != 1 || summary.Components != 1 {
		t.Fatalf("summary = %#v, want crossing same-net segments to complete one graph component", summary)
	}
}

func TestSummarizeRouteTreeContactGraphMergesOverlappingSameNetSegments(t *testing.T) {
	targets := InterBlockContactEvidence{Targets: []InterBlockContactTarget{
		routeTreeGraphTarget("SIG", "J1", "1", 0, 0),
		routeTreeGraphTarget("SIG", "U1", "1", 10, 0),
		routeTreeGraphTarget("SIG", "U2", "1", 4, 0),
		routeTreeGraphTarget("SIG", "U3", "1", 12, 0),
	}}
	operations := []transactions.Operation{
		mustRouteTreeAccessRouteOperation(t, "SIG", []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 10, YMM: 0}}),
		mustRouteTreeAccessRouteOperation(t, "SIG", []transactions.Point{{XMM: 4, YMM: 0}, {XMM: 12, YMM: 0}}),
	}

	summary := SummarizeRouteTreeContactGraph(targets, operations, nil)
	if summary.ProvenEndpoints != 4 || summary.CompleteGroups != 1 || summary.Components != 1 {
		t.Fatalf("summary = %#v, want overlapping same-net segments to complete one graph component", summary)
	}
}

func TestSummarizeRouteTreeContactGraphKeepsNearMissSameNetSegmentsSplit(t *testing.T) {
	targets := InterBlockContactEvidence{Targets: []InterBlockContactTarget{
		routeTreeGraphTarget("SIG", "J1", "1", 0, 0),
		routeTreeGraphTarget("SIG", "U1", "1", 10, 0),
		routeTreeGraphTarget("SIG", "U2", "1", 5, 0.01),
		routeTreeGraphTarget("SIG", "U3", "1", 5, 5),
	}}
	operations := []transactions.Operation{
		mustRouteTreeAccessRouteOperation(t, "SIG", []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 10, YMM: 0}}),
		mustRouteTreeAccessRouteOperation(t, "SIG", []transactions.Point{{XMM: 5, YMM: 0.01}, {XMM: 5, YMM: 5}}),
	}

	summary := SummarizeRouteTreeContactGraph(targets, operations, nil)
	if summary.ProvenEndpoints != 4 || summary.CompleteGroups != 0 || summary.PartialGroups != 1 || summary.Components != 2 {
		t.Fatalf("summary = %#v, want near-miss same-net segments to remain split", summary)
	}
}

func TestSummarizeRouteTreeContactGraphCountsBranchToLocalRouteMerge(t *testing.T) {
	targets := InterBlockContactEvidence{Targets: []InterBlockContactTarget{
		routeTreeGraphTarget("SIG", "J1", "1", 0, 0),
		routeTreeGraphTarget("SIG", "U1", "1", 10, 0),
		routeTreeGraphTarget("SIG", "U2", "1", 20, 0),
	}}
	operations := []transactions.Operation{
		mustRouteTreeAccessRouteOperation(t, "SIG", []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 10, YMM: 0}}),
		mustRouteTreeAccessRouteOperation(t, "SIG", []transactions.Point{{XMM: 10, YMM: 0}, {XMM: 20, YMM: 0}}),
	}
	access := []RouteTreeEndpointAccess{{Role: RouteTreeAccessLocalRouteAnchor, Net: "SIG", Layer: "F.CU", XMM: 10, YMM: 0}}

	summary := SummarizeRouteTreeContactGraph(targets, operations, access)
	if summary.ProvenEndpoints != 3 || summary.CompleteGroups != 1 || summary.LocalRouteMerges != 1 {
		t.Fatalf("summary = %#v, want local-route merge to complete group", summary)
	}
}

func TestRouteTreeContactGraphCompletesThroughLocalRouteAnchorOperations(t *testing.T) {
	targets := InterBlockContactEvidence{Targets: []InterBlockContactTarget{
		routeTreeGraphTarget("SIG", "J1", "1", 0, 0),
		routeTreeGraphTarget("SIG", "U1", "1", 10, 0),
		routeTreeGraphTarget("SIG", "U2", "1", 20, 0),
	}}
	localOperations := []transactions.Operation{
		mustRouteTreeAccessRouteOperation(t, "SIG", []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 10, YMM: 0}}),
	}
	routeTreeOperations := []transactions.Operation{
		mustRouteTreeAccessRouteOperation(t, "SIG", []transactions.Point{{XMM: 10, YMM: 0}, {XMM: 20, YMM: 0}}),
	}

	access, issues := BuildRouteTreeEndpointAccessWithIssues(targets, localOperations)
	if len(issues) != 0 {
		t.Fatalf("access issues = %#v", issues)
	}
	accessSummary := SummarizeRouteTreeEndpointAccess(access)
	if accessSummary.LocalRouteAnchors == 0 {
		t.Fatalf("access summary = %#v, want local route anchor evidence", accessSummary)
	}
	contactGraphOperations := append(slices.Clone(routeTreeOperations), localOperations...)
	summary := SummarizeRouteTreeContactGraph(targets, contactGraphOperations, access)
	if summary.ProvenEndpoints != 3 || summary.CompleteGroups != 1 || summary.Components != 1 || summary.LocalRouteMerges == 0 {
		t.Fatalf("summary = %#v, want contact graph completion through local-route anchor operations", summary)
	}
}

func TestSummarizeRouteTreeContactGraphCountsBranchToSameNetCopperMerge(t *testing.T) {
	targets := InterBlockContactEvidence{Targets: []InterBlockContactTarget{
		routeTreeGraphTarget("VCC", "J1", "1", 0, 0),
		routeTreeGraphTarget("VCC", "U1", "1", 10, 0),
		routeTreeGraphTarget("VCC", "U2", "1", 20, 0),
	}}
	operations := []transactions.Operation{
		mustRouteTreeAccessRouteOperation(t, "VCC", []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 5, YMM: 0}, {XMM: 10, YMM: 0}}),
		mustRouteTreeAccessRouteOperation(t, "VCC", []transactions.Point{{XMM: 20, YMM: 0}, {XMM: 5, YMM: 0}}),
	}
	access := []RouteTreeEndpointAccess{{Role: RouteTreeAccessSameNetCopper, Net: "VCC", Layer: "F.CU", XMM: 5, YMM: 0}}

	summary := SummarizeRouteTreeContactGraph(targets, operations, access)
	if summary.ProvenEndpoints != 3 || summary.CompleteGroups != 1 || summary.SameNetMerges != 1 {
		t.Fatalf("summary = %#v, want same-net copper merge to complete group", summary)
	}
}

func TestSummarizeRouteTreeContactGraphConnectsViaLayerTransitions(t *testing.T) {
	targets := InterBlockContactEvidence{Targets: []InterBlockContactTarget{
		routeTreeGraphTargetOnLayer("SIG", "J1", "1", 0, 0, "F.Cu"),
		routeTreeGraphTargetOnLayer("SIG", "U1", "1", 5, 5, "B.Cu"),
	}}
	operations := []transactions.Operation{
		mustRouteTreeAccessRouteOperationWithVias(t, "SIG", "F.Cu", []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 5, YMM: 0}}, []transactions.RouteViaSpec{
			{At: transactions.Point{XMM: 5, YMM: 0}, DiameterMM: 0.8, DrillMM: 0.4, Layers: []string{"F.Cu", "B.Cu"}},
		}),
		mustRouteTreeAccessRouteOperationWithVias(t, "SIG", "B.Cu", []transactions.Point{{XMM: 5, YMM: -5}, {XMM: 5, YMM: 5}}, nil),
	}

	summary := SummarizeRouteTreeContactGraph(targets, operations, nil)
	if summary.ProvenEndpoints != 2 || summary.CompleteGroups != 1 || summary.Components != 1 {
		t.Fatalf("summary = %#v, want via-connected cross-layer route graph", summary)
	}
}

func TestSummarizeRouteTreeContactGraphRejectsWrongNetAndWrongLayerContacts(t *testing.T) {
	targets := InterBlockContactEvidence{Targets: []InterBlockContactTarget{
		routeTreeGraphTarget("SIG", "J1", "1", 0, 0),
		routeTreeGraphTarget("SIG", "U1", "1", 10, 0),
		routeTreeGraphTargetOnLayer("CLK", "J2", "1", 0, 5, "B.Cu"),
		routeTreeGraphTargetOnLayer("CLK", "U2", "1", 10, 5, "B.Cu"),
	}}
	operations := []transactions.Operation{
		mustRouteTreeAccessRouteOperation(t, "GND", []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 10, YMM: 0}}),
		mustRouteTreeAccessRouteOperation(t, "CLK", []transactions.Point{{XMM: 0, YMM: 5}, {XMM: 10, YMM: 5}}),
	}

	summary := SummarizeRouteTreeContactGraph(targets, operations, nil)
	if summary.ProvenEndpoints != 0 || summary.CompleteGroups != 0 || summary.BlockedGroups != 2 {
		t.Fatalf("summary = %#v, want wrong-net and wrong-layer contacts rejected", summary)
	}
}

func TestRouteTreeContactGraphSummaryJSONStable(t *testing.T) {
	summary := RouteTreeContactGraphSummary{
		Nets: []string{"GND", "SIG"},
		Groups: []RouteTreeContactGraphGroupSummary{
			{
				NetName:            "GND",
				Status:             RouteTreeContactGraphGroupPartial,
				RequiredEndpoints:  2,
				ProvenEndpoints:    1,
				Components:         1,
				MissingEndpointIDs: []string{"U1.2"},
			},
			{
				NetName:           "SIG",
				Status:            RouteTreeContactGraphGroupComplete,
				RequiredEndpoints: 2,
				ProvenEndpoints:   2,
				Components:        1,
			},
		},
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
	want := `{"nets":["GND","SIG"],"groups":[{"net_name":"GND","status":"partial","required_endpoints":2,"proven_endpoints":1,"components":1,"missing_endpoint_ids":["U1.2"]},{"net_name":"SIG","status":"complete","required_endpoints":2,"proven_endpoints":2,"components":1}],"required_endpoints":4,"proven_endpoints":3,"components":2,"complete_groups":1,"partial_groups":1,"blocked_groups":0,"same_net_merges":1,"local_route_merges":2}`
	if string(data) != want {
		t.Fatalf("summary JSON = %q, want %q", data, want)
	}
}

func TestSummarizeRequiredNetClassification(t *testing.T) {
	graph := RouteTreeContactGraphSummary{
		Groups: []RouteTreeContactGraphGroupSummary{
			{
				NetName:           "AUDIO_IN",
				Status:            RouteTreeContactGraphGroupComplete,
				RequiredEndpoints: 2,
				ProvenEndpoints:   2,
			},
			{
				NetName:            "VCC",
				Status:             RouteTreeContactGraphGroupPartial,
				RequiredEndpoints:  5,
				ProvenEndpoints:    4,
				MissingEndpointIDs: []string{"output.3"},
			},
			{
				NetName:           "UNUSED",
				Status:            RouteTreeContactGraphGroupBlocked,
				RequiredEndpoints: 0,
			},
		},
	}

	summary := SummarizeRequiredNetClassification(&graph)
	if summary.RequiredInterBlock != 2 || summary.Complete != 1 || summary.Partial != 1 || summary.Blocked != 0 || summary.MissingEndpoints != 1 {
		t.Fatalf("classification = %#v, want one complete and one partial required inter-block net", summary)
	}
	if len(summary.Nets) != 2 {
		t.Fatalf("classification nets = %#v, want only required nets", summary.Nets)
	}
	if summary.Nets[0].NetName != "AUDIO_IN" || summary.Nets[0].Blocking {
		t.Fatalf("classification nets = %#v, want complete AUDIO_IN first and non-blocking", summary.Nets)
	}
	if summary.Nets[1].NetName != "VCC" || !summary.Nets[1].Blocking || len(summary.Nets[1].MissingEndpointIDs) != 1 || summary.Nets[1].MissingEndpointIDs[0] != "output.3" {
		t.Fatalf("classification nets = %#v, want partial blocking VCC evidence", summary.Nets)
	}
}

func TestRequiredNetClassificationSummaryJSONStable(t *testing.T) {
	summary := RequiredNetClassificationSummary{
		Nets: []RequiredNetClassification{
			{
				NetName:           "SIG",
				Kind:              RequiredNetKindInterBlock,
				Status:            RouteTreeContactGraphGroupComplete,
				RequiredEndpoints: 2,
				ProvenEndpoints:   2,
			},
			{
				NetName:            "VCC",
				Kind:               RequiredNetKindInterBlock,
				Status:             RouteTreeContactGraphGroupPartial,
				RequiredEndpoints:  5,
				ProvenEndpoints:    4,
				MissingEndpointIDs: []string{"U1.5"},
				Blocking:           true,
			},
		},
		RequiredInterBlock: 2,
		Complete:           1,
		Partial:            1,
		MissingEndpoints:   1,
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"nets":[{"net_name":"SIG","kind":"required_inter_block","status":"complete","required_endpoints":2,"proven_endpoints":2,"blocking":false},{"net_name":"VCC","kind":"required_inter_block","status":"partial","required_endpoints":5,"proven_endpoints":4,"missing_endpoint_ids":["U1.5"],"blocking":true}],"required_inter_block":2,"complete":1,"partial":1,"blocked":0,"missing_endpoints":1}`
	if string(data) != want {
		t.Fatalf("summary JSON = %q, want %q", data, want)
	}
}

func routeTreeGraphTarget(net string, ref string, pad string, x float64, y float64) InterBlockContactTarget {
	return routeTreeGraphTargetOnLayer(net, ref, pad, x, y, "F.Cu")
}

func routeTreeGraphTargetOnLayer(net string, ref string, pad string, x float64, y float64, layer string) InterBlockContactTarget {
	return InterBlockContactTarget{
		NetName:    net,
		Kind:       InterBlockContactTargetPad,
		EndpointID: ref + "." + pad,
		Ref:        ref,
		Pad:        pad,
		Point:      transactions.Point{XMM: x, YMM: y},
		Layer:      layer,
		Confidence: InterBlockContactConfidenceHigh,
	}
}

func mustRouteTreeAccessRouteOperationWithVias(t *testing.T, net string, layer string, points []transactions.Point, vias []transactions.RouteViaSpec) transactions.Operation {
	t.Helper()
	payload := transactions.RouteOperation{Op: transactions.OpRoute, NetName: net, Layer: layer, Points: points, Vias: vias}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return transactions.Operation{Op: transactions.OpRoute, Net: net, Raw: raw}
}
