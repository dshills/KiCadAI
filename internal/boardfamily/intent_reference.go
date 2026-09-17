package boardfamily

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ReferenceIntentVersion is an OFFLINE candidate, not the production provider
// contract. InterpretWithPolicy continues to use the frozen version-2 protocol.
const ReferenceIntentVersion = "3-indexed-quantities-experimental"

// ReferencedFact cites application-owned clauses rather than asking a model to
// copy quotes or re-emit a complete clause inventory. Several sources can retain
// an antecedent together with a pronoun, negation, or temporal qualifier.
// References establish provenance, NOT correctness of the fact's meaning.
type ReferencedFact struct {
	Kind       string `json:"kind"`
	Value      string `json:"value,omitempty"`
	State      string `json:"state"`
	Sources    []int  `json:"sources"`
	Quantities []int  `json:"quantities"`
	Detail     string `json:"detail,omitempty"`
}

type ReferencedIntent struct {
	Version string           `json:"version"`
	Facts   []ReferencedFact `json:"facts"`
}

// DecodeReferencedIntent is pure local code: no provider, budget, filesystem,
// generator, or native output is touched. Historical provider bytes must remain
// version 2 and be replayed with DecodeIntent, never silently migrated here.
func DecodeReferencedIntent(prompt string, raw []byte) (Decision, error) {
	return decodeReferencedIntentWithLimit(prompt, raw, 64)
}

