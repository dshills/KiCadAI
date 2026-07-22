package routing

import (
	"fmt"
	"math"

	"kicadai/internal/reports"
)

type ValidationReport struct {
	Issues []reports.Issue `json:"issues,omitempty"`
}

// ValidatePhysicalClearance checks the complete emitted copper set against
// itself and every foreign-net pad. It is intended for workflows that compose
// multiple independently routed phases before writing one board.
func ValidatePhysicalClearance(request Request, routes []Route) []reports.Issue {
	return validatePhysicalClearance(request, routes, true)
}

// ValidatePhysicalTrackClearance validates cross-copper geometry and tracks
// against foreign pads. Layer-transition via-to-pad clearance is owned by the
// transition repair path, which can move the via and its attached vertices.
func ValidatePhysicalTrackClearance(request Request, routes []Route) []reports.Issue {
	return validatePhysicalClearance(request, routes, false)
}

func validatePhysicalClearance(request Request, routes []Route, includeViaPad bool) []reports.Issue {
	request = cloneRequest(request)
	NormalizeRequest(&request)
	issues := clearanceIssues(routes, request.Rules.ClearanceMM)
	for _, route := range routes {
		for _, segment := range route.Segments {
			for _, component := range request.Components {
				for _, pad := range component.Pads {
					if sameOccupancyNet(pad.Net, route.Net) || !padAppliesToCopperLayer(pad, segment.Layer, request.Board.Layers) {
						continue
					}
					clearanceMM := request.Rules.ClearanceMM
					if pad.Clearance != nil {
						clearanceMM = max(clearanceMM, *pad.Clearance)
					}
					if segmentShapeDistance(segment, padRect(component, pad))-segment.WidthMM/2 < clearanceMM-distanceEpsilon {
						center := absolutePadPoint(component, pad.Position)
						issues = append(issues, reports.Issue{
							Code:       reports.CodeValidationFailed,
							Severity:   reports.SeverityBlocked,
							Message:    fmt.Sprintf("segment %s to %s clearance violation with pad %s.%s at %s", formatClearancePoint(segment.Start), formatClearancePoint(segment.End), component.Ref, pad.Name, formatClearancePoint(center)),
							Refs:       []string{component.Ref},
							Nets:       []string{route.Net, pad.Net},
							Suggestion: "reroute the conflicting net or move the foreign pad",
						})
					}
				}
			}
		}
		if !includeViaPad {
			continue
		}
		for _, via := range route.Vias {
			probe := Segment{Start: via.At, End: via.At}
			for _, component := range request.Components {
				for _, pad := range component.Pads {
					if sameOccupancyNet(pad.Net, route.Net) || !viaAndPadShareCopperLayer(via, pad, request.Board.Layers) {
						continue
					}
					clearanceMM := request.Rules.ClearanceMM
					if pad.Clearance != nil {
						clearanceMM = max(clearanceMM, *pad.Clearance)
					}
					if segmentShapeDistance(probe, padRect(component, pad))-via.DiameterMM/2 < clearanceMM-distanceEpsilon {
						issues = append(issues, reports.Issue{
							Code:       reports.CodeValidationFailed,
							Severity:   reports.SeverityBlocked,
							Message:    fmt.Sprintf("via clearance violation with pad %s.%s", component.Ref, pad.Name),
							Refs:       []string{component.Ref},
							Nets:       []string{route.Net, pad.Net},
							Suggestion: "move the via or reroute the conflicting net",
						})
					}
				}
			}
		}
	}
	return issues
}

// ValidatePhysicalClearanceForNet checks only violations whose geometry is on
// netName. It is equivalent to the affected subset of ValidatePhysicalClearance
// and lets deterministic repair search score one changed net without repeatedly
// revalidating unrelated copper.
func ValidatePhysicalClearanceForNet(request Request, routes []Route, netName string) []reports.Issue {
	return validatePhysicalClearanceForNet(request, routes, netName, true)
}

// ValidatePhysicalTrackClearanceForNet is the affected-net form of
// ValidatePhysicalTrackClearance for deterministic repair search.
func ValidatePhysicalTrackClearanceForNet(request Request, routes []Route, netName string) []reports.Issue {
	return validatePhysicalClearanceForNet(request, routes, netName, false)
}

