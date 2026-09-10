package simmodel

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func sweepPlanV23(t *testing.T, worstCase bool) Plan {
	t.Helper()
	plan := activeStatePlanV23(t, 2.4)
	plan.Analyses[0].DCSweep = &DCSweep{Component: "stimulus", StartValue: 0, StopValue: 9, Points: 51, Bidirectional: true}
	plan.Assertions = []Assertion{{AnalysisID: plan.Analyses[0].ID, Node: "OUT", Quantity: QuantityDCSweepVoltageSpanV, Min: 8.59, Max: 8.61}}
	plan.WorstCase = worstCase
	if worstCase {
		plan.Uncertainties = []Uncertainty{{Target: "devices.termination.value_si", Source: "independent-reviewed-load-tolerance", Nominal: 22000, Minimum: 20000, Maximum: 24000}}
	}
	if diagnostics := ValidatePlan(plan); len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	return plan
}

func TestSolverExecutionV23FullSweepAndWorstCase(t *testing.T) {
	for _, worstCase := range []bool{false, true} {
		plan := sweepPlanV23(t, worstCase)
		before, _ := json.Marshal(plan)
		prior, priorDiagnostics := Evaluate(plan)
		if len(priorDiagnostics) == 0 || prior.Status == "pass" {
			t.Fatal("historical sweep should reproduce the solver refusal")
		}
		var first SolverExecutionV23
		for replay := 0; replay < 2; replay++ {
			report, diagnostics, execution := EvaluateWithSolverV23(plan)
			if len(diagnostics) != 0 || report.Status != "pass" {
				t.Fatalf("worstCase=%v: status=%s diagnostics=%v", worstCase, report.Status, diagnostics)
			}
			if execution.Path != "linear_dc_sweep_clamp_recovery" || execution.MaximumRecoverySeedsPerPoint != 1 || execution.MaximumAdditionalSolvesPerPoint != 11 {
				t.Fatalf("unexpected solver selection/bounds: %#v", execution)
			}
			if err := VerifySolverExecutionV23(plan, report, diagnostics, execution); err != nil {
				t.Fatal(err)
			}
			if len(report.Analyses) != 1 || len(report.Analyses[0].Points) != 102 {
				t.Fatal("missing full bidirectional sweep")
			}
			recovered := 0
			for _, point := range report.Analyses[0].Points {
				if point.Solver == nil {
					t.Fatal("missing versioned solver identity")
				}
				if point.Solver.Method == "bounded_opamp_active_set_v23_clamp_release" {
					recovered++
				}
				want := math.Max(.2, math.Min(8.8, point.SweepValue*100000/100001))
				found := false
				for _, node := range point.Nodes {
					if node.Node == "OUT" {
						found = true
						if math.Abs(node.Real-want) > 1e-9 {
							t.Fatalf("sweep %g: output %g, want %g", point.SweepValue, node.Real, want)
						}
					}
				}
				if !found {
					t.Fatal("missing output observation")
				}
			}
			if recovered != 2 {
				t.Fatalf("got %d recovered rail exits, want 2", recovered)
			}
			if worstCase {
				if len(report.Corners) < 3 {
					t.Fatal("missing worst-case proof")
				}
				for _, corner := range report.Corners {
					if corner.Status != "pass" {
						t.Fatalf("nonpassing corner %v", corner)
					}
				}
			}
			if replay == 0 {
				first = execution
			} else if first != execution {
				t.Fatal("nondeterministic authenticated execution")
			}
		}
		after, _ := json.Marshal(plan)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("solver changed input evidence")
		}
	}
}

