package closedloopsynthesis

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

type dividerSimulationResolver struct{}

func (dividerSimulationResolver) ResolveSimulation(_ context.Context, state CandidateState) (SimulationResolution, error) {
	lower := 10_000.0
	for _, variable := range state.Variables {
		if variable.ID == "lower_resistance" {
			lower = variable.Value
		}
	}
	intent := simmodel.Intent{
		ModelID:    simmodel.ModelResistorDividerDCV1,
		Bindings:   []simmodel.Binding{{Role: "upper_resistor", Component: "r1"}, {Role: "lower_resistor", Component: "r2"}},
		Inputs:     []simmodel.NamedValue{{Name: "input_voltage_v", Value: 5}},
		Assertions: []simmodel.Assertion{{Metric: "output_voltage_v", Min: 2.45, Max: 2.55}},
	}
	components := []simmodel.ComponentEvidence{
		{InstanceID: "r1", CatalogID: "resistor.generic.0603", Family: "resistor", ValueSI: 10_000, HasValueSI: true, ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.ModelResistorDividerDCV1}}},
		{InstanceID: "r2", CatalogID: "resistor.generic.0603", Family: "resistor", ValueSI: lower, HasValueSI: true, ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.ModelResistorDividerDCV1}}},
	}
	plan, diagnostics := simmodel.Resolve(intent, "test-catalog", testHash("catalog"), components)
	if len(diagnostics) != 0 {
		return SimulationResolution{}, fmt.Errorf("resolve diagnostics: %#v", diagnostics)
	}
	return SimulationResolution{
		Plan:         plan,
		Measurements: []SimulationMeasurementLink{{RequirementID: "output", OperatingCase: "nominal", Assertion: 0}},
	}, nil
}

func TestSimModelEvaluatorRepairsThroughFreshTrustedResolution(t *testing.T) {
	requirement := closedLoopTestRequirement()
	minimum, maximum := 2.45, 2.55
	requirement.Requirements.OperatingCases[0].ID = "nominal"
	requirement.Requirements.BehavioralRequirements = []architecturesearch.BehavioralRequirement{{
		ID: "output", Metric: "dc_voltage", Analysis: simmodel.AnalysisDCOperatingPoint,
		Observation: architecturesearch.Observation{Kind: "port", ID: "output"}, Min: &minimum, Max: &maximum, Unit: "V", OperatingCases: []string{"nominal"}, Critical: true,
	}}
	input := Input{
		Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formula"), ModelRegistryHash: testHash("models"),
		Candidates: []Candidate{{
			Fingerprint: testHash("divider"),
			Variables: []Variable{{
				ID: "lower_resistance", Kind: "passive_value", Value: 5_000, AllowedValues: []float64{5_000, 10_000},
				Effects: []RepairEffect{{Analysis: simmodel.AnalysisDCOperatingPoint, Metric: "dc_voltage", Direction: RepairMetricIncreases}},
			}},
		}},
	}
	registry, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatalf("model provenance registry diagnostics: %#v", diagnostics)
	}
	evaluator := SimModelEvaluator{Resolver: dividerSimulationResolver{}, ProvenanceRegistry: registry}
	report := Run(context.Background(), input, evaluator, DefaultPolicy())
	if report.Status != "pass" || report.Selected == nil || report.Selected.State.Variables[0].Value != 10_000 {
		t.Fatalf("closed-loop simmodel report=%#v", report)
	}
	if report.Consumption.Evaluations != 2 || report.Consumption.RepairsApplied != 1 {
		t.Fatalf("consumption=%#v", report.Consumption)
	}
	if got := len(report.Candidates[0].Attempts[0].ModelDecisions); got != 2 {
		t.Fatalf("independently derived model decisions = %d, want 2", got)
	}
	replay := Run(context.Background(), input, evaluator, DefaultPolicy())
	if hashJSON(report) != hashJSON(replay) {
		t.Fatal("trusted simmodel closed-loop replay differs")
	}
}

