package opentopologysynthesis

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"sort"

	"kicadai/internal/simmodel"
)

type regulatedCurrentRelationship struct {
	input      string
	output     string
	direction  string
	activation string
	fault      string
}

type currentSenseDifferentialValues struct {
	shunt               PrimitiveCandidate
	shuntCount          int
	shuntResistance     float64
	input               PrimitiveCandidate
	inputCount          int
	inputResistance     float64
	feedback            PrimitiveCandidate
	feedbackCount       int
	feedbackResistance  float64
	effectiveResistance float64
}

type currentSenseDifferentialPair struct {
	input              PrimitiveCandidate
	inputCount         int
	inputResistance    float64
	feedback           PrimitiveCandidate
	feedbackCount      int
	feedbackResistance float64
	ratio              float64
	cornerMinimumRatio float64
	cornerMaximumRatio float64
	observationPenalty float64
}

type currentSenseResistanceNetwork struct {
	segments            [][]PrimitiveCandidate
	effectiveResistance float64
}

type currentLimitedSwitchRelationship struct {
	control        string
	output         string
	minimumCurrent float64
	maximumCurrent float64
	targetCurrent  float64
	onVoltageLimit float64
}

// topologyCurrentLimitedSwitchRelationshipSeeds derives a bang-bang current
// regulator when a bounded periodic command drives an energy-storing load and
// behavior requires both low switch loss and bounded peak current. Two
// open-collector decisions share the gate node: one enforces command state and
// the other removes drive when a catalog-backed source shunt reaches the
// derived current threshold. The relationship is selected only from behavior,
// port roles, reviewed primitive evidence, and catalog values.
func topologyCurrentLimitedSwitchRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	relationships := currentLimitedSwitchRelationships(requirement)
	if len(relationships) == 0 {
		return nil, Consumption{}, map[string][]string{}
	}
	var comparator, resistor, nmos PrimitiveCandidate
	for _, primitive := range representatives {
		switch primitive.Kind {
		case "comparator":
			comparator = primitive
		case "resistor":
			resistor = primitive
		case "n_channel_mosfet":
			nmos = primitive
		}
	}
	if rated := topologyRatedPowerPrimitive(requirement, inventory, "n_channel_mosfet"); rated.Key != "" {
		nmos = rated
	}
	flyback := topologyFlybackDiodePrimitive(requirement, inventory)
	if comparator.Key == "" || resistor.Key == "" || nmos.Key == "" || flyback.Key == "" {
		return nil, Consumption{}, map[string][]string{
			"current_limited_switch_primitives_unavailable": {"reviewed comparator, low-side switch, resistor, and flyback relationships are required"},
		}
	}
	supplies := topologyNodesByRole(initial.graph, "supply")
	references := topologyNodesByRole(initial.graph, "reference")
	if len(supplies) == 0 || len(references) == 0 {
		return nil, Consumption{}, map[string][]string{}
	}
	consumption := Consumption{}
	rejections := map[string][]string{}
	retained := map[string]TopologyCandidate{}
	for _, relationship := range relationships {
		if ctx.Err() != nil || consumption.ExpandedStates >= policy.MaxExpandedStates ||
			consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
			consumption.BudgetExhausted = true
			break
		}
		control := externalRelationshipNode(initial.graph, relationship.control)
		output := externalRelationshipNode(initial.graph, relationship.output)
		if control == "" || output == "" || topologyControlledSwitchHighSideOutput(requirement, initial.graph, output) {
			continue
		}
		loadSupply, found := topologyControlledSwitchLoadSupply(requirement, initial.graph, output, supplies)
		if !found {
			rejections["relationship_gap"] = append(rejections["relationship_gap"],
				relationship.output+": current-limited inductive switching requires one unambiguous load supply")
			continue
		}
		controlSupply := currentLimitedSwitchControlSupply(requirement, initial.graph, supplies)
		controlSupplyVoltage, controlSupplyOK := topologyNodeNominalVoltage(requirement, initial.graph, controlSupply)
		if !controlSupplyOK || controlSupplyVoltage <= 0 {
			continue
		}
		senseVoltage := 0.1
		if relationship.onVoltageLimit > 0 {
			senseVoltage = math.Min(senseVoltage, relationship.onVoltageLimit*.25)
		}
		senseTarget := senseVoltage / relationship.targetCurrent
		senseMaximum := math.Inf(1)
		if relationship.onVoltageLimit > 0 {
			senseMaximum = relationship.onVoltageLimit * .7 / relationship.maximumCurrent
		}
		senseNetwork, senseOK := currentSenseSeriesParallelCompositionWithin(
			ctx, requirement, inventory, senseTarget, 2, 0, senseMaximum,
		)
		if !senseOK || senseNetwork.effectiveResistance <= 0 {
			rejections["current_limited_switch_primitives_unavailable"] = append(
				rejections["current_limited_switch_primitives_unavailable"],
				relationship.output+": no reviewed low-ohmic shunt composition satisfies the on-state loss bound",
			)
			continue
		}
		currentReference := relationship.targetCurrent * senseNetwork.effectiveResistance
		currentUpper, currentLower, _, currentDividerOK := currentLimitedSwitchDivider(
			requirement, inventory, controlSupplyVoltage, currentReference,
		)
		commandHigh, _, commandHighOK := requirementControlHighVoltageRange(requirement, relationship.control)
		if !commandHighOK {
			continue
		}
		commandTarget := commandHigh / 2
		commandUpper, commandLower, _, commandDividerOK := currentLimitedSwitchDivider(
			requirement, inventory, controlSupplyVoltage, commandTarget,
		)
		if !currentDividerOK || !commandDividerOK {
			rejections["current_limited_switch_primitives_unavailable"] = append(
				rejections["current_limited_switch_primitives_unavailable"],
				relationship.output+": reviewed divider values cannot realize command and current thresholds",
			)
			continue
		}
		currentUpperResistance := *currentUpper.ValueDomain.Nominal
		currentLowerResistance := *currentLower.ValueDomain.Nominal
		gateLimit := primitiveModelParameter(
			nmos,
			simmodel.PrimitiveNMOSSwitchV1,
			"max_gate_source_voltage_v",
		)
		conservativeUpperSense := .9 * relationship.maximumCurrent * senseNetwork.effectiveResistance
		dividerConductance := 1/currentUpperResistance + 1/currentLowerResistance
		feedbackTarget := 0.0
		if gateLimit > conservativeUpperSense && conservativeUpperSense > currentReference &&
			dividerConductance > 0 {
			feedbackTarget = (gateLimit - conservativeUpperSense) /
				(dividerConductance * (conservativeUpperSense - currentReference))
		}
		feedbackNetwork, feedbackOK := currentSenseSeriesParallelCompositionWithin(
			ctx,
			requirement,
			inventory,
			feedbackTarget,
			5,
			0,
			math.Inf(1),
		)
		if !feedbackOK {
			rejections["current_limited_switch_primitives_unavailable"] = append(
				rejections["current_limited_switch_primitives_unavailable"],
				fmt.Sprintf("%s: reviewed resistor values cannot realize %.6g ohm tolerance-bounded current-control hysteresis",
					relationship.output, feedbackTarget),
			)
			continue
		}
		consumption.ExpandedStates++
		state := initial
		senseIntermediateCount := len(senseNetwork.segments) - 1
		feedbackIntermediateCount := len(feedbackNetwork.segments) - 1
		internalCount := 4 + senseIntermediateCount + feedbackIntermediateCount
		internal := make([]string, 0, internalCount)
		for len(internal) < internalCount {
			var node string
			state, node = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
			if node == "" {
				break
			}
			internal = append(internal, node)
		}
		if len(internal) != internalCount || internalNodeCount(state.graph) > limits.MaxInternalNodes {
			continue
		}
		gate, commandThreshold, currentThreshold, sense := internal[0], internal[1], internal[2], internal[3]
		reference := references[0]
		for _, placement := range [][]TerminalConnection{
			{
				{Terminal: "IN_MINUS", Node: commandThreshold}, {Terminal: "IN_PLUS", Node: control},
				{Terminal: "OUT", Node: gate}, {Terminal: "V_MINUS", Node: reference},
				{Terminal: "V_PLUS", Node: loadSupply},
			},
			{
				{Terminal: "IN_MINUS", Node: sense}, {Terminal: "IN_PLUS", Node: currentThreshold},
				{Terminal: "OUT", Node: gate}, {Terminal: "V_MINUS", Node: reference},
				{Terminal: "V_PLUS", Node: loadSupply},
			},
		} {
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, comparator, placement, &consumption)
		}
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, nmos, []TerminalConnection{
			{Terminal: "DRAIN", Node: output}, {Terminal: "GATE", Node: gate}, {Terminal: "SOURCE", Node: sense},
		}, &consumption)
		for _, edge := range []struct {
			primitive PrimitiveCandidate
			first     string
			second    string
		}{
			{commandUpper, controlSupply, commandThreshold},
			{commandLower, commandThreshold, reference},
			{currentUpper, controlSupply, currentThreshold},
			{currentLower, currentThreshold, reference},
			{resistor, loadSupply, gate},
			{resistor, gate, reference},
		} {
			state = addRelationshipPrimitiveAtValue(
				state, requirement, inventoryByKey, edge.primitive, *edge.primitive.ValueDomain.Nominal,
				topologyTwoTerminalPlacement(edge.first, edge.second), &consumption,
			)
		}
		senseIntermediateEnd := 4 + senseIntermediateCount
		senseNodes := append([]string{sense}, internal[4:senseIntermediateEnd]...)
		senseNodes = append(senseNodes, reference)
		for index, segment := range senseNetwork.segments {
			for _, part := range segment {
				state = addRelationshipPrimitiveAtValue(
					state, requirement, inventoryByKey, part, *part.ValueDomain.Nominal,
					topologyTwoTerminalPlacement(senseNodes[index], senseNodes[index+1]), &consumption,
				)
			}
		}
		feedbackNodes := append([]string{gate}, internal[senseIntermediateEnd:]...)
		feedbackNodes = append(feedbackNodes, currentThreshold)
		for index, segment := range feedbackNetwork.segments {
			for _, part := range segment {
				state = addRelationshipPrimitiveAtValue(
					state, requirement, inventoryByKey, part, *part.ValueDomain.Nominal,
					topologyTwoTerminalPlacement(feedbackNodes[index], feedbackNodes[index+1]), &consumption,
				)
			}
		}
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, flyback, []TerminalConnection{
			{Terminal: "ANODE", Node: output}, {Terminal: "CATHODE", Node: loadSupply},
		}, &consumption)
		retainControlledSwitchRelationshipCandidate(
			state, inventory, limits, policy, &consumption, retained, rejections,
		)
	}
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

