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
	if !hasKnownGap(plan, "mcu.clock.pin_assignment.clock") {
		t.Fatalf("missing MCU clock known gap: %#v", plan.KnownGaps)
	}
	if !hasKnownGap(plan, "mcu.programming.pin_assignment.programming") {
		t.Fatalf("missing MCU programming known gap: %#v", plan.KnownGaps)
	}
	if issues := designworkflow.ValidateRequest(*plan.GeneratedRequest); len(issues) != 0 {
		t.Fatalf("generated request validation issues = %#v", issues)
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
