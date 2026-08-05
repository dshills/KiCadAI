package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

func TestDefaultPrimitiveInventoryIsReviewedDeterministicAndPrimitiveOnly(t *testing.T) {
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatalf("model-provenance diagnostics: %#v", diagnostics)
	}
	catalogHash := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog}).CatalogHash()

	first, issues := BuildPrimitiveInventory(catalog, catalogHash, registry)
	if len(issues) != 0 {
		t.Fatalf("inventory issues: %#v", issues)
	}
	if first.Schema != PrimitiveInventorySchema ||
		first.Version != PrimitiveInventoryVersion ||
		len(first.Hash) != 64 ||
		first.CatalogHash != catalogHash ||
		len(first.ModelRegistryHash) != 64 ||
		len(first.PrimitiveRegistry) != 64 ||
		first.Stats.PrimitiveCandidates != len(first.Primitives) ||
		first.Stats.PrimitiveModelClaims < len(first.Primitives) ||
		len(first.Primitives) < 20 {
		t.Fatalf("inventory identity/stats = %#v", first.Stats)
	}

	kinds := map[string]bool{}
	keys := map[string]bool{}
	for index, primitive := range first.Primitives {
		if primitive.Key == "" || keys[primitive.Key] {
			t.Fatalf("primitive %d has empty or duplicate key %q", index, primitive.Key)
		}
		keys[primitive.Key] = true
		kinds[primitive.Kind] = true
		if primitive.CatalogID == "" || primitive.VariantID == "" ||
			primitive.FootprintID == "" || len(primitive.SymbolIDs) == 0 ||
			len(primitive.Terminals) < 2 || len(primitive.Models) == 0 {
			t.Fatalf("incomplete primitive %s: %#v", primitive.Key, primitive)
		}
		if index > 0 && comparePrimitiveCandidates(first.Primitives[index-1], primitive) >= 0 {
			t.Fatalf("primitive inventory is not strictly sorted at %s", primitive.Key)
		}
		for _, terminal := range primitive.Terminals {
			if terminal.Terminal == "" || terminal.Function == "" ||
				terminal.SymbolID == "" || terminal.SymbolPin == "" || terminal.Pad == "" {
				t.Fatalf("%s incomplete terminal: %#v", primitive.Key, terminal)
			}
		}
		for _, model := range primitive.Models {
			if !allowedPrimitiveModelID(model.ModelID) ||
				len(model.ProvenanceSHA256) != 64 ||
				model.ProvenanceSource == "" ||
				model.ProvenanceRevision == "" ||
				len(model.AllowedAnalyses) == 0 {
				t.Fatalf("%s invalid model: %#v", primitive.Key, model)
			}
		}
	}
	for _, requiredKind := range []string{
		"adjustable_voltage_regulator",
		"capacitor",
		"comparator",
		"inductor",
		"n_channel_mosfet",
		"npn_bjt",
		"opamp",
		"p_channel_mosfet",
		"pnp_bjt",
		"reference_diode",
		"resistor",
		"signal_diode",
		"synchronous_buck_regulator",
	} {
		if !kinds[requiredKind] {
			t.Errorf("default inventory lacks %s; rejections=%#v", requiredKind, first.Rejections)
		}
	}
	if !keys["capacitor.kemet.c1210k153f5eml.1210|1210"] {
		t.Fatal("default inventory lacks the concrete fabrication-ready precision capacitor")
	}
	panasonicESR := -1.0
	for _, primitive := range first.Primitives {
		if primitive.CatalogID != "capacitor.panasonic.eeufc1j220.radial" {
			continue
		}
		for _, model := range primitive.Models {
			if model.ModelID != simmodel.PrimitiveCapacitorTransientV1 {
				continue
			}
			for _, parameter := range model.Parameters {
				if parameter.Name == "series_resistance_ohm" {
					panasonicESR = parameter.Value
				}
			}
		}
	}
	if panasonicESR != 1 {
		t.Fatalf("reviewed Panasonic capacitor ESR propagated to transient model = %g, want 1 ohm", panasonicESR)
	}
	if !keys["resistor.vishay.tnpw0805.4k99.0p1|0805"] {
		t.Fatal("default inventory lacks the concrete fabrication-ready precision resistor")
	}
	for _, key := range []string{
		"resistor.vishay.tnpw0805.13k3.0p1|0805",
		"resistor.vishay.tnpw0805.15k0.0p1|0805",
		"resistor.vishay.tnpw0805.90k9.0p1|0805",
		"resistor.vishay.tnpw0805.169k.0p1|0805",
	} {
		if !keys[key] {
			t.Fatalf("default inventory lacks threshold-network resistor %s", key)
		}
	}

	second, issues := BuildPrimitiveInventory(catalog, catalogHash, registry)
	if len(issues) != 0 {
		t.Fatalf("second inventory issues: %#v", issues)
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
		t.Fatal("repeated primitive inventory bytes differ")
	}

	reorderedCatalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(reorderedCatalog.Records)
	reorderedRegistry := registry
	reorderedRegistry.Records = append([]modelprovenance.Record(nil), registry.Records...)
	slices.Reverse(reorderedRegistry.Records)
	reordered, issues := BuildPrimitiveInventory(reorderedCatalog, catalogHash, reorderedRegistry)
	if len(issues) != 0 {
		t.Fatalf("reordered inventory issues: %#v", issues)
	}
	if reordered.Hash != first.Hash {
		t.Fatalf("inventory hash changed under source order: %s != %s", reordered.Hash, first.Hash)
	}
}

func TestPrimitiveInventoryFailsClosedOnMissingTrustInputs(t *testing.T) {
	if _, issues := BuildPrimitiveInventory(nil, "", modelprovenance.Registry{}); len(issues) != 1 ||
		issues[0].Code != CodePrimitiveUnavailable {
		t.Fatalf("nil catalog issues = %#v", issues)
	}

	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	invalidRegistry := modelprovenance.Registry{
		Schema:  modelprovenance.Schema,
		Version: modelprovenance.Version,
	}
	if _, issues := BuildPrimitiveInventory(catalog, strings.Repeat("a", 64), invalidRegistry); len(issues) == 0 ||
		issues[0].Code != CodeModelUnavailable {
		t.Fatalf("invalid model registry issues = %#v", issues)
	}
}

func allowedPrimitiveModelID(id string) bool {
	return slices.Contains([]string{
		simmodel.PrimitiveResistorV1,
		simmodel.PrimitiveCapacitorV1,
		simmodel.PrimitiveCapacitorTransientV1,
		simmodel.PrimitiveInductorTransientV1,
		simmodel.PrimitiveOpAmpV1,
		simmodel.PrimitiveComparatorOpenCollectorV1,
		simmodel.PrimitiveAdjustableLinearRegulatorV1,
		simmodel.PrimitiveFixedLinearRegulatorV1,
		simmodel.PrimitiveSynchronousBuckRegulatorV1,
		simmodel.PrimitiveFloatingAdjustableRegulatorV1,
		simmodel.PrimitiveShuntVoltageReferenceV1,
		simmodel.PrimitiveBidirectionalTVSV1,
		simmodel.PrimitiveUnidirectionalZenerV1,
		simmodel.PrimitiveDiodeShockleyV1,
		simmodel.PrimitiveNMOSSwitchV1,
		simmodel.PrimitivePMOSSwitchV1,
		simmodel.PrimitiveBJTNPNV1,
		simmodel.PrimitiveBJTPNPV1,
	}, id)
}
