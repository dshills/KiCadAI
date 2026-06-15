package placement

import "testing"

func TestGroupAnchorInfluencesPlacement(t *testing.T) {
	req := minimalRequest()
	req.Board.WidthMM = 100
	req.Board.HeightMM = 40
	req.Components[0].GroupID = "power"
	req.Groups = []Group{{
		ID:         "power",
		Components: []string{"R1"},
		Anchor:     GroupAnchor{At: &Point{XMM: 80, YMM: 20}},
	}}

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	if result.Placements[0].Position.XMM < 60 {
		t.Fatalf("anchored placement X = %.2f, want near anchor", result.Placements[0].Position.XMM)
	}
}

func TestKeepTogetherInfluencesPlacementNearPlacedPeer(t *testing.T) {
	req := twoComponentRequest()
	req.Board.WidthMM = 80
	req.Board.HeightMM = 40
	req.Components[0].Fixed = true
	req.Components[0].Position = &Placement{XMM: 60, YMM: 20, Layer: "F.Cu"}
	req.Groups = []Group{{
		ID:           "analog",
		Components:   []string{"R1", "R2"},
		KeepTogether: true,
	}}

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	var moving PlacementResult
	for _, placement := range result.Placements {
		if placement.Ref == "R2" {
			moving = placement
		}
	}
	if moving.Ref == "" {
		t.Fatalf("missing R2 placement: %#v", result.Placements)
	}
	if moving.Position.XMM < 45 {
		t.Fatalf("R2 X = %.2f, want keep-together placement near fixed R1", moving.Position.XMM)
	}
}

func TestKeepTogetherTargetCombinesMultipleGroups(t *testing.T) {
	req := Request{
		Components: []Component{{Ref: "U1"}},
		Groups: []Group{
			{ID: "analog", Components: []string{"U1", "R1"}, KeepTogether: true},
			{ID: "power", Components: []string{"U1", "R1", "C1"}, KeepTogether: true},
		},
	}
	target, ok := groupKeepTogetherTarget("U1", keepTogetherPeersByComponent(req), map[string]PlacementResult{
		"R1": {Ref: "R1", Bounds: Rect{Min: Point{XMM: 10, YMM: 10}, Max: Point{XMM: 10, YMM: 10}}},
		"C1": {Ref: "C1", Bounds: Rect{Min: Point{XMM: 30, YMM: 20}, Max: Point{XMM: 30, YMM: 20}}},
	})
	if !ok {
		t.Fatal("groupKeepTogetherTarget returned no target")
	}
	if target.XMM != 20 || target.YMM != 15 {
		t.Fatalf("target = %#v, want centroid of both group peers", target)
	}
}

func TestValidateGroupsRejectsSpreadViolation(t *testing.T) {
	req := twoComponentRequest()
	req.Groups = []Group{{
		ID:          "analog",
		Components:  []string{"R1", "R2"},
		MaxSpreadMM: 2,
	}}
	placements := []PlacementResult{
		{Ref: "R1", Bounds: Rect{Min: Point{XMM: 0, YMM: 0}, Max: Point{XMM: 1, YMM: 1}}},
		{Ref: "R2", Bounds: Rect{Min: Point{XMM: 10, YMM: 0}, Max: Point{XMM: 11, YMM: 1}}},
	}

	issues := ValidateGroups(req, placements)
	assertIssueContains(t, issues, "group analog spread")
}

func TestValidateGroupsUsesEuclideanSpread(t *testing.T) {
	req := twoComponentRequest()
	req.Groups = []Group{{
		ID:          "analog",
		Components:  []string{"R1", "R2"},
		MaxSpreadMM: 5,
	}}
	placements := []PlacementResult{
		{Ref: "R1", Bounds: Rect{Min: Point{XMM: 0, YMM: 0}, Max: Point{XMM: 0, YMM: 0}}},
		{Ref: "R2", Bounds: Rect{Min: Point{XMM: 3, YMM: 4}, Max: Point{XMM: 3, YMM: 4}}},
	}

	issues := ValidateGroups(req, placements)
	if len(issues) != 0 {
		t.Fatalf("ValidateGroups returned issues for 3-4-5 spread: %#v", issues)
	}
}
