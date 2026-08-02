package closedloopsynthesis

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
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
	supplyNodes := semanticSupplyNodes(requirement, analysisPlan.Bindings)
	operatingBindings := resolvedOperatingBindings(analysisPlan, plans, &diagnostics)
	operatingBindings = appendEventSupplyBindings(operatingBindings, supplyNodes, plans, &diagnostics)
	operatingBindings = appendGeneratedDomainControlBindings(operatingBindings, requirement, analysisPlan.Bindings, plans)
	seenAssertions := map[string]bool{}
	for _, assertion := range analysisPlan.Assertions {
		key := assertion.Metric + "\x00" + assertion.Target
		if seenAssertions[key] {
			continue
		}
		seenAssertions[key] = true
		resolvedAssertion := assertion
		if strings.HasPrefix(assertion.Target, "event:") {
			eventID := strings.TrimPrefix(assertion.Target, "event:")
			eventTarget, exists := plannedEventTarget(analysisPlan.Events, eventID, assertion.OperatingCase)
			if !exists {
				diagnostics = append(diagnostics, Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "event assertion lacks a planned target in its operating case"})
				continue
			}
			if assertion.Metric == "sequence_delay" {
				sequenceTarget, ok := sequenceResponseTarget(requirement, analysisPlan.Bindings)
				if !ok {
					diagnostics = append(diagnostics, Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "sequence-delay event requires exactly one resolved dependent rail or sequencer state output"})
					continue
				}
				eventTarget = sequenceTarget
			}
			if assertion.Metric == "protection_response_time" {
				protectionTarget, ok := protectionResponseTarget(requirement, analysisPlan.Bindings, eventTarget)
				if !ok {
					diagnostics = append(diagnostics, Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "protection-response event requires exactly one resolved protection control or affected protected path"})
					continue
				}
				eventTarget = protectionTarget
			}
			if assertion.Metric == "muted_output_voltage" {
				muteTarget, ok := muteResponseTarget(requirement, analysisPlan.Bindings)
				if !ok {
					diagnostics = append(diagnostics, Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "event-driven mute measurement requires exactly one resolved protected output"})
					continue
				}
				eventTarget = muteTarget
			}
			resolvedAssertion.Target = eventTarget
		}
		binding, diagnostic := resolvedAssertionBinding(resolvedAssertion, referenceNode, supplyNodes, operatingBindings, plans[assertionAnalysisKind(analysisPlan, assertion.AnalysisID)], requirement, analysisPlan.Bindings)
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
	if diagnostic := configureThresholdSweep(analysisPlan, plans, referenceNode, templates); diagnostic != nil {
		diagnostics = append(diagnostics, *diagnostic)
	}

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

func appendGeneratedDomainControlBindings(bindings []SimulationOperatingBinding, requirement architecturesearch.Requirement, semanticBindings []SemanticBinding, plans map[string]simmodel.Plan) []SimulationOperatingBinding {
	generatedDomains := map[string]bool{}
	for _, domain := range requirement.Requirements.Domains {
		generatedDomains[domain.ID] = domain.Source != "" && !strings.EqualFold(domain.Source, "external")
	}
	targets := map[string]string{}
	for _, binding := range semanticBindings {
		if binding.Kind == "port" {
			targets[binding.ID] = binding.Target
		}
	}
	seenComponents := map[string]bool{}
	for _, binding := range bindings {
		if binding.Kind == OperatingGeneratedControl {
			seenComponents[binding.Component] = true
		}
	}
	for _, port := range requirement.Requirements.Ports {
		if !generatedDomains[port.Domain] || port.Direction == "source" {
			continue
		}
		switch port.Kind {
		case "digital_logic", "digital_bus", "analog_control":
		default:
			continue
		}
		target := targets[port.ID]
		if target == "" {
			continue
		}
		component, ok := uniqueVoltageSourceAcrossPlans(plans, target)
		if !ok || seenComponents[component] {
			continue
		}
		seenComponents[component] = true
		bindings = append(bindings, SimulationOperatingBinding{
			Axis: "generated_domain_control", Target: target, Kind: OperatingGeneratedControl, Component: component,
		})
	}
	slices.SortStableFunc(bindings, func(left, right SimulationOperatingBinding) int {
		if order := strings.Compare(left.Axis, right.Axis); order != 0 {
			return order
		}
		if order := strings.Compare(left.Target, right.Target); order != 0 {
			return order
		}
		return strings.Compare(left.Component, right.Component)
	})
	return bindings
}

func appendEventSupplyBindings(bindings []SimulationOperatingBinding, supplyNodes []string, plans map[string]simmodel.Plan, diagnostics *[]Diagnostic) []SimulationOperatingBinding {
	boundTargets := map[string]bool{}
	for _, binding := range bindings {
		if binding.Kind == OperatingSourceDCValue {
			boundTargets[binding.Target] = true
		}
	}
	for _, target := range supplyNodes {
		if boundTargets[target] {
			continue
		}
		component, ok := uniqueVoltageSourceAcrossPlans(plans, target)
		if !ok {
			*diagnostics = append(*diagnostics, Diagnostic{Path: "events.supplies." + target, Message: "external supply event target is missing or ambiguous"})
			continue
		}
		bindings = append(bindings, SimulationOperatingBinding{
			Axis: eventSupplyAxis, Target: target, Kind: OperatingSourceDCValue, Component: component,
		})
	}
	slices.SortStableFunc(bindings, func(left, right SimulationOperatingBinding) int {
		if order := strings.Compare(left.Axis, right.Axis); order != 0 {
			return order
		}
		return strings.Compare(left.Target, right.Target)
	})
	return bindings
}

func plannedEventTarget(events []PlannedEvent, eventID, operatingCase string) (string, bool) {
	for _, event := range events {
		if event.ID == eventID && event.OperatingCase == operatingCase {
			return event.Target, true
		}
	}
	return "", false
}

func sequenceResponseTarget(requirement architecturesearch.Requirement, bindings []SemanticBinding) (string, bool) {
	var railTargets, stateTargets []string
	for _, objective := range requirement.Requirements.Objectives {
		if objective.Capability != "rail_sequencing" {
			continue
		}
		for _, binding := range objective.Bindings {
			if target := semanticBindingTarget(bindings, binding); target != "" {
				switch binding.Role {
				case "rail_b":
					if downstream := downstreamPublicResponseTargets(requirement, binding, bindings); len(downstream) != 0 {
						railTargets = append(railTargets, downstream...)
					} else {
						railTargets = append(railTargets, target)
					}
				case "state":
					stateTargets = append(stateTargets, target)
				}
			}
		}
	}
	slices.Sort(railTargets)
	if target, ok := uniqueString(slices.Compact(railTargets)); ok {
		return target, true
	}
	slices.Sort(stateTargets)
	return uniqueString(slices.Compact(stateTargets))
}

