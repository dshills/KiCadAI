package simmodel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode"
)

const (
	maxMNAAnalyses        = 256
	maxMNASweepPoints     = 64
	maxMNADCSweepPoints   = 256
	maxTotalDCSweepWork   = 16_384
	maxMNAExcitations     = 16
	maxMNADeviceOverrides = 64
	maxMNASourceMagnitude = 1e6
	maxTransientSteps     = 2048
	maxTransientWork      = (maxTransientSteps + 6) * transientMaxNewtonIterations
	maxTotalDynamicWork   = 2_000_000
	minTransientTimeStepS = 1e-9
	maxTransientDurationS = 10
)

type primitiveDefinition struct {
	ID                string              `json:"id"`
	Family            string              `json:"family"`
	Terminals         []string            `json:"terminals"`
	TerminalAliases   map[string][]string `json:"terminal_aliases,omitempty"`
	RequiresValueSI   bool                `json:"requires_value_si,omitempty"`
	CatalogParameters []valueRule         `json:"catalog_parameters,omitempty"`
	Source            bool                `json:"source,omitempty"`
	OpAmp             bool                `json:"op_amp,omitempty"`
	Comparator        bool                `json:"comparator,omitempty"`
	Nonlinear         bool                `json:"nonlinear,omitempty"`
	Transient         bool                `json:"transient,omitempty"`
}

