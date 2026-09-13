package schematiclayout

import (
	"encoding/json"
	"math"
	"reflect"
	"slices"
	"sync"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestFunctionalSupportLocality(t *testing.T) {
	components := []Component{{Ref: "U", Role: "ic"}, {Ref: "C1", Role: "decoupling_capacitor"}, {Ref: "C2", Role: "decoupling_capacitor"}, {Ref: "C3", Role: "decoupling_capacitor"}, {Ref: "C4", Role: "decoupling_capacitor"}, {Ref: "J", Role: "connector", SupportParent: "U"}}
	positions := map[string]kicadfiles.Point{}
	for i, c := range components {
		positions[c.Ref] = kicadfiles.Point{X: kicadfiles.MM(float64(i) * 100), Y: kicadfiles.MM(50)}
	}
	before, _ := json.Marshal(positions)
	rules := DefaultRules(ProfileStandard)
	a := functionalSupportPositions(components, positions, rules)
	for _, c := range components[1:] {
		p, q := a[c.Ref], a["U"]
		if math.Hypot(float64(p.X-q.X), float64(p.Y-q.Y)) >= float64(kicadfiles.MM(90)) {
			t.Fatal("support left remote", c.Ref, a)
		}
	}
	if _, _, overlap := firstPlacementOverlap(components, a); overlap {
		t.Fatal("support overlaps body")
	}
	slices.Reverse(components)
	if b := functionalSupportPositions(components, positions, rules); !reflect.DeepEqual(a, b) {
		t.Fatal("permutation changed support placement")
	}
	after, _ := json.Marshal(positions)
	if string(before) != string(after) {
		t.Fatal("caller positions mutated")
	}
}

func TestFunctionalSupportConcurrentExplicitParents(t *testing.T) {
	components := []Component{{Ref: "a", Role: "ic"}, {Ref: "b", Role: "ic"}, {Ref: "cap", Role: "decoupling_capacitor", SupportParent: "b"}, {Ref: "child", Role: "connector", SupportParent: "cap"}}
	positions := map[string]kicadfiles.Point{"a": {}, "b": {X: kicadfiles.MM(200)}, "cap": {X: kicadfiles.MM(400)}, "child": {X: kicadfiles.MM(600)}}
	rules := DefaultRules(ProfileStandard)
	want := functionalSupportPositions(components, positions, rules)
	if absIU(want["cap"].X-want["b"].X) >= absIU(want["cap"].X-want["a"].X) || want["child"] == positions["child"] {
		t.Fatal("explicit multi-active or chained support association ignored")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if got := functionalSupportPositions(components, positions, rules); !reflect.DeepEqual(got, want) {
				t.Error("concurrent identical input changed placement")
			}
		})
	}
	wg.Wait()
}

func TestFunctionalSupportAmbiguityAndCycles(t *testing.T) {
	rules := DefaultRules(ProfileStandard)
	positions := map[string]kicadfiles.Point{"a": {}, "b": {X: kicadfiles.MM(100)}, "c": {X: kicadfiles.MM(200)}}
	for _, components := range [][]Component{
		{{Ref: "a", Role: "ic"}, {Ref: "b", Role: "ic"}, {Ref: "c", Role: "decoupling_capacitor"}},
		{{Ref: "a", SupportParent: "b"}, {Ref: "b", SupportParent: "a"}, {Ref: "c"}},
	} {
		if got := functionalSupportPositions(components, positions, rules); !reflect.DeepEqual(got, positions) {
			t.Fatal("ambiguous/cyclic ownership moved symbols", got)
		}
	}
}

func TestFunctionalSupportFixedAndLegacy(t *testing.T) {
	r := Request{Sheet: testSheet(), Rules: DefaultRules(ProfileStandard), FunctionalGroups: true,
		Groups: []Group{{ID: "a"}}, Components: []Component{{Ref: "U", Role: "ic", GroupID: "a"}, {Ref: "C", Role: "decoupling_capacitor", GroupID: "a"}}}
	legacy := placedPositions(Place(r).Components)
	r.Components[1].SupportParent = "U"
	if !reflect.DeepEqual(legacy, placedPositions(Place(r).Components)) {
		t.Fatal("support metadata changed ownership-v1")
	}
	r.Components[0].Fixed, r.Components[0].Position = true, kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)}
	before := placedPositions(Place(r).Components)
	r.FunctionalLocality = true
	if !reflect.DeepEqual(before, placedPositions(Place(r).Components)) {
		t.Fatal("locality overrode fixed-placement request")
	}
}
