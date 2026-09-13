package schematiclayout

import (
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestFunctionalPowerLocalityPinAxisTransformsAndIsolation(t *testing.T) {
	p := func(x, y float64) kicadfiles.Point { return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)} }
	u := Component{Ref: "arbitrary-active", Role: "regulator", Pins: []Pin{{Number: "rail", At: p(5, 0), Direction: p(1, 0)}}}
	c := Component{Ref: "arbitrary-support", Role: "decoupling", Pins: []Pin{{Number: "supply", At: p(0, -4)}}}
	net := Net{Name: "EXACT_NET", Role: "power", LocalWiring: true, Endpoints: []Endpoint{{Ref: u.Ref, Pin: "rail"}, {Ref: c.Ref, Pin: "supply"}}}
	for _, angle := range []kicadfiles.Angle{0, 90, 180, 270} {
		for _, mirror := range []Mirror{MirrorNone, MirrorX, MirrorY} {
			u.Rotation, u.Mirror, c.Rotation, c.Mirror = angle, mirror, angle, mirror
			aligned := TransformPoint(p(30, 4), angle, mirror)
			nearer := TransformPoint(p(25, -10), angle, mirror)
			offsets := []kicadfiles.Point{nearer, aligned}
			before := slices.Clone(offsets)
			got := powerSupportOffsets(c, u, []Net{net}, offsets)
			if got[0] != aligned || !reflect.DeepEqual(offsets, before) {
				t.Fatal("pin axis preference or input preservation failed", angle, mirror, got)
			}
			for _, role := range []string{"signal", "ground", "bias"} {
				n := net
				n.Role = role
				if !reflect.DeepEqual(powerSupportOffsets(c, u, []Net{n}, offsets), before) {
					t.Fatal("non-power candidate order changed", role)
				}
			}
			n := net
			n.LocalWiring = false
			if !reflect.DeepEqual(powerSupportOffsets(c, u, []Net{n}, offsets), before) {
				t.Fatal("explicit label-only rail changed")
			}
			other := c
			other.Role = "connector"
			if !reflect.DeepEqual(powerSupportOffsets(other, u, []Net{net}, offsets), before) {
				t.Fatal("non-capacitor support changed")
			}
			ambiguous := u
			ambiguous.Pins = append(slices.Clone(u.Pins), Pin{Number: "other", At: p(-5, 0), Direction: p(-1, 0)})
			n = net
			n.Endpoints = append(slices.Clone(net.Endpoints), Endpoint{Ref: u.Ref, Pin: "other"})
			if !reflect.DeepEqual(powerSupportOffsets(c, ambiguous, []Net{n}, offsets), before) {
				t.Fatal("ambiguous owner rail gained a fabricated axis")
			}
		}
	}
}

func TestFunctionalPowerLocalitySharedCorridorsAndLegacy(t *testing.T) {
	p := func(x, y float64) kicadfiles.Point { return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)} }
	u := Component{Ref: "owner", BodyKnown: true, Body: Rect{MinX: -1, MinY: -1, MaxX: 1, MaxY: 1}, Pins: []Pin{{Number: "1", At: p(5, 0), Direction: p(1, 0)}}}
	c := Component{Ref: "support", BodyKnown: true, Body: u.Body, Pins: []Pin{{Number: "1", At: p(-5, 0), Direction: p(-1, 0)}}}
	n := Net{Name: "RAIL", Role: "power", LocalWiring: true, Endpoints: []Endpoint{{Ref: u.Ref, Pin: "1"}, {Ref: c.Ref, Pin: "1"}}}
	parts := map[string]Component{u.Ref: u, c.Ref: c}
	positions := map[string]kicadfiles.Point{u.Ref: p(0, 0)}
	placed := map[string]bool{u.Ref: true}
	if jointSupportClear(c, p(25, 0), parts, positions, placed, []Net{n}, 0) {
		t.Fatal("fixture must have overlapping legacy corridors")
	}
	if !jointSupportClearPower(c, p(25, 0), parts, positions, placed, []Net{n}, 0, true) {
		t.Fatal("same-net power access corridors could not share a rail")
	}
	foreign := n
	foreign.Name, foreign.Endpoints = "FOREIGN", []Endpoint{{Ref: c.Ref, Pin: "1"}, {Ref: "outside", Pin: "1"}}
	n.Endpoints = []Endpoint{{Ref: u.Ref, Pin: "1"}, {Ref: "outside", Pin: "2"}}
	if jointSupportClearPower(c, p(25, 0), parts, positions, placed, []Net{n, foreign}, 0, true) {
		t.Fatal("foreign corridors were allowed to overlap")
	}
}

