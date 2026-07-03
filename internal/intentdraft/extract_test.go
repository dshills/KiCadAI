package intentdraft

import (
	"testing"

	"kicadai/internal/intentplanner"
)

func TestDraftExtractsI2CSensorBreakout(t *testing.T) {
	result := Draft("make a 3.3V I2C temperature sensor breakout board 50x30mm", Options{})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if result.Request.Kind != intentplanner.IntentSensorNode {
		t.Fatalf("kind = %q", result.Request.Kind)
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
