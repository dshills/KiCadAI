package schematiclayout

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
)

const routeHardPenalty int64 = 1_000_000_000_000

func Route(request Request, result Result) Result {
	request = Classify(request)
	rules := normalizeRules(request.Rules)
	anchors := pinAnchors(result.Components)
	anchorIndex := newPinAnchorIndex(anchors)
	labeled := map[string]kicadfiles.Point{}
	for _, net := range request.Nets {
		if len(net.Endpoints) == 0 {
			continue
		}
		startIndex, start, ok := firstRoutableEndpoint(net, anchors)
		if !ok {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityWarning, Code: "missing_pin_anchor", NetName: net.Name, Message: "net has no routable endpoint anchors"})
			continue
		}
		fromEndpoint := net.Endpoints[startIndex]
		if len(net.Endpoints) == 1 {
			if net.PreferredLabels {
				appendEndpointLabel(&result, labeled, net.Name, fromEndpoint, start, request, rules)
			}
			continue
		}
		forceLabels := shouldUseLabels(net, anchors, request.Components, rules)
		for _, toEndpoint := range net.Endpoints[startIndex+1:] {
			end, exists := anchors[toEndpoint]
			if !exists {
				result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityWarning, Code: "missing_pin_anchor", NetName: net.Name, Ref: toEndpoint.Ref, Message: "net endpoint has no pin anchor"})
				continue
			}
			if forceLabels {
				fromLabel := appendEndpointLabel(&result, labeled, net.Name, fromEndpoint, start, request, rules)
				toLabel := appendEndpointLabel(&result, labeled, net.Name, toEndpoint, end, request, rules)
				result.Connections = append(result.Connections, RoutedConnection{NetName: net.Name, From: fromEndpoint, To: toEndpoint, UseLabels: true, FromLabelAt: &fromLabel, ToLabelAt: &toLabel})
			} else {
				points, clean := routeConnectionPoints(net.Name, fromEndpoint, toEndpoint, start, end, result, request, rules, anchorIndex)
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
	}
	result = Validate(result, request)
	return NormalizeResult(result, rules)
}

func appendEndpointLabel(result *Result, seen map[string]kicadfiles.Point, netName string, endpoint Endpoint, anchor kicadfiles.Point, request Request, rules Rules) kicadfiles.Point {
	key := netName + "\x00" + endpoint.Ref + "\x00" + endpoint.Pin
	if position, ok := seen[key]; ok {
		return position
	}
	position, clean := labelStubPoint(netName, endpoint, anchor, *result, request, rules)
	seen[key] = position
	result.Labels = append(result.Labels, Label{NetName: netName, Text: netName, Position: position})
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
	pinDirected := false
	if direction, ok := endpointLabelDirection(endpoint, result.Components, grid); ok {
		preferred = direction
		pinDirected = true
	}
	for _, component := range result.Components {
		if component.Ref != endpoint.Ref {
			continue
		}
		if pinDirected {
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
	if !pinDirected {
		directions = append(directions, kicadfiles.Point{X: grid}, kicadfiles.Point{X: -grid}, kicadfiles.Point{Y: -grid}, kicadfiles.Point{Y: grid})
	}
	for index := 1; index < len(directions); index++ {
		for _, component := range result.Components {
			if component.Ref == endpoint.Ref {
				directions[index] = TransformPoint(directions[index], component.Rotation, component.Mirror)
				break
			}
		}
	}
	usable := UsableSheet(request.Sheet)
	for _, scale := range []kicadfiles.IU{2, 4, 6, 8, 12} {
		for _, direction := range directions {
			position := kicadfiles.Point{X: anchor.X + direction.X*scale, Y: anchor.Y + direction.Y*scale}
			segment := WireSegment{NetName: netName, From: anchor, To: position}
			labelBox := TextEstimate(netName, position, 0, 0)
			if !usable.ContainsRect(labelBox) || labelPlacementCollides(labelBox, segment, endpoint, result, request) {
				continue
			}
			return position, true
		}
	}
	return kicadfiles.Point{X: anchor.X + preferred.X*2, Y: anchor.Y + preferred.Y*2}, false
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
		if labelBox.Intersects(TextEstimate(label.Text, label.Position, 0, 0)) {
			return true
		}
	}
	for _, wire := range result.Wires {
		if wire.NetName != stub.NetName && wireSegmentsCross(stub, wire) {
			return true
		}
	}
	return false
}

func routeConnectionPoints(netName string, from, to Endpoint, start, end kicadfiles.Point, result Result, request Request, rules Rules, anchorIndex pinAnchorIndex) ([]kicadfiles.Point, bool) {
	candidates := routeCandidates(start, end, result.Components, rules, anchorIndex)
	type scoredRoute struct {
		points []kicadfiles.Point
		score  int64
		clean  bool
	}
	scored := make([]scoredRoute, 0, len(candidates))
	for _, candidate := range candidates {
		score, clean := scoreRoute(candidate, netName, from, to, result, request)
		scored = append(scored, scoredRoute{points: candidate, score: score, clean: clean})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score < scored[j].score
		}
		return comparePointPaths(scored[i].points, scored[j].points) < 0
	})
	if len(scored) == 0 {
		return []kicadfiles.Point{start, end}, false
	}
	return scored[0].points, scored[0].clean
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
}

