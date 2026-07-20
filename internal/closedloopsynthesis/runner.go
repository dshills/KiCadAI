package closedloopsynthesis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/simmodel"
)

func Run(ctx context.Context, input Input, evaluator Evaluator, policy Policy) Report {
	report := Report{
		Schema: ReportSchema, PolicyVersion: PolicyVersion, CatalogHash: input.CatalogHash,
		FormulaLibraryHash: input.FormulaLibraryHash, ModelRegistryHash: input.ModelRegistryHash, RegistryHash: simmodel.RegistryHash(),
		Policy: policy, StopReason: StopInvalidInput, Status: "blocked", Diagnostics: []Diagnostic{},
	}
	report.PolicyHash = hashJSON(policy)
	if !validHash(report.PolicyHash) {
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Path: "policy", Message: "closed-loop policy could not be canonically hashed"})
		return report
	}
	requirementHash, err := architecturesearch.CanonicalHash(input.Requirement)
	if err == nil {
		report.RequirementHash = requirementHash
	}
	if diagnostics := validateInput(input, evaluator, policy, err); len(diagnostics) != 0 {
		report.Diagnostics = diagnostics
		return report
	}

	candidates := cloneCandidates(input.Candidates)
	slices.SortStableFunc(candidates, func(left, right Candidate) int { return strings.Compare(left.Fingerprint, right.Fingerprint) })
	passing := []int{}
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			report.StopReason = StopCanceled
			report.Diagnostics = append(report.Diagnostics, Diagnostic{Path: "context", Message: err.Error()})
			break
		}
		if report.Consumption.Evaluations >= policy.MaxEvaluations {
			report.StopReason = StopBudgetExhausted
			report.Consumption.BudgetExhausted = true
			break
		}
		candidateReport := evaluateCandidate(ctx, input.Requirement, candidate, evaluator, policy, &report.Consumption)
		report.Candidates = append(report.Candidates, candidateReport)
		report.Consumption.CandidatesEvaluated++
		if candidateReport.Status == "pass" {
			passing = append(passing, len(report.Candidates)-1)
		}
	}
	if len(passing) == 0 {
		if report.StopReason != StopCanceled && report.StopReason != StopBudgetExhausted {
			report.StopReason = StopNoCandidate
		}
		for _, candidate := range report.Candidates {
			report.Diagnostics = append(report.Diagnostics, Diagnostic{Path: "candidates." + candidate.Fingerprint, Message: "candidate stopped: " + string(candidate.StopReason)})
		}
		return report
	}

	selectedIndex := passing[0]
	for _, index := range passing[1:] {
		if betterCandidate(report.Candidates[index], report.Candidates[selectedIndex]) {
			selectedIndex = index
		}
	}
	selected := report.Candidates[selectedIndex]
	report.Selected = &SelectedResult{
		Fingerprint: selected.Fingerprint, State: cloneState(selected.FinalState), Score: selected.FinalScore,
		Repairs: len(selected.Repairs), Rationale: selectionRationale(selected, len(passing)),
	}
	report.Status = "pass"
	report.StopReason = StopPassed
	return report
}

