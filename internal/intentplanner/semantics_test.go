package intentplanner

import (
	"testing"
)

func TestSemanticIndexCapturesMCUPorts(t *testing.T) {
	builder := newTestPlanBuilder()
	id := builder.addBlock("function.1", "mcu", "mcu_minimal", map[string]any{"supply_voltage": "5V"}, "test")
	instance, ok := builder.semantic.instance(id)
	if !ok {
		t.Fatalf("missing semantic instance %q", id)
	}
	if instance.Role != "mcu" {
		t.Fatalf("role = %q", instance.Role)
	}
	for _, role := range []string{"power.vcc", "power.gnd", "mcu.i2c.sda", "mcu.i2c.scl", "mcu.spi.mosi", "mcu.clock.xtal1"} {
		if !instance.hasPortRole(role) {
			t.Fatalf("missing role %s in %#v", role, instance.Ports)
		}
	}
	if instance.SupplyVoltage != "5V" {
		t.Fatalf("supply voltage = %q", instance.SupplyVoltage)
	}
}

func TestSemanticIndexCapturesConnectorPins(t *testing.T) {
	builder := newTestPlanBuilder()
	id := builder.addConnector("interface.1", "i2c_connector", []string{"SDA", "SCL", "VCC", "GND"}, StrengthRequired)
	instance, ok := builder.semantic.instance(id)
	if !ok {
		t.Fatalf("missing semantic instance %q", id)
	}
	for _, role := range []string{"i2c.sda", "i2c.scl", "power.vcc", "power.gnd"} {
		if !instance.hasPortRole(role) {
			t.Fatalf("missing role %s in %#v", role, instance.Ports)
		}
	}
	if got := builder.semantic.byRole("connector"); len(got) != 1 || got[0].ID != id {
		t.Fatalf("connectors = %#v", got)
	}
}

func TestSemanticIndexFindsCapability(t *testing.T) {
	builder := newTestPlanBuilder()
	mcuID := builder.addBlock("function.1", "mcu", "mcu_minimal", nil, "test")
	builder.addBlock("function.2", "indicator", "led_indicator", nil, "test")
	values := builder.semantic.withPortRole("mcu.i2c.sda")
	if len(values) != 1 || values[0].ID != mcuID {
		t.Fatalf("values = %#v", values)
	}
}

func TestResolveSemanticTargetInfersSingleMCU(t *testing.T) {
	builder := newTestPlanBuilder()
	mcuID := builder.addBlock("function.1", "mcu", "mcu_minimal", nil, "test")
	clockID := builder.addBlock("function.2", "clock", "canned_oscillator", nil, "test")
	builder.recordSupportTarget(clockID, "function.2", "functions[1].target", TargetRef{}, StrengthRequired)
	target, ok := builder.resolveMCUSupportTarget(clockID, "mcu.clock.xtal1")
	if !ok || target.ID != mcuID {
		t.Fatalf("target = %#v ok=%v", target, ok)
	}
	if len(builder.plan.Issues) != 0 {
		t.Fatalf("issues = %#v", builder.plan.Issues)
	}
}

func TestResolveSemanticTargetHonorsExplicitID(t *testing.T) {
	builder := newTestPlanBuilder()
	builder.addBlock("function.1", "mcu", "mcu_minimal", nil, "test")
	mcuID := builder.addBlock("function.2", "mcu", "mcu_minimal", nil, "test")
	clockID := builder.addBlock("function.3", "clock", "canned_oscillator", nil, "test")
	builder.recordSupportTarget(clockID, "function.3", "functions[2].target", TargetRef{ID: mcuID}, StrengthRequired)
	target, ok := builder.resolveMCUSupportTarget(clockID, "mcu.clock.xtal1")
	if !ok || target.ID != mcuID {
		t.Fatalf("target = %#v ok=%v", target, ok)
	}
}

func TestResolveSemanticTargetReportsMissingExplicitID(t *testing.T) {
	builder := newTestPlanBuilder()
	clockID := builder.addBlock("function.1", "clock", "canned_oscillator", nil, "test")
	builder.recordSupportTarget(clockID, "function.1", "functions[0].target", TargetRef{ID: "missing"}, StrengthRequired)
	if _, ok := builder.resolveMCUSupportTarget(clockID, "mcu.clock.xtal1"); ok {
		t.Fatal("target resolved unexpectedly")
	}
	if !hasIssuePath(builder.plan.Issues, "functions[0].target.id") {
		t.Fatalf("issues = %#v", builder.plan.Issues)
	}
}

func TestResolveSemanticTargetReportsAmbiguousMCU(t *testing.T) {
	builder := newTestPlanBuilder()
	builder.addBlock("function.1", "mcu", "mcu_minimal", nil, "test")
	builder.addBlock("function.2", "mcu", "mcu_minimal", nil, "test")
	clockID := builder.addBlock("function.3", "clock", "canned_oscillator", nil, "test")
	builder.recordSupportTarget(clockID, "function.3", "functions[2].target", TargetRef{}, StrengthRequired)
	if _, ok := builder.resolveMCUSupportTarget(clockID, "mcu.clock.xtal1"); ok {
		t.Fatal("target resolved unexpectedly")
	}
	if !hasIssuePath(builder.plan.Issues, "functions[2].target") {
		t.Fatalf("issues = %#v", builder.plan.Issues)
	}
}

func newTestPlanBuilder() planBuilder {
	return planBuilder{
		registry:         builtinIntentRegistry,
		ids:              map[string]int{},
		usedIDs:          map[string]bool{},
		instanceBlockIDs: map[string]string{},
		instanceParams:   map[string]map[string]any{},
		instanceVoltages: map[string]string{},
		regulatorSources: map[string]powerSource{},
		protectedSources: map[string]string{},
		semantic:         newSemanticIndex(),
		supportTargets:   map[string]semanticSupportIntent{},
		i2cBuses:         map[string]string{},
		i2cMCUBus:        map[string]string{},
	}
}
