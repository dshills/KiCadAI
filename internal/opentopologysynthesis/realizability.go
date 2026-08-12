package opentopologysynthesis

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"kicadai/internal/reports"
)

// RequirementRealizabilityFinding describes a topology-independent property
// that can refine a later terminal topology-search failure. A finding is not a
// synthesis failure by itself and absence of findings is not proof of
// feasibility.
type RequirementRealizabilityFinding struct {
	Code           reports.Code `json:"code"`
	Path           string       `json:"path"`
	RequirementIDs []string     `json:"requirement_ids,omitempty"`
	OperatingCases []string     `json:"operating_cases,omitempty"`
	AnalysisKinds  []string     `json:"analysis_kinds,omitempty"`
	Message        string       `json:"message"`
	Suggestion     string       `json:"suggestion"`
}

// RequirementRealizabilityAssessment is deterministic evidence about covered
// direct-domain and obligation-graph properties. Invalid requirements produce
// only Issues and are never classified.
type RequirementRealizabilityAssessment struct {
	Findings []RequirementRealizabilityFinding `json:"findings"`
	Issues   []reports.Issue                   `json:"issues,omitempty"`
}

type directVoltageEnvelope struct {
	outerLower      float64
	outerUpper      float64
	guaranteedLower float64
	guaranteedUpper float64
	uncertain       bool
}

// AssessRequirementRealizability classifies only properties that follow from
// the normalized behavior contract. It uses no inventory, topology, or host
// state and therefore cannot select an implementation family.
func AssessRequirementRealizability(requirement Requirement) RequirementRealizabilityAssessment {
	requirement = Normalize(requirement)
	if issues := Validate(requirement); len(issues) != 0 {
		return RequirementRealizabilityAssessment{Findings: []RequirementRealizabilityFinding{}, Issues: issues}
	}

	ports := make(map[string]Port, len(requirement.Requirements.Ports))
	for _, port := range requirement.Requirements.Ports {
		ports[port.ID] = port
	}
	cases := make(map[string]OperatingCase, len(requirement.Requirements.OperatingCases))
	envelopes := make(map[string]directVoltageEnvelope, len(requirement.Requirements.OperatingCases))
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		cases[operatingCase.ID] = operatingCase
		envelopes[operatingCase.ID] = directSupplyVoltageEnvelope(requirement, operatingCase, ports)
	}

	findings := requirementVoltageDomainFindings(requirement, ports, cases, envelopes)
	findings = append(findings, requirementObligationFindings(requirement, ports)...)
	for index := range findings {
		findings[index].RequirementIDs = sortedCompactStrings(findings[index].RequirementIDs)
		findings[index].OperatingCases = sortedCompactStrings(findings[index].OperatingCases)
		findings[index].AnalysisKinds = sortedCompactStrings(findings[index].AnalysisKinds)
	}
	slices.SortFunc(findings, func(left, right RequirementRealizabilityFinding) int {
		return cmp.Or(
			cmp.Compare(left.Code, right.Code),
			cmp.Compare(left.Path, right.Path),
			cmp.Compare(left.Message, right.Message),
		)
	})
	return RequirementRealizabilityAssessment{Findings: findings, Issues: []reports.Issue{}}
}

func requirementVoltageDomainFindings(
	requirement Requirement,
	ports map[string]Port,
	cases map[string]OperatingCase,
	envelopes map[string]directVoltageEnvelope,
) []RequirementRealizabilityFinding {
	findings := []RequirementRealizabilityFinding{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		port, found := ports[assertion.Observation.ID]
		if assertion.Observation.Kind != "port" || !found || port.Direction != "source" {
			continue
		}
		for _, caseID := range assertion.OperatingCases {
			if _, found := cases[caseID]; !found {
				continue
			}
			envelope := envelopes[caseID]
			required, reason := assertionRequiresCreatedEnergyDomain(assertion, envelope)
			if !required {
				continue
			}
			findings = append(findings, RequirementRealizabilityFinding{
				Code:           CodeEnergyDomainCreationRequired,
				Path:           "requirements.behavioral_requirements." + assertion.ID + ".operating_cases." + caseID,
				RequirementIDs: []string{assertion.ID}, OperatingCases: []string{caseID},
				AnalysisKinds: []string{assertion.Analysis},
				Message: fmt.Sprintf(
					"%s; direct external supplies provide outer %.12g..%.12g V and guaranteed %.12g..%.12g V",
					reason, envelope.outerLower, envelope.outerUpper,
					envelope.guaranteedLower, envelope.guaranteedUpper,
				),
				Suggestion: "declare a sufficient external energy domain or provide reviewed generic energy-domain creation capability",
			})
		}
	}
	return findings
}

func directSupplyVoltageEnvelope(
	requirement Requirement,
	operatingCase OperatingCase,
	ports map[string]Port,
) directVoltageEnvelope {
	// Validate requires a reference domain. Its zero-volt potential is always
	// part of both the outer and guaranteed direct envelope, so zero is the
	// intentional physical bound rather than an uninitialized sentinel.
	result := directVoltageEnvelope{
		outerLower: 0, outerUpper: 0,
		guaranteedLower: 0, guaranteedUpper: 0,
	}
	conditions := map[string]OperatingCondition{}
	conflictingConditions := map[string]bool{}
	for _, condition := range operatingCase.Conditions {
		if condition.Axis != "supply_voltage" {
			continue
		}
		target := condition.Target
		if port, found := ports[target]; found {
			target = port.Domain
		}
		if prior, found := conditions[target]; found {
			condition.Min = math.Max(prior.Min, condition.Min)
			condition.Max = math.Min(prior.Max, condition.Max)
			if condition.Min > condition.Max {
				conflictingConditions[target] = true
			}
		}
		conditions[target] = condition
	}
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind != "supply" {
			continue
		}
		if conflictingConditions[domain.ID] {
			result.uncertain = true
			continue
		}
		minimum, maximum, found := declaredDomainVoltageRange(domain)
		if condition, ok := conditions[domain.ID]; ok {
			minimum, maximum, found = condition.Min, condition.Max, realizabilityFinite(condition.Min) && realizabilityFinite(condition.Max)
		}
		if !found {
			result.uncertain = true
			continue
		}
		result.outerLower = math.Min(result.outerLower, minimum)
		result.outerUpper = math.Max(result.outerUpper, maximum)
		switch {
		case minimum > 0:
			result.guaranteedUpper = math.Max(result.guaranteedUpper, minimum)
		case maximum < 0:
			result.guaranteedLower = math.Min(result.guaranteedLower, maximum)
		}
	}
	return result
}