func TestFunctionalPowerLocalityRootDeterministic(t *testing.T) {
	r := Request{FunctionalLocalWiring: true, FunctionalPowerLocality: true, Sheet: testSheet(), Rules: DefaultRules(ProfileStandard)}
	net := Net{Name: "RAIL", Role: "power", LocalWiring: true}
	var placed []PlacedComponent
	for i, ref := range []string{"a-cap", "b-cap", "z-active"} {
		role := "decoupling"
		if i == 2 {
			role = "regulator"
		}
		c := Component{Ref: ref, Role: role, GroupID: "group", Pins: []Pin{{Number: "1"}}}
		r.Components = append(r.Components, c)
		placed = append(placed, PlacedComponent{Component: c, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(float64(20 + 25*i)), Y: kicadfiles.MM(30)}})
		net.Endpoints = append(net.Endpoints, Endpoint{Ref: ref, Pin: "1"})
	}
	r.Nets = []Net{net}
	got := Route(r, Result{Sheet: r.Sheet, Components: placed})
	if len(got.Connections) != 2 || !slices.ContainsFunc(got.Connections, func(c RoutedConnection) bool { return c.From.Ref == "z-active" && c.To.Ref == "a-cap" }) {
		t.Fatal("active device did not root the tree", got.Connections)
	}
	for _, connection := range got.Connections {
		if connection.UseLabels || !pathOrthogonal(connection.Points) {
			t.Fatal("invalid local tree", connection)
		}
	}
	slices.Reverse(r.Components)
	slices.Reverse(r.Nets[0].Endpoints)
	slices.Reverse(placed)
	if other := Route(r, Result{Sheet: r.Sheet, Components: placed}); !reflect.DeepEqual(got.Connections, other.Connections) {
		t.Fatal("input ordering changed routes")
	}
	r.FunctionalPowerLocality = false
	legacy := Route(r, Result{Sheet: r.Sheet, Components: placed})
	if legacy.Connections[0].From.Ref != "a-cap" {
		t.Fatal("V5 root changed", legacy.Connections)
	}
}

func TestFunctionalPowerLocalityEscapesPaddedPinEnvelope(t *testing.T) {
	p := func(x, y float64) kicadfiles.Point { return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)} }
	for _, angle := range []kicadfiles.Angle{0, 90, 180, 270} {
		for _, mirror := range []Mirror{MirrorNone, MirrorX, MirrorY} {
			u := Component{Ref: "owner", BodyKnown: true, Body: Rect{MinX: kicadfiles.MM(-10), MinY: kicadfiles.MM(-10), MaxX: kicadfiles.MM(10), MaxY: kicadfiles.MM(10)}, Rotation: angle, Mirror: mirror, Pins: []Pin{{Number: "1", At: p(7.62, 0), Direction: p(1, 0)}}}
			anchor := TransformPoint(u.Pins[0].At, angle, mirror)
			direction := TransformPoint(p(1.27, 0), angle, mirror)
			body := componentBoundsAt(u, kicadfiles.Point{})
			access := powerPinAccess(anchor, direction, body, kicadfiles.MM(1.27))
			if body.ContainsPoint(access) || !pathOrthogonal([]kicadfiles.Point{anchor, access}) {
				t.Fatal("access trapped inside padded body", angle, mirror, access)
			}
		}
	}
	u := Component{Ref: "owner", Role: "regulator", BodyKnown: true, Body: Rect{MinX: kicadfiles.MM(-10), MinY: kicadfiles.MM(-10), MaxX: kicadfiles.MM(10), MaxY: kicadfiles.MM(10)}, Pins: []Pin{{Number: "1", At: p(7.62, 0), Direction: p(1, 0)}}}
	c := Component{Ref: "support", Pins: []Pin{{Number: "1", Direction: p(0, -1)}}}
	placed := []PlacedComponent{{Component: u, PlacedAt: p(40.64, 40.64)}, {Component: c, PlacedAt: p(71.12, 50.8)}}
	n := Net{Name: "RAIL", Role: "power", LocalWiring: true, Endpoints: []Endpoint{{Ref: u.Ref, Pin: "1"}, {Ref: c.Ref, Pin: "1"}}}
	r := Request{Sheet: testSheet(), FunctionalPowerLocality: true, Nets: []Net{n}}
	result := Result{Sheet: r.Sheet, Components: placed}
	anchors := pinAnchors(placed)
	points, clean := routeConnectionPoints(n.Name, n.Endpoints[0], n.Endpoints[1], anchors[n.Endpoints[0]], anchors[n.Endpoints[1]], result, r, DefaultRules(ProfileStandard), newPinAnchorIndex(anchors), true)
	if !clean || len(points) < 3 {
		t.Fatal("padded-body power dogleg remains blocked", points, clean)
	}
	if _, ok := scoreRoute(points, "RAIL", n.Endpoints[0], n.Endpoints[1], result, r); !ok {
		t.Fatal("escape bypassed route scoring")
	}
	for _, role := range []string{"signal", "bias", "no_connect"} {
		n.Role = role
		if functionalPowerNet("RAIL", []Net{n}) {
			t.Fatal("non-power escape enabled", role)
		}
	}
}

func TestFunctionalPowerLocalityLeavesMixedAndInterfaceGroupsAlone(t *testing.T) {
	parts := []Component{{Ref: "any-owner", GroupID: "pure", Role: "regulator"}, {Ref: "any-cap", GroupID: "pure", Role: "decoupling"}, {Ref: "port", GroupID: "interfaces", Role: "connector"}}
	if !powerLocalityGroup("pure", parts) || powerLocalityGroup("interfaces", parts) || powerLocalityGroup("", parts) || powerLocalityGroup("absent", parts) {
		t.Fatal("incorrect pure-block eligibility")
	}
	parts = append(parts, Component{Ref: "programming", GroupID: "pure", Role: "connector"})
	if powerLocalityGroup("pure", parts) {
		t.Fatal("mixed MCU/interface group entered power-only routing")
	}
	slices.Reverse(parts)
	if powerLocalityGroup("pure", parts) {
		t.Fatal("eligibility depends on input order")
	}
}
