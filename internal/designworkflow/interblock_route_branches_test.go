package designworkflow

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/transactions"
)

func TestRouteInterBlockTreeBranchesRoutesThreeEndpointTree(t *testing.T) {
	group := routeTreeTestGroup("SIG",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "R1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	tree := BuildInterBlockRouteTree(group, routeTreeTestTargets("SIG", map[string]transactions.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"R1.1": {XMM: 15, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	}))
	base := routeBranchTestRequest("SIG", map[string]routing.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"R1.1": {XMM: 15, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	})

	result := RouteInterBlockTreeBranches(context.Background(), base, group, tree)
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if len(result.Branches) != 2 {
		t.Fatalf("branches = %#v, want two branch attempts", result.Branches)
	}
	if len(result.Operations) != 2 {
		t.Fatalf("operations = %#v, want one operation per branch", result.Operations)
	}
	if len(result.ExistingCopper) == 0 {
		t.Fatalf("existing copper = %#v, want successful branches to feed same-net copper forward", result.ExistingCopper)
	}
	for _, branch := range result.Branches {
		if branch.Status != routing.StatusRouted || branch.OperationCount == 0 || branch.IssueCount != 0 {
			t.Fatalf("branch = %#v, want routed clean branch evidence", branch)
		}
	}
}

func TestRouteInterBlockTreeBranchesWithAccessReportsSelectedAccessPair(t *testing.T) {
	group := routeTreeTestGroup("SIG",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	tree := BuildInterBlockRouteTree(group, routeTreeTestTargets("SIG", map[string]transactions.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	}))
	base := routeBranchTestRequest("SIG", map[string]routing.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	})
	access := []RouteTreeEndpointAccess{
		{EndpointID: "J1.1", Role: RouteTreeAccessTargetPad, Ref: "J1", Pad: "1", Net: "SIG", Layer: "F.Cu", XMM: 5, YMM: 5},
		{EndpointID: "J1.1", Role: RouteTreeAccessLocalRouteAnchor, Net: "SIG", Layer: "F.Cu", XMM: 8, YMM: 5},
		{EndpointID: "U1.1", Role: RouteTreeAccessTargetPad, Ref: "U1", Pad: "1", Net: "SIG", Layer: "F.Cu", XMM: 25, YMM: 5},
	}

	result := RouteInterBlockTreeBranchesWithAccess(context.Background(), base, group, tree, access)
	if len(result.Branches) != 1 {
		t.Fatalf("branches = %#v, want one", result.Branches)
	}
	branch := result.Branches[0]
	if branch.Status != routing.StatusRouted || branch.AccessPairsTried == 0 {
		t.Fatalf("branch = %#v, want routed access-driven branch evidence", branch)
	}
	if branch.SelectedSourceRole != RouteTreeAccessLocalRouteAnchor || branch.SelectedTargetRole != RouteTreeAccessTargetPad {
		t.Fatalf("branch = %#v, want selected local-anchor to pad access roles", branch)
	}
	if branch.SelectedSourceEndpointID != "J1.1" || branch.SelectedTargetEndpointID != "U1.1" {
		t.Fatalf("branch = %#v, want source endpoint J1.1 and target endpoint U1.1", branch)
	}
	if !strings.Contains(branch.SelectedSourceReason, "preferred_local_route_anchor") || !strings.Contains(branch.SelectedTargetReason, "pad_access_fallback") {
		t.Fatalf("branch = %#v, want selected access rank reasons", branch)
	}
	if branch.SelectedSourceLayer != "F.Cu" || branch.SelectedTargetLayer != "F.Cu" {
		t.Fatalf("branch = %#v, want selected access layers", branch)
	}
	if math.Abs(branch.SelectedSourceXMM-8) > 1e-9 || math.Abs(branch.SelectedSourceYMM-5) > 1e-9 {
		t.Fatalf("branch = %#v, want selected source coordinates from local-route anchor", branch)
	}
	if branch.SelectedTargetRef != "U1" || branch.SelectedTargetPad != "1" || math.Abs(branch.SelectedTargetXMM-25) > 1e-9 || math.Abs(branch.SelectedTargetYMM-5) > 1e-9 {
		t.Fatalf("branch = %#v, want selected target pad evidence", branch)
	}
	if !branch.SnapExemptRoute {
		t.Fatalf("branch = %#v, want access-selected route to be snap-exempt", branch)
	}
	if len(branch.AccessAttempts) == 0 || len(branch.AccessAttempts) > routeTreeBranchAccessPairLimit {
		t.Fatalf("branch = %#v, want bounded access attempt evidence", branch)
	}
	attempt := branch.AccessAttempts[0]
	if attempt.PairRank != 0 || attempt.SourceRole != RouteTreeAccessLocalRouteAnchor || attempt.TargetRole != RouteTreeAccessTargetPad || attempt.Status != routing.StatusRouted {
		t.Fatalf("first access attempt = %#v, want routed local-anchor to pad attempt", attempt)
	}
	if attempt.SourceEndpointID != "J1.1" || attempt.TargetEndpointID != "U1.1" {
		t.Fatalf("first access attempt = %#v, want endpoint evidence for selected source and target", attempt)
	}
	if !strings.Contains(attempt.SourceReason, "preferred_local_route_anchor") || !strings.Contains(attempt.TargetReason, "pad_access_fallback") {
		t.Fatalf("first access attempt = %#v, want rank reason evidence", attempt)
	}
	if attempt.SameNetPads != 2 || attempt.SameNetAnchors != 1 || attempt.SameNetCopper != 0 {
		t.Fatalf("first access attempt = %#v, want same-net pad/local-anchor merge audit", attempt)
	}
	if attempt.ObstacleKind != "" || attempt.ObstacleRef != "" || attempt.ObstacleNet != "" {
		t.Fatalf("first access attempt = %#v, want no other-net obstacle audit", attempt)
	}
}