func currentLimitedSwitchRelationships(requirement Requirement) []currentLimitedSwitchRelationship {
	ports := map[string]Port{}
	for _, port := range requirement.Requirements.Ports {
		ports[port.ID] = port
	}
	relationships := map[string]currentLimitedSwitchRelationship{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "peak_current" || assertion.Observation.Kind != "port" ||
			assertion.Min == nil || assertion.Max == nil || *assertion.Min <= 0 || *assertion.Max <= *assertion.Min {
			continue
		}
		output, found := ports[assertion.Observation.ID]
		if !found || output.Kind != "controlled_current" {
			continue
		}
		control := ""
		for _, candidate := range requirement.Requirements.BehavioralRequirements {
			if candidate.Observation.Kind != "port" || candidate.Observation.ID != assertion.Observation.ID ||
				candidate.Excitation == nil || candidate.Excitation.Kind != "port" ||
				!requirementPortIsControl(requirement, candidate.Excitation.ID) ||
				!slices.Contains([]string{"duty_cycle", "off_state_current"}, candidate.Metric) {
				continue
			}
			if control == "" || candidate.Excitation.ID < control {
				control = candidate.Excitation.ID
			}
		}
		if control == "" || !currentLimitedSwitchHasInductiveLoad(requirement, assertion.Observation.ID) {
			continue
		}
		relationships[assertion.Observation.ID] = currentLimitedSwitchRelationship{
			control: control, output: assertion.Observation.ID,
			minimumCurrent: *assertion.Min, maximumCurrent: *assertion.Max,
			targetCurrent: *assertion.Min + .65*(*assertion.Max-*assertion.Min),
		}
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "on_state_voltage" || assertion.Max == nil ||
			assertion.Observation.Kind != "port" {
			continue
		}
		relationship := relationships[assertion.Observation.ID]
		relationship.onVoltageLimit = *assertion.Max
		relationships[assertion.Observation.ID] = relationship
	}
	result := make([]currentLimitedSwitchRelationship, 0, len(relationships))
	for _, relationship := range relationships {
		if relationship.control != "" {
			result = append(result, relationship)
		}
	}
	slices.SortFunc(result, func(left, right currentLimitedSwitchRelationship) int {
		return cmp.Or(cmp.Compare(left.output, right.output), cmp.Compare(left.control, right.control))
	})
	return result
}

func currentLimitedSwitchHasInductiveLoad(requirement Requirement, output string) bool {
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Target == output && condition.Axis == "load_inductance" && condition.Max > 0 {
				return true
			}
		}
	}
	return false
}

func currentLimitedSwitchControlSupply(requirement Requirement, graph CandidateGraph, supplies []string) string {
	best, bestVoltage := "", math.Inf(1)
	for _, supply := range supplies {
		voltage, found := topologyNodeNominalVoltage(requirement, graph, supply)
		if found && voltage > 0 && (voltage < bestVoltage || voltage == bestVoltage && supply < best) {
			best, bestVoltage = supply, voltage
		}
	}
	return best
}

func currentLimitedSwitchDivider(
	requirement Requirement,
	inventory PrimitiveInventory,
	supplyVoltage float64,
	targetVoltage float64,
) (PrimitiveCandidate, PrimitiveCandidate, float64, bool) {
	if supplyVoltage <= 0 || targetVoltage <= 0 || targetVoltage >= supplyVoltage {
		return PrimitiveCandidate{}, PrimitiveCandidate{}, 0, false
	}
	requiredAnalyses := requirementAnalysisSet(requirement)
	choices := []PrimitiveCandidate{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "resistor" || primitive.ValueDomain == nil || primitive.ValueDomain.Nominal == nil ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) || !ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		value := *primitive.ValueDomain.Nominal
		if value >= 100 && value <= 1_000_000 && finite(value) {
			choices = append(choices, primitive)
		}
	}
	slices.SortFunc(choices, func(left, right PrimitiveCandidate) int { return cmp.Compare(left.Key, right.Key) })
	bestUpper, bestLower := PrimitiveCandidate{}, PrimitiveCandidate{}
	bestVoltage, bestError, bestAnchorError, bestKey := 0.0, math.Inf(1), math.Inf(1), ""
	for _, upper := range choices {
		for _, lower := range choices {
			upperValue, lowerValue := *upper.ValueDomain.Nominal, *lower.ValueDomain.Nominal
			voltage := supplyVoltage * lowerValue / (upperValue + lowerValue)
			error := math.Abs(voltage-targetVoltage) / targetVoltage
			anchorError := math.Abs(math.Log((upperValue + lowerValue) / 20_000))
			key := upper.Key + "|" + lower.Key
			if error < bestError || error == bestError && (anchorError < bestAnchorError ||
				anchorError == bestAnchorError && (bestKey == "" || key < bestKey)) {
				bestUpper, bestLower, bestVoltage = upper, lower, voltage
				bestError, bestAnchorError, bestKey = error, anchorError, key
			}
		}
	}
	return bestUpper, bestLower, bestVoltage, bestKey != ""
}

func topologyTransconductanceRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	highCandidates, highConsumption, highRejections := topologyHighSideTransconductanceRelationshipSeeds(
		ctx, requirement, inventory, representatives, inventoryByKey, limits, policy, initial,
	)
	lowCandidates, lowConsumption, lowRejections := topologyLowSideTransconductanceRelationshipSeeds(
		ctx, requirement, inventory, representatives, inventoryByKey, limits, policy, initial,
	)
	addSearchConsumption(&highConsumption, lowConsumption)
	for code, samples := range lowRejections {
		highRejections[code] = append(highRejections[code], samples...)
	}
	retained := map[string]TopologyCandidate{}
	for _, candidate := range append(highCandidates, lowCandidates...) {
		if existing, found := retained[candidate.TopologyHash]; !found ||
			compareTopologyCandidates(candidate, existing) < 0 {
			retained[candidate.TopologyHash] = candidate
		}
	}
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, highConsumption, highRejections
}

// topologyUnprotectedTransconductanceRelationshipSeeds preserves the compact
// generic current-output architecture used when the behavioral interface does
// not declare independent startup and fault controls. Protected controlled-
// current ports take the explicit source/sink paths instead.
func topologyUnprotectedTransconductanceRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	requireTransconductance := false
	requireThermal := false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		requireTransconductance = requireTransconductance || assertion.Metric == "transconductance"
		requireThermal = requireThermal || assertion.Metric == "junction_temperature"
	}
	if !requireTransconductance {
		return nil, Consumption{}, map[string][]string{}
	}
	var opamp, resistor PrimitiveCandidate
	for _, primitive := range representatives {
		switch primitive.Kind {
		case "opamp":
			opamp = primitive
		case "resistor":
			resistor = primitive
		}
	}
	requiredAnalyses := requirementAnalysisSet(requirement)
	passCandidates := []PrimitiveCandidate{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "pnp_bjt" ||
			(requireThermal && !primitiveHasThermalEvidence(primitive)) ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			!ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		passCandidates = append(passCandidates, primitive)
	}
	slices.SortFunc(passCandidates, func(left, right PrimitiveCandidate) int {
		return compareRepresentativePrimitives(left, right, requiredAnalyses)
	})
	if opamp.Key == "" || resistor.Key == "" || len(passCandidates) == 0 {
		return nil, Consumption{}, map[string][]string{}
	}
	passDevice := passCandidates[0]
	inputs := topologyNodesByRole(initial.graph, "input", "control")
	outputs := topologyNodesByRole(initial.graph, "output")
	supplies := topologyNodesByRole(initial.graph, "supply")
	references := topologyNodesByRole(initial.graph, "reference")
	consumption := Consumption{}
	rejections := map[string][]string{}
	retained := map[string]TopologyCandidate{}
	for _, input := range inputs {
		for _, output := range outputs {
			for _, supply := range supplies {
				for _, reference := range references {
					if ctx.Err() != nil || consumption.ExpandedStates >= policy.MaxExpandedStates ||
						consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
						break
					}
					consumption.ExpandedStates++
					state := initial
					passDeviceCount := 1
					if requireThermal {
						passDeviceCount = 2
					}
					internalCount := 6
					if passDeviceCount > 1 {
						internalCount += passDeviceCount
					}
					internal := make([]string, 0, internalCount)
					for index := 0; index < internalCount; index++ {
						var node string
						state, node = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
						if node == "" {
							break
						}
						internal = append(internal, node)
					}
					if len(internal) != internalCount || internalNodeCount(state.graph) > limits.MaxInternalNodes {
						continue
					}
					passCollector := internal[0]
					senseMinus := internal[1]
					sensePlus := internal[2]
					senseOutput := internal[3]
					controlOutput := internal[4]
					passBase := internal[5]
					emitterNodes := []string{supply}
					if passDeviceCount > 1 {
						emitterNodes = internal[6:]
					}
					for _, emitter := range emitterNodes {
						state = addRelationshipPrimitive(state, requirement, inventoryByKey, passDevice, []TerminalConnection{
							{Terminal: "BASE", Node: passBase},
							{Terminal: "COLLECTOR", Node: passCollector},
							{Terminal: "EMITTER", Node: emitter},
						}, &consumption)
						if emitter != supply {
							state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor,
								topologyTwoTerminalPlacement(supply, emitter), &consumption)
						}
					}
					for _, edge := range [][2]string{
						{passCollector, output}, {output, senseMinus}, {senseOutput, senseMinus},
						{passCollector, sensePlus}, {sensePlus, reference}, {controlOutput, passBase},
						{passBase, supply},
					} {
						state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor,
							topologyTwoTerminalPlacement(edge[0], edge[1]), &consumption)
					}
					state = addRelationshipPrimitive(state, requirement, inventoryByKey, opamp, []TerminalConnection{
						{Terminal: "IN_MINUS", Node: senseMinus}, {Terminal: "IN_PLUS", Node: sensePlus},
						{Terminal: "OUT", Node: senseOutput}, {Terminal: "V_MINUS", Node: reference},
						{Terminal: "V_PLUS", Node: supply},
					}, &consumption)
					state = addRelationshipPrimitive(state, requirement, inventoryByKey, opamp, []TerminalConnection{
						{Terminal: "IN_MINUS", Node: input}, {Terminal: "IN_PLUS", Node: senseOutput},
						{Terminal: "OUT", Node: controlOutput}, {Terminal: "V_MINUS", Node: reference},
						{Terminal: "V_PLUS", Node: supply},
					}, &consumption)
					if len(state.graph.Instances) > limits.MaxPrimitiveInstances ||
						consumption.GeneratedGraphs > policy.MaxGeneratedGraphs {
						continue
					}
					if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
						for _, issue := range issues {
							rejections[string(issue.Code)] = append(rejections[string(issue.Code)], issue.Path+":"+issue.Message)
						}
						continue
					}
					if state.score.BehaviorGap != 0 {
						rejections["relationship_gap"] = append(rejections["relationship_gap"], fmt.Sprintf("%s:gap=%d", state.hash, state.score.BehaviorGap))
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
					candidate := TopologyCandidate{Fingerprint: state.hash, TopologyHash: topologyHash, Score: state.score, Graph: normalized, Operations: cloneGraphOperations(state.operations)}
					if existing, found := retained[topologyHash]; !found || compareTopologyCandidates(candidate, existing) < 0 {
						retained[topologyHash] = candidate
					}
				}
			}
		}
	}
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

func regulatedCurrentRelationships(requirement Requirement) []regulatedCurrentRelationship {
	ports := map[string]Port{}
	for _, port := range requirement.Requirements.Ports {
		ports[port.ID] = port
	}
	cases := map[string]OperatingCase{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		cases[operatingCase.ID] = operatingCase
	}
	startupCases := map[string]bool{}
	activationByOutput := map[string]string{}
	faultByOutput := map[string]string{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "startup_current" {
			for _, operatingCase := range assertion.OperatingCases {
				startupCases[operatingCase] = true
			}
		}
		if assertion.Metric == "off_state_current" && assertion.Excitation != nil &&
			assertion.Excitation.Kind == "port" && assertion.Observation.Kind == "port" &&
			requirementPortIsControl(requirement, assertion.Excitation.ID) {
			if regulatedCurrentControlActsAsEnable(
				requirement, assertion.Observation.ID, assertion.Excitation.ID, cases,
			) {
				activationByOutput[assertion.Observation.ID] = assertion.Excitation.ID
			} else {
				faultByOutput[assertion.Observation.ID] = assertion.Excitation.ID
			}
		}
	}
	result := []regulatedCurrentRelationship{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "transconductance" || assertion.Excitation == nil ||
			assertion.Excitation.Kind != "port" || assertion.Observation.Kind != "port" {
			continue
		}
		output, found := ports[assertion.Observation.ID]
		if !found || output.Kind != "controlled_current" ||
			(output.Direction != "source" && output.Direction != "sink") {
			continue
		}
		relationship := regulatedCurrentRelationship{
			input:      assertion.Excitation.ID,
			output:     assertion.Observation.ID,
			direction:  output.Direction,
			activation: activationByOutput[assertion.Observation.ID],
			fault:      faultByOutput[assertion.Observation.ID],
		}
		activationCandidates := []string{}
		for _, port := range requirement.Requirements.Ports {
			if relationship.activation != "" {
				break
			}
			if !requirementPortIsControl(requirement, port.ID) || port.ID == relationship.fault ||
				port.Electrical.DefaultState != "low" {
				continue
			}
			highInRegulation := false
			for _, caseID := range assertion.OperatingCases {
				for _, condition := range cases[caseID].Conditions {
					highInRegulation = highInRegulation ||
						(condition.Target == port.ID && condition.Axis == "input_voltage" && condition.Min > 0)
				}
			}
			lowAtStartup := false
			for caseID := range startupCases {
				for _, condition := range cases[caseID].Conditions {
					lowAtStartup = lowAtStartup ||
						(condition.Target == port.ID && condition.Axis == "input_voltage" && condition.Max == 0)
				}
			}
			if highInRegulation && lowAtStartup {
				activationCandidates = append(activationCandidates, port.ID)
			}
		}
		slices.Sort(activationCandidates)
		if len(activationCandidates) == 1 {
			relationship.activation = activationCandidates[0]
		}
		result = append(result, relationship)
	}
	slices.SortFunc(result, func(left, right regulatedCurrentRelationship) int {
		return cmp.Or(
			cmp.Compare(left.input, right.input),
			cmp.Compare(left.output, right.output),
			cmp.Compare(left.direction, right.direction),
			cmp.Compare(left.activation, right.activation),
			cmp.Compare(left.fault, right.fault),
		)
	})
	return result
}

