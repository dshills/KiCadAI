package blocks

const (
	defaultConnectorSymbol    = "Connector_Generic:Conn_01x02"
	defaultConnectorFootprint = "Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical"
	defaultI2CSensorSymbol    = "Sensor:Generic_I2C"
	defaultMCUSymbol          = "MCU_Microchip_ATmega:ATmega328P-A"
	defaultMCUFootprint       = "Package_QFP:TQFP-32_7x7mm_P0.8mm"
	defaultOpAmpSymbol        = "Amplifier_Operational:LMV321"
	defaultUSBCSymbol         = "Connector:USB_C_Receptacle_USB2.0_16P"
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
		Components:     ledIndicatorComponents(),
		PCBRealization: ledIndicatorPCBRealization(),
		ValidationRules: []BlockValidationRule{
			{ID: "led.current.positive", Severity: BlockValidationSeverityBlocked, Description: "LED current must be a positive current literal."},
			{ID: "led.forward_voltage.below_supply", Severity: BlockValidationSeverityBlocked, Description: "LED forward voltage must be below the selected supply rail."},
			{ID: "led.resistor.required", Severity: BlockValidationSeverityBlocked, Description: "A positive current-limiting resistor value must be provided or derived."},
			{ID: "led.polarity.evidence", Severity: BlockValidationSeverityBlocked, Description: "LED anode/cathode evidence is required for connectivity-level use."},
			{ID: "led.series_route.required", Severity: BlockValidationSeverityBlocked, Description: "PCB realization must include the local resistor-to-LED route."},
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
			{Name: "input_voltage_min", Type: ParameterVoltage, Default: "4.5V", Description: "Minimum expected input rail."},
			{Name: "input_voltage_max", Type: ParameterVoltage, Default: "12V", Description: "Maximum expected input rail."},
			{Name: "input_voltage", Type: ParameterVoltage, Default: "5V", Description: "Nominal input rail."},
			{Name: "output_voltage", Type: ParameterVoltage, Default: "3.3V", Description: "Regulated output rail."},
			{Name: "output_current", Type: ParameterCurrent, Default: "250mA", Description: "Expected output current."},
			{Name: "regulator_symbol", Type: ParameterSymbolID, Default: defaultRegulatorSymbol, Description: "KiCad symbol ID for the supported fixed three-pin regulator template."},
			{Name: "regulator_footprint", Type: ParameterFootprintID, Default: "Package_TO_SOT_SMD:SOT-223-3_TabPin2", Description: "KiCad footprint ID for the regulator."},
			{Name: "input_capacitance", Type: ParameterCapacitance, Default: "10uF", Description: "Input bypass capacitor value."},
			{Name: "output_capacitance", Type: ParameterCapacitance, Default: "10uF", Description: "Output bypass capacitor value."},
			{Name: "capacitor_footprint", Type: ParameterFootprintID, Default: "Capacitor_SMD:C_0805_2012Metric", Description: "Default capacitor footprint ID."},
			{Name: "enable_mode", Type: ParameterEnum, Default: "none", Allowed: []any{"none", "tied_input", "export"}, Description: "Enable-pin handling when a regulator pin-role map is available."},
			{Name: "include_power_led", Type: ParameterBool, Default: false, Description: "Include a downstream power indicator."},
		},
		Ports: []BlockPort{
			{Name: "VIN", Direction: PortPower, Voltage: "input_voltage", Description: "Unregulated input."},
			{Name: "VOUT", Direction: PortPower, Voltage: "output_voltage", Description: "Regulated output."},
			{Name: "GND", Direction: PortPower, Description: "Ground return."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: defaultRegulatorSymbol, Required: true, Description: "Default regulator candidate."},
			{Kind: "symbol", ID: "Device:C", Required: true, Description: "Input and output capacitors."},
			{Kind: "footprint", ID: "Package_TO_SOT_SMD:SOT-223-3_TabPin2", Required: true, Description: "Default regulator footprint."},
			{Kind: "footprint", ID: "Capacitor_SMD:C_0805_2012Metric", Required: true, Description: "Default capacitor footprint."},
		},
		Components:     voltageRegulatorComponents(),
		PCBRealization: voltageRegulatorPCBRealization(),
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Notes: []string{"Emits deterministic fixed-linear-regulator transactions; part-specific thermal and stability requirements remain warnings until resolver metadata improves."},
		},
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
			{Name: "mcu_symbol", Type: ParameterSymbolID, Default: defaultMCUSymbol, Description: "KiCad symbol ID for a supported MCU pin-role template. Only the default ATmega328P-A template is currently supported."},
			{Name: "mcu_footprint", Type: ParameterFootprintID, Default: defaultMCUFootprint, Description: "KiCad footprint ID compatible with the selected MCU pin-role template."},
			{Name: "supply_voltage", Type: ParameterVoltage, Default: "3.3V", Description: "MCU supply rail."},
			{Name: "clock_mode", Type: ParameterEnum, Default: "internal", Allowed: []any{"internal"}, Description: "Clock source mode. Only internal clock wiring is currently supported."},
			{Name: "reset_mode", Type: ParameterEnum, Default: "pullup", Allowed: []any{"pullup", "pullup_switch", "external"}, Description: "Reset-pin support circuit."},
			{Name: "programming_header", Type: ParameterEnum, Default: "isp", Allowed: []any{"none", "isp", "uart"}, Description: "Programming/debug header mode."},
			{Name: "decoupling_count", Type: ParameterNumber, Default: 3, Description: "Number of local supply decoupling capacitors."},
			{Name: "decoupling_value", Type: ParameterCapacitance, Default: "100nF", Description: "Local decoupling capacitor value."},
			{Name: "capacitor_footprint", Type: ParameterFootprintID, Default: "Capacitor_SMD:C_0805_2012Metric", Description: "Decoupling capacitor footprint ID."},
			{Name: "reset_pullup_value", Type: ParameterResistance, Default: "10k", Description: "Reset pull-up resistor value."},
			{Name: "reset_resistor_footprint", Type: ParameterFootprintID, Default: "Resistor_SMD:R_0805_2012Metric", Description: "Reset pull-up resistor footprint ID."},
		},
		Ports: []BlockPort{
			{Name: "VCC", Direction: PortPower, Voltage: "supply_voltage", Description: "MCU supply input."},
			{Name: "GND", Direction: PortPower, Description: "Ground return."},
			{Name: "RESET", Direction: PortInput, Description: "Reset signal."},
			{Name: "AREF", Direction: PortPassive, Description: "Analog reference node with local decoupling."},
			{Name: "GPIO", Direction: PortBidirectional, Description: "General-purpose PB0 application pin."},
			{Name: "MOSI", Direction: PortBidirectional, Description: "SPI programming data from programmer."},
			{Name: "MISO", Direction: PortBidirectional, Description: "SPI programming data to programmer."},
			{Name: "SCK", Direction: PortInput, Description: "SPI programming clock."},
			{Name: "UART_TX", Direction: PortOutput, Description: "UART transmit pin when UART header is enabled."},
			{Name: "UART_RX", Direction: PortInput, Description: "UART receive pin when UART header is enabled."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: defaultMCUSymbol, Required: true, Description: "Supported MCU symbol template."},
			{Kind: "symbol", ID: "Device:C", Required: true, Description: "Decoupling capacitors."},
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "Reset pull-up resistor."},
			{Kind: "symbol", ID: "Connector_Generic:Conn_02x03_Odd_Even", Required: false, Description: "Optional AVR ISP header."},
			{Kind: "symbol", ID: "Connector_Generic:Conn_01x04", Required: false, Description: "Optional UART header."},
			{Kind: "symbol", ID: "Switch:SW_Push", Required: false, Description: "Optional reset switch."},
			{Kind: "footprint", ID: defaultMCUFootprint, Required: true, Description: "Default MCU footprint."},
			{Kind: "footprint", ID: "Capacitor_SMD:C_0805_2012Metric", Required: false, Description: "Default decoupling capacitor footprint."},
			{Kind: "footprint", ID: "Resistor_SMD:R_0805_2012Metric", Required: false, Description: "Default reset pull-up resistor footprint."},
		},
		Components:     mcuMinimalComponents(),
		PCBRealization: mcuMinimalPCBRealization(),
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Notes: []string{"Uses an explicit ATmega328P-A pin-role map; arbitrary MCU symbols remain blocked until resolver semantic metadata is available."},
		},
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
			{Name: "connector_footprint", Type: ParameterFootprintID, Default: "Connector_USB:USB_C_Receptacle_HRO_TYPE-C-31-M-12", Description: "USB-C receptacle footprint ID."},
			{Name: "current_limit", Type: ParameterCurrent, Default: "500mA", Description: "Expected maximum VBUS current."},
			{Name: "include_fuse", Type: ParameterBool, Default: true, Description: "Include a resettable fuse on VBUS."},
			{Name: "include_tvs", Type: ParameterBool, Default: true, Description: "Include VBUS ESD/TVS protection."},
			{Name: "include_bulk_capacitor", Type: ParameterBool, Default: true, Description: "Include a VBUS bulk capacitor."},
			{Name: "include_power_led", Type: ParameterBool, Default: false, Description: "Include a VBUS power indicator."},
			{Name: "shield_policy", Type: ParameterEnum, Default: "chassis", Allowed: []any{"chassis", "gnd", "floating"}, Description: "Connector shield handling."},
			{Name: "data_mode", Type: ParameterEnum, Default: "power_only", Allowed: []any{"power_only"}, Description: "USB data handling mode. Only power_only is currently supported."},
		},
		Ports: []BlockPort{
			{Name: "VBUS_OUT", Direction: PortPower, Voltage: "5V", Description: "Protected 5 V output."},
			{Name: "GND", Direction: PortPower, Description: "Ground return."},
			{Name: "SHIELD", Direction: PortPassive, Description: "Connector shield node."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: defaultUSBCSymbol, Required: true, Description: "USB-C sink connector."},
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "CC pull-down resistors."},
			{Kind: "symbol", ID: "Device:Fuse", Required: false, Description: "Optional VBUS fuse."},
			{Kind: "symbol", ID: "Device:D_TVS", Required: false, Description: "Optional VBUS TVS diode."},
			{Kind: "symbol", ID: "Device:C", Required: false, Description: "Optional VBUS bulk capacitor."},
			{Kind: "footprint", ID: "Connector_USB:USB_C_Receptacle_HRO_TYPE-C-31-M-12", Required: false, Description: "Default USB-C receptacle footprint when connector_footprint is not overridden."},
		},
		Components:     usbCPowerComponents(),
		PCBRealization: usbCPowerPCBRealization(),
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Notes: []string{"Implements USB-C sink power-only wiring with CC pull-downs; USB2 data and no-connect marker emission remain explicit gaps."},
		},
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
			{Name: "sensor_symbol", Type: ParameterSymbolID, Default: defaultI2CSensorSymbol, Description: "KiCad symbol ID for the supported generic I2C sensor template."},
			{Name: "sensor_footprint", Type: ParameterFootprintID, Default: "Package_SO:SOIC-8_3.9x4.9mm_P1.27mm", Description: "KiCad footprint ID for the sensor package."},
			{Name: "i2c_address", Type: ParameterString, Required: true, Description: "7-bit I2C address such as 0x48."},
			{Name: "pullup_value", Type: ParameterResistance, Default: "4.7k", Description: "I2C pull-up resistor value."},
			{Name: "pullup_footprint", Type: ParameterFootprintID, Default: "Resistor_SMD:R_0805_2012Metric", Description: "Pull-up resistor footprint ID."},
			{Name: "include_pullups", Type: ParameterBool, Default: true, Description: "Include SDA/SCL pull-up resistors."},
			{Name: "include_interrupt", Type: ParameterBool, Default: false, Description: "Export and wire an interrupt port."},
			{Name: "include_decoupling", Type: ParameterBool, Default: true, Description: "Include a local supply decoupling capacitor."},
			{Name: "decoupling_value", Type: ParameterCapacitance, Default: "100nF", Description: "Local decoupling capacitor value."},
			{Name: "decoupling_footprint", Type: ParameterFootprintID, Default: "Capacitor_SMD:C_0805_2012Metric", Description: "Decoupling capacitor footprint ID."},
		},
		Ports: []BlockPort{
			{Name: "VCC", Direction: PortPower, Voltage: "supply_voltage", Description: "Sensor supply input."},
			{Name: "GND", Direction: PortPower, Description: "Ground return."},
			{Name: "SDA", Direction: PortBidirectional, Description: "I2C data."},
			{Name: "SCL", Direction: PortInput, Description: "I2C clock."},
			{Name: "INT", Direction: PortOutput, Description: "Optional interrupt output."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: defaultI2CSensorSymbol, Required: true, Description: "Generic I2C sensor template."},
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "Optional pull-up resistors."},
			{Kind: "symbol", ID: "Device:C", Required: true, Description: "Optional decoupling capacitor."},
			{Kind: "footprint", ID: "Package_SO:SOIC-8_3.9x4.9mm_P1.27mm", Required: true, Description: "Default sensor footprint."},
			{Kind: "footprint", ID: "Resistor_SMD:R_0805_2012Metric", Required: false, Description: "Default pull-up resistor footprint."},
			{Kind: "footprint", ID: "Capacitor_SMD:C_0805_2012Metric", Required: false, Description: "Default decoupling capacitor footprint."},
		},
		Components:     i2cSensorComponents(),
		PCBRealization: i2cSensorPCBRealization(),
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Notes: []string{"Uses an explicit generic I2C sensor pin-role template; real part-specific symbols require future pin-role metadata."},
		},
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
			{Name: "opamp_symbol", Type: ParameterSymbolID, Default: defaultOpAmpSymbol, Description: "KiCad symbol ID for the supported op-amp template."},
			{Name: "opamp_footprint", Type: ParameterFootprintID, Default: "Package_TO_SOT_SMD:SOT-23-5", Description: "Op-amp footprint ID."},
			{Name: "single_supply", Type: ParameterBool, Default: true, Description: "Use a single-supply bias network."},
			{Name: "input_coupling", Type: ParameterEnum, Default: "dc", Allowed: []any{"dc", "ac"}, Description: "Input coupling mode."},
			{Name: "feedback_footprint", Type: ParameterFootprintID, Default: "Resistor_SMD:R_0805_2012Metric", Description: "Feedback resistor footprint ID."},
			{Name: "include_output_resistor", Type: ParameterBool, Default: false, Description: "Add a small series resistor at the output."},
		},
		Ports: []BlockPort{
			{Name: "IN", Direction: PortInput, Description: "Signal input."},
			{Name: "OUT", Direction: PortOutput, Description: "Signal output."},
			{Name: "VCC", Direction: PortPower, Description: "Positive supply."},
			{Name: "GND", Direction: PortPower, Description: "Ground or negative supply."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "Feedback resistors."},
			{Kind: "symbol", ID: "Device:C", Required: true, Description: "Decoupling and coupling capacitors."},
			{Kind: "symbol", ID: defaultOpAmpSymbol, Required: true, Description: "Default op-amp candidate."},
			{Kind: "footprint", ID: "Package_TO_SOT_SMD:SOT-23-5", Required: true, Description: "Default op-amp footprint."},
		},
		Components:     opAmpGainStageComponents(),
		PCBRealization: opAmpGainStagePCBRealization(),
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Notes: []string{"Implements a non-inverting LMV321 template with explicit pin roles; other op-amp symbols require future pin-role metadata."},
		},
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
		Components:     connectorBreakoutComponents(),
		PCBRealization: connectorBreakoutPCBRealization(),
		ValidationRules: []BlockValidationRule{
			{ID: "connector.pin_names.required", Severity: BlockValidationSeverityBlocked, Description: "Connector pin names must be non-empty and unique."},
			{ID: "connector.pin_count.matches_names", Severity: BlockValidationSeverityBlocked, Description: "Connector pin count must match the requested pin-name list."},
			{ID: "connector.symbol.resolved", Severity: BlockValidationSeverityBlocked, Description: "Connector symbol must be known for the requested pin count."},
			{ID: "connector.footprint.resolved", Severity: BlockValidationSeverityBlocked, Description: "Connector footprint must be known for the requested pin count."},
			{ID: "connector.pin_numbering.evidence", Severity: BlockValidationSeverityBlocked, Description: "Connector pin numbering must be explicit before connectivity readiness."},
		},
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Notes: []string{"Dynamic connector port expansion is implemented; pin numbering remains structural until resolver evidence is attached."},
		},
	}
}

func experimentalVerification(note string) VerificationRecord {
	return VerificationRecord{
		Level: VerificationExperimental,
		Notes: []string{note},
	}
}
