package placement

import (
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type ValidationReport struct {
	Ready             bool                          `json:"ready"`
	RequestIssues     []reports.Issue               `json:"request_issues"`
	GeometryIssues    []reports.Issue               `json:"geometry_issues"`
	GroupIssues       []reports.Issue               `json:"group_issues"`
	TransactionResult transactions.ValidationResult `json:"transaction_result"`
	Issues            []reports.Issue               `json:"-"`
}

func ValidateResult(request *Request, result *Result) ValidationReport {
	if request == nil || result == nil {
		issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "placement.validation", Message: "request and result are required"}
		return ValidationReport{Ready: false, Issues: []reports.Issue{issue}, RequestIssues: []reports.Issue{issue}}
	}
	successful := successfulPlacementResults(result.Placements)
	report := ValidationReport{
		RequestIssues:  Validate(*request),
		GeometryIssues: ValidateGeometry(*request, successful),
		GroupIssues:    ValidateGroups(*request, successful),
	}
	if len(result.Operations) > 0 {
		report.TransactionResult = transactions.Validate(transactions.Transaction{Operations: result.Operations})
	}
	totalIssues := len(report.RequestIssues) + len(report.GeometryIssues) + len(report.GroupIssues) + len(report.TransactionResult.Issues)
	report.Issues = make([]reports.Issue, 0, totalIssues)
	report.Issues = append(report.Issues, report.RequestIssues...)
	report.Issues = append(report.Issues, report.GeometryIssues...)
	report.Issues = append(report.Issues, report.GroupIssues...)
	report.Issues = append(report.Issues, report.TransactionResult.Issues...)
	report.Ready = !reports.HasBlockingIssue(report.Issues) && result.Status == StatusPlaced
	return report
}