var primitiveRegistry = []primitiveDefinition{
	{ID: PrimitiveResistorV1, Family: "resistor", Terminals: []string{"A", "B"}, RequiresValueSI: true, CatalogParameters: thermalParameterRules()},
	{
		ID: PrimitiveFuseClosedStateV1, Family: "fuse", Terminals: []string{"A", "B"},
		CatalogParameters: []valueRule{
			{Name: "cold_resistance_ohm", Positive: true, Minimum: 1e-6, Maximum: 1e6},
			{Name: "rated_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
			{Name: "max_voltage_v", Positive: true, Minimum: .01, Maximum: 1e6},
			{Name: "nominal_melting_i2t_a2s", Optional: true, Positive: true, Minimum: 1e-12, Maximum: 1e12},
		},
	},
	{
		ID: PrimitiveRelayClosedV1, Family: "relay", Terminals: []string{"COIL_A", "COIL_B", "CONTACT_IN", "CONTACT_OUT"},
		CatalogParameters: []valueRule{
			{Name: "coil_resistance_ohm", Positive: true, Minimum: 1e-3, Maximum: 1e9},
			{Name: "contact_on_resistance_ohm", Positive: true, Minimum: 1e-6, Maximum: 1e6},
			{Name: "max_contact_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e5},
			{Name: "max_contact_voltage_v", Positive: true, Minimum: 1e-3, Maximum: 1e6},
		},
	},
	{
		ID: PrimitiveRelayNormallyOpenV1, Family: "relay", Terminals: []string{"COIL_A", "COIL_B", "CONTACT_IN", "CONTACT_OUT"}, Transient: true,
		CatalogParameters: []valueRule{
			{Name: "coil_resistance_ohm", Positive: true, Minimum: 1e-3, Maximum: 1e9},
			{Name: "operate_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
			{Name: "contact_on_resistance_ohm", Positive: true, Minimum: 1e-6, Maximum: 1e6},
			{Name: "contact_off_resistance_ohm", Positive: true, Minimum: 1, Maximum: 1e15},
			{Name: "operate_delay_s", Positive: true, Minimum: 1e-9, Maximum: 10},
			{Name: "max_contact_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e5},
			{Name: "max_contact_voltage_v", Positive: true, Minimum: 1e-3, Maximum: 1e6},
		},
	},
	{ID: PrimitiveCapacitorV1, Family: "capacitor", Terminals: []string{"A", "B"}, RequiresValueSI: true},
	{ID: PrimitiveCapacitorTransientV1, Family: "capacitor", Terminals: []string{"A", "B"}, RequiresValueSI: true, Transient: true,
		CatalogParameters: []valueRule{{Name: "max_voltage_v", Positive: true, Minimum: .01, Maximum: 1e6}}},
	{ID: PrimitiveVoltageSourceV1, Family: "voltage_source", Terminals: []string{"POSITIVE", "NEGATIVE"}, Source: true},
	{ID: PrimitiveConnectorVoltageSourceV1, Family: "connector", Terminals: []string{"PIN_1", "PIN_2"}, Source: true},
	{ID: PrimitiveCurrentSourceV1, Family: "current_source", Terminals: []string{"POSITIVE", "NEGATIVE"}, Source: true},
	{
		ID: PrimitiveMCUStaticSupplyLoadV1, Family: "mcu", Terminals: []string{"POWER", "GROUND"},
		TerminalAliases: map[string][]string{
			"POWER":  {"VDD", "VCC", "AVCC", "VDDA"},
			"GROUND": {"GND", "VSS", "AGND", "VSSA"},
		},
		CatalogParameters: append([]valueRule{{Name: "maximum_supply_current_a", Positive: true, Maximum: 100}}, thermalParameterRules()...),
	},
	{
		ID: PrimitiveSensorStaticSupplyLoadV1, Family: "sensor", Terminals: []string{"POWER", "GROUND"},
		TerminalAliases: map[string][]string{
			"POWER":  {"VDD", "VDDIO", "VCC", "VDDA"},
			"GROUND": {"GND", "VSS", "AGND", "VSSA"},
		},
		CatalogParameters: append([]valueRule{{Name: "maximum_supply_current_a", Positive: true, Maximum: 100}}, thermalParameterRules()...),
	},
	{
		ID: PrimitiveOpAmpV1, Family: "opamp", Terminals: []string{"IN_PLUS", "IN_MINUS", "OUT", "V_PLUS", "V_MINUS"}, OpAmp: true,
		CatalogParameters: []valueRule{
			{Name: "dc_open_loop_gain", Positive: true, Maximum: 1e9},
			{Name: "gain_bandwidth_hz", Positive: true, Maximum: 1e12},
			{Name: "supply_min_v", Positive: true, Maximum: 1000},
			{Name: "supply_max_v", Positive: true, Maximum: 1000},
			{Name: "output_low_margin_v", Nonnegative: true, Maximum: 100},
			{Name: "output_high_margin_v", Nonnegative: true, Maximum: 100},
			{Name: "input_voltage_noise_density_v_sqrt_hz", Optional: true, Positive: true, Maximum: 1},
			{Name: "quiescent_current_a", Optional: true, Nonnegative: true, Maximum: 100},
			{Name: "max_temperature_c", Optional: true, Maximum: 1000},
			{Name: "thermal_resistance_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
			{Name: "junction_to_ambient_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
			{Name: "junction_to_case_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
		},
	},
	{
		ID: PrimitiveCurrentSenseAmplifierV1, Family: "current_sensor",
		Terminals: []string{"IN_PLUS", "IN_MINUS", "REF1", "REF2", "OUT", "VCC", "GND_A", "GND_B"},
		CatalogParameters: []valueRule{
			{Name: "gain_v_per_v", Positive: true, Maximum: 1e6},
			{Name: "bandwidth_hz", Positive: true, Maximum: 1e12},
			{Name: "input_offset_voltage_v", Minimum: -1, Maximum: 1},
			{Name: "supply_min_v", Positive: true, Maximum: 1000},
			{Name: "supply_max_v", Positive: true, Maximum: 1000},
			{Name: "common_mode_min_v", Minimum: -1e6, Maximum: 1e6},
			{Name: "common_mode_max_v", Minimum: -1e6, Maximum: 1e6},
			{Name: "output_low_margin_v", Nonnegative: true, Maximum: 100},
			{Name: "output_high_margin_v", Nonnegative: true, Maximum: 100},
			{Name: "quiescent_current_a", Nonnegative: true, Maximum: 100},
		},
	},
	{
		ID: PrimitiveComparatorOpenCollectorV1, Family: "comparator", Terminals: []string{"IN_PLUS", "IN_MINUS", "OUT", "V_PLUS", "V_MINUS"}, Comparator: true, Nonlinear: true,
		CatalogParameters: []valueRule{
			{Name: "input_offset_v", Minimum: -1, Maximum: 1},
			{Name: "output_on_resistance_ohm", Positive: true, Minimum: 1e-6, Maximum: 1e12},
			{Name: "output_off_resistance_ohm", Positive: true, Minimum: 1, Maximum: 1e15},
			{Name: "max_sink_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
			{Name: "propagation_delay_s", Nonnegative: true, Maximum: 10},
			{Name: "supply_min_v", Positive: true, Maximum: 1000},
			{Name: "supply_max_v", Positive: true, Maximum: 1000},
			{Name: "quiescent_current_a", Nonnegative: true, Maximum: 100},
			{Name: "max_temperature_c", Optional: true, Maximum: 1000},
			{Name: "thermal_resistance_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
			{Name: "junction_to_ambient_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
			{Name: "junction_to_case_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
		},
	},
	{
		ID: PrimitiveAdjustableLinearRegulatorV1, Family: "regulator", Terminals: []string{"VIN", "VOUT", "GND", "ADJ"},
		CatalogParameters: []valueRule{
			{Name: "reference_voltage_v", Positive: true, Minimum: .01, Maximum: 100},
			{Name: "min_headroom_v", Nonnegative: true, Maximum: 20},
			{Name: "max_load_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
			{Name: "min_input_voltage_v", Positive: true, Maximum: 1000},
			{Name: "max_input_voltage_v", Positive: true, Maximum: 1000},
			{Name: "quiescent_current_a", Nonnegative: true, Maximum: 100},
			{Name: "soft_start_time_s", Nonnegative: true, Maximum: 10},
			{Name: "max_temperature_c", Maximum: 1000},
			{Name: "junction_to_ambient_c_per_w", Positive: true, Maximum: 1e6},
		},
	},
	{
		ID: PrimitiveFixedLinearRegulatorV1, Family: "regulator", Terminals: []string{"VIN", "VOUT", "GND"},
		CatalogParameters: []valueRule{
			{Name: "output_voltage_v", Positive: true, Minimum: .01, Maximum: 100},
			{Name: "min_headroom_v", Nonnegative: true, Maximum: 20},
			{Name: "max_load_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
			{Name: "min_input_voltage_v", Positive: true, Maximum: 1000},
			{Name: "max_input_voltage_v", Positive: true, Maximum: 1000},
			{Name: "quiescent_current_a", Nonnegative: true, Maximum: 100},
			{Name: "soft_start_time_s", Nonnegative: true, Maximum: 10},
			{Name: "max_temperature_c", Maximum: 1000},
			{Name: "junction_to_ambient_c_per_w", Positive: true, Maximum: 1e6},
		},
	},
	{
		ID: PrimitiveFloatingAdjustableRegulatorV1, Family: "regulator", Terminals: []string{"VIN", "VOUT", "ADJ"},
		CatalogParameters: []valueRule{
			{Name: "reference_voltage_v", Positive: true, Minimum: .01, Maximum: 100},
			{Name: "polarity", Minimum: -1, Maximum: 1},
			{Name: "min_headroom_v", Nonnegative: true, Maximum: 20},
			{Name: "max_load_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
			{Name: "max_input_output_voltage_v", Positive: true, Maximum: 1000},
			{Name: "adjustment_pin_current_a", Nonnegative: true, Maximum: 1},
			{Name: "soft_start_time_s", Nonnegative: true, Maximum: 10},
			{Name: "max_temperature_c", Optional: true, Maximum: 1000},
			{Name: "junction_to_ambient_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
		},
	},
	{
		ID: PrimitiveProgrammableCurrentSourceV1, Family: "current_regulator", Terminals: []string{"IN", "SET", "OUT"},
		CatalogParameters: []valueRule{
			{Name: "reference_current_a", Positive: true, Minimum: 1e-12, Maximum: 1},
			{Name: "offset_voltage_v", Maximum: 100},
			{Name: "min_headroom_v", Positive: true, Minimum: 1e-3, Maximum: 100},
			{Name: "max_output_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
			{Name: "max_input_output_voltage_v", Positive: true, Minimum: 1e-3, Maximum: 1000},
			{Name: "soft_start_time_s", Nonnegative: true, Maximum: 10},
			{Name: "max_temperature_c", Optional: true, Maximum: 1000},
			{Name: "junction_to_ambient_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
			{Name: "junction_to_case_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
		},
	},
	{
		ID: PrimitiveShuntVoltageReferenceV1, Family: "voltage_reference", Terminals: []string{"CATHODE", "ANODE"},
		CatalogParameters: []valueRule{
			{Name: "output_voltage_v", Positive: true, Minimum: 1e-3, Maximum: 1000},
			{Name: "min_bias_current_a", Positive: true, Minimum: 1e-12, Maximum: 100},
			{Name: "max_bias_current_a", Positive: true, Minimum: 1e-12, Maximum: 100},
			{Name: "max_reverse_voltage_v", Positive: true, Minimum: 1e-3, Maximum: 1000},
			{Name: "max_temperature_c", Optional: true, Maximum: 1000},
			{Name: "junction_to_ambient_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
		},
	},
	{
		ID: PrimitiveDualOutputIsolatedConverterV1, Family: "isolated_converter", Terminals: []string{"VIN_PLUS", "VIN_MINUS", "COMMON", "VOUT_PLUS", "VOUT_MINUS"},
		CatalogParameters: []valueRule{
			{Name: "input_min_v", Positive: true, Maximum: 1000},
			{Name: "input_max_v", Positive: true, Maximum: 1000},
			{Name: "positive_output_voltage_v", Positive: true, Minimum: .01, Maximum: 1000},
			{Name: "negative_output_voltage_v", Positive: true, Minimum: .01, Maximum: 1000},
			{Name: "positive_max_output_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
			{Name: "negative_max_output_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
			{Name: "soft_start_time_s", Nonnegative: true, Maximum: 10},
		},
	},
	{
		ID: PrimitiveBidirectionalOpenDrainTranslatorV1, Family: "level_translator", Terminals: []string{"A1", "A2", "B1", "B2", "VCCA", "VCCB", "GND", "OE"}, Nonlinear: true,
		CatalogParameters: []valueRule{
			{Name: "vcca_min_v", Positive: true, Maximum: 1000},
			{Name: "vcca_max_v", Positive: true, Maximum: 1000},
			{Name: "vccb_min_v", Positive: true, Maximum: 1000},
			{Name: "vccb_max_v", Positive: true, Maximum: 1000},
			{Name: "low_level_threshold_v", Positive: true, Maximum: 100},
			{Name: "enable_high_ratio", Positive: true, Maximum: 1},
			{Name: "channel_on_resistance_ohm", Positive: true, Minimum: 1e-6, Maximum: 1e12},
			{Name: "channel_off_resistance_ohm", Positive: true, Minimum: 1, Maximum: 1e15},
			{Name: "max_channel_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
			{Name: "vcca_quiescent_current_a", Nonnegative: true, Maximum: 100},
			{Name: "vccb_quiescent_current_a", Nonnegative: true, Maximum: 100},
			{Name: "max_temperature_c", Maximum: 1000},
			{Name: "junction_to_ambient_c_per_w", Positive: true, Maximum: 1e6},
		},
	},
	{
		ID: PrimitiveBidirectionalTVSV1, Family: "protection", Terminals: []string{"ANODE", "CATHODE"}, Nonlinear: true,
		CatalogParameters: []valueRule{
			{Name: "breakdown_voltage_v", Positive: true, Minimum: .01, Maximum: 1e6},
			{Name: "dynamic_resistance_ohm", Positive: true, Minimum: 1e-6, Maximum: 1e12},
			{Name: "off_resistance_ohm", Positive: true, Minimum: 1, Maximum: 1e15},
			{Name: "junction_capacitance_f", Positive: true, Minimum: 1e-15, Maximum: 1},
			{Name: "max_pulse_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e6},
			{Name: "max_temperature_c", Optional: true, Maximum: 1000},
			{Name: "thermal_resistance_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
		},
	},
	{
		ID: PrimitiveDiodeShockleyV1, Family: "diode", Terminals: []string{"ANODE", "CATHODE"}, Nonlinear: true,
		CatalogParameters: []valueRule{
			{Name: "saturation_current_a", Positive: true, Minimum: 1e-30, Maximum: 1e-2},
			{Name: "emission_coefficient", Positive: true, Minimum: .5, Maximum: 10},
			{Name: "junction_temperature_k", Positive: true, Minimum: 200, Maximum: 1000},
			{Name: "max_forward_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
			{Name: "max_reverse_voltage_v", Positive: true, Minimum: .01, Maximum: 1e6},
			{Name: "max_temperature_c", Optional: true, Maximum: 1000},
			{Name: "thermal_resistance_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
		},
	},
	{
		ID: PrimitiveUnidirectionalZenerV1, Family: "diode", Terminals: []string{"ANODE", "CATHODE"}, Nonlinear: true,
		CatalogParameters: []valueRule{
			{Name: "forward_saturation_current_a", Positive: true, Minimum: 1e-30, Maximum: 1e-3},
			{Name: "forward_series_resistance_ohm", Nonnegative: true, Maximum: 1e12},
			{Name: "forward_emission_coefficient", Positive: true, Minimum: .5, Maximum: 10},
			{Name: "reverse_saturation_current_a", Positive: true, Minimum: 1e-30, Maximum: 1e-3},
			{Name: "reverse_series_resistance_ohm", Nonnegative: true, Maximum: 1e12},
			{Name: "reverse_emission_coefficient", Positive: true, Minimum: .5, Maximum: 10},
			{Name: "zener_offset_voltage_v", Positive: true, Minimum: .01, Maximum: 1e6},
			{Name: "junction_temperature_k", Positive: true, Minimum: 200, Maximum: 1000},
			{Name: "max_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
			{Name: "max_temperature_c", Optional: true, Maximum: 1000},
			{Name: "thermal_resistance_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
		},
	},
	{
		ID: PrimitiveNMOSSwitchV1, Family: "mosfet", Terminals: []string{"GATE", "DRAIN", "SOURCE"}, Nonlinear: true,
		CatalogParameters: []valueRule{
			{Name: "gate_on_voltage_v", Positive: true, Minimum: .01, Maximum: 1000},
			{Name: "on_resistance_ohm", Positive: true, Minimum: 1e-6, Maximum: 1e12},
			{Name: "max_drain_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
			{Name: "max_drain_source_voltage_v", Positive: true, Minimum: .01, Maximum: 1e6},
			{Name: "max_gate_source_voltage_v", Positive: true, Minimum: .01, Maximum: 1e6},
			{Name: "max_temperature_c", Optional: true, Maximum: 1000},
			{Name: "thermal_resistance_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
			{Name: "junction_to_ambient_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
			{Name: "junction_to_case_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
		},
	},
	{
		ID: PrimitivePMOSSwitchV1, Family: "mosfet", Terminals: []string{"GATE", "DRAIN", "SOURCE"}, Nonlinear: true,
		CatalogParameters: []valueRule{
			{Name: "gate_on_voltage_v", Positive: true, Minimum: .01, Maximum: 1000},
			{Name: "on_resistance_ohm", Positive: true, Minimum: 1e-6, Maximum: 1e12},
			{Name: "max_drain_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
			{Name: "max_drain_source_voltage_v", Positive: true, Minimum: .01, Maximum: 1e6},
			{Name: "max_gate_source_voltage_v", Positive: true, Minimum: .01, Maximum: 1e6},
			{Name: "max_temperature_c", Optional: true, Maximum: 1000},
			{Name: "thermal_resistance_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
			{Name: "junction_to_ambient_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
			{Name: "junction_to_case_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
		},
	},
	// NPN and PNP are distinct primitive equations under the catalog's
	// shared bjt family; polarity is selected by the trusted primitive ID.
	{ID: PrimitiveBJTNPNV1, Family: "bjt", Terminals: []string{"BASE", "COLLECTOR", "EMITTER"}, Nonlinear: true, CatalogParameters: bjtParameterRules()},
	{ID: PrimitiveBJTPNPV1, Family: "bjt", Terminals: []string{"BASE", "COLLECTOR", "EMITTER"}, Nonlinear: true, CatalogParameters: bjtParameterRules()},
}

// PrimitiveModelIDs returns the canonical trusted primitive identities in
// registry order.
func PrimitiveModelIDs() []string {
	ids := make([]string, 0, len(primitiveRegistry))
	for _, primitive := range primitiveRegistry {
		ids = append(ids, primitive.ID)
	}
	return ids
}

func bjtParameterRules() []valueRule {
	rules := []valueRule{
		{Name: "saturation_current_a", Positive: true, Minimum: 1e-30, Maximum: 1e-3},
		{Name: "forward_beta", Positive: true, Minimum: 1, Maximum: 1e6},
		{Name: "reverse_beta", Positive: true, Minimum: .01, Maximum: 1e6},
		{Name: "emission_coefficient", Positive: true, Minimum: .5, Maximum: 10},
		{Name: "junction_temperature_k", Positive: true, Minimum: 200, Maximum: 1000},
		{Name: "transition_frequency_hz", Optional: true, Positive: true, Minimum: 1, Maximum: 1e12},
		{Name: "max_collector_current_a", Positive: true, Minimum: 1e-9, Maximum: 1e4},
		{Name: "max_collector_emitter_voltage_v", Positive: true, Minimum: .01, Maximum: 1e6},
	}
	return append(rules, thermalParameterRules()...)
}

func thermalParameterRules() []valueRule {
	return []valueRule{
		{Name: "max_temperature_c", Optional: true, Maximum: 1000},
		{Name: "thermal_resistance_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
		{Name: "junction_to_ambient_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
		{Name: "junction_to_case_c_per_w", Optional: true, Positive: true, Maximum: 1e6},
	}
}

type NodeEvidence struct {
	Name          string
	Role          string
	VoltageDomain string
}

func primitiveByID(id string) (primitiveDefinition, bool) {
	for _, primitive := range primitiveRegistry {
		if primitive.ID == id {
			return primitive, true
		}
	}
	return primitiveDefinition{}, false
}

func primitiveFamilyCompatible(primitiveFamily, componentFamily string) bool {
	return primitiveFamily == componentFamily || (primitiveFamily == "diode" && componentFamily == "led")
}

func componentHasSourceClaim(component ComponentEvidence) bool {
	for _, claim := range component.ModelClaims {
		primitive, exists := primitiveByID(strings.TrimSpace(claim.ModelID))
		if exists && primitive.Source && primitiveFamilyCompatible(primitive.Family, component.Family) {
			return true
		}
	}
	return false
}

type primitiveClaim struct {
	primitive primitiveDefinition
	claim     CatalogEvidence
}

func compatiblePrimitiveClaims(component ComponentEvidence, model definition, analysisKind string) []primitiveClaim {
	var matches []primitiveClaim
	for _, claim := range component.ModelClaims {
		primitive, exists := primitiveByID(strings.TrimSpace(claim.ModelID))
		if !exists || !primitiveFamilyCompatible(primitive.Family, component.Family) || (primitive.Nonlinear && !model.NonlinearDC && !model.SmallSignalNonlinear) || (primitive.Transient && !model.Transient) || (model.Transient && primitive.Family == "capacitor" && !primitive.Transient) {
			continue
		}
		matches = append(matches, primitiveClaim{primitive: primitive, claim: claim})
	}
	if component.Family == "relay" && len(matches) > 1 {
		preferred := ""
		switch analysisKind {
		case AnalysisStartup:
			preferred = PrimitiveRelayNormallyOpenV1
		case AnalysisTransient, AnalysisDistortion:
			preferred = PrimitiveRelayNormallyOpenV1
		case AnalysisThermal:
			preferred = PrimitiveRelayClosedV1
		}
		if preferred != "" {
			matches = slices.DeleteFunc(matches, func(match primitiveClaim) bool { return match.primitive.ID != preferred })
		}
	}
	return matches
}

func uniqueIntentAnalysisKind(analyses []Analysis) string {
	kind := ""
	for _, analysis := range analyses {
		if kind == "" {
			kind = analysis.Kind
			continue
		}
		if analysis.Kind != kind {
			return ""
		}
	}
	return kind
}

// ApplicableGraphModel returns a graph workflow only when every connected
// non-boundary component has exactly one compatible trusted primitive. This
// keeps synthesis fail-closed: an incomplete catalog model never produces a
// partial or optimistic simulation.
func ApplicableGraphModel(components []ComponentEvidence) (string, bool, string) {
	return applicableGraphModel(components, "", "")
}

// ApplicableGraphModelForAnalysis selects a registered graph workflow for an
// explicitly requested trusted analysis kind. It does not accept provider
// model IDs and still requires every connected component to have exactly one
// compatible reviewed primitive.
func ApplicableGraphModelForAnalysis(components []ComponentEvidence, analysisKind string) (string, bool, string) {
	switch analysisKind {
	case AnalysisACSweep:
		return applicableGraphModel(components, ModelLinearCircuitMNAV1, analysisKind)
	case AnalysisTransient, AnalysisStartup, AnalysisDistortion:
		return applicableGraphModel(components, ModelTransientCircuitV1, analysisKind)
	case AnalysisNoise, AnalysisStability:
		return applicableGraphModel(components, ModelLinearCircuitMNAV1, analysisKind)
	case AnalysisThermal:
		return applicableGraphModel(components, "", analysisKind)
	default:
		return "", false, "unsupported_graph_analysis"
	}
}

func applicableGraphModel(components []ComponentEvidence, requestedModelID, analysisKind string) (string, bool, string) {
	hasSource := false
	hasDevice := false
	hasNonlinear := false
	for _, component := range components {
		if len(component.Connections) == 0 || (mnaBoundaryFamily(component.Family) && !componentHasSourceClaim(component)) {
			continue
		}
		for _, claim := range component.ModelClaims {
			primitive, exists := primitiveByID(strings.TrimSpace(claim.ModelID))
			if !exists || !primitiveFamilyCompatible(primitive.Family, component.Family) {
				continue
			}
			hasSource = hasSource || primitive.Source
			hasDevice = hasDevice || !primitive.Source
			hasNonlinear = hasNonlinear || primitive.Nonlinear
		}
	}
	if !hasSource || !hasDevice {
		return "", false, "missing_trusted_source_or_device"
	}
	modelID := requestedModelID
	if modelID == "" {
		modelID = ModelLinearCircuitMNAV1
		if hasNonlinear {
			modelID = ModelNonlinearCircuitDCV1
		}
	}
	model, exists := definitionByID(modelID)
	if !exists {
		return "", false, "registered_graph_model_missing"
	}
	for _, component := range components {
		if len(component.Connections) == 0 || (mnaBoundaryFamily(component.Family) && !componentHasSourceClaim(component)) {
			continue
		}
		// compatiblePrimitiveClaims admits trusted primitive IDs only; legacy
		// component/workflow claims never participate in this uniqueness rule.
		if len(compatiblePrimitiveClaims(component, model, analysisKind)) != 1 {
			return "", false, "component_" + component.InstanceID + "_has_no_unique_compatible_primitive"
		}
	}
	return modelID, true, "complete_registered_graph_model"
}

func validateMNAIntent(intent Intent, components map[string]string) []Diagnostic {
	var diagnostics []Diagnostic
	model, _ := definitionByID(strings.TrimSpace(intent.ModelID))
	if len(intent.Bindings) != 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "bindings", Message: "graph MNA derives devices from resolved connectivity and does not accept topology bindings"})
	}
	if len(intent.Inputs) != 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "inputs", Message: "graph MNA accepts operating conditions only inside trusted analyses"})
	}
	if len(intent.Analyses) == 0 || len(intent.Analyses) > maxMNAAnalyses {
		diagnostics = append(diagnostics, Diagnostic{Path: "analyses", Message: fmt.Sprintf("graph MNA requires 1..%d bounded analyses", maxMNAAnalyses)})
	}
	totalDynamicWork := 0
	totalDCSweepWork := 0
	analysisKinds := make(map[string]string, len(intent.Analyses))
	for index, analysis := range intent.Analyses {
		path := fmt.Sprintf("analyses[%d]", index)
		id := strings.TrimSpace(analysis.ID)
		if !validAnalysisID(id) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".id", Message: "analysis id must contain only lowercase letters, digits, and underscores and start with a letter"})
		} else if _, duplicate := analysisKinds[id]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".id", Message: "analysis id is duplicated"})
		}
		analysisKinds[id] = analysis.Kind
		switch analysis.Kind {
		case AnalysisDCOperatingPoint:
			if model.Transient {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".kind", Message: "transient circuit workflow supports transient analysis only"})
			}
			if analysis.StartFrequencyHz != 0 || analysis.StopFrequencyHz != 0 || analysis.Points != 0 || analysis.DurationS != 0 || analysis.TimeStepS != 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "DC operating-point analysis cannot contain AC sweep fields"})
			}
			if analysis.DCSweep != nil {
				sweep := analysis.DCSweep
				if strings.TrimSpace(sweep.Component) == "" || !finite(sweep.StartValue) || !finite(sweep.StopValue) || !boundedMagnitude(sweep.StartValue) || !boundedMagnitude(sweep.StopValue) || sweep.StartValue >= sweep.StopValue || sweep.Points < 3 || sweep.Points > maxMNADCSweepPoints {
					diagnostics = append(diagnostics, Diagnostic{Path: path + ".dc_sweep", Message: fmt.Sprintf("DC source sweep requires a resolved component, finite bounded start < stop, and 3..%d points", maxMNADCSweepPoints)})
				}
				if sweep.ExcitationScale != 0 && sweep.ExcitationScale != -1 && sweep.ExcitationScale != 1 {
					diagnostics = append(diagnostics, Diagnostic{Path: path + ".dc_sweep.excitation_scale", Message: "DC source sweep excitation scale must be exactly -1 or 1 when specified"})
				}
				family := components[strings.TrimSpace(sweep.Component)]
				if family != "voltage_source" && family != "current_source" && family != "connector" {
					diagnostics = append(diagnostics, Diagnostic{Path: path + ".dc_sweep.component", Message: "DC sweep component must be a resolved independent voltage or current source"})
				}
				work := sweep.Points
				if sweep.Bidirectional {
					work *= 2
				}
				totalDCSweepWork += work
			}
		case AnalysisACSweep, AnalysisNoise, AnalysisStability:
			if analysis.DCSweep != nil {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".dc_sweep", Message: "DC source sweep is accepted only by DC operating-point analysis"})
			}
			if model.NonlinearDC {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".kind", Message: "nonlinear circuit analysis supports DC operating points only", Suggestion: "use dc_operating_point or select the linear MNA workflow for small-signal analysis"})
			}
			if !finite(analysis.StartFrequencyHz) || !finite(analysis.StopFrequencyHz) || analysis.StartFrequencyHz <= 0 || analysis.StopFrequencyHz < analysis.StartFrequencyHz || analysis.StopFrequencyHz > 1e12 || analysis.Points < 2 || analysis.Points > maxMNASweepPoints {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: fmt.Sprintf("small-signal sweep requires finite 0 < start <= stop <= 1e12 Hz and 2..%d points", maxMNASweepPoints)})
			}
			if analysis.DurationS != 0 || analysis.TimeStepS != 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "small-signal sweep cannot contain transient grid fields"})
			}
		case AnalysisTransient, AnalysisStartup, AnalysisDistortion:
			if analysis.DCSweep != nil {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".dc_sweep", Message: "DC source sweep is accepted only by DC operating-point analysis"})
			}
			if !model.Transient {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".kind", Message: "dynamic analysis requires transient_circuit_v1"})
			}
			if analysis.StartFrequencyHz != 0 || analysis.StopFrequencyHz != 0 || analysis.Points != 0 || !validTransientGrid(analysis.DurationS, analysis.TimeStepS) || transientWork(analysis) > maxTransientWork {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: fmt.Sprintf("dynamic analysis requires finite %.0e <= time_step_s, duration_s <= %d, an exact integer grid, and at most %d steps", minTransientTimeStepS, maxTransientDurationS, maxTransientSteps)})
			}
			totalDynamicWork += transientWork(analysis)
		case AnalysisThermal:
			if analysis.DCSweep != nil {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".dc_sweep", Message: "DC source sweep is accepted only by DC operating-point analysis"})
			}
			if analysis.StartFrequencyHz != 0 || analysis.StopFrequencyHz != 0 || analysis.Points != 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "steady-state thermal analysis cannot contain frequency-sweep fields"})
			}
			driven := analysis.DurationS != 0 || analysis.TimeStepS != 0
			if driven {
				if !model.Transient {
					diagnostics = append(diagnostics, Diagnostic{Path: path + ".kind", Message: "periodically driven thermal analysis requires transient_circuit_v1"})
				}
				if !validTransientGrid(analysis.DurationS, analysis.TimeStepS) || transientWork(analysis) > maxTransientWork {
					diagnostics = append(diagnostics, Diagnostic{Path: path, Message: fmt.Sprintf("periodically driven thermal analysis requires finite %.0e <= time_step_s, duration_s <= %d, an exact integer grid, and at most %d steps", minTransientTimeStepS, maxTransientDurationS, maxTransientSteps)})
				}
				totalDynamicWork += transientWork(analysis)
			} else if model.Transient {
				// A transient-capable graph can still perform the legacy DC thermal
				// operating point. This keeps one resolved device set usable for both
				// quiescent and periodically driven thermal contracts.
			}
		default:
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".kind", Message: "analysis kind is not supported by graph MNA", Suggestion: "use dc_operating_point, ac_sweep, or transient in its dedicated workflow"})
		}
		if analysis.Kind == AnalysisThermal {
			if diagnosticsForConditions := validateNamedValues(path+".conditions", analysis.Conditions, []valueRule{{Name: "ambient_temperature_c", Minimum: -100, Maximum: 300}, {Name: "case_temperature_c", Optional: true, Minimum: -100, Maximum: 300}}); len(diagnosticsForConditions) != 0 {
				diagnostics = append(diagnostics, diagnosticsForConditions...)
			}
			if ambient := namedValueMap(analysis.Conditions)["ambient_temperature_c"]; !finite(ambient) || ambient < -100 || ambient > 300 {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".conditions.ambient_temperature_c", Message: "ambient temperature must be finite and within -100..300 C"})
			}
			if caseTemperature, exists := namedValue(namedValueMap(analysis.Conditions), "case_temperature_c"); exists && (!finite(caseTemperature) || caseTemperature < -100 || caseTemperature > 300) {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".conditions.case_temperature_c", Message: "case temperature must be finite and within -100..300 C"})
			}
		} else if len(analysis.Conditions) != 0 {
			if diagnosticsForConditions := validateNamedValues(path+".conditions", analysis.Conditions, []valueRule{{Name: "ambient_temperature_c", Minimum: -100, Maximum: 300}}); len(diagnosticsForConditions) != 0 {
				diagnostics = append(diagnostics, diagnosticsForConditions...)
			}
			if ambient := namedValueMap(analysis.Conditions)["ambient_temperature_c"]; !finite(ambient) || ambient < -100 || ambient > 300 {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".conditions.ambient_temperature_c", Message: "ambient temperature must be finite and within -100..300 C"})
			}
		}
		if len(analysis.DeviceOverrides) > maxMNADeviceOverrides {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".device_overrides", Message: fmt.Sprintf("analysis exceeds %d bounded device overrides", maxMNADeviceOverrides)})
		}
		seenOverrides := map[string]bool{}
		for overrideIndex, override := range analysis.DeviceOverrides {
			overridePath := fmt.Sprintf("%s.device_overrides[%d]", path, overrideIndex)
			component := strings.TrimSpace(override.Component)
			if _, exists := components[component]; !exists {
				diagnostics = append(diagnostics, Diagnostic{Path: overridePath + ".component", Message: "device override references a component absent from the circuit graph"})
			}
			if seenOverrides[component] {
				diagnostics = append(diagnostics, Diagnostic{Path: overridePath + ".component", Message: "device override is duplicated"})
			}
			seenOverrides[component] = true
			if override.ValueSI == nil && len(override.ModelParameters) == 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: overridePath, Message: "device override requires a bounded value or model parameter"})
			}
			if override.ValueSI != nil && (!finite(*override.ValueSI) || *override.ValueSI < 0 || (*override.ValueSI == 0 && components[component] != "capacitor")) {
				diagnostics = append(diagnostics, Diagnostic{Path: overridePath + ".value_si", Message: "device override value must be finite and positive, except that a capacitor may be zero to represent an absent load"})
			}
			previousParameter := ""
			for parameterIndex, parameter := range override.ModelParameters {
				if strings.TrimSpace(parameter.Name) == "" || parameter.Name <= previousParameter || !finite(parameter.Value) {
					diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("%s.model_parameters[%d]", overridePath, parameterIndex), Message: "override model parameters must be finite, unique, and canonically ordered"})
				}
				previousParameter = parameter.Name
			}
		}
		if len(analysis.Excitations) == 0 || len(analysis.Excitations) > maxMNAExcitations {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".excitations", Message: fmt.Sprintf("analysis requires 1..%d catalog source excitations", maxMNAExcitations)})
		}
		seenSources := map[string]struct{}{}
		sineSources := 0
		for sourceIndex, excitation := range analysis.Excitations {
			sourcePath := fmt.Sprintf("%s.excitations[%d]", path, sourceIndex)
			component := strings.TrimSpace(excitation.Component)
			_, exists := components[component]
			if !exists {
				diagnostics = append(diagnostics, Diagnostic{Path: sourcePath + ".component", Message: "source component is not declared in the circuit graph"})
			}
			if _, duplicate := seenSources[component]; duplicate {
				diagnostics = append(diagnostics, Diagnostic{Path: sourcePath + ".component", Message: "source component is duplicated within the analysis"})
			}
			seenSources[component] = struct{}{}
			if !boundedMagnitude(excitation.DCValue) || !boundedMagnitude(excitation.ACMagnitude) || excitation.ACMagnitude < 0 || !finite(excitation.ACPhaseDeg) || excitation.ACPhaseDeg < -360 || excitation.ACPhaseDeg > 360 || !boundedMagnitude(excitation.PulseInitialValue) || !boundedMagnitude(excitation.PulseValue) || !boundedMagnitude(excitation.SineAmplitude) || !finite(excitation.SineFrequencyHz) || !finite(excitation.SinePhaseDeg) {
				diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "source conditions must be finite and bounded; AC magnitude must be nonnegative and phase within -360..360 degrees"})
			}
			if hasSine(excitation) {
				sineSources++
			}
			if analysis.Kind == AnalysisDCOperatingPoint && (excitation.ACMagnitude != 0 || excitation.ACPhaseDeg != 0) {
				diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "DC operating-point excitation cannot contain AC magnitude or phase"})
			}
			if analysis.Kind == AnalysisThermal && (excitation.ACMagnitude != 0 || excitation.ACPhaseDeg != 0) {
				diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "steady-state thermal excitation accepts DC values only"})
			}
			if (analysis.Kind == AnalysisNoise || analysis.Kind == AnalysisStability) && (excitation.ACMagnitude != 0 || excitation.ACPhaseDeg != 0) {
				diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "noise and stability analyses require zeroed small-signal independent sources; dc_value is retained only to solve the trusted operating point"})
			}
			if analysis.Kind != AnalysisTransient && hasPulse(excitation) {
				diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "pulse conditions are accepted only by transient analysis"})
			}
			if analysis.Kind != AnalysisDistortion && analysis.Kind != AnalysisTransient && analysis.Kind != AnalysisThermal && hasSine(excitation) {
				diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "sine conditions are accepted only by transient, distortion, or periodically driven thermal analysis"})
			}
			if analysis.Kind == AnalysisDistortion {
				if excitation.ACMagnitude != 0 || excitation.ACPhaseDeg != 0 || hasPulse(excitation) {
					diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "distortion excitation cannot mix AC-sweep or pulse conditions with its bounded sine"})
				}
				if hasSine(excitation) && (!validDistortionSine(excitation, analysis) || excitation.SineAmplitude <= 0 || excitation.SineFrequencyHz <= 0 || excitation.SinePhaseDeg < -360 || excitation.SinePhaseDeg > 360) {
					diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "distortion sine requires positive amplitude/frequency, phase within -360..360 degrees, at least 16 samples per cycle, and an exact grid containing at least four complete cycles"})
				}
			}
			if analysis.Kind == AnalysisThermal && hasSine(excitation) {
				if excitation.ACMagnitude != 0 || excitation.ACPhaseDeg != 0 || hasPulse(excitation) || !validDistortionSine(excitation, analysis) || excitation.SineAmplitude <= 0 || excitation.SineFrequencyHz <= 0 || excitation.SinePhaseDeg < -360 || excitation.SinePhaseDeg > 360 {
					diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "periodically driven thermal sine requires positive amplitude/frequency, phase within -360..360 degrees, at least 16 samples per cycle, and an exact grid containing at least four complete cycles"})
				}
			}
			if analysis.Kind == AnalysisStartup && (excitation.ACMagnitude != 0 || excitation.ACPhaseDeg != 0) {
				diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "startup excitation accepts only the bounded final dc_value applied after the zero-energy initial point"})
			}
			if analysis.Kind == AnalysisTransient {
				if excitation.ACMagnitude != 0 || excitation.ACPhaseDeg != 0 {
					diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "transient excitation cannot contain AC magnitude or phase"})
				}
				if excitation.PulsePeriodS != 0 {
					// PulseInitialValue and PulseValue are absolute source levels,
					// not offsets from DCValue. Requiring zero DCValue keeps one
					// canonical representation and prevents an ambiguous double bias.
					if excitation.DCValue != 0 || !finite(excitation.PulseDelayS) || !finite(excitation.PulseWidthS) || !finite(excitation.PulsePeriodS) || excitation.PulseDelayS < 0 || excitation.PulseDelayS >= analysis.DurationS || excitation.PulseWidthS <= 0 || excitation.PulsePeriodS <= excitation.PulseWidthS || !onTransientGrid(excitation.PulseDelayS, analysis.TimeStepS) || !onTransientGrid(excitation.PulseWidthS, analysis.TimeStepS) || !onTransientGrid(excitation.PulsePeriodS, analysis.TimeStepS) {
						diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "transient pulse uses absolute initial/pulsed levels and requires zero dc_value, a rising edge within duration, 0 < width < period, and all times exactly on the observation grid"})
					}
				} else if excitation.PulseDelayS != 0 || excitation.PulseWidthS != 0 || excitation.PulseInitialValue != 0 || excitation.PulseValue != 0 {
					diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "transient pulse fields require a positive pulse_period_s"})
				}
				if hasSine(excitation) {
					samplesPerCycle := 1 / (excitation.SineFrequencyHz * analysis.TimeStepS)
					if hasPulse(excitation) || excitation.SineAmplitude <= 0 || excitation.SineFrequencyHz <= 0 || excitation.SinePhaseDeg < -360 || excitation.SinePhaseDeg > 360 || !finite(samplesPerCycle) || samplesPerCycle < 16 {
						diagnostics = append(diagnostics, Diagnostic{Path: sourcePath, Message: "transient sine requires positive amplitude/frequency, phase within -360..360 degrees, no pulse fields, and at least 16 samples per cycle"})
					}
				}
			}
		}
		if analysis.DCSweep != nil {
			if _, exists := seenSources[analysis.DCSweep.Component]; !exists {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".dc_sweep.component", Message: "DC sweep component must also have a canonical source excitation"})
			}
		}
		if analysis.Kind == AnalysisDistortion && sineSources != 1 {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".excitations", Message: "distortion analysis requires exactly one bounded sine source"})
		}
		if analysis.Kind == AnalysisThermal {
			driven := analysis.DurationS != 0 || analysis.TimeStepS != 0
			if (driven && sineSources != 1) || (!driven && sineSources != 0) {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".excitations", Message: "periodically driven thermal analysis requires exactly one bounded sine source and DC thermal analysis requires none"})
			}
		}
	}
	if model.Transient && totalDynamicWork > maxTotalDynamicWork {
		diagnostics = append(diagnostics, Diagnostic{Path: "analyses", Message: fmt.Sprintf("dynamic analysis set exceeds bounded total work limit %d", maxTotalDynamicWork), Suggestion: "partition operating cases into smaller trusted plan batches"})
	}
	if totalDCSweepWork > maxTotalDCSweepWork {
		diagnostics = append(diagnostics, Diagnostic{Path: "analyses", Message: fmt.Sprintf("DC source sweep set exceeds bounded total work limit %d", maxTotalDCSweepWork), Suggestion: "partition operating cases into smaller trusted plan batches"})
	}
	if len(intent.Assertions) == 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "assertions", Message: "graph MNA requires at least one structured node assertion"})
	}
	seenAssertions := map[string]struct{}{}
	for index, assertion := range intent.Assertions {
		path := fmt.Sprintf("assertions[%d]", index)
		if assertion.Metric != "" {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".metric", Message: "graph MNA assertions are structured by analysis, node, quantity, and optional frequency"})
		}
		kind, exists := analysisKinds[assertion.AnalysisID]
		if !exists {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".analysis_id", Message: "assertion references an unknown analysis"})
		}
		nodeOptional := kind == AnalysisThermal || assertion.Quantity == QuantityDeviceCurrentA || assertion.Quantity == QuantityTotalSupplyCurrentA
		if !nodeOptional && strings.TrimSpace(assertion.Node) == "" {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".node", Message: "assertion node is required"})
		}
		componentRequired := kind == AnalysisThermal || assertion.Quantity == QuantityDeviceCurrentA || assertion.Quantity == QuantityTransimpedanceOhm || assertion.Quantity == QuantityOutputPowerW
		if componentRequired && strings.TrimSpace(assertion.Component) == "" {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".component", Message: "component-scoped assertion requires a resolved component"})
		}
		if !componentRequired && assertion.Component != "" {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".component", Message: "assertion quantity does not accept a component scope"})
		}
		componentsRequired := assertion.Quantity == QuantityTotalSupplyCurrentA
		if componentsRequired && (len(assertion.Components) == 0 || !slices.IsSorted(assertion.Components)) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".components", Message: "total-supply-current assertion requires canonically ordered resolved components"})
		}
		for componentIndex, component := range assertion.Components {
			if strings.TrimSpace(component) == "" || (componentIndex > 0 && assertion.Components[componentIndex-1] == component) {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".components", Message: "assertion components must be non-empty and unique"})
				break
			}
		}
		if !componentsRequired && len(assertion.Components) != 0 {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".components", Message: "assertion quantity does not accept a component collection"})
		}
		switch assertion.Quantity {
		case QuantityVoltageV:
			if kind != AnalysisDCOperatingPoint && kind != AnalysisTransient {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "AC assertions must use magnitude, phase, or dBV"})
			}
		case QuantityVoltageMagnitudeV, QuantityVoltagePhaseDeg, QuantityVoltageDBV:
			if kind != AnalysisACSweep {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "voltage magnitude, phase, and dBV assertions require AC sweep analysis"})
			}
		case QuantityRiseTimeS, QuantityFallTimeS:
			if kind != AnalysisTransient {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "edge-time assertions require transient analysis"})
			}
		case QuantityIntegratedNoiseVRMS:
			if kind != AnalysisNoise {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "integrated-noise assertions require noise analysis"})
			}
		case QuantityPhaseMarginDeg, QuantityGainMarginDB:
			if kind != AnalysisStability {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "phase- and gain-margin assertions require stability analysis"})
			}
		case QuantityPeakAbsVoltageV:
			if kind != AnalysisStartup && kind != AnalysisTransient {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "peak absolute voltage assertions require startup or transient analysis"})
			}
		case QuantityTHDPercent:
			if kind != AnalysisDistortion {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "THD assertions require distortion analysis"})
			}
		case QuantityDeviceDissipationW, QuantityJunctionTemperatureC:
			if kind != AnalysisThermal {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "device dissipation and junction-temperature assertions require thermal analysis"})
			}
		case QuantityVoltageGainRatio:
			if kind != AnalysisACSweep || strings.TrimSpace(assertion.ReferenceNode) == "" {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "voltage-gain assertion requires AC sweep and a reference node"})
			}
		case QuantityCutoffFrequencyHz, QuantityBandwidthHz:
			if kind != AnalysisACSweep || strings.TrimSpace(assertion.ReferenceNode) == "" {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "cutoff/bandwidth assertion requires AC sweep and a reference node"})
			}
		case QuantityOutputSwingVPP, QuantitySettlingTimeS, QuantityResponseTimeS:
			if kind != AnalysisTransient {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "waveform-derived assertion requires transient analysis"})
			}
		case QuantityDeviceCurrentA, QuantityTotalSupplyCurrentA, QuantityTransimpedanceOhm:
			if kind != AnalysisDCOperatingPoint {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "device-current/total-supply-current/transimpedance assertion requires DC operating-point analysis"})
			}
		case QuantityOutputPowerW:
			if kind != AnalysisTransient {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "output-power assertion requires transient analysis"})
			}
		case QuantityThresholdVoltageV, QuantityThresholdCurrentA, QuantityHysteresisVoltageV:
			analysis, _ := analysisByID(intent.Analyses, assertion.AnalysisID)
			if kind != AnalysisDCOperatingPoint || analysis.DCSweep == nil {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "threshold and hysteresis assertions require a bounded DC source sweep"})
			} else {
				family := components[analysis.DCSweep.Component]
				if assertion.Quantity == QuantityThresholdVoltageV && family != "voltage_source" && family != "connector" {
					diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "voltage threshold requires a swept voltage source"})
				}
				if assertion.Quantity == QuantityThresholdCurrentA && family != "current_source" {
					diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "current threshold requires a swept current source"})
				}
				if assertion.Quantity == QuantityHysteresisVoltageV && (!analysis.DCSweep.Bidirectional || (family != "voltage_source" && family != "connector")) {
					diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "voltage hysteresis requires a bidirectional swept voltage source"})
				}
			}
		default:
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".quantity", Message: "assertion quantity is not supported"})
		}
		acPointQuantity := assertion.Quantity == QuantityVoltageMagnitudeV || assertion.Quantity == QuantityVoltagePhaseDeg || assertion.Quantity == QuantityVoltageDBV || assertion.Quantity == QuantityVoltageGainRatio
		if kind == AnalysisACSweep && acPointQuantity && (!finite(assertion.FrequencyHz) || assertion.FrequencyHz <= 0) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".frequency_hz", Message: "point AC assertion requires a finite positive sweep frequency"})
		}
		if kind == AnalysisACSweep && !acPointQuantity && assertion.FrequencyHz != 0 {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".frequency_hz", Message: "sweep-derived AC assertion cannot specify a frequency"})
		}
		if kind != AnalysisACSweep && assertion.FrequencyHz != 0 {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".frequency_hz", Message: "derived and non-frequency assertions cannot specify a frequency"})
		}
		if kind == AnalysisTransient {
			if assertion.FrequencyHz != 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".frequency_hz", Message: "transient assertion cannot specify a frequency"})
			}
			analysis, _ := analysisByID(intent.Analyses, assertion.AnalysisID)
			if assertion.Quantity == QuantityVoltageV && (!finite(assertion.TimeS) || assertion.TimeS < 0 || assertion.TimeS > analysis.DurationS || !onTransientGrid(assertion.TimeS, analysis.TimeStepS)) {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".time_s", Message: "transient voltage assertion time must be an exact observation point"})
			}
			if assertion.Quantity != QuantityVoltageV && assertion.TimeS != 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".time_s", Message: "edge-time assertion derives its interval and cannot specify time_s"})
			}
		} else if assertion.TimeS != 0 {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".time_s", Message: "non-transient assertion cannot specify time_s"})
		}
		if !finite(assertion.Min) || !finite(assertion.Max) || assertion.Min > assertion.Max {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "assertion bounds must be finite and minimum must not exceed maximum"})
		}
		key := assertionKey(assertion)
		if _, duplicate := seenAssertions[key]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "structured assertion is duplicated"})
		}
		seenAssertions[key] = struct{}{}
	}
	return diagnostics
}

