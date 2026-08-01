package schematiclayout

import (
	"container/heap"
	"math"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
)

const routeHardPenalty int64 = 1_000_000_000_000

func Route(request Request, result Result) Result {
	request = Classify(request)
	rules := normalizeRules(request.Rules)
	anchors := pinAnchors(result.Components)
	anchorIndex := newPinAnchorIndex(anchors)
	labeled := map[string]kicadfiles.Point{}
	nets := orderedNetsForRouting(request.Nets, anchors, request.Components, rules)
	for _, net := range nets {
		if len(net.Endpoints) == 0 {
			continue
		}
		orderedEndpoints, missingEndpoints := orderedRoutableEndpoints(net, anchors)
		if len(orderedEndpoints) == 0 {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityWarning, Code: "missing_pin_anchor", NetName: net.Name, Message: "net has no routable endpoint anchors"})
			continue
		}
		for _, endpoint := range missingEndpoints {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityWarning, Code: "missing_pin_anchor", NetName: net.Name, Ref: endpoint.Ref, Message: "net endpoint has no pin anchor"})
		}
		fromEndpoint := orderedEndpoints[0].endpoint
		start := orderedEndpoints[0].anchor
		if len(orderedEndpoints) == 1 {
			if net.PreferredLabels {
				appendEndpointLabel(&result, labeled, net.Name, fromEndpoint, start, request, rules)
			}
			continue
		}
		forceLabels := shouldUseLabels(net, anchors, request.Components, rules)
		for _, orderedEndpoint := range orderedEndpoints[1:] {
			toEndpoint := orderedEndpoint.endpoint
			end := orderedEndpoint.anchor
			if forceLabels {
				fromLabel := appendEndpointLabel(&result, labeled, net.Name, fromEndpoint, start, request, rules)
				toLabel := appendEndpointLabel(&result, labeled, net.Name, toEndpoint, end, request, rules)
				result.Connections = append(result.Connections, RoutedConnection{NetName: net.Name, From: fromEndpoint, To: toEndpoint, UseLabels: true, FromLabelAt: &fromLabel, ToLabelAt: &toLabel})
			} else {
				points, clean := routeConnectionPoints(net.Name, fromEndpoint, toEndpoint, start, end, result, request, rules, anchorIndex, net.PreferDirect || !rules.LabelFallbackEnabled)
				if !clean && rules.LabelFallbackEnabled {
					fromLabel := appendEndpointLabel(&result, labeled, net.Name, fromEndpoint, start, request, rules)
					toLabel := appendEndpointLabel(&result, labeled, net.Name, toEndpoint, end, request, rules)
					result.Connections = append(result.Connections, RoutedConnection{NetName: net.Name, From: fromEndpoint, To: toEndpoint, UseLabels: true, FromLabelAt: &fromLabel, ToLabelAt: &toLabel})
					result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityInfo, Code: "route_label_fallback", NetName: net.Name, Message: "local route obstacles required label fallback"})
				} else {
					result.Connections = append(result.Connections, RoutedConnection{NetName: net.Name, From: fromEndpoint, To: toEndpoint, Points: points})
					result.Wires = append(result.Wires, segmentsForPoints(net.Name, points)...)
				}
			}
			fromEndpoint = toEndpoint
			start = end
		}
		if !forceLabels && net.PreferredLabels && !net.PreferDirect {
			appendRouteAnnotation(&result, net.Name, request, rules)
		}
	}
	result.Wires = compactWireSegments(result.Wires)
	result.Junctions = append(result.Junctions, branchJunctions(result.Wires)...)
	recordReadabilityMetrics(&result, request)
	result = Validate(result, request)
	return NormalizeResult(result, rules)
}

// orderedNetsForRouting reserves short endpoint-label stubs before laying
// continuous local conductors. This lets direct routes see those stubs as
// occupied geometry and route around them, instead of adding a late label
// stub that electrically contacts an already-routed net.
func orderedNetsForRouting(nets []Net, anchors map[Endpoint]kicadfiles.Point, components []Component, rules Rules) []Net {
	ordered := append([]Net(nil), nets...)
	labelFirst := func(net Net) bool {
		return (len(net.Endpoints) == 1 && net.PreferredLabels) ||
			shouldUseLabels(net, anchors, components, rules)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		leftLabels, rightLabels := labelFirst(ordered[i]), labelFirst(ordered[j])
		if leftLabels != rightLabels {
			return leftLabels
		}
		return compareNets(ordered[i], ordered[j]) < 0
	})
	return ordered
}

func appendRouteAnnotation(result *Result, netName string, request Request, rules Rules) {
	type candidate struct {
		position   kicadfiles.Point
		clearRun   kicadfiles.IU
		midSegment bool
	}
	candidates := make([]candidate, 0)
	fallbackCandidates := make([]candidate, 0)
	for _, connection := range result.Connections {
		if connection.NetName != netName || connection.UseLabels {
			continue
		}
		for index := 1; index < len(connection.Points)-1; index++ {
			candidates = append(candidates, candidate{
				position: connection.Points[index],
				clearRun: manhattan(connection.Points[index-1], connection.Points[index]) +
					manhattan(connection.Points[index], connection.Points[index+1]),
			})
		}
		for index := 1; index < len(connection.Points); index++ {
			from, to := connection.Points[index-1], connection.Points[index]
			position := from
			switch {
			case from.X == to.X:
				position.Y = SnapIU(from.Y+(to.Y-from.Y)/2, rules.Grid)
			case from.Y == to.Y:
				position.X = SnapIU(from.X+(to.X-from.X)/2, rules.Grid)
			default:
				continue
			}
			if position == from || position == to {
				continue
			}
			candidates = append(candidates, candidate{
				position:   position,
				clearRun:   manhattan(from, to),
				midSegment: true,
			})
			segmentLength := manhattan(from, to)
			for _, inset := range []kicadfiles.IU{rules.Grid, rules.Grid * 2} {
				if inset <= 0 || segmentLength <= inset*2 {
					continue
				}
				nearFrom, nearTo := from, to
				switch {
				case from.X == to.X:
					direction := kicadfiles.IU(1)
					if to.Y < from.Y {
						direction = -1
					}
					nearFrom.Y += direction * inset
					nearTo.Y -= direction * inset
				case from.Y == to.Y:
					direction := kicadfiles.IU(1)
					if to.X < from.X {
						direction = -1
					}
					nearFrom.X += direction * inset
					nearTo.X -= direction * inset
				default:
					continue
				}
				fallbackCandidates = append(fallbackCandidates,
					candidate{position: nearFrom, clearRun: segmentLength},
					candidate{position: nearTo, clearRun: segmentLength},
				)
			}
		}
	}
	compareCandidates := func(values []candidate) func(int, int) bool {
		return func(i, j int) bool {
			if values[i].midSegment != values[j].midSegment {
				return values[i].midSegment
			}
			if values[i].clearRun != values[j].clearRun {
				return values[i].clearRun > values[j].clearRun
			}
			return comparePoints(values[i].position, values[j].position) < 0
		}
	}
	sort.SliceStable(candidates, compareCandidates(candidates))
	sort.SliceStable(fallbackCandidates, compareCandidates(fallbackCandidates))
	candidates = append(candidates, fallbackCandidates...)
	if len(candidates) > rules.MaxRouteAnnotations {
		candidates = candidates[:rules.MaxRouteAnnotations]
	}
	for _, candidate := range candidates {
		box := TextEstimate(netName, candidate.position, 0, 0)
		if !UsableSheet(request.Sheet).ContainsRect(box) || routeAnnotationCollides(box, candidate.position, netName, *result) {
			continue
		}
		result.Labels = append(result.Labels, Label{
			NetName:         netName,
			Text:            netName,
			Position:        candidate.position,
			RouteAnnotation: true,
		})
		return
	}
}