func protectionResponseTarget(requirement architecturesearch.Requirement, bindings []SemanticBinding, eventTarget string) (string, bool) {
	var targets []string
	hasInterlock := false
	for _, objective := range requirement.Requirements.Objectives {
		if objective.Capability != "safety_interlock" {
			continue
		}
		hasInterlock = true
		for _, binding := range objective.Bindings {
			if binding.Direction != "source" {
				continue
			}
			switch binding.Role {
			case "control", "drive", "enable", "output", "permit":
				if target := semanticBindingTarget(bindings, binding); target != "" {
					targets = append(targets, target)
				}
			}
		}
	}
	slices.Sort(targets)
	if hasInterlock {
		return uniqueString(slices.Compact(targets))
	}

	if target, ok := sequenceResponseTarget(requirement, bindings); ok {
		return target, true
	}

	for _, objective := range requirement.Requirements.Objectives {
		if objective.Capability != "output_protection" {
			continue
		}
		var inputs, outputs []string
		for _, binding := range objective.Bindings {
			target := semanticBindingTarget(bindings, binding)
			if target == "" {
				continue
			}
			switch binding.Role {
			case "input":
				inputs = append(inputs, target)
			case "output", "protected":
				outputs = append(outputs, target)
			}
		}
		if slices.Contains(outputs, eventTarget) {
			targets = append(targets, inputs...)
		} else if slices.Contains(inputs, eventTarget) {
			targets = append(targets, outputs...)
		}
	}
	slices.Sort(targets)
	return uniqueString(slices.Compact(targets))
}

func downstreamPublicResponseTargets(requirement architecturesearch.Requirement, source architecturesearch.Binding, bindings []SemanticBinding) []string {
	var targets []string
	for _, objective := range requirement.Requirements.Objectives {
		consumesSource := false
		for _, binding := range objective.Bindings {
			if binding.Role == "input" && sameSemanticEndpoint(binding, source) {
				consumesSource = true
				break
			}
		}
		if !consumesSource {
			continue
		}
		for _, binding := range objective.Bindings {
			if binding.Role != "output" || binding.Port == "" {
				continue
			}
			if target := semanticBindingTarget(bindings, binding); target != "" {
				targets = append(targets, target)
			}
		}
	}
	slices.Sort(targets)
	return slices.Compact(targets)
}

func sameSemanticEndpoint(left, right architecturesearch.Binding) bool {
	return left.Port != "" && left.Port == right.Port ||
		left.Signal != "" && left.Signal == right.Signal ||
		left.Participant != "" && left.Participant == right.Participant && left.ParticipantPort != "" && left.ParticipantPort == right.ParticipantPort
}

func muteResponseTarget(requirement architecturesearch.Requirement, bindings []SemanticBinding) (string, bool) {
	var candidates []string
	for _, objective := range requirement.Requirements.Objectives {
		if objective.Capability != "mute_control" {
			continue
		}
		for _, binding := range objective.Bindings {
			switch binding.Role {
			case "protected", "output":
				if target := semanticBindingTarget(bindings, binding); target != "" {
					candidates = append(candidates, target)
				}
			}
		}
	}
	slices.Sort(candidates)
	return uniqueString(slices.Compact(candidates))
}

func configureThresholdSweep(analysisPlan AnalysisPlan, plans map[string]simmodel.Plan, referenceNode string, templates []SimulationAnalysisTemplate) *Diagnostic {
	thresholdKind := ""
	minimum, maximum := math.Inf(1), math.Inf(-1)
	bidirectional := false
	for _, assertion := range analysisPlan.Assertions {
		switch assertion.Metric {
		case "threshold_voltage", "threshold_current":
			if thresholdKind != "" && thresholdKind != assertion.Metric {
				return &Diagnostic{Path: "assertions", Message: "one DC workflow cannot combine voltage and current threshold sweeps"}
			}
			thresholdKind = assertion.Metric
			if assertion.Min != nil {
				minimum = math.Min(minimum, *assertion.Min)
			}
			if assertion.Max != nil {
				maximum = math.Max(maximum, *assertion.Max)
			}
		case "hysteresis_voltage":
			bidirectional = true
		}
	}
	if thresholdKind == "" && !bidirectional {
		return nil
	}
	if thresholdKind == "" || !finiteClosedLoopBound(minimum) || !finiteClosedLoopBound(maximum) || minimum > maximum {
		return &Diagnostic{Path: "assertions", Message: "threshold or hysteresis measurement requires a finite absolute threshold bound"}
	}
	plan, exists := plans[simmodel.AnalysisDCOperatingPoint]
	if !exists {
		return &Diagnostic{Path: "plans.dc_operating_point", Message: "threshold measurement requires a resolved DC workflow"}
	}
	component, ok := "", false
	if thresholdKind == "threshold_current" {
		component, ok = uniqueDeviceFamilyInPlan(plan, "current_source")
	} else if referenceNode != "" {
		component, ok = uniqueSourceComponent(plan, referenceNode)
	}
	if !ok {
		return &Diagnostic{Path: "plans.dc_operating_point.dc_sweep", Message: "threshold measurement requires exactly one compatible resolved input source"}
	}
	excitationScale, ok := sourceSweepExcitationScale(plan, component, referenceNode, thresholdKind)
	if !ok {
		return &Diagnostic{Path: "plans.dc_operating_point.dc_sweep.component", Message: "threshold sweep source polarity is ambiguous at the semantic input"}
	}
	// Voltage thresholds need a local sweep window. Scaling the window by the
	// absolute threshold (or by an unrelated operating rail) can make the fixed
	// bounded grid too coarse to resolve a narrow hysteresis requirement.
	thresholdRange := maximum - minimum
	thresholdScale := math.Max(math.Abs(minimum), math.Abs(maximum))
	span := 2 * math.Max(thresholdRange, 0.05*thresholdScale)
	span = math.Max(span, 1e-9)
	start, stop := minimum-span, maximum+span
	if minimum >= 0 && start < 0 {
		start = 0
	}
	if maximum <= 0 && stop > 0 {
		stop = 0
	}
	axis := "input_amplitude"
	if thresholdKind == "threshold_current" {
		axis = "load_current"
	}
	if operatingMinimum, operatingMaximum, bounded := thresholdOperatingBounds(analysisPlan, thresholdKind, axis); bounded {
		if minimum < operatingMinimum || maximum > operatingMaximum {
			return &Diagnostic{Path: "assertions", Message: "threshold bounds exceed the declared operating-axis range"}
		}
		if thresholdKind == "threshold_current" {
			start, stop = operatingMinimum, operatingMaximum
		} else {
			start = math.Max(start, operatingMinimum)
			stop = math.Min(stop, operatingMaximum)
		}
	}
	for index := range templates {
		if templates[index].Kind != simmodel.AnalysisDCOperatingPoint {
			continue
		}
		found := false
		for _, excitation := range templates[index].Analysis.Excitations {
			found = found || excitation.Component == component
		}
		if !found {
			return &Diagnostic{Path: "plans.dc_operating_point.dc_sweep.component", Message: "threshold sweep source is absent from the resolved analysis excitations"}
		}
		templates[index].Analysis.DCSweep = &simmodel.DCSweep{Component: component, StartValue: start, StopValue: stop, Points: 201, Bidirectional: bidirectional, ExcitationScale: excitationScale}
		return nil
	}
	return &Diagnostic{Path: "plans.dc_operating_point", Message: "threshold measurement has no DC analysis template"}
}

func thresholdOperatingBounds(plan AnalysisPlan, metric, axis string) (float64, float64, bool) {
	operatingCases := map[string]bool{}
	for _, assertion := range plan.Assertions {
		if assertion.Metric == metric {
			operatingCases[assertion.OperatingCase] = true
		}
	}
	minimum, maximum := math.Inf(1), math.Inf(-1)
	for _, corner := range plan.Corners {
		if !operatingCases[corner.OperatingCase] {
			continue
		}
		for _, assignment := range corner.Assignments {
			if assignment.Axis != axis || assignment.Value == nil {
				continue
			}
			minimum = math.Min(minimum, *assignment.Value)
			maximum = math.Max(maximum, *assignment.Value)
		}
	}
	return minimum, maximum, !math.IsInf(minimum, 1) && !math.IsInf(maximum, -1)
}

