package designworkflow

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type AnchorBindingOptions struct {
	MaxProximityMM           float64
	RequiredBlockIDs         map[string]bool
	ExternalEndpointBlockIDs map[string]bool
}

type collectedEntryAnchor struct {
	BlockInstanceID          string
	BlockID                  string
	ID                       string
	Port                     string
	NetName                  string
	Point                    transactions.Point
	Layers                   []string
	Policy                   AnchorBindingPolicy
	ExternalEndpointRequired bool
}

const defaultAnchorBindingMaxProximityMM = 10

var defaultRequiredAnchorBindingBlockIDs = map[string]bool{"esd_protection": true, "reverse_polarity_protection": true}
var defaultExternalEndpointBlockIDs = map[string]bool{"esd_protection": true, "reverse_polarity_protection": true}

func ResolveAnchorBindings(fragments PCBFragmentResult, endpoints []PhysicalEndpoint, opts AnchorBindingOptions) AnchorBindingSummary {
	anchors := collectEntryAnchors(fragments, opts)
	maxDistance := opts.MaxProximityMM
	if maxDistance <= 0 {
		maxDistance = defaultAnchorBindingMaxProximityMM
	}
	endpointIndex := newPhysicalEndpointGrid(endpoints, maxDistance)
	bindings := make([]AnchorBinding, 0, len(anchors))
	issues := []AnchorBindingIssue{}
	for _, anchor := range anchors {
		binding, bindingIssues := resolveAnchorBinding(anchor, endpointIndex, maxDistance)
		bindings = append(bindings, binding)
		issues = append(issues, bindingIssues...)
	}
	sortAnchorBindings(bindings)
	sortAnchorBindingIssues(issues)
	return SummarizeAnchorBindings(bindings, issues)
}

func collectEntryAnchors(fragments PCBFragmentResult, opts AnchorBindingOptions) []collectedEntryAnchor {
	anchors := []collectedEntryAnchor{}
	for _, fragment := range fragments.Fragments {
		for _, anchor := range fragment.Realization.EntryAnchors {
			point := transactions.Point{XMM: anchor.Placement.XMM, YMM: anchor.Placement.YMM}
			layers := []string{firstNonEmpty(anchor.Placement.Layer, "F.Cu")}
			blockID := strings.TrimSpace(fragment.BlockID)
			anchors = append(anchors, collectedEntryAnchor{
				BlockInstanceID:          fragment.InstanceID,
				BlockID:                  blockID,
				ID:                       strings.TrimSpace(anchor.ID),
				Port:                     strings.TrimSpace(anchor.Port),
				NetName:                  strings.TrimSpace(anchor.NetName),
				Point:                    point,
				Layers:                   layers,
				Policy:                   defaultAnchorBindingPolicy(fragment, anchor, opts),
				ExternalEndpointRequired: externalEndpointRequiredForBlock(blockID, opts),
			})
		}
	}
	sort.SliceStable(anchors, func(i, j int) bool {
		if anchors[i].BlockInstanceID != anchors[j].BlockInstanceID {
			return anchors[i].BlockInstanceID < anchors[j].BlockInstanceID
		}
		return anchors[i].ID < anchors[j].ID
	})
	return anchors
}

func defaultAnchorBindingPolicy(fragment BlockFragment, anchor blocks.RealizedPCBEntryAnchor, opts AnchorBindingOptions) AnchorBindingPolicy {
	blockID := strings.ToLower(strings.TrimSpace(fragment.BlockID))
	if opts.RequiredBlockIDs == nil {
		return boolPolicy(defaultRequiredAnchorBindingBlockIDs[blockID])
	}
	return boolPolicy(opts.RequiredBlockIDs[blockID])
}

