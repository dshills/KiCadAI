package simulationadmission

import (
	"bytes"
	"encoding/json"
	"math"
	"slices"
	"sync"
	"testing"

	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

func TestAdmitDeterministicallySelectsExactModelsAndSolver(t *testing.T) {
	request, environment := admissionFixture(t)
	first := Admit(request, environment)
	if first.Status != StatusAdmitted || len(first.Diagnostics) != 0 || len(first.Analyses) != 1 || len(first.Models) != 2 {
		t.Fatalf("admission = status %q analyses=%d models=%d diagnostics=%#v", first.Status, len(first.Analyses), len(first.Models), first.Diagnostics)
	}
	if first.Analyses[0].SolverID != "kicadai_dc_mna_v1" || first.Analyses[0].WorkflowModelID != simmodel.ModelLinearCircuitMNAV1 {
		t.Fatalf("analysis decision = %#v", first.Analyses[0])
	}
	for _, model := range first.Models {
		if model.RegistrySourceID != "embedded" || model.RegistrySourceKind != SourceBundled ||
			!validSHA256(model.RegistrySourceSHA256) || !validSHA256(model.RegistryRecordSHA256) ||
			!validSHA256(model.ParametersSHA256) || model.CompatibilityStatus != StatusAdmitted {
			t.Fatalf("model decision = %#v", model)
		}
	}

	reversed := request
	reversed.Analyses = slices.Clone(request.Analyses)
	slices.Reverse(reversed.Analyses)
	reversed.Components = cloneComponents(request.Components)
	slices.Reverse(reversed.Components)
	for index := range reversed.Components {
		slices.Reverse(reversed.Components[index].Connections)
		slices.Reverse(reversed.Components[index].ModelClaims)
	}
	reorderedEnvironment := environment
	reorderedEnvironment.EnabledSolvers = slices.Clone(environment.EnabledSolvers)
	slices.Reverse(reorderedEnvironment.EnabledSolvers)
	second := Admit(reversed, reorderedEnvironment)
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) || first.Hash != second.Hash {
		t.Fatalf("order changed admission\nfirst:  %s\nsecond: %s", firstJSON, secondJSON)
	}
}

func TestAdmitProducesEveryTypedRefusalCategory(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Request, *Environment)
		code   DiagnosticCode
	}{
		{name: "missing model", code: CodeMissingModel, mutate: func(request *Request, _ *Environment) {
			request.Components[1].ModelClaims = nil
		}},
		{name: "incompatible model", code: CodeIncompatibleModel, mutate: func(request *Request, _ *Environment) {
			request.Components[1].ModelClaims = append(request.Components[1].ModelClaims, request.Components[1].ModelClaims[0])
		}},
		{name: "missing analysis definition", code: CodeMissingAnalysisDefinition, mutate: func(request *Request, _ *Environment) {
			request.Analyses[0].CanonicalKind = simmodel.AnalysisACSweep
		}},
		{name: "unsupported analysis", code: CodeUnsupportedAnalysis, mutate: func(request *Request, _ *Environment) {
			request.Analyses[0].AuthoredKind = "provider_solver"
		}},
		{name: "solver unavailable", code: CodeSolverUnavailable, mutate: func(_ *Request, environment *Environment) {
			environment.EnabledSolvers = []string{"kicadai_ac_mna_v1"}
		}},
		{name: "solver model incompatible", code: CodeSolverModelIncompatible, mutate: func(request *Request, _ *Environment) {
			request.Components = request.Components[1:]
		}},
		{name: "invalid model parameters", code: CodeInvalidModelParameters, mutate: func(request *Request, _ *Environment) {
			request.Components[1].ModelClaims[0].Parameters = []simmodel.NamedValue{{Name: "unknown_parameter", Value: 1}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, environment := admissionFixture(t)
			test.mutate(&request, &environment)
			decision := Admit(request, environment)
			if decision.Status != StatusRefused || !hasDiagnosticCode(decision.Diagnostics, test.code) {
				t.Fatalf("decision = status %q diagnostics %#v", decision.Status, decision.Diagnostics)
			}
		})
	}
}

