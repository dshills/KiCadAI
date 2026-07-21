package architecturesearch

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const FragmentRealizationSchema = "kicadai.fragment-realization.v3"

// FragmentRealization is the identity-neutral output contract between an
// architecture provider and function-level lowering. It contains semantic
// component functions only; KiCad pins, footprints, coordinates, and routes
// remain the responsibility of the existing resolver and writer pipeline.
type FragmentRealization struct {
	Schema            string                        `json:"schema"`
	Capability        string                        `json:"capability"`
	Instances         []RealizationInstance         `json:"instances"`
	PortBindings      []RealizationPortBinding      `json:"port_bindings"`
	SeriesTransitions []RealizationSeriesTransition `json:"series_transitions,omitempty"`
	Connections       []RealizationConnection       `json:"connections,omitempty"`
	Parameters        []RealizationParameter        `json:"parameters,omitempty"`
	RepairVariables   []RealizationRepairVariable   `json:"repair_variables,omitempty"`
}

type RealizationInstance struct {
	ID                string   `json:"id"`
	CatalogID         string   `json:"catalog_id"`
	VariantID         string   `json:"variant_id,omitempty"`
	Usage             string   `json:"usage"`
	Value             string   `json:"value,omitempty"`
	RequiredFunctions []string `json:"required_functions,omitempty"`
}

type RealizationPortBinding struct {
	Role     string `json:"role"`
	Lane     string `json:"lane,omitempty"`
	Instance string `json:"instance"`
	Function string `json:"function"`
}

// RealizationSeriesTransition places a two-terminal element in series with an
// obligation anchor. The input is the source-facing endpoint and the output is
// the load-facing endpoint. Lowering keeps the external anchor and every other
// consumer on opposite sides of the element instead of shorting both endpoints
// onto the shared semantic net.
type RealizationSeriesTransition struct {
	Role   string              `json:"role"`
	Lane   string              `json:"lane,omitempty"`
	Input  RealizationEndpoint `json:"input"`
	Output RealizationEndpoint `json:"output"`
}

// RealizationConnection records a provider-selected semantic net. Endpoints
// name catalog functions, never KiCad pins or pads. Port bindings may attach
// an endpoint to an obligation anchor during composition lowering.
type RealizationConnection struct {
	ID        string                `json:"id"`
	Role      string                `json:"role"`
	Endpoints []RealizationEndpoint `json:"endpoints"`
}

type RealizationEndpoint struct {
	Instance string `json:"instance"`
	Function string `json:"function"`
}

type RealizationParameter struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

// RealizationRepairVariable exposes a bounded provider-calculated design
// degree of freedom to the trusted closed loop. It names only a local semantic
// instance and registered behavioral effects; lowering remains responsible
// for namespacing it and simulation remains authoritative for every trial.
type RealizationRepairVariable struct {
	ID            string                    `json:"id"`
	Kind          string                    `json:"kind"`
	Instance      string                    `json:"instance"`
	Value         float64                   `json:"value"`
	AllowedValues []float64                 `json:"allowed_values"`
	Unit          string                    `json:"unit"`
	Effects       []RealizationRepairEffect `json:"effects"`
}

type RealizationRepairEffect struct {
	Analysis  string `json:"analysis"`
	Metric    string `json:"metric"`
	Direction string `json:"direction"`
}

