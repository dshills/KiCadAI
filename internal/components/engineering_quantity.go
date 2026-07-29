package components

import (
	"math/big"
	"strings"
	"unicode/utf8"
)

type engineeringQuantity struct {
	dimension string
	value     *big.Rat
}

func parseEngineeringQuantity(value string, fallbackUnit string) (engineeringQuantity, bool) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxEngineeringValueLength {
		return engineeringQuantity{}, false
	}

	numberText, suffix, embeddedPrefix, ok := splitEngineeringNumber(value)
	if !ok {
		return engineeringQuantity{}, false
	}
	number, ok := new(big.Rat).SetString(numberText)
	if !ok {
		return engineeringQuantity{}, false
	}

	dimension, multiplier, ok := parseEngineeringUnit(suffix, fallbackUnit, embeddedPrefix)
	if !ok {
		return engineeringQuantity{}, false
	}
	number.Mul(number, multiplier)
	return engineeringQuantity{dimension: dimension, value: number}, true
}

func splitEngineeringNumber(value string) (number string, suffix string, embeddedPrefix rune, ok bool) {
	runes := []rune(value)
	for i := 1; i < len(runes)-1; i++ {
		if !isEmbeddedEngineeringMarker(runes[i]) ||
			!asciiDigitRune(runes[i-1]) ||
			!asciiDigitRune(runes[i+1]) {
			continue
		}
		rightEnd := i + 1
		for rightEnd < len(runes) && asciiDigitRune(runes[rightEnd]) {
			rightEnd++
		}
		return string(runes[:i]) + "." + string(runes[i+1:rightEnd]),
			strings.TrimSpace(string(runes[rightEnd:])), runes[i], true
	}

	end := scanLeadingFloat(value)
	if end <= 0 {
		return "", "", 0, false
	}
	return value[:end], strings.TrimSpace(value[end:]), 0, true
}

func parseEngineeringUnit(suffix string, fallback string, embeddedPrefix rune) (string, *big.Rat, bool) {
	fallbackDimension, fallbackMultiplier, fallbackOK := parseUnitToken(strings.TrimSpace(fallback), "", true)
	if strings.TrimSpace(fallback) != "" && !fallbackOK {
		return "", nil, false
	}

	if embeddedPrefix != 0 {
		dimension := fallbackDimension
		unitMultiplier := fallbackMultiplier
		if suffix != "" {
			var ok bool
			dimension, unitMultiplier, ok = parseUnitToken(suffix, fallbackDimension, false)
			if !ok {
				return "", nil, false
			}
		}
		if fallbackDimension != "" && dimension != "" && dimension != fallbackDimension {
			return "", nil, false
		}
		return dimension, multiplyRats(prefixMultiplier(embeddedPrefix), unitMultiplier), true
	}

	if suffix == "" {
		if fallbackOK {
			return fallbackDimension, fallbackMultiplier, true
		}
		return "", big.NewRat(1, 1), true
	}

	dimension, multiplier, ok := parseUnitToken(suffix, fallbackDimension, false)
	if !ok {
		return "", nil, false
	}
	if fallbackDimension != "" && dimension != "" && dimension != fallbackDimension {
		return "", nil, false
	}
	return dimension, multiplier, true
}

func parseUnitToken(token string, fallbackDimension string, fallbackMode bool) (string, *big.Rat, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", big.NewRat(1, 1), true
	}

	// A bare SI prefix is meaningful only when the caller supplied a base unit.
	// This check precedes aliases so "m" in "100m" remains milli, while "mm"
	// is an explicit length unit.
	r, size := utf8.DecodeRuneInString(token)
	if size == len(token) && isEngineeringPrefix(r) && fallbackDimension != "" && !fallbackMode {
		return fallbackDimension, prefixMultiplier(r), true
	}

	if dimension, multiplier, ok := canonicalUnitAlias(token); ok {
		return dimension, multiplier, true
	}
	if !isEngineeringPrefix(r) {
		return "", nil, false
	}
	rest := strings.TrimSpace(token[size:])
	if rest == "" {
		return "", prefixMultiplier(r), true
	}
	dimension, unitMultiplier, ok := canonicalUnitAlias(rest)
	if !ok {
		return "", nil, false
	}
	return dimension, multiplyRats(prefixMultiplier(r), unitMultiplier), true
}

func canonicalUnitAlias(unit string) (string, *big.Rat, bool) {
	trimmed := strings.TrimSpace(unit)
	lower := strings.ToLower(trimmed)
	one := big.NewRat(1, 1)
	switch {
	case trimmed == "A" || lower == "amp" || lower == "amps" || lower == "ampere" || lower == "amperes":
		return "current", one, true
	case trimmed == "F" || lower == "farad" || lower == "farads":
		return "capacitance", one, true
	case trimmed == "H" || lower == "henry" || lower == "henries":
		return "inductance", one, true
	case trimmed == "Hz" || lower == "hz":
		return "frequency", one, true
	case trimmed == "V" || lower == "volt" || lower == "volts":
		return "voltage", one, true
	case trimmed == "W" || lower == "watt" || lower == "watts":
		return "power", one, true
	case trimmed == "Ohm" || lower == "ohm" || lower == "ohms" || lower == "o" || lower == "r" || trimmed == "Ω" || trimmed == "ω":
		return "resistance", one, true
	case lower == "s" || lower == "second" || lower == "seconds":
		return "time", one, true
	case trimmed == "C" || lower == "celsius":
		return "temperature", one, true
	case lower == "%":
		return "percent", one, true
	case lower == "1" || lower == "ratio":
		return "ratio", one, true
	case lower == "pins" || lower == "count" || lower == "class":
		return lower, one, true
	case lower == "mm":
		return "length", big.NewRat(1, 1000), true
	case lower == "a_rms":
		return "current_rms", one, true
	case lower == "v_pp":
		return "voltage_peak_to_peak", one, true
	case lower == "a*ohm":
		return "voltage", one, true
	case lower == "a2s":
		return "current_squared_time", one, true
	case lower == "v/v":
		return "voltage_gain", one, true
	case lower == "v/s":
		return "slew_rate", one, true
	case lower == "v/sqrt(hz)":
		return "voltage_noise_density", one, true
	case lower == "sps":
		return "samples_per_second", one, true
	case lower == "ppm":
		return "parts_per_million", one, true
	case lower == "ppm/c":
		return "temperature_coefficient", one, true
	default:
		return "", nil, false
	}
}

func prefixMultiplier(prefix rune) *big.Rat {
	switch prefix {
	case 'f':
		return big.NewRat(1, 1_000_000_000_000_000)
	case 'p':
		return big.NewRat(1, 1_000_000_000_000)
	case 'n':
		return big.NewRat(1, 1_000_000_000)
	case 'u', 'µ', 'μ':
		return big.NewRat(1, 1_000_000)
	case 'm':
		return big.NewRat(1, 1000)
	case 'k', 'K':
		return big.NewRat(1000, 1)
	case 'M':
		return big.NewRat(1_000_000, 1)
	case 'G':
		return big.NewRat(1_000_000_000, 1)
	case 'T':
		return big.NewRat(1_000_000_000_000, 1)
	case 'P':
		return big.NewRat(1_000_000_000_000_000, 1)
	default:
		return big.NewRat(1, 1)
	}
}

func multiplyRats(left *big.Rat, right *big.Rat) *big.Rat {
	return new(big.Rat).Mul(left, right)
}

func quantitiesEqual(left engineeringQuantity, right engineeringQuantity) bool {
	return left.dimension == right.dimension && left.value.Cmp(right.value) == 0
}
