package opentopologysynthesis

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"

	"kicadai/internal/simmodel"
)

const (
	protectedVoltageCurrentSenseDropV = 0.65
	protectedVoltageBallastDropV      = 0.2
	protectedVoltagePassJunctionDropV = 0.8
	protectedVoltageDriveHeadroomV    = 1.0
)

type regulatedVoltageRelationship struct {
	command                  string
	commandKind              string
	output                   string
	gain                     float64
	minimumGain              float64
	maximumGain              float64
	minimumOutputV           float64
	maximumOutputV           float64
	currentLimitA            float64
	maximumRatedCurrentA     float64
	minimumHeadroomV         float64
	minimumRequiredHeadroomV float64
	bidirectional            bool
	requireThermal           bool
	requireSOA               bool
	defaultCommand           string
}

type regulatedVoltageGainEnvelope struct {
	minimum        float64
	maximum        float64
	assertionCount int
}

type protectedVoltageDrivePath struct {
	resistor          PrimitiveCandidate
	sourceClamp       PrimitiveCandidate
	sinkClamp         PrimitiveCandidate
	sourceDriver      PrimitiveCandidate
	sourceDriverCount int
}

// topologyRegulatedVoltageRelationshipSeeds selects between the preserved
// fixed-reference relationship and the more general command-derived protected
// relationship. The discriminator is behavioral: the protected path is only
// entered when one external command is constrained alongside every requested
// output level. No project, fixture, part, or topology name participates.
func topologyRegulatedVoltageRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	if len(regulatedVoltageRelationships(requirement)) != 0 {
		return topologyProtectedVoltageRelationshipSeeds(
			ctx, requirement, inventory, representatives, inventoryByKey,
			limits, policy, initial,
		)
	}
	return topologySimpleRegulatedVoltageRelationshipSeeds(
		ctx, requirement, inventory, representatives, inventoryByKey,
		limits, policy, initial,
	)
}

// regulatedVoltageRelationships derives command/output relationships from
// operating cases shared with output-voltage assertions. The feasible gain is
// the intersection of every declared output and command interval. Signed load
// cases and fault events independently establish a bidirectional obligation.
func regulatedVoltageRelationships(requirement Requirement) []regulatedVoltageRelationship {
	ports := make(map[string]Port, len(requirement.Requirements.Ports))
	for _, port := range requirement.Requirements.Ports {
		ports[port.ID] = port
	}
	cases := make(map[string]OperatingCase, len(requirement.Requirements.OperatingCases))
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		cases[operatingCase.ID] = operatingCase
	}
	outputs := map[string][]BehavioralAssertion{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "output_voltage" || assertion.Observation.Kind != "port" {
			continue
		}
		port, found := ports[assertion.Observation.ID]
		if !found || port.Kind != "power" || port.Direction != "source" {
			continue
		}
		outputs[port.ID] = append(outputs[port.ID], assertion)
	}
	result := []regulatedVoltageRelationship{}
	for outputID, assertions := range outputs {
		envelopes := map[string]regulatedVoltageGainEnvelope{}
		for _, assertion := range assertions {
			seenForAssertion := map[string]bool{}
			for _, caseID := range assertion.OperatingCases {
				operatingCase, found := cases[caseID]
				if !found {
					continue
				}
				for _, condition := range operatingCase.Conditions {
					command, found := ports[condition.Target]
					if !found || condition.Axis != "input_voltage" || condition.Min <= 0 ||
						condition.Max < condition.Min ||
						!slices.Contains([]string{"analog_voltage", "control", "digital"}, command.Kind) {
						continue
					}
					minimumOutput := assertionTarget(assertion)
					maximumOutput := minimumOutput
					if assertion.Min != nil {
						minimumOutput = *assertion.Min
					}
					if assertion.Max != nil {
						maximumOutput = *assertion.Max
					}
					if minimumOutput <= 0 || maximumOutput < minimumOutput {
						continue
					}
					envelope, exists := envelopes[command.ID]
					if !exists {
						envelope.minimum = 0
						envelope.maximum = math.Inf(1)
					}
					envelope.minimum = math.Max(envelope.minimum, minimumOutput/condition.Max)
					envelope.maximum = math.Min(envelope.maximum, maximumOutput/condition.Min)
					if !seenForAssertion[command.ID] {
						envelope.assertionCount++
						seenForAssertion[command.ID] = true
					}
					envelopes[command.ID] = envelope
				}
			}
		}
		commands := make([]string, 0, len(envelopes))
		for commandID, envelope := range envelopes {
			if envelope.assertionCount == len(assertions) && envelope.minimum > 0 &&
				finite(envelope.maximum) && envelope.maximum >= envelope.minimum {
				commands = append(commands, commandID)
			}
		}
		slices.Sort(commands)
		if len(commands) != 1 {
			continue
		}
		commandID := commands[0]
		envelope := envelopes[commandID]
		relationship := regulatedVoltageRelationship{
			command:                  commandID,
			commandKind:              ports[commandID].Kind,
			output:                   outputID,
			minimumGain:              envelope.minimum,
			maximumGain:              envelope.maximum,
			gain:                     (envelope.minimum + envelope.maximum) / 2,
			defaultCommand:           ports[commandID].Electrical.DefaultState,
			minimumHeadroomV:         math.Inf(1),
			minimumRequiredHeadroomV: math.Inf(1),
		}
		if ports[outputID].Electrical.MaxCurrentA != nil {
			relationship.maximumRatedCurrentA = *ports[outputID].Electrical.MaxCurrentA
		}
		for _, assertion := range assertions {
			minimumValue := assertionTarget(assertion)
			if assertion.Min != nil {
				minimumValue = *assertion.Min
			}
			if relationship.minimumOutputV == 0 || minimumValue < relationship.minimumOutputV {
				relationship.minimumOutputV = minimumValue
			}
			value := assertionTarget(assertion)
			if assertion.Max != nil {
				value = *assertion.Max
			}
			relationship.maximumOutputV = math.Max(relationship.maximumOutputV, value)
			for _, caseID := range assertion.OperatingCases {
				for _, condition := range cases[caseID].Conditions {
					if condition.Axis == "supply_voltage" && condition.Min > 0 {
						relationship.minimumHeadroomV = math.Min(
							relationship.minimumHeadroomV,
							condition.Min-value,
						)
						relationship.minimumRequiredHeadroomV = math.Min(
							relationship.minimumRequiredHeadroomV,
							condition.Min-minimumValue,
						)
					}
				}
			}
		}
		if math.IsInf(relationship.minimumHeadroomV, 1) {
			for _, domain := range requirement.Requirements.Domains {
				if domain.Kind == "supply" && domain.MinVoltageV != nil && *domain.MinVoltageV > 0 {
					relationship.minimumHeadroomV = math.Min(
						relationship.minimumHeadroomV,
						*domain.MinVoltageV-relationship.maximumOutputV,
					)
				}
			}
		}
		if math.IsInf(relationship.minimumRequiredHeadroomV, 1) {
			relationship.minimumRequiredHeadroomV = relationship.minimumHeadroomV
		}
		for _, assertion := range requirement.Requirements.BehavioralRequirements {
			if assertion.Observation.Kind == "port" && assertion.Observation.ID == outputID &&
				assertion.Metric == "peak_current" && assertion.Max != nil && *assertion.Max > 0 &&
				(relationship.currentLimitA == 0 || *assertion.Max < relationship.currentLimitA) {
				relationship.currentLimitA = *assertion.Max
			}
			relationship.requireThermal = relationship.requireThermal || assertion.Metric == "junction_temperature"
			relationship.requireSOA = relationship.requireSOA || assertion.Metric == "soa_margin"
		}
		positiveFault, negativeFault := false, false
		for _, operatingCase := range requirement.Requirements.OperatingCases {
			for _, condition := range operatingCase.Conditions {
				if condition.Axis == "load_current" && condition.Target == outputID && condition.Min < 0 && condition.Max > 0 {
					relationship.bidirectional = true
				}
			}
			for _, event := range operatingCase.Events {
				if event.Kind != "short_circuit" || event.Target != outputID {
					continue
				}
				positiveFault = positiveFault || event.Applied > 0
				negativeFault = negativeFault || event.Applied < 0
			}
		}
		relationship.bidirectional = relationship.bidirectional || positiveFault && negativeFault
		result = append(result, relationship)
	}
	slices.SortFunc(result, func(left, right regulatedVoltageRelationship) int {
		return cmp.Or(
			cmp.Compare(left.command, right.command),
			cmp.Compare(left.output, right.output),
			cmp.Compare(left.gain, right.gain),
		)
	})
	return result
}

