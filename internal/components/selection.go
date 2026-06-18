package components

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"kicadai/internal/reports"
)

const (
	CodeComponentNotFound         reports.Code = "COMPONENT_NOT_FOUND"
	CodeComponentAmbiguous        reports.Code = "COMPONENT_AMBIGUOUS"
	CodeComponentUnsafe           reports.Code = "COMPONENT_UNSAFE_CONFIDENCE"
	CodeComponentRatingTooLow     reports.Code = "COMPONENT_RATING_TOO_LOW"
	CodeComponentRatingMissing    reports.Code = "COMPONENT_RATING_MISSING"
	CodeComponentFunctionMissing  reports.Code = "COMPONENT_FUNCTION_MISSING"
	CodeComponentVariantMissing   reports.Code = "COMPONENT_VARIANT_MISSING"
	CodeComponentConcreteRequired reports.Code = "COMPONENT_CONCRETE_REQUIRED"
	CodeComponentCompanionMissing reports.Code = "COMPONENT_COMPANION_MISSING"
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
	Query             Query            `json:"query"`
	Acceptance        AcceptanceLevel  `json:"acceptance,omitempty"`
	AllowAlternatives bool             `json:"allow_alternatives,omitempty"`
	RequiredRatings   []RequiredRating `json:"required_ratings,omitempty"`
	RequiredFunctions []string         `json:"required_functions,omitempty"`
	RequireConcrete   bool             `json:"require_concrete,omitempty"`
	RequireCompanions bool             `json:"require_companions,omitempty"`
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
	Candidate Candidate            `json:"candidate"`
	Component ComponentRecord      `json:"component"`
	Variant   PackageVariant       `json:"variant"`
	Warnings  []reports.Issue      `json:"warnings,omitempty"`
	Rejected  []CandidateRejection `json:"rejected,omitempty"`
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
	if !candidateAllowedForAcceptance(record, candidate, request.Acceptance) {
		issues = append(issues, NewIssue(CodeComponentUnsafe, reports.SeverityBlocked, "component."+candidate.ComponentID, fmt.Sprintf("component confidence %s is not allowed for %s acceptance", candidate.Confidence, request.Acceptance)))
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