func TestAdmitRejectsNonFiniteModelScalarsWithoutPanicking(t *testing.T) {
	for name, mutate := range map[string]func(*Request){
		"component value": func(request *Request) { request.Components[1].ValueSI = math.NaN() },
		"model parameter": func(request *Request) {
			request.Components[1].ModelClaims[0].Parameters = []simmodel.NamedValue{{Name: "resistance_ohm", Value: math.Inf(1)}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			request, environment := admissionFixture(t)
			mutate(&request)
			first := Admit(request, environment)
			second := Admit(request, environment)
			if first.Status != StatusRefused || !hasDiagnosticCode(first.Diagnostics, CodeInvalidModelParameters) ||
				first.Hash != second.Hash || !validSHA256(first.Hash) {
				t.Fatalf("non-finite refusal = %#v", first)
			}
		})
	}
}

func TestPreparedEnvironmentIsDeterministicAndConcurrentReadSafe(t *testing.T) {
	request, environment := admissionFixture(t)
	prepared := PrepareEnvironment(environment)
	want := Admit(request, environment)
	if got := AdmitPrepared(request, prepared); got.Hash != want.Hash {
		t.Fatalf("prepared admission hash = %s, want %s", got.Hash, want.Hash)
	}
	const workers = 16
	hashes := make(chan string, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			hashes <- AdmitPrepared(request, prepared).Hash
		}()
	}
	group.Wait()
	close(hashes)
	for hash := range hashes {
		if hash != want.Hash {
			t.Fatalf("concurrent prepared admission hash = %s, want %s", hash, want.Hash)
		}
	}
}

func TestAdmitRejectsSourceTamperingAndConflicts(t *testing.T) {
	request, environment := admissionFixture(t)
	environment.Sources[0].SHA256 = admissionTestHash("tampered")
	decision := Admit(request, environment)
	if decision.Status != StatusRefused || !hasDiagnosticCode(decision.Diagnostics, CodeIncompatibleModel) {
		t.Fatalf("tampered source decision = %#v", decision)
	}

	request, environment = admissionFixture(t)
	conflict := environment.Sources[0]
	conflict.ID = "project-overlay"
	conflict.Kind = SourceProject
	for index := range conflict.Registry.Records {
		if conflict.Registry.Records[index].CatalogID == "resistor.generic.0603" &&
			conflict.Registry.Records[index].ModelID == simmodel.PrimitiveResistorV1 {
			conflict.Registry.Records[index].Provenance.Source = "reviewed-project-source"
			break
		}
	}
	var err error
	conflict, err = NewSource(conflict.ID, conflict.Kind, conflict.Registry)
	if err != nil {
		t.Fatal(err)
	}
	environment.Sources = append(environment.Sources, conflict)
	decision = Admit(request, environment)
	if decision.Status != StatusRefused || !hasDiagnosticCode(decision.Diagnostics, CodeIncompatibleModel) {
		t.Fatalf("conflicting source decision = %#v", decision)
	}
}

func TestAdmitUsesAuthenticatedProjectAndConfiguredSources(t *testing.T) {
	for _, kind := range []SourceKind{SourceProject, SourceConfigured} {
		t.Run(string(kind), func(t *testing.T) {
			request, environment := admissionFixture(t)
			source, err := NewSource(string(kind)+"-registry", kind, environment.Sources[0].Registry)
			if err != nil {
				t.Fatal(err)
			}
			environment.Sources = []Source{source}
			decision := Admit(request, environment)
			if decision.Status != StatusAdmitted || len(decision.Models) == 0 {
				t.Fatalf("%s source admission = %#v", kind, decision)
			}
			for _, model := range decision.Models {
				if model.RegistrySourceID != source.ID || model.RegistrySourceKind != kind ||
					model.RegistrySourceSHA256 != source.SHA256 {
					t.Fatalf("%s source provenance = %#v", kind, model)
				}
			}
		})
	}
}

