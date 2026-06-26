package pinmap

import (
	"fmt"
	"strings"

	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
)

type Entry struct {
	Symbol    string `json:"symbol"`
	Footprint string `json:"footprint"`
	Pins      []Pin  `json:"pins"`
	Source    string `json:"source"`
	Notes     string `json:"notes,omitempty"`
}

type Pin struct {
	SymbolPin    string `json:"symbol_pin"`
	Function     string `json:"function,omitempty"`
	FootprintPad string `json:"footprint_pad"`
}

type Report struct {
	Target                 string          `json:"target"`
	FabricationReady       bool            `json:"fabrication_ready"`
	FabricationReadyReason string          `json:"fabrication_ready_reason,omitempty"`
	Mappings               []Mapping       `json:"mappings"`
	Issues                 []reports.Issue `json:"issues"`
}

type ValidateOptions struct {
	LibraryIndex  *libraryresolver.LibraryIndex `json:"-"`
	LibraryIssues []reports.Issue               `json:"-"`
}

type Mapping struct {
	Ref              string                                  `json:"ref"`
	Symbol           string                                  `json:"symbol"`
	Footprint        string                                  `json:"footprint"`
	Status           string                                  `json:"status"`
	Source           string                                  `json:"source,omitempty"`
	Notes            string                                  `json:"notes,omitempty"`
	PinCount         int                                     `json:"pin_count"`
	MapCount         int                                     `json:"map_count"`
	CandidatePinmap  []libraryresolver.PinmapCandidate       `json:"candidate_pinmap,omitempty"`
	ResolverEvidence []libraryresolver.CompatibilityEvidence `json:"resolver_evidence,omitempty"`
}

