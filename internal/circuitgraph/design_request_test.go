package circuitgraph

import (
	"context"
	"strings"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

func TestToDesignRequestCheckedInExamples(t *testing.T) {
	resolver := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"})
	for _, name := range []string{"rc_filter.json", "transistor_switch.json", "usb_c_led_indicator_protected.json", "usb_c_bmp280_breakout.json"} {
		t.Run(name, func(t *testing.T) {
			resolved, issues := resolver.Resolve(context.Background(), loadGraphExample(t, name))
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("resolve issues = %#v", issues)
			}
			request, issues := ToDesignRequest(resolved)
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("request issues = %#v", issues)
			}
			if request.ExplicitCircuit == nil || len(request.Blocks) != 0 {
				t.Fatalf("request mode = %#v", request)
			}
			if request.ExplicitCircuit.RoutingPolicy != "" {
				t.Fatalf("explicit graph unexpectedly opted into synthesized routing policy %q", request.ExplicitCircuit.RoutingPolicy)
			}
			if len(request.ExplicitCircuit.Components) != len(resolved.Components) || len(request.ExplicitCircuit.Nets) != len(resolved.Nets) {
				t.Fatalf("explicit counts = components %d nets %d", len(request.ExplicitCircuit.Components), len(request.ExplicitCircuit.Nets))
			}
			if validation := designworkflow.ValidateRequest(request); reports.HasBlockingIssue(validation) {
				t.Fatalf("workflow validation = %#v", validation)
			}
			if got, want := request.Constraints.AllowBackLayer, resolved.Source.Project.Board.Layers > 1; got != want {
				t.Fatalf("allow back layer = %v, want %v for %d layers", got, want, resolved.Source.Project.Board.Layers)
			}
			policy := request.RoutingRetry
			if !policy.Enabled || policy.MaxAttempts != 3 || !policy.PreserveFixed || !policy.StopOnNewBlockers || !policy.StopOnRepeatedSignature || !policy.StopOnNonImprovement {
				t.Fatalf("generic routing retry policy = %#v", policy)
			}
		})
	}
}

func TestToDesignRequestPreservesBMP280PadNets(t *testing.T) {
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), loadGraphExample(t, "usb_c_bmp280_breakout.json"))
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	request, issues := ToDesignRequest(resolved)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("request issues = %#v", issues)
	}
	want := map[string]string{"1": "GND", "2": "VCC_3v3", "3": "SDA", "4": "SCL", "5": "GND", "6": "VCC_3v3", "7": "GND", "8": "VCC_3v3"}
	for _, component := range request.ExplicitCircuit.Components {
		if component.ID != "sensor" {
			continue
		}
		for _, pad := range component.Pads {
			if want[pad.Name] != pad.Net {
				t.Fatalf("sensor pad %s net = %q, want %q", pad.Name, pad.Net, want[pad.Name])
			}
			delete(want, pad.Name)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing sensor pads = %#v", want)
	}
}

func TestToDesignRequestKeepsNamedLM358AsOnePhysicalComponent(t *testing.T) {
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), namedLM358Document())
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	request, issues := ToDesignRequest(resolved)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("request issues = %#v", issues)
	}
	count := 0
	for _, component := range request.ExplicitCircuit.Components {
		if component.ID != "amplifier" {
			continue
		}
		count++
		if component.Reference != "U1" || component.FootprintID != "Package_SO:SOIC-8_3.9x4.9mm_P1.27mm" || len(component.Pads) != 8 {
			t.Fatalf("physical LM358 component = %#v", component)
		}
	}
	if count != 1 {
		t.Fatalf("physical LM358 component count = %d, want 1", count)
	}
}

func TestToDesignRequestKeepsPowerFlagsOutOfPCBComponents(t *testing.T) {
	graph := loadGraphExample(t, "usb_c_bmp280_breakout.json")
	graph.PowerFlags = []PowerFlag{{Net: "VBUS_RAW"}, {Net: "GND"}}
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), graph)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	request, issues := ToDesignRequest(resolved)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("request issues = %#v", issues)
	}
	if got, want := len(request.ExplicitCircuit.Components), len(resolved.Components); got != want {
		t.Fatalf("PCB component count = %d, want %d", got, want)
	}
	for _, component := range request.ExplicitCircuit.Components {
		if strings.HasPrefix(component.ID, "kicadai_pwr_flag_") || component.FootprintID == "" {
			t.Fatalf("schematic-only component leaked into PCB request: %#v", component)
		}
	}
}

func TestToDesignRequestPreservesPCBIntent(t *testing.T) {
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), loadGraphExample(t, "usb_c_led_indicator_protected.json"))
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	request, issues := ToDesignRequest(resolved)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("request issues = %#v", issues)
	}
	if len(request.ExplicitCircuit.Regions) != 3 || len(request.ExplicitCircuit.Zones) != 1 {
		t.Fatalf("PCB regions/zones = %#v", request.ExplicitCircuit)
	}
	for _, net := range request.ExplicitCircuit.Nets {
		if net.Name == "VBUS_RAW" && (!net.Required || net.WidthMM != 0.8 || net.CurrentMA != 500) {
			t.Fatalf("VBUS_RAW policy = %#v", net)
		}
	}
}

func TestToDesignRequestCarriesAutomaticHierarchyPolicy(t *testing.T) {
	graph := loadGraphExample(t, "rc_filter.json")
	graph.Schematic.Hierarchy = HierarchyPolicy{Mode: "auto", MaxComponentsPerSheet: 12}
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), graph)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	request, issues := ToDesignRequest(resolved)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("request issues = %#v", issues)
	}
	if request.ExplicitCircuit == nil || !request.ExplicitCircuit.AutoHierarchy {
		t.Fatalf("automatic hierarchy policy was not lowered: %#v", request.ExplicitCircuit)
	}

	graph.Schematic.Hierarchy = HierarchyPolicy{Mode: "flat"}
	resolved, issues = NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), graph)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("flat resolve issues = %#v", issues)
	}
	request, issues = ToDesignRequest(resolved)
	if reports.HasBlockingIssue(issues) || request.ExplicitCircuit.AutoHierarchy {
		t.Fatalf("flat hierarchy policy = request %#v issues %#v", request.ExplicitCircuit, issues)
	}
}

func TestToDesignRequestLowersSimulationContract(t *testing.T) {
	graph := loadGraphExample(t, "usb_c_bmp280_breakout.json")
	graph.Simulation = &SimulationIntent{ModelID: simmodel.ModelLinearRegulatorIdealV1,
		Bindings:   []simmodel.Binding{{Role: "regulator", Component: "regulator"}},
		Inputs:     []simmodel.NamedValue{{Name: "input_voltage_v", Value: 5}, {Name: "load_current_ma", Value: 20}},
		Assertions: []simmodel.Assertion{{Metric: "output_voltage_v", Min: 3.2, Max: 3.4}}}
	resolved, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), graph)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	request, issues := ToDesignRequest(resolved)
	if reports.HasBlockingIssue(issues) || request.ExplicitCircuit.Simulation == nil {
		t.Fatalf("simulation lowering = %#v issues %#v", request.ExplicitCircuit, issues)
	}
	if got := request.ExplicitCircuit.Simulation; got.ModelID != graph.Simulation.ModelID || len(got.Bindings) != 1 || got.Bindings[0].Component != "regulator" || got.CatalogHash == "" || got.RegistryHash == "" {
		t.Fatalf("simulation = %#v", got)
	}
}
