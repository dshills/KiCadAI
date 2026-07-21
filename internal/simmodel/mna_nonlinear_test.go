package simmodel

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestNonlinearDCDiodeOperatingPointIsDeterministic(t *testing.T) {
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "5V", "GND"),
		resistorEvidence("limit", 1000, "5V", "OUT"),
		{InstanceID: "diode", CatalogID: "diode.onsemi.1n4148w.sod_123", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}},
	}
	plan := resolveNonlinearTestPlan(t, components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "OUT"}}, []Assertion{{AnalysisID: "bias", Node: "OUT", Quantity: QuantityVoltageV, Min: .55, Max: .9}})
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" || report.Analyses[0].Points[0].Solver == nil {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
	first, _ := json.Marshal(report)
	replayed, replayDiagnostics := Evaluate(ClonePlan(plan))
	if len(replayDiagnostics) != 0 {
		t.Fatalf("replay diagnostics=%+v", replayDiagnostics)
	}
	second, _ := json.Marshal(replayed)
	if string(first) != string(second) {
		t.Fatalf("nonlinear replay differs\n%s\n%s", first, second)
	}
}

func TestNonlinearDCManufacturerZenerModelClampsInReverse(t *testing.T) {
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "12V", "GND"),
		resistorEvidence("limit", 330, "12V", "OUT"),
		{InstanceID: "zener", CatalogID: "diode.diodes.ddz5v1b_7.sod123", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveUnidirectionalZenerV1, Parameters: zenerParameters()}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "GND"}, {Function: "CATHODE", Net: "OUT"}}},
	}
	intent := nonlinearTestIntent([]Assertion{{AnalysisID: "bias", Node: "OUT", Quantity: QuantityVoltageV, Min: 4.8, Max: 5.5}})
	intent.Analyses[0].Excitations[0].DCValue = 12
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "12V"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestNonlinearDCGuaranteedNMOSSwitchUsesCatalogGateAndResistance(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{
			{ID: "off", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}, {Component: "gate", DCValue: 0}}},
			{ID: "on", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}, {Component: "gate", DCValue: 2.5}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "off", Node: "DRAIN", Quantity: QuantityVoltageV, Min: 4.99, Max: 5.01},
			{AnalysisID: "on", Node: "DRAIN", Quantity: QuantityVoltageV, Min: 0, Max: .001},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "5V", "GND"), voltageSourceEvidence("gate", "GATE", "GND"), resistorEvidence("load", 1000, "5V", "DRAIN"),
		{InstanceID: "switch", CatalogID: "mosfet.aos.ao3400a.sot23", Family: "mosfet", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveNMOSSwitchV1, Parameters: nmosSwitchParameters()}}, Connections: []ConnectionEvidence{{Function: "GATE", Net: "GATE"}, {Function: "DRAIN", Net: "DRAIN"}, {Function: "SOURCE", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "GATE"}, {Name: "DRAIN"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestNonlinearDCGuaranteedPMOSSwitchUsesSourceReferencedGate(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{
			{ID: "off", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 12}, {Component: "gate", DCValue: 12}}},
			{ID: "on", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 12}, {Component: "gate", DCValue: 2}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "off", Node: "DRAIN", Quantity: QuantityVoltageV, Min: 0, Max: 1e-6},
			{AnalysisID: "on", Node: "DRAIN", Quantity: QuantityVoltageV, Min: 11.98, Max: 12.01},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "12V", "GND"), voltageSourceEvidence("gate", "GATE", "GND"), resistorEvidence("load", 1000, "DRAIN", "GND"),
		{InstanceID: "switch", CatalogID: "mosfet.vishay.irfp9240.to247", Family: "mosfet", ModelClaims: []CatalogEvidence{{ModelID: PrimitivePMOSSwitchV1, Parameters: pmosSwitchParameters()}}, Connections: []ConnectionEvidence{{Function: "GATE", Net: "GATE"}, {Function: "DRAIN", Net: "DRAIN"}, {Function: "SOURCE", Net: "12V"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "12V"}, {Name: "GATE"}, {Name: "DRAIN"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestNonlinearDCNPNAndPNPBias(t *testing.T) {
	for _, test := range []struct {
		name       string
		primitive  string
		components []ComponentEvidence
		nodes      []NodeEvidence
		assertions []Assertion
	}{
		{
			name: "npn", primitive: PrimitiveBJTNPNV1,
			components: []ComponentEvidence{voltageSourceEvidence("supply", "5V", "GND"), resistorEvidence("base_bias", 470000, "5V", "BASE"), resistorEvidence("collector_load", 1000, "5V", "COLLECTOR")},
			nodes:      []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "BASE"}, {Name: "COLLECTOR"}},
			assertions: []Assertion{{AnalysisID: "bias", Node: "BASE", Quantity: QuantityVoltageV, Min: .5, Max: .9}, {AnalysisID: "bias", Node: "COLLECTOR", Quantity: QuantityVoltageV, Min: 3.5, Max: 4.8}},
		},
		{
			name: "pnp", primitive: PrimitiveBJTPNPV1,
			components: []ComponentEvidence{voltageSourceEvidence("supply", "5V", "GND"), resistorEvidence("base_bias", 470000, "BASE", "GND"), resistorEvidence("collector_load", 1000, "COLLECTOR", "GND")},
			nodes:      []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "BASE"}, {Name: "COLLECTOR"}},
			assertions: []Assertion{{AnalysisID: "bias", Node: "BASE", Quantity: QuantityVoltageV, Min: 4.1, Max: 4.5}, {AnalysisID: "bias", Node: "COLLECTOR", Quantity: QuantityVoltageV, Min: .2, Max: 1.5}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			emitterNet := "GND"
			if test.primitive == PrimitiveBJTPNPV1 {
				emitterNet = "5V"
			}
			test.components = append(test.components, ComponentEvidence{InstanceID: "q1", CatalogID: "reviewed-bjt", Family: "bjt", ModelClaims: []CatalogEvidence{{ModelID: test.primitive, Parameters: bjtParameters(.2, 40)}}, Connections: []ConnectionEvidence{{Function: "BASE", Net: "BASE"}, {Function: "COLLECTOR", Net: "COLLECTOR"}, {Function: "EMITTER", Net: emitterNet}}})
			plan := resolveNonlinearTestPlan(t, test.components, test.nodes, test.assertions)
			report, diagnostics := Evaluate(plan)
			if len(diagnostics) != 0 || report.Status != "pass" {
				t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
			}
		})
	}
}

