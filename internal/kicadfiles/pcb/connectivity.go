package pcb

import (
	"fmt"
	"math"
	"strings"

	"kicadai/internal/kicadfiles"
)

const (
	connectivityToleranceIU = kicadfiles.IU(100)
	// A 2 mm cell is comfortably larger than the 0.0001 mm numerical contact
	// tolerance while remaining below common PCB feature spacing. Correctness is
	// independent of this tuning value: large pads occupy all intersected cells
	// and tracks/arcs enumerate every traversed cell.
	connectivityCellSizeIU = kicadfiles.IU(2_000_000)
)

type connectivityAnchor struct {
	id               int
	itemKey          string
	electricalPadKey string
	field            string
	kind             string
	netCode          int
	point            kicadfiles.Point
	layers           []kicadfiles.BoardLayer
	allCopper        bool
	radius           kicadfiles.IU
	padShape         string
	padSize          kicadfiles.Point
	padRotation      kicadfiles.Angle
	routeEndpoint    bool
	hasSegment       bool
	segmentStart     kicadfiles.Point
	segmentEnd       kicadfiles.Point
	segmentWidth     kicadfiles.IU
	hasArc           bool
	arcStart         kicadfiles.Point
	arcMid           kicadfiles.Point
	arcEnd           kicadfiles.Point
	arcWidth         kicadfiles.IU
}

// ValidateGeneratedConnectivity checks that generated board routing is
// electrically meaningful. It assumes Validate has already passed, then checks
// same-net pad and route endpoint connectivity using deterministic geometry.
func ValidateGeneratedConnectivity(board PCBFile) error {
	if err := Validate(board); err != nil {
		return err
	}

	anchors := collectConnectivityAnchors(board)
	if len(anchors) == 0 {
		return nil
	}

	spatialIndex := buildConnectivitySpatialIndex(anchors)
	segmentIndex := buildConnectivitySegmentIndex(anchors)
	uf := newConnectivityUnion(len(anchors))
	unionEquivalentFootprintPads(anchors, uf)
	unionRoutedCopperEndpoints(anchors, uf)
	seenIndexes := map[int]struct{}{}
	for i := range anchors {
		visitNearbyAnchorIndexes(i, anchors, spatialIndex, seenIndexes, func(j int) bool {
			if j <= i {
				return true
			}
			if anchorsTouch(anchors[i], anchors[j]) {
				uf.union(i, j)
			}
			return true
		})
	}
	unionAnchorsToSegments(anchors, segmentIndex, uf)

	var errs kicadfiles.ValidationErrors
	errs = append(errs, validatePadConnectivity(board, anchors, uf)...)
	errs = append(errs, validateRouteEndpointConnectivity(anchors, spatialIndex, segmentIndex)...)
	return errs.Err()
}

func collectConnectivityAnchors(board PCBFile) []connectivityAnchor {
	anchors := []connectivityAnchor{}
	boardCopperLayers := connectivityBoardCopperLayers(board.Layers)
	for footprintIndex, footprint := range board.Footprints {
		for padIndex, pad := range footprint.Pads {
			if pad.NetCode == 0 || padType(pad) == "np_thru_hole" {
				continue
			}
			layers, allCopper := copperLayersForConnectivity(pad.Layers)
			if len(layers) == 0 && !allCopper {
				continue
			}
			field := indexed(indexedValue("footprints", footprintIndex)+".pads", padIndex, "connectivity")
			anchors = append(anchors, connectivityAnchor{
				id:               len(anchors),
				itemKey:          "pad:" + string(pad.UUID),
				electricalPadKey: fmt.Sprintf("%s:%s:%d", footprint.UUID, pad.Name, pad.NetCode),
				field:            field,
				kind:             "pad",
				netCode:          pad.NetCode,
				point:            absolutePadPosition(footprint, pad),
				layers:           layers,
				allCopper:        allCopper,
				padShape:         pad.Shape,
				padSize:          pad.Size,
				// KiCad applies the footprint rotation to the pad's local rotation.
				padRotation: footprint.Rotation + pad.Rotation,
			})
		}
	}
	for index, track := range board.Tracks {
		if track.NetCode == 0 {
			continue
		}
		itemKey := "track:" + string(track.UUID)
		start := connectivityAnchor{id: len(anchors), itemKey: itemKey, field: indexed("tracks", index, "start"), kind: "track", netCode: track.NetCode, point: track.Start, layers: []kicadfiles.BoardLayer{track.Layer}, radius: track.Width / 2, routeEndpoint: true, hasSegment: true, segmentStart: track.Start, segmentEnd: track.End, segmentWidth: track.Width}
		anchors = append(anchors, start)
		end := connectivityAnchor{id: len(anchors), itemKey: itemKey, field: indexed("tracks", index, "end"), kind: "track", netCode: track.NetCode, point: track.End, layers: []kicadfiles.BoardLayer{track.Layer}, radius: track.Width / 2, routeEndpoint: true}
		anchors = append(anchors, end)
	}
	for index, arc := range board.TrackArcs {
		if arc.NetCode == 0 {
			continue
		}
		itemKey := "track_arc:" + string(arc.UUID)
		start := connectivityAnchor{id: len(anchors), itemKey: itemKey, field: indexed("track_arcs", index, "start"), kind: "track_arc", netCode: arc.NetCode, point: arc.Start, layers: []kicadfiles.BoardLayer{arc.Layer}, radius: arc.Width / 2, routeEndpoint: true, hasArc: true, arcStart: arc.Start, arcMid: arc.Mid, arcEnd: arc.End, arcWidth: arc.Width}
		anchors = append(anchors, start)
		end := connectivityAnchor{id: len(anchors), itemKey: itemKey, field: indexed("track_arcs", index, "end"), kind: "track_arc", netCode: arc.NetCode, point: arc.End, layers: []kicadfiles.BoardLayer{arc.Layer}, radius: arc.Width / 2, routeEndpoint: true}
		anchors = append(anchors, end)
	}
	for index, via := range board.Vias {
		if via.NetCode == 0 {
			continue
		}
		layers, _ := copperLayersForConnectivity(via.Layers)
		layers = expandConnectivityLayerSpan(via.Layers, layers, boardCopperLayers)
		anchors = append(anchors, connectivityAnchor{
			id:        len(anchors),
			itemKey:   "via:" + string(via.UUID),
			field:     indexed("vias", index, "connectivity"),
			kind:      "via",
			netCode:   via.NetCode,
			point:     via.Position,
			layers:    layers,
			allCopper: false,
			radius:    via.Size / 2,
		})
	}
	return anchors
}

