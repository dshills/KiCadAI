package routing

import (
	"container/heap"
	"context"
	"math"
	"sort"

	"kicadai/internal/reports"
)

type GridPath struct {
	Net            string         `json:"net"`
	Layer          string         `json:"layer"`
	LayerNames     map[int]string `json:"layer_names,omitempty"`
	Coordinates    []GridCoord    `json:"coordinates"`
	Points         []Point        `json:"points"`
	SearchNodes    int            `json:"search_nodes"`
	SearchLimitHit bool           `json:"search_limit_hit,omitempty"`
}

type astarState struct {
	Coord GridCoord
	Dir   int
	Vias  int
}

type astarNode struct {
	State    astarState
	G        float64
	F        float64
	Sequence int
}

type astarQueue []*astarNode

const (
	routeDirNone = iota
	routeDirEast
	routeDirWest
	routeDirSouth
	routeDirNorth
)

const astarContextCheckInterval = 1024

func routeSingleLayerPath(ctx context.Context, request Request, access PadAccess, occupancy Occupancy, netName string, pair EndpointPair, layerName string) (GridPath, []reports.Issue) {
	layers := normalizedSearchLayers(request.Board.Layers)
	rules := normalizedSearchRules(request.Rules)
	layerIndexes, err := LayerIndexes(layers)
	if err != nil {
		return GridPath{}, []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityBlocked,
			Path:     "board.layers",
			Message:  err.Error(),
			Nets:     []string{netName},
		}}
	}
	normalizedLayer := normalizeLayer(layerName)
	layerIndex, ok := layerIndexes[normalizedLayer]
	if !ok {
		return GridPath{}, []reports.Issue{routeFailureIssue(netName, pair, "routing layer is not available")}
	}
	layerGrid := occupancy.Layers[layerIndex]
	if layerGrid == nil {
		return GridPath{}, []reports.Issue{routeFailureIssue(netName, pair, "routing layer has no occupancy grid")}
	}
	starts := accessCoordsOnLayer(access, occupancy.Grid, pair.From, normalizedLayer, layerIndex)
	targets := accessCoordsOnLayer(access, occupancy.Grid, pair.To, normalizedLayer, layerIndex)
	if len(starts) == 0 || len(targets) == 0 {
		return GridPath{}, []reports.Issue{routeFailureIssue(netName, pair, "endpoint pair has no access points on routing layer")}
	}
	path, searchNodes, found, canceled := astarSearch(ctx, occupancy, layerIndex, starts, targets, rules)
	if canceled {
		return GridPath{}, []reports.Issue{routeCanceledIssue(ctx.Err())}
	}
	if !found {
		return GridPath{
				Net:            netName,
				SearchNodes:    searchNodes,
				SearchLimitHit: searchNodes >= rules.MaxSearchNodes,
			}, []reports.Issue{routeFailureIssueWithObstacle(
				netName,
				pair,
				"no legal single-layer path found",
				nearestObstacleSummary(occupancy, starts, targets),
			)}
	}
	points := make([]Point, 0, len(path))
	for _, coord := range path {
		points = append(points, occupancy.Grid.ToPoint(coord))
	}
	return GridPath{
		Net:            netName,
		Layer:          normalizedLayer,
		LayerNames:     map[int]string{layerIndex: normalizedLayer},
		Coordinates:    path,
		Points:         points,
		SearchNodes:    searchNodes,
		SearchLimitHit: searchNodes >= rules.MaxSearchNodes,
	}, nil
}

