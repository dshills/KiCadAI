package components

import (
	"context"
	"testing"

	"kicadai/internal/reports"
)

func TestValueAndToleranceMatchingContinuesAcrossSameKindConstraints(t *testing.T) {
	record := ComponentRecord{
		Values: []ValueConstraint{
			{Kind: "capacitance", Typ: "malformed", Unit: "F"},
			{Kind: "capacitance", Typ: "100n", Unit: "F"},
		},
		Tolerances: []ToleranceConstraint{
			{Kind: "capacitance", Max: "malformed", Unit: "%"},
			{Kind: "capacitance", Max: "5", Unit: "%"},
		},
	}
	if !recordHasValue(record, "capacitance", "0.1u") {
		t.Fatal("a malformed first value constraint masked a later canonical match")
	}
	if !recordMeetsTolerance(record, Query{ValueKind: "capacitance", MaximumTolerance: 10, ToleranceUnit: "%"}) {
		t.Fatal("a malformed first tolerance constraint masked a later satisfying constraint")
	}
}

func TestMinOnlyRatingRepresentsGuaranteedMinimumCapability(t *testing.T) {
	record := ComponentRecord{Ratings: []RatingConstraint{{
		Kind: "supply_current_minimum",
		Min:  "0.5",
		Unit: "A",
	}}}
	if match := recordSatisfiesRating(record, RequiredRating{Kind: "supply_current_minimum", Value: "450", Unit: "mA"}); !match.requestValid || !match.found || !match.satisfied {
		t.Fatalf("min-only rating did not satisfy a lower request: %#v", match)
	}
	if match := recordSatisfiesRating(record, RequiredRating{Kind: "supply_current_minimum", Value: "600", Unit: "mA"}); !match.requestValid || !match.found || match.satisfied {
		t.Fatalf("min-only rating satisfied a request above its guarantee: %#v", match)
	}
}

func TestInvalidRequestedRatingIsNotReportedAsInsufficient(t *testing.T) {
	record := ComponentRecord{ID: "test", Ratings: []RatingConstraint{{Kind: "voltage", Max: "10", Unit: "V"}}}
	issues := requiredRatingIssues(record, []RequiredRating{{Kind: "voltage", Value: "5VX", Unit: "V"}})
	if len(issues) != 1 || issues[0].Code != reports.CodeInvalidArgument {
		t.Fatalf("invalid requested rating issues = %#v", issues)
	}
}

func TestSelectAmplifierOutputPairSearchesCompatiblePairsJointly(t *testing.T) {
	source := loadCheckedInCatalog(t)
	npn := *requireCatalogRecord(t, source, "bjt.onsemi.mmbt3904.sot23")
	pnp := *requireCatalogRecord(t, source, "bjt.onsemi.mmbt3906.sot23")
	makeRecord := func(base ComponentRecord, id string, group string) ComponentRecord {
		base.ID = id
		evidence := *base.AmplifierOutput
		evidence.ComplementaryGroup = group
		base.AmplifierOutput = &evidence
		if base.PowerSemiconductor != nil {
			power := *base.PowerSemiconductor
			power.ComplementaryGroup = group
			base.PowerSemiconductor = &power
		}
		return base
	}
	catalog := &Catalog{Records: []ComponentRecord{
		makeRecord(npn, "bjt.test.a_upper", "alpha"),
		makeRecord(npn, "bjt.test.b_upper", "beta"),
		makeRecord(pnp, "bjt.test.a_lower", "beta"),
		makeRecord(pnp, "bjt.test.b_lower", "alpha"),
	}}
	RebuildCatalogIndexes(catalog)

	pair, result := SelectAmplifierOutputPair(context.Background(), catalog, AmplifierOutputPairRequest{
		SupplyVoltage: "9",
		LoadImpedance: "32",
		Acceptance:    AcceptanceConnectivity,
	})
	if !result.OK {
		t.Fatalf("joint pair selection failed: %#v", result.Issues)
	}
	if pair.Upper.Component.ID != "bjt.test.a_upper" || pair.Lower.Component.ID != "bjt.test.b_lower" {
		t.Fatalf("joint pair selection = %s / %s", pair.Upper.Component.ID, pair.Lower.Component.ID)
	}
}

func TestValidateComplementaryGroupsRejectsOneSidedMetadata(t *testing.T) {
	issues := validateComplementaryGroups(map[string][]complementaryMember{
		"orphan": {{path: "records[0].power_semiconductor_evidence.complementary_group", recordID: "only-npn", deviceClass: "bjt", polarity: "npn"}},
	})
	if len(issues) != 1 || issues[0].Code != CodeInvalidMetadata {
		t.Fatalf("one-sided complementary group issues = %#v", issues)
	}
}
