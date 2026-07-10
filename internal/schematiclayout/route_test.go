package schematiclayout

import (
	"testing"

	"kicadai/internal/kicadfiles"
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
		{Component: Component{Ref: "BLOCK", Pins: []Pin{{Number: "1", At: kicadfiles.Point{X: kicadfiles.MM(10)}}}}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(20)}},
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
