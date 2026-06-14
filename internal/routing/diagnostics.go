package routing

import (
	"strings"

	"kicadai/internal/reports"
)

type RepairCategory string

const (
	RepairRouteSearch   RepairCategory = "route_search"
	RepairLayerAccess   RepairCategory = "layer_access"
	RepairPadAccess     RepairCategory = "pad_access"
	RepairClearance     RepairCategory = "clearance"
	RepairConnectivity  RepairCategory = "connectivity"
	RepairBoardBoundary RepairCategory = "board_boundary"
	RepairExternalCheck RepairCategory = "external_check"
	RepairInputModel    RepairCategory = "input_model"
	RepairUnknown       RepairCategory = "unknown"
)

type RepairAction string

const (
	ActionMoveComponents       RepairAction = "move_components"
	ActionReduceClearance      RepairAction = "reduce_clearance"
	ActionAllowAdditionalLayer RepairAction = "allow_additional_layer"
	ActionFixPadGeometry       RepairAction = "fix_pad_geometry"
	ActionFixNetAssignment     RepairAction = "fix_net_assignment"
	ActionAddBoardOutline      RepairAction = "add_board_outline"
	ActionRepairGeneratedFile  RepairAction = "repair_generated_file"
	ActionRerunExternalCheck   RepairAction = "rerun_external_check"
	ActionInspectManually      RepairAction = "inspect_manually"
)

type RepairDiagnostic struct {
	Category   RepairCategory   `json:"category"`
	Action     RepairAction     `json:"action"`
	Severity   reports.Severity `json:"severity"`
	Message    string           `json:"message"`
	Suggestion string           `json:"suggestion,omitempty"`
	Path       string           `json:"path,omitempty"`
	Refs       []string         `json:"refs,omitempty"`
	Nets       []string         `json:"nets,omitempty"`
}

func DiagnosticsForResult(result Result) []RepairDiagnostic {
	diagnostics := make([]RepairDiagnostic, 0, len(result.Issues))
	for _, issue := range result.Issues {
		diagnostics = append(diagnostics, DiagnosticForIssue(issue))
	}
	return diagnostics
}

func DiagnosticForIssue(issue reports.Issue) RepairDiagnostic {
	category := repairCategoryForIssue(issue)
	return RepairDiagnostic{
		Category:   category,
		Action:     repairActionForCategory(category),
		Severity:   issue.Severity,
		Message:    issue.Message,
		Suggestion: diagnosticSuggestion(issue.Suggestion, category),
		Path:       issue.Path,
		Refs:       append([]string(nil), issue.Refs...),
		Nets:       append([]string(nil), issue.Nets...),
	}
}

func repairCategoryForIssue(issue reports.Issue) RepairCategory {
	switch issue.Code {
	case reports.CodeKiCadCLIFailed:
		return RepairExternalCheck
	case reports.CodeMissingBoardOutline:
		return RepairBoardBoundary
	case reports.CodeDisconnectedPad:
		return RepairConnectivity
	case reports.CodeInvalidNetAssignment:
		return RepairConnectivity
	case reports.CodePlacementOutsideBoard:
		return RepairBoardBoundary
	case reports.CodeInvalidArgument:
		return RepairInputModel
	}
	text := strings.ToLower(string(issue.Code) + " " + issue.Message + " " + issue.Suggestion)
	switch {
	case containsAnyText(text, "clearance", "too close", "intersects obstacle", "keepout"):
		return RepairClearance
	case containsAnyText(text, "layer is not available", "routing layer", "back layer") || containsAllText(text, "via", "not allowed"):
		return RepairLayerAccess
	case containsAnyText(text, "pad geometry", "pad layer", "access point", "two-layer routing access"):
		return RepairPadAccess
	case containsAnyText(text, "not connected", "unconnected", "no legal", "route does not connect"):
		return RepairRouteSearch
	default:
		return RepairUnknown
	}
}

func diagnosticSuggestion(explicit string, category RepairCategory) string {
	trimmed := strings.TrimSpace(explicit)
	if trimmed != "" {
		return trimmed
	}
	return suggestionForRoutingRepair(category)
}

func repairActionForCategory(category RepairCategory) RepairAction {
	switch category {
	case RepairRouteSearch:
		return ActionMoveComponents
	case RepairLayerAccess:
		return ActionAllowAdditionalLayer
	case RepairPadAccess:
		return ActionFixPadGeometry
	case RepairClearance:
		return ActionReduceClearance
	case RepairConnectivity:
		return ActionFixNetAssignment
	case RepairBoardBoundary:
		return ActionAddBoardOutline
	case RepairExternalCheck:
		return ActionRerunExternalCheck
	case RepairInputModel:
		return ActionRepairGeneratedFile
	default:
		return ActionInspectManually
	}
}

func suggestionForRoutingRepair(category RepairCategory) string {
	switch category {
	case RepairRouteSearch:
		return "Move components closer, reduce constraints, or allow additional layers before routing again."
	case RepairLayerAccess:
		return "Enable a routable copper layer or vias that match the endpoints and board stackup."
	case RepairPadAccess:
		return "Verify pad geometry, pad layers, drill data, and footprint placement."
	case RepairClearance:
		return "Increase spacing, reduce trace width, move placement, or relax clearance rules."
	case RepairConnectivity:
		return "Check net names, endpoint pins, generated vias, and routed segment continuity."
	case RepairBoardBoundary:
		return "Repair the board outline or move generated geometry inside the usable board area."
	case RepairExternalCheck:
		return "Fix the KiCad CLI invocation or generated project files, then rerun validation."
	case RepairInputModel:
		return "Correct the routing request schema and required design inputs before routing."
	default:
		return "Inspect the reported issue and update the design before rerunning routing."
	}
}

func containsAnyText(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func containsAllText(value string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}
