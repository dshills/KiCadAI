package designworkflow

import (
	"context"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/routingadapters"
	"kicadai/internal/transactions"
)

type RoutingOptions struct {
	Skip         bool
	Mode         routing.RouteMode
	GridMM       float64
	TraceWidthMM float64
	ClearanceMM  float64
	AllowPartial *bool
}

type RoutingStageResult struct {
	Request    routing.Request          `json:"request"`
	Result     routing.Result           `json:"result"`
	Operations []transactions.Operation `json:"operations,omitempty"`
	Stage      StageResult              `json:"stage"`
}

type LocalRouteConnectivitySummary struct {
	RoutesAttempted        int `json:"routes_attempted"`
	RoutesBound            int `json:"routes_bound"`
	EndpointsResolved      int `json:"endpoints_resolved"`
	EndpointsUnresolved    int `json:"endpoints_unresolved"`
	EndpointContactsProven int `json:"endpoint_contacts_proven"`
	EndpointNetMismatches  int `json:"endpoint_net_mismatches"`
	EmittedTrackSegments   int `json:"emitted_track_segments"`
	IssueCount             int `json:"issue_count"`
}

func RoutePlacement(ctx context.Context, request Request, fragments PCBFragmentResult, placed PlacementStageResult, opts RoutingOptions) RoutingStageResult {
	normalized := NormalizeRequest(request)
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return RoutingStageResult{Stage: NewStageResult(StageRouting, []reports.Issue{{
			Code:     reports.CodeOperationCanceled,
			Severity: reports.SeverityBlocked,
			Message:  err.Error(),
		}})}
	}
	localOperations, localRouteIssues, localRouteConnectivity := localRouteOperations(fragments, &placed)
	localRouteMobility := classifyLocalRouteMobility(fragments, placed.Request)
	if opts.Skip || normalized.Validation.SkipRouting {
		stage := StageResult{Name: StageRouting, Status: StageStatusSkipped, Summary: map[string]any{
			"reason":               "routing skipped",
			"local_route_mobility": localRouteMobility,
			"route_connectivity":   localRouteConnectivity,
		}}
		stage.Issues = append(stage.Issues, localRouteIssues...)
		anchorSummary, _, anchorIssues := anchorBindingDiagnostics(normalized, fragments, placed, false, opts)
		reportAnchorDiagnostics(&stage, anchorSummary, anchorIssues)
		return RoutingStageResult{Operations: localOperations, Stage: stage}
	}
	if placed.Stage.Status == StageStatusBlocked || reports.HasBlockingIssue(placed.Stage.Issues) {
		stage := StageResult{Name: StageRouting, Status: StageStatusSkipped, Summary: map[string]any{
			"reason":               "placement did not complete",
			"local_route_mobility": localRouteMobility,
			"route_connectivity":   localRouteConnectivity,
		}}
		stage.Issues = append(stage.Issues, localRouteIssues...)
		anchorSummary, _, anchorIssues := anchorBindingDiagnostics(normalized, fragments, placed, false, opts)
		reportAnchorDiagnostics(&stage, anchorSummary, anchorIssues)
		return RoutingStageResult{Operations: localOperations, Stage: stage}
	}
	anchorBindings, anchorOperations, anchorIssues := anchorBindingDiagnostics(normalized, fragments, placed, true, opts)

	routingRequest, issues := routingadapters.RequestFromPlacement(placed.Request, placed.Result)
	issues = append(issues, localRouteIssues...)
	issues = append(issues, anchorIssues...)
	applyRoutingOptions(normalized, opts, &routingRequest)
	result := routing.Result{Status: routing.StatusBlocked}
	if !reports.HasBlockingIssue(issues) {
		result = routing.RouteRequestContext(ctx, routingRequest)
		issues = append(issues, result.Issues...)
	}
	routeOperations := transactionRouteOperations(result.Operations)
	operations := append(localOperations, anchorOperations...)
	operations = append(operations, routeOperations...)
	stage := NewStageResult(StageRouting, issues)
	routeDiagnostics := routing.DiagnosticsForResult(result)
	stage.Summary = map[string]any{
		"local_route_operations": len(localOperations),
		"route_operations":       len(result.Operations),
		"routed_nets":            result.Metrics.RoutedNetCount,
		"failed_nets":            result.Metrics.FailedNetCount,
		"status":                 result.Status,
		"repair_diagnostics":     len(routeDiagnostics),
		"local_route_mobility":   localRouteMobility,
		"route_connectivity":     localRouteConnectivity,
	}
	if len(anchorOperations) > 0 {
		stage.Summary["anchor_binding_route_operations"] = len(anchorOperations)
	}
	addAnchorBindingSummaryToStage(&stage, anchorBindings)
	if result.Quality != nil {
		stage.Summary["quality_score"] = result.Quality.Score.Overall
		stage.Summary["route_reports"] = len(result.Quality.NetReports)
	}
	return RoutingStageResult{Request: routingRequest, Result: result, Operations: operations, Stage: stage}
}

