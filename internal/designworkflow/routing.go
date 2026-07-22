package designworkflow

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"kicadai/internal/blocks"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/routingadapters"
	"kicadai/internal/transactions"
)

type RoutingOptions struct {
	Skip                bool
	Mode                routing.RouteMode
	GridMM              float64
	TraceWidthMM        float64
	ClearanceMM         float64
	AllowPartial        *bool
	ComponentSelections []ComponentSelectionEntry
}

type RoutingStageResult struct {
	Request    routing.Request          `json:"request"`
	Result     routing.Result           `json:"result"`
	Operations []transactions.Operation `json:"operations,omitempty"`
	Stage      StageResult              `json:"stage"`
}

const interBlockRouteSnapMaxDistanceMM = 0.75

const localRouteShapeEndpointDeltaMismatchMaxMM = 25.0

const localRouteEntryAnchorSource = "pcb_realization.entry_anchor"

const localRouteEndpointDogboneOperationRef = "local_route.endpoint_dogbone"

const localRouteRebuildMaxRouterCalls = 256

const localRouteRebuildMaxDuration = 30 * time.Second

const (
	defaultLocalRouteTopLayer    = "F.Cu"
	defaultLocalRouteBottomLayer = "B.Cu"
)

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

type InterBlockRouteCompletionSummary struct {
	NetsConsidered      int `json:"nets_considered"`
	Candidates          int `json:"candidates"`
	RoutesAttempted     int `json:"routes_attempted"`
	RoutesCompleted     int `json:"routes_completed"`
	EndpointsResolved   int `json:"endpoints_resolved"`
	EndpointsUnresolved int `json:"endpoints_unresolved"`
	PartialNets         int `json:"partial_nets"`
	UnroutedNets        int `json:"unrouted_nets"`
	EmittedSegments     int `json:"emitted_segments"`
	IssueCount          int `json:"issue_count"`
	MultiEndpointNets   int `json:"multi_endpoint_nets"`
	RequiredEndpoints   int `json:"required_endpoints"`
	ProvenEndpoints     int `json:"proven_endpoints"`
	BranchesPlanned     int `json:"branches_planned"`
	BranchesAttempted   int `json:"branches_attempted"`
	BranchesCompleted   int `json:"branches_completed"`
	GraphComponentCount int `json:"graph_component_count"`
	MissingRequired     int `json:"missing_required_endpoints"`
	CompleteGroups      int `json:"complete_groups"`
	PartialGroups       int `json:"partial_groups"`
	BlockedGroups       int `json:"blocked_groups"`
}

type InterBlockRouteTreeExecutionSummary struct {
	GroupsPlanned       int      `json:"groups_planned"`
	GroupsAttempted     int      `json:"groups_attempted"`
	GroupsComplete      int      `json:"groups_complete"`
	GroupsPartial       int      `json:"groups_partial"`
	GroupsBlocked       int      `json:"groups_blocked"`
	BranchesPlanned     int      `json:"branches_planned"`
	BranchesAttempted   int      `json:"branches_attempted"`
	BranchesRouted      int      `json:"branches_routed"`
	BranchesBlocked     int      `json:"branches_blocked"`
	ContactMisses       int      `json:"contact_misses"`
	GraphSplits         int      `json:"graph_splits"`
	IssueCount          int      `json:"issue_count"`
	BlockingIssueCount  int      `json:"blocking_issue_count,omitempty"`
	WarningIssueCount   int      `json:"warning_issue_count,omitempty"`
	InfoIssueCount      int      `json:"info_issue_count,omitempty"`
	FixedNetSkipNotices int      `json:"fixed_net_skip_notices,omitempty"`
	ManagedNets         []string `json:"managed_nets,omitempty"`
	OrderAttempts       int      `json:"order_attempts,omitempty"`
	SelectedOrder       string   `json:"selected_order,omitempty"`
}

type RouteTreeBranchEvidenceSummary struct {
	NetName  string                            `json:"net_name"`
	Branches []InterBlockBranchRoutingEvidence `json:"branches,omitempty"`
}

type PhysicalPadRoutingCompletionSummary struct {
	NetsConsidered int            `json:"nets_considered"`
	Endpoints      int            `json:"endpoints"`
	EndpointsByNet map[string]int `json:"endpoints_by_net,omitempty"`
	ConnectedNets  []string       `json:"connected_nets,omitempty"`
	RemainingNets  []string       `json:"remaining_nets,omitempty"`
}

type ResidualPhysicalRouteTreeSummary struct {
	Candidates    int      `json:"candidates"`
	Attempts      int      `json:"attempts"`
	AttemptedNets []string `json:"attempted_nets,omitempty"`
	CompletedNets []string `json:"completed_nets,omitempty"`
}

type residualPhysicalRouteTreeResult struct {
	Operations []transactions.Operation
	Issues     []reports.Issue
	Summary    ResidualPhysicalRouteTreeSummary
}

type FinalRouteOrderNegotiationSummary struct {
	Attempts      int      `json:"attempts"`
	SelectedOrder string   `json:"selected_order,omitempty"`
	PromotedNets  []string `json:"promoted_nets,omitempty"`
}

type interBlockRouteTreeExecutionResult struct {
	Operations []transactions.Operation
	Issues     []reports.Issue
	Summary    InterBlockRouteTreeExecutionSummary
	Branches   []RouteTreeBranchEvidenceSummary
}

func RoutePlacement(ctx context.Context, request Request, fragments PCBFragmentResult, placed PlacementStageResult, opts RoutingOptions) RoutingStageResult {
	normalized := NormalizeRequest(request)
	if ctx == nil {
		ctx = context.Background()
	}
	if canceled, ok := canceledRoutingStageResult(ctx); ok {
		return canceled
	}
	localOperations, localRouteIssues, localRouteConnectivity := localRouteOperations(fragments, &placed)
	interBlockCandidates, interBlockCandidateIssues := BuildInterBlockRouteCandidates(fragments, placed)
	localRouteMobility := classifyLocalRouteMobility(fragments, placed.Request)
	componentHintResult := componentRoutingHints(opts.ComponentSelections, fragments)
	componentHintIssues := ComponentHintIssues(componentHintResult.Evidence)
	if canceled, ok := canceledRoutingStageResult(ctx); ok {
		return canceled
	}
	if opts.Skip || normalized.Validation.SkipRouting {
		stage := StageResult{Name: StageRouting, Status: StageStatusSkipped, Summary: map[string]any{
			"reason":               "routing skipped",
			"local_route_mobility": localRouteMobility,
			"route_connectivity":   localRouteConnectivity,
			"inter_block_routing":  summarizeInterBlockRouteCompletion(interBlockCandidates, nil, append(localRouteIssues, interBlockCandidateIssues...), InterBlockContactEvidence{}),
		}}
		addComponentHintSummaryToStage(&stage, componentHintResult.Evidence)
		stage.Issues = append(stage.Issues, componentHintIssues...)
		stage.Issues = append(stage.Issues, localRouteIssues...)
		stage.Issues = append(stage.Issues, interBlockCandidateIssues...)
		anchorSummary, _, anchorIssues := anchorBindingDiagnostics(normalized, fragments, placed, false, opts)
		reportAnchorDiagnostics(&stage, anchorSummary, anchorIssues)
		return RoutingStageResult{Operations: localOperations, Stage: stage}
	}
	if placed.Stage.Status == StageStatusBlocked || reports.HasBlockingIssue(placed.Stage.Issues) {
		stage := StageResult{Name: StageRouting, Status: StageStatusSkipped, Summary: map[string]any{
			"reason":               "placement did not complete",
			"local_route_mobility": localRouteMobility,
			"route_connectivity":   localRouteConnectivity,
			"inter_block_routing":  summarizeInterBlockRouteCompletion(interBlockCandidates, nil, append(localRouteIssues, interBlockCandidateIssues...), InterBlockContactEvidence{}),
		}}
		addComponentHintSummaryToStage(&stage, componentHintResult.Evidence)
		stage.Issues = append(stage.Issues, componentHintIssues...)
		stage.Issues = append(stage.Issues, localRouteIssues...)
		stage.Issues = append(stage.Issues, interBlockCandidateIssues...)
		anchorSummary, _, anchorIssues := anchorBindingDiagnostics(normalized, fragments, placed, false, opts)
		reportAnchorDiagnostics(&stage, anchorSummary, anchorIssues)
		return RoutingStageResult{Operations: localOperations, Stage: stage}
	}
	anchorBindings, anchorOperations, anchorIssues := anchorBindingDiagnostics(normalized, fragments, placed, true, opts)

	routingRequest, issues := routingadapters.RequestFromPlacement(placed.Request, placed.Result)
	issues = append(issues, componentHintIssues...)
	issues = append(issues, localRouteIssues...)
	issues = append(issues, interBlockCandidateIssues...)
	issues = append(issues, anchorIssues...)
	applyRoutingOptions(normalized, opts, &routingRequest)
	issues = append(issues, fitRoutingClearanceToIntrinsicPads(&routingRequest, placed.Request.Components, opts.ClearanceMM > 0 || normalized.Constraints.ClearanceMM > 0)...)
	var localRebuildIssues []reports.Issue
	localOperations, localRebuildIssues = rebuildMovedLocalRouteOperations(ctx, routingRequest, localOperations)
	issues = append(issues, localRebuildIssues...)
	if canceled, ok := canceledRoutingStageResult(ctx); ok {
		return canceled
	}
	localRouteConnectivity = localRouteConnectivityWithAdditionalIssues(localRouteConnectivity, len(localRebuildIssues))
	localRouteRoutingBlocked := reports.HasBlockingIssue(localRouteIssues) || reports.HasBlockingIssue(localRebuildIssues)
	if !localRouteRoutingBlocked {
		if requestRequiresStrictDRC(normalized, fragments) {
			// DRC-required fragments commit all block-local copper before global
			// routing. Every inter-block branch must therefore avoid foreign-net
			// via-free signal corridors as well as power traces and vias; same-net
			// merges remain legal because obstacles retain their net identity.
			routingRequest.Existing = append(routingRequest.Existing, existingCopperFromAllRouteOperations(localOperations, routeBranchDefaultLayer(routingRequest.Board), routingRequest.Rules)...)
		} else if normalized.Constraints.TreatLocalPowerRoutesAsObstacles {
			routingRequest.Existing = append(routingRequest.Existing, existingCopperFromRouteOperations(localOperations, routeBranchDefaultLayer(routingRequest.Board), routingRequest.Rules)...)
		} else {
			routingRequest.Existing = append(routingRequest.Existing, existingUSBConfigurationCopperFromRouteOperations(localOperations, routeBranchDefaultLayer(routingRequest.Board), routingRequest.Rules)...)
		}
	}
	selectiveLocalRouteObstacles := []routing.ExistingCopper(nil)
	selectiveLocalRouteObstacleNets := map[string]struct{}{}
	if !localRouteRoutingBlocked {
		addMultiEndpointGroundRouteObstacleNets(selectiveLocalRouteObstacleNets, interBlockCandidates)
		for _, netName := range normalized.Constraints.LocalRouteObstacleNets {
			selectiveLocalRouteObstacleNets[strings.TrimSpace(netName)] = struct{}{}
		}
		if len(selectiveLocalRouteObstacleNets) != 0 {
			// Explicit selective nets and automatic multi-endpoint ground trees
			// route after block-local copper is committed, so they must avoid every
			// foreign-net local trace. Existing copper keeps its net identity,
			// preserving legal same-net merges.
			selectiveLocalRouteObstacles = existingCopperFromAllRouteOperations(localOperations, routeBranchDefaultLayer(routingRequest.Board), routingRequest.Rules)
		}
	}
	baseContactOperations := slices.Concat(localOperations, anchorOperations)
	physicalContext := newPhysicalPadRoutingContext(&placed)
	physicalCandidates := physicalContext.candidates
	targetEvidence := BuildInterBlockContactTargets(interBlockCandidates, &placed)
	routeTreeAccess, routeTreeAccessIssues := BuildRouteTreeEndpointAccessWithIssues(targetEvidence, baseContactOperations)
	issues = append(issues, routeTreeAccessIssues...)
	if canceled, ok := canceledRoutingStageResult(ctx); ok {
		return canceled
	}
	routeTreeExecution := executeInterBlockRouteTrees(ctx, routingRequest, interBlockCandidates, targetEvidence, routeTreeAccess, selectiveLocalRouteObstacles, selectiveLocalRouteObstacleNets)
	if canceled, ok := canceledRoutingStageResult(ctx); ok {
		return canceled
	}
	// Inter-block trees connect constrained, shared block boundaries. Commit those
	// paths before routing unrelated residual pads so the residual router can work
	// around the proven topology instead of consuming endpoint escape corridors.
	routingRequest.Existing = append(routingRequest.Existing, existingCopperFromAllRouteOperations(routeTreeExecution.Operations, routeBranchDefaultLayer(routingRequest.Board), routingRequest.Rules)...)
	issues = append(issues, targetEvidence.Issues...)
	issues = append(issues, routeTreeExecution.Issues...)
	preTreeContacts := slices.Concat(baseContactOperations, routeTreeExecution.Operations)
	preTreeNets, _ := remainingPhysicalPadRoutingNetsWithCandidates(routingRequest.Nets, &placed, preTreeContacts, physicalCandidates)
	preTreeNets = excludeRoutingNetsByName(preTreeNets, interBlockCandidateNets(interBlockCandidates))
	preTreeResult := routing.Result{Status: routing.StatusRouted}
	preTreeOrder := FinalRouteOrderNegotiationSummary{}
	if len(preTreeNets) != 0 && !reports.HasBlockingIssue(issues) {
		preTreeRequest := routingRequest
		preTreeRequest.Nets = preTreeNets
		preTreeResult, preTreeOrder = routeWithFailedNetFirstNegotiation(ctx, preTreeRequest)
	}
	if canceled, ok := canceledRoutingStageResult(ctx); ok {
		return canceled
	}
	preTreeRouteOperations := transactionRouteOperations(preTreeResult.Operations)
	failedPreTreeNets := stringBoolSet(blockingRoutingIssueNets(preTreeResult.Issues, preTreeNets))
	fallbackRequest := routingRequest
	fallbackRequest.Existing = append(append([]routing.ExistingCopper(nil), routingRequest.Existing...), existingCopperFromAllRouteOperations(preTreeRouteOperations, routeBranchDefaultLayer(routingRequest.Board), routingRequest.Rules)...)
	fallbackContacts := slices.Concat(preTreeContacts, preTreeRouteOperations)
	residualTreeExecution := routeResidualPhysicalPadTrees(ctx, fallbackRequest, physicalCandidates, failedPreTreeNets, &placed, fallbackContacts)
	preTreeResult = reconcileContactProvenRoutingResult(preTreeResult, residualTreeExecution.Summary.CompletedNets)
	issues = append(issues, preTreeResult.Issues...)
	issues = append(issues, residualTreeExecution.Issues...)
	preTreeRouteOperations = append(preTreeRouteOperations, residualTreeExecution.Operations...)
	routingRequest.Existing = append(routingRequest.Existing, existingCopperFromAllRouteOperations(preTreeRouteOperations, routeBranchDefaultLayer(routingRequest.Board), routingRequest.Rules)...)
	// Inter-block completion proves only block-boundary endpoints. Retain every
	// placed-pad endpoint for the final router pass, and expose the route-tree
	// branches as fixed net-aware copper so remaining pads can merge into the
	// already proven topology without crossing it.
	physicalContactOperations := slices.Concat(preTreeContacts, preTreeRouteOperations)
	var physicalPadRouting PhysicalPadRoutingCompletionSummary
	routingRequest.Nets, physicalPadRouting = remainingPhysicalPadRoutingNetsWithCandidates(routingRequest.Nets, &placed, physicalContactOperations, physicalCandidates)
	result := routing.Result{Status: routing.StatusBlocked}
	finalRouteOrder := FinalRouteOrderNegotiationSummary{}
	if !reports.HasBlockingIssue(issues) {
		result, finalRouteOrder = routeWithFailedNetFirstNegotiation(ctx, routingRequest)
		issues = append(issues, result.Issues...)
	}
	if canceled, ok := canceledRoutingStageResult(ctx); ok {
		return canceled
	}
	finalRouteOperations := transactionRouteOperations(result.Operations)
	qualityRequest := routingRequest
	qualityRequest.Nets = uniqueRoutingNets(preTreeNets, routingRequest.Nets)
	result = combineSequentialRoutingResults(preTreeResult, result)
	quality := routing.BuildQualityReport(qualityRequest, result)
	result.Quality = &quality
	finalRouteOperations, snapIssues := snapInterBlockRouteEndpoints(interBlockCandidates, finalRouteOperations, &placed)
	issues = append(issues, snapIssues...)
	// Route-tree operations already carry explicit endpoint-access evidence and
	// are validated by the same-net contact graph below. Re-snapping them to an
	// arbitrary pair of net targets would corrupt legitimate copper merges.
	routeOperations := slices.Concat(preTreeRouteOperations, finalRouteOperations, routeTreeExecution.Operations)
	contactGraphOperations := slices.Concat(routeOperations, baseContactOperations)
	interBlockContactEvidence := ValidateInterBlockRouteEndpointContacts(interBlockCandidates, contactGraphOperations, &placed)
	issues = append(issues, interBlockContactEvidence.Issues...)
	issues = suppressProvenRouteDisconnectedIssues(issues, interBlockContactEvidence, routeOperations, localOperations, localRouteConnectivity)
	routeTreeRepairHints := BuildRouteTreeRepairHints(issues)
	operations := slices.Concat(baseContactOperations, routeOperations)
	for operationIndex := 0; operationIndex < len(localOperations); operationIndex++ {
		operations[operationIndex].PruneProtected = true
	}
	physicalContactEvidence := BuildInterBlockContactTargets(physicalContext.candidates, &placed)
	var routePostProcessIssues []reports.Issue
	if canceled, ok := canceledRoutingStageResult(ctx); ok {
		return canceled
	}
	operations, routePostProcessIssues = postProcessRouteOperations(operations, &placed, physicalContext, physicalContactEvidence)
	issues = append(issues, routePostProcessIssues...)
	operations, physicalClearanceRepairIssues, physicalClearanceBlockersBeforeRepair, physicalClearanceBlockersAfterRepair, physicalClearanceMM := finalizeEmittedRoutePhysicalClearanceWhenRequired(request.Validation.RequireDRC, routingRequest, operations)
	physicalClearanceRepairIssues, physicalClearanceDeferredToDRC := deferPhysicalClearanceIssuesToRequiredDRC(request.Validation.RequireDRC, physicalClearanceRepairIssues)
	issues = append(issues, physicalClearanceRepairIssues...)
	stage := NewStageResult(StageRouting, issues)
	stage.Issues = cloneIssues(issues)
	routeDiagnostics := routing.DiagnosticsForResult(result)
	routeTreeContactGraph := SummarizeRouteTreeContactGraph(interBlockContactEvidence, contactGraphOperations, routeTreeAccess)
	stage.Summary = map[string]any{
		"local_route_operations":           len(localOperations),
		"route_operations":                 len(result.Operations),
		"routed_nets":                      result.Metrics.RoutedNetCount,
		"failed_nets":                      result.Metrics.FailedNetCount,
		"status":                           result.Status,
		"repair_diagnostics":               len(routeDiagnostics),
		"local_route_mobility":             localRouteMobility,
		"route_connectivity":               localRouteConnectivity,
		"inter_block_routing":              summarizeInterBlockRouteCompletionWithGraphOperations(interBlockCandidates, routeOperations, contactGraphOperations, issues, interBlockContactEvidence),
		"inter_block_route_trees":          routeTreeExecution.Summary,
		"route_tree_branches":              routeTreeExecution.Branches,
		"route_tree_access":                SummarizeRouteTreeEndpointAccess(routeTreeAccess),
		"route_tree_contact_graph":         routeTreeContactGraph,
		"physical_pad_routing":             physicalPadRouting,
		"residual_physical_route_trees":    residualTreeExecution.Summary,
		"pre_tree_route_operations":        len(preTreeRouteOperations),
		"pre_tree_routed_nets":             preTreeResult.Metrics.RoutedNetCount,
		"pre_tree_failed_nets":             preTreeResult.Metrics.FailedNetCount,
		"pre_tree_route_order":             preTreeOrder,
		"final_route_order":                finalRouteOrder,
		"physical_clearance_before_repair": physicalClearanceBlockersBeforeRepair,
		"physical_clearance_after_repair":  physicalClearanceBlockersAfterRepair,
		"physical_clearance_deferred_drc":  physicalClearanceDeferredToDRC,
		"physical_clearance_mm":            physicalClearanceMM,
		"route_tree_missing_endpoints":     SummarizeRouteTreeMissingEndpointTrace(interBlockContactEvidence, routeTreeAccess),
		"required_net_classification":      SummarizeRequiredNetClassification(&routeTreeContactGraph),
		"route_tree_repair":                SummarizeRouteTreeRepair(routeTreeRepairHints),
		"inter_block_contacts":             SummarizeInterBlockContacts(interBlockContactEvidence),
	}
	if len(anchorOperations) > 0 {
		stage.Summary["anchor_binding_route_operations"] = len(anchorOperations)
	}
	addComponentHintSummaryToStage(&stage, componentHintResult.Evidence)
	addAnchorBindingSummaryToStage(&stage, anchorBindings)
	if result.Quality != nil {
		stage.Summary["quality_score"] = result.Quality.Score.Overall
		stage.Summary["route_reports"] = len(result.Quality.NetReports)
	}
	return RoutingStageResult{Request: routingRequest, Result: result, Operations: operations, Stage: stage}
}

// finalizeEmittedRoutePhysicalClearance is the shared last copper-geometry
// boundary for every routing pipeline. The search grid may reduce its working
// clearance to escape intrinsic footprint pad spacing, but the writer still
// emits the project default for ordinary tracks. Validate and repair against
// that unchanged writer-visible floor before returning transaction operations.
func finalizeEmittedRoutePhysicalClearance(request routing.Request, operations []transactions.Operation) ([]transactions.Operation, []reports.Issue, int, int, float64) {
	request.Rules.ClearanceMM = math.Max(request.Rules.ClearanceMM, routing.DefaultRules().ClearanceMM)
	before := blockingIssueCount(routing.ValidatePhysicalTrackClearance(request, routingRoutesFromOperations(operations)))
	repaired, issues := repairEmittedRoutePhysicalClearance(request, operations)
	after := blockingIssueCount(routing.ValidatePhysicalTrackClearance(request, routingRoutesFromOperations(repaired)))
	return repaired, issues, before, after, request.Rules.ClearanceMM
}

// finalizeEmittedRoutePhysicalClearanceWhenRequired reserves writer-level DRC
// geometry enforcement for workflows that explicitly request authoritative
// KiCad DRC. Structural/offline workflows may use conservative template pad
// geometry and do not claim this physical acceptance level.
func finalizeEmittedRoutePhysicalClearanceWhenRequired(requireDRC bool, request routing.Request, operations []transactions.Operation) ([]transactions.Operation, []reports.Issue, int, int, float64) {
	if !requireDRC {
		return operations, nil, 0, 0, request.Rules.ClearanceMM
	}
	return finalizeEmittedRoutePhysicalClearance(request, operations)
}

// deferPhysicalClearanceIssuesToRequiredDRC keeps offline workflows fail-closed
// while allowing the authoritative installed-KiCad DRC to decide conservative
// internal geometry findings. Strict-DRC workflows retain the delegated count
// in stage evidence instead of reporting findings that KiCad may disprove.
func deferPhysicalClearanceIssuesToRequiredDRC(requireDRC bool, issues []reports.Issue) ([]reports.Issue, int) {
	deferred := cloneIssues(issues)
	if !requireDRC {
		return deferred, 0
	}
	count := 0
	retained := deferred[:0]
	for _, issue := range deferred {
		if issue.Blocking() {
			count++
			continue
		}
		retained = append(retained, issue)
	}
	return retained, count
}

func routingRoutesFromOperations(operations []transactions.Operation) []routing.Route {
	return routingRoutesFromDecodedOperations(decodeRouteOperations(operations))
}

func routingRoutesFromDecodedOperations(operations []decodedRouteOperation) []routing.Route {
	routesByNet := map[string]*routing.Route{}
	order := make([]string, 0)
	for _, operation := range operations {
		if !operation.decoded {
			continue
		}
		payload := operation.payload
		netName := strings.TrimSpace(payload.NetName)
		if netName == "" {
			netName = strings.TrimSpace(operation.operation.Net)
		}
		if netName == "" {
			continue
		}
		route := routesByNet[netName]
		if route == nil {
			route = &routing.Route{Net: netName, Status: routing.RouteStatusRouted}
			routesByNet[netName] = route
			order = append(order, netName)
		}
		for index := 1; index < len(payload.Points); index++ {
			route.Segments = append(route.Segments, routing.Segment{
				Net: netName, Layer: payload.Layer,
				Start:   routing.Point{XMM: payload.Points[index-1].XMM, YMM: payload.Points[index-1].YMM},
				End:     routing.Point{XMM: payload.Points[index].XMM, YMM: payload.Points[index].YMM},
				WidthMM: payload.WidthMM,
			})
		}
		for _, via := range payload.Vias {
			layers := append([]string(nil), via.Layers...)
			if len(layers) >= 2 {
				// RouteViaSpec has no blind/buried via type, and the PCB writer
				// deliberately emits every ordinary route via as a plated F.Cu-to-
				// B.Cu through via regardless of logical transition endpoints. Model
				// that exact writer contract during candidate validation.
				layers = []string{"F.Cu", "B.Cu"}
			}
			route.Vias = append(route.Vias, routing.Via{
				Net: netName, At: routing.Point{XMM: via.At.XMM, YMM: via.At.YMM},
				DiameterMM: via.DiameterMM, DrillMM: via.DrillMM, Layers: layers,
			})
		}
	}
	routes := make([]routing.Route, 0, len(order))
	for _, netName := range order {
		routes = append(routes, *routesByNet[netName])
	}
	return routes
}

func routingRoutesWithPayload(operations []decodedRouteOperation, operationIndex int, payload transactions.RouteOperation) []routing.Route {
	candidate := append([]decodedRouteOperation(nil), operations...)
	if operationIndex >= 0 && operationIndex < len(candidate) {
		candidate[operationIndex].payload = payload
		candidate[operationIndex].decoded = true
	}
	return routingRoutesFromDecodedOperations(candidate)
}

func replaceRouteOperationPayload(operations []transactions.Operation, operationIndex int, payload transactions.RouteOperation) ([]transactions.Operation, bool) {
	if operationIndex < 0 || operationIndex >= len(operations) {
		return nil, false
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, false
	}
	candidate := append([]transactions.Operation(nil), operations...)
	candidate[operationIndex].Raw = raw
	return candidate, true
}

type emittedRouteTransitionVia struct {
	operationIndex int
	viaIndex       int
	netName        string
	via            transactions.RouteViaSpec
}

// repairRouteTransitionViaClearance moves only explicit, free-space layer
// transitions whose via is proven to participate in a physical-clearance
// violation. Every same-net route vertex at the transition moves with the via,
// preserving the copper graph on both layers. Candidate order is grid-based
// and deterministic, and a candidate is accepted only when it reduces physical
// blockers without introducing any other routing validation blocker.
func repairRouteTransitionViaClearance(request routing.Request, operations []transactions.Operation) ([]transactions.Operation, []reports.Issue) {
	repaired := append([]transactions.Operation(nil), operations...)
	maxRepairs := len(emittedRouteTransitionVias(repaired))
	for attempt := 0; attempt < maxRepairs; attempt++ {
		routes := routingRoutesFromOperations(repaired)
		baselinePhysical := blockingIssueCount(routing.ValidatePhysicalClearance(request, routes))
		if baselinePhysical == 0 {
			return repaired, nil
		}
		baselineValidation := blockingIssueCount(routing.ValidateResult(request, routing.Result{Status: routing.StatusRouted, Routes: routes}).Issues)
		progress := false
		for _, transition := range emittedRouteTransitionVias(repaired) {
			withoutVia := routingRoutesWithoutVia(routes, transition.netName, transition.via)
			if blockingIssueCount(routing.ValidatePhysicalClearance(request, withoutVia)) >= baselinePhysical {
				continue
			}
			for _, candidatePoint := range routeTransitionViaCandidates(request, transition.via) {
				candidate, ok := moveRouteTransitionVia(repaired, transition, candidatePoint)
				if !ok {
					continue
				}
				candidateRoutes := routingRoutesFromOperations(candidate)
				candidatePhysical := blockingIssueCount(routing.ValidatePhysicalClearance(request, candidateRoutes))
				candidateWithoutVia := routingRoutesWithoutVia(candidateRoutes, transition.netName, transactions.RouteViaSpec{At: candidatePoint})
				if candidatePhysical >= baselinePhysical ||
					blockingIssueCount(routing.ValidatePhysicalClearance(request, candidateWithoutVia)) < candidatePhysical ||
					blockingIssueCount(routing.ValidateResult(request, routing.Result{Status: routing.StatusRouted, Routes: candidateRoutes}).Issues) > baselineValidation {
					continue
				}
				repaired = candidate
				progress = true
				break
			}
			if progress {
				break
			}
			return repaired, []reports.Issue{{
				Code:       reports.CodeValidationFailed,
				Severity:   reports.SeverityBlocked,
				Path:       fmt.Sprintf("operations[%d].vias[%d]", transition.operationIndex, transition.viaIndex),
				Message:    fmt.Sprintf("layer-transition via at (%.6g,%.6g) on net %s has no clearance-safe grid relocation", transition.via.At.XMM, transition.via.At.YMM, transition.netName),
				Nets:       []string{transition.netName},
				Suggestion: "reroute the layer transition with additional free-space clearance",
			}}
		}
		if !progress {
			return repaired, nil
		}
	}
	return repaired, nil
}

