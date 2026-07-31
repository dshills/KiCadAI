package schematiclayout

import (
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
)

func TestRouteEmitsOrthogonalSegments(t *testing.T) {
	result := Route(Request{
		Sheet: testSheet(),
		Nets:  []Net{{Name: "SIG", Endpoints: []Endpoint{{Ref: "R1", Pin: "1"}, {Ref: "R2", Pin: "1"}}}},
	}, Result{Components: []PlacedComponent{
		{Component: Component{Ref: "R1", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "R2", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(30)}},
	}})
	if len(result.Wires) < 2 {
		t.Fatalf("wire count = %d, want an orthogonal routed path", len(result.Wires))
	}
	if len(result.Connections) != 1 || len(result.Connections[0].Points) < 3 {
		t.Fatalf("routed connection = %#v", result.Connections)
	}
	for _, wire := range result.Wires {
		if wire.From.X != wire.To.X && wire.From.Y != wire.To.Y {
			t.Fatalf("diagonal wire = %#v", wire)
		}
	}
}

func TestRouteSelfLoopUsesCanonicalPinAnchors(t *testing.T) {
	component := PlacedComponent{
		Component: Component{Ref: "u1_a", Pins: []Pin{
			{Number: "1", At: kicadfiles.Point{X: -kicadfiles.MM(2.54)}},
			{Number: "2", At: kicadfiles.Point{X: kicadfiles.MM(2.54)}},
		}},
		PlacedAt: kicadfiles.Point{X: kicadfiles.MM(40.1), Y: kicadfiles.MM(30.9)},
	}
	result := Route(Request{Nets: []Net{{Name: "FB", Endpoints: []Endpoint{{Ref: "u1_a", Pin: "1"}, {Ref: "u1_a", Pin: "2"}}}}}, Result{Components: []PlacedComponent{component}})
	if len(result.Connections) != 1 || len(result.Connections[0].Points) < 2 {
		t.Fatalf("self-loop route = %#v", result.Connections)
	}
	points := result.Connections[0].Points
	wantStart := schematic.CanonicalConnectionAnchor(component.PlacedAt, component.Pins[0].At, 0, schematic.SymbolMirrorNone)
	wantEnd := schematic.CanonicalConnectionAnchor(component.PlacedAt, component.Pins[1].At, 0, schematic.SymbolMirrorNone)
	if points[0] != wantStart || points[len(points)-1] != wantEnd {
		t.Fatalf("self-loop endpoints = %#v/%#v, want %#v/%#v", points[0], points[len(points)-1], wantStart, wantEnd)
	}
}

func TestRouteAcrossPackageUnitsUsesCanonicalPinAnchors(t *testing.T) {
	components := []PlacedComponent{
		{Component: Component{Ref: "u1_a", Pins: []Pin{{Number: "1", At: kicadfiles.Point{X: kicadfiles.MM(3.81)}}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(30.1), Y: kicadfiles.MM(30.9)}},
		{Component: Component{Ref: "u1_b", Pins: []Pin{{Number: "1", At: kicadfiles.Point{X: -kicadfiles.MM(3.81)}}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(60.1), Y: kicadfiles.MM(30.9)}},
	}
	result := Route(Request{Nets: []Net{{Name: "INTERSTAGE", Endpoints: []Endpoint{{Ref: "u1_a", Pin: "1"}, {Ref: "u1_b", Pin: "1"}}}}}, Result{Components: components})
	if len(result.Connections) != 1 || len(result.Connections[0].Points) < 2 {
		t.Fatalf("cross-unit route = %#v", result.Connections)
	}
	points := result.Connections[0].Points
	wantStart := schematic.CanonicalConnectionAnchor(components[0].PlacedAt, components[0].Pins[0].At, 0, schematic.SymbolMirrorNone)
	wantEnd := schematic.CanonicalConnectionAnchor(components[1].PlacedAt, components[1].Pins[0].At, 0, schematic.SymbolMirrorNone)
	if points[0] != wantStart || points[len(points)-1] != wantEnd {
		t.Fatalf("cross-unit endpoints = %#v/%#v, want %#v/%#v", points[0], points[len(points)-1], wantStart, wantEnd)
	}
}

func TestRouteKeepsLongLocalNetVisible(t *testing.T) {
	result := Route(Request{
		Sheet: testSheet(),
		Rules: Rules{Profile: ProfileStandard, LongWireThreshold: kicadfiles.MM(10), LabelFallbackEnabled: true},
		Nets:  []Net{{Name: "LONG_SIG", Endpoints: []Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "J2", Pin: "1"}}}},
	}, Result{Components: []PlacedComponent{
		{Component: Component{Ref: "J1", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "J2", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(80), Y: kicadfiles.MM(20)}},
	}})
	if len(result.Labels) != 0 || len(result.Wires) == 0 {
		t.Fatalf("labels=%#v wires=%#v, want a visible continuous local route", result.Labels, result.Wires)
	}
}