func TestNonlinearDCComplementaryEmitterFollowerIsPolaritySymmetric(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{
			{ID: "positive", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "positive_supply", DCValue: 5}, {Component: "negative_supply", DCValue: -5}, {Component: "drive", DCValue: 1}}},
			{ID: "negative", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "positive_supply", DCValue: 5}, {Component: "negative_supply", DCValue: -5}, {Component: "drive", DCValue: -1}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "positive", Node: "OUT", Quantity: QuantityVoltageV, Min: .1, Max: .8},
			{AnalysisID: "negative", Node: "OUT", Quantity: QuantityVoltageV, Min: -.8, Max: -.1},
		},
	}
	parameters := bjtParameters(.2, 40)
	components := []ComponentEvidence{
		voltageSourceEvidence("positive_supply", "VCC", "GND"),
		voltageSourceEvidence("negative_supply", "VEE", "GND"),
		voltageSourceEvidence("drive", "DRIVE", "GND"),
		resistorEvidence("npn_emitter", .22, "NPN_EMITTER", "OUT"),
		resistorEvidence("pnp_emitter", .22, "PNP_EMITTER", "OUT"),
		resistorEvidence("load", 8, "OUT", "GND"),
		{InstanceID: "npn", CatalogID: "reviewed-npn", Family: "bjt", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBJTNPNV1, Parameters: parameters}}, Connections: []ConnectionEvidence{{Function: "BASE", Net: "DRIVE"}, {Function: "COLLECTOR", Net: "VCC"}, {Function: "EMITTER", Net: "NPN_EMITTER"}}},
		{InstanceID: "pnp", CatalogID: "reviewed-pnp", Family: "bjt", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBJTPNPV1, Parameters: parameters}}, Connections: []ConnectionEvidence{{Function: "BASE", Net: "DRIVE"}, {Function: "COLLECTOR", Net: "VEE"}, {Function: "EMITTER", Net: "PNP_EMITTER"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VCC"}, {Name: "VEE"}, {Name: "DRIVE"}, {Name: "NPN_EMITTER"}, {Name: "PNP_EMITTER"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestNonlinearDCRejectsACAmbiguousClaimsAndOperatingLimit(t *testing.T) {
	base := []ComponentEvidence{voltageSourceEvidence("supply", "5V", "GND"), resistorEvidence("limit", 1000, "5V", "OUT"), {InstanceID: "diode", CatalogID: "diode", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}}}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "OUT"}}
	ac := Intent{ModelID: ModelNonlinearCircuitDCV1, Analyses: []Analysis{{ID: "ac", Kind: AnalysisACSweep, StartFrequencyHz: 1, StopFrequencyHz: 10, Points: 2, Excitations: []SourceExcitation{{Component: "supply", ACMagnitude: 1}}}}, Assertions: []Assertion{{AnalysisID: "ac", Node: "OUT", Quantity: QuantityVoltageMagnitudeV, FrequencyHz: 1, Min: 0, Max: 10}}}
	if _, diagnostics := ResolveWithTopology(ac, "test", "hash", base, nodes); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "DC operating points only") {
		t.Fatalf("AC diagnostics=%+v", diagnostics)
	}
	ambiguous := append([]ComponentEvidence(nil), base...)
	ambiguous[2].ModelClaims = append(ambiguous[2].ModelClaims, ambiguous[2].ModelClaims[0])
	intent := nonlinearTestIntent([]Assertion{{AnalysisID: "bias", Node: "OUT", Quantity: QuantityVoltageV, Min: 0, Max: 5}})
	if _, diagnostics := ResolveWithTopology(intent, "test", "hash", ambiguous, nodes); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "ambiguous") {
		t.Fatalf("ambiguous diagnostics=%+v", diagnostics)
	}
	missing := append([]ComponentEvidence(nil), base...)
	missing[2].ModelClaims = []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)[1:]}}
	if _, diagnostics := ResolveWithTopology(intent, "test", "hash", missing, nodes); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "missing required parameter saturation_current_a") {
		t.Fatalf("missing-parameter diagnostics=%+v", diagnostics)
	}
	limited := append([]ComponentEvidence(nil), base...)
	limited[2].ModelClaims = []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(1e-6, 100)}}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", limited, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	_, diagnostics = Evaluate(plan)
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "forward current") || diagnostics[0].Suggestion == "" {
		t.Fatalf("limit diagnostics=%+v", diagnostics)
	}
}