// unionEquivalentFootprintPads models KiCad's duplicate pad-name convention:
// physically separate pads with the same footprint, pad number, and net are
// one electrical package pin (for example an SOT-223 output tab and pin 2).
func unionEquivalentFootprintPads(anchors []connectivityAnchor, uf connectivityUnion) {
	firstByKey := map[string]int{}
	for index, anchor := range anchors {
		if anchor.kind != "pad" || anchor.electricalPadKey == "" {
			continue
		}
		if first, ok := firstByKey[anchor.electricalPadKey]; ok {
			uf.union(first, index)
			continue
		}
		firstByKey[anchor.electricalPadKey] = index
	}
}

func unionRoutedCopperEndpoints(anchors []connectivityAnchor, uf connectivityUnion) {
	anchorsByItem := map[string][]int{}
	for index, anchor := range anchors {
		if !anchor.routeEndpoint || anchor.itemKey == "" {
			continue
		}
		anchorsByItem[anchor.itemKey] = append(anchorsByItem[anchor.itemKey], index)
	}
	for _, indexes := range anchorsByItem {
		for i := 1; i < len(indexes); i++ {
			uf.union(indexes[0], indexes[i])
		}
	}
}

func validatePadConnectivity(board PCBFile, anchors []connectivityAnchor, uf connectivityUnion) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	netNames := netNameMap(board.Nets)
	firstPadRoot := map[int]int{}
	firstPadField := map[int]string{}
	for _, anchor := range anchors {
		if anchor.kind != "pad" {
			continue
		}
		root := uf.find(anchor.id)
		if firstRoot, ok := firstPadRoot[anchor.netCode]; ok && root != firstRoot {
			name := netDisplayName(anchor.netCode, netNames)
			errs = append(errs, fieldError(anchor.field, fmt.Sprintf("pad is disconnected from net %q; first pad at %s", name, firstPadField[anchor.netCode])))
			continue
		}
		firstPadRoot[anchor.netCode] = root
		firstPadField[anchor.netCode] = anchor.field
	}
	return errs
}

func validateRouteEndpointConnectivity(anchors []connectivityAnchor, spatialIndex connectivitySpatialIndex, segmentIndex connectivitySegmentIndex) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	seenIndexes := map[int]struct{}{}
	for _, anchor := range anchors {
		if !anchor.routeEndpoint {
			continue
		}
		connected := false
		visitNearbyAnchorIndexes(anchor.id, anchors, spatialIndex, seenIndexes, func(candidateIndex int) bool {
			candidate := anchors[candidateIndex]
			if anchor.id == candidate.id || anchor.itemKey == candidate.itemKey {
				return true
			}
			if anchorsTouch(anchor, candidate) {
				connected = true
				return false
			}
			return true
		})
		if !connected {
			visitNearbySegmentIndexes(anchor.point, anchor.netCode, segmentIndex, seenIndexes, func(candidateIndex int) bool {
				candidate := anchors[candidateIndex]
				if anchor.itemKey == candidate.itemKey {
					return true
				}
				if anchorTouchesSegment(anchor, candidate) {
					connected = true
					return false
				}
				return true
			})
		}
		if !connected {
			errs = append(errs, fieldError(anchor.field, "route endpoint is not connected to a same-net pad, via, or route endpoint"))
		}
	}
	return errs
}

type connectivityCell struct {
	x int64
	y int64
}

type connectivitySpatialIndex map[int]map[connectivityCell][]int
type connectivitySegmentIndex map[int]map[connectivityCell][]int

func buildConnectivitySpatialIndex(anchors []connectivityAnchor) connectivitySpatialIndex {
	index := connectivitySpatialIndex{}
	for anchorIndex, anchor := range anchors {
		if index[anchor.netCode] == nil {
			index[anchor.netCode] = map[connectivityCell][]int{}
		}
		for _, cell := range cellsForBounds(anchorConnectivityBounds(anchor)) {
			index[anchor.netCode][cell] = append(index[anchor.netCode][cell], anchorIndex)
		}
	}
	return index
}

