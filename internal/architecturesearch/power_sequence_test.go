package architecturesearch

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"kicadai/internal/components"
)

func TestValidatePowerSequenceConstraintUsesProducerStartupEvidence(t *testing.T) {
	requirement := powerTreeRequirement(false)
	selections := powerSequenceSelections(t, 0.001, 0.002)
	constraint := Constraint{Name: "rail_sequence_before", Relation: "required", Value: json.RawMessage(`["rail_a_signal","rail_b_signal"]`)}
	check, validation := validatePowerSequenceConstraint(requirement, selections, constraint, "candidate.system_constraints.rail_sequence_before")
	if validation != nil || check.Margin == nil || *check.Margin != 0.001 {
		t.Fatalf("sequence proof = %#v validation=%#v", check, validation)
	}

	delay := Constraint{Name: "rail_sequence_delay", Relation: "minimum", Unit: "s", Value: json.RawMessage(`{"before":"rail_a_signal","after":"rail_b_signal","seconds":0.0005}`)}
	check, validation = validatePowerSequenceConstraint(requirement, selections, delay, "candidate.system_constraints.rail_sequence_delay")
	if validation != nil || check.Observed == nil || *check.Observed != 0.001 {
		t.Fatalf("delay proof = %#v validation=%#v", check, validation)
	}
}

func TestValidatePowerSequenceConstraintFailsClosed(t *testing.T) {
	requirement := powerTreeRequirement(false)
	constraint := Constraint{Name: "rail_sequence_before", Relation: "required", Value: json.RawMessage(`["rail_a_signal","rail_b_signal"]`)}
	_, validation := validatePowerSequenceConstraint(requirement, powerSequenceSelections(t, 0.003, 0.002), constraint, "sequence")
	if validation == nil || validation.Code != CodePowerSequenceUnproven {
		t.Fatalf("reversed sequence validation = %#v", validation)
	}

	monotonic := Constraint{Name: "startup_monotonic", Relation: "required", Value: json.RawMessage(`"rail_a_signal"`)}
	_, validation = validatePowerSequenceConstraint(requirement, powerSequenceSelections(t, 0.001, 0.002), monotonic, "monotonic")
	if validation == nil || validation.Code != CodePowerSequenceUnproven {
		t.Fatalf("unproven monotonic startup = %#v", validation)
	}
}

func TestValidatePowerSequenceConstraintProvesMonotonicityAndInrush(t *testing.T) {
	requirement := powerTreeRequirement(false)
	selections := powerSequenceSelectionsWithStartupEvidence(t, .2)
	monotonic := Constraint{Name: "startup_monotonic", Relation: "required", Value: json.RawMessage(`"rail_a_signal"`)}
	check, validation := validatePowerSequenceConstraint(requirement, selections, monotonic, "monotonic")
	if validation != nil || check.Observed == nil || *check.Observed != 1 {
		t.Fatalf("monotonic proof = %#v validation=%#v", check, validation)
	}
	inrush := Constraint{Name: "startup_inrush_current", Relation: "maximum", Unit: "A", Value: json.RawMessage(`{"signal":"rail_a_signal","current_a":0.25}`)}
	check, validation = validatePowerSequenceConstraint(requirement, selections, inrush, "inrush")
	if validation != nil || check.Margin == nil || math.Abs(*check.Margin-.05) > 1e-12 {
		t.Fatalf("inrush proof = %#v validation=%#v", check, validation)
	}

	inrush.Value = json.RawMessage(`{"signal":"rail_a_signal","current_a":0.1}`)
	_, validation = validatePowerSequenceConstraint(requirement, selections, inrush, "inrush")
	if validation == nil || validation.Code != CodePowerSequenceUnproven {
		t.Fatalf("excess inrush validation = %#v", validation)
	}
}

