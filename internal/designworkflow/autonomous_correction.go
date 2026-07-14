package designworkflow

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

const AutonomousCorrectionSchemaV1 = "kicadai.autonomous-correction.v1"

type AutonomousCorrectionCategory string

const (
	CorrectionComponentOverlap                AutonomousCorrectionCategory = "component_overlap"
	CorrectionInaccessiblePad                 AutonomousCorrectionCategory = "inaccessible_pad"
	CorrectionBlockedEscapeDirection          AutonomousCorrectionCategory = "blocked_escape_direction"
	CorrectionRouteTreeBranchOrder            AutonomousCorrectionCategory = "route_tree_branch_order"
	CorrectionMissingLayerTransition          AutonomousCorrectionCategory = "missing_layer_transition"
	CorrectionSameNetBranchMerge              AutonomousCorrectionCategory = "same_net_branch_merge"
	CorrectionRequiredNetDisconnectedEndpoint AutonomousCorrectionCategory = "required_net_disconnected_endpoint"
	CorrectionRoutingRegionExhaustion         AutonomousCorrectionCategory = "routing_region_exhaustion"
	CorrectionUnsupportedGeometry             AutonomousCorrectionCategory = "unsupported_geometry"
)

type AutonomousCorrectionDiagnostic struct {
	Category          AutonomousCorrectionCategory `json:"category"`
	Source            string                       `json:"source"`
	SourceCategory    routing.RepairCategory       `json:"source_category,omitempty"`
	SourceAction      routing.RepairAction         `json:"source_action,omitempty"`
	IssueCode         reports.Code                 `json:"issue_code"`
	Severity          reports.Severity             `json:"severity"`
	Path              string                       `json:"path,omitempty"`
	Refs              []string                     `json:"refs,omitempty"`
	Nets              []string                     `json:"nets,omitempty"`
	Evidence          []string                     `json:"evidence,omitempty"`
	AutomaticAction   bool                         `json:"automatic_action"`
	UnsupportedReason string                       `json:"unsupported_reason,omitempty"`
}

type AutonomousCorrectionActionKind string

const (
	CorrectionActionAdjustRelativeSpacing    AutonomousCorrectionActionKind = "adjust_relative_spacing"
	CorrectionActionMoveWithinRegion         AutonomousCorrectionActionKind = "move_within_declared_region"
	CorrectionActionImproveEndpointFanout    AutonomousCorrectionActionKind = "improve_endpoint_fanout"
	CorrectionActionReduceEndpointDistance   AutonomousCorrectionActionKind = "reduce_endpoint_distance"
	CorrectionActionRebuildRouteTree         AutonomousCorrectionActionKind = "rebuild_route_tree"
	CorrectionActionReorderRouteTreeBranches AutonomousCorrectionActionKind = "reorder_route_tree_branches"
	CorrectionActionInsertLayerTransition    AutonomousCorrectionActionKind = "insert_layer_transition"
)

type AutonomousCorrectionAction struct {
	Kind          AutonomousCorrectionActionKind `json:"kind"`
	Category      AutonomousCorrectionCategory   `json:"category"`
	Refs          []string                       `json:"refs,omitempty"`
	Nets          []string                       `json:"nets,omitempty"`
	PlacementHint PlacementRetryHintCategory     `json:"placement_hint,omitempty"`
	Authorized    bool                           `json:"authorized"`
	Reason        string                         `json:"reason,omitempty"`
}

type AutonomousCorrectionPlan struct {
	SchemaVersion        string                           `json:"schema_version"`
	Attempt              int                              `json:"attempt"`
	MaxAttempts          int                              `json:"max_attempts"`
	RetryKey             string                           `json:"retry_key,omitempty"`
	InvariantFingerprint string                           `json:"invariant_fingerprint,omitempty"`
	PlacementStateHash   string                           `json:"placement_state_hash,omitempty"`
	Diagnostics          []AutonomousCorrectionDiagnostic `json:"diagnostics,omitempty"`
	Actions              []AutonomousCorrectionAction     `json:"actions,omitempty"`
	Authorized           bool                             `json:"authorized"`
	StopReason           string                           `json:"stop_reason,omitempty"`
}

type AutonomousCorrectionPlanOptions struct {
	Attempt          int
	MaxAttempts      int
	AppliedRetryKeys []string
}

