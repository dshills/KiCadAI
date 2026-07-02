package components

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSelectConcreteResistorByPackageAndValue(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query: Query{
			Family:    "resistor",
			Package:   "0805",
			ValueKind: "resistance",
			Value:     "10k",
		},
		Acceptance: AcceptanceConnectivity,
	})
	if !result.OK {
		t.Fatalf("select resistor failed: %+v", result.Issues)
	}
	if selection.Component.Generic {
		t.Fatalf("selected generic resistor: %+v", selection.Component)
	}
	if selection.Component.Manufacturer != "Yageo" || selection.Component.MPN != "RC0805FR-0710KL" {
		t.Fatalf("selected resistor missing procurement identity: %+v", selection.Component)
	}
}

func TestSelectIncludesActiveProcurementEvidence(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	sources, err := LoadSources(context.Background(), SourceLoadOptions{SourceDir: sourceFixtureDir("valid")})
	if err != nil {
		t.Fatal(err)
	}
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:           Query{Family: "regulator", Package: "sot23_5", ValueKind: "output_voltage", Value: "3.3"},
		Acceptance:      AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{{Kind: "input_voltage", Value: "5", Unit: "V"}},
		Sources:         sources,
	})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if selection.Procurement == nil || selection.Procurement.LifecycleStatus != LifecycleActive || selection.Procurement.Outcome != "accepted" {
		t.Fatalf("procurement = %#v", selection.Procurement)
	}
}

func TestSelectBlocksObsoleteLifecycle(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	now := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	sources := sourceCollectionFor(SourceRecord{
		Manufacturer: "Advanced Monolithic Systems",
		MPN:          "AMS1117-3.3",
		SourceID:     "test",
		Lifecycle:    &LifecycleEvidence{Status: LifecycleObsolete, Source: "manual", SourceDate: "2026-06-26", Confidence: SourceConfidenceManualReview},
	})
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:       Query{Family: "regulator", Package: "sot223", ValueKind: "output_voltage", Value: "3.3"},
		Acceptance:  AcceptanceConnectivity,
		Sources:     sources,
		Procurement: ProcurementPolicy{Now: &now},
	})
	assertIssueCode(t, result.Issues, CodeComponentLifecycleBlocked)
}

func TestSelectBlocksStaleLifecycleForFabricationCandidate(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	now := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	sources, err := LoadSources(context.Background(), SourceLoadOptions{SourceDir: sourceFixtureDir("stale")})
	if err != nil {
		t.Fatal(err)
	}
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:       Query{Family: "regulator", Package: "sot23_5", ValueKind: "output_voltage", Value: "3.3"},
		Acceptance:  AcceptanceFabricationCandidate,
		Sources:     sources,
		Procurement: ProcurementPolicy{Now: &now},
	})
	assertIssueCode(t, result.Issues, CodeComponentLifecycleStale)
}

func TestSelectBlocksRequiredUnavailableAvailability(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	now := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	sources := sourceCollectionFor(SourceRecord{
		Manufacturer: "Diodes Incorporated",
		MPN:          "AP2112K-3.3",
		SourceID:     "test",
		Lifecycle:    &LifecycleEvidence{Status: LifecycleActive, Source: "manual", SourceDate: "2026-06-26", Confidence: SourceConfidenceManualReview},
		Availability: &AvailabilityEvidence{Status: AvailabilityUnavailable, Source: "manual", SourceDate: "2026-06-26", Confidence: SourceConfidenceManualReview},
	})
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:       Query{Family: "regulator", Package: "sot23_5", ValueKind: "output_voltage", Value: "3.3"},
		Acceptance:  AcceptanceConnectivity,
		Sources:     sources,
		Procurement: ProcurementPolicy{RequireAvailability: true, Now: &now},
	})
	assertIssueCode(t, result.Issues, CodeComponentAvailabilityBlocked)
}

