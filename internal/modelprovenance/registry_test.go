package modelprovenance

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/simmodel"
)

func TestRegistryStrictDecodeHashLookupAndOrderIndependence(t *testing.T) {
	registry := testRegistry()
	hash, err := Hash(registry)
	if err != nil || hash == "" {
		t.Fatalf("hash=%q err=%v", hash, err)
	}
	reversed := registry
	reversed.Records = append([]Record(nil), registry.Records...)
	slices.Reverse(reversed.Records)
	reversedHash, err := Hash(reversed)
	if err != nil || reversedHash != hash {
		t.Fatalf("reordered hash=%q err=%v want=%q", reversedHash, err, hash)
	}
	modelHash, _ := simmodel.ModelContentHash(simmodel.PrimitiveResistorV1)
	data := []byte(`{"schema":"kicadai.model-provenance-registry.v1","version":1,"records":[{"catalog_id":"r","family":"resistor","model_id":"mna_resistor_v1","provenance":{"source":"datasheet:r","revision":"a","sha256":"` + modelHash + `","review_status":"reviewed","allowed_analyses":["ac_sweep","dc_operating_point"]}}]}`)
	decoded, diagnostics := DecodeStrict(bytes.NewReader(data))
	if len(diagnostics) != 0 {
		t.Fatalf("decode diagnostics: %#v", diagnostics)
	}
	if record, ok := Lookup(decoded, "r", simmodel.PrimitiveResistorV1); !ok || record.Family != "resistor" {
		t.Fatalf("lookup = %#v %t", record, ok)
	}
}

func TestRegistryFailsClosedForMissingTrustUnknownFieldsAndDuplicates(t *testing.T) {
	registry := testRegistry()
	registry.Records[0].Provenance.ReviewStatus = "unreviewed"
	if diagnostics := Validate(Normalize(registry)); len(diagnostics) == 0 {
		t.Fatal("unreviewed provenance was accepted")
	}
	duplicate := testRegistry()
	duplicate.Records = append(duplicate.Records, duplicate.Records[0])
	if diagnostics := Validate(Normalize(duplicate)); len(diagnostics) == 0 {
		t.Fatal("duplicate provenance was accepted")
	}
	unknown := []byte(`{"schema":"kicadai.model-provenance-registry.v1","version":1,"records":[],"model_text":"unsafe"}`)
	if _, diagnostics := DecodeStrict(bytes.NewReader(unknown)); len(diagnostics) == 0 {
		t.Fatal("unknown provider-like model content was accepted")
	}
}

func TestEmbeddedRegistryCoversEveryCheckedInCatalogModelClaim(t *testing.T) {
	registry, diagnostics := LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatalf("default registry diagnostics: %#v", diagnostics)
	}
	if hash, err := Hash(registry); err != nil || hash == "" {
		t.Fatalf("default registry hash=%q err=%v", hash, err)
	}
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	claims := 0
	for _, component := range catalog.Records {
		for _, claim := range component.SimulationModels {
			claims++
			record, exists := Lookup(registry, component.ID, claim.ModelID)
			if !exists {
				t.Errorf("missing provenance for %s/%s", component.ID, claim.ModelID)
				continue
			}
			if record.Family != component.Family {
				t.Errorf("provenance family %s/%s=%s want %s", component.ID, claim.ModelID, record.Family, component.Family)
			}
		}
	}
	if claims == 0 || claims != len(registry.Records) {
		t.Fatalf("catalog claims=%d registry records=%d", claims, len(registry.Records))
	}
}

func testRegistry() Registry {
	modelHash, _ := simmodel.ModelContentHash(simmodel.PrimitiveResistorV1)
	provenance := simmodel.ModelProvenance{Source: "datasheet:r", Revision: "a", SHA256: modelHash, ReviewStatus: "reviewed", AllowedAnalyses: []string{simmodel.AnalysisACSweep, simmodel.AnalysisDCOperatingPoint}}
	return Registry{Schema: Schema, Version: Version, Records: []Record{{CatalogID: "r2", Family: "resistor", ModelID: simmodel.PrimitiveResistorV1, Provenance: provenance}, {CatalogID: "r1", Family: "resistor", ModelID: simmodel.PrimitiveResistorV1, Provenance: provenance}}}
}
