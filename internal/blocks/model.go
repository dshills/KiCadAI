package blocks

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type VerificationLevel string

const (
	VerificationExperimental      VerificationLevel = "experimental"
	VerificationStructural        VerificationLevel = "structural"
	VerificationRoundTripVerified VerificationLevel = "roundtrip_verified"
	VerificationERCDRCVerified    VerificationLevel = "erc_drc_verified"
	VerificationReferenceVerified VerificationLevel = "reference_verified"
)

// AllowsFabricationReadinessClaim reports whether this verification level has
// enough evidence to permit a fabrication-readiness claim.
func (level VerificationLevel) AllowsFabricationReadinessClaim() bool {
	switch level {
	case VerificationRoundTripVerified, VerificationERCDRCVerified, VerificationReferenceVerified:
		return true
	default:
		return false
	}
}

type BlockParameterType string

const (
	ParameterString      BlockParameterType = "string"
	ParameterEnum        BlockParameterType = "enum"
	ParameterNumber      BlockParameterType = "number"
	ParameterBool        BlockParameterType = "bool"
	ParameterVoltage     BlockParameterType = "voltage"
	ParameterCurrent     BlockParameterType = "current"
	ParameterResistance  BlockParameterType = "resistance"
	ParameterCapacitance BlockParameterType = "capacitance"
	ParameterFrequency   BlockParameterType = "frequency"
	ParameterFootprintID BlockParameterType = "footprint_id"
	ParameterSymbolID    BlockParameterType = "symbol_id"
	ParameterStringList  BlockParameterType = "string_list"
)

type PortDirection string

const (
	PortInput         PortDirection = "input"
	PortOutput        PortDirection = "output"
	PortBidirectional PortDirection = "bidirectional"
	PortPassive       PortDirection = "passive"
	PortPower         PortDirection = "power"
)

type BlockDefinition struct {
	ID                string                `json:"id"`
	Name              string                `json:"name"`
	Description       string                `json:"description,omitempty"`
	Version           string                `json:"version"`
	Category          string                `json:"category,omitempty"`
	Parameters        []BlockParameter      `json:"parameters,omitempty"`
	Ports             []BlockPort           `json:"ports,omitempty"`
	RequiredLibraries []LibraryRequirement  `json:"required_libraries,omitempty"`
	Components        []BlockComponent      `json:"components,omitempty"`
	Nets              []BlockNet            `json:"nets,omitempty"`
	SchematicHints    []SchematicHint       `json:"schematic_hints,omitempty"`
	PCBHints          []PCBHint             `json:"pcb_hints,omitempty"`
	PCBRealization    *PCBRealization       `json:"pcb_realization,omitempty"`
	ValidationRules   []BlockValidationRule `json:"validation_rules,omitempty"`
	Verification      VerificationRecord    `json:"verification"`
}

type BlockSummary struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Description       string            `json:"description,omitempty"`
	Version           string            `json:"version"`
	Category          string            `json:"category,omitempty"`
	VerificationLevel VerificationLevel `json:"verification_level"`
}

type BlockParameter struct {
	Name        string             `json:"name"`
	Type        BlockParameterType `json:"type"`
	Default     any                `json:"default,omitempty"`
	Required    bool               `json:"required,omitempty"`
	Allowed     []any              `json:"allowed,omitempty"`
	Min         *float64           `json:"min,omitempty"`
	Max         *float64           `json:"max,omitempty"`
	Units       string             `json:"units,omitempty"`
	Affects     []string           `json:"affects,omitempty"`
	Description string             `json:"description,omitempty"`
}

type BlockPort struct {
	Name        string        `json:"name"`
	Direction   PortDirection `json:"direction,omitempty"`
	NetClass    string        `json:"net_class,omitempty"`
	Voltage     string        `json:"voltage,omitempty"`
	Description string        `json:"description,omitempty"`
}

type LibraryRequirement struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

type BlockComponent struct {
	Role              string                     `json:"role"`
	RefPrefix         string                     `json:"ref_prefix"`
	Value             string                     `json:"value,omitempty"`
	SymbolID          string                     `json:"symbol_id"`
	FootprintID       string                     `json:"footprint_id,omitempty"`
	Pins              []transactions.PinSpec     `json:"pins,omitempty"`
	Properties        map[string]string          `json:"properties,omitempty"`
	PinmapRequired    bool                       `json:"pinmap_required,omitempty"`
	PlacementGroup    string                     `json:"placement_group,omitempty"`
	Alternatives      []string                   `json:"alternatives,omitempty"`
	ComponentID       string                     `json:"component_id,omitempty"`
	ComponentQuery    *components.Query          `json:"component_query,omitempty"`
	ComponentVariant  string                     `json:"component_variant,omitempty"`
	MinimumConfidence components.ConfidenceLevel `json:"minimum_confidence,omitempty"`
	Acceptance        components.AcceptanceLevel `json:"acceptance,omitempty"`
}

