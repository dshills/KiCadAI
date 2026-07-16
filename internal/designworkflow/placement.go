package designworkflow

import (
	"context"
	"math"
	"slices"
	"strings"
	"unicode"

	"kicadai/internal/blocks"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
)

var defaultWorkflowBounds = placement.Bounds{WidthMM: 2.0, HeightMM: 1.25, Source: placement.BoundsEstimated}

type PlacementOptions struct {
	DefaultBounds       placement.Bounds
	Rules               placement.Rules
	LibraryIndex        *libraryresolver.LibraryIndex
	ComponentSelections []ComponentSelectionEntry
}

type PlacementStageResult struct {
	Request placement.Request `json:"request"`
	Result  placement.Result  `json:"result"`
	Stage   StageResult       `json:"stage"`
}

func PlaceFragments(ctx context.Context, request Request, fragments PCBFragmentResult, opts PlacementOptions) PlacementStageResult {
	var issues []reports.Issue
	if ctx == nil {
		issues = append(issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "context", Message: "context is required"})
	} else if err := ctx.Err(); err != nil {
		issues = append(issues, reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityError, Path: "context", Message: err.Error()})
	}
	if reports.HasBlockingIssue(fragments.Stage.Issues) {
		return PlacementStageResult{Stage: StageResult{Name: StagePlacement, Status: StageStatusSkipped, Summary: map[string]any{"reason": "PCB realization did not complete"}}}
	}
	if reports.HasBlockingIssue(issues) {
		return PlacementStageResult{Stage: NewStageResult(StagePlacement, issues)}
	}
	normalized := NormalizeRequest(request)
	placementRequest := placement.Request{
		Board: placement.BoardPlacementArea{
			WidthMM:  normalized.Board.WidthMM,
			HeightMM: normalized.Board.HeightMM,
			MarginMM: normalized.Board.EdgeClearanceMM,
		},
		Rules: mergePlacementRules(opts.Rules),
		Seed:  normalized.Name,
	}
	if normalized.Board.EdgeClearanceMM > 0 {
		placementRequest.Rules.BoardEdgeClearanceMM = normalized.Board.EdgeClearanceMM
	}
	if placementRequest.Board.MarginMM == 0 {
		placementRequest.Board.MarginMM = 1
	}
	placementRequest.Rules.AllowBackLayer = normalized.Constraints.AllowBackLayer
	placementRequest.Rules.PreferTopLayer = normalized.Constraints.PreferTopLayer
	defaultBounds := opts.DefaultBounds
	if defaultBounds.WidthMM <= 0 || defaultBounds.HeightMM <= 0 {
		defaultBounds = defaultWorkflowBounds
	}
	netIndexes := map[string]int{}
	for _, fragment := range fragments.Fragments {
		groupIDByRole := placementGroupIDByRole(fragment)
		edgeByRole := placementEdgeByRole(fragment)
		localRouteRefs := placementLocalRouteRefs(fragment)
		for _, component := range fragment.Realization.Components {
			position := placement.Placement{
				XMM:         component.Placement.XMM,
				YMM:         component.Placement.YMM,
				RotationDeg: component.Placement.RotationDeg,
				Layer:       firstNonEmpty(component.Placement.Layer, "F.Cu"),
			}
			groupID := groupIDByRole[component.ComponentRole]
			edge := edgeByRole[component.ComponentRole]
			mobility, fixed := generatedPlacementMobility(request, fragment, component, groupID, edge, localRouteRefs[strings.TrimSpace(component.Ref)])
			placementRequest.Components = append(placementRequest.Components, placement.Component{
				Ref:         component.Ref,
				Value:       component.Value,
				FootprintID: component.FootprintID,
				Role:        component.ComponentRole,
				Bounds:      defaultBounds,
				Fixed:       fixed,
				Position:    &position,
				Side:        sideFromLayer(position.Layer),
				Rotation:    fixedRotation(component.Placement.RotationDeg),
				GroupID:     groupID,
				Edge:        edge,
				Mobility:    mobility,
			})
		}
		placementRequest.Groups = append(placementRequest.Groups, placementGroupsFromFragment(fragment)...)
		placementRequest.Keepouts = append(placementRequest.Keepouts, placementKeepoutsFromFragment(fragment)...)
		placementRequest.ProximityRules = append(placementRequest.ProximityRules, proximityRulesFromFragment(fragment)...)
		for _, route := range fragment.Realization.LocalRoutes {
			addPlacementRouteNet(&placementRequest, netIndexes, route)
		}
	}
	componentHintResult := componentPlacementHintRules(opts.ComponentSelections, fragments)
	placementRequest.ProximityRules = append(placementRequest.ProximityRules, componentHintResult.Rules...)
	issues = append(issues, ComponentHintIssues(componentHintResult.Evidence)...)
	issues = append(issues, addPlacementConnectionNets(&placementRequest, netIndexes, normalized, fragments)...)
	placementRequest, padEntries, padIssues := hydratePlacementRequestPads(placementRequest, opts.LibraryIndex)
	issues = append(issues, padIssues...)
	placementRequest = placement.NormalizeRequest(placementRequest)
	placementRequest = preserveAuthoredTranslatedGroupSpread(placementRequest)
	result := placement.PlaceContext(ctx, placementRequest)
	placementRequest.Keepouts = placement.TranslatedKeepoutsForPlacements(placementRequest, result.Placements)
	issues = append(issues, result.Issues...)
	stage := NewStageResult(StagePlacement, issues)
	mobilitySummary := placement.MobilitySummaryForComponents(placementRequest.Components)
	stage.Summary = map[string]any{
		"component_count": result.Metrics.ComponentCount,
		"placed_count":    result.Metrics.PlacedCount,
		"unplaced_count":  result.Metrics.UnplacedCount,
		"fixed_count":     result.Metrics.FixedCount,
		"mobility":        mobilitySummary,
	}
	if scoring := placementCandidateScoringSummary(result.CandidateScoring); scoring != nil {
		stage.Summary["candidate_scoring"] = scoring
	}
	if len(padEntries) != 0 || len(padIssues) != 0 {
		stage.Summary["pad_hydration"] = summarizePadHydration(padEntries, padIssues)
	}
	if len(componentHintResult.Evidence) != 0 {
		stage.Summary["component_hints"] = componentHintResult.Evidence
		stage.Summary["component_hint_summary"] = SummarizeComponentHints(componentHintResult.Evidence)
	}
	stage.Issues = issues
	if result.Status != placement.StatusPlaced && stage.Status == StageStatusOK {
		stage.Status = StageStatusWarning
	}
	return PlacementStageResult{Request: placementRequest, Result: result, Stage: stage}
}

