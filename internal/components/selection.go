package components

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"kicadai/internal/reports"
)

const (
	CodeComponentNotFound            reports.Code = "COMPONENT_NOT_FOUND"
	CodeComponentAmbiguous           reports.Code = "COMPONENT_AMBIGUOUS"
	CodeComponentUnsafe              reports.Code = "COMPONENT_UNSAFE_CONFIDENCE"
	CodeComponentRatingTooLow        reports.Code = "COMPONENT_RATING_TOO_LOW"
	CodeComponentRatingMissing       reports.Code = "COMPONENT_RATING_MISSING"
	CodeComponentFunctionMissing     reports.Code = "COMPONENT_FUNCTION_MISSING"
	CodeComponentVariantMissing      reports.Code = "COMPONENT_VARIANT_MISSING"
	CodeComponentConcreteRequired    reports.Code = "COMPONENT_CONCRETE_REQUIRED"
	CodeComponentCompanionMissing    reports.Code = "COMPONENT_COMPANION_MISSING"
	CodeComponentReviewRequired      reports.Code = "COMPONENT_REVIEW_REQUIRED"
	CodeComponentLifecycleBlocked    reports.Code = "COMPONENT_LIFECYCLE_BLOCKED"
	CodeComponentLifecycleStale      reports.Code = "COMPONENT_LIFECYCLE_STALE"
	CodeComponentAvailabilityBlocked reports.Code = "COMPONENT_AVAILABILITY_BLOCKED"
	CodeComponentAvailabilityStale   reports.Code = "COMPONENT_AVAILABILITY_STALE"
	CodeComponentSourceMissing       reports.Code = "COMPONENT_SOURCE_MISSING"
)

const (
	reviewStatusProven        = "proven"
	reviewStatusNotApplicable = "not_applicable"
	reviewStatusUnknown       = "unknown"
	componentFamilyLED        = "led"
)

var capacitorEvidenceReviewChecks = []struct {
	path  string
	label string
	get   func(*CapacitorEvidence) string
}{
	{path: "dc_bias_review", label: "DC-bias", get: func(evidence *CapacitorEvidence) string { return evidence.DCBiasReview }},
	{path: "effective_capacitance_review", label: "effective-capacitance", get: func(evidence *CapacitorEvidence) string { return evidence.EffectiveCapacitanceReview }},
	{path: "esr_review", label: "ESR", get: func(evidence *CapacitorEvidence) string { return evidence.ESRReview }},
}

var opAmpEvidenceReviewChecks = []struct {
	path  string
	label string
	get   func(*OpAmpEvidence) string
}{
	{path: "output_drive_status", label: "output-drive", get: func(evidence *OpAmpEvidence) string { return evidence.OutputDriveStatus }},
	{path: "load_compatibility_status", label: "load-compatibility", get: func(evidence *OpAmpEvidence) string { return evidence.LoadCompatibilityStatus }},
	{path: "gain_bandwidth_status", label: "gain-bandwidth", get: func(evidence *OpAmpEvidence) string { return evidence.GainBandwidthStatus }},
	{path: "stability_status", label: "stability", get: func(evidence *OpAmpEvidence) string { return evidence.StabilityStatus }},
	{path: "input_common_mode_status", label: "input-common-mode", get: func(evidence *OpAmpEvidence) string { return evidence.InputCommonModeStatus }},
}

type Query struct {
	Text              string          `json:"text,omitempty"`
	Family            string          `json:"family,omitempty"`
	Package           string          `json:"package,omitempty"`
	ValueKind         string          `json:"value_kind,omitempty"`
	Value             string          `json:"value,omitempty"`
	MinVoltageV       float64         `json:"min_voltage_v,omitempty"`
	MinimumConfidence ConfidenceLevel `json:"minimum_confidence,omitempty"`
	Limit             int             `json:"limit,omitempty"`
}

type SelectionRequest struct {
	Query             Query             `json:"query"`
	Acceptance        AcceptanceLevel   `json:"acceptance,omitempty"`
	AllowAlternatives bool              `json:"allow_alternatives,omitempty"`
	RequiredRatings   []RequiredRating  `json:"required_ratings,omitempty"`
	RequiredFunctions []string          `json:"required_functions,omitempty"`
	RequireConcrete   bool              `json:"require_concrete,omitempty"`
	RequireCompanions bool              `json:"require_companions,omitempty"`
	Procurement       ProcurementPolicy `json:"procurement_policy,omitempty"`
	Sources           *SourceCollection `json:"-"`
}

type RequiredRating struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Unit  string `json:"unit"`
}

type Candidate struct {
	ComponentID      string          `json:"component_id"`
	VariantID        string          `json:"variant_id"`
	Family           string          `json:"family"`
	Name             string          `json:"name"`
	FootprintID      string          `json:"footprint_id,omitempty"`
	Confidence       ConfidenceLevel `json:"confidence"`
	Score            int             `json:"score"`
	Generic          bool            `json:"generic,omitempty"`
	EquivalenceGroup string          `json:"equivalence_group,omitempty"`
	EquivalenceRole  EquivalenceRole `json:"equivalence_role,omitempty"`
	Reasons          []string        `json:"reasons,omitempty"`
	valueSpecificity int             `json:"-"`
}

