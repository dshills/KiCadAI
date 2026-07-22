package placement

import (
	"strings"
	"testing"
)

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
	u1, hasU1 := byRef["U1"]
	c1, hasC1 := byRef["C1"]
	if !hasU1 || !hasC1 {
		t.Fatalf("missing expected group placements: %#v", result.Placements)
	}
	if delta := c1.Position.XMM - u1.Position.XMM; delta != 4 {
		t.Fatalf("relative X offset = %v, want 4; placements = %#v", delta, result.Placements)
	}
	if delta := byRef["C1"].Position.YMM - byRef["U1"].Position.YMM; delta != 0 {
		t.Fatalf("relative Y offset = %v, want 0; placements = %#v", delta, result.Placements)
	}
	if issues := ValidateGeometry(request, successfulPlacementResults(result.Placements)); len(issues) != 0 {
		t.Fatalf("geometry issues = %#v", issues)
	}
}

func TestPlaceTranslatableFixedGroupBeforeRejectingAuthoredCoordinates(t *testing.T) {
	rules := DefaultRules()
	rules.GridMM = 1
	rules.ComponentSpacingMM = 0
	request := Request{
		Board: BoardPlacementArea{WidthMM: 20, HeightMM: 20},
		Rules: rules,
		Components: []Component{
			{Ref: "U1", FootprintID: "Test:U", Bounds: Bounds{WidthMM: 2, HeightMM: 2, Source: BoundsExplicit}, Fixed: true, Position: &Placement{XMM: 24, YMM: 5, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)}},
			{Ref: "C1", FootprintID: "Test:C", Bounds: Bounds{WidthMM: 2, HeightMM: 2, Source: BoundsExplicit}, Fixed: true, Position: &Placement{XMM: 28, YMM: 5, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)}},
		},
		Groups: []Group{{ID: "core", Components: []string{"U1", "C1"}, Anchor: GroupAnchor{Ref: "U1"}, KeepTogether: true, TranslateAsUnit: true}},
	}

	result := Place(request)
	if result.Status != StatusPlaced {
		t.Fatalf("placement = %#v, want atomic group translation onto board", result)
	}
	byRef := map[string]PlacementResult{}
	for _, placed := range result.Placements {
		byRef[placed.Ref] = placed
	}
	if delta := byRef["C1"].Position.XMM - byRef["U1"].Position.XMM; delta != 4 {
		t.Fatalf("relative X offset = %v, want 4; placements = %#v", delta, result.Placements)
	}
	if issues := ValidateGeometry(request, successfulPlacementResults(result.Placements)); len(issues) != 0 {
		t.Fatalf("geometry issues = %#v", issues)
	}
}

func TestPlaceTranslatableGroupFailureIsAtomic(t *testing.T) {
	rules := DefaultRules()
	rules.GridMM = 1
	rules.ComponentSpacingMM = 0
	request := Request{
		Board: BoardPlacementArea{WidthMM: 10, HeightMM: 10},
		Rules: rules,
		Components: []Component{
			{Ref: "U1", FootprintID: "Test:U", Bounds: Bounds{WidthMM: 2, HeightMM: 2, Source: BoundsExplicit}, Fixed: true, Position: &Placement{XMM: 1, YMM: 5, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)}},
			{Ref: "C1", FootprintID: "Test:C", Bounds: Bounds{WidthMM: 2, HeightMM: 2, Source: BoundsExplicit}, Fixed: true, Position: &Placement{XMM: 21, YMM: 5, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)}},
		},
		Groups: []Group{{ID: "core", Components: []string{"U1", "C1"}, Anchor: GroupAnchor{Ref: "U1"}, KeepTogether: true, TranslateAsUnit: true}},
	}

	result := Place(request)
	if result.Status != StatusBlocked || result.Metrics.PlacedCount != 0 || result.Metrics.UnplacedCount != 2 {
		t.Fatalf("placement = %#v, want atomic group failure", result)
	}
	for _, placed := range result.Placements {
		if placed.Reason == "" {
			t.Fatalf("partially committed group member: %#v", placed)
		}
	}
}

