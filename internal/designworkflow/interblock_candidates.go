package designworkflow

import (
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type InterBlockRouteStatus string

const (
	InterBlockRouteCandidateRoutable InterBlockRouteStatus = "routable"
	InterBlockRouteCandidatePartial  InterBlockRouteStatus = "partial"
	InterBlockRouteCandidateBlocked  InterBlockRouteStatus = "blocked"
	InterBlockRouteCandidateFailed   InterBlockRouteStatus = "failed"
	InterBlockRouteCandidateError    InterBlockRouteStatus = "error"
)

type InterBlockRouteCandidate struct {
	NetName     string                    `json:"net_name"`
	Status      InterBlockRouteStatus     `json:"status"`
	Endpoints   []InterBlockRouteEndpoint `json:"endpoints"`
	InstanceIDs []string                  `json:"instance_ids,omitempty"`
	BlockIDs    []string                  `json:"block_ids,omitempty"`
	Unresolved  int                       `json:"unresolved,omitempty"`
}

type InterBlockRouteEndpoint struct {
	Ref        string `json:"ref"`
	Pin        string `json:"pin"`
	InstanceID string `json:"instance_id,omitempty"`
	BlockID    string `json:"block_id,omitempty"`
}

type physicalPadEndpointsByNet map[string][]InterBlockRouteEndpoint

func BuildInterBlockRouteCandidates(fragments PCBFragmentResult, placed PlacementStageResult) ([]InterBlockRouteCandidate, []reports.Issue) {
	componentRefIndex := generatedComponentFragmentIndex(fragments)
	physicalEndpoints := indexPhysicalPadEndpointsByNet(placed.Request.Components, componentRefIndex)
	var candidates []InterBlockRouteCandidate
	var issues []reports.Issue
	for netIndex, net := range placed.Request.Nets {
		candidate, netIssues, ok := interBlockCandidateFromPlacementNet(netIndex, net, physicalEndpoints, componentRefIndex)
		issues = append(issues, netIssues...)
		if ok {
			candidates = append(candidates, candidate)
		}
	}
	return candidates, issues
}

func interBlockCandidateFromPlacementNet(index int, net placement.Net, physicalEndpoints physicalPadEndpointsByNet, componentRefIndex map[string]BlockFragment) (InterBlockRouteCandidate, []reports.Issue, bool) {
	instanceSet := map[string]struct{}{}
	blockSet := map[string]struct{}{}
	var endpoints []InterBlockRouteEndpoint
	var unresolved int
	var issues []reports.Issue
	for endpointIndex, endpoint := range net.Endpoints {
		ref := strings.TrimSpace(endpoint.Ref)
		pin := strings.TrimSpace(endpoint.Pin)
		if ref == "" || pin == "" {
			continue
		}
		fragment, ok := componentRefIndex[strings.ToUpper(ref)]
		if !ok {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityWarning,
				Path:     fmt.Sprintf("design.inter_block_routing.nets[%d].endpoints[%d]", index, endpointIndex),
				Message:  "net endpoint does not belong to a generated block component",
				Refs:     []string{ref},
				Nets:     []string{net.Name},
			})
			unresolved++
			continue
		}
		endpoints = append(endpoints, InterBlockRouteEndpoint{
			Ref:        ref,
			Pin:        pin,
			InstanceID: fragment.InstanceID,
			BlockID:    fragment.BlockID,
		})
		instanceSet[fragment.InstanceID] = struct{}{}
		blockSet[fragment.BlockID] = struct{}{}
	}
	if len(instanceSet) < 2 {
		return InterBlockRouteCandidate{}, issues, false
	}
	endpoints = appendParticipatingPhysicalPadEndpoints(endpoints, physicalEndpoints[strings.ToUpper(strings.TrimSpace(net.Name))])
	endpoints = pruneLocalRouteInternalEndpoints(net.Name, endpoints, componentRefIndex)
	return InterBlockRouteCandidate{
		NetName:     net.Name,
		Status:      InterBlockRouteCandidateRoutable,
		Endpoints:   endpoints,
		InstanceIDs: sortedStringSet(instanceSet),
		BlockIDs:    sortedStringSet(blockSet),
		Unresolved:  unresolved,
	}, issues, true
}

func indexPhysicalPadEndpointsByNet(components []placement.Component, componentRefIndex map[string]BlockFragment) physicalPadEndpointsByNet {
	byNet := physicalPadEndpointsByNet{}
	for _, component := range components {
		ref := strings.ToUpper(strings.TrimSpace(component.Ref))
		fragment, ok := componentRefIndex[ref]
		if !ok {
			continue
		}
		routingNames := routingPadNames(component.Pads)
		for padIndex, pad := range component.Pads {
			netKey := strings.ToUpper(strings.TrimSpace(pad.Net))
			pin := routingNames[padIndex]
			if netKey == "" || pin == "" {
				continue
			}
			byNet[netKey] = append(byNet[netKey], InterBlockRouteEndpoint{
				Ref:        strings.TrimSpace(component.Ref),
				Pin:        pin,
				InstanceID: fragment.InstanceID,
				BlockID:    fragment.BlockID,
			})
		}
	}
	return byNet
}

