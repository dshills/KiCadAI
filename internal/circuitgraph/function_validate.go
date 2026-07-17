package circuitgraph

import (
	"fmt"
	"math"
	"strings"

	"kicadai/internal/reports"
)

func validateFunctionDocument(document Document) []reports.Issue {
	validator := graphValidator{document: document}
	if document.Schema != SchemaID {
		validator.add(CodeSynthesisIntentInvalid, "schema", "schema must be "+SchemaID)
	}
	if document.Version != Version {
		validator.add(CodeSynthesisIntentInvalid, "version", fmt.Sprintf("version must be %d", Version))
	}
	project := document.Project
	if !projectNamePattern.MatchString(project.Name) || strings.Contains(project.Name, "..") {
		validator.add(CodeSynthesisIntentInvalid, "project.name", "project name must be a safe basename")
	}
	validator.boundedString("project.title", project.Title, MaxStringBytes)
	validator.boundedString("project.description", project.Description, MaxDescriptionBytes)
	if !validAcceptance(project.Acceptance) {
		validator.add(CodeSynthesisIntentInvalid, "project.acceptance", "unsupported acceptance level")
	}
	if project.Board != (Board{}) {
		validator.add(CodeSynthesisIntentInvalid, "project.board", "function-level intent cannot supply board dimensions or layers")
	}
	if hasExplicitGraphFields(document) {
		validator.add(CodeSynthesisIntentInvalid, "document", "function-level intent cannot mix explicit components, nets, layout, simulation, or KiCad directives")
	}
	validator.functionIntent(*document.Synthesis)
	validator.policy()
	return finalizeGraphIssues(validator.issues)
}

func hasExplicitGraphFields(document Document) bool {
	return len(document.Components) != 0 || len(document.Nets) != 0 || len(document.NoConnects) != 0 ||
		len(document.PowerFlags) != 0 || len(document.Buses) != 0 || document.Simulation != nil ||
		len(document.Schematic.Groups) != 0 || len(document.Schematic.Placements) != 0 ||
		document.Schematic.Flow != "" || document.Schematic.Origin != "" || document.Schematic.Lanes.Power != "" ||
		document.Schematic.Lanes.PowerNegative != nil || document.Schematic.Lanes.Signals != "" || document.Schematic.Lanes.Ground != "" ||
		len(document.PCB.Regions) != 0 || len(document.PCB.Placements) != 0 || len(document.PCB.Keepouts) != 0 || len(document.PCB.Zones) != 0
}

