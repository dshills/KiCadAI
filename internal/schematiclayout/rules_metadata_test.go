package schematiclayout

import "testing"

func TestReadabilityRuleProfilesExposeStableInventory(t *testing.T) {
	profiles := RuleProfiles()
	if len(profiles) != 2 {
		t.Fatalf("profile count = %d, want 2", len(profiles))
	}
	if profiles[0].ID != RuleProfileStandard || profiles[1].ID != RuleProfileAmplifier {
		t.Fatalf("profiles = %#v, want standard then amplifier", profiles)
	}
	for _, profile := range profiles {
		if profile.Description == "" {
			t.Fatalf("profile %s missing description", profile.ID)
		}
		if len(profile.RuleIDs) == 0 {
			t.Fatalf("profile %s has no rule IDs", profile.ID)
		}
		for _, ruleID := range profile.RuleIDs {
			if _, ok := RuleByID(ruleID); !ok {
				t.Fatalf("profile %s references missing rule ID %s", profile.ID, ruleID)
			}
		}
	}
}

func TestRuleForDiagnosticIncludesRepairGuidance(t *testing.T) {
	rule, ok := RuleForDiagnostic("amplifier_input_flow")
	if !ok {
		t.Fatal("missing amplifier_input_flow rule")
	}
	if rule.ID != "amplifier.input_flow" {
		t.Fatalf("rule ID = %q, want amplifier.input_flow", rule.ID)
	}
	if rule.Severity != SeverityError {
		t.Fatalf("severity = %q, want error", rule.Severity)
	}
	if rule.Repair == "" {
		t.Fatalf("missing repair guidance: %#v", rule)
	}
	if !ruleAppliesToProfile(rule, RuleProfileAmplifier) {
		t.Fatalf("rule does not apply to amplifier profile: %#v", rule)
	}
}

func TestRulesForProfileSeparatesStandardAndAmplifierRules(t *testing.T) {
	standard := RulesForProfile(RuleProfileStandard)
	amplifier := RulesForProfile(RuleProfileAmplifier)
	if !hasRuleCode(standard, "symbol_overlap") {
		t.Fatalf("standard rules missing symbol_overlap: %#v", standard)
	}
	if hasRuleCode(standard, "amplifier_input_flow") {
		t.Fatalf("standard rules unexpectedly include amplifier_input_flow")
	}
	if !hasRuleCode(amplifier, "amplifier_input_flow") {
		t.Fatalf("amplifier rules missing amplifier_input_flow")
	}
	if !hasRuleCode(amplifier, "diagonal_wire") {
		t.Fatalf("amplifier rules missing shared diagonal_wire rule")
	}
}

func hasRuleCode(rules []RuleMetadata, code string) bool {
	for _, rule := range rules {
		if rule.Code == code {
			return true
		}
	}
	return false
}