func resolveAnchorBinding(anchor collectedEntryAnchor, endpointIndex physicalEndpointGrid, maxDistanceMM float64) (AnchorBinding, []AnchorBindingIssue) {
	binding := AnchorBinding{
		ID:              anchor.BlockInstanceID + "." + anchor.ID,
		BlockInstanceID: anchor.BlockInstanceID,
		AnchorID:        anchor.ID,
		AnchorPort:      anchor.Port,
		AnchorNetName:   anchor.NetName,
		AnchorPoint:     &transactions.Point{XMM: anchor.Point.XMM, YMM: anchor.Point.YMM},
		AnchorLayers:    append([]string(nil), anchor.Layers...),
		Status:          AnchorBindingStatusUnbound,
		Required:        AnchorBindingRequired(false, anchor.Policy),
		Policy:          anchor.Policy,
		RouteStatus:     AnchorRouteStatusSkipped,
	}
	if anchor.ID == "" {
		issue := NewAnchorBindingIssue(AnchorBindingIssueMissingAnchor, reports.SeverityError, anchor.BlockInstanceID, anchor.ID, "", "entry anchor is missing an id", "fix the block PCB realization entry anchor id")
		binding.Status = AnchorBindingStatusInvalid
		binding.IssueIDs = append(binding.IssueIDs, issue.ID)
		return binding, []AnchorBindingIssue{issue}
	}
	nearbyEndpoints := endpointIndex.Near(anchor.Point)
	candidates := endpointCandidatesForAnchor(anchor, nearbyEndpoints, maxDistanceMM)
	switch len(candidates) {
	case 0:
		issue := missingEndpointIssue(anchor, nearbyEndpoints, maxDistanceMM)
		binding.IssueIDs = append(binding.IssueIDs, issue.ID)
		if issue.Category == AnchorBindingIssueNetMismatch {
			binding.Status = AnchorBindingStatusInvalid
		}
		return binding, []AnchorBindingIssue{issue}
	case 1:
		applyEndpointToBinding(&binding, candidates[0].endpoint, candidates[0].distanceMM)
		return binding, nil
	default:
		if equivalentEndpointCandidates(candidates) {
			selected := candidates[0]
			applyEndpointToBinding(&binding, selected.endpoint, selected.distanceMM)
			for _, candidate := range candidates[1:] {
				binding.EquivalentEndpointIDs = append(binding.EquivalentEndpointIDs, candidate.endpoint.ID)
			}
			issue := NewAnchorBindingIssue(AnchorBindingIssueEquivalentEndpointChosen, reports.SeverityInfo, anchor.BlockInstanceID, anchor.ID, selected.endpoint.ID, "selected nearest equivalent endpoint for shared-net interface", "review equivalent endpoint list if current path or return-path intent requires a specific pad")
			binding.IssueIDs = append(binding.IssueIDs, issue.ID)
			return binding, []AnchorBindingIssue{issue}
		}
		severity := reports.SeverityWarning
		if AnchorBindingRequired(false, anchor.Policy) {
			severity = reports.SeverityError
		}
		issue := NewAnchorBindingIssue(AnchorBindingIssueAmbiguousEndpoint, severity, anchor.BlockInstanceID, anchor.ID, "", "multiple physical endpoints match entry anchor "+anchor.ID, "add explicit connector pad or interface-group intent for this anchor")
		binding.Status = AnchorBindingStatusAmbiguous
		binding.IssueIDs = append(binding.IssueIDs, issue.ID)
		return binding, []AnchorBindingIssue{issue}
	}
}

type endpointCandidate struct {
	endpoint   PhysicalEndpoint
	distanceMM float64
}

type physicalEndpointGrid struct {
	cellSizeMM float64
	cells      map[gridCell][]PhysicalEndpoint
}

type gridCell struct {
	x int
	y int
}

func newPhysicalEndpointGrid(endpoints []PhysicalEndpoint, cellSizeMM float64) physicalEndpointGrid {
	if cellSizeMM <= 0 {
		cellSizeMM = defaultAnchorBindingMaxProximityMM
	}
	index := physicalEndpointGrid{cellSizeMM: cellSizeMM, cells: map[gridCell][]PhysicalEndpoint{}}
	for _, endpoint := range endpoints {
		if endpoint.Point == nil {
			continue
		}
		cell := index.cellForPoint(*endpoint.Point)
		index.cells[cell] = append(index.cells[cell], endpoint)
	}
	return index
}

