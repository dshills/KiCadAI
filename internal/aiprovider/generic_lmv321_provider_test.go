package aiprovider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
)

var analogReviewNormalizer = strings.NewReplacer("_", "", " ", "", "-", "")

var analogReviewCategoryNames = []struct {
	name    string
	compact string
}{
	{name: "stability", compact: "stability"},
	{name: "gain_bandwidth", compact: "gainbandwidth"},
	{name: "output_drive", compact: "outputdrive"},
	{name: "noise", compact: "noise"},
	{name: "distortion", compact: "distortion"},
	{name: "load_compatibility", compact: "loadcompatibility"},
}

func TestRecordedGenericLMV321GraphResolvesAndLowers(t *testing.T) {
	fixtureDir := filepath.Dir(providerFixturePath(t, "generic_lmv321_ac_gain_stage", "prompt.txt"))
	prompt, err := os.ReadFile(filepath.Join(fixtureDir, "prompt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := os.ReadFile(filepath.Join(fixtureDir, "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewRecordedProvider("generic_lmv321_ac_gain_stage", recorded)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.GenerateIntent(context.Background(), GenerateRequest{
		Prompt: string(prompt), SchemaVersion: EnvelopeSchemaV1, Attempt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	projection := genericLMV321ProjectionFor(t, decodeAndResolveGraph(t, result.IntentJSON, loadGenericFixtureCatalog(t)))
	assertGenericLMV321ProjectionComplete(t, projection)
}

func TestOpenAILiveGenericLMV321Graph(t *testing.T) {
	if os.Getenv("KICADAI_OPENAI_LIVE_TEST") != "1" {
		t.Skip("set KICADAI_OPENAI_LIVE_TEST=1 to run the live provider test")
	}
	fixtureDir := filepath.Dir(providerFixturePath(t, "generic_lmv321_ac_gain_stage", "prompt.txt"))
	prompt, err := os.ReadFile(filepath.Join(fixtureDir, "prompt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	catalog := loadGenericFixtureCatalog(t)
	capability, err := circuitgraph.ProviderCapabilityContext(catalog, MaxCapabilityBytes)
	if err != nil {
		t.Fatal(err)
	}
	liveJSON := loadOrGenerateLiveGraph(t, prompt, capability, "KICADAI_OPENAI_LIVE_GRAPH", "generic LMV321")
	live := genericLMV321ProjectionFor(t, decodeAndResolveGraph(t, liveJSON, catalog))
	recordedEnvelope, err := os.ReadFile(filepath.Join(fixtureDir, "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	recordedJSON, err := DecodeEnvelope(recordedEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	recorded := genericLMV321ProjectionFor(t, decodeAndResolveGraph(t, recordedJSON, catalog))
	assertGenericLMV321ProjectionComplete(t, live)
	assertGenericLMV321CriticalEquivalence(t, live, recorded)
}

func loadOrGenerateLiveGraph(t *testing.T, prompt []byte, capability, artifactEnv, description string) []byte {
	t.Helper()
	if path := strings.TrimSpace(os.Getenv(artifactEnv)); path != "" {
		graph, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read live OpenAI graph: %v", err)
		}
		return graph
	}
	provider, err := NewOpenAIProvider(OpenAIOptionsFromEnvironment())
	if err != nil {
		t.Fatalf("configure provider: %v", err)
	}
	profile := GenericCircuitProfile(capability)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	result, err := provider.GenerateIntent(ctx, GenerateRequest{
		Prompt: string(prompt), CapabilityContext: profile.CapabilityContext,
		OutputSchemaName: profile.SchemaName, OutputSchema: profile.IntentEnvelopeSchema(),
		SchemaVersion: EnvelopeSchemaV1, Attempt: 1,
	})
	if err != nil {
		t.Fatalf("generate %s graph: %v", description, err)
	}
	return result.IntentJSON
}

type genericLMV321Projection struct {
	ComponentCount int
	NetCount       int
	PowerFlagCount int
	Flow           circuitgraph.Flow
	ComponentID    string
	SymbolID       string
	FootprintID    string
	Components     []string
	Nets           []string
	PowerFlags     []string
	ReviewEvidence []string
}

func genericLMV321ProjectionFor(t *testing.T, resolved circuitgraph.ResolvedDocument) genericLMV321Projection {
	t.Helper()
	projection := genericLMV321Projection{
		ComponentCount: len(resolved.Components),
		NetCount:       len(resolved.Nets),
		PowerFlagCount: len(resolved.Source.PowerFlags),
		Flow:           resolved.Source.Schematic.Flow,
	}
	componentKeys := make(map[string]string, len(resolved.Components))
	for _, component := range resolved.Components {
		key := genericLMV321SemanticComponentKey(t, component)
		componentKeys[component.Instance.ID] = key
		projection.Components = append(projection.Components, key)
		if component.ComponentID != "opamp.ti.lmv321.sot23_5" {
			continue
		}
		projection.ComponentID = component.ComponentID
		projection.SymbolID = component.SymbolID
		projection.FootprintID = component.FootprintID
		projection.ReviewEvidence = append(projection.ReviewEvidence, analogReviewCategories(component)...)
	}
	slices.Sort(projection.Components)

	netProjectionByName := make(map[string]string, len(resolved.Nets))
	for _, net := range resolved.Nets {
		endpoints := make([]string, 0, len(net.Endpoints))
		for _, endpoint := range net.Endpoints {
			key, ok := componentKeys[endpoint.Intent.Component]
			if !ok {
				t.Fatalf("resolved net %q references unknown component %q", net.Intent.Name, endpoint.Intent.Component)
			}
			endpoints = append(endpoints, key+"."+endpoint.Function)
		}
		slices.Sort(endpoints)
		required := net.Intent.Required != nil && *net.Intent.Required
		netProjection := fmt.Sprintf("%s:%t:%.3f:%s", net.Intent.Role, required, net.Intent.WidthMM, strings.Join(endpoints, ","))
		projection.Nets = append(projection.Nets, netProjection)
		netProjectionByName[net.Intent.Name] = netProjection
	}
	slices.Sort(projection.Nets)
	for _, flag := range resolved.Source.PowerFlags {
		netProjection, ok := netProjectionByName[flag.Net]
		if !ok {
			t.Fatalf("power flag references unknown net %q", flag.Net)
		}
		projection.PowerFlags = append(projection.PowerFlags, netProjection)
	}
	slices.Sort(projection.PowerFlags)
	slices.Sort(projection.ReviewEvidence)
	return projection
}

func genericLMV321SemanticComponentKey(t *testing.T, component circuitgraph.ResolvedComponent) string {
	t.Helper()
	role := string(component.Instance.Role)
	switch role {
	case "ic":
		return role + ":" + component.ComponentID
	case "input_connector", "output_connector":
		return role + ":" + component.Family
	case "capacitor", "decoupling_capacitor", "resistor":
		value, ok := components.ParseEngineeringValue(component.Instance.Value)
		if !ok {
			t.Fatalf("component %q has non-engineering value %q", component.Instance.ID, component.Instance.Value)
		}
		return fmt.Sprintf("%s:%s:%.12g", role, component.Family, value)
	default:
		return "unsupported-role:" + role + ":" + component.ComponentID
	}
}

func analogReviewCategories(component circuitgraph.ResolvedComponent) []string {
	categories := make(map[string]struct{})
	for _, property := range component.Instance.Properties {
		text := strings.ToLower(property.Name + " " + property.Value)
		compact := analogReviewNormalizer.Replace(text)
		if !strings.Contains(compact, "reviewrequired") {
			continue
		}
		for _, category := range analogReviewCategoryNames {
			if strings.Contains(compact, category.compact) {
				categories[category.name] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(categories))
	for category := range categories {
		result = append(result, category)
	}
	slices.Sort(result)
	return result
}

func assertGenericLMV321ProjectionComplete(t *testing.T, projection genericLMV321Projection) {
	t.Helper()
	for _, component := range projection.Components {
		if strings.HasPrefix(component, "unsupported-role:") {
			t.Fatalf("generic LMV321 projection contains %q", component)
		}
	}
	if projection.ComponentCount != 14 || projection.NetCount != 8 || projection.PowerFlagCount != 2 {
		t.Fatalf("graph cardinality = components:%d nets:%d flags:%d, want components:14 nets:8 flags:2", projection.ComponentCount, projection.NetCount, projection.PowerFlagCount)
	}
	if projection.Flow != circuitgraph.FlowLeftToRight {
		t.Fatalf("schematic flow = %q", projection.Flow)
	}
	if projection.ComponentID != "opamp.ti.lmv321.sot23_5" || projection.SymbolID != "Amplifier_Operational:LMV321" || projection.FootprintID != "Package_TO_SOT_SMD:SOT-23-5" {
		t.Fatalf("amplifier identity = component:%q symbol:%q footprint:%q, want component:%q symbol:%q footprint:%q",
			projection.ComponentID, projection.SymbolID, projection.FootprintID,
			"opamp.ti.lmv321.sot23_5", "Amplifier_Operational:LMV321", "Package_TO_SOT_SMD:SOT-23-5")
	}
	wantReview := []string{"distortion", "gain_bandwidth", "load_compatibility", "noise", "output_drive", "stability"}
	if !slices.Equal(projection.ReviewEvidence, wantReview) {
		t.Fatalf("analog review evidence = %q, want %q", projection.ReviewEvidence, wantReview)
	}
}

func assertGenericLMV321CriticalEquivalence(t *testing.T, live, recorded genericLMV321Projection) {
	t.Helper()
	if live.Flow != recorded.Flow || live.ComponentID != recorded.ComponentID || live.SymbolID != recorded.SymbolID || live.FootprintID != recorded.FootprintID {
		t.Fatalf("live identity/layout differs from recorded\nlive=%#v\nrecorded=%#v", live, recorded)
	}
	for name, pair := range map[string][2][]string{
		"components":      {live.Components, recorded.Components},
		"nets":            {live.Nets, recorded.Nets},
		"power flags":     {live.PowerFlags, recorded.PowerFlags},
		"review evidence": {live.ReviewEvidence, recorded.ReviewEvidence},
	} {
		if !slices.Equal(pair[0], pair[1]) {
			t.Fatalf("live %s differ from recorded\nlive=%q\nrecorded=%q", name, pair[0], pair[1])
		}
	}
}
