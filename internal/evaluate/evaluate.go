package evaluate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/inspect"
	"kicadai/internal/reports"
)

type CodedError struct {
	Code reports.Code
	Err  error
}

func (err *CodedError) Error() string {
	if err == nil {
		return ""
	}
	if err.Err == nil {
		return string(err.Code)
	}
	return err.Err.Error()
}

func (err *CodedError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

func WithCode(err error, code reports.Code) error {
	return &CodedError{Code: code, Err: err}
}

func Project(path string) (Report, error) {
	if strings.TrimSpace(path) == "" {
		return Report{}, fmt.Errorf("project path required")
	}
	summary, err := inspect.Project(path)
	if err != nil {
		return Report{}, err
	}
	report := newReport(summary.Root)
	report.InspectionSummaryPresent = true
	if summary.Manifest.Present {
		status := CheckPassed
		issues := []reports.Issue{}
		if summary.Manifest.Stale {
			status = CheckBlocked
			message := "generated-project manifest is stale"
			if len(summary.Manifest.Issues) > 0 {
				message = message + ": " + strings.Join(summary.Manifest.Issues, "; ")
			}
			issues = append(issues, reports.Issue{
				Code:     reports.CodePreservationConflict,
				Severity: reports.SeverityBlocked,
				Path:     "manifest",
				Message:  message,
			})
		}
		report.addCheck(CheckResult{Name: "generated_manifest", Status: status, Required: false, Issues: issues})
	}
	check := CheckResult{Name: "project_structure", Status: CheckPassed, Required: true}
	for _, issue := range summary.Issues {
		if issue.Code == reports.CodeMissingFile {
			issue.Severity = reports.SeverityError
		}
		check.Issues = append(check.Issues, issue)
	}
	check.Status = statusForIssues(check.Issues)
	report.addCheck(check)
	if summary.Schematic != nil {
		report.mergeChecks(checksForSchematicSummary(*summary.Schematic)...)
	}
	if summary.PCB != nil {
		report.mergeChecks(checksForPCBSummary(*summary.PCB)...)
	}
	report.finish()
	return report, nil
}

func Schematic(path string) (Report, error) {
	if strings.TrimSpace(path) == "" {
		return Report{}, fmt.Errorf("schematic path required")
	}
	summary, err := inspect.Schematic(path)
	if err != nil {
		return Report{}, err
	}
	report := newReport(path)
	report.InspectionSummaryPresent = true
	report.mergeChecks(checksForSchematicSummary(summary)...)
	report.finish()
	return report, nil
}

func PCB(path string) (Report, error) {
	if strings.TrimSpace(path) == "" {
		return Report{}, fmt.Errorf("pcb path required")
	}
	summary, err := inspect.PCB(path)
	if err != nil {
		return Report{}, err
	}
	report := newReport(path)
	report.InspectionSummaryPresent = true
	report.mergeChecks(checksForPCBSummary(summary)...)
	report.finish()
	return report, nil
}

func checksForSchematicSummary(summary inspect.SchematicSummary) []CheckResult {
	return []CheckResult{
		{Name: "schematic_parse", Status: statusForIssues(summary.Issues), Required: true, Issues: summary.Issues},
		schematicReaderGapCheck(summary.Path),
	}
}

func checksForPCBSummary(summary inspect.PCBSummary) []CheckResult {
	scanIssues := append([]reports.Issue{}, summary.Issues...)
	for index := range scanIssues {
		if scanIssues[index].Code == reports.CodeMissingBoardOutline {
			scanIssues[index].Severity = reports.SeverityError
		}
	}
	for _, unsupported := range summary.Unsupported {
		scanIssues = append(scanIssues, reports.Issue{
			Code:     reports.CodeUnsupportedOperation,
			Severity: reports.SeverityWarning,
			Path:     "pcb." + unsupported.Kind,
			Message:  fmt.Sprintf("unsupported PCB node %q appears %d time(s)", unsupported.Kind, unsupported.Count),
		})
	}
	return []CheckResult{
		{
			Name:     "pcb_corpus_scan",
			Status:   statusForIssues(scanIssues),
			Required: true,
			Issues:   scanIssues,
		},
		{
			Name:   "pcb_reader_gap",
			Status: CheckSkipped,
			Issues: []reports.Issue{{
				Code:       reports.CodeUnsupportedOperation,
				Severity:   reports.SeverityWarning,
				Path:       "pcb.reader",
				Message:    "full structured PCB reader is not implemented; evaluation is limited to corpus scan checks",
				Suggestion: "use generated design validation or KiCad CLI round-trip checks for stronger validation",
			}},
		},
	}
}

func IssueFromError(err error, path string) reports.Issue {
	if err == nil {
		return reports.Issue{}
	}
	normalizedPath := filepath.ToSlash(path)
	if issue, ok := reports.IssueFromError(err); ok {
		if issue.Code == "" || issue.Code == reports.CodeUnknown || issue.Code == reports.CodeValidationFailed {
			issue.Code = codeForError(err)
		}
		if issue.Path == "" {
			issue.Path = normalizedPath
		} else {
			issue.Path = filepath.ToSlash(issue.Path)
		}
		return issue
	}
	return reports.Issue{
		Code:     codeForError(err),
		Severity: reports.SeverityError,
		Path:     normalizedPath,
		Message:  err.Error(),
	}
}

func codeForError(err error) reports.Code {
	if err == nil {
		return reports.CodeUnknown
	}
	if errors.Is(err, os.ErrNotExist) {
		return reports.CodeMissingFile
	}
	var coded *CodedError
	if errors.As(err, &coded) && coded.Code != "" {
		return coded.Code
	}
	return reports.CodeValidationFailed
}

func schematicReaderGapCheck(path string) CheckResult {
	return CheckResult{
		Name:   "schematic_reader_gap",
		Status: CheckSkipped,
		Issues: []reports.Issue{{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityWarning,
			Path:       filepath.ToSlash(path),
			Message:    "full structured schematic reader is not implemented; semantic schematic evaluation is skipped",
			Suggestion: "use generated design validation for modeled schematics until the reader gap is closed",
		}},
	}
}

func newReport(target string) Report {
	return Report{
		Target: filepath.ToSlash(target),
		Checks: []CheckResult{},
		Issues: []reports.Issue{},
	}
}

func (report *Report) addCheck(check CheckResult) {
	if check.Issues == nil {
		check.Issues = []reports.Issue{}
	}
	if check.Artifacts == nil {
		check.Artifacts = []reports.Artifact{}
	}
	report.Checks = append(report.Checks, check)
	report.Issues = append(report.Issues, check.Issues...)
}

func (report *Report) mergeChecks(checks ...CheckResult) {
	for _, check := range checks {
		report.addCheck(check)
	}
}

func (report *Report) finish() {
	if reports.HasBlockingIssue(report.Issues) {
		report.FabricationReady = false
		report.FabricationReadyReason = "blocking evaluation issues remain"
		return
	}
	for _, check := range report.Checks {
		if check.Status == CheckFailed || check.Status == CheckBlocked || (check.Required && check.Status == CheckSkipped) {
			report.FabricationReady = false
			report.FabricationReadyReason = "one or more required checks failed, were skipped, or were blocked"
			return
		}
	}
	report.FabricationReady = true
}

func statusForIssues(issues []reports.Issue) CheckStatus {
	if len(issues) == 0 {
		return CheckPassed
	}
	for _, issue := range issues {
		if issue.Severity == reports.SeverityBlocked {
			return CheckBlocked
		}
		if issue.Severity == reports.SeverityError {
			return CheckFailed
		}
	}
	return CheckPassed
}
