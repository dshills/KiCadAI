package simmodel

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestTransientSourceAndDeviceValueEventsExecuteDeterministically(t *testing.T) {
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "events", Kind: AnalysisTransient, DurationS: .2, TimeStepS: .1,
			Excitations: []SourceExcitation{{Component: "source", DCValue: 1}},
			SourceValueEvents: []SourceValueEvent{{
				ID: "source_step", Component: "source", TriggerTimeS: .1, DurationS: .1,
				Initial: 1, Applied: 2,
			}},
			DeviceValueEvents: []DeviceValueEvent{{
				ID: "load_step", Component: "load", TriggerTimeS: .1, DurationS: .1,
				InitialSI: 1000, AppliedSI: 500,
			}},
		}},
		Assertions: []Assertion{{AnalysisID: "events", Node: "OUT", Quantity: QuantityVoltageV, TimeS: .2, Min: 1.999, Max: 2.001}},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("source", "OUT", "GND"),
		resistorEvidence("load", 1000, "OUT", "GND"),
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve value-event plan: %#v", diagnostics)
	}
	originalPlan := ClonePlan(plan)
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("value-event report=%#v diagnostics=%#v", report, diagnostics)
	}
	if !reflect.DeepEqual(plan, originalPlan) {
		t.Fatal("transient value-event evaluation mutated its input plan")
	}
	last := report.Analyses[0].Points[len(report.Analyses[0].Points)-1]
	loadCurrent := 0.0
	for _, device := range last.Devices {
		if device.Component == "load" {
			loadCurrent = math.Abs(device.CurrentA)
		}
	}
	if math.Abs(loadCurrent-.004) > 1e-9 {
		t.Fatalf("event-applied load current = %.12g A, want 0.004 A", loadCurrent)
	}
	replay, replayDiagnostics := Evaluate(ClonePlan(plan))
	if len(replayDiagnostics) != 0 || !reflect.DeepEqual(report, replay) {
		t.Fatalf("value-event replay differs: diagnostics=%#v", replayDiagnostics)
	}
}

func TestReusableMNASystemCloneRestoresContentAndDoesNotAliasSource(t *testing.T) {
	source := mnaSystem{
		matrix: [][]complex128{{1, 2}, {3, 4}}, rhs: []complex128{5, 6},
		unknownLabels: []string{"A", "B"}, nodeIndex: map[string]int{"A": 0},
		branchIndex: map[string]int{"source": 1}, multiBranchIndex: map[mnaBranchKey]int{{component: "source", terminal: "OUT"}: 1},
	}
	first := acquireReusableMNASystemClone(&source)
	first.system.matrix[0][0] = 99
	first.system.rhs[0] = 88
	if source.matrix[0][0] != 1 || source.rhs[0] != 5 {
		t.Fatal("reusable MNA clone aliases source storage")
	}
	releaseReusableMNASystemClone(first)
	second := acquireReusableMNASystemClone(&source)
	defer releaseReusableMNASystemClone(second)
	if !reflect.DeepEqual(second.system.matrix, source.matrix) || !reflect.DeepEqual(second.system.rhs, source.rhs) {
		t.Fatalf("reused MNA clone was not restored: matrix=%v rhs=%v", second.system.matrix, second.system.rhs)
	}
	if !reflect.DeepEqual(second.system.unknownLabels, source.unknownLabels) || second.system.nodeIndex["A"] != 0 || second.system.branchIndex["source"] != 1 {
		t.Fatal("reused MNA clone lost immutable index metadata")
	}
}

func TestPushPullDigitalIsolatorFailsSafeLowAfterEitherSourceSupplyLoss(t *testing.T) {
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{
			{
				ID: "side_1_lost", Kind: AnalysisTransient, DurationS: 2e-6, TimeStepS: 100e-9,
				Excitations: []SourceExcitation{
					{Component: "supply_1", DCValue: 3.3}, {Component: "supply_2", DCValue: 3.3},
					{Component: "enable_1", DCValue: 3.3}, {Component: "enable_2", DCValue: 3.3},
					{Component: "ina1", DCValue: 3.3}, {Component: "ina2", DCValue: 0},
					{Component: "ina3", DCValue: 0}, {Component: "inb4", DCValue: 3.3},
				},
				SourceValueEvents: []SourceValueEvent{{
					ID: "remove_side_1_supply", Component: "supply_1",
					TriggerTimeS: 500e-9, DurationS: 1.5e-6, Initial: 3.3, Applied: 0,
				}},
			},
			{
				ID: "side_2_lost", Kind: AnalysisTransient, DurationS: 2e-6, TimeStepS: 100e-9,
				Excitations: []SourceExcitation{
					{Component: "supply_1", DCValue: 3.3}, {Component: "supply_2", DCValue: 3.3},
					{Component: "enable_1", DCValue: 3.3}, {Component: "enable_2", DCValue: 3.3},
					{Component: "ina1", DCValue: 3.3}, {Component: "ina2", DCValue: 0},
					{Component: "ina3", DCValue: 0}, {Component: "inb4", DCValue: 3.3},
				},
				SourceValueEvents: []SourceValueEvent{{
					ID: "remove_side_2_supply", Component: "supply_2",
					TriggerTimeS: 500e-9, DurationS: 1.5e-6, Initial: 3.3, Applied: 0,
				}},
			},
		},
		Assertions: []Assertion{
			{AnalysisID: "side_1_lost", Node: "OUTB1", ReferenceNode: "GND2", Quantity: QuantityVoltageV, TimeS: 2e-6, Min: 0, Max: .01},
			{AnalysisID: "side_2_lost", Node: "OUTA4", ReferenceNode: "GND1", Quantity: QuantityVoltageV, TimeS: 2e-6, Min: 0, Max: .01},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply_1", "VDD1", "GND1"), voltageSourceEvidence("supply_2", "VDD2", "GND2"),
		voltageSourceEvidence("enable_1", "EN1", "GND1"), voltageSourceEvidence("enable_2", "EN2", "GND2"),
		voltageSourceEvidence("ina1", "INA1", "GND1"), voltageSourceEvidence("ina2", "INA2", "GND1"),
		voltageSourceEvidence("ina3", "INA3", "GND1"), voltageSourceEvidence("inb4", "INB4", "GND2"),
		resistorEvidence("outb1_pullup", 1e6, "OUTB1", "VDD2"), resistorEvidence("outb2_load", 1e6, "OUTB2", "GND2"),
		resistorEvidence("outb3_load", 1e6, "OUTB3", "GND2"), resistorEvidence("outa4_pullup", 1e6, "OUTA4", "VDD1"),
		pushPullIsolatorEvidence("isolator"),
	}
	nodes := []NodeEvidence{
		{Name: "GND1", Role: "ground", VoltageDomain: "side_1"}, {Name: "GND2", Role: "ground", VoltageDomain: "side_2"},
		{Name: "VDD1", VoltageDomain: "side_1"}, {Name: "VDD2", VoltageDomain: "side_2"},
		{Name: "EN1", VoltageDomain: "side_1"}, {Name: "EN2", VoltageDomain: "side_2"},
		{Name: "INA1", VoltageDomain: "side_1"}, {Name: "INA2", VoltageDomain: "side_1"}, {Name: "INA3", VoltageDomain: "side_1"},
		{Name: "INB4", VoltageDomain: "side_2"}, {Name: "OUTB1", VoltageDomain: "side_2"}, {Name: "OUTB2", VoltageDomain: "side_2"},
		{Name: "OUTB3", VoltageDomain: "side_2"}, {Name: "OUTA4", VoltageDomain: "side_1"},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("push-pull isolator supply-loss resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("push-pull isolator supply-loss report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestTransientCurrentSourceObservationsUseInstantaneousExcitation(t *testing.T) {
	analysis := Analysis{
		ID: "load_event", Kind: AnalysisTransient, DurationS: .2, TimeStepS: .1,
		Excitations: []SourceExcitation{
			{Component: "supply", DCValue: 5},
			{Component: "load", DCValue: 1},
		},
		SourceValueEvents: []SourceValueEvent{{
			ID: "load_step", Component: "load", TriggerTimeS: .1, DurationS: .1,
			Initial: 1, Applied: 2,
		}},
	}
	plan := Plan{
		GroundNode: "GND",
		Nodes:      []string{"GND", "OUT"},
		Devices: []ResolvedDevice{
			{
				Component: "supply", PrimitiveModel: PrimitiveVoltageSourceV1,
				Terminals: []TerminalBinding{{Terminal: "POSITIVE", Net: "OUT"}, {Terminal: "NEGATIVE", Net: "GND"}},
			},
			{
				Component: "load", PrimitiveModel: PrimitiveCurrentSourceV1,
				Terminals: []TerminalBinding{{Terminal: "POSITIVE", Net: "OUT"}, {Terminal: "NEGATIVE", Net: "GND"}},
			},
		},
	}
	result, diagnostics := solveTransientAnalysis(plan, analysis)
	if len(diagnostics) != 0 {
		t.Fatalf("transient diagnostics = %#v", diagnostics)
	}
	want := []float64{1, 2, 2}
	if len(result.Points) != len(want) {
		t.Fatalf("transient point count = %d, want %d", len(result.Points), len(want))
	}
	for pointIndex, point := range result.Points {
		found := false
		for _, device := range point.Devices {
			if device.Component != "load" {
				continue
			}
			found = true
			if device.CurrentA != want[pointIndex] || device.CurrentMagnitudeA != want[pointIndex] {
				t.Fatalf("load observation at point %d = %#v, want current %.12g A", pointIndex, device, want[pointIndex])
			}
		}
		if !found {
			t.Fatalf("load observation missing at point %d", pointIndex)
		}
	}
}

func TestTransientEventAtZeroRetainsInitialBoundarySample(t *testing.T) {
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "startup", Kind: AnalysisTransient, DurationS: .2, TimeStepS: .1,
			Excitations: []SourceExcitation{{Component: "source", DCValue: 1}},
			SourceValueEvents: []SourceValueEvent{{
				ID: "power_up", Component: "source", TriggerTimeS: 0, DurationS: .2,
				Initial: 0, Applied: 1,
			}},
		}},
		Assertions: []Assertion{{AnalysisID: "startup", Node: "OUT", Quantity: QuantityVoltageV, TimeS: .2, Min: .999, Max: 1.001}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash",
		[]ComponentEvidence{voltageSourceEvidence("source", "OUT", "GND"), resistorEvidence("load", 1000, "OUT", "GND")},
		[]NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}},
	)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%#v diagnostics=%#v", report, diagnostics)
	}
	points := report.Analyses[0].Points
	if len(points) != 3 {
		t.Fatalf("boundary points=%#v", points)
	}
	initialV, initialOK := analysisNodeMagnitude(points[0], "OUT")
	appliedV, appliedOK := analysisNodeMagnitude(points[1], "OUT")
	if !initialOK || !appliedOK || initialV != 0 || appliedV < .999 {
		t.Fatalf("boundary points=%#v", points)
	}
}