func transconductanceInputConditioningRequired(
	requirement Requirement,
	relationship regulatedCurrentRelationship,
) bool {
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Observation.Kind != "port" || assertion.Observation.ID != relationship.output {
			continue
		}
		switch assertion.Metric {
		case "startup_current", "output_ripple":
			return true
		case "settling_time":
			if assertion.Excitation != nil && assertion.Excitation.Kind == "port" &&
				assertion.Excitation.ID == relationship.input {
				return true
			}
		}
	}
	return false
}

// regulatedCurrentControlActsAsEnable distinguishes a normal active-high
// permission from an independently asserted shutdown/fault input using only
// the transfer behavior. A control is an enable when the regulated transfer
// operates with that control high, either as a steady condition or as a
// low-to-high event in one of the transfer cases.
func regulatedCurrentControlActsAsEnable(
	requirement Requirement,
	outputID string,
	controlID string,
	cases map[string]OperatingCase,
) bool {
	defaultLow := false
	for _, port := range requirement.Requirements.Ports {
		if port.ID == controlID {
			defaultLow = port.Electrical.DefaultState == "low"
			break
		}
	}
	if !defaultLow {
		return false
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "transconductance" || assertion.Observation.Kind != "port" ||
			assertion.Observation.ID != outputID {
			continue
		}
		for _, caseID := range assertion.OperatingCases {
			operatingCase, found := cases[caseID]
			if !found {
				continue
			}
			for _, condition := range operatingCase.Conditions {
				if condition.Target == controlID && condition.Axis == "input_voltage" && condition.Min > 0 {
					return true
				}
			}
			for _, event := range operatingCase.Events {
				if event.Target == controlID && event.Kind == "input_step" &&
					event.Initial <= 0 && event.Applied > event.Initial {
					return true
				}
			}
		}
	}
	return false
}

func topologyLowSideTransconductanceRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	relationships := regulatedCurrentRelationships(requirement)
	if len(relationships) == 0 {
		return nil, Consumption{}, map[string][]string{}
	}
	requireThermal, requireSOA := false, false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		requireThermal = requireThermal || assertion.Metric == "junction_temperature"
		requireSOA = requireSOA || assertion.Metric == "soa_margin"
	}
	passDevice := selectCurrentRelationshipPrimitive(requirement, inventory, "npn_bjt", requireThermal, requireSOA)
	controller := topologyLowSideCurrentControllerPrimitive(requirement, inventory, passDevice)
	gateClamp := selectCurrentRelationshipPrimitive(requirement, inventory, "pnp_bjt", false, false)
	resistor := PrimitiveCandidate{}
	for _, primitive := range representatives {
		if primitive.Kind == "resistor" {
			resistor = primitive
			break
		}
	}
	if passDevice.Key == "" || controller.Key == "" || gateClamp.Key == "" || resistor.Key == "" {
		return nil, Consumption{}, map[string][]string{"current_sink_primitives_unavailable": {"reviewed controller, NPN pass device, enable switch, and resistor are required"}}
	}
	supplies := topologyNodesByRole(initial.graph, "supply")
	references := topologyNodesByRole(initial.graph, "reference")
	if len(supplies) != 1 || len(references) == 0 {
		return nil, Consumption{}, map[string][]string{}
	}
	transconductance := requirementTransconductance(requirement)
	if transconductance <= 0 || !finite(transconductance) {
		return nil, Consumption{}, map[string][]string{}
	}
	senseParts := currentSenseSeriesComposition(ctx, requirement, inventory, 1/transconductance, 4)
	if len(senseParts) == 0 {
		return nil, Consumption{}, map[string][]string{"current_sink_primitives_unavailable": {"no reviewed resistor values can realize the current-sense relationship"}}
	}
	senseSegments := len(senseParts)
	consumption := Consumption{}
	rejections := map[string][]string{}
	retained := map[string]TopologyCandidate{}
	for _, relationship := range relationships {
		if relationship.direction != "sink" || relationship.activation == "" || relationship.fault == "" {
			continue
		}
		switchDevice := selectClampedCurrentRelationshipSwitchPrimitive(
			requirement, inventory, "n_channel_mosfet", relationship.activation, requireThermal, requireSOA,
		)
		if switchDevice.Key == "" {
			rejections["current_sink_primitives_unavailable"] = append(rejections["current_sink_primitives_unavailable"],
				"no reviewed enable switch has guaranteed gate-drive margin")
			continue
		}
		if ctx.Err() != nil || consumption.ExpandedStates >= policy.MaxExpandedStates ||
			consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
			consumption.BudgetExhausted = true
			break
		}
		consumption.ExpandedStates++
		input := externalRelationshipNode(initial.graph, relationship.input)
		output := externalRelationshipNode(initial.graph, relationship.output)
		activation := externalRelationshipNode(initial.graph, relationship.activation)
		fault := externalRelationshipNode(initial.graph, relationship.fault)
		reference := references[0]
		supply := supplies[0]
		if input == "" || output == "" || activation == "" || fault == "" {
			continue
		}
		state := initial
		internalCount := 6 + senseSegments - 1
		internal := make([]string, 0, internalCount)
		for len(internal) < internalCount {
			var node string
			state, node = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
			if node == "" {
				break
			}
			internal = append(internal, node)
		}
		if len(internal) != internalCount || internalNodeCount(state.graph) > limits.MaxInternalNodes {
			continue
		}
		drive, sense, switchedReturn := internal[0], internal[1], internal[2]
		gateDrive, activationBase, faultBase := internal[3], internal[4], internal[5]
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, controller, []TerminalConnection{
			{Terminal: "IN_MINUS", Node: sense},
			{Terminal: "IN_PLUS", Node: input},
			{Terminal: "OUT", Node: drive},
			{Terminal: "V_MINUS", Node: reference},
			{Terminal: "V_PLUS", Node: supply},
		}, &consumption)
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, passDevice, []TerminalConnection{
			{Terminal: "BASE", Node: drive},
			{Terminal: "COLLECTOR", Node: output},
			{Terminal: "EMITTER", Node: sense},
		}, &consumption)
		senseNodes := append([]string{sense}, internal[6:]...)
		senseNodes = append(senseNodes, switchedReturn)
		for index := 0; index+1 < len(senseNodes); index++ {
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, senseParts[index],
				topologyTwoTerminalPlacement(senseNodes[index], senseNodes[index+1]), &consumption)
		}
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, switchDevice, []TerminalConnection{
			{Terminal: "DRAIN", Node: switchedReturn},
			{Terminal: "GATE", Node: gateDrive},
			{Terminal: "SOURCE", Node: reference},
		}, &consumption)
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, gateClamp, []TerminalConnection{
			{Terminal: "BASE", Node: activationBase},
			{Terminal: "COLLECTOR", Node: reference},
			{Terminal: "EMITTER", Node: gateDrive},
		}, &consumption)
		for _, edge := range [][2]string{{supply, gateDrive}, {activation, activationBase}, {activation, reference}} {
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor,
				topologyTwoTerminalPlacement(edge[0], edge[1]), &consumption)
		}
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, passDevice, []TerminalConnection{
			{Terminal: "BASE", Node: faultBase},
			{Terminal: "COLLECTOR", Node: gateDrive},
			{Terminal: "EMITTER", Node: reference},
		}, &consumption)
		for _, edge := range [][2]string{{fault, faultBase}, {faultBase, reference}} {
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor,
				topologyTwoTerminalPlacement(edge[0], edge[1]), &consumption)
		}
		if len(state.graph.Instances) > limits.MaxPrimitiveInstances {
			continue
		}
		if issues := ValidateCompleteGraph(state.graph, inventory, limits); len(issues) != 0 {
			for _, issue := range issues {
				rejections[string(issue.Code)] = append(rejections[string(issue.Code)], issue.Path+":"+issue.Message)
			}
			continue
		}
		if state.score.BehaviorGap != 0 {
			rejections["relationship_gap"] = append(rejections["relationship_gap"], fmt.Sprintf("%s:gap=%d", state.hash, state.score.BehaviorGap))
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
		candidate := TopologyCandidate{Fingerprint: state.hash, TopologyHash: topologyHash, Score: state.score, Graph: normalized, Operations: cloneGraphOperations(state.operations)}
		if existing, found := retained[topologyHash]; !found || compareTopologyCandidates(candidate, existing) < 0 {
			retained[topologyHash] = candidate
		}
	}
	result := make([]TopologyCandidate, 0, len(retained))
	for _, candidate := range retained {
		result = append(result, candidate)
	}
	slices.SortFunc(result, compareTopologyCandidates)
	return result, consumption, rejections
}

