package routing

import (
	"math"
	"testing"
)

func TestGridRoundTrip(t *testing.T) {
	grid := NewGrid(Point{}, 0.25)
	point := Point{XMM: 1.5, YMM: 2.75}

	coord := grid.ToGrid(point, 1)
	got := grid.ToPoint(coord)
	if coord.X != 6 || coord.Y != 11 || coord.Layer != 1 {
		t.Fatalf("coord = %#v, want 6,11,1", coord)
	}
	if !pointsClose(got, point) {
		t.Fatalf("point = %#v, want %#v", got, point)
	}
}

func TestGridUsesOrigin(t *testing.T) {
	grid := NewGrid(Point{XMM: 10, YMM: 5}, 0.5)
	coord := grid.ToGrid(Point{XMM: 11, YMM: 6.5}, 0)

	if coord.X != 2 || coord.Y != 3 {
		t.Fatalf("coord = %#v, want 2,3", coord)
	}
	if got := grid.ToPoint(coord); !pointsClose(got, Point{XMM: 11, YMM: 6.5}) {
		t.Fatalf("point = %#v", got)
	}
}

func TestBoardCoordinateConversion(t *testing.T) {
	board := Board{Origin: Point{XMM: 100, YMM: 50}}
	local := Point{XMM: 3.25, YMM: 4.5}

	global := LocalToGlobal(board, local)
	if !pointsClose(global, Point{XMM: 103.25, YMM: 54.5}) {
		t.Fatalf("global = %#v", global)
	}
	if got := GlobalToLocal(board, global); !pointsClose(got, local) {
		t.Fatalf("local = %#v", got)
	}
}

func TestNewGridDefaultsInvalidGrid(t *testing.T) {
	grid := NewGrid(Point{}, 0)
	if grid.GridMM != DefaultRules().GridMM {
		t.Fatalf("grid = %f, want default", grid.GridMM)
	}
}

func TestZeroValueGridRoundTripsWithDefaultSpacing(t *testing.T) {
	var grid Grid
	point := Point{XMM: 1, YMM: 2}
	coord := grid.ToGrid(point, 0)
	got := grid.ToPoint(coord)
	if !pointsClose(got, point) {
		t.Fatalf("zero grid round trip = %#v, want %#v", got, point)
	}
}

func TestRectHelpers(t *testing.T) {
	rect := Rect{Min: Point{XMM: 1, YMM: 2}, Max: Point{XMM: 4, YMM: 6}}

	if rect.WidthMM() != 3 || rect.HeightMM() != 4 {
		t.Fatalf("size = %.1f x %.1f", rect.WidthMM(), rect.HeightMM())
	}
	if !rect.ContainsPoint(Point{XMM: 2, YMM: 3}) {
		t.Fatal("rect should contain point")
	}
	if !rect.Intersects(Rect{Min: Point{XMM: 3, YMM: 5}, Max: Point{XMM: 5, YMM: 7}}) {
		t.Fatal("rect should intersect")
	}
	expanded := rect.Expand(1)
	if expanded.Min != (Point{XMM: 0, YMM: 1}) || expanded.Max != (Point{XMM: 5, YMM: 7}) {
		t.Fatalf("expanded = %#v", expanded)
	}
	shrunk := rect.Expand(-10)
	if shrunk.WidthMM() != 0 || shrunk.HeightMM() != 0 {
		t.Fatalf("over-shrunk rect = %#v, want zero area", shrunk)
	}
}

func TestUsableBoardRectIncludesMarginAndEdgeClearance(t *testing.T) {
	board := Board{WidthMM: 20, HeightMM: 10, MarginMM: 1}
	rules := Rules{EdgeClearanceMM: 0.5}

	got := UsableBoardRect(board, rules)
	want := Rect{Min: Point{XMM: 1.5, YMM: 1.5}, Max: Point{XMM: 18.5, YMM: 8.5}}
	if got != want {
		t.Fatalf("usable = %#v, want %#v", got, want)
	}
}

func TestUsableBoardRectClampsOverlappingMargins(t *testing.T) {
	got := UsableBoardRect(Board{WidthMM: 10, HeightMM: 4, MarginMM: 3}, Rules{EdgeClearanceMM: 3})
	if got.WidthMM() != 0 || got.HeightMM() != 0 {
		t.Fatalf("usable = %#v, want zero area", got)
	}
	if got.Min != (Point{XMM: 5, YMM: 2}) || got.Max != (Point{XMM: 5, YMM: 2}) {
		t.Fatalf("usable = %#v, want centered zero-area rect", got)
	}
}

func TestLayerIndexesTrimAndNormalize(t *testing.T) {
	indexes, err := LayerIndexes([]Layer{{Name: " F.Cu "}, {Name: "B.Cu"}})
	if err != nil {
		t.Fatalf("LayerIndexes returned error: %v", err)
	}
	if indexes["F.CU"] != 0 || indexes["B.CU"] != 1 {
		t.Fatalf("indexes = %#v", indexes)
	}
}

func TestLayerIndexesRejectsDuplicateNormalizedNames(t *testing.T) {
	if _, err := LayerIndexes([]Layer{{Name: " F.Cu "}, {Name: "f.cu"}}); err == nil {
		t.Fatal("expected duplicate layer error")
	}
}

func pointsClose(left Point, right Point) bool {
	const epsilon = 1e-9
	return math.Abs(left.XMM-right.XMM) <= epsilon && math.Abs(left.YMM-right.YMM) <= epsilon
}
