package designworkflow

import (
	"math"
	"slices"
	"strings"

	"kicadai/internal/transactions"
)

type InterBlockRouteTree struct {
	NetName               string                      `json:"net_name"`
	RootEndpointID        string                      `json:"root_endpoint_id"`
	RequiredEndpointCount int                         `json:"required_endpoint_count"`
	TargetCount           int                         `json:"target_count"`
	Branches              []InterBlockRouteTreeBranch `json:"branches,omitempty"`
	MissingEndpointIDs    []string                    `json:"missing_endpoint_ids,omitempty"`
}

type InterBlockRouteTreeBranch struct {
	Index             int     `json:"index"`
	StartEndpointID   string  `json:"start_endpoint_id"`
	EndEndpointID     string  `json:"end_endpoint_id"`
	PlannedDistanceMM float64 `json:"planned_distance_mm"`
}

type routeTreeEndpoint struct {
	ID     string
	Ref    string
	Pin    string
	Point  transactions.Point
	Layer  string
	Target InterBlockContactTarget
}

func BuildInterBlockRouteTrees(groups []InterBlockRouteGroup, evidence InterBlockContactEvidence) []InterBlockRouteTree {
	targetsByNetEndpoint := interBlockContactTargetsByNetEndpoint(evidence.Targets)
	trees := make([]InterBlockRouteTree, 0, len(groups))
	for _, group := range groups {
		tree := BuildInterBlockRouteTree(group, targetsByNetEndpoint[routeTreeNetKey(group.NetName)])
		trees = append(trees, tree)
	}
	return trees
}

func BuildInterBlockRouteTree(group InterBlockRouteGroup, targets map[string]InterBlockContactTarget) InterBlockRouteTree {
	endpoints, missing := routeTreeEndpoints(group, targets)
	tree := InterBlockRouteTree{
		NetName:               group.NetName,
		RequiredEndpointCount: len(group.RequiredEndpoints),
		TargetCount:           len(endpoints),
		MissingEndpointIDs:    missing,
	}
	if len(endpoints) == 0 {
		return tree
	}
	rootIndex := selectRouteTreeRoot(endpoints)
	root := endpoints[rootIndex]
	tree.RootEndpointID = root.ID
	tree.Branches = planRouteTreeBranches(endpoints, rootIndex)
	return tree
}

func routeTreeEndpoints(group InterBlockRouteGroup, targets map[string]InterBlockContactTarget) ([]routeTreeEndpoint, []string) {
	var endpoints []routeTreeEndpoint
	var missing []string
	for _, endpoint := range group.RequiredEndpoints {
		key := routeTreeEndpointKey(endpoint.Ref, endpoint.Pin)
		target, ok := targets[key]
		if !ok {
			missing = append(missing, endpoint.ID)
			continue
		}
		endpoints = append(endpoints, routeTreeEndpoint{
			ID:     endpoint.ID,
			Ref:    endpoint.Ref,
			Pin:    endpoint.Pin,
			Point:  target.Point,
			Layer:  target.Layer,
			Target: target,
		})
	}
	slices.Sort(missing)
	slices.SortFunc(endpoints, func(left, right routeTreeEndpoint) int {
		return strings.Compare(left.ID, right.ID)
	})
	return endpoints, missing
}

func selectRouteTreeRoot(endpoints []routeTreeEndpoint) int {
	if len(endpoints) == 0 {
		return -1
	}
	bestIndex := 0
	bestScore := routeTreeRootScore(endpoints, 0)
	for index := 1; index < len(endpoints); index++ {
		score := routeTreeRootScore(endpoints, index)
		if score.Less(bestScore) {
			bestIndex = index
			bestScore = score
		}
	}
	return bestIndex
}

type routeTreeRootRank struct {
	ConnectorRank int
	DistanceSumMM float64
	ID            string
}

func (score routeTreeRootRank) Less(other routeTreeRootRank) bool {
	if score.ConnectorRank != other.ConnectorRank {
		return score.ConnectorRank < other.ConnectorRank
	}
	if math.Abs(score.DistanceSumMM-other.DistanceSumMM) > 1e-9 {
		return score.DistanceSumMM < other.DistanceSumMM
	}
	return score.ID < other.ID
}

