package routing

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"kicadai/internal/kicadfiles"
)

const maxOccupancyCellsPerLayer = 5_000_000

type Occupancy struct {
	Grid      Grid
	Layers    map[int]*LayerOccupancy
	Obstacles []Obstacle
}

type LayerOccupancy struct {
	MinX          int
	MinY          int
	Width         int
	Height        int
	Blocked       []bool
	FirstObstacle []int
}

type gridPoint struct {
	X int
	Y int
}

func BuildOccupancy(request Request, currentNet string) (Occupancy, error) {
	request = cloneRequest(request)
	NormalizeRequest(&request)
	return buildOccupancy(request, currentNet, clearanceObjectTrace, request.Rules.TraceWidthMM/2, newClearancePolicy(request))
}

func BuildViaOccupancy(request Request, currentNet string) (Occupancy, error) {
	request = cloneRequest(request)
	NormalizeRequest(&request)
	return buildOccupancy(request, currentNet, clearanceObjectVia, request.Rules.ViaDiameterMM/2, newClearancePolicy(request))
}

func BuildTraceAndViaOccupancy(request Request, currentNet string) (Occupancy, Occupancy, error) {
	request = cloneRequest(request)
	NormalizeRequest(&request)
	clearances := newClearancePolicy(request)
	traceOccupancy, err := buildOccupancy(request, currentNet, clearanceObjectTrace, request.Rules.TraceWidthMM/2, clearances)
	if err != nil {
		return Occupancy{}, Occupancy{}, err
	}
	viaOccupancy, err := buildOccupancy(request, currentNet, clearanceObjectVia, request.Rules.ViaDiameterMM/2, clearances)
	if err != nil {
		return Occupancy{}, Occupancy{}, err
	}
	return traceOccupancy, viaOccupancy, nil
}

func buildOccupancy(request Request, currentNet string, movingKind clearanceObjectKind, movingCopperRadiusMM float64, clearances *clearancePolicy) (Occupancy, error) {
	currentNetKey := strings.TrimSpace(currentNet)
	if ruleIssue, blocked := clearances.firstBlockingIssue(); blocked {
		return Occupancy{}, fmt.Errorf("resolve routing clearance policy: %s", ruleIssue.Message)
	}
	grid := NewGrid(Point{}, request.Rules.GridMM)
	occupancy := Occupancy{Grid: grid, Layers: map[int]*LayerOccupancy{}}
	layerIndexes, err := LayerIndexes(request.Board.Layers)
	if err != nil {
		return occupancy, err
	}
	usable := usableBoardRectForRadius(request.Board, request.Rules, movingCopperRadiusMM)
	board := BoardRect(request.Board)
	for _, layer := range request.Board.Layers {
		if layer.Kind != LayerCopper {
			continue
		}
		layerIndex := layerIndexes[normalizeLayer(layer.Name)]
		if err := occupancy.ensureLayer(layerIndex, board); err != nil {
			return occupancy, err
		}
		occupancy.blockOutsideUsable(layerIndex, board, usable)
	}
	for _, obstacle := range request.Obstacles {
		clearanceMM := clearances.obstacle(currentNetKey, movingKind, obstacle)
		if strings.TrimSpace(obstacle.Layer) == "" {
			for _, layer := range request.Board.Layers {
				if layer.Kind == LayerCopper {
					occupancy.addShape(layerIndexes, layer.Name, obstacle.Geometry, clearanceMM+movingCopperRadiusMM, obstacle)
				}
			}
			continue
		}
		occupancy.addShape(layerIndexes, obstacle.Layer, obstacle.Geometry, clearanceMM+movingCopperRadiusMM, obstacle)
	}
	for _, copper := range request.Existing {
		if copper.Kind == CopperZone {
			switch request.Strategy.TreatZonesAs {
			case ZoneIgnore:
				continue
			case ZoneUnsupported, ZoneSufficient:
				return Occupancy{}, fmt.Errorf("zone routing policy %q is not implemented by the router", request.Strategy.TreatZonesAs)
			}
		}
		if sameOccupancyNet(copper.Net, currentNetKey) {
			continue
		}
		kind := ObstacleExistingCopper
		source := "existing_copper"
		if copper.Kind == CopperZone {
			kind = ObstacleZone
			source = "zone"
		}
		clearanceMM := clearances.pair(currentNetKey, movingKind, copper.Net, existingCopperKind(copper.Kind))
		obstacle := Obstacle{Kind: kind, Layer: copper.Layer, Geometry: copper.Geometry, Clearance: clearanceMM, Source: source, Net: copper.Net}
		if copper.Kind == CopperVia {
			for _, layer := range request.Board.Layers {
				if layer.Kind == LayerCopper {
					obstacle.Layer = layer.Name
					occupancy.addShape(layerIndexes, layer.Name, copper.Geometry, clearanceMM+movingCopperRadiusMM, obstacle)
				}
			}
			continue
		}
		occupancy.addShape(layerIndexes, copper.Layer, copper.Geometry, clearanceMM+movingCopperRadiusMM, obstacle)
	}
	for _, component := range request.Components {
		for _, pad := range component.Pads {
			if sameOccupancyNet(pad.Net, currentNetKey) {
				continue
			}
			clearanceMM := clearances.pad(currentNetKey, movingKind, pad)
			obstacle := Obstacle{Kind: ObstacleOtherNetPad, Geometry: padRect(component, pad), Clearance: clearanceMM, Source: component.Ref + "." + pad.Name}
			for _, layer := range padAccessLayers(pad, routableLayerNames(request.Board.Layers)) {
				obstacle.Layer = layer
				occupancy.addShape(layerIndexes, layer, obstacle.Geometry, clearanceMM+movingCopperRadiusMM, obstacle)
			}
		}
	}
	return occupancy, nil
}

