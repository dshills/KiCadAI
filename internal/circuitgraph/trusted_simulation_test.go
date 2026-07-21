package circuitgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

func TestTrustedSimulationResolvesCatalogValuesAndEvaluates(t *testing.T) {
	graph := loadGraphExample(t, "rc_filter.json")
	graph.Simulation = &SimulationIntent{
		ModelID:    simmodel.ModelRCLowpassACV1,
		Bindings:   []simmodel.Binding{{Role: "resistor", Component: "r1"}, {Role: "capacitor", Component: "c1"}},
		Inputs:     []simmodel.NamedValue{{Name: "frequency_hz", Value: 1000}},
		Assertions: []simmodel.Assertion{{Metric: "cutoff_frequency_hz", Min: 159, Max: 160}, {Metric: "gain_ratio", Min: 0.15, Max: 0.16}},
	}
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), graph)
	if reports.HasBlockingIssue(issues) || resolved.Simulation == nil {
		t.Fatalf("simulation resolution = %#v issues %#v", resolved.Simulation, issues)
	}
	if len(resolved.Simulation.Bindings) != 2 || resolved.Simulation.Bindings[0].ValueSI == nil || resolved.Simulation.Bindings[1].ValueSI == nil {
		t.Fatalf("resolved catalog values = %#v", resolved.Simulation.Bindings)
	}
	report, diagnostics := simmodel.Evaluate(*resolved.Simulation)
	if len(diagnostics) != 0 || report.Status != "pass" || report.CatalogHash != resolved.CatalogHash {
		t.Fatalf("trusted report = %#v diagnostics %#v", report, diagnostics)
	}
}

func TestGraphDerivedMNAFixtureResolvesConnectivityAndEvaluates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "ai", "generic_mna_buffered_two_pole", "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Intent Document `json:"intent"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), envelope.Intent)
	if reports.HasBlockingIssue(issues) || resolved.Simulation == nil {
		t.Fatalf("MNA simulation resolution = %#v issues %#v", resolved.Simulation, issues)
	}
	plan := resolved.Simulation
	if plan.ModelID != simmodel.ModelLinearCircuitMNAV1 || plan.TopologyHash == "" || plan.GroundNode != "GND" || len(plan.Devices) != 8 || len(plan.Analyses) != 2 {
		t.Fatalf("resolved MNA plan = %#v", plan)
	}
	report, diagnostics := simmodel.Evaluate(*plan)
	if len(diagnostics) != 0 || report.Status != "pass" || len(report.Analyses) != 2 || len(report.Assertions) != 5 {
		t.Fatalf("MNA report = %#v diagnostics %#v", report, diagnostics)
	}
	for _, assertion := range report.Assertions {
		if !assertion.Pass {
			t.Fatalf("MNA assertion = %#v", assertion)
		}
	}
	request, requestIssues := ToDesignRequest(resolved)
	if reports.HasBlockingIssue(requestIssues) {
		t.Fatalf("MNA design request issues = %#v", requestIssues)
	}
	index := schematicTestLibraryIndex(resolved)
	placed := designworkflow.PlaceExplicitCircuit(context.Background(), request, designworkflow.PlacementOptions{LibraryIndex: &index})
	routed := designworkflow.RouteExplicitCircuit(context.Background(), request, placed, designworkflow.RoutingOptions{})
	if routed.Stage.Status == designworkflow.StageStatusBlocked {
		t.Fatalf("MNA fixture routing blocked: placements=%#v operations=%#v result=%#v issues=%#v", placed.Result.Placements, routed.Operations, routed.Result, routed.Stage.Issues)
	}
}

func TestCatalogBackedNonlinearFixtureResolvesAndEvaluates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "ai", "generic_nonlinear_npn_bias", "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Intent Document `json:"intent"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), envelope.Intent)
	if reports.HasBlockingIssue(issues) || resolved.Simulation == nil {
		t.Fatalf("nonlinear simulation resolution = %#v issues %#v", resolved.Simulation, issues)
	}
	plan := resolved.Simulation
	if plan.ModelID != simmodel.ModelNonlinearCircuitDCV1 || plan.TopologyHash == "" || len(plan.Devices) != 6 || len(plan.Analyses) != 1 {
		t.Fatalf("resolved nonlinear plan = %#v", plan)
	}
	report, diagnostics := simmodel.Evaluate(*plan)
	if len(diagnostics) != 0 || report.Status != "pass" || len(report.Assertions) != 4 || report.Analyses[0].Points[0].Solver == nil {
		t.Fatalf("nonlinear report = %#v diagnostics %#v", report, diagnostics)
	}
	first, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	replayed, diagnostics := simmodel.Evaluate(simmodel.ClonePlan(*plan))
	if len(diagnostics) != 0 {
		t.Fatalf("replay diagnostics = %#v", diagnostics)
	}
	second, err := json.Marshal(replayed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("nonlinear report replay differs\n%s\n%s", first, second)
	}
}

