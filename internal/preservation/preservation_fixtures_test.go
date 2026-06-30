package preservation_test

import (
	"path/filepath"
	"testing"

	"kicadai/internal/inspect"
	"kicadai/internal/preservation"
)

func TestPreservationFixtureCorpus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                 string
		wantScope            preservation.Scope
		wantPreservationOnly int
		wantUnsupportedKinds map[string]int
	}{
		{name: "clean_project", wantScope: preservation.ScopeImported},
		{name: "schematic_raw", wantScope: preservation.ScopeImported, wantPreservationOnly: 1, wantUnsupportedKinds: map[string]int{"rule_area": 1}},
		{name: "pcb_preserved", wantScope: preservation.ScopeImported, wantPreservationOnly: 1, wantUnsupportedKinds: map[string]int{"future_widget": 1}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := filepath.Join("testdata", test.name)
			summary, err := inspect.Project(root)
			if err != nil {
				t.Fatal(err)
			}
			if summary.Preservation == nil {
				t.Fatalf("preservation report missing; unsupported=%d", len(summary.Unsupported))
			}
			if summary.Preservation.Scope != test.wantScope {
				t.Fatalf("scope = %s, want %s", summary.Preservation.Scope, test.wantScope)
			}
			if summary.Preservation.Summary.PreservationOnly != test.wantPreservationOnly {
				t.Fatalf("preservation-only = %d, want %d", summary.Preservation.Summary.PreservationOnly, test.wantPreservationOnly)
			}
			if !hasUnsupportedKinds(summary.Unsupported, test.wantUnsupportedKinds) {
				t.Fatalf("unsupported = %#v, want kinds %#v", summary.Unsupported, test.wantUnsupportedKinds)
			}
		})
	}
}

func hasUnsupportedKinds(nodes []inspect.UnsupportedNode, want map[string]int) bool {
	got := map[string]int{}
	for _, node := range nodes {
		got[node.Kind] += node.Count
	}
	if len(got) != len(want) {
		return false
	}
	for kind, count := range want {
		if got[kind] != count {
			return false
		}
	}
	return true
}
