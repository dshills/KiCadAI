package intentdraft

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

type primitiveValue struct {
	Field      ExtractedField
	TextValue  string
	FloatValue float64
	IntValue   int
	Unit       string
}

const (
	confidenceRegexHigh   = 0.9
	confidenceRegexMedium = 0.85
	confidenceRegexLow    = 0.75
)

var (
	voltagePattern   = regexp.MustCompile(`(?i)\b\d+(?:\.\d+)?\s*v\b|\b\d+v\d+\b|\b(vbus|vcc|avcc)\b`)
	currentPattern   = regexp.MustCompile(`(?i)\b(\d+(?:\.\d+)?)\s*(ma|a)\b`)
	dimensionPattern = regexp.MustCompile(`(?i)\b(?P<width>\d+(?:\.\d+)?)\s*(?P<width_unit>mm|in|inch|inches)?\s*(?:x|by)\s*(?P<height>\d+(?:\.\d+)?)\s*(?P<height_unit>mm|in|inch|inches)?\b`)
	layerPattern     = regexp.MustCompile(`(?i)\b(\d+|one|two|four)\s*[- ]?\s*layers?\b`)
	frequencyPattern = regexp.MustCompile(`(?i)\b(\d+(?:\.\d+)?)\s*(mhz|khz|hz)\b`)
	gainPattern      = regexp.MustCompile(`(?i)\b(?:gain(?:\s+of)?|x)\s*(\d+(?:\.\d+)?)\b|\b(\d+(?:\.\d+)?)\s*x\s+gain\b`)
	rcValuePattern   = regexp.MustCompile(`(?i)\b(\d+(?:\.\d+)?)\s*(k|r|ohms?|kohms?|nf|uf|pf)\b`)
	digitVDigit      = regexp.MustCompile(`^\d+V\d+$`)
)

func normalizedText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

func findVoltages(text string) []primitiveValue {
	var values []primitiveValue
	for _, match := range voltagePattern.FindAllStringIndex(text, -1) {
		excerpt := text[match[0]:match[1]]
		value := normalizeVoltage(excerpt)
		values = append(values, primitiveValue{TextValue: value, Unit: "V", Field: fieldFromMatch("", value, excerpt, match, confidenceRegexHigh, "regex.voltage")})
	}
	return values
}

func findCurrents(text string) []primitiveValue {
	var values []primitiveValue
	for _, loc := range currentPattern.FindAllStringSubmatchIndex(text, -1) {
		excerpt := text[loc[0]:loc[1]]
		number, err := strconv.ParseFloat(text[loc[2]:loc[3]], 64)
		if err != nil {
			continue
		}
		unit := strings.ToLower(text[loc[4]:loc[5]])
		currentMA := number
		if unit == "a" {
			currentMA = number * 1000
		}
		values = append(values, primitiveValue{FloatValue: currentMA, Unit: "mA", Field: fieldFromMatch("", currentMA, excerpt, []int{loc[0], loc[1]}, confidenceRegexHigh, "regex.current")})
	}
	return values
}

func findDimensions(text string) []primitiveValue {
	var values []primitiveValue
	for _, loc := range dimensionPattern.FindAllStringSubmatchIndex(text, -1) {
		width, widthErr := strconv.ParseFloat(namedSubmatch(text, loc, dimensionPattern, "width"), 64)
		height, heightErr := strconv.ParseFloat(namedSubmatch(text, loc, dimensionPattern, "height"), 64)
		if widthErr != nil || heightErr != nil {
			continue
		}
		unit := namedSubmatch(text, loc, dimensionPattern, "width_unit")
		if unit == "" {
			unit = namedSubmatch(text, loc, dimensionPattern, "height_unit")
		}
		if unit == "in" || unit == "inch" || unit == "inches" {
			width = roundMM(width * 25.4)
			height = roundMM(height * 25.4)
		}
		widthLoc := namedSubmatchLoc(loc, dimensionPattern, "width")
		heightLoc := namedSubmatchLoc(loc, dimensionPattern, "height")
		values = append(values, primitiveValue{TextValue: "board", FloatValue: width, Unit: "mm", Field: fieldFromMatch("board.width_mm", width, text[widthLoc[0]:widthLoc[1]], widthLoc, confidenceRegexHigh, "regex.dimension")})
		values = append(values, primitiveValue{TextValue: "board_height", FloatValue: height, Unit: "mm", Field: fieldFromMatch("board.height_mm", height, text[heightLoc[0]:heightLoc[1]], heightLoc, confidenceRegexHigh, "regex.dimension")})
	}
	return values
}