func diagnosticsContain(diagnostics []Diagnostic, fragment string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, fragment) {
			return true
		}
	}
	return false
}

func TestNonlinearDCReportsActionableBoundedSolveFailure(t *testing.T) {
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "5V", "GND"),
		voltageSourceEvidence("conflict", "5V", "GND"),
		resistorEvidence("limit", 1000, "5V", "OUT"),
		{InstanceID: "diode", CatalogID: "diode", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}},
	}
	intent := nonlinearTestIntent([]Assertion{{AnalysisID: "bias", Node: "OUT", Quantity: QuantityVoltageV, Min: 0, Max: 5}})
	intent.Analyses[0].Excitations = append(intent.Analyses[0].Excitations, SourceExcitation{Component: "conflict", DCValue: 4})
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	_, diagnostics = Evaluate(plan)
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "continuation stage") || !strings.Contains(diagnostics[0].Suggestion, "bias path") {
		t.Fatalf("solve diagnostics=%+v", diagnostics)
	}
}

func TestBidirectionalOpenDrainTranslatorPreservesHighRailsAndPropagatesLow(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{{ID: "bus", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
			{Component: "low_supply", DCValue: 3.3}, {Component: "high_supply", DCValue: 5}, {Component: "driver", DCValue: 0},
		}}},
		Assertions: []Assertion{
			{AnalysisID: "bus", Node: "A1", Quantity: QuantityVoltageV, Min: 0, Max: .01},
			{AnalysisID: "bus", Node: "B1", Quantity: QuantityVoltageV, Min: 0, Max: .4},
			{AnalysisID: "bus", Node: "A2", Quantity: QuantityVoltageV, Min: 3.29, Max: 3.31},
			{AnalysisID: "bus", Node: "B2", Quantity: QuantityVoltageV, Min: 4.99, Max: 5},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("low_supply", "VCCA", "GND"),
		voltageSourceEvidence("high_supply", "VCCB", "GND"),
		voltageSourceEvidence("driver", "A1", "GND"),
		resistorEvidence("a1_pullup", 4700, "VCCA", "A1"),
		resistorEvidence("b1_pullup", 4700, "VCCB", "B1"),
		resistorEvidence("a2_pullup", 4700, "VCCA", "A2"),
		resistorEvidence("b2_pullup", 4700, "VCCB", "B2"),
		{InstanceID: "translator", CatalogID: "translator", Family: "level_translator", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBidirectionalOpenDrainTranslatorV1, Parameters: openDrainTranslatorParameters()}}, Connections: []ConnectionEvidence{{Function: "A1", Net: "A1"}, {Function: "A2", Net: "A2"}, {Function: "B1", Net: "B1"}, {Function: "B2", Net: "B2"}, {Function: "VCCA", Net: "VCCA"}, {Function: "VCCB", Net: "VCCB"}, {Function: "GND", Net: "GND"}, {Function: "OE", Net: "VCCA"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VCCA"}, {Name: "VCCB"}, {Name: "A1"}, {Name: "A2"}, {Name: "B1"}, {Name: "B2"}})
	if len(diagnostics) != 0 {
		t.Fatalf("translator resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("translator report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestBidirectionalTVSClampsBothPolarities(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{
			{ID: "negative", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "source", DCValue: -12}}},
			{ID: "positive", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "source", DCValue: 12}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "negative", Node: "OUT", Quantity: QuantityVoltageV, Min: -9.52, Max: -9.50},
			{AnalysisID: "positive", Node: "OUT", Quantity: QuantityVoltageV, Min: 9.50, Max: 9.52},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("source", "SOURCE", "GND"),
		resistorEvidence("series", 100, "SOURCE", "OUT"),
		{InstanceID: "clamp", CatalogID: "tvs", Family: "protection", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBidirectionalTVSV1, Parameters: tvsParameters()}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}, {Name: "SOURCE"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestBidirectionalTVSSmallSignalIncludesCatalogJunctionCapacitance(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "response", Kind: AnalysisACSweep, StartFrequencyHz: 1e7, StopFrequencyHz: 1e7, Points: 2,
			Excitations: []SourceExcitation{{Component: "source", ACMagnitude: 1}},
		}},
		Assertions: []Assertion{{AnalysisID: "response", Node: "OUT", Quantity: QuantityVoltageMagnitudeV, FrequencyHz: 1e7, Min: .034, Max: .037}},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("source", "SOURCE", "GND"),
		resistorEvidence("series", 10_000, "SOURCE", "OUT"),
		{InstanceID: "clamp", CatalogID: "tvs", Family: "protection", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBidirectionalTVSV1, Parameters: tvsParameters()}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}, {Name: "SOURCE"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestPiecewiseLinearRegionStableRejectsBoundaryCrossing(t *testing.T) {
	system := mnaSystem{nodeIndex: map[string]int{"OUT": 0}}
	device := compiledNonlinearDevice{
		primitive:  PrimitiveBidirectionalTVSV1,
		terminals:  map[string]string{"ANODE": "OUT", "CATHODE": "GND"},
		parameters: map[string]float64{"breakdown_voltage_v": 5},
	}
	if !piecewiseLinearRegionStable([]compiledNonlinearDevice{device}, &system, []complex128{1}, []complex128{4}) {
		t.Fatal("same off-region TVS step was not recognized as exact")
	}
	if piecewiseLinearRegionStable([]compiledNonlinearDevice{device}, &system, []complex128{1}, []complex128{6}) {
		t.Fatal("TVS breakdown-region crossing was accepted as exact")
	}
}

func TestNonlinearResidualIsNormalizedByResolvedEquationScale(t *testing.T) {
	base := mnaSystem{
		matrix:        [][]complex128{{1e6}},
		rhs:           []complex128{1},
		unknownLabels: []string{"branch_current:high_gain"},
	}
	residual, label := nonlinearResidual(base, nil, []complex128{complex(1.000000000001e-6, 0)})
	if label != "branch_current:high_gain" || residual >= nonlinearResidualTolerance {
		t.Fatalf("normalized residual = %.12g label=%q", residual, label)
	}
}

func TestNonlinearIterationConvergenceAcceptsOnlyResidualBoundedNumericalFloor(t *testing.T) {
	if !nonlinearIterationConverged(5e-8, 5e-11, nonlinearResidualTolerance/2) {
		t.Fatal("residual-bounded nanovolt update should be accepted at the numerical floor")
	}
	if nonlinearIterationConverged(1e-6, 5e-11, 0) {
		t.Fatal("meaningful voltage update must not be hidden by a zero residual")
	}
	if nonlinearIterationConverged(nonlinearResidualFloorUpdateV*1.01, 5e-11, 0) {
		t.Fatal("voltage update above the numerical floor must still block convergence")
	}
	if nonlinearIterationConverged(5e-8, 5e-9, 0) {
		t.Fatal("meaningful branch-current update must still block convergence")
	}
	if nonlinearIterationConverged(5e-8, 5e-11, nonlinearResidualTolerance*2) {
		t.Fatal("nonzero normalized residual above tolerance must still block convergence")
	}
}

func resolveNonlinearTestPlan(t *testing.T, components []ComponentEvidence, nodes []NodeEvidence, assertions []Assertion) Plan {
	t.Helper()
	plan, diagnostics := ResolveWithTopology(nonlinearTestIntent(assertions), "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	return plan
}

func nonlinearTestIntent(assertions []Assertion) Intent {
	return Intent{ModelID: ModelNonlinearCircuitDCV1, Analyses: []Analysis{{ID: "bias", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}}}}, Assertions: assertions}
}

func voltageSourceEvidence(id, positive, negative string) ComponentEvidence {
	return ComponentEvidence{InstanceID: id, CatalogID: "source.voltage", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: positive}, {Function: "NEGATIVE", Net: negative}}}
}