func TestSelectRejectsCapacitorBelowVoltageRating(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query: Query{
			Family:  "capacitor",
			Package: "0805",
		},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{{
			Kind:  "voltage",
			Value: "100",
			Unit:  "V",
		}},
	})
	if result.OK {
		t.Fatal("expected voltage rating rejection")
	}
	assertIssueCode(t, result.Issues, CodeComponentRatingTooLow)
}

func TestSelectConcrete0603CapacitorByPackage(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query: Query{
			Family:    "capacitor",
			Package:   "0603",
			ValueKind: "capacitance",
			Value:     "100n",
		},
		Acceptance: AcceptanceConnectivity,
	})
	if !result.OK {
		t.Fatalf("select capacitor failed: %+v", result.Issues)
	}
	if selection.Component.ID != "capacitor.murata.grm188r71h104ka93d.0603" || selection.Candidate.Confidence != ConfidenceVerified {
		t.Fatalf("unexpected capacitor selection: ID=%q candidate=%+v", selection.Component.ID, selection.Candidate)
	}
}

func TestSelectRegulatorCompanionCapacitorWithRatings(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query: Query{
			Family:    "capacitor",
			Package:   "0805",
			ValueKind: "capacitance",
			Value:     "10u",
		},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{{
			Kind:  "voltage",
			Value: "5",
			Unit:  "V",
		}},
	})
	if !result.OK {
		t.Fatalf("select regulator capacitor failed: %+v", result.Issues)
	}
	if selection.Component.ID != "capacitor.murata.grm21br61a106ke19l.0805" || selection.Candidate.Confidence != ConfidenceVerified {
		t.Fatalf("unexpected capacitor selection: ID=%q candidate=%+v", selection.Component.ID, selection.Candidate)
	}
}

func TestFindConnectorByPinCountAndPackage(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	candidates, result := Find(context.Background(), catalog, Query{
		Family:    "connector",
		Package:   "1x04",
		ValueKind: "pin_count",
		Value:     "4",
	})
	if !result.OK {
		t.Fatalf("find connector failed: %+v", result.Issues)
	}
	found := map[string]bool{}
	for _, candidate := range candidates {
		found[candidate.ComponentID] = true
	}
	if len(candidates) != 2 || !found["connector.pinheader.1x04.2_54mm"] || !found["connector.samtec.tsw_104_07_l_s.1x04"] {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestFindConnectorByThreePinCount(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	candidates, result := Find(context.Background(), catalog, Query{
		Family:    "connector",
		Package:   "1x03",
		ValueKind: "pin_count",
		Value:     "3",
	})
	if !result.OK {
		t.Fatalf("find connector failed: %+v", result.Issues)
	}
	if len(candidates) != 1 || candidates[0].ComponentID != "connector.pinheader.1x03.2_54mm" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestSelectVerifiedSignalDiodeForConnectivity(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "diode", Package: "sod_123"},
		Acceptance: AcceptanceConnectivity,
	})
	if !result.OK {
		t.Fatalf("select diode failed: %+v", result.Issues)
	}
	if selection.Candidate.Confidence != ConfidenceVerified {
		t.Fatalf("expected verified diode, got %+v", selection.Candidate)
	}
	wantPads := map[string]string{"CATHODE": "1", "ANODE": "2"}
	for _, padFunction := range selection.Variant.PadFunctions {
		if want, ok := wantPads[padFunction.Function]; ok && padFunction.Pad != want {
			t.Fatalf("diode %s mapped to pad %s, want %s", padFunction.Function, padFunction.Pad, want)
		}
	}
	for function := range wantPads {
		found := false
		for _, padFunction := range selection.Variant.PadFunctions {
			if padFunction.Function == function {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("diode missing %s pad function: %+v", function, selection.Variant.PadFunctions)
		}
	}
}

func TestSelectVerifiedRegulatorForPowerRequest(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query: Query{
			Family:    "regulator",
			Package:   "sot223",
			ValueKind: "output_voltage",
			Value:     "3.3",
		},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{
			{Kind: "input_voltage", Value: "5", Unit: "V"},
			{Kind: "output_current", Value: "500", Unit: "mA"},
		},
	})
	if !result.OK {
		t.Fatalf("select regulator failed: %+v", result.Issues)
	}
	if selection.Component.ID != "regulator.linear.ams1117_3v3.sot223" || selection.Candidate.Confidence != ConfidenceVerified {
		t.Fatalf("unexpected regulator selection: %+v", selection.Candidate)
	}
	if len(selection.Component.Companions) < 2 {
		t.Fatalf("expected regulator companion requirements: %+v", selection.Component.Companions)
	}
}

