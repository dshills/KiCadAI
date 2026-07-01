package evaluate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"kicadai/internal/inspect"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
	schematicfiles "kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/preservation"
	"kicadai/internal/reports"
	"kicadai/internal/schematicrules"
)

var explicitSinglePadNetPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^(?i:PORT|EXPORT)_[A-Z0-9_.+\-]+$`),
	regexp.MustCompile(`^(?i:IN|OUT|INPUT|OUTPUT)$`),
	regexp.MustCompile(`^(?i:GND|GNDA|GNDD|VCC|VDD|VSS|VEE)$`),
	regexp.MustCompile(`^[+-]?(?:[0-9]+(?:V[0-9]+|[._][0-9]+V?)|[0-9]+V)$`),
}

var explicitSinglePadNetPatternsMu sync.RWMutex

func SetExplicitSinglePadNetPatterns(patterns []string) error {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		expression, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("compile explicit single-pad net pattern %q: %w", pattern, err)
		}
		compiled = append(compiled, expression)
	}
	explicitSinglePadNetPatternsMu.Lock()
	defer explicitSinglePadNetPatternsMu.Unlock()
	explicitSinglePadNetPatterns = compiled
	return nil
}

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
	return ProjectContext(context.Background(), path)
}

func ProjectContext(ctx context.Context, path string) (Report, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	if strings.TrimSpace(path) == "" {
		return Report{}, fmt.Errorf("project path required")
	}
	summary, err := inspectProjectContext(ctx, path)
	if err != nil {
		return Report{}, err
	}
	report := newReport(summary.Root)
	report.InspectionSummaryPresent = true
	report.addPreservation(summary.Preservation)
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
		report.addCheck(externalKiCadCheck("erc_validation", summary.Schematic.Path))
	}
	if summary.PCB != nil {
		report.mergeChecks(checksForPCBSummary(*summary.PCB)...)
		report.addCheck(externalKiCadCheck("drc_validation", summary.PCB.Path))
	}
	report.finish()
	return report, nil
}

func inspectProjectContext(ctx context.Context, path string) (inspect.ProjectSummary, error) {
	type result struct {
		summary inspect.ProjectSummary
		err     error
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				done <- result{err: fmt.Errorf("inspect project panic: %v", recovered)}
			}
		}()
		summary, err := inspect.Project(path)
		done <- result{summary: summary, err: err}
	}()
	select {
	case <-ctx.Done():
		return inspect.ProjectSummary{}, ctx.Err()
	case result := <-done:
		return result.summary, result.err
	}
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
	report.addPreservation(summary.Preservation)
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
	report.addPreservation(summary.Preservation)
	report.mergeChecks(checksForPCBSummary(summary)...)
	report.finish()
	return report, nil
}

func preservationCheck(report *preservation.Report) CheckResult {
	if report == nil {
		return CheckResult{Name: "imported_preservation", Status: CheckSkipped, Required: true}
	}
	status := CheckPassed
	switch report.Status {
	case preservation.StatusBlocked:
		status = CheckBlocked
	case preservation.StatusWarning, preservation.StatusClean:
		status = CheckPassed
	default:
		issues := append([]reports.Issue(nil), report.Issues...)
		issues = append(issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityBlocked,
			Path:     "preservation.status",
			Message:  "unsupported preservation status " + string(report.Status),
		})
		return CheckResult{Name: "imported_preservation", Status: CheckBlocked, Required: true, Issues: issues}
	}
	issues := append([]reports.Issue(nil), report.Issues...)
	if report.Summary.PreservationOnly > 0 {
		issues = append(issues, reports.Issue{
			Code:       reports.CodePreservationConflict,
			Severity:   reports.SeverityWarning,
			Path:       "preservation",
			Message:    "project contains preservation-only KiCad content; evaluation is read-only and mutation must use preservation-aware transactions",
			Suggestion: "inspect the preservation report before planning edits",
		})
	}
	if report.HasBlockedOperation() {
		status = CheckBlocked
	}
	return CheckResult{Name: "imported_preservation", Status: status, Required: true, Issues: issues}
}

func checksForSchematicSummary(summary inspect.SchematicSummary) []CheckResult {
	issues := append([]reports.Issue{}, summary.Issues...)
	var electricalCheck *CheckResult
	file, err := schematicfiles.ReadFile(summary.Path)
	if err != nil {
		issues = append(issues, IssueFromError(err, summary.Path))
	} else {
		issues = append(issues, schematicSemanticIssues(file)...)
		check := schematicElectricalCheck(file)
		electricalCheck = &check
	}
	for _, unsupported := range summary.Unsupported {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeUnsupportedOperation,
			Severity: reports.SeverityWarning,
			Path:     "schematic." + unsupported.Kind,
			Message:  fmt.Sprintf("unsupported schematic node %q appears %d time(s)", unsupported.Kind, unsupported.Count),
		})
	}
	checks := []CheckResult{{Name: "schematic_validation", Status: statusForIssues(issues), Required: true, Issues: issues}}
	if electricalCheck != nil {
		checks = append(checks, *electricalCheck)
	}
	return checks
}

func schematicElectricalCheck(file schematicfiles.SchematicFile) CheckResult {
	report := schematicrules.Inspect(file, schematicrules.Options{Scope: schematicrules.ScopeImported, Acceptance: schematicrules.AcceptanceStructural})
	issues := schematicElectricalIssues(report)
	return CheckResult{Name: "schematic_electrical", Status: statusForIssues(issues), Required: true, Issues: issues}
}

func schematicElectricalIssues(report schematicrules.Report) []reports.Issue {
	issues := make([]reports.Issue, 0, len(report.Findings))
	for _, finding := range report.Findings {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   finding.Severity,
			Path:       finding.Path,
			Message:    string(finding.RuleID) + ": " + finding.Message,
			Refs:       schematicElectricalFindingRefs(finding),
			Nets:       schematicElectricalFindingNets(finding),
			Suggestion: finding.Repair,
		})
	}
	return issues
}

func schematicElectricalFindingRefs(finding schematicrules.Finding) []string {
	if strings.TrimSpace(finding.Reference) == "" {
		return nil
	}
	return []string{strings.TrimSpace(finding.Reference)}
}

func schematicElectricalFindingNets(finding schematicrules.Finding) []string {
	if strings.TrimSpace(finding.Net) == "" {
		return nil
	}
	return []string{strings.TrimSpace(finding.Net)}
}

func checksForPCBSummary(summary inspect.PCBSummary) []CheckResult {
	issues := append([]reports.Issue{}, summary.Issues...)
	for index := range issues {
		if issues[index].Code == reports.CodeMissingBoardOutline {
			issues[index].Severity = reports.SeverityError
		}
	}
	for _, unsupported := range summary.Unsupported {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeUnsupportedOperation,
			Severity: reports.SeverityWarning,
			Path:     "pcb." + unsupported.Kind,
			Message:  fmt.Sprintf("unsupported PCB node %q appears %d time(s)", unsupported.Kind, unsupported.Count),
		})
	}
	board, err := pcbfiles.ReadFile(summary.Path)
	if err != nil {
		issues = append(issues, IssueFromError(err, summary.Path))
	} else {
		issues = append(issues, pcbSemanticIssues(board)...)
	}
	return []CheckResult{{Name: "pcb_validation", Status: statusForIssues(issues), Required: true, Issues: issues}}
}

func externalKiCadCheck(name string, path string) CheckResult {
	return CheckResult{
		Name:     name,
		Status:   CheckSkipped,
		Required: false,
		Issues: []reports.Issue{{
			Code:       reports.CodeSkippedExternalTool,
			Severity:   reports.SeverityInfo,
			Path:       filepath.ToSlash(path),
			Message:    name + " is available through the `check` command and is not run by default during structural evaluation",
			Suggestion: fmt.Sprintf("run `kicadai --json check project %q` for KiCad ERC/DRC evidence", filepath.ToSlash(filepath.Dir(path))),
		}},
	}
}

func schematicSemanticIssues(file schematicfiles.SchematicFile) []reports.Issue {
	seen := map[string]struct{}{}
	var issues []reports.Issue
	for _, symbol := range file.Symbols {
		ref := strings.TrimSpace(symbol.Reference)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeDuplicateReference,
				Severity: reports.SeverityError,
				Path:     "schematic.symbols." + ref,
				Message:  "duplicate schematic reference " + ref,
				Refs:     []string{ref},
			})
		}
		seen[ref] = struct{}{}
	}
	return issues
}

func pcbSemanticIssues(board pcbfiles.PCBFile) []reports.Issue {
	connectedNets := map[string]struct{}{}
	for _, track := range board.Tracks {
		if netName := strings.TrimSpace(track.NetName); netName != "" {
			connectedNets[netName] = struct{}{}
		}
	}
	for _, arc := range board.TrackArcs {
		if netName := strings.TrimSpace(arc.NetName); netName != "" {
			connectedNets[netName] = struct{}{}
		}
	}
	for _, via := range board.Vias {
		if netName := strings.TrimSpace(via.NetName); netName != "" {
			connectedNets[netName] = struct{}{}
		}
	}
	for _, zone := range board.Zones {
		if netName := strings.TrimSpace(zone.NetName); netName != "" {
			connectedNets[netName] = struct{}{}
		}
	}
	padCountByNet := map[string]int{}
	for _, footprint := range board.Footprints {
		for _, pad := range footprint.Pads {
			netName := strings.TrimSpace(pad.NetName)
			if netName != "" {
				padCountByNet[netName]++
			}
		}
	}
	var issues []reports.Issue
	for _, footprint := range board.Footprints {
		for _, pad := range footprint.Pads {
			netName := strings.TrimSpace(pad.NetName)
			if netName == "" {
				continue
			}
			if padCountByNet[netName] < 2 && isExplicitSinglePadNet(netName) {
				continue
			}
			if _, ok := connectedNets[netName]; !ok {
				ref := footprint.Reference
				if ref == "" {
					ref = footprint.LibraryID
				}
				issues = append(issues, reports.Issue{
					Code:     reports.CodeDisconnectedPad,
					Severity: reports.SeverityError,
					Path:     "pcb.footprints." + ref + ".pads." + pad.Name,
					Message:  "pad is assigned to net " + netName + " but no parsed track, arc, or zone uses that net",
					Refs:     []string{ref},
					Nets:     []string{netName},
				})
			}
		}
	}
	return issues
}

func isExplicitSinglePadNet(netName string) bool {
	normalized := strings.TrimSpace(netName)
	if normalized == "" {
		return false
	}
	explicitSinglePadNetPatternsMu.RLock()
	defer explicitSinglePadNetPatternsMu.RUnlock()
	for _, pattern := range explicitSinglePadNetPatterns {
		if pattern.MatchString(normalized) {
			return true
		}
	}
	return false
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

func (report *Report) addPreservation(preservationReport *preservation.Report) {
	if preservationReport == nil {
		return
	}
	report.Preservation = preservationReport
	report.addCheck(preservationCheck(preservationReport))
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
