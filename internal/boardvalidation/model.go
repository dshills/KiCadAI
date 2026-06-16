package boardvalidation

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/reports"
)

type Status string

const (
	StatusPass    Status = "pass"
	StatusFail    Status = "fail"
	StatusSkipped Status = "skipped"
	StatusError   Status = "error"
)

const (
	CheckPCBStructuralValidation = "pcb_structural_validation"
	CheckNetToPadValidation      = "net_to_pad_validation"
	CheckGeneratedConnectivity   = "generated_connectivity"
	CheckUnroutedNetValidation   = "unrouted_net_validation"
	CheckRouteCompletion         = "route_completion"
	CheckZoneValidation          = "zone_validation"
	CheckKiCadDRC                = "kicad_drc"
)

var checkPriority = map[string]int{
	CheckPCBStructuralValidation: 0,
	CheckNetToPadValidation:      1,
	CheckGeneratedConnectivity:   2,
	CheckUnroutedNetValidation:   3,
	CheckRouteCompletion:         4,
	CheckZoneValidation:          5,
	CheckKiCadDRC:                6,
}

type Options struct {
	StrictZones     bool
	StrictUnrouted  bool
	RequireDRC      bool
	AllowMissingDRC bool
	KiCadCLI        string
	KeepArtifacts   bool
	ArtifactDir     string
	AllowlistPath   string
}

type Result struct {
	Target            string             `json:"target"`
	BoardPath         string             `json:"board_path,omitempty"`
	ProjectPath       string             `json:"project_path,omitempty"`
	Status            Status             `json:"status"`
	FabricationReady  bool               `json:"fabrication_ready"`
	FabricationReason string             `json:"fabrication_reason,omitempty"`
	Checks            []Check            `json:"checks"`
	Nets              []NetStatus        `json:"nets,omitempty"`
	Zones             []ZoneStatus       `json:"zones,omitempty"`
	Issues            []reports.Issue    `json:"issues"`
	Artifacts         []reports.Artifact `json:"artifacts"`
	Summary           Summary            `json:"summary"`
}

type Check struct {
	Name      string             `json:"name"`
	Status    Status             `json:"status"`
	Required  bool               `json:"required"`
	Issues    []reports.Issue    `json:"issues,omitempty"`
	Evidence  string             `json:"evidence,omitempty"`
	Artifacts []reports.Artifact `json:"artifacts,omitempty"`
}

type Summary struct {
	TotalChecks    int            `json:"total_checks"`
	PassedChecks   int            `json:"passed_checks"`
	FailedChecks   int            `json:"failed_checks"`
	SkippedChecks  int            `json:"skipped_checks"`
	ErrorChecks    int            `json:"error_checks"`
	TotalIssues    int            `json:"total_issues"`
	BlockingIssues int            `json:"blocking_issues"`
	ByCode         map[string]int `json:"by_code,omitempty"`
	ByNet          map[string]int `json:"by_net,omitempty"`
}

type NetRouteStatus string

const (
	NetStatusSingleEndpoint  NetRouteStatus = "single_endpoint"
	NetStatusUnconnected     NetRouteStatus = "unconnected"
	NetStatusPartiallyRouted NetRouteStatus = "partially_routed"
	NetStatusFullyRouted     NetRouteStatus = "fully_routed"
	NetStatusZoneDependent   NetRouteStatus = "zone_dependent"
	NetStatusIgnored         NetRouteStatus = "ignored"
)

type NetStatus struct {
	Code        int            `json:"code"`
	Name        string         `json:"name"`
	Status      NetRouteStatus `json:"status"`
	PadCount    int            `json:"pad_count"`
	CopperCount int            `json:"copper_count"`
	Refs        []string       `json:"refs,omitempty"`
	IssueCodes  []string       `json:"issue_codes,omitempty"`
}

type ZoneStatus struct {
	Name               string   `json:"name,omitempty"`
	NetCode            int      `json:"net_code"`
	NetName            string   `json:"net_name,omitempty"`
	Layers             []string `json:"layers,omitempty"`
	PolygonCount       int      `json:"polygon_count"`
	FilledPolygonCount int      `json:"filled_polygon_count"`
	Status             Status   `json:"status"`
	Evidence           string   `json:"evidence,omitempty"`
}

func NewResult(target string) Result {
	return Result{
		Target:    filepath.ToSlash(target),
		Checks:    []Check{},
		Nets:      []NetStatus{},
		Zones:     []ZoneStatus{},
		Issues:    []reports.Issue{},
		Artifacts: []reports.Artifact{},
	}
}

func (result *Result) AddCheck(check Check) {
	if check.Issues == nil {
		check.Issues = []reports.Issue{}
	}
	if check.Artifacts == nil {
		check.Artifacts = []reports.Artifact{}
	}
	if check.Status == "" {
		check.Status = statusForCheckIssues(check.Issues)
	}
	result.Checks = append(result.Checks, check)
	result.Issues = append(result.Issues, check.Issues...)
	result.Artifacts = append(result.Artifacts, check.Artifacts...)
}

