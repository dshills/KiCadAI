package designworkflow

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/blocks"
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
}

type RouteTreeBranchEvidenceSummary struct {
	NetName  string                            `json:"net_name"`
	Branches []InterBlockBranchRoutingEvidence `json:"branches,omitempty"`
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
	if err := ctx.Err(); err != nil {
		return RoutingStageResult{Stage: NewStageResult(StageRouting, []reports.Issue{{
			Code:     reports.CodeOperationCanceled,
			Severity: reports.SeverityBlocked,
			Message:  err.Error(),
		}})}
	}
	localOperations, localRouteIssues, localRouteConnectivity := localRouteOperations(fragments, &placed)
	interBlockCandidates, interBlockCandidateIssues := BuildInterBlockRouteCandidates(fragments, placed)
	localRouteMobility := classifyLocalRouteMobility(fragments, placed.Request)
	componentHintResult := componentRoutingHints(opts.ComponentSelections, fragments)
	componentHintIssues := ComponentHintIssues(componentHintResult.Evidence)
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
	if localRouteConnectivity.IssueCount == 0 {
		routingRequest.Nets = excludeNetsWithRouteOperations(routingRequest.Nets, localOperations, interBlockCandidates)
	}
	targetEvidence := BuildInterBlockContactTargets(interBlockCandidates, &placed)
	routeTreeAccess, routeTreeAccessIssues := BuildRouteTreeEndpointAccessWithIssues(targetEvidence, localOperations)
	issues = append(issues, routeTreeAccessIssues...)
	routeTreeExecution := executeInterBlockRouteTrees(ctx, routingRequest, interBlockCandidates, targetEvidence, routeTreeAccess)
	routingRequest.Nets = excludeManagedInterBlockNets(routingRequest.Nets, routeTreeExecution.Summary.ManagedNets)
	issues = append(issues, targetEvidence.Issues...)
	issues = append(issues, routeTreeExecution.Issues...)
	result := routing.Result{Status: routing.StatusBlocked}
	if !reports.HasBlockingIssue(issues) {
		result = routing.RouteRequestContext(ctx, routingRequest)
		issues = append(issues, result.Issues...)
	}
	routeOperations := transactionRouteOperations(result.Operations)
	routeOperations = append(routeOperations, routeTreeExecution.Operations...)
	routeOperations, snapIssues := snapInterBlockRouteEndpoints(interBlockCandidates, routeOperations, &placed)
	issues = append(issues, snapIssues...)
	contactGraphOperations := make([]transactions.Operation, 0, len(routeOperations)+len(localOperations))
	contactGraphOperations = append(contactGraphOperations, routeOperations...)
	contactGraphOperations = append(contactGraphOperations, localOperations...)
	interBlockContactEvidence := ValidateInterBlockRouteEndpointContacts(interBlockCandidates, contactGraphOperations, &placed)
	issues = append(issues, interBlockContactEvidence.Issues...)
	issues = suppressProvenRouteDisconnectedIssues(issues, interBlockContactEvidence, routeOperations, localOperations, localRouteConnectivity)
	routeTreeRepairHints := BuildRouteTreeRepairHints(issues)
	operations := append(localOperations, anchorOperations...)
	operations = append(operations, routeOperations...)
	stage := NewStageResult(StageRouting, issues)
	stage.Issues = cloneIssues(issues)
	routeDiagnostics := routing.DiagnosticsForResult(result)
	stage.Summary = map[string]any{
		"local_route_operations":   len(localOperations),
		"route_operations":         len(result.Operations),
		"routed_nets":              result.Metrics.RoutedNetCount,
		"failed_nets":              result.Metrics.FailedNetCount,
		"status":                   result.Status,
		"repair_diagnostics":       len(routeDiagnostics),
		"local_route_mobility":     localRouteMobility,
		"route_connectivity":       localRouteConnectivity,
		"inter_block_routing":      summarizeInterBlockRouteCompletionWithGraphOperations(interBlockCandidates, routeOperations, contactGraphOperations, issues, interBlockContactEvidence),
		"inter_block_route_trees":  routeTreeExecution.Summary,
		"route_tree_branches":      routeTreeExecution.Branches,
		"route_tree_access":        SummarizeRouteTreeEndpointAccess(routeTreeAccess),
		"route_tree_contact_graph": SummarizeRouteTreeContactGraph(interBlockContactEvidence, contactGraphOperations, routeTreeAccess),
		"route_tree_repair":        SummarizeRouteTreeRepair(routeTreeRepairHints),
		"inter_block_contacts":     SummarizeInterBlockContacts(interBlockContactEvidence),
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
	for _, issue := range groupIssues {
		for _, net := range issue.Nets {
			net = interBlockSummaryNetKey(net)
			if net != "" {
				issueCountsByNet[net]++
			}
		}
	}
	connectedNets := interBlockConnectedNetsFromDecoded(targetsByNet, operationsByNet, operationIssues)
	treeByNet := interBlockRouteTreeByNet(trees)
	summarizeInterBlockRouteGroups(&summary, groups, treeByNet, provenEndpoints, graphComponents, routeSegmentsByNet, issueCountsByNet, connectedNets)
	for _, candidate := range candidates {
		netName := interBlockSummaryNetKey(candidate.NetName)
		summary.EndpointsResolved += len(candidate.Endpoints)
		summary.EndpointsUnresolved += candidate.Unresolved
		segments := routeSegmentsByNet[netName]
		summary.EmittedSegments += segments
		netIssueCount := issueCountsByNet[netName]
		summary.IssueCount += netIssueCount
		switch {
		case connectedNets[netName] && netIssueCount == 0:
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

func executeInterBlockRouteTrees(ctx context.Context, base routing.Request, candidates []InterBlockRouteCandidate, targetEvidence InterBlockContactEvidence, routeTreeAccess []RouteTreeEndpointAccess) interBlockRouteTreeExecutionResult {
	groups, groupIssues := BuildInterBlockRouteGroups(candidates)
	trees := BuildInterBlockRouteTrees(groups, targetEvidence)
	groupByNet := interBlockRouteGroupByNet(groups)
	execution := interBlockRouteTreeExecutionResult{}
	execution.Issues = append(execution.Issues, groupIssues...)
	workingBase := base
	workingBase.Existing = append([]routing.ExistingCopper(nil), base.Existing...)
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
		branchResult := RouteInterBlockTreeBranchesWithAccess(ctx, workingBase, group, tree, routeTreeAccess)
		execution.Operations = append(execution.Operations, branchResult.Operations...)
		execution.Issues = append(execution.Issues, branchResult.Issues...)
		execution.Branches = append(execution.Branches, RouteTreeBranchEvidenceSummary{
			NetName:  branchResult.NetName,
			Branches: append([]InterBlockBranchRoutingEvidence(nil), branchResult.Branches...),
		})
		workingBase.Existing = append(workingBase.Existing, branchResult.ExistingCopper...)
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
		raw := canonicalRouteOperationLayers(operation.Raw)
		txOperation := transactions.NewOperation(transactions.OpRoute, raw)
		txOperation.Index = index
		out = append(out, txOperation)
	}
	return out
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
		raw, err := json.Marshal(payload)
		if err != nil {
			issues = append(issues, interBlockRouteSnapIssue(index, operation, "route operation could not be encoded after endpoint snapping: "+err.Error()))
			continue
		}
		snapped := transactions.NewOperation(transactions.OpRoute, raw)
		snapped.Index = operation.Index
		out[index] = snapped
	}
	return out, issues
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
		return []reports.Issue{interBlockRouteSnapIssue(operationIndex, operation, "route start is too far from resolved contact target for endpoint snapping")}
	}
	if distance := math.Sqrt(routeSnapDistance(last, right.Point)); distance > interBlockRouteSnapMaxDistanceMM {
		return []reports.Issue{interBlockRouteSnapIssue(operationIndex, operation, "route end is too far from resolved contact target for endpoint snapping")}
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