func TestSelectVerifiedRegulatorRequiresCompanions(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:             Query{Family: "regulator", Package: "sot223", ValueKind: "output_voltage", Value: "3.3"},
		Acceptance:        AcceptanceConnectivity,
		RequireConcrete:   true,
		RequireCompanions: true,
		RequiredRatings: []RequiredRating{
			{Kind: "input_voltage", Value: "5", Unit: "V"},
			{Kind: "output_current", Value: "250", Unit: "mA"},
		},
	})
	if !result.OK {
		t.Fatalf("select regulator with companions failed: %+v", result.Issues)
	}
	if selection.Component.ID != "regulator.linear.ams1117_3v3.sot223" {
		t.Fatalf("unexpected regulator selection: got ID %q, want %q", selection.Component.ID, "regulator.linear.ams1117_3v3.sot223")
	}
	for _, role := range []string{"input_capacitor", "output_capacitor"} {
		if !componentHasRequiredCompanionRole(selection.Component, role) {
			t.Fatalf("selected regulator missing companion role %s: %+v", role, selection.Component.Companions)
		}
	}
}

func TestSelectAP2112KRegulatorVariant(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:             Query{Family: "regulator", Package: "sot23_5", ValueKind: "output_voltage", Value: "3.3"},
		Acceptance:        AcceptanceConnectivity,
		RequireConcrete:   true,
		RequireCompanions: true,
		RequiredRatings: []RequiredRating{
			{Kind: "input_voltage", Value: "5", Unit: "V"},
			{Kind: "output_current", Value: "100", Unit: "mA"},
			{Kind: "enable_voltage", Value: "5", Unit: "V"},
		},
		RequiredFunctions: []string{"VIN", "GND", "EN", "NC", "VOUT"},
	})
	if !result.OK {
		t.Fatalf("select AP2112K failed: %+v", result.Issues)
	}
	if selection.Component.ID != "regulator.linear.ap2112k_3v3.sot23_5" {
		t.Fatalf("unexpected regulator selection: %+v", selection.Candidate)
	}
	for _, kind := range []string{"thermal", "enable_voltage", "capacitor_stability"} {
		if !componentHasDeratingRule(selection.Component, kind) {
			t.Fatalf("selected AP2112K missing derating rule %s: %+v", kind, selection.Component.DeratingRules)
		}
	}
}

func TestSelectAP2112KRejectsInsufficientInputHeadroom(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "regulator", Package: "sot23_5", ValueKind: "output_voltage", Value: "3.3"},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{
			{Kind: "input_voltage", Value: "3.6", Unit: "V"},
			{Kind: "output_current", Value: "100", Unit: "mA"},
		},
	})
	if result.OK {
		t.Fatal("expected AP2112K low-input request to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentRatingTooLow)
}

func TestSelectAP2112KRejectsOverCurrent(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "regulator", Package: "sot23_5", ValueKind: "output_voltage", Value: "3.3"},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{
			{Kind: "input_voltage", Value: "5", Unit: "V"},
			{Kind: "output_current", Value: "700", Unit: "mA"},
		},
	})
	if result.OK {
		t.Fatal("expected AP2112K over-current request to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentRatingTooLow)
}

func TestSelectRegulatorRejectsUnsupportedOutputVoltage(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "regulator", ValueKind: "output_voltage", Value: "5"},
		Acceptance: AcceptanceConnectivity,
	})
	assertIssueCode(t, result.Issues, CodeComponentNotFound)
}