func TestTransientConditionValueEventAppliesAndRecovers(t *testing.T) {
	recovered := 1.0
	analysis := Analysis{
		ID: "thermal_event", Kind: AnalysisElectrothermal, DurationS: .3, TimeStepS: .1,
		Excitations: []SourceExcitation{{Component: "source", DCValue: 1}},
		Conditions:  []NamedValue{{Name: "ambient_temperature_c", Value: 25}, {Name: "thermal_resistance_scale", Value: 1}},
		ConditionValueEvents: []ConditionValueEvent{{
			ID: "blocked_airflow", Name: "thermal_resistance_scale", TriggerTimeS: .1, DurationS: .1,
			Initial: 1, Applied: 3, Recovered: &recovered,
		}},
	}
	for _, test := range []struct {
		time float64
		want float64
	}{{0, 1}, {.1, 3}, {.2, 1}} {
		resolved := transientDCAnalysis(analysis, test.time)
		if got := namedValueMap(resolved.Conditions)["thermal_resistance_scale"]; got != test.want {
			t.Fatalf("thermal scale at %.1f s = %g, want %g", test.time, got, test.want)
		}
	}
}

func TestTransientDCAnalysisInitializesSineAtDeclaredDCBias(t *testing.T) {
	analysis := Analysis{
		TimeStepS: 1.0 / 32_000,
		Excitations: []SourceExcitation{{
			Component:       "signal",
			DCValue:         .25,
			SineAmplitude:   1,
			SineFrequencyHz: 1000,
			SinePhaseDeg:    180,
		}},
	}
	resolved := transientDCAnalysis(analysis, -analysis.TimeStepS)
	if got := resolved.Excitations[0].DCValue; got != .25 {
		t.Fatalf("periodic initial DC value = %.12g, want declared bias 0.25", got)
	}
	if resolved.Excitations[0].SineAmplitude != 0 || resolved.Excitations[0].SineFrequencyHz != 0 {
		t.Fatalf("periodic source was not reduced to DC: %#v", resolved.Excitations[0])
	}
}

func TestTransientDCAnalysisPreservesPulseLeftHandBoundary(t *testing.T) {
	analysis := Analysis{
		TimeStepS: .1,
		Excitations: []SourceExcitation{{
			Component:         "supply",
			PulseInitialValue: 0,
			PulseValue:        5,
			PulseDelayS:       0,
			PulseWidthS:       .5,
			PulsePeriodS:      1,
		}},
	}
	resolved := transientDCAnalysis(analysis, -analysis.TimeStepS)
	if got := resolved.Excitations[0].DCValue; got != 0 {
		t.Fatalf("pulse left-hand initial DC value = %.12g, want 0", got)
	}
}

func TestPeriodicTransientMeasurementsUseSettledTwoCycleWindow(t *testing.T) {
	result := AnalysisResult{Kind: AnalysisTransient, FundamentalFrequencyHz: 1}
	for index := 0; index <= 12; index++ {
		voltage, powerVoltage := .02, 2.0
		if index < 4 {
			voltage, powerVoltage = 100, 100
		}
		result.Points = append(result.Points, AnalysisPoint{
			TimeS:   float64(index) * .25,
			Nodes:   []NodeResult{{Node: "OUT", Real: voltage}},
			Devices: []DeviceResult{{Component: "LOAD", VoltageV: powerVoltage, CurrentA: 2}},
		})
	}
	peak, diagnostic := peakAbsVoltage(result, Assertion{AnalysisID: "transient", Node: "OUT"})
	if diagnostic != nil || peak != .02 {
		t.Fatalf("peakAbsVoltage() = %.12g, %#v; want settled 0.02 V", peak, diagnostic)
	}
	power, diagnostic := transientDerivedValue(result, Assertion{AnalysisID: "transient", Component: "LOAD", Quantity: QuantityOutputPowerW})
	if diagnostic != nil || power != 4 {
		t.Fatalf("output power = %.12g, %#v; want settled 4 W", power, diagnostic)
	}
}

func TestTHDFiveHarmonicMeasurementConvergesAtTrustedMinimumGrid(t *testing.T) {
	build := func(samplesPerCycle int) AnalysisResult {
		frequency := 1000.0
		timeStep := 1 / (frequency * float64(samplesPerCycle))
		result := AnalysisResult{Kind: AnalysisDistortion, FundamentalFrequencyHz: frequency}
		for index := 0; index <= 4*samplesPerCycle; index++ {
			phase := 2 * math.Pi * frequency * float64(index) * timeStep
			value := math.Sin(phase) + .01*math.Sin(2*phase) + .02*math.Sin(3*phase)
			result.Points = append(result.Points, AnalysisPoint{TimeS: float64(index) * timeStep, Nodes: []NodeResult{{Node: "OUT", Real: value}}})
		}
		return result
	}
	assertion := Assertion{AnalysisID: "distortion", Node: "OUT", Quantity: QuantityTHDPercent}
	coarse, coarseDiagnostic := totalHarmonicDistortion(build(16), assertion)
	fine, fineDiagnostic := totalHarmonicDistortion(build(32), assertion)
	want := 100 * math.Hypot(.01, .02)
	if coarseDiagnostic != nil || fineDiagnostic != nil || math.Abs(coarse-want) > 1e-10 || math.Abs(fine-want) > 1e-10 || math.Abs(coarse-fine) > 1e-12 {
		t.Fatalf("THD convergence: 16-point=%.12g (%#v), 32-point=%.12g (%#v), want %.12g", coarse, coarseDiagnostic, fine, fineDiagnostic, want)
	}
}

