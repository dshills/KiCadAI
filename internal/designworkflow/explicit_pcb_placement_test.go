package designworkflow

import (
	"testing"

	"kicadai/internal/placement"
)

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
