package intentplanner

import (
	"fmt"
	"sort"
	"strings"

	"kicadai/internal/blocks"
)

type semanticIndex struct {
	instances map[string]semanticInstance
}

type semanticInstance struct {
	ID            string
	BlockID       string
	Role          string
	Params        map[string]any
	SupplyVoltage string
	Ports         []semanticPort
}

type semanticPort struct {
	Name      string
	Role      string
	Direction blocks.PortDirection
	Voltage   string
	Bus       string
}

func newSemanticIndex() *semanticIndex {
	return &semanticIndex{instances: map[string]semanticInstance{}}
}

func (index *semanticIndex) addInstance(id string, role string, blockID string, params map[string]any, definition blocks.BlockDefinition) {
	if index == nil || strings.TrimSpace(id) == "" {
		return
	}
	instance := semanticInstance{
		ID:            strings.TrimSpace(id),
		BlockID:       strings.TrimSpace(blockID),
		Role:          semanticRole(role, blockID),
		Params:        cloneParams(params),
		SupplyVoltage: semanticSupplyVoltage(params, definition),
		Ports:         semanticPortsForBlock(blockID, params, definition),
	}
	sort.Slice(instance.Ports, func(i, j int) bool {
		if instance.Ports[i].Role != instance.Ports[j].Role {
			return instance.Ports[i].Role < instance.Ports[j].Role
		}
		return instance.Ports[i].Name < instance.Ports[j].Name
	})
	index.instances[instance.ID] = instance
}

func (index *semanticIndex) instance(id string) (semanticInstance, bool) {
	if index == nil {
		return semanticInstance{}, false
	}
	instance, ok := index.instances[strings.TrimSpace(id)]
	return instance, ok
}

func (index *semanticIndex) byRole(role string) []semanticInstance {
	if index == nil {
		return nil
	}
	role = normalizeToken(role)
	var out []semanticInstance
	for _, instance := range index.instances {
		if instance.Role == role {
			out = append(out, instance)
		}
	}
	sortSemanticInstances(out)
	return out
}

func (index *semanticIndex) withPortRole(role string) []semanticInstance {
	if index == nil {
		return nil
	}
	role = normalizeToken(role)
	var out []semanticInstance
	for _, instance := range index.instances {
		if instance.hasPortRole(role) {
			out = append(out, instance)
		}
	}
	sortSemanticInstances(out)
	return out
}

func (instance semanticInstance) hasPortRole(role string) bool {
	role = normalizeToken(role)
	for _, port := range instance.Ports {
		if port.Role == role {
			return true
		}
	}
	return false
}

func (instance semanticInstance) portByRole(role string) (semanticPort, bool) {
	role = normalizeToken(role)
	for _, port := range instance.Ports {
		if port.Role == role {
			return port, true
		}
	}
	return semanticPort{}, false
}

func sortSemanticInstances(values []semanticInstance) {
	sort.Slice(values, func(i, j int) bool {
		return values[i].ID < values[j].ID
	})
}

func semanticRole(role string, blockID string) string {
	role = normalizeToken(role)
	if role != "" {
		switch role {
		case "i2c_connector", "gpio_connector", "signal_connector", "power_connector":
			return "connector"
		case "programming":
			return "programming"
		default:
			return role
		}
	}
	switch blockID {
	case "mcu_minimal":
		return "mcu"
	case "i2c_sensor":
		return "sensor"
	case "connector_breakout":
		return "connector"
	case "reset_programming_header":
		return "programming"
	case "crystal_oscillator", "canned_oscillator":
		return "clock"
	case "usb_c_power", "voltage_regulator", "reverse_polarity_protection":
		return "power"
	default:
		return normalizeToken(blockID)
	}
}

