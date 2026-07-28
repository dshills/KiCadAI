package circuitgraph

import (
	"slices"
	"strings"
)

const FunctionCapabilitySchema = "kicadai.function-level-capabilities.v1"

type FunctionLevelCapabilityDocument struct {
	Schema            string               `json:"schema"`
	InputSchema       string               `json:"input_schema"`
	PolicyVersion     string               `json:"policy_version"`
	RequiredSections  []string             `json:"required_sections"`
	UnitConventions   []FunctionUnit       `json:"unit_conventions"`
	Operations        []FunctionCapability `json:"operations"`
	ReadinessLimits   []string             `json:"readiness_limits"`
	UnsupportedClaims []string             `json:"unsupported_claims"`
}

type FunctionCapability struct {
	Name               string                       `json:"name"`
	Description        string                       `json:"description"`
	SupportedRoles     []ComponentRole              `json:"supported_roles"`
	RequiredParameters []FunctionParameter          `json:"required_parameters"`
	OptionalParameters []FunctionParameter          `json:"optional_parameters"`
	EndpointRoles      []FunctionEndpointCapability `json:"endpoint_roles"`
	ProvenReadiness    AcceptanceLevel              `json:"proven_readiness"`
	Limitations        []string                     `json:"limitations"`
}

type FunctionParameter struct {
	Name        string `json:"name"`
	ValueKind   string `json:"value_kind"`
	Unit        string `json:"unit,omitempty"`
	Description string `json:"description"`
}

type FunctionEndpointCapability struct {
	Role        string   `json:"role"`
	Functions   []string `json:"functions"`
	Required    bool     `json:"required"`
	Description string   `json:"description"`
}

type FunctionUnit struct {
	Field string `json:"field"`
	Unit  string `json:"unit"`
}

