package designworkflow

import (
	"cmp"
	"context"
	"encoding/json"
	"math"
	"slices"
	"strings"

	"kicadai/internal/libraryresolver"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/routingadapters"
	"kicadai/internal/transactions"
)

func PlaceExplicitCircuit(ctx context.Context, request Request, opts PlacementOptions) PlacementStageResult {
	if ctx == nil {
		ctx = context.Background()
	}
	var issues []reports.Issue
	if err := ctx.Err(); err != nil {
		issues = append(issues, reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityBlocked, Path: "context", Message: err.Error()})
	}
	if request.ExplicitCircuit == nil {
		issues = append(issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityBlocked, Path: "explicit_circuit", Message: "explicit circuit is required"})
		return PlacementStageResult{Stage: NewStageResult(StagePlacement, issues)}
	}
	if reports.HasBlockingIssue(issues) {
		return PlacementStageResult{Stage: NewStageResult(StagePlacement, issues)}
	}
	seed := request.ExplicitCircuit.GenerationHash
	if seed == "" {
		seed = request.ExplicitCircuit.ResolutionHash
	}
	placementRequest := placement.Request{
		Board: placement.BoardPlacementArea{WidthMM: request.Board.WidthMM, HeightMM: request.Board.HeightMM, MarginMM: request.Board.EdgeClearanceMM, Layers: request.Board.Layers},
		Rules: mergePlacementRules(opts.Rules), Seed: seed,
	}
	if placementRequest.Board.MarginMM == 0 {
		placementRequest.Board.MarginMM = 1
	}
	placementRequest.Rules.AllowBackLayer = request.Constraints.AllowBackLayer
	placementRequest.Rules.PreferTopLayer = request.Constraints.PreferTopLayer
	refsByID := make(map[string]string, len(request.ExplicitCircuit.Components))
	for _, component := range request.ExplicitCircuit.Components {
		refsByID[component.ID] = component.Reference
		rotations := []float64{0}
		if component.Placement.ThermalEdgeRequired {
			rotations = []float64{0, 90}
		}
		placementRequest.Components = append(placementRequest.Components, placement.Component{
			Ref: component.Reference, Value: component.Value, FootprintID: component.FootprintID,
			Role: component.Role, Edge: explicitPlacementEdge(component.Placement.Edge), Priority: component.Placement.Priority,
			Rotation: placement.RotationConstraint{AllowedDeg: rotations},
			Side:     placement.SideTop, Mobility: placement.MobilityPolicy{
				Class: placement.MobilitySoftPreferred, Reason: "catalog-resolved graph placement",
				OwnerScope: "explicit-circuit", RouteHandling: placement.RouteHandlingInvalidateRebuild,
				Transforms: []string{"translate"}, Constraints: []string{"catalog_resolved"},
			},
		})
		if rule, ok := explicitThermalPlacementRule(component); ok {
			placementRequest.AdvancedRules.Thermal = append(placementRequest.AdvancedRules.Thermal, rule)
		}
	}
	for _, net := range request.ExplicitCircuit.Nets {
		entry := placement.Net{Name: net.Name, Role: explicitPlacementNetRole(net.Role), Weight: explicitNetWeight(net), WidthClass: net.NetClass}
		for _, endpoint := range net.Endpoints {
			entry.Endpoints = append(entry.Endpoints, placement.Endpoint{Ref: refsByID[endpoint.Component], Pin: endpoint.Pad})
		}
		placementRequest.Nets = append(placementRequest.Nets, entry)
	}
	regionRefs := map[string][]string{}
	for _, component := range request.ExplicitCircuit.Components {
		if component.Placement.Region != "" && !component.Placement.ThermalEdgeRequired {
			regionRefs[component.Placement.Region] = append(regionRefs[component.Placement.Region], component.Reference)
		}
		if component.Placement.Near != "" {
			maxDistance := component.Placement.MaxDistanceMM
			if maxDistance == 0 {
				maxDistance = 12
			}
			placementRequest.ProximityRules = append(placementRequest.ProximityRules, placement.ProximityRule{
				ID: "explicit.near." + component.ID, Source: "circuit_graph", AnchorRef: refsByID[component.Placement.Near],
				TargetRefs: []string{component.Reference}, MaxDistanceMM: maxDistance, Weight: max(1, component.Placement.Priority), Required: true,
			})
		}
	}
	for _, region := range request.ExplicitCircuit.Regions {
		refs := regionRefs[region.ID]
		if len(refs) == 0 {
			continue
		}
		placementRequest.RegionRules = append(placementRequest.RegionRules, placement.RegionRule{
			ID: "explicit.region." + region.ID, Source: "circuit_graph", Region: region.ID, Refs: refs, Required: true, Weight: 10,
			Preferred: placement.Rect{Min: placement.Point{XMM: region.XMM, YMM: region.YMM}, Max: placement.Point{XMM: region.XMM + region.WidthMM, YMM: region.YMM + region.HeightMM}},
		})
	}
	for _, keepout := range request.ExplicitCircuit.Keepouts {
		blocksRoute := true
		placementRequest.Keepouts = append(placementRequest.Keepouts, placement.Keepout{
			ID: keepout.ID, Layers: append([]string(nil), keepout.Layers...), BlocksRoute: &blocksRoute,
			Bounds: placement.Rect{Min: placement.Point{XMM: keepout.XMM, YMM: keepout.YMM}, Max: placement.Point{XMM: keepout.XMM + keepout.WidthMM, YMM: keepout.YMM + keepout.HeightMM}},
			Reason: "catalog-resolved circuit graph keepout",
		})
	}
	placementRequest, padEntries, padIssues := hydratePlacementRequestPads(placementRequest, opts.LibraryIndex)
	placementRequest.Rules.ComponentSpacingMM = max(placementRequest.Rules.ComponentSpacingMM, explicitRoutingAccessSpacing(request.ExplicitCircuit.Nets))
	if request.ExplicitCircuit.RoutingPolicy == ExplicitRoutingPolicyConstrainedEndpointAccessV1 {
		placementRequest.ComponentOrder = placement.ComponentOrderLargestFootprintFirstV1
	}
	issues = append(issues, padIssues...)
	placementRequest = placement.NormalizeRequest(placementRequest)
	result := placement.PlaceContext(ctx, placementRequest)
	rightAngleFallback := false
	rightAngleFallbackPlacedCount := 0
	rightAngleFallbackUnplacedCount := 0
	rightAngleFallbackUnplacedRefs := []string{}
	if result.Status != placement.StatusPlaced &&
		request.ExplicitCircuit.RoutingPolicy == ExplicitRoutingPolicyConstrainedEndpointAccessV1 {
		rotatedRequest := explicitRightAnglePlacementFallback(placementRequest)
		rotatedResult := placement.PlaceContext(ctx, rotatedRequest)
		rightAngleFallbackPlacedCount = rotatedResult.Metrics.PlacedCount
		rightAngleFallbackUnplacedCount = rotatedResult.Metrics.UnplacedCount
		rightAngleFallbackUnplacedRefs = explicitUnplacedRefs(rotatedResult)
		if rotatedResult.Status == placement.StatusPlaced ||
			rotatedResult.Metrics.PlacedCount >= result.Metrics.PlacedCount {
			placementRequest = rotatedRequest
			result = rotatedResult
			rightAngleFallback = true
		}
	}
	issues = append(issues, result.Issues...)
	stage := NewStageResult(StagePlacement, issues)
	stage.Summary = map[string]any{
		"component_count": result.Metrics.ComponentCount, "placed_count": result.Metrics.PlacedCount,
		"unplaced_count": result.Metrics.UnplacedCount, "region_rule_count": len(placementRequest.RegionRules),
		"proximity_rule_count": len(placementRequest.ProximityRules), "pad_hydration": summarizePadHydration(padEntries, padIssues),
		"right_angle_fallback": rightAngleFallback, "right_angle_fallback_placed_count": rightAngleFallbackPlacedCount,
		"right_angle_fallback_unplaced_count": rightAngleFallbackUnplacedCount,
		"right_angle_fallback_unplaced_refs":  rightAngleFallbackUnplacedRefs,
	}
	if result.Status != placement.StatusPlaced && stage.Status == StageStatusOK {
		stage.Status = StageStatusWarning
	}
	return PlacementStageResult{Request: placementRequest, Result: result, Stage: stage}
}

func explicitThermalPlacementRule(component ExplicitComponentSpec) (placement.ThermalPlacementRule, bool) {
	thermalRole := strings.TrimSpace(component.Placement.ThermalRole)
	if thermalRole == "" {
		return placement.ThermalPlacementRule{}, false
	}
	thermalPathID := strings.TrimSpace(component.Placement.ThermalPathID)
	var keepAwayRoles []string
	if role := strings.TrimSpace(component.Placement.ThermalKeepAwayRole); role != "" {
		keepAwayRoles = []string{role}
	}
	preferredRegion := component.Placement.Region
	if component.Placement.ThermalEdgeRequired {
		preferredRegion = ""
	}
	return placement.ThermalPlacementRule{
		ID: "explicit.thermal." + component.ID, Source: "catalog_thermal_path:" + thermalPathID,
		Refs: []string{component.Reference}, ThermalRole: placement.ThermalRole(thermalRole),
		PreferredEdge: explicitPlacementEdge(component.Placement.Edge), PreferredRegion: preferredRegion,
		KeepAwayRoles: keepAwayRoles, MinDistanceMM: component.Placement.ThermalClearanceMM,
		PreferCopper: component.Placement.PreferThermalCopper, Severity: placement.AdvancedRuleSeverityError,
		Enforcement: placement.AdvancedRuleHard,
	}, true
}

