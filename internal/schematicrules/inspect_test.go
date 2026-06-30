package schematicrules

import (
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
)

func TestInspectDetectsDuplicateNonPowerReferences(t *testing.T) {
	file := ruleTestSchematic()
	file.Symbols = []schematic.SchematicSymbol{
		ruleTestSymbol("R1", kicadfiles.MM(10), kicadfiles.MM(10)),
		ruleTestSymbol("r1", kicadfiles.MM(20), kicadfiles.MM(10)),
		ruleTestSymbol("#PWR01", kicadfiles.MM(30), kicadfiles.MM(10)),
		ruleTestSymbol("#PWR01", kicadfiles.MM(40), kicadfiles.MM(10)),
	}

	report := Inspect(file, Options{Scope: ScopeGenerated})
	if !hasRule(report, RuleReferenceDuplicate) {
		t.Fatalf("report missing duplicate reference: %#v", report.Findings)
	}
	if countRule(report, RuleReferenceDuplicate) != 2 {
		t.Fatalf("duplicate reference count = %d", countRule(report, RuleReferenceDuplicate))
	}
}

func TestInspectIgnoresVirtualReferenceDuplicates(t *testing.T) {
	file := ruleTestSchematic()
	file.Symbols = []schematic.SchematicSymbol{
		ruleTestSymbol("#SYM01", kicadfiles.MM(10), kicadfiles.MM(10)),
		ruleTestSymbol("#SYM01", kicadfiles.MM(20), kicadfiles.MM(10)),
	}

	report := Inspect(file, Options{Scope: ScopeGenerated})
	if hasRule(report, RuleReferenceDuplicate) {
		t.Fatalf("virtual references should not produce duplicate findings: %#v", report.Findings)
	}
}

func TestInspectIgnoresUnannotatedReferenceDuplicates(t *testing.T) {
	file := ruleTestSchematic()
	file.Symbols = []schematic.SchematicSymbol{
		ruleTestSymbol("R?", kicadfiles.MM(10), kicadfiles.MM(10)),
		ruleTestSymbol("R?", kicadfiles.MM(20), kicadfiles.MM(10)),
	}

	report := Inspect(file, Options{Scope: ScopeImported})
	if hasRule(report, RuleReferenceDuplicate) {
		t.Fatalf("unannotated references should not produce duplicate findings: %#v", report.Findings)
	}
}

func TestInspectDetectsFloatingAndEmptyLabels(t *testing.T) {
	file := ruleTestSchematic()
	file.Labels = []schematic.Label{{
		UUID:     kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		Text:     "  ",
		Kind:     schematic.LabelLocal,
		Position: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)},
	}}

	report := Inspect(file, Options{})
	if !hasRule(report, RuleLabelEmpty) || !hasRule(report, RuleLabelFloating) {
		t.Fatalf("report missing label findings: %#v", report.Findings)
	}
}

func TestInspectHandlesNonLocalLabelKindsFromFlattenedLabels(t *testing.T) {
	file := ruleTestSchematic()
	point := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	file.Labels = []schematic.Label{{
		UUID:     kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		Text:     "GLOBAL_NET",
		Kind:     schematic.LabelGlobal,
		Position: point,
	}}

	report := Inspect(file, Options{})
	if !hasRule(report, RuleLabelFloating) {
		t.Fatalf("global label kind in flattened label list should be inspected: %#v", report.Findings)
	}
}

func TestInspectTreatsJunctionOnlyLabelAsFloating(t *testing.T) {
	file := ruleTestSchematic()
	point := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	file.Junctions = []schematic.Junction{{
		UUID:     kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		Position: point,
	}}
	file.Labels = []schematic.Label{{
		UUID:     kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		Text:     "NET",
		Kind:     schematic.LabelLocal,
		Position: point,
	}}

	report := Inspect(file, Options{})
	if !hasRule(report, RuleLabelFloating) {
		t.Fatalf("junction-only label should be floating: %#v", report.Findings)
	}
}

