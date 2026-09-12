package compositionlowering

import (
	"context"
	"encoding/json"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
	"kicadai/internal/schematiclayout"
)

func TestFunctionalOwnershipPreservesPhysicalRequest(t *testing.T) {
	ctx := context.Background()
	catalog, err := components.LoadCatalog(ctx, components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, diagnostics := architecturesearch.NewCatalogRegistry(catalog)
	if reports.HasBlockingIssue(diagnostics) {
		t.Fatal(diagnostics)
	}
	graph := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in", SchematicLayoutProfile: circuitgraph.SchematicLayoutTopologyV2})
	for _, controller := range []bool{false, true} {
		r := referencePromotionRequirement(t, controller)
		search := architecturesearch.Search(ctx, r, registry, architecturesearch.SearchOptions{CatalogHash: graph.CatalogHash()})
		if search.Selected == nil {
			t.Fatal(search.Issues)
		}
		resolver := ArchitectureSimulationPlanResolver{Requirement: r, Search: search, GraphResolver: graph}
		resolved, err := resolver.resolveArchitectureCandidate(ctx, closedloopsynthesis.CandidateState{Fingerprint: search.Selected.Fingerprint})
		if err != nil {
			t.Fatal(err)
		}
		request, diagnostics := circuitgraph.ToDesignRequest(resolved.Resolved)
		if reports.HasBlockingIssue(diagnostics) {
			t.Fatal(diagnostics)
		}
		before, _ := json.Marshal(request)
		oldLayout := schematicir.CloneLayout(request.ExplicitCircuit.Schematic.Layout)
		if err := applyFunctionalLayout(&request, *search.Selected, resolved.SynthesisReport, schematiclayout.FunctionalOwnershipV1); err != nil {
			t.Fatal(err)
		}
		layout := request.ExplicitCircuit.Schematic.Layout
		if len(layout.FunctionalOwners) != len(request.ExplicitCircuit.Schematic.Circuit.Components) || len(layout.Groups) < 2 {
			t.Fatal("incomplete ownership")
		}
		for _, selection := range resolved.SynthesisReport.Selections {
			if selection.ParentID == "" {
				continue
			}
			found := false
			for _, owner := range layout.FunctionalOwners {
				if owner.Component == selection.IntentID && owner.Parent == selection.ParentID {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing recorded support parent for %s", selection.IntentID)
			}
		}
		encoded, _ := json.Marshal(request)
		replay := request
		replay.ExplicitCircuit = nil
		if err := json.Unmarshal(encoded, &replay); err != nil {
			t.Fatal(err)
		}
		originalLayout, _ := json.Marshal(layout)
		replayedLayout, _ := json.Marshal(replay.ExplicitCircuit.Schematic.Layout)
		if string(originalLayout) != string(replayedLayout) {
			t.Fatal("serialized layout differs")
		}
		clone := schematicir.CloneLayout(layout)
		clone.FunctionalOwners[0].Group = "changed"
		if layout.FunctionalOwners[0].Group == "changed" {
			t.Fatal("owner slice aliases clone")
		}
		request.ExplicitCircuit.Schematic.Layout = oldLayout
		after, _ := json.Marshal(request)
		if string(before) != string(after) {
			t.Fatal("non-layout request data changed")
		}
		if err := applyFunctionalLayout(&request, *search.Selected, resolved.SynthesisReport, "unknown"); err == nil {
			t.Fatal("unknown policy accepted")
		}
		bad := *search.Selected
		bad.Selections = nil
		if err := applyFunctionalLayout(&request, bad, resolved.SynthesisReport, schematiclayout.FunctionalOwnershipV1); err == nil {
			t.Fatal("missing fragment provenance accepted")
		}
		bad = *search.Selected
		bad.Selections = append(append([]architecturesearch.FragmentSelection(nil), bad.Selections...), bad.Selections[0])
		if err := applyFunctionalLayout(&request, bad, resolved.SynthesisReport, schematiclayout.FunctionalOwnershipV1); err == nil {
			t.Fatal("duplicate provenance accepted")
		}
	}
}
