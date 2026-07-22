package intentplanner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"kicadai/internal/blocks"
	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestPlanMapsSensorBreakoutIntent(t *testing.T) {
	plan := Plan(Request{
		Version:    "0.1.0",
		Name:       "sensor_breakout",
		Kind:       IntentBreakout,
		Acceptance: designworkflow.AcceptanceConnectivity,
		Board:      BoardIntent{WidthMM: 50, HeightMM: 30, EdgeClearanceMM: 0.25, Layers: 2},
		Power: PowerIntent{
			Inputs: []PowerInputIntent{{Kind: "usb_c", Voltage: "5V"}},
			Rails:  []PowerRailIntent{{Name: "VCC", Voltage: "3.3V", CurrentMA: 100}},
		},
		Interfaces: []InterfaceIntent{{Kind: "i2c", Voltage: "3.3V"}},
		Functions:  []FunctionIntent{{Kind: "sensor", Family: "i2c_sensor"}},
	})
	if plan.Status == PlanStatusBlocked {
		t.Fatalf("plan blocked: %#v", plan.Issues)
	}
	if plan.GeneratedRequest == nil {
		t.Fatalf("GeneratedRequest missing")
	}
	if plan.GeneratedRequest.Board.EdgeClearanceMM != 0.25 {
		t.Fatalf("edge clearance = %v, want 0.25", plan.GeneratedRequest.Board.EdgeClearanceMM)
	}
	for _, blockID := range []string{"usb_c_power", "voltage_regulator", "i2c_sensor", "connector_breakout"} {
		if !hasWorkflowBlock(*plan.GeneratedRequest, blockID) {
			t.Fatalf("generated request missing block %s: %#v", blockID, plan.GeneratedRequest.Blocks)
		}
	}
	if !hasConnection(*plan.GeneratedRequest, "i2c_connector.SDA", "sensor.SDA") {
		t.Fatalf("missing sensor SDA connection: %#v", plan.GeneratedRequest.Connections)
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request validation issues = %#v", issues)
	}
	if !hasSynthesisDecisionType(plan, "topology") || !hasSynthesisDecisionType(plan, "bus_resolution") {
		t.Fatalf("missing synthesis decisions: %#v", plan.Synthesis.Decisions)
	}
	if !hasSynthesisConstraintKind(plan, "voltage") || !hasSynthesisConstraintKind(plan, "confidence") {
		t.Fatalf("missing synthesis constraints: %#v", plan.Synthesis.Constraints)
	}
	if !hasSynthesisCalculationKind(plan, "i2c_pullup") || !hasSynthesisCalculationKind(plan, "regulator_headroom") {
		t.Fatalf("missing synthesis calculations: %#v", plan.Synthesis.Calculations)
	}
	if got := workflowBlockParam(*plan.GeneratedRequest, "voltage_regulator", "regulator_symbol"); got != "Regulator_Linear:AP2112K-3.3" {
		t.Fatalf("regulator_symbol = %q; blocks=%#v", got, plan.GeneratedRequest.Blocks)
	}
	if got := workflowBlockParam(*plan.GeneratedRequest, "voltage_regulator", "enable_mode"); got != "tied_input" {
		t.Fatalf("enable_mode = %q; blocks=%#v", got, plan.GeneratedRequest.Blocks)
	}
	if got := synthesisCalculationResult(plan, "regulator_headroom", "variant"); got != "ap2112k_3v3_sot23_5" {
		t.Fatalf("regulator variant = %q; calculations=%#v", got, plan.Synthesis.Calculations)
	}
	if got := synthesisCalculationResult(plan, "regulator_headroom", "dropout_margin_required"); got != "0.5" {
		t.Fatalf("dropout margin = %q; calculations=%#v", got, plan.Synthesis.Calculations)
	}
	if got := synthesisCalculationResult(plan, "regulator_headroom", "capacitor_voltage_policy"); got != "minimum_125_percent_operating_voltage" {
		t.Fatalf("capacitor policy = %q; calculations=%#v", got, plan.Synthesis.Calculations)
	}
	if !synthesisCalculationRequirement(plan, "regulator_headroom", "regulator", "thermal_review") ||
		!synthesisCalculationRequirement(plan, "regulator_headroom", "regulator", "stability_review") ||
		!synthesisCalculationRequirement(plan, "regulator_headroom", "regulator.output_capacitor", "voltage_policy") {
		t.Fatalf("missing regulator review requirements: %#v", plan.Synthesis.Calculations)
	}
	for _, check := range []struct {
		key   string
		kind  string
		value string
		unit  string
	}{
		{key: "regulator.regulator", kind: "input_voltage", value: "5", unit: "V"},
		{key: "regulator.regulator", kind: "output_current", value: "0.1", unit: "A"},
		{key: "regulator.input_capacitor", kind: "voltage", value: "6.3", unit: "V"},
		{key: "regulator.output_capacitor", kind: "voltage", value: "6.3", unit: "V"},
	} {
		if !workflowRequiredRatingValue(*plan.GeneratedRequest, check.key, check.kind, check.value, check.unit) {
			t.Fatalf("missing required rating %s on %s: %#v", check.kind, check.key, plan.GeneratedRequest.Components.Overrides)
		}
	}
}

func TestPlanPreservesConcreteI2CSensorSelection(t *testing.T) {
	request := Request{
		Version:    RequestVersion,
		Name:       "concrete_humidity_sensor",
		Kind:       IntentBreakout,
		Acceptance: designworkflow.AcceptanceConnectivity,
		Power: PowerIntent{
			Inputs: []PowerInputIntent{{Kind: "usb_c", Voltage: "5V"}},
			Rails:  []PowerRailIntent{{Name: "VCC", Voltage: "3.3V", CurrentMA: 50}},
		},
		Interfaces: []InterfaceIntent{{Kind: "i2c", Voltage: "3.3V"}},
		Functions: []FunctionIntent{{
			Kind:   "sensor",
			Family: "i2c_sensor",
			Params: map[string]any{
				"sensor_component_id": "sensor.sensirion.sht31_dis.dfn8",
				"i2c_address":         "0x44",
				"supply_voltage":      "3.3V",
				"include_interrupt":   true,
			},
		}},
	}
	plan := Plan(request)
	if plan.Status != PlanStatusReady || plan.GeneratedRequest == nil {
		t.Fatalf("plan = %#v", plan)
	}
	for key, want := range map[string]any{
		"sensor_component_id": "sensor.sensirion.sht31_dis.dfn8",
		"i2c_address":         "0x44",
		"supply_voltage":      "3.3V",
		"include_interrupt":   "true",
	} {
		if got := workflowBlockParam(*plan.GeneratedRequest, "i2c_sensor", key); got != want {
			t.Fatalf("%s = %#v, want %#v", key, got, want)
		}
	}
}