func declaredDomainVoltageRange(domain Domain) (float64, float64, bool) {
	if domain.MinVoltageV == nil || domain.MaxVoltageV == nil ||
		!realizabilityFinite(*domain.MinVoltageV) || !realizabilityFinite(*domain.MaxVoltageV) {
		return 0, 0, false
	}
	return *domain.MinVoltageV, *domain.MaxVoltageV, true
}

func realizabilityFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func assertionRequiresCreatedEnergyDomain(
	assertion BehavioralAssertion,
	envelope directVoltageEnvelope,
) (bool, string) {
	if envelope.uncertain {
		return false, ""
	}
	tolerance := 1e-12
	switch assertion.Metric {
	case "output_high_voltage":
		if assertion.Min != nil && *assertion.Min > envelope.guaranteedUpper+tolerance {
			return true, fmt.Sprintf("required output-high floor %.12g V exceeds the least-favorable direct positive rail", *assertion.Min)
		}
	case "output_low_voltage":
		if assertion.Max != nil && *assertion.Max < envelope.guaranteedLower-tolerance {
			return true, fmt.Sprintf("required output-low ceiling %.12g V is below the least-favorable direct negative rail", *assertion.Max)
		}
	case "output_voltage":
		if assertion.Min != nil && *assertion.Min > envelope.guaranteedUpper+tolerance {
			return true, fmt.Sprintf("required output-voltage floor %.12g V exceeds the least-favorable direct positive rail", *assertion.Min)
		}
		if assertion.Max != nil && *assertion.Max < envelope.guaranteedLower-tolerance {
			return true, fmt.Sprintf("required output-voltage ceiling %.12g V is below the least-favorable direct negative rail", *assertion.Max)
		}
	case "startup_output_voltage", "peak_voltage":
		availableMagnitude := math.Max(math.Abs(envelope.guaranteedLower), math.Abs(envelope.guaranteedUpper))
		if assertion.Min != nil && math.Abs(*assertion.Min) > availableMagnitude+tolerance {
			return true, fmt.Sprintf("required absolute peak %.12g V exceeds the least-favorable direct rail magnitude", math.Abs(*assertion.Min))
		}
	case "output_swing":
		availableSpan := envelope.guaranteedUpper - envelope.guaranteedLower
		if assertion.Min != nil && *assertion.Min > availableSpan+tolerance {
			return true, fmt.Sprintf("required output swing %.12g V_pp exceeds the least-favorable direct rail span", *assertion.Min)
		}
	}
	return false, ""
}

func requirementObligationFindings(
	requirement Requirement,
	ports map[string]Port,
) []RequirementRealizabilityFinding {
	byOutput := map[string][]BehavioralAssertion{}
	controlsByOutput := map[string]map[string]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		output, found := ports[assertion.Observation.ID]
		if assertion.Observation.Kind != "port" || !found || output.Direction != "source" {
			continue
		}
		byOutput[output.ID] = append(byOutput[output.ID], assertion)
		if assertion.Excitation != nil && assertion.Excitation.Kind == "port" && assertion.Excitation.ID != output.ID &&
			requirementPortIsIndependentControl(ports, assertion.Excitation.ID) {
			if controlsByOutput[output.ID] == nil {
				controlsByOutput[output.ID] = map[string]bool{}
			}
			controlsByOutput[output.ID][assertion.Excitation.ID] = true
		}
	}

	findings := []RequirementRealizabilityFinding{}
	for outputID, controls := range controlsByOutput {
		if len(controls) < 2 {
			continue
		}
		finding := RequirementRealizabilityFinding{
			Code:       CodeMultiControlCompositionRequired,
			Path:       "requirements.ports." + outputID,
			Message:    fmt.Sprintf("%d independent external excitation ports converge on source output %s", len(controls), outputID),
			Suggestion: "compose all external control obligations without conflating internal feedback paths",
		}
		controlledAssertions := []BehavioralAssertion{}
		for _, assertion := range byOutput[outputID] {
			if assertion.Excitation != nil && controls[assertion.Excitation.ID] {
				controlledAssertions = append(controlledAssertions, assertion)
			}
		}
		appendAssertionFindingEvidence(&finding, controlledAssertions)
		findings = append(findings, finding)
	}
	return findings
}

func appendAssertionFindingEvidence(finding *RequirementRealizabilityFinding, assertions []BehavioralAssertion) {
	for _, assertion := range assertions {
		finding.RequirementIDs = append(finding.RequirementIDs, assertion.ID)
		finding.OperatingCases = append(finding.OperatingCases, assertion.OperatingCases...)
		finding.AnalysisKinds = append(finding.AnalysisKinds, assertion.Analysis)
	}
}

func sortedCompactStrings(values []string) []string {
	result := slices.Clone(values)
	slices.Sort(result)
	return slices.Compact(result)
}
