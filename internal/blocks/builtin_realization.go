package blocks

import "kicadai/internal/components"

func ledIndicatorComponents() []BlockComponent {
	return []BlockComponent{
		{
			Role:              "resistor",
			RefPrefix:         "R",
			Value:             "330",
			SymbolID:          "Device:R",
			FootprintID:       "Resistor_SMD:R_0805_2012Metric",
			Pins:              twoTerminalHorizontalPins(),
			ComponentQuery:    &components.Query{Family: "resistor", Package: "0805", ValueKind: "resistance"},
			MinimumConfidence: components.ConfidenceRuleInferred,
			Acceptance:        components.AcceptanceConnectivity,
		},
		{
			Role:              "led",
			RefPrefix:         "D",
			Value:             "LED",
			SymbolID:          "Device:LED",
			FootprintID:       "LED_SMD:LED_0805_2012Metric",
			Pins:              twoTerminalHorizontalPins(),
			ComponentID:       "led.generic.0805",
			ComponentVariant:  "0805",
			MinimumConfidence: components.ConfidenceVerified,
			Acceptance:        components.AcceptanceConnectivity,
		},
	}
}

func ledIndicatorPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "resistor", FootprintParam: "resistor_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "led", FootprintParam: "led_footprint", Placement: RelativePlacement{XMM: 5, YMM: 0, RotationDeg: 180, Layer: "F.Cu"}},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "inline_indicator", ComponentRoles: []string{"resistor", "led"}, AnchorRole: "resistor", Bounds: &RelativeBounds{MinXMM: -2, MinYMM: -2, MaxXMM: 8, MaxYMM: 2}}},
		LocalRoutes:     []PCBLocalRoute{{ID: "series", NetTemplate: "led_series", From: RouteEndpoint{ComponentRole: "resistor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "led", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true}},
		Validation:      PCBValidationExpectations{RequiredNets: []string{"in", "led_series", "gnd"}, RequiredRoutes: []string{"series"}},
	}
}

func connectorBreakoutComponents() []BlockComponent {
	return []BlockComponent{{Role: "connector", RefPrefix: "J", Value: "Connector", SymbolID: defaultConnectorSymbol, FootprintID: defaultConnectorFootprint, Pins: connectorSymbolPins(2)}}
}

func connectorBreakoutPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components:        []PCBComponentRealization{{ComponentRole: "connector", FootprintParam: "connector_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}}},
		PlacementGroups:   []PCBPlacementGroup{{ID: "connector_edge", ComponentRoles: []string{"connector"}, AnchorRole: "connector", Bounds: &RelativeBounds{MinXMM: -3, MinYMM: -10, MaxXMM: 3, MaxYMM: 10}}},
		Validation:        PCBValidationExpectations{AllowedUnroutedNets: []string{"*"}},
	}
}

func voltageRegulatorComponents() []BlockComponent {
	return []BlockComponent{
		{Role: "regulator", RefPrefix: "U", Value: "LDO", SymbolID: defaultRegulatorSymbol, FootprintID: "Package_TO_SOT_SMD:SOT-223-3_TabPin2", Pins: fixedRegulatorPins()},
		{Role: "input_capacitor", RefPrefix: "C", Value: "10uF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "output_capacitor", RefPrefix: "C", Value: "10uF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "power_led_resistor", RefPrefix: "R", Value: "1k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "power_led", RefPrefix: "D", Value: "POWER LED", SymbolID: "Device:LED", FootprintID: "LED_SMD:LED_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
	}
}

func voltageRegulatorPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "regulator", FootprintParam: "regulator_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "input_capacitor", FootprintParam: "capacitor_footprint", Placement: RelativePlacement{XMM: -6, YMM: -4, Layer: "F.Cu"}},
			{ComponentRole: "output_capacitor", FootprintParam: "capacitor_footprint", Placement: RelativePlacement{XMM: 6, YMM: -4, Layer: "F.Cu"}},
			{ComponentRole: "power_led_resistor", FootprintID: "Resistor_SMD:R_0805_2012Metric", Placement: RelativePlacement{XMM: 13, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "power_led", FootprintID: "LED_SMD:LED_0805_2012Metric", Placement: RelativePlacement{XMM: 18, YMM: 0, RotationDeg: 180, Layer: "F.Cu"}},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "regulator_core", ComponentRoles: []string{"regulator", "input_capacitor", "output_capacitor"}, AnchorRole: "regulator", Bounds: &RelativeBounds{MinXMM: -9, MinYMM: -7, MaxXMM: 9, MaxYMM: 5}}},
		LocalRoutes: []PCBLocalRoute{
			{ID: "vin_bypass", NetTemplate: "vin", From: RouteEndpoint{ComponentRole: "input_capacitor", Pin: "1"}, To: RouteEndpoint{ComponentRole: "regulator", Pin: "3"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
			{ID: "vout_bypass", NetTemplate: "vout", From: RouteEndpoint{ComponentRole: "output_capacitor", Pin: "1"}, To: RouteEndpoint{ComponentRole: "regulator", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
			{ID: "gnd_bypass", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "input_capacitor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "output_capacitor", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
		},
		Constraints: []PCBConstraint{
			{ID: "regulator_power_width", Kind: "min_width", NetTemplate: "vin", MinWidthMM: 0.5, Description: "Regulator input path should use a wider local route."},
			{ID: "regulator_output_width", Kind: "min_width", NetTemplate: "vout", MinWidthMM: 0.5, Description: "Regulator output path should use a wider local route."},
			{ID: "regulator_input_capacitor_proximity", Kind: "proximity", NetTemplate: "vin", AppliesTo: []string{"regulator", "input_capacitor"}, MaxLengthMM: 8, Description: "Input capacitor should remain close to the regulator input pin."},
			{ID: "regulator_output_capacitor_proximity", Kind: "proximity", NetTemplate: "vout", AppliesTo: []string{"regulator", "output_capacitor"}, MaxLengthMM: 8, Description: "Output capacitor should remain close to the regulator output pin."},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"vin", "vout", "gnd"}, RequiredRoutes: []string{"vin_bypass", "vout_bypass", "gnd_bypass"}},
	}
}

func i2cSensorComponents() []BlockComponent {
	return []BlockComponent{
		{Role: "sensor", RefPrefix: "U", Value: "I2C Sensor", SymbolID: defaultI2CSensorSymbol, FootprintID: "Package_SO:SOIC-8_3.9x4.9mm_P1.27mm", Pins: i2cSensorPins(genericI2CSensorPins)},
		{Role: "decoupling_capacitor", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "sda_pullup", RefPrefix: "R", Value: "4.7k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "scl_pullup", RefPrefix: "R", Value: "4.7k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
	}
}

func i2cSensorPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "sensor", FootprintParam: "sensor_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "decoupling_capacitor", FootprintParam: "decoupling_footprint", Placement: RelativePlacement{XMM: -5, YMM: -4, Layer: "F.Cu"}},
			{ComponentRole: "sda_pullup", FootprintParam: "pullup_footprint", Placement: RelativePlacement{XMM: 6, YMM: -5, Layer: "F.Cu"}},
			{ComponentRole: "scl_pullup", FootprintParam: "pullup_footprint", Placement: RelativePlacement{XMM: 6, YMM: 0, Layer: "F.Cu"}},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "sensor_core", ComponentRoles: []string{"sensor", "decoupling_capacitor", "sda_pullup", "scl_pullup"}, AnchorRole: "sensor", Bounds: &RelativeBounds{MinXMM: -8, MinYMM: -8, MaxXMM: 10, MaxYMM: 5}}},
		LocalRoutes: []PCBLocalRoute{
			{ID: "vcc_decoupling", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "decoupling_capacitor", Pin: "1"}, To: RouteEndpoint{ComponentRole: "sensor", Pin: genericI2CSensorPins.VCC}, Layer: "F.Cu", WidthMM: 0.3, Required: true},
			{ID: "gnd_decoupling", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "decoupling_capacitor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "sensor", Pin: genericI2CSensorPins.GND}, Layer: "F.Cu", WidthMM: 0.3, Required: true},
			{ID: "sda_pullup", NetTemplate: "sda", From: RouteEndpoint{ComponentRole: "sda_pullup", Pin: "2"}, To: RouteEndpoint{ComponentRole: "sensor", Pin: genericI2CSensorPins.SDA}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "scl_pullup", NetTemplate: "scl", From: RouteEndpoint{ComponentRole: "scl_pullup", Pin: "2"}, To: RouteEndpoint{ComponentRole: "sensor", Pin: genericI2CSensorPins.SCL}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"vcc", "gnd", "sda", "scl"}, RequiredRoutes: []string{"vcc_decoupling", "gnd_decoupling", "sda_pullup", "scl_pullup"}},
	}
}

