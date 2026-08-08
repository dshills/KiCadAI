package designworkflow

import (
	"fmt"
	"reflect"
	"testing"

	"kicadai/internal/placement"
)

func TestExplicitInitialPlacementRotationsIncludeEdgeConstrainedParts(t *testing.T) {
	ordinary := ExplicitComponentSpec{ID: "ordinary"}
	edge := ExplicitComponentSpec{ID: "edge", Placement: ExplicitPlacementSpec{Edge: "bottom"}}
	thermal := ExplicitComponentSpec{ID: "thermal", Placement: ExplicitPlacementSpec{ThermalEdgeRequired: true}}
	if got := explicitInitialPlacementRotations(ordinary); !reflect.DeepEqual(got, []float64{0, 90}) {
		t.Fatalf("ordinary rotations = %v", got)
	}
	for _, component := range []ExplicitComponentSpec{edge, thermal} {
		if got := explicitInitialPlacementRotations(component); !reflect.DeepEqual(got, []float64{0, 90}) {
			t.Fatalf("%s rotations = %v", component.ID, got)
		}
	}
}

func TestExplicitRightAnglePlacementFallbackAlignsMovableFootprintsToBoard(t *testing.T) {
	fixedRotation := 15.0
	request := placement.Request{
		Board: placement.BoardPlacementArea{WidthMM: 80, HeightMM: 60},
		Rules: placement.Rules{AllowBackLayer: true},
		Components: []placement.Component{
			{
				Ref: "J1", Edge: placement.EdgeLeft,
				Rotation: placement.RotationConstraint{AllowedDeg: []float64{0}},
			},
			{
				Ref: "U1", Fixed: true,
				Rotation: placement.RotationConstraint{FixedDeg: &fixedRotation},
			},
			{
				Ref: "U2", Bounds: placement.Bounds{WidthMM: 22, HeightMM: 34},
				Pads:     []placement.PadSummary{{Name: "1", Type: "smd"}},
				Rotation: placement.RotationConstraint{AllowedDeg: []float64{0}},
			},
			{
				Ref: "R1", Bounds: placement.Bounds{WidthMM: 30, HeightMM: 6},
				Pads:     []placement.PadSummary{{Name: "1", Type: "thru_hole"}},
				Rotation: placement.RotationConstraint{AllowedDeg: []float64{0}},
			},
		},
	}

	got := explicitRightAnglePlacementFallback(request)
	if got.ComponentOrder != placement.ComponentOrderDenseLargestFootprintFirstV1 {
		t.Fatalf("fallback component order = %q", got.ComponentOrder)
	}
	if len(got.Components) != 4 {
		t.Fatalf("fallback component count = %d, want 4", len(got.Components))
	}
	if len(got.Components[0].Rotation.AllowedDeg) != 1 || got.Components[0].Rotation.AllowedDeg[0] != 0 {
		t.Fatalf("edge component rotation changed: %#v", got.Components[0].Rotation)
	}
	if got.Components[1].Rotation.FixedDeg == nil || *got.Components[1].Rotation.FixedDeg != fixedRotation {
		t.Fatalf("fixed component rotation changed: %#v", got.Components[1].Rotation)
	}
	if len(got.Components[2].Rotation.AllowedDeg) != 1 || got.Components[2].Rotation.AllowedDeg[0] != 90 {
		t.Fatalf("tall movable component was not aligned to the wide board: %#v", got.Components[2].Rotation)
	}
	if got.Components[2].Side != placement.SideAny {
		t.Fatalf("movable SMD component side = %q, want either side", got.Components[2].Side)
	}
	if len(got.Components[3].Rotation.AllowedDeg) != 1 || got.Components[3].Rotation.AllowedDeg[0] != 0 {
		t.Fatalf("wide movable component was rotated away from the wide board: %#v", got.Components[3].Rotation)
	}
	if got.Components[3].Side == placement.SideAny {
		t.Fatalf("through-hole component was allowed to overlap on the back side: %#v", got.Components[3])
	}
	if len(request.Components[2].Rotation.AllowedDeg) != 1 {
		t.Fatalf("fallback mutated original request: %#v", request.Components[2].Rotation)
	}
}