func TestBMP280IntentClearsSchematicElectrical(t *testing.T) {
	file, err := os.Open("../../examples/intent/sensor_bmp280_breakout.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	request, issues := DecodeRequestStrict(file)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("decode issues = %#v", issues)
	}
	intentPlan := Plan(request)
	if intentPlan.Status != PlanStatusReady || intentPlan.GeneratedRequest == nil {
		t.Fatalf("intent plan = %#v", intentPlan)
	}
	if !hasConnectionWithNet(*intentPlan.GeneratedRequest, "regulator.VOUT", "i2c_connector.VCC", "VCC_3v3") {
		t.Fatalf("BMP280 connector VCC connection missing: %#v", intentPlan.GeneratedRequest.Connections)
	}
	if !intentPlan.GeneratedRequest.Constraints.AllowBackLayer {
		t.Fatalf("two-layer BMP280 workflow must allow B.Cu routing: %#v", intentPlan.GeneratedRequest.Constraints)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	registry := blocks.NewBuiltinRegistry()
	blockPlan := designworkflow.PlanBlocks(ctx, registry, *intentPlan.GeneratedRequest)
	if reports.HasBlockingIssue(blockPlan.Stage.Issues) {
		t.Fatalf("block plan issues = %#v", blockPlan.Stage.Issues)
	}
	selection := designworkflow.SelectWorkflowComponents(ctx, registry, blockPlan, designworkflow.ComponentSelectionOptions{})
	if reports.HasBlockingIssue(selection.Stage.Issues) {
		t.Fatalf("selection issues = %#v", selection.Stage.Issues)
	}
	if applyIssues := designworkflow.ApplyComponentSelectionsToPlan(&blockPlan, registry, selection.Selections); reports.HasBlockingIssue(applyIssues) {
		t.Fatalf("apply selection issues = %#v", applyIssues)
	}
	stage := designworkflow.SchematicElectricalStage(blockPlan)
	if stage.Status == designworkflow.StageStatusBlocked || reports.HasBlockingIssue(stage.Issues) {
		t.Fatalf("schematic electrical stage = %#v", stage)
	}
	fragments := designworkflow.RealizePCBFragments(ctx, registry, blockPlan)
	placed := designworkflow.PlaceFragments(ctx, *intentPlan.GeneratedRequest, fragments, designworkflow.PlacementOptions{ComponentSelections: selection.Selections})
	if reports.HasBlockingIssue(placed.Stage.Issues) {
		t.Fatalf("placement stage = %#v", placed.Stage)
	}
	sensorRef := ""
	regulatorRef := ""
	for _, fragment := range fragments.Fragments {
		switch fragment.InstanceID {
		case "sensor":
			sensorRef = fragment.Realization.RoleRefs["sensor"]
		case "regulator":
			regulatorRef = fragment.Realization.RoleRefs["regulator"]
		}
	}
	hasEndpoint := func(netName string, ref string, pin string) bool {
		for _, net := range placed.Request.Nets {
			if net.Name != netName {
				continue
			}
			for _, endpoint := range net.Endpoints {
				if endpoint.Ref == ref && endpoint.Pin == pin {
					return true
				}
			}
		}
		return false
	}
	if !hasEndpoint("VCC_3v3", sensorRef, "8") || !hasEndpoint("VCC_3v3", regulatorRef, "5") {
		t.Fatalf("concrete power pins missing from VCC_3v3: nets=%#v", placed.Request.Nets)
	}
	if hasEndpoint("GND", sensorRef, "8") || hasEndpoint("GND", regulatorRef, "5") {
		t.Fatalf("concrete power pins merged into GND: nets=%#v", placed.Request.Nets)
	}
	routed := designworkflow.RoutePlacement(ctx, *intentPlan.GeneratedRequest, fragments, placed, designworkflow.RoutingOptions{ComponentSelections: selection.Selections})
	if reports.HasBlockingIssue(routed.Stage.Issues) {
		t.Fatalf("routing stage = %#v", routed.Stage)
	}
	tx, txIssues := designworkflow.ProjectTransaction(intentPlan.GeneratedRequest, &blockPlan, &placed, &routed, true)
	if reports.HasBlockingIssue(txIssues) {
		t.Fatalf("transaction issues = %#v", txIssues)
	}
	validation := transactions.Validate(tx)
	if reports.HasBlockingIssue(validation.Issues) {
		bad := []transactions.Operation{}
		for _, index := range []int{90, 92} {
			if index < len(tx.Operations) {
				bad = append(bad, tx.Operations[index])
			}
		}
		t.Fatalf("transaction validation = %#v; suspect operations = %#v", validation.Issues, bad)
	}
}

func TestPlanPreservesExplicitBackLayerConstraint(t *testing.T) {
	disabled := false
	plan := Plan(Request{
		Version:    RequestVersion,
		Name:       "single_sided_breakout",
		Kind:       IntentBreakout,
		Acceptance: designworkflow.AcceptanceConnectivity,
		Board:      BoardIntent{Layers: 2},
		Constraints: ConstraintIntent{
			AllowBackLayer: &disabled,
		},
	})
	if plan.GeneratedRequest == nil {
		t.Fatalf("generated request missing: %#v", plan)
	}
	if plan.GeneratedRequest.Constraints.AllowBackLayer {
		t.Fatalf("explicit false allow_back_layer was not preserved: %#v", plan.GeneratedRequest.Constraints)
	}
}

func TestBMP280IntentMatchesKiCadBackedPassFixture(t *testing.T) {
	intentFile, err := os.Open("../../examples/intent/sensor_bmp280_breakout.json")
	if err != nil {
		t.Fatal(err)
	}
	defer intentFile.Close()
	request, issues := DecodeRequestStrict(intentFile)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("decode intent issues = %#v", issues)
	}
	plan := Plan(request)
	if plan.Status != PlanStatusReady || plan.GeneratedRequest == nil {
		t.Fatalf("intent plan = %#v", plan)
	}

	fixtureFile, err := os.Open("../../examples/design/kicad-backed/sensor_bmp280_breakout.json")
	if err != nil {
		t.Fatal(err)
	}
	defer fixtureFile.Close()
	fixture, fixtureIssues := designworkflow.DecodeRequestStrict(fixtureFile)
	if reports.HasBlockingIssue(fixtureIssues) {
		t.Fatalf("decode fixture issues = %#v", fixtureIssues)
	}
	generatedJSON, err := json.Marshal(plan.GeneratedRequest)
	if err != nil {
		t.Fatal(err)
	}
	fixtureJSON, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if string(generatedJSON) != string(fixtureJSON) {
		t.Fatalf("BMP280 KiCad-backed request drifted from structured intent\ngenerated: %s\nfixture: %s", generatedJSON, fixtureJSON)
	}
}

func TestPlanDoesNotSelectAP2112KAboveModeledCurrent(t *testing.T) {
	plan := Plan(Request{
		Version:    "0.1.0",
		Name:       "high_current_rail",
		Kind:       IntentPowerModule,
		Acceptance: designworkflow.AcceptanceConnectivity,
		Power: PowerIntent{
			Inputs: []PowerInputIntent{{Kind: "external", Voltage: "5V"}},
			Rails:  []PowerRailIntent{{Name: "VCC", Voltage: "3.3V", CurrentMA: 900}},
		},
	})
	if plan.GeneratedRequest == nil {
		t.Fatalf("GeneratedRequest missing")
	}
	if got := workflowBlockParam(*plan.GeneratedRequest, "voltage_regulator", "regulator_symbol"); got == "Regulator_Linear:AP2112K-3.3" {
		t.Fatalf("unexpected AP2112K selection for high-current rail")
	}
}

func TestPlanMapsMCUAndProtectionBlocks(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "protected_mcu",
		Kind:    IntentMCUMinimal,
		Board:   BoardIntent{WidthMM: 60, HeightMM: 40},
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "usb_c", Voltage: "5V"}}},
		Functions: []FunctionIntent{
			{Kind: "mcu", Params: map[string]any{"supply_voltage": "5V"}},
			{Kind: "clock", Family: "canned_oscillator", Params: map[string]any{"supply_voltage": "5V"}},
			{Kind: "reset_programming"},
		},
		Protection: ProtectionIntent{ESD: StrengthPreferred, ReversePolarity: StrengthPreferred},
	})
	for _, blockID := range []string{"mcu_minimal", "canned_oscillator", "reset_programming_header", "esd_protection", "reverse_polarity_protection"} {
		if !hasWorkflowBlock(*plan.GeneratedRequest, blockID) {
			t.Fatalf("generated request missing block %s: %#v", blockID, plan.GeneratedRequest.Blocks)
		}
	}
	for _, connection := range []struct {
		from string
		to   string
	}{
		{from: "reverse_polarity.VIN_PROTECTED", to: "mcu.VCC"},
	} {
		if !hasConnection(*plan.GeneratedRequest, connection.from, connection.to) {
			t.Fatalf("missing connection %s -> %s: %#v", connection.from, connection.to, plan.GeneratedRequest.Connections)
		}
	}
	if !hasKnownGap(plan, "mcu.clock.topology_unsupported.clock") {
		t.Fatalf("missing MCU clock known gap: %#v", plan.KnownGaps)
	}
	if hasKnownGap(plan, "mcu.programming.pin_assignment.programming") {
		t.Fatalf("unexpected MCU programming known gap: %#v", plan.KnownGaps)
	}
	for _, connection := range []struct {
		from string
		to   string
	}{
		{from: "mcu.MOSI", to: "programming.MOSI"},
		{from: "mcu.MISO", to: "programming.MISO"},
		{from: "mcu.SCK", to: "programming.SCK"},
		{from: "mcu.RESET", to: "programming.RESET"},
	} {
		if !hasConnection(*plan.GeneratedRequest, connection.from, connection.to) {
			t.Fatalf("missing programming connection %s -> %s: %#v", connection.from, connection.to, plan.GeneratedRequest.Connections)
		}
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request validation issues = %#v", issues)
	}
}

func TestPlanReportsExternalClockTopologyLimitation(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "external_clock",
		Kind:    IntentMCUMinimal,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "external", Voltage: "5V"}}},
		Functions: []FunctionIntent{
			{Kind: "mcu", Params: map[string]any{"supply_voltage": "5V"}},
			{Kind: "clock", Family: "crystal_oscillator"},
		},
	})
	if plan.Status == PlanStatusBlocked {
		t.Fatalf("plan blocked: %#v", plan.Issues)
	}
	if !hasKnownGap(plan, "mcu.clock.topology_unsupported.clock") {
		t.Fatalf("missing topology limitation: %#v", plan.KnownGaps)
	}
	if !hasSynthesisGapCategory(plan, "unsupported_peripheral") {
		t.Fatalf("missing synthesis clock gap: %#v", plan.Synthesis.Gaps)
	}
	if !hasSynthesisDecisionSelected(plan, "external_clock_blocked") {
		t.Fatalf("missing external clock topology gate: %#v", plan.Synthesis.Decisions)
	}
	if hasConnection(*plan.GeneratedRequest, "clock.XTAL1", "mcu.XTAL1") || hasConnection(*plan.GeneratedRequest, "clock.CLK_OUT", "mcu.XTAL1") {
		t.Fatalf("unexpected clock connection: %#v", plan.GeneratedRequest.Connections)
	}
}

