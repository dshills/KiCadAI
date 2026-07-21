package architecturesearch

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

const (
	CodeMCUPinAssignmentImpossible reports.Code = "MCU_PIN_ASSIGNMENT_IMPOSSIBLE"
	CodeMCUProgrammingUnavailable  reports.Code = "MCU_PROGRAMMING_INTERFACE_UNAVAILABLE"
	CodeMCUClockUnavailable        reports.Code = "MCU_CLOCK_UNAVAILABLE"
	CodeMCUCapabilityUnavailable   reports.Code = "MCU_REQUIRED_CAPABILITY_UNAVAILABLE"
	CodeMCUVoltageDomainMismatch   reports.Code = "MCU_VOLTAGE_DOMAIN_MISMATCH"
	CodeMCUPinCurrentExceeded      reports.Code = "MCU_PIN_CURRENT_EXCEEDED"
	CodeMCUAggregateCurrent        reports.Code = "MCU_AGGREGATE_CURRENT_EXCEEDED"
	CodeMCUPeripheralLoading       reports.Code = "MCU_PERIPHERAL_LOADING_EXCEEDED"
	CodeMCUClockFrequency          reports.Code = "MCU_CLOCK_FREQUENCY_UNAVAILABLE"
)

type mcuRoleDemand struct {
	Role          string
	Kind          string
	Signals       []string
	Direction     string
	Mode          string
	MaximumHz     float64
	InterruptOnly bool
}

type mcuPinAssignment struct {
	Role       string
	Lane       string
	Kind       string
	Instance   string
	Signal     string
	Function   string
	PackagePad string
}

type mcuAssignment struct {
	Pins                 []mcuPinAssignment
	ProgrammingInterface components.MCUProgrammingInterface
	ClockOption          components.MCUClockOption
}

// Bound adversarial combinatorics while leaving ample room for the normalized
// resource sets exercised by the catalog; exhaustion is a stable rejection,
// never a partial assignment.
const maxMCUAssignmentSearchStates = 10_000

type mcuAssignmentSearch struct {
	states    int
	exhausted bool
}

type assignableMCUCandidate struct {
	part       catalogPart
	assignment mcuAssignment
	areaMM2    float64
	unusedPins int
	queryMatch bool
}

type mcuSupplyGroup struct {
	ID              string
	PowerFunctions  []string
	GroundFunctions []string
	MinimumV        float64
	MaximumV        float64
}

type mcuAssignmentError struct {
	Code reports.Code
	Role string
	Text string
}

func (err *mcuAssignmentError) Error() string {
	if err.Role == "" {
		return fmt.Sprintf("%s: %s", err.Code, err.Text)
	}
	return fmt.Sprintf("%s at %s: %s", err.Code, err.Role, err.Text)
}

func (err *mcuAssignmentError) ArchitectureRejectionCode() reports.Code {
	if err == nil || err.Code == "" {
		return CodeMCUPinAssignmentImpossible
	}
	return err.Code
}

func mcuDemandsFromRequest(request ProviderRequest) []mcuRoleDemand {
	demands := make([]mcuRoleDemand, 0, len(request.Ports))
	for _, port := range request.Ports {
		contract := port.Contract
		if contract.Kind == "power" || contract.Kind == "reference" {
			continue
		}
		demand := mcuRoleDemand{Role: port.Role, Direction: contract.Direction}
		if contract.FrequencyMaxHz != nil {
			demand.MaximumHz = *contract.FrequencyMaxHz
		}
		if contract.Protocol != nil {
			demand.Kind = canonicalMCUPeripheralKind(contract.Protocol.Name)
			demand.Mode = strings.ToLower(strings.TrimSpace(contract.Protocol.Mode))
			if contract.Protocol.MaxFrequencyHz > demand.MaximumHz {
				demand.MaximumHz = contract.Protocol.MaxFrequencyHz
			}
			demand.Signals = mcuProtocolSignals(contract, demand.Kind)
		}
		if demand.Kind == "" {
			demand.Kind = mcuKindFromContract(contract, port.Role)
		}
		if demand.Kind == "interrupt" {
			demand.InterruptOnly = true
		}
		demands = append(demands, demand)
	}
	slices.SortStableFunc(demands, compareMCUDemands)
	return demands
}

func mcuProtocolSignals(contract PortContract, kind string) []string {
	var signals []string
	switch kind {
	case "i2c":
		signals = []string{"scl", "sda"}
	case "uart":
		switch contract.Direction {
		case "source":
			signals = []string{"tx"}
		case "sink":
			signals = []string{"rx"}
		default:
			signals = []string{"rx", "tx"}
		}
		if mcuContractHasTrait(contract, "hardware_flow_control", "uart_rts_cts") {
			signals = append(signals, "cts", "rts")
		}
	case "spi":
		signals = []string{"cs", "miso", "mosi", "sck"}
		optional := map[string]bool{
			"cs":   mcuContractHasTrait(contract, "spi_no_cs"),
			"miso": mcuContractHasTrait(contract, "spi_no_miso", "spi_write_only"),
			"mosi": mcuContractHasTrait(contract, "spi_no_mosi", "spi_read_only"),
		}
		signals = slices.DeleteFunc(signals, func(signal string) bool { return optional[signal] })
	}
	slices.Sort(signals)
	return signals
}

func mcuContractHasTrait(contract PortContract, wants ...string) bool {
	for _, traits := range [][]string{contract.Traits, contract.RequiredTraits} {
		for _, trait := range traits {
			for _, want := range wants {
				if strings.EqualFold(strings.TrimSpace(trait), want) {
					return true
				}
			}
		}
	}
	return false
}

func canonicalMCUPeripheralKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "i2c", "i²c", "twi":
		return "i2c"
	case "uart", "usart", "serial":
		return "uart"
	case "spi":
		return "spi"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func mcuKindFromContract(contract PortContract, role string) string {
	traits := append(slices.Clone(contract.Traits), contract.RequiredTraits...)
	for _, trait := range traits {
		switch strings.ToLower(strings.TrimSpace(trait)) {
		case "adc", "analog_input":
			return "adc"
		case "pwm":
			return "pwm"
		case "interrupt", "external_interrupt":
			return "interrupt"
		}
	}
	role = strings.ToLower(role)
	if strings.Contains(role, "interrupt") || strings.Contains(role, "irq") {
		return "interrupt"
	}
	switch contract.Kind {
	case "analog_voltage":
		if contract.Direction == "source" {
			return "dac"
		}
		return "adc"
	case "analog_control":
		if contract.Direction == "source" {
			return "pwm"
		}
		return "adc"
	default:
		return "gpio"
	}
}

func compareMCUDemands(left, right mcuRoleDemand) int {
	leftRank := mcuDemandRank(left)
	rightRank := mcuDemandRank(right)
	if leftRank != rightRank {
		return leftRank - rightRank
	}
	if order := strings.Compare(left.Kind, right.Kind); order != 0 {
		return order
	}
	return strings.Compare(left.Role, right.Role)
}

func mcuDemandRank(demand mcuRoleDemand) int {
	if len(demand.Signals) > 1 {
		return 0
	}
	switch demand.Kind {
	case "adc", "dac", "pwm", "interrupt":
		return 1
	default:
		return 2
	}
}

func solveMCUAssignment(record components.ComponentRecord, demands []mcuRoleDemand, programmingPreference, clockPreference string) (mcuAssignment, error) {
	if record.MCU == nil {
		return mcuAssignment{}, &mcuAssignmentError{Code: CodeMCUPinAssignmentImpossible, Text: "candidate has no normalized MCU evidence"}
	}
	programming, ok := selectMCUProgrammingInterface(record.MCU.ProgrammingInterfaces, programmingPreference)
	if !ok {
		return mcuAssignment{}, &mcuAssignmentError{Code: CodeMCUProgrammingUnavailable, Text: "requested programming interface is unavailable"}
	}
	clock, ok := selectMCUClockOption(record.MCU.ClockOptions, clockPreference)
	if !ok {
		return mcuAssignment{}, &mcuAssignmentError{Code: CodeMCUClockUnavailable, Text: "requested clock option is unavailable"}
	}
	reserved := map[string]bool{}
	for _, signal := range programming.Signals {
		if strings.Contains(strings.ToLower(programming.Kind), "spi") && !strings.EqualFold(signal.Signal, "reset") {
			// In-system programming intentionally shares the application SPI
			// pins. Electrical loading is validated separately; RESET remains
			// reserved so the programming entry path cannot be consumed.
			continue
		}
		reserved[signal.PinFunction] = true
	}
	for _, function := range clock.Pins {
		reserved[function] = true
	}
	for _, constraint := range record.MCU.BootConstraints {
		reserved[constraint.PinFunction] = true
	}
	packagePads := mcuPackagePads(record)
	normalizedDemands := slices.Clone(demands)
	slices.SortStableFunc(normalizedDemands, compareMCUDemands)
	assignment := mcuAssignment{ProgrammingInterface: programming, ClockOption: clock}
	used := map[string]bool{}
	search := &mcuAssignmentSearch{}
	if !solveMCUDemands(record.MCU.Pins, packagePads, normalizedDemands, 0, reserved, used, &assignment.Pins, search) {
		role := ""
		if len(normalizedDemands) > 0 {
			role = normalizedDemands[0].Role
		}
		text := "no conflict-free physical-pin assignment satisfies all roles"
		if search.exhausted {
			text = fmt.Sprintf("bounded physical-pin assignment search exhausted after %d states", maxMCUAssignmentSearchStates)
		}
		return mcuAssignment{}, &mcuAssignmentError{Code: CodeMCUPinAssignmentImpossible, Role: role, Text: text}
	}
	slices.SortStableFunc(assignment.Pins, func(left, right mcuPinAssignment) int {
		if order := strings.Compare(left.Role, right.Role); order != 0 {
			return order
		}
		if order := strings.Compare(left.Lane, right.Lane); order != 0 {
			return order
		}
		return strings.Compare(left.Function, right.Function)
	})
	return assignment, nil
}

