package placement

import "testing"

func TestPlacePlacesSimpleRequest(t *testing.T) {
	req := twoComponentRequest()

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v summary=%s", result.Status, result.Issues, placementSummary(result))
	}
	if result.Metrics.PlacedCount != 2 || len(result.Placements) != 2 {
		t.Fatalf("placed count = %d len=%d, want 2", result.Metrics.PlacedCount, len(result.Placements))
	}
	if len(ValidateGeometry(req, result.Placements)) != 0 {
		t.Fatalf("result placements failed geometry validation: %#v", ValidateGeometry(req, result.Placements))
	}
}

func TestPlacePreservesFixedComponent(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Fixed = true
	req.Components[0].Position = &Placement{XMM: 10, YMM: 10, RotationDeg: 90, Layer: "F.Cu"}

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	if result.Metrics.FixedCount != 1 {
		t.Fatalf("fixed count = %d, want 1", result.Metrics.FixedCount)
	}
	got := result.Placements[0].Position
	if got.XMM != 10 || got.YMM != 10 || got.RotationDeg != 90 {
		t.Fatalf("fixed position = %#v, want original", got)
	}
}

func TestPlacePreservesPositionWhenExistingPolicyPreserveFixed(t *testing.T) {
	req := minimalRequest()
	req.Existing.PreserveFixed = true
	req.Components[0].Position = &Placement{XMM: 10, YMM: 10, RotationDeg: 90, Layer: "F.Cu"}

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	if result.Metrics.FixedCount != 1 {
		t.Fatalf("fixed count = %d, want 1", result.Metrics.FixedCount)
	}
	got := result.Placements[0].Position
	if got.XMM != 10 || got.YMM != 10 || got.RotationDeg != 90 {
		t.Fatalf("preserved position = %#v, want original", got)
	}
}

func TestPlaceRejectsFixedComponentCollision(t *testing.T) {
	req := twoComponentRequest()
	req.Components[0].Fixed = true
	req.Components[0].Position = &Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}
	req.Components[1].Fixed = true
	req.Components[1].Position = &Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}

	result := Place(req)
	if result.Status != StatusPartial {
		t.Fatalf("status = %s, want partial; issues=%#v", result.Status, result.Issues)
	}
	if result.Metrics.PlacedCount != 1 || result.Metrics.FixedCount != 1 || result.Metrics.UnplacedCount != 1 {
		t.Fatalf("metrics = %#v, want one placed fixed and one unplaced", result.Metrics)
	}
	assertIssueContains(t, result.Issues, "fixed placement conflicts with component R1")
}

func TestPlaceRejectsFixedComponentKeepoutOverlap(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Fixed = true
	req.Components[0].Position = &Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}
	req.Keepouts = []Keepout{{
		ID:     "mounting",
		Bounds: Rect{Min: Point{XMM: 4, YMM: 4}, Max: Point{XMM: 6, YMM: 6}},
		Layers: []string{"F.Cu"},
	}}

	result := Place(req)
	if result.Status != StatusBlocked {
		t.Fatalf("status = %s, want blocked; issues=%#v", result.Status, result.Issues)
	}
	if result.Metrics.PlacedCount != 0 || result.Metrics.FixedCount != 0 || result.Metrics.UnplacedCount != 1 {
		t.Fatalf("metrics = %#v, want blocked fixed component unplaced", result.Metrics)
	}
	assertIssueContains(t, result.Issues, "fixed placement conflicts with keepout mounting")
}

func TestPlaceUsesPhysicalBoundsForBoardEdgeClearance(t *testing.T) {
	req := minimalRequest()
	req.Board.WidthMM = 10
	req.Board.HeightMM = 5
	req.Board.MarginMM = 0
	req.Rules.BoardEdgeClearanceMM = 1
	req.Rules.ComponentSpacingMM = 4
	req.Rules.MaxCandidatesPerPart = 100
	req.Components[0].Bounds = Bounds{WidthMM: 8, HeightMM: 3, AnchorOffset: Point{XMM: 4, YMM: 1.5}, Source: BoundsExplicit}

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v summary=%s", result.Status, result.Issues, placementSummary(result))
	}
}

func TestPlaceReportsPartialWhenNoLegalCandidate(t *testing.T) {
	req := minimalRequest()
	req.Board.WidthMM = 2
	req.Board.HeightMM = 2
	req.Board.MarginMM = 0
	req.Rules.BoardEdgeClearanceMM = 0.1

	result := Place(req)
	if result.Status != StatusBlocked && result.Status != StatusPartial {
		t.Fatalf("status = %s, want blocked or partial", result.Status)
	}
	if result.Metrics.UnplacedCount != 1 {
		t.Fatalf("unplaced count = %d, want 1", result.Metrics.UnplacedCount)
	}
	assertIssueContains(t, result.Issues, "no legal placement found")
	for _, issue := range result.Issues {
		if issue.Message == "placement is outside usable board area" {
			t.Fatalf("unplaced placeholder should not be geometry-validated: %#v", result.Issues)
		}
	}
}