func preserveAuthoredTranslatedGroupSpread(request placement.Request) placement.Request {
	components := make(map[string]placement.Component, len(request.Components))
	for _, component := range request.Components {
		components[strings.ToUpper(strings.TrimSpace(component.Ref))] = component
	}
	for groupIndex := range request.Groups {
		group := &request.Groups[groupIndex]
		if !group.TranslateAsUnit {
			continue
		}
		centers := make([]placement.Point, 0, len(group.Components))
		for _, ref := range group.Components {
			component, ok := components[strings.ToUpper(strings.TrimSpace(ref))]
			if !ok || component.Position == nil {
				continue
			}
			bounds, ok := placement.ComponentPhysicalBounds(component, *component.Position)
			if ok {
				centers = append(centers, bounds.Center())
			}
		}
		for left := 0; left < len(centers); left++ {
			for right := left + 1; right < len(centers); right++ {
				distance := math.Hypot(centers[left].XMM-centers[right].XMM, centers[left].YMM-centers[right].YMM)
				if distance > group.MaxSpreadMM {
					group.MaxSpreadMM = distance
				}
			}
		}
	}
	return request
}

type PlacementCandidateScoringSummary struct {
	Enabled             bool                          `json:"enabled"`
	Policy              string                        `json:"policy,omitempty"`
	ScoreVersion        string                        `json:"score_version,omitempty"`
	AverageWinningScore float64                       `json:"average_winning_score"`
	LowestWinningScore  float64                       `json:"lowest_winning_score"`
	WinningCount        int                           `json:"winning_count"`
	AlternativeCount    int                           `json:"alternative_count"`
	RejectedByReason    map[string]int                `json:"rejected_by_reason,omitempty"`
	AdvancedRules       *AdvancedPlacementRuleSummary `json:"advanced_rules,omitempty"`
}

type AdvancedPlacementRuleSummary struct {
	DimensionCounts map[string]int     `json:"dimension_counts,omitempty"`
	WorstScores     map[string]float64 `json:"worst_scores,omitempty"`
	HardViolations  int                `json:"hard_violations,omitempty"`
	Warnings        int                `json:"warnings,omitempty"`
	Unsupported     int                `json:"unsupported,omitempty"`
}

const advancedRuleWarningThreshold = 0.75

func placementCandidateScoringSummary(report *placement.CandidateScoringReport) *PlacementCandidateScoringSummary {
	if report == nil {
		return nil
	}
	return &PlacementCandidateScoringSummary{
		Enabled:             report.Enabled,
		Policy:              report.Policy,
		ScoreVersion:        report.ScoreVersion,
		AverageWinningScore: finitePlacementSummaryFloat(report.AverageWinningScore),
		LowestWinningScore:  finitePlacementSummaryFloat(report.LowestWinningScore),
		WinningCount:        len(report.WinningCandidates),
		AlternativeCount:    len(report.AlternativeCandidates),
		RejectedByReason:    cloneStringIntMap(report.RejectedByReason),
		AdvancedRules:       advancedPlacementRuleSummary(report),
	}
}

