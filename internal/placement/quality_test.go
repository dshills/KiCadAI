package placement

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestQualityReportScoresProximityWithPadEvidence(t *testing.T) {
	req := minimalRequest()
	req.Components = append(req.Components, Component{
		Ref:         "C1",
		FootprintID: "Capacitor_SMD:C_0805_2012Metric",
		Bounds:      Bounds{WidthMM: 2, HeightMM: 1.25, Source: BoundsExplicit},
		Pads:        []PadSummary{{Name: "1", XMM: 0, YMM: 0}},
	})
	req.Components[0].Pads[0].XMM = 0
	req.Components[0].Pads[0].YMM = 0
	req.ProximityRules = []ProximityRule{{
		ID:            "decoupling",
		Role:          IntentDecoupling,
		AnchorRef:     "R1",
		TargetRefs:    []string{"C1"},
		AnchorPins:    []string{"1"},
		TargetPins:    []string{"1"},
		MaxDistanceMM: 5,
		Required:      true,
	}}
	result := Result{
		Status: StatusPlaced,
		Placements: []PlacementResult{
			mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5}),
			mustPlacementResultForTest(t, req.Components[1], Placement{XMM: 8, YMM: 5}),
		},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.ProximityReports) != 1 {
		t.Fatalf("proximity reports = %#v", quality.ProximityReports)
	}
	report := quality.ProximityReports[0]
	if !report.Satisfied || report.Evidence != "pad" || report.ActualMM == nil {
		t.Fatalf("unexpected proximity report: %#v", report)
	}
	if len(quality.Score.Dimensions) == 0 || quality.Score.Dimensions[0].Status != "pass" {
		t.Fatalf("unexpected score: %#v", quality.Score)
	}
}

func TestQualityReportScoresFailedProximity(t *testing.T) {
	req := minimalRequest()
	req.Components = append(req.Components, Component{
		Ref:         "C1",
		FootprintID: "Capacitor_SMD:C_0805_2012Metric",
		Bounds:      Bounds{WidthMM: 2, HeightMM: 1.25, Source: BoundsExplicit},
	})
	req.ProximityRules = []ProximityRule{{
		ID:            "far",
		AnchorRef:     "R1",
		TargetRefs:    []string{"C1"},
		MaxDistanceMM: 2,
		Required:      true,
	}}
	result := Result{
		Status: StatusPlaced,
		Placements: []PlacementResult{
			mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5}),
			mustPlacementResultForTest(t, req.Components[1], Placement{XMM: 30, YMM: 5}),
		},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.ProximityReports) != 1 || quality.ProximityReports[0].Satisfied {
		t.Fatalf("unexpected proximity reports: %#v", quality.ProximityReports)
	}
	if len(quality.Score.Dimensions) == 0 || quality.Score.Dimensions[0].Status != "fail" {
		t.Fatalf("unexpected score: %#v", quality.Score)
	}
}

func TestQualityReportScoresGroupCohesion(t *testing.T) {
	req := twoComponentRequest()
	req.Groups = []Group{{
		ID:           "analog",
		Components:   []string{"R1", "R2"},
		KeepTogether: true,
		MaxSpreadMM:  2,
	}}
	result := Result{
		Status: StatusPlaced,
		Placements: []PlacementResult{
			mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5}),
			mustPlacementResultForTest(t, req.Components[1], Placement{XMM: 30, YMM: 5}),
		},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.GroupReports) != 1 || quality.GroupReports[0].SpreadSatisfied {
		t.Fatalf("unexpected group reports: %#v", quality.GroupReports)
	}
	var found bool
	for _, dimension := range quality.Score.Dimensions {
		if dimension.Name == "group_cohesion" {
			found = true
			if dimension.Status != "fail" {
				t.Fatalf("group cohesion status = %q, want fail", dimension.Status)
			}
		}
	}
	if !found {
		t.Fatalf("missing group cohesion score: %#v", quality.Score)
	}
}

