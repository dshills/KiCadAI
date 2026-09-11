package main

import (
	"bytes"
	"encoding/json"
	"kicadai/internal/behavioralintent"
	"kicadai/internal/reports"
	"testing"
)

func TestCanonicalOmittedIssues(t *testing.T) {
	r := behavioralintent.Result{Status: behavioralintent.StatusReady}
	b, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	var f map[string]json.RawMessage
	if e = json.Unmarshal(b, &f); e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(f["issues"], new([]json.RawMessage)); e == nil {
		t.Fatal("frozen failure not reproduced")
	}
	a, e := canonical(r)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(a, []byte(`"issues"`)) {
		t.Fatal("omitted field manufactured")
	}
	r.Status = behavioralintent.StatusInvalid
	different, e := canonical(r)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Equal(a, different) {
		t.Fatal("status difference lost")
	}
}
func TestCanonicalFullIssuesAndDuplicates(t *testing.T) {
	a := behavioralintent.Result{Issues: []reports.Issue{{Code: "B", Path: "p", Message: "one"}, {Code: "A"}, {Code: "B", Path: "p", Message: "one"}}}
	b := a
	b.Issues = []reports.Issue{a.Issues[2], a.Issues[0], a.Issues[1]}
	x, e := canonical(a)
	if e != nil {
		t.Fatal(e)
	}
	y, e := canonical(b)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(x, y) {
		t.Fatal("order not normalized")
	}
	b.Issues = b.Issues[:2]
	y, e = canonical(b)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Equal(x, y) {
		t.Fatal("duplicate loss hidden")
	}
	b = a
	b.Issues = append([]reports.Issue(nil), a.Issues...)
	b.Issues[0].Message = "changed"
	y, e = canonical(b)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Equal(x, y) {
		t.Fatal("issue field difference hidden")
	}
}