func evaluateCandidate(ctx context.Context, requirement architecturesearch.Requirement, candidate Candidate, evaluator Evaluator, policy Policy, consumption *Consumption) CandidateReport {
	result := CandidateReport{Fingerprint: candidate.Fingerprint, StaticScore: candidate.Score, Status: "blocked", StopReason: StopEvaluationFailed}
	state := CandidateState{Fingerprint: candidate.Fingerprint, Variables: cloneVariables(candidate.Variables)}
	normalizeState(&state)
	evaluated := map[string]bool{}
	current := runAttempt(ctx, requirement, state, evaluator, consumption, 1)
	result.Attempts = append(result.Attempts, current)
	evaluated[current.StateHash] = true
	if current.Status == "pass" {
		return finishCandidate(result, current, StopPassed)
	}
	if current.Status == "blocked" {
		return finishCandidate(result, current, attemptStopReason(current))
	}
	if len(state.Variables) == 0 {
		return finishCandidate(result, current, StopNoRepairVariables)
	}

	for repairNumber := 1; repairNumber <= policy.MaxRepairsPerCandidate; repairNumber++ {
		if err := ctx.Err(); err != nil {
			return finishCandidate(result, current, StopCanceled)
		}
		neighbors := neighboringStates(state)
		var best *Attempt
		trials := 0
		for _, neighbor := range neighbors {
			hash := stateHash(neighbor)
			if evaluated[hash] {
				continue
			}
			if consumption.Evaluations >= policy.MaxEvaluations {
				consumption.BudgetExhausted = true
				return finishCandidate(result, current, StopBudgetExhausted)
			}
			trial := runAttempt(ctx, requirement, neighbor, evaluator, consumption, len(result.Attempts)+1)
			result.Attempts = append(result.Attempts, trial)
			evaluated[hash] = true
			consumption.RepairTrials++
			trials++
			if trial.Status == "blocked" || !betterEvaluation(trial.Score, current.Score) {
				continue
			}
			if best == nil || betterTrial(trial, *best) {
				copy := trial
				best = &copy
			}
		}
		if best == nil {
			return finishCandidate(result, current, StopNonImprovement)
		}
		repair, ok := repairBetween(state, best.State, repairNumber, current.StateHash, best.StateHash, trials, current.Diagnoses)
		if !ok {
			return finishCandidate(result, current, StopRepeatedState)
		}
		result.Repairs = append(result.Repairs, repair)
		consumption.RepairsApplied++
		state = cloneState(best.State)
		current = *best
		if current.Status == "pass" {
			return finishCandidate(result, current, StopPassed)
		}
	}
	return finishCandidate(result, current, StopBudgetExhausted)
}

func runAttempt(ctx context.Context, requirement architecturesearch.Requirement, state CandidateState, evaluator Evaluator, consumption *Consumption, number int) Attempt {
	attempt := Attempt{Number: number, State: cloneState(state), StateHash: stateHash(state), Status: "blocked", Diagnostics: []Diagnostic{}}
	if !validHash(attempt.StateHash) {
		attempt.Diagnostics = append(attempt.Diagnostics, Diagnostic{Path: "state", Message: "candidate state could not be canonically hashed"})
		return attempt
	}
	if err := ctx.Err(); err != nil {
		attempt.Diagnostics = append(attempt.Diagnostics, Diagnostic{Path: "context", Message: err.Error()})
		return attempt
	}
	evaluation, err := evaluator.Evaluate(ctx, cloneState(state))
	consumption.Evaluations++
	if err != nil {
		attempt.Diagnostics = append(attempt.Diagnostics, Diagnostic{Path: "evaluation", Message: err.Error()})
		return attempt
	}
	attempt.EvidenceHash = evaluation.EvidenceHash
	if !validHash(evaluation.EvidenceHash) {
		attempt.Diagnostics = append(attempt.Diagnostics, Diagnostic{Path: "evaluation.evidence_hash", Message: "trusted evaluation must provide a lowercase SHA-256 evidence hash"})
	}
	assertions, assertionDiagnostics := evaluateMeasurements(requirement, evaluation.Measurements)
	attempt.Assertions = assertions
	attempt.Diagnostics = append(attempt.Diagnostics, assertionDiagnostics...)
	modelDecisions, modelUses, modelDiagnostics := validateModelDecisions(requirement, evaluation.ModelDecisions)
	attempt.ModelDecisions = modelDecisions
	attempt.Diagnostics = append(attempt.Diagnostics, modelDiagnostics...)
	attempt.Score = scoreAssertions(assertions, modelUses)
	attempt.Diagnoses = diagnose(assertions)
	if len(attempt.Diagnostics) != 0 {
		return attempt
	}
	attempt.Status = "fail"
	if attempt.Score.Failures == 0 {
		attempt.Status = "pass"
	}
	return attempt
}

