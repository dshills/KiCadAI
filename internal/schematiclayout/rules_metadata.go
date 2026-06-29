package schematiclayout

import "sort"

// RuleProfile identifies a reader-facing schematic readability profile.
type RuleProfile string

const (
	RuleProfileStandard  RuleProfile = "standard"
	RuleProfileAmplifier RuleProfile = "amplifier"
)

// RuleProfileMetadata describes a readability profile and its enforced rule IDs.
type RuleProfileMetadata struct {
	ID          RuleProfile `json:"id"`
	Description string      `json:"description"`
	RuleIDs     []string    `json:"rule_ids"`
}

// RuleMetadata describes one stable readability diagnostic rule.
type RuleMetadata struct {
	ID          string        `json:"id"`
	Code        string        `json:"code"`
	Severity    Severity      `json:"severity"`
	Profiles    []RuleProfile `json:"profiles"`
	Description string        `json:"description"`
	Repair      string        `json:"repair"`
}

var ruleProfileDescriptions = map[RuleProfile]string{
	RuleProfileStandard:  "General schematic readability for small generated boards and block diagrams.",
	RuleProfileAmplifier: "Audio and low-frequency analog amplifier readability for signal flow, rails, feedback, output stage, and returns.",
}
var readabilityRules = buildReadabilityRules()
var readabilityRuleByID = indexReadabilityRulesByID(readabilityRules)
var readabilityRuleByCode = indexReadabilityRules(readabilityRules)
var readabilityRuleProfiles = buildRuleProfiles(readabilityRules)

// RuleProfiles returns deterministic metadata for supported readability profiles.
func RuleProfiles() []RuleProfileMetadata {
	return cloneRuleProfileMetadataSlice(readabilityRuleProfiles)
}

// ReadabilityRules returns deterministic metadata for stable readability diagnostics.
func ReadabilityRules() []RuleMetadata {
	return cloneRuleMetadataSlice(readabilityRules)
}

func buildReadabilityRules() []RuleMetadata {
	rules := []RuleMetadata{
		ruleMetadata("standard.diagonal_wire", "diagonal_wire", SeverityError, "Schematic wires must be orthogonal.", "reroute the net with horizontal and vertical wire segments", RuleProfileStandard, RuleProfileAmplifier),
		ruleMetadata("standard.symbol_overlap", "symbol_overlap", SeverityError, "Symbols must not overlap each other.", "spread overlapping components apart on the sheet", RuleProfileStandard),
		ruleMetadata("standard.wire_symbol_overlap", "wire_symbol_overlap", SeverityError, "Wires must not cross through unrelated symbol bodies.", "reroute the wire around the symbol body or move the component", RuleProfileStandard),
		ruleMetadata("standard.text_symbol_overlap", "text_symbol_overlap", SeverityWarning, "Text should not overlap symbol bodies.", "move the text or spread nearby components", RuleProfileStandard),
		ruleMetadata("standard.text_wire_overlap", "text_wire_overlap", SeverityWarning, "Text should not overlap wires.", "move the text away from routed nets", RuleProfileStandard),
		ruleMetadata("standard.label_overlap", "label_overlap", SeverityWarning, "Labels should not overlap each other.", "move one label or use a less crowded net-label location", RuleProfileStandard),
		ruleMetadata("standard.page_overflow", "page_overflow", SeverityWarning, "Placed components should remain inside the preferred readable sheet area.", "move the component inside the readable area or split the design across sheets", RuleProfileStandard),
		ruleMetadata("standard.missing_pin_anchor", "missing_pin_anchor", SeverityWarning, "Readable routing should have endpoint pin anchors.", "add symbol pin anchors or use net labels for the connection", RuleProfileStandard),
		ruleMetadata("amplifier.active_missing", "amplifier_active_missing", SeverityError, "Amplifier schematics must expose an active gain stage.", "add or classify the active gain stage symbol", RuleProfileAmplifier),
		ruleMetadata("amplifier.input_missing", "amplifier_input_missing", SeverityError, "Amplifier schematics must expose an input label.", "add an input net label left of the gain stage", RuleProfileAmplifier),
		ruleMetadata("amplifier.input_flow", "amplifier_input_flow", SeverityError, "Amplifier input must be left of the active gain stage.", "move the input label or gain stage to restore left-to-right flow", RuleProfileAmplifier),
		ruleMetadata("amplifier.output_missing", "amplifier_output_missing", SeverityError, "Amplifier schematics must expose an output label.", "add an output net label right of the output stage", RuleProfileAmplifier),
		ruleMetadata("amplifier.output_flow", "amplifier_output_flow", SeverityError, "Amplifier output must be right of the active gain stage.", "move the output label or output stage to the right of the gain stage", RuleProfileAmplifier),
		ruleMetadata("amplifier.feedback_missing", "amplifier_feedback_missing", SeverityWarning, "Amplifier feedback should be identifiable.", "add a feedback label or route the feedback network above the active stage", RuleProfileAmplifier),
		ruleMetadata("amplifier.feedback_position", "amplifier_feedback_position", SeverityError, "Amplifier feedback should be above the active stage.", "move the feedback network above the gain stage", RuleProfileAmplifier),
		ruleMetadata("amplifier.positive_rail_position", "amplifier_positive_rail_position", SeverityError, "Positive rails should be above the active stage.", "move positive rail symbols above the signal lane", RuleProfileAmplifier),
		ruleMetadata("amplifier.return_position", "amplifier_return_position", SeverityError, "Ground, load, and return symbols should be below the active stage.", "move ground, load, and return symbols below the signal lane", RuleProfileAmplifier),
		ruleMetadata("amplifier.output_stage_flow", "amplifier_output_stage_flow", SeverityError, "Output stages should be right of the active gain stage.", "move the output stage to the right of the gain stage", RuleProfileAmplifier),
	}
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].ID < rules[j].ID
	})
	return rules
}