func sameOccupancyNet(left string, right string) bool {
	left = strings.TrimSpace(left)
	if left == "" || right == "" {
		return false
	}
	return strings.EqualFold(left, right)
}

func (occupancy Occupancy) BlockedCell(coord GridCoord) bool {
	layer, ok := occupancy.Layers[coord.Layer]
	if !ok {
		return false
	}
	index, ok := layer.index(gridPoint{X: coord.X, Y: coord.Y})
	return ok && layer.Blocked[index]
}

func (occupancy Occupancy) FirstObstacle(coord GridCoord) (Obstacle, bool) {
	layer, ok := occupancy.Layers[coord.Layer]
	if !ok {
		return Obstacle{}, false
	}
	cellIndex, ok := layer.index(gridPoint{X: coord.X, Y: coord.Y})
	if !ok {
		return Obstacle{}, false
	}
	obstacleIndex := layer.FirstObstacle[cellIndex]
	if obstacleIndex < 0 || obstacleIndex >= len(occupancy.Obstacles) {
		return Obstacle{}, false
	}
	return occupancy.Obstacles[obstacleIndex], true
}

func (occupancy *Occupancy) addShape(layerIndexes map[string]int, layer string, shape Shape, inflateMM float64, obstacle Obstacle) {
	if shape.Rect == nil && len(shape.Polygon) == 0 {
		return
	}
	layer = normalizeLayer(layer)
	layerIndex, ok := layerIndexes[layer]
	if !ok {
		return
	}
	obstacleIndex := occupancy.addObstacle(obstacle)
	rect := shapeBounds(shape).Expand(inflateMM)
	minCoord := occupancy.Grid.ToGrid(rect.Min, layerIndex)
	maxCoord := occupancy.Grid.ToGrid(rect.Max, layerIndex)
	layerGrid := occupancy.Layers[layerIndex]
	if layerGrid == nil {
		return
	}
	minX := max(minCoord.X, layerGrid.MinX)
	minY := max(minCoord.Y, layerGrid.MinY)
	maxX := min(maxCoord.X, layerGrid.MinX+layerGrid.Width-1)
	maxY := min(maxCoord.Y, layerGrid.MinY+layerGrid.Height-1)
	if minX > maxX || minY > maxY {
		return
	}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			point := occupancy.Grid.ToPoint(GridCoord{X: x, Y: y, Layer: layerIndex})
			withinShape := false
			if shape.Rect != nil && pointWithinRectClearance(point, *shape.Rect, inflateMM) {
				withinShape = true
			}
			if len(shape.Polygon) > 0 && pointWithinPolygonClearance(point, shape.Polygon, inflateMM) {
				withinShape = true
			}
			if !withinShape {
				continue
			}
			occupancy.block(layerIndex, gridPoint{X: x, Y: y}, obstacleIndex)
		}
	}
}

func pointWithinRectClearance(point Point, rect Rect, clearanceMM float64) bool {
	rect = normalizeRect(rect)
	if rect.ContainsPoint(point) {
		return true
	}
	dx := max(rect.Min.XMM-point.XMM, 0, point.XMM-rect.Max.XMM)
	dy := max(rect.Min.YMM-point.YMM, 0, point.YMM-rect.Max.YMM)
	effectiveClearanceMM := max(0, clearanceMM-distanceEpsilon)
	if effectiveClearanceMM == 0 {
		return false
	}
	return math.Hypot(dx, dy) < effectiveClearanceMM
}

