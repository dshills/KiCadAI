package intentplanner

import (
	"fmt"
	"strconv"
	"strings"

	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
)

func (builder *planBuilder) finalizeSynthesisTrace() {
	builder.recordSynthesisDecision(SynthesisDecision{
		ID:        "policy.validation",
		Type:      "validation_policy",
		Path:      "validation.acceptance",
		Selected:  string(builder.workflow.Validation.Acceptance),
		Rationale: "validation policy derived from requested acceptance",
	})
	if builder.workflow.RoutingRetry.Enabled {
		builder.recordSynthesisDecision(SynthesisDecision{
			ID:        "policy.routing_retry",
			Type:      "validation_policy",
			Path:      "routing_retry",
			Selected:  fmt.Sprintf("enabled:%d", builder.workflow.RoutingRetry.MaxAttempts),
			Rationale: "routing retry policy derived from acceptance and fabrication intent",
		})
	}
	builder.recordAcceptancePolicyTrace()
	builder.recordComponentPolicyTrace()
	builder.recordVoltageTrace()
	builder.recordBlockTrace()
	builder.recordConnectionTrace()
	builder.recordExistingGaps()
	builder.recordValueCalculationTrace()
}

func (builder *planBuilder) recordSynthesisDecision(record SynthesisDecision) {
	record.ID = normalizeTraceID(record.ID)
	if record.ID == "" || record.Type == "" {
		return
	}
	record.RequirementIDs = compactTraceStrings(record.RequirementIDs)
	record.EvidenceIDs = compactTraceStrings(record.EvidenceIDs)
	builder.plan.Synthesis.Decisions = append(builder.plan.Synthesis.Decisions, record)
}

func (builder *planBuilder) recordSynthesisEvidence(record SynthesisEvidence) {
	record.ID = normalizeTraceID(record.ID)
	if record.ID == "" || record.Kind == "" || record.Summary == "" {
		return
	}
	record.Refs = compactTraceStrings(record.Refs)
	builder.plan.Synthesis.Evidence = append(builder.plan.Synthesis.Evidence, record)
}

func (builder *planBuilder) recordSynthesisConstraint(record SynthesisConstraint) {
	record.ID = normalizeTraceID(record.ID)
	if record.ID == "" || record.Kind == "" || record.Subject == "" {
		return
	}
	builder.plan.Synthesis.Constraints = append(builder.plan.Synthesis.Constraints, record)
}

func (builder *planBuilder) recordSynthesisCalculation(record SynthesisCalculation) {
	record.ID = normalizeTraceID(record.ID)
	if record.ID == "" || record.Kind == "" {
		return
	}
	record.Requirements = compactCalculatedRequirements(record.Requirements)
	builder.plan.Synthesis.Calculations = append(builder.plan.Synthesis.Calculations, record)
}

func (builder *planBuilder) recordSynthesisGap(record SynthesisGap) {
	record.ID = normalizeTraceID(record.ID)
	if record.ID == "" || record.Category == "" || record.Message == "" {
		return
	}
	record.RequirementIDs = compactTraceStrings(record.RequirementIDs)
	record.EvidenceIDs = compactTraceStrings(record.EvidenceIDs)
	builder.plan.Synthesis.Gaps = append(builder.plan.Synthesis.Gaps, record)
}

func (builder *planBuilder) recordComponentPolicyTrace() {
	policy := builder.workflow.Components
	builder.recordSynthesisConstraint(SynthesisConstraint{
		ID:       "component.confidence",
		Kind:     "confidence",
		Subject:  "component_policy",
		Operator: ">=",
		Value:    string(policy.MinimumConfidence),
		Source:   "acceptance",
	})
	builder.recordSynthesisConstraint(SynthesisConstraint{
		ID:       "component.acceptance",
		Kind:     "acceptance",
		Subject:  "component_policy",
		Operator: ">=",
		Value:    string(policy.Acceptance),
		Source:   "acceptance",
	})
	for _, key := range sortedStringKeys(policy.PackagePreferences) {
		builder.recordSynthesisConstraint(SynthesisConstraint{
			ID:      "component.package." + key,
			Kind:    "package",
			Subject: key,
			Value:   policy.PackagePreferences[key],
			Source:  "constraints.package_preferences",
		})
	}
	for _, component := range builder.plan.SelectedComponents {
		for _, rating := range component.RequiredRatings {
			builder.recordSynthesisConstraint(SynthesisConstraint{
				ID:            "component.rating." + component.RequirementID + "." + normalizeToken(rating),
				Kind:          "current",
				Subject:       firstNonEmpty(component.Family, component.RequirementID),
				Operator:      "requires",
				Value:         rating,
				RequirementID: component.RequirementID,
			})
		}
	}
}