func routeTwoLayerPath(ctx context.Context, request Request, access PadAccess, occupancy Occupancy, netName string, pair EndpointPair) (GridPath, []reports.Issue) {
	layers := normalizedSearchLayers(request.Board.Layers)
	rules := normalizedSearchRules(request.Rules)
	if rules.AllowVias != nil && !*rules.AllowVias {
		return GridPath{}, []reports.Issue{routeFailureIssue(netName, pair, "vias are not allowed")}
	}
	layerIndexes, err := LayerIndexes(layers)
	if err != nil {
		return GridPath{}, []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityBlocked,
			Path:     "board.layers",
			Message:  err.Error(),
			Nets:     []string{netName},
		}}
	}
	routable := routableLayerNames(layers)
	if len(rules.AllowedLayers) != 0 {
		routable = filterAllowedLayers(routable, rules.AllowedLayers)
	}
	if rules.AllowBackLayer != nil && !*rules.AllowBackLayer {
		routable = filterAllowedLayers(routable, []string{rules.PreferLayer})
	}
	layerNames := map[int]string{}
	layerIDs := []int{}
	for _, layerName := range routable {
		index, ok := layerIndexes[normalizeLayer(layerName)]
		if !ok || occupancy.Layers[index] == nil {
			continue
		}
		layerIDs = append(layerIDs, index)
		layerNames[index] = normalizeLayer(layerName)
	}
	sort.Ints(layerIDs)
	starts := accessCoordsOnLayers(access, occupancy.Grid, pair.From, layerIndexes, layerNames)
	targets := accessCoordsOnLayers(access, occupancy.Grid, pair.To, layerIndexes, layerNames)
	if len(layerIDs) == 0 || len(starts) == 0 || len(targets) == 0 {
		return GridPath{}, []reports.Issue{routeFailureIssue(netName, pair, "endpoint pair has no two-layer routing access")}
	}
	path, searchNodes, found, canceled := astarSearchMultiLayer(ctx, occupancy, starts, targets, rules, layerIDs, true)
	if canceled {
		return GridPath{}, []reports.Issue{routeCanceledIssue(ctx.Err())}
	}
	if !found {
		return GridPath{
				Net:            netName,
				SearchNodes:    searchNodes,
				SearchLimitHit: searchNodes >= rules.MaxSearchNodes,
			}, []reports.Issue{routeFailureIssueWithObstacle(
				netName,
				pair,
				"no legal two-layer path found",
				nearestObstacleSummary(occupancy, starts, targets),
			)}
	}
	points := make([]Point, 0, len(path))
	for _, coord := range path {
		points = append(points, occupancy.Grid.ToPoint(coord))
	}
	return GridPath{
		Net:            netName,
		Layer:          layerNames[path[0].Layer],
		LayerNames:     layerNames,
		Coordinates:    path,
		Points:         points,
		SearchNodes:    searchNodes,
		SearchLimitHit: searchNodes >= rules.MaxSearchNodes,
	}, nil
}

func astarSearch(ctx context.Context, occupancy Occupancy, layerIndex int, starts []GridCoord, targets []GridCoord, rules Rules) ([]GridCoord, int, bool, bool) {
	return astarSearchMultiLayer(ctx, occupancy, starts, targets, rules, []int{layerIndex}, false)
}

