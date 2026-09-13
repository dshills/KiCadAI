package schematiclayout

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestFunctionalLocalWiringGroupTreesAndLegacy(t *testing.T) {
	r := Request{FunctionalLocalWiring: true, Sheet: testSheet(), Rules: DefaultRules(ProfileStandard)}
	net := Net{Name: "RAIL", Role: "power", LocalWiring: true, EndpointLabels: true, PreferredLabels: true}
	var placed []PlacedComponent
	for i, ref := range []string{"a", "b", "c", "d"} {
		group := "first"
		if i >= 2 {
			group = "second"
		}
		c := Component{Ref: ref, GroupID: group, Pins: []Pin{{Number: "1"}}}
		r.Components = append(r.Components, c)
		placed = append(placed, PlacedComponent{Component: c, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(float64(20 + 25*i)), Y: kicadfiles.MM(30)}})
		net.Endpoints = append(net.Endpoints, Endpoint{Ref: ref, Pin: "1"})
	}
	r.Nets = []Net{net}
	got := Route(r, Result{Sheet: r.Sheet, Components: placed})
	direct, labels := 0, 0
	for _, connection := range got.Connections {
		if connection.UseLabels {
			labels++
		} else {
			direct++
			if !pathOrthogonal(connection.Points) {
				t.Fatal("nonorthogonal local conductor")
			}
		}
	}
	if direct != 2 || labels != 1 || len(got.Connections) != len(net.Endpoints)-1 {
		t.Fatalf("direct=%d labels=%d connections=%v", direct, labels, got.Connections)
	}
	slices.Reverse(r.Components)
	slices.Reverse(r.Nets[0].Endpoints)
	slices.Reverse(placed)
	shuffled := Route(r, Result{Sheet: r.Sheet, Components: placed})
	if !reflect.DeepEqual(got.Connections, shuffled.Connections) || !reflect.DeepEqual(got.Wires, shuffled.Wires) {
		t.Fatal("input order changed local tree")
	}
	r.FunctionalLocalWiring = false
	legacy := Route(r, Result{Sheet: r.Sheet, Components: placed})
	for _, connection := range legacy.Connections {
		if !connection.UseLabels {
			t.Fatal("legacy endpoint-label behavior changed")
		}
	}
}

func TestFunctionalLocalWiringCorridorTransformAndIsolation(t *testing.T) {
	owner := Component{Ref: "owner", Pins: []Pin{{Number: "1", At: kicadfiles.Point{X: kicadfiles.MM(5)}, Direction: kicadfiles.Point{X: 1}}}}
	net := Net{Name: strings.Repeat("LONG", 10), Role: "power", Endpoints: []Endpoint{{Ref: "owner", Pin: "1"}, {Ref: "support", Pin: "1"}}}
	for _, angle := range []kicadfiles.Angle{0, 90, 180, 270} {
		for _, mirror := range []Mirror{MirrorNone, MirrorX, MirrorY} {
			owner.Rotation, owner.Mirror = angle, mirror
			before := supportOwnerPinCorridors(owner, []Net{net}, kicadfiles.Point{})
			local := net
			local.LocalWiring = true
			after := supportOwnerPinCorridors(owner, []Net{local}, kicadfiles.Point{})
			if len(before) != 1 || len(after) != 1 || after[0].Width()+after[0].Height() >= before[0].Width()+before[0].Height() {
				t.Fatal("local conductor did not reduce only its annotation reservation", angle, mirror)
			}
			if !reflect.DeepEqual(before, supportOwnerPinCorridors(owner, []Net{net}, kicadfiles.Point{})) {
				t.Fatal("legacy corridor mutated")
			}
			for _, role := range []string{"signal", "bias", "ground"} {
				conservative := local
				conservative.Role = role
				if !reflect.DeepEqual(before, supportOwnerPinCorridors(owner, []Net{conservative}, kicadfiles.Point{})) {
					t.Fatal("non-supply label corridor shortened", role)
				}
			}
			singleton := local
			singleton.Endpoints = singleton.Endpoints[:1]
			if !reflect.DeepEqual(before, supportOwnerPinCorridors(owner, []Net{singleton}, kicadfiles.Point{})) {
				t.Fatal("cross-group singleton lost full annotation clearance")
			}
		}
	}
}