func selectCurrentRelationshipPrimitive(
	requirement Requirement,
	inventory PrimitiveInventory,
	kind string,
	requireThermal bool,
	requireSOA bool,
) PrimitiveCandidate {
	return selectCurrentRelationshipPrimitiveMatching(
		requirement, inventory, kind, requireThermal, requireSOA, nil,
	)
}

func selectCurrentRelationshipPrimitiveMatching(
	requirement Requirement,
	inventory PrimitiveInventory,
	kind string,
	requireThermal bool,
	requireSOA bool,
	accept func(PrimitiveCandidate) bool,
) PrimitiveCandidate {
	requiredAnalyses := requirementAnalysisSet(requirement)
	candidates := []PrimitiveCandidate{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != kind || !primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			!ratingsCoverRequirement(requirement, primitive) ||
			(requireThermal && !primitiveHasThermalEvidence(primitive)) ||
			(requireSOA && !primitiveHasSOAEvidence(primitive)) ||
			(accept != nil && !accept(primitive)) {
			continue
		}
		candidates = append(candidates, primitive)
	}
	slices.SortFunc(candidates, func(left, right PrimitiveCandidate) int {
		return compareRepresentativePrimitives(left, right, requiredAnalyses)
	})
	if len(candidates) == 0 {
		return PrimitiveCandidate{}
	}
	return candidates[0]
}

func selectClampedCurrentRelationshipSwitchPrimitive(
	requirement Requirement,
	inventory PrimitiveInventory,
	kind string,
	control string,
	requireThermal bool,
	requireSOA bool,
) PrimitiveCandidate {
	minimumHigh, maximumHigh, found := requirementControlHighVoltageRange(requirement, control)
	if !found {
		return PrimitiveCandidate{}
	}
	return selectCurrentRelationshipPrimitiveMatching(
		requirement, inventory, kind, requireThermal, requireSOA,
		func(primitive PrimitiveCandidate) bool {
			return primitiveHasClampedMOSFETGateDrive(primitive, minimumHigh, maximumHigh)
		},
	)
}

func selectSupplyDrivenCurrentRelationshipSwitchPrimitive(
	requirement Requirement,
	inventory PrimitiveInventory,
	kind string,
	requireThermal bool,
) PrimitiveCandidate {
	minimumSupply := math.Inf(1)
	maximumSupply := 0.0
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind != "supply" {
			continue
		}
		if domain.MinVoltageV != nil && *domain.MinVoltageV > 0 {
			minimumSupply = math.Min(minimumSupply, *domain.MinVoltageV)
		}
		if domain.MaxVoltageV != nil {
			maximumSupply = math.Max(maximumSupply, *domain.MaxVoltageV)
		}
	}
	if math.IsInf(minimumSupply, 1) || maximumSupply <= 0 {
		return PrimitiveCandidate{}
	}
	return selectCurrentRelationshipPrimitiveMatching(
		requirement, inventory, kind, requireThermal, false,
		func(primitive PrimitiveCandidate) bool {
			for _, model := range primitive.Models {
				if model.ModelID != simmodel.PrimitivePMOSSwitchV1 {
					continue
				}
				parameters := map[string]float64{}
				for _, parameter := range model.Parameters {
					parameters[parameter.Name] = parameter.Value
				}
				if parameters["gate_on_voltage_v"] > 0 &&
					parameters["gate_on_voltage_v"] <= minimumSupply &&
					parameters["max_gate_source_voltage_v"] >= maximumSupply {
					return true
				}
			}
			return false
		},
	)
}

func requirementControlHighVoltageRange(requirement Requirement, control string) (float64, float64, bool) {
	minimumHigh := math.Inf(1)
	maximumHigh := 0.0
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Target != control || condition.Axis != "input_voltage" || condition.Min <= 0 {
				continue
			}
			minimumHigh = math.Min(minimumHigh, condition.Min)
			maximumHigh = math.Max(maximumHigh, condition.Max)
		}
		for _, event := range operatingCase.Events {
			if event.Target != control || event.Kind != "input_step" ||
				event.Applied <= event.Initial || event.Applied <= 0 {
				continue
			}
			minimumHigh = math.Min(minimumHigh, event.Applied)
			maximumHigh = math.Max(maximumHigh, event.Applied)
		}
	}
	if math.IsInf(minimumHigh, 1) || maximumHigh <= 0 {
		return 0, 0, false
	}
	return minimumHigh, maximumHigh, true
}

func primitiveHasClampedMOSFETGateDrive(primitive PrimitiveCandidate, minimumHigh, maximumHigh float64) bool {
	const conservativeJunctionDropV = 1.0
	for _, model := range primitive.Models {
		if model.ModelID != simmodel.PrimitiveNMOSSwitchV1 && model.ModelID != simmodel.PrimitivePMOSSwitchV1 {
			continue
		}
		parameters := map[string]float64{}
		for _, parameter := range model.Parameters {
			parameters[parameter.Name] = parameter.Value
		}
		if parameters["gate_on_voltage_v"] > 0 &&
			parameters["gate_on_voltage_v"] <= minimumHigh+conservativeJunctionDropV &&
			parameters["max_gate_source_voltage_v"] >= maximumHigh+conservativeJunctionDropV {
			return true
		}
	}
	return false
}