func TestInspectTreatsLabelOnWireSegmentAsAnchored(t *testing.T) {
	file := ruleTestSchematic()
	start := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	mid := kicadfiles.Point{X: kicadfiles.MM(15), Y: kicadfiles.MM(10)}
	end := kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)}
	file.Wires = []schematic.Wire{{
		UUID:   kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		Points: []kicadfiles.Point{start, end},
	}}
	file.Labels = []schematic.Label{{
		UUID:     kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		Text:     "NET",
		Kind:     schematic.LabelLocal,
		Position: mid,
	}}

	report := Inspect(file, Options{})
	if hasRule(report, RuleLabelFloating) {
		t.Fatalf("label on wire segment should be anchored: %#v", report.Findings)
	}
}

func TestInspectTreatsLabelOnDiagonalWireSegmentAsAnchored(t *testing.T) {
	file := ruleTestSchematic()
	start := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	mid := kicadfiles.Point{X: kicadfiles.MM(15), Y: kicadfiles.MM(15)}
	end := kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}
	file.Wires = []schematic.Wire{{
		UUID:   kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		Points: []kicadfiles.Point{start, end},
	}}
	file.Labels = []schematic.Label{{
		UUID:     kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		Text:     "NET",
		Kind:     schematic.LabelLocal,
		Position: mid,
	}}

	report := Inspect(file, Options{})
	if hasRule(report, RuleLabelFloating) {
		t.Fatalf("label on diagonal wire segment should be anchored: %#v", report.Findings)
	}
}

func TestInspectDetectsConnectedLabelConflict(t *testing.T) {
	file := ruleTestSchematic()
	a := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	b := kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)}
	file.Wires = []schematic.Wire{{UUID: kicadfiles.UUID("11111111-1111-4111-8111-111111111111"), Points: []kicadfiles.Point{a, b}}}
	file.Labels = []schematic.Label{
		{UUID: kicadfiles.UUID("22222222-2222-4222-8222-222222222222"), Text: "SDA", Kind: schematic.LabelLocal, Position: a},
		{UUID: kicadfiles.UUID("33333333-3333-4333-8333-333333333333"), Text: "SCL", Kind: schematic.LabelLocal, Position: b},
	}

	report := Inspect(file, Options{})
	if !hasRule(report, RuleLabelConflict) {
		t.Fatalf("report missing label conflict: %#v", report.Findings)
	}
}

func TestInspectDetectsSegmentConnectedLabelConflict(t *testing.T) {
	file := ruleTestSchematic()
	start := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	mid := kicadfiles.Point{X: kicadfiles.MM(15), Y: kicadfiles.MM(10)}
	end := kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)}
	file.Wires = []schematic.Wire{{UUID: kicadfiles.UUID("11111111-1111-4111-8111-111111111111"), Points: []kicadfiles.Point{start, end}}}
	file.Labels = []schematic.Label{
		{UUID: kicadfiles.UUID("22222222-2222-4222-8222-222222222222"), Text: "SDA", Kind: schematic.LabelLocal, Position: mid},
		{UUID: kicadfiles.UUID("33333333-3333-4333-8333-333333333333"), Text: "SCL", Kind: schematic.LabelLocal, Position: end},
	}

	report := Inspect(file, Options{})
	if !hasRule(report, RuleLabelConflict) {
		t.Fatalf("report missing segment-connected label conflict: %#v", report.Findings)
	}
}

func TestInspectDetectsSameAnchorLabelConflictWithoutWire(t *testing.T) {
	file := ruleTestSchematic()
	anchor := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	file.Symbols = []schematic.SchematicSymbol{ruleTestSymbol("U1", anchor.X, anchor.Y)}
	file.Labels = []schematic.Label{
		{UUID: kicadfiles.UUID("22222222-2222-4222-8222-222222222222"), Text: "VCC", Kind: schematic.LabelLocal, Position: anchor},
		{UUID: kicadfiles.UUID("33333333-3333-4333-8333-333333333333"), Text: "GND", Kind: schematic.LabelLocal, Position: anchor},
	}

	report := Inspect(file, Options{})
	if !hasRule(report, RuleLabelConflict) {
		t.Fatalf("report missing same-anchor label conflict: %#v", report.Findings)
	}
}

