package capabilityfeedback

import (
	"kicadai/internal/capabilityevaluation"
	ots "kicadai/internal/opentopologysynthesis"
)

func powerDiscoverySeed(id, file, name string, safety capabilityevaluation.SafetyImpact, supplyMin, supplyNom, supplyMax, supplyCurrent, outputMin, outputMax, outputCurrent float64) closedLoopSeed {
	return closedLoopSeed{
		ID: id, File: file, Name: name,
		Title: "Efficient low-voltage power delivery", Description: "Maintain a low-voltage output across line and load variation with bounded ripple, startup current, temperature, and fault stress.",
		Role: RoleDiscovery, Domain: capabilityevaluation.DomainPower, Safety: safety,
		SupplyMin: supplyMin, SupplyNom: supplyNom, SupplyMax: supplyMax, SupplyCurrent: supplyCurrent,
		Signals: []ots.Port{signal("regulated_output", "power", "source", 0, 5, outputMax+0.5, outputCurrent, ""), signal("enable", "digital", "sink", 0, 0, 5, 0.001, "low")},
		Cases: []ots.OperatingCase{
			operating("line_load", condition("supply_voltage", "supply", supplyMin, supplyMax, "V"), condition("load_current", "regulated_output", 0.1, outputCurrent, "A"), condition("ambient_temperature", "regulated_output", 0, 55, "degC")),
			withEvent(operating("startup", condition("load_resistance", "regulated_output", 5, 50, "ohm")), event("enable_rise", "input_step", "enable", 0.001, 0, 5, "V")),
			withEvent(operating("fault", condition("supply_voltage", "supply", supplyNom, supplyNom, "V")), event("output_short", "short_circuit", "regulated_output", 0.004, 5, 0, "V")),
		},
		Assertions: []ots.BehavioralAssertion{
			assertion("voltage", "output_voltage", "dc_operating_point", "", "regulated_output", between(outputMin, outputMax), "V", []string{"line_load"}, true),
			assertion("efficiency", "conversion_efficiency", "transient", "", "regulated_output", lower(82), "%", []string{"line_load"}, false),
			assertion("ripple", "output_ripple", "transient", "", "regulated_output", upper(0.05), "V", []string{"line_load"}, false),
			assertion("startup_current", "startup_current", "startup", "", "power", upper(supplyCurrent), "A", []string{"startup"}, true),
			assertion("fault_peak", "peak_current", "transient", "", "regulated_output", upper(outputCurrent*1.5), "A", []string{"fault"}, true),
			circuitAssertion("thermal", "junction_temperature", "electrothermal", "maximum_junction_temperature", upper(120), "degC", []string{"line_load", "fault"}, true),
			circuitAssertion("area_margin", "soa_margin", "electrothermal", "minimum_soa_margin", lower(1.35), "ratio", []string{"line_load", "fault"}, true),
		},
	}
}

func digitalDiscoverySeed(id, file, name string) closedLoopSeed {
	return closedLoopSeed{
		ID: id, File: file, Name: name,
		Title: "Dual-condition permission logic", Description: "Assert a permission output only while both external conditions are valid, with defined inactive startup and bounded decision delay.",
		Role: RoleDiscovery, Domain: capabilityevaluation.DomainDigital, Safety: capabilityevaluation.SafetyRelevant,
		SupplyMin: 4.5, SupplyNom: 5, SupplyMax: 5.5, SupplyCurrent: 0.03,
		Signals: []ots.Port{
			signal("condition_a", "digital", "sink", 0, 0, 5.5, 0.001, "low"), signal("condition_b", "digital", "sink", 0, 0, 5.5, 0.001, "low"),
			signal("permission", "digital", "source", 0, 0, 5.5, 0.01, ""),
		},
		Cases: []ots.OperatingCase{
			withEvent(operating("both_valid", condition("supply_voltage", "supply", 4.5, 5.5, "V")), event("a_valid", "input_step", "condition_a", 0.001, 0, 5, "V"), event("b_valid", "input_step", "condition_b", 0.002, 0, 5, "V")),
			withEvent(operating("condition_lost", condition("ambient_temperature", "permission", -20, 70, "degC")), event("a_lost", "input_step", "condition_a", 0.003, 5, 0, "V")),
		},
		Assertions: []ots.BehavioralAssertion{
			assertion("active_high", "output_high_voltage", "dc_operating_point", "", "permission", lower(4.3), "V", []string{"both_valid"}, true),
			assertion("inactive_low", "output_low_voltage", "dc_operating_point", "", "permission", upper(0.3), "V", []string{"condition_lost"}, true),
			assertion("decision_delay", "propagation_delay", "transient", "condition_a", "permission", upper(2e-5), "s", []string{"both_valid", "condition_lost"}, true),
			assertion("startup_state", "startup_output_voltage", "startup", "", "permission", upper(0.3), "V", []string{"both_valid"}, true),
		},
	}
}

