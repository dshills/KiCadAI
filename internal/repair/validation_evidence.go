package repair

import (
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/reports"
)

type PostValidationOptions struct {
	WriterCorrectness       bool   `json:"writer_correctness"`
	BoardValidation         bool   `json:"board_validation"`
	KiCadERC                bool   `json:"kicad_erc"`
	KiCadDRC                bool   `json:"kicad_drc"`
	RoundTrip               bool   `json:"round_trip"`
	RequireKiCadERC         bool   `json:"require_kicad_erc"`
	RequireKiCadDRC         bool   `json:"require_kicad_drc"`
	RequireRoundTrip        bool   `json:"require_round_trip"`
	StrictZones             bool   `json:"strict_zones"`
	StrictUnrouted          bool   `json:"strict_unrouted"`
	AllowMissingKiCadChecks bool   `json:"allow_missing_kicad_checks"`
	KeepArtifacts           bool   `json:"keep_artifacts"`
	ArtifactDir             string `json:"artifact_dir,omitempty"`
	KiCadCLI                string `json:"kicad_cli,omitempty"`
}

type ValidationSummary struct {
	AdapterCount  int            `json:"adapter_count"`
	SkippedCount  int            `json:"skipped_count"`
	IssueCount    int            `json:"issue_count"`
	BlockingCount int            `json:"blocking_count"`
	ErrorCount    int            `json:"error_count"`
	WarningCount  int            `json:"warning_count"`
	InfoCount     int            `json:"info_count"`
	ArtifactCount int            `json:"artifact_count"`
	ByCode        map[string]int `json:"by_code,omitempty"`
	ByAdapter     map[string]int `json:"by_adapter,omitempty"`
}

type ValidationDelta struct {
	Before   ValidationSummary `json:"before"`
	After    ValidationSummary `json:"after"`
	Cleared  []reports.Issue   `json:"cleared,omitempty"`
	Repeated []reports.Issue   `json:"repeated,omitempty"`
	New      []reports.Issue   `json:"new,omitempty"`
	Worsened bool              `json:"worsened,omitempty"`
}

type FindingSource string

const (
	FindingSourceTransaction FindingSource = "transaction"
	FindingSourceWriter      FindingSource = "writer_correctness"
	FindingSourceBoard       FindingSource = "board_validation"
	FindingSourceKiCadERC    FindingSource = "kicad_erc"
	FindingSourceKiCadDRC    FindingSource = "kicad_drc"
	FindingSourceRoundTrip   FindingSource = "round_trip"
	FindingSourceRepair      FindingSource = "repair"
	FindingSourceWorkflow    FindingSource = "workflow"
)

type FindingCategory string

const (
	FindingCategoryUnknown          FindingCategory = "unknown"
	FindingCategoryParse            FindingCategory = "parse"
	FindingCategoryProjectStructure FindingCategory = "project_structure"
	FindingCategorySchematicERC     FindingCategory = "schematic_erc"
	FindingCategoryBoardDRC         FindingCategory = "board_drc"
	FindingCategoryConnectivity     FindingCategory = "connectivity"
	FindingCategoryPadNet           FindingCategory = "pad_net"
	FindingCategoryRoute            FindingCategory = "route"
	FindingCategoryZone             FindingCategory = "zone"
	FindingCategoryOutline          FindingCategory = "outline"
	FindingCategoryRoundTrip        FindingCategory = "round_trip"
	FindingCategoryExternalTool     FindingCategory = "external_tool"
	FindingCategoryPreservation     FindingCategory = "preservation"
	FindingCategoryUnsupported      FindingCategory = "unsupported"
)

type Repairability string

const (
	RepairabilityRepairable          Repairability = "repairable"
	RepairabilityUnsupported         Repairability = "unsupported"
	RepairabilityExternalToolBlocked Repairability = "external_tool_blocked"
	RepairabilityPreservationBlocked Repairability = "preservation_blocked"
	RepairabilityInformational       Repairability = "informational"
)

type FindingSubject struct {
	Ref      string `json:"ref,omitempty"`
	Net      string `json:"net,omitempty"`
	Layer    string `json:"layer,omitempty"`
	Pad      string `json:"pad,omitempty"`
	File     string `json:"file,omitempty"`
	Rule     string `json:"rule,omitempty"`
	Location string `json:"location,omitempty"`
}