// repairEmittedRoutePhysicalClearance validates the complete cross-phase
// copper set, including foreign pads, after all route trees and local routes
// have been combined. It preserves every route endpoint and tries bounded,
// grid-ordered parallel doglegs only when they strictly reduce the number of
// physical blockers. This closes the gap between independently valid routing
// phases without recognizing a fixture, net, component, or coordinate.
func repairEmittedRoutePhysicalClearance(request routing.Request, operations []transactions.Operation) ([]transactions.Operation, []reports.Issue) {
	repaired := append([]transactions.Operation(nil), operations...)
	maximumAttempts := max(1, len(emittedRouteSegments(repaired))*2)
	gridMM := request.Rules.GridMM
	if gridMM <= 0 || math.IsNaN(gridMM) || math.IsInf(gridMM, 0) {
		gridMM = routing.DefaultRules().GridMM
	}
	for attempt := 0; attempt < maximumAttempts; attempt++ {
		baselineRoutes := routingRoutesFromOperations(repaired)
		baselineIssues := routing.ValidatePhysicalTrackClearance(request, baselineRoutes)
		baselineBlockers := blockingIssueCount(baselineIssues)
		if baselineBlockers == 0 {
			return compactRouteOperationGeometry(repaired), nil
		}
		baselineValidation := blockingIssueCount(routing.ValidateResult(request, routing.Result{Status: routing.StatusRouted, Routes: baselineRoutes}).Issues)
		progress := false
		decoded := decodeRouteOperations(repaired)
		for _, segment := range emittedRouteSegments(repaired) {
			if segment.operationIndex < 0 || segment.operationIndex >= len(repaired) || repaired[segment.operationIndex].Ref == localRouteEndpointDogboneOperationRef {
				continue
			}
			dx := segment.end.XMM - segment.start.XMM
			dy := segment.end.YMM - segment.start.YMM
			length := math.Hypot(dx, dy)
			if length <= 1e-9 {
				continue
			}
			if segment.operationIndex >= len(decoded) || !decoded[segment.operationIndex].decoded {
				continue
			}
			payload := decoded[segment.operationIndex].payload
			if segment.pointIndex < 0 || segment.pointIndex+1 >= len(payload.Points) {
				continue
			}
			targetSegment := routing.Segment{
				Net: payload.NetName, Layer: payload.Layer,
				Start: routing.Point{XMM: segment.start.XMM, YMM: segment.start.YMM},
				End:   routing.Point{XMM: segment.end.XMM, YMM: segment.end.YMM}, WidthMM: payload.WidthMM,
			}
			if !reports.HasBlockingIssue(routing.ValidatePhysicalTrackClearanceForSegment(request, baselineRoutes, targetSegment)) {
				continue
			}
			baselineNetBlockers := blockingIssueCount(routing.ValidatePhysicalTrackClearanceForNet(request, baselineRoutes, payload.NetName))
			if baselineNetBlockers == 0 {
				continue
			}
			if segment.pointIndex > 0 && segment.pointIndex+2 < len(payload.Points) {
				for ring := 1; ring <= 16 && !progress; ring++ {
					offsetMM := float64(ring) * gridMM
					for _, direction := range []float64{1, -1} {
						offsetX := direction * -dy / length * offsetMM
						offsetY := direction * dx / length * offsetMM
						candidatePayload := payload
						candidatePayload.Points = append([]transactions.Point(nil), payload.Points...)
						candidatePayload.Points[segment.pointIndex].XMM += offsetX
						candidatePayload.Points[segment.pointIndex].YMM += offsetY
						candidatePayload.Points[segment.pointIndex+1].XMM += offsetX
						candidatePayload.Points[segment.pointIndex+1].YMM += offsetY
						if !routePointsInsideBoard(request.Board, candidatePayload.Points) {
							continue
						}
						candidateRoutes := routingRoutesWithPayload(decoded, segment.operationIndex, candidatePayload)
						candidateNetBlockers := blockingIssueCount(routing.ValidatePhysicalTrackClearanceForNet(request, candidateRoutes, payload.NetName))
						if candidateNetBlockers >= baselineNetBlockers ||
							blockingIssueCount(routing.ValidateResult(request, routing.Result{Status: routing.StatusRouted, Routes: candidateRoutes}).Issues) > baselineValidation {
							continue
						}
						candidate, ok := replaceRouteOperationPayload(repaired, segment.operationIndex, candidatePayload)
						if !ok {
							continue
						}
						repaired = candidate
						progress = true
						break
					}
				}
			}
			if progress {
				break
			}
			padDetours := routing.PhysicalPadDetourCandidates(request, targetSegment, 16)
			for _, detour := range padDetours {
				candidatePoints := make([]transactions.Point, len(detour))
				for index, point := range detour {
					candidatePoints[index] = transactions.Point{XMM: point.XMM, YMM: point.YMM}
				}
				if !routePointsInsideBoard(request.Board, candidatePoints) {
					continue
				}
				candidatePayload := payload
				candidatePayload.Points = insertRoutePoints(payload.Points, segment.pointIndex+1, candidatePoints)
				candidateRoutes := routingRoutesWithPayload(decoded, segment.operationIndex, candidatePayload)
				candidateNetBlockers := blockingIssueCount(routing.ValidatePhysicalTrackClearanceForNet(request, candidateRoutes, payload.NetName))
				if candidateNetBlockers >= baselineNetBlockers ||
					blockingIssueCount(routing.ValidateResult(request, routing.Result{Status: routing.StatusRouted, Routes: candidateRoutes}).Issues) > baselineValidation {
					continue
				}
				candidate, ok := replaceRouteOperationPayload(repaired, segment.operationIndex, candidatePayload)
				if !ok {
					continue
				}
				repaired = candidate
				progress = true
				break
			}
			if progress {
				break
			}
			for ring := 1; ring <= 16 && !progress; ring++ {
				offsetMM := float64(ring) * gridMM
				for _, direction := range []float64{1, -1} {
					offsetX := direction * -dy / length * offsetMM
					offsetY := direction * dx / length * offsetMM
					dogleg := []transactions.Point{
						{XMM: segment.start.XMM + offsetX, YMM: segment.start.YMM + offsetY},
						{XMM: segment.end.XMM + offsetX, YMM: segment.end.YMM + offsetY},
					}
					if !routePointsInsideBoard(request.Board, dogleg) {
						continue
					}
					candidatePayload := payload
					candidatePayload.Points = insertRoutePoints(payload.Points, segment.pointIndex+1, dogleg)
					candidateRoutes := routingRoutesWithPayload(decoded, segment.operationIndex, candidatePayload)
					candidateNetBlockers := blockingIssueCount(routing.ValidatePhysicalTrackClearanceForNet(request, candidateRoutes, payload.NetName))
					if candidateNetBlockers >= baselineNetBlockers ||
						blockingIssueCount(routing.ValidateResult(request, routing.Result{Status: routing.StatusRouted, Routes: candidateRoutes}).Issues) > baselineValidation {
						continue
					}
					candidate, ok := replaceRouteOperationPayload(repaired, segment.operationIndex, candidatePayload)
					if !ok {
						continue
					}
					repaired = candidate
					progress = true
					break
				}
			}
			if !progress && len(padDetours) != 0 {
				baselineAllPhysical := blockingIssueCount(routing.ValidatePhysicalClearance(request, baselineRoutes))
				for _, detour := range padDetours {
					for spanRing := 0; spanRing <= 8 && !progress; spanRing++ {
						expandedDetour := expandPadDetourTransitionSpan(detour, routing.Point{XMM: segment.start.XMM, YMM: segment.start.YMM}, routing.Point{XMM: segment.end.XMM, YMM: segment.end.YMM}, float64(spanRing)*gridMM)
						for _, alternateLayer := range alternateRoutableLayers(request.Board, payload.Layer) {
							candidate, ok := buildAlternateLayerPadDetour(repaired, segment.operationIndex, segment.pointIndex, payload, expandedDetour, alternateLayer, request.Rules)
							if !ok {
								continue
							}
							candidateRoutes := routingRoutesFromOperations(candidate)
							candidateNetBlockers := blockingIssueCount(routing.ValidatePhysicalTrackClearanceForNet(request, candidateRoutes, payload.NetName))
							if candidateNetBlockers >= baselineNetBlockers ||
								blockingIssueCount(routing.ValidateResult(request, routing.Result{Status: routing.StatusRouted, Routes: candidateRoutes}).Issues) > baselineValidation ||
								blockingIssueCount(routing.ValidatePhysicalClearance(request, candidateRoutes)) > baselineAllPhysical {
								continue
							}
							repaired = candidate
							progress = true
							break
						}
					}
					if progress {
						break
					}
				}
			}
			if progress {
				break
			}
		}
		if !progress {
			return compactRouteOperationGeometry(repaired), baselineIssues
		}
	}
	repaired = compactRouteOperationGeometry(repaired)
	return repaired, routing.ValidatePhysicalTrackClearance(request, routingRoutesFromOperations(repaired))
}

func alternateRoutableLayers(board routing.Board, current string) []string {
	var layers []string
	for _, layer := range board.Layers {
		if !layer.Routable || layer.Kind != routing.LayerCopper || canonicalCopperLayer(layer.Name) == canonicalCopperLayer(current) {
			continue
		}
		layers = append(layers, layer.Name)
	}
	return layers
}

func expandPadDetourTransitionSpan(detour []routing.Point, segmentStart routing.Point, segmentEnd routing.Point, extensionMM float64) []routing.Point {
	expanded := append([]routing.Point(nil), detour...)
	if len(expanded) < 4 || extensionMM <= 0 {
		return expanded
	}
	dx := segmentEnd.XMM - segmentStart.XMM
	dy := segmentEnd.YMM - segmentStart.YMM
	length := math.Hypot(dx, dy)
	if length <= 1e-9 {
		return expanded
	}
	offsetX, offsetY := dx/length*extensionMM, dy/length*extensionMM
	expanded[0].XMM -= offsetX
	expanded[0].YMM -= offsetY
	expanded[1].XMM -= offsetX
	expanded[1].YMM -= offsetY
	last := len(expanded) - 1
	expanded[last-1].XMM += offsetX
	expanded[last-1].YMM += offsetY
	expanded[last].XMM += offsetX
	expanded[last].YMM += offsetY
	return expanded
}

func buildAlternateLayerPadDetour(operations []transactions.Operation, operationIndex int, pointIndex int, payload transactions.RouteOperation, detour []routing.Point, alternateLayer string, rules routing.Rules) ([]transactions.Operation, bool) {
	if operationIndex < 0 || operationIndex >= len(operations) || pointIndex < 0 || pointIndex+1 >= len(payload.Points) || len(detour) < 2 || strings.TrimSpace(alternateLayer) == "" {
		return nil, false
	}
	toTransactionPoint := func(point routing.Point) transactions.Point {
		return transactions.Point{XMM: point.XMM, YMM: point.YMM}
	}
	entry := toTransactionPoint(detour[0])
	exit := toTransactionPoint(detour[len(detour)-1])
	prefix := payload
	prefix.Points = append([]transactions.Point(nil), payload.Points[:pointIndex+1]...)
	prefix.Points = appendDistinctTransactionPoint(prefix.Points, entry)
	prefix.Vias = nil
	middle := transactions.RouteOperation{Op: transactions.OpRoute, NetName: payload.NetName, Layer: alternateLayer, WidthMM: payload.WidthMM}
	for _, point := range detour {
		middle.Points = appendDistinctTransactionPoint(middle.Points, toTransactionPoint(point))
	}
	suffix := payload
	suffix.Points = []transactions.Point{exit}
	for _, point := range payload.Points[pointIndex+1:] {
		suffix.Points = appendDistinctTransactionPoint(suffix.Points, point)
	}
	suffix.Vias = nil
	for _, via := range payload.Vias {
		switch {
		case routeViaOnTransactionPolyline(via, prefix.Points):
			prefix.Vias = append(prefix.Vias, via)
		case routeViaOnTransactionPolyline(via, suffix.Points):
			suffix.Vias = append(suffix.Vias, via)
		case routeViaOnTransactionPolyline(via, middle.Points):
			middle.Vias = append(middle.Vias, via)
		default:
			// Never silently orphan an existing physical layer transition when
			// replacing the segment that carried it.
			return nil, false
		}
	}
	defaults := routing.DefaultRules()
	viaDiameterMM := rules.ViaDiameterMM
	if viaDiameterMM <= 0 {
		viaDiameterMM = defaults.ViaDiameterMM
	}
	viaDrillMM := rules.ViaDrillMM
	if viaDrillMM <= 0 {
		viaDrillMM = defaults.ViaDrillMM
	}
	transitionVias := []transactions.RouteViaSpec{
		{At: entry, DiameterMM: viaDiameterMM, DrillMM: viaDrillMM, Layers: []string{payload.Layer, alternateLayer}},
		{At: exit, DiameterMM: viaDiameterMM, DrillMM: viaDrillMM, Layers: []string{payload.Layer, alternateLayer}},
	}
	for _, via := range transitionVias {
		if !routeViaAtTransactionPoint(prefix.Vias, via.At) && !routeViaAtTransactionPoint(middle.Vias, via.At) && !routeViaAtTransactionPoint(suffix.Vias, via.At) {
			middle.Vias = append(middle.Vias, via)
		}
	}
	if len(prefix.Points) < 2 || len(middle.Points) < 2 || len(suffix.Points) < 2 {
		return nil, false
	}
	encode := func(template transactions.Operation, route transactions.RouteOperation) (transactions.Operation, bool) {
		raw, err := json.Marshal(route)
		if err != nil {
			return transactions.Operation{}, false
		}
		encoded := template.Clone()
		encoded.Raw = raw
		encoded.Net = route.NetName
		return encoded, true
	}
	template := operations[operationIndex]
	prefixOperation, ok := encode(template, prefix)
	if !ok {
		return nil, false
	}
	middleOperation, ok := encode(template, middle)
	if !ok {
		return nil, false
	}
	suffixOperation, ok := encode(template, suffix)
	if !ok {
		return nil, false
	}
	candidate := make([]transactions.Operation, 0, len(operations)+2)
	candidate = append(candidate, operations[:operationIndex]...)
	candidate = append(candidate, prefixOperation, middleOperation, suffixOperation)
	candidate = append(candidate, operations[operationIndex+1:]...)
	return candidate, true
}

func routeViaOnTransactionPolyline(via transactions.RouteViaSpec, points []transactions.Point) bool {
	for index := 1; index < len(points); index++ {
		if pointToSegmentDistanceMM(via.At, points[index-1], points[index]) <= 1e-6 {
			return true
		}
	}
	return false
}

func routeViaAtTransactionPoint(vias []transactions.RouteViaSpec, point transactions.Point) bool {
	return slices.ContainsFunc(vias, func(via transactions.RouteViaSpec) bool {
		return sameRoutePoint(via.At, point)
	})
}

func appendDistinctTransactionPoint(points []transactions.Point, point transactions.Point) []transactions.Point {
	if len(points) != 0 && sameRoutePoint(points[len(points)-1], point) {
		return points
	}
	return append(points, point)
}

func routePointsInsideBoard(board routing.Board, points []transactions.Point) bool {
	for _, point := range points {
		if point.XMM < 0 || point.YMM < 0 || point.XMM > board.WidthMM || point.YMM > board.HeightMM ||
			math.IsNaN(point.XMM) || math.IsNaN(point.YMM) || math.IsInf(point.XMM, 0) || math.IsInf(point.YMM, 0) {
			return false
		}
	}
	return true
}

func emittedRouteTransitionVias(operations []transactions.Operation) []emittedRouteTransitionVia {
	decoded := decodeRouteOperations(operations)
	var transitions []emittedRouteTransitionVia
	for operationIndex, route := range decoded {
		if !route.decoded {
			continue
		}
		for viaIndex, via := range route.payload.Vias {
			layers := map[string]struct{}{}
			for _, candidate := range decoded {
				if !candidate.decoded || routeNetKey(candidate.payload.NetName) != routeNetKey(route.payload.NetName) || strings.TrimSpace(candidate.payload.Layer) == "" || len(candidate.payload.Points) == 0 {
					continue
				}
				first := candidate.payload.Points[0]
				last := candidate.payload.Points[len(candidate.payload.Points)-1]
				if sameRoutePoint(first, via.At) || sameRoutePoint(last, via.At) {
					layers[canonicalCopperLayer(candidate.payload.Layer)] = struct{}{}
				}
			}
			if len(layers) < 2 {
				continue
			}
			transitions = append(transitions, emittedRouteTransitionVia{
				operationIndex: operationIndex,
				viaIndex:       viaIndex,
				netName:        route.payload.NetName,
				via:            via,
			})
		}
	}
	return transitions
}

func routingRoutesWithoutVia(routes []routing.Route, netName string, via transactions.RouteViaSpec) []routing.Route {
	without := make([]routing.Route, len(routes))
	removed := false
	for routeIndex, route := range routes {
		without[routeIndex] = route
		without[routeIndex].Segments = append([]routing.Segment(nil), route.Segments...)
		without[routeIndex].Vias = make([]routing.Via, 0, len(route.Vias))
		for _, candidate := range route.Vias {
			if !removed && routeNetKey(route.Net) == routeNetKey(netName) &&
				math.Abs(candidate.At.XMM-via.At.XMM) <= 1e-6 && math.Abs(candidate.At.YMM-via.At.YMM) <= 1e-6 {
				removed = true
				continue
			}
			without[routeIndex].Vias = append(without[routeIndex].Vias, candidate)
		}
	}
	return without
}

func routeTransitionViaCandidates(request routing.Request, via transactions.RouteViaSpec) []transactions.Point {
	gridMM := request.Rules.GridMM
	if gridMM <= 0 || math.IsNaN(gridMM) || math.IsInf(gridMM, 0) {
		gridMM = routing.DefaultRules().GridMM
	}
	searchDistanceMM := max(4*gridMM, 2*(via.DiameterMM+max(request.Rules.ClearanceMM, request.Rules.ViaClearanceMM)))
	maxRing := max(1, int(math.Ceil(searchDistanceMM/gridMM)))
	var candidates []transactions.Point
	for ring := 1; ring <= maxRing; ring++ {
		type offset struct{ x, y int }
		var offsets []offset
		for x := -ring; x <= ring; x++ {
			for y := -ring; y <= ring; y++ {
				if max(absInt(x), absInt(y)) == ring {
					offsets = append(offsets, offset{x: x, y: y})
				}
			}
		}
		sort.SliceStable(offsets, func(i, j int) bool {
			leftDistance := offsets[i].x*offsets[i].x + offsets[i].y*offsets[i].y
			rightDistance := offsets[j].x*offsets[j].x + offsets[j].y*offsets[j].y
			if leftDistance != rightDistance {
				return leftDistance < rightDistance
			}
			if offsets[i].x != offsets[j].x {
				return offsets[i].x > offsets[j].x
			}
			return offsets[i].y > offsets[j].y
		})
		for _, candidate := range offsets {
			candidates = append(candidates, transactions.Point{
				XMM: via.At.XMM + float64(candidate.x)*gridMM,
				YMM: via.At.YMM + float64(candidate.y)*gridMM,
			})
		}
	}
	return candidates
}

func moveRouteTransitionVia(operations []transactions.Operation, transition emittedRouteTransitionVia, point transactions.Point) ([]transactions.Operation, bool) {
	candidate := append([]transactions.Operation(nil), operations...)
	changedVia := false
	changedLayerEndpoints := map[string]struct{}{}
	for operationIndex, route := range decodeRouteOperations(candidate) {
		if !route.decoded || routeNetKey(route.payload.NetName) != routeNetKey(transition.netName) {
			continue
		}
		payload := route.payload
		changed := false
		for viaIndex := range payload.Vias {
			if sameRoutePoint(payload.Vias[viaIndex].At, transition.via.At) {
				payload.Vias[viaIndex].At = point
				changed = true
				changedVia = true
			}
		}
		for pointIndex := range payload.Points {
			if !sameRoutePoint(payload.Points[pointIndex], transition.via.At) {
				continue
			}
			payload.Points[pointIndex] = point
			changed = true
			if strings.TrimSpace(payload.Layer) != "" {
				changedLayerEndpoints[canonicalCopperLayer(payload.Layer)] = struct{}{}
			}
		}
		if !changed {
			continue
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return operations, false
		}
		candidate[operationIndex].Raw = raw
	}
	return candidate, changedVia && len(changedLayerEndpoints) >= 2
}

// KiCad's copper-sliver DRC treats same-net junctions below 20 degrees as
// acute. Keep the transaction repair aligned with that boundary so ordinary
// shallow branches are not rejected before KiCad evaluates them.
const minimumEmittedRouteJunctionAngleDegrees = 20.0

type emittedRouteSegment struct {
	operationIndex int
	pointIndex     int
	netName        string
	layer          string
	widthMM        float64
	start          transactions.Point
	end            transactions.Point
}

type acuteEmittedRouteJunction struct {
	fixed       emittedRouteSegment
	repair      emittedRouteSegment
	shared      transactions.Point
	fixedRemote transactions.Point
}

func repairAcuteRouteOperationJunctions(request routing.Request, operations []transactions.Operation) ([]transactions.Operation, []reports.Issue) {
	repaired := append([]transactions.Operation(nil), operations...)
	ignoredFilledJunctions := map[string]struct{}{}
	maxRepairs := len(emittedRouteSegments(repaired)) * 2
	for attempt := 0; attempt < maxRepairs; attempt++ {
		junction, ok := firstAcuteRouteOperationJunctionIgnoring(repaired, ignoredFilledJunctions)
		if !ok {
			return repaired, nil
		}
		decoded := decodeRouteOperations(repaired)
		if junction.repair.operationIndex < 0 || junction.repair.operationIndex >= len(decoded) || !decoded[junction.repair.operationIndex].decoded {
			break
		}
		payload := decoded[junction.repair.operationIndex].payload
		if junction.repair.pointIndex < 0 || junction.repair.pointIndex+1 >= len(payload.Points) {
			break
		}
		remote := junction.repair.end
		if !sameRoutePoint(junction.repair.start, junction.shared) {
			remote = junction.repair.start
		}
		corners := []transactions.Point{
			{XMM: junction.shared.XMM, YMM: remote.YMM},
			{XMM: remote.XMM, YMM: junction.shared.YMM},
		}
		if emittedRouteJunctionAngleDegrees(junction.shared, junction.fixedRemote, corners[1]) > emittedRouteJunctionAngleDegrees(junction.shared, junction.fixedRemote, corners[0]) {
			corners[0], corners[1] = corners[1], corners[0]
		}
		baselineRoutes := routingRoutesFromOperations(repaired)
		baselineValidationBlockers := blockingIssueCount(routing.ValidateResult(request, routing.Result{Status: routing.StatusRouted, Routes: baselineRoutes}).Issues)
		baselineClearanceBlockers := blockingIssueCount(routing.ValidatePhysicalClearance(request, baselineRoutes))
		candidateDoglegs := make([][]transactions.Point, 0, 2+16*2)
		for _, corner := range corners {
			candidateDoglegs = append(candidateDoglegs, []transactions.Point{corner})
		}
		segmentX := remote.XMM - junction.shared.XMM
		segmentY := remote.YMM - junction.shared.YMM
		segmentLength := math.Hypot(segmentX, segmentY)
		if segmentLength > 1e-12 {
			step := request.Rules.GridMM / 4
			if step <= 0 {
				step = math.Max(junction.fixed.widthMM, junction.repair.widthMM) / 8
			}
			step = math.Max(step, 0.025)
			for ring := 1; ring <= 16; ring++ {
				offset := float64(ring) * step
				for _, direction := range []float64{1, -1} {
					dx := direction * -segmentY / segmentLength * offset
					dy := direction * segmentX / segmentLength * offset
					candidateDoglegs = append(candidateDoglegs, []transactions.Point{
						{XMM: junction.shared.XMM + dx, YMM: junction.shared.YMM + dy},
						{XMM: remote.XMM + dx, YMM: remote.YMM + dy},
					})
				}
			}
		}
		accepted := false
		for _, dogleg := range candidateDoglegs {
			if len(dogleg) == 0 || sameRoutePoint(dogleg[0], junction.shared) || sameRoutePoint(dogleg[0], remote) ||
				emittedRouteJunctionAngleDegrees(junction.shared, junction.fixedRemote, dogleg[0]) < minimumEmittedRouteJunctionAngleDegrees {
				continue
			}
			invalid := false
			for _, point := range dogleg {
				if sameRoutePoint(point, junction.shared) || sameRoutePoint(point, remote) {
					invalid = true
					break
				}
			}
			if invalid {
				continue
			}
			candidatePayload := payload
			candidatePayload.Points = insertRoutePoints(payload.Points, junction.repair.pointIndex+1, dogleg)
			raw, err := json.Marshal(candidatePayload)
			if err != nil {
				continue
			}
			candidate := append([]transactions.Operation(nil), repaired...)
			candidate[junction.repair.operationIndex].Raw = raw
			routes := routingRoutesFromOperations(candidate)
			if blockingIssueCount(routing.ValidateResult(request, routing.Result{Status: routing.StatusRouted, Routes: routes}).Issues) > baselineValidationBlockers ||
				blockingIssueCount(routing.ValidatePhysicalClearance(request, routes)) > baselineClearanceBlockers {
				continue
			}
			repaired = candidate
			accepted = true
			break
		}
		if !accepted {
			if acuteRouteJunctionUsesWidenedCopper(request, junction) && acuteRouteJunctionClosesCopperCycle(junction, emittedRouteSegments(repaired)) {
				ignoredFilledJunctions[acuteRouteJunctionKey(junction)] = struct{}{}
				continue
			}
			return repaired, []reports.Issue{{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityBlocked,
				Path:     fmt.Sprintf("operations[%d].points[%d]", junction.repair.operationIndex, junction.repair.pointIndex),
				Message: fmt.Sprintf(
					"acute same-net copper junction at (%.6g,%.6g) between (%.6g,%.6g) and (%.6g,%.6g), widths %.6g/%.6g mm, could not be replaced by a clearance-safe dogleg",
					junction.shared.XMM, junction.shared.YMM, junction.fixedRemote.XMM, junction.fixedRemote.YMM,
					emittedRouteJunctionRemote(junction).XMM, emittedRouteJunctionRemote(junction).YMM, junction.fixed.widthMM, junction.repair.widthMM,
				),
				Nets:       []string{junction.repair.netName},
				Suggestion: "reroute the affected net with a non-acute branch transition",
			}}
		}
	}
	if _, ok := firstAcuteRouteOperationJunction(repaired); ok {
		return repaired, []reports.Issue{{
			Code:       reports.CodeValidationFailed,
			Severity:   reports.SeverityBlocked,
			Message:    "acute same-net copper junction repair budget exhausted",
			Suggestion: "reroute the affected nets with non-acute branch transitions",
		}}
	}
	return repaired, nil
}

func blockingIssueCount(issues []reports.Issue) int {
	count := 0
	for _, issue := range issues {
		if issue.Severity == reports.SeverityBlocked {
			count++
		}
	}
	return count
}

func firstAcuteRouteOperationJunction(operations []transactions.Operation) (acuteEmittedRouteJunction, bool) {
	return firstAcuteRouteOperationJunctionIgnoring(operations, nil)
}

func firstAcuteRouteOperationJunctionIgnoring(operations []transactions.Operation, ignored map[string]struct{}) (acuteEmittedRouteJunction, bool) {
	segments := emittedRouteSegments(operations)
	for rightIndex := 1; rightIndex < len(segments); rightIndex++ {
		for leftIndex := 0; leftIndex < rightIndex; leftIndex++ {
			left := segments[leftIndex]
			right := segments[rightIndex]
			if routeNetKey(left.netName) != routeNetKey(right.netName) || canonicalCopperLayer(left.layer) != canonicalCopperLayer(right.layer) {
				continue
			}
			shared, leftRemote, rightRemote, ok := sharedEmittedRouteEndpoint(left, right)
			if !ok {
				continue
			}
			angle := emittedRouteJunctionAngleDegrees(shared, leftRemote, rightRemote)
			if angle > 1e-9 && angle < minimumEmittedRouteJunctionAngleDegrees-1e-9 {
				junction := acuteEmittedRouteJunction{fixed: left, repair: right, shared: shared, fixedRemote: leftRemote}
				if _, skip := ignored[acuteRouteJunctionKey(junction)]; skip {
					continue
				}
				return junction, true
			}
		}
	}
	return acuteEmittedRouteJunction{}, false
}

