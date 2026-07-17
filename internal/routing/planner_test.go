package routing

import "testing"

func TestPlanRoutesOrdersNetsDeterministically(t *testing.T) {
	request := minimalRequest()
	request.Nets = []Net{
		{Name: "Z_SIG", Role: NetSignal, Priority: 1, Endpoints: request.Nets[0].Endpoints},
		{Name: "PWR", Role: NetPower, Priority: 1, Endpoints: request.Nets[0].Endpoints},
		{Name: "FAST", Role: NetSignal, Priority: 10, Endpoints: request.Nets[0].Endpoints},
	}

	plans, issues := PlanRoutes(request, BuildPadAccess(request))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if got := []string{plans[0].Net.Name, plans[1].Net.Name, plans[2].Net.Name}; got[0] != "FAST" || got[1] != "PWR" || got[2] != "Z_SIG" {
		t.Fatalf("plan order = %#v", got)
	}
}

func TestPlanRoutesReservesCompactLocalEscapeBeforeBroadNet(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Components = append(request.Components,
		testComponent("J3", "1", "LOCAL", 10, 10),
		testComponent("J4", "1", "LOCAL", 11, 10),
	)
	request.Nets = []Net{
		{Name: "BROAD_POWER", Role: NetPower, Priority: 1, Endpoints: request.Nets[0].Endpoints},
		{Name: "LOCAL", Role: NetSignal, Priority: 1, Endpoints: []Endpoint{{Ref: "J3", Pin: "1"}, {Ref: "J4", Pin: "1"}}},
	}
	plans, issues := PlanRoutes(request, BuildPadAccess(request))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(plans) != 2 || plans[0].Net.Name != "LOCAL" {
		t.Fatalf("plan order = %#v", plans)
	}
}

func TestPlanRoutesPromotesHarderConstrainedEscapeFirst(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Strategy.NetOrder = NetOrderConstrainedEndpointAccessV1
	request.Components = append(request.Components,
		testComponent("J3", "1", "LOCAL", 10, 10),
		testComponent("J4", "1", "LOCAL", 11, 10),
		testComponent("J5", "1", "SECOND", 10, 12),
		testComponent("J6", "1", "SECOND", 12, 12),
	)
	request.Nets = []Net{
		{Name: "GND", Role: NetGround, Priority: 1, Endpoints: request.Nets[0].Endpoints},
		{Name: "LOCAL", Role: NetSignal, Priority: 1, Endpoints: []Endpoint{{Ref: "J3", Pin: "1"}, {Ref: "J4", Pin: "1"}}},
		{Name: "SECOND", Role: NetSignal, Priority: 1, Endpoints: []Endpoint{{Ref: "J5", Pin: "1"}, {Ref: "J6", Pin: "1"}}},
	}
	plans, issues := PlanRoutes(request, BuildPadAccess(request))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(plans) != 3 || plans[0].Net.Name != "SECOND" || plans[1].Net.Name != "LOCAL" || plans[2].Net.Name != "GND" {
		t.Fatalf("plan order = %#v", plans)
	}
}

func TestPlanRoutesEscapesConstrainedSmallFanoutPadBeforePowerTree(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Strategy.NetOrder = NetOrderConstrainedEndpointAccessV1
	request.Components = append(request.Components,
		testComponent("J3", "1", "NARROW", 10, 10),
		testComponent("J4", "1", "NARROW", 20, 10),
	)
	request.Components[2].Pads[0].Size = Size{WidthMM: 0.5, HeightMM: 0.25}
	request.Components[3].Pads[0].Size = Size{WidthMM: 0.5, HeightMM: 0.25}
	request.Nets = []Net{
		{Name: "POWER", Role: NetPower, Priority: 1, Endpoints: request.Nets[0].Endpoints},
		{Name: "NARROW", Role: NetSignal, Priority: 1, Endpoints: []Endpoint{{Ref: "J3", Pin: "1"}, {Ref: "J4", Pin: "1"}}},
	}
	plans, issues := PlanRoutes(request, BuildPadAccess(request))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(plans) != 2 || plans[0].Net.Name != "NARROW" {
		t.Fatalf("plan order = %#v, want constrained small-fanout escape first", plans)
	}
}

func TestPlanRoutesPrioritizesConstrainedSignalBeforeConstrainedSupportPower(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Strategy.NetOrder = NetOrderConstrainedEndpointAccessV1
	request.Rules.GridMM = 0.25
	request.Rules.TraceWidthMM = 0.25
	request.Components = append(request.Components,
		testComponent("J3", "1", "GPIO", 10, 10),
		testComponent("J4", "1", "GPIO", 20, 10),
		testComponent("J5", "1", "SUPPORT", 10, 12),
		testComponent("J6", "1", "SUPPORT", 20, 12),
	)
	for index := 2; index < len(request.Components); index++ {
		request.Components[index].Pads[0].Size = Size{WidthMM: 0.55, HeightMM: 1.6}
	}
	request.Nets = []Net{
		{Name: "SUPPORT", Role: NetPower, Priority: 1, Endpoints: []Endpoint{{Ref: "J5", Pin: "1"}, {Ref: "J6", Pin: "1"}}},
		{Name: "GPIO", Role: NetSignal, Priority: 1, Endpoints: []Endpoint{{Ref: "J3", Pin: "1"}, {Ref: "J4", Pin: "1"}}},
	}
	plans, issues := PlanRoutes(request, BuildPadAccess(request))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(plans) != 2 || plans[0].Net.Name != "GPIO" {
		t.Fatalf("plan order = %#v, want constrained signal before support power", plans)
	}
}