func TestExplicitOverlappingRegionFallbackRelaxesOnlyOverlappingSynthesizedRules(t *testing.T) {
	request := placement.Request{
		Rules: placement.Rules{AllowBackLayer: true},
		Components: []placement.Component{
			{Ref: "U1", Side: placement.SideTop, Pads: []placement.PadSummary{{Name: "1", Type: "smd"}}},
			{Ref: "D1", Edge: placement.EdgeTop, Side: placement.SideTop, Pads: []placement.PadSummary{{Name: "1", Type: "smd"}}},
			{Ref: "J1", Side: placement.SideTop, Pads: []placement.PadSummary{{Name: "1", Type: "thru_hole"}}},
		},
		RegionRules: []placement.RegionRule{
			{ID: "a", Source: "circuit_graph", Required: true, Preferred: placement.Rect{Min: placement.Point{XMM: 0, YMM: 0}, Max: placement.Point{XMM: 15, YMM: 20}}},
			{ID: "b", Source: "circuit_graph", Required: true, Preferred: placement.Rect{Min: placement.Point{XMM: 10, YMM: 0}, Max: placement.Point{XMM: 25, YMM: 20}}},
			{ID: "c", Source: "circuit_graph", Required: true, Preferred: placement.Rect{Min: placement.Point{XMM: 30, YMM: 0}, Max: placement.Point{XMM: 40, YMM: 20}}},
			{ID: "authored", Source: "user", Required: true, Preferred: placement.Rect{Min: placement.Point{XMM: 5, YMM: 0}, Max: placement.Point{XMM: 35, YMM: 20}}},
		}}
	got, available := explicitOverlappingRegionPlacementFallback(request)
	if !available || got.RegionRules[0].Required || got.RegionRules[1].Required ||
		!got.RegionRules[2].Required || !got.RegionRules[3].Required ||
		got.ComponentOrder != placement.ComponentOrderDenseLargestFootprintFirstV1 ||
		got.Rules.MaxCandidatesPerPart != explicitDensePlacementCandidateLimit {
		t.Fatalf("overlapping-region fallback = %#v available=%t", got.RegionRules, available)
	}
	if got.Components[0].Side != placement.SideAny || got.Components[1].Side != placement.SideBottom ||
		got.Components[2].Side != placement.SideTop {
		t.Fatalf("overlapping-region fallback component sides = %#v", got.Components)
	}
	for _, rule := range request.RegionRules {
		if !rule.Required {
			t.Fatalf("fallback mutated original request: %#v", request.RegionRules)
		}
	}
	if request.Components[0].Side != placement.SideTop {
		t.Fatalf("fallback mutated original components: %#v", request.Components)
	}
}