func acuteRouteJunctionKey(junction acuteEmittedRouteJunction) string {
	return fmt.Sprintf("%d:%d:%d:%d", junction.fixed.operationIndex, junction.fixed.pointIndex, junction.repair.operationIndex, junction.repair.pointIndex)
}

func emittedRouteJunctionRemote(junction acuteEmittedRouteJunction) transactions.Point {
	if sameRoutePoint(junction.repair.start, junction.shared) {
		return junction.repair.end
	}
	return junction.repair.start
}

func acuteRouteJunctionUsesWidenedCopper(request routing.Request, junction acuteEmittedRouteJunction) bool {
	nominalWidthMM := request.Rules.TraceWidthMM
	if nominalWidthMM <= 0 {
		return false
	}
	return math.Max(junction.fixed.widthMM, junction.repair.widthMM) >= 1.2*nominalWidthMM-1e-9
}

func acuteRouteJunctionClosesCopperCycle(junction acuteEmittedRouteJunction, segments []emittedRouteSegment) bool {
	type neighbor struct {
		point  routeCoordKey
		length float64
	}
	repairRemote := emittedRouteJunctionRemote(junction)
	adjacent := map[routeCoordKey][]neighbor{}
	for _, segment := range segments {
		if routeNetKey(segment.netName) != routeNetKey(junction.repair.netName) || canonicalCopperLayer(segment.layer) != canonicalCopperLayer(junction.repair.layer) {
			continue
		}
		if (segment.operationIndex == junction.fixed.operationIndex && segment.pointIndex == junction.fixed.pointIndex) ||
			(segment.operationIndex == junction.repair.operationIndex && segment.pointIndex == junction.repair.pointIndex) {
			continue
		}
		startKey := routePointKey(segment.start)
		endKey := routePointKey(segment.end)
		length := math.Hypot(segment.end.XMM-segment.start.XMM, segment.end.YMM-segment.start.YMM)
		adjacent[startKey] = append(adjacent[startKey], neighbor{point: endKey, length: length})
		adjacent[endKey] = append(adjacent[endKey], neighbor{point: startKey, length: length})
	}
	target := routePointKey(repairRemote)
	queue := []routeCoordKey{routePointKey(junction.fixedRemote)}
	distance := map[routeCoordKey]float64{queue[0]: 0}
	maximumLocalCycleLengthMM := 2 * math.Max(junction.fixed.widthMM, junction.repair.widthMM)
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		if current == target {
			return true
		}
		for _, next := range adjacent[current] {
			candidateDistance := distance[current] + next.length
			if candidateDistance > maximumLocalCycleLengthMM {
				continue
			}
			prior, seen := distance[next.point]
			if !seen || candidateDistance < prior {
				distance[next.point] = candidateDistance
				queue = append(queue, next.point)
			}
		}
	}
	return false
}

func emittedRouteSegments(operations []transactions.Operation) []emittedRouteSegment {
	segments := make([]emittedRouteSegment, 0)
	for operationIndex, route := range decodeRouteOperations(operations) {
		if !route.decoded {
			continue
		}
		for pointIndex := 0; pointIndex+1 < len(route.payload.Points); pointIndex++ {
			segments = append(segments, emittedRouteSegment{
				operationIndex: operationIndex,
				pointIndex:     pointIndex,
				netName:        route.payload.NetName,
				layer:          route.payload.Layer,
				widthMM:        route.payload.WidthMM,
				start:          route.payload.Points[pointIndex],
				end:            route.payload.Points[pointIndex+1],
			})
		}
	}
	return segments
}

func sharedEmittedRouteEndpoint(left, right emittedRouteSegment) (transactions.Point, transactions.Point, transactions.Point, bool) {
	for _, candidate := range []struct {
		shared, leftRemote, rightPoint, rightRemote transactions.Point
	}{
		{left.start, left.end, right.start, right.end},
		{left.start, left.end, right.end, right.start},
		{left.end, left.start, right.start, right.end},
		{left.end, left.start, right.end, right.start},
	} {
		if sameRoutePoint(candidate.shared, candidate.rightPoint) {
			return candidate.shared, candidate.leftRemote, candidate.rightRemote, true
		}
	}
	return transactions.Point{}, transactions.Point{}, transactions.Point{}, false
}

func emittedRouteJunctionAngleDegrees(origin, left, right transactions.Point) float64 {
	leftX := left.XMM - origin.XMM
	leftY := left.YMM - origin.YMM
	rightX := right.XMM - origin.XMM
	rightY := right.YMM - origin.YMM
	denominator := math.Hypot(leftX, leftY) * math.Hypot(rightX, rightY)
	if denominator <= 1e-12 {
		return 0
	}
	cosine := (leftX*rightX + leftY*rightY) / denominator
	cosine = math.Max(-1, math.Min(1, cosine))
	return math.Acos(cosine) * 180 / math.Pi
}

func insertRoutePoint(points []transactions.Point, index int, point transactions.Point) []transactions.Point {
	return insertRoutePoints(points, index, []transactions.Point{point})
}

func insertRoutePoints(points []transactions.Point, index int, insertedPoints []transactions.Point) []transactions.Point {
	inserted := make([]transactions.Point, 0, len(points)+len(insertedPoints))
	inserted = append(inserted, points[:index]...)
	inserted = append(inserted, insertedPoints...)
	inserted = append(inserted, points[index:]...)
	return inserted
}

func canceledRoutingStageResult(ctx context.Context) (RoutingStageResult, bool) {
	if err := ctx.Err(); err != nil {
		return RoutingStageResult{Stage: NewStageResult(StageRouting, []reports.Issue{{
			Code:     reports.CodeOperationCanceled,
			Severity: reports.SeverityBlocked,
			Message:  err.Error(),
		}})}, true
	}
	return RoutingStageResult{}, false
}

func postProcessRouteOperations(operations []transactions.Operation, placed *PlacementStageResult, physical physicalPadRoutingContext, physicalEvidence InterBlockContactEvidence) ([]transactions.Operation, []reports.Issue) {
	operations, _ = dedupeSameNetRouteViasWithEvidence(operations)
	operations, viaReducedRoutes, issues := removeRedundantRouteViasAtPlatedPadsWithContext(operations, placed, physical)
	if viaReducedRoutes == nil {
		viaReducedRoutes = map[int]struct{}{}
	}
	for operationIndex, operation := range operations {
		if operation.PruneProtected || operation.Op == transactions.OpRoute && operation.Ref == localRouteEndpointDogboneOperationRef {
			viaReducedRoutes[operationIndex] = struct{}{}
		}
	}
	decoded := decodeRouteOperations(operations)
	viaReducedRoutes = expandViaReducedRoutePairsDecoded(decoded, viaReducedRoutes)
	operations = pruneRedundantDanglingRouteStubsDecoded(operations, placed, viaReducedRoutes, physical, physicalEvidence, decoded)
	return compactRouteOperationGeometry(operations), issues
}

func combineSequentialRoutingResults(first routing.Result, second routing.Result) routing.Result {
	routes := combineSequentialRoutes(first.Routes, second.Routes)
	combined := routing.Result{
		Routes:     routes,
		Operations: slices.Concat(first.Operations, second.Operations),
		Issues:     slices.Concat(first.Issues, second.Issues),
		Metrics: routing.Metrics{
			SearchNodes:       first.Metrics.SearchNodes + second.Metrics.SearchNodes,
			MaxSearchNodesHit: first.Metrics.MaxSearchNodesHit || second.Metrics.MaxSearchNodesHit,
		},
	}
	for _, route := range routes {
		combined.Metrics.NetCount++
		switch route.Status {
		case routing.RouteStatusRouted:
			combined.Metrics.RoutedNetCount++
		case routing.RouteStatusFailed:
			combined.Metrics.FailedNetCount++
		}
		combined.Metrics.SegmentCount += len(route.Segments)
		combined.Metrics.ViaCount += len(route.Vias)
		combined.Metrics.MaxSearchNodesHit = combined.Metrics.MaxSearchNodesHit || route.SearchLimitHit
		for _, segment := range route.Segments {
			combined.Metrics.TotalLengthMM += math.Hypot(segment.End.XMM-segment.Start.XMM, segment.End.YMM-segment.Start.YMM)
		}
	}
	combined.Metrics.TotalLengthMM = math.Round(combined.Metrics.TotalLengthMM*1e6) / 1e6
	switch {
	case combined.Metrics.FailedNetCount == 0 && combined.Metrics.NetCount > 0:
		combined.Status = routing.StatusRouted
	case combined.Metrics.RoutedNetCount > 0:
		combined.Status = routing.StatusPartial
	case combined.Metrics.NetCount > 0:
		combined.Status = routing.StatusBlocked
	case first.Status == routing.StatusBlocked || second.Status == routing.StatusBlocked:
		combined.Status = routing.StatusBlocked
	case first.Status == routing.StatusPartial || second.Status == routing.StatusPartial:
		combined.Status = routing.StatusPartial
	default:
		combined.Status = routing.StatusRouted
	}
	return combined
}

// combineSequentialRoutes treats the second pass as a delta routed around the
// first pass's committed copper. Geometry and search evidence accumulate, but
// a net present in both passes remains one net and its later non-skipped status
// is authoritative. This prevents quality metrics from double-counting a
// partially completed net that required both routing passes.
func combineSequentialRoutes(first, second []routing.Route) []routing.Route {
	combined := make([]routing.Route, len(first))
	copy(combined, first)
	indexByNet := make(map[string]int, len(first)+len(second))
	for index, route := range combined {
		indexByNet[route.Net] = index
	}
	for _, route := range second {
		index, exists := indexByNet[route.Net]
		if !exists {
			indexByNet[route.Net] = len(combined)
			combined = append(combined, route)
			continue
		}
		prior := combined[index]
		prior.Segments = slices.Concat(prior.Segments, route.Segments)
		prior.Vias = slices.Concat(prior.Vias, route.Vias)
		prior.Issues = slices.Concat(prior.Issues, route.Issues)
		if prior.Status == routing.RouteStatusRouted && route.Status == routing.RouteStatusFailed {
			prior.Issues = append(prior.Issues, reports.Issue{
				Code:       reports.CodeValidationTrace,
				Severity:   reports.SeverityInfo,
				Path:       "routing.sequential." + route.Net,
				Message:    "a later routing pass failed after an earlier pass routed part of the net",
				Suggestion: "inspect the per-pass route issues and combined geometry metrics",
				Nets:       []string{route.Net},
			})
		}
		prior.SearchNodes += route.SearchNodes
		prior.SearchLimitHit = prior.SearchLimitHit || route.SearchLimitHit
		prior.Status = combinedSequentialRouteStatus(prior.Status, route.Status)
		combined[index] = prior
	}
	return combined
}

func combinedSequentialRouteStatus(prior, current routing.RouteStatus) routing.RouteStatus {
	switch current {
	case routing.RouteStatusSkipped:
		return prior
	case routing.RouteStatusRouted, routing.RouteStatusFailed:
		// The second pass routes the endpoints left by the first pass, so its
		// terminal result is authoritative for the combined net.
		return current
	default:
		// Preserve a known prior status if a future router emits an unknown one.
		return prior
	}
}

func uniqueRoutingNets(groups ...[]routing.Net) []routing.Net {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	unique := make([]routing.Net, 0, total)
	indexByName := make(map[string]int, total)
	for _, group := range groups {
		for _, net := range group {
			// Net names are normalized exactly once per occurrence. The key helper
			// is intentionally a trim-only operation so display case remains intact.
			key := interBlockSummaryNetKey(net.Name)
			if index, exists := indexByName[key]; exists {
				unique[index] = mergeSequentialRoutingNet(unique[index], net)
				continue
			}
			indexByName[key] = len(unique)
			unique = append(unique, net)
		}
	}
	return unique
}

func mergeSequentialRoutingNet(prior, current routing.Net) routing.Net {
	merged := prior
	if strings.TrimSpace(current.Name) != "" {
		merged.Name = current.Name
	}
	merged.Endpoints = mergeRoutingEndpoints(prior.Endpoints, current.Endpoints)
	if current.Role != "" {
		merged.Role = current.Role
	}
	if strings.TrimSpace(current.Class) != "" {
		merged.Class = current.Class
	}
	merged.Priority = max(prior.Priority, current.Priority)
	merged.Fixed = prior.Fixed || current.Fixed
	return merged
}

func mergeRoutingEndpoints(groups ...[]routing.Endpoint) []routing.Endpoint {
	merged := make([]routing.Endpoint, 0)
	type endpointKey struct {
		ref string
		pin string
	}
	seen := make(map[endpointKey]struct{})
	for _, group := range groups {
		for _, endpoint := range group {
			// routing.Endpoint carries only Ref and Pin, so this key preserves all
			// available endpoint identity. KiCad ref/pin lookup is case-insensitive.
			key := endpointKey{ref: strings.ToUpper(strings.TrimSpace(endpoint.Ref)), pin: strings.ToUpper(strings.TrimSpace(endpoint.Pin))}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, endpoint)
		}
	}
	return merged
}

func localRouteConnectivityWithAdditionalIssues(summary LocalRouteConnectivitySummary, additional int) LocalRouteConnectivitySummary {
	if additional > 0 {
		summary.IssueCount += additional
	}
	return summary
}

func fragmentsRequireStrictDRC(fragments PCBFragmentResult) bool {
	for _, fragment := range fragments.Fragments {
		if fragment.Realization.Validation.RequiresDRC {
			return true
		}
	}
	return false
}

func requestRequiresStrictDRC(request Request, fragments PCBFragmentResult) bool {
	return request.Validation.RequireDRC ||
		request.Validation.Acceptance == AcceptanceERCDRC ||
		request.Validation.Acceptance == AcceptanceFabricationCandidate ||
		(request.RoutingRetry.Enabled && request.Validation.Acceptance == AcceptanceConnectivity) ||
		fragmentsRequireStrictDRC(fragments)
}

func addMultiEndpointGroundRouteObstacleNets(nets map[string]struct{}, candidates []InterBlockRouteCandidate) {
	for _, candidate := range candidates {
		if len(candidate.Endpoints) < 3 {
			continue
		}
		netName := strings.TrimSpace(candidate.NetName)
		switch netRoleFromName(netName) {
		case placement.NetGround:
			nets[netName] = struct{}{}
		}
	}
}

func compactRouteOperationGeometry(operations []transactions.Operation) []transactions.Operation {
	out := make([]transactions.Operation, 0, len(operations))
	for _, operation := range operations {
		if operation.Op != transactions.OpRoute {
			out = append(out, operation)
			continue
		}
		var payload transactions.RouteOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			out = append(out, operation)
			continue
		}
		payload.Points = compactRoutePoints(payload.Points)
		if routeTrackSegmentCount(payload.Points) == 0 {
			payload.Points = nil
			if len(payload.Vias) == 0 {
				continue
			}
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			out = append(out, operation)
			continue
		}
		compacted := transactions.NewOperation(transactions.OpRoute, raw)
		compacted.Index = operation.Index
		compacted.Ref = operation.Ref
		compacted.Net = operation.Net
		compacted.SnapExempt = operation.SnapExempt
		out = append(out, compacted)
	}
	return out
}

type platedPadViaTarget struct {
	netName     string
	point       transactions.Point
	radiusMM    float64
	widthMM     float64
	heightMM    float64
	rotationDeg float64
	shape       string
	cosAngle    float64
	sinAngle    float64
}

type platedPadViaTargetIndex struct {
	byKey          map[placedPadSpatialKey][]platedPadViaTarget
	oversizedByKey map[placedPadSpatialKey][]platedPadViaTarget
}

const platedPadOversizedSpatialBucketMM = 25.0

func platedPadOversizedSpatialKey(point transactions.Point) placedPadSpatialKey {
	return placedPadSpatialKey{x: int64(math.Floor(point.XMM / platedPadOversizedSpatialBucketMM)), y: int64(math.Floor(point.YMM / platedPadOversizedSpatialBucketMM))}
}

func newPlatedPadViaTargetIndex(targets []platedPadViaTarget) platedPadViaTargetIndex {
	index := platedPadViaTargetIndex{
		byKey:          make(map[placedPadSpatialKey][]platedPadViaTarget, len(targets)),
		oversizedByKey: make(map[placedPadSpatialKey][]platedPadViaTarget),
	}
	for _, source := range targets {
		target := normalizedPlatedPadViaTarget(source)
		if target.radiusMM > placedPadSpatialBucketMM {
			minKey := platedPadOversizedSpatialKey(transactions.Point{XMM: target.point.XMM - target.radiusMM, YMM: target.point.YMM - target.radiusMM})
			maxKey := platedPadOversizedSpatialKey(transactions.Point{XMM: target.point.XMM + target.radiusMM, YMM: target.point.YMM + target.radiusMM})
			for x := minKey.x; x <= maxKey.x; x++ {
				for y := minKey.y; y <= maxKey.y; y++ {
					key := placedPadSpatialKey{x: x, y: y}
					index.oversizedByKey[key] = append(index.oversizedByKey[key], target)
				}
			}
			continue
		}
		key := placedPadSpatialKeyForPoint(target.point)
		index.byKey[key] = append(index.byKey[key], target)
	}
	return index
}

func normalizedPlatedPadViaTarget(source platedPadViaTarget) platedPadViaTarget {
	target := source
	target.netName = routeNetKey(source.netName)
	target.shape = strings.ToLower(strings.TrimSpace(source.shape))
	angle := source.rotationDeg * math.Pi / 180
	target.cosAngle = math.Cos(angle)
	target.sinAngle = math.Sin(angle)
	return target
}

func (index platedPadViaTargetIndex) contains(netName string, point transactions.Point) bool {
	netName = routeNetKey(netName)
	key := placedPadSpatialKeyForPoint(point)
	for dx := int64(-1); dx <= 1; dx++ {
		for dy := int64(-1); dy <= 1; dy++ {
			for _, target := range index.byKey[placedPadSpatialKey{x: key.x + dx, y: key.y + dy}] {
				if target.netName == netName && target.contains(point) {
					return true
				}
			}
		}
	}
	for _, target := range index.oversizedByKey[platedPadOversizedSpatialKey(point)] {
		if target.netName == netName && target.contains(point) {
			return true
		}
	}
	return false
}

func (target platedPadViaTarget) contains(point transactions.Point) bool {
	dx := point.XMM - target.point.XMM
	dy := point.YMM - target.point.YMM
	localX := dx*target.cosAngle + dy*target.sinAngle
	localY := -dx*target.sinAngle + dy*target.cosAngle
	halfWidth := math.Max(0, target.widthMM) / 2
	halfHeight := math.Max(0, target.heightMM) / 2
	if halfWidth > 0 && halfHeight > 0 {
		switch target.shape {
		case "rect":
			return math.Abs(localX) <= halfWidth && math.Abs(localY) <= halfHeight
		case "oval", "roundrect":
			// A maximum-radius rounded rectangle is a capsule. This is exact for
			// KiCad ovals and conservative for every permitted roundrect corner
			// ratio, so a via is removed only when it is certainly inside copper.
			return pointInsideAxisAlignedCapsule(localX, localY, halfWidth, halfHeight)
		}
	}
	return dx*dx+dy*dy <= target.radiusMM*target.radiusMM
}

func pointInsideAxisAlignedCapsule(x, y, halfWidth, halfHeight float64) bool {
	if halfWidth >= halfHeight {
		capCenterX := math.Max(-halfWidth+halfHeight, math.Min(halfWidth-halfHeight, x))
		return math.Hypot(x-capCenterX, y) <= halfHeight
	}
	capCenterY := math.Max(-halfHeight+halfWidth, math.Min(halfHeight-halfWidth, y))
	return math.Hypot(x, y-capCenterY) <= halfWidth
}

func removeRedundantRouteViasAtPlatedPads(operations []transactions.Operation, placed *PlacementStageResult) []transactions.Operation {
	out, _ := removeRedundantRouteViasAtPlatedPadsWithEvidence(operations, placed)
	return out
}

func removeRedundantRouteViasAtPlatedPadsWithEvidence(operations []transactions.Operation, placed *PlacementStageResult) ([]transactions.Operation, map[int]struct{}) {
	out, reduced, _ := removeRedundantRouteViasAtPlatedPadsWithContext(operations, placed, newPhysicalPadRoutingContext(placed))
	return out, reduced
}

func removeRedundantRouteViasAtPlatedPadsWithContext(operations []transactions.Operation, placed *PlacementStageResult, physical physicalPadRoutingContext) ([]transactions.Operation, map[int]struct{}, []reports.Issue) {
	if placed == nil || !physical.valid {
		return operations, nil, nil
	}
	resolver := physical.resolver
	var targets []platedPadViaTarget
	for _, component := range placed.Request.Components {
		refKey := strings.ToUpper(strings.TrimSpace(component.Ref))
		routingNames := routingPadNames(component.Pads)
		for padIndex, pad := range component.Pads {
			padType := strings.ToLower(strings.TrimSpace(pad.Type))
			if pad.DrillMM <= 0 || padType == "np_thru_hole" || padIndex >= len(routingNames) {
				continue
			}
			padKey := strings.ToUpper(strings.TrimSpace(routingNames[padIndex]))
			endpoint, ok := resolver.ResolveNormalized(refKey, padKey)
			if !ok || strings.TrimSpace(endpoint.NetName) == "" {
				continue
			}
			widthMM := math.Max(pad.WidthMM, pad.DrillMM)
			heightMM := math.Max(pad.HeightMM, pad.DrillMM)
			containmentRadius := math.Min(widthMM, heightMM) / 2
			searchRadius := containmentRadius
			switch strings.ToLower(strings.TrimSpace(pad.Shape)) {
			case "rect":
				searchRadius = math.Hypot(widthMM, heightMM) / 2
			case "oval", "roundrect":
				searchRadius = math.Max(widthMM, heightMM) / 2
			}
			targets = append(targets, platedPadViaTarget{
				netName: endpoint.NetName, point: endpoint.Point, radiusMM: searchRadius,
				widthMM: widthMM, heightMM: heightMM, rotationDeg: endpoint.ComponentRotation + pad.RotationDeg, shape: pad.Shape,
			})
		}
	}
	if len(targets) == 0 {
		return operations, nil, nil
	}
	targetIndex := newPlatedPadViaTargetIndex(targets)
	out := make([]transactions.Operation, 0, len(operations))
	reduced := map[int]struct{}{}
	decodedRoutes := decodeRouteOperations(operations)
	var issues []reports.Issue
	for operationIndex, decoded := range decodedRoutes {
		operation := decoded.operation
		if operation.Op != transactions.OpRoute {
			out = append(out, operation)
			continue
		}
		if !decoded.decoded || len(decoded.payload.Vias) == 0 {
			out = append(out, operation)
			continue
		}
		payload := decoded.payload
		netName := strings.TrimSpace(firstNonEmpty(payload.NetName, operation.Net))
		var kept []transactions.RouteViaSpec
		changed := false
		for viaIndex, via := range payload.Vias {
			if targetIndex.contains(netName, via.At) {
				if !changed {
					kept = append(make([]transactions.RouteViaSpec, 0, len(payload.Vias)-1), payload.Vias[:viaIndex]...)
					changed = true
				}
				continue
			}
			if changed {
				kept = append(kept, via)
			}
		}
		// Raw JSON is rewritten only for operations that actually lost a via.
		if !changed {
			out = append(out, operation)
			continue
		}
		payload.Vias = kept
		raw, err := json.Marshal(payload)
		if err != nil {
			out = append(out, operation)
			issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: fmt.Sprintf("routing.via_reduction.operations[%d]", operationIndex), Message: "could not serialize route after redundant plated-pad via reduction: " + err.Error(), Nets: compactContactStrings([]string{netName})})
			continue
		}
		updated := operation
		updated.Raw = raw
		updated.PruneProtected = true
		out = append(out, updated)
		reduced[operationIndex] = struct{}{}
	}
	return out, reduced, issues
}

type danglingRouteStubCandidate struct {
	operationIndex int
	netName        string
	layer          string
	points         []transactions.Point
	terminal       [2]bool
	neighbors      [2][]int
}

type routeStubPairKey struct {
	net    string
	first  routeCoordKey
	second routeCoordKey
}

func expandViaReducedRoutePairs(operations []transactions.Operation, reduced map[int]struct{}) map[int]struct{} {
	return expandViaReducedRoutePairsDecoded(decodeRouteOperations(operations), reduced)
}

func expandViaReducedRoutePairsDecoded(decoded []decodedRouteOperation, reduced map[int]struct{}) map[int]struct{} {
	if len(reduced) == 0 {
		return reduced
	}
	keys := map[routeStubPairKey]struct{}{}
	for operationIndex := range reduced {
		if operationIndex < 0 || operationIndex >= len(decoded) || !decoded[operationIndex].decoded {
			continue
		}
		if key, ok := routeStubPairGeometryKey(decoded[operationIndex].payload); ok {
			keys[key] = struct{}{}
		}
	}
	for operationIndex, route := range decoded {
		if !route.decoded {
			continue
		}
		key, ok := routeStubPairGeometryKey(route.payload)
		if !ok {
			continue
		}
		if _, paired := keys[key]; paired {
			reduced[operationIndex] = struct{}{}
		}
	}
	return reduced
}

func routeStubPairGeometryKey(payload transactions.RouteOperation) (routeStubPairKey, bool) {
	if len(payload.Points) != 2 {
		return routeStubPairKey{}, false
	}
	first := routePointKey(payload.Points[0])
	second := routePointKey(payload.Points[1])
	if second.x < first.x || second.x == first.x && second.y < first.y {
		first, second = second, first
	}
	return routeStubPairKey{net: routeNetKey(payload.NetName), first: first, second: second}, true
}

// pruneRedundantDanglingRouteStubs peels two-point leaf routes from nets whose
// physical pads are already connected. A single spatial contact graph supplies
// all endpoint adjacency, and the queue updates only neighboring leaves.
func pruneRedundantDanglingRouteStubs(operations []transactions.Operation, placed *PlacementStageResult, eligible map[int]struct{}, physical physicalPadRoutingContext) []transactions.Operation {
	physicalEvidence := BuildInterBlockContactTargets(physical.candidates, placed)
	return pruneRedundantDanglingRouteStubsDecoded(operations, placed, eligible, physical, physicalEvidence, decodeRouteOperations(operations))
}

