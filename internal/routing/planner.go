package routing

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/reports"
)

const (
	// A fourfold adjacent tree-distance jump separates a local escape cluster
	// from board-spanning work without depending on board units or coordinates.
	// The prefix cap bounds how much copper can be committed before established
	// power/ground ordering resumes.
	localEscapeGapRatio     = 4.0
	maxLocalRoutePrefixNets = 4
	maxDenseFanoutBranches  = 12
	denseLocalTotalGapRatio = 8.0
)

type PlannedNet struct {
	Net   Net
	Pairs []EndpointPair
}

type EndpointPair struct {
	From Endpoint
	To   Endpoint
}

func PlanRoutes(request Request, access PadAccess) ([]PlannedNet, []reports.Issue) {
	request = cloneRequest(request)
	NormalizeRequest(&request)
	issues := []reports.Issue{}
	nets := append([]Net(nil), request.Nets...)
	sort.SliceStable(nets, func(i int, j int) bool {
		return netLess(nets[i], nets[j])
	})
	plans := []PlannedNet{}
	for index, net := range nets {
		if strings.TrimSpace(net.Name) == "" {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityInfo,
				Path:     "nets[" + strconv.Itoa(index) + "].name",
				Message:  "unnamed net was skipped",
			})
			continue
		}
		if len(net.Endpoints) < 2 {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityInfo,
				Path:     "nets." + net.Name,
				Message:  "net has fewer than two endpoints and was skipped",
				Nets:     []string{net.Name},
			})
			continue
		}
		if net.Fixed {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeFixedNetSkipped,
				Severity: reports.SeverityInfo,
				Path:     "nets." + net.Name,
				Message:  "fixed net was preserved and skipped",
				Nets:     []string{net.Name},
			})
			continue
		}
		pairs, pairIssues := planEndpointPairs(net.Name, net.Endpoints, access)
		issues = append(issues, pairIssues...)
		if len(pairIssues) != 0 {
			continue
		}
		plans = append(plans, PlannedNet{Net: net, Pairs: pairs})
	}
	// Within each explicit priority, reserve a compact local escape cluster
	// before board-spanning nets add copper obstacles around it. A cluster is
	// recognized only when canonical endpoint-tree distances contain a clear
	// multiplicative gap; otherwise the established role/endpoint order is
	// preserved. This keeps the rule bounded, generic, and deterministic.
	advancedOrder := request.Strategy.NetOrder == NetOrderConstrainedEndpointAccessV1
	escapeNets, distances, accessSpans := compactEscapeNets(plans, access, advancedOrder)
	distanceFloors := plannedDistanceFloors(plans, distances)
	meanDistanceFloors := plannedMeanDistanceFloors(plans, distances)
	sort.SliceStable(plans, func(i, j int) bool {
		if plans[i].Net.Priority != plans[j].Net.Priority {
			return plans[i].Net.Priority > plans[j].Net.Priority
		}
		if advancedOrder {
			leftDenseFanout := denseConstrainedFanout(plans[i], access, request.Rules, distances, distanceFloors, meanDistanceFloors)
			rightDenseFanout := denseConstrainedFanout(plans[j], access, request.Rules, distances, distanceFloors, meanDistanceFloors)
			if leftDenseFanout != rightDenseFanout {
				return leftDenseFanout
			}
			if leftDenseFanout {
				return netLess(plans[i].Net, plans[j].Net)
			}
			leftSpan := accessSpans[plans[i].Net.Name]
			rightSpan := accessSpans[plans[j].Net.Name]
			if !distanceEqual(leftSpan, rightSpan) {
				return distanceLess(leftSpan, rightSpan)
			}
			leftConstrained := endpointAccessConstrained(plans[i], accessSpans, request.Rules)
			rightConstrained := endpointAccessConstrained(plans[j], accessSpans, request.Rules)
			leftScheduleRank := routingScheduleRank(plans[i], leftConstrained)
			rightScheduleRank := routingScheduleRank(plans[j], rightConstrained)
			if leftScheduleRank != rightScheduleRank {
				return leftScheduleRank < rightScheduleRank
			}
			if leftConstrained {
				if constrainedRoleRank(plans[i].Net.Role) != constrainedRoleRank(plans[j].Net.Role) {
					return constrainedRoleRank(plans[i].Net.Role) < constrainedRoleRank(plans[j].Net.Role)
				}
				if len(plans[i].Net.Endpoints) != len(plans[j].Net.Endpoints) {
					return len(plans[i].Net.Endpoints) < len(plans[j].Net.Endpoints)
				}
				if !distanceEqual(distances[plans[i].Net.Name], distances[plans[j].Net.Name]) {
					return distanceLess(distances[plans[j].Net.Name], distances[plans[i].Net.Name])
				}
			}
		}
		leftEscape := escapeNets[plans[i].Net.Priority][plans[i].Net.Name]
		rightEscape := escapeNets[plans[j].Net.Priority][plans[j].Net.Name]
		if leftEscape != rightEscape {
			return leftEscape
		}
		if leftEscape && advancedOrder {
			leftSpan := accessSpans[plans[i].Net.Name]
			rightSpan := accessSpans[plans[j].Net.Name]
			if !distanceEqual(leftSpan, rightSpan) {
				return distanceLess(leftSpan, rightSpan)
			}
			if !distanceEqual(distances[plans[i].Net.Name], distances[plans[j].Net.Name]) {
				return distanceLess(distances[plans[i].Net.Name], distances[plans[j].Net.Name])
			}
		} else if leftEscape && !distanceEqual(distances[plans[i].Net.Name], distances[plans[j].Net.Name]) {
			return distanceLess(distances[plans[i].Net.Name], distances[plans[j].Net.Name])
		}
		return netLess(plans[i].Net, plans[j].Net)
	})
	return plans, issues
}

