package compositionlowering

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
)

func TestHierarchicalCorpusLowersWithCompleteDeterministicEvidence(t *testing.T) {
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, registryIssues := architecturesearch.NewCatalogRegistry(catalog)
	if len(registryIssues) != 0 {
		t.Fatalf("registry issues = %#v", registryIssues)
	}
	resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"})
	paths, err := filepath.Glob(filepath.Join("..", "architecturesearch", "testdata", "hierarchical_multi_domain_corpus", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	paths = slices.DeleteFunc(paths, func(path string) bool { return filepath.Base(path) == "manifest.json" })
	slices.Sort(paths)
	if len(paths) != 6 {
		t.Fatalf("hierarchical corpus has %d requirements, want 6", len(paths))
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			requirement, decodeIssues := architecturesearch.DecodeStrict(bytes.NewReader(data))
			if len(decodeIssues) != 0 {
				t.Fatalf("decode issues = %#v", decodeIssues)
			}
			search := architecturesearch.Search(context.Background(), requirement, registry, architecturesearch.SearchOptions{CatalogHash: resolver.CatalogHash()})
			if search.Status != architecturesearch.SearchSelected || search.Selected == nil {
				t.Fatalf("search status=%s issues=%#v rejections=%#v", search.Status, search.Issues, search.Rejections)
			}
			first, loweringIssues := Lower(requirement, search)
			if len(loweringIssues) != 0 {
				t.Fatalf("lowering issues = %#v", loweringIssues)
			}
			if first.Evidence.SystemPlan == nil || first.Evidence.Backtracking == nil ||
				len(first.Evidence.HierarchyBindings) !=
					len(first.Evidence.SystemPlan.Hierarchy.Blocks)+len(first.Evidence.SystemPlan.Interfaces) {
				t.Fatalf("hierarchical lowering evidence is incomplete: %#v", first.Evidence)
			}
			second, replayIssues := Lower(requirement, search)
			if len(replayIssues) != 0 {
				t.Fatalf("replay lowering issues = %#v", replayIssues)
			}
			firstJSON, err := json.Marshal(first)
			if err != nil {
				t.Fatal(err)
			}
			secondJSON, err := json.Marshal(second)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(firstJSON, secondJSON) {
				t.Fatal("hierarchical lowering replay differs")
			}
			retained := append([]architecturesearch.CandidateResult{*search.Selected}, search.Alternatives...)
			for _, candidate := range retained {
				reselected := search
				reselected.Selected = &candidate
				reselected.Alternatives = retainedArchitectureAlternatives(search, candidate.Fingerprint)
				if err := validateRetainedBacktracking(reselected); err != nil {
					t.Fatalf("reselected candidate %s lost retained backtracking evidence: %v", candidate.Fingerprint, err)
				}
			}

			tamperedPlan := search
			selected := *search.Selected
			plan := *selected.SystemPlan
			plan.PlanHash = "tampered"
			selected.SystemPlan = &plan
			tamperedPlan.Selected = &selected
			if _, issues := Lower(requirement, tamperedPlan); len(issues) == 0 {
				t.Fatal("tampered system plan passed composition lowering")
			}

			tamperedOrder := search
			backtracking := *search.Backtracking
			backtracking.Deterministic = false
			tamperedOrder.Backtracking = &backtracking
			if _, issues := Lower(requirement, tamperedOrder); len(issues) == 0 {
				t.Fatal("tampered backtracking evidence passed composition lowering")
			}
		})
	}
}