func sourceSweepExcitationScale(plan simmodel.Plan, component, referenceNode, thresholdKind string) (float64, bool) {
	for _, device := range plan.Devices {
		if device.Component != component {
			continue
		}
		if thresholdKind == "threshold_current" && device.PrimitiveModel == simmodel.PrimitiveCurrentSourceV1 {
			return 1, true
		}
		positiveTerminal, negativeTerminal := "", ""
		switch device.PrimitiveModel {
		case simmodel.PrimitiveVoltageSourceV1:
			positiveTerminal, negativeTerminal = "POSITIVE", "NEGATIVE"
		case simmodel.PrimitiveConnectorVoltageSourceV1:
			positiveTerminal, negativeTerminal = "PIN_1", "PIN_2"
		default:
			return 0, false
		}
		polarity := 0.0
		for _, terminal := range device.Terminals {
			if terminal.Net != referenceNode {
				continue
			}
			switch terminal.Terminal {
			case positiveTerminal:
				polarity = 1
			case negativeTerminal:
				polarity = -1
			}
		}
		return polarity, polarity != 0
	}
	return 0, false
}

func finiteClosedLoopBound(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
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
	signalKinds := make(map[string]string, len(requirement.Requirements.Signals))
	for _, signal := range requirement.Requirements.Signals {
		signalKinds[signal.ID] = signal.Kind
	}
	type candidate struct {
		node  string
		score int
	}
	var candidates []candidate
	for _, port := range requirement.Requirements.Ports {
		if port.Kind == "power" || port.Kind == "reference" || port.Direction == "source" {
			continue
		}
		node := targets["port\x00"+port.ID]
		if node == "" {
			continue
		}
		ingress, roleScore := port.Direction == "sink", 0
		for _, objective := range requirement.Requirements.Objectives {
			boundToPort, producesSignal, consumesSignal := false, false, false
			for _, binding := range objective.Bindings {
				if binding.Port == port.ID {
					boundToPort = true
					switch binding.Role {
					case "input", "signal", "sense":
						roleScore = max(roleScore, 5)
					case "control", "enable", "mute", "bias":
						roleScore = min(roleScore, -5)
					}
				}
				if binding.Signal == "" || signalKinds[binding.Signal] == "power" || signalKinds[binding.Signal] == "reference" {
					continue
				}
				switch binding.Direction {
				case "source":
					producesSignal = true
				case "sink":
					consumesSignal = true
				}
			}
			if boundToPort && producesSignal && !consumesSignal {
				ingress = true
			}
		}
		if !ingress {
			continue
		}
		kindScore := 10
		switch port.Kind {
		case "analog_voltage", "analog_current":
			kindScore = 30
		case "digital_logic", "digital_bus", "open_drain_bus":
			kindScore = 20
		}
		candidates = append(candidates, candidate{node: node, score: kindScore + roleScore})
	}
	slices.SortStableFunc(candidates, func(left, right candidate) int {
		if left.score != right.score {
			return right.score - left.score
		}
		return strings.Compare(left.node, right.node)
	})
	if len(candidates) == 0 || (len(candidates) > 1 && candidates[0].score == candidates[1].score && candidates[0].node != candidates[1].node) {
		return "", false
	}
	return candidates[0].node, true
}

func resolvedAssertionBinding(assertion PlannedAssertion, referenceNode string, supplyNodes []string, operatingBindings []SimulationOperatingBinding, plan simmodel.Plan, requirement architecturesearch.Requirement, semanticBindings []SemanticBinding) (SimulationAssertionBinding, *Diagnostic) {
	prototype := simmodel.Assertion{Node: assertion.Target, ResponseDirection: assertion.ResponseDirection}
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
	case "phase_margin", "gain_margin", "loop_crossover_frequency", "closed_loop_peaking":
		switch assertion.Metric {
		case "phase_margin":
			prototype.Quantity = simmodel.QuantityPhaseMarginDeg
		case "gain_margin":
			prototype.Quantity = simmodel.QuantityGainMarginDB
		case "loop_crossover_frequency":
			prototype.Quantity = simmodel.QuantityLoopCrossoverHz
		case "closed_loop_peaking":
			prototype.Quantity = simmodel.QuantityClosedLoopPeakingDB
		}
		loopNode, ok := stabilityObservationNode(plan, assertion.Target)
		if !ok {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "stability target does not resolve through a unique passive output path to a trusted op-amp or emitter-degenerated BJT loop"}
		}
		prototype.Node = loopNode
	case "rise_time":
		prototype.Quantity = simmodel.QuantityRiseTimeS
	case "fall_time":
		prototype.Quantity = simmodel.QuantityFallTimeS
	case "settling_time":
		prototype.Quantity = simmodel.QuantitySettlingTimeS
	case "response_time":
		prototype.Quantity = simmodel.QuantityResponseTimeS
	case "protection_response_time", "protection_recovery_time", "sequence_delay":
		prototype.Quantity = simmodel.QuantityResponseTimeS
	case "overshoot_voltage":
		prototype.Quantity = simmodel.QuantityOvershootVoltageV
	case "peak_to_peak_ripple":
		prototype.Quantity = simmodel.QuantityOutputSwingVPP
	case "peak_device_voltage":
		prototype.Quantity, binding.BoundsMode = simmodel.QuantityPeakAbsVoltageV, AssertionBoundsAbsolute
	case "peak_device_current":
		component, ok := uniqueOperatingLoadForTarget(operatingBindings, assertion.Target)
		if !ok {
			component, ok = uniqueLoadComponent(plan, assertion.Target)
		}
		if !ok {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "peak device-current measurement requires one resolved operating load component"}
		}
		prototype.Node, prototype.Component, prototype.Quantity = "", component, simmodel.QuantityPeakAbsDeviceCurrentA
	case "muted_output_voltage":
		prototype.Quantity, binding.BoundsMode = simmodel.QuantityPeakAbsVoltageV, AssertionBoundsAbsolute
		override, ok := resolvedMuteExcitationOverride(requirement, semanticBindings, plan)
		if ok {
			binding.ExcitationOverrides = []SimulationExcitationOverride{override}
		} else if !behaviorObservesEvent(requirement, assertion.RequirementID, assertion.OperatingCase) {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "muted-output measurement requires one resolved active-high mute-control source"}
		}
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
	case "peak_junction_temperature":
		components := dynamicThermalComponentsForTarget(plan, assertion.Target, false)
		if len(components) == 0 {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "peak junction temperature target has no reviewed dynamic thermal RC component"}
		}
		for _, component := range components {
			binding.Prototypes = append(binding.Prototypes, simmodel.Assertion{Component: component, Quantity: simmodel.QuantityJunctionTemperatureC})
		}
		return binding, nil
	case "transient_soa_margin":
		components := dynamicThermalComponentsForTarget(plan, assertion.Target, true)
		if len(components) == 0 {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "transient SOA target has no reviewed time-dependent SOA component"}
		}
		for _, component := range components {
			binding.Prototypes = append(binding.Prototypes, simmodel.Assertion{Component: component, Quantity: simmodel.QuantityTransientSOAMargin})
		}
		return binding, nil
	case "output_power":
		component, ok := uniqueLoadComponent(plan, assertion.Target)
		if !ok {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "output power requires exactly one resolved load component"}
		}
		prototype.Quantity, prototype.Component = simmodel.QuantityOutputPowerW, component
	case "conversion_efficiency":
		component, ok := uniqueOperatingLoadForTarget(operatingBindings, assertion.Target)
		if !ok {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "conversion efficiency requires exactly one resolved operating load component"}
		}
		components, ok := supplySourceComponents(plan, supplyNodes)
		if !ok {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "conversion efficiency requires one catalog-backed source for every resolved supply domain"}
		}
		prototype.Node, prototype.Component, prototype.Components, prototype.Quantity = "", component, components, simmodel.QuantityConversionEfficiencyPct
	case "dc_current", "quiescent_current":
		if assertion.Metric == "quiescent_current" && assertion.Target == "circuit" {
			components, ok := supplySourceComponents(plan, supplyNodes)
			if !ok {
				return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "circuit quiescent-current measurement requires one catalog-backed source for every resolved supply domain"}
			}
			binding.Prototypes = append(binding.Prototypes, simmodel.Assertion{Quantity: simmodel.QuantityTotalSupplyCurrentA, Components: components})
			return binding, nil
		}
		component, ok := uniqueLoadComponent(plan, assertion.Target)
		if !ok {
			component, ok = uniqueSourceComponent(plan, assertion.Target)
		}
		if !ok {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "current measurement requires exactly one resolved operating load or source component"}
		}
		prototype.Node, prototype.Quantity, prototype.Component = "", simmodel.QuantityDeviceCurrentA, component
	case "transimpedance":
		component, ok := uniqueOperatingSourceForAxis(operatingBindings, "load_current", plan)
		if !ok {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "transimpedance requires exactly one resolved load-current excitation"}
		}
		prototype.Quantity, prototype.Component = simmodel.QuantityTransimpedanceOhm, component
	default:
		return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "behavioral metric has no registered structured simulation binding"}
	}
	switch assertion.Metric {
	case "dc_voltage", "output_high_voltage", "rise_time", "fall_time", "settling_time", "response_time", "muted_output_voltage", "output_swing", "startup_output_voltage", "overshoot_voltage", "peak_to_peak_ripple", "peak_device_voltage":
		localReference, required := behavioralObservationReferenceNode(requirement, assertion.RequirementID, assertion.OperatingCase, semanticBindings)
		if required && (localReference == "" || localReference == prototype.Node) {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "voltage-domain behavior requires one distinct resolved reference-domain node"}
		}
		prototype.ReferenceNode = localReference
	}
	binding.Prototypes = []simmodel.Assertion{prototype}
	return binding, nil
}

