package architecturesearch

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

func TestSolveMCUAssignmentAllocatesMixedSTM32Peripherals(t *testing.T) {
	record := architectureMCURecord(t, "mcu.st.stm32g031k8t6.lqfp32")
	demands := []mcuRoleDemand{
		{Role: "sensor_bus", Kind: "i2c", Signals: []string{"scl", "sda"}, Mode: "open_drain"},
		{Role: "storage_bus", Kind: "spi", Signals: []string{"cs", "miso", "mosi", "sck"}},
		{Role: "console", Kind: "uart", Signals: []string{"rx", "tx"}},
		{Role: "control", Kind: "pwm"},
		{Role: "measurement", Kind: "adc"},
		{Role: "alarm", Kind: "interrupt", InterruptOnly: true},
	}
	assignment, err := solveMCUAssignment(record, demands, "swd", "internal_hsi_pll")
	if err != nil {
		t.Fatal(err)
	}
	if assignment.ProgrammingInterface.ID != "swd" || assignment.ClockOption.ID != "internal_hsi_pll" {
		t.Fatalf("unexpected infrastructure selection: %#v", assignment)
	}
	assertMCUBundleInstance(t, assignment.Pins, "sensor_bus", 2)
	assertMCUBundleInstance(t, assignment.Pins, "storage_bus", 4)
	assertMCUBundleInstance(t, assignment.Pins, "console", 2)
	used := map[string]bool{}
	for _, pin := range assignment.Pins {
		if used[pin.Function] {
			t.Fatalf("physical pin %s assigned more than once: %#v", pin.Function, assignment.Pins)
		}
		used[pin.Function] = true
		if pin.Function == "PA13" || pin.Function == "PA14" || pin.Function == "PF2" {
			t.Fatalf("SWD/reset pin was consumed by an application role: %#v", pin)
		}
		if pin.PackagePad == "" {
			t.Fatalf("assignment lacks package-pad evidence: %#v", pin)
		}
	}
}

func TestSolveMCUAssignmentIsStableUnderShuffledEvidence(t *testing.T) {
	left := architectureMCURecord(t, "mcu.st.stm32g031k8t6.lqfp32")
	right := cloneMCURecord(t, left)
	slices.Reverse(right.MCU.Pins)
	for index := range right.MCU.Pins {
		slices.Reverse(right.MCU.Pins[index].AlternateFunctions)
	}
	demands := []mcuRoleDemand{{Role: "bus", Kind: "i2c", Signals: []string{"scl", "sda"}, Mode: "open_drain"}, {Role: "console", Kind: "uart", Signals: []string{"rx", "tx"}}, {Role: "analog", Kind: "adc"}}
	reversedDemands := slices.Clone(demands)
	slices.Reverse(reversedDemands)
	leftAssignment, leftErr := solveMCUAssignment(left, demands, "swd", "")
	rightAssignment, rightErr := solveMCUAssignment(right, reversedDemands, "swd", "")
	if leftErr != nil || rightErr != nil {
		t.Fatalf("assignment failed: left=%v right=%v", leftErr, rightErr)
	}
	if !reflect.DeepEqual(leftAssignment, rightAssignment) {
		t.Fatalf("assignment changed with input ordering:\nleft=%#v\nright=%#v", leftAssignment, rightAssignment)
	}
}

func TestSolveMCUAssignmentUsesGPIOForAVRSPICardSelect(t *testing.T) {
	record := architectureMCURecord(t, "mcu.microchip.atmega328p_a.tqfp32")
	assignment, err := solveMCUAssignment(record, []mcuRoleDemand{{Role: "spi_bus", Kind: "spi", Signals: []string{"cs", "miso", "mosi", "sck"}}}, "isp", "internal_rc")
	if err != nil {
		t.Fatal(err)
	}
	assertMCUBundleInstance(t, assignment.Pins, "spi_bus", 4)
	for _, pin := range assignment.Pins {
		if pin.Lane == "cs" && pin.Kind != "gpio" {
			t.Fatalf("AVR chip select should be allocated from GPIO: %#v", pin)
		}
	}
}

func TestMCUPinModeCompatibleRequiresPushPullForAnySource(t *testing.T) {
	inputOnly := components.MCUPinEvidence{ElectricalModes: []string{"input"}}
	if mcuPinModeCompatible(inputOnly, mcuRoleDemand{Kind: "uart", Direction: "source"}) {
		t.Fatal("input-only pin accepted for a driven UART role")
	}
	pushPull := components.MCUPinEvidence{ElectricalModes: []string{"input", "push_pull"}}
	if !mcuPinModeCompatible(pushPull, mcuRoleDemand{Kind: "uart", Direction: "source"}) {
		t.Fatal("push-pull pin rejected for a driven UART role")
	}
}

