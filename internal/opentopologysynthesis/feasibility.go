package opentopologysynthesis

import (
	"fmt"
	"math"
	"strings"

	"kicadai/internal/reports"
)

// requirementFeasibilityIssues proves contradictions that follow directly
// from the behavior envelope, before topology search can spend a bounded
// budget on an impossible request. It deliberately reports only necessary
// physical bounds; absence of an issue is not a claim that the request is
// feasible.
func requirementFeasibilityIssues(requirement Requirement) []reports.Issue {
	requirement = Normalize(requirement)
	ports := make(map[string]Port, len(requirement.Requirements.Ports))
	for _, port := range requirement.Requirements.Ports {
		ports[port.ID] = port
	}
	cases := make(map[string]OperatingCase, len(requirement.Requirements.OperatingCases))
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		cases[operatingCase.ID] = operatingCase
	}
	maximumRailVoltage := maximumDeclaredSupplySpan(requirement)
	issues := []reports.Issue{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "transconductance" || assertion.Unit != "A/V" ||
			assertion.Min == nil || *assertion.Min <= 0 || assertion.Excitation == nil ||
			assertion.Excitation.Kind != "port" || assertion.Observation.Kind != "port" {
			continue
		}
		input, inputFound := ports[assertion.Excitation.ID]
		output, outputFound := ports[assertion.Observation.ID]
		if !inputFound || !outputFound || input.Kind != "analog_voltage" ||
			output.Kind != "controlled_current" || output.Direction != "source" {
			continue
		}
		availableVoltage := maximumRailVoltage
		if domainVoltage, found := maximumDeclaredSupplyMagnitudeForDomain(requirement, output.Domain); found {
			availableVoltage = domainVoltage
		}
		if output.Electrical.MaxVoltageV != nil && *output.Electrical.MaxVoltageV > 0 {
			availableVoltage = math.Min(availableVoltage, *output.Electrical.MaxVoltageV)
		}
		if !finite(availableVoltage) || availableVoltage <= 0 {
			continue
		}
		for _, caseID := range assertion.OperatingCases {
			operatingCase, found := cases[caseID]
			if !found {
				continue
			}
			inputSpan, maximumLoad, found := transconductanceComplianceEnvelope(
				operatingCase, assertion.Excitation.ID, assertion.Observation.ID,
			)
			if !found {
				continue
			}
			minimumCurrentExcursion := *assertion.Min * inputSpan
			minimumLoadVoltageExcursion := minimumCurrentExcursion * maximumLoad
			if minimumLoadVoltageExcursion <= availableVoltage*(1+1e-12) {
				continue
			}
			issues = append(issues, reports.Issue{
				Code:     CodeRequirementInfeasible,
				Severity: reports.SeverityBlocked,
				Path:     "requirements.behavioral_requirements." + assertion.ID + ".operating_cases." + caseID,
				Message: fmt.Sprintf(
					"minimum transconductance %.12g A/V across the declared %.12g V input span requires at least %.12g A of current excursion and %.12g V across the %.12g ohm load, exceeding the declared %.12g V positive supply/output ceiling",
					*assertion.Min, inputSpan, minimumCurrentExcursion,
					minimumLoadVoltageExcursion, maximumLoad, availableVoltage,
				),
				Suggestion: "reduce the required transfer or simultaneous input/load envelope, or declare a sufficient positive supply and output-voltage envelope",
			})
		}
	}
	return issues
}

func maximumDeclaredSupplyMagnitudeForDomain(requirement Requirement, domainID string) (float64, bool) {
	domainID = strings.TrimSpace(domainID)
	if domainID == "" {
		return 0, false
	}
	for _, domain := range requirement.Requirements.Domains {
		if domain.ID != domainID || domain.Kind != "supply" {
			continue
		}
		maximum := 0.0
		for _, value := range []*float64{domain.MinVoltageV, domain.NominalVoltageV, domain.MaxVoltageV} {
			if value != nil && finite(*value) && math.Abs(*value) > maximum {
				maximum = math.Abs(*value)
			}
		}
		return maximum, maximum > 0
	}
	return 0, false
}