func TestPreserveRelativeGroupPlacementUsesPhysicalInternalClearance(t *testing.T) {
	rules := DefaultRules()
	rules.GridMM = 0.25
	rules.ComponentSpacingMM = 1
	request := Request{
		Board: BoardPlacementArea{WidthMM: 20, HeightMM: 20},
		Rules: rules,
		Components: []Component{
			{Ref: "U1", FootprintID: "Test:U", Bounds: Bounds{WidthMM: 2, HeightMM: 2, Source: BoundsExplicit}, Position: &Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)}},
			{Ref: "C1", FootprintID: "Test:C", Bounds: Bounds{WidthMM: 2, HeightMM: 2, Source: BoundsExplicit}, Position: &Placement{XMM: 7.25, YMM: 5, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)}},
		},
		Groups: []Group{{ID: "core", Components: []string{"U1", "C1"}, Anchor: GroupAnchor{Ref: "U1"}, KeepTogether: true, TranslateAsUnit: true}},
	}

	result := Place(request)
	if result.Status != StatusPlaced || result.Metrics.PlacedCount != 2 {
		t.Fatalf("placement = %#v, want authored physically clear group preserved", result)
	}
	if issues := ValidateGeometry(request, result.Placements); len(issues) != 0 {
		t.Fatalf("geometry issues = %#v", issues)
	}
}

func TestPreserveRelativeGroupPlacementKeepsEdgeConstrainedAnchorAtEdge(t *testing.T) {
	rules := DefaultRules()
	rules.GridMM = 1
	rules.ComponentSpacingMM = 0
	request := Request{
		Board: BoardPlacementArea{WidthMM: 30, HeightMM: 20},
		Rules: rules,
		Components: []Component{
			{Ref: "U1", FootprintID: "Test:U", Bounds: Bounds{WidthMM: 4, HeightMM: 4, AnchorOffset: Point{XMM: 2, YMM: 2}, Source: BoundsExplicit}, Position: &Placement{XMM: 2, YMM: 10, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)}, Edge: EdgeLeft},
			{Ref: "C1", FootprintID: "Test:C", Bounds: Bounds{WidthMM: 2, HeightMM: 2, AnchorOffset: Point{XMM: 1, YMM: 1}, Source: BoundsExplicit}, Position: &Placement{XMM: 7, YMM: 10, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)}},
		},
		Groups: []Group{{ID: "radio", Components: []string{"U1", "C1"}, Anchor: GroupAnchor{Ref: "U1"}, KeepTogether: true, TranslateAsUnit: true}},
	}

	result := Place(request)
	if result.Status != StatusPlaced {
		t.Fatalf("placement = %#v", result)
	}
	for _, placed := range result.Placements {
		if placed.Ref == "U1" && !edgeConstraintSatisfied(request.Board, request.Components[0], placed.Position, EdgeLeft, edgeConstraintTolerance(request.Board, rules)) {
			t.Fatalf("translated anchor left edge: %#v", placed.Position)
		}
	}
}

func TestPreserveRelativeGroupPlacementKeepsGroupBoundsInsideUsableBoard(t *testing.T) {
	rules := DefaultRules()
	rules.GridMM = 1
	rules.ComponentSpacingMM = 0
	bounds := Rect{Min: Point{XMM: 0, YMM: 0}, Max: Point{XMM: 5, YMM: 5}}
	request := Request{
		Board: BoardPlacementArea{WidthMM: 20, HeightMM: 20},
		Rules: rules,
		Components: []Component{{
			Ref: "U1", FootprintID: "Test:U", Bounds: Bounds{WidthMM: 2, HeightMM: 2, AnchorOffset: Point{XMM: 1, YMM: 1}, Source: BoundsExplicit},
			Position: &Placement{XMM: 2, YMM: 2, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)},
		}},
		Groups: []Group{{ID: "core", Components: []string{"U1"}, Anchor: GroupAnchor{Ref: "U1"}, Bounds: &bounds, TranslateAsUnit: true}},
	}

	result := Place(request)
	if result.Status != StatusPlaced {
		t.Fatalf("placement = %#v", result)
	}
	placed := result.Placements[0].Position
	if !BoardUsableRect(request.Board, rules).Contains(translatedGroupBounds(bounds, *request.Components[0].Position, placed)) {
		t.Fatalf("translated group bounds escape usable board: placement=%#v bounds=%#v", placed, translatedGroupBounds(bounds, *request.Components[0].Position, placed))
	}
}