func ResolveWithTopology(intent Intent, catalogID, catalogHash string, components []ComponentEvidence, nodes []NodeEvidence) (Plan, []Diagnostic) {
	intent.ModelID = strings.TrimSpace(intent.ModelID)
	model, ok := definitionByID(intent.ModelID)
	if ok && model.GraphMNA {
		return resolveMNA(intent, catalogID, catalogHash, components, nodes)
	}
	return Resolve(intent, catalogID, catalogHash, components)
}

func resolveMNA(intent Intent, catalogID, catalogHash string, components []ComponentEvidence, nodes ...[]NodeEvidence) (Plan, []Diagnostic) {
	families := make(map[string]string, len(components))
	for _, component := range components {
		families[component.InstanceID] = component.Family
	}
	if diagnostics := validateMNAIntent(intent, families); len(diagnostics) != 0 {
		return Plan{}, diagnostics
	}
	var nodeEvidence []NodeEvidence
	if len(nodes) != 0 {
		nodeEvidence = nodes[0]
	}
	if len(nodeEvidence) == 0 {
		return Plan{}, []Diagnostic{{Path: "topology.nodes", Message: "graph MNA resolution requires resolved circuit net evidence"}}
	}
	ground, nodeNames, diagnostics := canonicalNodes(nodeEvidence)
	if len(diagnostics) != 0 {
		return Plan{}, diagnostics
	}
	referencedSources := map[string]struct{}{}
	for _, analysis := range intent.Analyses {
		for _, excitation := range analysis.Excitations {
			referencedSources[excitation.Component] = struct{}{}
		}
	}
	sortedComponents := append([]ComponentEvidence(nil), components...)
	slices.SortStableFunc(sortedComponents, func(a, b ComponentEvidence) int { return strings.Compare(a.InstanceID, b.InstanceID) })
	devices := make([]ResolvedDevice, 0, len(sortedComponents))
	uncertainties := []Uncertainty{}
	model, _ := definitionByID(intent.ModelID)
	for _, component := range sortedComponents {
		matches := compatiblePrimitiveClaims(component, model, uniqueIntentAnalysisKind(intent.Analyses))
		_, sourceReferenced := referencedSources[component.InstanceID]
		if len(matches) == 0 {
			if sourceReferenced {
				diagnostics = append(diagnostics, Diagnostic{Path: "analyses.excitations." + component.InstanceID, Message: "source component family has no trusted MNA primitive"})
			} else if len(component.Connections) != 0 && !mnaBoundaryFamily(component.Family) {
				kind := "linear MNA"
				if model.NonlinearDC {
					kind = "nonlinear DC"
				}
				if model.Transient {
					kind = "transient"
				}
				diagnostics = append(diagnostics, Diagnostic{Path: "topology.devices." + component.InstanceID, Message: fmt.Sprintf("connected catalog component %s family %s has no trusted %s primitive claim", component.CatalogID, component.Family, kind), Suggestion: "select a component with a unique reviewed catalog primitive claim"})
			}
			continue
		}
		if len(matches) != 1 {
			diagnostics = append(diagnostics, Diagnostic{Path: "topology.devices." + component.InstanceID, Message: "catalog component declares ambiguous trusted primitive claims", Suggestion: "retain exactly one reviewed primitive claim compatible with this workflow"})
			continue
		}
		primitive, claim := matches[0].primitive, matches[0].claim
		if sourceReferenced && !primitive.Source {
			diagnostics = append(diagnostics, Diagnostic{Path: "analyses.excitations." + component.InstanceID, Message: fmt.Sprintf("catalog component family %s is not a trusted independent source", component.Family)})
			continue
		}
		if primitive.Source && !sourceReferenced {
			continue
		}
		if parameterDiagnostics := validatePrimitiveParameters("topology.devices."+component.InstanceID+".model_parameters", primitive, claim.Parameters); len(parameterDiagnostics) != 0 {
			diagnostics = append(diagnostics, parameterDiagnostics...)
			continue
		}
		device := ResolvedDevice{
			Component: component.InstanceID, PhysicalComponent: component.PhysicalComponent,
			CatalogID: component.CatalogID, Family: component.Family, PrimitiveModel: primitive.ID,
			ModelParameters: normalizeNamedValues(claim.Parameters),
		}
		if primitive.RequiresValueSI {
			if !component.HasValueSI || !finite(component.ValueSI) || component.ValueSI <= 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: "topology.devices." + component.InstanceID + ".value_si", Message: "trusted primitive requires a finite positive catalog-validated component value"})
				continue
			}
			value := component.ValueSI
			device.ValueSI = &value
		}
		for _, terminal := range primitive.Terminals {
			net, terminalDiagnostics := connectedPrimitiveNet(component, primitive, terminal)
			if len(terminalDiagnostics) != 0 {
				diagnostics = append(diagnostics, terminalDiagnostics...)
				continue
			}
			device.Terminals = append(device.Terminals, TerminalBinding{Terminal: terminal, Net: net})
		}
		if len(device.Terminals) == len(primitive.Terminals) {
			devices = append(devices, device)
			for _, uncertainty := range component.Uncertainties {
				if uncertainty.Target == "excitation_dc_value" {
					matched := false
					for _, analysis := range intent.Analyses {
						for _, excitation := range analysis.Excitations {
							if excitation.Component != component.InstanceID {
								continue
							}
							if !sameUncertaintyValue(excitation.DCValue, uncertainty.Nominal) {
								diagnostics = append(diagnostics, Diagnostic{Path: "analyses." + analysis.ID + ".excitations." + component.InstanceID, Message: "reviewed source uncertainty nominal does not match the bounded operating condition"})
								continue
							}
							bound := uncertainty
							bound.Target = "analyses." + analysis.ID + ".excitations." + component.InstanceID + ".dc_value"
							uncertainties = append(uncertainties, bound)
							matched = true
						}
					}
					if !matched {
						diagnostics = append(diagnostics, Diagnostic{Path: "topology.devices." + component.InstanceID + ".uncertainties", Message: "reviewed source uncertainty has no matching bounded DC excitation"})
					}
					continue
				}
				if uncertainty.Target != "value_si" && !strings.HasPrefix(uncertainty.Target, "model_parameters.") {
					diagnostics = append(diagnostics, Diagnostic{Path: "topology.devices." + component.InstanceID + ".uncertainties", Message: "catalog uncertainty target is incompatible with the trusted MNA primitive"})
					continue
				}
				uncertainty.Target = "devices." + component.InstanceID + "." + uncertainty.Target
				uncertainties = append(uncertainties, uncertainty)
			}
		}
	}
	if len(diagnostics) != 0 {
		return Plan{}, diagnostics
	}
	nodeNames = modeledNodeNames(ground, nodeNames, devices)
	plan := Plan{
		RegistryVersion: RegistryVersion, RegistryHash: RegistryHash(), CatalogID: catalogID, CatalogHash: catalogHash,
		ModelID: intent.ModelID, GroundNode: ground, Nodes: nodeNames, Devices: devices,
		Analyses: canonicalAnalyses(intent.Analyses), Assertions: append([]Assertion(nil), intent.Assertions...), WorstCase: intent.WorstCase,
		Uncertainties: append([]Uncertainty(nil), uncertainties...),
	}
	slices.SortStableFunc(plan.Uncertainties, func(a, b Uncertainty) int { return strings.Compare(a.Target, b.Target) })
	slices.SortStableFunc(plan.Assertions, func(a, b Assertion) int { return strings.Compare(assertionKey(a), assertionKey(b)) })
	plan.TopologyHash = topologyHash(plan.GroundNode, plan.Nodes, plan.Devices)
	if diagnostics := validateMNAPlan(plan); len(diagnostics) != 0 {
		return Plan{}, diagnostics
	}
	return plan, nil
}

