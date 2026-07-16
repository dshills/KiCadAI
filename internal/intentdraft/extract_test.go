package intentdraft

import (
	"testing"

	"kicadai/internal/intentplanner"
	"kicadai/internal/reports"
)

func TestDraftExtractsI2CSensorBreakout(t *testing.T) {
	result := Draft("make a 3.3V I2C temperature sensor breakout board 50x30mm", Options{})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if result.Request.Kind != intentplanner.IntentSensorNode {
		t.Fatalf("kind = %q", result.Request.Kind)
	}
	if len(result.Request.Power.Inputs) != 0 {
		t.Fatalf("voltage-only breakout should not infer a separate power input: %#v", result.Request.Power.Inputs)
	}
	if len(result.Request.Power.Rails) != 1 || result.Request.Power.Rails[0].Voltage != "3.3V" {
		t.Fatalf("power rails = %#v", result.Request.Power.Rails)
	}
	if len(result.Request.Interfaces) != 1 || result.Request.Interfaces[0].Kind != "i2c" {
		t.Fatalf("interfaces = %#v", result.Request.Interfaces)
	}
	if len(result.Request.Functions) != 1 || result.Request.Functions[0].Kind != "sensor" {
		t.Fatalf("functions = %#v", result.Request.Functions)
	}
	if result.Request.Board.WidthMM != 50 || result.Request.Board.HeightMM != 30 {
		t.Fatalf("board = %#v", result.Request.Board)
	}
	if result.Extraction.Confidence.Fields == 0 {
		t.Fatalf("missing extraction confidence: %#v", result.Extraction)
	}
}

func TestDraftKeepsExplicitBreakoutPowerInput(t *testing.T) {
	for _, prompt := range []string{
		"make a 3.3V I2C sensor breakout with barrel jack power input",
		"make a 3.3V I2C sensor breakout with screw terminal power pins",
	} {
		result := Draft(prompt, Options{})
		if len(result.Request.Power.Inputs) == 0 {
			t.Fatalf("expected explicit breakout power input for %q", prompt)
		}
	}
}

func TestDraftExtractsMCUProgrammerClock(t *testing.T) {
	result := Draft("ATmega minimal board with ISP header and 16 MHz crystal", Options{})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if result.Request.Kind != intentplanner.IntentMCUMinimal {
		t.Fatalf("kind = %q", result.Request.Kind)
	}
	if len(result.Request.Functions) < 3 {
		t.Fatalf("functions = %#v", result.Request.Functions)
	}
	if result.Request.Functions[2].Kind != "clock" || result.Request.Functions[2].Family != "crystal_oscillator" {
		t.Fatalf("clock function = %#v", result.Request.Functions)
	}
}

func TestDraftExtractsESP32Family(t *testing.T) {
	result := Draft("ESP32 module with 3.3V power and I2C", Options{})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if result.Request.Kind != intentplanner.IntentMCUMinimal {
		t.Fatalf("kind = %q", result.Request.Kind)
	}
	if len(result.Request.Functions) == 0 || result.Request.Functions[0].Kind != "mcu" || result.Request.Functions[0].Family != "esp32" {
		t.Fatalf("functions = %#v", result.Request.Functions)
	}
	plan := intentplanner.Plan(result.Request)
	if reports.HasBlockingIssue(plan.Issues) {
		t.Fatalf("plan issues = %#v", plan.Issues)
	}
	found := false
	if plan.GeneratedRequest == nil {
		t.Fatalf("missing generated request: %#v", plan)
	}
	for _, block := range plan.GeneratedRequest.Blocks {
		if block.BlockID == "esp32_wroom_32e_minimal" {
			found = true
		}
	}
	if !found {
		t.Fatalf("planned blocks = %#v", plan.GeneratedRequest.Blocks)
	}
}

