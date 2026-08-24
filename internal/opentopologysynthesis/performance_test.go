package opentopologysynthesis

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/modelprovenance"
)

func BenchmarkSynthesizePoweredLowpass(b *testing.B) {
	requirement, inventory, environment := benchmarkSimulationFixture(b)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 1_000
	policy.MaxGeneratedGraphs = 5_000
	policy.MaxRetainedCandidates = 4
	policy.MaxValueTrials = 4
	policy.MaxTopologyRepairs = 2
	policy.MaxCandidateSimulations = 256
	policy.MaxCornerEvaluations = 1_024

	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		run := Synthesize(context.Background(), requirement, inventory, environment, policy)
		if len(run.Hash) != 64 || run.Report.Status == StatusInvalid {
			b.Fatalf("incomplete synthesis result: status=%s hash=%q", run.Report.Status, run.Hash)
		}
	}
}

func benchmarkSimulationFixture(b *testing.B) (Requirement, PrimitiveInventory, SimulationEnvironment) {
	b.Helper()
	raw, err := os.ReadFile(filepath.Join(frozenCorpusRoot(), "powered_lowpass.json"))
	if err != nil {
		b.Fatal(err)
	}
	requirement, issues := DecodeStrict(bytes.NewReader(raw))
	if len(issues) != 0 {
		b.Fatalf("requirement issues: %#v", issues)
	}
	var cutoff BehavioralAssertion
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "cutoff_frequency" {
			cutoff = assertion
			break
		}
	}
	if cutoff.ID == "" {
		b.Fatal("powered-lowpass cutoff assertion is missing")
	}
	requirement.Requirements.BehavioralRequirements = []BehavioralAssertion{cutoff}
	operatingCase := requirement.Requirements.OperatingCases[0]
	operatingCase.Conditions = operatingCase.Conditions[:1]
	requirement.Requirements.OperatingCases = []OperatingCase{operatingCase}

	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		b.Fatal(err)
	}
	registry, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		b.Fatalf("model-provenance diagnostics: %#v", diagnostics)
	}
	catalogHash := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog}).CatalogHash()
	inventory, issues := BuildPrimitiveInventory(catalog, catalogHash, registry)
	if len(issues) != 0 {
		b.Fatalf("primitive inventory issues: %#v", issues)
	}
	return requirement, inventory, SimulationEnvironment{
		Catalog: catalog, CatalogHash: catalogHash, ModelRegistry: registry,
	}
}
