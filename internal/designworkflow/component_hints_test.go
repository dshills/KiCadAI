package designworkflow

import (
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/components"
)

func TestNormalizeComponentHintsClassifiesSupportedAndUnsupportedKinds(t *testing.T) {
	hints := NormalizeComponentHints([]ComponentSelectionEntry{{
		InstanceID:  "rail",
		BlockID:     "voltage_regulator",
		Role:        "regulator",
		ComponentID: "regulator.linear.ap2112k_3v3.sot23_5",
		PlacementHints: []components.PlacementHint{
			{Kind: "near", Target: "input_capacitor", Value: "2", Unit: "mm"},
		},
		RoutingHints: []components.RoutingHint{
			{Kind: "net_class", NetRole: "power", Value: "0.3", Unit: "mm"},
			{Kind: "short_loop", NetRole: "clock"},
		},
	}})

	if len(hints) != 3 {
		t.Fatalf("hints = %#v", hints)
	}
	summary := SummarizeComponentHints(hints)
	if summary.Total != 3 || summary.Placement != 1 || summary.Routing != 2 || summary.Supported != 2 || summary.Unsupported != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if got := hintByKind(hints, "short_loop"); got == nil || got.Status != ComponentHintUnsupported {
		t.Fatalf("short_loop hint = %#v", got)
	}
	if got := hintByKind(hints, "net_class"); got == nil || got.Status != ComponentHintPending {
		t.Fatalf("net_class hint = %#v", got)
	}
}

func TestNormalizeComponentHintsDeduplicatesDeterministically(t *testing.T) {
	hints := NormalizeComponentHints([]ComponentSelectionEntry{{
		InstanceID:  "rail",
		Role:        "regulator",
		ComponentID: "regulator.linear.ams1117_3v3.sot223",
		PlacementHints: []components.PlacementHint{
			{Kind: "near", Target: "output_capacitor", Value: "3", Unit: "mm"},
			{Kind: "near", Target: "output_capacitor", Value: "3", Unit: "mm"},
			{Kind: "near", Target: "input_capacitor", Value: "3", Unit: "mm"},
		},
	}})

	if len(hints) != 2 {
		t.Fatalf("hints = %#v", hints)
	}
	if hints[0].Target != "input_capacitor" || hints[1].Target != "output_capacitor" {
		t.Fatalf("hints not sorted deterministically: %#v", hints)
	}
}

func TestComponentPlacementHintRulesEnforcesNearHint(t *testing.T) {
	result := componentPlacementHintRules([]ComponentSelectionEntry{{
		InstanceID:  "rail",
		BlockID:     "voltage_regulator",
		Role:        "regulator",
		ComponentID: "regulator.linear.ap2112k_3v3.sot23_5",
		PlacementHints: []components.PlacementHint{
			{Kind: "near", Target: "output_capacitor", Value: "3", Unit: "mm"},
		},
	}}, PCBFragmentResult{Fragments: []BlockFragment{{
		InstanceID: "rail",
		BlockID:    "voltage_regulator",
		Realization: blocks.BlockPCBRealizationResult{RoleRefs: map[string]string{
			"regulator":        "U1",
			"output_capacitor": "C2",
		}},
	}}})

	if len(result.Rules) != 1 {
		t.Fatalf("rules = %#v", result.Rules)
	}
	rule := result.Rules[0]
	if rule.AnchorRef != "U1" || len(rule.TargetRefs) != 1 || rule.TargetRefs[0] != "C2" || rule.MaxDistanceMM != 3 {
		t.Fatalf("rule = %#v", rule)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Status != ComponentHintEnforced {
		t.Fatalf("evidence = %#v", result.Evidence)
	}
}

func TestComponentPlacementHintRulesSkipsMissingRefs(t *testing.T) {
	result := componentPlacementHintRules([]ComponentSelectionEntry{{
		InstanceID:  "rail",
		Role:        "regulator",
		ComponentID: "regulator.linear.ap2112k_3v3.sot23_5",
		PlacementHints: []components.PlacementHint{
			{Kind: "near", Target: "output_capacitor", Value: "3", Unit: "mm"},
		},
	}}, PCBFragmentResult{Fragments: []BlockFragment{{
		InstanceID: "rail",
		Realization: blocks.BlockPCBRealizationResult{RoleRefs: map[string]string{
			"regulator": "U1",
		}},
	}}})

	if len(result.Rules) != 0 {
		t.Fatalf("rules = %#v", result.Rules)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Status != ComponentHintSkipped {
		t.Fatalf("evidence = %#v", result.Evidence)
	}
}

func TestComponentPlacementHintRulesSkipsUnsupportedUnitsWithoutValue(t *testing.T) {
	result := componentPlacementHintRules([]ComponentSelectionEntry{{
		InstanceID:  "rail",
		Role:        "regulator",
		ComponentID: "regulator.linear.ap2112k_3v3.sot23_5",
		PlacementHints: []components.PlacementHint{
			{Kind: "near", Target: "input_capacitor", Unit: "mil"},
		},
	}}, PCBFragmentResult{Fragments: []BlockFragment{{
		InstanceID: "rail",
		Realization: blocks.BlockPCBRealizationResult{RoleRefs: map[string]string{
			"regulator":       "U1",
			"input_capacitor": "C1",
		}},
	}}})

	if len(result.Rules) != 0 {
		t.Fatalf("rules = %#v", result.Rules)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Status != ComponentHintSkipped {
		t.Fatalf("evidence = %#v", result.Evidence)
	}
}

func TestComponentPlacementHintRulesPreservesNonPlacementEvidence(t *testing.T) {
	result := componentPlacementHintRules([]ComponentSelectionEntry{{
		InstanceID:  "rail",
		Role:        "regulator",
		ComponentID: "regulator.linear.ap2112k_3v3.sot23_5",
		RoutingHints: []components.RoutingHint{
			{Kind: "net_class", NetRole: "power", Value: "0.3", Unit: "mm"},
		},
	}}, PCBFragmentResult{})

	if len(result.Rules) != 0 {
		t.Fatalf("rules = %#v", result.Rules)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Type != ComponentHintRouting || result.Evidence[0].Status != ComponentHintPending {
		t.Fatalf("evidence = %#v", result.Evidence)
	}
}

func hintByKind(hints []ComponentHintEvidence, kind string) *ComponentHintEvidence {
	for index := range hints {
		if hints[index].Kind == kind {
			return &hints[index]
		}
	}
	return nil
}
