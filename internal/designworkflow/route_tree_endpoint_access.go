package designworkflow

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type RouteTreeEndpointAccessRole string

const (
	RouteTreeAccessSourcePad        RouteTreeEndpointAccessRole = "source_pad"
	RouteTreeAccessTargetPad        RouteTreeEndpointAccessRole = "target_pad"
	RouteTreeAccessLocalRouteAnchor RouteTreeEndpointAccessRole = "local_route_anchor"
	RouteTreeAccessSameNetCopper    RouteTreeEndpointAccessRole = "same_net_copper"
	RouteTreeAccessExternalAnchor   RouteTreeEndpointAccessRole = "external_anchor"
)

type RouteTreeEndpointAccess struct {
	EndpointID string                      `json:"endpoint_id,omitempty"`
	Role       RouteTreeEndpointAccessRole `json:"role"`
	Ref        string                      `json:"ref,omitempty"`
	Pad        string                      `json:"pad,omitempty"`
	Net        string                      `json:"net"`
	Layer      string                      `json:"layer,omitempty"`
	XMM        float64                     `json:"x_mm"`
	YMM        float64                     `json:"y_mm"`
	Source     string                      `json:"source"`
}

type RouteTreeEndpointAccessSummary struct {
	AccessPoints      int      `json:"access_points"`
	PadAccess         int      `json:"pad_access"`
	LocalRouteAnchors int      `json:"local_route_anchors"`
	SameNetCopper     int      `json:"same_net_copper"`
	ExternalAnchors   int      `json:"external_anchors"`
	Nets              []string `json:"nets,omitempty"`
	Refs              []string `json:"refs,omitempty"`
}

const routeTreeBranchAccessPairLimit = 8

const (
	routeTreeAccessExactEndpointRank    int = 0
	routeTreeAccessFallbackEndpointRank int = 1
	routeTreeAccessPreferredRoleRank    int = 0
)

const (
	routeTreeAccessPreferredLayerRank int   = 0
	routeTreeAccessMissingLayerRank   int   = 1
	routeTreeAccessMissingDistance    int64 = 1<<63 - 1
)

type routeTreeBranchAccessCandidate struct {
	Access       RouteTreeEndpointAccess
	EndpointRank int
	RoleRank     int
	DistanceRank int64
	LayerRank    int
	ObstacleRank int
	RankReason   string
}

type routeTreeBranchAccessPair struct {
	Source routeTreeBranchAccessCandidate
	Target routeTreeBranchAccessCandidate
	Rank   int
}

func BuildRouteTreeEndpointAccess(targetEvidence InterBlockContactEvidence, routeOperations []transactions.Operation) []RouteTreeEndpointAccess {
	access, _ := BuildRouteTreeEndpointAccessWithIssues(targetEvidence, routeOperations)
	return access
}

func routeTreeAccessCandidatesForEndpoint(access []RouteTreeEndpointAccess, endpointID string, netName string, opposite RouteTreeEndpointAccess) []routeTreeBranchAccessCandidate {
	var candidates []routeTreeBranchAccessCandidate
	normalizedEndpointID := strings.TrimSpace(endpointID)
	netName = strings.TrimSpace(netName)
	for _, item := range access {
		if netName != "" && item.Net != netName {
			continue
		}
		itemEndpointID := strings.TrimSpace(item.EndpointID)
		if normalizedEndpointID != "" && itemEndpointID != "" && itemEndpointID != normalizedEndpointID {
			continue
		}
		endpointRank := routeTreeAccessEndpointRank(itemEndpointID, normalizedEndpointID)
		roleRank := routeTreeAccessRoleRank(item.Role)
		layerRank := routeTreeAccessLayerRank(item)
		distanceRank := routeTreeAccessDistanceRank(item, opposite)
		candidates = append(candidates, routeTreeBranchAccessCandidate{
			Access:       item,
			EndpointRank: endpointRank,
			RoleRank:     roleRank,
			DistanceRank: distanceRank,
			LayerRank:    layerRank,
			ObstacleRank: 0,
			RankReason:   routeTreeAccessRankReason(item, itemEndpointID, endpointRank, layerRank, distanceRank),
		})
	}
	slices.SortStableFunc(candidates, compareRouteTreeAccessCandidate)
	return candidates
}