var builtinEntries = []Entry{
	{Symbol: "Device:R", Footprint: "Resistor_SMD:R_0603_1608Metric", Source: "human_verified", Notes: "Generic two-terminal resistor mapping.", Pins: []Pin{{SymbolPin: "1", Function: "A", FootprintPad: "1"}, {SymbolPin: "2", Function: "B", FootprintPad: "2"}}},
	{Symbol: "Device:R", Footprint: "Resistor_SMD:R_0805_2012Metric", Source: "human_verified", Notes: "Generic two-terminal resistor mapping.", Pins: []Pin{{SymbolPin: "1", Function: "A", FootprintPad: "1"}, {SymbolPin: "2", Function: "B", FootprintPad: "2"}}},
	{Symbol: "Device:C", Footprint: "Capacitor_SMD:C_0603_1608Metric", Source: "human_verified", Notes: "Generic two-terminal nonpolar capacitor mapping.", Pins: []Pin{{SymbolPin: "1", Function: "A", FootprintPad: "1"}, {SymbolPin: "2", Function: "B", FootprintPad: "2"}}},
	{Symbol: "Device:C", Footprint: "Capacitor_SMD:C_0805_2012Metric", Source: "human_verified", Notes: "Generic two-terminal capacitor mapping.", Pins: []Pin{{SymbolPin: "1", Function: "A", FootprintPad: "1"}, {SymbolPin: "2", Function: "B", FootprintPad: "2"}}},
	{Symbol: "Device:LED", Footprint: "LED_SMD:LED_0805_2012Metric", Source: "human_verified", Notes: "Verify polarity marker against selected LED footprint before fabrication.", Pins: []Pin{{SymbolPin: "1", Function: "K", FootprintPad: "1"}, {SymbolPin: "2", Function: "A", FootprintPad: "2"}}},
	{Symbol: "Device:D", Footprint: "Diode_SMD:D_SOD-123", Source: "human_verified", Notes: "Generic signal diode cathode/anode mapping; verify exact package polarity marker before fabrication.", Pins: []Pin{{SymbolPin: "1", Function: "K", FootprintPad: "1"}, {SymbolPin: "2", Function: "A", FootprintPad: "2"}}},
	{Symbol: "Device:D_Schottky", Footprint: "Diode_SMD:D_SMA", Source: "human_verified", Notes: "Generic Schottky diode cathode/anode mapping; verify exact package polarity marker before fabrication.", Pins: []Pin{{SymbolPin: "1", Function: "K", FootprintPad: "1"}, {SymbolPin: "2", Function: "A", FootprintPad: "2"}}},
	{Symbol: "Device:D_TVS", Footprint: "Diode_SMD:D_SOD-323", Source: "human_verified", Notes: "Generic unidirectional TVS cathode/anode mapping; verify selected TVS datasheet before fabrication.", Pins: []Pin{{SymbolPin: "1", Function: "K", FootprintPad: "1"}, {SymbolPin: "2", Function: "A", FootprintPad: "2"}}},
	{Symbol: "Regulator_Linear:AMS1117-3.3", Footprint: "Package_TO_SOT_SMD:SOT-223-3_TabPin2", Source: "human_verified", Notes: "AMS1117-3.3 SOT-223 mapping: pin 1 GND, pin 2/tab VOUT, pin 3 VIN. Verify exact manufacturer ordering code before fabrication.", Pins: []Pin{{SymbolPin: "1", Function: "GND", FootprintPad: "1"}, {SymbolPin: "2", Function: "VOUT", FootprintPad: "2"}, {SymbolPin: "3", Function: "VIN", FootprintPad: "3"}}},
	{Symbol: "Regulator_Linear:AP2112K-3.3", Footprint: "Package_TO_SOT_SMD:SOT-23-5", Source: "human_verified", Notes: "AP2112K-3.3 SOT-23-5 mapping: pin 1 VIN, pin 2 GND, pin 3 EN, pin 4 NC, pin 5 VOUT. EN and NC require generated schematic handling before fabrication.", Pins: []Pin{{SymbolPin: "1", Function: "VIN", FootprintPad: "1"}, {SymbolPin: "2", Function: "GND", FootprintPad: "2"}, {SymbolPin: "3", Function: "EN", FootprintPad: "3"}, {SymbolPin: "4", Function: "NC", FootprintPad: "4"}, {SymbolPin: "5", Function: "VOUT", FootprintPad: "5"}}},
	{Symbol: "Amplifier_Operational:LMV321", Footprint: "Package_TO_SOT_SMD:SOT-23-5", Source: "human_verified", Notes: "LMV321 SOT-23-5 mapping from KiCad symbol and footprint pad numbers.", Pins: []Pin{{SymbolPin: "1", Function: "IN_PLUS", FootprintPad: "1"}, {SymbolPin: "2", Function: "V_MINUS", FootprintPad: "2"}, {SymbolPin: "3", Function: "IN_MINUS", FootprintPad: "3"}, {SymbolPin: "4", Function: "OUT", FootprintPad: "4"}, {SymbolPin: "5", Function: "V_PLUS", FootprintPad: "5"}}},
	{Symbol: "Sensor:BME280", Footprint: "Package_LGA:Bosch_LGA-8_2.5x2.5mm_P0.65mm_ClockwisePinNumbering", Source: "human_verified", Notes: "BME280 LGA-8 mapping from KiCad symbol and Bosch clockwise footprint numbering.", Pins: []Pin{{SymbolPin: "1", Function: "GND", FootprintPad: "1"}, {SymbolPin: "2", Function: "CSB", FootprintPad: "2"}, {SymbolPin: "3", Function: "SDA", FootprintPad: "3"}, {SymbolPin: "4", Function: "SCL", FootprintPad: "4"}, {SymbolPin: "5", Function: "SDO", FootprintPad: "5"}, {SymbolPin: "6", Function: "VDDIO", FootprintPad: "6"}, {SymbolPin: "7", Function: "GND", FootprintPad: "7"}, {SymbolPin: "8", Function: "VDD", FootprintPad: "8"}}},
	{Symbol: "MCU_Microchip_ATmega:ATmega328P-A", Footprint: "Package_QFP:TQFP-32_7x7mm_P0.8mm", Source: "human_verified", Notes: "ATmega328P-A TQFP-32 direct symbol-pin to footprint-pad mapping. Peripheral role assignment remains catalog/block constrained.", Pins: []Pin{{SymbolPin: "1", Function: "GPIO_1", FootprintPad: "1"}, {SymbolPin: "2", Function: "GPIO_2", FootprintPad: "2"}, {SymbolPin: "3", Function: "GND", FootprintPad: "3"}, {SymbolPin: "4", Function: "VCC", FootprintPad: "4"}, {SymbolPin: "5", Function: "GND", FootprintPad: "5"}, {SymbolPin: "6", Function: "VCC", FootprintPad: "6"}, {SymbolPin: "7", Function: "GPIO_7", FootprintPad: "7"}, {SymbolPin: "8", Function: "GPIO_8", FootprintPad: "8"}, {SymbolPin: "9", Function: "GPIO_9", FootprintPad: "9"}, {SymbolPin: "10", Function: "GPIO_10", FootprintPad: "10"}, {SymbolPin: "11", Function: "GPIO_11", FootprintPad: "11"}, {SymbolPin: "12", Function: "GPIO_12", FootprintPad: "12"}, {SymbolPin: "13", Function: "GPIO_13", FootprintPad: "13"}, {SymbolPin: "14", Function: "GPIO_14", FootprintPad: "14"}, {SymbolPin: "15", Function: "MOSI", FootprintPad: "15"}, {SymbolPin: "16", Function: "MISO", FootprintPad: "16"}, {SymbolPin: "17", Function: "SCK", FootprintPad: "17"}, {SymbolPin: "18", Function: "AVCC", FootprintPad: "18"}, {SymbolPin: "19", Function: "GPIO_19", FootprintPad: "19"}, {SymbolPin: "20", Function: "AREF", FootprintPad: "20"}, {SymbolPin: "21", Function: "AGND", FootprintPad: "21"}, {SymbolPin: "22", Function: "GPIO_22", FootprintPad: "22"}, {SymbolPin: "23", Function: "GPIO_23", FootprintPad: "23"}, {SymbolPin: "24", Function: "GPIO_24", FootprintPad: "24"}, {SymbolPin: "25", Function: "GPIO_25", FootprintPad: "25"}, {SymbolPin: "26", Function: "GPIO_26", FootprintPad: "26"}, {SymbolPin: "27", Function: "GPIO_27", FootprintPad: "27"}, {SymbolPin: "28", Function: "GPIO_28", FootprintPad: "28"}, {SymbolPin: "29", Function: "RESET", FootprintPad: "29"}, {SymbolPin: "30", Function: "GPIO_30", FootprintPad: "30"}, {SymbolPin: "31", Function: "GPIO_31", FootprintPad: "31"}, {SymbolPin: "32", Function: "GPIO_32", FootprintPad: "32"}}},
	{Symbol: "Device:Crystal", Footprint: "Crystal:Crystal_SMD_5032-2Pin_5.0x3.2mm", Source: "human_verified", Notes: "Generic two-pin crystal mapping.", Pins: []Pin{{SymbolPin: "1", Function: "XTAL_1", FootprintPad: "1"}, {SymbolPin: "2", Function: "XTAL_2", FootprintPad: "2"}}},
	{Symbol: "Connector:USB_C_Receptacle_PowerOnly_6P", Footprint: "Connector_USB:USB_C_Receptacle_GCT_USB4125-xx-x_6P_TopMnt_Horizontal", Source: "human_verified", Notes: "Power-only USB-C 6-pin receptacle mapping. Not valid for USB data designs.", Pins: []Pin{{SymbolPin: "A5", Function: "CC1", FootprintPad: "A5"}, {SymbolPin: "A9", Function: "VBUS", FootprintPad: "A9"}, {SymbolPin: "A12", Function: "GND", FootprintPad: "A12"}, {SymbolPin: "B5", Function: "CC2", FootprintPad: "B5"}, {SymbolPin: "B9", Function: "VBUS", FootprintPad: "B9"}, {SymbolPin: "B12", Function: "GND", FootprintPad: "B12"}, {SymbolPin: "SH", Function: "SHIELD", FootprintPad: "SH"}}},
	{Symbol: "Connector:Conn_01x01", Footprint: "Connector_PinHeader_2.54mm:PinHeader_1x01_P2.54mm_Vertical", Source: "human_verified", Notes: "Single-pin connector maps pin 1 to pad 1.", Pins: []Pin{{SymbolPin: "1", FootprintPad: "1"}}},
	{Symbol: "Connector_Generic:Conn_01x02", Footprint: "Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical", Source: "human_verified", Notes: "Two-pin header maps pins directly by number.", Pins: []Pin{{SymbolPin: "1", Function: "PIN_1", FootprintPad: "1"}, {SymbolPin: "2", Function: "PIN_2", FootprintPad: "2"}}},
	{Symbol: "Connector:Conn_01x02", Footprint: "Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical", Source: "human_verified", Notes: "Two-pin header maps pins directly by number.", Pins: []Pin{{SymbolPin: "1", FootprintPad: "1"}, {SymbolPin: "2", FootprintPad: "2"}}},
	{Symbol: "Connector_Generic:Conn_01x03", Footprint: "Connector_PinHeader_2.54mm:PinHeader_1x03_P2.54mm_Vertical", Source: "human_verified", Notes: "Three-pin header maps pins directly by number.", Pins: []Pin{{SymbolPin: "1", Function: "PIN_1", FootprintPad: "1"}, {SymbolPin: "2", Function: "PIN_2", FootprintPad: "2"}, {SymbolPin: "3", Function: "PIN_3", FootprintPad: "3"}}},
	{Symbol: "Connector_Generic:Conn_01x04", Footprint: "Connector_PinHeader_2.54mm:PinHeader_1x04_P2.54mm_Vertical", Source: "human_verified", Notes: "Four-pin header maps pins directly by number.", Pins: []Pin{{SymbolPin: "1", Function: "PIN_1", FootprintPad: "1"}, {SymbolPin: "2", Function: "PIN_2", FootprintPad: "2"}, {SymbolPin: "3", Function: "PIN_3", FootprintPad: "3"}, {SymbolPin: "4", Function: "PIN_4", FootprintPad: "4"}}},
	{Symbol: "Connector_Generic:Conn_01x05", Footprint: "Connector_PinHeader_2.54mm:PinHeader_1x05_P2.54mm_Vertical", Source: "human_verified", Notes: "Five-pin header maps pins directly by number.", Pins: []Pin{{SymbolPin: "1", Function: "PIN_1", FootprintPad: "1"}, {SymbolPin: "2", Function: "PIN_2", FootprintPad: "2"}, {SymbolPin: "3", Function: "PIN_3", FootprintPad: "3"}, {SymbolPin: "4", Function: "PIN_4", FootprintPad: "4"}, {SymbolPin: "5", Function: "PIN_5", FootprintPad: "5"}}},
	{Symbol: "Device:Q_NPN_BEC", Footprint: "Package_TO_SOT_THT:TO-92_Inline", Source: "human_verified", Notes: "Verify against selected transistor datasheet.", Pins: []Pin{{SymbolPin: "1", Function: "E", FootprintPad: "1"}, {SymbolPin: "2", Function: "B", FootprintPad: "2"}, {SymbolPin: "3", Function: "C", FootprintPad: "3"}}},
}