func TestSimulationEvaluationCacheIsBoundedAndEvidenceNeutral(t *testing.T) {
	registry, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatalf("model provenance registry diagnostics: %#v", diagnostics)
	}
	cache := NewSimulationEvaluationCache()
	cache.limit = 1
	evaluator := SimModelEvaluator{
		Resolver: dividerSimulationResolver{}, ProvenanceRegistry: registry, Cache: cache,
	}
	state := CandidateState{
		Fingerprint: testHash("cached-divider"),
		Variables:   []Variable{{ID: "lower_resistance", Value: 10_000}},
	}
	first, err := evaluator.Evaluate(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	second, err := evaluator.Evaluate(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if hashJSON(first) != hashJSON(second) {
		t.Fatal("cached trusted simulation evidence differs")
	}
	first.Simulation.Reports[0].Status = "tampered"
	third, err := evaluator.Evaluate(context.Background(), state)
	if err != nil || third.Simulation.Reports[0].Status == "tampered" {
		t.Fatalf("cached report alias escaped: status=%q err=%v", third.Simulation.Reports[0].Status, err)
	}
	different := cloneState(state)
	different.Variables[0].Value = 5_000
	if _, err := evaluator.Evaluate(context.Background(), different); err != nil {
		t.Fatal(err)
	}
	if len(cache.entries) != 1 {
		t.Fatalf("cache entries=%d, want hard limit 1", len(cache.entries))
	}
}

func TestTrustedTranscriptCacheIsBoundedAndEvaluatorOnly(t *testing.T) {
	cache := newTrustedTranscriptCache(2)
	first, second, third := testHash("first"), testHash("second"), testHash("third")
	cache.remember(first)
	cache.remember(second)
	cache.remember(third)
	if cache.contains(first) || !cache.contains(second) || !cache.contains(third) {
		t.Fatalf("bounded transcript cache retained wrong hashes: first=%t second=%t third=%t", cache.contains(first), cache.contains(second), cache.contains(third))
	}
	cache.remember("not-a-hash")
	if len(cache.entries) != 2 {
		t.Fatalf("invalid transcript hash changed bounded cache size: %d", len(cache.entries))
	}
}

func TestSimModelEvaluatorFailsClosedForInvalidLinksAndStructuralDiagnostics(t *testing.T) {
	evaluator := SimModelEvaluator{Resolver: invalidSimulationResolver{}}
	if _, err := evaluator.Evaluate(context.Background(), CandidateState{Fingerprint: testHash("invalid")}); err == nil {
		t.Fatal("invalid simulation resolution was accepted")
	}
}

func TestSimModelEvaluatorFailsClosedWithoutIndependentProvenanceRegistry(t *testing.T) {
	evaluator := SimModelEvaluator{Resolver: dividerSimulationResolver{}}
	if _, err := evaluator.Evaluate(context.Background(), CandidateState{Fingerprint: testHash("missing-provenance")}); err == nil {
		t.Fatal("simulation without independent model provenance was accepted")
	}
}

func TestSimulationResolutionAllowsEquivalentRequirementsToShareEvidence(t *testing.T) {
	resolution, err := (dividerSimulationResolver{}).ResolveSimulation(context.Background(), CandidateState{})
	if err != nil {
		t.Fatalf("resolve simulation: %v", err)
	}
	resolution.Measurements = append(resolution.Measurements, SimulationMeasurementLink{
		RequirementID: "equivalent_output", OperatingCase: "nominal", Assertion: 0,
	})
	if diagnostics := validateSimulationResolution(resolution); len(diagnostics) != 0 {
		t.Fatalf("shared deterministic assertion evidence diagnostics = %#v", diagnostics)
	}
}

func TestReplaySimulationEvidenceRequiresExactDeterministicTranscript(t *testing.T) {
	registry, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatalf("model provenance registry diagnostics: %#v", diagnostics)
	}
	evaluation, err := (SimModelEvaluator{
		Resolver: dividerSimulationResolver{}, ProvenanceRegistry: registry,
	}).Evaluate(context.Background(), CandidateState{
		Fingerprint: testHash("divider"),
		Variables:   []Variable{{ID: "lower_resistance", Value: 10_000}},
	})
	if err != nil || evaluation.Simulation == nil {
		t.Fatalf("evaluation = %#v, err = %v", evaluation, err)
	}
	if !recentTrustedSimulationTranscripts.contains(evaluation.EvidenceHash) {
		t.Fatal("trusted evaluator did not register its exact transcript hash")
	}
	if replayDiagnostics := ReplaySimulationEvidence(*evaluation.Simulation); len(replayDiagnostics) != 0 {
		t.Fatalf("replay diagnostics = %#v", replayDiagnostics)
	}
	parallel := cloneSimulationEvidence(evaluation.Simulation)
	parallel.Resolution.Plans = []simmodel.Plan{
		simmodel.ClonePlan(parallel.Resolution.Plan),
		simmodel.ClonePlan(parallel.Resolution.Plan),
	}
	parallel.Resolution.Plan = simmodel.Plan{}
	parallel.Reports = []simmodel.Report{
		simmodel.CloneReport(parallel.Reports[0]),
		simmodel.CloneReport(parallel.Reports[0]),
	}
	if replayDiagnostics := ReplaySimulationEvidence(*parallel); len(replayDiagnostics) != 0 {
		t.Fatalf("ordered parallel replay diagnostics = %#v", replayDiagnostics)
	}
	evaluation.Simulation.Reports[0].Status = "tampered"
	if replayDiagnostics := ReplaySimulationEvidence(*evaluation.Simulation); len(replayDiagnostics) == 0 {
		t.Fatal("tampered simulation transcript replayed successfully")
	}
}

