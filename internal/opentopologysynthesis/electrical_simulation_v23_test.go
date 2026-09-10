package opentopologysynthesis

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/simmodel"
	"kicadai/internal/simulationadmission"
)

// This extends the hand-authored 12 V / 12.5 kohm regression, not a corpus case.
// The sweep explicitly permits rail saturation and asks for its full-span slope.
func testElectricalFixtureV23(t *testing.T, sweep bool) (Requirement, CandidateGraph, CandidateGraph, GraphChange, PrimitiveInventory, SimulationEnvironment, simulationadmission.Environment) {
	t.Helper()
	r, before, inventory, environment, admission := testControlBindingFixtureV22(t)
	if sweep {
		r.Project.Name = "independent_saturation_release"
		r.Project.Description = "A twelve-volt monitor traverses both saturation rails and returns through its linear range with a bounded full-span transfer slope."
		for i := range r.Requirements.Ports {
			if r.Requirements.Ports[i].ID == "stimulus" {
				r.Requirements.Ports[i].Electrical.MinVoltageV = graphFloat(0)
				r.Requirements.Ports[i].Electrical.NominalVoltageV = graphFloat(6)
				r.Requirements.Ports[i].Electrical.MaxVoltageV = graphFloat(12)
			}
			if r.Requirements.Ports[i].ID == "reading" {
				r.Requirements.Ports[i].Electrical.MinVoltageV = graphFloat(0)
				r.Requirements.Ports[i].Electrical.NominalVoltageV = graphFloat(6)
				r.Requirements.Ports[i].Electrical.MaxVoltageV = graphFloat(12)
			}
		}
		for i := range r.Requirements.OperatingCases[0].Conditions {
			if r.Requirements.OperatingCases[0].Conditions[i].Axis == "input_voltage" {
				r.Requirements.OperatingCases[0].Conditions[i].Min = 0
				r.Requirements.OperatingCases[0].Conditions[i].Max = 12
			}
		}
		r.Requirements.BehavioralRequirements = []BehavioralAssertion{{ID: "span", Metric: "voltage_gain", Analysis: "dc_sweep", Excitation: &Observation{Kind: "port", ID: "stimulus"}, Observation: Observation{Kind: "port", ID: "reading"}, Min: graphFloat(.949), Max: graphFloat(.951), Unit: "ratio", OperatingCases: []string{"steady"}}}
		r = Normalize(r)
	}
	if issues := Validate(r); len(issues) != 0 {
		t.Fatalf("independent requirement invalid: %+v", issues)
	}
	after := CloneGraph(before)
	var change GraphChange
	for i := range after.Instances {
		for j, terminal := range after.Instances[i].Terminals {
			if terminal.Terminal == "IN_MINUS" {
				change = GraphChange{Kind: "redirect_terminal", Primitive: after.Instances[i].ID, Terminal: terminal.Terminal, FromNode: terminal.Node, ToNode: "port_reading"}
				after.Instances[i].Terminals[j].Node = change.ToNode
			}
		}
	}
	after, _ = NormalizeGraph(after)
	return r, before, after, change, inventory, environment, admission
}