func TestRouteGridSearchNavigatesAlternatingObstacles(t *testing.T) {
	sheet := Sheet{Name: "grid", Width: kicadfiles.MM(120), Height: kicadfiles.MM(100), Margin: kicadfiles.MM(5)}
	components := []PlacedComponent{
		{
			Component: Component{
				Ref: "J1", BodyKnown: true,
				Body: Rect{MinX: -kicadfiles.MM(5), MinY: -kicadfiles.MM(5), MaxX: kicadfiles.MM(5), MaxY: kicadfiles.MM(5)},
				Pins: []Pin{{Number: "1", At: kicadfiles.Point{X: kicadfiles.MM(5)}}},
			},
			PlacedAt: kicadfiles.Point{X: kicadfiles.MM(15), Y: kicadfiles.MM(50)},
		},
		{
			Component: Component{
				Ref: "J2", BodyKnown: true,
				Body: Rect{MinX: -kicadfiles.MM(5), MinY: -kicadfiles.MM(5), MaxX: kicadfiles.MM(5), MaxY: kicadfiles.MM(5)},
				Pins: []Pin{{Number: "1", At: kicadfiles.Point{X: -kicadfiles.MM(5)}}},
			},
			PlacedAt: kicadfiles.Point{X: kicadfiles.MM(105), Y: kicadfiles.MM(50)},
		},
		{
			Component: Component{
				Ref: "O1", BodyKnown: true,
				Body: Rect{MinX: -kicadfiles.MM(5), MinY: -kicadfiles.MM(45), MaxX: kicadfiles.MM(5), MaxY: kicadfiles.MM(10)},
			},
			PlacedAt: kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(50)},
		},
		{
			Component: Component{
				Ref: "O2", BodyKnown: true,
				Body: Rect{MinX: -kicadfiles.MM(5), MinY: -kicadfiles.MM(10), MaxX: kicadfiles.MM(5), MaxY: kicadfiles.MM(45)},
			},
			PlacedAt: kicadfiles.Point{X: kicadfiles.MM(80), Y: kicadfiles.MM(50)},
		},
	}
	request := Request{
		Sheet: sheet,
		Rules: Rules{Profile: ProfileStandard, LabelFallbackEnabled: false, LabelFallbackConfigured: true},
		Nets:  []Net{{Name: "WEAVE", Endpoints: []Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "J2", Pin: "1"}}}},
	}
	result := Route(request, Result{Sheet: sheet, Components: components})
	if len(result.Connections) != 1 || result.Connections[0].UseLabels {
		t.Fatalf("grid route connection = %#v", result.Connections)
	}
	if len(result.Connections[0].Points) < 6 {
		t.Fatalf("grid route points = %#v, want a multi-bend obstacle path", result.Connections[0].Points)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == SeverityError {
			t.Fatalf("grid route diagnostic = %#v", diagnostic)
		}
	}
}