func routeAnnotationCollides(box Rect, position kicadfiles.Point, netName string, result Result) bool {
	for _, component := range result.Components {
		if box.Intersects(componentBody(component)) {
			return true
		}
		for _, text := range []TextBox{component.ReferenceText, component.ValueText} {
			if !text.Box.Empty() && box.Intersects(text.Box.Translate(component.PlacedAt)) {
				return true
			}
		}
	}
	for _, label := range result.Labels {
		if box.Intersects(TextEstimateOriented(label.Text, label.Position, label.Rotation, label.JustifyRight)) {
			return true
		}
	}
	sameNetContacts := 0
	for _, wire := range result.Wires {
		if wire.NetName == netName {
			if pointOnSegment(wire.From, position, wire.To) {
				sameNetContacts++
			}
			continue
		}
		if pointOnSegment(wire.From, position, wire.To) || SegmentIntersectsRect(wire, box) {
			return true
		}
	}
	return sameNetContacts > 1
}

func recordReadabilityMetrics(result *Result, request Request) {
	connectionsByNet := map[string][]RoutedConnection{}
	for _, connection := range result.Connections {
		connectionsByNet[connection.NetName] = append(connectionsByNet[connection.NetName], connection)
	}
	for _, net := range request.Nets {
		role := normalizeRole(net.Role)
		local := !net.EndpointLabels && len(net.Endpoints) < 8 &&
			!containsNormalizedRole(role, "bus", "global", "cross_sheet")
		connections := connectionsByNet[net.Name]
		visible := len(connections) >= len(net.Endpoints)-1
		for _, connection := range connections {
			if connection.UseLabels || len(connection.Points) < 2 {
				visible = false
				break
			}
		}
		if local && len(net.Endpoints) == 2 {
			result.Report.LocalTwoPointNetCount++
			if visible {
				result.Report.ContinuousLocalNetCount++
			}
		}
		if local && len(net.Endpoints) > 2 {
			result.Report.LocalMultiPointNetCount++
			if visible {
				result.Report.RouteTreeNetCount++
			}
		}
		if containsNormalizedRole(role, "feedback", "sense") {
			result.Report.FeedbackPathCount++
			if visible {
				result.Report.VisibleFeedbackPathCount++
			}
		}
	}
	for _, label := range result.Labels {
		if label.RouteAnnotation {
			result.Report.NetAnnotationCount++
		} else {
			result.Report.EndpointLabelCount++
		}
	}
	usable := UsableSheet(request.Sheet)
	result.Report.UsablePageAreaMM2 = rectAreaMM2(usable)
	result.Report.OccupiedAreaMM2 = rectAreaMM2(result.Report.OccupiedBounds)
	if result.Report.UsablePageAreaMM2 > 0 {
		result.Report.OccupiedPageRatio = result.Report.OccupiedAreaMM2 / result.Report.UsablePageAreaMM2
		if result.Report.OccupiedPageRatio > 1 {
			result.Report.OccupiedPageRatio = 1
		}
		result.Report.WhitespaceRatio = 1 - result.Report.OccupiedPageRatio
	}
	componentArea := 0.0
	for _, component := range result.Components {
		componentArea += rectAreaMM2(componentBody(component))
	}
	if result.Report.OccupiedAreaMM2 > 0 {
		result.Report.ComponentDispersion = componentArea / result.Report.OccupiedAreaMM2
	}
	recordBoundaryPlacementMetrics(result, request.Rules)
}

func rectAreaMM2(rect Rect) float64 {
	if rect.Empty() {
		return 0
	}
	return float64(rect.MaxX-rect.MinX) / 1_000_000 * float64(rect.MaxY-rect.MinY) / 1_000_000
}

func recordBoundaryPlacementMetrics(result *Result, rules Rules) {
	if result.Report.OccupiedBounds.Empty() {
		return
	}
	tolerance := rules.BoundaryTolerance
	for _, component := range result.Components {
		role := normalizeRole(component.Role)
		body := componentBody(component)
		switch {
		case containsNormalizedRole(role, "input_connector"):
			if body.MinX-result.Report.OccupiedBounds.MinX > tolerance {
				result.Report.BoundaryPlacementViolations++
			}
		case containsNormalizedRole(role, "output_connector"):
			if result.Report.OccupiedBounds.MaxX-body.MaxX > tolerance {
				result.Report.BoundaryPlacementViolations++
			}
		}
	}
}

func appendEndpointLabel(result *Result, seen map[string]kicadfiles.Point, netName string, endpoint Endpoint, anchor kicadfiles.Point, request Request, rules Rules) kicadfiles.Point {
	key := netName + "\x00" + endpoint.Ref + "\x00" + endpoint.Pin
	if position, ok := seen[key]; ok {
		return position
	}
	position, clean := labelStubPoint(netName, endpoint, anchor, *result, request, rules)
	seen[key] = position
	rotation := kicadfiles.Angle(0)
	justifyRight := false
	if rules.OrientEndpointLabels {
		rotation, justifyRight = labelOrientationForStub(anchor, position)
	}
	result.Labels = append(result.Labels, Label{NetName: netName, Text: netName, Position: position, Rotation: rotation, JustifyRight: justifyRight})
	if anchor != position {
		result.Wires = append(result.Wires, WireSegment{NetName: netName, From: anchor, To: position})
	}
	if !clean {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityWarning, Code: "label_placement_fallback", Ref: endpoint.Ref, NetName: netName, Message: "label stub required crowded fallback placement"})
	}
	return position
}

