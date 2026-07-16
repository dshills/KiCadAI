package blocks

import (
	"fmt"

	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const (
	esp32WROOM32ESymbol          = "RF_Module:ESP32-WROOM-32E"
	esp32WROOM32EFootprint       = "RF_Module:ESP32-WROOM-32E"
	esp32WROOM32EComponentID     = "mcu.espressif.esp32_wroom_32e"
	esp32WROOM32ESchematicGND    = "[1,15,38,39]"
	esp32ProgrammingHeaderSymbol = "Connector_Generic:Conn_01x04"
	esp32ProgrammingHeader       = "Connector_PinHeader_2.54mm:PinHeader_1x04_P2.54mm_Vertical"
	esp32PeripheralHeaderSymbol  = "Connector_Generic:Conn_01x06"
	esp32PeripheralHeader        = "Connector_PinHeader_2.54mm:PinHeader_1x06_P2.54mm_Vertical"
	esp32ControlSwitchFootprint  = "Button_Switch_SMD:SW_SPST_B3U-1000P"
)

var esp32WROOM32ERoles = struct {
	VCC, EN, BOOT, GPIO, SDA, SCL, MOSI, MISO, SCK, UARTTX, UARTRX string
	GND                                                            []string
}{
	VCC: "2", EN: "3", BOOT: "25", GPIO: "28", SDA: "33", SCL: "36",
	MOSI: "37", MISO: "31", SCK: "30", UARTTX: "35", UARTRX: "34",
	GND: []string{"1", "15", "38", "39"},
}

func esp32WROOM32EMinimalDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          "esp32_wroom_32e_minimal",
		Name:        "ESP32-WROOM-32E Minimal System",
		Description: "Reviewed ESP32-WROOM-32E module with power decoupling, EN reset timing, IO0 boot strapping, manual reset/boot controls, UART programming, and antenna exclusion.",
		Version:     "0.1.0",
		Category:    "digital",
		Parameters: []BlockParameter{
			{Name: "supply_voltage", Type: ParameterVoltage, Default: "3.3V", Description: "ESP32 module supply; the reviewed realization accepts 3.0 V through 3.6 V."},
			{Name: "bulk_capacitance", Type: ParameterCapacitance, Default: "10uF", Description: "Local bulk capacitance at the module power entry."},
			{Name: "decoupling_value", Type: ParameterCapacitance, Default: "100nF", Description: "High-frequency local bypass capacitance."},
			{Name: "enable_pullup_value", Type: ParameterResistance, Default: "10k", Description: "EN pull-up used by the reviewed RC power-up circuit."},
			{Name: "enable_capacitance", Type: ParameterCapacitance, Default: "1uF", Description: "EN-to-ground capacitance used by the reviewed RC power-up circuit."},
			{Name: "boot_pullup_value", Type: ParameterResistance, Default: "10k", Description: "GPIO0 pull-up preserving normal SPI boot."},
			{Name: "capacitor_footprint", Type: ParameterFootprintID, Default: "Capacitor_SMD:C_0805_2012Metric", Description: "Reviewed support-capacitor footprint."},
			{Name: "resistor_footprint", Type: ParameterFootprintID, Default: "Resistor_SMD:R_0805_2012Metric", Description: "Reviewed strap-resistor footprint."},
			{Name: "switch_footprint", Type: ParameterFootprintID, Default: esp32ControlSwitchFootprint, Description: "Reviewed two-pad manual reset/boot switch footprint."},
		},
		Ports: []BlockPort{
			{Name: "VCC", Direction: PortPower, Voltage: "supply_voltage", Description: "Regulated ESP32 supply input."},
			{Name: "GND", Direction: PortPower, Description: "Module and support-circuit ground."},
			{Name: "GPIO", Direction: PortBidirectional, Voltage: "supply_voltage", Description: "Application GPIO17; not a boot strap and supports input/output."},
			{Name: "SDA", Direction: PortBidirectional, Voltage: "supply_voltage", Description: "I2C SDA mapped deterministically to GPIO21."},
			{Name: "SCL", Direction: PortBidirectional, Voltage: "supply_voltage", Description: "I2C SCL mapped deterministically to GPIO22."},
			{Name: "MOSI", Direction: PortOutput, Voltage: "supply_voltage", Description: "SPI MOSI mapped deterministically to GPIO23."},
			{Name: "MISO", Direction: PortInput, Voltage: "supply_voltage", Description: "SPI MISO mapped deterministically to GPIO19."},
			{Name: "SCK", Direction: PortOutput, Voltage: "supply_voltage", Description: "SPI clock mapped deterministically to GPIO18."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: esp32WROOM32ESymbol, Required: true, Description: "KiCad ESP32-WROOM-32E module symbol reviewed against the Espressif module pinout."},
			{Kind: "footprint", ID: esp32WROOM32EFootprint, Required: true, Description: "KiCad ESP32-WROOM-32E land pattern with native antenna keepout."},
			{Kind: "symbol", ID: "Device:C", Required: true, Description: "Power and EN timing capacitors."},
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "EN and IO0 pull-ups."},
			{Kind: "symbol", ID: resetSwitchSymbol, Required: true, Description: "Manual reset and boot controls."},
			{Kind: "symbol", ID: esp32ProgrammingHeaderSymbol, Required: true, Description: "3.3 V UART programming header."},
			{Kind: "symbol", ID: esp32PeripheralHeaderSymbol, Required: true, Description: "Deterministic GPIO, I2C, and SPI breakout header."},
			{Kind: "symbol", ID: "power:PWR_FLAG", Required: true, Description: "Standalone ERC source declarations for the externally supplied 3.3 V and ground rails."},
		},
		Components:     esp32WROOM32EComponents(),
		PCBRealization: esp32WROOM32EPCBRealization(),
		ValidationRules: []BlockValidationRule{
			{ID: "esp32.part.fixed", Severity: BlockValidationSeverityBlocked, Description: "The reviewed realization is fixed to ESP32-WROOM-32E and its matching KiCad land pattern."},
			{ID: "esp32.supply.range", Severity: BlockValidationSeverityBlocked, Description: "Supply must remain within the module's reviewed 3.0 V to 3.6 V range."},
			{ID: "esp32.enable.rc", Severity: BlockValidationSeverityBlocked, Description: "EN requires the reviewed 10 kΩ / 1 µF power-up and reset network."},
			{ID: "esp32.boot.strap", Severity: BlockValidationSeverityBlocked, Description: "GPIO0 requires a normal-boot pull-up and a manual download-mode path to ground."},
			{ID: "esp32.programming.uart", Severity: BlockValidationSeverityBlocked, Description: "UART0, 3.3 V, and ground must be exposed for programming."},
			{ID: "esp32.gpio.roles", Severity: BlockValidationSeverityBlocked, Description: "Peripheral roles must use the reviewed non-flash pin assignments and respect input-only and strapping restrictions."},
			{ID: "esp32.antenna.keepout", Severity: BlockValidationSeverityBlocked, Description: "Copper, vias, routes, and unrelated placement are prohibited in the antenna exclusion region."},
		},
		Verification: VerificationRecord{
			Level:       VerificationERCDRCVerified,
			Date:        "2026-07-16",
			KiCadTarget: "10.0.3",
			Evidence: []string{
				"Espressif ESP32-WROOM-32E/32UE Datasheet v2.0",
				"Espressif ESP32 Hardware Design Guidelines",
				"KiCad RF_Module:ESP32-WROOM-32E symbol and footprint",
			},
			Notes: []string{"Fixed module, pin-role, support-network, and antenna-layout profile; this does not imply support for other ESP32 variants."},
		},
	}
}