func TestInspectDetectsLabelNormalizationCollision(t *testing.T) {
	file := ruleTestSchematic()
	a := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	b := kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)}
	file.Wires = []schematic.Wire{
		{UUID: kicadfiles.UUID("11111111-1111-4111-8111-111111111111"), Points: []kicadfiles.Point{a, {X: kicadfiles.MM(15), Y: kicadfiles.MM(10)}}},
		{UUID: kicadfiles.UUID("22222222-2222-4222-8222-222222222222"), Points: []kicadfiles.Point{b, {X: kicadfiles.MM(25), Y: kicadfiles.MM(10)}}},
	}
	file.Labels = []schematic.Label{
		{UUID: kicadfiles.UUID("33333333-3333-4333-8333-333333333333"), Text: "Net  1", Kind: schematic.LabelLocal, Position: a},
		{UUID: kicadfiles.UUID("44444444-4444-4444-8444-444444444444"), Text: "net 1", Kind: schematic.LabelLocal, Position: b},
	}

	report := Inspect(file, Options{})
	if !hasRule(report, RuleLabelNormalizationCollision) {
		t.Fatalf("report missing normalization collision: %#v", report.Findings)
	}
}

func TestInspectDetectsNoConnectAwayFromPin(t *testing.T) {
	file := ruleTestSchematic()
	file.Symbols = []schematic.SchematicSymbol{ruleTestSymbol("R1", kicadfiles.MM(10), kicadfiles.MM(10))}
	file.NoConnects = []schematic.NoConnect{{
		UUID:     kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		Position: kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(10)},
	}}

	report := Inspect(file, Options{})
	if !hasRule(report, RulePinNoConnectMissing) {
		t.Fatalf("report missing no-connect finding: %#v", report.Findings)
	}
}

func TestInspectDetectsNoConnectOnConnectedPin(t *testing.T) {
	file := ruleTestSchematic()
	anchor := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	file.Symbols = []schematic.SchematicSymbol{ruleTestSymbol("R1", anchor.X, anchor.Y)}
	file.NoConnects = []schematic.NoConnect{{
		UUID:     kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		Position: anchor,
	}}
	file.Labels = []schematic.Label{{
		UUID:     kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		Text:     "NET",
		Kind:     schematic.LabelLocal,
		Position: anchor,
	}}

	report := Inspect(file, Options{})
	if !hasRule(report, RulePinNoConnectViolated) {
		t.Fatalf("report missing connected no-connect finding: %#v", report.Findings)
	}
}

func TestInspectDetectsNoConnectOnPinConnectedByWireSegment(t *testing.T) {
	file := ruleTestSchematic()
	start := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	anchor := kicadfiles.Point{X: kicadfiles.MM(15), Y: kicadfiles.MM(10)}
	end := kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)}
	file.Symbols = []schematic.SchematicSymbol{ruleTestSymbol("R1", anchor.X, anchor.Y)}
	file.Wires = []schematic.Wire{{UUID: kicadfiles.UUID("33333333-3333-4333-8333-333333333333"), Points: []kicadfiles.Point{start, end}}}
	file.NoConnects = []schematic.NoConnect{{
		UUID:     kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		Position: anchor,
	}}

	report := Inspect(file, Options{})
	if !hasRule(report, RulePinNoConnectViolated) {
		t.Fatalf("report missing wire-segment connected no-connect finding: %#v", report.Findings)
	}
}

func TestInspectPinIntentsDetectRequiredPinOpen(t *testing.T) {
	file := ruleTestSchematic()
	anchor := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	file.Symbols = []schematic.SchematicSymbol{ruleTestSymbol("U1", anchor.X, anchor.Y)}

	report := Inspect(file, Options{PinIntents: []PinIntent{ruleTestPinIntent("U1", "1", anchor, PinIntentRequired)}})
	if !hasRule(report, RulePinRequiredOpen) || report.CheckedRequiredPins != 1 {
		t.Fatalf("report missing required pin finding/count: %#v", report)
	}
}

