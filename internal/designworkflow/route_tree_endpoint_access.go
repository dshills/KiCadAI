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

func BuildRouteTreeEndpointAccess(targetEvidence InterBlockContactEvidence, routeOperations []transactions.Operation) []RouteTreeEndpointAccess {
	access, _ := BuildRouteTreeEndpointAccessWithIssues(targetEvidence, routeOperations)
	return access
}

func BuildRouteTreeEndpointAccessWithIssues(targetEvidence InterBlockContactEvidence, routeOperations []transactions.Operation) ([]RouteTreeEndpointAccess, []reports.Issue) {
	access := make([]RouteTreeEndpointAccess, 0, len(targetEvidence.Targets)+len(routeOperations)*2)
	for _, target := range targetEvidence.Targets {
		access = append(access, routeTreeEndpointAccessFromTarget(target))
	}
	operationsByNet, issues := decodeInterBlockRouteOperations(routeOperations)
	netNames := make([]string, 0, len(operationsByNet))
	for netName := range operationsByNet {
		netNames = append(netNames, netName)
	}
	slices.Sort(netNames)
	for _, netName := range netNames {
		operations := operationsByNet[netName]
		for _, operation := range operations {
			if len(operation.Points) == 0 {
				continue
			}
			for _, point := range routeTreeOperationAnchorPoints(operation.Points) {
				access = append(access, RouteTreeEndpointAccess{
					Role:   RouteTreeAccessLocalRouteAnchor,
					Net:    strings.TrimSpace(netName),
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
	if len(points) == 0 {
		return nil
	}
	if len(points) == 1 {
		return []transactions.Point{points[0]}
	}
	return []transactions.Point{points[0], points[len(points)-1]}
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