// maximumDeclaredSupplySpan returns the widest necessary voltage bound that
// can be proved from declared rails alone. Including zero makes a single-ended
// supply its voltage-to-reference magnitude, while bipolar rails yield their
// full negative-to-positive span. A later output-port bound may narrow it.
func maximumDeclaredSupplySpan(requirement Requirement) float64 {
	minimum, maximum := 0.0, 0.0
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind != "supply" {
			continue
		}
		for _, value := range []*float64{domain.MinVoltageV, domain.NominalVoltageV, domain.MaxVoltageV} {
			if value == nil || !finite(*value) {
				continue
			}
			minimum = math.Min(minimum, *value)
			maximum = math.Max(maximum, *value)
		}
	}
	return maximum - minimum
}

func transconductanceComplianceEnvelope(
	operatingCase OperatingCase,
	inputID string,
	outputID string,
) (float64, float64, bool) {
	inputSpan := 0.0
	maximumLoad := 0.0
	for _, condition := range operatingCase.Conditions {
		switch {
		case condition.Axis == "input_voltage" && condition.Target == inputID:
			inputSpan = condition.Max - condition.Min
		case condition.Axis == "load_resistance" && condition.Target == outputID:
			maximumLoad = condition.Max
		}
	}
	return inputSpan, maximumLoad,
		finite(inputSpan) && inputSpan > 0 && finite(maximumLoad) && maximumLoad > 0
}

// requirementCapabilityIssues identifies behavior that lies beyond every
// reviewed dynamic model in the supplied inventory. The comparison uses the
// fastest declared active-device parameter, so a rejection is conservative:
// adding a faster reviewed primitive can make the request eligible without a
// production-code change.
func requirementCapabilityIssues(requirement Requirement, inventory PrimitiveInventory) []reports.Issue {
	requirement = Normalize(requirement)
	regulatedCurrentOutputs := map[string]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "transconductance" && assertion.Observation.Kind == "port" &&
			assertion.Excitation != nil && assertion.Excitation.Kind == "port" {
			regulatedCurrentOutputs[assertion.Observation.ID] = true
		}
	}
	maximumReviewedFrequency := maximumReviewedActiveFrequency(inventory)
	if maximumReviewedFrequency <= 0 {
		return nil
	}
	issues := []reports.Issue{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Observation.Kind != "port" || !regulatedCurrentOutputs[assertion.Observation.ID] {
			continue
		}
		requiredFrequency := 0.0
		switch assertion.Metric {
		case "bandwidth":
			if assertion.Min != nil && *assertion.Min > 0 {
				requiredFrequency = topologyControllerGBWReserve * *assertion.Min
			}
		case "rise_time", "fall_time":
			if assertion.Max != nil && *assertion.Max > 0 {
				requiredFrequency = topologyControllerGBWReserve * .35 / *assertion.Max
			}
		}
		if requiredFrequency <= maximumReviewedFrequency*(1+1e-12) {
			continue
		}
		issues = append(issues, reports.Issue{
			Code:     CodeModelUnavailable,
			Severity: reports.SeverityBlocked,
			Path:     "requirements.behavioral_requirements." + assertion.ID,
			Message: fmt.Sprintf(
				"closed-loop controlled-current behavior requires at least %.12g Hz of reviewed active-device frequency capability, but the fastest declared inventory model is %.12g Hz",
				requiredFrequency, maximumReviewedFrequency,
			),
			Suggestion: "onboard a faster reviewed active-device model or reduce the required closed-loop bandwidth/transition envelope",
		})
	}
	return issues
}

func maximumReviewedActiveFrequency(inventory PrimitiveInventory) float64 {
	maximum := 0.0
	for _, primitive := range inventory.Primitives {
		for _, model := range primitive.Models {
			for _, parameter := range model.Parameters {
				switch parameter.Name {
				case "gain_bandwidth_hz", "bandwidth_hz", "transition_frequency_hz":
					if finite(parameter.Value) && parameter.Value > maximum {
						maximum = parameter.Value
					}
				}
			}
		}
	}
	return maximum
}