func evaluateMeasurements(requirement architecturesearch.Requirement, measurements []Measurement) ([]AssertionResult, []Diagnostic) {
	byKey := map[string]Measurement{}
	var diagnostics []Diagnostic
	for index, measurement := range measurements {
		path := fmt.Sprintf("evaluation.measurements[%d]", index)
		key := measurement.RequirementID + "\x00" + measurement.OperatingCase
		if _, duplicate := byKey[key]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "duplicate behavioral measurement"})
			continue
		}
		if !finite(measurement.Actual) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".actual", Message: "behavioral measurement must be finite"})
		}
		byKey[key] = measurement
	}
	results := []AssertionResult{}
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		for _, caseID := range behavior.OperatingCases {
			key := behavior.ID + "\x00" + caseID
			measurement, exists := byKey[key]
			if !exists {
				diagnostics = append(diagnostics, Diagnostic{Path: "evaluation.measurements", Message: "missing measurement for " + behavior.ID + " in operating case " + caseID})
				continue
			}
			delete(byKey, key)
			result := AssertionResult{
				RequirementID: behavior.ID, OperatingCase: caseID, Analysis: behavior.Analysis, Metric: behavior.Metric,
				Actual: measurement.Actual, Min: cloneFloat(behavior.Min), Max: cloneFloat(behavior.Max), Critical: behavior.Critical,
			}
			result.Margin, result.Pass = normalizedMargin(result.Actual, result.Min, result.Max)
			results = append(results, result)
		}
	}
	for _, measurement := range byKey {
		diagnostics = append(diagnostics, Diagnostic{Path: "evaluation.measurements", Message: "unexpected measurement for " + measurement.RequirementID + " in operating case " + measurement.OperatingCase})
	}
	slices.SortStableFunc(results, compareAssertionResults)
	slices.SortStableFunc(diagnostics, compareDiagnostics)
	return results, diagnostics
}

func validateModelDecisions(requirement architecturesearch.Requirement, decisions []ModelDecision) ([]ModelDecision, int, []Diagnostic) {
	normalized := cloneModelDecisions(decisions)
	for index := range normalized {
		slices.Sort(normalized[index].RequiredAnalyses)
	}
	slices.SortStableFunc(normalized, compareModelDecisions)
	required := map[string]bool{}
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		required[behavior.Analysis] = true
	}
	covered := map[string]bool{}
	seen := map[string]bool{}
	uses := 0
	var diagnostics []Diagnostic
	for index, decision := range normalized {
		path := fmt.Sprintf("evaluation.model_decisions[%d]", index)
		key := decision.Component + "\x00" + decision.Claim.ModelID
		if strings.TrimSpace(decision.Component) == "" || strings.TrimSpace(decision.Family) == "" || strings.TrimSpace(decision.Claim.ModelID) == "" {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "model decision requires component, family, and model identity"})
		}
		if seen[key] {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "duplicate model decision"})
		}
		seen[key] = true
		switch decision.Status {
		case "used":
			if decision.Reason == "" || len(decision.RequiredAnalyses) == 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "used model decision requires rationale and analysis applicability"})
			}
			for _, diagnostic := range simmodel.ValidateCatalogEvidence(decision.Family, []simmodel.CatalogEvidence{decision.Claim}) {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".claim." + diagnostic.Path, Message: diagnostic.Message, Suggestion: diagnostic.Suggestion})
			}
			for _, diagnostic := range simmodel.ValidateRequiredModelProvenance(decision.Provenance, decision.RequiredAnalyses) {
				diagnostics = append(diagnostics, Diagnostic{Path: path + "." + diagnostic.Path, Message: diagnostic.Message, Suggestion: diagnostic.Suggestion})
			}
			for _, analysis := range decision.RequiredAnalyses {
				if required[analysis] {
					covered[analysis] = true
				}
			}
			uses++
		case "rejected":
			if decision.Reason == "" || len(decision.RequiredAnalyses) != 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "rejected model decision requires reason and cannot claim analysis use"})
			}
		default:
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".status", Message: "model decision status must be used or rejected"})
		}
	}
	for analysis := range required {
		if !covered[analysis] {
			diagnostics = append(diagnostics, Diagnostic{Path: "evaluation.model_decisions", Message: "no reviewed used model covers required analysis " + analysis})
		}
	}
	slices.SortStableFunc(diagnostics, compareDiagnostics)
	return normalized, uses, diagnostics
}

