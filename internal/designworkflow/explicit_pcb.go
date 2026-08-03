package designworkflow

import (
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
		placementRequest.Components = append(placementRequest.Components, placement.Component{
			Ref: component.Reference, Value: component.Value, FootprintID: component.FootprintID,
			Role: component.Role, Edge: explicitPlacementEdge(component.Placement.Edge), Priority: component.Placement.Priority,
			Rotation: placement.RotationConstraint{AllowedDeg: []float64{0}},
			Side:     placement.SideTop, Mobility: placement.MobilityPolicy{
				Class: placement.MobilitySoftPreferred, Reason: "catalog-resolved graph placement",
				OwnerScope: "explicit-circuit", RouteHandling: placement.RouteHandlingInvalidateRebuild,
				Transforms: []string{"translate"}, Constraints: []string{"catalog_resolved"},
			},
		})
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
		if component.Placement.Region != "" {
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
	issues = append(issues, result.Issues...)
	stage := NewStageResult(StagePlacement, issues)
	stage.Summary = map[string]any{
		"component_count": result.Metrics.ComponentCount, "placed_count": result.Metrics.PlacedCount,
		"unplaced_count": result.Metrics.UnplacedCount, "region_rule_count": len(placementRequest.RegionRules),
		"proximity_rule_count": len(placementRequest.ProximityRules), "pad_hydration": summarizePadHydration(padEntries, padIssues),
	}
	if result.Status != placement.StatusPlaced && stage.Status == StageStatusOK {
		stage.Status = StageStatusWarning
	}
	return PlacementStageResult{Request: placementRequest, Result: result, Stage: stage}
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
		"route_endpoint_tail_cleanup": endpointTailCleanup,
		"dangling_route_vias_pruned":  danglingRouteViasPruned,
		"return_path_evidence":        returnPathEvidence,
	}
	if result.Status != routing.StatusRouted && stage.Status == StageStatusOK {
		stage.Status = StageStatusWarning
	}
	return RoutingStageResult{Request: routingRequest, Result: result, Operations: operations, Stage: stage}
}

type ExplicitReturnPathEvidence struct {
	Net                string   `json:"net"`
	ReturnNet          string   `json:"return_net"`
	PreferredLayer     string   `json:"preferred_layer,omitempty"`
	PreferredLayerUsed bool     `json:"preferred_layer_used"`
	UsedLayers         []string `json:"used_layers"`
	ReturnPlaneLayers  []string `json:"return_plane_layers,omitempty"`
	MaxLengthMM        float64  `json:"max_length_mm,omitempty"`
	RouteLengthMM      float64  `json:"route_length_mm"`
	MaxDistanceMM      float64  `json:"max_distance_mm"`
	WorstDistanceMM    float64  `json:"worst_distance_mm"`
	SampleCount        int      `json:"sample_count"`
	Pass               bool     `json:"pass"`
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
	copperLayers := make(map[string]bool, len(boardLayers))
	for _, layer := range copperLayerNames(boardLayers) {
		copperLayers[layer] = true
	}
	var evidence []ExplicitReturnPathEvidence
	var issues []reports.Issue
	for _, net := range nets {
		if net.ReturnNet == "" || net.ReturnPathMaxDistanceMM <= 0 {
			continue
		}
		signal := routesByNet[net.Name]
		returnPath := routesByNet[net.ReturnNet]
		var returnPlaneLayers []string
		for _, zone := range zones {
			if zone.Net != net.ReturnNet {
				continue
			}
			for _, layer := range zone.Layers {
				if copperLayers[layer] && !slices.Contains(returnPlaneLayers, layer) {
					returnPlaneLayers = append(returnPlaneLayers, layer)
				}
			}
		}
		slices.Sort(returnPlaneLayers)
		item := ExplicitReturnPathEvidence{
			Net: net.Name, ReturnNet: net.ReturnNet, PreferredLayer: net.PreferLayer, MaxLengthMM: net.MaxLengthMM,
			MaxDistanceMM: net.ReturnPathMaxDistanceMM, UsedLayers: []string{},
			ReturnPlaneLayers: returnPlaneLayers, Pass: true,
		}
		if (len(signal.Segments) == 0 && len(signal.Vias) == 0) ||
			(len(returnPath.Segments) == 0 && len(returnPath.Vias) == 0 && len(returnPlaneLayers) == 0) {
			item.Pass = false
		} else {
			usedLayers := map[string]bool{}
			for _, segment := range signal.Segments {
				usedLayers[segment.Layer] = true
				item.RouteLengthMM += math.Hypot(segment.End.XMM-segment.Start.XMM, segment.End.YMM-segment.Start.YMM)
				samples := []routing.Point{
					segment.Start,
					{XMM: (segment.Start.XMM + segment.End.XMM) / 2, YMM: (segment.Start.YMM + segment.End.YMM) / 2},
					segment.End,
				}
				for _, sample := range samples {
					distance := nearestReturnConductorDistance(
						sample, segment.Layer, returnPath, returnPlaneLayers, boardLayers, boardThicknessMM,
					)
					item.SampleCount++
					item.WorstDistanceMM = math.Max(item.WorstDistanceMM, distance)
					if distance > net.ReturnPathMaxDistanceMM {
						item.Pass = false
					}
				}
			}
			for _, via := range signal.Vias {
				viaLayers := copperLayerNames(boardLayers)
				if len(via.Layers) != 0 {
					viaLayers = append([]string(nil), via.Layers...)
				}
				for _, layer := range viaLayers {
					usedLayers[layer] = true
					distance := nearestReturnConductorDistance(
						via.At, layer, returnPath, returnPlaneLayers, boardLayers, boardThicknessMM,
					)
					item.SampleCount++
					item.WorstDistanceMM = math.Max(item.WorstDistanceMM, distance)
					if distance > net.ReturnPathMaxDistanceMM {
						item.Pass = false
					}
				}
				item.RouteLengthMM += viaVerticalSpanMM(via, boardLayers, boardThicknessMM)
			}
			for layer := range usedLayers {
				item.UsedLayers = append(item.UsedLayers, layer)
			}
			slices.Sort(item.UsedLayers)
			item.PreferredLayerUsed = net.PreferLayer == "" || usedLayers[net.PreferLayer]
			if !item.PreferredLayerUsed || (net.MaxLengthMM > 0 && item.RouteLengthMM > net.MaxLengthMM) {
				item.Pass = false
			}
		}
		evidence = append(evidence, item)
		if !item.Pass {
			issues = append(issues, reports.Issue{
				Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Stage: string(StageRouting),
				Path:       "explicit_circuit.nets." + net.Name + ".return_path",
				Message:    "routed net violates its declared route-length, preferred-layer, or return-path bound",
				Nets:       []string{net.Name, net.ReturnNet},
				Suggestion: "shorten the route, use the preferred layer, or move the return conductor closer",
			})
		}
	}
	return evidence, issues
}

