package opentopologysynthesis

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"

	"kicadai/internal/simmodel"
)

// topologyNonlinearSwitchingRelationshipSeeds composes relationship operators
// selected only from behavioral obligations. Each operator still produces a
// primitive graph that passes the ordinary completeness, scoring, simulation,
// and physical-promotion boundaries.
func topologyNonlinearSwitchingRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	result := []TopologyCandidate{}
	consumption := Consumption{}
	rejections := map[string][]string{}
	operators := []func() ([]TopologyCandidate, Consumption, map[string][]string){
		func() ([]TopologyCandidate, Consumption, map[string][]string) {
			return topologyBoundedBipolarRelationshipSeeds(ctx, requirement, inventory, inventoryByKey, limits, policy, initial)
		},
		func() ([]TopologyCandidate, Consumption, map[string][]string) {
			return topologyAutonomousPeriodicRelationshipSeeds(ctx, requirement, inventory, inventoryByKey, limits, policy, initial)
		},
		func() ([]TopologyCandidate, Consumption, map[string][]string) {
			return topologyStepDownRelationshipSeeds(ctx, requirement, inventory, representatives, inventoryByKey, limits, policy, initial)
		},
	}
	for _, operator := range operators {
		candidates, used, rejected := operator()
		result = append(result, candidates...)
		addSearchConsumption(&consumption, used)
		for code, samples := range rejected {
			rejections[code] = append(rejections[code], samples...)
		}
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

type boundedBipolarEnvelope struct {
	input             string
	output            string
	minimumGain       float64
	maximumInputAbsV  float64
	maximumOutputAbsV float64
	minimumLoadOhm    float64
}

func topologyBoundedBipolarEnvelope(requirement Requirement) (boundedBipolarEnvelope, bool) {
	envelope := boundedBipolarEnvelope{minimumLoadOhm: math.Inf(1)}
	positiveBound, negativeBound, passTransfer := false, false, false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Excitation != nil && assertion.Excitation.Kind == "port" &&
			assertion.Observation.Kind == "port" && assertion.Metric == "voltage_gain" &&
			assertion.Analysis == "dc_sweep" && assertion.Min != nil && *assertion.Min > 0 && *assertion.Min <= 1.1 {
			envelope.input = assertion.Excitation.ID
			envelope.output = assertion.Observation.ID
			envelope.minimumGain = *assertion.Min
			passTransfer = true
		}
		if assertion.Metric != "output_voltage" || assertion.Observation.Kind != "port" {
			continue
		}
		if envelope.output == "" {
			envelope.output = assertion.Observation.ID
		}
		if assertion.Max != nil && *assertion.Max < 0 {
			negativeBound = true
			envelope.maximumOutputAbsV = math.Max(envelope.maximumOutputAbsV, math.Abs(*assertion.Max))
			if assertion.Min != nil {
				envelope.maximumOutputAbsV = math.Max(envelope.maximumOutputAbsV, math.Abs(*assertion.Min))
			}
		}
		if assertion.Min != nil && *assertion.Min > 0 {
			positiveBound = true
			envelope.maximumOutputAbsV = math.Max(envelope.maximumOutputAbsV, math.Abs(*assertion.Min))
			if assertion.Max != nil {
				envelope.maximumOutputAbsV = math.Max(envelope.maximumOutputAbsV, math.Abs(*assertion.Max))
			}
		}
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			switch {
			case condition.Axis == "input_voltage" && condition.Target == envelope.input:
				envelope.maximumInputAbsV = math.Max(envelope.maximumInputAbsV, math.Max(math.Abs(condition.Min), math.Abs(condition.Max)))
			case condition.Axis == "load_resistance" && condition.Target == envelope.output && condition.Min > 0:
				envelope.minimumLoadOhm = math.Min(envelope.minimumLoadOhm, condition.Min)
			}
		}
	}
	valid := passTransfer && positiveBound && negativeBound && envelope.input != "" && envelope.output != "" &&
		envelope.maximumInputAbsV > envelope.maximumOutputAbsV && finite(envelope.minimumLoadOhm)
	return envelope, valid
}

func topologyBoundedBipolarRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	envelope, required := topologyBoundedBipolarEnvelope(requirement)
	if !required {
		return nil, Consumption{}, map[string][]string{}
	}
	highRail, lowRail := topologyPowerRails(requirement, initial.graph)
	references := topologyNodesByRole(initial.graph, "reference")
	if lowRail == "" && len(references) == 1 {
		lowRail = references[0]
	}
	if highRail == "" || lowRail == "" || len(references) != 1 {
		return nil, Consumption{}, map[string][]string{"relationship_gap": {"bounded bipolar transfer requires ordered positive and negative references plus one signal reference"}}
	}
	currentLimit := topologyBipolarReferenceCurrentLimit(requirement)
	forwardAllowance := envelope.maximumOutputAbsV - math.Max(
		math.Abs(topologyNodeNominalVoltageOrZero(requirement, initial.graph, highRail)),
		math.Abs(topologyNodeNominalVoltageOrZero(requirement, initial.graph, lowRail)),
	)
	diode := topologyLimiterDiodePrimitive(requirement, inventory, currentLimit, forwardAllowance)
	if diode.Key == "" || currentLimit <= 0 || forwardAllowance <= 0 {
		return nil, Consumption{}, map[string][]string{"relationship_gap": {"bounded bipolar transfer lacks a reviewed diode whose forward and thermal envelope fits the reference headroom"}}
	}
	minimumSeries := (envelope.maximumInputAbsV - envelope.maximumOutputAbsV) / currentLimit
	maximumSeries := envelope.minimumLoadOhm * (1/envelope.minimumGain - 1)
	if minimumSeries <= 0 || maximumSeries <= 0 || maximumSeries < minimumSeries {
		return nil, Consumption{}, map[string][]string{"relationship_gap": {fmt.Sprintf("bounded bipolar transfer has no series resistance satisfying clamp current and passband gain: %.12g..%.12g ohm", minimumSeries, maximumSeries)}}
	}
	// A pair of equal reviewed catalog resistors in parallel lets the
	// relationship realize a safe effective value even when the catalog has no
	// single part inside the derived interval. The two physical branches also
	// share clamp dissipation without introducing a special component family.
	series := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", 2*geometricMean(minimumSeries, maximumSeries))
	if series.Key == "" {
		return nil, Consumption{}, map[string][]string{"relationship_gap": {"bounded bipolar transfer requires a reviewed series resistor in the derived value range"}}
	}
	bias := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", math.Max(1e6, envelope.minimumLoadOhm*100))
	if bias.Key == "" {
		return nil, Consumption{}, map[string][]string{"relationship_gap": {"bounded bipolar transfer requires a reviewed bias resistor in the derived value range"}}
	}
	consumption := Consumption{ExpandedStates: 1}
	state := initial
	input, output := "port_"+envelope.input, "port_"+envelope.output
	state = addRelationshipPrimitive(state, requirement, inventoryByKey, series, topologyTwoTerminalPlacement(input, output), &consumption)
	state = addRelationshipPrimitive(state, requirement, inventoryByKey, series, topologyTwoTerminalPlacement(input, output), &consumption)
	state = addRelationshipPrimitive(state, requirement, inventoryByKey, diode, []TerminalConnection{{Terminal: "ANODE", Node: output}, {Terminal: "CATHODE", Node: highRail}}, &consumption)
	state = addRelationshipPrimitive(state, requirement, inventoryByKey, diode, []TerminalConnection{{Terminal: "ANODE", Node: lowRail}, {Terminal: "CATHODE", Node: output}}, &consumption)
	state = addRelationshipPrimitive(state, requirement, inventoryByKey, bias, topologyTwoTerminalPlacement(output, references[0]), &consumption)
	return finalizeNonlinearSwitchingRelationship(ctx, requirement, inventory, limits, policy, []topologySearchState{state}, consumption, "bounded bipolar clamp")
}

func topologyBipolarReferenceCurrentLimit(requirement Requirement) float64 {
	limit := math.Inf(1)
	positive, negative := false, false
	for _, domain := range requirement.Requirements.Domains {
		if domain.NominalVoltageV == nil || domain.MaxCurrentA == nil || *domain.MaxCurrentA <= 0 {
			continue
		}
		if *domain.NominalVoltageV > 0 {
			positive = true
			limit = math.Min(limit, *domain.MaxCurrentA)
		}
		if *domain.NominalVoltageV < 0 {
			negative = true
			limit = math.Min(limit, *domain.MaxCurrentA)
		}
	}
	if !positive || !negative || !finite(limit) {
		return 0
	}
	return limit
}