func interfaceDiscoverySeed(id, file, name, title, description string) closedLoopSeed {
	return closedLoopSeed{
		ID: id, File: file, Name: name, Title: title, Description: description,
		Role: RoleDiscovery, Domain: capabilityevaluation.DomainMCU, Safety: capabilityevaluation.SafetyNonSafety,
		SupplyMin: 2.7, SupplyNom: 3.3, SupplyMax: 3.6, SupplyCurrent: 0.03,
		Signals: []ots.Port{signal("command_input", "digital", "sink", 0, 1.65, 3.6, 0.001, "low"), signal("response_output", "digital", "source", 0, 1.65, 3.6, 0.008, "")},
		Cases: []ots.OperatingCase{
			withEvent(operating("rising_command", condition("load_capacitance", "response_output", 0, 1e-9, "F")), event("command_rise", "input_step", "command_input", 0.001, 0, 3.3, "V")),
			withEvent(operating("falling_command", condition("supply_voltage", "supply", 2.7, 3.6, "V")), event("command_fall", "input_step", "command_input", 0.001, 3.3, 0, "V")),
		},
		Assertions: []ots.BehavioralAssertion{
			assertion("high_level", "output_high_voltage", "dc_operating_point", "", "response_output", lower(2.7), "V", []string{"rising_command"}, false),
			assertion("low_level", "output_low_voltage", "dc_operating_point", "", "response_output", upper(0.3), "V", []string{"falling_command"}, false),
			assertion("delay", "propagation_delay", "transient", "command_input", "response_output", upper(1e-6), "s", []string{"rising_command", "falling_command"}, true),
			assertion("rise", "rise_time", "transient", "command_input", "response_output", upper(5e-7), "s", []string{"rising_command"}, false),
			assertion("fall", "fall_time", "transient", "command_input", "response_output", upper(5e-7), "s", []string{"falling_command"}, false),
		},
	}
}

func sensorDiscoverySeed(id, file, name string) closedLoopSeed {
	return closedLoopSeed{
		ID: id, File: file, Name: name,
		Title: "Low-current sensor reporting", Description: "Translate a unipolar measured current into a stable voltage with bounded scale error, noise, bandwidth, and startup output.",
		Role: RoleDiscovery, Domain: capabilityevaluation.DomainSensor, Safety: capabilityevaluation.SafetyReviewRequired,
		SupplyMin: 3, SupplyNom: 3.3, SupplyMax: 3.6, SupplyCurrent: 0.03,
		Signals: []ots.Port{signal("sensor_current", "analog_current", "sink", 0, 0.00005, 0.0001, 0.001, ""), signal("sensor_report", "analog_voltage", "source", 0, 1.5, 3.3, 0.005, "")},
		Cases: []ots.OperatingCase{
			operating("measurement", condition("input_current", "sensor_current", 0.00001, 0.00009, "A"), condition("ambient_temperature", "sensor_report", -20, 70, "degC")),
			withEvent(operating("startup", condition("supply_voltage", "supply", 0, 3.3, "V")), event("apply_power", "startup", "power", 0.001, 0, 3.3, "V")),
		},
		Assertions: []ots.BehavioralAssertion{
			assertion("scale", "transimpedance", "dc_sweep", "sensor_current", "sensor_report", between(29000, 31000), "V/A", []string{"measurement"}, true),
			assertion("noise", "output_noise_rms", "noise", "", "sensor_report", upper(0.001), "V_rms", []string{"measurement"}, false),
			assertion("band", "bandwidth", "ac_sweep", "sensor_current", "sensor_report", lower(5000), "Hz", []string{"measurement"}, false),
			assertion("safe_start", "startup_output_voltage", "startup", "", "sensor_report", upper(0.2), "V", []string{"startup"}, true),
		},
	}
}