func TestMCUProtocolSignalsFollowDirectionAndTraits(t *testing.T) {
	uartTX := providerRole("telemetry", "digital_bus", "source", 0, 3.3).Contract
	uartTX.Protocol = &Protocol{Name: "uart", Mode: "push_pull"}
	if got, want := mcuProtocolSignals(uartTX, "uart"), []string{"tx"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UART source signals = %#v, want %#v", got, want)
	}
	uartFlow := uartTX
	uartFlow.Direction = "bidirectional"
	uartFlow.RequiredTraits = []string{"hardware_flow_control"}
	if got, want := mcuProtocolSignals(uartFlow, "uart"), []string{"cts", "rts", "rx", "tx"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UART flow-control signals = %#v, want %#v", got, want)
	}
	spiWrite := providerRole("display", "digital_bus", "bidirectional", 0, 3.3).Contract
	spiWrite.Protocol = &Protocol{Name: "spi", Mode: "push_pull"}
	spiWrite.RequiredTraits = []string{"spi_write_only"}
	if got, want := mcuProtocolSignals(spiWrite, "spi"), []string{"cs", "mosi", "sck"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("write-only SPI signals = %#v, want %#v", got, want)
	}
}

func TestMCUSupportParentFunctionResolvesGPIOBackedSPIChipSelect(t *testing.T) {
	assignment := mcuAssignment{Pins: []mcuPinAssignment{
		{Kind: "spi", Instance: "spi1", Lane: "sck", Function: "PB3"},
		{Kind: "gpio", Instance: "spi1", Lane: "cs", Function: "PA4"},
	}}
	if got := mcuSupportParentFunction("peripheral:spi:cs", assignment, "spi1"); got != "PA4" {
		t.Fatalf("SPI chip-select support function = %q, want PA4", got)
	}
}

func TestMCUCompanionTargetsEveryAssignedPeripheralInstance(t *testing.T) {
	assignment := mcuAssignment{Pins: []mcuPinAssignment{
		{Kind: "i2c", Instance: "i2c2", Lane: "scl", Function: "PB10"},
		{Kind: "i2c", Instance: "i2c1", Lane: "sda", Function: "PB7"},
		{Kind: "i2c", Instance: "i2c1", Lane: "scl", Function: "PB6"},
		{Kind: "i2c", Instance: "i2c2", Lane: "sda", Function: "PB11"},
	}}
	companion := components.CompanionRequirement{AppliesTo: []string{"peripheral:i2c"}}
	if got, want := mcuCompanionTargets(companion, assignment), []string{"i2c1", "i2c2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("companion targets = %#v, want %#v", got, want)
	}
	if got := mcuSupportParentFunction("peripheral:i2c:sda", assignment, "i2c2"); got != "PB11" {
		t.Fatalf("I2C2 SDA support function = %q, want PB11", got)
	}
}

func TestMCUSupplyConnectionsKeepCatalogDomainsSeparate(t *testing.T) {
	record := architectureMCURecord(t, "mcu.microchip.atmega328p_a.tqfp32")
	record.MCU.SupplyDomains = []components.MCUSupplyDomain{
		{ID: "core", PowerFunctions: []string{"VCC", "AVCC"}, GroundFunctions: []string{"GND", "AGND"}},
		{ID: "aux", PowerFunctions: []string{"GPIO_1", "GPIO_2"}, GroundFunctions: []string{"GPIO_9", "GPIO_10"}},
	}
	part := catalogPart{selected: SelectedComponent{InstanceID: "mcu"}, record: record}
	connections := mcuSupplyConnections(part)
	if len(connections) != 4 {
		t.Fatalf("multi-domain supply connections = %#v, want four domain-local nets", connections)
	}
	for _, connection := range connections {
		if !strings.Contains(connection.ID, "core") && !strings.Contains(connection.ID, "aux") {
			t.Fatalf("multi-domain connection lacks domain identity: %#v", connection)
		}
		core, aux := false, false
		for _, endpoint := range connection.Endpoints {
			core = core || slices.Contains([]string{"VCC", "AVCC", "GND", "AGND"}, endpoint.Function)
			aux = aux || slices.Contains([]string{"GPIO_1", "GPIO_2", "GPIO_9", "GPIO_10"}, endpoint.Function)
		}
		if core && aux {
			t.Fatalf("MCU supply domains were merged: %#v", connection)
		}
	}
}

