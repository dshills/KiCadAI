package architecturesearch

import (
	"fmt"
	"strings"
)

// ResolveReferenceDomain returns a declared electrical reference, or the only
// reference in a legacy single-reference requirement. It never infers a return
// from spelling or ordering. An invalid explicit reference cannot fall back.
func ResolveReferenceDomain(requirement Requirement, domainID string) (string, bool) {
	domain := requirementDomain(requirement, domainID)
	if domain.Kind == "reference" && domain.ReferenceDomain == "" {
		return domain.ID, true
	}
	if domain.ReferenceDomain != "" {
		reference := requirementDomain(requirement, domain.ReferenceDomain)
		if domain.Kind != "supply" || reference.Kind != "reference" || reference.ReferenceDomain != "" {
			return "", false
		}
		return reference.ID, true
	}
	var reference string
	for _, candidate := range requirement.Requirements.Domains {
		if candidate.Kind == "reference" {
			if reference != "" {
				return "", false
			}
			reference = candidate.ID
		}
	}
	return reference, reference != ""
}

// Check the declared return against the objective's physical reference binding.
// A common reference covers every endpoint; side-specific references cover only
// their named side. Unknown side conventions fail closed for explicit domains.
func (validator *requirementValidator) explicitObjectiveReferences() {
	for i, objective := range validator.requirement.Requirements.Objectives {
		references := map[string]string{}
		for _, binding := range objective.Bindings {
			if binding.Role == "reference" || strings.HasPrefix(binding.Role, "reference_") {
				// Presence matters: an invalid explicit side return must not fall
				// back to an otherwise valid common return.
				references[binding.Role] = ""
				domain := validator.domainsByID[validator.portsByID[binding.Port].Domain]
				if domain.Kind == "reference" {
					references[binding.Role] = domain.ID
				}
			}
		}
		for j, binding := range objective.Bindings {
			domainID := validator.portsByID[binding.Port].Domain
			if binding.Signal != "" {
				domainID = validator.signalsByID[binding.Signal].Domain
			}
			if binding.Participant != "" {
				domainID = validator.participantsByID[binding.Participant].Domain
			}
			domain := validator.domainsByID[domainID]
			if domain.ReferenceDomain == "" {
				continue
			}
			reference := references["reference"]
			for _, side := range []string{"a", "b"} {
				if strings.HasSuffix(binding.Role, "_"+side) {
					if sideReference, explicit := references["reference_"+side]; explicit {
						reference = sideReference
					}
				}
			}
			if reference != domain.ReferenceDomain {
				validator.add(CodeDomainInvalid, fmt.Sprintf("requirements.objectives[%d].bindings[%d]", i, j), "explicit supply reference_domain must match this endpoint's physical objective reference binding")
			}
		}
	}
}
