package architecturesearch

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const FragmentRealizationSchema = "kicadai.fragment-realization.v1"

// FragmentRealization is the identity-neutral output contract between an
// architecture provider and function-level lowering. It contains semantic
// component functions only; KiCad pins, footprints, coordinates, and routes
// remain the responsibility of the existing resolver and writer pipeline.
type FragmentRealization struct {
	Schema       string                   `json:"schema"`
	Capability   string                   `json:"capability"`
	Instances    []RealizationInstance    `json:"instances"`
	PortBindings []RealizationPortBinding `json:"port_bindings"`
	Parameters   []RealizationParameter   `json:"parameters,omitempty"`
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
	Instance string `json:"instance"`
	Function string `json:"function"`
}

type RealizationParameter struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
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
		instance.RequiredFunctions = normalizeStringSet(instance.RequiredFunctions)
	}
	for index := range realization.PortBindings {
		binding := &realization.PortBindings[index]
		binding.Role = canonicalIdentifier(binding.Role)
		binding.Instance = canonicalIdentifier(binding.Instance)
		binding.Function = strings.ToUpper(strings.TrimSpace(binding.Function))
	}
	for index := range realization.Parameters {
		parameter := &realization.Parameters[index]
		parameter.Name = canonicalIdentifier(parameter.Name)
		parameter.Unit = canonicalUnit(parameter.Unit)
		parameter.Value = quantize(parameter.Value)
	}
	slices.SortStableFunc(realization.Instances, func(left, right RealizationInstance) int {
		return strings.Compare(left.ID, right.ID)
	})
	slices.SortStableFunc(realization.PortBindings, func(left, right RealizationPortBinding) int {
		if order := strings.Compare(left.Role, right.Role); order != 0 {
			return order
		}
		if order := strings.Compare(left.Instance, right.Instance); order != 0 {
			return order
		}
		return strings.Compare(left.Function, right.Function)
	})
	slices.SortStableFunc(realization.Parameters, func(left, right RealizationParameter) int {
		return strings.Compare(left.Name, right.Name)
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
	for _, binding := range realization.PortBindings {
		if !validSemanticID(binding.Role) || !instances[binding.Instance] || binding.Function == "" || roles[binding.Role] {
			return fmt.Errorf("fragment realization port binding is invalid or duplicated")
		}
		roles[binding.Role] = true
	}
	for _, parameter := range realization.Parameters {
		if !validSemanticID(parameter.Name) || !finiteNumbers(parameter.Value) || parameter.Unit == "" {
			return fmt.Errorf("fragment realization parameter is invalid")
		}
	}
	return nil
}
