package boardfamily

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

func member(value string, values ...string) bool {
	for _, v := range values {
		if value == v {
			return true
		}
	}
	return false
}

// Keep the literal identity rule shared by admission and the request schema.
// A mention permits extraction; it does not prove requirement state or scope.
func sensorIdentityGrounded(value, source string) bool {
	return member(value, "BMP280", "SHT31") && regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(value)+`\b`).MatchString(source)
}

func validateFact(f RequirementFact, raw []byte, source string) error {
	fields := []string{"kind", "value", "state", "quote"}
	switch f.Kind {
	case "none":
		fields = []string{"kind"}
	case "number":
		fields = []string{"kind", "value", "number", "quote"}
	case "unclear", "other":
		fields = []string{"kind", "quote", "detail"}
	case "sensor", "measurement", "profile", "feature":
		if !member(f.State, "required", "not_required", "forbidden", "uncertain") {
			return errors.New("invalid requirement state")
		}
	default:
		return errors.New("unknown fact kind")
	}
	if !exactFields(raw, fields...) {
		return errors.New("invalid fields for fact kind")
	}
	if f.Kind == "none" {
		return nil
	}
	if strings.TrimSpace(f.Quote) == "" || !strings.Contains(source, f.Quote) {
		return errors.New("fact quote is missing or not in its source clause")
	}
	switch f.Kind {
	case "sensor":
		if !sensorIdentityGrounded(f.Value, f.Quote) {
			return errors.New("sensor identity is not grounded in the quoted request")
		}
	case "measurement":
		if !member(f.Value, "pressure", "temperature", "humidity") {
			return errors.New("unknown measurement")
		}
		anchors := map[string]string{"pressure": `(?i)\b(?:pressures?|barometric|barometer)\b`, "temperature": `(?i)\b(?:temperatures?|thermometer|thermal)\b`, "humidity": `(?i)\b(?:humidity|humid)\b`}
		if !regexp.MustCompile(anchors[f.Value]).MatchString(f.Quote) {
			return errors.New("measurement identity is not grounded in the quoted request")
		}
	case "profile":
		if !member(f.Value, "standard", "fast", "low_current") {
			return errors.New("unknown profile")
		}
		anchor := regexp.QuoteMeta(f.Value)
		if f.Value == "low_current" {
			anchor = `low[_ -]current`
		}
		if !regexp.MustCompile(`(?i)\b` + anchor + `\b`).MatchString(f.Quote) {
			return errors.New("profile identity is not grounded in the quoted request")
		}
	case "feature":
		if _, ok := intentFeatureReasons[f.Value]; !ok {
			return errors.New("unknown feature")
		}
	case "number":
		if !member(f.Value, intentNumericFields...) || f.Number == nil || math.IsNaN(*f.Number) || math.IsInf(*f.Number, 0) {
			return errors.New("invalid numeric requirement")
		}
		if !numberGrounded(f.Value, *f.Number, f.Quote) {
			return errors.New("numeric value or unit is not grounded in its quoted request")
		}
	case "unclear", "other":
		if strings.TrimSpace(f.Detail) == "" {
			return errors.New("requirement lacks a specific question or explanation")
		}
	}
	return nil
}

const quotedNumber = `([-+]?(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+))`