func TestDraftExtractsPowerModule(t *testing.T) {
	result := Draft("USB-C 5V to 3.3V regulator module with 2 layer board", Options{})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if result.Request.Kind != intentplanner.IntentPowerModule {
		t.Fatalf("kind = %q", result.Request.Kind)
	}
	if len(result.Request.Power.Inputs) == 0 || result.Request.Power.Inputs[0].Kind != "usb_c" {
		t.Fatalf("power inputs = %#v", result.Request.Power.Inputs)
	}
	if len(result.Request.Power.Rails) == 0 || result.Request.Power.Rails[0].Voltage != "3.3V" {
		t.Fatalf("power rails = %#v", result.Request.Power.Rails)
	}
}

func TestDraftExtractsLEDIndicatorDefaultSupply(t *testing.T) {
	result := Draft("make a simple LED indicator board", Options{})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if result.Request.Kind != intentplanner.IntentBreakout {
		t.Fatalf("kind = %q", result.Request.Kind)
	}
	if result.Request.Name != "led_indicator" {
		t.Fatalf("name = %q", result.Request.Name)
	}
	if len(result.Request.Functions) != 1 || result.Request.Functions[0].Kind != "indicator" {
		t.Fatalf("functions = %#v", result.Request.Functions)
	}
	if len(result.Request.Interfaces) != 1 || result.Request.Interfaces[0].Kind != "gpio" || result.Request.Interfaces[0].Voltage != "3.3V" {
		t.Fatalf("interfaces = %#v", result.Request.Interfaces)
	}
	if len(result.Request.Power.Inputs) != 1 || result.Request.Power.Inputs[0].Kind != "external" || result.Request.Power.Inputs[0].Voltage != "3.3V" {
		t.Fatalf("power inputs = %#v", result.Request.Power.Inputs)
	}
	if len(result.Request.Power.Rails) != 1 || result.Request.Power.Rails[0].Voltage != "3.3V" {
		t.Fatalf("power rails = %#v", result.Request.Power.Rails)
	}
}

func TestDraftKeepsUSBCPoweredLEDIndicatorPowerOnly(t *testing.T) {
	result := Draft("make a USB-C powered LED indicator board", Options{})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if len(result.Request.Power.Inputs) != 1 || result.Request.Power.Inputs[0].Kind != "usb_c" || result.Request.Power.Inputs[0].Voltage != "5V" {
		t.Fatalf("power inputs = %#v", result.Request.Power.Inputs)
	}
	if len(result.Request.Functions) != 1 || result.Request.Functions[0].Kind != "indicator" {
		t.Fatalf("functions = %#v", result.Request.Functions)
	}
	if len(result.Request.Interfaces) != 0 {
		t.Fatalf("powered-only LED should not infer a GPIO connector: %#v", result.Request.Interfaces)
	}
}

func TestDraftDoesNotTreatControlledAsLEDIndicator(t *testing.T) {
	result := Draft("make a controlled power supply with 5V input", Options{})
	for _, function := range result.Request.Functions {
		if function.Kind == "indicator" {
			t.Fatalf("unexpected indicator function = %#v", result.Request.Functions)
		}
	}
	for _, iface := range result.Request.Interfaces {
		if iface.Kind == "gpio" {
			t.Fatalf("unexpected gpio interface = %#v", result.Request.Interfaces)
		}
	}
}

func TestDraftExtractsClassABHeadphoneAmplifier(t *testing.T) {
	result := Draft("build a class AB headphone amplifier with gain 2 for 64 ohms headphones on 12V", Options{})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if result.Request.Kind != intentplanner.IntentAmplifier {
		t.Fatalf("kind = %q", result.Request.Kind)
	}
	if len(result.Request.Functions) != 1 {
		t.Fatalf("functions = %#v", result.Request.Functions)
	}
	function := result.Request.Functions[0]
	if function.Kind != "amplifier" || function.Family != "class_ab_headphone" {
		t.Fatalf("function = %#v", function)
	}
	if function.Params["load_kind"] != "headphone" || function.Params["load_impedance"] != "64Ω" || function.Params["supply_voltage"] != "12V" {
		t.Fatalf("params = %#v", function.Params)
	}
	if len(result.Clarifications) != 0 {
		t.Fatalf("unexpected clarifications = %#v", result.Clarifications)
	}
}