// RuleForDiagnostic returns metadata for a stable diagnostic code.
func RuleForDiagnostic(code string) (RuleMetadata, bool) {
	rule, ok := readabilityRuleByCode[code]
	if !ok {
		return RuleMetadata{}, false
	}
	return cloneRuleMetadata(rule), true
}

// RuleByID returns metadata for a stable readability rule ID.
func RuleByID(id string) (RuleMetadata, bool) {
	rule, ok := readabilityRuleByID[id]
	if !ok {
		return RuleMetadata{}, false
	}
	return cloneRuleMetadata(rule), true
}

// RulesForProfile returns the rules associated with a reader-facing profile.
func RulesForProfile(profile RuleProfile) []RuleMetadata {
	rules := []RuleMetadata{}
	for _, rule := range readabilityRules {
		if ruleAppliesToProfile(rule, profile) {
			rules = append(rules, cloneRuleMetadata(rule))
		}
	}
	return rules
}

func ruleMetadata(id string, code string, severity Severity, description string, repair string, profiles ...RuleProfile) RuleMetadata {
	return RuleMetadata{
		ID:          id,
		Code:        code,
		Severity:    severity,
		Profiles:    append([]RuleProfile(nil), profiles...),
		Description: description,
		Repair:      repair,
	}
}

func indexReadabilityRules(rules []RuleMetadata) map[string]RuleMetadata {
	index := make(map[string]RuleMetadata, len(rules))
	for _, rule := range rules {
		if _, exists := index[rule.Code]; exists {
			panic("duplicate readability rule code: " + rule.Code)
		}
		index[rule.Code] = cloneRuleMetadata(rule)
	}
	return index
}

func indexReadabilityRulesByID(rules []RuleMetadata) map[string]RuleMetadata {
	index := make(map[string]RuleMetadata, len(rules))
	for _, rule := range rules {
		if _, exists := index[rule.ID]; exists {
			panic("duplicate readability rule id: " + rule.ID)
		}
		index[rule.ID] = cloneRuleMetadata(rule)
	}
	return index
}

func cloneRuleMetadataSlice(rules []RuleMetadata) []RuleMetadata {
	cloned := make([]RuleMetadata, 0, len(rules))
	for _, rule := range rules {
		cloned = append(cloned, cloneRuleMetadata(rule))
	}
	return cloned
}

func cloneRuleMetadata(rule RuleMetadata) RuleMetadata {
	rule.Profiles = append([]RuleProfile(nil), rule.Profiles...)
	return rule
}

func buildRuleProfiles(rules []RuleMetadata) []RuleProfileMetadata {
	profileRules := map[RuleProfile][]string{}
	for _, rule := range rules {
		for _, profile := range rule.Profiles {
			profileRules[profile] = append(profileRules[profile], rule.ID)
		}
	}
	profileIDs := make([]RuleProfile, 0, len(profileRules))
	for profile := range profileRules {
		profileIDs = append(profileIDs, profile)
	}
	sort.Slice(profileIDs, func(i, j int) bool {
		return ruleProfileRank(profileIDs[i]) < ruleProfileRank(profileIDs[j])
	})
	profiles := make([]RuleProfileMetadata, 0, len(profileIDs))
	for _, profile := range profileIDs {
		ruleIDs := append([]string(nil), profileRules[profile]...)
		sort.Strings(ruleIDs)
		profiles = append(profiles, RuleProfileMetadata{
			ID:          profile,
			Description: ruleProfileDescriptions[profile],
			RuleIDs:     ruleIDs,
		})
	}
	return profiles
}

func cloneRuleProfileMetadataSlice(profiles []RuleProfileMetadata) []RuleProfileMetadata {
	cloned := make([]RuleProfileMetadata, 0, len(profiles))
	for _, profile := range profiles {
		profile.RuleIDs = append([]string(nil), profile.RuleIDs...)
		cloned = append(cloned, profile)
	}
	return cloned
}

func ruleAppliesToProfile(rule RuleMetadata, profile RuleProfile) bool {
	for _, candidate := range rule.Profiles {
		if candidate == profile {
			return true
		}
	}
	return false
}

func ruleProfileRank(profile RuleProfile) int {
	switch profile {
	case RuleProfileStandard:
		return 0
	case RuleProfileAmplifier:
		return 1
	default:
		return 100
	}
}
