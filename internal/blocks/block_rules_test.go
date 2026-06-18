package blocks

import (
	"context"
	"slices"
	"testing"
)

func TestLEDIndicatorDefinitionDeclaresHardeningRules(t *testing.T) {
	definition := ledIndicatorDefinition()
	rules := validationRuleIDs(definition.ValidationRules)
	for _, id := range []string{
		"led.current.positive",
		"led.forward_voltage.below_supply",
		"led.polarity.evidence",
		"led.resistor.required",
		"led.series_route.required",
	} {
		if !slices.Contains(rules, id) {
			t.Errorf("LED validation rules = %#v, missing %s", rules, id)
		}
	}
	if definition.Verification.Level != VerificationStructural {
		t.Fatalf("LED verification = %q", definition.Verification.Level)
	}
}

func TestConnectorBreakoutDefinitionDeclaresHardeningRules(t *testing.T) {
	definition := connectorBreakoutDefinition()
	rules := validationRuleIDs(definition.ValidationRules)
	for _, id := range []string{
		"connector.pin_names.required",
		"connector.pin_count.matches_names",
		"connector.pin_numbering.evidence",
		"connector.symbol.resolved",
		"connector.footprint.resolved",
	} {
		if !slices.Contains(rules, id) {
			t.Errorf("connector validation rules = %#v, missing %s", rules, id)
		}
	}
	if definition.Verification.Level != VerificationStructural {
		t.Fatalf("connector verification = %q", definition.Verification.Level)
	}
}

func TestConnectorBreakoutFailsClosedOnDuplicatePins(t *testing.T) {
	output, issues := NewBuiltinRegistry().Instantiate(context.Background(), BlockRequest{
		BlockID:    "connector_breakout",
		InstanceID: "JX",
		Params: map[string]any{
			"pin_names": []string{"VIN", "VIN"},
		},
	})
	if len(issues) == 0 || !hasBlockingIssues(issues) {
		t.Fatalf("issues = %#v output = %#v", issues, output)
	}
}

func TestVoltageRegulatorDefinitionDeclaresPowerRules(t *testing.T) {
	definition := voltageRegulatorDefinition()
	rules := validationRuleIDs(definition.ValidationRules)
	for _, id := range []string{
		"regulator.rail.input_above_output",
		"regulator.dropout.margin",
		"regulator.current.rating",
		"regulator.enable.handled",
		"regulator.input_capacitor.required",
		"regulator.output_capacitor.required",
		"regulator.capacitor.proximity",
	} {
		if !slices.Contains(rules, id) {
			t.Errorf("regulator validation rules = %#v, missing %s", rules, id)
		}
	}
	constraints := pcbConstraintIDs(definition.PCBRealization)
	for _, id := range []string{"regulator_power_width", "regulator_output_width", "regulator_input_capacitor_proximity", "regulator_output_capacitor_proximity"} {
		if !slices.Contains(constraints, id) {
			t.Errorf("regulator PCB constraints = %#v, missing %s", constraints, id)
		}
	}
}

func TestUSBCPowerDefinitionDeclaresPowerRules(t *testing.T) {
	definition := usbCPowerDefinition()
	rules := validationRuleIDs(definition.ValidationRules)
	for _, id := range []string{
		"usb_c.power_only.required",
		"usb_c.cc_pull_downs.required",
		"usb_c.connector.pinmap",
		"usb_c.edge_placement.required",
		"usb_c.vbus_route.required",
		"usb_c.protection.companions",
	} {
		if !slices.Contains(rules, id) {
			t.Errorf("USB-C validation rules = %#v, missing %s", rules, id)
		}
	}
	constraints := pcbConstraintIDs(definition.PCBRealization)
	for _, id := range []string{"usb_c_vbus_width", "usb_c_edge_facing"} {
		if !slices.Contains(constraints, id) {
			t.Errorf("USB-C PCB constraints = %#v, missing %s", constraints, id)
		}
	}
	if definition.PCBRealization == nil {
		t.Fatal("USB-C block should define PCB realization")
	}
	if len(definition.PCBRealization.Keepouts) == 0 {
		t.Fatalf("USB-C block should define edge keepout metadata")
	}
}

func TestI2CSensorDefinitionDeclaresBusRules(t *testing.T) {
	definition := i2cSensorDefinition()
	rules := validationRuleIDs(definition.ValidationRules)
	for _, id := range []string{
		"i2c.address.valid",
		"i2c.rail.compatible",
		"i2c.decoupling.required",
		"i2c.pullups.owned_or_external",
		"i2c.pullups.no_duplicate",
		"i2c.sensor.pinmap",
	} {
		if !slices.Contains(rules, id) {
			t.Errorf("I2C validation rules = %#v, missing %s", rules, id)
		}
	}
	constraints := pcbConstraintIDs(definition.PCBRealization)
	for _, id := range []string{"i2c_decoupling_proximity", "i2c_bus_pullup_group"} {
		if !slices.Contains(constraints, id) {
			t.Errorf("I2C PCB constraints = %#v, missing %s", constraints, id)
		}
	}
	assertRequiredRoutesDefined(t, definition.PCBRealization)
}

func TestOpAmpGainStageDefinitionDeclaresAnalogRules(t *testing.T) {
	definition := opampGainStageDefinition()
	rules := validationRuleIDs(definition.ValidationRules)
	for _, id := range []string{
		"opamp.topology.supported",
		"opamp.gain.valid",
		"opamp.supply.compatible",
		"opamp.bias.required",
		"opamp.feedback.proximity",
		"opamp.feedback.route.required",
	} {
		if !slices.Contains(rules, id) {
			t.Errorf("op-amp validation rules = %#v, missing %s", rules, id)
		}
	}
	constraints := pcbConstraintIDs(definition.PCBRealization)
	for _, id := range []string{"opamp_feedback_proximity", "opamp_supply_decoupling_proximity"} {
		if !slices.Contains(constraints, id) {
			t.Errorf("op-amp PCB constraints = %#v, missing %s", constraints, id)
		}
	}
	assertRequiredRoutesDefined(t, definition.PCBRealization)
}

func validationRuleIDs(rules []BlockValidationRule) []string {
	ids := make([]string, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ID)
	}
	slices.Sort(ids)
	return ids
}

func pcbConstraintIDs(realization *PCBRealization) []string {
	if realization == nil {
		return nil
	}
	ids := make([]string, 0, len(realization.Constraints))
	for _, constraint := range realization.Constraints {
		ids = append(ids, constraint.ID)
	}
	slices.Sort(ids)
	return ids
}

func assertRequiredRoutesDefined(t *testing.T, realization *PCBRealization) {
	t.Helper()
	if realization == nil {
		t.Fatal("PCB realization is required")
	}
	defined := make(map[string]struct{}, len(realization.LocalRoutes))
	for _, route := range realization.LocalRoutes {
		defined[route.ID] = struct{}{}
	}
	for _, routeID := range realization.Validation.RequiredRoutes {
		if _, ok := defined[routeID]; !ok {
			t.Errorf("required route %s has no local route definition", routeID)
		}
	}
}
