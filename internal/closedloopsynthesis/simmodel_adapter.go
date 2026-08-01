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
	"sync"

	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

// maxPersistedAnalysisPointsPerReport bounds persisted waveform samples across
// one report. Reports with more than half this many non-empty analyses retain
// both endpoints for every analysis, so the effective budget grows only by the
// minimum needed to preserve those boundaries.
const maxPersistedAnalysisPointsPerReport = 256

const defaultSimulationEvaluationCacheEntries = 256

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
	Cache              *SimulationEvaluationCache
}

// SimulationEvaluationCache reuses trusted results only when the complete
// resolved plan is byte-identical. It is scoped to one synthesis run, bounded,
// and excluded from persisted evidence.
type SimulationEvaluationCache struct {
	mu      sync.Mutex
	limit   int
	entries map[string]simulationEvaluationCacheEntry
}

type simulationEvaluationCacheEntry struct {
	report      simmodel.Report
	diagnostics []simmodel.Diagnostic
}

func NewSimulationEvaluationCache() *SimulationEvaluationCache {
	return &SimulationEvaluationCache{
		limit:   defaultSimulationEvaluationCacheEntries,
		entries: make(map[string]simulationEvaluationCacheEntry),
	}
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
	reports, planDiagnostics := evaluateTrustedSimulationPlans(plans, evaluator.Cache)
	for index, diagnostics := range planDiagnostics {
		if len(diagnostics) != 0 && !onlyAssertionFailures(reports[index], diagnostics) {
			plan := plans[index]
			analysisKinds := make([]string, 0, len(plan.Analyses))
			for _, analysis := range plan.Analyses {
				analysisKinds = append(analysisKinds, analysis.Kind)
			}
			return Evaluation{}, fmt.Errorf("trusted simulation plan %d (%s: %s) failed: %s", index, plan.ModelID, strings.Join(analysisKinds, ","), joinSimModelDiagnostics(diagnostics))
		}
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
	canonicalReports := cloneSimulationReports(reports)
	evidenceHash, err := simulationEvidenceHash(resolution, canonicalReports)
	if err != nil {
		return Evaluation{}, fmt.Errorf("hash simulation evidence: %w", err)
	}
	return Evaluation{
		EvidenceHash: evidenceHash, Measurements: measurements,
		ModelDecisions: cloneModelDecisions(resolution.ModelDecisions),
		Simulation:     &SimulationEvidence{Resolution: cloneSimulationResolution(resolution), Reports: canonicalReports},
	}, nil
}

func evaluateTrustedSimulationPlans(plans []simmodel.Plan, cache *SimulationEvaluationCache) ([]simmodel.Report, [][]simmodel.Diagnostic) {
	reports := make([]simmodel.Report, len(plans))
	diagnostics := make([][]simmodel.Diagnostic, len(plans))
	// simmodel owns the bounded parallelism for a plan's worst-case corners and
	// analyses. Evaluating independent plans serially prevents those nested
	// worker pools from multiplying the CPU and memory work budget.
	for index := range plans {
		key, cacheable := "", false
		if cache != nil {
			key, cacheable = simulationEvaluationCacheKey(plans[index])
			if cacheable {
				if report, planDiagnostics, found := cache.get(key); found {
					reports[index], diagnostics[index] = report, planDiagnostics
					continue
				}
			}
		}
		reports[index], diagnostics[index] = simmodel.Evaluate(simmodel.ClonePlan(plans[index]))
		if cacheable {
			cache.put(key, reports[index], diagnostics[index])
		}
	}
	return reports, diagnostics
}

func (cache *SimulationEvaluationCache) get(key string) (simmodel.Report, []simmodel.Diagnostic, bool) {
	if cache == nil {
		return simmodel.Report{}, nil, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, found := cache.entries[key]
	if !found {
		return simmodel.Report{}, nil, false
	}
	return simmodel.CloneReport(entry.report), append([]simmodel.Diagnostic(nil), entry.diagnostics...), true
}

func (cache *SimulationEvaluationCache) put(key string, report simmodel.Report, diagnostics []simmodel.Diagnostic) {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if _, found := cache.entries[key]; found || len(cache.entries) >= cache.limit {
		return
	}
	cache.entries[key] = simulationEvaluationCacheEntry{
		report: simmodel.CloneReport(report), diagnostics: append([]simmodel.Diagnostic(nil), diagnostics...),
	}
}

func simulationEvaluationCacheKey(plan simmodel.Plan) (string, bool) {
	// encoding/json sorts string-keyed maps, so the complete typed plan has a
	// stable representation.  A future unsupported field fails closed by
	// bypassing the cache rather than reusing an ambiguous result.
	data, err := json.Marshal(plan)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), true
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
				if assertion < 0 || assertion >= len(plans[set.Plan].Assertions) {
					diagnostics = append(diagnostics, Diagnostic{Path: setPath + ".assertions", Message: "simulation assertion set references an out-of-range assertion"})
				}
				if assertion <= previous {
					diagnostics = append(diagnostics, Diagnostic{Path: setPath + ".assertions", Message: "simulation assertion indices must be unique and canonically ordered"})
				}
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
		for cornerIndex, corner := range report.Corners {
			if index >= len(corner.Assertions) {
				return simmodel.AssertionResult{}, fmt.Errorf("simulation measurement assertion index %d is outside corner %d assertion bounds %d", index, cornerIndex, len(corner.Assertions))
			}
		}
	}
	worst := report.Assertions[indices[0]]
	worstMargin := linkedAssertionMargin(plan.Assertions[indices[0]], worst.Actual)
	for _, index := range indices {
		if index != indices[0] {
			candidate := report.Assertions[index]
			margin := linkedAssertionMargin(plan.Assertions[index], candidate.Actual)
			if margin < worstMargin {
				worst, worstMargin = candidate, margin
			}
		}
		for _, corner := range report.Corners {
			candidate := corner.Assertions[index]
			margin := linkedAssertionMargin(plan.Assertions[index], candidate.Actual)
			if margin < worstMargin {
				worst, worstMargin = candidate, margin
			}
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
	for reportIndex := range reports {
		pointCount := 0
		nonEmptyAnalyses := 0
		for _, analysis := range reports[reportIndex].Analyses {
			count := len(analysis.Points)
			pointCount += count
			if count != 0 {
				nonEmptyAnalyses++
			}
		}
		pointBudget := max(maxPersistedAnalysisPointsPerReport, 2*nonEmptyAnalyses)
		if pointCount > pointBudget {
			return "", fmt.Errorf(
				"reports[%d].analyses have %d points, exceeds canonical persistence budget %d",
				reportIndex,
				pointCount,
				pointBudget,
			)
		}
	}
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
	var reports []simmodel.Report
	if source.Reports != nil {
		reports = make([]simmodel.Report, len(source.Reports))
		for index := range source.Reports {
			// The evaluator and replay boundaries already applied the canonical
			// persistence projection. Cloning must preserve that exact transcript
			// rather than re-projecting it and invalidating its evidence hash.
			reports[index] = simmodel.CloneReport(source.Reports[index])
		}
	}
	return &SimulationEvidence{Resolution: cloneSimulationResolution(source.Resolution), Reports: reports}
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
	if source == nil {
		return nil
	}
	clone := make([]simmodel.Report, len(source))
	for reportIndex := range source {
		clone[reportIndex] = simmodel.CloneReportWithAnalysisPointLimit(
			source[reportIndex],
			persistedAnalysisPointLimit(source[reportIndex]),
		)
		retainAssertionObservables(&clone[reportIndex])
	}
	return clone
}

func persistedAnalysisPointLimit(report simmodel.Report) int {
	nonEmptyAnalyses := 0
	for _, analysis := range report.Analyses {
		if len(analysis.Points) != 0 {
			nonEmptyAnalyses++
		}
	}
	if nonEmptyAnalyses == 0 {
		return maxPersistedAnalysisPointsPerReport
	}
	return max(2, maxPersistedAnalysisPointsPerReport/nonEmptyAnalyses)
}

func retainAssertionObservables(report *simmodel.Report) {
	if report == nil {
		return
	}
	nodes := map[string]bool{}
	components := map[string]bool{}
	retain := func(assertion simmodel.AssertionResult) {
		if assertion.Node != "" {
			nodes[assertion.Node] = true
		}
		if assertion.ReferenceNode != "" {
			nodes[assertion.ReferenceNode] = true
		}
		if assertion.Component != "" {
			components[assertion.Component] = true
		}
		for _, component := range assertion.Components {
			if component != "" {
				components[component] = true
			}
		}
	}
	for _, assertion := range report.Assertions {
		retain(assertion)
	}
	for _, corner := range report.Corners {
		for _, assertion := range corner.Assertions {
			retain(assertion)
		}
	}
	for analysisIndex := range report.Analyses {
		for pointIndex := range report.Analyses[analysisIndex].Points {
			point := &report.Analyses[analysisIndex].Points[pointIndex]
			point.Nodes = slices.DeleteFunc(point.Nodes, func(node simmodel.NodeResult) bool {
				return !nodes[node.Node]
			})
			if len(point.Nodes) == 0 {
				point.Nodes = nil
			}
			point.Devices = slices.DeleteFunc(point.Devices, func(device simmodel.DeviceResult) bool {
				return !components[device.Component]
			})
			if len(point.Devices) == 0 {
				point.Devices = nil
			}
		}
	}
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
