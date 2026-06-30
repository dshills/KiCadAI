package designworkflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"kicadai/internal/reports"
)

// PromotionReadiness describes a fixture's declared or achieved maturity.
type PromotionReadiness string

const (
	// PromotionReadinessExpectedFail means the fixture is expected to produce known blockers.
	PromotionReadinessExpectedFail PromotionReadiness = "expected_fail"
	// PromotionReadinessCandidate means the fixture has provisional promotion evidence.
	PromotionReadinessCandidate PromotionReadiness = "candidate"
	// PromotionReadinessPass means the fixture is stable enough for required evidence.
	PromotionReadinessPass PromotionReadiness = "pass"
	// PromotionReadinessBlocked means the fixture is documented but not runnable.
	PromotionReadinessBlocked PromotionReadiness = "blocked"
)

// PromotionStatus summarizes a promotion report run.
type PromotionStatus string

const (
	PromotionStatusPass           PromotionStatus = "pass"
	PromotionStatusWarn           PromotionStatus = "warn"
	PromotionStatusFailed         PromotionStatus = "failed"
	PromotionStatusExpectedFail   PromotionStatus = "expected_fail"
	PromotionStatusUnexpectedPass PromotionStatus = "unexpected_pass"
	PromotionStatusBlocked        PromotionStatus = "blocked"
	PromotionStatusSkipped        PromotionStatus = "skipped"
	PromotionStatusError          PromotionStatus = "error"
	PromotionStatusNotRun         PromotionStatus = "not_run"
)

// PromotionGateStatus summarizes one promotion gate.
type PromotionGateStatus string

const (
	PromotionGateStatusPass    PromotionGateStatus = "pass"
	PromotionGateStatusWarn    PromotionGateStatus = "warn"
	PromotionGateStatusFailed  PromotionGateStatus = "failed"
	PromotionGateStatusSkipped PromotionGateStatus = "skipped"
	PromotionGateStatusNotRun  PromotionGateStatus = "not_run"
)

// PromotionReport is the normalized report emitted for a KiCad-backed design fixture.
type PromotionReport struct {
	ID                 string                `json:"id"`
	Request            string                `json:"request,omitempty"`
	Tier               string                `json:"tier,omitempty"`
	DeclaredReadiness  PromotionReadiness    `json:"declared_readiness"`
	AchievedReadiness  PromotionReadiness    `json:"achieved_readiness"`
	Acceptance         AcceptanceLevel       `json:"acceptance,omitempty"`
	Status             PromotionStatus       `json:"status"`
	MatchesExpectation bool                  `json:"matches_expectation"`
	Summary            string                `json:"summary,omitempty"`
	Gates              []PromotionGate       `json:"gates"`
	Stages             PromotionStageReport  `json:"stages,omitempty"`
	Issues             []PromotionIssue      `json:"issues,omitempty"`
	NextActions        []PromotionNextAction `json:"next_actions,omitempty"`
	Artifacts          []PromotionArtifact   `json:"artifacts,omitempty"`
	KiCadVersion       string                `json:"kicad_version,omitempty"`
	ExternalEvidence   string                `json:"external_evidence,omitempty"`
	GeneratedAt        string                `json:"generated_at,omitempty"`
}

// PromotionGate records one promotion check and the readiness levels it affects.
type PromotionGate struct {
	ID          string               `json:"id"`
	Status      PromotionGateStatus  `json:"status"`
	RequiredFor []PromotionReadiness `json:"required_for,omitempty"`
	IssueCodes  []string             `json:"issue_codes,omitempty"`
	Artifacts   []string             `json:"artifacts,omitempty"`
	Summary     string               `json:"summary,omitempty"`
}

// PromotionStageReport records expected and reached workflow stages.
type PromotionStageReport struct {
	Expected  []StageName `json:"expected,omitempty"`
	Reached   []StageName `json:"reached,omitempty"`
	StoppedAt StageName   `json:"stopped_at,omitempty"`
}

// PromotionIssue records a machine-readable promotion blocker or warning.
type PromotionIssue struct {
	Code     string           `json:"code"`
	Severity reports.Severity `json:"severity"`
	Stage    StageName        `json:"stage,omitempty"`
	Message  string           `json:"message"`
	Path     string           `json:"path,omitempty"`
	Repair   string           `json:"repair,omitempty"`
	Refs     []string         `json:"refs,omitempty"`
	Nets     []string         `json:"nets,omitempty"`
	Artifact string           `json:"artifact,omitempty"`
}