func topologyLimiterDiodePrimitive(requirement Requirement, inventory PrimitiveInventory, currentA, maximumForwardV float64) PrimitiveCandidate {
	type scored struct {
		primitive PrimitiveCandidate
		forwardV  float64
	}
	requireThermal := requirementMetricPresent(requirement, "junction_temperature")
	// requirementAnalysisSet always allocates a caller-owned map; removing
	// analyses that do not traverse the limiter cannot affect another search.
	requiredAnalyses := requirementAnalysisSet(requirement)
	delete(requiredAnalyses, simmodel.AnalysisACSweep)
	delete(requiredAnalyses, simmodel.AnalysisNoise)
	candidates := []scored{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "signal_diode" || !ratingsCoverRequirement(requirement, primitive) ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			(requireThermal && !primitiveHasThermalEvidence(primitive)) {
			continue
		}
		forwardV, ok := topologyDiodeForwardVoltage(primitive, currentA)
		if ok && forwardV <= maximumForwardV {
			candidates = append(candidates, scored{primitive: primitive, forwardV: forwardV})
		}
	}
	slices.SortFunc(candidates, func(left, right scored) int {
		return cmp.Or(cmp.Compare(left.forwardV, right.forwardV), comparePositiveArea(left.primitive.AreaMM2, right.primitive.AreaMM2), cmp.Compare(left.primitive.Key, right.primitive.Key))
	})
	if len(candidates) == 0 {
		return PrimitiveCandidate{}
	}
	return candidates[0].primitive
}

func topologyDiodeForwardVoltage(primitive PrimitiveCandidate, currentA float64) (float64, bool) {
	for _, model := range primitive.Models {
		if model.ModelID != simmodel.PrimitiveDiodeShockleyV1 {
			continue
		}
		is, emission, temperature := 0.0, 0.0, 0.0
		for _, parameter := range model.Parameters {
			switch parameter.Name {
			case "saturation_current_a":
				is = parameter.Value
			case "emission_coefficient":
				emission = parameter.Value
			case "junction_temperature_k":
				temperature = parameter.Value
			}
		}
		if is > 0 && emission > 0 && temperature > 0 && currentA > 0 {
			const boltzmannOverCharge = 8.617333262145e-5
			return emission * boltzmannOverCharge * temperature * math.Log1p(currentA/is), true
		}
	}
	return 0, false
}

func topologyAutonomousPeriodicRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	frequency, output, required := topologyAutonomousPeriodicBehavior(requirement)
	if !required {
		return nil, Consumption{}, map[string][]string{}
	}
	highRail, lowRail := topologyPowerRails(requirement, initial.graph)
	references := topologyNodesByRole(initial.graph, "reference")
	controls := topologyNodesByRole(initial.graph, "control", "input")
	if lowRail == "" && len(references) == 1 {
		lowRail = references[0]
	}
	if highRail == "" || lowRail == "" || len(references) != 1 || len(controls) != 1 {
		return nil, Consumption{}, map[string][]string{"relationship_gap": {"autonomous periodic behavior requires one supply, one reference, one enable/control endpoint, and one output"}}
	}
	comparator := topologyPeriodicComparatorPrimitive(requirement, inventory)
	chargeResistor := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", 100_000)
	thresholdResistor := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", 100_000)
	pullupResistor := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", 10_000)
	enableIsolation := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", 1_000_000)
	if comparator.Key == "" || chargeResistor.Key == "" || thresholdResistor.Key == "" || pullupResistor.Key == "" || enableIsolation.Key == "" {
		return nil, Consumption{}, map[string][]string{"relationship_gap": {"autonomous periodic behavior lacks a reviewed comparator and timing/bias resistor set"}}
	}
	chargeR := primitiveSeedValueOrZero(chargeResistor)
	if chargeR <= 0 || !finite(chargeR) {
		return nil, Consumption{}, map[string][]string{"relationship_gap": {"autonomous periodic behavior lacks a positive reviewed charge-resistor value"}}
	}
	capTarget := 1 / (2 * math.Log(2) * frequency * chargeR)
	timingCapacitor := topologyPrimitiveClosestValue(inventory.Primitives, "capacitor", capTarget)
	if timingCapacitor.Key == "" {
		return nil, Consumption{}, map[string][]string{"relationship_gap": {fmt.Sprintf("autonomous periodic behavior lacks a reviewed timing capacitor near %.12g F", capTarget)}}
	}
	consumption := Consumption{ExpandedStates: 1}
	state := initial
	var threshold, timing string
	state, threshold = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
	state, timing = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
	if threshold == "" || timing == "" {
		return nil, consumption, map[string][]string{"graph_limit": {"autonomous periodic behavior requires two internal timing nodes"}}
	}
	outputNode := "port_" + output
	state = addRelationshipPrimitive(state, requirement, inventoryByKey, comparator, []TerminalConnection{
		{Terminal: "IN_PLUS", Node: threshold}, {Terminal: "IN_MINUS", Node: timing},
		{Terminal: "OUT", Node: outputNode}, {Terminal: "V_PLUS", Node: highRail}, {Terminal: "V_MINUS", Node: lowRail},
	}, &consumption)
	if comparator.Kind == "comparator" {
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, pullupResistor, topologyTwoTerminalPlacement(highRail, outputNode), &consumption)
	}
	for _, edge := range [][2]string{{outputNode, threshold}, {highRail, threshold}, {threshold, references[0]}} {
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, thresholdResistor, topologyTwoTerminalPlacement(edge[0], edge[1]), &consumption)
	}
	state = addRelationshipPrimitive(state, requirement, inventoryByKey, chargeResistor, topologyTwoTerminalPlacement(outputNode, timing), &consumption)
	state = addRelationshipPrimitive(state, requirement, inventoryByKey, timingCapacitor, topologyTwoTerminalPlacement(timing, references[0]), &consumption)
	state = addRelationshipPrimitive(state, requirement, inventoryByKey, enableIsolation, topologyTwoTerminalPlacement(controls[0], threshold), &consumption)
	return finalizeNonlinearSwitchingRelationship(ctx, requirement, inventory, limits, policy, []topologySearchState{state}, consumption, "autonomous hysteretic timing")
}