func findLayers(text string) []primitiveValue {
	var values []primitiveValue
	for _, loc := range layerPattern.FindAllStringSubmatchIndex(text, -1) {
		excerpt := text[loc[0]:loc[1]]
		layers := parseLayerWord(text[loc[2]:loc[3]])
		values = append(values, primitiveValue{IntValue: layers, Field: fieldFromMatch("board.layers", layers, excerpt, []int{loc[0], loc[1]}, confidenceRegexHigh, "regex.layers")})
	}
	return values
}

func findFrequencies(text string) []primitiveValue {
	var values []primitiveValue
	for _, loc := range frequencyPattern.FindAllStringSubmatchIndex(text, -1) {
		excerpt := text[loc[0]:loc[1]]
		number, err := strconv.ParseFloat(text[loc[2]:loc[3]], 64)
		if err != nil {
			continue
		}
		unit := strings.ToLower(text[loc[4]:loc[5]])
		values = append(values, primitiveValue{TextValue: strings.TrimSpace(excerpt), FloatValue: normalizeFrequencyHz(number, unit), Unit: "Hz", Field: fieldFromMatch("functions[].params.frequency", strings.TrimSpace(excerpt), excerpt, []int{loc[0], loc[1]}, confidenceRegexMedium, "regex.frequency")})
	}
	return values
}

func findGains(text string) []primitiveValue {
	var values []primitiveValue
	for _, loc := range gainPattern.FindAllStringSubmatchIndex(text, -1) {
		excerpt := text[loc[0]:loc[1]]
		numberText := submatchAt(text, loc[2], loc[3])
		if numberText == "" {
			numberText = submatchAt(text, loc[4], loc[5])
		}
		number, err := strconv.ParseFloat(numberText, 64)
		if err != nil {
			continue
		}
		values = append(values, primitiveValue{FloatValue: number, Field: fieldFromMatch("functions[].params.gain", number, excerpt, []int{loc[0], loc[1]}, confidenceRegexMedium, "regex.gain")})
	}
	return values
}

func findRCValues(text string) []primitiveValue {
	var values []primitiveValue
	for _, loc := range rcValuePattern.FindAllStringSubmatchIndex(text, -1) {
		excerpt := text[loc[0]:loc[1]]
		values = append(values, primitiveValue{TextValue: strings.TrimSpace(excerpt), Field: fieldFromMatch("functions[].params.value", strings.TrimSpace(excerpt), excerpt, []int{loc[0], loc[1]}, confidenceRegexLow, "regex.value")})
	}
	return values
}

func fieldFromMatch(path string, value any, excerpt string, match []int, confidence float64, method string) ExtractedField {
	return ExtractedField{Path: path, Value: value, SourceText: excerpt, StartByte: match[0], EndByte: match[1], Confidence: confidence, Method: method}
}

func normalizeVoltage(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	if digitVDigit.MatchString(value) {
		value = strings.Replace(value, "V", ".", 1) + "V"
	}
	return value
}

func namedSubmatch(text string, loc []int, pattern *regexp.Regexp, name string) string {
	matchLoc := namedSubmatchLoc(loc, pattern, name)
	return submatchAt(text, matchLoc[0], matchLoc[1])
}

func namedSubmatchLoc(loc []int, pattern *regexp.Regexp, name string) []int {
	index := pattern.SubexpIndex(name)
	if index <= 0 {
		return []int{-1, -1}
	}
	start := loc[index*2]
	end := loc[index*2+1]
	return []int{start, end}
}

func submatchAt(text string, start int, end int) string {
	if start < 0 || end < 0 || start >= end {
		return ""
	}
	return text[start:end]
}

func parseLayerWord(value string) int {
	switch strings.ToLower(value) {
	case "one":
		return 1
	case "two":
		return 2
	case "four":
		return 4
	default:
		parsed, _ := strconv.Atoi(value)
		return parsed
	}
}

func normalizeFrequencyHz(value float64, unit string) float64 {
	switch strings.ToLower(unit) {
	case "mhz":
		return value * 1_000_000
	case "khz":
		return value * 1_000
	default:
		return value
	}
}

func roundMM(value float64) float64 {
	return math.Round(value*100) / 100
}
