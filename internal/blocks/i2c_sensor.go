package blocks

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

// i2cAddressPattern accepts normal 7-bit device addresses and excludes
// reserved ranges 0x00-0x07 and 0x78-0x7f.
var i2cAddressPattern = regexp.MustCompile(`^0x(0[8-9A-Fa-f]|[1-6][0-9A-Fa-f]|7[0-7])$`)

type i2cSensorPinRoleMap struct {
	VCC string
	GND string
	SDA string
	SCL string
	INT string
}

var genericI2CSensorPins = i2cSensorPinRoleMap{VCC: "1", GND: "2", SDA: "3", SCL: "4", INT: "5"}

type i2cSensorProfile struct {
	ComponentID   string
	Value         string
	SymbolID      string
	FootprintID   string
	Roles         i2cSensorPinRoleMap
	Pins          []transactions.PinSpec
	AdditionalVCC []string
	AdditionalGND []string
	AddressPin    string
	AddressLevels map[string]string
	ModeHigh      []string
	ModeLow       []string
	NoConnect     []string
}

var concreteI2CSensorProfiles = map[string]i2cSensorProfile{
	"sensor.bosch.bme280.lga8": {
		ComponentID: "sensor.bosch.bme280.lga8",
		Value:       "BME280",
		SymbolID:    "Sensor:BME280",
		FootprintID: "Package_LGA:Bosch_LGA-8_2.5x2.5mm_P0.65mm_ClockwisePinNumbering",
		Roles:       i2cSensorPinRoleMap{VCC: "8", GND: "1", SDA: "3", SCL: "4"},
		Pins: []transactions.PinSpec{
			{Number: "1", XMM: -2.54, YMM: -15.24}, {Number: "2", XMM: 15.24, YMM: -7.62},
			{Number: "3", XMM: 15.24, YMM: -2.54}, {Number: "4", XMM: 15.24, YMM: 2.54},
			{Number: "5", XMM: 15.24, YMM: 7.62}, {Number: "6", XMM: -2.54, YMM: 15.24},
			{Number: "7", XMM: 2.54, YMM: -15.24}, {Number: "8", XMM: 2.54, YMM: 15.24},
		},
		AdditionalVCC: []string{"6"}, AdditionalGND: []string{"7"},
		AddressPin:    "5",
		AddressLevels: map[string]string{"0x76": "low", "0x77": "high"},
		ModeHigh:      []string{"2"},
	},
	"sensor.bosch.bmp280.lga8": {
		ComponentID: "sensor.bosch.bmp280.lga8",
		Value:       "BMP280",
		SymbolID:    "Sensor_Pressure:BMP280",
		FootprintID: "Package_LGA:Bosch_LGA-8_2x2.5mm_P0.65mm_ClockwisePinNumbering",
		Roles:       i2cSensorPinRoleMap{VCC: "8", GND: "1", SDA: "3", SCL: "4"},
		Pins: []transactions.PinSpec{
			{Number: "1", XMM: 0, YMM: -7.62}, {Number: "2", XMM: -10.16, YMM: -2.54},
			{Number: "3", XMM: -10.16, YMM: 2.54}, {Number: "4", XMM: -10.16, YMM: 5.08},
			{Number: "5", XMM: -10.16, YMM: 0}, {Number: "6", XMM: 0, YMM: 10.16},
			{Number: "7", XMM: 2.54, YMM: -7.62}, {Number: "8", XMM: 2.54, YMM: 10.16},
		},
		AdditionalVCC: []string{"6"}, AdditionalGND: []string{"7"},
		AddressPin:    "5",
		AddressLevels: map[string]string{"0x76": "low", "0x77": "high"},
		ModeHigh:      []string{"2"},
	},
	"sensor.sensirion.sht31_dis.dfn8": {
		ComponentID: "sensor.sensirion.sht31_dis.dfn8",
		Value:       "SHT31-DIS",
		SymbolID:    "Sensor_Humidity:SHT31-DIS",
		FootprintID: "Sensor_Humidity:Sensirion_DFN-8-1EP_2.5x2.5mm_P0.5mm_EP1.1x1.7mm",
		Roles:       i2cSensorPinRoleMap{VCC: "5", GND: "8", SDA: "1", SCL: "4", INT: "3"},
		Pins: []transactions.PinSpec{
			{Number: "1", XMM: 10.16, YMM: 2.54}, {Number: "2", XMM: -10.16, YMM: 2.54},
			{Number: "3", XMM: 10.16, YMM: -2.54}, {Number: "4", XMM: 10.16, YMM: 0},
			{Number: "5", XMM: 0, YMM: 7.62}, {Number: "6", XMM: -10.16, YMM: 0},
			{Number: "7", XMM: -10.16, YMM: -2.54}, {Number: "8", XMM: 0, YMM: -7.62},
			{Number: "9", XMM: 0, YMM: -7.62},
		},
		AdditionalGND: []string{"9"},
		AddressPin:    "2",
		AddressLevels: map[string]string{"0x44": "low", "0x45": "high"},
		ModeLow:       []string{"7"},
		NoConnect:     []string{"6"},
	},
}

