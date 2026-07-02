package designworkflow

import (
	"encoding/json"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/components"
	"kicadai/internal/transactions"
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

func TestComponentRoutingHintsEnforcesPowerNetClassWidth(t *testing.T) {
	result := componentRoutingHints([]ComponentSelectionEntry{{
		InstanceID:  "rail",
		Role:        "regulator",
		ComponentID: "regulator.linear.ap2112k_3v3.sot23_5",
		FunctionPins: []components.FunctionPin{
			{Function: "VOUT", SymbolPin: "5", Electrical: "power_out"},
		},
		RoutingHints: []components.RoutingHint{
			{Kind: "net_class", NetRole: "power", Value: "0.3", Unit: "mm"},
		},
	}}, PCBFragmentResult{Fragments: []BlockFragment{{
		InstanceID: "rail",
		Realization: blocks.BlockPCBRealizationResult{
			RoleRefs: map[string]string{"regulator": "U1"},
			LocalRoutes: []blocks.RealizedPCBLocalRoute{
				{
					ID: "quiet_name",
					From: transactions.Endpoint{
						Ref: "U1",
						Pin: "5",
					},
					To:      transactions.Endpoint{Ref: "C2", Pin: "1"},
					NetName: "rail_output",
					WidthMM: 0.5,
				},
			},
		},
	}}})

	if len(result.Evidence) != 1 || result.Evidence[0].Status != ComponentHintEnforced {
		t.Fatalf("evidence = %#v", result.Evidence)
	}
}

func TestComponentRoutingHintsSatisfiedByBlockTieAndNoConnect(t *testing.T) {
	connect := testOperation(t, transactions.OpConnect, transactions.ConnectOperation{
		Op:      transactions.OpConnect,
		From:    transactions.Endpoint{Ref: "U1", Pin: "3"},
		To:      transactions.Endpoint{Ref: "U1", Pin: "1"},
		NetName: "rail_vin",
	})
	noConnect := testOperation(t, transactions.OpAddNoConnect, transactions.AddNoConnectOperation{
		Op:       transactions.OpAddNoConnect,
		Endpoint: transactions.Endpoint{Ref: "U1", Pin: "4"},
	})
	result := componentRoutingHints([]ComponentSelectionEntry{{
		InstanceID:  "rail",
		Role:        "regulator",
		ComponentID: "regulator.linear.ap2112k_3v3.sot23_5",
		FunctionPins: []components.FunctionPin{
			{Function: "EN", SymbolPin: "3"},
			{Function: "NC", SymbolPin: "4"},
		},
		RoutingHints: []components.RoutingHint{
			{Kind: "tie", NetRole: "enable"},
			{Kind: "no_connect", NetRole: "nc"},
		},
	}}, PCBFragmentResult{Fragments: []BlockFragment{{
		InstanceID: "rail",
		Realization: blocks.BlockPCBRealizationResult{
			RoleRefs:   map[string]string{"regulator": "U1"},
			Operations: []transactions.Operation{connect, noConnect},
		},
	}}})

	if len(result.Evidence) != 2 {
		t.Fatalf("evidence = %#v", result.Evidence)
	}
	if got := hintByKind(result.Evidence, "tie"); got == nil || got.Status != ComponentHintSatisfiedByBlock {
		t.Fatalf("tie evidence = %#v", got)
	}
	if got := hintByKind(result.Evidence, "no_connect"); got == nil || got.Status != ComponentHintSatisfiedByBlock {
		t.Fatalf("no_connect evidence = %#v", got)
	}
}

func TestComponentRoutingHintsDoesNotSatisfyWrongNoConnectPin(t *testing.T) {
	noConnect := testOperation(t, transactions.OpAddNoConnect, transactions.AddNoConnectOperation{
		Op:       transactions.OpAddNoConnect,
		Endpoint: transactions.Endpoint{Ref: "U1", Pin: "5"},
	})
	result := componentRoutingHints([]ComponentSelectionEntry{{
		InstanceID:  "rail",
		Role:        "regulator",
		ComponentID: "regulator.linear.ap2112k_3v3.sot23_5",
		FunctionPins: []components.FunctionPin{
			{Function: "NC", SymbolPin: "4"},
		},
		RoutingHints: []components.RoutingHint{
			{Kind: "no_connect", NetRole: "nc"},
		},
	}}, PCBFragmentResult{Fragments: []BlockFragment{{
		InstanceID: "rail",
		Realization: blocks.BlockPCBRealizationResult{
			RoleRefs:   map[string]string{"regulator": "U1"},
			Operations: []transactions.Operation{noConnect},
		},
	}}})

	if len(result.Evidence) != 1 || result.Evidence[0].Status == ComponentHintSatisfiedByBlock {
		t.Fatalf("evidence = %#v", result.Evidence)
	}
}

func TestComponentRoutingHintsPreservesUnsupportedHint(t *testing.T) {
	result := componentRoutingHints([]ComponentSelectionEntry{{
		InstanceID:  "clock",
		Role:        "crystal",
		ComponentID: "crystal.abracon.abm3b.16mhz",
		RoutingHints: []components.RoutingHint{
			{Kind: "short_loop", NetRole: "clock"},
		},
	}}, PCBFragmentResult{})

	if len(result.Evidence) != 1 || result.Evidence[0].Status != ComponentHintUnsupported {
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

func testOperation(t *testing.T, kind transactions.OperationKind, payload any) transactions.Operation {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal operation: %v", err)
	}
	return transactions.NewOperation(kind, raw)
}