func validateMCUAssignmentElectrical(record components.ComponentRecord, request ProviderRequest, assignment mcuAssignment) error {
	if record.MCU == nil {
		return &mcuAssignmentError{Code: CodeMCUPinAssignmentImpossible, Text: "candidate has no normalized MCU evidence"}
	}
	if minimum, maximum, ok := firstNumericConstraint(request.Constraints, "clock_frequency", "cpu_clock_frequency"); ok {
		required := math.Max(minimum, maximum)
		if required > assignment.ClockOption.MaximumHz {
			return &mcuAssignmentError{Code: CodeMCUClockFrequency, Text: fmt.Sprintf("required clock %.9g Hz exceeds selected clock maximum %.9g Hz", required, assignment.ClockOption.MaximumHz)}
		}
	}
	pinsByFunction := map[string]components.MCUPinEvidence{}
	for _, pin := range record.MCU.Pins {
		pinsByFunction[pin.Function] = pin
	}
	portsByRole := map[string]RoleContract{}
	for _, port := range request.Ports {
		portsByRole[port.Role] = port
	}
	aggregateCurrentMA := 0.0
	for _, selected := range assignment.Pins {
		port, ok := portsByRole[selected.Role]
		if !ok {
			continue
		}
		pin := pinsByFunction[selected.Function]
		supplyMaximum, domainKnown := mcuPinSupplyMaximum(record.MCU, pin)
		if !domainKnown {
			return &mcuAssignmentError{Code: CodeMCUVoltageDomainMismatch, Role: selected.Role, Text: "assigned pin lacks an unambiguous verified supply domain"}
		}
		if maximum := numericMaximum(port.Contract.Voltage); maximum > supplyMaximum && !pin.FiveVTolerant {
			return &mcuAssignmentError{Code: CodeMCUVoltageDomainMismatch, Role: selected.Role, Text: fmt.Sprintf("required %.9g V exceeds verified pin domain %.9g V", maximum, supplyMaximum)}
		}
		requiredMA := requiredMCUCurrentMA(port.Contract)
		if requiredMA <= 0 {
			continue
		}
		limitMA := mcuAssignmentCurrentLimit(pin, selected, port.Contract, record.MCU.CurrentBudget)
		if limitMA > 0 && requiredMA > limitMA {
			return &mcuAssignmentError{Code: CodeMCUPinCurrentExceeded, Role: selected.Role, Text: fmt.Sprintf("required %.9g mA exceeds verified direction-specific per-pin limit %.9g mA", requiredMA, limitMA)}
		}
		aggregateCurrentMA += requiredMA
	}
	if record.MCU.CurrentBudget != nil {
		limitMA := optionalPositiveValue(record.MCU.CurrentBudget.MaximumAggregateMA)
		if limitMA > 0 && aggregateCurrentMA > limitMA {
			return &mcuAssignmentError{Code: CodeMCUAggregateCurrent, Text: fmt.Sprintf("required aggregate pin current %.9g mA exceeds verified limit %.9g mA", aggregateCurrentMA, limitMA)}
		}
	}
	i2cPullupOhm := 0.0
	if slices.ContainsFunc(assignment.Pins, func(pin mcuPinAssignment) bool { return pin.Kind == "i2c" }) {
		var ok bool
		i2cPullupOhm, ok = mcuI2CPullupResistance(record)
		if !ok {
			return &mcuAssignmentError{Code: CodeMCUPeripheralLoading, Text: "I2C assignment requires a consistent catalog pull-up recipe"}
		}
		for _, selected := range assignment.Pins {
			if selected.Kind != "i2c" {
				continue
			}
			pin := pinsByFunction[selected.Function]
			supplyMaximum, domainKnown := mcuPinSupplyMaximum(record.MCU, pin)
			if !domainKnown {
				return &mcuAssignmentError{Code: CodeMCUVoltageDomainMismatch, Role: selected.Role, Text: "assigned I2C pin lacks an unambiguous verified supply domain"}
			}
			busMaximum := numericMaximum(portsByRole[selected.Role].Contract.Voltage)
			if busMaximum <= 0 {
				busMaximum = supplyMaximum
			}
			pullupCurrentMA := busMaximum / i2cPullupOhm * 1000
			limitMA := optionalPositiveValue(pin.MaximumSinkMA)
			if limitMA == 0 && record.MCU.CurrentBudget != nil {
				limitMA = optionalPositiveValue(record.MCU.CurrentBudget.MaximumSinkPerPinMA)
			}
			if limitMA > 0 && pullupCurrentMA > limitMA {
				return &mcuAssignmentError{Code: CodeMCUPeripheralLoading, Role: selected.Role, Text: fmt.Sprintf("I2C pull-up sink current %.9g mA exceeds verified pin limit %.9g mA", pullupCurrentMA, limitMA)}
			}
		}
	}
	if maximumPF, _, ok := firstNumericConstraint(request.Constraints, "bus_capacitance", "i2c_bus_capacitance"); ok {
		frequency := 0.0
		for _, port := range request.Ports {
			if port.Contract.Protocol != nil && canonicalMCUPeripheralKind(port.Contract.Protocol.Name) == "i2c" && port.Contract.Protocol.MaxFrequencyHz > frequency {
				frequency = port.Contract.Protocol.MaxFrequencyHz
			}
		}
		if i2cPullupOhm == 0 {
			return &mcuAssignmentError{Code: CodeMCUPeripheralLoading, Text: "I2C bus loading was requested but the candidate has no consistent pull-up recipe"}
		}
		riseTimeLimitS := 1e-6
		switch {
		case frequency > 1_000_000:
			return &mcuAssignmentError{Code: CodeMCUPeripheralLoading, Text: fmt.Sprintf("I2C frequency %.9g Hz exceeds the modeled loading policy", frequency)}
		case frequency > 400_000:
			riseTimeLimitS = 120e-9
		case frequency > 100_000:
			riseTimeLimitS = 300e-9
		}
		modeledRiseTimeS := 0.8473 * i2cPullupOhm * maximumPF * 1e-12
		if modeledRiseTimeS > riseTimeLimitS {
			return &mcuAssignmentError{Code: CodeMCUPeripheralLoading, Text: fmt.Sprintf("I2C %.9g ohm pull-up with %.9g pF models %.9g ns rise time, exceeding %.9g ns at %.9g Hz", i2cPullupOhm, maximumPF, modeledRiseTimeS*1e9, riseTimeLimitS*1e9, frequency)}
		}
	}
	return nil
}

func mcuPinSupplyMaximum(evidence *components.MCUEvidence, pin components.MCUPinEvidence) (float64, bool) {
	groups := mcuSupplyGroups(evidence)
	if len(groups) == 1 && pin.SupplyDomain == "" {
		return groups[0].MaximumV, groups[0].MaximumV > 0
	}
	for _, domain := range evidence.SupplyDomains {
		if domain.ID != pin.SupplyDomain {
			continue
		}
		groupID := domain.RailGroup
		if groupID == "" {
			groupID = domain.ID
		}
		for _, group := range groups {
			if group.ID == groupID {
				return group.MaximumV, group.MaximumV > 0
			}
		}
	}
	return 0, false
}