func astarSearchMultiLayer(ctx context.Context, occupancy Occupancy, starts []GridCoord, targets []GridCoord, rules Rules, layerIndexes []int, allowVias bool) ([]GridCoord, int, bool, bool) {
	targetSet := make(map[GridCoord]struct{}, len(targets))
	allowedBlocked := make(map[GridCoord]struct{}, len(starts)+len(targets))
	for _, target := range targets {
		targetSet[target] = struct{}{}
		allowedBlocked[target] = struct{}{}
	}
	for _, start := range starts {
		allowedBlocked[start] = struct{}{}
	}
	gridStepMM := normalizedGridStepMM(occupancy.Grid.GridMM)
	turnPenaltyMM := gridStepMM * 0.15
	viaCostMM := rules.ViaDiameterMM + gridStepMM
	if viaCostMM <= 0 || math.IsNaN(viaCostMM) || math.IsInf(viaCostMM, 0) {
		viaCostMM = DefaultRules().ViaDiameterMM + gridStepMM
	}
	heuristic := newTargetHeuristic(targets, gridStepMM)
	open := astarQueue{}
	heap.Init(&open)
	cameFrom := map[astarState]astarState{}
	bestCost := map[astarState]float64{}
	sequence := 0
	for _, start := range starts {
		if !routableCell(occupancy, start, allowedBlocked) {
			continue
		}
		state := astarState{Coord: start, Dir: routeDirNone}
		bestCost[state] = 0
		heap.Push(&open, &astarNode{
			State:    state,
			G:        0,
			F:        heuristic.estimate(start),
			Sequence: sequence,
		})
		sequence++
	}
	searchNodes := 0
	maxNodes := rules.MaxSearchNodes
	for open.Len() > 0 {
		if searchNodes%astarContextCheckInterval == 0 && ctx.Err() != nil {
			return nil, searchNodes, false, true
		}
		if searchNodes >= maxNodes {
			return nil, searchNodes, false, false
		}
		currentNode := heap.Pop(&open).(*astarNode)
		current := currentNode.State
		if known, ok := bestCost[current]; ok && currentNode.G > known+distanceEpsilon {
			continue
		}
		searchNodes++
		if _, ok := targetSet[current.Coord]; ok {
			return reconstructGridPath(current, cameFrom), searchNodes, true, false
		}
		for _, neighbor := range orthogonalNeighbors(current) {
			if !routableCell(occupancy, neighbor.Coord, allowedBlocked) {
				continue
			}
			tentative := currentNode.G + movementCost(current.Dir, neighbor.Dir, gridStepMM, turnPenaltyMM)
			if existing, ok := bestCost[neighbor]; ok && !distanceLess(tentative, existing) {
				continue
			}
			bestCost[neighbor] = tentative
			cameFrom[neighbor] = current
			heap.Push(&open, &astarNode{
				State:    neighbor,
				G:        tentative,
				F:        tentative + heuristic.estimate(neighbor.Coord),
				Sequence: sequence,
			})
			sequence++
		}
		if allowVias && current.Vias < rules.MaxViasPerNet {
			for _, layerIndex := range layerIndexes {
				if layerIndex == current.Coord.Layer {
					continue
				}
				neighbor := astarState{
					Coord: GridCoord{X: current.Coord.X, Y: current.Coord.Y, Layer: layerIndex},
					Dir:   routeDirNone,
					Vias:  current.Vias + 1,
				}
				if !routableCell(occupancy, neighbor.Coord, allowedBlocked) {
					continue
				}
				tentative := currentNode.G + viaCostMM
				if existing, ok := bestCost[neighbor]; ok && !distanceLess(tentative, existing) {
					continue
				}
				bestCost[neighbor] = tentative
				cameFrom[neighbor] = current
				heap.Push(&open, &astarNode{
					State:    neighbor,
					G:        tentative,
					F:        tentative + heuristic.estimate(neighbor.Coord),
					Sequence: sequence,
				})
				sequence++
			}
		}
	}
	return nil, searchNodes, false, false
}

func accessCoordsOnLayer(access PadAccess, grid Grid, endpoint Endpoint, layerName string, layerIndex int) []GridCoord {
	points, ok := AccessPointsForEndpoint(access, endpoint)
	if !ok {
		return nil
	}
	coords := []GridCoord{}
	seen := map[GridCoord]struct{}{}
	for _, point := range points {
		if normalizeLayer(point.Layer) != layerName {
			continue
		}
		coord := grid.ToGrid(point.Point, layerIndex)
		if _, exists := seen[coord]; exists {
			continue
		}
		seen[coord] = struct{}{}
		coords = append(coords, coord)
	}
	sortGridCoords(coords)
	return coords
}

func accessCoordsOnLayers(access PadAccess, grid Grid, endpoint Endpoint, layerIndexes map[string]int, layerNames map[int]string) []GridCoord {
	points, ok := AccessPointsForEndpoint(access, endpoint)
	if !ok {
		return nil
	}
	coords := []GridCoord{}
	seen := map[GridCoord]struct{}{}
	for _, point := range points {
		layerIndex, ok := layerIndexes[normalizeLayer(point.Layer)]
		if !ok {
			continue
		}
		if _, allowed := layerNames[layerIndex]; !allowed {
			continue
		}
		coord := grid.ToGrid(point.Point, layerIndex)
		if _, exists := seen[coord]; exists {
			continue
		}
		seen[coord] = struct{}{}
		coords = append(coords, coord)
	}
	sortGridCoords(coords)
	return coords
}