func validateInput(input Input, evaluator Evaluator, policy Policy, requirementHashError error) []Diagnostic {
	var diagnostics []Diagnostic
	if evaluator == nil {
		diagnostics = append(diagnostics, Diagnostic{Path: "evaluator", Message: "trusted evaluator is required"})
	}
	if requirementHashError != nil {
		diagnostics = append(diagnostics, Diagnostic{Path: "requirement", Message: requirementHashError.Error()})
	} else if input.Requirement.Version != architecturesearch.VersionV3 || input.Requirement.Schema != architecturesearch.SchemaIDV3 {
		diagnostics = append(diagnostics, Diagnostic{Path: "requirement", Message: "closed-loop synthesis requires the v3 behavioral requirement schema"})
	}
	if !validHash(input.CatalogHash) || !validHash(input.FormulaLibraryHash) || !validHash(input.ModelRegistryHash) {
		diagnostics = append(diagnostics, Diagnostic{Path: "input", Message: "catalog, formula-library, and model-registry hashes must be lowercase SHA-256 values"})
	}
	if policy.MaxCandidates < 1 || policy.MaxCandidates > 64 || policy.MaxRepairsPerCandidate < 0 || policy.MaxRepairsPerCandidate > 32 || policy.MaxEvaluations < 1 || policy.MaxEvaluations > 4096 || policy.MaxVariablesPerCandidate < 0 || policy.MaxVariablesPerCandidate > 128 || policy.MaxValuesPerVariable < 2 || policy.MaxValuesPerVariable > 256 {
		diagnostics = append(diagnostics, Diagnostic{Path: "policy", Message: "closed-loop policy is outside bounded limits"})
	}
	if len(input.Candidates) == 0 || len(input.Candidates) > policy.MaxCandidates {
		diagnostics = append(diagnostics, Diagnostic{Path: "candidates", Message: "candidate count is empty or exceeds the bounded policy"})
	}
	seen := map[string]bool{}
	for index, candidate := range input.Candidates {
		path := fmt.Sprintf("candidates[%d]", index)
		if !validHash(candidate.Fingerprint) || seen[candidate.Fingerprint] {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".fingerprint", Message: "candidate fingerprint must be a unique lowercase SHA-256 value"})
		}
		seen[candidate.Fingerprint] = true
		if len(candidate.Variables) > policy.MaxVariablesPerCandidate {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".variables", Message: "candidate exceeds the bounded variable count"})
		}
		diagnostics = append(diagnostics, validateVariables(path+".variables", candidate.Variables, policy)...)
	}
	slices.SortStableFunc(diagnostics, compareDiagnostics)
	return diagnostics
}

func validateVariables(path string, variables []Variable, policy Policy) []Diagnostic {
	seen := map[string]bool{}
	var diagnostics []Diagnostic
	for index, variable := range variables {
		entry := fmt.Sprintf("%s[%d]", path, index)
		if variable.ID == "" || seen[variable.ID] {
			diagnostics = append(diagnostics, Diagnostic{Path: entry + ".id", Message: "repair variable id must be nonempty and unique"})
		}
		seen[variable.ID] = true
		if !allowedVariableKind(variable.Kind) {
			diagnostics = append(diagnostics, Diagnostic{Path: entry + ".kind", Message: "unsupported generic repair variable kind"})
		}
		if !finite(variable.Value) || len(variable.AllowedValues) < 2 || len(variable.AllowedValues) > policy.MaxValuesPerVariable {
			diagnostics = append(diagnostics, Diagnostic{Path: entry, Message: "repair variable requires finite current value and a bounded allowed set"})
			continue
		}
		found := false
		previous := math.Inf(-1)
		for _, value := range variable.AllowedValues {
			if !finite(value) || value <= previous {
				diagnostics = append(diagnostics, Diagnostic{Path: entry + ".allowed_values", Message: "allowed repair values must be finite, unique, and strictly ascending"})
				break
			}
			previous = value
			found = found || value == variable.Value
		}
		if !found {
			diagnostics = append(diagnostics, Diagnostic{Path: entry + ".value", Message: "current repair value must be present in allowed_values"})
		}
	}
	return diagnostics
}

func neighboringStates(state CandidateState) []CandidateState {
	var states []CandidateState
	for variableIndex, variable := range state.Variables {
		current := slices.Index(variable.AllowedValues, variable.Value)
		for _, next := range []int{current - 1, current + 1} {
			if next < 0 || next >= len(variable.AllowedValues) {
				continue
			}
			// AllowedValues is immutable after input normalization. Copy only the
			// variable records here; persisted attempts are deep-cloned at their
			// boundary.
			neighbor := CandidateState{Fingerprint: state.Fingerprint, Variables: append([]Variable(nil), state.Variables...)}
			neighbor.Variables[variableIndex].Value = variable.AllowedValues[next]
			states = append(states, neighbor)
		}
	}
	slices.SortStableFunc(states, func(left, right CandidateState) int { return strings.Compare(stateHash(left), stateHash(right)) })
	return states
}