func routeTreeRootScore(endpoints []routeTreeEndpoint, index int) routeTreeRootRank {
	endpoint := endpoints[index]
	score := routeTreeRootRank{ConnectorRank: 1, ID: endpoint.ID}
	if routeTreeEndpointLooksLikeConnector(endpoint) {
		score.ConnectorRank = 0
	}
	for candidateIndex := range endpoints {
		if candidateIndex == index {
			continue
		}
		score.DistanceSumMM += manhattanDistance(endpoint.Point, endpoints[candidateIndex].Point)
	}
	return score
}

func routeTreeEndpointLooksLikeConnector(endpoint routeTreeEndpoint) bool {
	ref := strings.ToUpper(strings.TrimSpace(endpoint.Ref))
	return strings.HasPrefix(ref, "J") || strings.HasPrefix(ref, "P") || strings.HasPrefix(ref, "CONN")
}

func planRouteTreeBranches(endpoints []routeTreeEndpoint, rootIndex int) []InterBlockRouteTreeBranch {
	if rootIndex < 0 || rootIndex >= len(endpoints) || len(endpoints) < 2 {
		return nil
	}
	connected := map[int]bool{rootIndex: true}
	bestStartByEnd := make([]int, len(endpoints))
	bestDistanceByEnd := make([]float64, len(endpoints))
	for index := range endpoints {
		bestStartByEnd[index] = -1
		bestDistanceByEnd[index] = math.Inf(1)
	}
	updateRouteTreeBestConnections(endpoints, connected, rootIndex, bestStartByEnd, bestDistanceByEnd)
	branches := make([]InterBlockRouteTreeBranch, 0, len(endpoints)-1)
	for len(connected) < len(endpoints) {
		startIndex, endIndex := nearestRouteTreeBranch(endpoints, connected, bestStartByEnd, bestDistanceByEnd)
		if startIndex < 0 || endIndex < 0 {
			break
		}
		branch := InterBlockRouteTreeBranch{
			Index:             len(branches),
			StartEndpointID:   endpoints[startIndex].ID,
			EndEndpointID:     endpoints[endIndex].ID,
			PlannedDistanceMM: manhattanDistance(endpoints[startIndex].Point, endpoints[endIndex].Point),
		}
		branches = append(branches, branch)
		connected[endIndex] = true
		updateRouteTreeBestConnections(endpoints, connected, endIndex, bestStartByEnd, bestDistanceByEnd)
	}
	return branches
}

func updateRouteTreeBestConnections(endpoints []routeTreeEndpoint, connected map[int]bool, startIndex int, bestStartByEnd []int, bestDistanceByEnd []float64) {
	for endIndex := range endpoints {
		if connected[endIndex] {
			continue
		}
		distance := manhattanDistance(endpoints[startIndex].Point, endpoints[endIndex].Point)
		if routeTreeBranchLess(distance, endpoints[startIndex].ID, endpoints[endIndex].ID, bestDistanceByEnd[endIndex], endpointID(endpoints, bestStartByEnd[endIndex]), endpoints[endIndex].ID) {
			bestStartByEnd[endIndex] = startIndex
			bestDistanceByEnd[endIndex] = distance
		}
	}
}

func nearestRouteTreeBranch(endpoints []routeTreeEndpoint, connected map[int]bool, bestStartByEnd []int, bestDistanceByEnd []float64) (int, int) {
	selectedStart := -1
	selectedEnd := -1
	selectedDistance := math.Inf(1)
	for endIndex := range endpoints {
		if connected[endIndex] || bestStartByEnd[endIndex] < 0 {
			continue
		}
		startIndex := bestStartByEnd[endIndex]
		distance := bestDistanceByEnd[endIndex]
		if routeTreeBranchLess(distance, endpoints[startIndex].ID, endpoints[endIndex].ID, selectedDistance, endpointID(endpoints, selectedStart), endpointID(endpoints, selectedEnd)) {
			selectedStart = startIndex
			selectedEnd = endIndex
			selectedDistance = distance
		}
	}
	return selectedStart, selectedEnd
}