func TestRouteTreeBranchAccessCandidateAuditReportsVCCLimitTruncation(t *testing.T) {
	group := routeTreeTestGroup("VCC",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	tree := BuildInterBlockRouteTree(group, routeTreeTestTargets("VCC", map[string]transactions.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	}))
	base := routeBranchTestRequest("VCC", map[string]routing.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	})
	branch := tree.Branches[0]
	access := []RouteTreeEndpointAccess{
		{EndpointID: "J1.1", Role: RouteTreeAccessTargetPad, Ref: "J1", Pad: "1", Net: "VCC", Layer: "F.Cu", XMM: 5, YMM: 5},
		{EndpointID: "J1.1", Role: RouteTreeAccessLocalRouteAnchor, Net: "VCC", Layer: "F.Cu", XMM: 6, YMM: 5},
		{EndpointID: "J1.1", Role: RouteTreeAccessSameNetCopper, Net: "VCC", Layer: "F.Cu", XMM: 7, YMM: 5},
		{EndpointID: "J1.1", Role: RouteTreeAccessExternalAnchor, Net: "VCC", Layer: "F.Cu", XMM: 8, YMM: 5},
		{EndpointID: "J1.1", Role: RouteTreeAccessExternalAnchor, Net: "VCC", Layer: "B.Cu", XMM: 8, YMM: 6},
		{EndpointID: "J1.1", Role: RouteTreeAccessExternalAnchor, Net: "VCC", Layer: "F.Cu", XMM: 8, YMM: 7},
		{EndpointID: "U1.1", Role: RouteTreeAccessTargetPad, Ref: "U1", Pad: "1", Net: "VCC", Layer: "F.Cu", XMM: 25, YMM: 5},
		{EndpointID: "U1.1", Role: RouteTreeAccessLocalRouteAnchor, Net: "VCC", Layer: "F.Cu", XMM: 24, YMM: 5},
		{EndpointID: "U1.1", Role: RouteTreeAccessSameNetCopper, Net: "VCC", Layer: "F.Cu", XMM: 23, YMM: 5},
		{EndpointID: "U1.1", Role: RouteTreeAccessExternalAnchor, Net: "VCC", Layer: "F.Cu", XMM: 22, YMM: 5},
		{EndpointID: "U1.1", Role: RouteTreeAccessExternalAnchor, Net: "VCC", Layer: "B.Cu", XMM: 22, YMM: 6},
		{EndpointID: "U1.1", Role: RouteTreeAccessExternalAnchor, Net: "VCC", Layer: "F.Cu", XMM: 22, YMM: 7},
	}

	audit := routeTreeBranchAccessAuditForBranch(access, "VCC", branch, routeTreeAccessCandidateCache{})
	if len(audit.SourceCandidates) != 6 || len(audit.TargetCandidates) != 6 || audit.TotalPairCount != 36 {
		t.Fatalf("audit = %#v, want 6 source, 6 target, and 36 total pairs", audit)
	}
	if len(audit.Pairs) != routeTreeBranchAccessPairLimit || !audit.Truncated {
		t.Fatalf("audit = %#v, want truncated candidate-pair list at limit", audit)
	}
	firstPair := audit.Pairs[0]
	if firstPair.Rank != 0 || firstPair.Source.Access.Role != RouteTreeAccessLocalRouteAnchor || firstPair.Target.Access.Role != RouteTreeAccessLocalRouteAnchor {
		t.Fatalf("first pair = %#v, want local-anchor to local-anchor first", firstPair)
	}

	result := RouteInterBlockTreeBranchesWithAccess(context.Background(), base, group, tree, access)
	if len(result.Branches) != 1 {
		t.Fatalf("branches = %#v, want one branch", result.Branches)
	}
	evidence := result.Branches[0]
	if evidence.AccessSourceCount != 6 || evidence.AccessTargetCount != 6 || evidence.AccessPairCount != 36 || evidence.AccessPairLimit != routeTreeBranchAccessPairLimit || !evidence.AccessPairsTruncated {
		t.Fatalf("branch evidence = %#v, want VCC candidate truncation audit fields", evidence)
	}
	if evidence.SelectedSourceRole != RouteTreeAccessLocalRouteAnchor || evidence.SelectedTargetRole != RouteTreeAccessLocalRouteAnchor {
		t.Fatalf("branch evidence = %#v, want selected local-anchor VCC pair", evidence)
	}
	if evidence.SelectedSourceReason == "" || evidence.SelectedTargetReason == "" {
		t.Fatalf("branch evidence = %#v, want selected access ranking reasons", evidence)
	}
}

