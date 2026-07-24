package promotionrunner

import (
	"path/filepath"
	"testing"
)

func TestStandaloneClockGenerationPromotionMatrixUsesFrozenRequirementCorpus(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	document, err := LoadMatrix(
		filepath.Join(root, "specs", "standalone-clock-generation", "PROMOTION_MATRIX.json"),
		root,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(document.Matrix.Scenarios), 2; got != want {
		t.Fatalf("scenario count = %d, want %d", got, want)
	}
	wantFixtures := map[string]bool{
		"internal/architecturesearch/testdata/standalone_clock_generation_corpus/precision_logic_clock.json": false,
		"internal/architecturesearch/testdata/standalone_clock_generation_corpus/relaxed_logic_clock.json":   false,
	}
	for _, scenario := range document.Matrix.Scenarios {
		if scenario.Lane != "requirement" {
			t.Fatalf("%s lane = %q, want requirement", scenario.ID, scenario.Lane)
		}
		if scenario.Board.Mode != "synthesized" || scenario.Board.Layers != 2 {
			t.Fatalf("%s board = %#v, want synthesized two-layer board", scenario.ID, scenario.Board)
		}
		if _, ok := wantFixtures[scenario.Fixture]; !ok {
			t.Fatalf("%s fixture = %q, want frozen clock corpus fixture", scenario.ID, scenario.Fixture)
		}
		wantFixtures[scenario.Fixture] = true
	}
	for fixture, seen := range wantFixtures {
		if !seen {
			t.Fatalf("promotion matrix is missing %q", fixture)
		}
	}
}