func (validator *graphValidator) functionIntent(intent FunctionIntent) {
	if len(intent.Functions) == 0 {
		validator.add(CodeSynthesisIntentInvalid, "synthesis.functions", "at least one primary function is required")
	}
	if len(intent.Functions) > MaxComponents {
		validator.add(CodeLimitExceeded, "synthesis.functions", "primary function count exceeds component limit")
	}
	functions := map[string]FunctionRequirement{}
	for index, function := range intent.Functions {
		path := fmt.Sprintf("synthesis.functions[%d]", index)
		if !graphIDPattern.MatchString(function.ID) {
			validator.add(CodeSynthesisIntentInvalid, path+".id", "function id must be a safe identifier")
		} else if _, exists := functions[function.ID]; exists {
			validator.add(CodeSynthesisIntentInvalid, path+".id", "duplicate function id "+function.ID)
		}
		functions[function.ID] = function
		if !validComponentRole(function.Role) {
			validator.add(CodeSynthesisIntentInvalid, path+".role", "unsupported component role")
		}
		hasID := strings.TrimSpace(function.ComponentID) != ""
		hasQuery := function.Query != nil
		if hasID == hasQuery {
			validator.add(CodeSynthesisIntentInvalid, path, "declare exactly one of component_id or query")
		}
		if hasQuery {
			validator.query(path+".query", *function.Query)
		}
		validator.boundedString(path+".value", function.Value, MaxStringBytes)
		if function.Usage != "" && !graphIDPattern.MatchString(function.Usage) {
			validator.add(CodeSynthesisIntentInvalid, path+".usage", "usage must be a safe semantic identifier")
		}
		parameterNames := map[string]bool{}
		for parameterIndex, parameter := range function.Parameters {
			parameterPath := fmt.Sprintf("%s.parameters[%d]", path, parameterIndex)
			if !graphIDPattern.MatchString(parameter.Name) || parameterNames[parameter.Name] {
				validator.add(CodeSynthesisIntentInvalid, parameterPath+".name", "parameter name must be a unique safe identifier")
			}
			parameterNames[parameter.Name] = true
			validator.parameterValue(parameterPath+".value", parameter.Value)
		}
		for ratingIndex, rating := range function.RequiredRatings {
			if strings.TrimSpace(rating.Kind) == "" || strings.TrimSpace(rating.Value) == "" || strings.TrimSpace(rating.Unit) == "" {
				validator.add(CodeSynthesisIntentInvalid, fmt.Sprintf("%s.required_ratings[%d]", path, ratingIndex), "rating kind, value, and unit are required")
			}
		}
		seenRequired := map[string]bool{}
		for requiredIndex, required := range function.RequiredFunctions {
			key := normalizedFunctionKey(required)
			if key == "" || seenRequired[key] {
				validator.add(CodeSynthesisIntentInvalid, fmt.Sprintf("%s.required_functions[%d]", path, requiredIndex), "required function must be non-empty and unique")
			}
			seenRequired[key] = true
		}
	}

	interfaces := map[string]map[string]bool{}
	for index, candidate := range intent.Interfaces {
		path := fmt.Sprintf("synthesis.interfaces[%d]", index)
		if !graphIDPattern.MatchString(candidate.ID) {
			validator.add(CodeSynthesisIntentInvalid, path+".id", "interface id must be a safe identifier")
		} else if _, exists := interfaces[candidate.ID]; exists {
			validator.add(CodeSynthesisIntentInvalid, path+".id", "duplicate interface id "+candidate.ID)
		}
		if !validInterfaceRole(candidate.Role) {
			validator.add(CodeSynthesisInterfaceUnsupported, path+".role", "unsupported interface role")
		}
		if len(candidate.Signals) == 0 || len(candidate.Signals) > MaxFunctionInterfaceSignals {
			validator.add(CodeSynthesisInterfaceUnsupported, path+".signals", fmt.Sprintf("interface must contain between one and %d signals", MaxFunctionInterfaceSignals))
		}
		signals := map[string]bool{}
		interfaces[candidate.ID] = signals
		for signalIndex, signal := range candidate.Signals {
			signalPath := fmt.Sprintf("%s.signals[%d]", path, signalIndex)
			if !semanticIDPattern.MatchString(signal.Name) || signals[signal.Name] {
				validator.add(CodeSynthesisIntentInvalid, signalPath+".name", "signal name must be a unique safe identifier")
			}
			signals[signal.Name] = true
			if !validNetRole(signal.Role) {
				validator.add(CodeSynthesisIntentInvalid, signalPath+".role", "unsupported signal role")
			}
		}
	}

	domains := map[string]PowerDomainIntent{}
	for index, domain := range intent.PowerDomains {
		path := fmt.Sprintf("synthesis.power_domains[%d]", index)
		if !semanticIDPattern.MatchString(domain.Name) {
			validator.add(CodeSynthesisPowerDomainInvalid, path+".name", "power-domain name must be a safe identifier")
		} else if _, exists := domains[domain.Name]; exists {
			validator.add(CodeSynthesisPowerDomainInvalid, path+".name", "duplicate power domain "+domain.Name)
		}
		domains[domain.Name] = domain
		if !validSynthesisPowerRole(domain.Role) {
			validator.add(CodeSynthesisPowerDomainInvalid, path+".role", "power domain role must be power, power_pos, power_neg, ground, or return")
		}
		if !finiteInRange(domain.VoltageV, -MaxSynthesisVoltageV, MaxSynthesisVoltageV, true) {
			validator.add(CodeSynthesisPowerDomainInvalid, path+".voltage_v", "power-domain voltage must be finite and bounded")
		}
		if !finiteInRange(domain.MaxCurrentMA, 0, math.MaxFloat64, true) {
			validator.add(CodeSynthesisPowerDomainInvalid, path+".max_current_ma", "power-domain current must be finite and non-negative")
		}
		if domain.Source != PowerDomainExternal && domain.Source != PowerDomainGenerated {
			validator.add(CodeSynthesisPowerDomainInvalid, path+".source", "power-domain source must be external or generated")
		}
	}

	connectionNames := map[string]bool{}
	connectedPorts := map[string]string{}
	for index, connection := range intent.Connections {
		path := fmt.Sprintf("synthesis.connections[%d]", index)
		if strings.TrimSpace(connection.Name) == "" || connectionNames[connection.Name] {
			validator.add(CodeSynthesisIntentInvalid, path+".name", "connection name must be non-empty and unique")
		}
		connectionNames[connection.Name] = true
		validator.boundedString(path+".name", connection.Name, MaxStringBytes)
		if !validNetRole(connection.Role) {
			validator.add(CodeSynthesisConnectionUnresolved, path+".role", "unsupported connection role")
		}
		if connection.VoltageDomain != "" {
			if _, exists := domains[connection.VoltageDomain]; !exists {
				validator.add(CodeSynthesisPowerDomainInvalid, path+".voltage_domain", "connection references unknown power domain "+connection.VoltageDomain)
			}
		}
		if !finiteInRange(connection.CurrentMA, 0, math.MaxFloat64, true) {
			validator.add(CodeSynthesisConnectionUnresolved, path+".current_ma", "connection current must be finite and non-negative")
		}
		if len(connection.Endpoints) < 2 || len(connection.Endpoints) > MaxEndpointsPerNet {
			validator.add(CodeSynthesisConnectionUnresolved, path+".endpoints", "connection requires between two and the endpoint limit")
		}
		seenEndpoints := map[string]bool{}
		for endpointIndex, endpoint := range connection.Endpoints {
			endpointPath := fmt.Sprintf("%s.endpoints[%d]", path, endpointIndex)
			functionEndpoint := endpoint.Function != "" || endpoint.Port != ""
			interfaceEndpoint := endpoint.Interface != "" || endpoint.Signal != ""
			if functionEndpoint == interfaceEndpoint {
				validator.add(CodeSynthesisConnectionUnresolved, endpointPath, "endpoint must declare exactly one function/port or interface/signal pair")
				continue
			}
			key := ""
			if functionEndpoint {
				if endpoint.Function == "" || endpoint.Port == "" {
					validator.add(CodeSynthesisConnectionUnresolved, endpointPath, "function endpoint requires function and semantic port")
					continue
				}
				if _, exists := functions[endpoint.Function]; !exists {
					validator.add(CodeSynthesisConnectionUnresolved, endpointPath+".function", "endpoint references unknown function "+endpoint.Function)
				}
				key = "function:" + endpoint.Function + ":" + normalizedFunctionKey(endpoint.Port)
				if previous := connectedPorts[key]; previous != "" && previous != connection.Name {
					validator.add(CodeSynthesisConnectionUnresolved, endpointPath, "semantic function port is already connected by "+previous)
				}
				connectedPorts[key] = connection.Name
			} else {
				if endpoint.Interface == "" || endpoint.Signal == "" {
					validator.add(CodeSynthesisConnectionUnresolved, endpointPath, "interface endpoint requires interface and signal")
					continue
				}
				signals, exists := interfaces[endpoint.Interface]
				if !exists || !signals[endpoint.Signal] {
					validator.add(CodeSynthesisConnectionUnresolved, endpointPath, "endpoint references unknown interface signal")
				}
				key = "interface:" + endpoint.Interface + ":" + endpoint.Signal
				if previous := connectedPorts[key]; previous != "" && previous != connection.Name {
					validator.add(CodeSynthesisConnectionUnresolved, endpointPath, "interface signal is already connected by "+previous)
				}
				connectedPorts[key] = connection.Name
			}
			if seenEndpoints[key] {
				validator.add(CodeSynthesisConnectionUnresolved, endpointPath, "duplicate connection endpoint")
			}
			seenEndpoints[key] = true
		}
	}

	constraints := intent.Constraints
	if !finiteInRange(constraints.MaxWidthMM, 0, MaxBoardDimensionMM, false) || !finiteInRange(constraints.MaxHeightMM, 0, MaxBoardDimensionMM, false) {
		validator.add(CodeSynthesisLayoutConstraintUnsupported, "synthesis.constraints", "maximum board dimensions must be finite, positive, and bounded")
	}
	if !finiteInRange(constraints.PreferredComponentSpacingMM, 0, MaxBoardDimensionMM, true) {
		validator.add(CodeSynthesisLayoutConstraintUnsupported, "synthesis.constraints.preferred_component_spacing_mm", "preferred spacing must be finite and non-negative")
	}
	if constraints.Protection != "" && constraints.Protection != "optional" && constraints.Protection != "required" {
		validator.add(CodeSynthesisIntentInvalid, "synthesis.constraints.protection", "protection must be optional or required")
	}
}

func validInterfaceRole(role InterfaceRole) bool {
	switch role {
	case InterfacePowerInput, InterfacePowerOutput, InterfaceAnalogInput, InterfaceAnalogOut,
		InterfaceDigitalIn, InterfaceDigitalOut, InterfaceI2C, InterfaceSPI, InterfaceUART,
		InterfaceGPIO, InterfaceProgramming:
		return true
	default:
		return false
	}
}

func validSynthesisPowerRole(role NetRole) bool {
	switch role {
	case NetRolePower, NetRolePowerPos, NetRolePowerNeg, NetRoleGround, NetRoleReturn:
		return true
	default:
		return false
	}
}