func TestExplicitDenseInteriorPlacementRegionDerivesCoreFromEdgesAndCourtyards(t *testing.T) {
	request := placement.Request{
		Board: placement.BoardPlacementArea{WidthMM: 80, HeightMM: 60, MarginMM: 1},
		Rules: placement.Rules{ComponentSpacingMM: .5},
		Components: []placement.Component{
			{Ref: "J1", Edge: placement.EdgeLeft, Side: placement.SideTop, Bounds: placement.Bounds{WidthMM: 3.6, HeightMM: 6.2}, Rotation: placement.RotationConstraint{AllowedDeg: []float64{0, 90}}},
			{Ref: "J2", Edge: placement.EdgeRight, Side: placement.SideTop, Bounds: placement.Bounds{WidthMM: 3.6, HeightMM: 6.2}, Rotation: placement.RotationConstraint{AllowedDeg: []float64{0, 90}}},
			{Ref: "Q1", Edge: placement.EdgeBottom, Side: placement.SideAny, Bounds: placement.Bounds{WidthMM: 10, HeightMM: 5}, Rotation: placement.RotationConstraint{AllowedDeg: []float64{0, 90}}},
		},
	}
	for _, ref := range []string{"U1", "U2", "U3", "U4"} {
		request.Components = append(request.Components, placement.Component{
			Ref: ref, Side: placement.SideTop,
			Bounds:   placement.Bounds{WidthMM: 21.8, HeightMM: 33.3},
			Rotation: placement.RotationConstraint{AllowedDeg: []float64{0, 90}},
		})
	}
	explicitOrientDenseEdgeComponents(&request)
	for _, component := range request.Components[:3] {
		if !reflect.DeepEqual(component.Rotation.AllowedDeg, []float64{0}) {
			t.Fatalf("dense edge orientation for %s = %#v", component.Ref, component.Rotation)
		}
	}
	rule, found := explicitDenseInteriorPlacementRegion(request)
	if !found || !rule.Required || rule.Source != "derived_package_floorplan" ||
		!reflect.DeepEqual(rule.Refs, []string{"U1", "U2", "U3", "U4"}) {
		t.Fatalf("dense interior rule = %#v found=%t", rule, found)
	}
	if rule.Preferred.Min.XMM < 5.1 || rule.Preferred.Max.XMM > 74.9 ||
		rule.Preferred.Max.YMM-rule.Preferred.Min.YMM < 44.6 {
		t.Fatalf("dense interior bounds do not preserve edge/core capacity: %#v", rule.Preferred)
	}
	explicitOrientDenseInteriorComponents(&request, rule.Refs)
	for _, component := range request.Components[3:] {
		if !reflect.DeepEqual(component.Rotation.AllowedDeg, []float64{90}) {
			t.Fatalf("dense interior orientation for %s = %#v", component.Ref, component.Rotation)
		}
	}
	slots := explicitDenseInteriorPlacementSlots(request, rule)
	if len(slots) != 4 {
		t.Fatalf("dense interior slots = %#v", slots)
	}
	for index, slot := range slots {
		if !slot.Required || slot.Source != "derived_package_floorplan" ||
			!reflect.DeepEqual(slot.Refs, []string{fmt.Sprintf("U%d", index+1)}) {
			t.Fatalf("dense interior slot %d = %#v", index, slot)
		}
		if slot.Preferred.Min != slot.Preferred.Max || !rule.Preferred.ContainsPoint(slot.Preferred.Min) {
			t.Fatalf("dense interior slot %d is not an exact in-region anchor: %#v", index, slot.Preferred)
		}
		if index > 0 && slot.Preferred.Min == slots[index-1].Preferred.Min {
			t.Fatalf("dense interior slot %d reuses the prior anchor: %#v", index, slot.Preferred)
		}
	}
}

