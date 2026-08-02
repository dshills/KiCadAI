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
	var sequenceConstraints []Constraint
	for _, constraint := range requirement.Requirements.SystemConstraints {
		switch constraint.Name {
		case "rail_sequence_before", "rail_sequence_delay", "startup_monotonic", "startup_inrush_current", "reference_separation":
			sequenceConstraints = append(sequenceConstraints, constraint)
		}
	}
	constraints = mergeProjectedConstraints(constraints, sequenceConstraints)
	constraints = mergeProjectedConstraints(constraints, controlConstraintsForObjective(requirement, objective))
	if !supportsBehavioralVerification(requirement.Version) {
		return constraints
	}

	derived := make([]Constraint, 0, len(requirement.Requirements.BehavioralRequirements)*2)
	cones := make(map[string]map[string]bool, len(requirement.Requirements.BehavioralRequirements))
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		cone := upstreamBehavioralObjectiveCone(requirement, behavior.Observation)
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
		if behavior.Metric == "threshold_voltage" &&
			(objectiveProducesObservation(requirement, objective, behavior.Observation) || objective.Capability == "threshold_detection") {
			if threshold, ok := projectedBehaviorTarget(behavior); ok {
				if gain, ok := nearestUpstreamVoltageGain(requirement, objective); ok {
					derived = append(derived, targetConstraint("threshold_voltage", threshold.value*gain.value, behavior.Unit, combinedTolerance(threshold.tolerance, gain.tolerance)))
				}
			}
		}
		behaviorConstraints := constraintsFromBehavior(behavior)
		derived = append(derived, behaviorConstraints...)
		if eventConstraint, ok := eventScopedBehaviorConstraint(requirement, behavior); ok {
			derived = append(derived, eventConstraint)
		}
		if durationConstraint, ok := eventScopedDurationConstraint(requirement, behavior); ok {
			derived = append(derived, durationConstraint)
		}
		for _, role := range objectiveRolesForObservation(requirement, objective, behavior.Observation) {
			derived = append(derived, roleScopedConstraints(role, behaviorConstraints)...)
		}
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
			roles := objectiveRolesForObservation(requirement, objective, observation)
			var sharedPowerDomainRoles []string
			if condition.Axis == "load_current" {
				sharedPowerDomainRoles = objectiveRolesForPowerDomainObservation(requirement, objective, observation)
				roles = append(roles, sharedPowerDomainRoles...)
				slices.Sort(roles)
				roles = slices.Compact(roles)
			}
			if condition.Axis == "supply_voltage" {
				roles = objectiveRolesForSupplyTarget(requirement, objective, condition.Target)
				if len(roles) == 0 {
					continue
				}
			} else if observation.Kind != "circuit" {
				key := observation.Kind + "\x00" + observation.ID
				cone, exists := conditionCones[key]
				if !exists {
					cone = upstreamBehavioralObjectiveCone(requirement, observation)
					conditionCones[key] = cone
				}
				if !cone[objective.ID] && len(sharedPowerDomainRoles) == 0 {
					continue
				}
			}
			conditionConstraints := constraintsFromOperatingCondition(condition)
			derived = append(derived, conditionConstraints...)
			for _, role := range roles {
				derived = append(derived, roleScopedConstraints(role, conditionConstraints)...)
			}
		}
		for _, event := range operatingCase.Events {
			if event.Unit != "A" || event.Target.Kind == "" || event.Target.ID == "" {
				continue
			}
			if event.Applied == nil {
				continue
			}
			peak := math.Abs(*event.Applied)
			if event.Initial != nil {
				peak = math.Max(peak, math.Abs(*event.Initial))
			}
			if event.Recovered != nil {
				peak = math.Max(peak, math.Abs(*event.Recovered))
			}
			for _, behavior := range requirement.Requirements.BehavioralRequirements {
				if behavior.Metric != "peak_device_current" || behavior.Max == nil ||
					behavior.Observation.Kind != event.Target.Kind || behavior.Observation.ID != event.Target.ID ||
					(len(behavior.OperatingCases) > 0 && !slices.Contains(behavior.OperatingCases, operatingCase.ID)) {
					continue
				}
				peak = math.Min(peak, math.Abs(*behavior.Max))
			}
			if peak <= 0 {
				continue
			}
			observation := Observation{Kind: event.Target.Kind, ID: event.Target.ID}
			cone := upstreamBehavioralObjectiveCone(requirement, observation)
			roles := objectiveRolesForPowerDomainObservation(requirement, objective, observation)
			if !cone[objective.ID] && len(roles) == 0 {
				continue
			}
			stress := numericProjectedConstraint("transient_load_current", "minimum", peak, event.Unit, nil)
			derived = append(derived, stress)
			for _, role := range roles {
				derived = append(derived, roleScopedConstraints(role, []Constraint{stress})...)
			}
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

func controlConstraintsForObjective(requirement Requirement, objective Objective) []Constraint {
	if requirement.Version != VersionV6 {
		return nil
	}
	var constraints []Constraint
	for _, binding := range objective.Bindings {
		control := controlForBinding(requirement, binding)
		if control == nil {
			continue
		}
		constraints = append(constraints,
			equalStringConstraint("control_function", control.Function),
			equalStringConstraint("control_polarity", control.Polarity),
			equalStringConstraint("control_startup_state", control.StartupState),
			equalStringConstraint("control_safe_state", control.SafeState),
		)
		port, hasPort := requirementPort(requirement, binding.Port)
		if binding.Direction == "source" || (hasPort && port.Direction == "source") {
			constraints = append(constraints,
				equalStringConstraint("output_polarity", control.Polarity),
				Constraint{Name: "inactive_at_power_up", Relation: "required", Value: json.RawMessage([]byte("true"))},
			)
		}
		if controlRole(binding.Role) && (binding.Direction == "sink" || binding.Port != "") {
			constraints = append(constraints, equalStringConstraint("control_active_state", physicalControlAction(*control)))
		}
	}
	return constraints
}

func controlForBinding(requirement Requirement, binding Binding) *ControlSemantics {
	if binding.Signal != "" {
		for index := range requirement.Requirements.Signals {
			if requirement.Requirements.Signals[index].ID == binding.Signal {
				return requirement.Requirements.Signals[index].Control
			}
		}
	}
	if binding.Port != "" {
		for index := range requirement.Requirements.Ports {
			if requirement.Requirements.Ports[index].ID == binding.Port {
				return requirement.Requirements.Ports[index].Control
			}
		}
	}
	return nil
}

func physicalControlAction(control ControlSemantics) string {
	assertionConnects := control.Function == "enable" || control.Function == "power_good" || control.Function == "state"
	assertedHigh := control.Polarity == "active_high"
	if assertionConnects {
		if assertedHigh {
			return "high"
		}
		return "high_disconnect"
	}
	if assertedHigh {
		return "high_disconnect"
	}
	return "low_disconnect"
}

func eventScopedDurationConstraint(requirement Requirement, behavior BehavioralRequirement) (Constraint, bool) {
	if behavior.Observation.Kind != "event" || behavior.Metric != "transient_soa_margin" {
		return Constraint{}, false
	}
	operatingCases := make(map[string]bool, len(behavior.OperatingCases))
	for _, operatingCase := range behavior.OperatingCases {
		operatingCases[operatingCase] = true
	}
	durationS := 0.0
	found := false
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		if len(operatingCases) != 0 && !operatingCases[operatingCase.ID] {
			continue
		}
		for _, event := range operatingCase.Events {
			if event.ID != behavior.Observation.ID || event.DurationS <= 0 {
				continue
			}
			durationS = math.Max(durationS, event.DurationS)
			found = true
		}
	}
	if !found {
		return Constraint{}, false
	}
	return targetConstraint("transient_soa_duration", durationS, "s", 0), true
}