func TestQualityReportScoresEdgeConstraints(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Edge = EdgeRight
	result := Result{
		Status:     StatusPlaced,
		Placements: []PlacementResult{mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5})},
	}

	quality := BuildQualityReport(req, result)
	if quality.EdgeConstraintCount != 1 || quality.EdgeConstraintSatisfied != 0 {
		t.Fatalf("unexpected edge counts: %#v", quality)
	}
	var found bool
	for _, dimension := range quality.Score.Dimensions {
		if dimension.Name == "edge_constraints" {
			found = true
			if dimension.Status != "fail" || dimension.Score != 0 {
				t.Fatalf("edge dimension = %#v, want failing zero score", dimension)
			}
		}
	}
	if !found {
		t.Fatalf("missing edge score: %#v", quality.Score)
	}
}

func TestQualityReportScoresRequiredRegionMiss(t *testing.T) {
	req := minimalRequest()
	req.RegionRules = []RegionRule{{
		ID:        "analog",
		Region:    "analog",
		Refs:      []string{"R1"},
		Preferred: Rect{Min: Point{XMM: 0, YMM: 0}, Max: Point{XMM: 10, YMM: 10}},
		Required:  true,
	}}
	result := Result{
		Status:     StatusPlaced,
		Placements: []PlacementResult{mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 30, YMM: 20})},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.RegionReports) != 1 || quality.RegionReports[0].Satisfied || len(quality.RegionReports[0].OutsideRefs) != 1 {
		t.Fatalf("unexpected region report: %#v", quality.RegionReports)
	}
	var found bool
	for _, dimension := range quality.Score.Dimensions {
		if dimension.Name == "regions" {
			found = true
			if dimension.Status != "fail" || dimension.Score != 0 {
				t.Fatalf("region dimension = %#v, want failing zero score", dimension)
			}
		}
	}
	if !found {
		t.Fatalf("missing region score: %#v", quality.Score)
	}
}

func TestQualityReportIncludesCongestionReports(t *testing.T) {
	req := twoComponentRequest()
	req.Board = BoardPlacementArea{WidthMM: 20, HeightMM: 20}
	req.Nets = []Net{
		{Name: "A", Weight: 3, Endpoints: []Endpoint{{Ref: "R1", Pin: "1"}, {Ref: "R2", Pin: "1"}}},
		{Name: "B", Weight: 3, Endpoints: []Endpoint{{Ref: "R1", Pin: "1"}, {Ref: "R2", Pin: "1"}}},
	}
	result := Result{
		Status: StatusPlaced,
		Placements: []PlacementResult{
			mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5}),
			mustPlacementResultForTest(t, req.Components[1], Placement{XMM: 15, YMM: 15}),
		},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.CongestionReports) == 0 {
		t.Fatalf("missing congestion reports")
	}
	for index := 1; index < len(quality.CongestionReports); index++ {
		if quality.CongestionReports[index-1].CellID > quality.CongestionReports[index].CellID {
			t.Fatalf("congestion reports not sorted: %#v", quality.CongestionReports)
		}
	}
	var foundScore bool
	for _, dimension := range quality.Score.Dimensions {
		if dimension.Name == "congestion" {
			foundScore = true
			if dimension.Score < 0 || dimension.Score > 1 {
				t.Fatalf("congestion score out of range: %#v", dimension)
			}
		}
	}
	if !foundScore {
		t.Fatalf("missing congestion score: %#v", quality.Score)
	}
}

func TestCongestionAxisCellsIsCapped(t *testing.T) {
	if got := congestionAxisCells(10000, 100000); got > congestionMaxGridCellsPerAxis {
		t.Fatalf("cells = %d, want capped at %d", got, congestionMaxGridCellsPerAxis)
	}
}

