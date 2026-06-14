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

func TestPlanEndpointPairsIsDeterministicOnTies(t *testing.T) {
	endpoints := []Endpoint{{Ref: "B", Pin: "1"}, {Ref: "A", Pin: "1"}, {Ref: "C", Pin: "1"}}
	access := PadAccess{AccessPoints: map[endpointID][]AccessPoint{
		endpointKey("A", "1"): {{Endpoint: Endpoint{Ref: "A", Pin: "1"}, Point: Point{XMM: 0, YMM: 0}, Layer: "F.CU"}},
		endpointKey("B", "1"): {{Endpoint: Endpoint{Ref: "B", Pin: "1"}, Point: Point{XMM: 1, YMM: 0}, Layer: "F.CU"}},
		endpointKey("C", "1"): {{Endpoint: Endpoint{Ref: "C", Pin: "1"}, Point: Point{XMM: -1, YMM: 0}, Layer: "F.CU"}},
	}}
	first, firstIssues := planEndpointPairs("SIG", endpoints, access)
	second, secondIssues := planEndpointPairs("SIG", endpoints, access)
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
