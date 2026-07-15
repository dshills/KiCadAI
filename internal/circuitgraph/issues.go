package circuitgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

const (
	PreflightStageSchema       = "schema"
	PreflightStageComponent    = "component_resolution"
	PreflightStageConnectivity = "connectivity"
	PreflightStageSchematic    = "schematic_layout"
	PreflightStagePCB          = "pcb_constraints"
	PreflightStagePolicy       = "repair_policy"
)

func annotateGraphIssue(issue reports.Issue) reports.Issue {
	if issue.Stage == "" {
		issue.Stage = graphIssueStage(issue)
	}
	if issue.RetryScope == "" {
		issue.RetryScope = graphRetryScope(issue.Stage)
	}
	if issue.Suggestion == "" {
		issue.Suggestion = graphSuggestedAction(issue.Stage)
	}
	if issue.IssueID == "" {
		hash := sha256.Sum256([]byte(strings.Join([]string{string(issue.Code), issue.Path, issue.Message}, "\x00")))
		issue.IssueID = "graph-" + hex.EncodeToString(hash[:8])
	}
	return issue
}

func graphDependentIssue(code reports.Code, path, message, rootCauseID, retryScope string) reports.Issue {
	issue := graphIssue(code, path, message)
	issue.RootCauseID = rootCauseID
	if retryScope != "" {
		issue.RetryScope = retryScope
	}
	return issue
}

func finalizeGraphIssues(issues []reports.Issue) []reports.Issue {
	annotated := make([]reports.Issue, 0, len(issues))
	roots := make(map[string]struct{}, len(issues))
	for _, issue := range issues {
		issue = annotateGraphIssue(issue)
		annotated = append(annotated, issue)
		if issue.RootCauseID == "" {
			roots[issue.IssueID] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(annotated))
	result := make([]reports.Issue, 0, len(annotated))
	for _, issue := range annotated {
		if issue.RootCauseID != "" {
			if _, rooted := roots[issue.RootCauseID]; rooted {
				continue
			}
		}
		key := issue.IssueID + "\x00" + issue.RootCauseID
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, issue)
	}
	slices.SortStableFunc(result, compareGraphIssues)
	return result
}

func compareGraphIssues(left, right reports.Issue) int {
	if rank := graphStageRank(left.Stage) - graphStageRank(right.Stage); rank != 0 {
		return rank
	}
	if left.Path != right.Path {
		return strings.Compare(left.Path, right.Path)
	}
	if left.Code != right.Code {
		return strings.Compare(string(left.Code), string(right.Code))
	}
	if left.Message != right.Message {
		return strings.Compare(left.Message, right.Message)
	}
	return strings.Compare(left.IssueID, right.IssueID)
}

func graphIssueStage(issue reports.Issue) string {
	path := strings.TrimSpace(issue.Path)
	switch {
	case strings.HasPrefix(path, "components"), path == "catalog", path == "resolver":
		return PreflightStageComponent
	case strings.HasPrefix(path, "nets"), strings.HasPrefix(path, "no_connects"), strings.HasPrefix(path, "power_flags"), strings.HasPrefix(path, "buses"):
		return PreflightStageConnectivity
	case strings.HasPrefix(path, "schematic"):
		return PreflightStageSchematic
	case strings.HasPrefix(path, "pcb"):
		return PreflightStagePCB
	case strings.HasPrefix(path, "policy"):
		return PreflightStagePolicy
	default:
		return PreflightStageSchema
	}
}

func graphRetryScope(stage string) string {
	switch stage {
	case PreflightStageComponent:
		return "component"
	case PreflightStageConnectivity:
		return "connectivity"
	case PreflightStageSchematic:
		return "schematic"
	case PreflightStagePCB:
		return "pcb"
	case PreflightStagePolicy:
		return "policy"
	default:
		return "graph"
	}
}

func graphSuggestedAction(stage string) string {
	switch stage {
	case PreflightStageComponent:
		return "correct the component selection, unit, symbol, footprint, or verified pin mapping"
	case PreflightStageConnectivity:
		return "correct net endpoints, selectors, no-connects, buses, or power flags"
	case PreflightStageSchematic:
		return "correct schematic groups, lanes, placements, or readability rules"
	case PreflightStagePCB:
		return "correct PCB regions, placements, keepouts, zones, or board constraints"
	case PreflightStagePolicy:
		return "make every automatic repair policy decision explicit"
	default:
		return "correct the strict circuit graph schema and bounded values"
	}
}

func graphStageRank(stage string) int {
	switch stage {
	case PreflightStageSchema:
		return 0
	case PreflightStageComponent:
		return 1
	case PreflightStageConnectivity:
		return 2
	case PreflightStageSchematic:
		return 3
	case PreflightStagePCB:
		return 4
	case PreflightStagePolicy:
		return 5
	default:
		return 6
	}
}