func TestPlanMapsUARTProgrammingIntent(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "uart_programming",
		Kind:    IntentMCUMinimal,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "external", Voltage: "5V"}}},
		Functions: []FunctionIntent{
			{Kind: "mcu", Params: map[string]any{"supply_voltage": "5V", "programming_header": "uart"}},
			{Kind: "reset_programming", Params: map[string]any{"programming_interface": "uart"}},
		},
	})
	if plan.Status == PlanStatusBlocked {
		t.Fatalf("plan blocked: %#v", plan.Issues)
	}
	for _, connection := range []struct {
		from string
		to   string
	}{
		{from: "mcu.UART_TX", to: "programming.UART_RX"},
		{from: "mcu.UART_RX", to: "programming.UART_TX"},
	} {
		if !hasConnection(*plan.GeneratedRequest, connection.from, connection.to) {
			t.Fatalf("missing UART programming connection %s -> %s: %#v", connection.from, connection.to, plan.GeneratedRequest.Connections)
		}
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request validation issues = %#v", issues)
	}
	if !hasSynthesisDecisionSelected(plan, "mcu.UART_TX -> programming.UART_RX") {
		t.Fatalf("missing UART synthesis decision: %#v", plan.Synthesis.Decisions)
	}
}

func TestPlanMapsMCUI2CBus(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "mcu_i2c",
		Kind:    IntentSensorNode,
		Power: PowerIntent{
			Inputs: []PowerInputIntent{{Kind: "external", Voltage: "5V"}},
			Rails:  []PowerRailIntent{{Name: "VCC", Voltage: "3.3V"}},
		},
		Interfaces: []InterfaceIntent{{Kind: "i2c", Voltage: "3.3V", Bus: "i2c1"}},
		Functions: []FunctionIntent{
			{Kind: "mcu", Params: map[string]any{"supply_voltage": "3.3V"}},
			{Kind: "sensor", Family: "i2c_sensor", Bus: "i2c1"},
		},
	})
	if plan.Status == PlanStatusBlocked {
		t.Fatalf("plan blocked: %#v", plan.Issues)
	}
	for _, connection := range []struct {
		from string
		to   string
		net  string
	}{
		{from: "mcu.SDA", to: "sensor.SDA", net: "I2C1_SDA"},
		{from: "mcu.SCL", to: "sensor.SCL", net: "I2C1_SCL"},
		{from: "i2c_connector.SDA", to: "sensor.SDA", net: "I2C1_SDA"},
		{from: "i2c_connector.SCL", to: "sensor.SCL", net: "I2C1_SCL"},
	} {
		if !hasConnectionWithNet(*plan.GeneratedRequest, connection.from, connection.to, connection.net) {
			t.Fatalf("missing connection %s -> %s net %s: %#v", connection.from, connection.to, connection.net, plan.GeneratedRequest.Connections)
		}
	}
	if hasKnownGap(plan, "mcu.i2c.pin_assignment") {
		t.Fatalf("unexpected old I2C known gap: %#v", plan.KnownGaps)
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request validation issues = %#v", issues)
	}
}

func TestPlanUsesI2CConnectorAsVoltageOnlyBreakoutPowerSource(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "i2c_sensor_breakout",
		Kind:    IntentSensorNode,
		Power: PowerIntent{
			Rails: []PowerRailIntent{{Name: "VCC", Voltage: "3.3V", Alias: "3v3"}},
		},
		Interfaces: []InterfaceIntent{{Kind: "i2c", Voltage: "3.3V", Bus: "i2c1"}},
		Functions: []FunctionIntent{
			{Kind: "sensor", Family: "i2c_sensor", Bus: "i2c1", Supply: "3v3"},
		},
	})
	if plan.Status == PlanStatusBlocked {
		t.Fatalf("plan blocked: %#v", plan.Issues)
	}
	if hasWorkflowBlock(*plan.GeneratedRequest, "connector_breakout") && hasWorkflowInstance(*plan.GeneratedRequest, "power_header") {
		t.Fatalf("voltage-only breakout should not add a separate power header: %#v", plan.GeneratedRequest.Blocks)
	}
	if got := workflowBlockParam(*plan.GeneratedRequest, "connector_breakout", "pin_names"); fmt.Sprint(got) != "[VCC GND SDA SCL]" {
		t.Fatalf("i2c connector pin_names = %#v, want [VCC GND SDA SCL]", got)
	}
	for _, connection := range []struct {
		from string
		to   string
		net  string
	}{
		{from: "i2c_connector.VCC", to: "sensor.VCC", net: "VCC"},
		{from: "i2c_connector.GND", to: "sensor.GND", net: "GND"},
		{from: "i2c_connector.SDA", to: "sensor.SDA", net: "SDA"},
		{from: "i2c_connector.SCL", to: "sensor.SCL", net: "SCL"},
	} {
		if !hasConnectionWithNet(*plan.GeneratedRequest, connection.from, connection.to, connection.net) {
			t.Fatalf("missing connection %s -> %s net %s: %#v", connection.from, connection.to, connection.net, plan.GeneratedRequest.Connections)
		}
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request validation issues = %#v", issues)
	}
}

func TestPlanBlocksTargetedGPIOAssignment(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "targeted_gpio",
		Kind:    IntentMCUMinimal,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "external", Voltage: "5V"}}},
		Functions: []FunctionIntent{
			{Kind: "mcu", Params: map[string]any{"supply_voltage": "5V"}},
		},
		Interfaces: []InterfaceIntent{{Kind: "gpio", Voltage: "5V", Target: TargetRef{Role: "mcu"}}},
	})
	if plan.Status != PlanStatusBlocked {
		t.Fatalf("status = %s, want blocked; issues=%#v", plan.Status, plan.Issues)
	}
	if !hasIssuePath(plan.Issues, "interfaces[0].target") {
		t.Fatalf("missing GPIO target issue: %#v", plan.Issues)
	}
	if !hasSynthesisGapCategory(plan, "target_resolution") {
		t.Fatalf("missing synthesis target gap: %#v", plan.Synthesis.Gaps)
	}
}

func TestPlanRecordsValueCalculationResults(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "calculated_values",
		Kind:    IntentAmplifier,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "external", Voltage: "5V"}}},
		Functions: []FunctionIntent{
			{Kind: "indicator", Params: map[string]any{"supply_voltage": "5V", "led_forward_voltage": "2V", "led_current_ma": 10}},
			{Kind: "amplifier", Params: map[string]any{"gain": 11}},
		},
		Interfaces: []InterfaceIntent{{Kind: "gpio", Voltage: "5V"}},
	})
	if plan.Status == PlanStatusBlocked {
		t.Fatalf("plan blocked: %#v", plan.Issues)
	}
	if got := synthesisCalculationResult(plan, "led_resistor", "resistance_ohms"); got != "300" {
		t.Fatalf("LED resistor result = %q; calculations=%#v", got, plan.Synthesis.Calculations)
	}
	if got := workflowBlockParam(*plan.GeneratedRequest, "led_indicator", "resistor_value"); got != "300" {
		t.Fatalf("generated LED resistor param = %q; blocks=%#v", got, plan.GeneratedRequest.Blocks)
	}
	if got := workflowBlockParam(*plan.GeneratedRequest, "led_indicator", "led_current"); got != "10mA" {
		t.Fatalf("generated LED current param = %q; blocks=%#v", got, plan.GeneratedRequest.Blocks)
	}
	if status := synthesisCalculationStatus(plan, "led_resistor"); status != "applied" {
		t.Fatalf("LED resistor status = %q; calculations=%#v", status, plan.Synthesis.Calculations)
	}
	if !synthesisCalculationAppliedPath(plan, "led_resistor", "blocks.indicator.params.resistor_value") {
		t.Fatalf("missing LED applied value: %#v", plan.Synthesis.Calculations)
	}
	if !synthesisCalculationRequirement(plan, "led_resistor", "resistor", "power") {
		t.Fatalf("missing LED resistor power requirement: %#v", plan.Synthesis.Calculations)
	}
	if !workflowRequiredRating(*plan.GeneratedRequest, "indicator.resistor", "power") {
		t.Fatalf("missing LED resistor power rating override: %#v", plan.GeneratedRequest.Components.Overrides)
	}
	if !workflowRequiredRating(*plan.GeneratedRequest, "indicator.led", "forward_current") {
		t.Fatalf("missing LED forward_current rating override: %#v", plan.GeneratedRequest.Components.Overrides)
	}
	if got := synthesisCalculationResult(plan, "opamp_gain", "rf_over_rg"); got != "10.00" {
		t.Fatalf("opamp gain result = %q; calculations=%#v", got, plan.Synthesis.Calculations)
	}
}