type indexedPinAnchor struct {
	endpoint Endpoint
	point    kicadfiles.Point
}

func newPinAnchorIndex(anchors map[Endpoint]kicadfiles.Point) pinAnchorIndex {
	index := pinAnchorIndex{cellSize: kicadfiles.MM(25.4), cells: map[pinAnchorCell][]indexedPinAnchor{}}
	seen := map[kicadfiles.Point]struct{}{}
	for endpoint, anchor := range anchors {
		if _, exists := seen[anchor]; exists {
			continue
		}
		seen[anchor] = struct{}{}
		cell := pinAnchorCell{x: pinAnchorCellCoordinate(anchor.X, index.cellSize), y: pinAnchorCellCoordinate(anchor.Y, index.cellSize)}
		index.cells[cell] = append(index.cells[cell], indexedPinAnchor{endpoint: endpoint, point: anchor})
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
			if wireSegmentsCross(segment, existing) {
				score += routeHardPenalty
				clean = false
			}
		}
		if wirePassesUnrelatedPin(segment, netName, result, request) {
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
	endpoints := netEndpointSet(request, netName)
	anchors := pinAnchors(result.Components)
	for endpoint, anchor := range anchors {
		if _, allowed := endpoints[endpoint]; allowed {
			continue
		}
		if pointOnSegment(segment.From, anchor, segment.To) {
			return endpoint, true
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
		break
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

func firstRoutableEndpoint(net Net, anchors map[Endpoint]kicadfiles.Point) (int, kicadfiles.Point, bool) {
	for index, endpoint := range net.Endpoints {
		if anchor, ok := anchors[endpoint]; ok {
			return index, anchor, true
		}
	}
	return -1, kicadfiles.Point{}, false
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
			selected = candidate
			selectedRequest = candidateRequest
			selectedFound = true
			break
		}
	}
	if selectedFound {
		selected = finalizeLayoutCandidate(selected, selectedRequest)
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

func finalizeLayoutCandidate(candidate Result, request Request) Result {
	var textDiagnostics []Diagnostic
	candidate.Components, textDiagnostics = reflowTextForWires(candidate.Components, candidate.Wires, candidate.Labels, request.Rules)
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
			offset := TransformPoint(pin.At, component.Rotation, component.Mirror)
			anchors[Endpoint{Ref: component.Ref, Pin: pin.Number}] = kicadfiles.Point{
				X: component.PlacedAt.X + offset.X,
				Y: component.PlacedAt.Y + offset.Y,
			}
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
	if net.PreferredLabels || len(net.Endpoints) > 2 || containsNormalizedRole(role, "power", "ground", "bus", "negative_rail") {
		return true
	}
	groupByRef := map[string]string{}
	for _, component := range components {
		groupByRef[component.Ref] = component.GroupID
	}
	groups := map[string]struct{}{}
	for _, endpoint := range net.Endpoints {
		if groupID := groupByRef[endpoint.Ref]; groupID != "" {
			groups[groupID] = struct{}{}
		}
	}
	if len(groups) > 1 {
		return true
	}
	startIndex, start, ok := firstRoutableEndpoint(net, anchors)
	if !ok {
		return false
	}
	for _, endpoint := range net.Endpoints[startIndex+1:] {
		end, ok := anchors[endpoint]
		if !ok {
			continue
		}
		if manhattan(start, end) > rules.LongWireThreshold {
			return true
		}
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