func TestInspectPinIntentsAcceptRequiredPinConnectedByLabel(t *testing.T) {
	file := ruleTestSchematic()
	anchor := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	file.Symbols = []schematic.SchematicSymbol{ruleTestSymbol("U1", anchor.X, anchor.Y)}
	file.Labels = []schematic.Label{{
		UUID:     kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		Text:     "VCC",
		Kind:     schematic.LabelLocal,
		Position: anchor,
	}}

	report := Inspect(file, Options{PinIntents: []PinIntent{ruleTestPinIntent("U1", "1", anchor, PinIntentRequired)}})
	if hasRule(report, RulePinRequiredOpen) {
		t.Fatalf("connected required pin should pass: %#v", report.Findings)
	}
}

func TestInspectPinIntentsOptionalOpenWarns(t *testing.T) {
	file := ruleTestSchematic()
	anchor := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	file.Symbols = []schematic.SchematicSymbol{ruleTestSymbol("U1", anchor.X, anchor.Y)}

	report := Inspect(file, Options{PinIntents: []PinIntent{ruleTestPinIntent("U1", "2", anchor, PinIntentOptional)}})
	if !hasRule(report, RulePinOptionalOpen) {
		t.Fatalf("report missing optional open finding: %#v", report.Findings)
	}
}

func TestInspectPinIntentsNoConnectPassesWithMarker(t *testing.T) {
	file := ruleTestSchematic()
	anchor := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	file.Symbols = []schematic.SchematicSymbol{ruleTestSymbol("U1", anchor.X, anchor.Y)}
	file.NoConnects = []schematic.NoConnect{{UUID: kicadfiles.UUID("11111111-1111-4111-8111-111111111111"), Position: anchor}}

	report := Inspect(file, Options{PinIntents: []PinIntent{ruleTestPinIntent("U1", "3", anchor, PinIntentNoConnect)}})
	if hasRule(report, RulePinNoConnectMissing) {
		t.Fatalf("no-connect intent with marker should pass: %#v", report.Findings)
	}
}

func TestInspectPinIntentsNoConnectFailsWhenConnected(t *testing.T) {
	file := ruleTestSchematic()
	anchor := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	file.Symbols = []schematic.SchematicSymbol{ruleTestSymbol("U1", anchor.X, anchor.Y)}
	file.NoConnects = []schematic.NoConnect{{UUID: kicadfiles.UUID("11111111-1111-4111-8111-111111111111"), Position: anchor}}
	file.Labels = []schematic.Label{{UUID: kicadfiles.UUID("22222222-2222-4222-8222-222222222222"), Text: "NET", Kind: schematic.LabelLocal, Position: anchor}}

	report := Inspect(file, Options{PinIntents: []PinIntent{ruleTestPinIntent("U1", "3", anchor, PinIntentNoConnect)}})
	if !hasRule(report, RulePinNoConnectViolated) {
		t.Fatalf("connected no-connect intent should fail: %#v", report.Findings)
	}
}

func TestInspectPinIntentsRequiredNoConnectBlocks(t *testing.T) {
	file := ruleTestSchematic()
	anchor := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	file.Symbols = []schematic.SchematicSymbol{ruleTestSymbol("U1", anchor.X, anchor.Y)}
	file.NoConnects = []schematic.NoConnect{{UUID: kicadfiles.UUID("11111111-1111-4111-8111-111111111111"), Position: anchor}}

	report := Inspect(file, Options{PinIntents: []PinIntent{ruleTestPinIntent("U1", "1", anchor, PinIntentRequired)}})
	if !hasRule(report, RulePinNoConnectOnRequired) {
		t.Fatalf("required pin with no-connect should block: %#v", report.Findings)
	}
}

func TestInspectPinIntentsExternalAcceptedPasses(t *testing.T) {
	file := ruleTestSchematic()
	anchor := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	file.Symbols = []schematic.SchematicSymbol{ruleTestSymbol("J1", anchor.X, anchor.Y)}
	intent := ruleTestPinIntent("J1", "1", anchor, PinIntentExternal)
	intent.Net = "VIN"

	report := Inspect(file, Options{AcceptedExternalRails: []string{"vin"}, PinIntents: []PinIntent{intent}})
	if hasRule(report, RulePinRequiredOpen) || report.CheckedRequiredPins != 1 {
		t.Fatalf("accepted external pin should pass and count as required: %#v", report)
	}
}

