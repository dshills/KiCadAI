package designworkflow

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const routeEndpointSourcePlacedPad = "placement.pad_endpoint"

// Five millimeters keeps typical PCB footprints in a small number of buckets.
// Query correctness is independent of this tuning value because bounds expand
// across every intersecting bucket and then test each pad's physical extent.
const placedPadSpatialBucketMM = 5.0

type PlacedPadEndpoint struct {
	Ref               string                     `json:"ref"`
	FootprintID       string                     `json:"footprint_id,omitempty"`
	Pad               string                     `json:"pad"`
	NetName           string                     `json:"net_name,omitempty"`
	NetCode           int                        `json:"net_code,omitempty"`
	NetCodeResolved   bool                       `json:"net_code_resolved"`
	Point             transactions.Point         `json:"point"`
	Layer             string                     `json:"layer,omitempty"`
	Layers            []string                   `json:"layers,omitempty"`
	ComponentAt       transactions.Point         `json:"component_at"`
	ComponentRotation float64                    `json:"component_rotation_deg,omitempty"`
	PadOffset         transactions.Point         `json:"pad_offset"`
	PadWidthMM        float64                    `json:"pad_width_mm,omitempty"`
	PadHeightMM       float64                    `json:"pad_height_mm,omitempty"`
	PadShape          string                     `json:"pad_shape,omitempty"`
	Source            string                     `json:"source,omitempty"`
	Confidence        PhysicalEndpointConfidence `json:"confidence,omitempty"`
}

type PlacedPadEndpointResolver struct {
	endpoints      map[routeEndpointMapKey]PlacedPadEndpoint
	sorted         []PlacedPadEndpoint
	spatial        map[placedPadSpatialKey][]int
	maxPadRadiusMM float64
	issues         []reports.Issue
}

type placedPadSpatialKey struct {
	x int64
	y int64
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
		routingNames := routingPadNames(component.Pads)
		for padIndex, pad := range component.Pads {
			endpoint, issue, ok := placedPadEndpoint(component, position, pad, table, placed.Request.Board.Layers)
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
			key := routeEndpointKeyNormalized(refKey, strings.ToUpper(routingNames[padIndex]))
			if existing, exists := resolver.endpoints[key]; exists {
				if samePlacedPadEndpointNet(existing, endpoint) {
					continue
				}
				resolver.issues = append(resolver.issues, routeEndpointIssue("refs."+ref+".pads."+endpoint.Pad, fmt.Sprintf("duplicate normalized pad endpoint conflicts with %s.%s", existing.Ref, existing.Pad), []string{ref, existing.Ref}))
				continue
			}
			resolver.endpoints[key] = endpoint
		}
	}
	resolver.sorted = sortedPlacedPadEndpoints(resolver.endpoints)
	resolver.spatial = make(map[placedPadSpatialKey][]int, len(resolver.sorted))
	for index, endpoint := range resolver.sorted {
		key := placedPadSpatialKeyForPoint(endpoint.Point)
		resolver.spatial[key] = append(resolver.spatial[key], index)
		resolver.maxPadRadiusMM = math.Max(resolver.maxPadRadiusMM, math.Hypot(math.Max(0, endpoint.PadWidthMM), math.Max(0, endpoint.PadHeightMM))/2)
	}
	return resolver
}

