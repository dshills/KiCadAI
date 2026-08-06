package opentopologysynthesis

import (
	"cmp"
	"math"
	"slices"

	"kicadai/internal/simmodel"
)

type topologyRegulatedLoadRailPlan struct {
	converter      PrimitiveCandidate
	ballast        [2]PrimitiveCandidate
	ballastValueSI [2]float64
}

// topologyRegulatedLoadRail selects a reviewed regulated converter and a
// catalog-rated series ballast only when the complete input, output, load,
// current, source-power, and thermal envelopes overlap. The direct supply path
// remains a separate topology candidate; this adapter is an additional
// compositional option.
func topologyRegulatedLoadRail(
	requirement Requirement,
	graph CandidateGraph,
	output string,
	inputSupply string,
	inventory PrimitiveInventory,
) (topologyRegulatedLoadRailPlan, bool) {
	outputSemanticID := ""
	for _, node := range graph.Nodes {
		if node.ID == output && node.Scope == "external" && node.SemanticKind == "port" {
			outputSemanticID = node.SemanticID
			break
		}
	}
	if outputSemanticID == "" {
		return topologyRegulatedLoadRailPlan{}, false
	}
	loadMinimum, loadMaximum, loadFound := topologyLoadResistanceEnvelope(
		requirement, outputSemanticID,
	)
	currentMinimum, currentMaximum, currentFound := topologyOutputCurrentEnvelope(
		requirement, outputSemanticID,
	)
	inputMinimum, inputMaximum, inputFound := topologyDeclaredNodeVoltageRange(
		requirement, graph, inputSupply,
	)
	if !loadFound || !currentFound || !inputFound ||
		loadMinimum <= 0 || loadMaximum < loadMinimum ||
		currentMinimum <= 0 || currentMaximum < currentMinimum ||
		inputMaximum < inputMinimum {
		return topologyRegulatedLoadRailPlan{}, false
	}
	outputVoltageMaximum := math.Inf(1)
	for _, port := range requirement.Requirements.Ports {
		if port.ID == outputSemanticID && port.Electrical.MaxVoltageV != nil {
			outputVoltageMaximum = *port.Electrical.MaxVoltageV
			break
		}
	}
	inputCurrentMaximum := topologyNodeDomainMaximumCurrent(requirement, graph, inputSupply)
	ambientMaximum := topologyMaximumAmbientTemperature(requirement)
	requiresThermalBound := topologyRequiresJunctionTemperatureBound(requirement)
	type candidate struct {
		primitive       PrimitiveCandidate
		ballast         [2]PrimitiveCandidate
		ballastValueSI  [2]float64
		outputVoltageV  float64
		currentHeadroom float64
		ballastError    float64
	}
	candidates := []candidate{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "isolated_converter" ||
			!primitiveHasModel(primitive, simmodel.PrimitiveProtectedIsolatedConverterV1) {
			continue
		}
		modelInputMinimum := primitiveModelParameter(
			primitive, simmodel.PrimitiveProtectedIsolatedConverterV1, "input_min_v",
		)
		modelInputMaximum := primitiveModelParameter(
			primitive, simmodel.PrimitiveProtectedIsolatedConverterV1, "input_max_v",
		)
		outputVoltage := primitiveModelParameter(
			primitive, simmodel.PrimitiveProtectedIsolatedConverterV1, "output_voltage_v",
		)
		maximumOutputCurrent := primitiveModelParameter(
			primitive, simmodel.PrimitiveProtectedIsolatedConverterV1, "max_output_current_a",
		)
		efficiency := primitiveModelParameter(
			primitive, simmodel.PrimitiveProtectedIsolatedConverterV1, "efficiency_ratio",
		)
		if modelInputMinimum <= 0 || modelInputMaximum < modelInputMinimum ||
			outputVoltage <= 0 || maximumOutputCurrent <= 0 || efficiency <= 0 || efficiency > 1 ||
			modelInputMinimum > inputMinimum || modelInputMaximum < inputMaximum ||
			outputVoltage > outputVoltageMaximum {
			continue
		}
		minimumSeriesResistance := max(
			0,
			outputVoltage/currentMaximum-loadMinimum,
			outputVoltage/maximumOutputCurrent-loadMinimum,
		)
		maximumSeriesResistance := outputVoltage/currentMinimum - loadMaximum
		if inputCurrentMaximum > 0 {
			maximumOutputPower := inputCurrentMaximum * inputMinimum * efficiency
			if maximumOutputPower > 0 {
				minimumSeriesResistance = math.Max(
					minimumSeriesResistance,
					outputVoltage*outputVoltage/maximumOutputPower-loadMinimum,
				)
			}
		}
		thermalOutputPower, thermalBounded := topologyConverterThermalOutputPower(
			requirement, primitive, ambientMaximum, efficiency,
		)
		if requiresThermalBound && !thermalBounded {
			continue
		}
		if thermalBounded {
			minimumSeriesResistance = math.Max(
				minimumSeriesResistance,
				outputVoltage*outputVoltage/thermalOutputPower-loadMinimum,
			)
		}
		if maximumSeriesResistance <= 0 ||
			minimumSeriesResistance > maximumSeriesResistance {
			continue
		}
		targetSeriesResistance := .5 * (math.Max(0, minimumSeriesResistance) + maximumSeriesResistance)
		ballast, ballastValues, ballastFound := topologyRegulatedRailBallastPair(
			requirement,
			inventory,
			outputVoltage,
			loadMinimum,
			math.Max(0, minimumSeriesResistance),
			maximumSeriesResistance,
			targetSeriesResistance,
		)
		if !ballastFound {
			continue
		}
		seriesResistance := ballastValues[0] + ballastValues[1]
		maximumLoadCurrent := outputVoltage / (loadMinimum + seriesResistance)
		candidates = append(candidates, candidate{
			primitive: primitive, ballast: ballast, ballastValueSI: ballastValues,
			outputVoltageV:  outputVoltage,
			currentHeadroom: maximumOutputCurrent - maximumLoadCurrent,
			ballastError:    math.Abs(seriesResistance-targetSeriesResistance) / targetSeriesResistance,
		})
	}
	if len(candidates) == 0 {
		return topologyRegulatedLoadRailPlan{}, false
	}
	slices.SortFunc(candidates, func(left, right candidate) int {
		return cmp.Or(
			cmp.Compare(left.outputVoltageV, right.outputVoltageV),
			cmp.Compare(left.ballastError, right.ballastError),
			cmp.Compare(left.currentHeadroom, right.currentHeadroom),
			cmp.Compare(
				left.primitive.AreaMM2+left.ballast[0].AreaMM2+left.ballast[1].AreaMM2,
				right.primitive.AreaMM2+right.ballast[0].AreaMM2+right.ballast[1].AreaMM2,
			),
			cmp.Compare(left.primitive.Key, right.primitive.Key),
		)
	})
	return topologyRegulatedLoadRailPlan{
		converter:      candidates[0].primitive,
		ballast:        candidates[0].ballast,
		ballastValueSI: candidates[0].ballastValueSI,
	}, true
}

