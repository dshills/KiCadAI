package routing

import (
	"math"
	"testing"
)

func TestBuildOccupancyBlocksBoardEdges(t *testing.T) {
	request := minimalRequest()
	request.Board.WidthMM = 10
	request.Board.HeightMM = 10
	request.Board.MarginMM = 1
	request.Rules.GridMM = 1
	request.Rules.EdgeClearanceMM = 0

	occupancy := mustBuildOccupancy(t, request, "SIG")
	if !occupancy.BlockedCell(GridCoord{X: 0, Y: 0, Layer: 0}) {
		t.Fatal("expected board edge cell to be blocked")
	}
	if occupancy.BlockedCell(GridCoord{X: 5, Y: 5, Layer: 0}) {
		t.Fatal("expected board center to be unblocked")
	}
}

func TestBuildOccupancyBlocksKeepout(t *testing.T) {
	request := minimalRequest()
	request.Rules.GridMM = 1
	request.Obstacles = []Obstacle{{
		Kind:     ObstacleKeepout,
		Layer:    "F.Cu",
		Geometry: Shape{Rect: &Rect{Min: Point{XMM: 4, YMM: 4}, Max: Point{XMM: 6, YMM: 6}}},
	}}

	occupancy := mustBuildOccupancy(t, request, "SIG")
	if !occupancy.BlockedCell(GridCoord{X: 5, Y: 5, Layer: 0}) {
		t.Fatal("expected keepout cell to be blocked")
	}
	obstacle, ok := occupancy.FirstObstacle(GridCoord{X: 5, Y: 5, Layer: 0})
	if !ok || obstacle.Kind != ObstacleKeepout {
		t.Fatalf("obstacle = %#v ok=%v, want keepout", obstacle, ok)
	}
}

func TestBuildOccupancyBlocksOtherNetPadButNotCurrentNetPad(t *testing.T) {
	request := minimalRequest()
	request.Rules.GridMM = 1
	request.Components[0].Pads[0].Net = "SIG"
	request.Components[1].Pads[0].Net = "OTHER"

	occupancy := mustBuildOccupancy(t, request, "SIG")
	if occupancy.BlockedCell(GridCoord{X: 5, Y: 10, Layer: 0}) {
		t.Fatal("current net pad should remain accessible")
	}
	if !occupancy.BlockedCell(GridCoord{X: 20, Y: 10, Layer: 0}) {
		t.Fatal("other net pad should be blocked")
	}
}

func TestPadRectAccountsForComponentRotation(t *testing.T) {
	component := Component{Position: Placement{RotationDeg: 90}}
	pad := Pad{Position: Point{XMM: 10, YMM: 10}, Size: Size{WidthMM: 4, HeightMM: 2}}

	rect := padRect(component, pad).Rect
	if rect == nil {
		t.Fatal("missing pad rect")
	}
	if !floatClose(rect.WidthMM(), 2) || !floatClose(rect.HeightMM(), 4) {
		t.Fatalf("rotated rect = %#v, want 2 x 4", rect)
	}
}

func TestAbsolutePadPointRotatesAroundComponentOrigin(t *testing.T) {
	component := Component{Position: Placement{XMM: 10, YMM: 20, RotationDeg: 90}}
	got := absolutePadPoint(component, Point{XMM: 2, YMM: 0})
	want := Point{XMM: 10, YMM: 22}
	if !floatClose(got.XMM, want.XMM) || !floatClose(got.YMM, want.YMM) {
		t.Fatalf("absolute pad point = %#v, want %#v", got, want)
	}
}

func TestPointWithinPolygonClearanceUsesEdgeDistance(t *testing.T) {
	polygon := []Point{{XMM: 0, YMM: 0}, {XMM: 2, YMM: 0}, {XMM: 2, YMM: 2}, {XMM: 0, YMM: 2}}
	if !pointWithinPolygonClearance(Point{XMM: 2.4, YMM: 1}, polygon, 0.5) {
		t.Fatal("point near polygon edge should be within clearance")
	}
	if pointWithinPolygonClearance(Point{XMM: 3, YMM: 1}, polygon, 0.5) {
		t.Fatal("distant point should not be within clearance")
	}
}