type Selection struct {
	Candidate   Candidate            `json:"candidate"`
	Component   ComponentRecord      `json:"component"`
	Variant     PackageVariant       `json:"variant"`
	Procurement *ProcurementEvidence `json:"procurement,omitempty"`
	Warnings    []reports.Issue      `json:"warnings,omitempty"`
	Rejected    []CandidateRejection `json:"rejected,omitempty"`
}

type ProcurementPolicy struct {
	RequireLifecycle       bool              `json:"require_lifecycle,omitempty"`
	RequireAvailability    bool              `json:"require_availability,omitempty"`
	AllowLifecycle         []LifecycleStatus `json:"allow_lifecycle,omitempty"`
	WarnLifecycle          []LifecycleStatus `json:"warn_lifecycle,omitempty"`
	BlockLifecycle         []LifecycleStatus `json:"block_lifecycle,omitempty"`
	MaxLifecycleAgeDays    int               `json:"max_lifecycle_age_days,omitempty"`
	MaxAvailabilityAgeDays int               `json:"max_availability_age_days,omitempty"`
	AllowUnknownLifecycle  bool              `json:"allow_unknown_lifecycle,omitempty"`
	Now                    *time.Time        `json:"-"`
}

type ProcurementEvidence struct {
	Manufacturer           string             `json:"manufacturer,omitempty"`
	MPN                    string             `json:"mpn,omitempty"`
	SourceID               string             `json:"source_id,omitempty"`
	LifecycleStatus        LifecycleStatus    `json:"lifecycle_status,omitempty"`
	LifecycleSourceDate    string             `json:"lifecycle_source_date,omitempty"`
	LifecycleFresh         *bool              `json:"lifecycle_fresh,omitempty"`
	AvailabilityStatus     AvailabilityStatus `json:"availability_status,omitempty"`
	AvailabilitySourceDate string             `json:"availability_source_date,omitempty"`
	AvailabilityFresh      *bool              `json:"availability_fresh,omitempty"`
	Outcome                string             `json:"outcome,omitempty"`
}

type CandidateRejection struct {
	Candidate Candidate       `json:"candidate"`
	Issues    []reports.Issue `json:"issues"`
}

type ResolvedComponent struct {
	Component ComponentRecord `json:"component"`
	Variant   PackageVariant  `json:"variant"`
	Symbol    SymbolBinding   `json:"symbol"`
}

// ValidateResolvedComponent applies the same requirement and acceptance gates
// used during catalog selection to an explicitly resolved component binding.
func ValidateResolvedComponent(resolved ResolvedComponent, request SelectionRequest) reports.Result {
	confidence := weakerConfidence(resolved.Component.Verification.Confidence, resolved.Variant.Verification.Confidence)
	candidate := Candidate{
		ComponentID: resolved.Component.ID,
		VariantID:   resolved.Variant.ID,
		Family:      resolved.Component.Family,
		Name:        resolved.Component.Name,
		FootprintID: resolved.Variant.FootprintID,
		Confidence:  confidence,
		Generic:     resolved.Component.Generic,
	}
	issues := selectionCandidateIssues(resolved.Component, candidate, request)
	return reports.ResultWithIssues("component resolve validate", resolved, issues, nil)
}

func Find(ctx context.Context, catalog *Catalog, query Query) ([]Candidate, reports.Result) {
	if err := ctx.Err(); err != nil {
		issue := reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityBlocked, Message: err.Error()}
		return nil, reports.ErrorResult("component find", issue)
	}
	if catalog == nil {
		issue := NewIssue(reports.CodeInvalidArgument, reports.SeverityBlocked, "catalog", "component catalog is nil")
		return nil, reports.ErrorResult("component find", issue)
	}
	query.Text = strings.ToLower(query.Text)
	query.Package = strings.ToLower(query.Package)
	normalizedQueryText := normalizeComponentSearchText(query.Text)
	var candidates []Candidate
	var issues []reports.Issue
	catalog.mu.RLock()
	defer catalog.mu.RUnlock()
	for _, record := range catalog.Records {
		if err := ctx.Err(); err != nil {
			issue := reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityBlocked, Message: err.Error()}
			return nil, reports.ErrorResult("component find", issue)
		}
		if !recordMatchesQuery(record, query, normalizedQueryText) {
			continue
		}
		group, role := candidateEquivalence(record)
		for _, variant := range record.Packages {
			if !variantMatchesQuery(variant, query) {
				continue
			}
			confidence := weakerConfidence(record.Verification.Confidence, variant.Verification.Confidence)
			if query.MinimumConfidence != "" && confidenceRank(confidence) < confidenceRank(query.MinimumConfidence) {
				continue
			}
			score := scoreCandidate(record, variant, query)
			candidates = append(candidates, Candidate{
				ComponentID:      record.ID,
				VariantID:        variant.ID,
				Family:           record.Family,
				Name:             record.Name,
				FootprintID:      variant.FootprintID,
				Confidence:       confidence,
				Score:            score,
				Generic:          record.Generic,
				EquivalenceGroup: group,
				EquivalenceRole:  role,
				Reasons:          candidateReasons(record, variant, query),
				valueSpecificity: recordValueSpecificity(record, query),
			})
		}
	}
	sortCandidates(candidates)
	if query.Limit > 0 && len(candidates) > query.Limit {
		candidates = candidates[:query.Limit]
	}
	if len(candidates) == 0 {
		issues = append(issues, NewIssue(CodeComponentNotFound, reports.SeverityWarning, "component.find", "no components matched query"))
	}
	return candidates, reports.ResultWithIssues("component find", map[string]any{"candidates": candidates}, issues, nil)
}