func labelStubPoint(netName string, endpoint Endpoint, anchor kicadfiles.Point, result Result, request Request, rules Rules) (kicadfiles.Point, bool) {
	grid := rules.MinorGrid
	if grid <= 0 {
		grid = kicadfiles.MM(1.27)
	}
	preferred := kicadfiles.Point{X: grid}
	hasPinDirection := false
	if direction, ok := endpointLabelDirection(endpoint, result.Components, grid); ok {
		preferred = direction
		hasPinDirection = true
	}
	for _, component := range result.Components {
		if component.Ref != endpoint.Ref {
			continue
		}
		if hasPinDirection {
			break
		}
		body := componentBody(component)
		if !body.Empty() {
			preferred = labelDirectionFromBody(anchor, body, grid)
			break
		}
		offset := InverseTransformPoint(kicadfiles.Point{X: anchor.X - component.PlacedAt.X, Y: anchor.Y - component.PlacedAt.Y}, component.Rotation, component.Mirror)
		switch {
		case absIU(offset.X) >= absIU(offset.Y) && offset.X < 0:
			preferred = kicadfiles.Point{X: -grid}
		case absIU(offset.X) >= absIU(offset.Y):
			preferred = kicadfiles.Point{X: grid}
		case offset.Y < 0:
			preferred = kicadfiles.Point{Y: -grid}
		default:
			preferred = kicadfiles.Point{Y: grid}
		}
		preferred = TransformPoint(preferred, component.Rotation, component.Mirror)
		break
	}
	directions := []kicadfiles.Point{preferred}
	alternatives := []kicadfiles.Point{{X: grid}, {X: -grid}, {Y: -grid}, {Y: grid}}
	for _, direction := range alternatives {
		for _, component := range result.Components {
			if component.Ref == endpoint.Ref {
				direction = TransformPoint(direction, component.Rotation, component.Mirror)
				break
			}
		}
		duplicate := false
		for _, existing := range directions {
			if existing == direction {
				duplicate = true
				break
			}
		}
		if !duplicate {
			directions = append(directions, direction)
		}
	}
	usable := UsableSheet(request.Sheet)
	for _, direction := range directions {
		for _, scale := range []kicadfiles.IU{1, 2, 3, 4, 6, 8, 12, 16} {
			position := kicadfiles.Point{X: anchor.X + direction.X*scale, Y: anchor.Y + direction.Y*scale}
			segment := WireSegment{NetName: netName, From: anchor, To: position}
			rotation := kicadfiles.Angle(0)
			justifyRight := false
			if rules.OrientEndpointLabels {
				rotation, justifyRight = labelOrientationForStub(anchor, position)
			}
			labelBox := TextEstimateOriented(netName, position, rotation, justifyRight)
			if !usable.ContainsRect(labelBox) || labelPlacementCollides(labelBox, segment, endpoint, result, request) {
				continue
			}
			return position, true
		}
	}
	return kicadfiles.Point{X: anchor.X + preferred.X*2, Y: anchor.Y + preferred.Y*2}, false
}

func labelOrientationForStub(anchor, label kicadfiles.Point) (kicadfiles.Angle, bool) {
	dx, dy := label.X-anchor.X, label.Y-anchor.Y
	switch {
	case dx < 0:
		return 180, true
	case dx > 0:
		return 0, false
	case dy < 0:
		return 270, true
	case dy > 0:
		return 90, false
	default:
		return 0, false
	}
}

// endpointLabelDirection obtains the intended outward pin direction before
// falling back to the component body. This matters for calibrated templates
// whose KiCad connection anchor is mirrored from the raw pin coordinate: the
// anchor alone can be closer to the wrong body edge.
func endpointLabelDirection(endpoint Endpoint, components []PlacedComponent, grid kicadfiles.IU) (kicadfiles.Point, bool) {
	for _, component := range components {
		if component.Ref != endpoint.Ref {
			continue
		}
		for _, pin := range component.Pins {
			if pin.Number != endpoint.Pin || (pin.Direction.X == 0 && pin.Direction.Y == 0) {
				continue
			}
			direction := TransformPoint(pin.Direction, component.Rotation, component.Mirror)
			if absIU(direction.X) >= absIU(direction.Y) {
				if direction.X < 0 {
					return kicadfiles.Point{X: -grid}, true
				}
				return kicadfiles.Point{X: grid}, true
			}
			if direction.Y < 0 {
				return kicadfiles.Point{Y: -grid}, true
			}
			return kicadfiles.Point{Y: grid}, true
		}
	}
	return kicadfiles.Point{}, false
}

func labelDirectionFromBody(anchor kicadfiles.Point, body Rect, grid kicadfiles.IU) kicadfiles.Point {
	type edge struct {
		distance  kicadfiles.IU
		direction kicadfiles.Point
	}
	edges := []edge{
		{distance: absIU(anchor.X - body.MinX), direction: kicadfiles.Point{X: -grid}},
		{distance: absIU(anchor.X - body.MaxX), direction: kicadfiles.Point{X: grid}},
		{distance: absIU(anchor.Y - body.MinY), direction: kicadfiles.Point{Y: -grid}},
		{distance: absIU(anchor.Y - body.MaxY), direction: kicadfiles.Point{Y: grid}},
	}
	best := edges[0]
	for _, candidate := range edges[1:] {
		if candidate.distance < best.distance {
			best = candidate
		}
	}
	return best.direction
}

func labelPlacementCollides(labelBox Rect, stub WireSegment, endpoint Endpoint, result Result, request Request) bool {
	if _, intersectsPin := unrelatedPinForWire(stub, stub.NetName, result, request); intersectsPin {
		return true
	}
	for _, component := range result.Components {
		body := componentBody(component)
		if labelBox.Intersects(body) {
			return true
		}
		if SegmentIntersectsRect(stub, body) {
			if component.Ref != endpoint.Ref || !wireLeavesAttachedSymbol(stub, ValidationObject{Ref: component.Ref, Box: body}, result.Components) {
				return true
			}
		}
		for _, text := range []TextBox{component.ReferenceText, component.ValueText} {
			if !text.Box.Empty() && labelBox.Intersects(text.Box.Translate(component.PlacedAt)) {
				return true
			}
		}
	}
	for _, label := range result.Labels {
		if labelBox.Intersects(TextEstimateOriented(label.Text, label.Position, label.Rotation, label.JustifyRight)) {
			return true
		}
	}
	for _, wire := range result.Wires {
		// Different nets may not touch at endpoints or overlap collinearly.  KiCad
		// treats either case as an electrical connection, even though the visual
		// crossing helper intentionally ignores shared endpoints.
		if wire.NetName != stub.NetName && segmentsIntersect(stub.From, stub.To, wire.From, wire.To) {
			return true
		}
	}
	return false
}

func routeConnectionPoints(netName string, from, to Endpoint, start, end kicadfiles.Point, result Result, request Request, rules Rules, anchorIndex pinAnchorIndex, allowGridFallback bool) ([]kicadfiles.Point, bool) {
	routeStart, routeEnd := start, end
	if direction, ok := endpointLabelDirection(from, result.Components, rules.Grid); ok {
		routeStart = kicadfiles.Point{X: start.X + direction.X, Y: start.Y + direction.Y}
	}
	if direction, ok := endpointLabelDirection(to, result.Components, rules.Grid); ok {
		routeEnd = kicadfiles.Point{X: end.X + direction.X, Y: end.Y + direction.Y}
	}
	withAccess := func(points []kicadfiles.Point) []kicadfiles.Point {
		if routeStart != start {
			points = append([]kicadfiles.Point{start}, points...)
		}
		if routeEnd != end {
			points = append(points, end)
		}
		return compactPointPath(points)
	}
	candidates := routeCandidates(routeStart, routeEnd, result.Components, rules, anchorIndex)
	type scoredRoute struct {
		points []kicadfiles.Point
		score  int64
		clean  bool
	}
	scored := make([]scoredRoute, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = withAccess(candidate)
		score, clean := scoreRouteIndexed(candidate, netName, from, to, result, request, anchorIndex)
		scored = append(scored, scoredRoute{points: candidate, score: score, clean: clean})
	}
	hasClean := false
	for _, route := range scored {
		if route.clean {
			hasClean = true
			break
		}
	}
	if !hasClean && allowGridFallback {
		if points, ok := orthogonalGridRoute(netName, from, to, routeStart, routeEnd, result, request, rules, anchorIndex); ok {
			points = withAccess(points)
			score, clean := scoreRouteIndexed(points, netName, from, to, result, request, anchorIndex)
			scored = append(scored, scoredRoute{points: points, score: score, clean: clean})
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score < scored[j].score
		}
		return comparePointPaths(scored[i].points, scored[j].points) < 0
	})
	if len(scored) == 0 {
		return withAccess([]kicadfiles.Point{routeStart, routeEnd}), false
	}
	return scored[0].points, scored[0].clean
}

