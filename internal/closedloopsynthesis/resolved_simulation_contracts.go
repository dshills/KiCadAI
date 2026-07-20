package closedloopsynthesis

import (
	"slices"
	"strings"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/simmodel"
)

// BuildResolvedSimulationContracts converts public behavioral metrics into
// structured simulator assertions only after semantic targets have been bound
// to candidate-specific nodes. Unsupported or ambiguous component scopes fail
// closed instead of falling back to analytic estimates.
func BuildResolvedSimulationContracts(requirement architecturesearch.Requirement, analysisPlan AnalysisPlan, plans map[string]simmodel.Plan) ([]SimulationAnalysisTemplate, []SimulationAssertionBinding, []SimulationOperatingBinding, []Diagnostic) {
	var diagnostics []Diagnostic
	var assertionBindings []SimulationAssertionBinding
	templates := make([]SimulationAnalysisTemplate, 0, len(plans))
	kinds := make([]string, 0, len(plans))
	for kind := range plans {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	for _, kind := range kinds {
		analysis, ok := resolvedTemplateAnalysis(plans[kind], kind)
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{Path: "plans." + kind, Message: "resolved workflow lacks a trusted analysis template"})
			continue
		}
		templates = append(templates, SimulationAnalysisTemplate{Kind: kind, Analysis: analysis})
	}

	referenceNode, _ := primaryInputReference(requirement, analysisPlan.Bindings)
	seenAssertions := map[string]bool{}
	for _, assertion := range analysisPlan.Assertions {
		key := assertion.Metric + "\x00" + assertion.Target
		if seenAssertions[key] {
			continue
		}
		seenAssertions[key] = true
		binding, diagnostic := resolvedAssertionBinding(assertion, referenceNode, plans[assertionAnalysisKind(analysisPlan, assertion.AnalysisID)])
		if diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			continue
		}
		binding.Metric, binding.Target = assertion.Metric, assertion.Target
		if binding.BoundsMode == "" {
			binding.BoundsMode = AssertionBoundsDirect
		}
		binding.Prototypes = append([]simmodel.Assertion(nil), binding.Prototypes...)
		assertionBindings = append(assertionBindings, binding)
	}

	operatingBindings := resolvedOperatingBindings(analysisPlan, plans, &diagnostics)
	slices.SortStableFunc(assertionBindings, func(left, right SimulationAssertionBinding) int {
		if order := strings.Compare(left.Metric, right.Metric); order != 0 {
			return order
		}
		return strings.Compare(left.Target, right.Target)
	})
	slices.SortStableFunc(diagnostics, compareDiagnostics)
	if len(diagnostics) != 0 {
		return nil, nil, nil, diagnostics
	}
	return templates, assertionBindings, operatingBindings, nil
}

func resolvedTemplateAnalysis(plan simmodel.Plan, kind string) (simmodel.Analysis, bool) {
	for _, analysis := range plan.Analyses {
		if analysis.Kind == kind {
			return cloneSimulationAnalysis(analysis), true
		}
	}
	return simmodel.Analysis{}, false
}

func assertionAnalysisKind(plan AnalysisPlan, analysisID string) string {
	for _, analysis := range plan.Analyses {
		if analysis.ID == analysisID {
			return analysis.Kind
		}
	}
	return ""
}

func primaryInputReference(requirement architecturesearch.Requirement, bindings []SemanticBinding) (string, bool) {
	targets := map[string]string{}
	for _, binding := range bindings {
		targets[binding.Kind+"\x00"+binding.ID] = binding.Target
	}
	var candidates []string
	for _, port := range requirement.Requirements.Ports {
		if port.Direction != "sink" || port.Kind == "power" || port.Kind == "reference" {
			continue
		}
		if target := targets["port\x00"+port.ID]; target != "" {
			candidates = append(candidates, target)
		}
	}
	slices.Sort(candidates)
	if len(candidates) != 1 {
		return "", false
	}
	return candidates[0], true
}

