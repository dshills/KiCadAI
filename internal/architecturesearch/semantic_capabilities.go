package architecturesearch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	SemanticCapabilitySchema  = "kicadai.behavioral-capabilities.v1"
	SemanticCapabilityVersion = 1
)

type BehavioralMetricCapability struct {
	Metric   string `json:"metric"`
	Analysis string `json:"analysis"`
	Unit     string `json:"unit"`
}

type OperatingAxisCapability struct {
	Axis      string `json:"axis"`
	Unit      string `json:"unit,omitempty"`
	Selection bool   `json:"selection"`
}

type SemanticCapabilityDocument struct {
	Schema              string                       `json:"schema"`
	Version             int                          `json:"version"`
	RequirementSchema   string                       `json:"requirement_schema"`
	RegistryHash        string                       `json:"registry_hash"`
	DocumentHash        string                       `json:"document_hash"`
	ObjectiveKinds      []string                     `json:"objective_kinds"`
	BehavioralMetrics   []BehavioralMetricCapability `json:"behavioral_metrics"`
	OperatingAxes       []OperatingAxisCapability    `json:"operating_axes"`
	PortKinds           []string                     `json:"port_kinds"`
	Directions          []string                     `json:"directions"`
	ConstraintRelations []string                     `json:"constraint_relations"`
	CanonicalUnits      []string                     `json:"canonical_units"`
	ProtocolModes       []string                     `json:"protocol_modes"`
}

var registeredBehavioralMetrics = []BehavioralMetricCapability{
	{Metric: "bandwidth", Analysis: "ac_sweep", Unit: "Hz"},
	{Metric: "cutoff_frequency", Analysis: "ac_sweep", Unit: "Hz"},
	{Metric: "dc_current", Analysis: "dc_operating_point", Unit: "A"},
	{Metric: "dc_voltage", Analysis: "dc_operating_point", Unit: "V"},
	{Metric: "fall_time", Analysis: "transient", Unit: "s"},
	{Metric: "hysteresis_voltage", Analysis: "dc_operating_point", Unit: "V"},
	{Metric: "integrated_output_noise", Analysis: "noise", Unit: "V_rms"},
	{Metric: "junction_temperature", Analysis: "thermal", Unit: "degC"},
	{Metric: "muted_output_voltage", Analysis: "transient", Unit: "V"},
	{Metric: "output_high_voltage", Analysis: "dc_operating_point", Unit: "V"},
	{Metric: "output_power", Analysis: "transient", Unit: "W"},
	{Metric: "output_swing", Analysis: "transient", Unit: "V_pp"},
	{Metric: "phase_margin", Analysis: "stability", Unit: "deg"},
	{Metric: "quiescent_current", Analysis: "dc_operating_point", Unit: "A"},
	{Metric: "response_time", Analysis: "transient", Unit: "s"},
	{Metric: "rise_time", Analysis: "transient", Unit: "s"},
	{Metric: "settling_time", Analysis: "transient", Unit: "s"},
	{Metric: "startup_output_voltage", Analysis: "startup", Unit: "V"},
	{Metric: "threshold_current", Analysis: "dc_operating_point", Unit: "A"},
	{Metric: "threshold_voltage", Analysis: "dc_operating_point", Unit: "V"},
	{Metric: "total_harmonic_distortion", Analysis: "distortion", Unit: "%"},
	{Metric: "transimpedance", Analysis: "dc_operating_point", Unit: "V/A"},
	{Metric: "voltage_gain", Analysis: "ac_sweep", Unit: "ratio"},
}

var registeredOperatingAxes = []OperatingAxisCapability{
	{Axis: "ambient_temperature", Unit: "degC"},
	{Axis: "input_amplitude", Unit: "V"},
	{Axis: "input_frequency", Unit: "Hz"},
	{Axis: "load_capacitance", Unit: "F"},
	{Axis: "load_current", Unit: "A"},
	{Axis: "load_resistance", Unit: "Ohm"},
	{Axis: "model_parameter", Selection: true},
	{Axis: "supply_voltage", Unit: "V"},
	{Axis: "tolerance", Selection: true},
}

var registeredPortKinds = []string{"analog_control", "analog_voltage", "differential_analog", "digital_bus", "digital_logic", "power", "protected_output", "reference", "switched_load"}
var registeredDirections = []string{"bidirectional", "sink", "source"}
var registeredConstraintRelations = []string{"equal", "maximum", "minimum", "one_of", "range", "required", "target"}
var registeredCanonicalUnits = []string{"%", "A", "F", "Hz", "Ohm", "V", "V/A", "V_pp", "V_rms", "W", "dB", "deg", "degC", "pF", "ratio", "s", "uV_rms", "us"}
var registeredProtocolModes = []string{"differential", "open_drain", "push_pull", "single_ended"}

