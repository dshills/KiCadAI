package circuitgraph

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/schematiclayout"
)

func TestAnnotationProfilePreservesLegacyPlacementSeed(t *testing.T) {
	resolver := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"})
	doc := loadGraphExample(t, "rc_filter.json")
	legacy, issues := resolver.Resolve(context.Background(), doc)
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	doc.Schematic.Rules.NativeProfile = schematiclayout.NativeAnnotationV2
	native, issues := resolver.Resolve(context.Background(), doc)
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	if legacy.GenerationHash == native.GenerationHash || legacy.ResolutionHash == native.ResolutionHash {
		t.Fatal("annotation provenance was erased")
	}
	before, _ := json.Marshal(native)
	request, issues := ToDesignRequest(native)
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	encoded, _ := json.Marshal(request)
	var value map[string]any
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	seed, ok := value["explicit_circuit"].(map[string]any)["placement_seed"].(map[string]any)
	if !ok {
		t.Fatal("drawing profile has no independent serialized placement seed")
	}
	if seed["policy"] != "annotation-independent-v1" || seed["hash"] != legacy.GenerationHash {
		t.Fatalf("seed=%v, legacy=%s", seed, legacy.GenerationHash)
	}
	after, _ := json.Marshal(native)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("seed derivation mutated resolved source")
	}
	oldRequest, issues := ToDesignRequest(legacy)
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	encoded, _ = json.Marshal(oldRequest)
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	if _, exists := value["explicit_circuit"].(map[string]any)["placement_seed"]; exists {
		t.Fatal("legacy request serialization changed")
	}
	seedBefore := annotationIndependentPlacementSeed(native).Hash
	native.Source.Project.Board.WidthMM++
	if annotationIndependentPlacementSeed(native).Hash == seedBefore {
		t.Fatal("board dimensions were excluded from placement identity")
	}
	native.Source.Project.Board.WidthMM--
	native.Source.Nets[0].WidthMM += 0.1
	if annotationIndependentPlacementSeed(native).Hash == seedBefore {
		t.Fatal("electrical routing constraint was excluded from placement identity")
	}
}