func TestInspectPinIntentsMissingMetadataBlocksWhenRequired(t *testing.T) {
	file := ruleTestSchematic()
	file.Symbols = []schematic.SchematicSymbol{ruleTestSymbol("U1", kicadfiles.MM(10), kicadfiles.MM(10))}

	report := Inspect(file, Options{RequireConfidence: true})
	if !hasRule(report, RulePinMetadataMissing) || report.Status != StatusBlocked {
		t.Fatalf("report missing metadata blocker: %#v", report)
	}
}

func TestInspectPowerRailSinkWithoutSourceBlocks(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{PowerRails: []PowerRail{{Name: "VCC", SinkRefs: []string{"U1"}}}})
	if !hasRule(report, RulePowerSourceMissing) || report.CheckedPowerRails != 1 {
		t.Fatalf("report missing power source blocker: %#v", report)
	}
}

func TestInspectPowerRailMissingNameTakesPrecedence(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{PowerRails: []PowerRail{{SinkRefs: []string{"U1"}}}})
	if !hasRule(report, RulePowerMetadataMissing) || hasRule(report, RulePowerSourceMissing) {
		t.Fatalf("missing rail name should report metadata first: %#v", report.Findings)
	}
}

func TestInspectPowerRailDoesNotMutateInputRefs(t *testing.T) {
	file := ruleTestSchematic()
	rails := []PowerRail{{Name: "VCC", SourceRefs: []string{"", "U1"}, SinkRefs: []string{"", "U2"}}}

	_ = Inspect(file, Options{PowerRails: rails})
	if len(rails[0].SourceRefs) != 2 || rails[0].SourceRefs[0] != "" || rails[0].SourceRefs[1] != "U1" {
		t.Fatalf("source refs mutated: %#v", rails[0].SourceRefs)
	}
	if len(rails[0].SinkRefs) != 2 || rails[0].SinkRefs[0] != "" || rails[0].SinkRefs[1] != "U2" {
		t.Fatalf("sink refs mutated: %#v", rails[0].SinkRefs)
	}
}

func TestInspectPowerRailExternalAcceptedPasses(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{
		AcceptedExternalRails: []string{"vcc"},
		PowerRails:            []PowerRail{{Name: "VCC", SinkRefs: []string{"U1"}}},
	})
	if hasRule(report, RulePowerSourceMissing) {
		t.Fatalf("accepted external rail should pass: %#v", report.Findings)
	}
}

func TestInspectPowerRailSourceWithoutSinkWarns(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{PowerRails: []PowerRail{{Name: "VCC", SourceRefs: []string{"U1"}}}})
	if !hasRule(report, RulePowerSinkMissing) || report.Status != StatusWarning {
		t.Fatalf("report missing power sink warning: %#v", report)
	}
}

func TestInspectPowerFlagWithoutRailBlocks(t *testing.T) {
	file := ruleTestSchematic()
	file.Symbols = []schematic.SchematicSymbol{{
		UUID:       kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		LibraryID:  "power:PWR_FLAG",
		Reference:  "#FLG01",
		Value:      "PWR_FLAG",
		Position:   kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
		PinAnchors: []kicadfiles.Point{{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}},
	}}

	report := Inspect(file, Options{})
	if !hasRule(report, RulePowerFlagWithoutRail) {
		t.Fatalf("report missing floating PWR_FLAG finding: %#v", report.Findings)
	}
}

