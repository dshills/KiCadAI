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
	if analysis.DurationS != .004 || analysis.TimeStepS != .00003125 {
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
	wantAmplitude := math.Sqrt(2*minimumPower*loadResistance) * 1.02 / minimumGain
	if intent.ModelID != simmodel.ModelTransientCircuitV1 || analysis.Kind != simmodel.AnalysisThermal || analysis.DurationS != .004 || analysis.TimeStepS != .00003125 {
		t.Fatalf("derived thermal workflow = model %s analysis %#v", intent.ModelID, analysis)
	}
	if analysis.Excitations[0].SineAmplitude != wantAmplitude || analysis.Excitations[0].SineFrequencyHz != 1000 || analysis.Excitations[1].SineFrequencyHz != 0 {
		t.Fatalf("derived thermal excitations = %#v", analysis.Excitations)
	}
	if len(analysis.Conditions) != 1 || analysis.Conditions[0].Name != "ambient_temperature_c" || analysis.Conditions[0].Value != 25 {
		t.Fatalf("derived thermal conditions = %#v", analysis.Conditions)
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
	for _, fixture := range []string{
		"active_filter_amplifier",
		"class_a_amplifier",
		"class_ab_amplifier",
		"current_sense_protection",
		"hysteretic_mosfet_load",
		"low_noise_sensor_decision",
		"mixed_function_control_power",
		"protected_mixed_signal_interface",
		"regulated_sensor_interface",
		"split_supply_frontend",
	} {
		t.Run(fixture, func(t *testing.T) {
			data, readErr := os.ReadFile("../architecturesearch/testdata/simulation_grounded_closed_loop_corpus/" + fixture + ".json")
			if readErr != nil {
				t.Fatal(readErr)
			}
			requirement, decodeIssues := architecturesearch.DecodeStrict(bytes.NewReader(data))
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
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains:        []architecturesearch.Domain{{ID: "supply", Kind: "supply"}, {ID: "ground", Kind: "reference"}},
		Ports:          []architecturesearch.Port{{ID: "power", Domain: "supply"}, {ID: "load", Domain: "supply"}, {ID: "ground", Domain: "ground"}},
		Objectives:     []architecturesearch.Objective{{Capability: "load_switch", Bindings: []architecturesearch.Binding{{Role: "power", Port: "power"}, {Role: "output", Port: "load"}, {Role: "reference", Port: "ground"}}}},
		OperatingCases: []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{Axis: "load_current", Target: "load", Min: &minimum, Max: &maximum, Unit: "A"}}}},
	}}
	bindings := []closedloopsynthesis.SemanticBinding{
		{Kind: "domain", ID: "ground", Target: "GND"},
		{Kind: "port", ID: "power", Target: "SUPPLY"},
		{Kind: "port", ID: "load", Target: "SWITCHED"},
	}
	devices, err := operatingHarnessDevices(requirement, bindings, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || !devices[0].Source || len(devices[0].Device.Connections) != 2 {
		t.Fatalf("load-current harness = %#v", devices)
	}
	connections := devices[0].Device.Connections
	if connections[0].Function != "POSITIVE" || connections[0].Net != "SUPPLY" || connections[1].Function != "NEGATIVE" || connections[1].Net != "SWITCHED" {
		t.Fatalf("load-current connections = %#v", connections)
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
	dc := []operatingHarnessDevice{{Device: circuitgraph.SimulationHarnessDevice{InstanceID: "dc"}}}
	startup := []operatingHarnessDevice{{Device: circuitgraph.SimulationHarnessDevice{InstanceID: "startup"}}}
	for kind, expected := range map[string]string{
		simmodel.AnalysisTransient:        "ordinary",
		simmodel.AnalysisDCOperatingPoint: "dc",
		simmodel.AnalysisStartup:          "startup",
	} {
		selected := operatingHarnessForAnalysis(kind, ordinary, dc, startup)
		if len(selected) != 1 || selected[0].Device.InstanceID != expected {
			t.Fatalf("%s harness = %#v, want %s", kind, selected, expected)
		}
	}
}

func TestTransientCurrentHarnessStepsFromQuiescentInitialCondition(t *testing.T) {
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
	if excitation.DCValue != 0 || excitation.PulseInitialValue != 0 || excitation.PulseValue != 2 || excitation.PulseDelayS != 20e-6 || excitation.PulseWidthS != 980e-6 || excitation.PulsePeriodS != 1.01e-3 {
		t.Fatalf("transient load step = %#v", excitation)
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