func TestSimulationEvidenceCompactsDenseAnalysesDeterministically(t *testing.T) {
	if cloneSimulationReports(nil) != nil {
		t.Fatal("nil simulation reports did not retain canonical nil representation")
	}
	points := make([]simmodel.AnalysisPoint, 2000)
	for index := range points {
		points[index] = simmodel.AnalysisPoint{TimeS: float64(index)}
	}
	reports := []simmodel.Report{{
		Analyses: []simmodel.AnalysisResult{{ID: "transient", Kind: simmodel.AnalysisTransient, Points: points}},
	}}
	compact := cloneSimulationReports(reports)
	got := compact[0].Analyses[0].Points
	if len(got) != maxPersistedAnalysisPointsPerReport {
		t.Fatalf("persisted points = %d, want %d", len(got), maxPersistedAnalysisPointsPerReport)
	}
	if got[0].TimeS != 0 || got[len(got)-1].TimeS != 1999 {
		t.Fatalf("persisted point endpoints = %g..%g, want 0..1999", got[0].TimeS, got[len(got)-1].TimeS)
	}
	if _, err := simulationEvidenceHash(SimulationResolution{}, reports); err == nil {
		t.Fatal("dense reports bypassed the canonical persistence boundary")
	}
	compactHash, err := simulationEvidenceHash(SimulationResolution{}, compact)
	if err != nil {
		t.Fatalf("hash compact evidence: %v", err)
	}
	replayedCompactHash, err := simulationEvidenceHash(SimulationResolution{}, cloneSimulationReports(reports))
	if err != nil || replayedCompactHash != compactHash {
		t.Fatalf("canonical replay hash = %s, %v; want %s", replayedCompactHash, err, compactHash)
	}
	clonedEvidence := cloneSimulationEvidence(&SimulationEvidence{Reports: compact})
	clonedHash, err := HashSimulationEvidence(*clonedEvidence)
	if err != nil || clonedHash != compactHash {
		t.Fatalf("cloned persisted evidence hash = %s, %v; want %s", clonedHash, err, compactHash)
	}
	compact[0].Analyses[0].Points[1].TimeS++
	tamperedHash, err := simulationEvidenceHash(SimulationResolution{}, compact)
	if err != nil {
		t.Fatalf("hash tampered evidence: %v", err)
	}
	if tamperedHash == compactHash {
		t.Fatal("tampered persisted point did not change canonical evidence hash")
	}
}

