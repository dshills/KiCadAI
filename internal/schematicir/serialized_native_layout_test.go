package schematicir

import (
	"encoding/json"
	"kicadai/internal/reports"
	"reflect"
	"testing"
)

func TestNativeLayoutPreservesInferredRanksAcrossSerialization(t *testing.T) {
	doc := validLEDDocument()
	// Use the wire contract so this regression also catches a missing decoder
	// field instead of relying on a test-only reconstruction of runtime state.
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatal(err)
	}
	wire["layout"].(map[string]any)["native_profile"] = "annotation-v2"
	b, err = json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	before := NormalizeLayoutIntent(doc)
	b, err = json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	var after Document
	if err := json.Unmarshal(b, &after); err != nil {
		t.Fatal(err)
	}
	after = NormalizeLayoutIntent(after)
	if !reflect.DeepEqual(before.Layout, after.Layout) {
		t.Fatalf("native layout lost inferred rank semantics during JSON round trip: before=%+v after=%+v", before.Layout.Groups, after.Layout.Groups)
	}
}

func TestNativeExplicitRankRemainsFixedAndPoliciesFailClosed(t *testing.T) {
	doc := validLEDDocument()
	doc.Layout.NativeProfile = "annotation-v2"
	doc.Layout.Groups = []Group{{ID: "explicit", Members: []string{doc.Circuit.Components[0].ID}, Rank: 7}}
	normalized := NormalizeLayoutIntent(doc)
	for _, g := range normalized.Layout.Groups {
		if g.ID == "explicit" && (g.Inferred || g.RankPolicy != "" || g.Rank != 7) {
			t.Fatalf("explicit group changed: %+v", g)
		}
	}
	for _, profile := range []string{"annotation-v3", "typo"} {
		doc = validLEDDocument()
		doc.Layout.NativeProfile = profile
		if !reports.HasBlockingIssue(Validate(doc)) {
			t.Fatal("unknown native profile accepted")
		}
	}
	for _, profile := range []string{"", "annotation-v2"} {
		doc = validLEDDocument()
		doc.Layout.NativeProfile = profile
		doc.Layout.Groups = []Group{{ID: "explicit", Members: []string{doc.Circuit.Components[0].ID}, RankPolicy: "inferred-v9"}}
		if !reports.HasBlockingIssue(Validate(doc)) {
			t.Fatal("unknown rank policy accepted")
		}
	}
}

func TestNativeReadingGuideUsesExplicitPinsAndLeavesLegacyEmpty(t *testing.T) {
	doc := validLEDDocument()
	if nativeSchematicNotes(doc) != nil {
		t.Fatal("legacy guide changed")
	}
	doc.Layout.NativeProfile = "annotation-v2"
	before, _ := json.Marshal(doc.Circuit)
	if len(nativeSchematicNotes(doc)) < len(doc.Circuit.Components) {
		t.Fatal("missing guide entries")
	}
	after, _ := json.Marshal(doc.Circuit)
	if string(before) != string(after) {
		t.Fatal("guide mutated circuit")
	}
}