func routeTreeBranchAccessPairs(sourceCandidates []routeTreeBranchAccessCandidate, targetCandidates []routeTreeBranchAccessCandidate, limit int) []routeTreeBranchAccessPair {
	if limit <= 0 {
		limit = routeTreeBranchAccessPairLimit
	}
	sources := slices.Clone(sourceCandidates)
	targets := slices.Clone(targetCandidates)
	slices.SortFunc(sources, compareRouteTreeAccessCandidate)
	slices.SortFunc(targets, compareRouteTreeAccessCandidate)
	pairs := make([]routeTreeBranchAccessPair, 0, min(len(sources)*len(targets), limit))
	for _, source := range sources {
		for _, target := range targets {
			if routeTreeAccessIsGeneratedSameNetCopper(source.Access) && routeTreeAccessIsGeneratedSameNetCopper(target.Access) {
				continue
			}
			if len(pairs) >= limit {
				return pairs
			}
			pairs = append(pairs, routeTreeBranchAccessPair{Source: source, Target: target, Rank: len(pairs)})
		}
	}
	return pairs
}

func routeTreeAccessIsGeneratedSameNetCopper(access RouteTreeEndpointAccess) bool {
	return access.Role == RouteTreeAccessSameNetCopper && strings.TrimSpace(access.Source) == routeTreeSameNetExistingCopperSource
}

func routeTreeAccessRoleRank(role RouteTreeEndpointAccessRole) int {
	switch role {
	case RouteTreeAccessLocalRouteAnchor:
		return 0
	case RouteTreeAccessSameNetCopper:
		return 1
	case RouteTreeAccessSourcePad, RouteTreeAccessTargetPad:
		return 2
	case RouteTreeAccessExternalAnchor:
		return 3
	default:
		return 4
	}
}

func routeTreeAccessEndpointRank(itemEndpointID string, endpointID string) int {
	if endpointID == "" {
		return routeTreeAccessExactEndpointRank
	}
	if itemEndpointID == endpointID {
		return routeTreeAccessExactEndpointRank
	}
	return routeTreeAccessFallbackEndpointRank
}

func routeTreeAccessDistanceRank(item RouteTreeEndpointAccess, opposite RouteTreeEndpointAccess) int64 {
	if opposite.Net == "" && opposite.XMM == 0 && opposite.YMM == 0 {
		return routeTreeAccessMissingDistance
	}
	dx := item.XMM - opposite.XMM
	dy := item.YMM - opposite.YMM
	return int64(math.Round((dx*dx + dy*dy) * 1_000_000))
}

func routeTreeAccessLayerRank(item RouteTreeEndpointAccess) int {
	if item.Layer == "" {
		return routeTreeAccessMissingLayerRank
	}
	return routeTreeAccessPreferredLayerRank
}

