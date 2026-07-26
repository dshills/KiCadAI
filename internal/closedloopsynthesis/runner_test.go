package closedloopsynthesis

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/simmodel"
)

type evaluatorFunc func(context.Context, CandidateState) (Evaluation, error)

func (function evaluatorFunc) Evaluate(ctx context.Context, state CandidateState) (Evaluation, error) {
	return function(ctx, state)
}

func TestClosedLoopRepairsByStrictWholeReportImprovementAndReplays(t *testing.T) {
	requirement := closedLoopTestRequirement()
	candidate := Candidate{Fingerprint: testHash("candidate-a"), Variables: []Variable{{ID: "gain_resistance", Kind: "gain", Value: 1, AllowedValues: []float64{1, 2, 3}, Effects: []RepairEffect{{Analysis: simmodel.AnalysisACSweep, Metric: "voltage_gain", Direction: RepairMetricIncreases}}}}}
	evaluator := closedLoopTestEvaluator(false)
	input := Input{Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formulas"), ModelRegistryHash: testHash("models"), Candidates: []Candidate{candidate}}
	first := Run(context.Background(), input, evaluator, DefaultPolicy())
	if first.Status != "pass" || first.StopReason != StopPassed || first.Selected == nil {
		t.Fatalf("closed loop = %#v", first)
	}
	if len(first.Candidates) != 1 || len(first.Candidates[0].Repairs) != 1 || first.Selected.State.Variables[0].Value != 2 {
		t.Fatalf("repair evidence = %#v", first.Candidates)
	}
	if first.Consumption.Evaluations != 3 || first.Consumption.RepairTrials != 2 || first.Consumption.RepairsApplied != 1 {
		t.Fatalf("consumption = %#v", first.Consumption)
	}
	repair := first.Candidates[0].Repairs[0]
	if repair.RequirementID != "gain" || repair.Analysis != simmodel.AnalysisACSweep || repair.Metric != "voltage_gain" || repair.Direction != "increase" || repair.AllowedMinimum != 1 || repair.AllowedMaximum != 3 {
		t.Fatalf("typed repair authorization evidence = %#v", repair)
	}
	if diagnostics := ValidatePromotionReport(first, input.CatalogHash); len(diagnostics) != 0 {
		t.Fatalf("passing report promotion diagnostics = %#v", diagnostics)
	}
	tampered := CloneReport(first)
	tampered.Selected.State.Variables[0].Value = 3
	if diagnostics := ValidatePromotionReport(tampered, input.CatalogHash); len(diagnostics) == 0 {
		t.Fatal("tampered selected state was accepted for promotion")
	}

	reordered := input
	reorderedCandidates, err := cloneCandidates(input.Candidates)
	if err != nil {
		t.Fatal(err)
	}
	reordered.Candidates = reorderedCandidates
	slices.Reverse(reordered.Candidates[0].Variables[0].AllowedValues)
	// Unsorted allowed values are rejected rather than silently normalized.
	invalid := Run(context.Background(), reordered, evaluator, DefaultPolicy())
	if invalid.Status != "blocked" || invalid.StopReason != StopInvalidInput {
		t.Fatalf("noncanonical repair domain was accepted: %#v", invalid)
	}
	second := Run(context.Background(), input, evaluator, DefaultPolicy())
	firstBytes, err := MarshalReport(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := MarshalReport(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Fatalf("closed-loop replay differs\nfirst: %s\nsecond: %s", firstBytes, secondBytes)
	}
}

func TestCloneSystemPlanFailsWithoutReturningAliasedStorage(t *testing.T) {
	capacity := 1.0
	source := &architecturesearch.SystemPlan{
		Resources: []architecturesearch.SharedResourcePlan{{
			ID: "rail", Consumers: []string{"block_a"}, CapacityA: &capacity,
		}},
	}
	cloned, err := cloneSystemPlan(source)
	if err != nil {
		t.Fatal(err)
	}
	cloned.Resources[0].Consumers[0] = "block_b"
	*cloned.Resources[0].CapacityA = 2
	if source.Resources[0].Consumers[0] != "block_a" || *source.Resources[0].CapacityA != 1 {
		t.Fatalf("clone shares nested storage with source: source=%#v clone=%#v", source, cloned)
	}

	invalid := math.NaN()
	source.Resources[0].CapacityA = &invalid
	if cloned, err := cloneSystemPlan(source); err == nil || cloned != nil {
		t.Fatalf("non-JSON system plan clone = %#v, %v; want explicit failure", cloned, err)
	}
}

func TestV4ClosedLoopBacktracksAcrossCompleteArchitecturesWithBlockAndEndToEndEvidence(t *testing.T) {
	requirement := closedLoopTestRequirement()
	requirement.Schema = architecturesearch.SchemaIDV4
	requirement.Version = architecturesearch.VersionV4
	requirement.Acceptance.RequireHierarchicalDecomposition = true
	requirement.Acceptance.RequireInterfaceContracts = true
	requirement.Acceptance.RequireSharedResourcePlanning = true
	requirement.Acceptance.RequireDeterministicBacktracking = true
	requirement.Acceptance.RequirePhysicalPartitioning = true
	requirement.Acceptance.RequireEndToEndTraceability = true
	firstFingerprint := testHash("v4-first")
	secondFingerprint := testHash("v4-second")
	input := Input{
		Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formulas"), ModelRegistryHash: testHash("models"),
		Candidates: []Candidate{
			{Fingerprint: firstFingerprint, Priority: 0, SystemPlan: closedLoopV4SystemPlan(t, requirement, firstFingerprint)},
			{Fingerprint: secondFingerprint, Priority: 1, SystemPlan: closedLoopV4SystemPlan(t, requirement, secondFingerprint)},
		},
	}
	evaluator := evaluatorFunc(func(ctx context.Context, state CandidateState) (Evaluation, error) {
		evaluation, err := closedLoopTestEvaluator(false).Evaluate(ctx, state)
		if err == nil && state.Fingerprint == firstFingerprint {
			evaluation.Measurements[0].Actual = 5
		}
		return evaluation, err
	})
	report := Run(context.Background(), input, evaluator, DefaultPolicy())
	if report.Status != "pass" || report.Selected == nil || report.Selected.Fingerprint != secondFingerprint ||
		report.Selected.SystemPlanHash != input.Candidates[1].SystemPlan.PlanHash {
		t.Fatalf("V4 global backtracking selection = %#v", report)
	}
	if report.Backtracking == nil || !report.Backtracking.Deterministic ||
		len(report.Backtracking.Candidates) != 2 ||
		report.Backtracking.Candidates[0].Fingerprint != firstFingerprint ||
		report.Backtracking.Candidates[0].Status != "rejected" ||
		report.Backtracking.Candidates[1].Fingerprint != secondFingerprint ||
		report.Backtracking.Candidates[1].Status != "pass" ||
		report.Backtracking.SelectedFingerprint != secondFingerprint {
		t.Fatalf("V4 backtracking evidence = %#v", report.Backtracking)
	}
	for _, candidate := range report.Candidates {
		if len(candidate.Attempts) == 0 || candidate.Attempts[0].Hierarchy == nil ||
			len(candidate.Attempts[0].Hierarchy.Blocks) != 1 ||
			len(candidate.Attempts[0].Hierarchy.EndToEnd) != len(requirement.Requirements.BehavioralRequirements) {
			t.Fatalf("V4 hierarchical verification evidence = %#v", candidate)
		}
	}
	replay := Run(context.Background(), input, evaluator, DefaultPolicy())
	firstBytes, err := MarshalReport(report)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := MarshalReport(replay)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(firstBytes, secondBytes) {
		t.Fatal("V4 closed-loop backtracking replay differs")
	}
}

func TestV5ClosedLoopRetainsHierarchicalBacktrackingAndDynamicRequirementIdentity(t *testing.T) {
	requirement := closedLoopTestRequirement()
	requirement.Schema = architecturesearch.SchemaIDV5
	requirement.Version = architecturesearch.VersionV5
	requirement.Acceptance.RequireHierarchicalDecomposition = true
	requirement.Acceptance.RequireInterfaceContracts = true
	requirement.Acceptance.RequireSharedResourcePlanning = true
	requirement.Acceptance.RequireDeterministicBacktracking = true
	requirement.Acceptance.RequirePhysicalPartitioning = true
	requirement.Acceptance.RequireEndToEndTraceability = true
	requirement.Acceptance.RequireDynamicModelProvenance = true
	requirement.Acceptance.RequireReturnRatioEvidence = true
	requirement.Acceptance.RequireDynamicElectrothermalEvidence = true
	requirement.Acceptance.RequireEventCoverage = true
	requirement.Acceptance.RequireDynamicArchitectureSelection = true
	requirement.Acceptance.RequireBoundedDynamicRepair = true
	applied := 1.0
	requirement.Requirements.OperatingCases[0].Events = []architecturesearch.OperatingEvent{{
		ID: "startup", Kind: "startup", Target: architecturesearch.Observation{Kind: "port", ID: "output"},
		DurationS: 1e-3, Applied: &applied, Unit: "V",
	}}
	fingerprint := testHash("v5-candidate")
	input := Input{
		Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formulas"), ModelRegistryHash: testHash("models"),
		Candidates: []Candidate{{Fingerprint: fingerprint, Priority: 0, SystemPlan: closedLoopV4SystemPlan(t, requirement, fingerprint)}},
	}
	report := Run(context.Background(), input, closedLoopTestEvaluator(false), DefaultPolicy())
	if report.Status != "pass" || report.Selected == nil || report.Selected.Fingerprint != fingerprint ||
		report.Backtracking == nil || report.Backtracking.SelectedFingerprint != fingerprint ||
		report.Candidates[0].Attempts[0].Hierarchy == nil {
		t.Fatalf("V5 closed-loop hierarchy/backtracking = %#v", report)
	}
}

func TestV5DynamicEvidenceRejectsStaticFavoriteAndSelectsSafeAlternative(t *testing.T) {
	requirement := closedLoopV5DynamicRequirement()

	preferred := testHash("v5-static-favorite")
	safe := testHash("v5-dynamic-safe")
	input := Input{
		Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formulas"), ModelRegistryHash: testHash("models"),
		Candidates: []Candidate{
			{Fingerprint: preferred, Priority: 0, SystemPlan: closedLoopV4SystemPlan(t, requirement, preferred)},
			{Fingerprint: safe, Priority: 1, SystemPlan: closedLoopV4SystemPlan(t, requirement, safe)},
		},
	}
	evaluator := evaluatorFunc(func(_ context.Context, state CandidateState) (Evaluation, error) {
		temperature := 80.0
		if state.Fingerprint == preferred {
			temperature = 140
		}
		simulation := &SimulationEvidence{}
		evidenceHash, _ := HashSimulationEvidence(*simulation)
		return Evaluation{
			EvidenceHash: evidenceHash, Simulation: simulation,
			Measurements: []Measurement{
				{RequirementID: "gain", OperatingCase: "rated", Actual: 10},
				{RequirementID: "thermal", OperatingCase: "rated", Actual: temperature},
			},
			ModelDecisions: []ModelDecision{closedLoopDynamicModelDecision()},
		}, nil
	})
	report := Run(context.Background(), input, evaluator, DefaultPolicy())
	if report.Status != "pass" || report.Selected == nil || report.Selected.Fingerprint != safe ||
		report.Candidates[0].Status != "rejected" || report.Candidates[1].Status != "pass" {
		t.Fatalf("V5 dynamic architecture selection = %#v", report)
	}
}

func TestV5DynamicRepairIsBoundedImmutableAndDeterministic(t *testing.T) {
	requirement := closedLoopV5DynamicRequirement()
	requirementBefore, err := architecturesearch.CanonicalHash(requirement)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := testHash("v5-dynamic-repair")
	input := Input{
		Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formulas"), ModelRegistryHash: testHash("models"),
		Candidates: []Candidate{{
			Fingerprint: fingerprint,
			Priority:    0,
			SystemPlan:  closedLoopV4SystemPlan(t, requirement, fingerprint),
			Variables: []Variable{{
				ID: "heatsink_thermal_resistance", Kind: "thermal_path", Value: 1,
				AllowedValues: []float64{1, 2, 3},
				Effects: []RepairEffect{{
					Analysis: simmodel.AnalysisElectrothermal, Metric: "peak_junction_temperature", Direction: RepairMetricDecreases,
				}},
			}},
		}},
	}
	evaluator := evaluatorFunc(func(_ context.Context, state CandidateState) (Evaluation, error) {
		temperature := 170 - 30*state.Variables[0].Value
		simulation := &SimulationEvidence{}
		evidenceHash, _ := HashSimulationEvidence(*simulation)
		return Evaluation{
			EvidenceHash: evidenceHash,
			Simulation:   simulation,
			Measurements: []Measurement{
				{RequirementID: "gain", OperatingCase: "rated", Actual: 10},
				{RequirementID: "thermal", OperatingCase: "rated", Actual: temperature},
			},
			ModelDecisions: []ModelDecision{closedLoopDynamicModelDecision()},
		}, nil
	})

	first := Run(context.Background(), input, evaluator, DefaultPolicy())
	if first.Status != "pass" || first.Selected == nil || first.Selected.State.Variables[0].Value != 3 ||
		len(first.Candidates[0].Repairs) != 1 || first.Candidates[0].Repairs[0].Kind != "thermal_path" {
		t.Fatalf("bounded dynamic repair = %#v", first)
	}
	requirementAfter, err := architecturesearch.CanonicalHash(input.Requirement)
	if err != nil {
		t.Fatal(err)
	}
	if requirementAfter != requirementBefore || *input.Requirement.Requirements.BehavioralRequirements[1].Max != 100 {
		t.Fatalf("dynamic repair changed immutable requirement: before=%s after=%s requirement=%#v", requirementBefore, requirementAfter, input.Requirement)
	}
	second := Run(context.Background(), input, evaluator, DefaultPolicy())
	firstBytes, err := MarshalReport(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := MarshalReport(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Fatalf("dynamic repair replay differs\nfirst: %s\nsecond: %s", firstBytes, secondBytes)
	}
}

func closedLoopV4SystemPlan(t *testing.T, requirement architecturesearch.Requirement, fingerprint string) *architecturesearch.SystemPlan {
	t.Helper()
	requirementHash, err := architecturesearch.CanonicalHash(requirement)
	if err != nil {
		t.Fatal(err)
	}
	plan := architecturesearch.SystemPlan{
		Schema: architecturesearch.SystemPlanSchema, RequirementHash: requirementHash, CandidateFingerprint: fingerprint,
		Hierarchy: architecturesearch.HierarchyPlan{
			Root: architecturesearch.HierarchyNode{ID: "system", Kind: "system", Children: []string{"subsystem_domain_ground"}, Domains: []string{"ground", "supply"}},
			Subsystems: []architecturesearch.HierarchyNode{{
				ID: "subsystem_domain_ground", Kind: "subsystem", ParentID: "system",
				Children: []string{"block_objective_amplify"}, Domains: []string{"ground", "supply"}, Classifications: []string{"analog"},
			}},
			Blocks: []architecturesearch.HierarchyBlock{{
				ID: "block_objective_amplify", ParentID: "subsystem_domain_ground",
				ObligationPath: "objective:amplify", Capability: "signal_amplification",
				RequirementKind: "objective", RequirementID: "amplify", Domains: []string{"ground", "supply"},
				Classifications: []string{"analog"}, InterfaceIDs: []string{"interface_output"},
				RequiredBehaviorIDs: []string{"gain", "thermal"},
				VerificationIDs:     []string{"behavior:gain", "behavior:thermal", "contract:interface_output"},
			}},
		},
		Interfaces: []architecturesearch.InterfaceContractPlan{{
			ID: "interface_output", Anchor: "external:output", Kind: "analog_voltage", Domain: "ground",
			Endpoints: []architecturesearch.InterfaceEndpoint{
				{BlockID: "block_objective_amplify", ObligationPath: "objective:amplify", Role: "output", Direction: "source"},
				{BlockID: "system", Role: "requirement_boundary", Direction: "source"},
			},
			Evidence: architecturesearch.ContractEvidence{Confidence: architecturesearch.EvidenceVerified, Sources: []string{"test:v4-system-plan"}},
			Status:   "pass",
		}},
		Resources: []architecturesearch.SharedResourcePlan{{
			ID: "resource_domain_supply", Kind: "supply", Domain: "supply", Source: "external:supply",
			Consumers: []string{"block_objective_amplify"},
			Evidence:  architecturesearch.ContractEvidence{Confidence: architecturesearch.EvidenceRuleInferred, Sources: []string{"test:v4-system-plan"}},
			Status:    "pass",
		}},
		Physical: architecturesearch.PhysicalPlan{Partitions: []architecturesearch.PhysicalPartition{{
			ID: "partition_domain_ground", SubsystemID: "subsystem_domain_ground",
			BlockIDs: []string{"block_objective_amplify"}, Classifications: []string{"analog"},
			Rules: []string{"keep_owned_components_within_partition", "preserve_continuous_reference_return"},
		}}},
		Traceability: []architecturesearch.SystemTraceabilityRecord{
			{RequirementKind: "objective", RequirementID: "amplify", BlockIDs: []string{"block_objective_amplify"}, InterfaceIDs: []string{"interface_output"}, ResourceIDs: []string{"resource_domain_supply"}, BehavioralRequirementIDs: []string{"gain", "thermal"}},
			{RequirementKind: "behavior", RequirementID: "gain", BlockIDs: []string{"block_objective_amplify"}, InterfaceIDs: []string{"interface_output"}, ResourceIDs: []string{"resource_domain_supply"}, BehavioralRequirementIDs: []string{"gain"}},
			{RequirementKind: "behavior", RequirementID: "thermal", BlockIDs: []string{"block_objective_amplify"}, InterfaceIDs: []string{"interface_output"}, ResourceIDs: []string{"resource_domain_supply"}, BehavioralRequirementIDs: []string{"thermal"}},
		},
	}
	hashInput := plan
	plan.PlanHash = hashJSON(hashInput)
	if err := architecturesearch.ValidateSystemPlan(requirement, fingerprint, plan); err != nil {
		t.Fatal(err)
	}
	return &plan
}

func TestClosedLoopCoordinatesTwoVariablesWhenSinglesRegressWholeReport(t *testing.T) {
	requirement := closedLoopTestRequirement()
	minimumCombined, balanceMinimum, balanceMaximum := 4.0, -0.1, 0.1
	requirement.Requirements.BehavioralRequirements = []architecturesearch.BehavioralRequirement{
		{ID: "gain", Metric: "voltage_gain", Analysis: simmodel.AnalysisACSweep, Observation: architecturesearch.Observation{Kind: "port", ID: "output"}, Min: &minimumCombined, Unit: "ratio", OperatingCases: []string{"rated"}},
		{ID: "bandwidth", Metric: "bandwidth", Analysis: simmodel.AnalysisACSweep, Observation: architecturesearch.Observation{Kind: "port", ID: "output"}, Min: &minimumCombined, Unit: "Hz", OperatingCases: []string{"rated"}},
		{ID: "balance", Metric: "phase_margin", Analysis: simmodel.AnalysisStability, Observation: architecturesearch.Observation{Kind: "port", ID: "output"}, Min: &balanceMinimum, Max: &balanceMaximum, Unit: "deg", OperatingCases: []string{"rated"}},
	}
	effects := []RepairEffect{
		{Analysis: simmodel.AnalysisACSweep, Metric: "bandwidth", Direction: RepairMetricIncreases},
		{Analysis: simmodel.AnalysisACSweep, Metric: "voltage_gain", Direction: RepairMetricIncreases},
	}
	candidate := Candidate{Fingerprint: testHash("coordinated"), Variables: []Variable{
		{ID: "collector", Kind: "gain", Value: 1, AllowedValues: []float64{1, 2}, Effects: effects},
		{ID: "emitter", Kind: "gain", Value: 1, AllowedValues: []float64{1, 2}, Effects: effects},
	}}
	evaluator := evaluatorFunc(func(_ context.Context, state CandidateState) (Evaluation, error) {
		x, y := state.Variables[0].Value, state.Variables[1].Value
		simulation := &SimulationEvidence{}
		evidenceHash, _ := HashSimulationEvidence(*simulation)
		return Evaluation{
			EvidenceHash: evidenceHash,
			Measurements: []Measurement{
				{RequirementID: "gain", OperatingCase: "rated", Actual: x + y},
				{RequirementID: "bandwidth", OperatingCase: "rated", Actual: x + y},
				{RequirementID: "balance", OperatingCase: "rated", Actual: x - y},
			},
			Simulation: simulation,
			ModelDecisions: []ModelDecision{{
				Component: "network", Family: "resistor", Status: "used", Reason: "trusted coordinated repair test",
				RequiredAnalyses: []string{simmodel.AnalysisACSweep, simmodel.AnalysisStability}, Claim: simmodel.CatalogEvidence{ModelID: simmodel.PrimitiveResistorV1},
				Provenance: &simmodel.ModelProvenance{Source: "manufacturer-datasheet:test", Revision: "rev-a", SHA256: testHash("coordinated-model"), ReviewStatus: "reviewed", AllowedAnalyses: []string{simmodel.AnalysisACSweep, simmodel.AnalysisStability}},
			}},
		}, nil
	})
	input := Input{Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formulas"), ModelRegistryHash: testHash("models"), Candidates: []Candidate{candidate}}
	report := Run(context.Background(), input, evaluator, DefaultPolicy())
	if report.Status != "pass" || report.Selected == nil || len(report.Candidates[0].Repairs) != 1 {
		t.Fatalf("coordinated closed loop = %#v", report)
	}
	repair := report.Candidates[0].Repairs[0]
	if len(repair.Changes) != 2 || repair.Changes[0].Variable != "collector" || repair.Changes[1].Variable != "emitter" || report.Selected.State.Variables[0].Value != 2 || report.Selected.State.Variables[1].Value != 2 {
		t.Fatalf("coordinated repair evidence = %#v selected=%#v", repair, report.Selected.State)
	}
}

func TestClosedLoopDoesNotTrialVariableWithoutMatchingAuthorizedEffect(t *testing.T) {
	requirement := closedLoopTestRequirement()
	candidate := Candidate{Fingerprint: testHash("candidate"), Variables: []Variable{{
		ID: "temperature_only", Kind: "bias", Value: 1, AllowedValues: []float64{1, 2},
		Effects: []RepairEffect{{Analysis: simmodel.AnalysisThermal, Metric: "junction_temperature", Direction: RepairMetricDecreases}},
	}}}
	input := Input{Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formulas"), ModelRegistryHash: testHash("models"), Candidates: []Candidate{candidate}}
	report := Run(context.Background(), input, closedLoopTestEvaluator(false), DefaultPolicy())
	if report.Status != "blocked" || report.Candidates[0].StopReason != StopUnsupportedDiagnosis || report.Consumption.Evaluations != 1 || report.Consumption.RepairTrials != 0 {
		t.Fatalf("unauthorized repair trial was not rejected before evaluation: %#v", report)
	}
}

func TestClosedLoopRejectsMissingModelTrustAndIncompleteAssertions(t *testing.T) {
	requirement := closedLoopTestRequirement()
	input := Input{Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formulas"), ModelRegistryHash: testHash("models"), Candidates: []Candidate{{Fingerprint: testHash("candidate")}}}
	missingTrust := Run(context.Background(), input, evaluatorFunc(func(_ context.Context, state CandidateState) (Evaluation, error) {
		return Evaluation{EvidenceHash: stateHash(state), Measurements: closedLoopMeasurements(10, 80)}, nil
	}), DefaultPolicy())
	if missingTrust.Status != "blocked" || missingTrust.Candidates[0].StopReason != StopModelTrustFailed {
		t.Fatalf("missing trust did not fail closed: %#v", missingTrust)
	}
	incomplete := Run(context.Background(), input, evaluatorFunc(func(_ context.Context, state CandidateState) (Evaluation, error) {
		evaluation, _ := closedLoopTestEvaluator(false).Evaluate(context.Background(), state)
		evaluation.Measurements = evaluation.Measurements[:1]
		return evaluation, nil
	}), DefaultPolicy())
	if incomplete.Status != "blocked" || incomplete.Candidates[0].StopReason != StopAssertionIncomplete {
		t.Fatalf("incomplete assertion coverage did not fail closed: %#v", incomplete)
	}
}

func TestClosedLoopWillNotTradeCriticalThermalFailureForGainRepair(t *testing.T) {
	requirement := closedLoopTestRequirement()
	candidate := Candidate{Fingerprint: testHash("candidate"), Variables: []Variable{{ID: "gain_resistance", Kind: "gain", Value: 1, AllowedValues: []float64{1, 2, 3}, Effects: []RepairEffect{{Analysis: simmodel.AnalysisACSweep, Metric: "voltage_gain", Direction: RepairMetricIncreases}, {Analysis: simmodel.AnalysisThermal, Metric: "junction_temperature", Direction: RepairMetricIncreases}}}}}
	input := Input{Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formulas"), ModelRegistryHash: testHash("models"), Candidates: []Candidate{candidate}}
	report := Run(context.Background(), input, closedLoopTestEvaluator(true), DefaultPolicy())
	if report.Status != "blocked" || report.Candidates[0].StopReason != StopNonImprovement || len(report.Candidates[0].Repairs) != 0 {
		t.Fatalf("unsafe tradeoff was accepted: %#v", report)
	}
}

func TestClosedLoopBoundsEvaluationErrorsAndBudget(t *testing.T) {
	requirement := closedLoopTestRequirement()
	input := Input{Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formulas"), ModelRegistryHash: testHash("models"), Candidates: []Candidate{{Fingerprint: testHash("candidate")}}}
	failure := Run(context.Background(), input, evaluatorFunc(func(context.Context, CandidateState) (Evaluation, error) {
		return Evaluation{}, errors.New("solver failed")
	}), DefaultPolicy())
	if failure.Candidates[0].StopReason != StopEvaluationFailed {
		t.Fatalf("evaluation error = %#v", failure)
	}
	policy := DefaultPolicy()
	policy.MaxEvaluations = 1
	input.Candidates[0].Variables = []Variable{{ID: "gain", Kind: "gain", Value: 1, AllowedValues: []float64{1, 2}, Effects: []RepairEffect{{Analysis: simmodel.AnalysisACSweep, Metric: "voltage_gain", Direction: RepairMetricIncreases}}}}
	exhausted := Run(context.Background(), input, closedLoopTestEvaluator(false), policy)
	if exhausted.Candidates[0].StopReason != StopBudgetExhausted || !exhausted.Consumption.BudgetExhausted {
		t.Fatalf("evaluation budget = %#v", exhausted)
	}
}

func closedLoopTestEvaluator(thermalRegression bool) evaluatorFunc {
	return func(_ context.Context, state CandidateState) (Evaluation, error) {
		gain := 10.0
		if len(state.Variables) != 0 {
			gain = state.Variables[0].Value * 5
		}
		temperature := 80.0
		if thermalRegression && gain >= 10 {
			temperature = 130
		}
		simulation := &SimulationEvidence{}
		evidenceHash, _ := HashSimulationEvidence(*simulation)
		return Evaluation{
			EvidenceHash: evidenceHash, Measurements: closedLoopMeasurements(gain, temperature), Simulation: simulation,
			ModelDecisions: []ModelDecision{{
				Component: "r1", Family: "resistor", Status: "used", Reason: "trusted full behavioral evaluation",
				RequiredAnalyses: []string{simmodel.AnalysisACSweep, simmodel.AnalysisThermal},
				Claim:            simmodel.CatalogEvidence{ModelID: simmodel.PrimitiveResistorV1}, Provenance: &simmodel.ModelProvenance{
					Source: "manufacturer-datasheet:test", Revision: "rev-a", SHA256: testHash("model"), ReviewStatus: "reviewed",
					AllowedAnalyses: []string{simmodel.AnalysisACSweep, simmodel.AnalysisThermal},
				},
			}},
		}, nil
	}
}

func closedLoopMeasurements(gain, temperature float64) []Measurement {
	return []Measurement{{RequirementID: "gain", OperatingCase: "rated", Actual: gain}, {RequirementID: "thermal", OperatingCase: "rated", Actual: temperature}}
}

func closedLoopV5DynamicRequirement() architecturesearch.Requirement {
	requirement := closedLoopTestRequirement()
	requirement.Schema = architecturesearch.SchemaIDV5
	requirement.Version = architecturesearch.VersionV5
	requirement.Acceptance.RequireHierarchicalDecomposition = true
	requirement.Acceptance.RequireInterfaceContracts = true
	requirement.Acceptance.RequireSharedResourcePlanning = true
	requirement.Acceptance.RequireDeterministicBacktracking = true
	requirement.Acceptance.RequirePhysicalPartitioning = true
	requirement.Acceptance.RequireEndToEndTraceability = true
	requirement.Acceptance.RequireDynamicModelProvenance = true
	requirement.Acceptance.RequireReturnRatioEvidence = true
	requirement.Acceptance.RequireDynamicElectrothermalEvidence = true
	requirement.Acceptance.RequireEventCoverage = true
	requirement.Acceptance.RequireDynamicArchitectureSelection = true
	requirement.Acceptance.RequireBoundedDynamicRepair = true
	applied := 1.0
	requirement.Requirements.OperatingCases[0].Events = []architecturesearch.OperatingEvent{{
		ID: "load_step", Kind: "load_step", Target: architecturesearch.Observation{Kind: "port", ID: "output"},
		DurationS: 1e-3, Applied: &applied, Unit: "A",
	}}
	requirement.Requirements.BehavioralRequirements[1].Metric = "peak_junction_temperature"
	requirement.Requirements.BehavioralRequirements[1].Analysis = simmodel.AnalysisElectrothermal
	return requirement
}

func closedLoopDynamicModelDecision() ModelDecision {
	return ModelDecision{
		Component: "dynamic_stage", Family: "mosfet", Status: "used", Reason: "reviewed dynamic model",
		RequiredAnalyses: []string{simmodel.AnalysisACSweep, simmodel.AnalysisElectrothermal},
		Claim: simmodel.CatalogEvidence{
			ModelID: simmodel.PrimitiveNMOSSwitchV1,
			Parameters: []simmodel.NamedValue{
				{Name: "gate_on_voltage_v", Value: 4.5},
				{Name: "on_resistance_ohm", Value: 0.05},
				{Name: "max_drain_current_a", Value: 5},
				{Name: "max_drain_source_voltage_v", Value: 30},
				{Name: "max_gate_source_voltage_v", Value: 12},
			},
		},
		Provenance: &simmodel.ModelProvenance{
			Source: "manufacturer-datasheet:test", Revision: "rev-a", SHA256: testHash("v5-dynamic-model"), ReviewStatus: "reviewed",
			AllowedAnalyses: []string{simmodel.AnalysisACSweep, simmodel.AnalysisElectrothermal},
		},
	}
}

func closedLoopTestRequirement() architecturesearch.Requirement {
	minimumGain, maximumGain, maximumTemperature := 9.0, 11.0, 100.0
	conditionMinimum, conditionMaximum := 4.5, 5.5
	return architecturesearch.Requirement{
		Schema: architecturesearch.SchemaIDV3, Version: architecturesearch.VersionV3,
		Project: architecturesearch.Project{Name: "closed_loop_test", Title: "Closed loop test", Description: "Behavioral synthesis test requirement."},
		Requirements: architecturesearch.Requirements{
			Domains:        []architecturesearch.Domain{{ID: "supply", Kind: "supply", NominalVoltageV: 5, Source: "external"}, {ID: "ground", Kind: "reference", NominalVoltageV: 0, Source: "external"}},
			Ports:          []architecturesearch.Port{{ID: "ground", Kind: "reference", Direction: "bidirectional", Domain: "ground"}, {ID: "input", Kind: "analog_voltage", Direction: "sink", Domain: "ground"}, {ID: "output", Kind: "analog_voltage", Direction: "source", Domain: "ground"}, {ID: "power", Kind: "power", Direction: "sink", Domain: "supply"}},
			Objectives:     []architecturesearch.Objective{{ID: "amplify", Capability: "signal_amplification", Bindings: []architecturesearch.Binding{{Role: "input", Port: "input"}, {Role: "output", Port: "output"}, {Role: "power", Port: "power"}, {Role: "reference", Port: "ground"}}, Constraints: []architecturesearch.Constraint{}}},
			OperatingCases: []architecturesearch.OperatingCase{{ID: "rated", Conditions: []architecturesearch.OperatingCondition{{Axis: "supply_voltage", Target: "supply", Min: &conditionMinimum, Max: &conditionMaximum, Unit: "V"}}}},
			BehavioralRequirements: []architecturesearch.BehavioralRequirement{
				{ID: "gain", Metric: "voltage_gain", Analysis: simmodel.AnalysisACSweep, Observation: architecturesearch.Observation{Kind: "port", ID: "output"}, Min: &minimumGain, Max: &maximumGain, Unit: "ratio", OperatingCases: []string{"rated"}},
				{ID: "thermal", Metric: "junction_temperature", Analysis: simmodel.AnalysisThermal, Observation: architecturesearch.Observation{Kind: "circuit", ID: "circuit"}, Max: &maximumTemperature, Unit: "degC", OperatingCases: []string{"rated"}, Critical: true},
			},
			Constraints: architecturesearch.BoardLimits{MaxComponents: 16, MaxWidthMM: 50, MaxHeightMM: 40},
		},
		Acceptance: architecturesearch.Acceptance{RequireERC: true, RequireStrictDRC: true, RequireCompleteRouting: true, RequireConnectivity: true, RequireWriterCorrectness: true, RequireRoundTripZeroDiff: true, RequireDeterministicReplay: true, RequireContractComposition: true, RequireGlobalReasoning: true, RequireCoverageAccounting: true, RequireAlternatives: true, RequireFailClosed: true, RequireSimulation: true, RequireAllCorners: true, RequireModelProvenance: true, RequireClosedLoopEvidence: true},
	}
}

func testHash(value string) string {
	data, _ := json.Marshal(value)
	return hashJSON(data)
}