func MarshalFragmentRealization(realization FragmentRealization) (json.RawMessage, error) {
	realization.Schema = FragmentRealizationSchema
	realization.Capability = canonicalIdentifier(realization.Capability)
	for index := range realization.Instances {
		instance := &realization.Instances[index]
		instance.ID = canonicalIdentifier(instance.ID)
		instance.CatalogID = strings.TrimSpace(instance.CatalogID)
		instance.VariantID = strings.TrimSpace(instance.VariantID)
		instance.Usage = canonicalIdentifier(instance.Usage)
		instance.Value = strings.TrimSpace(instance.Value)
		instance.RequiredFunctions = normalizeFunctionSet(instance.RequiredFunctions)
	}
	for index := range realization.PortBindings {
		binding := &realization.PortBindings[index]
		binding.Role = canonicalIdentifier(binding.Role)
		binding.Lane = canonicalIdentifier(binding.Lane)
		binding.Instance = canonicalIdentifier(binding.Instance)
		binding.Function = strings.ToUpper(strings.TrimSpace(binding.Function))
	}
	for index := range realization.SeriesTransitions {
		transition := &realization.SeriesTransitions[index]
		transition.Role = canonicalIdentifier(transition.Role)
		transition.Lane = canonicalIdentifier(transition.Lane)
		transition.Input.Instance = canonicalIdentifier(transition.Input.Instance)
		transition.Input.Function = strings.ToUpper(strings.TrimSpace(transition.Input.Function))
		transition.Output.Instance = canonicalIdentifier(transition.Output.Instance)
		transition.Output.Function = strings.ToUpper(strings.TrimSpace(transition.Output.Function))
	}
	for index := range realization.Connections {
		connection := &realization.Connections[index]
		connection.ID = canonicalIdentifier(connection.ID)
		connection.Role = canonicalIdentifier(connection.Role)
		for endpointIndex := range connection.Endpoints {
			endpoint := &connection.Endpoints[endpointIndex]
			endpoint.Instance = canonicalIdentifier(endpoint.Instance)
			endpoint.Function = strings.ToUpper(strings.TrimSpace(endpoint.Function))
		}
		slices.SortStableFunc(connection.Endpoints, compareRealizationEndpoints)
	}
	functionsByInstance := map[string][]string{}
	for _, binding := range realization.PortBindings {
		functionsByInstance[binding.Instance] = append(functionsByInstance[binding.Instance], binding.Function)
	}
	for _, transition := range realization.SeriesTransitions {
		functionsByInstance[transition.Input.Instance] = append(functionsByInstance[transition.Input.Instance], transition.Input.Function)
		functionsByInstance[transition.Output.Instance] = append(functionsByInstance[transition.Output.Instance], transition.Output.Function)
	}
	for _, connection := range realization.Connections {
		for _, endpoint := range connection.Endpoints {
			functionsByInstance[endpoint.Instance] = append(functionsByInstance[endpoint.Instance], endpoint.Function)
		}
	}
	for index := range realization.Instances {
		instance := &realization.Instances[index]
		instance.RequiredFunctions = normalizeFunctionSet(append(instance.RequiredFunctions, functionsByInstance[instance.ID]...))
	}
	for index := range realization.Parameters {
		parameter := &realization.Parameters[index]
		parameter.Name = canonicalIdentifier(parameter.Name)
		parameter.Unit = canonicalUnit(parameter.Unit)
		parameter.Value = quantize(parameter.Value)
	}
	for index := range realization.RepairVariables {
		variable := &realization.RepairVariables[index]
		variable.ID = canonicalIdentifier(variable.ID)
		variable.Kind = canonicalIdentifier(variable.Kind)
		variable.Instance = canonicalIdentifier(variable.Instance)
		variable.Unit = canonicalUnit(variable.Unit)
		variable.Value = quantize(variable.Value)
		for valueIndex := range variable.AllowedValues {
			variable.AllowedValues[valueIndex] = quantize(variable.AllowedValues[valueIndex])
		}
		slices.Sort(variable.AllowedValues)
		variable.AllowedValues = slices.Compact(variable.AllowedValues)
		for effectIndex := range variable.Effects {
			effect := &variable.Effects[effectIndex]
			effect.Analysis = canonicalIdentifier(effect.Analysis)
			effect.Metric = canonicalIdentifier(effect.Metric)
			effect.Direction = canonicalIdentifier(effect.Direction)
		}
		slices.SortStableFunc(variable.Effects, func(left, right RealizationRepairEffect) int {
			return strings.Compare(left.Analysis+"\x00"+left.Metric+"\x00"+left.Direction, right.Analysis+"\x00"+right.Metric+"\x00"+right.Direction)
		})
	}
	slices.SortStableFunc(realization.Instances, func(left, right RealizationInstance) int {
		return strings.Compare(left.ID, right.ID)
	})
	slices.SortStableFunc(realization.PortBindings, func(left, right RealizationPortBinding) int {
		if order := strings.Compare(left.Role, right.Role); order != 0 {
			return order
		}
		if order := strings.Compare(left.Lane, right.Lane); order != 0 {
			return order
		}
		if order := strings.Compare(left.Instance, right.Instance); order != 0 {
			return order
		}
		return strings.Compare(left.Function, right.Function)
	})
	slices.SortStableFunc(realization.SeriesTransitions, func(left, right RealizationSeriesTransition) int {
		if order := strings.Compare(left.Role, right.Role); order != 0 {
			return order
		}
		return strings.Compare(left.Lane, right.Lane)
	})
	slices.SortStableFunc(realization.Connections, func(left, right RealizationConnection) int {
		return strings.Compare(left.ID, right.ID)
	})
	slices.SortStableFunc(realization.Parameters, func(left, right RealizationParameter) int {
		return strings.Compare(left.Name, right.Name)
	})
	slices.SortStableFunc(realization.RepairVariables, func(left, right RealizationRepairVariable) int {
		return strings.Compare(left.ID, right.ID)
	})
	if err := validateFragmentRealization(realization); err != nil {
		return nil, err
	}
	return json.Marshal(realization)
}

func DecodeFragmentRealization(payload json.RawMessage) (FragmentRealization, error) {
	var realization FragmentRealization
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&realization); err != nil {
		return FragmentRealization{}, fmt.Errorf("decode fragment realization: %w", err)
	}
	if err := validateFragmentRealization(realization); err != nil {
		return FragmentRealization{}, err
	}
	return realization, nil
}

