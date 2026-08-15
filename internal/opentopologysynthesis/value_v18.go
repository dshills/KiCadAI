package opentopologysynthesis

import (
	"math"
	"slices"
)

// EnumerateValueTrialsV18 prepends one topology-aware analytic bundle for a
// threshold window, then preserves the existing rank-bounded enumeration.
// The bundle jointly solves coupled divider ratios that cannot be represented
// by assigning the same global analytic scales independently to every resistor.
func EnumerateValueTrialsV18(
	plan ValueSearchPlan,
	requirement Requirement,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	maximum int,
) ValueTrialEnumeration {
	result := EnumerateValueTrials(plan, maximum)
	if maximum <= 0 {
		return result
	}
	analytic, found := v18WindowValueTrial(requirement, graph, inventory, plan)
	if !found {
		return result
	}
	trials := make([]ValueTrial, 0, min(maximum, len(result.Trials)+1))
	trials = append(trials, analytic)
	for _, trial := range result.Trials {
		if trial.Hash == analytic.Hash || len(trials) >= maximum {
			continue
		}
		trials = append(trials, trial)
	}
	for index := range trials {
		trials[index].Number = index + 1
	}
	result.Trials = trials
	result.Exhausted = result.TotalCombinations > uint64(len(result.Trials))
	return result
}

func v18WindowValueTrial(
	requirement Requirement,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	plan ValueSearchPlan,
) (ValueTrial, bool) {
	envelope, found := topologyWindowThresholdEnvelope(requirement)
	if !found {
		return ValueTrial{}, false
	}
	inventoryByKey := primitiveInventoryByKey(inventory)
	absoluteReference, referenceNode, referenceVoltage, found := v18WindowReference(graph, inventoryByKey)
	if !found || referenceVoltage <= 0 {
		return ValueTrial{}, false
	}
	inputNode := "port_" + envelope.input
	lowerReference, upperReference, found := v18WindowDecisionReferences(graph, inputNode)
	if !found {
		return ValueTrial{}, false
	}
	lowerTap, lowerFeedback, found := v18WindowAmplifierNodes(graph, lowerReference)
	if !found {
		return ValueTrial{}, false
	}
	_ = lowerFeedback
	upperPositive, upperGain, found := v18WindowAmplifierNodes(graph, upperReference)
	if !found || upperPositive != absoluteReference {
		return ValueTrial{}, false
	}
	lowerTop := v18ResistorBetween(graph, absoluteReference, lowerTap)
	lowerBottom := v18ResistorBetween(graph, lowerTap, referenceNode)
	upperGround := v18ResistorBetween(graph, upperGain, referenceNode)
	upperFeedback := v18ResistorPath(graph, upperReference, upperGain, referenceNode, 3)
	if lowerTop == "" || lowerBottom == "" || upperGround == "" || len(upperFeedback) == 0 {
		return ValueTrial{}, false
	}
	domainByInstance := make(map[string]InstanceValueDomain, len(plan.Domains))
	for _, domain := range plan.Domains {
		domainByInstance[domain.InstanceID] = domain
	}
	assignment := map[string]ComponentValueCandidate{}
	lowerTopCandidate, lowerBottomCandidate, found := v18BestDividerPair(
		domainByInstance[lowerTop], domainByInstance[lowerBottom], referenceVoltage, envelope.lowerV,
	)
	if !found {
		return ValueTrial{}, false
	}
	assignment[lowerTop], assignment[lowerBottom] = lowerTopCandidate, lowerBottomCandidate
	upperAssignment, found := v18BestNonInvertingDivider(
		upperFeedback, upperGround, domainByInstance, referenceVoltage, envelope.upperV,
	)
	if !found {
		return ValueTrial{}, false
	}
	for instanceID, candidate := range upperAssignment {
		assignment[instanceID] = candidate
	}
	selections := make([]ValueTrialSelection, 0, len(plan.Domains))
	for _, domain := range plan.Domains {
		if len(domain.Candidates) == 0 {
			return ValueTrial{}, false
		}
		candidate := domain.Candidates[0]
		if assigned, exists := assignment[domain.InstanceID]; exists {
			candidate = assigned
		}
		selections = append(selections, ValueTrialSelection{
			InstanceID: domain.InstanceID, PrimitiveKey: candidate.PrimitiveKey,
			ValueSI: cloneInventoryFloat(candidate.ValueSI), CandidateHash: candidate.Hash,
		})
	}
	trial := ValueTrial{Number: 1, Selections: selections}
	trial.Hash = valueTrialHash(trial.Selections)
	return trial, true
}

func v18WindowReference(
	graph CandidateGraph,
	inventory map[string]PrimitiveCandidate,
) (absoluteReference, referenceNode string, voltage float64, found bool) {
	for _, instance := range graph.Instances {
		if instance.Kind != "reference_diode" {
			continue
		}
		primitive, exists := inventory[instance.PrimitiveKey]
		if !exists {
			continue
		}
		voltage, found = topologyPrimitiveReferenceVoltage(primitive)
		if !found {
			continue
		}
		for _, connection := range instance.Terminals {
			switch connection.Terminal {
			case "ANODE":
				referenceNode = connection.Node
			case "CATHODE":
				absoluteReference = connection.Node
			}
		}
		if absoluteReference != "" && referenceNode != "" {
			return absoluteReference, referenceNode, voltage, true
		}
	}
	return "", "", 0, false
}