func TestRouteTreeBranchAccessRankingPenalizesImmediateOtherNetObstacle(t *testing.T) {
	group := routeTreeTestGroup("VCC",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	tree := BuildInterBlockRouteTree(group, routeTreeTestTargets("VCC", map[string]transactions.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	}))
	base := routeBranchTestRequest("VCC", map[string]routing.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	})
	base.Components = append(base.Components, routing.Component{
		Ref:      "C1",
		Position: routing.Placement{Layer: "F.Cu"},
		Pads: []routing.Pad{{
			Ref:      "C1",
			Name:     "1",
			Net:      "GND",
			Position: routing.Point{XMM: 6, YMM: 5},
			Shape:    routing.PadRect,
			Type:     routing.PadSMD,
			Size:     routing.Size{WidthMM: 1, HeightMM: 1},
			Layers:   []string{"F.Cu"},
		}},
	})
	access := []RouteTreeEndpointAccess{
		{EndpointID: "J1.1", Role: RouteTreeAccessTargetPad, Ref: "J1", Pad: "1", Net: "VCC", Layer: "F.Cu", XMM: 5, YMM: 5},
		{EndpointID: "J1.1", Role: RouteTreeAccessLocalRouteAnchor, Net: "VCC", Layer: "F.Cu", XMM: 6, YMM: 5},
		{EndpointID: "U1.1", Role: RouteTreeAccessTargetPad, Ref: "U1", Pad: "1", Net: "VCC", Layer: "F.Cu", XMM: 25, YMM: 5},
	}

	audit := routeTreeBranchAccessAuditForBranchWithMergeAudit(access, "VCC", tree.Branches[0], routeTreeAccessCandidateCache{}, routeTreeMergeAuditBaseForRequest(base, "VCC", true))
	if len(audit.Pairs) == 0 {
		t.Fatalf("audit = %#v, want ranked access pairs", audit)
	}
	firstPair := audit.Pairs[0]
	if firstPair.Source.Access.Role != RouteTreeAccessTargetPad {
		t.Fatalf("first pair = %#v, want pad source before obstacle-pressured local anchor", firstPair)
	}
}

func TestRouteTreeBranchDiagnosticsSeparateFixedNetInfoFromVCCWarnings(t *testing.T) {
	group := routeTreeTestGroup("VCC",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	tree := BuildInterBlockRouteTree(group, routeTreeTestTargets("VCC", map[string]transactions.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	}))
	base := routeBranchTestRequest("VCC", map[string]routing.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	})
	base.Nets = []routing.Net{
		{Name: "VCC", Role: routing.NetPower, Endpoints: []routing.Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "U1", Pin: "1"}}},
		{Name: "GND", Role: routing.NetGround, Endpoints: []routing.Endpoint{{Ref: "J2", Pin: "1"}, {Ref: "J2", Pin: "2"}}},
	}
	base.Components = append(base.Components, routing.Component{
		Ref:      "J2",
		Position: routing.Placement{Layer: "F.Cu"},
		Pads: []routing.Pad{
			{Ref: "J2", Name: "1", Net: "GND", Position: routing.Point{XMM: 5, YMM: 15}, Shape: routing.PadRect, Type: routing.PadSMD, Size: routing.Size{WidthMM: 1, HeightMM: 1}, Layers: []string{"F.Cu"}},
			{Ref: "J2", Name: "2", Net: "GND", Position: routing.Point{XMM: 25, YMM: 15}, Shape: routing.PadRect, Type: routing.PadSMD, Size: routing.Size{WidthMM: 1, HeightMM: 1}, Layers: []string{"F.Cu"}},
		},
	})
	access := []RouteTreeEndpointAccess{
		{EndpointID: "J1.1", Role: RouteTreeAccessTargetPad, Ref: "J1", Pad: "1", Net: "VCC", Layer: "F.Cu", XMM: 5, YMM: 5},
		{EndpointID: "U1.1", Role: RouteTreeAccessTargetPad, Ref: "U1", Pad: "1", Net: "VCC", Layer: "F.Cu", XMM: 25, YMM: 5},
	}

	result := RouteInterBlockTreeBranchesWithAccess(context.Background(), base, group, tree, access)
	if len(result.Branches) != 1 {
		t.Fatalf("branches = %#v, want one branch", result.Branches)
	}
	branch := result.Branches[0]
	if branch.BlockingIssueCount != 0 || branch.WarningIssueCount == 0 || branch.InfoIssueCount == 0 || branch.FixedNetSkipNotices == 0 {
		t.Fatalf("branch = %#v, want VCC warning and fixed-net info separated from blockers", branch)
	}
	if len(branch.AccessAttempts) == 0 {
		t.Fatalf("branch = %#v, want access attempts", branch)
	}
	attempt := branch.AccessAttempts[0]
	if attempt.SameNetPads != 2 || attempt.ObstacleKind != "other_net_pad" || attempt.ObstacleNet != "GND" || attempt.ObstacleRef == "" || attempt.ObstacleDistMM <= 0 {
		t.Fatalf("first access attempt = %#v, want same-net pad count and nearest GND obstacle audit", attempt)
	}
	if !routeTreeBranchIssuesContainCode(result.Issues, reports.CodeMissingNetClass) || !routeTreeBranchIssuesContainCode(result.Issues, reports.CodeFixedNetSkipped) {
		t.Fatalf("issues = %#v, want missing-net-class warning and fixed-net skip info", result.Issues)
	}
	hints := BuildRouteTreeRepairHints(result.Issues)
	if len(hints) != 0 {
		t.Fatalf("repair hints = %#v, want fixed-net info and VCC net-class warning excluded from route-tree blockers", hints)
	}
}

