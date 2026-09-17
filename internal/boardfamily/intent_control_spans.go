package boardfamily

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"unicode"
	"unicode/utf8"
)

// SourceResidual is an exact, non-overlapping slice of the original request,
// not a rewritten prompt. Together with Controls, these spans partition every
// source byte. No residual is discarded merely because it looks like filler.
type SourceResidual struct {
	ID       string `json:"id"`
	ClauseID int    `json:"clause_id"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
	Text     string `json:"text"`
}

var compoundDefaultPhrase = regexp.MustCompile(`(?i)\b(?:reviewed\s+(?:default\s+)?(?:operating|electrical|supply|power|ambient)(?:\s+and\s+(?:electrical|supply|power|ambient))?\s+(?:limits|defaults)|standard\s+electrical\s+defaults)\b`)
var compoundRequest = regexp.MustCompile(`(?i)^\s*(?:(?:please\s+)?(?:use|make|build|keep|apply|retain)|i(?:'|’)d\s+like|i\s+would\s+like|(?:could|would)\s+you\s+(?:please\s+)?(?:use|make|build))\s+`)
var compoundAcceptance = regexp.MustCompile(`(?i)^\s*(?:your|the)\s+.+\s+(?:are\s+(?:fine|acceptable))\s*[.!?;]*\s*$`)
var compoundScopeAmbiguity = regexp.MustCompile(`(?i)\b(?:not|never|no|without|avoid|forbid\w*|says?|saying|label|text|mention\w*|rather)\b`)
var standardWord = regexp.MustCompile(`(?i)\bstandard\b`)

func compoundDefaultControls(clause RequestClause, offset int) []SourceControl {
	// Only affirmative requests/acceptance can recognize a default noun phrase.
	// Negation/quotation-like wording disables recognition, not the remainder.
	if (!compoundRequest.MatchString(clause.Text) && !compoundAcceptance.MatchString(clause.Text)) || compoundScopeAmbiguity.MatchString(clause.Text) {
		return nil
	}
	var controls []SourceControl
	for _, span := range compoundDefaultPhrase.FindAllStringIndex(clause.Text, -1) {
		if !boundaryWordEdges(clause.Text, span[0], span[1]) {
			continue
		}
		controls = append(controls, SourceControl{ClauseID: clause.ID, Kind: "reviewed_defaults_for_unspecified_values", Start: offset + span[0], End: offset + span[1]})
	}
	return controls
}

func boundaryWordEdges(text string, start, end int) bool {
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '_' }
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(text[:start])
		if isWord(r) {
			return false
		}
	}
	if end < len(text) {
		r, _ := utf8.DecodeRuneInString(text[end:])
		if isWord(r) {
			return false
		}
	}
	return true
}

func standardOnlyInsideControls(clause RequestClause, offset int, controls []SourceControl) bool {
	matches := standardWord.FindAllStringIndex(clause.Text, -1)
	if len(matches) == 0 {
		return false
	}
	for _, span := range matches {
		covered := false
		for _, control := range controls {
			covered = covered || control.ClauseID == clause.ID && offset+span[0] >= control.Start && offset+span[1] <= control.End
		}
		if !covered {
			return false // Another occurrence may name an actual standard profile.
		}
	}
	return true
}

func withBoundaryResiduals(input SemanticBoundaryRequest) SemanticBoundaryRequest {
	input.Residuals = []SourceResidual{}
	offset := 0
	for _, clause := range input.Source.Clauses {
		start := offset
		appendResidual := func(end int) {
			if start < end {
				input.Residuals = append(input.Residuals, SourceResidual{
					ID: "r" + strconv.Itoa(len(input.Residuals)), ClauseID: clause.ID,
					Start: start, End: end, Text: input.Source.Request[start:end],
				})
			}
		}
		for _, control := range input.Controls {
			if control.ClauseID != clause.ID {
				continue
			}
			appendResidual(control.Start)
			start = control.End
		}
		offset += len(clause.Text)
		appendResidual(offset)
	}
	return input
}

func boundaryAdditionalSchema(input SemanticBoundaryRequest, clauseID int) map[string]any {
	ids := []string{}
	for _, residual := range input.Residuals {
		if residual.ClauseID == clauseID {
			ids = append(ids, residual.ID)
		}
	}
	if len(ids) == 0 {
		return map[string]any{"type": "array", "maxItems": 0, "items": map[string]any{"type": "string"}}
	}
	variants := []any{}
	for _, kind := range []string{"other", "unclear"} {
		state := "state"
		if kind == "unclear" {
			state = "uncertain"
		}
		variants = append(variants, objectSchema(map[string]any{
			"kind": enumSchema(kind), "detail": map[string]any{"type": "string"},
			"state":   map[string]any{"$ref": "#/$defs/" + state},
			"context": map[string]any{"$ref": "#/$defs/context"}, "span": enumSchema(ids...),
		}))
	}
	return map[string]any{"type": "array", "maxItems": 64, "items": map[string]any{"anyOf": variants}}
}

// Validate residual ownership before internal lowering. Removing a span label
// is not dropping a fact: the complete original fact retains its clause anchor
// through v9 admission. The caller's provider bytes are never modified.
func lowerBoundaryAdditional(input SemanticBoundaryRequest, additional map[string][]json.RawMessage) error {
	owners := map[string]int{}
	for _, residual := range input.Residuals {
		owners[residual.ID] = residual.ClauseID
	}
	for _, clause := range input.Source.Clauses {
		id := "c" + strconv.Itoa(clause.ID)
		entries, ok := additional[id]
		if !ok || entries == nil || len(entries) > 64 {
			return fmt.Errorf("invalid residual additional inventory for %s", id)
		}
		for i, entry := range entries {
			var fact map[string]json.RawMessage
			if err := json.Unmarshal(entry, &fact); err != nil {
				return err
			}
			var span string
			if err := json.Unmarshal(fact["span"], &span); err != nil {
				return fmt.Errorf("additional %s[%d] requires a residual span", id, i)
			}
			owner, exists := owners[span]
			if !exists || owner != clause.ID || !exactFields(entry, "kind", "detail", "state", "context", "span") {
				return fmt.Errorf("additional %s[%d] has an invalid residual anchor or fields", id, i)
			}
			delete(fact, "span")
			lowered, err := json.Marshal(fact)
			if err != nil {
				return err
			}
			entries[i] = lowered
		}
	}
	return nil
}
