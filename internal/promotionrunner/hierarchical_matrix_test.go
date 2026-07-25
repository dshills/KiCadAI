package promotionrunner

import (
	"path/filepath"
	"testing"
)

func TestHierarchicalMultiDomainPromotionMatrixUsesFrozenRequirementCorpus(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	document, err := LoadMatrix(
		filepath.Join(root, "specs", "hierarchical-multi-domain-synthesis", "PROMOTION_MATRIX.json"),
		root,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(document.Matrix.Scenarios), 6; got != want {
		t.Fatalf("scenario count = %d, want %d", got, want)
	}
	wantFixtures := map[string]bool{
		"internal/architecturesearch/testdata/hierarchical_multi_domain_corpus/current_limited_switched_load_system.json":       false,
		"internal/architecturesearch/testdata/hierarchical_multi_domain_corpus/isolated_mixed_voltage_gateway_system.json":      false,
		"internal/architecturesearch/testdata/hierarchical_multi_domain_corpus/precision_acquisition_alarm_system.json":         false,
		"internal/architecturesearch/testdata/hierarchical_multi_domain_corpus/protected_class_ab_system.json":                  false,
		"internal/architecturesearch/testdata/hierarchical_multi_domain_corpus/regulated_sensor_mcu_communications_system.json": false,
		"internal/architecturesearch/testdata/hierarchical_multi_domain_corpus/split_supply_precision_monitor_system.json":      false,
	}
	for _, scenario := range document.Matrix.Scenarios {
		if scenario.Lane != "requirement" {
			t.Fatalf("%s lane = %q, want requirement", scenario.ID, scenario.Lane)
		}
		if scenario.Board.Mode != "synthesized" || scenario.Board.Layers != 2 {
			t.Fatalf("%s board = %#v, want synthesized two-layer board", scenario.ID, scenario.Board)
		}
		if _, ok := wantFixtures[scenario.Fixture]; !ok {
			t.Fatalf("%s fixture = %q, want frozen hierarchical corpus fixture", scenario.ID, scenario.Fixture)
		}
		wantFixtures[scenario.Fixture] = true
	}
	for fixture, seen := range wantFixtures {
		if !seen {
			t.Fatalf("promotion matrix is missing %q", fixture)
		}
	}
}