func TestRouteReservesEndpointLabelsBeforeDirectWires(t *testing.T) {
	request := Request{
		Sheet: testSheet(),
		Rules: Rules{Profile: ProfileStandard, LabelFallbackEnabled: true},
		Nets: []Net{
			{Name: "DIRECT", OriginalOrdinal: 0, Endpoints: []Endpoint{{Ref: "R1", Pin: "1"}, {Ref: "R2", Pin: "1"}}},
			{Name: "LABELED", OriginalOrdinal: 1, EndpointLabels: true, Endpoints: []Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "J2", Pin: "1"}}},
		},
	}
	components := []PlacedComponent{
		{Component: Component{Ref: "R1", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "R2", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "J1", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(40)}},
		{Component: Component{Ref: "J2", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(40)}},
	}

	placed := Result{Components: components}
	ordered := orderedNetsForRouting(request.Nets, pinAnchors(placed.Components), request.Components, normalizeRules(request.Rules))
	if len(ordered) != 2 || ordered[0].Name != "LABELED" || ordered[1].Name != "DIRECT" {
		t.Fatalf("routing order = %#v, want endpoint labels before direct wires", ordered)
	}
}

func TestLabelDirectionUsesPlacedBodyEdge(t *testing.T) {
	body := Rect{MinX: kicadfiles.MM(40), MinY: kicadfiles.MM(40), MaxX: kicadfiles.MM(60), MaxY: kicadfiles.MM(60)}
	if direction := labelDirectionFromBody(kicadfiles.Point{X: kicadfiles.MM(41), Y: kicadfiles.MM(50)}, body, kicadfiles.MM(1)); direction != (kicadfiles.Point{X: -kicadfiles.MM(1)}) {
		t.Fatalf("left-edge direction = %#v", direction)
	}
	if direction := labelDirectionFromBody(kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(59)}, body, kicadfiles.MM(1)); direction != (kicadfiles.Point{Y: kicadfiles.MM(1)}) {
		t.Fatalf("bottom-edge direction = %#v", direction)
	}
}

func TestRoutePrefersCalibratedPinDirectionForLabelStub(t *testing.T) {
	result := Route(Request{
		Sheet: testSheet(),
		Nets: []Net{{
			Name:            "I2C_SDA",
			PreferredLabels: true,
			Endpoints:       []Endpoint{{Ref: "U1", Pin: "1"}},
		}},
	}, Result{Components: []PlacedComponent{{
		Component: Component{
			Ref:       "U1",
			BodyKnown: true,
			Body:      Rect{MinX: -kicadfiles.MM(5), MinY: -kicadfiles.MM(5), MaxX: kicadfiles.MM(5), MaxY: kicadfiles.MM(5)},
			Pins: []Pin{{
				Number:    "1",
				At:        kicadfiles.Point{X: -kicadfiles.MM(2.54), Y: kicadfiles.MM(3.81)},
				Direction: kicadfiles.Point{X: -1},
			}},
		},
		PlacedAt: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)},
	}}})
	if len(result.Wires) != 1 {
		t.Fatalf("wires = %#v, want one label stub", result.Wires)
	}
	anchor := kicadfiles.Point{X: kicadfiles.MM(50 - 2.54), Y: kicadfiles.MM(50 + 3.81)}
	other := result.Wires[0].From
	if other == anchor {
		other = result.Wires[0].To
	}
	if result.Wires[0].From.Y != result.Wires[0].To.Y || other.X >= anchor.X {
		t.Fatalf("label stub = %#v, want a leftward horizontal stub", result.Wires[0])
	}
}

func TestRouteUsesOrthogonalEndpointAccessWhenPreferredStubIsBlocked(t *testing.T) {
	result := Route(Request{
		Sheet: testSheet(),
		Nets: []Net{{
			Name:            "CONTROL",
			PreferredLabels: true,
			Endpoints:       []Endpoint{{Ref: "U1", Pin: "1"}},
		}},
	}, Result{Components: []PlacedComponent{
		{
			Component: Component{
				Ref:       "U1",
				BodyKnown: true,
				Body:      Rect{MinX: -kicadfiles.MM(5), MinY: -kicadfiles.MM(5), MaxX: kicadfiles.MM(5), MaxY: kicadfiles.MM(5)},
				Pins: []Pin{{
					Number:    "1",
					At:        kicadfiles.Point{X: -kicadfiles.MM(5)},
					Direction: kicadfiles.Point{X: -1},
				}},
			},
			PlacedAt: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)},
		},
		{
			Component: Component{
				Ref:       "U2",
				BodyKnown: true,
				Body:      Rect{MinX: -kicadfiles.MM(17), MinY: -kicadfiles.MM(10), MaxX: kicadfiles.MM(17), MaxY: kicadfiles.MM(10)},
			},
			PlacedAt: kicadfiles.Point{X: kicadfiles.MM(27), Y: kicadfiles.MM(50)},
		},
	}})
	if len(result.Wires) != 1 {
		t.Fatalf("wires = %#v, want one endpoint stub", result.Wires)
	}
	stub := result.Wires[0]
	if stub.From.X != stub.To.X {
		t.Fatalf("stub = %#v, want orthogonal vertical access after the preferred left exit is blocked", stub)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "label_placement_fallback" {
			t.Fatalf("unexpected crowded fallback: %#v", result.Diagnostics)
		}
	}
}

