package closedloopsynthesis

import (
	"context"
	"fmt"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

func TestLoadCurrentCornerFollowsPulsedSupplyWindow(t *testing.T) {
	analysis := simmodel.Analysis{Kind: simmodel.AnalysisTransient, Excitations: []simmodel.SourceExcitation{
		{Component: "supply", PulseInitialValue: 0, PulseValue: 5, PulseDelayS: 1e-6, PulseWidthS: 8e-6, PulsePeriodS: 10e-6},
		{Component: "load", DCValue: 0},
	}}
	value := 0.15
	diagnostic := applyOperatingAssignment(&analysis, &simmodel.Plan{}, SimulationOperatingBinding{Axis: "load_current", Kind: OperatingSourceDCValue, Component: "load"}, CornerAssignment{Value: &value})
	if diagnostic != nil {
		t.Fatal(diagnostic)
	}
	load := analysis.Excitations[1]
	if load.DCValue != 0 || load.PulseInitialValue != 0 || load.PulseValue != value || load.PulseDelayS != 1e-6 || load.PulseWidthS != 8e-6 || load.PulsePeriodS != 10e-6 {
		t.Fatalf("dynamic load excitation = %#v", load)
	}
}

func TestLoadCurrentCornerUsesEquivalentStartupResistance(t *testing.T) {
	baseResistance := 4.0
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{Component: "load", Family: "resistor", ValueSI: &baseResistance}}}
	for current, expected := range map[float64]float64{3: 4, 1.5: 8, 0: maxCompiledAssertionBound} {
		analysis := simmodel.Analysis{Kind: simmodel.AnalysisStartup}
		value := current
		diagnostic := applyOperatingAssignment(&analysis, &plan, SimulationOperatingBinding{Axis: "load_current", Kind: OperatingLoadCurrent, Component: "load", Scale: 12}, CornerAssignment{Value: &value})
		if diagnostic != nil {
			t.Fatalf("current %.12g: %#v", current, diagnostic)
		}
		if len(analysis.DeviceOverrides) != 1 || analysis.DeviceOverrides[0].ValueSI == nil || *analysis.DeviceOverrides[0].ValueSI != expected {
			t.Fatalf("current %.12g override = %#v, want %.12g ohm", current, analysis.DeviceOverrides, expected)
		}
	}
}

func TestOperatingDeviceValueAllowsZeroOnlyForCapacitors(t *testing.T) {
	zero := 0.0
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{Component: "capacitive_load", Family: "capacitor"}, {Component: "resistive_load", Family: "resistor"}}}
	analysis := simmodel.Analysis{}
	if diagnostic := applyOperatingAssignment(&analysis, &plan, SimulationOperatingBinding{Axis: "load_capacitance", Kind: OperatingDeviceValueSI, Component: "capacitive_load"}, CornerAssignment{Value: &zero}); diagnostic != nil {
		t.Fatalf("zero-capacitance corner = %#v", diagnostic)
	}
	if len(analysis.DeviceOverrides) != 1 || analysis.DeviceOverrides[0].ValueSI == nil || *analysis.DeviceOverrides[0].ValueSI != 0 {
		t.Fatalf("zero-capacitance override = %#v", analysis.DeviceOverrides)
	}
	if diagnostic := applyOperatingAssignment(&analysis, &plan, SimulationOperatingBinding{Axis: "load_resistance", Kind: OperatingDeviceValueSI, Component: "resistive_load"}, CornerAssignment{Value: &zero}); diagnostic == nil {
		t.Fatal("zero-resistance corner was accepted")
	}
}