func mcuI2CPullupResistance(record components.ComponentRecord) (float64, bool) {
	value := 0.0
	for _, companion := range record.Companions {
		if !slices.ContainsFunc(companion.AppliesTo, func(candidate string) bool { return strings.EqualFold(strings.TrimSpace(candidate), "peripheral:i2c") }) {
			continue
		}
		for _, recipe := range companion.Recipes {
			if !strings.EqualFold(recipe.Family, "resistor") || !strings.Contains(strings.ToLower(string(recipe.Role)), "pullup") {
				continue
			}
			parsed, ok := components.ParseEngineeringValue(recipe.Value)
			if !ok || parsed <= 0 || (value != 0 && math.Abs(parsed-value) > value*1e-12) {
				return 0, false
			}
			value = parsed
		}
	}
	return value, value > 0
}

func numericMaximum(value NumericRange) float64 {
	if value.Maximum != nil {
		return *value.Maximum
	}
	if value.Minimum != nil {
		return *value.Minimum
	}
	return 0
}

func requiredMCUCurrentMA(contract PortContract) float64 {
	maximum := 0.0
	for _, value := range []*float64{contract.RequiredCurrentCapacityA, contract.CurrentDemandA, contract.MaximumCurrentDemandA} {
		if value != nil && *value*1000 > maximum {
			maximum = *value * 1000
		}
	}
	return maximum
}

func optionalPositiveValue(value *float64) float64 {
	if value == nil || *value <= 0 {
		return 0
	}
	return *value
}

func minimumPositiveValue(left, right float64) float64 {
	if left <= 0 {
		return right
	}
	if right <= 0 || left < right {
		return left
	}
	return right
}

func mcuAssignmentCurrentLimit(pin components.MCUPinEvidence, assignment mcuPinAssignment, contract PortContract, budget *components.MCUCurrentBudget) float64 {
	sourceLimit := optionalPositiveValue(pin.MaximumSourceMA)
	sinkLimit := optionalPositiveValue(pin.MaximumSinkMA)
	if budget != nil {
		if sourceLimit == 0 {
			sourceLimit = optionalPositiveValue(budget.MaximumSourcePerPinMA)
		}
		if sinkLimit == 0 {
			sinkLimit = optionalPositiveValue(budget.MaximumSinkPerPinMA)
		}
	}
	direction := strings.ToLower(contract.Direction)
	if assignment.Kind == "i2c" || (contract.Protocol != nil && strings.EqualFold(contract.Protocol.Mode, "open_drain")) {
		direction = "sink"
	}
	switch assignment.Lane {
	case "tx", "mosi", "sck", "cs":
		direction = "source"
	case "rx", "miso":
		direction = "sink"
	}
	if assignment.Kind == "pwm" || assignment.Kind == "dac" {
		direction = "source"
	}
	switch direction {
	case "source":
		return sourceLimit
	case "sink":
		return sinkLimit
	default:
		return minimumPositiveValue(sourceLimit, sinkLimit)
	}
}

func solveMCUDemands(pins []components.MCUPinEvidence, pads map[string]string, demands []mcuRoleDemand, index int, reserved, used map[string]bool, result *[]mcuPinAssignment, search *mcuAssignmentSearch) bool {
	search.states++
	if search.states > maxMCUAssignmentSearchStates {
		search.exhausted = true
		return false
	}
	if index == len(demands) {
		return true
	}
	candidates := mcuCandidatesForDemand(pins, pads, demands[index], reserved, used)
	for _, candidate := range candidates {
		for _, pin := range candidate {
			used[pin.Function] = true
			*result = append(*result, pin)
		}
		if solveMCUDemands(pins, pads, demands, index+1, reserved, used, result, search) {
			return true
		}
		*result = (*result)[:len(*result)-len(candidate)]
		for _, pin := range candidate {
			delete(used, pin.Function)
		}
	}
	return false
}

func mcuCandidatesForDemand(pins []components.MCUPinEvidence, pads map[string]string, demand mcuRoleDemand, reserved, used map[string]bool) [][]mcuPinAssignment {
	if len(demand.Signals) == 0 {
		return mcuSinglePinCandidates(pins, pads, demand, reserved, used)
	}
	instances := map[string]bool{}
	for _, pin := range pins {
		for _, alternate := range pin.AlternateFunctions {
			if canonicalMCUPeripheralKind(alternate.Kind) == demand.Kind {
				instances[strings.ToLower(alternate.Instance)] = true
			}
		}
	}
	instanceNames := make([]string, 0, len(instances))
	for instance := range instances {
		instanceNames = append(instanceNames, instance)
	}
	sort.Strings(instanceNames)
	var result [][]mcuPinAssignment
	for _, instance := range instanceNames {
		var combinations [][]mcuPinAssignment
		buildMCUBundleCandidates(pins, pads, demand, instance, demand.Signals, 0, reserved, used, map[string]bool{}, nil, &combinations)
		result = append(result, combinations...)
	}
	slices.SortStableFunc(result, compareMCUAssignmentBundles)
	return result
}

func buildMCUBundleCandidates(pins []components.MCUPinEvidence, pads map[string]string, demand mcuRoleDemand, instance string, signals []string, signalIndex int, reserved, used, localUsed map[string]bool, current []mcuPinAssignment, result *[][]mcuPinAssignment) {
	if signalIndex == len(signals) {
		*result = append(*result, slices.Clone(current))
		return
	}
	signal := signals[signalIndex]
	var choices []mcuPinAssignment
	for _, pin := range pins {
		if reserved[pin.Function] || used[pin.Function] || localUsed[pin.Function] || !mcuPinModeCompatible(pin, demand) {
			continue
		}
		matched := false
		for _, alternate := range pin.AlternateFunctions {
			if canonicalMCUPeripheralKind(alternate.Kind) != demand.Kind || !strings.EqualFold(alternate.Instance, instance) || !strings.EqualFold(alternate.Signal, signal) || !mcuAlternateSpeedCompatible(alternate, demand.MaximumHz) {
				continue
			}
			choices = append(choices, mcuPinAssignment{Role: demand.Role, Lane: signal, Kind: demand.Kind, Instance: alternate.Instance, Signal: alternate.Signal, Function: pin.Function, PackagePad: pads[pin.Function]})
			matched = true
		}
		if !matched && signal == "cs" && pin.GPIO != "" && slices.Contains(pin.ElectricalModes, "push_pull") {
			choices = append(choices, mcuPinAssignment{Role: demand.Role, Lane: signal, Kind: "gpio", Instance: instance, Signal: signal, Function: pin.Function, PackagePad: pads[pin.Function]})
		}
	}
	slices.SortStableFunc(choices, compareMCUPinAssignments)
	for _, choice := range choices {
		localUsed[choice.Function] = true
		buildMCUBundleCandidates(pins, pads, demand, instance, signals, signalIndex+1, reserved, used, localUsed, append(current, choice), result)
		delete(localUsed, choice.Function)
	}
}

