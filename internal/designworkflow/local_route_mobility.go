package designworkflow

import (
	"kicadai/internal/blocks"
	"kicadai/internal/placement"
)

type LocalRouteMobilitySummary struct {
	Total         int `json:"total"`
	Transformable int `json:"transformable"`
	Rebuildable   int `json:"rebuildable"`
	Preserved     int `json:"preserved"`
	Blocked       int `json:"blocked"`
}

// classifyLocalRouteMobility summarizes how generated local routes must be
// handled before placement retry can safely move their endpoint components.
func classifyLocalRouteMobility(fragments PCBFragmentResult, request placement.Request) LocalRouteMobilitySummary {
	policyByRef := make(map[string]placement.MobilityPolicy, len(request.Components))
	for _, component := range request.Components {
		policyByRef[component.Ref] = component.Mobility
	}
	summary := LocalRouteMobilitySummary{}
	for _, fragment := range fragments.Fragments {
		for _, route := range fragment.Realization.LocalRoutes {
			summary.Total++
			switch classifyLocalRoute(route, policyByRef) {
			case placement.RouteHandlingTransformWithGroup:
				summary.Transformable++
			case placement.RouteHandlingInvalidateRebuild:
				summary.Rebuildable++
			case placement.RouteHandlingPreserveFixed:
				summary.Preserved++
			default:
				summary.Blocked++
			}
		}
	}
	return summary
}

func classifyLocalRoute(route blocks.RealizedPCBLocalRoute, policyByRef map[string]placement.MobilityPolicy) placement.RouteHandlingPolicy {
	from := policyByRef[route.From.Ref]
	to := policyByRef[route.To.Ref]
	if localRouteEndpointBlocked(from) || localRouteEndpointBlocked(to) {
		return placement.RouteHandlingUnsupported
	}
	if from.RouteHandling == placement.RouteHandlingPreserveFixed && to.RouteHandling == placement.RouteHandlingPreserveFixed {
		return placement.RouteHandlingPreserveFixed
	}
	if from.RouteHandling == placement.RouteHandlingTransformWithGroup &&
		to.RouteHandling == placement.RouteHandlingTransformWithGroup &&
		from.GroupID != "" &&
		from.GroupID == to.GroupID {
		return placement.RouteHandlingTransformWithGroup
	}
	if localRouteEndpointMovable(from) || localRouteEndpointMovable(to) {
		return placement.RouteHandlingInvalidateRebuild
	}
	return placement.RouteHandlingUnsupported
}

func localRouteEndpointBlocked(policy placement.MobilityPolicy) bool {
	switch policy.Class {
	case placement.MobilityUnowned, "":
		return true
	default:
		return policy.RouteHandling == placement.RouteHandlingUnsupported
	}
}

func localRouteEndpointMovable(policy placement.MobilityPolicy) bool {
	switch policy.RouteHandling {
	case placement.RouteHandlingTransformWithGroup, placement.RouteHandlingInvalidateRebuild:
		return true
	default:
		return false
	}
}