func topologyAutonomousPeriodicBehavior(requirement Requirement) (float64, string, bool) {
	frequency, output := 0.0, ""
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "oscillation_frequency" || assertion.Analysis != "transient" ||
			assertion.Excitation != nil || assertion.Observation.Kind != "port" {
			continue
		}
		frequency = assertionTarget(assertion)
		output = assertion.Observation.ID
	}
	return frequency, output, frequency > 0 && output != ""
}

func topologyPeriodicComparatorPrimitive(requirement Requirement, inventory PrimitiveInventory) PrimitiveCandidate {
	requiredAnalyses := requirementAnalysisSet(requirement)
	pushPull, openCollector := []PrimitiveCandidate{}, []PrimitiveCandidate{}
	requiredSwing := requirementMinimumMetric(requirement, "output_swing")
	supply := nominalSupplyVoltage(requirement)
	for _, primitive := range inventory.Primitives {
		if !primitiveHasThermalEvidence(primitive) ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) || !ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		switch primitive.Kind {
		case "opamp":
			highMargin := primitiveModelParameter(primitive, simmodel.PrimitiveOpAmpV1, "output_high_margin_v")
			lowMargin := primitiveModelParameter(primitive, simmodel.PrimitiveOpAmpV1, "output_low_margin_v")
			if supply-highMargin-lowMargin >= requiredSwing {
				pushPull = append(pushPull, primitive)
			}
		case "comparator":
			openCollector = append(openCollector, primitive)
		}
	}
	slices.SortFunc(pushPull, func(left, right PrimitiveCandidate) int {
		return compareRepresentativePrimitives(left, right, requiredAnalyses)
	})
	slices.SortFunc(openCollector, func(left, right PrimitiveCandidate) int {
		return compareRepresentativePrimitives(left, right, requiredAnalyses)
	})
	if len(pushPull) != 0 {
		return pushPull[0]
	}
	if len(openCollector) != 0 {
		return openCollector[0]
	}
	return PrimitiveCandidate{}
}

func topologyStepDownRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	behavior, required := topologyStepDownBehavior(requirement)
	if !required {
		return nil, Consumption{}, map[string][]string{}
	}
	inputMin, inputMax := behavior.inputMinimumV, behavior.inputMaximumV
	outputVoltage, outputCurrent := behavior.outputVoltageV, behavior.outputCurrentA
	regulator := topologyStepDownPrimitive(requirement, inventory, inputMin, inputMax, outputVoltage, outputCurrent, behavior.minimumEfficiencyPct)
	if regulator.Key == "" {
		return nil, Consumption{}, map[string][]string{"dynamic_envelope_unsupported": {fmt.Sprintf("no reviewed step-down primitive covers %.12g..%.12g V input, %.12g V/%.12g A output, and %.12g%% efficiency", inputMin, inputMax, outputVoltage, outputCurrent, behavior.minimumEfficiencyPct)}}
	}
	highRail, lowRail := topologyPowerRails(requirement, initial.graph)
	references := topologyNodesByRole(initial.graph, "reference")
	if lowRail == "" && len(references) == 1 {
		lowRail = references[0]
	}
	if highRail == "" || lowRail == "" || len(references) != 1 {
		return nil, Consumption{}, map[string][]string{"relationship_gap": {"step-down energy transfer requires one ordered input supply and one reference"}}
	}
	frequency := primitiveModelParameter(regulator, simmodel.PrimitiveSynchronousBuckRegulatorV1, "switching_frequency_hz")
	referenceV := primitiveModelParameter(regulator, simmodel.PrimitiveSynchronousBuckRegulatorV1, "reference_voltage_v")
	if frequency <= 0 || referenceV <= 0 || outputVoltage <= referenceV {
		return nil, Consumption{}, map[string][]string{"relationship_gap": {"reviewed step-down primitive lacks switching-frequency or feedback-reference evidence"}}
	}
	rippleCurrent := math.Max(.2*outputCurrent, .01)
	duty := math.Min(.95, outputVoltage/inputMax)
	inductorTarget := (inputMax - outputVoltage) * duty / (rippleCurrent * frequency)
	rippleLimit := requirementMaximumMetric(requirement, "output_ripple")
	if rippleLimit <= 0 {
		rippleLimit = .02 * outputVoltage
	}
	capacitorTarget := rippleCurrent / (8 * frequency * rippleLimit)
	inductor := topologyPrimitiveClosestValue(inventory.Primitives, "inductor", inductorTarget)
	selectedInductance := primitiveSeedValueOrZero(inductor)
	selectedRippleCurrent := 0.0
	if selectedInductance > 0 {
		selectedRippleCurrent = (inputMax - outputVoltage) * duty / (selectedInductance * frequency)
	}
	outputCapacitor := topologyStepDownOutputCapacitor(
		inventory.Primitives, math.Max(capacitorTarget, 10e-6), selectedRippleCurrent, frequency, rippleLimit,
	)
	inputCapacitor := topologyPrimitiveClosestValue(inventory.Primitives, "capacitor", 10e-6)
	feedbackTop, feedbackBottom, feedbackFound := topologyStepDownFeedbackPrimitives(
		requirement, inventory, regulator, outputVoltage,
	)
	if !feedbackFound {
		return nil, Consumption{}, map[string][]string{"dynamic_envelope_unsupported": {"no catalog feedback pair keeps the reviewed controller reference inside the declared output-voltage envelope"}}
	}
	if inductor.Key == "" || outputCapacitor.Key == "" || inputCapacitor.Key == "" || feedbackBottom.Key == "" || len(feedbackTop) == 0 {
		return nil, Consumption{}, map[string][]string{"relationship_gap": {"step-down energy transfer lacks reviewed magnetics, capacitors, or feedback values derived from its behavior"}}
	}
	_ = representatives
	consumption := Consumption{ExpandedStates: 1}
	state := initial
	var switchNode, feedbackNode string
	state, switchNode = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
	state, feedbackNode = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
	if switchNode == "" || feedbackNode == "" {
		return nil, consumption, map[string][]string{"graph_limit": {"step-down energy transfer requires switch and feedback internal nodes"}}
	}
	outputNode, enableNode := "port_"+behavior.output, "port_"+behavior.enable
	state = addRelationshipPrimitive(state, requirement, inventoryByKey, regulator, []TerminalConnection{
		{Terminal: "PVIN", Node: highRail}, {Terminal: "SW", Node: switchNode}, {Terminal: "FB", Node: feedbackNode},
		{Terminal: "AGND", Node: lowRail}, {Terminal: "PGND", Node: lowRail}, {Terminal: "EN", Node: enableNode},
	}, &consumption)
	state = addRelationshipPrimitive(state, requirement, inventoryByKey, inductor, topologyTwoTerminalPlacement(switchNode, outputNode), &consumption)
	state = addRelationshipPrimitive(state, requirement, inventoryByKey, outputCapacitor, topologyTwoTerminalPlacement(outputNode, references[0]), &consumption)
	state = addRelationshipPrimitive(state, requirement, inventoryByKey, inputCapacitor, topologyTwoTerminalPlacement(highRail, references[0]), &consumption)
	if len(feedbackTop) == 1 {
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, feedbackTop[0], topologyTwoTerminalPlacement(outputNode, feedbackNode), &consumption)
	} else {
		var seriesNode string
		state, seriesNode = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
		if seriesNode == "" {
			return nil, consumption, map[string][]string{"graph_limit": {"compound feedback requires one bounded internal node"}}
		}
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, feedbackTop[0], topologyTwoTerminalPlacement(outputNode, seriesNode), &consumption)
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, feedbackTop[1], topologyTwoTerminalPlacement(seriesNode, feedbackNode), &consumption)
	}
	state = addRelationshipPrimitive(state, requirement, inventoryByKey, feedbackBottom, topologyTwoTerminalPlacement(feedbackNode, references[0]), &consumption)
	return finalizeNonlinearSwitchingRelationship(ctx, requirement, inventory, limits, policy, []topologySearchState{state}, consumption, "regulated step-down energy transfer")
}

