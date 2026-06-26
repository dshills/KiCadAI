package intentplanner

import (
	"math"
	"strconv"
	"strings"

	"kicadai/internal/blocks"
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

func valueApplicationRuleFor(blockID string, kind string) (valueApplicationRule, bool) {
	for _, rule := range valueApplicationRules {
		if rule.BlockID == blockID && rule.CalculationKind == kind {
			return rule, true
		}
	}
	return valueApplicationRule{}, false
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
	return strconv.FormatFloat(value, 'g', -1, 64)
}