func visitNearbyAnchorIndexes(anchorIndex int, anchors []connectivityAnchor, spatialIndex connectivitySpatialIndex, seen map[int]struct{}, visit func(int) bool) {
	anchor := anchors[anchorIndex]
	cell := connectivityPointCell(anchor.point)
	netCells := spatialIndex[anchor.netCode]
	if len(netCells) == 0 {
		return
	}
	clearIntSet(seen)
	for dx := int64(-1); dx <= 1; dx++ {
		for dy := int64(-1); dy <= 1; dy++ {
			for _, index := range netCells[connectivityCell{x: cell.x + dx, y: cell.y + dy}] {
				if _, ok := seen[index]; ok {
					continue
				}
				seen[index] = struct{}{}
				if !visit(index) {
					return
				}
			}
		}
	}
}

func connectivityPointCell(point kicadfiles.Point) connectivityCell {
	return connectivityCell{
		x: floorDivIU(point.X, connectivityCellSizeIU),
		y: floorDivIU(point.Y, connectivityCellSizeIU),
	}
}

type connectivityBounds struct {
	minX kicadfiles.IU
	maxX kicadfiles.IU
	minY kicadfiles.IU
	maxY kicadfiles.IU
}

func anchorConnectivityBounds(anchor connectivityAnchor) connectivityBounds {
	if anchor.kind == "pad" {
		corners := padBoundingCorners(anchor)
		bounds := connectivityBounds{minX: corners[0].X, maxX: corners[0].X, minY: corners[0].Y, maxY: corners[0].Y}
		for _, corner := range corners[1:] {
			bounds.minX = minIU(bounds.minX, corner.X)
			bounds.maxX = maxIU(bounds.maxX, corner.X)
			bounds.minY = minIU(bounds.minY, corner.Y)
			bounds.maxY = maxIU(bounds.maxY, corner.Y)
		}
		return bounds
	}
	radius := anchor.radius + connectivityToleranceIU
	if radius < connectivityToleranceIU {
		radius = connectivityToleranceIU
	}
	return connectivityBounds{
		minX: anchor.point.X - radius,
		maxX: anchor.point.X + radius,
		minY: anchor.point.Y - radius,
		maxY: anchor.point.Y + radius,
	}
}

func cellsForBounds(bounds connectivityBounds) []connectivityCell {
	startCell := connectivityPointCell(kicadfiles.Point{X: bounds.minX, Y: bounds.minY})
	endCell := connectivityPointCell(kicadfiles.Point{X: bounds.maxX, Y: bounds.maxY})
	cells := []connectivityCell{}
	for x := startCell.x; x <= endCell.x; x++ {
		for y := startCell.y; y <= endCell.y; y++ {
			cells = append(cells, connectivityCell{x: x, y: y})
		}
	}
	return cells
}

func buildConnectivitySegmentIndex(anchors []connectivityAnchor) connectivitySegmentIndex {
	index := connectivitySegmentIndex{}
	for anchorIndex, anchor := range anchors {
		if !anchor.hasSegment && !anchor.hasArc {
			continue
		}
		if index[anchor.netCode] == nil {
			index[anchor.netCode] = map[connectivityCell][]int{}
		}
		cells := []connectivityCell{}
		if anchor.hasArc {
			cells = arcCells(anchor.arcStart, anchor.arcMid, anchor.arcEnd)
		} else {
			cells = segmentCells(anchor.segmentStart, anchor.segmentEnd)
		}
		for _, cell := range cells {
			index[anchor.netCode][cell] = append(index[anchor.netCode][cell], anchorIndex)
		}
	}
	return index
}

func segmentCells(start, end kicadfiles.Point) []connectivityCell {
	startCell := connectivityPointCell(start)
	endCell := connectivityPointCell(end)
	cells := []connectivityCell{}
	seen := map[connectivityCell]struct{}{}
	addCell := func(cell connectivityCell) {
		if _, ok := seen[cell]; ok {
			return
		}
		seen[cell] = struct{}{}
		cells = append(cells, cell)
	}
	cell := startCell
	addCell(cell)
	if startCell == endCell {
		return cells
	}

	x0 := float64(start.X) / float64(connectivityCellSizeIU)
	y0 := float64(start.Y) / float64(connectivityCellSizeIU)
	x1 := float64(end.X) / float64(connectivityCellSizeIU)
	y1 := float64(end.Y) / float64(connectivityCellSizeIU)
	dx := x1 - x0
	dy := y1 - y0

	stepX, tMaxX, tDeltaX := ddaAxis(x0, dx, cell.x)
	stepY, tMaxY, tDeltaY := ddaAxis(y0, dy, cell.y)
	for cell != endCell {
		if math.Min(tMaxX, tMaxY) >= 1 {
			addCell(endCell)
			break
		}
		switch {
		case tMaxX < tMaxY:
			cell.x += stepX
			tMaxX += tDeltaX
			addCell(cell)
		case tMaxY < tMaxX:
			cell.y += stepY
			tMaxY += tDeltaY
			addCell(cell)
		default:
			xStepCell := connectivityCell{x: cell.x + stepX, y: cell.y}
			yStepCell := connectivityCell{x: cell.x, y: cell.y + stepY}
			cell.x += stepX
			cell.y += stepY
			tMaxX += tDeltaX
			tMaxY += tDeltaY
			addCell(xStepCell)
			addCell(yStepCell)
			addCell(cell)
		}
	}
	return cells
}

