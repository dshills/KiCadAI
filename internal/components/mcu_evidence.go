package components

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"kicadai/internal/reports"
)

// MCUEvidence describes the device-independent resources needed to select and
// assign an MCU. Physical functions are the canonical functions already used
// by the record's symbol and package mappings.
type MCUEvidence struct {
	Architecture          string                    `json:"architecture"`
	Family                string                    `json:"family"`
	Capabilities          []string                  `json:"capabilities,omitempty"`
	SupplyDomains         []MCUSupplyDomain         `json:"supply_domains"`
	Pins                  []MCUPinEvidence          `json:"pins"`
	ProgrammingInterfaces []MCUProgrammingInterface `json:"programming_interfaces"`
	ClockOptions          []MCUClockOption          `json:"clock_options"`
	BootConstraints       []MCUBootConstraint       `json:"boot_constraints,omitempty"`
	CurrentBudget         *MCUCurrentBudget         `json:"current_budget,omitempty"`
	ReviewNote            string                    `json:"review_note"`
}

type MCUSupplyDomain struct {
	ID              string   `json:"id"`
	RailGroup       string   `json:"rail_group,omitempty"`
	PowerFunctions  []string `json:"power_functions"`
	GroundFunctions []string `json:"ground_functions"`
	MinimumV        float64  `json:"minimum_v"`
	MaximumV        float64  `json:"maximum_v"`
}

type MCUPinEvidence struct {
	Function           string                 `json:"function"`
	SupplyDomain       string                 `json:"supply_domain,omitempty"`
	GPIO               string                 `json:"gpio,omitempty"`
	ElectricalModes    []string               `json:"electrical_modes,omitempty"`
	AlternateFunctions []MCUAlternateFunction `json:"alternate_functions,omitempty"`
	InterruptLine      string                 `json:"interrupt_line,omitempty"`
	FiveVTolerant      bool                   `json:"five_v_tolerant,omitempty"`
	MaximumSourceMA    *float64               `json:"maximum_source_ma,omitempty"`
	MaximumSinkMA      *float64               `json:"maximum_sink_ma,omitempty"`
}

type MCUAlternateFunction struct {
	Kind      string  `json:"kind"`
	Instance  string  `json:"instance"`
	Signal    string  `json:"signal"`
	Mode      string  `json:"mode,omitempty"`
	MaximumHz float64 `json:"maximum_hz,omitempty"`
}

type MCUProgrammingInterface struct {
	ID      string               `json:"id"`
	Kind    string               `json:"kind"`
	Signals []MCUInterfaceSignal `json:"signals"`
}

type MCUInterfaceSignal struct {
	Signal      string `json:"signal"`
	PinFunction string `json:"pin_function"`
}

type MCUClockOption struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	MinimumHz float64  `json:"minimum_hz,omitempty"`
	MaximumHz float64  `json:"maximum_hz"`
	Pins      []string `json:"pins,omitempty"`
	Default   bool     `json:"default,omitempty"`
}

type MCUBootConstraint struct {
	PinFunction string `json:"pin_function"`
	State       string `json:"state"`
	Level       string `json:"level"`
	Reason      string `json:"reason"`
}

type MCUCurrentBudget struct {
	MaximumSourcePerPinMA *float64 `json:"maximum_source_per_pin_ma,omitempty"`
	MaximumSinkPerPinMA   *float64 `json:"maximum_sink_per_pin_ma,omitempty"`
	MaximumAggregateMA    *float64 `json:"maximum_aggregate_ma,omitempty"`
	TypicalSupplyMA       *float64 `json:"typical_supply_ma,omitempty"`
	MaximumSupplyMA       *float64 `json:"maximum_supply_ma,omitempty"`
}

