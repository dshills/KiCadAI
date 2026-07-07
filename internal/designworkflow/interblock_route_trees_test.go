package designworkflow

import (
	"context"
	"testing"
	"time"

	"kicadai/internal/transactions"
)

func TestBuildInterBlockRouteTreePrefersConnectorRoot(t *testing.T) {
	group := routeTreeTestGroup("SIG",
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "R1", Pin: "1"},
	)
	tree := BuildInterBlockRouteTree(group, routeTreeTestTargets("SIG", map[string]transactions.Point{
		"U1.1": {XMM: 0, YMM: 0},
		"J1.1": {XMM: 40, YMM: 0},
		"R1.1": {XMM: 5, YMM: 0},
	}))
	if tree.RootEndpointID != "J1.1" {
		t.Fatalf("tree = %#v, want connector root J1.1", tree)
	}
	if len(tree.Branches) != 2 {
		t.Fatalf("branches = %#v, want two branches for three endpoints", tree.Branches)
	}
}

func TestBuildInterBlockRouteTreeUsesCentralEndpointFallback(t *testing.T) {
	group := routeTreeTestGroup("SIG",
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "R1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "C1", Pin: "1"},
	)
	tree := BuildInterBlockRouteTree(group, routeTreeTestTargets("SIG", map[string]transactions.Point{
		"U1.1": {XMM: 0, YMM: 0},
		"R1.1": {XMM: 5, YMM: 0},
		"C1.1": {XMM: 10, YMM: 0},
	}))
	if tree.RootEndpointID != "R1.1" {
		t.Fatalf("tree = %#v, want central root R1.1", tree)
	}
}

func TestBuildInterBlockRouteTreePlansNearestBranchesDeterministically(t *testing.T) {
	group := routeTreeTestGroup("SIG",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "R1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "C1", Pin: "1"},
	)
	tree := BuildInterBlockRouteTree(group, routeTreeTestTargets("SIG", map[string]transactions.Point{
		"J1.1": {XMM: 0, YMM: 0},
		"U1.1": {XMM: 10, YMM: 0},
		"R1.1": {XMM: 3, YMM: 0},
		"C1.1": {XMM: 4, YMM: 0},
	}))
	got := []string{}
	for _, branch := range tree.Branches {
		got = append(got, branch.StartEndpointID+"->"+branch.EndEndpointID)
	}
	want := []string{"J1.1->R1.1", "R1.1->C1.1", "C1.1->U1.1"}
	if len(got) != len(want) {
		t.Fatalf("branches = %#v, want %v", tree.Branches, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("branches = %v, want %v", got, want)
		}
	}
}

