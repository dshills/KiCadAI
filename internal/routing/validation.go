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
	access = expandSMDPadEdgeAccess(access, request, net.Endpoints)
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

type routeConnectivitySegment struct {
	segment  Segment
	layer    string
	startKey string
}

func newRouteConnectivity(route Route) routeConnectivity {
	graph := routeConnectivity{parent: map[string]string{}, points: map[string]layerPoint{}}
	segments := make([]routeConnectivitySegment, 0, len(route.Segments))
	indexedSegments := make([]clearanceSegment, 0, len(route.Segments))
	for _, segment := range route.Segments {
		segment.Start = roundPoint(segment.Start)
		segment.End = roundPoint(segment.End)
		layer := normalizeLayer(segment.Layer)
		start := graph.addPoint(segment.Start, layer)
		end := graph.addPoint(segment.End, layer)
		graph.union(start, end)
		segments = append(segments, routeConnectivitySegment{segment: segment, layer: layer, startKey: start})
		indexedSegments = append(indexedSegments, clearanceSegment{Layer: layer, Segment: segment})
	}
	spatialIndex := clearanceSpatialIndex(indexedSegments, 2.54)
	queryScratch := newClearanceQueryScratch(len(segments))
	for leftIndex, left := range segments {
		for _, rightIndex := range spatialIndex.query(left.layer, left.segment, distanceEpsilon, queryScratch) {
			if rightIndex <= leftIndex {
				continue
			}
			right := segments[rightIndex]
			if segmentsIntersect(left.segment.Start, left.segment.End, right.segment.Start, right.segment.End) {
				graph.union(left.startKey, right.startKey)
			}
		}
	}
	for _, via := range route.Vias {
		via.At = roundPoint(via.At)
		var first string
		for _, layer := range via.Layers {
			layer = normalizeLayer(layer)
			key := graph.addPoint(via.At, layer)
			if first == "" {
				first = key
			} else {
				graph.union(first, key)
			}
			pointSegment := Segment{Start: via.At, End: via.At}
			for _, candidateIndex := range spatialIndex.query(layer, pointSegment, distanceEpsilon, queryScratch) {
				segment := segments[candidateIndex]
				if routePointOnSegment(via.At, segment.segment) {
					graph.union(key, segment.startKey)
				}
			}
		}
	}
	return graph
}

func routePointOnSegment(point Point, segment Segment) bool {
	return orientation(segment.Start, segment.End, point) == 0 && pointOnSegment(point, segment.Start, segment.End)
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
	segments := clearanceSegments(routes)
	if len(segments) < 2 {
		return nil
	}
	maxHalfWidth := 0.0
	for _, candidate := range segments {
		maxHalfWidth = max(maxHalfWidth, candidate.Segment.WidthMM/2)
	}
	cellSize := clearanceIndexCellSize(clearanceMM)
	index := clearanceSpatialIndex(segments, cellSize)
	scratch := newClearanceQueryScratch(len(segments))
	issues := []reports.Issue{}
	for leftIndex, left := range segments {
		queryMargin := clearanceMM + left.Segment.WidthMM/2 + maxHalfWidth
		for _, rightIndex := range index.query(left.Layer, left.Segment, queryMargin, scratch) {
			if rightIndex <= leftIndex {
				continue
			}
			right := segments[rightIndex]
			if left.Net == right.Net {
				continue
			}
			requiredGap := clearanceMM + left.Segment.WidthMM/2 + right.Segment.WidthMM/2
			if !segmentBoundsWithin(left.Segment, right.Segment, requiredGap) {
				continue
			}
			copperClearance := segmentDistance(left.Segment, right.Segment) - left.Segment.WidthMM/2 - right.Segment.WidthMM/2
			if copperClearance < clearanceMM {
				issues = append(issues, routeValidationIssue(left.Net, reports.CodeValidationFailed, "segment clearance violation with net "+right.Net))
			}
		}
	}
	return issues
}

func clearanceIndexCellSize(clearanceMM float64) float64 {
	cellSize := max(clearanceMM, 0.25)
	return min(cellSize, 2.54)
}

type clearanceSegment struct {
	Net     string
	Layer   string
	Segment Segment
}

type clearanceGridIndex struct {
	cellSize float64
	cells    map[clearanceCellKey][]int
}

type clearanceQueryScratch struct {
	marks          []int
	generation     int
	cellMarks      map[clearanceCellKey]int
	cellGeneration int
	out            []int
}

const maxClearanceQueryGeneration = int(^uint(0) >> 1)

type clearanceCellKey struct {
	Layer string
	X     int
	Y     int
}

func clearanceSegments(routes []Route) []clearanceSegment {
	var out []clearanceSegment
	for _, route := range routes {
		for _, segment := range route.Segments {
			out = append(out, clearanceSegment{
				Net:     route.Net,
				Layer:   normalizeLayer(segment.Layer),
				Segment: segment,
			})
		}
	}
	return out
}

func clearanceSpatialIndex(segments []clearanceSegment, cellSize float64) clearanceGridIndex {
	index := clearanceGridIndex{cellSize: cellSize, cells: map[clearanceCellKey][]int{}}
	for segmentIndex, segment := range segments {
		index.addSegment(segmentIndex, segment.Layer, segment.Segment)
	}
	return index
}

func newClearanceQueryScratch(segmentCount int) *clearanceQueryScratch {
	return &clearanceQueryScratch{
		marks:     make([]int, segmentCount),
		cellMarks: map[clearanceCellKey]int{},
	}
}

func (scratch *clearanceQueryScratch) nextGeneration() {
	if scratch.generation == maxClearanceQueryGeneration {
		for i := range scratch.marks {
			scratch.marks[i] = 0
		}
		scratch.generation = 0
	}
	scratch.generation++
}