func TestCongestionReportsRespectBoardOrigin(t *testing.T) {
	req := twoComponentRequest()
	req.Board = BoardPlacementArea{
		Origin:   Point{XMM: 100, YMM: 50},
		WidthMM:  20,
		HeightMM: 20,
	}
	req.Nets = []Net{
		{Name: "A", Endpoints: []Endpoint{{Ref: "R1", Pin: "1"}, {Ref: "R2", Pin: "1"}}},
	}
	result := Result{
		Status: StatusPlaced,
		Placements: []PlacementResult{
			mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 105, YMM: 55}),
			mustPlacementResultForTest(t, req.Components[1], Placement{XMM: 115, YMM: 65}),
		},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.CongestionReports) == 0 {
		t.Fatalf("missing congestion reports")
	}
	for _, report := range quality.CongestionReports {
		if report.Bounds.Min.XMM < req.Board.Origin.XMM || report.Bounds.Min.YMM < req.Board.Origin.YMM {
			t.Fatalf("congestion cell outside board origin: %#v", report)
		}
	}
}

func TestSampleCongestionPointsHandlesLimitOne(t *testing.T) {
	points := []endpointPoint{
		{ref: "R1", point: Point{XMM: 0, YMM: 0}},
		{ref: "R2", point: Point{XMM: 1, YMM: 1}},
	}

	sampled := sampleCongestionPoints(points, 1)
	if len(sampled) != 1 || sampled[0].ref != "R1" {
		t.Fatalf("sampled = %#v, want first point only", sampled)
	}
}

func TestQualityReportFanoutAllowsConnectorNearEdge(t *testing.T) {
	req := fanoutRequest("J1", 12)
	req.Components[0].Role = string(IntentConnector)
	req.Components[0].Edge = EdgeLeft
	result := Result{
		Status:     StatusPlaced,
		Placements: []PlacementResult{mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 1.5, YMM: 20})},
	}

	quality := BuildQualityReport(req, result)
	report := findFanoutReport(quality.FanoutReports, "J1")
	if report == nil || report.Status != "pass" || report.ConnectedPadCount != 12 {
		t.Fatalf("connector fanout report = %#v", report)
	}
}

func TestQualityReportFanoutFailsDenseComponentWithPoorEscape(t *testing.T) {
	req := fanoutRequest("U1", 20)
	result := Result{
		Status:     StatusPlaced,
		Placements: []PlacementResult{mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 1.5, YMM: 1.5})},
	}

	quality := BuildQualityReport(req, result)
	report := findFanoutReport(quality.FanoutReports, "U1")
	if report == nil || report.Status != "fail" || report.SuggestedAction == "" {
		t.Fatalf("dense fanout report = %#v", report)
	}
	var foundScore bool
	for _, dimension := range quality.Score.Dimensions {
		if dimension.Name == "fanout" {
			foundScore = true
			if dimension.Status != "fail" || dimension.Score != 0 {
				t.Fatalf("fanout score = %#v, want failing zero score", dimension)
			}
		}
	}
	if !foundScore {
		t.Fatalf("missing fanout score: %#v", quality.Score)
	}
}

func TestQualityReportFanoutWarnsForKeepoutPressure(t *testing.T) {
	req := fanoutRequest("U1", 10)
	req.Keepouts = []Keepout{{ID: "mount", Bounds: Rect{Min: Point{XMM: 12, YMM: 10}, Max: Point{XMM: 18, YMM: 18}}}}
	result := Result{
		Status:     StatusPlaced,
		Placements: []PlacementResult{mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 10, YMM: 12})},
	}

	quality := BuildQualityReport(req, result)
	report := findFanoutReport(quality.FanoutReports, "U1")
	if report == nil || report.Status != "warning" || report.KeepoutPressure == 0 {
		t.Fatalf("keepout fanout report = %#v", report)
	}
}

