package opentopologysynthesis

import (
	"context"
	"fmt"
	"math"
)

// EvaluateCandidateV18 applies the bounded V18 DC input-impedance probe while
// retaining the V17 evaluator for every authored analysis and physical model.
func EvaluateCandidateV18(
	ctx context.Context,
	requirement Requirement,
	graph CandidateGraph,
	trial *ValueTrial,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	policy Policy,
) SimulationEvaluation {
	original := Normalize(requirement)
	originalHash, err := CanonicalHash(original)
	if err != nil {
		return EvaluateCandidateV17(ctx, original, graph, trial, inventory, environment, policy)
	}
	probeRequirement, originalCaseByProbe := v18InputImpedanceProbeRequirement(original)
	result := EvaluateCandidateV17(ctx, probeRequirement, graph, trial, inventory, environment, policy)
	result.RequirementHash = originalHash
	for index := range result.Attempts {
		if originalCase, found := originalCaseByProbe[result.Attempts[index].OperatingCase]; found {
			result.Attempts[index].OperatingCase = originalCase
		}
	}
	for index := range result.Diagnoses {
		if originalCase, found := originalCaseByProbe[result.Diagnoses[index].OperatingCase]; found {
			result.Diagnoses[index].OperatingCase = originalCase
		}
	}
	return finalizeSimulationEvaluationV17(result)
}

func v18InputImpedanceProbeRequirement(requirement Requirement) (Requirement, map[string]string) {
	result := cloneRequirement(Normalize(requirement))
	caseByID := make(map[string]OperatingCase, len(result.Requirements.OperatingCases))
	usedIDs := make(map[string]bool, len(result.Requirements.OperatingCases))
	for _, operatingCase := range result.Requirements.OperatingCases {
		caseByID[operatingCase.ID] = operatingCase
		usedIDs[operatingCase.ID] = true
	}
	originalCaseByProbe := map[string]string{}
	for assertionIndex := range result.Requirements.BehavioralRequirements {
		assertion := &result.Requirements.BehavioralRequirements[assertionIndex]
		if assertion.Analysis != "dc_operating_point" || assertion.Metric != "input_impedance" ||
			assertion.Observation.Kind != "port" {
			continue
		}
		probeCases := make([]string, 0, len(assertion.OperatingCases))
		for _, caseID := range assertion.OperatingCases {
			operatingCase, found := caseByID[caseID]
			if !found {
				probeCases = append(probeCases, caseID)
				continue
			}
			probe, axis, found := v18DCInputImpedanceProbeValue(result, operatingCase, assertion.Observation.ID)
			if !found {
				probeCases = append(probeCases, caseID)
				continue
			}
			probeCase := operatingCase
			probeCase.Conditions = append([]OperatingCondition(nil), operatingCase.Conditions...)
			replaced := false
			for conditionIndex := range probeCase.Conditions {
				condition := &probeCase.Conditions[conditionIndex]
				if condition.Target == assertion.Observation.ID &&
					(condition.Axis == "input_voltage" || condition.Axis == "control_voltage") {
					condition.Min, condition.Max = probe, probe
					replaced = true
				}
			}
			if !replaced {
				probeCase.Conditions = append(probeCase.Conditions, OperatingCondition{
					Axis: axis, Target: assertion.Observation.ID, Min: probe, Max: probe, Unit: "V",
				})
			}
			probeCase.ID = v18UniqueProbeCaseID(usedIDs)
			usedIDs[probeCase.ID] = true
			result.Requirements.OperatingCases = append(result.Requirements.OperatingCases, probeCase)
			originalCaseByProbe[probeCase.ID] = caseID
			probeCases = append(probeCases, probeCase.ID)
		}
		assertion.OperatingCases = probeCases
	}
	return Normalize(result), originalCaseByProbe
}

func v18DCInputImpedanceProbeValue(
	requirement Requirement,
	operatingCase OperatingCase,
	portID string,
) (float64, string, bool) {
	for _, condition := range operatingCase.Conditions {
		if condition.Target != portID ||
			(condition.Axis != "input_voltage" && condition.Axis != "control_voltage") {
			continue
		}
		if probe, found := v18BoundedNonzeroProbe(condition.Min, condition.Max); found {
			return probe, condition.Axis, true
		}
		return 0, "", false
	}
	for _, port := range requirement.Requirements.Ports {
		if port.ID != portID || port.Electrical.MinVoltageV == nil || port.Electrical.MaxVoltageV == nil {
			continue
		}
		probe, found := v18BoundedNonzeroProbe(*port.Electrical.MinVoltageV, *port.Electrical.MaxVoltageV)
		return probe, "input_voltage", found
	}
	return 0, "", false
}

func v18BoundedNonzeroProbe(minimum, maximum float64) (float64, bool) {
	if minimum > 0 || maximum < 0 || minimum == maximum {
		return 0, false
	}
	magnitude := math.Min(1e-3, (maximum-minimum)/2)
	if magnitude <= 0 || !finite(magnitude) {
		return 0, false
	}
	if maximum > 0 {
		return magnitude, true
	}
	if minimum < 0 {
		return -magnitude, true
	}
	return 0, false
}

func v18UniqueProbeCaseID(used map[string]bool) string {
	for suffix := 0; ; suffix++ {
		candidate := fmt.Sprintf("v18_probe_%02d", suffix)
		if !used[candidate] {
			return candidate
		}
	}
}