func explicitUnplacedRefs(result placement.Result) []string {
	refs := []string{}
	for _, component := range result.Placements {
		if component.Reason != "" {
			refs = append(refs, component.Ref)
		}
	}
	slices.Sort(refs)
	return refs
}

func explicitRightAnglePlacementFallback(request placement.Request) placement.Request {
	result := request
	result.ComponentOrder = placement.ComponentOrderDenseLargestFootprintFirstV1
	result.Components = append([]placement.Component(nil), request.Components...)
	for index := range result.Components {
		component := &result.Components[index]
		if component.Fixed || component.Edge != placement.EdgeNone {
			continue
		}
		if request.Rules.AllowBackLayer && explicitBackSidePlacementEligible(*component) {
			component.Side = placement.SideAny
		}
		width, height := component.Bounds.WidthMM, component.Bounds.HeightMM
		switch {
		case width <= 0 || height <= 0 || width == height || request.Board.WidthMM == request.Board.HeightMM:
			component.Rotation = placement.RotationConstraint{AllowedDeg: []float64{0, 90}}
		case request.Board.WidthMM > request.Board.HeightMM && height > width:
			component.Rotation = placement.RotationConstraint{AllowedDeg: []float64{90}}
		case request.Board.HeightMM > request.Board.WidthMM && width > height:
			component.Rotation = placement.RotationConstraint{AllowedDeg: []float64{90}}
		default:
			component.Rotation = placement.RotationConstraint{AllowedDeg: []float64{0}}
		}
	}
	return placement.NormalizeRequest(result)
}

func explicitBackSidePlacementEligible(component placement.Component) bool {
	if len(component.Pads) == 0 {
		return false
	}
	for _, pad := range component.Pads {
		if !strings.EqualFold(strings.TrimSpace(pad.Type), "smd") {
			return false
		}
	}
	return true
}

func explicitRoutingAccessSpacing(nets []ExplicitNetSpec) float64 {
	// This is a conservative endpoint-access envelope, not a fixed density
	// policy: each explicit net can tune both width and clearance, and the
	// placement rule uses only the largest declared envelope.
	defaultClearance := routing.DefaultRules().ClearanceMM
	spacing := 0.0
	for _, net := range nets {
		width := net.WidthMM
		if width <= 0 {
			width = routing.DefaultRules().TraceWidthMM
		}
		clearance := net.ClearanceMM
		if clearance <= 0 {
			clearance = defaultClearance
		}
		spacing = max(spacing, width+2*clearance)
	}
	return spacing
}

func RouteExplicitCircuit(ctx context.Context, request Request, placed PlacementStageResult, opts RoutingOptions) RoutingStageResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if request.ExplicitCircuit == nil {
		return RoutingStageResult{Stage: NewStageResult(StageRouting, []reports.Issue{{Code: reports.CodeInvalidArgument, Severity: reports.SeverityBlocked, Path: "explicit_circuit", Message: "explicit circuit is required"}})}
	}
	if opts.Skip || request.Validation.SkipRouting {
		return RoutingStageResult{Stage: StageResult{Name: StageRouting, Status: StageStatusSkipped, Summary: map[string]any{"reason": "routing skipped"}}}
	}
	if workflowStageBlocked(placed.Stage) {
		return RoutingStageResult{Stage: StageResult{Name: StageRouting, Status: StageStatusSkipped, Summary: map[string]any{"reason": "placement did not complete"}}}
	}
	routingRequest, issues := routingadapters.RequestFromPlacement(placed.Request, placed.Result)
	routingRequest.Strategy.NetOrder = request.ExplicitCircuit.RoutingPolicy
	routingRequest = expandExplicitPhysicalPadEndpoints(routingRequest)
	applyRoutingOptions(request, opts, &routingRequest)
	if routingRequest.Rules.NetOverrides == nil {
		routingRequest.Rules.NetOverrides = map[string]routing.NetRule{}
	}
	for _, net := range request.ExplicitCircuit.Nets {
		if net.NetClass == "" && net.WidthMM == 0 && net.ClearanceMM == 0 && len(net.AllowedLayers) == 0 && net.PreferLayer == "" && net.MaxLengthMM == 0 {
			continue
		}
		rule := routingRequest.Rules.NetOverrides[net.Name]
		if net.NetClass != "" {
			rule.ClassName = net.NetClass
		}
		rule.TraceWidthMM = net.WidthMM
		rule.ClearanceMM = net.ClearanceMM
		rule.AllowedLayers = append([]string(nil), net.AllowedLayers...)
		rule.PreferLayer = net.PreferLayer
		rule.MaxLengthMM = net.MaxLengthMM
		routingRequest.Rules.NetOverrides[net.Name] = rule
	}
	issues = append(issues, fitRoutingClearanceToIntrinsicPads(&routingRequest, placed.Request.Components, opts.ClearanceMM > 0 || request.Constraints.ClearanceMM > 0)...)
	result := routing.Result{Status: routing.StatusBlocked}
	routeOrder := FinalRouteOrderNegotiationSummary{}
	if !reports.HasBlockingIssue(issues) {
		result, routeOrder = routeWithFailedNetFirstNegotiationOptions(ctx, routingRequest, routeNegotiationOptions{YieldRepairableConflict: opts.yieldToPlacementRepair})
		issues = append(issues, result.Issues...)
	}
	issues = append(issues, explicitRequiredRouteIssues(request.ExplicitCircuit.Nets, result)...)
	operations, operationIssues := finalizeExplicitRouteOperations(result.Operations, &placed)
	issues = append(issues, operationIssues...)
	var clearanceIssues []reports.Issue
	clearanceBlockersBefore := 0
	clearanceBlockersAfter := 0
	clearanceMM := routingRequest.Rules.ClearanceMM
	layerTransitionViasAdded := 0
	if request.Validation.RequireDRC {
		var junctionIssues []reports.Issue
		operations, junctionIssues = repairAcuteRouteOperationJunctions(routingRequest, operations)
		issues = append(issues, junctionIssues...)
		result.Issues = append(result.Issues, junctionIssues...)
		if reports.HasBlockingIssue(junctionIssues) {
			result.Status = routing.StatusBlocked
		}
		operations, _, clearanceBlockersBefore, clearanceBlockersAfter, clearanceMM = finalizeEmittedRoutePhysicalClearance(routingRequest, operations)
		if !reports.HasBlockingIssue(junctionIssues) {
			var materializationIssues []reports.Issue
			operations, layerTransitionViasAdded, materializationIssues = ensureRouteLayerJunctionVias(operations, routingRequest.Rules)
			issues = append(issues, materializationIssues...)
			result.Issues = append(result.Issues, materializationIssues...)
			junctionIssues = append(junctionIssues, materializationIssues...)
			if reports.HasBlockingIssue(materializationIssues) {
				result.Status = routing.StatusBlocked
			}
		}
		if layerTransitionViasAdded > 0 && !reports.HasBlockingIssue(junctionIssues) {
			var reduced map[int]struct{}
			var reductionIssues []reports.Issue
			operations, reduced, reductionIssues = removeRedundantRouteViasAtPlatedPadsWithContext(operations, &placed, newPhysicalPadRoutingContext(&placed))
			layerTransitionViasAdded -= len(reduced)
			issues = append(issues, reductionIssues...)
			result.Issues = append(result.Issues, reductionIssues...)
			junctionIssues = append(junctionIssues, reductionIssues...)
		}
		if layerTransitionViasAdded > 0 && !reports.HasBlockingIssue(junctionIssues) {
			// Newly materialized transitions participate in the complete
			// cross-copper clearance set. Give their attached tracks the same
			// bounded generic repair used for combined route phases before the
			// transition-specific relocation pass proves via-to-pad clearance.
			var postJunctionClearanceIssues []reports.Issue
			operations, postJunctionClearanceIssues = repairEmittedRoutePhysicalClearance(routingRequest, operations)
			issues = append(issues, postJunctionClearanceIssues...)
			result.Issues = append(result.Issues, postJunctionClearanceIssues...)
			junctionIssues = append(junctionIssues, postJunctionClearanceIssues...)
		}
		if !reports.HasBlockingIssue(junctionIssues) {
			// Track-clearance repair may add alternate-layer doglegs or move
			// their attached vertices. Revalidate transition vias afterward;
			// only this final copper can prove transition clearance, so do not
			// retain a stale failure from the pre-repair geometry.
			var transitionIssues []reports.Issue
			operations, transitionIssues = repairRouteTransitionViaClearance(routingRequest, operations)
			issues = append(issues, transitionIssues...)
			result.Issues = append(result.Issues, transitionIssues...)
			junctionIssues = append(junctionIssues, transitionIssues...)
			if reports.HasBlockingIssue(transitionIssues) {
				result.Status = routing.StatusBlocked
			}
		}
		if !reports.HasBlockingIssue(junctionIssues) {
			var holeClearanceIssues []reports.Issue
			operations, holeClearanceIssues = repairRouteTransitionPadHoleClearance(routingRequest, operations, &placed)
			issues = append(issues, holeClearanceIssues...)
			result.Issues = append(result.Issues, holeClearanceIssues...)
			junctionIssues = append(junctionIssues, holeClearanceIssues...)
			if reports.HasBlockingIssue(holeClearanceIssues) {
				result.Status = routing.StatusBlocked
			}
		}
		if reports.HasBlockingIssue(junctionIssues) {
			result.Status = routing.StatusBlocked
		}
	}
	// Clearance and layer-transition repair can rewrite a simple route into a
	// tree walk. Normalize the final emitted geometry, not only the router's
	// initial operations, so repair-created reversal branches receive the same
	// physical-contact-aware cleanup as every other generated route.
	physical := newPhysicalPadRoutingContext(&placed)
	physicalEvidence := BuildInterBlockContactTargets(physical.candidates, &placed)
	var finalizationIssues []reports.Issue
	operations, finalizationIssues = postProcessRouteOperations(operations, &placed, physical, physicalEvidence)
	issues = append(issues, finalizationIssues...)
	result.Issues = append(result.Issues, finalizationIssues...)
	if reports.HasBlockingIssue(finalizationIssues) {
		result.Status = routing.StatusBlocked
	}
	operations, endpointTailCleanup := trimDisconnectedRouteTailsAtSameNetPadsWithSummary(operations, physical)
	operations = compactRouteOperationGeometry(operations)
	operations, danglingRouteViasPruned := pruneRouteViasWithoutTwoLayerContact(operations, newPhysicalPadRoutingContext(&placed))
	operations = compactRouteOperationGeometry(operations)
	operations, zoneAccessViasAdded, zoneAccessViaIssues := materializeExplicitZoneAccessVias(
		request.ExplicitCircuit.Zones, operations, routingRequest,
	)
	issues = append(issues, zoneAccessViaIssues...)
	result.Issues = append(result.Issues, zoneAccessViaIssues...)
	if reports.HasBlockingIssue(zoneAccessViaIssues) {
		result.Status = routing.StatusBlocked
	}
	operations, pairedReturnViasAdded, pairedReturnViaIssues := materializeExplicitReturnTransitionVias(
		request.ExplicitCircuit.Nets, request.ExplicitCircuit.Zones, operations, routingRequest, request.Board.ThicknessMM,
	)
	issues = append(issues, pairedReturnViaIssues...)
	result.Issues = append(result.Issues, pairedReturnViaIssues...)
	if reports.HasBlockingIssue(pairedReturnViaIssues) {
		result.Status = routing.StatusBlocked
	}
	if request.Validation.RequireDRC {
		finalClearanceRequest := routingRequest
		routing.NormalizeRequest(&finalClearanceRequest)
		clearanceIssues = routing.ValidatePhysicalTrackClearance(finalClearanceRequest, routingRoutesFromOperations(operations))
		clearanceBlockersAfter = blockingIssueCount(clearanceIssues)
	}
	returnPathEvidence, returnPathIssues := explicitReturnPathEvidence(
		request.ExplicitCircuit.Nets, request.ExplicitCircuit.Zones, routingRoutesFromOperations(operations),
		routingRequest.Board.Layers, request.Board.ThicknessMM,
	)
	issues = append(issues, returnPathIssues...)
	result.Issues = append(result.Issues, returnPathIssues...)
	if reports.HasBlockingIssue(returnPathIssues) {
		result.Status = routing.StatusBlocked
	}
	clearanceIssues, clearanceDeferredToDRC := deferPhysicalClearanceIssuesToRequiredDRC(
		request.Validation.RequireDRC,
		IsGenericAutonomousCorrectionRequest(request),
		clearanceIssues,
	)
	issues = append(issues, clearanceIssues...)
	result.Issues = append(result.Issues, clearanceIssues...)
	if reports.HasBlockingIssue(clearanceIssues) {
		result.Status = routing.StatusBlocked
	}
	stage := NewStageResult(StageRouting, issues)
	stage.Summary = map[string]any{
		"status": result.Status, "net_count": result.Metrics.NetCount, "routed_nets": result.Metrics.RoutedNetCount,
		"failed_nets": result.Metrics.FailedNetCount, "route_operations": len(operations), "route_order": routeOrder,
		"clearance_mm": clearanceMM, "physical_clearance_before_repair": clearanceBlockersBefore,
		"physical_clearance_after_repair": clearanceBlockersAfter, "physical_clearance_deferred_drc": clearanceDeferredToDRC,
		"layer_transition_vias_added": layerTransitionViasAdded,
		"zone_access_vias_added":      zoneAccessViasAdded,
		"paired_return_vias_added":    pairedReturnViasAdded,
		"route_endpoint_tail_cleanup": endpointTailCleanup,
		"dangling_route_vias_pruned":  danglingRouteViasPruned,
		"return_path_evidence":        returnPathEvidence,
	}
	if result.Status != routing.StatusRouted && stage.Status == StageStatusOK {
		stage.Status = StageStatusWarning
	}
	return RoutingStageResult{Request: routingRequest, Result: result, Operations: operations, Stage: stage}
}

