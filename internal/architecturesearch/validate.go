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
	CodeSchemaInvalid         reports.Code = "ARCHITECTURE_SCHEMA_INVALID"
	CodeLimitExceeded         reports.Code = "ARCHITECTURE_LIMIT_EXCEEDED"
	CodeIdentityDuplicate     reports.Code = "ARCHITECTURE_IDENTITY_DUPLICATE"
	CodeDomainInvalid         reports.Code = "ARCHITECTURE_DOMAIN_INVALID"
	CodePortInvalid           reports.Code = "ARCHITECTURE_PORT_INVALID"
	CodeSignalInvalid         reports.Code = "ARCHITECTURE_SIGNAL_INVALID"
	CodeBindingUnresolved     reports.Code = "ARCHITECTURE_BINDING_UNRESOLVED"
	CodeConstraintInvalid     reports.Code = "ARCHITECTURE_CONSTRAINT_INVALID"
	CodeAcceptanceInvalid     reports.Code = "ARCHITECTURE_ACCEPTANCE_INVALID"
	CodeOperatingCaseInvalid  reports.Code = "ARCHITECTURE_OPERATING_CASE_INVALID"
	CodeOperatingEventInvalid reports.Code = "ARCHITECTURE_OPERATING_EVENT_INVALID"
	CodeBehaviorInvalid       reports.Code = "ARCHITECTURE_BEHAVIOR_INVALID"
	CodeControlInvalid        reports.Code = "ARCHITECTURE_CONTROL_INVALID"
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
	validator.multiControlSafetyObjectives()
	validator.constraints("requirements.system_constraints", requirement.Requirements.SystemConstraints)
	validator.operatingCases()
	validator.controlTransitions()
	validator.behavioralRequirements()
	validator.controlStartupCoherence()
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

func (validator *requirementValidator) multiControlSafetyObjectives() {
	if validator.requirement.Version != VersionV6 {
		return
	}
	for objectiveIndex, objective := range validator.requirement.Requirements.Objectives {
		if objective.Capability != "safety_interlock" {
			continue
		}
		path := fmt.Sprintf("requirements.objectives[%d]", objectiveIndex)
		connectingInputs := 0
		disconnectingInputs := 0
		permitOutputs := 0
		for _, binding := range objective.Bindings {
			control := controlForBinding(validator.requirement, binding)
			if control == nil {
				continue
			}
			source := binding.Direction == "source"
			if binding.Port != "" {
				if port, exists := validator.portsByID[binding.Port]; exists && port.Direction == "source" {
					source = true
				}
			}
			if source {
				if control.Function == "enable" || control.Function == "power_good" {
					permitOutputs++
					if control.StartupState != "deasserted" || control.SafeState != "deasserted" {
						validator.add(CodeControlInvalid, path, "multi-control permit output must be deasserted at startup and in its safe state")
					}
				}
				continue
			}
			switch control.Function {
			case "enable", "power_good":
				connectingInputs++
				if control.StartupState != "deasserted" {
					validator.add(CodeControlInvalid, path, "multi-control connecting input must start deasserted")
				}
			case "fault", "inhibit", "reset":
				disconnectingInputs++
				if control.SafeState != "asserted" {
					validator.add(CodeControlInvalid, path, "multi-control protection input must assert in its safe state")
				}
			}
		}
		// The catalog coordinator has one connecting input and one permit
		// output, with one or more protection inputs. Reject broader graphs at
		// the semantic boundary instead of accepting a contract the provider
		// cannot lower deterministically.
		if connectingInputs != 1 || disconnectingInputs < 1 || permitOutputs != 1 {
			validator.add(CodeControlInvalid, path, "multi-control safety interlock requires one connecting input, at least one protection input, and one deasserted-safe permit output")
		}
	}
}

