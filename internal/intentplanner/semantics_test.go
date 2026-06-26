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
	}
}