// materializeExplicitZoneAccessVias ensures every generated inner-layer zone
// has a plated connection to same-net routed copper or a through-hole pad.
// Without this invariant KiCad can refill a syntactically valid zone as an
// electrically isolated copper island. Candidate vias stay on existing
// same-net segment centerlines and use the exact batch clearance selector.
func materializeExplicitZoneAccessVias(
	zones []ExplicitZoneSpec,
	operations []transactions.Operation,
	routingRequest routing.Request,
) ([]transactions.Operation, int, []reports.Issue) {
	materialized := append([]transactions.Operation(nil), operations...)
	if len(zones) == 0 || len(routingRequest.Board.Layers) < 3 {
		return materialized, 0, nil
	}
	stack := newCopperLayerStack(routingRequest.Board.Layers, 0)
	if stack.ambiguous {
		return materialized, 0, []reports.Issue{{
			Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Stage: string(StageRouting),
			Path: "board.layers", Message: "copper layer names must be unique after case and whitespace normalization",
		}}
	}
	routes := routingRoutesFromOperations(materialized)
	selector, selectorIssues := routing.NewPhysicalClearanceViaSelector(routingRequest, routes)
	if reports.HasBlockingIssue(selectorIssues) {
		return materialized, 0, selectorIssues
	}
	selector.SetMinimumHoleToHoleClearanceMM(minimumRouteHoleToHoleClearanceMM)

	routesByNet := make(map[string]routing.Route, len(routes))
	for _, route := range routes {
		routesByNet[strings.ToLower(strings.TrimSpace(route.Net))] = route
	}
	throughHoleAccessNets := make(map[string]bool)
	for _, component := range routingRequest.Components {
		for _, pad := range component.Pads {
			netKey := strings.ToLower(strings.TrimSpace(pad.Net))
			if netKey != "" && pad.Type == routing.PadThroughHole && pad.Drill != nil && pad.Drill.DiameterMM > 0 {
				throughHoleAccessNets[netKey] = true
			}
		}
	}
	orderedZones := append([]ExplicitZoneSpec(nil), zones...)
	slices.SortStableFunc(orderedZones, func(left, right ExplicitZoneSpec) int {
		if order := strings.Compare(strings.ToLower(strings.TrimSpace(left.Net)), strings.ToLower(strings.TrimSpace(right.Net))); order != 0 {
			return order
		}
		return strings.Compare(strings.Join(left.Layers, "\x00"), strings.Join(right.Layers, "\x00"))
	})
	seen := map[string]bool{}
	added := 0
	issues := append([]reports.Issue(nil), selectorIssues...)
	for _, zone := range orderedZones {
		netKey := strings.ToLower(strings.TrimSpace(zone.Net))
		route := routesByNet[netKey]
		for _, declaredLayer := range zone.Layers {
			layer := stack.canonicalName(declaredLayer)
			layerIndex, known := stack.stackIndexes[strings.ToLower(strings.TrimSpace(layer))]
			if !known || layerIndex == 0 || layerIndex == len(stack.names)-1 {
				continue
			}
			obligationKey := netKey + "\x00" + strings.ToLower(layer)
			if seen[obligationKey] {
				continue
			}
			seen[obligationKey] = true
			if explicitZoneLayerAlreadyAccessible(route, netKey, layerIndex, stack, throughHoleAccessNets) {
				continue
			}

			routingNet := routing.Net{Name: zone.Net}
			for _, candidate := range routingRequest.Nets {
				if strings.EqualFold(strings.TrimSpace(candidate.Name), strings.TrimSpace(zone.Net)) {
					routingNet = candidate
					break
				}
			}
			effectiveRule, ruleIssues := routing.ResolveNetRule(&routingRequest, routingNet)
			issues = append(issues, ruleIssues...)
			if reports.HasBlockingIssue(ruleIssues) {
				return materialized, added, issues
			}
			viaTemplate := routing.Via{
				Net: zone.Net, DiameterMM: effectiveRule.ViaDiameterMM, DrillMM: effectiveRule.ViaDrillMM,
				Layers: []string{stack.names[0], stack.names[len(stack.names)-1]},
			}
			candidates := explicitZoneAccessViaCandidates(route, viaTemplate)
			selected, found := selector.First(candidates)
			if !found {
				issues = append(issues, reports.Issue{
					Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Stage: string(StageRouting),
					Path:    "explicit_circuit.zones." + zone.Net,
					Message: "inner-layer zone has no clearance-safe same-net access-via location",
					Nets:    []string{zone.Net},
				})
				return materialized, added, issues
			}
			selector.Add(selected)
			route.Net = zone.Net
			route.Status = routing.RouteStatusRouted
			route.Vias = append(route.Vias, selected)
			routesByNet[netKey] = route
			operation, err := workflowOperation(transactions.OpRoute, transactions.RouteOperation{
				Op: transactions.OpRoute, NetName: zone.Net,
				Vias: []transactions.RouteViaSpec{{
					At:         transactions.Point{XMM: selected.At.XMM, YMM: selected.At.YMM},
					DiameterMM: selected.DiameterMM, DrillMM: selected.DrillMM,
					Layers: append([]string(nil), selected.Layers...),
				}},
			})
			if err != nil {
				issues = append(issues, reports.Issue{
					Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Stage: string(StageRouting),
					Path: "explicit_circuit.zones." + zone.Net, Message: "encode zone access via: " + err.Error(),
					Nets: []string{zone.Net},
				})
				return materialized, added, issues
			}
			materialized = append(materialized, operation)
			added++
		}
	}
	return materialized, added, issues
}