func repairBetween(before, after CandidateState, number int, beforeHash, afterHash string, trials int, diagnoses []Diagnosis) (Repair, bool) {
	for index := range before.Variables {
		if before.Variables[index].Value == after.Variables[index].Value {
			continue
		}
		reason := "improves the complete behavioral assertion score"
		if len(diagnoses) != 0 {
			reason = "addresses " + diagnoses[0].RequirementID + " in " + diagnoses[0].OperatingCase + " while preserving strict whole-report improvement"
		}
		return Repair{Number: number, Variable: before.Variables[index].ID, Kind: before.Variables[index].Kind, From: before.Variables[index].Value, To: after.Variables[index].Value, BeforeHash: beforeHash, AfterHash: afterHash, Reason: reason, EvaluatedTrials: trials}, true
	}
	return Repair{}, false
}

func normalizedMargin(actual float64, minimum, maximum *float64) (float64, bool) {
	scale := math.Max(math.Abs(actual), 1)
	if minimum != nil {
		scale = math.Max(scale, math.Abs(*minimum))
	}
	if maximum != nil {
		scale = math.Max(scale, math.Abs(*maximum))
	}
	margin := math.Inf(1)
	pass := true
	if minimum != nil {
		value := (actual - *minimum) / scale
		margin = math.Min(margin, value)
		pass = pass && actual >= *minimum
	}
	if maximum != nil {
		value := (*maximum - actual) / scale
		margin = math.Min(margin, value)
		pass = pass && actual <= *maximum
	}
	return margin, pass
}

func scoreAssertions(assertions []AssertionResult, modelUses int) EvaluationScore {
	score := EvaluationScore{WorstMargin: math.Inf(1), ModelUses: modelUses}
	for _, assertion := range assertions {
		score.WorstMargin = math.Min(score.WorstMargin, assertion.Margin)
		if assertion.Pass {
			continue
		}
		score.Failures++
		if assertion.Critical {
			score.CriticalFailures++
		}
	}
	if len(assertions) == 0 {
		score.WorstMargin = math.Inf(-1)
	}
	return score
}

func diagnose(assertions []AssertionResult) []Diagnosis {
	var diagnoses []Diagnosis
	for _, assertion := range assertions {
		if assertion.Pass {
			continue
		}
		direction := "decrease"
		if assertion.Min != nil && assertion.Actual < *assertion.Min {
			direction = "increase"
		}
		diagnoses = append(diagnoses, Diagnosis{RequirementID: assertion.RequirementID, OperatingCase: assertion.OperatingCase, Analysis: assertion.Analysis, Metric: assertion.Metric, Direction: direction, Critical: assertion.Critical, Message: fmt.Sprintf("measured %g is outside the required bounds", assertion.Actual)})
	}
	slices.SortStableFunc(diagnoses, func(left, right Diagnosis) int {
		if left.Critical != right.Critical {
			if left.Critical {
				return -1
			}
			return 1
		}
		if order := strings.Compare(left.RequirementID, right.RequirementID); order != 0 {
			return order
		}
		return strings.Compare(left.OperatingCase, right.OperatingCase)
	})
	return diagnoses
}

func betterEvaluation(left, right EvaluationScore) bool {
	if left.CriticalFailures != right.CriticalFailures {
		return left.CriticalFailures < right.CriticalFailures
	}
	if left.Failures != right.Failures {
		return left.Failures < right.Failures
	}
	if left.WorstMargin != right.WorstMargin {
		return left.WorstMargin > right.WorstMargin
	}
	return left.ModelUses > right.ModelUses
}

func betterTrial(left, right Attempt) bool {
	if betterEvaluation(left.Score, right.Score) {
		return true
	}
	if betterEvaluation(right.Score, left.Score) {
		return false
	}
	return left.StateHash < right.StateHash
}

func betterCandidate(left, right CandidateReport) bool {
	if betterEvaluation(left.FinalScore, right.FinalScore) {
		return true
	}
	if betterEvaluation(right.FinalScore, left.FinalScore) {
		return false
	}
	if len(left.Repairs) != len(right.Repairs) {
		return len(left.Repairs) < len(right.Repairs)
	}
	if order := compareArchitectureScores(left.StaticScore, right.StaticScore); order != 0 {
		return order < 0
	}
	return left.Fingerprint < right.Fingerprint
}

