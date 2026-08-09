package capabilityfeedback

import (
	"kicadai/internal/capabilityevaluation"
	ots "kicadai/internal/opentopologysynthesis"
)

type closedLoopSeed struct {
	ID, File, Name, Title, Description string
	Role                               CorpusRole
	Domain                             capabilityevaluation.Domain
	Safety                             capabilityevaluation.SafetyImpact
	SupplyMin, SupplyNom, SupplyMax    float64
	SupplyCurrent                      float64
	Signals                            []ots.Port
	Cases                              []ots.OperatingCase
	Assertions                         []ots.BehavioralAssertion
}

func closedLoopCorpusSeeds() []closedLoopSeed {
	return append(closedLoopDiscoverySeeds(), closedLoopHeldOutSeeds()...)
}

func closedLoopDiscoverySeeds() []closedLoopSeed {
	return []closedLoopSeed{
		{
			ID: "case_001", File: "discovery/request_001.json", Name: "request_001",
			Title: "Bipolar precision level transfer", Description: "Translate a bounded bipolar input into a quiet positive output with controlled gain and dynamic response.",
			Role: RoleDiscovery, Domain: capabilityevaluation.DomainAnalog, Safety: capabilityevaluation.SafetyReviewRequired,
			SupplyMin: 4.75, SupplyNom: 5, SupplyMax: 5.25, SupplyCurrent: 0.05,
			Signals: []ots.Port{signal("input", "analog_voltage", "sink", -1.2, 0, 1.2, 0.001, ""), signal("output", "analog_voltage", "source", 0, 2.5, 5, 0.01, "")},
			Cases: []ots.OperatingCase{
				operating("range", condition("input_voltage", "input", -1, 1, "V"), condition("ambient_temperature", "output", -10, 60, "degC")),
				withEvent(operating("step", condition("load_resistance", "output", 10000, 100000, "ohm")), event("input_step", "input_step", "input", 0.001, -0.5, 0.5, "V")),
			},
			Assertions: []ots.BehavioralAssertion{
				assertion("gain", "voltage_gain", "dc_sweep", "input", "output", between(1.9, 2.1), "ratio", []string{"range"}, true),
				assertion("quiet", "output_noise_rms", "noise", "", "output", upper(0.001), "V_rms", []string{"range"}, false),
				assertion("response", "settling_time", "transient", "input", "output", upper(0.0002), "s", []string{"step"}, false),
				assertion("band", "bandwidth", "ac_sweep", "input", "output", lower(20000), "Hz", []string{"range"}, false),
			},
		},
		{
			ID: "case_002", File: "discovery/request_002.json", Name: "request_002",
			Title: "Low-current measurement transfer", Description: "Convert a small bidirectional input current into a bounded low-noise voltage over the stated signal band.",
			Role: RoleDiscovery, Domain: capabilityevaluation.DomainAnalog, Safety: capabilityevaluation.SafetyNonSafety,
			SupplyMin: 8, SupplyNom: 9, SupplyMax: 10, SupplyCurrent: 0.04,
			Signals: []ots.Port{signal("measured_current", "analog_current", "sink", -0.0002, 0, 0.0002, 0.001, ""), signal("reported_voltage", "analog_voltage", "source", -2.5, 0, 2.5, 0.005, "")},
			Cases: []ots.OperatingCase{
				operating("measurement", condition("input_current", "measured_current", -0.0001, 0.0001, "A"), condition("ambient_temperature", "reported_voltage", 0, 50, "degC")),
				withEvent(operating("change", condition("load_capacitance", "reported_voltage", 0, 1e-9, "F")), event("current_step", "input_step", "measured_current", 0.001, -0.00005, 0.00005, "A")),
			},
			Assertions: []ots.BehavioralAssertion{
				assertion("scale", "transimpedance", "dc_sweep", "measured_current", "reported_voltage", between(9800, 10200), "V/A", []string{"measurement"}, true),
				assertion("noise", "output_noise_rms", "noise", "", "reported_voltage", upper(0.002), "V_rms", []string{"measurement"}, false),
				assertion("speed", "settling_time", "transient", "measured_current", "reported_voltage", upper(0.0005), "s", []string{"change"}, false),
			},
		},
		powerDiscoverySeed("case_003", "discovery/request_003.json", "request_003", capabilityevaluation.SafetyCritical, 18, 24, 30, 2.0, 4.9, 5.1, 1.0),
		{
			ID: "case_004", File: "discovery/request_004.json", Name: "request_004",
			Title: "Bidirectional midpoint supply", Description: "Hold a midpoint voltage while alternately sourcing and sinking bounded load current, including startup and short-duration faults.",
			Role: RoleDiscovery, Domain: capabilityevaluation.DomainPower, Safety: capabilityevaluation.SafetyCritical,
			SupplyMin: 10, SupplyNom: 12, SupplyMax: 14, SupplyCurrent: 0.5,
			Signals: []ots.Port{signal("load_demand", "analog_current", "sink", 0, 0, 0, 0.1, ""), signal("midpoint", "analog_voltage", "source", 0, 6, 12, 0.2, "")},
			Cases: []ots.OperatingCase{
				operating("bidirectional_load", condition("load_current", "midpoint", -0.1, 0.1, "A"), condition("ambient_temperature", "midpoint", 0, 60, "degC")),
				withEvent(operating("fault", condition("supply_voltage", "supply", 10, 14, "V")), event("short", "short_circuit", "midpoint", 0.002, 6, 0, "V")),
			},
			Assertions: []ots.BehavioralAssertion{
				assertion("accuracy", "output_voltage", "dc_operating_point", "", "midpoint", between(5.9, 6.1), "V", []string{"bidirectional_load"}, true),
				assertion("load_change", "load_regulation", "dc_sweep", "", "midpoint", upper(0.1), "V", []string{"bidirectional_load"}, false),
				assertion("fault_current", "peak_current", "transient", "", "midpoint", upper(0.2), "A", []string{"fault"}, true),
				circuitAssertion("thermal_limit", "junction_temperature", "electrothermal", "maximum_junction_temperature", upper(125), "degC", []string{"bidirectional_load", "fault"}, true),
				circuitAssertion("area_margin", "soa_margin", "electrothermal", "minimum_soa_margin", lower(1.3), "ratio", []string{"bidirectional_load", "fault"}, true),
			},
		},
		digitalDiscoverySeed("case_005", "discovery/request_005.json", "request_005"),
		{
			ID: "case_006", File: "discovery/request_006.json", Name: "request_006",
			Title: "Enabled periodic logic output", Description: "Produce a repeatable logic waveform only while enabled, with bounded rate, duty balance, transitions, and startup behavior.",
			Role: RoleDiscovery, Domain: capabilityevaluation.DomainDigital, Safety: capabilityevaluation.SafetyReviewRequired,
			SupplyMin: 3.1, SupplyNom: 3.3, SupplyMax: 3.5, SupplyCurrent: 0.04,
			Signals: []ots.Port{signal("enable", "digital", "sink", 0, 0, 3.5, 0.001, "low"), signal("clock_output", "digital", "source", 0, 1.65, 3.5, 0.005, "")},
			Cases: []ots.OperatingCase{
				withEvent(operating("enabled", condition("load_capacitance", "clock_output", 0, 2e-9, "F")), event("enable_rise", "input_step", "enable", 0.001, 0, 3.3, "V")),
				operating("corners", condition("supply_voltage", "supply", 3.1, 3.5, "V"), condition("ambient_temperature", "clock_output", -20, 70, "degC")),
			},
			Assertions: []ots.BehavioralAssertion{
				assertion("rate", "oscillation_frequency", "transient", "", "clock_output", between(9500, 10500), "Hz", []string{"enabled", "corners"}, true),
				assertion("duty", "duty_cycle", "transient", "", "clock_output", between(45, 55), "%", []string{"enabled", "corners"}, false),
				assertion("rise", "rise_time", "transient", "", "clock_output", upper(2e-6), "s", []string{"enabled"}, false),
				assertion("start", "startup_output_voltage", "startup", "", "clock_output", upper(0.3), "V", []string{"enabled"}, true),
			},
		},
		interfaceDiscoverySeed("case_007", "discovery/request_007.json", "request_007", "Command and response logic interface", "Convey an externally timed command to a response output with bounded thresholds, delay, and edge rates across supply variation."),
		{
			ID: "case_008", File: "discovery/request_008.json", Name: "request_008",
			Title: "Bidirectional two-line logic interface", Description: "Preserve externally driven low states and release both communication lines with bounded leakage and transition time.",
			Role: RoleDiscovery, Domain: capabilityevaluation.DomainMCU, Safety: capabilityevaluation.SafetyReviewRequired,
			SupplyMin: 2.9, SupplyNom: 3.3, SupplyMax: 3.6, SupplyCurrent: 0.03,
			Signals: []ots.Port{signal("interface_enable", "digital", "sink", 0, 3.3, 3.6, 0.001, "high"), signal("data_line", "digital", "bidirectional", 0, 3.3, 3.6, 0.01, "high"), signal("clock_line", "digital", "bidirectional", 0, 3.3, 3.6, 0.01, "high"), signal("activity_status", "digital", "source", 0, 0, 3.6, 0.005, "")},
			Cases: []ots.OperatingCase{
				withEvent(operating("data_release", condition("load_capacitance", "data_line", 1e-11, 4e-10, "F")), event("release_data", "input_step", "data_line", 0.001, 0, 3.3, "V")),
				withEvent(operating("clock_release", condition("load_capacitance", "clock_line", 1e-11, 4e-10, "F")), event("release_clock", "input_step", "clock_line", 0.001, 0, 3.3, "V")),
			},
			Assertions: []ots.BehavioralAssertion{
				assertion("data_low", "output_low_voltage", "dc_operating_point", "", "data_line", upper(0.4), "V", []string{"data_release"}, true),
				assertion("clock_low", "output_low_voltage", "dc_operating_point", "", "clock_line", upper(0.4), "V", []string{"clock_release"}, true),
				assertion("data_release_time", "rise_time", "transient", "data_line", "data_line", upper(2e-6), "s", []string{"data_release"}, false),
				assertion("released_leakage", "off_state_current", "dc_operating_point", "", "data_line", upper(1e-5), "A", []string{"data_release"}, false),
			},
		},
		sensorDiscoverySeed("case_009", "discovery/request_009.json", "request_009"),
		{
			ID: "case_010", File: "discovery/request_010.json", Name: "request_010",
			Title: "Differential low-level measurement", Description: "Report the difference between two low-level inputs while preserving bipolar response, low noise, and a bounded settling time.",
			Role: RoleDiscovery, Domain: capabilityevaluation.DomainSensor, Safety: capabilityevaluation.SafetyRelevant,
			SupplyMin: 4.75, SupplyNom: 5, SupplyMax: 5.25, SupplyCurrent: 0.04,
			Signals: []ots.Port{signal("positive_input", "analog_voltage", "sink", -0.05, 0, 0.05, 0.001, ""), signal("negative_input", "analog_voltage", "sink", -0.05, 0, 0.05, 0.001, ""), signal("measurement_output", "analog_voltage", "source", 0, 2.5, 5, 0.005, "")},
			Cases: []ots.OperatingCase{
				operating("positive_difference", condition("input_voltage", "positive_input", 0.005, 0.04, "V"), condition("input_voltage", "negative_input", -0.04, -0.005, "V")),
				withEvent(operating("reversal", condition("ambient_temperature", "measurement_output", -20, 70, "degC")), event("polarity_change", "input_step", "positive_input", 0.001, -0.02, 0.02, "V")),
			},
			Assertions: []ots.BehavioralAssertion{
				assertion("difference_gain", "voltage_gain", "dc_sweep", "positive_input", "measurement_output", between(45, 55), "ratio", []string{"positive_difference"}, true),
				assertion("noise", "output_noise_rms", "noise", "", "measurement_output", upper(0.001), "V_rms", []string{"positive_difference"}, false),
				assertion("recovery", "settling_time", "transient", "positive_input", "measurement_output", upper(0.001), "s", []string{"reversal"}, true),
			},
		},
		mixedDiscoverySeed("case_011", "discovery/request_011.json", "request_011"),
		{
			ID: "case_012", File: "discovery/request_012.json", Name: "request_012",
			Title: "Analog-permission power output", Description: "Enable a bounded load current only when an analog request is within the permitted range, with safe startup and fault behavior.",
			Role: RoleDiscovery, Domain: capabilityevaluation.DomainMixedSignal, Safety: capabilityevaluation.SafetyCritical,
			SupplyMin: 9, SupplyNom: 12, SupplyMax: 15, SupplyCurrent: 1.0,
			Signals: []ots.Port{signal("request", "analog_voltage", "sink", 0, 2.5, 5, 0.001, ""), signal("permission", "digital", "sink", 0, 0, 5, 0.001, "low"), signal("load_output", "controlled_current", "source", 0, 0.4, 0, 0.5, "")},
			Cases: []ots.OperatingCase{
				withEvent(operating("permission_change", condition("input_voltage", "request", 1, 4, "V"), condition("load_resistance", "load_output", 10, 30, "ohm")), event("permit", "input_step", "permission", 0.001, 0, 5, "V")),
				withEvent(operating("output_fault", condition("ambient_temperature", "load_output", 0, 60, "degC")), event("short", "short_circuit", "load_output", 0.003, 4, 0, "V")),
			},
			Assertions: []ots.BehavioralAssertion{
				assertion("requested_current", "output_current", "dc_operating_point", "request", "load_output", between(0.35, 0.45), "A", []string{"permission_change"}, true),
				assertion("permission_delay", "propagation_delay", "transient", "permission", "load_output", upper(0.002), "s", []string{"permission_change"}, false),
				assertion("startup_limit", "startup_current", "startup", "", "load_output", upper(0.05), "A", []string{"permission_change"}, true),
				assertion("fault_peak", "peak_current", "transient", "", "load_output", upper(0.6), "A", []string{"output_fault"}, true),
				circuitAssertion("thermal_limit", "junction_temperature", "electrothermal", "maximum_junction_temperature", upper(120), "degC", []string{"permission_change", "output_fault"}, true),
			},
		},
	}
}