func TestSplitMergedSourcesPreservesExactOriginAndRejectsRewrite(t *testing.T) {
	_, environment := admissionFixture(t)
	base := environment.Sources[0].Registry
	merged := modelprovenance.Normalize(base)
	added := merged.Records[0]
	added.CatalogID = "resistor.reviewed.project"
	added.Provenance.Source = "reviewed-project-model"
	merged.Records = append(merged.Records, added)
	sources, err := SplitMergedSources(
		"embedded", SourceBundled, base,
		"project", SourceProject, merged,
	)
	if err != nil || len(sources) != 2 {
		t.Fatalf("split sources = %#v, %v", sources, err)
	}
	if sources[0].ID != "embedded" || sources[1].ID != "project" ||
		sources[0].Kind != SourceBundled || sources[1].Kind != SourceProject ||
		len(sources[1].Registry.Records) != 1 || sources[1].Registry.Records[0].CatalogID != added.CatalogID {
		t.Fatalf("split source provenance = %#v", sources)
	}

	rewritten := modelprovenance.Normalize(merged)
	rewritten.Records[0].Provenance.Source = "silent-rewrite"
	if _, err := SplitMergedSources("embedded", SourceBundled, base, "configured", SourceConfigured, rewritten); err == nil {
		t.Fatal("trusted base rewrite was accepted")
	}
}

func TestAdmitRequirementPreflightUsesInventoryAndCanonicalDCSweep(t *testing.T) {
	_, environment := admissionFixture(t)
	request := Request{Analyses: []AnalysisRequirement{{ID: "sweep", AuthoredKind: "dc_sweep", Assertions: []string{"gain"}, OperatingCases: []string{"nominal"}, DCSweep: true}}, InventoryModels: []CatalogModel{{
		CatalogID: "resistor.generic.0603", Family: "resistor", ModelID: simmodel.PrimitiveResistorV1,
	}}}
	decision := Admit(request, environment)
	if decision.Status != StatusAdmitted || len(decision.Analyses) != 1 ||
		decision.Analyses[0].CanonicalKind != simmodel.AnalysisDCOperatingPoint || decision.Analyses[0].SolverID != "kicadai_dc_mna_v1" {
		t.Fatalf("DC sweep preflight = %#v", decision)
	}

	request.InventoryModels = nil
	decision = Admit(request, environment)
	if decision.Status != StatusRefused || !hasDiagnosticCode(decision.Diagnostics, CodeMissingModel) {
		t.Fatalf("empty inventory preflight = %#v", decision)
	}
}

func TestAdmitDoesNotAssumeSolverAvailability(t *testing.T) {
	request, environment := admissionFixture(t)
	environment.EnabledSolvers = nil
	decision := Admit(request, environment)
	if decision.Status != StatusRefused || !hasDiagnosticCode(decision.Diagnostics, CodeSolverUnavailable) {
		t.Fatalf("empty solver availability = %#v", decision)
	}
}