func TestRouteEmitsLabelForSingleEndpointNet(t *testing.T) {
	result := Route(Request{
		Sheet: testSheet(),
		Nets: []Net{{
			Name: "OFF_SHEET", PreferredLabels: true,
			Endpoints: []Endpoint{{Ref: "U1", Pin: "1"}},
		}},
	}, Result{Components: []PlacedComponent{{
		Component: Component{Ref: "U1", Pins: []Pin{{Number: "1", At: kicadfiles.Point{X: kicadfiles.MM(2.54)}}}},
		PlacedAt:  kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)},
	}}})
	if len(result.Labels) != 1 || result.Labels[0].Text != "OFF_SHEET" {
		t.Fatalf("labels = %#v, want one off-sheet label", result.Labels)
	}
	if len(result.Wires) != 1 {
		t.Fatalf("wires = %#v, want one label stub", result.Wires)
	}
	if result.Wires[0].From == result.Wires[0].To {
		t.Fatalf("label stub was not extended: %#v", result.Wires[0])
	}
}

func TestLabelPlacementRejectsDifferentNetEndpointContact(t *testing.T) {
	stub := WireSegment{NetName: "RIGHT", From: kicadfiles.Point{X: kicadfiles.MM(20)}, To: kicadfiles.Point{X: kicadfiles.MM(30)}}
	existing := WireSegment{NetName: "LEFT", From: kicadfiles.Point{X: kicadfiles.MM(10)}, To: stub.From}
	box := TextEstimate("RIGHT", stub.To, 0, 0)
	if !labelPlacementCollides(box, stub, Endpoint{}, Result{Wires: []WireSegment{existing}}, Request{}) {
		t.Fatalf("different-net stubs sharing an endpoint were accepted: new=%#v existing=%#v", stub, existing)
	}
}

func TestRouteRespectsDisabledLabelFallback(t *testing.T) {
	result := Route(Request{
		Sheet: testSheet(),
		Rules: Rules{Profile: ProfileStandard, LongWireThreshold: kicadfiles.MM(10), LabelFallbackEnabled: false, LabelFallbackConfigured: true},
		Nets:  []Net{{Name: "LONG_SIG", Endpoints: []Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "J2", Pin: "1"}}}},
	}, Result{Components: []PlacedComponent{
		{Component: Component{Ref: "J1", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "J2", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(80), Y: kicadfiles.MM(20)}},
	}})
	if len(result.Labels) != 0 {
		t.Fatalf("labels = %#v, want direct routing when fallback is disabled", result.Labels)
	}
	if len(result.Wires) == 0 {
		t.Fatal("disabled fallback dropped the routed connection")
	}
}

func TestRouteKeepsOrdinaryMultiEndpointPowerNetVisible(t *testing.T) {
	result := Route(Request{
		Sheet: testSheet(),
		Nets: []Net{{
			Name: "VCC",
			Role: "power",
			Endpoints: []Endpoint{
				{Ref: "U1", Pin: "1"},
				{Ref: "C1", Pin: "1"},
				{Ref: "J1", Pin: "1"},
			},
		}},
	}, Result{Components: []PlacedComponent{
		{Component: Component{Ref: "U1", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "C1", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "J1", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)}},
	}})
	if len(result.Labels) != 0 || len(result.Wires) < 2 {
		t.Fatalf("labels=%#v wires=%#v, want a visible routed power tree", result.Labels, result.Wires)
	}
}