func topologyStepDownOutputCapacitor(
	primitives []PrimitiveCandidate,
	targetCapacitance,
	peakToPeakInductorCurrent,
	frequency,
	rippleLimit float64,
) PrimitiveCandidate {
	eligible := []PrimitiveCandidate{}
	for _, primitive := range primitives {
		if primitive.Kind != "capacitor" {
			continue
		}
		capacitance := primitiveSeedValueOrZero(primitive)
		esr, _, _, esrFound := primitiveModelParameterRange(
			primitive, simmodel.PrimitiveCapacitorTransientV1, "series_resistance_ohm",
		)
		if capacitance <= 0 || !esrFound || esr < 0 || frequency <= 0 || rippleLimit <= 0 {
			continue
		}
		ripple := peakToPeakInductorCurrent * (1/(8*frequency*capacitance) + esr)
		if !finite(ripple) || ripple > rippleLimit {
			continue
		}
		eligible = append(eligible, primitive)
	}
	return topologyPrimitiveClosestValue(eligible, "capacitor", targetCapacitance)
}

func stepDownOutputCapacitorVariantAllowed(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
	variant PrimitiveCandidate,
	inventory map[string]PrimitiveCandidate,
) bool {
	if instance.Kind != "capacitor" || len(instance.Terminals) != 2 || variant.Kind != "capacitor" {
		return true
	}
	outputNodes := map[string]bool{}
	referenceNodes := map[string]bool{}
	for _, node := range graph.Nodes {
		outputNodes[node.ID] = node.Role == "output"
		referenceNodes[node.ID] = node.Role == "reference"
	}
	first, second := instance.Terminals[0].Node, instance.Terminals[1].Node
	if !((outputNodes[first] && referenceNodes[second]) || (outputNodes[second] && referenceNodes[first])) {
		return true
	}
	behavior, behaviorFound := topologyStepDownBehavior(requirement)
	rippleLimit := requirementMaximumMetric(requirement, "output_ripple")
	if !behaviorFound || rippleLimit <= 0 {
		return true
	}
	inputMax, outputVoltage := behavior.inputMaximumV, behavior.outputVoltageV
	frequency := 0.0
	switchNode := ""
	for _, candidate := range graph.Instances {
		primitive := inventory[candidate.PrimitiveKey]
		if primitive.Kind != "synchronous_buck_regulator" {
			continue
		}
		frequency = primitiveModelParameter(primitive, simmodel.PrimitiveSynchronousBuckRegulatorV1, "switching_frequency_hz")
		for _, terminal := range candidate.Terminals {
			if terminal.Terminal == "SW" {
				switchNode = terminal.Node
			}
		}
		break
	}
	if frequency <= 0 || switchNode == "" {
		return true
	}
	outputNode := first
	if !outputNodes[outputNode] {
		outputNode = second
	}
	inductance := 0.0
	for _, candidate := range graph.Instances {
		if candidate.Kind != "inductor" || len(candidate.Terminals) != 2 {
			continue
		}
		a, b := candidate.Terminals[0].Node, candidate.Terminals[1].Node
		if !((a == switchNode && b == outputNode) || (b == switchNode && a == outputNode)) {
			continue
		}
		inductance = primitiveSeedValueOrZero(inventory[candidate.PrimitiveKey])
		if candidate.ValueSI != nil {
			inductance = *candidate.ValueSI
		}
		break
	}
	capacitance := primitiveSeedValueOrZero(variant)
	esr, _, _, esrFound := primitiveModelParameterRange(
		variant, simmodel.PrimitiveCapacitorTransientV1, "series_resistance_ohm",
	)
	if inductance <= 0 || capacitance <= 0 || !esrFound || esr < 0 {
		return false
	}
	duty := math.Min(.95, outputVoltage/inputMax)
	peakToPeakInductorCurrent := (inputMax - outputVoltage) * duty / (inductance * frequency)
	ripple := peakToPeakInductorCurrent * (1/(8*frequency*capacitance) + esr)
	return finite(ripple) && ripple <= rippleLimit
}

type topologyStepDownEnvelope struct {
	inputMinimumV        float64
	inputMaximumV        float64
	outputVoltageV       float64
	outputCurrentA       float64
	minimumEfficiencyPct float64
	output               string
	enable               string
}

