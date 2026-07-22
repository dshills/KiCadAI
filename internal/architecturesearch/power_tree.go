package architecturesearch

import (
	"fmt"
	"slices"
	"strings"
)

// validatePowerTreeTopology proves only facts available from the normalized
// requirement and selected port contracts: every generated rail has exactly
// one selected producer and the derived rail graph is acyclic. Electrical
// capacity, voltage-window, tolerance, and thermal checks remain in the shared
// global validator and provider calculations.
func validatePowerTreeTopology(requirement Requirement, selections []FragmentSelection) ([]GlobalCheck, *candidateValidation) {
	if !supportsTypedSignals(requirement.Version) {
		return nil, nil
	}
	selectedPorts := map[string][]PortContract{}
	for _, selection := range selections {
		for _, port := range selection.Ports {
			selectedPorts[port.Anchor] = append(selectedPorts[port.Anchor], port.Contract)
		}
	}

	domains := append([]Domain(nil), requirement.Requirements.Domains...)
	slices.SortStableFunc(domains, func(left, right Domain) int { return strings.Compare(left.ID, right.ID) })
	edges := map[string]map[string]bool{}
	var checks []GlobalCheck
	for _, domain := range domains {
		if domain.Kind != "supply" {
			continue
		}
		// A missing source is legacy/underspecified domain metadata, not a
		// declaration that this requirement asks search to synthesize a rail.
		if domain.Source == "" {
			continue
		}
		path := "candidate.power_tree." + domain.ID
		if domain.Source == "external" {
			checks = append(checks, GlobalCheck{Code: CodePowerRailSourceMissing, Path: path + ".source", Message: "supply rail is explicitly externally sourced", Required: float64Pointer(1), Observed: float64Pointer(1), Margin: float64Pointer(0)})
			continue
		}
		anchor := signalAnchor(domain.Source)
		producers := 0
		for _, contract := range selectedPorts[anchor] {
			if contract.Kind == "power" && contract.Direction == "source" && contract.Domain == domain.ID {
				producers++
			}
		}
		if producers == 0 {
			return nil, &candidateValidation{Code: CodePowerRailSourceMissing, Path: path + ".source", Message: "generated supply rail lacks a selected power producer"}
		}
		if producers != 1 {
			return nil, &candidateValidation{Code: CodePowerRailSourceAmbiguous, Path: path + ".source", Message: fmt.Sprintf("generated supply rail has %d selected power producers", producers)}
		}
		checks = append(checks, GlobalCheck{Code: CodePowerRailSourceMissing, Path: path + ".source", Message: "generated supply rail has exactly one selected power producer", Required: float64Pointer(1), Observed: float64Pointer(1), Margin: float64Pointer(0)})

		producer, ok := powerSignalProducer(requirement, domain.Source)
		if !ok {
			return nil, &candidateValidation{Code: CodePowerRailSourceMissing, Path: path + ".producer", Message: "generated supply rail lacks a unique behavioral producer"}
		}
		for _, binding := range producer.Bindings {
			if canonicalIdentifier(binding.Role) != "input" {
				continue
			}
			inputDomain, ok := powerBindingDomain(requirement, binding)
			if !ok || inputDomain == domain.ID {
				continue
			}
			if edges[inputDomain] == nil {
				edges[inputDomain] = map[string]bool{}
			}
			edges[inputDomain][domain.ID] = true
		}
	}

	if cycle := firstPowerTreeCycle(domains, edges); len(cycle) != 0 {
		return nil, &candidateValidation{Code: CodePowerRailCycle, Path: "candidate.power_tree", Message: "power rail dependency cycle: " + strings.Join(cycle, " -> ")}
	}
	checks = append(checks, GlobalCheck{Code: CodePowerRailCycle, Path: "candidate.power_tree.acyclic", Message: "generated supply-rail dependency graph is acyclic", Required: float64Pointer(1), Observed: float64Pointer(1), Margin: float64Pointer(0)})
	return checks, nil
}

func powerBindingDomain(requirement Requirement, binding Binding) (string, bool) {
	if binding.Signal != "" {
		for _, signal := range requirement.Requirements.Signals {
			if signal.ID == canonicalIdentifier(binding.Signal) && signal.Kind == "power" {
				return signal.Domain, signal.Domain != ""
			}
		}
		return "", false
	}
	if binding.Port != "" {
		for _, port := range requirement.Requirements.Ports {
			if port.ID == canonicalIdentifier(binding.Port) && port.Kind == "power" {
				return port.Domain, port.Domain != ""
			}
		}
		return "", false
	}
	if binding.Participant == "" || binding.ParticipantPort == "" {
		return "", false
	}
	for _, participant := range requirement.Requirements.Participants {
		if participant.ID != canonicalIdentifier(binding.Participant) {
			continue
		}
		for _, port := range participant.RequiredPorts {
			if port.ID == canonicalIdentifier(binding.ParticipantPort) && port.Kind == "power" {
				return participant.Domain, participant.Domain != ""
			}
		}
	}
	return "", false
}

func powerSignalProducer(requirement Requirement, signalID string) (Objective, bool) {
	var producers []Objective
	for _, objective := range requirement.Requirements.Objectives {
		for _, binding := range objective.Bindings {
			if binding.Signal == signalID && binding.Direction == "source" {
				producers = append(producers, objective)
				break
			}
		}
	}
	slices.SortStableFunc(producers, func(left, right Objective) int { return strings.Compare(left.ID, right.ID) })
	if len(producers) != 1 {
		return Objective{}, false
	}
	return producers[0], true
}

func firstPowerTreeCycle(domains []Domain, edges map[string]map[string]bool) []string {
	state := map[string]uint8{}
	var stack []string
	var visit func(string) []string
	visit = func(domain string) []string {
		state[domain] = 1
		stack = append(stack, domain)
		children := make([]string, 0, len(edges[domain]))
		for child := range edges[domain] {
			children = append(children, child)
		}
		slices.Sort(children)
		for _, child := range children {
			switch state[child] {
			case 0:
				if cycle := visit(child); len(cycle) != 0 {
					return cycle
				}
			case 1:
				start := slices.Index(stack, child)
				cycle := append([]string(nil), stack[start:]...)
				return append(cycle, child)
			}
		}
		stack = stack[:len(stack)-1]
		state[domain] = 2
		return nil
	}
	for _, domain := range domains {
		if domain.Kind == "supply" && state[domain.ID] == 0 {
			if cycle := visit(domain.ID); len(cycle) != 0 {
				return cycle
			}
		}
	}
	return nil
}