func TestTransientAnalysisWorkersPreserveOrderAndReplay(t *testing.T) {
	plan := resolveTransientSwitchPlan(t, 25)
	second := cloneAnalyses(plan.Analyses)[0]
	second.ID = "z_switch"
	plan.Analyses = append(plan.Analyses, second)
	secondAssertions := append([]Assertion(nil), plan.Assertions...)
	for index := range secondAssertions {
		secondAssertions[index].AnalysisID = second.ID
	}
	plan.Assertions = append(plan.Assertions, secondAssertions...)
	first, firstDiagnostics := Evaluate(plan)
	secondReport, secondDiagnostics := Evaluate(ClonePlan(plan))
	if len(firstDiagnostics) != 0 || len(secondDiagnostics) != 0 {
		t.Fatalf("parallel transient diagnostics: first=%#v second=%#v", firstDiagnostics, secondDiagnostics)
	}
	if len(first.Analyses) != 2 || first.Analyses[0].ID != "switch" || first.Analyses[1].ID != "z_switch" {
		t.Fatalf("parallel transient result order = %#v", first.Analyses)
	}
	if !reflect.DeepEqual(first, secondReport) {
		t.Fatal("parallel transient replay is not deterministic")
	}
}

func TestNonlinearControlUpdateIgnoresCommonModeMovement(t *testing.T) {
	system := mnaSystem{nodeIndex: map[string]int{"B": 0, "C": 1, "E": 2}}
	devices := []compiledNonlinearDevice{{primitive: PrimitiveBJTNPNV1, terminals: map[string]string{"BASE": "B", "COLLECTOR": "C", "EMITTER": "E"}}}
	before := []complex128{.6, 0, 0}
	commonMode := []complex128{10.6, 10, 10}
	if update := maxNonlinearControlVoltageUpdate(devices, &system, before, commonMode); update > 1e-12 {
		t.Fatalf("common-mode nonlinear control update = %.12g", update)
	}
	changedJunction := []complex128{10.8, 10, 10}
	if update := maxNonlinearControlVoltageUpdate(devices, &system, before, changedJunction); math.Abs(update-.2) > 1e-12 {
		t.Fatalf("junction nonlinear control update = %.12g", update)
	}
}

func TestTransientPeriodicZeroCrossingUsesOperatingPointSeed(t *testing.T) {
	analysis := Analysis{Excitations: []SourceExcitation{{Component: "input", SineAmplitude: 1, SineFrequencyHz: 1000, SinePhaseDeg: 180}}}
	previous := []complex128{2, -3}
	history := [][]complex128{{.1, -.2}, previous}
	guess := transientInitialGuess(analysis, .0005, previous, history)
	if guess[0] != .1 || guess[1] != -.2 {
		t.Fatalf("zero-crossing guess=%#v", guess)
	}
	guess[0] = 99
	if history[0][0] != .1 {
		t.Fatal("initial guess aliases the accepted operating point")
	}
	ordinary := transientInitialGuess(analysis, .00025, previous, history)
	if ordinary[0] != 2 || ordinary[1] != -3 {
		t.Fatalf("ordinary guess=%#v", ordinary)
	}
}

func TestAcceptedPriorTransientBaseUsesPrecedingHistoryState(t *testing.T) {
	inductance := 1.0
	plan := Plan{
		GroundNode: "GND",
		Nodes:      []string{"GND", "LOAD"},
		Devices: []ResolvedDevice{{
			Component: "load_inductor", PrimitiveModel: PrimitiveInductorTransientV1,
			ValueSI:   &inductance,
			Terminals: []TerminalBinding{{Terminal: "A", Net: "LOAD"}, {Terminal: "B", Net: "GND"}},
		}},
	}
	analysis := Analysis{ID: "history", Kind: AnalysisTransient, DurationS: .2, TimeStepS: .1}
	template, diagnostics := buildTransientTemplate(plan, analysis)
	if len(diagnostics) != 0 {
		t.Fatalf("template diagnostics = %#v", diagnostics)
	}
	branch := template.branchIndex["load_inductor"]
	older := make([]complex128, len(template.rhs))
	accepted := make([]complex128, len(template.rhs))
	older[branch] = 2
	accepted[branch] = 7
	base, diagnostics, prepared := prepareAcceptedPriorTransientBase(
		template, plan, analysis, 2, .2, [][]complex128{older, accepted}, nil,
	)
	if !prepared || len(diagnostics) != 0 {
		t.Fatalf("prepared=%v diagnostics=%#v", prepared, diagnostics)
	}
	if got, want := real(base.rhs[branch]), -20.0; math.Abs(got-want) > 1e-12 {
		t.Fatalf("prior inductor history rhs = %.12g, want %.12g", got, want)
	}
	if _, _, prepared := prepareAcceptedPriorTransientBase(template, plan, analysis, 1, .1, [][]complex128{older}, nil); prepared {
		t.Fatal("first transient observation incorrectly claimed a reconstructable prior step")
	}
}

func TestTransientAcceptedSubstepsCloseOriginalObservationTime(t *testing.T) {
	plan := resolveTransientSwitchPlan(t, 25)
	analysis := plan.Analyses[0]
	initialAnalysis := transientDCAnalysis(analysis, 0)
	initialPlan := planWithRequestedAnalysis(plan, initialAnalysis)
	_, initial, _, diagnostic := solveNonlinearDC(initialPlan, initialAnalysis)
	if diagnostic != nil {
		t.Fatalf("initial operating point: %#v", diagnostic)
	}
	template, diagnostics := buildTransientTemplate(plan, analysis)
	if len(diagnostics) != 0 {
		t.Fatalf("template diagnostics = %#v", diagnostics)
	}
	const step = 50
	timeS := float64(step) * analysis.TimeStepS
	base := cloneMNASystem(template)
	_, _, diagnostics = prepareTransientBase(
		&base, template, plan, analysis, step, timeS,
		initial, [][]complex128{append([]complex128(nil), initial...)}, nil,
	)
	if len(diagnostics) != 0 {
		t.Fatalf("prepare diagnostics = %#v", diagnostics)
	}
	devices := compileNonlinearDevices(plan)
	predictedSystem, predictedSolution, evidence, _, predicted := solveTransientStepBySubstepPredictor(
		plan, analysis, timeS, plan.Devices, devices, initial,
		nil, nil,
	)
	if !predicted || evidence.Method != "backward_euler_bounded_accepted_substeps_v1" || evidence.TimeSteps < 2 {
		t.Fatalf("predicted=%v evidence=%#v", predicted, evidence)
	}
	predictedOut := nonlinearNodeVoltage(&predictedSystem, predictedSolution, "OUT")
	if math.IsNaN(predictedOut) || math.IsInf(predictedOut, 0) {
		t.Fatalf("accepted substep output is non-finite: %.12g", predictedOut)
	}
}

func TestTransientFuseI2TUsesAnalyticCurrentSquaredPulseEnergy(t *testing.T) {
	const (
		ratedCurrentA = 1.0
		pulseCurrentA = 2.0
		meltingI2TA2S = 0.0037
	)
	analyticTripTime := meltingI2TA2S / (pulseCurrentA * pulseCurrentA)
	previousError := math.Inf(1)
	for _, timeStep := range []float64{0.0004, 0.0002, 0.00005} {
		state := transientFuseI2TState{}
		elapsed := 0.0
		for state.integralA2S < meltingI2TA2S {
			state = advanceTransientFuseI2T(state, pulseCurrentA, ratedCurrentA, timeStep)
			elapsed += timeStep
		}
		errorS := elapsed - analyticTripTime
		if errorS < -1e-15 || errorS > timeStep+1e-15 {
			t.Fatalf("step %.12g trip time %.12g does not bound analytic %.12g", timeStep, elapsed, analyticTripTime)
		}
		if errorS > previousError+1e-15 {
			t.Fatalf("trip-time error grew under refinement: %.12g > %.12g", errorS, previousError)
		}
		previousError = errorS
	}
}

