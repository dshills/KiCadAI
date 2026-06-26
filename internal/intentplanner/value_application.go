package intentplanner

import (
	"math"
	"strconv"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
)

type valueApplicationRule struct {
	BlockID         string
	CalculationKind string
	ResultKey       string
	Param           string
	Unit            string
	Method          string
}

var valueApplicationRules = []valueApplicationRule{
	{BlockID: "led_indicator", CalculationKind: "led_resistor", ResultKey: "resistance_ohms", Param: "resistor_value", Unit: "ohm", Method: "calculated"},
	{BlockID: "i2c_sensor", CalculationKind: "i2c_pullup", ResultKey: "pullup_ohms", Param: "pullup_value", Unit: "ohm", Method: "policy"},
	{BlockID: "crystal_oscillator", CalculationKind: "crystal_load_cap", ResultKey: "capacitor_pf_each", Param: "load_capacitor_value", Unit: "pF", Method: "calculated"},
}

func (builder *planBuilder) applyCalculatedValueApplications() {
	for _, instanceID := range builder.ledIDs {
		builder.applyLEDResistorValue(instanceID)
	}
}

func (builder *planBuilder) applyLEDResistorValue(instanceID string) {
	params := builder.instanceParams[instanceID]
	if params == nil {
		return
	}
	normalizeLEDCurrentParam(params)
	if value := paramValue(params, "led_current"); value != "" {
		builder.updateSelectedBlockParam(instanceID, "led_current", value)
	}
	ohms, ok := ledResistorOhms(params)
	if !ok {
		if ledCalculationWasExplicit(params) {
			builder.addIssue("blocks."+instanceID+".params.resistor_value", "could not calculate LED resistor value from supplied voltage/current parameters", "ensure supply_voltage is above led_forward_voltage and led_current is positive")
		}
		return
	}
	literal := formatResistanceLiteral(ohms)
	if literal == "INVALID" {
		builder.addIssue("blocks."+instanceID+".params.resistor_value", "calculated LED resistor value is invalid", "adjust LED voltage and current inputs")
		return
	}
	rule, ok := valueApplicationRuleFor("led_indicator", "led_resistor")
	if !ok || !builder.blockSupportsParam("led_indicator", rule.Param) {
		builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{
			ID:         "calc.led_resistor." + instanceID + ".deferred",
			Path:       "blocks." + instanceID,
			Message:    "LED resistor calculation could not be applied because the block does not expose a compatible parameter",
			Severity:   reports.SeverityWarning,
			Suggestion: "add a supported resistor_value parameter to the LED block",
		})
		return
	}
	params[rule.Param] = literal
	builder.updateSelectedBlockParam(instanceID, rule.Param, literal)
}

func valueApplicationRuleFor(blockID string, kind string) (valueApplicationRule, bool) {
	for _, rule := range valueApplicationRules {
		if rule.BlockID == blockID && rule.CalculationKind == kind {
			return rule, true
		}
	}
	return valueApplicationRule{}, false
}

func normalizeLEDCurrentParam(params map[string]any) {
	if params == nil || paramValue(params, "led_current") != "" {
		return
	}
	currentMA, ok := parseFloatParam(params, "led_current_ma")
	if !ok {
		return
	}
	params["led_current"] = formatScaledLiteral(currentMA) + "mA"
}

func ledCalculationWasExplicit(params map[string]any) bool {
	return paramValue(params, "supply_voltage") != "" || paramValue(params, "led_forward_voltage") != "" || paramValue(params, "led_current") != "" || paramValue(params, "led_current_ma") != ""
}

func (builder *planBuilder) blockSupportsParam(blockID string, param string) bool {
	definition, ok := builder.registry.GetBlock(blockID)
	if !ok {
		return false
	}
	return blockDefinitionSupportsParam(definition, param)
}

func blockDefinitionSupportsParam(definition blocks.BlockDefinition, param string) bool {
	if param == "" {
		return false
	}
	for _, candidate := range definition.Parameters {
		if candidate.Name == param {
			return true
		}
	}
	return false
}

func appliedBlockValue(instanceID string, param string, value string, unit string, method string) AppliedValue {
	path := "blocks.params." + param
	if instanceID != "" {
		path = "blocks." + instanceID + ".params." + param
	}
	return AppliedValue{
		Target: "block",
		Path:   path,
		Value:  value,
		Unit:   unit,
		Method: method,
	}
}

func calculatedRequirement(subject string, kind string, operator string, value string, unit string, source string) CalculatedRequirement {
	return CalculatedRequirement{
		Subject:  strings.TrimSpace(subject),
		Kind:     strings.TrimSpace(kind),
		Operator: strings.TrimSpace(operator),
		Value:    strings.TrimSpace(value),
		Unit:     strings.TrimSpace(unit),
		Source:   strings.TrimSpace(source),
	}
}

func formatResistanceLiteral(ohms float64) string {
	if math.IsNaN(ohms) || math.IsInf(ohms, 0) || ohms < 0 {
		return "INVALID"
	}
	switch {
	case ohms == 0:
		return "0"
	case ohms >= 999_995:
		return formatScaledLiteral(ohms/1_000_000) + "M"
	case ohms >= 999.995:
		return formatScaledLiteral(ohms/1_000) + "k"
	default:
		return formatScaledLiteral(ohms)
	}
}

func formatCapacitancePFLiteral(pf float64) string {
	if math.IsNaN(pf) || math.IsInf(pf, 0) || pf < 0 {
		return "INVALID"
	}
	switch {
	case pf == 0:
		return "0pF"
	case pf >= 999_995:
		return formatScaledLiteral(pf/1_000_000) + "uF"
	case pf >= 999.995:
		return formatScaledLiteral(pf/1_000) + "nF"
	default:
		return formatScaledLiteral(pf) + "pF"
	}
}

func formatScaledLiteral(value float64) string {
	if value >= 0.999995 && value < 1 {
		return "1"
	}
	return strconv.FormatFloat(value, 'g', 6, 64)
}
