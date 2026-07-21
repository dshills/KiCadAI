package architecturesearch

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"kicadai/internal/reports"
)

const (
	CodeSchemaInvalid        reports.Code = "ARCHITECTURE_SCHEMA_INVALID"
	CodeLimitExceeded        reports.Code = "ARCHITECTURE_LIMIT_EXCEEDED"
	CodeIdentityDuplicate    reports.Code = "ARCHITECTURE_IDENTITY_DUPLICATE"
	CodeDomainInvalid        reports.Code = "ARCHITECTURE_DOMAIN_INVALID"
	CodePortInvalid          reports.Code = "ARCHITECTURE_PORT_INVALID"
	CodeSignalInvalid        reports.Code = "ARCHITECTURE_SIGNAL_INVALID"
	CodeBindingUnresolved    reports.Code = "ARCHITECTURE_BINDING_UNRESOLVED"
	CodeConstraintInvalid    reports.Code = "ARCHITECTURE_CONSTRAINT_INVALID"
	CodeAcceptanceInvalid    reports.Code = "ARCHITECTURE_ACCEPTANCE_INVALID"
	CodeOperatingCaseInvalid reports.Code = "ARCHITECTURE_OPERATING_CASE_INVALID"
	CodeBehaviorInvalid      reports.Code = "ARCHITECTURE_BEHAVIOR_INVALID"
)

var semanticIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func Validate(requirement Requirement) []reports.Issue {
	validator := requirementValidator{requirement: requirement}
	validator.header()
	validator.domains()
	validator.ports()
	validator.signals()
	validator.participants()
	validator.objectives()
	validator.constraints("requirements.system_constraints", requirement.Requirements.SystemConstraints)
	validator.operatingCases()
	validator.behavioralRequirements()
	validator.boardLimits()
	validator.acceptance()
	slices.SortStableFunc(validator.issues, func(left, right reports.Issue) int {
		if order := strings.Compare(left.Path, right.Path); order != 0 {
			return order
		}
		if order := strings.Compare(string(left.Code), string(right.Code)); order != 0 {
			return order
		}
		return strings.Compare(left.Message, right.Message)
	})
	return validator.issues
}

type requirementValidator struct {
	requirement      Requirement
	issues           []reports.Issue
	domainsByID      map[string]Domain
	portsByID        map[string]Port
	signalsByID      map[string]Signal
	participantsByID map[string]Participant
}

func (validator *requirementValidator) add(code reports.Code, path, message string) {
	validator.issues = append(validator.issues, architectureIssue(code, path, message))
}

func (validator *requirementValidator) header() {
	v1 := validator.requirement.Schema == SchemaID && validator.requirement.Version == Version
	v2 := validator.requirement.Schema == SchemaIDV2 && validator.requirement.Version == VersionV2
	v3 := validator.requirement.Schema == SchemaIDV3 && validator.requirement.Version == VersionV3
	if !v1 && !v2 && !v3 {
		validator.add(CodeSchemaInvalid, "schema", fmt.Sprintf("schema/version must be %q/%d, %q/%d, or %q/%d", SchemaID, Version, SchemaIDV2, VersionV2, SchemaIDV3, VersionV3))
	}
	project := validator.requirement.Project
	if !validSemanticID(project.Name) {
		validator.add(CodeSchemaInvalid, "project.name", "project name must be a normalized semantic identifier")
	}
	validator.boundedText("project.title", project.Title, 1, 128)
	validator.boundedText("project.description", project.Description, 1, 512)
	if len(validator.requirement.Requirements.Domains) == 0 {
		validator.add(CodeDomainInvalid, "requirements.domains", "at least one electrical domain is required")
	}
	if len(validator.requirement.Requirements.Ports) == 0 {
		validator.add(CodePortInvalid, "requirements.ports", "at least one external port is required")
	}
	if len(validator.requirement.Requirements.Objectives) == 0 {
		validator.add(CodeConstraintInvalid, "requirements.objectives", "at least one behavioral objective is required")
	}
	validator.limit("requirements.domains", len(validator.requirement.Requirements.Domains), MaxDomains)
	validator.limit("requirements.ports", len(validator.requirement.Requirements.Ports), MaxPorts)
	validator.limit("requirements.signals", len(validator.requirement.Requirements.Signals), MaxSignals)
	validator.limit("requirements.participants", len(validator.requirement.Requirements.Participants), MaxParticipants)
	validator.limit("requirements.objectives", len(validator.requirement.Requirements.Objectives), MaxObjectives)
	validator.limit("requirements.operating_cases", len(validator.requirement.Requirements.OperatingCases), MaxOperatingCases)
	validator.limit("requirements.behavioral_requirements", len(validator.requirement.Requirements.BehavioralRequirements), MaxBehavioralRequirements)
}

