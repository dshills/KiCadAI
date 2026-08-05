package opentopologysynthesis

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

var semanticIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

var allowedPortKinds = []string{
	"analog_current",
	"analog_voltage",
	"controlled_current",
	"digital",
	"power",
	"reference",
}

var allowedDirections = []string{"bidirectional", "sink", "source"}

var allowedAnalyses = []string{
	"ac_sweep",
	"dc_operating_point",
	"dc_sweep",
	"distortion",
	"electrothermal",
	"noise",
	"stability",
	"startup",
	"thermal",
	"transient",
}

var allowedMetrics = []string{
	"bandwidth",
	"cutoff_frequency",
	"dc_current",
	"dc_voltage",
	"duty_cycle",
	"fall_time",
	"falling_threshold",
	"hysteresis",
	"input_impedance",
	"junction_temperature",
	"line_regulation",
	"load_regulation",
	"lower_threshold",
	"off_state_current",
	"on_state_voltage",
	"oscillation_frequency",
	"output_current",
	"output_high_voltage",
	"output_low_voltage",
	"output_noise_rms",
	"output_power",
	"output_ripple",
	"output_swing",
	"output_voltage",
	"peak_current",
	"peak_voltage",
	"phase_margin",
	"propagation_delay",
	"quiescent_current",
	"rise_time",
	"rising_threshold",
	"settling_time",
	"soa_margin",
	"startup_current",
	"startup_output_voltage",
	"startup_overshoot",
	"thd",
	"threshold_current",
	"threshold_voltage",
	"total_harmonic_distortion",
	"transconductance",
	"transimpedance",
	"upper_threshold",
	"voltage_gain",
	"voltage_gain_at_frequency",
	"conversion_efficiency",
}

var allowedUnits = []string{
	"%",
	"A",
	"A/V",
	"F",
	"H",
	"Hz",
	"V",
	"V/A",
	"V_pp",
	"V_rms",
	"W",
	"deg",
	"degC",
	"ohm",
	"ratio",
	"s",
}

var allowedConditionAxes = []string{
	"ambient_temperature",
	"input_current",
	"input_voltage",
	"load_capacitance",
	"load_current",
	"load_inductance",
	"load_resistance",
	"model_corner",
	"supply_voltage",
	"tolerance_corner",
}

var allowedEventKinds = []string{
	"input_step",
	"load_step",
	"power_step",
	"rail_loss",
	"short_circuit",
	"startup",
}

func Validate(requirement Requirement) []reports.Issue {
	validator := requirementValidator{
		requirement: requirement,
		domains:     map[string]Domain{},
		ports:       map[string]Port{},
		cases:       map[string]OperatingCase{},
		assertions:  map[string]bool{},
	}
	validator.header()
	validator.validateDomains()
	validator.validatePorts()
	validator.validateCases()
	validator.validateAssertions()
	validator.validateBoardLimits()
	validator.validateAcceptance()
	return reports.SortedIssues(validator.issues)
}

type requirementValidator struct {
	requirement Requirement
	issues      []reports.Issue
	domains     map[string]Domain
	ports       map[string]Port
	cases       map[string]OperatingCase
	assertions  map[string]bool
}

func (validator *requirementValidator) add(path, message string) {
	validator.issues = append(validator.issues, requirementIssue(path, message))
}

func (validator *requirementValidator) header() {
	if validator.requirement.Schema != RequirementSchema || validator.requirement.Version != RequirementVersion {
		validator.add("schema", fmt.Sprintf("schema/version must be %q/%d", RequirementSchema, RequirementVersion))
	}
	validator.text("project.name", validator.requirement.Project.Name, 1, 64)
	if !semanticIDPattern.MatchString(validator.requirement.Project.Name) {
		validator.add("project.name", "project name must be a normalized semantic identifier")
	}
	validator.text("project.title", validator.requirement.Project.Title, 1, 256)
	validator.text("project.description", validator.requirement.Project.Description, 1, MaxTextBytes)
}