func TestSimulationEvidenceCompactsDenseAnalysesToReportBudgetDeterministically(t *testing.T) {
	const analysisCount = 20
	points := make([]simmodel.AnalysisPoint, 2000)
	for index := range points {
		points[index] = simmodel.AnalysisPoint{TimeS: float64(index)}
	}
	report := simmodel.Report{Analyses: make([]simmodel.AnalysisResult, analysisCount)}
	for index := range report.Analyses {
		report.Analyses[index] = simmodel.AnalysisResult{
			ID:     fmt.Sprintf("transient_%02d", index),
			Points: points,
		}
	}

	compact := cloneSimulationReports([]simmodel.Report{report})
	wantPerAnalysis := max(2, maxPersistedAnalysisPointsPerReport/analysisCount)
	totalPoints := 0
	for index := range compact[0].Analyses {
		got := compact[0].Analyses[index].Points
		totalPoints += len(got)
		if len(got) != wantPerAnalysis {
			t.Fatalf("analysis %d persisted points = %d, want %d", index, len(got), wantPerAnalysis)
		}
		if got[0].TimeS != 0 || got[len(got)-1].TimeS != 1999 {
			t.Fatalf("analysis %d persisted point endpoints = %g..%g, want 0..1999", index, got[0].TimeS, got[len(got)-1].TimeS)
		}
	}
	if totalPoints > maxPersistedAnalysisPointsPerReport {
		t.Fatalf("persisted report points = %d, budget %d", totalPoints, maxPersistedAnalysisPointsPerReport)
	}
	if _, err := simulationEvidenceHash(SimulationResolution{}, []simmodel.Report{report}); err == nil {
		t.Fatal("dense multi-analysis report bypassed the canonical persistence boundary")
	}
	compactHash, err := simulationEvidenceHash(SimulationResolution{}, compact)
	if err != nil {
		t.Fatalf("hash compact multi-analysis report: %v", err)
	}
	replayed := cloneSimulationReports([]simmodel.Report{report})
	replayedHash, err := simulationEvidenceHash(SimulationResolution{}, replayed)
	if err != nil || replayedHash != compactHash {
		t.Fatalf("multi-analysis persistence replay hash = %s, %v; want %s", replayedHash, err, compactHash)
	}
}

func TestSimulationEvidenceRetainsOnlyAssertionObservables(t *testing.T) {
	reports := []simmodel.Report{{
		Assertions: []simmodel.AssertionResult{{
			Node: "output", ReferenceNode: "reference", Component: "switch",
			Components: []string{"sense_a", "sense_b"},
		}},
		Analyses: []simmodel.AnalysisResult{{Points: []simmodel.AnalysisPoint{
			{
				Nodes: []simmodel.NodeResult{
					{Node: "unused"}, {Node: "output"}, {Node: "reference"},
				},
				Devices: []simmodel.DeviceResult{
					{Component: "unused"}, {Component: "switch"},
					{Component: "sense_a"}, {Component: "sense_b"},
				},
			},
			{
				Nodes:   []simmodel.NodeResult{{Node: "unused"}},
				Devices: []simmodel.DeviceResult{{Component: "unused"}},
			},
		}}},
	}}
	compact := cloneSimulationReports(reports)
	point := compact[0].Analyses[0].Points[0]
	if got, want := []string{point.Nodes[0].Node, point.Nodes[1].Node}, []string{"output", "reference"}; !slices.Equal(got, want) {
		t.Fatalf("persisted nodes = %#v, want %#v", got, want)
	}
	gotComponents := make([]string, 0, len(point.Devices))
	for _, device := range point.Devices {
		gotComponents = append(gotComponents, device.Component)
	}
	if want := []string{"switch", "sense_a", "sense_b"}; !slices.Equal(gotComponents, want) {
		t.Fatalf("persisted devices = %#v, want %#v", gotComponents, want)
	}
	if len(reports[0].Analyses[0].Points[0].Nodes) != 3 || len(reports[0].Analyses[0].Points[0].Devices) != 4 {
		t.Fatal("persistence projection mutated the full simulation report")
	}
	empty := compact[0].Analyses[0].Points[1]
	if empty.Nodes != nil || empty.Devices != nil {
		t.Fatalf("empty projected observables are not canonical nil slices: %#v", empty)
	}
	evidence := &SimulationEvidence{Reports: compact}
	before, err := HashSimulationEvidence(*evidence)
	if err != nil {
		t.Fatal(err)
	}
	after, err := HashSimulationEvidence(*cloneSimulationEvidence(evidence))
	if err != nil || after != before {
		t.Fatalf("projected evidence clone hash = %s, %v; want %s", after, err, before)
	}
}