type routeGridState struct {
	node      int
	direction uint8
}

type routeGridItem struct {
	state    routeGridState
	distance int64
	priority int64
	point    kicadfiles.Point
}

type routeGridEdge struct {
	from kicadfiles.Point
	to   kicadfiles.Point
}

type routeGridHeap []routeGridItem

func (items routeGridHeap) Len() int { return len(items) }
func (items routeGridHeap) Less(i, j int) bool {
	if items[i].priority != items[j].priority {
		return items[i].priority < items[j].priority
	}
	if items[i].distance != items[j].distance {
		return items[i].distance < items[j].distance
	}
	if cmp := comparePoints(items[i].point, items[j].point); cmp != 0 {
		return cmp < 0
	}
	return items[i].state.direction < items[j].state.direction
}
func (items routeGridHeap) Swap(i, j int) { items[i], items[j] = items[j], items[i] }
func (items *routeGridHeap) Push(value any) {
	*items = append(*items, value.(routeGridItem))
}
func (items *routeGridHeap) Pop() any {
	old := *items
	last := old[len(old)-1]
	*items = old[:len(old)-1]
	return last
}

// orthogonalGridRoute is the bounded fallback for local routes that need more
// than the common one- and two-bend candidates. Candidate tracks come only
// from endpoints, obstacle clearances, and pin-access lanes. Dijkstra search
// then finds the shortest clean orthogonal path with a deterministic bend
// penalty; no circuit family or absolute coordinate is encoded.
func orthogonalGridRoute(netName string, from, to Endpoint, start, end kicadfiles.Point, result Result, request Request, rules Rules, anchorIndex pinAnchorIndex) ([]kicadfiles.Point, bool) {
	usable := UsableSheet(request.Sheet)
	xSet := map[kicadfiles.IU]struct{}{start.X: {}, end.X: {}}
	ySet := map[kicadfiles.IU]struct{}{start.Y: {}, end.Y: {}}
	addX := func(value kicadfiles.IU) {
		if value >= usable.MinX && value <= usable.MaxX {
			xSet[value] = struct{}{}
		}
	}
	addY := func(value kicadfiles.IU) {
		if value >= usable.MinY && value <= usable.MaxY {
			ySet[value] = struct{}{}
		}
	}
	clearance := rules.MinTextSpacing
	if clearance <= 0 {
		clearance = kicadfiles.MM(2.54)
	}
	for _, component := range result.Components {
		body := componentBody(component).Inflate(clearance)
		addX(SnapIU(body.MinX, rules.Grid))
		addX(SnapIU(body.MaxX, rules.Grid))
		addY(SnapIU(body.MinY, rules.Grid))
		addY(SnapIU(body.MaxY, rules.Grid))
	}
	pinLane := rules.Grid
	if pinLane <= 0 {
		pinLane = kicadfiles.MM(1.27)
	}
	for _, indexed := range anchorIndex.query(usable.MinX, usable.MaxX, usable.MinY, usable.MaxY) {
		addX(indexed.point.X - pinLane)
		addX(indexed.point.X + pinLane)
		addY(indexed.point.Y - pinLane)
		addY(indexed.point.Y + pinLane)
	}
	for _, wire := range result.Wires {
		for _, point := range []kicadfiles.Point{wire.From, wire.To} {
			addX(point.X - pinLane)
			addX(point.X + pinLane)
			addY(point.Y - pinLane)
			addY(point.Y + pinLane)
		}
	}
	xValues := make([]kicadfiles.IU, 0, len(xSet))
	for value := range xSet {
		xValues = append(xValues, value)
	}
	yValues := make([]kicadfiles.IU, 0, len(ySet))
	for value := range ySet {
		yValues = append(yValues, value)
	}
	sort.Slice(xValues, func(i, j int) bool { return xValues[i] < xValues[j] })
	sort.Slice(yValues, func(i, j int) bool { return yValues[i] < yValues[j] })
	if len(xValues) == 0 || len(yValues) == 0 || len(xValues)*len(yValues) > rules.MaxRouteGridNodes {
		return nil, false
	}
	xIndex, yIndex := map[kicadfiles.IU]int{}, map[kicadfiles.IU]int{}
	for index, value := range xValues {
		xIndex[value] = index
	}
	for index, value := range yValues {
		yIndex[value] = index
	}
	startX, startXOK := xIndex[start.X]
	startY, startYOK := yIndex[start.Y]
	endX, endXOK := xIndex[end.X]
	endY, endYOK := yIndex[end.Y]
	if !startXOK || !startYOK || !endXOK || !endYOK {
		return nil, false
	}
	nodeFor := func(x, y int) int { return x*len(yValues) + y }
	pointFor := func(node int) kicadfiles.Point {
		return kicadfiles.Point{X: xValues[node/len(yValues)], Y: yValues[node%len(yValues)]}
	}
	startState := routeGridState{node: nodeFor(startX, startY)}
	endNode := nodeFor(endX, endY)
	distances := map[routeGridState]int64{startState: 0}
	previous := map[routeGridState]routeGridState{}
	queue := &routeGridHeap{{
		state:    startState,
		point:    start,
		priority: int64(manhattan(start, end)),
	}}
	heap.Init(queue)
	var final routeGridState
	found := false
	bendPenalty := int64(rules.RouteBendPenalty)
	edgeScores := make(map[routeGridEdge]int64)
	blockedEdges := make(map[routeGridEdge]struct{})
	for queue.Len() != 0 {
		item := heap.Pop(queue).(routeGridItem)
		if current, ok := distances[item.state]; !ok || current != item.distance {
			continue
		}
		if item.state.node == endNode {
			final = item.state
			found = true
			break
		}
		x := item.state.node / len(yValues)
		y := item.state.node % len(yValues)
		type neighbor struct {
			x, y      int
			direction uint8
		}
		candidates := []neighbor{
			{x: x - 1, y: y, direction: 1},
			{x: x + 1, y: y, direction: 1},
			{x: x, y: y - 1, direction: 2},
			{x: x, y: y + 1, direction: 2},
		}
		for _, candidate := range candidates {
			if candidate.x < 0 || candidate.x >= len(xValues) || candidate.y < 0 || candidate.y >= len(yValues) {
				continue
			}
			nextNode := nodeFor(candidate.x, candidate.y)
			nextPoint := pointFor(nextNode)
			edge := routeGridEdge{from: item.point, to: nextPoint}
			if comparePoints(edge.from, edge.to) > 0 {
				edge.from, edge.to = edge.to, edge.from
			}
			if _, blocked := blockedEdges[edge]; blocked {
				continue
			}
			edgeScore, scored := edgeScores[edge]
			if !scored {
				var clean bool
				edgeScore, clean = scoreRouteIndexed([]kicadfiles.Point{edge.from, edge.to}, netName, from, to, result, request, anchorIndex)
				if !clean {
					blockedEdges[edge] = struct{}{}
					continue
				}
				edgeScores[edge] = edgeScore
			}
			distance := item.distance + edgeScore
			if item.state.direction != 0 && item.state.direction != candidate.direction {
				distance += bendPenalty
			}
			nextState := routeGridState{node: nextNode, direction: candidate.direction}
			if current, exists := distances[nextState]; exists && current <= distance {
				continue
			}
			distances[nextState] = distance
			previous[nextState] = item.state
			heap.Push(queue, routeGridItem{
				state:    nextState,
				distance: distance,
				priority: distance + int64(manhattan(nextPoint, end)),
				point:    nextPoint,
			})
		}
	}
	if !found {
		return nil, false
	}
	var reversed []kicadfiles.Point
	for state := final; ; state = previous[state] {
		reversed = append(reversed, pointFor(state.node))
		if state == startState {
			break
		}
	}
	points := make([]kicadfiles.Point, len(reversed))
	for index := range reversed {
		points[len(reversed)-1-index] = reversed[index]
	}
	return compactPointPath(points), true
}

