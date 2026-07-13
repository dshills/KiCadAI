package circuitgraph

import (
	"context"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
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
			if len(request.ExplicitCircuit.Components) != len(resolved.Components) || len(request.ExplicitCircuit.Nets) != len(resolved.Nets) {
				t.Fatalf("explicit counts = components %d nets %d", len(request.ExplicitCircuit.Components), len(request.ExplicitCircuit.Nets))
			}
			if validation := designworkflow.ValidateRequest(request); reports.HasBlockingIssue(validation) {
				t.Fatalf("workflow validation = %#v", validation)
			}
			if got, want := request.Constraints.AllowBackLayer, resolved.Source.Project.Board.Layers > 1; got != want {
				t.Fatalf("allow back layer = %v, want %v for %d layers", got, want, resolved.Source.Project.Board.Layers)
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
