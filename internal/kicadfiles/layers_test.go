package kicadfiles

import "testing"

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
