package capabilityevaluation

import (
	"bytes"
	"slices"
	"testing"
)

func TestEvaluateRanksFrequencySafetyAndTrustedReuse(t *testing.T) {
	cases := []CaseResult{
		readyCase("ready_analog", DomainAnalog),
		blockedCase("clock_digital", DomainDigital, SafetyReviewRequired, OutcomeUnsupported, "clock_fanout_loading", "architecture_search", "CAPABILITY_UNSUPPORTED"),
		blockedCase("clock_mcu", DomainMCU, SafetyRelevant, OutcomeUnsupported, "clock_fanout_loading", "architecture_search", "CAPABILITY_UNSUPPORTED"),
		blockedCase("debug_mcu", DomainMCU, SafetyCritical, OutcomeUnsupported, "mcu_debug_loading", "architecture_search", "CAPABILITY_UNSUPPORTED"),
		blockedCase("clarify_sensor", DomainSensor, SafetyReviewRequired, OutcomeNeedsClarification, "bus_voltage_domain", "intent_compile", "CLARIFICATION_REQUIRED"),
		blockedCase("ambiguous_mixed", DomainMixedSignal, SafetyRelevant, OutcomeAmbiguous, "bus_direction_control", "architecture_search", "SEARCH_AMBIGUOUS"),
		blockedCase("budget_power", DomainPower, SafetyCritical, OutcomeBudgetExhausted, "isolated_converter_search", "architecture_search", "SEARCH_BUDGET_EXHAUSTED"),
	}
	registry := ImpactRegistry{Version: "impact_registry_v1", Records: []ImpactRecord{
		{Capability: "clock_fanout_loading", Consumers: []string{"clock_distribution", "mcu_clock_integration"}},
		{Capability: "clock_distribution", Consumers: []string{"multi_device_timing"}},
		{Capability: "mcu_debug_loading", Consumers: []string{"mcu_programming_interface"}},
	}}
	report, err := Evaluate(cases, registry, DefaultRankingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if report.Schema != ReportSchema || report.CaseCount != len(cases) || len(report.RankedClusters) != 5 {
		t.Fatalf("report summary = %#v", report)
	}
	first := report.RankedClusters[0]
	if first.Capability != "clock_fanout_loading" || first.FrequencyScore != 2 ||
		first.SafetyScore != 4 || first.ReuseScore != 3 || first.DomainCount != 2 {
		t.Fatalf("first cluster = %#v", first)
	}
	if !slices.Equal(first.DownstreamReuse, []string{"clock_distribution", "mcu_clock_integration", "multi_device_timing"}) {
		t.Fatalf("downstream reuse = %#v", first.DownstreamReuse)
	}
	assertOutcomeCount(t, report, OutcomeReady, 1)
	assertOutcomeCount(t, report, OutcomeNeedsClarification, 1)
	assertOutcomeCount(t, report, OutcomeUnsupported, 3)
	assertOutcomeCount(t, report, OutcomeAmbiguous, 1)
	assertOutcomeCount(t, report, OutcomeBudgetExhausted, 1)
}

func TestEvaluateIsByteStableUnderAllInputReordering(t *testing.T) {
	cases := []CaseResult{
		blockedCase("case_b", DomainPower, SafetyCritical, OutcomeUnsupported, "converter_fault_model", "closed_loop", "MODEL_UNAVAILABLE"),
		blockedCase("case_a", DomainAnalog, SafetyRelevant, OutcomeUnsupported, "converter_fault_model", "closed_loop", "MODEL_UNAVAILABLE"),
		readyCase("case_c", DomainSensor),
	}
	cases[0].Observations = append(cases[0].Observations, Observation{
		Capability: "converter_fault_model", Outcome: OutcomeUnsupported, Stage: "closed_loop",
		Code: "MODEL_UNAVAILABLE", Path: "requirements.events", Reason: "fault model missing",
		RequiredEvidence: []string{"reviewed transient model", "reviewed fault model"},
	})
	registry := ImpactRegistry{Version: "impact_registry_v1", Records: []ImpactRecord{
		{Capability: "converter_fault_model", Consumers: []string{"converter_protection", "power_tree_fault_response"}},
		{Capability: "converter_protection", Consumers: []string{"safe_power_conversion"}},
	}}
	first, err := Evaluate(cases, registry, DefaultRankingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(cases)
	slices.Reverse(cases[2].Observations)
	slices.Reverse(registry.Records)
	for index := range registry.Records {
		slices.Reverse(registry.Records[index].Consumers)
	}
	second, err := Evaluate(cases, registry, DefaultRankingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := first.MarshalJSONStable()
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := second.MarshalJSONStable()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("reports differ under reorder\nfirst:\n%s\nsecond:\n%s", firstJSON, secondJSON)
	}
	cluster := first.RankedClusters[0]
	if cluster.FrequencyScore != 2 || !slices.Equal(cluster.RequiredEvidence, []string{
		"reviewed converter_fault_model evidence",
		"reviewed fault model",
		"reviewed transient model",
	}) {
		t.Fatalf("deduplicated cluster = %#v", cluster)
	}
}

func TestEvaluateRejectsMalformedTerminalEvidence(t *testing.T) {
	validRegistry := ImpactRegistry{Version: "impact_registry_v1"}
	tests := []struct {
		name  string
		cases []CaseResult
	}{
		{name: "unknown outcome", cases: []CaseResult{{ID: "case_a", Domain: DomainAnalog, SafetyImpact: SafetyNonSafety, Outcome: "tool_error"}}},
		{name: "ready observation", cases: []CaseResult{{
			ID: "case_a", Domain: DomainAnalog, SafetyImpact: SafetyNonSafety, Outcome: OutcomeReady,
			Observations: []Observation{{Capability: "gain", Outcome: OutcomeReady}},
		}}},
		{name: "missing observation", cases: []CaseResult{{ID: "case_a", Domain: DomainAnalog, SafetyImpact: SafetyNonSafety, Outcome: OutcomeUnsupported}}},
		{name: "mismatched outcome", cases: []CaseResult{{
			ID: "case_a", Domain: DomainAnalog, SafetyImpact: SafetyNonSafety, Outcome: OutcomeUnsupported,
			Observations: []Observation{{
				Capability: "gain_model", Outcome: OutcomeAmbiguous, Stage: "closed_loop", Code: "MODEL_UNAVAILABLE",
				Path: "requirements.gain", Reason: "missing", RequiredEvidence: []string{"reviewed gain model"},
			}},
		}}},
		{name: "fixture capability", cases: []CaseResult{{
			ID: "case_a", Domain: DomainAnalog, SafetyImpact: SafetyNonSafety, Outcome: OutcomeUnsupported,
			Observations: []Observation{{
				Capability: "Fixture-A", Outcome: OutcomeUnsupported, Stage: "closed_loop", Code: "MODEL_UNAVAILABLE",
				Path: "requirements.gain", Reason: "missing", RequiredEvidence: []string{"reviewed gain model"},
			}},
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Evaluate(tt.cases, validRegistry, DefaultRankingPolicy()); err == nil {
				t.Fatal("expected evaluation error")
			}
		})
	}
}

func TestEvaluateRejectsInvalidImpactRegistry(t *testing.T) {
	cases := []CaseResult{readyCase("case_a", DomainAnalog)}
	registries := []ImpactRegistry{
		{Version: "impact_registry_v1", Records: []ImpactRecord{{Capability: "clock_fanout", Consumers: []string{"clock_fanout"}}}},
		{Version: "impact_registry_v1", Records: []ImpactRecord{
			{Capability: "clock_fanout", Consumers: []string{"clock_distribution"}},
			{Capability: "clock_distribution", Consumers: []string{"clock_fanout"}},
		}},
		{Version: "impact_registry_v1", Records: []ImpactRecord{{Capability: "Clock Fanout"}}},
	}
	for index, registry := range registries {
		if _, err := Evaluate(cases, registry, DefaultRankingPolicy()); err == nil {
			t.Fatalf("registry %d unexpectedly passed", index)
		}
	}
}

func readyCase(id string, domain Domain) CaseResult {
	return CaseResult{ID: id, Domain: domain, SafetyImpact: SafetyNonSafety, Outcome: OutcomeReady, Observations: []Observation{}}
}

func blockedCase(id string, domain Domain, safety SafetyImpact, outcome Outcome, capability, stage, code string) CaseResult {
	return CaseResult{
		ID: id, Domain: domain, SafetyImpact: safety, Outcome: outcome,
		Observations: []Observation{{
			Capability: capability, Outcome: outcome, Stage: stage, Code: code,
			Path: "requirements." + capability, Reason: "trusted evidence is unavailable",
			RequiredEvidence: []string{"reviewed " + capability + " evidence"},
		}},
	}
}

func assertOutcomeCount(t *testing.T, report Report, outcome Outcome, want int) {
	t.Helper()
	for _, count := range report.OutcomeCounts {
		if count.Key == string(outcome) {
			if count.Count != want {
				t.Fatalf("outcome %s count = %d, want %d", outcome, count.Count, want)
			}
			return
		}
	}
	t.Fatalf("outcome %s missing", outcome)
}
