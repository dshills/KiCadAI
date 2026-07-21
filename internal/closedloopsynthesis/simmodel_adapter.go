package closedloopsynthesis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"

	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

// SimulationResolver is the trusted boundary between a candidate state and a
// fully resolved simulation plan. Implementations must apply every variable,
// re-resolve catalog identities and connectivity, and return a fresh plan on
// every call.
type SimulationResolver interface {
	ResolveSimulation(context.Context, CandidateState) (SimulationResolution, error)
}

type SimulationResolution struct {
	Plan           simmodel.Plan               `json:"plan"`
	Plans          []simmodel.Plan             `json:"plans,omitempty"`
	Measurements   []SimulationMeasurementLink `json:"measurements"`
	ModelDecisions []ModelDecision             `json:"model_decisions"`
}

// SimulationMeasurementLink maps one behavioral assertion to one trusted
// simmodel assertion result. The index is resolver-owned and refers to the
// final validated plan returned in the same resolution.
type SimulationMeasurementLink struct {
	RequirementID string                   `json:"requirement_id"`
	OperatingCase string                   `json:"operating_case"`
	Plan          int                      `json:"plan,omitempty"`
	Assertion     int                      `json:"assertion,omitempty"`
	Assertions    []int                    `json:"assertions,omitempty"`
	Evidence      []SimulationAssertionSet `json:"evidence,omitempty"`
}

// SimulationAssertionSet identifies the assertions for one deterministic plan
// batch. A measurement may span several batches when independent operating
// corners cannot fit under one plan's trusted work bound.
type SimulationAssertionSet struct {
	Plan       int   `json:"plan"`
	Assertions []int `json:"assertions"`
}

// SimModelEvaluator executes resolved plans only through the registered
// simmodel evaluator and converts its assertion results into closed-loop
// measurements. Provider-authored diagnostics never enter this boundary.
type SimModelEvaluator struct {
	Resolver           SimulationResolver
	ProvenanceRegistry modelprovenance.Registry
}

func (evaluator SimModelEvaluator) Evaluate(ctx context.Context, state CandidateState) (Evaluation, error) {
	if evaluator.Resolver == nil {
		return Evaluation{}, fmt.Errorf("simulation resolver is required")
	}
	resolution, err := evaluator.Resolver.ResolveSimulation(ctx, cloneState(state))
	if err != nil {
		return Evaluation{}, fmt.Errorf("resolve simulation: %w", err)
	}
	if diagnostics := validateSimulationResolution(resolution); len(diagnostics) != 0 {
		return Evaluation{}, fmt.Errorf("invalid simulation resolution: %s", joinDiagnosticMessages(diagnostics))
	}
	plans := resolutionPlans(resolution)
	modelDecisions, modelDiagnostics := resolveResolutionModelDecisions(plans, evaluator.ProvenanceRegistry)
	if len(modelDiagnostics) != 0 {
		return Evaluation{}, fmt.Errorf("model trust resolution failed: %s", joinDiagnosticMessages(modelDiagnostics))
	}
	// Provenance is derived after trusted resolution. Any resolver-supplied
	// decisions are replaced so they cannot become promotion evidence.
	resolution.ModelDecisions = modelDecisions
	reports := make([]simmodel.Report, len(plans))
	for index, plan := range plans {
		report, diagnostics := simmodel.Evaluate(simmodel.ClonePlan(plan))
		if len(diagnostics) != 0 && !onlyAssertionFailures(report, diagnostics) {
			analysisKinds := make([]string, 0, len(plan.Analyses))
			for _, analysis := range plan.Analyses {
				analysisKinds = append(analysisKinds, analysis.Kind)
			}
			return Evaluation{}, fmt.Errorf("trusted simulation plan %d (%s: %s) failed: %s", index, plan.ModelID, strings.Join(analysisKinds, ","), joinSimModelDiagnostics(diagnostics))
		}
		reports[index] = report
	}
	measurements := make([]Measurement, 0, len(resolution.Measurements))
	for _, link := range resolution.Measurements {
		assertion, err := worstLinkedMeasurement(plans, reports, link)
		if err != nil {
			return Evaluation{}, err
		}
		measurements = append(measurements, Measurement{
			RequirementID: link.RequirementID,
			OperatingCase: link.OperatingCase,
			Actual:        assertion.Actual,
		})
	}
	slices.SortStableFunc(measurements, compareMeasurements)
	evidenceHash, err := simulationEvidenceHash(resolution, reports)
	if err != nil {
		return Evaluation{}, fmt.Errorf("hash simulation evidence: %w", err)
	}
	return Evaluation{
		EvidenceHash: evidenceHash, Measurements: measurements,
		ModelDecisions: cloneModelDecisions(resolution.ModelDecisions),
		Simulation:     &SimulationEvidence{Resolution: cloneSimulationResolution(resolution), Reports: cloneSimulationReports(reports)},
	}, nil
}