func routeCandidates(start, end kicadfiles.Point, components []PlacedComponent, rules Rules, anchorIndex pinAnchorIndex) [][]kicadfiles.Point {
	if start == end {
		return [][]kicadfiles.Point{{start}}
	}
	var candidates [][]kicadfiles.Point
	add := func(points ...kicadfiles.Point) {
		points = compactPointPath(points)
		if pathOrthogonal(points) {
			candidates = append(candidates, points)
		}
	}
	if start.X == end.X || start.Y == end.Y {
		add(start, end)
	}
	add(start, kicadfiles.Point{X: end.X, Y: start.Y}, end)
	add(start, kicadfiles.Point{X: start.X, Y: end.Y}, end)
	midX := SnapIU(start.X+(end.X-start.X)/2, rules.Grid)
	midY := SnapIU(start.Y+(end.Y-start.Y)/2, rules.Grid)
	add(start, kicadfiles.Point{X: midX, Y: start.Y}, kicadfiles.Point{X: midX, Y: end.Y}, end)
	add(start, kicadfiles.Point{X: start.X, Y: midY}, kicadfiles.Point{X: end.X, Y: midY}, end)
	clearance := rules.MinTextSpacing
	if clearance <= 0 {
		clearance = kicadfiles.MM(2.54)
	}
	for _, component := range components {
		body := componentBody(component).Inflate(clearance)
		for _, x := range []kicadfiles.IU{body.MinX, body.MaxX} {
			x = SnapIU(x, rules.Grid)
			add(start, kicadfiles.Point{X: x, Y: start.Y}, kicadfiles.Point{X: x, Y: end.Y}, end)
		}
		for _, y := range []kicadfiles.IU{body.MinY, body.MaxY} {
			y = SnapIU(y, rules.Grid)
			add(start, kicadfiles.Point{X: start.X, Y: y}, kicadfiles.Point{X: end.X, Y: y}, end)
		}
	}
	// Pin-only templates, especially generic connectors, do not always have a
	// trustworthy body rectangle. Add deterministic offset lanes around every
	// known pin so direct routes can step around those electrical anchors.
	pinLane := rules.Grid
	if pinLane <= 0 {
		pinLane = kicadfiles.MM(1.27)
	}
	minX, maxX := orderedIU(start.X, end.X)
	minY, maxY := orderedIU(start.Y, end.Y)
	margin := clearance + pinLane
	for _, indexed := range anchorIndex.query(minX-margin, maxX+margin, minY-margin, maxY+margin) {
		anchor := indexed.point
		for _, x := range []kicadfiles.IU{anchor.X - pinLane, anchor.X + pinLane} {
			add(start, kicadfiles.Point{X: x, Y: start.Y}, kicadfiles.Point{X: x, Y: end.Y}, end)
		}
		for _, y := range []kicadfiles.IU{anchor.Y - pinLane, anchor.Y + pinLane} {
			add(start, kicadfiles.Point{X: start.X, Y: y}, kicadfiles.Point{X: end.X, Y: y}, end)
		}
	}
	return uniquePointPaths(candidates)
}

type pinAnchorCell struct {
	x int
	y int
}

type pinAnchorIndex struct {
	cellSize kicadfiles.IU
	cells    map[pinAnchorCell][]indexedPinAnchor
	all      []indexedPinAnchor
}

type indexedPinAnchor struct {
	endpoint Endpoint
	point    kicadfiles.Point
}

func newPinAnchorIndex(anchors map[Endpoint]kicadfiles.Point) pinAnchorIndex {
	all := sortedPinAnchors(anchors)
	index := pinAnchorIndex{cellSize: kicadfiles.MM(25.4), cells: map[pinAnchorCell][]indexedPinAnchor{}, all: all}
	seen := map[kicadfiles.Point]struct{}{}
	for _, indexed := range all {
		if _, exists := seen[indexed.point]; exists {
			continue
		}
		seen[indexed.point] = struct{}{}
		cell := pinAnchorCell{x: pinAnchorCellCoordinate(indexed.point.X, index.cellSize), y: pinAnchorCellCoordinate(indexed.point.Y, index.cellSize)}
		index.cells[cell] = append(index.cells[cell], indexed)
	}
	for cell := range index.cells {
		sort.Slice(index.cells[cell], func(i, j int) bool {
			left, right := index.cells[cell][i].point, index.cells[cell][j].point
			if left.X != right.X {
				return left.X < right.X
			}
			if left.Y != right.Y {
				return left.Y < right.Y
			}
			leftEndpoint, rightEndpoint := index.cells[cell][i].endpoint, index.cells[cell][j].endpoint
			if leftEndpoint.Ref != rightEndpoint.Ref {
				return leftEndpoint.Ref < rightEndpoint.Ref
			}
			return leftEndpoint.Pin < rightEndpoint.Pin
		})
	}
	return index
}

func sortedPinAnchors(anchors map[Endpoint]kicadfiles.Point) []indexedPinAnchor {
	indexed := make([]indexedPinAnchor, 0, len(anchors))
	for endpoint, point := range anchors {
		indexed = append(indexed, indexedPinAnchor{endpoint: endpoint, point: point})
	}
	sort.Slice(indexed, func(i, j int) bool {
		if indexed[i].point.X != indexed[j].point.X {
			return indexed[i].point.X < indexed[j].point.X
		}
		if indexed[i].point.Y != indexed[j].point.Y {
			return indexed[i].point.Y < indexed[j].point.Y
		}
		if indexed[i].endpoint.Ref != indexed[j].endpoint.Ref {
			return indexed[i].endpoint.Ref < indexed[j].endpoint.Ref
		}
		return indexed[i].endpoint.Pin < indexed[j].endpoint.Pin
	})
	return indexed
}

