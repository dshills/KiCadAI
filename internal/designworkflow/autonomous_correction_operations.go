package designworkflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/transactions"
)

const (
	correctionOperationScopeIdentity  = "operation_identity"
	correctionOperationScopePath      = "operation_path"
	correctionOperationScopeNet       = "diagnostic_nets"
	correctionOperationScopeMissing   = "missing"
	correctionOperationScopeAmbiguous = "ambiguous"
)

type autonomousCorrectionRouteOperationTrace struct {
	ID             string
	SliceIndex     int
	OperationIndex int
	Net            string
}

func autonomousCorrectionRouteOperationTraces(operations []transactions.Operation) []autonomousCorrectionRouteOperationTrace {
	traces := make([]autonomousCorrectionRouteOperationTrace, 0, len(operations))
	seen := map[string]int{}
	for sliceIndex, operation := range operations {
		if operation.Op != transactions.OpRoute || strings.TrimSpace(operation.Net) == "" {
			continue
		}
		base := autonomousCorrectionRouteOperationID(operation)
		seen[base]++
		id := base
		if seen[base] > 1 {
			id += "-n" + strconv.Itoa(seen[base])
		}
		traces = append(traces, autonomousCorrectionRouteOperationTrace{
			ID:             id,
			SliceIndex:     sliceIndex,
			OperationIndex: operation.Index,
			Net:            strings.TrimSpace(operation.Net),
		})
	}
	return traces
}

func autonomousCorrectionRouteOperationID(operation transactions.Operation) string {
	hash := sha256.New()
	writeHashBytes(hash, []byte(operation.Op))
	writeHashBytes(hash, []byte(strings.TrimSpace(operation.Net)))
	writeHashBytes(hash, autonomousCorrectionCanonicalOperation(operation.Raw))
	return "route-" + hex.EncodeToString(hash.Sum(nil))[:16]
}

func autonomousCorrectionCanonicalOperation(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return nil
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return append([]byte(nil), raw...)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return append([]byte(nil), raw...)
	}
	return canonical
}

func correlateAutonomousCorrectionRouteOperations(issue reports.Issue, traces []autonomousCorrectionRouteOperationTrace) ([]string, []int, string) {
	var exact []autonomousCorrectionRouteOperationTrace
	exactScope := ""
	if operationID := strings.TrimSpace(issue.OperationID); operationID != "" {
		matches := correctionTracesByIdentity(operationID, traces)
		if len(matches) == 1 {
			exact = matches
			exactScope = correctionOperationScopeIdentity
		}
		if len(matches) > 1 {
			return nil, nil, correctionOperationScopeAmbiguous
		}
	}
	if len(exact) == 0 {
		operationIndex, ok := correctionOperationIndexFromPath(issue.Path)
		if !ok {
			operationIndex = -1
		}
		matches := correctionTracesBySliceIndex(operationIndex, traces)
		if len(matches) == 1 {
			exact = matches
			exactScope = correctionOperationScopePath
		}
		if len(matches) > 1 {
			return nil, nil, correctionOperationScopeAmbiguous
		}
	}
	nets := correctionSortedStrings(issue.Nets)
	if len(nets) == 0 {
		if len(exact) == 1 {
			return correctionTraceScope(exact, exactScope)
		}
		return nil, nil, correctionOperationScopeMissing
	}
	netSet := make(map[string]struct{}, len(nets))
	for _, net := range nets {
		netSet[net] = struct{}{}
	}
	if len(exact) == 1 {
		if _, ok := netSet[exact[0].Net]; !ok {
			return nil, nil, correctionOperationScopeAmbiguous
		}
	}
	matches := make([]autonomousCorrectionRouteOperationTrace, 0)
	matchedNets := map[string]struct{}{}
	for _, trace := range traces {
		if _, ok := netSet[trace.Net]; !ok {
			continue
		}
		matches = append(matches, trace)
		matchedNets[trace.Net] = struct{}{}
	}
	if len(matches) == 0 || len(matchedNets) != len(netSet) {
		return nil, nil, correctionOperationScopeMissing
	}
	if len(exact) == 1 {
		matches = append(matches, exact[0])
		matches = compactCorrectionRouteTraces(matches)
		return correctionTraceScope(matches, exactScope+"+"+correctionOperationScopeNet)
	}
	return correctionTraceScope(matches, correctionOperationScopeNet)
}