func validateFragmentRealization(realization FragmentRealization) error {
	if realization.Schema != FragmentRealizationSchema || !validSemanticID(realization.Capability) || len(realization.Instances) == 0 || len(realization.PortBindings) == 0 {
		return fmt.Errorf("fragment realization requires schema, capability, instances, and port bindings")
	}
	instances := map[string]bool{}
	for _, instance := range realization.Instances {
		if !validSemanticID(instance.ID) || instance.CatalogID == "" || !validSemanticID(instance.Usage) || instances[instance.ID] {
			return fmt.Errorf("fragment realization instance is invalid or duplicated")
		}
		instances[instance.ID] = true
	}
	roles := map[string]bool{}
	boundEndpoints := map[string]bool{}
	for _, binding := range realization.PortBindings {
		key := binding.Role + "\x00" + binding.Lane
		endpointKey := binding.Instance + "\x00" + binding.Function
		if !validSemanticID(binding.Role) || (binding.Lane != "" && !validSemanticID(binding.Lane)) || !instances[binding.Instance] || binding.Function == "" || roles[key] || boundEndpoints[endpointKey] {
			return fmt.Errorf("fragment realization port binding is invalid or duplicated")
		}
		roles[key] = true
		boundEndpoints[endpointKey] = true
	}
	for _, transition := range realization.SeriesTransitions {
		key := transition.Role + "\x00" + transition.Lane
		inputKey := transition.Input.Instance + "\x00" + transition.Input.Function
		outputKey := transition.Output.Instance + "\x00" + transition.Output.Function
		if !validSemanticID(transition.Role) || (transition.Lane != "" && !validSemanticID(transition.Lane)) ||
			!instances[transition.Input.Instance] || !instances[transition.Output.Instance] ||
			transition.Input.Function == "" || transition.Output.Function == "" || inputKey == outputKey || roles[key] ||
			boundEndpoints[inputKey] || boundEndpoints[outputKey] {
			return fmt.Errorf("fragment realization series transition is invalid or duplicated")
		}
		roles[key] = true
		boundEndpoints[inputKey] = true
		boundEndpoints[outputKey] = true
	}
	connectionIDs := map[string]bool{}
	connectedEndpoints := map[string]bool{}
	for _, connection := range realization.Connections {
		if !validSemanticID(connection.ID) || !validSemanticID(connection.Role) || connectionIDs[connection.ID] || len(connection.Endpoints) < 2 {
			return fmt.Errorf("fragment realization connection is invalid or duplicated")
		}
		connectionIDs[connection.ID] = true
		for _, endpoint := range connection.Endpoints {
			key := endpoint.Instance + "\x00" + endpoint.Function
			if !instances[endpoint.Instance] || endpoint.Function == "" || connectedEndpoints[key] {
				return fmt.Errorf("fragment realization connection endpoint is invalid or duplicated")
			}
			connectedEndpoints[key] = true
		}
	}
	for _, instance := range realization.Instances {
		required := map[string]bool{}
		for _, function := range instance.RequiredFunctions {
			required[function] = true
		}
		for key := range boundEndpoints {
			parts := strings.SplitN(key, "\x00", 2)
			if parts[0] == instance.ID && !required[parts[1]] {
				return fmt.Errorf("fragment realization binding function is not required by its instance")
			}
		}
		for key := range connectedEndpoints {
			parts := strings.SplitN(key, "\x00", 2)
			if parts[0] == instance.ID && !required[parts[1]] {
				return fmt.Errorf("fragment realization connection function is not required by its instance")
			}
		}
	}
	for _, parameter := range realization.Parameters {
		if !validSemanticID(parameter.Name) || !finiteNumbers(parameter.Value) || parameter.Unit == "" {
			return fmt.Errorf("fragment realization parameter is invalid")
		}
	}
	repairIDs := map[string]bool{}
	for _, variable := range realization.RepairVariables {
		if !validSemanticID(variable.ID) || repairIDs[variable.ID] || !validSemanticID(variable.Kind) || !instances[variable.Instance] || !finiteNumbers(variable.Value) || variable.Unit == "" || len(variable.AllowedValues) < 2 || len(variable.Effects) == 0 {
			return fmt.Errorf("fragment realization repair variable is invalid or duplicated")
		}
		repairIDs[variable.ID] = true
		found, previous := false, -1.0e300
		for _, value := range variable.AllowedValues {
			if !finiteNumbers(value) || value <= previous {
				return fmt.Errorf("fragment realization repair values are not finite and strictly ascending")
			}
			previous = value
			found = found || value == variable.Value
		}
		if !found {
			return fmt.Errorf("fragment realization repair values omit the current value")
		}
		previousEffect := ""
		for _, effect := range variable.Effects {
			key := effect.Analysis + "\x00" + effect.Metric + "\x00" + effect.Direction
			if !validSemanticID(effect.Analysis) || !validSemanticID(effect.Metric) || !validSemanticID(effect.Direction) || key <= previousEffect {
				return fmt.Errorf("fragment realization repair effect is invalid, duplicated, or unordered")
			}
			previousEffect = key
		}
	}
	return nil
}

func compareRealizationEndpoints(left, right RealizationEndpoint) int {
	if order := strings.Compare(left.Instance, right.Instance); order != 0 {
		return order
	}
	return strings.Compare(left.Function, right.Function)
}

func normalizeFunctionSet(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	slices.Sort(normalized)
	return normalized
}
