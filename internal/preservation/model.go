package preservation

import (
	"sort"
	"strings"

	"kicadai/internal/reports"
)

type Status string

const (
	StatusClean   Status = "clean"
	StatusWarning Status = "warning"
	StatusBlocked Status = "blocked"
)

type Scope string

const (
	ScopeGenerated Scope = "generated"
	ScopeImported  Scope = "imported"
	ScopeMixed     Scope = "mixed"
	ScopeUnknown   Scope = "unknown"
)

type Ownership string

const (
	OwnershipGeneratedOwned   Ownership = "generated_owned"
	OwnershipImportedUser     Ownership = "imported_user"
	OwnershipPreservationOnly Ownership = "preservation_only"
	OwnershipUnknown          Ownership = "unknown"
	OwnershipNewOperation     Ownership = "new_operation"
)

type Mutability string

const (
	MutabilityReadOnly Mutability = "read_only"
	MutabilityPlanOnly Mutability = "plan_only"
	MutabilitySafeAdd  Mutability = "safe_add"
	MutabilityUnsafe   Mutability = "unsafe"
)

type Report struct {
	Status           Status            `json:"status"`
	Scope            Scope             `json:"scope"`
	Summary          Summary           `json:"summary"`
	Files            []File            `json:"files,omitempty"`
	Objects          []Object          `json:"objects,omitempty"`
	OperationReviews []OperationReview `json:"operation_reviews,omitempty"`
	Issues           []reports.Issue   `json:"issues,omitempty"`
}

type Summary struct {
	Files              int `json:"files"`
	PreservationOnly   int `json:"preservation_only"`
	Unsupported        int `json:"unsupported"`
	SafeAddOperations  int `json:"safe_add_operations,omitempty"`
	BlockedOperations  int `json:"blocked_operations,omitempty"`
	PlanOnlyOperations int `json:"plan_only_operations,omitempty"`
}

type File struct {
	Path       string     `json:"path"`
	Kind       string     `json:"kind"`
	Ownership  Ownership  `json:"ownership"`
	Mutability Mutability `json:"mutability"`
	Objects    int        `json:"objects,omitempty"`
}

type Object struct {
	Path       string     `json:"path"`
	Kind       string     `json:"kind"`
	Count      int        `json:"count"`
	Ownership  Ownership  `json:"ownership"`
	Mutability Mutability `json:"mutability"`
	Message    string     `json:"message,omitempty"`
}

type OperationReview struct {
	OperationID string          `json:"operation_id,omitempty"`
	Index       int             `json:"index"`
	Op          string          `json:"op"`
	Status      Status          `json:"status"`
	Ownership   Ownership       `json:"ownership"`
	Mutability  Mutability      `json:"mutability"`
	Reason      string          `json:"reason,omitempty"`
	Issues      []reports.Issue `json:"issues,omitempty"`
}

func New(scope Scope) Report {
	return Report{Status: StatusClean, Scope: normalizeScope(scope)}
}

