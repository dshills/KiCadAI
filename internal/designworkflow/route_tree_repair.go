package designworkflow

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

type InterBlockBranchFailureCategory string

const (
	InterBlockBranchFailureOtherNetPad     InterBlockBranchFailureCategory = "blocked_by_other_net_pad"
	InterBlockBranchFailureKeepout         InterBlockBranchFailureCategory = "blocked_by_keepout"
	InterBlockBranchFailureBoardEdge       InterBlockBranchFailureCategory = "blocked_by_board_edge"
	InterBlockBranchFailureExistingCopper  InterBlockBranchFailureCategory = "blocked_by_existing_copper"
	InterBlockBranchFailureLayerAccess     InterBlockBranchFailureCategory = "layer_access"
	InterBlockBranchFailureViaPolicy       InterBlockBranchFailureCategory = "via_policy"
	InterBlockBranchFailureSearchExhausted InterBlockBranchFailureCategory = "search_exhausted"
	InterBlockBranchFailureContactMiss     InterBlockBranchFailureCategory = "contact_miss"
	InterBlockBranchFailureMissingTarget   InterBlockBranchFailureCategory = "missing_contact_target"
	InterBlockBranchFailureGraphSplit      InterBlockBranchFailureCategory = "graph_split"
	InterBlockBranchFailureUnsupported     InterBlockBranchFailureCategory = "unsupported"
	InterBlockBranchFailureUnknown         InterBlockBranchFailureCategory = "unknown"
)

type InterBlockBranchRepairHint struct {
	Category   InterBlockBranchFailureCategory `json:"category"`
	NetName    string                          `json:"net_name"`
	Refs       []string                        `json:"refs,omitempty"`
	Nets       []string                        `json:"nets,omitempty"`
	RetryScope RetryScope                      `json:"retry_scope"`
	Action     string                          `json:"action"`
	Path       string                          `json:"path,omitempty"`
	Repairable bool                            `json:"repairable"`
}

type InterBlockRouteTreeRepairSummary struct {
	BranchFailures       int                          `json:"branch_failures"`
	RepairableFailures   int                          `json:"repairable_failures"`
	UnrepairableFailures int                          `json:"unrepairable_failures"`
	HintCount            int                          `json:"hint_count"`
	Nets                 []string                     `json:"nets,omitempty"`
	Refs                 []string                     `json:"refs,omitempty"`
	Hints                []InterBlockBranchRepairHint `json:"hints,omitempty"`
}

func BuildRouteTreeRepairHints(issues []reports.Issue) []InterBlockBranchRepairHint {
	hints := make([]InterBlockBranchRepairHint, 0, len(issues))
	seen := map[routeTreeRepairHintKey]struct{}{}
	for _, issue := range issues {
		hint, ok := classifyRouteTreeRepairIssue(issue)
		if !ok {
			continue
		}
		key := routeTreeRepairHintKeyFor(hint)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		hints = append(hints, hint)
	}
	slices.SortFunc(hints, compareRouteTreeRepairHint)
	return hints
}

func SummarizeRouteTreeRepair(hints []InterBlockBranchRepairHint) InterBlockRouteTreeRepairSummary {
	summary := InterBlockRouteTreeRepairSummary{
		BranchFailures: len(hints),
	}
	if len(hints) != 0 {
		summary.Hints = make([]InterBlockBranchRepairHint, 0, len(hints))
	}
	netSet := map[string]struct{}{}
	refSet := map[string]struct{}{}
	for _, hint := range hints {
		summary.Hints = append(summary.Hints, routeTreeRepairHintCopy(hint))
		if hint.Repairable {
			summary.RepairableFailures++
			summary.HintCount++
		} else {
			summary.UnrepairableFailures++
		}
		for _, net := range hint.Nets {
			net = strings.TrimSpace(net)
			if net != "" {
				netSet[net] = struct{}{}
			}
		}
		if net := strings.TrimSpace(hint.NetName); net != "" {
			netSet[net] = struct{}{}
		}
		for _, ref := range hint.Refs {
			ref = strings.TrimSpace(ref)
			if ref != "" {
				refSet[ref] = struct{}{}
			}
		}
	}
	summary.Nets = sortedSetKeys(netSet)
	summary.Refs = sortedSetKeys(refSet)
	slices.SortFunc(summary.Hints, compareRouteTreeRepairHint)
	return summary
}

func routeTreeRepairHintCopy(hint InterBlockBranchRepairHint) InterBlockBranchRepairHint {
	hint.Refs = routeTreeSortedStringsCopy(hint.Refs)
	hint.Nets = routeTreeSortedStringsCopy(hint.Nets)
	return hint
}