// controlStartupCoherence rejects a low-at-startup proof claim when the only
// declared load-switch control state explicitly requests a connected startup.
// Such a requirement needs a distinct startup enable/sequencing dependency;
// the implementation cannot safely invent one from a fault or inhibit signal.
func (validator *requirementValidator) controlStartupCoherence() {
	if validator.requirement.Version != VersionV6 {
		return
	}
	for behaviorIndex, behavior := range validator.requirement.Requirements.BehavioralRequirements {
		if behavior.Metric != "startup_output_voltage" || behavior.Observation.Kind != "port" || behavior.Max == nil {
			continue
		}
		port, exists := validator.portsByID[behavior.Observation.ID]
		if !exists {
			continue
		}
		domain, exists := validator.domainsByID[port.Domain]
		if !exists || domain.MinVoltageV == nil || *behavior.Max >= *domain.MinVoltageV {
			continue
		}
		for _, objective := range validator.requirement.Requirements.Objectives {
			if objective.Capability != "load_switch" {
				continue
			}
			outputMatches := false
			var control *ControlSemantics
			for _, binding := range objective.Bindings {
				if binding.Role == "output" && binding.Port == port.ID {
					outputMatches = true
				}
				if binding.Role == "control" {
					control = controlForBinding(validator.requirement, binding)
				}
			}
			if !outputMatches || control == nil {
				continue
			}
			startupConnected := control.StartupState == "deasserted" && slices.Contains([]string{"inhibit", "reset", "fault"}, control.Function)
			startupConnected = startupConnected || control.StartupState == "asserted" && slices.Contains([]string{"enable", "power_good"}, control.Function)
			if startupConnected {
				validator.add(CodeControlInvalid, fmt.Sprintf("requirements.behavioral_requirements[%d]", behaviorIndex), "startup output requires a deenergized load while the declared load-switch control starts in its connected state; declare a separate startup enable or sequencing dependency")
			}
		}
	}
}

type requirementValidator struct {
	requirement      Requirement
	issues           []reports.Issue
	domainsByID      map[string]Domain
	portsByID        map[string]Port
	signalsByID      map[string]Signal
	participantsByID map[string]Participant
	eventsByID       map[string]string
	transitionsByID  map[string]ControlTransition
	transitionPaths  map[string]string
}

func (validator *requirementValidator) add(code reports.Code, path, message string) {
	validator.issues = append(validator.issues, architectureIssue(code, path, message))
}

