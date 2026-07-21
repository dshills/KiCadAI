package architecturesearch

import (
	"encoding/json"
	"math"
	"slices"
)

// effectiveObjectiveConstraints projects v3 behavior-level requirements onto
// every objective in the upstream cone of the observed behavior. The
// projection is deliberately capability-neutral: providers may consume the
// common physical constraint names they understand and ignore the rest.
// Explicit objective constraints always take precedence over inferred ones.
func effectiveObjectiveConstraints(requirement Requirement, objective Objective) []Constraint {
	constraints := cloneConstraints(objective.Constraints)
	if requirement.Version != VersionV3 {
		return constraints
	}

	derived := make([]Constraint, 0, len(requirement.Requirements.BehavioralRequirements)*2)
	cones := make(map[string]map[string]bool, len(requirement.Requirements.BehavioralRequirements))
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		cone := upstreamObjectiveCone(requirement, behavior.Observation)
		cones[behavior.ID] = cone
		if !cone[objective.ID] {
			continue
		}
		// Threshold voltage is expressed at the public semantic input, while a
		// threshold detector may consume a conditioned downstream signal. When
		// a cumulative voltage-gain requirement is declared on that exact data
		// path, size the detector's local reference in its own input domain. The
		// public assertion remains unchanged and is still measured by sweeping
		// the external semantic input.
		if behavior.Metric == "threshold_voltage" && objectiveProducesObservation(requirement, objective, behavior.Observation) {
			if threshold, ok := projectedBehaviorTarget(behavior); ok {
				if gain, ok := nearestUpstreamVoltageGain(requirement, objective); ok {
					derived = append(derived, targetConstraint("threshold_voltage", threshold.value*gain.value, behavior.Unit, combinedTolerance(threshold.tolerance, gain.tolerance)))
				}
			}
		}
		derived = append(derived, constraintsFromBehavior(behavior)...)
		derived = append(derived, requiredConstraint("analysis_"+derivedSemanticIdentifier(behavior.Analysis)))
		if behavior.Critical && behavior.Analysis == "startup" {
			derived = append(derived, requiredConstraint("fail_safe_interlock"))
		}
		if behavior.Critical && behavior.Analysis == "thermal" {
			derived = append(derived, requiredConstraint("thermal_tracking"))
		}
	}

	conditionCones := map[string]map[string]bool{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			observation := observationForOperatingTarget(requirement, condition.Target)
			if observation.Kind != "circuit" {
				key := observation.Kind + "\x00" + observation.ID
				cone, exists := conditionCones[key]
				if !exists {
					cone = upstreamObjectiveCone(requirement, observation)
					conditionCones[key] = cone
				}
				if !cone[objective.ID] {
					continue
				}
			}
			derived = append(derived, constraintsFromOperatingCondition(condition)...)
		}
	}

	if coneContainsMetric(requirement, cones, objective.ID, "threshold_current") {
		if threshold, ok := behaviorTarget(requirement.Requirements.BehavioralRequirements, "threshold_current"); ok {
			if transimpedance, ok := behaviorTarget(requirement.Requirements.BehavioralRequirements, "transimpedance"); ok {
				derived = append(derived, targetConstraint("threshold_voltage", threshold.value*transimpedance.value, "V", combinedTolerance(threshold.tolerance, transimpedance.tolerance)))
			}
		}
	}

	return mergeProjectedConstraints(constraints, derived)
}

type projectedTarget struct {
	value     float64
	tolerance float64
}

func projectedBehaviorTarget(behavior BehavioralRequirement) (projectedTarget, bool) {
	constraint, ok := boundedConstraint(behavior.Metric, behavior.Min, behavior.Max, behavior.Unit)
	if !ok {
		return projectedTarget{}, false
	}
	value, tolerance, ok := projectedNumericValue(constraint)
	if !ok {
		return projectedTarget{}, false
	}
	return projectedTarget{value: value, tolerance: tolerance}, true
}

func objectiveProducesObservation(requirement Requirement, objective Objective, observation Observation) bool {
	for _, endpoint := range observationEndpoints(requirement, observation) {
		if objectiveProducesEndpoint(requirement, objective, endpoint) {
			return true
		}
	}
	return false
}