func TestTransientFuseI2TAtRatingHoldsAndBelowRatingRecovers(t *testing.T) {
	state := transientFuseI2TState{integralA2S: 0.002}
	atRating := advanceTransientFuseI2T(state, 1, 1, 10)
	if atRating != state {
		t.Fatalf("at-rated interval changed contiguous-pulse state: %#v", atRating)
	}
	belowRating := advanceTransientFuseI2T(state, 0.999, 1, 0.001)
	if belowRating != (transientFuseI2TState{}) {
		t.Fatalf("below-rated recovery did not reset state: %#v", belowRating)
	}
}

func TestTransientFuseI2TSplitStepMatchesUnsplitAndPreservesAcceptedHistory(t *testing.T) {
	accepted := map[string]transientFuseI2TState{"F1": {integralA2S: 0.001}}
	rejected := cloneTransientFuseI2TStates(accepted)
	rejected["F1"] = advanceTransientFuseI2T(rejected["F1"], 2, 1, 0.001)
	if accepted["F1"].integralA2S != 0.001 {
		t.Fatalf("rejected trajectory mutated accepted history: %#v", accepted)
	}

	unsplit := advanceTransientFuseI2T(accepted["F1"], 2, 1, 0.001)
	split := accepted["F1"]
	for range 4 {
		split = advanceTransientFuseI2T(split, 2, 1, 0.00025)
	}
	if math.Abs(split.integralA2S-unsplit.integralA2S) > 1e-15 {
		t.Fatalf("split integral %.12g differs from unsplit %.12g", split.integralA2S, unsplit.integralA2S)
	}
	if split.integralA2S <= accepted["F1"].integralA2S {
		t.Fatalf("predictor trajectory discarded pre-existing history: accepted=%#v split=%#v", accepted["F1"], split)
	}
}

func TestCommitTransientFuseStepAdvancesHistoryAndOpensClearedFuses(t *testing.T) {
	accepted := map[string]transientFuseI2TState{"F1": {integralA2S: 0.001}}
	candidate := cloneTransientFuseI2TStates(accepted)
	candidate["F1"] = advanceTransientFuseI2T(candidate["F1"], 2, 1, 0.001)
	openFuses := map[string]bool{}

	committed := commitTransientFuseStep(candidate, openFuses, []string{"F1"})
	if committed["F1"] != candidate["F1"] || !openFuses["F1"] {
		t.Fatalf("committed history/topology = %#v %#v", committed, openFuses)
	}
	if accepted["F1"].integralA2S != 0.001 {
		t.Fatalf("candidate commit mutated prior accepted history: %#v", accepted)
	}
}

func TestTransientAcceptedSubstepsApplyOnlyToBoundedDynamicOutputPlans(t *testing.T) {
	legacy := resolveTransientSwitchPlan(t, 25)
	if transientAcceptedSubstepsApplicable(legacy) {
		t.Fatal("legacy switched-load transient unexpectedly enabled accepted substeps")
	}
	for _, primitive := range []string{
		PrimitiveOpAmpV1,
		PrimitiveCurrentSenseAmplifierV1,
		PrimitiveSynchronousBuckRegulatorV1,
	} {
		bounded := legacy
		bounded.Devices = append([]ResolvedDevice(nil), legacy.Devices...)
		bounded.Devices[0].PrimitiveModel = primitive
		if !transientAcceptedSubstepsApplicable(bounded) {
			t.Fatalf("%s transient did not enable accepted substeps", primitive)
		}
	}
}

func TestTransientOutputLimitGuessInterpolatesBracketSolutions(t *testing.T) {
	base := mnaSystem{nodeIndex: map[string]int{"OUT": 0, "OTHER": 1}}
	devices := []ResolvedDevice{{Component: "amp", PrimitiveModel: PrimitiveOpAmpV1, Terminals: []TerminalBinding{{Terminal: "OUT", Net: "OUT"}}}}
	guess := make([]complex128, 2)
	next := map[string]transientOutputLimitState{"amp": {
		value: 2.5, lower: 0, upper: 10,
		lowerSolution: []complex128{0, 4}, upperSolution: []complex128{10, 8},
	}}
	seedTransientOutputLimitGuess(base, devices, guess, nil, next)
	if guess[0] != 2.5 || guess[1] != 5 {
		t.Fatalf("interpolated transient output-limit guess = %#v", guess)
	}
}

func TestTransientBranchLimitReleasesOnlyWhenControlEquationTurnsInward(t *testing.T) {
	device := ResolvedDevice{
		Component:      "limited_source",
		PrimitiveModel: PrimitiveSynchronousBuckRegulatorV1,
		ModelParameters: []NamedValue{
			{Name: "peak_current_limit_a", Value: 3.4},
		},
	}
	tests := []struct {
		name        string
		state       float64
		residual    float64
		wantRelease bool
	}{
		{name: "positive_limit_releases", state: 3.4, residual: -0.5, wantRelease: true},
		{name: "positive_limit_remains", state: 3.4, residual: 0.5},
		{name: "negative_limit_releases", state: -3.4, residual: 0.5, wantRelease: true},
		{name: "negative_limit_remains", state: -3.4, residual: -0.5},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := mnaSystem{
				matrix:      [][]complex128{{0}},
				rhs:         []complex128{complex(-test.residual, 0)},
				branchIndex: map[string]int{device.Component: 0},
			}
			_, next, changed := advanceTransientActiveLimitState(
				base,
				[]ResolvedDevice{device},
				[]complex128{0},
				nil,
				map[int]float64{0: test.state},
				nil,
				nil,
			)
			_, remainsLimited := next[0]
			if changed != test.wantRelease || remainsLimited == test.wantRelease {
				t.Fatalf("changed=%t remains_limited=%t want_release=%t", changed, remainsLimited, test.wantRelease)
			}
		})
	}
}

func TestTransientStickyBranchLimitDoesNotRepeatAnInfeasibleRelease(t *testing.T) {
	device := ResolvedDevice{
		Component:      "limited_source",
		PrimitiveModel: PrimitiveSynchronousBuckRegulatorV1,
		ModelParameters: []NamedValue{
			{Name: "peak_current_limit_a", Value: 3.4},
		},
	}
	base := mnaSystem{
		matrix:      [][]complex128{{0}},
		rhs:         []complex128{.5},
		branchIndex: map[string]int{device.Component: 0},
	}
	_, next, changed := advanceTransientActiveLimitState(
		base,
		[]ResolvedDevice{device},
		[]complex128{0},
		nil,
		map[int]float64{0: 3.4},
		nil,
		map[int]bool{0: true},
	)
	if changed || next[0] != 3.4 {
		t.Fatalf("sticky branch limit changed=%t next=%#v", changed, next)
	}
}

func TestTransientInteriorOutputContinuationReleasesForIndependentCurrentLimit(t *testing.T) {
	device := ResolvedDevice{
		Component:      "regulated_source",
		PrimitiveModel: PrimitiveSynchronousBuckRegulatorV1,
		Terminals: []TerminalBinding{
			{Terminal: "PVIN", Net: "IN"},
			{Terminal: "PGND", Net: "GND"},
			{Terminal: "SW", Net: "SW"},
		},
	}
	base := mnaSystem{
		matrix:      make([][]complex128, 3),
		rhs:         make([]complex128, 3),
		nodeIndex:   map[string]int{"IN": 0, "SW": 1},
		branchIndex: map[string]int{device.Component: 2},
	}
	for index := range base.matrix {
		base.matrix[index] = make([]complex128, 3)
	}
	outputLimits := map[string]transientOutputLimitState{
		device.Component: {value: 5, lower: 5, upper: 5},
	}
	next, _, changed := advanceTransientActiveLimitState(
		base,
		[]ResolvedDevice{device},
		[]complex128{12, 5, 0},
		outputLimits,
		nil,
		nil,
		nil,
	)
	if !changed {
		t.Fatal("interior continuation root was not released")
	}
	if _, remainsLimited := next[device.Component]; remainsLimited {
		t.Fatalf("interior continuation clamp remains active: %#v", next)
	}
}