func (occupancy *Occupancy) blockOutsideUsable(layerIndex int, board Rect, usable Rect) {
	boardMin := occupancy.Grid.ToGrid(board.Min, layerIndex)
	boardMax := occupancy.Grid.ToGrid(board.Max, layerIndex)
	usableMin := occupancy.Grid.ToGridInwardMin(usable.Min, layerIndex)
	usableMax := occupancy.Grid.ToGridInwardMax(usable.Max, layerIndex)
	obstacleIndex := occupancy.addObstacle(Obstacle{Kind: ObstacleBoardEdge, Source: "board_edge"})
	occupancy.blockRect(layerIndex, boardMin.X, boardMin.Y, boardMax.X, usableMin.Y-1, obstacleIndex)
	occupancy.blockRect(layerIndex, boardMin.X, usableMax.Y+1, boardMax.X, boardMax.Y, obstacleIndex)
	occupancy.blockRect(layerIndex, boardMin.X, usableMin.Y, usableMin.X-1, usableMax.Y, obstacleIndex)
	occupancy.blockRect(layerIndex, usableMax.X+1, usableMin.Y, boardMax.X, usableMax.Y, obstacleIndex)
}

func (occupancy *Occupancy) blockRect(layerIndex int, minX int, minY int, maxX int, maxY int, obstacleIndex int) {
	layerGrid := occupancy.Layers[layerIndex]
	if layerGrid == nil {
		return
	}
	minX = max(minX, layerGrid.MinX)
	minY = max(minY, layerGrid.MinY)
	maxX = min(maxX, layerGrid.MinX+layerGrid.Width-1)
	maxY = min(maxY, layerGrid.MinY+layerGrid.Height-1)
	if minX > maxX || minY > maxY {
		return
	}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			occupancy.block(layerIndex, gridPoint{X: x, Y: y}, obstacleIndex)
		}
	}
}

func (occupancy *Occupancy) block(layer int, point gridPoint, obstacleIndex int) {
	layerGrid := occupancy.Layers[layer]
	if layerGrid == nil {
		return
	}
	index, ok := layerGrid.index(point)
	if !ok {
		return
	}
	layerGrid.Blocked[index] = true
	if layerGrid.FirstObstacle[index] < 0 {
		layerGrid.FirstObstacle[index] = obstacleIndex
	}
}

func (occupancy *Occupancy) ensureLayer(layerIndex int, board Rect) error {
	min := occupancy.Grid.ToGrid(board.Min, layerIndex)
	max := occupancy.Grid.ToGrid(board.Max, layerIndex)
	width := max.X - min.X + 1
	height := max.Y - min.Y + 1
	if width <= 0 || height <= 0 {
		width = 1
		height = 1
	}
	if width*height > maxOccupancyCellsPerLayer {
		return fmt.Errorf("occupancy grid too large for layer %d: %d cells", layerIndex, width*height)
	}
	firstObstacle := make([]int, width*height)
	for index := range firstObstacle {
		firstObstacle[index] = -1
	}
	occupancy.Layers[layerIndex] = &LayerOccupancy{
		MinX:          min.X,
		MinY:          min.Y,
		Width:         width,
		Height:        height,
		Blocked:       make([]bool, width*height),
		FirstObstacle: firstObstacle,
	}
	return nil
}

func (occupancy *Occupancy) addObstacle(obstacle Obstacle) int {
	index := len(occupancy.Obstacles)
	occupancy.Obstacles = append(occupancy.Obstacles, obstacle)
	return index
}

func (layer LayerOccupancy) index(point gridPoint) (int, bool) {
	x := point.X - layer.MinX
	y := point.Y - layer.MinY
	if x < 0 || y < 0 || x >= layer.Width || y >= layer.Height {
		return 0, false
	}
	return y*layer.Width + x, true
}

