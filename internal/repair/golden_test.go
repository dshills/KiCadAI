package repair

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kicadai/internal/reports"
)

func TestRepairPlanGoldens(t *testing.T) {
	cases := []struct {
		name   string
		golden string
		groups []StageIssues
		opts   Options
	}{
		{name: "no repair needed", golden: "no_repair_needed.golden.json", opts: Options{Enabled: true}},
		{name: "planned missing footprint", golden: "planned_missing_footprint.golden.json", opts: Options{Enabled: true, AllowFootprintAssignment: true}, groups: []StageIssues{{Stage: "validation", Issues: []reports.Issue{{Code: reports.CodeMissingFootprint, Message: "missing footprint", Refs: []string{"R1"}}}}}},
		{name: "blocked roundtrip diff", golden: "blocked_roundtrip_diff.golden.json", opts: Options{Enabled: true}, groups: []StageIssues{{Stage: "roundtrip", Issues: []reports.Issue{{Code: reports.CodeRoundTripDiff, Message: "diff"}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.MarshalIndent(BuildPlan(tc.groups, tc.opts), "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			want, err := os.ReadFile(filepath.Join("testdata", tc.golden))
			if err != nil {
				t.Fatal(err)
			}
			var gotJSON any
			var wantJSON any
			if err := json.Unmarshal(got, &gotJSON); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(want, &wantJSON); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotJSON, wantJSON) {
				t.Fatalf("golden mismatch for %s\nwant:\n%s\ngot:\n%s", tc.golden, string(want), string(got))
			}
		})
	}
}
