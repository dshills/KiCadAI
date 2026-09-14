package boardfamily

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// AdmissionVersion identifies a local policy, not a new provider evaluation.
const AdmissionVersion = "bounded-requirements-01"

// This is intentionally a bounded request grammar, NOT a natural-language
// classifier. Every token must be consumed by a reviewed rule. Unknown words,
// negation, units, temporal qualifications and instructions fail closed. The
// provider cannot expand the grammar or waive any of its constraints.
type requirements struct {
	families, profiles map[string]bool
	values             map[string]float64
	conflicts, unknown []string
	choice, lowPower   bool
}

type requirementRule struct {
	pattern *regexp.Regexp
	apply   func(*requirements, []string)
}

func rule(pattern string, apply func(*requirements, []string)) requirementRule {
	return requirementRule{regexp.MustCompile(`^(?:` + pattern + `)(?:$|[\s,;.!?])`), apply}
}

func familyRequirement(family string) func(*requirements, []string) {
	return func(r *requirements, _ []string) { r.families[family] = true }
}

func profileRequirement(profile string) func(*requirements, []string) {
	return func(r *requirements, _ []string) { r.profiles[profile] = true }
}

func unsupportedRequirement(reason string) func(*requirements, []string) {
	return func(r *requirements, _ []string) { r.conflicts = append(r.conflicts, reason) }
}

func (r *requirements) number(key string, value float64) {
	if previous, ok := r.values[key]; ok && previous != value {
		r.conflicts = append(r.conflicts, "Conflicting explicit values for "+key+"; no value was discarded.")
	}
	r.values[key] = value
}

const decimal = `(?:exactly )?([0-9]+(?:\.[0-9]+)?)`

func numericRequirement(key string) func(*requirements, []string) {
	return func(r *requirements, m []string) {
		value, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			r.conflicts = append(r.conflicts, "Unrepresentable numeric requirement.")
			return
		}
		r.number(key, value)
	}
}

func rangeRequirement(minKey, maxKey string) func(*requirements, []string) {
	return func(r *requirements, m []string) {
		minValue, e1 := strconv.ParseFloat(m[1], 64)
		maxValue, e2 := strconv.ParseFloat(m[2], 64)
		if e1 != nil || e2 != nil {
			r.conflicts = append(r.conflicts, "Unrepresentable numeric requirement.")
			return
		}
		r.number(minKey, minValue)
		r.number(maxKey, maxValue)
	}
}

// Exclusions are complete grammatical phrases and run BEFORE positive rules.
// We never delete a standalone "not" or apply its scope to a whole sentence.
// In particular "heater off at startup, but ..." does not authorize later heat.
const excludedCapability = `(?:humidity(?: sensing| measurement)?|pressure(?: sensing| measurement)?|wireless(?: telemetry)?|radios?|wi-fi|bluetooth|usb(?: power)?|(?:an? )?(?:ambient )?accuracy guarantee|(?:reduced |reducing )?whole-board power|battery (?:operation|power)|running on a battery)`
const exclusionList = excludedCapability + `(?: (?:and|or) ` + excludedCapability + `)*`