func semanticSupplyVoltage(params map[string]any, definition blocks.BlockDefinition) string {
	for _, key := range []string{"supply_voltage", "output_voltage", "input_voltage", "working_voltage"} {
		if value := semanticParamString(params, definition, key); value != "" {
			return value
		}
	}
	for _, port := range definition.Ports {
		if port.Direction == blocks.PortPower && port.Voltage != "" && !strings.EqualFold(port.Name, "GND") {
			return resolveSemanticVoltage(port.Voltage, params, definition)
		}
	}
	return ""
}

func semanticPortsForBlock(blockID string, params map[string]any, definition blocks.BlockDefinition) []semanticPort {
	switch blockID {
	case "mcu_minimal":
		return mcuSemanticPorts(params, definition)
	case "i2c_sensor":
		return i2cSemanticPorts(definition)
	case "connector_breakout":
		return connectorSemanticPorts(params, definition)
	case "reset_programming_header":
		return programmingSemanticPorts(params, definition)
	case "crystal_oscillator":
		return crystalSemanticPorts(definition)
	case "canned_oscillator":
		return cannedOscillatorSemanticPorts(definition)
	default:
		return genericSemanticPorts(params, definition)
	}
}

func mcuSemanticPorts(params map[string]any, definition blocks.BlockDefinition) []semanticPort {
	ports := genericSemanticPorts(params, definition)
	roles := map[string]string{
		"VCC":     "power.vcc",
		"GND":     "power.gnd",
		"RESET":   "mcu.reset",
		"AREF":    "mcu.aref",
		"GPIO":    "mcu.gpio",
		"MOSI":    "mcu.spi.mosi",
		"MISO":    "mcu.spi.miso",
		"SCK":     "mcu.spi.sck",
		"UART_TX": "mcu.uart.tx",
		"UART_RX": "mcu.uart.rx",
	}
	ports = applyPortRoles(ports, roles)
	if _, ok := findSemanticPort(ports, "mcu.i2c.sda"); !ok {
		ports = append(ports, semanticPort{Name: "SDA", Role: "mcu.i2c.sda", Direction: blocks.PortBidirectional, Voltage: semanticSupplyVoltage(params, definition), Bus: "i2c"})
	}
	if _, ok := findSemanticPort(ports, "mcu.i2c.scl"); !ok {
		ports = append(ports, semanticPort{Name: "SCL", Role: "mcu.i2c.scl", Direction: blocks.PortBidirectional, Voltage: semanticSupplyVoltage(params, definition), Bus: "i2c"})
	}
	if _, ok := findSemanticPort(ports, "mcu.clock.xtal1"); !ok {
		ports = append(ports, semanticPort{Name: "XTAL1", Role: "mcu.clock.xtal1", Direction: blocks.PortInput})
	}
	if _, ok := findSemanticPort(ports, "mcu.clock.xtal2"); !ok {
		ports = append(ports, semanticPort{Name: "XTAL2", Role: "mcu.clock.xtal2", Direction: blocks.PortOutput})
	}
	return ports
}

func i2cSemanticPorts(definition blocks.BlockDefinition) []semanticPort {
	return applyPortRoles(genericSemanticPorts(nil, definition), map[string]string{
		"SDA": "i2c.sda",
		"SCL": "i2c.scl",
		"VCC": "power.vcc",
		"GND": "power.gnd",
	})
}

func connectorSemanticPorts(params map[string]any, definition blocks.BlockDefinition) []semanticPort {
	ports := genericSemanticPorts(params, definition)
	for index := range ports {
		name := strings.ToUpper(ports[index].Name)
		switch name {
		case "SDA":
			ports[index].Role = "i2c.sda"
			ports[index].Bus = "i2c"
		case "SCL":
			ports[index].Role = "i2c.scl"
			ports[index].Bus = "i2c"
		case "VCC", "VDD", "VIN":
			ports[index].Role = "power.vcc"
		case "GND":
			ports[index].Role = "power.gnd"
		case "SIG":
			ports[index].Role = "signal"
		}
	}
	return ports
}

