package compositionlowering

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/components"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
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

func TestIsolatedHierarchicalTransientPlanReleasesDrivenBus(t *testing.T) {
	data, err := os.ReadFile("../architecturesearch/testdata/hierarchical_multi_domain_corpus/isolated_mixed_voltage_gateway_system.json")
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
	graphResolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"})
	search := architecturesearch.Search(context.Background(), requirement, registry, architecturesearch.SearchOptions{CatalogHash: graphResolver.CatalogHash()})
	if search.Status != architecturesearch.SearchSelected || search.Selected == nil {
		t.Fatalf("search status=%s issues=%#v rejections=%#v", search.Status, search.Issues, search.Rejections)
	}
	provenance, provenanceDiagnostics := modelprovenance.LoadDefault()
	if len(provenanceDiagnostics) != 0 {
		t.Fatalf("provenance diagnostics = %#v", provenanceDiagnostics)
	}
	resolver := ArchitectureSimulationPlanResolver{Requirement: requirement, Search: search, GraphResolver: graphResolver, ProvenanceRegistry: provenance}
	state := closedloopsynthesis.CandidateState{Fingerprint: search.Selected.Fingerprint}
	planSet, err := resolver.ResolveSimulationPlanSet(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	resolution, compileDiagnostics := closedloopsynthesis.CompileSimulationResolution(planSet.AnalysisPlan, planSet.Plans, planSet.Templates, planSet.Assertions, planSet.OperatingBindings)
	if len(compileDiagnostics) != 0 {
		t.Fatalf("compile diagnostics = %#v", compileDiagnostics)
	}
	planIndex := slices.IndexFunc(resolution.Plans, func(plan simmodel.Plan) bool {
		return slices.ContainsFunc(plan.Assertions, func(assertion simmodel.Assertion) bool {
			return assertion.Quantity == simmodel.QuantityRiseTimeS
		})
	})
	if planIndex < 0 {
		t.Fatalf("isolated hierarchy lacks a compiled rise-time plan: %#v", resolution.Plans)
	}
	transient := resolution.Plans[planIndex]
	riseIndex := slices.IndexFunc(transient.Assertions, func(assertion simmodel.Assertion) bool {
		return assertion.Quantity == simmodel.QuantityRiseTimeS
	})
	rise := transient.Assertions[riseIndex]
	transient.Assertions = []simmodel.Assertion{rise}
	report, diagnostics := simmodel.Evaluate(transient)
	if len(diagnostics) != 0 || report.Status != "pass" {
		var samples []string
		minimum, maximum := math.Inf(1), math.Inf(-1)
		for _, analysis := range report.Analyses {
			if analysis.ID != rise.AnalysisID {
				continue
			}
			for _, point := range analysis.Points {
				for _, node := range point.Nodes {
					if node.Node != rise.Node {
						continue
					}
					minimum, maximum = math.Min(minimum, node.Real), math.Max(maximum, node.Real)
					if len(samples) < 40 {
						samples = append(samples, fmt.Sprintf("%.9g:%.9g", point.TimeS, node.Real))
					}
				}
			}
		}
		t.Fatalf("isolated hierarchy rise min=%.9g max=%.9g samples=%v diagnostics=%#v", minimum, maximum, samples, diagnostics)
	}
}

func TestConstantCurrentResolverRetainsEveryBehavioralAnalysis(t *testing.T) {
	data, err := os.ReadFile("../architecturesearch/testdata/held_out_capability_expansion_corpus/power_constant_current_output.json")
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
	graphResolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"})
	search := architecturesearch.Search(context.Background(), requirement, registry, architecturesearch.SearchOptions{CatalogHash: graphResolver.CatalogHash()})
	if search.Status != architecturesearch.SearchSelected || search.Selected == nil {
		t.Fatalf("search = %#v", search)
	}
	lowered, loweringIssues := Lower(requirement, search)
	if len(loweringIssues) != 0 {
		t.Fatalf("lowering issues = %#v", loweringIssues)
	}
	resolved, resolveIssues := graphResolver.Resolve(context.Background(), lowered.Document)
	if len(resolveIssues) != 0 {
		t.Fatalf("resolve issues = %#v", resolveIssues)
	}
	if len(resolved.Source.PowerFlags) == 0 {
		t.Fatalf("external power domains lack synthesized PWR_FLAG declarations: %#v", resolved.Source)
	}
	regulatorInputNet := ""
	for _, net := range resolved.Source.Nets {
		if slices.ContainsFunc(net.Endpoints, func(endpoint circuitgraph.Endpoint) bool {
			return strings.Contains(endpoint.Component, "current_regulator") && strings.EqualFold(endpoint.Selector, "IN")
		}) {
			regulatorInputNet = net.Name
			break
		}
	}
	if regulatorInputNet == "" || !slices.ContainsFunc(resolved.Source.PowerFlags, func(flag circuitgraph.PowerFlag) bool {
		return flag.Net == regulatorInputNet
	}) {
		t.Fatalf("switched regulator input net %q lacks propagated PWR_FLAG: %#v", regulatorInputNet, resolved.Source.PowerFlags)
	}
	resolver := ArchitectureSimulationPlanResolver{Requirement: requirement, Search: search, GraphResolver: graphResolver}
	plans, err := resolver.ResolveSimulationPlans(context.Background(), closedloopsynthesis.CandidateState{Fingerprint: search.Selected.Fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{simmodel.AnalysisDCOperatingPoint, simmodel.AnalysisTransient, simmodel.AnalysisStartup, simmodel.AnalysisThermal} {
		if plan, ok := plans[kind]; !ok || len(plan.Analyses) != 1 || plan.Analyses[0].Kind != kind {
			t.Fatalf("resolved constant-current plans lack %s: %#v", kind, plans)
		}
	}
	transient := plans[simmodel.AnalysisTransient].Analyses[0]
	if !simulationAnalysisHasDynamicExcitation(transient) || transient.DurationS < .019 || transient.DurationS > .021 {
		t.Fatalf("autonomous response plan lacks requirement-scaled supply step: %#v", transient)
	}
}

func TestAutonomousCurrentRegulatorSupplyStepIgnoresUnrelatedRail(t *testing.T) {
	maximum := .001
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{{
			ID: "response", Metric: "response_time", Analysis: simmodel.AnalysisTransient,
			Max: &maximum, Unit: "s",
		}},
	}}
	base := simmodel.Plan{
		Devices: []simmodel.ResolvedDevice{
			{Component: "logic_supply", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "LOGIC"}, {Terminal: "NEGATIVE", Net: "GND"}}},
			{Component: "load_supply", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "LOAD"}, {Terminal: "NEGATIVE", Net: "GND"}}},
			{Component: "enable", PrimitiveModel: simmodel.PrimitivePMOSSwitchV1, Terminals: []simmodel.TerminalBinding{{Terminal: "SOURCE", Net: "LOAD"}, {Terminal: "DRAIN", Net: "SWITCHED"}, {Terminal: "GATE", Net: "GATE"}}},
			{Component: "regulator", PrimitiveModel: simmodel.PrimitiveProgrammableCurrentSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "IN", Net: "SWITCHED"}, {Terminal: "OUT", Net: "OUT"}, {Terminal: "SET", Net: "SET"}}},
		},
	}
	analysis := simmodel.Analysis{
		Kind: simmodel.AnalysisTransient, DurationS: .02, TimeStepS: .00005,
		Excitations: []simmodel.SourceExcitation{
			{Component: "logic_supply", DCValue: 5},
			{Component: "load_supply", DCValue: 12},
		},
	}
	if err := configureAutonomousTransientSupplyStep(requirement, base, &analysis); err != nil {
		t.Fatal(err)
	}
	if analysis.Excitations[0].DCValue != 5 || analysis.Excitations[0].PulsePeriodS != 0 {
		t.Fatalf("unrelated logic rail was modified: %#v", analysis.Excitations[0])
	}
	if analysis.Excitations[1].DCValue != 0 || analysis.Excitations[1].PulseValue != 12 || analysis.Excitations[1].PulsePeriodS == 0 {
		t.Fatalf("upstream load rail was not stepped: %#v", analysis.Excitations[1])
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
	for kind, plan := range planSet.Plans {
		if diagnostics := simmodel.ValidatePlan(plan); len(diagnostics) != 0 {
			t.Logf("base plan %s diagnostics=%#v", kind, diagnostics)
		}
		if diagnostics := simmodel.ValidatePlan(simmodel.ClonePlan(plan)); len(diagnostics) != 0 {
			t.Logf("cloned plan %s diagnostics=%#v", kind, diagnostics)
		}
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

func TestDerivedACSweepUsesBoundedDefaultWithoutBaseACAnalysis(t *testing.T) {
	base := simmodel.Plan{
		ModelID:    simmodel.ModelNonlinearCircuitDCV1,
		GroundNode: "GND",
		Nodes:      []string{"GND", "OUT"},
		Analyses: []simmodel.Analysis{{
			ID: "dc", Kind: simmodel.AnalysisDCOperatingPoint,
			Excitations: []simmodel.SourceExcitation{{Component: "signal", DCValue: 1}},
		}},
	}
	base.Devices = []simmodel.ResolvedDevice{{
		Component: "signal", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1,
		Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "IN"}, {Terminal: "NEGATIVE", Net: "GND"}},
	}}
	base.Nodes = append(base.Nodes, "IN")
	intent, err := derivedGraphWorkflowIntent(architecturesearch.Requirement{}, base, simmodel.AnalysisACSweep, "IN")
	if err != nil {
		t.Fatal(err)
	}
	if len(intent.Analyses) != 1 || intent.Analyses[0].StartFrequencyHz <= 0 || intent.Analyses[0].StopFrequencyHz < intent.Analyses[0].StartFrequencyHz || intent.Analyses[0].Points < 2 {
		t.Fatalf("derived AC analysis is not bounded: %#v", intent.Analyses)
	}
	if len(intent.Assertions) != 1 || intent.Assertions[0].FrequencyHz != intent.Analyses[0].StartFrequencyHz {
		t.Fatalf("derived AC assertion is not on the sweep: %#v", intent.Assertions)
	}
}

func TestDerivedStabilitySweepBracketsCatalogOpAmpBandwidth(t *testing.T) {
	base := simmodel.Plan{
		Analyses: []simmodel.Analysis{{ID: "ac", Kind: simmodel.AnalysisACSweep, StartFrequencyHz: 10, StopFrequencyHz: 100_000, Points: 64}},
		Devices: []simmodel.ResolvedDevice{{
			Component: "amplifier", PrimitiveModel: simmodel.PrimitiveOpAmpV1,
			ModelParameters: []simmodel.NamedValue{{Name: "dc_open_loop_gain", Value: 100_000}, {Name: "gain_bandwidth_hz", Value: 10_000_000}},
		}},
	}

	analysis := derivedAnalysisTemplate(base, simmodel.AnalysisStability)
	if analysis.StartFrequencyHz > 10 || analysis.StopFrequencyHz < 100_000_000 || analysis.Points < 64 {
		t.Fatalf("stability sweep does not bracket trusted open-loop model: %#v", analysis)
	}
}

func TestDerivedStabilitySweepBracketsCatalogBuckControlBandwidth(t *testing.T) {
	base := simmodel.Plan{
		Analyses: []simmodel.Analysis{{ID: "ac", Kind: simmodel.AnalysisACSweep, StartFrequencyHz: 10, StopFrequencyHz: 100_000, Points: 64}},
		Devices: []simmodel.ResolvedDevice{{
			Component: "converter", PrimitiveModel: simmodel.PrimitiveSynchronousBuckRegulatorV1,
			ModelParameters: []simmodel.NamedValue{
				{Name: "control_pole_hz", Value: 150_000},
				{Name: "switching_frequency_hz", Value: 500_000},
			},
		}},
	}

	analysis := derivedAnalysisTemplate(base, simmodel.AnalysisStability)
	if analysis.StartFrequencyHz > 10 || analysis.StopFrequencyHz < 5_000_000 || analysis.Points < 64 {
		t.Fatalf("stability sweep does not bracket trusted buck control model: %#v", analysis)
	}
}

func TestDerivedACSweepBracketsCatalogBJTTransitionFrequency(t *testing.T) {
	base := simmodel.Plan{
		GroundNode: "GND", Nodes: []string{"GND", "IN", "OUT"},
		Analyses: []simmodel.Analysis{{ID: "dc", Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{{Component: "input"}}}},
		Devices: []simmodel.ResolvedDevice{
			{Component: "input", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "IN"}, {Terminal: "NEGATIVE", Net: "GND"}}},
			{Component: "gain", PrimitiveModel: simmodel.PrimitiveBJTNPNV1, ModelParameters: []simmodel.NamedValue{{Name: "forward_beta", Value: 15}, {Name: "transition_frequency_hz", Value: 40_000_000}}},
		},
	}
	intent, err := derivedGraphWorkflowIntent(architecturesearch.Requirement{}, base, simmodel.AnalysisACSweep, "IN")
	if err != nil {
		t.Fatal(err)
	}
	analysis := intent.Analyses[0]
	if analysis.StopFrequencyHz < 400_000_000 || analysis.Points < 64 {
		t.Fatalf("AC sweep does not bracket trusted BJT response: %#v", analysis)
	}
}

func TestCenteredTransientPulsesOnlySemanticInputAndKeepsSupplyPowered(t *testing.T) {
	plan := simmodel.Plan{
		GroundNode: "GND",
		Nodes:      []string{"GND", "IN", "OUT", "VCC"},
		Devices: []simmodel.ResolvedDevice{
			{Component: "input", CatalogID: "input", Family: "voltage_source", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "IN"}, {Terminal: "NEGATIVE", Net: "GND"}}},
			{Component: "opamp", CatalogID: "opamp", Family: "opamp", PrimitiveModel: simmodel.PrimitiveOpAmpV1, ModelParameters: []simmodel.NamedValue{{Name: "dc_open_loop_gain", Value: 100000}, {Name: "gain_bandwidth_hz", Value: 1000000}, {Name: "output_high_margin_v", Value: .1}, {Name: "output_low_margin_v", Value: .1}, {Name: "supply_max_v", Value: 30}, {Name: "supply_min_v", Value: 3}}, Terminals: []simmodel.TerminalBinding{{Terminal: "IN_PLUS", Net: "IN"}, {Terminal: "IN_MINUS", Net: "OUT"}, {Terminal: "OUT", Net: "OUT"}, {Terminal: "V_PLUS", Net: "VCC"}, {Terminal: "V_MINUS", Net: "GND"}}},
			{Component: "supply", CatalogID: "supply", Family: "voltage_source", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "VCC"}, {Terminal: "NEGATIVE", Net: "GND"}}},
		},
		Analyses: []simmodel.Analysis{{ID: "base_transient", Kind: simmodel.AnalysisTransient, DurationS: .001, TimeStepS: .00001, Excitations: []simmodel.SourceExcitation{{Component: "input", DCValue: 1}, {Component: "supply", DCValue: 5}}}},
	}

	centered, err := centerBehavioralInputBias(plan, simmodel.AnalysisTransient, "IN")
	if err != nil {
		t.Fatal(err)
	}
	input, supply := centered.Analyses[0].Excitations[0], centered.Analyses[0].Excitations[1]
	if input.DCValue != 0 || input.PulsePeriodS == 0 || input.PulseInitialValue == input.PulseValue {
		t.Fatalf("input excitation is not a bounded centered pulse: %#v", input)
	}
	if supply.DCValue != 5 || supply.PulsePeriodS != 0 {
		t.Fatalf("supply excitation must remain powered during signal transient: %#v", supply)
	}
}