func pruneRedundantDanglingRouteStubsDecoded(operations []transactions.Operation, placed *PlacementStageResult, eligible map[int]struct{}, physical physicalPadRoutingContext, physicalEvidence InterBlockContactEvidence, decoded []decodedRouteOperation) []transactions.Operation {
	if len(eligible) == 0 {
		return operations
	}
	if !physical.valid || len(physical.candidates) == 0 {
		return operations
	}
	decodedByNet, decodeIssues := decodeInterBlockRouteOperationsFromDecoded(decoded)
	connected := interBlockConnectedNetsFromDecoded(interBlockContactTargetsByNet(physicalEvidence.Targets), decodedByNet, decodeIssues)
	padsBySummaryNet := map[string][]PlacedPadEndpoint{}
	for _, pad := range physical.resolver.Endpoints() {
		netKey := interBlockSummaryNetKey(pad.NetName)
		padsBySummaryNet[netKey] = append(padsBySummaryNet[netKey], pad)
	}
	if reports.HasBlockingIssue(decodeIssues) {
		return operations
	}
	decodedBySummaryNet := map[string][]decodedContactRouteOperation{}
	for netName, decoded := range decodedByNet {
		netKey := interBlockSummaryNetKey(netName)
		decodedBySummaryNet[netKey] = append(decodedBySummaryNet[netKey], decoded...)
	}
	removed := map[int]struct{}{}
	for netKey, decoded := range decodedBySummaryNet {
		if netKey == "" || !connected[netKey] {
			continue
		}
		candidates := map[int]*danglingRouteStubCandidate{}
		for _, route := range decoded {
			if len(route.Points) != 2 || len(route.Vias) != 0 {
				continue
			}
			if _, ok := eligible[route.SourceIndex]; !ok {
				continue
			}
			candidates[route.SourceIndex] = &danglingRouteStubCandidate{
				operationIndex: route.SourceIndex,
				netName:        route.NetName,
				layer:          route.Layer,
				points:         route.Points,
			}
		}
		if len(candidates) == 0 {
			continue
		}
		graph := newInterBlockContactGraph(decoded)
		pads := padsBySummaryNet[netKey]
		for operationIndex, candidate := range candidates {
			protectRouteStubPadContacts(candidate, pads)
			for endpointIndex, point := range candidate.points {
				for _, owner := range graph.copperOwnersAt(point, candidate.layer, interBlockContactToleranceMM, operationIndex) {
					if _, removableNeighbor := candidates[owner]; removableNeighbor {
						candidate.neighbors[endpointIndex] = append(candidate.neighbors[endpointIndex], owner)
					} else {
						candidate.terminal[endpointIndex] = true
					}
				}
			}
		}

		active := make(map[int]bool, len(candidates))
		queued := make(map[int]bool, len(candidates))
		queue := make([]int, 0, len(candidates))
		for operationIndex := range candidates {
			active[operationIndex] = true
		}
		enqueueLeaf := func(operationIndex int) {
			if !active[operationIndex] || queued[operationIndex] {
				return
			}
			candidate := candidates[operationIndex]
			contacts := [2]bool{candidate.terminal[0], candidate.terminal[1]}
			for endpointIndex := range contacts {
				for _, neighbor := range candidate.neighbors[endpointIndex] {
					if active[neighbor] {
						contacts[endpointIndex] = true
						break
					}
				}
			}
			if contacts[0] != contacts[1] {
				queue = append(queue, operationIndex)
				queued[operationIndex] = true
			}
		}
		for operationIndex := range candidates {
			enqueueLeaf(operationIndex)
		}
		for head := 0; head < len(queue); head++ {
			operationIndex := queue[head]
			queued[operationIndex] = false
			if !active[operationIndex] {
				continue
			}
			candidate := candidates[operationIndex]
			contacts := [2]bool{candidate.terminal[0], candidate.terminal[1]}
			for endpointIndex := range contacts {
				for _, neighbor := range candidate.neighbors[endpointIndex] {
					if active[neighbor] {
						contacts[endpointIndex] = true
						break
					}
				}
			}
			if contacts[0] == contacts[1] {
				continue
			}
			active[operationIndex] = false
			removed[candidate.operationIndex] = struct{}{}
			for _, endpointNeighbors := range candidate.neighbors {
				for _, neighbor := range endpointNeighbors {
					enqueueLeaf(neighbor)
				}
			}
		}
	}
	if len(removed) == 0 {
		return operations
	}
	out := make([]transactions.Operation, 0, len(operations)-len(removed))
	for operationIndex, operation := range operations {
		if _, drop := removed[operationIndex]; !drop {
			out = append(out, operation)
		}
	}
	return out
}

func protectRouteStubPadContacts(candidate *danglingRouteStubCandidate, pads []PlacedPadEndpoint) {
	if candidate == nil || len(candidate.points) != 2 {
		return
	}
	for _, pad := range pads {
		if !strings.EqualFold(strings.TrimSpace(pad.NetName), strings.TrimSpace(candidate.netName)) || !localRoutePadAppliesToLayer(pad, candidate.layer) {
			continue
		}
		tolerance := math.Hypot(math.Max(0, pad.PadWidthMM), math.Max(0, pad.PadHeightMM))/2 + interBlockContactToleranceMM
		if pointToSegmentDistanceMM(pad.Point, candidate.points[0], candidate.points[1]) > tolerance {
			continue
		}
		atEndpoint := false
		for endpointIndex, point := range candidate.points {
			if pointDistanceMM(point, pad.Point) <= tolerance {
				candidate.terminal[endpointIndex] = true
				atEndpoint = true
			}
		}
		if !atEndpoint {
			// A pad contacted in the segment interior makes both halves of this
			// unsplit operation electrically significant, so preserve it whole.
			candidate.terminal = [2]bool{true, true}
		}
	}
}

func addComponentHintSummaryToStage(stage *StageResult, hints []ComponentHintEvidence) {
	if stage == nil || len(hints) == 0 {
		return
	}
	if stage.Summary == nil {
		stage.Summary = map[string]any{}
	}
	stage.Summary["component_hints"] = hints
	stage.Summary["component_hint_summary"] = SummarizeComponentHints(hints)
}

func anchorBindingDiagnostics(request Request, fragments PCBFragmentResult, placed PlacementStageResult, route bool, opts RoutingOptions) (AnchorBindingSummary, []transactions.Operation, []reports.Issue) {
	if !fragmentsHaveEntryAnchors(fragments) {
		return AnchorBindingSummary{}, nil, nil
	}
	endpoints, endpointIssues := DiscoverPhysicalEndpointsWithOptions(placed, PhysicalEndpointDiscoveryOptions{
		ExternalEndpoints: request.ExternalEndpoints,
		Board:             request.Board,
	})
	summary := ResolveAnchorBindings(fragments, endpoints, AnchorBindingOptions{Placed: &placed})
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
	if request.Constraints.RouteWidthMM > 0 {
		routingRequest.Rules.TraceWidthMM = request.Constraints.RouteWidthMM
	}
	if request.Constraints.ClearanceMM > 0 {
		routingRequest.Rules.ClearanceMM = request.Constraints.ClearanceMM
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
	// Adapter defaults must never silently weaken the workflow-wide clearance.
	// A per-net override remains an explicit exception, while inherited classes
	// are raised to the same rule that the project writer will serialize.
	for name, class := range routingRequest.Rules.NetClasses {
		if class.ClearanceMM > 0 && class.ClearanceMM < routingRequest.Rules.ClearanceMM {
			class.ClearanceMM = routingRequest.Rules.ClearanceMM
			routingRequest.Rules.NetClasses[name] = class
		}
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
	operations, bindIssues, summary := bindLocalRouteOperations(fragments, resolver, placed.Request.Board)
	operations = append(preservedUnmodeledFragmentOperations(fragments), operations...)
	issues := append([]reports.Issue(nil), tableIssues...)
	issues = append(issues, resolver.Issues()...)
	issues = append(issues, bindIssues...)
	summary.IssueCount = len(issues)
	return operations, issues, summary
}

func rebuildMovedLocalRouteOperations(ctx context.Context, base routing.Request, operations []transactions.Operation) ([]transactions.Operation, []reports.Issue) {
	if ctx == nil {
		ctx = context.Background()
	}
	rebuildContext, cancelRebuild := context.WithTimeout(ctx, localRouteRebuildMaxDuration)
	defer cancelRebuild()
	ctx = rebuildContext
	type rebuildJob struct {
		index      int
		authored   transactions.RouteOperation
		netKey     string
		layerPairs [][2]string
		spanMM     float64
	}
	replacements := make([][]transactions.Operation, len(operations))
	fixed := make([]transactions.Operation, 0, len(operations))
	jobs := make([]rebuildJob, 0, len(operations))
	var issues []reports.Issue
	decodedRoutes := decodeRouteOperations(operations)
	for index, operation := range operations {
		if operation.Op != transactions.OpRoute || !operation.Rebuildable {
			replacements[index] = []transactions.Operation{operation}
			fixed = append(fixed, operation)
			continue
		}
		decoded := decodedRoutes[index]
		if !decoded.decoded || len(decoded.payload.Points) < 2 || strings.TrimSpace(decoded.payload.NetName) == "" {
			issues = append(issues, localRouteRebuildIssue(index, operation.Net, operation.RebuildRefs, "rebuildable local route has invalid authored geometry"))
			continue
		}
		authored := decoded.payload
		start := authored.Points[0]
		end := authored.Points[len(authored.Points)-1]
		sourceLayers := orderedLocalRouteRebuildLayers(operation.RebuildSourceLayers, authored.Layer, routeBranchDefaultLayer(base.Board))
		targetLayers := orderedLocalRouteRebuildLayers(operation.RebuildTargetLayers, authored.Layer, routeBranchDefaultLayer(base.Board))
		jobs = append(jobs, rebuildJob{
			index:      index,
			authored:   authored,
			netKey:     strings.TrimSpace(authored.NetName),
			layerPairs: localRouteRebuildLayerPairs(sourceLayers, targetLayers),
			spanMM:     math.Hypot(end.XMM-start.XMM, end.YMM-start.YMM),
		})
	}
	constrainedFirst := func(left, right rebuildJob) int {
		if left.authored.WidthMM != right.authored.WidthMM {
			if left.authored.WidthMM > right.authored.WidthMM {
				return -1
			}
			return 1
		}
		if math.Abs(left.spanMM-right.spanMM) > 0.001 {
			if left.spanMM < right.spanMM {
				return -1
			}
			return 1
		}
		if compare := strings.Compare(left.netKey, right.netKey); compare != 0 {
			return compare
		}
		return left.index - right.index
	}
	shortestFirst := func(left, right rebuildJob) int {
		if math.Abs(left.spanMM-right.spanMM) > 0.001 {
			if left.spanMM < right.spanMM {
				return -1
			}
			return 1
		}
		return constrainedFirst(left, right)
	}
	longestFirst := func(left, right rebuildJob) int {
		if math.Abs(left.spanMM-right.spanMM) > 0.001 {
			if left.spanMM > right.spanMM {
				return -1
			}
			return 1
		}
		return constrainedFirst(left, right)
	}
	signalFirst := func(left, right rebuildJob) int {
		if left.authored.WidthMM != right.authored.WidthMM {
			if left.authored.WidthMM < right.authored.WidthMM {
				return -1
			}
			return 1
		}
		return shortestFirst(left, right)
	}
	jobsByNet := map[string]int{}
	maxWidthByNet := map[string]float64{}
	for _, job := range jobs {
		jobsByNet[job.netKey]++
		maxWidthByNet[job.netKey] = max(maxWidthByNet[job.netKey], job.authored.WidthMM)
	}
	netCohesive := func(left, right rebuildJob) int {
		leftNet := left.netKey
		rightNet := right.netKey
		if leftNet != rightNet {
			if jobsByNet[leftNet] != jobsByNet[rightNet] {
				return jobsByNet[rightNet] - jobsByNet[leftNet]
			}
			if maxWidthByNet[leftNet] != maxWidthByNet[rightNet] {
				if maxWidthByNet[leftNet] > maxWidthByNet[rightNet] {
					return -1
				}
				return 1
			}
			return strings.Compare(leftNet, rightNet)
		}
		return shortestFirst(left, right)
	}
	wideNetCohesive := func(left, right rebuildJob) int {
		leftNet := left.netKey
		rightNet := right.netKey
		if leftNet != rightNet {
			if maxWidthByNet[leftNet] != maxWidthByNet[rightNet] {
				if maxWidthByNet[leftNet] > maxWidthByNet[rightNet] {
					return -1
				}
				return 1
			}
			if jobsByNet[leftNet] != jobsByNet[rightNet] {
				return jobsByNet[rightNet] - jobsByNet[leftNet]
			}
			return strings.Compare(leftNet, rightNet)
		}
		return constrainedFirst(left, right)
	}
	originalOrder := func(left, right rebuildJob) int { return left.index - right.index }
	strategies := []func(rebuildJob, rebuildJob) int{constrainedFirst, netCohesive, wideNetCohesive, shortestFirst, originalOrder, longestFirst, signalFirst}
	defaultLayer := routeBranchDefaultLayer(base.Board)
	fixedExisting := append([]routing.ExistingCopper(nil), base.Existing...)
	fixedExisting = append(fixedExisting, existingCopperFromAllRouteOperations(fixed, defaultLayer, base.Rules)...)
	bestReplacements := replacements
	bestIssues := append([]reports.Issue(nil), issues...)
	bestRouted := -1
	attempts := 0
	attemptBudget := localRouteRebuildStrategyBudget(len(jobs))
	routerCalls := 0
	routerCallBudget := localRouteRebuildRouterCallBudget(len(jobs))
	tryStrategy := func(strategy func(rebuildJob, rebuildJob) int) bool {
		if ctx != nil && ctx.Err() != nil || attempts >= attemptBudget || routerCalls >= routerCallBudget {
			return false
		}
		remainingAttempts := attemptBudget - attempts
		remainingCalls := routerCallBudget - routerCalls
		// Floor division reserves an equal call share for every later strategy;
		// this allowance is recomputed from actual consumption on each attempt.
		strategyCallAllowance := max(1, remainingCalls/remainingAttempts)
		strategyCallLimit := min(routerCallBudget, routerCalls+strategyCallAllowance)
		attempts++
		ordered := slices.Clone(jobs)
		slices.SortStableFunc(ordered, strategy)
		attemptReplacements := make([][]transactions.Operation, len(replacements))
		for index := range replacements {
			attemptReplacements[index] = append([]transactions.Operation(nil), replacements[index]...)
		}
		attemptIssues := append([]reports.Issue(nil), issues...)
		currentExisting := append([]routing.ExistingCopper(nil), fixedExisting...)
		routedCount := 0
		for _, job := range ordered {
			if ctx != nil && ctx.Err() != nil || routerCalls >= strategyCallLimit {
				return false
			}
			index := job.index
			authored := job.authored
			failureMessages := make([]string, 0, len(job.layerPairs))
			addFailureMessage := func(message string) {
				message = strings.TrimSpace(message)
				if message != "" && !slices.Contains(failureMessages, message) {
					failureMessages = append(failureMessages, message)
				}
			}
			var routed []transactions.Operation
			var routedBranches []routing.Route
			var selectedIssues []reports.Issue
			for pairRank, layerPair := range job.layerPairs {
				if ctx != nil && ctx.Err() != nil {
					return false
				}
				if routerCalls >= strategyCallLimit {
					addFailureMessage("local route rebuild per-strategy router-call budget exhausted")
					break
				}
				pair := routeTreeBranchAccessPair{
					Rank:   pairRank,
					Source: routeTreeBranchAccessCandidate{Access: RouteTreeEndpointAccess{Role: RouteTreeAccessLocalRouteAnchor, Net: authored.NetName, Layer: layerPair[0], XMM: authored.Points[0].XMM, YMM: authored.Points[0].YMM}},
					Target: routeTreeBranchAccessCandidate{Access: RouteTreeEndpointAccess{Role: RouteTreeAccessLocalRouteAnchor, Net: authored.NetName, Layer: layerPair[1], XMM: authored.Points[len(authored.Points)-1].XMM, YMM: authored.Points[len(authored.Points)-1].YMM}},
				}
				request := base
				request.Existing = currentExisting
				request = routeTreeAccessBranchRequest(request, authored.NetName, pair)
				routerCalls++
				result := routing.RouteRequestContext(ctx, request)
				candidateIssues := make([]reports.Issue, 0, len(result.Issues))
				for _, issue := range result.Issues {
					if issue.Blocking() || issue.Severity == reports.SeverityWarning {
						candidateIssues = append(candidateIssues, issue)
						if issue.Blocking() {
							addFailureMessage(issue.Message)
						}
					}
				}
				candidateRoutes := transactionRouteOperations(result.Operations)
				if result.Status == routing.StatusBlocked || len(candidateRoutes) == 0 {
					if len(selectedIssues) == 0 || len(candidateIssues) < len(selectedIssues) {
						selectedIssues = candidateIssues
					}
					continue
				}
				routed = candidateRoutes
				routedBranches = result.Routes
				selectedIssues = candidateIssues
				break
			}
			attemptIssues = append(attemptIssues, selectedIssues...)
			if len(routed) == 0 {
				if len(failureMessages) == 0 {
					addFailureMessage("obstacle-aware local route rebuild found no legal path")
				}
				attemptIssues = append(attemptIssues, localRouteRebuildIssue(index, authored.NetName, operations[index].RebuildRefs, strings.Join(failureMessages, "; ")))
				continue
			}
			attemptReplacements[index] = routed
			currentExisting = append(currentExisting, existingCopperFromRoutedBranches(routedBranches, defaultLayer, base.Rules)...)
			routedCount++
		}
		if routedCount > bestRouted || (routedCount == bestRouted && len(attemptIssues) < len(bestIssues)) {
			bestRouted = routedCount
			bestReplacements = attemptReplacements
			bestIssues = attemptIssues
		}
		return routedCount == len(jobs)
	}
	complete := false
	for _, strategy := range strategies {
		if ctx != nil && ctx.Err() != nil || attempts >= attemptBudget || routerCalls >= routerCallBudget {
			break
		}
		if tryStrategy(strategy) {
			complete = true
			break
		}
	}
	if !complete && attempts < attemptBudget && routerCalls < routerCallBudget {
		failed := map[int]struct{}{}
		for _, job := range jobs {
			if len(bestReplacements[job.index]) == 0 {
				failed[job.index] = struct{}{}
			}
		}
		failedFirst := func(left, right rebuildJob) int {
			_, leftFailed := failed[left.index]
			_, rightFailed := failed[right.index]
			if leftFailed != rightFailed {
				if leftFailed {
					return -1
				}
				return 1
			}
			return constrainedFirst(left, right)
		}
		complete = tryStrategy(failedFirst)
		if !complete && attempts < attemptBudget && routerCalls < routerCallBudget {
			failedIndexes := make([]int, 0, len(failed))
			for index := range failed {
				failedIndexes = append(failedIndexes, index)
			}
			slices.Sort(failedIndexes)
			for _, priorityIndex := range failedIndexes {
				if attempts >= attemptBudget || routerCalls >= routerCallBudget {
					break
				}
				priorityFirst := func(left, right rebuildJob) int {
					if left.index == priorityIndex && right.index != priorityIndex {
						return -1
					}
					if right.index == priorityIndex && left.index != priorityIndex {
						return 1
					}
					return constrainedFirst(left, right)
				}
				if tryStrategy(priorityFirst) {
					break
				}
			}
		}
	}
	if ctx != nil && ctx.Err() != nil {
		issues = append(issues, reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityBlocked, Path: "routing.local_route_rebuild", Message: ctx.Err().Error()})
		return operations, issues
	}
	replacements = bestReplacements
	issues = bestIssues
	out := make([]transactions.Operation, 0, len(operations))
	for _, replacement := range replacements {
		out = append(out, replacement...)
	}
	return out, issues
}

func localRouteRebuildStrategyBudget(jobCount int) int {
	switch {
	case jobCount <= 8:
		return 8
	case jobCount <= 32:
		return 4
	default:
		return 2
	}
}

func localRouteRebuildRouterCallBudget(jobCount int) int {
	if jobCount <= 0 {
		return 0
	}
	// Give every deterministic ordering two router calls per job while retaining
	// a strict linear bound. tryStrategy divides the remaining calls evenly
	// across the remaining orderings so an early ordering cannot starve later
	// alternatives.
	strategyCount := localRouteRebuildStrategyBudget(jobCount)
	if jobCount > math.MaxInt/(strategyCount*2) {
		return math.MaxInt
	}
	return min(localRouteRebuildMaxRouterCalls, jobCount*strategyCount*2)
}

func orderedLocalRouteRebuildLayers(layers []string, authoredLayer string, defaultLayer string) []string {
	preferred := canonicalCopperLayer(firstNonEmpty(authoredLayer, defaultLayer))
	seen := map[string]struct{}{}
	out := make([]string, 0, max(1, len(layers)))
	appendLayer := func(layer string) {
		canonical := canonicalCopperLayer(firstNonEmpty(layer, defaultLayer))
		key := strings.ToUpper(strings.TrimSpace(canonical))
		if key == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		out = append(out, canonical)
	}
	if len(layers) == 0 {
		appendLayer(preferred)
		return out
	}
	for _, layer := range layers {
		if strings.EqualFold(canonicalCopperLayer(layer), preferred) {
			appendLayer(layer)
		}
	}
	for _, layer := range layers {
		appendLayer(layer)
	}
	return out
}

func localRouteRebuildLayerPairs(sourceLayers []string, targetLayers []string) [][2]string {
	pairs := make([][2]string, 0, len(sourceLayers)*len(targetLayers))
	for _, sourceLayer := range sourceLayers {
		for _, targetLayer := range targetLayers {
			pairs = append(pairs, [2]string{sourceLayer, targetLayer})
		}
	}
	return pairs
}

func localRouteRebuildIssue(index int, netName string, refs []string, message string) reports.Issue {
	return reports.Issue{
		Code:       reports.CodeValidationFailed,
		Severity:   reports.SeverityBlocked,
		Path:       fmt.Sprintf("design.local_route_rebuild.operations[%d]", index),
		Message:    message,
		Refs:       compactContactStrings(refs),
		Nets:       compactContactStrings([]string{strings.TrimSpace(netName)}),
		Suggestion: "adjust movable placement or local routing constraints and retry deterministically",
	}
}

func summarizeInterBlockRouteCompletion(candidates []InterBlockRouteCandidate, operations []transactions.Operation, issues []reports.Issue, contactEvidence InterBlockContactEvidence) InterBlockRouteCompletionSummary {
	return summarizeInterBlockRouteCompletionWithGraphOperations(candidates, operations, operations, issues, contactEvidence)
}

func summarizeInterBlockRouteCompletionWithGraphOperations(candidates []InterBlockRouteCandidate, routeOperations []transactions.Operation, graphOperations []transactions.Operation, issues []reports.Issue, contactEvidence InterBlockContactEvidence) InterBlockRouteCompletionSummary {
	summary := InterBlockRouteCompletionSummary{
		NetsConsidered:  len(candidates),
		Candidates:      len(candidates),
		RoutesAttempted: len(candidates),
	}
	groups, groupIssues := BuildInterBlockRouteGroups(candidates)
	trees := BuildInterBlockRouteTrees(groups, contactEvidence)
	provenEndpoints := provenInterBlockEndpointSet(contactEvidence)
	targetsByNet := interBlockContactTargetsByNet(contactEvidence.Targets)
	operationsByNet, operationIssues := decodeInterBlockRouteOperations(graphOperations)
	graphComponents := interBlockGraphComponentCountsFromDecoded(targetsByNet, operationsByNet, operationIssues)
	routeSegmentsByNet := routeSegmentCountsByNet(routeOperations)
	issueCountsByNet := issueCountsByNet(issues)
	blockingIssueCountsByNet := blockingIssueCountsByNet(issues)
	for _, issue := range groupIssues {
		for _, net := range issue.Nets {
			net = interBlockSummaryNetKey(net)
			if net != "" {
				issueCountsByNet[net]++
				if issue.Blocking() {
					blockingIssueCountsByNet[net]++
				}
			}
		}
	}
	connectedNets := interBlockConnectedNetsFromDecoded(targetsByNet, operationsByNet, operationIssues)
	treeByNet := interBlockRouteTreeByNet(trees)
	summarizeInterBlockRouteGroups(&summary, groups, treeByNet, provenEndpoints, graphComponents, routeSegmentsByNet, blockingIssueCountsByNet, connectedNets)
	for _, candidate := range candidates {
		netName := interBlockSummaryNetKey(candidate.NetName)
		summary.EndpointsResolved += len(candidate.Endpoints)
		summary.EndpointsUnresolved += candidate.Unresolved
		segments := routeSegmentsByNet[netName]
		summary.EmittedSegments += segments
		netIssueCount := issueCountsByNet[netName]
		summary.IssueCount += netIssueCount
		blockingIssueCount := blockingIssueCountsByNet[netName]
		switch {
		case connectedNets[netName] && blockingIssueCount == 0:
			summary.RoutesCompleted++
		case segments > 0:
			summary.PartialNets++
		default:
			summary.UnroutedNets++
		}
	}
	return summary
}

func summarizeInterBlockRouteGroups(summary *InterBlockRouteCompletionSummary, groups []InterBlockRouteGroup, treeByNet map[string]InterBlockRouteTree, provenEndpoints map[string]bool, graphComponents map[string]int, routeSegmentsByNet map[string]int, issueCountsByNet map[string]int, connectedNets map[string]bool) {
	if summary == nil {
		return
	}
	for _, group := range groups {
		if len(group.RequiredEndpoints) > 2 {
			summary.MultiEndpointNets++
		}
		summary.RequiredEndpoints += len(group.RequiredEndpoints) + group.UnresolvedRequired
		netName := interBlockSummaryNetKey(group.NetName)
		tree := treeByNet[netName]
		missingRequired := group.UnresolvedRequired
		if missingRequired == 0 {
			missingRequired = len(tree.MissingEndpointIDs)
		}
		summary.MissingRequired += missingRequired
		summary.BranchesPlanned += len(tree.Branches)
		if routeSegmentsByNet[netName] > 0 {
			summary.BranchesAttempted += len(tree.Branches)
		}
		groupProven := 0
		for _, endpoint := range group.RequiredEndpoints {
			if provenEndpoints[interBlockEndpointKey(endpoint.Ref, endpoint.Pin)] {
				groupProven++
				summary.ProvenEndpoints++
			}
		}
		for _, branch := range tree.Branches {
			if provenEndpoints[branch.StartEndpointID] && provenEndpoints[branch.EndEndpointID] {
				summary.BranchesCompleted++
			}
		}
		componentCount := graphComponents[netName]
		summary.GraphComponentCount += componentCount
		netIssueCount := issueCountsByNet[netName]
		switch {
		case connectedNets[netName] && missingRequired == 0 && groupProven == len(group.RequiredEndpoints) && netIssueCount == 0:
			summary.CompleteGroups++
		case groupProven > 0 || routeSegmentsByNet[netName] > 0:
			summary.PartialGroups++
		default:
			summary.BlockedGroups++
		}
	}
}

func interBlockSummaryNetKey(netName string) string {
	return strings.TrimSpace(netName)
}

func interBlockRouteTreeByNet(trees []InterBlockRouteTree) map[string]InterBlockRouteTree {
	byNet := make(map[string]InterBlockRouteTree, len(trees))
	for _, tree := range trees {
		byNet[interBlockSummaryNetKey(tree.NetName)] = tree
	}
	return byNet
}

// routeResidualPhysicalPadTrees closes physical multi-pad nets against
// already-emitted same-net copper before the ordinary pad-to-pad router runs.
// The fallback is intentionally bounded to one deterministic tree attempt per
// eligible net and commits an attempt only when the physical contact graph
// proves every endpoint connected.
func routeResidualPhysicalPadTrees(ctx context.Context, base routing.Request, candidates []InterBlockRouteCandidate, included map[string]bool, placed *PlacementStageResult, contactOperations []transactions.Operation) residualPhysicalRouteTreeResult {
	result := residualPhysicalRouteTreeResult{}
	workingBase := base
	workingBase.Existing = append([]routing.ExistingCopper(nil), base.Existing...)
	workingContacts := append([]transactions.Operation(nil), contactOperations...)
	defaultLayer := routeBranchDefaultLayer(base.Board)
	for _, candidate := range candidates {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		netName := interBlockSummaryNetKey(candidate.NetName)
		if netName == "" || !included[netName] || candidate.Status != InterBlockRouteCandidateRoutable || !routingExistingContainsNet(workingBase.Existing, netName) {
			continue
		}
		_, before := remainingPhysicalPadRoutingNetsWithCandidates(workingBase.Nets, placed, workingContacts, []InterBlockRouteCandidate{candidate})
		if len(before.RemainingNets) == 0 {
			continue
		}
		result.Summary.Candidates++
		evidence := BuildInterBlockContactTargets([]InterBlockRouteCandidate{candidate}, placed)
		access, accessIssues := BuildRouteTreeEndpointAccessWithIssues(evidence, workingContacts)
		if reports.HasBlockingIssue(evidence.Issues) || reports.HasBlockingIssue(accessIssues) {
			continue
		}
		result.Summary.Attempts++
		result.Summary.AttemptedNets = append(result.Summary.AttemptedNets, netName)
		attempt := executeInterBlockRouteTrees(ctx, workingBase, []InterBlockRouteCandidate{candidate}, evidence, access, nil, nil)
		attemptContacts := slices.Concat(workingContacts, attempt.Operations)
		if !residualPhysicalRouteTreeContactProven(workingBase.Nets, placed, attemptContacts, candidate) {
			continue
		}
		result.Operations = append(result.Operations, attempt.Operations...)
		result.Issues = append(result.Issues, evidence.Issues...)
		result.Issues = append(result.Issues, accessIssues...)
		result.Issues = append(result.Issues, nonBlockingRouteTreeIssues(attempt.Issues)...)
		result.Summary.CompletedNets = append(result.Summary.CompletedNets, netName)
		workingContacts = attemptContacts
		workingBase.Existing = append(workingBase.Existing, existingCopperFromAllRouteOperations(attempt.Operations, defaultLayer, workingBase.Rules)...)
	}
	result.Summary.CompletedNets = uniqueSortedInterBlockNets(result.Summary.CompletedNets)
	result.Summary.AttemptedNets = uniqueSortedInterBlockNets(result.Summary.AttemptedNets)
	return result
}

func residualPhysicalRouteTreeContactProven(nets []routing.Net, placed *PlacementStageResult, operations []transactions.Operation, candidate InterBlockRouteCandidate) bool {
	_, completion := remainingPhysicalPadRoutingNetsWithCandidates(nets, placed, operations, []InterBlockRouteCandidate{candidate})
	return len(completion.RemainingNets) == 0
}

func stringBoolSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		if key := interBlockSummaryNetKey(value); key != "" {
			set[key] = true
		}
	}
	return set
}