var requirementRules = []requirementRule{
	rule(`(?:i (?:do not (?:need|require|mean)|am not asking for)|no|without) `+exclusionList, nil),
	rule(exclusionList+` (?:is|are) not (?:needed|required)`, nil),
	rule(`(?:keep |leave )?(?:the |its )?heater (?:off|disabled)(?: at all times)?`, nil),
	rule(`(?:disable|never (?:enable|activate)|do not (?:enable|activate|run)) (?:the |its )?heater`, nil),
	rule(`(?:keep |leave )?(?:the )?(?:radios?|wireless) (?:off|disabled)`, nil),
	rule(`(?:disable|do not (?:use|enable)) (?:the )?(?:radios?|wireless|wi-fi|bluetooth)`, nil),
	rule(`(?:run|enable|activate|energize|turn on|switch on) (?:the |its )?heater|(?:the |its )?heater (?:on|enabled|active|continuously)`, unsupportedRequirement("The SHT31 heater must remain off; requested heater operation is outside the reviewed family.")),
	rule(`wireless(?: telemetry)?|wi-fi|bluetooth|radio operation`, unsupportedRequirement("Wireless operation is unsupported; radios must remain disabled.")),
	rule(`usb(?: power)?|rs-232`, unsupportedRequirement("USB and direct USB power are unsupported; the board requires external regulated 3.2–3.4 V and has only 3.3 V UART.")),
	rule(`(?:guarantee|guaranteed|certify|certified|precision|accuracy)`, unsupportedRequirement("Assembled-board accuracy, measured performance and certification are not guaranteed by software qualification.")),
	rule(`battery (?:operation|power|life)|regulator|relays?|motors?|protection circuitry|deliver firmware`, unsupportedRequirement("Battery operation, extra circuitry and delivered firmware are outside the fixed reference families.")),
	rule(`(?:reduce|reducing) only (?:the )?i2c pull-up current|(?:reduce|reducing) (?:the )?i2c pull-up current`, profileRequirement("low_current")),
	rule(`low-power|low power|save energy|whole-board power|battery life`, func(r *requirements, _ []string) { r.lowPower = true }),
	rule(`(?:the )?(?:(?:normal|default|supported|standard|electrical|power|supply|ambient|and) )+(?:limits|envelope)`, nil),
	rule(`do not change the profile, resistors or capacitance requirement|keep all these requirements`, nil),
	rule(`either|or|not decided|not chosen`, func(r *requirements, _ []string) { r.choice = true }),
	rule(`esp32_bmp280_v1|bmp280|pressure(?:-monitoring)?(?: sensing| measurement)?|barometric`, familyRequirement(Family)),
	rule(`esp32_sht31_v1|sht31(?:-dis-b2\.5ks)?|temperature/humidity|temperature|humidity`, familyRequirement(FamilySHT31)),
	rule(`low_current|low-current (?:bus )?profile`, profileRequirement("low_current")),
	rule(`fast|faster-bus`, profileRequirement("fast")),
	rule(`standard`, profileRequirement("standard")),
	// Clock and source-current units must carry their intended role. A sensor
	// sampling-rate requirement must not be reinterpreted as an I2C clock, nor
	// an I2C sink-current requirement as available source capacity.
	rule(`(?:profile|pull-ups?) at `+decimal+`\s*khz`, numericRequirement("clock_khz")),
	rule(decimal+`\s*khz (?:i2c )?(?:bus|clock)`, numericRequirement("clock_khz")),
	rule(decimal+`\s*(?:k|kohms?|kilohms?)`, numericRequirement("pullup_kohm")),
	rule(decimal+`\s*pf`, numericRequirement("total_bus_capacitance_pf")),
	rule(decimal+`\s*(?:-|to)\s*`+decimal+`\s*v`, rangeRequirement("supply_min_v", "supply_max_v")),
	rule(decimal+`\s*v`, func(r *requirements, m []string) {
		rangeRequirement("supply_min_v", "supply_max_v")(r, []string{m[0], m[1], m[1]})
	}),
	rule(decimal+`\s*ma source capacity`, numericRequirement("supply_capacity_ma")),
	rule(`source capacity (?:of )?`+decimal+`\s*ma`, numericRequirement("supply_capacity_ma")),
	rule(decimal+`\s*(?:-|to)\s*`+decimal+`\s*(?:°c|c|degrees c)`, rangeRequirement("ambient_min_c", "ambient_max_c")),
	rule(`120(?:\.0+)?\s*(?:x|by|×)\s*80(?:\.0+)?\s*(?:mm|millimeters?|millimetres?)|two(?: copper)? layers|2-layer|two-layer|1\.6\s*mm fr4`, nil),
	rule(`(?:validate|check) crc|(?:the )?heater must remain off|radios must be disabled`, nil),
	// Only syntactic glue with no polarity, quantifier, condition or authority.
	// Other adjectives/verbs are deliberately NOT treated as harmless filler.
	rule(`create|make|generate|build|use|using|keep|declare|pick|select|with|and|at|a|an|the|its|i need|for|of`, nil),
	rule(`wired|esp32|shared|fixed|board|controller|sensor|monitoring|indoor|project|node|bus|i2c|profile's|profile|variant|pull-ups|total|capacitance|loading|limit|external regulated|ambient`, nil),
}

