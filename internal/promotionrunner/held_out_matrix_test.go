package promotionrunner

import (
	"path/filepath"
	"testing"
)

func TestHeldOutCapabilityPromotionMatrixUsesRequirementLane(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	document, err := LoadMatrix(
		filepath.Join(root, "specs", "held-out-capability-expansion", "PROMOTION_MATRIX.json"),
		root,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(document.Matrix.Scenarios), 5; got != want {
		t.Fatalf("scenario count = %d, want %d", got, want)
	}
	for _, scenario := range document.Matrix.Scenarios {
		if scenario.Lane != "requirement" {
			t.Fatalf("%s lane = %q, want requirement", scenario.ID, scenario.Lane)
		}
		if scenario.Board.Mode != "synthesized" || scenario.Board.Layers != 2 {
			t.Fatalf("%s board = %#v, want synthesized two-layer board", scenario.ID, scenario.Board)
		}
	}
}