func (validator *requirementValidator) domains() {
	validator.domainsByID = map[string]Domain{}
	for index, domain := range validator.requirement.Requirements.Domains {
		path := fmt.Sprintf("requirements.domains[%d]", index)
		if !validSemanticID(domain.ID) {
			validator.add(CodeDomainInvalid, path+".id", "domain id must be a normalized semantic identifier")
		} else if _, exists := validator.domainsByID[domain.ID]; exists {
			validator.add(CodeIdentityDuplicate, path+".id", "domain id is duplicated")
		} else {
			validator.domainsByID[domain.ID] = domain
		}
		if domain.Kind != "reference" && domain.Kind != "supply" {
			validator.add(CodeDomainInvalid, path+".kind", "domain kind must be reference or supply")
		}
		if validator.requirement.Version == Version {
			if domain.Source != "external" && domain.Source != "generated" {
				validator.add(CodeDomainInvalid, path+".source", "v1 domain source must be external or generated")
			}
		} else if domain.Source != "external" && !validSemanticID(domain.Source) {
			validator.add(CodeDomainInvalid, path+".source", "typed domain source must be external or a signal identity")
		}
		if !finiteInRange(domain.NominalVoltageV, -1000, 1000) {
			validator.add(CodeDomainInvalid, path+".nominal_voltage_v", "nominal voltage must be finite and within policy bounds")
		}
		validator.optionalNumber(CodeDomainInvalid, path+".min_voltage_v", domain.MinVoltageV, -1000, 1000)
		validator.optionalNumber(CodeDomainInvalid, path+".max_voltage_v", domain.MaxVoltageV, -1000, 1000)
		validator.optionalNumber(CodeDomainInvalid, path+".max_current_a", domain.MaxCurrentA, 0, 10000)
		if domain.MinVoltageV != nil && domain.MaxVoltageV != nil && *domain.MinVoltageV > *domain.MaxVoltageV {
			validator.add(CodeDomainInvalid, path, "domain minimum voltage exceeds maximum voltage")
		}
		if domain.MinVoltageV != nil && domain.NominalVoltageV < *domain.MinVoltageV {
			validator.add(CodeDomainInvalid, path+".nominal_voltage_v", "nominal voltage is below minimum voltage")
		}
		if domain.MaxVoltageV != nil && domain.NominalVoltageV > *domain.MaxVoltageV {
			validator.add(CodeDomainInvalid, path+".nominal_voltage_v", "nominal voltage exceeds maximum voltage")
		}
	}
}

func (validator *requirementValidator) signals() {
	validator.signalsByID = map[string]Signal{}
	for index, signal := range validator.requirement.Requirements.Signals {
		path := fmt.Sprintf("requirements.signals[%d]", index)
		if !validSemanticID(signal.ID) {
			validator.add(CodeSignalInvalid, path+".id", "signal id must be a normalized semantic identifier")
		} else if _, exists := validator.signalsByID[signal.ID]; exists {
			validator.add(CodeIdentityDuplicate, path+".id", "signal id is duplicated")
		} else {
			validator.signalsByID[signal.ID] = signal
		}
		if !allowedPortKind(signal.Kind) {
			validator.add(CodeSignalInvalid, path+".kind", "unsupported signal kind")
		}
		if _, exists := validator.domainsByID[signal.Domain]; !exists {
			validator.add(CodeBindingUnresolved, path+".domain", "signal references an unknown domain")
		}
		if signal.Electrical != nil {
			validator.electrical(path+".electrical", *signal.Electrical)
		}
		if signal.Protocol != nil {
			validator.protocol(path+".protocol", *signal.Protocol)
		}
	}
	if !supportsTypedSignals(validator.requirement.Version) {
		if len(validator.requirement.Requirements.Signals) != 0 || len(validator.requirement.Requirements.SystemConstraints) != 0 {
			validator.add(CodeSchemaInvalid, "requirements", "signals and system_constraints require the v2 schema")
		}
		return
	}
	for index, domain := range validator.requirement.Requirements.Domains {
		if domain.Source == "external" {
			continue
		}
		signal, exists := validator.signalsByID[domain.Source]
		if !exists {
			validator.add(CodeBindingUnresolved, fmt.Sprintf("requirements.domains[%d].source", index), "derived domain source references an unknown signal")
		} else if signal.Kind != "power" || signal.Domain != domain.ID {
			validator.add(CodeDomainInvalid, fmt.Sprintf("requirements.domains[%d].source", index), "derived domain source must be a power signal in the same domain")
		}
	}
}