func programmingSemanticPorts(params map[string]any, definition blocks.BlockDefinition) []semanticPort {
	return applyPortRoles(genericSemanticPorts(params, definition), map[string]string{
		"RESET":   "mcu.reset",
		"MOSI":    "mcu.spi.mosi",
		"MISO":    "mcu.spi.miso",
		"SCK":     "mcu.spi.sck",
		"UART_TX": "mcu.uart.tx",
		"UART_RX": "mcu.uart.rx",
		"VCC":     "power.vcc",
		"GND":     "power.gnd",
	})
}

func crystalSemanticPorts(definition blocks.BlockDefinition) []semanticPort {
	return applyPortRoles(genericSemanticPorts(nil, definition), map[string]string{
		"XTAL1": "clock.xtal1",
		"XTAL2": "clock.xtal2",
		"GND":   "power.gnd",
	})
}

func cannedOscillatorSemanticPorts(definition blocks.BlockDefinition) []semanticPort {
	return applyPortRoles(genericSemanticPorts(nil, definition), map[string]string{
		"CLK_OUT": "clock.output",
		"VCC":     "power.vcc",
		"GND":     "power.gnd",
		"EN":      "clock.enable",
	})
}

func genericSemanticPorts(params map[string]any, definition blocks.BlockDefinition) []semanticPort {
	var ports []semanticPort
	for _, port := range definition.Ports {
		ports = append(ports, semanticPort{
			Name:      port.Name,
			Role:      defaultPortRole(port),
			Direction: port.Direction,
			Voltage:   resolveSemanticVoltage(port.Voltage, params, definition),
		})
	}
	for _, pin := range stringListParam(params["pin_names"]) {
		if !hasSemanticPortName(ports, pin) {
			direction := blocks.PortPassive
			role := ""
			switch strings.ToUpper(pin) {
			case "VCC", "VDD", "VIN":
				direction = blocks.PortPower
				role = "power.vcc"
			case "GND":
				direction = blocks.PortPower
				role = "power.gnd"
			}
			ports = append(ports, semanticPort{Name: pin, Role: role, Direction: direction})
		}
	}
	return ports
}

func applyPortRoles(ports []semanticPort, roles map[string]string) []semanticPort {
	for index := range ports {
		if role := roles[strings.ToUpper(ports[index].Name)]; role != "" {
			ports[index].Role = role
			if strings.HasPrefix(role, "i2c.") || strings.Contains(role, ".i2c.") {
				ports[index].Bus = "i2c"
			}
		}
	}
	return ports
}

func defaultPortRole(port blocks.BlockPort) string {
	name := strings.ToUpper(port.Name)
	switch {
	case name == "GND":
		return "power.gnd"
	case name == "VCC" || name == "VDD" || name == "VIN" || strings.Contains(name, "VOUT") || strings.Contains(name, "VBUS"):
		return "power.vcc"
	case port.Direction == blocks.PortPower:
		return "power"
	default:
		return ""
	}
}

func resolveSemanticVoltage(value string, params map[string]any, definition blocks.BlockDefinition) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, ok := parseVoltage(value); ok {
		return fmt.Sprintf("%gV", parsed)
	}
	if param := semanticParamString(params, definition, value); param != "" {
		return param
	}
	return value
}

func semanticParamString(params map[string]any, definition blocks.BlockDefinition, key string) string {
	if params != nil {
		if value, ok := params[key]; ok && value != nil {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	for _, parameter := range definition.Parameters {
		if parameter.Name == key && parameter.Default != nil {
			return strings.TrimSpace(fmt.Sprint(parameter.Default))
		}
	}
	return ""
}

func findSemanticPort(ports []semanticPort, role string) (semanticPort, bool) {
	role = normalizeToken(role)
	for _, port := range ports {
		if port.Role == role {
			return port, true
		}
	}
	return semanticPort{}, false
}

func hasSemanticPortName(ports []semanticPort, name string) bool {
	for _, port := range ports {
		if strings.EqualFold(port.Name, name) {
			return true
		}
	}
	return false
}