func (index pinAnchorIndex) query(minX, maxX, minY, maxY kicadfiles.IU) []indexedPinAnchor {
	if index.cellSize <= 0 || len(index.cells) == 0 {
		return nil
	}
	minCell := pinAnchorCell{x: pinAnchorCellCoordinate(minX, index.cellSize), y: pinAnchorCellCoordinate(minY, index.cellSize)}
	maxCell := pinAnchorCell{x: pinAnchorCellCoordinate(maxX, index.cellSize), y: pinAnchorCellCoordinate(maxY, index.cellSize)}
	var points []indexedPinAnchor
	for x := minCell.x; x <= maxCell.x; x++ {
		for y := minCell.y; y <= maxCell.y; y++ {
			for _, indexed := range index.cells[pinAnchorCell{x: x, y: y}] {
				if indexed.point.X >= minX && indexed.point.X <= maxX && indexed.point.Y >= minY && indexed.point.Y <= maxY {
					points = append(points, indexed)
				}
			}
		}
	}
	return points
}

func pinAnchorCellCoordinate(value, cellSize kicadfiles.IU) int {
	return int(math.Floor(float64(value) / float64(cellSize)))
}

func scoreRoute(points []kicadfiles.Point, netName string, from, to Endpoint, result Result, request Request) (int64, bool) {
	return scoreRouteIndexed(points, netName, from, to, result, request, newPinAnchorIndex(pinAnchors(result.Components)))
}

func scoreRouteIndexed(points []kicadfiles.Point, netName string, from, to Endpoint, result Result, request Request, anchorIndex pinAnchorIndex) (int64, bool) {
	if len(points) < 2 || !pathOrthogonal(points) {
		return routeHardPenalty * 4, false
	}
	usable := UsableSheet(request.Sheet)
	score := int64(len(points)-2) * int64(kicadfiles.MM(10))
	clean := true
	segments := segmentsForPoints(netName, points)
	for _, segment := range segments {
		score += int64(manhattan(segment.From, segment.To))
		if !usable.ContainsPoint(segment.From) || !usable.ContainsPoint(segment.To) {
			score += routeHardPenalty
			clean = false
		}
		for _, component := range result.Components {
			if (component.Ref == from.Ref || component.Ref == to.Ref) && component.Body.Empty() {
				continue
			}
			body := componentBody(component)
			if SegmentIntersectsRect(segment, body) && !wireLeavesAttachedSymbol(segment, ValidationObject{Ref: component.Ref, Box: body}, result.Components) {
				score += routeHardPenalty
				clean = false
			}
		}
		for _, existing := range result.Wires {
			if existing.NetName == netName {
				continue
			}
			if wireSegmentsElectricallyContact(segment, existing) {
				score += routeHardPenalty
				clean = false
			}
		}
		if wirePassesUnrelatedPinIndexed(segment, netName, anchorIndex, request) {
			score += routeHardPenalty
			clean = false
		}
	}
	return score, clean
}

func wirePassesUnrelatedPin(segment WireSegment, netName string, result Result, request Request) bool {
	_, ok := unrelatedPinForWire(segment, netName, result, request)
	return ok
}

func unrelatedPinForWire(segment WireSegment, netName string, result Result, request Request) (Endpoint, bool) {
	return unrelatedPinForWireIndexed(segment, netName, newPinAnchorIndex(pinAnchors(result.Components)), request)
}

func wirePassesUnrelatedPinIndexed(segment WireSegment, netName string, anchorIndex pinAnchorIndex, request Request) bool {
	_, ok := unrelatedPinForWireIndexed(segment, netName, anchorIndex, request)
	return ok
}

func unrelatedPinForWireIndexed(segment WireSegment, netName string, anchorIndex pinAnchorIndex, request Request) (Endpoint, bool) {
	endpoints := netEndpointSet(request, netName)
	for _, indexed := range anchorIndex.all {
		if _, allowed := endpoints[indexed.endpoint]; allowed {
			continue
		}
		if pointOnSegment(segment.From, indexed.point, segment.To) {
			return indexed.endpoint, true
		}
	}
	return Endpoint{}, false
}

func netEndpointSet(request Request, netName string) map[Endpoint]struct{} {
	endpoints := map[Endpoint]struct{}{}
	for _, net := range request.Nets {
		if net.Name != netName {
			continue
		}
		for _, endpoint := range net.Endpoints {
			endpoints[endpoint] = struct{}{}
		}
	}
	return endpoints
}

func segmentsForPoints(netName string, points []kicadfiles.Point) []WireSegment {
	segments := make([]WireSegment, 0, len(points)-1)
	for index := 1; index < len(points); index++ {
		if points[index-1] == points[index] {
			continue
		}
		segments = append(segments, WireSegment{NetName: netName, From: points[index-1], To: points[index]})
	}
	return segments
}

func compactWireSegments(wires []WireSegment) []WireSegment {
	type wireKey struct {
		net      string
		from, to kicadfiles.Point
	}
	seen := make(map[wireKey]struct{}, len(wires))
	compacted := make([]WireSegment, 0, len(wires))
	for _, wire := range wires {
		if wire.From == wire.To {
			continue
		}
		from, to := wire.From, wire.To
		if comparePoints(from, to) > 0 {
			from, to = to, from
		}
		key := wireKey{net: wire.NetName, from: from, to: to}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		wire.From, wire.To = from, to
		compacted = append(compacted, wire)
	}
	return compacted
}