func TestQualityReportFanoutReportsAreSorted(t *testing.T) {
	req := fanoutRequest("U2", 8)
	second := fanoutComponent("U1", 8)
	req.Components = append(req.Components, second)
	req.Nets = append(req.Nets, fanoutNets("U1", 8)...)
	result := Result{
		Status: StatusPlaced,
		Placements: []PlacementResult{
			mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 20, YMM: 20}),
			mustPlacementResultForTest(t, req.Components[1], Placement{XMM: 30, YMM: 20}),
		},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.FanoutReports) != 2 || quality.FanoutReports[0].Ref != "U1" || quality.FanoutReports[1].Ref != "U2" {
		t.Fatalf("fanout reports not sorted: %#v", quality.FanoutReports)
	}
}

func TestQualityReportScoresOptionalRegionMissAsWarning(t *testing.T) {
	req := minimalRequest()
	req.RegionRules = []RegionRule{{
		ID:        "analog-soft",
		Region:    "analog",
		Refs:      []string{"R1"},
		Preferred: Rect{Min: Point{XMM: 0, YMM: 0}, Max: Point{XMM: 10, YMM: 10}},
	}}
	result := Result{
		Status:     StatusPlaced,
		Placements: []PlacementResult{mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 30, YMM: 20})},
	}

	quality := BuildQualityReport(req, result)
	for _, dimension := range quality.Score.Dimensions {
		if dimension.Name == "regions" {
			if dimension.Status != "warning" {
				t.Fatalf("region dimension = %#v, want warning", dimension)
			}
			return
		}
	}
	t.Fatalf("missing region score: %#v", quality.Score)
}

func TestQualityReportRegionRefsExpandFromNetRoles(t *testing.T) {
	req := twoComponentRequest()
	req.Nets[0].Role = NetAnalog
	req.RegionRules = []RegionRule{{
		ID:        "analog-net",
		Region:    "analog",
		NetRoles:  []NetRole{NetAnalog},
		Preferred: Rect{Min: Point{XMM: 0, YMM: 0}, Max: Point{XMM: 10, YMM: 10}},
	}}
	result := Result{
		Status: StatusPlaced,
		Placements: []PlacementResult{
			mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5}),
			mustPlacementResultForTest(t, req.Components[1], Placement{XMM: 6, YMM: 5}),
		},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.RegionReports) != 1 || quality.RegionReports[0].RequestedCount != 2 || !quality.RegionReports[0].Satisfied {
		t.Fatalf("unexpected net-role region report: %#v", quality.RegionReports)
	}
}

func TestQualityReportRegionWithNoRefsIsSatisfiedNoop(t *testing.T) {
	req := minimalRequest()
	req.RegionRules = []RegionRule{{
		ID:        "empty",
		Region:    "analog",
		Preferred: Rect{Min: Point{XMM: 0, YMM: 0}, Max: Point{XMM: 10, YMM: 10}},
	}}
	result := Result{
		Status:     StatusPlaced,
		Placements: []PlacementResult{mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5})},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.RegionReports) != 1 || !quality.RegionReports[0].Satisfied {
		t.Fatalf("empty region report should be satisfied no-op: %#v", quality.RegionReports)
	}
}

func TestQualityReportIncludesNetHPWL(t *testing.T) {
	req := twoComponentRequest()
	req.Nets[0].Role = NetSignal
	result := Result{
		Status: StatusPlaced,
		Placements: []PlacementResult{
			mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5}),
			mustPlacementResultForTest(t, req.Components[1], Placement{XMM: 8, YMM: 7}),
		},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.NetReports) != 1 {
		t.Fatalf("net reports = %#v", quality.NetReports)
	}
	report := quality.NetReports[0]
	if report.Name != "N1" || report.Role != string(NetSignal) || report.HPWLMM != 5 || report.Status != "pass" {
		t.Fatalf("unexpected net report: %#v", report)
	}
}

