package designworkflow

import (
	"math"
	"sort"
	"strings"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const physicalEndpointSourcePlacementPad = "placement.pad_summary"

func DiscoverPhysicalEndpoints(placed PlacementStageResult) ([]PhysicalEndpoint, []reports.Issue) {
	if placed.Stage.Status == StageStatusBlocked || reports.HasBlockingIssue(placed.Stage.Issues) {
		return nil, []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityWarning,
			Path:     "anchor_bindings.endpoints",
			Message:  "physical endpoint discovery skipped because placement did not complete",
		}}
	}
	positions := placementPositions(placed)
	netRoles := placementNetRoles(placed.Request.Nets)
	endpoints := []PhysicalEndpoint{}
	var issues []reports.Issue
	for _, component := range placed.Request.Components {
		ref := strings.TrimSpace(component.Ref)
		if ref == "" {
			continue
		}
		position, ok := positions[strings.ToUpper(ref)]
		if !ok {
			issues = append(issues, reports.Issue{
				Code:     reports.CodePlacementOutsideBoard,
				Severity: reports.SeverityWarning,
				Path:     "anchor_bindings.endpoints." + ref,
				Message:  "component has no placement for physical endpoint discovery",
				Refs:     []string{ref},
			})
			continue
		}
		layer := firstNonEmpty(position.Layer, "F.Cu")
		for _, pad := range component.Pads {
			padName := strings.TrimSpace(pad.Name)
			if padName == "" {
				issues = append(issues, reports.Issue{
					Code:     reports.CodeInvalidArgument,
					Severity: reports.SeverityWarning,
					Path:     "anchor_bindings.endpoints." + ref + ".pads",
					Message:  "unnamed pad skipped during physical endpoint discovery",
					Refs:     []string{ref},
				})
				continue
			}
			absolute := absolutePadPoint(position, pad)
			netName := strings.TrimSpace(pad.Net)
			endpoint := PhysicalEndpoint{
				ID:         physicalEndpointID(PhysicalEndpointFootprintPad, ref, padName),
				Kind:       PhysicalEndpointFootprintPad,
				Ref:        ref,
				Pad:        padName,
				NetName:    netName,
				Layers:     []string{layer},
				Roles:      endpointRoles(component.Role, netRoles[netName]),
				Point:      &transactions.Point{XMM: absolute.XMM, YMM: absolute.YMM},
				Source:     physicalEndpointSourcePlacementPad,
				Confidence: endpointConfidence(netName, component.Role),
			}
			endpoints = append(endpoints, endpoint)
		}
	}
	sortPhysicalEndpoints(endpoints)
	return endpoints, issues
}

func placementPositions(placed PlacementStageResult) map[string]placement.Placement {
	positions := map[string]placement.Placement{}
	for _, result := range placed.Result.Placements {
		ref := strings.ToUpper(strings.TrimSpace(result.Ref))
		if ref == "" || result.Reason != "" {
			continue
		}
		positions[ref] = result.Position
	}
	for _, component := range placed.Request.Components {
		ref := strings.ToUpper(strings.TrimSpace(component.Ref))
		if ref == "" {
			continue
		}
		if _, ok := positions[ref]; ok {
			continue
		}
		if component.Position != nil {
			positions[ref] = *component.Position
		}
	}
	return positions
}

func placementNetRoles(nets []placement.Net) map[string]placement.NetRole {
	roles := map[string]placement.NetRole{}
	for _, net := range nets {
		name := strings.TrimSpace(net.Name)
		if name != "" {
			roles[name] = net.Role
		}
	}
	return roles
}

func absolutePadPoint(position placement.Placement, pad placement.PadSummary) placement.Point {
	localX := pad.XMM
	if isBackCopperLayer(position.Layer) {
		localX = -localX
	}
	rotated := rotateEndpointPoint(placement.Point{XMM: localX, YMM: pad.YMM}, position.RotationDeg)
	return placement.Point{XMM: position.XMM + rotated.XMM, YMM: position.YMM + rotated.YMM}
}

func isBackCopperLayer(layer string) bool {
	return strings.EqualFold(strings.TrimSpace(layer), "B.Cu")
}

func rotateEndpointPoint(point placement.Point, rotationDeg float64) placement.Point {
	normalized := math.Mod(rotationDeg, 360)
	if normalized < 0 {
		normalized += 360
	}
	switch {
	case math.Abs(normalized) < 1e-9:
		return point
	case math.Abs(normalized-90) < 1e-9:
		return placement.Point{XMM: -point.YMM, YMM: point.XMM}
	case math.Abs(normalized-180) < 1e-9:
		return placement.Point{XMM: -point.XMM, YMM: -point.YMM}
	case math.Abs(normalized-270) < 1e-9:
		return placement.Point{XMM: point.YMM, YMM: -point.XMM}
	default:
		radians := normalized * math.Pi / 180
		cosTheta := math.Cos(radians)
		sinTheta := math.Sin(radians)
		return placement.Point{XMM: point.XMM*cosTheta - point.YMM*sinTheta, YMM: point.XMM*sinTheta + point.YMM*cosTheta}
	}
}

func endpointRoles(componentRole string, netRole placement.NetRole) []string {
	roles := []string{}
	componentRole = strings.TrimSpace(componentRole)
	if componentRole != "" {
		roles = append(roles, componentRole)
	}
	switch netRole {
	case placement.NetPower:
		roles = append(roles, "power")
	case placement.NetGround:
		roles = append(roles, "ground")
	case placement.NetSignal:
		roles = append(roles, "signal")
	case placement.NetClock:
		roles = append(roles, "clock")
	case placement.NetAnalog:
		roles = append(roles, "analog")
	case placement.NetDifferential:
		roles = append(roles, "differential")
	}
	return uniqueStrings(roles)
}

func endpointConfidence(netName string, componentRole string) PhysicalEndpointConfidence {
	if strings.TrimSpace(netName) != "" && strings.TrimSpace(componentRole) != "" {
		return PhysicalEndpointConfidenceHigh
	}
	if strings.TrimSpace(netName) != "" || strings.TrimSpace(componentRole) != "" {
		return PhysicalEndpointConfidenceMedium
	}
	return PhysicalEndpointConfidenceLow
}

func sortPhysicalEndpoints(endpoints []PhysicalEndpoint) {
	sort.SliceStable(endpoints, func(i, j int) bool {
		if endpoints[i].Ref != endpoints[j].Ref {
			return endpoints[i].Ref < endpoints[j].Ref
		}
		if endpoints[i].Pad != endpoints[j].Pad {
			return endpoints[i].Pad < endpoints[j].Pad
		}
		return endpoints[i].ID < endpoints[j].ID
	})
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