func compareArchitectureScores(left, right architecturesearch.CandidateScore) int {
	if left.UnprovenNonSafety != right.UnprovenNonSafety {
		return compareInt(left.UnprovenNonSafety, right.UnprovenNonSafety)
	}
	if order := compareOptionalDescending(left.WorstMargin, right.WorstMargin); order != 0 {
		return order
	}
	if left.EvidenceRank != right.EvidenceRank {
		return compareInt(right.EvidenceRank, left.EvidenceRank)
	}
	if left.ComponentCount != right.ComponentCount {
		return compareInt(left.ComponentCount, right.ComponentCount)
	}
	if left.FragmentCount != right.FragmentCount {
		return compareInt(left.FragmentCount, right.FragmentCount)
	}
	if order := compareOptionalAscending(left.QuiescentPowerW, right.QuiescentPowerW); order != 0 {
		return order
	}
	return compareOptionalAscending(left.AreaMM2, right.AreaMM2)
}

func finishCandidate(result CandidateReport, final Attempt, reason StopReason) CandidateReport {
	result.FinalState = cloneState(final.State)
	result.FinalScore = final.Score
	result.StopReason = reason
	if final.Status == "pass" && reason == StopPassed {
		result.Status = "pass"
	} else {
		result.Status = "rejected"
	}
	return result
}

func attemptStopReason(attempt Attempt) StopReason {
	for _, diagnostic := range attempt.Diagnostics {
		if strings.Contains(diagnostic.Path, "model_decisions") || strings.Contains(diagnostic.Message, "model") {
			return StopModelTrustFailed
		}
		if strings.Contains(diagnostic.Message, "measurement") {
			return StopAssertionIncomplete
		}
	}
	return StopEvaluationFailed
}

func selectionRationale(selected CandidateReport, passing int) string {
	return fmt.Sprintf("selected %s from %d passing candidates by worst behavioral margin, reviewed model use, repair count, existing architecture score, and canonical fingerprint", selected.Fingerprint, passing)
}

func normalizeState(state *CandidateState) {
	for index := range state.Variables {
		state.Variables[index].AllowedValues = append([]float64(nil), state.Variables[index].AllowedValues...)
	}
	slices.SortStableFunc(state.Variables, func(left, right Variable) int { return strings.Compare(left.ID, right.ID) })
}

func stateHash(state CandidateState) string { return hashJSON(state) }

func hashJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func MarshalReport(report Report) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

func validHash(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func cloneCandidates(source []Candidate) []Candidate {
	clone := append([]Candidate(nil), source...)
	for index := range clone {
		clone[index].Variables = cloneVariables(source[index].Variables)
	}
	return clone
}

func cloneVariables(source []Variable) []Variable {
	clone := append([]Variable(nil), source...)
	for index := range clone {
		clone[index].AllowedValues = append([]float64(nil), source[index].AllowedValues...)
	}
	return clone
}

func cloneState(source CandidateState) CandidateState {
	return CandidateState{Fingerprint: source.Fingerprint, Variables: cloneVariables(source.Variables)}
}

func cloneFloat(source *float64) *float64 {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}

func cloneModelDecisions(source []ModelDecision) []ModelDecision {
	data, err := json.Marshal(source)
	if err != nil {
		return append([]ModelDecision(nil), source...)
	}
	var clone []ModelDecision
	if err := json.Unmarshal(data, &clone); err != nil {
		return append([]ModelDecision(nil), source...)
	}
	return clone
}

func compareAssertionResults(left, right AssertionResult) int {
	if order := strings.Compare(left.RequirementID, right.RequirementID); order != 0 {
		return order
	}
	return strings.Compare(left.OperatingCase, right.OperatingCase)
}

func compareModelDecisions(left, right ModelDecision) int {
	if order := strings.Compare(left.Component, right.Component); order != 0 {
		return order
	}
	if order := strings.Compare(left.Claim.ModelID, right.Claim.ModelID); order != 0 {
		return order
	}
	return strings.Compare(left.Status, right.Status)
}

func compareDiagnostics(left, right Diagnostic) int {
	if order := strings.Compare(left.Path, right.Path); order != 0 {
		return order
	}
	return strings.Compare(left.Message, right.Message)
}

func allowedVariableKind(kind string) bool {
	switch kind {
	case "passive_value", "bias", "gain", "filter", "compensation", "protection", "catalog_variant_index":
		return true
	default:
		return false
	}
}

func compareInt(left, right int) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareOptionalDescending(left, right *float64) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return 1
	}
	if right == nil {
		return -1
	}
	if *left > *right {
		return -1
	}
	if *left < *right {
		return 1
	}
	return 0
}

func compareOptionalAscending(left, right *float64) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return 1
	}
	if right == nil {
		return -1
	}
	if *left < *right {
		return -1
	}
	if *left > *right {
		return 1
	}
	return 0
}
