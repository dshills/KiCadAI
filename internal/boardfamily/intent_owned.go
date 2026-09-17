package boardfamily

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
)

// OwnedEvidenceVersion is an experimental opt-in wire protocol. It is never
// selected by the CLI default or inferred from historical extraction bytes.
const OwnedEvidenceVersion = "4-owned-evidence-experimental"

// OwnedNumericChoice binds one occurrence, field, converted value and owning
// source clause. This table is generated locally, never supplied by a model.
// Dimensional eligibility is NOT proof that the field is semantically correct.
type OwnedNumericChoice struct {
	ID         string  `json:"id"`
	QuantityID int     `json:"quantity_id"`
	ClauseID   int     `json:"clause_id"`
	Field      string  `json:"field"`
	Value      float64 `json:"value"`
}

type OwnedEvidenceRequest struct {
	ReferencedRequest
	NumericChoices []OwnedNumericChoice `json:"numeric_choices"`
}

// PrepareOwnedEvidenceRequest retains the entire request and every occurrence.
// References cN and qN address application-owned clauses and quantities. Numeric
// references qN/field bind the relation atomically, without model-authored joins.
func PrepareOwnedEvidenceRequest(prompt string) (OwnedEvidenceRequest, error) {
	source, err := PrepareReferencedRequest(prompt)
	request := OwnedEvidenceRequest{ReferencedRequest: source, NumericChoices: []OwnedNumericChoice{}}
	if err != nil {
		return request, err
	}
	for _, q := range source.Quantities {
		fields := make([]string, 0, len(q.Fields))
		for field := range q.Fields {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		for _, field := range fields {
			request.NumericChoices = append(request.NumericChoices, OwnedNumericChoice{
				ID: "q" + strconv.Itoa(q.ID) + "/" + field, QuantityID: q.ID,
				ClauseID: q.ClauseID, Field: field, Value: q.Fields[field],
			})
		}
	}
	return request, nil
}

func ownedReferences(request OwnedEvidenceRequest) (clauses, all []string) {
	for _, c := range request.Clauses {
		clauses = append(clauses, "c"+strconv.Itoa(c.ID))
	}
	all = append(all, clauses...)
	for _, q := range request.Quantities {
		all = append(all, "q"+strconv.Itoa(q.ID))
	}
	return clauses, all
}

// OwnedEvidenceSchema has a bounded flat fact list. Repeated references have
// set-union meaning by definition; their raw bytes are not rewritten. Every qN
// automatically contributes its owning clause. Additional clause references
// preserve antecedents, exclusions and temporal context but still need review.
func OwnedEvidenceSchema(prompt string) (map[string]any, error) {
	request, err := PrepareOwnedEvidenceRequest(prompt)
	if err != nil {
		return nil, err
	}
	clauses, all := ownedReferences(request)
	references := func(values []string, minimum int) map[string]any {
		return map[string]any{"type": "array", "minItems": minimum, "maxItems": len(values), "items": enumSchema(values...)}
	}
	states := enumSchema("required", "not_required", "forbidden", "uncertain")
	var sensors, features []string
	for _, sensor := range []string{"BMP280", "SHT31"} {
		if sensorIdentityGrounded(sensor, prompt) {
			sensors = append(sensors, sensor)
		}
	}
	for feature := range intentFeatureReasons {
		features = append(features, feature)
	}
	sort.Strings(features)
	var variants []any
	for _, v := range []struct {
		kind   string
		values []string
		refs   []string
	}{
		{"sensor", sensors, clauses},
		{"measurement", []string{"pressure", "temperature", "humidity"}, clauses},
		{"profile", []string{"standard", "fast", "low_current"}, clauses},
		{"feature", features, all},
	} {
		if len(v.values) == 0 {
			continue
		}
		variants = append(variants, objectSchema(map[string]any{
			"kind": enumSchema(v.kind), "value": enumSchema(v.values...),
			"state": states, "evidence": references(v.refs, 1),
		}))
	}
	for _, kind := range []string{"other", "unclear"} {
		state := states
		if kind == "unclear" {
			state = enumSchema("uncertain")
		}
		variants = append(variants, objectSchema(map[string]any{
			"kind": enumSchema(kind), "detail": map[string]any{"type": "string"},
			"state": state, "evidence": references(all, 1),
		}))
	}
	if len(request.NumericChoices) > 0 {
		choices := make([]string, 0, len(request.NumericChoices))
		for _, choice := range request.NumericChoices {
			choices = append(choices, choice.ID)
		}
		variants = append(variants, objectSchema(map[string]any{
			"kind": enumSchema("number"), "choice": enumSchema(choices...),
			"state": states, "context": references(clauses, 0),
		}))
	}
	return objectSchema(map[string]any{"version": enumSchema(OwnedEvidenceVersion),
		"facts": map[string]any{"type": "array", "maxItems": 64, "items": map[string]any{"anyOf": variants}}}), nil
}

// CompileOwnedEvidenceIntent is a pure representation compiler. Its result is
// an internal admission input, not a model response or historical v3 evidence.
// It establishes reference ownership/dimensions, not semantic correctness.
func CompileOwnedEvidenceIntent(prompt string, raw []byte) (ReferencedIntent, error) {
	return compileOwnedEvidenceWithLimit(prompt, raw, 64)
}

func compileOwnedEvidenceWithLimit(prompt string, raw []byte, maxFacts int) (ReferencedIntent, error) {
	compiled := ReferencedIntent{Version: ReferenceIntentVersion, Facts: []ReferencedFact{}}
	request, err := PrepareOwnedEvidenceRequest(prompt)
	if err != nil {
		return compiled, err
	}
	if len(raw) > 65536 {
		return compiled, errors.New("owned evidence exceeds 65536 bytes")
	}
	if err := validateIntentJSON(raw); err != nil {
		return compiled, err
	}
	var envelope struct {
		Version string            `json:"version"`
		Facts   []json.RawMessage `json:"facts"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return compiled, err
	}
	if !exactFields(raw, "version", "facts") || envelope.Version != OwnedEvidenceVersion || envelope.Facts == nil || len(envelope.Facts) > maxFacts {
		return compiled, errors.New("invalid owned evidence version or fact inventory")
	}
	clauses, all := ownedReferences(request)
	numbers := make(map[string]OwnedNumericChoice, len(request.NumericChoices))
	for _, choice := range request.NumericChoices {
		numbers[choice.ID] = choice
	}
	facts := make([]ReferencedFact, 0, len(envelope.Facts))
	for i, rawFact := range envelope.Facts {
		var fact struct {
			Kind     string   `json:"kind"`
			Value    string   `json:"value"`
			Detail   *string  `json:"detail"`
			State    string   `json:"state"`
			Evidence []string `json:"evidence"`
			Choice   string   `json:"choice"`
			Context  []string `json:"context"`
		}
		if err := json.Unmarshal(rawFact, &fact); err != nil {
			return compiled, fmt.Errorf("fact %d: %w", i, err)
		}
		if !member(fact.State, "required", "not_required", "forbidden", "uncertain") {
			return compiled, fmt.Errorf("fact %d: invalid state", i)
		}
		out := ReferencedFact{Kind: fact.Kind, Value: fact.Value,
			State: fact.State, Sources: []int{}, Quantities: []int{}}
		seenClauses, seenQuantities := map[int]bool{}, map[int]bool{}
		refs, allowed, minimum := fact.Evidence, all, 1
		fields := []string{"kind", "value", "state", "evidence"}
		switch fact.Kind {
		case "number":
			fields = []string{"kind", "choice", "state", "context"}
			choice, ok := numbers[fact.Choice]
			if !ok {
				return compiled, fmt.Errorf("fact %d: unknown numeric choice", i)
			}
			out.Value = choice.Field
			seenClauses[choice.ClauseID], seenQuantities[choice.QuantityID] = true, true
			refs, allowed, minimum = fact.Context, clauses, 0
		case "sensor":
			allowed = clauses
			if !member(fact.Value, "BMP280", "SHT31") || !sensorIdentityGrounded(fact.Value, prompt) {
				return compiled, fmt.Errorf("fact %d: sensor not explicitly named", i)
			}
		case "measurement":
			allowed = clauses
			if !member(fact.Value, "pressure", "temperature", "humidity") {
				return compiled, fmt.Errorf("fact %d: invalid measurement", i)
			}
		case "profile":
			allowed = clauses
			if !member(fact.Value, "standard", "fast", "low_current") {
				return compiled, fmt.Errorf("fact %d: invalid profile", i)
			}
		case "feature":
			if _, ok := intentFeatureReasons[fact.Value]; !ok {
				return compiled, fmt.Errorf("fact %d: invalid feature", i)
			}
		case "other", "unclear":
			fields = []string{"kind", "detail", "state", "evidence"}
			if fact.Detail == nil {
				return compiled, fmt.Errorf("fact %d: detail must be a string", i)
			}
			out.Detail = *fact.Detail
			if fact.Kind == "unclear" && fact.State != "uncertain" {
				return compiled, fmt.Errorf("fact %d: unclear must remain uncertain", i)
			}
		default:
			return compiled, fmt.Errorf("fact %d: invalid kind", i)
		}
		if !exactFields(rawFact, fields...) || refs == nil || len(refs) < minimum || len(refs) > len(allowed) {
			return compiled, fmt.Errorf("fact %d: invalid fields or reference array", i)
		}
		for _, ref := range refs {
			if !member(ref, allowed...) {
				return compiled, fmt.Errorf("fact %d: unknown or ineligible evidence reference", i)
			}
			// Lookup membership above is authoritative, not permissive parsing.
			id, parseErr := strconv.Atoi(ref[1:])
			if parseErr != nil {
				return compiled, fmt.Errorf("fact %d: invalid generated reference", i)
			}
			if ref[0] == 'c' {
				seenClauses[id] = true
			} else {
				seenQuantities[id] = true
				seenClauses[request.Quantities[id].ClauseID] = true
			}
		}
		for _, c := range request.Clauses {
			if seenClauses[c.ID] {
				out.Sources = append(out.Sources, c.ID)
			}
		}
		for _, q := range request.Quantities {
			if seenQuantities[q.ID] {
				out.Quantities = append(out.Quantities, q.ID)
			}
		}
		facts = append(facts, out)
	}
	compiled.Facts = facts
	return compiled, nil
}

// DecodeOwnedEvidenceIntent is a pure offline prototype adapter to the existing
// deterministic admission engine. Raw v4 bytes remain authoritative; callers
// must not present the internal compiled form as captured provider evidence.
func DecodeOwnedEvidenceIntent(prompt string, raw []byte) (Decision, error) {
	return decodeOwnedEvidenceWithLimit(prompt, raw, 64)
}

func decodeOwnedEvidenceWithLimit(prompt string, raw []byte, maxFacts int) (Decision, error) {
	compiled, err := compileOwnedEvidenceWithLimit(prompt, raw, maxFacts)
	if err != nil {
		return localDecision(prompt, "clarify", "The owned-evidence extraction could not be validated; no board was generated.", nil), err
	}
	encoded, err := json.Marshal(compiled)
	if err != nil {
		return Decision{}, err
	}
	return decodeReferencedIntentWithLimit(prompt, encoded, maxFacts)
}
