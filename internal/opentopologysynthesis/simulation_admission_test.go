package opentopologysynthesis

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"kicadai/internal/simulationadmission"
)

func TestCandidateSimulationAdmissionAttachesExactEvidence(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	source, err := simulationadmission.NewSource("embedded", simulationadmission.SourceBundled, environment.ModelRegistry)
	if err != nil {
		t.Fatal(err)
	}
	admission := simulationadmission.Environment{
		Sources: []simulationadmission.Source{source}, EnabledSolvers: simulationadmission.EnabledBuiltinSolverIDs(),
	}
	first := EvaluateCandidateV20(context.Background(), requirement, graph, nil, inventory, environment, admission, DefaultPolicy())
	second := EvaluateCandidateV20(context.Background(), requirement, graph, nil, inventory, environment, admission, DefaultPolicy())
	if first.Status != SimulationEvaluationPassed || len(first.Attempts) == 0 {
		t.Fatalf("simulation = status %q attempts=%d issues=%#v diagnoses=%#v", first.Status, len(first.Attempts), first.Issues, first.Diagnoses)
	}
	for _, attempt := range first.Attempts {
		if attempt.WorkflowModel == "" || len(attempt.ModelEvidenceSHA256s) < 4 {
			t.Fatalf("attempt admission = %#v", attempt)
		}
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) || first.Hash != second.Hash {
		t.Fatal("admitted simulation replay is not byte-identical")
	}
}

func TestSynthesisAdmissionRefusesDisabledSolverBeforeSearch(t *testing.T) {
	requirement, _, inventory, environment := testSimulationFixture(t)
	source, err := simulationadmission.NewSource("embedded", simulationadmission.SourceBundled, environment.ModelRegistry)
	if err != nil {
		t.Fatal(err)
	}
	admission := simulationadmission.Environment{
		Sources: []simulationadmission.Source{source}, EnabledSolvers: []string{"kicadai_dc_mna_v1"},
	}
	result := SynthesizeAdmittedV20(context.Background(), requirement, inventory, environment, admission, DefaultPolicy())
	run := result.Synthesis
	if run.Report.Status != StatusUnsupported ||
		result.Admission.Status != simulationadmission.StatusRefused ||
		run.Report.Consumption.ExpandedStates != 0 || run.Report.Consumption.GeneratedGraphs != 0 ||
		len(run.Report.Diagnostics) == 0 || string(run.Report.Diagnostics[0].Code) != string(simulationadmission.CodeSolverUnavailable) {
		t.Fatalf("early admission refusal = status %q consumption=%#v admission=%#v diagnostics=%#v", run.Report.Status, run.Report.Consumption, result.Admission, run.Report.Diagnostics)
	}
}

func TestLegacySimulationEnvironmentOmitsAdmissionEvidence(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	result := EvaluateCandidate(context.Background(), requirement, graph, nil, inventory, environment, DefaultPolicy())
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"admission"`) {
		t.Fatalf("legacy simulation acquired successor admission evidence: %s", data)
	}
}

func TestV20GraphPreparationFailureDoesNotFallBackPastAdmission(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	graph.Schema = ""
	graph.Nodes[0].ID = "not-canonical"
	source, err := simulationadmission.NewSource("embedded", simulationadmission.SourceBundled, environment.ModelRegistry)
	if err != nil {
		t.Fatal(err)
	}
	result := EvaluateCandidateV20(
		context.Background(), requirement, graph, nil, inventory, environment,
		simulationadmission.Environment{Sources: []simulationadmission.Source{source}, EnabledSolvers: simulationadmission.EnabledBuiltinSolverIDs()},
		DefaultPolicy(),
	)
	if result.Status != SimulationEvaluationUnsupported || len(result.Issues) == 0 ||
		!strings.Contains(result.Issues[0].Message, "before admission") {
		t.Fatalf("preparation failure = status %q issues %#v", result.Status, result.Issues)
	}
}

func TestV20DirectDCMetricsUseGenericElectricalQuantities(t *testing.T) {
	requirement := Requirement{}
	for _, metric := range []string{"dc_voltage", "dc_current"} {
		if _, _, supported := directSimulationQuantityForRequirementV20(
			requirement,
			BehavioralAssertion{Metric: metric, Analysis: "dc_operating_point"},
		); !supported {
			t.Fatalf("V20 direct metric %q is unsupported", metric)
		}
	}
}

func TestV20RepairEvaluationsRetainAdmissionEvidence(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	requirement.Requirements.BehavioralRequirements[0].Min = graphFloat(1000)
	requirement.Requirements.BehavioralRequirements[0].Max = graphFloat(10_000)
	graph = seriesPathOnly(t, graph)
	source, err := simulationadmission.NewSource("embedded", simulationadmission.SourceBundled, environment.ModelRegistry)
	if err != nil {
		t.Fatal(err)
	}
	admission := simulationadmission.Environment{
		Sources: []simulationadmission.Source{source}, EnabledSolvers: simulationadmission.EnabledBuiltinSolverIDs(),
	}
	policy := DefaultPolicy()
	policy.MaxTopologyRepairs = 32
	policy.MaxValueTrials = 64
	policy.MaxCandidateSimulations = 256
	policy.MaxCornerEvaluations = 1024
	initial := EvaluateCandidateV20(context.Background(), requirement, graph, nil, inventory, environment, admission, policy)
	result := RepairCandidateV20(context.Background(), requirement, graph, initial, inventory, environment, admission, policy)
	if len(result.Attempts) == 0 {
		t.Fatalf("V20 repair produced no attempts: status=%s issues=%#v", result.Status, result.Issues)
	}
	for _, repairAttempt := range result.Attempts {
		for _, attempt := range repairAttempt.Evaluation.Attempts {
			if len(attempt.ModelEvidenceSHA256s) < 4 {
				t.Fatalf("repair attempt lacks admission evidence: %#v", repairAttempt)
			}
		}
	}
}