func Select(ctx context.Context, catalog *Catalog, request SelectionRequest) (Selection, reports.Result) {
	if request.Acceptance == "" {
		request.Acceptance = AcceptanceDraft
	}
	candidates, findResult := Find(ctx, catalog, request.Query)
	if !findResult.OK || len(candidates) == 0 {
		issues := append([]reports.Issue(nil), findResult.Issues...)
		for index := range issues {
			if issues[index].Code == CodeComponentNotFound {
				issues[index].Severity = reports.SeverityBlocked
			}
		}
		return Selection{}, reports.ResultWithIssues("component select", map[string]any{"candidates": candidates}, issues, nil)
	}
	var issues []reports.Issue
	filtered := make([]acceptedCandidate, 0, len(candidates))
	rejected := make([]CandidateRejection, 0, len(candidates))
	for _, candidate := range candidates {
		record, _, ok := findRecordVariant(catalog, candidate.ComponentID, candidate.VariantID)
		if !ok {
			continue
		}
		candidateIssues := selectionCandidateIssues(record, candidate, request)
		if reports.HasBlockingIssue(candidateIssues) {
			rejected = append(rejected, CandidateRejection{Candidate: candidate, Issues: candidateIssues})
			continue
		}
		filtered = append(filtered, acceptedCandidate{Candidate: candidate, Warnings: candidateIssues})
	}
	sortAcceptedCandidates(filtered, request.Acceptance)
	if len(filtered) == 0 {
		issues = append(issues, dedupeRejectedIssues(rejected)...)
		return Selection{}, reports.ResultWithIssues("component select", map[string]any{"candidates": candidates}, issues, nil)
	}
	if ambiguous, pair := ambiguousTopTie(filtered); ambiguous && !request.AllowAlternatives {
		issues = append(issues, NewIssue(CodeComponentAmbiguous, reports.SeverityBlocked, "component.select", "multiple components matched with equal score and confidence"))
		return Selection{}, reports.ResultWithIssues("component select", map[string]any{"candidates": pair}, issues, nil)
	}
	selected := filtered[0]
	record, variant, _ := findRecordVariant(catalog, selected.Candidate.ComponentID, selected.Candidate.VariantID)
	selection := Selection{Candidate: selected.Candidate, Component: record, Variant: variant, Rejected: rejected}
	procurement, procurementWarnings := evaluateProcurement(record, request, false)
	if procurement != nil {
		selection.Procurement = procurement
	}
	selection.Warnings = append(selection.Warnings, selected.Warnings...)
	selection.Warnings = append(selection.Warnings, procurementWarnings...)
	if selected.Candidate.Confidence == ConfidencePlaceholder {
		selection.Warnings = append(selection.Warnings, NewIssue(CodeComponentUnsafe, reports.SeverityWarning, "component."+selected.Candidate.ComponentID, "placeholder component selected for draft output"))
	}
	return selection, reports.ResultWithIssues("component select", selection, append(issues, selection.Warnings...), nil)
}

type acceptedCandidate struct {
	Candidate Candidate
	Warnings  []reports.Issue
}

func sortAcceptedCandidates(candidates []acceptedCandidate, acceptance AcceptanceLevel) {
	strongAcceptance := CompareAcceptance(acceptance, AcceptanceConnectivity) >= 0
	sort.Slice(candidates, func(i, j int) bool {
		return acceptedCandidateLess(candidates[i], candidates[j], strongAcceptance)
	})
}

func acceptedCandidateLess(leftAccepted acceptedCandidate, rightAccepted acceptedCandidate, strongAcceptance bool) bool {
	left := leftAccepted.Candidate
	right := rightAccepted.Candidate
	if left.Score != right.Score {
		return left.Score > right.Score
	}
	if left.Confidence != right.Confidence {
		return confidenceRank(left.Confidence) > confidenceRank(right.Confidence)
	}
	if left.valueSpecificity != right.valueSpecificity {
		return left.valueSpecificity > right.valueSpecificity
	}
	if left.Generic != right.Generic {
		if strongAcceptance {
			return !left.Generic
		}
		return left.Generic
	}
	if equivalentCandidate(left, right) && left.EquivalenceRole != right.EquivalenceRole {
		return equivalenceRoleRank(left.EquivalenceRole) > equivalenceRoleRank(right.EquivalenceRole)
	}
	if left.ComponentID != right.ComponentID {
		return left.ComponentID < right.ComponentID
	}
	return left.VariantID < right.VariantID
}

func ambiguousTopTie(candidates []acceptedCandidate) (bool, []Candidate) {
	if len(candidates) < 2 {
		return false, nil
	}
	selected := candidates[0].Candidate
	for _, candidate := range candidates[1:] {
		current := candidate.Candidate
		if current.Score != selected.Score || current.Confidence != selected.Confidence || current.valueSpecificity != selected.valueSpecificity {
			break
		}
		if equivalentCandidate(selected, current) || hasExplicitSelectionPreference(selected, current) {
			continue
		}
		return true, []Candidate{selected, current}
	}
	return false, nil
}

func equivalentCandidate(left Candidate, right Candidate) bool {
	return left.EquivalenceGroup != "" && left.EquivalenceGroup == right.EquivalenceGroup
}

func hasExplicitSelectionPreference(left Candidate, right Candidate) bool {
	return left.Generic != right.Generic
}