var builtinIndex = entryIndex(builtinEntries)

func Builtins() []Entry {
	entries := make([]Entry, len(builtinEntries))
	for i, entry := range builtinEntries {
		entries[i] = entry
		entries[i].Pins = append([]Pin(nil), entry.Pins...)
	}
	return entries
}

func ValidateProject(path string) (Report, error) {
	return ValidateProjectWithOptions(path, ValidateOptions{})
}

func ValidateProjectWithOptions(path string, opts ValidateOptions) (Report, error) {
	design, err := kicaddesign.ReadProjectDirectory(path)
	if err != nil {
		return Report{}, err
	}
	report := Report{Target: path, Mappings: []Mapping{}, Issues: []reports.Issue{}}
	report.Issues = append(report.Issues, opts.LibraryIssues...)
	if design.Schematic == nil {
		report.Issues = append(report.Issues, reports.Issue{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: "pinmap.project", Message: "project has no root schematic"})
		finalize(&report)
		return report, nil
	}
	if len(design.Schematic.Sheets) > 0 {
		report.Issues = append(report.Issues, reports.Issue{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityBlocked,
			Path:       "pinmap.hierarchy",
			Message:    "hierarchical pinmap validation is not implemented; child sheets may contain unverified mappings",
			Suggestion: "run pinmap validation on child sheets explicitly until hierarchy flattening is implemented",
		})
	}
	for _, symbol := range design.Schematic.Symbols {
		footprint := symbolFootprint(symbol)
		if strings.TrimSpace(footprint) == "" {
			continue
		}
		key := mappingKey(symbol.LibraryID, footprint)
		entry, ok := builtinIndex[key]
		mapping := Mapping{Ref: symbol.Reference, Symbol: symbol.LibraryID, Footprint: footprint, PinCount: len(symbol.Pins)}
		resolverOK := true
		if opts.LibraryIndex != nil {
			resolverOK = resolverRecordsExist(opts.LibraryIndex, symbol.LibraryID, footprint, &report, symbol.Reference)
			if !resolverOK && !ok {
				mapping.Status = "missing"
				report.Mappings = append(report.Mappings, mapping)
				continue
			}
		}
		if !ok {
			if opts.LibraryIndex != nil {
				addResolverCandidate(opts.LibraryIndex, &report, &mapping, symbol.Reference)
			} else {
				addUnverifiedIssue(&report, symbol.Reference, symbol.LibraryID, footprint, "pinmap is not verified")
				mapping.Status = "unverified"
			}
			report.Mappings = append(report.Mappings, mapping)
			continue
		}
		mapping.Status = "verified"
		mapping.Source = entry.Source
		mapping.Notes = entry.Notes
		mapping.MapCount = len(entry.Pins)
		if opts.LibraryIndex != nil && resolverOK {
			compatibility := libraryresolver.ValidateAssignment(*opts.LibraryIndex, symbol.LibraryID, footprint)
			mapping.ResolverEvidence = compatibility.Evidence
			for _, padIssue := range footprintPadIssues(opts.LibraryIndex, symbol.Reference, entry) {
				mapping.Status = "mismatch"
				report.Issues = append(report.Issues, padIssue)
			}
		}
		if len(symbol.Pins) != len(entry.Pins) {
			mapping.Status = "mismatch"
			report.Issues = append(report.Issues, reports.Issue{
				Code:     reports.CodePinmapUnverified,
				Severity: reports.SeverityBlocked,
				Path:     "pinmap." + symbol.Reference,
				Message:  fmt.Sprintf("pinmap pin count mismatch for %s -> %s: symbol has %d pins, mapping has %d", symbol.LibraryID, footprint, len(symbol.Pins), len(entry.Pins)),
				Refs:     []string{symbol.Reference},
			})
		}
		for _, pinIssue := range pinIdentifierIssues(symbol, entry) {
			mapping.Status = "mismatch"
			report.Issues = append(report.Issues, pinIssue)
		}
		report.Mappings = append(report.Mappings, mapping)
	}
	finalize(&report)
	return report, nil
}