func instantiateI2CSensor(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	if _, ok := parseUnit(params["supply_voltage"], "V", voltageMultipliers()); !ok {
		issues = append(issues, blockIssue("params.supply_voltage", "supply_voltage must be a voltage literal"))
	}
	address := strings.ToLower(stringParam(params, "i2c_address"))
	if !i2cAddressPattern.MatchString(address) {
		issues = append(issues, blockIssue("params.i2c_address", "i2c_address must be a 7-bit hexadecimal address such as 0x48"))
	}
	if boolParam(params, "include_pullups", true) {
		if _, ok := parseUnit(params["pullup_value"], "Ω", resistanceMultipliers()); !ok {
			issues = append(issues, blockIssue("params.pullup_value", "pullup_value must be a resistance literal"))
		}
		if stringParam(params, "pullup_footprint") == "" {
			issues = append(issues, blockIssue("params.pullup_footprint", "pullup_footprint is required"))
		}
	}
	if boolParam(params, "include_decoupling", true) {
		if _, ok := parseUnit(params["decoupling_value"], "F", capacitanceMultipliers()); !ok {
			issues = append(issues, blockIssue("params.decoupling_value", "decoupling_value must be a capacitance literal"))
		}
		if stringParam(params, "decoupling_footprint") == "" {
			issues = append(issues, blockIssue("params.decoupling_footprint", "decoupling_footprint is required"))
		}
	}
	componentID := strings.TrimSpace(stringParam(params, "sensor_component_id"))
	profile, concreteProfile := concreteI2CSensorProfiles[componentID]
	if componentID != "" && !concreteProfile {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityBlocked,
			Path:       "params.sensor_component_id",
			Message:    "i2c_sensor does not have a verified topology profile for " + componentID,
			Suggestion: "use a component ID listed in the i2c_sensor block documentation",
		})
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	addressLevel := ""
	roles := genericI2CSensorPins
	pins := i2cSensorPins(genericI2CSensorPins)
	sensorValue := "I2C Sensor"
	sensorSymbol := stringParam(params, "sensor_symbol")
	sensorFootprint := stringParam(params, "sensor_footprint")
	if concreteProfile {
		roles = profile.Roles
		pins = append([]transactions.PinSpec(nil), profile.Pins...)
		sensorValue = profile.Value
		var addressSupported bool
		addressLevel, addressSupported = profile.AddressLevels[address]
		if !addressSupported {
			issues = append(issues, blockIssue("params.i2c_address", "i2c_address is not supported by "+componentID))
		}
		if sensorSymbol != "" && sensorSymbol != defaultI2CSensorSymbol && sensorSymbol != profile.SymbolID {
			issues = append(issues, blockIssue("params.sensor_symbol", "sensor_symbol does not match the selected sensor component"))
		}
		if sensorFootprint != "" && sensorFootprint != "Package_SO:SOIC-8_3.9x4.9mm_P1.27mm" && sensorFootprint != profile.FootprintID {
			issues = append(issues, blockIssue("params.sensor_footprint", "sensor_footprint does not match the selected sensor component"))
		}
		sensorSymbol = profile.SymbolID
		sensorFootprint = profile.FootprintID
		if boolParam(params, "include_interrupt", false) && roles.INT == "" {
			issues = append(issues, blockIssue("params.include_interrupt", "selected sensor does not expose a verified interrupt pin"))
		}
	} else {
		switch {
		case sensorSymbol == "":
			issues = append(issues, blockIssue("params.sensor_symbol", "sensor_symbol is required"))
		case sensorSymbol != defaultI2CSensorSymbol:
			issues = append(issues, reports.Issue{
				Code:       reports.CodeUnsupportedOperation,
				Severity:   reports.SeverityBlocked,
				Path:       "params.sensor_symbol",
				Message:    "custom sensor symbols require a verified sensor_component_id topology profile",
				Suggestion: "use " + defaultI2CSensorSymbol + " or select a verified concrete sensor component",
			})
		}
		if sensorFootprint == "" {
			issues = append(issues, blockIssue("params.sensor_footprint", "sensor_footprint is required"))
		}
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	instanceParams := cloneAnyParams(params)
	instanceParams["i2c_address"] = address
	instanceParams["sensor_symbol"] = sensorSymbol
	instanceParams["sensor_footprint"] = sensorFootprint

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	sensorRef := allocator.Next("U")
	sensor := BlockComponent{
		Role:        "sensor",
		RefPrefix:   "U",
		Value:       sensorValue + " " + address,
		SymbolID:    sensorSymbol,
		FootprintID: sensorFootprint,
		Pins:        pins,
	}
	var issuesOut []reports.Issue
	issuesOut = append(issuesOut, issues...)
	var operations []transactions.Operation
	sensorOps, sensorIssues := ComponentOperations(sensor, sensorRef, transactions.Point{XMM: 10, YMM: 0})
	issuesOut = append(issuesOut, sensorIssues...)
	operations = append(operations, sensorOps...)

	vccNet := InstanceNetName(request.InstanceID, "vcc")
	gndNet := InstanceNetName(request.InstanceID, "gnd")
	sdaNet := InstanceNetName(request.InstanceID, "sda")
	sclNet := InstanceNetName(request.InstanceID, "scl")
	appendPortConnection := appendConnectOperation
	if concreteProfile {
		appendPortConnection = appendLabeledConnectOperation
	}
	appendPortConnection(&operations, &issuesOut, sensorRef, roles.VCC, request.InstanceID, "VCC", vccNet)
	appendPortConnection(&operations, &issuesOut, sensorRef, roles.GND, request.InstanceID, "GND", gndNet)
	appendPortConnection(&operations, &issuesOut, sensorRef, roles.SDA, request.InstanceID, "SDA", sdaNet)
	appendPortConnection(&operations, &issuesOut, sensorRef, roles.SCL, request.InstanceID, "SCL", sclNet)
	if concreteProfile {
		// Only verified concrete profiles define auxiliary supply, address,
		// mode, and unused pins. The generic topology intentionally exposes
		// only its standard VCC/GND/SDA/SCL/INT contract.
		for _, pin := range profile.AdditionalVCC {
			appendLabeledConnectOperation(&operations, &issuesOut, sensorRef, pin, request.InstanceID, "VCC", vccNet)
		}
		for _, pin := range profile.AdditionalGND {
			appendLabeledConnectOperation(&operations, &issuesOut, sensorRef, pin, request.InstanceID, "GND", gndNet)
		}
		appendSensorPinLevel(&operations, &issuesOut, request.InstanceID, sensorRef, profile.AddressPin, addressLevel, vccNet, gndNet)
		for _, pin := range profile.ModeHigh {
			appendSensorPinLevel(&operations, &issuesOut, request.InstanceID, sensorRef, pin, "high", vccNet, gndNet)
		}
		for _, pin := range profile.ModeLow {
			appendSensorPinLevel(&operations, &issuesOut, request.InstanceID, sensorRef, pin, "low", vccNet, gndNet)
		}
	}

	refs := []string{sensorRef}
	nets := []string{vccNet, gndNet, sdaNet, sclNet}
	if boolParam(params, "include_interrupt", false) {
		appendPortConnection(&operations, &issuesOut, sensorRef, roles.INT, request.InstanceID, "INT", InstanceNetName(request.InstanceID, "int"))
		nets = append(nets, InstanceNetName(request.InstanceID, "int"))
	} else if roles.INT != "" {
		appendNoConnectOperation(&operations, &issuesOut, sensorRef, roles.INT)
	}
	if concreteProfile {
		for _, pin := range profile.NoConnect {
			appendNoConnectOperation(&operations, &issuesOut, sensorRef, pin)
		}
	}
	if boolParam(params, "include_decoupling", true) {
		capRef := allocator.Next("C")
		capacitor := BlockComponent{Role: "decoupling_capacitor", RefPrefix: "C", Value: stringParam(params, "decoupling_value"), SymbolID: "Device:C", FootprintID: stringParam(params, "decoupling_footprint"), Pins: deviceCTemplatePins()}
		capOps, capIssues := ComponentOperations(capacitor, capRef, transactions.Point{XMM: 0, YMM: 10})
		issuesOut = append(issuesOut, capIssues...)
		operations = append(operations, capOps...)
		appendConnectOperation(&operations, &issuesOut, capRef, "1", sensorRef, roles.VCC, vccNet)
		appendConnectOperation(&operations, &issuesOut, capRef, "2", sensorRef, roles.GND, gndNet)
		refs = append(refs, capRef)
	}
	if boolParam(params, "include_pullups", true) {
		for _, pullup := range []struct {
			role string
			net  string
			pin  string
			xmm  float64
		}{
			{role: "sda_pullup", net: sdaNet, pin: roles.SDA, xmm: 25},
			{role: "scl_pullup", net: sclNet, pin: roles.SCL, xmm: 35},
		} {
			ref := allocator.Next("R")
			component := BlockComponent{Role: pullup.role, RefPrefix: "R", Value: stringParam(params, "pullup_value"), SymbolID: "Device:R", FootprintID: stringParam(params, "pullup_footprint"), Pins: deviceRTemplatePins()}
			pullupOps, pullupIssues := ComponentOperations(component, ref, transactions.Point{XMM: pullup.xmm, YMM: -10})
			issuesOut = append(issuesOut, pullupIssues...)
			operations = append(operations, pullupOps...)
			appendConnectOperation(&operations, &issuesOut, ref, "1", sensorRef, roles.VCC, vccNet)
			appendConnectOperation(&operations, &issuesOut, ref, "2", sensorRef, pullup.pin, pullup.net)
			refs = append(refs, ref)
		}
	}

	output := dryRunBlockOutput(definition, request, operations, issuesOut)
	output.Instance.Params = instanceParams
	output.Instance.Ports = resolvePortVoltages(i2cSensorPorts(definition.Ports, boolParam(params, "include_interrupt", false)), instanceParams)
	output.Instance.Refs = refs
	output.Instance.Nets = nets
	return output
}