func resistorEvidence(id string, value float64, a, b string) ComponentEvidence {
	return ComponentEvidence{InstanceID: id, CatalogID: "resistor", Family: "resistor", ValueSI: value, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: a}, {Function: "B", Net: b}}}
}

func diodeParameters(maxCurrent, maxReverse float64) []NamedValue {
	return []NamedValue{{Name: "saturation_current_a", Value: 4e-9}, {Name: "emission_coefficient", Value: 1.9}, {Name: "junction_temperature_k", Value: 300.15}, {Name: "max_forward_current_a", Value: maxCurrent}, {Name: "max_reverse_voltage_v", Value: maxReverse}}
}

func tvsParameters() []NamedValue {
	return []NamedValue{
		{Name: "breakdown_voltage_v", Value: 9.5},
		{Name: "dynamic_resistance_ohm", Value: .5},
		{Name: "junction_capacitance_f", Value: 45e-12},
		{Name: "max_pulse_current_a", Value: 12},
		{Name: "off_resistance_ohm", Value: 50e6},
	}
}

func openDrainTranslatorParameters() []NamedValue {
	return []NamedValue{
		{Name: "vcca_min_v", Value: 1.65}, {Name: "vcca_max_v", Value: 3.6},
		{Name: "vccb_min_v", Value: 2.3}, {Name: "vccb_max_v", Value: 5.5},
		{Name: "low_level_threshold_v", Value: .15}, {Name: "enable_high_ratio", Value: .65},
		{Name: "channel_on_resistance_ohm", Value: 400}, {Name: "channel_off_resistance_ohm", Value: 2.75e6},
		{Name: "max_channel_current_a", Value: .05},
		{Name: "vcca_quiescent_current_a", Value: 2.4e-6}, {Name: "vccb_quiescent_current_a", Value: 12e-6},
		{Name: "max_temperature_c", Value: 150}, {Name: "junction_to_ambient_c_per_w", Value: 239.8},
	}
}