func routeTreeBranchIssuesContainCode(issues []reports.Issue, code reports.Code) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestRouteInterBlockTreeBranchesReportsMissingGroupEndpoint(t *testing.T) {
	group := routeTreeTestGroup("SIG",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	tree := InterBlockRouteTree{
		NetName:        "SIG",
		RootEndpointID: "J1.1",
		Branches: []InterBlockRouteTreeBranch{{
			Index:           0,
			StartEndpointID: "J1.1",
			EndEndpointID:   "MISSING.1",
		}},
	}
	base := routeBranchTestRequest("SIG", map[string]routing.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	})

	result := RouteInterBlockTreeBranches(context.Background(), base, group, tree)
	if len(result.Issues) != 1 {
		t.Fatalf("issues = %#v, want missing endpoint issue", result.Issues)
	}
	if result.Issues[0].Code != reports.CodeValidationFailed || result.Issues[0].Severity != reports.SeverityBlocked {
		t.Fatalf("issue = %#v, want blocked validation issue", result.Issues[0])
	}
	if len(result.Branches) != 1 || result.Branches[0].Status != routing.StatusBlocked {
		t.Fatalf("branches = %#v, want blocked branch evidence", result.Branches)
	}
}

func TestRouteInterBlockTreeBranchesReportsAllBranchesOnCanceledContext(t *testing.T) {
	group := routeTreeTestGroup("SIG",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "R1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	tree := BuildInterBlockRouteTree(group, routeTreeTestTargets("SIG", map[string]transactions.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"R1.1": {XMM: 15, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	}))
	base := routeBranchTestRequest("SIG", map[string]routing.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"R1.1": {XMM: 15, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := RouteInterBlockTreeBranches(ctx, base, group, tree)

	if len(result.Branches) != len(tree.Branches) {
		t.Fatalf("branches = %d, want %d", len(result.Branches), len(tree.Branches))
	}
	for _, branch := range result.Branches {
		if branch.Status != routing.StatusBlocked {
			t.Fatalf("branch %d status = %s, want blocked", branch.BranchIndex, branch.Status)
		}
	}
	if len(result.Issues) != 1 {
		t.Fatalf("issues = %d, want one cancellation issue", len(result.Issues))
	}
}

func TestRouteInterBlockTreeBranchesScopesRoutingIssuesToBranch(t *testing.T) {
	group := routeTreeTestGroup("SIG",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	tree := BuildInterBlockRouteTree(group, routeTreeTestTargets("SIG", map[string]transactions.Point{
		"J1.1": {XMM: 2, YMM: 2},
		"U1.1": {XMM: 8, YMM: 2},
	}))
	base := routeBranchTestRequest("SIG", map[string]routing.Point{
		"J1.1": {XMM: 2, YMM: 2},
		"U1.1": {XMM: 8, YMM: 2},
	})
	base.Obstacles = append(base.Obstacles, routing.Obstacle{
		Kind:  routing.ObstacleKeepout,
		Layer: "F.Cu",
		Geometry: routing.Shape{Rect: &routing.Rect{
			Min: routing.Point{XMM: 0, YMM: 0},
			Max: routing.Point{XMM: 10, YMM: 4},
		}},
	})

	result := RouteInterBlockTreeBranches(context.Background(), base, group, tree)
	if len(result.Issues) == 0 {
		t.Fatalf("issues = %#v, want branch-scoped pathfinding issue", result.Issues)
	}
	for _, issue := range result.Issues {
		if !strings.Contains(issue.Path, `design.inter_block_route_groups["SIG"].branches[0]`) {
			t.Fatalf("issue path = %q, want branch-scoped route-tree path", issue.Path)
		}
		if !strings.Contains(issue.Suggestion, "route-tree branch") {
			t.Fatalf("issue suggestion = %q, want route-tree repair context", issue.Suggestion)
		}
	}
}

func TestRouteTreeBranchesForRoutingOrdersShortConstrainedBranchesFirst(t *testing.T) {
	branches := routeTreeBranchesForRouting([]InterBlockRouteTreeBranch{
		{Index: 3, StartEndpointID: "J1.1", EndEndpointID: "U3.1", PlannedDistanceMM: 30},
		{Index: 1, StartEndpointID: "J1.1", EndEndpointID: "U1.1", PlannedDistanceMM: 5},
		{Index: 2, StartEndpointID: "J1.1", EndEndpointID: "U2.1", PlannedDistanceMM: 5},
		{Index: 0, StartEndpointID: "J1.1", EndEndpointID: "U0.1"},
	})
	got := []int{}
	for _, branch := range branches {
		got = append(got, branch.Index)
	}
	want := []int{0, 1, 2, 3}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("branch order = %v, want %v", got, want)
		}
	}
}

func TestRouteTreeBranchesForRoutingWithAccessOrdersConstrainedBranchesFirst(t *testing.T) {
	branches := []InterBlockRouteTreeBranch{
		{Index: 0, StartEndpointID: "J1.1", EndEndpointID: "U1.1", PlannedDistanceMM: 5},
		{Index: 1, StartEndpointID: "J1.1", EndEndpointID: "U2.1", PlannedDistanceMM: 20},
	}
	access := []RouteTreeEndpointAccess{
		{EndpointID: "J1.1", Role: RouteTreeAccessTargetPad, Net: "SIG", Layer: "F.Cu", XMM: 0, YMM: 0},
		{EndpointID: "J1.1", Role: RouteTreeAccessLocalRouteAnchor, Net: "SIG", Layer: "F.Cu", XMM: 1, YMM: 0},
		{EndpointID: "U1.1", Role: RouteTreeAccessTargetPad, Net: "SIG", Layer: "F.Cu", XMM: 5, YMM: 0},
		{EndpointID: "U1.1", Role: RouteTreeAccessLocalRouteAnchor, Net: "SIG", Layer: "F.Cu", XMM: 6, YMM: 0},
		{EndpointID: "U2.1", Role: RouteTreeAccessTargetPad, Net: "SIG", Layer: "F.Cu", XMM: 20, YMM: 0},
	}

	ordered := routeTreeBranchesForRoutingWithAccess(branches, access, "SIG", routeTreeAccessCandidateCache{})
	got := []int{}
	for _, branch := range ordered {
		got = append(got, branch.Index)
	}
	want := []int{1, 0}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("branch order = %v, want constrained branch first %v", got, want)
		}
	}
}

