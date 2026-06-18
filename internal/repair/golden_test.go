package repair

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
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

func TestPersistedRepairApplyGolden(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	tx := persistedBaseTransaction(t, "demo",
		mustRepairOperation(t, transactions.OpSetBoardOutline, transactions.SetBoardOutlineOperation{Op: transactions.OpSetBoardOutline, Board: &transactions.BoardSize{WidthMM: 40, HeightMM: 25}}, ""),
		mustRepairOperation(t, transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject}, ""),
	)
	if result := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output}); len(result.Issues) != 0 {
		t.Fatalf("apply issues: %#v", result.Issues)
	}
	apply := ApplyPersistedBundle(output, Bundle{
		Schema:        BundleSchemaV1,
		ProjectRoot:   output,
		ProjectName:   "demo",
		Generated:     true,
		Transaction:   &tx,
		RepairOptions: Options{Enabled: true},
	}, PersistedApplyOptions{Execute: true, Overwrite: true, InspectProject: cleanInspection})
	got, err := json.MarshalIndent(persistedGoldenSummary(apply), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	want, err := os.ReadFile(filepath.Join("testdata", "persisted_apply_success.golden.json"))
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
		t.Fatalf("golden mismatch\nwant:\n%s\ngot:\n%s", string(want), string(got))
	}
}

func persistedGoldenSummary(result PersistedApplyResult) map[string]any {
	ops := make([]string, 0, len(result.Transaction.Operations))
	for _, op := range result.Transaction.Operations {
		ops = append(ops, string(op.Op))
	}
	validations := make([]string, 0, len(result.Validation))
	for _, validation := range result.Validation {
		validations = append(validations, validation.Name)
	}
	artifactKinds := make([]string, 0, len(result.Artifacts))
	for _, artifact := range result.Artifacts {
		artifactKinds = append(artifactKinds, string(artifact.Kind))
	}
	return map[string]any{
		"status":          result.Status,
		"repair_status":   result.Repair.Status,
		"operation_kinds": ops,
		"validations":     validations,
		"artifact_kinds":  artifactKinds,
		"issue_count":     len(result.Issues),
	}
}
