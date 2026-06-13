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