func mcuSinglePinCandidates(pins []components.MCUPinEvidence, pads map[string]string, demand mcuRoleDemand, reserved, used map[string]bool) [][]mcuPinAssignment {
	var result [][]mcuPinAssignment
	for _, pin := range pins {
		if reserved[pin.Function] || used[pin.Function] || !mcuPinModeCompatible(pin, demand) {
			continue
		}
		if demand.Kind == "gpio" && pin.GPIO != "" {
			result = append(result, []mcuPinAssignment{{Role: demand.Role, Kind: "gpio", Instance: "gpio", Signal: pin.GPIO, Function: pin.Function, PackagePad: pads[pin.Function]}})
			continue
		}
		if demand.Kind == "interrupt" && pin.InterruptLine != "" {
			result = append(result, []mcuPinAssignment{{Role: demand.Role, Kind: "interrupt", Instance: "exti", Signal: pin.InterruptLine, Function: pin.Function, PackagePad: pads[pin.Function]}})
			continue
		}
		for _, alternate := range pin.AlternateFunctions {
			if canonicalMCUPeripheralKind(alternate.Kind) != demand.Kind || !mcuAlternateSpeedCompatible(alternate, demand.MaximumHz) {
				continue
			}
			result = append(result, []mcuPinAssignment{{Role: demand.Role, Kind: demand.Kind, Instance: alternate.Instance, Signal: alternate.Signal, Function: pin.Function, PackagePad: pads[pin.Function]}})
		}
	}
	slices.SortStableFunc(result, compareMCUAssignmentBundles)
	return result
}

func mcuPinModeCompatible(pin components.MCUPinEvidence, demand mcuRoleDemand) bool {
	if demand.InterruptOnly && pin.InterruptLine == "" {
		return false
	}
	if demand.Kind == "i2c" || demand.Mode == "open_drain" {
		return slices.Contains(pin.ElectricalModes, "open_drain")
	}
	if demand.Kind == "adc" {
		return slices.Contains(pin.ElectricalModes, "analog_input")
	}
	if demand.Direction == "source" && !slices.Contains(pin.ElectricalModes, "push_pull") {
		return false
	}
	return true
}

func mcuAlternateSpeedCompatible(alternate components.MCUAlternateFunction, required float64) bool {
	return required <= 0 || alternate.MaximumHz <= 0 || alternate.MaximumHz >= required
}

func compareMCUAssignmentBundles(left, right []mcuPinAssignment) int {
	for index := 0; index < len(left) && index < len(right); index++ {
		if order := compareMCUPinAssignments(left[index], right[index]); order != 0 {
			return order
		}
	}
	return len(left) - len(right)
}

func compareMCUPinAssignments(left, right mcuPinAssignment) int {
	if left.Kind == "gpio" && right.Kind == "gpio" {
		if order := strings.Compare(left.Function, right.Function); order != 0 {
			return order
		}
	}
	if order := strings.Compare(strings.ToLower(left.Instance), strings.ToLower(right.Instance)); order != 0 {
		return order
	}
	if order := strings.Compare(strings.ToLower(left.Signal), strings.ToLower(right.Signal)); order != 0 {
		return order
	}
	return strings.Compare(left.Function, right.Function)
}

func selectMCUProgrammingInterface(interfaces []components.MCUProgrammingInterface, preference string) (components.MCUProgrammingInterface, bool) {
	options := slices.Clone(interfaces)
	slices.SortStableFunc(options, func(left, right components.MCUProgrammingInterface) int { return strings.Compare(left.ID, right.ID) })
	preference = strings.ToLower(strings.TrimSpace(preference))
	for _, option := range options {
		if preference == "" || strings.EqualFold(option.ID, preference) || strings.EqualFold(option.Kind, preference) {
			return option, true
		}
	}
	return components.MCUProgrammingInterface{}, false
}

func selectMCUClockOption(options []components.MCUClockOption, preference string) (components.MCUClockOption, bool) {
	clocks := slices.Clone(options)
	slices.SortStableFunc(clocks, func(left, right components.MCUClockOption) int { return strings.Compare(left.ID, right.ID) })
	preference = strings.ToLower(strings.TrimSpace(preference))
	for _, option := range clocks {
		if preference != "" && (strings.EqualFold(option.ID, preference) || strings.EqualFold(option.Kind, preference)) {
			return option, true
		}
	}
	if preference != "" {
		return components.MCUClockOption{}, false
	}
	for _, option := range clocks {
		if option.Default {
			return option, true
		}
	}
	return components.MCUClockOption{}, false
}

func mcuPackagePads(record components.ComponentRecord) map[string]string {
	result := map[string]string{}
	if len(record.Packages) == 0 {
		return result
	}
	variants := slices.Clone(record.Packages)
	slices.SortStableFunc(variants, func(left, right components.PackageVariant) int { return strings.Compare(left.ID, right.ID) })
	for _, pad := range variants[0].PadFunctions {
		if _, exists := result[pad.Function]; !exists {
			result[pad.Function] = pad.Pad
		}
	}
	return result
}