func (builder *planBuilder) recordVoltageTrace() {
	for index, input := range builder.request.Power.Inputs {
		reqID := fmt.Sprintf("power.input.%d", index+1)
		if input.Voltage != "" {
			builder.recordSynthesisConstraint(SynthesisConstraint{
				ID:            reqID + ".voltage",
				Path:          fmt.Sprintf("power.inputs[%d].voltage", index),
				Kind:          "voltage",
				Subject:       input.Kind,
				Operator:      "=",
				Value:         input.Voltage,
				Source:        "intent.power.inputs",
				RequirementID: reqID,
			})
		}
		if input.CurrentMA > 0 {
			builder.recordSynthesisConstraint(SynthesisConstraint{
				ID:            reqID + ".current",
				Path:          fmt.Sprintf("power.inputs[%d].current_ma", index),
				Kind:          "current",
				Subject:       input.Kind,
				Operator:      ">=",
				Value:         fmt.Sprintf("%gmA", input.CurrentMA),
				Source:        "intent.power.inputs",
				RequirementID: reqID,
			})
		}
	}
	for index, rail := range builder.request.Power.Rails {
		reqID := fmt.Sprintf("power.rail.%d", index+1)
		subject := firstNonEmpty(rail.Alias, rail.Name, reqID)
		if rail.Voltage != "" {
			builder.recordSynthesisDecision(SynthesisDecision{
				ID:             reqID + ".domain",
				Type:           "voltage_domain",
				Path:           fmt.Sprintf("power.rails[%d]", index),
				Selected:       subject + ":" + rail.Voltage,
				Rationale:      "voltage domain derived from requested rail",
				RequirementIDs: []string{reqID},
			})
			builder.recordSynthesisConstraint(SynthesisConstraint{
				ID:            reqID + ".voltage",
				Path:          fmt.Sprintf("power.rails[%d].voltage", index),
				Kind:          "voltage",
				Subject:       subject,
				Operator:      "=",
				Value:         rail.Voltage,
				Source:        "intent.power.rails",
				RequirementID: reqID,
			})
		}
		if rail.CurrentMA > 0 {
			builder.recordSynthesisConstraint(SynthesisConstraint{
				ID:            reqID + ".current",
				Path:          fmt.Sprintf("power.rails[%d].current_ma", index),
				Kind:          "current",
				Subject:       subject,
				Operator:      ">=",
				Value:         fmt.Sprintf("%gmA", rail.CurrentMA),
				Source:        "intent.power.rails",
				RequirementID: reqID,
			})
		}
	}
	for _, connection := range builder.plan.Connections {
		if strings.HasPrefix(connection.NetAlias, "VCC") || connection.NetAlias == "GND" || strings.HasPrefix(connection.NetAlias, "VIN") {
			builder.recordSynthesisEvidence(SynthesisEvidence{
				ID:      "voltage.connection." + normalizeToken(connection.From+"."+connection.To+"."+connection.NetAlias),
				Kind:    "workflow_policy",
				Path:    "connections",
				Summary: connection.From + " -> " + connection.To + " on " + connection.NetAlias,
				Source:  "planner.connection",
			})
		}
	}
}