func appendSensorPinLevel(operations *[]transactions.Operation, issues *[]reports.Issue, instanceID string, ref string, pin string, level string, vccNet string, gndNet string) {
	if pin == "" || level == "" {
		return
	}
	if level == "high" {
		appendLabeledConnectOperation(operations, issues, ref, pin, instanceID, "VCC", vccNet)
		return
	}
	appendLabeledConnectOperation(operations, issues, ref, pin, instanceID, "GND", gndNet)
}

func appendLabeledConnectOperation(operations *[]transactions.Operation, issues *[]reports.Issue, fromRef string, fromPin string, toRef string, toPin string, netName string) {
	if fromRef == "" || fromPin == "" || toRef == "" || toPin == "" || netName == "" {
		*issues = append(*issues, blockIssue("connect", "connect operation requires from ref/pin, to ref/pin, and net name"))
		return
	}
	useLabels := true
	operation, err := wrapOperation(transactions.OpConnect, transactions.ConnectOperation{
		Op:        transactions.OpConnect,
		From:      transactions.Endpoint{Ref: fromRef, Pin: fromPin},
		To:        transactions.Endpoint{Ref: toRef, Pin: toPin},
		NetName:   netName,
		UseLabels: &useLabels,
	})
	if err != nil {
		*issues = append(*issues, blockIssue("connect", err.Error()))
		return
	}
	*operations = append(*operations, operation)
}