func validateSimulationResolution(resolution SimulationResolution) []Diagnostic {
	var diagnostics []Diagnostic
	plans := resolutionPlans(resolution)
	if len(resolution.Plans) != 0 && resolution.Plan.ModelID != "" {
		diagnostics = append(diagnostics, Diagnostic{Path: "plans", Message: "simulation resolution must use either legacy plan or plans, not both"})
	}
	if len(plans) == 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "plans", Message: "simulation resolution requires at least one resolved plan"})
	}
	for planIndex, plan := range plans {
		for _, diagnostic := range simmodel.ValidatePlan(plan) {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("plans[%d].%s", planIndex, diagnostic.Path), Message: diagnostic.Message, Suggestion: diagnostic.Suggestion})
		}
	}
	seenBehavior := map[string]bool{}
	seenAssertion := map[string]bool{}
	for index, link := range resolution.Measurements {
		path := fmt.Sprintf("measurements[%d]", index)
		if strings.TrimSpace(link.RequirementID) == "" || strings.TrimSpace(link.OperatingCase) == "" {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "simulation measurement link requires requirement and operating-case identities"})
		}
		sets := measurementAssertionSets(link)
		if len(sets) == 0 {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".evidence", Message: "simulation measurement link requires at least one assertion set"})
		}
		behaviorKey := link.RequirementID + "\x00" + link.OperatingCase
		if seenBehavior[behaviorKey] {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "simulation measurement link duplicates a behavioral assertion"})
		}
		previousPlan := -1
		for setIndex, set := range sets {
			setPath := fmt.Sprintf("%s.evidence[%d]", path, setIndex)
			if set.Plan < 0 || set.Plan >= len(plans) {
				diagnostics = append(diagnostics, Diagnostic{Path: setPath + ".plan", Message: "simulation assertion set references an out-of-range plan"})
				continue
			}
			if set.Plan <= previousPlan {
				diagnostics = append(diagnostics, Diagnostic{Path: setPath + ".plan", Message: "simulation assertion sets must use unique canonically ordered plans"})
			}
			if len(set.Assertions) == 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: setPath + ".assertions", Message: "simulation assertion set requires at least one assertion"})
			}
			previous := -1
			for _, assertion := range set.Assertions {
				assertionKey := fmt.Sprintf("%d:%d", set.Plan, assertion)
				if assertion < 0 || assertion >= len(plans[set.Plan].Assertions) {
					diagnostics = append(diagnostics, Diagnostic{Path: setPath + ".assertions", Message: "simulation assertion set references an out-of-range assertion"})
				}
				if assertion <= previous {
					diagnostics = append(diagnostics, Diagnostic{Path: setPath + ".assertions", Message: "simulation assertion indices must be unique and canonically ordered"})
				}
				if seenAssertion[assertionKey] {
					diagnostics = append(diagnostics, Diagnostic{Path: setPath + ".assertions", Message: "simulation assertion is mapped more than once"})
				}
				seenAssertion[assertionKey] = true
				previous = assertion
			}
			previousPlan = set.Plan
		}
		seenBehavior[behaviorKey] = true
	}
	if len(resolution.Measurements) == 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "measurements", Message: "simulation resolution requires behavioral measurement links"})
	}
	slices.SortStableFunc(diagnostics, compareDiagnostics)
	return diagnostics
}

func resolutionPlans(resolution SimulationResolution) []simmodel.Plan {
	if len(resolution.Plans) != 0 {
		return resolution.Plans
	}
	if resolution.Plan.ModelID != "" {
		return []simmodel.Plan{resolution.Plan}
	}
	return nil
}

