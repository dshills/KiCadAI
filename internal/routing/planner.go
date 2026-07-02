package routing

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/reports"
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
	return plans, issues
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