func TestBehavioralCenteredBiasCacheKeyIgnoresAnalysisGridButPreservesOperatingConditions(t *testing.T) {
	plan := simmodel.Plan{
		RegistryVersion: "registry-v1",
		TopologyHash:    "topology",
		GroundNode:      "GND",
		Nodes:           []string{"GND", "IN", "VCC"},
		Devices: []simmodel.ResolvedDevice{
			{Component: "input", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1},
			{Component: "supply", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1},
		},
		Analyses: []simmodel.Analysis{{
			ID: "transient", Kind: simmodel.AnalysisTransient, DurationS: .01, TimeStepS: .001,
			Excitations: []simmodel.SourceExcitation{{Component: "input", DCValue: 1, PulseInitialValue: 1, PulseValue: 2}, {Component: "supply", DCValue: 5}},
		}},
	}
	first, ok := behavioralCenteredBiasCacheKey(plan, "input", false)
	if !ok {
		t.Fatal("expected cacheable plan")
	}
	equivalent := simmodel.ClonePlan(plan)
	equivalent.Analyses[0].ID = "ac"
	equivalent.Analyses[0].Kind = simmodel.AnalysisACSweep
	equivalent.Analyses[0].StartFrequencyHz = 10
	equivalent.Analyses[0].StopFrequencyHz = 1e6
	equivalent.Analyses[0].Points = 128
	equivalent.Analyses[0].Excitations[0].DCValue = 2.5
	equivalent.Analyses[0].Excitations[0].ACMagnitude = 1
	second, ok := behavioralCenteredBiasCacheKey(equivalent, "input", false)
	if !ok || second != first {
		t.Fatalf("analysis-only variation changed centered-bias identity: %q != %q", second, first)
	}
	differentSupply := simmodel.ClonePlan(plan)
	differentSupply.Analyses[0].Excitations[1].DCValue = 6
	third, ok := behavioralCenteredBiasCacheKey(differentSupply, "input", false)
	if !ok || third == first {
		t.Fatal("supply operating condition must change centered-bias identity")
	}
	thermal, ok := behavioralCenteredBiasCacheKey(plan, "input", true)
	if !ok || thermal == first {
		t.Fatal("thermal selection must use a separate centered-bias identity")
	}
}

func TestBehavioralTransientStimulusUsesDataflowIngressAndKeepsSupplyPowered(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{{ID: "host_5v", Kind: "supply", NominalVoltageV: 5}, {ID: "device_3v3", Kind: "supply", NominalVoltageV: 3.3}},
		Ports: []architecturesearch.Port{
			{ID: "host", Kind: "digital_bus", Direction: "bidirectional", Domain: "host_5v"},
			{ID: "device", Kind: "digital_bus", Direction: "bidirectional", Domain: "device_3v3"},
		},
		Signals: []architecturesearch.Signal{{ID: "translated", Kind: "digital_bus", Domain: "device_3v3"}, {ID: "regulated", Kind: "power", Domain: "device_3v3"}},
		Objectives: []architecturesearch.Objective{
			{ID: "translate", Bindings: []architecturesearch.Binding{{Role: "side_a", Port: "host"}, {Role: "side_b", Signal: "translated", Direction: "source"}, {Role: "power", Signal: "regulated", Direction: "sink"}}},
			{ID: "protect", Bindings: []architecturesearch.Binding{{Role: "input", Signal: "translated", Direction: "sink"}, {Role: "output", Port: "device"}}},
		},
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{{ID: "rise", Analysis: simmodel.AnalysisTransient}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{{Kind: "port", ID: "host", Target: "HOST"}, {Kind: "port", ID: "device", Target: "DEVICE"}}
	stimulus, ok := behavioralTransientStimulusForRequirement(requirement, bindings)
	if !ok || stimulus.Node != "HOST" || stimulus.FinalV != 5 {
		t.Fatalf("derived transient stimulus = %#v, ok=%t", stimulus, ok)
	}
	plan := simmodel.Plan{
		GroundNode: "GND",
		Nodes:      []string{"GND", "HOST", "DEVICE", "VCC"},
		Devices: []simmodel.ResolvedDevice{
			{Component: "host", PrimitiveModel: simmodel.PrimitiveConnectorVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "PIN_1", Net: "GND"}, {Terminal: "PIN_2", Net: "HOST"}}},
			{Component: "supply", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "VCC"}, {Terminal: "NEGATIVE", Net: "GND"}}},
		},
		Analyses: []simmodel.Analysis{{ID: "response", Kind: simmodel.AnalysisTransient, DurationS: 100e-6, TimeStepS: 1e-6, Excitations: []simmodel.SourceExcitation{{Component: "host"}, {Component: "supply", DCValue: 5}}}},
	}
	stimulated, err := configureBehavioralTransientStimulus(plan, simmodel.AnalysisTransient, stimulus, true)
	if err != nil {
		t.Fatal(err)
	}
	host, supply := stimulated.Analyses[0].Excitations[0], stimulated.Analyses[0].Excitations[1]
	if host.DCValue != 0 || host.PulseInitialValue != 0 || host.PulseValue != -5 || host.PulseDelayS != 1e-6 || host.PulseWidthS != plan.Analyses[0].DurationS || host.PulsePeriodS != 2*plan.Analyses[0].DurationS {
		t.Fatalf("host ingress is not a polarity-correct bounded pulse: %#v", host)
	}
	if supply.DCValue != 5 || supply.PulsePeriodS != 0 {
		t.Fatalf("supply excitation must remain powered during interface transient: %#v", supply)
	}
	electrothermal := simmodel.ClonePlan(plan)
	electrothermal.Analyses[0].Kind = simmodel.AnalysisElectrothermal
	stimulated, err = configureBehavioralTransientStimulus(electrothermal, simmodel.AnalysisElectrothermal, stimulus, true)
	if err != nil || stimulated.Analyses[0].Excitations[0].PulseValue != -5 {
		t.Fatalf("electrothermal event stimulus = %#v, err=%v", stimulated.Analyses, err)
	}
}

func TestBehavioralTransientStimulusUsesExplicitBidirectionalInputBoundary(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{
			{ID: "host_5v", Kind: "supply", NominalVoltageV: 5},
			{ID: "remote_1v8", Kind: "supply", NominalVoltageV: 1.8},
		},
		Ports: []architecturesearch.Port{
			{ID: "host_bus", Kind: "digital_bus", Direction: "bidirectional", Domain: "host_5v"},
			{ID: "remote_bus", Kind: "digital_bus", Direction: "bidirectional", Domain: "remote_1v8"},
		},
		Signals: []architecturesearch.Signal{{ID: "protected_bus", Kind: "digital_bus", Domain: "host_5v"}},
		Objectives: []architecturesearch.Objective{
			{ID: "protect", Bindings: []architecturesearch.Binding{
				{Role: "input", Port: "host_bus"},
				{Role: "output", Signal: "protected_bus", Direction: "bidirectional"},
			}},
			{ID: "bridge", Bindings: []architecturesearch.Binding{
				{Role: "side_a", Signal: "protected_bus", Direction: "bidirectional"},
				{Role: "side_b", Port: "remote_bus"},
			}},
		},
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{{
			ID: "rise", Metric: "rise_time", Analysis: simmodel.AnalysisTransient,
			Observation: architecturesearch.Observation{Kind: "port", ID: "remote_bus"},
		}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{
		{Kind: "port", ID: "host_bus", Target: "HOST_BUS"},
		{Kind: "port", ID: "remote_bus", Target: "REMOTE_BUS"},
	}
	stimulus, ok := behavioralTransientStimulusForRequirement(requirement, bindings)
	if !ok || stimulus.SemanticID != "host_bus" || stimulus.Node != "HOST_BUS" || stimulus.InitialV != 0 || stimulus.FinalV != 5 {
		t.Fatalf("explicit bidirectional ingress stimulus = %#v, ok=%t", stimulus, ok)
	}
}

func TestBehavioralLoadCurrentStepUsesDeclaredOperatingBounds(t *testing.T) {
	minimum, maximum := 0.25, 2.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{{
			ID: "response", Metric: "response_time", Analysis: simmodel.AnalysisTransient,
		}},
	}}
	plan := simmodel.Plan{Analyses: []simmodel.Analysis{{
		ID: "response", Kind: simmodel.AnalysisTransient, DurationS: .01, TimeStepS: .0001,
		Excitations: []simmodel.SourceExcitation{{Component: "load"}},
	}}}
	harness := []operatingHarnessDevice{{
		Device: circuitgraph.SimulationHarnessDevice{
			InstanceID: "load", CatalogID: "source.current.connector.1x02",
		},
		Source: true, InitialValue: minimum, HasInitialValue: true,
		DefaultValue: maximum, HasDefaultValue: true,
	}}
	configured, err := configureBehavioralLoadCurrentStep(requirement, simmodel.AnalysisTransient, plan, harness, true)
	if err != nil {
		t.Fatal(err)
	}
	excitation := configured.Analyses[0].Excitations[0]
	if excitation.DCValue != 0 || excitation.PulseInitialValue != minimum || excitation.PulseValue != maximum ||
		excitation.PulseDelayS != configured.Analyses[0].TimeStepS || excitation.PulseWidthS != configured.Analyses[0].DurationS ||
		excitation.PulsePeriodS != 2*configured.Analyses[0].DurationS {
		t.Fatalf("bounded load-current edge = %#v", excitation)
	}
	if unchanged, err := configureBehavioralLoadCurrentStep(requirement, simmodel.AnalysisTransient, plan, harness, false); err != nil || simulationAnalysisHasDynamicExcitation(unchanged.Analyses[0]) {
		t.Fatalf("disabled load-current edge changed plan: err=%v analysis=%#v", err, unchanged.Analyses[0])
	}

	requirement.Requirements.BehavioralRequirements[0].Observation = architecturesearch.Observation{Kind: "port", ID: "current_output"}
	requirement.Requirements.BehavioralRequirements = append(requirement.Requirements.BehavioralRequirements, architecturesearch.BehavioralRequirement{
		ID: "regulated_current", Metric: "dc_current", Analysis: simmodel.AnalysisDCOperatingPoint,
		Observation: architecturesearch.Observation{Kind: "port", ID: "current_output"},
	})
	currentControlled, err := configureBehavioralLoadCurrentStep(requirement, simmodel.AnalysisTransient, plan, harness, true)
	if err != nil || simulationAnalysisHasDynamicExcitation(currentControlled.Analyses[0]) {
		t.Fatalf("current-controlled output received a load-current edge: err=%v analysis=%#v", err, currentControlled.Analyses[0])
	}
}

func TestBehavioralTransientStimulusBracketsThresholdForReferenceDomainInput(t *testing.T) {
	minimum, maximum := 2.4, 2.6
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{{ID: "ground", Kind: "reference"}},
		Ports:   []architecturesearch.Port{{ID: "command", Kind: "analog_voltage", Direction: "sink", Domain: "ground"}},
		Objectives: []architecturesearch.Objective{{ID: "compare", Bindings: []architecturesearch.Binding{
			{Role: "input", Port: "command"}, {Role: "output", Signal: "decision", Direction: "source"},
		}}},
		Signals: []architecturesearch.Signal{{ID: "decision", Kind: "digital_logic"}},
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{
			{ID: "response", Metric: "response_time", Analysis: simmodel.AnalysisTransient},
			{ID: "threshold", Metric: "threshold_voltage", Analysis: simmodel.AnalysisDCOperatingPoint, Min: &minimum, Max: &maximum},
		},
	}}
	stimulus, ok := behavioralTransientStimulusForRequirement(requirement, []closedloopsynthesis.SemanticBinding{{Kind: "port", ID: "command", Target: "COMMAND"}})
	if !ok || stimulus.Node != "COMMAND" || stimulus.InitialV >= minimum || stimulus.FinalV <= maximum {
		t.Fatalf("threshold-bracketing stimulus = %#v, ok=%t", stimulus, ok)
	}
}

func TestBehavioralTransientStimulusSpansRequestedOutputSwing(t *testing.T) {
	minimumSwing, minimumGain, maximumGain := 8.0, 7.6, 8.4
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{{ID: "ground", Kind: "reference"}},
		Ports: []architecturesearch.Port{
			{ID: "input", Kind: "analog_voltage", Direction: "sink", Domain: "ground"},
		},
		Signals: []architecturesearch.Signal{{ID: "amplified", Kind: "analog_voltage", Domain: "ground"}},
		Objectives: []architecturesearch.Objective{{ID: "amplify", Bindings: []architecturesearch.Binding{
			{Role: "input", Port: "input"}, {Role: "output", Signal: "amplified", Direction: "source"},
		}}},
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{
			{ID: "gain", Metric: "voltage_gain", Analysis: simmodel.AnalysisACSweep, Observation: architecturesearch.Observation{Kind: "signal", ID: "amplified"}, Min: &minimumGain, Max: &maximumGain, Unit: "ratio"},
			{ID: "swing", Metric: "output_swing", Analysis: simmodel.AnalysisTransient, Observation: architecturesearch.Observation{Kind: "signal", ID: "amplified"}, Min: &minimumSwing, Unit: "V_pp"},
		},
	}}
	stimulus, ok := behavioralTransientStimulusForRequirement(requirement, []closedloopsynthesis.SemanticBinding{{Kind: "port", ID: "input", Target: "INPUT"}})
	expectedAmplitude := (minimumSwing / 2) / minimumGain
	if !ok || stimulus.InitialV != -expectedAmplitude || stimulus.FinalV != expectedAmplitude || !stimulus.Periodic || stimulus.PeriodicFrequencyHz != 1000 {
		t.Fatalf("symmetric output-swing stimulus = %#v, ok=%t", stimulus, ok)
	}
	plan := simmodel.Plan{
		GroundNode: "GND", Nodes: []string{"GND", "INPUT"},
		Devices:  []simmodel.ResolvedDevice{{Component: "input", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "INPUT"}, {Terminal: "NEGATIVE", Net: "GND"}}}},
		Analyses: []simmodel.Analysis{{ID: "swing", Kind: simmodel.AnalysisTransient, DurationS: 1e-3, TimeStepS: 1e-6, Excitations: []simmodel.SourceExcitation{{Component: "input"}}}},
	}
	stimulated, err := configureBehavioralTransientStimulus(plan, simmodel.AnalysisTransient, stimulus, true)
	if err != nil {
		t.Fatal(err)
	}
	excitation := stimulated.Analyses[0].Excitations[0]
	if excitation.DCValue != 0 || excitation.SineAmplitude != expectedAmplitude || excitation.SineFrequencyHz != 1000 || excitation.PulsePeriodS != 0 {
		t.Fatalf("output-swing excitation is not one smooth full-span cycle: %#v", excitation)
	}
}