func explicitZoneLayerAlreadyAccessible(
	route routing.Route,
	netKey string,
	layerIndex int,
	stack copperLayerStack,
	throughHoleAccessNets map[string]bool,
) bool {
	for _, segment := range route.Segments {
		if stack.canonicalName(segment.Layer) == stack.names[layerIndex] {
			return true
		}
	}
	for _, via := range route.Vias {
		minimum, maximum, valid := viaLayerBounds(via, stack)
		if valid && minimum <= layerIndex && maximum >= layerIndex {
			return true
		}
	}
	return throughHoleAccessNets[netKey]
}

func explicitZoneAccessViaCandidates(route routing.Route, template routing.Via) []routing.Via {
	segments := append([]routing.Segment(nil), route.Segments...)
	slices.SortStableFunc(segments, func(left, right routing.Segment) int {
		if order := strings.Compare(strings.ToLower(left.Layer), strings.ToLower(right.Layer)); order != 0 {
			return order
		}
		for _, pair := range [][2]float64{{left.Start.XMM, right.Start.XMM}, {left.Start.YMM, right.Start.YMM}, {left.End.XMM, right.End.XMM}, {left.End.YMM, right.End.YMM}} {
			if order := cmp.Compare(pair[0], pair[1]); order != 0 {
				return order
			}
		}
		return 0
	})
	fractions := []float64{.5, .25, .75, 0, 1}
	candidates := make([]routing.Via, 0, len(segments)*len(fractions))
	seen := map[routeCoordKey]bool{}
	for _, segment := range segments {
		for _, fraction := range fractions {
			point := routing.Point{
				XMM: segment.Start.XMM + (segment.End.XMM-segment.Start.XMM)*fraction,
				YMM: segment.Start.YMM + (segment.End.YMM-segment.Start.YMM)*fraction,
			}
			// PCB transaction coordinates serialize at integer nanometre
			// precision. Deduplicate at that physical identity rather than using
			// float64 coordinates as exact map keys.
			key := routeCoordKey{
				x: int64(math.Round(point.XMM * 1_000_000)),
				y: int64(math.Round(point.YMM * 1_000_000)),
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			candidate := template
			candidate.At = point
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

type ExplicitReturnPathEvidence struct {
	Net                string                             `json:"net"`
	ReturnNet          string                             `json:"return_net"`
	PreferredLayer     string                             `json:"preferred_layer,omitempty"`
	PreferredLayerUsed bool                               `json:"preferred_layer_used"`
	UsedLayers         []string                           `json:"used_layers"`
	ReturnPlaneLayers  []string                           `json:"return_plane_layers,omitempty"`
	MaxLengthMM        float64                            `json:"max_length_mm,omitempty"`
	RouteLengthMM      float64                            `json:"route_length_mm"`
	MaxDistanceMM      float64                            `json:"max_distance_mm"`
	WorstDistanceMM    float64                            `json:"worst_distance_mm"`
	SampleSpacingMM    float64                            `json:"sample_spacing_mm"`
	SampleCount        int                                `json:"sample_count"`
	SamplingComplete   bool                               `json:"sampling_complete"`
	LayerTransitions   []ExplicitReturnTransitionEvidence `json:"layer_transitions,omitempty"`
	Pass               bool                               `json:"pass"`
}

type ExplicitReturnTransitionEvidence struct {
	XMM                 float64  `json:"x_mm"`
	YMM                 float64  `json:"y_mm"`
	SignalLayers        []string `json:"signal_layers"`
	ReferenceLayers     []string `json:"reference_layers"`
	ReturnViaRequired   bool     `json:"return_via_required"`
	ReturnViaFound      bool     `json:"return_via_found"`
	ReturnViaDistanceMM float64  `json:"return_via_distance_mm"`
	Pass                bool     `json:"pass"`
}

func explicitReturnPathEvidence(
	nets []ExplicitNetSpec,
	zones []ExplicitZoneSpec,
	routes []routing.Route,
	boardLayers []routing.Layer,
	boardThicknessMM float64,
) ([]ExplicitReturnPathEvidence, []reports.Issue) {
	routesByNet := make(map[string]routing.Route, len(routes))
	for _, route := range routes {
		routesByNet[route.Net] = route
	}
	stack := newCopperLayerStack(boardLayers, boardThicknessMM)
	if stack.ambiguous {
		return nil, []reports.Issue{{
			Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Stage: string(StageRouting),
			Path: "board.layers", Message: "copper layer names must be unique after case and whitespace normalization",
		}}
	}
	copperLayers := stack.names
	canonicalLayer := stack.canonicalName
	canonicalRoutesByNet := make(map[string]routing.Route, len(routesByNet))
	for netName, route := range routesByNet {
		canonicalRoutesByNet[netName] = canonicalRouteLayers(route, stack)
	}
	returnPlaneLayerSets := map[string]map[string]bool{}
	for _, zone := range zones {
		for _, layer := range zone.Layers {
			canonical := canonicalLayer(layer)
			if canonical == "" {
				continue
			}
			if returnPlaneLayerSets[zone.Net] == nil {
				returnPlaneLayerSets[zone.Net] = map[string]bool{}
			}
			returnPlaneLayerSets[zone.Net][canonical] = true
		}
	}
	returnPlaneLayersByNet := make(map[string][]string, len(returnPlaneLayerSets))
	for netName, layerSet := range returnPlaneLayerSets {
		for layer := range layerSet {
			returnPlaneLayersByNet[netName] = append(returnPlaneLayersByNet[netName], layer)
		}
		slices.Sort(returnPlaneLayersByNet[netName])
	}
	var evidence []ExplicitReturnPathEvidence
	var issues []reports.Issue
	for _, net := range nets {
		if net.ReturnNet == "" || net.ReturnPathMaxDistanceMM <= 0 {
			continue
		}
		signal := canonicalRoutesByNet[net.Name]
		returnPath := canonicalRoutesByNet[net.ReturnNet]
		returnPlaneLayers := returnPlaneLayersByNet[net.ReturnNet]
		item := ExplicitReturnPathEvidence{
			Net: net.Name, ReturnNet: net.ReturnNet, PreferredLayer: net.PreferLayer, MaxLengthMM: net.MaxLengthMM,
			MaxDistanceMM: net.ReturnPathMaxDistanceMM, UsedLayers: []string{},
			ReturnPlaneLayers: returnPlaneLayers, SamplingComplete: true, Pass: true,
		}
		if (len(signal.Segments) == 0 && len(signal.Vias) == 0) ||
			(len(returnPath.Segments) == 0 && len(returnPath.Vias) == 0 && len(returnPlaneLayers) == 0) {
			item.Pass = false
		} else {
			usedLayers := map[string]bool{}
			remainingDistanceEvaluations := explicitReturnPathMaximumDistanceEvaluations
			returnConductorCount := len(returnPath.Segments) + len(returnPath.Vias)
			for _, segment := range signal.Segments {
				layer := canonicalLayer(segment.Layer)
				if layer == "" {
					layer = segment.Layer
				}
				usedLayers[layer] = true
				item.RouteLengthMM += math.Hypot(segment.End.XMM-segment.Start.XMM, segment.End.YMM-segment.Start.YMM)
				intervals, sampleSpacingMM, complete := explicitReturnPathSegmentSampling(segment, net.ReturnPathMaxDistanceMM)
				item.SampleSpacingMM = math.Max(item.SampleSpacingMM, sampleSpacingMM)
				if !complete {
					item.SamplingComplete = false
					item.Pass = false
					continue
				}
				if !consumeReturnPathEvaluationBudget(&remainingDistanceEvaluations, intervals+1, returnConductorCount) {
					item.SamplingComplete = false
					item.Pass = false
					continue
				}
				returnPlaneDistanceMM := nearestReturnPlaneDistance(layer, returnPlaneLayers, stack)
				for sampleIndex := 0; sampleIndex <= intervals; sampleIndex++ {
					ratio := float64(sampleIndex) / float64(intervals)
					sample := routing.Point{
						XMM: segment.Start.XMM + (segment.End.XMM-segment.Start.XMM)*ratio,
						YMM: segment.Start.YMM + (segment.End.YMM-segment.Start.YMM)*ratio,
					}
					distance := nearestReturnConductorDistance(
						sample, layer, returnPath, returnPlaneDistanceMM, stack,
					)
					item.SampleCount++
					item.WorstDistanceMM = math.Max(item.WorstDistanceMM, distance)
					if distance > net.ReturnPathMaxDistanceMM {
						item.Pass = false
					}
				}
			}
			for _, via := range signal.Vias {
				// A declared via span is authoritative. A missing span denotes a
				// through-via, so sample every copper layer in stack order.
				viaLayers := copperLayers
				if len(via.Layers) != 0 {
					viaLayers = append([]string(nil), via.Layers...)
				}
				item.RouteLengthMM += viaVerticalSpanMM(via, stack)
				for _, layer := range viaLayers {
					canonical := canonicalLayer(layer)
					if canonical == "" {
						canonical = layer
					}
					usedLayers[canonical] = true
				}
				if !consumeReturnPathEvaluationBudget(&remainingDistanceEvaluations, len(viaLayers), returnConductorCount) {
					item.SamplingComplete = false
					item.Pass = false
					continue
				}
				for _, layer := range viaLayers {
					canonical := canonicalLayer(layer)
					if canonical == "" {
						canonical = layer
					}
					returnPlaneDistanceMM := nearestReturnPlaneDistance(canonical, returnPlaneLayers, stack)
					distance := nearestReturnConductorDistance(
						via.At, canonical, returnPath, returnPlaneDistanceMM, stack,
					)
					item.SampleCount++
					item.WorstDistanceMM = math.Max(item.WorstDistanceMM, distance)
					if distance > net.ReturnPathMaxDistanceMM {
						item.Pass = false
					}
				}
			}
			item.LayerTransitions = explicitReturnTransitionEvidence(
				signal, returnPath, returnPlaneLayers, stack, net.ReturnPathMaxDistanceMM,
			)
			for _, transition := range item.LayerTransitions {
				if !transition.Pass {
					item.Pass = false
				}
			}
			for layer := range usedLayers {
				item.UsedLayers = append(item.UsedLayers, layer)
			}
			slices.Sort(item.UsedLayers)
			preferredLayer := stack.canonicalName(net.PreferLayer)
			item.PreferredLayerUsed = net.PreferLayer == "" || (preferredLayer != "" && usedLayers[preferredLayer])
			// PreferLayer influences routing cost; AllowedLayers is the hard layer
			// constraint. Preserve preference use as evidence, but accept another
			// allowed layer when its measured return path satisfies the hard bound.
			if net.MaxLengthMM > 0 && item.RouteLengthMM > net.MaxLengthMM {
				item.Pass = false
			}
		}
		item.MaxLengthMM, item.Pass = finiteReturnPathEvidenceValue(item.MaxLengthMM, item.Pass)
		item.MaxDistanceMM, item.Pass = finiteReturnPathEvidenceValue(item.MaxDistanceMM, item.Pass)
		item.RouteLengthMM, item.Pass = finiteReturnPathEvidenceValue(item.RouteLengthMM, item.Pass)
		item.WorstDistanceMM, item.Pass = finiteReturnPathEvidenceValue(item.WorstDistanceMM, item.Pass)
		item.SampleSpacingMM, item.Pass = finiteReturnPathEvidenceValue(item.SampleSpacingMM, item.Pass)
		evidence = append(evidence, item)
		if !item.Pass {
			issues = append(issues, reports.Issue{
				Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Stage: string(StageRouting),
				Path:       "explicit_circuit.nets." + net.Name + ".return_path",
				Message:    "routed net violates its declared route-length or return-path bound",
				Nets:       []string{net.Name, net.ReturnNet},
				Suggestion: "shorten the route or move the return conductor closer",
			})
		}
	}
	return evidence, issues
}

func explicitReturnTransitionEvidence(
	signal routing.Route,
	returnPath routing.Route,
	returnPlaneLayers []string,
	stack copperLayerStack,
	maximumDistanceMM float64,
) []ExplicitReturnTransitionEvidence {
	var evidence []ExplicitReturnTransitionEvidence
	for _, signalVia := range signal.Vias {
		signalMinimum, signalMaximum, valid := viaLayerBounds(signalVia, stack)
		if !valid || signalMinimum == signalMaximum {
			continue
		}
		item := ExplicitReturnTransitionEvidence{
			XMM: signalVia.At.XMM, YMM: signalVia.At.YMM,
			SignalLayers: []string{stack.names[signalMinimum], stack.names[signalMaximum]},
			Pass:         true,
		}
		minimumReference, minimumFound := nearestReferenceLayerIndex(signalMinimum, returnPlaneLayers, stack)
		maximumReference, maximumFound := nearestReferenceLayerIndex(signalMaximum, returnPlaneLayers, stack)
		if minimumFound && maximumFound {
			item.ReferenceLayers = []string{stack.names[minimumReference]}
			if maximumReference != minimumReference {
				item.ReferenceLayers = []string{
					stack.names[min(minimumReference, maximumReference)],
					stack.names[max(minimumReference, maximumReference)],
				}
				item.ReturnViaRequired = true
			}
		} else {
			// Without declared reference planes, require the return conductor to
			// make the same layer transition as the signal.
			minimumReference, maximumReference = signalMinimum, signalMaximum
			item.ReturnViaRequired = true
		}
		if item.ReturnViaRequired {
			item.ReturnViaDistanceMM = math.MaxFloat64
			for _, returnVia := range returnPath.Vias {
				returnMinimum, returnMaximum, valid := viaLayerBounds(returnVia, stack)
				if !valid || returnMinimum > min(minimumReference, maximumReference) ||
					returnMaximum < max(minimumReference, maximumReference) {
					continue
				}
				distance := math.Hypot(signalVia.At.XMM-returnVia.At.XMM, signalVia.At.YMM-returnVia.At.YMM)
				item.ReturnViaDistanceMM = math.Min(item.ReturnViaDistanceMM, distance)
			}
			item.ReturnViaFound = item.ReturnViaDistanceMM <= maximumDistanceMM
			item.Pass = item.ReturnViaFound
		}
		evidence = append(evidence, item)
	}
	slices.SortFunc(evidence, func(left, right ExplicitReturnTransitionEvidence) int {
		if order := cmp.Compare(left.XMM, right.XMM); order != 0 {
			return order
		}
		if order := cmp.Compare(left.YMM, right.YMM); order != 0 {
			return order
		}
		if order := cmp.Compare(left.SignalLayers[0], right.SignalLayers[0]); order != 0 {
			return order
		}
		return cmp.Compare(left.SignalLayers[1], right.SignalLayers[1])
	})
	return evidence
}

const explicitReturnViaMaximumCandidatePoints = 10000
const explicitReturnViaDistanceToleranceMM = 1e-12

// materializeExplicitReturnTransitionVias returns the updated operations and
// the number of newly emitted paired return vias; existing route operations do
// not contribute to that count.
func materializeExplicitReturnTransitionVias(
	nets []ExplicitNetSpec,
	zones []ExplicitZoneSpec,
	operations []transactions.Operation,
	routingRequest routing.Request,
	boardThicknessMM float64,
) ([]transactions.Operation, int, []reports.Issue) {
	materialized := append([]transactions.Operation(nil), operations...)
	if len(routingRequest.Board.Layers) < 2 {
		return materialized, 0, nil
	}
	routes := routingRoutesFromOperations(materialized)
	evidence, evidenceIssues := explicitReturnPathEvidence(nets, zones, routes, routingRequest.Board.Layers, boardThicknessMM)
	repairableEvidenceNets := map[string]bool{}
	for _, item := range evidence {
		repairableEvidenceNets[item.Net] = explicitReturnPathEvidenceRepairableByPairedVias(item)
	}
	var unresolvedEvidenceIssues []reports.Issue
	for _, issue := range evidenceIssues {
		if len(issue.Nets) == 0 || !repairableEvidenceNets[issue.Nets[0]] {
			unresolvedEvidenceIssues = append(unresolvedEvidenceIssues, issue)
		}
	}
	if reports.HasBlockingIssue(unresolvedEvidenceIssues) {
		return materialized, 0, unresolvedEvidenceIssues
	}
	netsByName := make(map[string]*ExplicitNetSpec, len(nets))
	for netIndex := range nets {
		netsByName[nets[netIndex].Name] = &nets[netIndex]
	}
	type returnViaObligation struct {
		net        *ExplicitNetSpec
		transition ExplicitReturnTransitionEvidence
	}
	var obligations []returnViaObligation
	for _, item := range evidence {
		for _, transition := range item.LayerTransitions {
			if transition.ReturnViaRequired && !transition.ReturnViaFound {
				obligations = append(obligations, returnViaObligation{net: netsByName[item.Net], transition: transition})
			}
		}
	}
	if len(obligations) == 0 {
		return materialized, 0, nil
	}
	selector, selectorIssues := routing.NewPhysicalClearanceViaSelector(routingRequest, routes)
	if reports.HasBlockingIssue(selectorIssues) {
		return materialized, 0, selectorIssues
	}
	diagnostics := make([]reports.Issue, 0, len(unresolvedEvidenceIssues)+len(selectorIssues))
	diagnostics = append(diagnostics, unresolvedEvidenceIssues...)
	diagnostics = append(diagnostics, selectorIssues...)
	selector.SetMinimumHoleToHoleClearanceMM(minimumRouteHoleToHoleClearanceMM)
	stack := newCopperLayerStack(routingRequest.Board.Layers, boardThicknessMM)
	routesByNet := make(map[string]routing.Route, len(routes))
	for _, route := range routes {
		routesByNet[route.Net] = route
	}
	viaTemplates := map[string]transactions.RouteViaSpec{}
	type selectedReturnVia struct {
		net string
		via transactions.RouteViaSpec
	}
	var selectedVias []selectedReturnVia
	var routingCandidates []routing.Via
	for obligationIndex := range obligations {
		obligation := &obligations[obligationIndex]
		if obligation.net == nil {
			return materialized, 0, appendExplicitReturnViaDiagnostics(diagnostics, explicitReturnViaMaterializationIssue(
				nil, &obligation.transition, "return-via obligation has no matching explicit net",
			))
		}
		if explicitReturnTransitionAlreadySatisfied(
			obligation.transition, routesByNet[obligation.net.ReturnNet], stack, obligation.net.ReturnPathMaxDistanceMM,
		) {
			continue
		}
		if len(obligation.transition.ReferenceLayers) != 2 {
			return materialized, 0, appendExplicitReturnViaDiagnostics(diagnostics, explicitReturnViaMaterializationIssue(
				obligation.net, &obligation.transition, "automatic return-via insertion requires two declared reference-plane layers",
			))
		}
		via, found := viaTemplates[obligation.net.ReturnNet]
		if !found {
			returnRoutingNet := routing.Net{Name: obligation.net.ReturnNet, Role: routing.NetGround}
			for _, candidateNet := range routingRequest.Nets {
				if candidateNet.Name == obligation.net.ReturnNet {
					returnRoutingNet = candidateNet
					break
				}
			}
			effectiveRule, ruleIssues := routing.ResolveNetRule(&routingRequest, returnRoutingNet)
			diagnostics = append(diagnostics, ruleIssues...)
			if reports.HasBlockingIssue(ruleIssues) {
				return materialized, 0, diagnostics
			}
			via = transactions.RouteViaSpec{
				DiameterMM: effectiveRule.ViaDiameterMM, DrillMM: effectiveRule.ViaDrillMM,
				// RouteViaSpec and the KiCad writer currently model ordinary
				// plated through vias, not blind/buried fabrication types. Span
				// the declared outer copper layers so the via intersects both
				// selected internal reference planes.
				Layers: []string{routingRequest.Board.Layers[0].Name, routingRequest.Board.Layers[len(routingRequest.Board.Layers)-1].Name},
			}
			viaTemplates[obligation.net.ReturnNet] = via
		}
		via.At = transactions.Point{XMM: obligation.transition.XMM, YMM: obligation.transition.YMM}
		candidates, complete := explicitReturnViaCandidates(
			via.At, routingRequest.Rules.GridMM, obligation.net.ReturnPathMaxDistanceMM,
		)
		if !complete {
			return materialized, 0, appendExplicitReturnViaDiagnostics(diagnostics, explicitReturnViaMaterializationIssue(
				obligation.net, &obligation.transition, "return-via candidate search exceeds its deterministic work bound",
			))
		}
		if cap(routingCandidates) < len(candidates) {
			routingCandidates = make([]routing.Via, 0, len(candidates))
		} else {
			routingCandidates = routingCandidates[:0]
		}
		for _, candidatePoint := range candidates {
			routingCandidates = append(routingCandidates, routing.Via{
				Net: obligation.net.ReturnNet, At: routing.Point{XMM: candidatePoint.XMM, YMM: candidatePoint.YMM},
				DiameterMM: via.DiameterMM, DrillMM: via.DrillMM, Layers: via.Layers,
			})
		}
		selected, found := selector.First(routingCandidates)
		if !found {
			return materialized, 0, appendExplicitReturnViaDiagnostics(diagnostics, explicitReturnViaMaterializationIssue(
				obligation.net, &obligation.transition, "reference-plane change has no clearance-safe paired return-via location",
			))
		}
		selector.Add(selected)
		returnRoute := routesByNet[obligation.net.ReturnNet]
		returnRoute.Net = obligation.net.ReturnNet
		returnRoute.Status = routing.RouteStatusRouted
		returnRoute.Vias = append(returnRoute.Vias, selected)
		routesByNet[obligation.net.ReturnNet] = returnRoute
		via.At = transactions.Point{XMM: selected.At.XMM, YMM: selected.At.YMM}
		selectedVias = append(selectedVias, selectedReturnVia{net: obligation.net.ReturnNet, via: via})
	}
	emittedOperations := make([]transactions.Operation, 0, len(selectedVias))
	for _, selected := range selectedVias {
		operation, err := workflowOperation(transactions.OpRoute, transactions.RouteOperation{
			Op: transactions.OpRoute, NetName: selected.net, Vias: []transactions.RouteViaSpec{selected.via},
		})
		if err != nil {
			return materialized, 0, appendExplicitReturnViaDiagnostics(diagnostics, reports.Issue{
				Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Stage: string(StageRouting),
				Path: "explicit_circuit.return_path", Message: "encode paired return-via operation: " + err.Error(),
			})
		}
		emittedOperations = append(emittedOperations, operation)
	}
	materialized = append(materialized, emittedOperations...)
	return materialized, len(selectedVias), diagnostics
}

func appendExplicitReturnViaDiagnostics(diagnostics []reports.Issue, additional ...reports.Issue) []reports.Issue {
	combined := make([]reports.Issue, 0, len(diagnostics)+len(additional))
	combined = append(combined, diagnostics...)
	return append(combined, additional...)
}

func explicitReturnPathEvidenceRepairableByPairedVias(item ExplicitReturnPathEvidence) bool {
	if !item.SamplingComplete || item.SampleCount == 0 || item.WorstDistanceMM > item.MaxDistanceMM ||
		(item.MaxLengthMM > 0 && item.RouteLengthMM > item.MaxLengthMM) {
		return false
	}
	missingPair := false
	for _, transition := range item.LayerTransitions {
		if transition.Pass {
			continue
		}
		if !transition.ReturnViaRequired || transition.ReturnViaFound {
			return false
		}
		missingPair = true
	}
	return missingPair
}

func explicitReturnViaCandidates(
	center transactions.Point,
	gridMM float64,
	maximumDistanceMM float64,
) ([]transactions.Point, bool) {
	if !finiteScalar(gridMM) || gridMM <= 0 {
		gridMM = routing.DefaultRules().GridMM
	}
	if !finiteScalar(maximumDistanceMM) || maximumDistanceMM <= 0 {
		return nil, false
	}
	maximumRingFloat := math.Ceil(maximumDistanceMM / gridMM)
	diameterSteps := 2*maximumRingFloat + 1
	if !finiteScalar(maximumRingFloat) || diameterSteps*diameterSteps-1 > explicitReturnViaMaximumCandidatePoints {
		return nil, false
	}
	maximumRing := int(maximumRingFloat)
	maximumDistanceWithToleranceMM := maximumDistanceMM + explicitReturnViaDistanceToleranceMM
	maximumDistanceSquared := maximumDistanceWithToleranceMM * maximumDistanceWithToleranceMM
	type offset struct{ x, y int }
	squarePointCount := (2*maximumRing+1)*(2*maximumRing+1) - 1
	offsets := make([]offset, 0, min(explicitReturnViaMaximumCandidatePoints, squarePointCount))
	appendOffset := func(x, y int) bool {
		dx, dy := float64(x)*gridMM, float64(y)*gridMM
		if dx*dx+dy*dy > maximumDistanceSquared {
			return true
		}
		if len(offsets) >= explicitReturnViaMaximumCandidatePoints {
			return false
		}
		offsets = append(offsets, offset{x: x, y: y})
		return true
	}
	for ring := 1; ring <= maximumRing; ring++ {
		for x := -ring; x <= ring; x++ {
			if !appendOffset(x, ring) || !appendOffset(x, -ring) {
				return nil, false
			}
		}
		for y := -ring + 1; y < ring; y++ {
			if !appendOffset(ring, y) || !appendOffset(-ring, y) {
				return nil, false
			}
		}
	}
	slices.SortFunc(offsets, func(left, right offset) int {
		leftDistance := left.x*left.x + left.y*left.y
		rightDistance := right.x*right.x + right.y*right.y
		if order := cmp.Compare(leftDistance, rightDistance); order != 0 {
			return order
		}
		// At equal Euclidean distance, prefer +X and then +Y. This stable
		// directional tie-break matches the deterministic transition repair
		// search used elsewhere in the router.
		if order := cmp.Compare(right.x, left.x); order != 0 {
			return order
		}
		return cmp.Compare(right.y, left.y)
	})
	candidates := make([]transactions.Point, len(offsets))
	for index, candidate := range offsets {
		candidates[index] = transactions.Point{
			XMM: center.XMM + float64(candidate.x)*gridMM,
			YMM: center.YMM + float64(candidate.y)*gridMM,
		}
	}
	return candidates, true
}

func explicitReturnTransitionAlreadySatisfied(
	transition ExplicitReturnTransitionEvidence,
	returnRoute routing.Route,
	stack copperLayerStack,
	maximumDistanceMM float64,
) bool {
	if len(transition.SignalLayers) != 2 {
		return false
	}
	signal := routing.Route{Vias: []routing.Via{{
		At:     routing.Point{XMM: transition.XMM, YMM: transition.YMM},
		Layers: append([]string(nil), transition.SignalLayers...),
	}}}
	evidence := explicitReturnTransitionEvidence(signal, returnRoute, transition.ReferenceLayers, stack, maximumDistanceMM)
	return len(evidence) == 1 && evidence[0].Pass
}

func explicitReturnViaMaterializationIssue(
	net *ExplicitNetSpec,
	transition *ExplicitReturnTransitionEvidence,
	message string,
) reports.Issue {
	issue := reports.Issue{
		Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Stage: string(StageRouting),
		Path: "explicit_circuit.return_path", Message: message,
	}
	if net != nil {
		issue.Path = "explicit_circuit.nets." + net.Name + ".return_path"
		issue.Nets = []string{net.Name, net.ReturnNet}
	}
	if transition != nil {
		issue.Suggestion = "reserve paired-via clearance within the declared return-path distance of the signal transition"
	}
	return issue
}

func nearestReferenceLayerIndex(signalIndex int, returnPlaneLayers []string, stack copperLayerStack) (int, bool) {
	nearestIndex, nearestDelta := -1, len(stack.names)+1
	for _, layer := range returnPlaneLayers {
		index, found := stack.stackIndexes[normalizedCopperLayerName(layer)]
		if !found {
			continue
		}
		delta := index - signalIndex
		if delta < 0 {
			delta = -delta
		}
		if delta < nearestDelta || (delta == nearestDelta && index < nearestIndex) {
			nearestIndex, nearestDelta = index, delta
		}
	}
	return nearestIndex, nearestIndex >= 0
}

func finiteReturnPathEvidenceValue(value float64, pass bool) (float64, bool) {
	if finiteScalar(value) {
		return value, pass
	}
	// JSON has no representation for NaN or infinity. MaxFloat64 is a
	// deterministic, encodable fail-closed sentinel for unbounded evidence.
	return math.MaxFloat64, false
}

func nearestReturnConductorDistance(
	point routing.Point,
	signalLayer string,
	route routing.Route,
	returnPlaneDistanceMM float64,
	stack copperLayerStack,
) float64 {
	distance := returnPlaneDistanceMM
	for _, segment := range route.Segments {
		planarDistance := pointToSegmentDistance(point, segment.Start, segment.End)
		layerDistance := stack.separationCanonicalMM(signalLayer, segment.Layer)
		distance = math.Min(distance, math.Hypot(planarDistance, layerDistance))
	}
	for _, via := range route.Vias {
		planarDistance := math.Hypot(point.XMM-via.At.XMM, point.YMM-via.At.YMM)
		layerDistance := viaLayerSeparationMM(signalLayer, via, stack)
		distance = math.Min(distance, math.Hypot(planarDistance, layerDistance))
	}
	return distance
}

func nearestReturnPlaneDistance(signalLayer string, returnPlaneLayers []string, stack copperLayerStack) float64 {
	distance := math.Inf(1)
	for _, planeLayer := range returnPlaneLayers {
		if planeLayer == signalLayer {
			continue
		}
		distance = math.Min(distance, stack.separationCanonicalMM(signalLayer, planeLayer))
	}
	return distance
}

const explicitReturnPathMaximumSegmentIntervals = 100000
const explicitReturnPathMaximumDistanceEvaluations = 1000000

func consumeReturnPathEvaluationBudget(remaining *int, sampleCount, conductorCount int) bool {
	if sampleCount < 0 || conductorCount < 0 || remaining == nil || *remaining < 0 {
		return false
	}
	if sampleCount == 0 || conductorCount == 0 {
		return true
	}
	if sampleCount > *remaining/conductorCount {
		return false
	}
	*remaining -= sampleCount * conductorCount
	return true
}

func explicitReturnPathSegmentSampling(segment routing.Segment, maximumDistanceMM float64) (int, float64, bool) {
	length := math.Hypot(segment.End.XMM-segment.Start.XMM, segment.End.YMM-segment.Start.YMM)
	if length == 0 {
		return 1, 0, true
	}
	if !finiteScalar(length) || !finiteScalar(maximumDistanceMM) || maximumDistanceMM <= 0 {
		return 0, math.Inf(1), false
	}
	targetSpacingMM := math.Min(1, maximumDistanceMM/2)
	requiredIntervals := math.Ceil(length / targetSpacingMM)
	if !finiteScalar(requiredIntervals) || requiredIntervals > explicitReturnPathMaximumSegmentIntervals {
		return 0, length / explicitReturnPathMaximumSegmentIntervals, false
	}
	intervals := max(1, int(requiredIntervals))
	spacingMM := length / float64(intervals)
	return intervals, spacingMM, true
}

func copperLayerNames(layers []routing.Layer) []string {
	var names []string
	for _, layer := range layers {
		if layer.Kind == routing.LayerCopper && layer.Routable {
			names = append(names, layer.Name)
		}
	}
	return names
}

func normalizedCopperLayerName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

type copperLayerStack struct {
	names        []string
	canonical    map[string]string
	stackIndexes map[string]int
	thicknessMM  float64
	ambiguous    bool
}

func newCopperLayerStack(layers []routing.Layer, boardThicknessMM float64) copperLayerStack {
	stack := copperLayerStack{
		names: copperLayerNames(layers), canonical: map[string]string{}, stackIndexes: map[string]int{},
		thicknessMM: boardThicknessMM,
	}
	for index, name := range stack.names {
		key := normalizedCopperLayerName(name)
		if existing, found := stack.canonical[key]; found && existing != name {
			stack.ambiguous = true
			continue
		}
		stack.canonical[key] = name
		stack.stackIndexes[key] = index
	}
	return stack
}

func (stack copperLayerStack) canonicalName(name string) string {
	return stack.canonical[normalizedCopperLayerName(name)]
}

func (stack copperLayerStack) separationMM(left, right string) float64 {
	return stack.separationCanonicalMM(stack.canonicalName(left), stack.canonicalName(right))
}

func (stack copperLayerStack) separationCanonicalMM(left, right string) float64 {
	if left == right && left != "" {
		return 0
	}
	// Generated boards declare copper order and total thickness but not a
	// manufacturer-specific core/prepreg build. Use the deterministic uniform
	// stack model implied by that declaration; the emitted evidence remains
	// reproducible and can be replaced by explicit stackup geometry later.
	if len(stack.names) < 2 || !finiteScalar(stack.thicknessMM) || stack.thicknessMM <= 0 || stack.ambiguous {
		return math.Inf(1)
	}
	leftIndex, leftFound := stack.stackIndexes[normalizedCopperLayerName(left)]
	rightIndex, rightFound := stack.stackIndexes[normalizedCopperLayerName(right)]
	if !leftFound || !rightFound {
		return math.Inf(1)
	}
	return math.Abs(float64(leftIndex-rightIndex)) * stack.thicknessMM / float64(len(stack.names)-1)
}

func viaVerticalSpanMM(via routing.Via, stack copperLayerStack) float64 {
	minimumIndex, maximumIndex, valid := viaLayerBounds(via, stack)
	if !valid || len(stack.names) < 2 || !finiteScalar(stack.thicknessMM) || stack.thicknessMM <= 0 {
		return math.Inf(1)
	}
	return float64(maximumIndex-minimumIndex) * stack.thicknessMM / float64(len(stack.names)-1)
}

func viaLayerBounds(via routing.Via, stack copperLayerStack) (int, int, bool) {
	if len(stack.names) == 0 || stack.ambiguous {
		return 0, 0, false
	}
	if len(via.Layers) == 0 {
		return 0, len(stack.names) - 1, true
	}
	minimumIndex, maximumIndex := len(stack.names), -1
	for _, layer := range via.Layers {
		index, found := stack.stackIndexes[normalizedCopperLayerName(layer)]
		if !found {
			return 0, 0, false
		}
		minimumIndex = min(minimumIndex, index)
		maximumIndex = max(maximumIndex, index)
	}
	return minimumIndex, maximumIndex, true
}

func viaLayerSeparationMM(signalLayer string, via routing.Via, stack copperLayerStack) float64 {
	if len(stack.names) < 2 || !finiteScalar(stack.thicknessMM) || stack.thicknessMM <= 0 || stack.ambiguous {
		return math.Inf(1)
	}
	signalIndex, found := stack.stackIndexes[normalizedCopperLayerName(signalLayer)]
	if !found {
		return math.Inf(1)
	}
	minimumIndex, maximumIndex, valid := viaLayerBounds(via, stack)
	if !valid {
		return math.Inf(1)
	}
	if signalIndex >= minimumIndex && signalIndex <= maximumIndex {
		return 0
	}
	delta := minimumIndex - signalIndex
	if signalIndex > maximumIndex {
		delta = signalIndex - maximumIndex
	}
	return float64(delta) * stack.thicknessMM / float64(len(stack.names)-1)
}

func canonicalRouteLayers(route routing.Route, stack copperLayerStack) routing.Route {
	route.Segments = append([]routing.Segment(nil), route.Segments...)
	for index := range route.Segments {
		if canonical := stack.canonicalName(route.Segments[index].Layer); canonical != "" {
			route.Segments[index].Layer = canonical
		}
	}
	route.Vias = append([]routing.Via(nil), route.Vias...)
	for index := range route.Vias {
		route.Vias[index].Layers = append([]string(nil), route.Vias[index].Layers...)
		for layerIndex := range route.Vias[index].Layers {
			if canonical := stack.canonicalName(route.Vias[index].Layers[layerIndex]); canonical != "" {
				route.Vias[index].Layers[layerIndex] = canonical
			}
		}
	}
	return route
}

func pointToSegmentDistance(point, start, end routing.Point) float64 {
	dx := end.XMM - start.XMM
	dy := end.YMM - start.YMM
	if dx == 0 && dy == 0 {
		return math.Hypot(point.XMM-start.XMM, point.YMM-start.YMM)
	}
	projection := ((point.XMM-start.XMM)*dx + (point.YMM-start.YMM)*dy) / (dx*dx + dy*dy)
	projection = math.Max(0, math.Min(1, projection))
	nearestX := start.XMM + projection*dx
	nearestY := start.YMM + projection*dy
	return math.Hypot(point.XMM-nearestX, point.YMM-nearestY)
}

func finalizeExplicitRouteOperations(operations []routing.Operation, placed *PlacementStageResult) ([]transactions.Operation, []reports.Issue) {
	transactionOperations := transactionRouteOperations(operations)
	physical := newPhysicalPadRoutingContext(placed)
	physicalEvidence := BuildInterBlockContactTargets(physical.candidates, placed)
	return postProcessRouteOperations(transactionOperations, placed, physical, physicalEvidence)
}

func expandExplicitPhysicalPadEndpoints(request routing.Request) routing.Request {
	request.Components = append([]routing.Component(nil), request.Components...)
	for componentIndex := range request.Components {
		request.Components[componentIndex].Pads = append([]routing.Pad(nil), request.Components[componentIndex].Pads...)
	}
	request.Nets = append([]routing.Net(nil), request.Nets...)
	for netIndex := range request.Nets {
		request.Nets[netIndex].Endpoints = append([]routing.Endpoint(nil), request.Nets[netIndex].Endpoints...)
	}
	endpointKeys := make(map[string]map[string]struct{}, len(request.Nets))
	participatingRefs := make(map[string]map[string]struct{}, len(request.Nets))
	netIndexes := make(map[string]int, len(request.Nets))
	for netIndex := range request.Nets {
		netKey := strings.ToUpper(strings.TrimSpace(request.Nets[netIndex].Name))
		netIndexes[netKey] = netIndex
		endpointKeys[netKey] = map[string]struct{}{}
		participatingRefs[netKey] = map[string]struct{}{}
		for _, endpoint := range request.Nets[netIndex].Endpoints {
			ref := strings.ToUpper(strings.TrimSpace(endpoint.Ref))
			pin := strings.ToUpper(strings.TrimSpace(endpoint.Pin))
			endpointKeys[netKey][ref+"."+pin] = struct{}{}
			participatingRefs[netKey][ref] = struct{}{}
		}
	}
	for componentIndex := range request.Components {
		component := &request.Components[componentIndex]
		names := make([]string, len(component.Pads))
		for padIndex, pad := range component.Pads {
			names[padIndex] = pad.Name
		}
		aliases := uniqueRoutingPadNames(names)
		for padIndex := range component.Pads {
			component.Pads[padIndex].Name = aliases[padIndex]
		}
		ref := strings.ToUpper(strings.TrimSpace(component.Ref))
		for _, pad := range component.Pads {
			netKey := strings.ToUpper(strings.TrimSpace(pad.Net))
			netIndex, exists := netIndexes[netKey]
			if !exists {
				continue
			}
			if _, participates := participatingRefs[netKey][ref]; !participates {
				continue
			}
			pin := strings.TrimSpace(pad.Name)
			key := ref + "." + strings.ToUpper(pin)
			if pin == "" {
				continue
			}
			if _, exists := endpointKeys[netKey][key]; exists {
				continue
			}
			endpointKeys[netKey][key] = struct{}{}
			request.Nets[netIndex].Endpoints = append(request.Nets[netIndex].Endpoints, routing.Endpoint{Ref: component.Ref, Pin: pin})
		}
	}
	return request
}

func explicitRequiredRouteIssues(nets []ExplicitNetSpec, result routing.Result) []reports.Issue {
	routed := map[string]bool{}
	for _, route := range result.Routes {
		routed[route.Net] = route.Status == routing.RouteStatusRouted
	}
	var issues []reports.Issue
	for _, net := range nets {
		if net.Required && !routed[net.Name] {
			issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Stage: string(StageRouting), Path: "explicit_circuit.nets." + net.Name, Message: "required explicit net was not completely routed", Nets: []string{net.Name}})
		}
	}
	return issues
}

func explicitPlacementEdge(edge string) placement.EdgeConstraint {
	switch strings.ToLower(strings.TrimSpace(edge)) {
	case "left":
		return placement.EdgeLeft
	case "right":
		return placement.EdgeRight
	case "top":
		return placement.EdgeTop
	case "bottom":
		return placement.EdgeBottom
	default:
		return placement.EdgeNone
	}
}

func explicitPlacementNetRole(role string) placement.NetRole {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "power", "power_pos", "power_neg":
		return placement.NetPower
	case "ground", "return", "shield":
		return placement.NetGround
	case "clock":
		return placement.NetClock
	case "analog", "feedback", "bias", "timing":
		return placement.NetAnalog
	default:
		return placement.NetSignal
	}
}

func explicitNetWeight(net ExplicitNetSpec) int {
	weight := 4
	if net.Required {
		weight += 6
	}
	// A shared ground or return net often touches most of the board. Giving
	// its aggregate current an additional placement bonus overwhelms local
	// signal and power-rail relationships instead of improving return paths.
	if explicitPlacementNetRole(net.Role) == placement.NetGround {
		return weight
	}
	if net.CurrentMA >= 500 {
		weight += 4
	} else if net.CurrentMA > 0 {
		weight += 2
	}
	return weight
}

func explicitZoneOperations(request Request) ([]transactions.Operation, []reports.Issue) {
	if request.ExplicitCircuit == nil {
		return nil, nil
	}
	inset := max(request.Board.EdgeClearanceMM, 0.25)
	polygon := []transactions.Point{{XMM: inset, YMM: inset}, {XMM: request.Board.WidthMM - inset, YMM: inset}, {XMM: request.Board.WidthMM - inset, YMM: request.Board.HeightMM - inset}, {XMM: inset, YMM: request.Board.HeightMM - inset}}
	counts := map[string]int{}
	for _, zone := range request.ExplicitCircuit.Zones {
		counts[zone.Net]++
	}
	var operations []transactions.Operation
	var issues []reports.Issue
	for _, zone := range request.ExplicitCircuit.Zones {
		net := zone.Net
		name := "explicit_" + zone.Net
		if counts[zone.Net] > 1 {
			name += "_" + strings.NewReplacer(".", "_", "/", "_").Replace(strings.Join(zone.Layers, "_"))
		}
		appendExplicitOperationToSlice(&operations, transactions.OpAddZone, transactions.AddZoneOperation{Op: transactions.OpAddZone, Name: name, NetName: &net, Layers: append([]string(nil), zone.Layers...), Polygon: polygon, ClearanceMM: zone.ClearanceMM}, &issues)
	}
	return operations, issues
}

func appendExplicitOperationToSlice(operations *[]transactions.Operation, kind transactions.OperationKind, payload any, issues *[]reports.Issue) {
	op, err := workflowOperation(kind, payload)
	if err != nil {
		*issues = append(*issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "explicit_circuit.transaction", Message: err.Error()})
		return
	}
	*operations = append(*operations, op)
}

