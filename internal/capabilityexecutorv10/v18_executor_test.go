package capabilityexecutorv10

import (
	"context"
	"reflect"
	"runtime"
	"testing"

	"kicadai/internal/opentopologysynthesis"
)

func TestNewV18BindsOnlyV18Synthesis(t *testing.T) {
	v17 := NewV17()
	v18 := NewV18()
	if v18.decode == nil || v18.synthesize == nil || v18.promote == nil || v18.observe == nil {
		t.Fatal("V18 executor is incomplete")
	}
	v17Name := runtime.FuncForPC(reflect.ValueOf(v17.synthesize).Pointer()).Name()
	v18Name := runtime.FuncForPC(reflect.ValueOf(v18.synthesize).Pointer()).Name()
	if v17Name == v18Name || v18Name != "kicadai/internal/opentopologysynthesis.SynthesizeV18" {
		t.Fatalf("synthesis bindings = V17 %q V18 %q", v17Name, v18Name)
	}
}

func TestNewV18WithLegacyBindsSeparatedHistoricalInputs(t *testing.T) {
	legacyInventory := opentopologysynthesis.PrimitiveInventory{Hash: "legacy", CatalogHash: "legacy"}
	legacySimulation := opentopologysynthesis.SimulationEnvironment{CatalogHash: "legacy"}
	executor := NewV18WithLegacy(legacyInventory, legacySimulation)
	if executor.synthesize == nil {
		t.Fatal("V18 legacy-isolated synthesis binding is missing")
	}
	got := executor.synthesize(
		context.Background(),
		opentopologysynthesis.Requirement{},
		opentopologysynthesis.PrimitiveInventory{Hash: "extension"},
		opentopologysynthesis.SimulationEnvironment{CatalogHash: "extension"},
		opentopologysynthesis.DefaultPolicy(),
	)
	if got.Report.PrimitiveInventoryHash != legacyInventory.Hash || got.Report.CatalogHash != legacySimulation.CatalogHash {
		t.Fatalf("noneligible V18 inputs = inventory %q catalog %q", got.Report.PrimitiveInventoryHash, got.Report.CatalogHash)
	}
}
