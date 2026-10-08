package boardfamily

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func eligibleRaw(t testing.TB, requirements []map[string]any, quantities map[string][]map[string]any) []byte {
	t.Helper()
	// Synthetic fixtures only; this helper never reads recorded model output.
	return bytes.Replace(groundedRaw(t, requirements, quantities), []byte(GroundedEvidenceVersion), []byte(SourceEligibleVersion), 1)
}

func eligibleFeature(value, state, anchor string, evidence ...string) map[string]any {
	f := ownedFact("feature", value, state, evidence...)
	f["anchor"] = anchor
	return f
}

func checkEligible(t testing.TB, prompt string, raw []byte, reject bool) {
	t.Helper()
	schema, err := SourceEligibleEvidenceSchema(prompt)
	if err != nil {
		t.Fatal(err)
	}
	before := bytes.Clone(raw)
	schemaErr := checkGroundedSchema(t, schema, raw)
	_, decodeErr := CompileSourceEligibleEvidence(prompt, raw)
	if reject && (schemaErr == nil || decodeErr == nil) {
		t.Fatalf("guard failed: schema=%v decoder=%v", schemaErr, decodeErr)
	}
	if !reject && (schemaErr != nil || decodeErr != nil) {
		t.Fatalf("valid representation rejected: schema=%v decoder=%v", schemaErr, decodeErr)
	}
	if !bytes.Equal(before, raw) {
		t.Fatal("raw extraction was changed")
	}
}

func TestSourceEligibleEveryFeatureHasAnExplicitMentionRule(t *testing.T) {
	phrases := map[string]string{
		"accuracy_guarantee": "Guarantee assembled-board accuracy.", "battery_operation": "Use a lithium battery.",
		"certification": "Require FCC certification.", "custom_geometry": "Change the board outline.",
		"delivered_firmware": "Deliver firmware source code.", "different_sensor_address": "Change its I2C address.",
		"external_gpio_load": "Drive this GPIO load.", "extra_peripheral": "Add a relay.",
		"heater_operation": "Enable the heater.", "internal_pullups": "Use internal pull-ups.",
		"protection_circuitry": "Add ESD protection.", "regulator": "Include a regulator.",
		"sensor_coating_or_wash": "Wash the sensor.", "skip_crc": "Skip CRC checking.",
		"usb": "Power it from USB.", "whole_board_low_power": "Reduce whole-board power.",
	}
	for feature := range intentFeatureReasons {
		if feature == "wireless_operation" {
			continue // Wireless retains the separate connection representation.
		}
		if phrases[feature] == "" || eligibleFeaturePatterns[feature] == nil {
			t.Fatal("missing feature rule", feature)
		}
	}
	if len(phrases) != len(eligibleFeaturePatterns) {
		t.Fatal("unreviewed extra feature rule")
	}
	for feature, phrase := range phrases {
		t.Run(feature, func(t *testing.T) {
			for _, state := range []string{"requested", "unnecessary_but_allowed", "must_not_occur", "unresolved_choice"} {
				raw := eligibleRaw(t, []map[string]any{eligibleFeature(feature, state, "c0", "c0")}, nil)
				checkEligible(t, phrase, raw, false)
				checkEligible(t, "Please make a sensor board.", raw, true)
			}
		})
	}
}

func TestSourceEligibleRejectsInventedFeaturesFromPowerAndAccuracy(t *testing.T) {
	for _, tc := range []struct {
		prompt   string
		features []string
	}{
		{"Power a BMP280 board directly from USB without an external adapter.", []string{"delivered_firmware", "extra_peripheral", "external_gpio_load"}},
		{"Guarantee assembled-board accuracy without calibration or bench characterization.", []string{"certification", "delivered_firmware", "custom_geometry", "battery_operation"}},
	} {
		for _, feature := range tc.features {
			t.Run(feature+tc.prompt, func(t *testing.T) {
				checkEligible(t, tc.prompt, eligibleRaw(t, []map[string]any{eligibleFeature(feature, "requested", "c0", "c0")}, nil), true)
			})
		}
	}
}