func routingScheduleRank(plan PlannedNet, constrained bool) int {
	if constrained {
		return 0
	}
	if plan.Net.Role == NetPower || plan.Net.Role == NetGround {
		return 1
	}
	return 2
}

func plannedDistanceFloors(plans []PlannedNet, distances map[string]float64) map[int]float64 {
	floors := map[int]float64{}
	for _, plan := range plans {
		distance := distances[plan.Net.Name]
		if distance <= distanceEpsilon || math.IsInf(distance, 0) {
			continue
		}
		floor, exists := floors[plan.Net.Priority]
		if !exists || distanceLess(distance, floor) {
			floors[plan.Net.Priority] = distance
		}
	}
	return floors
}

func plannedMeanDistanceFloors(plans []PlannedNet, distances map[string]float64) map[int]float64 {
	floors := map[int]float64{}
	for _, plan := range plans {
		distance := plannedMeanDistance(plan, distances)
		if distance <= distanceEpsilon || math.IsInf(distance, 0) {
			continue
		}
		floor, exists := floors[plan.Net.Priority]
		if !exists || distanceLess(distance, floor) {
			floors[plan.Net.Priority] = distance
		}
	}
	return floors
}

func denseConstrainedFanout(plan PlannedNet, access PadAccess, rules Rules, distances map[string]float64, totalFloors, meanFloors map[int]float64) bool {
	if len(plan.Net.Endpoints) <= 3 || len(plan.Pairs) > maxDenseFanoutBranches || rules.GridMM <= 0 {
		return false
	}
	totalDistance := distances[plan.Net.Name]
	totalFloor := totalFloors[plan.Net.Priority]
	meanDistance := plannedMeanDistance(plan, distances)
	meanFloor := meanFloors[plan.Net.Priority]
	if totalFloor <= distanceEpsilon || meanFloor <= distanceEpsilon ||
		math.IsInf(totalDistance, 0) || math.IsInf(meanDistance, 0) ||
		totalDistance > denseLocalTotalGapRatio*totalFloor+distanceEpsilon ||
		meanDistance > localEscapeGapRatio*meanFloor+distanceEpsilon {
		return false
	}
	accessPitch := 2*rules.GridMM + rules.TraceWidthMM
	constrained := 0
	for _, endpoint := range plan.Net.Endpoints {
		span := endpointPadSpan(endpoint, access)
		if span > 0 && !math.IsInf(span, 0) && span <= accessPitch+distanceEpsilon {
			constrained++
		}
	}
	return constrained >= 2
}

func plannedMeanDistance(plan PlannedNet, distances map[string]float64) float64 {
	distance := distances[plan.Net.Name]
	if len(plan.Pairs) == 0 || math.IsInf(distance, 0) {
		return distance
	}
	return distance / float64(len(plan.Pairs))
}

func endpointAccessConstrained(plan PlannedNet, accessSpans map[string]float64, rules Rules) bool {
	// Bound the escape promotion to small fanout nets. This includes a signal
	// with one local bias component while excluding broad power/ground trees.
	if len(plan.Net.Endpoints) < 2 || len(plan.Net.Endpoints) > 3 || rules.GridMM <= 0 {
		return false
	}
	span := accessSpans[plan.Net.Name]
	accessPitch := 2*rules.GridMM + rules.TraceWidthMM
	return span > 0 && !math.IsInf(span, 0) && span <= accessPitch+distanceEpsilon
}

func constrainedRoleRank(role NetRole) int {
	if role == NetSignal {
		return 0
	}
	return 1
}