type NormalizedFinding struct {
	Key           string           `json:"key"`
	Source        FindingSource    `json:"source"`
	Adapter       string           `json:"adapter,omitempty"`
	Category      FindingCategory  `json:"category"`
	Repairability Repairability    `json:"repairability"`
	Code          reports.Code     `json:"code"`
	Severity      reports.Severity `json:"severity"`
	Path          string           `json:"path,omitempty"`
	Message       string           `json:"message"`
	Subject       FindingSubject   `json:"subject,omitempty"`
	OperationID   string           `json:"operation_id,omitempty"`
	EvidencePath  string           `json:"evidence_path,omitempty"`
	RawCode       string           `json:"raw_code,omitempty"`
}

type NormalizeFindingOptions struct {
	Source        FindingSource
	Adapter       string
	Category      FindingCategory
	Repairability Repairability
	EvidencePath  string
	RawCode       string
	Subject       FindingSubject
}

func SummarizePostValidation(validations []PostApplyValidation) ValidationSummary {
	summary := ValidationSummary{AdapterCount: len(validations)}
	uniqueIssues := map[string]reports.Issue{}
	for _, validation := range validations {
		if validation.Skipped {
			summary.SkippedCount++
		}
		if len(validation.Issues) > 0 {
			if summary.ByAdapter == nil {
				summary.ByAdapter = map[string]int{}
			}
			summary.ByAdapter[validation.Name] += len(validation.Issues)
		}
		summary.ArtifactCount += len(validation.Artifacts)
		for _, issue := range validation.Issues {
			key := StableIssueKey(issue)
			if _, ok := uniqueIssues[key]; !ok {
				uniqueIssues[key] = issue
			}
		}
	}
	addIssueSummary(&summary, issuesFromMap(uniqueIssues, false))
	return summary
}

func SummarizeIssues(issues []reports.Issue) ValidationSummary {
	summary := ValidationSummary{}
	addIssueSummary(&summary, issues)
	return summary
}

func CompareValidationIssues(before []reports.Issue, after []reports.Issue) ValidationDelta {
	beforeByKey := issueMap(before)
	afterByKey := issueMap(after)
	delta := ValidationDelta{
		Before: SummarizeIssues(issuesFromMap(beforeByKey, false)),
		After:  SummarizeIssues(issuesFromMap(afterByKey, false)),
	}
	for key, issue := range beforeByKey {
		if _, ok := afterByKey[key]; ok {
			delta.Repeated = append(delta.Repeated, issue)
			continue
		}
		delta.Cleared = append(delta.Cleared, issue)
	}
	for key, issue := range afterByKey {
		if _, ok := beforeByKey[key]; !ok {
			delta.New = append(delta.New, issue)
		}
	}
	sortIssuesForEvidence(delta.Cleared)
	sortIssuesForEvidence(delta.Repeated)
	sortIssuesForEvidence(delta.New)
	delta.Worsened = delta.After.BlockingCount > delta.Before.BlockingCount || len(blockingIssues(delta.New)) > 0
	return delta
}

func NormalizeIssue(issue reports.Issue, opts NormalizeFindingOptions) NormalizedFinding {
	normalizedPath := slashPathForEvidence(issue.Path)
	finding := NormalizedFinding{
		Source:        opts.Source,
		Adapter:       strings.TrimSpace(opts.Adapter),
		Category:      opts.Category,
		Repairability: opts.Repairability,
		Code:          issue.Code,
		Severity:      issue.Severity,
		Path:          normalizedPath,
		Message:       strings.TrimSpace(issue.Message),
		Subject:       opts.Subject,
		OperationID:   strings.TrimSpace(issue.OperationID),
		EvidencePath:  slashPathForEvidence(opts.EvidencePath),
		RawCode:       strings.TrimSpace(opts.RawCode),
	}
	if finding.Source == "" {
		finding.Source = FindingSourceRepair
	}
	if finding.Category == "" {
		finding.Category = defaultFindingCategory(issue)
	}
	if finding.Repairability == "" {
		finding.Repairability = defaultRepairability(issue, finding.Category)
	}
	if finding.Subject.Ref == "" && len(issue.Refs) == 1 {
		finding.Subject.Ref = strings.TrimSpace(issue.Refs[0])
	}
	if finding.Subject.Net == "" && len(issue.Nets) == 1 {
		finding.Subject.Net = strings.TrimSpace(issue.Nets[0])
	}
	if finding.Subject.File == "" {
		finding.Subject.File = fileSubjectFromPath(normalizedPath)
	}
	finding.Subject = normalizeFindingSubject(finding.Subject)
	finding.Key = NormalizedFindingKey(finding)
	return finding
}