func routeTreeAccessRankReason(item RouteTreeEndpointAccess, itemEndpointID string, endpointRank int, layerRank int, distanceRank int64) string {
	reasons := []string{string(item.Role)}
	switch endpointRank {
	case routeTreeAccessExactEndpointRank:
		if itemEndpointID != "" {
			reasons = append(reasons, "exact_endpoint")
		} else {
			reasons = append(reasons, "endpoint_unscoped")
		}
	case routeTreeAccessFallbackEndpointRank:
		reasons = append(reasons, "net_scoped_fallback")
	default:
		reasons = append(reasons, "endpoint_rank_unknown")
	}
	switch item.Role {
	case RouteTreeAccessLocalRouteAnchor:
		reasons = append(reasons, "preferred_local_route_anchor")
	case RouteTreeAccessSameNetCopper:
		reasons = append(reasons, "same_net_copper_merge_candidate")
	case RouteTreeAccessSourcePad, RouteTreeAccessTargetPad:
		reasons = append(reasons, "pad_access_fallback")
	case RouteTreeAccessExternalAnchor:
		reasons = append(reasons, "external_anchor_fallback")
	}
	if layerRank == routeTreeAccessPreferredLayerRank {
		reasons = append(reasons, "layer_known")
	} else {
		reasons = append(reasons, "layer_missing")
	}
	if distanceRank == routeTreeAccessMissingDistance {
		reasons = append(reasons, "opposite_missing")
	} else {
		reasons = append(reasons, "distance_ranked")
	}
	return joinRouteTreeAccessRankReasons(reasons)
}

func joinRouteTreeAccessRankReasons(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	out := reasons[0]
	for _, reason := range reasons[1:] {
		out += "," + reason
	}
	return out
}

func compareRouteTreeAccessCandidate(left, right routeTreeBranchAccessCandidate) int {
	if compare := cmp.Compare(left.EndpointRank, right.EndpointRank); compare != 0 {
		return compare
	}
	if compare := cmp.Compare(left.RoleRank, right.RoleRank); compare != 0 {
		return compare
	}
	if compare := cmp.Compare(left.LayerRank, right.LayerRank); compare != 0 {
		return compare
	}
	if compare := cmp.Compare(left.ObstacleRank, right.ObstacleRank); compare != 0 {
		return compare
	}
	if compare := cmp.Compare(left.DistanceRank, right.DistanceRank); compare != 0 {
		return compare
	}
	return compareRouteTreeEndpointAccess(left.Access, right.Access)
}

func BuildRouteTreeEndpointAccessWithIssues(targetEvidence InterBlockContactEvidence, routeOperations []transactions.Operation) ([]RouteTreeEndpointAccess, []reports.Issue) {
	access := make([]RouteTreeEndpointAccess, 0, len(targetEvidence.Targets)+len(routeOperations)*2)
	for _, target := range targetEvidence.Targets {
		access = append(access, routeTreeEndpointAccessFromTarget(target))
	}
	targetsByNet := routeTreeEndpointTargetsByNet(targetEvidence)
	operationsByNet, issues := decodeInterBlockRouteOperations(routeOperations)
	netNames := make([]string, 0, len(operationsByNet))
	for netName := range operationsByNet {
		netNames = append(netNames, netName)
	}
	slices.Sort(netNames)
	for _, netName := range netNames {
		operationNet := strings.TrimSpace(netName)
		operationNetKey := routeTreeEndpointNetKey(netName)
		operations := operationsByNet[netName]
		for _, operation := range operations {
			if len(operation.Points) == 0 {
				continue
			}
			anchors := routeTreeOperationAnchorPoints(operation.Points)
			if scopedAnchors := routeTreeEndpointScopedOperationAnchors(targetsByNet, operation, anchors, operationNet, operationNetKey); len(scopedAnchors) != 0 {
				access = append(access, scopedAnchors...)
				continue
			}
			for _, point := range anchors {
				access = append(access, RouteTreeEndpointAccess{
					Role:   RouteTreeAccessLocalRouteAnchor,
					Net:    operationNet,
					Layer:  normalizeContactLayer(operation.Layer),
					XMM:    point.XMM,
					YMM:    point.YMM,
					Source: "generated_route_operation",
				})
			}
		}
	}
	access = uniqueRouteTreeEndpointAccess(access)
	slices.SortFunc(access, compareRouteTreeEndpointAccess)
	return access, issues
}

