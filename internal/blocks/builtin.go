package blocks

const (
	defaultConnectorSymbol    = "Connector_Generic:Conn_01x02"
	defaultConnectorFootprint = "Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical"
)

func BuiltinDefinitions() []BlockDefinition {
	return []BlockDefinition{
		ledIndicatorDefinition(),
		voltageRegulatorDefinition(),
		mcuMinimalDefinition(),
		usbCPowerDefinition(),
		i2cSensorDefinition(),
		opampGainStageDefinition(),
		connectorBreakoutDefinition(),
	}
}

func ledIndicatorDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          "led_indicator",
		Name:        "LED Indicator",
		Description: "Series resistor and LED indicator with active-high or active-low polarity.",
		Version:     "0.1.0",
		Category:    "indicator",
		Parameters: []BlockParameter{
			{Name: "supply_voltage", Type: ParameterVoltage, Default: "3.3V", Description: "Rail voltage feeding the indicator."},
			{Name: "led_forward_voltage", Type: ParameterVoltage, Default: "2.0V", Description: "Expected LED forward voltage."},
			{Name: "led_current", Type: ParameterCurrent, Default: "5mA", Description: "Target LED current."},
			{Name: "resistor_value", Type: ParameterResistance, Description: "Optional explicit current-limiting resistor value."},
			{Name: "color", Type: ParameterEnum, Default: "green", Allowed: []any{"red", "green", "blue", "amber", "white"}, Description: "LED color intent."},
			{Name: "active_high", Type: ParameterBool, Default: true, Description: "When true, IN sources current through the LED."},
			{Name: "resistor_footprint", Type: ParameterFootprintID, Default: "Resistor_SMD:R_0805_2012Metric", Description: "Resistor footprint ID."},
			{Name: "led_footprint", Type: ParameterFootprintID, Default: "LED_SMD:LED_0805_2012Metric", Description: "LED footprint ID."},
		},
		Ports: []BlockPort{
			{Name: "IN", Direction: PortInput, Description: "Control signal."},
			{Name: "VCC", Direction: PortPower, Voltage: "supply_voltage", Description: "Optional rail for active-low use."},
			{Name: "GND", Direction: PortPower, Description: "Ground return."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "Current-limiting resistor."},
			{Kind: "symbol", ID: "Device:LED", Required: true, Description: "Indicator LED."},
			{Kind: "footprint", ID: "Resistor_SMD:R_0805_2012Metric", Required: true, Description: "Default resistor footprint."},
			{Kind: "footprint", ID: "LED_SMD:LED_0805_2012Metric", Required: true, Description: "Default LED footprint."},
		},
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Notes: []string{"Emits deterministic schematic transactions; resolver-backed pinmap validation is implemented in later phases."},
		},
	}
}

func voltageRegulatorDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          "voltage_regulator",
		Name:        "Voltage Regulator",
		Description: "Fixed-output linear regulator with input and output capacitors.",
		Version:     "0.1.0",
		Category:    "power",
		Parameters: []BlockParameter{
			{Name: "input_voltage", Type: ParameterVoltage, Default: "5V", Description: "Nominal input rail."},
			{Name: "output_voltage", Type: ParameterVoltage, Default: "3.3V", Description: "Regulated output rail."},
			{Name: "output_current", Type: ParameterCurrent, Default: "250mA", Description: "Expected output current."},
			{Name: "include_power_led", Type: ParameterBool, Default: false, Description: "Include a downstream power indicator."},
		},
		Ports: []BlockPort{
			{Name: "VIN", Direction: PortPower, Voltage: "input_voltage", Description: "Unregulated input."},
			{Name: "VOUT", Direction: PortPower, Voltage: "output_voltage", Description: "Regulated output."},
			{Name: "GND", Direction: PortPower, Description: "Ground return."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Regulator_Linear:AMS1117-3.3", Required: true, Description: "Default regulator candidate."},
			{Kind: "symbol", ID: "Device:C", Required: true, Description: "Input and output capacitors."},
		},
		Verification: experimentalVerification("Initial metadata placeholder; regulator topology checks are implemented in later phases."),
	}
}

func mcuMinimalDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          "mcu_minimal",
		Name:        "MCU Minimal System",
		Description: "Microcontroller power, reset, programming, and decoupling support.",
		Version:     "0.1.0",
		Category:    "digital",
		Parameters: []BlockParameter{
			{Name: "mcu_symbol", Type: ParameterSymbolID, Required: true, Description: "KiCad symbol ID for the MCU."},
			{Name: "mcu_footprint", Type: ParameterFootprintID, Required: true, Description: "KiCad footprint ID for the MCU package."},
			{Name: "supply_voltage", Type: ParameterVoltage, Default: "3.3V", Description: "MCU supply rail."},
			{Name: "programming_header", Type: ParameterEnum, Default: "swd", Allowed: []any{"none", "swd", "isp", "uart"}, Description: "Programming/debug header mode."},
		},
		Ports: []BlockPort{
			{Name: "VCC", Direction: PortPower, Voltage: "supply_voltage", Description: "MCU supply input."},
			{Name: "GND", Direction: PortPower, Description: "Ground return."},
			{Name: "RESET", Direction: PortInput, Description: "Reset signal."},
			{Name: "GPIO", Direction: PortBidirectional, Description: "Application GPIO export group."},
		},
		Verification: experimentalVerification("Initial metadata placeholder; MCU pin-role metadata remains a known gap."),
	}
}

func usbCPowerDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          "usb_c_power",
		Name:        "USB-C Power Input",
		Description: "USB-C sink power input with CC pull-downs and optional protection parts.",
		Version:     "0.1.0",
		Category:    "power",
		Parameters: []BlockParameter{
			{Name: "current_limit", Type: ParameterCurrent, Default: "500mA", Description: "Expected maximum VBUS current."},
			{Name: "include_fuse", Type: ParameterBool, Default: true, Description: "Include a resettable fuse on VBUS."},
			{Name: "include_tvs", Type: ParameterBool, Default: true, Description: "Include VBUS ESD/TVS protection."},
			{Name: "shield_policy", Type: ParameterEnum, Default: "chassis", Allowed: []any{"chassis", "gnd", "floating"}, Description: "Connector shield handling."},
		},
		Ports: []BlockPort{
			{Name: "VBUS_OUT", Direction: PortPower, Voltage: "5V", Description: "Protected 5 V output."},
			{Name: "GND", Direction: PortPower, Description: "Ground return."},
			{Name: "SHIELD", Direction: PortPassive, Description: "Connector shield node."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Connector:USB_C_Receptacle_USB2.0", Required: true, Description: "USB-C sink connector."},
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "CC pull-down resistors."},
		},
		Verification: experimentalVerification("Initial metadata placeholder; USB-C sink validation is implemented in later phases."),
	}
}

func i2cSensorDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          "i2c_sensor",
		Name:        "I2C Sensor",
		Description: "I2C peripheral with optional pull-ups, interrupt, and decoupling.",
		Version:     "0.1.0",
		Category:    "sensor",
		Parameters: []BlockParameter{
			{Name: "supply_voltage", Type: ParameterVoltage, Default: "3.3V", Description: "Sensor supply rail."},
			{Name: "sensor_symbol", Type: ParameterSymbolID, Required: true, Description: "KiCad symbol ID for the sensor."},
			{Name: "sensor_footprint", Type: ParameterFootprintID, Required: true, Description: "KiCad footprint ID for the sensor package."},
			{Name: "i2c_address", Type: ParameterString, Required: true, Description: "7-bit I2C address such as 0x48."},
			{Name: "include_pullups", Type: ParameterBool, Default: true, Description: "Include SDA/SCL pull-up resistors."},
		},
		Ports: []BlockPort{
			{Name: "VCC", Direction: PortPower, Voltage: "supply_voltage", Description: "Sensor supply input."},
			{Name: "GND", Direction: PortPower, Description: "Ground return."},
			{Name: "SDA", Direction: PortBidirectional, Description: "I2C data."},
			{Name: "SCL", Direction: PortInput, Description: "I2C clock."},
			{Name: "INT", Direction: PortOutput, Description: "Optional interrupt output."},
		},
		Verification: experimentalVerification("Initial metadata placeholder; sensor-specific pin roles are supplied by later phases."),
	}
}

func opampGainStageDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          "opamp_gain_stage",
		Name:        "Op-Amp Gain Stage",
		Description: "Parameterized op-amp gain stage with feedback network.",
		Version:     "0.1.0",
		Category:    "analog",
		Parameters: []BlockParameter{
			{Name: "topology", Type: ParameterEnum, Default: "non_inverting", Allowed: []any{"non_inverting", "inverting"}, Description: "Amplifier topology."},
			{Name: "gain", Type: ParameterNumber, Default: 2.0, Description: "Target voltage gain."},
			{Name: "opamp_symbol", Type: ParameterSymbolID, Default: "Amplifier_Operational:LMV321", Description: "KiCad symbol ID for the op-amp."},
			{Name: "single_supply", Type: ParameterBool, Default: true, Description: "Use a single-supply bias network."},
		},
		Ports: []BlockPort{
			{Name: "IN", Direction: PortInput, Description: "Signal input."},
			{Name: "OUT", Direction: PortOutput, Description: "Signal output."},
			{Name: "VCC", Direction: PortPower, Description: "Positive supply."},
			{Name: "GND", Direction: PortPower, Description: "Ground or negative supply."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "Feedback resistors."},
			{Kind: "symbol", ID: "Amplifier_Operational:LMV321", Required: false, Description: "Default op-amp candidate."},
		},
		Verification: experimentalVerification("Initial metadata placeholder; resistor synthesis is implemented in later phases."),
	}
}

func connectorBreakoutDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          "connector_breakout",
		Name:        "Connector Breakout",
		Description: "Generic connector with exported named pins.",
		Version:     "0.1.0",
		Category:    "interconnect",
		Parameters: []BlockParameter{
			{Name: "pin_names", Type: ParameterStringList, Required: true, Description: "Ordered connector pin names."},
			{Name: "pin_count", Type: ParameterNumber, Description: "Expected connector pin count; inferred from pin_names when omitted."},
			{Name: "connector_symbol", Type: ParameterSymbolID, Default: defaultConnectorSymbol, Description: "KiCad connector symbol ID."},
			{Name: "connector_footprint", Type: ParameterFootprintID, Default: defaultConnectorFootprint, Description: "KiCad connector footprint ID."},
			{Name: "include_labels", Type: ParameterBool, Default: true, Description: "Add schematic labels for exported pins."},
			{Name: "include_mounting_holes", Type: ParameterBool, Default: false, Description: "Reserve mounting-hole support for later PCB generation."},
		},
		Ports: []BlockPort{
			{Name: "PINS", Direction: PortPassive, Description: "Dynamic exported pin group; concrete ports are generated from pin_names in later phases."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: defaultConnectorSymbol, Required: false, Description: "Default two-pin connector."},
			{Kind: "footprint", ID: defaultConnectorFootprint, Required: false, Description: "Default two-pin header."},
		},
		Verification: experimentalVerification("Initial metadata placeholder; dynamic port expansion is implemented in later phases."),
	}
}

func experimentalVerification(note string) VerificationRecord {
	return VerificationRecord{
		Level: VerificationExperimental,
		Notes: []string{note},
	}
}