func resolverRecordsExist(index *libraryresolver.LibraryIndex, symbolID string, footprintID string, report *Report, ref string) bool {
	ok := true
	if _, found := libraryresolver.ResolveSymbol(*index, symbolID); !found {
		ok = false
		report.Issues = append(report.Issues, reports.Issue{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: "pinmap." + ref, Message: "symbol library record not found: " + symbolID, Refs: []string{ref}})
	}
	if _, found := libraryresolver.ResolveFootprint(*index, footprintID); !found {
		ok = false
		report.Issues = append(report.Issues, reports.Issue{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: "pinmap." + ref, Message: "footprint library record not found: " + footprintID, Refs: []string{ref}})
	}
	return ok
}

func addResolverCandidate(index *libraryresolver.LibraryIndex, report *Report, mapping *Mapping, ref string) {
	candidate := libraryresolver.GeneratePinmapCandidate(*index, mapping.Symbol, mapping.Footprint)
	mapping.ResolverEvidence = candidate.Evidence
	mapping.CandidatePinmap = candidate.PinmapCandidate
	mapping.MapCount = len(candidate.PinmapCandidate)
	if candidate.Status == libraryresolver.CompatibilityCandidate && len(candidate.PinmapCandidate) > 0 {
		mapping.Status = "candidate"
		addUnverifiedIssue(report, ref, mapping.Symbol, mapping.Footprint, "pinmap candidate is inferred but not verified")
		return
	}
	mapping.Status = "mismatch"
	report.Issues = append(report.Issues, candidate.Issues...)
}