// Check literal magnitude and units, not unrestricted English semantics. The
// model must identify the quantity's role; independent role checks below reject
// known measurement-rate / bus-clock and load-current / source-capacity mixups.
func numberGrounded(field string, value float64, quote string) bool {
	s := strings.ToLower(strings.NewReplacer("–", " to ", "—", " to ", "°", "").Replace(quote))
	s = regexp.MustCompile(`([0-9])-([0-9])`).ReplaceAllString(s, "$1 to $2")
	unit := ""
	switch field {
	case "clock_hz":
		unit = `(khz|kilohertz|hz|hertz)`
		if regexp.MustCompile(`\b(samples?|sampling|measurements?|readings?)\b`).MatchString(s) && !regexp.MustCompile(`\b(?:i2c|bus|clock|profile)\b`).MatchString(s) {
			return false
		}
	case "pullup_ohms":
		unit = `(kohms?|kilohms?|kiloohms?|ohms?|k)`
	case "supply_min_v", "supply_max_v":
		unit = `(v|volts?)`
	case "supply_capacity_ma":
		unit = `(ma|milliamps?|milliamperes?|a|amps?|amperes?)`
		if regexp.MustCompile(`\b(?:gpio|sink|i2c|pull-up|load)\b`).MatchString(s) && !regexp.MustCompile(`\b(?:source|supply|capacity)\b`).MatchString(s) {
			return false
		}
	case "ambient_min_c", "ambient_max_c":
		unit = `(?:degrees\s+)?(c|celsius)`
	case "total_bus_capacitance_pf":
		unit = `(pf|picofarads?)`
	default:
		return false
	}
	closeValue := func(n float64) bool { return math.Abs(n-value) <= 1e-9*math.Max(1, math.Abs(value)) }
	if strings.HasSuffix(field, "_min_v") || strings.HasSuffix(field, "_max_v") || strings.HasSuffix(field, "_min_c") || strings.HasSuffix(field, "_max_c") {
		// Range endpoints are positional. Merely finding 3.2 somewhere in a
		// 3.2–3.4 V quote cannot justify silently changing the upper bound to 3.2.
		rangePattern := regexp.MustCompile(quotedNumber + `\s*(?:[a-z]+\s*)?to\s*` + quotedNumber + `\s*` + unit + `\b`)
		if matches := rangePattern.FindAllStringSubmatch(s, -1); len(matches) != 0 {
			for _, m := range matches {
				index := 1
				if strings.Contains(field, "_max_") {
					index = 2
				}
				n, err := strconv.ParseFloat(m[index], 64)
				if err == nil && closeValue(n) {
					return true
				}
			}
			return false
		}
	}
	for _, m := range regexp.MustCompile(quotedNumber+`\s*`+unit+`\b`).FindAllStringSubmatch(s, -1) {
		n, err := strconv.ParseFloat(m[1], 64)
		factor := 1.0
		if field == "clock_hz" && strings.HasPrefix(m[2], "k") || field == "pullup_ohms" && strings.HasPrefix(m[2], "k") || field == "supply_capacity_ma" && strings.HasPrefix(m[2], "a") {
			factor = 1000
		}
		if err == nil && closeValue(n*factor) {
			return true
		}
	}
	return false
}

type intentRequirements struct {
	families, profiles, forbiddenFamilies, forbiddenProfiles map[string]bool
	forbiddenMeasurements                                    map[string]bool
	numbers                                                  map[string]float64
	conflicts, questions                                     []string
}

func (r *intentRequirements) add(f RequirementFact) {
	if f.Kind == "none" || f.State == "not_required" {
		return
	}
	if f.Kind == "unclear" {
		r.questions = append(r.questions, f.Detail)
		return
	}
	if f.Kind == "other" {
		r.conflicts = append(r.conflicts, "No reviewed family implements this additional requirement: "+f.Detail)
		return
	}
	if f.State == "uncertain" {
		question := "Please resolve the requested " + f.Kind + " (" + f.Value + ")."
		if f.Kind == "sensor" || f.Kind == "measurement" {
			question = "Which measurement do you need: pressure with BMP280, or temperature/humidity with SHT31?"
		}
		if f.Kind == "profile" {
			question = "Which I2C profile do you require: standard, fast, or BMP280-only low_current?"
		}
		r.questions = append(r.questions, question)
		return
	}
	if f.Kind == "feature" {
		if f.State == "required" {
			r.conflicts = append(r.conflicts, intentFeatureReasons[f.Value])
		}
		return
	}
	if f.Kind == "number" {
		if prior, ok := r.numbers[f.Value]; ok && prior != *f.Number {
			r.conflicts = append(r.conflicts, "Conflicting explicit values for "+f.Value+"; no requirement was discarded.")
		}
		r.numbers[f.Value] = *f.Number
		return
	}
	if f.Kind == "profile" {
		if f.State == "forbidden" {
			r.forbiddenProfiles[f.Value] = true
		} else {
			r.profiles[f.Value] = true
		}
		return
	}
	family := FamilySHT31
	if f.Value == "BMP280" || f.Value == "pressure" {
		family = Family
	}
	if f.State == "forbidden" {
		if f.Kind == "sensor" {
			r.forbiddenFamilies[family] = true
		} else {
			r.forbiddenMeasurements[f.Value] = true
		}
	} else {
		r.families[family] = true
	}
}

