package routing

import (
	"context"
	"math"
	"reflect"
	"testing"

	"kicadai/internal/pcbrules"
	"kicadai/internal/reports"
)

func TestRouteRequestRoutesSimpleBoard(t *testing.T) {
	request := singleLayerSearchRequest()

	result := RouteRequest(request)
	if result.Status != StatusRouted {
		t.Fatalf("status = %s, issues = %#v", result.Status, result.Issues)
	}
	if result.Metrics.RoutedNetCount != 1 || result.Metrics.SegmentCount == 0 {
		t.Fatalf("metrics = %#v", result.Metrics)
	}
	if len(result.Routes) != 1 || len(result.Routes[0].Segments) == 0 {
		t.Fatalf("routes = %#v", result.Routes)
	}
}

func TestRouteRequestRejectsEndpointAccessBlockedByForeignPad(t *testing.T) {
	request := singleLayerSearchRequest()
	foreign := request.Components[0]
	foreign.Ref = "X1"
	foreign.Pads = append([]Pad(nil), foreign.Pads...)
	foreign.Pads[0].Ref = foreign.Ref
	foreign.Pads[0].Net = "OTHER"
	request.Components = append(request.Components, foreign)

	result := RouteRequest(request)
	if result.Status == StatusRouted || result.Metrics.FailedNetCount != 1 {
		t.Fatalf("status = %s metrics = %#v issues = %#v, want blocked foreign-pad endpoint access", result.Status, result.Metrics, result.Issues)
	}
}

func TestExpandSMDPadEdgeAccessComposesPadRotation(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Components = []Component{{
		Ref:      "U1",
		Position: Placement{XMM: 10, YMM: 10, Layer: "F.Cu"},
		Pads: []Pad{{
			Ref: "U1", Name: "1", Net: "SIG", Position: Point{},
			RotationDeg: 90, Shape: PadRoundedRect, Type: PadSMD,
			Size: Size{WidthMM: 0.25, HeightMM: 0.875}, Layers: []string{"F.Cu"},
		}},
	}}
	request.Rules.GridMM = 0.25
	request.Rules.TraceWidthMM = 0.25
	request.Rules.ClearanceMM = 0.2
	request.Rules.ViaDiameterMM = 0.6
	request.Rules.ViaClearanceMM = 0.2
	endpoint := Endpoint{Ref: "U1", Pin: "1"}

	expanded := expandSMDPadEdgeAccess(BuildPadAccess(request), request, []Endpoint{endpoint})
	points, ok := AccessPointsForEndpoint(expanded, endpoint)
	if !ok {
		t.Fatal("rotated SMD endpoint has no access")
	}
	minX, maxX := math.Inf(1), math.Inf(-1)
	minY, maxY := math.Inf(1), math.Inf(-1)
	searchPoints := 0
	for _, point := range points {
		if point.SearchPoint == nil {
			continue
		}
		searchPoints++
		minX = min(minX, point.SearchPoint.XMM)
		maxX = max(maxX, point.SearchPoint.XMM)
		minY = min(minY, point.SearchPoint.YMM)
		maxY = max(maxY, point.SearchPoint.YMM)
	}
	if searchPoints != 4 ||
		math.Abs(minX-9.5625) > 1e-9 || math.Abs(maxX-10.4375) > 1e-9 ||
		math.Abs(minY-9.875) > 1e-9 || math.Abs(maxY-10.125) > 1e-9 {
		t.Fatalf("rotated edge search bounds = x[%g,%g] y[%g,%g] across %d points", minX, maxX, minY, maxY, searchPoints)
	}
}

func TestCrowdedSMDPadViaAccessStaggersAndFitsAdjacentPitch(t *testing.T) {
	request := minimalRequest()
	request.Board.Layers = []Layer{
		{Name: "F.Cu", Kind: LayerCopper, Routable: true},
		{Name: "In1.Cu", Kind: LayerCopper, Routable: true},
		{Name: "In2.Cu", Kind: LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: LayerCopper, Routable: true},
	}
	request.Rules.GridMM = 0.25
	request.Rules.TraceWidthMM = 0.2
	request.Rules.ClearanceMM = 0.2
	request.Rules.ViaDiameterMM = 0.7
	request.Rules.ViaDrillMM = 0.35
	request.Rules.MaxViasPerNet = 4
	request.Components = []Component{{
		Ref: "U1", Position: Placement{XMM: 10, YMM: 10, Layer: "F.Cu"},
		Pads: []Pad{
			{Ref: "U1", Name: "1", Net: "A", Position: Point{XMM: 2, YMM: 0}, Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 1, HeightMM: 0.25}, Layers: []string{"F.Cu"}},
			{Ref: "U1", Name: "2", Net: "B", Position: Point{XMM: 2, YMM: 0.5}, Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 1, HeightMM: 0.25}, Layers: []string{"F.Cu"}},
		},
	}}
	endpoints := []Endpoint{{Ref: "U1", Pin: "1"}, {Ref: "U1", Pin: "2"}}

	adjusted, diameters := applyCrowdedSMDPadViaAccess(BuildPadAccess(request), request, componentsByNormalizedRef(request.Components), endpoints)
	if len(diameters) != 2 {
		t.Fatalf("forced endpoint vias = %#v, want both crowded pads", diameters)
	}
	first, firstOK := AccessPointsForEndpoint(adjusted, endpoints[0])
	second, secondOK := AccessPointsForEndpoint(adjusted, endpoints[1])
	if !firstOK || !secondOK || len(first) != 2 || len(second) != 2 || first[0].SearchPoint == nil || second[0].SearchPoint == nil {
		t.Fatalf("staggered access = first %#v second %#v", first, second)
	}
	if math.Abs(first[0].SearchPoint.XMM-second[0].SearchPoint.XMM) < 0.5 {
		t.Fatalf("dogbone columns were not staggered: first=%#v second=%#v", first[0], second[0])
	}
	attempts := crowdedSMDPadViaAccessAttempts(adjusted, diameters)
	if len(attempts) != 4 {
		t.Fatalf("crowded access attempts = %d, want preferred plus three bounded alternate combinations", len(attempts))
	}
	for attemptIndex, attempt := range attempts {
		for _, endpoint := range endpoints {
			points, ok := AccessPointsForEndpoint(attempt, endpoint)
			if !ok || len(points) != 1 {
				t.Fatalf("attempt %d endpoint %#v access = %#v", attemptIndex, endpoint, points)
			}
		}
	}
	preferredFirst, _ := AccessPointsForEndpoint(attempts[0], endpoints[0])
	preferredSecond, _ := AccessPointsForEndpoint(attempts[0], endpoints[1])
	if *preferredFirst[0].SearchPoint != *first[0].SearchPoint || *preferredSecond[0].SearchPoint != *second[0].SearchPoint {
		t.Fatalf("first attempt did not retain deterministic stagger: %#v", attempts[0])
	}
	rules := crowdedEndpointViaRules(request.Rules, diameters)
	if math.Abs(rules.ViaDiameterMM-0.4) > 1e-9 || math.Abs(rules.ViaDrillMM-0.2) > 1e-9 {
		t.Fatalf("crowded endpoint via rules = %#v, want trace-derived 0.4/0.2mm after physical clearance proof", rules)
	}
}

