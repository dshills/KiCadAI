package placement

import "testing"

func TestPreserveRelativeGroupPlacementTranslatesClusterAroundObstacle(t *testing.T) {
	rules := DefaultRules()
	rules.GridMM = 1
	rules.ComponentSpacingMM = 0
	request := Request{
		Board: BoardPlacementArea{WidthMM: 20, HeightMM: 20},
		Rules: rules,
		Components: []Component{
			{Ref: "U1", FootprintID: "Test:U", Bounds: Bounds{WidthMM: 2, HeightMM: 2, Source: BoundsExplicit}, Position: &Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)}},
			{Ref: "C1", FootprintID: "Test:C", Bounds: Bounds{WidthMM: 2, HeightMM: 2, Source: BoundsExplicit}, Position: &Placement{XMM: 9, YMM: 5, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)}},
			{Ref: "X1", FootprintID: "Test:X", Bounds: Bounds{WidthMM: 4, HeightMM: 4, Source: BoundsExplicit}, Fixed: true, Position: &Placement{XMM: 9, YMM: 5, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)}},
		},
		Groups: []Group{{ID: "core", Components: []string{"U1", "C1"}, Anchor: GroupAnchor{Ref: "U1"}, KeepTogether: true, TranslateAsUnit: true}},
	}

	result := Place(request)
	if result.Status != StatusPlaced {
		t.Fatalf("placement = %#v", result)
	}
	byRef := map[string]PlacementResult{}
	for _, placed := range result.Placements {
		byRef[placed.Ref] = placed
	}
	if delta := byRef["C1"].Position.XMM - byRef["U1"].Position.XMM; delta != 4 {
		t.Fatalf("relative X offset = %v, want 4; placements = %#v", delta, result.Placements)
	}
	if delta := byRef["C1"].Position.YMM - byRef["U1"].Position.YMM; delta != 0 {
		t.Fatalf("relative Y offset = %v, want 0; placements = %#v", delta, result.Placements)
	}
	if issues := ValidateGeometry(request, successfulPlacementResults(result.Placements)); len(issues) != 0 {
		t.Fatalf("geometry issues = %#v", issues)
	}
}

func TestValidateRelativeGroupRequiresAuthoredPositions(t *testing.T) {
	request := Request{
		Board:      BoardPlacementArea{WidthMM: 20, HeightMM: 20},
		Components: []Component{{Ref: "U1", Bounds: Bounds{WidthMM: 2, HeightMM: 2, Source: BoundsExplicit}}},
		Groups:     []Group{{ID: "core", Components: []string{"U1"}, Anchor: GroupAnchor{Ref: "U1"}, TranslateAsUnit: true}},
	}
	if issues := Validate(request); len(issues) == 0 {
		t.Fatal("expected missing authored-position validation issue")
	}
}

func TestValidateTranslatedGroupRequiresAnchorMembership(t *testing.T) {
	position := Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}
	request := Request{
		Board: BoardPlacementArea{WidthMM: 20, HeightMM: 20},
		Components: []Component{
			{Ref: "U1", Bounds: Bounds{WidthMM: 2, HeightMM: 2, Source: BoundsExplicit}, Position: &position},
			{Ref: "C1", Bounds: Bounds{WidthMM: 2, HeightMM: 2, Source: BoundsExplicit}, Position: &position},
		},
		Groups: []Group{{ID: "core", Components: []string{"C1"}, Anchor: GroupAnchor{Ref: "U1"}, TranslateAsUnit: true}},
	}
	if issues := Validate(request); len(issues) == 0 {
		t.Fatal("expected anchor-membership validation issue")
	}
}

func float64Pointer(value float64) *float64 {
	return &value
}
