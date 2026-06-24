package repair

import (
	"fmt"
	"path/filepath"
	"strings"

	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
)

func NormalizeKiCadCheckResult(result checks.CheckResult, adapter string) []NormalizedFinding {
	findings := make([]NormalizedFinding, 0, len(result.Findings)+len(result.ParserIssues)+len(result.ContextWarnings)+1)
	for _, finding := range result.Findings {
		findings = append(findings, NormalizeKiCadCheckFinding(finding, result, adapter))
	}
	for _, parserIssue := range result.ParserIssues {
		findings = append(findings, NormalizeKiCadParserIssue(parserIssue, result, adapter))
	}
	for _, warning := range result.ContextWarnings {
		findings = append(findings, NormalizeKiCadContextWarning(warning, result, adapter))
	}
	if result.Status == checks.CheckStatusError && len(result.ParserIssues) == 0 {
		issue := reports.Issue{
			Code:     reports.CodeKiCadCLIFailed,
			Severity: reports.SeverityError,
			Path:     firstNonEmptySlashPath(result.TargetPath, string(result.Kind)),
			Message:  kicadToolErrorMessage(result),
		}
		findings = append(findings, NormalizeIssue(issue, NormalizeFindingOptions{
			Source:        findingSourceForCheckKind(result.Kind),
			Adapter:       adapterForCheckKind(adapter, result.Kind),
			Category:      FindingCategoryExternalTool,
			Repairability: RepairabilityExternalToolBlocked,
			EvidencePath:  result.ReportPath,
			RawCode:       "tool_error",
		}))
	}
	SortNormalizedFindings(findings)
	return findings
}

func NormalizeKiCadCheckFinding(finding checks.CheckFinding, result checks.CheckResult, adapter string) NormalizedFinding {
	if finding.Kind == "" {
		finding.Kind = result.Kind
	}
	category := findingCategoryForKiCadFinding(finding)
	issue := reports.Issue{
		Code:        reportCodeForKiCadFinding(finding),
		Severity:    kicadFindingSeverity(finding.Severity),
		Path:        firstNonEmptySlashPath(finding.File, result.TargetPath),
		Message:     finding.Message,
		Refs:        append([]string(nil), finding.References...),
		Nets:        kicadFindingNets(finding),
		OperationID: finding.ID,
	}
	return NormalizeIssue(issue, NormalizeFindingOptions{
		Source:        findingSourceForCheckKind(finding.Kind),
		Adapter:       adapterForCheckKind(adapter, finding.Kind),
		Category:      category,
		Repairability: repairabilityForKiCadFinding(finding, category),
		EvidencePath:  result.ReportPath,
		RawCode:       firstNonEmptyStringValue(finding.Rule, finding.Code, string(finding.RepairCategory)),
		Subject:       subjectForKiCadFinding(finding),
	})
}

func NormalizeKiCadParserIssue(parserIssue checks.ParserIssue, result checks.CheckResult, adapter string) NormalizedFinding {
	issue := reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     firstNonEmptySlashPath(result.ReportPath, result.TargetPath, string(result.Kind)+"/parser"),
		Message:  parserIssue.Message,
	}
	return NormalizeIssue(issue, NormalizeFindingOptions{
		Source:        findingSourceForCheckKind(result.Kind),
		Adapter:       adapterForCheckKind(adapter, result.Kind),
		Category:      FindingCategoryParse,
		Repairability: RepairabilityExternalToolBlocked,
		EvidencePath:  result.ReportPath,
		RawCode:       "parser_issue",
		Subject:       FindingSubject{File: result.ReportPath},
	})
}

func NormalizeKiCadContextWarning(warning checks.ContextWarning, result checks.CheckResult, adapter string) NormalizedFinding {
	issue := reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityWarning,
		Path:     firstNonEmptySlashPath(warning.Path, result.TargetPath, string(result.Kind)+"/context"),
		Message:  warning.Message,
	}
	return NormalizeIssue(issue, NormalizeFindingOptions{
		Source:        findingSourceForCheckKind(result.Kind),
		Adapter:       adapterForCheckKind(adapter, result.Kind),
		Category:      FindingCategoryExternalTool,
		Repairability: RepairabilityInformational,
		EvidencePath:  result.ReportPath,
		RawCode:       firstNonEmptyStringValue(warning.Code, "context_warning"),
	})
}

func NormalizeKiCadUnavailable(issue reports.Issue, kind checks.CheckKind, adapter string) NormalizedFinding {
	return NormalizeIssue(issue, NormalizeFindingOptions{
		Source:        findingSourceForCheckKind(kind),
		Adapter:       adapterForCheckKind(adapter, kind),
		Category:      FindingCategoryExternalTool,
		Repairability: RepairabilityExternalToolBlocked,
		RawCode:       "missing_kicad_cli",
	})
}