func candidateEquivalence(record ComponentRecord) (string, EquivalenceRole) {
	if record.Equivalence == nil {
		return "", ""
	}
	return normalizeMetadata(record.Equivalence.Group), record.Equivalence.Role
}

func equivalenceRoleRank(role EquivalenceRole) int {
	switch role {
	case EquivalencePreferred:
		return 3
	case EquivalenceAlternate:
		return 2
	case EquivalenceFallback:
		return 1
	default:
		return 0
	}
}

func firstCandidates(candidates []acceptedCandidate, limit int) []Candidate {
	if limit < 0 {
		limit = 0
	}
	if len(candidates) < limit {
		limit = len(candidates)
	}
	out := make([]Candidate, 0, limit)
	for _, candidate := range candidates[:limit] {
		out = append(out, candidate.Candidate)
	}
	return out
}

func dedupeRejectedIssues(rejected []CandidateRejection) []reports.Issue {
	var issues []reports.Issue
	seen := map[string]struct{}{}
	for _, rejection := range rejected {
		for _, issue := range rejection.Issues {
			key := string(issue.Code) + "\x00" + issue.Path + "\x00" + issue.Message
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			issues = append(issues, issue)
		}
	}
	sortIssues(issues)
	return issues
}

func selectionCandidateIssues(record ComponentRecord, candidate Candidate, request SelectionRequest) []reports.Issue {
	var issues []reports.Issue
	issues = append(issues, requiredRatingIssues(record, request.RequiredRatings)...)
	issues = append(issues, requiredFunctionIssues(record, request.RequiredFunctions)...)
	if request.RequireConcrete && record.Generic {
		issues = append(issues, NewIssue(CodeComponentConcreteRequired, reports.SeverityBlocked, "component."+record.ID, "concrete component record is required"))
	}
	if request.RequireCompanions && !recordHasRequiredCompanions(record) {
		issues = append(issues, NewIssue(CodeComponentCompanionMissing, reports.SeverityBlocked, "component."+record.ID+".companions", "component requires explicit companion component metadata"))
	}
	if CompareAcceptance(request.Acceptance, AcceptanceFabricationCandidate) >= 0 {
		issues = append(issues, fabricationCandidateReviewIssues(record)...)
	} else if CompareAcceptance(request.Acceptance, AcceptanceConnectivity) >= 0 {
		issues = append(issues, structuredEvidenceReviewIssues(record, reports.SeverityWarning)...)
	}
	_, procurementIssues := evaluateProcurement(record, request, true)
	issues = append(issues, procurementIssues...)
	if !candidateAllowedForAcceptance(record, candidate, request.Acceptance) {
		issues = append(issues, NewIssue(CodeComponentUnsafe, reports.SeverityBlocked, "component."+candidate.ComponentID, fmt.Sprintf("component confidence %s is not allowed for %s acceptance", candidate.Confidence, request.Acceptance)))
	}
	return issues
}

func evaluateProcurement(record ComponentRecord, request SelectionRequest, candidateGate bool) (*ProcurementEvidence, []reports.Issue) {
	policy := defaultProcurementPolicy(request.Acceptance, request.Procurement)
	if request.Sources == nil && !policy.RequireLifecycle && !policy.RequireAvailability {
		return nil, nil
	}
	now := time.Now().UTC()
	if policy.Now != nil {
		now = policy.Now.UTC()
	}
	evidence := &ProcurementEvidence{Manufacturer: record.Manufacturer, MPN: record.MPN, Outcome: "not_required"}
	if strings.TrimSpace(record.Manufacturer) == "" || strings.TrimSpace(record.MPN) == "" {
		if request.Acceptance == AcceptanceFabricationCandidate || policy.RequireLifecycle || policy.RequireAvailability {
			evidence.Outcome = "blocked"
			return evidence, []reports.Issue{NewIssue(CodeComponentSourceMissing, reports.SeverityBlocked, "component."+record.ID+".source", "component source evidence requires manufacturer and MPN")}
		}
		if request.Acceptance == AcceptanceConnectivity || request.Acceptance == AcceptanceERCDRC {
			evidence.Outcome = "warning"
			return evidence, []reports.Issue{NewIssue(CodeComponentSourceMissing, reports.SeverityWarning, "component."+record.ID+".source", "component source evidence requires manufacturer and MPN")}
		}
		return evidence, nil
	}
	source, found := SourceRecord{}, false
	if request.Sources != nil {
		source, found = request.Sources.Find(record.Manufacturer, record.MPN)
	}
	if !found {
		if policy.RequireLifecycle || (request.Sources != nil && request.Acceptance == AcceptanceFabricationCandidate) {
			evidence.Outcome = "blocked"
			return evidence, []reports.Issue{NewIssue(CodeComponentSourceMissing, reports.SeverityBlocked, "component."+record.ID+".source", "fresh lifecycle source evidence is required")}
		}
		if request.Acceptance == AcceptanceConnectivity || request.Acceptance == AcceptanceERCDRC {
			evidence.Outcome = "warning"
			return evidence, []reports.Issue{NewIssue(CodeComponentSourceMissing, reports.SeverityWarning, "component."+record.ID+".source", "component lifecycle source evidence is missing")}
		}
		return evidence, nil
	}
	evidence.SourceID = source.SourceID
	var issues []reports.Issue
	if source.Lifecycle != nil {
		evidence.LifecycleStatus = source.Lifecycle.Status
		evidence.LifecycleSourceDate = source.Lifecycle.SourceDate
		fresh := evidenceFresh(source.Lifecycle.SourceDate, now, policy.MaxLifecycleAgeDays)
		evidence.LifecycleFresh = &fresh
		issues = append(issues, lifecyclePolicyIssues(record.ID, source.Lifecycle.Status, fresh, policy, request.Acceptance)...)
	} else if policy.RequireLifecycle || request.Acceptance == AcceptanceFabricationCandidate {
		issues = append(issues, NewIssue(CodeComponentSourceMissing, reports.SeverityBlocked, "component."+record.ID+".lifecycle", "lifecycle source evidence is required"))
	}
	if source.Availability != nil {
		evidence.AvailabilityStatus = source.Availability.Status
		evidence.AvailabilitySourceDate = source.Availability.SourceDate
		fresh := evidenceFresh(source.Availability.SourceDate, now, policy.MaxAvailabilityAgeDays)
		evidence.AvailabilityFresh = &fresh
		issues = append(issues, availabilityPolicyIssues(record.ID, source.Availability.Status, fresh, policy)...)
	} else if policy.RequireAvailability {
		issues = append(issues, NewIssue(CodeComponentSourceMissing, reports.SeverityBlocked, "component."+record.ID+".availability", "availability source evidence is required"))
	}
	if len(issues) == 0 {
		evidence.Outcome = "accepted"
	} else if reports.HasBlockingIssue(issues) {
		evidence.Outcome = "blocked"
	} else {
		evidence.Outcome = "warning"
	}
	if candidateGate {
		return evidence, blockingIssues(issues)
	}
	return evidence, nonBlockingIssues(issues)
}