func TestPlanDoesNotPowerDefaultActiveHighLEDVCCPort(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "led_indicator",
		Kind:    IntentBreakout,
		Power: PowerIntent{
			Inputs: []PowerInputIntent{{Kind: "external", Voltage: "3.3V"}},
			Rails:  []PowerRailIntent{{Name: "VCC", Voltage: "3.3V"}},
		},
		Interfaces: []InterfaceIntent{{Kind: "gpio", Voltage: "3.3V"}},
		Functions:  []FunctionIntent{{Kind: "indicator"}},
	})
	if plan.GeneratedRequest == nil {
		t.Fatalf("GeneratedRequest missing: status=%s issues=%#v", plan.Status, plan.Issues)
	}
	if hasConnection(*plan.GeneratedRequest, "power_header.VIN", "indicator.VCC") {
		t.Fatalf("default active-high LED should not get an unresolved VCC endpoint: %#v", plan.GeneratedRequest.Connections)
	}
	if !hasConnection(*plan.GeneratedRequest, "connector.SIG", "indicator.IN") {
		t.Fatalf("missing connector signal connection: %#v", plan.GeneratedRequest.Connections)
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request validation issues = %#v", issues)
	}
}

func TestPlanPowersExplicitActiveLowLEDVCCPort(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "active_low_led",
		Kind:    IntentBreakout,
		Power: PowerIntent{
			Inputs: []PowerInputIntent{{Kind: "external", Voltage: "3.3V"}},
			Rails:  []PowerRailIntent{{Name: "VCC", Voltage: "3.3V"}},
		},
		Interfaces: []InterfaceIntent{{Kind: "gpio", Voltage: "3.3V"}},
		Functions:  []FunctionIntent{{Kind: "indicator", Params: map[string]any{"active_high": false}}},
	})
	if plan.GeneratedRequest == nil {
		t.Fatalf("GeneratedRequest missing: status=%s issues=%#v", plan.Status, plan.Issues)
	}
	if !hasConnection(*plan.GeneratedRequest, "power_header.VIN", "indicator.VCC") {
		t.Fatalf("active-low LED should keep VCC power endpoint: %#v", plan.GeneratedRequest.Connections)
	}
	if !hasConnection(*plan.GeneratedRequest, "connector.SIG", "indicator.IN") {
		t.Fatalf("missing connector signal connection: %#v", plan.GeneratedRequest.Connections)
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request validation issues = %#v", issues)
	}
}

func TestPlanConnectsUSBCPoweredActiveHighLEDInput(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "usb_c_led_indicator",
		Kind:    IntentBreakout,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "usb_c", Voltage: "5V"}}},
		Functions: []FunctionIntent{
			{Kind: "indicator"},
		},
	})
	if plan.Status == PlanStatusBlocked {
		t.Fatalf("plan blocked: %#v", plan.Issues)
	}
	if plan.GeneratedRequest == nil {
		t.Fatalf("GeneratedRequest missing")
	}
	if !hasConnectionWithNet(*plan.GeneratedRequest, "usb_power.VBUS_OUT", "indicator.IN", "VCC_5v") {
		t.Fatalf("missing USB-C VBUS to LED input connection: %#v", plan.GeneratedRequest.Connections)
	}
	if !hasConnection(*plan.GeneratedRequest, "usb_power.GND", "indicator.GND") {
		t.Fatalf("missing USB-C ground to LED ground connection: %#v", plan.GeneratedRequest.Connections)
	}
	if hasWorkflowBlock(*plan.GeneratedRequest, "connector_breakout") {
		t.Fatalf("powered-only LED should not require a connector block: %#v", plan.GeneratedRequest.Blocks)
	}
	if params := workflowBlockParams(*plan.GeneratedRequest, "usb_power"); params["include_tvs"] != false || params["include_bulk_capacitor"] != false || params["include_power_led"] != false || params["include_fuse"] != false || params["shield_policy"] != "floating" {
		t.Fatalf("USB-C LED fixture should use minimal USB-C power params: %#v", params)
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request validation issues = %#v", issues)
	}
}

func TestPlanMapsProtectedUSBPowerRequirementsToExistingBlock(t *testing.T) {
	plan := Plan(Request{
		Version: RequestVersion,
		Name:    "protected_usb_power",
		Kind:    IntentBreakout,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "usb_c", Voltage: "5V"}}},
		Protection: ProtectionIntent{
			Overcurrent:     StrengthRequired,
			Transient:       StrengthPreferred,
			BulkCapacitance: StrengthRequired,
		},
	})
	if plan.Status == PlanStatusBlocked || plan.GeneratedRequest == nil {
		t.Fatalf("plan blocked: status=%s issues=%#v", plan.Status, plan.Issues)
	}
	params := workflowBlockParams(*plan.GeneratedRequest, "usb_power")
	for _, key := range []string{"include_fuse", "include_tvs", "include_bulk_capacitor"} {
		if params[key] != true {
			t.Fatalf("%s = %#v, want true; params=%#v", key, params[key], params)
		}
	}
	for _, id := range []string{"protection.overcurrent", "protection.transient", "protection.bulk_capacitance"} {
		if _, ok := requirementByID(plan, id); !ok {
			t.Fatalf("missing requirement %q in %#v", id, plan.Requirements)
		}
	}
}

func TestPlanMapsOptionalAndForbiddenUSBPowerProtectionToDisabled(t *testing.T) {
	plan := Plan(Request{
		Version: RequestVersion,
		Name:    "minimal_usb_power",
		Kind:    IntentBreakout,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "usb_c", Voltage: "5V"}}},
		Protection: ProtectionIntent{
			Overcurrent:     StrengthForbidden,
			Transient:       StrengthOptional,
			BulkCapacitance: StrengthForbidden,
		},
	})
	if plan.Status == PlanStatusBlocked || plan.GeneratedRequest == nil {
		t.Fatalf("plan blocked: status=%s issues=%#v", plan.Status, plan.Issues)
	}
	params := workflowBlockParams(*plan.GeneratedRequest, "usb_power")
	for _, key := range []string{"include_fuse", "include_tvs", "include_bulk_capacitor"} {
		if params[key] != false {
			t.Fatalf("%s = %#v, want false; params=%#v", key, params[key], params)
		}
	}
}

func TestPlanBlocksRequiredUSBPowerProtectionForUnsupportedNonUSBInput(t *testing.T) {
	plan := Plan(Request{
		Version:    RequestVersion,
		Name:       "unsupported_protection",
		Kind:       IntentBreakout,
		Power:      PowerIntent{Inputs: []PowerInputIntent{{Kind: "external", Voltage: "5V"}}},
		Protection: ProtectionIntent{Overcurrent: StrengthRequired},
	})
	if plan.Status != PlanStatusBlocked {
		t.Fatalf("status = %s, want blocked; issues=%#v", plan.Status, plan.Issues)
	}
	if !hasIssuePath(plan.Issues, "protection.overcurrent") {
		t.Fatalf("missing protection blocker: %#v", plan.Issues)
	}
}

func TestPlanConnectsUSBCPoweredActiveLowLEDInput(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "usb_c_active_low_led_indicator",
		Kind:    IntentBreakout,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "usb_c", Voltage: "5V"}}},
		Functions: []FunctionIntent{
			{Kind: "indicator", Params: map[string]any{"active_high": false}},
		},
	})
	if plan.Status == PlanStatusBlocked {
		t.Fatalf("plan blocked: %#v", plan.Issues)
	}
	if plan.GeneratedRequest == nil {
		t.Fatalf("GeneratedRequest missing")
	}
	if !hasConnectionWithNet(*plan.GeneratedRequest, "usb_power.VBUS_OUT", "indicator.VCC", "VCC_5v") {
		t.Fatalf("missing USB-C VBUS to active-low LED VCC connection: %#v", plan.GeneratedRequest.Connections)
	}
	if !hasConnectionWithNet(*plan.GeneratedRequest, "usb_power.GND", "indicator.IN", "GND") {
		t.Fatalf("missing USB-C ground to active-low LED input connection: %#v", plan.GeneratedRequest.Connections)
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request validation issues = %#v", issues)
	}
}