func optionalMCUConstraintString(constraints []Constraint, name string) string {
	for _, constraint := range constraints {
		if !strings.EqualFold(constraint.Name, name) {
			continue
		}
		var value string
		if json.Unmarshal(constraint.Value, &value) == nil {
			return strings.ToLower(strings.TrimSpace(value))
		}
	}
	return ""
}

func optionalMCUCapabilities(constraints []Constraint) []string {
	for _, constraint := range constraints {
		if !strings.EqualFold(constraint.Name, "required_capabilities") {
			continue
		}
		var values []string
		if json.Unmarshal(constraint.Value, &values) == nil {
			for index := range values {
				values[index] = strings.ToLower(strings.TrimSpace(values[index]))
			}
			sort.Strings(values)
			return slices.Compact(values)
		}
		var value string
		if json.Unmarshal(constraint.Value, &value) == nil && strings.TrimSpace(value) != "" {
			return []string{strings.ToLower(strings.TrimSpace(value))}
		}
	}
	return nil
}

func mcuSupportsCapabilities(record components.ComponentRecord, required []string) bool {
	if len(required) == 0 {
		return true
	}
	offered := map[string]bool{}
	if record.MCU != nil {
		for _, capability := range record.MCU.Capabilities {
			offered[strings.ToLower(capability)] = true
		}
	}
	for _, capability := range required {
		if !offered[strings.ToLower(capability)] {
			return false
		}
	}
	return true
}

func (provider *CatalogProvider) selectAssignableMCU(request ProviderRequest, searchText string, ratings []components.RequiredRating) (catalogPart, mcuAssignment, error) {
	demands := mcuDemandsFromRequest(request)
	programmingPreference := optionalMCUConstraintString(request.Constraints, "programming_interface")
	if programmingPreference == "" {
		programmingPreference = optionalMCUConstraintString(request.Constraints, "programming_kind")
	}
	clockPreference := optionalMCUConstraintString(request.Constraints, "clock_source")
	requiredCapabilities := optionalMCUCapabilities(request.Constraints)
	var candidates []assignableMCUCandidate
	var rejectedDetails []string
	var rejectedCodes []reports.Code
	for _, record := range provider.catalog.Records {
		if record.Family != "mcu" || record.Generic || record.MPN == "" || record.MCU == nil || !recordSupportsRatings(record, ratings) {
			continue
		}
		if !mcuSupportsCapabilities(record, requiredCapabilities) {
			rejectedCodes = append(rejectedCodes, CodeMCUCapabilityUnavailable)
			rejectedDetails = append(rejectedDetails, record.ID+":"+string(CodeMCUCapabilityUnavailable))
			continue
		}
		assignment, err := solveMCUAssignment(record, demands, programmingPreference, clockPreference)
		if err != nil {
			var assignmentErr *mcuAssignmentError
			if errors.As(err, &assignmentErr) {
				rejectedCodes = append(rejectedCodes, assignmentErr.Code)
				rejectedDetails = append(rejectedDetails, record.ID+":"+string(assignmentErr.Code))
			} else {
				rejectedCodes = append(rejectedCodes, CodeMCUPinAssignmentImpossible)
				rejectedDetails = append(rejectedDetails, record.ID+":"+string(CodeMCUPinAssignmentImpossible))
			}
			continue
		}
		if err := validateMCUAssignmentElectrical(record, request, assignment); err != nil {
			var assignmentErr *mcuAssignmentError
			if errors.As(err, &assignmentErr) {
				rejectedCodes = append(rejectedCodes, assignmentErr.Code)
				rejectedDetails = append(rejectedDetails, record.ID+":"+string(assignmentErr.Code))
			} else {
				rejectedCodes = append(rejectedCodes, CodeMCUPinAssignmentImpossible)
				rejectedDetails = append(rejectedDetails, record.ID+":"+string(CodeMCUPinAssignmentImpossible))
			}
			continue
		}
		for _, variant := range record.Packages {
			if variant.DimensionsMM == nil || confidenceRank(EvidenceConfidence(variant.Verification.Confidence)) < confidenceRank(EvidenceRuleInferred) {
				continue
			}
			area := variant.DimensionsMM.Width * variant.DimensionsMM.Height
			evidence := componentEvidence(record, variant.Verification.Confidence)
			part := catalogPart{
				selected: SelectedComponent{InstanceID: "mcu", CatalogID: record.ID, VariantID: variant.ID, Evidence: evidence.Confidence},
				record:   record, usage: "programmable_controller", evidence: evidence,
			}
			candidates = append(candidates, assignableMCUCandidate{
				part: part, assignment: assignment, areaMM2: area, unusedPins: len(record.MCU.Pins) - len(assignment.Pins),
				queryMatch: mcuComponentSearchMatch(record, variant, searchText),
			})
		}
	}
	if len(candidates) == 0 {
		sort.Strings(rejectedDetails)
		const maximumRejectionDetails = 8
		rejectedCount := len(rejectedDetails)
		if rejectedCount > maximumRejectionDetails {
			rejectedDetails = append(rejectedDetails[:maximumRejectionDetails], fmt.Sprintf("and_%d_more", rejectedCount-maximumRejectionDetails))
		}
		return catalogPart{}, mcuAssignment{}, &mcuAssignmentError{
			Code: dominantMCURejectionCode(rejectedCodes), Role: "controller",
			Text: "no catalog-backed MCU satisfies normalized resources (" + strings.Join(rejectedDetails, ",") + ")",
		}
	}
	slices.SortStableFunc(candidates, func(left, right assignableMCUCandidate) int {
		if left.queryMatch != right.queryMatch {
			if left.queryMatch {
				return -1
			}
			return 1
		}
		if left.areaMM2 < right.areaMM2 {
			return -1
		}
		if left.areaMM2 > right.areaMM2 {
			return 1
		}
		if left.unusedPins < right.unusedPins {
			return -1
		}
		if left.unusedPins > right.unusedPins {
			return 1
		}
		if order := strings.Compare(left.part.record.ID, right.part.record.ID); order != 0 {
			return order
		}
		return strings.Compare(left.part.selected.VariantID, right.part.selected.VariantID)
	})
	return candidates[0].part, candidates[0].assignment, nil
}