// routeTreeBranchLess orders candidate branches by shorter Manhattan distance,
// then stable start endpoint ID, then stable end endpoint ID.
func routeTreeBranchLess(distance float64, startID string, endID string, bestDistance float64, bestStartID string, bestEndID string) bool {
	if math.Abs(distance-bestDistance) > 1e-9 {
		return distance < bestDistance
	}
	if startID != bestStartID {
		return startID < bestStartID
	}
	return endID < bestEndID
}

func endpointID(endpoints []routeTreeEndpoint, index int) string {
	if index < 0 || index >= len(endpoints) {
		return ""
	}
	return endpoints[index].ID
}

func interBlockContactTargetsByNetEndpoint(targets []InterBlockContactTarget) map[string]map[string]InterBlockContactTarget {
	byNet := map[string]map[string]InterBlockContactTarget{}
	ordered := routeTreeTargetEntries(targets)
	slices.SortFunc(ordered, compareRouteTreeTargetEntry)
	for _, entry := range ordered {
		if entry.NetKey == "" {
			continue
		}
		if entry.EndpointKey == "." {
			continue
		}
		if byNet[entry.NetKey] == nil {
			byNet[entry.NetKey] = map[string]InterBlockContactTarget{}
		}
		if _, exists := byNet[entry.NetKey][entry.EndpointKey]; !exists {
			byNet[entry.NetKey][entry.EndpointKey] = entry.Target
		}
	}
	return byNet
}

type routeTreeTargetEntry struct {
	Target         InterBlockContactTarget
	NetKey         string
	EndpointKey    string
	ConfidenceRank int
	Layer          string
	XMicron        int64
	YMicron        int64
	GeometrySource string
}

func routeTreeTargetEntries(targets []InterBlockContactTarget) []routeTreeTargetEntry {
	entries := make([]routeTreeTargetEntry, 0, len(targets))
	for _, target := range targets {
		entries = append(entries, routeTreeTargetEntry{
			Target:         target,
			NetKey:         routeTreeNetKey(target.NetName),
			EndpointKey:    routeTreeEndpointKey(target.Ref, target.Pad),
			ConfidenceRank: interBlockContactConfidenceRank(target.Confidence),
			Layer:          target.Layer,
			XMicron:        routeTreeCoordinateMicron(target.Point.XMM),
			YMicron:        routeTreeCoordinateMicron(target.Point.YMM),
			GeometrySource: target.GeometrySource,
		})
	}
	return entries
}

func compareRouteTreeTargetEntry(left, right routeTreeTargetEntry) int {
	if cmp := strings.Compare(left.NetKey, right.NetKey); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(left.EndpointKey, right.EndpointKey); cmp != 0 {
		return cmp
	}
	if confidenceCmp := right.ConfidenceRank - left.ConfidenceRank; confidenceCmp != 0 {
		return confidenceCmp
	}
	if cmp := strings.Compare(left.Layer, right.Layer); cmp != 0 {
		return cmp
	}
	if left.XMicron < right.XMicron {
		return -1
	}
	if left.XMicron > right.XMicron {
		return 1
	}
	if left.YMicron < right.YMicron {
		return -1
	}
	if left.YMicron > right.YMicron {
		return 1
	}
	return strings.Compare(left.GeometrySource, right.GeometrySource)
}

func routeTreeCoordinateMicron(mm float64) int64 {
	return int64(math.Round(mm * 1000))
}

func interBlockContactConfidenceRank(confidence InterBlockContactConfidence) int {
	switch confidence {
	case InterBlockContactConfidenceHigh:
		return 3
	case InterBlockContactConfidenceMedium:
		return 2
	case InterBlockContactConfidenceBlocked:
		return 1
	default:
		return 0
	}
}

func routeTreeNetKey(netName string) string {
	// KiCad preserves net-name case, so route-tree target lookup must not merge
	// distinct nets such as VCC and vcc.
	return strings.TrimSpace(netName)
}

func routeTreeEndpointKey(ref string, pin string) string {
	return normalizedRouteGroupEndpointKey(ref, pin)
}

func manhattanDistance(left transactions.Point, right transactions.Point) float64 {
	return math.Abs(left.XMM-right.XMM) + math.Abs(left.YMM-right.YMM)
}