func ddaAxis(start, delta float64, cell int64) (step int64, tMax, tDelta float64) {
	if delta > 0 {
		return 1, (float64(cell+1) - start) / delta, 1 / delta
	}
	if delta < 0 {
		return -1, (float64(cell) - start) / delta, -1 / delta
	}
	return 0, math.Inf(1), math.Inf(1)
}

func visitNearbySegmentIndexes(point kicadfiles.Point, netCode int, segmentIndex connectivitySegmentIndex, seen map[int]struct{}, visit func(int) bool) {
	cell := connectivityPointCell(point)
	netCells := segmentIndex[netCode]
	if len(netCells) == 0 {
		return
	}
	clearIntSet(seen)
	for dx := int64(-1); dx <= 1; dx++ {
		for dy := int64(-1); dy <= 1; dy++ {
			for _, index := range netCells[connectivityCell{x: cell.x + dx, y: cell.y + dy}] {
				if _, ok := seen[index]; ok {
					continue
				}
				seen[index] = struct{}{}
				if !visit(index) {
					return
				}
			}
		}
	}
}

func unionAnchorsToSegments(anchors []connectivityAnchor, segmentIndex connectivitySegmentIndex, uf connectivityUnion) {
	seenIndexes := map[int]struct{}{}
	for _, anchor := range anchors {
		visitNearbySegmentIndexes(anchor.point, anchor.netCode, segmentIndex, seenIndexes, func(candidateIndex int) bool {
			candidate := anchors[candidateIndex]
			if anchor.itemKey == candidate.itemKey {
				return true
			}
			if anchorTouchesSegment(anchor, candidate) {
				uf.union(anchor.id, candidate.id)
			}
			return true
		})
	}
}

func clearIntSet(values map[int]struct{}) {
	for value := range values {
		delete(values, value)
	}
}

func anchorsTouch(a, b connectivityAnchor) bool {
	if !anchorsShareCopperLayer(a, b) {
		return false
	}
	if pointsWithinTolerance(a.point, b.point, connectivityToleranceIU) {
		return true
	}
	if a.kind == "pad" && b.kind == "pad" && padsOverlap(a, b) {
		return true
	}
	if a.kind == "pad" && b.radius > 0 && circleTouchesPad(b.point, b.radius, a) {
		return true
	}
	if b.kind == "pad" && a.radius > 0 && circleTouchesPad(a.point, a.radius, b) {
		return true
	}
	if a.kind == "pad" && pointTouchesPad(b.point, a) {
		return true
	}
	if b.kind == "pad" && pointTouchesPad(a.point, b) {
		return true
	}
	if a.radius > 0 && b.radius > 0 && pointWithinRadius(a.point, b.point, a.radius+b.radius+connectivityToleranceIU) {
		return true
	}
	return false
}

func pointsWithinTolerance(a, b kicadfiles.Point, tolerance kicadfiles.IU) bool {
	xDelta := absIU(a.X - b.X)
	yDelta := absIU(a.Y - b.Y)
	if xDelta > tolerance || yDelta > tolerance {
		return false
	}
	return math.Hypot(float64(xDelta), float64(yDelta)) <= float64(tolerance)
}

func pointWithinRadius(point, center kicadfiles.Point, radius kicadfiles.IU) bool {
	xDelta := absIU(point.X - center.X)
	yDelta := absIU(point.Y - center.Y)
	if xDelta > radius || yDelta > radius {
		return false
	}
	return math.Hypot(float64(xDelta), float64(yDelta)) <= float64(radius)
}

func anchorTouchesSegment(anchor, segment connectivityAnchor) bool {
	if !anchorsShareCopperLayer(anchor, segment) {
		return false
	}
	if segment.hasArc {
		if anchor.kind == "pad" {
			return arcTouchesPad(segment.arcStart, segment.arcMid, segment.arcEnd, segment.arcWidth/2, anchor)
		}
		return pointOnArc(anchor.point, segment.arcStart, segment.arcMid, segment.arcEnd, segment.arcWidth/2+anchor.radius+connectivityToleranceIU)
	}
	if anchor.kind == "pad" {
		return pointTouchesPad(segment.segmentStart, anchor) || pointOnSegmentThroughPad(segment.segmentStart, segment.segmentEnd, segment.segmentWidth/2, anchor)
	}
	return pointOnSegment(anchor.point, segment.segmentStart, segment.segmentEnd, segment.segmentWidth/2+anchor.radius+connectivityToleranceIU)
}

func pointOnSegment(point, start, end kicadfiles.Point, tolerance kicadfiles.IU) bool {
	minX := minIU(start.X, end.X) - tolerance
	maxX := maxIU(start.X, end.X) + tolerance
	minY := minIU(start.Y, end.Y) - tolerance
	maxY := maxIU(start.Y, end.Y) + tolerance
	if point.X < minX || point.X > maxX || point.Y < minY || point.Y > maxY {
		return false
	}
	dx := int64(end.X - start.X)
	dy := int64(end.Y - start.Y)
	px := int64(point.X - start.X)
	py := int64(point.Y - start.Y)
	cross := math.Abs(float64(dx)*float64(py) - float64(dy)*float64(px))
	length := math.Hypot(float64(dx), float64(dy))
	if length == 0 {
		return false
	}
	return cross/length <= float64(tolerance)
}