func TestOperatingModelParameterMergesExistingDeviceOverride(t *testing.T) {
	valueSI := 1_000.0
	analysis := simmodel.Analysis{DeviceOverrides: []simmodel.DeviceOverride{{
		Component: "gain_device",
		ValueSI:   &valueSI,
		ModelParameters: []simmodel.NamedValue{
			{Name: "forward_beta", Value: 80},
		},
	}}}
	earlyVoltage := 120.0
	diagnostic := applyOperatingAssignment(&analysis, &simmodel.Plan{}, SimulationOperatingBinding{
		Axis: "model_corner", Kind: OperatingModelParameter, Component: "gain_device", Parameter: "early_voltage",
	}, CornerAssignment{Value: &earlyVoltage})
	if diagnostic != nil {
		t.Fatal(diagnostic)
	}
	if len(analysis.DeviceOverrides) != 1 {
		t.Fatalf("device overrides = %#v, want one merged component override", analysis.DeviceOverrides)
	}
	override := analysis.DeviceOverrides[0]
	if override.ValueSI == nil || *override.ValueSI != valueSI {
		t.Fatalf("merged override lost device value: %#v", override)
	}
	if len(override.ModelParameters) != 2 || override.ModelParameters[0] != (simmodel.NamedValue{Name: "early_voltage", Value: 120}) || override.ModelParameters[1] != (simmodel.NamedValue{Name: "forward_beta", Value: 80}) {
		t.Fatalf("merged model parameters = %#v", override.ModelParameters)
	}
}

func TestAmbientTemperatureCornerOverridesTemperatureSensitiveDevices(t *testing.T) {
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{Component: "output_npn", ModelParameters: []simmodel.NamedValue{{Name: "junction_temperature_k", Value: 298.15}}},
		{Component: "output_pnp", ModelParameters: []simmodel.NamedValue{{Name: "junction_temperature_k", Value: 298.15}}},
		{Component: "load", ModelParameters: []simmodel.NamedValue{{Name: "resistance_temperature_coefficient", Value: 1e-6}}},
	}}
	analysis := simmodel.Analysis{DeviceOverrides: []simmodel.DeviceOverride{{Component: "output_npn", ModelParameters: []simmodel.NamedValue{{Name: "forward_beta", Value: 80}}}}}
	ambientC := 50.0
	diagnostic := applyOperatingAssignment(&analysis, &plan, SimulationOperatingBinding{Axis: "ambient_temperature", Kind: OperatingAnalysisCondition, Parameter: "ambient_temperature_c"}, CornerAssignment{Value: &ambientC})
	if diagnostic != nil {
		t.Fatal(diagnostic)
	}
	if len(analysis.Conditions) != 1 || analysis.Conditions[0].Name != "ambient_temperature_c" || analysis.Conditions[0].Value != 50 {
		t.Fatalf("ambient condition = %#v", analysis.Conditions)
	}
	if len(analysis.DeviceOverrides) != 2 {
		t.Fatalf("temperature overrides = %#v", analysis.DeviceOverrides)
	}
	for _, override := range analysis.DeviceOverrides {
		if override.Component == "load" {
			t.Fatalf("temperature-insensitive device received override: %#v", override)
		}
		foundTemperature := false
		for _, parameter := range override.ModelParameters {
			if parameter.Name == "junction_temperature_k" {
				foundTemperature = true
				if parameter.Value != 323.15 {
					t.Fatalf("%s junction temperature = %.12g K", override.Component, parameter.Value)
				}
			}
		}
		if !foundTemperature {
			t.Fatalf("%s missing junction-temperature override: %#v", override.Component, override)
		}
	}
}

func TestAmbientTemperatureCornerRejectsAbsoluteZero(t *testing.T) {
	analysis := simmodel.Analysis{}
	ambientC := -273.15
	if diagnostic := applyOperatingAssignment(&analysis, &simmodel.Plan{}, SimulationOperatingBinding{Axis: "ambient_temperature", Kind: OperatingAnalysisCondition, Parameter: "ambient_temperature_c"}, CornerAssignment{Value: &ambientC}); diagnostic == nil {
		t.Fatal("absolute-zero ambient corner was accepted")
	}
}