// nearestUpstreamVoltageGain finds the closest declared cumulative gain on a
// directed signal path into objective. A closer observation supersedes a more
// distant one because behavioral gain is measured from the public excitation
// to its observation, rather than being a per-stage multiplier.
func nearestUpstreamVoltageGain(requirement Requirement, objective Objective) (projectedTarget, bool) {
	bestDistance := math.MaxInt
	bestID := ""
	best := projectedTarget{}
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		if behavior.Metric != "voltage_gain" {
			continue
		}
		gain, ok := projectedBehaviorTarget(behavior)
		if !ok || gain.value <= 0 {
			continue
		}
		distance, ok := observationDistanceToObjective(requirement, behavior.Observation, objective.ID)
		if !ok || distance > bestDistance || (distance == bestDistance && bestID != "" && behavior.ID >= bestID) {
			continue
		}
		bestDistance, bestID, best = distance, behavior.ID, gain
	}
	return best, bestID != ""
}

func observationDistanceToObjective(requirement Requirement, observation Observation, targetObjective string) (int, bool) {
	frontier := observationEndpoints(requirement, observation)
	seenEndpoints := map[string]bool{}
	for distance := 0; len(frontier) != 0; distance++ {
		next := []string{}
		for _, endpoint := range frontier {
			if seenEndpoints[endpoint] {
				continue
			}
			seenEndpoints[endpoint] = true
			for _, objective := range requirement.Requirements.Objectives {
				if !objectiveConsumesEndpoint(requirement, objective, endpoint) {
					continue
				}
				if objective.ID == targetObjective {
					return distance, true
				}
				next = append(next, objectiveOutputEndpoints(requirement, objective)...)
			}
		}
		slices.Sort(next)
		frontier = slices.Compact(next)
	}
	return 0, false
}

func objectiveConsumesEndpoint(requirement Requirement, objective Objective, endpoint string) bool {
	return slices.Contains(objectiveInputEndpoints(requirement, objective), endpoint)
}

func objectiveOutputEndpoints(requirement Requirement, objective Objective) []string {
	var endpoints []string
	for _, binding := range objective.Bindings {
		if binding.Signal != "" && (binding.Direction == "source" || binding.Direction == "bidirectional") {
			endpoints = append(endpoints, "signal:"+binding.Signal)
			continue
		}
		if binding.Port == "" {
			continue
		}
		port, ok := requirementPort(requirement, binding.Port)
		if ok && (port.Direction == "source" || port.Direction == "bidirectional") {
			endpoints = append(endpoints, "port:"+binding.Port)
		}
	}
	slices.Sort(endpoints)
	return endpoints
}

func constraintsFromBehavior(behavior BehavioralRequirement) []Constraint {
	constraint, ok := boundedConstraint(behavior.Metric, behavior.Min, behavior.Max, behavior.Unit)
	var result []Constraint
	if !ok && (behavior.Metric == "muted_output_voltage" || behavior.Metric == "startup_output_voltage") && behavior.Min != nil && behavior.Max != nil && *behavior.Min <= 0 && *behavior.Max >= 0 {
		// These metrics are evaluated as peak absolute voltage. Preserve that
		// semantic magnitude when projecting a bipolar public interval instead
		// of inventing an invalid zero-centered percentage target.
		limit := math.Max(math.Abs(*behavior.Min), math.Abs(*behavior.Max))
		constraint = numericProjectedConstraint(behavior.Metric, "maximum", limit, behavior.Unit, nil)
		ok = limit > 0
	}
	if behavior.Critical && behavior.Analysis == "startup" {
		result = append(result, requiredConstraint("startup_isolation"))
	}
	if !ok {
		return result
	}
	result = append(result, constraint)
	switch behavior.Metric {
	case "bandwidth":
		result = append(result, renamedConstraint(constraint, "cutoff_frequency"), equalStringConstraint("response", "low_pass"))
	case "cutoff_frequency":
		result = append(result, equalStringConstraint("response", "low_pass"))
	case "dc_voltage":
		value, _, numeric := projectedNumericValue(constraint)
		if numeric && value > 0 {
			result = append(result, renamedConstraint(constraint, "output_voltage"), renamedConstraint(constraint, "positive_voltage"))
		} else if numeric && value < 0 {
			result = append(result, renamedConstraint(constraint, "negative_voltage"))
		}
	case "hysteresis_voltage":
		result = append(result, renamedConstraint(constraint, "hysteresis_width"))
	case "output_power":
		result = append(result, renamedConstraint(constraint, "continuous_output_power"))
	case "rise_time":
		if behavior.Max != nil && *behavior.Max > 0 {
			maximumRiseTimeS := *behavior.Max
			minimumToggleFrequencyHz := 1 / (2 * maximumRiseTimeS)
			result = append(result, numericProjectedConstraint("bus_frequency", "minimum", minimumToggleFrequencyHz, "Hz", nil))
		}
	}
	return result
}

