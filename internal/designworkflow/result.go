package designworkflow

import (
	"cmp"
	"slices"

	"kicadai/internal/reports"
)

type StageName string

const (
	StageParseRequest       StageName = "parse_request"
	StageLibraryContext     StageName = "library_context"
	StageBlockPlanning      StageName = "block_planning"
	StageComponentSelection StageName = "component_selection"
	StageSchematic          StageName = "schematic"
	StagePCBRealization     StageName = "pcb_realization"
	StageSchematicToPCB     StageName = "schematic_to_pcb"
	StagePlacement          StageName = "placement"
	StageRouting            StageName = "routing"
	StageProjectWrite       StageName = "project_write"
	StageWriterCorrect      StageName = "writer_correctness"
	StageValidation         StageName = "validation"
	StageValidationRepair   StageName = "validation_repair"
	StageKiCadChecks        StageName = "kicad_checks"
	StageFabricationReady   StageName = "fabrication_readiness"
	StageFeedback           StageName = "feedback"
)

type StageStatus string

const (
	StageStatusOK      StageStatus = "ok"
	StageStatusWarning StageStatus = "warning"
	StageStatusBlocked StageStatus = "blocked"
	StageStatusSkipped StageStatus = "skipped"
)

type RetryScope string

const (
	RetryScopeRequest   RetryScope = "request"
	RetryScopeBlock     RetryScope = "block"
	RetryScopePlacement RetryScope = "placement"
	RetryScopeRouting   RetryScope = "routing"
	RetryScopeWriter    RetryScope = "writer"
	RetryScopeExternal  RetryScope = "external"
)

type WorkflowResult struct {
	Project    ProjectSummary    `json:"project"`
	Acceptance AcceptanceResult  `json:"acceptance"`
	Stages     []StageResult     `json:"stages"`
	Feedback   Feedback          `json:"feedback"`
	Promotion  *PromotionSummary `json:"promotion,omitempty"`
}

type ProjectSummary struct {
	Name      string `json:"name"`
	OutputDir string `json:"output_dir,omitempty"`
}

type AcceptanceResult struct {
	Requested        AcceptanceLevel `json:"requested"`
	Achieved         AcceptanceLevel `json:"achieved"`
	FabricationReady bool            `json:"fabrication_ready"`
}

type StageResult struct {
	Name      StageName          `json:"name"`
	Status    StageStatus        `json:"status"`
	Summary   map[string]any     `json:"summary,omitempty"`
	Issues    []reports.Issue    `json:"issues,omitempty"`
	Artifacts []reports.Artifact `json:"artifacts,omitempty"`
}

type Feedback struct {
	Summary FeedbackSummary    `json:"summary"`
	Repairs []RepairSuggestion `json:"repairs,omitempty"`
}

type FeedbackSummary struct {
	IssueCount    int `json:"issue_count"`
	BlockingCount int `json:"blocking_count"`
	ErrorCount    int `json:"error_count"`
	WarningCount  int `json:"warning_count"`
	SkippedCount  int `json:"skipped_count"`
}

type RepairSuggestion struct {
	Stage           StageName        `json:"stage"`
	Severity        reports.Severity `json:"severity"`
	Code            reports.Code     `json:"code"`
	Message         string           `json:"message"`
	BlockID         string           `json:"block_id,omitempty"`
	InstanceID      string           `json:"instance_id,omitempty"`
	OperationID     string           `json:"operation_id,omitempty"`
	Refs            []string         `json:"refs,omitempty"`
	Nets            []string         `json:"nets,omitempty"`
	Artifact        string           `json:"artifact,omitempty"`
	SuggestedAction string           `json:"suggested_action,omitempty"`
	RetryScope      RetryScope       `json:"retry_scope"`
}

func NewStageResult(name StageName, issues []reports.Issue) StageResult {
	return StageResult{Name: name, Status: StageStatusForIssues(issues), Issues: cloneIssues(issues)}
}

func StageStatusForIssues(issues []reports.Issue) StageStatus {
	if len(issues) == 0 {
		return StageStatusOK
	}
	for _, issue := range issues {
		if issue.Blocking() {
			return StageStatusBlocked
		}
	}
	return StageStatusWarning
}

