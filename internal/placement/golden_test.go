package placement

import "testing"

func TestGoldenConnectorEdgePlacement(t *testing.T) {
	req := Request{
		Board: BoardPlacementArea{WidthMM: 60, HeightMM: 30, MarginMM: 1},
		Components: []Component{
			{
				Ref:         "J1",
				FootprintID: "Connector_Generic:Conn_01x04",
				Bounds:      Bounds{WidthMM: 1, HeightMM: 1, AnchorOffset: Point{XMM: 0.5, YMM: 0.5}, Source: BoundsExplicit},
				Edge:        EdgeLeft,
				Pads:        []PadSummary{{Name: "1"}, {Name: "2"}, {Name: "3"}, {Name: "4"}},
			},
		},
		Rules: Rules{MaxCandidatesPerPart: 100000},
	}

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	quality := BuildQualityReport(req, result)
	if quality.EdgeConstraintSatisfied != 1 {
		t.Fatalf("edge constraints = %d/%d, want 1/1; placement=%#v", quality.EdgeConstraintSatisfied, quality.EdgeConstraintCount, result.Placements)
	}
}

func TestGoldenDecouplingPlacementNearIC(t *testing.T) {
	req := Request{
		Board: BoardPlacementArea{WidthMM: 50, HeightMM: 25, MarginMM: 1},
		Components: []Component{
			{
				Ref:         "U1",
				FootprintID: "Package_SO:SOIC-8",
				Bounds:      Bounds{WidthMM: 5, HeightMM: 4, AnchorOffset: Point{XMM: 2.5, YMM: 2}, Source: BoundsExplicit},
				Fixed:       true,
				Position:    &Placement{XMM: 35, YMM: 12, Layer: "F.Cu"},
				Pads:        []PadSummary{{Name: "8"}},
			},
			{
				Ref:         "C1",
				FootprintID: "Capacitor_SMD:C_0805_2012Metric",
				Bounds:      Bounds{WidthMM: 2, HeightMM: 1.25, AnchorOffset: Point{XMM: 1, YMM: 0.625}, Source: BoundsExplicit},
				Pads:        []PadSummary{{Name: "1"}},
			},
		},
		Nets: []Net{{Name: "VCC", Weight: 5, Endpoints: []Endpoint{{Ref: "U1", Pin: "8"}, {Ref: "C1", Pin: "1"}}}},
	}

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	placements := placementResultsByRef(result.Placements)
	if boardDistance(placements["U1"].Bounds.Center().XMM-placements["C1"].Bounds.Center().XMM, placements["U1"].Bounds.Center().YMM-placements["C1"].Bounds.Center().YMM) > 15 {
		t.Fatalf("C1 placed too far from U1: U1=%#v C1=%#v", placements["U1"].Position, placements["C1"].Position)
	}
}

func TestGoldenKeepoutAvoidancePlacement(t *testing.T) {
	req := minimalRequest()
	req.Keepouts = []Keepout{{
		ID:     "mounting",
		Bounds: Rect{Min: Point{XMM: 0, YMM: 0}, Max: Point{XMM: 8, YMM: 8}},
		Layers: []string{"F.Cu"},
	}}

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	if result.Placements[0].Bounds.Intersects(req.Keepouts[0].Bounds) {
		t.Fatalf("placement intersects keepout: placement=%#v keepout=%#v", result.Placements[0].Bounds, req.Keepouts[0].Bounds)
	}
	quality := BuildQualityReport(req, result)
	if quality.KeepoutCount != 1 || quality.GeometryIssueCount != 0 {
		t.Fatalf("quality keepout summary = %#v", quality)
	}
}

func TestGoldenBottomSidePlacement(t *testing.T) {
	req := minimalRequest()
	req.Rules.AllowBackLayer = true
	req.Components[0].Side = SideBottom

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	if result.Placements[0].Position.Layer != "B.Cu" {
		t.Fatalf("layer = %s, want B.Cu", result.Placements[0].Position.Layer)
	}
	quality := BuildQualityReport(req, result)
	if quality.SideConstraintSatisfied != 1 {
		t.Fatalf("side constraints = %d/%d, want 1/1", quality.SideConstraintSatisfied, quality.SideConstraintCount)
	}
}

func TestGoldenQualityReportsEstimatedBounds(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Bounds.Source = BoundsEstimated

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	quality := BuildQualityReport(req, result)
	if quality.Metrics.EstimatedBoundsCount != 1 || len(quality.EstimatedBoundsRefs) != 1 || quality.EstimatedBoundsRefs[0] != "R1" {
		t.Fatalf("estimated bounds quality = %#v", quality)
	}
	if len(quality.PlacementQualityWarnings) == 0 {
		t.Fatalf("expected estimated bounds warning: %#v", quality)
	}
}