const (
	CorrectionStopNotGeneric              = "not_generic_circuit"
	CorrectionStopNotRequired             = "correction_not_required"
	CorrectionStopBudgetExhausted         = "budget_exhausted"
	CorrectionStopUnsupportedDiagnostic   = "unsupported_diagnostic"
	CorrectionStopAmbiguousDiagnostics    = "ambiguous_diagnostics"
	CorrectionStopFixedConstraintConflict = "fixed_constraint_conflict"
	CorrectionStopRepeatedRetryKey        = "repeated_retry_key"
)

// PlanAutonomousCorrection is pure: it derives an authorized plan without
// mutating the design request, placement request, placements, or diagnostics.
func PlanAutonomousCorrection(request Request, placementRequest placement.Request, placements []placement.PlacementResult, diagnostics []AutonomousCorrectionDiagnostic, opts AutonomousCorrectionPlanOptions) (AutonomousCorrectionPlan, error) {
	plan := AutonomousCorrectionPlan{
		SchemaVersion: AutonomousCorrectionSchemaV1,
		Attempt:       opts.Attempt,
		MaxAttempts:   opts.MaxAttempts,
		Diagnostics:   slices.Clone(diagnostics),
	}
	slices.SortFunc(plan.Diagnostics, compareAutonomousCorrectionDiagnostic)
	if !IsGenericAutonomousCorrectionRequest(request) {
		plan.StopReason = CorrectionStopNotGeneric
		return plan, nil
	}
	if plan.Attempt < 2 {
		plan.Attempt = 2
	}
	if plan.MaxAttempts < 1 || plan.Attempt > plan.MaxAttempts {
		plan.StopReason = CorrectionStopBudgetExhausted
		return plan, nil
	}
	fingerprint, err := AutonomousCorrectionInvariantFingerprint(request)
	if err != nil {
		return plan, err
	}
	plan.InvariantFingerprint = fingerprint
	plan.PlacementStateHash = placementStateHash(placements)
	blocking := blockingAutonomousCorrectionDiagnostics(plan.Diagnostics)
	if len(blocking) == 0 {
		plan.StopReason = CorrectionStopNotRequired
		return plan, nil
	}
	for _, diagnostic := range blocking {
		if !diagnostic.AutomaticAction {
			plan.Actions = append(plan.Actions, autonomousCorrectionActionForDiagnostic(diagnostic))
			plan.StopReason = CorrectionStopUnsupportedDiagnostic
			return plan, nil
		}
		plan.Actions = append(plan.Actions, autonomousCorrectionActionsForDiagnostic(diagnostic)...)
	}
	plan.Actions = normalizeAutonomousCorrectionActions(plan.Actions)
	for _, action := range plan.Actions {
		if !action.Authorized {
			plan.StopReason = CorrectionStopUnsupportedDiagnostic
			return plan, nil
		}
	}
	if autonomousCorrectionActionsAmbiguous(plan.Actions) {
		plan.StopReason = CorrectionStopAmbiguousDiagnostics
		return plan, nil
	}
	if autonomousCorrectionActionsFixed(placementRequest.Components, plan.Actions) {
		plan.StopReason = CorrectionStopFixedConstraintConflict
		return plan, nil
	}
	actionKinds := make([]string, 0, len(plan.Actions))
	for _, action := range plan.Actions {
		actionKinds = append(actionKinds, string(action.Kind))
	}
	plan.RetryKey = AutonomousCorrectionRetryKey(plan.Diagnostics, actionKinds, plan.InvariantFingerprint, plan.PlacementStateHash)
	if slices.Contains(opts.AppliedRetryKeys, plan.RetryKey) {
		plan.StopReason = CorrectionStopRepeatedRetryKey
		return plan, nil
	}
	plan.Authorized = len(plan.Actions) > 0
	if !plan.Authorized {
		plan.StopReason = CorrectionStopUnsupportedDiagnostic
	}
	return plan, nil
}

func blockingAutonomousCorrectionDiagnostics(diagnostics []AutonomousCorrectionDiagnostic) []AutonomousCorrectionDiagnostic {
	result := make([]AutonomousCorrectionDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == reports.SeverityBlocked || diagnostic.Severity == reports.SeverityError {
			result = append(result, diagnostic)
		}
	}
	return result
}

