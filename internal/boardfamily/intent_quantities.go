package boardfamily

import (
	"errors"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SourceQuantity is application-owned. Start/End address exact UTF-8 bytes in
// Request, not a model-authored quote. Fields contain only literal unit
// conversions, not default values or a determination of the quantity's role.
type SourceQuantity struct {
	ID       int                `json:"id"`
	ClauseID int                `json:"clause_id"`
	Start    int                `json:"start"`
	End      int                `json:"end"`
	Text     string             `json:"text"`
	Fields   map[string]float64 `json:"fields"`
}

type ReferencedRequest struct {
	Request    string           `json:"request"`
	Clauses    []RequestClause  `json:"clauses"`
	Quantities []SourceQuantity `json:"quantities"`
}

const sourceQuantityNumber = `[-+]?(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+)(?:[eE][-+]?[0-9]+)?`
const sourceQuantityUnit = `kilohertz|khz|hertz|hz|kiloohms?|kilohms?|kohms?|ohms?|k|milliamperes?|milliamps?|ma|amperes?|amps?|a|millivolts?|mv|volts?|v|(?:degrees\s+)?celsius|degrees\s+c|°\s*c|c|picofarads?|pf|nanofarads?|nf|microfarads?|[uµμ]f|mm`

// Optional units on the lower endpoint allow both 3.2–3.4 V and 3200 mV–3.4 V.
// A mismatched dimension, reversed range, overflow, or unsupported dimension
// is retained as evidence with no configurable Fields, never silently repaired.
var sourceQuantityPattern = regexp.MustCompile(`(?i)(` + sourceQuantityNumber + `)\s*(?:(` + sourceQuantityUnit + `)?\s*(?:to|–|—|-)\s*(` + sourceQuantityNumber + `)\s*)?(` + sourceQuantityUnit + `)\b`)
var sourceBetweenQuantityPattern = regexp.MustCompile(`(?i)\bbetween\s+(` + sourceQuantityNumber + `)\s*(` + sourceQuantityUnit + `)?\s+and\s+(` + sourceQuantityNumber + `)\s*(` + sourceQuantityUnit + `)\b`)

// PrepareReferencedRequest never accepts a caller-supplied source table. Both
// request preparation and decoding regenerate the same table from input bytes.
func PrepareReferencedRequest(prompt string) (ReferencedRequest, error) {
	r := ReferencedRequest{Request: prompt, Quantities: []SourceQuantity{}}
	clauses, err := referencedRequestClauses(prompt)
	if err != nil {
		return r, err
	}
	r.Clauses = clauses
	offset := 0
	for _, c := range clauses {
		// Prefer complete between-ranges to individual endpoint matches. Both
		// patterns expose identical low/unit/high/unit capture positions.
		matches := sourceBetweenQuantityPattern.FindAllStringSubmatchIndex(c.Text, -1)
		for _, match := range sourceQuantityPattern.FindAllStringSubmatchIndex(c.Text, -1) {
			overlaps := false
			for _, existing := range matches {
				if match[0] < existing[1] && existing[0] < match[1] {
					overlaps = true
					break
				}
			}
			if !overlaps {
				matches = append(matches, match)
			}
		}
		sort.Slice(matches, func(i, j int) bool { return matches[i][0] < matches[j][0] })
		for _, match := range matches {
			if match[0] > 0 {
				before, _ := utf8.DecodeLastRuneInString(c.Text[:match[0]])
				if unicode.IsLetter(before) || unicode.IsDigit(before) || before == '_' || before == '.' {
					continue // I2C, part IDs and version strings are not quantities.
				}
			}
			if match[1] < len(c.Text) {
				after, _ := utf8.DecodeRuneInString(c.Text[match[1]:])
				if unicode.IsLetter(after) || unicode.IsDigit(after) || after == '_' {
					continue // Do not truncate a longer identifier or unit name.
				}
			}
			part := func(index int) string {
				if match[2*index] < 0 {
					return ""
				}
				return c.Text[match[2*index]:match[2*index+1]]
			}
			q := SourceQuantity{ID: len(r.Quantities), ClauseID: c.ID,
				Start: offset + match[0], End: offset + match[1], Text: part(0),
				Fields: normalizedQuantityFields(part(1), part(2), part(3), part(4))}
			r.Quantities = append(r.Quantities, q)
			if len(r.Quantities) > 128 {
				return r, errors.New("request exceeds 128 source quantities")
			}
		}
		offset += len(c.Text)
	}
	return r, nil
}

// This candidate needs leading decimals (.1 nF) not to become sentence breaks.
// Keep the old requestClauses implementation unchanged for historical replay.
func referencedRequestClauses(prompt string) ([]RequestClause, error) {
	if strings.TrimSpace(prompt) == "" || len(prompt) > 2000 || !utf8.ValidString(prompt) {
		return nil, errors.New("prompt must contain 1–2000 bytes")
	}
	var clauses []RequestClause
	start := 0
	appendClause := func(end int) {
		text := prompt[start:end]
		if strings.TrimSpace(text) != "" {
			clauses = append(clauses, RequestClause{ID: len(clauses), Text: text})
		} else if len(clauses) != 0 {
			clauses[len(clauses)-1].Text += text
		} else {
			return
		}
		start = end
	}
	for i, char := range prompt {
		if char == '.' && i+1 < len(prompt) && prompt[i+1] >= '0' && prompt[i+1] <= '9' {
			continue
		}
		if strings.ContainsRune(".!?;\n", char) {
			appendClause(i + 1)
		}
	}
	appendClause(len(prompt))
	if len(clauses) > 32 {
		return nil, errors.New("request exceeds 32 clauses")
	}
	return clauses, nil
}

func quantityUnit(unit string) (string, float64) {
	u := strings.ToLower(strings.Join(strings.Fields(unit), ""))
	switch {
	case member(u, "khz", "kilohertz"):
		return "clock", 1000
	case member(u, "hz", "hertz"):
		return "clock", 1
	case member(u, "k", "kohm", "kohms", "kilohm", "kilohms", "kiloohm", "kiloohms"):
		return "resistance", 1000
	case member(u, "ohm", "ohms"):
		return "resistance", 1
	case member(u, "v", "volt", "volts"):
		return "voltage", 1
	case member(u, "mv", "millivolt", "millivolts"):
		return "voltage", .001
	case member(u, "a", "amp", "amps", "ampere", "amperes"):
		return "current", 1000
	case member(u, "ma", "milliamp", "milliamps", "milliampere", "milliamperes"):
		return "current", 1
	case member(u, "c", "°c", "celsius", "degreesc", "degreescelsius"):
		return "temperature", 1
	case member(u, "pf", "picofarad", "picofarads"):
		return "capacitance", 1
	case member(u, "nf", "nanofarad", "nanofarads"):
		return "capacitance", 1000
	case member(u, "uf", "µf", "μf", "microfarad", "microfarads"):
		return "capacitance", 1e6
	default:
		return "", 0 // e.g. mm is evidence, not a configurable dimension.
	}
}

func normalizedQuantityFields(lowText, lowUnit, highText, highUnit string) map[string]float64 {
	fields := map[string]float64{}
	if lowUnit == "" {
		lowUnit = highUnit
	}
	if highText == "" {
		highText = lowText
	}
	lowDimension, lowFactor := quantityUnit(lowUnit)
	highDimension, highFactor := quantityUnit(highUnit)
	low, lowErr := strconv.ParseFloat(lowText, 64)
	high, highErr := strconv.ParseFloat(highText, 64)
	if lowErr != nil || highErr != nil {
		return fields
	}
	if low == 0 && !zeroQuantityMantissa(lowText) || high == 0 && !zeroQuantityMantissa(highText) {
		return fields // Float underflow must not invent a literal zero.
	}
	lowBeforeScale, highBeforeScale := low, high
	low, high = low*lowFactor, high*highFactor
	if low == 0 && lowBeforeScale != 0 || high == 0 && highBeforeScale != 0 {
		return fields
	}
	if lowDimension == "" || lowDimension != highDimension || math.IsInf(low, 0) || math.IsInf(high, 0) || math.IsNaN(low) || math.IsNaN(high) || low > high {
		return fields
	}
	if lowDimension == "voltage" {
		fields["supply_min_v"], fields["supply_max_v"] = low, high
	} else if lowDimension == "temperature" {
		fields["ambient_min_c"], fields["ambient_max_c"] = low, high
	} else if low == high {
		field := map[string]string{"clock": "clock_hz", "resistance": "pullup_ohms", "current": "supply_capacity_ma", "capacitance": "total_bus_capacitance_pf"}[lowDimension]
		fields[field] = low
	}
	return fields
}

func zeroQuantityMantissa(text string) bool {
	mantissa := strings.FieldsFunc(text, func(r rune) bool { return r == 'e' || r == 'E' })[0]
	return strings.Trim(mantissa, "+-0.") == ""
}

// Retain the existing independent, narrow role checks. They are not a general
// semantic classifier; indexed provenance cannot prove how a quantity is used.
func referencedQuantityRoleCompatible(field, source string) bool {
	s := strings.ToLower(source)
	switch field {
	case "clock_hz":
		return !regexp.MustCompile(`\b(samples?|sampling|measurements?|readings?)\b`).MatchString(s) || regexp.MustCompile(`\b(i2c|bus|clock|profile)\b`).MatchString(s)
	case "supply_capacity_ma":
		return !regexp.MustCompile(`\b(gpio|sink|i2c|pull-up|load)\b`).MatchString(s) || regexp.MustCompile(`\b(source|supply|capacity)\b`).MatchString(s)
	default:
		return true
	}
}