func TestRoutePreferredLocalNetDoesNotHideWire(t *testing.T) {
	result := Route(Request{
		Sheet: testSheet(),
		Nets: []Net{{
			Name:            "SENSE",
			PreferredLabels: true,
			Endpoints:       []Endpoint{{Ref: "R1", Pin: "1"}, {Ref: "U1", Pin: "1"}},
		}},
	}, Result{Components: []PlacedComponent{
		{Component: Component{Ref: "R1", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "U1", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(30)}},
	}})
	if len(result.Wires) == 0 || len(result.Connections) != 1 || result.Connections[0].UseLabels {
		t.Fatalf("preferred local net was hidden: connections=%#v wires=%#v", result.Connections, result.Wires)
	}
	if len(result.Labels) != 1 || !result.Labels[0].RouteAnnotation {
		t.Fatalf("route annotation = %#v, want one label on the visible conductor", result.Labels)
	}
	for index := 1; index < len(result.Connections[0].Points)-1; index++ {
		if result.Labels[0].Position == result.Connections[0].Points[index] {
			t.Fatalf("route annotation landed on multi-wire bend: %#v", result.Labels[0])
		}
	}
	if result.Report.LocalTwoPointNetCount != 1 || result.Report.ContinuousLocalNetCount != 1 {
		t.Fatalf("continuous-local metrics = %#v", result.Report)
	}
}

func TestRouteAnnotatesStraightPreferredLocalWire(t *testing.T) {
	result := Route(Request{
		Sheet: testSheet(),
		Nets: []Net{{
			Name:            "PASSIVE_LINK",
			PreferredLabels: true,
			Endpoints:       []Endpoint{{Ref: "R1", Pin: "1"}, {Ref: "R2", Pin: "1"}},
		}},
	}, Result{Components: []PlacedComponent{
		{Component: Component{Ref: "R1", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "R2", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(80), Y: kicadfiles.MM(20)}},
	}})
	if len(result.Connections) != 1 || result.Connections[0].UseLabels || len(result.Wires) == 0 {
		t.Fatalf("straight preferred route = %#v wires=%#v", result.Connections, result.Wires)
	}
	if len(result.Labels) != 1 || !result.Labels[0].RouteAnnotation {
		t.Fatalf("straight-route annotation = %#v", result.Labels)
	}
	if !schematicPointOnLayoutConnection(result.Labels[0].Position, result.Connections[0].Points) {
		t.Fatalf("annotation %v is not on route %v", result.Labels[0].Position, result.Connections[0].Points)
	}
}

func schematicPointOnLayoutConnection(point kicadfiles.Point, points []kicadfiles.Point) bool {
	for index := 1; index < len(points); index++ {
		from, to := points[index-1], points[index]
		switch {
		case from.X == to.X && point.X == from.X && point.Y >= minIU(from.Y, to.Y) && point.Y <= maxIU(from.Y, to.Y):
			return true
		case from.Y == to.Y && point.Y == from.Y && point.X >= minIU(from.X, to.X) && point.X <= maxIU(from.X, to.X):
			return true
		}
	}
	return false
}

func TestBranchJunctionsMarksRealThreeWayBranch(t *testing.T) {
	center := kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(30)}
	junctions := branchJunctions([]WireSegment{
		{NetName: "TREE", From: kicadfiles.Point{X: kicadfiles.MM(10), Y: center.Y}, To: center},
		{NetName: "TREE", From: center, To: kicadfiles.Point{X: kicadfiles.MM(50), Y: center.Y}},
		{NetName: "TREE", From: center, To: kicadfiles.Point{X: center.X, Y: kicadfiles.MM(50)}},
	})
	if len(junctions) != 1 || junctions[0].Position != center {
		t.Fatalf("junctions = %#v, want one real branch marker", junctions)
	}
}