func mcuComponentSearchMatch(record components.ComponentRecord, variant components.PackageVariant, searchText string) bool {
	query := strings.ToLower(strings.TrimSpace(searchText))
	if query == "" {
		return false
	}
	return strings.Contains(record.SearchText, query) || strings.Contains(variant.SearchText, query)
}

func dominantMCURejectionCode(codes []reports.Code) reports.Code {
	priority := []reports.Code{
		CodeMCUCapabilityUnavailable, CodeMCUProgrammingUnavailable,
		CodeMCUClockUnavailable, CodeMCUClockFrequency, CodeMCUVoltageDomainMismatch,
		CodeMCUPinCurrentExceeded, CodeMCUAggregateCurrent, CodeMCUPeripheralLoading,
		CodeMCUPinAssignmentImpossible,
	}
	for _, candidate := range priority {
		for _, code := range codes {
			if code == candidate {
				return candidate
			}
		}
	}
	return CodeMCUPinAssignmentImpossible
}

func mcuRealizationBindings(ports []RoleContract, instance string, assignment mcuAssignment, record components.ComponentRecord) []RealizationPortBinding {
	byRole := map[string][]mcuPinAssignment{}
	for _, pin := range assignment.Pins {
		byRole[pin.Role] = append(byRole[pin.Role], pin)
	}
	var bindings []RealizationPortBinding
	for _, port := range ports {
		pins := byRole[port.Role]
		if len(pins) > 0 {
			for _, pin := range pins {
				bindings = append(bindings, RealizationPortBinding{Role: port.Role, Lane: pin.Lane, Instance: instance, Function: pin.Function})
			}
			continue
		}
		if record.MCU == nil || (port.Contract.Kind != "power" && port.Contract.Kind != "reference") {
			continue
		}
		groups := mcuSupplyGroups(record.MCU)
		for _, group := range groups {
			functions := group.PowerFunctions
			if port.Contract.Kind == "reference" {
				functions = group.GroundFunctions
			}
			lane := ""
			if len(groups) > 1 {
				lane = group.ID
			}
			if len(functions) > 0 {
				bindings = append(bindings, RealizationPortBinding{Role: port.Role, Lane: lane, Instance: instance, Function: functions[0]})
			}
		}
	}
	return bindings
}

func mcuSupplyConnections(part catalogPart) []RealizationConnection {
	if part.record.MCU == nil {
		return nil
	}
	var connections []RealizationConnection
	groups := mcuSupplyGroups(part.record.MCU)
	for _, group := range groups {
		powerID, groundID := "controller_power", "controller_ground"
		if len(groups) > 1 {
			domainID := boundedMCUIdentifier(group.ID)
			powerID = boundedMCUIdentifier("controller_" + domainID + "_power")
			groundID = boundedMCUIdentifier("controller_" + domainID + "_ground")
		}
		powerEndpoints := recordSemanticEndpoints(part, group.PowerFunctions...)
		groundEndpoints := recordSemanticEndpoints(part, group.GroundFunctions...)
		if len(powerEndpoints) > 1 {
			connections = append(connections, semanticNet(powerID, "power", powerEndpoints...))
		}
		if len(groundEndpoints) > 1 {
			connections = append(connections, semanticNet(groundID, "reference", groundEndpoints...))
		}
	}
	return connections
}

func mcuSupplyGroups(evidence *components.MCUEvidence) []mcuSupplyGroup {
	if evidence == nil {
		return nil
	}
	byID := map[string]*mcuSupplyGroup{}
	for _, domain := range evidence.SupplyDomains {
		groupID := strings.TrimSpace(domain.RailGroup)
		if groupID == "" {
			groupID = domain.ID
		}
		group := byID[groupID]
		if group == nil {
			group = &mcuSupplyGroup{ID: groupID, MinimumV: domain.MinimumV, MaximumV: domain.MaximumV}
			byID[groupID] = group
		} else {
			group.MinimumV = math.Max(group.MinimumV, domain.MinimumV)
			group.MaximumV = math.Min(group.MaximumV, domain.MaximumV)
		}
		group.PowerFunctions = append(group.PowerFunctions, domain.PowerFunctions...)
		group.GroundFunctions = append(group.GroundFunctions, domain.GroundFunctions...)
	}
	groups := make([]mcuSupplyGroup, 0, len(byID))
	for _, group := range byID {
		slices.Sort(group.PowerFunctions)
		group.PowerFunctions = slices.Compact(group.PowerFunctions)
		slices.Sort(group.GroundFunctions)
		group.GroundFunctions = slices.Compact(group.GroundFunctions)
		groups = append(groups, *group)
	}
	slices.SortStableFunc(groups, func(left, right mcuSupplyGroup) int { return strings.Compare(left.ID, right.ID) })
	return groups
}