func (validator *requirementValidator) ports() {
	validator.portsByID = map[string]Port{}
	for index, port := range validator.requirement.Requirements.Ports {
		path := fmt.Sprintf("requirements.ports[%d]", index)
		if !validSemanticID(port.ID) {
			validator.add(CodePortInvalid, path+".id", "port id must be a normalized semantic identifier")
		} else if _, exists := validator.portsByID[port.ID]; exists {
			validator.add(CodeIdentityDuplicate, path+".id", "external port id is duplicated")
		} else {
			validator.portsByID[port.ID] = port
		}
		if !allowedPortKind(port.Kind) {
			validator.add(CodePortInvalid, path+".kind", "unsupported external port kind")
		}
		if !allowedDirection(port.Direction) {
			validator.add(CodePortInvalid, path+".direction", "direction must be source, sink, or bidirectional")
		}
		if _, exists := validator.domainsByID[port.Domain]; !exists {
			validator.add(CodeBindingUnresolved, path+".domain", "external port references an unknown domain")
		}
		if port.Electrical != nil {
			validator.electrical(path+".electrical", *port.Electrical)
		}
		if port.Protocol != nil {
			validator.protocol(path+".protocol", *port.Protocol)
		}
	}
}

func (validator *requirementValidator) participants() {
	validator.participantsByID = map[string]Participant{}
	for index, participant := range validator.requirement.Requirements.Participants {
		path := fmt.Sprintf("requirements.participants[%d]", index)
		if !validSemanticID(participant.ID) {
			validator.add(CodePortInvalid, path+".id", "participant id must be a normalized semantic identifier")
		} else if _, exists := validator.participantsByID[participant.ID]; exists {
			validator.add(CodeIdentityDuplicate, path+".id", "participant id is duplicated")
		} else {
			validator.participantsByID[participant.ID] = participant
		}
		if !validSemanticID(participant.Capability) {
			validator.add(CodeConstraintInvalid, path+".capability", "participant capability must be a normalized semantic identifier")
		}
		if _, exists := validator.domainsByID[participant.Domain]; !exists {
			validator.add(CodeBindingUnresolved, path+".domain", "participant references an unknown domain")
		}
		validator.limit(path+".required_ports", len(participant.RequiredPorts), MaxParticipantPorts)
		if len(participant.RequiredPorts) == 0 {
			validator.add(CodePortInvalid, path+".required_ports", "participant requires at least one typed port")
		}
		seenPorts := map[string]bool{}
		for portIndex, port := range participant.RequiredPorts {
			portPath := fmt.Sprintf("%s.required_ports[%d]", path, portIndex)
			if !validSemanticID(port.ID) {
				validator.add(CodePortInvalid, portPath+".id", "participant port id must be a normalized semantic identifier")
			} else if seenPorts[port.ID] {
				validator.add(CodeIdentityDuplicate, portPath+".id", "participant port id is duplicated")
			}
			seenPorts[port.ID] = true
			if !allowedPortKind(port.Kind) {
				validator.add(CodePortInvalid, portPath+".kind", "unsupported participant port kind")
			}
			if !allowedDirection(port.Direction) {
				validator.add(CodePortInvalid, portPath+".direction", "direction must be source, sink, or bidirectional")
			}
			if port.Protocol != nil {
				validator.protocol(portPath+".protocol", *port.Protocol)
			}
		}
		validator.constraints(path+".constraints", participant.Constraints)
	}
}

