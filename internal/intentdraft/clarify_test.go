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

func TestDraftStrictTurnsBlockingClarificationIntoIssue(t *testing.T) {
	result := Draft("battery powered sensor", Options{Strict: true})
	if len(result.Issues) == 0 {
		t.Fatalf("expected strict issue, result = %#v", result)
	}
}