func TestPlanMapsClassABHeadphoneIntentToProtectedOutputPath(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "class_ab_headphone",
		Kind:    IntentAmplifier,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "external", Voltage: "9V"}}},
		Functions: []FunctionIntent{
			{Kind: "amplifier", Family: "class_ab_headphone", Params: map[string]any{"gain": 2, "load_impedance": "32Ω", "supply_voltage": "9V"}},
		},
		Interfaces: []InterfaceIntent{{Kind: "analog", Connector: "audio_input", Voltage: "1V"}},
	})
	if plan.GeneratedRequest == nil {
		t.Fatalf("GeneratedRequest missing: status=%s issues=%#v", plan.Status, plan.Issues)
	}
	for _, blockID := range []string{"amplifier_input_buffer", "opamp_gain_stage", "amplifier_supply_decoupling", "amplifier_bias_network", "class_ab_output_pair", "headphone_output_protection", "headphone_output_connector"} {
		if !hasWorkflowBlock(*plan.GeneratedRequest, blockID) {
			t.Fatalf("generated request missing block %s: %#v", blockID, plan.GeneratedRequest.Blocks)
		}
	}
	for _, connection := range []struct {
		from string
		to   string
	}{
		{"connector.SIG", "input_buffer.IN"},
		{"input_buffer.OUT", "amplifier.IN"},
		{"amplifier.OUT", "bias.DRIVER_OUT"},
		{"bias.BIAS_P", "output.BIAS_P"},
		{"bias.BIAS_N", "output.BIAS_N"},
		{"bias.AMP_OUT", "output.AMP_OUT"},
		{"output.AMP_OUT", "output_protection.AMP_OUT"},
		{"output_protection.HP_OUT", "headphones.HP_OUT"},
		{"output.LOAD_REF", "output_protection.LOAD_REF"},
		{"output_protection.LOAD_RET", "headphones.LOAD_RET"},
		{"output_protection.LOAD_REF", "headphones.LOAD_REF"},
		{"power_header.GND", "input_buffer.GND"},
		{"power_header.GND", "amplifier.GND"},
		{"power_header.GND", "supply_decoupling.GND"},
		{"power_header.GND", "bias.VEE"},
		{"power_header.GND", "output.VEE"},
		{"power_header.VIN", "input_buffer.VCC"},
		{"power_header.VIN", "amplifier.VCC"},
		{"power_header.VIN", "supply_decoupling.VCC"},
		{"power_header.VIN", "bias.VCC"},
		{"power_header.VIN", "output.VCC"},
	} {
		if !hasConnection(*plan.GeneratedRequest, connection.from, connection.to) {
			t.Fatalf("missing connection %s -> %s: %#v", connection.from, connection.to, plan.GeneratedRequest.Connections)
		}
	}
	if got := workflowBlockParam(*plan.GeneratedRequest, "headphone_output_protection", "nominal_load_ohms"); got != "32Ω" {
		t.Fatalf("nominal_load_ohms = %q", got)
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request validation issues = %#v", issues)
	}
}

func TestPlanBlocksUnsafeClassABHeadphoneIntent(t *testing.T) {
	for _, tt := range []struct {
		name   string
		params map[string]any
	}{
		{name: "speaker", params: map[string]any{"load_kind": "speaker", "load_impedance": "8Ω"}},
		{name: "bridge", params: map[string]any{"load_kind": "bridge", "load_impedance": "32Ω"}},
		{name: "unknown_load", params: map[string]any{"load_impedance": "unknown"}},
		{name: "output_power", params: map[string]any{"output_power_w": "1W", "load_impedance": "32Ω"}},
		{name: "bipolar_supply", params: map[string]any{"single_supply": false, "load_impedance": "32Ω"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			plan := Plan(Request{
				Version: "0.1.0",
				Name:    "blocked_" + tt.name,
				Kind:    IntentAmplifier,
				Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "external", Voltage: "9V"}}},
				Functions: []FunctionIntent{
					{Kind: "amplifier", Family: "class_ab_headphone", Params: tt.params},
				},
			})
			if plan.Status != PlanStatusBlocked {
				t.Fatalf("status = %s, want blocked; issues=%#v", plan.Status, plan.Issues)
			}
			if plan.GeneratedRequest != nil && hasWorkflowBlock(*plan.GeneratedRequest, "headphone_output_protection") {
				t.Fatalf("unsafe intent should not emit output protection path: %#v", plan.GeneratedRequest.Blocks)
			}
		})
	}
}

func TestPlanBlocksClassABHeadphoneIntentWithoutGroundSource(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "class_ab_headphone_no_ground",
		Kind:    IntentAmplifier,
		Functions: []FunctionIntent{
			{Kind: "amplifier", Family: "class_ab_headphone", Params: map[string]any{"load_impedance": "32Ω"}},
		},
	})
	if plan.Status != PlanStatusBlocked {
		t.Fatalf("status = %s, want blocked; issues=%#v", plan.Status, plan.Issues)
	}
	if !hasIssuePath(plan.Issues, "functions[0].power.ground") {
		t.Fatalf("missing ground issue: %#v", plan.Issues)
	}
}

func TestPlanMapsMultipleClassABHeadphoneIntentsWithDistinctOutputNets(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "dual_class_ab_headphone",
		Kind:    IntentAmplifier,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "external", Voltage: "9V"}}},
		Functions: []FunctionIntent{
			{Kind: "amplifier", Family: "class_ab_headphone", Quantity: 2, Params: map[string]any{"load_impedance": "32Ω"}},
		},
		Interfaces: []InterfaceIntent{
			{Kind: "analog", Connector: "audio_input", Voltage: "1V"},
			{Kind: "analog", Connector: "audio_input", Voltage: "1V"},
		},
	})
	if plan.GeneratedRequest == nil {
		t.Fatalf("GeneratedRequest missing: status=%s issues=%#v", plan.Status, plan.Issues)
	}
	if !hasConnectionWithNet(*plan.GeneratedRequest, "output.AMP_OUT", "output_protection.AMP_OUT", "AMP_OUT_DC_BIASED_output_protection") {
		t.Fatalf("missing first scoped output net: %#v", plan.GeneratedRequest.Connections)
	}
	if !hasConnectionWithNet(*plan.GeneratedRequest, "output_2.AMP_OUT", "output_protection_2.AMP_OUT", "AMP_OUT_DC_BIASED_output_protection_2") {
		t.Fatalf("missing second scoped output net: %#v", plan.GeneratedRequest.Connections)
	}
}

func TestPlanBlocksInvalidExplicitLEDCalculation(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "bad_led",
		Kind:    IntentBreakout,
		Functions: []FunctionIntent{
			{Kind: "indicator", Params: map[string]any{"supply_voltage": "2V", "led_forward_voltage": "3V", "led_current": "5mA"}},
		},
	})
	if plan.Status != PlanStatusBlocked {
		t.Fatalf("status = %s, want blocked; issues=%#v", plan.Status, plan.Issues)
	}
	if status := synthesisCalculationStatus(plan, "led_resistor"); status != "blocked" {
		t.Fatalf("LED resistor status = %q; calculations=%#v", status, plan.Synthesis.Calculations)
	}
}

func TestPlanPreservesFractionalLEDResistance(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "low_resistance_led",
		Kind:    IntentBreakout,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "external", Voltage: "3.3V"}}},
		Functions: []FunctionIntent{
			{Kind: "indicator", Params: map[string]any{"supply_voltage": "3.3V", "led_forward_voltage": "3.244V", "led_current": "10mA"}},
		},
	})
	if plan.Status == PlanStatusBlocked {
		t.Fatalf("plan blocked: %#v", plan.Issues)
	}
	if got := synthesisCalculationResult(plan, "led_resistor", "resistance_ohms"); got != "5.6" {
		t.Fatalf("LED resistor result = %q; calculations=%#v", got, plan.Synthesis.Calculations)
	}
}

func TestPlanAppliesI2CPullupValue(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "i2c_pullups",
		Kind:    IntentSensorNode,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "external", Voltage: "3.3V"}}},
		Functions: []FunctionIntent{
			{Kind: "sensor", Family: "i2c_sensor", Params: map[string]any{"supply_voltage": "3.3V", "bus_speed_hz": 400000}},
		},
		Interfaces: []InterfaceIntent{{Kind: "i2c", Voltage: "3.3V"}},
	})
	if plan.Status == PlanStatusBlocked {
		t.Fatalf("plan blocked: %#v", plan.Issues)
	}
	if got := workflowBlockParam(*plan.GeneratedRequest, "i2c_sensor", "pullup_value"); got != "2.2k" {
		t.Fatalf("pullup_value = %q; blocks=%#v", got, plan.GeneratedRequest.Blocks)
	}
	if status := synthesisCalculationStatus(plan, "i2c_pullup"); status != "applied" {
		t.Fatalf("I2C pull-up status = %q; calculations=%#v", status, plan.Synthesis.Calculations)
	}
	if !synthesisCalculationRequirement(plan, "i2c_pullup", "i2c_pullup", "voltage") {
		t.Fatalf("missing I2C voltage requirement: %#v", plan.Synthesis.Calculations)
	}
}

func TestPlanDefersExternalI2CPullups(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "external_i2c_pullups",
		Kind:    IntentSensorNode,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "external", Voltage: "3.3V"}}},
		Functions: []FunctionIntent{
			{Kind: "sensor", Family: "i2c_sensor", Params: map[string]any{"supply_voltage": "3.3V", "include_pullups": false}},
		},
		Interfaces: []InterfaceIntent{{Kind: "i2c", Voltage: "3.3V"}},
	})
	if plan.Status == PlanStatusBlocked {
		t.Fatalf("plan blocked: %#v", plan.Issues)
	}
	if got := workflowBlockParam(*plan.GeneratedRequest, "i2c_sensor", "pullup_value"); got != "" {
		t.Fatalf("pullup_value = %q, want deferred empty", got)
	}
	if status := synthesisCalculationStatus(plan, "i2c_pullup"); status != "deferred" {
		t.Fatalf("I2C pull-up status = %q; calculations=%#v", status, plan.Synthesis.Calculations)
	}
}

