package blocks

import (
	"slices"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

func TestEvaluateCommonRulesBlocksMissingRequiredSurfaces(t *testing.T) {
	definition := ledIndicatorDefinition()
	report := EvaluateCommonRules(RuleContext{
		Definition:     definition,
		RequiredRoles:  []string{"led", "resistor", "switch"},
		RequiredPorts:  []string{"GND", "IN", "OUT"},
		RequiredNets:   []string{"led_series", "missing_net"},
		RequiredRoutes: []string{"led_series"},
	})
	if !report.Blocking() {
		t.Fatalf("expected blocking report: %#v", report)
	}
	got := outcomeIDs(report.Outcomes)
	for _, id := range []string{
		"pcb_routes.unrealized",
		"required_net.missing_net",
		"required_port.OUT",
		"required_role.switch",
	} {
		if !slices.Contains(got, id) {
			t.Fatalf("outcomes = %#v, missing %s", got, id)
		}
	}
	if report.Readiness != BlockReadinessUnsupported {
		t.Fatalf("readiness = %q", report.Readiness)
	}
}

func TestEvaluateCommonRulesRequiresComponentEvidenceForConnectivity(t *testing.T) {
	definition := ledIndicatorDefinition()
	definition.Components[0].ComponentID = "generic_resistor_0805"
	report := EvaluateCommonRules(RuleContext{
		Definition: definition,
		Acceptance: components.AcceptanceConnectivity,
	})
	if !report.Blocking() {
		t.Fatalf("expected missing component evidence blocker: %#v", report)
	}
	if got := outcomeIDs(report.Outcomes); !slices.Contains(got, "component_evidence.none") {
		t.Fatalf("outcomes = %#v", got)
	}
}

func TestEvaluateCommonRulesRejectsUnsafeSelectedConfidence(t *testing.T) {
	definition := ledIndicatorDefinition()
	definition.Components[0].ComponentID = "placeholder_resistor"
	report := EvaluateCommonRules(RuleContext{
		Definition: definition,
		Acceptance: components.AcceptanceConnectivity,
		Selections: []BlockComponentSelection{{
			Role: "resistor",
			Selection: components.Selection{
				Candidate: components.Candidate{
					ComponentID: "placeholder_resistor",
					VariantID:   "0805",
					FootprintID: "Resistor_SMD:R_0805_2012Metric",
					Confidence:  components.ConfidencePlaceholder,
				},
			},
		}},
	})
	if !report.Blocking() {
		t.Fatalf("expected unsafe confidence blocker: %#v", report)
	}
	if got := outcomeIDs(report.Outcomes); !slices.Contains(got, "component_evidence.unsafe.resistor") {
		t.Fatalf("outcomes = %#v", got)
	}
}

func TestRuleOutcomeConvertsToIssue(t *testing.T) {
	outcome := RuleOutcome{
		ID:         "route",
		Kind:       RuleKindPCB,
		Severity:   RuleSeverityBlocker,
		Path:       "block.demo",
		Message:    "blocked",
		Refs:       []string{"R1"},
		Nets:       []string{"N1"},
		Suggestion: "fix it",
	}
	issue := outcome.Issue()
	if issue.Severity != reports.SeverityBlocked || issue.Path != outcome.Path || issue.Message != outcome.Message {
		t.Fatalf("issue = %#v", issue)
	}
	issue.Refs[0] = "mutated"
	issue.Nets[0] = "mutated"
	if outcome.Refs[0] != "R1" || outcome.Nets[0] != "N1" {
		t.Fatalf("outcome was mutated through issue slices: %#v", outcome)
	}
}

func outcomeIDs(outcomes []RuleOutcome) []string {
	ids := make([]string, 0, len(outcomes))
	for _, outcome := range outcomes {
		ids = append(ids, outcome.ID)
	}
	return ids
}
