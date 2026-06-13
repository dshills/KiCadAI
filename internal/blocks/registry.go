package blocks

import (
	"cmp"
	"context"
	"fmt"
	"regexp"
	"slices"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

var voltageLiteralPattern = regexp.MustCompile(`^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?\s*(?:[munpkgMUNPKGTFf]|\x{00B5}|\x{03BC})?[Vv]$`)
var currentLiteralPattern = regexp.MustCompile(`^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?\s*(?:[munpkgMUNPKGTFf]|\x{00B5}|\x{03BC})?[Aa]$`)

var _ Registry = BuiltinRegistry{}

type Registry interface {
	ListBlocks() []BlockSummary
	GetBlock(id string) (BlockDefinition, bool)
	ValidateDefinition(definition BlockDefinition) []reports.Issue
	ValidateRequest(request BlockRequest) []reports.Issue
	Instantiate(ctx context.Context, request BlockRequest) (BlockOutput, []reports.Issue)
}

type BuiltinRegistry struct {
	definitions   map[string]BlockDefinition
	instantiators map[string]BlockInstantiator
	summaries     []BlockSummary
	issues        []reports.Issue
}

type BlockInstantiator func(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput

func NewBuiltinRegistry() BuiltinRegistry {
	registry, issues := NewBuiltinRegistryChecked()
	if len(issues) != 0 {
		panic(fmt.Sprintf("invalid built-in circuit block definitions: %d issue(s)", len(issues)))
	}
	return registry
}

func NewBuiltinRegistryChecked() (BuiltinRegistry, []reports.Issue) {
	registry := NewRegistry(BuiltinDefinitions())
	registry.instantiators = map[string]BlockInstantiator{
		"connector_breakout": instantiateConnectorBreakout,
		"i2c_sensor":         instantiateI2CSensor,
		"led_indicator":      instantiateLEDIndicator,
		"voltage_regulator":  instantiateVoltageRegulator,
	}
	return registry, registry.Issues()
}

func NewRegistry(definitions []BlockDefinition) BuiltinRegistry {
	registry := BuiltinRegistry{definitions: map[string]BlockDefinition{}, instantiators: map[string]BlockInstantiator{}}
	for _, definition := range definitions {
		if issues := registry.ValidateDefinition(definition); len(issues) != 0 {
			registry.issues = append(registry.issues, issues...)
			continue
		}
		if _, exists := registry.definitions[definition.ID]; exists {
			registry.issues = append(registry.issues, blockIssue("block."+definition.ID, "duplicate block ID "+definition.ID))
			continue
		}
		registry.definitions[definition.ID] = cloneBlockDefinition(definition)
	}
	registry.summaries = sortedBlockSummaries(registry.definitions)
	return registry
}

func (registry BuiltinRegistry) Issues() []reports.Issue {
	return append([]reports.Issue(nil), registry.issues...)
}

func (registry BuiltinRegistry) ListBlocks() []BlockSummary {
	return append([]BlockSummary(nil), registry.summaries...)
}

func (registry BuiltinRegistry) GetBlock(id string) (BlockDefinition, bool) {
	definition, ok := registry.definitions[id]
	if !ok {
		return BlockDefinition{}, false
	}
	return cloneBlockDefinition(definition), true
}

func (registry BuiltinRegistry) ValidateDefinition(definition BlockDefinition) []reports.Issue {
	var issues []reports.Issue
	issues = append(issues, ValidateBlockID(definition.ID)...)
	parameters := map[string]BlockParameter{}
	for _, parameter := range definition.Parameters {
		if _, exists := parameters[parameter.Name]; exists {
			issues = append(issues, blockIssue("block."+definition.ID+".parameters."+parameter.Name, "duplicate parameter name "+parameter.Name))
		}
		parameters[parameter.Name] = parameter
		issues = append(issues, validateParameterDefault(definition.ID, parameter)...)
	}
	if definition.Name == "" {
		issues = append(issues, blockIssue("block."+definition.ID+".name", "block name is required"))
	}
	if definition.Version == "" {
		issues = append(issues, blockIssue("block."+definition.ID+".version", "block version is required"))
	}
	if definition.Verification.Level == "" {
		issues = append(issues, blockIssue("block."+definition.ID+".verification.level", "verification level is required"))
	}
	if len(definition.Parameters) == 0 {
		issues = append(issues, blockIssue("block."+definition.ID+".parameters", "block must define at least one parameter"))
	}
	if len(definition.Ports) == 0 {
		issues = append(issues, blockIssue("block."+definition.ID+".ports", "block must define at least one port"))
	}
	ports := map[string]struct{}{}
	for _, port := range definition.Ports {
		if _, exists := ports[port.Name]; exists {
			issues = append(issues, blockIssue("block."+definition.ID+".ports."+port.Name, "duplicate port name "+port.Name))
		}
		ports[port.Name] = struct{}{}
		if port.Voltage == "" {
			continue
		}
		parameter, ok := parameters[port.Voltage]
		if !ok {
			if isVoltageLiteral(port.Voltage) {
				continue
			}
			issues = append(issues, blockIssue("block."+definition.ID+".ports."+port.Name+".voltage", "port voltage references unknown parameter "+port.Voltage))
			continue
		}
		if parameter.Type != ParameterVoltage {
			issues = append(issues, blockIssue("block."+definition.ID+".ports."+port.Name+".voltage", "port voltage parameter "+port.Voltage+" must use voltage type"))
		}
	}
	return issues
}

func isVoltageLiteral(value string) bool {
	return voltageLiteralPattern.MatchString(value)
}

func isCurrentLiteral(value string) bool {
	return currentLiteralPattern.MatchString(value)
}

func sortedBlockSummaries(definitions map[string]BlockDefinition) []BlockSummary {
	summaries := make([]BlockSummary, 0, len(definitions))
	for _, definition := range definitions {
		summaries = append(summaries, Summary(definition))
	}
	slices.SortFunc(summaries, func(a, b BlockSummary) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return summaries
}

func cloneBlockDefinition(definition BlockDefinition) BlockDefinition {
	definition.Parameters = append([]BlockParameter(nil), definition.Parameters...)
	for i := range definition.Parameters {
		definition.Parameters[i].Default = cloneParameterValue(definition.Parameters[i].Default)
		definition.Parameters[i].Allowed = append([]any(nil), definition.Parameters[i].Allowed...)
		for j := range definition.Parameters[i].Allowed {
			definition.Parameters[i].Allowed[j] = cloneParameterValue(definition.Parameters[i].Allowed[j])
		}
		definition.Parameters[i].Affects = append([]string(nil), definition.Parameters[i].Affects...)
	}
	definition.Ports = append([]BlockPort(nil), definition.Ports...)
	definition.RequiredLibraries = append([]LibraryRequirement(nil), definition.RequiredLibraries...)
	definition.Components = append([]BlockComponent(nil), definition.Components...)
	for i := range definition.Components {
		definition.Components[i].Pins = append([]transactions.PinSpec(nil), definition.Components[i].Pins...)
		definition.Components[i].Properties = cloneStringMap(definition.Components[i].Properties)
		definition.Components[i].Alternatives = append([]string(nil), definition.Components[i].Alternatives...)
	}
	definition.Nets = append([]BlockNet(nil), definition.Nets...)
	for i := range definition.Nets {
		definition.Nets[i].Pins = append([]NetPin(nil), definition.Nets[i].Pins...)
		definition.Nets[i].Constraints = append([]string(nil), definition.Nets[i].Constraints...)
	}
	definition.SchematicHints = append([]SchematicHint(nil), definition.SchematicHints...)
	definition.PCBHints = append([]PCBHint(nil), definition.PCBHints...)
	definition.ValidationRules = append([]BlockValidationRule(nil), definition.ValidationRules...)
	definition.Verification.Evidence = append([]string(nil), definition.Verification.Evidence...)
	definition.Verification.Notes = append([]string(nil), definition.Verification.Notes...)
	return definition
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func (registry BuiltinRegistry) ValidateRequest(request BlockRequest) []reports.Issue {
	_, _, issues := registry.normalizeRequest(request)
	return issues
}

func (registry BuiltinRegistry) Instantiate(ctx context.Context, request BlockRequest) (BlockOutput, []reports.Issue) {
	if ctx == nil {
		issue := reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "block", Message: "context is required"}
		return BlockOutput{}, []reports.Issue{issue}
	}
	if ctx.Err() != nil {
		issue := reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "block", Message: ctx.Err().Error()}
		return BlockOutput{}, []reports.Issue{issue}
	}
	definition, params, issues := registry.normalizeRequest(request)
	if params == nil {
		return BlockOutput{}, issues
	}
	instanceID := request.InstanceID
	if instanceID == "" {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   reports.SeverityError,
			Path:       "instance_id",
			Message:    "instance ID is required",
			Suggestion: "Provide a unique instance_id for every block instance.",
		})
	}
	if instantiator, ok := registry.instantiators[definition.ID]; ok {
		output := instantiator(definition, request, params, issues)
		return output, output.Issues
	}
	output := BlockOutput{
		Definition: Summary(definition),
		Instance: BlockInstance{
			BlockID:    definition.ID,
			InstanceID: instanceID,
			Params:     params,
			Ports:      append([]BlockPort(nil), definition.Ports...),
		},
		Issues: issues,
	}
	return output, issues
}