func resolvedAssertionBinding(assertion PlannedAssertion, referenceNode string, plan simmodel.Plan) (SimulationAssertionBinding, *Diagnostic) {
	prototype := simmodel.Assertion{Node: assertion.Target}
	binding := SimulationAssertionBinding{BoundsMode: AssertionBoundsDirect}
	switch assertion.Metric {
	case "dc_voltage", "output_high_voltage":
		prototype.Quantity = simmodel.QuantityVoltageV
	case "threshold_voltage":
		prototype.Quantity = simmodel.QuantityThresholdVoltageV
	case "threshold_current":
		prototype.Quantity = simmodel.QuantityThresholdCurrentA
	case "hysteresis_voltage":
		prototype.Quantity = simmodel.QuantityHysteresisVoltageV
	case "voltage_gain":
		if referenceNode == "" {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "voltage gain requires exactly one resolved signal input reference"}
		}
		prototype.Quantity, prototype.ReferenceNode = simmodel.QuantityVoltageGainRatio, referenceNode
		if analysis, ok := resolvedTemplateAnalysis(plan, simmodel.AnalysisACSweep); ok {
			prototype.FrequencyHz = analysis.StartFrequencyHz
		}
	case "bandwidth":
		if referenceNode == "" {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "bandwidth requires exactly one resolved signal input reference"}
		}
		prototype.Quantity, prototype.ReferenceNode = simmodel.QuantityBandwidthHz, referenceNode
	case "cutoff_frequency":
		if referenceNode == "" {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "cutoff frequency requires exactly one resolved signal input reference"}
		}
		prototype.Quantity, prototype.ReferenceNode = simmodel.QuantityCutoffFrequencyHz, referenceNode
	case "integrated_output_noise":
		prototype.Quantity = simmodel.QuantityIntegratedNoiseVRMS
	case "phase_margin":
		prototype.Quantity = simmodel.QuantityPhaseMarginDeg
	case "rise_time":
		prototype.Quantity = simmodel.QuantityRiseTimeS
	case "fall_time":
		prototype.Quantity = simmodel.QuantityFallTimeS
	case "settling_time":
		prototype.Quantity = simmodel.QuantitySettlingTimeS
	case "response_time":
		prototype.Quantity = simmodel.QuantityResponseTimeS
	case "muted_output_voltage":
		prototype.Quantity, binding.BoundsMode = simmodel.QuantityPeakAbsVoltageV, AssertionBoundsAbsolute
	case "output_swing":
		prototype.Quantity = simmodel.QuantityOutputSwingVPP
	case "startup_output_voltage":
		prototype.Quantity, binding.BoundsMode = simmodel.QuantityPeakAbsVoltageV, AssertionBoundsAbsolute
	case "total_harmonic_distortion":
		prototype.Quantity = simmodel.QuantityTHDPercent
	case "junction_temperature":
		components := thermalComponentsForTarget(plan, assertion.Target)
		if len(components) == 0 {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "junction temperature target has no catalog-backed thermal component"}
		}
		for _, component := range components {
			binding.Prototypes = append(binding.Prototypes, simmodel.Assertion{Component: component, Quantity: simmodel.QuantityJunctionTemperatureC})
		}
		return binding, nil
	case "output_power":
		component, ok := uniqueLoadComponent(plan, assertion.Target)
		if !ok {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "output power requires exactly one resolved load component"}
		}
		prototype.Quantity, prototype.Component = simmodel.QuantityOutputPowerW, component
	case "dc_current", "quiescent_current":
		component, ok := uniqueSourceComponent(plan, assertion.Target)
		if !ok {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "current measurement requires exactly one resolved source component"}
		}
		prototype.Node, prototype.Quantity, prototype.Component = "", simmodel.QuantityDeviceCurrentA, component
	case "transimpedance":
		component, ok := uniqueSourceComponent(plan, "")
		if !ok {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "transimpedance requires exactly one resolved input source component"}
		}
		prototype.Quantity, prototype.Component = simmodel.QuantityTransimpedanceOhm, component
	default:
		return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "behavioral metric has no registered structured simulation binding"}
	}
	binding.Prototypes = []simmodel.Assertion{prototype}
	return binding, nil
}