func TestElectricalV23ExecutionAndCertificate(t *testing.T) {
	for _, sweep := range []bool{false, true} {
		r, before, graph, change, inventory, environment, admission := testElectricalFixtureV23(t, sweep)
		ctx := context.Background()
		inputHash := causalCrossStageHash(struct {
			R Requirement
			G CandidateGraph
		}{r, graph})
		old := EvaluateElectricalCandidateV22(ctx, r, graph, inventory, environment, admission, DefaultPolicy())
		var first ElectricalEvaluationV23
		for replay := 0; replay < 2; replay++ {
			result := EvaluateElectricalCandidateV23(ctx, r, graph, inventory, environment, admission, DefaultPolicy())
			if result.Evaluation.Status != SimulationEvaluationPassed || len(result.SolverAttempts) == 0 {
				t.Fatalf("sweep=%v evaluation failed: %+v", sweep, result.Evaluation)
			}
			if err := VerifyElectricalEvaluationV23(ctx, result, r, graph, inventory, environment, admission); err != nil {
				t.Fatalf("sweep=%v verification: %v", sweep, err)
			}
			if !sweep && !reflect.DeepEqual(old, result.Evaluation) {
				t.Fatal("non-sweep historical evaluation changed")
			}
			if sweep {
				if old.Status == SimulationEvaluationPassed {
					t.Fatal("independent sweep no longer reproduces historical refusal")
				}
				released := false
				for _, attempt := range result.Evaluation.Attempts {
					for _, analysis := range attempt.Report.Analyses {
						for _, point := range analysis.Points {
							released = released || point.Solver != nil && point.Solver.Method == "bounded_opamp_active_set_v23_clamp_release"
						}
					}
				}
				if !released {
					t.Fatal("independent sweep did not exercise clamp release")
				}
			}
			certificate, err := certifyElectricalPathV23(ctx, r, before, graph, inventory, environment, simulationadmission.PrepareEnvironment(admission), []GraphChange{change}, result)
			if err != nil {
				t.Fatalf("sweep=%v certificate: %v", sweep, err)
			}
			if err := verifyElectricalCertificateV23(ctx, certificate, r, before, graph, inventory, environment, simulationadmission.PrepareEnvironment(admission), result); err != nil {
				t.Fatal(err)
			}
			if replay == 0 {
				first = result
			} else if !reflect.DeepEqual(first, result) {
				t.Fatal("V23 generation evidence is nondeterministic")
			}
		}
		if inputHash != causalCrossStageHash(struct {
			R Requirement
			G CandidateGraph
		}{r, graph}) {
			t.Fatal("V23 generation modified input evidence")
		}
	}
}

