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
	if len(result.Wires) != 3 {
		t.Fatalf("wire count = %d, want 3", len(result.Wires))
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
	if len(result.Labels) != 2 || len(result.Wires) != 0 {
		t.Fatalf("labels=%#v wires=%#v, want label fallback", result.Labels, result.Wires)
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