func TestPlanAppliesCrystalLoadCapacitorValue(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "crystal_load",
		Kind:    IntentMCUMinimal,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "external", Voltage: "5V"}}},
		Functions: []FunctionIntent{
			{Kind: "mcu", Params: map[string]any{"supply_voltage": "5V"}},
			{Kind: "clock", Family: "crystal_oscillator", Params: map[string]any{"frequency": "16MHz", "load_cap_pf": 18, "stray_cap_pf": 2}},
		},
	})
	if plan.Status == PlanStatusBlocked {
		t.Fatalf("plan blocked: %#v", plan.Issues)
	}
	if got := workflowBlockParam(*plan.GeneratedRequest, "crystal_oscillator", "load_capacitor_value"); got != "32pF" {
		t.Fatalf("load_capacitor_value = %q; blocks=%#v", got, plan.GeneratedRequest.Blocks)
	}
	if status := synthesisCalculationStatus(plan, "crystal_load_cap"); status != "applied" {
		t.Fatalf("crystal status = %q; calculations=%#v", status, plan.Synthesis.Calculations)
	}
}

func TestPlanBlocksInvalidRegulatorHeadroom(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "bad_regulator",
		Kind:    IntentPowerModule,
		Functions: []FunctionIntent{
			{Kind: "regulator", Params: map[string]any{"input_voltage": "3.3V", "output_voltage": "5V"}},
		},
	})
	if plan.Status != PlanStatusBlocked {
		t.Fatalf("status = %s, want blocked; issues=%#v", plan.Status, plan.Issues)
	}
	if status := synthesisCalculationStatus(plan, "regulator_headroom"); status != "blocked" {
		t.Fatalf("regulator status = %q; calculations=%#v", status, plan.Synthesis.Calculations)
	}
}

func TestPlanRecordsDeferredOpAmpGainRequirements(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "opamp_requirements",
		Kind:    IntentAmplifier,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "external", Voltage: "5V"}}},
		Functions: []FunctionIntent{
			{Kind: "amplifier", Params: map[string]any{"gain": 4, "supply_voltage": "5V"}},
		},
	})
	if plan.Status == PlanStatusBlocked {
		t.Fatalf("plan blocked: %#v", plan.Issues)
	}
	if status := synthesisCalculationStatus(plan, "opamp_gain"); status != "deferred" {
		t.Fatalf("opamp status = %q; calculations=%#v", status, plan.Synthesis.Calculations)
	}
	if !synthesisCalculationRequirement(plan, "opamp_gain", "opamp_feedback", "gain") {
		t.Fatalf("missing opamp gain requirement: %#v", plan.Synthesis.Calculations)
	}
	if !workflowRequiredRating(*plan.GeneratedRequest, "amplifier.opamp", "supply_voltage") {
		t.Fatalf("missing opamp supply voltage rating override: %#v", plan.GeneratedRequest.Components.Overrides)
	}
}

func TestPlanAllowsUnityGainOpAmp(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "unity_buffer",
		Kind:    IntentAmplifier,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "external", Voltage: "5V"}}},
		Functions: []FunctionIntent{
			{Kind: "amplifier", Params: map[string]any{"gain": 1}},
		},
	})
	if plan.Status == PlanStatusBlocked {
		t.Fatalf("plan blocked: %#v", plan.Issues)
	}
	if got := synthesisCalculationResult(plan, "opamp_gain", "rf_over_rg"); got != "0.00" {
		t.Fatalf("opamp gain result = %q; calculations=%#v", got, plan.Synthesis.Calculations)
	}
}

func synthesisCalculationResult(plan PlanResult, kind string, key string) string {
	for _, calculation := range plan.Synthesis.Calculations {
		if calculation.Kind == kind {
			return calculation.Result[key]
		}
	}
	return ""
}

func synthesisCalculationStatus(plan PlanResult, kind string) string {
	for _, calculation := range plan.Synthesis.Calculations {
		if calculation.Kind == kind {
			return calculation.Status
		}
	}
	return ""
}

func synthesisCalculationAppliedPath(plan PlanResult, kind string, path string) bool {
	for _, calculation := range plan.Synthesis.Calculations {
		if calculation.Kind != kind {
			continue
		}
		for _, applied := range calculation.Applied {
			if applied.Path == path {
				return true
			}
		}
	}
	return false
}

func synthesisCalculationRequirement(plan PlanResult, kind string, subject string, requirementKind string) bool {
	for _, calculation := range plan.Synthesis.Calculations {
		if calculation.Kind != kind {
			continue
		}
		for _, requirement := range calculation.Requirements {
			if requirement.Subject == subject && requirement.Kind == requirementKind {
				return true
			}
		}
	}
	return false
}

func TestPlanBlocksMultipleI2CBusesOnSingleMCU(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "mcu_two_i2c",
		Kind:    IntentSensorNode,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "external", Voltage: "5V"}}},
		Interfaces: []InterfaceIntent{
			{Kind: "i2c", Voltage: "5V", Bus: "i2c1"},
			{Kind: "i2c", Voltage: "5V", Bus: "i2c2"},
		},
		Functions: []FunctionIntent{
			{Kind: "mcu", Params: map[string]any{"supply_voltage": "5V"}},
			{Kind: "sensor", Family: "i2c_sensor", Bus: "i2c1", Params: map[string]any{"supply_voltage": "5V"}},
			{Kind: "sensor", Family: "i2c_sensor", Bus: "i2c2", Params: map[string]any{"supply_voltage": "5V"}},
		},
	})
	if plan.Status != PlanStatusBlocked {
		t.Fatalf("status = %s, want blocked; issues=%#v", plan.Status, plan.Issues)
	}
	if !hasIssuePath(plan.Issues, "interfaces.i2c2") {
		t.Fatalf("issues = %#v", plan.Issues)
	}
}

func TestPlanRecordsVoltageDomainEvidence(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "voltage_domain",
		Kind:    IntentSensorNode,
		Power: PowerIntent{
			Inputs: []PowerInputIntent{{Kind: "external", Voltage: "5V"}},
			Rails:  []PowerRailIntent{{Name: "VCC", Voltage: "3.3V", Alias: "3v3"}},
		},
		Functions: []FunctionIntent{
			{Kind: "sensor", Family: "i2c_sensor", Supply: "3v3"},
		},
	})
	if plan.Status == PlanStatusBlocked {
		t.Fatalf("plan blocked: %#v", plan.Issues)
	}
	requirement, ok := requirementByID(plan, "function.1")
	if !ok {
		t.Fatalf("missing function requirement: %#v", plan.Requirements)
	}
	for _, evidence := range []string{"supply:regulator.VOUT", "net:VCC_3v3"} {
		if !containsString(requirement.Evidence, evidence) {
			t.Fatalf("missing evidence %s in %#v", evidence, requirement.Evidence)
		}
	}
	if !hasConnectionWithNet(*plan.GeneratedRequest, "regulator.VOUT", "sensor.VCC", "VCC_3v3") {
		t.Fatalf("missing sensor supply connection: %#v", plan.GeneratedRequest.Connections)
	}
}

func TestPlanResolvesSupplyByRailNameAndBlocksUnknownSupply(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "rail_name_supply",
		Kind:    IntentSensorNode,
		Power: PowerIntent{
			Inputs: []PowerInputIntent{{Kind: "external", Voltage: "5V"}},
			Rails:  []PowerRailIntent{{Name: "VCC", Voltage: "3.3V"}},
		},
		Functions: []FunctionIntent{{Kind: "sensor", Family: "i2c_sensor", Supply: "vcc"}},
	})
	if plan.Status == PlanStatusBlocked {
		t.Fatalf("plan blocked: %#v", plan.Issues)
	}
	if !hasConnectionWithNet(*plan.GeneratedRequest, "regulator.VOUT", "sensor.VCC", "VCC_3v3") {
		t.Fatalf("missing rail-name supply connection: %#v", plan.GeneratedRequest.Connections)
	}

	blocked := Plan(Request{
		Version: "0.1.0",
		Name:    "bad_supply",
		Kind:    IntentSensorNode,
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "external", Voltage: "5V"}}},
		Functions: []FunctionIntent{{
			Kind:   "sensor",
			Family: "i2c_sensor",
			Supply: "unknown_rail",
			Params: map[string]any{"supply_voltage": "5V"},
		}},
	})
	if blocked.Status != PlanStatusBlocked {
		t.Fatalf("status = %s, want blocked for unknown explicit supply; issues=%#v", blocked.Status, blocked.Issues)
	}
	if !hasSynthesisGapCategory(blocked, "voltage_domain") {
		t.Fatalf("missing synthesis voltage-domain gap: %#v", blocked.Synthesis.Gaps)
	}
	if hasConnection(*blocked.GeneratedRequest, "power_header.VIN", "sensor.VCC") {
		t.Fatalf("unexpected fallback supply connection: %#v", blocked.GeneratedRequest.Connections)
	}
}

