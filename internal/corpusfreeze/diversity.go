package corpusfreeze

import (
	"fmt"
	"sort"
	"strings"

	ots "kicadai/internal/opentopologysynthesis"
)

type diversityEvidence struct {
	supplyConfigurations  map[string]bool
	observations          map[string]bool
	analysisCategories    map[string]bool
	analysisKinds         map[string]bool
	variations            map[string]bool
	events                map[string]bool
	criticalDomains       map[string]bool
	multiOutput           int
	convergingExcitations int
}

func newDiversityEvidence() *diversityEvidence {
	return &diversityEvidence{
		supplyConfigurations: map[string]bool{}, observations: map[string]bool{},
		analysisCategories: map[string]bool{}, analysisKinds: map[string]bool{},
		variations: map[string]bool{}, events: map[string]bool{}, criticalDomains: map[string]bool{},
	}
}

func (evidence *diversityEvidence) observe(reportingDomain string, requirement ots.Requirement) {
	supplyCount, positive, negative := 0, false, false
	ports := map[string]ots.Port{}
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind != "supply" {
			continue
		}
		supplyCount++
		if domain.NominalVoltageV != nil && *domain.NominalVoltageV > 0 {
			positive = true
		}
		if domain.NominalVoltageV != nil && *domain.NominalVoltageV < 0 {
			negative = true
		}
	}
	if supplyCount == 1 && positive {
		evidence.supplyConfigurations["single_positive"] = true
	}
	if positive && negative {
		evidence.supplyConfigurations["bipolar"] = true
	}
	if supplyCount >= 2 {
		evidence.supplyConfigurations["multiple"] = true
	}
	for _, port := range requirement.Requirements.Ports {
		ports[port.ID] = port
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			switch condition.Axis {
			case "load_capacitance", "load_current", "load_inductance", "load_resistance":
				evidence.variations["load"] = true
			case "model_corner", "tolerance_corner":
				evidence.variations["tolerance_model"] = true
			case "ambient_temperature":
				evidence.variations["temperature"] = true
			case "supply_voltage":
				evidence.variations["supply"] = true
			}
		}
		for _, event := range operatingCase.Events {
			evidence.events[event.Kind] = true
		}
	}
	observedOutputs := map[string]bool{}
	excitationsByObservation := map[string]map[string]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		evidence.analysisKinds[assertion.Analysis] = true
		if assertion.Critical {
			evidence.criticalDomains[reportingDomain] = true
		}
		switch assertion.Analysis {
		case "dc_operating_point", "dc_sweep":
			evidence.analysisCategories["dc"] = true
		case "ac_sweep", "noise", "stability", "distortion":
			evidence.analysisCategories["ac_noise_stability"] = true
		case "transient", "startup":
			evidence.analysisCategories["transient_startup"] = true
		case "thermal", "electrothermal":
			evidence.analysisCategories["thermal"] = true
		}
		switch assertion.Metric {
		case "dc_voltage", "output_voltage", "output_high_voltage", "output_low_voltage", "output_swing", "peak_voltage", "startup_output_voltage":
			evidence.observations["voltage"] = true
		case "dc_current", "output_current", "peak_current", "startup_current", "off_state_current", "quiescent_current":
			evidence.observations["current"] = true
		case "output_power":
			evidence.observations["power"] = true
		}
		if port, ok := ports[assertion.Observation.ID]; assertion.Observation.Kind == "port" && ok && port.Direction == "source" {
			observedOutputs[port.ID] = true
			if assertion.Excitation != nil {
				if excitationsByObservation[port.ID] == nil {
					excitationsByObservation[port.ID] = map[string]bool{}
				}
				excitationsByObservation[port.ID][assertion.Excitation.Kind+":"+assertion.Excitation.ID] = true
			}
		}
	}
	if len(observedOutputs) >= 2 {
		evidence.multiOutput++
	}
	for _, excitations := range excitationsByObservation {
		if len(excitations) >= 2 {
			evidence.convergingExcitations++
			break
		}
	}
}

func validateRoleDiversity(role string, evidence *diversityEvidence, policy Policy) error {
	checks := []struct {
		name     string
		got      map[string]bool
		required []string
	}{
		{"supply configurations", evidence.supplyConfigurations, policy.RequiredSupplyConfigurations},
		{"observation kinds", evidence.observations, policy.RequiredObservationKinds},
		{"analysis categories", evidence.analysisCategories, policy.RequiredAnalysisCategories},
		{"variation categories", evidence.variations, policy.RequiredVariationCategories},
		{"event kinds", evidence.events, policy.RequiredEventKinds},
	}
	for _, check := range checks {
		if missing := missingValues(check.got, check.required); len(missing) != 0 {
			return fmt.Errorf("role %s omits required %s: %s", role, check.name, strings.Join(missing, ","))
		}
	}
	if evidence.multiOutput < policy.MinimumMultiOutputPerRole {
		return fmt.Errorf("role %s multi-output cases = %d, want at least %d", role, evidence.multiOutput, policy.MinimumMultiOutputPerRole)
	}
	if evidence.convergingExcitations < policy.MinimumConvergingInputsPerRole {
		return fmt.Errorf("role %s converging-input cases = %d, want at least %d", role, evidence.convergingExcitations, policy.MinimumConvergingInputsPerRole)
	}
	if len(evidence.criticalDomains) < policy.MinimumCriticalDomainsPerRole {
		return fmt.Errorf("role %s critical domains = %d, want at least %d", role, len(evidence.criticalDomains), policy.MinimumCriticalDomainsPerRole)
	}
	return nil
}

func missingValues(got map[string]bool, required []string) []string {
	var missing []string
	for _, value := range required {
		if !got[value] {
			missing = append(missing, value)
		}
	}
	sort.Strings(missing)
	return missing
}

func requirementSignatures(requirement ots.Requirement) (string, string, string) {
	ports := make([]string, 0, len(requirement.Requirements.Ports))
	for _, port := range requirement.Requirements.Ports {
		ports = append(ports, port.Kind+"|"+port.Direction)
	}
	assertions := make([]string, 0, len(requirement.Requirements.BehavioralRequirements))
	analyses := map[string]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		assertions = append(assertions, fmt.Sprintf("%s|%s|%t|%t|%s", assertion.Metric, assertion.Unit, assertion.Min != nil, assertion.Max != nil, assertion.Observation.Kind))
		analyses[assertion.Analysis] = true
	}
	analysisList := make([]string, 0, len(analyses))
	for analysis := range analyses {
		analysisList = append(analysisList, analysis)
	}
	sort.Strings(ports)
	sort.Strings(assertions)
	sort.Strings(analysisList)
	return strings.Join(ports, ";"), strings.Join(assertions, ";"), strings.Join(analysisList, ";")
}
