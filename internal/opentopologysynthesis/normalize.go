package opentopologysynthesis

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
)

func Normalize(requirement Requirement) Requirement {
	result := cloneRequirement(requirement)
	result.Schema = strings.TrimSpace(result.Schema)
	result.Project.Name = canonicalIdentifier(result.Project.Name)
	result.Project.Title = strings.TrimSpace(result.Project.Title)
	result.Project.Description = strings.TrimSpace(result.Project.Description)

	for index := range result.Requirements.Domains {
		domain := &result.Requirements.Domains[index]
		domain.ID = canonicalIdentifier(domain.ID)
		domain.Kind = canonicalIdentifier(domain.Kind)
		domain.Source = canonicalIdentifier(domain.Source)
	}
	slices.SortFunc(result.Requirements.Domains, func(left, right Domain) int {
		return cmp.Compare(left.ID, right.ID)
	})

	for index := range result.Requirements.Ports {
		port := &result.Requirements.Ports[index]
		port.ID = canonicalIdentifier(port.ID)
		port.Kind = canonicalIdentifier(port.Kind)
		port.Direction = canonicalIdentifier(port.Direction)
		port.Domain = canonicalIdentifier(port.Domain)
		port.Electrical.DefaultState = canonicalIdentifier(port.Electrical.DefaultState)
	}
	slices.SortFunc(result.Requirements.Ports, func(left, right Port) int {
		return cmp.Compare(left.ID, right.ID)
	})

	for caseIndex := range result.Requirements.OperatingCases {
		operatingCase := &result.Requirements.OperatingCases[caseIndex]
		operatingCase.ID = canonicalIdentifier(operatingCase.ID)
		for conditionIndex := range operatingCase.Conditions {
			condition := &operatingCase.Conditions[conditionIndex]
			condition.Axis = canonicalIdentifier(condition.Axis)
			condition.Target = canonicalIdentifier(condition.Target)
			condition.Unit = canonicalUnit(condition.Unit)
		}
		slices.SortFunc(operatingCase.Conditions, compareOperatingConditions)
		for eventIndex := range operatingCase.Events {
			event := &operatingCase.Events[eventIndex]
			event.ID = canonicalIdentifier(event.ID)
			event.Kind = canonicalIdentifier(event.Kind)
			event.Target = canonicalIdentifier(event.Target)
			event.Unit = canonicalUnit(event.Unit)
		}
		slices.SortFunc(operatingCase.Events, func(left, right OperatingEvent) int {
			return cmp.Or(
				cmp.Compare(left.ID, right.ID),
				cmp.Compare(left.Kind, right.Kind),
				cmp.Compare(left.Target, right.Target),
			)
		})
	}
	slices.SortFunc(result.Requirements.OperatingCases, func(left, right OperatingCase) int {
		return cmp.Compare(left.ID, right.ID)
	})

	for index := range result.Requirements.BehavioralRequirements {
		assertion := &result.Requirements.BehavioralRequirements[index]
		assertion.ID = canonicalIdentifier(assertion.ID)
		assertion.Metric = canonicalIdentifier(assertion.Metric)
		assertion.Analysis = canonicalIdentifier(assertion.Analysis)
		assertion.Unit = canonicalUnit(assertion.Unit)
		assertion.Observation.Kind = canonicalIdentifier(assertion.Observation.Kind)
		assertion.Observation.ID = canonicalIdentifier(assertion.Observation.ID)
		if assertion.Excitation != nil {
			assertion.Excitation.Kind = canonicalIdentifier(assertion.Excitation.Kind)
			assertion.Excitation.ID = canonicalIdentifier(assertion.Excitation.ID)
		}
		for caseIndex := range assertion.OperatingCases {
			assertion.OperatingCases[caseIndex] = canonicalIdentifier(assertion.OperatingCases[caseIndex])
		}
		slices.Sort(assertion.OperatingCases)
		assertion.OperatingCases = slices.Compact(assertion.OperatingCases)
	}
	slices.SortFunc(result.Requirements.BehavioralRequirements, func(left, right BehavioralAssertion) int {
		return cmp.Compare(left.ID, right.ID)
	})
	return result
}

func CanonicalJSON(requirement Requirement) ([]byte, error) {
	return json.Marshal(Normalize(requirement))
}

func CanonicalHash(requirement Requirement) (string, error) {
	data, err := CanonicalJSON(requirement)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func PolicyHash(policy Policy) (string, error) {
	data, err := json.Marshal(policy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func compareOperatingConditions(left, right OperatingCondition) int {
	return cmp.Or(
		cmp.Compare(left.Axis, right.Axis),
		cmp.Compare(left.Target, right.Target),
		cmp.Compare(left.Unit, right.Unit),
		cmp.Compare(left.Min, right.Min),
		cmp.Compare(left.Max, right.Max),
	)
}

func canonicalIdentifier(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	previousUnderscore := false
	for _, current := range value {
		valid := current >= 'a' && current <= 'z' || current >= '0' && current <= '9'
		if valid {
			builder.WriteRune(current)
			previousUnderscore = false
			continue
		}
		if builder.Len() > 0 && !previousUnderscore {
			builder.WriteByte('_')
			previousUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

func canonicalUnit(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "v":
		return "V"
	case "a":
		return "A"
	case "a/v":
		return "A/V"
	case "v/a":
		return "V/A"
	case "hz":
		return "Hz"
	case "s":
		return "s"
	case "ohm", "ω":
		return "ohm"
	case "f":
		return "F"
	case "h":
		return "H"
	case "deg":
		return "deg"
	case "degc", "°c":
		return "degC"
	case "ratio":
		return "ratio"
	case "v_rms":
		return "V_rms"
	case "v_pp":
		return "V_pp"
	case "w":
		return "W"
	case "%":
		return "%"
	default:
		return strings.TrimSpace(value)
	}
}

func cloneRequirement(requirement Requirement) Requirement {
	data, err := json.Marshal(requirement)
	if err != nil {
		return requirement
	}
	var result Requirement
	if err := json.Unmarshal(data, &result); err != nil {
		return requirement
	}
	return result
}