func (report *Report) Normalize() {
	baseIssues := append([]reports.Issue(nil), report.Issues...)
	report.Scope = normalizeScope(report.Scope)
	for index := range report.Objects {
		if report.Objects[index].Count <= 0 {
			report.Objects[index].Count = 1
		}
		if report.Objects[index].Ownership == "" {
			report.Objects[index].Ownership = OwnershipUnknown
		}
		if report.Objects[index].Mutability == "" {
			report.Objects[index].Mutability = MutabilityReadOnly
		}
	}
	for index := range report.Files {
		if report.Files[index].Ownership == "" {
			report.Files[index].Ownership = OwnershipUnknown
		}
		if report.Files[index].Mutability == "" {
			report.Files[index].Mutability = MutabilityReadOnly
		}
	}
	for index := range report.OperationReviews {
		review := &report.OperationReviews[index]
		if review.Status == "" {
			review.Status = StatusClean
		}
		if review.Ownership == "" {
			review.Ownership = OwnershipNewOperation
		}
		if review.Mutability == "" {
			review.Mutability = MutabilityReadOnly
		}
		review.Status = statusForIssues(review.Status, review.Issues)
		if review.Mutability == MutabilityUnsafe {
			review.Status = StatusBlocked
		}
	}
	report.Summary = Summary{}
	report.Issues = nil
	seenIssues := map[string]struct{}{}
	for _, issue := range baseIssues {
		key := issueKey(issue)
		if _, exists := seenIssues[key]; !exists {
			report.Issues = append(report.Issues, issue)
			seenIssues[key] = struct{}{}
		}
	}
	report.Summary.Files = len(report.Files)
	for _, file := range report.Files {
		switch file.Ownership {
		case OwnershipPreservationOnly:
			report.Summary.PreservationOnly++
		case OwnershipUnknown:
			report.Summary.Unsupported++
		}
	}
	for _, object := range report.Objects {
		switch object.Ownership {
		case OwnershipPreservationOnly:
			report.Summary.PreservationOnly += object.Count
		case OwnershipUnknown:
			report.Summary.Unsupported += object.Count
		}
	}
	for _, review := range report.OperationReviews {
		switch review.Mutability {
		case MutabilitySafeAdd:
			report.Summary.SafeAddOperations++
		case MutabilityPlanOnly:
			report.Summary.PlanOnlyOperations++
		case MutabilityUnsafe:
			report.Summary.BlockedOperations++
		}
		for _, issue := range review.Issues {
			key := issueKey(issue)
			if _, exists := seenIssues[key]; !exists {
				report.Issues = append(report.Issues, issue)
				seenIssues[key] = struct{}{}
			}
		}
	}
	sort.SliceStable(report.Files, func(i, j int) bool {
		return report.Files[i].Path < report.Files[j].Path
	})
	sort.SliceStable(report.Objects, func(i, j int) bool {
		if report.Objects[i].Path == report.Objects[j].Path {
			return report.Objects[i].Kind < report.Objects[j].Kind
		}
		return report.Objects[i].Path < report.Objects[j].Path
	})
	report.Status = statusForIssues(StatusClean, report.Issues)
	if report.Summary.BlockedOperations > 0 {
		report.Status = StatusBlocked
	}
	if report.Status == StatusClean && report.Summary.Unsupported > 0 {
		report.Status = StatusWarning
	}
}

func issueKey(issue reports.Issue) string {
	return strings.Join([]string{
		string(issue.Code),
		string(issue.Severity),
		issue.Path,
		issue.Message,
		issue.OperationID,
	}, "\x00")
}

func (report Report) HasBlockedOperation() bool {
	for _, review := range report.OperationReviews {
		if review.Status == StatusBlocked || review.Mutability == MutabilityUnsafe {
			return true
		}
	}
	return false
}

func OperationReviewFor(index int, op string, operationID string, mutability Mutability, reason string, issues []reports.Issue) OperationReview {
	review := OperationReview{
		OperationID: strings.TrimSpace(operationID),
		Index:       index,
		Op:          strings.TrimSpace(op),
		Status:      statusForIssues(StatusClean, issues),
		Ownership:   OwnershipNewOperation,
		Mutability:  mutability,
		Reason:      strings.TrimSpace(reason),
		Issues:      append([]reports.Issue(nil), issues...),
	}
	if review.Mutability == "" {
		review.Mutability = MutabilityReadOnly
	}
	if review.Mutability == MutabilityUnsafe {
		review.Status = StatusBlocked
	}
	return review
}

func statusForIssues(base Status, issues []reports.Issue) Status {
	status := normalizeStatus(base)
	for _, issue := range issues {
		switch issue.Severity {
		case reports.SeverityBlocked:
			return StatusBlocked
		case reports.SeverityError:
			if status != StatusBlocked {
				status = StatusBlocked
			}
		case reports.SeverityWarning:
			if status == StatusClean {
				status = StatusWarning
			}
		}
	}
	return status
}

func normalizeScope(scope Scope) Scope {
	switch scope {
	case ScopeGenerated, ScopeImported, ScopeMixed:
		return scope
	default:
		return ScopeUnknown
	}
}

func normalizeStatus(status Status) Status {
	switch status {
	case StatusWarning, StatusBlocked:
		return status
	default:
		return StatusClean
	}
}