func TestRouteAvoidsUnrelatedSymbolBody(t *testing.T) {
	request := Request{
		Sheet: testSheet(),
		Nets:  []Net{{Name: "SIG", Endpoints: []Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "J2", Pin: "1"}}}},
	}
	result := Route(request, Result{Components: []PlacedComponent{
		{Component: Component{Ref: "J1", Pins: []Pin{{Number: "1", At: kicadfiles.Point{X: kicadfiles.MM(5)}}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(60)}},
		{Component: Component{Ref: "U1", Role: "mcu"}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(60), Y: kicadfiles.MM(60)}},
		{Component: Component{Ref: "J2", Pins: []Pin{{Number: "1", At: kicadfiles.Point{X: kicadfiles.MM(-5)}}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(100), Y: kicadfiles.MM(60)}},
	}})
	obstacle := componentBody(resultComponentByRef(t, result.Components, "U1"))
	if len(result.Wires) == 0 {
		t.Fatalf("expected direct routed wires, got labels %#v", result.Labels)
	}
	for _, wire := range result.Wires {
		if SegmentIntersectsRect(wire, obstacle) {
			t.Fatalf("wire %#v intersects obstacle %#v", wire, obstacle)
		}
	}
}

func TestRouteAvoidsExistingUnrelatedWire(t *testing.T) {
	components := []PlacedComponent{
		{Component: Component{Ref: "A", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "B", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(80), Y: kicadfiles.MM(80)}},
	}
	result := Route(Request{
		Sheet: testSheet(),
		Nets:  []Net{{Name: "NEW", Endpoints: []Endpoint{{Ref: "A", Pin: "1"}, {Ref: "B", Pin: "1"}}}},
	}, Result{
		Components: components,
		Wires:      []WireSegment{{NetName: "EXISTING", From: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(10)}, To: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(90)}}},
	})
	for _, wire := range result.Wires {
		if wire.NetName != "NEW" {
			continue
		}
		if wireSegmentsCross(wire, result.Wires[0]) {
			t.Fatalf("new wire %#v crosses existing wire %#v", wire, result.Wires[0])
		}
	}
}

func TestRouteRejectsUnrelatedPinAnchor(t *testing.T) {
	components := []PlacedComponent{
		{Component: Component{Ref: "A", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "B", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(80), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "BLOCK", BodyKnown: true, Pins: []Pin{{Number: "1", At: kicadfiles.Point{X: kicadfiles.MM(10)}}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(20)}},
	}
	request := Request{
		Sheet: testSheet(),
		Rules: Rules{Profile: ProfileStandard, LabelFallbackEnabled: false, LabelFallbackConfigured: true},
		Nets:  []Net{{Name: "SIG", Endpoints: []Endpoint{{Ref: "A", Pin: "1"}, {Ref: "B", Pin: "1"}}}},
	}
	result := Route(request, Result{Components: components})
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == DiagnosticWirePinOverlap {
			t.Fatalf("route retained unrelated pin overlap: %#v", result.Diagnostics)
		}
	}
}

func TestScoreRouteRejectsDifferentNetEndpointContact(t *testing.T) {
	points := []kicadfiles.Point{
		{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)},
		{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)},
	}
	existing := WireSegment{
		NetName: "B",
		From:    kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)},
		To:      kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(40)},
	}
	_, clean := scoreRoute(points, "A", Endpoint{}, Endpoint{}, Result{Wires: []WireSegment{existing}}, Request{Sheet: testSheet()})
	if clean {
		t.Fatal("route ending on a different net endpoint was accepted")
	}
}

func TestSameNameNetFragmentsShareAllowedPinAnchors(t *testing.T) {
	components := []PlacedComponent{
		{Component: Component{Ref: "A", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "B", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "C", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(80), Y: kicadfiles.MM(20)}},
	}
	request := Request{Nets: []Net{
		{Name: "SIG", Endpoints: []Endpoint{{Ref: "A", Pin: "1"}, {Ref: "B", Pin: "1"}}},
		{Name: "SIG", Endpoints: []Endpoint{{Ref: "B", Pin: "1"}, {Ref: "C", Pin: "1"}}},
	}}
	segment := WireSegment{NetName: "SIG", From: components[0].PlacedAt, To: components[2].PlacedAt}
	if endpoint, overlaps := unrelatedPinForWire(segment, "SIG", Result{Components: components}, request); overlaps {
		t.Fatalf("same-net pin %s.%s classified as unrelated", endpoint.Ref, endpoint.Pin)
	}
}