func TestQualityReportWarnsForLongNetHPWL(t *testing.T) {
	req := twoComponentRequest()
	req.Nets[0].Weight = 2
	result := Result{
		Status: StatusPlaced,
		Placements: []PlacementResult{
			mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 2, YMM: 2}),
			mustPlacementResultForTest(t, req.Components[1], Placement{XMM: 38, YMM: 23}),
		},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.NetReports) != 1 || quality.NetReports[0].Status != "warning" || quality.NetReports[0].WeightedHPWLMM <= quality.NetReports[0].HPWLMM {
		t.Fatalf("unexpected long-net report: %#v", quality.NetReports)
	}
	for _, dimension := range quality.Score.Dimensions {
		if dimension.Name == "routing_readiness" {
			if dimension.Status != "warning" || dimension.Score != 0.5 {
				t.Fatalf("routing dimension = %#v, want warning half score", dimension)
			}
			return
		}
	}
	t.Fatalf("missing routing readiness score: %#v", quality.Score)
}

func TestQualityReportFailsRoutingReadinessForMissingEndpoint(t *testing.T) {
	req := twoComponentRequest()
	result := Result{
		Status:     StatusPlaced,
		Placements: []PlacementResult{mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5})},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.NetReports) != 1 || quality.NetReports[0].Status != "fail" {
		t.Fatalf("unexpected missing-endpoint report: %#v", quality.NetReports)
	}
	for _, dimension := range quality.Score.Dimensions {
		if dimension.Name == "routing_readiness" {
			if dimension.Status != "fail" || dimension.Score != 0 {
				t.Fatalf("routing dimension = %#v, want fail zero score", dimension)
			}
			return
		}
	}
	t.Fatalf("missing routing readiness score: %#v", quality.Score)
}

func TestQualityReportCountsMultiplePinsOnSamePlacedComponent(t *testing.T) {
	req := minimalRequest()
	req.Nets[0].Endpoints = []Endpoint{{Ref: "R1", Pin: "1"}, {Ref: "R1", Pin: "2"}}
	result := Result{
		Status:     StatusPlaced,
		Placements: []PlacementResult{mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5})},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.NetReports) != 1 || quality.NetReports[0].PlacedEndpointCount != 2 || quality.NetReports[0].Status != "pass" {
		t.Fatalf("unexpected same-component net report: %#v", quality.NetReports)
	}
}

func TestQualityReportScoresMechanicalKeepouts(t *testing.T) {
	req := minimalRequest()
	req.Mechanical = []MechanicalConstraint{{
		ID:     "mounting-hole",
		Kind:   "mounting_hole",
		Bounds: Rect{Min: Point{XMM: 20, YMM: 20}, Max: Point{XMM: 25, YMM: 25}},
	}}
	result := Result{
		Status:     StatusPlaced,
		Placements: []PlacementResult{mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5})},
	}

	quality := BuildQualityReport(req, result)
	if quality.KeepoutCount != 1 {
		t.Fatalf("KeepoutCount = %d, want 1", quality.KeepoutCount)
	}
	var found bool
	for _, dimension := range quality.Score.Dimensions {
		if dimension.Name == "mechanical" {
			found = true
			if dimension.Status != "pass" {
				t.Fatalf("mechanical dimension = %#v, want pass", dimension)
			}
		}
	}
	if !found {
		t.Fatalf("missing mechanical score: %#v", quality.Score)
	}
}

func TestQualityReportWarnsForOptionalMechanicalOverlap(t *testing.T) {
	req := minimalRequest()
	req.Mechanical = []MechanicalConstraint{{
		ID:       "service-zone",
		Kind:     "service_area",
		Bounds:   Rect{Min: Point{XMM: 4, YMM: 4}, Max: Point{XMM: 8, YMM: 8}},
		Optional: true,
	}}
	result := Result{
		Status:     StatusPlaced,
		Placements: []PlacementResult{mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5})},
	}

	quality := BuildQualityReport(req, result)
	if quality.OptionalKeepoutViolations != 1 || quality.GeometryIssueCount != 0 {
		t.Fatalf("unexpected optional keepout accounting: %#v", quality)
	}
	var found bool
	for _, dimension := range quality.Score.Dimensions {
		if dimension.Name == "mechanical" {
			found = true
			if dimension.Status != "warning" || dimension.Score != 0.5 {
				t.Fatalf("mechanical dimension = %#v, want warning half score", dimension)
			}
		}
	}
	if !found {
		t.Fatalf("missing mechanical score: %#v", quality.Score)
	}
}