func BuildWorkflowResult(project ProjectSummary, requested AcceptanceLevel, stages []StageResult) WorkflowResult {
	normalized := append([]StageResult(nil), stages...)
	for i := range normalized {
		if normalized[i].Status == "" {
			normalized[i].Status = StageStatusForIssues(normalized[i].Issues)
		}
		normalized[i].Issues = cloneIssues(normalized[i].Issues)
		normalized[i].Artifacts = append([]reports.Artifact(nil), normalized[i].Artifacts...)
	}
	feedback := BuildFeedback(normalized)
	achieved := AchievedAcceptance(requested, normalized)
	return WorkflowResult{
		Project: project,
		Acceptance: AcceptanceResult{
			Requested:        requested,
			Achieved:         achieved,
			FabricationReady: achieved == AcceptanceFabricationCandidate,
		},
		Stages:   normalized,
		Feedback: feedback,
	}
}

func BuildFeedback(stages []StageResult) Feedback {
	var feedback Feedback
	for _, stage := range stages {
		if stage.Status == StageStatusSkipped {
			feedback.Summary.SkippedCount++
		}
		for _, issue := range stage.Issues {
			feedback.Summary.IssueCount++
			switch issue.Severity {
			case reports.SeverityWarning:
				feedback.Summary.WarningCount++
			case reports.SeverityError:
				feedback.Summary.ErrorCount++
			case reports.SeverityBlocked:
				feedback.Summary.ErrorCount++
			}
			if issue.Blocking() {
				feedback.Summary.BlockingCount++
			}
			feedback.Repairs = append(feedback.Repairs, RepairSuggestion{
				Stage:           stage.Name,
				Severity:        issue.Severity,
				Code:            issue.Code,
				Message:         issue.Message,
				OperationID:     issue.OperationID,
				Refs:            append([]string(nil), issue.Refs...),
				Nets:            append([]string(nil), issue.Nets...),
				SuggestedAction: firstNonEmpty(issue.Suggestion, defaultRepairAction(stage.Name, issue)),
				RetryScope:      RetryScopeForStage(stage.Name, issue),
			})
		}
	}
	sortRepairs(feedback.Repairs)
	return feedback
}

func AchievedAcceptance(requested AcceptanceLevel, stages []StageResult) AcceptanceLevel {
	if len(stages) == 0 {
		return AcceptanceDraft
	}
	blocked := map[StageName]struct{}{}
	completed := map[StageName]struct{}{}
	for _, stage := range stages {
		if stage.Status == StageStatusSkipped {
			continue
		}
		if stage.Status == StageStatusBlocked || reports.HasBlockingIssue(stage.Issues) {
			blocked[stage.Name] = struct{}{}
			continue
		}
		completed[stage.Name] = struct{}{}
	}
	_, completedKiCadChecks := completed[StageKiCadChecks]
	_, completedFabrication := completed[StageFabricationReady]
	_, completedValidation := completed[StageValidation]
	switch {
	case hasAnyBlocked(blocked, StageParseRequest, StageBlockPlanning, StageComponentSelection, StageSchematic, StageProjectWrite):
		return ""
	case missingAny(completed, StageSchematic):
		return AcceptanceDraft
	case hasAnyBlocked(blocked, StagePCBRealization, StageSchematicToPCB, StagePlacement, StageRouting, StageWriterCorrect, StageValidation):
		return AcceptanceStructural
	case hasAnyBlocked(blocked, StageKiCadChecks):
		return AcceptanceConnectivity
	case hasAnyBlocked(blocked, StageFabricationReady) && requestedAtLeast(requested, AcceptanceFabricationCandidate):
		if completedKiCadChecks {
			return AcceptanceERCDRC
		}
		return AcceptanceConnectivity
	case completedKiCadChecks && completedFabrication && requestedAtLeast(requested, AcceptanceFabricationCandidate):
		return AcceptanceFabricationCandidate
	case completedKiCadChecks && requestedAtLeast(requested, AcceptanceERCDRC):
		return AcceptanceERCDRC
	case completedValidation && requestedAtLeast(requested, AcceptanceConnectivity):
		return minAcceptance(requested, AcceptanceConnectivity)
	default:
		return minAcceptance(requested, AcceptanceStructural)
	}
}

func AcceptanceSatisfied(requested AcceptanceLevel, achieved AcceptanceLevel) bool {
	if requested == "" {
		requested = AcceptanceDraft
	}
	return acceptanceRank(achieved) >= acceptanceRank(requested)
}