func TestInspectConnectedPowerFlagPasses(t *testing.T) {
	file := ruleTestSchematic()
	anchor := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	file.Symbols = []schematic.SchematicSymbol{{
		UUID:       kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		LibraryID:  "power:PWR_FLAG",
		Reference:  "#FLG01",
		Value:      "PWR_FLAG",
		Position:   anchor,
		PinAnchors: []kicadfiles.Point{anchor},
	}}
	file.Labels = []schematic.Label{{UUID: kicadfiles.UUID("22222222-2222-4222-8222-222222222222"), Text: "VCC", Kind: schematic.LabelLocal, Position: anchor}}

	report := Inspect(file, Options{})
	if hasRule(report, RulePowerFlagWithoutRail) {
		t.Fatalf("connected PWR_FLAG should pass: %#v", report.Findings)
	}
}

func TestInspectDecouplingRequirementMissingBlocks(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{Decoupling: []DecouplingRequirement{{Reference: "U1", Rail: "VCC"}}})
	if !hasRule(report, RuleDecouplingMissing) || report.CheckedDecouplingRequirements != 1 {
		t.Fatalf("report missing decoupling blocker: %#v", report)
	}
}

func TestInspectDecouplingIgnoresEmptyPlaceholders(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{Decoupling: []DecouplingRequirement{{}}})
	if hasRule(report, RuleDecouplingMissing) || report.CheckedDecouplingRequirements != 0 {
		t.Fatalf("empty decoupling placeholder should not produce findings or counts: %#v", report)
	}
}

func TestInspectDecouplingRequirementSatisfiedPasses(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{Decoupling: []DecouplingRequirement{{Reference: "U1", Rail: "VCC", CapacitorRefs: []string{"C1"}}}})
	if hasRule(report, RuleDecouplingMissing) {
		t.Fatalf("satisfied decoupling should pass: %#v", report.Findings)
	}
}

func TestInspectDecouplingValueAndRailMismatch(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{Decoupling: []DecouplingRequirement{{
		Reference:     "U1",
		CapacitorRefs: []string{"C1"},
		ExpectedValue: "100n",
		ActualValue:   "10n",
		ExpectedRail:  "VCC",
		ActualRail:    "GND",
	}}})
	if !hasRule(report, RuleDecouplingValueMismatch) || !hasRule(report, RuleDecouplingRailMismatch) {
		t.Fatalf("report missing decoupling mismatch findings: %#v", report.Findings)
	}
}

func TestInspectDecouplingEquivalentValuesPass(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{Decoupling: []DecouplingRequirement{{
		Reference:     "U1",
		CapacitorRefs: []string{"C1"},
		ExpectedValue: "100n",
		ActualValue:   "0.1uF",
	}}})
	if hasRule(report, RuleDecouplingValueMismatch) {
		t.Fatalf("equivalent decoupling values should pass: %#v", report.Findings)
	}
}

func TestInspectDecouplingMicroSymbolValuePasses(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{Decoupling: []DecouplingRequirement{{
		Reference:     "U1",
		CapacitorRefs: []string{"C1"},
		ExpectedValue: "1µF",
		ActualValue:   "1000n",
	}}})
	if hasRule(report, RuleDecouplingValueMismatch) {
		t.Fatalf("micro symbol decoupling value should parse: %#v", report.Findings)
	}
}

func TestInspectDecouplingGreekMuValuePasses(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{Decoupling: []DecouplingRequirement{{
		Reference:     "U1",
		CapacitorRefs: []string{"C1"},
		ExpectedValue: "1μF",
		ActualValue:   "1000n",
	}}})
	if hasRule(report, RuleDecouplingValueMismatch) {
		t.Fatalf("greek mu decoupling value should parse: %#v", report.Findings)
	}
}

func TestInspectDecouplingEmbeddedUnitValuePasses(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{Decoupling: []DecouplingRequirement{{
		Reference:     "U1",
		CapacitorRefs: []string{"C1"},
		ExpectedValue: "4n7",
		ActualValue:   "4700pF",
	}}})
	if hasRule(report, RuleDecouplingValueMismatch) {
		t.Fatalf("embedded unit decoupling value should parse: %#v", report.Findings)
	}
}

func TestInspectDecouplingLeadingUnitValuePasses(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{Decoupling: []DecouplingRequirement{{
		Reference:     "U1",
		CapacitorRefs: []string{"C1"},
		ExpectedValue: "u1",
		ActualValue:   "100n",
	}}})
	if hasRule(report, RuleDecouplingValueMismatch) {
		t.Fatalf("leading unit decoupling value should parse: %#v", report.Findings)
	}
}

