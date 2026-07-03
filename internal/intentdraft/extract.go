package intentdraft

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"kicadai/internal/designworkflow"
	"kicadai/internal/intentplanner"
)

const (
	nearbyVoltageThresholdBytes = 24
	defaultIndicatorVoltage     = "3.3V"
)

var ledIndicatorPhrases = []string{"led", "status light", "indicator"}

func extractStructuredIntent(source string, normalized string, request *intentplanner.Request, extraction *ExtractionReport) {
	request.Name = deriveName(normalized)
	request.Kind = deriveKind(normalized)
	kindEvidence := findKindEvidence(source, normalized)
	addField(extraction, "name", request.Name, source, kindEvidence, confidenceRegexMedium, "inferred.name")
	addField(extraction, "kind", request.Kind, source, kindEvidence, confidenceRegexMedium, "keyword.kind")

	if containsAny(normalized, "fab ready", "fabrication", "manufacturable", "order from fab") {
		request.Acceptance = designworkflow.AcceptanceFabricationCandidate
		request.Manufacturing.FabricationCandidate = true
		addField(extraction, "manufacturing.fabrication_candidate", true, source, findFirstPhrase(source, []string{"fab ready", "fabrication", "manufacturable", "order from fab"}), confidenceRegexHigh, "keyword.fabrication")
	}

	for _, dim := range findDimensions(source) {
		switch dim.Field.Path {
		case "board.width_mm":
			request.Board.WidthMM = dim.FloatValue
		case "board.height_mm":
			request.Board.HeightMM = dim.FloatValue
		}
		extraction.Fields = append(extraction.Fields, dim.Field)
	}
	if layers := findLayers(source); len(layers) > 0 {
		request.Board.Layers = layers[0].IntValue
		field := layers[0].Field
		field.Value = layers[0].IntValue
		extraction.Fields = append(extraction.Fields, field)
	}
	if containsAny(normalized, "no mounting holes", "without mounting holes") {
		request.Board.MountingHoles = intentplanner.StrengthForbidden
		addField(extraction, "board.mounting_holes", request.Board.MountingHoles, source, findFirstPhrase(source, []string{"no mounting holes", "without mounting holes"}), confidenceRegexHigh, "keyword.mounting_holes")
	} else if containsAny(normalized, "mounting holes", "mount holes") {
		request.Board.MountingHoles = intentplanner.StrengthRequired
		addField(extraction, "board.mounting_holes", request.Board.MountingHoles, source, findFirstPhrase(source, []string{"mounting holes", "mount holes"}), confidenceRegexHigh, "keyword.mounting_holes")
	}

	extractPower(source, normalized, request, extraction)
	extractInterfaces(source, normalized, request, extraction)
	extractFunctions(source, normalized, request, extraction)
	defaultIndicatorSupply(source, normalized, request, extraction)
}

func extractPower(source string, normalized string, request *intentplanner.Request, extraction *ExtractionReport) {
	voltages := findVoltages(source)
	for _, voltage := range voltages {
		extraction.Fields = append(extraction.Fields, voltage.Field)
	}
	if containsAny(normalized, "usb-c", "usb c", "usbc") {
		input := intentplanner.PowerInputIntent{Kind: "usb_c", Voltage: firstVoltageOr(voltages, "5V")}
		request.Power.Inputs = append(request.Power.Inputs, input)
		addField(extraction, fmt.Sprintf("power.inputs[%d].kind", len(request.Power.Inputs)-1), input.Kind, source, findFirstPhrase(source, []string{"usb-c", "usb c", "usbc"}), confidenceRegexHigh, "keyword.power_input")
	}
	if containsAny(normalized, "battery") {
		input := intentplanner.PowerInputIntent{Kind: "battery", Voltage: nearbyVoltageOr(source, voltages, "battery", "")}
		request.Power.Inputs = append(request.Power.Inputs, input)
		addField(extraction, fmt.Sprintf("power.inputs[%d].kind", len(request.Power.Inputs)-1), input.Kind, source, findPhrase(source, "battery"), confidenceRegexHigh, "keyword.power_input")
	}
	if len(request.Power.Inputs) == 0 && len(voltages) > 0 {
		input := intentplanner.PowerInputIntent{Kind: "external", Voltage: voltages[0].TextValue}
		request.Power.Inputs = append(request.Power.Inputs, input)
		addField(extraction, fmt.Sprintf("power.inputs[%d].kind", len(request.Power.Inputs)-1), input.Kind, source, voltages[0].Field, confidenceRegexLow, "inferred.power_input")
	}
	if len(voltages) > 0 {
		railVoltage := voltages[len(voltages)-1].TextValue
		rail := intentplanner.PowerRailIntent{Name: "VCC", Voltage: railVoltage, Alias: voltageAlias(railVoltage)}
		if containsAny(normalized, "sensor") {
			rail.SuppliedTargets = []intentplanner.TargetRef{{Role: "sensor"}}
		}
		request.Power.Rails = append(request.Power.Rails, rail)
		addField(extraction, fmt.Sprintf("power.rails[%d].voltage", len(request.Power.Rails)-1), rail.Voltage, source, voltages[len(voltages)-1].Field, confidenceRegexHigh, "regex.voltage")
	}
}