// ValidatePhysicalTrackClearanceForSegment reports only blockers involving the
// supplied segment. It is used to avoid launching repair search for unrelated
// segments merely because another segment on the same net is blocked.
func ValidatePhysicalTrackClearanceForSegment(request Request, routes []Route, segment Segment) []reports.Issue {
	localized := make([]Route, 0, len(routes)+1)
	for _, route := range routes {
		if sameOccupancyNet(route.Net, segment.Net) {
			continue
		}
		localized = append(localized, route)
	}
	localized = append(localized, Route{Net: segment.Net, Status: RouteStatusRouted, Segments: []Segment{segment}})
	return validatePhysicalClearanceForNet(request, localized, segment.Net, false)
}

func validatePhysicalClearanceForNet(request Request, routes []Route, netName string, includeViaPad bool) []reports.Issue {
	request = cloneRequest(request)
	NormalizeRequest(&request)
	issues := []reports.Issue{}
	for _, route := range routes {
		if !sameOccupancyNet(route.Net, netName) {
			continue
		}
		for _, segment := range route.Segments {
			for _, otherRoute := range routes {
				if sameOccupancyNet(otherRoute.Net, route.Net) {
					continue
				}
				for _, other := range otherRoute.Segments {
					if normalizeLayer(segment.Layer) != normalizeLayer(other.Layer) {
						continue
					}
					requiredGap := request.Rules.ClearanceMM + segment.WidthMM/2 + other.WidthMM/2
					if !segmentBoundsWithin(segment, other, requiredGap) {
						continue
					}
					if segmentDistance(segment, other)-segment.WidthMM/2-other.WidthMM/2 < request.Rules.ClearanceMM-distanceEpsilon {
						issues = append(issues, routeValidationIssue(route.Net, reports.CodeValidationFailed, fmt.Sprintf(
							"segment clearance violation with net %s: %s to %s crosses %s to %s",
							otherRoute.Net, formatClearancePoint(segment.Start), formatClearancePoint(segment.End), formatClearancePoint(other.Start), formatClearancePoint(other.End),
						)))
					}
				}
				for _, via := range otherRoute.Vias {
					if !viaTouchesLayer(via, segment.Layer) {
						continue
					}
					if distancePointToSegment(via.At, segment.Start, segment.End)-via.DiameterMM/2-segment.WidthMM/2 < request.Rules.ClearanceMM-distanceEpsilon {
						issues = append(issues, routeValidationIssue(route.Net, reports.CodeValidationFailed, "segment clearance violation with via on net "+otherRoute.Net))
					}
				}
			}
			for _, component := range request.Components {
				for _, pad := range component.Pads {
					if sameOccupancyNet(pad.Net, route.Net) || !padAppliesToCopperLayer(pad, segment.Layer, request.Board.Layers) {
						continue
					}
					clearanceMM := request.Rules.ClearanceMM
					if pad.Clearance != nil {
						clearanceMM = max(clearanceMM, *pad.Clearance)
					}
					if segmentShapeDistance(segment, padRect(component, pad))-segment.WidthMM/2 < clearanceMM-distanceEpsilon {
						center := absolutePadPoint(component, pad.Position)
						issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Message: fmt.Sprintf("segment %s to %s clearance violation with pad %s.%s at %s", formatClearancePoint(segment.Start), formatClearancePoint(segment.End), component.Ref, pad.Name, formatClearancePoint(center)), Refs: []string{component.Ref}, Nets: []string{route.Net, pad.Net}, Suggestion: "reroute the conflicting net or move the foreign pad"})
					}
				}
			}
		}
		if !includeViaPad {
			continue
		}
		for _, via := range route.Vias {
			for _, otherRoute := range routes {
				if sameOccupancyNet(otherRoute.Net, route.Net) {
					continue
				}
				for _, segment := range otherRoute.Segments {
					if !viaTouchesLayer(via, segment.Layer) {
						continue
					}
					if distancePointToSegment(via.At, segment.Start, segment.End)-via.DiameterMM/2-segment.WidthMM/2 < request.Rules.ClearanceMM-distanceEpsilon {
						issues = append(issues, routeValidationIssue(route.Net, reports.CodeValidationFailed, "via clearance violation with segment on net "+otherRoute.Net))
					}
				}
				for _, other := range otherRoute.Vias {
					if !viasShareLayer(via, other) {
						continue
					}
					if pointDistance(via.At, other.At)-via.DiameterMM/2-other.DiameterMM/2 < request.Rules.ClearanceMM-distanceEpsilon {
						issues = append(issues, routeValidationIssue(route.Net, reports.CodeValidationFailed, "via clearance violation with net "+otherRoute.Net))
					}
				}
			}
			probe := Segment{Start: via.At, End: via.At}
			for _, component := range request.Components {
				for _, pad := range component.Pads {
					if sameOccupancyNet(pad.Net, route.Net) || !viaAndPadShareCopperLayer(via, pad, request.Board.Layers) {
						continue
					}
					clearanceMM := request.Rules.ClearanceMM
					if pad.Clearance != nil {
						clearanceMM = max(clearanceMM, *pad.Clearance)
					}
					if segmentShapeDistance(probe, padRect(component, pad))-via.DiameterMM/2 < clearanceMM-distanceEpsilon {
						issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Message: fmt.Sprintf("via clearance violation with pad %s.%s", component.Ref, pad.Name), Refs: []string{component.Ref}, Nets: []string{route.Net, pad.Net}, Suggestion: "move the via or reroute the conflicting net"})
					}
				}
			}
		}
	}
	return issues
}

