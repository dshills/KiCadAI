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
	advancedOrder := request.Strategy.NetOrder == NetOrderConstrainedEndpointAccessV1
	hasInternalRoutingLayers := len(routableLayerNames(request.Board.Layers)) > 2
	usesFanoutPressureOrder := false
	if hasInternalRoutingLayers {
		powerNets := 0
		for _, net := range request.Nets {
			if net.Role == NetPower {
				powerNets++
			}
			if net.Role == NetSignal && len(net.Endpoints) > 3 {
				usesFanoutPressureOrder = true
			}
		}
		usesFanoutPressureOrder = usesFanoutPressureOrder || powerNets > 1
	}
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
		pairs, pairIssues := planEndpointPairs(net.Name, net.Role, net.Endpoints, access, request.Rules, usesFanoutPressureOrder)
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
	escapeNets, distances, accessSpans := compactEscapeNets(plans, access, advancedOrder)
	distanceFloors := plannedDistanceFloors(plans, distances)
	meanDistanceFloors := plannedMeanDistanceFloors(plans, distances)
	edgeDistances := plannedConstrainedEndpointEdgeDistances(plans, access, request.Board, request.Rules)
	denseBundleRefs := plannedDenseSignalBundleRefs(plans, access, request.Rules)
	sort.SliceStable(plans, func(i, j int) bool {
		if plans[i].Net.OrderFirst != plans[j].Net.OrderFirst {
			// A bounded failed-net retry is stronger evidence than the baseline
			// schedule heuristic: it records an observed routing failure, not a
			// predicted one. Let that explicit promotion lead the next attempt.
			return plans[i].Net.OrderFirst
		}
		if advancedOrder {
			leftConstrained := endpointAccessConstrained(plans[i], accessSpans, request.Rules, usesFanoutPressureOrder)
			rightConstrained := endpointAccessConstrained(plans[j], accessSpans, request.Rules, usesFanoutPressureOrder)
			leftScheduleRank := routingScheduleRank(plans[i], leftConstrained)
			rightScheduleRank := routingScheduleRank(plans[j], rightConstrained)
			if leftScheduleRank != rightScheduleRank {
				// A declared net priority describes electrical importance, but it
				// cannot create routing space after a physically constrained pad
				// escape has been sealed. The explicit constrained-access policy
				// therefore reserves scarce endpoint access before applying priority
				// within each physical schedule class.
				return leftScheduleRank < rightScheduleRank
			}
		}
		if plans[i].Net.Priority != plans[j].Net.Priority {
			return plans[i].Net.Priority > plans[j].Net.Priority
		}
		if advancedOrder {
			leftDenseBundle := denseSignalBundle(plans[i], denseBundleRefs)
			rightDenseBundle := denseSignalBundle(plans[j], denseBundleRefs)
			if leftDenseBundle != rightDenseBundle {
				return leftDenseBundle
			}
			if leftDenseBundle && !distanceEqual(distances[plans[i].Net.Name], distances[plans[j].Net.Name]) {
				// In a narrow-pitch bundle, commit the compact escape first.
				// Longer peers have more free-space alternatives, while a short
				// central escape can be sealed by either adjacent route.
				return distanceLess(distances[plans[i].Net.Name], distances[plans[j].Net.Name])
			}
			leftDenseFanout := denseConstrainedFanout(plans[i], access, request.Rules, distances, distanceFloors, meanDistanceFloors)
			rightDenseFanout := denseConstrainedFanout(plans[j], access, request.Rules, distances, distanceFloors, meanDistanceFloors)
			if !usesFanoutPressureOrder && leftDenseFanout != rightDenseFanout {
				return leftDenseFanout
			}
			if !usesFanoutPressureOrder && leftDenseFanout {
				return netLess(plans[i].Net, plans[j].Net)
			}
			if !usesFanoutPressureOrder {
				leftSpan := accessSpans[plans[i].Net.Name]
				rightSpan := accessSpans[plans[j].Net.Name]
				if !distanceEqual(leftSpan, rightSpan) {
					return distanceLess(leftSpan, rightSpan)
				}
			}
			leftConstrained := endpointAccessConstrained(plans[i], accessSpans, request.Rules, usesFanoutPressureOrder)
			rightConstrained := endpointAccessConstrained(plans[j], accessSpans, request.Rules, usesFanoutPressureOrder)
			leftScheduleRank := routingScheduleRank(plans[i], leftConstrained)
			rightScheduleRank := routingScheduleRank(plans[j], rightConstrained)
			if leftScheduleRank != rightScheduleRank {
				return leftScheduleRank < rightScheduleRank
			}
			if usesFanoutPressureOrder && leftDenseFanout != rightDenseFanout {
				// Reserve a small escape before a dense peer, without allowing
				// unrelated broad power or ground work to outrank constrained
				// signal fanout merely because it is not dense.
				return !leftDenseFanout
			}
			if usesFanoutPressureOrder && leftDenseFanout {
				return netLess(plans[i].Net, plans[j].Net)
			}
			if leftConstrained {
				if constrainedRoleRank(plans[i].Net.Role) != constrainedRoleRank(plans[j].Net.Role) {
					return constrainedRoleRank(plans[i].Net.Role) < constrainedRoleRank(plans[j].Net.Role)
				}
				if len(plans[i].Net.Endpoints) != len(plans[j].Net.Endpoints) {
					return len(plans[i].Net.Endpoints) < len(plans[j].Net.Endpoints)
				}
				if usesFanoutPressureOrder && len(plans[i].Net.Endpoints) > 3 && !distanceEqual(distances[plans[i].Net.Name], distances[plans[j].Net.Name]) {
					// For comparable fanout trees, commit the compact tree first;
					// its shared local corridors are easier for a longer peer to
					// route around than the reverse ordering.
					return distanceLess(distances[plans[i].Net.Name], distances[plans[j].Net.Name])
				}
				if usesFanoutPressureOrder {
					leftEdgeDistance := edgeDistances[plans[i].Net.Name]
					rightEdgeDistance := edgeDistances[plans[j].Net.Name]
					if !distanceEqual(leftEdgeDistance, rightEdgeDistance) {
						return distanceLess(leftEdgeDistance, rightEdgeDistance)
					}
					leftSpan := accessSpans[plans[i].Net.Name]
					rightSpan := accessSpans[plans[j].Net.Name]
					if !distanceEqual(leftSpan, rightSpan) {
						return distanceLess(leftSpan, rightSpan)
					}
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

func plannedDenseSignalBundleRefs(plans []PlannedNet, access PadAccess, rules Rules) map[string]bool {
	netsByRef := map[string]map[string]bool{}
	distanceByNet := map[string]float64{}
	accessPitch := 2*rules.GridMM + rules.TraceWidthMM
	if rules.GridMM <= 0 {
		return nil
	}
	for _, plan := range plans {
		if plan.Net.Role != NetSignal {
			continue
		}
		distanceByNet[plan.Net.Name] = plannedNetDistance(plan, access)
		for _, endpoint := range plan.Net.Endpoints {
			pad, ok := access.Pads[endpointKey(normalizeKey(endpoint.Ref), normalizeKey(endpoint.Pin))]
			if !ok {
				continue
			}
			span := math.Min(pad.Size.WidthMM, pad.Size.HeightMM)
			if span <= 0 || span > accessPitch+distanceEpsilon {
				continue
			}
			ref := normalizeKey(endpoint.Ref)
			if netsByRef[ref] == nil {
				netsByRef[ref] = map[string]bool{}
			}
			netsByRef[ref][plan.Net.Name] = true
		}
	}
	dense := map[string]bool{}
	for ref, nets := range netsByRef {
		if len(nets) < 3 {
			continue
		}
		shortest := math.Inf(1)
		longest := 0.0
		for net := range nets {
			distance := distanceByNet[net]
			shortest = math.Min(shortest, distance)
			longest = math.Max(longest, distance)
		}
		// Short-first scheduling is appropriate only for a spatially compact
		// bundle. When peers span widely different distances or board regions,
		// preserve the established constrained-net order so a long escape is not
		// sealed by several unrelated local routes.
		if shortest > distanceEpsilon && !math.IsInf(shortest, 0) && longest <= localEscapeGapRatio*shortest+distanceEpsilon {
			dense[ref] = true
		}
	}
	return dense
}

func denseSignalBundle(plan PlannedNet, denseRefs map[string]bool) bool {
	if plan.Net.Role != NetSignal || len(denseRefs) == 0 {
		return false
	}
	for _, endpoint := range plan.Net.Endpoints {
		if denseRefs[normalizeKey(endpoint.Ref)] {
			return true
		}
	}
	return false
}

func plannedConstrainedEndpointEdgeDistances(plans []PlannedNet, access PadAccess, board Board, rules Rules) map[string]float64 {
	distances := make(map[string]float64, len(plans))
	usable := UsableBoardRect(board, rules)
	accessPitch := 2*rules.GridMM + rules.TraceWidthMM
	for _, plan := range plans {
		best := math.Inf(1)
		for _, endpoint := range plan.Net.Endpoints {
			pad, ok := access.Pads[endpointKey(endpoint.Ref, endpoint.Pin)]
			if !ok {
				continue
			}
			span := math.Min(pad.Size.WidthMM, pad.Size.HeightMM)
			if span <= 0 || span > accessPitch+distanceEpsilon {
				continue
			}
			edgeDistance := min(
				math.Abs(pad.Position.XMM-usable.Min.XMM),
				math.Abs(usable.Max.XMM-pad.Position.XMM),
				math.Abs(pad.Position.YMM-usable.Min.YMM),
				math.Abs(usable.Max.YMM-pad.Position.YMM),
			)
			best = math.Min(best, edgeDistance)
		}
		distances[plan.Net.Name] = best
	}
	return distances
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

func endpointAccessConstrained(plan PlannedNet, accessSpans map[string]float64, rules Rules, allowBoundedSignalFanout bool) bool {
	// Bound escape promotion by the number of tree branches. Signals may have
	// several receivers that share one narrow-pitch controller pad, while broad
	// power and ground trees remain limited to a small local fanout.
	maxEndpoints := 3
	if allowBoundedSignalFanout && plan.Net.Role == NetSignal {
		maxEndpoints = maxDenseFanoutBranches + 1
	}
	if len(plan.Net.Endpoints) < 2 || len(plan.Net.Endpoints) > maxEndpoints || rules.GridMM <= 0 {
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

func planEndpointPairs(netName string, role NetRole, endpoints []Endpoint, access PadAccess, rules Rules, preserveConstrainedTerminals bool) ([]EndpointPair, []reports.Issue) {
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
	if preserveConstrainedTerminals && repeatedConstrainedEndpointRef(ordered, access, rules) {
		return planRepeatedConstrainedLeaves(ordered, accessPoints, access, rules), nil
	}
	preserveConstrainedLeaves :=
		preserveConstrainedTerminals && ((role == NetSignal && len(ordered) > 3 && len(ordered) <= maxDenseFanoutBranches+1) ||
			(role == NetPower && len(ordered) > maxDenseFanoutBranches+maxLocalRoutePrefixNets))
	rootIndex := 0
	if preserveConstrainedLeaves {
		for index, endpoint := range ordered {
			if !endpointBranchConstrained(endpoint, access, rules) {
				rootIndex = index
				break
			}
		}
	}
	inTree := make([]bool, len(ordered))
	bestFrom := make([]int, len(ordered))
	bestDistances := make([]float64, len(ordered))
	for index := range bestDistances {
		bestFrom[index] = rootIndex
		bestDistances[index] = math.Inf(1)
	}
	inTree[rootIndex] = true
	for index := range ordered {
		if index == rootIndex {
			continue
		}
		bestDistances[index] = endpointDistance(accessPoints[rootIndex], accessPoints[index])
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
		if preserveConstrainedLeaves && endpointBranchConstrained(ordered[bestIndex], access, rules) {
			continue
		}
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

func planRepeatedConstrainedLeaves(ordered []Endpoint, accessPoints [][]AccessPoint, access PadAccess, rules Rules) []EndpointPair {
	refCounts := map[string]int{}
	for _, endpoint := range ordered {
		refCounts[normalizeKey(endpoint.Ref)]++
	}
	trunkIndexes := make([]int, 0, len(ordered))
	leafIndexes := make([]int, 0, len(ordered))
	for index, endpoint := range ordered {
		if refCounts[normalizeKey(endpoint.Ref)] > 1 && endpointBranchConstrained(endpoint, access, rules) {
			leafIndexes = append(leafIndexes, index)
		} else {
			trunkIndexes = append(trunkIndexes, index)
		}
	}
	if len(trunkIndexes) == 0 {
		return nil
	}
	pairs := planIndexedEndpointTree(ordered, accessPoints, trunkIndexes)
	for _, leafIndex := range leafIndexes {
		bestTrunk := trunkIndexes[0]
		bestDistance := endpointDistance(accessPoints[bestTrunk], accessPoints[leafIndex])
		for _, trunkIndex := range trunkIndexes[1:] {
			distance := endpointDistance(accessPoints[trunkIndex], accessPoints[leafIndex])
			candidate := EndpointPair{From: ordered[trunkIndex], To: ordered[leafIndex]}
			current := EndpointPair{From: ordered[bestTrunk], To: ordered[leafIndex]}
			if distanceLess(distance, bestDistance) || (distanceEqual(distance, bestDistance) && endpointPairLess(candidate, current)) {
				bestTrunk = trunkIndex
				bestDistance = distance
			}
		}
		pairs = append(pairs, EndpointPair{From: ordered[bestTrunk], To: ordered[leafIndex]})
	}
	return pairs
}

func planIndexedEndpointTree(ordered []Endpoint, accessPoints [][]AccessPoint, indexes []int) []EndpointPair {
	if len(indexes) < 2 {
		return nil
	}
	inTree := map[int]bool{indexes[0]: true}
	pairs := make([]EndpointPair, 0, len(indexes)-1)
	for len(pairs) < len(indexes)-1 {
		bestFrom := -1
		bestTo := -1
		bestDistance := math.Inf(1)
		for _, from := range indexes {
			if !inTree[from] {
				continue
			}
			for _, to := range indexes {
				if inTree[to] {
					continue
				}
				distance := endpointDistance(accessPoints[from], accessPoints[to])
				candidate := EndpointPair{From: ordered[from], To: ordered[to]}
				current := EndpointPair{}
				if bestFrom >= 0 {
					current = EndpointPair{From: ordered[bestFrom], To: ordered[bestTo]}
				}
				if bestFrom < 0 || distanceLess(distance, bestDistance) || (distanceEqual(distance, bestDistance) && endpointPairLess(candidate, current)) {
					bestFrom = from
					bestTo = to
					bestDistance = distance
				}
			}
		}
		pairs = append(pairs, EndpointPair{From: ordered[bestFrom], To: ordered[bestTo]})
		inTree[bestTo] = true
	}
	return pairs
}

func repeatedConstrainedEndpointRef(endpoints []Endpoint, access PadAccess, rules Rules) bool {
	refs := map[string]int{}
	for _, endpoint := range endpoints {
		ref := normalizeKey(endpoint.Ref)
		if ref == "" {
			continue
		}
		refs[ref]++
	}
	if len(refs) < 2 {
		return false
	}
	for _, endpoint := range endpoints {
		if refs[normalizeKey(endpoint.Ref)] > 1 && endpointBranchConstrained(endpoint, access, rules) {
			return true
		}
	}
	return false
}

func endpointBranchConstrained(endpoint Endpoint, access PadAccess, rules Rules) bool {
	if rules.GridMM <= 0 {
		return false
	}
	span := endpointPadSpan(endpoint, access)
	accessPitch := 2*rules.GridMM + rules.TraceWidthMM
	return span > 0 && !math.IsInf(span, 0) && span <= accessPitch+distanceEpsilon
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