func TestTwoTerminalSMDPadViaAccessEscapesAwayFromOppositePad(t *testing.T) {
	request := minimalRequest()
	request.Board.Layers = []Layer{
		{Name: "F.Cu", Kind: LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: LayerCopper, Routable: true},
	}
	request.Rules.GridMM = 0.25
	request.Rules.TraceWidthMM = 0.2
	request.Rules.ViaDiameterMM = 0.7
	request.Rules.ViaClearanceMM = 0.2
	component := Component{
		Ref: "R1", Position: Placement{XMM: 10, YMM: 10, Layer: "F.Cu"},
		Pads: []Pad{
			{Name: "1", Net: "A", Position: Point{XMM: -0.825}, Type: PadSMD, Size: Size{WidthMM: 0.9, HeightMM: 0.95}, Layers: []string{"F.Cu"}},
			{Name: "2", Net: "B", Position: Point{XMM: 0.825}, Type: PadSMD, Size: Size{WidthMM: 0.9, HeightMM: 0.95}, Layers: []string{"F.Cu"}},
			{Name: "3", Net: "A", Position: Point{XMM: -0.825, YMM: 0.4}, Type: PadSMD, Size: Size{WidthMM: 0.4, HeightMM: 0.4}, Layers: []string{"F.Cu"}},
		},
	}
	request.Components = []Component{component}
	adjusted, forced := applyTwoTerminalSMDPadViaAccess(
		BuildPadAccess(request), request, componentsByNormalizedRef(request.Components), []Endpoint{{Ref: "R1", Pin: "2"}},
	)
	points, ok := AccessPointsForEndpoint(adjusted, Endpoint{Ref: "R1", Pin: "2"})
	if !ok || len(points) != 1 || points[0].SearchPoint == nil || len(forced) != 1 {
		t.Fatalf("two-terminal dogbone access = points %#v forced %#v", points, forced)
	}
	center := absolutePadPoint(component, component.Pads[1].Position)
	if points[0].Point.XMM <= center.XMM || points[0].SearchPoint.XMM <= points[0].Point.XMM {
		t.Fatalf("dogbone did not escape outward from opposite pad: center=%#v access=%#v", center, points[0])
	}
}

func TestCrowdedSMDPadViaAccessSupportsTwoLayerDogbone(t *testing.T) {
	request := minimalRequest()
	request.Board.Layers = []Layer{
		{Name: "F.Cu", Kind: LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: LayerCopper, Routable: true},
	}
	request.Rules.GridMM = 0.25
	request.Rules.TraceWidthMM = 0.2
	request.Rules.ClearanceMM = 0.2
	request.Rules.ViaDiameterMM = 0.7
	request.Rules.ViaDrillMM = 0.35
	request.Rules.MaxViasPerNet = 2
	request.Components = []Component{{
		Ref: "U1", Position: Placement{XMM: 10, YMM: 10, Layer: "F.Cu"},
		Pads: []Pad{
			{Ref: "U1", Name: "1", Net: "A", Position: Point{XMM: 2, YMM: 0}, Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 1, HeightMM: 0.25}, Layers: []string{"F.Cu"}},
			{Ref: "U1", Name: "2", Net: "B", Position: Point{XMM: 2, YMM: 0.5}, Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 1, HeightMM: 0.25}, Layers: []string{"F.Cu"}},
		},
	}}
	endpoint := Endpoint{Ref: "U1", Pin: "1"}

	adjusted, diameters := applyCrowdedSMDPadViaAccess(BuildPadAccess(request), request, componentsByNormalizedRef(request.Components), []Endpoint{endpoint})
	points, ok := AccessPointsForEndpoint(adjusted, endpoint)
	if len(diameters) != 1 || !ok || len(points) != 2 || points[0].SearchPoint == nil || points[1].SearchPoint == nil {
		t.Fatalf("two-layer crowded dogbone = points %#v diameters %#v", points, diameters)
	}
}

func TestCrowdedSMDPadViaAccessDoesNotForceOrdinaryPitchPowerPad(t *testing.T) {
	request := minimalRequest()
	request.Board.Layers = []Layer{
		{Name: "F.Cu", Kind: LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: LayerCopper, Routable: true},
	}
	request.Rules.GridMM = 0.25
	request.Rules.TraceWidthMM = 0.8
	request.Rules.ClearanceMM = 0.2
	request.Rules.ViaDiameterMM = 0.7
	request.Rules.ViaDrillMM = 0.35
	request.Rules.MaxViasPerNet = 2
	request.Components = []Component{{
		Ref: "U1", Position: Placement{XMM: 10, YMM: 10, Layer: "F.Cu"},
		Pads: []Pad{
			{Ref: "U1", Name: "1", Net: "POWER", Position: Point{XMM: 2, YMM: 0}, Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 1, HeightMM: 0.6}, Layers: []string{"F.Cu"}},
			{Ref: "U1", Name: "2", Net: "OTHER", Position: Point{XMM: 2, YMM: 0.95}, Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 1, HeightMM: 0.6}, Layers: []string{"F.Cu"}},
		},
	}}
	endpoint := Endpoint{Ref: "U1", Pin: "1"}
	access := BuildPadAccess(request)

	adjusted, diameters := applyCrowdedSMDPadViaAccess(access, request, componentsByNormalizedRef(request.Components), []Endpoint{endpoint})
	if len(diameters) != 0 {
		t.Fatalf("forced endpoint vias = %#v, want ordinary-pitch pad to retain normal access", diameters)
	}
	got, gotOK := AccessPointsForEndpoint(adjusted, endpoint)
	want, wantOK := AccessPointsForEndpoint(access, endpoint)
	if !gotOK || !wantOK || !reflect.DeepEqual(got, want) {
		t.Fatalf("ordinary-pitch access = %#v, want unchanged %#v", got, want)
	}
}