func TestDerivedOutputSwingTransientUsesCanonicalBoundedGrid(t *testing.T) {
	minimumSwing, minimumGain, maximumGain := 8.0, 7.6, 8.4
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{BehavioralRequirements: []architecturesearch.BehavioralRequirement{
		{ID: "gain", Metric: "voltage_gain", Min: &minimumGain, Max: &maximumGain, Unit: "ratio"},
		{ID: "swing", Metric: "output_swing", Analysis: simmodel.AnalysisTransient, Min: &minimumSwing, Unit: "V_pp"},
	}}}
	base := simmodel.Plan{
		ModelID: simmodel.ModelNonlinearCircuitDCV1, GroundNode: "GND", Nodes: []string{"GND", "IN", "OUT"},
		Devices:  []simmodel.ResolvedDevice{{Component: "input", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "IN"}, {Terminal: "NEGATIVE", Net: "GND"}}}},
		Analyses: []simmodel.Analysis{{ID: "dc", Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{{Component: "input"}}}},
	}
	intent, err := derivedGraphWorkflowIntent(requirement, base, simmodel.AnalysisTransient, "IN")
	if err != nil {
		t.Fatal(err)
	}
	analysis := intent.Analyses[0]
	steps := analysis.DurationS / analysis.TimeStepS
	if analysis.DurationS != 4e-3 || steps != behavioralDistortionCycles*behavioralTransientSamplesPerCycle || steps != math.Trunc(steps) {
		t.Fatalf("derived swing grid = duration %.12g step %.12g steps %.12g", analysis.DurationS, analysis.TimeStepS, steps)
	}
}

func TestDerivedOutputSwingTransientRetainsBoundedTimingGridWhenRequested(t *testing.T) {
	minimumSwing, minimumGain, maximumResponse := 8.0, 8.0, 10e-6
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{BehavioralRequirements: []architecturesearch.BehavioralRequirement{
		{ID: "gain", Metric: "voltage_gain", Min: &minimumGain, Unit: "ratio"},
		{ID: "swing", Metric: "output_swing", Analysis: simmodel.AnalysisTransient, Min: &minimumSwing, Unit: "V_pp"},
		{ID: "settling", Metric: "settling_time", Analysis: simmodel.AnalysisTransient, Max: &maximumResponse, Unit: "s"},
	}}}
	base := simmodel.Plan{
		ModelID: simmodel.ModelNonlinearCircuitDCV1, GroundNode: "GND", Nodes: []string{"GND", "IN", "OUT"},
		Devices:  []simmodel.ResolvedDevice{{Component: "input", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "IN"}, {Terminal: "NEGATIVE", Net: "GND"}}}},
		Analyses: []simmodel.Analysis{{ID: "dc", Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{{Component: "input"}}}},
	}
	intent, err := derivedGraphWorkflowIntent(requirement, base, simmodel.AnalysisTransient, "IN")
	if err != nil {
		t.Fatal(err)
	}
	analysis := intent.Analyses[0]
	if analysis.DurationS != 4e-3 || analysis.DurationS/analysis.TimeStepS != 2048 || analysis.TimeStepS >= 1/(behavioralDistortionFrequencyHz*behavioralDistortionSamplesPerCycle) {
		t.Fatalf("timing-constrained swing grid = duration %.12g step %.12g", analysis.DurationS, analysis.TimeStepS)
	}
}

func TestBoundedBehavioralDynamicGridNeverRoundsBelowMinimumStep(t *testing.T) {
	duration, step := boundedBehavioralDynamicGrid(200e-9, 1e-9, behavioralDynamicMaxSteps)
	steps := duration / step
	if step < 1e-9*(1-1e-12) || math.Abs(steps-math.Round(steps)) > 1e-12 || steps > 2048 {
		t.Fatalf("bounded grid = duration %.12g step %.12g steps %.12g", duration, step, steps)
	}
}

func TestBoundedBehavioralDynamicGridDoesNotCoarsenRequestedResolution(t *testing.T) {
	duration, step := boundedBehavioralDynamicGrid(10e-9, 3e-9, behavioralDynamicMaxSteps)
	if duration != 10e-9 || step > 3e-9 {
		t.Fatalf("bounded grid = duration %.12g step %.12g, want step no greater than 3e-9", duration, step)
	}
}

func TestBehavioralDynamicGridCoversAutonomousClockCycles(t *testing.T) {
	maximumEdgeTime := 200e-9
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{BehavioralRequirements: []architecturesearch.BehavioralRequirement{{
		ID: "rise", Metric: "rise_time", Analysis: simmodel.AnalysisTransient, Max: &maximumEdgeTime, Unit: "s",
	}}}}
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{
		Component: "clock", PrimitiveModel: simmodel.PrimitiveFixedClockSourceV1,
		ModelParameters: []simmodel.NamedValue{{Name: "frequency_hz", Value: 100e3}},
	}}}
	duration, step := behavioralDynamicGrid(requirement, plan, simmodel.AnalysisTransient)
	if duration < 30e-6 || step > 1/(100e3*40) || duration/step > behavioralAutonomousClockMaxSteps {
		t.Fatalf("autonomous clock grid = duration %.12g step %.12g steps %.12g", duration, step, duration/step)
	}
}

func TestBehavioralDynamicGridCoversProgrammedClockStartup(t *testing.T) {
	timingResistance := 1e6
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{
			Component: "clock", PrimitiveModel: simmodel.PrimitiveResistorProgrammedClockSourceV1,
			ModelParameters: []simmodel.NamedValue{
				{Name: "frequency_scale_hz_ohm", Value: 1e11},
				{Name: "divider_ratio", Value: 1},
				{Name: "startup_fixed_s", Value: 100e-6},
				{Name: "startup_cycles", Value: 64},
			},
			Terminals: []simmodel.TerminalBinding{{Terminal: "SET", Net: "SET"}, {Terminal: "GND", Net: "GND"}},
		},
		{
			Component: "timing", Usage: "timing_resistor", PrimitiveModel: simmodel.PrimitiveResistorV1, ValueSI: &timingResistance,
			Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "SET"}, {Terminal: "B", Net: "GND"}},
		},
	}}
	duration, step := behavioralDynamicGrid(architecturesearch.Requirement{}, plan, simmodel.AnalysisStartup)
	if duration+1e-15 < 740e-6 || step > 1/(100e3*20) || duration/step > behavioralDynamicMaxSteps {
		t.Fatalf("programmed clock startup grid = duration %.12g step %.12g steps %.12g", duration, step, duration/step)
	}
}

func TestResistorProgrammedClockFrequencyRequiresUniqueSetToGroundResistor(t *testing.T) {
	timingResistance := 1e6
	decoyResistance := 10e3
	source := simmodel.ResolvedDevice{
		Component: "clock", PrimitiveModel: simmodel.PrimitiveResistorProgrammedClockSourceV1,
		ModelParameters: []simmodel.NamedValue{
			{Name: "frequency_scale_hz_ohm", Value: 1e11},
			{Name: "divider_ratio", Value: 1},
		},
		Terminals: []simmodel.TerminalBinding{{Terminal: "SET", Net: "SET"}, {Terminal: "GND", Net: "GND"}},
	}
	timing := simmodel.ResolvedDevice{
		Component: "timing", Usage: "timing_resistor", PrimitiveModel: simmodel.PrimitiveResistorV1, ValueSI: &timingResistance,
		Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "SET"}, {Terminal: "B", Net: "GND"}},
	}
	decoy := simmodel.ResolvedDevice{
		Component: "decoy", PrimitiveModel: simmodel.PrimitiveResistorV1, ValueSI: &decoyResistance,
		Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "SET"}, {Terminal: "B", Net: "AUX"}},
	}
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{source, decoy, timing}}
	if got := resistorProgrammedClockFrequency(plan, source); got != 100e3 {
		t.Fatalf("programmed frequency = %g, want 100 kHz", got)
	}
	duplicate := timing
	duplicate.Component = "duplicate"
	plan.Devices = append(plan.Devices, duplicate)
	if got := resistorProgrammedClockFrequency(plan, source); got != 0 {
		t.Fatalf("ambiguous programmed frequency = %g, want fail-closed zero", got)
	}
}

func TestDerivedACSweepExtendsBeyondRequiredBandwidth(t *testing.T) {
	minimumBandwidth := 100_000.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{BehavioralRequirements: []architecturesearch.BehavioralRequirement{{
		ID: "bandwidth", Metric: "bandwidth", Analysis: simmodel.AnalysisACSweep, Min: &minimumBandwidth, Unit: "Hz",
	}}}}
	base := simmodel.Plan{
		ModelID: simmodel.ModelLinearCircuitMNAV1, GroundNode: "GND", Nodes: []string{"GND", "IN"},
		Devices:  []simmodel.ResolvedDevice{{Component: "input", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "IN"}, {Terminal: "NEGATIVE", Net: "GND"}}}},
		Analyses: []simmodel.Analysis{{ID: "ac", Kind: simmodel.AnalysisACSweep, StartFrequencyHz: 10, StopFrequencyHz: minimumBandwidth, Points: 64, Excitations: []simmodel.SourceExcitation{{Component: "input"}}}},
	}
	intent, err := derivedGraphWorkflowIntent(requirement, base, simmodel.AnalysisACSweep, "IN")
	if err != nil {
		t.Fatal(err)
	}
	if stop := intent.Analyses[0].StopFrequencyHz; stop < minimumBandwidth*10 {
		t.Fatalf("AC stop frequency %.12g does not bracket required bandwidth", stop)
	}
}

func TestDerivedACSweepBracketsRequiredCutoffRange(t *testing.T) {
	minimumCutoff, maximumCutoff := 18_000.0, 22_000.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{BehavioralRequirements: []architecturesearch.BehavioralRequirement{{
		ID: "cutoff", Metric: "cutoff_frequency", Analysis: simmodel.AnalysisACSweep, Min: &minimumCutoff, Max: &maximumCutoff, Unit: "Hz",
	}}}}
	base := simmodel.Plan{
		ModelID: simmodel.ModelLinearCircuitMNAV1, GroundNode: "GND", Nodes: []string{"GND", "IN"},
		Devices:  []simmodel.ResolvedDevice{{Component: "input", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "IN"}, {Terminal: "NEGATIVE", Net: "GND"}}}},
		Analyses: []simmodel.Analysis{{ID: "ac", Kind: simmodel.AnalysisACSweep, StartFrequencyHz: minimumCutoff, StopFrequencyHz: maximumCutoff, Points: 21, Excitations: []simmodel.SourceExcitation{{Component: "input"}}}},
	}
	intent, err := derivedGraphWorkflowIntent(requirement, base, simmodel.AnalysisACSweep, "IN")
	if err != nil {
		t.Fatal(err)
	}
	analysis := intent.Analyses[0]
	if analysis.StartFrequencyHz > minimumCutoff/10 || analysis.StopFrequencyHz < maximumCutoff*10 || analysis.Points < 64 {
		t.Fatalf("AC cutoff sweep = %#v, want a decade below and above the required range", analysis)
	}
}

func TestDerivedDistortionUsesBehavioralSwingGainAndOneSemanticInput(t *testing.T) {
	minimumGain, maximumGain, minimumSwing, minimumBandwidth := 7.6, 8.4, 8.0, 100_000.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{BehavioralRequirements: []architecturesearch.BehavioralRequirement{
		{ID: "gain", Metric: "voltage_gain", Min: &minimumGain, Max: &maximumGain, Unit: "ratio"},
		{ID: "swing", Metric: "output_swing", Min: &minimumSwing, Unit: "V_pp"},
		{ID: "bandwidth", Metric: "bandwidth", Min: &minimumBandwidth, Unit: "Hz"},
	}}}
	base := simmodel.Plan{
		ModelID:    simmodel.ModelNonlinearCircuitDCV1,
		GroundNode: "GND",
		Nodes:      []string{"GND", "IN", "OUT", "VCC"},
		Devices: []simmodel.ResolvedDevice{
			{Component: "input", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "IN"}, {Terminal: "NEGATIVE", Net: "GND"}}},
			{Component: "supply", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "VCC"}, {Terminal: "NEGATIVE", Net: "GND"}}},
		},
		Analyses: []simmodel.Analysis{{ID: "dc", Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{{Component: "input"}, {Component: "supply", DCValue: 18}}}},
	}
	intent, err := derivedGraphWorkflowIntent(requirement, base, simmodel.AnalysisDistortion, "IN")
	if err != nil {
		t.Fatal(err)
	}
	analysis := intent.Analyses[0]
	if analysis.DurationS != .004 || analysis.TimeStepS != 1/(behavioralDistortionFrequencyHz*behavioralDistortionSamplesPerCycle) {
		t.Fatalf("distortion grid = duration %.12g step %.12g", analysis.DurationS, analysis.TimeStepS)
	}
	input, supply := analysis.Excitations[0], analysis.Excitations[1]
	nominalGain := (minimumGain + maximumGain) / 2
	if input.SineAmplitude != minimumSwing/(2*nominalGain) || input.SineFrequencyHz != 1000 || input.DCValue != 0 {
		t.Fatalf("semantic distortion input = %#v", input)
	}
	if supply.DCValue != 18 || supply.SineFrequencyHz != 0 {
		t.Fatalf("distortion supply must remain DC-powered: %#v", supply)
	}
}

func TestDerivedDistortionMeasuresOutputPowerAtTheRequestedOperatingPoint(t *testing.T) {
	minimumGain, minimumPower, loadResistance := 20.0, 10.0, 8.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{
			{ID: "gain", Metric: "voltage_gain", Min: &minimumGain, Unit: "ratio"},
			{ID: "power", Metric: "output_power", Min: &minimumPower, Unit: "W"},
		},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{Axis: "load_resistance", Min: &loadResistance, Max: &loadResistance}}}},
	}}
	base := simmodel.Plan{
		ModelID: simmodel.ModelNonlinearCircuitDCV1, GroundNode: "GND", Nodes: []string{"GND", "IN"},
		Devices: []simmodel.ResolvedDevice{{
			Component: "input", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1,
			Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "IN"}, {Terminal: "NEGATIVE", Net: "GND"}},
		}},
		Analyses: []simmodel.Analysis{{ID: "dc", Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{{Component: "input"}}}},
	}
	intent, err := derivedGraphWorkflowIntent(requirement, base, simmodel.AnalysisDistortion, "IN")
	if err != nil {
		t.Fatal(err)
	}
	wantRatedAmplitude := math.Sqrt(2*minimumPower*loadResistance) / minimumGain
	if got := intent.Analyses[0].Excitations[0].SineAmplitude; got != wantRatedAmplitude {
		t.Fatalf("distortion input amplitude = %.12g, want exact rated-power amplitude %.12g", got, wantRatedAmplitude)
	}
	if guarded, ok := behavioralInputAmplitude(requirement); !ok || guarded != wantRatedAmplitude*behavioralRatedPowerVoltageGuard {
		t.Fatalf("guarded transient/thermal amplitude = %.12g, %t", guarded, ok)
	}
}

func TestBehavioralPowerStimulusUsesMaximumDeclaredLoadResistance(t *testing.T) {
	minimumPower, minimumLoad, maximumLoad := 15.0, 4.0, 8.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{{
			ID: "power", Metric: "output_power", Min: &minimumPower, Unit: "W",
		}},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{
			Axis: "load_resistance", Min: &minimumLoad, Max: &maximumLoad,
		}}}},
	}}
	got, ok := behavioralOutputSpanInputAmplitude(requirement)
	want := math.Sqrt(2*minimumPower*maximumLoad) * behavioralRatedPowerVoltageGuard
	if !ok || got != want {
		t.Fatalf("behavioral output-power amplitude = %.12g, %t; want %.12g", got, ok, want)
	}
}