var functionCapabilities = []FunctionCapability{
	{
		Name: "load_capacitor", Description: "Provide a catalog-qualified load capacitor for a passive resonator or oscillator network.", SupportedRoles: []ComponentRole{RoleCapacitor},
		EndpointRoles: []FunctionEndpointCapability{
			{Role: "crystal", Functions: []string{"A", "B"}, Required: false, Description: "Crystal-side capacitor terminal."},
			{Role: "return", Functions: []string{"A", "B"}, Required: false, Description: "Reference-return capacitor terminal."},
		},
		ProvenReadiness: AcceptanceERCDRC,
		Limitations:     []string{"load capacitance, board stray capacitance, tolerance, startup, and crystal drive require explicit calculation"},
	},
	{
		Name: "external_crystal_resonator", Description: "Select a passive crystal resonator for a controller's differential crystal-driver pins.", SupportedRoles: []ComponentRole{RoleOscillator},
		EndpointRoles: []FunctionEndpointCapability{
			{Role: "input", Functions: []string{"XTAL_1"}, Required: true, Description: "First resonator terminal connected to the controller oscillator input."},
			{Role: "output", Functions: []string{"XTAL_2"}, Required: true, Description: "Second resonator terminal connected to the controller oscillator output."},
		},
		ProvenReadiness: AcceptanceERCDRC,
		Limitations:     []string{"frequency, load capacitance, drive level, startup, tolerance, and environmental margins require explicit catalog and calculation evidence"},
	},
	{
		Name: "adjustable_linear_regulator", Description: "Select an adjustable linear regulator and derive its reviewed feedback network.", SupportedRoles: []ComponentRole{RoleRegulator},
		RequiredParameters: []FunctionParameter{{Name: "output_voltage_v", ValueKind: "string", Unit: "V", Description: "Requested nominal output voltage."}},
		OptionalParameters: []FunctionParameter{{Name: "maximum_output_current_ma", ValueKind: "string", Unit: "mA", Description: "Required output-current rating."}},
		EndpointRoles: []FunctionEndpointCapability{
			{Role: "input", Functions: []string{"VIN"}, Required: true, Description: "Positive input supply."},
			{Role: "output", Functions: []string{"VOUT"}, Required: true, Description: "Regulated output."},
			{Role: "return", Functions: []string{"GND"}, Required: true, Description: "Supply return."},
			{Role: "feedback", Functions: []string{"ADJ"}, Required: true, Description: "Feedback-divider node."},
		},
		ProvenReadiness: AcceptanceERCDRC,
		Limitations:     []string{"nominal divider synthesis does not prove tolerance, thermal, or transient performance"},
	},
	{
		Name: "fixed_linear_regulator", Description: "Select a fixed-output linear regulator and expand reviewed bypass companions.", SupportedRoles: []ComponentRole{RoleRegulator},
		EndpointRoles: []FunctionEndpointCapability{
			{Role: "input", Functions: []string{"VIN"}, Required: true, Description: "Positive input supply."},
			{Role: "output", Functions: []string{"VOUT"}, Required: true, Description: "Regulated output."},
			{Role: "return", Functions: []string{"GND"}, Required: true, Description: "Supply return."},
			{Role: "enable", Functions: []string{"EN"}, Required: false, Description: "Optional enable control."},
		},
		ProvenReadiness: AcceptanceERCDRC,
		Limitations:     []string{"thermal headroom and capacitor derating remain evidence requirements"},
	},
	{
		Name: "standalone_clock_source", Description: "Select a self-contained clock source with catalog-backed timing and output evidence.", SupportedRoles: []ComponentRole{RoleOscillator},
		EndpointRoles: []FunctionEndpointCapability{
			{Role: "output", Functions: []string{"OUT"}, Required: false, Description: "Generated clock output."},
			{Role: "supply", Functions: []string{"VDD"}, Required: false, Description: "Clock-source supply."},
			{Role: "return", Functions: []string{"GND"}, Required: false, Description: "Clock-source return."},
			{Role: "control", Functions: []string{"ENABLE", "DIV"}, Required: false, Description: "Optional enable or divider control."},
			{Role: "timing", Functions: []string{"SET", "GUARD"}, Required: false, Description: "Optional timing and guard nodes."},
		},
		ProvenReadiness: AcceptanceERCDRC,
		Limitations:     []string{"frequency, startup, jitter, duty cycle, environment, and load limits require quantitative catalog and simulation evidence"},
	},
	{
		Name: "clock_buffer", Description: "Buffer a generated clock through a qualified logic receiver and output stage.", SupportedRoles: []ComponentRole{RoleIC},
		EndpointRoles: []FunctionEndpointCapability{
			{Role: "input", Functions: []string{"IN"}, Required: false, Description: "Clock input."},
			{Role: "output", Functions: []string{"OUT"}, Required: false, Description: "Buffered clock output."},
			{Role: "supply", Functions: []string{"VCC"}, Required: false, Description: "Buffer supply."},
			{Role: "return", Functions: []string{"GND"}, Required: false, Description: "Buffer return."},
		},
		ProvenReadiness: AcceptanceERCDRC,
		Limitations:     []string{"input thresholds, propagation delay, output levels, edge rate, fanout, and capacitive loading require quantitative evidence"},
	},
	{
		Name: "timing_resistor", Description: "Program a timing function with a catalog-qualified precision resistor.", SupportedRoles: []ComponentRole{RoleResistor},
		EndpointRoles: []FunctionEndpointCapability{
			{Role: "timing", Functions: []string{"A", "B"}, Required: false, Description: "Timing-network terminals."},
		},
		ProvenReadiness: AcceptanceERCDRC,
		Limitations:     []string{"frequency accuracy requires tolerance and temperature-coefficient corner analysis"},
	},
	{
		Name: "i2c_peripheral", Description: "Select an I2C peripheral and apply reviewed address and unused-pin policies.", SupportedRoles: []ComponentRole{RoleSensor, RoleIC},
		OptionalParameters: []FunctionParameter{{Name: "i2c_address", ValueKind: "string", Description: "Reviewed address such as 0x44 or 0x76."}},
		EndpointRoles: []FunctionEndpointCapability{
			{Role: "data", Functions: []string{"SDA"}, Required: true, Description: "I2C data."},
			{Role: "clock", Functions: []string{"SCL"}, Required: true, Description: "I2C clock."},
			{Role: "supply", Functions: []string{"VDD", "VDDIO", "VCC"}, Required: false, Description: "Peripheral supply pins vary by selected component."},
			{Role: "return", Functions: []string{"GND", "VSS"}, Required: false, Description: "Peripheral return pin."},
		},
		ProvenReadiness: AcceptanceERCDRC,
		Limitations:     []string{"only addresses and strap policies present in reviewed component evidence are accepted"},
	},
	{
		Name: "low_side_switch", Description: "Use a transistor as a low-side load switch.", SupportedRoles: []ComponentRole{RoleBJT},
		EndpointRoles: []FunctionEndpointCapability{
			{Role: "control", Functions: []string{"BASE"}, Required: true, Description: "Base-drive input."},
			{Role: "load", Functions: []string{"COLLECTOR"}, Required: true, Description: "Switched load node."},
			{Role: "return", Functions: []string{"EMITTER"}, Required: true, Description: "Low-side return."},
		},
		ProvenReadiness: AcceptanceERCDRC,
		Limitations:     []string{"base drive, saturation, load transient, and thermal margins require explicit calculation"},
	},
	{
		Name: "noninverting_amplifier", Description: "Use an op-amp with an explicit non-inverting feedback network.", SupportedRoles: []ComponentRole{RoleIC},
		EndpointRoles: opAmpEndpointCapabilities(), ProvenReadiness: AcceptanceERCDRC,
		Limitations: []string{"ERC/DRC readiness does not prove gain accuracy, stability, noise, bandwidth, or output drive"},
	},
	{
		Name: "usb_c_power_sink", Description: "Use a reviewed USB-C power-only receptacle as a sink interface.", SupportedRoles: []ComponentRole{RoleConnector, RoleInputConnector},
		EndpointRoles: []FunctionEndpointCapability{
			{Role: "power", Functions: []string{"VBUS"}, Required: true, Description: "USB VBUS input."},
			{Role: "return", Functions: []string{"GND"}, Required: true, Description: "USB power return."},
			{Role: "configuration_1", Functions: []string{"CC1"}, Required: true, Description: "First configuration channel."},
			{Role: "configuration_2", Functions: []string{"CC2"}, Required: true, Description: "Second configuration channel."},
			{Role: "shield", Functions: []string{"SHIELD"}, Required: false, Description: "Connector shield policy endpoint."},
		},
		ProvenReadiness: AcceptanceERCDRC,
		Limitations:     []string{"power-only sink support does not claim USB data, USB PD negotiation, or regulatory compliance"},
	},
	{
		Name: "voltage_follower", Description: "Use an op-amp as a unity-gain buffer.", SupportedRoles: []ComponentRole{RoleIC},
		EndpointRoles: opAmpEndpointCapabilities(), ProvenReadiness: AcceptanceERCDRC,
		Limitations: []string{"ERC/DRC readiness does not prove stability, common-mode range, bandwidth, or output drive"},
	},
}