// PhysicalPadDetourCandidates returns deterministic, grid-aligned local
// doglegs around foreign pads that currently violate segment clearance. The
// detour rejoins the original segment outside the pad envelope, so distant
// endpoint geometry is preserved.
func PhysicalPadDetourCandidates(request Request, segment Segment, maxRing int) [][]Point {
	request = cloneRequest(request)
	NormalizeRequest(&request)
	dx := segment.End.XMM - segment.Start.XMM
	dy := segment.End.YMM - segment.Start.YMM
	length := math.Hypot(dx, dy)
	if length <= distanceEpsilon || maxRing <= 0 {
		return nil
	}
	gridMM := request.Rules.GridMM
	if gridMM <= 0 || math.IsNaN(gridMM) || math.IsInf(gridMM, 0) {
		gridMM = DefaultRules().GridMM
	}
	ux, uy := dx/length, dy/length
	px, py := -uy, ux
	var candidates [][]Point
	seen := map[string]struct{}{}
	for _, component := range request.Components {
		for _, pad := range component.Pads {
			if sameOccupancyNet(pad.Net, segment.Net) || !padAppliesToCopperLayer(pad, segment.Layer, request.Board.Layers) {
				continue
			}
			clearanceMM := request.Rules.ClearanceMM
			if pad.Clearance != nil {
				clearanceMM = max(clearanceMM, *pad.Clearance)
			}
			shape := padRect(component, pad)
			if segmentShapeDistance(segment, shape)-segment.WidthMM/2 >= clearanceMM-distanceEpsilon {
				continue
			}
			vertices := shape.Polygon
			if len(vertices) == 0 && shape.Rect != nil {
				vertices = []Point{
					{XMM: shape.Rect.Min.XMM, YMM: shape.Rect.Min.YMM},
					{XMM: shape.Rect.Max.XMM, YMM: shape.Rect.Min.YMM},
					{XMM: shape.Rect.Max.XMM, YMM: shape.Rect.Max.YMM},
					{XMM: shape.Rect.Min.XMM, YMM: shape.Rect.Max.YMM},
				}
			}
			if len(vertices) == 0 {
				continue
			}
			minAlong, maxAlong := math.Inf(1), math.Inf(-1)
			for _, vertex := range vertices {
				relX := vertex.XMM - segment.Start.XMM
				relY := vertex.YMM - segment.Start.YMM
				along := relX*ux + relY*uy
				minAlong = min(minAlong, along)
				maxAlong = max(maxAlong, along)
			}
			margin := clearanceMM + segment.WidthMM/2 + gridMM
			entryDistance := max(gridMM, minAlong-margin)
			exitDistance := min(length-gridMM, maxAlong+margin)
			if entryDistance >= exitDistance-distanceEpsilon {
				continue
			}
			entry := Point{XMM: segment.Start.XMM + ux*entryDistance, YMM: segment.Start.YMM + uy*entryDistance}
			exit := Point{XMM: segment.Start.XMM + ux*exitDistance, YMM: segment.Start.YMM + uy*exitDistance}
			for ring := 1; ring <= maxRing; ring++ {
				offsetMM := float64(ring) * gridMM
				for _, direction := range []float64{1, -1} {
					offsetX, offsetY := direction*px*offsetMM, direction*py*offsetMM
					candidate := []Point{
						entry,
						{XMM: entry.XMM + offsetX, YMM: entry.YMM + offsetY},
						{XMM: exit.XMM + offsetX, YMM: exit.YMM + offsetY},
						exit,
					}
					complete := append([]Point{segment.Start}, candidate...)
					complete = append(complete, segment.End)
					if !detourClearsForeignPads(request, segment, complete) {
						continue
					}
					key := fmt.Sprintf("%.6f,%.6f:%.6f,%.6f:%.6f,%.6f:%.6f,%.6f", candidate[0].XMM, candidate[0].YMM, candidate[1].XMM, candidate[1].YMM, candidate[2].XMM, candidate[2].YMM, candidate[3].XMM, candidate[3].YMM)
					if _, exists := seen[key]; exists {
						continue
					}
					seen[key] = struct{}{}
					candidates = append(candidates, candidate)
				}
			}
		}
	}
	return candidates
}