func TestReleasedInteriorContinuationKeepsPhysicalEnvelopeEligible(t *testing.T) {
	base := mnaSystem{branchIndex: map[string]int{"regulated_source": 3}}
	outputLimits := map[string]transientOutputLimitState{
		"regulated_source": {value: 5, lower: 5, upper: 5},
	}
	deferredOutputLimits := map[string]bool{"regulated_source": true}
	deferredBranchLimits := map[int]bool{3: true}

	recordReleasedTransientActiveLimits(
		base,
		outputLimits, nil,
		nil, nil,
		deferredOutputLimits, deferredBranchLimits,
	)

	if deferredOutputLimits["regulated_source"] {
		t.Fatal("synthetic interior continuation release deferred the physical output envelope")
	}
	if deferredBranchLimits[3] {
		t.Fatal("branch limit did not become eligible after the output continuation released")
	}
}

func TestTransientCurrentLimitSupersedesOutputClampOnSharedControlEquation(t *testing.T) {
	device := ResolvedDevice{
		Component:      "regulated_source",
		PrimitiveModel: PrimitiveSynchronousBuckRegulatorV1,
		ModelParameters: []NamedValue{
			{Name: "peak_current_limit_a", Value: 3.4},
		},
		Terminals: []TerminalBinding{
			{Terminal: "PVIN", Net: "IN"},
			{Terminal: "PGND", Net: "GND"},
			{Terminal: "SW", Net: "SW"},
		},
	}
	base := mnaSystem{
		nodeIndex:   map[string]int{"IN": 0, "SW": 1},
		branchIndex: map[string]int{device.Component: 2},
	}
	outputLimits := map[string]transientOutputLimitState{
		device.Component: {side: 1, value: 12},
	}
	solution := []complex128{12, 12, 22}

	nextOutputLimits, nextBranchLimits, changed := addViolatedTransientActiveLimit(
		base,
		[]ResolvedDevice{device},
		solution,
		outputLimits,
		nil,
		nil,
		nil,
		nil,
	)
	if !changed {
		t.Fatal("expected the overcurrent state to replace the output clamp")
	}
	if _, limited := nextOutputLimits[device.Component]; limited {
		t.Fatal("output clamp remained active on the shared control equation")
	}
	if got := nextBranchLimits[2]; got != 3.4 {
		t.Fatalf("branch limit = %.12g A, want 3.4 A", got)
	}

	nextOutputLimits, nextBranchLimits, changed = addViolatedTransientActiveLimit(
		base,
		[]ResolvedDevice{device},
		[]complex128{12, 13, 3.4},
		nextOutputLimits,
		nextBranchLimits,
		nil,
		nil,
		nil,
	)
	if !changed || nextOutputLimits[device.Component].value != 12 {
		t.Fatalf("changed=%t output limits=%#v; physical high clamp did not replace infeasible current clamp", changed, nextOutputLimits)
	}
	if _, limited := nextBranchLimits[2]; limited {
		t.Fatal("infeasible current clamp remained active with the physical output clamp")
	}
}

func TestTransientPhysicalOutputEnvelopeSupersedesInfeasibleCurrentClamp(t *testing.T) {
	device := ResolvedDevice{
		Component:      "buck",
		PrimitiveModel: PrimitiveSynchronousBuckRegulatorV1,
		Terminals: []TerminalBinding{
			{Terminal: "PVIN", Net: "IN"},
			{Terminal: "PGND", Net: "GND"},
			{Terminal: "SW", Net: "SW"},
		},
	}
	base := mnaSystem{
		nodeIndex:   map[string]int{"SW": 0, "IN": 1},
		branchIndex: map[string]int{device.Component: 2},
	}
	deferredBranches := map[int]bool{}
	nextOutputs, nextBranches, changed := addViolatedTransientActiveLimit(
		base,
		[]ResolvedDevice{device},
		[]complex128{-3.4, 12, 3.5},
		nil,
		map[int]float64{2: 3.5},
		nil,
		nil,
		deferredBranches,
	)
	if !changed || nextOutputs[device.Component].value != 0 {
		t.Fatalf("changed=%t output limits=%#v; want 0 V physical clamp", changed, nextOutputs)
	}
	if _, limited := nextBranches[2]; limited || !deferredBranches[2] {
		t.Fatalf("branch limits=%#v deferred=%#v; infeasible current clamp was not deferred", nextBranches, deferredBranches)
	}
}

func TestTransientBuckLowRailDefersPeakLimitDuringFreewheel(t *testing.T) {
	device := ResolvedDevice{
		Component:      "buck",
		PrimitiveModel: PrimitiveSynchronousBuckRegulatorV1,
		ModelParameters: []NamedValue{
			{Name: "peak_current_limit_a", Value: 3.5},
		},
		Terminals: []TerminalBinding{
			{Terminal: "PVIN", Net: "IN"},
			{Terminal: "PGND", Net: "GND"},
			{Terminal: "SW", Net: "SW"},
		},
	}
	base := mnaSystem{
		nodeIndex:   map[string]int{"SW": 0, "IN": 1},
		branchIndex: map[string]int{device.Component: 2},
	}
	outputLimits := map[string]transientOutputLimitState{
		device.Component: {side: -1, value: 0},
	}
	deferredBranches := map[int]bool{}
	nextOutputs, nextBranches, changed := addViolatedTransientActiveLimit(
		base,
		[]ResolvedDevice{device},
		[]complex128{0, 12, 3.6},
		outputLimits,
		nil,
		nil,
		nil,
		deferredBranches,
	)
	if changed || nextOutputs[device.Component].value != 0 || len(nextBranches) != 0 || !deferredBranches[2] {
		t.Fatalf("changed=%t outputs=%#v branches=%#v deferred=%#v", changed, nextOutputs, nextBranches, deferredBranches)
	}
}

func TestNormallyOpenRelayIsolatesStartupAndClosesForNormalAnalyses(t *testing.T) {
	parameters := []NamedValue{
		{Name: "coil_resistance_ohm", Value: 720}, {Name: "contact_off_resistance_ohm", Value: 1e12},
		{Name: "contact_on_resistance_ohm", Value: .05}, {Name: "max_contact_current_a", Value: 5},
		{Name: "max_contact_voltage_v", Value: 30}, {Name: "operate_current_a", Value: .005}, {Name: "operate_delay_s", Value: .01},
	}
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{
			{ID: "muted", Kind: AnalysisTransient, DurationS: 10e-6, TimeStepS: 1e-6, Excitations: []SourceExcitation{{Component: "control", DCValue: 0}, {Component: "source", DCValue: 5}}},
			{ID: "normal", Kind: AnalysisTransient, DurationS: 10e-6, TimeStepS: 1e-6, Excitations: []SourceExcitation{{Component: "control", DCValue: 5}, {Component: "source", DCValue: 5}}},
			{ID: "startup", Kind: AnalysisStartup, DurationS: 100e-6, TimeStepS: 10e-6, Excitations: []SourceExcitation{{Component: "control", DCValue: 5}, {Component: "source", DCValue: 5}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "muted", Node: "OUT", Quantity: QuantityVoltageV, TimeS: 10e-6, Min: 0, Max: 1e-6},
			{AnalysisID: "normal", Node: "OUT", Quantity: QuantityVoltageV, TimeS: 10e-6, Min: 4.99, Max: 5.01},
			{AnalysisID: "startup", Node: "OUT", Quantity: QuantityPeakAbsVoltageV, Min: 0, Max: 1e-6},
		},
	}
	components := []ComponentEvidence{
		{InstanceID: "control", CatalogID: "source", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "CONTROL"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "source", CatalogID: "source", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "IN"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "relay", CatalogID: "relay", Family: "relay", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveRelayNormallyOpenV1, Parameters: parameters}}, Connections: []ConnectionEvidence{{Function: "COIL_A", Net: "CONTROL"}, {Function: "COIL_B", Net: "GND"}, {Function: "CONTACT_IN", Net: "IN"}, {Function: "CONTACT_OUT", Net: "OUT"}}},
		{InstanceID: "load", CatalogID: "resistor", Family: "resistor", HasValueSI: true, ValueSI: 1000, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "CONTROL"}, {Name: "IN"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestTransientNPNSwitchWaveformIsDeterministic(t *testing.T) {
	plan := resolveTransientSwitchPlan(t, 25)
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
	if len(report.Analyses) != 1 || len(report.Analyses[0].Points) != 301 {
		t.Fatalf("transient points=%d", len(report.Analyses[0].Points))
	}
	lastEvidence := report.Analyses[0].Points[len(report.Analyses[0].Points)-1].Solver
	if lastEvidence == nil || lastEvidence.Method != "backward_euler_bounded_newton_v1" || lastEvidence.TimeSteps != 300 || lastEvidence.TotalIterations <= 0 || lastEvidence.MaxIterationsPerStep != transientMaxNewtonIterations*transientMaxNewtonAttemptsPerObservation || lastEvidence.MaxTotalIterations != maxTransientWork {
		t.Fatalf("transient evidence=%+v", lastEvidence)
	}
	first, _ := json.Marshal(report)
	replay, replayDiagnostics := Evaluate(ClonePlan(plan))
	if len(replayDiagnostics) != 0 {
		t.Fatalf("replay diagnostics=%+v", replayDiagnostics)
	}
	second, _ := json.Marshal(replay)
	if string(first) != string(second) {
		t.Fatal("transient replay is not byte-identical")
	}
}

func TestTransientDiodeAndPNPSwitching(t *testing.T) {
	for _, test := range []struct {
		name       string
		intent     Intent
		components []ComponentEvidence
		nodes      []NodeEvidence
	}{
		{name: "diode", intent: transientDiodeIntent(), components: transientDiodeComponents(), nodes: []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "SOURCE"}, {Name: "OUT"}}},
		{name: "pnp", intent: transientPNPIntent(), components: transientPNPComponents(), nodes: transientSwitchNodes()},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan, diagnostics := ResolveWithTopology(test.intent, "test", "catalog-hash", test.components, test.nodes)
			if len(diagnostics) != 0 {
				t.Fatalf("resolve diagnostics=%+v", diagnostics)
			}
			report, diagnostics := Evaluate(plan)
			if len(diagnostics) != 0 || report.Status != "pass" {
				t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
			}
		})
	}
}