func reconcileContactProvenRoutingResult(result routing.Result, completedNets []string) routing.Result {
	completed := stringBoolSet(completedNets)
	if len(completed) == 0 {
		return result
	}
	converted := 0
	for index := range result.Routes {
		route := &result.Routes[index]
		if !completed[interBlockSummaryNetKey(route.Net)] || route.Status != routing.RouteStatusFailed {
			continue
		}
		route.Status = routing.RouteStatusRouted
		route.Issues = nonBlockingRouteTreeIssues(route.Issues)
		converted++
	}
	filteredIssues := make([]reports.Issue, 0, len(result.Issues))
	for _, issue := range result.Issues {
		if issue.Blocking() && issueNetsAreCompleted(issue, completed) {
			continue
		}
		filteredIssues = append(filteredIssues, issue)
	}
	result.Issues = filteredIssues
	result.Metrics.FailedNetCount = max(0, result.Metrics.FailedNetCount-converted)
	result.Metrics.RoutedNetCount += converted
	switch {
	case reports.HasBlockingIssue(result.Issues):
		result.Status = routing.StatusBlocked
	case result.Metrics.FailedNetCount > 0:
		result.Status = routing.StatusPartial
	default:
		result.Status = routing.StatusRouted
	}
	return result
}

func issueNetsAreCompleted(issue reports.Issue, completed map[string]bool) bool {
	if len(issue.Nets) == 0 {
		return false
	}
	for _, netName := range issue.Nets {
		if !completed[interBlockSummaryNetKey(netName)] {
			return false
		}
	}
	return true
}

func nonBlockingRouteTreeIssues(issues []reports.Issue) []reports.Issue {
	filtered := make([]reports.Issue, 0, len(issues))
	for _, issue := range issues {
		if !issue.Blocking() {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}

func routingExistingContainsNet(existing []routing.ExistingCopper, netName string) bool {
	for _, copper := range existing {
		if strings.EqualFold(strings.TrimSpace(copper.Net), strings.TrimSpace(netName)) {
			return true
		}
	}
	return false
}

func executeInterBlockRouteTrees(ctx context.Context, base routing.Request, candidates []InterBlockRouteCandidate, targetEvidence InterBlockContactEvidence, routeTreeAccess []RouteTreeEndpointAccess, selectiveExisting []routing.ExistingCopper, selectiveNets map[string]struct{}) interBlockRouteTreeExecutionResult {
	groups, groupIssues := BuildInterBlockRouteGroups(candidates)
	trees := BuildInterBlockRouteTrees(groups, targetEvidence)
	trees = orderInterBlockRouteTreesForRouting(trees, base.Nets)
	groupByNet := interBlockRouteGroupByNet(groups)
	baseline := executeInterBlockRouteTreesOrdered(ctx, base, trees, groupByNet, routeTreeAccess, selectiveExisting, selectiveNets)
	baseline.Summary.OrderAttempts = 1
	baseline.Summary.SelectedOrder = "baseline"
	selected := baseline
	if (ctx == nil || ctx.Err() == nil) && baseline.Summary.BranchesBlocked > 0 {
		blockedNets := blockedInterBlockRouteTreeNets(baseline.Branches)
		alternateTrees := promoteInterBlockRouteTrees(trees, blockedNets)
		if !slices.EqualFunc(trees, alternateTrees, func(left, right InterBlockRouteTree) bool { return left.NetName == right.NetName }) {
			alternate := executeInterBlockRouteTreesOrdered(ctx, base, alternateTrees, groupByNet, routeTreeAccess, selectiveExisting, selectiveNets)
			alternate.Summary.OrderAttempts = 2
			alternate.Summary.SelectedOrder = "blocked_tree_first"
			if interBlockRouteTreeExecutionBetter(alternate, baseline) {
				selected = alternate
			} else {
				selected.Summary.OrderAttempts = 2
			}
		}
	}
	selected.Issues = append(append([]reports.Issue(nil), groupIssues...), selected.Issues...)
	selected.Summary.IssueCount = len(selected.Issues)
	selected.Summary.BlockingIssueCount, selected.Summary.WarningIssueCount, selected.Summary.InfoIssueCount, selected.Summary.FixedNetSkipNotices = routeTreeIssueCounters(selected.Issues)
	return selected
}

func executeInterBlockRouteTreesOrdered(ctx context.Context, base routing.Request, trees []InterBlockRouteTree, groupByNet map[string]InterBlockRouteGroup, routeTreeAccess []RouteTreeEndpointAccess, selectiveExisting []routing.ExistingCopper, selectiveNets map[string]struct{}) interBlockRouteTreeExecutionResult {
	execution := interBlockRouteTreeExecutionResult{}
	workingBase := base
	workingBase.Existing = append([]routing.ExistingCopper(nil), base.Existing...)
	selectiveBase := base
	selectiveBase.Existing = make([]routing.ExistingCopper, 0, len(base.Existing)+len(selectiveExisting))
	selectiveBase.Existing = append(selectiveBase.Existing, base.Existing...)
	selectiveBase.Existing = append(selectiveBase.Existing, selectiveExisting...)
	for _, tree := range trees {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		netName := interBlockSummaryNetKey(tree.NetName)
		if netName == "" || len(tree.Branches) == 0 || len(tree.MissingEndpointIDs) != 0 {
			continue
		}
		group, ok := groupByNet[netName]
		if !ok || group.Status != InterBlockRouteCandidateRoutable {
			continue
		}
		execution.Summary.GroupsPlanned++
		execution.Summary.GroupsAttempted++
		execution.Summary.BranchesPlanned += len(tree.Branches)
		execution.Summary.ManagedNets = append(execution.Summary.ManagedNets, netName)
		treeBase := workingBase
		if _, selected := selectiveNets[strings.TrimSpace(netName)]; selected {
			treeBase = selectiveBase
		}
		branchResult := RouteInterBlockTreeBranchesWithAccess(ctx, treeBase, group, tree, routeTreeAccess)
		execution.Operations = append(execution.Operations, branchResult.Operations...)
		execution.Issues = append(execution.Issues, branchResult.Issues...)
		execution.Branches = append(execution.Branches, RouteTreeBranchEvidenceSummary{
			NetName:  branchResult.NetName,
			Branches: append([]InterBlockBranchRoutingEvidence(nil), branchResult.Branches...),
		})
		workingBase.Existing = append(workingBase.Existing, branchResult.ExistingCopper...)
		selectiveBase.Existing = append(selectiveBase.Existing, branchResult.ExistingCopper...)
		routedBranches := 0
		blockedBranches := 0
		for _, branch := range branchResult.Branches {
			execution.Summary.BranchesAttempted++
			if branch.Status == routing.StatusRouted {
				routedBranches++
				execution.Summary.BranchesRouted++
				continue
			}
			blockedBranches++
			execution.Summary.BranchesBlocked++
		}
		switch {
		case routedBranches == len(tree.Branches) && blockedBranches == 0:
			execution.Summary.GroupsComplete++
		case routedBranches > 0:
			execution.Summary.GroupsPartial++
		default:
			execution.Summary.GroupsBlocked++
		}
	}
	execution.Summary.ManagedNets = uniqueSortedInterBlockNets(execution.Summary.ManagedNets)
	execution.Summary.IssueCount = len(execution.Issues)
	execution.Summary.BlockingIssueCount, execution.Summary.WarningIssueCount, execution.Summary.InfoIssueCount, execution.Summary.FixedNetSkipNotices = routeTreeIssueCounters(execution.Issues)
	return execution
}

func blockedInterBlockRouteTreeNets(branches []RouteTreeBranchEvidenceSummary) map[string]struct{} {
	blocked := map[string]struct{}{}
	for _, tree := range branches {
		for _, branch := range tree.Branches {
			if branch.Status != routing.StatusRouted {
				blocked[interBlockSummaryNetKey(tree.NetName)] = struct{}{}
				break
			}
		}
	}
	return blocked
}

func promoteInterBlockRouteTrees(trees []InterBlockRouteTree, promoted map[string]struct{}) []InterBlockRouteTree {
	if len(trees) == 0 || len(promoted) == 0 {
		return append([]InterBlockRouteTree(nil), trees...)
	}
	ordered := make([]InterBlockRouteTree, 0, len(trees))
	for _, tree := range trees {
		if _, ok := promoted[interBlockSummaryNetKey(tree.NetName)]; ok {
			ordered = append(ordered, tree)
		}
	}
	for _, tree := range trees {
		if _, ok := promoted[interBlockSummaryNetKey(tree.NetName)]; !ok {
			ordered = append(ordered, tree)
		}
	}
	return ordered
}

func interBlockRouteTreeExecutionBetter(candidate, baseline interBlockRouteTreeExecutionResult) bool {
	if candidate.Summary.BranchesBlocked != baseline.Summary.BranchesBlocked {
		return candidate.Summary.BranchesBlocked < baseline.Summary.BranchesBlocked
	}
	if candidate.Summary.BranchesRouted != baseline.Summary.BranchesRouted {
		return candidate.Summary.BranchesRouted > baseline.Summary.BranchesRouted
	}
	if candidate.Summary.GroupsComplete != baseline.Summary.GroupsComplete {
		return candidate.Summary.GroupsComplete > baseline.Summary.GroupsComplete
	}
	return candidate.Summary.BlockingIssueCount < baseline.Summary.BlockingIssueCount
}

func promoteFailedNetPriorities(nets []routing.Net, failedSet map[string]struct{}) []routing.Net {
	promoted := append([]routing.Net(nil), nets...)
	maxPriority := 0
	for _, net := range promoted {
		if net.Priority > maxPriority {
			maxPriority = net.Priority
		}
	}
	maxInt := math.MaxInt
	promotedPriority := maxPriority
	if maxPriority < maxInt {
		promotedPriority++
	} else {
		promotedPriority = maxInt
		// Reserve maxInt for the failed nets without collapsing any existing
		// non-failed priority levels. There cannot be enough distinct levels in
		// an in-memory net slice to exhaust the signed integer range below it.
		nonFailedPriorities := make(map[int]struct{}, len(promoted))
		for _, net := range promoted {
			if _, failed := failedSet[interBlockSummaryNetKey(net.Name)]; !failed {
				nonFailedPriorities[net.Priority] = struct{}{}
			}
		}
		levels := make([]int, 0, len(nonFailedPriorities))
		for priority := range nonFailedPriorities {
			levels = append(levels, priority)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(levels)))
		normalized := make(map[int]int, len(levels))
		for index, priority := range levels {
			normalized[priority] = maxInt - 1 - index
		}
		for index := range promoted {
			if _, failed := failedSet[interBlockSummaryNetKey(promoted[index].Name)]; !failed {
				promoted[index].Priority = normalized[promoted[index].Priority]
			}
		}
	}
	for index := range promoted {
		if _, failed := failedSet[interBlockSummaryNetKey(promoted[index].Name)]; failed {
			promoted[index].Priority = promotedPriority
			promoted[index].OrderFirst = true
		}
	}
	return promoted
}

func blockingRoutingIssueNets(issues []reports.Issue, nets []routing.Net) []string {
	known := make(map[string]string, len(nets))
	for _, net := range nets {
		key := interBlockSummaryNetKey(net.Name)
		known[key] = net.Name
	}
	selected := map[string]string{}
	for _, issue := range issues {
		if issue.Severity != reports.SeverityBlocked && issue.Severity != reports.SeverityError {
			continue
		}
		for _, netName := range issue.Nets {
			key := interBlockSummaryNetKey(netName)
			if canonical, ok := known[key]; ok {
				selected[key] = canonical
			}
		}
	}
	out := make([]string, 0, len(selected))
	for _, netName := range selected {
		out = append(out, netName)
	}
	slices.Sort(out)
	return out
}

func routingResultBetter(candidate, baseline routing.Result) bool {
	if candidate.Metrics.FailedNetCount != baseline.Metrics.FailedNetCount {
		return candidate.Metrics.FailedNetCount < baseline.Metrics.FailedNetCount
	}
	if candidate.Metrics.RoutedNetCount != baseline.Metrics.RoutedNetCount {
		return candidate.Metrics.RoutedNetCount > baseline.Metrics.RoutedNetCount
	}
	return routingStatusRank(candidate.Status) > routingStatusRank(baseline.Status)
}

func orderInterBlockRouteTreesForRouting(trees []InterBlockRouteTree, nets []routing.Net) []InterBlockRouteTree {
	netByName := make(map[string]routing.Net, len(nets))
	for _, net := range nets {
		netByName[interBlockSummaryNetKey(net.Name)] = net
	}
	type rankedTree struct {
		tree     InterBlockRouteTree
		priority int
		roleRank int
	}
	ranked := make([]rankedTree, len(trees))
	for index, tree := range trees {
		net := netByName[interBlockSummaryNetKey(tree.NetName)]
		ranked[index] = rankedTree{tree: tree, priority: net.Priority, roleRank: interBlockRouteRoleRank(net.Role)}
	}
	sort.SliceStable(ranked, func(left, right int) bool {
		if ranked[left].priority != ranked[right].priority {
			return ranked[left].priority > ranked[right].priority
		}
		if ranked[left].roleRank != ranked[right].roleRank {
			return ranked[left].roleRank < ranked[right].roleRank
		}
		// High-fanout trees consume the most routing channels. Route them before
		// short point-to-point nets so later copper cannot partition their tree.
		if len(ranked[left].tree.Branches) != len(ranked[right].tree.Branches) {
			return len(ranked[left].tree.Branches) > len(ranked[right].tree.Branches)
		}
		return ranked[left].tree.NetName < ranked[right].tree.NetName
	})
	ordered := make([]InterBlockRouteTree, len(ranked))
	for index := range ranked {
		ordered[index] = ranked[index].tree
	}
	return ordered
}

func interBlockRouteRoleRank(role routing.NetRole) int {
	switch role {
	case routing.NetHighCurrent:
		return 0
	case routing.NetPower, routing.NetGround:
		return 1
	case routing.NetDifferential, routing.NetClock:
		return 2
	case routing.NetAnalog:
		return 3
	case routing.NetSignal:
		return 4
	default:
		return 5
	}
}

func interBlockRouteGroupByNet(groups []InterBlockRouteGroup) map[string]InterBlockRouteGroup {
	byNet := make(map[string]InterBlockRouteGroup, len(groups))
	for _, group := range groups {
		if netName := interBlockSummaryNetKey(group.NetName); netName != "" {
			byNet[netName] = group
		}
	}
	return byNet
}

func uniqueSortedInterBlockNets(nets []string) []string {
	if len(nets) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(nets))
	for _, net := range nets {
		net = interBlockSummaryNetKey(net)
		if net == "" {
			continue
		}
		if _, exists := seen[net]; exists {
			continue
		}
		seen[net] = struct{}{}
		out = append(out, net)
	}
	slices.Sort(out)
	return out
}

func provenInterBlockEndpointSet(evidence InterBlockContactEvidence) map[string]bool {
	proven := map[string]bool{}
	for _, proof := range evidence.Proofs {
		if proof.Status != InterBlockContactProven {
			continue
		}
		key := strings.TrimSpace(proof.Target.EndpointID)
		if key == "" {
			key = interBlockEndpointKey(proof.Target.Ref, proof.Target.Pad)
		}
		if key != "" {
			proven[key] = true
		}
	}
	return proven
}

func interBlockEndpointKey(ref string, pinOrPad string) string {
	ref = strings.TrimSpace(ref)
	pinOrPad = strings.TrimSpace(pinOrPad)
	if ref == "" || pinOrPad == "" {
		return ""
	}
	return normalizedRouteGroupEndpointKey(ref, pinOrPad)
}

func interBlockGraphComponentCountsFromDecoded(targetsByNet map[string][]InterBlockContactTarget, operationsByNet map[string][]decodedContactRouteOperation, operationIssues []reports.Issue) map[string]int {
	counts := map[string]int{}
	if len(targetsByNet) == 0 {
		return counts
	}
	issueNets := map[string]bool{}
	for _, issue := range operationIssues {
		for _, netName := range issue.Nets {
			netName = interBlockSummaryNetKey(netName)
			if netName != "" {
				issueNets[netName] = true
			}
		}
	}
	for rawNetName, targets := range targetsByNet {
		netName := interBlockSummaryNetKey(rawNetName)
		if issueNets[netName] {
			counts[netName] = len(targets)
			continue
		}
		graph := newInterBlockContactGraph(operationsByNet[netName])
		roots := map[int]bool{}
		missing := 0
		for _, target := range targets {
			node, ok := graph.findTargetNode(target)
			if !ok {
				missing++
				continue
			}
			roots[graph.find(node)] = true
		}
		counts[netName] = len(roots) + missing
	}
	return counts
}

func routeSegmentCountsByNet(operations []transactions.Operation) map[string]int {
	counts := map[string]int{}
	for _, operation := range operations {
		net := interBlockSummaryNetKey(operation.Net)
		if operation.Op != transactions.OpRoute || net == "" {
			continue
		}
		counts[net]++
	}
	return counts
}

func excludeNetsWithRouteOperations(nets []routing.Net, operations []transactions.Operation, candidates []InterBlockRouteCandidate) []routing.Net {
	routed := routeSegmentCountsByNet(operations)
	if len(routed) == 0 {
		return nets
	}
	interBlock := interBlockCandidateNets(candidates)
	filtered := make([]routing.Net, 0, len(nets))
	for _, net := range nets {
		netName := interBlockSummaryNetKey(net.Name)
		if _, ok := routed[netName]; ok && !interBlock[netName] {
			continue
		}
		filtered = append(filtered, net)
	}
	return filtered
}

func interBlockCandidateNets(candidates []InterBlockRouteCandidate) map[string]bool {
	nets := map[string]bool{}
	for _, candidate := range candidates {
		if netName := interBlockSummaryNetKey(candidate.NetName); netName != "" {
			nets[netName] = true
		}
	}
	return nets
}

func excludeRoutingNetsByName(nets []routing.Net, excluded map[string]bool) []routing.Net {
	if len(nets) == 0 || len(excluded) == 0 {
		return nets
	}
	filtered := make([]routing.Net, 0, len(nets))
	for _, net := range nets {
		if excluded[interBlockSummaryNetKey(net.Name)] {
			continue
		}
		filtered = append(filtered, net)
	}
	return filtered
}

func excludeManagedInterBlockNets(nets []routing.Net, managed []string) []routing.Net {
	if len(nets) == 0 || len(managed) == 0 {
		return nets
	}
	managedSet := map[string]struct{}{}
	for _, netName := range managed {
		if netName = interBlockSummaryNetKey(netName); netName != "" {
			managedSet[netName] = struct{}{}
		}
	}
	if len(managedSet) == 0 {
		return nets
	}
	filtered := make([]routing.Net, 0, len(nets))
	for _, net := range nets {
		if _, managed := managedSet[interBlockSummaryNetKey(net.Name)]; managed {
			continue
		}
		filtered = append(filtered, net)
	}
	return filtered
}

func remainingPhysicalPadRoutingNets(nets []routing.Net, placed *PlacementStageResult, operations []transactions.Operation) []routing.Net {
	remaining, _ := remainingPhysicalPadRoutingNetsWithSummary(nets, placed, operations)
	return remaining
}

func remainingPhysicalPadRoutingNetsWithSummary(nets []routing.Net, placed *PlacementStageResult, operations []transactions.Operation) ([]routing.Net, PhysicalPadRoutingCompletionSummary) {
	return remainingPhysicalPadRoutingNetsWithCandidates(nets, placed, operations, physicalPadRouteCandidates(placed))
}

func remainingPhysicalPadRoutingNetsWithCandidates(nets []routing.Net, placed *PlacementStageResult, operations []transactions.Operation, candidates []InterBlockRouteCandidate) ([]routing.Net, PhysicalPadRoutingCompletionSummary) {
	summary := PhysicalPadRoutingCompletionSummary{NetsConsidered: len(candidates), EndpointsByNet: map[string]int{}}
	if len(candidates) == 0 {
		return nets, summary
	}
	evidence := BuildInterBlockContactTargets(candidates, placed)
	connected := interBlockConnectedNets(evidence, operations)
	physical := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		key := interBlockSummaryNetKey(candidate.NetName)
		physical[key] = struct{}{}
		summary.EndpointsByNet[key] += len(candidate.Endpoints)
		summary.Endpoints += len(candidate.Endpoints)
		if connected[key] {
			summary.ConnectedNets = append(summary.ConnectedNets, key)
		}
	}
	slices.Sort(summary.ConnectedNets)
	remaining := make([]routing.Net, 0, len(nets))
	for _, net := range nets {
		key := interBlockSummaryNetKey(net.Name)
		if _, exists := physical[key]; !exists {
			continue
		}
		if connected[key] {
			continue
		}
		remaining = append(remaining, net)
		summary.RemainingNets = append(summary.RemainingNets, key)
	}
	slices.Sort(summary.RemainingNets)
	return remaining, summary
}

type physicalPadRoutingContext struct {
	resolver   PlacedPadEndpointResolver
	candidates []InterBlockRouteCandidate
	valid      bool
}

func newPhysicalPadRoutingContext(placed *PlacementStageResult) physicalPadRoutingContext {
	if placed == nil || placed.Stage.Status == StageStatusBlocked {
		return physicalPadRoutingContext{}
	}
	table, tableIssues := BuildGeneratedNetTable(placed, nil)
	if reports.HasBlockingIssue(tableIssues) {
		return physicalPadRoutingContext{}
	}
	resolver := NewPlacedPadEndpointResolver(placed, table)
	if reports.HasBlockingIssue(resolver.Issues()) {
		return physicalPadRoutingContext{}
	}
	return physicalPadRoutingContext{resolver: resolver, candidates: physicalPadRouteCandidatesFromResolver(resolver), valid: true}
}

func physicalPadRouteCandidates(placed *PlacementStageResult) []InterBlockRouteCandidate {
	return newPhysicalPadRoutingContext(placed).candidates
}

func physicalPadRouteCandidatesFromResolver(resolver PlacedPadEndpointResolver) []InterBlockRouteCandidate {
	byNet := map[string][]InterBlockRouteEndpoint{}
	seen := map[string]map[string]struct{}{}
	for _, endpoint := range resolver.Endpoints() {
		netName := strings.TrimSpace(endpoint.NetName)
		ref := strings.TrimSpace(endpoint.Ref)
		pad := strings.TrimSpace(endpoint.Pad)
		if netName == "" || ref == "" || pad == "" {
			continue
		}
		netKey := interBlockSummaryNetKey(netName)
		endpointKey := interBlockEndpointKey(ref, pad)
		if seen[netKey] == nil {
			seen[netKey] = map[string]struct{}{}
		}
		if _, exists := seen[netKey][endpointKey]; exists {
			continue
		}
		seen[netKey][endpointKey] = struct{}{}
		byNet[netKey] = append(byNet[netKey], InterBlockRouteEndpoint{Ref: ref, Pin: pad})
	}
	netNames := make([]string, 0, len(byNet))
	for netName, endpoints := range byNet {
		if len(endpoints) >= 2 {
			netNames = append(netNames, netName)
		}
	}
	slices.Sort(netNames)
	candidates := make([]InterBlockRouteCandidate, 0, len(netNames))
	for _, netName := range netNames {
		endpoints := byNet[netName]
		slices.SortFunc(endpoints, func(left, right InterBlockRouteEndpoint) int {
			if compare := strings.Compare(strings.ToUpper(left.Ref), strings.ToUpper(right.Ref)); compare != 0 {
				return compare
			}
			return strings.Compare(strings.ToUpper(left.Pin), strings.ToUpper(right.Pin))
		})
		candidates = append(candidates, InterBlockRouteCandidate{
			NetName:   netName,
			Status:    InterBlockRouteCandidateRoutable,
			Endpoints: endpoints,
		})
	}
	return candidates
}

func issueCountsByNet(issues []reports.Issue) map[string]int {
	counts := map[string]int{}
	for _, issue := range issues {
		for _, net := range issue.Nets {
			net = interBlockSummaryNetKey(net)
			if net != "" {
				counts[net]++
			}
		}
	}
	return counts
}

func blockingIssueCountsByNet(issues []reports.Issue) map[string]int {
	counts := map[string]int{}
	for _, issue := range issues {
		if !issue.Blocking() {
			continue
		}
		for _, net := range issue.Nets {
			net = interBlockSummaryNetKey(net)
			if net != "" {
				counts[net]++
			}
		}
	}
	return counts
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

func existingCopperFromRouteOperations(operations []transactions.Operation, defaultLayer string, rules routing.Rules) []routing.ExistingCopper {
	routes := decodedRouteOperations(operations, routeOperationBlocksInterBlockRouting)
	return existingCopperFromDecodedRoutes(routes, defaultLayer, rules)
}

func existingCopperFromAllRouteOperations(operations []transactions.Operation, defaultLayer string, rules routing.Rules) []routing.ExistingCopper {
	return existingCopperFromDecodedRoutes(decodedRouteOperations(operations, nil), defaultLayer, rules)
}

func decodedRouteOperations(operations []transactions.Operation, include func(*transactions.RouteOperation) bool) []transactions.RouteOperation {
	routes := make([]transactions.RouteOperation, 0, len(operations))
	for _, operation := range operations {
		if operation.Op != transactions.OpRoute {
			continue
		}
		var route transactions.RouteOperation
		if err := json.Unmarshal(operation.Raw, &route); err != nil {
			continue
		}
		if include != nil && !include(&route) {
			continue
		}
		routes = append(routes, route)
	}
	return routes
}

func existingCopperFromDecodedRoutes(routes []transactions.RouteOperation, defaultLayer string, rules routing.Rules) []routing.ExistingCopper {
	existing := []routing.ExistingCopper{}
	for _, route := range routes {
		layer := routeBranchCanonicalLayer(route.Layer, defaultLayer)
		for index := 1; index < len(route.Points); index++ {
			segment := routing.Segment{
				Net:     route.NetName,
				Layer:   layer,
				Start:   routing.Point{XMM: route.Points[index-1].XMM, YMM: route.Points[index-1].YMM},
				End:     routing.Point{XMM: route.Points[index].XMM, YMM: route.Points[index].YMM},
				WidthMM: route.WidthMM,
			}
			existing = append(existing, routing.ExistingCopper{
				Kind:       routing.CopperSegment,
				Net:        segment.Net,
				Layer:      layer,
				Geometry:   routeBranchSegmentShape(segment, rules),
				Centerline: []routing.Point{segment.Start, segment.End},
			})
		}
		for _, via := range route.Vias {
			routingVia := routing.Via{
				Net:        route.NetName,
				At:         routing.Point{XMM: via.At.XMM, YMM: via.At.YMM},
				DiameterMM: via.DiameterMM,
				DrillMM:    via.DrillMM,
				Layers:     append([]string(nil), via.Layers...),
			}
			shape := routeBranchViaShape(routingVia, rules)
			// Through-via copper blocks every participating layer, so emit one
			// obstacle per layer even when the track segment itself is single-sided.
			for _, viaLayer := range routingVia.Layers {
				existing = append(existing, routing.ExistingCopper{
					Kind:     routing.CopperVia,
					Net:      routingVia.Net,
					Layer:    routeBranchCanonicalLayer(viaLayer, defaultLayer),
					Geometry: shape,
				})
			}
		}
	}
	return existing
}

func existingUSBConfigurationCopperFromRouteOperations(operations []transactions.Operation, defaultLayer string, rules routing.Rules) []routing.ExistingCopper {
	routes := make([]transactions.RouteOperation, 0, len(operations))
	for _, operation := range operations {
		if operation.Op != transactions.OpRoute {
			continue
		}
		var route transactions.RouteOperation
		if err := json.Unmarshal(operation.Raw, &route); err != nil || !usbConfigurationChannelNet(route.NetName) {
			continue
		}
		routes = append(routes, route)
	}
	return existingCopperFromDecodedRoutes(routes, defaultLayer, rules)
}

func routeOperationBlocksInterBlockRouting(route *transactions.RouteOperation) bool {
	if route == nil {
		return false
	}
	switch netRoleFromName(route.NetName) {
	case placement.NetPower, placement.NetGround:
		return true
	case placement.NetSignal:
		// Via-bearing local signal routes are fixed cross-layer copper. Keeping
		// via-free signal traces out preserves existing routability while still
		// preventing route-tree branches from shorting through signal vias. USB-C
		// configuration-channel routes are also fixed electrical entry geometry.
		return len(route.Vias) != 0 || usbConfigurationChannelNet(route.NetName)
	default:
		return false
	}
}

func usbConfigurationChannelNet(netName string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(netName))
	return strings.HasSuffix(normalized, "_CC1") || strings.HasSuffix(normalized, "_CC2") || normalized == "CC1" || normalized == "CC2"
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

func bindLocalRouteOperations(fragments PCBFragmentResult, resolver PlacedPadEndpointResolver, board placement.BoardPlacementArea) ([]transactions.Operation, []reports.Issue, LocalRouteConnectivitySummary) {
	operations := []transactions.Operation{}
	var issues []reports.Issue
	summary := LocalRouteConnectivitySummary{}
	for _, fragment := range fragments.Fragments {
		unitContext := newTranslatedUnitRouteContext(fragment)
		for _, route := range fragment.Realization.LocalRoutes {
			summary.RoutesAttempted++
			routeOperations, routeIssues, routeSummary, ok := bindLocalRouteOperation(fragment, route, resolver, unitContext, board)
			issues = append(issues, routeIssues...)
			summary.RoutesBound += routeSummary.RoutesBound
			summary.EndpointsResolved += routeSummary.EndpointsResolved
			summary.EndpointsUnresolved += routeSummary.EndpointsUnresolved
			summary.EndpointContactsProven += routeSummary.EndpointContactsProven
			summary.EndpointNetMismatches += routeSummary.EndpointNetMismatches
			summary.EmittedTrackSegments += routeSummary.EmittedTrackSegments
			if ok {
				adjusted, adjustmentIssues, segmentDelta, adjustedOK := detourLocalRouteOperationsAroundExisting(routeOperations, operations, resolver)
				issues = append(issues, adjustmentIssues...)
				summary.EmittedTrackSegments += segmentDelta
				if adjustedOK {
					operations = append(operations, adjusted...)
				}
			}
		}
	}
	summary.IssueCount = len(issues)
	return operations, issues, summary
}

func detourLocalRouteOperationsAroundExisting(current []transactions.Operation, existing []transactions.Operation, resolver PlacedPadEndpointResolver) ([]transactions.Operation, []reports.Issue, int, bool) {
	if len(current) == 0 || len(existing) == 0 {
		return current, nil, 0, true
	}
	out := append([]transactions.Operation(nil), current...)
	committed := append([]transactions.Operation(nil), existing...)
	segmentDelta := 0
	for index, operation := range out {
		if operation.Op != transactions.OpRoute {
			committed = append(committed, operation)
			continue
		}
		var payload transactions.RouteOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			return current, []reports.Issue{localRouteCopperConflictIssue(index, operation.Net, "route operation could not be decoded: "+err.Error())}, 0, false
		}
		if len(payload.Points) >= 2 && strings.TrimSpace(payload.Layer) != "" {
			copper := localRouteForeignCopperObstacles(committed, payload.NetName, payload.Layer, math.Max(0, payload.WidthMM)/2)
			if !localRoutePolylineClearsForeignCopper(payload.Points, copper) {
				originalSegments := routeTrackSegmentCount(payload.Points)
				detoured, ok := detourLocalRoutePolyline(payload.Points, localRouteForeignCopperObstacleRects(copper))
				if !ok || !localRoutePolylineClearsForeignCopper(detoured, copper) || !localRoutePolylineClearsForeignPads(detoured, payload.Layer, payload.WidthMM, payload.NetName, PlacedPadEndpoint{}, PlacedPadEndpoint{}, resolver) {
					// Preserve authored copper when the small deterministic detour cannot
					// prove an improvement. Board validation and strict DRC remain the
					// fail-closed authority; this opportunistic composition pass must not
					// turn a conservative envelope overlap into a false routing blocker.
					committed = append(committed, operation)
					continue
				}
				payload.Points = detoured
				segmentDelta += routeTrackSegmentCount(detoured) - originalSegments
				raw, err := json.Marshal(payload)
				if err != nil {
					return current, []reports.Issue{localRouteCopperConflictIssue(index, payload.NetName, "detoured route operation could not be encoded: "+err.Error())}, 0, false
				}
				operation.Raw = raw
				operation.Net = strings.TrimSpace(payload.NetName)
				out[index] = operation
			}
		}
		viaObstacles := localRouteForeignCopperObstacles(committed, payload.NetName, payload.Layer, defaultLocalRouteViaDiameterMM/2)
		viaConflict := false
		for _, via := range payload.Vias {
			if localRouteViaAppliesToLayer(via, payload.Layer) && !localRoutePointClearsForeignCopper(via.At, viaObstacles) {
				viaConflict = true
				break
			}
		}
		if viaConflict {
			committed = append(committed, operation)
			continue
		}
		committed = append(committed, out[index])
	}
	return out, nil, segmentDelta, true
}

