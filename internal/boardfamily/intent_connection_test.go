package boardfamily

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func connectionRaw(t testing.TB, facts ...map[string]any) []byte {
	t.Helper()
	if facts == nil {
		facts = []map[string]any{}
	}
	raw, err := json.Marshal(map[string]any{"version": ConnectionEvidenceVersion, "facts": facts})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestConnectionHistoricalExpressivenessGap(t *testing.T) {
	prompt := "Use BMP280 standard with a wired connection."
	base := []map[string]any{ownedFact("sensor", "BMP280", "required", "c0"), ownedFact("profile", "standard", "required", "c0")}
	faithfulOther := append(append([]map[string]any{}, base...), map[string]any{"kind": "other", "state": "required", "detail": "wired connection", "evidence": []string{"c0"}})
	d, err := DecodeOwnedEvidenceIntent(prompt, ownedRaw(t, faithfulOther...))
	if err != nil || d.Disposition != "unsupported" {
		t.Fatalf("historical required-other behavior changed: %s %v", d.Disposition, err)
	}
	oldRaw := ownedRaw(t, append(base, ownedFact("feature", "wireless_operation", "required", "c0"))...)
	d, err = DecodeOwnedEvidenceIntent(prompt, oldRaw)
	if err != nil || d.Disposition != "unsupported" {
		t.Fatalf("historical false wireless refusal changed: %s %v", d.Disposition, err)
	}
	if _, err := DecodeConnectionEvidenceIntent(prompt, oldRaw); err == nil {
		t.Fatal("historical response must never be migrated to v5")
	}
}

func TestConnectionSyntheticPositiveWording(t *testing.T) {
	// These are ideal, explicitly synthetic facts, not new provider responses.
	for _, wording := range []string{"wired", "cable-connected", "tethered", "non-radio", "connected by a cable"} {
		for _, polite := range []string{"Use", "Could you use", "Please use"} {
			prompt := polite + " BMP280 standard, " + wording + "."
			t.Run(prompt, func(t *testing.T) {
				connection := ownedFact("connection", "wired", "required", "c0")
				if wording == "non-radio" {
					connection = ownedFact("connection", "wireless", "forbidden", "c0")
				}
				raw := connectionRaw(t, ownedFact("sensor", "BMP280", "required", "c0"), ownedFact("profile", "standard", "required", "c0"), connection)
				before := bytes.Clone(raw)
				schema, err := ConnectionEvidenceSchema(prompt)
				if err != nil {
					t.Fatal(err)
				}
				if err := checkReferencedSchemaJSON(schema, raw); err != nil {
					t.Fatal(err)
				}
				d, err := DecodeConnectionEvidenceIntent(prompt, raw)
				if err != nil || d.Disposition != "supported" {
					t.Fatalf("%s %v", d.Disposition, err)
				}
				if !bytes.Equal(raw, before) {
					t.Fatal("caller raw bytes changed")
				}
			})
		}
	}
}

func TestConnectionStateAndConstraintPreservation(t *testing.T) {
	for _, tc := range []struct {
		name, prompt, expected string
		facts                  []map[string]any
	}{
		{"radio-required", "Use BMP280 standard with wireless telemetry.", "unsupported", []map[string]any{ownedFact("connection", "wireless", "required", "c0")}},
		{"radio-not-needed", "Use BMP280 standard; wireless is not needed.", "supported", []map[string]any{ownedFact("connection", "wireless", "not_required", "c0")}},
		{"radio-forbidden", "Use BMP280 standard; do not enable wireless.", "supported", []map[string]any{ownedFact("connection", "wireless", "forbidden", "c0")}},
		{"wired-forbidden", "Use BMP280 standard; wired operation is forbidden.", "unsupported", []map[string]any{ownedFact("connection", "wired", "forbidden", "c0")}},
		{"wired-not-needed", "Use BMP280 standard; wired operation is not a requirement.", "supported", []map[string]any{ownedFact("connection", "wired", "not_required", "c0")}},
		{"unresolved", "Use BMP280 standard; I have not chosen wired or wireless.", "clarify", []map[string]any{ownedFact("connection", "wired", "uncertain", "c0"), ownedFact("connection", "wireless", "uncertain", "c0")}},
		{"wire-does-not-cancel-radio", "Use BMP280 standard; I require both wired and wireless.", "unsupported", []map[string]any{ownedFact("connection", "wired", "required", "c0"), ownedFact("connection", "wireless", "required", "c0")}},
		{"exclusion-does-not-cancel-requirement", "Use BMP280 standard; disable wireless initially but require it later.", "unsupported", []map[string]any{ownedFact("connection", "wireless", "forbidden", "c0"), ownedFact("connection", "wireless", "required", "c0")}},
		{"unknown-connector", "Use BMP280 standard with wired Ethernet.", "unsupported", []map[string]any{ownedFact("connection", "wired", "required", "c0"), {"kind": "other", "detail": "Ethernet interface", "state": "required", "evidence": []string{"c0"}}}},
		{"omitted-quantity", "Use BMP280 standard with wired operation and a 50 mA load.", "clarify", []map[string]any{ownedFact("connection", "wired", "required", "c0")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := append([]map[string]any{ownedFact("sensor", "BMP280", "required", "c0"), ownedFact("profile", "standard", "required", "c0")}, tc.facts...)
			raw := connectionRaw(t, facts...)
			schema, err := ConnectionEvidenceSchema(tc.prompt)
			if err != nil {
				t.Fatal(err)
			}
			if err := checkReferencedSchemaJSON(schema, raw); err != nil {
				t.Fatal(err)
			}
			d, err := DecodeConnectionEvidenceIntent(tc.prompt, raw)
			if err != nil || d.Disposition != tc.expected {
				t.Fatalf("got %s, expected %s: %v (%s)", d.Disposition, tc.expected, err, d.Message)
			}
			if tc.name == "unresolved" && !strings.Contains(d.Message, "connection mode") {
				t.Fatal("question does not resolve the actual choice")
			}
		})
	}
}

func TestConnectionRejectsInvalidFacts(t *testing.T) {
	prompt := "Use BMP280 standard with a wired connection at 100 kHz."
	for _, bad := range []map[string]any{
		ownedFact("connection", "wifi", "required", "c0"),
		ownedFact("connection", "wired", "none", "c0"),
		ownedFact("connection", "wired", "required", "q0"),
		ownedFact("connection", "wired", "required", "c99"),
		ownedFact("connection", "wired", "required"),
		ownedFact("feature", "wireless_operation", "required", "c0"),
		{"kind": "connection", "value": "wired", "state": "required", "evidence": []string{"c0"}, "detail": "hidden requirement"},
	} {
		raw := connectionRaw(t, bad)
		if _, err := DecodeConnectionEvidenceIntent(prompt, raw); err == nil {
			t.Fatalf("accepted invalid fact %s", raw)
		}
		schema, err := ConnectionEvidenceSchema(prompt)
		if err != nil {
			t.Fatal(err)
		}
		if err := checkReferencedSchemaJSON(schema, raw); err == nil {
			t.Fatalf("schema accepted invalid fact %s", raw)
		}
	}
}

func TestConnectionSchemaDoesNotChangeOwnedV4(t *testing.T) {
	prompt := "Use BMP280 standard with a wired connection."
	before, err := OwnedEvidenceSchema(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ConnectionEvidenceSchema(prompt); err != nil {
		t.Fatal(err)
	}
	after, err := OwnedEvidenceSchema(prompt)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("v4 schema changed")
	}
	if _, err := DecodeOwnedEvidenceIntent(prompt, connectionRaw(t)); err == nil {
		t.Fatal("v4 accepted v5 bytes")
	}
}

func TestConnectionPreservesFiveQualifiedConfigurations(t *testing.T) {
	for _, pair := range [][2]string{{"BMP280", "standard"}, {"BMP280", "fast"}, {"BMP280", "low_current"}, {"SHT31", "standard"}, {"SHT31", "fast"}} {
		t.Run(pair[0]+"/"+pair[1], func(t *testing.T) {
			prompt := "Use " + pair[0] + " with the " + pair[1] + " profile and a wired connection."
			facts := []map[string]any{ownedFact("sensor", pair[0], "required", "c0"), ownedFact("profile", pair[1], "required", "c0")}
			baseline, err := DecodeOwnedEvidenceIntent(prompt, ownedRaw(t, facts...))
			if err != nil || baseline.Disposition != "supported" {
				t.Fatalf("invalid baseline: %+v %v", baseline, err)
			}
			d, err := DecodeConnectionEvidenceIntent(prompt, connectionRaw(t, append(facts, ownedFact("connection", "wired", "required", "c0"))...))
			if err != nil || !reflect.DeepEqual(d, baseline) {
				t.Fatalf("qualified configuration or constraints changed: %+v vs %+v (%v)", d, baseline, err)
			}
		})
	}
}

func TestConnectionStillRequiresSemanticEvaluation(t *testing.T) {
	prompt := "Use BMP280 standard with a wired connection."
	// Deliberately wrong SYNTHETIC interpretation. Schema/decoder still cannot
	// prove that a valid clause citation supports the selected connection mode.
	raw := connectionRaw(t, ownedFact("sensor", "BMP280", "required", "c0"), ownedFact("profile", "standard", "required", "c0"), ownedFact("connection", "wireless", "required", "c0"))
	schema, err := ConnectionEvidenceSchema(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkReferencedSchemaJSON(schema, raw); err != nil {
		t.Fatal(err)
	}
	d, err := DecodeConnectionEvidenceIntent(prompt, raw)
	if err != nil || d.Disposition != "unsupported" {
		t.Fatalf("expected known remaining semantic weakness, got %s %v", d.Disposition, err)
	}
}

func FuzzConnectionEvidenceDoesNotPanic(f *testing.F) {
	f.Add("Use BMP280 standard with a wired connection.", string(connectionRaw(f, ownedFact("connection", "wired", "required", "c0"))))
	f.Add("Use SHT31 with 100 pF.", string(connectionRaw(f)))
	f.Fuzz(func(t *testing.T, prompt, raw string) {
		if len(prompt) > 2500 || len(raw) > 70000 {
			t.Skip()
		}
		before := []byte(raw)
		input := bytes.Clone(before)
		_, _ = DecodeConnectionEvidenceIntent(prompt, input)
		if !bytes.Equal(input, before) {
			t.Fatal("raw mutated")
		}
	})
}
