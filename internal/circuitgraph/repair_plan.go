package circuitgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"kicadai/internal/reports"
)

const RepairPlanHashVersion = "normalized-graph-sha256-v1"

type RepairPlanState string

const (
	RepairPlanReady       RepairPlanState = "ready"
	RepairPlanAvailable   RepairPlanState = "repair_available"
	RepairPlanNeedsReview RepairPlanState = "needs_review"
	RepairPlanBlocked     RepairPlanState = "blocked"
)

type RepairPlan struct {
	HashVersion string          `json:"hash_version"`
	InputHash   string          `json:"input_hash"`
	State       RepairPlanState `json:"state"`
	StopReason  string          `json:"stop_reason"`
	Selected    *RepairOption   `json:"selected_repair_option,omitempty"`
	Patch       *PatchDocument  `json:"patch,omitempty"`
	Issues      []reports.Issue `json:"issues"`
}

func NormalizedGraphHash(document Document) (string, error) {
	encoded, err := json.Marshal(Normalize(document))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// PlanRepair selects only a fully populated, valid patch. Unknown or multiple
// candidates deliberately stop for review rather than guessing.
func PlanRepair(document Document, ready bool, options []RepairOption, issues []reports.Issue, previous []string, maxAttempts int) RepairPlan {
	hash, err := NormalizedGraphHash(document)
	if err != nil {
		return RepairPlan{HashVersion: RepairPlanHashVersion, State: RepairPlanBlocked, StopReason: "hash_failed", Issues: issues}
	}
	plan := RepairPlan{HashVersion: RepairPlanHashVersion, InputHash: hash, Issues: append([]reports.Issue(nil), issues...)}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	if len(previous) >= maxAttempts {
		plan.State, plan.StopReason = RepairPlanBlocked, "max_attempts_exceeded"
		return plan
	}
	for _, prior := range previous {
		if prior == hash {
			plan.State, plan.StopReason = RepairPlanBlocked, "repeated_input_hash"
			return plan
		}
	}
	if ready {
		plan.State, plan.StopReason = RepairPlanReady, "preflight_ready"
		return plan
	}
	eligible := make([]RepairOption, 0, len(options))
	for _, option := range options {
		if option.Disposition == "agent_selectable" && len(option.RequiredValues) == 0 {
			eligible = append(eligible, option)
		}
	}
	sort.SliceStable(eligible, func(i, j int) bool { return eligible[i].DiagnosticID < eligible[j].DiagnosticID })
	if len(eligible) == 0 {
		plan.State, plan.StopReason = RepairPlanNeedsReview, "no_fully_derived_repair"
		return plan
	}
	if len(eligible) != 1 {
		plan.State, plan.StopReason = RepairPlanNeedsReview, "ambiguous_repair_options"
		return plan
	}
	patch := PatchDocument{Schema: PatchSchemaID, Version: PatchVersion, Operations: []PatchOperation{eligible[0].OperationTemplate}}
	if patchIssues := ValidatePatch(patch); reports.HasBlockingIssue(patchIssues) {
		plan.State, plan.StopReason, plan.Issues = RepairPlanBlocked, "invalid_candidate_patch", append(plan.Issues, patchIssues...)
		return plan
	}
	plan.State, plan.StopReason, plan.Selected, plan.Patch = RepairPlanAvailable, "single_safe_repair", &eligible[0], &patch
	return plan
}