func TestTrustedSimulationFailsClosedWithoutCatalogModelEvidence(t *testing.T) {
	graph := loadGraphExample(t, "rc_filter.json")
	graph.Simulation = &SimulationIntent{
		ModelID:    simmodel.ModelRCLowpassACV1,
		Bindings:   []simmodel.Binding{{Role: "resistor", Component: "r1"}, {Role: "capacitor", Component: "c1"}},
		Inputs:     []simmodel.NamedValue{{Name: "frequency_hz", Value: 1000}},
		Assertions: []simmodel.Assertion{{Metric: "gain_ratio", Min: 0, Max: 1}},
	}
	catalog := loadGraphCatalog(t)
	for index := range catalog.Records {
		catalog.Records[index].SimulationModels = nil
	}
	_, issues := NewResolver(ResolveOptions{Catalog: catalog, CatalogID: "without-model-evidence"}).Resolve(context.Background(), graph)
	for _, issue := range issues {
		if issue.Code == CodeSimulationInvalid && issue.Blocking() && issue.Suggestion != "" {
			return
		}
	}
	t.Fatalf("missing actionable catalog-model blocker: %#v", issues)
}

func TestSimulationHarnessRejectsPhysicalComponentIdentityCollision(t *testing.T) {
	graph := loadGraphExample(t, "rc_filter.json")
	resolver := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"})
	resolved, issues := resolver.Resolve(context.Background(), graph)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("physical graph resolution issues = %#v", issues)
	}

	_, issues = resolver.ResolveSimulationPlanWithHarness(simmodel.Intent{}, resolved, []SimulationHarnessDevice{{
		InstanceID: resolved.Components[0].Instance.ID,
		CatalogID:  resolved.Components[0].ComponentID,
	}})
	for _, issue := range issues {
		if issue.Code == CodeSimulationInvalid && issue.Path == "simulation.harness[0].instance_id" && issue.Blocking() {
			return
		}
	}
	t.Fatalf("physical/harness identity collision was accepted: %#v", issues)
}

func TestTrustedSimulationSchemaRejectsProviderModelContent(t *testing.T) {
	graph := loadGraphExample(t, "rc_filter.json")
	graph.Simulation = &SimulationIntent{
		ModelID:    simmodel.ModelRCLowpassACV1,
		Bindings:   []simmodel.Binding{{Role: "resistor", Component: "r1"}, {Role: "capacitor", Component: "c1"}},
		Inputs:     []simmodel.NamedValue{{Name: "frequency_hz", Value: 1000}},
		Assertions: []simmodel.Assertion{{Metric: "gain_ratio", Min: 0, Max: 1}},
	}
	data, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"model_id":"rc_lowpass_ac_v1"`), []byte(`"model_id":"rc_lowpass_ac_v1","model_file":"provider.cir"`), 1)
	if _, issues := DecodeStrict(bytes.NewReader(data)); len(issues) == 0 || !reports.HasBlockingIssue(issues) {
		t.Fatalf("provider model content was accepted: %#v", issues)
	}
}

func TestGraphDerivedMNASchemaRejectsProviderSolverContent(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "ai", "generic_mna_buffered_two_pole", "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Intent Document `json:"intent"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(envelope.Intent)
	if err != nil {
		t.Fatal(err)
	}
	needle := []byte(`"model_id":"linear_circuit_mna_v1"`)
	for name, field := range map[string]string{
		"equations":               `"equations":["provider-controlled"]`,
		"executable_code":         `"executable_code":"solve()"`,
		"matrix":                  `"matrix":[[1]]`,
		"model_file":              `"model_file":"provider.cir"`,
		"topology_classification": `"topology_classification":"known_filter"`,
	} {
		t.Run(name, func(t *testing.T) {
			injected := bytes.Replace(data, needle, []byte(string(needle)+","+field), 1)
			if _, issues := DecodeStrict(bytes.NewReader(injected)); len(issues) == 0 || !reports.HasBlockingIssue(issues) {
				t.Fatalf("provider %s content was accepted: %#v", name, issues)
			}
		})
	}
	t.Run("undersized_ac_sweep", func(t *testing.T) {
		invalid := bytes.Replace(data, []byte(`"points":3`), []byte(`"points":1`), 1)
		if _, issues := DecodeStrict(bytes.NewReader(invalid)); len(issues) == 0 || !reports.HasBlockingIssue(issues) {
			t.Fatalf("undersized AC sweep was accepted: %#v", issues)
		}
	})
}

func TestNonlinearSchemaRejectsProviderModelsTopologyAndAC(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "ai", "generic_nonlinear_npn_bias", "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Intent Document `json:"intent"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(envelope.Intent)
	if err != nil {
		t.Fatal(err)
	}
	needle := []byte(`"model_id":"nonlinear_circuit_dc_v1"`)
	for name, field := range map[string]string{
		"device_model":            `"device_model":"provider-controlled"`,
		"parameters":              `"parameters":{"beta":999}`,
		"solver_settings":         `"solver_settings":{"iterations":999999}`,
		"equations":               `"equations":["provider-controlled"]`,
		"matrix":                  `"matrix":[[1]]`,
		"topology_classification": `"topology_classification":"transistor_bias"`,
	} {
		t.Run(name, func(t *testing.T) {
			injected := bytes.Replace(data, needle, []byte(string(needle)+","+field), 1)
			if _, issues := DecodeStrict(bytes.NewReader(injected)); len(issues) == 0 || !reports.HasBlockingIssue(issues) {
				t.Fatalf("provider %s content was accepted: %#v", name, issues)
			}
		})
	}
	ac := bytes.Replace(data, []byte(`"kind":"dc_operating_point"`), []byte(`"kind":"ac_sweep"`), 1)
	if _, issues := DecodeStrict(bytes.NewReader(ac)); len(issues) == 0 || !reports.HasBlockingIssue(issues) {
		t.Fatalf("nonlinear AC request was accepted: %#v", issues)
	}
}
