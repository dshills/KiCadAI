package architecturesearch

import (
	"math"
	"slices"

	"kicadai/internal/reports"
)

var preferredMantissas = map[PreferredSeries][]float64{
	SeriesE6:  {1.0, 1.5, 2.2, 3.3, 4.7, 6.8},
	SeriesE12: {1.0, 1.2, 1.5, 1.8, 2.2, 2.7, 3.3, 3.9, 4.7, 5.6, 6.8, 8.2},
	SeriesE24: {1.0, 1.1, 1.2, 1.3, 1.5, 1.6, 1.8, 2.0, 2.2, 2.4, 2.7, 3.0, 3.3, 3.6, 3.9, 4.3, 4.7, 5.1, 5.6, 6.2, 6.8, 7.5, 8.2, 9.1},
	SeriesE48: {1.00, 1.05, 1.10, 1.15, 1.21, 1.27, 1.33, 1.40, 1.47, 1.54, 1.62, 1.69, 1.78, 1.87, 1.96, 2.05, 2.15, 2.26, 2.37, 2.49, 2.61, 2.74, 2.87, 3.01, 3.16, 3.32, 3.48, 3.65, 3.83, 4.02, 4.22, 4.42, 4.64, 4.87, 5.11, 5.36, 5.62, 5.90, 6.19, 6.49, 6.81, 7.15, 7.50, 7.87, 8.25, 8.66, 9.09, 9.53},
	SeriesE96: {1.00, 1.02, 1.05, 1.07, 1.10, 1.13, 1.15, 1.18, 1.21, 1.24, 1.27, 1.30, 1.33, 1.37, 1.40, 1.43, 1.47, 1.50, 1.54, 1.58, 1.62, 1.65, 1.69, 1.74, 1.78, 1.82, 1.87, 1.91, 1.96, 2.00, 2.05, 2.10, 2.15, 2.21, 2.26, 2.32, 2.37, 2.43, 2.49, 2.55, 2.61, 2.67, 2.74, 2.80, 2.87, 2.94, 3.01, 3.09, 3.16, 3.24, 3.32, 3.40, 3.48, 3.57, 3.65, 3.74, 3.83, 3.92, 4.02, 4.12, 4.22, 4.32, 4.42, 4.53, 4.64, 4.75, 4.87, 4.99, 5.11, 5.23, 5.36, 5.49, 5.62, 5.76, 5.90, 6.04, 6.19, 6.34, 6.49, 6.65, 6.81, 6.98, 7.15, 7.32, 7.50, 7.68, 7.87, 8.06, 8.25, 8.45, 8.66, 8.87, 9.09, 9.31, 9.53, 9.76},
}

func PreferredValueCandidates(ideal float64, series PreferredSeries, minimum, maximum float64, limit int) ([]float64, []reports.Issue) {
	mantissas, ok := preferredSeriesMantissas(series)
	if !ok || !finitePositive(ideal) || !finitePositive(minimum) || !finitePositive(maximum) || minimum > maximum || limit <= 0 || limit > DefaultMaxValueCandidates {
		return nil, calculationIssue(CodeValueInputInvalid, "preferred_values", "invalid preferred-value series, range, ideal, or candidate limit")
	}
	minimumDecade := int(math.Floor(math.Log10(minimum))) - 1
	maximumDecade := int(math.Ceil(math.Log10(maximum))) + 1
	if minimumDecade < -15 {
		minimumDecade = -15
	}
	if maximumDecade > 15 {
		maximumDecade = 15
	}
	seen := map[float64]bool{}
	var candidates []float64
	for decade := minimumDecade; decade <= maximumDecade; decade++ {
		scale := math.Pow10(decade)
		for _, mantissa := range mantissas {
			value := quantize(mantissa * scale)
			if value < minimum || value > maximum || seen[value] {
				continue
			}
			seen[value] = true
			candidates = append(candidates, value)
		}
	}
	slices.SortStableFunc(candidates, func(left, right float64) int {
		leftError := math.Abs(left-ideal) / ideal
		rightError := math.Abs(right-ideal) / ideal
		if leftError < rightError {
			return -1
		}
		if leftError > rightError {
			return 1
		}
		if left < right {
			return -1
		}
		if left > right {
			return 1
		}
		return 0
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	if len(candidates) == 0 {
		return nil, calculationIssue(CodeValueUnsolved, "preferred_values", "no preferred value falls within the permitted range")
	}
	return candidates, nil
}

func preferredSeriesMantissas(series PreferredSeries) ([]float64, bool) {
	if series != SeriesE192 {
		mantissas, ok := preferredMantissas[series]
		return mantissas, ok
	}
	// IEC 60063 E192 values are the 192 logarithmically even values in a
	// decade, rounded to three significant digits. Generate them from the
	// series definition so the preferred-value table remains auditable and
	// cannot acquire hand-transcription gaps.
	mantissas := make([]float64, 0, 192)
	for index := 0; index < 192; index++ {
		value := math.Pow(10, float64(index)/192)
		mantissas = append(mantissas, math.Round(value*100)/100)
	}
	return mantissas, true
}

func quantize(value float64) float64 {
	if value == 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return value
	}
	exponent := math.Floor(math.Log10(math.Abs(value)))
	scale := math.Pow10(int(12 - exponent))
	if math.IsInf(scale, 0) || scale == 0 {
		return value
	}
	return math.Round(value*scale) / scale
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func finiteNumbers(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}
