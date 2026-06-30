package profiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/reports"
)

func TestValidateAcceptsValidProfile(t *testing.T) {
	profile := loadProfileFixture(t, "valid_profile.json")
	if issues := Validate(profile); len(issues) != 0 {
		t.Fatalf("Validate() issues = %#v", issues)
	}
	summary := Summarize(profile)
	if summary.ID != "local_standard" || summary.Hash == "" {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestValidateRejectsInvalidProfile(t *testing.T) {
	profile := Profile{
		Schema:  "bad.schema",
		ID:      "bad id",
		Version: "bad version",
		Source:  Source{Kind: "remote", RetrievedAt: "yesterday"},
		Units:   "mil",
		Stackup: Stackup{MinLayers: 4, MaxLayers: 6, AllowedLayerCounts: []int{2, 2, -1, 8}, MinBoardThicknessMM: 1.0, MaxBoardThicknessMM: 1.6, DefaultBoardThicknessMM: 2.0},
		Copper:  Copper{MinTraceWidthMM: -0.1},
		Drill:   Drill{MinDrillMM: -0.3},
		EdgePlating: EdgePolicy{
			MinCastellationDrillMM: 0.6,
			MinCastellationPitchMM: 0.5,
		},
		Metadata: Metadata{
			AllowedBoardFinishes: []string{"ENIG", "ENIG", " "},
			WarningOnlyFields:    []string{"edge_plating", "edge_plating", "typo.field"},
		},
	}
	issues := Validate(profile)
	for _, want := range []string{
		"fabrication_profile.schema",
		"fabrication_profile.id",
		"fabrication_profile.name",
		"fabrication_profile.version",
		"fabrication_profile.source.kind",
		"fabrication_profile.source.retrieved_at",
		"fabrication_profile.units",
		"fabrication_profile.stackup.allowed_layer_counts[1]",
		"fabrication_profile.stackup.allowed_layer_counts[2]",
		"fabrication_profile.stackup.allowed_layer_counts[3]",
		"fabrication_profile.stackup.default_board_thickness_mm",
		"fabrication_profile.copper.min_trace_width_mm",
		"fabrication_profile.drill.min_drill_mm",
		"fabrication_profile.edge_plating.min_castellation_pitch_mm",
		"fabrication_profile.metadata.allowed_board_finishes[1]",
		"fabrication_profile.metadata.allowed_board_finishes[2]",
		"fabrication_profile.metadata.warning_only_fields[1]",
		"fabrication_profile.metadata.warning_only_fields[2]",
	} {
		if !hasIssuePath(issues, want) {
			t.Fatalf("missing issue path %s in %#v", want, issues)
		}
	}
}

func TestHashIgnoresFormatting(t *testing.T) {
	compact := []byte(`{"schema":"kicadai.fabrication.profile.v1","id":"same","name":"Same","version":"1","units":"mm","copper":{"min_trace_width_mm":0.15}}`)
	pretty := []byte(`{
	  "schema": "kicadai.fabrication.profile.v1",
	  "id": "same",
	  "name": "Same",
	  "version": "1",
	  "units": "mm",
	  "copper": {
	    "min_trace_width_mm": 0.15
	  }
	}`)
	var first Profile
	var second Profile
	if err := json.Unmarshal(compact, &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(pretty, &second); err != nil {
		t.Fatal(err)
	}
	firstHash, err := Hash(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := Hash(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("hashes differ: %s != %s", firstHash, secondHash)
	}
	first.Source = Source{Kind: SourceLocal, Path: "/tmp/a.json", URL: "https://example.invalid/a", RetrievedAt: "2026-01-01"}
	second.Source = Source{Kind: SourceLocal, Path: "/tmp/b.json", URL: "https://example.invalid/b", RetrievedAt: "2026-02-01"}
	firstHash, err = Hash(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err = Hash(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("hashes should ignore source metadata: %s != %s", firstHash, secondHash)
	}

	first.Stackup.AllowedLayerCounts = []int{4, 2}
	first.Metadata.AllowedBoardFinishes = []string{"HASL", "ENIG"}
	first.Metadata.WarningOnlyFields = []string{"panelization", "edge_plating"}
	second.Stackup.AllowedLayerCounts = []int{2, 4}
	second.Metadata.AllowedBoardFinishes = []string{"ENIG", "HASL"}
	second.Metadata.WarningOnlyFields = []string{"edge_plating", "panelization"}
	firstHash, err = Hash(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err = Hash(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("hashes should canonicalize set-like slices: %s != %s", firstHash, secondHash)
	}
	if first.Stackup.AllowedLayerCounts[0] != 4 {
		t.Fatalf("Hash mutated caller slices: %#v", first.Stackup.AllowedLayerCounts)
	}

	first.ID = " same "
	first.Name = " Same "
	first.Version = " 1 "
	first.Units = " mm "
	first.Metadata.AllowedBoardFinishes = []string{" ENIG "}
	second.ID = "same"
	second.Name = "Same"
	second.Version = "1"
	second.Units = "mm"
	second.Metadata.AllowedBoardFinishes = []string{"ENIG"}
	firstHash, err = Hash(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err = Hash(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("hashes should trim canonical string fields: %s != %s", firstHash, secondHash)
	}

	first.Metadata.WarningOnlyFields = nil
	second.Metadata.WarningOnlyFields = []string{}
	firstHash, err = Hash(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err = Hash(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("hashes should treat nil and empty slices equally: %s != %s", firstHash, secondHash)
	}

	first.Metadata.AllowedBoardFinishes = []string{" ENIG ", "", "ENIG"}
	second.Metadata.AllowedBoardFinishes = []string{"ENIG"}
	first.Stackup.AllowedLayerCounts = []int{4, 2, 2}
	second.Stackup.AllowedLayerCounts = []int{2, 4}
	firstHash, err = Hash(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err = Hash(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("hashes should deduplicate canonical set fields: %s != %s", firstHash, secondHash)
	}
}

func loadProfileFixture(t *testing.T, name string) Profile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatal(err)
	}
	return profile
}

func hasIssuePath(issues []reports.Issue, path string) bool {
	for _, issue := range issues {
		if issue.Path == path {
			return true
		}
	}
	return false
}