func TestOrderedRoutableEndpointsUsesGeometryNotInputOrder(t *testing.T) {
	anchors := map[Endpoint]kicadfiles.Point{
		{Ref: "left", Pin: "1"}:   {X: kicadfiles.MM(10), Y: kicadfiles.MM(20)},
		{Ref: "middle", Pin: "1"}: {X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		{Ref: "right", Pin: "1"}:  {X: kicadfiles.MM(30), Y: kicadfiles.MM(10)},
	}
	net := Net{Name: "BRANCH", Endpoints: []Endpoint{{Ref: "right", Pin: "1"}, {Ref: "left", Pin: "1"}, {Ref: "middle", Pin: "1"}}}
	ordered, missing := orderedRoutableEndpoints(net, anchors)
	if len(missing) != 0 || len(ordered) != 3 {
		t.Fatalf("ordered=%#v missing=%#v", ordered, missing)
	}
	if ordered[0].endpoint.Ref != "left" || ordered[1].endpoint.Ref != "middle" || ordered[2].endpoint.Ref != "right" {
		t.Fatalf("endpoint order = %#v", ordered)
	}
}

func TestOrderedRoutableEndpointsVisitsBranchPointBeforeShunt(t *testing.T) {
	anchors := map[Endpoint]kicadfiles.Point{
		{Ref: "junction", Pin: "1"}: {X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
		{Ref: "shunt", Pin: "1"}:    {X: kicadfiles.MM(10), Y: kicadfiles.MM(30)},
		{Ref: "output", Pin: "1"}:   {X: kicadfiles.MM(40), Y: kicadfiles.MM(10)},
	}
	net := Net{Name: "BRANCH", Endpoints: []Endpoint{{Ref: "junction", Pin: "1"}, {Ref: "shunt", Pin: "1"}, {Ref: "output", Pin: "1"}}}
	ordered, missing := orderedRoutableEndpoints(net, anchors)
	if len(missing) != 0 || len(ordered) != 3 {
		t.Fatalf("ordered=%#v missing=%#v", ordered, missing)
	}
	if ordered[0].endpoint.Ref != "output" || ordered[1].endpoint.Ref != "junction" || ordered[2].endpoint.Ref != "shunt" {
		t.Fatalf("branch traversal = %#v", ordered)
	}
}

func TestRouteAnnotationFallsBackFromBlockedMidpointToSegmentInset(t *testing.T) {
	from := kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(30)}
	to := kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(30)}
	result := Result{
		Components: []PlacedComponent{{
			Component: Component{Ref: "blocker", Body: Rect{MinX: -kicadfiles.MM(2), MinY: -kicadfiles.MM(2), MaxX: kicadfiles.MM(2), MaxY: kicadfiles.MM(2)}, BodyKnown: true},
			PlacedAt:  kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(30)},
		}},
		Connections: []RoutedConnection{{NetName: "SIGNAL", Points: []kicadfiles.Point{from, to}}},
		Wires:       []WireSegment{{NetName: "SIGNAL", From: from, To: to}},
	}
	rules := DefaultRules(ProfileStandard)
	appendRouteAnnotation(&result, "SIGNAL", Request{Sheet: testSheet()}, rules)
	if len(result.Labels) != 1 || !result.Labels[0].RouteAnnotation {
		t.Fatalf("route labels = %#v", result.Labels)
	}
	if result.Labels[0].Position == (kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(30)}) {
		t.Fatalf("annotation stayed on blocked midpoint: %#v", result.Labels[0])
	}
}

func resultComponentByRef(t *testing.T, components []PlacedComponent, ref string) PlacedComponent {
	t.Helper()
	for _, component := range components {
		if component.Ref == ref {
			return component
		}
	}
	t.Fatalf("missing component %s", ref)
	return PlacedComponent{}
}