func (validator *requirementValidator) objectives() {
	seenObjectives := map[string]bool{}
	type signalUse struct{ sources, sinks, bidirectional int }
	signalUses := map[string]signalUse{}
	for index, objective := range validator.requirement.Requirements.Objectives {
		path := fmt.Sprintf("requirements.objectives[%d]", index)
		if !validSemanticID(objective.ID) {
			validator.add(CodeConstraintInvalid, path+".id", "objective id must be a normalized semantic identifier")
		} else if seenObjectives[objective.ID] {
			validator.add(CodeIdentityDuplicate, path+".id", "objective id is duplicated")
		}
		seenObjectives[objective.ID] = true
		if !validSemanticID(objective.Capability) {
			validator.add(CodeConstraintInvalid, path+".capability", "objective capability must be a normalized semantic identifier")
		}
		validator.limit(path+".bindings", len(objective.Bindings), MaxBindings)
		if len(objective.Bindings) == 0 {
			validator.add(CodeBindingUnresolved, path+".bindings", "objective requires at least one binding")
		}
		seenRoles := map[string]bool{}
		for bindingIndex, binding := range objective.Bindings {
			bindingPath := fmt.Sprintf("%s.bindings[%d]", path, bindingIndex)
			if !validSemanticID(binding.Role) {
				validator.add(CodeBindingUnresolved, bindingPath+".role", "binding role must be a normalized semantic identifier")
			} else if seenRoles[binding.Role] {
				validator.add(CodeIdentityDuplicate, bindingPath+".role", "binding role is duplicated within the objective")
			}
			seenRoles[binding.Role] = true
			external := binding.Port != "" && binding.Signal == "" && binding.Direction == "" && binding.Participant == "" && binding.ParticipantPort == ""
			abstract := binding.Port == "" && binding.Signal == "" && binding.Direction == "" && binding.Participant != "" && binding.ParticipantPort != ""
			signal := binding.Port == "" && binding.Signal != "" && allowedDirection(binding.Direction) && binding.Participant == "" && binding.ParticipantPort == ""
			if !external && !abstract && !signal {
				validator.add(CodeBindingUnresolved, bindingPath, "binding must select exactly one external, participant, or directed signal endpoint")
				continue
			}
			if external {
				if _, exists := validator.portsByID[binding.Port]; !exists {
					validator.add(CodeBindingUnresolved, bindingPath+".port", "binding references an unknown external port")
				}
				continue
			}
			if signal {
				if !supportsTypedSignals(validator.requirement.Version) {
					validator.add(CodeSchemaInvalid, bindingPath+".signal", "signal bindings require the v2 or v3 schema")
					continue
				}
				if _, exists := validator.signalsByID[binding.Signal]; !exists {
					validator.add(CodeBindingUnresolved, bindingPath+".signal", "binding references an unknown signal")
					continue
				}
				use := signalUses[binding.Signal]
				switch binding.Direction {
				case "source":
					use.sources++
				case "sink":
					use.sinks++
				case "bidirectional":
					use.bidirectional++
				}
				signalUses[binding.Signal] = use
				continue
			}
			participant, exists := validator.participantsByID[binding.Participant]
			if !exists {
				validator.add(CodeBindingUnresolved, bindingPath+".participant", "binding references an unknown participant")
				continue
			}
			foundPort := false
			for _, port := range participant.RequiredPorts {
				if port.ID == binding.ParticipantPort {
					foundPort = true
					break
				}
			}
			if !foundPort {
				validator.add(CodeBindingUnresolved, bindingPath+".participant_port", "binding references an unknown participant port")
			}
		}
		validator.constraints(path+".constraints", objective.Constraints)
	}
	for index, signal := range validator.requirement.Requirements.Signals {
		use := signalUses[signal.ID]
		unidirectional := use.sources == 1 && use.sinks >= 1 && use.bidirectional == 0
		bidirectional := use.sources == 0 && use.sinks == 0 && use.bidirectional >= 2
		if !unidirectional && !bidirectional {
			validator.add(CodeSignalInvalid, fmt.Sprintf("requirements.signals[%d]", index), fmt.Sprintf("signal endpoints require one source and at least one sink, or at least two bidirectional endpoints; got %d source, %d sink, %d bidirectional", use.sources, use.sinks, use.bidirectional))
		}
	}
}

func (validator *requirementValidator) operatingCases() {
	cases := validator.requirement.Requirements.OperatingCases
	if validator.requirement.Version != VersionV3 {
		if len(cases) != 0 || len(validator.requirement.Requirements.BehavioralRequirements) != 0 {
			validator.add(CodeSchemaInvalid, "requirements", "operating_cases and behavioral_requirements require the v3 schema")
		}
		return
	}
	if len(cases) == 0 {
		validator.add(CodeOperatingCaseInvalid, "requirements.operating_cases", "v3 requires at least one bounded operating case")
		return
	}
	seen := map[string]bool{}
	for index, operatingCase := range cases {
		path := fmt.Sprintf("requirements.operating_cases[%d]", index)
		if !validSemanticID(operatingCase.ID) {
			validator.add(CodeOperatingCaseInvalid, path+".id", "operating case id must be a normalized semantic identifier")
		} else if seen[operatingCase.ID] {
			validator.add(CodeIdentityDuplicate, path+".id", "operating case id is duplicated")
		}
		seen[operatingCase.ID] = true
		validator.limit(path+".conditions", len(operatingCase.Conditions), MaxCaseConditions)
		if len(operatingCase.Conditions) == 0 {
			validator.add(CodeOperatingCaseInvalid, path+".conditions", "operating case requires at least one bounded condition")
		}
		seenConditions := map[string]bool{}
		for conditionIndex, condition := range operatingCase.Conditions {
			conditionPath := fmt.Sprintf("%s.conditions[%d]", path, conditionIndex)
			key := condition.Axis + "\x00" + condition.Target
			if seenConditions[key] {
				validator.add(CodeIdentityDuplicate, conditionPath, "operating condition axis and target are duplicated")
			}
			seenConditions[key] = true
			validator.operatingCondition(conditionPath, condition)
		}
	}
}