func defaultProcurementPolicy(acceptance AcceptanceLevel, policy ProcurementPolicy) ProcurementPolicy {
	if policy.MaxLifecycleAgeDays == 0 {
		policy.MaxLifecycleAgeDays = 730
	}
	if policy.MaxAvailabilityAgeDays == 0 {
		policy.MaxAvailabilityAgeDays = 30
	}
	if len(policy.BlockLifecycle) == 0 {
		policy.BlockLifecycle = []LifecycleStatus{LifecycleEOL, LifecycleObsolete}
	}
	if len(policy.WarnLifecycle) == 0 {
		policy.WarnLifecycle = []LifecycleStatus{LifecycleNRND, LifecycleUnknown}
	}
	if len(policy.AllowLifecycle) == 0 {
		policy.AllowLifecycle = []LifecycleStatus{LifecycleActive, LifecycleMature}
	}
	return policy
}

func lifecyclePolicyIssues(componentID string, status LifecycleStatus, fresh bool, policy ProcurementPolicy, acceptance AcceptanceLevel) []reports.Issue {
	path := "component." + componentID + ".lifecycle"
	if !fresh {
		severity := reports.SeverityWarning
		if acceptance == AcceptanceFabricationCandidate || policy.RequireLifecycle {
			severity = reports.SeverityBlocked
		}
		return []reports.Issue{NewIssue(CodeComponentLifecycleStale, severity, path, "component lifecycle source evidence is stale")}
	}
	if containsLifecycle(policy.BlockLifecycle, status) {
		return []reports.Issue{NewIssue(CodeComponentLifecycleBlocked, reports.SeverityBlocked, path, "component lifecycle status is blocked: "+string(status))}
	}
	if (status == LifecycleNRND || status == LifecycleUnknown) && acceptance == AcceptanceFabricationCandidate && !policy.AllowUnknownLifecycle {
		return []reports.Issue{NewIssue(CodeComponentLifecycleBlocked, reports.SeverityBlocked, path, "component lifecycle status is not allowed for fabrication-candidate: "+string(status))}
	}
	if containsLifecycle(policy.WarnLifecycle, status) {
		return []reports.Issue{NewIssue(CodeComponentLifecycleBlocked, reports.SeverityWarning, path, "component lifecycle status requires review: "+string(status))}
	}
	return nil
}

func availabilityPolicyIssues(componentID string, status AvailabilityStatus, fresh bool, policy ProcurementPolicy) []reports.Issue {
	path := "component." + componentID + ".availability"
	if !fresh {
		severity := reports.SeverityWarning
		if policy.RequireAvailability {
			severity = reports.SeverityBlocked
		}
		return []reports.Issue{NewIssue(CodeComponentAvailabilityStale, severity, path, "component availability source evidence is stale")}
	}
	if status == AvailabilityUnavailable && policy.RequireAvailability {
		return []reports.Issue{NewIssue(CodeComponentAvailabilityBlocked, reports.SeverityBlocked, path, "component availability is unavailable")}
	}
	return nil
}

func evidenceFresh(sourceDate string, now time.Time, maxAgeDays int) bool {
	parsed, err := parseSourceDate(sourceDate)
	if err != nil {
		return false
	}
	if maxAgeDays <= 0 {
		return true
	}
	return !parsed.Before(now.AddDate(0, 0, -maxAgeDays))
}

func containsLifecycle(values []LifecycleStatus, status LifecycleStatus) bool {
	for _, value := range values {
		if value == status {
			return true
		}
	}
	return false
}

func nonBlockingIssues(issues []reports.Issue) []reports.Issue {
	var out []reports.Issue
	for _, issue := range issues {
		if issue.Severity != reports.SeverityBlocked {
			out = append(out, issue)
		}
	}
	return out
}