func topologyNodeDomainMaximumCurrent(
	requirement Requirement,
	graph CandidateGraph,
	nodeID string,
) float64 {
	domainID := ""
	for _, node := range graph.Nodes {
		if node.ID == nodeID {
			domainID = node.Domain
			break
		}
	}
	for _, domain := range requirement.Requirements.Domains {
		if domain.ID == domainID && domain.MaxCurrentA != nil && *domain.MaxCurrentA > 0 {
			return *domain.MaxCurrentA
		}
	}
	return 0
}

func topologyMaximumAmbientTemperature(requirement Requirement) float64 {
	maximum := math.Inf(-1)
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Axis == "ambient_temperature" && finite(condition.Max) {
				maximum = math.Max(maximum, condition.Max)
			}
		}
	}
	return maximum
}

func topologyRequiresJunctionTemperatureBound(requirement Requirement) bool {
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "junction_temperature" && assertion.Max != nil &&
			finite(*assertion.Max) {
			return true
		}
	}
	return false
}

func topologyConverterThermalOutputPower(
	requirement Requirement,
	primitive PrimitiveCandidate,
	ambientMaximum float64,
	efficiency float64,
) (float64, bool) {
	if !finite(ambientMaximum) || efficiency <= 0 || efficiency >= 1 {
		return 0, false
	}
	maximumTemperature := primitiveModelParameter(
		primitive, simmodel.PrimitiveProtectedIsolatedConverterV1, "max_temperature_c",
	)
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "junction_temperature" && assertion.Max != nil &&
			*assertion.Max > 0 &&
			(maximumTemperature <= 0 || *assertion.Max < maximumTemperature) {
			maximumTemperature = *assertion.Max
		}
	}
	thermalResistance := primitiveModelParameter(
		primitive,
		simmodel.PrimitiveProtectedIsolatedConverterV1,
		"junction_to_ambient_c_per_w",
	)
	if maximumTemperature <= ambientMaximum || thermalResistance <= 0 {
		return 0, false
	}
	maximumDissipation := (maximumTemperature - ambientMaximum) / thermalResistance
	conversionLossRatio := 1/efficiency - 1
	if maximumDissipation <= 0 || conversionLossRatio <= 0 {
		return 0, false
	}
	return maximumDissipation / conversionLossRatio, true
}