func TestPrioritizeCrowdedSMDPadPairsPreservesOtherBranchOrder(t *testing.T) {
	request := minimalRequest()
	request.Board.Layers = []Layer{
		{Name: "F.Cu", Kind: LayerCopper, Routable: true},
		{Name: "In1.Cu", Kind: LayerCopper, Routable: true},
		{Name: "In2.Cu", Kind: LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: LayerCopper, Routable: true},
	}
	request.Rules.GridMM = 0.25
	request.Rules.TraceWidthMM = 0.2
	request.Rules.ClearanceMM = 0.2
	request.Rules.ViaDiameterMM = 0.7
	request.Rules.MaxViasPerNet = 4
	request.Components = []Component{{
		Ref: "U1",
		Pads: []Pad{
			{Ref: "U1", Name: "1", Net: "SIG", Position: Point{XMM: 2, YMM: 0}, Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 1, HeightMM: 0.25}, Layers: []string{"F.Cu"}},
			{Ref: "U1", Name: "2", Net: "OTHER", Position: Point{XMM: 2, YMM: 0.5}, Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 1, HeightMM: 0.25}, Layers: []string{"F.Cu"}},
		},
	}}
	ordinaryFirst := EndpointPair{From: Endpoint{Ref: "R1", Pin: "1"}, To: Endpoint{Ref: "R2", Pin: "1"}}
	crowded := EndpointPair{From: Endpoint{Ref: "R2", Pin: "1"}, To: Endpoint{Ref: "U1", Pin: "1"}}
	ordinaryLast := EndpointPair{From: Endpoint{Ref: "R2", Pin: "1"}, To: Endpoint{Ref: "C1", Pin: "1"}}

	got := prioritizeCrowdedSMDPadPairs([]EndpointPair{ordinaryFirst, crowded, ordinaryLast}, request)
	want := []EndpointPair{crowded, ordinaryFirst, ordinaryLast}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prioritized pairs = %#v, want %#v", got, want)
	}
}

func TestRouteRequestTransitionsCrowdedSMDPadAtPitchSafeDogbone(t *testing.T) {
	request := minimalRequest()
	request.Board.WidthMM = 30
	request.Board.HeightMM = 20
	request.Board.Layers = []Layer{
		{Name: "F.Cu", Kind: LayerCopper, Routable: true},
		{Name: "In1.Cu", Kind: LayerCopper, Routable: true},
		{Name: "In2.Cu", Kind: LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: LayerCopper, Routable: true},
	}
	request.Rules.GridMM = 0.25
	request.Rules.TraceWidthMM = 0.2
	request.Rules.ClearanceMM = 0.2
	request.Rules.ViaDiameterMM = 0.7
	request.Rules.ViaDrillMM = 0.35
	request.Rules.MaxSearchNodes = 100000
	request.Rules.MaxViasPerNet = 4
	request.Strategy.Mode = ModeTwoLayer
	request.Strategy.NetOrder = NetOrderConstrainedEndpointAccessV1
	request.Components = []Component{
		{
			Ref: "U1", Position: Placement{XMM: 5, YMM: 10, Layer: "F.Cu"},
			Pads: []Pad{
				{Ref: "U1", Name: "1", Net: "SIG", Position: Point{XMM: 2, YMM: 0}, Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 1, HeightMM: 0.25}, Layers: []string{"F.Cu"}},
				{Ref: "U1", Name: "2", Net: "OTHER", Position: Point{XMM: 2, YMM: 0.5}, Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 1, HeightMM: 0.25}, Layers: []string{"F.Cu"}},
			},
		},
		{
			Ref: "J1", Position: Placement{XMM: 25, YMM: 10, Layer: "F.Cu"},
			Pads: []Pad{{Ref: "J1", Name: "1", Net: "SIG", Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 1, HeightMM: 1}, Layers: []string{"F.Cu"}}},
		},
	}
	request.Nets = []Net{{Name: "SIG", Role: NetSignal, Endpoints: []Endpoint{{Ref: "U1", Pin: "1"}, {Ref: "J1", Pin: "1"}}}}

	result := RouteRequest(request)
	if result.Status != StatusRouted || len(result.Routes) != 1 {
		t.Fatalf("crowded dogbone route = %#v", result)
	}
	foundPitchSafeVia := false
	for _, via := range result.Routes[0].Vias {
		if math.Abs(via.DiameterMM-0.4) <= 1e-9 && math.Abs(via.DrillMM-0.2) <= 1e-9 {
			foundPitchSafeVia = true
		}
	}
	if !foundPitchSafeVia {
		t.Fatalf("vias = %#v, want pitch-derived crowded endpoint transition", result.Routes[0].Vias)
	}
}

func TestRouteRequestUsesAlternateCrowdedSMDPadDogboneWhenPreferredColumnIsBlocked(t *testing.T) {
	request := minimalRequest()
	request.Board.WidthMM = 30
	request.Board.HeightMM = 20
	request.Board.Layers = []Layer{
		{Name: "F.Cu", Kind: LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: LayerCopper, Routable: true},
	}
	request.Rules.GridMM = 0.25
	request.Rules.TraceWidthMM = 0.2
	request.Rules.ClearanceMM = 0.2
	request.Rules.ViaDiameterMM = 0.7
	request.Rules.ViaDrillMM = 0.35
	request.Rules.MaxSearchNodes = 100000
	request.Rules.MaxViasPerNet = 4
	request.Strategy.Mode = ModeTwoLayer
	request.Strategy.NetOrder = NetOrderConstrainedEndpointAccessV1
	request.Components = []Component{
		{
			Ref: "U1", Position: Placement{XMM: 5, YMM: 10, Layer: "F.Cu"},
			Pads: []Pad{
				{Ref: "U1", Name: "1", Net: "OTHER", Position: Point{XMM: 2, YMM: 0}, Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 1, HeightMM: 0.25}, Layers: []string{"F.Cu"}},
				{Ref: "U1", Name: "2", Net: "SIG", Position: Point{XMM: 2, YMM: 0.5}, Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 1, HeightMM: 0.25}, Layers: []string{"F.Cu"}},
			},
		},
		{
			Ref: "X1", Position: Placement{XMM: 8.5, YMM: 10.5, Layer: "F.Cu"},
			Pads: []Pad{{Ref: "X1", Name: "1", Net: "BLOCK", Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 0.2, HeightMM: 0.2}, Layers: []string{"F.Cu"}}},
		},
		{
			Ref: "J1", Position: Placement{XMM: 25, YMM: 10.5, Layer: "F.Cu"},
			Pads: []Pad{{Ref: "J1", Name: "1", Net: "SIG", Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 1, HeightMM: 1}, Layers: []string{"F.Cu"}}},
		},
	}
	request.Nets = []Net{{Name: "SIG", Role: NetSignal, Endpoints: []Endpoint{{Ref: "U1", Pin: "2"}, {Ref: "J1", Pin: "1"}}}}

	result := RouteRequest(request)
	if result.Status != StatusRouted || len(result.Routes) != 1 {
		t.Fatalf("alternate crowded dogbone route = %#v", result)
	}
	foundAlternateVia := false
	for _, via := range result.Routes[0].Vias {
		if math.Abs(via.At.XMM-8.0) <= 1e-9 && math.Abs(via.At.YMM-10.5) <= 1e-9 {
			foundAlternateVia = true
		}
	}
	if !foundAlternateVia {
		t.Fatalf("vias = %#v, want alternate pitch-safe dogbone at (8.0, 10.5)", result.Routes[0].Vias)
	}
}

