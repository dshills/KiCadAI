package schematiclayout

import (
	"cmp"
	"slices"

	"kicadai/internal/kicadfiles"
)

type functionalLabelCorridor struct {
	net string
	box Rect
}

// Reserve the possibility of a label before routing any net. Otherwise an
// early local conductor can trap later fallback labels behind its vertical
// segments. Use narrow glyph-height corridors, not placement body spacing.
func functionalRouteLabelCorridors(request Request, result Result) []functionalLabelCorridor {
	var corridors []functionalLabelCorridor
	for _, component := range result.Components {
		for _, net := range request.Nets {
			net.LocalWiring = false
			for _, box := range supportOwnerPinCorridors(component.Component, []Net{net}, component.PlacedAt) {
				corridors = append(corridors, functionalLabelCorridor{net: net.Name, box: box.Inflate(-kicadfiles.MM(1.905))})
			}
		}
	}
	return corridors
}

// Connect only clean local branches. Each disconnected group/obstructed island
// retains one labeled connection to the next island, preserving an N-1 logical
// tree for the transaction adapter. Grid routing keeps its existing node bound;
// a failed route is never emitted as a conductor.
func routeFunctionalLocalTrees(result *Result, labeled map[string]kicadfiles.Point, net Net, endpoints []routableEndpoint, request Request, rules Rules, anchors pinAnchorIndex) {
	groupsByRef := map[string]string{}
	for _, component := range request.Components {
		groupsByRef[component.Ref] = component.GroupID
	}
	groups := map[string][]routableEndpoint{}
	var keys []string
	for _, endpoint := range endpoints {
		key := groupsByRef[endpoint.endpoint.Ref]
		if key == "" {
			key = "\x00" + endpoint.endpoint.Ref // Unowned components never coalesce.
		}
		if _, exists := groups[key]; !exists {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], endpoint)
	}
	slices.Sort(keys)
	var islands []routableEndpoint
	for _, key := range keys {
		local := groups[key]
		localRequest := request
		localRequest.FunctionalPowerLocality = request.FunctionalPowerLocality && powerLocalityGroup(key, request.Components)
		// Stable endpoint order is independent of input order and coordinates.
		slices.SortFunc(local, func(a, b routableEndpoint) int {
			if localRequest.FunctionalPowerLocality && containsNormalizedRole(net.Role, "power", "power_pos", "power_neg", "ground", "return") {
				if order := cmp.Compare(powerRootPriority(a.endpoint.Ref, request.Components), powerRootPriority(b.endpoint.Ref, request.Components)); order != 0 {
					return order
				}
			}
			if order := cmp.Compare(a.endpoint.Ref, b.endpoint.Ref); order != 0 {
				return order
			}
			return cmp.Compare(a.endpoint.Pin, b.endpoint.Pin)
		})
		islands = append(islands, local[0])
		for i, endpoint := range local[1:] {
			parents := slices.Clone(local[:i+1])
			slices.SortStableFunc(parents, func(a, b routableEndpoint) int {
				return cmp.Compare(manhattan(a.anchor, endpoint.anchor), manhattan(b.anchor, endpoint.anchor))
			})
			connected := false
			for _, parent := range parents {
				points, clean := routeConnectionPoints(net.Name, parent.endpoint, endpoint.endpoint, parent.anchor, endpoint.anchor, *result, localRequest, rules, anchors, true)
				if !clean {
					continue
				}
				result.Connections = append(result.Connections, RoutedConnection{NetName: net.Name, From: parent.endpoint, To: endpoint.endpoint, Points: points})
				result.Wires = append(result.Wires, segmentsForPoints(net.Name, points)...)
				connected = true
				break
			}
			if !connected {
				islands = append(islands, endpoint)
				result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityInfo, Code: "functional_local_route_fallback", NetName: net.Name, Ref: endpoint.endpoint.Ref, Message: "no clean bounded local branch; retained a labeled island"})
			}
		}
	}
	if len(islands) == 1 {
		appendRouteAnnotation(result, net.Name, request, rules)
		return
	}
	for i := 1; i < len(islands); i++ {
		from, to := islands[i-1], islands[i]
		fromLabel := appendEndpointLabel(result, labeled, net.Name, from.endpoint, from.anchor, request, rules)
		toLabel := appendEndpointLabel(result, labeled, net.Name, to.endpoint, to.anchor, request, rules)
		result.Connections = append(result.Connections, RoutedConnection{NetName: net.Name, From: from.endpoint, To: to.endpoint, UseLabels: true, FromLabelAt: &fromLabel, ToLabelAt: &toLabel})
	}
}
