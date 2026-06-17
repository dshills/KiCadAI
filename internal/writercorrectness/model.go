package writercorrectness

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

type CheckStatus string

const (
	CheckPass    CheckStatus = "pass"
	CheckFail    CheckStatus = "fail"
	CheckWarning CheckStatus = "warning"
	CheckSkipped CheckStatus = "skipped"
)

const (
	CheckProjectStructure      = "project_structure"
	CheckSchematicParse        = "schematic_parse"
	CheckSchematicConnectivity = "schematic_connectivity"
	CheckSchematicPCBTransfer  = "schematic_pcb_transfer"
	CheckPCBParse              = "pcb_parse"
	CheckPCBNetTable           = "pcb_net_table"
	CheckFootprintPadNets      = "footprint_pad_nets"
	CheckCopperNetReferences   = "copper_net_references"
	CheckGeneratedConnectivity = "generated_connectivity"
	CheckZoneNetReferences     = "zone_net_references"
	CheckKiCadRoundTrip        = "kicad_round_trip"
)

var checkPriority = map[string]int{
	CheckProjectStructure:      0,
	CheckSchematicParse:        1,
	CheckSchematicConnectivity: 2,
	CheckSchematicPCBTransfer:  3,
	CheckPCBParse:              4,
	CheckPCBNetTable:           5,
	CheckFootprintPadNets:      6,
	CheckCopperNetReferences:   7,
	CheckGeneratedConnectivity: 8,
	CheckZoneNetReferences:     9,
	CheckKiCadRoundTrip:        10,
}

type Options struct {
	RequireKiCadRoundTrip bool
	KiCadCLI              string
	KeepArtifacts         bool
	ArtifactDir           string
	StrictDiffs           bool
	AllowUnrouted         bool
	GeometryToleranceIU   int64
}

type Target struct {
	Input          string   `json:"input"`
	ProjectDir     string   `json:"project_dir,omitempty"`
	ProjectPath    string   `json:"project_path,omitempty"`
	SchematicPath  string   `json:"schematic_path,omitempty"`
	PCBPath        string   `json:"pcb_path,omitempty"`
	SchematicFiles []string `json:"schematic_files,omitempty"`
}

type Result struct {
	OK             bool               `json:"ok"`
	Target         Target             `json:"target"`
	Checks         []CheckResult      `json:"checks"`
	Issues         []reports.Issue    `json:"issues"`
	Artifacts      []reports.Artifact `json:"artifacts"`
	OverallSummary Summary            `json:"overall_summary"`
}

type CheckResult struct {
	Name      string             `json:"name"`
	Status    CheckStatus        `json:"status"`
	Required  bool               `json:"required"`
	Issues    []reports.Issue    `json:"issues,omitempty"`
	Artifacts []reports.Artifact `json:"artifacts,omitempty"`
	Summary   string             `json:"summary,omitempty"`
}

type Summary struct {
	CheckCount    int            `json:"check_count"`
	PassCount     int            `json:"pass_count"`
	FailCount     int            `json:"fail_count"`
	WarningCount  int            `json:"warning_count"`
	SkippedCount  int            `json:"skipped_count"`
	IssueCount    int            `json:"issue_count"`
	BlockingCount int            `json:"blocking_count"`
	ByCode        map[string]int `json:"by_code,omitempty"`
	ByCheck       map[string]int `json:"by_check,omitempty"`
	ByNet         map[string]int `json:"by_net,omitempty"`
}

func NewResult(input string) Result {
	return Result{
		OK: true,
		Target: Target{
			Input: slashPath(input),
		},
		Checks:    []CheckResult{},
		Issues:    []reports.Issue{},
		Artifacts: []reports.Artifact{},
	}
}

func (result *Result) AddCheck(check CheckResult) {
	normalizeCheck(&check)
	result.Checks = append(result.Checks, check)
}

func (result *Result) AddIssue(checkName string, issue reports.Issue) {
	result.AddCheck(CheckResult{
		Name:     checkName,
		Required: true,
		Issues:   []reports.Issue{issue},
	})
}

