package boardfamily

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

// This explicitly selected experimental candidate leaves v7, its recorded
// responses, and its admission rules unchanged. Integration grants no API spend.
const SourceEligibleVersion = "8-source-eligible-evidence-experimental"
const SourceEligibleSchemaName = "board_family_source_eligible_requirements_v8"

// Mentions are a necessary lexical condition, never proof of requirement state,
// subject, temporal scope, or completeness. Unknown wording remains representable
// as other/unclear; it must not be silently discarded or coerced to these labels.
var eligibleFeaturePatterns = map[string]*regexp.Regexp{
	"accuracy_guarantee":       regexp.MustCompile(`(?i)\b(accuracy|accurate|precision|tolerance|calibrat\w*)\b`),
	"battery_operation":        regexp.MustCompile(`(?i)\b(batter(y|ies)|lithium|li[- ]?ion|cell[- ]powered)\b`),
	"certification":            regexp.MustCompile(`(?i)\b(certif\w*|regulatory|compliance|fcc|ce)\b`),
	"custom_geometry":          regexp.MustCompile(`(?i)\b(geometry|outline|thickness|layers?|layout|placement|routing|dimensions?)\b`),
	"delivered_firmware":       regexp.MustCompile(`(?i)\b(firmware|software|code|program|programming|flashing)\b`),
	"different_sensor_address": regexp.MustCompile(`(?i)\b(address\w*|0x[0-9a-f]+)\b`),
	"external_gpio_load":       regexp.MustCompile(`(?i)\b(gpio|pin[- ]load|sink[- ]current)\b`),
	"extra_peripheral":         regexp.MustCompile(`(?i)\b(peripheral\w*|motors?|relays?|buzzers?|displays?|leds?|analog\s+outputs?)\b`),
	"heater_operation":         regexp.MustCompile(`(?i)\b(heaters?|heating)\b`),
	"internal_pullups":         regexp.MustCompile(`(?i)\b(internal|on[- ]chip)\s+pull[- _]?ups?\b|\bpull[- _]?ups?\s+(are\s+)?internal\b`),
	"protection_circuitry":     regexp.MustCompile(`(?i)\b(protection|surge|reverse[- ]polarity|esd|hot[- ]plug)\b`),
	"regulator":                regexp.MustCompile(`(?i)\b(regulator\w*|regulation|regulated)\b`),
	"sensor_coating_or_wash":   regexp.MustCompile(`(?i)\b(coat\w*|wash\w*|contaminat\w*)\b`),
	"skip_crc":                 regexp.MustCompile(`(?i)\b(crc|checksum|error[- ]checking)\b`),
	"usb":                      regexp.MustCompile(`(?i)\busb\b|\buniversal\s+serial\s+bus\b`),
	"whole_board_low_power":    regexp.MustCompile(`(?i)\bwhole[- _]board\s+(low\s+)?(power|current)\b|\b(low[- ]power|power\s+consumption|battery\s+life)\b`),
}

// The numeric scanner owns a leading sign: in "+/-0.2 C", the prefix ends
// in "+/" and the exact quantity starts at "-0.2". Do not lose that qualifier.
var precisionBeforeQuantity = regexp.MustCompile(`(?i)\b(accuracy|precision|resolution|tolerance|repeatability|error|offset|drift)\s*(?:(?:of|within|to|is|at\s+most|up\s+to|better\s+than|no\s+worse\s+than|[:=±]|\+/-?)\s*)*$`)
var precisionAfterQuantity = regexp.MustCompile(`(?i)^\s*(?:of\s+)?(accuracy|precision|resolution|tolerance|repeatability|error|offset|drift)\b`)

type sourceEligibility struct {
	FeatureAnchors map[string][]string `json:"feature_anchors"`
	QuantityRoles  map[string][]string `json:"quantity_roles"`
}

func eligibleSource(prompt string) (ReferencedRequest, sourceEligibility, error) {
	source, err := PrepareReferencedRequest(prompt)
	e := sourceEligibility{FeatureAnchors: map[string][]string{}, QuantityRoles: map[string][]string{}}
	if err != nil {
		return source, e, err
	}
	for name, pattern := range eligibleFeaturePatterns {
		for _, c := range source.Clauses {
			if pattern.MatchString(c.Text) {
				e.FeatureAnchors[name] = append(e.FeatureAnchors[name], "c"+strconv.Itoa(c.ID))
			}
		}
	}
	for _, q := range source.Quantities {
		roles := []string{}
		if !quantityIsExplicitPrecision(source, q) {
			for field := range q.Fields {
				roles = append(roles, field)
			}
		}
		sort.Strings(roles)
		e.QuantityRoles["q"+strconv.Itoa(q.ID)] = roles
	}
	return source, e, nil
}

// Restrict only directly attached precision wording, not an entire clause.
// This keeps an operating range and a separate tolerance in the same clause
// distinct, and uses application-owned UTF-8 byte offsets rather than searching
// for a repeated numeric string. Unknown meanings remain the model's task.
func quantityIsExplicitPrecision(source ReferencedRequest, q SourceQuantity) bool {
	offset := 0
	for _, c := range source.Clauses {
		if c.ID == q.ClauseID {
			start, end := q.Start-offset, q.End-offset
			return precisionBeforeQuantity.MatchString(c.Text[:start]) || precisionAfterQuantity.MatchString(c.Text[end:])
		}
		offset += len(c.Text)
	}
	return false
}