func mixedDiscoverySeed(id, file, name string) closedLoopSeed {
	return closedLoopSeed{
		ID: id, File: file, Name: name,
		Title: "Pulse-commanded average current", Description: "Deliver average load current according to an external pulse command while bounding ripple, enable recovery, temperature, and safe operating margin.",
		Role: RoleDiscovery, Domain: capabilityevaluation.DomainMixedSignal, Safety: capabilityevaluation.SafetyCritical,
		SupplyMin: 10, SupplyNom: 12, SupplyMax: 14, SupplyCurrent: 1.2,
		Signals: []ots.Port{signal("pulse_command", "digital", "sink", 0, 2.5, 5, 0.001, "low"), signal("enable", "digital", "sink", 0, 0, 5, 0.001, "low"), signal("average_current", "controlled_current", "source", 0, 0.4, 0, 0.8, "")},
		Cases: []ots.OperatingCase{
			withEvent(operating("commanded", condition("load_resistance", "average_current", 8, 25, "ohm"), condition("ambient_temperature", "average_current", 0, 60, "degC")), event("enable_rise", "input_step", "enable", 0.001, 0, 5, "V"), event("command_rise", "input_step", "pulse_command", 0.002, 0, 5, "V")),
			withEvent(operating("load_change", condition("supply_voltage", "supply", 10, 14, "V")), event("load_step", "load_step", "average_current", 0.003, 20, 10, "ohm")),
		},
		Assertions: []ots.BehavioralAssertion{
			assertion("average", "output_current", "transient", "pulse_command", "average_current", between(0.35, 0.45), "A", []string{"commanded"}, true),
			assertion("ripple", "output_ripple", "transient", "", "average_current", upper(0.03), "A", []string{"commanded"}, false),
			assertion("recovery", "settling_time", "transient", "", "average_current", upper(0.003), "s", []string{"load_change"}, false),
			circuitAssertion("thermal", "junction_temperature", "electrothermal", "maximum_junction_temperature", upper(115), "degC", []string{"commanded", "load_change"}, true),
			circuitAssertion("area_margin", "soa_margin", "electrothermal", "minimum_soa_margin", lower(1.3), "ratio", []string{"commanded", "load_change"}, true),
		},
	}
}