func resolveResolutionModelDecisions(plans []simmodel.Plan, registry modelprovenance.Registry) ([]ModelDecision, []Diagnostic) {
	byKey := map[string]ModelDecision{}
	var diagnostics []Diagnostic
	for planIndex, plan := range plans {
		decisions, planDiagnostics := ResolvePlanModelDecisions(plan, registry)
		for _, diagnostic := range planDiagnostics {
			diagnostic.Path = fmt.Sprintf("plans[%d].%s", planIndex, diagnostic.Path)
			diagnostics = append(diagnostics, diagnostic)
		}
		for _, decision := range decisions {
			key := decision.Component + "\x00" + decision.Claim.ModelID
			if existing, exists := byKey[key]; exists {
				existing.RequiredAnalyses = append(existing.RequiredAnalyses, decision.RequiredAnalyses...)
				slices.Sort(existing.RequiredAnalyses)
				existing.RequiredAnalyses = slices.Compact(existing.RequiredAnalyses)
				if existing.Status != decision.Status || existing.Family != decision.Family || !reflect.DeepEqual(existing.Claim, decision.Claim) || !reflect.DeepEqual(existing.Provenance, decision.Provenance) {
					diagnostics = append(diagnostics, Diagnostic{Path: "model_decisions." + decision.Component, Message: "resolved model decision differs across simulation plans"})
				}
				byKey[key] = existing
				continue
			}
			byKey[key] = decision
		}
	}
	result := make([]ModelDecision, 0, len(byKey))
	for _, decision := range byKey {
		result = append(result, decision)
	}
	slices.SortStableFunc(result, compareModelDecisions)
	slices.SortStableFunc(diagnostics, compareDiagnostics)
	return result, diagnostics
}