func TestShapeBoundsUnionsRectAndPolygon(t *testing.T) {
	shape := Shape{
		Rect:    &Rect{Min: Point{XMM: 0, YMM: 0}, Max: Point{XMM: 1, YMM: 1}},
		Polygon: []Point{{XMM: -2, YMM: 0}, {XMM: 0, YMM: 3}, {XMM: 2, YMM: 0}},
	}
	bounds := shapeBounds(shape)
	if bounds.Min != (Point{XMM: -2, YMM: 0}) || bounds.Max != (Point{XMM: 2, YMM: 3}) {
		t.Fatalf("bounds = %#v", bounds)
	}
}

func TestBuildOccupancyBlocksFixedCopper(t *testing.T) {
	request := minimalRequest()
	request.Rules.GridMM = 1
	request.Existing = []ExistingCopper{{
		Kind:     CopperSegment,
		Net:      "OTHER",
		Layer:    "F.Cu",
		Fixed:    true,
		Geometry: Shape{Rect: &Rect{Min: Point{XMM: 8, YMM: 8}, Max: Point{XMM: 9, YMM: 9}}},
	}}

	occupancy := mustBuildOccupancy(t, request, "SIG")
	if !occupancy.BlockedCell(GridCoord{X: 8, Y: 8, Layer: 0}) {
		t.Fatal("expected fixed copper to block occupancy")
	}
}

func TestBuildOccupancyDoesNotBlockSameNetFixedCopper(t *testing.T) {
	request := minimalRequest()
	request.Rules.GridMM = 1
	request.Existing = []ExistingCopper{{
		Kind:     CopperSegment,
		Net:      "SIG",
		Layer:    "F.Cu",
		Fixed:    true,
		Geometry: Shape{Rect: &Rect{Min: Point{XMM: 8, YMM: 8}, Max: Point{XMM: 9, YMM: 9}}},
	}}

	occupancy := mustBuildOccupancy(t, request, "SIG")
	if occupancy.BlockedCell(GridCoord{X: 8, Y: 8, Layer: 0}) {
		t.Fatal("same-net fixed copper should remain reusable")
	}
}

func TestBuildOccupancyIsLayerAware(t *testing.T) {
	request := minimalRequest()
	request.Rules.GridMM = 1
	request.Obstacles = []Obstacle{{
		Kind:     ObstacleKeepout,
		Layer:    "B.Cu",
		Geometry: Shape{Rect: &Rect{Min: Point{XMM: 4, YMM: 4}, Max: Point{XMM: 6, YMM: 6}}},
	}}

	occupancy := mustBuildOccupancy(t, request, "SIG")
	if occupancy.BlockedCell(GridCoord{X: 5, Y: 5, Layer: 0}) {
		t.Fatal("front layer should not be blocked by back layer keepout")
	}
	if !occupancy.BlockedCell(GridCoord{X: 5, Y: 5, Layer: 1}) {
		t.Fatal("back layer should be blocked")
	}
}

func TestBuildOccupancyFailsClosedForDuplicateLayers(t *testing.T) {
	request := minimalRequest()
	request.Board.Layers = append(request.Board.Layers, Layer{Name: " f.cu ", Kind: LayerCopper, Routable: true})

	if _, err := BuildOccupancy(request, "SIG"); err == nil {
		t.Fatal("expected duplicate layer error")
	}
}

func TestBuildOccupancyRejectsHugeGrid(t *testing.T) {
	request := minimalRequest()
	request.Board.WidthMM = 10000
	request.Board.HeightMM = 10000
	request.Rules.GridMM = 0.01

	if _, err := BuildOccupancy(request, "SIG"); err == nil {
		t.Fatal("expected huge occupancy grid error")
	}
}

func TestOccupancyLayerKeysAreDeterministic(t *testing.T) {
	occupancy := Occupancy{Layers: map[int]*LayerOccupancy{2: {}, 0: {}}}
	keys := occupancyLayerKeys(occupancy)
	if len(keys) != 2 || keys[0] != 0 || keys[1] != 2 {
		t.Fatalf("keys = %#v", keys)
	}
}

func mustBuildOccupancy(t *testing.T, request Request, currentNet string) Occupancy {
	t.Helper()
	occupancy, err := BuildOccupancy(request, currentNet)
	if err != nil {
		t.Fatalf("BuildOccupancy returned error: %v", err)
	}
	return occupancy
}

func floatClose(left float64, right float64) bool {
	return math.Abs(left-right) <= 1e-9
}