func TestRouteInterBlockTreeBranchesDoesNotEmitCopperForFailedBranch(t *testing.T) {
	group := routeTreeTestGroup("SIG",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	tree := BuildInterBlockRouteTree(group, routeTreeTestTargets("SIG", map[string]transactions.Point{
		"J1.1": {XMM: 2, YMM: 2},
		"U1.1": {XMM: 8, YMM: 2},
	}))
	base := routeBranchTestRequest("SIG", map[string]routing.Point{
		"J1.1": {XMM: 2, YMM: 2},
		"U1.1": {XMM: 8, YMM: 2},
	})
	base.Obstacles = append(base.Obstacles, routing.Obstacle{
		Kind:  routing.ObstacleKeepout,
		Layer: "F.Cu",
		Geometry: routing.Shape{Rect: &routing.Rect{
			Min: routing.Point{XMM: 0, YMM: 0},
			Max: routing.Point{XMM: 10, YMM: 4},
		}},
	})

	result := RouteInterBlockTreeBranches(context.Background(), base, group, tree)
	if len(result.Issues) == 0 {
		t.Fatalf("issues = %#v, want failed branch issue", result.Issues)
	}
	if len(result.Operations) != 0 || len(result.ExistingCopper) != 0 {
		t.Fatalf("operations=%#v existing=%#v, want no failed partial copper", result.Operations, result.ExistingCopper)
	}
}

func TestRouteInterBlockTreeBranchesMergesLaterBranchIntoGeneratedSameNetCopper(t *testing.T) {
	group := routeTreeTestGroup("VCC",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U2", Pin: "1"},
	)
	tree := InterBlockRouteTree{
		NetName: "VCC",
		Branches: []InterBlockRouteTreeBranch{
			{Index: 0, StartEndpointID: "J1.1", EndEndpointID: "U1.1", PlannedDistanceMM: 10},
			{Index: 1, StartEndpointID: "J1.1", EndEndpointID: "U2.1", PlannedDistanceMM: 12},
		},
	}
	base := routeBranchTestRequest("VCC", map[string]routing.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"U1.1": {XMM: 15, YMM: 5},
		"U2.1": {XMM: 10, YMM: 12},
	})
	base.Nets = []routing.Net{{
		Name: "VCC",
		Role: routing.NetPower,
		Endpoints: []routing.Endpoint{
			{Ref: "J1", Pin: "1"},
			{Ref: "U1", Pin: "1"},
			{Ref: "U2", Pin: "1"},
		},
	}}
	access := []RouteTreeEndpointAccess{
		{EndpointID: "J1.1", Role: RouteTreeAccessTargetPad, Ref: "J1", Pad: "1", Net: "VCC", Layer: "F.Cu", XMM: 5, YMM: 5},
		{EndpointID: "U1.1", Role: RouteTreeAccessTargetPad, Ref: "U1", Pad: "1", Net: "VCC", Layer: "F.Cu", XMM: 15, YMM: 5},
		{EndpointID: "U2.1", Role: RouteTreeAccessTargetPad, Ref: "U2", Pad: "1", Net: "VCC", Layer: "F.Cu", XMM: 10, YMM: 12},
	}

	result := RouteInterBlockTreeBranchesWithAccess(context.Background(), base, group, tree, access)
	if len(result.Branches) != 2 {
		t.Fatalf("branches = %#v, want two", result.Branches)
	}
	first := result.Branches[0]
	second := result.Branches[1]
	if first.Status != routing.StatusRouted || second.Status != routing.StatusRouted {
		t.Fatalf("branches = %#v, want both routed", result.Branches)
	}
	if second.SelectedSourceRole != RouteTreeAccessTargetPad || second.SelectedTargetRole != RouteTreeAccessTargetPad {
		t.Fatalf("second branch = %#v, want exact required endpoints to outrank generated same-net copper", second)
	}
	if len(second.AccessAttempts) == 0 || second.AccessAttempts[0].SameNetCopper == 0 {
		t.Fatalf("second branch attempts = %#v, want same-net copper merge audit", second.AccessAttempts)
	}
}