// ResolvePlanSetModelDecisions derives and merges provenance decisions from
// independently resolved workflow plans in canonical analysis-kind order.
func ResolvePlanSetModelDecisions(plans map[string]simmodel.Plan, registry modelprovenance.Registry) ([]ModelDecision, []Diagnostic) {
	kinds := make([]string, 0, len(plans))
	for kind := range plans {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	ordered := make([]simmodel.Plan, 0, len(kinds))
	for _, kind := range kinds {
		ordered = append(ordered, plans[kind])
	}
	return resolveResolutionModelDecisions(ordered, registry)
}

func measurementAssertionIndices(link SimulationMeasurementLink) []int {
	if len(link.Assertions) != 0 {
		return append([]int(nil), link.Assertions...)
	}
	return []int{link.Assertion}
}

func measurementAssertionSets(link SimulationMeasurementLink) []SimulationAssertionSet {
	if len(link.Evidence) != 0 {
		sets := make([]SimulationAssertionSet, len(link.Evidence))
		for index, set := range link.Evidence {
			sets[index] = SimulationAssertionSet{Plan: set.Plan, Assertions: append([]int(nil), set.Assertions...)}
		}
		return sets
	}
	return []SimulationAssertionSet{{Plan: link.Plan, Assertions: measurementAssertionIndices(link)}}
}

func worstLinkedMeasurement(plans []simmodel.Plan, reports []simmodel.Report, link SimulationMeasurementLink) (simmodel.AssertionResult, error) {
	sets := measurementAssertionSets(link)
	if len(sets) == 0 {
		return simmodel.AssertionResult{}, fmt.Errorf("simulation measurement link has no assertion sets")
	}
	var worst simmodel.AssertionResult
	worstMargin := math.Inf(1)
	for _, set := range sets {
		if set.Plan < 0 || set.Plan >= len(plans) || set.Plan >= len(reports) {
			return simmodel.AssertionResult{}, fmt.Errorf("simulation measurement plan index %d is outside plan/report bounds %d/%d", set.Plan, len(plans), len(reports))
		}
		candidate, err := worstLinkedAssertion(plans[set.Plan], reports[set.Plan], set.Assertions)
		if err != nil {
			return simmodel.AssertionResult{}, err
		}
		margin := linkedAssertionResultMargin(candidate)
		if margin < worstMargin {
			worst, worstMargin = candidate, margin
		}
	}
	return worst, nil
}

func linkedAssertionResultMargin(result simmodel.AssertionResult) float64 {
	scale := math.Max(1, math.Max(math.Abs(result.Min), math.Abs(result.Max)))
	return math.Min(result.Actual-result.Min, result.Max-result.Actual) / scale
}

func worstLinkedAssertion(plan simmodel.Plan, report simmodel.Report, indices []int) (simmodel.AssertionResult, error) {
	if len(indices) == 0 {
		return simmodel.AssertionResult{}, fmt.Errorf("simulation measurement link has no assertions")
	}
	for _, index := range indices {
		if index < 0 || index >= len(plan.Assertions) || index >= len(report.Assertions) {
			return simmodel.AssertionResult{}, fmt.Errorf("simulation measurement assertion index %d is outside plan/report bounds %d/%d", index, len(plan.Assertions), len(report.Assertions))
		}
	}
	worst := report.Assertions[indices[0]]
	worstMargin := linkedAssertionMargin(plan.Assertions[indices[0]], worst.Actual)
	for _, index := range indices[1:] {
		candidate := report.Assertions[index]
		margin := linkedAssertionMargin(plan.Assertions[index], candidate.Actual)
		if margin < worstMargin {
			worst, worstMargin = candidate, margin
		}
	}
	return worst, nil
}

func linkedAssertionMargin(assertion simmodel.Assertion, actual float64) float64 {
	scale := math.Max(1, math.Max(math.Abs(assertion.Min), math.Abs(assertion.Max)))
	return math.Min(actual-assertion.Min, assertion.Max-actual) / scale
}

func onlyAssertionFailures(report simmodel.Report, diagnostics []simmodel.Diagnostic) bool {
	if len(report.Assertions) == 0 {
		return false
	}
	for _, diagnostic := range diagnostics {
		measured := strings.HasPrefix(diagnostic.Message, "measured ") || strings.Contains(diagnostic.Message, " measured ")
		measuredOutOfBounds := measured && strings.Contains(diagnostic.Message, " outside trusted bounds ")
		nominalAssertion := strings.HasPrefix(diagnostic.Path, "assertions.") && measuredOutOfBounds
		cornerAssertion := strings.HasPrefix(diagnostic.Path, "worst_case.") && measuredOutOfBounds
		if !nominalAssertion && !cornerAssertion {
			return false
		}
	}
	return true
}

func simulationEvidenceHash(resolution SimulationResolution, reports []simmodel.Report) (string, error) {
	payload := struct {
		Resolution SimulationResolution `json:"resolution"`
		Reports    []simmodel.Report    `json:"reports"`
	}{Resolution: resolution, Reports: reports}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// HashSimulationEvidence returns the canonical digest used by an attempt.
func HashSimulationEvidence(evidence SimulationEvidence) (string, error) {
	return simulationEvidenceHash(evidence.Resolution, evidence.Reports)
}

func cloneSimulationEvidence(source *SimulationEvidence) *SimulationEvidence {
	if source == nil {
		return nil
	}
	return &SimulationEvidence{Resolution: cloneSimulationResolution(source.Resolution), Reports: cloneSimulationReports(source.Reports)}
}

func cloneSimulationResolution(source SimulationResolution) SimulationResolution {
	data, err := json.Marshal(source)
	if err != nil {
		return source
	}
	var clone SimulationResolution
	if err := json.Unmarshal(data, &clone); err != nil {
		return source
	}
	return clone
}

func cloneSimulationReports(source []simmodel.Report) []simmodel.Report {
	data, err := json.Marshal(source)
	if err != nil {
		return source
	}
	var clone []simmodel.Report
	if err := json.Unmarshal(data, &clone); err != nil {
		return source
	}
	return clone
}

func joinDiagnosticMessages(diagnostics []Diagnostic) string {
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		parts = append(parts, diagnostic.Path+": "+diagnostic.Message)
	}
	return strings.Join(parts, "; ")
}

func joinSimModelDiagnostics(diagnostics []simmodel.Diagnostic) string {
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		parts = append(parts, diagnostic.Path+": "+diagnostic.Message)
	}
	return strings.Join(parts, "; ")
}

func compareMeasurements(left, right Measurement) int {
	if order := strings.Compare(left.RequirementID, right.RequirementID); order != 0 {
		return order
	}
	return strings.Compare(left.OperatingCase, right.OperatingCase)
}