func extractInterfaces(source string, normalized string, request *intentplanner.Request, extraction *ExtractionReport) {
	if containsAny(normalized, "i2c", "iic", "qwiic", "stemma") {
		iface := intentplanner.InterfaceIntent{Kind: "i2c", Voltage: inferredInterfaceVoltage(request), Bus: "i2c1"}
		request.Interfaces = append(request.Interfaces, iface)
		addField(extraction, fmt.Sprintf("interfaces[%d].kind", len(request.Interfaces)-1), iface.Kind, source, findFirstPhrase(source, []string{"i2c", "iic", "qwiic", "stemma"}), confidenceRegexHigh, "keyword.interface")
	}
	if ledIndicatorRequested(normalized) && !containsAny(normalized, "i2c", "qwiic", "stemma", "spi", "uart") && !requestHasInterface(request, "gpio") {
		iface := intentplanner.InterfaceIntent{Kind: "gpio", Voltage: inferredInterfaceVoltage(request)}
		if iface.Voltage == "" {
			iface.Voltage = defaultIndicatorVoltage
		}
		request.Interfaces = append(request.Interfaces, iface)
		addField(extraction, fmt.Sprintf("interfaces[%d].kind", len(request.Interfaces)-1), iface.Kind, source, findFirstPhrase(source, ledIndicatorPhrases), confidenceRegexMedium, "inferred.interface")
	}
	if containsAny(normalized, "uart") {
		iface := intentplanner.InterfaceIntent{Kind: "uart", Voltage: inferredInterfaceVoltage(request)}
		request.Interfaces = append(request.Interfaces, iface)
		addField(extraction, fmt.Sprintf("interfaces[%d].kind", len(request.Interfaces)-1), iface.Kind, source, findPhrase(source, "uart"), confidenceRegexHigh, "keyword.interface")
	}
}

