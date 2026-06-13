package placement

import "testing"

func TestBoardUsableRectUsesLargestClearance(t *testing.T) {
	board := BoardPlacementArea{WidthMM: 20, HeightMM: 10, MarginMM: 1}
	rules := Rules{BoardEdgeClearanceMM: 2}

	got := BoardUsableRect(board, rules)
	want := Rect{Min: Point{XMM: 2, YMM: 2}, Max: Point{XMM: 18, YMM: 8}}
	if got != want {
		t.Fatalf("BoardUsableRect = %#v, want %#v", got, want)
	}
}

func TestBoardUsableRectClampsImpossibleClearance(t *testing.T) {
	board := BoardPlacementArea{WidthMM: 4, HeightMM: 2, MarginMM: 3}

	got := BoardUsableRect(board, Rules{})
	if got.Min.XMM > got.Max.XMM || got.Min.YMM > got.Max.YMM {
		t.Fatalf("BoardUsableRect returned inverted rect: %#v", got)
	}
	if !nearlyEqual(got.WidthMM(), 0) || !nearlyEqual(got.HeightMM(), 0) {
		t.Fatalf("BoardUsableRect size = %.3fx%.3f, want zero area", got.WidthMM(), got.HeightMM())
	}
}

func TestComponentPlacementBoundsUsesAnchorAndRotation(t *testing.T) {
	component := Component{
		Ref:    "U1",
		Bounds: Bounds{WidthMM: 4, HeightMM: 2, AnchorOffset: Point{XMM: 1, YMM: 0.5}},
	}
	placement := Placement{XMM: 10, YMM: 10, RotationDeg: 90}

	got, ok := ComponentPlacementBounds(component, placement, Rules{})
	if !ok {
		t.Fatal("ComponentPlacementBounds returned false")
	}
	if !nearlyEqual(got.WidthMM(), 2) || !nearlyEqual(got.HeightMM(), 4) {
		t.Fatalf("rotated bounds size = %.3fx%.3f, want 2x4", got.WidthMM(), got.HeightMM())
	}
	if !nearlyEqual(got.Center().XMM, 9.5) || !nearlyEqual(got.Center().YMM, 11) {
		t.Fatalf("rotated bounds center = %#v, want approximately {9.5, 11}", got.Center())
	}
}

func TestComponentPlacementBoundsRejectsNonRightAngleRotation(t *testing.T) {
	component := Component{
		Ref:    "U1",
		Bounds: Bounds{WidthMM: 4, HeightMM: 2},
	}

	if _, ok := ComponentPlacementBounds(component, Placement{RotationDeg: 45}, Rules{}); ok {
		t.Fatal("ComponentPlacementBounds accepted non-right-angle rotation")
	}
}

func TestComponentPlacementBoundsAppliesSpacingAndCourtyard(t *testing.T) {
	component := Component{
		Ref:    "C1",
		Bounds: Bounds{WidthMM: 2, HeightMM: 1, CourtyardMM: 0.2},
	}
	rules := Rules{ComponentSpacingMM: 0.3}

	got, ok := ComponentPlacementBounds(component, Placement{XMM: 5, YMM: 5}, rules)
	if !ok {
		t.Fatal("ComponentPlacementBounds returned false")
	}
	if !nearlyEqual(got.WidthMM(), 2.7) || !nearlyEqual(got.HeightMM(), 1.7) {
		t.Fatalf("bounds size with spacing = %.3fx%.3f, want 2.7x1.7", got.WidthMM(), got.HeightMM())
	}
}

func TestValidateGeometryRejectsOutsideBoard(t *testing.T) {
	req := minimalRequest()
	req.Rules.BoardEdgeClearanceMM = 1
	placement, ok := NewPlacementResult(req.Components[0], Placement{XMM: 0.5, YMM: 0.5}, normalizeRules(req.Rules))
	if !ok {
		t.Fatal("NewPlacementResult returned false")
	}

	issues := ValidateGeometry(req, []PlacementResult{placement})
	assertIssueContains(t, issues, "placement is outside usable board area")
}

func TestValidateGeometryUsesCalculatedBoundsWithoutMutatingInput(t *testing.T) {
	req := minimalRequest()
	placements := []PlacementResult{{
		Ref:      "R1",
		Position: Placement{XMM: 5, YMM: 5},
	}}

	issues := ValidateGeometry(req, placements)
	if len(issues) != 0 {
		t.Fatalf("ValidateGeometry returned issues: %#v", issues)
	}
	if !placements[0].Bounds.IsZero() {
		t.Fatal("ValidateGeometry mutated input placement bounds")
	}
}

func TestValidateGeometryRejectsSameLayerOverlap(t *testing.T) {
	req := twoComponentRequest()
	rules := normalizeRules(req.Rules)
	first, _ := NewPlacementResult(req.Components[0], Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}, rules)
	second, _ := NewPlacementResult(req.Components[1], Placement{XMM: 5.5, YMM: 5, Layer: "F.Cu"}, rules)

	issues := ValidateGeometry(req, []PlacementResult{first, second})
	assertIssueContains(t, issues, "placement conflicts with component R1")
}

func TestValidateGeometryAllowsDifferentLayerOverlap(t *testing.T) {
	req := twoComponentRequest()
	req.Rules.AllowBackLayer = true
	rules := normalizeRules(req.Rules)
	first, _ := NewPlacementResult(req.Components[0], Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}, rules)
	second, _ := NewPlacementResult(req.Components[1], Placement{XMM: 5, YMM: 5, Layer: "B.Cu"}, rules)

	issues := ValidateGeometry(req, []PlacementResult{first, second})
	if len(issues) != 0 {
		t.Fatalf("ValidateGeometry returned issues for different-layer overlap: %#v", issues)
	}
}

func TestValidateGeometryRejectsKeepoutOverlap(t *testing.T) {
	req := minimalRequest()
	req.Keepouts = []Keepout{{
		ID:     "mounting",
		Bounds: Rect{Min: Point{XMM: 4, YMM: 4}, Max: Point{XMM: 8, YMM: 8}},
		Layers: []string{"F.Cu"},
	}}
	placement, _ := NewPlacementResult(req.Components[0], Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}, normalizeRules(req.Rules))

	issues := ValidateGeometry(req, []PlacementResult{placement})
	assertIssueContains(t, issues, "placement conflicts with keepout mounting")
}

func TestOccupancyTreatsTouchingEdgesAsNonIntersecting(t *testing.T) {
	a := Rect{Min: Point{XMM: 0, YMM: 0}, Max: Point{XMM: 1, YMM: 1}}
	b := Rect{Min: Point{XMM: 1, YMM: 0}, Max: Point{XMM: 2, YMM: 1}}
	if a.Intersects(b) {
		t.Fatal("touching edges should not intersect")
	}
}

func twoComponentRequest() Request {
	req := minimalRequest()
	second := req.Components[0]
	second.Ref = "R2"
	req.Components = append(req.Components, second)
	req.Nets[0].Endpoints = append(req.Nets[0].Endpoints, Endpoint{Ref: "R2", Pin: "1"})
	return req
}
