package routing

import (
	"fmt"
	"path/filepath"
	"strings"

	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
)

func AttachCheckResult(result Result, check checks.CheckResult) Result {
	out := result
	checkIssues := IssuesFromCheckResult(check)
	if len(checkIssues) == 0 {
		return out
	}
	out.Issues = make([]reports.Issue, 0, len(result.Issues)+len(checkIssues))
	out.Issues = append(out.Issues, result.Issues...)
	out.Issues = append(out.Issues, checkIssues...)
	if hasBlockingIssue(out.Issues) {
		out.Status = StatusBlocked
	}
	return out
}

func IssuesFromCheckResult(check checks.CheckResult) []reports.Issue {
	issueCapacity := len(check.Findings) + len(check.ParserIssues) + len(check.ContextWarnings)
	if check.Status == checks.CheckStatusError {
		issueCapacity++
	}
	issues := make([]reports.Issue, 0, issueCapacity)
	for _, finding := range check.Findings {
		issues = append(issues, IssueFromCheckFinding(finding))
	}
	for _, parserIssue := range check.ParserIssues {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityWarning,
			Path:     string(check.Kind) + "/parser",
			Message:  "KiCad " + strings.ToUpper(string(check.Kind)) + " parser issue: " + parserIssue.Message,
		})
	}
	for _, warning := range check.ContextWarnings {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   reports.SeverityWarning,
			Path:       firstNonEmptyString(warning.Path, string(check.Kind)+"/context"),
			Message:    warning.Message,
			Suggestion: "Run KiCad checks with full project context before treating this result as final.",
		})
	}
	if check.Status == checks.CheckStatusError {
		message := strings.TrimSpace(check.Stderr)
		if message == "" {
			message = strings.TrimSpace(check.Stdout)
		}
		if message == "" {
			message = "KiCad " + strings.ToUpper(string(check.Kind)) + " failed"
			if len(check.Command) != 0 {
				message += ": " + filepath.Base(check.Command[0])
			}
			if check.ExitCode != 0 {
				message += fmt.Sprintf(" (exit code %d)", check.ExitCode)
			}
		}
		issues = append(issues, reports.Issue{
			Code:       reports.CodeKiCadCLIFailed,
			Severity:   reports.SeverityBlocked,
			Path:       string(check.Kind),
			Message:    message,
			Suggestion: "Fix the KiCad CLI invocation or generated project files, then rerun validation.",
		})
	}
	return issues
}

func IssueFromCheckFinding(finding checks.CheckFinding) reports.Issue {
	return reports.Issue{
		Code:       codeForCheckFinding(finding),
		Severity:   severityForCheckFinding(finding),
		Path:       pathForCheckFinding(finding),
		Message:    messageForCheckFinding(finding),
		Refs:       append([]string(nil), finding.References...),
		Nets:       netsForCheckFinding(finding),
		Suggestion: suggestionForRepairCategory(finding.RepairCategory),
	}
}

func codeForCheckFinding(finding checks.CheckFinding) reports.Code {
	switch finding.RepairCategory {
	case checks.RepairConnectivity:
		return reports.CodeDisconnectedPad
	case checks.RepairOutline:
		return reports.CodeMissingBoardOutline
	case checks.RepairFootprint:
		return reports.CodeMissingFootprint
	case checks.RepairClearance, checks.RepairPower, checks.RepairNoConnect, checks.RepairMetadata:
		return reports.CodeValidationFailed
	default:
		return reports.CodeValidationFailed
	}
}

func severityForCheckFinding(finding checks.CheckFinding) reports.Severity {
	switch strings.ToLower(strings.TrimSpace(finding.Severity)) {
	case "critical", "fatal", "error":
		return reports.SeverityBlocked
	case "warning", "warn":
		return reports.SeverityWarning
	case "info", "note", "ignore", "ignored", "exclusion", "excluded":
		return reports.SeverityInfo
	default:
		return reports.SeverityBlocked
	}
}

func pathForCheckFinding(finding checks.CheckFinding) string {
	parts := []string{string(finding.Kind)}
	if finding.File != "" {
		parts = append(parts, finding.File)
	}
	if finding.Layer != "" {
		parts = append(parts, finding.Layer)
	}
	if finding.ID != "" {
		parts = append(parts, finding.ID)
	}
	return strings.Join(parts, "/")
}

func messageForCheckFinding(finding checks.CheckFinding) string {
	kind := strings.ToUpper(string(finding.Kind))
	rule := firstNonEmptyString(finding.Rule, finding.Code)
	if rule == "" {
		return fmt.Sprintf("KiCad %s finding: %s", kind, finding.Message)
	}
	return fmt.Sprintf("KiCad %s %s: %s", kind, rule, finding.Message)
}

func netsForCheckFinding(finding checks.CheckFinding) []string {
	nets := make([]string, 0, len(finding.Nets)+1+len(finding.Objects))
	seen := map[string]struct{}{}
	for _, net := range finding.Nets {
		nets = appendUniqueString(nets, seen, net)
	}
	if finding.Net != "" {
		nets = appendUniqueString(nets, seen, finding.Net)
	}
	for _, object := range finding.Objects {
		nets = appendUniqueString(nets, seen, object.Net)
	}
	return nets
}

func suggestionForRepairCategory(category checks.RepairCategory) string {
	switch category {
	case checks.RepairClearance:
		return "Increase spacing, reduce trace width, move placement, or change route topology before rerunning DRC."
	case checks.RepairConnectivity:
		return "Inspect route endpoints, net assignments, vias, and remaining unrouted connections."
	case checks.RepairOutline:
		return "Add or repair a closed Edge.Cuts board outline before rerunning DRC."
	case checks.RepairFootprint:
		return "Verify footprint selection, pad geometry, drill sizes, and pad-to-net assignments."
	case checks.RepairPower:
		return "Add an explicit power source, power flag, or corrected power net assignment."
	case checks.RepairNoConnect:
		return "Add intentional no-connect markers or connect the reported pins."
	case checks.RepairMetadata:
		return "Fix library links, text variables, fields, or project metadata referenced by KiCad."
	default:
		return "Review the KiCad finding and update the design before rerunning validation."
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func appendUniqueString(values []string, seen map[string]struct{}, next string) []string {
	nextKey := strings.TrimSpace(next)
	if nextKey == "" {
		return values
	}
	if _, ok := seen[nextKey]; ok {
		return values
	}
	seen[nextKey] = struct{}{}
	return append(values, nextKey)
}