func extractFunctions(source string, normalized string, request *intentplanner.Request, extraction *ExtractionReport) {
	if containsAny(normalized, "mcu", "microcontroller", "atmega", "arduino") {
		function := intentplanner.FunctionIntent{Kind: "mcu", Family: detectMCUFamily(normalized)}
		request.Functions = append(request.Functions, function)
		addField(extraction, fmt.Sprintf("functions[%d].kind", len(request.Functions)-1), function.Kind, source, findFirstPhrase(source, []string{"mcu", "microcontroller", "atmega", "arduino"}), confidenceRegexHigh, "keyword.function")
	}
	if containsAny(normalized, "sensor", "temperature", "humidity", "pressure") {
		function := intentplanner.FunctionIntent{Kind: "sensor", Family: "i2c_sensor", Interface: maybeI2C(normalized), Bus: maybeBus(normalized), Supply: firstRailAlias(request)}
		if function.Interface == "i2c" && containsAny(normalized, "temperature") {
			function.Params = map[string]any{"i2c_address": "0x48"}
		}
		request.Functions = append(request.Functions, function)
		addField(extraction, fmt.Sprintf("functions[%d].kind", len(request.Functions)-1), function.Kind, source, findFirstPhrase(source, []string{"sensor", "temperature", "humidity", "pressure"}), confidenceRegexHigh, "keyword.function")
	}
	if containsAny(normalized, "programmer", "programming", "isp") {
		function := intentplanner.FunctionIntent{Kind: "reset_programming", Family: "isp", Params: map[string]any{"programming_interface": "isp"}}
		request.Functions = append(request.Functions, function)
		addField(extraction, fmt.Sprintf("functions[%d].kind", len(request.Functions)-1), function.Kind, source, findFirstPhrase(source, []string{"programmer", "programming", "isp"}), confidenceRegexHigh, "keyword.function")
	}
	if frequencies := findFrequencies(source); len(frequencies) > 0 {
		function := intentplanner.FunctionIntent{Kind: "clock", Family: "crystal_oscillator", Params: map[string]any{"frequency": frequencies[0].TextValue}}
		request.Functions = append(request.Functions, function)
		addField(extraction, fmt.Sprintf("functions[%d].params.frequency", len(request.Functions)-1), frequencies[0].TextValue, source, frequencies[0].Field, confidenceRegexMedium, "regex.frequency")
	}
	if containsAny(normalized, "regulator", "ldo", "buck") {
		function := intentplanner.FunctionIntent{Kind: "regulator"}
		request.Functions = append(request.Functions, function)
		addField(extraction, fmt.Sprintf("functions[%d].kind", len(request.Functions)-1), function.Kind, source, findFirstPhrase(source, []string{"regulator", "ldo", "buck"}), confidenceRegexHigh, "keyword.function")
	}
	if ledIndicatorRequested(normalized) {
		function := intentplanner.FunctionIntent{Kind: "indicator"}
		request.Functions = append(request.Functions, function)
		addField(extraction, fmt.Sprintf("functions[%d].kind", len(request.Functions)-1), function.Kind, source, findFirstPhrase(source, ledIndicatorPhrases), confidenceRegexHigh, "keyword.function")
	}
	if containsAny(normalized, "amplifier", "op amp", "op-amp", "gain stage", "headphone", "headphones") {
		function := intentplanner.FunctionIntent{Kind: "amplifier", Family: "op_amp_gain_stage"}
		if containsAny(normalized, "class ab", "class-ab") && containsAny(normalized, "headphone", "headphones") {
			function.Family = "class_ab_headphone"
			function.Params = classABHeadphoneParamsFromSource(source)
		}
		gains := findGains(source)
		if len(gains) > 0 {
			if function.Params == nil {
				function.Params = map[string]any{}
			}
			function.Params["gain"] = gains[0].FloatValue
		}
		request.Functions = append(request.Functions, function)
		index := len(request.Functions) - 1
		if len(gains) > 0 {
			addField(extraction, fmt.Sprintf("functions[%d].params.gain", index), gains[0].FloatValue, source, gains[0].Field, confidenceRegexMedium, "regex.gain")
		}
		addField(extraction, fmt.Sprintf("functions[%d].kind", index), function.Kind, source, findFirstPhrase(source, []string{"amplifier", "op amp", "op-amp", "gain stage", "headphone", "headphones"}), confidenceRegexHigh, "keyword.function")
	}
	if containsAny(normalized, "esd") {
		request.Protection.ESD = intentplanner.StrengthRequired
		addField(extraction, "protection.esd", request.Protection.ESD, source, findPhrase(source, "esd"), confidenceRegexHigh, "keyword.protection")
	}
	if containsAny(normalized, "reverse polarity", "reverse-polarity") {
		request.Protection.ReversePolarity = intentplanner.StrengthRequired
		addField(extraction, "protection.reverse_polarity", request.Protection.ReversePolarity, source, findFirstPhrase(source, []string{"reverse polarity", "reverse-polarity"}), confidenceRegexHigh, "keyword.protection")
	}
}

func defaultIndicatorSupply(source string, normalized string, request *intentplanner.Request, extraction *ExtractionReport) {
	if !requestHasFunction(request, "indicator") || len(request.Power.Inputs) > 0 || len(request.Power.Rails) > 0 {
		return
	}
	if containsAny(normalized, highVoltagePhrases...) {
		return
	}
	voltage := inferredInterfaceVoltage(request)
	if voltage == "" {
		voltage = defaultIndicatorVoltage
	}
	input := intentplanner.PowerInputIntent{Kind: "external", Voltage: voltage}
	request.Power.Inputs = append(request.Power.Inputs, input)
	inputIndex := len(request.Power.Inputs) - 1
	addField(extraction, fmt.Sprintf("power.inputs[%d].kind", inputIndex), input.Kind, source, ExtractedField{}, confidenceRegexLow, "default.power_input")
	rail := intentplanner.PowerRailIntent{Name: "VCC", Voltage: voltage, Alias: voltageAlias(voltage), SuppliedTargets: []intentplanner.TargetRef{{Role: "indicator"}}}
	request.Power.Rails = append(request.Power.Rails, rail)
	railIndex := len(request.Power.Rails) - 1
	addField(extraction, fmt.Sprintf("power.rails[%d].voltage", railIndex), rail.Voltage, source, ExtractedField{}, confidenceRegexLow, "default.power_rail")
}