func TestRouteTreeEndpointAccessWithSameNetCopperIgnoresOtherNetCopper(t *testing.T) {
	access := routeTreeEndpointAccessWithSameNetCopper(nil, []routing.ExistingCopper{
		{
			Kind:  routing.CopperSegment,
			Net:   "VCC",
			Layer: "F.Cu",
			Geometry: routing.Shape{Rect: &routing.Rect{
				Min: routing.Point{XMM: 4, YMM: 5},
				Max: routing.Point{XMM: 8, YMM: 5.5},
			}},
		},
		{
			Kind:  routing.CopperSegment,
			Net:   "GND",
			Layer: "F.Cu",
			Geometry: routing.Shape{Rect: &routing.Rect{
				Min: routing.Point{XMM: 10, YMM: 10},
				Max: routing.Point{XMM: 12, YMM: 10.5},
			}},
		},
	}, "VCC")
	if len(access) != 3 {
		t.Fatalf("access = %#v, want three VCC copper access points", access)
	}
	got := access[1]
	if got.Role != RouteTreeAccessSameNetCopper || got.Net != "VCC" || got.Layer != "F.CU" || got.Source != routeTreeSameNetExistingCopperSource {
		t.Fatalf("access = %#v, want same-net copper access metadata", got)
	}
	if math.Abs(got.XMM-6) > 1e-9 || math.Abs(got.YMM-5.25) > 1e-9 {
		t.Fatalf("access = %#v, want center of VCC copper geometry", got)
	}
}

func TestRouteTreeEndpointAccessWithSameNetCopperPrefersCenterline(t *testing.T) {
	access := routeTreeEndpointAccessWithSameNetCopper(nil, []routing.ExistingCopper{{
		Kind:  routing.CopperSegment,
		Net:   "GND",
		Layer: "F.Cu",
		Geometry: routing.Shape{Rect: &routing.Rect{
			Min: routing.Point{XMM: 5, YMM: 7},
			Max: routing.Point{XMM: 8, YMM: 9},
		}},
		Centerline: []routing.Point{
			{XMM: 5.25, YMM: 8.5},
			{XMM: 6, YMM: 8.5},
		},
	}}, "GND")
	if len(access) != 2 {
		t.Fatalf("access = %#v, want centerline endpoints only", access)
	}
	for _, item := range access {
		if item.YMM != 8.5 || (item.XMM != 5.25 && item.XMM != 6) {
			t.Fatalf("access sampled off-centerline point: %#v", access)
		}
	}
}

func TestRouteTreeEndpointAccessWithSameNetCopperFallsBackForIncompleteCenterline(t *testing.T) {
	access := routeTreeEndpointAccessWithSameNetCopper(nil, []routing.ExistingCopper{{
		Kind:  routing.CopperSegment,
		Net:   "GND",
		Layer: "F.Cu",
		Geometry: routing.Shape{Rect: &routing.Rect{
			Min: routing.Point{XMM: 5, YMM: 7},
			Max: routing.Point{XMM: 8, YMM: 9},
		}},
		Centerline: []routing.Point{{XMM: 5.25, YMM: 8.5}},
	}}, "GND")
	if len(access) == 0 {
		t.Fatalf("access = %#v, want geometry fallback points", access)
	}
	if !routeTreeAccessContainsPoint(access, 6.5, 8) {
		t.Fatalf("access = %#v, want center of fallback geometry", access)
	}
}

func TestRouteTreePrefersSameNetCopperAccessOnlyForPowerNets(t *testing.T) {
	nets := []routing.Net{
		{Name: "VCC", Role: routing.NetPower},
		{Name: "GND", Role: routing.NetGround},
		{Name: "SIG", Role: routing.NetSignal},
	}

	if !routeTreePrefersSameNetCopperAccess(nets, "VCC") {
		t.Fatalf("VCC should allow same-net copper access")
	}
	if !routeTreePrefersSameNetCopperAccess(nets, "GND") {
		t.Fatalf("GND should prefer same-net copper access")
	}
	if routeTreePrefersSameNetCopperAccess(nets, "SIG") {
		t.Fatalf("SIG should not use same-net copper in VCC closeout path")
	}
}