func TestTransientOpenCollectorComparatorAppliesCatalogDelay(t *testing.T) {
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "decision", Kind: AnalysisTransient, DurationS: 9e-6, TimeStepS: 1e-6,
			Excitations: []SourceExcitation{
				{Component: "signal", PulseInitialValue: 0, PulseValue: 5, PulseDelayS: 5e-6, PulseWidthS: 3e-6, PulsePeriodS: 20e-6},
				{Component: "supply", DCValue: 5},
				{Component: "threshold", DCValue: 2.5},
			},
		}},
		Assertions: []Assertion{
			{AnalysisID: "decision", Node: "OUT", Quantity: QuantityVoltageV, TimeS: 7e-6, Min: .10, Max: .12},
			{AnalysisID: "decision", Node: "OUT", Quantity: QuantityVoltageV, TimeS: 8e-6, Min: 4.99, Max: 5},
		},
	}
	components := []ComponentEvidence{
		{InstanceID: "supply", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "VP"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "threshold", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "THRESH"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "signal", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "IN"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "pullup", CatalogID: "r", Family: "resistor", HasValueSI: true, ValueSI: 10000, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "VP"}, {Function: "B", Net: "OUT"}}},
		{InstanceID: "comparator", CatalogID: "comparator", Family: "comparator", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveComparatorOpenCollectorV1, Parameters: comparatorParameters(2e-6)}}, Connections: []ConnectionEvidence{{Function: "IN_PLUS", Net: "IN"}, {Function: "IN_MINUS", Net: "THRESH"}, {Function: "OUT", Net: "OUT"}, {Function: "V_PLUS", Net: "VP"}, {Function: "V_MINUS", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "THRESH"}, {Name: "VP"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestTransientPNPResistiveSwitchDoesNotRequireCapacitiveState(t *testing.T) {
	components := transientPNPComponents()
	for index, component := range components {
		if component.InstanceID == "load" {
			components = append(components[:index], components[index+1:]...)
			break
		}
	}
	intent := transientPNPIntent()
	intent.Assertions = []Assertion{
		{AnalysisID: "switch", Node: "VCC", Quantity: QuantityVoltageV, TimeS: 0, Min: 4.99, Max: 5.01},
		{AnalysisID: "switch", Node: "DRIVE", Quantity: QuantityVoltageV, TimeS: .0002, Min: -.01, Max: .01},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, transientSwitchNodes())
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestTransientBoundsClaimsAndOperatingLimitsFailClosed(t *testing.T) {
	plan := resolveTransientSwitchPlan(t, 25)
	badGrid := transientSwitchIntent()
	badGrid.Analyses[0].DurationS = .003001
	if _, diagnostics := ResolveWithTopology(badGrid, "test", "hash", transientSwitchComponents(25), transientSwitchNodes()); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "exact integer grid") {
		t.Fatalf("grid diagnostics=%+v", diagnostics)
	}
	tooMuchWork := transientSwitchIntent()
	tooMuchWork.Analyses[0].DurationS = float64(maxTransientSteps+1) * tooMuchWork.Analyses[0].TimeStepS
	tooMuchWork.Analyses[0].Excitations[1].PulsePeriodS = tooMuchWork.Analyses[0].DurationS + tooMuchWork.Analyses[0].TimeStepS
	if _, diagnostics := ResolveWithTopology(tooMuchWork, "test", "hash", transientSwitchComponents(25), transientSwitchNodes()); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, fmt.Sprintf("at most %d steps", maxTransientSteps)) {
		t.Fatalf("work-limit diagnostics=%+v", diagnostics)
	}
	badPulse := transientSwitchIntent()
	badPulse.Analyses[0].Excitations[1].PulseWidthS = .001505
	if _, diagnostics := ResolveWithTopology(badPulse, "test", "hash", transientSwitchComponents(25), transientSwitchNodes()); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "exactly on the observation grid") {
		t.Fatalf("pulse diagnostics=%+v", diagnostics)
	}
	for index := range plan.Devices {
		if plan.Devices[index].PrimitiveModel == PrimitiveCapacitorTransientV1 {
			plan.Devices[index].ModelParameters[0].Value = 1
		}
	}
	plan.TopologyHash = topologyHash(plan.GroundNode, plan.Nodes, plan.Devices)
	plan.RegistryHash = RegistryHash()
	if _, diagnostics := Evaluate(plan); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "capacitor voltage") {
		t.Fatalf("operating-limit diagnostics=%+v", diagnostics)
	}
	components := transientSwitchComponents(25)
	for index := range components {
		if components[index].Family == "capacitor" {
			components[index].ModelClaims = []CatalogEvidence{{ModelID: PrimitiveCapacitorV1}}
		}
	}
	if _, diagnostics := ResolveWithTopology(transientSwitchIntent(), "test", "hash", components, transientSwitchNodes()); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "transient") {
		t.Fatalf("claim diagnostics=%+v", diagnostics)
	}
}

func TestNormalizeDynamicGridPreservesEventBoundaries(t *testing.T) {
	analysis := Analysis{
		ID: "extended", Kind: AnalysisTransient, DurationS: 0.5, TimeStepS: 0.0001,
		SourceValueEvents: []SourceValueEvent{
			{ID: "startup", Component: "supply", TriggerTimeS: 0, DurationS: 0.1, Initial: 0, Applied: 12},
			{ID: "shutdown", Component: "supply", TriggerTimeS: 0.4, DurationS: 0.1, Initial: 12, Applied: 0},
		},
		DeviceValueEvents: []DeviceValueEvent{
			{ID: "load", Component: "load", TriggerTimeS: 0.2, DurationS: 0.05, InitialSI: 100, AppliedSI: 10},
		},
	}
	if !NormalizeDynamicGrid(&analysis) {
		t.Fatal("expected an aligned bounded grid")
	}
	if steps := int(math.Round(analysis.DurationS / analysis.TimeStepS)); steps != 1000 {
		t.Fatalf("steps = %d, want 1000 (one hundred samples across the shortest event)", steps)
	}
	for _, value := range []float64{0.1, 0.2, 0.05, 0.25, 0.4, 0.5} {
		if !onTransientGrid(value, analysis.TimeStepS) {
			t.Fatalf("%g is not on normalized grid %g", value, analysis.TimeStepS)
		}
	}
}