// legacyFunctionUsages are accepted for compatibility with internal frozen
// corpora and lower-level tests. They are not published capability claims;
// new public operations must carry complete metadata above.
var legacyFunctionUsages = []string{
	"ac_coupling",
	"active_high_gate_buffer_stage_1",
	"active_high_output_buffer",
	"active_high_output_pullup",
	"adc_drive_buffer",
	"adc_drive_isolation",
	"analog_servo_control",
	"automatic_power_on_mute",
	"base_bias",
	"base_bias_feedback",
	"base_bias_isolation",
	"base_stop",
	"bias_current",
	"bias_injection",
	"bias_reference_decoupling",
	"bias_supply_decoupling",
	"bidirectional_i2c_isolation",
	"bootstrap_capacitor",
	"bus_pullup",
	"bus_transient_protection",
	"channel_1_filter",
	"channel_2_filter",
	"clamp_current_limit",
	"class_a_emitter_follower",
	"class_a_output_bias",
	"class_a_voltage_gain",
	"class_ab_output",
	"class_ab_predriver",
	"class_ab_thermal_bias",
	"class_ab_voltage_driver",
	"coil_flyback_clamp",
	"collector_load",
	"collector_pullup",
	"comparator",
	"complementary_buffer",
	"configuration_strap",
	"control_overvoltage_clamp",
	"controlled_output_disconnect",
	"controller_bypass_capacitor",
	"current_limit",
	"current_sense",
	"current_sense_amplifier",
	"dc_fault_disconnect",
	"decoupling",
	"decoupling_capacitor",
	"default_block",
	"default_clamp",
	"default_fault",
	"default_mute",
	"default_off",
	"default_off_high_side_switch",
	"default_state",
	"delayed_startup_isolation",
	"direction_controlled_level_translator",
	"direction_pulldown",
	"drive_limit",
	"driver_emitter_degeneration",
	"driver_turnoff",
	"dual_voltage_follower",
	"dual_window_comparator",
	"edge_timing",
	"emitter_resistor",
	"enable_pulldown",
	"enable_pullup",
	"fail_safe_bidirectional_mute",
	"fail_safe_enable",
	"fail_safe_fault_clamp",
	"fail_safe_interlock",
	"fail_safe_logic_mute",
	"fault_indicator",
	"feedback",
	"feedback_divider",
	"feedback_gain",
	"filter",
	"frequency_programming",
	"gain_degeneration",
	"gain_feedback",
	"gate_buffer_input",
	"gate_buffer_interstage",
	"gate_buffer_stage_pullup",
	"gate_clamp_current_limit",
	"gate_control_inverter",
	"gate_drive",
	"gate_drive_pullup",
	"gate_stopper",
	"high_side_current_measurement",
	"i2c_bidirectional_translation",
	"i2c_pullup",
	"indicator_current_limit",
	"indicator_driver",
	"inductive_transient_clamp",
	"input_bias",
	"input_bias_return",
	"input_bulk_capacitor",
	"input_bypass_capacitor",
	"input_gain",
	"instrumentation_amplification",
	"interlock_drive",
	"inverting_amplifier",
	"isolated_low_noise_regulator",
	"isolated_power_stage",
	"level_translator",
	"local_measurement_supply",
	"logic_drive",
	"logic_pullup",
	"logic_supply_decoupling",
	"loop_compensation",
	"low_esr_output_capacitor",
	"microcontroller",
	"midpoint_bias",
	"midpoint_bypass",
	"mosfet",
	"mute_drive",
	"mute_relay_driver",
	"negative_clamp",
	"negative_post_regulator",
	"negative_rail_low_side_switch",
	"opamp",
	"open_collector_pullup",
	"output_bank_turnoff",
	"output_capacitor_esr",
	"output_inverter_base",
	"output_transient_clamp",
	"overcurrent_limit",
	"positive_clamp",
	"positive_feedback",
	"positive_post_regulator",
	"programmable_controller",
	"programming_header",
	"programming_series_isolation",
	"protected_isolated_power_stage",
	"pulse_filter",
	"push_pull_functional_isolation",
	"push_pull_level_translator",
	"rail_sequencing",
	"reference_bias",
	"reference_output_decoupling",
	"regulated_base_bias",
	"regulator",
	"relay_coil_drop",
	"relay_control_current",
	"reverse_current_blocking",
	"sensor",
	"sequence_delay",
	"series_current_shunt",
	"series_gate_overvoltage_clamp",
	"shutdown_discharge",
	"signal_amplification",
	"soft_start_capacitor",
	"source_referenced_gate_sink",
	"spi_peripheral",
	"split_supply_converter",
	"stability",
	"startup_bias_clamp",
	"startup_inactive",
	"startup_precharge",
	"status_indicator",
	"switching_inductor",
	"synchronous_buck_controller",
	"thermal_tracking",
	"threshold_divider",
	"threshold_reference",
	"threshold_voltage_reference",
	"transient_high_side_switch",
	"transient_low_side_switch",
	"transient_positive_clamp",
	"transient_protection",
	"transient_series_impedance",
	"uart_dual_domain_translation",
	"unused_output_load",
}

