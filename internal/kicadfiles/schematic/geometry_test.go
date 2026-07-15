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

func TestCanonicalConnectionAnchorTransformMatrix(t *testing.T) {
	position := kicadfiles.Point{X: kicadfiles.MM(20.1), Y: kicadfiles.MM(30.9)}
	offset := kicadfiles.Point{X: kicadfiles.MM(3.81), Y: kicadfiles.MM(-2.54)}
	for _, rotation := range []kicadfiles.Angle{0, 90, 180, 270} {
		for _, mirror := range []SymbolMirror{SymbolMirrorNone, SymbolMirrorX, SymbolMirrorY} {
			got := CanonicalConnectionAnchor(position, offset, rotation, mirror)
			transformed := TransformConnectionAnchor(offset, rotation, mirror)
			origin := CanonicalConnectionPoint(position)
			want := CanonicalConnectionPoint(kicadfiles.Point{X: origin.X + transformed.X, Y: origin.Y + transformed.Y})
			if got != want {
				t.Fatalf("rotation=%v mirror=%q anchor=%#v want=%#v", rotation, mirror, got, want)
			}
			if got.X%ConnectionGrid != 0 || got.Y%ConnectionGrid != 0 {
				t.Fatalf("rotation=%v mirror=%q anchor=%#v is off grid", rotation, mirror, got)
			}
		}
	}
}

func TestCollisionFreeSymbolPositionIsDeterministic(t *testing.T) {
	requested := kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}
	offsets := []kicadfiles.Point{{}}
	occupied := map[kicadfiles.Point]struct{}{CanonicalConnectionAnchor(requested, offsets[0], 0, SymbolMirrorNone): {}}
	first, ok := CollisionFreeSymbolPosition(requested, offsets, 0, SymbolMirrorNone, occupied)
	if !ok {
		t.Fatal("expected a collision-free position")
	}
	second, ok := CollisionFreeSymbolPosition(requested, offsets, 0, SymbolMirrorNone, occupied)
	if !ok || first != second {
		t.Fatalf("collision resolution is not deterministic: %#v/%#v", first, second)
	}
	wantOrigin := CanonicalConnectionPoint(requested)
	want := kicadfiles.Point{X: wantOrigin.X - ConnectionGrid, Y: wantOrigin.Y - ConnectionGrid}
	if first != want {
		t.Fatalf("collision-free position = %#v, want %#v", first, want)
	}
}

func TestCollisionFreeSymbolPositionRejectsInternalSnappedPinCollision(t *testing.T) {
	position := kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}
	offsets := []kicadfiles.Point{{}, {X: kicadfiles.MM(0.1)}}
	if resolved, ok := CollisionFreeSymbolPosition(position, offsets, 0, SymbolMirrorNone, nil); ok {
		t.Fatalf("internally colliding pin anchors accepted at %#v", resolved)
	}
}

func TestCollisionFreeSymbolPositionAllowsIntentionalStackedPins(t *testing.T) {
	position := kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}
	offset := kicadfiles.Point{X: kicadfiles.MM(2.54)}
	if _, ok := CollisionFreeSymbolPosition(position, []kicadfiles.Point{offset, offset}, 0, SymbolMirrorNone, nil); !ok {
		t.Fatal("intentional stacked pin offsets were rejected")
	}
}

func TestCanonicalSymbolTransformMatchesKiCadSaveForms(t *testing.T) {
	tests := []struct {
		rotation     kicadfiles.Angle
		mirror       SymbolMirror
		wantRotation kicadfiles.Angle
		wantMirror   SymbolMirror
	}{
		{rotation: 180, mirror: SymbolMirrorX, wantMirror: SymbolMirrorY},
		{rotation: 90, mirror: SymbolMirrorY, wantRotation: 270, wantMirror: SymbolMirrorX},
		{rotation: 180, mirror: SymbolMirrorY, wantMirror: SymbolMirrorX},
		{rotation: 270, mirror: SymbolMirrorY, wantRotation: 90, wantMirror: SymbolMirrorX},
	}
	for _, tt := range tests {
		rotation, mirror := CanonicalSymbolTransform(tt.rotation, tt.mirror)
		if rotation != tt.wantRotation || mirror != tt.wantMirror {
			t.Fatalf("CanonicalSymbolTransform(%v, %q) = (%v, %q), want (%v, %q)", tt.rotation, tt.mirror, rotation, mirror, tt.wantRotation, tt.wantMirror)
		}
		probe := kicadfiles.Point{X: kicadfiles.MM(3), Y: kicadfiles.MM(4)}
		if got, want := TransformConnectionAnchor(probe, tt.rotation, tt.mirror), TransformConnectionAnchor(probe, rotation, mirror); got != want {
			t.Fatalf("canonical transform changed anchor: got %#v want %#v", got, want)
		}
	}
}