func anchorBindingDiagnostics(request Request, fragments PCBFragmentResult, placed PlacementStageResult, route bool, opts RoutingOptions) (AnchorBindingSummary, []transactions.Operation, []reports.Issue) {
	if !fragmentsHaveEntryAnchors(fragments) {
		return AnchorBindingSummary{}, nil, nil
	}
	endpoints, endpointIssues := DiscoverPhysicalEndpointsWithOptions(placed, PhysicalEndpointDiscoveryOptions{
		ExternalEndpoints: request.ExternalEndpoints,
		Board:             request.Board,
	})
	summary := ResolveAnchorBindings(fragments, endpoints, AnchorBindingOptions{})
	var operations []transactions.Operation
	if route {
		summary, operations = AddAnchorBindingRoutes(summary, AnchorBindingRouteOptions{WidthMM: opts.TraceWidthMM})
	}
	issues := append([]reports.Issue(nil), endpointIssues...)
	issues = append(issues, AnchorBindingIssuesToReports("anchor_bindings", summary.Issues)...)
	return summary, operations, issues
}

func reportAnchorDiagnostics(stage *StageResult, summary AnchorBindingSummary, issues []reports.Issue) {
	if stage == nil {
		return
	}
	stage.Issues = append(stage.Issues, issues...)
	addAnchorBindingSummaryToStage(stage, summary)
}

func addAnchorBindingSummaryToStage(stage *StageResult, summary AnchorBindingSummary) {
	if stage == nil || summary.Total == 0 && summary.IssueCount == 0 {
		return
	}
	if stage.Summary == nil {
		stage.Summary = map[string]any{}
	}
	stage.Summary["anchor_bindings"] = summary
}

func fragmentsHaveEntryAnchors(fragments PCBFragmentResult) bool {
	for _, fragment := range fragments.Fragments {
		if len(fragment.Realization.EntryAnchors) != 0 {
			return true
		}
	}
	return false
}

func applyRoutingOptions(request Request, opts RoutingOptions, routingRequest *routing.Request) {
	if routingRequest == nil {
		return
	}
	if request.Board.Layers <= 1 {
		layerName := preferredSingleLayer(routingRequest)
		routingRequest.Board.Layers = []routing.Layer{{Name: layerName, Kind: routing.LayerCopper, Routable: true}}
		routingRequest.Strategy.Mode = routing.ModeSingleLayer
		routingRequest.Rules.PreferLayer = layerName
		falseValue := false
		routingRequest.Rules.AllowBackLayer = &falseValue
	}
	if opts.Mode != "" {
		routingRequest.Strategy.Mode = opts.Mode
	}
	if opts.GridMM > 0 {
		routingRequest.Rules.GridMM = opts.GridMM
	}
	if opts.TraceWidthMM > 0 {
		routingRequest.Rules.TraceWidthMM = opts.TraceWidthMM
	}
	if opts.ClearanceMM > 0 {
		routingRequest.Rules.ClearanceMM = opts.ClearanceMM
	}
	if opts.AllowPartial != nil {
		routingRequest.Strategy.AllowPartial = *opts.AllowPartial
	}
	if request.Validation.StrictUnrouted {
		routingRequest.Strategy.AllowPartial = false
	}
}

func localRouteOperations(fragments PCBFragmentResult, placed *PlacementStageResult) ([]transactions.Operation, []reports.Issue, LocalRouteConnectivitySummary) {
	if placed == nil || placed.Stage.Status == StageStatusBlocked || len(placed.Request.Components) == 0 {
		summary := LocalRouteConnectivitySummary{RoutesAttempted: localRouteCount(fragments)}
		summary.EndpointsUnresolved = summary.RoutesAttempted * 2
		return preservedLocalRouteOperations(fragments), nil, summary
	}
	table, tableIssues := BuildGeneratedNetTable(placed, nil)
	resolver := NewPlacedPadEndpointResolver(placed, table)
	operations, bindIssues, summary := bindLocalRouteOperations(fragments, resolver)
	operations = append(preservedUnmodeledFragmentOperations(fragments), operations...)
	issues := append([]reports.Issue(nil), tableIssues...)
	issues = append(issues, resolver.Issues()...)
	issues = append(issues, bindIssues...)
	summary.IssueCount = len(issues)
	return operations, issues, summary
}