func topologyRegulatedRailBallastPair(
	requirement Requirement,
	inventory PrimitiveInventory,
	outputVoltage float64,
	loadMinimum float64,
	seriesMinimum float64,
	seriesMaximum float64,
	target float64,
) ([2]PrimitiveCandidate, [2]float64, bool) {
	type resistor struct {
		primitive PrimitiveCandidate
		value     float64
		powerW    float64
	}
	resistors := []resistor{}
	requiredAnalyses := requirementAnalysisSet(requirement)
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "resistor" || primitive.ValueDomain == nil ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			!ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		minimum, maximum, found := effectiveValueRange(*primitive.ValueDomain)
		power := primitiveMaximumRating(primitive, "power")
		if !found || minimum != maximum || minimum <= 0 || power <= 0 {
			continue
		}
		resistors = append(resistors, resistor{primitive: primitive, value: minimum, powerW: power})
	}
	type pair struct {
		resistors [2]resistor
		error     float64
		area      float64
		key       string
	}
	pairs := []pair{}
	for leftIndex, left := range resistors {
		for rightIndex := leftIndex; rightIndex < len(resistors); rightIndex++ {
			right := resistors[rightIndex]
			total := left.value + right.value
			if total < seriesMinimum || total > seriesMaximum {
				continue
			}
			current := outputVoltage / (loadMinimum + total)
			if current*current*left.value > left.powerW ||
				current*current*right.value > right.powerW {
				continue
			}
			pairs = append(pairs, pair{
				resistors: [2]resistor{left, right},
				error:     math.Abs(total-target) / target,
				area:      left.primitive.AreaMM2 + right.primitive.AreaMM2,
				key:       left.primitive.Key + "|" + right.primitive.Key,
			})
		}
	}
	if len(pairs) == 0 {
		return [2]PrimitiveCandidate{}, [2]float64{}, false
	}
	slices.SortFunc(pairs, func(left, right pair) int {
		return cmp.Or(
			cmp.Compare(left.error, right.error),
			cmp.Compare(left.area, right.area),
			cmp.Compare(left.key, right.key),
		)
	})
	return [2]PrimitiveCandidate{pairs[0].resistors[0].primitive, pairs[0].resistors[1].primitive},
		[2]float64{pairs[0].resistors[0].value, pairs[0].resistors[1].value}, true
}

