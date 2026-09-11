package architecturesearch

import "strings"

// ResolveParticipantPort resolves a qualified requirement identity, not a
// component pin number or an arbitrary net name. Each side has its own ID
// namespace; a local port name alone is never an unambiguous observation.
func ResolveParticipantPort(requirement Requirement, id string) (Participant, ParticipantPort, bool) {
	participantID, portID, qualified := strings.Cut(id, ".")
	if !qualified || !validSemanticID(participantID) || !validSemanticID(portID) {
		return Participant{}, ParticipantPort{}, false
	}
	for _, participant := range requirement.Requirements.Participants {
		if participant.ID != participantID {
			continue
		}
		for _, port := range participant.RequiredPorts {
			if port.ID == portID {
				return participant, port, true
			}
		}
	}
	return Participant{}, ParticipantPort{}, false
}

func scalarParticipantPort(port ParticipantPort) bool {
	// These interfaces have multiple physical lanes. Observing whichever lane
	// happens to sort first is not faithful pin-specific measurement.
	return port.Kind != "digital_bus" && port.Kind != "differential_analog" && allowedPortKind(port.Kind)
}

func hasSingleReferenceDomain(requirement Requirement) bool {
	count := 0
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind == "reference" {
			count++
		}
	}
	return count == 1
}

func participantBindingEndpoint(requirement Requirement, binding Binding, produces bool) (string, bool) {
	participant, port, found := ResolveParticipantPort(requirement, binding.Participant+"."+binding.ParticipantPort)
	if !found || !scalarParticipantPort(port) {
		return "", false
	}
	// Direction is from the participant's perspective: an ADC sink is driven
	// by its conditioning objective, while a GPIO source drives that objective.
	direction := "source"
	if produces {
		direction = "sink"
	}
	if port.Direction != direction && port.Direction != "bidirectional" {
		return "", false
	}
	return participantAnchor(participant.ID, port.ID), true
}

func generatedPortSourceID(source string) (string, bool) {
	id, explicit := strings.CutPrefix(source, "port:")
	return id, explicit && validSemanticID(id)
}

func generatedSupplySource(requirement Requirement, domain Domain) (Observation, bool) {
	if domain.Kind != "supply" || domain.Source == "external" || domain.Source == "" {
		return Observation{}, false
	}
	if id, explicit := generatedPortSourceID(domain.Source); explicit {
		if !supportsBehavioralVerification(requirement.Version) {
			return Observation{}, false
		}
		port, exists := requirementPort(requirement, id)
		return Observation{Kind: "port", ID: id}, exists && port.Kind == "power" && port.Direction == "source" && port.Domain == domain.ID
	}
	for _, signal := range requirement.Requirements.Signals {
		if signal.ID == domain.Source && signal.Kind == "power" && signal.Domain == domain.ID {
			return Observation{Kind: "signal", ID: signal.ID}, true
		}
	}
	return Observation{}, false
}

func generatedSupplyProducer(requirement Requirement, source Observation) (Objective, bool) {
	if source.Kind == "signal" {
		return powerSignalProducer(requirement, source.ID)
	}
	var found []Objective
	for _, objective := range requirement.Requirements.Objectives {
		for _, binding := range objective.Bindings {
			produces := source.Kind == "port" && binding.Port == source.ID && binding.Role == "output"
			if produces {
				found = append(found, objective)
				break
			}
		}
	}
	if len(found) != 1 {
		return Objective{}, false
	}
	return found[0], true
}

// A generated public output can also feed real internal consumers. Its public
// direction describes the board boundary, not every objective attached to it.
// The new source form requires an explicit output role for its producer. Unknown
// roles fail closed; legacy public-port direction semantics remain unchanged.
func objectivePortDirection(requirement Requirement, binding Binding) (string, bool) {
	port, exists := requirementPort(requirement, binding.Port)
	if !exists {
		return "", false
	}
	for _, domain := range requirement.Requirements.Domains {
		source, valid := generatedSupplySource(requirement, domain)
		if !valid || source.Kind != "port" || source.ID != binding.Port {
			continue
		}
		switch binding.Role {
		case "output":
			return "source", true
		case "input", "sense", "protected", "power", "positive_power", "negative_power", "power_a", "power_b", "rail_a", "rail_b":
			return "sink", true
		default:
			return "", false
		}
	}
	return port.Direction, true
}