func (result *Result) Finish() {
	SortChecks(result.Checks)
	result.Summary = Summarize(result.Checks, result.Issues)
	result.Status = AggregateStatus(result.Checks)
	result.FabricationReady = result.Status == StatusPass
	switch result.Status {
	case StatusPass:
		result.FabricationReason = "all required board-validation checks passed"
	case StatusSkipped:
		if result.hasRequiredCheck() {
			result.FabricationReason = "one or more required board-validation checks were skipped"
		} else {
			result.FabricationReason = "no board-validation checks produced usable evidence"
		}
	case StatusError:
		result.FabricationReason = "one or more board-validation checks could not complete"
	default:
		result.FabricationReason = "blocking board-validation issues remain"
	}
}

func AggregateStatus(checks []Check) Status {
	if len(checks) == 0 {
		return StatusSkipped
	}
	requiredSeen := false
	skippedRequired := false
	anyAttempted := false
	for _, check := range checks {
		if check.Status == StatusError {
			return StatusError
		}
		if check.Status == StatusFail {
			return StatusFail
		}
		if check.Status == StatusPass || check.Status == StatusFail {
			anyAttempted = true
		}
		if check.Required {
			requiredSeen = true
			if check.Status == StatusSkipped {
				skippedRequired = true
			}
		}
	}
	if skippedRequired {
		return StatusSkipped
	}
	if !requiredSeen && !anyAttempted {
		return StatusSkipped
	}
	return StatusPass
}

func Summarize(checks []Check, issues []reports.Issue) Summary {
	summary := Summary{
		TotalChecks: len(checks),
		TotalIssues: len(issues),
	}
	for _, check := range checks {
		switch check.Status {
		case StatusPass:
			summary.PassedChecks++
		case StatusFail:
			summary.FailedChecks++
		case StatusSkipped:
			summary.SkippedChecks++
		case StatusError:
			summary.ErrorChecks++
		}
	}
	for _, issue := range issues {
		if issue.Blocking() {
			summary.BlockingIssues++
		}
		if issue.Code != "" {
			if summary.ByCode == nil {
				summary.ByCode = map[string]int{}
			}
			summary.ByCode[string(issue.Code)]++
		}
		for _, rawNet := range issue.Nets {
			net := strings.TrimSpace(rawNet)
			if net != "" {
				if summary.ByNet == nil {
					summary.ByNet = map[string]int{}
				}
				summary.ByNet[net]++
			}
		}
	}
	return summary
}

func SortChecks(checks []Check) {
	slices.SortStableFunc(checks, func(a, b Check) int {
		aOrder, aKnown := checkPriority[a.Name]
		bOrder, bKnown := checkPriority[b.Name]
		switch {
		case aKnown && bKnown:
			return aOrder - bOrder
		case aKnown:
			return -1
		case bKnown:
			return 1
		default:
			return strings.Compare(a.Name, b.Name)
		}
	})
}

func IssuesFromError(err error, path string) []reports.Issue {
	if err == nil {
		return nil
	}
	var validationErrors kicadfiles.ValidationErrors
	if errors.As(err, &validationErrors) {
		issues := make([]reports.Issue, 0, len(validationErrors))
		for _, validationError := range validationErrors {
			issues = append(issues, issueFromValidationError(validationError, path))
		}
		return issues
	}
	return []reports.Issue{{
		Code:     codeForError(err),
		Severity: reports.SeverityError,
		Path:     filepath.ToSlash(path),
		Message:  err.Error(),
	}}
}

func issueFromValidationError(err kicadfiles.ValidationError, fallbackPath string) reports.Issue {
	pathParts := []string{}
	if err.File != "" {
		pathParts = append(pathParts, filepath.ToSlash(err.File))
	} else if fallbackPath != "" {
		pathParts = append(pathParts, filepath.ToSlash(fallbackPath))
	}
	if err.Section != "" {
		pathParts = append(pathParts, err.Section)
	}
	if err.Field != "" {
		pathParts = append(pathParts, err.Field)
	}
	return reports.Issue{
		Code:     codeForValidationError(err),
		Severity: reports.SeverityError,
		Path:     nonEmptyString(strings.Join(pathParts, "."), "board"),
		Message:  err.Message,
	}
}

func codeForValidationError(err kicadfiles.ValidationError) reports.Code {
	field := strings.ToLower(err.Field)
	section := strings.ToLower(err.Section)
	switch {
	case strings.Contains(field, "net_code") || strings.Contains(field, "net_name"):
		return reports.CodeInvalidNetAssignment
	case strings.Contains(section, "edge_cuts") || strings.Contains(field, "edge_cuts"):
		return reports.CodeMissingBoardOutline
	default:
		return reports.CodeValidationFailed
	}
}

func codeForError(err error) reports.Code {
	if err == nil {
		return reports.CodeUnknown
	}
	return reports.CodeValidationFailed
}

func statusForCheckIssues(issues []reports.Issue) Status {
	if len(issues) == 0 {
		return StatusPass
	}
	if hasBlockingIssue(issues) {
		return StatusFail
	}
	return StatusPass
}

func hasBlockingIssue(issues []reports.Issue) bool {
	for _, issue := range issues {
		if issue.Blocking() {
			return true
		}
	}
	return false
}

func (result Result) hasRequiredCheck() bool {
	for _, check := range result.Checks {
		if check.Required {
			return true
		}
	}
	return false
}

func nonEmptyString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