func TestPlanBlocksConflictingSupplyAliasVoltage(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "bad_supply_voltage",
		Kind:    IntentSensorNode,
		Power: PowerIntent{
			Inputs: []PowerInputIntent{{Kind: "external", Voltage: "5V"}},
			Rails:  []PowerRailIntent{{Name: "VCC", Voltage: "3.3V", Alias: "3v3"}},
		},
		Functions: []FunctionIntent{{
			Kind:   "sensor",
			Family: "i2c_sensor",
			Supply: "3v3",
			Params: map[string]any{"supply_voltage": "5V"},
		}},
	})
	if plan.Status != PlanStatusBlocked {
		t.Fatalf("status = %s, want blocked; issues=%#v", plan.Status, plan.Issues)
	}
	if !hasIssuePath(plan.Issues, "blocks.sensor.supply_voltage") {
		t.Fatalf("missing conflict issue: %#v", plan.Issues)
	}
	if !hasSynthesisGapCategory(plan, "voltage_domain") {
		t.Fatalf("missing synthesis voltage-domain gap: %#v", plan.Synthesis.Gaps)
	}
}

func TestPlanBlocksUnsupportedRequiredFunction(t *testing.T) {
	plan := Plan(Request{
		Version:   "0.1.0",
		Name:      "radio",
		Kind:      IntentCustomStructured,
		Board:     BoardIntent{WidthMM: 30, HeightMM: 20},
		Functions: []FunctionIntent{{Kind: "sensor", Family: "rf_sensor", Strength: StrengthRequired}},
	})
	if plan.Status != PlanStatusBlocked {
		t.Fatalf("status = %s, want blocked; issues=%#v", plan.Status, plan.Issues)
	}
	if len(plan.Clarifications) == 0 {
		t.Fatalf("clarifications missing")
	}
}

func TestPlanUsesStableInstanceIDs(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "stable",
		Kind:    IntentBreakout,
		Board:   BoardIntent{WidthMM: 40, HeightMM: 20},
		Functions: []FunctionIntent{
			{Kind: "indicator"},
			{Kind: "indicator"},
		},
	})
	var ids []string
	for _, block := range plan.GeneratedRequest.Blocks {
		if block.BlockID == "led_indicator" {
			ids = append(ids, block.ID)
		}
	}
	if !equalStrings(ids, []string{"indicator", "indicator_2"}) {
		t.Fatalf("ids = %#v", ids)
	}
}

func TestPlanConnectsRepeatedSensors(t *testing.T) {
	plan := Plan(Request{
		Version:    "0.1.0",
		Name:       "two_sensors",
		Kind:       IntentBreakout,
		Acceptance: designworkflow.AcceptanceConnectivity,
		Board:      BoardIntent{WidthMM: 45, HeightMM: 30, Layers: 2},
		Power:      PowerIntent{Inputs: []PowerInputIntent{{Kind: "usb_c", Voltage: "5V"}}},
		Interfaces: []InterfaceIntent{{Kind: "i2c", Voltage: "3.3V"}},
		Functions:  []FunctionIntent{{Kind: "sensor", Family: "i2c_sensor", Quantity: 2}},
	})
	if plan.GeneratedRequest == nil {
		t.Fatalf("GeneratedRequest missing: status=%s issues=%#v", plan.Status, plan.Issues)
	}
	for _, connection := range []struct {
		from string
		to   string
	}{
		{from: "i2c_connector.SDA", to: "sensor.SDA"},
		{from: "i2c_connector.SCL", to: "sensor.SCL"},
		{from: "i2c_connector.SDA", to: "sensor_2.SDA"},
		{from: "i2c_connector.SCL", to: "sensor_2.SCL"},
	} {
		if !hasConnection(*plan.GeneratedRequest, connection.from, connection.to) {
			t.Fatalf("missing connection %s -> %s: %#v", connection.from, connection.to, plan.GeneratedRequest.Connections)
		}
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request validation issues = %#v", issues)
	}
}

func TestPlanConnectsHeaderPowerInput(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "header_powered",
		Kind:    IntentBreakout,
		Board:   BoardIntent{WidthMM: 45, HeightMM: 30, Layers: 2},
		Power: PowerIntent{
			Inputs: []PowerInputIntent{{Kind: "header", Voltage: "5V"}},
			Rails:  []PowerRailIntent{{Name: "VCC", Voltage: "3.3V"}},
		},
		Functions: []FunctionIntent{{Kind: "sensor", Family: "i2c_sensor"}},
		Interfaces: []InterfaceIntent{{
			Kind:    "i2c",
			Voltage: "3.3V",
		}},
	})
	if plan.GeneratedRequest == nil {
		t.Fatalf("GeneratedRequest missing: status=%s issues=%#v", plan.Status, plan.Issues)
	}
	if !hasConnection(*plan.GeneratedRequest, "power_header.VIN", "regulator.VIN") {
		t.Fatalf("missing header VIN to regulator connection: %#v", plan.GeneratedRequest.Connections)
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request validation issues = %#v", issues)
	}
}

func TestPlanDoesNotRegulateWhenSecondaryInputMatchesRail(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "matched_secondary_input",
		Kind:    IntentBreakout,
		Board:   BoardIntent{WidthMM: 45, HeightMM: 30, Layers: 2},
		Power: PowerIntent{
			Inputs: []PowerInputIntent{
				{Kind: "usb_c", Voltage: "5V"},
				{Kind: "header", Voltage: "3.3V"},
			},
			Rails: []PowerRailIntent{{Name: "VCC", Voltage: "3.3V"}},
		},
		Functions: []FunctionIntent{{Kind: "sensor", Family: "i2c_sensor"}},
		Interfaces: []InterfaceIntent{{
			Kind:    "i2c",
			Voltage: "3.3V",
		}},
	})
	if plan.GeneratedRequest == nil {
		t.Fatalf("GeneratedRequest missing: status=%s issues=%#v", plan.Status, plan.Issues)
	}
	if hasWorkflowBlock(*plan.GeneratedRequest, "voltage_regulator") {
		t.Fatalf("unexpected regulator for rail supplied by matching input: %#v", plan.GeneratedRequest.Blocks)
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request validation issues = %#v", issues)
	}
}

func TestPlanDoesNotShortRepeatedSignalConsumers(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "two_indicators",
		Kind:    IntentBreakout,
		Board:   BoardIntent{WidthMM: 45, HeightMM: 30, Layers: 2},
		Power:   PowerIntent{Inputs: []PowerInputIntent{{Kind: "usb_c", Voltage: "5V"}}},
		Interfaces: []InterfaceIntent{{
			Kind:     "gpio",
			Voltage:  "3.3V",
			Quantity: 2,
		}},
		Functions: []FunctionIntent{{Kind: "indicator", Quantity: 2}},
	})
	if plan.GeneratedRequest == nil {
		t.Fatalf("GeneratedRequest missing: status=%s issues=%#v", plan.Status, plan.Issues)
	}
	if !hasConnection(*plan.GeneratedRequest, "connector.SIG", "indicator.IN") {
		t.Fatalf("missing first indicator signal connection: %#v", plan.GeneratedRequest.Connections)
	}
	if !hasConnection(*plan.GeneratedRequest, "connector_2.SIG", "indicator_2.IN") {
		t.Fatalf("missing second indicator signal connection: %#v", plan.GeneratedRequest.Connections)
	}
	if hasConnection(*plan.GeneratedRequest, "connector.SIG", "indicator_2.IN") {
		t.Fatalf("second indicator was tied to first connector signal: %#v", plan.GeneratedRequest.Connections)
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request validation issues = %#v", issues)
	}
}

func TestPlanDerivesDraftComponentPolicy(t *testing.T) {
	plan := Plan(Request{
		Version:    "0.1.0",
		Name:       "draft_policy",
		Kind:       IntentBreakout,
		Acceptance: designworkflow.AcceptanceDraft,
		Board:      BoardIntent{WidthMM: 30, HeightMM: 20, Layers: 2},
		Constraints: ConstraintIntent{
			AllowPlaceholders: true,
			SkipRouting:       true,
		},
		Functions: []FunctionIntent{{Kind: "connector"}},
	})
	if plan.GeneratedRequest == nil {
		t.Fatalf("GeneratedRequest missing: status=%s issues=%#v", plan.Status, plan.Issues)
	}
	if got := plan.GeneratedRequest.Components.MinimumConfidence; got != "placeholder" {
		t.Fatalf("minimum confidence = %q, want placeholder", got)
	}
	if !plan.GeneratedRequest.Validation.SkipRouting {
		t.Fatalf("SkipRouting not derived: %#v", plan.GeneratedRequest.Validation)
	}
}