func TestDerivedDistortionUsesUnityNominalWhenInputGainIsUnconstrained(t *testing.T) {
	minimumPower, loadResistance := 15.0, 4.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{
			{ID: "power", Metric: "output_power", Min: &minimumPower, Unit: "W"},
		},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{
			Axis: "load_resistance", Min: &loadResistance, Max: &loadResistance,
		}}}},
	}}
	base := simmodel.Plan{
		ModelID: simmodel.ModelNonlinearCircuitDCV1, GroundNode: "GND", Nodes: []string{"GND", "IN"},
		Devices: []simmodel.ResolvedDevice{{
			Component: "input", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1,
			Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "IN"}, {Terminal: "NEGATIVE", Net: "GND"}},
		}},
		Analyses: []simmodel.Analysis{{ID: "dc", Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{{Component: "input"}}}},
	}

	intent, err := derivedGraphWorkflowIntent(requirement, base, simmodel.AnalysisDistortion, "IN")
	if err != nil {
		t.Fatal(err)
	}
	want := math.Sqrt(2 * minimumPower * loadResistance)
	if got := intent.Analyses[0].Excitations[0].SineAmplitude; got != want {
		t.Fatalf("distortion input amplitude = %.12g, want unity-nominal %.12g", got, want)
	}
}

func TestEventObservedTransientDoesNotSuppressPeriodicPowerStimulus(t *testing.T) {
	minimumPower, loadResistance, maximumResponse := 15.0, 4.0, 1e-3
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Ports: []architecturesearch.Port{{
			ID: "input", Kind: "analog_voltage", Direction: "sink", Domain: "ground",
		}},
		Domains: []architecturesearch.Domain{{
			ID: "ground", Kind: "reference", NominalVoltageV: 0,
		}},
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{
			{ID: "power", Metric: "output_power", Analysis: simmodel.AnalysisTransient, Min: &minimumPower, Unit: "W"},
			{
				ID: "fault_response", Metric: "protection_response_time", Analysis: simmodel.AnalysisTransient,
				Observation: architecturesearch.Observation{Kind: "event", ID: "short_fault"},
				Max:         &maximumResponse, Unit: "s",
			},
		},
		OperatingCases: []architecturesearch.OperatingCase{{
			Conditions: []architecturesearch.OperatingCondition{{
				Axis: "load_resistance", Min: &loadResistance, Max: &loadResistance,
			}},
		}},
	}}
	stimulus, ok := behavioralTransientStimulusForRequirement(requirement, []closedloopsynthesis.SemanticBinding{{
		Kind: "port", ID: "input", Target: "IN",
	}})
	if !ok || !stimulus.Periodic || stimulus.Node != "IN" || stimulus.PeriodicFrequencyHz != behavioralDistortionFrequencyHz {
		t.Fatalf("periodic power stimulus = %#v, %t", stimulus, ok)
	}
}

func TestDerivedThermalUsesRatedPeriodicDrive(t *testing.T) {
	minimumGain, minimumPower, loadResistance := 19.0, 10.0, 8.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{
			{ID: "gain", Metric: "voltage_gain", Min: &minimumGain, Unit: "ratio"},
			{ID: "power", Metric: "output_power", Min: &minimumPower, Unit: "W"},
			{ID: "thermal", Metric: "junction_temperature", Analysis: simmodel.AnalysisThermal, Unit: "degC"},
		},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{Axis: "load_resistance", Min: &loadResistance, Max: &loadResistance}}}},
	}}
	base := simmodel.Plan{
		ModelID: simmodel.ModelNonlinearCircuitDCV1, GroundNode: "GND", Nodes: []string{"GND", "IN", "OUT", "VCC"},
		Devices: []simmodel.ResolvedDevice{
			{Component: "input", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "IN"}, {Terminal: "NEGATIVE", Net: "GND"}}},
			{Component: "supply", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "VCC"}, {Terminal: "NEGATIVE", Net: "GND"}}},
			{Component: "output", PrimitiveModel: simmodel.PrimitiveBJTNPNV1, ModelParameters: []simmodel.NamedValue{{Name: "junction_to_ambient_c_per_w", Value: 10}, {Name: "max_temperature_c", Value: 150}}},
		},
		Analyses: []simmodel.Analysis{{ID: "dc", Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{{Component: "input"}, {Component: "supply", DCValue: 18}}}},
	}
	intent, err := derivedGraphWorkflowIntent(requirement, base, simmodel.AnalysisThermal, "IN")
	if err != nil {
		t.Fatal(err)
	}
	analysis := intent.Analyses[0]
	wantAmplitude := math.Sqrt(2*minimumPower*loadResistance) * behavioralRatedPowerVoltageGuard / minimumGain
	if intent.ModelID != simmodel.ModelTransientCircuitV1 || analysis.Kind != simmodel.AnalysisThermal || analysis.DurationS != .004 || analysis.TimeStepS != 1/(behavioralDistortionFrequencyHz*behavioralDistortionSamplesPerCycle) {
		t.Fatalf("derived thermal workflow = model %s analysis %#v", intent.ModelID, analysis)
	}
	if analysis.Excitations[0].SineAmplitude != wantAmplitude || analysis.Excitations[0].SineFrequencyHz != 1000 || analysis.Excitations[1].SineFrequencyHz != 0 {
		t.Fatalf("derived thermal excitations = %#v", analysis.Excitations)
	}
	if len(analysis.Conditions) != 1 || analysis.Conditions[0].Name != "ambient_temperature_c" || analysis.Conditions[0].Value != 25 {
		t.Fatalf("derived thermal conditions = %#v", analysis.Conditions)
	}
}

func TestDerivedElectrothermalUsesBoundedDynamicGrid(t *testing.T) {
	maximumTemperature := 125.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{
			{ID: "thermal", Metric: "junction_temperature", Analysis: simmodel.AnalysisElectrothermal, Max: &maximumTemperature, Unit: "degC"},
		},
	}}
	base := simmodel.Plan{
		ModelID: simmodel.ModelNonlinearCircuitDCV1, GroundNode: "GND", Nodes: []string{"GND", "VCC"},
		Devices: []simmodel.ResolvedDevice{
			{Component: "supply", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "VCC"}, {Terminal: "NEGATIVE", Net: "GND"}}},
			{Component: "load", PrimitiveModel: simmodel.PrimitiveResistorV1, ModelParameters: []simmodel.NamedValue{{Name: "junction_to_ambient_c_per_w", Value: 10}, {Name: "max_temperature_c", Value: 150}}},
		},
		Analyses: []simmodel.Analysis{{ID: "dc", Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{{Component: "supply", DCValue: 5}}}},
	}
	intent, err := derivedGraphWorkflowIntent(requirement, base, simmodel.AnalysisElectrothermal, "VCC")
	if err != nil {
		t.Fatal(err)
	}
	analysis := intent.Analyses[0]
	if intent.ModelID != simmodel.ModelTransientCircuitV1 || analysis.Kind != simmodel.AnalysisElectrothermal || analysis.DurationS <= 0 || analysis.TimeStepS <= 0 {
		t.Fatalf("derived electrothermal workflow = model %s analysis %#v", intent.ModelID, analysis)
	}
	if len(analysis.Conditions) != 1 || analysis.Conditions[0].Name != "ambient_temperature_c" {
		t.Fatalf("derived electrothermal conditions = %#v", analysis.Conditions)
	}
	if len(intent.Assertions) != 1 || intent.Assertions[0].Component != "load" || intent.Assertions[0].Quantity != simmodel.QuantityJunctionTemperatureC {
		t.Fatalf("derived electrothermal assertion = %#v", intent.Assertions)
	}
}

func TestBehavioralCorpusResolvesAndExecutesTrustedAnalysisPlans(t *testing.T) {
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, registryIssues := architecturesearch.NewCatalogRegistry(catalog)
	if len(registryIssues) != 0 {
		t.Fatalf("registry issues = %#v", registryIssues)
	}
	provenance, provenanceDiagnostics := modelprovenance.LoadDefault()
	if len(provenanceDiagnostics) != 0 {
		t.Fatalf("provenance diagnostics = %#v", provenanceDiagnostics)
	}
	for _, fixture := range []struct{ id, file, expectedRejectionPath string }{
		{id: "active_filter_amplifier", file: "simulation_grounded_closed_loop_corpus/active_filter_amplifier.json"},
		{id: "class_a_amplifier", file: "simulation_grounded_closed_loop_corpus/class_a_amplifier.json"},
		{id: "class_ab_amplifier", file: "simulation_grounded_closed_loop_corpus/class_ab_amplifier.json"},
		{id: "current_sense_protection", file: "control_behavior_corpus/current_sense_protection.json", expectedRejectionPath: "requirements.behavioral_requirements[2]"},
		{id: "hysteretic_mosfet_load", file: "simulation_grounded_closed_loop_corpus/hysteretic_mosfet_load.json"},
		{id: "low_noise_sensor_decision", file: "simulation_grounded_closed_loop_corpus/low_noise_sensor_decision.json"},
		{id: "mixed_function_control_power", file: "control_behavior_corpus/mixed_function_control_power.json", expectedRejectionPath: "requirements.behavioral_requirements[2]"},
		{id: "protected_mixed_signal_interface", file: "simulation_grounded_closed_loop_corpus/protected_mixed_signal_interface.json"},
		{id: "regulated_sensor_interface", file: "simulation_grounded_closed_loop_corpus/regulated_sensor_interface.json"},
		{id: "split_supply_frontend", file: "simulation_grounded_closed_loop_corpus/split_supply_frontend.json"},
	} {
		t.Run(fixture.id, func(t *testing.T) {
			data, readErr := os.ReadFile("../architecturesearch/testdata/" + fixture.file)
			if readErr != nil {
				t.Fatal(readErr)
			}
			requirement, decodeIssues := architecturesearch.DecodeStrict(bytes.NewReader(data))
			if fixture.expectedRejectionPath != "" {
				if !slices.ContainsFunc(decodeIssues, func(issue reports.Issue) bool {
					return issue.Code == architecturesearch.CodeControlInvalid && issue.Path == fixture.expectedRejectionPath && strings.Contains(issue.Message, "declare a separate startup enable or sequencing dependency")
				}) {
					t.Fatalf("precise control rejection issues = %#v", decodeIssues)
				}
				return
			}
			if len(decodeIssues) != 0 {
				t.Fatalf("decode issues = %#v", decodeIssues)
			}
			search := architecturesearch.Search(context.Background(), requirement, registry, architecturesearch.SearchOptions{CatalogHash: "checked-in"})
			if search.Status != architecturesearch.SearchSelected || search.Selected == nil {
				t.Fatalf("search status=%s issues=%#v rejections=%#v", search.Status, search.Issues, search.Rejections)
			}
			resolver := ArchitectureSimulationPlanResolver{
				Requirement: requirement, Search: search, ProvenanceRegistry: provenance,
				GraphResolver: circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"}),
			}
			evaluator := closedloopsynthesis.SimModelEvaluator{
				Resolver: closedloopsynthesis.PlannedSimulationResolver{Base: resolver}, ProvenanceRegistry: provenance,
			}
			candidates := append([]architecturesearch.CandidateResult{*search.Selected}, search.Alternatives...)
			var evaluationErrors []string
			executed := false
			for _, candidate := range candidates {
				var selectionIDs []string
				for _, selection := range candidate.Selections {
					var componentIDs []string
					for _, component := range selection.Components {
						componentIDs = append(componentIDs, component.CatalogID)
					}
					selectionIDs = append(selectionIDs, selection.ExpansionID+"["+strings.Join(componentIDs, ",")+"]")
				}
				state := closedloopsynthesis.CandidateState{Fingerprint: candidate.Fingerprint}
				planSet, resolveErr := resolver.ResolveSimulationPlanSet(context.Background(), state)
				if resolveErr != nil {
					evaluationErrors = append(evaluationErrors, candidate.Fingerprint+" "+strings.Join(selectionIDs, "+")+": "+resolveErr.Error())
					continue
				}
				for _, required := range requiredBehavioralAnalyses(requirement) {
					if _, exists := planSet.Plans[required]; !exists {
						t.Fatalf("required analysis %s missing from %#v", required, planSet.Plans)
					}
				}
				evaluation, evaluateErr := evaluator.Evaluate(context.Background(), state)
				if evaluateErr != nil {
					evaluationErrors = append(evaluationErrors, candidate.Fingerprint+" "+strings.Join(selectionIDs, "+")+": "+evaluateErr.Error()+simulationPlanFailureNeighborhood(planSet.Plans, evaluateErr))
					continue
				}
				if evaluation.EvidenceHash == "" || len(evaluation.Measurements) != len(requirement.Requirements.BehavioralRequirements) {
					t.Fatalf("executed evaluation is incomplete: %#v", evaluation)
				}
				executed = true
				break
			}
			if !executed {
				t.Fatalf("no retained architecture executed all behavioral analyses: %v", evaluationErrors)
			}
		})
	}
}

func simulationPlanFailureNeighborhood(plans map[string]simmodel.Plan, failure error) string {
	const marker = "node:"
	message := failure.Error()
	start := strings.Index(message, marker)
	if start < 0 {
		return ""
	}
	start += len(marker)
	end := start
	for end < len(message) && message[end] != ':' && message[end] != ' ' {
		end++
	}
	node := message[start:end]
	if node == "" {
		return ""
	}
	keys := make([]string, 0, len(plans))
	for key := range plans {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	var neighbors []string
	for _, key := range keys {
		for _, device := range plans[key].Devices {
			for _, terminal := range device.Terminals {
				if terminal.Net == node {
					neighbors = append(neighbors, key+"/"+device.Component+"."+terminal.Terminal+"["+device.PrimitiveModel+"]")
				}
			}
		}
	}
	return " node " + node + " touches " + strings.Join(neighbors, ",")
}

func TestLoadCurrentHarnessSpansSemanticLoadSwitchPowerAndOutputRoles(t *testing.T) {
	minimum, maximum := 0.0, 2.0
	bindings := []closedloopsynthesis.SemanticBinding{
		{Kind: "domain", ID: "ground", Target: "GND"},
		{Kind: "port", ID: "power", Target: "SUPPLY"},
		{Kind: "port", ID: "load", Target: "SWITCHED"},
	}
	for _, supplyRole := range []string{"input", "power", "load_power"} {
		requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
			Domains:    []architecturesearch.Domain{{ID: "supply", Kind: "supply"}, {ID: "ground", Kind: "reference"}},
			Ports:      []architecturesearch.Port{{ID: "power", Domain: "supply"}, {ID: "load", Domain: "supply"}, {ID: "ground", Domain: "ground"}},
			Objectives: []architecturesearch.Objective{{Capability: "load_switch", Bindings: []architecturesearch.Binding{{Role: supplyRole, Port: "power"}, {Role: "output", Port: "load"}, {Role: "reference", Port: "ground"}}}},
			OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{
				Axis: "load_current", Target: "load", Min: &minimum, Max: &maximum, Unit: "A",
			}}}},
		}}
		devices, err := operatingHarnessDevices(requirement, bindings, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(devices) != 1 || !devices[0].Source || len(devices[0].Device.Connections) != 2 {
			t.Fatalf("%s load-current harness = %#v", supplyRole, devices)
		}
		connections := devices[0].Device.Connections
		if connections[0].Function != "POSITIVE" || connections[0].Net != "SUPPLY" || connections[1].Function != "NEGATIVE" || connections[1].Net != "SWITCHED" {
			t.Fatalf("%s load-current connections = %#v", supplyRole, connections)
		}
	}
}

