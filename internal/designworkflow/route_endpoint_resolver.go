package designworkflow

import (
	"fmt"
	"sort"
	"strings"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const routeEndpointSourcePlacedPad = "placement.pad_endpoint"

type PlacedPadEndpoint struct {
	Ref               string                     `json:"ref"`
	FootprintID       string                     `json:"footprint_id,omitempty"`
	Pad               string                     `json:"pad"`
	NetName           string                     `json:"net_name,omitempty"`
	NetCode           int                        `json:"net_code,omitempty"`
	NetCodeResolved   bool                       `json:"net_code_resolved"`
	Point             transactions.Point         `json:"point"`
	Layer             string                     `json:"layer,omitempty"`
	ComponentAt       transactions.Point         `json:"component_at"`
	ComponentRotation float64                    `json:"component_rotation_deg,omitempty"`
	PadOffset         transactions.Point         `json:"pad_offset"`
	Source            string                     `json:"source,omitempty"`
	Confidence        PhysicalEndpointConfidence `json:"confidence,omitempty"`
}

type PlacedPadEndpointResolver struct {
	endpoints map[routeEndpointMapKey]PlacedPadEndpoint
	sorted    []PlacedPadEndpoint
	issues    []reports.Issue
}

func NewPlacedPadEndpointResolver(placed *PlacementStageResult, table GeneratedNetTable) PlacedPadEndpointResolver {
	resolver := PlacedPadEndpointResolver{endpoints: map[routeEndpointMapKey]PlacedPadEndpoint{}}
	if placed == nil {
		resolver.issues = append(resolver.issues, routeEndpointIssue("placement", "placement result is required for route endpoint resolution", nil))
		return resolver
	}
	if placed.Stage.Status == StageStatusBlocked || reports.HasBlockingIssue(placed.Stage.Issues) {
		resolver.issues = append(resolver.issues, routeEndpointIssue("placement", "placed pad endpoint resolution skipped because placement did not complete", nil))
		return resolver
	}
	positions := placementPositions(placed)
	for componentIndex := range placed.Request.Components {
		component := &placed.Request.Components[componentIndex]
		ref := strings.TrimSpace(component.Ref)
		if ref == "" {
			continue
		}
		refKey := strings.ToUpper(ref)
		position, ok := positions[refKey]
		if !ok {
			resolver.issues = append(resolver.issues, routeEndpointIssue("refs."+ref, "component has no final placement for route endpoint resolution", []string{ref}))
			continue
		}
		if len(component.Pads) == 0 {
			resolver.issues = append(resolver.issues, routeEndpointWarning("refs."+ref+".pads", "component has no hydrated pads for route endpoint resolution", []string{ref}))
			continue
		}
		reportedUnnamedPad := false
		for _, pad := range component.Pads {
			endpoint, issue, ok := placedPadEndpoint(component, position, pad, table)
			if !ok {
				if !reportedUnnamedPad {
					resolver.issues = append(resolver.issues, routeEndpointIssue("refs."+ref+".pads", "unnamed pad skipped during route endpoint resolution", []string{ref}))
					reportedUnnamedPad = true
				}
				continue
			}
			if issue != nil {
				resolver.issues = append(resolver.issues, *issue)
			}
			key := routeEndpointKeyNormalized(refKey, strings.ToUpper(endpoint.Pad))
			if existing, exists := resolver.endpoints[key]; exists {
				resolver.issues = append(resolver.issues, routeEndpointIssue("refs."+ref+".pads."+endpoint.Pad, fmt.Sprintf("duplicate normalized pad endpoint conflicts with %s.%s", existing.Ref, existing.Pad), []string{ref, existing.Ref}))
				continue
			}
			resolver.endpoints[key] = endpoint
		}
	}
	resolver.sorted = sortedPlacedPadEndpoints(resolver.endpoints)
	return resolver
}

func (resolver PlacedPadEndpointResolver) Issues() []reports.Issue {
	return cloneIssues(resolver.issues)
}

func (resolver PlacedPadEndpointResolver) Resolve(endpoint transactions.Endpoint) (PlacedPadEndpoint, bool) {
	resolved, ok := resolver.endpoints[routeEndpointKey(endpoint.Ref, endpoint.Pin)]
	return resolved, ok
}

func (resolver PlacedPadEndpointResolver) ResolveNormalized(ref string, pad string) (PlacedPadEndpoint, bool) {
	resolved, ok := resolver.endpoints[routeEndpointKeyNormalized(ref, pad)]
	return resolved, ok
}

func (resolver PlacedPadEndpointResolver) Endpoints() []PlacedPadEndpoint {
	return append([]PlacedPadEndpoint(nil), resolver.sorted...)
}

func sortedPlacedPadEndpoints(endpoints map[routeEndpointMapKey]PlacedPadEndpoint) []PlacedPadEndpoint {
	keys := make([]routeEndpointMapKey, 0, len(endpoints))
	for key := range endpoints {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ref != keys[j].ref {
			return keys[i].ref < keys[j].ref
		}
		return keys[i].pad < keys[j].pad
	})
	out := make([]PlacedPadEndpoint, 0, len(keys))
	for _, key := range keys {
		out = append(out, endpoints[key])
	}
	return out
}

func placedPadEndpoint(component *placement.Component, position placement.Placement, pad placement.PadSummary, table GeneratedNetTable) (PlacedPadEndpoint, *reports.Issue, bool) {
	ref := strings.TrimSpace(component.Ref)
	padName := strings.TrimSpace(pad.Name)
	if ref == "" || padName == "" {
		return PlacedPadEndpoint{}, nil, false
	}
	point := absolutePadPoint(position, pad)
	netName := strings.TrimSpace(pad.Net)
	netCode, ok := generatedNetCode(table, netName)
	var issue *reports.Issue
	if netName != "" && !ok {
		missing := routeEndpointIssue("refs."+ref+".pads."+padName+".net_code", "pad net "+netName+" is missing from the generated net table", []string{ref})
		issue = &missing
	}
	layer := firstNonEmpty(position.Layer, "F.Cu")
	return PlacedPadEndpoint{
		Ref:               ref,
		FootprintID:       strings.TrimSpace(component.FootprintID),
		Pad:               padName,
		NetName:           netName,
		NetCode:           netCode,
		NetCodeResolved:   netName == "" || ok,
		Point:             transactions.Point{XMM: point.XMM, YMM: point.YMM},
		Layer:             layer,
		ComponentAt:       transactions.Point{XMM: position.XMM, YMM: position.YMM},
		ComponentRotation: position.RotationDeg,
		PadOffset:         transactions.Point{XMM: pad.XMM, YMM: pad.YMM},
		Source:            routeEndpointSourcePlacedPad,
		Confidence:        endpointConfidence(netName, component.Role),
	}, issue, true
}

func routeEndpointIssue(path string, message string, refs []string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityBlocked,
		Path:     "design.route_connectivity." + strings.Trim(path, "."),
		Message:  message,
		Refs:     append([]string(nil), refs...),
	}
}

func routeEndpointWarning(path string, message string, refs []string) reports.Issue {
	issue := routeEndpointIssue(path, message, refs)
	issue.Severity = reports.SeverityWarning
	return issue
}

type routeEndpointMapKey struct {
	ref string
	pad string
}

func routeEndpointKey(ref string, pad string) routeEndpointMapKey {
	return routeEndpointKeyNormalized(strings.ToUpper(strings.TrimSpace(ref)), strings.ToUpper(strings.TrimSpace(pad)))
}

func routeEndpointKeyNormalized(ref string, pad string) routeEndpointMapKey {
	return routeEndpointMapKey{ref: ref, pad: pad}
}