func preservedLocalRouteOperations(fragments PCBFragmentResult) []transactions.Operation {
	operations := []transactions.Operation{}
	for _, fragment := range fragments.Fragments {
		for _, operation := range fragment.Realization.Operations {
			if preserveFragmentOperationDuringRouting(operation) {
				operations = append(operations, operation)
			}
		}
	}
	return operations
}

func preservedUnmodeledFragmentOperations(fragments PCBFragmentResult) []transactions.Operation {
	operations := []transactions.Operation{}
	for _, fragment := range fragments.Fragments {
		localRouteNets := localRouteNetKeysForFragment(fragment)
		for _, operation := range fragment.Realization.Operations {
			if !preserveFragmentOperationDuringRouting(operation) {
				continue
			}
			if operation.Op != transactions.OpRoute {
				operations = append(operations, operation)
				continue
			}
			if _, modeled := localRouteNets[strings.ToUpper(routeOperationCachedNetName(operation))]; modeled {
				continue
			}
			operations = append(operations, operation)
		}
	}
	return operations
}

func localRouteNetKeysForFragment(fragment BlockFragment) map[string]struct{} {
	keys := map[string]struct{}{}
	for _, route := range fragment.Realization.LocalRoutes {
		if netName := strings.TrimSpace(route.NetName); netName != "" {
			keys[strings.ToUpper(netName)] = struct{}{}
		}
	}
	return keys
}

func preserveFragmentOperationDuringRouting(operation transactions.Operation) bool {
	return operation.Op != transactions.OpPlaceFootprint
}

func routeOperationCachedNetName(operation transactions.Operation) string {
	return strings.TrimSpace(operation.Net)
}

func bindLocalRouteOperations(fragments PCBFragmentResult, resolver PlacedPadEndpointResolver) ([]transactions.Operation, []reports.Issue, LocalRouteConnectivitySummary) {
	operations := []transactions.Operation{}
	var issues []reports.Issue
	summary := LocalRouteConnectivitySummary{}
	for _, fragment := range fragments.Fragments {
		for _, route := range fragment.Realization.LocalRoutes {
			summary.RoutesAttempted++
			operation, routeIssues, routeSummary, ok := bindLocalRouteOperation(fragment, route, resolver)
			issues = append(issues, routeIssues...)
			summary.RoutesBound += routeSummary.RoutesBound
			summary.EndpointsResolved += routeSummary.EndpointsResolved
			summary.EndpointsUnresolved += routeSummary.EndpointsUnresolved
			summary.EndpointContactsProven += routeSummary.EndpointContactsProven
			summary.EndpointNetMismatches += routeSummary.EndpointNetMismatches
			summary.EmittedTrackSegments += routeSummary.EmittedTrackSegments
			if ok {
				operations = append(operations, operation)
			}
		}
	}
	summary.IssueCount = len(issues)
	return operations, issues, summary
}

func bindLocalRouteOperation(fragment BlockFragment, route blocks.RealizedPCBLocalRoute, resolver PlacedPadEndpointResolver) (transactions.Operation, []reports.Issue, LocalRouteConnectivitySummary, bool) {
	var issues []reports.Issue
	summary := LocalRouteConnectivitySummary{RoutesAttempted: 1}
	netName := strings.TrimSpace(route.NetName)
	path := "routes." + firstNonEmpty(fragment.InstanceID, fragment.BlockID, "fragment") + "." + firstNonEmpty(route.ID, netName, "unnamed")
	if netName == "" {
		summary.EndpointsUnresolved = 2
		summary.IssueCount = 1
		return transactions.Operation{}, []reports.Issue{localRouteBindingIssue(path+".net_name", "local route net name is required", nil)}, summary, false
	}
	from, fromIssues, fromOK, fromNetMismatch := resolveLocalRouteEndpoint(path+".from", netName, route.From, resolver)
	to, toIssues, toOK, toNetMismatch := resolveLocalRouteEndpoint(path+".to", netName, route.To, resolver)
	issues = append(issues, fromIssues...)
	issues = append(issues, toIssues...)
	if fromNetMismatch {
		summary.EndpointNetMismatches++
	}
	if toNetMismatch {
		summary.EndpointNetMismatches++
	}
	if fromOK {
		summary.EndpointsResolved++
	} else {
		summary.EndpointsUnresolved++
	}
	if toOK {
		summary.EndpointsResolved++
	} else {
		summary.EndpointsUnresolved++
	}
	if !fromOK || !toOK {
		summary.IssueCount = len(issues)
		return transactions.Operation{}, issues, summary, false
	}
	layer := firstNonEmpty(route.Layer, from.Layer, to.Layer, "F.Cu")
	if !strings.EqualFold(layer, from.Layer) || !strings.EqualFold(layer, to.Layer) {
		issues = append(issues, localRouteBindingIssue(path+".layer", "local route layer "+layer+" does not match endpoint layers "+from.Layer+" and "+to.Layer, []string{from.Ref, to.Ref}))
		summary.IssueCount = len(issues)
		return transactions.Operation{}, issues, summary, false
	}
	operation, err := workflowOperation(transactions.OpRoute, transactions.RouteOperation{
		Op:      transactions.OpRoute,
		NetName: netName,
		Layer:   layer,
		WidthMM: route.WidthMM,
		Points: []transactions.Point{
			from.Point,
			to.Point,
		},
	})
	if err != nil {
		issues = append(issues, localRouteBindingIssue(path, err.Error(), []string{from.Ref, to.Ref}))
		summary.IssueCount = len(issues)
		return transactions.Operation{}, issues, summary, false
	}
	summary.RoutesBound = 1
	summary.EndpointContactsProven = 2
	summary.EmittedTrackSegments = 1
	summary.IssueCount = len(issues)
	return operation, issues, summary, true
}