func esp32WROOM32EComponents() []BlockComponent {
	return []BlockComponent{
		{Role: "module", RefPrefix: "U", Value: "ESP32-WROOM-32E", SymbolID: esp32WROOM32ESymbol, FootprintID: esp32WROOM32EFootprint, Pins: esp32WROOM32EPins(), PreferResolverSymbol: true, ComponentID: esp32WROOM32EComponentID, ComponentVariant: "module", MinimumConfidence: components.ConfidenceVerified, Acceptance: components.AcceptanceERCDRC, PinmapRequired: true},
		{Role: "decoupling_capacitor", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "capacitor", Package: "0805", ValueKind: "capacitance"}, ComponentValueParam: "decoupling_value", ComponentPackageParam: "capacitor_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity},
		{Role: "bulk_capacitor", RefPrefix: "C", Value: "10uF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "capacitor", Package: "0805", ValueKind: "capacitance"}, ComponentValueParam: "bulk_capacitance", ComponentPackageParam: "capacitor_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity},
		{Role: "enable_pullup", RefPrefix: "R", Value: "10k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", Package: "0805", ValueKind: "resistance"}, ComponentValueParam: "enable_pullup_value", ComponentPackageParam: "resistor_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity},
		{Role: "enable_capacitor", RefPrefix: "C", Value: "1uF", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "capacitor", Package: "0805", ValueKind: "capacitance"}, ComponentValueParam: "enable_capacitance", ComponentPackageParam: "capacitor_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity},
		{Role: "boot_pullup", RefPrefix: "R", Value: "10k", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", Package: "0805", ValueKind: "resistance"}, ComponentValueParam: "boot_pullup_value", ComponentPackageParam: "resistor_footprint", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity},
		{Role: "reset_switch", RefPrefix: "SW", Value: "RESET", SymbolID: resetSwitchSymbol, FootprintID: esp32ControlSwitchFootprint, Pins: twoTerminalHorizontalPins(), ComponentID: "switch.omron.b3u_1000p", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity},
		{Role: "boot_switch", RefPrefix: "SW", Value: "BOOT", SymbolID: resetSwitchSymbol, FootprintID: esp32ControlSwitchFootprint, Pins: twoTerminalHorizontalPins(), ComponentID: "switch.omron.b3u_1000p", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity},
		{Role: "programming_header", RefPrefix: "J", Value: "UART PROG", SymbolID: esp32ProgrammingHeaderSymbol, FootprintID: esp32ProgrammingHeader, Pins: connectorSymbolPins(4), ComponentQuery: &components.Query{Family: "connector", ValueKind: "pin_count", Value: "4"}, MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity},
		{Role: "peripheral_header", RefPrefix: "J", Value: "GPIO I2C SPI", SymbolID: esp32PeripheralHeaderSymbol, FootprintID: esp32PeripheralHeader, Pins: esp32PeripheralHeaderPins(), ComponentQuery: &components.Query{Family: "connector", ValueKind: "pin_count", Value: "6"}, MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity},
		{Role: "supply_driver", RefPrefix: "#FLG", Value: "PWR_FLAG", SymbolID: "power:PWR_FLAG", Pins: []transactions.PinSpec{{Number: "1", ExplicitOffset: true}}, SchematicOnly: true},
		{Role: "ground_driver", RefPrefix: "#FLG", Value: "PWR_FLAG", SymbolID: "power:PWR_FLAG", Pins: []transactions.PinSpec{{Number: "1", ExplicitOffset: true}}, SchematicOnly: true},
	}
}

func instantiateESP32WROOM32EMinimal(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if supply, ok := parseUnit(params["supply_voltage"], "V", voltageMultipliers()); !ok || supply < 3.0 || supply > 3.6 {
		issues = append(issues, blockIssue("params.supply_voltage", "ESP32-WROOM-32E supply_voltage must be between 3.0V and 3.6V"))
	}
	for _, requirement := range []struct {
		name, suffix string
		multipliers  []unitMultiplier
	}{
		{name: "bulk_capacitance", suffix: "F", multipliers: capacitanceMultipliers()},
		{name: "decoupling_value", suffix: "F", multipliers: capacitanceMultipliers()},
		{name: "enable_pullup_value", suffix: "Ω", multipliers: resistanceMultipliers()},
		{name: "enable_capacitance", suffix: "F", multipliers: capacitanceMultipliers()},
		{name: "boot_pullup_value", suffix: "Ω", multipliers: resistanceMultipliers()},
	} {
		if value, ok := parseUnit(params[requirement.name], requirement.suffix, requirement.multipliers); !ok || value <= 0 {
			issues = append(issues, blockIssue("params."+requirement.name, requirement.name+" must be a positive engineering literal"))
		}
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	componentsByRole := map[string]BlockComponent{}
	for _, component := range definition.Components {
		componentsByRole[component.Role] = component
	}
	placements := map[string]transactions.Point{
		"module": {XMM: 80, YMM: 80}, "decoupling_capacitor": {XMM: 35, YMM: 135},
		"bulk_capacitor": {XMM: 50, YMM: 135}, "enable_pullup": {XMM: 55, YMM: 120},
		"enable_capacitor": {XMM: 35, YMM: 100}, "boot_pullup": {XMM: 110, YMM: 120},
		"reset_switch": {XMM: 35, YMM: 80}, "boot_switch": {XMM: 120, YMM: 100},
		"programming_header": {XMM: 130, YMM: 70}, "peripheral_header": {XMM: 160, YMM: 90},
		"supply_driver": {XMM: 160, YMM: 120}, "ground_driver": {XMM: 175, YMM: 120},
	}
	refs := map[string]string{}
	var operations []transactions.Operation
	for _, role := range []string{"module", "decoupling_capacitor", "bulk_capacitor", "enable_pullup", "enable_capacitor", "boot_pullup", "reset_switch", "boot_switch", "programming_header", "peripheral_header", "supply_driver", "ground_driver"} {
		component := componentsByRole[role]
		switch role {
		case "decoupling_capacitor":
			component.Value = stringParam(params, "decoupling_value")
			component.FootprintID = stringParam(params, "capacitor_footprint")
		case "bulk_capacitor":
			component.Value = stringParam(params, "bulk_capacitance")
			component.FootprintID = stringParam(params, "capacitor_footprint")
		case "enable_pullup":
			component.Value = stringParam(params, "enable_pullup_value")
			component.FootprintID = stringParam(params, "resistor_footprint")
		case "enable_capacitor":
			component.Value = stringParam(params, "enable_capacitance")
			component.FootprintID = stringParam(params, "capacitor_footprint")
		case "boot_pullup":
			component.Value = stringParam(params, "boot_pullup_value")
			component.FootprintID = stringParam(params, "resistor_footprint")
		case "reset_switch", "boot_switch":
			component.FootprintID = stringParam(params, "switch_footprint")
		}
		ref := allocator.Next(component.RefPrefix)
		refs[role] = ref
		componentOps, componentIssues := ComponentOperations(component, ref, placements[role])
		operations = append(operations, componentOps...)
		issues = append(issues, componentIssues...)
	}

	nets := map[string]string{}
	for _, name := range []string{"vcc", "gnd", "enable", "boot", "gpio", "sda", "scl", "mosi", "miso", "sck", "uart_tx", "uart_rx"} {
		nets[name] = InstanceNetName(request.InstanceID, name)
	}
	connect := func(fromRole, fromPin, toRole, toPin, net string) {
		appendConnectOperation(&operations, &issues, refs[fromRole], fromPin, refs[toRole], toPin, nets[net])
	}
	appendConnectOperation(&operations, &issues, request.InstanceID, "GND", refs["module"], esp32WROOM32ESchematicGND, nets["gnd"])
	appendConnectOperation(&operations, &issues, request.InstanceID, "VCC", refs["module"], esp32WROOM32ERoles.VCC, nets["vcc"])
	for _, binding := range []struct{ port, pin, net string }{
		{"GPIO", esp32WROOM32ERoles.GPIO, "gpio"}, {"SDA", esp32WROOM32ERoles.SDA, "sda"},
		{"SCL", esp32WROOM32ERoles.SCL, "scl"}, {"MOSI", esp32WROOM32ERoles.MOSI, "mosi"},
		{"MISO", esp32WROOM32ERoles.MISO, "miso"}, {"SCK", esp32WROOM32ERoles.SCK, "sck"},
	} {
		appendConnectOperation(&operations, &issues, request.InstanceID, binding.port, refs["module"], binding.pin, nets[binding.net])
	}
	connect("decoupling_capacitor", "1", "module", esp32WROOM32ERoles.VCC, "vcc")
	connect("decoupling_capacitor", "2", "module", esp32WROOM32ESchematicGND, "gnd")
	connect("bulk_capacitor", "1", "module", esp32WROOM32ERoles.VCC, "vcc")
	connect("bulk_capacitor", "2", "module", esp32WROOM32ESchematicGND, "gnd")
	connect("enable_pullup", "1", "module", esp32WROOM32ERoles.VCC, "vcc")
	connect("enable_pullup", "2", "module", esp32WROOM32ERoles.EN, "enable")
	connect("enable_capacitor", "1", "module", esp32WROOM32ERoles.EN, "enable")
	connect("enable_capacitor", "2", "module", esp32WROOM32ESchematicGND, "gnd")
	connect("reset_switch", "1", "module", esp32WROOM32ERoles.EN, "enable")
	connect("reset_switch", "2", "module", esp32WROOM32ESchematicGND, "gnd")
	connect("boot_pullup", "1", "module", esp32WROOM32ERoles.VCC, "vcc")
	connect("boot_pullup", "2", "module", esp32WROOM32ERoles.BOOT, "boot")
	connect("boot_switch", "1", "module", esp32WROOM32ERoles.BOOT, "boot")
	connect("boot_switch", "2", "module", esp32WROOM32ESchematicGND, "gnd")
	connect("programming_header", "1", "module", esp32WROOM32ERoles.VCC, "vcc")
	connect("programming_header", "2", "module", esp32WROOM32ESchematicGND, "gnd")
	connect("programming_header", "3", "module", esp32WROOM32ERoles.UARTRX, "uart_rx")
	connect("programming_header", "4", "module", esp32WROOM32ERoles.UARTTX, "uart_tx")
	connect("supply_driver", "1", "module", esp32WROOM32ERoles.VCC, "vcc")
	connect("ground_driver", "1", "module", esp32WROOM32ESchematicGND, "gnd")
	for _, binding := range []struct{ headerPin, modulePin, net string }{
		{"1", esp32WROOM32ERoles.MOSI, "mosi"}, {"2", esp32WROOM32ERoles.SCL, "scl"},
		{"3", esp32WROOM32ERoles.SDA, "sda"}, {"4", esp32WROOM32ERoles.MISO, "miso"},
		{"5", esp32WROOM32ERoles.SCK, "sck"}, {"6", esp32WROOM32ERoles.GPIO, "gpio"},
	} {
		connect("peripheral_header", binding.headerPin, "module", binding.modulePin, binding.net)
	}

	usedPins := map[string]bool{esp32WROOM32ESchematicGND: true}
	for _, pin := range []string{
		esp32WROOM32ERoles.VCC, esp32WROOM32ERoles.EN, esp32WROOM32ERoles.BOOT,
		esp32WROOM32ERoles.GPIO, esp32WROOM32ERoles.SDA, esp32WROOM32ERoles.SCL,
		esp32WROOM32ERoles.MOSI, esp32WROOM32ERoles.MISO, esp32WROOM32ERoles.SCK,
		esp32WROOM32ERoles.UARTTX, esp32WROOM32ERoles.UARTRX,
	} {
		usedPins[pin] = true
	}
	intrinsicNoConnect := map[string]bool{"17": true, "18": true, "19": true, "20": true, "21": true, "22": true, "32": true}
	for _, pin := range esp32WROOM32EPins() {
		if usedPins[pin.Number] || intrinsicNoConnect[pin.Number] {
			continue
		}
		operation, noConnectIssues := NoConnectOperation(refs["module"], pin.Number)
		issues = append(issues, noConnectIssues...)
		if len(noConnectIssues) == 0 {
			operations = append(operations, operation)
		}
	}

	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = cloneAnyParams(params)
	output.Instance.Ports = resolvePortVoltages(definition.Ports, params)
	for _, role := range []string{"module", "decoupling_capacitor", "bulk_capacitor", "enable_pullup", "enable_capacitor", "boot_pullup", "reset_switch", "boot_switch", "programming_header", "peripheral_header", "supply_driver", "ground_driver"} {
		output.Instance.Refs = append(output.Instance.Refs, refs[role])
	}
	for _, name := range []string{"vcc", "gnd", "enable", "boot", "gpio", "sda", "scl", "mosi", "miso", "sck", "uart_tx", "uart_rx"} {
		output.Instance.Nets = append(output.Instance.Nets, nets[name])
	}
	return output
}

func esp32WROOM32EPins() []transactions.PinSpec {
	positions := map[string]transactions.Point{
		"2": {XMM: 0, YMM: -35.56}, "3": {XMM: -15.24, YMM: -30.48}, "4": {XMM: -15.24, YMM: -25.4}, "5": {XMM: -15.24, YMM: -22.86},
		"6": {XMM: 15.24, YMM: 25.4}, "7": {XMM: 15.24, YMM: 27.94}, "8": {XMM: 15.24, YMM: 20.32}, "9": {XMM: 15.24, YMM: 22.86},
		"10": {XMM: 15.24, YMM: 12.7}, "11": {XMM: 15.24, YMM: 15.24}, "12": {XMM: 15.24, YMM: 17.78}, "13": {XMM: 15.24, YMM: -10.16},
		"14": {XMM: 15.24, YMM: -15.24}, "16": {XMM: 15.24, YMM: -12.7}, "17": {XMM: -12.7, YMM: 5.08}, "18": {XMM: -12.7, YMM: 7.62},
		"19": {XMM: -12.7, YMM: 12.7}, "20": {XMM: -12.7, YMM: 10.16}, "21": {XMM: -12.7, YMM: 0}, "22": {XMM: -12.7, YMM: 2.54},
		"23": {XMM: 15.24, YMM: -7.62}, "24": {XMM: 15.24, YMM: -25.4}, "25": {XMM: 15.24, YMM: -30.48}, "26": {XMM: 15.24, YMM: -20.32},
		"27": {XMM: 15.24, YMM: -5.08}, "28": {XMM: 15.24, YMM: -2.54}, "29": {XMM: 15.24, YMM: -17.78}, "30": {XMM: 15.24, YMM: 0},
		"31": {XMM: 15.24, YMM: 2.54}, "32": {XMM: -12.7, YMM: 27.94}, "33": {XMM: 15.24, YMM: 5.08}, "34": {XMM: 15.24, YMM: -22.86},
		"35": {XMM: 15.24, YMM: -27.94}, "36": {XMM: 15.24, YMM: 7.62}, "37": {XMM: 15.24, YMM: 10.16}, esp32WROOM32ESchematicGND: {XMM: 0, YMM: 35.56},
	}
	order := []string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "16", "17", "18", "19", "20", "21", "22", "23", "24", "25", "26", "27", "28", "29", "30", "31", "32", "33", "34", "35", "36", "37", esp32WROOM32ESchematicGND}
	pins := make([]transactions.PinSpec, 0, len(order))
	for _, number := range order {
		point := positions[number]
		pins = append(pins, transactions.PinSpec{Number: number, XMM: point.XMM, YMM: point.YMM, ExplicitOffset: true})
	}
	return pins
}

func esp32PeripheralHeaderPins() []transactions.PinSpec {
	pins := make([]transactions.PinSpec, 0, 6)
	for number := 1; number <= 6; number++ {
		pins = append(pins, transactions.PinSpec{
			Number:         fmt.Sprint(number),
			XMM:            -5.08,
			YMM:            -5.08 + float64(number-1)*2.54,
			ExplicitOffset: true,
		})
	}
	return pins
}

func esp32WROOM32EPhysicalPins() []transactions.PinSpec {
	pins := make([]transactions.PinSpec, 0, 39)
	appendPin := func(number int, xMM, yMM float64) {
		pins = append(pins, transactions.PinSpec{Number: fmt.Sprint(number), XMM: xMM, YMM: yMM, ExplicitOffset: true})
	}
	for number := 1; number <= 14; number++ {
		appendPin(number, -8.75, -5.26+float64(number-1)*1.27)
	}
	for number := 15; number <= 24; number++ {
		appendPin(number, -5.715+float64(number-15)*1.27, 12.5)
	}
	for number := 25; number <= 38; number++ {
		appendPin(number, 8.75, 11.25-float64(number-25)*1.27)
	}
	appendPin(39, -0.1, 2.46)
	return pins
}

func esp32WROOM32EPCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:              "0.1.0",
		VerificationLevel:    PCBVerificationDRCVerified,
		FabricationReadiness: true,
		Components: []PCBComponentRealization{
			{ComponentRole: "module", FootprintID: esp32WROOM32EFootprint, PhysicalPins: esp32WROOM32EPhysicalPins(), Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"}},
			{ComponentRole: "decoupling_capacitor", FootprintParam: "capacitor_footprint", Placement: RelativePlacement{XMM: -12, YMM: -4, RotationDeg: 180, Layer: "F.Cu"}},
			{ComponentRole: "bulk_capacitor", FootprintParam: "capacitor_footprint", Placement: RelativePlacement{XMM: -12, YMM: -1, RotationDeg: 180, Layer: "F.Cu"}},
			{ComponentRole: "enable_pullup", FootprintParam: "resistor_footprint", Placement: RelativePlacement{XMM: -12, YMM: 2, RotationDeg: 180, Layer: "F.Cu"}},
			{ComponentRole: "enable_capacitor", FootprintParam: "capacitor_footprint", Placement: RelativePlacement{XMM: -12, YMM: 5, RotationDeg: 180, Layer: "F.Cu"}},
			{ComponentRole: "boot_pullup", FootprintParam: "resistor_footprint", Placement: RelativePlacement{XMM: 22, YMM: 11, RotationDeg: 180, Layer: "F.Cu"}},
			{ComponentRole: "reset_switch", FootprintParam: "switch_footprint", Placement: RelativePlacement{XMM: -18, YMM: 2, Layer: "F.Cu"}},
			{ComponentRole: "boot_switch", FootprintParam: "switch_footprint", Placement: RelativePlacement{XMM: 28, YMM: 11, Layer: "F.Cu"}},
			{ComponentRole: "programming_header", FootprintID: esp32ProgrammingHeader, Placement: RelativePlacement{XMM: 14, YMM: 20, Layer: "F.Cu"}},
			{ComponentRole: "peripheral_header", FootprintID: esp32PeripheralHeader, Placement: RelativePlacement{XMM: 14, YMM: 2, Layer: "F.Cu"}},
		},
		EntryAnchors: []PCBEntryAnchor{
			{ID: "vcc", Port: "VCC", NetTemplate: "vcc", Placement: RelativePlacement{XMM: -18, YMM: -4, Layer: "F.Cu"}},
			{ID: "gnd", Port: "GND", NetTemplate: "gnd", Placement: RelativePlacement{XMM: -18, YMM: 6, Layer: "F.Cu"}},
			{ID: "gpio", Port: "GPIO", NetTemplate: "gpio", Placement: RelativePlacement{XMM: -12, YMM: 9, Layer: "F.Cu"}},
			{ID: "sda", Port: "SDA", NetTemplate: "sda", Placement: RelativePlacement{XMM: 18, YMM: 1, Layer: "F.Cu"}},
			{ID: "scl", Port: "SCL", NetTemplate: "scl", Placement: RelativePlacement{XMM: 18, YMM: -3, Layer: "F.Cu"}},
			{ID: "mosi", Port: "MOSI", NetTemplate: "mosi", Placement: RelativePlacement{XMM: 18, YMM: -5, Layer: "F.Cu"}},
			{ID: "miso", Port: "MISO", NetTemplate: "miso", Placement: RelativePlacement{XMM: 18, YMM: 4, Layer: "F.Cu"}},
			{ID: "sck", Port: "SCK", NetTemplate: "sck", Placement: RelativePlacement{XMM: 18, YMM: 6, Layer: "F.Cu"}},
		},
		PlacementGroups: []PCBPlacementGroup{{
			ID: "esp32_module_system", ComponentRoles: []string{"module", "decoupling_capacitor", "bulk_capacitor", "enable_pullup", "enable_capacitor", "boot_pullup", "reset_switch", "boot_switch", "programming_header", "peripheral_header"},
			AnchorRole: "module", Bounds: &RelativeBounds{MinXMM: -24, MinYMM: -13, MaxXMM: 33, MaxYMM: 27}, TranslateAsUnit: true,
			Description: "Keep the reviewed support network and antenna exclusion aligned with the module.",
		}},
		Keepouts: []PCBKeepout{{
			ID: "esp32_pcb_antenna_exclusion", Layer: "*.Cu", Bounds: RelativeBounds{MinXMM: -24, MinYMM: -28, MaxXMM: 24, MaxYMM: -6.56}, PlacementGroupID: "esp32_module_system",
			AppliesTo: []string{"module"}, BlocksRoute: boolPtr(true), Description: "Espressif antenna area: prohibit copper, vias, routes, and unrelated placement on every copper layer.",
		}},
		LocalRoutes: []PCBLocalRoute{
			{ID: "vcc_bypass", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "module", Pin: "2"}, To: RouteEndpoint{ComponentRole: "decoupling_capacitor", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
			{ID: "vcc_bulk", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "decoupling_capacitor", Pin: "1"}, To: RouteEndpoint{ComponentRole: "bulk_capacitor", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
			{ID: "vcc_enable", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "bulk_capacitor", Pin: "1"}, To: RouteEndpoint{ComponentRole: "enable_pullup", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.4, Required: true},
			{ID: "vcc_boot", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "enable_pullup", Pin: "1"}, To: RouteEndpoint{ComponentRole: "boot_pullup", Pin: "1"}, Waypoints: []RelativePoint{{XMM: -7, YMM: 2}, {XMM: -7, YMM: 28}, {XMM: 23.5, YMM: 28}, {XMM: 23.5, YMM: 11}}, Layer: "B.Cu", WidthMM: 0.4, Required: true},
			{ID: "vcc_programming", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "boot_pullup", Pin: "1"}, To: RouteEndpoint{ComponentRole: "programming_header", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 25, YMM: 11}, {XMM: 25, YMM: 16.19}}, Layer: "B.Cu", WidthMM: 0.4, Required: true},
			{ID: "gnd_bypass", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "module", Pin: "1"}, To: RouteEndpoint{ComponentRole: "decoupling_capacitor", Pin: "2"}, Waypoints: []RelativePoint{{XMM: -9, YMM: -6}, {XMM: -14, YMM: -6}, {XMM: -14, YMM: -4}}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
			{ID: "gnd_bulk", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "decoupling_capacitor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "bulk_capacitor", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
			{ID: "gnd_enable", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "bulk_capacitor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "enable_capacitor", Pin: "2"}, Waypoints: []RelativePoint{{XMM: -15, YMM: -1}, {XMM: -15, YMM: 5}}, Layer: "B.Cu", WidthMM: 0.4, Required: true},
			{ID: "gnd_reset", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "enable_capacitor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "reset_switch", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.4, Required: true},
			{ID: "gnd_module_left_to_exposed", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "module", Pin: "1"}, To: RouteEndpoint{ComponentRole: "module", Pin: "39"}, Waypoints: []RelativePoint{{XMM: -7, YMM: 0}}, Layer: "B.Cu", WidthMM: 0.5, Required: true},
			{ID: "gnd_module_bottom_to_exposed", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "module", Pin: "15"}, To: RouteEndpoint{ComponentRole: "module", Pin: "39"}, Layer: "B.Cu", WidthMM: 0.5, Required: true},
			{ID: "gnd_module_right_to_exposed", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "module", Pin: "38"}, To: RouteEndpoint{ComponentRole: "module", Pin: "39"}, Waypoints: []RelativePoint{{XMM: 8, YMM: -5.8}, {XMM: 0, YMM: -5.8}}, Layer: "B.Cu", WidthMM: 0.5, Required: true},
			{ID: "gnd_boot", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "module", Pin: "38"}, To: RouteEndpoint{ComponentRole: "boot_switch", Pin: "2"}, Waypoints: []RelativePoint{{XMM: 10, YMM: -6}, {XMM: 34, YMM: -6}, {XMM: 34, YMM: 12.85}}, Layer: "F.Cu", WidthMM: 0.4, Required: true},
			{ID: "gnd_programming", NetTemplate: "gnd", From: RouteEndpoint{ComponentRole: "boot_switch", Pin: "2"}, To: RouteEndpoint{ComponentRole: "programming_header", Pin: "2"}, Waypoints: []RelativePoint{{XMM: 24.9, YMM: 27}, {XMM: 12, YMM: 27}, {XMM: 12, YMM: 18.73}}, Layer: "F.Cu", WidthMM: 0.4, Required: true},
			{ID: "enable_pullup", NetTemplate: "enable", From: RouteEndpoint{ComponentRole: "enable_pullup", Pin: "2"}, To: RouteEndpoint{ComponentRole: "module", Pin: "3"}, Layer: "B.Cu", WidthMM: 0.25, Required: true},
			{ID: "enable_capacitor", NetTemplate: "enable", From: RouteEndpoint{ComponentRole: "enable_pullup", Pin: "2"}, To: RouteEndpoint{ComponentRole: "enable_capacitor", Pin: "1"}, Layer: "B.Cu", WidthMM: 0.25, Required: true},
			{ID: "enable_reset", NetTemplate: "enable", From: RouteEndpoint{ComponentRole: "enable_pullup", Pin: "2"}, To: RouteEndpoint{ComponentRole: "reset_switch", Pin: "1"}, Waypoints: []RelativePoint{{XMM: -11, YMM: 7}, {XMM: -22, YMM: 7}, {XMM: -22, YMM: 0.15}}, Layer: "B.Cu", WidthMM: 0.25, Required: true},
			{ID: "boot_pullup", NetTemplate: "boot", From: RouteEndpoint{ComponentRole: "boot_pullup", Pin: "2"}, To: RouteEndpoint{ComponentRole: "module", Pin: "25"}, Layer: "B.Cu", WidthMM: 0.25, Required: true},
			{ID: "boot_switch", NetTemplate: "boot", From: RouteEndpoint{ComponentRole: "boot_pullup", Pin: "2"}, To: RouteEndpoint{ComponentRole: "boot_switch", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 20.5, YMM: 11}, {XMM: 20.5, YMM: 4}, {XMM: 24.9, YMM: 4}}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "uart_rx_programming", NetTemplate: "uart_rx", From: RouteEndpoint{ComponentRole: "module", Pin: "34"}, To: RouteEndpoint{ComponentRole: "programming_header", Pin: "3"}, Waypoints: []RelativePoint{{XMM: 10, YMM: -0.18}, {XMM: 10, YMM: 11.5}, {XMM: 16, YMM: 11.5}, {XMM: 16, YMM: 21.27}}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "uart_tx_programming", NetTemplate: "uart_tx", From: RouteEndpoint{ComponentRole: "module", Pin: "35"}, To: RouteEndpoint{ComponentRole: "programming_header", Pin: "4"}, Waypoints: []RelativePoint{{XMM: 11, YMM: -1.45}, {XMM: 11, YMM: 10.5}, {XMM: 17, YMM: 10.5}, {XMM: 17, YMM: 23.81}}, Layer: "F.Cu", WidthMM: 0.25, Required: true},
			{ID: "mosi_peripheral", NetTemplate: "mosi", From: RouteEndpoint{ComponentRole: "module", Pin: "37"}, To: RouteEndpoint{ComponentRole: "peripheral_header", Pin: "1"}, Layer: "B.Cu", WidthMM: 0.25, Required: true},
			{ID: "scl_peripheral", NetTemplate: "scl", From: RouteEndpoint{ComponentRole: "module", Pin: "36"}, To: RouteEndpoint{ComponentRole: "peripheral_header", Pin: "2"}, Layer: "B.Cu", WidthMM: 0.25, Required: true},
			{ID: "sda_peripheral", NetTemplate: "sda", From: RouteEndpoint{ComponentRole: "module", Pin: "33"}, To: RouteEndpoint{ComponentRole: "peripheral_header", Pin: "3"}, Layer: "B.Cu", WidthMM: 0.25, Required: true},
			{ID: "miso_peripheral", NetTemplate: "miso", From: RouteEndpoint{ComponentRole: "module", Pin: "31"}, To: RouteEndpoint{ComponentRole: "peripheral_header", Pin: "4"}, Layer: "B.Cu", WidthMM: 0.25, Required: true},
			{ID: "sck_peripheral", NetTemplate: "sck", From: RouteEndpoint{ComponentRole: "module", Pin: "30"}, To: RouteEndpoint{ComponentRole: "peripheral_header", Pin: "5"}, Layer: "B.Cu", WidthMM: 0.25, Required: true},
			{ID: "gpio_peripheral", NetTemplate: "gpio", From: RouteEndpoint{ComponentRole: "module", Pin: "28"}, To: RouteEndpoint{ComponentRole: "peripheral_header", Pin: "6"}, Layer: "B.Cu", WidthMM: 0.25, Required: true},
		},
		Constraints: []PCBConstraint{
			{ID: "esp32_antenna_edge", Kind: "edge_facing", AppliesTo: []string{"module"}, Description: "Place the PCB antenna at a board edge with its exclusion region free of base-board copper and components."},
			{ID: "esp32_power_width", Kind: "min_width", NetTemplate: "vcc", MinWidthMM: 0.5, Description: "Use a low-impedance local 3.3 V feed for radio current transients."},
			{ID: "esp32_decoupling_proximity", Kind: "proximity", NetTemplate: "vcc", AppliesTo: []string{"module", "decoupling_capacitor", "bulk_capacitor"}, MaxLengthMM: 6, Description: "Keep high-frequency and bulk decoupling close to the module supply pad."},
			{ID: "esp32_enable_trace_short", Kind: "max_length", NetTemplate: "enable", AppliesTo: []string{"module", "enable_pullup", "enable_capacitor", "reset_switch"}, MaxLengthMM: 12, Description: "Keep the noise-sensitive EN network short."},
		},
		Validation: PCBValidationExpectations{
			RequiredNets:   []string{"vcc", "gnd", "enable", "boot", "uart_rx", "uart_tx", "gpio", "sda", "scl", "mosi", "miso", "sck"},
			RequiredRoutes: []string{"vcc_bypass", "vcc_bulk", "vcc_enable", "vcc_boot", "vcc_programming", "gnd_bypass", "gnd_bulk", "gnd_enable", "gnd_reset", "gnd_module_left_to_exposed", "gnd_module_bottom_to_exposed", "gnd_module_right_to_exposed", "gnd_boot", "gnd_programming", "enable_pullup", "enable_capacitor", "enable_reset", "boot_pullup", "boot_switch", "uart_rx_programming", "uart_tx_programming", "mosi_peripheral", "scl_peripheral", "sda_peripheral", "miso_peripheral", "sck_peripheral", "gpio_peripheral"},
			RequiresDRC:    true,
		},
	}
}
