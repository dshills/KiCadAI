package simmodel

import (
	"fmt"
	"testing"
)

func BenchmarkEvaluateTransientSwitch(b *testing.B) {
	plan, diagnostics := ResolveWithTopology(transientSwitchIntent(), "test", "catalog-hash", transientSwitchComponents(25), transientSwitchNodes())
	if len(diagnostics) != 0 {
		b.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		report, diagnostics := Evaluate(ClonePlan(plan))
		if len(diagnostics) != 0 || report.Status != "pass" {
			b.Fatalf("evaluate status=%q diagnostics=%+v", report.Status, diagnostics)
		}
	}
}

func BenchmarkResistorPathWithin(b *testing.B) {
	devices := make([]ResolvedDevice, 64)
	for index := range devices {
		devices[index] = ResolvedDevice{
			Component:      fmt.Sprintf("R%d", index+1),
			PrimitiveModel: PrimitiveResistorV1,
			Terminals: []TerminalBinding{
				{Terminal: "A", Net: fmt.Sprintf("net_%d", index)},
				{Terminal: "B", Net: fmt.Sprintf("net_%d", index+1)},
			},
		}
	}
	for _, test := range []struct {
		name string
		plan Plan
	}{
		{name: "unindexed", plan: Plan{Devices: devices}},
		{name: "cached", plan: Plan{Devices: devices, TopologyHash: "benchmark-resistor-chain"}},
	} {
		b.Run(test.name, func(b *testing.B) {
			if !resistorPathWithin(test.plan, "net_0", "net_64", 64) {
				b.Fatal("resistor path was not found during cache warmup")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				if !resistorPathWithin(test.plan, "net_0", "net_64", 64) {
					b.Fatal("resistor path was not found")
				}
			}
		})
	}
}
