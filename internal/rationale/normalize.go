package rationale

import (
	"fmt"
	"strings"

	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
)

func deriveStatus(report Report) Status {
	if len(report.Clarifications) > 0 {
		return StatusNeedsClarification
	}
	for _, limit := range report.KnownLimits {
		if limit.Severity == string(reports.SeverityError) || limit.Severity == string(reports.SeverityBlocked) {
			return StatusBlocked
		}
	}
	if report.Validation.RequestedAcceptance != "" && report.Validation.AchievedAcceptance != "" {
		if !designworkflow.AcceptanceSatisfied(designworkflow.AcceptanceLevel(report.Validation.RequestedAcceptance), designworkflow.AcceptanceLevel(report.Validation.AchievedAcceptance)) {
			return StatusPartial
		}
	}
	if report.Validation.WarningCount > 0 || report.Validation.SkippedStages > 0 || len(report.KnownLimits) > 0 {
		return StatusPartial
	}
	return StatusReady
}

func categoryForPath(path string, message string) string {
	value := strings.ToLower(path + " " + message)
	switch {
	case strings.Contains(value, "component") || strings.Contains(value, "footprint") || strings.Contains(value, "symbol"):
		return "missing_component_evidence"
	case strings.Contains(value, "block"):
		return "missing_block_evidence"
	case strings.Contains(value, "placement"):
		return "placement_blocked"
	case strings.Contains(value, "routing") || strings.Contains(value, "route"):
		return "routing_blocked"
	case strings.Contains(value, "fabrication") || strings.Contains(value, "fab"):
		return "fabrication_not_proven"
	case strings.Contains(value, "kicad") || strings.Contains(value, "external tool"):
		return "external_tool_missing"
	case strings.Contains(value, "roundtrip") || strings.Contains(value, "preservation"):
		return "preservation_unsupported"
	case strings.Contains(value, "unsupported"):
		return "unsupported_intent"
	default:
		return "validation_blocked"
	}
}

func limitFromIssue(id string, stage string, issue reports.Issue) KnownLimit {
	message := issue.Message
	if stage != "" {
		message = string(stage) + ": " + message
	}
	return KnownLimit{
		ID:         id,
		Category:   categoryForIssue(issue),
		Severity:   severityString(issue.Severity),
		Path:       issue.Path,
		Message:    message,
		Suggestion: issue.Suggestion,
	}
}

func categoryForIssue(issue reports.Issue) string {
	switch issue.Code {
	case reports.CodeMissingFootprint, reports.CodeUnknownFootprintLibrary, reports.CodeUnknownSymbolLibrary, reports.CodePinmapUnverified:
		return "missing_component_evidence"
	case reports.CodePlacementCollision, reports.CodePlacementOutsideBoard:
		return "placement_blocked"
	case reports.CodeDisconnectedPad, reports.CodeInvalidNetAssignment, reports.CodeMissingBoardOutline:
		return "validation_blocked"
	case reports.CodeKiCadCLIFailed, reports.CodeSkippedExternalTool:
		return "external_tool_missing"
	case reports.CodeRoundTripDiff, reports.CodePreservationConflict, reports.CodeUnsupportedImportedObject:
		return "preservation_unsupported"
	case reports.CodeUnsupportedOperation:
		return "unsupported_intent"
	default:
		return categoryForPath(issue.Path, issue.Message)
	}
}

func nextActions(report Report) []NextAction {
	existing := map[string]struct{}{}
	var out []NextAction
	for _, action := range report.NextActions {
		key := action.Action + "|" + action.Reason
		existing[key] = struct{}{}
		out = append(out, action)
	}
	for _, limit := range report.KnownLimits {
		action := actionForLimit(limit)
		if action.Action == "" {
			continue
		}
		key := action.Action + "|" + action.Reason
		if _, ok := existing[key]; ok {
			continue
		}
		existing[key] = struct{}{}
		action.ID = fmt.Sprintf("next:%03d", len(out)+1)
		out = append(out, action)
	}
	return out
}

func actionForLimit(limit KnownLimit) NextAction {
	action := NextAction{Priority: priorityForLimit(limit), Reason: limit.Message, TargetPath: limit.Path}
	if limit.Suggestion != "" {
		action.Action = limit.Suggestion
		return action
	}
	switch limit.Category {
	case "missing_component_evidence":
		action.Action = "add or verify component library evidence for the selected part"
	case "missing_block_evidence":
		action.Action = "verify the selected circuit block or choose a supported block"
	case "placement_blocked":
		action.Action = "adjust board dimensions, component anchors, or placement constraints"
	case "routing_blocked":
		action.Action = "relax routing constraints or rerun routing with repair enabled"
	case "external_tool_missing":
		action.Action = "run with KiCad CLI configured to collect external validation evidence"
	case "fabrication_not_proven":
		action.Action = "run fabrication readiness checks before treating this as manufacturable"
	case "unsupported_intent":
		action.Action = "clarify or narrow the requested design intent"
	case "preservation_unsupported":
		action.Action = "inspect unsupported KiCad content before rewriting the project"
	case "validation_blocked":
		action.Action = "resolve validation issues before trusting the design electrically"
	}
	return action
}

func priorityForLimit(limit KnownLimit) int {
	switch limit.Severity {
	case string(reports.SeverityError), string(reports.SeverityBlocked):
		return 1
	case string(reports.SeverityWarning):
		return 2
	default:
		return 3
	}
}

func priorityForSeverity(severity reports.Severity) int {
	switch severity {
	case reports.SeverityError, reports.SeverityBlocked:
		return 1
	case reports.SeverityWarning:
		return 2
	default:
		return 3
	}
}

func severityString(severity reports.Severity) string {
	if severity == "" {
		return string(reports.SeverityWarning)
	}
	return string(severity)
}

func connectionSummary(from string, to string, alias string) string {
	value := strings.TrimSpace(from + " -> " + to)
	if alias != "" {
		value += " (" + alias + ")"
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func compactStrings(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