type localRouteCopperObstacle struct {
	ref    string
	start  transactions.Point
	end    transactions.Point
	radius float64
}

func localRouteForeignCopperObstacles(operations []transactions.Operation, currentNet string, layer string, movingRadiusMM float64) []localRouteCopperObstacle {
	clearanceMM := routing.DefaultRules().ClearanceMM
	layer = canonicalCopperLayer(layer)
	var obstacles []localRouteCopperObstacle
	for operationIndex, operation := range operations {
		if operation.Op != transactions.OpRoute {
			continue
		}
		var payload transactions.RouteOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil || strings.EqualFold(strings.TrimSpace(payload.NetName), strings.TrimSpace(currentNet)) {
			continue
		}
		if strings.EqualFold(canonicalCopperLayer(payload.Layer), layer) {
			existingRadiusMM := math.Max(0, payload.WidthMM) / 2
			for pointIndex := 1; pointIndex < len(payload.Points); pointIndex++ {
				start := payload.Points[pointIndex-1]
				end := payload.Points[pointIndex]
				inflateMM := existingRadiusMM + movingRadiusMM + clearanceMM
				obstacles = append(obstacles, localRouteCopperObstacle{
					ref:    fmt.Sprintf("local_route[%d].segment[%d]", operationIndex, pointIndex-1),
					start:  start,
					end:    end,
					radius: inflateMM,
				})
			}
		}
		for viaIndex, via := range payload.Vias {
			if !localRouteViaAppliesToLayer(via, layer) {
				continue
			}
			inflateMM := math.Max(0, via.DiameterMM)/2 + movingRadiusMM + clearanceMM
			obstacles = append(obstacles, localRouteCopperObstacle{
				ref:    fmt.Sprintf("local_route[%d].via[%d]", operationIndex, viaIndex),
				start:  via.At,
				end:    via.At,
				radius: inflateMM,
			})
		}
	}
	return obstacles
}

func localRouteForeignCopperRects(operations []transactions.Operation, currentNet string, layer string, movingRadiusMM float64) []localRoutePadRect {
	return localRouteForeignCopperObstacleRects(localRouteForeignCopperObstacles(operations, currentNet, layer, movingRadiusMM))
}

func localRouteForeignCopperObstacleRects(obstacles []localRouteCopperObstacle) []localRoutePadRect {
	rects := make([]localRoutePadRect, 0, len(obstacles))
	for _, obstacle := range obstacles {
		rects = append(rects, localRoutePadRect{
			ref:        obstacle.ref,
			center:     transactions.Point{XMM: (obstacle.start.XMM + obstacle.end.XMM) / 2, YMM: (obstacle.start.YMM + obstacle.end.YMM) / 2},
			halfWidth:  math.Abs(obstacle.end.XMM-obstacle.start.XMM)/2 + obstacle.radius,
			halfHeight: math.Abs(obstacle.end.YMM-obstacle.start.YMM)/2 + obstacle.radius,
		})
	}
	return rects
}

func localRoutePolylineClearsForeignCopper(points []transactions.Point, obstacles []localRouteCopperObstacle) bool {
	for pointIndex := 1; pointIndex < len(points); pointIndex++ {
		start := projectClearancePoint{x: points[pointIndex-1].XMM, y: points[pointIndex-1].YMM}
		end := projectClearancePoint{x: points[pointIndex].XMM, y: points[pointIndex].YMM}
		for _, obstacle := range obstacles {
			obstacleStart := projectClearancePoint{x: obstacle.start.XMM, y: obstacle.start.YMM}
			obstacleEnd := projectClearancePoint{x: obstacle.end.XMM, y: obstacle.end.YMM}
			if projectSegmentDistance(start, end, obstacleStart, obstacleEnd) < obstacle.radius-projectClearancePrecisionMM {
				return false
			}
		}
	}
	return true
}

func localRoutePointClearsForeignCopper(point transactions.Point, obstacles []localRouteCopperObstacle) bool {
	return localRoutePolylineClearsForeignCopper([]transactions.Point{point, point}, obstacles)
}

func localRouteViaAppliesToLayer(via transactions.RouteViaSpec, layer string) bool {
	layer = canonicalCopperLayer(layer)
	for _, candidate := range via.Layers {
		if strings.EqualFold(canonicalCopperLayer(candidate), layer) {
			return true
		}
	}
	return false
}

func localRouteCopperConflictIssue(index int, netName string, message string) reports.Issue {
	return reports.Issue{
		Code:       reports.CodeValidationFailed,
		Severity:   reports.SeverityBlocked,
		Path:       fmt.Sprintf("routing.local_routes[%d]", index),
		Message:    message,
		Nets:       compactContactStrings([]string{strings.TrimSpace(netName)}),
		Suggestion: "allow the bounded local-route detour more board space or move the connected components",
	}
}

func bindLocalRouteOperation(fragment BlockFragment, route blocks.RealizedPCBLocalRoute, resolver PlacedPadEndpointResolver, unitContext translatedUnitRouteContext, board placement.BoardPlacementArea) ([]transactions.Operation, []reports.Issue, LocalRouteConnectivitySummary, bool) {
	var issues []reports.Issue
	summary := LocalRouteConnectivitySummary{RoutesAttempted: 1}
	netName := strings.TrimSpace(route.NetName)
	routePath := "routes." + firstNonEmpty(fragment.InstanceID, fragment.BlockID, "fragment") + "." + firstNonEmpty(route.ID, netName, "unnamed")
	if netName == "" {
		summary.EndpointsUnresolved = 2
		summary.IssueCount = 1
		return nil, []reports.Issue{localRouteBindingIssue(routePath+".net_name", "local route net name is required", nil)}, summary, false
	}
	from, fromIssues, fromOK, fromNetMismatch := resolveLocalRouteEndpoint(fragment, routePath+".from", netName, route.WidthMM, route.From, resolver, board)
	to, toIssues, toOK, toNetMismatch := resolveLocalRouteEndpoint(fragment, routePath+".to", netName, route.WidthMM, route.To, resolver, board)
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
		return nil, issues, summary, false
	}
	if sameRoutePoint(from.Point, to.Point) && (localRouteEndpointIsEntryAnchor(from) || localRouteEndpointIsEntryAnchor(to)) && route.EntryAnchorDogbone == nil && !route.FromEndpointDogbone && !route.ToEndpointDogbone {
		summary.RoutesBound = 1
		summary.EndpointContactsProven = 2
		summary.IssueCount = len(issues)
		return nil, issues, summary, true
	}
	layer := firstNonEmpty(route.Layer, from.Layer, to.Layer, "F.Cu")
	points := []transactions.Point{
		from.Point,
		to.Point,
	}
	preservedTransform := false
	if routedPoints, ok := translatedUnitLocalRoutePoints(unitContext, route, from, to); ok {
		points = routedPoints
		preservedTransform = true
	} else if routedPoints, ok := placedLocalRoutePoints(route.Points, from.Point, to.Point); ok {
		points = routedPoints
	}
	if detoured, changed, ok := padClearLocalRouteEndpointSiblings(points, layer, route.WidthMM, netName, from, to, resolver); ok {
		points = detoured
		if changed {
			preservedTransform = false
		}
	}
	if detoured, changed, ok := padClearLocalRouteForeignPads(points, layer, route.WidthMM, netName, from, to, resolver); ok {
		points = detoured
		if changed {
			preservedTransform = false
		}
	}
	if detoured, changed, ok := padClearDirectLocalRoute(points, layer, route.WidthMM, netName, from, to, resolver); ok {
		points = detoured
		if changed {
			preservedTransform = false
		}
	} else {
		// This failure is intentionally terminal: retaining the original direct
		// segment would emit a known foreign-pad clearance violation, and the
		// downstream router does not rewrite authored block-local copper.
		issues = append(issues, localRouteBindingIssue(routePath, "direct local route crosses foreign pad clearance and has no clear orthogonal access path", []string{from.Ref, to.Ref}))
		summary.IssueCount = len(issues)
		return nil, issues, summary, false
	}
	fromEndpointDogbone := route.FromEndpointDogbone
	toEndpointDogbone := route.ToEndpointDogbone
	if !strings.EqualFold(canonicalCopperLayer(layer), canonicalCopperLayer(from.Layer)) && !localRouteEndpointViaClearsForeignPads(from, layer, route.WidthMM, netName, from, to, resolver) {
		fromEndpointDogbone = true
	}
	if !strings.EqualFold(canonicalCopperLayer(layer), canonicalCopperLayer(to.Layer)) && !localRouteEndpointViaClearsForeignPads(to, layer, route.WidthMM, netName, from, to, resolver) {
		toEndpointDogbone = true
	}
	mainRoutePoints := append([]transactions.Point(nil), points...)
	fromDogbonePoints := []transactions.Point(nil)
	toDogbonePoints := []transactions.Point(nil)
	if fromEndpointDogbone {
		if len(mainRoutePoints) < 3 || sameRoutePoint(mainRoutePoints[1], from.Point) {
			issues = append(issues, localRouteBindingIssue(routePath+".from_endpoint_dogbone", "source endpoint dogbone requires a distinct first waypoint", []string{from.Ref}))
			summary.IssueCount = len(issues)
			return nil, issues, summary, false
		}
		if strings.EqualFold(canonicalCopperLayer(layer), canonicalCopperLayer(from.Layer)) {
			issues = append(issues, localRouteBindingIssue(routePath+".from_endpoint_dogbone", "source endpoint dogbone requires a route layer different from the source pad layer", []string{from.Ref}))
			summary.IssueCount = len(issues)
			return nil, issues, summary, false
		}
		var changed bool
		var ok bool
		mainRoutePoints, fromDogbonePoints, changed, ok = padClearLocalRouteEndpointDogbone(mainRoutePoints, true, layer, route.WidthMM, netName, from, to, resolver, board)
		if !ok {
			issues = append(issues, localRouteBindingIssue(routePath+".from_endpoint_dogbone", "source endpoint dogbone has no bounded via-clear transition", []string{from.Ref}))
			summary.IssueCount = len(issues)
			return nil, issues, summary, false
		}
		if changed {
			preservedTransform = false
		}
	}
	if toEndpointDogbone {
		if len(mainRoutePoints) < 2 || sameRoutePoint(mainRoutePoints[len(mainRoutePoints)-2], to.Point) {
			issues = append(issues, localRouteBindingIssue(routePath+".to_endpoint_dogbone", "destination endpoint dogbone requires a distinct final waypoint", []string{to.Ref}))
			summary.IssueCount = len(issues)
			return nil, issues, summary, false
		}
		if strings.EqualFold(canonicalCopperLayer(layer), canonicalCopperLayer(to.Layer)) {
			issues = append(issues, localRouteBindingIssue(routePath+".to_endpoint_dogbone", "destination endpoint dogbone requires a route layer different from the destination pad layer", []string{to.Ref}))
			summary.IssueCount = len(issues)
			return nil, issues, summary, false
		}
		var changed bool
		var ok bool
		mainRoutePoints, toDogbonePoints, changed, ok = padClearLocalRouteEndpointDogbone(mainRoutePoints, false, layer, route.WidthMM, netName, from, to, resolver, board)
		if !ok {
			issues = append(issues, localRouteBindingIssue(routePath+".to_endpoint_dogbone", "destination endpoint dogbone has no bounded via-clear transition", []string{to.Ref}))
			summary.IssueCount = len(issues)
			return nil, issues, summary, false
		}
		if changed {
			preservedTransform = false
		}
	}
	if (fromEndpointDogbone || toEndpointDogbone) && (len(mainRoutePoints) < 2 || sameRoutePoint(mainRoutePoints[0], mainRoutePoints[len(mainRoutePoints)-1])) {
		issues = append(issues, localRouteBindingIssue(routePath, "endpoint dogbones require distinct source and destination transitions", []string{from.Ref, to.Ref}))
		summary.IssueCount = len(issues)
		return nil, issues, summary, false
	}
	vias, viaOK := localRouteEndpointVias(layer, from, to, fromEndpointDogbone, toEndpointDogbone)
	if !viaOK {
		issues = append(issues, localRouteBindingIssue(routePath+".layer", "local route layer "+layer+" does not match endpoint layers "+from.Layer+" and "+to.Layer, []string{from.Ref, to.Ref}))
		summary.IssueCount = len(issues)
		return nil, issues, summary, false
	}
	if len(fromDogbonePoints) > 0 {
		vias = append(vias, localRouteEndpointVia(fromDogbonePoints[len(fromDogbonePoints)-1], from.Layer, layer))
	}
	if len(toDogbonePoints) > 0 {
		vias = append(vias, localRouteEndpointVia(toDogbonePoints[0], to.Layer, layer))
	}
	if !route.DisableEntryAnchorVia {
		vias = append(vias, localRouteEntryAnchorVias(layer, from, to, vias)...)
	}
	trackSegments := routeTrackSegmentCount(mainRoutePoints) + routeTrackSegmentCount(fromDogbonePoints) + routeTrackSegmentCount(toDogbonePoints)
	if trackSegments == 0 {
		mainRoutePoints = nil
	}
	operations := []transactions.Operation{}
	if len(mainRoutePoints) > 0 || len(vias) > 0 {
		operation, err := workflowOperation(transactions.OpRoute, transactions.RouteOperation{
			Op:      transactions.OpRoute,
			NetName: netName,
			Layer:   layer,
			WidthMM: route.WidthMM,
			Points:  mainRoutePoints,
			Vias:    vias,
		})
		if err != nil {
			issues = append(issues, localRouteBindingIssue(routePath, err.Error(), []string{from.Ref, to.Ref}))
			summary.IssueCount = len(issues)
			return nil, issues, summary, false
		}
		operation.Rebuildable = !preservedTransform && route.EntryAnchorDogbone == nil && !fromEndpointDogbone && !toEndpointDogbone
		if operation.Rebuildable {
			operation.RebuildRefs = compactContactStrings([]string{from.Ref, to.Ref})
			operation.RebuildSourceLayers = localRouteEndpointCopperLayers(from, layer)
			operation.RebuildTargetLayers = localRouteEndpointCopperLayers(to, layer)
		}
		operations = append(operations, operation)
	}
	for _, dogbone := range []struct {
		path   string
		ref    string
		layer  string
		points []transactions.Point
	}{
		{path: ".from_endpoint_dogbone", ref: from.Ref, layer: from.Layer, points: fromDogbonePoints},
		{path: ".to_endpoint_dogbone", ref: to.Ref, layer: to.Layer, points: toDogbonePoints},
	} {
		if len(dogbone.points) == 0 {
			continue
		}
		operation, err := workflowOperation(transactions.OpRoute, transactions.RouteOperation{
			Op:      transactions.OpRoute,
			NetName: netName,
			Layer:   canonicalCopperLayer(dogbone.layer),
			WidthMM: route.WidthMM,
			Points:  dogbone.points,
		})
		if err != nil {
			issues = append(issues, localRouteBindingIssue(routePath+dogbone.path, err.Error(), []string{dogbone.ref}))
			summary.IssueCount = len(issues)
			return nil, issues, summary, false
		}
		operation.Ref = localRouteEndpointDogboneOperationRef
		operations = append(operations, operation)
	}
	dogbones, dogboneIssues := localRouteEntryAnchorDogboneOperations(routePath, netName, layer, route.WidthMM, points, from, to, vias, route.EntryAnchorDogbone)
	issues = append(issues, dogboneIssues...)
	operations = append(operations, dogbones...)
	summary.RoutesBound = 1
	summary.EndpointContactsProven = 2
	summary.EmittedTrackSegments = trackSegments + len(dogbones)
	summary.IssueCount = len(issues)
	return operations, issues, summary, true
}

func padClearLocalRouteEndpointSiblings(points []transactions.Point, layer string, widthMM float64, netName string, from PlacedPadEndpoint, to PlacedPadEndpoint, resolver PlacedPadEndpointResolver) ([]transactions.Point, bool, bool) {
	if len(points) < 2 {
		return points, false, true
	}
	endpointRefs := map[string]struct{}{}
	for _, endpoint := range []PlacedPadEndpoint{from, to} {
		if !localRouteEndpointIsEntryAnchor(endpoint) && strings.TrimSpace(endpoint.Ref) != "" {
			endpointRefs[strings.ToUpper(strings.TrimSpace(endpoint.Ref))] = struct{}{}
		}
	}
	if len(endpointRefs) == 0 {
		return points, false, true
	}
	all := localRouteForeignPadRects(points, layer, widthMM, netName, from, to, resolver)
	siblings := make([]localRoutePadRect, 0, len(all))
	for _, obstacle := range all {
		if _, ok := endpointRefs[strings.ToUpper(strings.TrimSpace(obstacle.ref))]; ok {
			siblings = append(siblings, obstacle)
		}
	}
	if len(siblings) == 0 || localRoutePolylineClearsPadRects(points, siblings) {
		return points, false, true
	}
	detoured, ok := detourLocalRoutePolyline(points, siblings)
	return detoured, ok, ok
}

func detourLocalRoutePolyline(points []transactions.Point, obstacles []localRoutePadRect) ([]transactions.Point, bool) {
	laneMarginMM := routing.DefaultRules().GridMM
	result, ok := relocateLocalRouteInteriorPoints(points, obstacles, laneMarginMM)
	if !ok {
		return points, false
	}
	rewriteBudget := max(1, len(obstacles)*(len(result)+1)*4)
	rewrites := 0
	for index := 1; index < len(result); {
		start := result[index-1]
		end := result[index]
		var blocker *localRoutePadRect
		for obstacleIndex := range obstacles {
			obstacle := &obstacles[obstacleIndex]
			if segmentIntersectsLocalRouteRect(start, end, obstacle.center, obstacle.halfWidth, obstacle.halfHeight) {
				blocker = obstacle
				break
			}
		}
		if blocker == nil {
			index++
			continue
		}
		candidates := [][]transactions.Point{
			compactRoutePoints([]transactions.Point{start, {XMM: start.XMM, YMM: end.YMM}, end}),
			compactRoutePoints([]transactions.Point{start, {XMM: end.XMM, YMM: start.YMM}, end}),
		}
		for _, x := range []float64{blocker.center.XMM - blocker.halfWidth - laneMarginMM, blocker.center.XMM + blocker.halfWidth + laneMarginMM} {
			candidates = append(candidates, compactRoutePoints([]transactions.Point{start, {XMM: x, YMM: start.YMM}, {XMM: x, YMM: end.YMM}, end}))
		}
		for _, y := range []float64{blocker.center.YMM - blocker.halfHeight - laneMarginMM, blocker.center.YMM + blocker.halfHeight + laneMarginMM} {
			candidates = append(candidates, compactRoutePoints([]transactions.Point{start, {XMM: start.XMM, YMM: y}, {XMM: end.XMM, YMM: y}, end}))
		}
		selected := []transactions.Point(nil)
		for _, candidate := range candidates {
			if localRoutePolylineClearsPadRects(candidate, obstacles) {
				selected = candidate
				break
			}
		}
		if len(selected) == 0 {
			return points, false
		}
		if rewrites >= rewriteBudget {
			return points, false
		}
		rewrites++
		replacement := make([]transactions.Point, 0, len(result)+len(selected)-2)
		replacement = append(replacement, result[:index-1]...)
		replacement = append(replacement, selected...)
		replacement = append(replacement, result[index+1:]...)
		result = compactRoutePoints(replacement)
		// Revalidate from the segment before the replacement; compaction may
		// remove collinear points, so no arithmetic based on selected length is
		// safe here.
		index = max(1, index-1)
	}
	return result, true
}

const localRouteDogboneSearchMaxRings = 32

// padClearLocalRouteEndpointDogbone gives an endpoint-layer dogbone a
// via-clear transition without discarding its authored main-layer waypoint.
// The original transition remains as the next main-route point when a move is
// required, so the correction is local, deterministic, and topology-neutral.
func padClearLocalRouteEndpointDogbone(points []transactions.Point, fromEnd bool, routeLayer string, widthMM float64, netName string, from PlacedPadEndpoint, to PlacedPadEndpoint, resolver PlacedPadEndpointResolver, board placement.BoardPlacementArea) ([]transactions.Point, []transactions.Point, bool, bool) {
	if len(points) < 2 {
		return points, nil, false, false
	}
	endpoint := to
	authoredTransition := points[len(points)-2]
	if fromEnd {
		endpoint = from
		authoredTransition = points[1]
	}
	candidates := localRouteDogboneTransitionCandidates(authoredTransition, endpoint, widthMM, netName, from, to, resolver)
	for _, candidate := range candidates {
		if sameRoutePoint(candidate, endpoint.Point) || !localRouteDogboneTransitionInsideBoard(candidate, board) {
			continue
		}
		viaWidthMM := math.Max(widthMM, defaultLocalRouteViaDiameterMM)
		viaProbe := []transactions.Point{endpoint.Point, candidate}
		viaObstacles := localRouteForeignPadRects(viaProbe, endpoint.Layer, viaWidthMM, netName, from, to, resolver)
		if !localRoutePointClearsPadRects(candidate, viaObstacles) {
			continue
		}

		dogbone := []transactions.Point{candidate, endpoint.Point}
		if fromEnd {
			dogbone[0], dogbone[1] = dogbone[1], dogbone[0]
		}
		dogboneObstacles := localRouteForeignPadRects(dogbone, endpoint.Layer, widthMM, netName, from, to, resolver)
		if !localRoutePolylineClearsPadRects(dogbone, dogboneObstacles) {
			var ok bool
			dogbone, ok = detourLocalRoutePolyline(dogbone, dogboneObstacles)
			if !ok || !localRoutePolylineClearsPadRects(dogbone, dogboneObstacles) {
				continue
			}
		}

		main := make([]transactions.Point, 0, len(points)+1)
		if fromEnd {
			main = append(main, candidate)
			main = append(main, points[1:]...)
		} else {
			main = append(main, points[:len(points)-1]...)
			main = append(main, candidate)
		}
		main = compactRoutePoints(main)
		mainObstacles := localRouteForeignPadRects(main, routeLayer, widthMM, netName, from, to, resolver)
		if !localRoutePolylineClearsPadRects(main, mainObstacles) {
			var ok bool
			main, ok = detourLocalRoutePolyline(main, mainObstacles)
			if !ok || !localRoutePolylineClearsPadRects(main, mainObstacles) {
				continue
			}
		}
		changed := !sameRoutePoint(candidate, authoredTransition) || len(dogbone) != 2 || len(main) != len(points)-1
		return main, dogbone, changed, true
	}
	return points, nil, false, false
}

func localRouteDogboneTransitionCandidates(authored transactions.Point, endpoint PlacedPadEndpoint, widthMM float64, netName string, from PlacedPadEndpoint, to PlacedPadEndpoint, resolver PlacedPadEndpointResolver) []transactions.Point {
	viaWidthMM := math.Max(widthMM, defaultLocalRouteViaDiameterMM)
	probe := []transactions.Point{endpoint.Point, authored}
	obstacles := localRouteForeignPadRects(probe, endpoint.Layer, viaWidthMM, netName, from, to, resolver)
	gridMM := routing.DefaultRules().GridMM
	if gridMM <= 0 {
		gridMM = 0.25
	}
	candidates := []transactions.Point{authored}
	for _, obstacle := range obstacles {
		if localRoutePointClearsPadRects(authored, []localRoutePadRect{obstacle}) {
			continue
		}
		candidates = append(candidates,
			transactions.Point{XMM: obstacle.center.XMM - obstacle.halfWidth - gridMM, YMM: authored.YMM},
			transactions.Point{XMM: obstacle.center.XMM + obstacle.halfWidth + gridMM, YMM: authored.YMM},
			transactions.Point{XMM: authored.XMM, YMM: obstacle.center.YMM - obstacle.halfHeight - gridMM},
			transactions.Point{XMM: authored.XMM, YMM: obstacle.center.YMM + obstacle.halfHeight + gridMM},
		)
	}
	geometryRadiusMM := resolver.MaximumPadRadiusMM() + viaWidthMM/2 + routing.DefaultRules().ClearanceMM + gridMM
	rings := min(localRouteDogboneSearchMaxRings, max(1, int(math.Ceil(geometryRadiusMM/gridMM))))
	for ring := 1; ring <= rings; ring++ {
		for dx := -ring; dx <= ring; dx++ {
			for _, dy := range []int{-ring, ring} {
				candidates = append(candidates, transactions.Point{XMM: authored.XMM + float64(dx)*gridMM, YMM: authored.YMM + float64(dy)*gridMM})
			}
		}
		for dy := -ring + 1; dy < ring; dy++ {
			for _, dx := range []int{-ring, ring} {
				candidates = append(candidates, transactions.Point{XMM: authored.XMM + float64(dx)*gridMM, YMM: authored.YMM + float64(dy)*gridMM})
			}
		}
	}
	sort.SliceStable(candidates[1:], func(i, j int) bool {
		left := candidates[i+1]
		right := candidates[j+1]
		leftDistance := math.Hypot(left.XMM-authored.XMM, left.YMM-authored.YMM)
		rightDistance := math.Hypot(right.XMM-authored.XMM, right.YMM-authored.YMM)
		if math.Abs(leftDistance-rightDistance) > interBlockContactToleranceMM {
			return leftDistance < rightDistance
		}
		if left.XMM != right.XMM {
			return left.XMM < right.XMM
		}
		return left.YMM < right.YMM
	})
	unique := make([]transactions.Point, 0, len(candidates))
	seen := map[routeCoordKey]struct{}{}
	for _, candidate := range candidates {
		key := routeCoordKey{x: int64(math.Round(candidate.XMM * 1e6)), y: int64(math.Round(candidate.YMM * 1e6))}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, candidate)
	}
	return unique
}

