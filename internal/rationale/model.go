package rationale

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/designworkflow"
	"kicadai/internal/intentdraft"
	"kicadai/internal/intentplanner"
	"kicadai/internal/reports"
)

const (
	Schema          = "kicadai.design.rationale.v1"
	ArtifactName    = "design-rationale.json"
	MetadataDirName = ".kicadai"
)

type Status string

const (
	StatusReady              Status = "ready"
	StatusPartial            Status = "partial"
	StatusNeedsClarification Status = "needs_clarification"
	StatusBlocked            Status = "blocked"
)

type Report struct {
	Schema         string             `json:"schema"`
	Status         Status             `json:"status"`
	Source         SourceSummary      `json:"source"`
	Intent         IntentSummary      `json:"intent"`
	Decisions      []Decision         `json:"decisions,omitempty"`
	Evidence       []EvidenceRecord   `json:"evidence,omitempty"`
	Assumptions    []RationaleNote    `json:"assumptions,omitempty"`
	Clarifications []RationaleNote    `json:"clarifications,omitempty"`
	KnownLimits    []KnownLimit       `json:"known_limits,omitempty"`
	Validation     ValidationSummary  `json:"validation,omitempty"`
	NextActions    []NextAction       `json:"next_actions,omitempty"`
	ArtifactRefs   []reports.Artifact `json:"artifact_refs,omitempty"`
}

type SourceSummary struct {
	Mode       string `json:"mode"`
	Path       string `json:"path,omitempty"`
	SourceHash string `json:"source_hash,omitempty"`
	Summary    string `json:"summary,omitempty"`
}

type IntentSummary struct {
	Name                string                         `json:"name,omitempty"`
	Kind                string                         `json:"kind,omitempty"`
	RequestedAcceptance designworkflow.AcceptanceLevel `json:"requested_acceptance,omitempty"`
	NormalizedSummary   string                         `json:"normalized_summary,omitempty"`
	DraftedSummary      string                         `json:"drafted_summary,omitempty"`
	Board               BoardSummary                   `json:"board,omitempty"`
	Power               []string                       `json:"power,omitempty"`
	Interfaces          []string                       `json:"interfaces,omitempty"`
	Functions           []string                       `json:"functions,omitempty"`
	Manufacturing       []string                       `json:"manufacturing,omitempty"`
	Constraints         []string                       `json:"constraints,omitempty"`
}

type BoardSummary struct {
	WidthMM  float64 `json:"width_mm,omitempty"`
	HeightMM float64 `json:"height_mm,omitempty"`
	Layers   int     `json:"layers,omitempty"`
}

type Decision struct {
	ID             string   `json:"id"`
	Type           string   `json:"type"`
	Path           string   `json:"path,omitempty"`
	Selected       string   `json:"selected"`
	Rationale      string   `json:"rationale"`
	RequirementIDs []string `json:"requirement_ids,omitempty"`
	EvidenceIDs    []string `json:"evidence_ids,omitempty"`
	Confidence     string   `json:"confidence,omitempty"`
	Status         string   `json:"status,omitempty"`
}

type EvidenceRecord struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Path        string   `json:"path,omitempty"`
	Summary     string   `json:"summary"`
	SourceText  string   `json:"source_text,omitempty"`
	Confidence  float64  `json:"confidence,omitempty"`
	ArtifactRef string   `json:"artifact_ref,omitempty"`
	Notes       []string `json:"notes,omitempty"`
}

type RationaleNote struct {
	ID          string           `json:"id"`
	Path        string           `json:"path,omitempty"`
	Message     string           `json:"message"`
	Severity    reports.Severity `json:"severity,omitempty"`
	Suggestion  string           `json:"suggestion,omitempty"`
	EvidenceIDs []string         `json:"evidence_ids,omitempty"`
}