func FunctionLevelCapabilities() FunctionLevelCapabilityDocument {
	operations := cloneFunctionCapabilities(functionCapabilities)
	slices.SortStableFunc(operations, func(left, right FunctionCapability) int {
		return strings.Compare(left.Name, right.Name)
	})
	return FunctionLevelCapabilityDocument{
		Schema:           FunctionCapabilitySchema,
		InputSchema:      SchemaID,
		PolicyVersion:    SynthesisPolicyVersion,
		RequiredSections: []string{"schema", "version", "project", "synthesis.functions", "synthesis.interfaces", "synthesis.power_domains", "synthesis.connections", "synthesis.constraints", "policy"},
		UnitConventions: []FunctionUnit{
			{Field: "power_domains.voltage_v", Unit: "V"},
			{Field: "power_domains.max_current_ma", Unit: "mA"},
			{Field: "connections.current_ma", Unit: "mA"},
			{Field: "constraints.max_width_mm", Unit: "mm"},
			{Field: "constraints.max_height_mm", Unit: "mm"},
			{Field: "constraints.preferred_component_spacing_mm", Unit: "mm"},
		},
		Operations:        operations,
		ReadinessLimits:   []string{"proven_readiness is fixture evidence, not a guarantee for every selected component or topology", "KiCad ERC/DRC, connectivity, route completion, writer correctness, and zero round-trip differences must be requested and pass for erc-drc promotion"},
		UnsupportedClaims: []string{"fabrication release", "regulatory compliance", "unmodeled analog or thermal performance", "general high-speed, RF, or unrestricted autorouting"},
	}
}