func opAmpGainStageComponents() []BlockComponent {
	return []BlockComponent{
		{Role: "opamp", RefPrefix: "U", Value: "LMV321", SymbolID: defaultOpAmpSymbol, FootprintID: "Package_TO_SOT_SMD:SOT-23-5", Pins: opAmpPins(lmv321Pins), ComponentID: "opamp.generic.single.lmv321", ComponentVariant: "sot23_5", Acceptance: components.AcceptanceConnectivity},
		{Role: "gain_to_ground", RefPrefix: "R", Value: "10k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "feedback", RefPrefix: "R", Value: "10k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "decoupling_capacitor", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "output_resistor", RefPrefix: "R", Value: "100", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "bias_top", RefPrefix: "R", Value: "100k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "bias_bottom", RefPrefix: "R", Value: "100k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "input_coupling", RefPrefix: "C", Value: "1uF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
	}
}

func opAmpGainStagePCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "opamp", FootprintParam: "opamp_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "feedback", FootprintParam: "feedback_footprint", Placement: RelativePlacement{XMM: -4, YMM: -5, Layer: "F.Cu"}},
			{ComponentRole: "gain_to_ground", FootprintParam: "feedback_footprint", Placement: RelativePlacement{XMM: -4, YMM: 5, Layer: "F.Cu"}},
			{ComponentRole: "decoupling_capacitor", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Placement: RelativePlacement{XMM: 4, YMM: -5, Layer: "F.Cu"}},
			{ComponentRole: "output_resistor", FootprintParam: "feedback_footprint", Placement: RelativePlacement{XMM: 8, YMM: 0, Layer: "F.Cu"}},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "opamp_core", ComponentRoles: []string{"opamp", "feedback", "gain_to_ground", "decoupling_capacitor"}, AnchorRole: "opamp", Bounds: &RelativeBounds{MinXMM: -8, MinYMM: -8, MaxXMM: 8, MaxYMM: 8}}},
		LocalRoutes: []PCBLocalRoute{
			{ID: "feedback_loop", NetTemplate: "feedback", From: RouteEndpoint{ComponentRole: "feedback", Pin: "2"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.INN}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "supply_decoupling", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "decoupling_capacitor", Pin: "1"}, To: RouteEndpoint{ComponentRole: "opamp", Pin: lmv321Pins.VCC}, Layer: "F.Cu", WidthMM: 0.3, Required: true},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"in", "out", "feedback", "vcc", "gnd"}, RequiredRoutes: []string{"feedback_loop", "supply_decoupling"}},
	}
}