func modeledNodeNames(ground string, nodes []string, devices []ResolvedDevice) []string {
	used := map[string]struct{}{ground: {}}
	for _, device := range devices {
		for _, terminal := range device.Terminals {
			used[terminal.Net] = struct{}{}
		}
	}
	modeled := make([]string, 0, len(used))
	for _, node := range nodes {
		if _, ok := used[node]; ok {
			modeled = append(modeled, node)
		}
	}
	return modeled
}

func mnaBoundaryFamily(family string) bool {
	switch family {
	case "connector", "testpoint":
		return true
	default:
		return false
	}
}

func validatePrimitiveParameters(path string, primitive primitiveDefinition, parameters []NamedValue) []Diagnostic {
	diagnostics := validateNamedValues(path, parameters, primitive.CatalogParameters)
	if len(diagnostics) != 0 {
		return diagnostics
	}
	values := namedValueMap(parameters)
	if primitive.OpAmp {
		minimum := values["supply_min_v"]
		maximum := values["supply_max_v"]
		lowMargin := values["output_low_margin_v"]
		highMargin := values["output_high_margin_v"]
		if maximum <= minimum {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "op-amp supply_max_v must exceed supply_min_v"})
		}
		if lowMargin+highMargin >= minimum {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "op-amp output margins leave no valid output range at supply_min_v"})
		}
	}
	if primitive.ID == PrimitiveCurrentSenseAmplifierV1 {
		if values["supply_max_v"] <= values["supply_min_v"] {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "current-sense amplifier supply_max_v must exceed supply_min_v"})
		}
		if values["common_mode_max_v"] <= values["common_mode_min_v"] {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "current-sense amplifier common_mode_max_v must exceed common_mode_min_v"})
		}
		if values["output_low_margin_v"]+values["output_high_margin_v"] >= values["supply_min_v"] {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "current-sense amplifier output margins leave no valid output range at supply_min_v"})
		}
	}
	if (primitive.ID == PrimitiveAdjustableLinearRegulatorV1 || primitive.ID == PrimitiveFixedLinearRegulatorV1) && values["max_input_voltage_v"] <= values["min_input_voltage_v"] {
		diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "linear regulator max_input_voltage_v must exceed min_input_voltage_v"})
	}
	if primitive.ID == PrimitiveFloatingAdjustableRegulatorV1 && math.Abs(values["polarity"]) != 1 {
		diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "floating adjustable regulator polarity must be -1 or 1"})
	}
	if primitive.ID == PrimitiveDualOutputIsolatedConverterV1 && values["input_max_v"] <= values["input_min_v"] {
		diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "dual-output isolated converter input_max_v must exceed input_min_v"})
	}
	if primitive.ID == PrimitiveBidirectionalOpenDrainTranslatorV1 {
		if values["vcca_max_v"] <= values["vcca_min_v"] || values["vccb_max_v"] <= values["vccb_min_v"] {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "level-translator supply maxima must exceed their corresponding minima"})
		}
		if values["channel_off_resistance_ohm"] <= values["channel_on_resistance_ohm"] {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "level-translator off resistance must exceed on resistance"})
		}
	}
	return diagnostics
}