func TestSelectAP2112KBlocksFabricationCandidateReviewGaps(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:             Query{Family: "regulator", Package: "sot23_5", ValueKind: "output_voltage", Value: "3.3"},
		Acceptance:        AcceptanceFabricationCandidate,
		RequireConcrete:   true,
		RequireCompanions: true,
		RequiredRatings: []RequiredRating{
			{Kind: "input_voltage", Value: "5", Unit: "V"},
			{Kind: "output_current", Value: "100", Unit: "mA"},
		},
	})
	if result.OK {
		t.Fatal("expected AP2112K fabrication-candidate selection to block on review evidence")
	}
	assertIssueCode(t, result.Issues, CodeComponentReviewRequired)
}

func TestSelectRejectsRegulatorOverCurrent(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "regulator", Package: "sot223"},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{{
			Kind:  "output_current",
			Value: "2",
			Unit:  "A",
		}},
	})
	if result.OK {
		t.Fatal("expected regulator over-current request to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentRatingTooLow)
}

func TestSelectRejectsPlaceholderForConnectivity(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: filepath.Join("testdata", "catalog", "unsafe_placeholder")})
	if err != nil {
		t.Fatalf("load unsafe placeholder fixture: %v", err)
	}
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "opamp"},
		Acceptance: AcceptanceConnectivity,
	})
	if result.OK {
		t.Fatal("expected placeholder opamp to be rejected")
	}
	assertIssueCode(t, result.Issues, CodeComponentUnsafe)
}

func TestSelectAllowsPlaceholderForDraft(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: filepath.Join("testdata", "catalog", "unsafe_placeholder")})
	if err != nil {
		t.Fatalf("load unsafe placeholder fixture: %v", err)
	}
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "opamp"},
		Acceptance: AcceptanceDraft,
	})
	if !result.OK {
		t.Fatalf("draft placeholder select failed: %+v", result.Issues)
	}
	if selection.Candidate.Confidence != ConfidencePlaceholder {
		t.Fatalf("expected placeholder confidence, got %s", selection.Candidate.Confidence)
	}
	if len(selection.Warnings) == 0 {
		t.Fatal("expected placeholder warning")
	}
}

func TestSelectPrefersEquivalentConcreteForConnectivity(t *testing.T) {
	catalog := validCatalog()
	concrete := cloneSelectionTestRecord(catalog.Records[0])
	concrete.ID = "resistor.yageo.rc0805.10k"
	concrete.Name = "Yageo 10k 0805 resistor"
	concrete.Generic = false
	concrete.Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
	catalog.Records[0].ID = "resistor.generic.10k.0805"
	catalog.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalenceFallback}
	catalog.Records = append(catalog.Records, concrete)

	selection, result := Select(context.Background(), &catalog, SelectionRequest{
		Query:      Query{Family: "resistor", Package: "0805", ValueKind: "resistance", Value: "10k"},
		Acceptance: AcceptanceConnectivity,
	})
	if !result.OK {
		t.Fatalf("select equivalent concrete failed: %+v", result.Issues)
	}
	if selection.Component.ID != "resistor.yageo.rc0805.10k" {
		t.Fatalf("selected %s", selection.Component.ID)
	}
	if selection.Candidate.EquivalenceGroup != "resistor.10k.0805" || selection.Candidate.EquivalenceRole != EquivalencePreferred {
		t.Fatalf("missing equivalence candidate metadata: %+v", selection.Candidate)
	}
}

func TestSelectKeepsGenericFallbackForDraft(t *testing.T) {
	catalog := validCatalog()
	concrete := cloneSelectionTestRecord(catalog.Records[0])
	concrete.ID = "resistor.aaa_concrete.10k.0805"
	concrete.Name = "Yageo 10k 0805 resistor"
	concrete.Generic = false
	concrete.Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
	catalog.Records[0].ID = "resistor.generic.10k.0805"
	catalog.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalenceFallback}
	catalog.Records = append(catalog.Records, concrete)

	selection, result := Select(context.Background(), &catalog, SelectionRequest{
		Query:      Query{Family: "resistor", Package: "0805", ValueKind: "resistance", Value: "10k"},
		Acceptance: AcceptanceDraft,
	})
	if !result.OK {
		t.Fatalf("select draft fallback failed: %+v", result.Issues)
	}
	if selection.Component.ID != "resistor.generic.10k.0805" {
		t.Fatalf("selected %s", selection.Component.ID)
	}
}

