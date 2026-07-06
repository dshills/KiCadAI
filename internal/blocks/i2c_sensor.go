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
	sensorSymbol := stringParam(params, "sensor_symbol")
	switch {
	case sensorSymbol == "":
		issues = append(issues, blockIssue("params.sensor_symbol", "sensor_symbol is required"))
	case sensorSymbol != defaultI2CSensorSymbol:
		issues = append(issues, reports.Issue{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityBlocked,
			Path:       "params.sensor_symbol",
			Message:    "i2c_sensor currently supports only the generic I2C sensor template pinout",
			Suggestion: "use " + defaultI2CSensorSymbol + " or wait for sensor pin-role map support",
		})
	}
	sensorFootprint := stringParam(params, "sensor_footprint")
	if sensorFootprint == "" {
		issues = append(issues, blockIssue("params.sensor_footprint", "sensor_footprint is required"))
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	instanceParams := cloneAnyParams(params)
	instanceParams["i2c_address"] = address

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	sensorRef := allocator.Next("U")
	sensor := BlockComponent{
		Role:        "sensor",
		RefPrefix:   "U",
		Value:       "I2C Sensor " + address,
		SymbolID:    sensorSymbol,
		FootprintID: sensorFootprint,
		Pins:        i2cSensorPins(genericI2CSensorPins),
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
	appendConnectOperation(&operations, &issuesOut, request.InstanceID, "VCC", sensorRef, genericI2CSensorPins.VCC, vccNet)
	appendConnectOperation(&operations, &issuesOut, sensorRef, genericI2CSensorPins.GND, request.InstanceID, "GND", gndNet)
	appendConnectOperation(&operations, &issuesOut, request.InstanceID, "SDA", sensorRef, genericI2CSensorPins.SDA, sdaNet)
	appendConnectOperation(&operations, &issuesOut, request.InstanceID, "SCL", sensorRef, genericI2CSensorPins.SCL, sclNet)

	refs := []string{sensorRef}
	nets := []string{vccNet, gndNet, sdaNet, sclNet}
	if boolParam(params, "include_interrupt", false) {
		appendConnectOperation(&operations, &issuesOut, sensorRef, genericI2CSensorPins.INT, request.InstanceID, "INT", InstanceNetName(request.InstanceID, "int"))
		nets = append(nets, InstanceNetName(request.InstanceID, "int"))
	} else {
		appendNoConnectOperation(&operations, &issuesOut, sensorRef, genericI2CSensorPins.INT)
	}
	if boolParam(params, "include_decoupling", true) {
		capRef := allocator.Next("C")
		capacitor := BlockComponent{Role: "decoupling_capacitor", RefPrefix: "C", Value: stringParam(params, "decoupling_value"), SymbolID: "Device:C", FootprintID: stringParam(params, "decoupling_footprint"), Pins: twoTerminalHorizontalPins()}
		capOps, capIssues := ComponentOperations(capacitor, capRef, transactions.Point{XMM: 0, YMM: 10})
		issuesOut = append(issuesOut, capIssues...)
		operations = append(operations, capOps...)
		appendConnectOperation(&operations, &issuesOut, capRef, "1", sensorRef, genericI2CSensorPins.VCC, vccNet)
		appendConnectOperation(&operations, &issuesOut, capRef, "2", sensorRef, genericI2CSensorPins.GND, gndNet)
		refs = append(refs, capRef)
	}
	if boolParam(params, "include_pullups", true) {
		for _, pullup := range []struct {
			role string
			net  string
			pin  string
			xmm  float64
		}{
			{role: "sda_pullup", net: sdaNet, pin: genericI2CSensorPins.SDA, xmm: 25},
			{role: "scl_pullup", net: sclNet, pin: genericI2CSensorPins.SCL, xmm: 35},
		} {
			ref := allocator.Next("R")
			component := BlockComponent{Role: pullup.role, RefPrefix: "R", Value: stringParam(params, "pullup_value"), SymbolID: "Device:R", FootprintID: stringParam(params, "pullup_footprint"), Pins: deviceRTemplatePins()}
			pullupOps, pullupIssues := ComponentOperations(component, ref, transactions.Point{XMM: pullup.xmm, YMM: -10})
			issuesOut = append(issuesOut, pullupIssues...)
			operations = append(operations, pullupOps...)
			appendConnectOperation(&operations, &issuesOut, ref, "1", sensorRef, genericI2CSensorPins.VCC, vccNet)
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

func i2cSensorPins(roles i2cSensorPinRoleMap) []transactions.PinSpec {
	return []transactions.PinSpec{
		{Number: roles.VCC, XMM: -2.54, YMM: -3.81},
		{Number: roles.GND, XMM: -2.54, YMM: 3.81},
		{Number: roles.SDA, XMM: -2.54, YMM: -1.27},
		{Number: roles.SCL, XMM: -2.54, YMM: 1.27},
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
