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
const SynthesisSchema = "kicadai.intent.synthesis.v1"

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
	Synthesis          SynthesisTrace            `json:"synthesis"`
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

type SynthesisTrace struct {
	Schema       string                 `json:"schema"`
	Status       PlanStatus             `json:"status"`
	Decisions    []SynthesisDecision    `json:"decisions,omitempty"`
	Evidence     []SynthesisEvidence    `json:"evidence,omitempty"`
	Constraints  []SynthesisConstraint  `json:"constraints,omitempty"`
	Calculations []SynthesisCalculation `json:"calculations,omitempty"`
	Gaps         []SynthesisGap         `json:"gaps,omitempty"`
}

type SynthesisDecision struct {
	ID             string   `json:"id"`
	Type           string   `json:"type"`
	Path           string   `json:"path,omitempty"`
	Selected       string   `json:"selected"`
	Rationale      string   `json:"rationale"`
	RequirementIDs []string `json:"requirement_ids,omitempty"`
	EvidenceIDs    []string `json:"evidence_ids,omitempty"`
	Confidence     string   `json:"confidence,omitempty"`
}

type SynthesisEvidence struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Path       string   `json:"path,omitempty"`
	Summary    string   `json:"summary"`
	Source     string   `json:"source,omitempty"`
	Confidence string   `json:"confidence,omitempty"`
	Refs       []string `json:"refs,omitempty"`
}

type SynthesisConstraint struct {
	ID            string `json:"id"`
	Path          string `json:"path,omitempty"`
	Kind          string `json:"kind"`
	Subject       string `json:"subject"`
	Operator      string `json:"operator,omitempty"`
	Value         string `json:"value,omitempty"`
	Source        string `json:"source,omitempty"`
	RequirementID string `json:"requirement_id,omitempty"`
}

type SynthesisCalculation struct {
	ID           string                  `json:"id"`
	Kind         string                  `json:"kind"`
	Path         string                  `json:"path,omitempty"`
	Inputs       map[string]string       `json:"inputs,omitempty"`
	Result       map[string]string       `json:"result,omitempty"`
	Formula      string                  `json:"formula,omitempty"`
	Assumptions  []string                `json:"assumptions,omitempty"`
	Confidence   string                  `json:"confidence,omitempty"`
	Status       string                  `json:"status,omitempty"`
	Applied      []AppliedValue          `json:"applied,omitempty"`
	Requirements []CalculatedRequirement `json:"requirements,omitempty"`
	Issues       []reports.Issue         `json:"issues,omitempty"`
}

type AppliedValue struct {
	Target string `json:"target"`
	Path   string `json:"path"`
	Value  string `json:"value"`
	Unit   string `json:"unit,omitempty"`
	Method string `json:"method,omitempty"`
}

type CalculatedRequirement struct {
	Subject  string `json:"subject"`
	Kind     string `json:"kind"`
	Operator string `json:"operator,omitempty"`
	Value    string `json:"value"`
	Unit     string `json:"unit,omitempty"`
	Source   string `json:"source,omitempty"`
}