func topologyProtectedVoltageRelationshipSeeds(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	representatives []PrimitiveCandidate,
	inventoryByKey map[string]PrimitiveCandidate,
	limits GraphLimits,
	policy Policy,
	initial topologySearchState,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	relationships := regulatedVoltageRelationships(requirement)
	if len(relationships) == 0 {
		return nil, Consumption{}, map[string][]string{}
	}
	resistor := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", 10_000)
	inputResistor := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", 1_000)
	inputCapacitor := topologyPrimitiveClosestValue(inventory.Primitives, "capacitor", 1e-6)
	settlingLimitS := math.Inf(1)
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "settling_time" && assertion.Max != nil && *assertion.Max > 0 {
			settlingLimitS = math.Min(settlingLimitS, *assertion.Max)
		}
	}
	if finite(settlingLimitS) {
		inputResistance := seedPrimitiveValue(inputResistor)
		tolerancePercent, toleranceFound := primitiveTolerancePercent(inputResistor, "resistance")
		if inputResistance == nil || !toleranceFound {
			inputCapacitor = PrimitiveCandidate{}
		} else {
			maximumCapacitance := settlingLimitS /
				(5 * *inputResistance * (1 + tolerancePercent/100))
			inputCapacitor = topologyPrimitiveAtMostValue(
				inventory.Primitives, "capacitor", "capacitance", maximumCapacitance,
			)
		}
	}
	outputCapacitor := topologyPrimitiveClosestValue(inventory.Primitives, "capacitor", 10e-6)
	compensationCapacitor := topologyPrimitiveClosestValue(inventory.Primitives, "capacitor", 1e-9)
	if resistor.Key == "" || inputResistor.Key == "" ||
		inputCapacitor.Key == "" || outputCapacitor.Key == "" ||
		compensationCapacitor.Key == "" {
		return nil, Consumption{}, map[string][]string{
			"voltage_relationship_primitives_unavailable": {"reviewed resistor and capacitor values are required for feedback, compensation, startup, and endpoint access"},
		}
	}
	supplies := topologyNodesByRole(initial.graph, "supply")
	references := topologyNodesByRole(initial.graph, "reference")
	if len(supplies) != 1 || len(references) != 1 {
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
		if relationship.minimumHeadroomV < protectedVoltageCurrentSenseDropV+protectedVoltagePassJunctionDropV {
			rejections["voltage_relationship_dropout"] = append(
				rejections["voltage_relationship_dropout"],
				fmt.Sprintf("%s has only %.6g V of minimum input/output headroom", relationship.output, relationship.minimumHeadroomV),
			)
			continue
		}
		sourcePass, sourcePassCount := selectProtectedVoltagePassPrimitive(
			requirement, inventory, "npn_bjt", relationship,
		)
		sinkPass := PrimitiveCandidate{}
		sinkPassCount := 0
		if relationship.bidirectional {
			sinkPass, sinkPassCount = selectProtectedVoltagePassPrimitive(
				requirement, inventory, "pnp_bjt", relationship,
			)
		}
		if sourcePass.Key == "" || sourcePassCount == 0 ||
			(relationship.bidirectional && (sinkPass.Key == "" || sinkPassCount == 0)) {
			rejections["voltage_relationship_primitives_unavailable"] = append(
				rejections["voltage_relationship_primitives_unavailable"],
				relationship.output+": reviewed source/sink pass devices are required",
			)
			continue
		}
		maximumPassBranchCurrentA := relationship.maximumRatedCurrentA / float64(sourcePassCount)
		if relationship.bidirectional {
			maximumPassBranchCurrentA = math.Max(
				maximumPassBranchCurrentA,
				relationship.maximumRatedCurrentA/float64(sinkPassCount),
			)
		}
		passBallast := topologyPrimitiveAtMostValue(
			inventory.Primitives, "resistor", "resistance",
			protectedVoltageBallastDropV/maximumPassBranchCurrentA,
		)
		if passBallast.Key == "" {
			rejections["voltage_relationship_ballast"] = append(
				rejections["voltage_relationship_ballast"],
				relationship.output+": no reviewed resistor keeps per-device current-sharing ballast inside the dropout budget",
			)
			continue
		}
		currentLimit := relationship.currentLimitA
		if currentLimit <= 0 {
			if port, found := requirementPort(requirement, relationship.output); found && port.Electrical.MaxCurrentA != nil {
				currentLimit = *port.Electrical.MaxCurrentA * 1.25
			}
		}
		if currentLimit <= 0 || !finite(currentLimit) {
			rejections["voltage_relationship_current_limit"] = append(
				rejections["voltage_relationship_current_limit"],
				relationship.output+": bounded output-current evidence is required",
			)
			continue
		}
		minimumSenseResistance := protectedVoltageCurrentSenseDropV / currentLimit
		maximumSenseResistance := math.Inf(1)
		if relationship.maximumRatedCurrentA > 0 {
			maximumSenseResistance = protectedVoltageCurrentSenseDropV / (relationship.maximumRatedCurrentA * 1.05)
		}
		currentThreshold := currentLimit * 0.9
		if relationship.maximumRatedCurrentA > 0 && relationship.maximumRatedCurrentA < currentLimit {
			currentThreshold = relationship.maximumRatedCurrentA + 0.2*(currentLimit-relationship.maximumRatedCurrentA)
		}
		senseNetwork, senseFound := currentSenseSeriesParallelCompositionWithin(
			ctx, requirement, inventory, protectedVoltageCurrentSenseDropV/currentThreshold, 5,
			minimumSenseResistance, maximumSenseResistance,
		)
		if !senseFound {
			rejections["voltage_relationship_current_limit"] = append(
				rejections["voltage_relationship_current_limit"],
				relationship.output+": no reviewed resistor composition realizes the bounded current threshold",
			)
			continue
		}
		controller := topologyProtectedVoltageControllerPrimitive(requirement, inventory, sourcePass, relationship)
		drivePath := selectProtectedVoltageDrivePath(
			requirement, inventory, controller, sourcePass, senseNetwork.effectiveResistance, passBallast,
			sourcePassCount, relationship,
		)
		if controller.Key == "" || drivePath.resistor.Key == "" || drivePath.sourceClamp.Key == "" ||
			(relationship.bidirectional && drivePath.sinkClamp.Key == "") {
			rejections["voltage_relationship_drive_headroom"] = append(
				rejections["voltage_relationship_drive_headroom"],
				relationship.output+": no reviewed controller, drive resistance, and protection clamp jointly satisfy rated-load headroom and fault current",
			)
			continue
		}
		feedbackUpperValues := []float64(nil)
		feedbackLowerValues := []float64(nil)
		feedbackFound := false
		feedbackPartCount := 0
		feedbackNominalError := math.Inf(1)
		for upperBranchCount := 1; upperBranchCount <= 3; upperBranchCount++ {
			for lowerBranchCount := 1; lowerBranchCount <= 3; lowerBranchCount++ {
				upper, lowers, found := catalogResistanceDivider(
					requirement,
					inventoryByKey,
					(relationship.gain-1)*float64(upperBranchCount),
					10_000,
					lowerBranchCount,
					relationship.maximumOutputV,
					relationship.maximumOutputV,
					0,
					0,
				)
				if !found {
					continue
				}
				upperPrimitive := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", upper)
				lowerPrimitives := make([]PrimitiveCandidate, 0, len(lowers))
				for _, lower := range lowers {
					lowerPrimitives = append(lowerPrimitives, topologyPrimitiveClosestValue(inventory.Primitives, "resistor", lower))
				}
				minimumGain, maximumGain, gainFound := protectedVoltageFeedbackGainEnvelope(
					upperPrimitive, upperBranchCount, lowerPrimitives,
				)
				upperEffective := upper / float64(upperBranchCount)
				lowerConductance := 0.0
				for _, lower := range lowers {
					lowerConductance += 1 / lower
				}
				nominalError := math.Abs(1+upperEffective*lowerConductance-relationship.gain) / relationship.gain
				partCount := upperBranchCount + lowerBranchCount
				if gainFound && minimumGain >= relationship.minimumGain && maximumGain <= relationship.maximumGain &&
					(!feedbackFound || partCount < feedbackPartCount ||
						(partCount == feedbackPartCount && nominalError < feedbackNominalError)) {
					feedbackUpperValues = make([]float64, upperBranchCount)
					for index := range feedbackUpperValues {
						feedbackUpperValues[index] = upper
					}
					feedbackLowerValues, feedbackFound = lowers, true
					feedbackPartCount, feedbackNominalError = partCount, nominalError
				}
			}
		}
		feedbackUppers := make([]PrimitiveCandidate, 0, len(feedbackUpperValues))
		for _, value := range feedbackUpperValues {
			primitive := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", value)
			if primitive.Key != "" {
				feedbackUppers = append(feedbackUppers, primitive)
			}
		}
		feedbackLowers := make([]PrimitiveCandidate, 0, len(feedbackLowerValues))
		for _, value := range feedbackLowerValues {
			primitive := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", value)
			if primitive.Key != "" {
				feedbackLowers = append(feedbackLowers, primitive)
			}
		}
		if !feedbackFound || len(feedbackUppers) != len(feedbackUpperValues) || len(feedbackLowers) != len(feedbackLowerValues) {
			rejections["voltage_relationship_feedback"] = append(
				rejections["voltage_relationship_feedback"],
				relationship.output+": catalog-backed feedback ratio is unavailable",
			)
			continue
		}
		consumption.ExpandedStates++
		state := initial
		internalCount := 5
		if relationship.bidirectional {
			internalCount += 2
		}
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
		setpoint, feedback, drive := internal[0], internal[1], internal[2]
		sourceBase, sourceEmitter := internal[3], internal[4]
		command := externalRelationshipNode(initial.graph, relationship.command)
		output := externalRelationshipNode(initial.graph, relationship.output)
		supply, reference := supplies[0], references[0]
		if command == "" || output == "" {
			continue
		}
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, inputResistor,
			topologyTwoTerminalPlacement(command, setpoint), &consumption)
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, inputCapacitor,
			topologyTwoTerminalPlacement(setpoint, reference), &consumption)
		positiveInput, negativeInput := setpoint, feedback
		if drivePath.sourceDriver.Key != "" {
			positiveInput, negativeInput = feedback, setpoint
		}
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, controller, []TerminalConnection{
			{Terminal: "IN_PLUS", Node: positiveInput},
			{Terminal: "IN_MINUS", Node: negativeInput},
			{Terminal: "OUT", Node: drive},
			{Terminal: "V_PLUS", Node: supply},
			{Terminal: "V_MINUS", Node: reference},
		}, &consumption)
		passBankComplete := true
		for passIndex := 0; passIndex < sourcePassCount; passIndex++ {
			var passEmitter string
			state, passEmitter = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
			if passEmitter == "" {
				passBankComplete = false
				break
			}
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, sourcePass, []TerminalConnection{
				{Terminal: "BASE", Node: sourceBase},
				{Terminal: "COLLECTOR", Node: supply},
				{Terminal: "EMITTER", Node: passEmitter},
			}, &consumption)
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, passBallast,
				topologyTwoTerminalPlacement(passEmitter, sourceEmitter), &consumption)
		}
		if !passBankComplete {
			continue
		}
		state = addRelationshipPrimitive(state, requirement, inventoryByKey, drivePath.sourceClamp, []TerminalConnection{
			{Terminal: "BASE", Node: sourceEmitter},
			{Terminal: "COLLECTOR", Node: sourceBase},
			{Terminal: "EMITTER", Node: output},
		}, &consumption)
		compensationDrive := drive
		if drivePath.sourceDriver.Key != "" {
			compensationDrive = sourceBase
		}
		for _, passive := range []struct {
			primitive PrimitiveCandidate
			left      string
			right     string
		}{
			{resistor, sourceBase, sourceEmitter},
			{outputCapacitor, output, reference},
			{compensationCapacitor, compensationDrive, feedback},
		} {
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, passive.primitive,
				topologyTwoTerminalPlacement(passive.left, passive.right), &consumption)
		}
		for _, feedbackUpper := range feedbackUppers {
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, feedbackUpper,
				topologyTwoTerminalPlacement(output, feedback), &consumption)
		}
		if drivePath.sourceDriver.Key == "" {
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, drivePath.resistor,
				topologyTwoTerminalPlacement(drive, sourceBase), &consumption)
		} else {
			for driverIndex := 0; driverIndex < drivePath.sourceDriverCount; driverIndex++ {
				var driverBase string
				state, driverBase = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
				if driverBase == "" {
					passBankComplete = false
					break
				}
				state = addRelationshipPrimitive(state, requirement, inventoryByKey, drivePath.sourceDriver, []TerminalConnection{
					{Terminal: "BASE", Node: driverBase},
					{Terminal: "COLLECTOR", Node: sourceBase},
					{Terminal: "EMITTER", Node: supply},
				}, &consumption)
				state = addRelationshipPrimitive(state, requirement, inventoryByKey, drivePath.resistor,
					topologyTwoTerminalPlacement(drive, driverBase), &consumption)
			}
			if !passBankComplete {
				continue
			}
		}
		for _, feedbackLower := range feedbackLowers {
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, feedbackLower,
				topologyTwoTerminalPlacement(feedback, reference), &consumption)
		}
		sourceSenseNodes := append([]string{sourceEmitter}, make([]string, len(senseNetwork.segments)-1)...)
		for index := 1; index < len(sourceSenseNodes); index++ {
			state, sourceSenseNodes[index] = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
		}
		sourceSenseNodes = append(sourceSenseNodes, output)
		for index, segment := range senseNetwork.segments {
			for _, part := range segment {
				state = addRelationshipPrimitive(state, requirement, inventoryByKey, part,
					topologyTwoTerminalPlacement(sourceSenseNodes[index], sourceSenseNodes[index+1]), &consumption)
			}
		}
		if relationship.bidirectional {
			sinkOffset := 5
			sinkBase, sinkEmitter := internal[sinkOffset], internal[sinkOffset+1]
			for passIndex := 0; passIndex < sinkPassCount; passIndex++ {
				var passEmitter string
				state, passEmitter = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
				if passEmitter == "" {
					passBankComplete = false
					break
				}
				state = addRelationshipPrimitive(state, requirement, inventoryByKey, sinkPass, []TerminalConnection{
					{Terminal: "BASE", Node: sinkBase},
					{Terminal: "COLLECTOR", Node: reference},
					{Terminal: "EMITTER", Node: passEmitter},
				}, &consumption)
				state = addRelationshipPrimitive(state, requirement, inventoryByKey, passBallast,
					topologyTwoTerminalPlacement(sinkEmitter, passEmitter), &consumption)
			}
			if !passBankComplete {
				continue
			}
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, drivePath.sinkClamp, []TerminalConnection{
				{Terminal: "BASE", Node: sinkEmitter},
				{Terminal: "COLLECTOR", Node: sinkBase},
				{Terminal: "EMITTER", Node: output},
			}, &consumption)
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, drivePath.resistor,
				topologyTwoTerminalPlacement(drive, sinkBase), &consumption)
			state = addRelationshipPrimitive(state, requirement, inventoryByKey, resistor,
				topologyTwoTerminalPlacement(sinkBase, sinkEmitter), &consumption)
			sinkSenseNodes := append([]string{output}, make([]string, len(senseNetwork.segments)-1)...)
			for index := 1; index < len(sinkSenseNodes); index++ {
				state, sinkSenseNodes[index] = addRelationshipInternalNode(state, requirement, inventoryByKey, &consumption)
			}
			sinkSenseNodes = append(sinkSenseNodes, sinkEmitter)
			for index, segment := range senseNetwork.segments {
				for _, part := range segment {
					state = addRelationshipPrimitive(state, requirement, inventoryByKey, part,
						topologyTwoTerminalPlacement(sinkSenseNodes[index], sinkSenseNodes[index+1]), &consumption)
				}
			}
		}
		if len(state.graph.Instances) > limits.MaxPrimitiveInstances || internalNodeCount(state.graph) > limits.MaxInternalNodes {
			rejections["graph_limit"] = append(rejections["graph_limit"], state.hash+": protected voltage relationship exceeds graph limits")
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
		candidate := TopologyCandidate{
			Fingerprint:  state.hash,
			TopologyHash: topologyHash,
			Score:        state.score,
			Graph:        normalized,
			Operations:   cloneGraphOperations(state.operations),
		}
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

func requirementPort(requirement Requirement, id string) (Port, bool) {
	for _, port := range requirement.Requirements.Ports {
		if port.ID == id {
			return port, true
		}
	}
	return Port{}, false
}

func topologyPrimitiveAtMostValue(
	primitives []PrimitiveCandidate,
	kind string,
	quantity string,
	maximum float64,
) PrimitiveCandidate {
	if maximum <= 0 || !finite(maximum) {
		return PrimitiveCandidate{}
	}
	best := PrimitiveCandidate{}
	bestValue := 0.0
	for _, primitive := range primitives {
		if primitive.Kind != kind {
			continue
		}
		value := seedPrimitiveValue(primitive)
		tolerancePercent, toleranceFound := primitiveTolerancePercent(primitive, quantity)
		if value == nil || *value <= 0 || !finite(*value) || !toleranceFound ||
			*value*(1+tolerancePercent/100) > maximum {
			continue
		}
		if *value > bestValue || (*value == bestValue && (best.Key == "" || primitive.Key < best.Key)) {
			best, bestValue = primitive, *value
		}
	}
	return best
}

// topologyProtectedVoltageControllerPrimitive selects the linear controller
// from behavioral obligations rather than a named regulator architecture. It
// must cover every requested trusted analysis, source the pass-device base at
// rated load, and retain enough output swing at the minimum input supply to
// drive the pass junction and current-sense element. Bidirectional rails also
// require the corresponding low-side drive range for the sink device.
func topologyProtectedVoltageControllerPrimitive(
	requirement Requirement,
	inventory PrimitiveInventory,
	driven PrimitiveCandidate,
	relationship regulatedVoltageRelationship,
) PrimitiveCandidate {
	minimumBeta := primitiveMinimumForwardBeta(driven)
	minimumSupplyV := relationship.maximumOutputV + relationship.minimumHeadroomV
	if minimumBeta <= 0 || relationship.maximumRatedCurrentA <= 0 ||
		!finite(minimumSupplyV) || minimumSupplyV <= 0 {
		return PrimitiveCandidate{}
	}
	requiredOutputCurrentA := 1.5 * relationship.maximumRatedCurrentA / minimumBeta
	requiredHighOutputV := relationship.maximumOutputV + protectedVoltageCurrentSenseDropV + protectedVoltagePassJunctionDropV
	maximumHighMarginV := minimumSupplyV - requiredHighOutputV
	maximumLowMarginV := math.Inf(1)
	if relationship.bidirectional {
		maximumLowMarginV = relationship.minimumOutputV - protectedVoltageCurrentSenseDropV - protectedVoltagePassJunctionDropV
	}
	if maximumHighMarginV < 0 || maximumLowMarginV < 0 {
		return PrimitiveCandidate{}
	}
	requiredGBW := 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "settling_time" && assertion.Max != nil && *assertion.Max > 0 {
			requiredGBW = math.Max(requiredGBW, topologyControllerGBWReserve / *assertion.Max)
		}
	}
	requiredAnalyses := requirementAnalysisSet(requirement)
	type scoredController struct {
		primitive        PrimitiveCandidate
		highMarginV      float64
		lowMarginV       float64
		outputCurrentA   float64
		quiescentCurrent float64
		gbwHz            float64
	}
	candidates := []scoredController{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "opamp" || !ratingsCoverRequirement(requirement, primitive) ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) {
			continue
		}
		outputCurrentA := 0.0
		for _, rating := range primitive.Ratings {
			if rating.Kind == "output_current" {
				outputCurrentA = math.Max(outputCurrentA, boundMaximum(rating))
			}
		}
		lowMarginV, highMarginV, gbwHz, found := primitiveOpAmpCapabilities(primitive)
		if !found || outputCurrentA < requiredOutputCurrentA ||
			highMarginV > maximumHighMarginV || lowMarginV > maximumLowMarginV ||
			gbwHz < requiredGBW {
			continue
		}
		quiescentCurrent := math.Inf(1)
		for _, model := range primitive.Models {
			for _, parameter := range model.Parameters {
				if parameter.Name == "quiescent_current_a" && parameter.Value >= 0 {
					quiescentCurrent = math.Min(quiescentCurrent, parameter.Value)
				}
			}
		}
		candidates = append(candidates, scoredController{
			primitive: primitive, highMarginV: highMarginV, lowMarginV: lowMarginV,
			outputCurrentA: outputCurrentA, quiescentCurrent: quiescentCurrent, gbwHz: gbwHz,
		})
	}
	slices.SortFunc(candidates, func(left, right scoredController) int {
		return cmp.Or(
			cmp.Compare(left.highMarginV, right.highMarginV),
			cmp.Compare(left.lowMarginV, right.lowMarginV),
			cmp.Compare(left.quiescentCurrent, right.quiescentCurrent),
			cmp.Compare(right.outputCurrentA, left.outputCurrentA),
			cmp.Compare(right.gbwHz, left.gbwHz),
			cmp.Compare(primitiveEvidencePenalty(left.primitive.Evidence), primitiveEvidencePenalty(right.primitive.Evidence)),
			comparePositiveArea(left.primitive.AreaMM2, right.primitive.AreaMM2),
			cmp.Compare(left.primitive.Key, right.primitive.Key),
		)
	})
	if len(candidates) == 0 {
		return PrimitiveCandidate{}
	}
	return candidates[0].primitive
}