func anchorsShareCopperLayer(a, b connectivityAnchor) bool {
	if a.allCopper {
		return b.allCopper || len(b.layers) > 0
	}
	if b.allCopper {
		return len(a.layers) > 0
	}
	for _, left := range a.layers {
		for _, right := range b.layers {
			if left == right {
				return true
			}
		}
	}
	return false
}

func copperLayersForConnectivity(layers []kicadfiles.BoardLayer) ([]kicadfiles.BoardLayer, bool) {
	copperLayers := make([]kicadfiles.BoardLayer, 0, len(layers))
	seen := map[kicadfiles.BoardLayer]struct{}{}
	allCopper := false
	for _, layer := range layers {
		if layer == kicadfiles.LayerAllCu {
			allCopper = true
			continue
		}
		if !isCopperLayer(layer) {
			continue
		}
		if _, ok := seen[layer]; ok {
			continue
		}
		seen[layer] = struct{}{}
		copperLayers = append(copperLayers, layer)
	}
	return copperLayers, allCopper
}

func connectivityBoardCopperLayers(layers []LayerDefinition) []kicadfiles.BoardLayer {
	var out []kicadfiles.BoardLayer
	seen := map[kicadfiles.BoardLayer]struct{}{}
	for _, layer := range layers {
		if !isCopperLayer(layer.Name) || layer.Name == kicadfiles.LayerAllCu {
			continue
		}
		if _, ok := seen[layer.Name]; ok {
			continue
		}
		seen[layer.Name] = struct{}{}
		out = append(out, layer.Name)
	}
	if len(out) == 0 {
		return []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu}
	}
	return out
}

func expandConnectivityLayerSpan(rawLayers []kicadfiles.BoardLayer, parsed []kicadfiles.BoardLayer, boardCopperLayers []kicadfiles.BoardLayer) []kicadfiles.BoardLayer {
	if len(rawLayers) < 2 {
		return parsed
	}
	start := rawLayers[0]
	end := rawLayers[len(rawLayers)-1]
	if start == kicadfiles.LayerAllCu || end == kicadfiles.LayerAllCu {
		return append([]kicadfiles.BoardLayer(nil), boardCopperLayers...)
	}
	startIndex := -1
	endIndex := -1
	for index, layer := range boardCopperLayers {
		if layer == start {
			startIndex = index
		}
		if layer == end {
			endIndex = index
		}
	}
	if startIndex < 0 || endIndex < 0 {
		return parsed
	}
	if startIndex > endIndex {
		startIndex, endIndex = endIndex, startIndex
	}
	if endIndex-startIndex+1 <= len(parsed) {
		return parsed
	}
	return append([]kicadfiles.BoardLayer(nil), boardCopperLayers[startIndex:endIndex+1]...)
}

func pointTouchesPad(point kicadfiles.Point, pad connectivityAnchor) bool {
	local := rotatePoint(kicadfiles.Point{X: point.X - pad.point.X, Y: point.Y - pad.point.Y}, -pad.padRotation)
	halfX := pad.padSize.X / 2
	halfY := pad.padSize.Y / 2
	switch pad.padShape {
	case "circle":
		radius := halfX
		if halfY < radius {
			radius = halfY
		}
		return pointWithinRadius(local, kicadfiles.Point{}, radius+connectivityToleranceIU)
	case "oval":
		return pointTouchesOval(local, halfX, halfY)
	default:
		return absIU(local.X) <= halfX+connectivityToleranceIU && absIU(local.Y) <= halfY+connectivityToleranceIU
	}
}

func pointTouchesOval(local kicadfiles.Point, halfX, halfY kicadfiles.IU) bool {
	if halfX >= halfY {
		capOffset := halfX - halfY
		if absIU(local.X) <= capOffset && absIU(local.Y) <= halfY+connectivityToleranceIU {
			return true
		}
		centerX := capOffset
		if local.X < 0 {
			centerX = -capOffset
		}
		return pointWithinRadius(local, kicadfiles.Point{X: centerX}, halfY+connectivityToleranceIU)
	}

	capOffset := halfY - halfX
	if absIU(local.Y) <= capOffset && absIU(local.X) <= halfX+connectivityToleranceIU {
		return true
	}
	centerY := capOffset
	if local.Y < 0 {
		centerY = -capOffset
	}
	return pointWithinRadius(local, kicadfiles.Point{Y: centerY}, halfX+connectivityToleranceIU)
}

func circleTouchesPad(center kicadfiles.Point, radius kicadfiles.IU, pad connectivityAnchor) bool {
	local := rotatePoint(kicadfiles.Point{X: center.X - pad.point.X, Y: center.Y - pad.point.Y}, -pad.padRotation)
	halfX := pad.padSize.X / 2
	halfY := pad.padSize.Y / 2
	switch pad.padShape {
	case "circle":
		padRadius := halfX
		if halfY < padRadius {
			padRadius = halfY
		}
		return pointWithinRadius(local, kicadfiles.Point{}, radius+padRadius+connectivityToleranceIU)
	case "oval":
		return pointTouchesInflatedOval(local, halfX, halfY, radius)
	default:
		closest := kicadfiles.Point{
			X: clampIU(local.X, -halfX, halfX),
			Y: clampIU(local.Y, -halfY, halfY),
		}
		return pointWithinRadius(local, closest, radius+connectivityToleranceIU)
	}
}