func mcuMinimalComponents() []BlockComponent {
	return []BlockComponent{
		{Role: "mcu", RefPrefix: "U", Value: "ATmega328P-A", SymbolID: defaultMCUSymbol, FootprintID: defaultMCUFootprint, Pins: mcuPins(supportedMCUTemplates[defaultMCUSymbol])},
		{Role: "decoupling_capacitor", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "aref_decoupling_capacitor", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "reset_pullup", RefPrefix: "R", Value: "10k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "reset_switch", RefPrefix: "SW", Value: "RESET", SymbolID: resetSwitchSymbol, FootprintID: resetSwitchFootprint, Pins: twoTerminalHorizontalPins()},
		{Role: "isp_header", RefPrefix: "J", Value: "AVR ISP", SymbolID: ispHeaderSymbol, FootprintID: ispHeaderFootprint, Pins: twoByThreeHeaderPins()},
		{Role: "uart_header", RefPrefix: "J", Value: "UART", SymbolID: uartHeaderSymbol, FootprintID: uartHeaderFootprint, Pins: connectorSymbolPins(4)},
	}
}

func mcuMinimalPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "mcu", FootprintParam: "mcu_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "decoupling_capacitor", FootprintParam: "capacitor_footprint", Placement: RelativePlacement{XMM: -8, YMM: -8, Layer: "F.Cu"}},
			{ComponentRole: "aref_decoupling_capacitor", FootprintParam: "capacitor_footprint", Placement: RelativePlacement{XMM: 8, YMM: -8, Layer: "F.Cu"}},
			{ComponentRole: "reset_pullup", FootprintParam: "reset_resistor_footprint", Placement: RelativePlacement{XMM: -10, YMM: 7, Layer: "F.Cu"}},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "mcu_core", ComponentRoles: []string{"mcu", "decoupling_capacitor", "aref_decoupling_capacitor", "reset_pullup"}, AnchorRole: "mcu", Bounds: &RelativeBounds{MinXMM: -14, MinYMM: -14, MaxXMM: 14, MaxYMM: 14}}},
		Validation:      PCBValidationExpectations{RequiredNets: []string{"vcc", "gnd", "reset"}},
		UnsupportedBehaviors: []string{
			"multiple decoupling capacitor instances share one component role until indexed component realization is implemented",
			"programming headers and reset switch placement are metadata-only until conditional realizations are implemented",
		},
	}
}