func detourClearsForeignPads(request Request, segment Segment, points []Point) bool {
	for pointIndex := 1; pointIndex < len(points); pointIndex++ {
		candidate := Segment{Net: segment.Net, Layer: segment.Layer, Start: points[pointIndex-1], End: points[pointIndex], WidthMM: segment.WidthMM}
		for _, component := range request.Components {
			for _, pad := range component.Pads {
				if sameOccupancyNet(pad.Net, segment.Net) || !padAppliesToCopperLayer(pad, segment.Layer, request.Board.Layers) {
					continue
				}
				clearanceMM := request.Rules.ClearanceMM
				if pad.Clearance != nil {
					clearanceMM = max(clearanceMM, *pad.Clearance)
				}
				if segmentShapeDistance(candidate, padRect(component, pad))-candidate.WidthMM/2 < clearanceMM-distanceEpsilon {
					return false
				}
			}
		}
	}
	return true
}

func padAppliesToCopperLayer(pad Pad, layer string, boardLayers []Layer) bool {
	wanted := normalizeLayer(layer)
	for _, candidate := range padAccessLayers(pad, routableLayerNames(boardLayers)) {
		if normalizeLayer(candidate) == wanted {
			return true
		}
	}
	return false
}

func viaAndPadShareCopperLayer(via Via, pad Pad, boardLayers []Layer) bool {
	if throughVia(via) {
		for _, layer := range routableLayerNames(boardLayers) {
			if padAppliesToCopperLayer(pad, layer, boardLayers) {
				return true
			}
		}
	}
	for _, layer := range via.Layers {
		if padAppliesToCopperLayer(pad, layer, boardLayers) {
			return true
		}
	}
	return false
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

func segmentShapeDistance(segment Segment, shape Shape) float64 {
	if len(shape.Polygon) >= 3 {
		if pointInPolygon(segment.Start, shape.Polygon) || pointInPolygon(segment.End, shape.Polygon) {
			return 0
		}
		best := math.Inf(1)
		for index, start := range shape.Polygon {
			end := shape.Polygon[(index+1)%len(shape.Polygon)]
			best = min(best, segmentDistance(segment, Segment{Start: start, End: end}))
		}
		return best
	}
	if shape.Rect == nil {
		return math.Inf(1)
	}
	rect := normalizeRect(*shape.Rect)
	if rect.ContainsPoint(segment.Start) || rect.ContainsPoint(segment.End) || segmentIntersectsRect(segment, rect) {
		return 0
	}
	corners := []Point{
		{XMM: rect.Min.XMM, YMM: rect.Min.YMM},
		{XMM: rect.Max.XMM, YMM: rect.Min.YMM},
		{XMM: rect.Max.XMM, YMM: rect.Max.YMM},
		{XMM: rect.Min.XMM, YMM: rect.Max.YMM},
	}
	best := math.Inf(1)
	for index, start := range corners {
		best = min(best, segmentDistance(segment, Segment{Start: start, End: corners[(index+1)%len(corners)]}))
	}
	return best
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
	maxHalfWidth := 0.0
	for _, candidate := range segments {
		maxHalfWidth = max(maxHalfWidth, candidate.Segment.WidthMM/2)
	}
	issues := []reports.Issue{}
	if len(segments) >= 2 {
		cellSize := clearanceIndexCellSize(clearanceMM)
		index := clearanceSpatialIndex(segments, cellSize)
		scratch := newClearanceQueryScratch(len(segments))
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
				if copperClearance < clearanceMM-distanceEpsilon {
					issues = append(issues, routeValidationIssue(left.Net, reports.CodeValidationFailed, fmt.Sprintf(
						"segment clearance violation with net %s: %s to %s crosses %s to %s",
						right.Net, formatClearancePoint(left.Segment.Start), formatClearancePoint(left.Segment.End), formatClearancePoint(right.Segment.Start), formatClearancePoint(right.Segment.End),
					)))
				}
			}
		}
	}
	vias := clearanceVias(routes)
	for _, segment := range segments {
		for _, via := range vias {
			if segment.Net == via.Net || !viaTouchesLayer(via.Via, segment.Layer) {
				continue
			}
			copperClearance := distancePointToSegment(via.Via.At, segment.Segment.Start, segment.Segment.End) - via.Via.DiameterMM/2 - segment.Segment.WidthMM/2
			if copperClearance < clearanceMM-distanceEpsilon {
				issues = append(issues, routeValidationIssue(segment.Net, reports.CodeValidationFailed, "segment clearance violation with via on net "+via.Net))
			}
		}
	}
	for leftIndex, left := range vias {
		for rightIndex := leftIndex + 1; rightIndex < len(vias); rightIndex++ {
			right := vias[rightIndex]
			if left.Net == right.Net || !viasShareLayer(left.Via, right.Via) {
				continue
			}
			copperClearance := pointDistance(left.Via.At, right.Via.At) - left.Via.DiameterMM/2 - right.Via.DiameterMM/2
			if copperClearance < clearanceMM-distanceEpsilon {
				issues = append(issues, routeValidationIssue(left.Net, reports.CodeValidationFailed, "via clearance violation with net "+right.Net))
			}
		}
	}
	return issues
}