func requestHasFunction(request *intentplanner.Request, kind string) bool {
	for _, function := range request.Functions {
		if function.Kind == kind {
			return true
		}
	}
	return false
}

func requestHasInterface(request *intentplanner.Request, kind string) bool {
	for _, iface := range request.Interfaces {
		if iface.Kind == kind {
			return true
		}
	}
	return false
}

func ledIndicatorRequested(normalized string) bool {
	return containsAny(normalized, ledIndicatorPhrases...)
}

func classABHeadphoneParamsFromSource(source string) map[string]any {
	params := map[string]any{
		"load_kind":               "headphone",
		"load_impedance":          "32Ω",
		"supply_voltage":          "9V",
		"fault_protection_status": "placeholder_blocked",
	}
	if voltage := firstExtractedVoltage(source); voltage != "" {
		params["supply_voltage"] = voltage
	}
	if load := firstExtractedHeadphoneLoad(source); load != "" {
		params["load_impedance"] = load
	}
	return params
}

func firstExtractedVoltage(source string) string {
	voltages := findVoltages(source)
	if len(voltages) == 0 {
		return ""
	}
	return voltages[0].TextValue
}

func firstExtractedHeadphoneLoad(source string) string {
	for _, value := range findRCValues(source) {
		normalized := strings.ToLower(strings.ReplaceAll(value.TextValue, " ", ""))
		normalized = strings.TrimSuffix(normalized, "s")
		switch {
		case strings.HasSuffix(normalized, "ohm"):
			return strings.TrimSuffix(normalized, "ohm") + "Ω"
		case strings.HasSuffix(normalized, "r"):
			return strings.TrimSuffix(normalized, "r") + "Ω"
		}
	}
	return ""
}

func deriveKind(normalized string) intentplanner.IntentKind {
	switch {
	case containsAny(normalized, "amplifier", "op amp", "op-amp", "gain stage", "headphone", "headphones"):
		return intentplanner.IntentAmplifier
	case containsAny(normalized, "power module", "power supply", "regulator", "ldo", "buck"):
		return intentplanner.IntentPowerModule
	case containsAny(normalized, "mcu", "microcontroller", "atmega", "arduino", "programmer", "programming"):
		return intentplanner.IntentMCUMinimal
	case containsAny(normalized, "sensor", "temperature", "humidity", "pressure"):
		return intentplanner.IntentSensorNode
	case ledIndicatorRequested(normalized):
		return intentplanner.IntentBreakout
	case containsAny(normalized, "breakout", "adapter", "connector"):
		return intentplanner.IntentBreakout
	default:
		return intentplanner.IntentCustomStructured
	}
}

func deriveName(normalized string) string {
	switch deriveKind(normalized) {
	case intentplanner.IntentAmplifier:
		return "amplifier_module"
	case intentplanner.IntentPowerModule:
		return "power_module"
	case intentplanner.IntentMCUMinimal:
		return "mcu_minimal"
	case intentplanner.IntentSensorNode:
		if containsPhrase(normalized, "i2c") {
			return "i2c_sensor_breakout"
		}
		return "sensor_node"
	case intentplanner.IntentBreakout:
		if ledIndicatorRequested(normalized) {
			return "led_indicator"
		}
		return "connector_breakout"
	default:
		return "natural_language_intent"
	}
}

func summarizeConfidence(fields []ExtractedField) ConfidenceSummary {
	if len(fields) == 0 {
		zero := 0.0
		return ConfidenceSummary{Overall: &zero, Minimum: &zero}
	}
	total := 0.0
	minimum := 1.0
	for _, field := range fields {
		total += field.Confidence
		if field.Confidence < minimum {
			minimum = field.Confidence
		}
	}
	overall := total / float64(len(fields))
	return ConfidenceSummary{Overall: &overall, Minimum: &minimum, Fields: len(fields)}
}

func addField(extraction *ExtractionReport, path string, value any, source string, evidence ExtractedField, confidence float64, method string) {
	field := evidence
	field.Path = path
	field.Value = value
	field.Confidence = confidence
	field.Method = method
	if field.SourceText == "" {
		field.Notes = append(field.Notes, "source evidence unavailable")
	}
	extraction.Fields = append(extraction.Fields, field)
}

func findKindEvidence(source string, normalized string) ExtractedField {
	return findFirstPhrase(source, []string{"amplifier", "op amp", "op-amp", "gain stage", "headphone", "headphones", "power module", "power supply", "regulator", "ldo", "buck", "mcu", "microcontroller", "atmega", "arduino", "programmer", "programming", "sensor", "temperature", "humidity", "pressure", "breakout", "adapter", "connector"})
}

