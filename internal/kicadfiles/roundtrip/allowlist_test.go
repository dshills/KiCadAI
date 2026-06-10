package roundtrip

import "testing"

func TestValidateAllowlistRequiresReason(t *testing.T) {
	err := ValidateAllowlist([]AllowlistEntry{{Category: "normalized-diff"}})
	if err == nil {
		t.Fatal("expected missing reason error")
	}
}

func TestValidateAllowlistRequiresNarrowMatcher(t *testing.T) {
	err := ValidateAllowlist([]AllowlistEntry{{Reason: "known KiCad rewrite"}})
	if err == nil {
		t.Fatal("expected missing matcher error")
	}
}

func TestFilterAllowedDifferencesKeepsUnexpected(t *testing.T) {
	result := Result{
		FixtureName: "fixture",
		FileType:    FileTypePCB,
		Equal:       false,
		Differences: []Difference{
			{Category: "normalized-diff", Section: "layers", Message: "known"},
			{Category: "normalized-diff", Section: "nets", Message: "unexpected"},
		},
	}
	entries := []AllowlistEntry{{
		FileType:    FileTypePCB,
		FixtureName: "fixture",
		Category:    "normalized-diff",
		Section:     "layers",
		Message:     "known",
		Reason:      "temporary layer rewrite",
	}}

	filtered, allowed, err := FilterAllowedDifferences(result, entries)
	if err != nil {
		t.Fatalf("FilterAllowedDifferences returned error: %v", err)
	}
	if len(allowed) != 1 {
		t.Fatalf("allowed = %d, want 1", len(allowed))
	}
	if len(filtered.Differences) != 1 {
		t.Fatalf("remaining = %d, want 1", len(filtered.Differences))
	}
	if filtered.Equal {
		t.Fatal("Equal = true with unexpected difference remaining")
	}
}

func TestFilterAllowedDifferencesMarksEqualWhenAllAllowed(t *testing.T) {
	result := Result{
		FixtureName: "fixture",
		FileType:    FileTypePCB,
		Equal:       false,
		Differences: []Difference{{Category: "summary-diff", Message: "PCB section summary changed"}},
	}
	entries := []AllowlistEntry{{
		FileType: "pcb",
		Category: "summary-diff",
		Message:  "summary changed",
		Reason:   "documented fixture gap",
	}}

	filtered, _, err := FilterAllowedDifferences(result, entries)
	if err != nil {
		t.Fatalf("FilterAllowedDifferences returned error: %v", err)
	}
	if !filtered.Equal {
		t.Fatal("Equal = false, want true after all differences allowed")
	}
	if len(filtered.Differences) != 0 {
		t.Fatalf("differences remain: %#v", filtered.Differences)
	}
}

func TestFilterAllowedDifferencesMatchesDiffContents(t *testing.T) {
	result := Result{
		FixtureName: "fixture",
		FileType:    FileTypePCB,
		Equal:       false,
		Differences: []Difference{{
			Category: "normalized-diff",
			Message:  "first hunk",
			Diff:     "-old layer\n+new layer\n",
		}},
	}
	entries := []AllowlistEntry{{
		Category:     "normalized-diff",
		DiffContains: "+new layer",
		Reason:       "documented rewrite",
	}}

	filtered, allowed, err := FilterAllowedDifferences(result, entries)
	if err != nil {
		t.Fatalf("FilterAllowedDifferences returned error: %v", err)
	}
	if len(allowed) != 1 || len(filtered.Differences) != 0 {
		t.Fatalf("allowed=%d remaining=%d", len(allowed), len(filtered.Differences))
	}
}
