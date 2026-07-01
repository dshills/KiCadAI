package physicalrules

import (
	"math"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestDFMPolygonAreaBoundsAndWinding(t *testing.T) {
	polygon := []kicadfiles.Point{point(0, 0), point(4, 0), point(4, 2), point(0, 2), point(0, 0)}
	if got := dfmPolygonAbsAreaMM2(polygon); got != 8 {
		t.Fatalf("area = %g, want 8", got)
	}
	bounds := dfmPolygonBounds(polygon)
	if !bounds.Valid || bounds.MinX != 0 || bounds.MinY != 0 || bounds.MaxX != 4 || bounds.MaxY != 2 {
		t.Fatalf("bounds = %#v", bounds)
	}
	reversed := []kicadfiles.Point{point(0, 0), point(0, 2), point(4, 2), point(4, 0)}
	if dfmPolygonAreaMM2(reversed) >= 0 {
		t.Fatalf("expected reversed polygon to have negative signed area")
	}
	normalized := dfmNormalizeWinding(reversed)
	if dfmPolygonAreaMM2(normalized) <= 0 {
		t.Fatalf("normalized winding area = %g", dfmPolygonAreaMM2(normalized))
	}
}

func TestDFMPointInPolygon(t *testing.T) {
	polygon := []kicadfiles.Point{point(0, 0), point(4, 0), point(4, 2), point(0, 2)}
	for _, pt := range []kicadfiles.Point{point(2, 1), point(0, 1)} {
		if !dfmPointInPolygon(polygon, pt) {
			t.Fatalf("point %#v should be inside/on polygon", pt)
		}
	}
	if dfmPointInPolygon(polygon, point(5, 1)) {
		t.Fatal("outside point reported inside")
	}
}

func TestDFMSegmentIntersectionAndDistances(t *testing.T) {
	if !dfmSegmentsIntersect(point(0, 0), point(2, 2), point(0, 2), point(2, 0)) {
		t.Fatal("crossing segments did not intersect")
	}
	if dfmSegmentsIntersect(point(0, 0), point(1, 0), point(0, 1), point(1, 1)) {
		t.Fatal("parallel separated segments intersected")
	}
	if got := dfmPointSegmentDistanceMM(point(1, 1), point(0, 0), point(2, 0)); math.Abs(got-1) > 1e-9 {
		t.Fatalf("point-segment distance = %g, want 1", got)
	}
	if got := dfmSegmentSegmentDistanceMM(point(0, 0), point(1, 0), point(0, 2), point(1, 2)); math.Abs(got-2) > 1e-9 {
		t.Fatalf("segment-segment distance = %g, want 2", got)
	}
}

func TestDFMPolygonDistanceAndSelfIntersection(t *testing.T) {
	a := []kicadfiles.Point{point(0, 0), point(1, 0), point(1, 1), point(0, 1)}
	b := []kicadfiles.Point{point(3, 0), point(4, 0), point(4, 1), point(3, 1)}
	if got := dfmPolygonDistanceMM(a, b); math.Abs(got-2) > 1e-9 {
		t.Fatalf("polygon distance = %g, want 2", got)
	}
	overlap := []kicadfiles.Point{point(0.5, 0.5), point(1.5, 0.5), point(1.5, 1.5), point(0.5, 1.5)}
	if got := dfmPolygonDistanceMM(a, overlap); got != 0 {
		t.Fatalf("overlap distance = %g, want 0", got)
	}
	bowtie := []kicadfiles.Point{point(0, 0), point(2, 2), point(0, 2), point(2, 0)}
	if !dfmSelfIntersects(bowtie) {
		t.Fatal("self-intersecting polygon was not detected")
	}
	if dfmSelfIntersects(a) {
		t.Fatal("simple polygon reported self-intersecting")
	}
}

func TestDFMRectDistance(t *testing.T) {
	a := dfmRect{Valid: true, MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}
	b := dfmRect{Valid: true, MinX: 3, MinY: 0, MaxX: 4, MaxY: 1}
	if got := dfmRectDistanceMM(a, b); math.Abs(got-2) > 1e-9 {
		t.Fatalf("rect distance = %g, want 2", got)
	}
	c := dfmRect{Valid: true, MinX: 0.5, MinY: 0.5, MaxX: 1.5, MaxY: 1.5}
	if got := dfmRectDistanceMM(a, c); got != 0 {
		t.Fatalf("overlap rect distance = %g, want 0", got)
	}
}
