package designworkflow

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const physicalEndpointSourcePlacementPad = "placement.pad_summary"
const physicalEndpointSourceExternalRequest = "request.external_endpoints"
const physicalEndpointSourceEdgePad = "placement.edge_pad"
const defaultDerivedBoardEdgeEndpointThresholdMM = 1.5

type PhysicalEndpointDiscoveryOptions struct {
	ExternalEndpoints []ExternalEndpointSpec
	Board             BoardSpec
}

func DiscoverPhysicalEndpoints(placed PlacementStageResult) ([]PhysicalEndpoint, []reports.Issue) {
	return DiscoverPhysicalEndpointsWithOptions(placed, PhysicalEndpointDiscoveryOptions{})
}

func DiscoverPhysicalEndpointsWithOptions(placed PlacementStageResult, opts PhysicalEndpointDiscoveryOptions) ([]PhysicalEndpoint, []reports.Issue) {
	endpoints, issues := explicitPhysicalEndpoints(opts.ExternalEndpoints, opts.Board)
	seenEndpointIDs := physicalEndpointIDSet(endpoints)
	if placed.Stage.Status == StageStatusBlocked || reports.HasBlockingIssue(placed.Stage.Issues) {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityWarning,
			Path:     "anchor_bindings.endpoints",
			Message:  "physical endpoint discovery skipped because placement did not complete",
		})
		return endpoints, issues
	}
	positions := placementPositions(placed)
	netRoles := placementNetRoles(placed.Request.Nets)
	frame := endpointBoardFrame(placed.Request, opts.Board)
	edgeThresholdMM := derivedBoardEdgeEndpointThreshold(placed.Request, opts.Board)
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
		padNameCounts := componentPadNameCounts(component.Pads)
		reportedDuplicatePadWarning := map[string]struct{}{}
		if edgeFacingComponent(component.Edge) && (frame.WidthMM <= 0 || frame.HeightMM <= 0) {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityWarning,
				Path:     "anchor_bindings.endpoints." + ref + ".edge",
				Message:  "board-edge endpoint derivation skipped because board width and height must be positive",
				Refs:     []string{ref},
			})
		}
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
			if endpoint.ID != "" {
				if _, exists := seenEndpointIDs[endpoint.ID]; exists {
					issues = append(issues, reports.Issue{
						Code:     reports.CodeInvalidArgument,
						Severity: reports.SeverityWarning,
						Path:     "anchor_bindings.endpoints." + ref + "." + padName,
						Message:  "physical endpoint skipped because ID " + endpoint.ID + " is already in use",
						Refs:     []string{ref},
					})
					continue
				}
				seenEndpointIDs[endpoint.ID] = struct{}{}
			}
			endpoints = append(endpoints, endpoint)
			if padNameCounts[padName] > 1 {
				if _, reported := reportedDuplicatePadWarning[padName]; reported {
					continue
				}
				reportedDuplicatePadWarning[padName] = struct{}{}
			}
			derived, deriveIssue, ok := derivedBoardEdgeEndpoint(component, endpoint, padNameCounts[padName], frame, edgeThresholdMM)
			if deriveIssue != nil {
				issues = append(issues, *deriveIssue)
			}
			if !ok {
				continue
			}
			if derived.ID != "" {
				if _, exists := seenEndpointIDs[derived.ID]; exists {
					issues = append(issues, reports.Issue{
						Code:     reports.CodeInvalidArgument,
						Severity: reports.SeverityWarning,
						Path:     "anchor_bindings.endpoints." + ref + "." + padName + ".edge",
						Message:  "derived board-edge endpoint skipped because ID " + derived.ID + " is already in use",
						Refs:     []string{ref},
					})
					continue
				}
				seenEndpointIDs[derived.ID] = struct{}{}
			}
			endpoints = append(endpoints, derived)
		}
	}
	sortPhysicalEndpoints(endpoints)
	return endpoints, issues
}