func (validator *requirementValidator) operatingCondition(path string, condition OperatingCondition) {
	expectedUnit, selectionAxis := operatingAxisContract(condition.Axis)
	if expectedUnit == "" && !selectionAxis {
		validator.add(CodeOperatingCaseInvalid, path+".axis", "unsupported operating condition axis")
		return
	}
	if !validator.semanticTargetExists(condition.Target) {
		validator.add(CodeBindingUnresolved, path+".target", "operating condition references an unknown semantic target")
	}
	if selectionAxis {
		if condition.Min != nil || condition.Max != nil || condition.Unit != "" {
			validator.add(CodeOperatingCaseInvalid, path, "selection corner axes cannot declare numeric bounds or units")
		}
		if condition.Selection != "all" && condition.Selection != "nominal" && condition.Selection != "minimum" && condition.Selection != "maximum" {
			validator.add(CodeOperatingCaseInvalid, path+".selection", "corner selection must be all, nominal, minimum, or maximum")
		}
		return
	}
	if condition.Selection != "" {
		validator.add(CodeOperatingCaseInvalid, path+".selection", "numeric operating axes cannot declare a corner selection")
	}
	if condition.Unit != expectedUnit {
		validator.add(CodeOperatingCaseInvalid, path+".unit", "operating condition requires canonical unit "+expectedUnit)
	}
	if condition.Min == nil && condition.Max == nil {
		validator.add(CodeOperatingCaseInvalid, path, "numeric operating condition requires a minimum or maximum")
	}
	validator.optionalNumber(CodeOperatingCaseInvalid, path+".min", condition.Min, -1e15, 1e15)
	validator.optionalNumber(CodeOperatingCaseInvalid, path+".max", condition.Max, -1e15, 1e15)
	if condition.Min != nil && condition.Max != nil && *condition.Min > *condition.Max {
		validator.add(CodeOperatingCaseInvalid, path, "operating condition minimum exceeds maximum")
	}
}

func (validator *requirementValidator) behavioralRequirements() {
	behaviors := validator.requirement.Requirements.BehavioralRequirements
	if validator.requirement.Version != VersionV3 {
		return
	}
	if len(behaviors) == 0 {
		validator.add(CodeBehaviorInvalid, "requirements.behavioral_requirements", "v3 requires at least one measurable behavioral requirement")
		return
	}
	cases := map[string]bool{}
	for _, operatingCase := range validator.requirement.Requirements.OperatingCases {
		cases[operatingCase.ID] = true
	}
	seen := map[string]bool{}
	for index, behavior := range behaviors {
		path := fmt.Sprintf("requirements.behavioral_requirements[%d]", index)
		if !validSemanticID(behavior.ID) {
			validator.add(CodeBehaviorInvalid, path+".id", "behavioral requirement id must be a normalized semantic identifier")
		} else if seen[behavior.ID] {
			validator.add(CodeIdentityDuplicate, path+".id", "behavioral requirement id is duplicated")
		}
		seen[behavior.ID] = true
		expectedAnalysis, expectedUnit, knownMetric := behavioralMetricContract(behavior.Metric)
		if !knownMetric {
			validator.add(CodeBehaviorInvalid, path+".metric", "unsupported behavioral metric")
		} else {
			if behavior.Analysis != expectedAnalysis {
				validator.add(CodeBehaviorInvalid, path+".analysis", "behavioral metric requires registered analysis "+expectedAnalysis)
			}
			if behavior.Unit != expectedUnit {
				validator.add(CodeBehaviorInvalid, path+".unit", "behavioral metric requires canonical unit "+expectedUnit)
			}
		}
		validator.behaviorObservation(path+".observation", behavior.Observation)
		if behavior.Min == nil && behavior.Max == nil {
			validator.add(CodeBehaviorInvalid, path, "behavioral requirement requires a minimum or maximum")
		}
		validator.optionalNumber(CodeBehaviorInvalid, path+".min", behavior.Min, -1e15, 1e15)
		validator.optionalNumber(CodeBehaviorInvalid, path+".max", behavior.Max, -1e15, 1e15)
		if behavior.Min != nil && behavior.Max != nil && *behavior.Min > *behavior.Max {
			validator.add(CodeBehaviorInvalid, path, "behavioral requirement minimum exceeds maximum")
		}
		if len(behavior.OperatingCases) == 0 {
			validator.add(CodeBehaviorInvalid, path+".operating_cases", "behavioral requirement must name at least one operating case")
		}
		seenCases := map[string]bool{}
		for caseIndex, caseID := range behavior.OperatingCases {
			casePath := fmt.Sprintf("%s.operating_cases[%d]", path, caseIndex)
			if !validSemanticID(caseID) || !cases[caseID] {
				validator.add(CodeBindingUnresolved, casePath, "behavioral requirement references an unknown operating case")
			} else if seenCases[caseID] {
				validator.add(CodeIdentityDuplicate, casePath, "behavioral operating case is duplicated")
			}
			seenCases[caseID] = true
		}
	}
}