type BlockNet struct {
	NameTemplate string   `json:"name_template"`
	Visibility   string   `json:"visibility,omitempty"`
	Role         string   `json:"role,omitempty"`
	Pins         []NetPin `json:"pins,omitempty"`
	Constraints  []string `json:"constraints,omitempty"`
}

type NetPin struct {
	ComponentRole string `json:"component_role"`
	Pin           string `json:"pin"`
}

type SchematicHint struct {
	Kind          string  `json:"kind"`
	ComponentRole string  `json:"component_role,omitempty"`
	XMM           float64 `json:"x_mm,omitempty"`
	YMM           float64 `json:"y_mm,omitempty"`
	Note          string  `json:"note,omitempty"`
}

type PCBHint struct {
	Kind          string  `json:"kind"`
	ComponentRole string  `json:"component_role,omitempty"`
	XMM           float64 `json:"x_mm,omitempty"`
	YMM           float64 `json:"y_mm,omitempty"`
	RotationDeg   float64 `json:"rotation_deg,omitempty"`
	Layer         string  `json:"layer,omitempty"`
	Note          string  `json:"note,omitempty"`
}

type BlockValidationRule struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

type VerificationRecord struct {
	Level       VerificationLevel `json:"level"`
	Date        string            `json:"date,omitempty"`
	KiCadTarget string            `json:"kicad_target,omitempty"`
	Evidence    []string          `json:"evidence,omitempty"`
	Notes       []string          `json:"notes,omitempty"`
}

type BlockRequest struct {
	BlockID    string         `json:"block_id"`
	InstanceID string         `json:"instance_id,omitempty"`
	Params     map[string]any `json:"params,omitempty"`
}

type BlockInstance struct {
	BlockID    string         `json:"block_id"`
	InstanceID string         `json:"instance_id"`
	Params     map[string]any `json:"params,omitempty"`
	Ports      []BlockPort    `json:"ports,omitempty"`
	Refs       []string       `json:"refs,omitempty"`
	Nets       []string       `json:"nets,omitempty"`
}

type BlockOutput struct {
	Definition BlockSummary             `json:"definition"`
	Instance   BlockInstance            `json:"instance"`
	Operations []transactions.Operation `json:"operations,omitempty"`
	Issues     []reports.Issue          `json:"issues,omitempty"`
}

var blockIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func Summary(definition BlockDefinition) BlockSummary {
	return BlockSummary{
		ID:                definition.ID,
		Name:              definition.Name,
		Description:       definition.Description,
		Version:           definition.Version,
		Category:          definition.Category,
		VerificationLevel: definition.Verification.Level,
	}
}

func ValidateBlockID(id string) []reports.Issue {
	if id == "" {
		return []reports.Issue{blockIssue("block.id", "block ID is required")}
	}
	if strings.TrimSpace(id) != id {
		return []reports.Issue{blockIssue("block.id", "block ID must not contain leading or trailing whitespace")}
	}
	if !blockIDPattern.MatchString(id) {
		return []reports.Issue{blockIssue("block.id", "block ID must use lowercase letters, digits, and underscores")}
	}
	return nil
}

func ApplyParameterDefaults(definition BlockDefinition, params map[string]any) map[string]any {
	normalized := map[string]any{}
	for key, value := range params {
		normalized[key] = cloneParameterValue(value)
	}
	for _, parameter := range definition.Parameters {
		if _, ok := normalized[parameter.Name]; ok {
			continue
		}
		if parameter.Default != nil {
			normalized[parameter.Name] = cloneParameterValue(parameter.Default)
		}
	}
	return normalized
}

func ValidateParameters(definition BlockDefinition, params map[string]any) []reports.Issue {
	var issues []reports.Issue
	known := map[string]BlockParameter{}
	for _, parameter := range definition.Parameters {
		known[parameter.Name] = parameter
	}
	for name := range params {
		if _, ok := known[name]; !ok {
			issues = append(issues, blockIssue("params."+name, "unknown parameter "+name))
		}
	}
	for _, parameter := range definition.Parameters {
		value, ok := params[parameter.Name]
		if !ok {
			if parameter.Required && parameter.Default == nil {
				issues = append(issues, blockIssue("params."+parameter.Name, "required parameter is missing"))
			}
			continue
		}
		issues = append(issues, validateParameterValue(parameter, value)...)
	}
	return issues
}

