package schematic

import (
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestIncludeArcBoundsIncludesCardinalExtrema(t *testing.T) {
	var bounds symbolBoundsAccumulator
	includeArcBounds(&bounds,
		kicadfiles.Point{X: kicadfiles.MM(7.071067), Y: kicadfiles.MM(7.071067)},
		kicadfiles.Point{X: kicadfiles.MM(-7.071067), Y: kicadfiles.MM(7.071067)},
		kicadfiles.Point{X: kicadfiles.MM(-7.071067), Y: kicadfiles.MM(-7.071067)},
	)
	body, ok := bounds.result()
	if !ok {
		t.Fatal("expected arc bounds")
	}
	if body.Min.X > kicadfiles.MM(-9.5) || body.Max.Y < kicadfiles.MM(9.5) {
		t.Fatalf("arc bounds = %#v, want cardinal extrema beyond endpoint bounds", body)
	}
}

func TestTransformConnectionAnchorUsesKiCadMirrorThenRotateOrder(t *testing.T) {
	offset := kicadfiles.Point{X: kicadfiles.MM(3), Y: kicadfiles.MM(4)}
	if got := TransformConnectionAnchor(offset, 0, SymbolMirrorX); got != (kicadfiles.Point{X: kicadfiles.MM(3), Y: kicadfiles.MM(-4)}) {
		t.Fatalf("mirror x = %#v", got)
	}
	if got := TransformConnectionAnchor(offset, 0, SymbolMirrorY); got != (kicadfiles.Point{X: kicadfiles.MM(-3), Y: kicadfiles.MM(4)}) {
		t.Fatalf("mirror y = %#v", got)
	}
	if got := TransformConnectionAnchor(offset, 90, SymbolMirrorX); got != (kicadfiles.Point{X: kicadfiles.MM(4), Y: kicadfiles.MM(3)}) {
		t.Fatalf("mirror x then rotate = %#v", got)
	}
}