func validateMNAPlan(plan Plan) []Diagnostic {
	if strings.TrimSpace(plan.CatalogID) == "" || strings.TrimSpace(plan.CatalogHash) == "" {
		return []Diagnostic{{Path: "catalog", Message: "resolved MNA plan is missing immutable catalog identity evidence"}}
	}
	if len(plan.Bindings) != 0 || len(plan.Inputs) != 0 {
		return []Diagnostic{{Path: "topology", Message: "resolved MNA plan contains legacy topology bindings or inputs"}}
	}
	var diagnostics []Diagnostic
	model, modelExists := definitionByID(plan.ModelID)
	if !modelExists || !model.GraphMNA {
		diagnostics = append(diagnostics, Diagnostic{Path: "model_id", Message: "resolved MNA plan references an unsupported workflow model"})
	}
	if plan.GroundNode == "" || !slices.Contains(plan.Nodes, plan.GroundNode) {
		diagnostics = append(diagnostics, Diagnostic{Path: "ground_node", Message: "resolved MNA plan is missing its reference node"})
	}
	for index, node := range plan.Nodes {
		if strings.TrimSpace(node) == "" || (index > 0 && plan.Nodes[index-1] >= node) {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("nodes[%d]", index), Message: "resolved nodes must be nonempty, unique, and canonically ordered"})
		}
	}
	deviceFamilies := make(map[string]string, len(plan.Devices))
	devicePrimitives := make(map[string]string, len(plan.Devices))
	nonlinearDevices := 0
	for index, device := range plan.Devices {
		path := fmt.Sprintf("devices[%d]", index)
		if index > 0 && plan.Devices[index-1].Component >= device.Component {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".component", Message: "resolved devices must be unique and canonically ordered"})
		}
		primitive, exists := primitiveByID(device.PrimitiveModel)
		if !exists || device.Component == "" || device.CatalogID == "" || !primitiveFamilyCompatible(primitive.Family, device.Family) {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "resolved device is missing compatible primitive/catalog evidence"})
			continue
		}
		deviceFamilies[device.Component] = device.Family
		devicePrimitives[device.Component] = primitive.ID
		if primitive.Nonlinear {
			nonlinearDevices++
		}
		if primitive.Nonlinear && !model.NonlinearDC && (!model.SmallSignalNonlinear || !onlySmallSignalAnalyses(plan.Analyses)) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".primitive_model", Message: "workflow does not support this nonlinear primitive for every requested analysis"})
		}
		if primitive.Transient && !model.Transient {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".primitive_model", Message: "non-transient plan contains a transient-only primitive"})
		}
		if primitive.Comparator {
			for _, analysis := range plan.Analyses {
				switch analysis.Kind {
				case AnalysisDCOperatingPoint, AnalysisACSweep, AnalysisNoise, AnalysisStability, AnalysisTransient, AnalysisStartup, AnalysisThermal:
				default:
					diagnostics = append(diagnostics, Diagnostic{Path: path + ".primitive_model", Message: "open-collector comparator primitive does not support " + analysis.Kind + " analysis"})
				}
			}
		}
		if model.Transient && primitive.Family == "capacitor" && !primitive.Transient {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".primitive_model", Message: "transient plan capacitor requires a reviewed transient capacitor primitive"})
		}
		if primitive.RequiresValueSI && (device.ValueSI == nil || !finite(*device.ValueSI) || *device.ValueSI <= 0) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".value_si", Message: "resolved primitive requires a finite positive value"})
		}
		diagnostics = append(diagnostics, validatePrimitiveParameters(path+".model_parameters", primitive, device.ModelParameters)...)
		if len(device.Terminals) != len(primitive.Terminals) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".terminals", Message: "resolved primitive has an incomplete terminal set"})
			continue
		}
		for terminalIndex, terminal := range device.Terminals {
			if terminal.Terminal != primitive.Terminals[terminalIndex] || !slices.Contains(plan.Nodes, terminal.Net) {
				diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("%s.terminals[%d]", path, terminalIndex), Message: "resolved terminal is not canonical or references an unknown node"})
			}
		}
	}
	if model.ID == ModelNonlinearCircuitDCV1 && nonlinearDevices == 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "devices", Message: "nonlinear DC workflow requires at least one reviewed nonlinear device"})
	}
	intent := Intent{ModelID: plan.ModelID, Analyses: cloneAnalyses(plan.Analyses), Assertions: append([]Assertion(nil), plan.Assertions...)}
	diagnostics = append(diagnostics, validateMNAIntent(intent, deviceFamilies)...)
	for analysisIndex, analysis := range plan.Analyses {
		if analysisIndex > 0 && plan.Analyses[analysisIndex-1].ID >= analysis.ID {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("analyses[%d].id", analysisIndex), Message: "resolved analyses must be unique and canonically ordered"})
		}
		for sourceIndex, excitation := range analysis.Excitations {
			if sourceIndex > 0 && analysis.Excitations[sourceIndex-1].Component >= excitation.Component {
				diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("analyses[%d].excitations[%d]", analysisIndex, sourceIndex), Message: "resolved excitations must be unique and canonically ordered"})
			}
			primitive, exists := primitiveByID(devicePrimitives[excitation.Component])
			if !exists || !primitive.Source {
				diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("analyses[%d].excitations[%d].component", analysisIndex, sourceIndex), Message: "resolved excitation does not reference a trusted source primitive"})
			}
		}
		for overrideIndex, override := range analysis.DeviceOverrides {
			path := fmt.Sprintf("analyses[%d].device_overrides[%d]", analysisIndex, overrideIndex)
			if overrideIndex > 0 && analysis.DeviceOverrides[overrideIndex-1].Component >= override.Component {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "device overrides must be unique and canonically ordered"})
			}
			device, exists := resolvedDeviceByComponent(plan.Devices, override.Component)
			if !exists {
				continue
			}
			primitive, _ := primitiveByID(device.PrimitiveModel)
			if override.ValueSI != nil && !primitive.RequiresValueSI {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".value_si", Message: "resolved primitive does not accept a value override"})
			}
			device = applyDeviceOverride(device, override)
			diagnostics = append(diagnostics, validatePrimitiveParameters(path+".model_parameters", primitive, device.ModelParameters)...)
		}
	}
	for index, assertion := range plan.Assertions {
		analysis, exists := analysisByID(plan.Analyses, assertion.AnalysisID)
		nodeRequired := assertion.Quantity != QuantityDeviceCurrentA && assertion.Quantity != QuantityTotalSupplyCurrentA && analysis.Kind != AnalysisThermal
		if exists && nodeRequired && !slices.Contains(plan.Nodes, assertion.Node) {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("assertions[%d].node", index), Message: "assertion references a node absent from resolved topology"})
		}
		if assertion.ReferenceNode != "" && !slices.Contains(plan.Nodes, assertion.ReferenceNode) {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("assertions[%d].reference_node", index), Message: "assertion reference node is absent from resolved topology"})
		}
		if exists && (analysis.Kind == AnalysisThermal || assertion.Component != "") {
			if _, componentExists := deviceFamilies[assertion.Component]; !componentExists {
				diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("assertions[%d].component", index), Message: "thermal assertion references a component absent from resolved topology"})
			}
		}
		if exists {
			for _, component := range assertion.Components {
				if _, componentExists := deviceFamilies[component]; !componentExists {
					diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("assertions[%d].components", index), Message: "assertion references a component absent from resolved topology"})
				}
			}
		}
		if exists && analysis.Kind == AnalysisACSweep && assertion.FrequencyHz > 0 && !frequencyInSweep(analysis, assertion.FrequencyHz) {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("assertions[%d].frequency_hz", index), Message: "assertion frequency is not an exact point in the deterministic AC sweep", Suggestion: "choose one of the frequencies generated by start, stop, and point count"})
		}
		if exists && analysis.Kind == AnalysisTransient && assertion.Quantity == QuantityVoltageV && !onTransientGrid(assertion.TimeS, analysis.TimeStepS) {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("assertions[%d].time_s", index), Message: "assertion time is not an exact point in the deterministic transient grid"})
		}
		if index > 0 && assertionKey(plan.Assertions[index-1]) >= assertionKey(assertion) {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("assertions[%d]", index), Message: "resolved assertions must be unique and canonically ordered"})
		}
	}
	if expected := topologyHash(plan.GroundNode, plan.Nodes, plan.Devices); plan.TopologyHash == "" || plan.TopologyHash != expected {
		diagnostics = append(diagnostics, Diagnostic{Path: "topology_hash", Message: "resolved topology hash does not match canonical nodes and devices", Suggestion: "resolve the circuit again"})
	}
	return diagnostics
}