func TestNormalizeDynamicGridCoarsensFastBaseStepToAlignedEventGrid(t *testing.T) {
	analysis := Analysis{
		ID: "inductive_fault", Kind: AnalysisElectrothermal,
		DurationS: 0.08, TimeStepS: 5e-8,
		DeviceValueEvents: []DeviceValueEvent{{
			ID: "short", Component: "load", TriggerTimeS: 0.07, DurationS: 0.01,
			InitialSI: 8, AppliedSI: 0.01,
		}},
	}
	if !NormalizeDynamicGrid(&analysis) {
		t.Fatal("fast base grid did not coarsen to a bounded aligned event grid")
	}
	if steps := int(math.Round(analysis.DurationS / analysis.TimeStepS)); steps != 800 {
		t.Fatalf("steps = %d, want 800 (one hundred samples across the 10 ms event)", steps)
	}
	for _, landmark := range []float64{0.07, 0.01, 0.08} {
		if !onTransientGrid(landmark, analysis.TimeStepS) {
			t.Fatalf("%g is not on normalized grid %g", landmark, analysis.TimeStepS)
		}
	}
}

func TestTransientRejectsProviderSolverAndTopologyFieldsByContract(t *testing.T) {
	intent := transientSwitchIntent()
	data, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"equation", "matrix", "solver", "integration_method", "initial_conditions", "topology", "model_file"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("provider intent unexpectedly contains %q: %s", forbidden, data)
		}
	}
}

func TestTransientReportsPointSpecificFailureAndRejectsTampering(t *testing.T) {
	components := transientSwitchComponents(25)
	components = append(components, voltageSourceEvidence("conflict", "VCC", "GND"))
	intent := transientSwitchIntent()
	intent.Analyses[0].Excitations = append(intent.Analyses[0].Excitations, SourceExcitation{Component: "conflict", DCValue: 4})
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, transientSwitchNodes())
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	_, diagnostics = Evaluate(plan)
	if len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "initial condition failed") || diagnostics[0].Suggestion == "" {
		t.Fatalf("convergence diagnostics=%+v", diagnostics)
	}

	plan = resolveTransientSwitchPlan(t, 25)
	plan.Analyses[0].TimeStepS = .000011
	if _, diagnostics = Evaluate(plan); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "exact integer grid") {
		t.Fatalf("grid tamper diagnostics=%+v", diagnostics)
	}
	plan = resolveTransientSwitchPlan(t, 25)
	for index := range plan.Devices {
		if plan.Devices[index].PrimitiveModel == PrimitiveCapacitorTransientV1 {
			plan.Devices[index].PrimitiveModel = PrimitiveCapacitorV1
			plan.Devices[index].ModelParameters = nil
		}
	}
	plan.TopologyHash = topologyHash(plan.GroundNode, plan.Nodes, plan.Devices)
	if _, diagnostics = Evaluate(plan); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "transient capacitor") {
		t.Fatalf("primitive tamper diagnostics=%+v", diagnostics)
	}
}

func resolveTransientSwitchPlan(t *testing.T, capacitorLimit float64) Plan {
	t.Helper()
	plan, diagnostics := ResolveWithTopology(transientSwitchIntent(), "test", "catalog-hash", transientSwitchComponents(capacitorLimit), transientSwitchNodes())
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	return plan
}

func transientSwitchIntent() Intent {
	return Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{ID: "switch", Kind: AnalysisTransient, DurationS: .003, TimeStepS: .00001, Excitations: []SourceExcitation{
			{Component: "supply", DCValue: 5},
			{Component: "drive", PulseInitialValue: 0, PulseValue: 5, PulseDelayS: .0005, PulseWidthS: .0015, PulsePeriodS: .004},
		}}},
		Assertions: []Assertion{
			{AnalysisID: "switch", Node: "OUT", Quantity: QuantityVoltageV, TimeS: 0, Min: 4.9, Max: 5.01},
			{AnalysisID: "switch", Node: "OUT", Quantity: QuantityVoltageV, TimeS: .001, Min: 0, Max: 1},
			{AnalysisID: "switch", Node: "OUT", Quantity: QuantityVoltageV, TimeS: .0028, Min: 4.5, Max: 5.01},
			{AnalysisID: "switch", Node: "OUT", Quantity: QuantityFallTimeS, Min: 0, Max: .0005},
			{AnalysisID: "switch", Node: "OUT", Quantity: QuantityRiseTimeS, Min: .0001, Max: .0004},
		},
	}
}

func transientSwitchComponents(capacitorLimit float64) []ComponentEvidence {
	return []ComponentEvidence{
		voltageSourceEvidence("supply", "VCC", "GND"),
		voltageSourceEvidence("drive", "DRIVE", "GND"),
		resistorEvidence("base", 10000, "DRIVE", "BASE"),
		resistorEvidence("collector", 1000, "VCC", "OUT"),
		{InstanceID: "load", CatalogID: "capacitor.ceramic.0603", Family: "capacitor", ValueSI: 100e-9, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorTransientV1, Parameters: []NamedValue{{Name: "max_voltage_v", Value: capacitorLimit}}}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
		{InstanceID: "q1", CatalogID: "bjt.onsemi.mmbt3904.sot23", Family: "bjt", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBJTNPNV1, Parameters: bjtParameters(.2, 40)}}, Connections: []ConnectionEvidence{{Function: "BASE", Net: "BASE"}, {Function: "COLLECTOR", Net: "OUT"}, {Function: "EMITTER", Net: "GND"}}},
	}
}

func transientSwitchNodes() []NodeEvidence {
	return []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VCC"}, {Name: "DRIVE"}, {Name: "BASE"}, {Name: "OUT"}}
}

func transientDiodeIntent() Intent {
	return Intent{ModelID: ModelTransientCircuitV1,
		Analyses:   []Analysis{{ID: "clamp", Kind: AnalysisTransient, DurationS: .0008, TimeStepS: .00001, Excitations: []SourceExcitation{{Component: "source", PulseValue: 5, PulseDelayS: .0001, PulseWidthS: .0005, PulsePeriodS: .001}}}},
		Assertions: []Assertion{{AnalysisID: "clamp", Node: "OUT", Quantity: QuantityVoltageV, TimeS: .0003, Min: .5, Max: .9}, {AnalysisID: "clamp", Node: "OUT", Quantity: QuantityVoltageV, TimeS: .00075, Min: 0, Max: .1}}}
}

func transientDiodeComponents() []ComponentEvidence {
	return []ComponentEvidence{
		voltageSourceEvidence("source", "SOURCE", "GND"),
		resistorEvidence("limit", 1000, "SOURCE", "OUT"),
		{InstanceID: "load", CatalogID: "capacitor.ceramic.0603", Family: "capacitor", ValueSI: 10e-9, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorTransientV1, Parameters: []NamedValue{{Name: "max_voltage_v", Value: 25}}}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
		{InstanceID: "diode", CatalogID: "diode.onsemi.1n4148w.sod_123", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}},
	}
}

func transientPNPIntent() Intent {
	return Intent{ModelID: ModelTransientCircuitV1,
		Analyses:   []Analysis{{ID: "switch", Kind: AnalysisTransient, DurationS: .0015, TimeStepS: .00001, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}, {Component: "drive", PulseInitialValue: 5, PulseValue: 0, PulseDelayS: .0002, PulseWidthS: .0007, PulsePeriodS: .002}}}},
		Assertions: []Assertion{{AnalysisID: "switch", Node: "OUT", Quantity: QuantityVoltageV, TimeS: .0001, Min: 0, Max: .1}, {AnalysisID: "switch", Node: "OUT", Quantity: QuantityVoltageV, TimeS: .0006, Min: 4, Max: 5.1}, {AnalysisID: "switch", Node: "OUT", Quantity: QuantityFallTimeS, Min: 0, Max: .0005}, {AnalysisID: "switch", Node: "OUT", Quantity: QuantityRiseTimeS, Min: 0, Max: .0005}}}
}