func sortMCUEvidence(evidence *MCUEvidence) {
	if evidence == nil {
		return
	}
	sort.Strings(evidence.Capabilities)
	sort.SliceStable(evidence.SupplyDomains, func(i, j int) bool { return evidence.SupplyDomains[i].ID < evidence.SupplyDomains[j].ID })
	for i := range evidence.SupplyDomains {
		sort.Strings(evidence.SupplyDomains[i].PowerFunctions)
		sort.Strings(evidence.SupplyDomains[i].GroundFunctions)
	}
	sort.SliceStable(evidence.Pins, func(i, j int) bool { return evidence.Pins[i].Function < evidence.Pins[j].Function })
	for i := range evidence.Pins {
		sort.Strings(evidence.Pins[i].ElectricalModes)
		sort.SliceStable(evidence.Pins[i].AlternateFunctions, func(a, b int) bool {
			left := evidence.Pins[i].AlternateFunctions[a]
			right := evidence.Pins[i].AlternateFunctions[b]
			return mcuAlternateKey(left) < mcuAlternateKey(right)
		})
	}
	sort.SliceStable(evidence.ProgrammingInterfaces, func(i, j int) bool {
		return evidence.ProgrammingInterfaces[i].ID < evidence.ProgrammingInterfaces[j].ID
	})
	for i := range evidence.ProgrammingInterfaces {
		sort.SliceStable(evidence.ProgrammingInterfaces[i].Signals, func(a, b int) bool {
			left := evidence.ProgrammingInterfaces[i].Signals[a]
			right := evidence.ProgrammingInterfaces[i].Signals[b]
			if left.Signal == right.Signal {
				return left.PinFunction < right.PinFunction
			}
			return left.Signal < right.Signal
		})
	}
	sort.SliceStable(evidence.ClockOptions, func(i, j int) bool { return evidence.ClockOptions[i].ID < evidence.ClockOptions[j].ID })
	for i := range evidence.ClockOptions {
		sort.Strings(evidence.ClockOptions[i].Pins)
	}
	sort.SliceStable(evidence.BootConstraints, func(i, j int) bool {
		left := evidence.BootConstraints[i]
		right := evidence.BootConstraints[j]
		if left.PinFunction == right.PinFunction {
			if left.State == right.State {
				return left.Level < right.Level
			}
			return left.State < right.State
		}
		return left.PinFunction < right.PinFunction
	})
}