func bjtParameters(maxCurrent, maxVoltage float64) []NamedValue {
	return []NamedValue{{Name: "saturation_current_a", Value: 1e-14}, {Name: "forward_beta", Value: 100}, {Name: "reverse_beta", Value: 1}, {Name: "emission_coefficient", Value: 1}, {Name: "junction_temperature_k", Value: 300.15}, {Name: "max_collector_current_a", Value: maxCurrent}, {Name: "max_collector_emitter_voltage_v", Value: maxVoltage}}
}

func TestSiliconSaturationCurrentTracksTemperatureFromNominalReference(t *testing.T) {
	nominal := 1e-14
	if actual := siliconSaturationCurrentAtTemperature(nominal, 1, nonlinearNominalTemperatureK); math.Abs(actual-nominal) > nominal*1e-12 {
		t.Fatalf("nominal saturation current = %.12g, want %.12g", actual, nominal)
	}
	cold := siliconSaturationCurrentAtTemperature(nominal, 1, 273.15)
	hot := siliconSaturationCurrentAtTemperature(nominal, 1, 323.15)
	if !(cold > 0 && cold < nominal && hot > nominal) {
		t.Fatalf("temperature-adjusted saturation currents = cold %.12g nominal %.12g hot %.12g", cold, nominal, hot)
	}
}