func TestQualityReportMechanicalScoreIgnoresComponentOverlap(t *testing.T) {
	req := twoComponentRequest()
	req.Keepouts = []Keepout{{
		ID:     "mounting",
		Bounds: Rect{Min: Point{XMM: 20, YMM: 20}, Max: Point{XMM: 25, YMM: 25}},
	}}
	result := Result{
		Status: StatusPlaced,
		Placements: []PlacementResult{
			mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5}),
			mustPlacementResultForTest(t, req.Components[1], Placement{XMM: 5, YMM: 5}),
		},
	}

	quality := BuildQualityReport(req, result)
	if quality.GeometryIssueCount == 0 || quality.RequiredKeepoutViolations != 0 {
		t.Fatalf("test setup should have only component geometry issues: %#v", quality)
	}
	for _, dimension := range quality.Score.Dimensions {
		if dimension.Name == "mechanical" && dimension.Status != "pass" {
			t.Fatalf("mechanical dimension = %#v, want pass for unrelated overlap", dimension)
		}
	}
}

func TestQualityReportMissingProximityTargetMarshalsJSON(t *testing.T) {
	req := minimalRequest()
	req.ProximityRules = []ProximityRule{{
		ID:         "missing",
		AnchorRef:  "R1",
		TargetRefs: []string{"C1"},
		Required:   true,
	}}
	result := Result{
		Status:     StatusPlaced,
		Placements: []PlacementResult{mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5})},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.ProximityReports) != 1 || quality.ProximityReports[0].ActualMM != nil {
		t.Fatalf("unexpected proximity report: %#v", quality.ProximityReports)
	}
	if _, err := json.Marshal(quality); err != nil {
		t.Fatalf("quality report should marshal without infinities: %v", err)
	}
}

func fanoutRequest(ref string, count int) Request {
	return Request{
		Board: BoardPlacementArea{WidthMM: 50, HeightMM: 40},
		Components: []Component{
			fanoutComponent(ref, count),
		},
		Nets: fanoutNets(ref, count),
		Rules: Rules{
			ComponentSpacingMM: 2,
		},
	}
}

func fanoutComponent(ref string, count int) Component {
	pads := make([]PadSummary, 0, count)
	for index := 1; index <= count; index++ {
		pads = append(pads, PadSummary{Name: fmt.Sprintf("%d", index)})
	}
	return Component{
		Ref:         ref,
		FootprintID: "Test:Dense",
		Bounds:      Bounds{WidthMM: 3, HeightMM: 3, AnchorOffset: Point{XMM: 1.5, YMM: 1.5}, Source: BoundsExplicit},
		Pads:        pads,
	}
}

func fanoutNets(ref string, count int) []Net {
	nets := make([]Net, 0, count)
	for index := 1; index <= count; index++ {
		pin := fmt.Sprintf("%d", index)
		nets = append(nets, Net{Name: "N" + pin, Endpoints: []Endpoint{{Ref: ref, Pin: pin}}})
	}
	return nets
}

func findFanoutReport(reports []FanoutReport, ref string) *FanoutReport {
	for index := range reports {
		if reports[index].Ref == ref {
			return &reports[index]
		}
	}
	return nil
}

func mustPlacementResultForTest(t *testing.T, component Component, position Placement) PlacementResult {
	t.Helper()
	result, ok := NewPlacementResult(component, position, DefaultRules())
	if !ok {
		t.Fatalf("failed to create placement result for %#v", component)
	}
	return result
}