type KnownLimit struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Severity    string   `json:"severity"`
	Path        string   `json:"path,omitempty"`
	Message     string   `json:"message"`
	Suggestion  string   `json:"suggestion,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
}

type ValidationSummary struct {
	RequestedAcceptance string `json:"requested_acceptance,omitempty"`
	AchievedAcceptance  string `json:"achieved_acceptance,omitempty"`
	BlockingCount       int    `json:"blocking_count"`
	WarningCount        int    `json:"warning_count"`
	StageCount          int    `json:"stage_count,omitempty"`
	CompletedStages     int    `json:"completed_stages,omitempty"`
	SkippedStages       int    `json:"skipped_stages,omitempty"`
}

type NextAction struct {
	ID         string `json:"id"`
	Priority   int    `json:"priority"`
	Action     string `json:"action"`
	Reason     string `json:"reason"`
	Command    string `json:"command,omitempty"`
	TargetPath string `json:"target_path,omitempty"`
}

type BuildOptions struct {
	Source   SourceSummary
	Request  *intentplanner.Request
	Draft    *intentdraft.Result
	Plan     *intentplanner.PlanResult
	Workflow *designworkflow.WorkflowResult
}

type TargetLoadResult struct {
	Report Report
	Issues []reports.Issue
}

func BuildFromPlan(plan intentplanner.PlanResult, source SourceSummary) Report {
	normalized := intentplanner.NormalizePlan(plan)
	return Build(BuildOptions{Source: source, Plan: &normalized})
}

func BuildFromDraftAndPlan(draft intentdraft.Result, plan intentplanner.PlanResult, source SourceSummary) Report {
	normalizedPlan := intentplanner.NormalizePlan(plan)
	return Build(BuildOptions{Source: source, Draft: &draft, Plan: &normalizedPlan, Request: &draft.Request})
}

func Build(options BuildOptions) Report {
	report := Report{
		Schema: Schema,
		Source: normalizeSource(options.Source),
	}
	if options.Request != nil {
		request := intentplanner.NormalizeRequest(*options.Request)
		report.Intent = intentFromRequest(request)
	}
	if options.Plan != nil {
		plan := intentplanner.NormalizePlan(*options.Plan)
		if report.Intent.Name == "" {
			report.Intent = intentFromPlan(plan)
		}
		report.Evidence = append(report.Evidence, evidenceFromRequirements(plan.Requirements)...)
		report.Decisions = append(report.Decisions, decisionsFromPlan(plan)...)
		report.Assumptions = append(report.Assumptions, notesFromPlan(plan.Assumptions, nil)...)
		report.Clarifications = append(report.Clarifications, notesFromPlan(plan.Clarifications, nil)...)
		report.KnownLimits = append(report.KnownLimits, limitsFromPlan(plan)...)
		report.Validation.RequestedAcceptance = string(plan.Intent.Acceptance)
		report.ArtifactRefs = append(report.ArtifactRefs, plan.Artifacts...)
	}
	if options.Draft != nil {
		applyDraft(&report, *options.Draft)
	}
	if options.Workflow != nil {
		applyWorkflow(&report, *options.Workflow)
	}
	report.Status = deriveStatus(report)
	report.NextActions = append(report.NextActions, nextActions(report)...)
	return Normalize(report)
}

func Normalize(report Report) Report {
	report.Schema = firstNonEmpty(report.Schema, Schema)
	report.Source = normalizeSource(report.Source)
	report.Evidence = cloneEvidence(report.Evidence)
	report.Decisions = cloneDecisions(report.Decisions)
	report.Assumptions = cloneNotes(report.Assumptions)
	report.Clarifications = cloneNotes(report.Clarifications)
	report.KnownLimits = cloneLimits(report.KnownLimits)
	report.NextActions = cloneNextActions(report.NextActions)
	report.ArtifactRefs = append([]reports.Artifact(nil), report.ArtifactRefs...)
	slices.SortFunc(report.Evidence, compareEvidence)
	slices.SortFunc(report.Decisions, compareDecisions)
	slices.SortFunc(report.Assumptions, compareRationaleNotes)
	slices.SortFunc(report.Clarifications, compareRationaleNotes)
	slices.SortFunc(report.KnownLimits, compareKnownLimits)
	slices.SortFunc(report.NextActions, compareNextActions)
	slices.SortFunc(report.ArtifactRefs, compareArtifacts)
	if report.Status == "" {
		report.Status = deriveStatus(report)
	}
	return report
}

func MarshalJSON(report Report) ([]byte, error) {
	normalized := Normalize(report)
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func WriteArtifact(path string, report Report) (reports.Artifact, *reports.Issue) {
	data, err := MarshalJSON(report)
	if err != nil {
		issue := artifactIssue(path, err.Error())
		return reports.Artifact{}, &issue
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		issue := artifactIssue(path, err.Error())
		return reports.Artifact{}, &issue
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		issue := artifactIssue(path, err.Error())
		return reports.Artifact{}, &issue
	}
	return reports.Artifact{Kind: reports.ArtifactPreview, Path: filepath.Base(path), Description: "design rationale report"}, nil
}

func ReportPathForTarget(target string) string {
	return filepath.Join(target, MetadataDirName, ArtifactName)
}

func normalizeSource(source SourceSummary) SourceSummary {
	source.Mode = strings.TrimSpace(source.Mode)
	source.Path = strings.TrimSpace(source.Path)
	source.SourceHash = strings.TrimSpace(source.SourceHash)
	source.Summary = strings.TrimSpace(source.Summary)
	return source
}

func artifactIssue(path string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: path, Message: message}
}
