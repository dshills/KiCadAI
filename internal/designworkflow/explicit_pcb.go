package designworkflow

import (
	"context"
	"encoding/json"
	"strings"

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
	placementRequest := placement.Request{
		Board: placement.BoardPlacementArea{WidthMM: request.Board.WidthMM, HeightMM: request.Board.HeightMM, MarginMM: request.Board.EdgeClearanceMM},
		Rules: mergePlacementRules(opts.Rules), Seed: request.ExplicitCircuit.ResolutionHash,
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
	routingRequest = expandExplicitPhysicalPadEndpoints(routingRequest)
	applyRoutingOptions(request, opts, &routingRequest)
	if routingRequest.Rules.NetOverrides == nil {
		routingRequest.Rules.NetOverrides = map[string]routing.NetRule{}
	}
	for _, net := range request.ExplicitCircuit.Nets {
		if net.NetClass == "" && net.WidthMM == 0 && net.ClearanceMM == 0 {
			continue
		}
		rule := routingRequest.Rules.NetOverrides[net.Name]
		if net.NetClass != "" {
			rule.ClassName = net.NetClass
		}
		rule.TraceWidthMM = net.WidthMM
		rule.ClearanceMM = net.ClearanceMM
		routingRequest.Rules.NetOverrides[net.Name] = rule
	}
	result := routing.Result{Status: routing.StatusBlocked}
	if !reports.HasBlockingIssue(issues) {
		result = routing.RouteRequestContext(ctx, routingRequest)
		issues = append(issues, result.Issues...)
	}
	issues = append(issues, explicitRequiredRouteIssues(request.ExplicitCircuit.Nets, result)...)
	operations := compactRouteOperationGeometry(transactionRouteOperations(result.Operations))
	stage := NewStageResult(StageRouting, issues)
	stage.Summary = map[string]any{
		"status": result.Status, "net_count": result.Metrics.NetCount, "routed_nets": result.Metrics.RoutedNetCount,
		"failed_nets": result.Metrics.FailedNetCount, "route_operations": len(operations),
	}
	if result.Status != routing.StatusRouted && stage.Status == StageStatusOK {
		stage.Status = StageStatusWarning
	}
	return RoutingStageResult{Request: routingRequest, Result: result, Operations: operations, Stage: stage}
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
			issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "explicit_circuit.nets." + net.Name, Message: "required explicit net was not completely routed", Nets: []string{net.Name}})
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
	case "power", "power_pos", "power_neg", "bias":
		return placement.NetPower
	case "ground", "return", "shield":
		return placement.NetGround
	case "clock":
		return placement.NetClock
	case "analog", "feedback":
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
	var operations []transactions.Operation
	var issues []reports.Issue
	for _, zone := range request.ExplicitCircuit.Zones {
		net := zone.Net
		appendExplicitOperationToSlice(&operations, transactions.OpAddZone, transactions.AddZoneOperation{Op: transactions.OpAddZone, Name: "explicit_" + zone.Net, NetName: &net, Layers: append([]string(nil), zone.Layers...), Polygon: polygon, ClearanceMM: zone.ClearanceMM}, &issues)
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

func explicitPlacementWriteOperations(source []transactions.Operation) ([]transactions.Operation, []reports.Issue) {
	operations := make([]transactions.Operation, 0, len(source))
	var issues []reports.Issue
	for index, operation := range source {
		if operation.Op != transactions.OpPlaceFootprint {
			operations = append(operations, operation)
			continue
		}
		var payload transactions.PlaceFootprintOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			issues = append(issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "explicit_circuit.placement_operations", Message: err.Error()})
			continue
		}
		for padIndex, pad := range payload.Pads {
			payload.Pads[padIndex] = transactions.PadSpec{Name: pad.Name, Net: pad.Net}
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			issues = append(issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "explicit_circuit.placement_operations", Message: err.Error()})
			continue
		}
		converted := transactions.NewOperationWithMetadata(transactions.OpPlaceFootprint, raw, payload.Ref, "")
		converted.Index = index
		operations = append(operations, converted)
	}
	return operations, issues
}