func (index physicalEndpointGrid) Near(point transactions.Point) []PhysicalEndpoint {
	if len(index.cells) == 0 {
		return nil
	}
	center := index.cellForPoint(point)
	out := []PhysicalEndpoint{}
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			cell := gridCell{x: center.x + dx, y: center.y + dy}
			out = append(out, index.cells[cell]...)
		}
	}
	return out
}

func (index physicalEndpointGrid) cellForPoint(point transactions.Point) gridCell {
	return gridCell{x: int(math.Floor(point.XMM / index.cellSizeMM)), y: int(math.Floor(point.YMM / index.cellSizeMM))}
}

func endpointCandidatesForAnchor(anchor collectedEntryAnchor, endpoints []PhysicalEndpoint, maxDistanceMM float64) []endpointCandidate {
	candidates := []endpointCandidate{}
	externalEndpointRequired := requiresExternalEndpoint(anchor)
	for _, endpoint := range endpoints {
		if endpoint.Point == nil {
			continue
		}
		distance := pointDistanceMM(anchor.Point, *endpoint.Point)
		if distance > maxDistanceMM {
			continue
		}
		if !netNamesMatch(anchor.NetName, endpoint.NetName) {
			continue
		}
		if !layersCompatible(anchor.Layers, endpoint.Layers) {
			continue
		}
		if externalEndpointRequired && !endpointLooksExternal(endpoint) {
			continue
		}
		candidates = append(candidates, endpointCandidate{endpoint: endpoint, distanceMM: distance})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if math.Abs(candidates[i].distanceMM-candidates[j].distanceMM) > 1e-9 {
			return candidates[i].distanceMM < candidates[j].distanceMM
		}
		return candidates[i].endpoint.ID < candidates[j].endpoint.ID
	})
	return candidates
}

func requiresExternalEndpoint(anchor collectedEntryAnchor) bool {
	return anchor.ExternalEndpointRequired
}

func externalEndpointRequiredForBlock(blockID string, opts AnchorBindingOptions) bool {
	blockID = strings.ToLower(strings.TrimSpace(blockID))
	if opts.ExternalEndpointBlockIDs == nil {
		return defaultExternalEndpointBlockIDs[blockID]
	}
	return opts.ExternalEndpointBlockIDs[blockID]
}

func endpointHasRole(endpoint PhysicalEndpoint, role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	for _, candidate := range endpoint.Roles {
		if strings.ToLower(strings.TrimSpace(candidate)) == role {
			return true
		}
	}
	return false
}

func endpointLooksExternal(endpoint PhysicalEndpoint) bool {
	if endpoint.Kind == PhysicalEndpointBoardEdgePoint {
		return true
	}
	ref := strings.ToUpper(strings.TrimSpace(endpoint.Ref))
	if strings.HasPrefix(ref, "J") {
		return true
	}
	return endpointHasExternalRole(endpoint)
}

func endpointHasExternalRole(endpoint PhysicalEndpoint) bool {
	for _, role := range endpoint.Roles {
		switch strings.ToLower(strings.TrimSpace(role)) {
		case "connector", "edge", "external", "power_entry", "mechanical_interface", "castellated":
			return true
		}
	}
	return false
}

func missingEndpointIssue(anchor collectedEntryAnchor, endpoints []PhysicalEndpoint, maxDistanceMM float64) AnchorBindingIssue {
	severity := RequiredAnchorBindingIssueSeverity(false, anchor.Policy, AnchorBindingStatusUnbound, AnchorRouteStatusSkipped)
	if hasEndpointNetMismatch(anchor, endpoints, maxDistanceMM) {
		severity = RequiredAnchorBindingIssueSeverity(false, anchor.Policy, AnchorBindingStatusInvalid, AnchorRouteStatusSkipped)
		return NewAnchorBindingIssue(AnchorBindingIssueNetMismatch, severity, anchor.BlockInstanceID, anchor.ID, "", "no physical endpoint shares required net "+anchor.NetName, "connect or rename the external endpoint net so it matches the anchor net")
	}
	return NewAnchorBindingIssue(AnchorBindingIssueMissingEndpoint, severity, anchor.BlockInstanceID, anchor.ID, "", "no physical endpoint found for entry anchor "+anchor.ID, "add a connector pad, board-edge point, or explicit endpoint binding")
}

