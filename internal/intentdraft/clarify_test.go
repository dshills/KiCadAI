package intentdraft

import "testing"

func TestDraftClarifiesBatteryVoltage(t *testing.T) {
	result := Draft("battery powered sensor", Options{})
	if !BlockingClarifications(result.Clarifications) {
		t.Fatalf("clarifications = %#v", result.Clarifications)
	}
	if result.Clarifications[0].ID != "intent.power.battery_voltage_missing" {
		t.Fatalf("clarifications = %#v", result.Clarifications)
	}
}

func TestDraftClarifiesUnsupportedInterface(t *testing.T) {
	result := Draft("make a CAN sensor board with 5V input", Options{})
	if !BlockingClarifications(result.Clarifications) {
		t.Fatalf("clarifications = %#v", result.Clarifications)
	}
}

func TestDraftClarifiesHyphenatedUnsupportedInterface(t *testing.T) {
	result := Draft("make a usb-data adapter with 5V input", Options{})
	if !BlockingClarifications(result.Clarifications) {
		t.Fatalf("clarifications = %#v", result.Clarifications)
	}
}

func TestDraftDoesNotClarifyOrdinaryCanVerb(t *testing.T) {
	result := Draft("can you make an I2C temperature sensor board with 3.3V input", Options{})
	for _, clarification := range result.Clarifications {
		if clarification.ID == "intent.interface.kind_unsupported" {
			t.Fatalf("clarifications = %#v", result.Clarifications)
		}
	}
}

func TestDraftDoesNotClarifyUppercaseCanModalVerb(t *testing.T) {
	result := Draft("CAN I have an I2C temperature sensor board with 3.3V input", Options{})
	for _, clarification := range result.Clarifications {
		if clarification.ID == "intent.interface.kind_unsupported" {
			t.Fatalf("clarifications = %#v", result.Clarifications)
		}
	}
}

func TestDraftDoesNotClarifyCanInsideOtherWords(t *testing.T) {
	result := Draft("make an I2C toucan adapter with a 3.3V volcano sensor input", Options{})
	for _, clarification := range result.Clarifications {
		if clarification.ID == "intent.interface.kind_unsupported" {
			t.Fatalf("clarifications = %#v", result.Clarifications)
		}
	}
}

func TestDraftStrictTurnsBlockingClarificationIntoIssue(t *testing.T) {
	result := Draft("battery powered sensor", Options{Strict: true})
	if len(result.Issues) == 0 {
		t.Fatalf("expected strict issue, result = %#v", result)
	}
}