func behaviorObservesEvent(requirement architecturesearch.Requirement, requirementID, operatingCase string) bool {
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		if behavior.ID != requirementID || behavior.Observation.Kind != "event" {
			continue
		}
		return slices.Contains(behavior.OperatingCases, operatingCase)
	}
	return false
}

func dynamicThermalComponentsForTarget(plan simmodel.Plan, target string, requireSOA bool) []string {
	matches := func(device simmodel.ResolvedDevice) bool {
		if requireSOA {
			return len(device.TransientSOA) != 0
		}
		return device.ThermalModel != nil
	}
	if target == "circuit" {
		var result []string
		for _, device := range plan.Devices {
			if matches(device) {
				result = append(result, device.Component)
			}
		}
		slices.Sort(result)
		return slices.Compact(result)
	}
	frontier := []string{target}
	visited := map[string]bool{target: true}
	for depth := 0; depth < 8 && len(frontier) != 0; depth++ {
		var result, next []string
		for _, net := range frontier {
			for _, device := range plan.Devices {
				if !deviceTouchesNet(device, net) {
					continue
				}
				if matches(device) {
					result = append(result, device.Component)
					continue
				}
				if !stabilityPassivePathPrimitive(device.PrimitiveModel) {
					continue
				}
				for _, terminal := range device.Terminals {
					if terminal.Net == "" || visited[terminal.Net] {
						continue
					}
					visited[terminal.Net] = true
					next = append(next, terminal.Net)
				}
			}
		}
		if len(result) != 0 {
			slices.Sort(result)
			return slices.Compact(result)
		}
		slices.Sort(next)
		frontier = slices.Compact(next)
	}
	return nil
}

func behavioralObservationReferenceNode(requirement architecturesearch.Requirement, requirementID, operatingCase string, semanticBindings []SemanticBinding) (string, bool) {
	var observation architecturesearch.Observation
	found := false
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		if behavior.ID == requirementID {
			observation, found = behavior.Observation, true
			break
		}
	}
	if !found || observation.Kind == "circuit" {
		return "", false
	}
	if observation.Kind == "event" {
		found = false
		for _, candidate := range requirement.Requirements.OperatingCases {
			if candidate.ID != operatingCase {
				continue
			}
			for _, event := range candidate.Events {
				if event.ID == observation.ID {
					observation, found = event.Target, true
					break
				}
			}
			break
		}
		if !found || observation.Kind == "circuit" {
			return "", !found
		}
	}
	domainID := ""
	switch observation.Kind {
	case "port":
		for _, port := range requirement.Requirements.Ports {
			if port.ID == observation.ID {
				domainID = port.Domain
				break
			}
		}
	case "signal":
		for _, signal := range requirement.Requirements.Signals {
			if signal.ID == observation.ID {
				domainID = signal.Domain
				break
			}
		}
	case "domain":
		domainID = observation.ID
	}
	if domainID == "" {
		return "", true
	}
	referenceID, ok := behavioralReferenceDomain(requirement, domainID)
	if !ok {
		return "", true
	}
	for _, binding := range semanticBindings {
		if binding.Kind == "domain" && binding.ID == referenceID {
			return binding.Target, binding.Target != ""
		}
	}
	return "", true
}

func behavioralReferenceDomain(requirement architecturesearch.Requirement, domainID string) (string, bool) {
	var references []string
	for _, domain := range requirement.Requirements.Domains {
		if domain.ID == domainID && domain.Kind == "reference" {
			return domain.ID, true
		}
		if domain.Kind == "reference" {
			references = append(references, domain.ID)
		}
	}
	slices.Sort(references)
	if len(references) == 0 {
		return "", false
	}
	if len(references) == 1 {
		return references[0], true
	}
	if reference, ok := objectiveOutputReferenceDomain(requirement, domainID); ok {
		return reference, true
	}
	domainTokens := behavioralDomainTokens(domainID)
	best, bestScore, ambiguous := "", 0, false
	for _, reference := range references {
		score := sharedBehavioralDomainTokens(domainTokens, behavioralDomainTokens(reference))
		switch {
		case score > bestScore:
			best, bestScore, ambiguous = reference, score, false
		case score == bestScore && score > 0:
			ambiguous = true
		}
	}
	return best, bestScore > 0 && !ambiguous
}