func hasEndpointNetMismatch(anchor collectedEntryAnchor, endpoints []PhysicalEndpoint, maxDistanceMM float64) bool {
	if strings.TrimSpace(anchor.NetName) == "" {
		return false
	}
	for _, endpoint := range endpoints {
		if endpoint.Point == nil || !layersCompatible(anchor.Layers, endpoint.Layers) {
			continue
		}
		if pointDistanceMM(anchor.Point, *endpoint.Point) > maxDistanceMM {
			continue
		}
		if strings.TrimSpace(endpoint.NetName) != "" && !netNamesMatch(anchor.NetName, endpoint.NetName) {
			return true
		}
	}
	return false
}

func applyEndpointToBinding(binding *AnchorBinding, endpoint PhysicalEndpoint, distanceMM float64) {
	binding.EndpointID = endpoint.ID
	binding.EndpointKind = endpoint.Kind
	binding.EndpointRef = endpoint.Ref
	binding.EndpointPad = endpoint.Pad
	binding.EndpointNetName = endpoint.NetName
	binding.EndpointLayers = append([]string(nil), endpoint.Layers...)
	if endpoint.Point != nil {
		point := *endpoint.Point
		binding.EndpointPoint = &point
	}
	binding.DistanceMM = distanceMM
	binding.Status = AnchorBindingStatusBound
	binding.RouteStatus = AnchorRouteStatusSkipped
}

func equivalentEndpointCandidates(candidates []endpointCandidate) bool {
	if len(candidates) <= 1 {
		return true
	}
	first := candidates[0].endpoint
	for _, candidate := range candidates[1:] {
		endpoint := candidate.endpoint
		if !strings.EqualFold(first.Ref, endpoint.Ref) || !netNamesMatch(first.NetName, endpoint.NetName) || !sameStringSet(first.Layers, endpoint.Layers) {
			return false
		}
	}
	return true
}

func netNamesMatch(left string, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	return left == right
}

func boolPolicy(required bool) AnchorBindingPolicy {
	if required {
		return AnchorBindingPolicyRequired
	}
	return AnchorBindingPolicyOptional
}

func layersCompatible(anchorLayers []string, endpointLayers []string) bool {
	if len(anchorLayers) == 0 || len(endpointLayers) == 0 {
		return true
	}
	for _, left := range anchorLayers {
		for _, right := range endpointLayers {
			if strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right)) {
				return true
			}
		}
	}
	return false
}

func sameStringSet(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := map[string]int{}
	for _, value := range left {
		seen[strings.ToLower(strings.TrimSpace(value))]++
	}
	for _, value := range right {
		key := strings.ToLower(strings.TrimSpace(value))
		seen[key]--
		if seen[key] < 0 {
			return false
		}
	}
	return true
}

func pointDistanceMM(left transactions.Point, right transactions.Point) float64 {
	dx := left.XMM - right.XMM
	dy := left.YMM - right.YMM
	return math.Sqrt(dx*dx + dy*dy)
}

func anchorBindingSummaryStage(summary AnchorBindingSummary) StageResult {
	issues := AnchorBindingIssuesToReports("anchor_bindings", summary.Issues)
	stage := NewStageResult(StageFeedback, issues)
	stage.Summary = map[string]any{"anchor_bindings": summary}
	if summary.BlockingIssues > 0 {
		stage.Status = StageStatusBlocked
	}
	return stage
}

func formatAnchorEndpoint(endpoint PhysicalEndpoint) string {
	if endpoint.Ref == "" && endpoint.Pad == "" {
		return endpoint.ID
	}
	return fmt.Sprintf("%s.%s", endpoint.Ref, endpoint.Pad)
}