func TestEdgeTimeAssertionsRequireDynamicExcitation(t *testing.T) {
	static := simmodel.Analysis{Excitations: []simmodel.SourceExcitation{{Component: "load", PulseInitialValue: 0, PulseValue: 0, PulseWidthS: 1e-3, PulsePeriodS: 2e-3}}}
	dynamicPulse := static
	dynamicPulse.Excitations = append([]simmodel.SourceExcitation(nil), static.Excitations...)
	dynamicPulse.Excitations[0].PulseValue = 3
	dynamicSine := simmodel.Analysis{Excitations: []simmodel.SourceExcitation{{Component: "input", SineAmplitude: 1, SineFrequencyHz: 1000}}}
	if analysisHasDynamicExcitation(static) {
		t.Fatal("constant pulse endpoints were treated as a dynamic excitation")
	}
	if !analysisHasDynamicExcitation(dynamicPulse) || !analysisHasDynamicExcitation(dynamicSine) {
		t.Fatal("bounded changing excitation was not recognized")
	}
	for _, quantity := range []string{simmodel.QuantityRiseTimeS, simmodel.QuantityFallTimeS, simmodel.QuantitySettlingTimeS, simmodel.QuantityResponseTimeS} {
		if !edgeTimeQuantity(quantity) {
			t.Fatalf("%s is not recognized as an edge-time quantity", quantity)
		}
	}
	if edgeTimeQuantity(simmodel.QuantityOutputPowerW) {
		t.Fatal("non-edge measurement was classified as edge time")
	}
}

func TestDynamicAnalysesPartitionAtTrustedPlanWorkBound(t *testing.T) {
	analyses := make([]simmodel.Analysis, 16)
	for index := range analyses {
		analyses[index] = simmodel.Analysis{ID: fmt.Sprintf("startup_%02d", index), Kind: simmodel.AnalysisStartup, DurationS: 100e-6, TimeStepS: 100e-6 / 256}
	}
	if simmodel.FitsPlanDynamicWork(analyses) {
		t.Fatal("oversized dynamic analysis set unexpectedly fits one plan")
	}
	batches := partitionAnalysesByDynamicWork(analyses)
	if len(batches) != 2 {
		t.Fatalf("dynamic batches = %d, want 2", len(batches))
	}
	covered := 0
	for _, batch := range batches {
		if !simmodel.FitsPlanDynamicWork(batch) {
			t.Fatalf("partition exceeds trusted work bound: %d analyses", len(batch))
		}
		covered += len(batch)
	}
	if covered != len(analyses) {
		t.Fatalf("partition covered %d/%d analyses", covered, len(analyses))
	}
}

