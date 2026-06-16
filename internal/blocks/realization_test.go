package blocks

import (
	"math"
	"testing"

	"kicadai/internal/reports"
)

func TestValidatePCBRealizationAcceptsValidRealization(t *testing.T) {
	definition := minimalRealizationDefinition()
	issues := ValidatePCBRealization(definition)
	if len(issues) != 0 {
		t.Fatalf("ValidatePCBRealization issues = %#v, want none", issues)
	}
}

func TestValidatePCBRealizationRejectsUnknownRoleAndLayer(t *testing.T) {
	definition := minimalRealizationDefinition()
	definition.PCBRealization.Components[0].ComponentRole = "missing"
	definition.PCBRealization.Components[0].FootprintParam = "missing_footprint"
	definition.PCBRealization.Components[0].Placement.Layer = "Nope.Cu"
	definition.PCBRealization.VerificationLevel = "made_up"

	issues := ValidatePCBRealization(definition)
	if len(issues) < 2 {
		t.Fatalf("ValidatePCBRealization issues = %#v, want role and layer failures", issues)
	}
	assertIssuePath(t, issues, "block.demo.pcb_realization.verification_level")
	assertIssuePath(t, issues, "block.demo.pcb_realization.components.0.component_role")
	assertIssuePath(t, issues, "block.demo.pcb_realization.components.0.footprint_param")
	assertIssuePath(t, issues, "block.demo.pcb_realization.components.0.placement.layer")
}

func TestValidatePCBRealizationRejectsInvalidRoute(t *testing.T) {
	definition := minimalRealizationDefinition()
	definition.PCBRealization.LocalRoutes = append(definition.PCBRealization.LocalRoutes, PCBLocalRoute{
		ID:          "sig",
		NetTemplate: "",
		From:        RouteEndpoint{ComponentRole: "resistor", Pin: "1"},
		To:          RouteEndpoint{ComponentRole: "missing", Pin: ""},
		Layer:       "F.Cu",
		WidthMM:     math.NaN(),
	})

	issues := ValidatePCBRealization(definition)
	assertIssuePath(t, issues, "block.demo.pcb_realization.local_routes.1.net_template")
	assertIssuePath(t, issues, "block.demo.pcb_realization.local_routes.1.to.component_role")
	assertIssuePath(t, issues, "block.demo.pcb_realization.local_routes.1.to.pin")
	assertIssuePath(t, issues, "block.demo.pcb_realization.local_routes.1.width_mm")
}

func TestValidatePCBRealizationRejectsAnchorOutsideGroup(t *testing.T) {
	definition := minimalRealizationDefinition()
	definition.PCBRealization.PlacementGroups[0].AnchorRole = "led"
	definition.PCBRealization.PlacementGroups[0].ComponentRoles = []string{"resistor"}

	issues := ValidatePCBRealization(definition)
	assertIssuePath(t, issues, "block.demo.pcb_realization.placement_groups.0.anchor_role")
}

func TestCloneBlockDefinitionClonesPCBRealization(t *testing.T) {
	definition := minimalRealizationDefinition()
	clone := cloneBlockDefinition(definition)
	clone.PCBRealization.Components[0].Properties["k"] = "changed"
	clone.PCBRealization.LocalRoutes[0].Waypoints[0].XMM = 99

	if definition.PCBRealization.Components[0].Properties["k"] != "v" {
		t.Fatalf("component properties were not cloned: %#v", definition.PCBRealization.Components[0].Properties)
	}
	if definition.PCBRealization.LocalRoutes[0].Waypoints[0].XMM == 99 {
		t.Fatalf("route waypoints were not cloned")
	}
}

func minimalRealizationDefinition() BlockDefinition {
	return BlockDefinition{
		ID:      "demo",
		Name:    "Demo",
		Version: "0.1.0",
		Parameters: []BlockParameter{{
			Name:    "footprint",
			Type:    ParameterFootprintID,
			Default: "Device:R_0805",
		}},
		Ports: []BlockPort{{Name: "IN"}},
		Components: []BlockComponent{
			{Role: "resistor", RefPrefix: "R", SymbolID: "Device:R", FootprintID: "Device:R_0805"},
			{Role: "led", RefPrefix: "D", SymbolID: "Device:LED", FootprintID: "LED:D_0805"},
		},
		Verification: VerificationRecord{Level: VerificationStructural},
		PCBRealization: &PCBRealization{
			Version:           "0.1.0",
			VerificationLevel: PCBVerificationPlacementVerified,
			Components: []PCBComponentRealization{{
				ComponentRole:  "resistor",
				FootprintParam: "footprint",
				Placement:      RelativePlacement{XMM: 1, YMM: 2, Layer: "F.Cu"},
				Properties:     map[string]string{"k": "v"},
			}},
			PlacementGroups: []PCBPlacementGroup{{
				ID:             "inline",
				ComponentRoles: []string{"resistor", "led"},
				AnchorRole:     "resistor",
				Bounds:         &RelativeBounds{MinXMM: 0, MinYMM: 0, MaxXMM: 10, MaxYMM: 5},
			}},
			LocalRoutes: []PCBLocalRoute{{
				ID:          "sig",
				NetTemplate: "SIG",
				From:        RouteEndpoint{ComponentRole: "resistor", Pin: "1"},
				To:          RouteEndpoint{ComponentRole: "led", Pin: "A"},
				Layer:       "F.Cu",
				WidthMM:     0.25,
				Waypoints:   []RelativePoint{{XMM: 2, YMM: 2}},
			}},
			Zones: []PCBZoneRealization{{
				ID:          "gnd",
				NetTemplate: "GND",
				Layer:       "F.Cu",
				Points:      []RelativePoint{{XMM: 0, YMM: 0}, {XMM: 5, YMM: 0}, {XMM: 5, YMM: 5}},
			}},
			Keepouts: []PCBKeepout{{
				ID:        "antenna",
				Layer:     "F.Cu",
				Bounds:    RelativeBounds{MinXMM: 0, MinYMM: 0, MaxXMM: 1, MaxYMM: 1},
				AppliesTo: []string{"copper"},
			}},
			Constraints: []PCBConstraint{{
				ID:          "sig_width",
				Kind:        "min_width",
				NetTemplate: "SIG",
				MinWidthMM:  0.2,
			}},
		},
	}
}

func assertIssuePath(t *testing.T, issues []reports.Issue, path string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Path == path {
			return
		}
	}
	t.Fatalf("missing issue path %q in %#v", path, issues)
}