func RetryScopeForStage(stage StageName, issue reports.Issue) RetryScope {
	switch stage {
	case StageParseRequest:
		return RetryScopeRequest
	case StageBlockPlanning, StageComponentSelection, StagePCBRealization:
		return RetryScopeBlock
	case StagePlacement:
		return RetryScopePlacement
	case StageRouting:
		return RetryScopeRouting
	case StageProjectWrite, StageSchematic, StageSchematicToPCB:
		return RetryScopeWriter
	case StageLibraryContext, StageKiCadChecks, StageFabricationReady, StageValidation:
		if issue.Code == reports.CodeKiCadCLIFailed || issue.Code == reports.CodeSkippedExternalTool {
			return RetryScopeExternal
		}
		return RetryScopeRequest
	default:
		return RetryScopeRequest
	}
}

func defaultRepairAction(stage StageName, issue reports.Issue) string {
	if issue.Suggestion != "" {
		return issue.Suggestion
	}
	switch issue.Code {
	case reports.CodeMissingFile:
		if stage == StageBlockPlanning {
			return "select a supported circuit block or add the block to the registry"
		}
		return "provide the missing file or update the configured path"
	case reports.CodeInvalidArgument:
		return "correct the invalid request field and rerun the workflow"
	case reports.CodeMissingFootprint, reports.CodeUnknownFootprintLibrary:
		return "assign a KiCad-resolvable footprint or configure the footprint library roots"
	case reports.CodePlacementOutsideBoard:
		return "increase board size or relax placement constraints"
	case reports.CodePlacementCollision:
		return "adjust component positions, spacing, or placement constraints"
	case reports.CodeDisconnectedPad, reports.CodeInvalidNetAssignment:
		return "repair net-to-pad assignments and rerun routing"
	case reports.CodeKiCadCLIFailed:
		return "inspect the KiCad ERC/DRC report and repair the reported design rule finding"
	case reports.CodeSkippedExternalTool:
		return "provide kicad-cli or lower the requested external validation level"
	}
	switch RetryScopeForStage(stage, issue) {
	case RetryScopeRequest:
		return "modify the design request and rerun the workflow"
	case RetryScopeBlock:
		return "adjust block selection or block parameters"
	case RetryScopePlacement:
		return "adjust board size, keepouts, or placement constraints"
	case RetryScopeRouting:
		return "adjust routing constraints, board size, or allow additional layers"
	case RetryScopeWriter:
		return "inspect the generated transaction and writer inputs"
	case RetryScopeExternal:
		return "provide the required external tool or review its output"
	default:
		return "review the issue and rerun the workflow"
	}
}

func requestedAtLeast(level AcceptanceLevel, threshold AcceptanceLevel) bool {
	return acceptanceRank(level) >= acceptanceRank(threshold)
}

func minAcceptance(a AcceptanceLevel, b AcceptanceLevel) AcceptanceLevel {
	if acceptanceRank(a) < acceptanceRank(b) {
		return a
	}
	return b
}

func acceptanceRank(level AcceptanceLevel) int {
	switch level {
	case AcceptanceDraft:
		return 1
	case AcceptanceStructural:
		return 2
	case AcceptanceConnectivity:
		return 3
	case AcceptanceERCDRC:
		return 4
	case AcceptanceFabricationCandidate:
		return 5
	default:
		return 0
	}
}

func hasAnyBlocked(blocked map[StageName]struct{}, names ...StageName) bool {
	for _, name := range names {
		if _, ok := blocked[name]; ok {
			return true
		}
	}
	return false
}

func missingAny(completed map[StageName]struct{}, names ...StageName) bool {
	for _, name := range names {
		if _, ok := completed[name]; !ok {
			return true
		}
	}
	return false
}

func cloneIssues(issues []reports.Issue) []reports.Issue {
	clone := append([]reports.Issue(nil), issues...)
	for i := range clone {
		clone[i].UUIDs = append([]string(nil), issues[i].UUIDs...)
		clone[i].Refs = append([]string(nil), issues[i].Refs...)
		clone[i].Nets = append([]string(nil), issues[i].Nets...)
	}
	return clone
}

func sortRepairs(repairs []RepairSuggestion) {
	slices.SortFunc(repairs, func(a, b RepairSuggestion) int {
		if byStage := cmp.Compare(a.Stage, b.Stage); byStage != 0 {
			return byStage
		}
		if byCode := cmp.Compare(a.Code, b.Code); byCode != 0 {
			return byCode
		}
		return cmp.Compare(a.Message, b.Message)
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