func TestCompileSimulationResolutionBindsDeviceCornersAndAggregateLinks(t *testing.T) {
	baseIntent := simmodel.Intent{
		ModelID:    simmodel.ModelLinearCircuitMNAV1,
		Analyses:   []simmodel.Analysis{{ID: "placeholder", Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{{Component: "source", DCValue: .001}}}},
		Assertions: []simmodel.Assertion{{AnalysisID: "placeholder", Node: "OUT", Quantity: simmodel.QuantityVoltageV, Min: 0, Max: 3}},
	}
	components := []simmodel.ComponentEvidence{
		{InstanceID: "load", CatalogID: "resistor.generic.0603", Family: "resistor", HasValueSI: true, ValueSI: 1000, ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveResistorV1}}, Connections: []simmodel.ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
		{InstanceID: "source", CatalogID: "source.current.generic", Family: "current_source", ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveCurrentSourceV1}}, Connections: []simmodel.ConnectionEvidence{{Function: "POSITIVE", Net: "GND"}, {Function: "NEGATIVE", Net: "OUT"}}},
	}
	base, baseDiagnostics := simmodel.ResolveWithTopology(baseIntent, "catalog", testHash("catalog"), components, []simmodel.NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}})
	if len(baseDiagnostics) != 0 {
		t.Fatalf("base diagnostics = %#v", baseDiagnostics)
	}
	minimum, maximum := 0.9, 2.1
	analysisPlan := AnalysisPlan{
		Schema: AnalysisPlanSchema, PlanHash: testHash("analysis-plan"),
		Analyses: []PlannedAnalysis{{ID: "dc_operating_point:load_range", Kind: simmodel.AnalysisDCOperatingPoint, OperatingCase: "load_range", Requirements: []string{"output"}}},
		Corners: []PlannedCorner{
			{ID: "load_range:low", OperatingCase: "load_range", Assignments: []CornerAssignment{{Axis: "load_resistance", Target: "OUT", Value: float64PointerForTest(1000), Unit: "ohm"}}},
			{ID: "load_range:high", OperatingCase: "load_range", Assignments: []CornerAssignment{{Axis: "load_resistance", Target: "OUT", Value: float64PointerForTest(2000), Unit: "ohm"}}},
		},
		Assertions: []PlannedAssertion{{RequirementID: "output", AnalysisID: "dc_operating_point:load_range", OperatingCase: "load_range", Metric: "dc_voltage", Target: "OUT", Min: &minimum, Max: &maximum, Unit: "V"}},
	}
	template := base.Analyses[0]
	resolution, diagnostics := CompileSimulationResolution(
		analysisPlan,
		map[string]simmodel.Plan{simmodel.AnalysisDCOperatingPoint: base},
		[]SimulationAnalysisTemplate{{Kind: simmodel.AnalysisDCOperatingPoint, Analysis: template}},
		[]SimulationAssertionBinding{{Metric: "dc_voltage", Target: "OUT", BoundsMode: AssertionBoundsDirect, Prototypes: []simmodel.Assertion{{Node: "OUT", Quantity: simmodel.QuantityVoltageV}}}},
		[]SimulationOperatingBinding{{Axis: "load_resistance", Target: "OUT", Kind: OperatingDeviceValueSI, Component: "load"}},
	)
	if len(diagnostics) != 0 {
		t.Fatalf("compile diagnostics = %#v", diagnostics)
	}
	if len(resolution.Plans) != 1 || len(resolution.Plans[0].Analyses) != 2 || len(resolution.Measurements) != 1 || len(resolution.Measurements[0].Assertions) != 2 {
		t.Fatalf("resolution = %#v", resolution)
	}
	report, evaluationDiagnostics := simmodel.Evaluate(resolution.Plans[0])
	if len(evaluationDiagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("compiled evaluation = %#v diagnostics=%#v", report, evaluationDiagnostics)
	}
	if report.Assertions[0].Actual != 2 || report.Assertions[1].Actual != 1 {
		t.Fatalf("compiled corner assertions = %#v", report.Assertions)
	}
}

func float64PointerForTest(value float64) *float64 { return &value }

type repairableFreshMNAResolver struct{}

func (repairableFreshMNAResolver) ResolveSimulationPlans(_ context.Context, state CandidateState) (map[string]simmodel.Plan, error) {
	load := 500.0
	for _, variable := range state.Variables {
		if variable.ID == "load_resistance" {
			load = variable.Value
		}
	}
	intent := simmodel.Intent{
		ModelID:    simmodel.ModelLinearCircuitMNAV1,
		Analyses:   []simmodel.Analysis{{ID: "placeholder", Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{{Component: "source", DCValue: 5}}}},
		Assertions: []simmodel.Assertion{{AnalysisID: "placeholder", Node: "OUT", Quantity: simmodel.QuantityVoltageV, Min: 0, Max: 6}},
	}
	components := []simmodel.ComponentEvidence{
		{InstanceID: "upper", CatalogID: "resistor.generic.0603", Family: "resistor", HasValueSI: true, ValueSI: 1000, ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveResistorV1}}, Connections: []simmodel.ConnectionEvidence{{Function: "A", Net: "VIN"}, {Function: "B", Net: "OUT"}}},
		{InstanceID: "load", CatalogID: "resistor.generic.0603", Family: "resistor", HasValueSI: true, ValueSI: load, ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveResistorV1}}, Connections: []simmodel.ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
		{InstanceID: "source", CatalogID: "source.voltage.connector.1x02", Family: "voltage_source", ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveVoltageSourceV1}}, Connections: []simmodel.ConnectionEvidence{{Function: "POSITIVE", Net: "VIN"}, {Function: "NEGATIVE", Net: "GND"}}},
	}
	plan, diagnostics := simmodel.ResolveWithTopology(intent, "catalog", testHash("catalog"), components, []simmodel.NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}, {Name: "VIN"}})
	if len(diagnostics) != 0 {
		return nil, fmt.Errorf("fresh MNA resolve diagnostics: %#v", diagnostics)
	}
	return map[string]simmodel.Plan{simmodel.AnalysisDCOperatingPoint: plan}, nil
}