func SummarizeRouteTreeEndpointAccess(access []RouteTreeEndpointAccess) RouteTreeEndpointAccessSummary {
	summary := RouteTreeEndpointAccessSummary{AccessPoints: len(access)}
	netSet := map[string]struct{}{}
	refSet := map[string]struct{}{}
	for _, item := range access {
		switch item.Role {
		case RouteTreeAccessSourcePad, RouteTreeAccessTargetPad:
			summary.PadAccess++
		case RouteTreeAccessLocalRouteAnchor:
			summary.LocalRouteAnchors++
		case RouteTreeAccessSameNetCopper:
			summary.SameNetCopper++
		case RouteTreeAccessExternalAnchor:
			summary.ExternalAnchors++
		}
		if item.Net != "" {
			netSet[item.Net] = struct{}{}
		}
		if item.Ref != "" {
			refSet[item.Ref] = struct{}{}
		}
	}
	summary.Nets = sortedSetKeys(netSet)
	summary.Refs = sortedSetKeys(refSet)
	return summary
}

func routeTreeEndpointAccessFromTarget(target InterBlockContactTarget) RouteTreeEndpointAccess {
	role := RouteTreeAccessTargetPad
	switch target.Kind {
	case InterBlockContactTargetSameNetCopper:
		role = RouteTreeAccessSameNetCopper
	case InterBlockContactTargetTrackEndpoint, InterBlockContactTargetVia:
		role = RouteTreeAccessLocalRouteAnchor
	}
	return RouteTreeEndpointAccess{
		EndpointID: target.EndpointID,
		Role:       role,
		Ref:        target.Ref,
		Pad:        target.Pad,
		Net:        target.NetName,
		Layer:      normalizeContactLayer(target.Layer),
		XMM:        target.Point.XMM,
		YMM:        target.Point.YMM,
		Source:     target.GeometrySource,
	}
}

func routeTreeOperationAnchorPoints(points []transactions.Point) []transactions.Point {
	switch len(points) {
	case 0:
		return nil
	case 1:
		return []transactions.Point{points[0]}
	}
	first := points[0]
	last := points[len(points)-1]
	if routeTreeSamePoint(first, last) {
		return []transactions.Point{first}
	}
	return []transactions.Point{first, last}
}

func routeTreeSamePoint(left transactions.Point, right transactions.Point) bool {
	const epsilon = interBlockContactToleranceMM
	dx := left.XMM - right.XMM
	dy := left.YMM - right.YMM
	return dx*dx+dy*dy <= epsilon*epsilon
}

func routeTreeEndpointTargetsByNet(targetEvidence InterBlockContactEvidence) map[string][]InterBlockContactTarget {
	targetsByNet := make(map[string][]InterBlockContactTarget)
	for _, target := range targetEvidence.Targets {
		netKey := routeTreeEndpointNetKey(target.NetName)
		if netKey == "" {
			continue
		}
		targetsByNet[netKey] = append(targetsByNet[netKey], target)
	}
	return targetsByNet
}

func routeTreeEndpointNetKey(netName string) string {
	return strings.ToLower(strings.TrimSpace(netName))
}

func routeTreeEndpointScopedOperationAnchors(targetsByNet map[string][]InterBlockContactTarget, operation decodedContactRouteOperation, anchors []transactions.Point, operationNet string, operationNetKey string) []RouteTreeEndpointAccess {
	if len(anchors) == 0 {
		return nil
	}
	operationLayer := normalizeContactLayer(operation.Layer)
	matches := routeTreeOperationEndpointMatches(targetsByNet[operationNetKey], operation)
	if len(matches) != 1 {
		return nil
	}
	target := matches[0]
	if target.Layer != "" && operationLayer != "" && normalizeContactLayer(target.Layer) != operationLayer {
		return nil
	}
	out := make([]RouteTreeEndpointAccess, 0, len(anchors))
	// When a generated local route touches exactly one route-tree endpoint, the
	// far end of that continuous same-net route is a valid access point for the
	// endpoint. Routes touching multiple route-tree endpoints remain unscoped to
	// avoid assigning one endpoint's pad coordinate to another endpoint.
	for _, anchor := range anchors {
		out = append(out, RouteTreeEndpointAccess{
			EndpointID: target.EndpointID,
			Role:       RouteTreeAccessLocalRouteAnchor,
			Ref:        target.Ref,
			Pad:        target.Pad,
			Net:        operationNet,
			Layer:      operationLayer,
			XMM:        anchor.XMM,
			YMM:        anchor.YMM,
			Source:     "generated_route_endpoint_binding",
		})
	}
	return out
}