func transientPNPComponents() []ComponentEvidence {
	return []ComponentEvidence{
		voltageSourceEvidence("supply", "VCC", "GND"), voltageSourceEvidence("drive", "DRIVE", "GND"),
		resistorEvidence("base", 10000, "DRIVE", "BASE"), resistorEvidence("collector", 1000, "OUT", "GND"),
		{InstanceID: "load", CatalogID: "capacitor.ceramic.0603", Family: "capacitor", ValueSI: 100e-9, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorTransientV1, Parameters: []NamedValue{{Name: "max_voltage_v", Value: 25}}}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
		{InstanceID: "q1", CatalogID: "reviewed-pnp", Family: "bjt", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBJTPNPV1, Parameters: bjtParameters(.2, 40)}}, Connections: []ConnectionEvidence{{Function: "BASE", Net: "BASE"}, {Function: "COLLECTOR", Net: "OUT"}, {Function: "EMITTER", Net: "VCC"}}},
	}
}

func TestTransientPositiveFeedbackOpAmpSelectsConsistentRailState(t *testing.T) {
	parameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 1e6},
		{Name: "gain_bandwidth_hz", Value: 10e6},
		{Name: "output_high_margin_v", Value: .3},
		{Name: "output_low_margin_v", Value: .3},
		{Name: "supply_max_v", Value: 40},
		{Name: "supply_min_v", Value: 2.7},
	}
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "decision", Kind: AnalysisTransient, DurationS: 20e-6, TimeStepS: 1e-6,
			Excitations: []SourceExcitation{
				{Component: "supply", DCValue: 5},
				{Component: "reference", DCValue: 2.5},
				{Component: "signal", PulseInitialValue: 0, PulseValue: 5, PulseDelayS: 5e-6, PulseWidthS: 10e-6, PulsePeriodS: 25e-6},
			},
		}},
		Assertions: []Assertion{
			{AnalysisID: "decision", Node: "OUT", Quantity: QuantityVoltageV, TimeS: 0, Min: .29, Max: .31},
			{AnalysisID: "decision", Node: "OUT", Quantity: QuantityVoltageV, TimeS: 10e-6, Min: 4.69, Max: 4.71},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VCC", "GND"),
		voltageSourceEvidence("reference", "REF", "GND"),
		voltageSourceEvidence("signal", "IN", "GND"),
		resistorEvidence("input", 10_000, "IN", "DECISION"),
		resistorEvidence("feedback", 100_000, "OUT", "DECISION"),
		{
			InstanceID: "amplifier", CatalogID: "opamp", Family: "opamp",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: parameters}},
			Connections: []ConnectionEvidence{
				{Function: "IN_PLUS", Net: "DECISION"}, {Function: "IN_MINUS", Net: "REF"},
				{Function: "OUT", Net: "OUT"}, {Function: "V_PLUS", Net: "VCC"}, {Function: "V_MINUS", Net: "GND"},
			},
		},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, []NodeEvidence{
		{Name: "GND", Role: "ground"}, {Name: "VCC"}, {Name: "REF"}, {Name: "IN"}, {Name: "DECISION"}, {Name: "OUT"},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
	if len(report.Analyses) != 1 || len(report.Analyses[0].Points) != 21 ||
		report.Analyses[0].Points[0].Solver == nil ||
		report.Analyses[0].Points[0].Solver.Method != "bounded_newton_opamp_rail_active_set_v1" {
		t.Fatalf("positive-feedback initial evidence = %#v", report.Analyses)
	}
}

func TestOpAmpPowerTransitionIsInactiveBelowMinimumAndResumesAboveIt(t *testing.T) {
	parameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 100_000},
		{Name: "gain_bandwidth_hz", Value: 1e6},
		{Name: "output_high_margin_v", Value: .1},
		{Name: "output_low_margin_v", Value: .1},
		{Name: "supply_max_v", Value: 30},
		{Name: "supply_min_v", Value: 3},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VCC", "GND"),
		voltageSourceEvidence("reference", "REF", "GND"),
		voltageSourceEvidence("signal", "IN", "GND"),
		{
			InstanceID: "amplifier", CatalogID: "opamp", Family: "opamp",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: parameters}},
			Connections: []ConnectionEvidence{
				{Function: "IN_PLUS", Net: "IN"}, {Function: "IN_MINUS", Net: "REF"},
				{Function: "OUT", Net: "OUT"}, {Function: "V_PLUS", Net: "VCC"}, {Function: "V_MINUS", Net: "GND"},
			},
		},
	}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VCC"}, {Name: "REF"}, {Name: "IN"}, {Name: "OUT"}}
	dcIntent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{ID: "underpowered", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
			{Component: "supply", DCValue: 1}, {Component: "reference", DCValue: .5}, {Component: "signal", DCValue: .8},
		}}},
		Assertions: []Assertion{{AnalysisID: "underpowered", Node: "OUT", Quantity: QuantityVoltageV, Min: 0, Max: 1}},
	}
	dcPlan, diagnostics := ResolveWithTopology(dcIntent, "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("DC resolve diagnostics = %+v", diagnostics)
	}
	if _, diagnostics = Evaluate(dcPlan); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "outside catalog-backed range") {
		t.Fatalf("ordinary underpowered DC diagnostics = %+v", diagnostics)
	}

	transientIntent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "power_transition", Kind: AnalysisTransient, DurationS: 20e-6, TimeStepS: 1e-6,
			Excitations: []SourceExcitation{
				{Component: "supply", PulseInitialValue: 1, PulseValue: 5, PulseDelayS: 5e-6, PulseWidthS: 20e-6, PulsePeriodS: 30e-6},
				{Component: "reference", DCValue: 2}, {Component: "signal", DCValue: 3},
			},
		}},
		Assertions: []Assertion{
			{AnalysisID: "power_transition", Node: "OUT", Quantity: QuantityVoltageV, TimeS: 0, Min: 0, Max: 1e-9},
			{AnalysisID: "power_transition", Node: "OUT", Quantity: QuantityVoltageV, TimeS: 15e-6, Min: 4.89, Max: 4.91},
		},
	}
	transientPlan, diagnostics := ResolveWithTopology(transientIntent, "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("transient resolve diagnostics = %+v", diagnostics)
	}
	report, diagnostics := Evaluate(transientPlan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("power-transition report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestTransientZeroEnergyRecognizesAllZeroIndependentSources(t *testing.T) {
	plan := Plan{
		GroundNode: "GND",
		Devices: []ResolvedDevice{
			{
				Component: "supply", PrimitiveModel: PrimitiveConnectorVoltageSourceV1,
				Terminals: []TerminalBinding{{Terminal: "PIN_1", Net: "VCC"}, {Terminal: "PIN_2", Net: "GND"}},
			},
			{
				Component: "regulator", PrimitiveModel: PrimitiveFixedLinearRegulatorV1,
				Terminals: []TerminalBinding{{Terminal: "VIN", Net: "SWITCHED"}, {Terminal: "GND", Net: "GND"}},
			},
		},
	}
	analysis := Analysis{
		Kind: AnalysisTransient, TimeStepS: 1e-6,
		Excitations: []SourceExcitation{{
			Component: "supply", PulseInitialValue: 0, PulseValue: 5,
			PulseDelayS: 2e-6, PulseWidthS: 10e-6, PulsePeriodS: 20e-6,
		}},
	}
	if !transientPowerSourcesZeroAtTime(plan, analysis, -analysis.TimeStepS) {
		t.Fatal("all-zero independent-source boundary was not recognized as zero energy")
	}

	plan.Devices = append(plan.Devices, ResolvedDevice{
		Component: "control", PrimitiveModel: PrimitiveVoltageSourceV1,
		Terminals: []TerminalBinding{{Terminal: "POSITIVE", Net: "CONTROL"}, {Terminal: "NEGATIVE", Net: "GND"}},
	})
	plan.Devices[1].Terminals[0].Net = "VCC"
	analysis.Excitations = append(analysis.Excitations, SourceExcitation{Component: "control", DCValue: 3.3})
	if !transientPowerSourcesZeroAtTime(plan, analysis, -analysis.TimeStepS) {
		t.Fatal("active control source prevented zero-power initialization")
	}
	analysis.Excitations[0].DCValue = 5
	analysis.Excitations[0].PulsePeriodS = 0
	if transientPowerSourcesZeroAtTime(plan, analysis, -analysis.TimeStepS) {
		t.Fatal("energized power source was classified as zero energy")
	}
}