func pointTouchesInflatedOval(local kicadfiles.Point, halfX, halfY, radius kicadfiles.IU) bool {
	if halfX >= halfY {
		capOffset := halfX - halfY
		inflatedRadius := halfY + radius + connectivityToleranceIU
		if absIU(local.X) <= capOffset && absIU(local.Y) <= inflatedRadius {
			return true
		}
		centerX := capOffset
		if local.X < 0 {
			centerX = -capOffset
		}
		return pointWithinRadius(local, kicadfiles.Point{X: centerX}, inflatedRadius)
	}

	capOffset := halfY - halfX
	inflatedRadius := halfX + radius + connectivityToleranceIU
	if absIU(local.Y) <= capOffset && absIU(local.X) <= inflatedRadius {
		return true
	}
	centerY := capOffset
	if local.Y < 0 {
		centerY = -capOffset
	}
	return pointWithinRadius(local, kicadfiles.Point{Y: centerY}, inflatedRadius)
}

func pointOnSegmentThroughPad(start, end kicadfiles.Point, inflate kicadfiles.IU, pad connectivityAnchor) bool {
	return pointTouchesPad(start, pad) || pointTouchesPad(end, pad) || segmentIntersectsPad(start, end, inflate, pad)
}

func segmentIntersectsPad(start, end kicadfiles.Point, inflate kicadfiles.IU, pad connectivityAnchor) bool {
	localStart := rotatePoint(kicadfiles.Point{X: start.X - pad.point.X, Y: start.Y - pad.point.Y}, -pad.padRotation)
	localEnd := rotatePoint(kicadfiles.Point{X: end.X - pad.point.X, Y: end.Y - pad.point.Y}, -pad.padRotation)
	halfX := pad.padSize.X / 2
	halfY := pad.padSize.Y / 2
	switch pad.padShape {
	case "circle":
		radius := halfX
		if halfY < radius {
			radius = halfY
		}
		return segmentDistanceToPoint(localStart, localEnd, kicadfiles.Point{}) <= float64(radius+inflate+connectivityToleranceIU)
	case "oval":
		return segmentIntersectsOval(localStart, localEnd, halfX, halfY, inflate)
	default:
		return segmentIntersectsBox(localStart, localEnd, halfX+inflate+connectivityToleranceIU, halfY+inflate+connectivityToleranceIU)
	}
}

func segmentIntersectsOval(localStart, localEnd kicadfiles.Point, halfX, halfY, inflate kicadfiles.IU) bool {
	if halfX >= halfY {
		capOffset := halfX - halfY
		if segmentIntersectsBox(localStart, localEnd, capOffset, halfY+inflate+connectivityToleranceIU) {
			return true
		}
		radius := float64(halfY + inflate + connectivityToleranceIU)
		return segmentDistanceToPoint(localStart, localEnd, kicadfiles.Point{X: -capOffset}) <= radius ||
			segmentDistanceToPoint(localStart, localEnd, kicadfiles.Point{X: capOffset}) <= radius
	}
	capOffset := halfY - halfX
	if segmentIntersectsBox(localStart, localEnd, halfX+inflate+connectivityToleranceIU, capOffset) {
		return true
	}
	radius := float64(halfX + inflate + connectivityToleranceIU)
	return segmentDistanceToPoint(localStart, localEnd, kicadfiles.Point{Y: -capOffset}) <= radius ||
		segmentDistanceToPoint(localStart, localEnd, kicadfiles.Point{Y: capOffset}) <= radius
}

func segmentIntersectsBox(localStart, localEnd kicadfiles.Point, halfX, halfY kicadfiles.IU) bool {
	if pointInsideBox(localStart, halfX, halfY) || pointInsideBox(localEnd, halfX, halfY) {
		return true
	}
	edges := [][2]kicadfiles.Point{
		{{X: -halfX, Y: -halfY}, {X: halfX, Y: -halfY}},
		{{X: halfX, Y: -halfY}, {X: halfX, Y: halfY}},
		{{X: halfX, Y: halfY}, {X: -halfX, Y: halfY}},
		{{X: -halfX, Y: halfY}, {X: -halfX, Y: -halfY}},
	}
	for _, edge := range edges {
		if segmentsIntersect(localStart, localEnd, edge[0], edge[1]) {
			return true
		}
	}
	return false
}

func segmentDistanceToPoint(start, end, point kicadfiles.Point) float64 {
	dx := float64(end.X - start.X)
	dy := float64(end.Y - start.Y)
	if dx == 0 && dy == 0 {
		return math.Hypot(float64(point.X-start.X), float64(point.Y-start.Y))
	}
	t := ((float64(point.X-start.X) * dx) + (float64(point.Y-start.Y) * dy)) / (dx*dx + dy*dy)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	closestX := float64(start.X) + t*dx
	closestY := float64(start.Y) + t*dy
	return math.Hypot(float64(point.X)-closestX, float64(point.Y)-closestY)
}

func pointInsideBox(point kicadfiles.Point, halfX, halfY kicadfiles.IU) bool {
	return absIU(point.X) <= halfX && absIU(point.Y) <= halfY
}

