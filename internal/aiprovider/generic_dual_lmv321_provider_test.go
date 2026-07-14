package aiprovider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/circuitgraph"
)

func TestRecordedGenericDualLMV321GraphResolvesAndLowers(t *testing.T) {
	fixtureDir := filepath.Dir(providerFixturePath(t, "generic_dual_lmv321_signal_conditioner", "prompt.txt"))
	prompt, err := os.ReadFile(filepath.Join(fixtureDir, "prompt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := os.ReadFile(filepath.Join(fixtureDir, "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewRecordedProvider("generic_dual_lmv321_signal_conditioner", recorded)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.GenerateIntent(context.Background(), GenerateRequest{
		Prompt: string(prompt), SchemaVersion: EnvelopeSchemaV1, Attempt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved := decodeAndResolveGraph(t, result.IntentJSON, loadGenericFixtureCatalog(t))
	projection := genericLMV321ProjectionFor(t, resolved)
	assertGenericDualLMV321ProjectionComplete(t, resolved, projection)
}

func TestOpenAILiveGenericDualLMV321Graph(t *testing.T) {
	if os.Getenv("KICADAI_OPENAI_LIVE_TEST") != "1" {
		t.Skip("set KICADAI_OPENAI_LIVE_TEST=1 to run the live provider test")
	}
	fixtureDir := filepath.Dir(providerFixturePath(t, "generic_dual_lmv321_signal_conditioner", "prompt.txt"))
	prompt, err := os.ReadFile(filepath.Join(fixtureDir, "prompt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	catalog := loadGenericFixtureCatalog(t)
	capability, err := circuitgraph.ProviderCapabilityContext(catalog, MaxCapabilityBytes)
	if err != nil {
		t.Fatal(err)
	}
	liveJSON := loadOrGenerateLiveGraph(t, prompt, capability, "KICADAI_OPENAI_DUAL_LMV321_LIVE_GRAPH", "generic dual LMV321")
	liveResolved := decodeAndResolveGraph(t, liveJSON, catalog)
	live := genericLMV321ProjectionFor(t, liveResolved)
	recordedEnvelope, err := os.ReadFile(filepath.Join(fixtureDir, "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	recordedJSON, err := DecodeEnvelope(recordedEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	recordedResolved := decodeAndResolveGraph(t, recordedJSON, catalog)
	recorded := genericLMV321ProjectionFor(t, recordedResolved)
	assertGenericDualLMV321ProjectionComplete(t, liveResolved, live)
	assertGenericLMV321CriticalEquivalence(t, live, recorded)
}

func assertGenericDualLMV321ProjectionComplete(t *testing.T, resolved circuitgraph.ResolvedDocument, projection genericLMV321Projection) {
	t.Helper()
	for _, component := range projection.Components {
		if strings.HasPrefix(component, "unsupported-role:") {
			t.Fatalf("generic dual LMV321 projection contains %q", component)
		}
	}
	if projection.ComponentCount != 18 || projection.NetCount != 10 || projection.PowerFlagCount != 2 {
		t.Fatalf("graph cardinality = components:%d nets:%d flags:%d, want components:18 nets:10 flags:2", projection.ComponentCount, projection.NetCount, projection.PowerFlagCount)
	}
	if projection.Flow != circuitgraph.FlowLeftToRight {
		t.Fatalf("schematic flow = %q", projection.Flow)
	}
	opampKey := "ic:opamp.ti.lmv321.sot23_5"
	if count := countStrings(projection.Components, opampKey); count != 2 {
		t.Fatalf("LMV321 component multiplicity = %d, want 2", count)
	}
	assertReviewEvidenceMultiplicity(t, projection.ReviewEvidence, 2)
	assertDualLMV321Identities(t, resolved)
	assertDualLMV321StageOrder(t, resolved)
}

func assertDualLMV321Identities(t *testing.T, resolved circuitgraph.ResolvedDocument) {
	t.Helper()
	count := 0
	for _, component := range resolved.Components {
		if component.ComponentID != "opamp.ti.lmv321.sot23_5" {
			continue
		}
		count++
		if component.SymbolID != "Amplifier_Operational:LMV321" || component.FootprintID != "Package_TO_SOT_SMD:SOT-23-5" {
			t.Fatalf("LMV321 %q identity = symbol:%q footprint:%q", component.Instance.ID, component.SymbolID, component.FootprintID)
		}
		assertReviewEvidenceMultiplicity(t, analogReviewCategories(component), 1)
	}
	if count != 2 {
		t.Fatalf("resolved LMV321 count = %d, want 2", count)
	}
}

func assertDualLMV321StageOrder(t *testing.T, resolved circuitgraph.ResolvedDocument) {
	t.Helper()
	opamps := make(map[string]struct{})
	for _, component := range resolved.Components {
		if component.ComponentID == "opamp.ti.lmv321.sot23_5" {
			opamps[component.Instance.ID] = struct{}{}
		}
	}
	var upstream, downstream string
	feedbackNets := 0
	for _, net := range resolved.Nets {
		var output, inputPlus string
		invertingInputs := 0
		for _, endpoint := range net.Endpoints {
			if _, ok := opamps[endpoint.Intent.Component]; !ok {
				continue
			}
			switch endpoint.Function {
			case "OUT":
				output = endpoint.Intent.Component
			case "IN_PLUS":
				inputPlus = endpoint.Intent.Component
			case "IN_MINUS":
				invertingInputs++
			}
		}
		if output != "" && inputPlus != "" && output != inputPlus {
			if upstream != "" {
				t.Fatal("multiple inter-stage LMV321 nets found")
			}
			upstream, downstream = output, inputPlus
		}
		if net.Intent.Role == circuitgraph.NetRoleFeedback {
			feedbackNets++
			if invertingInputs != 1 {
				t.Fatalf("feedback net %q has %d LMV321 inverting inputs, want 1", net.Intent.Name, invertingInputs)
			}
		}
	}
	if upstream == "" || downstream == "" {
		t.Fatal("no LMV321 OUT-to-IN_PLUS inter-stage net found")
	}
	if feedbackNets != 2 {
		t.Fatalf("feedback net count = %d, want 2", feedbackNets)
	}
	ranks := make(map[string]int)
	for _, group := range resolved.Source.Schematic.Groups {
		for _, member := range group.Members {
			if member == upstream || member == downstream {
				ranks[member] = group.Rank
			}
		}
	}
	upstreamRank, upstreamOK := ranks[upstream]
	downstreamRank, downstreamOK := ranks[downstream]
	if !upstreamOK {
		t.Fatalf("upstream LMV321 %q is not assigned to a schematic group", upstream)
	}
	if !downstreamOK {
		t.Fatalf("downstream LMV321 %q is not assigned to a schematic group", downstream)
	}
	if upstreamRank >= downstreamRank {
		t.Fatalf("inter-stage layout ranks = %q:%d %q:%d, want upstream before downstream", upstream, upstreamRank, downstream, downstreamRank)
	}
}

func assertReviewEvidenceMultiplicity(t *testing.T, evidence []string, wantCount int) {
	t.Helper()
	counts := make(map[string]int, len(analogReviewCategoryNames))
	for _, category := range evidence {
		counts[category]++
	}
	if len(evidence) != wantCount*len(analogReviewCategoryNames) {
		t.Fatalf("analog review evidence = %q, want %d copies of every category", evidence, wantCount)
	}
	for _, category := range analogReviewCategoryNames {
		if counts[category.name] != wantCount {
			t.Fatalf("analog review category %q count = %d, want %d; all evidence = %q", category.name, counts[category.name], wantCount, evidence)
		}
	}
}

func countStrings(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}
