package blocks

import (
	"context"
	"slices"
	"testing"
)

func TestLEDIndicatorDefinitionDeclaresHardeningRules(t *testing.T) {
	definition := ledIndicatorDefinition()
	rules := validationRuleIDs(definition.ValidationRules)
	for _, id := range []string{
		"led.current.positive",
		"led.forward_voltage.below_supply",
		"led.polarity.evidence",
		"led.resistor.required",
		"led.series_route.required",
	} {
		if !slices.Contains(rules, id) {
			t.Errorf("LED validation rules = %#v, missing %s", rules, id)
		}
	}
	if definition.Verification.Level != VerificationStructural {
		t.Fatalf("LED verification = %q", definition.Verification.Level)
	}
}

func TestConnectorBreakoutDefinitionDeclaresHardeningRules(t *testing.T) {
	definition := connectorBreakoutDefinition()
	rules := validationRuleIDs(definition.ValidationRules)
	for _, id := range []string{
		"connector.pin_names.required",
		"connector.pin_count.matches_names",
		"connector.pin_numbering.evidence",
		"connector.symbol.resolved",
		"connector.footprint.resolved",
	} {
		if !slices.Contains(rules, id) {
			t.Errorf("connector validation rules = %#v, missing %s", rules, id)
		}
	}
	if definition.Verification.Level != VerificationStructural {
		t.Fatalf("connector verification = %q", definition.Verification.Level)
	}
}

func TestConnectorBreakoutFailsClosedOnDuplicatePins(t *testing.T) {
	output, issues := NewBuiltinRegistry().Instantiate(context.Background(), BlockRequest{
		BlockID:    "connector_breakout",
		InstanceID: "JX",
		Params: map[string]any{
			"pin_names": []string{"VIN", "VIN"},
		},
	})
	if len(issues) == 0 || !hasBlockingIssues(issues) {
		t.Fatalf("issues = %#v output = %#v", issues, output)
	}
}

func validationRuleIDs(rules []BlockValidationRule) []string {
	ids := make([]string, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ID)
	}
	slices.Sort(ids)
	return ids
}