func advancedPlacementRuleSummary(report *placement.CandidateScoringReport) *AdvancedPlacementRuleSummary {
	if report == nil {
		return nil
	}
	summary := &AdvancedPlacementRuleSummary{}
	for _, candidate := range report.WinningCandidates {
		for _, dimension := range candidate.Dimensions {
			if !advancedPlacementDimension(dimension.Name) {
				continue
			}
			if summary.DimensionCounts == nil {
				summary.DimensionCounts = map[string]int{}
			}
			if summary.WorstScores == nil {
				summary.WorstScores = map[string]float64{}
			}
			name := string(dimension.Name)
			summary.DimensionCounts[name]++
			score := finitePlacementSummaryFloat(dimension.Score)
			if _, ok := summary.WorstScores[name]; !ok || score < summary.WorstScores[name] {
				summary.WorstScores[name] = score
			}
			if score < advancedRuleWarningThreshold {
				summary.Warnings++
			}
			unsupported := false
			for _, evidence := range dimension.Evidence {
				evidence = strings.ToLower(evidence)
				if strings.Contains(evidence, "unsupported") || strings.Contains(evidence, "missing") {
					unsupported = true
					break
				}
			}
			if unsupported {
				summary.Unsupported++
			}
		}
	}
	if report.RejectedByReason != nil {
		summary.HardViolations = report.RejectedByReason[string(placement.CandidateRejectAdvancedRule)]
	}
	if len(summary.DimensionCounts) == 0 && summary.HardViolations == 0 {
		return nil
	}
	if len(summary.DimensionCounts) == 0 {
		summary.DimensionCounts = nil
	}
	if len(summary.WorstScores) == 0 {
		summary.WorstScores = nil
	}
	return summary
}

func advancedPlacementDimension(name placement.CandidateScoreDimensionName) bool {
	switch name {
	case placement.CandidateScoreThermal,
		placement.CandidateScoreHighCurrent,
		placement.CandidateScoreCreepageClearance,
		placement.CandidateScoreDifferentialPair,
		placement.CandidateScoreControlledImpedance,
		placement.CandidateScoreTimingSensitive:
		return true
	default:
		return false
	}
}