func TestLoadInductanceHarnessSpansSemanticLoadSwitchPowerAndOutputRoles(t *testing.T) {
	minimum, maximum := 2e-3, 80e-3
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains:    []architecturesearch.Domain{{ID: "supply", Kind: "supply"}, {ID: "ground", Kind: "reference"}},
		Ports:      []architecturesearch.Port{{ID: "power", Domain: "supply"}, {ID: "load", Domain: "supply"}, {ID: "ground", Domain: "ground"}},
		Objectives: []architecturesearch.Objective{{Capability: "load_switch", Bindings: []architecturesearch.Binding{{Role: "input", Port: "power"}, {Role: "output", Port: "load"}, {Role: "reference", Port: "ground"}}}},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{
			Axis: "load_inductance", Target: "load", Min: &minimum, Max: &maximum, Unit: "H",
		}}}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{
		{Kind: "domain", ID: "ground", Target: "GND"},
		{Kind: "port", ID: "power", Target: "SUPPLY"},
		{Kind: "port", ID: "load", Target: "SWITCHED"},
	}

	devices, err := operatingHarnessDevices(requirement, bindings, nil, simmodel.AnalysisElectrothermal)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Source || devices[0].Device.CatalogID != "load.inductive.external.1x02" ||
		!devices[0].Device.HasValueSI || devices[0].Device.ValueSI != minimum {
		t.Fatalf("load-inductance harness = %#v", devices)
	}
	connections := devices[0].Device.Connections
	if len(connections) != 2 ||
		connections[0] != (simmodel.ConnectionEvidence{Function: "A", Net: "SUPPLY"}) ||
		connections[1] != (simmodel.ConnectionEvidence{Function: "B", Net: "SWITCHED"}) {
		t.Fatalf("load-inductance connections = %#v", connections)
	}
}

func TestCurrentAndInductanceHarnessesFormOneDeterministicSeriesLoad(t *testing.T) {
	minimumCurrent, maximumCurrent, inductance := .2, 3.0, 80e-3
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains:    []architecturesearch.Domain{{ID: "supply", Kind: "supply", NominalVoltageV: 24}, {ID: "ground", Kind: "reference"}},
		Ports:      []architecturesearch.Port{{ID: "power", Domain: "supply"}, {ID: "load", Domain: "supply"}, {ID: "ground", Domain: "ground"}},
		Objectives: []architecturesearch.Objective{{Capability: "load_switch", Bindings: []architecturesearch.Binding{{Role: "power", Port: "power"}, {Role: "output", Port: "load"}, {Role: "reference", Port: "ground"}}}},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{
			{Axis: "load_current", Target: "load", Min: &minimumCurrent, Max: &maximumCurrent, Unit: "A"},
			{Axis: "load_inductance", Target: "load", Min: &inductance, Max: &inductance, Unit: "H"},
		}}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{
		{Kind: "domain", ID: "ground", Target: "GND"},
		{Kind: "port", ID: "power", Target: "SUPPLY"},
		{Kind: "port", ID: "load", Target: "SWITCHED"},
	}

	devices, err := operatingHarnessDevices(requirement, bindings, nil, simmodel.AnalysisElectrothermal)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 {
		t.Fatalf("series load devices = %#v", devices)
	}
	seriesNode := operatingHarnessSeriesNode("SWITCHED")
	edges := map[string][2]string{}
	for _, entry := range devices {
		connections := entry.Device.Connections
		edges[entry.Device.CatalogID] = [2]string{connections[0].Net, connections[1].Net}
	}
	if edges["resistor.generic.0603"] != [2]string{"SUPPLY", seriesNode} ||
		edges["load.inductive.external.1x02"] != [2]string{seriesNode, "SWITCHED"} {
		t.Fatalf("series load edges = %#v", edges)
	}
}

func TestOperatingHarnessUsesTargetDomainReferenceForIsolatedLoadCapacitance(t *testing.T) {
	minimum, maximum := 1e-12, 400e-12
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{
			{ID: "host_5v", Kind: "supply", NominalVoltageV: 5},
			{ID: "host_ground", Kind: "reference"},
			{ID: "remote_1v8", Kind: "supply", NominalVoltageV: 1.8},
			{ID: "remote_ground", Kind: "reference"},
		},
		Ports: []architecturesearch.Port{
			{ID: "host_bus", Kind: "digital_bus", Direction: "bidirectional", Domain: "host_5v"},
			{ID: "remote_bus", Kind: "digital_bus", Direction: "bidirectional", Domain: "remote_1v8"},
		},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{
			Axis: "load_capacitance", Target: "remote_bus", Min: &minimum, Max: &maximum, Unit: "F",
		}}}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{
		{Kind: "domain", ID: "host_ground", Target: "HOST_GROUND"},
		{Kind: "domain", ID: "remote_ground", Target: "REMOTE_GROUND"},
		{Kind: "port", ID: "remote_bus", Target: "REMOTE_BUS"},
	}
	devices, err := operatingHarnessDevices(requirement, bindings, nil, simmodel.AnalysisTransient)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("load-capacitance harness = %#v", devices)
	}
	connections := devices[0].Device.Connections
	if len(connections) != 2 || connections[0].Net != "REMOTE_BUS" || connections[1].Net != "REMOTE_GROUND" {
		t.Fatalf("isolated load-capacitance connections = %#v", connections)
	}
}

func TestParticipantControlOutputHarnessIsAssertedExceptDuringStartup(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{
			{ID: "logic", Kind: "supply", NominalVoltageV: 5},
			{ID: "ground", Kind: "reference"},
		},
		Participants: []architecturesearch.Participant{{
			ID: "controller", Domain: "logic",
			RequiredPorts: []architecturesearch.ParticipantPort{{
				ID: "enable", Kind: "digital_logic", Direction: "source",
				Protocol: &architecturesearch.Protocol{Name: "gpio", Mode: "push_pull"},
			}},
		}},
		Objectives: []architecturesearch.Objective{{
			Capability: "controlled_function",
			Bindings:   []architecturesearch.Binding{{Role: "control", Participant: "controller", ParticipantPort: "enable"}},
		}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{
		{Kind: "domain", ID: "ground", Target: "GND"},
		{Kind: "participant_port", ID: "controller.enable", Target: "CONTROL"},
	}
	ordinary, err := operatingHarnessDevices(requirement, bindings, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	startup, err := operatingHarnessDevices(requirement, bindings, nil, simmodel.AnalysisStartup)
	if err != nil {
		t.Fatal(err)
	}
	transient, err := operatingHarnessDevices(requirement, bindings, nil, simmodel.AnalysisTransient)
	if err != nil {
		t.Fatal(err)
	}
	if len(ordinary) != 1 || !ordinary[0].Source || !ordinary[0].HasDefaultValue || ordinary[0].DefaultValue != 5 {
		t.Fatalf("ordinary participant output harness = %#v", ordinary)
	}
	if len(startup) != 1 || startup[0].DefaultValue != 0 {
		t.Fatalf("startup participant output harness = %#v", startup)
	}
	if len(transient) != 1 || !transient[0].TransientEdge {
		t.Fatalf("transient participant output harness = %#v", transient)
	}
	intent := simmodel.Intent{Analyses: []simmodel.Analysis{{
		Kind: simmodel.AnalysisTransient, DurationS: 10e-6, TimeStepS: 1e-6,
	}}}
	addOperatingHarnessExcitations(&intent, transient)
	excitation := intent.Analyses[0].Excitations[0]
	if excitation.DCValue != 0 || excitation.PulseInitialValue != 0 ||
		excitation.PulseValue != 5 || excitation.PulseDelayS != 2e-6 ||
		excitation.PulseWidthS != 10e-6 || math.Abs(excitation.PulsePeriodS-11e-6) > 1e-18 {
		t.Fatalf("transient participant control edge = %#v", excitation)
	}
	if ordinary[0].Device.InstanceID != startup[0].Device.InstanceID ||
		ordinary[0].Device.Connections[0].Net != "CONTROL" ||
		ordinary[0].Device.Connections[1].Net != "GND" {
		t.Fatalf("participant output harness identity or connectivity differs: ordinary=%#v startup=%#v", ordinary, startup)
	}
}

func TestParticipantBehavioralOutputHarnessDrivesOppositeObservedBoundary(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{
			{ID: "target_1v8", Kind: "supply", NominalVoltageV: 1.8},
			{ID: "ground", Kind: "reference"},
		},
		Ports: []architecturesearch.Port{{
			ID: "host_debug", Kind: "digital_bus", Direction: "source",
		}},
		Participants: []architecturesearch.Participant{{
			ID: "controller", Domain: "target_1v8",
			RequiredPorts: []architecturesearch.ParticipantPort{{
				ID: "debug", Kind: "digital_bus", Direction: "bidirectional",
				Protocol: &architecturesearch.Protocol{Name: "swd", Mode: "push_pull"},
			}},
		}},
		Objectives: []architecturesearch.Objective{{
			Capability: "logic_level_translation",
			Bindings: []architecturesearch.Binding{
				{Role: "side_a", Port: "host_debug"},
				{Role: "side_b", Participant: "controller", ParticipantPort: "debug"},
			},
		}},
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{{
			ID: "edge", Metric: "rise_time", Analysis: simmodel.AnalysisTransient,
			Observation: architecturesearch.Observation{Kind: "port", ID: "host_debug"},
		}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{
		{Kind: "domain", ID: "ground", Target: "GND"},
		{Kind: "port", ID: "host_debug", Target: "HOST"},
		{Kind: "participant_port", ID: "controller.debug", Target: "TARGET"},
	}
	if stimulus, ok := behavioralTransientStimulusForRequirement(requirement, bindings); ok {
		t.Fatalf("external stimulus must be suppressed when the opposite participant is the behavioral source: %#v", stimulus)
	}
	transient, err := operatingHarnessDevices(requirement, bindings, nil, simmodel.AnalysisTransient)
	if err != nil {
		t.Fatal(err)
	}
	startup, err := operatingHarnessDevices(requirement, bindings, nil, simmodel.AnalysisStartup)
	if err != nil {
		t.Fatal(err)
	}
	if len(transient) != 1 || !transient[0].Source || !transient[0].TransientEdge ||
		transient[0].DefaultValue != 1.8 || len(transient[0].Device.Connections) != 2 ||
		transient[0].Device.Connections[0].Net != "TARGET" || transient[0].Device.Connections[1].Net != "GND" {
		t.Fatalf("participant behavioral transient harness = %#v", transient)
	}
	if len(startup) != 1 || startup[0].DefaultValue != 0 || startup[0].TransientEdge {
		t.Fatalf("participant behavioral startup harness = %#v", startup)
	}
}

func TestBehavioralPrimaryInputIgnoresControlOnlyPort(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Ports: []architecturesearch.Port{{
			ID: "shutdown", Kind: "digital_logic", Direction: "sink",
		}},
		Objectives: []architecturesearch.Objective{{
			Capability: "voltage_regulation",
			Bindings:   []architecturesearch.Binding{{Role: "shutdown", Port: "shutdown"}},
		}},
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{{
			Analysis:    simmodel.AnalysisThermal,
			Observation: architecturesearch.Observation{Kind: "domain", ID: "output"},
		}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{{Kind: "port", ID: "shutdown", Target: "SHUTDOWN"}}
	if port, node, ok := behavioralPrimaryInputPort(requirement, bindings); ok {
		t.Fatalf("control-only port was selected as primary signal input: port=%#v node=%q", port, node)
	}
}

func TestBehavioralStartupDefaultsDigitalInputsLowWithoutChangingPower(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Ports: []architecturesearch.Port{
			{ID: "command", Kind: "digital_logic", Electrical: &architecturesearch.Electrical{DefaultState: "Inactive"}},
			{ID: "power", Kind: "power"},
		},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{
		{Kind: "port", ID: "command", Target: "COMMAND"},
		{Kind: "port", ID: "power", Target: "VCC"},
	}
	plan := simmodel.Plan{
		Devices: []simmodel.ResolvedDevice{
			{Component: "command_source", PrimitiveModel: simmodel.PrimitiveConnectorVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "PIN_1", Net: "COMMAND"}, {Terminal: "PIN_2", Net: "GND"}}},
			{Component: "power_source", PrimitiveModel: simmodel.PrimitiveConnectorVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "PIN_1", Net: "VCC"}, {Terminal: "PIN_2", Net: "GND"}}},
		},
		Analyses: []simmodel.Analysis{{
			Kind: simmodel.AnalysisStartup,
			Excitations: []simmodel.SourceExcitation{
				{Component: "command_source", DCValue: 3.3, PulseValue: 3.3},
				{Component: "power_source", DCValue: 3.3, PulseValue: 3.3},
			},
		}},
	}
	got := simmodel.ClonePlan(plan)
	if err := configureBehavioralStartupInputState(&got, simmodel.AnalysisStartup, requirement, behavioralDomainNominalVoltageIndex(requirement), bindings); err != nil {
		t.Fatal(err)
	}
	if got.Analyses[0].Excitations[0].DCValue != 0 || got.Analyses[0].Excitations[0].PulseValue != 0 {
		t.Fatalf("startup digital input = %#v, want inactive low", got.Analyses[0].Excitations[0])
	}
	if got.Analyses[0].Excitations[1].DCValue != 3.3 || got.Analyses[0].Excitations[1].PulseValue != 3.3 {
		t.Fatalf("startup power source changed = %#v", got.Analyses[0].Excitations[1])
	}
}