func (builder *planBuilder) recordBlockTrace() {
	for _, block := range builder.plan.SelectedBlocks {
		builder.recordSynthesisDecision(SynthesisDecision{
			ID:             "block." + block.InstanceID,
			Type:           "topology",
			Path:           "selected_blocks." + block.InstanceID,
			Selected:       block.InstanceID + ":" + block.BlockID,
			Rationale:      firstNonEmpty(block.Rationale, "selected block satisfies a planner requirement"),
			RequirementIDs: append([]string(nil), block.RequirementIDs...),
			Confidence:     block.Verification,
		})
		builder.recordSynthesisEvidence(SynthesisEvidence{
			ID:         "block.capability." + block.InstanceID,
			Kind:       "block_capability",
			Path:       "blocks." + block.InstanceID,
			Summary:    block.BlockID + " readiness=" + block.Readiness,
			Source:     "blocks.registry",
			Confidence: block.Verification,
			Refs:       append([]string(nil), block.KnownGaps...),
		})
	}
	for _, requirement := range builder.plan.Requirements {
		builder.recordSynthesisEvidence(SynthesisEvidence{
			ID:      "requirement." + requirement.ID,
			Kind:    "intent_field",
			Path:    requirement.Path,
			Summary: firstNonEmpty(requirement.Value, requirement.Type),
			Source:  "intent.request",
			Refs:    append([]string(nil), requirement.Evidence...),
		})
	}
}

func (builder *planBuilder) recordConnectionTrace() {
	for index, connection := range builder.plan.Connections {
		decisionType := "topology"
		rationale := strings.ToLower(connection.Rationale + " " + connection.NetAlias)
		if strings.Contains(rationale, "i2c") || strings.Contains(rationale, "uart") || strings.Contains(rationale, "isp") || strings.Contains(rationale, "spi") || strings.Contains(rationale, "gpio") {
			decisionType = "bus_resolution"
		}
		builder.recordSynthesisDecision(SynthesisDecision{
			ID:             fmt.Sprintf("connection.%03d", index+1),
			Type:           decisionType,
			Path:           "connections",
			Selected:       connection.From + " -> " + connection.To,
			Rationale:      firstNonEmpty(connection.Rationale, "connection satisfies generated topology"),
			RequirementIDs: append([]string(nil), connection.RequirementIDs...),
		})
		if connection.NetAlias != "" {
			builder.recordSynthesisEvidence(SynthesisEvidence{
				ID:      fmt.Sprintf("net.%03d", index+1),
				Kind:    "workflow_policy",
				Path:    "connections",
				Summary: connection.NetAlias + ": " + connection.From + " -> " + connection.To,
				Source:  "planner.connection",
			})
		}
	}
}

func (builder *planBuilder) recordExistingGaps() {
	for _, gap := range builder.plan.KnownGaps {
		builder.recordSynthesisGap(SynthesisGap{
			ID:         gap.ID,
			Category:   categoryForSynthesisGap(gap.Path, gap.Message),
			Path:       gap.Path,
			Message:    gap.Message,
			Severity:   firstSeverity(gap.Severity, reports.SeverityWarning),
			Suggestion: gap.Suggestion,
		})
	}
	for _, clarification := range builder.plan.Clarifications {
		builder.recordSynthesisGap(SynthesisGap{
			ID:         clarification.ID,
			Category:   categoryForSynthesisGap(clarification.Path, clarification.Message),
			Path:       clarification.Path,
			Message:    clarification.Message,
			Severity:   reports.SeverityError,
			Suggestion: clarification.Suggestion,
		})
	}
	for index, issue := range builder.plan.Issues {
		builder.recordSynthesisGap(SynthesisGap{
			ID:         fmt.Sprintf("issue.%03d", index+1),
			Category:   categoryForSynthesisGap(issue.Path, issue.Message),
			Path:       issue.Path,
			Message:    issue.Message,
			Severity:   firstSeverity(issue.Severity, reports.SeverityError),
			Suggestion: issue.Suggestion,
		})
	}
}