func currentSenseSeriesComposition(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	targetResistance float64,
	maximum int,
) []PrimitiveCandidate {
	if targetResistance <= 0 || !finite(targetResistance) || maximum <= 0 {
		return nil
	}
	requiredAnalyses := requirementAnalysisSet(requirement)
	byValue := map[uint64]PrimitiveCandidate{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "resistor" || primitive.ValueDomain.Nominal == nil ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			!ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		value := *primitive.ValueDomain.Nominal
		if value <= 0 || !finite(value) {
			continue
		}
		valueKey := math.Float64bits(value)
		existing, found := byValue[valueKey]
		primitiveTolerance, primitiveToleranceProven := primitiveTolerancePercent(primitive, "resistance")
		existingTolerance, existingToleranceProven := primitiveTolerancePercent(existing, "resistance")
		if !found ||
			(primitiveToleranceProven && !existingToleranceProven) ||
			(primitiveToleranceProven == existingToleranceProven && primitiveTolerance < existingTolerance) ||
			(primitiveToleranceProven == existingToleranceProven && primitiveTolerance == existingTolerance &&
				compareRepresentativePrimitives(primitive, existing, requiredAnalyses) < 0) {
			byValue[valueKey] = primitive
		}
	}
	choices := make([]PrimitiveCandidate, 0, len(byValue))
	for _, primitive := range byValue {
		choices = append(choices, primitive)
	}
	const maximumDifferentialResistorChoices = MaxComponents
	slices.SortFunc(choices, func(left, right PrimitiveCandidate) int {
		leftTolerance, leftProven := primitiveTolerancePercent(left, "resistance")
		rightTolerance, rightProven := primitiveTolerancePercent(right, "resistance")
		leftUnproven, rightUnproven := 0, 0
		if !leftProven {
			leftUnproven = 1
		}
		if !rightProven {
			rightUnproven = 1
		}
		return cmp.Or(
			cmp.Compare(leftUnproven, rightUnproven),
			cmp.Compare(leftTolerance, rightTolerance),
			cmp.Compare(
				multiplicativeRelativeError(*left.ValueDomain.Nominal, 10_000),
				multiplicativeRelativeError(*right.ValueDomain.Nominal, 10_000),
			),
			cmp.Compare(*left.ValueDomain.Nominal, *right.ValueDomain.Nominal),
			cmp.Compare(left.Key, right.Key),
		)
	})
	if len(choices) > maximumDifferentialResistorChoices {
		choices = choices[:maximumDifferentialResistorChoices]
	}
	slices.SortFunc(choices, func(left, right PrimitiveCandidate) int {
		return cmp.Or(
			cmp.Compare(*left.ValueDomain.Nominal, *right.ValueDomain.Nominal),
			cmp.Compare(left.Key, right.Key),
		)
	})
	best := []PrimitiveCandidate(nil)
	bestError := math.Inf(1)
	var visit func(start int, sum float64, selected []PrimitiveCandidate)
	visit = func(start int, sum float64, selected []PrimitiveCandidate) {
		if ctx.Err() != nil {
			return
		}
		if len(selected) != 0 {
			error := math.Abs(sum-targetResistance) / targetResistance
			if error < bestError ||
				(error == bestError && (len(best) == 0 || len(selected) < len(best))) ||
				(error == bestError && len(selected) == len(best) && slices.CompareFunc(selected, best, func(left, right PrimitiveCandidate) int {
					return cmp.Compare(left.Key, right.Key)
				}) < 0) {
				bestError = error
				best = append([]PrimitiveCandidate(nil), selected...)
			}
		}
		// Every reviewed resistance is positive and choices are sorted. Once a
		// branch reaches or exceeds the target, adding another part can only make
		// its error worse.
		if len(selected) == maximum || sum >= targetResistance {
			return
		}
		for index := start; index < len(choices); index++ {
			if ctx.Err() != nil {
				return
			}
			value := *choices[index].ValueDomain.Nominal
			nextSum := sum + value
			if !math.IsInf(bestError, 1) && nextSum > targetResistance &&
				(nextSum-targetResistance)/targetResistance > bestError {
				break
			}
			visit(index, nextSum, append(selected, choices[index]))
		}
	}
	visit(0, 0, nil)
	return best
}

// currentSenseSeriesParallelComposition searches a bounded series chain whose
// segments may contain one reviewed resistor or two reviewed resistors in
// parallel. It is deliberately topology-generic: values are selected only by
// effective resistance, evidence coverage, part count, and stable catalog key.
func currentSenseSeriesParallelComposition(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	targetResistance float64,
	maximumParts int,
) (currentSenseResistanceNetwork, bool) {
	return currentSenseSeriesParallelCompositionWithin(
		ctx, requirement, inventory, targetResistance, maximumParts, 0, math.Inf(1),
	)
}

func currentSenseSeriesParallelCompositionWithin(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	targetResistance float64,
	maximumParts int,
	minimumResistance float64,
	maximumResistance float64,
) (currentSenseResistanceNetwork, bool) {
	if targetResistance <= 0 || !finite(targetResistance) || maximumParts <= 0 ||
		minimumResistance < 0 || maximumResistance < minimumResistance {
		return currentSenseResistanceNetwork{}, false
	}
	requiredAnalyses := requirementAnalysisSet(requirement)
	choices := []PrimitiveCandidate{}
	seenValues := map[uint64]struct{}{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "resistor" || primitive.ValueDomain.Nominal == nil ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			!ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		value := *primitive.ValueDomain.Nominal
		if value <= 0 || !finite(value) {
			continue
		}
		key := math.Float64bits(value)
		if _, found := seenValues[key]; found {
			continue
		}
		seenValues[key] = struct{}{}
		choices = append(choices, primitive)
	}
	slices.SortFunc(choices, func(left, right PrimitiveCandidate) int {
		return cmp.Or(cmp.Compare(*left.ValueDomain.Nominal, *right.ValueDomain.Nominal), cmp.Compare(left.Key, right.Key))
	})
	type segment struct {
		parts      []PrimitiveCandidate
		resistance float64
		key        string
	}
	segments := []segment{}
	for _, choice := range choices {
		value := *choice.ValueDomain.Nominal
		segments = append(segments,
			segment{parts: []PrimitiveCandidate{choice}, resistance: value, key: choice.Key},
			segment{parts: []PrimitiveCandidate{choice, choice}, resistance: value / 2, key: choice.Key + "+" + choice.Key},
		)
	}
	slices.SortFunc(segments, func(left, right segment) int {
		return cmp.Or(cmp.Compare(left.resistance, right.resistance), cmp.Compare(len(left.parts), len(right.parts)), cmp.Compare(left.key, right.key))
	})
	best := currentSenseResistanceNetwork{}
	bestParts := 0
	bestError := math.Inf(1)
	bestBelowTarget := true
	bestKey := ""
	var visit func(start, partCount int, resistance float64, selected [][]PrimitiveCandidate, key string)
	visit = func(start, partCount int, resistance float64, selected [][]PrimitiveCandidate, key string) {
		if ctx.Err() != nil {
			return
		}
		if partCount != 0 && resistance >= minimumResistance && resistance <= maximumResistance {
			error := math.Abs(resistance-targetResistance) / targetResistance
			belowTarget := resistance < targetResistance
			if bestParts == 0 || (bestBelowTarget && !belowTarget) ||
				(bestBelowTarget == belowTarget && error < bestError) ||
				(bestBelowTarget == belowTarget && error == bestError && partCount < bestParts) ||
				(bestBelowTarget == belowTarget && error == bestError && partCount == bestParts && key < bestKey) {
				bestError, bestParts, bestBelowTarget, bestKey = error, partCount, belowTarget, key
				best = currentSenseResistanceNetwork{segments: clonePrimitiveSegments(selected), effectiveResistance: resistance}
			}
		}
		if partCount >= maximumParts || resistance >= targetResistance {
			return
		}
		for index := start; index < len(segments); index++ {
			candidate := segments[index]
			if partCount+len(candidate.parts) > maximumParts {
				continue
			}
			nextKey := key
			if nextKey != "" {
				nextKey += "|"
			}
			nextKey += candidate.key
			visit(index, partCount+len(candidate.parts), resistance+candidate.resistance,
				append(selected, candidate.parts), nextKey)
		}
	}
	visit(0, 0, 0, nil, "")
	return best, len(best.segments) != 0
}

func clonePrimitiveSegments(segments [][]PrimitiveCandidate) [][]PrimitiveCandidate {
	result := make([][]PrimitiveCandidate, len(segments))
	for index := range segments {
		result[index] = append([]PrimitiveCandidate(nil), segments[index]...)
	}
	return result
}