func BuildRouteTreePlacementRetryHints(issues []reports.Issue) []PlacementRetryHint {
	branchHints := BuildRouteTreeRepairHints(issues)
	hints := make([]PlacementRetryHint, 0, len(branchHints))
	for _, branchHint := range branchHints {
		category, eligible := routeTreePlacementRetryCategory(branchHint.Category)
		hint := PlacementRetryHint{
			Category:          category,
			SourceCategory:    routeTreeRepairCategory(branchHint.Category),
			SourceAction:      routeTreeRepairAction(category),
			Severity:          reports.SeverityBlocked,
			Refs:              routeTreeSortedStringsCopy(branchHint.Refs),
			Nets:              routeTreeSortedStringsCopy(branchHint.Nets),
			SuggestedAction:   branchHint.Action,
			RetryEligible:     eligible && branchHint.Repairable,
			PlacementEvidence: routeTreePlacementEvidence(branchHint),
		}
		hints = append(hints, hint)
	}
	return hints
}

func classifyRouteTreeRepairIssue(issue reports.Issue) (InterBlockBranchRepairHint, bool) {
	if !isRouteTreeRepairIssue(issue) {
		return InterBlockBranchRepairHint{}, false
	}
	netName := ""
	if len(issue.Nets) > 0 {
		netName = strings.TrimSpace(issue.Nets[0])
	}
	category := routeTreeFailureCategory(issue)
	hint := InterBlockBranchRepairHint{
		Category:   category,
		NetName:    netName,
		Refs:       routeTreeSortedStringsCopy(issue.Refs),
		Nets:       routeTreeSortedStringsCopy(issue.Nets),
		RetryScope: RetryScopeRouting,
		Action:     routeTreeRepairActionText(category),
		Path:       issue.Path,
		Repairable: routeTreeFailureRepairable(category),
	}
	return hint, true
}

func isRouteTreeRepairIssue(issue reports.Issue) bool {
	if issue.Severity == reports.SeverityInfo {
		return false
	}
	if issue.Code == reports.CodeFixedNetSkipped || issue.Code == reports.CodeMissingNetClass {
		return false
	}
	if strings.Contains(issue.Path, "design.inter_block_route_groups") {
		return true
	}
	if strings.Contains(issue.Path, "design.local_route_rebuild") {
		return true
	}
	switch issue.Code {
	case reports.CodeRouteContactMiss,
		reports.CodeRouteContactMissingTarget,
		reports.CodeRouteGraphIncomplete,
		reports.CodeRouteContactUnsupported,
		reports.CodeUnsupportedOperation:
		return true
	default:
		return false
	}
}

func routeTreeFailureCategory(issue reports.Issue) InterBlockBranchFailureCategory {
	switch issue.Code {
	case reports.CodeRouteContactMiss:
		return InterBlockBranchFailureContactMiss
	case reports.CodeRouteContactMissingTarget:
		return InterBlockBranchFailureMissingTarget
	case reports.CodeRouteGraphIncomplete:
		return InterBlockBranchFailureGraphSplit
	case reports.CodeRouteContactUnsupported, reports.CodeUnsupportedOperation:
		return InterBlockBranchFailureUnsupported
	}
	diagnostic := routing.DiagnosticForIssue(issue)
	switch diagnostic.Category {
	case routing.RepairClearance:
		return InterBlockBranchFailureOtherNetPad
	case routing.RepairBoardBoundary:
		return InterBlockBranchFailureBoardEdge
	case routing.RepairLayerAccess, routing.RepairPadAccess:
		return InterBlockBranchFailureLayerAccess
	case routing.RepairViaPolicy:
		return InterBlockBranchFailureViaPolicy
	case routing.RepairRouteSearch, routing.RepairConnectivity:
		return InterBlockBranchFailureSearchExhausted
	case routing.RepairZonePolicy, routing.RepairInputModel, routing.RepairExternalCheck:
		return InterBlockBranchFailureUnsupported
	default:
		return InterBlockBranchFailureUnknown
	}
}

func routeTreeFailureRepairable(category InterBlockBranchFailureCategory) bool {
	switch category {
	case InterBlockBranchFailureOtherNetPad,
		InterBlockBranchFailureKeepout,
		InterBlockBranchFailureBoardEdge,
		InterBlockBranchFailureExistingCopper,
		InterBlockBranchFailureLayerAccess,
		InterBlockBranchFailureViaPolicy,
		InterBlockBranchFailureSearchExhausted,
		InterBlockBranchFailureContactMiss,
		InterBlockBranchFailureMissingTarget,
		InterBlockBranchFailureGraphSplit:
		return true
	default:
		return false
	}
}