func v18WindowDecisionReferences(graph CandidateGraph, inputNode string) (lower, upper string, found bool) {
	for _, instance := range graph.Instances {
		if instance.Kind != "comparator" {
			continue
		}
		terminals := v18TerminalNodes(instance)
		switch {
		case terminals["IN_PLUS"] == inputNode && terminals["IN_MINUS"] != "":
			lower = terminals["IN_MINUS"]
		case terminals["IN_MINUS"] == inputNode && terminals["IN_PLUS"] != "":
			upper = terminals["IN_PLUS"]
		}
	}
	return lower, upper, lower != "" && upper != ""
}

func v18WindowAmplifierNodes(graph CandidateGraph, outputNode string) (positive, negative string, found bool) {
	for _, instance := range graph.Instances {
		if instance.Kind != "opamp" {
			continue
		}
		terminals := v18TerminalNodes(instance)
		if terminals["OUT"] == outputNode {
			return terminals["IN_PLUS"], terminals["IN_MINUS"], true
		}
	}
	return "", "", false
}

func v18TerminalNodes(instance GraphInstance) map[string]string {
	result := make(map[string]string, len(instance.Terminals))
	for _, connection := range instance.Terminals {
		result[connection.Terminal] = connection.Node
	}
	return result
}

func v18ResistorBetween(graph CandidateGraph, left, right string) string {
	for _, instance := range graph.Instances {
		if instance.Kind != "resistor" || len(instance.Terminals) != 2 {
			continue
		}
		a, b := instance.Terminals[0].Node, instance.Terminals[1].Node
		if a == left && b == right || a == right && b == left {
			return instance.ID
		}
	}
	return ""
}

func v18ResistorPath(graph CandidateGraph, start, end, excluded string, maximumEdges int) []string {
	type edge struct {
		instance, next string
	}
	adjacency := map[string][]edge{}
	for _, instance := range graph.Instances {
		if instance.Kind != "resistor" || len(instance.Terminals) != 2 {
			continue
		}
		left, right := instance.Terminals[0].Node, instance.Terminals[1].Node
		adjacency[left] = append(adjacency[left], edge{instance: instance.ID, next: right})
		adjacency[right] = append(adjacency[right], edge{instance: instance.ID, next: left})
	}
	for node := range adjacency {
		slices.SortFunc(adjacency[node], func(left, right edge) int {
			if left.instance < right.instance {
				return -1
			}
			if left.instance > right.instance {
				return 1
			}
			return 0
		})
	}
	var search func(string, []string, map[string]bool) []string
	search = func(node string, path []string, visited map[string]bool) []string {
		if node == end {
			return path
		}
		if len(path) >= maximumEdges {
			return nil
		}
		for _, candidate := range adjacency[node] {
			if candidate.next == excluded || visited[candidate.next] {
				continue
			}
			nextVisited := make(map[string]bool, len(visited)+1)
			for key, value := range visited {
				nextVisited[key] = value
			}
			nextVisited[candidate.next] = true
			if result := search(candidate.next, append(append([]string(nil), path...), candidate.instance), nextVisited); len(result) != 0 {
				return result
			}
		}
		return nil
	}
	return search(start, nil, map[string]bool{start: true})
}

func v18BestDividerPair(
	top, bottom InstanceValueDomain,
	referenceVoltage, target float64,
) (ComponentValueCandidate, ComponentValueCandidate, bool) {
	bestError := math.Inf(1)
	bestTop, bestBottom := ComponentValueCandidate{}, ComponentValueCandidate{}
	for _, topCandidate := range top.Candidates {
		topValue, topOK := v18CandidateResistance(topCandidate)
		if !topOK {
			continue
		}
		for _, bottomCandidate := range bottom.Candidates {
			bottomValue, bottomOK := v18CandidateResistance(bottomCandidate)
			if !bottomOK {
				continue
			}
			actual := referenceVoltage * bottomValue / (topValue + bottomValue)
			error := math.Abs(actual - target)
			if error < bestError {
				bestError, bestTop, bestBottom = error, topCandidate, bottomCandidate
			}
		}
	}
	return bestTop, bestBottom, finite(bestError)
}

func v18BestNonInvertingDivider(
	feedback []string,
	ground string,
	domains map[string]InstanceValueDomain,
	referenceVoltage, target float64,
) (map[string]ComponentValueCandidate, bool) {
	instances := append([]string(nil), feedback...)
	instances = append(instances, ground)
	bestError := math.Inf(1)
	best := map[string]ComponentValueCandidate{}
	current := map[string]ComponentValueCandidate{}
	var enumerate func(int)
	enumerate = func(index int) {
		if index == len(instances) {
			groundValue, ok := v18CandidateResistance(current[ground])
			if !ok {
				return
			}
			feedbackValue := 0.0
			for _, instanceID := range feedback {
				value, valueOK := v18CandidateResistance(current[instanceID])
				if !valueOK {
					return
				}
				feedbackValue += value
			}
			actual := referenceVoltage * (1 + feedbackValue/groundValue)
			error := math.Abs(actual - target)
			if error < bestError {
				bestError = error
				best = make(map[string]ComponentValueCandidate, len(current))
				for instanceID, candidate := range current {
					best[instanceID] = candidate
				}
			}
			return
		}
		instanceID := instances[index]
		for _, candidate := range domains[instanceID].Candidates {
			if _, ok := v18CandidateResistance(candidate); !ok {
				continue
			}
			current[instanceID] = candidate
			enumerate(index + 1)
		}
	}
	enumerate(0)
	return best, finite(bestError)
}

func v18CandidateResistance(candidate ComponentValueCandidate) (float64, bool) {
	if candidate.ValueSI == nil || *candidate.ValueSI <= 0 || !finite(*candidate.ValueSI) {
		return 0, false
	}
	return *candidate.ValueSI, true
}