func TestRouteTreeAccessCandidatesRequireCopperContactWithExactEndpoint(t *testing.T) {
	candidates := []routeTreeBranchAccessCandidate{
		{Access: RouteTreeEndpointAccess{EndpointID: "U1.1", Role: RouteTreeAccessTargetPad, Net: "VCC", Layer: "F.Cu", XMM: 10, YMM: 10}, EndpointRank: routeTreeAccessExactEndpointRank},
		{Access: RouteTreeEndpointAccess{Role: RouteTreeAccessSameNetCopper, Net: "VCC", Layer: "F.Cu", XMM: 2, YMM: 2, Source: routeTreeSameNetExistingCopperSource}, EndpointRank: routeTreeAccessFallbackEndpointRank},
	}
	filtered := routeTreeAccessCandidatesWithProvenCopperContact(candidates)
	if len(filtered) != 1 || filtered[0].Access.Role != RouteTreeAccessTargetPad {
		t.Fatalf("filtered = %#v, want unconnected copper fallback removed", filtered)
	}

	candidates[1].Access.XMM = 10
	candidates[1].Access.YMM = 10
	filtered = routeTreeAccessCandidatesWithProvenCopperContact(candidates)
	if len(filtered) != 2 {
		t.Fatalf("filtered = %#v, want contacting copper retained", filtered)
	}
}

func routeTreeAccessContainsPoint(access []RouteTreeEndpointAccess, xMM float64, yMM float64) bool {
	for _, item := range access {
		if math.Abs(item.XMM-xMM) <= 1e-9 && math.Abs(item.YMM-yMM) <= 1e-9 {
			return true
		}
	}
	return false
}

func TestRouteTreeAccessBranchRequestRoutesSyntheticAccessPoints(t *testing.T) {
	base := routeBranchTestRequest("SIG", map[string]routing.Point{
		"J1.1": {XMM: 2, YMM: 2},
		"U1.1": {XMM: 18, YMM: 2},
	})
	base.Rules.GridMM = 0.5
	base.Rules.NetClasses = map[string]routing.NetClass{"audio": {TraceWidthMM: 0.25, ClearanceMM: 0.2}}
	base.Nets = []routing.Net{
		{Name: "SIG", Class: "audio", Role: routing.NetAnalog, Priority: 7, Fixed: true},
		{Name: "GND", Role: routing.NetGround},
	}
	pair := routeTreeBranchAccessPair{
		Rank: 3,
		Source: routeTreeBranchAccessCandidate{Access: RouteTreeEndpointAccess{
			Role:  RouteTreeAccessLocalRouteAnchor,
			Net:   "SIG",
			Layer: "F.Cu",
			XMM:   5,
			YMM:   5,
		}},
		Target: routeTreeBranchAccessCandidate{Access: RouteTreeEndpointAccess{
			Role:  RouteTreeAccessTargetPad,
			Ref:   "U1",
			Pad:   "1",
			Net:   "SIG",
			Layer: "F.Cu",
			XMM:   15,
			YMM:   5,
		}},
	}

	request := routeTreeAccessBranchRequest(base, "SIG", pair)
	if len(base.Components) != 2 {
		t.Fatalf("base components = %d, want unmodified base request", len(base.Components))
	}
	if len(request.Components) != 4 {
		t.Fatalf("components = %d, want base plus synthetic access components", len(request.Components))
	}
	if request.Nets[0].Endpoints[0].Ref != "__KICADAI_RT_SRC_3" || request.Nets[0].Endpoints[1].Ref != "__KICADAI_RT_DST_3" {
		t.Fatalf("endpoints = %#v, want synthetic access refs", request.Nets[0].Endpoints)
	}
	if request.Nets[0].Class != "audio" || request.Nets[0].Role != routing.NetAnalog || request.Nets[0].Priority != 7 {
		t.Fatalf("branch net = %#v, want preserved metadata from base net", request.Nets[0])
	}
	if request.Nets[0].Fixed {
		t.Fatalf("branch net = %#v, want selected branch net to be routable", request.Nets[0])
	}
	if len(request.Nets) != 2 || request.Nets[1].Name != "GND" || !request.Nets[1].Fixed {
		t.Fatalf("nets = %#v, want other net metadata preserved as fixed", request.Nets)
	}

	result := routing.RouteRequestContext(context.Background(), request)
	if result.Status != routing.StatusRouted || len(result.Operations) != 1 {
		t.Fatalf("result = %#v, want routed synthetic access branch", result)
	}
	operations := transactionRouteOperations(result.Operations)
	if len(operations) != 1 {
		t.Fatalf("operations = %#v, want one transaction route", operations)
	}
	var route transactions.RouteOperation
	if err := json.Unmarshal(operations[0].Raw, &route); err != nil {
		t.Fatal(err)
	}
	if len(route.Points) < 2 {
		t.Fatalf("points = %#v, want routed access path", route.Points)
	}
	first := route.Points[0]
	last := route.Points[len(route.Points)-1]
	forward := math.Abs(first.XMM-5) <= 1e-9 && math.Abs(first.YMM-5) <= 1e-9 && math.Abs(last.XMM-15) <= 1e-9 && math.Abs(last.YMM-5) <= 1e-9
	reverse := math.Abs(first.XMM-15) <= 1e-9 && math.Abs(first.YMM-5) <= 1e-9 && math.Abs(last.XMM-5) <= 1e-9 && math.Abs(last.YMM-5) <= 1e-9
	if !forward && !reverse {
		t.Fatalf("route points = %#v, want route snapped to selected access coordinates", route.Points)
	}
}