func correctionTracesByIdentity(operationID string, traces []autonomousCorrectionRouteOperationTrace) []autonomousCorrectionRouteOperationTrace {
	matches := make([]autonomousCorrectionRouteOperationTrace, 0, 1)
	for _, trace := range traces {
		if trace.ID == operationID {
			matches = append(matches, trace)
			continue
		}
		if strings.HasPrefix(operationID, "route:") {
			index, err := strconv.Atoi(strings.TrimPrefix(operationID, "route:"))
			if err == nil && trace.OperationIndex == index {
				matches = append(matches, trace)
			}
		}
	}
	return matches
}

func correctionOperationIndexFromPath(value string) (int, bool) {
	value = strings.TrimSpace(value)
	start := strings.Index(value, "operations[")
	if start < 0 {
		return 0, false
	}
	rest := value[start+len("operations["):]
	end := strings.IndexByte(rest, ']')
	if end <= 0 {
		return 0, false
	}
	index, err := strconv.Atoi(rest[:end])
	if err != nil || index < 0 {
		return 0, false
	}
	return index, true
}

func correctionTracesBySliceIndex(index int, traces []autonomousCorrectionRouteOperationTrace) []autonomousCorrectionRouteOperationTrace {
	for _, trace := range traces {
		if trace.SliceIndex == index {
			return []autonomousCorrectionRouteOperationTrace{trace}
		}
	}
	return nil
}