func (validator *requirementValidator) validateDomains() {
	domains := validator.requirement.Requirements.Domains
	if len(domains) < 2 || len(domains) > MaxDomains {
		validator.add("requirements.domains", fmt.Sprintf("domain count must be between 2 and %d", MaxDomains))
	}
	references := 0
	for index, domain := range domains {
		path := fmt.Sprintf("requirements.domains[%d]", index)
		validator.semanticID(path+".id", domain.ID)
		if _, duplicate := validator.domains[domain.ID]; duplicate {
			validator.add(path+".id", "domain ID must be unique")
		}
		validator.domains[domain.ID] = domain
		if domain.Kind != "reference" && domain.Kind != "supply" {
			validator.add(path+".kind", "domain kind must be reference or supply")
		}
		if domain.Source != "external" {
			validator.add(path+".source", "open-topology requirements may declare only external domains")
		}
		if domain.Kind == "reference" {
			references++
		}
		validator.orderedElectrical(path, domain.MinVoltageV, domain.NominalVoltageV, domain.MaxVoltageV)
		validator.optionalPositive(path+".max_current_a", domain.MaxCurrentA, 1_000)
	}
	if references == 0 {
		validator.add("requirements.domains", "at least one reference domain is required")
	}
}

func (validator *requirementValidator) validatePorts() {
	ports := validator.requirement.Requirements.Ports
	if len(ports) < 2 || len(ports) > MaxPorts {
		validator.add("requirements.ports", fmt.Sprintf("port count must be between 2 and %d", MaxPorts))
	}
	outputs := 0
	excitations := 0
	for index, port := range ports {
		path := fmt.Sprintf("requirements.ports[%d]", index)
		validator.semanticID(path+".id", port.ID)
		if _, duplicate := validator.ports[port.ID]; duplicate {
			validator.add(path+".id", "port ID must be unique")
		}
		validator.ports[port.ID] = port
		if !slices.Contains(allowedPortKinds, port.Kind) {
			validator.add(path+".kind", "unsupported semantic port kind")
		}
		if !slices.Contains(allowedDirections, port.Direction) {
			validator.add(path+".direction", "direction must be sink, source, or bidirectional")
		}
		if _, exists := validator.domains[port.Domain]; !exists {
			validator.add(path+".domain", "port domain must refer to a declared domain")
		}
		if port.Direction == "source" || port.Kind == "controlled_current" {
			outputs++
		}
		if port.Direction == "sink" && port.Kind != "power" && port.Kind != "reference" && port.Kind != "controlled_current" {
			excitations++
		}
		validator.orderedElectrical(path+".electrical", port.Electrical.MinVoltageV, port.Electrical.NominalVoltageV, port.Electrical.MaxVoltageV)
		validator.optionalPositive(path+".electrical.max_current_a", port.Electrical.MaxCurrentA, 1_000)
		validator.optionalPositive(path+".electrical.input_impedance_min_ohm", port.Electrical.InputImpedanceMinOhm, 1e15)
		if port.Electrical.DefaultState != "" && port.Electrical.DefaultState != "low" && port.Electrical.DefaultState != "high" && port.Electrical.DefaultState != "floating" {
			validator.add(path+".electrical.default_state", "default state must be low, high, or floating")
		}
	}
	if outputs == 0 {
		validator.add("requirements.ports", "at least one source port is required")
	}
	if excitations == 0 {
		validator.add("requirements.ports", "at least one non-power sink port is required")
	}
}