func TestSourceEligibleAnchorMustBeEligibleAndCited(t *testing.T) {
	prompt := "Use USB power. Do not include firmware."
	checkEligible(t, prompt, eligibleRaw(t, []map[string]any{eligibleFeature("delivered_firmware", "must_not_occur", "c1", "c1")}, nil), false)
	checkEligible(t, prompt, eligibleRaw(t, []map[string]any{eligibleFeature("delivered_firmware", "requested", "c0", "c0")}, nil), true)
	// A legal anchor with a disjoint evidence list is a decoder-level invariant.
	raw := eligibleRaw(t, []map[string]any{eligibleFeature("delivered_firmware", "requested", "c1", "c0")}, nil)
	if d, err := DecodeSourceEligibleEvidenceIntent(prompt, raw); err == nil || d.Configuration != nil {
		t.Fatal("uncited anchor was laundered into evidence")
	}
	checkEligible(t, "Do not use firmwareless hardware.", eligibleRaw(t, []map[string]any{eligibleFeature("delivered_firmware", "must_not_occur", "c0", "c0")}, nil), true)
}

func TestSourceEligiblePrecisionIsNotAnOperatingQuantity(t *testing.T) {
	for _, prompt := range []string{
		"Accuracy within 0.2 C.", "Temperature accuracy: ±0.2 degrees C.", "Precision no worse than 0.2 C.",
		"Resolution of 0.2 C.", "Require 0.2 C repeatability.", "Offset = 0.2 C.", "Clock tolerance within 10 Hz.",
		"Accuracy within +/-0.2 C.", "Accuracy: +0.2 C.", "Accuracy: -0.2 C.",
	} {
		t.Run(prompt, func(t *testing.T) {
			source, _, err := eligibleSource(prompt)
			if err != nil || len(source.Quantities) != 1 {
				t.Fatal("source", source, err)
			}
			for field := range source.Quantities[0].Fields {
				checkEligible(t, prompt, eligibleRaw(t, nil, map[string][]map[string]any{"q0": {groundedNumber(field, "requested")}}), true)
			}
			other := map[string]any{"kind": "other", "detail": prompt, "state": "requested", "context": []string{"c0"}}
			checkEligible(t, prompt, eligibleRaw(t, nil, map[string][]map[string]any{"q0": {other}}), false)
		})
	}
}

func TestSourceEligibleQuantityMeaningIsLocalToExactOccurrence(t *testing.T) {
	for _, prompt := range []string{
		"Operate at 10 to 35 C with accuracy within 0.2 C.",
		"Accuracy within 0.2 C while operating at 10 to 35 C.",
		"Operate at 0.2 C with accuracy within 0.2 C.",
		"Température: operate at 10 to 35 C with accuracy within 0.2 C.",
		"No accuracy guarantee is requested, use an ambient range of 10 to 35 C.",
	} {
		t.Run(prompt, func(t *testing.T) {
			source, eligibility, err := eligibleSource(prompt)
			if err != nil {
				t.Fatal(err)
			}
			operating, precision := 0, 0
			for _, roles := range eligibility.QuantityRoles {
				if len(roles) == 0 {
					precision++
				} else {
					operating++
				}
			}
			wantPrecision := len(source.Quantities) - 1
			if operating != 1 || precision != wantPrecision {
				t.Fatalf("operating=%d precision=%d source=%+v", operating, precision, source)
			}
		})
	}
}

func TestSourceEligiblePreservesUnknownRequirementsAndStateDistinctions(t *testing.T) {
	prompt := "Use BMP280 standard. No external adapter may be added."
	facts := []map[string]any{ownedFact("sensor", "BMP280", "requested", "c0"), ownedFact("profile", "standard", "requested", "c0"),
		{"kind": "other", "detail": "No external adapter may be added.", "state": "requested", "evidence": []string{"c1"}}}
	raw := eligibleRaw(t, facts, nil)
	checkEligible(t, prompt, raw, false)
	d, err := DecodeSourceEligibleEvidenceIntent(prompt, raw)
	if err != nil || d.Disposition != "unsupported" || !strings.Contains(d.Message, "external adapter") {
		t.Fatal("unknown requirement was lost", d, err)
	}
	for _, state := range []string{"unnecessary_but_allowed", "must_not_occur"} {
		p := "Use SHT31 standard. Heater operation is not needed."
		r := eligibleRaw(t, []map[string]any{eligibleFeature("heater_operation", state, "c1", "c1")}, nil)
		compiled, err := CompileSourceEligibleEvidence(p, r)
		if err != nil || !bytes.Contains(compiled, []byte(groundedStates[state])) {
			t.Fatal("state rewritten", err)
		}
	}
}