func compactEscapeNets(plans []PlannedNet, access PadAccess, preferNarrow bool) (map[int]map[string]bool, map[string]float64, map[string]float64) {
	type candidate struct {
		name     string
		role     NetRole
		distance float64
		span     float64
	}
	byPriority := map[int][]candidate{}
	distances := map[string]float64{}
	accessSpans := map[string]float64{}
	for _, plan := range plans {
		distance := plannedNetDistance(plan, access)
		span := plannedNetMinimumPadSpan(plan, access)
		distances[plan.Net.Name] = distance
		accessSpans[plan.Net.Name] = span
		byPriority[plan.Net.Priority] = append(byPriority[plan.Net.Priority], candidate{name: plan.Net.Name, role: plan.Net.Role, distance: distance, span: span})
	}
	escape := map[int]map[string]bool{}
	for priority, candidates := range byPriority {
		sort.SliceStable(candidates, func(i, j int) bool {
			if !distanceEqual(candidates[i].distance, candidates[j].distance) {
				return distanceLess(candidates[i].distance, candidates[j].distance)
			}
			return candidates[i].name < candidates[j].name
		})
		bestGap := localEscapeGapRatio
		split := -1
		for index := 0; index+1 < len(candidates); index++ {
			left := candidates[index].distance
			right := candidates[index+1].distance
			ratio := math.Inf(1)
			if left > distanceEpsilon {
				ratio = right / left
			}
			if ratio > bestGap {
				bestGap = ratio
				split = index
			}
		}
		if split < 0 {
			continue
		}
		escape[priority] = map[string]bool{}
		powerNets := 0
		for index := 0; index <= split; index++ {
			if candidates[index].role == NetPower {
				powerNets++
			}
		}
		escapeBudget := maxLocalRoutePrefixNets - powerNets
		if escapeBudget < 0 {
			escapeBudget = 0
		}
		eligible := append([]candidate(nil), candidates[:split+1]...)
		if preferNarrow {
			sort.SliceStable(eligible, func(i, j int) bool {
				if !distanceEqual(eligible[i].span, eligible[j].span) {
					return distanceLess(eligible[i].span, eligible[j].span)
				}
				if !distanceEqual(eligible[i].distance, eligible[j].distance) {
					return distanceLess(eligible[i].distance, eligible[j].distance)
				}
				return eligible[i].name < eligible[j].name
			})
		}
		count := 0
		for _, candidate := range eligible {
			if count >= escapeBudget {
				break
			}
			if candidate.role == NetPower || candidate.role == NetGround {
				continue
			}
			escape[priority][candidate.name] = true
			count++
		}
	}
	return escape, distances, accessSpans
}

func plannedNetMinimumPadSpan(plan PlannedNet, access PadAccess) float64 {
	span := math.Inf(1)
	for _, endpoint := range plan.Net.Endpoints {
		pad, ok := access.Pads[endpointKey(normalizeKey(endpoint.Ref), normalizeKey(endpoint.Pin))]
		if !ok {
			continue
		}
		candidate := math.Min(pad.Size.WidthMM, pad.Size.HeightMM)
		if candidate > 0 && candidate < span {
			span = candidate
		}
	}
	return span
}

func plannedNetDistance(plan PlannedNet, access PadAccess) float64 {
	distance := 0.0
	for _, pair := range plan.Pairs {
		from, fromOK := AccessPointsForEndpoint(access, pair.From)
		to, toOK := AccessPointsForEndpoint(access, pair.To)
		if !fromOK || !toOK {
			return math.Inf(1)
		}
		distance += endpointDistance(from, to)
	}
	return distance
}

func netLess(left Net, right Net) bool {
	if left.Priority != right.Priority {
		return left.Priority > right.Priority
	}
	if roleRank(left.Role) != roleRank(right.Role) {
		return roleRank(left.Role) < roleRank(right.Role)
	}
	if len(left.Endpoints) != len(right.Endpoints) {
		return len(left.Endpoints) < len(right.Endpoints)
	}
	return left.Name < right.Name
}

func roleRank(role NetRole) int {
	switch role {
	case NetPower, NetGround:
		return 0
	case NetSignal:
		return 1
	default:
		return 2
	}
}