func NormalizeIssues(issues []reports.Issue, opts NormalizeFindingOptions) []NormalizedFinding {
	findings := make([]NormalizedFinding, 0, len(issues))
	for _, issue := range issues {
		findings = append(findings, NormalizeIssue(issue, opts))
	}
	SortNormalizedFindings(findings)
	return findings
}

func NormalizedFindingKey(finding NormalizedFinding) string {
	var builder strings.Builder
	writeKeyPart(&builder, string(finding.Source))
	writeKeyPart(&builder, string(finding.Category))
	if finding.RawCode != "" {
		writeKeyPart(&builder, finding.RawCode)
	} else {
		writeKeyPart(&builder, string(finding.Code))
	}
	writeKeyPart(&builder, string(finding.Severity))
	writeKeyPart(&builder, finding.Path)
	writeKeyPart(&builder, strings.TrimSpace(finding.OperationID))
	writeKeyPart(&builder, finding.EvidencePath)
	writeKeyPart(&builder, finding.Subject.Ref)
	writeKeyPart(&builder, finding.Subject.Net)
	writeKeyPart(&builder, finding.Subject.Layer)
	writeKeyPart(&builder, finding.Subject.Pad)
	writeKeyPart(&builder, finding.Subject.File)
	writeKeyPart(&builder, finding.Subject.Rule)
	writeKeyPart(&builder, finding.Subject.Location)
	writeKeyPart(&builder, strings.TrimSpace(finding.Message))
	return builder.String()
}

func SortNormalizedFindings(findings []NormalizedFinding) {
	if len(findings) < 2 {
		return
	}
	for index := range findings {
		if findings[index].Key == "" {
			findings[index].Key = NormalizedFindingKey(findings[index])
		}
	}
	sort.SliceStable(findings, func(i, j int) bool {
		return findings[i].Key < findings[j].Key
	})
}

func StableIssueKey(issue reports.Issue) string {
	var builder strings.Builder
	writeKeyPart(&builder, string(issue.Code))
	writeKeyPart(&builder, string(issue.Severity))
	writeKeyPart(&builder, slashPathForEvidence(issue.Path))
	writeKeyPart(&builder, strings.TrimSpace(issue.Message))
	writeKeyPart(&builder, strings.TrimSpace(issue.OperationID))
	writeKeyStringList(&builder, issue.UUIDs)
	writeKeyStringList(&builder, issue.Refs)
	writeKeyStringList(&builder, issue.Nets)
	return builder.String()
}

func writeKeyStringList(builder *strings.Builder, values []string) {
	values = sortedStrings(values)
	writeKeyPart(builder, strconv.Itoa(len(values)))
	for _, value := range values {
		writeKeyPart(builder, value)
	}
}

func writeKeyPart(builder *strings.Builder, part string) {
	builder.WriteString(strconv.Itoa(len(part)))
	builder.WriteByte(':')
	builder.WriteString(part)
	builder.WriteByte(0)
}

func addIssueSummary(summary *ValidationSummary, issues []reports.Issue) {
	for _, issue := range issues {
		summary.IssueCount++
		if summary.ByCode == nil {
			summary.ByCode = map[string]int{}
		}
		summary.ByCode[string(issue.Code)]++
		if issue.Blocking() {
			summary.BlockingCount++
		}
		switch issue.Severity {
		case reports.SeverityError, reports.SeverityBlocked:
			summary.ErrorCount++
		case reports.SeverityWarning:
			summary.WarningCount++
		case reports.SeverityInfo:
			summary.InfoCount++
		}
	}
}

func issueMap(issues []reports.Issue) map[string]reports.Issue {
	mapped := make(map[string]reports.Issue, len(issues))
	for _, issue := range issues {
		key := StableIssueKey(issue)
		if _, ok := mapped[key]; !ok {
			mapped[key] = issue
		}
	}
	return mapped
}

