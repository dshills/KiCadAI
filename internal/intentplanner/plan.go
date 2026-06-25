package intentplanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
)

const PlanSchema = "kicadai.intent.plan.v1"

type PlanStatus string

const (
	PlanStatusReady              PlanStatus = "ready"
	PlanStatusNeedsClarification PlanStatus = "needs_clarification"
	PlanStatusBlocked            PlanStatus = "blocked"
	PlanStatusPartial            PlanStatus = "partial"
)

type PlanResult struct {
	Schema             string                    `json:"schema"`
	Status             PlanStatus                `json:"status"`
	Score              int                       `json:"score"`
	Intent             PlanIntentSummary         `json:"intent"`
	GeneratedRequest   *designworkflow.Request   `json:"generated_request,omitempty"`
	Requirements       []RequirementRecord       `json:"requirements"`
	SelectedBlocks     []SelectedBlockRecord     `json:"selected_blocks"`
	SelectedComponents []SelectedComponentRecord `json:"selected_components"`
	Connections        []ConnectionRecord        `json:"connections"`
	Assumptions        []PlanNote                `json:"assumptions"`
	Clarifications     []PlanNote                `json:"clarifications"`
	KnownGaps          []PlanNote                `json:"known_gaps"`
	Issues             []reports.Issue           `json:"issues"`
	Artifacts          []reports.Artifact        `json:"artifacts"`
}

type PlanIntentSummary struct {
	Name       string                         `json:"name"`
	Kind       IntentKind                     `json:"kind"`
	Acceptance designworkflow.AcceptanceLevel `json:"acceptance"`
	Summary    string                         `json:"summary,omitempty"`
}