func TestDecisionStrictDecodeAndTamperValidation(t *testing.T) {
	request, environment := admissionFixture(t)
	decision := Admit(request, environment)
	data, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeDecisionStrict(bytes.NewReader(data))
	if err != nil || decoded.Hash != decision.Hash {
		t.Fatalf("strict decode = hash %q error %v", decoded.Hash, err)
	}

	unknown := append([]byte(nil), data[:len(data)-1]...)
	unknown = append(unknown, []byte(`,"provider_model":"unsafe"}`)...)
	if _, err := DecodeDecisionStrict(bytes.NewReader(unknown)); err == nil {
		t.Fatal("unknown admission field was accepted")
	}

	tampered := CloneDecision(decision)
	modelIndex := slices.IndexFunc(tampered.Models, func(model ModelDecision) bool { return model.ValueSI != nil })
	if modelIndex < 0 {
		t.Fatal("fixture has no value-bearing model decision")
	}
	value := *tampered.Models[modelIndex].ValueSI
	value *= 2
	tampered.Models[modelIndex].ValueSI = &value
	if diagnostics := ValidateDecision(tampered); !hasDiagnosticCode(diagnostics, CodeInvalidModelParameters) || !hasDiagnosticCode(diagnostics, CodeIncompatibleModel) {
		t.Fatalf("tamper diagnostics = %#v", diagnostics)
	}

	tampered = CloneDecision(decision)
	tampered.Models[0].ModelClaim.ModelID = simmodel.PrimitiveCapacitorV1
	if diagnostics := ValidateDecision(tampered); !hasDiagnosticCode(diagnostics, CodeIncompatibleModel) {
		t.Fatalf("model-claim tamper diagnostics = %#v", diagnostics)
	}

	tampered = CloneDecision(decision)
	tampered.Models[0].Provenance.Revision = "tampered"
	if diagnostics := ValidateDecision(tampered); !hasDiagnosticCode(diagnostics, CodeIncompatibleModel) {
		t.Fatalf("provenance-record tamper diagnostics = %#v", diagnostics)
	}

	tampered = CloneDecision(decision)
	tampered.Analyses[0].SolverSHA256 = admissionTestHash("substituted-solver")
	if diagnostics := ValidateDecision(tampered); !hasDiagnosticCode(diagnostics, CodeSolverUnavailable) {
		t.Fatalf("solver tamper diagnostics = %#v", diagnostics)
	}
}

func TestCloneDecisionDoesNotAliasNestedProvenance(t *testing.T) {
	request, environment := admissionFixture(t)
	decision := Admit(request, environment)
	clone := CloneDecision(decision)
	clone.Models[0].Provenance.AllowedAnalyses[0] = "tampered"
	*clone.Models[0].Provenance.MinTemperatureC = -999
	if decision.Models[0].Provenance.AllowedAnalyses[0] == "tampered" ||
		*decision.Models[0].Provenance.MinTemperatureC == -999 {
		t.Fatal("cloned admission decision aliases nested provenance")
	}
}

func admissionFixture(t *testing.T) (Request, Environment) {
	t.Helper()
	registry, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatalf("load model registry: %#v", diagnostics)
	}
	source, err := NewSource("embedded", SourceBundled, registry)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{
		Analyses: []AnalysisRequirement{{ID: "bias", AuthoredKind: simmodel.AnalysisDCOperatingPoint, Assertions: []string{"output"}, OperatingCases: []string{"nominal"}}},
		InventoryModels: []CatalogModel{
			{CatalogID: "source.voltage.connector.1x02", Family: "voltage_source", ModelID: simmodel.PrimitiveVoltageSourceV1},
			{CatalogID: "resistor.generic.0603", Family: "resistor", ModelID: simmodel.PrimitiveResistorV1},
		},
		Components: []simmodel.ComponentEvidence{
			{
				InstanceID: "supply", CatalogID: "source.voltage.connector.1x02", Family: "voltage_source",
				ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveVoltageSourceV1}},
				Connections: []simmodel.ConnectionEvidence{{Function: "POSITIVE", Net: "VCC"}, {Function: "NEGATIVE", Net: "GND"}},
			},
			{
				InstanceID: "load", CatalogID: "resistor.generic.0603", Family: "resistor", HasValueSI: true, ValueSI: 1000,
				ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveResistorV1}},
				Connections: []simmodel.ConnectionEvidence{{Function: "A", Net: "VCC"}, {Function: "B", Net: "GND"}},
			},
		},
	}
	return request, Environment{Sources: []Source{source}, EnabledSolvers: EnabledBuiltinSolverIDs()}
}

func hasDiagnosticCode(diagnostics []Diagnostic, code DiagnosticCode) bool {
	return slices.ContainsFunc(diagnostics, func(diagnostic Diagnostic) bool { return diagnostic.Code == code })
}

func admissionTestHash(seed string) string {
	digest, err := hashJSON(seed)
	if err != nil {
		panic(err)
	}
	return digest
}