func TestSelectPreservesAmbiguityForNonEquivalentTie(t *testing.T) {
	catalog := validCatalog()
	alternate := cloneSelectionTestRecord(catalog.Records[0])
	catalog.Records[0].ID = "resistor.alpha.0805"
	alternate.ID = "resistor.beta.0805"
	catalog.Records = append(catalog.Records, alternate)

	_, result := Select(context.Background(), &catalog, SelectionRequest{
		Query:      Query{Family: "resistor", Package: "0805", ValueKind: "resistance", Value: "10k"},
		Acceptance: AcceptanceConnectivity,
	})
	if result.OK {
		t.Fatal("expected non-equivalent tie to remain ambiguous")
	}
	assertIssueCode(t, result.Issues, CodeComponentAmbiguous)
}

func TestSelectVerifiedOpAmpBySupplyRange(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Text: "ti", Family: "opamp", Package: "sot23_5"},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{{
			Kind:  "supply_voltage",
			Value: "3.3",
			Unit:  "V",
		}},
	})
	if !result.OK {
		t.Fatalf("select opamp failed: %+v", result.Issues)
	}
	if selection.Component.ID != "opamp.ti.lmv321.sot23_5" {
		t.Fatalf("unexpected opamp selection: %+v", selection.Candidate)
	}
}

func cloneSelectionTestRecord(record ComponentRecord) ComponentRecord {
	clone := record
	clone.Values = append([]ValueConstraint(nil), record.Values...)
	clone.Ratings = append([]RatingConstraint(nil), record.Ratings...)
	clone.Tolerances = append([]ToleranceConstraint(nil), record.Tolerances...)
	clone.Symbols = append([]SymbolBinding(nil), record.Symbols...)
	for i := range clone.Symbols {
		clone.Symbols[i].FunctionPins = append([]FunctionPin(nil), record.Symbols[i].FunctionPins...)
	}
	clone.Packages = append([]PackageVariant(nil), record.Packages...)
	for i := range clone.Packages {
		clone.Packages[i].PadFunctions = append([]PadFunction(nil), record.Packages[i].PadFunctions...)
	}
	if record.Equivalence != nil {
		equivalence := *record.Equivalence
		equivalence.Notes = append([]string(nil), record.Equivalence.Notes...)
		clone.Equivalence = &equivalence
	}
	return clone
}

func TestSelectRejectsOpAmpOutsideSupplyRange(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Text: "ti", Family: "opamp", Package: "sot23_5"},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{{
			Kind:  "supply_voltage",
			Value: "6",
			Unit:  "V",
		}},
	})
	if result.OK {
		t.Fatal("expected opamp over-voltage request to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentRatingTooLow)
}

func TestSelectRejectsOpAmpBelowMinimumSupplyRange(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Text: "ti", Family: "opamp", Package: "sot23_5"},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{{
			Kind:  "supply_voltage",
			Value: "1.8",
			Unit:  "V",
		}},
	})
	if result.OK {
		t.Fatal("expected opamp under-voltage request to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentRatingTooLow)
}

func TestSelectWithRejectedPlaceholderAlternativeStillSucceeds(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "opamp", Package: "sot23_5"},
		Acceptance: AcceptanceConnectivity,
	})
	if !result.OK {
		t.Fatalf("expected verified opamp to win despite rejected placeholder: %+v", result.Issues)
	}
	if selection.Component.ID != "opamp.ti.lmv321.sot23_5" {
		t.Fatalf("unexpected selection: %+v", selection.Candidate)
	}
	if len(selection.Rejected) == 0 {
		t.Fatal("expected rejected placeholder diagnostics")
	}
}

