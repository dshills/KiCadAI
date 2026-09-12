package schematiclayout

import (
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestFunctionalJointRoleNormalizationAndLegacy(t *testing.T) {
	u := Component{Ref: "owner", Pins: []Pin{{Number: "supply", At: kicadfiles.Point{X: -100}, Direction: kicadfiles.Point{X: -1}}, {Number: "signal", At: kicadfiles.Point{X: 100}, Direction: kicadfiles.Point{X: 1}}}}
	net := func(role, pin string) Net {
		return Net{Name: "unchanged", Role: role, Endpoints: []Endpoint{{Ref: "support", Pin: "1"}, {Ref: "owner", Pin: pin}}}
	}
	for _, role := range []string{"power", "power_pos", "power_neg"} {
		for _, ground := range []string{"ground", "return"} {
			for _, transform := range []struct {
				angle  kicadfiles.Angle
				mirror Mirror
				want   string
			}{{0, MirrorNone, "right"}, {90, MirrorNone, "top"}, {0, MirrorY, "left"}} {
				u.Rotation, u.Mirror = transform.angle, transform.mirror
				nets := []Net{net(role, "supply"), net("bias", "signal"), net(ground, "supply")}
				before := slices.Clone(nets)
				for range 2 {
					if got := supportAttachmentSide("support", u, jointCanonicalNetRoles(nets)); got != transform.want {
						t.Fatalf("%s/%s side=%s want=%s", role, ground, got, transform.want)
					}
					slices.Reverse(nets)
				}
				if !reflect.DeepEqual(nets, before) {
					t.Fatal("normalization mutated electrical metadata")
				}
			}
		}
	}
	u.Rotation, u.Mirror = 0, MirrorNone
	variant := []Net{net("power_pos", "supply"), net("bias", "signal")}
	if got := supportAttachmentSide("support", u, variant); got != "ambiguous" {
		t.Fatal("legacy role handling changed", got)
	}
	if got := supportAttachmentSide("support", u, jointCanonicalNetRoles([]Net{net("return", "supply")})); got != "" {
		t.Fatal("return selected a side", got)
	}
	if got := supportAttachmentSide("support", u, jointCanonicalNetRoles([]Net{net("power_neg", "supply"), net("power_pos", "signal")})); got != "ambiguous" {
		t.Fatal("equal supply priority conflict lost", got)
	}
}

func TestFunctionalJointReservesSupportCorridors(t *testing.T) {
	p := func(x, y float64) kicadfiles.Point { return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)} }
	u := Component{Ref: "owner", Role: "ic", BodyKnown: true, Body: Rect{MinX: kicadfiles.MM(-5), MinY: kicadfiles.MM(-5), MaxX: kicadfiles.MM(5), MaxY: kicadfiles.MM(5)}, Pins: []Pin{{Number: "1", At: p(10, 0), Direction: p(1, 0)}}}
	j := Component{Ref: "support", Role: "connector", SupportParent: "owner", BodyKnown: true, Body: Rect{MinX: kicadfiles.MM(-2), MinY: kicadfiles.MM(-2), MaxX: kicadfiles.MM(2), MaxY: kicadfiles.MM(2)}, Pins: []Pin{{Number: "1", At: p(-5, 0), Direction: p(-1, 0)}}}
	nets := []Net{{Name: "LONG_EXACT_ELECTRICAL_INTERFACE_NAME", Role: "bias", Endpoints: []Endpoint{{Ref: u.Ref, Pin: "1"}, {Ref: j.Ref, Pin: "1"}}}}
	parts := []Component{u, j}
	original := map[string]kicadfiles.Point{u.Ref: p(100, 100), j.Ref: p(300, 300)}
	legacy := functionalSupportPositionsWithPins(parts, nets, original, DefaultRules(ProfileStandard), true)
	byRef := map[string]Component{u.Ref: u, j.Ref: j}
	placed := map[string]bool{u.Ref: true}
	gap := kicadfiles.MM(17.78) / 2
	if jointSupportClear(j, legacy[j.Ref], byRef, legacy, placed, nets, gap) {
		t.Fatal("fixture must reproduce body-only clearance missing the support label envelope")
	}
	joint := functionalSupportPositionsJoint(parts, nets, original, DefaultRules(ProfileStandard), true, true)
	if !jointSupportClear(j, joint[j.Ref], byRef, joint, placed, nets, gap) {
		t.Fatal("joint corridor conflict remains", joint)
	}
	if !supportOnAttachmentSide(componentBoundsAt(j, joint[j.Ref]), componentBoundsAt(u, joint[u.Ref]), "right") {
		t.Fatal("attachment side changed")
	}
	slices.Reverse(parts)
	slices.Reverse(nets[0].Endpoints)
	if !reflect.DeepEqual(joint, functionalSupportPositionsJoint(parts, nets, original, DefaultRules(ProfileStandard), true, true)) {
		t.Fatal("shuffled input changed placement")
	}
	if !reflect.DeepEqual(legacy, functionalSupportPositionsWithPins(parts, nets, original, DefaultRules(ProfileStandard), true)) {
		t.Fatal("legacy projection changed")
	}
}
