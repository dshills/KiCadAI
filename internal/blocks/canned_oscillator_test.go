package blocks

import (
	"context"
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestCannedOscillatorInventoryAndDefinition(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("canned_oscillator")
	if !ok {
		t.Fatal("missing canned_oscillator")
	}
	if len(definition.Components) != 3 || len(definition.Ports) != 4 {
		t.Fatalf("definition = %#v", definition)
	}
	inventory := registry.Inventory()
	var family BlockFamilyInventory
	for _, candidate := range inventory.Families {
		if candidate.ID == "canned_oscillator" {
			family = candidate
			break
		}
	}
	if !family.Implemented || family.Readiness != BlockReadinessPartial {
		t.Fatalf("inventory family = %#v", family)
	}
	if !slices.Contains(family.RequiredRoles, "oscillator") || !slices.Contains(family.ExportedPorts, "CLK_OUT") {
		t.Fatalf("inventory family = %#v", family)
	}
}

func TestCannedOscillatorInstantiate(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("canned_oscillator")
	if !ok {
		t.Fatal("missing canned_oscillator")
	}
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "canned_oscillator", InstanceID: "clk1"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 3 || !slices.Contains(output.Instance.Nets, "clk1_clk_out") {
		t.Fatalf("output instance = %#v", output.Instance)
	}
	if countOperations(output.Operations, transactions.OpAddSymbol) != 3 || countOperations(output.Operations, transactions.OpConnect) != 8 {
		t.Fatalf("operations = %#v", output.Operations)
	}
	placements := placeFootprintOperations(t, output.Operations)
	if placements[output.Instance.Refs[1]].At.XMM != 4 || placements[output.Instance.Refs[1]].At.YMM != -2 || placements[output.Instance.Refs[2]].At.XMM != 4 {
		t.Fatalf("placements = %#v", placements)
	}
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{OriginXMM: 10, OriginYMM: 5})
	if reports.HasBlockingIssue(realized.Issues) {
		t.Fatalf("realize issues = %#v", realized.Issues)
	}
	if len(realized.Components) != 3 || len(realized.LocalRoutes) != 4 {
		t.Fatalf("realized = %#v", realized)
	}
	if len(realized.Timing) != 1 {
		t.Fatalf("timing evidence = %#v, want one fixture", realized.Timing)
	}
	timing := realized.Timing[0]
	if timing.ID != "canned_oscillator_core" || !timing.Satisfied || !timing.GroundReturnPresent || !timing.EnableControlPresent {
		t.Fatalf("timing evidence = %#v", timing)
	}
	if len(timing.DecouplingRefs) != 1 || len(timing.EnableControlRefs) != 1 || len(timing.DecouplingDistancesMM) != 1 {
		t.Fatalf("timing proximity evidence = %#v", timing)
	}
}

func TestOscillatorFourPinPinsMatch7050Pitch(t *testing.T) {
	pins := oscillatorFourPinPins()
	if pins[0].XMM != -2.54 || pins[0].YMM != -1.8 || pins[2].XMM != 2.54 || pins[2].YMM != 1.8 {
		t.Fatalf("pins = %#v", pins)
	}
}

func TestCannedOscillatorRejectsUnsupportedFrequency(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "canned_oscillator",
		InstanceID: "clk1",
		Params:     map[string]any{"frequency": "8MHz"},
	})
	if !reports.HasBlockingIssue(issues) || !hasBlockIssuePath(issues, "params.frequency") {
		t.Fatalf("issues = %#v, want unsupported frequency blocker", issues)
	}
}

func TestCannedOscillatorRejectsUnsupportedPinMapOverride(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "canned_oscillator",
		InstanceID: "clk1",
		Params: map[string]any{
			"oscillator_symbol":    "Oscillator:Other",
			"oscillator_footprint": "Oscillator:Other_Footprint",
		},
	})
	if !reports.HasBlockingIssue(issues) ||
		!hasBlockIssuePath(issues, "params.oscillator_symbol") ||
		!hasBlockIssuePath(issues, "params.oscillator_footprint") {
		t.Fatalf("issues = %#v, want unsupported symbol and footprint blockers", issues)
	}
}

func TestCannedOscillatorTimingFindsMissingDecoupling(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("canned_oscillator")
	if !ok {
		t.Fatal("missing canned_oscillator")
	}
	definition = cloneBlockDefinition(definition)
	definition.PCBRealization.Components = []PCBComponentRealization{
		definition.PCBRealization.Components[0],
		definition.PCBRealization.Components[2],
	}
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "canned_oscillator", InstanceID: "clk1"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
	if len(realized.Timing) != 1 || realized.Timing[0].Satisfied {
		t.Fatalf("timing evidence = %#v, want unsatisfied missing decoupling", realized.Timing)
	}
	if !hasTimingFinding(realized.Timing[0].Findings, TimingFindingDecouplingPresent) {
		t.Fatalf("findings = %#v, want missing decoupling", realized.Timing[0].Findings)
	}
}

func TestCannedOscillatorTimingFindsLongClockRoute(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("canned_oscillator")
	if !ok {
		t.Fatal("missing canned_oscillator")
	}
	definition = cloneBlockDefinition(definition)
	definition.PCBRealization.LocalRoutes = append(definition.PCBRealization.LocalRoutes, PCBLocalRoute{
		ID:          "osc_clock_test_route",
		NetTemplate: "clk_out",
		From:        RouteEndpoint{ComponentRole: "oscillator", Pin: "3"},
		To:          RouteEndpoint{ComponentRole: "decoupling", Pin: "1"},
		Waypoints:   []RelativePoint{{XMM: 100, YMM: 0}},
		Layer:       "F.Cu",
		WidthMM:     0.2,
		Required:    true,
	})
	definition.PCBRealization.TimingFixtures[0].LocalRouteIDs = append(definition.PCBRealization.TimingFixtures[0].LocalRouteIDs, "osc_clock_test_route")
	definition.PCBRealization.TimingFixtures[0].MaxClockRouteLengthMM = timingMM(10)
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "canned_oscillator", InstanceID: "clk1"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
	if len(realized.Timing) != 1 || realized.Timing[0].Satisfied {
		t.Fatalf("timing evidence = %#v, want unsatisfied long clock route", realized.Timing)
	}
	if !hasTimingFinding(realized.Timing[0].Findings, TimingFindingClockRoutesLength) {
		t.Fatalf("findings = %#v, want long clock route", realized.Timing[0].Findings)
	}
}

func hasTimingFinding(findings []TimingFixtureFinding, id string) bool {
	for _, finding := range findings {
		if finding.ID == id {
			return true
		}
	}
	return false
}