func TestBuildInterBlockRouteTreeReportsMissingTargets(t *testing.T) {
	group := routeTreeTestGroup("SIG",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	tree := BuildInterBlockRouteTree(group, routeTreeTestTargets("SIG", map[string]transactions.Point{
		"J1.1": {XMM: 0, YMM: 0},
	}))
	if tree.RootEndpointID != "J1.1" || len(tree.Branches) != 0 {
		t.Fatalf("tree = %#v, want only resolved connector root", tree)
	}
	if len(tree.MissingEndpointIDs) != 1 || tree.MissingEndpointIDs[0] != "U1.1" {
		t.Fatalf("missing endpoint IDs = %#v, want U1.1", tree.MissingEndpointIDs)
	}
}

func TestBuildInterBlockRouteTreesPreservesCaseSensitiveNetNames(t *testing.T) {
	group := routeTreeTestGroup("vcc",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	trees := BuildInterBlockRouteTrees([]InterBlockRouteGroup{group}, InterBlockContactEvidence{
		Targets: []InterBlockContactTarget{
			{
				NetName:    "VCC",
				Kind:       InterBlockContactTargetPad,
				Ref:        "J1",
				Pad:        "1",
				Point:      transactions.Point{XMM: 0, YMM: 0},
				Layer:      "F.Cu",
				Confidence: InterBlockContactConfidenceHigh,
			},
			{
				NetName:    "VCC",
				Kind:       InterBlockContactTargetPad,
				Ref:        "U1",
				Pad:        "1",
				Point:      transactions.Point{XMM: 10, YMM: 0},
				Layer:      "F.Cu",
				Confidence: InterBlockContactConfidenceHigh,
			},
		},
	})
	if len(trees) != 1 {
		t.Fatalf("trees = %#v, want one tree", trees)
	}
	if trees[0].TargetCount != 0 || len(trees[0].MissingEndpointIDs) != 2 {
		t.Fatalf("tree = %#v, want lower-case vcc group not to match upper-case VCC targets", trees[0])
	}
}

func TestBuildInterBlockRouteTreesSelectsDuplicateTargetsDeterministically(t *testing.T) {
	group := routeTreeTestGroup("SIG",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	targets := []InterBlockContactTarget{
		{
			NetName:    "SIG",
			Kind:       InterBlockContactTargetPad,
			Ref:        "J1",
			Pad:        "1",
			Point:      transactions.Point{XMM: 20, YMM: 0},
			Layer:      "F.Cu",
			Confidence: InterBlockContactConfidenceMedium,
		},
		{
			NetName:    "SIG",
			Kind:       InterBlockContactTargetPad,
			Ref:        "J1",
			Pad:        "1",
			Point:      transactions.Point{XMM: 0, YMM: 0},
			Layer:      "F.Cu",
			Confidence: InterBlockContactConfidenceHigh,
		},
		{
			NetName:    "SIG",
			Kind:       InterBlockContactTargetPad,
			Ref:        "U1",
			Pad:        "1",
			Point:      transactions.Point{XMM: 10, YMM: 0},
			Layer:      "F.Cu",
			Confidence: InterBlockContactConfidenceHigh,
		},
	}
	tree := BuildInterBlockRouteTrees([]InterBlockRouteGroup{group}, InterBlockContactEvidence{Targets: targets})[0]
	if len(tree.Branches) != 1 || tree.Branches[0].PlannedDistanceMM != 10 {
		t.Fatalf("tree = %#v, want high-confidence J1.1 target selected deterministically", tree)
	}
}

func TestBuildInterBlockRouteTreesI2CSensorBreakoutPrunesLocallyRoutedPassives(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, fragments, placed := i2cSensorBreakoutRoutingFixture(t, ctx)
	candidates, candidateIssues := BuildInterBlockRouteCandidates(fragments, placed)
	if len(candidateIssues) != 0 {
		t.Fatalf("candidate issues = %#v", candidateIssues)
	}
	groups, groupIssues := BuildInterBlockRouteGroups(candidates)
	if len(groupIssues) != 0 {
		t.Fatalf("group issues = %#v", groupIssues)
	}
	targetEvidence := BuildInterBlockContactTargets(candidates, &placed)
	if len(targetEvidence.Issues) != 0 {
		t.Fatalf("target issues = %#v", targetEvidence.Issues)
	}
	trees := BuildInterBlockRouteTrees(groups, targetEvidence)
	if len(trees) != len(groups) {
		t.Fatalf("trees = %#v, groups=%#v", trees, groups)
	}
	for _, tree := range trees {
		if tree.RootEndpointID == "" {
			t.Fatalf("tree = %#v, want deterministic root", tree)
		}
		if tree.RequiredEndpointCount != 2 || tree.TargetCount != tree.RequiredEndpointCount {
			t.Fatalf("tree = %#v, want connector-to-sensor target coverage with locally routed passives pruned", tree)
		}
		if len(tree.Branches) != tree.RequiredEndpointCount-1 {
			t.Fatalf("tree = %#v, want one branch per non-root endpoint", tree)
		}
	}
}

func routeTreeTestGroup(netName string, endpoints ...InterBlockRouteEndpoint) InterBlockRouteGroup {
	groups, issues := BuildInterBlockRouteGroups([]InterBlockRouteCandidate{{
		NetName:   netName,
		Status:    InterBlockRouteCandidateRoutable,
		Endpoints: endpoints,
	}})
	if len(issues) != 0 || len(groups) != 1 {
		panic("invalid route tree test group")
	}
	return groups[0]
}

func routeTreeTestTargets(netName string, points map[string]transactions.Point) map[string]InterBlockContactTarget {
	targets := map[string]InterBlockContactTarget{}
	for id, point := range points {
		ref, pin, ok := splitRouteTreeEndpointID(id)
		if !ok {
			panic("invalid endpoint id")
		}
		targets[normalizedRouteGroupEndpointKey(ref, pin)] = InterBlockContactTarget{
			NetName:    netName,
			Kind:       InterBlockContactTargetPad,
			Ref:        ref,
			Pad:        pin,
			Point:      point,
			Layer:      "F.Cu",
			Confidence: InterBlockContactConfidenceHigh,
		}
	}
	return targets
}

func splitRouteTreeEndpointID(id string) (string, string, bool) {
	for index, char := range id {
		if char == '.' {
			return id[:index], id[index+1:], true
		}
	}
	return "", "", false
}
