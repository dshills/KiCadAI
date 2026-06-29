package schematiclayout

import (
	"reflect"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestParseProfileDefaultsToStandard(t *testing.T) {
	profile, err := ParseProfile("")
	if err != nil {
		t.Fatalf("ParseProfile returned error: %v", err)
	}
	if profile != ProfileStandard {
		t.Fatalf("profile = %s, want %s", profile, ProfileStandard)
	}
	if _, err := ParseProfile("bogus"); err == nil {
		t.Fatalf("ParseProfile accepted invalid profile")
	}
}

func TestSnapPointUsesStableGrid(t *testing.T) {
	got := SnapPoint(kicadfiles.Point{X: kicadfiles.MM(3.1), Y: kicadfiles.MM(4.9)}, kicadfiles.MM(2.54))
	want := kicadfiles.Point{X: kicadfiles.MM(2.54), Y: kicadfiles.MM(5.08)}
	if got != want {
		t.Fatalf("SnapPoint = %#v, want %#v", got, want)
	}
}

func TestNormalizeRequestOrdersDeterministically(t *testing.T) {
	request := Request{
		Components: []Component{
			{Ref: "U1", Stage: StageProcessing, OriginalOrdinal: 2},
			{Ref: "J2", Stage: StageBoundaryOutput, OriginalOrdinal: 1},
			{Ref: "J1", Stage: StageBoundaryInput, OriginalOrdinal: 3},
		},
		Nets: []Net{
			{Name: "OUT", OriginalOrdinal: 2},
			{Name: "IN", OriginalOrdinal: 1},
		},
		Groups: []Group{
			{ID: "b", Stage: StageProcessing, OriginalOrdinal: 2},
			{ID: "a", Stage: StageBoundaryInput, OriginalOrdinal: 1},
		},
	}
	got := NormalizeRequest(request)
	if refs := []string{got.Components[0].Ref, got.Components[1].Ref, got.Components[2].Ref}; !reflect.DeepEqual(refs, []string{"J1", "U1", "J2"}) {
		t.Fatalf("component order = %#v", refs)
	}
	if got.Nets[0].Name != "IN" || got.Groups[0].ID != "a" {
		t.Fatalf("unexpected normalized request = %#v", got)
	}
}

func TestEmptyResultBuildsPassingReport(t *testing.T) {
	result := NormalizeResult(Result{}, DefaultRules(ProfileBasic))
	if !result.Report.Passed {
		t.Fatalf("empty result should pass: %#v", result.Report)
	}
	if result.Report.Profile != ProfileBasic {
		t.Fatalf("profile = %s, want basic", result.Report.Profile)
	}
}

func TestNormalizeDiagnosticsSortsAndBounds(t *testing.T) {
	diagnostics := []Diagnostic{
		{Severity: SeverityInfo, Code: "z", Message: "late"},
		{Severity: SeverityError, Code: "a", Message: "first"},
		{Severity: SeverityWarning, Code: "b", Message: "second"},
	}
	got := NormalizeDiagnostics(diagnostics, 2)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Severity != SeverityError || got[1].Severity != SeverityWarning {
		t.Fatalf("unexpected diagnostic order = %#v", got)
	}
}