// selectProtectedVoltageDrivePath jointly sizes the controller-to-pass drive
// resistance and the current-limit clamp. The upper resistance bound is the
// remaining rated-load voltage budget after the sense element, pass junction,
// and controller output swing consume their shares. The clamp imposes the
// opposing lower bound because it must safely divert the worst available drive
// current during a fault. Selecting from their intersection avoids treating
// either dropout or protection as an after-the-fact value-search concern.
func selectProtectedVoltageDrivePath(
	requirement Requirement,
	inventory PrimitiveInventory,
	controller PrimitiveCandidate,
	driven PrimitiveCandidate,
	senseResistance float64,
	passBallast PrimitiveCandidate,
	passCount int,
	relationship regulatedVoltageRelationship,
) protectedVoltageDrivePath {
	minimumBeta := primitiveMinimumForwardBeta(driven)
	lowMarginV, highMarginV, _, controllerFound := primitiveOpAmpCapabilities(controller)
	if controller.Key == "" || minimumBeta <= 0 || relationship.maximumRatedCurrentA <= 0 || passCount <= 0 ||
		!controllerFound || !finite(relationship.minimumRequiredHeadroomV) {
		return protectedVoltageDrivePath{}
	}
	worstSenseResistance := senseResistance
	if worstSenseResistance <= 0 || !finite(worstSenseResistance) {
		return protectedVoltageDrivePath{}
	}
	ballastValue := seedPrimitiveValue(passBallast)
	ballastTolerancePercent, ballastFound := primitiveTolerancePercent(passBallast, "resistance")
	if ballastValue == nil || *ballastValue <= 0 || !ballastFound ||
		ballastTolerancePercent < 0 || ballastTolerancePercent >= 100 {
		return protectedVoltageDrivePath{}
	}
	worstBallastResistance := *ballastValue * (1 + ballastTolerancePercent/100)
	baseCurrentA := relationship.maximumRatedCurrentA / minimumBeta
	controllerOutputCurrentA := primitiveMaximumRating(controller, "output_current")
	if baseCurrentA > controllerOutputCurrentA/2 {
		if relationship.bidirectional {
			return protectedVoltageDrivePath{}
		}
		return selectProtectedVoltageBufferedDrivePath(
			requirement, inventory, controller, baseCurrentA, relationship,
		)
	}
	maximumDriveDropV := relationship.minimumRequiredHeadroomV -
		relationship.maximumRatedCurrentA*worstSenseResistance -
		(relationship.maximumRatedCurrentA/float64(passCount))*worstBallastResistance - protectedVoltageDriveHeadroomV - highMarginV
	if relationship.bidirectional {
		maximumSinkDriveDropV := relationship.minimumOutputV -
			relationship.maximumRatedCurrentA*worstSenseResistance -
			(relationship.maximumRatedCurrentA/float64(passCount))*worstBallastResistance - protectedVoltageDriveHeadroomV - lowMarginV
		maximumDriveDropV = math.Min(maximumDriveDropV, maximumSinkDriveDropV)
	}
	if baseCurrentA <= 0 || maximumDriveDropV <= 0 {
		return protectedVoltageDrivePath{}
	}
	maximumDriveResistance := maximumDriveDropV / baseCurrentA
	candidates := []PrimitiveCandidate{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "resistor" {
			continue
		}
		value := seedPrimitiveValue(primitive)
		tolerancePercent, found := primitiveTolerancePercent(primitive, "resistance")
		if value == nil || *value <= 0 || !found || tolerancePercent < 0 || tolerancePercent >= 100 ||
			*value*(1+tolerancePercent/100) > maximumDriveResistance {
			continue
		}
		candidates = append(candidates, primitive)
	}
	slices.SortFunc(candidates, func(left, right PrimitiveCandidate) int {
		leftValue, rightValue := seedPrimitiveValue(left), seedPrimitiveValue(right)
		return cmp.Or(
			cmp.Compare(*rightValue, *leftValue),
			cmp.Compare(left.Key, right.Key),
		)
	})
	for _, drive := range candidates {
		value := seedPrimitiveValue(drive)
		tolerancePercent, _ := primitiveTolerancePercent(drive, "resistance")
		minimumResistance := *value * (1 - tolerancePercent/100)
		minimumClampCurrentA := 1.5 * protectedVoltageMaximumSupply(requirement) / minimumResistance
		sourceClamp := selectVoltageLimitClampPrimitive(requirement, inventory, "npn_bjt", minimumClampCurrentA)
		if sourceClamp.Key == "" {
			continue
		}
		sinkClamp := PrimitiveCandidate{}
		if relationship.bidirectional {
			sinkClamp = selectVoltageLimitClampPrimitive(requirement, inventory, "pnp_bjt", minimumClampCurrentA)
			if sinkClamp.Key == "" {
				continue
			}
		}
		return protectedVoltageDrivePath{
			resistor: drive, sourceClamp: sourceClamp, sinkClamp: sinkClamp,
		}
	}
	return protectedVoltageDrivePath{}
}