func segmentsIntersect(aStart, aEnd, bStart, bEnd kicadfiles.Point) bool {
	o1 := orientationSign(aStart, aEnd, bStart)
	o2 := orientationSign(aStart, aEnd, bEnd)
	o3 := orientationSign(bStart, bEnd, aStart)
	o4 := orientationSign(bStart, bEnd, aEnd)
	if o1 == 0 && pointWithinSegmentBounds(bStart, aStart, aEnd) {
		return true
	}
	if o2 == 0 && pointWithinSegmentBounds(bEnd, aStart, aEnd) {
		return true
	}
	if o3 == 0 && pointWithinSegmentBounds(aStart, bStart, bEnd) {
		return true
	}
	if o4 == 0 && pointWithinSegmentBounds(aEnd, bStart, bEnd) {
		return true
	}
	return o1 != o2 && o3 != o4
}

func orientationSign(a, b, c kicadfiles.Point) int {
	cross := int64(b.X-a.X)*int64(c.Y-a.Y) - int64(b.Y-a.Y)*int64(c.X-a.X)
	if cross > 0 {
		return 1
	}
	if cross < 0 {
		return -1
	}
	return 0
}

func pointWithinSegmentBounds(point, start, end kicadfiles.Point) bool {
	return point.X >= minIU(start.X, end.X)-connectivityToleranceIU &&
		point.X <= maxIU(start.X, end.X)+connectivityToleranceIU &&
		point.Y >= minIU(start.Y, end.Y)-connectivityToleranceIU &&
		point.Y <= maxIU(start.Y, end.Y)+connectivityToleranceIU
}

func padsOverlap(a, b connectivityAnchor) bool {
	aCorners := padBoundingCorners(a)
	bCorners := padBoundingCorners(b)
	for _, corner := range aCorners {
		if pointTouchesPad(corner, b) {
			return true
		}
	}
	for _, corner := range bCorners {
		if pointTouchesPad(corner, a) {
			return true
		}
	}
	for i := range aCorners {
		aStart := aCorners[i]
		aEnd := aCorners[(i+1)%len(aCorners)]
		for j := range bCorners {
			bStart := bCorners[j]
			bEnd := bCorners[(j+1)%len(bCorners)]
			if segmentsIntersect(aStart, aEnd, bStart, bEnd) {
				return true
			}
		}
	}
	return false
}

func padBoundingCorners(pad connectivityAnchor) []kicadfiles.Point {
	halfX := pad.padSize.X/2 + connectivityToleranceIU
	halfY := pad.padSize.Y/2 + connectivityToleranceIU
	localCorners := []kicadfiles.Point{
		{X: -halfX, Y: -halfY},
		{X: halfX, Y: -halfY},
		{X: halfX, Y: halfY},
		{X: -halfX, Y: halfY},
	}
	corners := make([]kicadfiles.Point, 0, len(localCorners))
	for _, corner := range localCorners {
		rotated := rotatePoint(corner, pad.padRotation)
		corners = append(corners, kicadfiles.Point{X: pad.point.X + rotated.X, Y: pad.point.Y + rotated.Y})
	}
	return corners
}

func arcCells(start, mid, end kicadfiles.Point) []connectivityCell {
	points := arcPolylinePoints(start, mid, end)
	if len(points) < 2 {
		return segmentCells(start, end)
	}
	seen := map[connectivityCell]struct{}{}
	cells := []connectivityCell{}
	for i := 1; i < len(points); i++ {
		for _, cell := range segmentCells(points[i-1], points[i]) {
			if _, ok := seen[cell]; ok {
				continue
			}
			seen[cell] = struct{}{}
			cells = append(cells, cell)
		}
	}
	return cells
}

func arcTouchesPad(start, mid, end kicadfiles.Point, inflate kicadfiles.IU, pad connectivityAnchor) bool {
	points := arcPolylinePoints(start, mid, end)
	if len(points) < 2 {
		return pointOnSegmentThroughPad(start, end, inflate, pad)
	}
	for i := 1; i < len(points); i++ {
		if pointTouchesPad(points[i], pad) || segmentIntersectsPad(points[i-1], points[i], inflate, pad) {
			return true
		}
	}
	return false
}

func pointOnArc(point, start, mid, end kicadfiles.Point, tolerance kicadfiles.IU) bool {
	circle, ok := circleFromThreePoints(start, mid, end)
	if !ok {
		return pointOnSegment(point, start, end, tolerance)
	}
	distance := math.Hypot(float64(point.X)-circle.x, float64(point.Y)-circle.y)
	if math.Abs(distance-circle.radius) > float64(tolerance) {
		return false
	}
	return angleOnArc(math.Atan2(float64(point.Y)-circle.y, float64(point.X)-circle.x), arcAngles(circle, start, mid, end))
}

func arcPolylinePoints(start, mid, end kicadfiles.Point) []kicadfiles.Point {
	circle, ok := circleFromThreePoints(start, mid, end)
	if !ok {
		return []kicadfiles.Point{start, end}
	}
	angles := arcAngles(circle, start, mid, end)
	segments := arcPolylineSegmentCount(circle.radius, math.Abs(angles.sweep), float64(connectivityToleranceIU))
	points := make([]kicadfiles.Point, 0, segments+1)
	for i := 0; i <= segments; i++ {
		t := float64(i) / float64(segments)
		angle := angles.start + angles.sweep*t
		points = append(points, kicadfiles.Point{
			X: kicadfiles.IU(math.Round(circle.x + circle.radius*math.Cos(angle))),
			Y: kicadfiles.IU(math.Round(circle.y + circle.radius*math.Sin(angle))),
		})
	}
	return points
}

