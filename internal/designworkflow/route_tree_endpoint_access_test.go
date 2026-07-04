package designworkflow

import (
	"encoding/json"
	"strings"
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

func TestRouteTreeAccessCandidatesPreferLocalRouteAnchor(t *testing.T) {
	access := []RouteTreeEndpointAccess{
		{EndpointID: "U1.1", Role: RouteTreeAccessTargetPad, Ref: "U1", Pad: "1", Net: "SDA", Layer: "F.CU", XMM: 10, YMM: 0, Source: "pad"},
		{EndpointID: "U1.1", Role: RouteTreeAccessLocalRouteAnchor, Net: "SDA", Layer: "F.CU", XMM: 12, YMM: 0, Source: "local_route"},
	}
	candidates := routeTreeAccessCandidatesForEndpoint(access, "U1.1", "SDA", RouteTreeEndpointAccess{Net: "SDA", XMM: 0, YMM: 0})
	if len(candidates) != 2 {
		t.Fatalf("candidates = %#v, want two", candidates)
	}
	if candidates[0].Access.Role != RouteTreeAccessLocalRouteAnchor {
		t.Fatalf("candidates = %#v, want local-route anchor first", candidates)
	}
	if candidates[0].EndpointRank != 0 || !strings.Contains(candidates[0].RankReason, "exact_endpoint") || !strings.Contains(candidates[0].RankReason, "preferred_local_route_anchor") || !strings.Contains(candidates[0].RankReason, "distance_ranked") {
		t.Fatalf("candidate = %#v, want exact endpoint local-route and distance evidence", candidates[0])
	}
}

func TestRouteTreeAccessCandidatesPreferExactEndpointPadOverEndpointlessAnchor(t *testing.T) {
	access := []RouteTreeEndpointAccess{
		{EndpointID: "J1.4", Role: RouteTreeAccessTargetPad, Ref: "J1", Pad: "4", Net: "SCL", Layer: "F.Cu", XMM: 2.5, YMM: 12.31, Source: "pad"},
		{Role: RouteTreeAccessLocalRouteAnchor, Net: "SCL", Layer: "F.Cu", XMM: 4.1, YMM: 13.7, Source: "local_route"},
		{Role: RouteTreeAccessSameNetCopper, Net: "SCL", Layer: "F.Cu", XMM: 4.2, YMM: 13.8, Source: routeTreeSameNetExistingCopperSource},
	}
	candidates := routeTreeAccessCandidatesForEndpoint(access, "J1.4", "SCL", RouteTreeEndpointAccess{Net: "SCL", XMM: 8, YMM: 12})
	if len(candidates) != 3 {
		t.Fatalf("candidates = %#v, want three", candidates)
	}
	if candidates[0].Access.Role != RouteTreeAccessTargetPad || candidates[0].Access.EndpointID != "J1.4" {
		t.Fatalf("candidates = %#v, want exact endpoint pad before endpointless copper", candidates)
	}
	if candidates[0].EndpointRank != 0 || candidates[1].EndpointRank != 1 || candidates[2].EndpointRank != 1 {
		t.Fatalf("candidates = %#v, want exact endpoint before net-scoped fallback candidates", candidates)
	}
	if !strings.Contains(candidates[0].RankReason, "exact_endpoint") || !strings.Contains(candidates[1].RankReason, "net_scoped_fallback") {
		t.Fatalf("candidates = %#v, want endpoint rank reasons", candidates)
	}
}

func TestRouteTreeAccessCandidatesFallBackToPadAndTieBreakDeterministically(t *testing.T) {
	access := []RouteTreeEndpointAccess{
		{EndpointID: "U1.1", Role: RouteTreeAccessTargetPad, Ref: "U2", Pad: "1", Net: "SDA", Layer: "F.CU", XMM: 10, YMM: 0, Source: "pad"},
		{EndpointID: "U1.1", Role: RouteTreeAccessTargetPad, Ref: "U1", Pad: "1", Net: "SDA", Layer: "F.CU", XMM: 10, YMM: 0, Source: "pad"},
	}
	candidates := routeTreeAccessCandidatesForEndpoint(access, "U1.1", "SDA", RouteTreeEndpointAccess{Net: "SDA", XMM: 0, YMM: 0})
	if len(candidates) != 2 {
		t.Fatalf("candidates = %#v, want two", candidates)
	}
	if candidates[0].Access.Ref != "U1" || candidates[1].Access.Ref != "U2" {
		t.Fatalf("candidates = %#v, want deterministic ref ordering", candidates)
	}
}

func TestRouteTreeBranchAccessPairsAreBoundedAndRanked(t *testing.T) {
	sources := []routeTreeBranchAccessCandidate{
		{Access: RouteTreeEndpointAccess{EndpointID: "J1.1", Role: RouteTreeAccessTargetPad, Ref: "J2", Net: "SDA"}, RoleRank: 2},
		{Access: RouteTreeEndpointAccess{EndpointID: "J1.1", Role: RouteTreeAccessLocalRouteAnchor, Net: "SDA"}, RoleRank: 0},
	}
	targets := []routeTreeBranchAccessCandidate{
		{Access: RouteTreeEndpointAccess{EndpointID: "U1.1", Role: RouteTreeAccessTargetPad, Ref: "U1", Net: "SDA"}, RoleRank: 2},
		{Access: RouteTreeEndpointAccess{EndpointID: "U1.1", Role: RouteTreeAccessSameNetCopper, Net: "SDA"}, RoleRank: 1},
	}
	pairs := routeTreeBranchAccessPairs(sources, targets, 2)
	if len(pairs) != 2 {
		t.Fatalf("pairs = %#v, want bounded pair list", pairs)
	}
	if pairs[0].Rank != 0 || pairs[0].Source.Access.Role != RouteTreeAccessLocalRouteAnchor || pairs[0].Target.Access.Role != RouteTreeAccessSameNetCopper {
		t.Fatalf("first pair = %#v, want best source/target roles", pairs[0])
	}
}

func TestRouteTreeBranchAccessPairsSkipSameNetCopperToSameNetCopper(t *testing.T) {
	sources := []routeTreeBranchAccessCandidate{
		{Access: RouteTreeEndpointAccess{Role: RouteTreeAccessSameNetCopper, Net: "VCC", Source: routeTreeSameNetExistingCopperSource}, RoleRank: 1},
		{Access: RouteTreeEndpointAccess{Role: RouteTreeAccessTargetPad, Ref: "J1", Pad: "1", Net: "VCC"}, RoleRank: 2},
	}
	targets := []routeTreeBranchAccessCandidate{
		{Access: RouteTreeEndpointAccess{Role: RouteTreeAccessSameNetCopper, Net: "VCC", Source: routeTreeSameNetExistingCopperSource}, RoleRank: 1},
		{Access: RouteTreeEndpointAccess{Role: RouteTreeAccessTargetPad, Ref: "U1", Pad: "1", Net: "VCC"}, RoleRank: 2},
	}

	pairs := routeTreeBranchAccessPairs(sources, targets, 4)
	if len(pairs) != 3 {
		t.Fatalf("pairs = %#v, want only non-copper-to-copper combinations", pairs)
	}
	for _, pair := range pairs {
		if pair.Source.Access.Role == RouteTreeAccessSameNetCopper && pair.Target.Access.Role == RouteTreeAccessSameNetCopper {
			t.Fatalf("pairs = %#v, want copper-to-copper pair filtered", pairs)
		}
	}
}

func TestRouteTreeAccessDistanceRankPenalizesMissingOpposite(t *testing.T) {
	rank := routeTreeAccessDistanceRank(RouteTreeEndpointAccess{Net: "SDA", XMM: 0, YMM: 0}, RouteTreeEndpointAccess{})
	if rank != routeTreeAccessMissingDistance {
		t.Fatalf("rank = %d, want missing-distance penalty", rank)
	}
}

func TestRouteTreeAccessCandidatesReportMissingOppositeReason(t *testing.T) {
	candidates := routeTreeAccessCandidatesForEndpoint([]RouteTreeEndpointAccess{{
		EndpointID: "U1.1",
		Role:       RouteTreeAccessTargetPad,
		Ref:        "U1",
		Pad:        "1",
		Net:        "SDA",
		Layer:      "F.CU",
		XMM:        10,
		YMM:        0,
		Source:     "pad",
	}}, "U1.1", "SDA", RouteTreeEndpointAccess{})
	if len(candidates) != 1 || !strings.Contains(candidates[0].RankReason, "opposite_missing") {
		t.Fatalf("candidates = %#v, want missing opposite rank reason", candidates)
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