func validateParameterDefault(blockID string, parameter BlockParameter) []reports.Issue {
	if parameter.Default == nil {
		return nil
	}
	path := "block." + blockID + ".parameters." + parameter.Name + ".default"
	if parameter.Type == ParameterEnum && !allowedValue(parameter.Allowed, parameter.Default) {
		return []reports.Issue{blockIssue(path, "enum default for "+parameter.Name+" is not allowed")}
	}
	if parameter.Type == ParameterVoltage || parameter.Type == ParameterCurrent {
		value, ok := parameter.Default.(string)
		if !ok {
			return []reports.Issue{blockIssue(path, "default for "+parameter.Name+" has invalid type")}
		}
		if parameter.Type == ParameterVoltage && !isVoltageLiteral(value) {
			return []reports.Issue{blockIssue(path, "voltage default for "+parameter.Name+" must include a voltage unit")}
		}
		if parameter.Type == ParameterCurrent && !isCurrentLiteral(value) {
			return []reports.Issue{blockIssue(path, "current default for "+parameter.Name+" must include a current unit")}
		}
		return nil
	}
	if issues := validateParameterValue(parameter, parameter.Default); len(issues) != 0 {
		return []reports.Issue{blockIssue(path, "default for "+parameter.Name+" has invalid type")}
	}
	return nil
}

func (registry BuiltinRegistry) normalizeRequest(request BlockRequest) (BlockDefinition, map[string]any, []reports.Issue) {
	definition, ok := registry.GetBlock(request.BlockID)
	if !ok {
		return BlockDefinition{}, nil, []reports.Issue{{
			Code:     reports.CodeMissingFile,
			Severity: reports.SeverityError,
			Path:     "block_id",
			Message:  "block not found: " + request.BlockID,
		}}
	}
	params := ApplyParameterDefaults(definition, request.Params)
	return definition, params, ValidateParameters(definition, params)
}