func finitePlacementSummaryFloat(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func cloneStringIntMap(values map[string]int) map[string]int {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]int, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func hydratePlacementRequestPads(request placement.Request, index *libraryresolver.LibraryIndex) (placement.Request, []PadHydrationEntry, []reports.Issue) {
	resolver := padHydrationResolver{}
	if index != nil {
		resolver.index = *index
	}
	netAssignments := buildPadNetAssignmentIndex(request.Nets)
	entries := make([]PadHydrationEntry, 0, len(request.Components))
	var issues []reports.Issue
	for componentIndex := range request.Components {
		component := &request.Components[componentIndex]
		if len(component.Pads) != 0 {
			if hydrated, attempted := preferredPhysicalPadHydration(&resolver, index, component.Ref, component.FootprintID); attempted {
				if len(hydrated.Pads) != 0 {
					component.Bounds = hydrated.Bounds
					component.AllowUnmatchedUnconnectedPads = hasPackageOnlyPads(component.Ref, hydrated.Pads, netAssignments)
				}
				pads, netIssues := assignPadNetsFromIndex(component.Ref, hydrated.Pads, netAssignments)
				component.Pads = pads
				hydrated.Entry.PadCount = len(pads)
				entries = append(entries, hydrated.Entry)
				issues = append(issues, hydrated.Issues...)
				issues = append(issues, netIssues...)
				continue
			}
			pads, netIssues := assignPadNetsFromIndex(component.Ref, component.Pads, netAssignments)
			component.Pads = pads
			entries = append(entries, PadHydrationEntry{Ref: component.Ref, FootprintID: component.FootprintID, Source: PadHydrationSourceInput, PadCount: len(component.Pads)})
			issues = append(issues, netIssues...)
			continue
		}
		hydrated := resolver.Hydrate(component.Ref, component.FootprintID)
		if len(hydrated.Pads) == 0 {
			fallback := hydratePadsFromVerifiedTemplate(component.Ref, component.FootprintID)
			if len(fallback.Pads) != 0 {
				hydrated = fallback
			} else {
				hydrated.Issues = append(hydrated.Issues, fallback.Issues...)
			}
		}
		if len(hydrated.Pads) != 0 {
			component.Bounds = hydrated.Bounds
			component.AllowUnmatchedUnconnectedPads = hasPackageOnlyPads(component.Ref, hydrated.Pads, netAssignments)
		}
		pads, netIssues := assignPadNetsFromIndex(component.Ref, hydrated.Pads, netAssignments)
		component.Pads = pads
		hydrated.Entry.PadCount = len(pads)
		entries = append(entries, hydrated.Entry)
		issues = append(issues, hydrated.Issues...)
		issues = append(issues, netIssues...)
	}
	request = expandGroupedPlacementPins(request)
	return request, entries, issues
}

func preferredPhysicalPadHydration(resolver *padHydrationResolver, index *libraryresolver.LibraryIndex, ref string, footprintID string) (padHydrationResult, bool) {
	if index != nil {
		if _, ok := libraryresolver.ResolveFootprint(*index, strings.TrimSpace(footprintID)); ok {
			return resolver.Hydrate(ref, footprintID), true
		}
	}
	if fallback := hydratePadsFromVerifiedTemplate(ref, footprintID); len(fallback.Pads) != 0 {
		return fallback, true
	}
	return padHydrationResult{}, false
}

func hasPackageOnlyPads(ref string, pads []placement.PadSummary, assignments padNetAssignmentIndex) bool {
	assignedPins := map[string]struct{}{}
	for _, assignment := range assignments[strings.ToUpper(strings.TrimSpace(ref))] {
		for _, pin := range groupedPinMembers(assignment.Pin) {
			assignedPins[pin] = struct{}{}
		}
	}
	for _, pad := range pads {
		name := strings.TrimSpace(pad.Name)
		if name == "" {
			continue
		}
		if _, ok := assignedPins[name]; !ok {
			return true
		}
	}
	return false
}

func expandGroupedPlacementPins(request placement.Request) placement.Request {
	for netIndex := range request.Nets {
		expanded := make([]placement.Endpoint, 0, len(request.Nets[netIndex].Endpoints))
		for _, endpoint := range request.Nets[netIndex].Endpoints {
			for _, member := range groupedPinMembers(endpoint.Pin) {
				expanded = appendUniquePlacementEndpoints(expanded, placement.Endpoint{Ref: endpoint.Ref, Pin: member})
			}
		}
		request.Nets[netIndex].Endpoints = expanded
	}
	for ruleIndex := range request.ProximityRules {
		request.ProximityRules[ruleIndex].AnchorPins = expandGroupedPinList(request.ProximityRules[ruleIndex].AnchorPins)
		request.ProximityRules[ruleIndex].TargetPins = expandGroupedPinList(request.ProximityRules[ruleIndex].TargetPins)
	}
	return request
}

func expandGroupedPinList(pins []string) []string {
	expanded := []string{}
	seen := map[string]struct{}{}
	for _, pin := range pins {
		for _, member := range groupedPinMembers(pin) {
			if _, exists := seen[member]; exists {
				continue
			}
			seen[member] = struct{}{}
			expanded = append(expanded, member)
		}
	}
	slices.Sort(expanded)
	return expanded
}

func addPlacementRouteNet(request *placement.Request, indexes map[string]int, route blocks.RealizedPCBLocalRoute) {
	name := strings.TrimSpace(route.NetName)
	if name == "" {
		return
	}
	endpoints := placementPhysicalEndpoints(
		placement.Endpoint{Ref: route.From.Ref, Pin: route.From.Pin},
		placement.Endpoint{Ref: route.To.Ref, Pin: route.To.Pin},
	)
	addPlacementNet(request, indexes, name, netRoleFromName(name), 10, endpoints...)
}

func addPlacementNet(request *placement.Request, indexes map[string]int, name string, role placement.NetRole, weight int, endpoints ...placement.Endpoint) {
	name = strings.TrimSpace(name)
	if request == nil || name == "" {
		return
	}
	endpoints = placementPhysicalEndpoints(endpoints...)
	if len(endpoints) == 0 {
		return
	}
	key := strings.ToUpper(name)
	if index, ok := indexes[key]; ok {
		request.Nets[index].Endpoints = appendUniquePlacementEndpoints(request.Nets[index].Endpoints, endpoints...)
		if request.Nets[index].Weight < weight {
			request.Nets[index].Weight = weight
		}
		if request.Nets[index].Role == "" || request.Nets[index].Role == placement.NetUnknown {
			request.Nets[index].Role = role
		}
		return
	}
	indexes[key] = len(request.Nets)
	request.Nets = append(request.Nets, placement.Net{
		Name:      name,
		Endpoints: endpoints,
		Role:      role,
		Weight:    weight,
	})
}

func addPlacementNetWithEndpointMerges(request *placement.Request, indexes map[string]int, name string, role placement.NetRole, weight int, endpoints ...placement.Endpoint) {
	name = strings.TrimSpace(name)
	if request == nil || name == "" {
		return
	}
	endpoints = placementPhysicalEndpoints(endpoints...)
	if len(endpoints) == 0 {
		return
	}
	merged := appendUniquePlacementEndpoints(make([]placement.Endpoint, 0, len(endpoints)), endpoints...)
	mergedKeys := placementEndpointKeySet(merged)
	remaining := append([]placement.Net(nil), request.Nets...)
	for {
		changed := false
		kept := make([]placement.Net, 0, len(remaining))
		for _, net := range remaining {
			if strings.EqualFold(net.Name, name) || placementNetSharesEndpointKeys(net, mergedKeys) {
				merged = appendUniquePlacementEndpoints(merged, net.Endpoints...)
				for _, endpoint := range net.Endpoints {
					if key := placementEndpointKey(endpoint); key != "" {
						mergedKeys[key] = struct{}{}
					}
				}
				changed = true
				continue
			}
			kept = append(kept, net)
		}
		remaining = kept
		if !changed {
			break
		}
	}
	request.Nets = remaining
	rebuildPlacementNetIndexes(indexes, request.Nets)
	addPlacementNet(request, indexes, name, role, weight, merged...)
}

func placementPhysicalEndpoints(endpoints ...placement.Endpoint) []placement.Endpoint {
	filtered := make([]placement.Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if placementEndpointIsPseudoAnchor(endpoint) {
			// Entry anchors are logical routing targets, not placeable physical
			// components. Local-route binding preserves them for route-tree access.
			continue
		}
		filtered = append(filtered, endpoint)
	}
	return filtered
}

func placementEndpointIsPseudoAnchor(endpoint placement.Endpoint) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(endpoint.Ref)), "@anchor:")
}