func (builder *planBuilder) recordValueCalculationTrace() {
	for _, block := range builder.plan.SelectedBlocks {
		switch block.BlockID {
		case "led_indicator":
			builder.recordSynthesisCalculation(SynthesisCalculation{
				ID:          "calc.led_resistor." + block.InstanceID,
				Kind:        "led_resistor",
				Path:        "blocks." + block.InstanceID,
				Inputs:      paramsToStringMap(block.Params, "supply_voltage", "led_forward_voltage", "led_current", "led_current_ma"),
				Result:      ledResistorResult(block.Params),
				Formula:     "(Vsupply - Vf) / Iled",
				Assumptions: []string{"uses block defaults when request omits LED voltage or current"},
				Confidence:  "policy",
				Status:      ledResistorCalculationStatus(block),
				Applied:     ledResistorAppliedValues(block),
				Requirements: []CalculatedRequirement{
					ledResistorRequirement(block),
					ledCurrentRequirement(block),
					ledResistorPowerRequirement(block),
				},
			})
		case "i2c_sensor":
			builder.recordSynthesisCalculation(SynthesisCalculation{
				ID:          "calc.i2c_pullup." + block.InstanceID,
				Kind:        "i2c_pullup",
				Path:        "blocks." + block.InstanceID,
				Inputs:      paramsToStringMap(block.Params, "supply_voltage", "bus_speed_hz", "pullup_ohms", "pullup_value", "include_pullups"),
				Result:      i2cPullupResult(block.Params),
				Formula:     "default low-speed I2C pull-up policy",
				Assumptions: []string{"exact pull-up value remains block/component policy unless explicitly requested"},
				Confidence:  "policy",
				Status:      i2cPullupCalculationStatus(block),
				Applied:     i2cPullupAppliedValues(block),
				Requirements: []CalculatedRequirement{
					i2cPullupResistanceRequirement(block),
					i2cPullupVoltageRequirement(block),
				},
			})
		case "voltage_regulator":
			builder.recordSynthesisCalculation(SynthesisCalculation{
				ID:          "calc.regulator_headroom." + block.InstanceID,
				Kind:        "regulator_headroom",
				Path:        "blocks." + block.InstanceID,
				Inputs:      paramsToStringMap(block.Params, "input_voltage", "output_voltage", "output_current", "regulator_symbol", "regulator_footprint", "enable_mode"),
				Result:      regulatorHeadroomResult(block.Params),
				Formula:     "Vin must exceed Vout plus regulator dropout and safety margin",
				Assumptions: regulatorReviewAssumptions(block),
				Confidence:  "policy",
				Status:      regulatorHeadroomStatus(block),
				Requirements: []CalculatedRequirement{
					regulatorVoltageRequirement(block, "input_voltage"),
					regulatorVoltageRequirement(block, "output_voltage"),
					regulatorCurrentRequirement(block),
					regulatorHeadroomRequirement(block),
					regulatorDropoutRequirement(block),
					regulatorCapacitorPolicyRequirement(block, "input_capacitor", "input_voltage"),
					regulatorCapacitorPolicyRequirement(block, "output_capacitor", "output_voltage"),
					regulatorThermalReviewRequirement(block),
					regulatorStabilityReviewRequirement(block),
				},
			})
		case "crystal_oscillator":
			builder.recordSynthesisCalculation(SynthesisCalculation{
				ID:          "calc.crystal_load." + block.InstanceID,
				Kind:        "crystal_load_cap",
				Path:        "blocks." + block.InstanceID,
				Inputs:      paramsToStringMap(block.Params, "frequency", "load_cap_pf", "stray_cap_pf"),
				Result:      crystalLoadResult(block.Params),
				Formula:     "Cload caps derived from crystal CL and estimated stray capacitance",
				Assumptions: []string{"blocked from MCU wiring until external-clock topology is supported"},
				Confidence:  "policy",
				Status:      crystalLoadStatus(block),
				Applied:     crystalLoadAppliedValues(block),
				Requirements: []CalculatedRequirement{
					crystalLoadCapRequirement(block),
				},
			})
		case "opamp_gain_stage":
			builder.recordSynthesisCalculation(SynthesisCalculation{
				ID:         "calc.opamp_gain." + block.InstanceID,
				Kind:       "opamp_gain",
				Path:       "blocks." + block.InstanceID,
				Inputs:     paramsToStringMap(block.Params, "gain"),
				Result:     opampGainResult(block.Params),
				Formula:    "non-inverting gain = 1 + Rf/Rg",
				Confidence: "policy",
				Status:     opampGainStatus(block),
				Requirements: []CalculatedRequirement{
					opampGainRequirement(block),
					opampSupplyRequirement(block),
				},
			})
		}
	}
}