func (validator *requirementValidator) validateCases() {
	cases := validator.requirement.Requirements.OperatingCases
	if len(cases) == 0 || len(cases) > MaxOperatingCases {
		validator.add("requirements.operating_cases", fmt.Sprintf("operating-case count must be between 1 and %d", MaxOperatingCases))
	}
	eventCount := 0
	for caseIndex, operatingCase := range cases {
		path := fmt.Sprintf("requirements.operating_cases[%d]", caseIndex)
		validator.semanticID(path+".id", operatingCase.ID)
		if _, duplicate := validator.cases[operatingCase.ID]; duplicate {
			validator.add(path+".id", "operating-case ID must be unique")
		}
		validator.cases[operatingCase.ID] = operatingCase
		if len(operatingCase.Conditions) == 0 || len(operatingCase.Conditions) > MaxConditions {
			validator.add(path+".conditions", fmt.Sprintf("condition count must be between 1 and %d", MaxConditions))
		}
		seenConditions := map[string]bool{}
		for conditionIndex, condition := range operatingCase.Conditions {
			conditionPath := fmt.Sprintf("%s.conditions[%d]", path, conditionIndex)
			if !slices.Contains(allowedConditionAxes, condition.Axis) {
				validator.add(conditionPath+".axis", "unsupported operating-condition axis")
			}
			if _, portExists := validator.ports[condition.Target]; !portExists {
				if _, domainExists := validator.domains[condition.Target]; !domainExists {
					validator.add(conditionPath+".target", "condition target must refer to a declared port or domain")
				}
			}
			if !finite(condition.Min) || !finite(condition.Max) || condition.Min > condition.Max {
				validator.add(conditionPath, "condition requires finite min <= max")
			}
			if !slices.Contains(allowedUnits, condition.Unit) {
				validator.add(conditionPath+".unit", "unsupported condition unit")
			}
			key := condition.Axis + "\x00" + condition.Target
			if seenConditions[key] {
				validator.add(conditionPath, "condition axis and target must be unique within a case")
			}
			seenConditions[key] = true
		}
		seenEvents := map[string]bool{}
		for eventIndex, event := range operatingCase.Events {
			eventCount++
			eventPath := fmt.Sprintf("%s.events[%d]", path, eventIndex)
			validator.semanticID(eventPath+".id", event.ID)
			if seenEvents[event.ID] {
				validator.add(eventPath+".id", "event ID must be unique within an operating case")
			}
			seenEvents[event.ID] = true
			if !slices.Contains(allowedEventKinds, event.Kind) {
				validator.add(eventPath+".kind", "unsupported event kind")
			}
			if _, exists := validator.ports[event.Target]; !exists {
				validator.add(eventPath+".target", "event target must refer to a declared port")
			}
			if !finite(event.TriggerTimeS) || event.TriggerTimeS < 0 ||
				!finite(event.Initial) || !finite(event.Applied) {
				validator.add(eventPath, "event values and trigger time must be finite and trigger time nonnegative")
			}
			if !slices.Contains(allowedUnits, event.Unit) {
				validator.add(eventPath+".unit", "unsupported event unit")
			}
		}
	}
	if eventCount > MaxEvents {
		validator.add("requirements.operating_cases", fmt.Sprintf("total event count exceeds %d", MaxEvents))
	}
}

func (validator *requirementValidator) validateAssertions() {
	assertions := validator.requirement.Requirements.BehavioralRequirements
	if len(assertions) == 0 || len(assertions) > MaxAssertions {
		validator.add("requirements.behavioral_requirements", fmt.Sprintf("assertion count must be between 1 and %d", MaxAssertions))
	}
	for index, assertion := range assertions {
		path := fmt.Sprintf("requirements.behavioral_requirements[%d]", index)
		validator.semanticID(path+".id", assertion.ID)
		if validator.assertions[assertion.ID] {
			validator.add(path+".id", "behavioral assertion ID must be unique")
		}
		validator.assertions[assertion.ID] = true
		if !slices.Contains(allowedMetrics, assertion.Metric) {
			validator.add(path+".metric", "unsupported behavioral metric")
		}
		if !slices.Contains(allowedAnalyses, assertion.Analysis) {
			validator.add(path+".analysis", "unsupported analysis")
		}
		if !slices.Contains(allowedUnits, assertion.Unit) {
			validator.add(path+".unit", "unsupported assertion unit")
		}
		if assertion.Min == nil && assertion.Max == nil {
			validator.add(path, "assertion requires a minimum and/or maximum")
		}
		if assertion.Min != nil && !finite(*assertion.Min) {
			validator.add(path+".min", "minimum must be finite")
		}
		if assertion.Max != nil && !finite(*assertion.Max) {
			validator.add(path+".max", "maximum must be finite")
		}
		if assertion.Min != nil && assertion.Max != nil && *assertion.Min > *assertion.Max {
			validator.add(path, "assertion requires min <= max")
		}
		if assertion.FrequencyHz != nil && (!finite(*assertion.FrequencyHz) || *assertion.FrequencyHz <= 0 || *assertion.FrequencyHz > 1e12) {
			validator.add(path+".frequency_hz", "frequency must be finite and in (0, 1e12] Hz")
		}
		validator.validateObservation(path+".observation", assertion.Observation, true)
		if assertion.Excitation != nil {
			validator.validateObservation(path+".excitation", *assertion.Excitation, false)
		}
		if len(assertion.OperatingCases) == 0 || len(assertion.OperatingCases) > MaxOperatingCases {
			validator.add(path+".operating_cases", "assertion must name at least one bounded operating case")
		}
		seenCases := map[string]bool{}
		for caseIndex, caseID := range assertion.OperatingCases {
			casePath := fmt.Sprintf("%s.operating_cases[%d]", path, caseIndex)
			if _, exists := validator.cases[caseID]; !exists {
				validator.add(casePath, "assertion refers to an unknown operating case")
			}
			if seenCases[caseID] {
				validator.add(casePath, "assertion operating cases must be unique")
			}
			seenCases[caseID] = true
		}
	}
}

