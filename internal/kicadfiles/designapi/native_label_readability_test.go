package designapi

import (
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestNativeEndpointLabelGlyphBoundsFaceAwayFromAnchor(t *testing.T) {
	anchor := kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)}
	for _, delta := range []kicadfiles.Point{{X: kicadfiles.MM(5)}, {X: -kicadfiles.MM(5)}, {Y: kicadfiles.MM(5)}, {Y: -kicadfiles.MM(5)}} {
		p := kicadfiles.Point{X: anchor.X + delta.X, Y: anchor.Y + delta.Y}
		o := schematicLabelOptionsForStub(anchor, p)
		b := nativeSchematicLabelTextBounds("Long pin label", p, o.Rotation, containsFold(o.Justify, "right"))
		if delta.X > 0 && b.minX < p.X || delta.X < 0 && b.maxX > p.X || delta.Y > 0 && b.minY < p.Y || delta.Y < 0 && b.maxY > p.Y {
			t.Fatalf("inward label delta=%+v bounds=%+v", delta, b)
		}
	}
}

func TestNativeLabelStubsCannotCrossVisibleSymbolBodies(t *testing.T) {
	builder := newTestBuilder(t)
	center := kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)}
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", center)
	left, right := center, center
	left.X -= kicadfiles.MM(10)
	right.X += kicadfiles.MM(10)
	if builder.nativeStubCrossesSymbolBody(left, right) {
		t.Fatal("legacy policy changed")
	}
	builder.nativeSchematicLabels = true
	if !builder.nativeStubCrossesSymbolBody(left, right) {
		t.Fatal("body-crossing label stub accepted")
	}
	left.Y += kicadfiles.MM(10)
	right.Y += kicadfiles.MM(10)
	if builder.nativeStubCrossesSymbolBody(left, right) {
		t.Fatal("clear label stub rejected")
	}
}