func TestSolveMCUAssignmentReturnsStableExhaustionCode(t *testing.T) {
	record := architectureMCURecord(t, "mcu.microchip.atmega328p_a.tqfp32")
	var demands []mcuRoleDemand
	for index := 0; index < len(record.MCU.Pins)+1; index++ {
		demands = append(demands, mcuRoleDemand{Role: "gpio_" + string(rune('a'+index)), Kind: "gpio"})
	}
	_, err := solveMCUAssignment(record, demands, "isp", "internal_rc")
	var assignmentErr *mcuAssignmentError
	if !errors.As(err, &assignmentErr) || assignmentErr.Code != CodeMCUPinAssignmentImpossible {
		t.Fatalf("expected stable pin exhaustion code, got %v", err)
	}
}

func TestValidateMCUAssignmentRejectsPerPinCurrent(t *testing.T) {
	record := architectureMCURecord(t, "mcu.microchip.atmega328p_a.tqfp32")
	request := ProviderRequest{Capability: "programmable_controller", Ports: []RoleContract{providerRole("drive", "digital_logic", "source", 0, 5)}}
	current := 0.025
	request.Ports[0].Contract.RequiredCurrentCapacityA = &current
	assignment, err := solveMCUAssignment(record, mcuDemandsFromRequest(request), "isp", "internal_rc")
	if err != nil {
		t.Fatal(err)
	}
	assertMCUElectricalCode(t, validateMCUAssignmentElectrical(record, request, assignment), CodeMCUPinCurrentExceeded)
}

func TestValidateMCUAssignmentUsesDirectionSpecificCurrentLimit(t *testing.T) {
	record := architectureMCURecord(t, "mcu.microchip.atmega328p_a.tqfp32")
	source, sink := 30.0, 10.0
	record.MCU.CurrentBudget.MaximumSourcePerPinMA = &source
	record.MCU.CurrentBudget.MaximumSinkPerPinMA = &sink
	request := ProviderRequest{Capability: "programmable_controller", Ports: []RoleContract{providerRole("drive", "digital_logic", "source", 0, 5)}}
	current := 0.015
	request.Ports[0].Contract.RequiredCurrentCapacityA = &current
	assignment, err := solveMCUAssignment(record, mcuDemandsFromRequest(request), "isp", "internal_rc")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateMCUAssignmentElectrical(record, request, assignment); err != nil {
		t.Fatalf("source current within source-specific limit was rejected: %v", err)
	}
	request.Ports[0].Contract.Direction = "sink"
	assignment, err = solveMCUAssignment(record, mcuDemandsFromRequest(request), "isp", "internal_rc")
	if err != nil {
		t.Fatal(err)
	}
	assertMCUElectricalCode(t, validateMCUAssignmentElectrical(record, request, assignment), CodeMCUPinCurrentExceeded)
}

func TestValidateMCUAssignmentRejectsAggregateCurrent(t *testing.T) {
	record := architectureMCURecord(t, "mcu.microchip.atmega328p_a.tqfp32")
	request := ProviderRequest{Capability: "programmable_controller"}
	current := 0.018
	for _, role := range []string{"drive_a", "drive_b", "drive_c", "drive_d", "drive_e", "drive_f"} {
		port := providerRole(role, "digital_logic", "source", 0, 5)
		port.Contract.RequiredCurrentCapacityA = &current
		request.Ports = append(request.Ports, port)
	}
	assignment, err := solveMCUAssignment(record, mcuDemandsFromRequest(request), "isp", "internal_rc")
	if err != nil {
		t.Fatal(err)
	}
	assertMCUElectricalCode(t, validateMCUAssignmentElectrical(record, request, assignment), CodeMCUAggregateCurrent)
}

func TestValidateMCUAssignmentRejectsPinVoltageDomain(t *testing.T) {
	record := architectureMCURecord(t, "mcu.espressif.esp32_wroom_32e")
	request := ProviderRequest{Capability: "programmable_controller", Ports: []RoleContract{providerRole("logic", "digital_logic", "source", 0, 5)}}
	assignment, err := solveMCUAssignment(record, mcuDemandsFromRequest(request), "uart0_bootloader", "module_crystal")
	if err != nil {
		t.Fatal(err)
	}
	assertMCUElectricalCode(t, validateMCUAssignmentElectrical(record, request, assignment), CodeMCUVoltageDomainMismatch)
}

