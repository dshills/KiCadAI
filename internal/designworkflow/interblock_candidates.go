package designworkflow

import (
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
)

type InterBlockRouteStatus string

const (
	InterBlockRouteCandidateRoutable InterBlockRouteStatus = "routable"
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

func BuildInterBlockRouteCandidates(fragments PCBFragmentResult, placed PlacementStageResult) ([]InterBlockRouteCandidate, []reports.Issue) {
	refIndex := generatedComponentFragmentIndex(fragments)
	var candidates []InterBlockRouteCandidate
	var issues []reports.Issue
	for netIndex, net := range placed.Request.Nets {
		candidate, netIssues, ok := interBlockCandidateFromPlacementNet(netIndex, net, refIndex)
		issues = append(issues, netIssues...)
		if ok {
			candidates = append(candidates, candidate)
		}
	}
	return candidates, issues
}

func interBlockCandidateFromPlacementNet(index int, net placement.Net, refIndex map[string]BlockFragment) (InterBlockRouteCandidate, []reports.Issue, bool) {
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
		fragment, ok := refIndex[strings.ToUpper(ref)]
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
	return InterBlockRouteCandidate{
		NetName:     net.Name,
		Status:      InterBlockRouteCandidateRoutable,
		Endpoints:   endpoints,
		InstanceIDs: sortedStringSet(instanceSet),
		BlockIDs:    sortedStringSet(blockSet),
		Unresolved:  unresolved,
	}, issues, true
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