func closedLoopHeldOutSeeds() []closedLoopSeed {
	return []closedLoopSeed{
		heldAnalogBandSeed("case_013", "held_out/request_013.json", "request_013"),
		{
			ID: "case_014", File: "held_out/request_014.json", Name: "request_014",
			Title: "Bipolar current replication", Description: "Produce a bounded output current proportional to a bidirectional command current with controlled error, noise, and recovery time.",
			Role: RoleHeldOut, Domain: capabilityevaluation.DomainAnalog, Safety: capabilityevaluation.SafetyReviewRequired,
			SupplyMin: 10, SupplyNom: 12, SupplyMax: 14, SupplyCurrent: 0.2,
			Signals: []ots.Port{signal("command_current", "analog_current", "sink", -0.01, 0, 0.01, 0.02, ""), signal("replicated_current", "controlled_current", "source", -0.05, 0, 0.05, 0.06, "")},
			Cases: []ots.OperatingCase{
				operating("bipolar_range", condition("input_current", "command_current", -0.008, 0.008, "A"), condition("load_resistance", "replicated_current", 20, 200, "ohm")),
				withEvent(operating("recovery", condition("ambient_temperature", "replicated_current", -10, 60, "degC")), event("command_reversal", "input_step", "command_current", 0.001, -0.005, 0.005, "A")),
			},
			Assertions: []ots.BehavioralAssertion{
				assertion("scale", "output_current", "dc_sweep", "command_current", "replicated_current", between(0.024, 0.026), "A", []string{"bipolar_range"}, true),
				assertion("noise", "output_noise_rms", "noise", "", "replicated_current", upper(0.0002), "A", []string{"bipolar_range"}, false),
				assertion("recovery_time", "settling_time", "transient", "command_current", "replicated_current", upper(0.0005), "s", []string{"recovery"}, false),
			},
		},
		heldPowerVoltageSeed("case_015", "held_out/request_015.json", "request_015"),
		{
			ID: "case_016", File: "held_out/request_016.json", Name: "request_016",
			Title: "Enabled constant load current", Description: "Deliver a stable load current after enable across line and load variation while bounding startup, temperature, and fault current.",
			Role: RoleHeldOut, Domain: capabilityevaluation.DomainPower, Safety: capabilityevaluation.SafetyCritical,
			SupplyMin: 7, SupplyNom: 9, SupplyMax: 12, SupplyCurrent: 0.7,
			Signals: []ots.Port{signal("enable", "digital", "sink", 0, 0, 5, 0.001, "low"), signal("load_current", "controlled_current", "source", 0, 0.35, 0, 0.5, "")},
			Cases: []ots.OperatingCase{
				withEvent(operating("enabled_load", condition("load_resistance", "load_current", 5, 20, "ohm"), condition("supply_voltage", "supply", 7, 12, "V")), event("enable_rise", "input_step", "enable", 0.001, 0, 5, "V")),
				withEvent(operating("shorted_load", condition("ambient_temperature", "load_current", 0, 55, "degC")), event("short", "short_circuit", "load_current", 0.004, 3.5, 0, "V")),
			},
			Assertions: []ots.BehavioralAssertion{
				assertion("current", "output_current", "dc_operating_point", "", "load_current", between(0.34, 0.36), "A", []string{"enabled_load"}, true),
				assertion("load_stability", "load_regulation", "dc_sweep", "", "load_current", upper(0.01), "A", []string{"enabled_load"}, false),
				assertion("startup", "startup_current", "startup", "enable", "load_current", upper(0.4), "A", []string{"enabled_load"}, true),
				assertion("fault_peak", "peak_current", "transient", "", "load_current", upper(0.55), "A", []string{"shorted_load"}, true),
				circuitAssertion("thermal", "junction_temperature", "electrothermal", "maximum_junction_temperature", upper(115), "degC", []string{"enabled_load", "shorted_load"}, true),
			},
		},
		heldDigitalWindowSeed("case_017", "held_out/request_017.json", "request_017"),
		{
			ID: "case_018", File: "held_out/request_018.json", Name: "request_018",
			Title: "Inhibitable pulse restoration", Description: "Restore externally timed pulses when permitted, hold a safe inactive level otherwise, and bound delay and transition time.",
			Role: RoleHeldOut, Domain: capabilityevaluation.DomainDigital, Safety: capabilityevaluation.SafetyRelevant,
			SupplyMin: 4.5, SupplyNom: 5, SupplyMax: 5.5, SupplyCurrent: 0.05,
			Signals: []ots.Port{signal("pulse_input", "digital", "sink", 0, 2.5, 5.5, 0.001, "low"), signal("inhibit", "digital", "sink", 0, 0, 5.5, 0.001, "low"), signal("pulse_output", "digital", "source", 0, 2.5, 5.5, 0.01, "")},
			Cases: []ots.OperatingCase{
				withEvent(operating("permitted_pulse", condition("load_capacitance", "pulse_output", 0, 5e-10, "F")), event("pulse_rise", "input_step", "pulse_input", 0.001, 0, 5, "V")),
				withEvent(operating("inhibited", condition("supply_voltage", "supply", 4.5, 5.5, "V")), event("inhibit_rise", "input_step", "inhibit", 0.001, 0, 5, "V")),
			},
			Assertions: []ots.BehavioralAssertion{
				assertion("delay", "propagation_delay", "transient", "pulse_input", "pulse_output", upper(2e-7), "s", []string{"permitted_pulse"}, true),
				assertion("rise", "rise_time", "transient", "pulse_input", "pulse_output", upper(1e-7), "s", []string{"permitted_pulse"}, false),
				assertion("high", "output_high_voltage", "dc_operating_point", "", "pulse_output", lower(4.3), "V", []string{"permitted_pulse"}, false),
				assertion("inactive", "output_low_voltage", "dc_operating_point", "inhibit", "pulse_output", upper(0.3), "V", []string{"inhibited"}, true),
			},
		},
		heldInterfaceSeed("case_019", "held_out/request_019.json", "request_019"),
		{
			ID: "case_020", File: "held_out/request_020.json", Name: "request_020",
			Title: "Analog request status interface", Description: "Convert a bounded analog request into two mutually exclusive status outputs with stable decision levels and safe startup.",
			Role: RoleHeldOut, Domain: capabilityevaluation.DomainMCU, Safety: capabilityevaluation.SafetyReviewRequired,
			SupplyMin: 3, SupplyNom: 3.3, SupplyMax: 3.6, SupplyCurrent: 0.03,
			Signals: []ots.Port{signal("request", "analog_voltage", "sink", -0.5, 1.5, 3.8, 0.001, ""), signal("accepted", "digital", "source", 0, 0, 3.6, 0.005, ""), signal("rejected", "digital", "source", 0, 0, 3.6, 0.005, "")},
			Cases: []ots.OperatingCase{
				operating("request_sweep", condition("input_voltage", "request", -0.2, 3.5, "V"), condition("ambient_temperature", "accepted", -20, 70, "degC")),
				withEvent(operating("startup", condition("supply_voltage", "supply", 0, 3.3, "V")), event("apply_power", "startup", "power", 0.001, 0, 3.3, "V")),
			},
			Assertions: []ots.BehavioralAssertion{
				assertion("accept_level", "rising_threshold", "dc_sweep", "request", "accepted", between(1.9, 2.1), "V", []string{"request_sweep"}, true),
				assertion("reject_level", "falling_threshold", "dc_sweep", "request", "rejected", between(0.9, 1.1), "V", []string{"request_sweep"}, true),
				assertion("decision_delay", "propagation_delay", "transient", "request", "accepted", upper(5e-5), "s", []string{"request_sweep"}, false),
				assertion("safe_start", "startup_output_voltage", "startup", "", "accepted", upper(0.3), "V", []string{"startup"}, true),
			},
		},
		heldSensorCurrentSeed("case_021", "held_out/request_021.json", "request_021"),
		{
			ID: "case_022", File: "held_out/request_022.json", Name: "request_022",
			Title: "Low-level event detector", Description: "Assert a logic indication when a small measured current crosses a stable boundary, with bounded hysteresis, delay, and noise susceptibility.",
			Role: RoleHeldOut, Domain: capabilityevaluation.DomainSensor, Safety: capabilityevaluation.SafetyRelevant,
			SupplyMin: 2.8, SupplyNom: 3, SupplyMax: 3.2, SupplyCurrent: 0.025,
			Signals: []ots.Port{signal("measured_current", "analog_current", "sink", 0, 0.00005, 0.0001, 0.001, ""), signal("event_output", "digital", "source", 0, 0, 3.2, 0.005, "")},
			Cases: []ots.OperatingCase{
				operating("threshold_sweep", condition("input_current", "measured_current", 0, 0.0001, "A"), condition("ambient_temperature", "event_output", -10, 60, "degC")),
				withEvent(operating("event_step", condition("load_capacitance", "event_output", 0, 1e-9, "F")), event("current_rise", "input_step", "measured_current", 0.001, 0.00002, 0.00008, "A")),
			},
			Assertions: []ots.BehavioralAssertion{
				assertion("trip", "threshold_current", "dc_sweep", "measured_current", "event_output", between(0.000045, 0.000055), "A", []string{"threshold_sweep"}, true),
				assertion("memory", "hysteresis", "dc_sweep", "measured_current", "event_output", between(0.000005, 0.000015), "A", []string{"threshold_sweep"}, false),
				assertion("delay", "propagation_delay", "transient", "measured_current", "event_output", upper(0.0002), "s", []string{"event_step"}, true),
				assertion("noise", "output_noise_rms", "noise", "", "event_output", upper(0.01), "V_rms", []string{"threshold_sweep"}, false),
			},
		},
		heldMixedPulseSeed("case_023", "held_out/request_023.json", "request_023"),
		{
			ID: "case_024", File: "held_out/request_024.json", Name: "request_024",
			Title: "Measured-demand power control", Description: "Deliver load current proportional to a measured analog demand and assert a fault status when safe delivery cannot be maintained.",
			Role: RoleHeldOut, Domain: capabilityevaluation.DomainMixedSignal, Safety: capabilityevaluation.SafetyCritical,
			SupplyMin: 14, SupplyNom: 18, SupplyMax: 22, SupplyCurrent: 1.5,
			Signals: []ots.Port{signal("measured_demand", "analog_voltage", "sink", -1, 1, 3, 0.001, ""), signal("load_current", "controlled_current", "source", 0, 0.5, 0, 1.0, ""), signal("fault_status", "digital", "source", 0, 0, 5, 0.005, "")},
			Cases: []ots.OperatingCase{
				operating("demand_range", condition("input_voltage", "measured_demand", -0.5, 2.5, "V"), condition("load_resistance", "load_current", 8, 30, "ohm")),
				withEvent(operating("load_fault", condition("ambient_temperature", "load_current", 0, 65, "degC")), event("short", "short_circuit", "load_current", 0.003, 5, 0, "V")),
			},
			Assertions: []ots.BehavioralAssertion{
				assertion("demand_scale", "transconductance", "dc_sweep", "measured_demand", "load_current", between(0.24, 0.26), "A/V", []string{"demand_range"}, true),
				assertion("fault_threshold", "threshold_current", "transient", "", "fault_status", between(0.8, 1.0), "A", []string{"load_fault"}, true),
				assertion("fault_delay", "propagation_delay", "transient", "load_current", "fault_status", upper(0.001), "s", []string{"load_fault"}, true),
				circuitAssertion("thermal", "junction_temperature", "electrothermal", "maximum_junction_temperature", upper(120), "degC", []string{"demand_range", "load_fault"}, true),
				circuitAssertion("area_margin", "soa_margin", "electrothermal", "minimum_soa_margin", lower(1.4), "ratio", []string{"demand_range", "load_fault"}, true),
			},
		},
	}
}