func placementNetSharesEndpointKeys(net placement.Net, keys map[string]struct{}) bool {
	for _, endpoint := range net.Endpoints {
		if _, ok := keys[placementEndpointKey(endpoint)]; ok {
			return true
		}
	}
	return false
}

func placementEndpointKeySet(endpoints []placement.Endpoint) map[string]struct{} {
	keys := map[string]struct{}{}
	for _, endpoint := range endpoints {
		if key := placementEndpointKey(endpoint); key != "" {
			keys[key] = struct{}{}
		}
	}
	return keys
}

func placementEndpointKey(endpoint placement.Endpoint) string {
	ref := strings.ToUpper(strings.TrimSpace(endpoint.Ref))
	pin := strings.ToUpper(strings.TrimSpace(endpoint.Pin))
	if ref == "" || pin == "" {
		return ""
	}
	return ref + "|" + pin
}

func rebuildPlacementNetIndexes(indexes map[string]int, nets []placement.Net) {
	clear(indexes)
	for index, net := range nets {
		name := strings.TrimSpace(net.Name)
		if name == "" {
			continue
		}
		indexes[strings.ToUpper(name)] = index
	}
}

func appendUniquePlacementEndpoints(existing []placement.Endpoint, incoming ...placement.Endpoint) []placement.Endpoint {
	seen := map[string]struct{}{}
	for _, endpoint := range existing {
		key := strings.ToUpper(strings.TrimSpace(endpoint.Ref)) + "|" + strings.TrimSpace(endpoint.Pin)
		seen[key] = struct{}{}
	}
	for _, endpoint := range incoming {
		key := strings.ToUpper(strings.TrimSpace(endpoint.Ref)) + "|" + strings.TrimSpace(endpoint.Pin)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		existing = append(existing, endpoint)
	}
	return existing
}

func placementGroupIDByRole(fragment BlockFragment) map[string]string {
	byRole := map[string]string{}
	for _, group := range fragment.PlacementGroups {
		groupID := blockPlacementGroupID(fragment, group.ID)
		for _, role := range group.ComponentRoles {
			byRole[strings.TrimSpace(role)] = groupID
		}
	}
	return byRole
}

func placementEdgeByRole(fragment BlockFragment) map[string]placement.EdgeConstraint {
	byRole := map[string]placement.EdgeConstraint{}
	for _, constraint := range fragment.Constraints {
		kind := strings.TrimSpace(constraint.Kind)
		if !strings.EqualFold(kind, "edge_facing") {
			continue
		}
		edge := placementEdgeFromConstraint(constraint)
		for _, role := range constraint.AppliesTo {
			role = strings.TrimSpace(role)
			if role != "" {
				byRole[role] = edge
			}
		}
	}
	return byRole
}

func placementLocalRouteRefs(fragment BlockFragment) map[string]bool {
	refs := map[string]bool{}
	for _, route := range fragment.Realization.LocalRoutes {
		if ref := strings.TrimSpace(route.From.Ref); ref != "" {
			refs[ref] = true
		}
		if ref := strings.TrimSpace(route.To.Ref); ref != "" {
			refs[ref] = true
		}
	}
	return refs
}