func sortGridCoords(coords []GridCoord) {
	sort.Slice(coords, func(i int, j int) bool {
		if coords[i].Layer != coords[j].Layer {
			return coords[i].Layer < coords[j].Layer
		}
		if coords[i].X != coords[j].X {
			return coords[i].X < coords[j].X
		}
		return coords[i].Y < coords[j].Y
	})
}

func orthogonalNeighbors(state astarState) [4]astarState {
	x := state.Coord.X
	y := state.Coord.Y
	layerIndex := state.Coord.Layer
	return [4]astarState{
		{Coord: GridCoord{X: x + 1, Y: y, Layer: layerIndex}, Dir: routeDirEast, Vias: state.Vias},
		{Coord: GridCoord{X: x - 1, Y: y, Layer: layerIndex}, Dir: routeDirWest, Vias: state.Vias},
		{Coord: GridCoord{X: x, Y: y + 1, Layer: layerIndex}, Dir: routeDirSouth, Vias: state.Vias},
		{Coord: GridCoord{X: x, Y: y - 1, Layer: layerIndex}, Dir: routeDirNorth, Vias: state.Vias},
	}
}

func routableCell(occupancy Occupancy, coord GridCoord, allowedBlocked map[GridCoord]struct{}) bool {
	layer := occupancy.Layers[coord.Layer]
	if layer == nil {
		return false
	}
	point := gridPoint{X: coord.X, Y: coord.Y}
	if _, ok := layer.index(point); !ok {
		return false
	}
	if _, ok := allowedBlocked[coord]; ok {
		return true
	}
	return !occupancy.BlockedCell(coord)
}

func movementCost(fromDir int, toDir int, gridMM float64, turnPenaltyMM float64) float64 {
	cost := gridMM
	if fromDir != routeDirNone && fromDir != toDir {
		cost += turnPenaltyMM
	}
	return cost
}

type targetHeuristic struct {
	MinX   int
	MaxX   int
	MinY   int
	MaxY   int
	GridMM float64
}

func newTargetHeuristic(targets []GridCoord, gridMM float64) targetHeuristic {
	if len(targets) == 0 {
		return targetHeuristic{GridMM: normalizedGridStepMM(gridMM)}
	}
	heuristic := targetHeuristic{
		MinX:   targets[0].X,
		MaxX:   targets[0].X,
		MinY:   targets[0].Y,
		MaxY:   targets[0].Y,
		GridMM: gridMM,
	}
	for _, target := range targets[1:] {
		heuristic.MinX = min(heuristic.MinX, target.X)
		heuristic.MaxX = max(heuristic.MaxX, target.X)
		heuristic.MinY = min(heuristic.MinY, target.Y)
		heuristic.MaxY = max(heuristic.MaxY, target.Y)
	}
	heuristic.GridMM = normalizedGridStepMM(heuristic.GridMM)
	return heuristic
}

func normalizedGridStepMM(gridMM float64) float64 {
	if gridMM <= 0 || math.IsNaN(gridMM) || math.IsInf(gridMM, 0) {
		return DefaultRules().GridMM
	}
	return gridMM
}

func (heuristic targetHeuristic) estimate(coord GridCoord) float64 {
	dx := 0
	if coord.X < heuristic.MinX {
		dx = heuristic.MinX - coord.X
	} else if coord.X > heuristic.MaxX {
		dx = coord.X - heuristic.MaxX
	}
	dy := 0
	if coord.Y < heuristic.MinY {
		dy = heuristic.MinY - coord.Y
	} else if coord.Y > heuristic.MaxY {
		dy = coord.Y - heuristic.MaxY
	}
	return float64(dx+dy) * heuristic.GridMM
}