func TestVoltageNetTokenDoesNotDuplicateDecimalVoltageUnit(t *testing.T) {
	tests := map[string]string{
		"3.3V": "3v3",
		"3.3":  "3v3",
		"5V":   "5v",
	}
	for input, want := range tests {
		if got := voltageNetToken(input); got != want {
			t.Errorf("voltageNetToken(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPlanDerivesFabricationValidationPolicy(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "fab_policy",
		Kind:    IntentBreakout,
		Board:   BoardIntent{WidthMM: 30, HeightMM: 20, Layers: 2},
		Manufacturing: ManufacturingIntent{
			FabricationCandidate: true,
			Profile:              "jlcpcb-2layer",
		},
		Constraints: ConstraintIntent{
			RouteWidthMM: 0.25,
			ClearanceMM:  0.2,
		},
		Functions: []FunctionIntent{{Kind: "connector"}},
	})
	if plan.GeneratedRequest == nil {
		t.Fatalf("GeneratedRequest missing: status=%s issues=%#v", plan.Status, plan.Issues)
	}
	validation := plan.GeneratedRequest.Validation
	if validation.Acceptance != designworkflow.AcceptanceFabricationCandidate || !validation.RequireERC || !validation.RequireDRC || !validation.StrictUnrouted || !validation.StrictZones {
		t.Fatalf("validation = %#v", validation)
	}
	if !plan.GeneratedRequest.RoutingRetry.Enabled {
		t.Fatalf("routing retry not enabled: %#v", plan.GeneratedRequest.RoutingRetry)
	}
	if plan.GeneratedRequest.Components.MinimumConfidence != "verified" {
		t.Fatalf("component confidence = %q", plan.GeneratedRequest.Components.MinimumConfidence)
	}
	if plan.GeneratedRequest.Constraints.RouteWidthMM != 0.25 || plan.GeneratedRequest.Constraints.ClearanceMM != 0.2 {
		t.Fatalf("constraints = %#v", plan.GeneratedRequest.Constraints)
	}
	if !hasKnownGap(plan, "manufacturing.profile") {
		t.Fatalf("manufacturing profile gap missing: %#v", plan.KnownGaps)
	}
}

func TestPlanEnablesPlacementCorrectionForConnectivityIntent(t *testing.T) {
	plan := Plan(Request{
		Version:    "0.1.0",
		Name:       "connectivity_policy",
		Kind:       IntentBreakout,
		Acceptance: designworkflow.AcceptanceConnectivity,
		Board:      BoardIntent{WidthMM: 30, HeightMM: 20, Layers: 2},
		Functions:  []FunctionIntent{{Kind: "connector"}},
	})
	if plan.GeneratedRequest == nil {
		t.Fatalf("GeneratedRequest missing: status=%s issues=%#v", plan.Status, plan.Issues)
	}
	if !plan.GeneratedRequest.RoutingRetry.Enabled || plan.GeneratedRequest.RoutingRetry.MaxAttempts != 2 {
		t.Fatalf("routing retry = %#v", plan.GeneratedRequest.RoutingRetry)
	}
	if plan.GeneratedRequest.RoutingRetry.PreserveFixed {
		t.Fatalf("connectivity placement unexpectedly frozen: %#v", plan.GeneratedRequest.RoutingRetry)
	}
}

func TestPlanDerivesPackagePreferencesAndRatings(t *testing.T) {
	plan := Plan(Request{
		Version: "0.1.0",
		Name:    "ratings_policy",
		Kind:    IntentBreakout,
		Board:   BoardIntent{WidthMM: 30, HeightMM: 20, Layers: 2},
		Power: PowerIntent{
			Inputs: []PowerInputIntent{{Kind: "usb_c", Voltage: "5V", CurrentMA: 500}},
			Rails:  []PowerRailIntent{{Name: "VCC", Voltage: "3.3V", CurrentMA: 100}},
		},
		Constraints: ConstraintIntent{
			PackagePreferences: map[string]string{"resistor": "0603"},
		},
		Functions: []FunctionIntent{{Kind: "sensor", Family: "i2c_sensor"}},
		Interfaces: []InterfaceIntent{{
			Kind:    "i2c",
			Voltage: "3.3V",
		}},
	})
	if plan.GeneratedRequest == nil {
		t.Fatalf("GeneratedRequest missing: status=%s issues=%#v", plan.Status, plan.Issues)
	}
	if got := plan.GeneratedRequest.Components.PackagePreferences["resistor"]; got != "0603" {
		t.Fatalf("package preference = %q", got)
	}
	if !hasSynthesisConstraintSubject(plan, "resistor", "package", "0603") {
		t.Fatalf("missing package synthesis constraint: %#v", plan.Synthesis.Constraints)
	}
	if !hasSynthesisConstraintKind(plan, "current") {
		t.Fatalf("missing current/rating synthesis constraint: %#v", plan.Synthesis.Constraints)
	}
	note, ok := noteByID(plan.Assumptions, "constraints.component_policy")
	if !ok || !strings.Contains(note.Message, "input_voltage:5V") || !strings.Contains(note.Message, "rail_current:100mA") {
		t.Fatalf("component policy note = %#v", note)
	}
}

func hasWorkflowBlock(request designworkflow.Request, blockID string) bool {
	for _, block := range request.Blocks {
		if block.BlockID == blockID {
			return true
		}
	}
	return false
}

func hasWorkflowInstance(request designworkflow.Request, instanceID string) bool {
	for _, block := range request.Blocks {
		if block.ID == instanceID {
			return true
		}
	}
	return false
}

func workflowBlockParams(request designworkflow.Request, instanceID string) map[string]any {
	for _, block := range request.Blocks {
		if block.ID == instanceID {
			return block.Params
		}
	}
	return nil
}

func workflowBlockParam(request designworkflow.Request, blockID string, key string) string {
	for _, block := range request.Blocks {
		if block.BlockID != blockID {
			continue
		}
		if value, ok := block.Params[key]; ok {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func workflowRequiredRating(request designworkflow.Request, key string, kind string) bool {
	override, ok := request.Components.Overrides[key]
	if !ok {
		return false
	}
	for _, rating := range override.RequiredRatings {
		if rating.Kind == kind {
			return true
		}
	}
	return false
}

func workflowRequiredRatingValue(request designworkflow.Request, key string, kind string, value string, unit string) bool {
	override, ok := request.Components.Overrides[key]
	if !ok {
		return false
	}
	for _, rating := range override.RequiredRatings {
		if rating.Kind == kind && rating.Value == value && rating.Unit == unit {
			return true
		}
	}
	return false
}

func hasConnection(request designworkflow.Request, from string, to string) bool {
	for _, connection := range request.Connections {
		if connection.From == from && connection.To == to {
			return true
		}
	}
	return false
}

func hasConnectionWithNet(request designworkflow.Request, from string, to string, net string) bool {
	for _, connection := range request.Connections {
		if connection.From == from && connection.To == to && connection.NetAlias == net {
			return true
		}
	}
	return false
}

func hasKnownGap(plan PlanResult, id string) bool {
	for _, gap := range plan.KnownGaps {
		if gap.ID == id {
			return true
		}
	}
	return false
}

func hasSynthesisDecisionType(plan PlanResult, kind string) bool {
	for _, decision := range plan.Synthesis.Decisions {
		if decision.Type == kind {
			return true
		}
	}
	return false
}

func hasSynthesisDecisionSelected(plan PlanResult, selected string) bool {
	for _, decision := range plan.Synthesis.Decisions {
		if decision.Selected == selected {
			return true
		}
	}
	return false
}

func hasSynthesisConstraintKind(plan PlanResult, kind string) bool {
	for _, constraint := range plan.Synthesis.Constraints {
		if constraint.Kind == kind {
			return true
		}
	}
	return false
}

func hasSynthesisConstraintSubject(plan PlanResult, subject string, kind string, value string) bool {
	for _, constraint := range plan.Synthesis.Constraints {
		if constraint.Subject == subject && constraint.Kind == kind && constraint.Value == value {
			return true
		}
	}
	return false
}

func hasSynthesisCalculationKind(plan PlanResult, kind string) bool {
	for _, calculation := range plan.Synthesis.Calculations {
		if calculation.Kind == kind {
			return true
		}
	}
	return false
}

func hasSynthesisGapCategory(plan PlanResult, category string) bool {
	for _, gap := range plan.Synthesis.Gaps {
		if gap.Category == category {
			return true
		}
	}
	return false
}

func requirementByID(plan PlanResult, id string) (RequirementRecord, bool) {
	for _, requirement := range plan.Requirements {
		if requirement.ID == id {
			return requirement, true
		}
	}
	return RequirementRecord{}, false
}

func containsString(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}

func equalStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func noteByID(notes []PlanNote, id string) (PlanNote, bool) {
	for _, note := range notes {
		if note.ID == id {
			return note, true
		}
	}
	return PlanNote{}, false
}
