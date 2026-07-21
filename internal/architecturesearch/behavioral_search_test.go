package architecturesearch

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFrozenSimulationGroundedCorpusGeneratesDistinctArchitecturesDeterministically(t *testing.T) {
	root := frozenClosedLoopCorpusRoot()
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest frozenClosedLoopManifest
	decodeFrozenClosedLoopStrict(t, manifestBytes, &manifest)
	registry, issues := NewCatalogRegistry(loadArchitectureCatalog(t))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	for _, fixture := range manifest.Fixtures {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			contents, readErr := os.ReadFile(filepath.Join(root, fixture.File))
			if readErr != nil {
				t.Fatal(readErr)
			}
			requirement, decodeIssues := DecodeStrict(bytes.NewReader(contents))
			if len(decodeIssues) != 0 {
				t.Fatalf("decode issues: %#v", decodeIssues)
			}
			first := Search(context.Background(), requirement, registry, SearchOptions{CatalogHash: "behavioral-corpus"})
			if first.Status != SearchSelected || first.Selected == nil {
				t.Fatalf("search=%s issues=%#v rejections=%#v", first.Status, first.Issues, first.Rejections)
			}
			if requirement.Acceptance.RequireAlternatives && len(first.Alternatives) == 0 {
				t.Fatal("behavioral requirement did not retain a materially distinct architecture")
			}
			firstBytes, marshalErr := json.Marshal(first)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			second := Search(context.Background(), requirement, registry, SearchOptions{CatalogHash: "behavioral-corpus"})
			secondBytes, _ := json.Marshal(second)
			if !bytes.Equal(firstBytes, secondBytes) {
				t.Fatal("v3 architecture search replay differs")
			}
		})
	}
}

func TestBehavioralProjectionAvoidsInventedBipolarTarget(t *testing.T) {
	minimum, maximum := -0.5, 0.5
	if constraints := constraintsFromBehavior(BehavioralRequirement{Metric: "dc_voltage", Min: &minimum, Max: &maximum, Unit: "V"}); len(constraints) != 0 {
		t.Fatalf("bipolar behavioral interval projected as target constraints: %#v", constraints)
	}
}

func TestBehavioralProjectionPreservesAbsoluteMuteLimit(t *testing.T) {
	minimum, maximum := -0.05, 0.05
	constraints := constraintsFromBehavior(BehavioralRequirement{Metric: "muted_output_voltage", Min: &minimum, Max: &maximum, Unit: "V"})
	if len(constraints) != 1 || constraints[0].Relation != "maximum" {
		t.Fatalf("mute constraints = %#v, want one absolute maximum", constraints)
	}
	value, _, ok := projectedNumericValue(constraints[0])
	if !ok || value != .05 {
		t.Fatalf("mute magnitude = %.12g, %t; want 0.05 V", value, ok)
	}
}

func TestRiseTimeProjectionUsesHalfPeriodFrequencyBound(t *testing.T) {
	maximum := 0.5e-6
	constraints := constraintsFromBehavior(BehavioralRequirement{Metric: "rise_time", Max: &maximum, Unit: "s"})
	if len(constraints) != 2 {
		t.Fatalf("rise-time constraints = %#v", constraints)
	}
	value, _, ok := projectedNumericValue(constraints[1])
	if !ok || value != 1e6 {
		t.Fatalf("projected bus frequency = %g, %t, want 1e6", value, ok)
	}
}
