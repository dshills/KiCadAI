package capabilityexecutorv10

import (
	"context"
	"reflect"
	"runtime"
	"testing"

	"kicadai/internal/opentopologysynthesis"
)

func TestNewV19BindsOnlyV19Synthesis(t *testing.T) {
	v18 := NewV18()
	v19 := NewV19()
	if v19.decode == nil || v19.synthesize == nil || v19.promote == nil || v19.observe == nil {
		t.Fatal("V19 executor is incomplete")
	}
	v18Name := runtime.FuncForPC(reflect.ValueOf(v18.synthesize).Pointer()).Name()
	v19Name := runtime.FuncForPC(reflect.ValueOf(v19.synthesize).Pointer()).Name()
	if v18Name == v19Name || v19Name != "kicadai/internal/opentopologysynthesis.SynthesizeV19" {
		t.Fatalf("synthesis bindings = V18 %q V19 %q", v18Name, v19Name)
	}
}

func TestNewV19WithLegacyBindsExactV18AndV17InputsSeparately(t *testing.T) {
	v18Inventory := opentopologysynthesis.PrimitiveInventory{Hash: "v18", CatalogHash: "v18"}
	v18Simulation := opentopologysynthesis.SimulationEnvironment{CatalogHash: "v18"}
	legacyInventory := opentopologysynthesis.PrimitiveInventory{Hash: "legacy", CatalogHash: "legacy"}
	legacySimulation := opentopologysynthesis.SimulationEnvironment{CatalogHash: "legacy"}
	executor := NewV19WithLegacy(v18Inventory, v18Simulation, legacyInventory, legacySimulation)
	if executor.synthesize == nil {
		t.Fatal("V19 isolated synthesis binding is missing")
	}
	got := executor.synthesize(
		context.Background(),
		opentopologysynthesis.Requirement{},
		opentopologysynthesis.PrimitiveInventory{Hash: "v19"},
		opentopologysynthesis.SimulationEnvironment{CatalogHash: "v19"},
		opentopologysynthesis.DefaultPolicy(),
	)
	if got.Report.PrimitiveInventoryHash != legacyInventory.Hash || got.Report.CatalogHash != legacySimulation.CatalogHash {
		t.Fatalf("V19 noneligible delegation inputs = inventory %q catalog %q", got.Report.PrimitiveInventoryHash, got.Report.CatalogHash)
	}
}