func selectProtectedVoltageBufferedDrivePath(
	requirement Requirement,
	inventory PrimitiveInventory,
	controller PrimitiveCandidate,
	requiredPassBaseCurrentA float64,
	relationship regulatedVoltageRelationship,
) protectedVoltageDrivePath {
	lowMarginV, _, _, found := primitiveOpAmpCapabilities(controller)
	minimumSupplyV := relationship.maximumOutputV + relationship.minimumHeadroomV
	maximumSupplyV := protectedVoltageMaximumSupply(requirement)
	maximumAmbientC := math.Inf(-1)
	maximumJunctionC := math.Inf(1)
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Axis == "ambient_temperature" {
				maximumAmbientC = math.Max(maximumAmbientC, condition.Max)
			}
		}
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "junction_temperature" && assertion.Max != nil {
			maximumJunctionC = math.Min(maximumJunctionC, *assertion.Max)
		}
	}
	if !found || requiredPassBaseCurrentA <= 0 || minimumSupplyV <= 0 || maximumSupplyV <= 0 ||
		!finite(maximumAmbientC) || !finite(maximumJunctionC) || maximumJunctionC <= maximumAmbientC {
		return protectedVoltageDrivePath{}
	}
	type bufferedCandidate struct {
		driver   PrimitiveCandidate
		resistor PrimitiveCandidate
		clamp    PrimitiveCandidate
		count    int
	}
	candidates := []bufferedCandidate{}
	for _, driver := range inventory.Primitives {
		if driver.Kind != "pnp_bjt" || len(driver.Models) == 0 {
			continue
		}
		beta := primitiveMinimumForwardBeta(driver)
		voltageRating := primitiveMaximumRating(driver, "collector_emitter_voltage")
		collectorRatingA := primitiveMaximumRating(driver, "collector_current")
		maximumModelTemperatureC := math.Inf(1)
		junctionToAmbientCPerW := math.Inf(1)
		for _, model := range driver.Models {
			for _, parameter := range model.Parameters {
				switch parameter.Name {
				case "max_temperature_c":
					maximumModelTemperatureC = math.Min(maximumModelTemperatureC, parameter.Value)
				case "junction_to_ambient_c_per_w":
					junctionToAmbientCPerW = math.Min(junctionToAmbientCPerW, parameter.Value)
				}
			}
		}
		allowedJunctionC := math.Min(maximumJunctionC, maximumModelTemperatureC)
		if beta <= 0 || voltageRating < maximumSupplyV || collectorRatingA <= 0 ||
			!finite(junctionToAmbientCPerW) || junctionToAmbientCPerW <= 0 || allowedJunctionC <= maximumAmbientC {
			continue
		}
		thermalCurrentA := (allowedJunctionC - maximumAmbientC) /
			(junctionToAmbientCPerW * maximumSupplyV)
		allowedCollectorCurrentA := math.Min(collectorRatingA, thermalCurrentA)
		if allowedCollectorCurrentA <= 0 {
			continue
		}
		for count := 1; count <= 8; count++ {
			minimumDriveSpanV := minimumSupplyV - protectedVoltageDriveHeadroomV - lowMarginV
			maximumDriveSpanV := maximumSupplyV - protectedVoltageDriveHeadroomV - lowMarginV
			if minimumDriveSpanV <= 0 || maximumDriveSpanV <= 0 {
				continue
			}
			minimumResistance := beta * maximumDriveSpanV / allowedCollectorCurrentA
			maximumResistance := beta * minimumDriveSpanV * float64(count) / requiredPassBaseCurrentA
			for _, resistor := range inventory.Primitives {
				if resistor.Kind != "resistor" {
					continue
				}
				value := seedPrimitiveValue(resistor)
				tolerancePercent, toleranceFound := primitiveTolerancePercent(resistor, "resistance")
				if value == nil || *value <= 0 || !toleranceFound ||
					*value*(1-tolerancePercent/100) < minimumResistance ||
					*value*(1+tolerancePercent/100) > maximumResistance {
					continue
				}
				maximumDriverCurrentA := float64(count) * beta * maximumDriveSpanV /
					(*value * (1 - tolerancePercent/100))
				clamp := selectVoltageLimitClampPrimitive(
					requirement, inventory, "npn_bjt", 1.5*maximumDriverCurrentA,
				)
				if clamp.Key != "" {
					candidates = append(candidates, bufferedCandidate{
						driver: driver, resistor: resistor, clamp: clamp, count: count,
					})
				}
			}
		}
	}
	slices.SortFunc(candidates, func(left, right bufferedCandidate) int {
		leftResistance, rightResistance := seedPrimitiveValue(left.resistor), seedPrimitiveValue(right.resistor)
		return cmp.Or(
			cmp.Compare(left.count, right.count),
			cmp.Compare(*rightResistance, *leftResistance),
			comparePositiveArea(left.driver.AreaMM2, right.driver.AreaMM2),
			cmp.Compare(left.driver.Key, right.driver.Key),
			cmp.Compare(left.resistor.Key, right.resistor.Key),
		)
	})
	if len(candidates) == 0 {
		return protectedVoltageDrivePath{}
	}
	best := candidates[0]
	return protectedVoltageDrivePath{
		resistor: best.resistor, sourceClamp: best.clamp,
		sourceDriver: best.driver, sourceDriverCount: best.count,
	}
}