func validateMCUEvidence(path string, record *ComponentRecord) []reports.Issue {
	evidence := record.MCU
	if evidence == nil {
		if record.Family == "mcu" && record.Verification.Confidence == ConfidenceVerified {
			return []reports.Issue{NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path, "verified MCU records require MCU evidence")}
		}
		return nil
	}
	var issues []reports.Issue
	if record.Family != "mcu" {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path, "MCU evidence is only valid for the mcu family"))
	}
	if strings.TrimSpace(evidence.Architecture) == "" {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".architecture", "MCU architecture is required"))
	}
	if strings.TrimSpace(evidence.Family) == "" {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".family", "MCU family is required"))
	}
	if strings.TrimSpace(evidence.ReviewNote) == "" {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".review_note", "MCU evidence requires an explicit review note"))
	}
	knownFunctions := catalogPhysicalFunctions(record)
	seenDomains := map[string]struct{}{}
	domainGroups := map[string]struct{}{}
	for i, domain := range evidence.SupplyDomains {
		domainPath := fmt.Sprintf("%s.supply_domains[%d]", path, i)
		id := strings.TrimSpace(domain.ID)
		if id == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, domainPath+".id", "MCU supply domain id is required"))
		} else if _, exists := seenDomains[id]; exists {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, domainPath+".id", "duplicate MCU supply domain: "+id))
		}
		seenDomains[id] = struct{}{}
		groupID := strings.TrimSpace(domain.RailGroup)
		if groupID == "" {
			groupID = id
		}
		if groupID != "" {
			domainGroups[groupID] = struct{}{}
		}
		if domain.RailGroup != strings.TrimSpace(domain.RailGroup) {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, domainPath+".rail_group", "MCU rail group must be trimmed"))
		}
		if !finitePositiveMCU(domain.MinimumV) || !finitePositiveMCU(domain.MaximumV) || domain.MinimumV > domain.MaximumV {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, domainPath, "MCU supply domain requires a finite positive voltage range"))
		}
		if len(domain.PowerFunctions) == 0 || len(domain.GroundFunctions) == 0 {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, domainPath, "MCU supply domain requires power and ground functions"))
		}
		for _, function := range append(slices.Clone(domain.PowerFunctions), domain.GroundFunctions...) {
			if _, ok := knownFunctions[function]; !ok {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, domainPath, "MCU supply function is not mapped by both symbol and package: "+function))
			}
		}
	}
	if len(evidence.SupplyDomains) == 0 {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".supply_domains", "MCU evidence requires at least one supply domain"))
	}
	seenPins := map[string]struct{}{}
	for i, pin := range evidence.Pins {
		pinPath := fmt.Sprintf("%s.pins[%d]", path, i)
		function := strings.TrimSpace(pin.Function)
		if function == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, pinPath+".function", "MCU pin function is required"))
		} else if _, exists := seenPins[function]; exists {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, pinPath+".function", "duplicate MCU physical function: "+function))
		} else if _, ok := knownFunctions[function]; !ok {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, pinPath+".function", "MCU physical function is not mapped by both symbol and package: "+function))
		}
		seenPins[function] = struct{}{}
		supplyDomain := strings.TrimSpace(pin.SupplyDomain)
		if pin.SupplyDomain != supplyDomain {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, pinPath+".supply_domain", "MCU pin supply domain must be trimmed"))
		} else if supplyDomain == "" && len(domainGroups) > 1 {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, pinPath+".supply_domain", "MCU pins require a supply domain when independent rail groups exist"))
		} else if supplyDomain != "" {
			if _, ok := seenDomains[supplyDomain]; !ok {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, pinPath+".supply_domain", "MCU pin references an unknown supply domain: "+supplyDomain))
			}
		}
		seenAlternates := map[string]struct{}{}
		for j, alternate := range pin.AlternateFunctions {
			alternatePath := fmt.Sprintf("%s.alternate_functions[%d]", pinPath, j)
			if strings.TrimSpace(alternate.Kind) == "" || strings.TrimSpace(alternate.Instance) == "" || strings.TrimSpace(alternate.Signal) == "" {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, alternatePath, "MCU alternate function requires kind, instance, and signal"))
			}
			key := mcuAlternateKey(alternate)
			if _, exists := seenAlternates[key]; exists {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, alternatePath, "duplicate MCU alternate function: "+key))
			}
			seenAlternates[key] = struct{}{}
			if alternate.MaximumHz < 0 || math.IsNaN(alternate.MaximumHz) || math.IsInf(alternate.MaximumHz, 0) {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, alternatePath+".maximum_hz", "MCU alternate-function frequency must be finite and non-negative"))
			}
		}
		issues = append(issues, validateOptionalPositiveMCU(pinPath+".maximum_source_ma", pin.MaximumSourceMA)...)
		issues = append(issues, validateOptionalPositiveMCU(pinPath+".maximum_sink_ma", pin.MaximumSinkMA)...)
	}
	if len(evidence.Pins) == 0 {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".pins", "MCU evidence requires assignable physical pins"))
	}
	if record.Verification.Confidence == ConfidenceVerified {
		if evidence.CurrentBudget == nil || evidence.CurrentBudget.MaximumSourcePerPinMA == nil || evidence.CurrentBudget.MaximumSinkPerPinMA == nil || evidence.CurrentBudget.MaximumAggregateMA == nil {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".current_budget", "verified MCU evidence requires source, sink, and aggregate current limits"))
		}
	}
	if evidence.CurrentBudget != nil {
		issues = append(issues, validateOptionalPositiveMCU(path+".current_budget.maximum_source_per_pin_ma", evidence.CurrentBudget.MaximumSourcePerPinMA)...)
		issues = append(issues, validateOptionalPositiveMCU(path+".current_budget.maximum_sink_per_pin_ma", evidence.CurrentBudget.MaximumSinkPerPinMA)...)
		issues = append(issues, validateOptionalPositiveMCU(path+".current_budget.maximum_aggregate_ma", evidence.CurrentBudget.MaximumAggregateMA)...)
		issues = append(issues, validateOptionalPositiveMCU(path+".current_budget.typical_supply_ma", evidence.CurrentBudget.TypicalSupplyMA)...)
		issues = append(issues, validateOptionalPositiveMCU(path+".current_budget.maximum_supply_ma", evidence.CurrentBudget.MaximumSupplyMA)...)
	}
	seenProgramming := map[string]struct{}{}
	for i, programming := range evidence.ProgrammingInterfaces {
		interfacePath := fmt.Sprintf("%s.programming_interfaces[%d]", path, i)
		if strings.TrimSpace(programming.ID) == "" || strings.TrimSpace(programming.Kind) == "" || len(programming.Signals) == 0 {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, interfacePath, "MCU programming interface requires id, kind, and signals"))
		}
		if _, exists := seenProgramming[programming.ID]; exists {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, interfacePath+".id", "duplicate MCU programming interface: "+programming.ID))
		}
		seenProgramming[programming.ID] = struct{}{}
		for j, signal := range programming.Signals {
			signalPath := fmt.Sprintf("%s.signals[%d]", interfacePath, j)
			if strings.TrimSpace(signal.Signal) == "" {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, signalPath+".signal", "programming signal name is required"))
			}
			if _, ok := knownFunctions[signal.PinFunction]; !ok {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, signalPath+".pin_function", "programming signal function is not mapped by both symbol and package: "+signal.PinFunction))
			}
		}
	}
	if len(evidence.ProgrammingInterfaces) == 0 {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".programming_interfaces", "MCU evidence requires a programming interface"))
	}
	defaultClocks := 0
	seenClocks := map[string]struct{}{}
	for i, clock := range evidence.ClockOptions {
		clockPath := fmt.Sprintf("%s.clock_options[%d]", path, i)
		if strings.TrimSpace(clock.ID) == "" || strings.TrimSpace(clock.Kind) == "" || !finitePositiveMCU(clock.MaximumHz) || clock.MinimumHz < 0 || clock.MinimumHz > clock.MaximumHz {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, clockPath, "MCU clock option requires id, kind, and a valid frequency range"))
		}
		if _, exists := seenClocks[clock.ID]; exists {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, clockPath+".id", "duplicate MCU clock option: "+clock.ID))
		}
		seenClocks[clock.ID] = struct{}{}
		if clock.Default {
			defaultClocks++
		}
		for _, function := range clock.Pins {
			if _, ok := knownFunctions[function]; !ok {
				issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, clockPath+".pins", "MCU clock function is not mapped by both symbol and package: "+function))
			}
		}
	}
	if len(evidence.ClockOptions) == 0 || defaultClocks != 1 {
		issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path+".clock_options", "MCU evidence requires exactly one default clock option"))
	}
	for i, constraint := range evidence.BootConstraints {
		constraintPath := fmt.Sprintf("%s.boot_constraints[%d]", path, i)
		if _, ok := knownFunctions[constraint.PinFunction]; !ok {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, constraintPath+".pin_function", "MCU boot function is not mapped by both symbol and package: "+constraint.PinFunction))
		}
		if strings.TrimSpace(constraint.State) == "" || strings.TrimSpace(constraint.Level) == "" || strings.TrimSpace(constraint.Reason) == "" {
			issues = append(issues, NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, constraintPath, "MCU boot constraint requires state, level, and reason"))
		}
	}
	if evidence.CurrentBudget != nil {
		budget := evidence.CurrentBudget
		issues = append(issues, validateOptionalPositiveMCU(path+".current_budget.maximum_source_per_pin_ma", budget.MaximumSourcePerPinMA)...)
		issues = append(issues, validateOptionalPositiveMCU(path+".current_budget.maximum_sink_per_pin_ma", budget.MaximumSinkPerPinMA)...)
		issues = append(issues, validateOptionalPositiveMCU(path+".current_budget.maximum_aggregate_ma", budget.MaximumAggregateMA)...)
		issues = append(issues, validateOptionalPositiveMCU(path+".current_budget.typical_supply_ma", budget.TypicalSupplyMA)...)
		issues = append(issues, validateOptionalPositiveMCU(path+".current_budget.maximum_supply_ma", budget.MaximumSupplyMA)...)
	}
	return issues
}

