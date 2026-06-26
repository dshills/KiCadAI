package intentplanner

import (
	"fmt"
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
				Inputs:      paramsToStringMap(block.Params, "supply_voltage", "led_forward_voltage", "led_current_ma"),
				Formula:     "(Vsupply - Vf) / Iled",
				Assumptions: []string{"uses block defaults when request omits LED voltage or current"},
				Confidence:  "policy",
			})
		case "i2c_sensor":
			builder.recordSynthesisCalculation(SynthesisCalculation{
				ID:          "calc.i2c_pullup." + block.InstanceID,
				Kind:        "i2c_pullup",
				Path:        "blocks." + block.InstanceID,
				Inputs:      paramsToStringMap(block.Params, "supply_voltage", "bus_speed_hz", "pullup_ohms"),
				Formula:     "default low-speed I2C pull-up policy",
				Assumptions: []string{"exact pull-up value remains block/component policy unless explicitly requested"},
				Confidence:  "policy",
			})
		case "voltage_regulator":
			builder.recordSynthesisCalculation(SynthesisCalculation{
				ID:         "calc.regulator_headroom." + block.InstanceID,
				Kind:       "regulator_headroom",
				Path:       "blocks." + block.InstanceID,
				Inputs:     paramsToStringMap(block.Params, "input_voltage", "output_voltage"),
				Formula:    "Vin must exceed Vout plus regulator dropout",
				Confidence: "policy",
			})
		case "crystal_oscillator":
			builder.recordSynthesisCalculation(SynthesisCalculation{
				ID:          "calc.crystal_load." + block.InstanceID,
				Kind:        "crystal_load_cap",
				Path:        "blocks." + block.InstanceID,
				Inputs:      paramsToStringMap(block.Params, "frequency", "load_cap_pf", "stray_cap_pf"),
				Formula:     "Cload caps derived from crystal CL and estimated stray capacitance",
				Assumptions: []string{"blocked from MCU wiring until external-clock topology is supported"},
				Confidence:  "policy",
			})
		case "opamp_gain_stage":
			builder.recordSynthesisCalculation(SynthesisCalculation{
				ID:         "calc.opamp_gain." + block.InstanceID,
				Kind:       "opamp_gain",
				Path:       "blocks." + block.InstanceID,
				Inputs:     paramsToStringMap(block.Params, "gain"),
				Formula:    "non-inverting gain = 1 + Rf/Rg",
				Confidence: "policy",
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