func issuesFromMap(mapped map[string]reports.Issue, sorted bool) []reports.Issue {
	issues := make([]reports.Issue, 0, len(mapped))
	for _, issue := range mapped {
		issues = append(issues, issue)
	}
	if sorted {
		sortIssuesForEvidence(issues)
	}
	return issues
}

func sortIssuesForEvidence(issues []reports.Issue) {
	if len(issues) < 2 {
		return
	}
	type keyedIssue struct {
		key   string
		issue reports.Issue
	}
	keyed := make([]keyedIssue, 0, len(issues))
	for _, issue := range issues {
		keyed = append(keyed, keyedIssue{key: StableIssueKey(issue), issue: issue})
	}
	sort.SliceStable(keyed, func(i, j int) bool {
		return keyed[i].key < keyed[j].key
	})
	for index, entry := range keyed {
		issues[index] = entry.issue
	}
}

func blockingIssues(issues []reports.Issue) []reports.Issue {
	blocking := make([]reports.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Blocking() {
			blocking = append(blocking, issue)
		}
	}
	return blocking
}

func sortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	return copied
}

func slashPathForEvidence(path string) string {
	return strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
}

func defaultFindingCategory(issue reports.Issue) FindingCategory {
	switch issue.Code {
	case reports.CodeMissingBoardOutline:
		return FindingCategoryOutline
	case reports.CodeDisconnectedPad:
		return FindingCategoryConnectivity
	case reports.CodeInvalidNetAssignment:
		if containsEvidenceText(issue.Path, "zone") || containsEvidenceText(issue.Message, "zone") {
			return FindingCategoryZone
		}
		return FindingCategoryPadNet
	case reports.CodeKiCadCLIFailed, reports.CodeSkippedExternalTool:
		return FindingCategoryExternalTool
	case reports.CodeRoundTripDiff:
		return FindingCategoryRoundTrip
	case reports.CodePreservationConflict:
		return FindingCategoryPreservation
	case reports.CodeUnsupportedImportedObject, reports.CodeUnsupportedOperation:
		return FindingCategoryUnsupported
	case reports.CodeMissingFile:
		return FindingCategoryProjectStructure
	default:
		path := strings.ToLower(issue.Path)
		message := strings.ToLower(issue.Message)
		switch {
		case strings.Contains(path, "parse") || strings.Contains(message, "parse"):
			return FindingCategoryParse
		case strings.Contains(path, "route") || strings.Contains(message, "route"):
			return FindingCategoryRoute
		case strings.Contains(path, "zone") || strings.Contains(message, "zone"):
			return FindingCategoryZone
		case strings.Contains(path, "outline") || strings.Contains(message, "outline") || strings.Contains(path, "edge.cuts") || strings.Contains(message, "edge.cuts"):
			return FindingCategoryOutline
		default:
			return FindingCategoryUnknown
		}
	}
}

func defaultRepairability(issue reports.Issue, category FindingCategory) Repairability {
	if issue.Severity == reports.SeverityInfo || issue.Severity == reports.SeverityWarning {
		return RepairabilityInformational
	}
	switch category {
	case FindingCategoryExternalTool:
		return RepairabilityExternalToolBlocked
	case FindingCategoryPreservation:
		return RepairabilityPreservationBlocked
	case FindingCategoryUnsupported:
		return RepairabilityUnsupported
	default:
		return RepairabilityRepairable
	}
}

func normalizeFindingSubject(subject FindingSubject) FindingSubject {
	return FindingSubject{
		Ref:      strings.TrimSpace(subject.Ref),
		Net:      strings.TrimSpace(subject.Net),
		Layer:    strings.TrimSpace(subject.Layer),
		Pad:      strings.TrimSpace(subject.Pad),
		File:     slashPathForEvidence(subject.File),
		Rule:     strings.TrimSpace(subject.Rule),
		Location: strings.TrimSpace(subject.Location),
	}
}

func fileSubjectFromPath(path string) string {
	if path == "" {
		return ""
	}
	for _, marker := range []string{".kicad_sch", ".kicad_pcb", ".kicad_pro", ".kicad_sym"} {
		index := strings.Index(path, marker)
		if index >= 0 {
			return path[:index+len(marker)]
		}
	}
	return ""
}

func containsEvidenceText(value string, needle string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(needle))
}
