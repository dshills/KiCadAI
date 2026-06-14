package routing

import (
	"fmt"
	"math"

	"kicadai/internal/reports"
)

type ValidationReport struct {
	Issues []reports.Issue `json:"issues,omitempty"`
}

func ValidateResult(request Request, result Result) ValidationReport {
	request = cloneRequest(request)
	NormalizeRequest(&request)
	layerIndexes, _ := LayerIndexes(request.Board.Layers)
	board := BoardRect(request.Board)
	issues := []reports.Issue{}
	access := BuildPadAccess(request)
	for _, route := range result.Routes {
		for _, segment := range route.Segments {
			if _, ok := layerIndexes[normalizeLayer(segment.Layer)]; !ok {
				issues = append(issues, routeValidationIssue(route.Net, reports.CodeInvalidArgument, "segment layer is not routable"))
			}
			if !board.ContainsPoint(segment.Start) || !board.ContainsPoint(segment.End) {
				issues = append(issues, routeValidationIssue(route.Net, reports.CodePlacementOutsideBoard, "segment endpoint is outside board"))
			}
			for _, obstacle := range request.Obstacles {
				if normalizeLayer(obstacle.Layer) == normalizeLayer(segment.Layer) && segmentIntersectsShape(segment, obstacle.Geometry) {
					issues = append(issues, routeValidationIssue(route.Net, reports.CodeValidationFailed, "segment intersects obstacle"))
				}
			}
		}
		for _, via := range route.Vias {
			if !board.ContainsPoint(via.At) {
				issues = append(issues, routeValidationIssue(route.Net, reports.CodePlacementOutsideBoard, "via is outside board"))
			}
			if len(via.Layers) < 2 {
				issues = append(issues, routeValidationIssue(route.Net, reports.CodeInvalidArgument, "via must span at least two layers"))
			}
		}
		if !routeEndpointsConnected(request, route, access) {
			issues = append(issues, routeValidationIssue(route.Net, reports.CodeDisconnectedPad, "route does not connect all intended endpoints"))
		}
	}
	issues = append(issues, clearanceIssues(result.Routes, request.Rules.ClearanceMM)...)
	return ValidationReport{Issues: issues}
}

func routeValidationIssue(netName string, code reports.Code, message string) reports.Issue {
	return reports.Issue{Code: code, Severity: reports.SeverityBlocked, Message: message, Nets: []string{netName}}
}

func segmentIntersectsShape(segment Segment, shape Shape) bool {
	if shape.Rect == nil && len(shape.Polygon) == 0 {
		return false
	}
	bounds := shapeBounds(shape)
	if !shapeBounds(segmentGeometry(segment, segment.WidthMM/2)).Intersects(bounds) {
		return false
	}
	if shape.Rect != nil {
		return segmentIntersectsRect(segment, *shape.Rect)
	}
	return segmentIntersectsPolygon(segment, shape.Polygon)
}

func routeEndpointsConnected(request Request, route Route, access PadAccess) bool {
	var net Net
	for _, candidate := range request.Nets {
		if candidate.Name == route.Net {
			net = candidate
			break
		}
	}
	if len(net.Endpoints) < 2 {
		return true
	}
	graph := newRouteConnectivity(route)
	if len(graph.parent) == 0 {
		return false
	}
	root := ""
	for _, endpoint := range net.Endpoints {
		endpointPoints, ok := AccessPointsForEndpoint(access, endpoint)
		if !ok {
			return false
		}
		endpointRoot := ""
		for _, endpointPoint := range endpointPoints {
			if key, ok := graph.nearestKey(endpointPoint.Point, endpointPoint.Layer); ok {
				endpointRoot = graph.find(key)
				break
			}
		}
		if endpointRoot == "" {
			return false
		}
		if root == "" {
			root = endpointRoot
			continue
		}
		if root != endpointRoot {
			return false
		}
	}
	return true
}

type routeConnectivity struct {
	parent map[string]string
	points map[string]layerPoint
}

type layerPoint struct {
	Point Point
	Layer string
}

func newRouteConnectivity(route Route) routeConnectivity {
	graph := routeConnectivity{parent: map[string]string{}, points: map[string]layerPoint{}}
	for _, segment := range route.Segments {
		start := graph.addPoint(segment.Start, segment.Layer)
		end := graph.addPoint(segment.End, segment.Layer)
		graph.union(start, end)
	}
	for _, via := range route.Vias {
		var first string
		for _, layer := range via.Layers {
			key := graph.addPoint(via.At, layer)
			if first == "" {
				first = key
				continue
			}
			graph.union(first, key)
		}
	}
	return graph
}

func (graph routeConnectivity) addPoint(point Point, layer string) string {
	point = roundPoint(point)
	layer = normalizeLayer(layer)
	key := pointKey(point, layer)
	if _, ok := graph.parent[key]; !ok {
		graph.parent[key] = key
		graph.points[key] = layerPoint{Point: point, Layer: layer}
	}
	return key
}

func (graph routeConnectivity) nearestKey(point Point, layer string) (string, bool) {
	point = roundPoint(point)
	layer = normalizeLayer(layer)
	key := pointKey(point, layer)
	if _, ok := graph.parent[key]; ok {
		return key, true
	}
	for candidate, routePoint := range graph.points {
		if routePoint.Layer == layer && pointDistanceSquared(point, routePoint.Point) <= distanceEpsilonSquared {
			return candidate, true
		}
	}
	return "", false
}

func (graph routeConnectivity) find(key string) string {
	parent := graph.parent[key]
	if parent == key {
		return key
	}
	root := graph.find(parent)
	graph.parent[key] = root
	return root
}