func currentSenseDifferentialComposition(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	targetResistance float64,
) (currentSenseDifferentialValues, bool) {
	if targetResistance <= 0 || !finite(targetResistance) {
		return currentSenseDifferentialValues{}, false
	}
	requiredAnalyses := requirementAnalysisSet(requirement)
	byValue := map[uint64]PrimitiveCandidate{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "resistor" || primitive.ValueDomain == nil ||
			primitive.ValueDomain.Nominal == nil ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			!ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		value := *primitive.ValueDomain.Nominal
		if value <= 0 || !finite(value) {
			continue
		}
		valueKey := math.Float64bits(value)
		existing, found := byValue[valueKey]
		primitiveTolerance, primitiveToleranceProven := primitiveTolerancePercent(primitive, "resistance")
		existingTolerance, existingToleranceProven := primitiveTolerancePercent(existing, "resistance")
		if !found ||
			(primitiveToleranceProven && !existingToleranceProven) ||
			(primitiveToleranceProven == existingToleranceProven && primitiveTolerance < existingTolerance) ||
			(primitiveToleranceProven == existingToleranceProven && primitiveTolerance == existingTolerance &&
				compareRepresentativePrimitives(primitive, existing, requiredAnalyses) < 0) {
			byValue[valueKey] = primitive
		}
	}
	choices := make([]PrimitiveCandidate, 0, len(byValue))
	for _, primitive := range byValue {
		choices = append(choices, primitive)
	}
	slices.SortFunc(choices, func(left, right PrimitiveCandidate) int {
		return cmp.Or(
			cmp.Compare(*left.ValueDomain.Nominal, *right.ValueDomain.Nominal),
			cmp.Compare(left.Key, right.Key),
		)
	})
	if len(choices) == 0 {
		return currentSenseDifferentialValues{}, false
	}

	minimumEffective := 0.0
	maximumEffective := math.Inf(1)
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "transconductance" {
			continue
		}
		if assertion.Max != nil && *assertion.Max > 0 {
			minimumEffective = 1 / *assertion.Max
		}
		if assertion.Min != nil && *assertion.Min > 0 {
			maximumEffective = 1 / *assertion.Min
		}
		break
	}

	const (
		observationAnchorResistance = 10_000.0
		maximumParallelArmParts     = 4
	)
	pairs := make([]currentSenseDifferentialPair, 0, len(choices)*len(choices)*maximumParallelArmParts*maximumParallelArmParts)
	for _, input := range choices {
		if ctx.Err() != nil {
			return currentSenseDifferentialValues{}, false
		}
		inputValue := *input.ValueDomain.Nominal
		inputTolerance, inputToleranceProven := primitiveTolerancePercent(input, "resistance")
		if requirement.Acceptance.RequireAllCorners && !inputToleranceProven {
			continue
		}
		inputFraction := inputTolerance / 100
		if inputFraction >= 1 {
			continue
		}
		for inputCount := 1; inputCount <= maximumParallelArmParts; inputCount++ {
			effectiveInput := inputValue / float64(inputCount)
			for _, feedback := range choices {
				feedbackValue := *feedback.ValueDomain.Nominal
				feedbackTolerance, feedbackToleranceProven := primitiveTolerancePercent(feedback, "resistance")
				if requirement.Acceptance.RequireAllCorners && !feedbackToleranceProven {
					continue
				}
				feedbackFraction := feedbackTolerance / 100
				for feedbackCount := 1; feedbackCount <= maximumParallelArmParts; feedbackCount++ {
					effectiveFeedback := feedbackValue / float64(feedbackCount)
					pairs = append(pairs, currentSenseDifferentialPair{
						input: input, inputCount: inputCount, inputResistance: effectiveInput,
						feedback: feedback, feedbackCount: feedbackCount, feedbackResistance: effectiveFeedback,
						ratio:              effectiveFeedback / effectiveInput,
						cornerMinimumRatio: effectiveFeedback * (1 - feedbackFraction) / (effectiveInput * (1 + inputFraction)),
						cornerMaximumRatio: effectiveFeedback * (1 + feedbackFraction) / (effectiveInput * (1 - inputFraction)),
						observationPenalty: multiplicativeRelativeError(effectiveInput, observationAnchorResistance) +
							multiplicativeRelativeError(effectiveFeedback, observationAnchorResistance),
					})
				}
			}
		}
	}
	slices.SortFunc(pairs, func(left, right currentSenseDifferentialPair) int {
		return cmp.Or(
			cmp.Compare(left.ratio, right.ratio),
			cmp.Compare(left.observationPenalty, right.observationPenalty),
			cmp.Compare(left.input.Key, right.input.Key),
			cmp.Compare(left.inputCount, right.inputCount),
			cmp.Compare(left.feedback.Key, right.feedback.Key),
			cmp.Compare(left.feedbackCount, right.feedbackCount),
		)
	})
	if len(pairs) == 0 {
		return currentSenseDifferentialValues{}, false
	}

	type shuntOption struct {
		primitive  PrimitiveCandidate
		count      int
		resistance float64
		key        string
	}
	shunts := make([]shuntOption, 0, len(choices)*4)
	for _, primitive := range choices {
		for count := 1; count <= 4; count++ {
			shunts = append(shunts, shuntOption{
				primitive: primitive,
				count:     count, resistance: *primitive.ValueDomain.Nominal / float64(count),
				key: fmt.Sprintf("%s*%d", primitive.Key, count),
			})
		}
	}
	slices.SortFunc(shunts, func(left, right shuntOption) int {
		return cmp.Or(
			cmp.Compare(left.resistance, right.resistance),
			cmp.Compare(left.count, right.count),
			cmp.Compare(left.key, right.key),
		)
	})

	best := currentSenseDifferentialValues{}
	bestShuntCount := int(^uint(0) >> 1)
	bestNominalError := math.Inf(1)
	bestShuntPenalty := math.Inf(1)
	bestObservationPenalty := math.Inf(1)
	maximumShuntResistance := regulatedCurrentMaximumShuntResistance(requirement)
	for _, option := range shunts {
		if ctx.Err() != nil {
			return currentSenseDifferentialValues{}, false
		}
		shuntValue := option.resistance
		shuntTolerance, shuntToleranceProven := primitiveTolerancePercent(option.primitive, "resistance")
		if requirement.Acceptance.RequireAllCorners && !shuntToleranceProven {
			continue
		}
		shuntFraction := shuntTolerance / 100
		if maximumShuntResistance > 0 && finite(maximumShuntResistance) &&
			shuntValue*(1+shuntFraction) >= maximumShuntResistance {
			continue
		}
		targetRatio := targetResistance / shuntValue
		insertion := sort.Search(len(pairs), func(index int) bool { return pairs[index].ratio >= targetRatio })
		left, right := insertion-1, insertion
		bestPairDelta := math.Inf(1)
		for left >= 0 || right < len(pairs) {
			if ctx.Err() != nil {
				return currentSenseDifferentialValues{}, false
			}
			leftDelta, rightDelta := math.Inf(1), math.Inf(1)
			if left >= 0 {
				leftDelta = math.Abs(pairs[left].ratio - targetRatio)
			}
			if right < len(pairs) {
				rightDelta = math.Abs(pairs[right].ratio - targetRatio)
			}
			if math.Min(leftDelta, rightDelta) > bestPairDelta {
				break
			}
			pairIndex := right
			if leftDelta <= rightDelta {
				pairIndex = left
				left--
			} else {
				right++
			}
			pair := pairs[pairIndex]
			cornerMinimum := shuntValue * (1 - shuntFraction) * pair.cornerMinimumRatio
			cornerMaximum := shuntValue * (1 + shuntFraction) * pair.cornerMaximumRatio
			if requirement.Acceptance.RequireAllCorners &&
				(cornerMinimum < minimumEffective || cornerMaximum > maximumEffective) {
				continue
			}
			pairDelta := math.Abs(pair.ratio - targetRatio)
			bestPairDelta = math.Min(bestPairDelta, pairDelta)
			effective := shuntValue * pair.ratio
			nominalError := math.Abs(effective-targetResistance) / targetResistance
			// Once the effective feedback resistance and its tolerance are
			// satisfied, minimize series burden in the delivered-current path.
			// The differential ratio recovers observation amplitude without
			// spending compliance voltage in the shunt.
			shuntPenalty := shuntValue
			if maximumShuntResistance > 0 {
				shuntPenalty = multiplicativeRelativeError(shuntValue, math.Min(targetResistance, maximumShuntResistance))
			}
			candidate := currentSenseDifferentialValues{
				shunt: option.primitive, shuntCount: option.count, shuntResistance: option.resistance,
				input: pair.input, inputCount: pair.inputCount, inputResistance: pair.inputResistance,
				feedback: pair.feedback, feedbackCount: pair.feedbackCount, feedbackResistance: pair.feedbackResistance,
				effectiveResistance: effective,
			}
			if option.count < bestShuntCount ||
				(option.count == bestShuntCount && shuntPenalty < bestShuntPenalty) ||
				(option.count == bestShuntCount && shuntPenalty == bestShuntPenalty && nominalError < bestNominalError) ||
				(option.count == bestShuntCount && shuntPenalty == bestShuntPenalty && nominalError == bestNominalError &&
					pair.observationPenalty < bestObservationPenalty) ||
				(option.count == bestShuntCount && shuntPenalty == bestShuntPenalty && nominalError == bestNominalError &&
					pair.observationPenalty == bestObservationPenalty && (best.shunt.Key == "" || compareCurrentSenseDifferentialKeys(candidate, best) < 0)) {
				bestShuntCount = option.count
				bestNominalError = nominalError
				bestShuntPenalty = shuntPenalty
				bestObservationPenalty = pair.observationPenalty
				best = candidate
			}
		}
	}
	return best, best.shunt.Key != ""
}