func TestWorstLinkedAssertionSelectsWorstCornerDeterministically(t *testing.T) {
	plan := simmodel.Plan{Assertions: []simmodel.Assertion{{Min: 4.5, Max: 5.5}, {Min: 4.5, Max: 5.5}, {Min: 4.5, Max: 5.5}}}
	report := simmodel.Report{
		Assertions: []simmodel.AssertionResult{{Actual: 5}, {Actual: 4.6}, {Actual: 5.4}},
		Corners: []simmodel.CornerResult{
			{ID: "nominal", Assertions: []simmodel.AssertionResult{{Actual: 5}, {Actual: 4.6}, {Actual: 5.4}}},
			{ID: "minimum", Assertions: []simmodel.AssertionResult{{Actual: 4.4}, {Actual: 5}, {Actual: 5}}},
		},
	}
	worst, err := worstLinkedAssertion(plan, report, []int{0, 1, 2})
	if err != nil || worst.Actual != 4.4 {
		t.Fatalf("worst linked assertion = %#v err=%v", worst, err)
	}
	if diagnostics := validateSimulationResolution(SimulationResolution{Plan: simmodel.Plan{}, Measurements: []SimulationMeasurementLink{{RequirementID: "r", OperatingCase: "c", Assertions: []int{1, 0}}}}); len(diagnostics) == 0 {
		t.Fatal("non-canonical aggregate assertion link was accepted")
	}
}

func TestWorstLinkedMeasurementSpansDeterministicPlanBatches(t *testing.T) {
	plans := []simmodel.Plan{
		{Assertions: []simmodel.Assertion{{Min: 4.5, Max: 5.5}}},
		{Assertions: []simmodel.Assertion{{Min: 4.5, Max: 5.5}}},
	}
	reports := []simmodel.Report{
		{Assertions: []simmodel.AssertionResult{{Min: 4.5, Max: 5.5, Actual: 5}}},
		{Assertions: []simmodel.AssertionResult{{Min: 4.5, Max: 5.5, Actual: 4.6}}},
	}
	link := SimulationMeasurementLink{Evidence: []SimulationAssertionSet{{Plan: 0, Assertions: []int{0}}, {Plan: 1, Assertions: []int{0}}}}
	worst, err := worstLinkedMeasurement(plans, reports, link)
	if err != nil || worst.Actual != 4.6 {
		t.Fatalf("worst batched assertion = %#v err=%v", worst, err)
	}
}

func TestOnlyAssertionFailuresRecognizesWorstCaseAssertionDiagnostics(t *testing.T) {
	report := simmodel.Report{Assertions: []simmodel.AssertionResult{{Pass: false}}}
	if !onlyAssertionFailures(report, []simmodel.Diagnostic{{Path: "assertions.bandwidth", Message: "measured 90000 is outside trusted bounds 100000..1e+12"}}) {
		t.Fatal("nominal measured assertion failure was treated as a model execution failure")
	}
	if !onlyAssertionFailures(report, []simmodel.Diagnostic{{Path: "worst_case.devices.r1.value_si=900", Message: "worst-case corner devices.r1.value_si=900 measured 8.5 outside trusted bounds 9..11"}}) {
		t.Fatal("worst-case assertion failure was treated as a model execution failure")
	}
	if onlyAssertionFailures(report, []simmodel.Diagnostic{{Path: "assertions.bandwidth", Message: "solved AC sweep does not bracket the -3 dB cutoff"}}) {
		t.Fatal("unavailable derived measurement was treated as numeric assertion evidence")
	}
	if onlyAssertionFailures(report, []simmodel.Diagnostic{{Path: "worst_case", Message: "corner could not be evaluated"}}) {
		t.Fatal("worst-case execution failure was treated as an assertion failure")
	}
}

type invalidSimulationResolver struct{}

func (invalidSimulationResolver) ResolveSimulation(context.Context, CandidateState) (SimulationResolution, error) {
	return SimulationResolution{Measurements: []SimulationMeasurementLink{{RequirementID: "x", OperatingCase: "y", Assertion: 7}}}, nil
}