func TestBehavioralStartupDeassertsActiveLowControlHigh(t *testing.T) {
	nominal := 1.8
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{{ID: "logic", NominalVoltageV: nominal}},
		Ports: []architecturesearch.Port{{
			ID: "enable", Kind: "digital_logic", Domain: "logic",
			Electrical: &architecturesearch.Electrical{MaxVoltageV: &nominal, DefaultState: "inactive"},
		}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{{Kind: "port", ID: "enable", Target: "OE"}}
	plan := simmodel.Plan{
		Devices: []simmodel.ResolvedDevice{
			{
				Component: "enable_source", PrimitiveModel: simmodel.PrimitiveConnectorVoltageSourceV1,
				Terminals: []simmodel.TerminalBinding{{Terminal: "PIN_1", Net: "OE"}, {Terminal: "PIN_2", Net: "GND"}},
			},
			{
				Component: "translator", PrimitiveModel: simmodel.PrimitiveDirectionControlledTranslatorV1,
				Terminals: []simmodel.TerminalBinding{{Terminal: "OE", Net: "OE"}},
			},
		},
		Analyses: []simmodel.Analysis{{
			ID: "startup", Kind: simmodel.AnalysisStartup,
			Excitations: []simmodel.SourceExcitation{{Component: "enable_source", PulseValue: 1}},
		}},
	}
	if err := configureBehavioralStartupInputState(&plan, simmodel.AnalysisStartup, requirement, behavioralDomainNominalVoltageIndex(requirement), bindings); err != nil {
		t.Fatal(err)
	}
	if excitation := plan.Analyses[0].Excitations[0]; excitation.DCValue != nominal || excitation.PulseValue != 0 {
		t.Fatalf("active-low inactive startup source = %#v, want steady %.12g V", excitation, nominal)
	}
}

func TestBehavioralTransferControlsUseAnalysisSpecificSupplyForParticipantSource(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Ports: []architecturesearch.Port{
			{ID: "target", Kind: "digital_bus"},
			{ID: "enable", Kind: "digital_logic"},
			{ID: "direction", Kind: "digital_logic"},
		},
		Participants: []architecturesearch.Participant{{
			ID: "host", RequiredPorts: []architecturesearch.ParticipantPort{{
				ID: "debug", Kind: "digital_bus", Direction: "bidirectional",
			}},
		}},
		Objectives: []architecturesearch.Objective{{
			Capability: "logic_level_translation",
			Bindings: []architecturesearch.Binding{
				{Role: "side_a", Participant: "host", ParticipantPort: "debug"},
				{Role: "side_b", Port: "target"},
				{Role: "enable", Port: "enable"},
				{Role: "direction_control", Port: "direction"},
			},
		}},
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{{
			Analysis:    simmodel.AnalysisTransient,
			Observation: architecturesearch.Observation{Kind: "port", ID: "target"},
		}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{
		{Kind: "port", ID: "target", Target: "TARGET"},
		{Kind: "port", ID: "enable", Target: "OE"},
		{Kind: "port", ID: "direction", Target: "DIR"},
		{Kind: "participant_port", ID: "host.debug", Target: "HOST"},
	}
	plan := simmodel.Plan{
		Devices: []simmodel.ResolvedDevice{
			{Component: "vcca_source", PrimitiveModel: simmodel.PrimitiveConnectorVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "PIN_1", Net: "VCCA"}, {Terminal: "PIN_2", Net: "GND"}}},
			{Component: "enable_source", PrimitiveModel: simmodel.PrimitiveConnectorVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "PIN_1", Net: "GND"}, {Terminal: "PIN_2", Net: "OE"}}},
			{Component: "direction_source", PrimitiveModel: simmodel.PrimitiveConnectorVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "PIN_1", Net: "GND"}, {Terminal: "PIN_2", Net: "DIR"}}},
			{Component: "translator", PrimitiveModel: simmodel.PrimitiveDirectionControlledTranslatorV1, Terminals: []simmodel.TerminalBinding{
				{Terminal: "A1", Net: "HOST"}, {Terminal: "B1", Net: "TARGET"},
				{Terminal: "VCCA", Net: "VCCA"}, {Terminal: "OE", Net: "OE"},
				{Terminal: "DIR1", Net: "DIR"}, {Terminal: "DIR2", Net: "DIR"},
			}},
		},
		Analyses: []simmodel.Analysis{
			{
				ID:   "first",
				Kind: simmodel.AnalysisTransient,
				Excitations: []simmodel.SourceExcitation{
					{Component: "vcca_source", DCValue: 3.3},
					{Component: "enable_source", DCValue: -1.8, PulseValue: 1.8, PulsePeriodS: 1e-3},
					{Component: "direction_source", DCValue: -1.8, ACMagnitude: 1, ACPhaseDeg: 45},
				},
			},
			{
				ID:   "second",
				Kind: simmodel.AnalysisTransient,
				Excitations: []simmodel.SourceExcitation{
					{Component: "vcca_source", DCValue: 5},
					{Component: "enable_source", DCValue: -3.3, SineAmplitude: 1, SineFrequencyHz: 1000},
					{Component: "direction_source", DCValue: -3.3, PulseValue: 3.3, PulsePeriodS: 2e-3},
				},
			},
		},
	}
	got := simmodel.ClonePlan(plan)
	err := configureBehavioralTransferControls(&got, simmodel.AnalysisTransient, requirement, bindings)
	if err != nil {
		t.Fatal(err)
	}
	for index := range got.Analyses {
		if got.Analyses[index].Excitations[1].DCValue != 0 {
			t.Fatalf("analysis %d active-low enable source = %#v, want zero", index, got.Analyses[index].Excitations[1])
		}
		wantDirection := []float64{-3.3, -5}[index]
		if got.Analyses[index].Excitations[2].DCValue != wantDirection {
			t.Fatalf("analysis %d A-to-B direction source = %#v, want %.12g", index, got.Analyses[index].Excitations[2], wantDirection)
		}
	}
	if plan.Analyses[0].Excitations[1].DCValue != -1.8 {
		t.Fatal("transfer control configuration mutated the input plan")
	}
	if got.Analyses[0].Excitations[1].PulseValue != 0 ||
		got.Analyses[0].Excitations[2].ACMagnitude != 0 ||
		got.Analyses[1].Excitations[1].SineAmplitude != 0 ||
		got.Analyses[1].Excitations[2].PulseValue != 0 {
		t.Fatalf("transfer controls retain active waveform drive: %#v", got.Analyses)
	}
	if got.Analyses[0].Excitations[1].PulsePeriodS != 1e-3 ||
		got.Analyses[0].Excitations[2].ACPhaseDeg != 45 ||
		got.Analyses[1].Excitations[1].SineFrequencyHz != 1000 ||
		got.Analyses[1].Excitations[2].PulsePeriodS != 2e-3 {
		t.Fatalf("transfer control configuration discarded waveform provenance: %#v", got.Analyses)
	}
}

func TestPlanSourceNodeVoltageUsesRequestedAnalysis(t *testing.T) {
	plan := simmodel.Plan{
		Devices: []simmodel.ResolvedDevice{{
			Component:      "supply",
			PrimitiveModel: simmodel.PrimitiveConnectorVoltageSourceV1,
			Terminals: []simmodel.TerminalBinding{
				{Terminal: "PIN_1", Net: "VCC"},
				{Terminal: "PIN_2", Net: "GND"},
			},
		}},
		Analyses: []simmodel.Analysis{
			{ID: "first", Excitations: []simmodel.SourceExcitation{{Component: "supply", DCValue: 3.3}}},
			{ID: "second", Excitations: []simmodel.SourceExcitation{{Component: "supply", DCValue: 5}}},
		},
	}
	if voltage, ok := planSourceNodeVoltageForAnalysis(&plan, 0, "VCC"); !ok || voltage != 3.3 {
		t.Fatalf("first-analysis voltage = %.12g, %t; want 3.3, true", voltage, ok)
	}
	if voltage, ok := planSourceNodeVoltageForAnalysis(&plan, 1, "VCC"); !ok || voltage != 5 {
		t.Fatalf("second-analysis voltage = %.12g, %t; want 5, true", voltage, ok)
	}
	if voltage, ok := planSourceNodeVoltageForAnalysis(&plan, 2, "VCC"); ok {
		t.Fatalf("out-of-range analysis voltage = %.12g, true; want false", voltage)
	}
}

func TestLoadCurrentHarnessUsesGroundReferencedPhysicalLoadForHighSideSwitch(t *testing.T) {
	minimum, maximum := 0.0, 2.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains:        []architecturesearch.Domain{{ID: "supply", Kind: "supply"}, {ID: "ground", Kind: "reference"}},
		Ports:          []architecturesearch.Port{{ID: "power", Domain: "supply"}, {ID: "load", Domain: "supply"}, {ID: "ground", Domain: "ground"}},
		Objectives:     []architecturesearch.Objective{{Capability: "load_switch", Bindings: []architecturesearch.Binding{{Role: "power", Port: "power"}, {Role: "output", Port: "load"}, {Role: "reference", Port: "ground"}}}},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{Axis: "load_current", Target: "load", Min: &minimum, Max: &maximum, Unit: "A"}}}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{{Kind: "domain", ID: "ground", Target: "GND"}, {Kind: "port", ID: "power", Target: "SUPPLY"}, {Kind: "port", ID: "load", Target: "SWITCHED"}}
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{PrimitiveModel: simmodel.PrimitivePMOSSwitchV1, Terminals: []simmodel.TerminalBinding{{Terminal: "SOURCE", Net: "SUPPLY"}, {Terminal: "DRAIN", Net: "SWITCHED"}}}}}
	devices, err := operatingHarnessDevices(requirement, bindings, &plan, "")
	if err != nil {
		t.Fatal(err)
	}
	connections := devices[0].Device.Connections
	if connections[0].Net != "SWITCHED" || connections[1].Net != "GND" {
		t.Fatalf("high-side load-current connections = %#v", connections)
	}
}

func TestOperatingLoadUsesExplicitObjectiveReferenceAcrossIsolationBoundary(t *testing.T) {
	minimum, maximum := 0.02, 0.25
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{
			{ID: "primary_power", Kind: "supply"},
			{ID: "secondary_power", Kind: "supply"},
			{ID: "primary_return", Kind: "reference"},
			{ID: "secondary_return", Kind: "reference"},
		},
		Ports: []architecturesearch.Port{
			{ID: "input", Kind: "power", Domain: "primary_power"},
			{ID: "output", Kind: "power", Domain: "secondary_power"},
			{ID: "secondary_return", Kind: "reference", Domain: "secondary_return"},
		},
		Signals: []architecturesearch.Signal{{ID: "regulated", Kind: "power", Domain: "secondary_power"}},
		Objectives: []architecturesearch.Objective{{
			Capability: "voltage_regulation",
			Bindings: []architecturesearch.Binding{
				{Role: "input", Port: "input"},
				{Role: "output", Signal: "regulated"},
				{Role: "reference", Port: "secondary_return"},
			},
		}, {
			Capability: "transient_protection",
			Bindings: []architecturesearch.Binding{
				{Role: "input", Signal: "regulated"},
				{Role: "output", Port: "output"},
				{Role: "reference", Port: "secondary_return"},
			},
		}},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{
			Axis: "load_current", Target: "secondary_power", Min: &minimum, Max: &maximum, Unit: "A",
		}}}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{
		{Kind: "domain", ID: "primary_return", Target: "PRIMARY_GROUND"},
		{Kind: "domain", ID: "secondary_return", Target: "SECONDARY_GROUND"},
		{Kind: "domain", ID: "secondary_power", Target: "OUTPUT"},
	}
	harness, err := operatingHarnessDevices(requirement, bindings, nil, simmodel.AnalysisDCOperatingPoint)
	if err != nil {
		t.Fatal(err)
	}
	if len(harness) != 1 {
		t.Fatalf("load harness = %#v", harness)
	}
	connections := harness[0].Device.Connections
	if connections[0].Net != "OUTPUT" || connections[1].Net != "SECONDARY_GROUND" {
		t.Fatalf("isolated load endpoints = %#v, want output-to-secondary-return", connections)
	}
}

func TestOperatingLoadBudgetsCatalogBackedParallelSupportCurrent(t *testing.T) {
	minimum, maximum := .02, .25
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{
			{ID: "rail", Kind: "supply", NominalVoltageV: 12},
			{ID: "return", Kind: "reference"},
		},
		Ports: []architecturesearch.Port{
			{ID: "output", Kind: "power", Domain: "rail"},
			{ID: "return", Kind: "reference", Domain: "return"},
		},
		Objectives: []architecturesearch.Objective{{
			Capability: "voltage_regulation",
			Bindings: []architecturesearch.Binding{
				{Role: "output", Port: "output"},
				{Role: "reference", Port: "return"},
			},
		}},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{
			Axis: "load_current", Target: "rail", Min: &minimum, Max: &maximum, Unit: "A",
		}}}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{
		{Kind: "domain", ID: "rail", Target: "OUTPUT"},
		{Kind: "domain", ID: "return", Target: "GROUND"},
	}
	resistance := 240_000.0
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{
		Component: "discharge", PrimitiveModel: simmodel.PrimitiveResistorV1, ValueSI: &resistance,
		Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "OUTPUT"}, {Terminal: "B", Net: "GROUND"}},
	}}}
	harness, err := operatingHarnessDevices(requirement, bindings, &plan, simmodel.AnalysisDCOperatingPoint)
	if err != nil {
		t.Fatal(err)
	}
	if len(harness) != 1 {
		t.Fatalf("load harness = %#v", harness)
	}
	wantSupport := 12 / resistance
	if math.Abs(harness[0].DefaultValue-(maximum-wantSupport)) > 1e-15 ||
		math.Abs(harness[0].InitialValue-(minimum-wantSupport)) > 1e-15 {
		t.Fatalf("budgeted load-current harness = %#v, want total-load corners less %.12g A support", harness[0], wantSupport)
	}
}

func TestLoadCurrentHarnessUsesResolvedDownstreamCurrentSenseNode(t *testing.T) {
	minimum, maximum := 0.0, 3.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{{ID: "supply", Kind: "supply"}, {ID: "ground", Kind: "reference"}},
		Ports:   []architecturesearch.Port{{ID: "power", Domain: "supply"}, {ID: "load", Domain: "supply"}, {ID: "ground", Domain: "ground"}},
		Objectives: []architecturesearch.Objective{
			{Capability: "current_sensing", Bindings: []architecturesearch.Binding{{Role: "power", Port: "power"}}},
			{Capability: "load_switch", Bindings: []architecturesearch.Binding{{Role: "power", Port: "power"}, {Role: "output", Port: "load"}, {Role: "reference", Port: "ground"}}},
		},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{Axis: "load_current", Target: "load", Min: &minimum, Max: &maximum, Unit: "A"}}}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{{Kind: "domain", ID: "ground", Target: "GND"}, {Kind: "port", ID: "power", Target: "SUPPLY"}, {Kind: "port", ID: "load", Target: "SWITCHED"}}
	value := .01
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{Component: "sensor", PrimitiveModel: simmodel.PrimitiveCurrentSenseAmplifierV1, Terminals: []simmodel.TerminalBinding{{Terminal: "IN_PLUS", Net: "SUPPLY"}, {Terminal: "IN_MINUS", Net: "SENSED"}}},
		{Component: "shunt", PrimitiveModel: simmodel.PrimitiveResistorV1, ValueSI: &value, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "SUPPLY"}, {Terminal: "B", Net: "SENSED"}}},
	}}
	devices, err := operatingHarnessDevices(requirement, bindings, &plan, "")
	if err != nil {
		t.Fatal(err)
	}
	connections := devices[0].Device.Connections
	if connections[0].Net != "SENSED" || connections[1].Net != "SWITCHED" {
		t.Fatalf("load-current connections = %#v", connections)
	}
}