func TestPlaceIsDeterministic(t *testing.T) {
	req := twoComponentRequest()

	first := Place(req)
	second := Place(req)
	if len(first.Placements) != len(second.Placements) {
		t.Fatalf("placement lengths differ: %d vs %d", len(first.Placements), len(second.Placements))
	}
	for i := range first.Placements {
		if first.Placements[i] != second.Placements[i] {
			t.Fatalf("placement[%d] differs:\nfirst=%#v\nsecond=%#v", i, first.Placements[i], second.Placements[i])
		}
	}
}

func TestPlaceComputesHPWL(t *testing.T) {
	req := twoComponentRequest()

	result := Place(req)
	if result.Metrics.HPWLMM <= 0 {
		t.Fatalf("HPWLMM = %f, want positive", result.Metrics.HPWLMM)
	}
}

func TestPlaceCountsEstimatedBoundsSources(t *testing.T) {
	req := Request{
		Board: BoardPlacementArea{WidthMM: 50, HeightMM: 25, MarginMM: 1},
		Components: []Component{
			{Ref: "R1", FootprintID: "Test:R1", Bounds: Bounds{WidthMM: 2, HeightMM: 1, Source: BoundsExplicit}},
			{Ref: "R2", FootprintID: "Test:R2", Bounds: Bounds{WidthMM: 2, HeightMM: 1, Source: BoundsLibraryCourtyard}},
			{Ref: "R3", FootprintID: "Test:R3", Bounds: Bounds{WidthMM: 2, HeightMM: 1, Source: BoundsLibraryPads}},
			{Ref: "R4", FootprintID: "Test:R4", Bounds: Bounds{WidthMM: 2, HeightMM: 1, Source: BoundsGeneratedPads}},
			{Ref: "R5", FootprintID: "Test:R5", Bounds: Bounds{WidthMM: 2, HeightMM: 1, Source: BoundsEstimated}},
		},
		Rules: Rules{MaxCandidatesPerPart: 200},
	}

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v summary=%s", result.Status, result.Issues, placementSummary(result))
	}
	if result.Metrics.EstimatedBoundsCount != 3 {
		t.Fatalf("estimated bounds count = %d, want 3", result.Metrics.EstimatedBoundsCount)
	}
}

func TestPlaceSamplesAcrossBoardWhenCandidateCapIsSmall(t *testing.T) {
	req := minimalRequest()
	req.Board.WidthMM = 100
	req.Board.HeightMM = 20
	req.Components[0].Edge = EdgeRight
	req.Rules.MaxCandidatesPerPart = 10

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	if result.Placements[0].Position.XMM < 80 {
		t.Fatalf("right-edge placement X = %.2f, want sampled near right edge", result.Placements[0].Position.XMM)
	}
}

func TestPlaceScoresBottomEdgeWithoutCancelingY(t *testing.T) {
	req := minimalRequest()
	req.Board.WidthMM = 30
	req.Board.HeightMM = 100
	req.Components[0].Edge = EdgeBottom
	req.Rules.MaxCandidatesPerPart = 10

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	if result.Placements[0].Position.YMM < 80 {
		t.Fatalf("bottom-edge placement Y = %.2f, want sampled near bottom edge", result.Placements[0].Position.YMM)
	}
}

func TestPlaceCanUseBottomLayerForSideAny(t *testing.T) {
	req := twoComponentRequest()
	req.Rules.AllowBackLayer = true
	req.Components[0].Side = SideTop
	req.Components[1].Side = SideAny
	req.Components[0].Fixed = true
	req.Components[0].Position = &Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}
	req.Components[1].Fixed = true
	req.Components[1].Position = &Placement{XMM: 5, YMM: 5, Layer: "B.Cu"}

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
}

func TestCandidateLayersRespectSideConstraints(t *testing.T) {
	rules := Rules{AllowBackLayer: true}
	if got := candidateLayers(Component{Side: SideTop}, rules); len(got) != 1 || got[0] != "F.Cu" {
		t.Fatalf("top candidate layers = %#v, want F.Cu", got)
	}
	if got := candidateLayers(Component{Side: SideBottom}, rules); len(got) != 1 || got[0] != "B.Cu" {
		t.Fatalf("bottom candidate layers = %#v, want B.Cu", got)
	}
	if got := candidateLayers(Component{Side: SideAny}, rules); len(got) != 2 {
		t.Fatalf("any candidate layers = %#v, want top and bottom", got)
	}
}