func localRouteDogboneTransitionInsideBoard(point transactions.Point, board placement.BoardPlacementArea) bool {
	if board.WidthMM <= 0 || board.HeightMM <= 0 {
		return true
	}
	marginMM := math.Max(0, board.MarginMM) + defaultLocalRouteViaDiameterMM/2
	return point.XMM >= board.Origin.XMM+marginMM && point.XMM <= board.Origin.XMM+board.WidthMM-marginMM &&
		point.YMM >= board.Origin.YMM+marginMM && point.YMM <= board.Origin.YMM+board.HeightMM-marginMM
}

func relocateLocalRouteInteriorPoints(points []transactions.Point, obstacles []localRoutePadRect, marginMM float64) ([]transactions.Point, bool) {
	result := append([]transactions.Point(nil), points...)
	for index := 1; index+1 < len(result); index++ {
		point := result[index]
		if localRoutePointClearsPadRects(point, obstacles) {
			continue
		}
		candidates := make([]transactions.Point, 0, len(obstacles)*4)
		for _, obstacle := range obstacles {
			if segmentIntersectsLocalRouteRect(point, point, obstacle.center, obstacle.halfWidth, obstacle.halfHeight) {
				candidates = append(candidates,
					transactions.Point{XMM: obstacle.center.XMM - obstacle.halfWidth - marginMM, YMM: point.YMM},
					transactions.Point{XMM: obstacle.center.XMM + obstacle.halfWidth + marginMM, YMM: point.YMM},
					transactions.Point{XMM: point.XMM, YMM: obstacle.center.YMM - obstacle.halfHeight - marginMM},
					transactions.Point{XMM: point.XMM, YMM: obstacle.center.YMM + obstacle.halfHeight + marginMM},
				)
			}
		}
		selected := transactions.Point{}
		selectedDistance := math.Inf(1)
		found := false
		for _, candidate := range candidates {
			if !localRoutePointClearsPadRects(candidate, obstacles) {
				continue
			}
			dx := candidate.XMM - point.XMM
			dy := candidate.YMM - point.YMM
			distance := dx*dx + dy*dy
			if !found || distance < selectedDistance {
				selected = candidate
				selectedDistance = distance
				found = true
			}
		}
		if !found {
			return points, false
		}
		result[index] = selected
	}
	return compactRoutePoints(result), true
}

func localRoutePointClearsPadRects(point transactions.Point, obstacles []localRoutePadRect) bool {
	for _, obstacle := range obstacles {
		if segmentIntersectsLocalRouteRect(point, point, obstacle.center, obstacle.halfWidth, obstacle.halfHeight) {
			return false
		}
	}
	return true
}

func padClearLocalRouteForeignPads(points []transactions.Point, layer string, widthMM float64, netName string, from PlacedPadEndpoint, to PlacedPadEndpoint, resolver PlacedPadEndpointResolver) ([]transactions.Point, bool, bool) {
	if len(points) < 2 {
		return points, false, true
	}
	obstacles := localRouteForeignPadRects(points, layer, widthMM, netName, from, to, resolver)
	if len(obstacles) == 0 || localRoutePolylineClearsPadRects(points, obstacles) {
		return points, false, true
	}
	detoured, ok := detourLocalRoutePolyline(points, obstacles)
	return detoured, ok, ok
}

func padClearDirectLocalRoute(points []transactions.Point, layer string, widthMM float64, netName string, from PlacedPadEndpoint, to PlacedPadEndpoint, resolver PlacedPadEndpointResolver) ([]transactions.Point, bool, bool) {
	if len(points) != 2 || (!localRouteEndpointIsEntryAnchor(from) && !localRouteEndpointIsEntryAnchor(to)) || localRoutePolylineClearsForeignPads(points, layer, widthMM, netName, from, to, resolver) {
		return points, false, true
	}
	candidates := [][]transactions.Point{
		compactRoutePoints([]transactions.Point{points[0], {XMM: points[0].XMM, YMM: points[1].YMM}, points[1]}),
		compactRoutePoints([]transactions.Point{points[0], {XMM: points[1].XMM, YMM: points[0].YMM}, points[1]}),
	}
	laneMarginMM := routing.DefaultRules().GridMM
	for _, obstacle := range localRouteForeignPadRects(points, layer, widthMM, netName, from, to, resolver) {
		if !segmentIntersectsLocalRouteRect(points[0], points[1], obstacle.center, obstacle.halfWidth, obstacle.halfHeight) {
			continue
		}
		for _, x := range []float64{obstacle.center.XMM - obstacle.halfWidth - laneMarginMM, obstacle.center.XMM + obstacle.halfWidth + laneMarginMM} {
			candidates = append(candidates, compactRoutePoints([]transactions.Point{points[0], {XMM: x, YMM: points[0].YMM}, {XMM: x, YMM: points[1].YMM}, points[1]}))
		}
		for _, y := range []float64{obstacle.center.YMM - obstacle.halfHeight - laneMarginMM, obstacle.center.YMM + obstacle.halfHeight + laneMarginMM} {
			candidates = append(candidates, compactRoutePoints([]transactions.Point{points[0], {XMM: points[0].XMM, YMM: y}, {XMM: points[1].XMM, YMM: y}, points[1]}))
		}
	}
	for _, candidate := range candidates {
		if localRoutePolylineClearsForeignPads(candidate, layer, widthMM, netName, from, to, resolver) {
			return candidate, true, true
		}
	}
	return points, false, false
}

type localRoutePadRect struct {
	ref        string
	center     transactions.Point
	halfWidth  float64
	halfHeight float64
}

func localRouteForeignPadRects(points []transactions.Point, layer string, widthMM float64, netName string, from PlacedPadEndpoint, to PlacedPadEndpoint, resolver PlacedPadEndpointResolver) []localRoutePadRect {
	endpointKeys := map[routeEndpointMapKey]struct{}{
		routeEndpointKey(from.Ref, from.Pad): {},
		routeEndpointKey(to.Ref, to.Pad):     {},
	}
	clearanceMM := routing.DefaultRules().ClearanceMM
	pads := resolver.Endpoints()
	if len(points) != 0 {
		minX, minY := points[0].XMM, points[0].YMM
		maxX, maxY := minX, minY
		for _, point := range points[1:] {
			minX = math.Min(minX, point.XMM)
			minY = math.Min(minY, point.YMM)
			maxX = math.Max(maxX, point.XMM)
			maxY = math.Max(maxY, point.YMM)
		}
		padding := resolver.MaximumPadRadiusMM() + math.Max(0, widthMM)/2 + clearanceMM
		pads = resolver.EndpointsWithinBounds(minX-padding, minY-padding, maxX+padding, maxY+padding)
	}
	rects := make([]localRoutePadRect, 0, len(pads))
	rotationTrig := map[float64][2]float64{}
	for _, pad := range pads {
		if _, endpoint := endpointKeys[routeEndpointKey(pad.Ref, pad.Pad)]; endpoint {
			continue
		}
		if strings.TrimSpace(netName) != "" && strings.EqualFold(strings.TrimSpace(pad.NetName), strings.TrimSpace(netName)) {
			continue
		}
		if !localRoutePadAppliesToLayer(pad, layer) {
			continue
		}
		trig, ok := rotationTrig[pad.ComponentRotation]
		if !ok {
			angleRadians := pad.ComponentRotation * math.Pi / 180
			trig = [2]float64{math.Abs(math.Cos(angleRadians)), math.Abs(math.Sin(angleRadians))}
			rotationTrig[pad.ComponentRotation] = trig
		}
		cosine, sine := trig[0], trig[1]
		padWidthMM := math.Max(0, pad.PadWidthMM)
		padHeightMM := math.Max(0, pad.PadHeightMM)
		rotatedWidthMM := cosine*padWidthMM + sine*padHeightMM
		rotatedHeightMM := sine*padWidthMM + cosine*padHeightMM
		rects = append(rects, localRoutePadRect{
			ref:        pad.Ref,
			center:     pad.Point,
			halfWidth:  rotatedWidthMM/2 + math.Max(0, widthMM)/2 + clearanceMM,
			halfHeight: rotatedHeightMM/2 + math.Max(0, widthMM)/2 + clearanceMM,
		})
	}
	return rects
}

func localRoutePolylineClearsForeignPads(points []transactions.Point, layer string, widthMM float64, netName string, from PlacedPadEndpoint, to PlacedPadEndpoint, resolver PlacedPadEndpointResolver) bool {
	return localRoutePolylineClearsPadRects(points, localRouteForeignPadRects(points, layer, widthMM, netName, from, to, resolver))
}

func localRoutePolylineClearsPadRects(points []transactions.Point, obstacles []localRoutePadRect) bool {
	for _, obstacle := range obstacles {
		for index := 1; index < len(points); index++ {
			if segmentIntersectsLocalRouteRect(points[index-1], points[index], obstacle.center, obstacle.halfWidth, obstacle.halfHeight) {
				return false
			}
		}
	}
	return true
}

func localRoutePadAppliesToLayer(pad PlacedPadEndpoint, layer string) bool {
	layer = canonicalCopperLayer(layer)
	layers := append([]string(nil), pad.Layers...)
	if len(layers) == 0 {
		layers = []string{pad.Layer}
	}
	for _, candidate := range layers {
		candidate = canonicalCopperLayer(candidate)
		if strings.EqualFold(candidate, "*.Cu") || strings.EqualFold(candidate, layer) {
			return true
		}
	}
	return false
}

func segmentIntersectsLocalRouteRect(start transactions.Point, end transactions.Point, center transactions.Point, halfWidth float64, halfHeight float64) bool {
	minX := center.XMM - halfWidth
	maxX := center.XMM + halfWidth
	minY := center.YMM - halfHeight
	maxY := center.YMM + halfHeight
	tMin := 0.0
	tMax := 1.0
	for _, axis := range []struct {
		start float64
		delta float64
		min   float64
		max   float64
	}{
		{start: start.XMM, delta: end.XMM - start.XMM, min: minX, max: maxX},
		{start: start.YMM, delta: end.YMM - start.YMM, min: minY, max: maxY},
	} {
		if math.Abs(axis.delta) <= interBlockContactToleranceMM {
			if axis.start < axis.min || axis.start > axis.max {
				return false
			}
			continue
		}
		near := (axis.min - axis.start) / axis.delta
		far := (axis.max - axis.start) / axis.delta
		if near > far {
			near, far = far, near
		}
		tMin = math.Max(tMin, near)
		tMax = math.Min(tMax, far)
		if tMin > tMax {
			return false
		}
	}
	return true
}

func localRouteEndpointCopperLayers(endpoint PlacedPadEndpoint, fallback string) []string {
	layers := endpoint.Layers
	if len(layers) == 0 {
		layers = []string{firstNonEmpty(endpoint.Layer, fallback)}
	}
	return orderedLocalRouteRebuildLayers(layers, fallback, fallback)
}

func localRouteEndpointViaClearsForeignPads(endpoint PlacedPadEndpoint, routeLayer string, widthMM float64, netName string, from PlacedPadEndpoint, to PlacedPadEndpoint, resolver PlacedPadEndpointResolver) bool {
	if strings.EqualFold(canonicalCopperLayer(routeLayer), canonicalCopperLayer(endpoint.Layer)) {
		return true
	}
	probe := []transactions.Point{endpoint.Point, endpoint.Point}
	viaWidthMM := math.Max(widthMM, defaultLocalRouteViaDiameterMM)
	obstacles := localRouteForeignPadRects(probe, endpoint.Layer, viaWidthMM, netName, from, to, resolver)
	return localRoutePointClearsPadRects(endpoint.Point, obstacles)
}

func routeTrackSegmentCount(points []transactions.Point) int {
	count := 0
	for index := 1; index < len(points); index++ {
		if !sameRoutePoint(points[index-1], points[index]) {
			count++
		}
	}
	return count
}

func localRouteEndpointVias(layer string, from PlacedPadEndpoint, to PlacedPadEndpoint, skipFrom bool, skipTo bool) ([]transactions.RouteViaSpec, bool) {
	routeLayer := canonicalCopperLayer(layer)
	fromLayer := canonicalCopperLayer(from.Layer)
	toLayer := canonicalCopperLayer(to.Layer)
	if !localRouteCopperLayer(routeLayer) || !localRouteCopperLayer(fromLayer) || !localRouteCopperLayer(toLayer) {
		return nil, false
	}
	var vias []transactions.RouteViaSpec
	if !skipFrom && !strings.EqualFold(routeLayer, fromLayer) {
		vias = append(vias, localRouteEndpointVia(from.Point, fromLayer, routeLayer))
	}
	if !skipTo && !strings.EqualFold(routeLayer, toLayer) && !sameRoutePoint(from.Point, to.Point) {
		vias = append(vias, localRouteEndpointVia(to.Point, toLayer, routeLayer))
	}
	return vias, true
}

func localRouteCopperLayer(layer string) bool {
	return strings.HasSuffix(strings.ToUpper(strings.TrimSpace(layer)), ".CU")
}

const (
	defaultLocalRouteViaDiameterMM = 0.6
	defaultLocalRouteViaDrillMM    = 0.3
)

func localRouteEndpointVia(point transactions.Point, endpointLayer string, routeLayer string) transactions.RouteViaSpec {
	return transactions.RouteViaSpec{
		At:         point,
		DiameterMM: defaultLocalRouteViaDiameterMM,
		DrillMM:    defaultLocalRouteViaDrillMM,
		Layers:     []string{canonicalCopperLayer(endpointLayer), canonicalCopperLayer(routeLayer)},
	}
}

func localRouteEntryAnchorVias(routeLayer string, from PlacedPadEndpoint, to PlacedPadEndpoint, existing []transactions.RouteViaSpec) []transactions.RouteViaSpec {
	var vias []transactions.RouteViaSpec
	for _, endpoint := range []PlacedPadEndpoint{from, to} {
		if !localRouteEndpointIsEntryAnchor(endpoint) || localRouteHasViaAt(existing, endpoint.Point) || localRouteHasViaAt(vias, endpoint.Point) {
			continue
		}
		vias = append(vias, localRouteMaterializedAnchorVia(endpoint.Point, routeLayer))
	}
	return vias
}

func localRouteEntryAnchorDogboneOperations(routePath string, netName string, routeLayer string, widthMM float64, routePoints []transactions.Point, from PlacedPadEndpoint, to PlacedPadEndpoint, vias []transactions.RouteViaSpec, dogbone *blocks.PCBEntryAnchorDogbone) ([]transactions.Operation, []reports.Issue) {
	if dogbone == nil {
		return nil, nil
	}
	routeCopperLayer, oppositeCopperLayer, supportedLayer := localRouteEntryAnchorDogboneExternalLayers(routeLayer)
	if !supportedLayer {
		return nil, []reports.Issue{localRouteBindingIssue(routePath+".entry_anchor_dogbone", "entry anchor dogbones are only supported on F.Cu and B.Cu routes", []string{from.Ref, to.Ref})}
	}
	var operations []transactions.Operation
	var issues []reports.Issue
	for _, endpoint := range []PlacedPadEndpoint{from, to} {
		if !localRouteEndpointIsEntryAnchor(endpoint) || !localRouteHasViaAt(vias, endpoint.Point) {
			continue
		}
		tie, ok := localRouteEntryAnchorDogboneTiePoint(endpoint.Point, routePoints, dogbone.TieOffset)
		if !ok {
			issues = append(issues, localRouteBindingIssue(routePath+".entry_anchor_dogbone", "no bounded dogbone tie point is available for entry anchor", []string{endpoint.Ref}))
			continue
		}
		// Virtual entry anchors need a physical copper contact for internal and
		// KiCad connectivity evidence. The short same-net tie gives the anchor
		// via copper on both layers without rerouting the main local route.
		routeLayerOperation, err := workflowOperation(transactions.OpRoute, transactions.RouteOperation{
			Op:      transactions.OpRoute,
			NetName: netName,
			Layer:   routeCopperLayer,
			WidthMM: widthMM,
			Points:  []transactions.Point{endpoint.Point, tie},
		})
		if err != nil {
			issues = append(issues, localRouteBindingIssue(routePath+".entry_anchor_dogbone", err.Error(), []string{endpoint.Ref}))
			continue
		}
		routeLayerOperation.Ref = localRouteEndpointDogboneOperationRef
		oppositeLayerOperation, err := workflowOperation(transactions.OpRoute, transactions.RouteOperation{
			Op:      transactions.OpRoute,
			NetName: netName,
			Layer:   oppositeCopperLayer,
			WidthMM: widthMM,
			Points:  []transactions.Point{endpoint.Point, tie},
			// The tie via is intentional: the anchor via connects one end of
			// this stub, and the tie via gives the far end a proven endpoint for
			// internal route-connectivity checks without changing the main route.
			Vias: []transactions.RouteViaSpec{{
				At:         tie,
				DiameterMM: defaultLocalRouteViaDiameterMM,
				DrillMM:    defaultLocalRouteViaDrillMM,
				Layers:     []string{routeCopperLayer, oppositeCopperLayer},
			}},
		})
		if err != nil {
			issues = append(issues, localRouteBindingIssue(routePath+".entry_anchor_dogbone", err.Error(), []string{endpoint.Ref}))
			continue
		}
		oppositeLayerOperation.Ref = localRouteEndpointDogboneOperationRef
		operations = append(operations, routeLayerOperation, oppositeLayerOperation)
	}
	return operations, issues
}

func localRouteEntryAnchorDogboneExternalLayers(routeLayer string) (string, string, bool) {
	routeCopperLayer := canonicalCopperLayer(routeLayer)
	switch {
	case strings.EqualFold(routeCopperLayer, defaultLocalRouteTopLayer):
		return defaultLocalRouteTopLayer, defaultLocalRouteBottomLayer, true
	case strings.EqualFold(routeCopperLayer, defaultLocalRouteBottomLayer):
		return defaultLocalRouteBottomLayer, defaultLocalRouteTopLayer, true
	default:
		return routeCopperLayer, "", false
	}
}

func localRouteEntryAnchorDogboneTiePoint(point transactions.Point, routePoints []transactions.Point, offset blocks.RelativePoint) (transactions.Point, bool) {
	candidate := transactions.Point{XMM: point.XMM + offset.XMM, YMM: point.YMM + offset.YMM}
	if !localRouteEntryAnchorDogboneTiePointAllowed(point, candidate, routePoints) {
		return transactions.Point{}, false
	}
	return candidate, true
}

func localRouteEntryAnchorDogboneTiePointAllowed(anchor transactions.Point, candidate transactions.Point, routePoints []transactions.Point) bool {
	for _, point := range routePoints {
		if sameRoutePoint(point, anchor) {
			continue
		}
		if sameRoutePoint(point, candidate) {
			return false
		}
	}
	return true
}

func localRouteEndpointIsEntryAnchor(endpoint PlacedPadEndpoint) bool {
	return strings.TrimSpace(endpoint.Source) == localRouteEntryAnchorSource
}

func localRouteHasViaAt(vias []transactions.RouteViaSpec, point transactions.Point) bool {
	for _, via := range vias {
		if sameRoutePoint(via.At, point) {
			return true
		}
	}
	return false
}

func localRouteMaterializedAnchorVia(point transactions.Point, routeLayer string) transactions.RouteViaSpec {
	layers := []string{defaultLocalRouteTopLayer, defaultLocalRouteBottomLayer}
	if strings.EqualFold(canonicalCopperLayer(routeLayer), defaultLocalRouteBottomLayer) {
		layers = []string{defaultLocalRouteBottomLayer, defaultLocalRouteTopLayer}
	}
	return transactions.RouteViaSpec{
		At:         point,
		DiameterMM: defaultLocalRouteViaDiameterMM,
		DrillMM:    defaultLocalRouteViaDrillMM,
		Layers:     layers,
	}
}

func sameRoutePoint(a transactions.Point, b transactions.Point) bool {
	const toleranceMM = 0.001
	return math.Hypot(a.XMM-b.XMM, a.YMM-b.YMM) <= toleranceMM
}

type translatedUnitRouteContext struct {
	authored map[string]transactions.Point
	groups   []map[string]struct{}
}

func newTranslatedUnitRouteContext(fragment BlockFragment) translatedUnitRouteContext {
	context := translatedUnitRouteContext{authored: make(map[string]transactions.Point, len(fragment.Realization.Components))}
	for _, component := range fragment.Realization.Components {
		context.authored[strings.ToUpper(strings.TrimSpace(component.Ref))] = transactions.Point{XMM: component.Placement.XMM, YMM: component.Placement.YMM}
	}
	for _, group := range fragment.PlacementGroups {
		if !group.TranslateAsUnit {
			continue
		}
		members := make(map[string]struct{}, len(group.ComponentRoles))
		for _, role := range group.ComponentRoles {
			if ref := strings.TrimSpace(fragment.Realization.RoleRefs[strings.TrimSpace(role)]); ref != "" {
				members[strings.ToUpper(ref)] = struct{}{}
			}
		}
		if len(members) != 0 {
			context.groups = append(context.groups, members)
		}
	}
	return context
}

func translatedUnitLocalRoutePoints(context translatedUnitRouteContext, route blocks.RealizedPCBLocalRoute, from PlacedPadEndpoint, to PlacedPadEndpoint) ([]transactions.Point, bool) {
	if len(route.Points) < 2 {
		return nil, false
	}
	fromRef := strings.ToUpper(strings.TrimSpace(from.Ref))
	toRef := strings.ToUpper(strings.TrimSpace(to.Ref))
	if points, ok := translatedLocalRoutePointsForCommonPlacementDelta(context, route, from, to, fromRef, toRef); ok {
		return points, true
	}
	for _, members := range context.groups {
		if _, ok := members[fromRef]; !ok {
			continue
		}
		if _, ok := members[toRef]; !ok {
			continue
		}
		fromAuthored, fromOK := context.authored[fromRef]
		toAuthored, toOK := context.authored[toRef]
		if !fromOK || !toOK {
			continue
		}
		fromDelta := transactions.Point{XMM: from.ComponentAt.XMM - fromAuthored.XMM, YMM: from.ComponentAt.YMM - fromAuthored.YMM}
		toDelta := transactions.Point{XMM: to.ComponentAt.XMM - toAuthored.XMM, YMM: to.ComponentAt.YMM - toAuthored.YMM}
		const toleranceMM = 0.001
		if math.Hypot(fromDelta.XMM-toDelta.XMM, fromDelta.YMM-toDelta.YMM) > toleranceMM {
			return nil, false
		}
		points := make([]transactions.Point, 0, len(route.Points))
		points = append(points, from.Point)
		for _, point := range route.Points[1 : len(route.Points)-1] {
			points = append(points, transactions.Point{XMM: point.XMM + fromDelta.XMM, YMM: point.YMM + fromDelta.YMM})
		}
		points = append(points, to.Point)
		return compactRoutePoints(points), true
	}
	return nil, false
}

func translatedLocalRoutePointsForCommonPlacementDelta(context translatedUnitRouteContext, route blocks.RealizedPCBLocalRoute, from PlacedPadEndpoint, to PlacedPadEndpoint, fromRef string, toRef string) ([]transactions.Point, bool) {
	fromDelta, fromOK := translatedLocalRouteEndpointDelta(context, fromRef, from, route.Points[0])
	toDelta, toOK := translatedLocalRouteEndpointDelta(context, toRef, to, route.Points[len(route.Points)-1])
	if !fromOK || !toOK {
		return nil, false
	}
	const toleranceMM = 0.001
	if math.Hypot(fromDelta.XMM-toDelta.XMM, fromDelta.YMM-toDelta.YMM) > toleranceMM {
		return nil, false
	}
	points := make([]transactions.Point, 0, len(route.Points))
	points = append(points, from.Point)
	for _, point := range route.Points[1 : len(route.Points)-1] {
		points = append(points, transactions.Point{XMM: point.XMM + fromDelta.XMM, YMM: point.YMM + fromDelta.YMM})
	}
	points = append(points, to.Point)
	return compactRoutePoints(points), true
}

func translatedLocalRouteEndpointDelta(context translatedUnitRouteContext, ref string, endpoint PlacedPadEndpoint, authoredEndpoint transactions.Point) (transactions.Point, bool) {
	if authored, ok := context.authored[ref]; ok {
		return transactions.Point{XMM: endpoint.ComponentAt.XMM - authored.XMM, YMM: endpoint.ComponentAt.YMM - authored.YMM}, true
	}
	if localRouteEndpointIsEntryAnchor(endpoint) {
		return transactions.Point{XMM: endpoint.Point.XMM - authoredEndpoint.XMM, YMM: endpoint.Point.YMM - authoredEndpoint.YMM}, true
	}
	return transactions.Point{}, false
}

func placedLocalRoutePoints(points []transactions.Point, from transactions.Point, to transactions.Point) ([]transactions.Point, bool) {
	// Realized local-route points include authored source/destination anchors:
	// [source endpoint, zero or more waypoints, destination endpoint].
	if transformed, ok := transformedLocalRoutePoints(points, from, to); ok {
		return compactRoutePoints(transformed), true
	}
	if len(points) < 3 {
		return nil, false
	}
	if transformed, ok := transformedLocalRouteShape(points, from, to); ok {
		return compactRoutePoints(transformed), true
	}
	if authoredRoutePointsNearPlacedEndpoints(points, from, to) {
		routed := make([]transactions.Point, 0, len(points))
		routed = append(routed, from)
		routed = append(routed, points[1:len(points)-1]...)
		routed = append(routed, to)
		return compactRoutePoints(routed), true
	}
	return compactRoutePoints([]transactions.Point{from, to}), true
}

func authoredRoutePointsNearPlacedEndpoints(points []transactions.Point, from transactions.Point, to transactions.Point) bool {
	if len(points) < 3 {
		return false
	}
	minX := math.Min(from.XMM, to.XMM)
	maxX := math.Max(from.XMM, to.XMM)
	minY := math.Min(from.YMM, to.YMM)
	maxY := math.Max(from.YMM, to.YMM)
	const marginMM = 25.0
	for _, point := range points[1 : len(points)-1] {
		if point.XMM < minX-marginMM || point.XMM > maxX+marginMM || point.YMM < minY-marginMM || point.YMM > maxY+marginMM {
			return false
		}
	}
	return true
}

func compactRoutePoints(points []transactions.Point) []transactions.Point {
	if len(points) < 2 {
		return points
	}
	const toleranceMM = 0.001
	compacted := make([]transactions.Point, 0, len(points))
	for _, point := range points {
		if len(compacted) == 0 {
			compacted = append(compacted, point)
			continue
		}
		previous := compacted[len(compacted)-1]
		if math.Hypot(previous.XMM-point.XMM, previous.YMM-point.YMM) <= toleranceMM {
			continue
		}
		compacted = append(compacted, point)
	}
	if len(compacted) < 2 {
		compacted = append(compacted, points[len(points)-1])
	}
	return compacted
}

func transformedLocalRoutePoints(points []transactions.Point, from transactions.Point, to transactions.Point) ([]transactions.Point, bool) {
	if len(points) < 2 {
		return nil, false
	}
	const toleranceMM = 0.001
	first := points[0]
	last := points[len(points)-1]
	fromDelta := transactions.Point{XMM: from.XMM - first.XMM, YMM: from.YMM - first.YMM}
	toDelta := transactions.Point{XMM: to.XMM - last.XMM, YMM: to.YMM - last.YMM}
	if math.Hypot(fromDelta.XMM-toDelta.XMM, fromDelta.YMM-toDelta.YMM) > toleranceMM {
		return nil, false
	}
	transformed := make([]transactions.Point, len(points))
	for index, point := range points {
		transformed[index] = transactions.Point{XMM: point.XMM + fromDelta.XMM, YMM: point.YMM + fromDelta.YMM}
	}
	return transformed, true
}