func eventScopedBehaviorConstraint(requirement Requirement, behavior BehavioralRequirement) (Constraint, bool) {
	if behavior.Observation.Kind != "event" {
		return Constraint{}, false
	}
	constraint, ok := boundedConstraint(behavior.Metric, behavior.Min, behavior.Max, behavior.Unit)
	if !ok {
		return Constraint{}, false
	}
	eventKind := ""
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, event := range operatingCase.Events {
			if event.ID != behavior.Observation.ID {
				continue
			}
			if eventKind != "" && eventKind != event.Kind {
				return Constraint{}, false
			}
			eventKind = event.Kind
		}
	}
	eventKind = derivedSemanticIdentifier(eventKind)
	if eventKind == "" {
		return Constraint{}, false
	}
	name := eventKind + "_" + derivedSemanticIdentifier(behavior.Metric)
	if behavior.Metric == "sequence_delay" {
		name = eventKind + "_delay"
	}
	return renamedConstraint(constraint, name), true
}

func objectiveRolesForPowerDomainObservation(requirement Requirement, objective Objective, observation Observation) []string {
	domain, ok := observationPowerDomain(requirement, observation)
	if !ok {
		return nil
	}
	var roles []string
	for _, binding := range objective.Bindings {
		bindingDomain, bindingOK := powerBindingDomain(requirement, binding)
		if bindingOK && bindingDomain == domain && binding.Role != "" {
			roles = append(roles, binding.Role)
		}
	}
	slices.Sort(roles)
	return slices.Compact(roles)
}