func samePlacedPadEndpointNet(first, second PlacedPadEndpoint) bool {
	return strings.TrimSpace(first.NetName) != "" && strings.TrimSpace(first.NetName) == strings.TrimSpace(second.NetName)
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

func (resolver PlacedPadEndpointResolver) MaximumPadRadiusMM() float64 {
	return resolver.maxPadRadiusMM
}

func (resolver PlacedPadEndpointResolver) EndpointsWithinBounds(minX, minY, maxX, maxY float64) []PlacedPadEndpoint {
	if minX > maxX || minY > maxY {
		return nil
	}
	if len(resolver.spatial) == 0 {
		out := make([]PlacedPadEndpoint, 0)
		for _, endpoint := range resolver.sorted {
			// Preserve zero-value resolver compatibility for focused synthetic
			// tests by checking each pad's own extent when no index was built.
			radius := math.Hypot(math.Max(0, endpoint.PadWidthMM), math.Max(0, endpoint.PadHeightMM)) / 2
			if endpoint.Point.XMM+radius >= minX && endpoint.Point.XMM-radius <= maxX && endpoint.Point.YMM+radius >= minY && endpoint.Point.YMM-radius <= maxY {
				out = append(out, endpoint)
			}
		}
		return out
	}
	// Pad centers outside the requested bounds can still overlap them. Expand
	// the indexed lookup by the largest pad extent, then apply each pad's own
	// conservative extent to the final overlap check.
	minKey := placedPadSpatialKeyForPoint(transactions.Point{XMM: minX - resolver.maxPadRadiusMM, YMM: minY - resolver.maxPadRadiusMM})
	maxKey := placedPadSpatialKeyForPoint(transactions.Point{XMM: maxX + resolver.maxPadRadiusMM, YMM: maxY + resolver.maxPadRadiusMM})
	indices := make([]int, 0)
	for x := minKey.x; x <= maxKey.x; x++ {
		for y := minKey.y; y <= maxKey.y; y++ {
			for _, index := range resolver.spatial[placedPadSpatialKey{x: x, y: y}] {
				endpoint := resolver.sorted[index]
				radius := math.Hypot(math.Max(0, endpoint.PadWidthMM), math.Max(0, endpoint.PadHeightMM)) / 2
				if endpoint.Point.XMM+radius >= minX && endpoint.Point.XMM-radius <= maxX && endpoint.Point.YMM+radius >= minY && endpoint.Point.YMM-radius <= maxY {
					indices = append(indices, index)
				}
			}
		}
	}
	sort.Ints(indices)
	out := make([]PlacedPadEndpoint, 0, len(indices))
	for _, index := range indices {
		out = append(out, resolver.sorted[index])
	}
	return out
}

func placedPadSpatialKeyForPoint(point transactions.Point) placedPadSpatialKey {
	return placedPadSpatialKey{
		x: int64(math.Floor(point.XMM / placedPadSpatialBucketMM)),
		y: int64(math.Floor(point.YMM / placedPadSpatialBucketMM)),
	}
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

func placedPadEndpoint(component *placement.Component, position placement.Placement, pad placement.PadSummary, table GeneratedNetTable, boardLayerCount int) (PlacedPadEndpoint, *reports.Issue, bool) {
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
	layers := placedPadCopperLayers(pad, layer, boardLayerCount)
	return PlacedPadEndpoint{
		Ref:               ref,
		FootprintID:       strings.TrimSpace(component.FootprintID),
		Pad:               padName,
		NetName:           netName,
		NetCode:           netCode,
		NetCodeResolved:   netName == "" || ok,
		Point:             transactions.Point{XMM: point.XMM, YMM: point.YMM},
		Layer:             layer,
		Layers:            layers,
		ComponentAt:       transactions.Point{XMM: position.XMM, YMM: position.YMM},
		ComponentRotation: position.RotationDeg,
		PadOffset:         transactions.Point{XMM: pad.XMM, YMM: pad.YMM},
		PadWidthMM:        pad.WidthMM,
		PadHeightMM:       pad.HeightMM,
		PadShape:          strings.TrimSpace(pad.Shape),
		Source:            routeEndpointSourcePlacedPad,
		Confidence:        endpointConfidence(netName, component.Role),
	}, issue, true
}

func placedPadCopperLayers(pad placement.PadSummary, placementLayer string, boardLayerCount int) []string {
	typeKey := strings.ToLower(strings.TrimSpace(pad.Type))
	spansCopper := pad.DrillMM > 0 ||
		strings.Contains(typeKey, "thru") ||
		strings.Contains(typeKey, "through") ||
		typeKey == "tht"
	for _, layer := range pad.Layers {
		normalized := strings.ToLower(strings.TrimSpace(layer))
		if normalized == "*.cu" || normalized == "all.cu" {
			spansCopper = true
			break
		}
	}
	if spansCopper {
		if boardLayerCount < 2 {
			boardLayerCount = 2
		}
		layers := make([]string, 0, boardLayerCount)
		layers = append(layers, "F.Cu")
		for index := 1; index <= boardLayerCount-2; index++ {
			layers = append(layers, fmt.Sprintf("In%d.Cu", index))
		}
		return append(layers, "B.Cu")
	}
	return []string{firstNonEmpty(placementLayer, "F.Cu")}
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
