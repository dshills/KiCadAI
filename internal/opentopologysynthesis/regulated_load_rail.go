package opentopologysynthesis

import (
	"cmp"
	"math"
	"slices"

	"kicadai/internal/simmodel"
)

type topologyRegulatedLoadRailPlan struct {
	converter      PrimitiveCandidate
	seriesCount    int
	parallelCount  int
	outputVoltageV float64
	ballast        []PrimitiveCandidate
	ballastValueSI []float64
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
		seriesCount     int
		parallelCount   int
		ballast         []PrimitiveCandidate
		ballastValueSI  []float64
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
		thermalOutputPower, thermalBounded := topologyConverterThermalOutputPower(
			requirement, primitive, ambientMaximum, efficiency,
		)
		if requiresThermalBound && !thermalBounded {
			continue
		}
		for seriesCount := 1; seriesCount <= 4; seriesCount++ {
			combinedVoltage := outputVoltage * float64(seriesCount)
			if combinedVoltage > outputVoltageMaximum {
				break
			}
			for parallelCount := 1; parallelCount <= 4; parallelCount++ {
				parallel := float64(parallelCount)
				minimumSeriesResistance := max(
					0,
					parallel*(combinedVoltage/currentMaximum-loadMinimum),
					combinedVoltage/maximumOutputCurrent-parallel*loadMinimum,
				)
				maximumSeriesResistance := parallel * (combinedVoltage/currentMinimum - loadMaximum)
				if inputCurrentMaximum > 0 {
					maximumOutputPower := inputCurrentMaximum * inputMinimum * efficiency
					if maximumOutputPower > 0 {
						minimumSeriesResistance = math.Max(
							minimumSeriesResistance,
							parallel*(combinedVoltage*combinedVoltage/maximumOutputPower-loadMinimum),
						)
					}
				}
				if thermalBounded {
					maximumOutputPower := thermalOutputPower * float64(seriesCount*parallelCount)
					minimumSeriesResistance = math.Max(
						minimumSeriesResistance,
						parallel*(combinedVoltage*combinedVoltage/maximumOutputPower-loadMinimum),
					)
				}
				if maximumSeriesResistance <= 0 ||
					minimumSeriesResistance > maximumSeriesResistance {
					continue
				}
				targetSeriesResistance := .5 * (math.Max(0, minimumSeriesResistance) + maximumSeriesResistance)
				ballast, ballastValues, ballastFound := topologyRegulatedRailBallastNetwork(
					requirement,
					inventory,
					combinedVoltage,
					parallel*loadMinimum,
					math.Max(0, minimumSeriesResistance),
					maximumSeriesResistance,
					targetSeriesResistance,
				)
				if !ballastFound {
					continue
				}
				seriesResistance := 0.0
				for _, value := range ballastValues {
					seriesResistance += value
				}
				maximumLoadCurrent := combinedVoltage /
					(loadMinimum + seriesResistance/parallel)
				candidates = append(candidates, candidate{
					primitive: primitive, seriesCount: seriesCount, parallelCount: parallelCount,
					ballast: ballast, ballastValueSI: ballastValues,
					outputVoltageV:  combinedVoltage,
					currentHeadroom: maximumOutputCurrent - maximumLoadCurrent/parallel,
					ballastError:    math.Abs(seriesResistance-targetSeriesResistance) / targetSeriesResistance,
				})
			}
		}
	}
	if len(candidates) == 0 {
		return topologyRegulatedLoadRailPlan{}, false
	}
	slices.SortFunc(candidates, func(left, right candidate) int {
		leftComponentCount := left.seriesCount*left.parallelCount + left.parallelCount*len(left.ballast)
		rightComponentCount := right.seriesCount*right.parallelCount + right.parallelCount*len(right.ballast)
		leftArea := float64(left.seriesCount*left.parallelCount) * left.primitive.AreaMM2
		rightArea := float64(right.seriesCount*right.parallelCount) * right.primitive.AreaMM2
		for _, primitive := range left.ballast {
			leftArea += float64(left.parallelCount) * primitive.AreaMM2
		}
		for _, primitive := range right.ballast {
			rightArea += float64(right.parallelCount) * primitive.AreaMM2
		}
		return cmp.Or(
			cmp.Compare(left.outputVoltageV, right.outputVoltageV),
			cmp.Compare(leftComponentCount, rightComponentCount),
			cmp.Compare(left.ballastError, right.ballastError),
			cmp.Compare(left.currentHeadroom, right.currentHeadroom),
			cmp.Compare(leftArea, rightArea),
			cmp.Compare(left.primitive.Key, right.primitive.Key),
		)
	})
	return topologyRegulatedLoadRailPlan{
		converter:      candidates[0].primitive,
		seriesCount:    candidates[0].seriesCount,
		parallelCount:  candidates[0].parallelCount,
		outputVoltageV: candidates[0].outputVoltageV,
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

func topologyRegulatedRailBallastNetwork(
	requirement Requirement,
	inventory PrimitiveInventory,
	outputVoltage float64,
	loadMinimum float64,
	seriesMinimum float64,
	seriesMaximum float64,
	target float64,
) ([]PrimitiveCandidate, []float64, bool) {
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
	type network struct {
		resistors []resistor
		error     float64
		area      float64
		key       string
	}
	networks := []network{}
	for _, candidate := range resistors {
		if candidate.value < seriesMinimum || candidate.value > seriesMaximum {
			continue
		}
		current := outputVoltage / (loadMinimum + candidate.value)
		if current*current*candidate.value > candidate.powerW {
			continue
		}
		networks = append(networks, network{
			resistors: []resistor{candidate},
			error:     math.Abs(candidate.value-target) / target,
			area:      candidate.primitive.AreaMM2,
			key:       candidate.primitive.Key,
		})
	}
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
			networks = append(networks, network{
				resistors: []resistor{left, right},
				error:     math.Abs(total-target) / target,
				area:      left.primitive.AreaMM2 + right.primitive.AreaMM2,
				key:       left.primitive.Key + "|" + right.primitive.Key,
			})
		}
	}
	if len(networks) == 0 {
		return nil, nil, false
	}
	slices.SortFunc(networks, func(left, right network) int {
		return cmp.Or(
			cmp.Compare(len(left.resistors), len(right.resistors)),
			cmp.Compare(left.error, right.error),
			cmp.Compare(left.area, right.area),
			cmp.Compare(left.key, right.key),
		)
	})
	selected := networks[0]
	primitives := make([]PrimitiveCandidate, len(selected.resistors))
	values := make([]float64, len(selected.resistors))
	for index, resistor := range selected.resistors {
		primitives[index] = resistor.primitive
		values[index] = resistor.value
	}
	return primitives, values, true
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
		case "p_channel_mosfet":
			loadRails[terminals["SOURCE"]] = true
		case "signal_diode":
			loadRails[terminals["CATHODE"]] = true
		}
	}
	left, right := instance.Terminals[0].Node, instance.Terminals[1].Node
	directPath := converterOutputs[left] && loadRails[right] ||
		converterOutputs[right] && loadRails[left]
	if directPath {
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
		referenceNodes := topologyNodesByRole(graph, "reference")
		if len(referenceNodes) == 1 &&
			topologyGraphHasLowSideCurrentRegulation(graph, outputNode.ID, referenceNodes[0]) {
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

func topologyGraphHasLowSideCurrentRegulation(
	graph CandidateGraph,
	output string,
	reference string,
) bool {
	if output == "" || reference == "" {
		return false
	}
	for _, instance := range graph.Instances {
		if instance.Kind != "n_channel_mosfet" {
			continue
		}
		terminals := topologyTerminalNodes(instance)
		sense := terminals["SOURCE"]
		if terminals["DRAIN"] != output || sense == "" || sense == reference {
			continue
		}
		hasSenseReturn := false
		for _, candidate := range graph.Instances {
			if candidate.Kind != "resistor" || len(candidate.Terminals) != 2 {
				continue
			}
			left, right := candidate.Terminals[0].Node, candidate.Terminals[1].Node
			if left == sense && right == reference || right == sense && left == reference {
				hasSenseReturn = true
				break
			}
		}
		if !hasSenseReturn {
			continue
		}
		gate := terminals["GATE"]
		for _, controller := range graph.Instances {
			if controller.Kind != "opamp" {
				continue
			}
			controllerTerminals := topologyTerminalNodes(controller)
			feedback := controllerTerminals["IN_MINUS"]
			feedbackObservesSense := feedback == sense
			feedbackHasReferenceBias := feedback == sense
			if feedback != "" && feedback != sense {
				for _, candidate := range graph.Instances {
					if candidate.Kind != "resistor" || len(candidate.Terminals) != 2 {
						continue
					}
					left, right := candidate.Terminals[0].Node, candidate.Terminals[1].Node
					feedbackObservesSense = feedbackObservesSense ||
						(left == feedback && right == sense || right == feedback && left == sense)
					feedbackHasReferenceBias = feedbackHasReferenceBias ||
						(left == feedback && right != sense || right == feedback && left != sense)
				}
			}
			if controllerTerminals["OUT"] == gate &&
				feedbackObservesSense && feedbackHasReferenceBias &&
				controllerTerminals["IN_PLUS"] != "" &&
				controllerTerminals["IN_PLUS"] != sense {
				return true
			}
		}
	}
	return false
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
		from, to   string
		voltage    float64
		resistance float64
	}
	edges := []edge{}
	for _, instance := range graph.Instances {
		switch instance.Kind {
		case "resistor":
			if instance.ValueSI == nil || *instance.ValueSI <= 0 || len(instance.Terminals) != 2 {
				continue
			}
			left, right := instance.Terminals[0].Node, instance.Terminals[1].Node
			edges = append(
				edges,
				edge{from: left, to: right, resistance: *instance.ValueSI},
				edge{from: right, to: left, resistance: *instance.ValueSI},
			)
		case "isolated_converter":
			voltage := primitiveModelParameter(
				inventory[instance.PrimitiveKey],
				simmodel.PrimitiveProtectedIsolatedConverterV1,
				"output_voltage_v",
			)
			terminals := topologyTerminalNodes(instance)
			if voltage <= 0 || terminals["VOUT_MINUS"] == "" || terminals["VOUT_PLUS"] == "" {
				continue
			}
			edges = append(edges, edge{
				from: terminals["VOUT_MINUS"], to: terminals["VOUT_PLUS"], voltage: voltage,
			})
		}
	}
	adjacency := map[string][]edge{}
	for _, candidate := range edges {
		adjacency[candidate.from] = append(adjacency[candidate.from], candidate)
	}
	for node := range adjacency {
		slices.SortFunc(adjacency[node], func(left, right edge) int {
			return cmp.Or(
				cmp.Compare(left.to, right.to),
				cmp.Compare(left.voltage, right.voltage),
				cmp.Compare(left.resistance, right.resistance),
			)
		})
	}
	type path struct {
		voltage, resistance float64
	}
	paths := []path{}
	visited := map[string]bool{}
	references := topologyNodesByRole(graph, "reference")
	for _, reference := range references {
		// All reference nodes form one virtual source boundary. Marking every
		// reference visited prevents a resistor or other passive tie between
		// equivalent references from rediscovering a powered branch.
		visited[reference] = true
	}
	var walk func(string, float64, float64)
	walk = func(node string, voltage, resistance float64) {
		if node == rail {
			if voltage > 0 && resistance > 0 {
				paths = append(paths, path{voltage: voltage, resistance: resistance})
			}
			return
		}
		if len(visited) > len(graph.Nodes) {
			return
		}
		for _, candidate := range adjacency[node] {
			if visited[candidate.to] {
				continue
			}
			visited[candidate.to] = true
			walk(
				candidate.to,
				voltage+candidate.voltage,
				resistance+candidate.resistance,
			)
			delete(visited, candidate.to)
		}
	}
	for _, reference := range references {
		walk(reference, 0, 0)
	}
	conductanceByVoltage := map[float64]float64{}
	for _, candidate := range paths {
		conductanceByVoltage[candidate.voltage] += 1 / candidate.resistance
	}
	voltages := make([]float64, 0, len(conductanceByVoltage))
	for voltage := range conductanceByVoltage {
		voltages = append(voltages, voltage)
	}
	slices.Sort(voltages)
	for _, voltage := range voltages {
		conductance := conductanceByVoltage[voltage]
		if voltage > 0 && conductance > 0 {
			return voltage, voltage, 1 / conductance, true
		}
	}
	return 0, 0, 0, false
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