// PromotionNextAction records the next contributor or AI repair action for a non-passing gate.
type PromotionNextAction struct {
	Gate       string           `json:"gate"`
	Severity   reports.Severity `json:"severity"`
	Summary    string           `json:"summary"`
	Action     string           `json:"action"`
	IssueCodes []string         `json:"issue_codes,omitempty"`
	Artifacts  []string         `json:"artifacts,omitempty"`
}

// PromotionArtifact records an artifact referenced by promotion evidence.
type PromotionArtifact struct {
	Path        string               `json:"path"`
	Kind        reports.ArtifactKind `json:"kind,omitempty"`
	Description string               `json:"description,omitempty"`
	Required    bool                 `json:"required,omitempty"`
}

const PromotionReportArtifactPath = ".kicadai/design-promotion.json"

// PromotionSummary is the compact workflow/CLI view of a full promotion report.
type PromotionSummary struct {
	Status             PromotionStatus    `json:"status"`
	DeclaredReadiness  PromotionReadiness `json:"declared_readiness"`
	AchievedReadiness  PromotionReadiness `json:"achieved_readiness"`
	MatchesExpectation bool               `json:"matches_expectation"`
	ReportPath         string             `json:"report_path,omitempty"`
}

// Validate checks the promotion report schema before it is serialized.
func (report PromotionReport) Validate() error {
	if strings.TrimSpace(report.ID) == "" {
		return fmt.Errorf("promotion report id is required")
	}
	if !validPromotionReadiness(report.DeclaredReadiness) {
		return fmt.Errorf("unsupported declared readiness %q", report.DeclaredReadiness)
	}
	if !validPromotionReadiness(report.AchievedReadiness) {
		return fmt.Errorf("unsupported achieved readiness %q", report.AchievedReadiness)
	}
	if !validPromotionStatus(report.Status) {
		return fmt.Errorf("unsupported promotion status %q", report.Status)
	}
	if report.Acceptance != "" && !validPromotionAcceptance(report.Acceptance) {
		return fmt.Errorf("unsupported promotion acceptance %q", report.Acceptance)
	}
	if report.GeneratedAt != "" {
		if _, err := time.Parse(time.RFC3339, report.GeneratedAt); err != nil {
			return fmt.Errorf("promotion generated_at must be RFC3339: %w", err)
		}
	}
	if !promotionStatusBypassesReadiness(report.Status) {
		switch report.AchievedReadiness {
		case PromotionReadinessPass:
			if report.DeclaredReadiness == PromotionReadinessExpectedFail {
				if report.Status != PromotionStatusUnexpectedPass {
					return fmt.Errorf("unexpected pass requires unexpected_pass status")
				}
			} else if report.Status != PromotionStatusPass {
				return fmt.Errorf("pass achieved readiness requires pass status")
			}
		case PromotionReadinessCandidate:
			if report.DeclaredReadiness == PromotionReadinessExpectedFail {
				if report.Status != PromotionStatusUnexpectedPass {
					return fmt.Errorf("unexpected pass requires unexpected_pass status")
				}
			} else if report.Status != PromotionStatusPass && report.Status != PromotionStatusWarn {
				return fmt.Errorf("candidate achieved readiness requires pass or warn status")
			}
		case PromotionReadinessExpectedFail:
			if report.Status != PromotionStatusExpectedFail {
				return fmt.Errorf("expected_fail achieved readiness requires expected_fail status")
			}
		case PromotionReadinessBlocked:
			if report.Status != PromotionStatusBlocked && report.Status != PromotionStatusFailed {
				return fmt.Errorf("blocked achieved readiness requires blocked or failed status")
			}
		}
	}
	if len(report.Gates) == 0 {
		return fmt.Errorf("promotion report gates must not be empty")
	}
	if report.Stages.StoppedAt != "" {
		if !validPromotionStageName(report.Stages.StoppedAt) {
			return fmt.Errorf("unsupported promotion stopped_at stage %q", report.Stages.StoppedAt)
		}
		reached := false
		for _, stage := range report.Stages.Reached {
			if stage == report.Stages.StoppedAt {
				reached = true
				break
			}
		}
		if !reached {
			return fmt.Errorf("promotion stopped_at stage %q was not reached", report.Stages.StoppedAt)
		}
	}
	for _, stage := range report.Stages.Expected {
		if !validPromotionStageName(stage) {
			return fmt.Errorf("unsupported promotion expected stage %q", stage)
		}
	}
	for _, stage := range report.Stages.Reached {
		if !validPromotionStageName(stage) {
			return fmt.Errorf("unsupported promotion reached stage %q", stage)
		}
	}
	artifactPaths := map[string]struct{}{}
	for _, artifact := range report.Artifacts {
		if strings.TrimSpace(artifact.Path) == "" {
			return fmt.Errorf("promotion artifact path is required")
		}
		if artifact.Kind != "" && !validPromotionArtifactKind(artifact.Kind) {
			return fmt.Errorf("unsupported promotion artifact kind %q", artifact.Kind)
		}
		if _, exists := artifactPaths[artifact.Path]; exists {
			return fmt.Errorf("duplicate promotion artifact path %q", artifact.Path)
		}
		artifactPaths[artifact.Path] = struct{}{}
	}
	issueCodes := map[string]struct{}{}
	for _, issue := range report.Issues {
		if strings.TrimSpace(issue.Code) == "" {
			return fmt.Errorf("promotion issue code is required")
		}
		if !validPromotionIssueSeverity(issue.Severity) {
			return fmt.Errorf("unsupported promotion issue severity %q", issue.Severity)
		}
		if strings.TrimSpace(issue.Message) == "" {
			return fmt.Errorf("promotion issue message is required")
		}
		if issue.Artifact != "" {
			if _, exists := artifactPaths[issue.Artifact]; !exists {
				return fmt.Errorf("promotion issue %q references missing artifact %q", issue.Code, issue.Artifact)
			}
		}
		if _, exists := issueCodes[issue.Code]; exists {
			return fmt.Errorf("duplicate promotion issue code %q", issue.Code)
		}
		issueCodes[issue.Code] = struct{}{}
	}
	gateIDs := map[string]struct{}{}
	for _, gate := range report.Gates {
		if strings.TrimSpace(gate.ID) == "" {
			return fmt.Errorf("promotion gate id is required")
		}
		if _, exists := gateIDs[gate.ID]; exists {
			return fmt.Errorf("duplicate promotion gate id %q", gate.ID)
		}
		gateIDs[gate.ID] = struct{}{}
		if !validPromotionGateStatus(gate.Status) {
			return fmt.Errorf("unsupported promotion gate status %q", gate.Status)
		}
		for _, readiness := range gate.RequiredFor {
			if !validPromotionReadiness(readiness) {
				return fmt.Errorf("unsupported promotion gate required_for readiness %q", readiness)
			}
		}
		if hasDuplicates(gate.RequiredFor) {
			return fmt.Errorf("promotion gate %q has duplicate required_for readiness", gate.ID)
		}
		if hasDuplicates(gate.IssueCodes) {
			return fmt.Errorf("promotion gate %q has duplicate issue code references", gate.ID)
		}
		if hasDuplicates(gate.Artifacts) {
			return fmt.Errorf("promotion gate %q has duplicate artifact references", gate.ID)
		}
		for _, code := range gate.IssueCodes {
			if _, exists := issueCodes[code]; !exists {
				return fmt.Errorf("promotion gate %q references missing issue code %q", gate.ID, code)
			}
		}
		for _, path := range gate.Artifacts {
			if _, exists := artifactPaths[path]; !exists {
				return fmt.Errorf("promotion gate %q references missing artifact %q", gate.ID, path)
			}
		}
		if gateRequiresReadiness(gate, report.AchievedReadiness) {
			switch report.AchievedReadiness {
			case PromotionReadinessPass:
				if gate.Status != PromotionGateStatusPass {
					return fmt.Errorf("promotion gate %q blocks achieved readiness %q with status %q", gate.ID, report.AchievedReadiness, gate.Status)
				}
			case PromotionReadinessCandidate:
				if gate.Status != PromotionGateStatusPass && gate.Status != PromotionGateStatusWarn {
					return fmt.Errorf("promotion gate %q blocks achieved readiness %q with status %q", gate.ID, report.AchievedReadiness, gate.Status)
				}
			case PromotionReadinessExpectedFail:
				if gate.Status != PromotionGateStatusFailed {
					return fmt.Errorf("promotion gate %q blocks achieved readiness %q with status %q", gate.ID, report.AchievedReadiness, gate.Status)
				}
			case PromotionReadinessBlocked:
				if gate.Status != PromotionGateStatusFailed && gate.Status != PromotionGateStatusSkipped && gate.Status != PromotionGateStatusNotRun {
					return fmt.Errorf("promotion gate %q blocks achieved readiness %q with status %q", gate.ID, report.AchievedReadiness, gate.Status)
				}
			}
		}
	}
	actionKeys := map[string]struct{}{}
	for _, action := range report.NextActions {
		if strings.TrimSpace(action.Gate) == "" {
			return fmt.Errorf("promotion next action gate is required")
		}
		if _, exists := gateIDs[action.Gate]; !exists {
			return fmt.Errorf("promotion next action references missing gate %q", action.Gate)
		}
		if !validPromotionIssueSeverity(action.Severity) {
			return fmt.Errorf("unsupported promotion next action severity %q", action.Severity)
		}
		if strings.TrimSpace(action.Summary) == "" {
			return fmt.Errorf("promotion next action summary is required")
		}
		if strings.TrimSpace(action.Action) == "" {
			return fmt.Errorf("promotion next action action is required")
		}
		if hasDuplicates(action.IssueCodes) {
			return fmt.Errorf("promotion next action %q has duplicate issue code references", action.Gate)
		}
		if hasDuplicates(action.Artifacts) {
			return fmt.Errorf("promotion next action %q has duplicate artifact references", action.Gate)
		}
		for _, code := range action.IssueCodes {
			if _, exists := issueCodes[code]; !exists {
				return fmt.Errorf("promotion next action %q references missing issue code %q", action.Gate, code)
			}
		}
		for _, path := range action.Artifacts {
			if _, exists := artifactPaths[path]; !exists {
				return fmt.Errorf("promotion next action %q references missing artifact %q", action.Gate, path)
			}
		}
		keyIssueCodes := slices.Clone(action.IssueCodes)
		keyArtifacts := slices.Clone(action.Artifacts)
		sort.Strings(keyIssueCodes)
		sort.Strings(keyArtifacts)
		key := action.Gate + "\x01" + strings.Join(keyIssueCodes, "\x00") + "\x02" + strings.Join(keyArtifacts, "\x00") + "\x03" + action.Action
		if _, exists := actionKeys[key]; exists {
			return fmt.Errorf("duplicate promotion next action for gate %q", action.Gate)
		}
		actionKeys[key] = struct{}{}
	}
	return nil
}