func routeTreePlacementRetryCategory(category InterBlockBranchFailureCategory) (PlacementRetryHintCategory, bool) {
	switch category {
	case InterBlockBranchFailureOtherNetPad, InterBlockBranchFailureExistingCopper:
		return PlacementRetryIncreaseSpacing, true
	case InterBlockBranchFailureKeepout, InterBlockBranchFailureBoardEdge:
		return PlacementRetryMoveFromEdge, true
	case InterBlockBranchFailureLayerAccess, InterBlockBranchFailureViaPolicy:
		return PlacementRetryImproveFanout, true
	case InterBlockBranchFailureSearchExhausted:
		return PlacementRetryReduceDistance, true
	case InterBlockBranchFailureContactMiss, InterBlockBranchFailureMissingTarget, InterBlockBranchFailureGraphSplit:
		return PlacementRetryImproveFanout, true
	default:
		return PlacementRetryUnsupported, false
	}
}

func routeTreeRepairCategory(category InterBlockBranchFailureCategory) routing.RepairCategory {
	switch category {
	case InterBlockBranchFailureOtherNetPad, InterBlockBranchFailureExistingCopper:
		return routing.RepairClearance
	case InterBlockBranchFailureKeepout, InterBlockBranchFailureBoardEdge:
		return routing.RepairBoardBoundary
	case InterBlockBranchFailureLayerAccess:
		return routing.RepairLayerAccess
	case InterBlockBranchFailureViaPolicy:
		return routing.RepairViaPolicy
	case InterBlockBranchFailureSearchExhausted:
		return routing.RepairRouteSearch
	case InterBlockBranchFailureContactMiss, InterBlockBranchFailureMissingTarget, InterBlockBranchFailureGraphSplit:
		return routing.RepairConnectivity
	default:
		return routing.RepairInputModel
	}
}

func routeTreeRepairAction(category PlacementRetryHintCategory) routing.RepairAction {
	switch category {
	case PlacementRetryIncreaseSpacing:
		return routing.ActionMoveComponents
	case PlacementRetryReduceDistance:
		return routing.ActionMoveComponents
	case PlacementRetryImproveFanout:
		return routing.ActionFixPadGeometry
	case PlacementRetryMoveFromEdge:
		return routing.ActionAddBoardOutline
	default:
		return routing.ActionInspectManually
	}
}

func routeTreeRepairActionText(category InterBlockBranchFailureCategory) string {
	switch category {
	case InterBlockBranchFailureOtherNetPad, InterBlockBranchFailureExistingCopper:
		return "increase branch spacing or move eligible refs away from blocking copper"
	case InterBlockBranchFailureKeepout, InterBlockBranchFailureBoardEdge:
		return "move eligible branch refs away from blocked board regions"
	case InterBlockBranchFailureLayerAccess, InterBlockBranchFailureViaPolicy:
		return "increase fanout room or repair layer access for this branch"
	case InterBlockBranchFailureSearchExhausted:
		return "move connected branch endpoints closer or reduce routing pressure"
	case InterBlockBranchFailureContactMiss:
		return "snap or rebuild route endpoint contact for this branch"
	case InterBlockBranchFailureMissingTarget:
		return "repair endpoint target resolution or emit required branch route copper"
	case InterBlockBranchFailureGraphSplit:
		return "connect same-net graph components for this route group"
	default:
		return "manual route-tree repair review required"
	}
}

func routeTreePlacementEvidence(hint InterBlockBranchRepairHint) []string {
	var evidence []string
	if hint.NetName != "" {
		evidence = append(evidence, "route_tree_net:"+hint.NetName)
	}
	return evidence
}

type routeTreeRepairHintKey struct {
	Category InterBlockBranchFailureCategory
	NetName  string
	Refs     string
	Nets     string
	Path     string
}

func routeTreeRepairHintKeyFor(hint InterBlockBranchRepairHint) routeTreeRepairHintKey {
	return routeTreeRepairHintKey{
		Category: hint.Category,
		NetName:  hint.NetName,
		Refs:     routeTreeStringListKey(hint.Refs),
		Nets:     routeTreeStringListKey(hint.Nets),
		Path:     hint.Path,
	}
}

func routeTreeStringListKey(values []string) string {
	if len(values) == 0 {
		return "0:"
	}
	var builder strings.Builder
	for _, value := range values {
		builder.WriteString(strconv.Itoa(len(value)))
		builder.WriteByte(':')
		builder.WriteString(value)
		builder.WriteByte(';')
	}
	return builder.String()
}

func compareRouteTreeRepairHint(left, right InterBlockBranchRepairHint) int {
	if compare := cmp.Compare(left.NetName, right.NetName); compare != 0 {
		return compare
	}
	if compare := cmp.Compare(left.Category, right.Category); compare != 0 {
		return compare
	}
	if compare := slices.Compare(left.Refs, right.Refs); compare != 0 {
		return compare
	}
	if compare := slices.Compare(left.Nets, right.Nets); compare != 0 {
		return compare
	}
	return cmp.Compare(left.Path, right.Path)
}

func sortedSetKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func routeTreeSortedStringsCopy(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := slices.Clone(values)
	slices.Sort(out)
	return out
}