func TestExplicitOverlappingRegionFallbackPacksDenseFrontOnlyCohort(t *testing.T) {
	rules := placement.DefaultRules()
	rules.AllowBackLayer = true
	rules.GridMM = .5
	rules.ComponentSpacingMM = .5
	request := placement.Request{
		Board: placement.BoardPlacementArea{WidthMM: 80, HeightMM: 60, MarginMM: .25, Layers: 4},
		Rules: rules,
	}
	for index := 1; index <= 3; index++ {
		request.Components = append(request.Components, placement.Component{
			Ref: fmt.Sprintf("JL%d", index), Edge: placement.EdgeLeft, Side: placement.SideTop,
			Bounds:   placement.Bounds{WidthMM: 3.69, HeightMM: 6.24},
			Pads:     []placement.PadSummary{{Name: "1", Type: "thru_hole"}},
			Rotation: placement.RotationConstraint{AllowedDeg: []float64{0, 90}},
		})
	}
	request.Components = append(request.Components, placement.Component{
		Ref: "JR1", Edge: placement.EdgeRight, Side: placement.SideTop,
		Bounds:   placement.Bounds{WidthMM: 3.69, HeightMM: 6.24},
		Pads:     []placement.PadSummary{{Name: "1", Type: "thru_hole"}},
		Rotation: placement.RotationConstraint{AllowedDeg: []float64{0, 90}},
	})
	for index, edge := range []placement.EdgeConstraint{placement.EdgeBottom, placement.EdgeTop, placement.EdgeTop} {
		request.Components = append(request.Components, placement.Component{
			Ref: fmt.Sprintf("EDGE%d", index+1), Edge: edge, Side: placement.SideTop,
			Bounds:   placement.Bounds{WidthMM: 11.25, HeightMM: 7.15},
			Pads:     []placement.PadSummary{{Name: "1", Type: "smd"}},
			Rotation: placement.RotationConstraint{AllowedDeg: []float64{0, 90}},
		})
	}
	for index := 1; index <= 4; index++ {
		request.Components = append(request.Components, placement.Component{
			Ref: fmt.Sprintf("MODULE%d", index), Side: placement.SideTop,
			Bounds:   placement.Bounds{WidthMM: 21.8, HeightMM: 33.3},
			Pads:     []placement.PadSummary{{Name: "1", Type: "thru_hole"}},
			Rotation: placement.RotationConstraint{AllowedDeg: []float64{0, 90}},
		})
	}
	for index := 1; index <= 2; index++ {
		request.Components = append(request.Components, placement.Component{
			Ref: fmt.Sprintf("BALLAST%d", index), Side: placement.SideTop,
			Bounds:   placement.Bounds{WidthMM: 28.85, HeightMM: 6.35},
			Pads:     []placement.PadSummary{{Name: "1", Type: "thru_hole"}},
			Rotation: placement.RotationConstraint{AllowedDeg: []float64{0, 90}},
		})
	}
	for index := 1; index <= 14; index++ {
		request.Components = append(request.Components, placement.Component{
			Ref: fmt.Sprintf("SMD%d", index), Side: placement.SideTop,
			Bounds:   placement.Bounds{WidthMM: 4, HeightMM: 2.5},
			Pads:     []placement.PadSummary{{Name: "1", Type: "smd"}},
			Rotation: placement.RotationConstraint{AllowedDeg: []float64{0, 90}},
		})
	}
	refs := make([]string, 0, len(request.Components))
	for _, component := range request.Components {
		refs = append(refs, component.Ref)
	}
	request.RegionRules = []placement.RegionRule{
		{ID: "left", Region: "left", Source: "circuit_graph", Refs: refs, Required: true, Preferred: placement.Rect{Min: placement.Point{}, Max: placement.Point{XMM: 55, YMM: 60}}},
		{ID: "right", Region: "right", Source: "circuit_graph", Refs: refs, Required: true, Preferred: placement.Rect{Min: placement.Point{XMM: 25}, Max: placement.Point{XMM: 80, YMM: 60}}},
	}
	dense, available := explicitOverlappingRegionPlacementFallback(request)
	if !available || len(dense.RegionRules) != 7 {
		t.Fatalf("dense fallback = %#v available=%t", dense.RegionRules, available)
	}
	result := placement.Place(dense)
	if result.Metrics.PlacedCount != len(request.Components) || result.Metrics.UnplacedCount != 0 ||
		result.Metrics.CollisionCount != 0 || result.Metrics.OutsideOutlineCount != 0 {
		t.Fatalf("dense explicit cohort placement = %s metrics=%#v issues=%#v placements=%#v", result.Status, result.Metrics, result.Issues, result.Placements)
	}
}

func TestExplicitThermalEdgeRuleDoesNotRequireFunctionalRegionContainment(t *testing.T) {
	component := ExplicitComponentSpec{
		ID: "power_device", Reference: "Q1",
		Placement: ExplicitPlacementSpec{
			Region: "functional_power", Edge: "bottom", ThermalRole: "power_switch",
			ThermalPathID: "reviewed_sink", ThermalEdgeRequired: true,
		},
	}
	rule, ok := explicitThermalPlacementRule(component)
	if !ok || rule.PreferredRegion != "" || rule.PreferredEdge != placement.EdgeBottom {
		t.Fatalf("board-edge thermal rule = %#v ok=%t", rule, ok)
	}
	component.Placement.ThermalEdgeRequired = false
	rule, ok = explicitThermalPlacementRule(component)
	if !ok || rule.PreferredRegion != "functional_power" {
		t.Fatalf("intrinsic thermal rule lost its functional region: %#v ok=%t", rule, ok)
	}
}