func nearestReturnConductorDistance(
	point routing.Point,
	signalLayer string,
	route routing.Route,
	returnPlaneLayers []string,
	boardLayers []routing.Layer,
	boardThicknessMM float64,
) float64 {
	distance := math.Inf(1)
	for _, segment := range route.Segments {
		planarDistance := pointToSegmentDistance(point, segment.Start, segment.End)
		layerDistance := copperLayerSeparationMM(signalLayer, segment.Layer, boardLayers, boardThicknessMM)
		distance = math.Min(distance, math.Hypot(planarDistance, layerDistance))
	}
	for _, via := range route.Vias {
		distance = math.Min(distance, math.Hypot(point.XMM-via.At.XMM, point.YMM-via.At.YMM))
	}
	for _, planeLayer := range returnPlaneLayers {
		if planeLayer == signalLayer {
			continue
		}
		distance = math.Min(distance, copperLayerSeparationMM(signalLayer, planeLayer, boardLayers, boardThicknessMM))
	}
	return distance
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

func copperLayerSeparationMM(left, right string, layers []routing.Layer, boardThicknessMM float64) float64 {
	if left == right {
		return 0
	}
	names := copperLayerNames(layers)
	// Generated boards declare copper order and total thickness but not a
	// manufacturer-specific core/prepreg build. Use the deterministic uniform
	// stack model implied by that declaration; the emitted evidence remains
	// reproducible and can be replaced by explicit stackup geometry later.
	if len(names) < 2 || !finiteScalar(boardThicknessMM) || boardThicknessMM <= 0 {
		return math.Inf(1)
	}
	leftIndex, rightIndex := -1, -1
	for index, name := range names {
		if name == left {
			leftIndex = index
		}
		if name == right {
			rightIndex = index
		}
	}
	if leftIndex < 0 || rightIndex < 0 {
		return math.Inf(1)
	}
	return math.Abs(float64(leftIndex-rightIndex)) * boardThicknessMM / float64(len(names)-1)
}

func viaVerticalSpanMM(via routing.Via, layers []routing.Layer, boardThicknessMM float64) float64 {
	names := copperLayerNames(layers)
	if len(names) < 2 {
		return boardThicknessMM
	}
	if len(via.Layers) < 2 {
		return boardThicknessMM
	}
	return copperLayerSeparationMM(via.Layers[0], via.Layers[len(via.Layers)-1], layers, boardThicknessMM)
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
