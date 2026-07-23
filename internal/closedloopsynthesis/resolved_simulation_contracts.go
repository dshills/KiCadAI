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
	seenAssertions := map[string]bool{}
	for _, assertion := range analysisPlan.Assertions {
		key := assertion.Metric + "\x00" + assertion.Target
		if seenAssertions[key] {
			continue
		}
		seenAssertions[key] = true
		binding, diagnostic := resolvedAssertionBinding(assertion, referenceNode, supplyNodes, operatingBindings, plans[assertionAnalysisKind(analysisPlan, assertion.AnalysisID)], requirement, analysisPlan.Bindings)
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
		loopNode, ok := stabilityObservationNode(plan, assertion.Target)
		if !ok {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "phase margin target does not resolve through a unique passive output path to a trusted op-amp or emitter-degenerated BJT loop"}
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
	case "muted_output_voltage":
		prototype.Quantity, binding.BoundsMode = simmodel.QuantityPeakAbsVoltageV, AssertionBoundsAbsolute
		override, ok := resolvedMuteExcitationOverride(requirement, semanticBindings, plan)
		if !ok {
			return binding, &Diagnostic{Path: "assertions." + assertion.RequirementID, Message: "muted-output measurement requires one resolved active-high mute-control source"}
		}
		binding.ExcitationOverrides = []SimulationExcitationOverride{override}
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
	binding.Prototypes = []simmodel.Assertion{prototype}
	return binding, nil
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

func semanticSupplyNodes(requirement architecturesearch.Requirement, bindings []SemanticBinding) []string {
	targets := make(map[string]string, len(bindings))
	for _, binding := range bindings {
		targets[binding.Kind+"\x00"+binding.ID] = binding.Target
	}
	var nodes []string
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind != "supply" {
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
		var opAmpOutputs, bjtOutputs []string
		for _, net := range frontier {
			for _, device := range plan.Devices {
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
				candidateNets := []string{}
				if stabilityPassivePathPrimitive(device.PrimitiveModel) && deviceTouchesNet(device, net) {
					for _, terminal := range device.Terminals {
						candidateNets = append(candidateNets, terminal.Net)
					}
				} else {
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
		simmodel.PrimitiveCapacitorV1,
		simmodel.PrimitiveCapacitorTransientV1,
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
			case "tolerance", "model_parameter":
				binding.Kind = OperatingWorstCase
			case "supply_voltage", "input_amplitude":
				component, ok := uniqueVoltageSourceAcrossPlans(plans, assignment.Target)
				if !ok {
					*diagnostics = append(*diagnostics, Diagnostic{Path: "corners." + corner.ID + "." + assignment.Axis, Message: "operating source target is missing or ambiguous"})
					continue
				}
				binding.Kind, binding.Component = OperatingSourceDCValue, component
			case "load_current":
				component := OperatingHarnessComponentID(assignment.Axis, assignment.Target)
				maximum, maximumOK := maximumOperatingAssignment(analysisPlan, assignment.Axis, assignment.Target)
				scale, scaleIssue := resolvedLoadCurrentScale(plans, component, maximum)
				if !maximumOK || scaleIssue != "" {
					if !maximumOK {
						scaleIssue = "declared current corners have no positive finite maximum"
					}
					*diagnostics = append(*diagnostics, Diagnostic{Path: "corners." + corner.ID + ".load_current", Message: "catalog-backed current load or its equivalent physical-load scale is missing or ambiguous: " + scaleIssue})
					continue
				}
				binding.Kind, binding.Component, binding.Scale = OperatingLoadCurrent, component, scale
			case "load_resistance":
				component := OperatingHarnessComponentID(assignment.Axis, assignment.Target)
				if !deviceComponentAcrossPlans(plans, component, assignment.Target, "resistor") {
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
				if !deviceComponentAcrossPlans(plans, component, assignment.Target, "capacitor") {
					var ok bool
					component, ok = uniqueDeviceAcrossPlans(plans, assignment.Target, "capacitor")
					if !ok {
						*diagnostics = append(*diagnostics, Diagnostic{Path: "corners." + corner.ID + ".load_capacitance", Message: "load capacitance target is missing or ambiguous"})
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

// resolvedLoadCurrentScale proves that every analysis plan represents the same
// semantic current load either as an independently driven current source or as
// a startup-safe physical resistance. The latter is derived at the maximum
// declared current, so R*I recovers the deterministic voltage scale used to
// compile every current corner without embedding a topology-specific formula
// in provider output.
func resolvedLoadCurrentScale(plans map[string]simmodel.Plan, component string, maximumCurrent float64) (float64, string) {
	if !finiteClosedLoopBound(maximumCurrent) || maximumCurrent <= 0 {
		return 0, "maximum current is not positive and finite"
	}
	keys := make([]string, 0, len(plans))
	for key := range plans {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	scale := 0.0
	for _, key := range keys {
		found := false
		for _, device := range plans[key].Devices {
			if device.Component != component {
				continue
			}
			switch device.Family {
			case "current_source":
				found = true
			case "resistor":
				if device.ValueSI == nil || !finiteClosedLoopBound(*device.ValueSI) || *device.ValueSI <= 0 {
					return 0, "startup load resistance is not positive and finite in " + key
				}
				candidate := *device.ValueSI * maximumCurrent
				if scale != 0 && math.Abs(candidate-scale) > 1e-12*math.Max(1, math.Abs(scale)) {
					return 0, "physical load voltage scales disagree across analysis plans"
				}
				scale, found = candidate, true
			default:
				return 0, "load component " + component + " has unsupported family " + device.Family + " in " + key
			}
		}
		if !found {
			return 0, "load component " + component + " is absent from " + key
		}
	}
	return scale, ""
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

func deviceComponentAcrossPlans(plans map[string]simmodel.Plan, component, target, family string) bool {
	if len(plans) == 0 || component == "" {
		return false
	}
	for _, plan := range plans {
		found := false
		for _, device := range plan.Devices {
			if device.Component == component && device.Family == family && deviceTouchesNet(device, target) {
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
