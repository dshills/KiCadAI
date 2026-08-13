package corpusfreezev9

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	ots "kicadai/internal/opentopologysynthesis"
)

func policyHash(policy Policy) (string, error) {
	data, err := json.Marshal(policy)
	if err != nil {
		return "", fmt.Errorf("marshal V9 policy: %w", err)
	}
	return hashBytes(data), nil
}

func semanticHashes(requirement ots.Requirement) (string, string, error) {
	data, err := json.Marshal(requirement)
	if err != nil {
		return "", "", err
	}
	var clone ots.Requirement
	if err := json.Unmarshal(data, &clone); err != nil {
		return "", "", err
	}
	clone.Project = ots.Project{}
	neutral, err := ots.CanonicalHash(clone)
	if err != nil {
		return "", "", fmt.Errorf("neutral semantic hash: %w", err)
	}

	domainReferences := map[string]string{}
	for index := range clone.Requirements.Domains {
		domain := &clone.Requirements.Domains[index]
		oldID := domain.ID
		domain.ID = ""
		descriptor, err := canonicalDescriptor(*domain)
		if err != nil {
			return "", "", err
		}
		domainReferences[oldID] = "domain:" + hashBytes([]byte(descriptor))
	}
	slices.SortFunc(clone.Requirements.Domains, compareDomain)

	portReferences := map[string]string{}
	for index := range clone.Requirements.Ports {
		port := &clone.Requirements.Ports[index]
		oldID := port.ID
		port.ID = ""
		port.Domain = domainReferences[port.Domain]
		descriptor, err := canonicalDescriptor(*port)
		if err != nil {
			return "", "", err
		}
		portReferences[oldID] = "port:" + hashBytes([]byte(descriptor))
	}
	slices.SortFunc(clone.Requirements.Ports, comparePort)

	caseReferences := map[string]string{}
	for index := range clone.Requirements.OperatingCases {
		operatingCase := &clone.Requirements.OperatingCases[index]
		oldID := operatingCase.ID
		operatingCase.ID = ""
		for conditionIndex := range operatingCase.Conditions {
			condition := &operatingCase.Conditions[conditionIndex]
			condition.Target = normalizedReference(condition.Target, domainReferences, portReferences)
		}
		slices.SortFunc(operatingCase.Conditions, compareCondition)
		for eventIndex := range operatingCase.Events {
			event := &operatingCase.Events[eventIndex]
			event.ID = ""
			event.Target = normalizedReference(event.Target, domainReferences, portReferences)
		}
		slices.SortFunc(operatingCase.Events, compareEvent)
		descriptor, err := canonicalDescriptor(*operatingCase)
		if err != nil {
			return "", "", err
		}
		caseReferences[oldID] = "case:" + hashBytes([]byte(descriptor))
	}
	slices.SortFunc(clone.Requirements.OperatingCases, compareOperatingCase)

	for index := range clone.Requirements.BehavioralRequirements {
		assertion := &clone.Requirements.BehavioralRequirements[index]
		assertion.ID = ""
		if err := normalizeObservation(assertion.Excitation, domainReferences, portReferences); err != nil {
			return "", "", err
		}
		if err := normalizeObservation(&assertion.Observation, domainReferences, portReferences); err != nil {
			return "", "", err
		}
		for caseIndex, id := range assertion.OperatingCases {
			value, exists := caseReferences[id]
			if !exists {
				return "", "", fmt.Errorf("normalize assertion: unresolved operating case %q", id)
			}
			assertion.OperatingCases[caseIndex] = value
		}
		sort.Strings(assertion.OperatingCases)
	}
	slices.SortFunc(clone.Requirements.BehavioralRequirements, compareAssertion)
	normalizedJSON, err := json.Marshal(clone)
	if err != nil {
		return "", "", err
	}
	return neutral, hashBytes(normalizedJSON), nil
}

func normalizedReference(id string, domains, ports map[string]string) string {
	if value, ok := domains[id]; ok {
		return value
	}
	if value, ok := ports[id]; ok {
		return value
	}
	return id
}