func explicitPhysicalEndpoints(specs []ExternalEndpointSpec, board BoardSpec) ([]PhysicalEndpoint, []reports.Issue) {
	if len(specs) == 0 {
		return []PhysicalEndpoint{}, nil
	}
	seen := map[string]struct{}{}
	endpoints := make([]PhysicalEndpoint, 0, len(specs))
	var issues []reports.Issue
	for _, spec := range normalizeExternalEndpoints(specs) {
		endpoint := PhysicalEndpoint{
			ID:         spec.ID,
			Kind:       spec.Kind,
			NetName:    spec.NetName,
			Layers:     append([]string(nil), spec.Layers...),
			Roles:      append([]string(nil), spec.Roles...),
			Source:     firstNonEmpty(spec.Source, physicalEndpointSourceExternalRequest),
			Confidence: spec.Confidence,
		}
		if spec.Point != nil {
			point := boardRelativeEndpointPoint(*spec.Point, board)
			endpoint.Point = &point
		}
		if endpoint.ID != "" {
			if _, exists := seen[endpoint.ID]; exists {
				issues = append(issues, reports.Issue{
					Code:     reports.CodeInvalidArgument,
					Severity: reports.SeverityWarning,
					Path:     "anchor_bindings.endpoints.external_endpoints." + endpoint.ID,
					Message:  "explicit physical endpoint skipped because ID " + endpoint.ID + " is already in use",
				})
				continue
			}
			seen[endpoint.ID] = struct{}{}
		}
		endpoints = append(endpoints, endpoint)
	}
	return endpoints, issues
}

func physicalEndpointIDSet(endpoints []PhysicalEndpoint) map[string]struct{} {
	seen := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.ID != "" {
			seen[endpoint.ID] = struct{}{}
		}
	}
	return seen
}

func boardRelativeEndpointPoint(point transactions.Point, board BoardSpec) transactions.Point {
	if point.XMM < 0 && point.XMM >= -anchorBindingGeometryEpsilonMM {
		point.XMM = 0
	}
	if point.YMM < 0 && point.YMM >= -anchorBindingGeometryEpsilonMM {
		point.YMM = 0
	}
	if board.WidthMM > 0 && point.XMM > board.WidthMM && point.XMM <= board.WidthMM+anchorBindingGeometryEpsilonMM {
		point.XMM = board.WidthMM
	}
	if board.HeightMM > 0 && point.YMM > board.HeightMM && point.YMM <= board.HeightMM+anchorBindingGeometryEpsilonMM {
		point.YMM = board.HeightMM
	}
	return point
}

type endpointFrame struct {
	OriginXMM float64
	OriginYMM float64
	WidthMM   float64
	HeightMM  float64
}

func endpointBoardFrame(request placement.Request, board BoardSpec) endpointFrame {
	frame := endpointFrame{
		OriginXMM: request.Board.Origin.XMM,
		OriginYMM: request.Board.Origin.YMM,
		WidthMM:   request.Board.WidthMM,
		HeightMM:  request.Board.HeightMM,
	}
	if board.WidthMM > 0 {
		frame.WidthMM = board.WidthMM
	}
	if board.HeightMM > 0 {
		frame.HeightMM = board.HeightMM
	}
	return frame
}

func (frame endpointFrame) toBoardRelative(point placement.Point) transactions.Point {
	return transactions.Point{XMM: point.XMM - frame.OriginXMM, YMM: point.YMM - frame.OriginYMM}
}

func (frame endpointFrame) toGlobal(point transactions.Point) transactions.Point {
	return transactions.Point{XMM: point.XMM + frame.OriginXMM, YMM: point.YMM + frame.OriginYMM}
}

func derivedBoardEdgeEndpointThreshold(request placement.Request, board BoardSpec) float64 {
	threshold := defaultDerivedBoardEdgeEndpointThresholdMM
	if request.Rules.BoardEdgeClearanceMM > threshold {
		threshold = request.Rules.BoardEdgeClearanceMM
	}
	if board.EdgeClearanceMM > threshold {
		threshold = board.EdgeClearanceMM
	}
	return threshold
}

func componentPadNameCounts(pads []placement.PadSummary) map[string]int {
	counts := map[string]int{}
	for _, pad := range pads {
		name := strings.TrimSpace(pad.Name)
		if name != "" {
			counts[name]++
		}
	}
	return counts
}