func branchJunctions(wires []WireSegment) []Junction {
	candidates := map[kicadfiles.Point]struct{}{}
	type netSegments struct {
		horizontal []WireSegment
		vertical   []WireSegment
	}
	segmentsByNet := map[string]*netSegments{}
	for _, wire := range wires {
		candidates[wire.From] = struct{}{}
		candidates[wire.To] = struct{}{}
		group := segmentsByNet[wire.NetName]
		if group == nil {
			group = &netSegments{}
			segmentsByNet[wire.NetName] = group
		}
		if wire.From.Y == wire.To.Y {
			group.horizontal = append(group.horizontal, wire)
		} else if wire.From.X == wire.To.X {
			group.vertical = append(group.vertical, wire)
		}
	}
	for _, group := range segmentsByNet {
		sort.Slice(group.vertical, func(i, j int) bool {
			if group.vertical[i].From.X != group.vertical[j].From.X {
				return group.vertical[i].From.X < group.vertical[j].From.X
			}
			return comparePoints(group.vertical[i].From, group.vertical[j].From) < 0
		})
		for _, horizontal := range group.horizontal {
			minX, maxX := minIU(horizontal.From.X, horizontal.To.X), maxIU(horizontal.From.X, horizontal.To.X)
			first := sort.Search(len(group.vertical), func(index int) bool {
				return group.vertical[index].From.X >= minX
			})
			for index := first; index < len(group.vertical) && group.vertical[index].From.X <= maxX; index++ {
				if point, ok := orthogonalIntersection(horizontal, group.vertical[index]); ok {
					candidates[point] = struct{}{}
				}
			}
		}
	}
	points := make([]kicadfiles.Point, 0, len(candidates))
	for point := range candidates {
		points = append(points, point)
	}
	sort.Slice(points, func(i, j int) bool { return comparePoints(points[i], points[j]) < 0 })
	junctions := make([]Junction, 0)
	for _, point := range points {
		raysByNet := map[string]map[string]struct{}{}
		for _, wire := range wires {
			if !pointOnSegment(wire.From, point, wire.To) {
				continue
			}
			rays := raysByNet[wire.NetName]
			if rays == nil {
				rays = map[string]struct{}{}
				raysByNet[wire.NetName] = rays
			}
			if wire.From.X < point.X || wire.To.X < point.X {
				rays["left"] = struct{}{}
			}
			if wire.From.X > point.X || wire.To.X > point.X {
				rays["right"] = struct{}{}
			}
			if wire.From.Y < point.Y || wire.To.Y < point.Y {
				rays["up"] = struct{}{}
			}
			if wire.From.Y > point.Y || wire.To.Y > point.Y {
				rays["down"] = struct{}{}
			}
		}
		for _, rays := range raysByNet {
			if len(rays) >= 3 {
				junctions = append(junctions, Junction{Position: point})
				break
			}
		}
	}
	return junctions
}

func orthogonalIntersection(first, second WireSegment) (kicadfiles.Point, bool) {
	firstHorizontal := first.From.Y == first.To.Y
	secondHorizontal := second.From.Y == second.To.Y
	if firstHorizontal == secondHorizontal {
		return kicadfiles.Point{}, false
	}
	if !firstHorizontal {
		first, second = second, first
	}
	point := kicadfiles.Point{X: second.From.X, Y: first.From.Y}
	return point, pointOnSegment(first.From, point, first.To) && pointOnSegment(second.From, point, second.To)
}

func compactPointPath(points []kicadfiles.Point) []kicadfiles.Point {
	compacted := make([]kicadfiles.Point, 0, len(points))
	for _, point := range points {
		if len(compacted) != 0 && compacted[len(compacted)-1] == point {
			continue
		}
		compacted = append(compacted, point)
	}
	return compacted
}

func pathOrthogonal(points []kicadfiles.Point) bool {
	for index := 1; index < len(points); index++ {
		if points[index-1].X != points[index].X && points[index-1].Y != points[index].Y {
			return false
		}
	}
	return true
}

func uniquePointPaths(paths [][]kicadfiles.Point) [][]kicadfiles.Point {
	seen := map[string]struct{}{}
	var unique [][]kicadfiles.Point
	for _, path := range paths {
		key := pointPathKey(path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, path)
	}
	return unique
}

func pointPathKey(points []kicadfiles.Point) string {
	var key strings.Builder
	for _, point := range points {
		key.WriteString(strconv.FormatInt(int64(point.X), 10))
		key.WriteByte(':')
		key.WriteString(strconv.FormatInt(int64(point.Y), 10))
		key.WriteByte(';')
	}
	return key.String()
}

func comparePointPaths(first, second []kicadfiles.Point) int {
	limit := len(first)
	if len(second) < limit {
		limit = len(second)
	}
	for index := 0; index < limit; index++ {
		if value := comparePoints(first[index], second[index]); value != 0 {
			return value
		}
	}
	return compareInts(len(first), len(second))
}

func wireSegmentsCross(first, second WireSegment) bool {
	if !segmentsIntersect(first.From, first.To, second.From, second.To) {
		return false
	}
	for _, point := range []kicadfiles.Point{first.From, first.To} {
		if point == second.From || point == second.To {
			return false
		}
	}
	return true
}

func wireSegmentsElectricallyContact(first, second WireSegment) bool {
	return segmentsIntersect(first.From, first.To, second.From, second.To)
}

type routableEndpoint struct {
	endpoint Endpoint
	anchor   kicadfiles.Point
}

// orderedRoutableEndpoints starts at a peripheral endpoint, then visits the
// nearest remaining anchor. Routing multi-endpoint nets in caller order can
// double back over an already emitted branch, producing overlapping KiCad
// wire objects. This deterministic geometry-first traversal tends to visit a
// branch point before continuing to the opposite edge of the net.
func orderedRoutableEndpoints(net Net, anchors map[Endpoint]kicadfiles.Point) ([]routableEndpoint, []Endpoint) {
	remaining := make([]routableEndpoint, 0, len(net.Endpoints))
	missing := make([]Endpoint, 0)
	for _, endpoint := range net.Endpoints {
		anchor, ok := anchors[endpoint]
		if !ok {
			missing = append(missing, endpoint)
			continue
		}
		remaining = append(remaining, routableEndpoint{endpoint: endpoint, anchor: anchor})
	}
	sort.SliceStable(missing, func(i, j int) bool {
		if missing[i].Ref != missing[j].Ref {
			return missing[i].Ref < missing[j].Ref
		}
		return missing[i].Pin < missing[j].Pin
	})
	if len(remaining) < 2 {
		return remaining, missing
	}
	compareEndpoint := func(left, right routableEndpoint) int {
		if value := comparePoints(left.anchor, right.anchor); value != 0 {
			return value
		}
		if left.endpoint.Ref != right.endpoint.Ref {
			return strings.Compare(left.endpoint.Ref, right.endpoint.Ref)
		}
		return strings.Compare(left.endpoint.Pin, right.endpoint.Pin)
	}
	sort.SliceStable(remaining, func(i, j int) bool { return compareEndpoint(remaining[i], remaining[j]) < 0 })
	var sumX, sumY kicadfiles.IU
	for _, candidate := range remaining {
		sumX += candidate.anchor.X
		sumY += candidate.anchor.Y
	}
	startIndex := 0
	var greatestDistance kicadfiles.IU = -1
	count := kicadfiles.IU(len(remaining))
	for index, candidate := range remaining {
		// Scaling by the endpoint count avoids fractional centroid coordinates
		// while choosing a deterministic peripheral pin in linear time.
		distance := absIU(count*candidate.anchor.X-sumX) + absIU(count*candidate.anchor.Y-sumY)
		if distance > greatestDistance {
			greatestDistance = distance
			startIndex = index
		}
	}
	ordered := make([]routableEndpoint, 0, len(remaining))
	ordered = append(ordered, remaining[startIndex])
	remaining = append(remaining[:startIndex], remaining[startIndex+1:]...)
	for len(remaining) != 0 {
		current := ordered[len(ordered)-1]
		nearestIndex := 0
		nearestDistance := manhattan(current.anchor, remaining[0].anchor)
		for index := 1; index < len(remaining); index++ {
			distance := manhattan(current.anchor, remaining[index].anchor)
			if distance < nearestDistance || distance == nearestDistance && compareEndpoint(remaining[index], remaining[nearestIndex]) < 0 {
				nearestIndex = index
				nearestDistance = distance
			}
		}
		ordered = append(ordered, remaining[nearestIndex])
		remaining = append(remaining[:nearestIndex], remaining[nearestIndex+1:]...)
	}
	return ordered, missing
}

