package architecturesearch

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFrozenDynamicElectrothermalCorpusSearchesDeterministically(t *testing.T) {
	root := frozenDynamicElectrothermalCorpusRoot()
	manifestData, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest frozenDynamicManifest
	decodeFrozenClosedLoopStrict(t, manifestData, &manifest)
	registry, issues := NewCatalogRegistry(loadArchitectureCatalog(t))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	for _, row := range manifest.Fixtures {
		row := row
		t.Run(row.ID, func(t *testing.T) {
			data, readErr := os.ReadFile(filepath.Join(root, row.File))
			if readErr != nil {
				t.Fatal(readErr)
			}
			requirement, decodeIssues := DecodeStrict(bytes.NewReader(data))
			if len(decodeIssues) != 0 {
				t.Fatalf("decode issues = %#v", decodeIssues)
			}
			first := Search(context.Background(), requirement, registry, SearchOptions{CatalogHash: "dynamic-electrothermal-corpus"})
			if first.Status != SearchSelected || first.Selected == nil {
				t.Fatalf("search=%s issues=%#v rejections=%#v", first.Status, first.Issues, first.Rejections)
			}
			if first.Selected.SystemPlan == nil || first.Backtracking == nil {
				t.Fatalf("V5 search omitted hierarchical plan/backtracking: %#v", first)
			}
			firstBytes, marshalErr := json.Marshal(first)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			second := Search(context.Background(), requirement, registry, SearchOptions{CatalogHash: "dynamic-electrothermal-corpus"})
			secondBytes, marshalErr := json.Marshal(second)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if !bytes.Equal(firstBytes, secondBytes) {
				t.Fatal("V5 dynamic search result changed on replay")
			}
		})
	}
}

func TestSequencedRailStabilityProjectsToUpstreamRegulators(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(frozenDynamicElectrothermalCorpusRoot(), "sequenced_dual_rail_controller.json"))
	if err != nil {
		t.Fatal(err)
	}
	requirement, issues := DecodeStrict(bytes.NewReader(data))
	if len(issues) != 0 {
		t.Fatalf("decode issues = %#v", issues)
	}
	for _, objectiveID := range []string{"generate_3v3", "generate_5v"} {
		index := slices.IndexFunc(requirement.Requirements.Objectives, func(objective Objective) bool { return objective.ID == objectiveID })
		if index < 0 {
			t.Fatalf("objective %s absent", objectiveID)
		}
		constraints := effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[index])
		if !slices.ContainsFunc(constraints, func(constraint Constraint) bool {
			return constraint.Name == "analysis_stability" && constraint.Relation == "required"
		}) {
			t.Fatalf("%s constraints omit stability: %#v", objectiveID, constraints)
		}
	}
}
