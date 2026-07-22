package circuitgraph

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/reports"
)

func TestFunctionLevelCapabilitiesAreDeterministicAndValidate(t *testing.T) {
	first := FunctionLevelCapabilities()
	second := FunctionLevelCapabilities()
	if !reflect.DeepEqual(first, second) || first.Schema != FunctionCapabilitySchema || len(first.Operations) == 0 {
		t.Fatalf("unstable function capabilities: first=%#v second=%#v", first, second)
	}
	names := make([]string, 0, len(first.Operations))
	for _, capability := range first.Operations {
		names = append(names, capability.Name)
		if len(capability.SupportedRoles) == 0 {
			t.Fatalf("published operation %s has no supported roles", capability.Name)
		}
		function := FunctionRequirement{
			ID:         "device",
			Role:       capability.SupportedRoles[0],
			Usage:      capability.Name,
			Query:      &ComponentQuery{Family: "test"},
			Parameters: capabilityParameters(capability.RequiredParameters),
		}
		for _, endpoint := range capability.EndpointRoles {
			if endpoint.Required {
				function.RequiredFunctions = append(function.RequiredFunctions, endpoint.Functions[0])
			}
		}
		validator := graphValidator{}
		parameterNames := make(map[string]bool)
		for _, parameter := range function.Parameters {
			parameterNames[parameter.Name] = true
		}
		requiredFunctions := make(map[string]bool)
		for _, name := range function.RequiredFunctions {
			requiredFunctions[normalizedFunctionKey(name)] = true
		}
		validator.functionCapability("synthesis.functions[0]", function, parameterNames, requiredFunctions)
		if len(validator.issues) != 0 {
			t.Fatalf("published operation %s does not validate: %#v", capability.Name, validator.issues)
		}
	}
	if !slices.IsSorted(names) {
		t.Fatalf("operation names are not sorted: %v", names)
	}
	encoded, err := json.Marshal(first)
	if err != nil || !json.Valid(encoded) {
		t.Fatalf("invalid capability JSON: err=%v json=%s", err, encoded)
	}
}

func TestUnknownFunctionUsageFailsClosed(t *testing.T) {
	if !slices.IsSorted(legacyFunctionUsages) {
		t.Fatal("legacyFunctionUsages must remain sorted for binary search")
	}
	validator := graphValidator{}
	validator.functionCapability("synthesis.functions[0]", FunctionRequirement{Usage: "low_side_switsh"}, map[string]bool{}, map[string]bool{})
	if len(validator.issues) != 1 || validator.issues[0].Path != "synthesis.functions[0].usage" {
		t.Fatalf("unknown usage issues = %#v", validator.issues)
	}
}

func TestFunctionCapabilityValidationAggregatesIndependentMistakes(t *testing.T) {
	capability, ok := FunctionCapabilityForUsage("adjustable_linear_regulator")
	if !ok {
		t.Fatal("missing adjustable regulator capability")
	}
	function := FunctionRequirement{Role: RoleSensor, Usage: capability.Name}
	validator := graphValidator{}
	validator.functionCapability("synthesis.functions[0]", function, map[string]bool{}, map[string]bool{})
	if got, want := len(validator.issues), 6; got != want {
		t.Fatalf("independent issue count = %d, want %d: %#v", got, want, validator.issues)
	}
}

func TestFunctionDocumentValidationAggregatesIndependentSections(t *testing.T) {
	document := loadPublicFunctionExample(t)
	document.Project.Acceptance = "unsupported"
	document.Synthesis.Functions[3].Role = RoleSensor
	document.Synthesis.Functions[3].Usage = "adjustable_linear_regulator"
	document.Synthesis.Functions[3].Parameters = nil
	document.Synthesis.Functions[3].RequiredFunctions = nil
	document.Synthesis.Interfaces[0].Role = "unsupported"
	document.Synthesis.PowerDomains[0].Source = "unsupported"
	document.Synthesis.Connections[0].CurrentMA = -1
	document.Synthesis.Constraints.MaxWidthMM = -1

	issues := Validate(document)
	for _, path := range []string{
		"project.acceptance",
		"synthesis.functions[3].role",
		"synthesis.functions[3].parameters",
		"synthesis.functions[3].required_functions",
		"synthesis.interfaces[0].role",
		"synthesis.power_domains[0].source",
		"synthesis.connections[0].current_ma",
		"synthesis.constraints",
	} {
		if !hasIssuePath(issues, path) {
			t.Fatalf("missing independent issue path %q in %#v", path, issues)
		}
	}
}

func TestFunctionLevelCapabilityCopiesCannotMutateRegistry(t *testing.T) {
	first := FunctionLevelCapabilities()
	if len(first.Operations) == 0 || len(first.Operations[0].EndpointRoles) == 0 || len(first.Operations[0].EndpointRoles[0].Functions) == 0 || len(first.Operations[0].RequiredParameters) == 0 {
		t.Fatal("first published operation must retain nested metadata for copy isolation test")
	}
	first.Operations[0].EndpointRoles[0].Functions[0] = "MUTATED"
	first.Operations[0].RequiredParameters[0].Name = "mutated"
	second := FunctionLevelCapabilities()
	if second.Operations[0].EndpointRoles[0].Functions[0] == "MUTATED" || second.Operations[0].RequiredParameters[0].Name == "mutated" {
		t.Fatal("caller mutated authoritative function registry")
	}
}

func capabilityParameters(capabilities []FunctionParameter) []Parameter {
	parameters := make([]Parameter, 0, len(capabilities))
	for _, capability := range capabilities {
		value := "1"
		parameters = append(parameters, Parameter{Name: capability.Name, Value: ParameterValue{String: &value}})
	}
	return parameters
}

func hasIssuePath(issues []reports.Issue, path string) bool {
	return slices.ContainsFunc(issues, func(issue reports.Issue) bool { return issue.Path == path })
}