type assertionBounds struct{ min, max *float64 }

func between(minimum, maximum float64) assertionBounds {
	return assertionBounds{min: number(minimum), max: number(maximum)}
}
func lower(minimum float64) assertionBounds { return assertionBounds{min: number(minimum)} }
func upper(maximum float64) assertionBounds { return assertionBounds{max: number(maximum)} }
func number(value float64) *float64         { return &value }

func signal(id, kind, direction string, minimum, nominal, maximum, current float64, defaultState string) ots.Port {
	electrical := ots.Electrical{MaxCurrentA: number(current), DefaultState: defaultState}
	if kind != "analog_current" && kind != "controlled_current" {
		electrical.MinVoltageV = number(minimum)
		electrical.NominalVoltageV = number(nominal)
		electrical.MaxVoltageV = number(maximum)
	}
	return ots.Port{ID: id, Kind: kind, Direction: direction, Domain: "ground", Electrical: electrical}
}

func condition(axis, target string, minimum, maximum float64, unit string) ots.OperatingCondition {
	return ots.OperatingCondition{Axis: axis, Target: target, Min: minimum, Max: maximum, Unit: unit}
}

func operating(id string, conditions ...ots.OperatingCondition) ots.OperatingCase {
	return ots.OperatingCase{ID: id, Conditions: conditions}
}