func TestSourceEligibleDoesNotPretendLexicalSupportProvesMeaning(t *testing.T) {
	// Deliberately semantically WRONG: these guards cannot prove negation scope.
	// Preserve this counterexample so synthetic schema tests are not called AI
	// accuracy or a proof that every hallucination has been eliminated.
	prompt := "Never enable the heater."
	checkEligible(t, prompt, eligibleRaw(t, []map[string]any{eligibleFeature("heater_operation", "requested", "c0", "c0")}, nil), false)
}

func TestSourceEligibleDenseQuantityInventoryAndInvalidPrompts(t *testing.T) {
	prompt := "Use BMP280 standard with total bus capacitance " + strings.Repeat("100 pF, ", 128)
	quantities := map[string][]map[string]any{}
	for i := range 128 {
		quantities[fmt.Sprintf("q%d", i)] = []map[string]any{groundedNumber("total_bus_capacitance_pf", "requested")}
	}
	raw := eligibleRaw(t, []map[string]any{ownedFact("sensor", "BMP280", "requested", "c0"), ownedFact("profile", "standard", "requested", "c0")}, quantities)
	checkEligible(t, prompt, raw, false)
	if d, err := DecodeSourceEligibleEvidenceIntent(prompt, raw); err != nil || d.Configuration == nil || d.Configuration.TotalBusCapacitancePF != 100 {
		t.Fatal("dense source inventory lost", d, err)
	}
	for _, prompt := range []string{"", strings.Repeat("x", 2001), string([]byte{0xff}), strings.Repeat("Hello. ", 33)} {
		if _, err := SourceEligibleEvidenceSchema(prompt); err == nil {
			t.Fatal("invalid prompt admitted by schema")
		}
		if _, err := SourceEligibleEvidenceContract(prompt); err == nil {
			t.Fatal("invalid prompt admitted by contract")
		}
		if _, err := CompileSourceEligibleEvidence(prompt, eligibleRaw(t, nil, nil)); err == nil {
			t.Fatal("invalid prompt admitted by compiler")
		}
	}
}

func TestSourceEligibleEnvelopeAndHistoricalSeparation(t *testing.T) {
	prompt := "Use BMP280 standard."
	facts := []map[string]any{ownedFact("sensor", "BMP280", "requested", "c0"), ownedFact("profile", "standard", "requested", "c0")}
	old, current := groundedRaw(t, facts, nil), eligibleRaw(t, facts, nil)
	if _, err := CompileSourceEligibleEvidence(prompt, old); err == nil {
		t.Fatal("v7 accepted as v8")
	}
	if _, err := CompileGroundedEvidence(prompt, current); err == nil {
		t.Fatal("v8 accepted as v7")
	}
	for _, raw := range [][]byte{[]byte(`null`), []byte(`{}`), append(bytes.Clone(current), 'x'), bytes.Repeat([]byte("x"), 65537),
		bytes.Replace(current, []byte(`"requirements":`), []byte(`"extra":true,"requirements":`), 1),
		bytes.Replace(current, []byte(`"quantities":{}`), []byte(`"quantities":null`), 1)} {
		before := bytes.Clone(raw)
		if d, err := DecodeSourceEligibleEvidenceIntent(prompt, raw); err == nil || d.Configuration != nil {
			t.Fatal("invalid envelope admitted")
		}
		if !bytes.Equal(before, raw) {
			t.Fatal("invalid bytes repaired")
		}
	}
	before, err := GroundedEvidenceContract(prompt)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := SourceEligibleEvidenceContract(prompt)
	if err != nil || contract["live_authorization_granted"] != false || contract["admission_version"] != SourceEligibleVersion {
		t.Fatal(contract, err)
	}
	after, err := GroundedEvidenceContract(prompt)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("v7 contract changed", err)
	}
}

func FuzzSourceEligibleRejectsWithoutMutation(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add(eligibleRaw(f, []map[string]any{ownedFact("sensor", "BMP280", "requested", "c0")}, nil))
	f.Fuzz(func(t *testing.T, raw []byte) {
		before := bytes.Clone(raw)
		d, err := DecodeSourceEligibleEvidenceIntent("Use BMP280 standard.", raw)
		if err != nil && d.Configuration != nil {
			t.Fatal("error with configuration")
		}
		if !bytes.Equal(before, raw) {
			t.Fatal("raw bytes mutated")
		}
	})
}