func primitiveMaximumRating(primitive PrimitiveCandidate, kind string) float64 {
	maximum := 0.0
	for _, rating := range primitive.Ratings {
		if rating.Kind != kind {
			continue
		}
		value := boundMaximum(rating)
		switch rating.Unit {
		case "mA":
			value *= 1e-3
		case "uA", "µA":
			value *= 1e-6
		}
		maximum = math.Max(maximum, value)
	}
	return maximum
}

func selectVoltageLimitClampPrimitive(
	requirement Requirement,
	inventory PrimitiveInventory,
	kind string,
	minimumCollectorCurrentA float64,
) PrimitiveCandidate {
	maximumSupply := protectedVoltageMaximumSupply(requirement)
	type clampCandidate struct {
		primitive PrimitiveCandidate
		currentA  float64
	}
	candidates := []clampCandidate{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != kind || len(primitive.Models) == 0 {
			continue
		}
		voltageRating := 0.0
		currentRating := 0.0
		for _, rating := range primitive.Ratings {
			if rating.Kind == "collector_emitter_voltage" {
				voltageRating = math.Max(voltageRating, boundMaximum(rating))
			}
			if rating.Kind == "collector_current" {
				current := boundMaximum(rating)
				switch rating.Unit {
				case "mA":
					current *= 1e-3
				case "uA", "µA":
					current *= 1e-6
				}
				currentRating = math.Max(currentRating, current)
			}
		}
		if voltageRating < maximumSupply || currentRating <= 0 || currentRating < minimumCollectorCurrentA {
			continue
		}
		candidates = append(candidates, clampCandidate{primitive: primitive, currentA: currentRating})
	}
	slices.SortFunc(candidates, func(left, right clampCandidate) int {
		return cmp.Or(
			cmp.Compare(left.currentA, right.currentA),
			comparePositiveArea(left.primitive.AreaMM2, right.primitive.AreaMM2),
			cmp.Compare(primitiveEvidencePenalty(left.primitive.Evidence), primitiveEvidencePenalty(right.primitive.Evidence)),
			cmp.Compare(left.primitive.Key, right.primitive.Key),
		)
	})
	if len(candidates) == 0 {
		return PrimitiveCandidate{}
	}
	return candidates[0].primitive
}