func resolveLocalRouteEndpoint(path string, netName string, endpoint transactions.Endpoint, resolver PlacedPadEndpointResolver) (PlacedPadEndpoint, []reports.Issue, bool, bool) {
	ref := strings.TrimSpace(endpoint.Ref)
	pin := strings.TrimSpace(endpoint.Pin)
	if ref == "" || pin == "" {
		return PlacedPadEndpoint{}, []reports.Issue{localRouteBindingIssue(path, "local route endpoint requires ref and pin", nil)}, false, false
	}
	resolved, ok := resolver.ResolveNormalized(strings.ToUpper(ref), strings.ToUpper(pin))
	if !ok {
		return PlacedPadEndpoint{}, []reports.Issue{localRouteBindingIssue(path, "local route endpoint does not resolve to a placed pad", []string{ref, ref + "." + pin})}, false, false
	}
	if !resolved.NetCodeResolved {
		return resolved, []reports.Issue{localRouteBindingIssue(path+".net_code", "local route endpoint pad net code is unresolved", []string{ref})}, false, false
	}
	padNet := strings.TrimSpace(resolved.NetName)
	if padNet == "" {
		return resolved, []reports.Issue{localRouteBindingIssue(path+".net_name", "local route endpoint pad has no assigned net", []string{ref})}, false, false
	}
	if !strings.EqualFold(padNet, netName) {
		return resolved, []reports.Issue{localRouteBindingIssue(path+".net_name", "local route endpoint pad net "+padNet+" does not match route net "+netName, []string{ref})}, false, true
	}
	return resolved, nil, true, false
}

func localRouteBindingIssue(path string, message string, refs []string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityBlocked,
		Path:     "design.route_connectivity." + strings.Trim(path, "."),
		Message:  message,
		Refs:     append([]string(nil), refs...),
	}
}

func localRouteCount(fragments PCBFragmentResult) int {
	count := 0
	for _, fragment := range fragments.Fragments {
		count += len(fragment.Realization.LocalRoutes)
	}
	return count
}

func preferredSingleLayer(request *routing.Request) string {
	layerCounts := map[string]int{}
	for _, component := range request.Components {
		layerName := component.Position.Layer
		if layerName == "" {
			continue
		}
		padCount := len(component.Pads)
		if padCount == 0 {
			padCount = 1
		}
		layerCounts[layerName] += padCount
	}
	bestLayer := ""
	bestCount := 0
	for _, layer := range request.Board.Layers {
		if !layer.Routable || layer.Kind != routing.LayerCopper || layer.Name == "" {
			continue
		}
		if count := layerCounts[layer.Name]; count > bestCount {
			bestLayer = layer.Name
			bestCount = count
		}
	}
	if bestLayer != "" {
		return bestLayer
	}
	for _, layer := range request.Board.Layers {
		if layer.Routable && layer.Kind == routing.LayerCopper && layer.Name != "" {
			return layer.Name
		}
	}
	return "F.Cu"
}

func transactionRouteOperations(operations []routing.Operation) []transactions.Operation {
	out := make([]transactions.Operation, 0, len(operations))
	for index, operation := range operations {
		if operation.Op != string(transactions.OpRoute) || len(operation.Raw) == 0 {
			continue
		}
		txOperation := transactions.NewOperation(transactions.OpRoute, operation.Raw)
		txOperation.Index = index
		out = append(out, txOperation)
	}
	return out
}