func addUnverifiedIssue(report *Report, ref string, symbolID string, footprintID string, prefix string) {
	report.Issues = append(report.Issues, reports.Issue{
		Code:       reports.CodePinmapUnverified,
		Severity:   reports.SeverityBlocked,
		Path:       "pinmap." + ref,
		Message:    fmt.Sprintf("%s for %s -> %s", prefix, symbolID, footprintID),
		Refs:       []string{ref},
		Suggestion: "add a human-verified pinmap entry before fabrication export",
	})
}

func footprintPadIssues(index *libraryresolver.LibraryIndex, ref string, entry Entry) []reports.Issue {
	footprint, ok := libraryresolver.ResolveFootprint(*index, entry.Footprint)
	if !ok {
		return nil
	}
	pads := map[string]struct{}{}
	for _, pad := range footprint.Pads {
		pads[strings.TrimSpace(pad.Name)] = struct{}{}
	}
	var issues []reports.Issue
	for _, pin := range entry.Pins {
		if _, ok := pads[strings.TrimSpace(pin.FootprintPad)]; ok {
			continue
		}
		issues = append(issues, reports.Issue{
			Code:     reports.CodePinmapUnverified,
			Severity: reports.SeverityBlocked,
			Path:     "pinmap." + ref,
			Message:  fmt.Sprintf("footprint pad %s is not present in verified pinmap for %s -> %s", pin.FootprintPad, entry.Symbol, entry.Footprint),
			Refs:     []string{ref},
		})
	}
	return issues
}