// protectedVoltagePassCount derives a parallel linear pass bank from the
// declared current stress and the device's reviewed DC safe-operating-area
// boundary. A short circuit can persist beyond every pulse envelope, so the DC
// envelope is the only defensible sizing boundary; transient simulation still
// verifies the complete event waveform afterward.
func protectedVoltagePassCount(
	requirement Requirement,
	pass PrimitiveCandidate,
	relationship regulatedVoltageRelationship,
) int {
	if pass.Key == "" {
		return 0
	}
	if !relationship.requireSOA {
		return 1
	}
	requiredMargin := 1.0
	maximumAmbientC := math.Inf(-1)
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "soa_margin" && assertion.Min != nil && *assertion.Min > 0 {
			requiredMargin = math.Max(requiredMargin, *assertion.Min)
		}
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Axis == "ambient_temperature" {
				maximumAmbientC = math.Max(maximumAmbientC, condition.Max)
			}
		}
	}
	stressCurrentA := math.Max(relationship.maximumRatedCurrentA, relationship.currentLimitA)
	stressVoltageV := protectedVoltageMaximumSupply(requirement)
	if stressCurrentA <= 0 || stressVoltageV <= 0 {
		return 0
	}
	allowedCurrentA := math.Inf(1)
	for _, model := range pass.Models {
		maximumTemperatureC := 0.0
		for _, parameter := range model.Parameters {
			if parameter.Name == "max_temperature_c" {
				maximumTemperatureC = math.Max(maximumTemperatureC, parameter.Value)
			}
		}
		for _, envelope := range model.TransientSOA {
			if !envelope.DC {
				continue
			}
			current, found := protectedVoltageSOACurrent(envelope.Points, stressVoltageV)
			if !found || current <= 0 {
				continue
			}
			if finite(maximumAmbientC) && maximumAmbientC > envelope.CaseTemperatureC {
				denominator := maximumTemperatureC - envelope.CaseTemperatureC
				if denominator <= 0 || maximumAmbientC >= maximumTemperatureC {
					return 0
				}
				current *= (maximumTemperatureC - maximumAmbientC) / denominator
			}
			allowedCurrentA = math.Min(allowedCurrentA, current)
		}
	}
	if !finite(allowedCurrentA) || allowedCurrentA <= 0 {
		return 0
	}
	count := int(math.Ceil(requiredMargin * stressCurrentA / allowedCurrentA))
	if count < 1 {
		count = 1
	}
	return count
}