func objectiveOutputReferenceDomain(requirement architecturesearch.Requirement, domainID string) (string, bool) {
	portDomains := make(map[string]string, len(requirement.Requirements.Ports))
	referencePorts := map[string]string{}
	for _, port := range requirement.Requirements.Ports {
		portDomains[port.ID] = port.Domain
		if port.Kind == "reference" {
			referencePorts[port.ID] = port.Domain
		}
	}
	signalDomains := make(map[string]string, len(requirement.Requirements.Signals))
	for _, signal := range requirement.Requirements.Signals {
		signalDomains[signal.ID] = signal.Domain
	}
	candidates := map[string]bool{}
	for _, objective := range requirement.Requirements.Objectives {
		producesDomain := false
		for _, binding := range objective.Bindings {
			bindingDomain := portDomains[binding.Port]
			if binding.Signal != "" {
				bindingDomain = signalDomains[binding.Signal]
			}
			if bindingDomain != domainID {
				continue
			}
			switch binding.Role {
			case "output", "load", "protected", "side_a", "side_b":
				producesDomain = true
			default:
				producesDomain = binding.Direction == "source"
			}
			if producesDomain {
				break
			}
		}
		if !producesDomain {
			continue
		}
		for _, binding := range objective.Bindings {
			switch binding.Role {
			case "reference", "ground", "return":
			default:
				continue
			}
			if referenceDomain := referencePorts[binding.Port]; referenceDomain != "" {
				candidates[referenceDomain] = true
			}
		}
	}
	if len(candidates) != 1 {
		return "", false
	}
	for candidate := range candidates {
		return candidate, true
	}
	return "", false
}

func behavioralDomainTokens(value string) []string {
	tokens := strings.FieldsFunc(strings.ToLower(value), func(candidate rune) bool {
		return candidate < 'a' || candidate > 'z'
	})
	filtered := tokens[:0]
	for _, token := range tokens {
		switch token {
		case "", "v", "volt", "volts", "supply", "power", "rail", "ground", "gnd", "reference", "return":
			continue
		default:
			filtered = append(filtered, token)
		}
	}
	return slices.Compact(filtered)
}

func sharedBehavioralDomainTokens(left, right []string) int {
	rightSet := map[string]bool{}
	for _, token := range right {
		rightSet[token] = true
	}
	count := 0
	for _, token := range left {
		if rightSet[token] {
			count++
		}
	}
	return count
}

func uniqueOperatingSourceForAxis(bindings []SimulationOperatingBinding, axis string, plan simmodel.Plan) (string, bool) {
	var candidates []string
	for _, binding := range bindings {
		if binding.Axis != axis || (binding.Kind != OperatingSourceDCValue && binding.Kind != OperatingLoadCurrent) || binding.Component == "" {
			continue
		}
		for _, device := range plan.Devices {
			if device.Component != binding.Component {
				continue
			}
			switch device.PrimitiveModel {
			case simmodel.PrimitiveVoltageSourceV1, simmodel.PrimitiveConnectorVoltageSourceV1, simmodel.PrimitiveCurrentSourceV1:
				candidates = append(candidates, binding.Component)
			}
			break
		}
	}
	slices.Sort(candidates)
	return uniqueString(slices.Compact(candidates))
}

func uniqueOperatingLoadForTarget(bindings []SimulationOperatingBinding, target string) (string, bool) {
	var candidates []string
	for _, binding := range bindings {
		if binding.Axis != "load_current" && binding.Axis != "load_resistance" {
			continue
		}
		if target != "" && target != "circuit" && binding.Target != target {
			continue
		}
		if binding.Component != "" {
			candidates = append(candidates, binding.Component)
		}
	}
	slices.Sort(candidates)
	return uniqueString(slices.Compact(candidates))
}

func semanticSupplyNodes(requirement architecturesearch.Requirement, bindings []SemanticBinding) []string {
	targets := make(map[string]string, len(bindings))
	for _, binding := range bindings {
		targets[binding.Kind+"\x00"+binding.ID] = binding.Target
	}
	var nodes []string
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind != "supply" || domain.Source != "external" {
			continue
		}
		if target := targets["domain\x00"+domain.ID]; target != "" {
			nodes = append(nodes, target)
		}
	}
	slices.Sort(nodes)
	return slices.Compact(nodes)
}

func supplySourceComponents(plan simmodel.Plan, supplyNodes []string) ([]string, bool) {
	if len(supplyNodes) == 0 {
		return nil, false
	}
	var result []string
	for _, node := range supplyNodes {
		component, ok := uniqueSourceComponent(plan, node)
		if !ok {
			return nil, false
		}
		result = append(result, component)
	}
	slices.Sort(result)
	result = slices.Compact(result)
	return result, len(result) != 0
}

func resolvedMuteExcitationOverride(requirement architecturesearch.Requirement, bindings []SemanticBinding, plan simmodel.Plan) (SimulationExcitationOverride, bool) {
	_, muted, ok := ResolveMuteExcitationStates(requirement, bindings, plan)
	return muted, ok
}

// ResolveMuteExcitationStates derives normal and muted control levels from
// semantic mute endpoints and trusted resolved switch topology. A series relay
// is energized for normal signal transfer and de-energized for fail-safe mute;
// other mute actuators retain the public active-high convention.
func ResolveMuteExcitationStates(requirement architecturesearch.Requirement, bindings []SemanticBinding, plan simmodel.Plan) (SimulationExcitationOverride, SimulationExcitationOverride, bool) {
	portID := ""
	inputTarget, outputTarget := "", ""
	for _, objective := range requirement.Requirements.Objectives {
		if objective.Capability != "mute_control" {
			continue
		}
		for _, binding := range objective.Bindings {
			if binding.Port != "" && (binding.Role == "control" || binding.Role == "mute") {
				if portID != "" && portID != binding.Port {
					return SimulationExcitationOverride{}, SimulationExcitationOverride{}, false
				}
				portID = binding.Port
			}
			target := semanticBindingTarget(bindings, binding)
			switch binding.Role {
			case "signal", "input":
				if inputTarget != "" && inputTarget != target {
					return SimulationExcitationOverride{}, SimulationExcitationOverride{}, false
				}
				inputTarget = target
			case "output":
				if outputTarget != "" && outputTarget != target {
					return SimulationExcitationOverride{}, SimulationExcitationOverride{}, false
				}
				outputTarget = target
			}
		}
	}
	if portID == "" {
		return SimulationExcitationOverride{}, SimulationExcitationOverride{}, false
	}
	target := ""
	for _, binding := range bindings {
		if binding.Kind == "port" && binding.ID == portID {
			target = binding.Target
			break
		}
	}
	domainID := ""
	for _, port := range requirement.Requirements.Ports {
		if port.ID == portID {
			domainID = port.Domain
			break
		}
	}
	activeV := 0.0
	for _, domain := range requirement.Requirements.Domains {
		if domain.ID == domainID {
			activeV = domain.NominalVoltageV
			break
		}
	}
	component, ok := uniqueSourceComponent(plan, target)
	if !ok || activeV <= 0 || math.IsNaN(activeV) || math.IsInf(activeV, 0) {
		return SimulationExcitationOverride{}, SimulationExcitationOverride{}, false
	}
	polarity, ok := resolvedVoltageSourcePolarity(plan, component, target)
	if !ok {
		return SimulationExcitationOverride{}, SimulationExcitationOverride{}, false
	}
	asserted := SimulationExcitationOverride{Component: component, DCValue: activeV * polarity}
	deasserted := SimulationExcitationOverride{Component: component, DCValue: 0}
	if resolvedSeriesRelay(plan, inputTarget, outputTarget) {
		return asserted, deasserted, true
	}
	return deasserted, asserted, true
}