func entryIndex(entries []Entry) map[string]Entry {
	index := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		index[mappingKey(entry.Symbol, entry.Footprint)] = entry
	}
	return index
}

func mappingKey(symbol string, footprint string) string {
	return strings.TrimSpace(symbol) + "\x00" + strings.TrimSpace(footprint)
}

func symbolFootprint(symbol schematic.SchematicSymbol) string {
	for _, property := range symbol.Properties {
		if strings.EqualFold(strings.TrimSpace(property.Name), "Footprint") {
			return strings.TrimSpace(property.Value)
		}
	}
	return ""
}

func pinIdentifierIssues(symbol schematic.SchematicSymbol, entry Entry) []reports.Issue {
	expected := make(map[string]struct{}, len(entry.Pins))
	for _, pin := range entry.Pins {
		expected[strings.TrimSpace(pin.SymbolPin)] = struct{}{}
	}
	var issues []reports.Issue
	for _, pin := range symbol.Pins {
		number := strings.TrimSpace(pin.Number)
		if _, ok := expected[number]; ok {
			continue
		}
		issues = append(issues, reports.Issue{
			Code:     reports.CodePinmapUnverified,
			Severity: reports.SeverityBlocked,
			Path:     "pinmap." + symbol.Reference,
			Message:  fmt.Sprintf("pin %s is not present in verified pinmap for %s -> %s", number, symbol.LibraryID, entry.Footprint),
			Refs:     []string{symbol.Reference},
		})
	}
	return issues
}

func finalize(report *Report) {
	if reports.HasBlockingIssue(report.Issues) {
		report.FabricationReady = false
		report.FabricationReadyReason = "pinmap verification has blocking issues"
		return
	}
	report.FabricationReady = true
}