func blockingIssues(issues []reports.Issue) []reports.Issue {
	var out []reports.Issue
	for _, issue := range issues {
		if issue.Severity == reports.SeverityBlocked {
			out = append(out, issue)
		}
	}
	return out
}

func fabricationCandidateReviewIssues(record ComponentRecord) []reports.Issue {
	var issues []reports.Issue
	for _, rule := range record.DeratingRules {
		switch rule.Kind {
		case "thermal", "capacitor_stability":
			message := "component requires unresolved " + strings.ReplaceAll(rule.Kind, "_", " ") + " evidence before fabrication-candidate selection"
			issues = append(issues, NewIssue(CodeComponentReviewRequired, reports.SeverityBlocked, "component."+record.ID+".derating_rules."+rule.Kind, message))
		}
	}
	issues = append(issues, structuredEvidenceReviewIssues(record, reports.SeverityBlocked)...)
	return issues
}

func structuredEvidenceReviewIssues(record ComponentRecord, severity reports.Severity) []reports.Issue {
	var issues []reports.Issue
	basePath := "component." + record.ID
	if record.Regulator != nil {
		if record.Regulator.ThermalReview != reviewStatusProven && record.Regulator.ThermalReview != reviewStatusNotApplicable {
			status := reviewStatusUnknown
			if record.Regulator.ThermalReview != "" {
				status = record.Regulator.ThermalReview
			}
			issues = append(issues, NewIssue(CodeComponentReviewRequired, severity, basePath+".regulator_evidence.thermal_review", "regulator thermal evidence is not proven: "+status))
		}
		if stability := record.Regulator.OutputCapacitor; stability != nil {
			if stability.FabricationCandidateBlocks || (stability.ProofStatus != reviewStatusProven && stability.ProofStatus != reviewStatusNotApplicable) {
				status := reviewStatusUnknown
				if stability.ProofStatus != "" {
					status = stability.ProofStatus
				}
				message := "regulator output-capacitor stability evidence is not proven"
				switch stability.Kind {
				case "esr_window_required":
					message = "regulator output capacitor requires ESR-window stability proof: " + status
				case "ceramic_stable":
					message = "regulator is ceramic-stable, but selected capacitor derating/effective-capacitance proof is still required: " + status
				case "datasheet_specific":
					message = "regulator output capacitor requires datasheet-specific stability proof: " + status
				default:
					message += ": " + status
				}
				issues = append(issues, NewIssue(CodeComponentReviewRequired, severity, basePath+".regulator_evidence.output_capacitor", message))
			}
		}
	}
	if record.Capacitor != nil {
		if record.Capacitor.FabricationCandidateBlocks {
			issues = append(issues, NewIssue(CodeComponentReviewRequired, severity, basePath+".capacitor_evidence.fabrication_candidate_blocks", "capacitor evidence blocks fabrication-candidate use until review is complete"))
		}
		for _, review := range capacitorEvidenceReviewChecks {
			value := review.get(record.Capacitor)
			if value == reviewStatusProven || value == reviewStatusNotApplicable {
				continue
			}
			if value == "" {
				value = reviewStatusUnknown
			}
			issues = append(issues, NewIssue(CodeComponentReviewRequired, severity, basePath+".capacitor_evidence."+review.path, "capacitor "+review.label+" evidence is not proven: "+value))
		}
	}
	if record.OpAmp != nil {
		if record.OpAmp.FabricationCandidateBlocks {
			issues = append(issues, NewIssue(CodeComponentReviewRequired, severity, basePath+".opamp_evidence.fabrication_candidate_blocks", "op-amp evidence blocks fabrication-candidate use until review is complete"))
		}
		for _, review := range opAmpEvidenceReviewChecks {
			value := review.get(record.OpAmp)
			if value == reviewStatusProven || value == reviewStatusNotApplicable {
				continue
			}
			if value == "" {
				value = reviewStatusUnknown
			}
			issues = append(issues, NewIssue(CodeComponentReviewRequired, severity, basePath+".opamp_evidence."+review.path, "op-amp "+review.label+" evidence is not proven: "+value))
		}
	}
	if record.AmplifierOutput != nil {
		if record.AmplifierOutput.FabricationCandidateBlocks {
			issues = append(issues, NewIssue(CodeComponentReviewRequired, severity, basePath+".amplifier_output_evidence.fabrication_candidate_blocks", "amplifier output evidence blocks fabrication-candidate use until review is complete"))
		}
		for _, review := range []struct {
			path  string
			label string
			value string
		}{
			{path: "voltage_rating_status", label: "voltage-rating", value: record.AmplifierOutput.VoltageRatingStatus},
			{path: "current_rating_status", label: "current-rating", value: record.AmplifierOutput.CurrentRatingStatus},
			{path: "power_dissipation_status", label: "power-dissipation", value: record.AmplifierOutput.PowerDissipationStatus},
			{path: "thermal_review", label: "thermal", value: record.AmplifierOutput.ThermalReview},
			{path: "safe_operating_area_status", label: "SOA", value: record.AmplifierOutput.SafeOperatingAreaStatus},
		} {
			value := review.value
			if value == reviewStatusProven || value == reviewStatusNotApplicable {
				continue
			}
			if value == "" {
				value = reviewStatusUnknown
			}
			issues = append(issues, NewIssue(CodeComponentReviewRequired, severity, basePath+".amplifier_output_evidence."+review.path, "amplifier output "+review.label+" evidence is not proven: "+value))
		}
	}
	return issues
}