func semanticBindingTarget(bindings []SemanticBinding, binding architecturesearch.Binding) string {
	kind, id := "", ""
	if binding.Port != "" {
		kind, id = "port", binding.Port
	} else if binding.Signal != "" {
		kind, id = "signal", binding.Signal
	}
	for _, candidate := range bindings {
		if candidate.Kind == kind && candidate.ID == id {
			return candidate.Target
		}
	}
	return ""
}

func resolvedSeriesRelay(plan simmodel.Plan, input, output string) bool {
	if input == "" || output == "" || input == output {
		return false
	}
	found := false
	for _, device := range plan.Devices {
		if device.PrimitiveModel != simmodel.PrimitiveRelayClosedV1 && device.PrimitiveModel != simmodel.PrimitiveRelayNormallyOpenV1 {
			continue
		}
		terminals := map[string]string{}
		for _, terminal := range device.Terminals {
			terminals[terminal.Terminal] = terminal.Net
		}
		spans := (terminals["CONTACT_IN"] == input && terminals["CONTACT_OUT"] == output) || (terminals["CONTACT_IN"] == output && terminals["CONTACT_OUT"] == input)
		if !spans {
			continue
		}
		if found {
			return false
		}
		found = true
	}
	return found
}

func resolvedVoltageSourcePolarity(plan simmodel.Plan, component, node string) (float64, bool) {
	for _, device := range plan.Devices {
		if device.Component != component {
			continue
		}
		terminals := map[string]string{}
		for _, terminal := range device.Terminals {
			terminals[terminal.Terminal] = terminal.Net
		}
		switch device.PrimitiveModel {
		case simmodel.PrimitiveVoltageSourceV1:
			if terminals["POSITIVE"] == node {
				return 1, true
			}
			if terminals["NEGATIVE"] == node {
				return -1, true
			}
		case simmodel.PrimitiveConnectorVoltageSourceV1:
			if terminals["PIN_1"] == node {
				return 1, true
			}
			if terminals["PIN_2"] == node {
				return -1, true
			}
		}
		return 0, false
	}
	return 0, false
}

func stabilityObservationNode(plan simmodel.Plan, target string) (string, bool) {
	if target == "" {
		return "", false
	}
	blocked := map[string]bool{plan.GroundNode: true}
	for _, device := range plan.Devices {
		switch device.PrimitiveModel {
		case simmodel.PrimitiveVoltageSourceV1, simmodel.PrimitiveConnectorVoltageSourceV1, simmodel.PrimitiveCurrentSourceV1:
			for _, terminal := range device.Terminals {
				blocked[terminal.Net] = true
			}
		}
	}
	delete(blocked, target)
	visited := map[string]bool{target: true}
	frontier := []string{target}
	var bjtFallbacks []string
	for len(frontier) != 0 {
		var opAmpOutputs, buckOutputs, bjtOutputs []string
		for _, net := range frontier {
			for _, device := range plan.Devices {
				if device.PrimitiveModel == simmodel.PrimitiveSynchronousBuckRegulatorV1 {
					if output, ok := synchronousBuckObservationNode(plan, device); ok && output == net {
						buckOutputs = append(buckOutputs, output)
					}
				}
				for _, terminal := range device.Terminals {
					if terminal.Net != net {
						continue
					}
					if device.PrimitiveModel == simmodel.PrimitiveOpAmpV1 && terminal.Terminal == "OUT" {
						opAmpOutputs = append(opAmpOutputs, net)
					}
					if (device.PrimitiveModel == simmodel.PrimitiveBJTNPNV1 || device.PrimitiveModel == simmodel.PrimitiveBJTPNPV1) && terminal.Terminal == "COLLECTOR" && bjtHasEmitterDegenerationEvidence(plan, device) {
						bjtOutputs = append(bjtOutputs, net)
					}
				}
			}
		}
		if len(opAmpOutputs) != 0 {
			return uniqueString(opAmpOutputs)
		}
		if len(buckOutputs) != 0 {
			return uniqueString(buckOutputs)
		}
		if len(bjtOutputs) != 0 {
			// A complementary compound output can expose both driver
			// collectors at the observed node while its controlling op-amp is
			// still uniquely reachable through the base/emitter paths. Retain
			// transistor stages as a fallback for discrete loops, but continue
			// the bounded traversal so the actual op-amp loop is preferred.
			bjtFallbacks = append(bjtFallbacks, bjtOutputs...)
		}
		var next []string
		for _, net := range frontier {
			for _, device := range plan.Devices {
				candidateNets := stabilityPassivePathNets(device, net)
				if len(candidateNets) == 0 {
					candidateNets = stabilityActiveOutputPathNets(device, net)
				}
				for _, candidateNet := range candidateNets {
					if candidateNet == net || blocked[candidateNet] || visited[candidateNet] {
						continue
					}
					visited[candidateNet] = true
					next = append(next, candidateNet)
				}
			}
		}
		slices.Sort(next)
		frontier = next
	}
	return uniqueString(bjtFallbacks)
}

func stabilityPassivePathNets(device simmodel.ResolvedDevice, net string) []string {
	if !stabilityPassivePathPrimitive(device.PrimitiveModel) || !deviceTouchesNet(device, net) {
		return nil
	}
	terminals := map[string]string{}
	for _, terminal := range device.Terminals {
		terminals[terminal.Terminal] = terminal.Net
	}
	switch device.PrimitiveModel {
	case simmodel.PrimitiveNMOSSwitchV1, simmodel.PrimitivePMOSSwitchV1:
		switch net {
		case terminals["DRAIN"]:
			return []string{terminals["SOURCE"]}
		case terminals["SOURCE"]:
			return []string{terminals["DRAIN"]}
		default:
			return nil
		}
	case simmodel.PrimitiveRelayClosedV1, simmodel.PrimitiveRelayNormallyOpenV1:
		switch net {
		case terminals["CONTACT_IN"]:
			return []string{terminals["CONTACT_OUT"]}
		case terminals["CONTACT_OUT"]:
			return []string{terminals["CONTACT_IN"]}
		default:
			return nil
		}
	default:
		var result []string
		for _, terminal := range device.Terminals {
			result = append(result, terminal.Net)
		}
		return result
	}
}

func synchronousBuckObservationNode(plan simmodel.Plan, controller simmodel.ResolvedDevice) (string, bool) {
	switchNode := ""
	for _, terminal := range controller.Terminals {
		if terminal.Terminal == "SW" {
			switchNode = terminal.Net
			break
		}
	}
	if switchNode == "" {
		return "", false
	}
	var outputs []string
	for _, device := range plan.Devices {
		if device.PrimitiveModel != simmodel.PrimitiveInductorTransientV1 {
			continue
		}
		terminals := map[string]string{}
		for _, terminal := range device.Terminals {
			terminals[terminal.Terminal] = terminal.Net
		}
		switch {
		case terminals["A"] == switchNode && terminals["B"] != "":
			outputs = append(outputs, terminals["B"])
		case terminals["B"] == switchNode && terminals["A"] != "":
			outputs = append(outputs, terminals["A"])
		}
	}
	slices.Sort(outputs)
	return uniqueString(slices.Compact(outputs))
}

