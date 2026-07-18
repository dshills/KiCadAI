package blocks

import (
	"context"
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestAmplifierSupplyDecouplingInstantiatesSingleSupply(t *testing.T) {
	output, issues := NewBuiltinRegistry().Instantiate(context.Background(), BlockRequest{
		BlockID:    "amplifier_supply_decoupling",
		InstanceID: "decouple",
		Params: map[string]any{
			"rail_mode":                "single_supply",
			"rail_voltage":             "9V",
			"ceramic_capacitance":      "100nF",
			"bulk_capacitance":         "10uF",
			"include_bulk":             true,
			"capacitor_voltage_rating": "16V",
			"ceramic_footprint":        "Capacitor_SMD:C_0805_2012Metric",
			"bulk_footprint":           "Capacitor_SMD:C_1210_3225Metric",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 2 {
		t.Fatalf("refs = %#v, want ceramic and bulk VCC decoupling", output.Instance.Refs)
	}
	for _, net := range []string{"decouple_vcc", "decouple_gnd"} {
		if !slices.Contains(output.Instance.Nets, net) {
			t.Fatalf("nets = %#v, want %s", output.Instance.Nets, net)
		}
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
	for _, operation := range output.Operations {
		if operation.Op == transactions.OpAddLabel {
			t.Fatalf("supply decoupling must not emit a standalone label: %#v", operation)
		}
	}
}

func TestAmplifierSupplyDecouplingInstantiatesDualSupply(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "amplifier_supply_decoupling",
		InstanceID: "dual",
		Params: map[string]any{
			"rail_mode":                "dual_supply",
			"rail_voltage":             "12V",
			"capacitor_voltage_rating": "25V",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if !slices.Contains(output.Instance.Nets, "dual_vee") {
		t.Fatalf("nets = %#v, want VEE decoupling", output.Instance.Nets)
	}
	definition, ok := registry.GetBlock("amplifier_supply_decoupling")
	if !ok {
		t.Fatal("amplifier supply decoupling definition is missing")
	}
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
	if len(realized.Issues) != 0 {
		t.Fatalf("PCB realization issues = %#v", realized.Issues)
	}
	if len(realized.EntryAnchors) != 3 || len(realized.LocalRoutes) != 8 {
		t.Fatalf("entry anchors/routes = %d/%d, want 3/8", len(realized.EntryAnchors), len(realized.LocalRoutes))
	}
	if len(definition.PCBRealization.PlacementGroups) != 1 || !definition.PCBRealization.PlacementGroups[0].TranslateAsUnit {
		t.Fatalf("placement groups = %#v, want rigid local decoupling star", definition.PCBRealization.PlacementGroups)
	}
	for _, route := range realized.LocalRoutes {
		if route.From.Ref == "" || route.To.Ref == "" {
			t.Fatalf("local route %s has unresolved endpoint: %#v", route.ID, route)
		}
		if !route.DisableEntryAnchorVia {
			t.Fatalf("local route %s must stay on the declared F.Cu entry layer without a redundant anchor via", route.ID)
		}
	}
	for _, routeID := range []string{"vcc_bulk", "vee_bulk"} {
		var route *PCBLocalRoute
		for index := range definition.PCBRealization.LocalRoutes {
			if definition.PCBRealization.LocalRoutes[index].ID == routeID {
				route = &definition.PCBRealization.LocalRoutes[index]
				break
			}
		}
		if route == nil || len(route.Waypoints) != 2 || route.Waypoints[0].YMM == 0 || route.Waypoints[0].YMM != route.Waypoints[1].YMM {
			t.Fatalf("bulk rail route %q = %#v, want a two-waypoint lane separated from capacitor return pads", routeID, route)
		}
	}
}

func TestAmplifierSupplyDecouplingBlocksUnderratedCapacitors(t *testing.T) {
	_, issues := NewBuiltinRegistry().Instantiate(context.Background(), BlockRequest{
		BlockID:    "amplifier_supply_decoupling",
		InstanceID: "bad_decouple",
		Params: map[string]any{
			"rail_voltage":             "12V",
			"capacitor_voltage_rating": "16V",
		},
	})
	if !reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v, want capacitor derating blocker", issues)
	}
}
