package designworkflow

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

const AutonomousCorrectionSchemaV1 = "kicadai.autonomous-correction.v1"
const GenericAutonomousCorrectionMaxAttempts = 3

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

type AutonomousCorrectionApplication struct {
	Applied                      bool                     `json:"applied"`
	RetryKey                     string                   `json:"retry_key,omitempty"`
	StopReason                   string                   `json:"stop_reason,omitempty"`
	InvariantFingerprintBefore   string                   `json:"invariant_fingerprint_before,omitempty"`
	InvariantFingerprintAfter    string                   `json:"invariant_fingerprint_after,omitempty"`
	PlacementInvariantBefore     string                   `json:"placement_invariant_before,omitempty"`
	PlacementInvariantAfter      string                   `json:"placement_invariant_after,omitempty"`
	Adjustment                   PlacementRetryAdjustment `json:"adjustment,omitempty"`
	ValidationIssues             []reports.Issue          `json:"validation_issues,omitempty"`
	ProtectedInvariantsPreserved bool                     `json:"protected_invariants_preserved"`
}

type AutonomousCorrectionAttempt struct {
	Attempt            int                              `json:"attempt"`
	Outcome            string                           `json:"outcome"`
	Plan               *AutonomousCorrectionPlan        `json:"plan,omitempty"`
	Application        *AutonomousCorrectionApplication `json:"application,omitempty"`
	RoutingStatus      routing.Status                   `json:"routing_status,omitempty"`
	RoutedNets         int                              `json:"routed_nets"`
	FailedNets         int                              `json:"failed_nets"`
	PlacementStateHash string                           `json:"placement_state_hash,omitempty"`
	Selected           bool                             `json:"selected,omitempty"`
	SelectedReason     string                           `json:"selected_reason,omitempty"`
	RegressionFlags    []string                         `json:"regression_flags,omitempty"`
}

type AutonomousCorrectionReport struct {
	SchemaVersion string `json:"schema_version"`
	Scope         string `json:"scope"`
	Enabled       bool   `json:"enabled"`
	MaxAttempts   int    `json:"max_attempts"`
	// Attempts counts the initial route plus retries that reached routing.
	Attempts int `json:"attempts"`
	// PlanEvaluations counts correction plans, including fail-closed plans.
	PlanEvaluations               int                           `json:"plan_evaluations"`
	Applied                       int                           `json:"applied"`
	StopReason                    string                        `json:"stop_reason,omitempty"`
	SelectedAttempt               int                           `json:"selected_attempt,omitempty"`
	SelectedReason                string                        `json:"selected_reason,omitempty"`
	InitialInvariantFingerprint   string                        `json:"initial_invariant_fingerprint,omitempty"`
	FinalInvariantFingerprint     string                        `json:"final_invariant_fingerprint,omitempty"`
	ProtectedInvariantsPreserved  bool                          `json:"protected_invariants_preserved"`
	AllAttemptInvariantsPreserved bool                          `json:"all_attempt_invariants_preserved"`
	AppliedRetryKeys              []string                      `json:"applied_retry_keys,omitempty"`
	AttemptHistory                []AutonomousCorrectionAttempt `json:"attempt_history,omitempty"`
}