func heldAnalogBandSeed(id, file, name string) closedLoopSeed {
	frequency := 1000.0
	assertionAtFrequency := assertion("midband_gain", "voltage_gain_at_frequency", "ac_sweep", "signal_input", "signal_output", between(3.8, 4.2), "ratio", []string{"band"}, false)
	assertionAtFrequency.FrequencyHz = &frequency
	return closedLoopSeed{
		ID: id, File: file, Name: name,
		Title: "Bipolar band-limited signal transfer", Description: "Apply bounded gain to a bipolar signal in the desired band while limiting low-frequency response, distortion, and output noise.",
		Role: RoleHeldOut, Domain: capabilityevaluation.DomainAnalog, Safety: capabilityevaluation.SafetyNonSafety,
		SupplyMin: 8, SupplyNom: 9, SupplyMax: 10, SupplyCurrent: 0.05,
		Signals: []ots.Port{signal("signal_input", "analog_voltage", "sink", -2, 0, 2, 0.001, ""), signal("signal_output", "analog_voltage", "source", -4, 0, 4, 0.01, "")},
		Cases: []ots.OperatingCase{
			operating("band", condition("input_voltage", "signal_input", -0.5, 0.5, "V"), condition("load_resistance", "signal_output", 10000, 100000, "ohm")),
			withEvent(operating("step", condition("ambient_temperature", "signal_output", 0, 50, "degC")), event("input_change", "input_step", "signal_input", 0.001, -0.2, 0.2, "V")),
		},
		Assertions: []ots.BehavioralAssertion{
			assertionAtFrequency,
			assertion("cutoff", "cutoff_frequency", "ac_sweep", "signal_input", "signal_output", between(80, 120), "Hz", []string{"band"}, false),
			assertion("distortion", "total_harmonic_distortion", "distortion", "signal_input", "signal_output", upper(0.2), "%", []string{"band"}, false),
			assertion("settling", "settling_time", "transient", "signal_input", "signal_output", upper(0.001), "s", []string{"step"}, false),
		},
	}
}

func heldPowerVoltageSeed(id, file, name string) closedLoopSeed {
	return closedLoopSeed{
		ID: id, File: file, Name: name,
		Title: "Enabled medium-voltage power delivery", Description: "Provide a stable enabled output across line and load variation with bounded ripple, startup overshoot, efficiency, and short-circuit stress.",
		Role: RoleHeldOut, Domain: capabilityevaluation.DomainPower, Safety: capabilityevaluation.SafetyCritical,
		SupplyMin: 18, SupplyNom: 22, SupplyMax: 26, SupplyCurrent: 2,
		Signals: []ots.Port{signal("enable", "digital", "sink", 0, 0, 5, 0.001, "low"), signal("power_output", "power", "source", 0, 12, 13, 1.2, "")},
		Cases: []ots.OperatingCase{
			withEvent(operating("enabled_load", condition("supply_voltage", "supply", 18, 26, "V"), condition("load_current", "power_output", 0.2, 1, "A")), event("enable_rise", "input_step", "enable", 0.001, 0, 5, "V")),
			withEvent(operating("shorted", condition("ambient_temperature", "power_output", 0, 55, "degC")), event("short", "short_circuit", "power_output", 0.005, 12, 0, "V")),
		},
		Assertions: []ots.BehavioralAssertion{
			assertion("voltage", "output_voltage", "dc_operating_point", "", "power_output", between(11.8, 12.2), "V", []string{"enabled_load"}, true),
			assertion("efficiency", "conversion_efficiency", "transient", "", "power_output", lower(85), "%", []string{"enabled_load"}, false),
			assertion("ripple", "output_ripple", "transient", "", "power_output", upper(0.08), "V", []string{"enabled_load"}, false),
			assertion("overshoot", "startup_overshoot", "startup", "enable", "power_output", upper(0.3), "V", []string{"enabled_load"}, true),
			assertion("fault_peak", "peak_current", "transient", "", "power_output", upper(1.6), "A", []string{"shorted"}, true),
			circuitAssertion("thermal", "junction_temperature", "electrothermal", "maximum_junction_temperature", upper(120), "degC", []string{"enabled_load", "shorted"}, true),
		},
	}
}

