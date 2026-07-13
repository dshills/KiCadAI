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

func TestRecordedGenericUSBCBMP280GraphResolvesAndLowers(t *testing.T) {
	fixtureDir := filepath.Dir(providerFixturePath(t, "generic_usb_c_bmp280_breakout", "prompt.txt"))
	prompt, err := os.ReadFile(filepath.Join(fixtureDir, "prompt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := os.ReadFile(filepath.Join(fixtureDir, "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewRecordedProvider("generic_usb_c_bmp280_breakout", recorded)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.GenerateIntent(context.Background(), GenerateRequest{
		Prompt: string(prompt), SchemaVersion: EnvelopeSchemaV1, Attempt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	projection := genericBMP280ProjectionFor(t, decodeAndResolveGraph(t, result.IntentJSON, loadGenericFixtureCatalog(t)))
	assertGenericBMP280ProjectionComplete(t, projection)
}

func TestOpenAILiveGenericUSBCBMP280Graph(t *testing.T) {
	if os.Getenv("KICADAI_OPENAI_LIVE_TEST") != "1" {
		t.Skip("set KICADAI_OPENAI_LIVE_TEST=1 to run the live provider test")
	}
	fixtureDir := filepath.Dir(providerFixturePath(t, "generic_usb_c_bmp280_breakout", "prompt.txt"))
	prompt, err := os.ReadFile(filepath.Join(fixtureDir, "prompt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	catalog := loadGenericFixtureCatalog(t)
	capability, err := circuitgraph.ProviderCapabilityContext(catalog, MaxCapabilityBytes)
	if err != nil {
		t.Fatal(err)
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
		t.Fatalf("generate generic USB-C BMP280 graph: %v", err)
	}
	live := genericBMP280ProjectionFor(t, decodeAndResolveGraph(t, result.IntentJSON, catalog))
	recordedEnvelope, err := os.ReadFile(filepath.Join(fixtureDir, "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	recordedJSON, err := DecodeEnvelope(recordedEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	recorded := genericBMP280ProjectionFor(t, decodeAndResolveGraph(t, recordedJSON, catalog))
	assertGenericBMP280ProjectionComplete(t, live)
	assertGenericBMP280CriticalEquivalence(t, live, recorded)
}

type genericBMP280Projection struct {
	Board      string
	Components []string
	Nets       []string
	PowerFlags []string
	Flow       circuitgraph.Flow
	SensorID   string
	SensorFoot string
}

func genericBMP280ProjectionFor(t *testing.T, resolved circuitgraph.ResolvedDocument) genericBMP280Projection {
	t.Helper()
	componentKeys := make(map[string]string, len(resolved.Components))
	componentsProjection := make([]string, 0, len(resolved.Components))
	projection := genericBMP280Projection{Flow: resolved.Source.Schematic.Flow, Nets: make([]string, 0, len(resolved.Nets))}
	for _, component := range resolved.Components {
		key := genericBMP280SemanticComponentKey(t, component)
		componentKeys[component.Instance.ID] = key
		componentsProjection = append(componentsProjection, key)
		if component.ComponentID == "sensor.bosch.bmp280.lga8" {
			projection.SensorID = component.ComponentID
			projection.SensorFoot = component.FootprintID
		}
	}
	slices.Sort(componentsProjection)
	projection.Components = componentsProjection

	netProjectionByName := make(map[string]string, len(resolved.Nets))
	for _, net := range resolved.Nets {
		endpoints := make([]string, 0, len(net.Endpoints))
		for _, endpoint := range net.Endpoints {
			componentKey, ok := componentKeys[endpoint.Intent.Component]
			if !ok {
				t.Fatalf("resolved net %q references unknown component %q", net.Intent.Name, endpoint.Intent.Component)
			}
			endpoints = append(endpoints, componentKey+"."+endpoint.Function)
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
	board := resolved.Source.Project.Board
	projection.Board = fmt.Sprintf("%.3fx%.3f:%d:%s", board.WidthMM, board.HeightMM, board.Layers, resolved.Source.Project.Acceptance)
	return projection
}

func genericBMP280SemanticComponentKey(t *testing.T, component circuitgraph.ResolvedComponent) string {
	t.Helper()
	role := string(component.Instance.Role)
	switch role {
	case "input_connector", "regulator", "sensor":
		return strings.Join([]string{role, component.ComponentID}, ":")
	case "bulk_capacitor", "decoupling_capacitor", "fuse", "pullup", "resistor":
		value, ok := components.ParseEngineeringValue(component.Instance.Value)
		if !ok {
			t.Fatalf("component %q has non-engineering value %q", component.Instance.ID, component.Instance.Value)
		}
		return fmt.Sprintf("%s:%s:%.12g", role, component.Family, value)
	case "output_connector", "tvs":
		return strings.Join([]string{role, component.Family}, ":")
	default:
		t.Fatalf("component %q has unsupported semantic role %q", component.Instance.ID, role)
		return ""
	}
}

func assertGenericBMP280CriticalEquivalence(t *testing.T, live, recorded genericBMP280Projection) {
	t.Helper()
	if live.Flow != recorded.Flow || live.SensorID != recorded.SensorID || live.SensorFoot != recorded.SensorFoot {
		t.Fatalf("live identity/layout differs from recorded\nlive=%#v\nrecorded=%#v", live, recorded)
	}
	if !slices.Equal(live.Components, recorded.Components) {
		t.Fatalf("live component semantics differ from recorded\nlive=%q\nrecorded=%q", live.Components, recorded.Components)
	}
	if !slices.Equal(live.Nets, recorded.Nets) {
		t.Fatalf("live net semantics differ from recorded\nlive=%q\nrecorded=%q", live.Nets, recorded.Nets)
	}
	if !slices.Equal(live.PowerFlags, recorded.PowerFlags) {
		t.Fatalf("live power-source intent differs from recorded\nlive=%q\nrecorded=%q", live.PowerFlags, recorded.PowerFlags)
	}
}

func assertGenericBMP280ProjectionComplete(t *testing.T, projection genericBMP280Projection) {
	t.Helper()
	if !strings.HasSuffix(projection.Board, ":2:erc-drc") || projection.Flow != circuitgraph.FlowLeftToRight {
		t.Fatalf("board/layout projection = %#v", projection)
	}
	if len(projection.Components) != 15 || len(projection.Nets) != 9 || len(projection.PowerFlags) != 2 {
		t.Fatalf("graph cardinality = components:%d nets:%d flags:%d", len(projection.Components), len(projection.Nets), len(projection.PowerFlags))
	}
	if projection.SensorID != "sensor.bosch.bmp280.lga8" || projection.SensorFoot != "Package_LGA:Bosch_LGA-8_2x2.5mm_P0.65mm_ClockwisePinNumbering" {
		t.Fatalf("sensor identity = %q / %q", projection.SensorID, projection.SensorFoot)
	}
}

func loadGenericFixtureCatalog(t *testing.T) *components.Catalog {
	t.Helper()
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{
		CatalogDir: filepath.Join(providerRepoRoot(t), "data", "components"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}