// NormalizePromotionReport returns a deterministic copy without mutating input.
func NormalizePromotionReport(report PromotionReport) PromotionReport {
	// Start with scalar fields, then deep-copy all slice-backed fields below.
	normalized := report
	normalized.Gates = slices.Clone(report.Gates)
	for i := range normalized.Gates {
		normalized.Gates[i].RequiredFor = slices.Clone(report.Gates[i].RequiredFor)
		normalized.Gates[i].IssueCodes = slices.Clone(report.Gates[i].IssueCodes)
		normalized.Gates[i].Artifacts = slices.Clone(report.Gates[i].Artifacts)
		sort.Slice(normalized.Gates[i].RequiredFor, func(a, b int) bool {
			return normalized.Gates[i].RequiredFor[a] < normalized.Gates[i].RequiredFor[b]
		})
		normalized.Gates[i].RequiredFor = slices.Compact(normalized.Gates[i].RequiredFor)
		sort.Strings(normalized.Gates[i].IssueCodes)
		normalized.Gates[i].IssueCodes = slices.Compact(normalized.Gates[i].IssueCodes)
		sort.Strings(normalized.Gates[i].Artifacts)
		normalized.Gates[i].Artifacts = slices.Compact(normalized.Gates[i].Artifacts)
	}
	sort.Slice(normalized.Gates, func(i, j int) bool {
		return normalized.Gates[i].ID < normalized.Gates[j].ID
	})
	normalized.Stages.Expected = slices.Clone(report.Stages.Expected)
	normalized.Stages.Reached = slices.Clone(report.Stages.Reached)
	normalized.Issues = slices.Clone(report.Issues)
	for i := range normalized.Issues {
		normalized.Issues[i].Refs = slices.Clone(report.Issues[i].Refs)
		normalized.Issues[i].Nets = slices.Clone(report.Issues[i].Nets)
		sort.Strings(normalized.Issues[i].Refs)
		normalized.Issues[i].Refs = slices.Compact(normalized.Issues[i].Refs)
		sort.Strings(normalized.Issues[i].Nets)
		normalized.Issues[i].Nets = slices.Compact(normalized.Issues[i].Nets)
	}
	sort.Slice(normalized.Issues, func(i, j int) bool {
		if normalized.Issues[i].Stage != normalized.Issues[j].Stage {
			return normalized.Issues[i].Stage < normalized.Issues[j].Stage
		}
		if normalized.Issues[i].Code != normalized.Issues[j].Code {
			return normalized.Issues[i].Code < normalized.Issues[j].Code
		}
		if normalized.Issues[i].Message != normalized.Issues[j].Message {
			return normalized.Issues[i].Message < normalized.Issues[j].Message
		}
		if normalized.Issues[i].Path != normalized.Issues[j].Path {
			return normalized.Issues[i].Path < normalized.Issues[j].Path
		}
		if normalized.Issues[i].Artifact != normalized.Issues[j].Artifact {
			return normalized.Issues[i].Artifact < normalized.Issues[j].Artifact
		}
		if normalized.Issues[i].Repair != normalized.Issues[j].Repair {
			return normalized.Issues[i].Repair < normalized.Issues[j].Repair
		}
		if normalized.Issues[i].Severity != normalized.Issues[j].Severity {
			return normalized.Issues[i].Severity < normalized.Issues[j].Severity
		}
		if cmp := slices.Compare(normalized.Issues[i].Refs, normalized.Issues[j].Refs); cmp != 0 {
			return cmp < 0
		}
		return slices.Compare(normalized.Issues[i].Nets, normalized.Issues[j].Nets) < 0
	})
	normalized.NextActions = slices.Clone(report.NextActions)
	for i := range normalized.NextActions {
		normalized.NextActions[i].IssueCodes = slices.Clone(report.NextActions[i].IssueCodes)
		normalized.NextActions[i].Artifacts = slices.Clone(report.NextActions[i].Artifacts)
		sort.Strings(normalized.NextActions[i].IssueCodes)
		normalized.NextActions[i].IssueCodes = slices.Compact(normalized.NextActions[i].IssueCodes)
		sort.Strings(normalized.NextActions[i].Artifacts)
		normalized.NextActions[i].Artifacts = slices.Compact(normalized.NextActions[i].Artifacts)
	}
	sort.Slice(normalized.NextActions, func(i, j int) bool {
		if normalized.NextActions[i].Gate != normalized.NextActions[j].Gate {
			return normalized.NextActions[i].Gate < normalized.NextActions[j].Gate
		}
		if normalized.NextActions[i].Severity != normalized.NextActions[j].Severity {
			return normalized.NextActions[i].Severity < normalized.NextActions[j].Severity
		}
		if normalized.NextActions[i].Summary != normalized.NextActions[j].Summary {
			return normalized.NextActions[i].Summary < normalized.NextActions[j].Summary
		}
		if normalized.NextActions[i].Action != normalized.NextActions[j].Action {
			return normalized.NextActions[i].Action < normalized.NextActions[j].Action
		}
		leftIssueCodes := normalized.NextActions[i].IssueCodes
		rightIssueCodes := normalized.NextActions[j].IssueCodes
		if cmp := slices.Compare(leftIssueCodes, rightIssueCodes); cmp != 0 {
			return cmp < 0
		}
		return slices.Compare(normalized.NextActions[i].Artifacts, normalized.NextActions[j].Artifacts) < 0
	})
	normalized.Artifacts = slices.Clone(report.Artifacts)
	sort.Slice(normalized.Artifacts, func(i, j int) bool {
		return normalized.Artifacts[i].Path < normalized.Artifacts[j].Path
	})
	return normalized
}