func heldDigitalWindowSeed(id, file, name string) closedLoopSeed {
	return closedLoopSeed{
		ID: id, File: file, Name: name,
		Title: "Bounded-range status indication", Description: "Assert an external status only while a monitored level lies within a bounded range, with stable boundaries and prompt transitions.",
		Role: RoleHeldOut, Domain: capabilityevaluation.DomainDigital, Safety: capabilityevaluation.SafetyRelevant,
		SupplyMin: 4.75, SupplyNom: 5, SupplyMax: 5.25, SupplyCurrent: 0.03,
		Signals: []ots.Port{signal("monitored_level", "analog_voltage", "sink", -1, 2.5, 6, 0.001, ""), signal("range_status", "digital", "source", 0, 0, 5.25, 0.01, "")},
		Cases: []ots.OperatingCase{
			operating("level_sweep", condition("input_voltage", "monitored_level", 0, 5, "V"), condition("ambient_temperature", "range_status", -20, 70, "degC")),
			withEvent(operating("edge_change", condition("load_capacitance", "range_status", 0, 1e-9, "F")), event("cross_boundary", "input_step", "monitored_level", 0.001, 1, 2.5, "V")),
		},
		Assertions: []ots.BehavioralAssertion{
			assertion("lower", "lower_threshold", "dc_sweep", "monitored_level", "range_status", between(1.4, 1.6), "V", []string{"level_sweep"}, true),
			assertion("upper", "upper_threshold", "dc_sweep", "monitored_level", "range_status", between(3.4, 3.6), "V", []string{"level_sweep"}, true),
			assertion("memory", "hysteresis", "dc_sweep", "monitored_level", "range_status", between(0.05, 0.15), "V", []string{"level_sweep"}, false),
			assertion("delay", "propagation_delay", "transient", "monitored_level", "range_status", upper(5e-5), "s", []string{"edge_change"}, true),
		},
	}
}

func heldInterfaceSeed(id, file, name string) closedLoopSeed {
	return closedLoopSeed{
		ID: id, File: file, Name: name,
		Title: "Cross-domain logic conveyance", Description: "Convey two external logic inputs into a lower-voltage domain with bounded thresholds, delay, output levels, and safe startup.",
		Role: RoleHeldOut, Domain: capabilityevaluation.DomainMCU, Safety: capabilityevaluation.SafetyRelevant,
		SupplyMin: 3, SupplyNom: 3.3, SupplyMax: 3.6, SupplyCurrent: 0.04,
		Signals: []ots.Port{
			signal("input_a", "digital", "sink", 0, 2.5, 5.5, 0.001, "low"), signal("input_b", "digital", "sink", 0, 2.5, 5.5, 0.001, "low"),
			signal("output_a", "digital", "source", 0, 1.65, 3.6, 0.006, ""), signal("output_b", "digital", "source", 0, 1.65, 3.6, 0.006, ""),
		},
		Cases: []ots.OperatingCase{
			withEvent(operating("channel_a", condition("load_capacitance", "output_a", 0, 8e-10, "F")), event("input_a_rise", "input_step", "input_a", 0.001, 0, 5, "V")),
			withEvent(operating("channel_b", condition("supply_voltage", "supply", 3, 3.6, "V")), event("input_b_fall", "input_step", "input_b", 0.001, 5, 0, "V")),
		},
		Assertions: []ots.BehavioralAssertion{
			assertion("a_high", "output_high_voltage", "dc_operating_point", "", "output_a", lower(2.7), "V", []string{"channel_a"}, false),
			assertion("b_low", "output_low_voltage", "dc_operating_point", "", "output_b", upper(0.3), "V", []string{"channel_b"}, false),
			assertion("a_delay", "propagation_delay", "transient", "input_a", "output_a", upper(8e-7), "s", []string{"channel_a"}, true),
			assertion("b_delay", "propagation_delay", "transient", "input_b", "output_b", upper(8e-7), "s", []string{"channel_b"}, true),
			assertion("safe_start", "startup_output_voltage", "startup", "", "output_a", upper(0.3), "V", []string{"channel_a"}, true),
		},
	}
}