func onlySmallSignalAnalyses(analyses []Analysis) bool {
	if len(analyses) == 0 {
		return false
	}
	for _, analysis := range analyses {
		if !smallSignalAnalysis(analysis.Kind) {
			return false
		}
	}
	return true
}

func canonicalNodes(nodes []NodeEvidence) (string, []string, []Diagnostic) {
	var diagnostics []Diagnostic
	names := make([]string, 0, len(nodes))
	groundCandidates := map[string]struct{}{}
	seen := map[string]struct{}{}
	for index, node := range nodes {
		name := strings.TrimSpace(node.Name)
		if name == "" {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("topology.nodes[%d]", index), Message: "resolved node name is empty"})
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("topology.nodes[%d]", index), Message: "resolved node name is duplicated"})
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
		if strings.EqualFold(strings.TrimSpace(node.Role), "ground") || strings.EqualFold(strings.TrimSpace(node.VoltageDomain), "0V") {
			groundCandidates[name] = struct{}{}
		}
	}
	slices.Sort(names)
	grounds := make([]string, 0, len(groundCandidates))
	for name := range groundCandidates {
		grounds = append(grounds, name)
	}
	slices.Sort(grounds)
	if len(grounds) != 1 {
		diagnostics = append(diagnostics, Diagnostic{Path: "topology.ground", Message: fmt.Sprintf("graph MNA requires exactly one resolved ground/0V node, got %d", len(grounds)), Suggestion: "mark one circuit net as ground with a 0V domain"})
		return "", names, diagnostics
	}
	return grounds[0], names, diagnostics
}