func (result *Result) Finish() {
	SortChecks(result.Checks)
	result.Issues = []reports.Issue{}
	result.Artifacts = []reports.Artifact{}
	for _, check := range result.Checks {
		result.Issues = append(result.Issues, check.Issues...)
		result.Artifacts = append(result.Artifacts, check.Artifacts...)
	}
	SortIssues(result.Issues)
	SortArtifacts(result.Artifacts)
	result.OverallSummary = Summarize(result.Checks, result.Issues)
	result.OK = !reports.HasBlockingIssue(result.Issues) && result.OverallSummary.FailCount == 0
}

func (result Result) ReportResult(command string) reports.Result {
	return reports.ResultWithIssues(command, result, result.Issues, result.Artifacts)
}

func NewIssue(code reports.Code, severity reports.Severity, path string, message string) reports.Issue {
	return reports.Issue{
		Code:     code,
		Severity: severity,
		Path:     slashPath(path),
		Message:  message,
	}
}

func BlockingIssue(code reports.Code, path string, message string) reports.Issue {
	return NewIssue(code, reports.SeverityBlocked, path, message)
}

func WarningIssue(code reports.Code, path string, message string) reports.Issue {
	return NewIssue(code, reports.SeverityWarning, path, message)
}

func StatusForIssues(issues []reports.Issue) CheckStatus {
	if len(issues) == 0 {
		return CheckPass
	}
	if reports.HasBlockingIssue(issues) {
		return CheckFail
	}
	return CheckWarning
}

func Summarize(checks []CheckResult, issues []reports.Issue) Summary {
	summary := Summary{
		CheckCount: len(checks),
		IssueCount: len(issues),
	}
	for _, check := range checks {
		switch check.Status {
		case CheckPass:
			summary.PassCount++
		case CheckFail:
			summary.FailCount++
		case CheckWarning:
			summary.WarningCount++
		case CheckSkipped:
			summary.SkippedCount++
		}
		if check.Name != "" {
			if summary.ByCheck == nil {
				summary.ByCheck = map[string]int{}
			}
			summary.ByCheck[check.Name]++
		}
	}
	for _, issue := range issues {
		if issue.Blocking() {
			summary.BlockingCount++
		}
		if issue.Code != "" {
			if summary.ByCode == nil {
				summary.ByCode = map[string]int{}
			}
			summary.ByCode[string(issue.Code)]++
		}
		for _, rawNet := range issue.Nets {
			net := strings.TrimSpace(rawNet)
			if net == "" {
				continue
			}
			if summary.ByNet == nil {
				summary.ByNet = map[string]int{}
			}
			summary.ByNet[net]++
		}
	}
	return summary
}

func SortChecks(checks []CheckResult) {
	for i := range checks {
		normalizeCheck(&checks[i])
		SortIssues(checks[i].Issues)
		SortArtifacts(checks[i].Artifacts)
	}
	slices.SortStableFunc(checks, func(a, b CheckResult) int {
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

func SortIssues(issues []reports.Issue) {
	slices.SortStableFunc(issues, func(a, b reports.Issue) int {
		return cmp.Or(
			strings.Compare(a.Path, b.Path),
			strings.Compare(string(a.Code), string(b.Code)),
			strings.Compare(string(a.Severity), string(b.Severity)),
			strings.Compare(a.Message, b.Message),
		)
	})
}

func SortArtifacts(artifacts []reports.Artifact) {
	slices.SortStableFunc(artifacts, func(a, b reports.Artifact) int {
		return cmp.Or(
			strings.Compare(string(a.Kind), string(b.Kind)),
			strings.Compare(a.Path, b.Path),
			strings.Compare(a.Description, b.Description),
		)
	})
}

func normalizeCheck(check *CheckResult) {
	if check.Issues == nil {
		check.Issues = []reports.Issue{}
	}
	if check.Artifacts == nil {
		check.Artifacts = []reports.Artifact{}
	}
	if check.Status != CheckSkipped {
		check.Status = StatusForIssues(check.Issues)
	}
	for i := range check.Issues {
		check.Issues[i].Path = slashPath(check.Issues[i].Path)
	}
	for i := range check.Artifacts {
		check.Artifacts[i].Path = slashPath(check.Artifacts[i].Path)
	}
}

func slashPath(path string) string {
	return strings.ReplaceAll(filepath.ToSlash(path), `\`, "/")
}