func formatClearancePoint(point Point) string {
	return fmt.Sprintf("(%.6g,%.6g)", point.XMM, point.YMM)
}

type clearanceVia struct {
	Net string
	Via Via
}

func clearanceVias(routes []Route) []clearanceVia {
	var out []clearanceVia
	for _, route := range routes {
		for _, via := range route.Vias {
			out = append(out, clearanceVia{Net: route.Net, Via: via})
		}
	}
	return out
}

func viaTouchesLayer(via Via, layer string) bool {
	if throughVia(via) {
		return true
	}
	layer = normalizeLayer(layer)
	for _, candidate := range via.Layers {
		if normalizeLayer(candidate) == layer {
			return true
		}
	}
	return false
}

func viasShareLayer(left Via, right Via) bool {
	if (throughVia(left) && len(right.Layers) != 0) || (throughVia(right) && len(left.Layers) != 0) {
		return true
	}
	for _, layer := range left.Layers {
		if viaTouchesLayer(right, layer) {
			return true
		}
	}
	return false
}

func throughVia(via Via) bool {
	front, back := false, false
	for _, layer := range via.Layers {
		switch normalizeLayer(layer) {
		case normalizeLayer("F.Cu"):
			front = true
		case normalizeLayer("B.Cu"):
			back = true
		}
	}
	return front && back
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