func reconstructGridPath(current astarState, cameFrom map[astarState]astarState) []GridCoord {
	path := []GridCoord{current.Coord}
	for {
		previous, ok := cameFrom[current]
		if !ok {
			break
		}
		current = previous
		path = append(path, current.Coord)
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

func routeFailureIssue(netName string, pair EndpointPair, message string) reports.Issue {
	return routeFailureIssueWithObstacle(netName, pair, message, "")
}

func routeFailureIssueWithObstacle(netName string, pair EndpointPair, message string, obstacleSummary string) reports.Issue {
	if obstacleSummary != "" {
		message += ": blocked near " + obstacleSummary
	}
	return reports.Issue{
		Code:       reports.CodeValidationFailed,
		Severity:   reports.SeverityBlocked,
		Path:       "nets." + netName,
		Message:    message,
		Refs:       []string{pair.From.Ref, pair.To.Ref},
		Nets:       []string{netName},
		Suggestion: "move components, reduce clearance, or allow another routing layer",
	}
}

func nearestObstacleSummary(occupancy Occupancy, coordSets ...[]GridCoord) string {
	for _, coords := range coordSets {
		for _, coord := range coords {
			if obstacle, ok := occupancy.FirstObstacle(coord); ok {
				return obstacleSummary(obstacle)
			}
			for _, neighbor := range orthogonalNeighbors(astarState{Coord: coord}) {
				if obstacle, ok := occupancy.FirstObstacle(neighbor.Coord); ok {
					return obstacleSummary(obstacle)
				}
			}
		}
	}
	return ""
}

func obstacleSummary(obstacle Obstacle) string {
	if obstacle.Source != "" {
		return string(obstacle.Kind) + " " + obstacle.Source
	}
	return string(obstacle.Kind)
}

func normalizedSearchLayers(layers []Layer) []Layer {
	if len(layers) != 0 {
		return layers
	}
	return []Layer{
		{Name: "F.Cu", Kind: LayerCopper, Routable: true},
		{Name: "B.Cu", Kind: LayerCopper, Routable: true},
	}
}

func normalizedSearchRules(rules Rules) Rules {
	defaults := DefaultRules()
	if rules.MaxSearchNodes == 0 {
		rules.MaxSearchNodes = defaults.MaxSearchNodes
	}
	if rules.MaxViasPerNet == 0 {
		rules.MaxViasPerNet = defaults.MaxViasPerNet
	}
	if rules.ViaDiameterMM == 0 {
		rules.ViaDiameterMM = defaults.ViaDiameterMM
	}
	if rules.ViaDrillMM == 0 {
		rules.ViaDrillMM = defaults.ViaDrillMM
	}
	return rules
}

func filterAllowedLayers(routable []string, allowed []string) []string {
	allowedSet := map[string]struct{}{}
	for _, layer := range allowed {
		allowedSet[normalizeLayer(layer)] = struct{}{}
	}
	filtered := make([]string, 0, len(routable))
	for _, layer := range routable {
		if _, ok := allowedSet[normalizeLayer(layer)]; ok {
			filtered = append(filtered, layer)
		}
	}
	return filtered
}

func (queue astarQueue) Len() int {
	return len(queue)
}

func (queue astarQueue) Less(i int, j int) bool {
	if !distanceEqual(queue[i].F, queue[j].F) {
		return queue[i].F < queue[j].F
	}
	if !distanceEqual(queue[i].G, queue[j].G) {
		return queue[i].G > queue[j].G
	}
	if queue[i].State.Coord.X != queue[j].State.Coord.X {
		return queue[i].State.Coord.X < queue[j].State.Coord.X
	}
	if queue[i].State.Coord.Y != queue[j].State.Coord.Y {
		return queue[i].State.Coord.Y < queue[j].State.Coord.Y
	}
	if queue[i].State.Dir != queue[j].State.Dir {
		return queue[i].State.Dir < queue[j].State.Dir
	}
	return queue[i].Sequence < queue[j].Sequence
}

func (queue astarQueue) Swap(i int, j int) {
	queue[i], queue[j] = queue[j], queue[i]
}

func (queue *astarQueue) Push(value any) {
	node := value.(*astarNode)
	*queue = append(*queue, node)
}

func (queue *astarQueue) Pop() any {
	old := *queue
	last := old[len(old)-1]
	old[len(old)-1] = nil
	*queue = old[:len(old)-1]
	return last
}