func appendParticipatingPhysicalPadEndpoints(endpoints []InterBlockRouteEndpoint, physicalEndpoints []InterBlockRouteEndpoint) []InterBlockRouteEndpoint {
	participatingRefs := map[string]struct{}{}
	seen := map[string]struct{}{}
	for _, endpoint := range endpoints {
		ref := strings.ToUpper(strings.TrimSpace(endpoint.Ref))
		if ref != "" {
			participatingRefs[ref] = struct{}{}
		}
		seen[normalizedRouteGroupEndpointKey(endpoint.Ref, endpoint.Pin)] = struct{}{}
	}
	for _, endpoint := range physicalEndpoints {
		ref := strings.ToUpper(strings.TrimSpace(endpoint.Ref))
		if _, participates := participatingRefs[ref]; !participates {
			continue
		}
		key := normalizedRouteGroupEndpointKey(endpoint.Ref, endpoint.Pin)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		endpoints = append(endpoints, endpoint)
	}
	return endpoints
}

func pruneLocalRouteInternalEndpoints(netName string, endpoints []InterBlockRouteEndpoint, componentRefIndex map[string]BlockFragment) []InterBlockRouteEndpoint {
	if len(endpoints) < 3 {
		return endpoints
	}
	islandByEndpoint := map[string]string{}
	seenInstances := map[string]struct{}{}
	for _, fragment := range componentRefIndex {
		if _, seen := seenInstances[fragment.InstanceID]; seen {
			continue
		}
		seenInstances[fragment.InstanceID] = struct{}{}
		for islandIndex, island := range localRouteEndpointIslands(netName, fragment) {
			islandKey := fmt.Sprintf("%s:%d", fragment.InstanceID, islandIndex)
			for _, endpointKey := range island {
				islandByEndpoint[fragment.InstanceID+"|"+endpointKey] = islandKey
			}
		}
	}
	if len(islandByEndpoint) == 0 {
		return endpoints
	}
	pruned := make([]InterBlockRouteEndpoint, 0, len(endpoints))
	keptIslands := map[string]struct{}{}
	for _, endpoint := range endpoints {
		endpointKey := endpoint.InstanceID + "|" + normalizedRouteGroupEndpointKey(endpoint.Ref, endpoint.Pin)
		islandKey, inIsland := islandByEndpoint[endpointKey]
		if !inIsland {
			pruned = append(pruned, endpoint)
			continue
		}
		if _, kept := keptIslands[islandKey]; kept {
			continue
		}
		pruned = append(pruned, endpoint)
		keptIslands[islandKey] = struct{}{}
	}
	return pruned
}

func localRouteEndpointIslands(netName string, fragment BlockFragment) [][]string {
	parent := map[string]string{}
	find := func(key string) string {
		if parent[key] == "" {
			parent[key] = key
			return key
		}
		root := key
		for parent[root] != root {
			root = parent[root]
		}
		for parent[key] != key {
			next := parent[key]
			parent[key] = root
			key = next
		}
		return root
	}
	union := func(a string, b string) {
		rootA := find(a)
		rootB := find(b)
		if rootA != rootB {
			parent[rootB] = rootA
		}
	}
	for _, route := range fragment.Realization.LocalRoutes {
		if !strings.EqualFold(strings.TrimSpace(route.NetName), strings.TrimSpace(netName)) {
			continue
		}
		fromKey := localRouteEndpointKey(route.From)
		toKey := localRouteEndpointKey(route.To)
		if fromKey == "" || toKey == "" {
			continue
		}
		union(fromKey, toKey)
	}
	if len(parent) == 0 {
		return nil
	}
	byRoot := map[string][]string{}
	for key := range parent {
		root := find(key)
		byRoot[root] = append(byRoot[root], key)
	}
	roots := make([]string, 0, len(byRoot))
	for root := range byRoot {
		roots = append(roots, root)
	}
	slices.Sort(roots)
	islands := make([][]string, 0, len(roots))
	for _, root := range roots {
		island := byRoot[root]
		slices.Sort(island)
		islands = append(islands, island)
	}
	return islands
}

func localRouteEndpointKey(endpoint transactions.Endpoint) string {
	ref := strings.TrimSpace(endpoint.Ref)
	pin := strings.TrimSpace(endpoint.Pin)
	if ref == "" || pin == "" {
		return ""
	}
	return normalizedRouteGroupEndpointKey(ref, pin)
}

func localRouteEndpointMatches(routeEndpoint transactions.Endpoint, endpoint InterBlockRouteEndpoint) bool {
	return strings.EqualFold(strings.TrimSpace(routeEndpoint.Ref), strings.TrimSpace(endpoint.Ref)) &&
		strings.TrimSpace(routeEndpoint.Pin) == strings.TrimSpace(endpoint.Pin)
}

func generatedComponentFragmentIndex(fragments PCBFragmentResult) map[string]BlockFragment {
	index := map[string]BlockFragment{}
	for _, fragment := range fragments.Fragments {
		for _, component := range fragment.Realization.Components {
			ref := strings.ToUpper(strings.TrimSpace(component.Ref))
			if ref != "" {
				index[ref] = fragment
			}
		}
	}
	return index
}

func sortedStringSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	slices.Sort(out)
	return out
}
