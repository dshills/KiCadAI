package schematiclayout

import (
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestTransformPointMatchesKiCadMirrorAxes(t *testing.T) {
	point := kicadfiles.Point{X: kicadfiles.MM(3), Y: kicadfiles.MM(4)}
	if got := TransformPoint(point, 0, MirrorX); got != (kicadfiles.Point{X: kicadfiles.MM(3), Y: kicadfiles.MM(-4)}) {
		t.Fatalf("mirror x = %#v, want reflection across X axis", got)
	}
	if got := TransformPoint(point, 0, MirrorY); got != (kicadfiles.Point{X: kicadfiles.MM(-3), Y: kicadfiles.MM(4)}) {
		t.Fatalf("mirror y = %#v, want reflection across Y axis", got)
	}
}

func TestTextEstimateRotatedKeepsAnchorAndTextSide(t *testing.T) {
	anchor := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	horizontal := TextEstimateRotated("AB", anchor, 0)
	left := TextEstimateRotated("AB", anchor, 180)
	if horizontal.MinX != anchor.X || horizontal.MaxX <= anchor.X || horizontal.MaxY != anchor.Y {
		t.Fatalf("horizontal label bounds = %#v", horizontal)
	}
	if left.MaxX != anchor.X || left.MinX >= anchor.X || left.MaxY != anchor.Y {
		t.Fatalf("left-facing label bounds = %#v", left)
	}
	canonicalLeft := TextEstimateOriented("AB", anchor, 180, true)
	if canonicalLeft != left {
		t.Fatalf("canonical right-justified bounds = %#v, want %#v", canonicalLeft, left)
	}
	above := TextEstimateOriented("AB", anchor, 270, true)
	if above.MaxY != anchor.Y || above.MinY >= anchor.Y {
		t.Fatalf("canonical upward label bounds = %#v", above)
	}
}