func selectProtectedVoltagePassPrimitive(
	requirement Requirement,
	inventory PrimitiveInventory,
	kind string,
	relationship regulatedVoltageRelationship,
) (PrimitiveCandidate, int) {
	requiredAnalyses := requirementAnalysisSet(requirement)
	type passCandidate struct {
		primitive PrimitiveCandidate
		count     int
		totalArea float64
	}
	candidates := []passCandidate{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != kind || !primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			!ratingsCoverRequirement(requirement, primitive) ||
			(relationship.requireThermal && !primitiveHasThermalEvidence(primitive)) ||
			(relationship.requireSOA && !primitiveHasSOAEvidence(primitive)) {
			continue
		}
		count := protectedVoltagePassCount(requirement, primitive, relationship)
		if count <= 0 {
			continue
		}
		area := primitive.AreaMM2
		if area <= 0 || !finite(area) {
			area = math.Inf(1)
		} else {
			area *= float64(count)
		}
		candidates = append(candidates, passCandidate{primitive: primitive, count: count, totalArea: area})
	}
	slices.SortFunc(candidates, func(left, right passCandidate) int {
		return cmp.Or(
			cmp.Compare(left.count, right.count),
			cmp.Compare(left.totalArea, right.totalArea),
			compareRepresentativePrimitives(left.primitive, right.primitive, requiredAnalyses),
		)
	})
	if len(candidates) == 0 {
		return PrimitiveCandidate{}, 0
	}
	return candidates[0].primitive, candidates[0].count
}