func connectedNet(component ComponentEvidence, terminal string) (string, []Diagnostic) {
	return connectedNetAlternatives(component, terminal, []string{terminal})
}

func connectedPrimitiveNet(component ComponentEvidence, primitive primitiveDefinition, terminal string) (string, []Diagnostic) {
	alternatives := primitive.TerminalAliases[terminal]
	if len(alternatives) == 0 {
		alternatives = []string{terminal}
	}
	return connectedNetAlternatives(component, terminal, alternatives)
}

func connectedNetAlternatives(component ComponentEvidence, terminal string, alternatives []string) (string, []Diagnostic) {
	nets := map[string]struct{}{}
	for _, connection := range component.Connections {
		for _, alternative := range alternatives {
			if strings.EqualFold(strings.TrimSpace(connection.Function), alternative) {
				nets[strings.TrimSpace(connection.Net)] = struct{}{}
				break
			}
		}
	}
	if len(nets) != 1 {
		return "", []Diagnostic{{Path: "topology.devices." + component.InstanceID + ".terminals." + terminal, Message: fmt.Sprintf("trusted primitive terminal must resolve to exactly one net, got %d", len(nets)), Suggestion: "connect every simulated terminal alias to one resolved circuit net"}}
	}
	for net := range nets {
		return net, nil
	}
	panic("unreachable")
}

