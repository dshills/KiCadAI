package corpusfreezev6

import (
	"strings"
	"testing"

	"kicadai/internal/corpusfreeze"
)

func TestRejectHistoricalNormalized(t *testing.T) {
	digest := strings.Repeat("a", 64)
	report := corpusfreeze.Report{Entries: []corpusfreeze.EntryEvidence{{ID: "new_case", NormalizedSemanticSHA256: digest}}}
	if err := rejectHistoricalNormalized(report, map[string]string{digest: "retired_case"}); err == nil ||
		!strings.Contains(err.Error(), "new_case duplicates historical normalized semantic requirement retired_case") {
		t.Fatalf("normalized historical reuse error = %v", err)
	}
	if err := rejectHistoricalNormalized(report, map[string]string{}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateNormalizedCommitments(t *testing.T) {
	if err := validateNormalizedCommitments(map[string]string{strings.Repeat("a", 64): "retired_case"}); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []map[string]string{
		{"invalid": "retired_case"},
		{strings.Repeat("a", 64): " "},
	} {
		if err := validateNormalizedCommitments(invalid); err == nil {
			t.Fatal("expected error")
		}
	}
}