func admitIntent(prompt string, intent RequirementIntent, clauses []RequestClause) Decision {
	r := intentRequirements{families: map[string]bool{}, profiles: map[string]bool{}, forbiddenFamilies: map[string]bool{}, forbiddenProfiles: map[string]bool{}, forbiddenMeasurements: map[string]bool{}, numbers: map[string]float64{}}
	// Process in application clause order for deterministic explanations even if
	// the provider reordered its ID-addressed response objects.
	byID := make(map[int]IntentClause, len(intent.Clauses))
	for _, c := range intent.Clauses {
		byID[c.ID] = c
	}
	for _, c := range clauses {
		for _, f := range byID[c.ID].Facts {
			r.add(f)
		}
		if missing := uncoveredQuantities(c.Text, byID[c.ID].Facts); len(missing) > 0 {
			r.questions = append(r.questions, "Please confirm the role and requirement for "+strings.Join(missing, ", ")+"; those explicit quantities were not retained by the extraction.")
		}
	}
	if reason := fixedGeometryConflict(prompt); reason != "" {
		r.conflicts = append(r.conflicts, reason)
	}
	for _, c := range clauses {
		r.conflicts = append(r.conflicts, directSourceConflicts(c.Text)...)
	}
	local := assessRequirements(prompt)
	if len(scanRequestConstraints(prompt).unknown) == 0 && local.Disposition == "unsupported" {
		r.conflicts = append(r.conflicts, local.Message)
	}
	if len(r.families) > 1 {
		r.conflicts = append(r.conflicts, "Neither family combines pressure with humidity/ambient-temperature sensing; no single-sensor substitution is authorized.")
	}
	if len(r.profiles) > 1 {
		r.conflicts = append(r.conflicts, "Two required I2C profiles cannot be implemented simultaneously by one fixed configuration.")
	}
	if len(r.conflicts) > 0 {
		return localDecision(prompt, "unsupported", strings.Join(uniqueStrings(r.conflicts), " "), nil)
	}
	if len(r.questions) > 0 {
		return localDecision(prompt, "clarify", strings.Join(uniqueStrings(r.questions), " "), nil)
	}
	if len(r.families) != 1 {
		return localDecision(prompt, "clarify", "Which measurement do you need: pressure with BMP280, or temperature/humidity with SHT31?", nil)
	}
	var family string
	for family = range r.families {
	}
	if r.forbiddenFamilies[family] || family == Family && r.forbiddenMeasurements["pressure"] || family == FamilySHT31 && (r.forbiddenMeasurements["temperature"] || r.forbiddenMeasurements["humidity"]) {
		return localDecision(prompt, "unsupported", "The selected family conflicts with an explicitly forbidden sensor or measurement.", nil)
	}
	var candidates []Profile
	for _, p := range ProfilesFor(family) {
		if r.forbiddenProfiles[p.ID] || len(r.profiles) != 0 && !r.profiles[p.ID] {
			continue
		}
		if v, ok := r.numbers["clock_hz"]; ok && v != float64(p.ClockHz) {
			continue
		}
		if v, ok := r.numbers["pullup_ohms"]; ok && v != p.ResistanceOhms {
			continue
		}
		candidates = append(candidates, p)
	}
	if len(candidates) == 0 {
		return localDecision(prompt, "unsupported", "The requested/forbidden profiles, I2C clock and pull-ups do not match a reviewed profile for "+family+". No alternative was substituted.", nil)
	}
	p := candidates[0]
	if p.ID == "low_current" && !explicitLowCurrentScope(prompt) {
		return localDecision(prompt, "clarify", "Do you explicitly want the BMP280 pull-up-only low_current profile? It does not reduce whole-board power or provide battery operation.", nil)
	}
	for _, pair := range [][2]string{{"supply_min_v", "supply_max_v"}, {"ambient_min_c", "ambient_max_c"}} {
		_, low := r.numbers[pair[0]]
		_, high := r.numbers[pair[1]]
		if low != high {
			return localDecision(prompt, "clarify", "Please confirm both "+pair[0]+" and "+pair[1]+"; the extraction retained only one explicit operating bound.", nil)
		}
	}
	c := Config{"1", family, p.ID, 3.2, 3.4, 1000, 10, 35, p.MaxBusPF}
	for key, target := range map[string]*float64{"supply_min_v": &c.SupplyMinV, "supply_max_v": &c.SupplyMaxV, "supply_capacity_ma": &c.SupplyCapacityMA, "ambient_min_c": &c.AmbientMinC, "ambient_max_c": &c.AmbientMaxC, "total_bus_capacitance_pf": &c.TotalBusCapacitancePF} {
		if v, ok := r.numbers[key]; ok {
			*target = v
		}
	}
	if _, err := Check(c); err != nil {
		return localDecision(prompt, "unsupported", err.Error()+"; no stated bound was weakened.", nil)
	}
	// When the original request is fully understood by the earlier deterministic
	// grammar, use its independent numeric/profile proof as an additional check.
	// Unknown words do NOT veto the semantic extraction in this successor path.
	if local.Disposition == "supported" && *local.Configuration != c {
		return localDecision(prompt, "clarify", "The extracted configuration conflicts with the independently recognized request. Please confirm the requested profile and operating bounds.", nil)
	}
	return localDecision(prompt, "supported", fmt.Sprintf("Selected %s/%s from the source-bound requirement extraction. Operating limits are declared bounds, not measurements. The fixed catalog conditions and separate firmware/assembly/bench obligations still apply.", family, p.ID), &c)
}