func categoryForSynthesisGap(path string, message string) string {
	value := strings.ToLower(path + " " + message)
	switch {
	case strings.Contains(value, "clock"):
		return "unsupported_peripheral"
	case strings.Contains(value, "supply") || strings.Contains(value, "voltage") || strings.Contains(value, "rail"):
		return "voltage_domain"
	case strings.Contains(value, "component") || strings.Contains(value, "footprint") || strings.Contains(value, "symbol"):
		return "component_constraint"
	case strings.Contains(value, "target") || strings.Contains(value, "mcu"):
		return "target_resolution"
	case strings.Contains(value, "i2c") || strings.Contains(value, "uart") || strings.Contains(value, "spi") || strings.Contains(value, "gpio"):
		return "bus_resolution"
	default:
		return "unsupported_gap"
	}
}

func firstSeverity(value reports.Severity, fallback reports.Severity) reports.Severity {
	if value != "" {
		return value
	}
	return fallback
}

func normalizeTraceID(value string) string {
	return strings.Trim(strings.ReplaceAll(normalizeToken(value), "__", "_"), "_")
}

func compactTraceStrings(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func compactCalculatedRequirements(values []CalculatedRequirement) []CalculatedRequirement {
	var out []CalculatedRequirement
	for _, value := range values {
		if value.Subject == "" || value.Kind == "" || value.Value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func paramsToStringMap(params map[string]any, keys ...string) map[string]string {
	out := map[string]string{}
	for _, key := range keys {
		if value, ok := params[key]; ok && value != nil {
			out[key] = strings.TrimSpace(fmt.Sprint(value))
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func ledResistorResult(params map[string]any) map[string]string {
	ohms, ok := ledResistorOhms(params)
	if !ok {
		return nil
	}
	return map[string]string{"resistance_ohms": formatScaledLiteral(ohms)}
}

func ledResistorOhms(params map[string]any) (float64, bool) {
	supply, supplyOK := parseVoltage(paramValue(params, "supply_voltage"))
	forward, forwardOK := parseVoltage(paramValue(params, "led_forward_voltage"))
	currentMA, currentOK := ledCurrentMA(params)
	if !supplyOK || !forwardOK || !currentOK || currentMA <= 0 || supply <= forward {
		return 0, false
	}
	ohms := (supply - forward) / (currentMA / 1000)
	return ohms, true
}

func ledCurrentMA(params map[string]any) (float64, bool) {
	if currentMA, ok := parseFloatParam(params, "led_current_ma"); ok {
		return currentMA, true
	}
	current := paramValue(params, "led_current")
	if current == "" {
		return 0, false
	}
	lower := strings.TrimSpace(strings.ToLower(current))
	switch {
	case strings.HasSuffix(lower, "ma"):
		return parseFloatString(strings.TrimSuffix(lower, "ma"))
	case strings.HasSuffix(lower, "a"):
		amps, ok := parseFloatString(strings.TrimSuffix(lower, "a"))
		return amps * 1000, ok
	default:
		return parseFloatString(lower)
	}
}

func parseFloatString(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func ledResistorCalculationStatus(block SelectedBlockRecord) string {
	if paramValue(block.Params, "resistor_value") != "" && ledResistorResult(block.Params) != nil {
		return "applied"
	}
	if ledCalculationWasExplicit(block.Params) && ledResistorResult(block.Params) == nil {
		return "blocked"
	}
	return "deferred"
}

func ledResistorAppliedValues(block SelectedBlockRecord) []AppliedValue {
	value := paramValue(block.Params, "resistor_value")
	if value == "" || ledResistorResult(block.Params) == nil {
		return nil
	}
	return []AppliedValue{appliedBlockValue(block.InstanceID, "resistor_value", value, "ohm", "calculated")}
}

func ledResistorRequirement(block SelectedBlockRecord) CalculatedRequirement {
	value := paramValue(block.Params, "resistor_value")
	if value == "" {
		return CalculatedRequirement{}
	}
	return calculatedRequirement("resistor", "resistance", "=", value, "ohm", "led_resistor")
}

func ledCurrentRequirement(block SelectedBlockRecord) CalculatedRequirement {
	currentMA, ok := ledCurrentMA(block.Params)
	if !ok || currentMA <= 0 {
		return CalculatedRequirement{}
	}
	return calculatedRequirement("led", "forward_current", ">=", formatScaledLiteral(currentMA), "mA", "led_resistor")
}

func ledResistorPowerRequirement(block SelectedBlockRecord) CalculatedRequirement {
	supply, supplyOK := parseVoltage(paramValue(block.Params, "supply_voltage"))
	forward, forwardOK := parseVoltage(paramValue(block.Params, "led_forward_voltage"))
	currentMA, currentOK := ledCurrentMA(block.Params)
	if !supplyOK || !forwardOK || !currentOK || currentMA <= 0 || supply <= forward {
		return CalculatedRequirement{}
	}
	powerW := (supply - forward) * (currentMA / 1000)
	return calculatedRequirement("resistor", "power", ">=", formatScaledLiteral(powerW), "W", "led_resistor")
}

func i2cPullupResult(params map[string]any) map[string]string {
	if explicit := firstNonEmpty(paramValue(params, "pullup_value"), paramValue(params, "pullup_ohms")); explicit != "" {
		return map[string]string{"pullup_ohms": explicit}
	}
	speed, speedOK := parseFloatParam(params, "bus_speed_hz")
	if speedOK && speed > 100000 {
		return map[string]string{"recommended_range_ohms": i2cPullupRangeFast}
	}
	return map[string]string{"recommended_range_ohms": i2cPullupRangeDefault}
}

func i2cPullupCalculationStatus(block SelectedBlockRecord) string {
	if !boolParamDefault(block.Params, "include_pullups", true) {
		return "deferred"
	}
	rule, _ := valueApplicationRuleFor(block.BlockID, "i2c_pullup")
	if rule.Param != "" && paramValue(block.Params, rule.Param) != "" {
		return "applied"
	}
	return "deferred"
}

func i2cPullupAppliedValues(block SelectedBlockRecord) []AppliedValue {
	if !boolParamDefault(block.Params, "include_pullups", true) {
		return nil
	}
	rule, _ := valueApplicationRuleFor(block.BlockID, "i2c_pullup")
	param := firstNonEmpty(rule.Param, "pullup_value")
	value := paramValue(block.Params, param)
	if value == "" {
		return nil
	}
	return []AppliedValue{appliedBlockValue(block.InstanceID, param, value, "ohm", "policy")}
}

func i2cPullupResistanceRequirement(block SelectedBlockRecord) CalculatedRequirement {
	value := paramValue(block.Params, "pullup_value")
	if value == "" {
		if result := i2cPullupResult(block.Params); result != nil {
			value = i2cPullupConcreteValue(result)
		}
	}
	if value == "" {
		return CalculatedRequirement{}
	}
	return calculatedRequirement("i2c_pullup", "resistance", "=", value, "ohm", "i2c_pullup")
}

func i2cPullupVoltageRequirement(block SelectedBlockRecord) CalculatedRequirement {
	voltage := paramValue(block.Params, "supply_voltage")
	if voltage == "" {
		return CalculatedRequirement{}
	}
	parsed, ok := parseVoltage(voltage)
	if !ok {
		return CalculatedRequirement{}
	}
	return calculatedRequirement("i2c_pullup", "voltage", "=", formatScaledLiteral(parsed), "V", "i2c_pullup")
}

func regulatorHeadroomResult(params map[string]any) map[string]string {
	input, inputOK := parseVoltage(paramValue(params, "input_voltage"))
	output, outputOK := parseVoltage(paramValue(params, "output_voltage"))
	if !inputOK || !outputOK {
		return nil
	}
	result := map[string]string{
		"headroom_v":               fmt.Sprintf("%.2f", input-output),
		"variant":                  regulatorVariantName(params),
		"dropout_margin_required":  regulatorDropoutMargin(params),
		"capacitor_voltage_policy": "minimum_125_percent_operating_voltage",
	}
	if thermal := regulatorThermalReview(params); thermal != "" {
		result["thermal_review"] = thermal
	}
	if stability := regulatorStabilityReview(params); stability != "" {
		result["stability_review"] = stability
	}
	return result
}

func regulatorHeadroomStatus(block SelectedBlockRecord) string {
	result := regulatorHeadroomResult(block.Params)
	if result == nil {
		return "deferred"
	}
	headroom, ok := parseFloatString(result["headroom_v"])
	if !ok || headroom <= 0 {
		return "blocked"
	}
	return "deferred"
}

func regulatorVoltageRequirement(block SelectedBlockRecord, key string) CalculatedRequirement {
	value := paramValue(block.Params, key)
	if value == "" {
		return CalculatedRequirement{}
	}
	parsed, ok := parseVoltage(value)
	if !ok {
		return CalculatedRequirement{}
	}
	return calculatedRequirement("regulator", key, "=", formatScaledLiteral(parsed), "V", "regulator_headroom")
}

func regulatorCurrentRequirement(block SelectedBlockRecord) CalculatedRequirement {
	current := paramValue(block.Params, "output_current")
	if current == "" {
		return CalculatedRequirement{}
	}
	return calculatedRequirement("regulator", "output_current", ">=", current, "", "regulator_headroom")
}

func regulatorHeadroomRequirement(block SelectedBlockRecord) CalculatedRequirement {
	result := regulatorHeadroomResult(block.Params)
	if result == nil || result["headroom_v"] == "" {
		return CalculatedRequirement{}
	}
	return calculatedRequirement("regulator", "headroom", ">", "0", "V", "regulator_headroom")
}

func regulatorDropoutRequirement(block SelectedBlockRecord) CalculatedRequirement {
	margin := regulatorDropoutMargin(block.Params)
	if margin == "" {
		return CalculatedRequirement{}
	}
	return calculatedRequirement("regulator", "dropout_margin", ">=", margin, "V", "regulator_headroom")
}

func regulatorCapacitorPolicyRequirement(block SelectedBlockRecord, subject string, voltageKey string) CalculatedRequirement {
	voltage, ok := parseVoltage(paramValue(block.Params, voltageKey))
	if !ok {
		return CalculatedRequirement{}
	}
	return calculatedRequirement("regulator."+subject, "voltage_policy", ">=", formatScaledLiteral(capacitorVoltageRating(voltage)), "V", "minimum_125_percent_operating_voltage")
}

func regulatorThermalReviewRequirement(block SelectedBlockRecord) CalculatedRequirement {
	review := regulatorThermalReview(block.Params)
	if review == "" {
		return CalculatedRequirement{}
	}
	return calculatedRequirement("regulator", "thermal_review", "requires", review, "", "regulator_profile")
}

func regulatorStabilityReviewRequirement(block SelectedBlockRecord) CalculatedRequirement {
	review := regulatorStabilityReview(block.Params)
	if review == "" {
		return CalculatedRequirement{}
	}
	return calculatedRequirement("regulator", "stability_review", "requires", review, "", "regulator_profile")
}

func regulatorReviewAssumptions(block SelectedBlockRecord) []string {
	return compactTraceStrings([]string{
		"capacitor voltage policy is minimum automated class, not MLCC DC-bias proof",
		regulatorThermalReview(block.Params),
		regulatorStabilityReview(block.Params),
	})
}

func regulatorVariantName(params map[string]any) string {
	switch paramValue(params, "regulator_symbol") {
	case "Regulator_Linear:AP2112K-3.3":
		return "ap2112k_3v3_sot23_5"
	case "":
		return "default_fixed_linear_regulator"
	case "Regulator_Linear:AMS1117-3.3":
		return "ams1117_3v3_sot223"
	default:
		return normalizeToken(paramValue(params, "regulator_symbol"))
	}
}

func regulatorDropoutMargin(params map[string]any) string {
	switch regulatorVariantName(params) {
	case "ap2112k_3v3_sot23_5":
		return "0.5"
	default:
		return "1.3"
	}
}

func regulatorThermalReview(params map[string]any) string {
	switch regulatorVariantName(params) {
	case "ap2112k_3v3_sot23_5":
		return "review SOT-23-5 dissipation against board copper and ambient temperature"
	default:
		return "review SOT-223 dissipation against board copper and ambient temperature"
	}
}

func regulatorStabilityReview(params map[string]any) string {
	switch regulatorVariantName(params) {
	case "ap2112k_3v3_sot23_5":
		return "verify AP2112K input/output capacitor effective capacitance and ESR"
	default:
		return "verify selected regulator input/output capacitor effective capacitance and ESR"
	}
}

func crystalLoadResult(params map[string]any) map[string]string {
	loadPF, loadOK := parseFloatParam(params, "load_cap_pf")
	if !loadOK {
		return nil
	}
	strayPF, strayOK := parseFloatParam(params, "stray_cap_pf")
	if !strayOK {
		strayPF = 2
	}
	capPF := (loadPF - strayPF) * 2
	if capPF <= 0 {
		return nil
	}
	return map[string]string{"capacitor_pf_each": fmt.Sprintf("%.1f", capPF)}
}

func crystalLoadStatus(block SelectedBlockRecord) string {
	param := crystalLoadParam(block)
	if paramValue(block.Params, param) != "" && crystalLoadResult(block.Params) != nil {
		return "applied"
	}
	if crystalLoadResult(block.Params) != nil {
		return "deferred"
	}
	return "blocked"
}

func crystalLoadAppliedValues(block SelectedBlockRecord) []AppliedValue {
	param := crystalLoadParam(block)
	value := paramValue(block.Params, param)
	if value == "" || crystalLoadResult(block.Params) == nil {
		return nil
	}
	return []AppliedValue{appliedBlockValue(block.InstanceID, param, value, "pF", "calculated")}
}

func crystalLoadCapRequirement(block SelectedBlockRecord) CalculatedRequirement {
	value := paramValue(block.Params, crystalLoadParam(block))
	if value == "" {
		if result := crystalLoadResult(block.Params); result != nil {
			value = result["capacitor_pf_each"]
		}
	}
	if value == "" {
		return CalculatedRequirement{}
	}
	return calculatedRequirement("crystal_load_capacitor", "capacitance", "=", value, "pF", "crystal_load_cap")
}

func crystalLoadParam(block SelectedBlockRecord) string {
	rule, _ := valueApplicationRuleFor(block.BlockID, "crystal_load_cap")
	return firstNonEmpty(rule.Param, "load_capacitor_value")
}

func opampGainResult(params map[string]any) map[string]string {
	gain, ok := parseFloatParam(params, "gain")
	if !ok || gain < 1 {
		return nil
	}
	return map[string]string{"rf_over_rg": fmt.Sprintf("%.2f", gain-1)}
}

func opampGainStatus(block SelectedBlockRecord) string {
	if opampGainResult(block.Params) == nil {
		return "blocked"
	}
	return "deferred"
}

func opampGainRequirement(block SelectedBlockRecord) CalculatedRequirement {
	gain := paramValue(block.Params, "gain")
	if gain == "" {
		return CalculatedRequirement{}
	}
	return calculatedRequirement("opamp_feedback", "gain", "=", gain, "", "opamp_gain")
}

func opampSupplyRequirement(block SelectedBlockRecord) CalculatedRequirement {
	voltage := paramValue(block.Params, "supply_voltage")
	if voltage == "" {
		return CalculatedRequirement{}
	}
	parsed, ok := parseVoltage(voltage)
	if !ok {
		return CalculatedRequirement{}
	}
	return calculatedRequirement("opamp", "supply_voltage", "=", formatScaledLiteral(parsed), "V", "opamp_gain")
}

func paramValue(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	value, ok := params[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func parseFloatParam(params map[string]any, key string) (float64, bool) {
	value := paramValue(params, key)
	if value == "" {
		return 0, false
	}
	var parsed float64
	if _, err := fmt.Sscanf(value, "%f", &parsed); err != nil {
		return 0, false
	}
	return parsed, true
}

func (builder *planBuilder) recordAcceptancePolicyTrace() {
	if builder.workflow.Validation.Acceptance == designworkflow.AcceptanceFabricationCandidate {
		builder.recordSynthesisConstraint(SynthesisConstraint{
			ID:      "fabrication.acceptance",
			Kind:    "fabrication",
			Subject: "validation",
			Value:   string(builder.workflow.Validation.Acceptance),
			Source:  "intent.acceptance",
		})
	}
}