func arcPolylineSegmentCount(radius, sweep, tolerance float64) int {
	if radius <= 0 || sweep <= 0 {
		return 1
	}
	if tolerance <= 0 {
		return int(math.Ceil(sweep / (math.Pi / 64)))
	}
	if radius <= tolerance {
		return 1
	}
	acosInput := 1 - tolerance/radius
	if acosInput < -1 {
		acosInput = -1
	} else if acosInput > 1 {
		acosInput = 1
	}
	maxTheta := 2 * math.Acos(acosInput)
	if maxTheta <= 0 || math.IsNaN(maxTheta) {
		return int(math.Ceil(sweep / (math.Pi / 64)))
	}
	segments := int(math.Ceil(sweep / maxTheta))
	if segments < 1 {
		return 1
	}
	return segments
}

type connectivityCircle struct {
	x      float64
	y      float64
	radius float64
}

type connectivityArcAngles struct {
	start float64
	sweep float64
}

func circleFromThreePoints(start, mid, end kicadfiles.Point) (connectivityCircle, bool) {
	x1, y1 := float64(start.X), float64(start.Y)
	x2, y2 := float64(mid.X), float64(mid.Y)
	x3, y3 := float64(end.X), float64(end.Y)
	d := 2 * (x1*(y2-y3) + x2*(y3-y1) + x3*(y1-y2))
	if math.Abs(d) < 1 {
		return connectivityCircle{}, false
	}
	ux := ((x1*x1+y1*y1)*(y2-y3) + (x2*x2+y2*y2)*(y3-y1) + (x3*x3+y3*y3)*(y1-y2)) / d
	uy := ((x1*x1+y1*y1)*(x3-x2) + (x2*x2+y2*y2)*(x1-x3) + (x3*x3+y3*y3)*(x2-x1)) / d
	return connectivityCircle{x: ux, y: uy, radius: math.Hypot(x1-ux, y1-uy)}, true
}

func arcAngles(circle connectivityCircle, start, mid, end kicadfiles.Point) connectivityArcAngles {
	startAngle := math.Atan2(float64(start.Y)-circle.y, float64(start.X)-circle.x)
	midAngle := math.Atan2(float64(mid.Y)-circle.y, float64(mid.X)-circle.x)
	endAngle := math.Atan2(float64(end.Y)-circle.y, float64(end.X)-circle.x)
	counterClockwiseSweep := normalizeAngle(endAngle - startAngle)
	midSweep := normalizeAngle(midAngle - startAngle)
	if midSweep <= counterClockwiseSweep {
		return connectivityArcAngles{start: startAngle, sweep: counterClockwiseSweep}
	}
	return connectivityArcAngles{start: startAngle, sweep: counterClockwiseSweep - 2*math.Pi}
}

func angleOnArc(angle float64, angles connectivityArcAngles) bool {
	position := normalizeAngle(angle - angles.start)
	if angles.sweep >= 0 {
		return position <= angles.sweep+1e-9
	}
	return position >= normalizeAngle(angles.sweep)-1e-9
}

func normalizeAngle(angle float64) float64 {
	angle = math.Mod(angle, 2*math.Pi)
	if angle < 0 {
		angle += 2 * math.Pi
	}
	return angle
}

func minIU(a, b kicadfiles.IU) kicadfiles.IU {
	if a < b {
		return a
	}
	return b
}

func maxIU(a, b kicadfiles.IU) kicadfiles.IU {
	if a > b {
		return a
	}
	return b
}

func clampIU(value, minValue, maxValue kicadfiles.IU) kicadfiles.IU {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func absolutePadPosition(footprint Footprint, pad Pad) kicadfiles.Point {
	padPosition := rotatePoint(pad.Position, footprint.Rotation)
	return kicadfiles.Point{X: footprint.Position.X + padPosition.X, Y: footprint.Position.Y + padPosition.Y}
}

func rotatePoint(point kicadfiles.Point, angle kicadfiles.Angle) kicadfiles.Point {
	x, y := kicadfiles.RotateBoardLocalXY(float64(point.X), float64(point.Y), float64(angle))
	return kicadfiles.Point{
		X: kicadfiles.IU(math.Round(x)),
		Y: kicadfiles.IU(math.Round(y)),
	}
}

type connectivityUnion struct {
	parent []int
}

func newConnectivityUnion(size int) connectivityUnion {
	parent := make([]int, size)
	for i := range parent {
		parent[i] = i
	}
	return connectivityUnion{parent: parent}
}

func (uf connectivityUnion) find(value int) int {
	for uf.parent[value] != value {
		uf.parent[value] = uf.parent[uf.parent[value]]
		value = uf.parent[value]
	}
	return value
}

func (uf connectivityUnion) union(a, b int) {
	rootA := uf.find(a)
	rootB := uf.find(b)
	if rootA != rootB {
		uf.parent[rootB] = rootA
	}
}

func netDisplayName(code int, netNames map[int]string) string {
	name := strings.TrimSpace(netNames[code])
	if name == "" {
		return fmt.Sprintf("%d", code)
	}
	return name
}
