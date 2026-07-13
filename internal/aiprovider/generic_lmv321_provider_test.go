package aiprovider

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"kicadai/internal/circuitgraph"
)

func TestRecordedGenericLMV321GraphResolvesAndLowers(t *testing.T) {
	fixtureDir := filepath.Dir(providerFixturePath(t, "generic_lmv321_ac_gain_stage", "prompt.txt"))
	prompt, err := os.ReadFile(filepath.Join(fixtureDir, "prompt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := os.ReadFile(filepath.Join(fixtureDir, "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewRecordedProvider("generic_lmv321_ac_gain_stage", recorded)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.GenerateIntent(context.Background(), GenerateRequest{
		Prompt: string(prompt), SchemaVersion: EnvelopeSchemaV1, Attempt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	projection := genericLMV321ProjectionFor(t, decodeAndResolveGraph(t, result.IntentJSON, loadGenericFixtureCatalog(t)))
	if projection.ComponentCount != 14 || projection.NetCount != 8 || projection.PowerFlagCount != 2 {
		t.Fatalf("graph cardinality = components:%d nets:%d flags:%d, want components:14 nets:8 flags:2", projection.ComponentCount, projection.NetCount, projection.PowerFlagCount)
	}
	if projection.Flow != circuitgraph.FlowLeftToRight {
		t.Fatalf("schematic flow = %q", projection.Flow)
	}
	if projection.ComponentID != "opamp.ti.lmv321.sot23_5" || projection.SymbolID != "Amplifier_Operational:LMV321" || projection.FootprintID != "Package_TO_SOT_SMD:SOT-23-5" {
		t.Fatalf("amplifier identity = component:%q symbol:%q footprint:%q, want component:%q symbol:%q footprint:%q",
			projection.ComponentID, projection.SymbolID, projection.FootprintID,
			"opamp.ti.lmv321.sot23_5", "Amplifier_Operational:LMV321", "Package_TO_SOT_SMD:SOT-23-5")
	}
	wantReview := []string{
		"gain_bandwidth_status=review_required",
		"load_compatibility_status=review_required",
		"noise_distortion_status=review_required",
		"output_drive_status=review_required",
		"stability_status=review_required",
	}
	if !slices.Equal(projection.ReviewEvidence, wantReview) {
		t.Fatalf("analog review evidence = %q, want %q", projection.ReviewEvidence, wantReview)
	}
}

type genericLMV321Projection struct {
	ComponentCount int
	NetCount       int
	PowerFlagCount int
	Flow           circuitgraph.Flow
	ComponentID    string
	SymbolID       string
	FootprintID    string
	ReviewEvidence []string
}

func genericLMV321ProjectionFor(t *testing.T, resolved circuitgraph.ResolvedDocument) genericLMV321Projection {
	t.Helper()
	projection := genericLMV321Projection{
		ComponentCount: len(resolved.Components),
		NetCount:       len(resolved.Nets),
		PowerFlagCount: len(resolved.Source.PowerFlags),
		Flow:           resolved.Source.Schematic.Flow,
	}
	for _, component := range resolved.Components {
		if component.ComponentID != "opamp.ti.lmv321.sot23_5" {
			continue
		}
		projection.ComponentID = component.ComponentID
		projection.SymbolID = component.SymbolID
		projection.FootprintID = component.FootprintID
		for _, property := range component.Instance.Properties {
			if property.Value == "review_required" {
				projection.ReviewEvidence = append(projection.ReviewEvidence, property.Name+"="+property.Value)
			}
		}
	}
	slices.Sort(projection.ReviewEvidence)
	return projection
}