func autonomousCorrectionActionsForDiagnostic(diagnostic AutonomousCorrectionDiagnostic) []AutonomousCorrectionAction {
	action := autonomousCorrectionActionForDiagnostic(diagnostic)
	if !action.Authorized {
		return []AutonomousCorrectionAction{action}
	}
	actions := []AutonomousCorrectionAction{action}
	if diagnostic.Category == CorrectionSameNetBranchMerge {
		actions = append(actions, AutonomousCorrectionAction{
			Kind: CorrectionActionRebuildRouteTree, Category: diagnostic.Category,
			Refs: slices.Clone(diagnostic.Refs), Nets: slices.Clone(diagnostic.Nets),
			Authorized: true, Reason: "rerun deterministic route-tree construction after endpoint-access correction",
		})
	}
	return actions
}

func autonomousCorrectionActionForDiagnostic(diagnostic AutonomousCorrectionDiagnostic) AutonomousCorrectionAction {
	action := AutonomousCorrectionAction{
		Category: diagnostic.Category,
		Refs:     slices.Clone(diagnostic.Refs),
		Nets:     slices.Clone(diagnostic.Nets),
	}
	switch diagnostic.Category {
	case CorrectionComponentOverlap:
		action.Kind, action.PlacementHint, action.Authorized = CorrectionActionAdjustRelativeSpacing, PlacementRetryIncreaseSpacing, true
	case CorrectionInaccessiblePad:
		action.Kind, action.PlacementHint, action.Authorized = CorrectionActionImproveEndpointFanout, PlacementRetryImproveFanout, true
	case CorrectionBlockedEscapeDirection:
		action.Kind, action.PlacementHint, action.Authorized = CorrectionActionMoveWithinRegion, PlacementRetryMoveFromEdge, true
	case CorrectionSameNetBranchMerge:
		action.Kind, action.PlacementHint, action.Authorized = CorrectionActionImproveEndpointFanout, PlacementRetryImproveFanout, true
	case CorrectionRequiredNetDisconnectedEndpoint:
		switch diagnostic.SourceCategory {
		case routing.RepairPadAccess, routing.RepairLayerAccess:
			action.Kind, action.PlacementHint, action.Authorized = CorrectionActionImproveEndpointFanout, PlacementRetryImproveFanout, true
		case routing.RepairClearance:
			action.Kind, action.PlacementHint, action.Authorized = CorrectionActionAdjustRelativeSpacing, PlacementRetryIncreaseSpacing, true
		default:
			action.Kind, action.PlacementHint, action.Authorized = CorrectionActionReduceEndpointDistance, PlacementRetryReduceDistance, true
		}
	case CorrectionRoutingRegionExhaustion:
		if diagnostic.SourceCategory == routing.RepairLengthPolicy {
			action.Kind, action.PlacementHint, action.Authorized = CorrectionActionReduceEndpointDistance, PlacementRetryReduceDistance, true
		} else {
			action.Kind, action.PlacementHint, action.Authorized = CorrectionActionAdjustRelativeSpacing, PlacementRetryIncreaseSpacing, true
		}
	case CorrectionRouteTreeBranchOrder:
		action.Kind, action.Reason = CorrectionActionReorderRouteTreeBranches, "route-tree branch reordering is reserved for a future correction contract"
	case CorrectionMissingLayerTransition:
		action.Kind, action.Reason = CorrectionActionInsertLayerTransition, "layer-transition insertion is not authorized in v1"
	default:
		action.Reason = "no deterministic correction is authorized for this geometry"
	}
	return action
}

func normalizeAutonomousCorrectionActions(actions []AutonomousCorrectionAction) []AutonomousCorrectionAction {
	result := make([]AutonomousCorrectionAction, 0, len(actions))
	seen := map[string]struct{}{}
	for _, action := range actions {
		action.Refs = correctionSortedStrings(action.Refs)
		action.Nets = correctionSortedStrings(action.Nets)
		key := autonomousCorrectionActionKey(action)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, action)
	}
	slices.SortFunc(result, func(left, right AutonomousCorrectionAction) int {
		if value := cmp.Compare(left.Kind, right.Kind); value != 0 {
			return value
		}
		if value := cmp.Compare(left.Category, right.Category); value != 0 {
			return value
		}
		if value := slices.Compare(left.Refs, right.Refs); value != 0 {
			return value
		}
		return slices.Compare(left.Nets, right.Nets)
	})
	return result
}

