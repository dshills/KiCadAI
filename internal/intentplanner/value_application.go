package intentplanner

import (
	"math"
	"strconv"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/components"
	"kicadai/internal/designworkflow"
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

const (
	i2cPullupRangeFast    = "2200-4700"
	i2cPullupRangeDefault = "4700-10000"
)

var valueApplicationRules = []valueApplicationRule{
	{BlockID: "led_indicator", CalculationKind: "led_resistor", ResultKey: "resistance_ohms", Param: "resistor_value", Unit: "ohm", Method: "calculated"},
	{BlockID: "i2c_sensor", CalculationKind: "i2c_pullup", ResultKey: "pullup_ohms", Param: "pullup_value", Unit: "ohm", Method: "policy"},
	{BlockID: "crystal_oscillator", CalculationKind: "crystal_load_cap", ResultKey: "capacitor_pf_each", Param: "load_capacitor_value", Unit: "pF", Method: "calculated"},
}

func (builder *planBuilder) applyCalculatedValueApplications() {
	for _, instanceID := range builder.ledIDs {
		builder.applyLEDResistorValue(instanceID)
	}
	for _, instanceID := range builder.sensorIDs {
		builder.applyI2CPullupValue(instanceID)
	}
	for _, instanceID := range builder.clockIDs {
		builder.applyCrystalLoadCapValue(instanceID)
	}
	for _, instanceID := range builder.regulatorIDs {
		builder.validateRegulatorHeadroom(instanceID)
	}
	builder.applyCalculatedRatingOverrides()
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

func (builder *planBuilder) applyI2CPullupValue(instanceID string) {
	blockID := builder.instanceBlockIDs[instanceID]
	if blockID != "i2c_sensor" {
		return
	}
	params := builder.instanceParams[instanceID]
	if params == nil || !boolParamDefault(params, "include_pullups", true) {
		return
	}
	result := i2cPullupResult(params)
	if result == nil {
		return
	}
	value := i2cPullupConcreteValue(result)
	if value == "" {
		return
	}
	rule, ok := valueApplicationRuleFor(blockID, "i2c_pullup")
	if !ok || !builder.blockSupportsParam(blockID, rule.Param) {
		builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{
			ID:         "calc.i2c_pullup." + instanceID + ".deferred",
			Path:       "blocks." + instanceID,
			Message:    "I2C pull-up calculation could not be applied because the block does not expose a compatible parameter",
			Severity:   reports.SeverityWarning,
			Suggestion: "add a supported pullup_value parameter to the I2C sensor block",
		})
		return
	}
	params[rule.Param] = value
	builder.updateSelectedBlockParam(instanceID, rule.Param, value)
}

func boolParamDefault(params map[string]any, key string, fallback bool) bool {
	value, ok := params[key]
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "yes", "1":
			return true
		case "false", "no", "0":
			return false
		}
	}
	return fallback
}

func i2cPullupConcreteValue(result map[string]string) string {
	if explicit := strings.TrimSpace(result["pullup_ohms"]); explicit != "" {
		if ohms, ok := parseFloatString(explicit); ok {
			return formatResistanceLiteral(ohms)
		}
		return explicit
	}
	switch strings.TrimSpace(result["recommended_range_ohms"]) {
	case i2cPullupRangeFast:
		return "2.2k"
	case i2cPullupRangeDefault:
		return "4.7k"
	default:
		return ""
	}
}

func (builder *planBuilder) applyCrystalLoadCapValue(instanceID string) {
	blockID := builder.instanceBlockIDs[instanceID]
	if blockID != "crystal_oscillator" {
		return
	}
	params := builder.instanceParams[instanceID]
	if params == nil {
		return
	}
	result := crystalLoadResult(params)
	if result == nil {
		return
	}
	capPF, ok := parseFloatString(result["capacitor_pf_each"])
	if !ok {
		return
	}
	literal := formatCapacitancePFLiteral(capPF)
	if literal == "INVALID" {
		return
	}
	rule, ok := valueApplicationRuleFor(blockID, "crystal_load_cap")
	if !ok || !builder.blockSupportsParam(blockID, rule.Param) {
		builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{
			ID:         "calc.crystal_load." + instanceID + ".deferred",
			Path:       "blocks." + instanceID,
			Message:    "crystal load calculation could not be applied because the block does not expose a compatible parameter",
			Severity:   reports.SeverityWarning,
			Suggestion: "add a supported load_capacitor_value parameter to the crystal block",
		})
		return
	}
	params[rule.Param] = literal
	builder.updateSelectedBlockParam(instanceID, rule.Param, literal)
}

func (builder *planBuilder) validateRegulatorHeadroom(instanceID string) {
	params := builder.instanceParams[instanceID]
	if params == nil {
		return
	}
	result := regulatorHeadroomResult(params)
	if result == nil {
		return
	}
	headroom, ok := parseFloatString(result["headroom_v"])
	if !ok || headroom <= 0 {
		builder.addIssue("blocks."+instanceID+".params.input_voltage", "regulator input voltage must exceed output voltage", "raise the regulator input voltage or lower the requested output rail")
	}
}

func (builder *planBuilder) applyCalculatedRatingOverrides() {
	for _, instanceID := range builder.ledIDs {
		builder.applyLEDRatingOverrides(instanceID)
	}
	for _, instanceID := range builder.amplifierIDs {
		builder.applyOpAmpRatingOverrides(instanceID)
	}
}

func (builder *planBuilder) applyLEDRatingOverrides(instanceID string) {
	params := builder.instanceParams[instanceID]
	if params == nil {
		return
	}
	supply, supplyOK := parseVoltage(paramValue(params, "supply_voltage"))
	forward, forwardOK := parseVoltage(paramValue(params, "led_forward_voltage"))
	currentMA, currentOK := ledCurrentMA(params)
	if supplyOK && forwardOK && currentOK && currentMA > 0 && supply > forward {
		powerW := (supply - forward) * (currentMA / 1000)
		builder.appendComponentRequiredRating(instanceID, "resistor", components.RequiredRating{Kind: "power", Value: formatScaledLiteral(powerW), Unit: "W"})
	}
	if currentOK && currentMA > 0 {
		builder.appendComponentRequiredRating(instanceID, "led", components.RequiredRating{Kind: "current", Value: formatScaledLiteral(currentMA / 1000), Unit: "A"})
	}
}

func (builder *planBuilder) applyOpAmpRatingOverrides(instanceID string) {
	params := builder.instanceParams[instanceID]
	if params == nil {
		return
	}
	voltage, ok := parseVoltage(paramValue(params, "supply_voltage"))
	if !ok {
		return
	}
	builder.appendComponentRequiredRating(instanceID, "opamp", components.RequiredRating{Kind: "supply_voltage", Value: formatScaledLiteral(voltage), Unit: "V"})
}

func (builder *planBuilder) appendComponentRequiredRating(instanceID string, role string, rating components.RequiredRating) {
	if rating.Kind == "" || rating.Value == "" {
		return
	}
	if builder.workflow.Components.Overrides == nil {
		builder.workflow.Components.Overrides = map[string]designworkflow.ComponentOverrideSpec{}
	}
	key := instanceID + "." + role
	override := builder.workflow.Components.Overrides[key]
	for _, existing := range override.RequiredRatings {
		if existing.Kind == rating.Kind && existing.Value == rating.Value && existing.Unit == rating.Unit {
			return
		}
	}
	override.RequiredRatings = append(override.RequiredRatings, rating)
	builder.workflow.Components.Overrides[key] = override
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