func (validator *requirementValidator) validateObservation(path string, observation Observation, allowCircuit bool) {
	if observation.Kind == "port" {
		if _, exists := validator.ports[observation.ID]; !exists {
			validator.add(path+".id", "observation refers to an unknown port")
		}
		return
	}
	if observation.Kind == "domain" {
		if _, exists := validator.domains[observation.ID]; !exists {
			validator.add(path+".id", "observation refers to an unknown domain")
		}
		return
	}
	if observation.Kind == "circuit" && allowCircuit {
		validator.semanticID(path+".id", observation.ID)
		return
	}
	validator.add(path+".kind", "observation kind must be port or domain; circuit is allowed only for observations")
}

func (validator *requirementValidator) validateBoardLimits() {
	limits := validator.requirement.Requirements.Constraints
	if limits.MaxComponents <= 0 || limits.MaxComponents > MaxComponents {
		validator.add("requirements.constraints.max_components", fmt.Sprintf("max components must be in [1, %d]", MaxComponents))
	}
	if !finite(limits.MaxWidthMM) || limits.MaxWidthMM <= 0 || limits.MaxWidthMM > MaxBoardDimensionMM {
		validator.add("requirements.constraints.max_width_mm", fmt.Sprintf("max width must be in (0, %d] mm", MaxBoardDimensionMM))
	}
	if !finite(limits.MaxHeightMM) || limits.MaxHeightMM <= 0 || limits.MaxHeightMM > MaxBoardDimensionMM {
		validator.add("requirements.constraints.max_height_mm", fmt.Sprintf("max height must be in (0, %d] mm", MaxBoardDimensionMM))
	}
}

func (validator *requirementValidator) validateAcceptance() {
	value := validator.requirement.Acceptance
	if !value.RequirePrimitiveOnly ||
		!value.RequireTopologySearch ||
		!value.RequireSimulation ||
		!value.RequireAllCorners ||
		!value.RequireModelProvenance ||
		!value.RequireClosedLoopEvidence ||
		!value.RequireCompleteRouting ||
		!value.RequireConnectivity ||
		!value.RequireWriterCorrectness ||
		!value.RequireRoundTripZeroDiff ||
		!value.RequireERC ||
		!value.RequireStrictDRC ||
		!value.RequireDeterministicReplay ||
		!value.RequireFailClosed {
		validator.add("acceptance", "open-topology requirements must enable the complete acceptance profile")
	}
}

func (validator *requirementValidator) semanticID(path, value string) {
	if !semanticIDPattern.MatchString(value) {
		validator.add(path, "value must be a normalized semantic identifier")
	}
}

func (validator *requirementValidator) text(path, value string, minimum, maximum int) {
	length := len(strings.TrimSpace(value))
	if length < minimum || length > maximum {
		validator.add(path, fmt.Sprintf("text length must be between %d and %d bytes", minimum, maximum))
	}
}

func (validator *requirementValidator) orderedElectrical(path string, minimum, nominal, maximum *float64) {
	for suffix, value := range map[string]*float64{"min_voltage_v": minimum, "nominal_voltage_v": nominal, "max_voltage_v": maximum} {
		if value != nil && (!finite(*value) || math.Abs(*value) > 1e6) {
			validator.add(path+"."+suffix, "voltage must be finite and within +/-1e6 V")
		}
	}
	if minimum != nil && nominal != nil && *minimum > *nominal {
		validator.add(path, "minimum voltage must not exceed nominal voltage")
	}
	if nominal != nil && maximum != nil && *nominal > *maximum {
		validator.add(path, "nominal voltage must not exceed maximum voltage")
	}
	if minimum != nil && maximum != nil && *minimum > *maximum {
		validator.add(path, "minimum voltage must not exceed maximum voltage")
	}
}

func (validator *requirementValidator) optionalPositive(path string, value *float64, maximum float64) {
	if value != nil && (!finite(*value) || *value <= 0 || *value > maximum) {
		validator.add(path, fmt.Sprintf("value must be finite and in (0, %g]", maximum))
	}
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