func deriveRegulatedLoadRailTopologyScales(
	graph CandidateGraph,
	instance GraphInstance,
) []AnalyticScale {
	if instance.Kind != "resistor" || instance.ValueSI == nil || *instance.ValueSI <= 0 ||
		len(instance.Terminals) != 2 {
		return nil
	}
	converterOutputs := map[string]bool{}
	loadRails := map[string]bool{}
	for _, candidate := range graph.Instances {
		terminals := topologyTerminalNodes(candidate)
		switch candidate.Kind {
		case "isolated_converter":
			converterOutputs[terminals["VOUT_PLUS"]] = true
		case "signal_diode":
			loadRails[terminals["CATHODE"]] = true
		}
	}
	left, right := instance.Terminals[0].Node, instance.Terminals[1].Node
	for _, other := range graph.Instances {
		if other.ID == instance.ID || other.Kind != "resistor" || len(other.Terminals) != 2 {
			continue
		}
		otherLeft, otherRight := other.Terminals[0].Node, other.Terminals[1].Node
		seriesPath := converterOutputs[left] && right == otherLeft && loadRails[otherRight] ||
			converterOutputs[left] && right == otherRight && loadRails[otherLeft] ||
			converterOutputs[right] && left == otherLeft && loadRails[otherRight] ||
			converterOutputs[right] && left == otherRight && loadRails[otherLeft] ||
			loadRails[left] && right == otherLeft && converterOutputs[otherRight] ||
			loadRails[left] && right == otherRight && converterOutputs[otherLeft] ||
			loadRails[right] && left == otherLeft && converterOutputs[otherRight] ||
			loadRails[right] && left == otherRight && converterOutputs[otherLeft]
		if !seriesPath {
			continue
		}
		return []AnalyticScale{{
			ID:         "topology:regulated_load_rail:" + instance.ID,
			Kind:       "resistance",
			ValueSI:    *instance.ValueSI,
			Unit:       "ohm",
			Derivation: "catalog-rated series ballast derived from overlapping load-current, source-power, and converter thermal envelopes",
			SourceKind: "candidate_topology",
			SourceID:   instance.ID,
			Priority:   1,
		}}
	}
	return nil
}

func topologySwitchedLoadEnvelopeGap(
	requirement Requirement,
	graph CandidateGraph,
	inventory map[string]PrimitiveCandidate,
) int {
	for _, outputNode := range graph.Nodes {
		if outputNode.Scope != "external" || outputNode.SemanticKind != "port" {
			continue
		}
		loadMinimum, loadMaximum, loadFound := topologyLoadResistanceEnvelope(
			requirement, outputNode.SemanticID,
		)
		currentMinimum, currentMaximum, currentFound := topologyOutputCurrentEnvelope(
			requirement, outputNode.SemanticID,
		)
		if !loadFound || !currentFound {
			continue
		}
		reference := ""
		for _, instance := range graph.Instances {
			if instance.Kind != "n_channel_mosfet" {
				continue
			}
			terminals := topologyTerminalNodes(instance)
			if terminals["DRAIN"] == outputNode.ID {
				reference = terminals["SOURCE"]
				break
			}
		}
		rail, found := topologySwitchedLoadRail(graph, outputNode.ID, reference)
		if !found {
			continue
		}
		voltageMinimum, voltageMaximum, seriesResistance, bounded := topologyLoadRailEnvelope(
			requirement, graph, inventory, rail,
		)
		if !bounded {
			continue
		}
		minimumLoadCurrent := voltageMinimum / (loadMaximum + seriesResistance)
		maximumLoadCurrent := voltageMaximum / (loadMinimum + seriesResistance)
		if minimumLoadCurrent < currentMinimum || maximumLoadCurrent > currentMaximum {
			return 1
		}
	}
	return 0
}