func stabilityActiveOutputPathNets(device simmodel.ResolvedDevice, net string) []string {
	if device.PrimitiveModel != simmodel.PrimitiveBJTNPNV1 && device.PrimitiveModel != simmodel.PrimitiveBJTPNPV1 {
		return nil
	}
	transition := 0.0
	terminals := map[string]string{}
	for _, parameter := range device.ModelParameters {
		if parameter.Name == "transition_frequency_hz" {
			transition = parameter.Value
		}
	}
	for _, terminal := range device.Terminals {
		terminals[terminal.Terminal] = terminal.Net
	}
	if transition <= 0 || terminals["BASE"] == "" || terminals["EMITTER"] == "" {
		return nil
	}
	switch net {
	case terminals["COLLECTOR"], terminals["EMITTER"]:
		// Walk a BJT signal path backward from either possible output
		// terminal to its controlling base. Supply-connected collectors are
		// already excluded by the traversal's blocked source-net set.
		return []string{terminals["BASE"]}
	default:
		return nil
	}
}

func bjtHasEmitterDegenerationEvidence(plan simmodel.Plan, device simmodel.ResolvedDevice) bool {
	transition := 0.0
	for _, parameter := range device.ModelParameters {
		if parameter.Name == "transition_frequency_hz" {
			transition = parameter.Value
			break
		}
	}
	if transition <= 0 {
		return false
	}
	emitter := ""
	for _, terminal := range device.Terminals {
		if terminal.Terminal == "EMITTER" {
			emitter = terminal.Net
			break
		}
	}
	if emitter == "" || emitter == plan.GroundNode {
		return false
	}
	for _, candidate := range plan.Devices {
		if stabilityPassivePathPrimitive(candidate.PrimitiveModel) && deviceTouchesNet(candidate, emitter) {
			return true
		}
	}
	return false
}