func (scratch *clearanceQueryScratch) nextCellGeneration() {
	if scratch.cellGeneration == maxClearanceQueryGeneration {
		clear(scratch.cellMarks)
		scratch.cellGeneration = 0
	}
	scratch.cellGeneration++
}

func (index clearanceGridIndex) addSegment(segmentIndex int, layer string, segment Segment) {
	index.forEachSegmentCell(segment, 0, func(key clearanceCellKey) {
		key.Layer = layer
		index.cells[key] = append(index.cells[key], segmentIndex)
	})
}

func (index clearanceGridIndex) query(layer string, segment Segment, marginMM float64, scratch *clearanceQueryScratch) []int {
	scratch.nextGeneration()
	scratch.nextCellGeneration()
	scratch.out = scratch.out[:0]
	index.forEachSegmentCell(segment, marginMM, func(key clearanceCellKey) {
		key.Layer = layer
		if scratch.cellMarks[key] == scratch.cellGeneration {
			return
		}
		scratch.cellMarks[key] = scratch.cellGeneration
		for _, segmentIndex := range index.cells[key] {
			if scratch.marks[segmentIndex] == scratch.generation {
				continue
			}
			scratch.marks[segmentIndex] = scratch.generation
			scratch.out = append(scratch.out, segmentIndex)
		}
	})
	return scratch.out
}

func (index clearanceGridIndex) forEachSegmentCell(segment Segment, marginMM float64, fn func(clearanceCellKey)) {
	cellSize := max(index.cellSize, distanceEpsilon)
	length := pointDistance(segment.Start, segment.End)
	steps := int(math.Ceil(length / (cellSize / 2)))
	if steps < 1 {
		steps = 1
	}
	radiusCells := max(0, int(math.Ceil(marginMM/cellSize)))
	var lastKey clearanceCellKey
	hasLastKey := false
	var lastCenterX int
	var lastCenterY int
	hasLastCenter := false
	for step := 0; step <= steps; step++ {
		t := float64(step) / float64(steps)
		point := Point{
			XMM: segment.Start.XMM + (segment.End.XMM-segment.Start.XMM)*t,
			YMM: segment.Start.YMM + (segment.End.YMM-segment.Start.YMM)*t,
		}
		centerX := int(math.Floor(point.XMM / cellSize))
		centerY := int(math.Floor(point.YMM / cellSize))
		if hasLastCenter && centerX == lastCenterX && centerY == lastCenterY {
			continue
		}
		lastCenterX = centerX
		lastCenterY = centerY
		hasLastCenter = true
		for offsetX := -radiusCells; offsetX <= radiusCells; offsetX++ {
			for offsetY := -radiusCells; offsetY <= radiusCells; offsetY++ {
				key := clearanceCellKey{X: centerX + offsetX, Y: centerY + offsetY}
				if hasLastKey && key == lastKey {
					continue
				}
				fn(key)
				lastKey = key
				hasLastKey = true
			}
		}
	}
}

func segmentDistance(left Segment, right Segment) float64 {
	if segmentsIntersect(left.Start, left.End, right.Start, right.End) {
		return 0
	}
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

func segmentBoundsWithin(left Segment, right Segment, marginMM float64) bool {
	leftMinX := min(left.Start.XMM, left.End.XMM) - marginMM
	leftMaxX := max(left.Start.XMM, left.End.XMM) + marginMM
	leftMinY := min(left.Start.YMM, left.End.YMM) - marginMM
	leftMaxY := max(left.Start.YMM, left.End.YMM) + marginMM
	rightMinX := min(right.Start.XMM, right.End.XMM)
	rightMaxX := max(right.Start.XMM, right.End.XMM)
	rightMinY := min(right.Start.YMM, right.End.YMM)
	rightMaxY := max(right.Start.YMM, right.End.YMM)
	return leftMinX <= rightMaxX && leftMaxX >= rightMinX &&
		leftMinY <= rightMaxY && leftMaxY >= rightMinY
}

func segmentsIntersect(a1 Point, a2 Point, b1 Point, b2 Point) bool {
	o1 := orientation(a1, a2, b1)
	o2 := orientation(a1, a2, b2)
	o3 := orientation(b1, b2, a1)
	o4 := orientation(b1, b2, a2)
	if o1 != o2 && o3 != o4 {
		return true
	}
	if o1 == 0 && pointOnSegment(b1, a1, a2) {
		return true
	}
	if o2 == 0 && pointOnSegment(b2, a1, a2) {
		return true
	}
	if o3 == 0 && pointOnSegment(a1, b1, b2) {
		return true
	}
	if o4 == 0 && pointOnSegment(a2, b1, b2) {
		return true
	}
	return false
}

func orientation(a Point, b Point, c Point) int {
	abX := b.XMM - a.XMM
	abY := b.YMM - a.YMM
	acX := c.XMM - a.XMM
	acY := c.YMM - a.YMM
	value := abX*acY - abY*acX
	scaleSq := abX*abX + abY*abY
	if scaleSq <= distanceEpsilonSquared || value*value <= distanceEpsilonSquared*scaleSq {
		return 0
	}
	if value > 0 {
		return 1
	}
	return 2
}

func pointOnSegment(point Point, start Point, end Point) bool {
	return point.XMM <= max(start.XMM, end.XMM)+distanceEpsilon &&
		point.XMM >= min(start.XMM, end.XMM)-distanceEpsilon &&
		point.YMM <= max(start.YMM, end.YMM)+distanceEpsilon &&
		point.YMM >= min(start.YMM, end.YMM)-distanceEpsilon
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