func (validator *requirementValidator) header() {
	v1 := validator.requirement.Schema == SchemaID && validator.requirement.Version == Version
	v2 := validator.requirement.Schema == SchemaIDV2 && validator.requirement.Version == VersionV2
	v3 := validator.requirement.Schema == SchemaIDV3 && validator.requirement.Version == VersionV3
	v4 := validator.requirement.Schema == SchemaIDV4 && validator.requirement.Version == VersionV4
	v5 := validator.requirement.Schema == SchemaIDV5 && validator.requirement.Version == VersionV5
	v6 := validator.requirement.Schema == SchemaIDV6 && validator.requirement.Version == VersionV6
	if !v1 && !v2 && !v3 && !v4 && !v5 && !v6 {
		validator.add(CodeSchemaInvalid, "schema", fmt.Sprintf("schema/version must be %q/%d, %q/%d, %q/%d, %q/%d, %q/%d, or %q/%d", SchemaID, Version, SchemaIDV2, VersionV2, SchemaIDV3, VersionV3, SchemaIDV4, VersionV4, SchemaIDV5, VersionV5, SchemaIDV6, VersionV6))
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
	validator.limit("requirements.control_transitions", len(validator.requirement.Requirements.ControlTransitions), MaxControlTransitions)
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
		validator.control(path+".control", signal.Kind, signal.Control)
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
		validator.control(path+".control", port.Kind, port.Control)
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
				port, exists := validator.portsByID[binding.Port]
				if !exists {
					validator.add(CodeBindingUnresolved, bindingPath+".port", "binding references an unknown external port")
				} else if validator.requirement.Version == VersionV6 && controlRole(binding.Role) && port.Control == nil {
					validator.add(CodeControlInvalid, bindingPath+".port", "control-role binding requires explicit v6 control semantics on its endpoint")
				}
				continue
			}
			if signal {
				if !supportsTypedSignals(validator.requirement.Version) {
					validator.add(CodeSchemaInvalid, bindingPath+".signal", "signal bindings require the v2 or v3 schema")
					continue
				}
				signalEndpoint, exists := validator.signalsByID[binding.Signal]
				if !exists {
					validator.add(CodeBindingUnresolved, bindingPath+".signal", "binding references an unknown signal")
					continue
				}
				if validator.requirement.Version == VersionV6 && controlRole(binding.Role) && signalEndpoint.Control == nil {
					validator.add(CodeControlInvalid, bindingPath+".signal", "control-role binding requires explicit v6 control semantics on its endpoint")
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

func controlRole(role string) bool {
	return slices.Contains([]string{"control", "enable", "fault", "inhibit", "reset", "power_good", "state"}, role)
}

func (validator *requirementValidator) operatingCases() {
	cases := validator.requirement.Requirements.OperatingCases
	validator.eventsByID = map[string]string{}
	if !supportsBehavioralVerification(validator.requirement.Version) {
		if len(cases) != 0 || len(validator.requirement.Requirements.BehavioralRequirements) != 0 {
			validator.add(CodeSchemaInvalid, "requirements", "operating_cases and behavioral_requirements require the v3 or v4 schema")
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
		if !supportsOperatingEvents(validator.requirement.Version) {
			if len(operatingCase.Events) != 0 {
				validator.add(CodeSchemaInvalid, path+".events", "operating events require the v5 or v6 schema")
			}
			continue
		}
		validator.limit(path+".events", len(operatingCase.Events), MaxCaseEvents)
		if validator.requirement.Version == VersionV5 && len(operatingCase.Events) == 0 {
			validator.add(CodeOperatingEventInvalid, path+".events", "v5 operating case requires at least one event")
		}
		for eventIndex, event := range operatingCase.Events {
			eventPath := fmt.Sprintf("%s.events[%d]", path, eventIndex)
			if previousCase, exists := validator.eventsByID[event.ID]; exists {
				validator.add(CodeIdentityDuplicate, eventPath+".id", "event id is duplicated in operating case "+previousCase)
			} else {
				validator.eventsByID[event.ID] = operatingCase.ID
			}
			validator.operatingEvent(eventPath, event)
		}
	}
	validator.limit("requirements.operating_cases.events", len(validator.eventsByID), MaxOperatingEvents)
}

func (validator *requirementValidator) operatingEvent(path string, event OperatingEvent) {
	if !validSemanticID(event.ID) {
		validator.add(CodeOperatingEventInvalid, path+".id", "event id must be a normalized semantic identifier")
	}
	if !slices.Contains(registeredOperatingEventKinds, event.Kind) {
		validator.add(CodeOperatingEventInvalid, path+".kind", "unsupported operating event kind")
	}
	validator.behaviorObservation(path+".target", event.Target)
	if event.Target.Kind == "event" {
		validator.add(CodeOperatingEventInvalid, path+".target.kind", "an operating event cannot target another event")
	}
	if !finiteInRange(event.TriggerTimeS, 0, 1e6) {
		validator.add(CodeOperatingEventInvalid, path+".trigger_time_s", "event trigger time must be finite, nonnegative, and bounded")
	}
	if !finiteInRange(event.DurationS, 1e-12, 1e6) {
		validator.add(CodeOperatingEventInvalid, path+".duration_s", "event duration must be finite, positive, and bounded")
	}
	if event.Applied == nil {
		validator.add(CodeOperatingEventInvalid, path+".applied", "event applied value is required")
	}
	validator.optionalNumber(CodeOperatingEventInvalid, path+".initial", event.Initial, -1e15, 1e15)
	validator.optionalNumber(CodeOperatingEventInvalid, path+".applied", event.Applied, -1e15, 1e15)
	validator.optionalNumber(CodeOperatingEventInvalid, path+".recovered", event.Recovered, -1e15, 1e15)
	if !eventKindAllowsUnit(event.Kind, event.Unit) {
		validator.add(CodeOperatingEventInvalid, path+".unit", "event kind does not support canonical unit "+event.Unit)
	}
}

func (validator *requirementValidator) control(path, kind string, control *ControlSemantics) {
	if control == nil {
		return
	}
	if validator.requirement.Version != VersionV6 {
		validator.add(CodeSchemaInvalid, path, "control semantics require the v6 schema")
		return
	}
	if kind != "digital_logic" && kind != "analog_control" {
		validator.add(CodeControlInvalid, path, "control semantics require a digital_logic or analog_control endpoint")
	}
	if !slices.Contains([]string{"enable", "inhibit", "reset", "fault", "power_good", "state"}, control.Function) {
		validator.add(CodeControlInvalid, path+".function", "control function must be enable, inhibit, reset, fault, power_good, or state")
	}
	if control.Polarity != "active_high" && control.Polarity != "active_low" {
		validator.add(CodeControlInvalid, path+".polarity", "control polarity must be active_high or active_low")
	}
	for field, state := range map[string]string{"startup_state": control.StartupState, "safe_state": control.SafeState} {
		if state != "asserted" && state != "deasserted" {
			validator.add(CodeControlInvalid, path+"."+field, field+" must be asserted or deasserted")
		}
	}
}

func (validator *requirementValidator) controlTransitions() {
	validator.transitionsByID = map[string]ControlTransition{}
	validator.transitionPaths = map[string]string{}
	transitions := validator.requirement.Requirements.ControlTransitions
	if validator.requirement.Version != VersionV6 {
		if len(transitions) != 0 {
			validator.add(CodeSchemaInvalid, "requirements.control_transitions", "control transitions require the v6 schema")
		}
		return
	}
	for index, transition := range transitions {
		path := fmt.Sprintf("requirements.control_transitions[%d]", index)
		if !validSemanticID(transition.ID) {
			validator.add(CodeControlInvalid, path+".id", "control transition id must be a normalized semantic identifier")
		} else if _, exists := validator.transitionsByID[transition.ID]; exists {
			validator.add(CodeIdentityDuplicate, path+".id", "control transition id is duplicated")
		} else {
			validator.transitionsByID[transition.ID] = transition
			validator.transitionPaths[transition.ID] = path
		}
		if transition.Event != "" {
			if _, exists := validator.eventsByID[transition.Event]; !exists {
				validator.add(CodeBindingUnresolved, path+".event", "control transition references an unknown operating event")
			}
		}
		validator.behaviorObservation(path+".target", transition.Target)
		validator.behaviorObservation(path+".trigger", transition.Trigger)
		if transition.Target.Kind != "port" && transition.Target.Kind != "signal" {
			validator.add(CodeControlInvalid, path+".target.kind", "control transition target must be a semantic port or signal")
		}
		if transition.Trigger.Kind == "circuit" {
			validator.add(CodeControlInvalid, path+".trigger.kind", "control transition trigger must be an event or semantic endpoint")
		}
		if transition.From == transition.To || !validTransitionState(transition.From) || !validTransitionState(transition.To) {
			validator.add(CodeControlInvalid, path, "control transition requires distinct registered from and to states")
		}
		if transition.Direction != "rising" && transition.Direction != "falling" {
			validator.add(CodeControlInvalid, path+".direction", "control transition direction must be rising or falling")
		}
		if control := validator.controlForObservation(transition.Target); control != nil {
			if (transition.From != "asserted" && transition.From != "deasserted") || (transition.To != "asserted" && transition.To != "deasserted") {
				validator.add(CodeControlInvalid, path, "a control endpoint transition must use asserted and deasserted states")
			} else if expected := controlTransitionDirection(*control, transition.From, transition.To); expected != transition.Direction {
				validator.add(CodeControlInvalid, path+".direction", "physical transition direction contradicts the target control polarity")
			}
		}
		validator.optionalNumber(CodeControlInvalid, path+".minimum_delay_s", transition.MinimumDelayS, 0, 1e6)
		validator.optionalNumber(CodeControlInvalid, path+".maximum_delay_s", transition.MaximumDelayS, 0, 1e6)
		if transition.MinimumDelayS == nil && transition.MaximumDelayS == nil {
			validator.add(CodeControlInvalid, path, "control transition requires a minimum or maximum delay")
		} else if transition.MinimumDelayS != nil && transition.MaximumDelayS != nil && *transition.MinimumDelayS > *transition.MaximumDelayS {
			validator.add(CodeControlInvalid, path, "control transition minimum delay exceeds maximum delay")
		}
		validator.limit(path+".dependencies", len(transition.Dependencies), MaxTransitionDependencies)
		seenDependencies := map[string]bool{}
		for dependencyIndex, dependency := range transition.Dependencies {
			dependencyPath := fmt.Sprintf("%s.dependencies[%d]", path, dependencyIndex)
			validator.behaviorObservation(dependencyPath+".target", dependency.Target)
			if dependency.Target.Kind == "event" || dependency.Target.Kind == "circuit" {
				validator.add(CodeControlInvalid, dependencyPath+".target.kind", "control dependency must reference a semantic state-bearing endpoint")
			}
			key := dependency.Target.Kind + "\x00" + dependency.Target.ID
			if seenDependencies[key] {
				validator.add(CodeIdentityDuplicate, dependencyPath+".target", "control transition dependency target is duplicated")
			}
			seenDependencies[key] = true
			if !validTransitionState(dependency.State) {
				validator.add(CodeControlInvalid, dependencyPath+".state", "control dependency state is unsupported")
			}
			if control := validator.controlForObservation(dependency.Target); control != nil && dependency.State != "asserted" && dependency.State != "deasserted" {
				validator.add(CodeControlInvalid, dependencyPath+".state", "control endpoint dependencies must use asserted or deasserted")
			}
			if !finiteInRange(dependency.StableForS, 0, 1e6) {
				validator.add(CodeControlInvalid, dependencyPath+".stable_for_s", "control dependency stability time must be finite, nonnegative, and bounded")
			}
		}
	}
}

func (validator *requirementValidator) controlForObservation(observation Observation) *ControlSemantics {
	switch observation.Kind {
	case "port":
		return validator.portsByID[observation.ID].Control
	case "signal":
		return validator.signalsByID[observation.ID].Control
	default:
		return nil
	}
}

func validTransitionState(state string) bool {
	return slices.Contains([]string{"asserted", "deasserted", "valid", "invalid", "energized", "deenergized"}, state)
}

func controlTransitionDirection(control ControlSemantics, from, to string) string {
	asserting := from == "deasserted" && to == "asserted"
	if (control.Polarity == "active_high" && asserting) || (control.Polarity == "active_low" && !asserting) {
		return "rising"
	}
	return "falling"
}

func (validator *requirementValidator) operatingCondition(path string, condition OperatingCondition) {
	expectedUnit, selectionAxis := operatingAxisContractForVersion(condition.Axis, validator.requirement.Version)
	if expectedUnit == "" && !selectionAxis {
		validator.add(CodeOperatingCaseInvalid, path+".axis", "unsupported operating condition axis")
		return
	}
	if selectionAxis {
		if !validator.operatingSelectionTargetExists(condition.Axis, condition.Target) {
			validator.add(CodeBindingUnresolved, path+".target", "operating condition references an unknown semantic or aggregate target")
		}
		if condition.Min != nil || condition.Max != nil || condition.Unit != "" {
			validator.add(CodeOperatingCaseInvalid, path, "selection corner axes cannot declare numeric bounds or units")
		}
		if !validOperatingSelection(condition.Axis, condition.Selection) {
			validator.add(CodeOperatingCaseInvalid, path+".selection", "corner selection is unsupported for this operating axis")
		}
		return
	}
	if !validator.semanticTargetExists(condition.Target) {
		validator.add(CodeBindingUnresolved, path+".target", "operating condition references an unknown semantic target")
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

func (validator *requirementValidator) operatingSelectionTargetExists(axis, target string) bool {
	if validator.semanticTargetExists(target) {
		return true
	}
	return axis == "tolerance" && (target == "all_components" || target == "all_passives")
}

func validOperatingSelection(axis, selection string) bool {
	switch axis {
	case "cooling_mode":
		return selection == "all" || selection == "nominal" || selection == "blocked_airflow"
	case "tolerance":
		return selection == "all" || selection == "nominal" || selection == "minimum" ||
			selection == "maximum" || selection == "minimum_nominal_maximum"
	default:
		return selection == "all" || selection == "nominal" || selection == "minimum" || selection == "maximum"
	}
}

func (validator *requirementValidator) behavioralRequirements() {
	behaviors := validator.requirement.Requirements.BehavioralRequirements
	if !supportsBehavioralVerification(validator.requirement.Version) {
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
	transitionReferences := map[string]int{}
	for index, behavior := range behaviors {
		path := fmt.Sprintf("requirements.behavioral_requirements[%d]", index)
		if !validSemanticID(behavior.ID) {
			validator.add(CodeBehaviorInvalid, path+".id", "behavioral requirement id must be a normalized semantic identifier")
		} else if seen[behavior.ID] {
			validator.add(CodeIdentityDuplicate, path+".id", "behavioral requirement id is duplicated")
		}
		seen[behavior.ID] = true
		expectedAnalysis, expectedUnit, knownMetric := behavioralMetricContractForVersion(behavior.Metric, validator.requirement.Version)
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
		if validator.requirement.Version == VersionV6 && slices.Contains([]string{"response_time", "protection_response_time", "sequence_delay"}, behavior.Metric) && behavior.Transition == "" {
			validator.add(CodeControlInvalid, path+".transition", "directed response timing requires an explicit v6 control transition")
		}
		if behavior.Transition != "" {
			if validator.requirement.Version != VersionV6 {
				validator.add(CodeSchemaInvalid, path+".transition", "behavior transition references require the v6 schema")
			} else if transition, exists := validator.transitionsByID[behavior.Transition]; !exists {
				validator.add(CodeBindingUnresolved, path+".transition", "behavior references an unknown control transition")
			} else {
				transitionReferences[behavior.Transition]++
				if behavior.Analysis != "transient" {
					validator.add(CodeControlInvalid, path+".analysis", "control transition behavior requires transient analysis")
				}
				if behavior.Observation != transition.Target {
					validator.add(CodeControlInvalid, path+".observation", "control transition behavior must observe the transition target")
				}
				if behavior.Min != nil && transition.MinimumDelayS != nil && *behavior.Min < *transition.MinimumDelayS {
					validator.add(CodeControlInvalid, path+".min", "behavior minimum falls outside the control transition timing envelope")
				}
				if behavior.Max != nil && transition.MaximumDelayS != nil && *behavior.Max > *transition.MaximumDelayS {
					validator.add(CodeControlInvalid, path+".max", "behavior maximum falls outside the control transition timing envelope")
				}
			}
		}
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
		transitionEvent := ""
		if transition, exists := validator.transitionsByID[behavior.Transition]; exists {
			transitionEvent = transition.Event
		}
		for caseIndex, caseID := range behavior.OperatingCases {
			casePath := fmt.Sprintf("%s.operating_cases[%d]", path, caseIndex)
			if !validSemanticID(caseID) || !cases[caseID] {
				validator.add(CodeBindingUnresolved, casePath, "behavioral requirement references an unknown operating case")
			} else if seenCases[caseID] {
				validator.add(CodeIdentityDuplicate, casePath, "behavioral operating case is duplicated")
			} else if behavior.Observation.Kind == "event" && validator.eventsByID[behavior.Observation.ID] != caseID {
				validator.add(CodeBindingUnresolved, casePath, "event observation must be evaluated in the operating case that declares the event")
			} else if transitionEvent != "" && validator.eventsByID[transitionEvent] != caseID {
				validator.add(CodeBindingUnresolved, casePath, "control transition must be evaluated in the operating case that declares its event")
			}
			seenCases[caseID] = true
		}
	}
	if validator.requirement.Version == VersionV6 {
		for id := range validator.transitionsByID {
			if transitionReferences[id] == 0 {
				validator.add(CodeControlInvalid, validator.transitionPaths[id], "control transition lacks a measurable behavioral requirement")
			}
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
	case "event":
		if !supportsOperatingEvents(validator.requirement.Version) {
			validator.add(CodeBehaviorInvalid, path+".kind", "event observations require the v5 or v6 schema")
		} else if _, exists := validator.eventsByID[observation.ID]; !exists {
			validator.add(CodeBindingUnresolved, path+".id", "behavior observation references an unknown event")
		}
	default:
		validator.add(CodeBehaviorInvalid, path+".kind", "observation kind must be port, signal, domain, circuit, or event")
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
	maximumComponents := MaxComponents
	if validator.requirement.Version == VersionV5 {
		maximumComponents = MaxComponentsV5
	}
	if limits.MaxComponents <= 0 || limits.MaxComponents > maximumComponents {
		validator.add(CodeLimitExceeded, "requirements.constraints.max_components", fmt.Sprintf("max_components must be between 1 and %d", maximumComponents))
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
	if supportsBehavioralVerification(validator.requirement.Version) {
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
	if validator.requirement.Version == VersionV4 || validator.requirement.Version == VersionV5 {
		required = append(required,
			struct {
				path  string
				value bool
			}{"require_hierarchical_decomposition", acceptance.RequireHierarchicalDecomposition},
			struct {
				path  string
				value bool
			}{"require_interface_contracts", acceptance.RequireInterfaceContracts},
			struct {
				path  string
				value bool
			}{"require_shared_resource_planning", acceptance.RequireSharedResourcePlanning},
			struct {
				path  string
				value bool
			}{"require_deterministic_backtracking", acceptance.RequireDeterministicBacktracking},
			struct {
				path  string
				value bool
			}{"require_physical_partitioning", acceptance.RequirePhysicalPartitioning},
			struct {
				path  string
				value bool
			}{"require_end_to_end_traceability", acceptance.RequireEndToEndTraceability},
		)
	}
	if supportsDynamicVerification(validator.requirement.Version) {
		required = append(required,
			struct {
				path  string
				value bool
			}{"require_dynamic_model_provenance", acceptance.RequireDynamicModelProvenance},
			struct {
				path  string
				value bool
			}{"require_return_ratio_evidence", acceptance.RequireReturnRatioEvidence},
			struct {
				path  string
				value bool
			}{"require_dynamic_electrothermal_evidence", acceptance.RequireDynamicElectrothermalEvidence},
			struct {
				path  string
				value bool
			}{"require_event_coverage", acceptance.RequireEventCoverage},
			struct {
				path  string
				value bool
			}{"require_dynamic_architecture_selection", acceptance.RequireDynamicArchitectureSelection},
			struct {
				path  string
				value bool
			}{"require_bounded_dynamic_repair", acceptance.RequireBoundedDynamicRepair},
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
		if constraint.Unit != "" && !allowedUnitForVersion(constraint.Unit, validator.requirement.Version) {
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
		if required, ok := value.(bool); ok && required {
			break
		}
		if validator.requirement.Version == VersionV5 {
			if required, ok := value.(string); ok && validSemanticID(required) {
				break
			}
		}
		{
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

func allowedUnitForVersion(value string, version int) bool {
	return allowedUnit(value) || (version == VersionV5 && slices.Contains(registeredDynamicCanonicalUnits, value))
}

func supportsTypedSignals(version int) bool {
	return version == VersionV2 || version == VersionV3 || version == VersionV4 || version == VersionV5 || version == VersionV6
}

func supportsBehavioralVerification(version int) bool {
	return version == VersionV3 || version == VersionV4 || version == VersionV5 || version == VersionV6
}

func supportsDynamicVerification(version int) bool {
	return version == VersionV5
}

func supportsOperatingEvents(version int) bool {
	return version == VersionV5 || version == VersionV6
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
