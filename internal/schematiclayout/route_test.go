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

func TestRouteUsesLabelsForLongNet(t *testing.T) {
	result := Route(Request{
		Sheet: testSheet(),
		Rules: Rules{Profile: ProfileStandard, LongWireThreshold: kicadfiles.MM(10), LabelFallbackEnabled: true},
		Nets:  []Net{{Name: "LONG_SIG", Endpoints: []Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "J2", Pin: "1"}}}},
	}, Result{Components: []PlacedComponent{
		{Component: Component{Ref: "J1", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "J2", Pins: []Pin{{Number: "1"}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(80), Y: kicadfiles.MM(20)}},
	}})
	if len(result.Labels) != 2 || len(result.Wires) != 2 {
		t.Fatalf("labels=%#v wires=%#v, want two bounded label stubs", result.Labels, result.Wires)
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

func TestRouteUsesLabelsForMultiEndpointPowerNet(t *testing.T) {
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
	if len(result.Labels) != 3 {
		t.Fatalf("labels = %#v, want one per endpoint", result.Labels)
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