func TestPlanRoutesPrioritizesBoundedConstrainedSignalFanoutBeforeGroundTree(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Board.Layers = []Layer{
		{Name: "F.Cu", Kind: LayerCopper, Routable: true},
		{Name: "In1.Cu", Kind: LayerCopper, Routable: true},
		{Name: "In2.Cu", Kind: LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: LayerCopper, Routable: true},
	}
	request.Strategy.NetOrder = NetOrderConstrainedEndpointAccessV1
	request.Rules.GridMM = 0.25
	request.Rules.TraceWidthMM = 0.25
	endpoints := make([]Endpoint, 0, 6)
	for index := 0; index < 6; index++ {
		ref := "S" + string(rune('1'+index))
		request.Components = append(request.Components, testComponent(ref, "1", "BUS", 10+20*float64(index), 12))
		request.Components[len(request.Components)-1].Pads[0].Size = Size{WidthMM: 0.25, HeightMM: 0.6}
		endpoints = append(endpoints, Endpoint{Ref: ref, Pin: "1"})
	}
	request.Nets = []Net{
		{Name: "GND", Role: NetGround, Priority: 1, Endpoints: request.Nets[0].Endpoints},
		{Name: "BUS", Role: NetSignal, Priority: 1, Endpoints: endpoints},
	}
	plans, issues := PlanRoutes(request, BuildPadAccess(request))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(plans) != 2 || plans[0].Net.Name != "BUS" {
		t.Fatalf("plan order = %#v, want bounded constrained signal fanout first", plans)
	}
}

func TestPlanRoutesReservesSmallEscapeBeforeDenseConstrainedFanout(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Board.Layers = []Layer{
		{Name: "F.Cu", Kind: LayerCopper, Routable: true},
		{Name: "In1.Cu", Kind: LayerCopper, Routable: true},
		{Name: "In2.Cu", Kind: LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: LayerCopper, Routable: true},
	}
	request.Strategy.NetOrder = NetOrderConstrainedEndpointAccessV1
	request.Rules.GridMM = 0.25
	request.Rules.TraceWidthMM = 0.2
	request.Components = append(request.Components,
		testComponent("U1", "1", "POWER", 10, 10),
		testComponent("U1", "2", "POWER", 11, 10),
		testComponent("J3", "1", "POWER", 14, 10),
		testComponent("J4", "1", "SIGNAL", 16, 10),
	)
	request.Components[2].Pads[0].Size = Size{WidthMM: 0.35, HeightMM: 0.7}
	request.Components[3].Pads[0].Size = Size{WidthMM: 0.35, HeightMM: 0.7}
	request.Components[len(request.Components)-1].Pads[0].Size = Size{WidthMM: 0.35, HeightMM: 0.7}
	request.Nets = []Net{
		{Name: "SIGNAL", Role: NetSignal, Priority: 1, Endpoints: []Endpoint{{Ref: "J2", Pin: "1"}, {Ref: "J4", Pin: "1"}}},
		{Name: "POWER", Role: NetSignal, Priority: 1, Endpoints: []Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "U1", Pin: "1"}, {Ref: "U1", Pin: "2"}, {Ref: "J3", Pin: "1"}}},
	}
	plans, issues := PlanRoutes(request, BuildPadAccess(request))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(plans) != 2 || plans[0].Net.Name != "SIGNAL" {
		t.Fatalf("plan order = %#v, want small constrained escape before dense fanout", plans)
	}
}

func TestPlanRoutesSkipsOneEndpointAndFixedNets(t *testing.T) {
	request := minimalRequest()
	request.Nets = []Net{
		{Name: "  ", Endpoints: request.Nets[0].Endpoints},
		{Name: "ONE", Endpoints: []Endpoint{{Ref: "J1", Pin: "1"}}},
		{Name: "FIXED", Fixed: true, Endpoints: request.Nets[0].Endpoints},
	}

	plans, issues := PlanRoutes(request, BuildPadAccess(request))
	if len(plans) != 0 {
		t.Fatalf("plans = %#v, want none", plans)
	}
	if len(issues) != 3 {
		t.Fatalf("issues = %#v, want three skip issues", issues)
	}
}