func (provider *CatalogProvider) expandMCUSupport(ctx context.Context, parent catalogPart, assignment mcuAssignment, connections []RealizationConnection) ([]catalogPart, []RealizationConnection, error) {
	parts := []catalogPart{parent}
	companions := slices.Clone(parent.record.Companions)
	slices.SortStableFunc(companions, func(left, right components.CompanionRequirement) int {
		if order := strings.Compare(left.ID, right.ID); order != 0 {
			return order
		}
		return strings.Compare(left.Role, right.Role)
	})
	for _, companion := range companions {
		// Unconditional companions are expanded by circuitgraph synthesis after
		// composition lowering. Only selection-dependent policies belong here,
		// because the downstream function intent does not carry the chosen
		// clock/programming option.
		targets := mcuCompanionTargets(companion, assignment)
		if !companion.Required || len(companion.AppliesTo) == 0 || len(targets) == 0 {
			continue
		}
		if len(companion.Recipes) == 0 {
			return nil, nil, fmt.Errorf("required MCU companion %s has no catalog recipe", companion.ID)
		}
		recipes := slices.Clone(companion.Recipes)
		slices.SortStableFunc(recipes, func(left, right components.CompanionPartRecipe) int { return strings.Compare(left.ID, right.ID) })
		for _, target := range targets {
			for _, recipe := range recipes {
				minimumConfidence := recipe.MinimumConfidence
				if minimumConfidence == "" {
					minimumConfidence = components.ConfidenceRuleInferred
				}
				selection, result := components.Select(ctx, provider.catalog, components.SelectionRequest{
					Query: components.Query{
						Family: recipe.Family, Package: recipe.Package, ValueKind: recipe.ValueKind,
						Value: recipe.Value, MinVoltageV: recipe.MinVoltageV, MinimumConfidence: minimumConfidence, Limit: 64,
					},
					Acceptance: components.AcceptanceStructural, RequiredFunctions: recipe.RequiredFunctions,
					AllowAlternatives: true,
				})
				if !result.OK {
					return nil, nil, fmt.Errorf("select MCU companion %s/%s: %v", companion.ID, recipe.ID, result.Issues)
				}
				instanceID := boundedMCUIdentifier("support_" + companion.ID + "_" + target + "_" + recipe.ID)
				evidence := componentEvidence(selection.Component, selection.Variant.Verification.Confidence)
				part := catalogPart{
					selected: SelectedComponent{InstanceID: instanceID, CatalogID: selection.Candidate.ComponentID, VariantID: selection.Candidate.VariantID, Evidence: evidence.Confidence},
					record:   selection.Component, usage: string(recipe.Role), value: recipe.Value, evidence: evidence,
				}
				parts = append(parts, part)
				for _, connection := range recipe.Connections {
					parentFunction := mcuSupportParentFunction(connection.ParentFunction, assignment, target)
					if strings.HasPrefix(strings.ToLower(parentFunction), "peripheral:") {
						return nil, nil, fmt.Errorf("MCU companion %s/%s references an unassigned peripheral role %s", companion.ID, recipe.ID, connection.ParentFunction)
					}
					connections = appendMCUSupportConnection(connections, parent, parentFunction, RealizationEndpoint{Instance: instanceID, Function: connection.Function})
				}
			}
		}
	}
	return parts, connections, nil
}

func mcuCompanionTargets(companion components.CompanionRequirement, assignment mcuAssignment) []string {
	if len(companion.AppliesTo) == 0 {
		return []string{""}
	}
	targets := map[string]bool{}
	for _, candidate := range companion.AppliesTo {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == "clock:"+strings.ToLower(assignment.ClockOption.ID) || candidate == "programming:"+strings.ToLower(assignment.ProgrammingInterface.ID) {
			targets[""] = true
		}
		if strings.HasPrefix(candidate, "peripheral:") {
			parts := strings.Split(candidate, ":")
			if len(parts) < 2 {
				continue
			}
			for _, pin := range assignment.Pins {
				if strings.EqualFold(pin.Kind, parts[1]) && (len(parts) < 3 || strings.EqualFold(pin.Instance, parts[2])) {
					targets[strings.ToLower(pin.Instance)] = true
				}
			}
		}
	}
	result := make([]string, 0, len(targets))
	for target := range targets {
		result = append(result, target)
	}
	slices.Sort(result)
	return result
}

func mcuSupportParentFunction(parentFunction string, assignment mcuAssignment, targetInstance string) string {
	const prefix = "peripheral:"
	value := strings.ToLower(strings.TrimSpace(parentFunction))
	if !strings.HasPrefix(value, prefix) {
		return parentFunction
	}
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return parentFunction
	}
	for _, pin := range assignment.Pins {
		kindMatches := strings.EqualFold(pin.Kind, parts[1]) || (parts[1] == "spi" && parts[2] == "cs" && pin.Kind == "gpio")
		instanceMatches := targetInstance == "" || strings.EqualFold(pin.Instance, targetInstance)
		if kindMatches && instanceMatches && strings.EqualFold(pin.Lane, parts[2]) {
			return pin.Function
		}
	}
	return parentFunction
}

func appendMCUSupportConnection(connections []RealizationConnection, parent catalogPart, parentFunction string, support RealizationEndpoint) []RealizationConnection {
	for index := range connections {
		for _, endpoint := range connections[index].Endpoints {
			if endpoint.Instance == parent.selected.InstanceID && strings.EqualFold(endpoint.Function, parentFunction) {
				connections[index].Endpoints = append(connections[index].Endpoints, support)
				return connections
			}
		}
	}
	role := "support"
	for _, domain := range parent.record.MCU.SupplyDomains {
		if slices.ContainsFunc(domain.PowerFunctions, func(function string) bool { return strings.EqualFold(function, parentFunction) }) {
			role = "power"
		}
		if slices.ContainsFunc(domain.GroundFunctions, func(function string) bool { return strings.EqualFold(function, parentFunction) }) {
			role = "reference"
		}
	}
	id := boundedMCUIdentifier("mcu_support_" + parentFunction)
	return append(connections, semanticNet(id, role, endpoint(parent, parentFunction), support))
}

func boundedMCUIdentifier(value string) string {
	identifier := derivedSemanticIdentifier(value)
	if len(identifier) <= 36 {
		return identifier
	}
	digest := sha256.Sum256([]byte(value))
	prefixLength := min(27, len(identifier))
	return identifier[:prefixLength] + "_" + fmt.Sprintf("%x", digest[:4])
}