func autonomousCorrectionActionKey(action AutonomousCorrectionAction) string {
	hash := sha256.New()
	writeHashBytes(hash, []byte(action.Kind))
	writeHashBytes(hash, []byte(action.Category))
	writeHashBytes(hash, []byte(action.PlacementHint))
	writeHashBytes(hash, []byte("refs:"+strconv.Itoa(len(action.Refs))))
	for _, ref := range action.Refs {
		writeHashBytes(hash, []byte(ref))
	}
	writeHashBytes(hash, []byte("nets:"+strconv.Itoa(len(action.Nets))))
	for _, net := range action.Nets {
		writeHashBytes(hash, []byte(net))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func autonomousCorrectionActionsAmbiguous(actions []AutonomousCorrectionAction) bool {
	byNet := map[string]map[AutonomousCorrectionActionKind]struct{}{}
	for _, action := range actions {
		if action.Kind != CorrectionActionAdjustRelativeSpacing && action.Kind != CorrectionActionReduceEndpointDistance {
			continue
		}
		for _, net := range action.Nets {
			if byNet[net] == nil {
				byNet[net] = map[AutonomousCorrectionActionKind]struct{}{}
			}
			byNet[net][action.Kind] = struct{}{}
		}
	}
	for _, kinds := range byNet {
		if len(kinds) > 1 {
			return true
		}
	}
	return false
}

func autonomousCorrectionActionsFixed(components []placement.Component, actions []AutonomousCorrectionAction) bool {
	movable, _ := placementRetryMobilityRefs(components)
	if len(movable) == 0 {
		return true
	}
	for _, action := range actions {
		if action.Kind == CorrectionActionRebuildRouteTree {
			continue
		}
		if len(action.Refs) == 0 {
			continue
		}
		actionMovable := false
		for _, ref := range action.Refs {
			if _, ok := movable[ref]; ok {
				actionMovable = true
				break
			}
		}
		if !actionMovable {
			return true
		}
	}
	return false
}

// BuildAutonomousCorrectionDiagnostics converts subsystem issues into the
// stable correction taxonomy. Messages and suggestions are intentionally not
// copied because provider or external-tool text is not correction evidence.
func BuildAutonomousCorrectionDiagnostics(placementIssues, routingIssues []reports.Issue) []AutonomousCorrectionDiagnostic {
	diagnostics := make([]AutonomousCorrectionDiagnostic, 0, len(placementIssues)+len(routingIssues))
	for _, issue := range placementIssues {
		diagnostics = append(diagnostics, autonomousCorrectionDiagnostic("placement", issue))
	}
	for _, issue := range routingIssues {
		diagnostics = append(diagnostics, autonomousCorrectionDiagnostic("routing", issue))
	}
	diagnostics = dedupeAutonomousCorrectionDiagnostics(diagnostics)
	slices.SortFunc(diagnostics, compareAutonomousCorrectionDiagnostic)
	return diagnostics
}

func autonomousCorrectionDiagnostic(source string, issue reports.Issue) AutonomousCorrectionDiagnostic {
	sourceDiagnostic := routing.DiagnosticForIssue(issue)
	category := autonomousCorrectionCategory(issue, sourceDiagnostic)
	diagnostic := AutonomousCorrectionDiagnostic{
		Category:       category,
		Source:         source,
		SourceCategory: sourceDiagnostic.Category,
		SourceAction:   sourceDiagnostic.Action,
		IssueCode:      issue.Code,
		Severity:       issue.Severity,
		Path:           normalizeAutonomousCorrectionPath(issue.Path),
		Refs:           correctionSortedStrings(issue.Refs),
		Nets:           correctionSortedStrings(issue.Nets),
		Evidence:       autonomousCorrectionEvidence(issue, sourceDiagnostic),
	}
	diagnostic.AutomaticAction, diagnostic.UnsupportedReason = autonomousCorrectionSupport(diagnostic)
	return diagnostic
}

func autonomousCorrectionCategory(issue reports.Issue, diagnostic routing.RepairDiagnostic) AutonomousCorrectionCategory {
	switch issue.Code {
	case reports.CodePlacementCollision:
		return CorrectionComponentOverlap
	case reports.CodePlacementOutsideBoard:
		return CorrectionBlockedEscapeDirection
	case reports.CodeRouteContactLayerMismatch:
		return CorrectionMissingLayerTransition
	case reports.CodeRouteContactAmbiguous, reports.CodeRouteContactUnsupported:
		return CorrectionUnsupportedGeometry
	case reports.CodeRouteGraphIncomplete:
		return CorrectionSameNetBranchMerge
	case reports.CodeDisconnectedPad, reports.CodeRouteContactMissingTarget, reports.CodeRouteContactMiss, reports.CodeRouteCompletionPartial:
		return CorrectionRequiredNetDisconnectedEndpoint
	}
	if strings.Contains(normalizeAutonomousCorrectionPath(issue.Path), "branches[") && diagnostic.Category == routing.RepairRouteSearch {
		return CorrectionRouteTreeBranchOrder
	}
	switch diagnostic.Category {
	case routing.RepairPadAccess:
		return CorrectionInaccessiblePad
	case routing.RepairBoardBoundary:
		return CorrectionBlockedEscapeDirection
	case routing.RepairLayerAccess, routing.RepairViaPolicy:
		return CorrectionMissingLayerTransition
	case routing.RepairRouteSearch, routing.RepairClearance, routing.RepairLengthPolicy:
		return CorrectionRoutingRegionExhaustion
	case routing.RepairConnectivity:
		return CorrectionRequiredNetDisconnectedEndpoint
	default:
		return CorrectionUnsupportedGeometry
	}
}

func autonomousCorrectionSupport(diagnostic AutonomousCorrectionDiagnostic) (bool, string) {
	switch diagnostic.Category {
	case CorrectionComponentOverlap, CorrectionInaccessiblePad, CorrectionBlockedEscapeDirection, CorrectionRoutingRegionExhaustion:
		return true, ""
	case CorrectionSameNetBranchMerge:
		if len(diagnostic.Refs) >= 1 && len(diagnostic.Nets) >= 1 {
			return true, ""
		}
		return false, "same-net merge correction requires resolved refs and nets"
	case CorrectionRequiredNetDisconnectedEndpoint:
		if len(diagnostic.Refs) >= 2 && len(diagnostic.Nets) >= 1 {
			switch diagnostic.SourceCategory {
			case routing.RepairRouteSearch, routing.RepairLengthPolicy, routing.RepairPadAccess, routing.RepairLayerAccess, routing.RepairClearance, routing.RepairConnectivity:
				return true, ""
			}
		}
		return false, "disconnected endpoint correction requires an unambiguous source category, net, and endpoint refs"
	case CorrectionRouteTreeBranchOrder:
		return false, "route-tree branch reordering is reserved for a future correction contract"
	case CorrectionMissingLayerTransition:
		return false, "layer-transition insertion and relocation are not authorized in v1"
	default:
		return false, "no deterministic correction is authorized for this geometry"
	}
}

func autonomousCorrectionEvidence(issue reports.Issue, diagnostic routing.RepairDiagnostic) []string {
	evidence := []string{
		"issue_code:" + string(issue.Code),
		"source_category:" + string(diagnostic.Category),
		"source_action:" + string(diagnostic.Action),
	}
	return correctionSortedStrings(evidence)
}

func normalizeAutonomousCorrectionPath(value string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(value), `\`, "/")
	if normalized == "" {
		return ""
	}
	if autonomousCorrectionPathIsAbsolute(normalized) {
		normalized = path.Base(strings.TrimRight(normalized, "/"))
	}
	normalized = path.Clean(normalized)
	if normalized == "." {
		return ""
	}
	return normalized
}

func autonomousCorrectionPathIsAbsolute(value string) bool {
	if strings.HasPrefix(value, "/") {
		return true
	}
	return len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && value[2] == '/'
}

func correctionSortedStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	slices.Sort(result)
	if len(result) == 0 {
		return nil
	}
	return result
}

func dedupeAutonomousCorrectionDiagnostics(diagnostics []AutonomousCorrectionDiagnostic) []AutonomousCorrectionDiagnostic {
	seen := map[string]struct{}{}
	result := make([]AutonomousCorrectionDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		key := autonomousCorrectionDiagnosticKey(diagnostic)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, diagnostic)
	}
	return result
}

func autonomousCorrectionDiagnosticKey(diagnostic AutonomousCorrectionDiagnostic) string {
	hash := sha256.New()
	writeHashBytes(hash, []byte(diagnostic.Category))
	writeHashBytes(hash, []byte(diagnostic.Source))
	writeHashBytes(hash, []byte(diagnostic.SourceCategory))
	writeHashBytes(hash, []byte(diagnostic.SourceAction))
	writeHashBytes(hash, []byte(diagnostic.IssueCode))
	writeHashBytes(hash, []byte(diagnostic.Path))
	writeHashBytes(hash, []byte("refs:"+strconv.Itoa(len(diagnostic.Refs))))
	for _, ref := range diagnostic.Refs {
		writeHashBytes(hash, []byte(ref))
	}
	writeHashBytes(hash, []byte("nets:"+strconv.Itoa(len(diagnostic.Nets))))
	for _, net := range diagnostic.Nets {
		writeHashBytes(hash, []byte(net))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func compareAutonomousCorrectionDiagnostic(left, right AutonomousCorrectionDiagnostic) int {
	if value := cmp.Compare(left.Category, right.Category); value != 0 {
		return value
	}
	if value := cmp.Compare(left.Source, right.Source); value != 0 {
		return value
	}
	if value := cmp.Compare(left.SourceCategory, right.SourceCategory); value != 0 {
		return value
	}
	if value := cmp.Compare(left.SourceAction, right.SourceAction); value != 0 {
		return value
	}
	if value := cmp.Compare(left.IssueCode, right.IssueCode); value != 0 {
		return value
	}
	if value := cmp.Compare(left.Path, right.Path); value != 0 {
		return value
	}
	if value := slices.Compare(left.Refs, right.Refs); value != 0 {
		return value
	}
	return slices.Compare(left.Nets, right.Nets)
}

// AutonomousCorrectionInvariantFingerprint hashes the design fields that an
// autonomous placement/routing correction is never allowed to change.
func AutonomousCorrectionInvariantFingerprint(request Request) (string, error) {
	normalized := NormalizeRequest(request)
	projection := struct {
		Version         string               `json:"version"`
		Intent          Intent               `json:"intent"`
		Board           BoardSpec            `json:"board"`
		Constraints     ConstraintSpec       `json:"constraints"`
		Validation      ValidationSpec       `json:"validation"`
		ExplicitCircuit *ExplicitCircuitSpec `json:"explicit_circuit,omitempty"`
	}{
		Version:         normalized.Version,
		Intent:          normalized.Intent,
		Board:           normalized.Board,
		Constraints:     normalized.Constraints,
		Validation:      normalized.Validation,
		ExplicitCircuit: cloneExplicitCircuit(normalized.ExplicitCircuit),
	}
	data, err := json.Marshal(projection)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func IsGenericAutonomousCorrectionRequest(request Request) bool {
	return request.ExplicitCircuit != nil && strings.TrimSpace(request.Intent.Category) == "explicit_circuit_graph"
}

func AutonomousCorrectionRetryKey(diagnostics []AutonomousCorrectionDiagnostic, actionKinds []string, invariantFingerprint, placementState string) string {
	diagnostics = slices.Clone(diagnostics)
	slices.SortFunc(diagnostics, compareAutonomousCorrectionDiagnostic)
	actionKinds = correctionSortedStrings(actionKinds)
	hash := sha256.New()
	// writeHashBytes length-prefixes every field, so adjacent variable-length
	// values cannot create an ambiguous hash input.
	writeHashBytes(hash, []byte(AutonomousCorrectionSchemaV1))
	writeHashBytes(hash, []byte(invariantFingerprint))
	writeHashBytes(hash, []byte(placementState))
	writeHashBytes(hash, []byte("diagnostics:"+strconv.Itoa(len(diagnostics))))
	for _, diagnostic := range diagnostics {
		writeHashBytes(hash, []byte("diagnostic"))
		writeHashBytes(hash, []byte(diagnostic.Category))
		writeHashBytes(hash, []byte(diagnostic.SourceCategory))
		writeHashBytes(hash, []byte(diagnostic.SourceAction))
		writeHashBytes(hash, []byte(diagnostic.IssueCode))
		writeHashBytes(hash, []byte(diagnostic.Path))
		writeHashBytes(hash, []byte("refs:"+strconv.Itoa(len(diagnostic.Refs))))
		for _, ref := range diagnostic.Refs {
			writeHashBytes(hash, []byte("ref:"+ref))
		}
		writeHashBytes(hash, []byte("nets:"+strconv.Itoa(len(diagnostic.Nets))))
		for _, net := range diagnostic.Nets {
			writeHashBytes(hash, []byte("net:"+net))
		}
	}
	writeHashBytes(hash, []byte("actions:"+strconv.Itoa(len(actionKinds))))
	for _, action := range actionKinds {
		writeHashBytes(hash, []byte("action:"+action))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