func observationPowerDomain(requirement Requirement, observation Observation) (string, bool) {
	switch observation.Kind {
	case "port":
		for _, port := range requirement.Requirements.Ports {
			if port.ID != observation.ID || port.Domain == "" {
				continue
			}
			for _, domain := range requirement.Requirements.Domains {
				if domain.ID == port.Domain && domain.Kind == "supply" {
					return port.Domain, true
				}
			}
		}
	case "signal":
		for _, signal := range requirement.Requirements.Signals {
			if signal.ID == observation.ID && signal.Kind == "power" && signal.Domain != "" {
				return signal.Domain, true
			}
		}
	case "domain":
		for _, domain := range requirement.Requirements.Domains {
			if domain.ID == observation.ID && domain.Kind == "supply" {
				return domain.ID, true
			}
		}
	}
	return "", false
}

func objectiveRolesForSupplyTarget(requirement Requirement, objective Objective, target string) []string {
	var roles []string
	for _, binding := range objective.Bindings {
		matches := binding.Port == target || binding.Signal == target
		if !matches && binding.Port != "" {
			if port, ok := requirementPort(requirement, binding.Port); ok {
				matches = port.Domain == target
			}
		}
		if !matches && binding.Signal != "" {
			for _, signal := range requirement.Requirements.Signals {
				if signal.ID == binding.Signal && signal.Domain == target {
					matches = true
					break
				}
			}
		}
		if matches && binding.Role != "" {
			roles = append(roles, binding.Role)
		}
	}
	slices.Sort(roles)
	return slices.Compact(roles)
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

func objectiveRolesForObservation(requirement Requirement, objective Objective, observation Observation) []string {
	endpoints := observationEndpoints(requirement, observation)
	var roles []string
	for _, binding := range objective.Bindings {
		endpoint := ""
		switch {
		case binding.Port != "":
			endpoint = "port:" + binding.Port
		case binding.Signal != "":
			endpoint = "signal:" + binding.Signal
		default:
			continue
		}
		if binding.Role != "" && slices.Contains(endpoints, endpoint) {
			roles = append(roles, binding.Role)
		}
	}
	slices.Sort(roles)
	return slices.Compact(roles)
}

func roleScopedConstraints(role string, constraints []Constraint) []Constraint {
	if role == "" {
		return nil
	}
	result := make([]Constraint, 0, len(constraints))
	for _, constraint := range constraints {
		result = append(result, renamedConstraint(constraint, role+"_"+constraint.Name))
	}
	return result
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
		load, ok := boundedConstraint("load_current", condition.Min, condition.Max, condition.Unit)
		if !ok {
			return nil
		}
		return []Constraint{
			load,
			numericProjectedConstraint("full_scale_current", "minimum", *condition.Max, condition.Unit, nil),
			numericProjectedConstraint("output_current", "minimum", *condition.Max, condition.Unit, nil),
		}
	case "load_capacitance":
		constraint, ok := boundedConstraint("load_capacitance", condition.Min, condition.Max, condition.Unit)
		if !ok {
			return nil
		}
		return []Constraint{constraint}
	case "supply_voltage":
		constraint, ok := boundedConstraint("supply_voltage", condition.Min, condition.Max, condition.Unit)
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
	explicitNames := make(map[string]bool, len(explicit))
	for _, constraint := range explicit {
		explicitNames[constraint.Name] = true
	}
	derivedIndices := make(map[string]int, len(derived))
	for _, constraint := range derived {
		if explicitNames[constraint.Name] {
			continue
		}
		if index, exists := derivedIndices[constraint.Name]; exists {
			if merged, ok := mergeDerivedConstraintEnvelope(result[index], constraint); ok {
				result[index] = merged
			}
			continue
		}
		derivedIndices[constraint.Name] = len(result)
		result = append(result, constraint)
	}
	normalizeConstraints(result)
	return result
}

func mergeDerivedConstraintEnvelope(left, right Constraint) (Constraint, bool) {
	if left.Name == "" || left.Name != right.Name || left.Unit != right.Unit || left.Relation != right.Relation {
		return Constraint{}, false
	}
	leftValue, _, leftOK := projectedNumericValue(left)
	rightValue, _, rightOK := projectedNumericValue(right)
	if !leftOK || !rightOK {
		return Constraint{}, false
	}
	switch left.Relation {
	case "minimum":
		return numericProjectedConstraint(left.Name, left.Relation, math.Max(leftValue, rightValue), left.Unit, nil), true
	case "maximum":
		return numericProjectedConstraint(left.Name, left.Relation, math.Min(leftValue, rightValue), left.Unit, nil), true
	case "target":
		leftTolerance, rightTolerance := 0.0, 0.0
		if left.TolerancePercent != nil {
			leftTolerance = *left.TolerancePercent
		}
		if right.TolerancePercent != nil {
			rightTolerance = *right.TolerancePercent
		}
		leftRadius := math.Abs(leftValue) * leftTolerance / 100
		rightRadius := math.Abs(rightValue) * rightTolerance / 100
		leftMinimum, leftMaximum := leftValue-leftRadius, leftValue+leftRadius
		rightMinimum, rightMaximum := rightValue-rightRadius, rightValue+rightRadius
		if leftMaximum < rightMinimum || rightMaximum < leftMinimum {
			// Derived targets are appended from most-local to most-public.
			// Disjoint intervals can represent distinct semantic domains (for
			// example a conditioned detector threshold and its external input
			// threshold), so retain the first instead of inventing a midpoint.
			return left, true
		}
		minimum := math.Min(leftMinimum, rightMinimum)
		maximum := math.Max(leftMaximum, rightMaximum)
		center := (minimum + maximum) / 2
		if center == 0 {
			return left, true
		}
		tolerance := 100 * math.Abs(maximum-minimum) / (2 * math.Abs(center))
		if tolerance > 100 {
			return left, true
		}
		return numericProjectedConstraint(left.Name, left.Relation, center, left.Unit, &tolerance), true
	default:
		return Constraint{}, false
	}
}

func upstreamObjectiveCone(requirement Requirement, observation Observation) map[string]bool {
	if observation.Kind == "event" {
		// An event assertion measures the complete candidate response to a
		// declared disturbance. The disturbance may enter at an input or load
		// while the measured recovery depends on downstream and feedback
		// objectives, so every objective must see the behavior-level contract.
		result := make(map[string]bool, len(requirement.Requirements.Objectives))
		for _, objective := range requirement.Requirements.Objectives {
			result[objective.ID] = true
		}
		return result
	}
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

// upstreamBehavioralObjectiveCone follows only inputs that can carry the
// observed physical behavior. Supervisory, bias, reference, and supply
// dependencies still participate in hierarchy and composition, but they must
// not project an output voltage, load, gain, or timing bound into sibling
// signal or power paths.
func upstreamBehavioralObjectiveCone(requirement Requirement, observation Observation) map[string]bool {
	if observation.Kind == "event" {
		return upstreamObjectiveCone(requirement, observation)
	}
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
			frontier = append(frontier, behavioralObjectiveInputEndpoints(requirement, objective)...)
		}
	}
	if observation.Kind == "circuit" {
		for _, objective := range requirement.Requirements.Objectives {
			result[objective.ID] = true
		}
	}
	return result
}

func behavioralObjectiveInputEndpoints(requirement Requirement, objective Objective) []string {
	var endpoints []string
	for _, binding := range objective.Bindings {
		switch binding.Role {
		case "bias", "control", "enable", "fault", "interlock", "negative_power", "permit", "positive_power", "power", "reference", "trip":
			continue
		}
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