func topologyLoadRailEnvelope(
	requirement Requirement,
	graph CandidateGraph,
	inventory map[string]PrimitiveCandidate,
	rail string,
) (float64, float64, float64, bool) {
	if minimum, maximum, found := topologyDeclaredNodeVoltageRange(requirement, graph, rail); found {
		return minimum, maximum, 0, true
	}
	type edge struct {
		left, right string
		resistance  float64
	}
	edges := []edge{}
	for _, instance := range graph.Instances {
		if instance.Kind != "resistor" || instance.ValueSI == nil || *instance.ValueSI <= 0 ||
			len(instance.Terminals) != 2 {
			continue
		}
		edges = append(edges, edge{
			left: instance.Terminals[0].Node, right: instance.Terminals[1].Node,
			resistance: *instance.ValueSI,
		})
	}
	bestVoltage, bestResistance := 0.0, math.Inf(1)
	for _, instance := range graph.Instances {
		if instance.Kind != "isolated_converter" {
			continue
		}
		primitive := inventory[instance.PrimitiveKey]
		voltage := primitiveModelParameter(
			primitive, simmodel.PrimitiveProtectedIsolatedConverterV1, "output_voltage_v",
		)
		start := topologyTerminalNodes(instance)["VOUT_PLUS"]
		if voltage <= 0 || start == "" {
			continue
		}
		distance := map[string]float64{start: 0}
		for range graph.Nodes {
			changed := false
			for _, edge := range edges {
				if left, found := distance[edge.left]; found {
					candidate := left + edge.resistance
					if right, exists := distance[edge.right]; !exists || candidate < right {
						distance[edge.right] = candidate
						changed = true
					}
				}
				if right, found := distance[edge.right]; found {
					candidate := right + edge.resistance
					if left, exists := distance[edge.left]; !exists || candidate < left {
						distance[edge.left] = candidate
						changed = true
					}
				}
			}
			if !changed {
				break
			}
		}
		if resistance, found := distance[rail]; found && resistance < bestResistance {
			bestVoltage, bestResistance = voltage, resistance
		}
	}
	return bestVoltage, bestVoltage, bestResistance, bestVoltage > 0 && finite(bestResistance)
}

func topologyLoadResistanceEnvelope(
	requirement Requirement,
	outputSemanticID string,
) (float64, float64, bool) {
	minimum, maximum := math.Inf(1), 0.0
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Axis != "load_resistance" || condition.Target != outputSemanticID ||
				condition.Min <= 0 || condition.Max < condition.Min || !finite(condition.Max) {
				continue
			}
			minimum = math.Min(minimum, condition.Min)
			maximum = math.Max(maximum, condition.Max)
		}
	}
	return minimum, maximum, finite(minimum) && maximum >= minimum
}

func topologyOutputCurrentEnvelope(
	requirement Requirement,
	outputSemanticID string,
) (float64, float64, bool) {
	minimum, maximum := 0.0, math.Inf(1)
	foundMinimum, foundMaximum := false, false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "output_current" || assertion.Observation.Kind != "port" ||
			assertion.Observation.ID != outputSemanticID {
			continue
		}
		if assertion.Min != nil && finite(*assertion.Min) {
			minimum = math.Max(minimum, *assertion.Min)
			foundMinimum = true
		}
		if assertion.Max != nil && finite(*assertion.Max) {
			maximum = math.Min(maximum, *assertion.Max)
			foundMaximum = true
		}
	}
	return minimum, maximum, foundMinimum && foundMaximum && maximum >= minimum
}

func topologySwitchedLoadRail(graph CandidateGraph, output, reference string) (string, bool) {
	controlled := false
	for _, instance := range graph.Instances {
		if instance.Kind != "n_channel_mosfet" {
			continue
		}
		terminals := topologyTerminalNodes(instance)
		if terminals["DRAIN"] == output && terminals["SOURCE"] == reference {
			controlled = true
			break
		}
	}
	if !controlled {
		return "", false
	}
	rail := ""
	for _, instance := range graph.Instances {
		if instance.Kind != "signal_diode" {
			continue
		}
		terminals := topologyTerminalNodes(instance)
		if terminals["ANODE"] != output || terminals["CATHODE"] == "" {
			continue
		}
		if rail != "" && rail != terminals["CATHODE"] {
			return "", false
		}
		rail = terminals["CATHODE"]
	}
	return rail, rail != ""
}