func TestValidateMCUAssignmentUsesAssignedPinSupplyDomain(t *testing.T) {
	record := architectureMCURecord(t, "mcu.st.stm32g031k8t6.lqfp32")
	record.MCU.SupplyDomains = []components.MCUSupplyDomain{
		{ID: "core", PowerFunctions: []string{"VDD"}, GroundFunctions: []string{"VSS"}, MinimumV: 1.7, MaximumV: 1.9},
		{ID: "io", PowerFunctions: []string{"VDD"}, GroundFunctions: []string{"VSS"}, MinimumV: 3.0, MaximumV: 3.6},
	}
	request := ProviderRequest{Capability: "programmable_controller", Ports: []RoleContract{providerRole("logic", "digital_logic", "source", 0, 3.3)}}
	assignment, err := solveMCUAssignment(record, mcuDemandsFromRequest(request), "swd", "internal_hsi_pll")
	if err != nil {
		t.Fatal(err)
	}
	for index := range record.MCU.Pins {
		if record.MCU.Pins[index].Function == assignment.Pins[0].Function {
			record.MCU.Pins[index].SupplyDomain = "core"
		}
	}
	assertMCUElectricalCode(t, validateMCUAssignmentElectrical(record, request, assignment), CodeMCUVoltageDomainMismatch)
}

func TestESP32ReviewedStrappingPinsAreReserved(t *testing.T) {
	record := architectureMCURecord(t, "mcu.espressif.esp32_wroom_32e")
	want := map[string]bool{"GPIO0": false, "GPIO2": false, "GPIO5": false, "GPIO12": false, "GPIO15": false}
	for _, constraint := range record.MCU.BootConstraints {
		if _, exists := want[constraint.PinFunction]; exists {
			want[constraint.PinFunction] = true
		}
	}
	for function, found := range want {
		if !found {
			t.Fatalf("ESP32 strapping pin %s lacks boot reservation evidence: %#v", function, record.MCU.BootConstraints)
		}
	}
}

func TestValidateMCUAssignmentRejectsClockAndI2CLoading(t *testing.T) {
	record := architectureMCURecord(t, "mcu.st.stm32g031k8t6.lqfp32")
	request := ProviderRequest{Capability: "programmable_controller", Ports: []RoleContract{providerRole("bus", "digital_bus", "bidirectional", 3.0, 3.6)}}
	request.Ports[0].Contract.Protocol = &Protocol{Name: "i2c", Mode: "open_drain", MaxFrequencyHz: 400_000}
	request.Constraints = []Constraint{constraintNumber("clock_frequency", "minimum", 80_000_000, "Hz", 0)}
	assignment, err := solveMCUAssignment(record, mcuDemandsFromRequest(request), "swd", "internal_hsi_pll")
	if err != nil {
		t.Fatal(err)
	}
	assertMCUElectricalCode(t, validateMCUAssignmentElectrical(record, request, assignment), CodeMCUClockFrequency)
	request.Constraints = []Constraint{constraintNumber("bus_capacitance", "maximum", 500, "pF", 0)}
	assertMCUElectricalCode(t, validateMCUAssignmentElectrical(record, request, assignment), CodeMCUPeripheralLoading)
}

func architectureMCURecord(t *testing.T, id string) components.ComponentRecord {
	t.Helper()
	catalog := loadArchitectureCatalog(t)
	for _, record := range catalog.Records {
		if record.ID == id {
			return record
		}
	}
	t.Fatalf("MCU record %s not found", id)
	return components.ComponentRecord{}
}

func cloneMCURecord(t *testing.T, record components.ComponentRecord) components.ComponentRecord {
	t.Helper()
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var result components.ComponentRecord
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertMCUBundleInstance(t *testing.T, pins []mcuPinAssignment, role string, count int) {
	t.Helper()
	instance := ""
	found := 0
	for _, pin := range pins {
		if pin.Role != role {
			continue
		}
		found++
		if pin.Lane == "cs" && pin.Kind == "gpio" {
			continue
		}
		if instance == "" {
			instance = pin.Instance
		} else if pin.Instance != instance {
			t.Fatalf("role %s spans peripheral instances: %#v", role, pins)
		}
	}
	if found != count {
		t.Fatalf("role %s has %d pins, want %d: %#v", role, found, count, pins)
	}
}

func assertMCUElectricalCode(t *testing.T, err error, want reports.Code) {
	t.Helper()
	var assignmentErr *mcuAssignmentError
	if !errors.As(err, &assignmentErr) || assignmentErr.Code != want {
		t.Fatalf("electrical validation error = %v, want %s", err, want)
	}
}