func FunctionCapabilityForUsage(usage string) (FunctionCapability, bool) {
	usage = strings.TrimSpace(usage)
	for _, capability := range functionCapabilities {
		if capability.Name == usage {
			return cloneFunctionCapability(capability), true
		}
	}
	return FunctionCapability{}, false
}

func knownLegacyFunctionUsage(usage string) bool {
	usage = strings.TrimSpace(usage)
	_, found := slices.BinarySearch(legacyFunctionUsages, usage)
	return found
}

func opAmpEndpointCapabilities() []FunctionEndpointCapability {
	return []FunctionEndpointCapability{
		{Role: "negative_input", Functions: []string{"IN_MINUS"}, Required: true, Description: "Inverting input."},
		{Role: "positive_input", Functions: []string{"IN_PLUS"}, Required: true, Description: "Non-inverting input."},
		{Role: "output", Functions: []string{"OUT"}, Required: true, Description: "Amplifier output."},
		{Role: "positive_supply", Functions: []string{"V_PLUS"}, Required: true, Description: "Positive supply."},
		{Role: "negative_supply", Functions: []string{"V_MINUS"}, Required: true, Description: "Negative supply or return."},
	}
}

func cloneFunctionCapabilities(source []FunctionCapability) []FunctionCapability {
	result := make([]FunctionCapability, len(source))
	for index, capability := range source {
		result[index] = cloneFunctionCapability(capability)
	}
	return result
}

func cloneFunctionCapability(capability FunctionCapability) FunctionCapability {
	capability.SupportedRoles = slices.Clone(capability.SupportedRoles)
	capability.RequiredParameters = slices.Clone(capability.RequiredParameters)
	if capability.RequiredParameters == nil {
		capability.RequiredParameters = []FunctionParameter{}
	}
	capability.OptionalParameters = slices.Clone(capability.OptionalParameters)
	if capability.OptionalParameters == nil {
		capability.OptionalParameters = []FunctionParameter{}
	}
	capability.EndpointRoles = slices.Clone(capability.EndpointRoles)
	for index := range capability.EndpointRoles {
		capability.EndpointRoles[index].Functions = slices.Clone(capability.EndpointRoles[index].Functions)
	}
	capability.Limitations = slices.Clone(capability.Limitations)
	if capability.Limitations == nil {
		capability.Limitations = []string{}
	}
	return capability
}