func canonicalAnalyses(source []Analysis) []Analysis {
	analyses := cloneAnalyses(source)
	for index := range analyses {
		analyses[index].ID = strings.TrimSpace(analyses[index].ID)
		slices.SortStableFunc(analyses[index].Excitations, func(a, b SourceExcitation) int { return strings.Compare(a.Component, b.Component) })
		analyses[index].Conditions = normalizeNamedValues(analyses[index].Conditions)
		for overrideIndex := range analyses[index].DeviceOverrides {
			analyses[index].DeviceOverrides[overrideIndex].Component = strings.TrimSpace(analyses[index].DeviceOverrides[overrideIndex].Component)
			analyses[index].DeviceOverrides[overrideIndex].ModelParameters = normalizeNamedValues(analyses[index].DeviceOverrides[overrideIndex].ModelParameters)
		}
		slices.SortStableFunc(analyses[index].DeviceOverrides, func(a, b DeviceOverride) int { return strings.Compare(a.Component, b.Component) })
	}
	slices.SortStableFunc(analyses, func(a, b Analysis) int { return strings.Compare(a.ID, b.ID) })
	return analyses
}

func resolvedDeviceByComponent(devices []ResolvedDevice, component string) (ResolvedDevice, bool) {
	for _, device := range devices {
		if device.Component == component {
			return device, true
		}
	}
	return ResolvedDevice{}, false
}

func applyDeviceOverride(device ResolvedDevice, override DeviceOverride) ResolvedDevice {
	if override.ValueSI != nil {
		value := *override.ValueSI
		device.ValueSI = &value
	}
	parameters := namedValueMap(device.ModelParameters)
	for _, parameter := range override.ModelParameters {
		parameters[parameter.Name] = parameter.Value
	}
	device.ModelParameters = make([]NamedValue, 0, len(parameters))
	for name, value := range parameters {
		device.ModelParameters = append(device.ModelParameters, NamedValue{Name: name, Value: value})
	}
	device.ModelParameters = normalizeNamedValues(device.ModelParameters)
	return device
}

func topologyHash(ground string, nodes []string, devices []ResolvedDevice) string {
	payload := struct {
		Ground  string           `json:"ground"`
		Nodes   []string         `json:"nodes"`
		Devices []ResolvedDevice `json:"devices"`
	}{Ground: ground, Nodes: nodes, Devices: devices}
	data, err := json.Marshal(payload)
	if err != nil {
		panic("MNA topology is not serializable: " + err.Error())
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func cloneDevices(source []ResolvedDevice) []ResolvedDevice {
	clone := append([]ResolvedDevice(nil), source...)
	for index := range clone {
		clone[index].ModelParameters = append([]NamedValue(nil), source[index].ModelParameters...)
		clone[index].Terminals = append([]TerminalBinding(nil), source[index].Terminals...)
		if source[index].ValueSI != nil {
			value := *source[index].ValueSI
			clone[index].ValueSI = &value
		}
	}
	return clone
}

func assertionKey(assertion Assertion) string {
	if assertion.Metric != "" {
		return "legacy\x00" + assertion.Metric
	}
	return fmt.Sprintf("mna\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%024.12e\x00%024.12e", assertion.AnalysisID, assertion.Node, assertion.Component, strings.Join(assertion.Components, "\x1f"), assertion.ReferenceNode, assertion.Quantity, assertion.FrequencyHz, assertion.TimeS)
}

func analysisByID(analyses []Analysis, id string) (Analysis, bool) {
	for _, analysis := range analyses {
		if analysis.ID == id {
			return analysis, true
		}
	}
	return Analysis{}, false
}

func validAnalysisID(value string) bool {
	for index, r := range value {
		if index == 0 && !unicode.IsLower(r) {
			return false
		}
		if !unicode.IsLower(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return value != "" && len(value) <= 64
}

func boundedMagnitude(value float64) bool {
	return finite(value) && math.Abs(value) <= maxMNASourceMagnitude
}

func hasPulse(excitation SourceExcitation) bool {
	return excitation.PulseInitialValue != 0 || excitation.PulseValue != 0 || excitation.PulseDelayS != 0 || excitation.PulseWidthS != 0 || excitation.PulsePeriodS != 0
}

func hasSine(excitation SourceExcitation) bool {
	return excitation.SineAmplitude != 0 || excitation.SineFrequencyHz != 0 || excitation.SinePhaseDeg != 0
}

func validDistortionSine(excitation SourceExcitation, analysis Analysis) bool {
	if excitation.SineFrequencyHz <= 0 || analysis.TimeStepS <= 0 || analysis.DurationS <= 0 {
		return false
	}
	samplesPerCycle := 1 / (excitation.SineFrequencyHz * analysis.TimeStepS)
	cycles := excitation.SineFrequencyHz * analysis.DurationS
	return samplesPerCycle >= 16 && math.Abs(samplesPerCycle-math.Round(samplesPerCycle)) <= 1e-9 && cycles >= 4 && math.Abs(cycles-math.Round(cycles)) <= 1e-9
}

func validTransientGrid(duration, step float64) bool {
	if !finite(duration) || !finite(step) || step < minTransientTimeStepS || duration < step || duration > maxTransientDurationS {
		return false
	}
	steps := math.Round(duration / step)
	return steps >= 1 && steps <= maxTransientSteps && math.Abs(duration-steps*step) <= math.Max(duration, step)*1e-12
}

func transientWork(analysis Analysis) int {
	if !finite(analysis.DurationS) || !finite(analysis.TimeStepS) || analysis.TimeStepS <= 0 {
		return maxTransientWork + 1
	}
	return (int(math.Round(analysis.DurationS/analysis.TimeStepS)) + len(nonlinearContinuation)) * transientMaxNewtonIterations
}

// FitsPlanDynamicWork reports whether a canonical analysis batch remains
// within the trusted per-plan dynamic-work bound. Callers may use it to split
// independent operating corners without weakening any individual analysis.
func FitsPlanDynamicWork(analyses []Analysis) bool {
	total := 0
	for _, analysis := range analyses {
		switch analysis.Kind {
		case AnalysisTransient, AnalysisStartup, AnalysisDistortion:
			total += transientWork(analysis)
		case AnalysisThermal:
			if analysis.DurationS != 0 || analysis.TimeStepS != 0 {
				total += transientWork(analysis)
			}
		}
		if total > maxTotalDynamicWork {
			return false
		}
	}
	return true
}

func onTransientGrid(value, step float64) bool {
	if !finite(value) || !finite(step) || step <= 0 || value < 0 {
		return false
	}
	index := math.Round(value / step)
	return math.Abs(value-index*step) <= math.Max(step, math.Abs(value))*1e-12
}

func frequencyInSweep(analysis Analysis, frequency float64) bool {
	for _, candidate := range sweepFrequencies(analysis) {
		if math.Abs(candidate-frequency) <= math.Max(1, math.Abs(candidate))*1e-12 {
			return true
		}
	}
	return false
}