// SourceEligibleEvidenceSchema is key-free and does not authorize dispatch.
func SourceEligibleEvidenceSchema(prompt string) (map[string]any, error) {
	_, eligibility, err := eligibleSource(prompt)
	if err != nil {
		return nil, err
	}
	schema, err := GroundedEvidenceSchema(prompt)
	if err != nil {
		return nil, err
	}
	props := schema["properties"].(map[string]any)
	props["version"] = enumSchema(SourceEligibleVersion)
	requirements := props["requirements"].(map[string]any)
	variants := requirements["items"].(map[string]any)["anyOf"].([]any)
	kept := make([]any, 0, len(variants)+len(eligibility.FeatureAnchors))
	for _, variant := range variants {
		p := variant.(map[string]any)["properties"].(map[string]any)
		if p["kind"].(map[string]any)["enum"].([]string)[0] != "feature" {
			kept = append(kept, variant)
		}
	}
	names := make([]string, 0, len(eligibility.FeatureAnchors))
	for name := range eligibility.FeatureAnchors {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		kept = append(kept, objectSchema(map[string]any{
			"kind": enumSchema("feature"), "value": enumSchema(name),
			"anchor": enumSchema(eligibility.FeatureAnchors[name]...),
			"state":  map[string]any{"$ref": "#/$defs/state"}, "evidence": map[string]any{"$ref": "#/$defs/evidence"},
		}))
	}
	requirements["items"] = map[string]any{"anyOf": kept}
	quantities := props["quantities"].(map[string]any)["properties"].(map[string]any)
	for id, roles := range eligibility.QuantityRoles {
		if len(roles) == 0 {
			var choices []any
			for _, kind := range []string{"other", "unclear"} {
				choices = append(choices, map[string]any{"type": "array", "minItems": 1, "maxItems": 1, "items": map[string]any{"$ref": "#/$defs/quantity_" + kind}})
			}
			quantities[id] = map[string]any{"anyOf": choices}
		}
	}
	return schema, nil
}

// CompileSourceEligibleEvidence validates v8 before making a new INTERNAL v7
// admission object. Raw caller/provider bytes are never mutated or repaired.
// Any failed guard rejects the whole extraction, rather than dropping a fact.
func CompileSourceEligibleEvidence(prompt string, raw []byte) ([]byte, error) {
	if len(raw) > 65536 {
		return nil, errors.New("source-eligible evidence exceeds 65536 bytes")
	}
	if err := validateIntentJSON(raw); err != nil {
		return nil, err
	}
	var envelope struct {
		Version      string                       `json:"version"`
		Requirements []json.RawMessage            `json:"requirements"`
		Quantities   map[string][]json.RawMessage `json:"quantities"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if !exactFields(raw, "version", "requirements", "quantities") || envelope.Version != SourceEligibleVersion {
		return nil, errors.New("invalid source-eligible evidence envelope")
	}
	_, eligibility, err := eligibleSource(prompt)
	if err != nil {
		return nil, err
	}
	for i, entry := range envelope.Requirements {
		var f struct {
			Kind, Value, Anchor string
			Evidence            []string
		}
		if err := json.Unmarshal(entry, &f); err != nil {
			return nil, err
		}
		if f.Kind != "feature" {
			continue
		}
		if !exactFields(entry, "kind", "value", "state", "evidence", "anchor") || !member(f.Anchor, eligibility.FeatureAnchors[f.Value]...) || !member(f.Anchor, f.Evidence...) {
			return nil, fmt.Errorf("requirement %d: feature lacks an eligible cited anchor", i)
		}
		var projected map[string]json.RawMessage
		if err := json.Unmarshal(entry, &projected); err != nil {
			return nil, err
		}
		delete(projected, "anchor")
		projectedBytes, err := json.Marshal(projected)
		if err != nil {
			return nil, err
		}
		envelope.Requirements[i] = projectedBytes
	}
	for id, entries := range envelope.Quantities {
		for _, entry := range entries {
			var f struct{ Kind, Field string }
			if err := json.Unmarshal(entry, &f); err != nil {
				return nil, err
			}
			if f.Kind == "number" && !member(f.Field, eligibility.QuantityRoles[id]...) {
				return nil, fmt.Errorf("quantity %s: numeric role lacks source eligibility", id)
			}
		}
	}
	envelope.Version = GroundedEvidenceVersion
	projected, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	return CompileGroundedEvidence(prompt, projected)
}

func DecodeSourceEligibleEvidenceIntent(prompt string, raw []byte) (Decision, error) {
	compiled, err := CompileSourceEligibleEvidence(prompt, raw)
	if err != nil {
		return localDecision(prompt, "clarify", "The source-eligible extraction could not be validated; no board was generated.", nil), err
	}
	return decodeConnectionEvidenceWithLimit(prompt, compiled, groundedMaxAssertions)
}