func protectedVoltageFeedbackGainEnvelope(
	upper PrimitiveCandidate,
	upperCount int,
	lowers []PrimitiveCandidate,
) (float64, float64, bool) {
	upperValue := seedPrimitiveValue(upper)
	upperTolerance, upperFound := primitiveTolerancePercent(upper, "resistance")
	if upperValue == nil || *upperValue <= 0 || upperCount <= 0 || !upperFound ||
		upperTolerance < 0 || upperTolerance >= 100 || len(lowers) == 0 {
		return 0, 0, false
	}
	upperMinimum := *upperValue * (1 - upperTolerance/100) / float64(upperCount)
	upperMaximum := *upperValue * (1 + upperTolerance/100) / float64(upperCount)
	minimumConductance, maximumConductance := 0.0, 0.0
	for _, lower := range lowers {
		value := seedPrimitiveValue(lower)
		tolerance, found := primitiveTolerancePercent(lower, "resistance")
		if value == nil || *value <= 0 || !found || tolerance < 0 || tolerance >= 100 {
			return 0, 0, false
		}
		minimumConductance += 1 / (*value * (1 + tolerance/100))
		maximumConductance += 1 / (*value * (1 - tolerance/100))
	}
	return 1 + upperMinimum*minimumConductance, 1 + upperMaximum*maximumConductance, true
}

func protectedVoltageSOACurrent(points []simmodel.TransientSOAPoint, voltageV float64) (float64, bool) {
	if len(points) < 2 || voltageV <= 0 || voltageV > points[len(points)-1].VoltageV {
		return 0, false
	}
	if voltageV <= points[0].VoltageV {
		return points[0].CurrentA, points[0].CurrentA > 0
	}
	for index := 1; index < len(points); index++ {
		if voltageV > points[index].VoltageV {
			continue
		}
		left, right := points[index-1], points[index]
		if left.VoltageV <= 0 || right.VoltageV <= left.VoltageV ||
			left.CurrentA <= 0 || right.CurrentA <= 0 {
			return 0, false
		}
		fraction := (math.Log(voltageV) - math.Log(left.VoltageV)) /
			(math.Log(right.VoltageV) - math.Log(left.VoltageV))
		current := math.Exp(math.Log(left.CurrentA) + fraction*(math.Log(right.CurrentA)-math.Log(left.CurrentA)))
		return current, current > 0 && finite(current)
	}
	return 0, false
}

func protectedVoltageMaximumSupply(requirement Requirement) float64 {
	maximumSupply := 0.0
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind == "supply" && domain.MaxVoltageV != nil {
			maximumSupply = math.Max(maximumSupply, *domain.MaxVoltageV)
		}
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Axis == "supply_voltage" {
				maximumSupply = math.Max(maximumSupply, condition.Max)
			}
		}
		for _, event := range operatingCase.Events {
			if event.Kind == "startup" {
				maximumSupply = math.Max(maximumSupply, math.Max(event.Initial, event.Applied))
			}
		}
	}
	return maximumSupply
}