func TestRouteTreeAccessBranchRequestUsesMatchedNetNameForSyntheticPads(t *testing.T) {
	base := routeBranchTestRequest("sig", map[string]routing.Point{
		"J1.1": {XMM: 2, YMM: 2},
		"U1.1": {XMM: 18, YMM: 2},
	})
	base.Nets = []routing.Net{{Name: "sig", Class: "signal"}, {Name: "SIG", Class: "duplicate"}}
	pair := routeTreeBranchAccessPair{
		Source: routeTreeBranchAccessCandidate{Access: RouteTreeEndpointAccess{Net: "SIG", Layer: "F.Cu", XMM: 5, YMM: 5}},
		Target: routeTreeBranchAccessCandidate{Access: RouteTreeEndpointAccess{Net: "SIG", Layer: "F.Cu", XMM: 15, YMM: 5}},
	}

	request := routeTreeAccessBranchRequest(base, "SIG", pair)
	if request.Nets[0].Name != "sig" {
		t.Fatalf("branch net name = %q, want matched canonical name", request.Nets[0].Name)
	}
	if len(request.Nets) != 1 {
		t.Fatalf("nets = %#v, want duplicate target nets collapsed", request.Nets)
	}
	last := request.Components[len(request.Components)-1]
	if got := last.Pads[0].Net; got != "sig" {
		t.Fatalf("synthetic pad net = %q, want matched canonical net name", got)
	}
}

func TestInterBlockRouteGroupEndpointsByIDKeepsRequiredOnDuplicate(t *testing.T) {
	group := InterBlockRouteGroup{
		RequiredEndpoints: []InterBlockRouteGroupEndpoint{{ID: "U1.1", Ref: "U1", Pin: "1"}},
		OptionalEndpoints: []InterBlockRouteGroupEndpoint{{ID: "U1.1", Ref: "ALT", Pin: "9"}},
	}
	endpoints := interBlockRouteGroupEndpointsByID(group)
	if endpoints["U1.1"].Ref != "U1" || endpoints["U1.1"].Pin != "1" {
		t.Fatalf("duplicate endpoint = %#v, want required endpoint", endpoints["U1.1"])
	}
}

func TestRouteBranchSegmentShapeUsesWidthAwarePolygonForDiagonal(t *testing.T) {
	shape := routeBranchSegmentShape(routing.Segment{
		Start:   routing.Point{XMM: 1, YMM: 1},
		End:     routing.Point{XMM: 3, YMM: 3},
		WidthMM: 0.4,
	}, routing.DefaultRules())
	if shape.Rect != nil {
		t.Fatalf("diagonal shape used rect = %#v, want polygon", shape.Rect)
	}
	if len(shape.Polygon) != 4 {
		t.Fatalf("polygon points = %d, want 4", len(shape.Polygon))
	}
}

func TestRouteBranchSegmentShapeUsesSquareCapsForHorizontal(t *testing.T) {
	shape := routeBranchSegmentShape(routing.Segment{
		Start:   routing.Point{XMM: 1, YMM: 2},
		End:     routing.Point{XMM: 3, YMM: 2},
		WidthMM: 0.4,
	}, routing.DefaultRules())
	if shape.Rect == nil {
		t.Fatalf("horizontal shape used polygon, want rect")
	}
	if math.Abs(shape.Rect.Min.XMM-0.8) > 1e-9 || math.Abs(shape.Rect.Max.XMM-3.2) > 1e-9 {
		t.Fatalf("horizontal caps extended to [%f,%f], want [0.8,3.2]", shape.Rect.Min.XMM, shape.Rect.Max.XMM)
	}
	if math.Abs(shape.Rect.Min.YMM-1.8) > 1e-9 || math.Abs(shape.Rect.Max.YMM-2.2) > 1e-9 {
		t.Fatalf("horizontal width bounds = [%f,%f], want [1.8,2.2]", shape.Rect.Min.YMM, shape.Rect.Max.YMM)
	}
}

func TestRouteBranchViaShapeUsesPolygon(t *testing.T) {
	shape := routeBranchViaShape(routing.Via{
		At:         routing.Point{XMM: 10, YMM: 10},
		DiameterMM: 0.8,
	}, routing.DefaultRules())
	if shape.Rect != nil {
		t.Fatalf("via shape used rect = %#v, want polygon", shape.Rect)
	}
	if len(shape.Polygon) != 8 {
		t.Fatalf("via polygon points = %d, want 8", len(shape.Polygon))
	}
}

func routeBranchTestRequest(netName string, pads map[string]routing.Point) routing.Request {
	request := routing.Request{
		Board: routing.Board{
			WidthMM:  40,
			HeightMM: 20,
			Layers:   []routing.Layer{{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true}},
		},
		Rules:    routing.DefaultRules(),
		Strategy: routing.Strategy{Mode: routing.ModeSingleLayer},
	}
	for id, point := range pads {
		ref, pin, ok := splitRouteTreeEndpointID(id)
		if !ok {
			panic("invalid endpoint ID")
		}
		request.Components = append(request.Components, routing.Component{
			Ref:      ref,
			Position: routing.Placement{Layer: "F.Cu"},
			Pads: []routing.Pad{{
				Ref:      ref,
				Name:     pin,
				Net:      netName,
				Position: point,
				Shape:    routing.PadRect,
				Type:     routing.PadSMD,
				Size:     routing.Size{WidthMM: 1, HeightMM: 1},
				Layers:   []string{"F.Cu"},
			}},
		})
	}
	return request
}