func TestSolverExecutionV23DeviceValueSweepEveryPoint(t *testing.T) {
	for _, existingOverride := range []bool{false, true} {
		plan := centeredBiasTestPlan(t,
			[]ComponentEvidence{voltageSourceEvidence("stimulus", "IN", "GND"), resistorEvidence("series", 3300, "IN", "OUT"), resistorEvidence("termination", 3900, "OUT", "GND")},
			[]NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}},
			[]SourceExcitation{{Component: "stimulus", DCValue: 7}},
		)
		plan.Analyses[0].DCSweep = &DCSweep{Component: "termination", DeviceValue: true, StartValue: 900, StopValue: 6900, Points: 11, Bidirectional: true}
		if existingOverride {
			value := 2700.0
			plan.Analyses[0].DeviceOverrides = []DeviceOverride{{Component: "termination", ValueSI: &value}}
		}
		plan.Assertions = []Assertion{{AnalysisID: plan.Analyses[0].ID, Node: "OUT", Quantity: QuantityDCSweepVoltageSpanV, Min: 3.2, Max: 3.3}}
		before, _ := json.Marshal(plan)
		prior, priorDiagnostics := Evaluate(plan)
		if len(priorDiagnostics) != 0 || prior.Status != "pass" {
			t.Fatalf("independent historical divider failed: %v", priorDiagnostics)
		}
		var first SolverExecutionV23
		for replay := 0; replay < 2; replay++ {
			report, diagnostics, execution := EvaluateWithSolverV23(plan)
			if len(diagnostics) != 0 || report.Status != "pass" || len(report.Analyses) != 1 || len(report.Analyses[0].Points) != 22 {
				t.Fatalf("override=%v: missing full device sweep: report=%+v diagnostics=%v", existingOverride, report, diagnostics)
			}
			if err := VerifySolverExecutionV23(plan, report, diagnostics, execution); err != nil {
				t.Fatal(err)
			}
			for i, point := range report.Analyses[0].Points {
				index, direction := i, dcSweepForward
				if i >= 11 {
					index, direction = 21-i, dcSweepReverse
				}
				resistance := 900 + float64(index)*600
				want := 7 * resistance / (3300 + resistance)
				if point.Sweep != direction || point.SweepValue != resistance || math.Abs(nodeReal(point.Nodes, "OUT")-want) > 1e-10 {
					t.Fatalf("override=%v point=%d: got %+v; want %s, R=%g, V=%g", existingOverride, i, point, direction, resistance, want)
				}
				if point.Solver == nil || point.Solver.Method != "bounded_opamp_active_set_v23" {
					t.Fatal("missing solver identity or spurious recovery")
				}
				point.Solver = nil
				if !reflect.DeepEqual(point, prior.Analyses[0].Points[i]) {
					t.Fatal("device sweep changed historical numerical evidence")
				}
			}
			if replay == 0 {
				first = execution
			} else if execution != first {
				t.Fatal("device sweep execution is not deterministic")
			}
		}
		after, _ := json.Marshal(plan)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("device sweep mutated its input plan or override")
		}
	}
}

func TestSolverExecutionV23PreservesRealFailures(t *testing.T) {
	plan := sweepPlanV23(t, true)
	plan.Assertions[0].Min = 8.9
	plan.Assertions[0].Max = 9
	report, diagnostics, execution := EvaluateWithSolverV23(plan)
	if report.Status == "pass" || len(diagnostics) == 0 {
		t.Fatal("real output span failure became a pass")
	}
	found := false
	for _, diagnostic := range diagnostics {
		found = found || diagnostic.Code == DiagnosticAssertionOutOfBounds
	}
	if !found {
		t.Fatalf("missing real bound failure: %v", diagnostics)
	}
	if err := VerifySolverExecutionV23(plan, report, diagnostics, execution); err != nil {
		t.Fatal(err)
	}
	plan.WorstCase = true
	plan.Uncertainties = nil
	report, diagnostics, _ = EvaluateWithSolverV23(plan)
	if len(diagnostics) == 0 || diagnostics[0].Path != "uncertainties" {
		t.Fatal("missing tolerance evidence accepted")
	}
}