func TestRouteRequestReusesDuplicatePadAccessAcrossNetBranches(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Components = []Component{
		{
			Ref:      "J1",
			Position: Placement{Layer: "F.Cu"},
			Pads: []Pad{
				duplicateAccessTestPad("J1", "SH", Point{XMM: 15, YMM: 5}),
				duplicateAccessTestPad("J1", "SH", Point{XMM: 15, YMM: 15}),
			},
		},
		{
			Ref:      "J2",
			Position: Placement{Layer: "F.Cu"},
			Pads:     []Pad{duplicateAccessTestPad("J2", "1", Point{XMM: 25, YMM: 5})},
		},
		{
			Ref:      "J3",
			Position: Placement{Layer: "F.Cu"},
			Pads:     []Pad{duplicateAccessTestPad("J3", "1", Point{XMM: 5, YMM: 15})},
		},
	}
	request.Nets = []Net{{
		Name: "SIG",
		Endpoints: []Endpoint{
			{Ref: "J1", Pin: "SH"},
			{Ref: "J2", Pin: "1"},
			{Ref: "J3", Pin: "1"},
		},
	}}

	result := RouteRequest(request)
	if result.Status != StatusRouted {
		t.Fatalf("status = %s, issues = %#v, routes = %#v", result.Status, result.Issues, result.Routes)
	}
	if len(result.Routes) != 1 || len(result.Routes[0].Segments) == 0 {
		t.Fatalf("routes = %#v, want one routed tree", result.Routes)
	}
	graph := newRouteConnectivity(result.Routes[0])
	root, rootOK := graph.nearestKey(Point{XMM: 15, YMM: 5}, "F.Cu")
	second, secondOK := graph.nearestKey(Point{XMM: 25, YMM: 5}, "F.Cu")
	third, thirdOK := graph.nearestKey(Point{XMM: 5, YMM: 15}, "F.Cu")
	if !rootOK || !secondOK || !thirdOK || graph.find(root) != graph.find(second) || graph.find(root) != graph.find(third) {
		t.Fatalf("duplicate-pad route tree is disconnected: root=%v second=%v third=%v route=%#v", rootOK, secondOK, thirdOK, result.Routes[0])
	}
}

func duplicateAccessTestPad(ref, name string, point Point) Pad {
	return Pad{
		Ref:      ref,
		Name:     name,
		Net:      "SIG",
		Position: point,
		Shape:    PadRect,
		Type:     PadSMD,
		Size:     Size{WidthMM: 1, HeightMM: 1},
		Layers:   []string{"F.Cu"},
	}
}

func TestEndpointNeckdownTrunkIssueIdentifiesPair(t *testing.T) {
	issue := endpointNeckdownTrunkIssue("GND", 3, EndpointPair{
		From: Endpoint{Ref: "U2", Pin: "7"},
		To:   Endpoint{Ref: "R5", Pin: "2"},
	})
	if issue.Path != `nets["GND"].pairs[3]` || issue.Message != "endpoint neckdown path between U2.7 and R5.2 does not leave a clearance-safe full-width trunk" {
		t.Fatalf("issue = %#v", issue)
	}
	if !reflect.DeepEqual(issue.Refs, []string{"U2", "R5"}) || !reflect.DeepEqual(issue.Nets, []string{"GND"}) {
		t.Fatalf("issue identity = %#v", issue)
	}
}

func TestRouteRequestContextHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := RouteRequestContext(ctx, singleLayerSearchRequest())
	if result.Status != StatusBlocked {
		t.Fatalf("status = %s, want blocked", result.Status)
	}
	if len(result.Issues) != 1 || result.Issues[0].Code != reports.CodeOperationCanceled {
		t.Fatalf("issues = %#v", result.Issues)
	}
}

func TestRouteRequestLaterNetAvoidsEarlierCopper(t *testing.T) {
	request := crossingNetsRequest()

	result := RouteRequest(request)
	if result.Status != StatusRouted {
		t.Fatalf("status = %s, issues = %#v", result.Status, result.Issues)
	}
	if len(result.Routes) != 2 {
		t.Fatalf("routes = %#v, want two", result.Routes)
	}
	var vertical Route
	for _, route := range result.Routes {
		if route.Net == "B_VERTICAL" {
			vertical = route
		}
	}
	for _, segment := range vertical.Segments {
		if segmentContainsPoint(segment, Point{XMM: 12, YMM: 10}) {
			t.Fatalf("vertical net crosses first route at %#v in segments %#v", Point{XMM: 12, YMM: 10}, vertical.Segments)
		}
	}
}

func TestRouteRequestBlockedWhenPartialDisallowed(t *testing.T) {
	request := partialRoutingRequest()
	request.Strategy.AllowPartial = false

	result := RouteRequest(request)
	if result.Status != StatusBlocked {
		t.Fatalf("status = %s, want blocked; issues = %#v", result.Status, result.Issues)
	}
	if result.Metrics.FailedNetCount != 1 {
		t.Fatalf("metrics = %#v, want one failed net", result.Metrics)
	}
}

func TestRouteRequestPartialWhenAllowed(t *testing.T) {
	request := partialRoutingRequest()
	request.Strategy.AllowPartial = true

	result := RouteRequest(request)
	if result.Status != StatusPartial {
		t.Fatalf("status = %s, want partial; issues = %#v", result.Status, result.Issues)
	}
	if result.Metrics.RoutedNetCount != 1 || result.Metrics.FailedNetCount != 1 {
		t.Fatalf("metrics = %#v, want one routed and one failed", result.Metrics)
	}
}

