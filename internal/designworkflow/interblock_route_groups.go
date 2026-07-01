package designworkflow

import (
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

type InterBlockRouteEndpointRequirement string

const (
	InterBlockRouteEndpointRequired InterBlockRouteEndpointRequirement = "required"
	InterBlockRouteEndpointOptional InterBlockRouteEndpointRequirement = "optional"
)

type InterBlockRouteGroup struct {
	NetName                string                         `json:"net_name"`
	Status                 InterBlockRouteStatus          `json:"status"`
	RequiredEndpoints      []InterBlockRouteGroupEndpoint `json:"required_endpoints,omitempty"`
	OptionalEndpoints      []InterBlockRouteGroupEndpoint `json:"optional_endpoints,omitempty"`
	InstanceIDs            []string                       `json:"instance_ids,omitempty"`
	BlockIDs               []string                       `json:"block_ids,omitempty"`
	SourceCandidateIndices []int                          `json:"source_candidate_indices,omitempty"`
	ExpectedRequired       int                            `json:"expected_required,omitempty"`
	UnresolvedRequired     int                            `json:"unresolved_required,omitempty"`
}

type InterBlockRouteGroupEndpoint struct {
	ID          string                             `json:"id"`
	Ref         string                             `json:"ref"`
	Pin         string                             `json:"pin"`
	InstanceID  string                             `json:"instance_id,omitempty"`
	BlockID     string                             `json:"block_id,omitempty"`
	Requirement InterBlockRouteEndpointRequirement `json:"requirement"`
	Source      string                             `json:"source,omitempty"`
	SortKey     string                             `json:"-"`
}

func BuildInterBlockRouteGroups(candidates []InterBlockRouteCandidate) ([]InterBlockRouteGroup, []reports.Issue) {
	groupIndex := map[string]int{}
	groups := []InterBlockRouteGroup{}
	endpointKeys := map[string]map[string]bool{}
	var issues []reports.Issue
	for candidateIndex, candidate := range candidates {
		netName := strings.TrimSpace(candidate.NetName)
		if netName == "" {
			issues = append(issues, reports.Issue{
				Code:       reports.CodeValidationFailed,
				Severity:   reports.SeverityError,
				Path:       fmt.Sprintf("design.inter_block_route_groups.candidates[%d].net_name", candidateIndex),
				Message:    "inter-block route candidate has no net name",
				Suggestion: "preserve canonical net name when building inter-block route candidates",
			})
			continue
		}
		groupOffset, ok := groupIndex[netName]
		if !ok {
			groupOffset = len(groups)
			groupIndex[netName] = groupOffset
			groups = append(groups, InterBlockRouteGroup{NetName: netName, Status: candidate.Status})
			endpointKeys[netName] = map[string]bool{}
		} else {
			groups[groupOffset].Status = mergeInterBlockRouteStatus(groups[groupOffset].Status, candidate.Status)
		}
		group := &groups[groupOffset]
		group.SourceCandidateIndices = append(group.SourceCandidateIndices, candidateIndex)
		group.InstanceIDs = append(group.InstanceIDs, candidate.InstanceIDs...)
		group.BlockIDs = append(group.BlockIDs, candidate.BlockIDs...)
		if expected := len(candidate.Endpoints) + candidate.Unresolved; expected > group.ExpectedRequired {
			group.ExpectedRequired = expected
		}
		for _, endpoint := range candidate.Endpoints {
			groupEndpoint := interBlockRouteGroupEndpoint(endpoint, InterBlockRouteEndpointRequired)
			if groupEndpoint.Ref == "" || groupEndpoint.Pin == "" {
				continue
			}
			key := routeGroupEndpointKey(groupEndpoint)
			if endpointKeys[netName][key] {
				continue
			}
			endpointKeys[netName][key] = true
			group.RequiredEndpoints = append(group.RequiredEndpoints, groupEndpoint)
		}
	}
	for index := range groups {
		sortInterBlockRouteGroup(&groups[index])
		groups[index].UnresolvedRequired = max(0, groups[index].ExpectedRequired-len(groups[index].RequiredEndpoints))
	}
	slices.SortFunc(groups, func(left, right InterBlockRouteGroup) int {
		return strings.Compare(left.NetName, right.NetName)
	})
	for _, group := range groups {
		if group.UnresolvedRequired > 0 {
			issues = append(issues, reports.Issue{
				Code:       reports.CodeValidationFailed,
				Severity:   reports.SeverityError,
				Path:       fmt.Sprintf("design.inter_block_route_groups[%q].unresolved_required", group.NetName),
				Message:    fmt.Sprintf("inter-block route group %s has %d unresolved required endpoint(s)", group.NetName, group.UnresolvedRequired),
				Nets:       []string{group.NetName},
				Suggestion: "resolve all required physical endpoints before multi-endpoint routing",
			})
		}
	}
	return groups, issues
}

func interBlockRouteGroupEndpoint(endpoint InterBlockRouteEndpoint, requirement InterBlockRouteEndpointRequirement) InterBlockRouteGroupEndpoint {
	ref := strings.TrimSpace(endpoint.Ref)
	pin := strings.TrimSpace(endpoint.Pin)
	sortKey := normalizedRouteGroupEndpointKey(ref, pin)
	return InterBlockRouteGroupEndpoint{
		ID:          sortKey,
		Ref:         ref,
		Pin:         pin,
		InstanceID:  strings.TrimSpace(endpoint.InstanceID),
		BlockID:     strings.TrimSpace(endpoint.BlockID),
		Requirement: requirement,
		Source:      "inter_block_candidate",
		SortKey:     sortKey,
	}
}

func routeGroupEndpointKey(endpoint InterBlockRouteGroupEndpoint) string {
	if endpoint.SortKey != "" {
		return endpoint.SortKey
	}
	return normalizedRouteGroupEndpointKey(endpoint.Ref, endpoint.Pin)
}

func normalizedRouteGroupEndpointKey(ref string, pin string) string {
	return strings.ToUpper(strings.TrimSpace(ref)) + "." + strings.ToUpper(strings.TrimSpace(pin))
}

func sortInterBlockRouteGroup(group *InterBlockRouteGroup) {
	if group == nil {
		return
	}
	group.InstanceIDs = compactSortedNonEmptyStrings(group.InstanceIDs)
	group.BlockIDs = compactSortedNonEmptyStrings(group.BlockIDs)
	slices.SortFunc(group.RequiredEndpoints, compareInterBlockRouteGroupEndpoint)
	slices.SortFunc(group.OptionalEndpoints, compareInterBlockRouteGroupEndpoint)
}

func mergeInterBlockRouteStatus(current InterBlockRouteStatus, next InterBlockRouteStatus) InterBlockRouteStatus {
	if next == "" {
		return current
	}
	if current == "" {
		return next
	}
	nextRank := interBlockRouteStatusRank(next)
	currentRank := interBlockRouteStatusRank(current)
	if nextRank > currentRank || nextRank == currentRank && next > current {
		return next
	}
	return current
}

func interBlockRouteStatusRank(status InterBlockRouteStatus) int {
	switch status {
	case "":
		return -1
	case InterBlockRouteCandidateRoutable:
		return 0
	case InterBlockRouteCandidatePartial:
		return 1
	case InterBlockRouteCandidateBlocked:
		return 2
	case InterBlockRouteCandidateFailed:
		return 3
	case InterBlockRouteCandidateError:
		return 4
	default:
		// Unknown non-routable statuses are treated as most severe and use a
		// lexical merge tie-breaker so output is deterministic independent of
		// candidate ordering.
		return 5
	}
}

func compactSortedNonEmptyStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func compareInterBlockRouteGroupEndpoint(left, right InterBlockRouteGroupEndpoint) int {
	return strings.Compare(routeGroupEndpointKey(left), routeGroupEndpointKey(right))
}