func (validator *requirementValidator) behaviorObservation(path string, observation Observation) {
	switch observation.Kind {
	case "port":
		if _, exists := validator.portsByID[observation.ID]; !exists {
			validator.add(CodeBindingUnresolved, path+".id", "behavior observation references an unknown port")
		}
	case "signal":
		if _, exists := validator.signalsByID[observation.ID]; !exists {
			validator.add(CodeBindingUnresolved, path+".id", "behavior observation references an unknown signal")
		}
	case "domain":
		if _, exists := validator.domainsByID[observation.ID]; !exists {
			validator.add(CodeBindingUnresolved, path+".id", "behavior observation references an unknown domain")
		}
	case "circuit":
		if observation.ID != "circuit" {
			validator.add(CodeBehaviorInvalid, path+".id", "whole-circuit observation id must be circuit")
		}
	default:
		validator.add(CodeBehaviorInvalid, path+".kind", "observation kind must be port, signal, domain, or circuit")
	}
}

func (validator *requirementValidator) semanticTargetExists(target string) bool {
	if target == "circuit" {
		return true
	}
	if _, exists := validator.domainsByID[target]; exists {
		return true
	}
	if _, exists := validator.portsByID[target]; exists {
		return true
	}
	_, exists := validator.signalsByID[target]
	return exists
}

func (validator *requirementValidator) boardLimits() {
	limits := validator.requirement.Requirements.Constraints
	if limits.MaxComponents <= 0 || limits.MaxComponents > MaxComponents {
		validator.add(CodeLimitExceeded, "requirements.constraints.max_components", fmt.Sprintf("max_components must be between 1 and %d", MaxComponents))
	}
	if !finiteInRange(limits.MaxWidthMM, 0.01, MaxBoardDimensionMM) {
		validator.add(CodeLimitExceeded, "requirements.constraints.max_width_mm", "max_width_mm must be finite, positive, and within policy bounds")
	}
	if !finiteInRange(limits.MaxHeightMM, 0.01, MaxBoardDimensionMM) {
		validator.add(CodeLimitExceeded, "requirements.constraints.max_height_mm", "max_height_mm must be finite, positive, and within policy bounds")
	}
}

func (validator *requirementValidator) acceptance() {
	acceptance := validator.requirement.Acceptance
	required := []struct {
		path  string
		value bool
	}{
		{"require_erc", acceptance.RequireERC},
		{"require_strict_drc", acceptance.RequireStrictDRC},
		{"require_complete_routing", acceptance.RequireCompleteRouting},
		{"require_connectivity", acceptance.RequireConnectivity},
		{"require_writer_correctness", acceptance.RequireWriterCorrectness},
		{"require_round_trip_zero_diff", acceptance.RequireRoundTripZeroDiff},
		{"require_deterministic_replay", acceptance.RequireDeterministicReplay},
	}
	if supportsTypedSignals(validator.requirement.Version) {
		required = append(required,
			struct {
				path  string
				value bool
			}{"require_contract_composition", acceptance.RequireContractComposition},
			struct {
				path  string
				value bool
			}{"require_global_reasoning", acceptance.RequireGlobalReasoning},
			struct {
				path  string
				value bool
			}{"require_coverage_accounting", acceptance.RequireCoverageAccounting},
			struct {
				path  string
				value bool
			}{"require_alternatives", acceptance.RequireAlternatives},
			struct {
				path  string
				value bool
			}{"require_fail_closed", acceptance.RequireFailClosed},
		)
	}
	if validator.requirement.Version == VersionV3 {
		required = append(required,
			struct {
				path  string
				value bool
			}{"require_simulation", acceptance.RequireSimulation},
			struct {
				path  string
				value bool
			}{"require_all_corners", acceptance.RequireAllCorners},
			struct {
				path  string
				value bool
			}{"require_model_provenance", acceptance.RequireModelProvenance},
			struct {
				path  string
				value bool
			}{"require_closed_loop_evidence", acceptance.RequireClosedLoopEvidence},
		)
	}
	for _, gate := range required {
		if !gate.value {
			validator.add(CodeAcceptanceInvalid, "acceptance."+gate.path, "open-set schema requires this fail-closed acceptance gate")
		}
	}
}