func normalizeObservation(observation *ots.Observation, domains, ports map[string]string) error {
	if observation == nil {
		return nil
	}
	switch observation.Kind {
	case "domain":
		value, exists := domains[observation.ID]
		if !exists {
			return fmt.Errorf("normalize observation: unresolved domain %q", observation.ID)
		}
		observation.ID = value
	case "port":
		value, exists := ports[observation.ID]
		if !exists {
			return fmt.Errorf("normalize observation: unresolved port %q", observation.ID)
		}
		observation.ID = value
	case "circuit":
		observation.ID = "circuit"
	default:
		return fmt.Errorf("normalize observation: unsupported kind %q", observation.Kind)
	}
	return nil
}

func canonicalDescriptor(value any) (string, error) {
	data, err := json.Marshal(value)
	return string(data), err
}

func compareDomain(left, right ots.Domain) int {
	return cmp.Or(cmp.Compare(left.ID, right.ID), cmp.Compare(left.Kind, right.Kind),
		compareOptionalFloat(left.MinVoltageV, right.MinVoltageV), compareOptionalFloat(left.NominalVoltageV, right.NominalVoltageV),
		compareOptionalFloat(left.MaxVoltageV, right.MaxVoltageV), compareOptionalFloat(left.MaxCurrentA, right.MaxCurrentA), cmp.Compare(left.Source, right.Source))
}

func comparePort(left, right ots.Port) int {
	return cmp.Or(cmp.Compare(left.ID, right.ID), cmp.Compare(left.Kind, right.Kind), cmp.Compare(left.Direction, right.Direction),
		cmp.Compare(left.Domain, right.Domain), compareElectrical(left.Electrical, right.Electrical))
}

func compareElectrical(left, right ots.Electrical) int {
	return cmp.Or(compareOptionalFloat(left.MinVoltageV, right.MinVoltageV), compareOptionalFloat(left.NominalVoltageV, right.NominalVoltageV),
		compareOptionalFloat(left.MaxVoltageV, right.MaxVoltageV), compareOptionalFloat(left.MaxCurrentA, right.MaxCurrentA),
		compareOptionalFloat(left.InputImpedanceMinOhm, right.InputImpedanceMinOhm), cmp.Compare(left.DefaultState, right.DefaultState))
}

func compareCondition(left, right ots.OperatingCondition) int {
	return cmp.Or(cmp.Compare(left.Axis, right.Axis), cmp.Compare(left.Target, right.Target), cmp.Compare(left.Min, right.Min),
		cmp.Compare(left.Max, right.Max), cmp.Compare(left.Unit, right.Unit))
}

func compareEvent(left, right ots.OperatingEvent) int {
	return cmp.Or(cmp.Compare(left.ID, right.ID), cmp.Compare(left.Kind, right.Kind), cmp.Compare(left.Target, right.Target),
		cmp.Compare(left.TriggerTimeS, right.TriggerTimeS), cmp.Compare(left.Initial, right.Initial), cmp.Compare(left.Applied, right.Applied), cmp.Compare(left.Unit, right.Unit))
}

func compareOperatingCase(left, right ots.OperatingCase) int {
	return cmp.Or(cmp.Compare(left.ID, right.ID), slices.CompareFunc(left.Conditions, right.Conditions, compareCondition),
		slices.CompareFunc(left.Events, right.Events, compareEvent))
}

func compareAssertion(left, right ots.BehavioralAssertion) int {
	return cmp.Or(cmp.Compare(left.ID, right.ID), cmp.Compare(left.Metric, right.Metric), cmp.Compare(left.Analysis, right.Analysis),
		compareOptionalObservation(left.Excitation, right.Excitation), compareObservation(left.Observation, right.Observation),
		compareOptionalFloat(left.Min, right.Min), compareOptionalFloat(left.Max, right.Max), cmp.Compare(left.Unit, right.Unit),
		slices.Compare(left.OperatingCases, right.OperatingCases), compareOptionalFloat(left.FrequencyHz, right.FrequencyHz), compareBool(left.Critical, right.Critical))
}

func compareBool(left, right bool) int {
	if left == right {
		return 0
	}
	if !left {
		return -1
	}
	return 1
}

func compareObservation(left, right ots.Observation) int {
	return cmp.Or(cmp.Compare(left.Kind, right.Kind), cmp.Compare(left.ID, right.ID))
}

func compareOptionalObservation(left, right *ots.Observation) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	return compareObservation(*left, *right)
}

func compareOptionalFloat(left, right *float64) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	return cmp.Compare(*left, *right)
}