func TestFunctionalLocalWiringRejectsForeignLabelCorridor(t *testing.T) {
	r := Request{Sheet: testSheet(), functionalLabelCorridors: []functionalLabelCorridor{{net: "OTHER", box: Rect{MinX: kicadfiles.MM(29), MinY: kicadfiles.MM(10), MaxX: kicadfiles.MM(31), MaxY: kicadfiles.MM(50)}}}}
	points := []kicadfiles.Point{{X: kicadfiles.MM(20), Y: kicadfiles.MM(30)}, {X: kicadfiles.MM(40), Y: kicadfiles.MM(30)}}
	if _, clean := scoreRoute(points, "SIGNAL", Endpoint{}, Endpoint{}, Result{}, r); clean {
		t.Fatal("wire trapped a future foreign label")
	}
	if _, clean := scoreRoute(points, "OTHER", Endpoint{}, Endpoint{}, Result{}, r); !clean {
		t.Fatal("same-net conductor blocked by its annotation corridor")
	}
	r.functionalLabelCorridors = nil
	if _, clean := scoreRoute(points, "SIGNAL", Endpoint{}, Endpoint{}, Result{}, r); !clean {
		t.Fatal("legacy scoring changed")
	}
}

func TestFunctionalLocalWiringObstructionFallsBackWithoutDirtyWire(t *testing.T) {
	point := func(x float64) kicadfiles.Point { return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(30)} }
	r := Request{Sheet: testSheet(), Components: []Component{{Ref: "a", GroupID: "group"}, {Ref: "b", GroupID: "group"}}, functionalLabelCorridors: []functionalLabelCorridor{{net: "OTHER", box: Rect{MinX: 0, MinY: 0, MaxX: kicadfiles.MM(300), MaxY: kicadfiles.MM(300)}}}}
	endpoints := []routableEndpoint{{endpoint: Endpoint{Ref: "a", Pin: "1"}, anchor: point(20)}, {endpoint: Endpoint{Ref: "b", Pin: "1"}, anchor: point(40)}}
	net := Net{Name: "SIGNAL", Endpoints: []Endpoint{endpoints[0].endpoint, endpoints[1].endpoint}}
	result := Result{Sheet: r.Sheet}
	routeFunctionalLocalTrees(&result, map[string]kicadfiles.Point{}, net, endpoints, r, DefaultRules(ProfileStandard), newPinAnchorIndex(nil))
	if len(result.Connections) != 1 || !result.Connections[0].UseLabels || len(result.Connections[0].Points) != 0 || !hasDiagnosticCode(result.Diagnostics, "functional_local_route_fallback") {
		t.Fatal("failed local routing emitted a dirty conductor", result.Connections, result.Diagnostics)
	}
}

func TestFunctionalLocalWiringBalancesFourGroups(t *testing.T) {
	r := Request{FunctionalLocalWiring: true}
	for _, id := range []string{"a", "b", "c", "d"} {
		r.Groups = append(r.Groups, Group{ID: id, OriginalOrdinal: len(r.Groups)})
		r.Components = append(r.Components, Component{Ref: id, GroupID: id, Role: "ic", BodyKnown: true, Body: Rect{MinX: kicadfiles.MM(-110), MinY: kicadfiles.MM(-10), MaxX: kicadfiles.MM(110), MaxY: kicadfiles.MM(10)}})
	}
	positions := functionalGroupPositions(r, DefaultRules(ProfileStandard))
	if positions["a"].Y != positions["b"].Y || positions["c"].Y != positions["d"].Y || positions["c"].Y <= positions["a"].Y {
		t.Fatal("four explicit groups were not balanced into two rows", positions)
	}
	slices.Reverse(r.Groups)
	slices.Reverse(r.Components)
	if !reflect.DeepEqual(positions, functionalGroupPositions(r, DefaultRules(ProfileStandard))) {
		t.Fatal("input ordering changed group packing")
	}
	r.FunctionalLocalWiring = false
	legacy := functionalGroupPositions(r, DefaultRules(ProfileStandard))
	if legacy["a"].Y == legacy["b"].Y {
		t.Fatal("legacy fixed-width packing changed")
	}
}