func TestRouteRequestFailedNetDoesNotOccupyCopperForLaterNets(t *testing.T) {
	request := failedBranchOccupancyRequest()

	result := RouteRequest(request)
	if result.Status != StatusPartial {
		t.Fatalf("status = %s, want partial; issues = %#v", result.Status, result.Issues)
	}
	var failedRoute, laterRoute Route
	for _, route := range result.Routes {
		switch route.Net {
		case "A_FAIL":
			failedRoute = route
		case "B_LATER":
			laterRoute = route
		}
	}
	if failedRoute.Status != RouteStatusFailed || len(failedRoute.Segments) == 0 {
		t.Fatalf("failed route = %#v, want a discarded successful branch", failedRoute)
	}
	if laterRoute.Status != RouteStatusRouted || len(laterRoute.Segments) == 0 {
		t.Fatalf("later route = %#v, want routed after failed branch rollback", laterRoute)
	}
	if result.Metrics.SegmentCount != len(laterRoute.Segments) || result.Metrics.ViaCount != len(laterRoute.Vias) {
		t.Fatalf("committed metrics = %#v, want only later route geometry", result.Metrics)
	}
}

func TestRouteRequestAppliesNetClassTraceAndViaRules(t *testing.T) {
	request := twoLayerViaRequest()
	request.Rules.NetClasses = map[string]NetClass{
		"WIDE": {
			TraceWidthMM:  0.45,
			ViaDiameterMM: 0.8,
			ViaDrillMM:    0.35,
			MaxViasPerNet: 1,
		},
	}
	request.Nets[0].Class = "WIDE"

	result := RouteRequest(request)
	if result.Status != StatusRouted {
		t.Fatalf("status = %s issues=%#v", result.Status, result.Issues)
	}
	if len(result.Routes) != 1 || len(result.Routes[0].Segments) == 0 || len(result.Routes[0].Vias) == 0 {
		t.Fatalf("expected routed segments and via: %#v", result.Routes)
	}
	if result.Routes[0].Segments[0].WidthMM != 0.45 {
		t.Fatalf("segment width = %v, want net class width", result.Routes[0].Segments[0].WidthMM)
	}
	if result.Routes[0].Vias[0].DiameterMM != 0.8 || result.Routes[0].Vias[0].DrillMM != 0.35 {
		t.Fatalf("via geometry = %#v", result.Routes[0].Vias[0])
	}
}

func TestRouteRequestKeepsViasClearOfNoNetPads(t *testing.T) {
	request := twoLayerViaRequest()
	request.Rules.GridMM = 0.25
	request.Rules.TraceWidthMM = 0.25
	request.Rules.ClearanceMM = 0.2
	request.Rules.ViaDiameterMM = 0.6
	request.Rules.ViaDrillMM = 0.3
	request.Components = append(request.Components, Component{
		Ref:      "U1",
		Position: Placement{XMM: 15, YMM: 10, Layer: "F.Cu"},
		Pads: []Pad{{
			Ref:      "U1",
			Name:     "NC",
			Position: Point{},
			Shape:    PadRect,
			Type:     PadSMD,
			Size:     Size{WidthMM: 0.7, HeightMM: 0.9},
			Layers:   []string{"F.Cu"},
		}},
	})

	result := RouteRequest(request)
	if result.Status != StatusRouted {
		t.Fatalf("status = %s issues=%#v routes=%#v", result.Status, result.Issues, result.Routes)
	}
	if len(result.Routes) != 1 || len(result.Routes[0].Vias) == 0 {
		t.Fatalf("routes = %#v, want a routed via", result.Routes)
	}
	noNetPad := Rect{
		Min: Point{XMM: 15 - 0.35, YMM: 10 - 0.45},
		Max: Point{XMM: 15 + 0.35, YMM: 10 + 0.45},
	}
	requiredClearance := request.Rules.ViaDiameterMM/2 + request.Rules.ClearanceMM
	for _, via := range result.Routes[0].Vias {
		if distancePointToRect(via.At, noNetPad) < requiredClearance-1e-9 {
			t.Fatalf("via at %#v is too close to no-net pad %#v; want clearance >= %.3f", via.At, noNetPad, requiredClearance)
		}
	}
}

func TestRoutableViaSpanChecksIntermediateLayers(t *testing.T) {
	request := twoLayerViaRequest()
	request.Board.Layers = []Layer{
		{Name: "F.Cu", Kind: LayerCopper, Routable: true},
		{Name: "In1.Cu", Kind: LayerCopper},
		{Name: "B.Cu", Kind: LayerCopper, Routable: true},
	}
	request.Rules.GridMM = 0.25
	request.Rules.ViaDiameterMM = 0.6
	request.Rules.ClearanceMM = 0.2
	request.Obstacles = []Obstacle{{
		Kind:     ObstacleKeepout,
		Layer:    "In1.Cu",
		Geometry: Shape{Rect: &Rect{Min: Point{XMM: 9.75, YMM: 9.75}, Max: Point{XMM: 10.25, YMM: 10.25}}},
	}}
	viaOccupancy, err := BuildViaOccupancy(request, "SIG")
	if err != nil {
		t.Fatalf("BuildViaOccupancy error: %v", err)
	}
	layerIndexes, err := LayerIndexes(request.Board.Layers)
	if err != nil {
		t.Fatalf("LayerIndexes error: %v", err)
	}
	coord := viaOccupancy.Grid.ToGrid(Point{XMM: 10, YMM: 10}, layerIndexes[normalizeLayer("F.Cu")])
	target := coord
	target.Layer = layerIndexes[normalizeLayer("B.Cu")]

	if routableViaSpan(viaOccupancy, coord, target) {
		t.Fatal("via span should be blocked by intermediate-layer keepout")
	}
}

func TestRoutableViaSpanChecksFullPhysicalThroughViaSpan(t *testing.T) {
	request := twoLayerViaRequest()
	request.Board.Layers = []Layer{
		{Name: "F.Cu", Kind: LayerCopper, Routable: true},
		{Name: "In1.Cu", Kind: LayerCopper, Routable: true},
		{Name: "In2.Cu", Kind: LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: LayerCopper, Routable: true},
	}
	request.Rules.GridMM = 0.25
	request.Rules.ViaDiameterMM = 0.6
	request.Rules.ClearanceMM = 0.2
	request.Obstacles = []Obstacle{{
		Kind:     ObstacleKeepout,
		Layer:    "F.Cu",
		Geometry: Shape{Rect: &Rect{Min: Point{XMM: 9.75, YMM: 9.75}, Max: Point{XMM: 10.25, YMM: 10.25}}},
	}}
	viaOccupancy, err := BuildViaOccupancy(request, "SIG")
	if err != nil {
		t.Fatalf("BuildViaOccupancy error: %v", err)
	}
	layerIndexes, err := LayerIndexes(request.Board.Layers)
	if err != nil {
		t.Fatalf("LayerIndexes error: %v", err)
	}
	from := viaOccupancy.Grid.ToGrid(Point{XMM: 10, YMM: 10}, layerIndexes[normalizeLayer("In1.Cu")])
	to := from
	to.Layer = layerIndexes[normalizeLayer("In2.Cu")]

	if routableViaSpan(viaOccupancy, from, to) {
		t.Fatal("inner-layer transition should be blocked by outer-layer copper crossed by the physical through via")
	}
}