func resolvedOperatingBindings(analysisPlan AnalysisPlan, plans map[string]simmodel.Plan, diagnostics *[]Diagnostic) []SimulationOperatingBinding {
	seen := map[string]bool{}
	var result []SimulationOperatingBinding
	for _, corner := range analysisPlan.Corners {
		for _, assignment := range corner.Assignments {
			key := assignment.Axis + "\x00" + assignment.Target
			if seen[key] {
				continue
			}
			seen[key] = true
			binding := SimulationOperatingBinding{Axis: assignment.Axis, Target: assignment.Target}
			switch assignment.Axis {
			case "ambient_temperature":
				binding.Kind, binding.Parameter = OperatingAnalysisCondition, "ambient_temperature_c"
			case "tolerance":
				binding.Kind = OperatingWorstCase
			case "supply_voltage", "input_amplitude", "load_current":
				component, ok := uniqueSourceAcrossPlans(plans, assignment.Target)
				if !ok {
					*diagnostics = append(*diagnostics, Diagnostic{Path: "corners." + corner.ID + "." + assignment.Axis, Message: "operating source target is missing or ambiguous"})
					continue
				}
				binding.Kind, binding.Component = OperatingSourceDCValue, component
			case "load_resistance":
				component, ok := uniqueDeviceAcrossPlans(plans, assignment.Target, "resistor")
				if !ok {
					*diagnostics = append(*diagnostics, Diagnostic{Path: "corners." + corner.ID + ".load_resistance", Message: "load resistance target is missing or ambiguous"})
					continue
				}
				binding.Kind, binding.Component = OperatingDeviceValueSI, component
			case "load_capacitance":
				component, ok := uniqueDeviceAcrossPlans(plans, assignment.Target, "capacitor")
				if !ok {
					*diagnostics = append(*diagnostics, Diagnostic{Path: "corners." + corner.ID + ".load_capacitance", Message: "load capacitance target is missing or ambiguous"})
					continue
				}
				binding.Kind, binding.Component = OperatingDeviceValueSI, component
			default:
				*diagnostics = append(*diagnostics, Diagnostic{Path: "corners." + corner.ID + "." + assignment.Axis, Message: "operating axis has no registered resolved simulation binding"})
				continue
			}
			result = append(result, binding)
		}
	}
	slices.SortStableFunc(result, func(left, right SimulationOperatingBinding) int {
		if order := strings.Compare(left.Axis, right.Axis); order != 0 {
			return order
		}
		return strings.Compare(left.Target, right.Target)
	})
	return result
}

func thermalComponentsForTarget(plan simmodel.Plan, target string) []string {
	var result []string
	for _, device := range plan.Devices {
		if target != "circuit" && !deviceTouchesNet(device, target) {
			continue
		}
		if hasThermalPath(device.ModelParameters) {
			result = append(result, device.Component)
		}
	}
	slices.Sort(result)
	return result
}

func hasThermalPath(parameters []simmodel.NamedValue) bool {
	for _, parameter := range parameters {
		switch parameter.Name {
		case "thermal_resistance_c_per_w", "junction_to_ambient_c_per_w", "junction_to_case_c_per_w":
			return true
		}
	}
	return false
}

func uniqueLoadComponent(plan simmodel.Plan, target string) (string, bool) {
	return uniqueDeviceInPlan(plan, target, "resistor")
}

func uniqueSourceComponent(plan simmodel.Plan, target string) (string, bool) {
	var candidates []string
	for _, device := range plan.Devices {
		if target != "" && !deviceTouchesNet(device, target) {
			continue
		}
		switch device.PrimitiveModel {
		case simmodel.PrimitiveVoltageSourceV1, simmodel.PrimitiveConnectorVoltageSourceV1, simmodel.PrimitiveCurrentSourceV1:
			candidates = append(candidates, device.Component)
		}
	}
	slices.Sort(candidates)
	return uniqueString(candidates)
}

func uniqueSourceAcrossPlans(plans map[string]simmodel.Plan, target string) (string, bool) {
	var candidates []string
	for _, plan := range plans {
		if component, ok := uniqueSourceComponent(plan, target); ok {
			candidates = append(candidates, component)
		}
	}
	slices.Sort(candidates)
	candidates = slices.Compact(candidates)
	return uniqueString(candidates)
}

func uniqueDeviceAcrossPlans(plans map[string]simmodel.Plan, target, family string) (string, bool) {
	var candidates []string
	for _, plan := range plans {
		if component, ok := uniqueDeviceInPlan(plan, target, family); ok {
			candidates = append(candidates, component)
		}
	}
	slices.Sort(candidates)
	candidates = slices.Compact(candidates)
	return uniqueString(candidates)
}

func uniqueDeviceInPlan(plan simmodel.Plan, target, family string) (string, bool) {
	var candidates []string
	for _, device := range plan.Devices {
		if device.Family == family && deviceTouchesNet(device, target) {
			candidates = append(candidates, device.Component)
		}
	}
	slices.Sort(candidates)
	return uniqueString(candidates)
}

func uniqueString(values []string) (string, bool) {
	if len(values) != 1 {
		return "", false
	}
	return values[0], true
}

func deviceTouchesNet(device simmodel.ResolvedDevice, target string) bool {
	for _, terminal := range device.Terminals {
		if terminal.Net == target {
			return true
		}
	}
	return false
}