func planEndpointPairs(netName string, endpoints []Endpoint, access PadAccess) ([]EndpointPair, []reports.Issue) {
	ordered := append([]Endpoint(nil), endpoints...)
	sort.SliceStable(ordered, func(i int, j int) bool {
		return endpointLess(ordered[i], ordered[j])
	})
	if len(ordered) < 2 {
		return nil, nil
	}
	accessPoints := make([][]AccessPoint, len(ordered))
	issues := []reports.Issue{}
	for index, endpoint := range ordered {
		points, ok := AccessPointsForEndpoint(access, endpoint)
		if !ok {
			issues = append(issues, unreachableEndpointIssue(netName, endpoint))
			continue
		}
		accessPoints[index] = points
	}
	if len(issues) != 0 {
		sortIssues(issues)
		return nil, issues
	}
	inTree := make([]bool, len(ordered))
	bestFrom := make([]int, len(ordered))
	bestDistances := make([]float64, len(ordered))
	for index := range bestDistances {
		bestFrom[index] = 0
		bestDistances[index] = math.Inf(1)
	}
	inTree[0] = true
	for index := 1; index < len(ordered); index++ {
		bestDistances[index] = endpointDistance(accessPoints[0], accessPoints[index])
	}
	pairs := make([]EndpointPair, 0, len(ordered)-1)
	for len(pairs) < len(ordered)-1 {
		bestIndex := -1
		bestDistance := math.Inf(1)
		for index := range ordered {
			if inTree[index] {
				continue
			}
			pair := EndpointPair{From: ordered[bestFrom[index]], To: ordered[index]}
			if bestIndex == -1 ||
				distanceLess(bestDistances[index], bestDistance) ||
				(distanceEqual(bestDistances[index], bestDistance) && endpointPairLess(pair, EndpointPair{From: ordered[bestFrom[bestIndex]], To: ordered[bestIndex]})) {
				bestDistance = bestDistances[index]
				bestIndex = index
			}
		}
		pair := EndpointPair{From: ordered[bestFrom[bestIndex]], To: ordered[bestIndex]}
		pairs = append(pairs, pair)
		inTree[bestIndex] = true
		for index := range ordered {
			if inTree[index] {
				continue
			}
			distance := endpointDistance(accessPoints[bestIndex], accessPoints[index])
			candidate := EndpointPair{From: ordered[bestIndex], To: ordered[index]}
			current := EndpointPair{From: ordered[bestFrom[index]], To: ordered[index]}
			if distanceLess(distance, bestDistances[index]) || (distanceEqual(distance, bestDistances[index]) && endpointPairLess(candidate, current)) {
				bestDistances[index] = distance
				bestFrom[index] = bestIndex
			}
		}
	}
	return pairs, nil
}

func endpointPadSpan(endpoint Endpoint, access PadAccess) float64 {
	pad, ok := access.Pads[endpointKey(normalizeKey(endpoint.Ref), normalizeKey(endpoint.Pin))]
	if !ok {
		return math.Inf(1)
	}
	span := math.Min(pad.Size.WidthMM, pad.Size.HeightMM)
	if span <= 0 {
		return math.Inf(1)
	}
	return span
}

func unreachableEndpointIssue(netName string, endpoint Endpoint) reports.Issue {
	return reports.Issue{
		Code:       reports.CodeValidationFailed,
		Severity:   reports.SeverityBlocked,
		Path:       "nets." + netName + ".endpoints." + endpoint.Ref + "." + endpoint.Pin,
		Message:    "endpoint has no reachable routing access point",
		Refs:       []string{endpoint.Ref},
		Nets:       []string{netName},
		Suggestion: "verify footprint pad geometry, pad layers, and placement before routing",
	}
}

func endpointDistance(leftPoints []AccessPoint, rightPoints []AccessPoint) float64 {
	best := math.Inf(1)
	for _, leftPoint := range leftPoints {
		for _, rightPoint := range rightPoints {
			dx := leftPoint.Point.XMM - rightPoint.Point.XMM
			dy := leftPoint.Point.YMM - rightPoint.Point.YMM
			best = math.Min(best, dx*dx+dy*dy)
		}
	}
	return best
}

const distanceEpsilon = 1e-12

func distanceLess(left float64, right float64) bool {
	if math.IsInf(left, 1) && math.IsInf(right, 1) {
		return false
	}
	if math.IsInf(left, 1) {
		return false
	}
	if math.IsInf(right, 1) {
		return true
	}
	return left < right-distanceEpsilon
}

func distanceEqual(left float64, right float64) bool {
	if math.IsInf(left, 1) && math.IsInf(right, 1) {
		return true
	}
	if math.IsInf(left, 1) || math.IsInf(right, 1) {
		return false
	}
	return math.Abs(left-right) <= distanceEpsilon
}

func endpointPairLess(left EndpointPair, right EndpointPair) bool {
	leftFrom := endpointKey(left.From.Ref, left.From.Pin)
	rightFrom := endpointKey(right.From.Ref, right.From.Pin)
	if leftFrom != rightFrom {
		return endpointIDLess(leftFrom, rightFrom)
	}
	return endpointIDLess(endpointKey(left.To.Ref, left.To.Pin), endpointKey(right.To.Ref, right.To.Pin))
}

func endpointLess(left Endpoint, right Endpoint) bool {
	return endpointIDLess(endpointKey(left.Ref, left.Pin), endpointKey(right.Ref, right.Pin))
}

func endpointIDLess(left endpointID, right endpointID) bool {
	if left.Ref != right.Ref {
		return left.Ref < right.Ref
	}
	return left.Pin < right.Pin
}