func stabilityPassivePathPrimitive(primitive string) bool {
	switch primitive {
	case simmodel.PrimitiveResistorV1,
		simmodel.PrimitiveFuseClosedStateV1,
		simmodel.PrimitiveRelayClosedV1,
		simmodel.PrimitiveRelayNormallyOpenV1,
		simmodel.PrimitiveNMOSSwitchV1,
		simmodel.PrimitivePMOSSwitchV1,
		simmodel.PrimitiveCapacitorV1,
		simmodel.PrimitiveCapacitorTransientV1,
		simmodel.PrimitiveInductorTransientV1,
		simmodel.PrimitiveBidirectionalTVSV1,
		simmodel.PrimitiveUnidirectionalZenerV1,
		simmodel.PrimitiveDiodeShockleyV1:
		return true
	default:
		return false
	}
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
			case "cooling_mode", "tolerance", "model_parameter":
				binding.Kind = OperatingWorstCase
			case "supply_voltage", "input_amplitude":
				component, ok := uniqueVoltageSourceAcrossPlans(plans, assignment.Target)
				if !ok {
					*diagnostics = append(*diagnostics, Diagnostic{Path: "corners." + corner.ID + "." + assignment.Axis, Message: "operating source target is missing or ambiguous"})
					continue
				}
				binding.Kind, binding.Component = OperatingSourceDCValue, component
			case "input_frequency":
				component, ok := uniqueVoltageSourceAcrossPlans(plans, assignment.Target)
				if !ok {
					*diagnostics = append(*diagnostics, Diagnostic{Path: "corners." + corner.ID + ".input_frequency", Message: "operating input-frequency target is missing or ambiguous"})
					continue
				}
				binding.Kind, binding.Component = OperatingSourceFrequencyHz, component
			case "load_current":
				component := OperatingHarnessComponentID(assignment.Axis, assignment.Target)
				maximum, maximumOK := maximumOperatingAssignment(analysisPlan, assignment.Axis, assignment.Target)
				scale, offset, scaleIssue := resolvedLoadCurrentTransfer(plans, component, maximum)
				if !maximumOK || scaleIssue != "" {
					if !maximumOK {
						scaleIssue = "declared current corners have no positive finite maximum"
					}
					*diagnostics = append(*diagnostics, Diagnostic{Path: "corners." + corner.ID + ".load_current", Message: "catalog-backed current load or its equivalent physical-load scale is missing or ambiguous: " + scaleIssue})
					continue
				}
				binding.Kind, binding.Component, binding.Scale, binding.Offset = OperatingLoadCurrent, component, scale, offset
				binding.ReferenceComponent, _ = resolvedLoadCurrentReference(plans, scale)
			case "load_resistance":
				component := OperatingHarnessComponentID(assignment.Axis, assignment.Target)
				if !deviceComponentFamilyAcrossPlans(plans, component, "resistor") {
					var ok bool
					component, ok = uniqueDeviceAcrossPlans(plans, assignment.Target, "resistor")
					if !ok {
						*diagnostics = append(*diagnostics, Diagnostic{Path: "corners." + corner.ID + ".load_resistance", Message: "load resistance target is missing or ambiguous"})
						continue
					}
				}
				binding.Kind, binding.Component = OperatingDeviceValueSI, component
			case "load_capacitance":
				component := OperatingHarnessComponentID(assignment.Axis, assignment.Target)
				if !deviceComponentFamilyAcrossPlans(plans, component, "capacitor") {
					var ok bool
					component, ok = uniqueDeviceAcrossPlans(plans, assignment.Target, "capacitor")
					if !ok {
						*diagnostics = append(*diagnostics, Diagnostic{Path: "corners." + corner.ID + ".load_capacitance", Message: "load capacitance target is missing or ambiguous"})
						continue
					}
				}
				binding.Kind, binding.Component = OperatingDeviceValueSI, component
			case "load_inductance":
				component := OperatingHarnessComponentID(assignment.Axis, assignment.Target)
				if !deviceComponentFamilyAcrossPlans(plans, component, "inductor") {
					var ok bool
					component, ok = uniqueDeviceAcrossPlans(plans, assignment.Target, "inductor")
					if !ok {
						*diagnostics = append(*diagnostics, Diagnostic{Path: "corners." + corner.ID + ".load_inductance", Message: "load inductance target is missing or ambiguous"})
						continue
					}
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

func maximumOperatingAssignment(plan AnalysisPlan, axis, target string) (float64, bool) {
	maximum := math.Inf(-1)
	for _, corner := range plan.Corners {
		for _, assignment := range corner.Assignments {
			if assignment.Axis == axis && assignment.Target == target && assignment.Value != nil && finiteClosedLoopBound(*assignment.Value) {
				maximum = math.Max(maximum, *assignment.Value)
			}
		}
	}
	return maximum, finiteClosedLoopBound(maximum) && maximum > 0
}

// resolvedLoadCurrentTransfer proves the affine mapping from a semantic total
// rail current to the independently driven physical harness load. Catalog-backed
// parallel support current is already present in the plan, so it appears as a
// negative offset. Startup-safe resistance plans must recover the same voltage
// scale from the residual maximum current.
func resolvedLoadCurrentTransfer(plans map[string]simmodel.Plan, component string, maximumCurrent float64) (float64, float64, string) {
	if !finiteClosedLoopBound(maximumCurrent) || maximumCurrent <= 0 {
		return 0, 0, "maximum current is not positive and finite"
	}
	keys := make([]string, 0, len(plans))
	for key := range plans {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	physicalMaximum := 0.0
	for _, key := range keys {
		for _, device := range plans[key].Devices {
			if device.Component != component || device.Family != "current_source" {
				continue
			}
			candidate, ok := maximumLoadSourceExcitation(plans[key], component)
			if !ok {
				return 0, 0, "current-source load has no finite nonnegative excitation in " + key
			}
			if physicalMaximum != 0 && math.Abs(candidate-physicalMaximum) > 1e-12*math.Max(1, math.Abs(physicalMaximum)) {
				return 0, 0, "physical load-current maxima disagree across analysis plans"
			}
			physicalMaximum = candidate
		}
	}
	if physicalMaximum == 0 {
		physicalMaximum = maximumCurrent
	}
	offset := physicalMaximum - maximumCurrent
	if !finiteClosedLoopBound(offset) || physicalMaximum <= 0 {
		return 0, 0, "resolved physical load-current maximum is not positive and finite"
	}
	scale := 0.0
	for _, key := range keys {
		found := false
		for _, device := range plans[key].Devices {
			if device.Component != component {
				continue
			}
			switch device.Family {
			case "current_source":
				candidate, ok := maximumLoadSourceExcitation(plans[key], component)
				if !ok || math.Abs(candidate-physicalMaximum) > 1e-12*math.Max(1, math.Abs(physicalMaximum)) {
					return 0, 0, "physical load-current maximum is missing or inconsistent in " + key
				}
				found = true
			case "resistor":
				if device.ValueSI == nil || !finiteClosedLoopBound(*device.ValueSI) || *device.ValueSI <= 0 {
					return 0, 0, "startup load resistance is not positive and finite in " + key
				}
				candidate := *device.ValueSI * physicalMaximum
				if scale != 0 && math.Abs(candidate-scale) > 1e-12*math.Max(1, math.Abs(scale)) {
					return 0, 0, "physical load voltage scales disagree across analysis plans"
				}
				scale, found = candidate, true
			default:
				return 0, 0, "load component " + component + " has unsupported family " + device.Family + " in " + key
			}
		}
		if !found {
			return 0, 0, "load component " + component + " is absent from " + key
		}
	}
	return scale, offset, ""
}

func maximumLoadSourceExcitation(plan simmodel.Plan, component string) (float64, bool) {
	maximum := math.Inf(-1)
	found := false
	for _, analysis := range plan.Analyses {
		for _, excitation := range analysis.Excitations {
			if excitation.Component != component {
				continue
			}
			for _, candidate := range []float64{excitation.DCValue, excitation.PulseInitialValue, excitation.PulseValue} {
				if finiteClosedLoopBound(candidate) && candidate >= 0 {
					maximum = math.Max(maximum, candidate)
					found = true
				}
			}
		}
	}
	return maximum, found
}

func resolvedLoadCurrentReference(plans map[string]simmodel.Plan, scale float64) (string, bool) {
	if !finiteClosedLoopBound(scale) || scale <= 0 {
		return "", false
	}
	keys := make([]string, 0, len(plans))
	for key := range plans {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	reference := ""
	for _, key := range keys {
		var candidates []string
		for _, analysis := range plans[key].Analyses {
			for _, excitation := range analysis.Excitations {
				if math.Abs(math.Abs(excitation.DCValue)-scale) <= 1e-12*math.Max(1, scale) {
					candidates = append(candidates, excitation.Component)
				}
			}
		}
		slices.Sort(candidates)
		candidates = slices.Compact(candidates)
		if len(candidates) != 1 || reference != "" && reference != candidates[0] {
			return "", false
		}
		reference = candidates[0]
	}
	return reference, reference != ""
}

// OperatingHarnessComponentID gives every operating axis/semantic target one
// stable testbench identity without leaking requirement-specific names into
// catalogs, schemas, or physical writer output.
func OperatingHarnessComponentID(axis, target string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(axis) + "\x00" + strings.TrimSpace(target)))
	return "simulation_harness_" + strings.TrimSpace(axis) + "_" + hex.EncodeToString(digest[:8])
}

func thermalComponentsForTarget(plan simmodel.Plan, target string) []string {
	if target == "circuit" {
		var result []string
		for _, device := range plan.Devices {
			if hasThermalPath(device.ModelParameters) {
				result = append(result, device.Component)
			}
		}
		slices.Sort(result)
		return slices.Compact(result)
	}
	frontier := []string{target}
	visited := map[string]bool{target: true}
	for depth := 0; depth < 8 && len(frontier) != 0; depth++ {
		var result, next []string
		for _, net := range frontier {
			for _, device := range plan.Devices {
				if !deviceTouchesNet(device, net) {
					continue
				}
				if hasThermalPath(device.ModelParameters) {
					result = append(result, device.Component)
					continue
				}
				if !stabilityPassivePathPrimitive(device.PrimitiveModel) {
					continue
				}
				for _, terminal := range device.Terminals {
					if terminal.Net == "" || visited[terminal.Net] {
						continue
					}
					visited[terminal.Net] = true
					next = append(next, terminal.Net)
				}
			}
		}
		if len(result) != 0 {
			slices.Sort(result)
			return slices.Compact(result)
		}
		slices.Sort(next)
		frontier = slices.Compact(next)
	}
	return nil
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
	harnessID := OperatingHarnessComponentID("load_resistance", target)
	for _, device := range plan.Devices {
		if device.Component == harnessID && device.Family == "resistor" && deviceTouchesNet(device, target) {
			return harnessID, true
		}
	}
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

func uniqueVoltageSourceAcrossPlans(plans map[string]simmodel.Plan, target string) (string, bool) {
	var candidates []string
	for _, plan := range plans {
		if component, ok := uniqueVoltageSourceComponent(plan, target); ok {
			candidates = append(candidates, component)
		}
	}
	slices.Sort(candidates)
	candidates = slices.Compact(candidates)
	return uniqueString(candidates)
}

func uniqueVoltageSourceComponent(plan simmodel.Plan, target string) (string, bool) {
	var candidates []string
	for _, device := range plan.Devices {
		if target != "" && !deviceTouchesNet(device, target) {
			continue
		}
		switch device.PrimitiveModel {
		case simmodel.PrimitiveVoltageSourceV1, simmodel.PrimitiveConnectorVoltageSourceV1:
			candidates = append(candidates, device.Component)
		}
	}
	slices.Sort(candidates)
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

func deviceComponentFamilyAcrossPlans(plans map[string]simmodel.Plan, component, family string) bool {
	if len(plans) == 0 || component == "" {
		return false
	}
	for _, plan := range plans {
		found := false
		for _, device := range plan.Devices {
			if device.Component == component && device.Family == family {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
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

func uniqueDeviceFamilyInPlan(plan simmodel.Plan, family string) (string, bool) {
	var candidates []string
	for _, device := range plan.Devices {
		if device.Family == family {
			candidates = append(candidates, device.Component)
		}
	}
	slices.Sort(candidates)
	return uniqueString(candidates)
}

func uniqueString(values []string) (string, bool) {
	if len(values) == 0 {
		return "", false
	}
	canonical := append([]string(nil), values...)
	slices.Sort(canonical)
	canonical = slices.Compact(canonical)
	if len(canonical) != 1 {
		return "", false
	}
	return canonical[0], true
}

func deviceTouchesNet(device simmodel.ResolvedDevice, target string) bool {
	for _, terminal := range device.Terminals {
		if terminal.Net == target {
			return true
		}
	}
	return false
}