func TestSelectRequiresFunction(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:             Query{Family: "usb_c", Package: "6p"},
		Acceptance:        AcceptanceConnectivity,
		RequiredFunctions: []string{"CC1", "CC2"},
	})
	if !result.OK {
		t.Fatalf("expected usb-c function selection to pass: %+v", result.Issues)
	}
	if selection.Component.ID != "usb_c.gct.usb4125_power_only_6p" {
		t.Fatalf("unexpected selection: %+v", selection.Candidate)
	}

	_, result = Select(context.Background(), catalog, SelectionRequest{
		Query:             Query{Family: "usb_c", Package: "6p"},
		Acceptance:        AcceptanceConnectivity,
		RequiredFunctions: []string{"D_PLUS"},
	})
	if result.OK {
		t.Fatal("expected missing USB data function to fail")
	}
	assertIssueCode(t, result.Issues, CodeComponentFunctionMissing)
}

func TestSelectRequiresConcreteAndCompanions(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:           Query{Family: "resistor", Package: "0805", ValueKind: "resistance", Value: "10k"},
		Acceptance:      AcceptanceConnectivity,
		RequireConcrete: true,
	})
	if !result.OK {
		t.Fatalf("expected concrete resistor to satisfy concrete requirement: %+v", result.Issues)
	}
	if selection.Component.Generic {
		t.Fatalf("expected concrete resistor, got %+v", selection.Component)
	}

	_, result = Select(context.Background(), catalog, SelectionRequest{
		Query:           Query{Family: "capacitor", Package: "0603", ValueKind: "capacitance", Value: "1u"},
		Acceptance:      AcceptanceConnectivity,
		RequireConcrete: true,
	})
	if result.OK {
		t.Fatal("expected generic-only capacitor to fail concrete requirement")
	}
	assertIssueCode(t, result.Issues, CodeComponentConcreteRequired)

	selection, result = Select(context.Background(), catalog, SelectionRequest{
		Query:             Query{Text: "ti", Family: "opamp", Package: "sot23_5"},
		Acceptance:        AcceptanceConnectivity,
		RequireConcrete:   true,
		RequireCompanions: true,
	})
	if !result.OK {
		t.Fatalf("expected concrete opamp with companions to pass: %+v", result.Issues)
	}
	if selection.Component.ID != "opamp.ti.lmv321.sot23_5" {
		t.Fatalf("unexpected selection: %+v", selection.Candidate)
	}
}

func TestSelectVerifiedMCUAndRequiredFunctions(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Text: "microchip", Family: "mcu", Package: "tqfp32"},
		Acceptance: AcceptanceConnectivity,
	})
	if !result.OK {
		t.Fatalf("select mcu failed: %+v", result.Issues)
	}
	for _, fn := range []string{"VCC", "GND", "RESET"} {
		if !componentHasFunction(selection.Component, fn) {
			t.Fatalf("selected MCU missing function %s", fn)
		}
	}
}

func TestSelectVerifiedI2CSensor(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "sensor", Package: "lga8"},
		Acceptance: AcceptanceConnectivity,
	})
	if !result.OK {
		t.Fatalf("select sensor failed: %+v", result.Issues)
	}
	for _, fn := range []string{"VDD", "GND", "SDA", "SCL"} {
		if !componentHasFunction(selection.Component, fn) {
			t.Fatalf("selected sensor missing function %s", fn)
		}
	}
}

func TestSelectVerifiedCrystal(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query: Query{
			Text:      "generic",
			Family:    "crystal",
			Package:   "5032",
			ValueKind: "frequency",
			Value:     "16",
		},
		Acceptance: AcceptanceConnectivity,
	})
	if !result.OK {
		t.Fatalf("select crystal failed: %+v", result.Issues)
	}
	if selection.Component.ID != "crystal.generic.5032_2pin" {
		t.Fatalf("unexpected crystal selection: %+v", selection.Candidate)
	}
}