type RequirementRecord struct {
	ID             string   `json:"id"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	Strength       Strength `json:"strength"`
	Value          string   `json:"value,omitempty"`
	Implementation string   `json:"implementation,omitempty"`
	OmittedReason  string   `json:"omitted_reason,omitempty"`
	Evidence       []string `json:"evidence,omitempty"`
}

type SelectedBlockRecord struct {
	RequirementIDs []string       `json:"requirement_ids"`
	InstanceID     string         `json:"instance_id"`
	BlockID        string         `json:"block_id"`
	Params         map[string]any `json:"params,omitempty"`
	Readiness      string         `json:"readiness,omitempty"`
	Verification   string         `json:"verification,omitempty"`
	RequiredRoutes []string       `json:"required_routes,omitempty"`
	KnownGaps      []string       `json:"known_gaps,omitempty"`
	Rationale      string         `json:"rationale,omitempty"`
}

type SelectedComponentRecord struct {
	RequirementID     string   `json:"requirement_id,omitempty"`
	Family            string   `json:"family,omitempty"`
	PackagePreference string   `json:"package_preference,omitempty"`
	MinimumConfidence string   `json:"minimum_confidence,omitempty"`
	Acceptance        string   `json:"acceptance,omitempty"`
	RequiredRatings   []string `json:"required_ratings,omitempty"`
	AllowPlaceholder  bool     `json:"allow_placeholder,omitempty"`
	Rationale         string   `json:"rationale,omitempty"`
}

type ConnectionRecord struct {
	RequirementIDs []string `json:"requirement_ids,omitempty"`
	From           string   `json:"from"`
	To             string   `json:"to"`
	NetAlias       string   `json:"net_alias,omitempty"`
	Rationale      string   `json:"rationale,omitempty"`
}

type PlanNote struct {
	ID         string           `json:"id"`
	Path       string           `json:"path,omitempty"`
	Message    string           `json:"message"`
	Severity   reports.Severity `json:"severity,omitempty"`
	Suggestion string           `json:"suggestion,omitempty"`
}

type ArtifactOptions struct {
	OutputDir string
	Overwrite bool
}

type planAlias PlanResult

func NewPlan(request Request) PlanResult {
	normalized := NormalizeRequest(request)
	issues := ValidateRequest(normalized)
	plan := PlanResult{
		Schema: PlanSchema,
		Intent: PlanIntentSummary{
			Name:       normalized.Name,
			Kind:       normalized.Kind,
			Acceptance: normalized.Acceptance,
			Summary:    normalized.Summary,
		},
		Issues: issues,
	}
	return NormalizePlan(plan)
}

func NormalizePlan(plan PlanResult) PlanResult {
	if strings.TrimSpace(plan.Schema) == "" {
		plan.Schema = PlanSchema
	}
	plan.Requirements = cloneRequirements(plan.Requirements)
	plan.SelectedBlocks = cloneSelectedBlocks(plan.SelectedBlocks)
	plan.SelectedComponents = cloneSelectedComponents(plan.SelectedComponents)
	plan.Connections = cloneConnections(plan.Connections)
	plan.Assumptions = cloneNotes(plan.Assumptions)
	plan.Clarifications = cloneNotes(plan.Clarifications)
	plan.KnownGaps = cloneNotes(plan.KnownGaps)
	plan.Issues = append([]reports.Issue(nil), plan.Issues...)
	plan.Artifacts = append([]reports.Artifact(nil), plan.Artifacts...)
	slices.SortFunc(plan.Requirements, compareRequirements)
	slices.SortFunc(plan.SelectedBlocks, compareSelectedBlocks)
	slices.SortFunc(plan.SelectedComponents, compareSelectedComponents)
	slices.SortFunc(plan.Connections, compareConnections)
	slices.SortFunc(plan.Assumptions, compareNotes)
	slices.SortFunc(plan.Clarifications, compareNotes)
	slices.SortFunc(plan.KnownGaps, compareNotes)
	slices.SortFunc(plan.Issues, compareIssues)
	slices.SortFunc(plan.Artifacts, compareArtifacts)
	if plan.Status == "" {
		plan.Status = calculatePlanStatus(plan)
	}
	plan.Score = calculatePlanScore(plan)
	if plan.Requirements == nil {
		plan.Requirements = []RequirementRecord{}
	}
	if plan.SelectedBlocks == nil {
		plan.SelectedBlocks = []SelectedBlockRecord{}
	}
	if plan.SelectedComponents == nil {
		plan.SelectedComponents = []SelectedComponentRecord{}
	}
	if plan.Connections == nil {
		plan.Connections = []ConnectionRecord{}
	}
	if plan.Assumptions == nil {
		plan.Assumptions = []PlanNote{}
	}
	if plan.Clarifications == nil {
		plan.Clarifications = []PlanNote{}
	}
	if plan.KnownGaps == nil {
		plan.KnownGaps = []PlanNote{}
	}
	if plan.Issues == nil {
		plan.Issues = []reports.Issue{}
	}
	if plan.Artifacts == nil {
		plan.Artifacts = []reports.Artifact{}
	}
	return plan
}

func (plan PlanResult) MarshalJSON() ([]byte, error) {
	normalized := NormalizePlan(plan)
	return json.Marshal(planAlias(normalized))
}

func MarshalPlanJSON(plan PlanResult) ([]byte, error) {
	normalized := NormalizePlan(plan)
	data, err := json.MarshalIndent(planAlias(normalized), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func WriteArtifacts(plan PlanResult, opts ArtifactOptions) (PlanResult, []reports.Issue) {
	plan = NormalizePlan(plan)
	outputDir := strings.TrimSpace(opts.OutputDir)
	if outputDir == "" {
		return plan, nil
	}
	var issues []reports.Issue
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return plan, []reports.Issue{artifactIssue("output", err.Error())}
	}
	if data, err := MarshalPlanJSON(plan); err != nil {
		issues = append(issues, artifactIssue("intent-plan.json", err.Error()))
	} else if writeIssue := writeArtifactFile(filepath.Join(outputDir, "intent-plan.json"), data, opts.Overwrite); writeIssue != nil {
		issues = append(issues, *writeIssue)
	} else {
		plan.Artifacts = append(plan.Artifacts, reports.Artifact{Kind: reports.ArtifactPreview, Path: "intent-plan.json", Description: "intent planner report"})
	}
	if plan.GeneratedRequest != nil {
		data, err := json.MarshalIndent(plan.GeneratedRequest, "", "  ")
		if err != nil {
			issues = append(issues, artifactIssue("generated-request.json", err.Error()))
		} else {
			data = append(data, '\n')
			if writeIssue := writeArtifactFile(filepath.Join(outputDir, "generated-request.json"), data, opts.Overwrite); writeIssue != nil {
				issues = append(issues, *writeIssue)
			} else {
				plan.Artifacts = append(plan.Artifacts, reports.Artifact{Kind: reports.ArtifactPreview, Path: "generated-request.json", Description: "generated design workflow request"})
			}
		}
	}
	plan.Issues = append(plan.Issues, issues...)
	return NormalizePlan(plan), issues
}

func writeArtifactFile(path string, data []byte, overwrite bool) *reports.Issue {
	flags := os.O_WRONLY | os.O_CREATE
	if overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		issue := artifactIssue(filepath.Base(path), err.Error())
		return &issue
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		issue := artifactIssue(filepath.Base(path), writeErr.Error())
		return &issue
	}
	if closeErr != nil {
		issue := artifactIssue(filepath.Base(path), closeErr.Error())
		return &issue
	}
	return nil
}

func artifactIssue(path string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: path, Message: message}
}

func calculatePlanStatus(plan PlanResult) PlanStatus {
	if reports.HasBlockingIssue(plan.Issues) {
		return PlanStatusBlocked
	}
	if len(plan.Clarifications) > 0 {
		return PlanStatusNeedsClarification
	}
	if len(plan.KnownGaps) > 0 {
		return PlanStatusPartial
	}
	for _, issue := range plan.Issues {
		if issue.Severity == reports.SeverityWarning {
			return PlanStatusPartial
		}
	}
	return PlanStatusReady
}

func calculatePlanScore(plan PlanResult) int {
	switch calculatePlanStatus(plan) {
	case PlanStatusReady:
		return 100
	case PlanStatusPartial:
		return 75
	case PlanStatusNeedsClarification:
		return 50
	default:
		return 0
	}
}

func cloneRequirements(values []RequirementRecord) []RequirementRecord {
	out := append([]RequirementRecord(nil), values...)
	for i := range out {
		out[i].Evidence = append([]string(nil), out[i].Evidence...)
	}
	return out
}

func cloneSelectedBlocks(values []SelectedBlockRecord) []SelectedBlockRecord {
	out := append([]SelectedBlockRecord(nil), values...)
	for i := range out {
		out[i].RequirementIDs = append([]string(nil), out[i].RequirementIDs...)
		out[i].Params = cloneParams(out[i].Params)
		out[i].RequiredRoutes = append([]string(nil), out[i].RequiredRoutes...)
		out[i].KnownGaps = append([]string(nil), out[i].KnownGaps...)
	}
	return out
}

func cloneSelectedComponents(values []SelectedComponentRecord) []SelectedComponentRecord {
	out := append([]SelectedComponentRecord(nil), values...)
	for i := range out {
		out[i].RequiredRatings = append([]string(nil), out[i].RequiredRatings...)
	}
	return out
}

func cloneConnections(values []ConnectionRecord) []ConnectionRecord {
	out := append([]ConnectionRecord(nil), values...)
	for i := range out {
		out[i].RequirementIDs = append([]string(nil), out[i].RequirementIDs...)
	}
	return out
}

func cloneNotes(values []PlanNote) []PlanNote {
	return append([]PlanNote(nil), values...)
}

func compareRequirements(a, b RequirementRecord) int {
	if a.ID != b.ID {
		return strings.Compare(a.ID, b.ID)
	}
	return strings.Compare(a.Path, b.Path)
}

func compareSelectedBlocks(a, b SelectedBlockRecord) int {
	if a.InstanceID != b.InstanceID {
		return strings.Compare(a.InstanceID, b.InstanceID)
	}
	return strings.Compare(a.BlockID, b.BlockID)
}

func compareSelectedComponents(a, b SelectedComponentRecord) int {
	if a.RequirementID != b.RequirementID {
		return strings.Compare(a.RequirementID, b.RequirementID)
	}
	return strings.Compare(a.Family, b.Family)
}

func compareConnections(a, b ConnectionRecord) int {
	if a.From != b.From {
		return strings.Compare(a.From, b.From)
	}
	if a.To != b.To {
		return strings.Compare(a.To, b.To)
	}
	return strings.Compare(a.NetAlias, b.NetAlias)
}

func compareNotes(a, b PlanNote) int {
	if a.ID != b.ID {
		return strings.Compare(a.ID, b.ID)
	}
	return strings.Compare(a.Path, b.Path)
}

func compareArtifacts(a, b reports.Artifact) int {
	if a.Kind != b.Kind {
		return strings.Compare(string(a.Kind), string(b.Kind))
	}
	return strings.Compare(a.Path, b.Path)
}
