package promotionrunner

import (
	"path/filepath"
	"testing"
)

func TestProtocolAwareBusPromotionMatrixUsesFrozenRequirementCorpus(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	document, err := LoadMatrix(
		filepath.Join(root, "specs", "protocol-aware-bus-synthesis", "PROMOTION_MATRIX.json"),
		root,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(document.Matrix.Scenarios), 4; got != want {
		t.Fatalf("scenario count = %d, want %d", got, want)
	}
	if got, want := len(document.Matrix.NegativeCases), 11; got != want {
		t.Fatalf("negative case count = %d, want %d", got, want)
	}
	wantFixtures := map[string]bool{
		"internal/architecturesearch/testdata/protocol_aware_bus_corpus/i2c_partial_power.json":           false,
		"internal/architecturesearch/testdata/protocol_aware_bus_corpus/segmented_smbus.json":             false,
		"internal/architecturesearch/testdata/protocol_aware_bus_corpus/spi_mixed_direction.json":         false,
		"internal/architecturesearch/testdata/protocol_aware_bus_corpus/uart_inactive_partial_power.json": false,
	}
	for _, scenario := range document.Matrix.Scenarios {
		if scenario.Lane != "requirement" {
			t.Fatalf("%s lane = %q, want requirement", scenario.ID, scenario.Lane)
		}
		if scenario.Board.Mode != "synthesized" || scenario.Board.Layers != 2 {
			t.Fatalf("%s board = %#v, want synthesized two-layer board", scenario.ID, scenario.Board)
		}
		if _, ok := wantFixtures[scenario.Fixture]; !ok {
			t.Fatalf("%s fixture = %q, want frozen bus corpus fixture", scenario.ID, scenario.Fixture)
		}
		wantFixtures[scenario.Fixture] = true
	}
	for fixture, seen := range wantFixtures {
		if !seen {
			t.Fatalf("promotion matrix is missing %q", fixture)
		}
	}
}