func gateRequiresReadiness(gate PromotionGate, readiness PromotionReadiness) bool {
	for _, required := range gate.RequiredFor {
		if required == readiness {
			return true
		}
	}
	return false
}

func promotionStatusBypassesReadiness(status PromotionStatus) bool {
	switch status {
	case PromotionStatusError, PromotionStatusSkipped, PromotionStatusNotRun:
		return true
	default:
		return false
	}
}

func hasDuplicates[T comparable](values []T) bool {
	seen := map[T]struct{}{}
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

// MarshalPromotionReportJSON serializes a validated normalized promotion report.
func MarshalPromotionReportJSON(report PromotionReport) ([]byte, error) {
	if err := report.Validate(); err != nil {
		return nil, err
	}
	normalized := NormalizePromotionReport(report)
	return json.MarshalIndent(normalized, "", "  ")
}

func PromotionSummaryFromReport(report PromotionReport, reportPath string) PromotionSummary {
	return PromotionSummary{
		Status:             report.Status,
		DeclaredReadiness:  report.DeclaredReadiness,
		AchievedReadiness:  report.AchievedReadiness,
		MatchesExpectation: report.MatchesExpectation,
		ReportPath:         filepath.ToSlash(reportPath),
	}
}

func WritePromotionReportArtifact(outputRoot string, report PromotionReport, overwrite bool) (reports.Artifact, *reports.Issue) {
	if strings.TrimSpace(outputRoot) == "" {
		return reports.Artifact{}, &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "output", Message: "output root is required"}
	}
	data, err := MarshalPromotionReportJSON(report)
	if err != nil {
		return reports.Artifact{}, &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: PromotionReportArtifactPath, Message: err.Error()}
	}
	path := filepath.Join(outputRoot, filepath.FromSlash(PromotionReportArtifactPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return reports.Artifact{}, &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: PromotionReportArtifactPath, Message: err.Error()}
	}
	flags := os.O_WRONLY | os.O_CREATE
	if overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return reports.Artifact{}, &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: PromotionReportArtifactPath, Message: err.Error()}
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return reports.Artifact{}, &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: PromotionReportArtifactPath, Message: err.Error()}
	}
	if _, err := file.Write([]byte{'\n'}); err != nil {
		_ = file.Close()
		return reports.Artifact{}, &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: PromotionReportArtifactPath, Message: err.Error()}
	}
	if err := file.Close(); err != nil {
		return reports.Artifact{}, &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: PromotionReportArtifactPath, Message: err.Error()}
	}
	return reports.Artifact{Kind: reports.ArtifactPromotionReport, Path: PromotionReportArtifactPath, Description: "KiCad-backed design promotion report"}, nil
}

