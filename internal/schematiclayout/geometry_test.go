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