func i2cSensorPins(roles i2cSensorPinRoleMap) []transactions.PinSpec {
	// These are physical schematic connection anchors, not the raw local-library
	// pin coordinates. Sensor:Generic_I2C serializes pins 1-4 with an inverted
	// Y axis; the embedded-template calibration maps them back to KiCad's
	// physical wire endpoints.
	return []transactions.PinSpec{
		{Number: roles.VCC, XMM: -2.54, YMM: 3.81},
		{Number: roles.GND, XMM: -2.54, YMM: -3.81},
		{Number: roles.SDA, XMM: -2.54, YMM: 1.27},
		{Number: roles.SCL, XMM: -2.54, YMM: -1.27},
		{Number: roles.INT, XMM: 2.54, YMM: 0},
	}
}

func i2cSensorPorts(base []BlockPort, includeInterrupt bool) []BlockPort {
	ports := make([]BlockPort, 0, len(base))
	for _, port := range base {
		if port.Name == "INT" && !includeInterrupt {
			continue
		}
		ports = append(ports, port)
	}
	return ports
}

func i2cAddressKey(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return i2cAddressStringKey(typed)
	case fmt.Stringer:
		return i2cAddressStringKey(typed.String())
	default:
		number, ok := numericValue(value)
		if !ok || number != float64(int(number)) || number < 0x08 || number > 0x77 {
			return ""
		}
		return formatI2CAddressKey(int(number))
	}
}

