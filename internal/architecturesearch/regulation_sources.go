package architecturesearch

import "fmt"

// regulationSources checks declared behavioral provenance, not electrical or
// physical feasibility. An external rail must not stand in for the output that
// a voltage-regulation objective is supposed to generate. This is a v3+ check;
// legacy v1/v2 contracts and their support boundary remain unchanged.
func (validator *requirementValidator) regulationSources() {
	if !supportsBehavioralVerification(validator.requirement.Version) {
		return
	}
	for index, domain := range validator.requirement.Requirements.Domains {
		if _, explicit := generatedPortSourceID(domain.Source); !explicit {
			continue
		}
		source, valid := generatedSupplySource(validator.requirement, domain)
		if !valid {
			continue
		} // The domain-source validator reports this.
		if _, unique := generatedSupplyProducer(validator.requirement, source); !unique {
			validator.add(CodeDomainInvalid, fmt.Sprintf("requirements.domains[%d].source", index), "derived supply output port must have exactly one declared objective producer")
		}
	}
	for index, objective := range validator.requirement.Requirements.Objectives {
		if objective.Capability != "voltage_regulation" {
			continue
		}
		path := fmt.Sprintf("requirements.objectives[%d].bindings", index)
		produced := map[string]bool{}
		for _, domain := range validator.requirement.Requirements.Domains {
			source, valid := generatedSupplySource(validator.requirement, domain)
			if !valid || source.Kind != "port" {
				continue
			}
			producer, unique := generatedSupplyProducer(validator.requirement, source)
			if unique && producer.ID == objective.ID {
				produced[domain.ID] = true
			}
		}
		for bindingIndex, binding := range objective.Bindings {
			if binding.Signal == "" || binding.Direction != "source" {
				continue
			}
			signal, exists := validator.signalsByID[binding.Signal]
			if !exists || signal.Kind != "power" {
				continue
			}
			domain, exists := validator.domainsByID[signal.Domain]
			if !exists || domain.Kind != "supply" || domain.Source != signal.ID {
				validator.add(CodeDomainInvalid, fmt.Sprintf("%s[%d].signal", path, bindingIndex), fmt.Sprintf("regulation output signal %q must be the declared source of its own derived supply domain", signal.ID))
				continue
			}
			produced[domain.ID] = true
		}
		if len(produced) == 0 {
			validator.add(CodeDomainInvalid, path, "voltage_regulation must produce a declared power signal or explicit power-output port for a derived supply domain; an independent external supply is not a regulated output")
		}
		for bindingIndex, binding := range objective.Bindings {
			// The registered input role is an input even in legacy declarations
			// whose external-port direction is expressed from the supply side.
			if binding.Role == "input" {
				continue
			}
			output := binding.Role == "output"
			if port, exists := validator.portsByID[binding.Port]; exists {
				output = output || port.Direction == "source"
			}
			if !output || binding.Signal != "" {
				continue
			}
			domainID, isPower := powerBindingDomain(validator.requirement, binding)
			if isPower && !produced[domainID] {
				validator.add(CodeDomainInvalid, fmt.Sprintf("%s[%d]", path, bindingIndex), fmt.Sprintf("regulation output on domain %q must belong to a derived supply produced by this objective, not an external source or another producer", domainID))
			}
		}
	}
}
