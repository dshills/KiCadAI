package compositionlowering

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/components"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

func TestArchitectureSimulationPlanResolverRelowersAndResolvesRetainedCandidate(t *testing.T) {
	data, err := os.ReadFile("../circuitgraph/testdata/open_set_composition_corpus/fourth_order_active_lowpass.json")
	if err != nil {
		t.Fatal(err)
	}
	requirement, decodeIssues := architecturesearch.DecodeStrict(bytes.NewReader(data))
	if len(decodeIssues) != 0 {
		t.Fatalf("decode issues = %#v", decodeIssues)
	}
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, registryIssues := architecturesearch.NewCatalogRegistry(catalog)
	if len(registryIssues) != 0 {
		t.Fatalf("registry issues = %#v", registryIssues)
	}
	search := architecturesearch.Search(context.Background(), requirement, registry, architecturesearch.SearchOptions{CatalogHash: "checked-in"})
	if search.Status != architecturesearch.SearchSelected || search.Selected == nil {
		t.Fatalf("search = %#v", search)
	}
	resolver := ArchitectureSimulationPlanResolver{
		Requirement: requirement, Search: search,
		GraphResolver: circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"}), RequiredAnalyses: []string{simmodel.AnalysisACSweep},
	}
	plans, err := resolver.ResolveSimulationPlans(context.Background(), closedloopsynthesis.CandidateState{Fingerprint: search.Selected.Fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) == 0 {
		t.Fatal("re-lowered retained candidate produced no trusted simulation workflows")
	}
	for kind, plan := range plans {
		if plan.TopologyHash == "" || plan.CatalogHash == "" || !simmodel.SupportsAnalysis(plan.ModelID, kind) {
			t.Fatalf("resolved %s plan = %#v", kind, plan)
		}
	}
	basePlan, ok := plans[simmodel.AnalysisACSweep]
	if !ok {
		t.Fatalf("active-filter candidate lacks AC workflow: %#v", plans)
	}
	component, value := firstRepairablePassive(basePlan)
	if component == "" || value <= 0 {
		t.Fatalf("active-filter plan has no repairable passive: %#v", basePlan.Devices)
	}
	repairedValue := value * 1.1
	resolver.VariableBindings = []ArchitectureVariableBinding{{
		CandidateFingerprint: search.Selected.Fingerprint, VariableID: "passive", Kind: ArchitectureVariableComponentValue, Component: component,
	}}
	repairedPlans, err := resolver.ResolveSimulationPlans(context.Background(), closedloopsynthesis.CandidateState{
		Fingerprint: search.Selected.Fingerprint,
		Variables:   []closedloopsynthesis.Variable{{ID: "passive", Value: repairedValue}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := resolvedDeviceValue(repairedPlans[simmodel.AnalysisACSweep], component); !ok || got != repairedValue {
		t.Fatalf("fresh repaired passive value = %g, %t; want %g", got, ok, repairedValue)
	}
	if _, err := resolver.ResolveSimulationPlans(context.Background(), closedloopsynthesis.CandidateState{Fingerprint: closedLoopIntegrationHash("missing")}); err == nil {
		t.Fatal("unretained candidate fingerprint was accepted")
	}
}

func TestBuildClosedLoopInputPreservesEveryRetainedArchitecture(t *testing.T) {
	data, err := os.ReadFile("../architecturesearch/testdata/simulation_grounded_closed_loop_corpus/low_noise_sensor_decision.json")
	if err != nil {
		t.Fatal(err)
	}
	requirement, decodeIssues := architecturesearch.DecodeStrict(bytes.NewReader(data))
	if len(decodeIssues) != 0 {
		t.Fatalf("decode issues = %#v", decodeIssues)
	}
	requirementHash, err := architecturesearch.CanonicalHash(architecturesearch.Normalize(requirement))
	if err != nil {
		t.Fatal(err)
	}
	first, second := closedLoopIntegrationHash("architecture-a"), closedLoopIntegrationHash("architecture-b")
	search := architecturesearch.SearchResult{
		Status: architecturesearch.SearchSelected, RequirementHash: requirementHash,
		CatalogHash: closedLoopIntegrationHash("catalog"), FormulaLibraryHash: architecturesearch.FormulaLibraryHash(),
		Selected:     &architecturesearch.CandidateResult{Fingerprint: second, Score: architecturesearch.CandidateScore{Fingerprint: second, ComponentCount: 8}},
		Alternatives: []architecturesearch.CandidateResult{{Fingerprint: first, Score: architecturesearch.CandidateScore{Fingerprint: first, ComponentCount: 10}}},
	}
	variables := []ClosedLoopCandidateVariables{{Fingerprint: first, Variables: []closedloopsynthesis.Variable{{
		ID: "gain", Kind: "gain", Value: 1, AllowedValues: []float64{1, 2},
		Effects: []closedloopsynthesis.RepairEffect{{Analysis: simmodel.AnalysisACSweep, Metric: "voltage_gain", Direction: closedloopsynthesis.RepairMetricIncreases}},
	}}}}
	input, diagnostics := BuildClosedLoopInput(requirement, search, closedLoopIntegrationHash("model-registry"), variables)
	if len(diagnostics) != 0 {
		t.Fatalf("input diagnostics = %#v", diagnostics)
	}
	if len(input.Candidates) != 2 || input.Candidates[0].Fingerprint != first || len(input.Candidates[0].Variables) != 1 || input.Candidates[1].Fingerprint != second {
		t.Fatalf("closed-loop input = %#v", input)
	}
	search.RequirementHash = closedLoopIntegrationHash("stale")
	if _, diagnostics := BuildClosedLoopInput(requirement, search, closedLoopIntegrationHash("model-registry"), variables); len(diagnostics) == 0 {
		t.Fatal("stale architecture search was accepted")
	}
}

func TestArchitectureSimulationPlanResolverRebindsV3AnalysisPlanPerCandidate(t *testing.T) {
	data, err := os.ReadFile("../circuitgraph/testdata/open_set_composition_corpus/fourth_order_active_lowpass.json")
	if err != nil {
		t.Fatal(err)
	}
	requirement, decodeIssues := architecturesearch.DecodeStrict(bytes.NewReader(data))
	if len(decodeIssues) != 0 {
		t.Fatalf("decode issues = %#v", decodeIssues)
	}
	requirement.Schema, requirement.Version = architecturesearch.SchemaIDV3, architecturesearch.VersionV3
	supplyMinimum, supplyMaximum := 4.75, 5.25
	minimumGain, maximumGain := .9, 1.1
	minimumCutoff, maximumCutoff := 1900.0, 2100.0
	requirement.Requirements.OperatingCases = []architecturesearch.OperatingCase{{ID: "supply", Conditions: []architecturesearch.OperatingCondition{{Axis: "supply_voltage", Target: "logic_5v", Min: &supplyMinimum, Max: &supplyMaximum, Unit: "V"}}}}
	requirement.Requirements.BehavioralRequirements = []architecturesearch.BehavioralRequirement{
		{ID: "cutoff", Metric: "cutoff_frequency", Analysis: simmodel.AnalysisACSweep, Observation: architecturesearch.Observation{Kind: "port", ID: "signal_out"}, Min: &minimumCutoff, Max: &maximumCutoff, Unit: "Hz", OperatingCases: []string{"supply"}},
		{ID: "gain", Metric: "voltage_gain", Analysis: simmodel.AnalysisACSweep, Observation: architecturesearch.Observation{Kind: "port", ID: "signal_out"}, Min: &minimumGain, Max: &maximumGain, Unit: "ratio", OperatingCases: []string{"supply"}},
	}
	requirement.Acceptance.RequireContractComposition = true
	requirement.Acceptance.RequireGlobalReasoning = true
	requirement.Acceptance.RequireCoverageAccounting = true
	requirement.Acceptance.RequireAlternatives = true
	requirement.Acceptance.RequireFailClosed = true
	requirement.Acceptance.RequireSimulation = true
	requirement.Acceptance.RequireAllCorners = true
	requirement.Acceptance.RequireModelProvenance = true
	requirement.Acceptance.RequireClosedLoopEvidence = true
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, registryIssues := architecturesearch.NewCatalogRegistry(catalog)
	if len(registryIssues) != 0 {
		t.Fatalf("registry issues = %#v", registryIssues)
	}
	search := architecturesearch.Search(context.Background(), requirement, registry, architecturesearch.SearchOptions{CatalogHash: "behavioral-corpus"})
	if search.Status != architecturesearch.SearchSelected || search.Selected == nil {
		t.Fatalf("search = %#v", search)
	}
	provenance, provenanceDiagnostics := modelprovenance.LoadDefault()
	if len(provenanceDiagnostics) != 0 {
		t.Fatalf("provenance diagnostics = %#v", provenanceDiagnostics)
	}
	resolver := ArchitectureSimulationPlanResolver{
		Requirement: requirement, Search: search, ProvenanceRegistry: provenance,
		GraphResolver: circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"}),
	}
	planSet, err := resolver.ResolveSimulationPlanSet(context.Background(), closedloopsynthesis.CandidateState{Fingerprint: search.Selected.Fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	if len(planSet.Plans) != 1 || planSet.AnalysisPlan.PlanHash == "" || len(planSet.AnalysisPlan.Assertions) != len(requirement.Requirements.BehavioralRequirements) {
		t.Fatalf("candidate plan set = %#v", planSet)
	}
	if len(planSet.Templates) != 1 || len(planSet.Assertions) != 2 || len(planSet.OperatingBindings) != 1 {
		t.Fatalf("candidate simulation contracts = %#v", planSet)
	}
	for _, assertion := range planSet.AnalysisPlan.Assertions {
		if assertion.Target == "" || assertion.Target == assertion.RequirementID {
			t.Fatalf("behavioral assertion was not rebound to a lowered net: %#v", assertion)
		}
	}
	var nominalPlan simmodel.Plan
	for _, plan := range planSet.Plans {
		search.CatalogHash = plan.CatalogHash
		nominalPlan = plan
		break
	}
	component, nominalValue := firstRepairablePassive(nominalPlan)
	if component == "" || nominalValue <= 0 {
		t.Fatalf("candidate has no bounded filter repair variable: %#v", nominalPlan.Devices)
	}
	failedValue := nominalValue * 2
	resolver.VariableBindings = []ArchitectureVariableBinding{{CandidateFingerprint: search.Selected.Fingerprint, VariableID: "filter_value", Kind: ArchitectureVariableComponentValue, Component: component}}
	search.Alternatives = nil
	resolver.Search = search
	modelRegistryHash, err := modelprovenance.Hash(provenance)
	if err != nil {
		t.Fatal(err)
	}
	input, inputDiagnostics := BuildClosedLoopInput(requirement, search, modelRegistryHash, []ClosedLoopCandidateVariables{{
		Fingerprint: search.Selected.Fingerprint,
		Variables: []closedloopsynthesis.Variable{{
			ID: "filter_value", Kind: "filter", Value: failedValue, AllowedValues: []float64{nominalValue, failedValue},
			Effects: []closedloopsynthesis.RepairEffect{{Analysis: simmodel.AnalysisACSweep, Metric: "cutoff_frequency", Direction: closedloopsynthesis.RepairMetricDecreases}},
		}},
	}})
	if len(inputDiagnostics) != 0 {
		t.Fatalf("closed-loop input diagnostics = %#v", inputDiagnostics)
	}
	evaluator := closedloopsynthesis.SimModelEvaluator{
		Resolver: closedloopsynthesis.PlannedSimulationResolver{Base: resolver}, ProvenanceRegistry: provenance,
	}
	report := closedloopsynthesis.Run(context.Background(), input, evaluator, closedloopsynthesis.DefaultPolicy())
	if report.Status != "pass" || report.Selected == nil || report.Consumption.Evaluations != 2 || report.Consumption.RepairsApplied != 1 || report.Selected.State.Variables[0].Value != nominalValue {
		t.Fatalf("production architecture closed loop = %#v", report)
	}
}

func firstRepairablePassive(plan simmodel.Plan) (string, float64) {
	for _, device := range plan.Devices {
		if device.ValueSI == nil || (device.Family != "resistor" && device.Family != "capacitor") {
			continue
		}
		component := device.PhysicalComponent
		if component == "" {
			component = device.Component
		}
		return component, *device.ValueSI
	}
	return "", 0
}

func resolvedDeviceValue(plan simmodel.Plan, physicalComponent string) (float64, bool) {
	for _, device := range plan.Devices {
		component := device.PhysicalComponent
		if component == "" {
			component = device.Component
		}
		if component == physicalComponent && device.ValueSI != nil {
			return *device.ValueSI, true
		}
	}
	return 0, false
}

func closedLoopIntegrationHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest[:])
}