func assessRequirements(prompt string) Decision {
	r := requirements{families: map[string]bool{}, profiles: map[string]bool{}, values: map[string]float64{}}
	s := strings.ToLower(strings.NewReplacer("–", "-", "—", "-", "‑", "-", "’", "'").Replace(prompt))
	s = strings.Join(strings.Fields(s), " ")
	for s != "" {
		s = strings.TrimLeft(s, " ,;.!?\t\r\n")
		if s == "" {
			break
		}
		matched := false
		for _, rule := range requirementRules {
			if m := rule.pattern.FindStringSubmatch(s); m != nil {
				if rule.apply != nil {
					rule.apply(&r, m)
				}
				s = s[len(m[0]):]
				matched = true
				break
			}
		}
		if !matched {
			// Continue only to collect additional conflicts. Even a single
			// unrecognized token permanently prevents supported admission.
			n := strings.IndexAny(s, " ,;.!?\t\r\n")
			if n < 0 {
				n = len(s)
			}
			r.unknown = append(r.unknown, s[:n])
			s = s[n:]
		}
	}
	if reason := fixedGeometryConflict(prompt); reason != "" {
		r.conflicts = append(r.conflicts, reason)
	}
	if len(r.families) > 1 && !r.choice {
		r.conflicts = append(r.conflicts, "Neither family combines pressure with humidity/ambient-temperature sensing; no single-sensor substitution is authorized.")
	}
	if len(r.conflicts) > 0 {
		return localDecision(prompt, "unsupported", strings.Join(uniqueStrings(r.conflicts), " "), nil)
	}
	if len(r.families) != 1 {
		return localDecision(prompt, "clarify", "Which measurement is required: pressure with BMP280, or temperature/humidity with SHT31? Neither family integrates both.", nil)
	}
	var family string
	for family = range r.families {
	}
	profiles := ProfilesFor(family)
	for key, field := range map[string]func(Profile) float64{"clock_khz": func(p Profile) float64 { return float64(p.ClockHz) / 1000 }, "pullup_kohm": func(p Profile) float64 { return p.ResistanceOhms / 1000 }} {
		if value, ok := r.values[key]; ok {
			filtered := profiles[:0]
			for _, p := range profiles {
				if field(p) == value {
					filtered = append(filtered, p)
				}
			}
			profiles = filtered
		}
	}
	if len(r.profiles) > 1 {
		return localDecision(prompt, "clarify", "Choose exactly one I2C profile: standard or fast (or BMP280-only low_current). Conflicting or alternative profiles cannot be selected silently.", nil)
	}
	for requested := range r.profiles {
		filtered := profiles[:0]
		for _, p := range profiles {
			if p.ID == requested {
				filtered = append(filtered, p)
			}
		}
		profiles = filtered
	}
	if len(profiles) == 0 {
		return localDecision(prompt, "unsupported", "The requested I2C profile, clock and pull-up values do not match a supported profile for "+family+". No alternative was selected.", nil)
	}
	if r.lowPower {
		return localDecision(prompt, "clarify", "Do you mean reducing only I2C pull-up current, or whole-board power/battery use? Only the BMP280 pull-up-only low_current option is supported.", nil)
	}
	if len(r.unknown) > 0 || r.choice {
		return localDecision(prompt, "clarify", fmt.Sprintf("Please resolve or restate these unrecognized request terms using the reviewed family/profile and operating limits: %q. No requirement was assumed supported. Use --list-families to inspect the fixed conditions.", strings.Join(uniqueStrings(r.unknown), " ")), nil)
	}
	// Standard is a default only after family selection. Numerical constraints
	// can narrow the profile, but never silently select low_current.
	p := profiles[0]
	if p.ID == "low_current" && !explicitLowCurrentScope(prompt) {
		return localDecision(prompt, "clarify", "Please explicitly request the BMP280 low_current pull-up-only profile; it does not reduce whole-board power.", nil)
	}
	c := Config{"1", family, p.ID, 3.2, 3.4, 1000, 10, 35, p.MaxBusPF}
	for key, dest := range map[string]*float64{"supply_min_v": &c.SupplyMinV, "supply_max_v": &c.SupplyMaxV, "supply_capacity_ma": &c.SupplyCapacityMA, "ambient_min_c": &c.AmbientMinC, "ambient_max_c": &c.AmbientMaxC, "total_bus_capacitance_pf": &c.TotalBusCapacitancePF} {
		if value, ok := r.values[key]; ok {
			*dest = value
		}
	}
	if _, err := Check(c); err != nil {
		return localDecision(prompt, "unsupported", err.Error()+"; no requested bound was weakened.", nil)
	}
	return localDecision(prompt, "supported", "All request terms match the bounded local grammar and reviewed family limits. Values are declared operating bounds, not measurements or hardware guarantees.", &c)
}

func localDecision(prompt, disposition, message string, config *Config) Decision {
	return Decision{Disposition: disposition, Message: message, Clauses: []Clause{{Text: prompt, Disposition: disposition, Reason: message}}, Configuration: config}
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