func TestElectricalV23RejectsRehashedExecutionTampering(t *testing.T) {
	r, before, graph, change, inventory, environment, admission := testElectricalFixtureV23(t, false)
	ctx := context.Background()
	result := EvaluateElectricalCandidateV23(ctx, r, graph, inventory, environment, admission, DefaultPolicy())
	encoded, err := json.Marshal(result)
	if err != nil || len(result.SolverAttempts) < 2 {
		t.Fatal("independent multi-analysis fixture unavailable")
	}
	for name, mutate := range map[string]func(*ElectricalEvaluationV23){
		"policy ID":           func(e *ElectricalEvaluationV23) { e.SolverPolicyID = "historical" },
		"policy hash":         func(e *ElectricalEvaluationV23) { e.SolverPolicyHash = "changed" },
		"missing execution":   func(e *ElectricalEvaluationV23) { e.SolverAttempts = e.SolverAttempts[1:] },
		"duplicate execution": func(e *ElectricalEvaluationV23) { e.SolverAttempts = append(e.SolverAttempts, e.SolverAttempts[0]) },
		"reordered execution": func(e *ElectricalEvaluationV23) { slices.Reverse(e.SolverAttempts) },
		"wrong corner":        func(e *ElectricalEvaluationV23) { e.SolverAttempts[0].CornerID = "substituted" },
		"wrong solver":        func(e *ElectricalEvaluationV23) { e.SolverAttempts[0].Execution.SolverID = "substituted" },
		"wrong bound":         func(e *ElectricalEvaluationV23) { e.SolverAttempts[0].Execution.MaximumRecoverySeedsPerPoint++ },
		"hidden diagnostic": func(e *ElectricalEvaluationV23) {
			e.SolverAttempts[0].Diagnostics = []simmodel.Diagnostic{{Path: "model", Message: "unresolved"}}
		},
		"missing report":     func(e *ElectricalEvaluationV23) { e.Evaluation.Attempts[0].Report = nil },
		"missing provenance": func(e *ElectricalEvaluationV23) { e.Evaluation.Attempts[0].ModelEvidenceSHA256s = nil },
		"false plan":         func(e *ElectricalEvaluationV23) { e.Evaluation.Attempts[0].PlanHash = "substituted" },
		"missing complete corner": func(e *ElectricalEvaluationV23) {
			e.SolverAttempts = e.SolverAttempts[:1]
			e.Evaluation.Attempts = e.Evaluation.Attempts[:1]
			e.Evaluation.Consumption.CandidateSimulations = 1
			e.Evaluation.Consumption.CornerEvaluations = 1 + len(e.Evaluation.Attempts[0].Report.Corners)
		},
		"rehashed internal corner omission": func(e *ElectricalEvaluationV23) {
			a := &e.Evaluation.Attempts[0]
			a.Report.Corners = a.Report.Corners[:len(a.Report.Corners)-1]
			a.ReportHash = causalCrossStageHash(*a.Report)
			e.SolverAttempts[0].Execution.ReportSHA256 = a.ReportHash
			e.Evaluation.Consumption.CornerEvaluations--
		},
		"changed bound":      func(e *ElectricalEvaluationV23) { e.Evaluation.Attempts[0].RequiredMin = graphFloat(-1) },
		"changed accounting": func(e *ElectricalEvaluationV23) { e.Evaluation.Consumption.CandidateSimulations-- },
		"false success":      func(e *ElectricalEvaluationV23) { e.Evaluation.Attempts[0].AssertionPass = false },
		"duplicate attempt": func(e *ElectricalEvaluationV23) {
			e.Evaluation.Attempts = append(e.Evaluation.Attempts, e.Evaluation.Attempts[0])
		},
	} {
		t.Run(name, func(t *testing.T) {
			var altered ElectricalEvaluationV23
			if err := json.Unmarshal(encoded, &altered); err != nil {
				t.Fatal(err)
			}
			mutate(&altered)
			for i := range altered.SolverAttempts {
				e := &altered.SolverAttempts[i].Execution
				e.Hash = ""
				e.Hash = causalCrossStageHash(*e)
			}
			altered.Evaluation = finalizeSimulationEvaluation(altered.Evaluation)
			altered = finalizeElectricalEvaluationV23(altered)
			if VerifyElectricalEvaluationV23(ctx, altered, r, graph, inventory, environment, admission) == nil {
				t.Fatal("rehashed substitution verified")
			}
			if _, err := certifyElectricalPathV23(ctx, r, before, graph, inventory, environment, simulationadmission.PrepareEnvironment(admission), []GraphChange{change}, altered); err == nil {
				t.Fatal("rehashed substitution certified")
			}
		})
	}
}