func TestLoadCurrentHarnessUsesResolvedSwitchedLoadSenseNode(t *testing.T) {
	minimum, maximum := 0.0, 3.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{{ID: "supply", Kind: "supply"}, {ID: "ground", Kind: "reference"}},
		Ports:   []architecturesearch.Port{{ID: "power", Domain: "supply"}, {ID: "load", Kind: "switched_load", Domain: "supply"}, {ID: "ground", Domain: "ground"}},
		Objectives: []architecturesearch.Objective{
			{Capability: "current_sensing", Bindings: []architecturesearch.Binding{{Role: "input", Port: "load"}}},
			{Capability: "load_switch", Bindings: []architecturesearch.Binding{{Role: "power", Port: "power"}, {Role: "output", Port: "load"}, {Role: "reference", Port: "ground"}}},
		},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{Axis: "load_current", Target: "load", Min: &minimum, Max: &maximum, Unit: "A"}}}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{{Kind: "domain", ID: "ground", Target: "GND"}, {Kind: "port", ID: "power", Target: "SUPPLY"}, {Kind: "port", ID: "load", Target: "SWITCHED"}}
	value := .01
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{Component: "sensor", PrimitiveModel: simmodel.PrimitiveCurrentSenseAmplifierV1, Terminals: []simmodel.TerminalBinding{{Terminal: "IN_PLUS", Net: "SENSED"}, {Terminal: "IN_MINUS", Net: "SWITCHED"}}},
		{Component: "shunt", PrimitiveModel: simmodel.PrimitiveResistorV1, ValueSI: &value, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "SWITCHED"}, {Terminal: "B", Net: "SENSED"}}},
	}}
	devices, err := operatingHarnessDevices(requirement, bindings, &plan, simmodel.AnalysisTransient)
	if err != nil {
		t.Fatal(err)
	}
	connections := devices[0].Device.Connections
	if connections[0].Net != "SUPPLY" || connections[1].Net != "SENSED" {
		t.Fatalf("switched-load current connections = %#v", connections)
	}
}

func TestDCLoadCurrentHarnessImposesSensedCurrentIndependentlyOfActuator(t *testing.T) {
	minimum, maximum := 0.0, 3.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{{ID: "supply", Kind: "supply"}, {ID: "ground", Kind: "reference"}},
		Ports:   []architecturesearch.Port{{ID: "power", Domain: "supply"}, {ID: "load", Domain: "supply"}, {ID: "ground", Domain: "ground"}},
		Objectives: []architecturesearch.Objective{
			{Capability: "current_sensing", Bindings: []architecturesearch.Binding{{Role: "power", Port: "power"}}},
			{Capability: "load_switch", Bindings: []architecturesearch.Binding{{Role: "power", Port: "power"}, {Role: "output", Port: "load"}, {Role: "reference", Port: "ground"}}},
		},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{Axis: "load_current", Target: "load", Min: &minimum, Max: &maximum, Unit: "A"}}}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{{Kind: "domain", ID: "ground", Target: "GND"}, {Kind: "port", ID: "power", Target: "SUPPLY"}, {Kind: "port", ID: "load", Target: "SWITCHED"}}
	value := .01
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{Component: "sensor", PrimitiveModel: simmodel.PrimitiveCurrentSenseAmplifierV1, Terminals: []simmodel.TerminalBinding{{Terminal: "IN_PLUS", Net: "SUPPLY"}, {Terminal: "IN_MINUS", Net: "SENSED"}}},
		{Component: "shunt", PrimitiveModel: simmodel.PrimitiveResistorV1, ValueSI: &value, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "SUPPLY"}, {Terminal: "B", Net: "SENSED"}}},
	}}
	devices, err := operatingHarnessDevices(requirement, bindings, &plan, simmodel.AnalysisDCOperatingPoint)
	if err != nil {
		t.Fatal(err)
	}
	connections := devices[0].Device.Connections
	if connections[0].Net != "SENSED" || connections[1].Net != "GND" {
		t.Fatalf("DC sensed-current connections = %#v", connections)
	}
}

func TestStartupLoadCurrentHarnessUsesVoltageDependentPhysicalLoad(t *testing.T) {
	minimum, maximum := 0.0, 3.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{{ID: "supply", Kind: "supply", NominalVoltageV: 12}, {ID: "ground", Kind: "reference"}},
		Ports:   []architecturesearch.Port{{ID: "power", Domain: "supply"}, {ID: "load", Domain: "supply"}, {ID: "ground", Domain: "ground"}},
		Objectives: []architecturesearch.Objective{{Capability: "load_switch", Bindings: []architecturesearch.Binding{
			{Role: "power", Port: "power"}, {Role: "output", Port: "load"}, {Role: "reference", Port: "ground"},
		}}},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{Axis: "load_current", Target: "load", Min: &minimum, Max: &maximum, Unit: "A"}}}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{{Kind: "domain", ID: "ground", Target: "GND"}, {Kind: "port", ID: "power", Target: "SUPPLY"}, {Kind: "port", ID: "load", Target: "SWITCHED"}}
	devices, err := operatingHarnessDevices(requirement, bindings, nil, simmodel.AnalysisStartup)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Source || !devices[0].Device.HasValueSI || math.Abs(devices[0].Device.ValueSI-4) > 1e-12 || devices[0].Device.CatalogID != "resistor.generic.0603" {
		t.Fatalf("startup load harness = %#v", devices)
	}
}

func TestTransientLoadCurrentHarnessUsesPhysicalLoadBehindHighSideDisconnect(t *testing.T) {
	minimum, maximum := 0.02, 1.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{{ID: "supply", Kind: "supply", NominalVoltageV: 5}, {ID: "ground", Kind: "reference"}},
		Ports:   []architecturesearch.Port{{ID: "load", Domain: "supply"}, {ID: "ground", Domain: "ground"}},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{
			Axis: "load_current", Target: "load", Min: &minimum, Max: &maximum, Unit: "A",
		}}}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{{Kind: "domain", ID: "ground", Target: "GND"}, {Kind: "port", ID: "load", Target: "OUTPUT"}}
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{Component: "disconnect", PrimitiveModel: simmodel.PrimitivePMOSSwitchV1, Terminals: []simmodel.TerminalBinding{{Terminal: "SOURCE", Net: "RAIL"}, {Terminal: "DRAIN", Net: "PROTECTED"}, {Terminal: "GATE", Net: "GATE"}}},
		{Component: "fuse", PrimitiveModel: simmodel.PrimitiveFuseClosedStateV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "PROTECTED"}, {Terminal: "B", Net: "OUTPUT"}}},
	}}
	devices, err := operatingHarnessDevices(requirement, bindings, &plan, simmodel.AnalysisTransient)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Source || devices[0].Device.CatalogID != "resistor.generic.0603" ||
		!devices[0].Device.HasValueSI || math.Abs(devices[0].Device.ValueSI-5) > 1e-12 {
		t.Fatalf("protected transient load harness = %#v", devices)
	}
}

func TestStartupLoadCurrentHarnessUsesPoweredTargetWithoutLoadSwitch(t *testing.T) {
	minimum, maximum := 0.0, 0.03
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains:        []architecturesearch.Domain{{ID: "regulated", Kind: "supply", NominalVoltageV: 3.3}, {ID: "ground", Kind: "reference"}},
		Ports:          []architecturesearch.Port{{ID: "sensor_power", Domain: "regulated"}, {ID: "ground", Domain: "ground"}},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{Axis: "load_current", Target: "sensor_power", Min: &minimum, Max: &maximum, Unit: "A"}}}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{{Kind: "domain", ID: "ground", Target: "GND"}, {Kind: "port", ID: "sensor_power", Target: "VCC_3V3"}}
	devices, err := operatingHarnessDevices(requirement, bindings, nil, simmodel.AnalysisStartup)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Source || !devices[0].Device.HasValueSI || math.Abs(devices[0].Device.ValueSI-110) > 1e-12 {
		t.Fatalf("direct powered-port startup load = %#v", devices)
	}
}

func TestStartupLoadCurrentHarnessUsesTargetDomainWithoutLoadSwitch(t *testing.T) {
	minimum, maximum := 0.001, 0.15
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains:        []architecturesearch.Domain{{ID: "sensor_3v3", Kind: "supply", NominalVoltageV: 3.3}, {ID: "ground", Kind: "reference"}},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{Axis: "load_current", Target: "sensor_3v3", Min: &minimum, Max: &maximum, Unit: "A"}}}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{{Kind: "domain", ID: "ground", Target: "GND"}, {Kind: "domain", ID: "sensor_3v3", Target: "VCC_3V3"}}
	devices, err := operatingHarnessDevices(requirement, bindings, nil, simmodel.AnalysisStartup)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Source || !devices[0].Device.HasValueSI || math.Abs(devices[0].Device.ValueSI-22) > 1e-12 {
		t.Fatalf("direct domain startup load = %#v", devices)
	}
}

func TestOperatingHarnessSelectionUsesAnalysisSpecificLoads(t *testing.T) {
	ordinary := []operatingHarnessDevice{{Device: circuitgraph.SimulationHarnessDevice{InstanceID: "ordinary"}}}
	dynamic := []operatingHarnessDevice{{Device: circuitgraph.SimulationHarnessDevice{InstanceID: "dynamic"}}}
	dc := []operatingHarnessDevice{{Device: circuitgraph.SimulationHarnessDevice{InstanceID: "dc"}}}
	startup := []operatingHarnessDevice{{Device: circuitgraph.SimulationHarnessDevice{InstanceID: "startup"}}}
	for kind, expected := range map[string]string{
		simmodel.AnalysisTransient:        "dynamic",
		simmodel.AnalysisElectrothermal:   "dynamic",
		simmodel.AnalysisDCOperatingPoint: "dc",
		simmodel.AnalysisStartup:          "startup",
	} {
		selected := operatingHarnessForAnalysis(kind, ordinary, dynamic, dc, startup)
		if len(selected) != 1 || selected[0].Device.InstanceID != expected {
			t.Fatalf("%s harness = %#v, want %s", kind, selected, expected)
		}
	}
}

func TestDynamicLoadCurrentHarnessUsesAnalysisAppropriateLoad(t *testing.T) {
	minimum, maximum := 0.02, 1.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{
			{ID: "regulated", Kind: "supply", NominalVoltageV: 5},
			{ID: "ground", Kind: "reference"},
		},
		Ports: []architecturesearch.Port{{ID: "output", Domain: "regulated"}, {ID: "ground", Domain: "ground"}},
		OperatingCases: []architecturesearch.OperatingCase{{
			Conditions: []architecturesearch.OperatingCondition{{
				Axis: "load_current", Target: "output", Min: &minimum, Max: &maximum, Unit: "A",
			}},
		}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{
		{Kind: "domain", ID: "ground", Target: "GND"},
		{Kind: "port", ID: "output", Target: "VOUT"},
	}
	transient, err := operatingHarnessDevices(requirement, bindings, nil, simmodel.AnalysisTransient)
	if err != nil {
		t.Fatal(err)
	}
	if len(transient) != 1 || !transient[0].Source || transient[0].Device.CatalogID != "source.current.connector.1x02" {
		t.Fatalf("transient load harness = %#v", transient)
	}
	electrothermal, err := operatingHarnessDevices(requirement, bindings, nil, simmodel.AnalysisElectrothermal)
	if err != nil {
		t.Fatal(err)
	}
	if len(electrothermal) != 1 || electrothermal[0].Source || !electrothermal[0].Device.HasValueSI || math.Abs(electrothermal[0].Device.ValueSI-5) > 1e-12 {
		t.Fatalf("electrothermal load harness = %#v", electrothermal)
	}
}

func TestVoltageEventHarnessCreatesOnlyMissingDynamicVoltageSource(t *testing.T) {
	initial := 3.3
	applied := 0.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		OperatingCases: []architecturesearch.OperatingCase{{
			Events: []architecturesearch.OperatingEvent{{
				ID: "rail_loss", Kind: "rail_loss",
				Target:  architecturesearch.Observation{Kind: "port", ID: "output_3v3"},
				Initial: &initial, Applied: &applied, Unit: "V",
			}},
		}},
	}}
	targets := map[string]string{"port\x00output_3v3": "VCC_3V3"}

	devices, err := voltageEventHarnessDevices(requirement, targets, "GND", nil, simmodel.AnalysisTransient)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || !devices[0].Source || !devices[0].HasDefaultValue || devices[0].DefaultValue != initial {
		t.Fatalf("voltage-event harness = %#v", devices)
	}
	intent := simmodel.Intent{Analyses: []simmodel.Analysis{{
		Kind: simmodel.AnalysisTransient, DurationS: 1e-3, TimeStepS: 10e-6,
	}}}
	addOperatingHarnessExcitations(&intent, devices)
	excitation := intent.Analyses[0].Excitations[0]
	if excitation.DCValue != initial || excitation.PulsePeriodS != 0 {
		t.Fatalf("voltage-event harness initial boundary = %#v", excitation)
	}
	device := devices[0].Device
	if device.CatalogID != "source.voltage.connector.1x02" ||
		len(device.Connections) != 2 ||
		device.Connections[0] != (simmodel.ConnectionEvidence{Function: "POSITIVE", Net: "VCC_3V3"}) ||
		device.Connections[1] != (simmodel.ConnectionEvidence{Function: "NEGATIVE", Net: "GND"}) {
		t.Fatalf("voltage-event source = %#v", device)
	}

	devices, err = voltageEventHarnessDevices(requirement, targets, "GND", nil, simmodel.AnalysisDCOperatingPoint)
	if err != nil || len(devices) != 0 {
		t.Fatalf("DC voltage-event harness = %#v, err=%v", devices, err)
	}

	resolved := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{
		Component: "existing_source", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1,
		Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "VCC_3V3"}, {Terminal: "NEGATIVE", Net: "GND"}},
	}}}
	devices, err = voltageEventHarnessDevices(requirement, targets, "GND", &resolved, simmodel.AnalysisTransient)
	if err != nil || len(devices) != 0 {
		t.Fatalf("duplicate voltage-event harness = %#v, err=%v", devices, err)
	}
}

func TestShortCircuitEventHarnessUsesCatalogBackedShunt(t *testing.T) {
	initial, applied, recovered := 5.0, 0.0, 5.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		OperatingCases: []architecturesearch.OperatingCase{{
			Events: []architecturesearch.OperatingEvent{{
				ID: "output_short", Kind: "short_circuit",
				Target:    architecturesearch.Observation{Kind: "port", ID: "output"},
				Initial:   &initial,
				Applied:   &applied,
				Recovered: &recovered,
				Unit:      "Ohm",
			}},
		}},
	}}
	targets := map[string]string{"port\x00output": "VOUT"}

	devices, err := voltageEventHarnessDevices(requirement, targets, "GND", nil, simmodel.AnalysisElectrothermal)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Source || !devices[0].Device.HasValueSI ||
		devices[0].Device.ValueSI != closedloopsynthesis.ShortCircuitHarnessOpenResistanceOhm {
		t.Fatalf("short-circuit harness = %#v", devices)
	}
	device := devices[0].Device
	if device.InstanceID != closedloopsynthesis.OperatingHarnessComponentID("short_circuit", "VOUT") ||
		device.CatalogID != "resistor.generic.0603" ||
		len(device.Connections) != 2 ||
		device.Connections[0] != (simmodel.ConnectionEvidence{Function: "A", Net: "VOUT"}) ||
		device.Connections[1] != (simmodel.ConnectionEvidence{Function: "B", Net: "GND"}) {
		t.Fatalf("short-circuit shunt = %#v", device)
	}
}