// Only a separately validated successor envelope may use a larger internal
// inventory. The public historical decoder retains its original 64-fact cap.
func decodeReferencedIntentWithLimit(prompt string, raw []byte, maxFacts int) (Decision, error) {
	failure := localDecision(prompt, "clarify", "The source-referenced extraction could not be validated; no board was generated.", nil)
	request, err := PrepareReferencedRequest(prompt)
	if err != nil {
		return failure, err
	}
	clauses := request.Clauses
	if len(raw) > 65536 {
		return failure, errors.New("referenced intent exceeds 65536 bytes")
	}
	if err := validateIntentJSON(raw); err != nil {
		return failure, err
	}
	var intent ReferencedIntent
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&intent); err != nil {
		return failure, err
	}
	if !exactFields(raw, "version", "facts") || intent.Version != ReferenceIntentVersion || len(intent.Facts) > maxFacts {
		return failure, errors.New("invalid referenced intent version or fact inventory")
	}
	var envelope struct {
		Facts []json.RawMessage `json:"facts"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return failure, err
	}

	// The admission engine still receives every original clause, in original
	// order. Empty/background clauses no longer require model-authored "none"
	// facts. This never certifies that the model captured all user requirements.
	derived := RequirementIntent{Version: "2", Clauses: make([]IntentClause, len(clauses))}
	for i := range clauses {
		derived.Clauses[i].ID = i
	}
	covered := map[int]bool{}
	for i, fact := range intent.Facts {
		fields := []string{"kind", "value", "state", "sources", "quantities"}
		switch fact.Kind {
		case "number":
			if len(fact.Quantities) != 1 {
				return failure, fmt.Errorf("fact %d: a numeric field requires exactly one source quantity", i)
			}
		case "other", "unclear":
			fields = []string{"kind", "detail", "state", "sources", "quantities"}
		case "sensor", "measurement", "profile":
			if len(fact.Quantities) != 0 {
				return failure, fmt.Errorf("fact %d: identity facts cannot consume numeric requirements", i)
			}
		case "feature":
		default:
			return failure, fmt.Errorf("fact %d: unknown referenced fact kind", i)
		}
		if !exactFields(envelope.Facts[i], fields...) || len(fact.Sources) == 0 || len(fact.Sources) > len(clauses) {
			return failure, fmt.Errorf("fact %d: invalid fields or missing source references", i)
		}
		if !member(fact.State, "required", "not_required", "forbidden", "uncertain") || fact.Kind == "unclear" && fact.State != "uncertain" {
			return failure, fmt.Errorf("fact %d: invalid requirement state", i)
		}
		seen := make(map[int]bool, len(fact.Sources))
		for _, id := range fact.Sources {
			if id < 0 || id >= len(clauses) || seen[id] {
				return failure, fmt.Errorf("fact %d: invalid or duplicated source reference", i)
			}
			seen[id] = true
		}
		quantitySeen := map[int]bool{}
		for _, id := range fact.Quantities {
			if id < 0 || id >= len(request.Quantities) || quantitySeen[id] || !seen[request.Quantities[id].ClauseID] {
				return failure, fmt.Errorf("fact %d: invalid, duplicated or uncited source quantity", i)
			}
			quantitySeen[id], covered[id] = true, true
		}
		var evidence []string
		for _, clause := range clauses {
			if seen[clause.ID] {
				evidence = append(evidence, clause.Text)
			}
		}
		// Join with a boundary: non-adjacent clauses must not manufacture a word
		// or numeric literal at their junction. The original sources remain the
		// authoritative evidence, not this ephemeral validation representation.
		f := RequirementFact{Kind: fact.Kind, Value: fact.Value, State: fact.State,
			Detail: fact.Detail, Quote: strings.Join(evidence, "\n")}
		if fact.Kind == "number" {
			q := request.Quantities[fact.Quantities[0]]
			value, ok := q.Fields[fact.Value]
			if !ok || !referencedQuantityRoleCompatible(fact.Value, clauses[q.ClauseID].Text) {
				return failure, fmt.Errorf("fact %d: source quantity has incompatible units, endpoints or role for %s", i, fact.Value)
			}
			f.Number = &value
			if fact.State == "uncertain" || fact.State == "forbidden" {
				derived.Clauses[q.ClauseID].Facts = append(derived.Clauses[q.ClauseID].Facts, referencedNumberConstraint(f, q.Text)...)
				f.State = "not_required" // Retain evidence without applying a positive value.
			}
		} else {
			validationFact := f
			if member(f.Kind, "other", "unclear") {
				validationFact.State = "" // v2 shape validation; states were checked above.
			}
			encoded, err := json.Marshal(validationFact)
			if err != nil {
				return failure, err
			}
			if err := validateFact(validationFact, encoded, validationFact.Quote); err != nil {
				return failure, fmt.Errorf("fact %d: %w", i, err)
			}
			if f.Kind == "other" && f.State == "uncertain" {
				f.Kind = "unclear"
				f.Detail = "Do you require the following additional constraint? " + f.Detail
			}
		}
		for _, clause := range clauses {
			if seen[clause.ID] {
				local := f
				local.Quote = clause.Text // Application bytes; never a repaired provider quote.
				derived.Clauses[clause.ID].Facts = append(derived.Clauses[clause.ID].Facts, local)
			}
		}
	}

	// Coverage is by occurrence ID, not matching text, magnitude or shared
	// clause. A feature fact cannot conceal an omitted unrelated quantity.
	for _, q := range request.Quantities {
		if !covered[q.ID] {
			// Feed the question into the existing decision precedence rather
			// than hiding a known unsupported requirement behind an early
			// clarification. This is application-derived, not a provider fact.
			derived.Clauses[q.ClauseID].Facts = append(derived.Clauses[q.ClauseID].Facts, RequirementFact{
				Kind: "unclear", Quote: q.Text,
				Detail: fmt.Sprintf("Please confirm the role and requirement for %s (source quantity %d); that explicit quantity was not retained by the extraction.", q.Text, q.ID),
			})
		}
	}
	return admitIntent(prompt, derived, clauses), nil
}

func referencedNumberConstraint(f RequirementFact, quote string) []RequirementFact {
	question := RequirementFact{Kind: "unclear", Quote: quote,
		Detail: "Please clarify the " + f.State + " numeric constraint for " + f.Value + " (" + quote + "); no positive value was inferred."}
	if f.State != "forbidden" || !member(f.Value, "clock_hz", "pullup_ohms") {
		return []RequirementFact{question}
	}
	// Clock/pull-up prohibitions can be implemented as profile exclusions. If
	// future families reuse a profile ID with different values, ask a question
	// instead of accidentally excluding that profile in the wrong family.
	matches := map[string]bool{}
	for _, family := range Catalog() {
		for _, p := range family.Profiles {
			value := p.ResistanceOhms
			if f.Value == "clock_hz" {
				value = float64(p.ClockHz)
			}
			matched := value == *f.Number
			if prior, ok := matches[p.ID]; ok && prior != matched {
				return []RequirementFact{question}
			}
			matches[p.ID] = matched
		}
	}
	var profiles []string
	for profile, matched := range matches {
		if matched {
			profiles = append(profiles, profile)
		}
	}
	sortStrings(profiles)
	var constraints []RequirementFact
	for _, profile := range profiles {
		constraints = append(constraints, RequirementFact{Kind: "profile", Value: profile, State: "forbidden", Quote: quote})
	}
	return constraints
}