func TestPreserveRelativeEdgeGroupAllowsEnvelopeInsidePhysicalBoardMargin(t *testing.T) {
	rules := DefaultRules()
	rules.GridMM = 0.5
	rules.BoardEdgeClearanceMM = 1
	bounds := Rect{Min: Point{XMM: 0, YMM: 4}, Max: Point{XMM: 8, YMM: 16}}
	request := Request{
		Board: BoardPlacementArea{WidthMM: 20, HeightMM: 20},
		Rules: rules,
		Components: []Component{{
			Ref: "J1", FootprintID: "Test:J", Bounds: Bounds{WidthMM: 2, HeightMM: 4, AnchorOffset: Point{XMM: 1, YMM: 2}, Source: BoundsExplicit},
			Position: &Placement{XMM: 2, YMM: 10, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)}, Edge: EdgeLeft,
		}},
		Groups: []Group{{ID: "edge", Components: []string{"J1"}, Anchor: GroupAnchor{Ref: "J1"}, Bounds: &bounds, TranslateAsUnit: true}},
	}

	result := Place(request)
	if result.Status != StatusPlaced {
		t.Fatalf("placement = %#v, want edge group envelope preserved inside physical board", result)
	}
	placed := result.Placements[0].Position
	translated := translatedGroupBounds(bounds, *request.Components[0].Position, placed)
	if translated.Min.XMM != 0 || translated.Max.XMM > request.Board.WidthMM {
		t.Fatalf("translated edge envelope = %#v, want physical-board containment", translated)
	}
}

func TestTranslatedKeepoutFollowsRigidGroupAnchor(t *testing.T) {
	request := Request{
		Components: []Component{{Ref: "U1", Position: &Placement{XMM: 5, YMM: 6}}},
		Groups:     []Group{{ID: "radio", Anchor: GroupAnchor{Ref: "U1"}}},
		Keepouts:   []Keepout{{ID: "antenna", GroupID: "radio", Bounds: Rect{Min: Point{XMM: 1, YMM: 2}, Max: Point{XMM: 3, YMM: 4}}}},
	}
	placements := []PlacementResult{{Ref: "U1", Position: Placement{XMM: 15, YMM: 26}}}

	keepouts := TranslatedKeepoutsForPlacements(request, placements)
	if len(keepouts) != 1 || keepouts[0].Bounds.Min.XMM != 11 || keepouts[0].Bounds.Min.YMM != 22 || keepouts[0].Bounds.Max.XMM != 13 || keepouts[0].Bounds.Max.YMM != 24 {
		t.Fatalf("translated keepout = %#v", keepouts)
	}
}

