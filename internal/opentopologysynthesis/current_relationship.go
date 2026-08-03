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
	input               PrimitiveCandidate
	feedback            PrimitiveCandidate
	effectiveResistance float64
}

type currentSenseDifferentialPair struct {
	input              PrimitiveCandidate
	feedback           PrimitiveCandidate
	ratio              float64
	cornerMinimumRatio float64
	cornerMaximumRatio float64
	observationPenalty float64
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
			faultByOutput[assertion.Observation.ID] = assertion.Excitation.ID
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
			input:     assertion.Excitation.ID,
			output:    assertion.Observation.ID,
			direction: output.Direction,
			fault:     faultByOutput[assertion.Observation.ID],
		}
		activationCandidates := []string{}
		for _, port := range requirement.Requirements.Ports {
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

	const observationAnchorResistance = 10_000.0
	pairs := make([]currentSenseDifferentialPair, 0, len(choices)*len(choices))
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
		for _, feedback := range choices {
			feedbackValue := *feedback.ValueDomain.Nominal
			feedbackTolerance, feedbackToleranceProven := primitiveTolerancePercent(feedback, "resistance")
			if requirement.Acceptance.RequireAllCorners && !feedbackToleranceProven {
				continue
			}
			feedbackFraction := feedbackTolerance / 100
			pairs = append(pairs, currentSenseDifferentialPair{
				input:              input,
				feedback:           feedback,
				ratio:              feedbackValue / inputValue,
				cornerMinimumRatio: feedbackValue * (1 - feedbackFraction) / (inputValue * (1 + inputFraction)),
				cornerMaximumRatio: feedbackValue * (1 + feedbackFraction) / (inputValue * (1 - inputFraction)),
				observationPenalty: multiplicativeRelativeError(inputValue, observationAnchorResistance) +
					multiplicativeRelativeError(feedbackValue, observationAnchorResistance),
			})
		}
	}
	slices.SortFunc(pairs, func(left, right currentSenseDifferentialPair) int {
		return cmp.Or(
			cmp.Compare(left.ratio, right.ratio),
			cmp.Compare(left.observationPenalty, right.observationPenalty),
			cmp.Compare(left.input.Key, right.input.Key),
			cmp.Compare(left.feedback.Key, right.feedback.Key),
		)
	})
	if len(pairs) == 0 {
		return currentSenseDifferentialValues{}, false
	}

	best := currentSenseDifferentialValues{}
	bestNominalError := math.Inf(1)
	bestObservationPenalty := math.Inf(1)
	bestShuntPenalty := math.Inf(1)
	for _, shunt := range choices {
		if ctx.Err() != nil {
			return currentSenseDifferentialValues{}, false
		}
		shuntValue := *shunt.ValueDomain.Nominal
		shuntTolerance, shuntToleranceProven := primitiveTolerancePercent(shunt, "resistance")
		if requirement.Acceptance.RequireAllCorners && !shuntToleranceProven {
			continue
		}
		shuntFraction := shuntTolerance / 100
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
			shuntPenalty := multiplicativeRelativeError(shuntValue, targetResistance)
			candidate := currentSenseDifferentialValues{
				shunt: shunt, input: pair.input, feedback: pair.feedback,
				effectiveResistance: effective,
			}
			if nominalError < bestNominalError ||
				(nominalError == bestNominalError && pair.observationPenalty < bestObservationPenalty) ||
				(nominalError == bestNominalError && pair.observationPenalty == bestObservationPenalty &&
					shuntPenalty < bestShuntPenalty) ||
				(nominalError == bestNominalError && pair.observationPenalty == bestObservationPenalty &&
					shuntPenalty == bestShuntPenalty && (best.shunt.Key == "" || compareCurrentSenseDifferentialKeys(candidate, best) < 0)) {
				bestNominalError = nominalError
				bestObservationPenalty = pair.observationPenalty
				bestShuntPenalty = shuntPenalty
				best = candidate
			}
		}
	}
	return best, best.shunt.Key != ""
}

func compareCurrentSenseDifferentialKeys(left, right currentSenseDifferentialValues) int {
	return cmp.Or(
		cmp.Compare(left.shunt.Key, right.shunt.Key),
		cmp.Compare(left.input.Key, right.input.Key),
		cmp.Compare(left.feedback.Key, right.feedback.Key),
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