func generatedPlacementMobility(request Request, fragment BlockFragment, component blocks.RealizedPCBComponent, groupID string, edge placement.EdgeConstraint, hasLocalRoute bool) (placement.MobilityPolicy, bool) {
	ownerScope := "block:" + fragment.BlockID + "/" + fragment.InstanceID
	constraints := []string{"generated", "block:" + fragment.BlockID}
	// A translated block group preserves its authored local copper, so an
	// edge-constrained member can move safely with the rest of the group. Only
	// standalone edge components need to remain fixed to protect local routes.
	hardEdge := edge != placement.EdgeNone && hasLocalRoute && groupID == ""
	hardFixed := component.Placement.Fixed || request.RoutingRetry.PreserveFixed || hardEdge
	if !request.RoutingRetry.Enabled {
		hardFixed = true
		constraints = append(constraints, "retry_disabled")
	}
	if request.RoutingRetry.PreserveFixed {
		constraints = append(constraints, "preserve_fixed")
	}
	if hardEdge {
		constraints = append(constraints, "edge")
	}
	if component.Placement.Fixed {
		constraints = append(constraints, "component_fixed")
	}
	if hardFixed {
		return placement.MobilityPolicy{
			Class:         placement.MobilityFixed,
			Reason:        generatedMobilityFixedReason(request, component, hardEdge),
			OwnerScope:    ownerScope,
			GroupID:       groupID,
			RouteHandling: placement.RouteHandlingPreserveFixed,
			Constraints:   constraints,
		}, true
	}
	policy := placement.MobilityPolicy{
		OwnerScope:  ownerScope,
		GroupID:     groupID,
		Transforms:  []string{"translate"},
		Reason:      "generated block placement is retry-movable",
		Constraints: constraints,
	}
	switch {
	case groupID != "":
		policy.Class = placement.MobilityGroupTransform
		policy.RouteHandling = placement.RouteHandlingTransformWithGroup
	case hasLocalRoute:
		policy.Class = placement.MobilityLocalRebuild
		policy.RouteHandling = placement.RouteHandlingInvalidateRebuild
	default:
		policy.Class = placement.MobilitySoftPreferred
		policy.RouteHandling = placement.RouteHandlingInvalidateRebuild
	}
	return policy, false
}

func generatedMobilityFixedReason(request Request, component blocks.RealizedPCBComponent, hardEdge bool) string {
	switch {
	case !request.RoutingRetry.Enabled:
		return "routing retry is disabled"
	case request.RoutingRetry.PreserveFixed:
		return "routing retry preserve_fixed is enabled"
	case component.Placement.Fixed:
		return "generated component has fixed placement attribute"
	case hardEdge:
		return "generated component has hard edge constraint"
	}
	return "generated component is treated as fixed by safety policy"
}

func placementEdgeFromConstraint(constraint blocks.PCBConstraint) placement.EdgeConstraint {
	text := normalizeRoleName(constraint.ID + " " + constraint.Kind + " " + constraint.Description)
	switch {
	case containsToken(text, "left"):
		return placement.EdgeLeft
	case containsToken(text, "right"):
		return placement.EdgeRight
	case containsToken(text, "top"):
		return placement.EdgeTop
	case containsToken(text, "bottom"):
		return placement.EdgeBottom
	default:
		return placement.EdgeAny
	}
}

func placementGroupsFromFragment(fragment BlockFragment) []placement.Group {
	groups := make([]placement.Group, 0, len(fragment.PlacementGroups))
	for _, group := range fragment.PlacementGroups {
		converted := placement.Group{
			ID:              blockPlacementGroupID(fragment, group.ID),
			Role:            group.ID,
			Anchor:          placement.GroupAnchor{Ref: fragment.Realization.RoleRefs[strings.TrimSpace(group.AnchorRole)]},
			KeepTogether:    true,
			TranslateAsUnit: group.TranslateAsUnit,
			Priority:        10,
		}
		for _, role := range group.ComponentRoles {
			if ref := fragment.Realization.RoleRefs[strings.TrimSpace(role)]; ref != "" {
				converted.Components = append(converted.Components, ref)
			}
		}
		if group.Bounds != nil {
			converted.MaxSpreadMM = boundsDiagonal(*group.Bounds)
			bounds := placementGroupBoundsRect(fragment, converted.Anchor.Ref, *group.Bounds)
			converted.Bounds = &bounds
		}
		if len(converted.Components) > 0 {
			groups = append(groups, converted)
		}
	}
	return groups
}

