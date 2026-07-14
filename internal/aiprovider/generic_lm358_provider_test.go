package aiprovider

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/reports"
)

func TestRecordedGenericLM358GraphResolvesAndLowersOnePackage(t *testing.T) {
	fixtureDir := filepath.Dir(providerFixturePath(t, "generic_lm358_buffered_signal_conditioner", "prompt.txt"))
	prompt, err := os.ReadFile(filepath.Join(fixtureDir, "prompt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := os.ReadFile(filepath.Join(fixtureDir, "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewRecordedProvider("generic_lm358_buffered_signal_conditioner", recorded)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.GenerateIntent(context.Background(), GenerateRequest{Prompt: string(prompt), SchemaVersion: EnvelopeSchemaV1, Attempt: 1})
	if err != nil {
		t.Fatal(err)
	}
	resolved := decodeAndResolveGraph(t, result.IntentJSON, loadGenericFixtureCatalog(t))
	assertGenericLM358ProjectionComplete(t, resolved)
}

func TestOpenAILiveGenericLM358Graph(t *testing.T) {
	if os.Getenv("KICADAI_OPENAI_LIVE_TEST") != "1" {
		t.Skip("set KICADAI_OPENAI_LIVE_TEST=1 to run the live provider test")
	}
	fixtureDir := filepath.Dir(providerFixturePath(t, "generic_lm358_buffered_signal_conditioner", "prompt.txt"))
	prompt, err := os.ReadFile(filepath.Join(fixtureDir, "prompt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	catalog := loadGenericFixtureCatalog(t)
	capability, err := circuitgraph.ProviderCapabilityContext(catalog, MaxCapabilityBytes)
	if err != nil {
		t.Fatal(err)
	}
	liveJSON := loadOrGenerateLiveGraph(t, prompt, capability, "KICADAI_OPENAI_LM358_LIVE_GRAPH", "generic LM358")
	live := decodeAndResolveGraph(t, liveJSON, catalog)
	recordedEnvelope, err := os.ReadFile(filepath.Join(fixtureDir, "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	recordedJSON, err := DecodeEnvelope(recordedEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	recorded := decodeAndResolveGraph(t, recordedJSON, catalog)
	assertGenericLM358ProjectionComplete(t, live)
	if got, want := strings.Join(genericLM358CriticalProjection(t, live), "\n"), strings.Join(genericLM358CriticalProjection(t, recorded), "\n"); got != want {
		t.Fatalf("live LM358 critical projection differs\nlive:\n%s\nrecorded:\n%s", got, want)
	}
}

func assertGenericLM358ProjectionComplete(t *testing.T, resolved circuitgraph.ResolvedDocument) {
	t.Helper()
	const wantComponents, wantNets, wantPowerFlags = 14, 9, 2
	if len(resolved.Components) != wantComponents || len(resolved.Nets) != wantNets || len(resolved.Source.PowerFlags) != wantPowerFlags {
		t.Fatalf("LM358 graph cardinality = components:%d (want %d), nets:%d (want %d), flags:%d (want %d)", len(resolved.Components), wantComponents, len(resolved.Nets), wantNets, len(resolved.Source.PowerFlags), wantPowerFlags)
	}
	var amplifier *circuitgraph.ResolvedComponent
	for index := range resolved.Components {
		if resolved.Components[index].ComponentID == "opamp.ti.lm358.soic8" {
			if amplifier != nil {
				t.Fatal("resolved more than one LM358 physical component")
			}
			amplifier = &resolved.Components[index]
		}
	}
	if amplifier == nil {
		t.Fatal("missing resolved LM358 component")
	}
	if amplifier.Instance.Reference != "U1" || amplifier.SymbolID != "Amplifier_Operational:LM358" || amplifier.FootprintID != "Package_SO:SOIC-8_3.9x4.9mm_P1.27mm" || len(amplifier.Units) != 3 {
		t.Fatalf("resolved LM358 identity = %#v", amplifier)
	}
	unitIDs := make([]string, 0, len(amplifier.Units))
	for _, unit := range amplifier.Units {
		unitIDs = append(unitIDs, unit.ID)
	}
	slices.Sort(unitIDs)
	if !slices.Equal(unitIDs, []string{"A", "B", "P"}) {
		t.Fatalf("resolved LM358 units = %#v", amplifier.Units)
	}

	document, issues := circuitgraph.ToSchematicIR(resolved)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("LM358 schematic issues = %#v", issues)
	}
	u1Units := 0
	for _, component := range document.Circuit.Components {
		if component.Ref == "U1" {
			u1Units++
		}
	}
	if u1Units != 3 {
		t.Fatalf("LM358 schematic unit count = %d, want 3", u1Units)
	}
	request, issues := circuitgraph.ToDesignRequest(resolved)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("LM358 design request issues = %#v", issues)
	}
	physicalCount := 0
	for _, component := range request.ExplicitCircuit.Components {
		if component.ID != amplifier.Instance.ID {
			continue
		}
		physicalCount++
		if component.Reference != "U1" || len(component.SchematicUnits) != 3 || len(component.Pads) != 8 {
			t.Fatalf("physical LM358 request component = %#v", component)
		}
	}
	if physicalCount != 1 {
		t.Fatalf("physical LM358 request count = %d, want 1", physicalCount)
	}

	wantReview := []string{"stability_status", "gain_bandwidth_status", "output_swing_status", "input_common_mode_status", "output_drive_status", "noise_status", "distortion_status", "load_compatibility_status"}
	properties := map[string]string{}
	for _, property := range amplifier.Instance.Properties {
		properties[property.Name] = property.Value
	}
	for _, name := range wantReview {
		if properties[name] != "review_required" {
			t.Fatalf("LM358 review property %s = %q", name, properties[name])
		}
	}
}

func genericLM358CriticalProjection(t *testing.T, resolved circuitgraph.ResolvedDocument) []string {
	t.Helper()
	projection := make([]string, 0, len(resolved.Components)+len(resolved.Nets)*4)
	componentsByID := make(map[string]circuitgraph.ResolvedComponent, len(resolved.Components))
	for _, component := range resolved.Components {
		componentsByID[component.Instance.ID] = component
		projection = append(projection, "component:"+string(component.Instance.Role)+":"+component.ComponentID+":"+component.FootprintID)
		for _, unit := range component.Units {
			projection = append(projection, "unit:"+component.ComponentID+":"+unit.ID+":"+unit.Role)
		}
	}
	for _, net := range resolved.Nets {
		endpoints := make([]string, 0, len(net.Endpoints))
		for _, endpoint := range net.Endpoints {
			component, exists := componentsByID[endpoint.Intent.Component]
			if !exists {
				t.Fatalf("resolved net %s references unknown component %s", net.Intent.Name, endpoint.Intent.Component)
			}
			for _, binding := range endpoint.Bindings {
				endpoints = append(endpoints, component.ComponentID+":"+endpoint.Intent.Unit+":"+endpoint.Function+":"+binding.Pad)
			}
		}
		slices.Sort(endpoints)
		projection = append(projection, "net:"+string(net.Intent.Role)+":"+strings.Join(endpoints, ","))
	}
	slices.Sort(projection)
	return projection
}