func TestRouteRequestAllowedLayersCanBlockRoute(t *testing.T) {
	request := twoLayerViaRequest()
	request.Rules.NetClasses = map[string]NetClass{
		"TOP_ONLY": {AllowedLayers: []string{"F.Cu"}},
	}
	request.Nets[0].Class = "TOP_ONLY"

	result := RouteRequest(request)
	if result.Status != StatusBlocked {
		t.Fatalf("status = %s, want blocked", result.Status)
	}
	if result.Quality == nil || result.Quality.NetReports[0].Status != RouteStatusFailed {
		t.Fatalf("expected failed quality report: %#v", result.Quality)
	}
}

func distancePointToRect(point Point, rect Rect) float64 {
	dx := max(max(rect.Min.XMM-point.XMM, 0), point.XMM-rect.Max.XMM)
	dy := max(max(rect.Min.YMM-point.YMM, 0), point.YMM-rect.Max.YMM)
	return math.Hypot(dx, dy)
}

func TestRouteQualityReportsSameNetMergeCandidates(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Existing = []ExistingCopper{
		{
			Kind:     CopperSegment,
			Net:      "SIG",
			Layer:    "F.Cu",
			Geometry: Shape{Rect: &Rect{Min: Point{XMM: 10, YMM: 9}, Max: Point{XMM: 11, YMM: 10}}},
		},
		{
			Kind:     CopperSegment,
			Net:      "OTHER",
			Layer:    "F.Cu",
			Geometry: Shape{Rect: &Rect{Min: Point{XMM: 18, YMM: 1}, Max: Point{XMM: 19, YMM: 2}}},
		},
	}

	result := RouteRequest(request)
	if result.Status != StatusRouted {
		t.Fatalf("status = %s, issues = %#v", result.Status, result.Issues)
	}
	if result.Quality == nil || len(result.Quality.NetReports) != 1 {
		t.Fatalf("quality = %#v", result.Quality)
	}
	report := result.Quality.NetReports[0]
	if report.SameNetPads != 2 || report.SameNetCopper != 1 {
		t.Fatalf("same-net evidence = pads %d copper %d, want 2/1", report.SameNetPads, report.SameNetCopper)
	}
}

func TestRouteRequestLengthWarningDoesNotFailRoute(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Rules.NetOverrides = map[string]NetRule{
		"SIG": {WarningLengthMM: 1},
	}

	result := RouteRequest(request)
	if result.Status != StatusRouted {
		t.Fatalf("status = %s issues=%#v", result.Status, result.Issues)
	}
	if len(result.Routes[0].Issues) == 0 {
		t.Fatalf("expected length warning")
	}
	if result.Routes[0].Status != RouteStatusRouted {
		t.Fatalf("route status = %s", result.Routes[0].Status)
	}
}

func TestRouteRequestMaxLengthFailsRoute(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Rules.NetOverrides = map[string]NetRule{
		"SIG": {MaxLengthMM: 1},
	}

	result := RouteRequest(request)
	if result.Status != StatusBlocked {
		t.Fatalf("status = %s, want blocked", result.Status)
	}
	if result.Routes[0].Status != RouteStatusFailed {
		t.Fatalf("route status = %s", result.Routes[0].Status)
	}
}

func TestExistingCopperForSegmentsIncludesTraceWidth(t *testing.T) {
	existing := existingCopperForSegments([]Segment{{
		Net:     "SIG",
		Layer:   "F.CU",
		Start:   Point{XMM: 1, YMM: 2},
		End:     Point{XMM: 5, YMM: 2},
		WidthMM: 0.4,
	}})
	if len(existing) != 1 || existing[0].Geometry.Rect == nil {
		t.Fatalf("existing = %#v", existing)
	}
	rect := *existing[0].Geometry.Rect
	if rect.Min != (Point{XMM: 0.8, YMM: 1.8}) || rect.Max != (Point{XMM: 5.2, YMM: 2.2}) {
		t.Fatalf("rect = %#v, want trace-width-expanded bounds", rect)
	}
	if len(existing[0].Geometry.Polygon) != 4 {
		t.Fatalf("polygon = %#v, want oriented trace body", existing[0].Geometry.Polygon)
	}
}

func TestExistingCopperForViasCoversPhysicalThroughViaSpan(t *testing.T) {
	existing := existingCopperForVias([]Via{{
		Net: "SIG", At: Point{XMM: 4, YMM: 5}, DiameterMM: 0.6, DrillMM: 0.3, Layers: []string{"F.Cu", "In1.Cu"},
	}}, []Layer{
		{Name: "F.Cu", Kind: LayerCopper},
		{Name: "In1.Cu", Kind: LayerCopper},
		{Name: "In2.Cu", Kind: LayerCopper},
		{Name: "B.Cu", Kind: LayerCopper},
		{Name: "F.SilkS", Kind: LayerOther},
	})
	if len(existing) != 4 {
		t.Fatalf("existing via layers = %#v, want all four physical copper layers", existing)
	}
	for index, want := range []string{"F.Cu", "In1.Cu", "In2.Cu", "B.Cu"} {
		if existing[index].Layer != want || existing[index].Kind != CopperVia {
			t.Fatalf("existing[%d] = %#v, want via on %s", index, existing[index], want)
		}
	}
}

func TestNominalSegmentsClearOccupancyRejectsThickenedCollision(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Rules.TraceWidthMM = 0.8
	request.Obstacles = append(request.Obstacles, Obstacle{
		Layer:    "F.Cu",
		Geometry: Shape{Rect: &Rect{Min: Point{XMM: 9, YMM: 9}, Max: Point{XMM: 11, YMM: 11}}},
	})
	occupancy, err := BuildOccupancy(request, "SIG")
	if err != nil {
		t.Fatalf("build occupancy: %v", err)
	}
	segments := []Segment{
		{Net: "SIG", Layer: "F.Cu", Start: Point{XMM: 2, YMM: 10}, End: Point{XMM: 5, YMM: 10}, WidthMM: 0.2},
		{Net: "SIG", Layer: "F.Cu", Start: Point{XMM: 5, YMM: 10}, End: Point{XMM: 15, YMM: 10}, WidthMM: 0.8},
	}
	if nominalSegmentsClearOccupancy(segments, 0.8, occupancy, request.Board.Layers) {
		t.Fatal("thickened segment crossing an obstacle was accepted")
	}
}