func constraintsFromOperatingCondition(condition OperatingCondition) []Constraint {
	switch condition.Axis {
	case "load_resistance":
		constraint, ok := boundedConstraint("load_impedance", condition.Min, condition.Max, condition.Unit)
		if !ok {
			return nil
		}
		return []Constraint{constraint}
	case "load_current":
		if condition.Max == nil || *condition.Max <= 0 {
			return nil
		}
		tolerance := operatingRangeTolerance(condition.Min, condition.Max)
		return []Constraint{
			targetConstraint("load_current", *condition.Max, condition.Unit, tolerance),
			targetConstraint("full_scale_current", *condition.Max, condition.Unit, tolerance),
			targetConstraint("output_current", *condition.Max, condition.Unit, tolerance),
		}
	case "load_capacitance":
		constraint, ok := boundedConstraint("load_capacitance", condition.Min, condition.Max, condition.Unit)
		if !ok {
			return nil
		}
		return []Constraint{constraint}
	case "ambient_temperature":
		var constraints []Constraint
		if condition.Min != nil {
			constraints = append(constraints, numericProjectedConstraint("ambient_temperature_minimum", "minimum", *condition.Min, condition.Unit, nil))
		}
		if condition.Max != nil {
			constraints = append(constraints, numericProjectedConstraint("ambient_temperature", "maximum", *condition.Max, condition.Unit, nil))
		}
		return constraints
	default:
		return nil
	}
}

func boundedConstraint(name string, minimum, maximum *float64, unit string) (Constraint, bool) {
	switch {
	case minimum != nil && maximum != nil:
		// A single target-plus-percentage constraint cannot faithfully encode
		// a bipolar interval. Keep the behavioral assertion authoritative and
		// avoid inventing an extremely tight zero-centered target.
		if *minimum < 0 && *maximum > 0 {
			return Constraint{}, false
		}
		value := (*minimum + *maximum) / 2
		tolerance := 1.0
		if value != 0 && *maximum != *minimum {
			tolerance = math.Abs((*maximum-*minimum)/(2*value)) * 100
		}
		return targetConstraint(name, value, unit, math.Min(tolerance, 100)), true
	case minimum != nil:
		return numericProjectedConstraint(name, "minimum", *minimum, unit, nil), true
	case maximum != nil:
		return numericProjectedConstraint(name, "maximum", *maximum, unit, nil), true
	default:
		return Constraint{}, false
	}
}

func targetConstraint(name string, value float64, unit string, tolerance float64) Constraint {
	if tolerance <= 0 {
		tolerance = 1
	}
	return numericProjectedConstraint(name, "target", value, unit, &tolerance)
}

func numericProjectedConstraint(name, relation string, value float64, unit string, tolerance *float64) Constraint {
	encoded, _ := json.Marshal(value)
	return Constraint{Name: name, Relation: relation, Value: encoded, Unit: unit, TolerancePercent: cloneFloat64(tolerance)}
}

func requiredConstraint(name string) Constraint {
	return Constraint{Name: name, Relation: "required", Value: json.RawMessage(`true`)}
}

func equalStringConstraint(name, value string) Constraint {
	encoded, _ := json.Marshal(value)
	return Constraint{Name: name, Relation: "equal", Value: encoded}
}

func renamedConstraint(constraint Constraint, name string) Constraint {
	constraint.Name = name
	constraint.Value = append(json.RawMessage(nil), constraint.Value...)
	constraint.TolerancePercent = cloneFloat64(constraint.TolerancePercent)
	return constraint
}

func mergeProjectedConstraints(explicit, derived []Constraint) []Constraint {
	result := cloneConstraints(explicit)
	seen := make(map[string]bool, len(explicit)+len(derived))
	for _, constraint := range explicit {
		seen[constraint.Name] = true
	}
	for _, constraint := range derived {
		if seen[constraint.Name] {
			continue
		}
		seen[constraint.Name] = true
		result = append(result, constraint)
	}
	normalizeConstraints(result)
	return result
}

