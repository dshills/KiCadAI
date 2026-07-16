package circuitgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

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