func TestSolverExecutionV23DelegatesUnchanged(t *testing.T) {
	for _, nonlinear := range []bool{false, true} {
		plan := activeStatePlanV23(t, 2.4)
		if nonlinear {
			components := []ComponentEvidence{voltageSourceEvidence("supply", "5V", "GND"), resistorEvidence("limit", 1500, "5V", "OUT"), {InstanceID: "diode", CatalogID: "independent.diode", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}}}
			plan = resolveNonlinearTestPlan(t, components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "OUT"}}, []Assertion{{AnalysisID: "bias", Node: "OUT", Quantity: QuantityVoltageV, Min: .5, Max: .95}})
		}
		prior, priorDiagnostics := Evaluate(plan)
		report, diagnostics, execution := EvaluateWithSolverV23(plan)
		if !reflect.DeepEqual(prior, report) || !reflect.DeepEqual(priorDiagnostics, diagnostics) || execution.Path != "historical_delegate" || execution.MaximumRecoverySeedsPerPoint != 0 || execution.MaximumAdditionalSolvesPerPoint != 0 {
			t.Fatal("historical noneligible workflow changed")
		}
		if err := VerifySolverExecutionV23(plan, report, diagnostics, execution); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSolverExecutionV23TamperAndInvalidPlan(t *testing.T) {
	plan := sweepPlanV23(t, false)
	report, diagnostics, execution := EvaluateWithSolverV23(plan)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	for _, mutate := range []func(*SolverExecutionV23){
		func(e *SolverExecutionV23) { e.SolverID = "historical" }, func(e *SolverExecutionV23) { e.SolverSHA256 = "different" },
		func(e *SolverExecutionV23) { e.Path = "historical_delegate" }, func(e *SolverExecutionV23) { e.PlanSHA256 = "different" },
		func(e *SolverExecutionV23) { e.ReportSHA256 = "different" }, func(e *SolverExecutionV23) { e.DiagnosticsSHA256 = "different" },
		func(e *SolverExecutionV23) { e.MaximumRecoverySeedsPerPoint++ }, func(e *SolverExecutionV23) { e.MaximumAdditionalSolvesPerPoint++ },
	} {
		bad := execution
		mutate(&bad)
		bad.Hash = ""
		var err error
		bad.Hash, err = hashSolverValueV23(bad)
		if err != nil {
			t.Fatal(err)
		}
		if VerifySolverExecutionV23(plan, report, diagnostics, bad) == nil {
			t.Fatal("accepted rehashed solver-selection tamper")
		}
	}
	other := ClonePlan(plan)
	other.Analyses[0].DCSweep.StopValue = 8
	if VerifySolverExecutionV23(other, report, diagnostics, execution) == nil {
		t.Fatal("accepted changed requested sweep")
	}
	changed := CloneReport(report)
	changed.CatalogHash = "different"
	forged, err := solverExecutionV23(plan, changed, diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	if VerifySolverExecutionV23(plan, changed, diagnostics, forged) == nil {
		t.Fatal("accepted report from another catalog")
	}
	plan.RegistryHash = "wrong"
	_, invalidDiagnostics, _ := EvaluateWithSolverV23(plan)
	if len(invalidDiagnostics) == 0 {
		t.Fatal("invalid registry accepted")
	}
	plan.Analyses[0].DCSweep.StopValue = math.Inf(1)
	invalidReport, invalidDiagnostics, invalidExecution := EvaluateWithSolverV23(plan)
	if invalidReport.Status == "pass" || len(invalidDiagnostics) == 0 || invalidExecution.Hash != "" {
		t.Fatal("unauthenticatable plan accepted")
	}
}

func TestSolverExecutionV23RejectsRehashedInvalidModelParameters(t *testing.T) {
	plan := sweepPlanV23(t, false)
	for i := range plan.Devices {
		if plan.Devices[i].Component != "amplifier" {
			continue
		}
		for j := range plan.Devices[i].ModelParameters {
			if plan.Devices[i].ModelParameters[j].Name == "dc_open_loop_gain" {
				plan.Devices[i].ModelParameters[j].Value = 0
			}
		}
	}
	RefreshTopologyHash(&plan)
	report, diagnostics, execution := EvaluateWithSolverV23(plan)
	if report.Status == "pass" || len(diagnostics) == 0 {
		t.Fatal("invalid finite model parameter accepted")
	}
	if VerifySolverExecutionV23(plan, report, diagnostics, execution) == nil {
		t.Fatal("invalid model execution verified")
	}
	for _, analysis := range report.Analyses {
		if len(analysis.Points) != 0 {
			t.Fatal("numerical work preceded model validation")
		}
	}
}