func zenerParameters() []NamedValue {
	return []NamedValue{
		{Name: "forward_saturation_current_a", Value: 4.04e-11}, {Name: "forward_series_resistance_ohm", Value: 34.9}, {Name: "forward_emission_coefficient", Value: 1.1},
		{Name: "reverse_saturation_current_a", Value: 8.08e-15}, {Name: "reverse_series_resistance_ohm", Value: 13.1}, {Name: "reverse_emission_coefficient", Value: 3},
		{Name: "zener_offset_voltage_v", Value: 2.62}, {Name: "junction_temperature_k", Value: 300.15}, {Name: "max_current_a", Value: .0980392156863},
	}
}

func nmosSwitchParameters() []NamedValue {
	return []NamedValue{
		{Name: "gate_on_voltage_v", Value: 2.5}, {Name: "on_resistance_ohm", Value: .048},
		{Name: "max_drain_current_a", Value: 5.7}, {Name: "max_drain_source_voltage_v", Value: 30}, {Name: "max_gate_source_voltage_v", Value: 12},
	}
}

func pmosSwitchParameters() []NamedValue {
	return []NamedValue{
		{Name: "gate_on_voltage_v", Value: 10}, {Name: "on_resistance_ohm", Value: .5},
		{Name: "max_drain_current_a", Value: 12}, {Name: "max_drain_source_voltage_v", Value: 200},
		{Name: "max_gate_source_voltage_v", Value: 20}, {Name: "max_temperature_c", Value: 150},
		{Name: "junction_to_ambient_c_per_w", Value: 40},
	}
}

func TestBoundedExponentialIsFinite(t *testing.T) {
	value, derivative := boundedExponential(1e9)
	if math.IsInf(value, 0) || math.IsInf(derivative, 0) || value <= 0 || derivative <= 0 {
		t.Fatalf("value=%g derivative=%g", value, derivative)
	}
}

func TestAdvanceActiveDeviceStateChangesOneDeviceInPlanOrder(t *testing.T) {
	plan := Plan{Devices: []ResolvedDevice{
		{Component: "first", PrimitiveModel: PrimitiveOpAmpV1},
		{Component: "second", PrimitiveModel: PrimitiveComparatorOpenCollectorV1},
	}}
	resolved := map[string]float64{"first": 4.9, "second": 1}

	first := advanceActiveDeviceState(plan, nil, resolved)
	if len(first) != 1 || first["first"] != 4.9 {
		t.Fatalf("first transition = %#v", first)
	}
	second := advanceActiveDeviceState(plan, first, resolved)
	if len(second) != 2 || second["first"] != 4.9 || second["second"] != 1 {
		t.Fatalf("second transition = %#v", second)
	}
}

func TestAdvanceActiveDeviceStateReleasesOpAmpBeforeOppositeRail(t *testing.T) {
	plan := Plan{Devices: []ResolvedDevice{{Component: "amplifier", PrimitiveModel: PrimitiveOpAmpV1}}}
	next := advanceActiveDeviceState(plan, map[string]float64{"amplifier": 0.1}, map[string]float64{"amplifier": 4.9})
	if _, clamped := next["amplifier"]; clamped {
		t.Fatalf("op-amp jumped directly between rail clamps: %#v", next)
	}
}

func TestAdvanceActiveDeviceStatePrioritizesLinearOutputBeforeComparatorOrder(t *testing.T) {
	plan := Plan{Devices: []ResolvedDevice{
		{Component: "decision", PrimitiveModel: PrimitiveComparatorOpenCollectorV1},
		{Component: "filter", PrimitiveModel: PrimitiveOpAmpV1},
	}}
	current := map[string]float64{"decision": 1, "filter": .065}
	resolved := map[string]float64{"decision": 0, "filter": 4.38}
	next := advanceActiveDeviceState(plan, current, resolved)
	if next["decision"] != 1 {
		t.Fatalf("comparator changed before upstream linear output: %#v", next)
	}
	if _, clamped := next["filter"]; clamped {
		t.Fatalf("upstream op-amp clamp was not released first: %#v", next)
	}
}