func (graph routeConnectivity) union(left string, right string) {
	leftRoot := graph.find(left)
	rightRoot := graph.find(right)
	if leftRoot != rightRoot {
		graph.parent[rightRoot] = leftRoot
	}
}

func pointKey(point Point, layer string) string {
	point = roundPoint(point)
	return fmt.Sprintf("%s:%.6f,%.6f", normalizeLayer(layer), point.XMM, point.YMM)
}

func clearanceIssues(routes []Route, clearanceMM float64) []reports.Issue {
	issues := []reports.Issue{}
	for leftIndex, leftRoute := range routes {
		for _, left := range leftRoute.Segments {
			for rightIndex := leftIndex + 1; rightIndex < len(routes); rightIndex++ {
				rightRoute := routes[rightIndex]
				for _, right := range rightRoute.Segments {
					if normalizeLayer(left.Layer) != normalizeLayer(right.Layer) {
						continue
					}
					copperClearance := segmentDistance(left, right) - left.WidthMM/2 - right.WidthMM/2
					if copperClearance < clearanceMM {
						issues = append(issues, routeValidationIssue(leftRoute.Net, reports.CodeValidationFailed, "segment clearance violation with net "+rightRoute.Net))
					}
				}
			}
		}
	}
	return issues
}

func segmentDistance(left Segment, right Segment) float64 {
	if left.Start.YMM == left.End.YMM && right.Start.YMM == right.End.YMM {
		if rangesOverlap(left.Start.XMM, left.End.XMM, right.Start.XMM, right.End.XMM) {
			return math.Abs(left.Start.YMM - right.Start.YMM)
		}
	}
	if left.Start.XMM == left.End.XMM && right.Start.XMM == right.End.XMM {
		if rangesOverlap(left.Start.YMM, left.End.YMM, right.Start.YMM, right.End.YMM) {
			return math.Abs(left.Start.XMM - right.Start.XMM)
		}
	}
	distances := []float64{
		distancePointToSegment(left.Start, right.Start, right.End),
		distancePointToSegment(left.End, right.Start, right.End),
		distancePointToSegment(right.Start, left.Start, left.End),
		distancePointToSegment(right.End, left.Start, left.End),
	}
	best := distances[0]
	for _, distance := range distances[1:] {
		best = min(best, distance)
	}
	return best
}

func rangesOverlap(a1 float64, a2 float64, b1 float64, b2 float64) bool {
	return min(a1, a2) <= max(b1, b2) && min(b1, b2) <= max(a1, a2)
}

func segmentIntersectsRect(segment Segment, rect Rect) bool {
	rect = normalizeRect(rect)
	if rect.ContainsPoint(segment.Start) || rect.ContainsPoint(segment.End) {
		return true
	}
	x1 := segment.Start.XMM
	y1 := segment.Start.YMM
	x2 := segment.End.XMM
	y2 := segment.End.YMM
	dx := x2 - x1
	dy := y2 - y1
	t0 := 0.0
	t1 := 1.0
	for _, edge := range []struct {
		p float64
		q float64
	}{
		{-dx, x1 - rect.Min.XMM},
		{dx, rect.Max.XMM - x1},
		{-dy, y1 - rect.Min.YMM},
		{dy, rect.Max.YMM - y1},
	} {
		if edge.p == 0 {
			if edge.q < 0 {
				return false
			}
			continue
		}
		r := edge.q / edge.p
		if edge.p < 0 {
			if r > t1 {
				return false
			}
			if r > t0 {
				t0 = r
			}
		} else {
			if r < t0 {
				return false
			}
			if r < t1 {
				t1 = r
			}
		}
	}
	return true
}

func segmentIntersectsPolygon(segment Segment, polygon []Point) bool {
	if len(polygon) < 3 {
		return false
	}
	if pointInPolygon(segment.Start, polygon) || pointInPolygon(segment.End, polygon) {
		return true
	}
	for index := range polygon {
		next := (index + 1) % len(polygon)
		if lineSegmentsIntersect(segment.Start, segment.End, polygon[index], polygon[next]) {
			return true
		}
	}
	return false
}

func lineSegmentsIntersect(a Point, b Point, c Point, d Point) bool {
	orientation := func(p Point, q Point, r Point) float64 {
		return (q.YMM-p.YMM)*(r.XMM-q.XMM) - (q.XMM-p.XMM)*(r.YMM-q.YMM)
	}
	onSegment := func(p Point, q Point, r Point) bool {
		return q.XMM >= min(p.XMM, r.XMM)-distanceEpsilon &&
			q.XMM <= max(p.XMM, r.XMM)+distanceEpsilon &&
			q.YMM >= min(p.YMM, r.YMM)-distanceEpsilon &&
			q.YMM <= max(p.YMM, r.YMM)+distanceEpsilon
	}
	o1 := orientation(a, b, c)
	o2 := orientation(a, b, d)
	o3 := orientation(c, d, a)
	o4 := orientation(c, d, b)
	if o1*o2 < 0 && o3*o4 < 0 {
		return true
	}
	if math.Abs(o1) <= distanceEpsilon && onSegment(a, c, b) {
		return true
	}
	if math.Abs(o2) <= distanceEpsilon && onSegment(a, d, b) {
		return true
	}
	if math.Abs(o3) <= distanceEpsilon && onSegment(c, a, d) {
		return true
	}
	if math.Abs(o4) <= distanceEpsilon && onSegment(c, b, d) {
		return true
	}
	return false
}