func transformedLocalRouteShape(points []transactions.Point, from transactions.Point, to transactions.Point) ([]transactions.Point, bool) {
	// Shape transforms require at least the authored source anchor, one
	// waypoint, and the authored destination anchor.
	if len(points) < 3 {
		return nil, false
	}
	first := points[0]
	last := points[len(points)-1]
	if !authoredRoutePointsNearPlacedEndpoints(points, first, last) {
		return nil, false
	}
	fromDelta := transactions.Point{XMM: from.XMM - first.XMM, YMM: from.YMM - first.YMM}
	toDelta := transactions.Point{XMM: to.XMM - last.XMM, YMM: to.YMM - last.YMM}
	if math.Hypot(fromDelta.XMM-toDelta.XMM, fromDelta.YMM-toDelta.YMM) > localRouteShapeEndpointDeltaMismatchMaxMM {
		return nil, false
	}
	sourceX := last.XMM - first.XMM
	sourceY := last.YMM - first.YMM
	sourceLength := math.Hypot(sourceX, sourceY)
	destinationX := to.XMM - from.XMM
	destinationY := to.YMM - from.YMM
	destinationLength := math.Hypot(destinationX, destinationY)
	const toleranceMM = 0.001
	if sourceLength <= toleranceMM || destinationLength <= toleranceMM {
		return nil, false
	}
	sourceLengthSquared := sourceLength * sourceLength
	destinationUnitX := destinationX / destinationLength
	destinationUnitY := destinationY / destinationLength
	destinationPerpX := -destinationUnitY
	destinationPerpY := destinationUnitX
	transformed := make([]transactions.Point, 0, len(points))
	transformed = append(transformed, from)
	for _, point := range points[1 : len(points)-1] {
		relativeX := point.XMM - first.XMM
		relativeY := point.YMM - first.YMM
		progress := (relativeX*sourceX + relativeY*sourceY) / sourceLengthSquared
		// Keep perpendicular dogleg offsets in authored millimeters so clearance
		// intent is preserved when only endpoint spacing changes.
		perpendicularOffset := (relativeY*sourceX - relativeX*sourceY) / sourceLength
		transformed = append(transformed, transactions.Point{
			XMM: from.XMM + progress*destinationX + perpendicularOffset*destinationPerpX,
			YMM: from.YMM + progress*destinationY + perpendicularOffset*destinationPerpY,
		})
	}
	transformed = append(transformed, to)
	return transformed, true
}

func resolveLocalRouteEndpoint(fragment BlockFragment, routePath string, netName string, routeWidthMM float64, endpoint transactions.Endpoint, resolver PlacedPadEndpointResolver, board placement.BoardPlacementArea) (PlacedPadEndpoint, []reports.Issue, bool, bool) {
	ref := strings.TrimSpace(endpoint.Ref)
	pin := strings.TrimSpace(endpoint.Pin)
	if ref == "" || pin == "" {
		return PlacedPadEndpoint{}, []reports.Issue{localRouteBindingIssue(routePath, "local route endpoint requires ref and pin", nil)}, false, false
	}
	if anchor, ok := resolveLocalRouteAnchorEndpoint(fragment, ref, pin, routeWidthMM, resolver, board); ok {
		if strings.TrimSpace(anchor.NetName) != "" && !strings.EqualFold(anchor.NetName, netName) {
			return anchor, []reports.Issue{localRouteBindingIssue(routePath+".net_name", "local route anchor net "+anchor.NetName+" does not match route net "+netName, []string{ref})}, false, true
		}
		anchor.NetName = netName
		anchor.NetCodeResolved = true
		return anchor, nil, true, false
	}
	resolved, ok := resolver.ResolveNormalized(strings.ToUpper(ref), strings.ToUpper(pin))
	if !ok {
		return PlacedPadEndpoint{}, []reports.Issue{localRouteBindingIssue(routePath, "local route endpoint does not resolve to a placed pad", []string{ref, ref + "." + pin})}, false, false
	}
	if !resolved.NetCodeResolved {
		return resolved, []reports.Issue{localRouteBindingIssue(routePath+".net_code", "local route endpoint pad net code is unresolved", []string{ref})}, false, false
	}
	padNet := strings.TrimSpace(resolved.NetName)
	if padNet == "" {
		return resolved, []reports.Issue{localRouteBindingIssue(routePath+".net_name", "local route endpoint pad has no assigned net", []string{ref})}, false, false
	}
	if !strings.EqualFold(padNet, netName) {
		return resolved, []reports.Issue{localRouteBindingIssue(routePath+".net_name", "local route endpoint pad net "+padNet+" does not match route net "+netName, []string{ref})}, false, true
	}
	return resolved, nil, true, false
}

func resolveLocalRouteAnchorEndpoint(fragment BlockFragment, ref string, pin string, routeWidthMM float64, resolver PlacedPadEndpointResolver, board placement.BoardPlacementArea) (PlacedPadEndpoint, bool) {
	anchorID, ok := localRouteAnchorID(ref)
	if !ok {
		return PlacedPadEndpoint{}, false
	}
	for _, anchor := range fragment.Realization.EntryAnchors {
		if !strings.EqualFold(strings.TrimSpace(anchor.ID), anchorID) {
			continue
		}
		anchorPin := strings.TrimSpace(firstNonEmpty(anchor.Port, anchor.ID))
		if anchorPin != "" && pin != "" && !strings.EqualFold(anchorPin, pin) {
			continue
		}
		layer := firstNonEmpty(anchor.Placement.Layer, "F.Cu")
		point := transactions.Point{XMM: anchor.Placement.XMM, YMM: anchor.Placement.YMM}
		if placed, placedOK := placedLocalRouteEntryAnchorPointWithWidth(fragment, anchorID, point, routeWidthMM, resolver, board); placedOK {
			point = placed
		}
		// RealizeBlockPCB offsets entry anchors by the fragment origin before
		// they are stored here, so these coordinates are already board-level.
		// Preserve a proven common translation. When attached components moved
		// independently, rebuild the virtual junction at their pad centroid.
		return PlacedPadEndpoint{
			Ref:             strings.TrimSpace(ref),
			Pad:             strings.TrimSpace(pin),
			NetName:         strings.TrimSpace(anchor.NetName),
			NetCodeResolved: true,
			Point:           point,
			Layer:           layer,
			ComponentAt:     point,
			Source:          "pcb_realization.entry_anchor",
			Confidence:      PhysicalEndpointConfidenceHigh,
		}, true
	}
	return PlacedPadEndpoint{}, false
}

func placedLocalRouteEntryAnchorPoint(fragment BlockFragment, anchorID string, point transactions.Point, resolver PlacedPadEndpointResolver, board placement.BoardPlacementArea) (transactions.Point, bool) {
	return placedLocalRouteEntryAnchorPointWithWidth(fragment, anchorID, point, 0, resolver, board)
}

func placedLocalRouteEntryAnchorPointWithWidth(fragment BlockFragment, anchorID string, point transactions.Point, routeWidthMM float64, resolver PlacedPadEndpointResolver, board placement.BoardPlacementArea) (transactions.Point, bool) {
	anchorRef := "@anchor:" + strings.TrimSpace(anchorID)
	attached := map[routeEndpointMapKey]transactions.Endpoint{}
	appendAttached := func(endpoint transactions.Endpoint) {
		if _, isAnchor := localRouteAnchorID(endpoint.Ref); isAnchor || strings.TrimSpace(endpoint.Ref) == "" || strings.TrimSpace(endpoint.Pin) == "" {
			return
		}
		attached[routeEndpointKey(endpoint.Ref, endpoint.Pin)] = endpoint
	}
	for _, route := range fragment.Realization.LocalRoutes {
		switch {
		case strings.EqualFold(strings.TrimSpace(route.From.Ref), anchorRef):
			if route.EntryAnchorDogbone != nil {
				return point, true
			}
			appendAttached(route.To)
		case strings.EqualFold(strings.TrimSpace(route.To.Ref), anchorRef):
			if route.EntryAnchorDogbone != nil {
				return point, true
			}
			appendAttached(route.From)
		}
	}
	if len(attached) == 0 {
		return transactions.Point{}, false
	}
	authoredByRef := make(map[string]transactions.Point, len(fragment.Realization.Components))
	for _, component := range fragment.Realization.Components {
		authoredByRef[strings.ToUpper(strings.TrimSpace(component.Ref))] = transactions.Point{XMM: component.Placement.XMM, YMM: component.Placement.YMM}
	}
	placedByRef := map[string]transactions.Point{}
	for _, endpoint := range resolver.Endpoints() {
		refKey := strings.ToUpper(strings.TrimSpace(endpoint.Ref))
		if _, exists := placedByRef[refKey]; !exists {
			placedByRef[refKey] = endpoint.ComponentAt
		}
	}

	var commonDelta transactions.Point
	commonDeltaProved := false
	commonDeltaValid := true
	for _, endpoint := range attached {
		refKey := strings.ToUpper(strings.TrimSpace(endpoint.Ref))
		authored, authoredOK := authoredByRef[refKey]
		placed, placedOK := placedByRef[refKey]
		if !authoredOK || !placedOK {
			commonDeltaValid = false
			break
		}
		delta := transactions.Point{XMM: placed.XMM - authored.XMM, YMM: placed.YMM - authored.YMM}
		if !commonDeltaProved {
			commonDelta = delta
			commonDeltaProved = true
			continue
		}
		if math.Hypot(commonDelta.XMM-delta.XMM, commonDelta.YMM-delta.YMM) > 0.001 {
			commonDeltaValid = false
			break
		}
	}
	if commonDeltaValid && commonDeltaProved {
		translated := transactions.Point{XMM: point.XMM + commonDelta.XMM, YMM: point.YMM + commonDelta.YMM}
		return relocateSinglePadEntryAnchor(translated, attached, routeWidthMM, resolver, board)
	}

	centroid := transactions.Point{}
	for _, endpoint := range attached {
		resolved, ok := resolver.Resolve(endpoint)
		if !ok {
			return transactions.Point{}, false
		}
		centroid.XMM += resolved.Point.XMM
		centroid.YMM += resolved.Point.YMM
	}
	count := float64(len(attached))
	centroid.XMM /= count
	centroid.YMM /= count
	return centroid, true
}

func relocateSinglePadEntryAnchor(point transactions.Point, attached map[routeEndpointMapKey]transactions.Endpoint, routeWidthMM float64, resolver PlacedPadEndpointResolver, board placement.BoardPlacementArea) (transactions.Point, bool) {
	if len(attached) == 0 {
		return point, true
	}
	physical := make([]PlacedPadEndpoint, 0, len(attached))
	attachedKeys := make(map[routeEndpointMapKey]struct{}, len(attached))
	for _, endpoint := range attached {
		resolved, ok := resolver.Resolve(endpoint)
		if !ok {
			// Some synthetic callers provide only component centers. In that case
			// preserve the already-proven common translation rather than guessing
			// a new electrical junction.
			return point, true
		}
		physical = append(physical, resolved)
		attachedKeys[routeEndpointKey(endpoint.Ref, endpoint.Pin)] = struct{}{}
	}
	centroid := transactions.Point{}
	netName := ""
	for _, endpoint := range physical {
		centroid.XMM += endpoint.Point.XMM
		centroid.YMM += endpoint.Point.YMM
		netName = firstNonEmpty(netName, strings.TrimSpace(endpoint.NetName))
	}
	centroid.XMM /= float64(len(physical))
	centroid.YMM /= float64(len(physical))
	relocate := entryAnchorOutsideBoardCopperBounds(point, board)
	searchRadius := resolver.MaximumPadRadiusMM() + math.Max(0, routeWidthMM)/2 + interBlockContactToleranceMM
	nearby := resolver.EndpointsWithinBounds(point.XMM-searchRadius, point.YMM-searchRadius, point.XMM+searchRadius, point.YMM+searchRadius)
	for _, candidate := range nearby {
		if _, attachedEndpoint := attachedKeys[routeEndpointKey(candidate.Ref, candidate.Pad)]; attachedEndpoint {
			continue
		}
		if netName != "" && strings.EqualFold(strings.TrimSpace(candidate.NetName), netName) {
			continue
		}
		foreignPadRadius := math.Hypot(candidate.PadWidthMM, candidate.PadHeightMM) / 2
		if strings.EqualFold(strings.TrimSpace(candidate.PadShape), "circle") {
			foreignPadRadius = math.Max(candidate.PadWidthMM, candidate.PadHeightMM) / 2
		}
		traceRadius := math.Max(0, routeWidthMM) / 2
		if pointDistanceMM(point, candidate.Point) <= foreignPadRadius+traceRadius+interBlockContactToleranceMM {
			relocate = true
			break
		}
	}
	if relocate && !entryAnchorOutsideBoardCopperBounds(centroid, board) {
		return centroid, true
	}
	return point, true
}

func entryAnchorOutsideBoardCopperBounds(point transactions.Point, board placement.BoardPlacementArea) bool {
	if board.WidthMM <= 0 || board.HeightMM <= 0 {
		return false
	}
	clearance := math.Max(0, board.MarginMM)
	minX := board.Origin.XMM + clearance
	minY := board.Origin.YMM + clearance
	maxX := board.Origin.XMM + board.WidthMM - clearance
	maxY := board.Origin.YMM + board.HeightMM - clearance
	return point.XMM < minX || point.YMM < minY || point.XMM > maxX || point.YMM > maxY
}

func localRouteAnchorID(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	const prefix = "@anchor:"
	if !strings.HasPrefix(strings.ToLower(ref), prefix) {
		return "", false
	}
	id := strings.TrimSpace(ref[len(prefix):])
	return id, id != ""
}

func localRouteBindingIssue(routePath string, message string, refs []string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityBlocked,
		Path:     "design.route_connectivity." + strings.Trim(routePath, "."),
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
		raw := canonicalRouteOperationLayers(operation.Raw)
		txOperation := transactions.NewOperation(transactions.OpRoute, raw)
		txOperation.Index = index
		out = append(out, txOperation)
	}
	return out
}

func dedupeSameNetRouteVias(operations []transactions.Operation) []transactions.Operation {
	out, _ := dedupeSameNetRouteViasWithEvidence(operations)
	return out
}

func dedupeSameNetRouteViasWithEvidence(operations []transactions.Operation) ([]transactions.Operation, map[int]struct{}) {
	if len(operations) == 0 {
		return nil, nil
	}
	routes := decodeRouteOperations(operations)
	snapPoints := sameNetViaSnapPoints(routes)
	seen := map[routeViaPointKey]struct{}{}
	out := make([]transactions.Operation, 0, len(operations))
	reduced := map[int]struct{}{}
	for operationIndex, route := range routes {
		operation := route.operation
		if !route.decoded {
			out = append(out, operation)
			continue
		}
		payload := route.payload
		changed := false
		for index := range payload.Vias {
			if point, ok := snapPoints.vias[sameNetViaPointKey(payload.NetName, payload.Vias[index].At, payload.Vias[index].Layers)]; ok {
				payload.Vias[index].At = point
				changed = true
			}
		}
		for index := range payload.Points {
			if point, ok := snapPoints.layerPoints[sameNetLayerPointKey(payload.NetName, payload.Points[index], payload.Layer)]; ok {
				payload.Points[index] = point
				changed = true
			}
		}
		originalViaCount := len(payload.Vias)
		filtered := payload.Vias[:0]
		for _, via := range payload.Vias {
			key := sameNetViaPointKey(payload.NetName, via.At, via.Layers)
			if _, exists := seen[key]; exists {
				changed = true
				continue
			}
			seen[key] = struct{}{}
			filtered = append(filtered, via)
		}
		payload.Vias = filtered
		if len(payload.Vias) < originalViaCount {
			reduced[operationIndex] = struct{}{}
		}
		if !changed {
			out = append(out, operation)
			continue
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			out = append(out, operation)
			continue
		}
		operation.Raw = raw
		if len(payload.Vias) < originalViaCount {
			operation.PruneProtected = true
		}
		out = append(out, operation)
	}
	return out, reduced
}

type decodedRouteOperation struct {
	operation transactions.Operation
	payload   transactions.RouteOperation
	decoded   bool
	decodeErr error
}

type routeViaRecord struct {
	netName string
	point   transactions.Point
	drillMM float64
	layers  string
}

type routeViaSnapPoints struct {
	vias        map[routeViaPointKey]transactions.Point
	layerPoints map[routeLayerPointKey]transactions.Point
}

type routeCoordKey struct {
	x int64
	y int64
}

type routeViaPointKey struct {
	net    string
	point  routeCoordKey
	layers string
}

type routeLayerPointKey struct {
	net   string
	point routeCoordKey
	layer string
}

type routeCellKey struct {
	x int
	y int
}

func decodeRouteOperations(operations []transactions.Operation) []decodedRouteOperation {
	// Keep one aligned entry per input operation because downstream pruning uses
	// slice positions as evidence keys. Only route operations are unmarshaled;
	// non-route entries are zero-cost placeholders that preserve that alignment.
	routes := make([]decodedRouteOperation, 0, len(operations))
	for _, operation := range operations {
		route := decodedRouteOperation{operation: operation}
		if operation.Op == transactions.OpRoute && len(operation.Raw) > 0 {
			if err := json.Unmarshal(operation.Raw, &route.payload); err == nil {
				route.decoded = true
			} else {
				route.decodeErr = err
			}
		}
		routes = append(routes, route)
	}
	return routes
}

func sameNetViaSnapPoints(routes []decodedRouteOperation) routeViaSnapPoints {
	const minHoleClearanceMM = 0.25
	snapPoints := routeViaSnapPoints{
		vias:        map[routeViaPointKey]transactions.Point{},
		layerPoints: map[routeLayerPointKey]transactions.Point{},
	}
	seen := newRouteViaSpatialIndex()
	for _, route := range routes {
		if !route.decoded {
			continue
		}
		payload := route.payload
		for _, via := range payload.Vias {
			current := routeViaRecord{netName: payload.NetName, point: via.At, drillMM: via.DrillMM, layers: viaLayerSpanKey(via.Layers)}
			if match, ok := nearestSameNetVia(current, seen, minHoleClearanceMM); ok {
				snapPoints.vias[sameNetViaPointKey(payload.NetName, via.At, via.Layers)] = match.point
				for _, layer := range layersForRoutePointSnap(via.Layers, payload.Layer) {
					snapPoints.layerPoints[sameNetLayerPointKey(payload.NetName, via.At, layer)] = match.point
				}
				continue
			}
			seen.add(current)
		}
	}
	return snapPoints
}

type routeViaSpatialIndex struct {
	cellSizeMM float64
	maxDrillMM float64
	byNet      map[string]map[routeCellKey][]routeViaRecord
}

func newRouteViaSpatialIndex() *routeViaSpatialIndex {
	return &routeViaSpatialIndex{
		cellSizeMM: 1,
		byNet:      map[string]map[routeCellKey][]routeViaRecord{},
	}
}

func (index *routeViaSpatialIndex) add(record routeViaRecord) {
	netKey := routeNetKey(record.netName)
	if _, ok := index.byNet[netKey]; !ok {
		index.byNet[netKey] = map[routeCellKey][]routeViaRecord{}
	}
	cellKey := index.cellKey(record.point)
	index.byNet[netKey][cellKey] = append(index.byNet[netKey][cellKey], record)
	if record.drillMM > index.maxDrillMM {
		index.maxDrillMM = record.drillMM
	}
}

func (index *routeViaSpatialIndex) candidates(record routeViaRecord, minHoleClearanceMM float64) []routeViaRecord {
	netCells := index.byNet[routeNetKey(record.netName)]
	if len(netCells) == 0 {
		return nil
	}
	cellX, cellY := index.cell(record.point)
	searchDistanceMM := minHoleClearanceMM + record.drillMM/2 + index.maxDrillMM/2
	cellRange := int(math.Ceil(searchDistanceMM / index.cellSizeMM))
	if cellRange < 1 {
		cellRange = 1
	}
	var candidates []routeViaRecord
	for x := cellX - cellRange; x <= cellX+cellRange; x++ {
		for y := cellY - cellRange; y <= cellY+cellRange; y++ {
			candidates = append(candidates, netCells[index.cellCoordKey(x, y)]...)
		}
	}
	return candidates
}

func (index *routeViaSpatialIndex) cell(point transactions.Point) (int, int) {
	return int(math.Floor(point.XMM / index.cellSizeMM)), int(math.Floor(point.YMM / index.cellSizeMM))
}

func (index *routeViaSpatialIndex) cellKey(point transactions.Point) routeCellKey {
	x, y := index.cell(point)
	return index.cellCoordKey(x, y)
}

func (index *routeViaSpatialIndex) cellCoordKey(x int, y int) routeCellKey {
	return routeCellKey{x: x, y: y}
}

func nearestSameNetVia(current routeViaRecord, seen *routeViaSpatialIndex, minHoleClearanceMM float64) (routeViaRecord, bool) {
	for _, existing := range seen.candidates(current, minHoleClearanceMM) {
		if existing.layers != current.layers {
			continue
		}
		if routePointKey(existing.point) == routePointKey(current.point) {
			return existing, true
		}
		requiredCenterDistance := minHoleClearanceMM + existing.drillMM/2 + current.drillMM/2
		if math.Hypot(existing.point.XMM-current.point.XMM, existing.point.YMM-current.point.YMM) < requiredCenterDistance {
			return existing, true
		}
	}
	return routeViaRecord{}, false
}

func sameNetViaPointKey(netName string, point transactions.Point, layers []string) routeViaPointKey {
	return routeViaPointKey{net: routeNetKey(netName), point: routePointKey(point), layers: viaLayerSpanKey(layers)}
}

func sameNetLayerPointKey(netName string, point transactions.Point, layer string) routeLayerPointKey {
	return routeLayerPointKey{net: routeNetKey(netName), point: routePointKey(point), layer: strings.ToUpper(strings.TrimSpace(layer))}
}

func routeNetKey(netName string) string {
	return strings.ToUpper(strings.TrimSpace(netName))
}

func layersForRoutePointSnap(viaLayers []string, routeLayer string) []string {
	if len(viaLayers) == 0 {
		return []string{routeLayer}
	}
	return viaLayers
}

func viaLayerSpanKey(layers []string) string {
	normalized := make([]string, 0, len(layers))
	for _, layer := range layers {
		layer = strings.TrimSpace(layer)
		if layer == "" {
			continue
		}
		normalized = append(normalized, strings.ToUpper(layer))
	}
	sort.Strings(normalized)
	if len(normalized) >= 2 {
		// RouteViaSpec currently represents only ordinary plated through vias;
		// designapi canonicalizes every multi-layer transition to F.Cu-B.Cu.
		// Deduplicate by that emitted physical identity rather than by the
		// router's logical source and destination layers.
		return "THROUGH"
	}
	return strings.Join(normalized, "/")
}

func routePointKey(point transactions.Point) routeCoordKey {
	return routeCoordKey{x: int64(math.Round(point.XMM * 1000)), y: int64(math.Round(point.YMM * 1000))}
}

func canonicalRouteOperationLayers(raw json.RawMessage) json.RawMessage {
	var payload transactions.RouteOperation
	if err := json.Unmarshal(raw, &payload); err != nil {
		return raw
	}
	payload.Layer = canonicalCopperLayer(payload.Layer)
	for index := range payload.Vias {
		for layerIndex := range payload.Vias[index].Layers {
			payload.Vias[index].Layers[layerIndex] = canonicalCopperLayer(payload.Vias[index].Layers[layerIndex])
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return encoded
}

func canonicalCopperLayer(layer string) string {
	switch strings.ToUpper(strings.TrimSpace(layer)) {
	case "F.CU":
		return "F.Cu"
	case "B.CU":
		return "B.Cu"
	default:
		return layer
	}
}

func snapInterBlockRouteEndpoints(candidates []InterBlockRouteCandidate, operations []transactions.Operation, placed *PlacementStageResult) ([]transactions.Operation, []reports.Issue) {
	if len(candidates) == 0 || len(operations) == 0 {
		return operations, nil
	}
	evidence := BuildInterBlockContactTargets(candidates, placed)
	issues := append([]reports.Issue(nil), evidence.Issues...)
	targetsByNet := interBlockContactTargetsByNet(evidence.Targets)
	if len(targetsByNet) == 0 {
		return operations, issues
	}
	out := append([]transactions.Operation(nil), operations...)
	for index, operation := range out {
		if operation.SnapExempt {
			continue
		}
		if operation.Op != transactions.OpRoute {
			continue
		}
		var payload transactions.RouteOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			issues = append(issues, interBlockRouteSnapIssue(index, operation, "route operation could not be decoded for endpoint snapping: "+err.Error()))
			continue
		}
		netName := strings.TrimSpace(operation.Net)
		if netName == "" {
			netName = strings.TrimSpace(payload.NetName)
		}
		targets := targetsByNet[netName]
		if len(payload.Points) < 2 || len(targets) < 2 {
			continue
		}
		snapIssues := snapRoutePayloadEndpoints(&payload, targets, index, operation)
		if len(snapIssues) != 0 {
			issues = append(issues, snapIssues...)
			continue
		}
		payload.Points = compactRoutePoints(payload.Points)
		if routeTrackSegmentCount(payload.Points) == 0 {
			payload.Points = nil
			if len(payload.Vias) == 0 {
				out[index] = transactions.Operation{}
				continue
			}
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			issues = append(issues, interBlockRouteSnapIssue(index, operation, "route operation could not be encoded after endpoint snapping: "+err.Error()))
			continue
		}
		snapped := transactions.NewOperation(transactions.OpRoute, raw)
		snapped.Index = operation.Index
		out[index] = snapped
	}
	compacted := out[:0]
	for _, operation := range out {
		if operation.Op != "" {
			compacted = append(compacted, operation)
		}
	}
	return compacted, issues
}

func suppressProvenRouteDisconnectedIssues(issues []reports.Issue, evidence InterBlockContactEvidence, interBlockOperations []transactions.Operation, localOperations []transactions.Operation, localSummary LocalRouteConnectivitySummary) []reports.Issue {
	if len(issues) == 0 {
		return nil
	}
	proven := interBlockConnectedNets(evidence, interBlockOperations)
	if localSummary.IssueCount == 0 && localSummary.EndpointContactsProven >= localSummary.RoutesBound*2 {
		for netName := range routeSegmentCountsByNet(localOperations) {
			proven[netName] = true
		}
	}
	if len(proven) == 0 {
		return issues
	}
	filtered := make([]reports.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Code == reports.CodeDisconnectedPad && issueNetsProven(issue, proven) {
			continue
		}
		filtered = append(filtered, issue)
	}
	return filtered
}

func issueNetsProven(issue reports.Issue, proven map[string]bool) bool {
	if len(issue.Nets) == 0 {
		return false
	}
	for _, netName := range issue.Nets {
		if !proven[strings.TrimSpace(netName)] {
			return false
		}
	}
	return true
}

func snapRoutePayloadEndpoints(payload *transactions.RouteOperation, targets []InterBlockContactTarget, operationIndex int, operation transactions.Operation) []reports.Issue {
	if payload == nil || len(payload.Points) < 2 || len(targets) < 2 {
		return nil
	}
	first := payload.Points[0]
	lastIndex := len(payload.Points) - 1
	last := payload.Points[lastIndex]
	left, right := nearestEndpointTargetPair(first, last, targets)
	if distance := math.Sqrt(routeSnapDistance(first, left.Point)); distance > interBlockRouteSnapMaxDistanceMM {
		// A route may intentionally merge into already-proven same-net copper
		// instead of terminating at a second concrete pad. Snapping is only a
		// coordinate-normalization aid; the contact graph below remains the
		// authoritative completion proof for unsnapped merge geometry.
		return []reports.Issue{interBlockRouteSnapInfoIssue(operationIndex, operation, "route start was left unchanged because it is too far from the nearest resolved contact target for optional endpoint snapping")}
	}
	if distance := math.Sqrt(routeSnapDistance(last, right.Point)); distance > interBlockRouteSnapMaxDistanceMM {
		return []reports.Issue{interBlockRouteSnapInfoIssue(operationIndex, operation, "route end was left unchanged because it is too far from the nearest resolved contact target for optional endpoint snapping")}
	}
	payload.Points[0] = left.Point
	payload.Points[lastIndex] = right.Point
	return nil
}

func nearestEndpointTargetPair(first transactions.Point, last transactions.Point, targets []InterBlockContactTarget) (InterBlockContactTarget, InterBlockContactTarget) {
	bestLeft := targets[0]
	bestRight := targets[1]
	bestDistance := math.Inf(1)
	for leftIndex, left := range targets {
		for rightIndex, right := range targets {
			if leftIndex == rightIndex {
				continue
			}
			distance := routeSnapDistance(first, left.Point) + routeSnapDistance(last, right.Point)
			if distance < bestDistance {
				bestDistance = distance
				bestLeft = left
				bestRight = right
			}
		}
	}
	return bestLeft, bestRight
}

func routeSnapDistance(left transactions.Point, right transactions.Point) float64 {
	dx := left.XMM - right.XMM
	dy := left.YMM - right.YMM
	return dx*dx + dy*dy
}

func interBlockRouteSnapIssue(index int, operation transactions.Operation, message string) reports.Issue {
	return reports.Issue{
		Code:        reports.CodeRouteContactUnsupported,
		Severity:    reports.SeverityBlocked,
		Path:        "design.inter_block_contact.operations[" + strconv.Itoa(index) + "]",
		Message:     message,
		OperationID: contactOperationID(operation),
	}
}

func interBlockRouteSnapInfoIssue(index int, operation transactions.Operation, message string) reports.Issue {
	issue := interBlockRouteSnapIssue(index, operation, message)
	issue.Severity = reports.SeverityInfo
	return issue
}
