package components

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

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

type Query struct {
	Text              string          `json:"text,omitempty"`
	Family            string          `json:"family,omitempty"`
	Package           string          `json:"package,omitempty"`
	ValueKind         string          `json:"value_kind,omitempty"`
	Value             string          `json:"value,omitempty"`
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
	ComponentID string          `json:"component_id"`
	VariantID   string          `json:"variant_id"`
	Family      string          `json:"family"`
	Name        string          `json:"name"`
	FootprintID string          `json:"footprint_id,omitempty"`
	Confidence  ConfidenceLevel `json:"confidence"`
	Score       int             `json:"score"`
	Reasons     []string        `json:"reasons,omitempty"`
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
	var candidates []Candidate
	var issues []reports.Issue
	catalog.mu.RLock()
	defer catalog.mu.RUnlock()
	for _, record := range catalog.Records {
		if err := ctx.Err(); err != nil {
			issue := reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityBlocked, Message: err.Error()}
			return nil, reports.ErrorResult("component find", issue)
		}
		if !recordMatchesQuery(record, query) {
			continue
		}
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
				ComponentID: record.ID,
				VariantID:   variant.ID,
				Family:      record.Family,
				Name:        record.Name,
				FootprintID: variant.FootprintID,
				Confidence:  confidence,
				Score:       score,
				Reasons:     candidateReasons(record, variant, query),
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
		return Selection{}, reports.ResultWithIssues("component select", map[string]any{"candidates": candidates}, findResult.Issues, nil)
	}
	var issues []reports.Issue
	filtered := make([]Candidate, 0, len(candidates))
	rejected := make([]CandidateRejection, 0, len(candidates))
	for _, candidate := range candidates {
		record, _, ok := findRecordVariant(catalog, candidate.ComponentID, candidate.VariantID)
		if !ok {
			continue
		}
		candidateIssues := selectionCandidateIssues(record, candidate, request)
		if len(candidateIssues) > 0 {
			rejected = append(rejected, CandidateRejection{Candidate: candidate, Issues: candidateIssues})
			continue
		}
		filtered = append(filtered, candidate)
	}
	sortCandidates(filtered)
	if len(filtered) == 0 {
		issues = append(issues, dedupeRejectedIssues(rejected)...)
		return Selection{}, reports.ResultWithIssues("component select", map[string]any{"candidates": candidates}, issues, nil)
	}
	if len(filtered) > 1 && !request.AllowAlternatives && filtered[0].Score == filtered[1].Score && filtered[0].Confidence == filtered[1].Confidence {
		issues = append(issues, NewIssue(CodeComponentAmbiguous, reports.SeverityBlocked, "component.select", "multiple components matched with equal score and confidence"))
		return Selection{}, reports.ResultWithIssues("component select", map[string]any{"candidates": filtered[:2]}, issues, nil)
	}
	record, variant, _ := findRecordVariant(catalog, filtered[0].ComponentID, filtered[0].VariantID)
	selection := Selection{Candidate: filtered[0], Component: record, Variant: variant, Rejected: rejected}
	procurement, procurementWarnings := evaluateProcurement(record, request, false)
	if procurement != nil {
		selection.Procurement = procurement
	}
	selection.Warnings = append(selection.Warnings, procurementWarnings...)
	if filtered[0].Confidence == ConfidencePlaceholder {
		selection.Warnings = append(selection.Warnings, NewIssue(CodeComponentUnsafe, reports.SeverityWarning, "component."+filtered[0].ComponentID, "placeholder component selected for draft output"))
	}
	return selection, reports.ResultWithIssues("component select", selection, append(issues, selection.Warnings...), nil)
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
	if request.Acceptance == AcceptanceFabricationCandidate {
		issues = append(issues, fabricationCandidateReviewIssues(record)...)
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

func recordMatchesQuery(record ComponentRecord, query Query) bool {
	if query.Family != "" && record.Family != query.Family {
		return false
	}
	if query.Text != "" {
		text := record.SearchText
		if text == "" {
			text = strings.ToLower(record.ID + " " + record.Name + " " + record.Description + " " + strings.Join(record.Tags, " "))
		}
		if !strings.Contains(text, query.Text) {
			return false
		}
	}
	if query.ValueKind != "" && !recordHasValue(record, query.ValueKind, query.Value) {
		return false
	}
	return true
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
	for _, rating := range record.Ratings {
		if rating.Kind != required.Kind {
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
	case "resistor":
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
			if pin.Function == function {
				return true
			}
			for _, alias := range pin.Aliases {
				if alias == function {
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