func usbCPowerComponents() []BlockComponent {
	return []BlockComponent{
		{Role: "usb_c_receptacle", RefPrefix: "J", Value: "USB-C Power", SymbolID: defaultUSBCSymbol, FootprintID: "Connector_USB:USB_C_Receptacle_HRO_TYPE-C-31-M-12", Pins: usbCSymbolPins(usbCPowerPins)},
		{Role: "cc1_rd", RefPrefix: "R", Value: "5.1k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "cc2_rd", RefPrefix: "R", Value: "5.1k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "vbus_fuse", RefPrefix: "F", Value: "Fuse", SymbolID: "Device:Fuse", FootprintID: "Fuse:Fuse_1206_3216Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "vbus_tvs", RefPrefix: "D", Value: "VBUS TVS", SymbolID: "Device:D_TVS", FootprintID: "Diode_SMD:D_SOD-323", Pins: twoTerminalHorizontalPins()},
		{Role: "bulk_capacitor", RefPrefix: "C", Value: "10uF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "power_led_resistor", RefPrefix: "R", Value: usbPowerLEDResistorValue, SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
		{Role: "power_led", RefPrefix: "D", Value: "VBUS LED", SymbolID: "Device:LED", FootprintID: "LED_SMD:LED_0805_2012Metric", Pins: twoTerminalHorizontalPins()},
	}
}

func usbCPowerPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationPlacementVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "usb_c_receptacle", FootprintParam: "connector_footprint", Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "cc1_rd", FootprintID: "Resistor_SMD:R_0805_2012Metric", Placement: RelativePlacement{XMM: 7, YMM: -5, Layer: "F.Cu"}},
			{ComponentRole: "cc2_rd", FootprintID: "Resistor_SMD:R_0805_2012Metric", Placement: RelativePlacement{XMM: 7, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "vbus_fuse", FootprintID: "Fuse:Fuse_1206_3216Metric", Placement: RelativePlacement{XMM: 12, YMM: 6, Layer: "F.Cu"}},
			{ComponentRole: "vbus_tvs", FootprintID: "Diode_SMD:D_SOD-323", Placement: RelativePlacement{XMM: 18, YMM: 6, Layer: "F.Cu"}},
			{ComponentRole: "bulk_capacitor", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Placement: RelativePlacement{XMM: 18, YMM: 0, Layer: "F.Cu"}},
		},
		PlacementGroups: []PCBPlacementGroup{{ID: "usb_c_power_entry", ComponentRoles: []string{"usb_c_receptacle", "cc1_rd", "cc2_rd", "vbus_fuse", "vbus_tvs", "bulk_capacitor"}, AnchorRole: "usb_c_receptacle", Bounds: &RelativeBounds{MinXMM: -5, MinYMM: -8, MaxXMM: 22, MaxYMM: 10}}},
		Keepouts: []PCBKeepout{
			{ID: "usb_c_edge_keepout", Layer: "F.Cu", Bounds: RelativeBounds{MinXMM: -5, MinYMM: -8, MaxXMM: 3, MaxYMM: 8}, AppliesTo: []string{"usb_c_receptacle"}, Description: "Reserve board-edge clearance around the USB-C receptacle."},
		},
		LocalRoutes: []PCBLocalRoute{
			{ID: "cc1_pull_down", NetTemplate: "cc1", From: RouteEndpoint{ComponentRole: "usb_c_receptacle", Pin: usbCPowerPins.CC1}, To: RouteEndpoint{ComponentRole: "cc1_rd", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "cc2_pull_down", NetTemplate: "cc2", From: RouteEndpoint{ComponentRole: "usb_c_receptacle", Pin: usbCPowerPins.CC2}, To: RouteEndpoint{ComponentRole: "cc2_rd", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "vbus_entry", NetTemplate: "vbus_connector", From: RouteEndpoint{ComponentRole: "usb_c_receptacle", Pin: "A4"}, To: RouteEndpoint{ComponentRole: "vbus_fuse", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.75, Required: true},
		},
		Constraints: []PCBConstraint{
			{ID: "usb_c_vbus_width", Kind: "min_width", NetTemplate: "vbus_connector", MinWidthMM: 0.75, Description: "VBUS entry path should support requested current."},
			{ID: "usb_c_edge_facing", Kind: "edge_facing", AppliesTo: []string{"usb_c_receptacle"}, Description: "USB-C receptacle should be placed at the board edge."},
		},
		Validation: PCBValidationExpectations{RequiredNets: []string{"vbus_connector", "vbus_out", "gnd", "cc1", "cc2"}, RequiredRoutes: []string{"cc1_pull_down", "cc2_pull_down", "vbus_entry"}},
		UnsupportedBehaviors: []string{
			"USB2 data no-connect markers remain schematic metadata only",
			"shield policy routing depends on project grounding strategy",
		},
	}
}
