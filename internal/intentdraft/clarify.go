package intentdraft

import (
	"regexp"

	"kicadai/internal/intentplanner"
)

var unsupportedInterfacePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(?:ethernet|usb[- ]?data|controller area network)\b`),
	regexp.MustCompile(`\b(?:canbus|can[- ]?(?:adapter|bus|interface|transceiver|sensor|board|connector|controller|node|shield|breakout|port))\b`),
	regexp.MustCompile(`\bCAN(?:[- ]?)(?:adapter|bus|interface|transceiver|sensor|board|connector|controller|node|shield|breakout|port)\b`),
}

func clarifyDraft(source string, normalized string, request intentplanner.Request, extraction ExtractionReport) []Clarification {
	var clarifications []Clarification
	finder := newPhraseFinder(source)
	if containsAny(normalized, "battery") && !powerInputHasVoltage(request, "battery") {
		clarifications = append(clarifications, Clarification{
			ID:         "intent.power.battery_voltage_missing",
			Path:       "power.inputs",
			Severity:   ClarificationBlocking,
			Question:   "What battery chemistry or input voltage should be used?",
			Options:    []string{"3.7V Li-ion", "2xAA", "external regulated input"},
			Evidence:   []ExtractedField{finder.findPhrase("battery")},
			Suggestion: "Specify the battery voltage or chemistry.",
		})
	}
	if unsupportedInterfaceClarificationRequested(source, normalized) {
		clarifications = append(clarifications, Clarification{
			ID:         "intent.interface.kind_unsupported",
			Path:       "interfaces",
			Severity:   ClarificationBlocking,
			Question:   "The requested interface is not supported by the current intent planner. Which supported interface should be used?",
			Options:    []string{"i2c", "uart", "spi", "gpio"},
			Evidence:   []ExtractedField{finder.findFirstPhrase([]string{"can", "ethernet", "usb data"})},
			Suggestion: "Use a supported interface or wait for a block that implements this interface.",
		})
	}
	if containsAny(normalized, "headphone") {
		clarifications = append(clarifications, Clarification{
			ID:         "intent.function.headphone_amplifier_unverified",
			Path:       "functions",
			Severity:   ClarificationBlocking,
			Question:   "Headphone amplifier output-stage safety is not verified yet. Should this be treated as an op-amp gain-stage draft instead?",
			Options:    []string{"op-amp gain stage draft", "block until headphone amplifier block exists"},
			Evidence:   []ExtractedField{finder.findPhrase("headphone")},
			Suggestion: "Use an explicitly supported amplifier topology.",
		})
	}
	if containsAny(normalized, "fabrication", "fab ready", "manufacturable") {
		clarifications = append(clarifications, Clarification{
			ID:         "intent.acceptance.fabrication_requested_without_evidence",
			Path:       "manufacturing.fabrication_candidate",
			Severity:   ClarificationWarning,
			Question:   "Fabrication readiness still depends on downstream validation and local KiCad CLI evidence.",
			Evidence:   []ExtractedField{finder.findFirstPhrase([]string{"fabrication", "fab ready", "manufacturable"})},
			Suggestion: "Review fabrication readiness output before ordering boards.",
		})
	}
	if extraction.Confidence.Fields == 0 && source != "" {
		clarifications = append(clarifications, Clarification{
			ID:         "intent.function.family_ambiguous",
			Path:       "intent",
			Severity:   ClarificationBlocking,
			Question:   "What supported design family should be generated?",
			Options:    []string{"sensor breakout", "mcu minimal", "power module", "amplifier", "connector breakout"},
			Suggestion: "Name one supported circuit family and the required voltage/interface.",
		})
	}
	return clarifications
}

func unsupportedInterfaceClarificationRequested(source string, normalized string) bool {
	for _, pattern := range unsupportedInterfacePatterns[:2] {
		if pattern.MatchString(normalized) {
			return true
		}
	}
	return unsupportedInterfacePatterns[2].MatchString(source)
}

func BlockingClarifications(values []Clarification) bool {
	for _, value := range values {
		if value.Severity == ClarificationBlocking {
			return true
		}
	}
	return false
}

func powerInputHasVoltage(request intentplanner.Request, kind string) bool {
	for _, input := range request.Power.Inputs {
		if input.Kind == kind && input.Voltage != "" {
			return true
		}
	}
	return false
}