func TestInspectValueChecks(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{ValueChecks: []ValueCheck{
		{Reference: "R1", Required: true},
		{Reference: "C1", Value: "abc", ParseOK: false},
		{Reference: "R2", Value: "999M", ParseOK: true, OutOfPolicy: true},
	}})
	if report.CheckedValueChecks != 3 {
		t.Fatalf("CheckedValueChecks = %d, want 3", report.CheckedValueChecks)
	}
	for _, rule := range []RuleID{RuleValueMissing, RuleValueParseFailed, RuleValueOutOfPolicy} {
		if !hasRule(report, rule) {
			t.Fatalf("report missing %s: %#v", rule, report.Findings)
		}
	}
}

func TestInspectRatingChecks(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{
		Acceptance: AcceptanceFabricationCandidate,
		RatingChecks: []RatingCheck{
			{Reference: "C1", Kind: "voltage", Required: 10, Actual: 6.3, ActualKnown: true, Unit: "V", Evidence: true},
			{Reference: "R1", Kind: "power", Required: 0.125, Unit: "W", Evidence: false},
		},
	})
	if report.CheckedRatingChecks != 2 {
		t.Fatalf("CheckedRatingChecks = %d, want 2", report.CheckedRatingChecks)
	}
	if !hasRule(report, RuleRatingInsufficient) || !hasRule(report, RuleRatingEvidenceMissing) {
		t.Fatalf("report missing rating findings: %#v", report.Findings)
	}
}

func TestInspectRatingZeroActualIsInsufficient(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{RatingChecks: []RatingCheck{{Reference: "C1", Kind: "voltage", Required: 10, Actual: 0, ActualKnown: true, Unit: "V", Evidence: true}}})
	if !hasRule(report, RuleRatingInsufficient) {
		t.Fatalf("zero actual rating should be insufficient: %#v", report.Findings)
	}
}

func TestInspectRatingUnknownActualDoesNotCompare(t *testing.T) {
	file := ruleTestSchematic()

	report := Inspect(file, Options{RatingChecks: []RatingCheck{{Reference: "C1", Kind: "voltage", Required: 10, Evidence: true}}})
	if hasRule(report, RuleRatingInsufficient) {
		t.Fatalf("unknown actual rating should not be compared: %#v", report.Findings)
	}
}

func TestIntentCoordinateConversionPreservesInternalUnits(t *testing.T) {
	value := int64(kicadfiles.MM(123.456))
	if got := int64(iuFromIntentCoordinate(value)); got != value {
		t.Fatalf("IU conversion = %d, want %d", got, value)
	}
}

func ruleTestSchematic() schematic.SchematicFile {
	return schematic.SchematicFile{
		Version:          "20250114",
		Generator:        "kicadai-test",
		GeneratorVersion: "test",
		UUID:             kicadfiles.UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		Paper:            kicadfiles.Paper{Name: "A4"},
	}
}

func ruleTestSymbol(reference string, x kicadfiles.IU, y kicadfiles.IU) schematic.SchematicSymbol {
	point := kicadfiles.Point{X: x, Y: y}
	return schematic.SchematicSymbol{
		UUID:       kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
		LibraryID:  "Device:R",
		Reference:  reference,
		Value:      "1k",
		Position:   point,
		PinAnchors: []kicadfiles.Point{point},
	}
}

func ruleTestPinIntent(reference string, pin string, point kicadfiles.Point, kind PinIntentKind) PinIntent {
	return PinIntent{
		Reference: reference,
		Pin:       pin,
		Position:  Point{X: int64(point.X), Y: int64(point.Y)},
		Kind:      kind,
	}
}

func hasRule(report Report, rule RuleID) bool {
	return countRule(report, rule) > 0
}

func countRule(report Report, rule RuleID) int {
	count := 0
	for _, finding := range report.Findings {
		if finding.RuleID == rule {
			count++
		}
	}
	return count
}
