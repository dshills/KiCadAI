package airequirementeval

import (
	"bytes"
	"testing"

	"kicadai/internal/behavioralintent"
	"kicadai/internal/reports"
)

func TestCanonicalCompilationPreservesOmittedIssues(t *testing.T) {
	for _, status := range []behavioralintent.Status{behavioralintent.StatusReady, behavioralintent.StatusUnsupported, behavioralintent.StatusNeedsClarification} {
		for _, issues := range [][]reports.Issue{nil, {}} {
			r := behavioralintent.Result{Status: status, Issues: issues}
			encoded, err := canonicalCompilation(r)
			if err != nil {
				t.Fatalf("%s empty issues: %v", status, err)
			}
			if bytes.Contains(encoded, []byte(`"issues"`)) {
				t.Fatal("omitted issues member manufactured")
			}
			r.Status = behavioralintent.StatusInvalid
			changed, err := canonicalCompilation(r)
			if err != nil || bytes.Equal(encoded, changed) {
				t.Fatal("status difference hidden")
			}
		}
	}
}

func TestCanonicalCompilationPreservesNonDiagnosticArrayOrder(t *testing.T) {
	r := behavioralintent.Result{Status: behavioralintent.StatusReady, Coverage: []behavioralintent.CoverageRecord{{StatementID: "statement_001"}, {StatementID: "statement_002"}}}
	before, err := canonicalCompilation(r)
	if err != nil {
		t.Fatal(err)
	}
	r.Coverage[0], r.Coverage[1] = r.Coverage[1], r.Coverage[0]
	after, err := canonicalCompilation(r)
	if err != nil || bytes.Equal(before, after) {
		t.Fatal("non-diagnostic array order was ignored")
	}
}