func TestPlannedSimulationResolverRepairsAndReevaluatesAllSupplyCorners(t *testing.T) {
	requirement := closedLoopTestRequirement()
	minimum, maximum := 2.2, 2.8
	requirement.Requirements.BehavioralRequirements = []architecturesearch.BehavioralRequirement{{
		ID: "output", Metric: "dc_voltage", Analysis: simmodel.AnalysisDCOperatingPoint,
		Observation: architecturesearch.Observation{Kind: "port", ID: "output"}, Min: &minimum, Max: &maximum, Unit: "V", OperatingCases: []string{"rated"}, Critical: true,
	}}
	modelDecision := ModelDecision{
		Component: "load", Family: "resistor", Claim: simmodel.CatalogEvidence{ModelID: simmodel.PrimitiveResistorV1}, Status: "used", Reason: "trusted DC model",
		RequiredAnalyses: []string{simmodel.AnalysisDCOperatingPoint},
		Provenance:       &simmodel.ModelProvenance{Source: "manufacturer:test", Revision: "a", SHA256: testHash("model"), ReviewStatus: "reviewed", AllowedAnalyses: []string{simmodel.AnalysisDCOperatingPoint}},
	}
	analysisPlan, diagnostics := BuildAnalysisPlan(requirement, []SemanticBinding{{Kind: "port", ID: "output", Target: "OUT"}, {Kind: "domain", ID: "supply", Target: "SUPPLY"}}, []ModelDecision{modelDecision})
	if len(diagnostics) != 0 {
		t.Fatalf("analysis plan diagnostics = %#v", diagnostics)
	}
	resolver := PlannedSimulationResolver{
		Plan: analysisPlan, Base: repairableFreshMNAResolver{},
		Templates:         []SimulationAnalysisTemplate{{Kind: simmodel.AnalysisDCOperatingPoint, Analysis: simmodel.Analysis{ID: "template", Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{{Component: "source", DCValue: 5}}}}},
		Assertions:        []SimulationAssertionBinding{{Metric: "dc_voltage", Target: "OUT", BoundsMode: AssertionBoundsDirect, Prototypes: []simmodel.Assertion{{Node: "OUT", Quantity: simmodel.QuantityVoltageV}}}},
		OperatingBindings: []SimulationOperatingBinding{{Axis: "supply_voltage", Target: "SUPPLY", Kind: OperatingSourceDCValue, Component: "source"}},
	}
	registry, provenanceDiagnostics := modelprovenance.LoadDefault()
	if len(provenanceDiagnostics) != 0 {
		t.Fatalf("model provenance registry diagnostics = %#v", provenanceDiagnostics)
	}
	input := Input{
		Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formula"), ModelRegistryHash: testHash("models"),
		Candidates: []Candidate{{
			Fingerprint: testHash("candidate"),
			Variables: []Variable{{
				ID: "load_resistance", Kind: "passive_value", Value: 500, AllowedValues: []float64{500, 1000},
				Effects: []RepairEffect{{Analysis: simmodel.AnalysisDCOperatingPoint, Metric: "dc_voltage", Direction: RepairMetricIncreases}},
			}},
		}},
	}
	report := Run(context.Background(), input, SimModelEvaluator{Resolver: resolver, ProvenanceRegistry: registry}, DefaultPolicy())
	if report.Status != "pass" || report.Selected == nil || report.Selected.State.Variables[0].Value != 1000 {
		t.Fatalf("planned closed-loop report = %#v", report)
	}
	if report.Consumption.Evaluations != 2 || report.Consumption.RepairsApplied != 1 || len(report.Candidates[0].Attempts[1].Assertions) != 1 {
		t.Fatalf("planned repair consumption/evidence = %#v", report)
	}
	resolution, err := resolver.ResolveSimulation(context.Background(), report.Selected.State)
	if err != nil || len(resolution.Plans) != 1 || len(resolution.Plans[0].Analyses) != 2 || len(resolution.Measurements[0].Assertions) != 2 {
		t.Fatalf("fresh selected resolution = %#v err=%v", resolution, err)
	}
}