func TestEndpointNeckdownCanAwaitLaterNetTrunk(t *testing.T) {
	request := singleLayerSearchRequest()
	occupancy, err := BuildOccupancy(request, "SIG")
	if err != nil {
		t.Fatal(err)
	}
	path := GridPath{Net: "SIG", Layer: "F.Cu", Points: []Point{{XMM: 2, YMM: 10}, {XMM: 6, YMM: 10}}}
	segments, _, ok := endpointNeckdownAwaitingNetTrunk(path, .8, .2, occupancy, request.Board.Layers)
	if !ok || len(segments) == 0 {
		t.Fatalf("provisional neckdown = %#v, ok=%t", segments, ok)
	}
	if segmentsContainNominalWidth(segments, .8) {
		t.Fatalf("provisional branch unexpectedly contains a full-width trunk: %#v", segments)
	}
}

func TestAutomaticEndpointNeckdownAppliesToWideCurrentCarryingNets(t *testing.T) {
	for _, role := range []NetRole{NetPower, NetGround, NetHighCurrent} {
		rules := applyAutomaticEndpointNeckdown(Rules{TraceWidthMM: 0.5, MinNeckdownWidthMM: 0.25}, role, true)
		if rules.NeckdownWidthMM != 0.25 || rules.NeckdownLengthMM != pcbrules.DefaultPowerNeckdownLengthMM {
			t.Fatalf("role %s rules = %#v", role, rules)
		}
	}
}

func TestCrowdedPowerPadTriggersAutomaticEndpointNeckdownAtNominalWidth(t *testing.T) {
	request := minimalRequest()
	request.Board.Layers = []Layer{
		{Name: "F.Cu", Kind: LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: LayerCopper, Routable: true},
	}
	request.Rules.GridMM = 0.25
	request.Rules.TraceWidthMM = 0.3
	request.Rules.ClearanceMM = 0.2
	request.Rules.ViaDiameterMM = 0.7
	request.Rules.MaxViasPerNet = 2
	request.Components = []Component{{
		Ref: "U1", Position: Placement{XMM: 10, YMM: 10, Layer: "F.Cu"},
		Pads: []Pad{
			{Ref: "U1", Name: "1", Net: "POWER", Position: Point{XMM: 2, YMM: 0}, Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 1.45, HeightMM: 0.3}, Layers: []string{"F.Cu"}},
			{Ref: "U1", Name: "2", Net: "OTHER", Position: Point{XMM: 2, YMM: 0.5}, Shape: PadRect, Type: PadSMD, Size: Size{WidthMM: 1.45, HeightMM: 0.3}, Layers: []string{"F.Cu"}},
		},
	}}
	net := Net{Name: "POWER", Role: NetPower, Endpoints: []Endpoint{{Ref: "U1", Pin: "1"}}}

	if !netHasNarrowEndpoint(request, net) {
		t.Fatal("crowded nominal-width power pad did not request endpoint neckdown")
	}
	rules := applyAutomaticEndpointNeckdown(request.Rules, net.Role, netHasNarrowEndpoint(request, net))
	if rules.NeckdownWidthMM != pcbrules.DefaultPowerNeckdownWidthMM {
		t.Fatalf("crowded endpoint neckdown = %#v", rules)
	}
}

func TestAutomaticEndpointNeckdownPreservesExplicitPolicyAndSignals(t *testing.T) {
	explicit := Rules{TraceWidthMM: 0.5, NeckdownWidthMM: 0.3, NeckdownLengthMM: 1}
	if got := applyAutomaticEndpointNeckdown(explicit, NetPower, true); !reflect.DeepEqual(got, explicit) {
		t.Fatalf("explicit neckdown changed: %#v", got)
	}
	signal := Rules{TraceWidthMM: 0.5}
	if got := applyAutomaticEndpointNeckdown(signal, NetSignal, true); !reflect.DeepEqual(got, signal) {
		t.Fatalf("signal rules changed: %#v", got)
	}
	if got := applyAutomaticEndpointNeckdown(signal, NetPower, false); !reflect.DeepEqual(got, signal) {
		t.Fatalf("wide endpoint rules changed: %#v", got)
	}
}

func TestFallbackSMDEndpointConnectionPreservesSearchedEscapeGeometry(t *testing.T) {
	from := Endpoint{Ref: "U1", Pin: "1"}
	to := Endpoint{Ref: "U2", Pin: "1"}
	fromKey := endpointKey(from.Ref, from.Pin)
	toKey := endpointKey(to.Ref, to.Pin)
	fromEdge := Point{XMM: 2.5, YMM: 3}
	toEdge := Point{XMM: 8.5, YMM: 7}
	access := PadAccess{
		Pads: map[endpointID]Pad{
			fromKey: {Position: Point{XMM: 3, YMM: 3}, Type: PadSMD},
			toKey:   {Position: Point{XMM: 8, YMM: 7}, Type: PadSMD},
		},
		AccessPoints: map[endpointID][]AccessPoint{
			fromKey: {{Endpoint: from, Point: fromEdge, SearchPoint: &fromEdge, Layer: "F.Cu"}},
			toKey:   {{Endpoint: to, Point: toEdge, SearchPoint: &toEdge, Layer: "F.Cu"}},
		},
	}
	escape := Segment{Net: "VCC", Layer: "F.Cu", Start: fromEdge, End: toEdge, WidthMM: 0.3}
	got := connectFallbackSMDEndpointsToCenters([]Segment{escape}, access, EndpointPair{From: from, To: to})
	if len(got) != 3 {
		t.Fatalf("segments = %#v, want center connector, unchanged escape, center connector", got)
	}
	if got[0].Start != access.Pads[fromKey].Position || got[0].End != fromEdge || got[1] != escape || got[2].Start != toEdge || got[2].End != access.Pads[toKey].Position {
		t.Fatalf("segments = %#v, searched escape geometry changed", got)
	}
}

func TestPruneSameLayerSegmentCyclesKeepsDeterministicSpanningCopper(t *testing.T) {
	segments := []Segment{{Net: "SIG", Layer: "F.Cu", Start: Point{XMM: 1, YMM: 1}, End: Point{XMM: 2, YMM: 1}}, {Net: "SIG", Layer: "F.Cu", Start: Point{XMM: 2, YMM: 1}, End: Point{XMM: 2, YMM: 2}}, {Net: "SIG", Layer: "F.Cu", Start: Point{XMM: 2, YMM: 2}, End: Point{XMM: 1, YMM: 1}}, {Net: "SIG", Layer: "B.Cu", Start: Point{XMM: 2, YMM: 2}, End: Point{XMM: 1, YMM: 1}}}
	got := removeSegmentIndexes(segments, sameLayerCycleClosingIndexes(segments))
	if len(got) != 3 || got[0] != segments[0] || got[1] != segments[1] || got[2] != segments[3] {
		t.Fatalf("segments = %#v, want stable same-layer spanning copper", got)
	}
}