func heldSensorCurrentSeed(id, file, name string) closedLoopSeed {
	return closedLoopSeed{
		ID: id, File: file, Name: name,
		Title: "Bidirectional current measurement", Description: "Report a small bidirectional measured current as a centered voltage with bounded scale error, noise, bandwidth, and overload recovery.",
		Role: RoleHeldOut, Domain: capabilityevaluation.DomainSensor, Safety: capabilityevaluation.SafetyReviewRequired,
		SupplyMin: 4.5, SupplyNom: 5, SupplyMax: 5.5, SupplyCurrent: 0.04,
		Signals: []ots.Port{signal("measured_current", "analog_current", "sink", -0.02, 0, 0.02, 0.03, ""), signal("measurement_voltage", "analog_voltage", "source", 0, 2.5, 5, 0.005, "")},
		Cases: []ots.OperatingCase{
			operating("measurement", condition("input_current", "measured_current", -0.01, 0.01, "A"), condition("ambient_temperature", "measurement_voltage", -20, 70, "degC")),
			withEvent(operating("overload_recovery", condition("load_resistance", "measurement_voltage", 10000, 100000, "ohm")), event("overload_clear", "input_step", "measured_current", 0.001, 0.018, 0.002, "A")),
		},
		Assertions: []ots.BehavioralAssertion{
			assertion("scale", "transimpedance", "dc_sweep", "measured_current", "measurement_voltage", between(190, 210), "V/A", []string{"measurement"}, true),
			assertion("noise", "output_noise_rms", "noise", "", "measurement_voltage", upper(0.0008), "V_rms", []string{"measurement"}, false),
			assertion("bandwidth", "bandwidth", "ac_sweep", "measured_current", "measurement_voltage", lower(10000), "Hz", []string{"measurement"}, false),
			assertion("recovery", "settling_time", "transient", "measured_current", "measurement_voltage", upper(0.001), "s", []string{"overload_recovery"}, true),
		},
	}
}

func heldMixedPulseSeed(id, file, name string) closedLoopSeed {
	return closedLoopSeed{
		ID: id, File: file, Name: name,
		Title: "Pulse-controlled voltage delivery", Description: "Provide a stable load voltage while an external pulse command is active, with bounded ripple, shutdown delay, startup, and thermal stress.",
		Role: RoleHeldOut, Domain: capabilityevaluation.DomainMixedSignal, Safety: capabilityevaluation.SafetyCritical,
		SupplyMin: 9, SupplyNom: 12, SupplyMax: 15, SupplyCurrent: 1.2,
		Signals: []ots.Port{signal("pulse_enable", "digital", "sink", 0, 0, 5, 0.001, "low"), signal("load_voltage", "power", "source", 0, 6, 7, 0.8, "")},
		Cases: []ots.OperatingCase{
			withEvent(operating("active", condition("load_current", "load_voltage", 0.1, 0.6, "A"), condition("supply_voltage", "supply", 9, 15, "V")), event("pulse_rise", "input_step", "pulse_enable", 0.001, 0, 5, "V")),
			withEvent(operating("shutdown", condition("ambient_temperature", "load_voltage", 0, 60, "degC")), event("pulse_fall", "input_step", "pulse_enable", 0.004, 5, 0, "V")),
		},
		Assertions: []ots.BehavioralAssertion{
			assertion("active_voltage", "output_voltage", "transient", "pulse_enable", "load_voltage", between(5.8, 6.2), "V", []string{"active"}, true),
			assertion("ripple", "output_ripple", "transient", "", "load_voltage", upper(0.08), "V", []string{"active"}, false),
			assertion("shutdown_delay", "propagation_delay", "transient", "pulse_enable", "load_voltage", upper(0.001), "s", []string{"shutdown"}, true),
			assertion("startup_peak", "startup_overshoot", "startup", "pulse_enable", "load_voltage", upper(0.3), "V", []string{"active"}, true),
			circuitAssertion("thermal", "junction_temperature", "electrothermal", "maximum_junction_temperature", upper(115), "degC", []string{"active", "shutdown"}, true),
		},
	}
}