func placementGroupBoundsRect(fragment BlockFragment, anchorRef string, bounds blocks.RelativeBounds) placement.Rect {
	anchorX := fragment.OriginXMM
	anchorY := fragment.OriginYMM
	for _, component := range fragment.Realization.Components {
		if strings.EqualFold(strings.TrimSpace(component.Ref), strings.TrimSpace(anchorRef)) {
			anchorX = component.Placement.XMM
			anchorY = component.Placement.YMM
			break
		}
	}
	return placement.Rect{
		Min: placement.Point{XMM: anchorX + bounds.MinXMM, YMM: anchorY + bounds.MinYMM},
		Max: placement.Point{XMM: anchorX + bounds.MaxXMM, YMM: anchorY + bounds.MaxYMM},
	}
}

func placementKeepoutsFromFragment(fragment BlockFragment) []placement.Keepout {
	keepouts := make([]placement.Keepout, 0, len(fragment.Keepouts))
	for _, keepout := range fragment.Keepouts {
		keepouts = append(keepouts, placement.Keepout{
			ID:          blockPlacementGroupID(fragment, keepout.ID),
			GroupID:     placementKeepoutGroupID(fragment, keepout),
			Bounds:      relativeBoundsToPlacementRect(fragment, keepout.Bounds),
			Layers:      []string{keepout.Layer},
			ExemptRefs:  keepoutExemptRefsFromFragment(fragment, keepout),
			Reason:      keepout.Description,
			BlocksRoute: cloneBoolPtr(keepout.BlocksRoute),
		})
	}
	return keepouts
}

func placementKeepoutGroupID(fragment BlockFragment, keepout blocks.PCBKeepout) string {
	groupID := strings.TrimSpace(keepout.PlacementGroupID)
	if groupID == "" {
		return ""
	}
	return blockPlacementGroupID(fragment, groupID)
}

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func keepoutExemptRefsFromFragment(fragment BlockFragment, keepout blocks.PCBKeepout) []string {
	refs := make([]string, 0, len(keepout.AppliesTo))
	seen := map[string]struct{}{}
	for _, role := range keepout.AppliesTo {
		ref := fragment.Realization.RoleRefs[strings.TrimSpace(role)]
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		key := strings.ToUpper(ref)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		refs = append(refs, key)
	}
	slices.Sort(refs)
	return refs
}

func proximityRulesFromFragment(fragment BlockFragment) []placement.ProximityRule {
	rules := []placement.ProximityRule{}
	timingIntentByGroup := placementTimingIntentByGroup(fragment)
	for _, group := range fragment.PlacementGroups {
		anchorRef := fragment.Realization.RoleRefs[strings.TrimSpace(group.AnchorRole)]
		if anchorRef == "" && len(group.ComponentRoles) > 0 {
			anchorRef = fragment.Realization.RoleRefs[strings.TrimSpace(group.ComponentRoles[0])]
		}
		if anchorRef == "" {
			continue
		}
		var targets []string
		for _, role := range group.ComponentRoles {
			ref := fragment.Realization.RoleRefs[strings.TrimSpace(role)]
			if ref != "" && ref != anchorRef {
				targets = append(targets, ref)
			}
		}
		if len(targets) == 0 {
			continue
		}
		role := placementRoleFromGroup(group)
		if timingRole := timingIntentByGroup[strings.TrimSpace(group.ID)]; timingRole != "" {
			role = timingRole
		}
		rules = append(rules, placement.ProximityRule{
			ID:            blockPlacementGroupID(fragment, group.ID) + ".cohesion",
			Source:        "block:" + fragment.BlockID,
			Role:          role,
			AnchorRef:     anchorRef,
			TargetRefs:    targets,
			MaxDistanceMM: max(2, boundsDiagonalValue(group.Bounds)),
			Weight:        5,
			Required:      group.Bounds != nil,
		})
	}
	for _, route := range fragment.Realization.LocalRoutes {
		if placementEndpointIsPseudoAnchor(placement.Endpoint{Ref: route.From.Ref}) || placementEndpointIsPseudoAnchor(placement.Endpoint{Ref: route.To.Ref}) {
			continue
		}
		role := placementRoleFromNetName(route.NetName)
		rules = append(rules, placement.ProximityRule{
			ID:            blockPlacementGroupID(fragment, route.ID) + ".route",
			Source:        "block:" + fragment.BlockID,
			Role:          role,
			AnchorRef:     route.From.Ref,
			TargetRefs:    []string{route.To.Ref},
			AnchorPins:    []string{route.From.Pin},
			TargetPins:    []string{route.To.Pin},
			MaxDistanceMM: 15,
			Weight:        3,
			Required:      false,
		})
	}
	return rules
}

func blockPlacementGroupID(fragment BlockFragment, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		id = "placement"
	}
	return fragment.InstanceID + "." + id
}

