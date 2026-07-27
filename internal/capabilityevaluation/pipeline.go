package capabilityevaluation

import (
	"fmt"
	"slices"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/behavioralintent"
)

const (
	CodeClarificationRequired    = "CLARIFICATION_REQUIRED"
	CodeCapabilityUnsupported    = "CAPABILITY_UNSUPPORTED"
	CodeCapabilityAmbiguous      = "CAPABILITY_AMBIGUOUS"
	CodeSearchBudgetExhausted    = "SEARCH_BUDGET_EXHAUSTED"
	CodeArchitectureSearchFailed = "ARCHITECTURE_SEARCH_FAILED"
)

// CaseFromPipeline converts normal compiler and architecture-search evidence
// into the evaluator's terminal schema. It deliberately receives no prompt or
// fixture identity beyond the frozen corpus metadata.
func CaseFromPipeline(meta CorpusCase, compilation behavioralintent.Result, search *architecturesearch.SearchResult) (CaseResult, error) {
	result := CaseResult{ID: meta.ID, Domain: meta.Domain, SafetyImpact: meta.SafetyImpact}
	switch compilation.Status {
	case behavioralintent.StatusNeedsClarification:
		result.Outcome = OutcomeNeedsClarification
		result.Observations = clarificationObservations(compilation)
	case behavioralintent.StatusUnsupported:
		result.Outcome = OutcomeUnsupported
		result.Observations = capabilityGapObservations(compilation)
	case behavioralintent.StatusReady:
		if search == nil {
			return CaseResult{}, fmt.Errorf("ready compilation for %q requires architecture-search evidence", meta.ID)
		}
		outcome, observations, err := searchObservations(*search)
		if err != nil {
			return CaseResult{}, fmt.Errorf("case %q: %w", meta.ID, err)
		}
		result.Outcome, result.Observations = outcome, observations
	default:
		return CaseResult{}, fmt.Errorf("case %q has unsupported compilation status %q", meta.ID, compilation.Status)
	}
	if result.Outcome != OutcomeReady && len(result.Observations) == 0 {
		return CaseResult{}, fmt.Errorf("case %q terminal outcome %q lacks blocking evidence", meta.ID, result.Outcome)
	}
	return result, nil
}

func clarificationObservations(compilation behavioralintent.Result) []Observation {
	uncertaintyKind := map[string]string{}
	for _, uncertainty := range compilation.Uncertainties {
		if uncertainty.ResolvedBy != "" {
			uncertaintyKind[uncertainty.ResolvedBy] = uncertainty.Kind
		}
	}
	result := make([]Observation, 0, len(compilation.Clarifications))
	for _, clarification := range compilation.Clarifications {
		capability := uncertaintyKind[clarification.ID]
		if !semanticIDPattern.MatchString(capability) {
			capability = "user_clarification"
		}
		result = append(result, Observation{
			Capability: capability, Outcome: OutcomeNeedsClarification,
			Stage: "behavioral_compilation", Code: CodeClarificationRequired,
			Path: clarification.Path, Reason: clarification.WhyNeeded,
			RequiredEvidence: []string{"user answer resolving " + clarification.Path},
		})
	}
	return result
}

func capabilityGapObservations(compilation behavioralintent.Result) []Observation {
	result := make([]Observation, 0, len(compilation.CapabilityGaps))
	for _, gap := range compilation.CapabilityGaps {
		result = append(result, Observation{
			Capability: gap.Capability, Outcome: OutcomeUnsupported,
			Stage: "behavioral_compilation", Code: CodeCapabilityUnsupported,
			Path: gap.Path, Reason: gap.Reason, RequiredEvidence: slices.Clone(gap.RequiredEvidence),
		})
	}
	return result
}

func searchObservations(search architecturesearch.SearchResult) (Outcome, []Observation, error) {
	if search.Status == architecturesearch.SearchSelected && search.Selected == nil {
		return "", nil, fmt.Errorf("selected architecture-search status lacks a selected candidate")
	}
	outcome, code, reason, requiredEvidence, err := searchTerminalEvidence(search.Status)
	if err != nil {
		return "", nil, err
	}
	if outcome == OutcomeReady {
		return outcome, nil, nil
	}
	var result []Observation
	if search.Coverage != nil {
		for _, record := range search.Coverage.Records {
			if coverageOutcome(record.Status) != outcome {
				continue
			}
			result = append(result, Observation{
				Capability: record.Capability, Outcome: outcome,
				Stage: "architecture_search", Code: code, Path: record.Path,
				Reason: reason, RequiredEvidence: slices.Clone(requiredEvidence),
			})
		}
	}
	if len(result) == 0 {
		result = append(result, Observation{
			Capability: "architecture_search", Outcome: outcome,
			Stage: "architecture_search", Code: code, Path: "architecture_search",
			Reason: reason, RequiredEvidence: slices.Clone(requiredEvidence),
		})
	}
	return outcome, result, nil
}

func searchTerminalEvidence(status architecturesearch.SearchStatus) (Outcome, string, string, []string, error) {
	switch status {
	case architecturesearch.SearchSelected:
		return OutcomeReady, "", "", nil, nil
	case architecturesearch.SearchUnsupported:
		return OutcomeUnsupported, CodeCapabilityUnsupported,
			"installed architecture providers did not support the required semantic capability",
			[]string{"registered semantic architecture provider", "reviewed implementation and verification evidence"}, nil
	case architecturesearch.SearchAmbiguous:
		return OutcomeAmbiguous, CodeCapabilityAmbiguous,
			"multiple architecture candidates remained without a deterministic qualified selection",
			[]string{"deterministic candidate distinction", "reviewed selection evidence"}, nil
	case architecturesearch.SearchExhausted:
		return OutcomeBudgetExhausted, CodeSearchBudgetExhausted,
			"bounded architecture search ended before a qualified conclusion",
			[]string{"search completion within the reviewed deterministic budget"}, nil
	case architecturesearch.SearchFailed:
		return OutcomeUnsupported, CodeArchitectureSearchFailed,
			"architecture search failed without a qualified candidate",
			[]string{"validated architecture-search evidence", "resolved blocking search diagnostics"}, nil
	default:
		return "", "", "", nil, fmt.Errorf("unsupported architecture-search status %q", status)
	}
}

func coverageOutcome(status architecturesearch.CoverageStatus) Outcome {
	switch status {
	case architecturesearch.CoverageUnsupported:
		return OutcomeUnsupported
	case architecturesearch.CoverageAmbiguous:
		return OutcomeAmbiguous
	case architecturesearch.CoverageBudgetExhausted:
		return OutcomeBudgetExhausted
	default:
		return ""
	}
}