func topologyStepDownBehavior(requirement Requirement) (topologyStepDownEnvelope, bool) {
	envelope := topologyStepDownEnvelope{inputMinimumV: math.Inf(1)}
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind != "supply" || domain.MinVoltageV == nil || domain.MaxVoltageV == nil {
			continue
		}
		envelope.inputMinimumV = math.Min(envelope.inputMinimumV, *domain.MinVoltageV)
		envelope.inputMaximumV = math.Max(envelope.inputMaximumV, *domain.MaxVoltageV)
	}
	for _, port := range requirement.Requirements.Ports {
		if port.Direction == "source" && port.Kind == "power" && port.Electrical.NominalVoltageV != nil {
			envelope.output, envelope.outputVoltageV = port.ID, *port.Electrical.NominalVoltageV
			if port.Electrical.MaxCurrentA != nil {
				envelope.outputCurrentA = *port.Electrical.MaxCurrentA
			}
		}
		if port.Kind == "digital" && port.Direction == "sink" {
			envelope.enable = port.ID
		}
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "conversion_efficiency" && assertion.Min != nil {
			envelope.minimumEfficiencyPct = math.Max(envelope.minimumEfficiencyPct, *assertion.Min)
		}
	}
	valid := finite(envelope.inputMinimumV) && envelope.inputMaximumV > envelope.inputMinimumV &&
		envelope.outputVoltageV > 0 && envelope.outputVoltageV < envelope.inputMinimumV &&
		envelope.outputCurrentA > 0 && envelope.minimumEfficiencyPct > 0 &&
		envelope.output != "" && envelope.enable != ""
	return envelope, valid
}

func topologyStepDownPrimitive(requirement Requirement, inventory PrimitiveInventory, inputMin, inputMax, outputV, outputA, efficiencyPct float64) PrimitiveCandidate {
	type scored struct {
		primitive  PrimitiveCandidate
		voltage    float64
		efficiency float64
	}
	requiredAnalyses := requirementAnalysisSet(requirement)
	candidates := []scored{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "synchronous_buck_regulator" || !primitiveHasThermalEvidence(primitive) ||
			!primitiveHasSOAEvidence(primitive) || !primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			!ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		minimum := primitiveModelParameter(primitive, simmodel.PrimitiveSynchronousBuckRegulatorV1, "min_input_voltage_v")
		maximum := primitiveModelParameter(primitive, simmodel.PrimitiveSynchronousBuckRegulatorV1, "max_input_voltage_v")
		current := primitiveModelParameter(primitive, simmodel.PrimitiveSynchronousBuckRegulatorV1, "max_output_current_a")
		efficiency := 100 * primitiveModelParameter(primitive, simmodel.PrimitiveSynchronousBuckRegulatorV1, "conversion_efficiency_fraction")
		nominal := primitiveModelParameter(primitive, simmodel.PrimitiveSynchronousBuckRegulatorV1, "nominal_output_voltage_v")
		_, _, feedbackFound := topologyStepDownFeedbackPrimitives(requirement, inventory, primitive, outputV)
		if minimum <= inputMin && maximum >= inputMax && current >= outputA && efficiency >= efficiencyPct && feedbackFound {
			candidates = append(candidates, scored{primitive: primitive, voltage: math.Abs(nominal - outputV), efficiency: efficiency})
		}
	}
	slices.SortFunc(candidates, func(left, right scored) int {
		return cmp.Or(cmp.Compare(left.voltage, right.voltage), cmp.Compare(-left.efficiency, -right.efficiency), cmp.Compare(left.primitive.Key, right.primitive.Key))
	})
	if len(candidates) == 0 {
		return PrimitiveCandidate{}
	}
	return candidates[0].primitive
}

func topologyStepDownFeedbackPrimitives(
	requirement Requirement,
	inventory PrimitiveInventory,
	regulator PrimitiveCandidate,
	outputTarget float64,
) ([]PrimitiveCandidate, PrimitiveCandidate, bool) {
	referenceNominal, referenceMinimum, referenceMaximum, found := primitiveModelParameterRange(
		regulator, simmodel.PrimitiveSynchronousBuckRegulatorV1, "reference_voltage_v",
	)
	if !found {
		return nil, PrimitiveCandidate{}, false
	}
	outputMinimum, outputMaximum := 0.0, 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "output_voltage" || assertion.Observation.Kind != "port" {
			continue
		}
		if assertion.Min != nil {
			outputMinimum = *assertion.Min
		}
		if assertion.Max != nil {
			outputMaximum = *assertion.Max
		}
		break
	}
	if outputMinimum <= 0 || outputMaximum < outputMinimum {
		outputMinimum, outputMaximum = outputTarget, outputTarget
	}
	upperValue, lowerValue, found := catalogControllerFeedbackDivider(
		requirement,
		primitiveInventoryByKey(inventory),
		referenceNominal,
		referenceMinimum,
		referenceMaximum,
		outputMinimum,
		outputMaximum,
		10_000,
	)
	if found {
		upper := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", upperValue)
		lower := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", lowerValue)
		return []PrimitiveCandidate{upper}, lower, upper.Key != "" && lower.Key != ""
	}
	firstValue, secondValue, seriesLowerValue, seriesFound := catalogControllerSeriesFeedbackDivider(
		requirement,
		primitiveInventoryByKey(inventory),
		referenceNominal,
		referenceMinimum,
		referenceMaximum,
		outputMinimum,
		outputMaximum,
		10_000,
	)
	if !seriesFound {
		return nil, PrimitiveCandidate{}, false
	}
	first := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", firstValue)
	second := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", secondValue)
	lower := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", seriesLowerValue)
	return []PrimitiveCandidate{first, second}, lower, first.Key != "" && second.Key != "" && lower.Key != ""
}