func explicitPlacementWriteOperations(source []transactions.Operation, index *libraryresolver.LibraryIndex) ([]transactions.Operation, []reports.Issue) {
	operations := make([]transactions.Operation, 0, len(source))
	var issues []reports.Issue
	for operationIndex, operation := range source {
		if operation.Op != transactions.OpPlaceFootprint {
			operations = append(operations, operation)
			continue
		}
		var payload transactions.PlaceFootprintOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			issues = append(issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "explicit_circuit.placement_operations", Message: err.Error()})
			continue
		}
		// Prefer authoritative resolver geometry when it is complete. Otherwise
		// retain the already-hydrated generic placement geometry so an offline
		// workflow writes the exact pad anchors that routing used.
		if record, ok := resolvedFootprintWithPadGeometry(index, payload.FootprintID); ok && len(record.Pads) != 0 {
			for padIndex, pad := range payload.Pads {
				payload.Pads[padIndex] = transactions.PadSpec{Name: pad.Name, Net: pad.Net}
			}
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			issues = append(issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "explicit_circuit.placement_operations", Message: err.Error()})
			continue
		}
		converted := transactions.NewOperationWithMetadata(transactions.OpPlaceFootprint, raw, payload.Ref, "")
		converted.Index = operationIndex
		operations = append(operations, converted)
	}
	return operations, issues
}

func resolvedFootprintWithPadGeometry(index *libraryresolver.LibraryIndex, footprintID string) (libraryresolver.FootprintRecord, bool) {
	if index == nil {
		return libraryresolver.FootprintRecord{}, false
	}
	record, ok := libraryresolver.ResolveFootprint(*index, strings.TrimSpace(footprintID))
	if !ok || len(record.Pads) == 0 {
		return libraryresolver.FootprintRecord{}, false
	}
	for _, pad := range record.Pads {
		if strings.TrimSpace(pad.Name) == "" || pad.Size.X <= 0 || pad.Size.Y <= 0 {
			return libraryresolver.FootprintRecord{}, false
		}
	}
	return record, true
}