func relativeBoundsToPlacementRect(fragment BlockFragment, bounds blocks.RelativeBounds) placement.Rect {
	return placement.Rect{
		Min: placement.Point{XMM: fragment.OriginXMM + bounds.MinXMM, YMM: fragment.OriginYMM + bounds.MinYMM},
		Max: placement.Point{XMM: fragment.OriginXMM + bounds.MaxXMM, YMM: fragment.OriginYMM + bounds.MaxYMM},
	}
}

func boundsDiagonal(bounds blocks.RelativeBounds) float64 {
	return math.Hypot(bounds.MaxXMM-bounds.MinXMM, bounds.MaxYMM-bounds.MinYMM)
}

func boundsDiagonalValue(bounds *blocks.RelativeBounds) float64 {
	if bounds == nil {
		return 10
	}
	return boundsDiagonal(*bounds)
}

func placementRoleFromGroup(group blocks.PCBPlacementGroup) placement.IntentRole {
	name := normalizeRoleName(group.ID + " " + group.Description + " " + strings.Join(group.ComponentRoles, " "))
	switch {
	case placementTextHasToken(name, "decoupling"):
		return placement.IntentDecoupling
	case placementTextHasToken(name, "feedback"):
		return placement.IntentFeedback
	case placementTextHasToken(name, "connector"):
		return placement.IntentConnector
	case placementTextHasToken(name, "regulator"), placementTextHasToken(name, "power"):
		return placement.IntentPowerPath
	case clockRelatedPlacementText(name):
		return placement.IntentClock
	default:
		return ""
	}
}

func clockRelatedPlacementText(text string) bool {
	return placementTextHasToken(text, "clock") || placementTextHasToken(text, "crystal") || placementTextHasToken(text, "xtal") || placementTextHasToken(text, "oscillator") || placementTextHasToken(text, "osc")
}

func placementTextHasToken(normalizedText string, token string) bool {
	for _, candidate := range strings.Fields(strings.ReplaceAll(normalizeRoleName(normalizedText), "_", " ")) {
		if candidate == token || strings.HasPrefix(candidate, token) {
			return true
		}
	}
	return false
}

func placementTimingIntentByGroup(fragment BlockFragment) map[string]placement.IntentRole {
	intents := map[string]placement.IntentRole{}
	for _, timing := range fragment.Realization.Timing {
		groupID := strings.TrimSpace(timing.TimingGroupID)
		if groupID == "" {
			continue
		}
		switch timing.Kind {
		case blocks.PCBTimingKindCrystal, blocks.PCBTimingKindOscillator:
			intents[groupID] = placement.IntentClock
		}
	}
	return intents
}

func placementRoleFromNetName(name string) placement.IntentRole {
	switch netRoleFromName(name) {
	case placement.NetPower, placement.NetGround:
		return placement.IntentPowerPath
	case placement.NetClock:
		return placement.IntentClock
	default:
		return ""
	}
}

func mergePlacementRules(rules placement.Rules) placement.Rules {
	defaults := placement.DefaultRules()
	if rules.GridMM <= 0 {
		rules.GridMM = defaults.GridMM
	}
	if rules.ComponentSpacingMM <= 0 {
		rules.ComponentSpacingMM = defaults.ComponentSpacingMM
	}
	if rules.BoardEdgeClearanceMM <= 0 {
		rules.BoardEdgeClearanceMM = defaults.BoardEdgeClearanceMM
	}
	if rules.GroupSpacingMM <= 0 {
		rules.GroupSpacingMM = defaults.GroupSpacingMM
	}
	if rules.ConnectorEdgeClearanceMM <= 0 {
		rules.ConnectorEdgeClearanceMM = defaults.ConnectorEdgeClearanceMM
	}
	if rules.MaxCandidatesPerPart <= 0 {
		rules.MaxCandidatesPerPart = defaults.MaxCandidatesPerPart
	}
	return rules
}

func fixedRotation(rotation float64) placement.RotationConstraint {
	value := rotation
	return placement.RotationConstraint{FixedDeg: &value}
}

func sideFromLayer(layer string) placement.SideConstraint {
	if layer == "B.Cu" {
		return placement.SideBottom
	}
	return placement.SideTop
}

func netRoleFromName(name string) placement.NetRole {
	switch {
	case containsToken(name, "gnd"), containsToken(name, "ground"):
		return placement.NetGround
	case containsToken(name, "vcc"), containsToken(name, "vdd"), containsToken(name, "vbus"), containsToken(name, "vin"), containsToken(name, "vout"):
		return placement.NetPower
	case containsToken(name, "scl"), containsToken(name, "clk"), containsToken(name, "clock"):
		return placement.NetClock
	default:
		return placement.NetSignal
	}
}

func containsToken(name string, token string) bool {
	name = "_" + normalizeRoleName(name) + "_"
	return strings.Contains(name, "_"+token+"_")
}

func normalizeRoleName(name string) string {
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}