func ResolveBinding(ctx context.Context, catalog *Catalog, id string, variantID string) (ResolvedComponent, reports.Result) {
	if err := ctx.Err(); err != nil {
		issue := reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityBlocked, Message: err.Error()}
		return ResolvedComponent{}, reports.ErrorResult("component resolve", issue)
	}
	record, variant, ok := findRecordVariant(catalog, id, variantID)
	if !ok {
		issue := NewIssue(CodeComponentVariantMissing, reports.SeverityBlocked, "component."+id, "component or variant not found")
		return ResolvedComponent{}, reports.ErrorResult("component resolve", issue)
	}
	if len(record.Symbols) == 0 {
		issue := NewIssue(CodeMissingSymbolBinding, reports.SeverityBlocked, "component."+id+".symbols", "component has no symbol binding")
		return ResolvedComponent{}, reports.ErrorResult("component resolve", issue)
	}
	resolved := ResolvedComponent{Component: record, Variant: variant, Symbol: record.Symbols[0]}
	return resolved, reports.OKResult("component resolve", resolved, nil)
}

func recordMatchesQuery(record ComponentRecord, query Query, normalizedQueryText string) bool {
	if query.Family != "" && record.Family != query.Family {
		return false
	}
	if query.Text != "" {
		text := record.SearchText
		if text == "" {
			text = strings.ToLower(record.ID + " " + record.Name + " " + record.Description + " " + strings.Join(record.Tags, " "))
		}
		if !strings.Contains(text, query.Text) && !strings.Contains(normalizeComponentSearchText(text), normalizedQueryText) {
			return false
		}
	}
	if query.ValueKind != "" && !recordHasValue(record, query.ValueKind, query.Value) {
		return false
	}
	return true
}

func normalizeComponentSearchText(value string) string {
	fields := strings.FieldsFunc(strings.ToLower(value), func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character)
	})
	return strings.Join(fields, " ")
}

func variantMatchesQuery(variant PackageVariant, query Query) bool {
	if query.Package == "" {
		return true
	}
	searchText := variant.SearchText
	if searchText == "" {
		searchText = strings.ToLower(variant.ID + " " + variant.Name + " " + variant.PackageType + " " + variant.FootprintID)
	}
	return strings.Contains(searchText, query.Package)
}

func recordHasValue(record ComponentRecord, kind string, value string) bool {
	for _, constraint := range record.Values {
		if constraint.Kind != kind {
			continue
		}
		if value == "" {
			return true
		}
		want, ok := parseValueWithUnit(value, constraint.Unit)
		if !ok {
			return false
		}
		if constraint.Typ != "" {
			got, ok := parseValueWithUnit(constraint.Typ, constraint.Unit)
			return ok && got == want
		}
		min, minOK := parseValueWithUnit(constraint.Min, constraint.Unit)
		max, maxOK := parseValueWithUnit(constraint.Max, constraint.Unit)
		return (!minOK || want >= min) && (!maxOK || want <= max)
	}
	return false
}

func recordValueSpecificity(record ComponentRecord, query Query) int {
	if query.ValueKind == "" || query.Value == "" {
		return 0
	}
	for _, constraint := range record.Values {
		if constraint.Kind != query.ValueKind {
			continue
		}
		if constraint.Typ != "" {
			return 2
		}
		return 1
	}
	return 0
}

func scoreCandidate(record ComponentRecord, variant PackageVariant, query Query) int {
	score := confidenceRank(weakerConfidence(record.Verification.Confidence, variant.Verification.Confidence)) * 100
	if query.Family != "" && record.Family == query.Family {
		score += 20
	}
	if query.Package != "" && (strings.EqualFold(variant.ID, query.Package) || strings.EqualFold(variant.PackageType, query.Package)) {
		score += 10
	}
	if query.ValueKind != "" {
		score += 5
	}
	return score
}

func candidateReasons(record ComponentRecord, variant PackageVariant, query Query) []string {
	var reasons []string
	if query.Family != "" && record.Family == query.Family {
		reasons = append(reasons, "family")
	}
	if query.Package != "" && variantMatchesQuery(variant, query) {
		reasons = append(reasons, "package")
	}
	if query.ValueKind != "" {
		reasons = append(reasons, "value")
	}
	return reasons
}

func sortCandidates(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].Confidence != candidates[j].Confidence {
			return confidenceRank(candidates[i].Confidence) > confidenceRank(candidates[j].Confidence)
		}
		if candidates[i].valueSpecificity != candidates[j].valueSpecificity {
			return candidates[i].valueSpecificity > candidates[j].valueSpecificity
		}
		if candidates[i].ComponentID != candidates[j].ComponentID {
			return candidates[i].ComponentID < candidates[j].ComponentID
		}
		return candidates[i].VariantID < candidates[j].VariantID
	})
}

