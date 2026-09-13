package schematiclayout

import (
	"reflect"
	"slices"
	"testing"
)

func TestFunctionalGroupsRemainLocalAndDeterministic(t *testing.T) {
	r := Request{Sheet: testSheet(), Rules: DefaultRules(ProfileStandard), FunctionalGroups: true,
		Groups:     []Group{{ID: "a", OriginalOrdinal: 0}, {ID: "b", OriginalOrdinal: 1}},
		Components: []Component{{Ref: "active-a", GroupID: "a", Role: "ic"}, {Ref: "cap-a", GroupID: "a", Role: "decoupling_capacitor"}, {Ref: "active-b", GroupID: "b", Role: "ic"}, {Ref: "cap-b", GroupID: "b", Role: "decoupling_capacitor"}}}
	a := Place(r)
	positions := placedPositions(a.Components)
	distance := func(a, b string) int64 {
		p, q := positions[a], positions[b]
		return int64(absIU(p.X-q.X) + absIU(p.Y-q.Y))
	}
	if distance("active-a", "cap-a") >= distance("active-a", "cap-b") || distance("active-b", "cap-b") >= distance("active-b", "cap-a") {
		t.Fatal("support not local", positions)
	}
	slices.Reverse(r.Components)
	slices.Reverse(r.Groups)
	b := Place(r)
	if !reflect.DeepEqual(positions, placedPositions(b.Components)) {
		t.Fatal("input order changed functional geometry")
	}
	for _, d := range a.Diagnostics {
		if d.Severity == SeverityError {
			t.Fatal(d)
		}
	}
	if !reflect.DeepEqual(r.Nets, []Net(nil)) {
		t.Fatal("unexpected electrical mutation")
	}
}