func findingSourceForCheckKind(kind checks.CheckKind) FindingSource {
	if kind == checks.CheckKindERC {
		return FindingSourceKiCadERC
	}
	return FindingSourceKiCadDRC
}

func adapterForCheckKind(adapter string, kind checks.CheckKind) string {
	adapter = strings.TrimSpace(adapter)
	if adapter != "" {
		return adapter
	}
	if kind == checks.CheckKindERC {
		return postValidatorKiCadERC
	}
	return postValidatorKiCadDRC
}

func findingCategoryForKiCadFinding(finding checks.CheckFinding) FindingCategory {
	if finding.Kind == checks.CheckKindERC {
		return FindingCategorySchematicERC
	}
	rule := strings.ToLower(finding.Rule)
	code := strings.ToLower(finding.Code)
	message := strings.ToLower(finding.Message)
	switch finding.RepairCategory {
	case checks.RepairConnectivity, checks.RepairNetAssignment:
		return FindingCategoryConnectivity
	case checks.RepairClearance:
		if containsAnyLowerEvidenceText(rule, code, message, "zone", "filled area") {
			return FindingCategoryZone
		}
		return FindingCategoryRoute
	case checks.RepairOutline:
		return FindingCategoryOutline
	case checks.RepairFootprint:
		return FindingCategoryPadNet
	case checks.RepairPower, checks.RepairNoConnect, checks.RepairMetadata:
		return FindingCategoryBoardDRC
	default:
		if containsAnyLowerEvidenceText(rule, code, message, "zone", "copper pour") {
			return FindingCategoryZone
		}
		if containsAnyLowerEvidenceText(rule, code, message, "edge cuts", "outline") {
			return FindingCategoryOutline
		}
		if containsAnyLowerEvidenceText(rule, code, message, "clearance", "track", "via", "route") {
			return FindingCategoryRoute
		}
		return FindingCategoryBoardDRC
	}
}

func reportCodeForKiCadFinding(finding checks.CheckFinding) reports.Code {
	switch finding.RepairCategory {
	case checks.RepairConnectivity:
		return reports.CodeDisconnectedPad
	case checks.RepairNetAssignment:
		return reports.CodeInvalidNetAssignment
	case checks.RepairOutline:
		return reports.CodeMissingBoardOutline
	case checks.RepairFootprint:
		return reports.CodeMissingFootprint
	default:
		return reports.CodeValidationFailed
	}
}

func repairabilityForKiCadFinding(finding checks.CheckFinding, category FindingCategory) Repairability {
	if kicadFindingSeverity(finding.Severity) == reports.SeverityInfo {
		return RepairabilityInformational
	}
	if finding.RepairCategory == checks.RepairUnknown && category == FindingCategoryBoardDRC {
		return RepairabilityUnsupported
	}
	return RepairabilityRepairable
}

func subjectForKiCadFinding(finding checks.CheckFinding) FindingSubject {
	subject := FindingSubject{
		Ref:      firstNonEmptyStringValue(finding.References...),
		Net:      firstNonEmptyStringValue(append([]string{finding.Net}, finding.Nets...)...),
		Layer:    finding.Layer,
		File:     finding.File,
		Rule:     firstNonEmptyStringValue(finding.Rule, finding.Code),
		Location: locationSubject(finding.Location),
	}
	for _, object := range finding.Objects {
		if subject.Ref == "" {
			subject.Ref = object.Reference
		}
		if subject.Pad == "" {
			subject.Pad = object.Pad
		}
		if subject.Net == "" {
			subject.Net = object.Net
		}
		if subject.Layer == "" {
			subject.Layer = object.Layer
		}
	}
	return subject
}

func locationSubject(location *checks.CheckLocation) string {
	if location == nil {
		return ""
	}
	units := strings.TrimSpace(location.Units)
	if units == "" {
		units = "mm"
	}
	return strings.TrimSpace(strings.Join([]string{
		fmt.Sprintf("%.6f", location.X),
		fmt.Sprintf("%.6f", location.Y),
		units,
	}, ","))
}

func kicadToolErrorMessage(result checks.CheckResult) string {
	message := strings.TrimSpace(result.Stderr)
	if message == "" {
		message = strings.TrimSpace(result.Stdout)
	}
	if message == "" {
		message = "KiCad " + strings.ToUpper(string(result.Kind)) + " failed"
	}
	return message
}

func firstNonEmptyStringValue(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptySlashPath(values ...string) string {
	return filepath.ToSlash(firstNonEmptyStringValue(values...))
}

func containsAnyLowerEvidenceText(rule string, code string, message string, needles ...string) bool {
	for _, value := range []string{rule, code, message} {
		for _, needle := range needles {
			if strings.Contains(value, needle) {
				return true
			}
		}
	}
	return false
}
