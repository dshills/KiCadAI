package kicadfiles

import (
	"slices"
	"testing"
)

func TestBoardLayerForPlacementMapsFrontLayersToBackPlacement(t *testing.T) {
	tests := []struct {
		name string
		in   BoardLayer
		want BoardLayer
	}{
		{name: "copper", in: LayerFCu, want: LayerBCu},
		{name: "mask", in: LayerFMask, want: LayerBMask},
		{name: "paste", in: LayerFPaste, want: LayerBPaste},
		{name: "adhesive", in: LayerFAdhes, want: LayerBAdhes},
		{name: "silk", in: LayerFSilkS, want: LayerBSilkS},
		{name: "fab", in: LayerFFab, want: LayerBFab},
		{name: "courtyard", in: LayerFCrtYd, want: LayerBCrtYd},
		{name: "unmapped", in: BoardLayer("Dwgs.User"), want: BoardLayer("Dwgs.User")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BoardLayerForPlacement(tt.in, LayerBCu); got != tt.want {
				t.Fatalf("BoardLayerForPlacement(%q, B.Cu) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBoardTextJustifyForPlacementTogglesBottomMirror(t *testing.T) {
	front := []string{"left"}
	bottom := BoardTextJustifyForPlacement(front, LayerBCu)
	if !slices.Equal(bottom, []string{"left", "mirror"}) {
		t.Fatalf("bottom justification = %#v", bottom)
	}
	if restored := BoardTextJustifyForPlacement(bottom, LayerBCu); !slices.Equal(restored, front) {
		t.Fatalf("restored justification = %#v, want %#v", restored, front)
	}
	if got := BoardTextJustifyForPlacement([]string{"mirror"}, LayerFCu); !slices.Equal(got, []string{"mirror"}) {
		t.Fatalf("front justification changed: %#v", got)
	}
}

func TestBoardLayerForPlacementPreservesFrontPlacement(t *testing.T) {
	if got := BoardLayerForPlacement(LayerFSilkS, LayerFCu); got != LayerFSilkS {
		t.Fatalf("BoardLayerForPlacement(F.SilkS, F.Cu) = %q, want F.SilkS", got)
	}
}

func TestDefaultFootprintPropertyPositionUsesFootprintLocalCoordinates(t *testing.T) {
	if got := DefaultFootprintPropertyPosition("Reference"); got != (Point{Y: MM(-1.5)}) {
		t.Fatalf("DefaultFootprintPropertyPosition(Reference) = %#v", got)
	}
	if got := DefaultFootprintPropertyPosition("Value"); got != (Point{Y: MM(1.5)}) {
		t.Fatalf("DefaultFootprintPropertyPosition(Value) = %#v", got)
	}
	if got := DefaultFootprintPropertyPosition("Datasheet"); got != (Point{}) {
		t.Fatalf("DefaultFootprintPropertyPosition(Datasheet) = %#v, want origin", got)
	}
}

func TestBoardLocalPlacementTransformMatchesKiCadTopBottomFlip(t *testing.T) {
	point := Point{X: MM(-0.9375), Y: MM(-0.95)}
	if got, want := BoardLocalPointForPlacement(point, LayerBCu), (Point{X: MM(-0.9375), Y: MM(0.95)}); got != want {
		t.Fatalf("bottom point = %#v, want %#v", got, want)
	}
	if got := BoardLocalAngleForPlacement(90, LayerBCu); got != 270 {
		t.Fatalf("bottom geometric angle = %g, want 270", got)
	}
	if got := BoardTextAngleForPlacement(0, LayerBCu); got != 180 {
		t.Fatalf("bottom text angle = %g, want 180", got)
	}
	if got := BoardTextAngleForPlacement(90, LayerBCu); got != 90 {
		t.Fatalf("bottom vertical text angle = %g, want 90", got)
	}
	if restored := BoardLocalPointForPlacement(BoardLocalPointForPlacement(point, LayerBCu), LayerBCu); restored != point {
		t.Fatalf("point transform is not involutive: %#v", restored)
	}
	if restored := BoardLocalAngleForPlacement(BoardLocalAngleForPlacement(90, LayerBCu), LayerBCu); restored != 90 {
		t.Fatalf("angle transform is not involutive: %g", restored)
	}
}