func padRect(component Component, pad Pad) Shape {
	width := pad.Size.WidthMM
	height := pad.Size.HeightMM
	center := absolutePadPoint(component, pad.Position)
	corners := []Point{
		{XMM: -width / 2, YMM: -height / 2},
		{XMM: width / 2, YMM: -height / 2},
		{XMM: width / 2, YMM: height / 2},
		{XMM: -width / 2, YMM: height / 2},
	}
	minPoint := Point{XMM: math.Inf(1), YMM: math.Inf(1)}
	maxPoint := Point{XMM: math.Inf(-1), YMM: math.Inf(-1)}
	polygon := make([]Point, 0, len(corners))
	for _, corner := range corners {
		rotatedX, rotatedY := kicadfiles.RotateBoardLocalXY(corner.XMM, corner.YMM, component.Position.RotationDeg+pad.RotationDeg)
		x := center.XMM + rotatedX
		y := center.YMM + rotatedY
		polygon = append(polygon, Point{XMM: x, YMM: y})
		minPoint.XMM = math.Min(minPoint.XMM, x)
		minPoint.YMM = math.Min(minPoint.YMM, y)
		maxPoint.XMM = math.Max(maxPoint.XMM, x)
		maxPoint.YMM = math.Max(maxPoint.YMM, y)
	}
	return Shape{Rect: &Rect{
		Min: minPoint,
		Max: maxPoint,
	}, Polygon: polygon}
}

func absolutePadPoint(component Component, relative Point) Point {
	x, y := kicadfiles.RotateBoardLocalXY(relative.XMM, relative.YMM, component.Position.RotationDeg)
	return Point{
		XMM: component.Position.XMM + x,
		YMM: component.Position.YMM + y,
	}
}

func shapeBounds(shape Shape) Rect {
	var bounds Rect
	hasBounds := false
	if shape.Rect != nil {
		bounds = *shape.Rect
		hasBounds = true
	}
	if len(shape.Polygon) == 0 {
		return bounds
	}
	minPoint := Point{XMM: math.Inf(1), YMM: math.Inf(1)}
	maxPoint := Point{XMM: math.Inf(-1), YMM: math.Inf(-1)}
	for _, point := range shape.Polygon {
		minPoint.XMM = math.Min(minPoint.XMM, point.XMM)
		minPoint.YMM = math.Min(minPoint.YMM, point.YMM)
		maxPoint.XMM = math.Max(maxPoint.XMM, point.XMM)
		maxPoint.YMM = math.Max(maxPoint.YMM, point.YMM)
	}
	polygonBounds := Rect{Min: minPoint, Max: maxPoint}
	if !hasBounds {
		return polygonBounds
	}
	return Rect{
		Min: Point{XMM: math.Min(bounds.Min.XMM, polygonBounds.Min.XMM), YMM: math.Min(bounds.Min.YMM, polygonBounds.Min.YMM)},
		Max: Point{XMM: math.Max(bounds.Max.XMM, polygonBounds.Max.XMM), YMM: math.Max(bounds.Max.YMM, polygonBounds.Max.YMM)},
	}
}

func pointInPolygon(point Point, polygon []Point) bool {
	if len(polygon) < 3 {
		return false
	}
	inside := false
	j := len(polygon) - 1
	for i := range polygon {
		pi := polygon[i]
		pj := polygon[j]
		if ((pi.YMM > point.YMM) != (pj.YMM > point.YMM)) &&
			(point.XMM < (pj.XMM-pi.XMM)*(point.YMM-pi.YMM)/(pj.YMM-pi.YMM)+pi.XMM) {
			inside = !inside
		}
		j = i
	}
	return inside
}

func pointWithinPolygonClearance(point Point, polygon []Point, clearanceMM float64) bool {
	if pointInPolygon(point, polygon) {
		return true
	}
	effectiveClearanceMM := max(0, clearanceMM-distanceEpsilon)
	if effectiveClearanceMM == 0 {
		return false
	}
	return distanceToPolygon(point, polygon) < effectiveClearanceMM
}

func distanceToPolygon(point Point, polygon []Point) float64 {
	if len(polygon) == 0 {
		return math.Inf(1)
	}
	best := math.Inf(1)
	for index := range polygon {
		next := (index + 1) % len(polygon)
		best = math.Min(best, distancePointToSegment(point, polygon[index], polygon[next]))
	}
	return best
}

func distancePointToSegment(point Point, start Point, end Point) float64 {
	dx := end.XMM - start.XMM
	dy := end.YMM - start.YMM
	if dx == 0 && dy == 0 {
		return math.Hypot(point.XMM-start.XMM, point.YMM-start.YMM)
	}
	t := ((point.XMM-start.XMM)*dx + (point.YMM-start.YMM)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	closest := Point{XMM: start.XMM + t*dx, YMM: start.YMM + t*dy}
	return math.Hypot(point.XMM-closest.XMM, point.YMM-closest.YMM)
}

func occupancyLayerKeys(occupancy Occupancy) []int {
	keys := make([]int, 0, len(occupancy.Layers))
	for key := range occupancy.Layers {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}