func Layout(request Request) Result {
	request = NormalizeRequest(request)
	candidates := pageCandidates(request.Sheet)
	if len(candidates) == 0 {
		candidates = []Sheet{request.Sheet}
	}
	var last Result
	var lastRequest Request
	var selected Result
	var selectedRequest Request
	selectedFound := false
	for index, sheet := range candidates {
		if selectedFound && sheet.Name != selected.Sheet.Name {
			break
		}
		candidateRequest := request
		candidateRequest.Sheet = sheet
		candidate := Place(candidateRequest)
		if !hasPageOverflow(candidate) {
			candidate = Route(candidateRequest, candidate)
		}
		candidate.Sheet = sheet
		candidate.Report.SelectedPaper = sheet.Name
		candidate.Report.PageEscalationCount = index
		if index > 0 {
			candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{
				Severity: SeverityInfo,
				Code:     "page_escalated",
				Message:  "paper size was escalated to contain the readable drawing",
				Repair:   "retain the selected paper or provide explicit sheet constraints",
			})
		}
		last = candidate
		lastRequest = candidateRequest
		if !hasPageOverflow(candidate) {
			if !selectedFound || layoutAspectMismatch(candidate) < layoutAspectMismatch(selected) {
				selected = candidate
				selectedRequest = candidateRequest
				selectedFound = true
			}
		}
	}
	if selectedFound {
		selected = finalizeLayoutCandidate(selected, selectedRequest)
		if request.MaxComponentsPerSheet > 0 && len(request.Components) > request.MaxComponentsPerSheet {
			partition := PartitionPlaced(request, selected.Components)
			if len(partition.Sheets) > 1 {
				selected.Partition = &partition
				selected.Report.PartitionCount = len(partition.Sheets)
				selected.Report.PartitionSplitGroupCount = len(partition.SplitGroups)
				selected.Report.CrossSheetNetCount = len(partition.CrossSheetNets)
				selected.Diagnostics = append(selected.Diagnostics, Diagnostic{
					Severity: SeverityInfo,
					Code:     "hierarchy_partition_requested",
					Message:  "the graph was partitioned to satisfy the requested sheet component limit",
					Repair:   "emit KiCad hierarchical sheets and cross-sheet labels",
				})
			}
		}
		return NormalizeResult(selected, selectedRequest.Rules)
	}
	last = finalizeLayoutCandidate(last, lastRequest)
	last.Diagnostics = append(last.Diagnostics, Diagnostic{
		Severity: SeverityError,
		Code:     "page_fit_exhausted",
		Message:  "the drawing does not fit on the largest supported standard paper",
		Repair:   "partition the design into hierarchical sheets or provide a larger custom sheet",
	})
	partition := PartitionPlaced(request, last.Components)
	last.Partition = &partition
	last.Report.PartitionCount = len(partition.Sheets)
	last.Report.PartitionSplitGroupCount = len(partition.SplitGroups)
	last.Report.CrossSheetNetCount = len(partition.CrossSheetNets)
	if len(partition.Sheets) > 1 {
		last.Diagnostics = append(last.Diagnostics, Diagnostic{
			Severity: SeverityInfo,
			Code:     "hierarchy_partition_required",
			Message:  "the graph was partitioned into deterministic sheet regions",
			Repair:   "emit KiCad hierarchical sheets and cross-sheet labels",
		})
	}
	for _, group := range partition.SplitGroups {
		last.Diagnostics = append(last.Diagnostics, Diagnostic{
			Severity: SeverityInfo,
			Code:     "hierarchy_group_split",
			Ref:      group,
			Message:  "an oversized layout group was split across hierarchy sheets to preserve readable child pages",
			Repair:   "add smaller layout groups if this functional region should remain on one sheet",
		})
	}
	return NormalizeResult(last, request.Rules)
}

func layoutAspectMismatch(result Result) float64 {
	usable := UsableSheet(result.Sheet)
	occupied := result.Report.OccupiedBounds
	if usable.Empty() || occupied.Empty() || usable.Height() == 0 || occupied.Height() == 0 {
		return math.MaxFloat64
	}
	usableAspect := float64(usable.Width()) / float64(usable.Height())
	occupiedAspect := float64(occupied.Width()) / float64(occupied.Height())
	if usableAspect <= 0 || occupiedAspect <= 0 {
		return math.MaxFloat64
	}
	return math.Abs(math.Log(usableAspect / occupiedAspect))
}

func finalizeLayoutCandidate(candidate Result, request Request) Result {
	var textDiagnostics []Diagnostic
	candidate.Components, textDiagnostics = reflowTextForWires(candidate.Components, candidate.Wires, candidate.Labels, request.Rules, UsableSheet(request.Sheet))
	candidate.Diagnostics = filterTextDiagnostics(candidate.Diagnostics)
	candidate.Diagnostics = append(candidate.Diagnostics, textDiagnostics...)
	return Validate(candidate, request)
}

func filterTextDiagnostics(diagnostics []Diagnostic) []Diagnostic {
	filtered := make([]Diagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		switch diagnostic.Code {
		case "text_placement_fallback", "text_symbol_overlap", "text_wire_overlap":
			continue
		default:
			filtered = append(filtered, diagnostic)
		}
	}
	return filtered
}

func pinAnchors(components []PlacedComponent) map[Endpoint]kicadfiles.Point {
	anchors := map[Endpoint]kicadfiles.Point{}
	for _, component := range components {
		if len(component.Pins) == 0 {
			anchors[Endpoint{Ref: component.Ref, Pin: "1"}] = component.PlacedAt
			continue
		}
		for _, pin := range component.Pins {
			anchors[Endpoint{Ref: component.Ref, Pin: pin.Number}] = schematic.CanonicalConnectionAnchor(
				component.PlacedAt, pin.At, component.Rotation, schematic.SymbolMirror(component.Mirror),
			)
		}
	}
	return anchors
}

func shouldUseLabels(net Net, anchors map[Endpoint]kicadfiles.Point, components []Component, rules Rules) bool {
	if !rules.LabelFallbackEnabled || len(net.Endpoints) < 2 {
		return false
	}
	if net.PreferDirect {
		return false
	}
	role := normalizeRole(net.Role)
	if net.EndpointLabels || containsNormalizedRole(role, "cross_sheet", "global", "bus") {
		return true
	}
	// Very high fanout is the remaining local case where endpoint labels are
	// clearer than a page-spanning tree. Ordinary three-to-seven endpoint
	// power, feedback, and signal nets remain visibly wired.
	if len(net.Endpoints) >= 8 {
		return true
	}
	return false
}

func manhattan(first, second kicadfiles.Point) kicadfiles.IU {
	dx := first.X - second.X
	if dx < 0 {
		dx = -dx
	}
	dy := first.Y - second.Y
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}