func TestValidateNamedStartupAndShutdownOrderFromSequencerTopology(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{
		Capability: "rail_sequencing",
		Ports: []RoleContract{
			sequencingRole("enable", externalAnchor("enable"), "digital_logic", "sink", 0, 3.3),
			sequencingRole("rail_a", signalAnchor("alpha_source"), "power", "sink", 3.2, 3.4),
			sequencingRole("rail_b", signalAnchor("beta_source"), "power", "sink", 4.85, 5.15),
			sequencingRole("state", signalAnchor("sequence_state"), "digital_logic", "source", 0, 3.3),
			sequencingRole("reference", externalAnchor("ground"), "reference", "bidirectional", 0, 0),
		},
		Constraints: []Constraint{constraintNumber("sequence_delay", "target", .01, "s", 50)},
	}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("rail sequencer expansion=%#v err=%v", expansions, err)
	}
	expansion := expansions[0]
	selection := FragmentSelection{
		Capability: "rail_sequencing", Ports: expansion.OfferedPorts,
		Calculations: expansion.Calculations, Payload: expansion.Payload,
	}
	requirement := Requirement{Requirements: Requirements{
		Domains: []Domain{
			{ID: "alpha", Kind: "supply", Source: "alpha_source"},
			{ID: "beta", Kind: "supply", Source: "beta_source"},
		},
		Signals: []Signal{
			{ID: "alpha_source", Kind: "power", Domain: "alpha"},
			{ID: "beta_source", Kind: "power", Domain: "beta"},
		},
	}}
	for _, constraint := range []Constraint{
		{Name: "startup_order", Relation: "required", Value: json.RawMessage(`"alpha_before_beta"`)},
		{Name: "shutdown_order", Relation: "required", Value: json.RawMessage(`"beta_before_alpha"`)},
	} {
		check, validation := validatePowerSequenceConstraint(requirement, []FragmentSelection{selection}, constraint, constraint.Name)
		if validation != nil || check.Observed == nil || *check.Observed <= 0 {
			t.Fatalf("%s check=%#v validation=%#v", constraint.Name, check, validation)
		}
	}

	reversed := Constraint{Name: "startup_order", Relation: "required", Value: json.RawMessage(`"beta_before_alpha"`)}
	if _, validation := validatePowerSequenceConstraint(requirement, []FragmentSelection{selection}, reversed, reversed.Name); validation == nil || validation.Code != CodePowerSequenceUnproven {
		t.Fatalf("reversed startup order validation = %#v", validation)
	}
	withoutTopology := selection
	withoutTopology.Payload = nil
	if _, validation := validatePowerSequenceConstraint(requirement, []FragmentSelection{withoutTopology}, Constraint{Name: "shutdown_order", Relation: "required", Value: json.RawMessage(`"beta_before_alpha"`)}, "shutdown"); validation == nil || validation.Code != CodePowerSequenceUnproven {
		t.Fatalf("missing sequencer topology validation = %#v", validation)
	}
}

func TestCatalogRailSequencerSizesCatalogBackedShutdownEnergy(t *testing.T) {
	baseCatalog := loadArchitectureCatalog(t)
	var firstPayload string
	var firstCalculationHash string
	for index, catalog := range []*components.Catalog{baseCatalog, reversedArchitectureCatalog(baseCatalog)} {
		provider, err := NewCatalogProvider(catalog)
		if err != nil {
			t.Fatal(err)
		}
		railB := sequencingRole("rail_b", signalAnchor("beta_source"), "power", "sink", 4.85, 5.15)
		railB.Contract.RequiredCurrentCapacityA = float64Pointer(1)
		railB.Contract.MaximumCurrentDemandA = float64Pointer(1)
		request := ProviderRequest{
			Capability: "rail_sequencing",
			Ports: []RoleContract{
				sequencingRole("enable", externalAnchor("enable"), "digital_logic", "sink", 0, 3.3),
				sequencingRole("rail_a", signalAnchor("alpha_source"), "power", "sink", 3.2, 3.4),
				railB,
				sequencingRole("state", signalAnchor("sequence_state"), "digital_logic", "source", 0, 3.3),
				sequencingRole("reference", externalAnchor("ground"), "reference", "bidirectional", 0, 0),
			},
			Constraints: []Constraint{
				constraintNumber("sequence_delay", "target", .011, "s", 81.81818181818181),
				constraintNumber("shutdown_delay", "target", .0105, "s", 90.47619047619048),
				constraintNumber("load_current", "target", .51, "A", 96.07843137254902),
				constraintNumber("ambient_temperature_minimum", "minimum", -20, "degC", 0),
				constraintNumber("ambient_temperature", "maximum", 70, "degC", 0),
			},
		}
		expansions, err := provider.Expand(context.Background(), request)
		if err != nil || len(expansions) == 0 {
			t.Fatalf("catalog order %d rail sequencer expansion=%#v err=%v", index, expansions, err)
		}
		realization, err := DecodeFragmentRealization(expansions[0].Payload)
		if err != nil {
			t.Fatal(err)
		}
		holdUpCount, legacyMonitorCount := 0, 0
		for _, instance := range realization.Instances {
			switch instance.CatalogID {
			case "capacitor.panasonic.eeufr1a682l.radial":
				holdUpCount++
			}
			if instance.ID == "sequence_rail_b_monitor" {
				legacyMonitorCount++
			}
		}
		if holdUpCount != 1 || legacyMonitorCount != 0 {
			t.Fatalf("catalog order %d hold-up/legacy-monitor count = %d/%d; realization=%#v", index, holdUpCount, legacyMonitorCount, realization.Instances)
		}
		railBinding := false
		for _, binding := range realization.PortBindings {
			railBinding = railBinding || binding.Role == "rail_b" && binding.Instance == "sequence_rail_b_hold_up_1" && binding.Function == "A"
		}
		if !railBinding {
			t.Fatalf("catalog order %d lacks deterministic hold-up rail binding: %#v", index, realization.PortBindings)
		}
		var holdUpCalculation CalculationEvidence
		for _, calculation := range expansions[0].Calculations {
			if calculation.ID == "rail_b_shutdown_hold_up" {
				holdUpCalculation = calculation
				break
			}
		}
		if holdUpCalculation.Hash == "" || !holdUpCalculation.Pass || len(ValidateCalculation(holdUpCalculation)) != 0 {
			t.Fatalf("catalog order %d hold-up calculation = %#v", index, holdUpCalculation)
		}
		minimumDelay, minimumOK := 0.0, false
		for _, output := range holdUpCalculation.Corners[0].Outputs {
			if output.Name == "shutdown_delay" {
				minimumDelay, minimumOK = output.Value, true
				break
			}
		}
		if !minimumOK || minimumDelay < .001 {
			t.Fatalf("catalog order %d minimum shutdown delay = %.9g, want >= 1 ms", index, minimumDelay)
		}
		payload := string(expansions[0].Payload)
		if index == 0 {
			firstPayload, firstCalculationHash = payload, holdUpCalculation.Hash
		} else if payload != firstPayload || holdUpCalculation.Hash != firstCalculationHash {
			t.Fatalf("catalog order changed hold-up result: payload_equal=%t hash=%q/%q", payload == firstPayload, firstCalculationHash, holdUpCalculation.Hash)
		}
	}
}