func (validator *requirementValidator) constraints(path string, constraints []Constraint) {
	validator.limit(path, len(constraints), MaxConstraints)
	seen := map[string]bool{}
	for index, constraint := range constraints {
		constraintPath := fmt.Sprintf("%s[%d]", path, index)
		if !validSemanticID(constraint.Name) {
			validator.add(CodeConstraintInvalid, constraintPath+".name", "constraint name must be a normalized semantic identifier")
		} else if seen[constraint.Name] {
			validator.add(CodeIdentityDuplicate, constraintPath+".name", "constraint name is duplicated")
		}
		seen[constraint.Name] = true
		if !allowedRelation(constraint.Relation) {
			validator.add(CodeConstraintInvalid, constraintPath+".relation", "unsupported constraint relation")
		}
		if constraint.Unit != "" && !allowedUnit(constraint.Unit) {
			validator.add(CodeConstraintInvalid, constraintPath+".unit", "unsupported or non-canonical unit")
		}
		if constraint.TolerancePercent != nil {
			if constraint.Relation != "target" || !finiteInRange(*constraint.TolerancePercent, 0, 100) {
				validator.add(CodeConstraintInvalid, constraintPath+".tolerance_percent", "tolerance_percent requires a target relation and must be between 0 and 100")
			}
		}
		validator.constraintValue(constraintPath+".value", constraint)
	}
}

func (validator *requirementValidator) constraintValue(path string, constraint Constraint) {
	if len(constraint.Value) == 0 {
		validator.add(CodeConstraintInvalid, path, "constraint value is required")
		return
	}
	var value any
	if err := json.Unmarshal(constraint.Value, &value); err != nil {
		validator.add(CodeConstraintInvalid, path, "constraint value is not valid JSON: "+err.Error())
		return
	}
	switch constraint.Relation {
	case "required":
		if required, ok := value.(bool); !ok || !required {
			validator.add(CodeConstraintInvalid, path, "required relation must have boolean value true")
		}
	case "range":
		values, ok := value.([]any)
		if !ok || len(values) != 2 {
			validator.add(CodeConstraintInvalid, path, "range relation requires a two-number array")
			return
		}
		minimum, minimumOK := finiteJSONNumber(values[0])
		maximum, maximumOK := finiteJSONNumber(values[1])
		if !minimumOK || !maximumOK || minimum > maximum {
			validator.add(CodeConstraintInvalid, path, "range values must be finite and ascending")
		}
	case "one_of":
		values, ok := value.([]any)
		if !ok || len(values) == 0 {
			validator.add(CodeConstraintInvalid, path, "one_of relation requires a non-empty array")
			return
		}
		seen := map[string]bool{}
		for _, option := range values {
			if !validConstraintScalar(option) {
				validator.add(CodeConstraintInvalid, path, "one_of options must be finite scalar values")
				break
			}
			encoded, _ := json.Marshal(option)
			if seen[string(encoded)] {
				validator.add(CodeConstraintInvalid, path, "one_of options must be unique")
				break
			}
			seen[string(encoded)] = true
		}
	case "minimum", "maximum", "target":
		if _, ok := finiteJSONNumber(value); !ok {
			validator.add(CodeConstraintInvalid, path, constraint.Relation+" relation requires a finite number")
		}
	case "equal":
		if !validConstraintScalar(value) {
			validator.add(CodeConstraintInvalid, path, "equal relation requires a finite scalar value")
		}
	default:
		if value == nil {
			validator.add(CodeConstraintInvalid, path, "constraint value must not be null")
		}
	}
}