func validateParameterValue(parameter BlockParameter, value any) []reports.Issue {
	path := "params." + parameter.Name
	switch parameter.Type {
	case ParameterString, ParameterVoltage, ParameterCurrent, ParameterResistance, ParameterCapacitance, ParameterFrequency, ParameterFootprintID, ParameterSymbolID:
		if _, ok := value.(string); !ok {
			return []reports.Issue{blockIssue(path, fmt.Sprintf("parameter %s must be a string", parameter.Name))}
		}
	case ParameterEnum:
		if len(parameter.Allowed) == 0 {
			return []reports.Issue{blockIssue(path, "enum parameter has no allowed values")}
		}
		if !allowedValue(parameter.Allowed, value) {
			return []reports.Issue{blockIssue(path, fmt.Sprintf("parameter %s has unsupported value", parameter.Name))}
		}
	case ParameterNumber:
		number, ok := numericValue(value)
		if !ok {
			return []reports.Issue{blockIssue(path, fmt.Sprintf("parameter %s must be a number", parameter.Name))}
		}
		if parameter.Min != nil && number < *parameter.Min {
			return []reports.Issue{blockIssue(path, fmt.Sprintf("parameter %s is below minimum", parameter.Name))}
		}
		if parameter.Max != nil && number > *parameter.Max {
			return []reports.Issue{blockIssue(path, fmt.Sprintf("parameter %s is above maximum", parameter.Name))}
		}
	case ParameterBool:
		if _, ok := value.(bool); !ok {
			return []reports.Issue{blockIssue(path, fmt.Sprintf("parameter %s must be a bool", parameter.Name))}
		}
	case ParameterStringList:
		if !isStringList(value) {
			return []reports.Issue{blockIssue(path, fmt.Sprintf("parameter %s must be a string list", parameter.Name))}
		}
		if parameter.Required && stringListLen(value) == 0 {
			return []reports.Issue{blockIssue(path, fmt.Sprintf("parameter %s must not be empty", parameter.Name))}
		}
	default:
		return []reports.Issue{blockIssue(path, "unsupported parameter type "+string(parameter.Type))}
	}
	return nil
}

func Parameter(definition BlockDefinition, name string) (BlockParameter, bool) {
	for _, parameter := range definition.Parameters {
		if parameter.Name == name {
			return parameter, true
		}
	}
	return BlockParameter{}, false
}

func SortedParamKeys(params map[string]any) []string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case json.Number:
		number, err := typed.Float64()
		return number, err == nil
	default:
		return 0, false
	}
}

func allowedValue(allowed []any, value any) bool {
	for _, candidate := range allowed {
		if scalarEqual(candidate, value) {
			return true
		}
	}
	return false
}

func scalarEqual(left any, right any) bool {
	if left == nil && right == nil {
		return true
	}
	if leftString, ok := left.(string); ok {
		rightString, ok := right.(string)
		return ok && leftString == rightString
	}
	if leftBool, ok := left.(bool); ok {
		rightBool, ok := right.(bool)
		return ok && leftBool == rightBool
	}
	leftNumber, leftOK := numericValue(left)
	rightNumber, rightOK := numericValue(right)
	if leftOK || rightOK {
		if !leftOK || !rightOK {
			return false
		}
		scale := math.Max(1, math.Max(math.Abs(leftNumber), math.Abs(rightNumber)))
		return math.Abs(leftNumber-rightNumber) <= 1e-15*scale
	}
	return false
}

func cloneParameterValue(value any) any {
	if value == nil {
		return value
	}
	cloned := cloneReflectValue(reflect.ValueOf(value))
	return cloned.Interface()
}

func cloneReflectValue(value reflect.Value) reflect.Value {
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := cloneReflectValue(value.Elem())
		if cloned.Type().AssignableTo(value.Type()) {
			return cloned
		}
		wrapped := reflect.New(value.Type()).Elem()
		wrapped.Set(cloned)
		return wrapped
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.New(value.Type().Elem())
		cloned.Elem().Set(cloneAssignableValue(value.Elem(), value.Type().Elem()))
		return cloned
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := 0; index < value.Len(); index++ {
			cloned.Index(index).Set(cloneAssignableValue(value.Index(index), value.Type().Elem()))
		}
		return cloned
	case reflect.Array:
		cloned := reflect.New(value.Type()).Elem()
		for index := 0; index < value.Len(); index++ {
			cloned.Index(index).Set(cloneAssignableValue(value.Index(index), value.Type().Elem()))
		}
		return cloned
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.MakeMapWithSize(value.Type(), value.Len())
		for _, key := range value.MapKeys() {
			cloned.SetMapIndex(key, cloneAssignableValue(value.MapIndex(key), value.Type().Elem()))
		}
		return cloned
	default:
		return value
	}
}

func cloneAssignableValue(value reflect.Value, target reflect.Type) reflect.Value {
	cloned := cloneReflectValue(value)
	if cloned.Type().AssignableTo(target) {
		return cloned
	}
	if cloned.Type().ConvertibleTo(target) {
		return cloned.Convert(target)
	}
	return value
}

func isStringList(value any) bool {
	switch typed := value.(type) {
	case []string:
		return true
	case []any:
		for _, item := range typed {
			if _, ok := item.(string); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func stringListLen(value any) int {
	switch typed := value.(type) {
	case []string:
		return len(typed)
	case []any:
		return len(typed)
	default:
		return 0
	}
}

func blockIssue(path string, message string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     path,
		Message:  message,
	}
}