func TestCatalogBehaviorCalculationsExposeTypedRegulatorStartupEvidence(t *testing.T) {
	request := ProviderRequest{Constraints: []Constraint{{Name: "startup_monotonic", Relation: "required", Value: json.RawMessage(`"rail"`)}}}
	parts := []catalogPart{{
		usage: "regulator", selected: SelectedComponent{InstanceID: "regulator"},
		record: components.ComponentRecord{Regulator: &components.RegulatorEvidence{
			StartupTime: &components.EvidenceMeasurement{Value: 2e-3, Unit: "s"}, StartupMonotonicStatus: "proven",
			MaximumInrushCurrent: &components.EvidenceMeasurement{Value: 200, Unit: "mA"},
		}},
	}}
	calculations, unproven, err := catalogBehaviorCalculations(request, parts)
	if err != nil || unproven != 0 {
		t.Fatalf("calculations error=%v unproven=%d", err, unproven)
	}
	want := map[string]float64{"startup_time": .002, "startup_monotonic": 1, "startup_inrush_current": .2}
	for _, calculation := range calculations {
		for _, output := range calculation.NominalOutputs {
			if expected, ok := want[output.Name]; ok && output.Value == expected {
				delete(want, output.Name)
			}
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing startup outputs %#v in %#v", want, calculations)
	}
}

func powerSequenceSelections(t *testing.T, first, second float64) []FragmentSelection {
	t.Helper()
	firstCalculation, err := ObservedCalculation("rail_a_startup", NamedQuantity{Name: "startup_time", Value: first, Unit: "s"})
	if err != nil {
		t.Fatal(err)
	}
	secondCalculation, err := ObservedCalculation("rail_b_startup", NamedQuantity{Name: "startup_time", Value: second, Unit: "s"})
	if err != nil {
		t.Fatal(err)
	}
	return []FragmentSelection{
		{ObligationPath: "objective:make_a", Calculations: []CalculationEvidence{firstCalculation}},
		{ObligationPath: "objective:make_b", Calculations: []CalculationEvidence{secondCalculation}},
	}
}

func powerSequenceSelectionsWithStartupEvidence(t *testing.T, inrush float64) []FragmentSelection {
	t.Helper()
	monotonic, err := ObservedCalculation("rail_a_monotonic", NamedQuantity{Name: "startup_monotonic", Value: 1, Unit: "ratio"})
	if err != nil {
		t.Fatal(err)
	}
	boundedInrush, err := ObservedCalculation("rail_a_inrush", NamedQuantity{Name: "startup_inrush_current", Value: inrush, Unit: "A"})
	if err != nil {
		t.Fatal(err)
	}
	return []FragmentSelection{{ObligationPath: "objective:make_a", Calculations: []CalculationEvidence{monotonic, boundedInrush}}}
}

func sequencingRole(role, anchor, kind, direction string, minimum, maximum float64) RoleContract {
	result := providerRole(role, kind, direction, minimum, maximum)
	result.Anchor = anchor
	return result
}