func TestElectricalV23PreservesAdmissionBudgetsAndRealFailures(t *testing.T) {
	r, before, graph, change, inventory, environment, admission := testElectricalFixtureV23(t, true)
	ctx := context.Background()
	missing := admission
	missing.EnabledSolvers = nil
	refused := EvaluateElectricalCandidateV23(ctx, r, graph, inventory, environment, missing, DefaultPolicy())
	if refused.Evaluation.Status == SimulationEvaluationPassed || len(refused.SolverAttempts) != 0 || refused.Evaluation.Consumption.CandidateSimulations != 0 {
		t.Fatal("numerical execution bypassed unavailable backend admission")
	}
	policy := DefaultPolicy()
	policy.MaxCornerEvaluations = 1
	exhausted := EvaluateElectricalCandidateV23(ctx, r, graph, inventory, environment, admission, policy)
	if exhausted.Evaluation.Status != SimulationEvaluationExhausted || len(exhausted.SolverAttempts) != 0 {
		t.Fatal("atomic corner budget was bypassed")
	}
	r.Requirements.BehavioralRequirements[0].Min = graphFloat(.98)
	r.Requirements.BehavioralRequirements[0].Max = graphFloat(.99)
	failed := EvaluateElectricalCandidateV23(ctx, r, graph, inventory, environment, admission, DefaultPolicy())
	if failed.Evaluation.Status != SimulationEvaluationFailed || len(failed.SolverAttempts) == 0 || len(failed.SolverAttempts[0].Diagnostics) == 0 {
		t.Fatal("real electrical bound failure was suppressed")
	}
	if err := VerifyElectricalEvaluationV23(ctx, failed, r, graph, inventory, environment, admission); err != nil {
		t.Fatal(err)
	}
	if _, err := certifyElectricalPathV23(ctx, r, before, graph, inventory, environment, simulationadmission.PrepareEnvironment(admission), []GraphChange{change}, failed); err == nil {
		t.Fatal("real electrical failure certified")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	stopped := EvaluateElectricalCandidateV23(canceled, r, graph, inventory, environment, admission, DefaultPolicy())
	if stopped.Evaluation.Status != SimulationEvaluationCanceled || len(stopped.SolverAttempts) != 0 {
		t.Fatal("cancellation ignored")
	}
}

func TestElectricalV23OneSidedBoundsKeepDiagnosticDirection(t *testing.T) {
	for _, test := range []struct {
		name             string
		minimum, maximum *float64
		code             string
	}{
		{"maximum only pass", nil, graphFloat(.96), ""},
		{"minimum only pass", graphFloat(.94), nil, ""},
		{"maximum only refusal", nil, graphFloat(.94), diagnosisAssertionAboveMaximum},
		{"minimum only refusal", graphFloat(.96), nil, diagnosisAssertionBelowMinimum},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, _, graph, _, inventory, environment, admission := testElectricalFixtureV23(t, true)
			r.Requirements.BehavioralRequirements[0].Min = test.minimum
			r.Requirements.BehavioralRequirements[0].Max = test.maximum
			result := EvaluateElectricalCandidateV23(context.Background(), r, graph, inventory, environment, admission, DefaultPolicy())
			if test.code == "" {
				if result.Evaluation.Status != SimulationEvaluationPassed || len(result.Evaluation.Diagnoses) != 0 {
					t.Fatalf("satisfied one-sided bound refused: %+v", result.Evaluation.Diagnoses)
				}
			} else if result.Evaluation.Status != SimulationEvaluationFailed || len(result.Evaluation.Diagnoses) != 1 || result.Evaluation.Diagnoses[0].Code != test.code {
				t.Fatalf("incorrect one-sided refusal: %+v", result.Evaluation.Diagnoses)
			}
			if err := VerifyElectricalEvaluationV23(context.Background(), result, r, graph, inventory, environment, admission); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestElectricalV23MetricTranslationPreservesNormalizedIDs(t *testing.T) {
	r, _, graph, _, inventory, environment, admission := testElectricalFixtureV23(t, false)
	for i := range r.Requirements.BehavioralRequirements {
		if r.Requirements.BehavioralRequirements[i].Metric == "output_voltage" {
			r.Requirements.BehavioralRequirements[i].Metric = "dc_voltage"
			r.Requirements.BehavioralRequirements[i].ID = "  reading  "
		}
	}
	slices.Reverse(r.Requirements.BehavioralRequirements)
	ctx := context.Background()
	result := EvaluateElectricalCandidateV23(ctx, r, graph, inventory, environment, admission, DefaultPolicy())
	if result.Evaluation.Status != SimulationEvaluationPassed {
		t.Fatalf("metric translation refused: %+v", result.Evaluation.Diagnoses)
	}
	canonical := Normalize(r)
	replay := EvaluateElectricalCandidateV23(ctx, canonical, graph, inventory, environment, admission, DefaultPolicy())
	if !reflect.DeepEqual(result, replay) {
		t.Fatal("normalization changed exact execution evidence")
	}
	found := false
	for _, attempt := range result.Evaluation.Attempts {
		if attempt.RequirementID == "reading" {
			found = true
			if attempt.Metric != "dc_voltage" {
				t.Fatal("original metric identity was lost")
			}
		}
	}
	if !found {
		t.Fatal("normalized assertion ID was lost")
	}
	if err := VerifyElectricalEvaluationV23(ctx, result, r, graph, inventory, environment, admission); err != nil {
		t.Fatal(err)
	}
}