func TestPruneSameLayerSegmentCyclesRemovesRedundantDanglingLeaf(t *testing.T) {
	request := singleLayerSearchRequest()
	access := BuildPadAccess(request)
	from := access.Pads[endpointKey(request.Nets[0].Endpoints[0].Ref, request.Nets[0].Endpoints[0].Pin)].Position
	to := access.Pads[endpointKey(request.Nets[0].Endpoints[1].Ref, request.Nets[0].Endpoints[1].Pin)].Position
	main := Segment{Net: request.Nets[0].Name, Layer: "F.Cu", Start: from, End: to, WidthMM: request.Rules.TraceWidthMM}
	stub := Segment{
		Net: request.Nets[0].Name, Layer: "F.Cu", Start: from,
		End: Point{XMM: from.XMM, YMM: from.YMM + 2}, WidthMM: request.Rules.TraceWidthMM,
	}
	route := Route{Net: request.Nets[0].Name, Segments: []Segment{main, stub}}
	got := pruneConnectedSameLayerSegmentCycles(request, route, access)
	if len(got) != 1 || got[0] != main {
		t.Fatalf("segments = %#v, want only endpoint-spanning copper", got)
	}
}

func crossingNetsRequest() Request {
	request := singleLayerSearchRequest()
	request.Nets[0].Name = "A_HORIZONTAL"
	request.Nets[0].Priority = 10
	request.Components[0].Pads[0].Net = "A_HORIZONTAL"
	request.Components[1].Pads[0].Net = "A_HORIZONTAL"
	request.Components = append(request.Components,
		testComponent("J3", "1", "B_VERTICAL", 12, 5),
		testComponent("J4", "1", "B_VERTICAL", 12, 15),
	)
	request.Nets = append(request.Nets, Net{
		Name:     "B_VERTICAL",
		Priority: 1,
		Endpoints: []Endpoint{
			{Ref: "J3", Pin: "1"},
			{Ref: "J4", Pin: "1"},
		},
	})
	return request
}

func partialRoutingRequest() Request {
	request := singleLayerSearchRequest()
	request.Nets[0].Name = "A_OK"
	request.Components[0].Pads[0].Net = "A_OK"
	request.Components[1].Pads[0].Net = "A_OK"
	request.Components[0].Position.XMM = 2
	request.Components[0].Position.YMM = 5
	request.Components[1].Position.XMM = 8
	request.Components[1].Position.YMM = 5
	request.Components = append(request.Components,
		testComponent("J3", "1", "Z_FAIL", 5, 15),
		testComponent("J4", "1", "Z_FAIL", 25, 15),
	)
	request.Nets = append(request.Nets, Net{
		Name: "Z_FAIL",
		Endpoints: []Endpoint{
			{Ref: "J3", Pin: "1"},
			{Ref: "J4", Pin: "1"},
		},
	})
	request.Obstacles = []Obstacle{{
		Kind:  ObstacleKeepout,
		Layer: "F.Cu",
		Geometry: Shape{Rect: &Rect{
			Min: Point{XMM: 10, YMM: 0},
			Max: Point{XMM: 15, YMM: 20},
		}},
	}}
	return request
}

func failedBranchOccupancyRequest() Request {
	request := singleLayerSearchRequest()
	request.Board.WidthMM = 10
	request.Board.HeightMM = 10
	request.Board.MarginMM = 1
	request.Strategy.AllowPartial = true
	request.Components = []Component{
		testComponent("A1", "1", "A_FAIL", 1.5, 5),
		testComponent("A2", "1", "A_FAIL", 8.5, 5),
		testComponent("A3", "1", "A_FAIL", 8, 8),
		testComponent("B1", "1", "B_LATER", 5, 2),
		testComponent("B2", "1", "B_LATER", 5, 8),
	}
	request.Nets = []Net{
		{Name: "A_FAIL", Priority: 10, Endpoints: []Endpoint{{Ref: "A1", Pin: "1"}, {Ref: "A2", Pin: "1"}, {Ref: "A3", Pin: "1"}}},
		{Name: "B_LATER", Priority: 1, Endpoints: []Endpoint{{Ref: "B1", Pin: "1"}, {Ref: "B2", Pin: "1"}}},
	}
	request.Obstacles = []Obstacle{
		{Kind: ObstacleKeepout, Layer: "F.Cu", Geometry: Shape{Rect: &Rect{Min: Point{XMM: 6.5, YMM: 6.5}, Max: Point{XMM: 9.5, YMM: 7}}}},
		{Kind: ObstacleKeepout, Layer: "F.Cu", Geometry: Shape{Rect: &Rect{Min: Point{XMM: 6.5, YMM: 9}, Max: Point{XMM: 9.5, YMM: 9.5}}}},
		{Kind: ObstacleKeepout, Layer: "F.Cu", Geometry: Shape{Rect: &Rect{Min: Point{XMM: 6.5, YMM: 7}, Max: Point{XMM: 7, YMM: 9}}}},
		{Kind: ObstacleKeepout, Layer: "F.Cu", Geometry: Shape{Rect: &Rect{Min: Point{XMM: 9, YMM: 7}, Max: Point{XMM: 9.5, YMM: 9}}}},
	}
	return request
}

func testComponent(ref string, pin string, net string, x float64, y float64) Component {
	return Component{
		Ref:      ref,
		Position: Placement{XMM: x, YMM: y, Layer: "F.Cu"},
		Pads: []Pad{{
			Ref:      ref,
			Name:     pin,
			Net:      net,
			Position: Point{},
			Shape:    PadCircle,
			Type:     PadSMD,
			Size:     Size{WidthMM: 1, HeightMM: 1},
			Layers:   []string{"F.Cu"},
		}},
	}
}

func segmentContainsPoint(segment Segment, point Point) bool {
	minX := min(segment.Start.XMM, segment.End.XMM)
	maxX := max(segment.Start.XMM, segment.End.XMM)
	minY := min(segment.Start.YMM, segment.End.YMM)
	maxY := max(segment.Start.YMM, segment.End.YMM)
	return point.XMM >= minX && point.XMM <= maxX && point.YMM >= minY && point.YMM <= maxY
}