func TestPlanEndpointPairsUsesNearestNeighborTree(t *testing.T) {
	request := minimalRequest()
	request.Components = append(request.Components, Component{
		Ref:      "J3",
		Position: Placement{XMM: 22, YMM: 10, Layer: "F.Cu"},
		Pads: []Pad{{
			Name:   "1",
			Net:    "SIG",
			Shape:  PadCircle,
			Type:   PadThroughHole,
			Size:   Size{WidthMM: 1, HeightMM: 1},
			Drill:  &Drill{DiameterMM: 0.5},
			Layers: []string{"F.Cu", "B.Cu"},
		}},
	})
	request.Nets[0].Endpoints = append(request.Nets[0].Endpoints, Endpoint{Ref: "J3", Pin: "1"})

	plans, issues := PlanRoutes(request, BuildPadAccess(request))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(plans) != 1 || len(plans[0].Pairs) != 2 {
		t.Fatalf("plans = %#v", plans)
	}
	if plans[0].Pairs[1].To.Ref != "J3" {
		t.Fatalf("second pair = %#v, want nearest J3 after J2", plans[0].Pairs[1])
	}
}

func TestPlanEndpointPairsKeepsConstrainedEndpointAsLeaf(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Rules.GridMM = 0.25
	request.Rules.TraceWidthMM = 0.25
	request.Components = []Component{
		testComponent("W1", "1", "SIG", 10, 10),
		testComponent("N", "1", "SIG", 11, 10),
		testComponent("W2", "1", "SIG", 12, 10),
		testComponent("W3", "1", "SIG", 13, 10),
	}
	request.Components[0].Pads[0].Size = Size{WidthMM: 1, HeightMM: 1}
	request.Components[1].Pads[0].Size = Size{WidthMM: 0.5, HeightMM: 0.25}
	request.Components[2].Pads[0].Size = Size{WidthMM: 1, HeightMM: 1}
	request.Components[3].Pads[0].Size = Size{WidthMM: 1, HeightMM: 1}
	access := BuildPadAccess(request)
	pairs, issues := planEndpointPairs("SIG", NetSignal, []Endpoint{{Ref: "W1", Pin: "1"}, {Ref: "N", Pin: "1"}, {Ref: "W2", Pin: "1"}, {Ref: "W3", Pin: "1"}}, access, request.Rules, true)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(pairs) != 3 {
		t.Fatalf("pairs = %#v", pairs)
	}
	for _, pair := range pairs {
		if pair.From.Ref == "N" {
			t.Fatalf("pairs = %#v, constrained endpoint became an internal branch", pairs)
		}
	}
}

func TestPlanEndpointPairsIsDeterministicOnTies(t *testing.T) {
	endpoints := []Endpoint{{Ref: "B", Pin: "1"}, {Ref: "A", Pin: "1"}, {Ref: "C", Pin: "1"}}
	access := PadAccess{AccessPoints: map[endpointID][]AccessPoint{
		endpointKey("A", "1"): {{Endpoint: Endpoint{Ref: "A", Pin: "1"}, Point: Point{XMM: 0, YMM: 0}, Layer: "F.CU"}},
		endpointKey("B", "1"): {{Endpoint: Endpoint{Ref: "B", Pin: "1"}, Point: Point{XMM: 1, YMM: 0}, Layer: "F.CU"}},
		endpointKey("C", "1"): {{Endpoint: Endpoint{Ref: "C", Pin: "1"}, Point: Point{XMM: -1, YMM: 0}, Layer: "F.CU"}},
	}}
	first, firstIssues := planEndpointPairs("SIG", NetSignal, endpoints, access, Rules{}, false)
	second, secondIssues := planEndpointPairs("SIG", NetSignal, endpoints, access, Rules{}, false)
	if len(firstIssues) != 0 || len(secondIssues) != 0 {
		t.Fatalf("issues = %#v %#v", firstIssues, secondIssues)
	}
	if len(first) != len(second) {
		t.Fatalf("length mismatch")
	}
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("pair[%d] changed: %#v vs %#v", index, first[index], second[index])
		}
	}
	if first[0].From.Ref != "A" {
		t.Fatalf("first pair = %#v, want lexical seed A", first[0])
	}
}

func TestPlanRoutesReportsUnreachableEndpoint(t *testing.T) {
	request := minimalRequest()
	access := BuildPadAccess(request)
	delete(access.AccessPoints, endpointKey("J2", "1"))

	plans, issues := PlanRoutes(request, access)
	if len(plans) != 0 {
		t.Fatalf("plans = %#v, want none", plans)
	}
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one unreachable issue", issues)
	}
	if issues[0].Severity != "blocked" {
		t.Fatalf("severity = %s, want blocked", issues[0].Severity)
	}
}

func TestPlanRoutesReportsAllUnreachableEndpoints(t *testing.T) {
	request := minimalRequest()
	access := BuildPadAccess(request)
	delete(access.AccessPoints, endpointKey("J1", "1"))
	delete(access.AccessPoints, endpointKey("J2", "1"))

	plans, issues := PlanRoutes(request, access)
	if len(plans) != 0 {
		t.Fatalf("plans = %#v, want none", plans)
	}
	if len(issues) != 2 {
		t.Fatalf("issues = %#v, want both endpoints", issues)
	}
}