func event(id, kind, target string, trigger, initial, applied float64, unit string) ots.OperatingEvent {
	return ots.OperatingEvent{ID: id, Kind: kind, Target: target, TriggerTimeS: trigger, Initial: initial, Applied: applied, Unit: unit}
}

func withEvent(current ots.OperatingCase, events ...ots.OperatingEvent) ots.OperatingCase {
	current.Events = events
	return current
}

func assertion(id, metric, analysis, excitation, observation string, bounds assertionBounds, unit string, cases []string, critical bool) ots.BehavioralAssertion {
	result := ots.BehavioralAssertion{
		ID: id, Metric: metric, Analysis: analysis, Observation: ots.Observation{Kind: "port", ID: observation},
		Min: bounds.min, Max: bounds.max, Unit: unit, OperatingCases: cases, Critical: critical,
	}
	if excitation != "" {
		result.Excitation = &ots.Observation{Kind: "port", ID: excitation}
	}
	return result
}

func circuitAssertion(id, metric, analysis, observation string, bounds assertionBounds, unit string, cases []string, critical bool) ots.BehavioralAssertion {
	result := assertion(id, metric, analysis, "", observation, bounds, unit, cases, critical)
	result.Observation.Kind = "circuit"
	return result
}

func closedLoopRequirement(seed closedLoopSeed) ots.Requirement {
	zero := 0.0
	ports := []ots.Port{
		{ID: "ground", Kind: "reference", Direction: "bidirectional", Domain: "ground"},
		{ID: "power", Kind: "power", Direction: "sink", Domain: "supply"},
	}
	ports = append(ports, seed.Signals...)
	return ots.Normalize(ots.Requirement{
		Schema: ots.RequirementSchema, Version: ots.RequirementVersion,
		Project: ots.Project{Name: seed.Name, Title: seed.Title, Description: seed.Description},
		Requirements: ots.Requirements{
			Domains: []ots.Domain{
				{ID: "ground", Kind: "reference", NominalVoltageV: &zero, Source: "external"},
				{ID: "supply", Kind: "supply", MinVoltageV: number(seed.SupplyMin), NominalVoltageV: number(seed.SupplyNom), MaxVoltageV: number(seed.SupplyMax), MaxCurrentA: number(seed.SupplyCurrent), Source: "external"},
			},
			Ports: ports, OperatingCases: seed.Cases, BehavioralRequirements: seed.Assertions,
			Constraints: ots.BoardLimits{MaxComponents: 32, MaxWidthMM: 80, MaxHeightMM: 60},
		},
		Acceptance: ots.Acceptance{
			RequirePrimitiveOnly: true, RequireTopologySearch: true, RequireSimulation: true, RequireAllCorners: true,
			RequireModelProvenance: true, RequireClosedLoopEvidence: true, RequireCompleteRouting: true, RequireConnectivity: true,
			RequireWriterCorrectness: true, RequireRoundTripZeroDiff: true, RequireERC: true, RequireStrictDRC: true,
			RequireDeterministicReplay: true, RequireFailClosed: true,
		},
	})
}