func i2cAddressStringKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	number, err := strconv.ParseInt(value, 0, 64)
	if err != nil {
		return ""
	}
	if number < 0x08 || number > 0x77 {
		return ""
	}
	return formatI2CAddressKey(int(number))
}

func formatI2CAddressKey(value int) string {
	return fmt.Sprintf("0x%02x", value)
}

func sameI2CBus(a BlockInstance, b BlockInstance, netGroups portDisjointSet) bool {
	return netGroups.find(PortRef{InstanceID: a.InstanceID, Port: "SDA"}) == netGroups.find(PortRef{InstanceID: b.InstanceID, Port: "SDA"}) ||
		netGroups.find(PortRef{InstanceID: a.InstanceID, Port: "SCL"}) == netGroups.find(PortRef{InstanceID: b.InstanceID, Port: "SCL"})
}

func i2cAddressCollisionIssue(left BlockInstance, right BlockInstance, address string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     fmt.Sprintf("instances.%s.params.i2c_address", right.InstanceID),
		Message:  fmt.Sprintf("duplicate I2C address %s on shared bus between %s and %s", address, left.InstanceID, right.InstanceID),
	}
}

func cloneAnyParams(params map[string]any) map[string]any {
	clone := make(map[string]any, len(params))
	for key, value := range params {
		clone[key] = cloneParameterValue(value)
	}
	return clone
}
