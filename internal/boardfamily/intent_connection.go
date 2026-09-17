package boardfamily

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ConnectionEvidenceVersion is a separately selected experimental protocol.
// It does not change or reinterpret owned-v4, its runtime, or live results.
// The offline prototype's distinct version is intentionally not accepted here.
const ConnectionEvidenceVersion = "5-connection-evidence-experimental"

// ConnectionEvidenceSchema gives wired and wireless symmetric representations.
// Wired means the reviewed non-radio connection mode, not a promise of an
// arbitrary connector, signalling protocol, delivered firmware, or GPIO load.
// Such additional requirements must retain their own facts. A source reference
// still proves only location, not the correctness or completeness of meaning.
func ConnectionEvidenceSchema(prompt string) (map[string]any, error) {
	schema, err := OwnedEvidenceSchema(prompt)
	if err != nil {
		return nil, err
	}
	request, err := PrepareOwnedEvidenceRequest(prompt)
	if err != nil {
		return nil, err
	}
	clauses, _ := ownedReferences(request)
	properties := schema["properties"].(map[string]any)
	properties["version"] = enumSchema(ConnectionEvidenceVersion)
	items := properties["facts"].(map[string]any)["items"].(map[string]any)
	variants := items["anyOf"].([]any)
	// Only the successor schema is changed. A fresh v4 schema is built per call.
	// One canonical radio representation avoids competing feature/connection facts.
	for _, variant := range variants {
		p := variant.(map[string]any)["properties"].(map[string]any)
		if p["kind"].(map[string]any)["enum"].([]string)[0] != "feature" {
			continue
		}
		values := p["value"].(map[string]any)["enum"].([]string)
		kept := make([]string, 0, len(values))
		for _, value := range values {
			if value != "wireless_operation" {
				kept = append(kept, value)
			}
		}
		p["value"] = enumSchema(kept...)
	}
	items["anyOf"] = append(variants, objectSchema(map[string]any{
		"kind": enumSchema("connection"), "value": enumSchema("wired", "wireless"),
		"state":    enumSchema("required", "not_required", "forbidden", "uncertain"),
		"evidence": map[string]any{"type": "array", "minItems": 1, "maxItems": len(clauses), "items": enumSchema(clauses...)},
	}))
	return schema, nil
}

// DecodeConnectionEvidenceIntent performs pure admission of v5
// inputs. The ephemeral lowering is an internal program, never provider output
// or a migration of historical evidence. Original caller bytes are untouched.
// All non-connection constraints use unchanged owned-v4 validation/admission.
func DecodeConnectionEvidenceIntent(prompt string, raw []byte) (Decision, error) {
	return decodeConnectionEvidenceWithLimit(prompt, raw, 64)
}

func decodeConnectionEvidenceWithLimit(prompt string, raw []byte, maxFacts int) (Decision, error) {
	failure := localDecision(prompt, "clarify", "The connection extraction could not be validated; no board was generated.", nil)
	if len(raw) > 65536 {
		return failure, errors.New("connection evidence exceeds 65536 bytes")
	}
	if err := validateIntentJSON(raw); err != nil {
		return failure, err
	}
	var envelope struct {
		Version string            `json:"version"`
		Facts   []json.RawMessage `json:"facts"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return failure, err
	}
	if !exactFields(raw, "version", "facts") || envelope.Version != ConnectionEvidenceVersion || envelope.Facts == nil || len(envelope.Facts) > maxFacts {
		return failure, errors.New("invalid connection evidence version or fact inventory")
	}
	request, err := PrepareOwnedEvidenceRequest(prompt)
	if err != nil {
		return failure, err
	}
	clauses, _ := ownedReferences(request)
	lowered := make([]json.RawMessage, 0, len(envelope.Facts))
	for i, rawFact := range envelope.Facts {
		var fact struct {
			Kind     string   `json:"kind"`
			Value    string   `json:"value"`
			State    string   `json:"state"`
			Evidence []string `json:"evidence"`
		}
		if err := json.Unmarshal(rawFact, &fact); err != nil {
			return failure, fmt.Errorf("fact %d: %w", i, err)
		}
		if fact.Kind != "connection" {
			if fact.Kind == "feature" && fact.Value == "wireless_operation" {
				return failure, errors.New("v5 radio requirements must use connection facts")
			}
			lowered = append(lowered, rawFact)
			continue
		}
		if !exactFields(rawFact, "kind", "value", "state", "evidence") || !member(fact.Value, "wired", "wireless") ||
			!member(fact.State, "required", "not_required", "forbidden", "uncertain") || len(fact.Evidence) == 0 || len(fact.Evidence) > len(clauses) {
			return failure, fmt.Errorf("fact %d: invalid connection fact", i)
		}
		for _, ref := range fact.Evidence {
			if !member(ref, clauses...) {
				return failure, fmt.Errorf("fact %d: invalid connection source", i)
			}
		}
		// Connection facts cannot consume qN occurrences or hide omitted numeric
		// constraints. Neither a supported wire nor an exclusion cancels radio.
		var local map[string]any
		switch {
		case fact.State == "uncertain":
			local = map[string]any{"kind": "unclear", "detail": "Which connection mode do you require: wired or wireless?", "state": "uncertain", "evidence": fact.Evidence}
		case fact.Value == "wireless":
			local = map[string]any{"kind": "feature", "value": "wireless_operation", "state": fact.State, "evidence": fact.Evidence}
		case fact.State == "forbidden":
			local = map[string]any{"kind": "other", "detail": "Wired connection is forbidden, but the reviewed families provide only wired, non-radio operation.", "state": "required", "evidence": fact.Evidence}
		default:
			// Required wired is satisfied by either reviewed family. Not-required
			// wired imposes no prohibition. Retain all other original constraints.
			continue
		}
		encoded, err := json.Marshal(local)
		if err != nil {
			return failure, err
		}
		lowered = append(lowered, encoded)
	}
	encoded, err := json.Marshal(map[string]any{"version": OwnedEvidenceVersion, "facts": lowered})
	if err != nil {
		return failure, err
	}
	return decodeOwnedEvidenceWithLimit(prompt, encoded, maxFacts)
}
