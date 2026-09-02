package simulationadmission

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/simmodel"
)

type publicAdmissionFixture struct {
	name   string
	status Status
	code   DiagnosticCode
	mutate func(*Request, *Environment)
}

func TestPublicAdmissionDecisionFixtures(t *testing.T) {
	root := filepath.Join("..", "..", "examples", "simulation-admission")
	for _, fixture := range publicAdmissionFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, fixture.name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			decision, err := DecodeDecisionStrict(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if decision.Status != fixture.status ||
				(fixture.code != "" && !hasDiagnosticCode(decision.Diagnostics, fixture.code)) {
				t.Fatalf("fixture status/codes = %q/%#v", decision.Status, decision.Diagnostics)
			}
		})
	}
}

func TestGeneratePublicAdmissionDecisionFixtures(t *testing.T) {
	root := os.Getenv("KICADAI_ADMISSION_FIXTURE_OUTPUT")
	if root == "" {
		t.Skip("set KICADAI_ADMISSION_FIXTURE_OUTPUT to regenerate public artifacts")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range publicAdmissionFixtures() {
		request, environment := admissionFixture(t)
		if fixture.mutate != nil {
			fixture.mutate(&request, &environment)
		}
		decision := Admit(request, environment)
		if decision.Status != fixture.status ||
			(fixture.code != "" && !hasDiagnosticCode(decision.Diagnostics, fixture.code)) {
			t.Fatalf("generated %s status/codes = %q/%#v", fixture.name, decision.Status, decision.Diagnostics)
		}
		data, err := json.MarshalIndent(decision, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(filepath.Join(root, fixture.name+".json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func publicAdmissionFixtures() []publicAdmissionFixture {
	return []publicAdmissionFixture{
		{name: "admitted", status: StatusAdmitted},
		{name: "missing_model", status: StatusRefused, code: CodeMissingModel, mutate: func(request *Request, _ *Environment) {
			request.Components[1].ModelClaims = nil
		}},
		{name: "incompatible_model", status: StatusRefused, code: CodeIncompatibleModel, mutate: func(request *Request, _ *Environment) {
			request.Components[1].ModelClaims = append(request.Components[1].ModelClaims, request.Components[1].ModelClaims[0])
		}},
		{name: "missing_analysis_definition", status: StatusRefused, code: CodeMissingAnalysisDefinition, mutate: func(request *Request, _ *Environment) {
			request.Analyses[0].CanonicalKind = simmodel.AnalysisACSweep
		}},
		{name: "unsupported_analysis", status: StatusRefused, code: CodeUnsupportedAnalysis, mutate: func(request *Request, _ *Environment) {
			request.Analyses[0].AuthoredKind = "provider_solver"
		}},
		{name: "solver_unavailable", status: StatusRefused, code: CodeSolverUnavailable, mutate: func(_ *Request, environment *Environment) {
			environment.EnabledSolvers = []string{"kicadai_ac_mna_v1"}
		}},
		{name: "solver_model_incompatible", status: StatusRefused, code: CodeSolverModelIncompatible, mutate: func(request *Request, _ *Environment) {
			request.Components = request.Components[1:]
		}},
		{name: "invalid_model_parameters", status: StatusRefused, code: CodeInvalidModelParameters, mutate: func(request *Request, _ *Environment) {
			request.Components[1].ModelClaims[0].Parameters = []simmodel.NamedValue{{Name: "unknown_parameter", Value: 1}}
		}},
	}
}
