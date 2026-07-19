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
	"kicadai/internal/reports"
)

func TestFrozenOpenSetCorpusLowersDeterministically(t *testing.T) {
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, registryIssues := architecturesearch.NewCatalogRegistry(catalog)
	if len(registryIssues) != 0 {
		t.Fatalf("registry issues = %#v", registryIssues)
	}
	resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"})
	paths, err := filepath.Glob(filepath.Join("..", "circuitgraph", "testdata", "open_set_composition_corpus", "*.json"))
	paths = slices.DeleteFunc(paths, func(path string) bool { return filepath.Base(path) == "manifest.json" })
	if err != nil || len(paths) != 5 {
		t.Fatalf("corpus paths = %#v, %v", paths, err)
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
			search := architecturesearch.Search(context.Background(), requirement, registry, architecturesearch.SearchOptions{CatalogHash: "test-catalog"})
			if search.Status != architecturesearch.SearchSelected {
				t.Fatalf("search status = %s issues=%#v rejections=%#v", search.Status, search.Issues, search.Rejections)
			}
			first, lowerIssues := Lower(requirement, search)
			if len(lowerIssues) != 0 {
				t.Fatalf("lower issues = %#v", lowerIssues)
			}
			if validation := circuitgraph.Validate(first.Document); len(validation) != 0 {
				t.Fatalf("graph validation = %#v", validation)
			}
			resolved, resolveIssues := resolver.Resolve(context.Background(), first.Document)
			if reports.HasBlockingIssue(resolveIssues) {
				t.Fatalf("resolve issues = %#v", resolveIssues)
			}
			if len(resolved.Components) == 0 || len(resolved.Nets) == 0 || resolved.Synthesis == nil {
				t.Fatalf("resolved document is incomplete: components=%d nets=%d synthesis=%v", len(resolved.Components), len(resolved.Nets), resolved.Synthesis != nil)
			}
			schematic, schematicIssues := circuitgraph.ToSchematicIR(resolved)
			if reports.HasBlockingIssue(schematicIssues) || len(schematic.Circuit.Components) == 0 || len(schematic.Circuit.Nets) == 0 {
				t.Fatalf("schematic lowering is incomplete: issues=%#v components=%d nets=%d", schematicIssues, len(schematic.Circuit.Components), len(schematic.Circuit.Nets))
			}
			designRequest, designIssues := circuitgraph.ToDesignRequest(resolved)
			if reports.HasBlockingIssue(designIssues) || designRequest.ExplicitCircuit == nil {
				t.Fatalf("writer request lowering is incomplete: issues=%#v request=%#v", designIssues, designRequest)
			}
			second, secondIssues := Lower(requirement, search)
			if len(secondIssues) != 0 {
				t.Fatalf("second lower issues = %#v", secondIssues)
			}
			firstJSON, _ := json.Marshal(first)
			secondJSON, _ := json.Marshal(second)
			if !bytes.Equal(firstJSON, secondJSON) {
				t.Fatal("composition lowering replay differs")
			}
		})
	}
}

func TestLowerInterfacesDoesNotDuplicateReferencePortOnItsOwnReturnNet(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{{ID: "gnd", Kind: "reference"}},
		Ports:   []architecturesearch.Port{{ID: "ground", Kind: "reference", Direction: "bidirectional", Domain: "gnd"}},
	}}
	interfaces, _ := lowerInterfaces(requirement, newDisjointSet(), map[string]circuitgraph.FunctionalEndpoint{}, map[string]nodeMetadata{})
	if len(interfaces) != 1 || len(interfaces[0].Signals) != 1 {
		t.Fatalf("interfaces = %#v, want one physical reference signal", interfaces)
	}
	if interfaces[0].Signals[0].Role != circuitgraph.NetRoleGround || interfaces[0].Signals[0].Name == "return" {
		t.Fatalf("reference signal = %#v", interfaces[0].Signals[0])
	}
}