const (
	CorrectionStopNotGeneric                = "not_generic_circuit"
	CorrectionStopNotRequired               = "correction_not_required"
	CorrectionStopBudgetExhausted           = "budget_exhausted"
	CorrectionStopUnsupportedDiagnostic     = "unsupported_diagnostic"
	CorrectionStopAmbiguousDiagnostics      = "ambiguous_diagnostics"
	CorrectionStopFixedConstraintConflict   = "fixed_constraint_conflict"
	CorrectionStopRepeatedRetryKey          = "repeated_retry_key"
	CorrectionStopPlanNotAuthorized         = "plan_not_authorized"
	CorrectionStopInvariantMismatch         = "invariant_mismatch"
	CorrectionStopNoSafeAdjustment          = "no_safe_adjustment"
	CorrectionStopAdjustedRequestInvalid    = "adjusted_request_invalid"
	CorrectionStopContextCanceled           = "context_canceled"
	CorrectionStopRepeatedPlacementState    = "repeated_placement_state"
	CorrectionStopNonImprovingRetry         = "non_improving_retry"
	CorrectionStopMaxAttempts               = "max_attempts"
	CorrectionStopRouted                    = "routed"
	CorrectionStopDRCRegression             = "drc_regression"
	CorrectionStopBoardValidationRegression = "board_validation_regression"
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
	for _, diagnostic := range autonomousCorrectionAuthorizationDiagnostics(blocking) {
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

func autonomousCorrectionAuthorizationDiagnostics(diagnostics []AutonomousCorrectionDiagnostic) []AutonomousCorrectionDiagnostic {
	actionableNets := map[string]struct{}{}
	for _, diagnostic := range diagnostics {
		if !diagnostic.AutomaticAction || len(diagnostic.Refs) == 0 {
			continue
		}
		for _, net := range diagnostic.Nets {
			actionableNets[net] = struct{}{}
		}
	}
	result := make([]AutonomousCorrectionDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if autonomousCorrectionTerminalDiagnosticCovered(diagnostic, actionableNets) {
			continue
		}
		result = append(result, diagnostic)
	}
	return result
}

func autonomousCorrectionTerminalDiagnosticCovered(diagnostic AutonomousCorrectionDiagnostic, actionableNets map[string]struct{}) bool {
	if diagnostic.AutomaticAction || len(diagnostic.Refs) != 0 || len(diagnostic.Nets) == 0 {
		return false
	}
	terminal := diagnostic.IssueCode == reports.CodeDisconnectedPad ||
		diagnostic.IssueCode == reports.CodeValidationFailed && strings.HasPrefix(diagnostic.Path, "explicit_circuit.nets.")
	if !terminal {
		return false
	}
	for _, net := range diagnostic.Nets {
		if _, covered := actionableNets[net]; !covered {
			return false
		}
	}
	return true
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

// ApplyAutonomousCorrectionPlan guards and applies only the existing bounded
// placement adjustments represented by an authorized plan. It never writes
// files or runs placement/routing itself.
func ApplyAutonomousCorrectionPlan(request Request, placementRequest placement.Request, placements []placement.PlacementResult, plan AutonomousCorrectionPlan, appliedRetryKeys []string) (placement.Request, AutonomousCorrectionApplication, error) {
	current := placement.CloneRequest(placementRequest)
	application := AutonomousCorrectionApplication{RetryKey: plan.RetryKey}
	if !IsGenericAutonomousCorrectionRequest(request) || !plan.Authorized || plan.StopReason != "" || plan.Attempt < 2 || plan.Attempt > plan.MaxAttempts {
		application.StopReason = CorrectionStopPlanNotAuthorized
		return current, application, nil
	}
	fingerprint, err := AutonomousCorrectionInvariantFingerprint(request)
	if err != nil {
		return current, application, err
	}
	application.InvariantFingerprintBefore = fingerprint
	application.InvariantFingerprintAfter = fingerprint
	if fingerprint != plan.InvariantFingerprint || placementStateHash(placements) != plan.PlacementStateHash {
		application.StopReason = CorrectionStopInvariantMismatch
		return current, application, nil
	}
	if plan.RetryKey == "" || slices.Contains(appliedRetryKeys, plan.RetryKey) {
		application.StopReason = CorrectionStopRepeatedRetryKey
		return current, application, nil
	}
	beforePlacementInvariant, err := autonomousCorrectionPlacementInvariantFingerprint(current)
	if err != nil {
		return current, application, err
	}
	application.PlacementInvariantBefore = beforePlacementInvariant
	hints := autonomousCorrectionPlacementHints(plan.Actions)
	if len(hints) == 0 {
		application.StopReason = CorrectionStopNoSafeAdjustment
		return current, application, nil
	}
	adjusted, adjustment := BuildPlacementRetryAdjustment(current, hints, plan.Attempt-1)
	adjustment.Attempt = plan.Attempt - 1
	application.Adjustment = adjustment
	if !adjustment.Applied || adjustment.SpacingDeltaMM > placementRetryMaxSpacingDeltaMM+retryScoreComparisonEpsilon || len(adjustment.ProximityRules) == 0 && math.Abs(adjustment.SpacingDeltaMM) < retryScoreComparisonEpsilon {
		application.StopReason = CorrectionStopNoSafeAdjustment
		return current, application, nil
	}
	afterPlacementInvariant, err := autonomousCorrectionPlacementInvariantFingerprint(adjusted)
	if err != nil {
		return current, application, err
	}
	application.PlacementInvariantAfter = afterPlacementInvariant
	if beforePlacementInvariant != afterPlacementInvariant {
		application.StopReason = CorrectionStopInvariantMismatch
		return current, application, nil
	}
	issues := placement.Validate(adjusted)
	application.ValidationIssues = append([]reports.Issue(nil), issues...)
	if reports.HasBlockingIssue(issues) {
		application.StopReason = CorrectionStopAdjustedRequestInvalid
		return current, application, nil
	}
	application.Applied = true
	application.ProtectedInvariantsPreserved = true
	return adjusted, application, nil
}

func autonomousCorrectionPlacementHints(actions []AutonomousCorrectionAction) []PlacementRetryHint {
	hints := make([]PlacementRetryHint, 0, len(actions))
	for _, action := range actions {
		if !action.Authorized || action.PlacementHint == "" {
			continue
		}
		hints = append(hints, PlacementRetryHint{
			Category: action.PlacementHint, SourceCategory: autonomousCorrectionActionSourceCategory(action),
			SourceAction: routing.ActionMoveComponents, Severity: reports.SeverityBlocked,
			Refs: slices.Clone(action.Refs), Nets: slices.Clone(action.Nets),
			SuggestedAction: placementRetryHintAction(action.PlacementHint), RetryEligible: true,
			PlacementEvidence: []string{"autonomous_correction:" + string(action.Kind)},
		})
	}
	return mergePlacementRetryHints(nil, hints...)
}

func autonomousCorrectionActionSourceCategory(action AutonomousCorrectionAction) routing.RepairCategory {
	switch action.PlacementHint {
	case PlacementRetryIncreaseSpacing:
		return routing.RepairClearance
	case PlacementRetryImproveFanout:
		return routing.RepairPadAccess
	case PlacementRetryMoveFromEdge:
		return routing.RepairBoardBoundary
	case PlacementRetryReduceDistance:
		return routing.RepairLengthPolicy
	default:
		return routing.RepairUnknown
	}
}

func autonomousCorrectionPlacementInvariantFingerprint(request placement.Request) (string, error) {
	normalized := placement.NormalizeRequest(request)
	rules := struct {
		GridMM                   float64
		BoardEdgeClearanceMM     float64
		PreferTopLayer           bool
		AllowBackLayer           bool
		ConnectorEdgeClearanceMM float64
	}{
		GridMM: normalized.Rules.GridMM, BoardEdgeClearanceMM: normalized.Rules.BoardEdgeClearanceMM,
		PreferTopLayer: normalized.Rules.PreferTopLayer, AllowBackLayer: normalized.Rules.AllowBackLayer,
		ConnectorEdgeClearanceMM: normalized.Rules.ConnectorEdgeClearanceMM,
	}
	projection := struct {
		Board         placement.BoardPlacementArea
		Components    []placement.Component
		Nets          []placement.Net
		Groups        []placement.Group
		Keepouts      []placement.Keepout
		Mechanical    []placement.MechanicalConstraint
		RegionRules   []placement.RegionRule
		AdvancedRules placement.AdvancedPlacementRules
		Existing      placement.ExistingPlacementPolicy
		Rules         any
	}{
		Board: normalized.Board, Components: normalized.Components, Nets: normalized.Nets,
		Groups: normalized.Groups, Keepouts: normalized.Keepouts, Mechanical: normalized.Mechanical,
		RegionRules: normalized.RegionRules, AdvancedRules: normalized.AdvancedRules,
		Existing: normalized.Existing, Rules: rules,
	}
	data, err := json.Marshal(projection)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func newAutonomousCorrectionReport(request Request, policy RoutingRetryPolicySpec, placed PlacementStageResult, routed RoutingStageResult) *AutonomousCorrectionReport {
	if !IsGenericAutonomousCorrectionRequest(request) {
		return nil
	}
	fingerprint, err := AutonomousCorrectionInvariantFingerprint(request)
	preserved := err == nil && fingerprint != ""
	return &AutonomousCorrectionReport{
		SchemaVersion:                 AutonomousCorrectionSchemaV1,
		Scope:                         "generic-circuit-v1",
		Enabled:                       policy.Enabled,
		MaxAttempts:                   policy.MaxAttempts,
		Attempts:                      1,
		InitialInvariantFingerprint:   fingerprint,
		FinalInvariantFingerprint:     fingerprint,
		ProtectedInvariantsPreserved:  preserved,
		AllAttemptInvariantsPreserved: preserved,
		AttemptHistory:                []AutonomousCorrectionAttempt{autonomousCorrectionAttemptForResult(1, nil, nil, placed, routed)},
	}
}

func autonomousCorrectionAttemptForResult(attempt int, plan *AutonomousCorrectionPlan, application *AutonomousCorrectionApplication, placed PlacementStageResult, routed RoutingStageResult) AutonomousCorrectionAttempt {
	result := AutonomousCorrectionAttempt{
		Attempt: attempt, Outcome: "materialized", Plan: plan, Application: application,
		RoutingStatus: routed.Result.Status, RoutedNets: routed.Result.Metrics.RoutedNetCount,
		FailedNets:         routed.Result.Metrics.FailedNetCount,
		PlacementStateHash: placementStateHash(placed.Result.Placements),
	}
	return result
}

func autonomousCorrectionUnmaterializedAttempt(attempt int, outcome string, plan *AutonomousCorrectionPlan, application *AutonomousCorrectionApplication, placed PlacementStageResult) AutonomousCorrectionAttempt {
	return AutonomousCorrectionAttempt{
		Attempt: attempt, Outcome: outcome, Plan: plan, Application: application,
		PlacementStateHash: placementStateHash(placed.Result.Placements),
	}
}

func finalizeAutonomousCorrectionReport(report *AutonomousCorrectionReport, request Request, summary placementRoutingRetrySummary) {
	if report == nil {
		return
	}
	finalFingerprint, err := AutonomousCorrectionInvariantFingerprint(request)
	if err != nil {
		finalFingerprint = ""
	}
	report.FinalInvariantFingerprint = finalFingerprint
	report.Attempts = summary.Attempts
	report.PlanEvaluations = 0
	for _, attempt := range report.AttemptHistory {
		if attempt.Plan != nil {
			report.PlanEvaluations++
		}
	}
	report.Applied = summary.Applied
	report.StopReason = summary.StopReason
	report.SelectedAttempt = summary.SelectedAttempt
	report.SelectedReason = summary.SelectedReason
	if report.SelectedAttempt == 0 {
		report.SelectedAttempt = 1
	}
	if report.SelectedReason == "" {
		report.SelectedReason = "initial_attempt"
	}
	fingerprintPreserved := report.InitialInvariantFingerprint != "" && report.InitialInvariantFingerprint == report.FinalInvariantFingerprint
	selectedAttemptPreserved := true
	allAttemptsPreserved := fingerprintPreserved
	for index := range report.AttemptHistory {
		report.AttemptHistory[index].Selected = report.AttemptHistory[index].Attempt == report.SelectedAttempt
		if report.AttemptHistory[index].Selected {
			report.AttemptHistory[index].SelectedReason = report.SelectedReason
		}
		for _, legacy := range summary.AttemptHistory {
			if legacy.Attempt == report.AttemptHistory[index].Attempt {
				report.AttemptHistory[index].RegressionFlags = slices.Clone(legacy.RegressionFlags)
				break
			}
		}
		if application := report.AttemptHistory[index].Application; application != nil && application.Applied && !application.ProtectedInvariantsPreserved {
			allAttemptsPreserved = false
			if report.AttemptHistory[index].Selected {
				selectedAttemptPreserved = false
			}
		}
	}
	report.ProtectedInvariantsPreserved = fingerprintPreserved && selectedAttemptPreserved
	report.AllAttemptInvariantsPreserved = allAttemptsPreserved
}

func AutonomousCorrectionReportFromWorkflow(workflow WorkflowResult) (AutonomousCorrectionReport, bool) {
	for _, stage := range workflow.Stages {
		if stage.Name != StageRouting || stage.Summary == nil {
			continue
		}
		value, exists := stage.Summary["autonomous_correction"]
		if !exists {
			continue
		}
		switch report := value.(type) {
		case AutonomousCorrectionReport:
			return report, true
		case *AutonomousCorrectionReport:
			if report != nil {
				return *report, true
			}
		case map[string]any:
			data, err := json.Marshal(report)
			if err != nil {
				continue
			}
			var decoded AutonomousCorrectionReport
			if err := json.Unmarshal(data, &decoded); err == nil && decoded.SchemaVersion == AutonomousCorrectionSchemaV1 {
				return decoded, true
			}
		}
	}
	return AutonomousCorrectionReport{}, false
}

func AutonomousCorrectionEvidence(request Request, workflow WorkflowResult) (AutonomousCorrectionReport, bool) {
	if report, ok := AutonomousCorrectionReportFromWorkflow(workflow); ok {
		return report, true
	}
	if !IsGenericAutonomousCorrectionRequest(request) {
		return AutonomousCorrectionReport{}, false
	}
	fingerprint, err := AutonomousCorrectionInvariantFingerprint(request)
	preserved := err == nil && fingerprint != ""
	stopReason := "workflow_ended_before_routing"
	for _, stage := range workflow.Stages {
		if stage.Status == StageStatusBlocked {
			stopReason = "blocked_before_routing"
			break
		}
		if stage.Name == StageRouting {
			stopReason = "routing_evidence_missing"
			break
		}
	}
	return AutonomousCorrectionReport{
		SchemaVersion:                 AutonomousCorrectionSchemaV1,
		Scope:                         "generic-circuit-v1",
		Enabled:                       request.RoutingRetry.Enabled,
		MaxAttempts:                   request.RoutingRetry.MaxAttempts,
		StopReason:                    stopReason,
		InitialInvariantFingerprint:   fingerprint,
		FinalInvariantFingerprint:     fingerprint,
		ProtectedInvariantsPreserved:  preserved,
		AllAttemptInvariantsPreserved: preserved,
	}, true
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

type autonomousCorrectionClosedLoopBinding struct {
	Schema              string `json:"schema"`
	PolicyVersion       string `json:"policy_version"`
	PolicyHash          string `json:"policy_hash"`
	RequirementHash     string `json:"requirement_hash"`
	RegistryHash        string `json:"registry_hash"`
	CatalogHash         string `json:"catalog_hash"`
	FormulaLibraryHash  string `json:"formula_library_hash"`
	ModelRegistryHash   string `json:"model_registry_hash"`
	SelectedCircuitHash string `json:"selected_circuit_hash"`
	StopReason          string `json:"stop_reason"`
	Status              string `json:"status"`
}

// AutonomousCorrectionInvariantFingerprint hashes the design fields that an
// autonomous placement/routing correction is never allowed to change.
func AutonomousCorrectionInvariantFingerprint(request Request) (string, error) {
	// Closed-loop reports can contain full transient and sweep traces. They are
	// validation evidence, not placement/routing design input, so hashing the
	// complete report made every correction attempt clone and encode hundreds of
	// megabytes. Retain the immutable synthesis binding in the fingerprint and
	// remove only the bulky evaluation transcript before normalizing the request.
	fingerprintRequest := request
	var closedLoopBinding *autonomousCorrectionClosedLoopBinding
	if fingerprintRequest.ExplicitCircuit != nil {
		detached := *fingerprintRequest.ExplicitCircuit
		if report := detached.ClosedLoop; report != nil {
			closedLoopBinding = &autonomousCorrectionClosedLoopBinding{
				Schema: report.Schema, PolicyVersion: report.PolicyVersion,
				PolicyHash: report.PolicyHash, RequirementHash: report.RequirementHash,
				RegistryHash: report.RegistryHash, CatalogHash: report.CatalogHash,
				FormulaLibraryHash: report.FormulaLibraryHash, ModelRegistryHash: report.ModelRegistryHash,
				SelectedCircuitHash: report.SelectedCircuitHash,
				StopReason:          string(report.StopReason), Status: report.Status,
			}
		}
		detached.ClosedLoop = nil
		fingerprintRequest.ExplicitCircuit = cloneExplicitCircuit(&detached)
	}
	// NormalizeRequest returns isolated mutable fields; the invariant tests
	// assert byte-for-byte that this path never mutates the caller's request.
	normalized := NormalizeRequest(fingerprintRequest)
	projection := struct {
		Version           string                                 `json:"version"`
		Intent            Intent                                 `json:"intent"`
		Board             BoardSpec                              `json:"board"`
		Constraints       ConstraintSpec                         `json:"constraints"`
		Validation        ValidationSpec                         `json:"validation"`
		ExplicitCircuit   *ExplicitCircuitSpec                   `json:"explicit_circuit,omitempty"`
		ClosedLoopBinding *autonomousCorrectionClosedLoopBinding `json:"closed_loop_binding,omitempty"`
	}{
		Version:           normalized.Version,
		Intent:            normalized.Intent,
		Board:             normalized.Board,
		Constraints:       normalized.Constraints,
		Validation:        normalized.Validation,
		ExplicitCircuit:   normalized.ExplicitCircuit,
		ClosedLoopBinding: closedLoopBinding,
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