type phraseFinder struct {
	source string
}

func newPhraseFinder(source string) phraseFinder {
	return phraseFinder{source: source}
}

func (finder phraseFinder) findPhrase(phrase string) ExtractedField {
	return findPhrase(finder.source, phrase)
}

func (finder phraseFinder) findFirstPhrase(phrases []string) ExtractedField {
	return findFirstPhrase(finder.source, phrases)
}

func findFirstPhrase(source string, phrases []string) ExtractedField {
	for _, phrase := range phrases {
		field := findPhrase(source, phrase)
		if field.SourceText != "" {
			return field
		}
	}
	return ExtractedField{}
}

func findPhrase(source string, phrase string) ExtractedField {
	index := equalFoldIndex(source, phrase)
	if index < 0 {
		return ExtractedField{}
	}
	return ExtractedField{SourceText: source[index : index+len(phrase)], StartByte: index, EndByte: index + len(phrase)}
}

func equalFoldIndex(source string, phrase string) int {
	if phrase == "" {
		return 0
	}
	for index := range source {
		end := index + len(phrase)
		if end > len(source) {
			return -1
		}
		if strings.EqualFold(source[index:end], phrase) {
			return index
		}
	}
	return -1
}

func containsAny(text string, phrases ...string) bool {
	for _, phrase := range phrases {
		if containsPhrase(text, strings.ToLower(phrase)) {
			return true
		}
	}
	return false
}

func containsPhrase(text string, phrase string) bool {
	text = strings.ToLower(text)
	phrase = strings.ToLower(phrase)
	index := strings.Index(text, phrase)
	for index >= 0 {
		startOK := index == 0 || !previousTokenChar(text, index)
		end := index + len(phrase)
		endOK := end == len(text) || !nextTokenChar(text, end)
		if startOK && endOK {
			return true
		}
		next := strings.Index(text[index+1:], phrase)
		if next < 0 {
			return false
		}
		index += next + 1
	}
	return false
}

func previousTokenChar(text string, index int) bool {
	r, _ := utf8.DecodeLastRuneInString(text[:index])
	return isTokenRune(r)
}

func nextTokenChar(text string, index int) bool {
	r, _ := utf8.DecodeRuneInString(text[index:])
	return isTokenRune(r)
}

func isTokenRune(value rune) bool {
	return unicode.IsLetter(value) || unicode.IsDigit(value) || value == '_'
}

func firstVoltageOr(values []primitiveValue, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return values[0].TextValue
}

func nearbyVoltageOr(source string, voltages []primitiveValue, phrase string, fallback string) string {
	phraseField := findPhrase(source, phrase)
	if phraseField.SourceText == "" {
		return fallback
	}
	for _, voltage := range voltages {
		if absInt(voltage.Field.StartByte-phraseField.StartByte) <= nearbyVoltageThresholdBytes || absInt(voltage.Field.EndByte-phraseField.EndByte) <= nearbyVoltageThresholdBytes {
			return voltage.TextValue
		}
	}
	return fallback
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func voltageAlias(voltage string) string {
	switch strings.ToUpper(voltage) {
	case "3.3V":
		return "3v3"
	case "5V":
		return "vcc"
	default:
		return strings.ToLower(strings.TrimSuffix(voltage, "V"))
	}
}

func inferredInterfaceVoltage(request *intentplanner.Request) string {
	if len(request.Power.Rails) > 0 {
		return request.Power.Rails[len(request.Power.Rails)-1].Voltage
	}
	if len(request.Power.Inputs) > 0 {
		return request.Power.Inputs[0].Voltage
	}
	return ""
}

func maybeI2C(normalized string) string {
	if containsAny(normalized, "i2c", "iic", "qwiic", "stemma") {
		return "i2c"
	}
	return ""
}

func maybeBus(normalized string) string {
	if maybeI2C(normalized) != "" {
		return "i2c1"
	}
	return ""
}

func firstRailAlias(request *intentplanner.Request) string {
	if len(request.Power.Rails) == 0 {
		return ""
	}
	return request.Power.Rails[0].Alias
}

func detectMCUFamily(normalized string) string {
	switch {
	case containsAny(normalized, "atmega", "arduino"):
		return "atmega328p"
	case containsAny(normalized, "rp2040"):
		return "rp2040"
	case containsAny(normalized, "stm32"):
		return "stm32"
	case containsAny(normalized, "esp32"):
		return "esp32"
	default:
		return ""
	}
}