func catalogPhysicalFunctions(record *ComponentRecord) map[string]struct{} {
	symbolFunctions := map[string]struct{}{}
	packageFunctions := map[string]struct{}{}
	for _, symbol := range record.Symbols {
		for _, pin := range symbol.FunctionPins {
			symbolFunctions[pin.Function] = struct{}{}
		}
	}
	for _, variant := range record.Packages {
		for _, pad := range variant.PadFunctions {
			packageFunctions[pad.Function] = struct{}{}
		}
	}
	result := map[string]struct{}{}
	for function := range symbolFunctions {
		if _, ok := packageFunctions[function]; ok {
			result[function] = struct{}{}
		}
	}
	return result
}

func mcuAlternateKey(alternate MCUAlternateFunction) string {
	return strings.ToLower(strings.TrimSpace(alternate.Kind)) + "/" + strings.ToLower(strings.TrimSpace(alternate.Instance)) + "/" + strings.ToLower(strings.TrimSpace(alternate.Signal)) + "/" + strings.ToLower(strings.TrimSpace(alternate.Mode))
}

func finitePositiveMCU(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func validateOptionalPositiveMCU(path string, value *float64) []reports.Issue {
	if value == nil || finitePositiveMCU(*value) {
		return nil
	}
	return []reports.Issue{NewIssue(CodeInvalidMetadata, reports.SeverityBlocked, path, "MCU current evidence must be finite and positive")}
}