func TestSelectConcreteCrystalForFabricationCandidate(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query: Query{
			Family:    "crystal",
			Package:   "5032",
			ValueKind: "frequency",
			Value:     "16",
		},
		Acceptance:        AcceptanceFabricationCandidate,
		RequireConcrete:   true,
		RequireCompanions: true,
	})
	if !result.OK {
		t.Fatalf("select concrete crystal failed: %+v", result.Issues)
	}
	if selection.Component.ID != "crystal.abracon.abm3_16mhz.5032_2pin" {
		t.Fatalf("selected component ID = %s", selection.Component.ID)
	}
	if selection.Component.Manufacturer != "Abracon" || selection.Component.MPN != "ABM3-16.000MHZ-D2Y-T" {
		t.Fatalf("selected crystal missing manufacturer evidence: %+v", selection.Component)
	}
	for _, fn := range []string{"XTAL_1", "XTAL_2"} {
		if !componentHasFunction(selection.Component, fn) {
			t.Fatalf("selected crystal missing function %s", fn)
		}
	}
}

func TestSelectVerifiedUSBCPowerOnlyConnector(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	selection, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "usb_c", Package: "6p"},
		Acceptance: AcceptanceConnectivity,
		RequiredRatings: []RequiredRating{{
			Kind:  "current",
			Value: "3",
			Unit:  "A",
		}},
	})
	if !result.OK {
		t.Fatalf("select usb-c failed: %+v", result.Issues)
	}
	if !componentHasFunction(selection.Component, "CC1") || !componentHasFunction(selection.Component, "CC2") {
		t.Fatalf("selected USB-C record missing CC pins")
	}
}

func TestSelectReportsAmbiguousEqualCandidates(t *testing.T) {
	catalog := &Catalog{
		Families: []FamilyDefinition{{ID: "resistor", Name: "Resistor"}},
		Records: []ComponentRecord{
			testSelectableResistor("resistor.a"),
			testSelectableResistor("resistor.b"),
		},
	}
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "resistor", Package: "0805"},
		Acceptance: AcceptanceConnectivity,
	})
	if result.OK {
		t.Fatal("expected ambiguous selection")
	}
	assertIssueCode(t, result.Issues, CodeComponentAmbiguous)
}

func TestResolveBinding(t *testing.T) {
	catalog := loadCheckedInCatalog(t)
	resolved, result := ResolveBinding(context.Background(), catalog, "resistor.generic.0805", "0805")
	if !result.OK {
		t.Fatalf("resolve binding failed: %+v", result.Issues)
	}
	if resolved.Symbol.SymbolID != "Device:R" || resolved.Variant.FootprintID == "" {
		t.Fatalf("unexpected resolved binding: %+v", resolved)
	}
}

func loadCheckedInCatalog(t *testing.T) *Catalog {
	t.Helper()
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}
	return catalog
}

func componentHasFunction(record ComponentRecord, function string) bool {
	for _, symbol := range record.Symbols {
		for _, pin := range symbol.FunctionPins {
			if pin.Function == function {
				return true
			}
		}
	}
	return false
}

func componentHasRequiredCompanionRole(record ComponentRecord, role string) bool {
	for _, companion := range record.Companions {
		if companion.Role == role && companion.Required {
			return true
		}
	}
	return false
}

func componentHasDeratingRule(record ComponentRecord, kind string) bool {
	for _, rule := range record.DeratingRules {
		if rule.Kind == kind {
			return true
		}
	}
	return false
}

func sourceCollectionFor(records ...SourceRecord) *SourceCollection {
	collection := &SourceCollection{Records: records}
	collection.rebuildIndex()
	return collection
}

func testSelectableResistor(id string) ComponentRecord {
	return ComponentRecord{
		ID:      id,
		Family:  "resistor",
		Name:    id,
		Generic: true,
		Symbols: []SymbolBinding{{
			SymbolID:     "Device:R",
			Verification: VerificationRecord{Confidence: ConfidenceRuleInferred},
		}},
		Packages: []PackageVariant{{
			ID:           "0805",
			FootprintID:  "Resistor_SMD:R_0805_2012Metric",
			Verification: VerificationRecord{Confidence: ConfidenceRuleInferred},
		}},
		Verification: VerificationRecord{Confidence: ConfidenceRuleInferred},
	}
}