func findRecordVariant(catalog *Catalog, id string, variantID string) (ComponentRecord, PackageVariant, bool) {
	if catalog == nil {
		return ComponentRecord{}, PackageVariant{}, false
	}
	catalog.mu.RLock()
	defer catalog.mu.RUnlock()
	if catalog.variantIndex != nil && variantID != "" {
		if idx, ok := catalog.variantIndex[recordVariantKey(id, variantID)]; ok {
			record := catalog.Records[idx.Record]
			return record, record.Packages[idx.Variant], true
		}
	}
	if catalog.recordIndex != nil && variantID == "" {
		idx, ok := catalog.recordIndex[id]
		if !ok {
			return ComponentRecord{}, PackageVariant{}, false
		}
		record := catalog.Records[idx]
		if len(record.Packages) == 0 {
			return ComponentRecord{}, PackageVariant{}, false
		}
		return record, record.Packages[0], true
	}
	for _, record := range catalog.Records {
		if record.ID != id {
			continue
		}
		for _, variant := range record.Packages {
			if variantID == "" || variant.ID == variantID {
				return record, variant, true
			}
		}
	}
	return ComponentRecord{}, PackageVariant{}, false
}

func recordVariantKey(recordID string, variantID string) string {
	return recordID + "\x00" + variantID
}

func requiredRatingIssues(record ComponentRecord, ratings []RequiredRating) []reports.Issue {
	var issues []reports.Issue
	for _, required := range ratings {
		ok, found := recordSatisfiesRating(record, required)
		if !found {
			issues = append(issues, NewIssue(CodeComponentRatingMissing, reports.SeverityBlocked, "component."+record.ID+".ratings."+required.Kind, "component is missing requested rating "+required.Kind))
			continue
		}
		if !ok {
			issues = append(issues, NewIssue(CodeComponentRatingTooLow, reports.SeverityBlocked, "component."+record.ID+".ratings."+required.Kind, "component rating does not satisfy requested "+required.Kind))
		}
	}
	return issues
}

func requiredFunctionIssues(record ComponentRecord, functions []string) []reports.Issue {
	var issues []reports.Issue
	for _, function := range functions {
		function = strings.TrimSpace(function)
		if function == "" {
			continue
		}
		if !recordHasFunction(record, function) {
			issues = append(issues, NewIssue(CodeComponentFunctionMissing, reports.SeverityBlocked, "component."+record.ID+".functions."+function, "component is missing required function "+function))
		}
	}
	return issues
}

func recordSatisfiesRating(record ComponentRecord, required RequiredRating) (bool, bool) {
	want, ok := parseValueWithUnit(required.Value, required.Unit)
	if !ok {
		return false, true
	}
	found := false
	ledFamily := isLEDFamily(record.Family)
	for _, rating := range record.Ratings {
		if !ratingKindMatches(ledFamily, rating.Kind, required.Kind) {
			continue
		}
		found = true
		if rating.Min != "" {
			minValue, ok := parseValueWithUnit(rating.Min, rating.Unit)
			if ok && want < minValue {
				continue
			}
		}
		if rating.Max != "" {
			maxValue, ok := parseValueWithUnit(rating.Max, rating.Unit)
			if ok && want > maxValue {
				continue
			}
			if ok {
				return true, true
			}
		}
		if rating.Typ != "" {
			typValue, ok := parseValueWithUnit(rating.Typ, rating.Unit)
			if ok && typValue >= want {
				return true, true
			}
		}
	}
	return false, found
}

func ratingKindMatches(ledFamily bool, actual string, required string) bool {
	if actual == required {
		return true
	}
	if ledFamily {
		return (actual == "current" && required == "forward_current") ||
			(actual == "forward_current" && required == "current")
	}
	return false
}

func isLEDFamily(family string) bool {
	return family == componentFamilyLED
}

func parseValueWithUnit(value string, unit string) (float64, bool) {
	if valueHasSuffix(value) {
		return parseLeadingEngineeringNumber(value)
	}
	return parseLeadingEngineeringNumber(value + unit)
}

func valueHasSuffix(value string) bool {
	value = strings.TrimSpace(value)
	end := scanLeadingFloat(value)
	return end > 0 && end < len(value)
}

func candidateAllowedForAcceptance(record ComponentRecord, candidate Candidate, acceptance AcceptanceLevel) bool {
	if AcceptanceAllows(acceptance, candidate.Confidence) {
		return true
	}
	return passiveRuleInferred(record) && AcceptanceAllowsPassiveRuleInferred(acceptance, candidate.Confidence)
}

func passiveRuleInferred(record ComponentRecord) bool {
	if record.Verification.Confidence != ConfidenceRuleInferred {
		return false
	}
	switch record.Family {
	case "resistor", "fuse":
		return true
	case "capacitor":
		for _, tag := range record.Tags {
			if tag == "polarized" {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func recordHasFunction(record ComponentRecord, function string) bool {
	for _, symbol := range record.Symbols {
		for _, pin := range symbol.FunctionPins {
			if strings.EqualFold(pin.Function, function) {
				return true
			}
			for _, alias := range pin.Aliases {
				if strings.EqualFold(alias, function) {
					return true
				}
			}
		}
	}
	return false
}

func recordHasRequiredCompanions(record ComponentRecord) bool {
	for _, companion := range record.Companions {
		if companion.Required {
			return true
		}
	}
	return false
}

func weakerConfidence(a ConfidenceLevel, b ConfidenceLevel) ConfidenceLevel {
	if confidenceRank(a) <= confidenceRank(b) {
		return a
	}
	return b
}

func confidenceRank(level ConfidenceLevel) int {
	switch level {
	case ConfidenceVerified:
		return 5
	case ConfidenceLibraryDerived:
		return 4
	case ConfidenceRuleInferred:
		return 3
	case ConfidencePlaceholder:
		return 1
	case ConfidenceBlocked:
		return 0
	default:
		return -1
	}
}
