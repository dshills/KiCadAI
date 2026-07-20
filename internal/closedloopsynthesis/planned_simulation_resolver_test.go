package closedloopsynthesis

import (
	"context"
	"fmt"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

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