// ValidateSemanticCapabilities verifies that an encoded capability document is
// self-consistent and uses this binary's exact registered semantic vocabulary.
func ValidateSemanticCapabilities(document SemanticCapabilityDocument) error {
	if document.Schema != SemanticCapabilitySchema || document.Version != SemanticCapabilityVersion || document.RequirementSchema != SchemaIDV3 {
		return fmt.Errorf("semantic capability schema, version, or target requirement is unsupported")
	}
	if !sha256Hex(document.RegistryHash) || !sha256Hex(document.DocumentHash) || len(document.ObjectiveKinds) == 0 {
		return fmt.Errorf("semantic capability hashes and objective kinds are required")
	}
	if !sortedUniqueSemanticIDs(document.ObjectiveKinds) || !slices.Equal(document.BehavioralMetrics, registeredBehavioralMetrics) || !slices.Equal(document.OperatingAxes, registeredOperatingAxes) || !slices.Equal(document.PortKinds, registeredPortKinds) || !slices.Equal(document.Directions, registeredDirections) || !slices.Equal(document.ConstraintRelations, registeredConstraintRelations) || !slices.Equal(document.CanonicalUnits, registeredCanonicalUnits) || !slices.Equal(document.ProtocolModes, registeredProtocolModes) {
		return fmt.Errorf("semantic capability vocabulary is malformed or stale for this binary")
	}
	hashInput := document
	hashInput.DocumentHash = ""
	encoded, err := json.Marshal(hashInput)
	if err != nil {
		return fmt.Errorf("encode semantic capability document: %w", err)
	}
	hash := sha256.Sum256(encoded)
	if document.DocumentHash != hex.EncodeToString(hash[:]) {
		return fmt.Errorf("semantic capability document hash does not match its content")
	}
	return nil
}

func (registry *Registry) SemanticCapabilities() (SemanticCapabilityDocument, error) {
	if registry == nil || registry.hash == "" || len(registry.providers) == 0 {
		return SemanticCapabilityDocument{}, fmt.Errorf("architecture capability registry is unavailable")
	}
	capabilitySet := map[string]bool{}
	for _, provider := range registry.providers {
		for _, capability := range provider.descriptor.Capabilities {
			capabilitySet[capability] = true
		}
	}
	objectiveKinds := make([]string, 0, len(capabilitySet))
	for capability := range capabilitySet {
		objectiveKinds = append(objectiveKinds, capability)
	}
	slices.Sort(objectiveKinds)
	document := SemanticCapabilityDocument{
		Schema: SemanticCapabilitySchema, Version: SemanticCapabilityVersion,
		RequirementSchema: SchemaIDV3, RegistryHash: registry.hash,
		ObjectiveKinds: objectiveKinds, BehavioralMetrics: slices.Clone(registeredBehavioralMetrics),
		OperatingAxes: slices.Clone(registeredOperatingAxes), PortKinds: slices.Clone(registeredPortKinds),
		Directions: slices.Clone(registeredDirections), ConstraintRelations: slices.Clone(registeredConstraintRelations),
		CanonicalUnits: slices.Clone(registeredCanonicalUnits), ProtocolModes: slices.Clone(registeredProtocolModes),
	}
	hashInput := document
	hashInput.DocumentHash = ""
	encoded, err := json.Marshal(hashInput)
	if err != nil {
		return SemanticCapabilityDocument{}, fmt.Errorf("encode semantic capability document: %w", err)
	}
	hash := sha256.Sum256(encoded)
	document.DocumentHash = hex.EncodeToString(hash[:])
	if err := ValidateSemanticCapabilities(document); err != nil {
		return SemanticCapabilityDocument{}, err
	}
	return document, nil
}

func EncodeSemanticCapabilities(registry *Registry, maxBytes int) (json.RawMessage, error) {
	document, err := registry.SemanticCapabilities()
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode semantic capability document: %w", err)
	}
	if maxBytes > 0 && len(encoded) > maxBytes {
		return nil, fmt.Errorf("semantic capability document is %d bytes, exceeds %d-byte provider limit", len(encoded), maxBytes)
	}
	return encoded, nil
}

func operatingAxisContract(axis string) (unit string, selection bool) {
	for _, capability := range registeredOperatingAxes {
		if capability.Axis == axis {
			return capability.Unit, capability.Selection
		}
	}
	return "", false
}

func behavioralMetricContract(metric string) (analysis, unit string, ok bool) {
	index, found := slices.BinarySearchFunc(registeredBehavioralMetrics, metric, func(capability BehavioralMetricCapability, target string) int {
		return strings.Compare(capability.Metric, target)
	})
	if !found {
		return "", "", false
	}
	capability := registeredBehavioralMetrics[index]
	return capability.Analysis, capability.Unit, true
}

func sha256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func sortedUniqueSemanticIDs(values []string) bool {
	if len(values) == 0 || !slices.IsSorted(values) {
		return false
	}
	for index, value := range values {
		if canonicalIdentifier(value) != value || (index > 0 && values[index-1] == value) {
			return false
		}
	}
	return true
}