// regulatedCurrentMaximumShuntResistance bounds sense insertion loss using
// the declared current rating and the compliance left by the largest declared
// load at the lowest qualified external rail. When no compliance remains, the
// caller deliberately falls back to the least-burden reviewed shunt.
func regulatedCurrentMaximumShuntResistance(requirement Requirement) float64 {
	maximumCurrent := 0.0
	outputs := map[string]bool{}
	for _, relationship := range regulatedCurrentRelationships(requirement) {
		outputs[relationship.output] = true
	}
	for _, port := range requirement.Requirements.Ports {
		if outputs[port.ID] && port.Electrical.MaxCurrentA != nil && *port.Electrical.MaxCurrentA > 0 {
			maximumCurrent = math.Max(maximumCurrent, *port.Electrical.MaxCurrentA)
		}
	}
	if maximumCurrent <= 0 || !finite(maximumCurrent) {
		return math.Inf(1)
	}
	minimumRail := math.Inf(1)
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind != "supply" || domain.MinVoltageV == nil || *domain.MinVoltageV <= 0 ||
			domain.MaxCurrentA == nil || *domain.MaxCurrentA < maximumCurrent {
			continue
		}
		minimumRail = math.Min(minimumRail, *domain.MinVoltageV)
	}
	if !finite(minimumRail) {
		return math.Inf(1)
	}
	maximumLoadResistance := 0.0
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Axis == "load_resistance" && outputs[condition.Target] {
				maximumLoadResistance = math.Max(maximumLoadResistance, condition.Max)
			}
		}
	}
	headroom := minimumRail - maximumCurrent*maximumLoadResistance
	if headroom <= 0 {
		return 0
	}
	return headroom / maximumCurrent
}

// topologyHighSidePassDeviceCount derives the minimum parallel pass count from
// the declared worst-case electrical and thermal envelopes. Parallel current
// sharing is introduced only when the calculated package limit requires it.
func topologyHighSidePassDeviceCount(requirement Requirement, primitive PrimitiveCandidate) int {
	const minimumThermalPassCount = 1
	maximumAmbient := topologyMaximumAmbientTemperature(requirement)
	if !finite(maximumAmbient) {
		maximumAmbient = 25
	}
	maximumJunction := math.Inf(1)
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "junction_temperature" && assertion.Max != nil && *assertion.Max > 0 {
			maximumJunction = math.Min(maximumJunction, *assertion.Max)
		}
	}
	theta := 0.0
	for _, model := range primitive.Models {
		if model.ModelID != simmodel.PrimitiveBJTPNPV1 {
			continue
		}
		modelTheta := 0.0
		for _, parameter := range model.Parameters {
			switch parameter.Name {
			case "thermal_resistance_c_per_w", "junction_to_ambient_c_per_w":
				modelTheta = math.Max(modelTheta, parameter.Value)
			case "max_temperature_c":
				if parameter.Value > 0 {
					maximumJunction = math.Min(maximumJunction, parameter.Value)
				}
			}
		}
		for _, uncertainty := range model.Uncertainties {
			switch uncertainty.Target {
			case "model_parameters.thermal_resistance_c_per_w", "model_parameters.junction_to_ambient_c_per_w":
				modelTheta = math.Max(modelTheta, uncertainty.Maximum)
			case "model_parameters.max_temperature_c":
				if uncertainty.Minimum > 0 {
					maximumJunction = math.Min(maximumJunction, uncertainty.Minimum)
				}
			}
		}
		if modelTheta <= 0 && model.ThermalModel != nil && model.ThermalModel.Reference == "junction_to_ambient" {
			for _, stage := range model.ThermalModel.Stages {
				modelTheta += stage.ThermalResistanceCPerW
			}
		}
		theta = math.Max(theta, modelTheta)
	}
	if theta <= 0 || math.IsInf(maximumJunction, 1) || maximumJunction <= maximumAmbient {
		return minimumThermalPassCount
	}
	maximumSupply := 0.0
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind != "supply" {
			continue
		}
		if domain.MaxVoltageV != nil {
			maximumSupply = math.Max(maximumSupply, *domain.MaxVoltageV)
		} else if domain.NominalVoltageV != nil {
			maximumSupply = math.Max(maximumSupply, *domain.NominalVoltageV)
		}
	}
	minimumLoad := math.Inf(1)
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Axis == "load_resistance" && condition.Min > 0 {
				minimumLoad = math.Min(minimumLoad, condition.Min)
			}
		}
	}
	requiredCurrent := requiredTransconductanceOutputCurrent(requirement)
	if maximumSupply <= 0 || requiredCurrent <= 0 {
		return minimumThermalPassCount
	}
	loadVoltage := 0.0
	if !math.IsInf(minimumLoad, 1) {
		loadVoltage = requiredCurrent * minimumLoad
	}
	totalDissipation := requiredCurrent * math.Max(0, maximumSupply-loadVoltage)
	perDeviceDissipation := (maximumJunction - maximumAmbient) / theta
	if totalDissipation <= 0 || perDeviceDissipation <= 0 {
		return minimumThermalPassCount
	}
	requiredCount := math.Ceil(totalDissipation / perDeviceDissipation)
	maximumPassCount := min(requirement.Requirements.Constraints.MaxComponents, MaxComponents)
	if maximumPassCount < minimumThermalPassCount || !finite(requiredCount) || requiredCount > float64(maximumPassCount) {
		return 0
	}
	return max(minimumThermalPassCount, int(requiredCount))
}

func compareCurrentSenseDifferentialKeys(left, right currentSenseDifferentialValues) int {
	return cmp.Or(
		cmp.Compare(left.shunt.Key, right.shunt.Key),
		cmp.Compare(left.shuntCount, right.shuntCount),
		cmp.Compare(left.input.Key, right.input.Key),
		cmp.Compare(left.inputCount, right.inputCount),
		cmp.Compare(left.feedback.Key, right.feedback.Key),
		cmp.Compare(left.feedbackCount, right.feedbackCount),
	)
}

func requirementTransconductance(requirement Requirement) float64 {
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "transconductance" {
			return assertionTarget(assertion)
		}
	}
	return 0
}

func externalRelationshipNode(graph CandidateGraph, semanticID string) string {
	for _, node := range graph.Nodes {
		if node.Scope == "external" && node.SemanticID == semanticID {
			return node.ID
		}
	}
	return ""
}