func TestPlaceContinuesPastInvalidCandidateRotation(t *testing.T) {
	req := minimalRequest()
	req = NormalizeRequest(req)
	req.Components[0].Rotation.AllowedDeg = []float64{45, 0}

	padsByRef := componentPadMaps(req.Components)
	result, ok, _ := placeComponent(req.Components[0], req, newOccupancy(req), nil, padsByRef, componentRotatedPadMaps(req.Components, padsByRef), netsByComponent(req.Nets), keepTogetherPeersByComponent(req))
	if !ok {
		t.Fatal("placeComponent failed after invalid candidate rotation")
	}
	if result.Position.RotationDeg != 0 {
		t.Fatalf("rotation = %.1f, want valid fallback rotation", result.Position.RotationDeg)
	}
}

func TestRoundToGridHandlesNegativeCoordinates(t *testing.T) {
	if got := roundToGrid(-1.1, 0.5); got != -1 {
		t.Fatalf("roundToGrid(-1.1, 0.5) = %v, want -1", got)
	}
}

func TestPlaceScoresConnectedComponentNearPlacedNetPeer(t *testing.T) {
	req := Request{
		Board: BoardPlacementArea{WidthMM: 50, HeightMM: 25, MarginMM: 1},
		Components: []Component{
			{
				Ref:         "U1",
				FootprintID: "Package_SO:SOIC-8",
				Bounds:      Bounds{WidthMM: 5, HeightMM: 4, Source: BoundsExplicit},
				Fixed:       true,
				Position:    &Placement{XMM: 35, YMM: 12, Layer: "F.Cu"},
			},
			{
				Ref:         "C1",
				FootprintID: "Capacitor_SMD:C_0805_2012Metric",
				Bounds:      Bounds{WidthMM: 2, HeightMM: 1.25, Source: BoundsExplicit},
			},
		},
		Nets: []Net{{Name: "VCC", Weight: 5, Endpoints: []Endpoint{{Ref: "U1", Pin: "8"}, {Ref: "C1", Pin: "1"}}}},
	}

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	var capPlacement PlacementResult
	for _, placement := range result.Placements {
		if placement.Ref == "C1" {
			capPlacement = placement
		}
	}
	if capPlacement.Ref == "" {
		t.Fatalf("missing C1 placement: %#v", result.Placements)
	}
	if capPlacement.Position.XMM < 25 {
		t.Fatalf("C1 X = %.2f, want placement near fixed U1", capPlacement.Position.XMM)
	}
}

func TestSeedTieBreakIsStable(t *testing.T) {
	placement := Placement{XMM: 1, YMM: 2, RotationDeg: 90, Layer: "F.Cu"}
	base := seedTieBreakBase("abc", "R1")
	first := seedTieBreak(base, placement)
	second := seedTieBreak(base, placement)
	if first != second {
		t.Fatalf("seed tie break changed: %f vs %f", first, second)
	}
}

func TestSeedTieBreakInfluencesEquivalentPlacements(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Bounds = Bounds{
		WidthMM:      1,
		HeightMM:     1,
		AnchorOffset: Point{XMM: 0.5, YMM: 0.5},
		Source:       BoundsExplicit,
	}
	req.Rules.MaxCandidatesPerPart = 100
	req.Components[0].Rotation.AllowedDeg = []float64{0, 90, 180, 270}
	req.Seed = "seed-0"

	first := Place(req)
	second := Place(req)
	if first.Status != StatusPlaced || second.Status != StatusPlaced {
		t.Fatalf("seeded placements failed: first=%#v second=%#v", first, second)
	}
	if first.Placements[0].Position != second.Placements[0].Position {
		t.Fatalf("same seed produced different placements: %#v vs %#v", first.Placements[0].Position, second.Placements[0].Position)
	}

	different := false
	for _, seed := range []string{"seed-1", "seed-2", "seed-3", "seed-4", "seed-5", "seed-6", "seed-7"} {
		req.Seed = seed
		result := Place(req)
		if result.Status != StatusPlaced {
			t.Fatalf("placement for %s failed: %#v", seed, result)
		}
		if result.Placements[0].Position != first.Placements[0].Position {
			different = true
			break
		}
	}
	if !different {
		t.Fatalf("different seeds did not choose a different equivalent placement from %#v", first.Placements[0].Position)
	}
}