func derivedBoardEdgeEndpoint(component placement.Component, padEndpoint PhysicalEndpoint, padNameCount int, frame endpointFrame, thresholdMM float64) (PhysicalEndpoint, *reports.Issue, bool) {
	if !edgeFacingComponent(component.Edge) || padEndpoint.Point == nil || frame.WidthMM <= 0 || frame.HeightMM <= 0 {
		return PhysicalEndpoint{}, nil, false
	}
	if padNameCount > 1 {
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityWarning,
			Path:     "anchor_bindings.endpoints." + padEndpoint.Ref + "." + padEndpoint.Pad + ".edge",
			Message:  "derived board-edge endpoint skipped for duplicate pad name without durable pad discriminator",
			Refs:     []string{padEndpoint.Ref},
		}
		return PhysicalEndpoint{}, &issue, false
	}
	boardPoint := frame.toBoardRelative(placement.Point{XMM: padEndpoint.Point.XMM, YMM: padEndpoint.Point.YMM})
	edge, distance, ok := nearestBoardEdge(boardPoint, frame, component.Edge)
	if !ok || distance > thresholdMM+anchorBindingGeometryEpsilonMM {
		return PhysicalEndpoint{}, nil, false
	}
	projected := projectPointToBoardEdge(boardPoint, frame, edge)
	globalProjected := frame.toGlobal(projected)
	endpoint := PhysicalEndpoint{
		ID:         derivedBoardEdgeEndpointID(padEndpoint.Ref, padEndpoint.Pad, ""),
		Kind:       PhysicalEndpointBoardEdgePoint,
		Ref:        padEndpoint.Ref,
		Pad:        padEndpoint.Pad,
		NetName:    padEndpoint.NetName,
		Layers:     append([]string(nil), padEndpoint.Layers...),
		Roles:      uniqueStrings(append(append([]string(nil), padEndpoint.Roles...), "edge", string(edge))),
		Point:      &globalProjected,
		Source:     physicalEndpointSourceEdgePad,
		Confidence: padEndpoint.Confidence,
	}
	return endpoint, nil, true
}

func projectPointToBoardEdge(point transactions.Point, frame endpointFrame, edge placement.EdgeConstraint) transactions.Point {
	switch edge {
	case placement.EdgeLeft:
		point.XMM = 0
	case placement.EdgeRight:
		point.XMM = frame.WidthMM
	case placement.EdgeTop:
		point.YMM = 0
	case placement.EdgeBottom:
		point.YMM = frame.HeightMM
	}
	return point
}

func edgeFacingComponent(edge placement.EdgeConstraint) bool {
	switch edge {
	case placement.EdgeAny, placement.EdgeLeft, placement.EdgeRight, placement.EdgeTop, placement.EdgeBottom:
		return true
	default:
		return false
	}
}

func nearestBoardEdge(point transactions.Point, frame endpointFrame, preferred placement.EdgeConstraint) (placement.EdgeConstraint, float64, bool) {
	distances := []struct {
		edge     placement.EdgeConstraint
		distance float64
	}{
		{edge: placement.EdgeLeft, distance: math.Abs(point.XMM)},
		{edge: placement.EdgeRight, distance: math.Abs(frame.WidthMM - point.XMM)},
		{edge: placement.EdgeTop, distance: math.Abs(point.YMM)},
		{edge: placement.EdgeBottom, distance: math.Abs(frame.HeightMM - point.YMM)},
	}
	best := distances[0]
	for _, candidate := range distances[1:] {
		if candidate.distance < best.distance {
			best = candidate
		}
	}
	if preferred != placement.EdgeNone && preferred != placement.EdgeAny {
		for _, candidate := range distances {
			if candidate.edge == preferred && math.Abs(candidate.distance-best.distance) <= anchorBindingGeometryEpsilonMM {
				return candidate.edge, candidate.distance, true
			}
		}
	}
	return best.edge, best.distance, true
}

func derivedBoardEdgeEndpointID(ref string, pad string, discriminator string) string {
	kind := string(PhysicalEndpointBoardEdgePoint)
	seed := lengthPrefixedEndpointField("kind", kind) +
		lengthPrefixedEndpointField("ref", strings.TrimSpace(ref)) +
		lengthPrefixedEndpointField("pad", strings.TrimSpace(pad)) +
		lengthPrefixedEndpointField("pad_discriminator", strings.TrimSpace(discriminator))
	sum := sha256.Sum256([]byte(seed))
	hash := hex.EncodeToString(sum[:])[:8]
	return fmt.Sprintf("%s:%s:%s:%s", kind, endpointIDText(ref), endpointIDText(pad), hash)
}

func lengthPrefixedEndpointField(name string, value string) string {
	return fmt.Sprintf("%s:%d:%s\n", name, len(value), value)
}

func endpointIDText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = externalEndpointIDPattern.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "unnamed"
	}
	return value
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
