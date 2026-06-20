package routing

import (
	"context"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestRouteSingleLayerPathStraight(t *testing.T) {
	request := singleLayerSearchRequest()
	path, issues := routeFirstPair(t, request)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(path.Points) < 2 {
		t.Fatalf("path = %#v, want at least start and end", path.Points)
	}
	if path.Points[0] != (Point{XMM: 5, YMM: 10}) {
		t.Fatalf("start = %#v", path.Points[0])
	}
	if path.Points[len(path.Points)-1] != (Point{XMM: 20, YMM: 10}) {
		t.Fatalf("end = %#v", path.Points[len(path.Points)-1])
	}
	for _, point := range path.Points {
		if point.YMM != 10 {
			t.Fatalf("straight path point = %#v, want y=10", point)
		}
	}
}

func TestRouteSingleLayerPathLShape(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Components[1].Position.YMM = 14
	request.Components[1].Pads[0].Position = Point{}
	request.Nets[0].Endpoints[1] = Endpoint{Ref: "J2", Pin: "1"}

	path, issues := routeFirstPair(t, request)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	sawHorizontal := false
	sawVertical := false
	for index := 1; index < len(path.Points); index++ {
		prev := path.Points[index-1]
		next := path.Points[index]
		if prev.YMM == next.YMM && prev.XMM != next.XMM {
			sawHorizontal = true
		}
		if prev.XMM == next.XMM && prev.YMM != next.YMM {
			sawVertical = true
		}
	}
	if !sawHorizontal || !sawVertical {
		t.Fatalf("path = %#v, want both horizontal and vertical movement", path.Points)
	}
}

func TestRouteSingleLayerPathAvoidsObstacle(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Obstacles = []Obstacle{{
		Kind:  ObstacleKeepout,
		Layer: "F.Cu",
		Geometry: Shape{Rect: &Rect{
			Min: Point{XMM: 10, YMM: 9},
			Max: Point{XMM: 15, YMM: 11},
		}},
	}}

	path, issues := routeFirstPair(t, request)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	for _, point := range path.Points {
		if point.XMM >= 10 && point.XMM <= 15 && point.YMM >= 9 && point.YMM <= 11 {
			t.Fatalf("path crosses keepout at %#v: %#v", point, path.Points)
		}
	}
}

func TestRouteFailureNamesNearbyObstacle(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Obstacles = []Obstacle{{
		Kind:   ObstacleKeepout,
		Layer:  "F.Cu",
		Source: "fixture_keepout",
		Geometry: Shape{Rect: &Rect{
			Min: Point{XMM: 0, YMM: 0},
			Max: Point{XMM: 30, YMM: 30},
		}},
	}}

	result := RouteRequest(request)
	if result.Status != StatusBlocked || len(result.Routes) == 0 || len(result.Routes[0].Issues) == 0 {
		t.Fatalf("expected blocked route with issue: %#v", result)
	}
	if got := result.Routes[0].Issues[0].Message; !strings.Contains(got, "fixture_keepout") {
		t.Fatalf("issue message = %q, want obstacle source", got)
	}
}

func TestRouteSingleLayerPathNoPath(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Obstacles = []Obstacle{{
		Kind:  ObstacleKeepout,
		Layer: "F.Cu",
		Geometry: Shape{Rect: &Rect{
			Min: Point{XMM: 10, YMM: 0},
			Max: Point{XMM: 15, YMM: 20},
		}},
	}}

	path, issues := routeFirstPair(t, request)
	if len(path.Points) != 0 {
		t.Fatalf("path = %#v, want no path", path.Points)
	}
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one no-path issue", issues)
	}
}

func TestRouteSingleLayerPathAllowsBlockedEndpointCells(t *testing.T) {
	request := singleLayerSearchRequest()
	access := BuildPadAccess(request)
	occupancy, err := BuildOccupancy(request, "SIG")
	if err != nil {
		t.Fatalf("BuildOccupancy error: %v", err)
	}
	layerIndexes, err := LayerIndexes(request.Board.Layers)
	if err != nil {
		t.Fatalf("LayerIndexes error: %v", err)
	}
	layerIndex := layerIndexes[normalizeLayer("F.Cu")]
	obstacleIndex := occupancy.addObstacle(Obstacle{Kind: ObstacleSameNetPad, Layer: "F.Cu", Source: "test"})
	occupancy.block(layerIndex, gridPoint{X: 5, Y: 10}, obstacleIndex)
	occupancy.block(layerIndex, gridPoint{X: 20, Y: 10}, obstacleIndex)
	plans, issues := PlanRoutes(request, access)
	if len(issues) != 0 {
		t.Fatalf("plan issues = %#v", issues)
	}

	path, routeIssues := routeSingleLayerPath(context.Background(), request, access, occupancy, "SIG", plans[0].Pairs[0], "F.Cu")
	if len(routeIssues) != 0 {
		t.Fatalf("route issues = %#v", routeIssues)
	}
	if len(path.Points) == 0 {
		t.Fatalf("path = %#v, want routed path", path)
	}
}

func TestRouteSingleLayerPathDeterministic(t *testing.T) {
	request := singleLayerSearchRequest()
	first, firstIssues := routeFirstPair(t, request)
	second, secondIssues := routeFirstPair(t, request)
	if len(firstIssues) != 0 || len(secondIssues) != 0 {
		t.Fatalf("issues = %#v %#v", firstIssues, secondIssues)
	}
	if len(first.Coordinates) != len(second.Coordinates) {
		t.Fatalf("path lengths changed: %d vs %d", len(first.Coordinates), len(second.Coordinates))
	}
	for index := range first.Coordinates {
		if first.Coordinates[index] != second.Coordinates[index] {
			t.Fatalf("coord[%d] changed: %#v vs %#v", index, first.Coordinates[index], second.Coordinates[index])
		}
	}
}

func routeFirstPair(t *testing.T, request Request) (GridPath, []reports.Issue) {
	t.Helper()
	access := BuildPadAccess(request)
	if len(access.Issues) != 0 {
		t.Fatalf("access issues = %#v", access.Issues)
	}
	occupancy, err := BuildOccupancy(request, "SIG")
	if err != nil {
		t.Fatalf("BuildOccupancy error: %v", err)
	}
	plans, issues := PlanRoutes(request, access)
	if len(issues) != 0 {
		t.Fatalf("plan issues = %#v", issues)
	}
	if len(plans) != 1 || len(plans[0].Pairs) != 1 {
		t.Fatalf("plans = %#v, want one pair", plans)
	}
	return routeSingleLayerPath(context.Background(), request, access, occupancy, "SIG", plans[0].Pairs[0], "F.Cu")
}

func singleLayerSearchRequest() Request {
	request := minimalRequest()
	request.Rules.GridMM = 1
	request.Rules.TraceWidthMM = 0.1
	request.Rules.ClearanceMM = 0.01
	request.Rules.EdgeClearanceMM = 0.01
	request.Rules.MaxSearchNodes = 10000
	request.Strategy.Mode = ModeSingleLayer
	request.Board.Layers = []Layer{{Name: "F.Cu", Kind: LayerCopper, Routable: true}}
	for componentIndex := range request.Components {
		for padIndex := range request.Components[componentIndex].Pads {
			request.Components[componentIndex].Pads[padIndex].Type = PadSMD
			request.Components[componentIndex].Pads[padIndex].Drill = nil
			request.Components[componentIndex].Pads[padIndex].Layers = []string{"F.Cu"}
		}
	}
	return request
}
