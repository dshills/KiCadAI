package compositionlowering

import "kicadai/internal/architecturesearch"

// The boolean means a declaration exists, even if its reference is invalid.
// Callers must then fail closed rather than use a legacy reference heuristic.
func explicitSemanticReference(requirement architecturesearch.Requirement, kind, id string) (string, bool) {
	domainID := ""
	switch kind {
	case "port":
		for _, port := range requirement.Requirements.Ports {
			if port.ID == id {
				domainID = port.Domain
				break
			}
		}
	case "signal":
		for _, signal := range requirement.Requirements.Signals {
			if signal.ID == id {
				domainID = signal.Domain
				break
			}
		}
	case "domain":
		domainID = id
	case "participant_port":
		if participant, _, ok := architecturesearch.ResolveParticipantPort(requirement, id); ok {
			domainID = participant.Domain
		}
	}
	for _, domain := range requirement.Requirements.Domains {
		if domain.ID == domainID && domain.ReferenceDomain != "" {
			reference, _ := architecturesearch.ResolveReferenceDomain(requirement, domainID)
			return reference, true
		}
	}
	return "", false
}