func TestResistanceEventHarnessCreatesOnlyMissingDynamicLoad(t *testing.T) {
	initial, otherInitial, applied, recovered := 10.0, 20.0, 0.01, 10.0
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{
			{ID: "regulated", Kind: "supply", NominalVoltageV: 5},
			{ID: "ground", Kind: "reference"},
		},
		Ports: []architecturesearch.Port{{ID: "output", Domain: "regulated"}, {ID: "ground", Domain: "ground"}},
		OperatingCases: []architecturesearch.OperatingCase{{
			Events: []architecturesearch.OperatingEvent{{
				ID: "short", Kind: "short_circuit",
				Target:  architecturesearch.Observation{Kind: "port", ID: "output"},
				Initial: &initial, Applied: &applied, Recovered: &recovered, Unit: "Ohm",
			}},
		}, {
			Events: []architecturesearch.OperatingEvent{{
				ID: "alternate_short", Kind: "short_circuit",
				Target:  architecturesearch.Observation{Kind: "port", ID: "output"},
				Initial: &otherInitial, Applied: &applied, Recovered: &recovered, Unit: "Ohm",
			}},
		}},
	}}
	targets := map[string]string{
		"port\x00output":   "VOUT",
		"domain\x00ground": "GND",
	}
	devices, err := resistanceEventHarnessDevices(requirement, targets, nil, simmodel.AnalysisElectrothermal)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Source || !devices[0].Device.HasValueSI || devices[0].Device.ValueSI != otherInitial {
		t.Fatalf("resistance-event harness = %#v", devices)
	}
	device := devices[0].Device
	if device.CatalogID != "resistor.generic.0603" ||
		len(device.Connections) != 2 ||
		device.Connections[0] != (simmodel.ConnectionEvidence{Function: "A", Net: "VOUT"}) ||
		device.Connections[1] != (simmodel.ConnectionEvidence{Function: "B", Net: "GND"}) {
		t.Fatalf("resistance-event load = %#v", device)
	}

	resolved := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{
		Component: closedloopsynthesis.OperatingHarnessComponentID("load_resistance", "VOUT"), Family: "resistor", PrimitiveModel: simmodel.PrimitiveResistorV1,
		Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "VOUT"}, {Terminal: "B", Net: "GND"}},
	}}}
	devices, err = resistanceEventHarnessDevices(requirement, targets, &resolved, simmodel.AnalysisTransient)
	if err != nil || len(devices) != 0 {
		t.Fatalf("duplicate resistance-event harness = %#v, err=%v", devices, err)
	}

	maximumCurrent := 1.0
	requirement.Requirements.OperatingCases[0].Conditions = []architecturesearch.OperatingCondition{{
		Axis: "load_current", Target: "output", Max: &maximumCurrent, Unit: "A",
	}}
	devices, err = resistanceEventHarnessDevices(requirement, targets, nil, simmodel.AnalysisTransient)
	if err != nil || len(devices) != 0 {
		t.Fatalf("resistance event duplicated the physical current-load harness: %#v, err=%v", devices, err)
	}
}

func TestSequencedDualRailDynamicPlansUseControlledDisconnectAndVoltageEventSource(t *testing.T) {
	data, err := os.ReadFile("../architecturesearch/testdata/dynamic_electrothermal_control_loop_corpus/sequenced_dual_rail_controller.json")
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
	graphResolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"})
	search := architecturesearch.Search(context.Background(), requirement, registry, architecturesearch.SearchOptions{CatalogHash: graphResolver.CatalogHash()})
	if search.Status != architecturesearch.SearchSelected || search.Selected == nil {
		t.Fatalf("search status = %s issues=%#v rejections=%#v coverage=%#v", search.Status, search.Issues, search.Rejections, search.Coverage)
	}
	provenance, provenanceDiagnostics := modelprovenance.LoadDefault()
	if len(provenanceDiagnostics) != 0 {
		t.Fatalf("model provenance diagnostics = %#v", provenanceDiagnostics)
	}
	resolver := ArchitectureSimulationPlanResolver{
		Requirement: requirement, Search: search, GraphResolver: graphResolver, ProvenanceRegistry: provenance,
	}
	state := closedloopsynthesis.CandidateState{Fingerprint: search.Selected.Fingerprint}
	planSet, err := resolver.ResolveSimulationPlanSet(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	transientPlan := planSet.Plans[simmodel.AnalysisTransient]
	disconnects := 0
	for _, device := range transientPlan.Devices {
		if strings.Contains(device.Component, "output_disconnect") {
			disconnects++
			if device.PrimitiveModel != simmodel.PrimitivePMOSSwitchV1 {
				t.Fatalf("transient disconnect %s model = %s", device.Component, device.PrimitiveModel)
			}
		}
	}
	if disconnects != 2 {
		t.Fatalf("transient disconnect count = %d", disconnects)
	}

	resolution, diagnostics := closedloopsynthesis.CompileSimulationResolution(
		planSet.AnalysisPlan, planSet.Plans, planSet.Templates, planSet.Assertions, planSet.OperatingBindings,
	)
	if len(diagnostics) != 0 {
		t.Fatalf("compile diagnostics = %#v", diagnostics)
	}
	generatedControl := ""
	for _, binding := range planSet.OperatingBindings {
		if binding.Kind != closedloopsynthesis.OperatingGeneratedControl {
			continue
		}
		if generatedControl != "" && generatedControl != binding.Component {
			t.Fatalf("generated-domain controls are ambiguous: %q and %q", generatedControl, binding.Component)
		}
		generatedControl = binding.Component
	}
	if generatedControl == "" {
		t.Fatal("sequenced rail plan lacks a generated-domain control binding")
	}
	startupControlEvents := 0
	for _, plan := range resolution.Plans {
		for _, analysis := range plan.Analyses {
			for _, event := range analysis.SourceValueEvents {
				if event.Component != generatedControl || !strings.HasPrefix(event.ID, "startup_generated_control_") {
					continue
				}
				startupControlEvents++
				if event.Initial != 0 || event.Applied == 0 {
					t.Fatalf("compiled generated-domain startup control event = %#v", event)
				}
			}
		}
	}
	if startupControlEvents == 0 {
		t.Fatal("compiled sequenced rail plans lack a generated-domain startup control transition")
	}
	railLossSources := map[string]bool{}
	for _, plan := range resolution.Plans {
		for _, analysis := range plan.Analyses {
			for _, event := range analysis.SourceValueEvents {
				if strings.HasPrefix(event.ID, "rail_loss_") {
					railLossSources[event.Component] = true
				}
			}
		}
	}
	if len(railLossSources) != 1 {
		t.Fatalf("rail-loss event sources = %#v", railLossSources)
	}
	for component := range railLossSources {
		foundVoltageSource := false
		for _, plan := range resolution.Plans {
			for _, device := range plan.Devices {
				if device.Component == component && device.PrimitiveModel == simmodel.PrimitiveVoltageSourceV1 {
					foundVoltageSource = true
				}
			}
		}
		if !foundVoltageSource || strings.Contains(component, "load_current") {
			t.Fatalf("rail-loss event source %q is not its dedicated voltage source", component)
		}
	}
}

func TestClassABDynamicPlansResolveMuteRelayPerAnalysis(t *testing.T) {
	planSet := classABDynamicPlanSetForTest(t)
	plans := planSet.Plans
	for _, test := range []struct {
		kind string
		want string
	}{
		{simmodel.AnalysisTransient, simmodel.PrimitiveRelayNormallyOpenV1},
		{simmodel.AnalysisDistortion, simmodel.PrimitiveRelayNormallyOpenV1},
		{simmodel.AnalysisElectrothermal, simmodel.PrimitiveRelayNormallyOpenV1},
		{simmodel.AnalysisStability, simmodel.PrimitiveRelayClosedV1},
	} {
		plan, ok := plans[test.kind]
		if !ok {
			t.Fatalf("%s plan is missing", test.kind)
		}
		var relays []simmodel.ResolvedDevice
		for _, device := range plan.Devices {
			if device.Family == "relay" {
				relays = append(relays, device)
			}
		}
		if len(relays) != 1 {
			t.Fatalf("%s relay devices = %#v", test.kind, relays)
		}
		if relays[0].PrimitiveModel != test.want {
			t.Fatalf("%s relay model = %s, want %s", test.kind, relays[0].PrimitiveModel, test.want)
		}
	}
}

func classABDynamicPlanSetForTest(t *testing.T) closedloopsynthesis.FreshSimulationPlanSet {
	t.Helper()
	data, err := os.ReadFile("../architecturesearch/testdata/dynamic_electrothermal_control_loop_corpus/class_ab_dynamic_output_stage.json")
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
	graphResolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"})
	search := architecturesearch.Search(context.Background(), requirement, registry, architecturesearch.SearchOptions{CatalogHash: graphResolver.CatalogHash()})
	if search.Status != architecturesearch.SearchSelected || search.Selected == nil {
		t.Fatalf("search status = %s issues=%#v rejections=%#v coverage=%#v", search.Status, search.Issues, search.Rejections, search.Coverage)
	}
	provenance, provenanceDiagnostics := modelprovenance.LoadDefault()
	if len(provenanceDiagnostics) != 0 {
		t.Fatalf("model provenance diagnostics = %#v", provenanceDiagnostics)
	}
	resolver := ArchitectureSimulationPlanResolver{
		Requirement: requirement, Search: search, GraphResolver: graphResolver, ProvenanceRegistry: provenance,
	}
	planSet, err := resolver.ResolveSimulationPlanSet(context.Background(), closedloopsynthesis.CandidateState{Fingerprint: search.Selected.Fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	return planSet
}

func TestClassABDynamicPerformancePlansPass(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping exhaustive Class-AB dynamic performance sweep in short mode")
	}
	planSet := classABDynamicPlanSetForTest(t)
	resolution, diagnostics := closedloopsynthesis.CompileSimulationResolution(
		planSet.AnalysisPlan,
		planSet.Plans,
		planSet.Templates,
		planSet.Assertions,
		planSet.OperatingBindings,
	)
	if len(diagnostics) != 0 {
		t.Fatalf("compile diagnostics = %#v", diagnostics)
	}
	targets := []string{
		simmodel.QuantityOutputPowerW,
		simmodel.QuantityTHDPercent,
		simmodel.QuantityPhaseMarginDeg,
		simmodel.QuantityGainMarginDB,
	}
	for _, quantity := range targets {
		t.Run(quantity, func(t *testing.T) {
			seen := false
			for _, source := range resolution.Plans {
				var assertions []simmodel.Assertion
				for _, assertion := range source.Assertions {
					if assertion.Quantity == quantity {
						assertions = append(assertions, assertion)
						seen = true
					}
				}
				if len(assertions) == 0 {
					continue
				}
				plan := simmodel.ClonePlan(source)
				plan.Assertions = assertions
				report, evaluationDiagnostics := simmodel.Evaluate(plan)
				if len(evaluationDiagnostics) != 0 || report.Status != "pass" {
					var loops []simmodel.ControlLoop
					for _, analysis := range report.Analyses {
						loops = append(loops, analysis.ControlLoops...)
					}
					detail := ""
					if quantity == simmodel.QuantityTHDPercent {
						minimum, maximum := math.Inf(1), math.Inf(-1)
						nodeRanges := map[string][2]float64{}
						nodeTHD := map[string]float64{}
						nodeMean := map[string]float64{}
						for _, analysis := range report.Analyses {
							if analysis.ID != assertions[0].AnalysisID {
								continue
							}
							nodeTHD, nodeMean = distortionNodeTHDForTest(analysis)
							for _, point := range analysis.Points {
								for _, node := range point.Nodes {
									bounds, exists := nodeRanges[node.Node]
									if !exists {
										bounds = [2]float64{math.Inf(1), math.Inf(-1)}
									}
									nodeRanges[node.Node] = [2]float64{math.Min(bounds[0], node.Real), math.Max(bounds[1], node.Real)}
									if node.Node == assertions[0].Node {
										minimum, maximum = math.Min(minimum, node.Real), math.Max(maximum, node.Real)
									}
								}
							}
						}
						var stageDevices []simmodel.ResolvedDevice
						for _, device := range plan.Devices {
							if strings.Contains(device.Component, "objective_drive") {
								stageDevices = append(stageDevices, device)
							}
						}
						for _, analysis := range plan.Analyses {
							if analysis.ID == assertions[0].AnalysisID {
								detail = fmt.Sprintf(" output_range=%g..%g node_ranges=%#v node_thd=%v node_mean=%v excitations=%#v stage_devices=%#v", minimum, maximum, nodeRanges, nodeTHD, nodeMean, analysis.Excitations, stageDevices)
							}
						}
					}
					t.Errorf("analysis %s failed:%s assertions=%#v loops=%#v diagnostics=%#v", assertions[0].AnalysisID, detail, report.Assertions, loops, evaluationDiagnostics)
				}
			}
			if !seen {
				t.Errorf("compiled Class-AB plans omitted %s", quantity)
			}
		})
	}
}

func distortionNodeTHDForTest(analysis simmodel.AnalysisResult) (map[string]float64, map[string]float64) {
	if analysis.FundamentalFrequencyHz <= 0 || len(analysis.Points) < 2 || analysis.Points[1].TimeS <= 0 {
		return nil, nil
	}
	samplesPerCycle := int(math.Round(1 / (analysis.FundamentalFrequencyHz * analysis.Points[1].TimeS)))
	window := 2 * samplesPerCycle
	if len(analysis.Points)-1 < window {
		return nil, nil
	}
	start := len(analysis.Points) - 1 - window
	values := map[string][]float64{}
	for _, point := range analysis.Points[start : start+window] {
		for _, node := range point.Nodes {
			values[node.Node] = append(values[node.Node], node.Real)
		}
	}
	magnitude := func(samples []float64, bin int) float64 {
		realPart, imaginary := 0.0, 0.0
		for index, value := range samples {
			angle := 2 * math.Pi * float64(bin*index) / float64(len(samples))
			realPart += value * math.Cos(angle)
			imaginary -= value * math.Sin(angle)
		}
		return 2 * math.Hypot(realPart, imaginary) / float64(len(samples))
	}
	result := map[string]float64{}
	means := map[string]float64{}
	for node, samples := range values {
		fundamental := magnitude(samples, 2)
		if fundamental <= 1e-15 {
			continue
		}
		harmonicPower := 0.0
		for harmonic := 2; harmonic <= 5; harmonic++ {
			harmonicMagnitude := magnitude(samples, 2*harmonic)
			harmonicPower += harmonicMagnitude * harmonicMagnitude
		}
		mean := 0.0
		for _, sample := range samples {
			mean += sample
		}
		mean /= float64(len(samples))
		result[node] = 100 * math.Sqrt(harmonicPower) / fundamental
		means[node] = mean
	}
	return result, means
}

func TestTransientCurrentHarnessUsesDeclaredSteadyBoundary(t *testing.T) {
	intent := simmodel.Intent{Analyses: []simmodel.Analysis{{Kind: simmodel.AnalysisTransient, DurationS: 1e-3, TimeStepS: 10e-6}}}
	harness := []operatingHarnessDevice{{
		Device:          circuitgraph.SimulationHarnessDevice{InstanceID: "load"},
		Source:          true,
		DefaultValue:    2,
		HasDefaultValue: true,
	}}
	addOperatingHarnessExcitations(&intent, harness)
	if len(intent.Analyses[0].Excitations) != 1 {
		t.Fatalf("transient harness excitations = %#v", intent.Analyses[0].Excitations)
	}
	excitation := intent.Analyses[0].Excitations[0]
	if excitation.DCValue != 2 || excitation.PulsePeriodS != 0 {
		t.Fatalf("transient steady load boundary = %#v", excitation)
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
