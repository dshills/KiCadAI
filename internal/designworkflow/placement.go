package designworkflow

import (
	"context"
	"math"
	"strings"
	"unicode"

	"kicadai/internal/blocks"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
)

var defaultWorkflowBounds = placement.Bounds{WidthMM: 2.0, HeightMM: 1.25, Source: placement.BoundsEstimated}

type PlacementOptions struct {
	DefaultBounds placement.Bounds
	Rules         placement.Rules
	LibraryIndex  *libraryresolver.LibraryIndex
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
		for _, component := range fragment.Realization.Components {
			position := placement.Placement{
				XMM:         component.Placement.XMM,
				YMM:         component.Placement.YMM,
				RotationDeg: component.Placement.RotationDeg,
				Layer:       firstNonEmpty(component.Placement.Layer, "F.Cu"),
			}
			placementRequest.Components = append(placementRequest.Components, placement.Component{
				Ref:         component.Ref,
				Value:       component.Value,
				FootprintID: component.FootprintID,
				Role:        component.ComponentRole,
				Bounds:      defaultBounds,
				Fixed:       true,
				Position:    &position,
				Side:        sideFromLayer(position.Layer),
				Rotation:    fixedRotation(component.Placement.RotationDeg),
				GroupID:     groupIDByRole[component.ComponentRole],
				Edge:        edgeByRole[component.ComponentRole],
			})
		}
		placementRequest.Groups = append(placementRequest.Groups, placementGroupsFromFragment(fragment)...)
		placementRequest.Keepouts = append(placementRequest.Keepouts, placementKeepoutsFromFragment(fragment)...)
		placementRequest.ProximityRules = append(placementRequest.ProximityRules, proximityRulesFromFragment(fragment)...)
		for _, route := range fragment.Realization.LocalRoutes {
			addPlacementRouteNet(&placementRequest, netIndexes, route)
		}
	}
	var padEntries []PadHydrationEntry
	var padIssues []reports.Issue
	if opts.LibraryIndex != nil {
		placementRequest, padEntries, padIssues = hydratePlacementRequestPads(placementRequest, opts.LibraryIndex)
		issues = append(issues, padIssues...)
	}
	placementRequest = placement.NormalizeRequest(placementRequest)
	result := placement.PlaceContext(ctx, placementRequest)
	issues = append(issues, result.Issues...)
	stage := NewStageResult(StagePlacement, issues)
	stage.Summary = map[string]any{
		"component_count": result.Metrics.ComponentCount,
		"placed_count":    result.Metrics.PlacedCount,
		"unplaced_count":  result.Metrics.UnplacedCount,
		"fixed_count":     result.Metrics.FixedCount,
	}
	if len(padEntries) != 0 || len(padIssues) != 0 {
		stage.Summary["pad_hydration"] = summarizePadHydration(padEntries, padIssues)
	}
	stage.Issues = issues
	if result.Status != placement.StatusPlaced && stage.Status == StageStatusOK {
		stage.Status = StageStatusWarning
	}
	return PlacementStageResult{Request: placementRequest, Result: result, Stage: stage}
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
			pads, netIssues := assignPadNetsFromIndex(component.Ref, component.Pads, netAssignments)
			component.Pads = pads
			entries = append(entries, PadHydrationEntry{Ref: component.Ref, FootprintID: component.FootprintID, Source: PadHydrationSourceInput, PadCount: len(component.Pads)})
			issues = append(issues, netIssues...)
			continue
		}
		hydrated := resolver.Hydrate(component.Ref, component.FootprintID)
		if len(hydrated.Pads) != 0 {
			component.Bounds = hydrated.Bounds
		}
		pads, netIssues := assignPadNetsFromIndex(component.Ref, hydrated.Pads, netAssignments)
		component.Pads = pads
		hydrated.Entry.PadCount = len(pads)
		entries = append(entries, hydrated.Entry)
		issues = append(issues, hydrated.Issues...)
		issues = append(issues, netIssues...)
	}
	return request, entries, issues
}

func addPlacementRouteNet(request *placement.Request, indexes map[string]int, route blocks.RealizedPCBLocalRoute) {
	name := strings.TrimSpace(route.NetName)
	if name == "" {
		return
	}
	key := strings.ToUpper(name)
	endpoints := []placement.Endpoint{
		{Ref: route.From.Ref, Pin: route.From.Pin},
		{Ref: route.To.Ref, Pin: route.To.Pin},
	}
	if index, ok := indexes[key]; ok {
		request.Nets[index].Endpoints = appendUniquePlacementEndpoints(request.Nets[index].Endpoints, endpoints...)
		if request.Nets[index].Weight < 10 {
			request.Nets[index].Weight = 10
		}
		return
	}
	indexes[key] = len(request.Nets)
	request.Nets = append(request.Nets, placement.Net{
		Name:      name,
		Endpoints: endpoints,
		Role:      netRoleFromName(name),
		Weight:    10,
	})
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
			ID:           blockPlacementGroupID(fragment, group.ID),
			Role:         group.ID,
			KeepTogether: true,
			Priority:     10,
		}
		for _, role := range group.ComponentRoles {
			if ref := fragment.Realization.RoleRefs[strings.TrimSpace(role)]; ref != "" {
				converted.Components = append(converted.Components, ref)
			}
		}
		if group.AnchorRole != "" {
			converted.Anchor.Ref = fragment.Realization.RoleRefs[strings.TrimSpace(group.AnchorRole)]
		}
		if group.Bounds != nil {
			converted.MaxSpreadMM = boundsDiagonal(*group.Bounds)
		}
		if len(converted.Components) > 0 {
			groups = append(groups, converted)
		}
	}
	return groups
}

func placementKeepoutsFromFragment(fragment BlockFragment) []placement.Keepout {
	keepouts := make([]placement.Keepout, 0, len(fragment.Keepouts))
	for _, keepout := range fragment.Keepouts {
		keepouts = append(keepouts, placement.Keepout{
			ID:     blockPlacementGroupID(fragment, keepout.ID),
			Bounds: relativeBoundsToPlacementRect(fragment, keepout.Bounds),
			Layers: []string{keepout.Layer},
			Reason: keepout.Description,
		})
	}
	return keepouts
}

func proximityRulesFromFragment(fragment BlockFragment) []placement.ProximityRule {
	rules := []placement.ProximityRule{}
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
		rules = append(rules, placement.ProximityRule{
			ID:            blockPlacementGroupID(fragment, group.ID) + ".cohesion",
			Source:        "block:" + fragment.BlockID,
			Role:          placementRoleFromGroup(group),
			AnchorRef:     anchorRef,
			TargetRefs:    targets,
			MaxDistanceMM: max(2, boundsDiagonalValue(group.Bounds)),
			Weight:        5,
			Required:      group.Bounds != nil,
		})
	}
	for _, route := range fragment.Realization.LocalRoutes {
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
	case strings.Contains(name, "decoupling"):
		return placement.IntentDecoupling
	case strings.Contains(name, "feedback"):
		return placement.IntentFeedback
	case strings.Contains(name, "connector"):
		return placement.IntentConnector
	case strings.Contains(name, "regulator"), strings.Contains(name, "power"):
		return placement.IntentPowerPath
	case strings.Contains(name, "clock"), strings.Contains(name, "crystal"):
		return placement.IntentClock
	default:
		return ""
	}
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