func TestRigidGroupTranslationKeepsOwnedKeepoutClearOfExistingComponent(t *testing.T) {
	rules := DefaultRules()
	rules.GridMM = 1
	rules.ComponentSpacingMM = 0
	request := Request{
		Board: BoardPlacementArea{WidthMM: 30, HeightMM: 20},
		Rules: rules,
		Components: []Component{
			{Ref: "U1", FootprintID: "Test:U", Bounds: Bounds{WidthMM: 2, HeightMM: 2, AnchorOffset: Point{XMM: 1, YMM: 1}, Source: BoundsExplicit}, Position: &Placement{XMM: 5, YMM: 10, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)}},
			{Ref: "X1", FootprintID: "Test:X", Bounds: Bounds{WidthMM: 2, HeightMM: 2, AnchorOffset: Point{XMM: 1, YMM: 1}, Source: BoundsExplicit}, Fixed: true, Position: &Placement{XMM: 9, YMM: 10, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)}},
		},
		Groups:   []Group{{ID: "radio", Components: []string{"U1"}, Anchor: GroupAnchor{Ref: "U1"}, TranslateAsUnit: true}},
		Keepouts: []Keepout{{ID: "antenna", GroupID: "radio", Bounds: Rect{Min: Point{XMM: 11, YMM: 17}, Max: Point{XMM: 13, YMM: 19}}, ExemptRefs: []string{"U1"}}},
	}

	result := Place(request)
	if result.Status != StatusPlaced {
		t.Fatalf("placement = %#v", result)
	}
	keepouts := TranslatedKeepoutsForPlacements(request, result.Placements)
	var existing PlacementResult
	for _, placed := range result.Placements {
		if placed.Ref == "X1" {
			existing = placed
		}
	}
	if keepouts[0].Bounds.Intersects(existing.Bounds) {
		t.Fatalf("translated keepout %#v intersects existing component %#v", keepouts[0], existing)
	}
}

func TestRigidGroupOwnedKeepoutDoesNotRejectAuthoredMember(t *testing.T) {
	rules := DefaultRules()
	rules.GridMM = 1
	request := Request{
		Board: BoardPlacementArea{WidthMM: 20, HeightMM: 20},
		Rules: rules,
		Components: []Component{
			{Ref: "J1", FootprintID: "Test:J", Bounds: Bounds{WidthMM: 2, HeightMM: 2, AnchorOffset: Point{XMM: 1, YMM: 1}, Source: BoundsExplicit}, Position: &Placement{XMM: 5, YMM: 10, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)}},
			{Ref: "R1", FootprintID: "Test:R", Bounds: Bounds{WidthMM: 2, HeightMM: 1, AnchorOffset: Point{XMM: 1, YMM: 0.5}, Source: BoundsExplicit}, Position: &Placement{XMM: 8, YMM: 10, Layer: "F.Cu"}, Rotation: RotationConstraint{FixedDeg: float64Pointer(0)}},
		},
		Groups:   []Group{{ID: "entry", Components: []string{"J1", "R1"}, Anchor: GroupAnchor{Ref: "J1"}, TranslateAsUnit: true}},
		Keepouts: []Keepout{{ID: "edge", GroupID: "entry", Bounds: Rect{Min: Point{XMM: 3, YMM: 7}, Max: Point{XMM: 8.25, YMM: 13}}, ExemptRefs: []string{"J1"}}},
	}

	result := Place(request)
	if result.Status != StatusPlaced {
		t.Fatalf("placement = %#v, want authored member preserved inside its group-owned envelope", result)
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

func TestValidateRejectsComponentInMultipleTranslatedGroups(t *testing.T) {
	position := Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}
	request := Request{
		Board:      BoardPlacementArea{WidthMM: 20, HeightMM: 20},
		Components: []Component{{Ref: "U1", Bounds: Bounds{WidthMM: 2, HeightMM: 2, Source: BoundsExplicit}, Position: &position}},
		Groups: []Group{
			{ID: "first", Components: []string{"U1"}, Anchor: GroupAnchor{Ref: "U1"}, TranslateAsUnit: true},
			{ID: "second", Components: []string{"U1"}, Anchor: GroupAnchor{Ref: "U1"}, TranslateAsUnit: true},
		},
	}
	issues := Validate(request)
	for _, validationIssue := range issues {
		if strings.Contains(validationIssue.Message, "belongs to multiple translated groups") {
			return
		}
	}
	t.Fatalf("validation issues = %#v, want translated-group ownership issue", issues)
}

func float64Pointer(value float64) *float64 {
	return &value
}
