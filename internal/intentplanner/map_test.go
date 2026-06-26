package intentplanner

import (
	"strings"
	"testing"

	"kicadai/internal/designworkflow"
)

func TestPlanMapsSensorBreakoutIntent(t *testing.T) {
	plan := Plan(Request{
		Version:    "0.1.0",
		Name:       "sensor_breakout",
		Kind:       IntentBreakout,
		Acceptance: designworkflow.AcceptanceConnectivity,
		Board:      BoardIntent{WidthMM: 50, HeightMM: 30, Layers: 2},
		Power: PowerIntent{
			Inputs: []PowerInputIntent{{Kind: "usb_c", Voltage: "5V"}},
			Rails:  []PowerRailIntent{{Name: "VCC", Voltage: "3.3V"}},
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
	if got := synthesisCalculationResult(plan, "opamp_gain", "rf_over_rg"); got != "10.00" {
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
	for _, evidence := range []string{"supply:regulator.VOUT", "net:VCC_3v3v"} {
		if !containsString(requirement.Evidence, evidence) {
			t.Fatalf("missing evidence %s in %#v", evidence, requirement.Evidence)
		}
	}
	if !hasConnectionWithNet(*plan.GeneratedRequest, "regulator.VOUT", "sensor.VCC", "VCC_3v3v") {
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
	if !hasConnectionWithNet(*plan.GeneratedRequest, "regulator.VOUT", "sensor.VCC", "VCC_3v3v") {
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
