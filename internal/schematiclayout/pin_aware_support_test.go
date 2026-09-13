package schematiclayout

import (
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestFunctionalPinAwareSidesAndTransforms(t *testing.T) {
	p := func(x, y float64) kicadfiles.Point { return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)} }
	owner := Component{Ref: "U", Role: "ic", BodyKnown: true, Body: Rect{MinX: kicadfiles.MM(-4), MinY: kicadfiles.MM(-4), MaxX: kicadfiles.MM(4), MaxY: kicadfiles.MM(4)}}
	for _, tc := range []struct {
		name   string
		at     kicadfiles.Point
		angle  kicadfiles.Angle
		mirror Mirror
		want   string
	}{
		{"left", p(-8, 0), 0, MirrorNone, "left"}, {"right", p(8, 0), 0, MirrorNone, "right"},
		{"top", p(0, -8), 0, MirrorNone, "top"}, {"bottom", p(0, 8), 0, MirrorNone, "bottom"},
		{"rotated", p(8, 0), 90, MirrorNone, "top"}, {"mirrored", p(8, 0), 0, MirrorY, "left"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := owner
			u.Pins = []Pin{{Number: "1", At: tc.at}}
			u.Rotation = tc.angle
			u.Mirror = tc.mirror
			nets := []Net{{Name: "opaque", Role: "power", Endpoints: []Endpoint{{Ref: "C", Pin: "1"}, {Ref: "U", Pin: "1"}}}}
			if got := supportAttachmentSide("C", u, nets); got != tc.want {
				t.Fatalf("side=%s want %s", got, tc.want)
			}
			components := []Component{u, {Ref: "C", Role: "decoupling_capacitor", SupportParent: "U"}}
			positions := map[string]kicadfiles.Point{"U": p(100, 100), "C": p(300, 300)}
			before := map[string]kicadfiles.Point{"U": p(100, 100), "C": p(300, 300)}
			got := functionalSupportPositionsWithPins(components, nets, positions, DefaultRules(ProfileStandard), true)
			if !supportOnAttachmentSide(componentBoundsAt(components[1], got["C"]), componentBoundsAt(u, got["U"]), tc.want) {
				t.Fatal("body not on pin side", got)
			}
			slices.Reverse(components)
			slices.Reverse(nets[0].Endpoints)
			if !reflect.DeepEqual(got, functionalSupportPositionsWithPins(components, nets, positions, DefaultRules(ProfileStandard), true)) {
				t.Fatal("input order changed placement")
			}
			if !reflect.DeepEqual(positions, before) {
				t.Fatal("input mutated")
			}
		})
	}
}

func TestFunctionalPinAwarePriorityAmbiguityAndFixed(t *testing.T) {
	u := Component{Ref: "U", Pins: []Pin{{Number: "1", At: kicadfiles.Point{X: kicadfiles.MM(-20)}}, {Number: "2", At: kicadfiles.Point{X: kicadfiles.MM(20)}}}}
	net := func(role, pin string) Net {
		return Net{Role: role, Endpoints: []Endpoint{{Ref: "C", Pin: "1"}, {Ref: "U", Pin: pin}}}
	}
	nets := []Net{net("power", "1"), net("bias", "2"), net("ground", "1")}
	for range 2 {
		if got := supportAttachmentSide("C", u, nets); got != "right" {
			t.Fatal(got)
		}
		slices.Reverse(nets)
	}
	if got := supportAttachmentSide("C", u, []Net{net("power", "1"), net("power", "2")}); got != "ambiguous" {
		t.Fatal(got)
	}
	if got := supportAttachmentSide("C", u, []Net{net("ground", "1")}); got != "" {
		t.Fatal(got)
	}
	r := Request{Sheet: testSheet(), Rules: DefaultRules(ProfileStandard), FunctionalGroups: true, Groups: []Group{{ID: "a"}}, Components: []Component{{Ref: "U", Role: "ic", GroupID: "a", Fixed: true, Position: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)}}, {Ref: "C", Role: "decoupling_capacitor", GroupID: "a", SupportParent: "U"}}}
	before := placedPositions(Place(r).Components)
	r.FunctionalPinAware = true
	if !reflect.DeepEqual(before, placedPositions(Place(r).Components)) {
		t.Fatal("pin-aware policy overrode fixed placement")
	}
}

func TestFunctionalPinAwareOffCenterDirection(t *testing.T) {
	u := Component{Ref: "U", Pins: []Pin{{Number: "1", At: kicadfiles.Point{X: kicadfiles.MM(-10), Y: kicadfiles.MM(-40)}, Direction: kicadfiles.Point{X: -1}}}}
	nets := []Net{{Role: "bias", Endpoints: []Endpoint{{Ref: "C", Pin: "1"}, {Ref: "U", Pin: "1"}}}}
	if got := supportAttachmentSide("C", u, nets); got != "left" {
		t.Fatal("off-center left pin misclassified", got)
	}
	u.Rotation = 90
	if got := supportAttachmentSide("C", u, nets); got != "bottom" {
		t.Fatal("direction was not rotated", got)
	}
}

func TestFunctionalPinAwareReservesNetLabelCorridors(t *testing.T) {
	u := Component{Ref: "U", Role: "ic", Pins: []Pin{{Number: "1", At: kicadfiles.Point{X: kicadfiles.MM(10)}, Direction: kicadfiles.Point{X: 1}}}}
	nets := []Net{{Name: "LONG_EXACT_ELECTRICAL_NAME_FOR_INTERFACE_PIN", Role: "bias", Endpoints: []Endpoint{{Ref: "U", Pin: "1"}, {Ref: "J", Pin: "1"}}}}
	parts := []Component{u, {Ref: "J", Role: "connector", SupportParent: "U"}}
	positions := map[string]kicadfiles.Point{"U": {X: kicadfiles.MM(100), Y: kicadfiles.MM(100)}, "J": {X: kicadfiles.MM(500)}}
	result := functionalSupportPositionsWithPins(parts, nets, positions, DefaultRules(ProfileStandard), true)
	for _, corridor := range supportOwnerPinCorridors(u, nets, result["U"]) {
		if corridor.Intersects(componentBoundsAt(parts[1], result["J"])) {
			t.Fatal("support covers owner's long pin label")
		}
	}
	if !supportOnAttachmentSide(componentBoundsAt(parts[1], result["J"]), componentBoundsAt(u, result["U"]), "right") {
		t.Fatal("corridor reservation changed attachment side")
	}
}