var directHeaterOperation = regexp.MustCompile(`(?i)\b(?:run|enable|activate|energize|turn on|switch on|use)\s+(?:the\s+|its\s+)?heater\b`)
var directFiveVolt = regexp.MustCompile(`(?i)\b(?:power(?:ed)?|suppl(?:y|ied)|input)\b[^.;!?]{0,60}\b5\s*(?:v|volts?)\b[^.;!?]{0,20}\busb\b`)

var explicitQuantity = regexp.MustCompile(`(?i)` + quotedNumber + `\s*(?:khz|kilohertz|hz|hertz|kohms?|kilohms?|kiloohms?|ohms?|k|v|volts?|ma|milliamps?|milliamperes?|a|amps?|amperes?|°\s*c|degrees\s+c|celsius|c|pf|picofarads?|nf|uf|mm)\b`)

// A recognized numeric literal cannot disappear just because the rest of the
// sentence uses ordinary phrasing. Quotes bind its interpretation to a fact;
// this is an omission check, not a proof of that fact's English semantics.
func uncoveredQuantities(source string, facts []RequirementFact) []string {
	var missing []string
	for _, match := range explicitQuantity.FindAllStringIndex(source, -1) {
		if match[0] > 0 {
			previous := source[match[0]-1]
			if previous >= 'a' && previous <= 'z' || previous >= 'A' && previous <= 'Z' || previous >= '0' && previous <= '9' || previous == '_' {
				continue // An identifier such as I2C is not a temperature value.
			}
		}
		covered := false
		for _, f := range facts {
			if !member(f.Kind, "number", "feature", "other", "unclear") || f.Quote == "" {
				continue
			}
			for start := 0; start < len(source); {
				at := strings.Index(source[start:], f.Quote)
				if at < 0 {
					break
				}
				at += start
				if at <= match[0] && at+len(f.Quote) >= match[1] {
					covered = true
					break
				}
				start = at + 1
			}
			if covered {
				break
			}
		}
		if !covered {
			missing = append(missing, source[match[0]:match[1]])
		}
	}
	return uniqueStrings(missing)
}

// These are independent checks for demonstrated affirmative contradictions,
// not a general English classifier. Unknown language is handled by typed facts,
// not by an allowlist. Negation and condition interpretation still need evals.
func directSourceConflicts(source string) []string {
	var result []string
	for _, match := range directHeaterOperation.FindAllStringIndex(source, -1) {
		prefix := strings.ToLower(strings.TrimSpace(source[:match[0]]))
		negated := false
		for _, end := range []string{"do not", "don't", "never", "not to", "don't want to", "do not want to", "no need to", "not required to"} {
			if strings.HasSuffix(prefix, end) {
				negated = true
			}
		}
		if !negated {
			result = append(result, intentFeatureReasons["heater_operation"])
		}
	}
	if directFiveVolt.MatchString(source) {
		lower := strings.ToLower(source)
		if !strings.Contains(lower, "not") && !strings.Contains(lower, "no ") && !strings.Contains(lower, "without") {
			result = append(result, intentFeatureReasons["usb"])
		}
	}
	return result
}