func correctionTraceScope(traces []autonomousCorrectionRouteOperationTrace, scope string) ([]string, []int, string) {
	ids := make([]string, 0, len(traces))
	indexes := make([]int, 0, len(traces))
	for _, trace := range traces {
		ids = append(ids, trace.ID)
		indexes = append(indexes, trace.SliceIndex)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	slices.Sort(indexes)
	indexes = slices.Compact(indexes)
	return ids, indexes, scope
}

func correctionTraceNetsForIndexes(indexes []int, traces []autonomousCorrectionRouteOperationTrace) []string {
	indexSet := make(map[int]struct{}, len(indexes))
	for _, index := range indexes {
		indexSet[index] = struct{}{}
	}
	nets := make([]string, 0, len(indexSet))
	for _, trace := range traces {
		if _, ok := indexSet[trace.SliceIndex]; ok {
			nets = append(nets, trace.Net)
		}
	}
	return correctionSortedStrings(nets)
}

func compactCorrectionRouteTraces(traces []autonomousCorrectionRouteOperationTrace) []autonomousCorrectionRouteOperationTrace {
	slices.SortFunc(traces, func(left, right autonomousCorrectionRouteOperationTrace) int {
		return left.SliceIndex - right.SliceIndex
	})
	out := traces[:0]
	last := -1
	for _, trace := range traces {
		if len(out) != 0 && trace.SliceIndex == last {
			continue
		}
		out = append(out, trace)
		last = trace.SliceIndex
	}
	return out
}

func autonomousCorrectionDiagnosticHasRoutingScope(diagnostic AutonomousCorrectionDiagnostic) bool {
	return len(diagnostic.Nets) != 0 &&
		len(diagnostic.OperationIDs) != 0 &&
		len(diagnostic.OperationIDs) == len(diagnostic.OperationIndexes) &&
		diagnostic.OperationScope != correctionOperationScopeMissing &&
		diagnostic.OperationScope != correctionOperationScopeAmbiguous
}

func autonomousCorrectionRoutingAction(kind AutonomousCorrectionActionKind) bool {
	switch kind {
	case CorrectionActionRerouteAffectedNets, CorrectionActionReorderRouteTreeBranches, CorrectionActionInsertLayerTransition:
		return true
	default:
		return false
	}
}

func autonomousCorrectionHasRoutingActions(actions []AutonomousCorrectionAction) bool {
	for _, action := range actions {
		if autonomousCorrectionRoutingAction(action.Kind) {
			return true
		}
	}
	return false
}

func autonomousCorrectionOnlyRoutingActions(actions []AutonomousCorrectionAction) bool {
	if len(actions) == 0 {
		return false
	}
	for _, action := range actions {
		if !autonomousCorrectionRoutingAction(action.Kind) {
			return false
		}
	}
	return true
}

func autonomousCorrectionRoutingScopeMatches(actions []AutonomousCorrectionAction, operations []transactions.Operation) bool {
	if !autonomousCorrectionHasRoutingActions(actions) {
		return true
	}
	if len(operations) == 0 {
		return false
	}
	traces := autonomousCorrectionRouteOperationTraces(operations)
	traceByIndex := make(map[int]autonomousCorrectionRouteOperationTrace, len(traces))
	for _, trace := range traces {
		traceByIndex[trace.SliceIndex] = trace
	}
	affectedNets := map[string]struct{}{}
	scopedIndexes := map[int]struct{}{}
	for _, action := range actions {
		if !autonomousCorrectionRoutingAction(action.Kind) {
			continue
		}
		if len(action.Nets) == 0 || len(action.OperationIDs) == 0 || len(action.OperationIDs) != len(action.OperationIndexes) {
			return false
		}
		for _, net := range action.Nets {
			affectedNets[net] = struct{}{}
		}
		idSet := make(map[string]struct{}, len(action.OperationIDs))
		for _, id := range action.OperationIDs {
			idSet[id] = struct{}{}
		}
		for _, index := range action.OperationIndexes {
			trace, ok := traceByIndex[index]
			if !ok {
				return false
			}
			if _, ok := idSet[trace.ID]; !ok {
				return false
			}
			if _, ok := affectedNets[trace.Net]; !ok {
				return false
			}
			scopedIndexes[index] = struct{}{}
		}
	}
	for _, trace := range traces {
		if _, affected := affectedNets[trace.Net]; !affected {
			continue
		}
		if _, scoped := scopedIndexes[trace.SliceIndex]; !scoped {
			return false
		}
	}
	return len(scopedIndexes) != 0
}

func autonomousCorrectionRouteStateHash(operations []transactions.Operation) string {
	hash := sha256.New()
	writeHashBytes(hash, []byte("route_state_v1"))
	writeHashBytes(hash, []byte(strconv.Itoa(len(operations))))
	for index, operation := range operations {
		writeHashBytes(hash, []byte(strconv.Itoa(index)))
		writeHashBytes(hash, []byte(operation.Op))
		writeHashBytes(hash, []byte(strings.TrimSpace(operation.Ref)))
		writeHashBytes(hash, []byte(strings.TrimSpace(operation.Net)))
		writeHashBytes(hash, autonomousCorrectionCanonicalOperation(operation.Raw))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func autonomousCorrectionAffectedNets(actions []AutonomousCorrectionAction) []string {
	nets := make([]string, 0)
	for _, action := range actions {
		if autonomousCorrectionRoutingAction(action.Kind) {
			nets = append(nets, action.Nets...)
		}
	}
	return correctionSortedStrings(nets)
}

// ApplyAutonomousRoutingCorrectionPlan performs an in-memory selective route
// replacement. Unaffected operations become fixed net-aware obstacles and are
// spliced back without changing their raw bytes or relative order.
func ApplyAutonomousRoutingCorrectionPlan(ctx context.Context, request Request, current RoutingStageResult, plan AutonomousCorrectionPlan, appliedRetryKeys []string) (RoutingStageResult, AutonomousCorrectionApplication, error) {
	candidate := current
	application := AutonomousCorrectionApplication{RetryKey: plan.RetryKey}
	if ctx == nil {
		ctx = context.Background()
	}
	if !IsGenericAutonomousCorrectionRequest(request) || !plan.Authorized || plan.StopReason != "" || !autonomousCorrectionHasRoutingActions(plan.Actions) {
		application.StopReason = CorrectionStopPlanNotAuthorized
		return candidate, application, nil
	}
	fingerprint, err := AutonomousCorrectionInvariantFingerprint(request)
	if err != nil {
		return candidate, application, err
	}
	application.InvariantFingerprintBefore = fingerprint
	application.InvariantFingerprintAfter = fingerprint
	application.RouteStateHashBefore = autonomousCorrectionRouteStateHash(current.Operations)
	if fingerprint != plan.InvariantFingerprint || plan.RouteStateHash == "" || application.RouteStateHashBefore != plan.RouteStateHash {
		application.StopReason = CorrectionStopInvariantMismatch
		return candidate, application, nil
	}
	if plan.RetryKey == "" || slices.Contains(appliedRetryKeys, plan.RetryKey) {
		application.StopReason = CorrectionStopRepeatedRetryKey
		return candidate, application, nil
	}
	if !autonomousCorrectionRoutingScopeMatches(plan.Actions, current.Operations) {
		application.StopReason = CorrectionStopRouteOperationScopeMismatch
		return candidate, application, nil
	}

	affectedNets := autonomousCorrectionAffectedNets(plan.Actions)
	affectedSet := make(map[string]struct{}, len(affectedNets))
	for _, net := range affectedNets {
		affectedSet[net] = struct{}{}
	}
	preserved := make([]transactions.Operation, 0, len(current.Operations))
	firstAffected := -1
	replacedCount := 0
	for index, operation := range current.Operations {
		if operation.Op == transactions.OpRoute {
			if _, affected := affectedSet[strings.TrimSpace(operation.Net)]; affected {
				if firstAffected < 0 {
					firstAffected = index
				}
				replacedCount++
				continue
			}
		}
		preserved = append(preserved, operation.Clone())
	}
	if firstAffected < 0 || replacedCount == 0 {
		application.StopReason = CorrectionStopRouteOperationScopeMismatch
		return candidate, application, nil
	}
	application.AffectedNets = slices.Clone(affectedNets)
	application.ReplacedOperationCount = replacedCount
	application.PreservedOperationCount = len(preserved)
	application.RoutePreservationBefore = autonomousCorrectionRouteStateHash(preserved)

	baseRoutingRequest := current.CorrectionRequest
	if len(baseRoutingRequest.Nets) == 0 {
		baseRoutingRequest = current.Request
	}
	rerouteRequest := baseRoutingRequest
	rerouteRequest.Nets = nil
	for _, net := range current.Request.Nets {
		if _, affected := affectedSet[strings.TrimSpace(net.Name)]; affected {
			rerouteRequest.Nets = append(rerouteRequest.Nets, net)
		}
	}
	if len(rerouteRequest.Nets) != len(affectedNets) {
		application.StopReason = CorrectionStopRouteOperationScopeMismatch
		return candidate, application, nil
	}
	rerouteRequest.Existing = append([]routing.ExistingCopper(nil), current.Request.Existing...)
	rerouteRequest.Existing = append(rerouteRequest.Existing, existingCopperFromAllRouteOperations(preserved, routeBranchDefaultLayer(rerouteRequest.Board), rerouteRequest.Rules)...)
	rerouteRequest.Strategy.PreserveExisting = true

	var replacements []transactions.Operation
	var replacementIssues []reports.Issue
	directTransition := false
	if autonomousCorrectionPlanHasAction(plan.Actions, CorrectionActionInsertLayerTransition) && autonomousCorrectionViaPolicyAllows(rerouteRequest, affectedSet) {
		affectedOperations := autonomousCorrectionAffectedOperations(current.Operations, affectedSet)
		transitioned, inserted, transitionIssues := ensureRouteLayerJunctionVias(affectedOperations, rerouteRequest.Rules)
		if inserted > 0 && !reports.HasBlockingIssue(transitionIssues) {
			trial := spliceAutonomousCorrectionRouteOperations(current.Operations, affectedSet, firstAffected, transitioned)
			trialRoutes := routingRoutesFromOperations(trial)
			validation := routing.ValidateResult(baseRoutingRequest, routing.Result{Status: routing.StatusRouted, Routes: trialRoutes})
			validation.Issues = append(validation.Issues, routing.ValidatePhysicalClearance(baseRoutingRequest, trialRoutes)...)
			if !reports.HasBlockingIssue(validation.Issues) {
				replacements = transitioned
				replacementIssues = transitionIssues
				directTransition = true
			}
		}
	}
	if !directTransition {
		rerouteResult := routing.RouteRequestContext(ctx, rerouteRequest)
		replacements = transactionRouteOperations(rerouteResult.Operations)
		replacementIssues = append(replacementIssues, rerouteResult.Issues...)
		if autonomousCorrectionPlanHasAction(plan.Actions, CorrectionActionInsertLayerTransition) && autonomousCorrectionViaPolicyAllows(rerouteRequest, affectedSet) {
			var transitionIssues []reports.Issue
			replacements, _, transitionIssues = ensureRouteLayerJunctionVias(replacements, rerouteRequest.Rules)
			replacementIssues = append(replacementIssues, transitionIssues...)
		}
		if rerouteResult.Status != routing.StatusRouted || reports.HasBlockingIssue(replacementIssues) || len(replacements) == 0 {
			application.StopReason = CorrectionStopRouteReplacementInvalid
			application.ValidationIssues = cloneIssues(replacementIssues)
			return candidate, application, nil
		}
	}

	merged := spliceAutonomousCorrectionRouteOperations(current.Operations, affectedSet, firstAffected, replacements)
	mergedRoutes := routingRoutesFromOperations(merged)
	fullValidation := routing.ValidateResult(baseRoutingRequest, routing.Result{Status: routing.StatusRouted, Routes: mergedRoutes})
	fullValidation.Issues = append(fullValidation.Issues, routing.ValidatePhysicalClearance(baseRoutingRequest, mergedRoutes)...)
	if reports.HasBlockingIssue(fullValidation.Issues) {
		application.StopReason = CorrectionStopRouteReplacementInvalid
		application.ValidationIssues = cloneIssues(fullValidation.Issues)
		return candidate, application, nil
	}
	afterPreserved := autonomousCorrectionPreservedOperations(merged, affectedSet)
	application.RoutePreservationAfter = autonomousCorrectionRouteStateHash(afterPreserved)
	if application.RoutePreservationBefore != application.RoutePreservationAfter {
		application.StopReason = CorrectionStopInvariantMismatch
		return candidate, application, nil
	}

	candidate.Operations = merged
	candidate.Request = current.Request
	candidate.CorrectionRequest = baseRoutingRequest
	candidate.Result = current.Result
	candidate.Result.Routes = mergedRoutes
	candidate.Result.Issues = cloneIssues(replacementIssues)
	candidate.Result.Status = routing.StatusRouted
	candidate.Result.Metrics.NetCount = len(baseRoutingRequest.Nets)
	candidate.Result.Metrics.RoutedNetCount = len(baseRoutingRequest.Nets)
	candidate.Result.Metrics.FailedNetCount = 0
	quality := routing.BuildQualityReport(baseRoutingRequest, candidate.Result)
	candidate.Result.Quality = &quality
	remainingIssues := autonomousCorrectionUnrelatedIssues(current.Stage.Issues, affectedSet)
	remainingIssues = append(remainingIssues, replacementIssues...)
	remainingIssues = append(remainingIssues, fullValidation.Issues...)
	candidate.Stage = NewStageResult(StageRouting, remainingIssues)
	candidate.Stage.Summary = cloneSummaryMap(current.Stage.Summary)
	ensureStageSummary(&candidate.Stage)
	candidate.Stage.Summary["selective_route_correction"] = map[string]any{
		"affected_nets": affectedNets, "replaced_operations": replacedCount,
		"preserved_operations": len(preserved), "preservation_fingerprint": application.RoutePreservationAfter,
		"direct_layer_transition": directTransition,
	}
	application.RouteStateHashAfter = autonomousCorrectionRouteStateHash(candidate.Operations)
	application.Applied = true
	application.ProtectedInvariantsPreserved = true
	return candidate, application, nil
}

func autonomousCorrectionViaPolicyAllows(request routing.Request, affected map[string]struct{}) bool {
	if request.Strategy.Mode == routing.ModeSingleLayer ||
		request.Rules.AllowVias != nil && !*request.Rules.AllowVias ||
		request.Rules.AllowBackLayer != nil && !*request.Rules.AllowBackLayer {
		return false
	}
	for _, net := range request.Nets {
		if _, ok := affected[strings.TrimSpace(net.Name)]; !ok {
			continue
		}
		effective, issues := routing.ResolveNetRule(&request, net)
		if reports.HasBlockingIssue(issues) || effective.MaxViasPerNet < 1 {
			return false
		}
		if len(effective.AllowedLayers) == 1 {
			return false
		}
	}
	return true
}

func autonomousCorrectionAffectedOperations(operations []transactions.Operation, affected map[string]struct{}) []transactions.Operation {
	out := make([]transactions.Operation, 0, len(operations))
	for _, operation := range operations {
		if operation.Op != transactions.OpRoute {
			continue
		}
		if _, ok := affected[strings.TrimSpace(operation.Net)]; ok {
			out = append(out, operation.Clone())
		}
	}
	return out
}

func autonomousCorrectionPlanHasAction(actions []AutonomousCorrectionAction, kind AutonomousCorrectionActionKind) bool {
	for _, action := range actions {
		if action.Kind == kind {
			return true
		}
	}
	return false
}

func spliceAutonomousCorrectionRouteOperations(current []transactions.Operation, affected map[string]struct{}, firstAffected int, replacements []transactions.Operation) []transactions.Operation {
	out := make([]transactions.Operation, 0, len(current)-1+len(replacements))
	inserted := false
	for index, operation := range current {
		if operation.Op == transactions.OpRoute {
			if _, remove := affected[strings.TrimSpace(operation.Net)]; remove {
				if !inserted && index >= firstAffected {
					for _, replacement := range replacements {
						out = append(out, replacement.Clone())
					}
					inserted = true
				}
				continue
			}
		}
		out = append(out, operation.Clone())
	}
	if !inserted {
		for _, replacement := range replacements {
			out = append(out, replacement.Clone())
		}
	}
	return out
}

func autonomousCorrectionPreservedOperations(operations []transactions.Operation, affected map[string]struct{}) []transactions.Operation {
	preserved := make([]transactions.Operation, 0, len(operations))
	for _, operation := range operations {
		if operation.Op == transactions.OpRoute {
			if _, changed := affected[strings.TrimSpace(operation.Net)]; changed {
				continue
			}
		}
		preserved = append(preserved, operation.Clone())
	}
	return preserved
}

func autonomousCorrectionUnrelatedIssues(issues []reports.Issue, affected map[string]struct{}) []reports.Issue {
	out := make([]reports.Issue, 0, len(issues))
	for _, issue := range issues {
		if len(issue.Nets) == 0 {
			out = append(out, issue)
			continue
		}
		related := true
		for _, net := range issue.Nets {
			if _, ok := affected[strings.TrimSpace(net)]; !ok {
				related = false
				break
			}
		}
		if !related {
			out = append(out, issue)
		}
	}
	return out
}

func cloneSummaryMap(summary map[string]any) map[string]any {
	if summary == nil {
		return map[string]any{}
	}
	clone := make(map[string]any, len(summary))
	for key, value := range summary {
		clone[key] = value
	}
	return clone
}

func autonomousCorrectionPlacementAffectedNets(plan AutonomousCorrectionPlan, routingRequest routing.Request, before, after []placement.PlacementResult) []string {
	nets := make([]string, 0)
	for _, action := range plan.Actions {
		nets = append(nets, action.Nets...)
	}
	beforeByRef := make(map[string]placement.PlacementResult, len(before))
	for _, result := range before {
		beforeByRef[strings.TrimSpace(result.Ref)] = result
	}
	movedRefs := map[string]struct{}{}
	for _, result := range after {
		ref := strings.TrimSpace(result.Ref)
		previous, ok := beforeByRef[ref]
		if !ok || previous.Position.XMM != result.Position.XMM ||
			previous.Position.YMM != result.Position.YMM ||
			previous.Position.RotationDeg != result.Position.RotationDeg ||
			!strings.EqualFold(strings.TrimSpace(previous.Position.Layer), strings.TrimSpace(result.Position.Layer)) {
			movedRefs[ref] = struct{}{}
		}
	}
	for _, net := range routingRequest.Nets {
		for _, endpoint := range net.Endpoints {
			if _, moved := movedRefs[strings.TrimSpace(endpoint.Ref)]; moved {
				nets = append(nets, net.Name)
				break
			}
		}
	}
	return correctionSortedStrings(nets)
}

func preserveAutonomousCorrectionUnaffectedRoutes(current, rerouted RoutingStageResult, affectedNets []string) (RoutingStageResult, bool) {
	if len(affectedNets) == 0 {
		return RoutingStageResult{}, false
	}
	affected := make(map[string]struct{}, len(affectedNets))
	for _, net := range affectedNets {
		affected[net] = struct{}{}
	}
	replacements := autonomousCorrectionAffectedOperations(rerouted.Operations, affected)
	if len(replacements) == 0 {
		return RoutingStageResult{}, false
	}
	firstAffected := len(current.Operations)
	for index, operation := range current.Operations {
		if operation.Op != transactions.OpRoute {
			continue
		}
		if _, ok := affected[strings.TrimSpace(operation.Net)]; ok {
			firstAffected = index
			break
		}
	}
	preservedBefore := autonomousCorrectionPreservedOperations(current.Operations, affected)
	merged := spliceAutonomousCorrectionRouteOperations(current.Operations, affected, firstAffected, replacements)
	preservedAfter := autonomousCorrectionPreservedOperations(merged, affected)
	if autonomousCorrectionRouteStateHash(preservedBefore) != autonomousCorrectionRouteStateHash(preservedAfter) {
		return RoutingStageResult{}, false
	}
	baseRequest := rerouted.CorrectionRequest
	if len(baseRequest.Nets) == 0 {
		baseRequest = rerouted.Request
	}
	routes := routingRoutesFromOperations(merged)
	validation := routing.ValidateResult(baseRequest, routing.Result{Status: routing.StatusRouted, Routes: routes})
	validation.Issues = append(validation.Issues, routing.ValidatePhysicalClearance(baseRequest, routes)...)
	if reports.HasBlockingIssue(validation.Issues) {
		return RoutingStageResult{}, false
	}
	candidate := rerouted
	candidate.Operations = merged
	candidate.Result.Routes = routes
	candidate.Result.Issues = append(autonomousCorrectionUnrelatedIssues(rerouted.Stage.Issues, affected), validation.Issues...)
	candidate.Result.Status = routing.StatusRouted
	candidate.Result.Metrics.NetCount = len(baseRequest.Nets)
	candidate.Result.Metrics.RoutedNetCount = len(baseRequest.Nets)
	candidate.Result.Metrics.FailedNetCount = 0
	quality := routing.BuildQualityReport(baseRequest, candidate.Result)
	candidate.Result.Quality = &quality
	candidate.Stage = NewStageResult(StageRouting, candidate.Result.Issues)
	candidate.Stage.Summary = cloneSummaryMap(rerouted.Stage.Summary)
	ensureStageSummary(&candidate.Stage)
	candidate.Stage.Summary["placement_selective_reroute"] = map[string]any{
		"affected_nets":            affectedNets,
		"preserved_operations":     len(preservedBefore),
		"preservation_fingerprint": autonomousCorrectionRouteStateHash(preservedAfter),
	}
	return candidate, true
}