type SynthesisGap struct {
	ID             string           `json:"id"`
	Category       string           `json:"category"`
	Path           string           `json:"path,omitempty"`
	Message        string           `json:"message"`
	Severity       reports.Severity `json:"severity,omitempty"`
	Suggestion     string           `json:"suggestion,omitempty"`
	RequirementIDs []string         `json:"requirement_ids,omitempty"`
	EvidenceIDs    []string         `json:"evidence_ids,omitempty"`
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
	plan.Synthesis = normalizeSynthesisTrace(plan.Synthesis, plan)
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
	plan.Status = calculatePlanStatus(plan)
	plan.Score = calculatePlanScore(plan)
	plan.Synthesis.Status = synthesisStatus(plan)
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

func normalizeSynthesisTrace(trace SynthesisTrace, plan PlanResult) SynthesisTrace {
	if strings.TrimSpace(trace.Schema) == "" {
		trace.Schema = SynthesisSchema
	}
	trace.Decisions = cloneSynthesisDecisions(trace.Decisions)
	trace.Evidence = cloneSynthesisEvidence(trace.Evidence)
	trace.Constraints = cloneSynthesisConstraints(trace.Constraints)
	trace.Calculations = cloneSynthesisCalculations(trace.Calculations)
	trace.Gaps = cloneSynthesisGaps(trace.Gaps)
	slices.SortFunc(trace.Decisions, compareSynthesisDecisions)
	slices.SortFunc(trace.Evidence, compareSynthesisEvidence)
	slices.SortFunc(trace.Constraints, compareSynthesisConstraints)
	slices.SortFunc(trace.Calculations, compareSynthesisCalculations)
	slices.SortFunc(trace.Gaps, compareSynthesisGaps)
	trace.Status = synthesisStatus(PlanResult{
		Status:         plan.Status,
		Issues:         plan.Issues,
		Clarifications: plan.Clarifications,
		KnownGaps:      plan.KnownGaps,
		Synthesis:      trace,
	})
	if trace.Decisions == nil {
		trace.Decisions = []SynthesisDecision{}
	}
	if trace.Evidence == nil {
		trace.Evidence = []SynthesisEvidence{}
	}
	if trace.Constraints == nil {
		trace.Constraints = []SynthesisConstraint{}
	}
	if trace.Calculations == nil {
		trace.Calculations = []SynthesisCalculation{}
	}
	if trace.Gaps == nil {
		trace.Gaps = []SynthesisGap{}
	}
	return trace
}

func synthesisStatus(plan PlanResult) PlanStatus {
	parent := calculatePlanStatus(plan)
	if parent == PlanStatusBlocked || parent == PlanStatusNeedsClarification {
		return parent
	}
	for _, gap := range plan.Synthesis.Gaps {
		if gap.Severity == reports.SeverityError || gap.Severity == reports.SeverityBlocked {
			return PlanStatusBlocked
		}
	}
	if len(plan.Synthesis.Gaps) > 0 || parent == PlanStatusPartial {
		return PlanStatusPartial
	}
	return PlanStatusReady
}

func cloneSynthesisDecisions(values []SynthesisDecision) []SynthesisDecision {
	out := append([]SynthesisDecision(nil), values...)
	for i := range out {
		out[i].RequirementIDs = append([]string(nil), out[i].RequirementIDs...)
		out[i].EvidenceIDs = append([]string(nil), out[i].EvidenceIDs...)
	}
	return out
}

func cloneSynthesisEvidence(values []SynthesisEvidence) []SynthesisEvidence {
	out := append([]SynthesisEvidence(nil), values...)
	for i := range out {
		out[i].Refs = append([]string(nil), out[i].Refs...)
	}
	return out
}

func cloneSynthesisConstraints(values []SynthesisConstraint) []SynthesisConstraint {
	return append([]SynthesisConstraint(nil), values...)
}

func cloneSynthesisCalculations(values []SynthesisCalculation) []SynthesisCalculation {
	out := append([]SynthesisCalculation(nil), values...)
	for i := range out {
		out[i].Inputs = cloneStringMap(out[i].Inputs)
		out[i].Result = cloneStringMap(out[i].Result)
		out[i].Assumptions = append([]string(nil), out[i].Assumptions...)
		out[i].Applied = append([]AppliedValue(nil), out[i].Applied...)
		out[i].Requirements = append([]CalculatedRequirement(nil), out[i].Requirements...)
		out[i].Issues = append([]reports.Issue(nil), out[i].Issues...)
		slices.SortFunc(out[i].Applied, compareAppliedValues)
		slices.SortFunc(out[i].Requirements, compareCalculatedRequirements)
		slices.SortFunc(out[i].Issues, compareIssues)
	}
	return out
}

func cloneSynthesisGaps(values []SynthesisGap) []SynthesisGap {
	out := append([]SynthesisGap(nil), values...)
	for i := range out {
		out[i].RequirementIDs = append([]string(nil), out[i].RequirementIDs...)
		out[i].EvidenceIDs = append([]string(nil), out[i].EvidenceIDs...)
	}
	return out
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

func compareSynthesisDecisions(a, b SynthesisDecision) int {
	if a.ID != b.ID {
		return strings.Compare(a.ID, b.ID)
	}
	if a.Type != b.Type {
		return strings.Compare(a.Type, b.Type)
	}
	return strings.Compare(a.Path, b.Path)
}

func compareSynthesisEvidence(a, b SynthesisEvidence) int {
	if a.ID != b.ID {
		return strings.Compare(a.ID, b.ID)
	}
	if a.Kind != b.Kind {
		return strings.Compare(a.Kind, b.Kind)
	}
	return strings.Compare(a.Path, b.Path)
}

func compareSynthesisConstraints(a, b SynthesisConstraint) int {
	if a.ID != b.ID {
		return strings.Compare(a.ID, b.ID)
	}
	if a.Kind != b.Kind {
		return strings.Compare(a.Kind, b.Kind)
	}
	return strings.Compare(a.Subject, b.Subject)
}

func compareSynthesisCalculations(a, b SynthesisCalculation) int {
	if a.ID != b.ID {
		return strings.Compare(a.ID, b.ID)
	}
	if a.Kind != b.Kind {
		return strings.Compare(a.Kind, b.Kind)
	}
	return strings.Compare(a.Path, b.Path)
}

func compareAppliedValues(a, b AppliedValue) int {
	if a.Target != b.Target {
		return strings.Compare(a.Target, b.Target)
	}
	if a.Path != b.Path {
		return strings.Compare(a.Path, b.Path)
	}
	return strings.Compare(a.Value, b.Value)
}

func compareCalculatedRequirements(a, b CalculatedRequirement) int {
	if a.Subject != b.Subject {
		return strings.Compare(a.Subject, b.Subject)
	}
	if a.Kind != b.Kind {
		return strings.Compare(a.Kind, b.Kind)
	}
	if a.Value != b.Value {
		return strings.Compare(a.Value, b.Value)
	}
	return strings.Compare(a.Unit, b.Unit)
}

func compareSynthesisGaps(a, b SynthesisGap) int {
	if a.ID != b.ID {
		return strings.Compare(a.ID, b.ID)
	}
	if a.Category != b.Category {
		return strings.Compare(a.Category, b.Category)
	}
	return strings.Compare(a.Path, b.Path)
}