func upstreamObjectiveCone(requirement Requirement, observation Observation) map[string]bool {
	frontier := observationEndpoints(requirement, observation)
	result := map[string]bool{}
	for len(frontier) != 0 {
		endpoint := frontier[0]
		frontier = frontier[1:]
		for _, objective := range requirement.Requirements.Objectives {
			if result[objective.ID] || !objectiveProducesEndpoint(requirement, objective, endpoint) {
				continue
			}
			result[objective.ID] = true
			frontier = append(frontier, objectiveInputEndpoints(requirement, objective)...)
		}
	}
	if observation.Kind == "circuit" {
		for _, objective := range requirement.Requirements.Objectives {
			result[objective.ID] = true
		}
	}
	return result
}

func observationEndpoints(requirement Requirement, observation Observation) []string {
	switch observation.Kind {
	case "port":
		return []string{"port:" + observation.ID}
	case "signal":
		return []string{"signal:" + observation.ID}
	case "domain":
		var endpoints []string
		for _, port := range requirement.Requirements.Ports {
			if port.Domain == observation.ID && (port.Direction == "source" || port.Direction == "bidirectional") {
				endpoints = append(endpoints, "port:"+port.ID)
			}
		}
		for _, signal := range requirement.Requirements.Signals {
			if signal.Domain == observation.ID {
				endpoints = append(endpoints, "signal:"+signal.ID)
			}
		}
		slices.Sort(endpoints)
		return endpoints
	default:
		return nil
	}
}

func objectiveProducesEndpoint(requirement Requirement, objective Objective, endpoint string) bool {
	for _, binding := range objective.Bindings {
		if binding.Signal != "" && binding.Direction == "source" && endpoint == "signal:"+binding.Signal {
			return true
		}
		if binding.Port != "" && endpoint == "port:"+binding.Port {
			port, ok := requirementPort(requirement, binding.Port)
			if ok && (port.Direction == "source" || port.Direction == "bidirectional") {
				return true
			}
		}
	}
	return false
}

func objectiveInputEndpoints(requirement Requirement, objective Objective) []string {
	var endpoints []string
	for _, binding := range objective.Bindings {
		if binding.Signal != "" && (binding.Direction == "sink" || binding.Direction == "bidirectional") {
			endpoints = append(endpoints, "signal:"+binding.Signal)
			continue
		}
		if binding.Port == "" {
			continue
		}
		port, ok := requirementPort(requirement, binding.Port)
		if ok && (port.Direction == "sink" || port.Direction == "bidirectional") {
			endpoints = append(endpoints, "port:"+binding.Port)
		}
	}
	slices.Sort(endpoints)
	return endpoints
}

func requirementPort(requirement Requirement, id string) (Port, bool) {
	for _, port := range requirement.Requirements.Ports {
		if port.ID == id {
			return port, true
		}
	}
	return Port{}, false
}

func observationForOperatingTarget(requirement Requirement, target string) Observation {
	for _, port := range requirement.Requirements.Ports {
		if port.ID == target {
			return Observation{Kind: "port", ID: target}
		}
	}
	for _, signal := range requirement.Requirements.Signals {
		if signal.ID == target {
			return Observation{Kind: "signal", ID: target}
		}
	}
	for _, domain := range requirement.Requirements.Domains {
		if domain.ID == target {
			return Observation{Kind: "domain", ID: target}
		}
	}
	return Observation{Kind: "circuit", ID: target}
}

func behaviorTarget(behaviors []BehavioralRequirement, metric string) (projectedTarget, bool) {
	for _, behavior := range behaviors {
		if behavior.Metric != metric {
			continue
		}
		if target, ok := projectedBehaviorTarget(behavior); ok {
			return target, true
		}
	}
	return projectedTarget{}, false
}

func projectedNumericValue(constraint Constraint) (float64, float64, bool) {
	var value float64
	if json.Unmarshal(constraint.Value, &value) != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, 0, false
	}
	tolerance := 0.0
	if constraint.TolerancePercent != nil {
		tolerance = *constraint.TolerancePercent
	}
	return value, tolerance, true
}

func combinedTolerance(left, right float64) float64 {
	combined := math.Sqrt(left*left + right*right)
	if combined == 0 {
		return 1
	}
	return math.Min(combined, 100)
}

func operatingRangeTolerance(minimum, maximum *float64) float64 {
	if minimum == nil || maximum == nil || *maximum == 0 {
		return 1
	}
	return math.Max(1, math.Min(100, math.Abs((*maximum-*minimum)/(*maximum))*100))
}

func coneContainsMetric(requirement Requirement, cones map[string]map[string]bool, objectiveID, metric string) bool {
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		if behavior.Metric == metric && cones[behavior.ID][objectiveID] {
			return true
		}
	}
	return false
}