func validPromotionReadiness(readiness PromotionReadiness) bool {
	switch readiness {
	case PromotionReadinessExpectedFail, PromotionReadinessCandidate, PromotionReadinessPass, PromotionReadinessBlocked:
		return true
	default:
		return false
	}
}

func validPromotionStatus(status PromotionStatus) bool {
	switch status {
	case PromotionStatusPass, PromotionStatusWarn, PromotionStatusFailed, PromotionStatusExpectedFail, PromotionStatusUnexpectedPass, PromotionStatusBlocked, PromotionStatusSkipped, PromotionStatusError, PromotionStatusNotRun:
		return true
	default:
		return false
	}
}

func validPromotionGateStatus(status PromotionGateStatus) bool {
	switch status {
	case PromotionGateStatusPass, PromotionGateStatusWarn, PromotionGateStatusFailed, PromotionGateStatusSkipped, PromotionGateStatusNotRun:
		return true
	default:
		return false
	}
}

func validPromotionAcceptance(acceptance AcceptanceLevel) bool {
	switch acceptance {
	case AcceptanceDraft, AcceptanceStructural, AcceptanceConnectivity, AcceptanceERCDRC, AcceptanceFabricationCandidate:
		return true
	default:
		return false
	}
}

func validPromotionStageName(stage StageName) bool {
	switch stage {
	case StageParseRequest, StageLibraryContext, StageBlockPlanning, StageComponentSelection, StageSchematic, StageSchematicElectrical, StagePCBRealization, StageSchematicToPCB, StagePlacement, StageRouting, StageProjectWrite, StageWriterCorrect, StageValidation, StageValidationRepair, StageKiCadChecks, StageFabricationReady, StageFeedback:
		return true
	default:
		return false
	}
}

func validPromotionArtifactKind(kind reports.ArtifactKind) bool {
	switch kind {
	case reports.ArtifactKiCadProject, reports.ArtifactSchematic, reports.ArtifactPCB, reports.ArtifactSymbolLibraryTable, reports.ArtifactFootprintLibraryTable, reports.ArtifactValidationReport, reports.ArtifactPromotionReport, reports.ArtifactRoundTripReport, reports.ArtifactDRCReport, reports.ArtifactERCReport, reports.ArtifactPreview, reports.ArtifactBOM, reports.ArtifactCPL, reports.ArtifactGerber, reports.ArtifactDrill, reports.ArtifactFabricationPackage:
		return true
	default:
		return false
	}
}

func validPromotionIssueSeverity(severity reports.Severity) bool {
	switch severity {
	case reports.SeverityInfo, reports.SeverityWarning, reports.SeverityError, reports.SeverityBlocked:
		return true
	default:
		return false
	}
}