func primitiveModelParameter(primitive PrimitiveCandidate, modelID, name string) float64 {
	for _, model := range primitive.Models {
		if model.ModelID != modelID {
			continue
		}
		for _, parameter := range model.Parameters {
			if parameter.Name == name {
				return parameter.Value
			}
		}
	}
	return 0
}

func primitiveModelParameterRange(
	primitive PrimitiveCandidate,
	modelID, name string,
) (float64, float64, float64, bool) {
	for _, model := range primitive.Models {
		if model.ModelID != modelID {
			continue
		}
		nominal := 0.0
		found := false
		for _, parameter := range model.Parameters {
			if parameter.Name == name {
				nominal, found = parameter.Value, true
				break
			}
		}
		if !found {
			return 0, 0, 0, false
		}
		minimum, maximum := nominal, nominal
		target := "model_parameters." + name
		for _, uncertainty := range model.Uncertainties {
			if uncertainty.Target == target {
				minimum, maximum = uncertainty.Minimum, uncertainty.Maximum
				break
			}
		}
		if nominal <= 0 || minimum <= 0 || maximum < minimum ||
			nominal < minimum || nominal > maximum {
			return 0, 0, 0, false
		}
		return nominal, minimum, maximum, true
	}
	return 0, 0, 0, false
}

func primitiveSeedValueOrZero(primitive PrimitiveCandidate) float64 {
	value := seedPrimitiveValue(primitive)
	if value == nil {
		return 0
	}
	return *value
}

func requirementMaximumMetric(requirement Requirement, metric string) float64 {
	value := math.Inf(1)
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == metric && assertion.Max != nil {
			value = math.Min(value, *assertion.Max)
		}
	}
	if !finite(value) {
		return 0
	}
	return value
}

func requirementMinimumMetric(requirement Requirement, metric string) float64 {
	minimum := 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == metric && assertion.Min != nil {
			minimum = math.Max(minimum, *assertion.Min)
		}
	}
	return minimum
}

func requirementMetricPresent(requirement Requirement, metric string) bool {
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == metric {
			return true
		}
	}
	return false
}

func topologyNodeNominalVoltageOrZero(requirement Requirement, graph CandidateGraph, node string) float64 {
	value, _ := topologyNodeNominalVoltage(requirement, graph, node)
	return value
}

func finalizeNonlinearSwitchingRelationship(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	limits GraphLimits,
	policy Policy,
	states []topologySearchState,
	consumption Consumption,
	label string,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	rejections := map[string][]string{}
	result := []TopologyCandidate{}
	for _, state := range states {
		if ctx.Err() != nil || consumption.GeneratedGraphs > policy.MaxGeneratedGraphs {
			consumption.BudgetExhausted = true
			break
		}
		if len(state.graph.Instances) > limits.MaxPrimitiveInstances || internalNodeCount(state.graph) > limits.MaxInternalNodes {
			rejections["graph_limit"] = append(rejections["graph_limit"], fmt.Sprintf("%s uses %d primitives and %d internal nodes", label, len(state.graph.Instances), internalNodeCount(state.graph)))
			continue
		}
		if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
			for _, issue := range issues {
				rejections[string(issue.Code)] = append(rejections[string(issue.Code)], issue.Path+":"+issue.Message)
			}
			continue
		}
		if state.score.BehaviorGap != 0 {
			rejections["relationship_gap"] = append(rejections["relationship_gap"], fmt.Sprintf("%s:%s:gap=%d", label, state.hash, state.score.BehaviorGap))
			continue
		}
		normalized, err := NormalizeGraph(state.graph)
		if err != nil {
			rejections["canonical_normalization_failed"] = append(rejections["canonical_normalization_failed"], err.Error())
			continue
		}
		topologyHash, err := TopologyHash(normalized)
		if err != nil {
			rejections["canonical_topology_hash_failed"] = append(rejections["canonical_topology_hash_failed"], err.Error())
			continue
		}
		consumption.CompleteGraphs++
		result = append(result, TopologyCandidate{Fingerprint: state.hash, TopologyHash: topologyHash, Score: state.score, Graph: normalized, Operations: cloneGraphOperations(state.operations)})
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}