func routeTreeOperationEndpointMatches(targets []InterBlockContactTarget, operation decodedContactRouteOperation) []InterBlockContactTarget {
	matches := make([]InterBlockContactTarget, 0, 1)
	for _, target := range targets {
		if strings.TrimSpace(target.EndpointID) == "" {
			continue
		}
		if !routeTreeOperationTouchesTarget(operation.Points, target) {
			continue
		}
		matches = append(matches, target)
	}
	return matches
}

func routeTreeOperationTouchesTarget(points []transactions.Point, target InterBlockContactTarget) bool {
	tolerance := target.ToleranceMM
	if tolerance <= 0 {
		tolerance = interBlockContactToleranceMM
	}
	toleranceSquared := tolerance * tolerance
	for _, point := range points {
		dx := point.XMM - target.Point.XMM
		dy := point.YMM - target.Point.YMM
		if dx*dx+dy*dy <= toleranceSquared {
			return true
		}
	}
	return false
}

func uniqueRouteTreeEndpointAccess(access []RouteTreeEndpointAccess) []RouteTreeEndpointAccess {
	seen := map[routeTreeEndpointAccessKey]struct{}{}
	out := make([]RouteTreeEndpointAccess, 0, len(access))
	for _, item := range access {
		key := routeTreeEndpointAccessKeyFor(item)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

type routeTreeEndpointAccessKey struct {
	role      RouteTreeEndpointAccessRole
	endpoint  string
	net       string
	ref       string
	pad       string
	layer     string
	xMicron   int64
	yMicron   int64
	sourceKey string
}

func routeTreeEndpointAccessKeyFor(item RouteTreeEndpointAccess) routeTreeEndpointAccessKey {
	return routeTreeEndpointAccessKey{
		role:      item.Role,
		endpoint:  item.EndpointID,
		net:       item.Net,
		ref:       item.Ref,
		pad:       item.Pad,
		layer:     item.Layer,
		xMicron:   routeTreeMicronKey(item.XMM),
		yMicron:   routeTreeMicronKey(item.YMM),
		sourceKey: strings.TrimSpace(item.Source),
	}
}

func compareRouteTreeEndpointAccess(left, right RouteTreeEndpointAccess) int {
	if compare := strings.Compare(left.Net, right.Net); compare != 0 {
		return compare
	}
	if compare := strings.Compare(string(left.Role), string(right.Role)); compare != 0 {
		return compare
	}
	if compare := strings.Compare(left.EndpointID, right.EndpointID); compare != 0 {
		return compare
	}
	if compare := strings.Compare(left.Ref, right.Ref); compare != 0 {
		return compare
	}
	if compare := strings.Compare(left.Pad, right.Pad); compare != 0 {
		return compare
	}
	if compare := strings.Compare(left.Layer, right.Layer); compare != 0 {
		return compare
	}
	if compare := cmp.Compare(routeTreeMicronKey(left.XMM), routeTreeMicronKey(right.XMM)); compare != 0 {
		return compare
	}
	if compare := cmp.Compare(routeTreeMicronKey(left.YMM), routeTreeMicronKey(right.YMM)); compare != 0 {
		return compare
	}
	return strings.Compare(left.Source, right.Source)
}

func routeTreeMicronKey(value float64) int64 {
	return int64(math.Round(value * 1000))
}