func (validator *requirementValidator) electrical(path string, electrical Electrical) {
	validator.optionalNumber(CodePortInvalid, path+".min_voltage_v", electrical.MinVoltageV, -1000, 1000)
	validator.optionalNumber(CodePortInvalid, path+".nominal_voltage_v", electrical.NominalVoltageV, -1000, 1000)
	validator.optionalNumber(CodePortInvalid, path+".max_voltage_v", electrical.MaxVoltageV, -1000, 1000)
	validator.optionalNumber(CodePortInvalid, path+".max_current_a", electrical.MaxCurrentA, 0, 10000)
	validator.optionalNumber(CodePortInvalid, path+".max_source_current_ma", electrical.MaxSourceCurrentMA, 0, 1000000)
	validator.optionalNumber(CodePortInvalid, path+".input_impedance_min_ohm", electrical.InputImpedanceMinOhm, 0, 1e15)
	validator.optionalNumber(CodePortInvalid, path+".frequency_max_hz", electrical.FrequencyMaxHz, 0, 1e15)
	if electrical.MinVoltageV != nil && electrical.MaxVoltageV != nil && *electrical.MinVoltageV > *electrical.MaxVoltageV {
		validator.add(CodePortInvalid, path, "electrical minimum voltage exceeds maximum voltage")
	}
	if electrical.NominalVoltageV != nil && electrical.MinVoltageV != nil && *electrical.NominalVoltageV < *electrical.MinVoltageV {
		validator.add(CodePortInvalid, path+".nominal_voltage_v", "nominal voltage is below minimum voltage")
	}
	if electrical.NominalVoltageV != nil && electrical.MaxVoltageV != nil && *electrical.NominalVoltageV > *electrical.MaxVoltageV {
		validator.add(CodePortInvalid, path+".nominal_voltage_v", "nominal voltage exceeds maximum voltage")
	}
	if electrical.DefaultState != "" && !validSemanticID(electrical.DefaultState) {
		validator.add(CodePortInvalid, path+".default_state", "default state must be a normalized semantic identifier")
	}
}

func (validator *requirementValidator) protocol(path string, protocol Protocol) {
	if !validSemanticID(protocol.Name) {
		validator.add(CodePortInvalid, path+".name", "protocol name must be a normalized semantic identifier")
	}
	if !slices.Contains(registeredProtocolModes, protocol.Mode) {
		validator.add(CodePortInvalid, path+".mode", "unsupported signaling mode")
	}
	if !finiteInRange(protocol.MaxFrequencyHz, 0.000001, 1e15) {
		validator.add(CodePortInvalid, path+".max_frequency_hz", "protocol maximum frequency must be finite and positive")
	}
}

func (validator *requirementValidator) optionalNumber(code reports.Code, path string, value *float64, minimum, maximum float64) {
	if value != nil && !finiteInRange(*value, minimum, maximum) {
		validator.add(code, path, "value must be finite and within policy bounds")
	}
}

func (validator *requirementValidator) boundedText(path, value string, minimum, maximum int) {
	length := utf8.RuneCountInString(value)
	if length < minimum || length > maximum {
		validator.add(CodeSchemaInvalid, path, fmt.Sprintf("text length must be between %d and %d characters", minimum, maximum))
	}
}

func (validator *requirementValidator) limit(path string, count, maximum int) {
	if count > maximum {
		validator.add(CodeLimitExceeded, path, fmt.Sprintf("contains %d entries; maximum is %d", count, maximum))
	}
}

func architectureIssue(code reports.Code, path, message string) reports.Issue {
	return reports.Issue{Code: code, Severity: reports.SeverityError, Path: path, Message: message}
}

func validSemanticID(value string) bool {
	return semanticIDPattern.MatchString(value)
}

func allowedPortKind(value string) bool {
	return slices.Contains(registeredPortKinds, value)
}

func allowedDirection(value string) bool {
	return slices.Contains(registeredDirections, value)
}

func allowedRelation(value string) bool {
	return slices.Contains(registeredConstraintRelations, value)
}

func allowedUnit(value string) bool {
	return slices.Contains(registeredCanonicalUnits, value)
}

func supportsTypedSignals(version int) bool {
	return version == VersionV2 || version == VersionV3
}

func validConstraintScalar(value any) bool {
	switch typed := value.(type) {
	case string:
		return typed != "" && utf8.RuneCountInString(typed) <= 128
	case bool:
		return true
	case float64:
		return !math.IsNaN(typed) && !math.IsInf(typed, 0)
	default:
		return false
	}
}

func finiteJSONNumber(value any) (float64, bool) {
	number, ok := value.(float64)
	return number, ok && !math.IsNaN(number) && !math.IsInf(number, 0)
}

func finiteInRange(value, minimum, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= minimum && value <= maximum
}
