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

func TestValidatePCBRealizationRejectsUnconditionalWaypointVariant(t *testing.T) {
	definition := minimalRealizationDefinition()
	definition.PCBRealization.LocalRoutes[0].WaypointVariants = []PCBWaypointVariant{{
		Waypoints: []RelativePoint{{XMM: 3, YMM: 2}},
	}}

	issues := ValidatePCBRealization(definition)
	assertIssuePath(t, issues, "block.demo.pcb_realization.local_routes.0.waypoint_variants.0.when")
}

func TestValidatePCBRealizationRejectsInvalidEndpointVariant(t *testing.T) {
	definition := minimalRealizationDefinition()
	definition.PCBRealization.LocalRoutes[0].Waypoints = nil
	definition.PCBRealization.LocalRoutes[0].EndpointVariants = []PCBEndpointVariant{{}}

	issues := ValidatePCBRealization(definition)
	assertIssuePath(t, issues, "block.demo.pcb_realization.local_routes.0.endpoint_variants.0.to_endpoint_dogbone")
	assertIssuePath(t, issues, "block.demo.pcb_realization.local_routes.0.endpoint_variants.0.when")
}

func TestValidatePCBRealizationAcceptsEntryAnchorRoute(t *testing.T) {
	definition := minimalRealizationDefinition()
	definition.PCBRealization.EntryAnchors = []PCBEntryAnchor{{
		ID:          "input_entry",
		Port:        "IN",
		NetTemplate: "SIG",
		Placement:   RelativePlacement{XMM: -1, YMM: 2, Layer: "F.Cu"},
	}}
	definition.PCBRealization.LocalRoutes = append(definition.PCBRealization.LocalRoutes, PCBLocalRoute{
		ID:          "entry_to_resistor",
		NetTemplate: "SIG",
		From:        RouteEndpoint{AnchorID: "input_entry"},
		To:          RouteEndpoint{ComponentRole: "resistor", Pin: "1"},
		Layer:       "F.Cu",
		WidthMM:     0.25,
	})

	issues := ValidatePCBRealization(definition)
	if len(issues) != 0 {
		t.Fatalf("ValidatePCBRealization issues = %#v, want none", issues)
	}
}

func TestValidatePCBRealizationRejectsInvalidEntryAnchors(t *testing.T) {
	definition := minimalRealizationDefinition()
	definition.PCBRealization.EntryAnchors = []PCBEntryAnchor{
		{
			ID:        "input_entry",
			Port:      "IN",
			Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"},
		},
		{
			ID:          "input_entry",
			Port:        "OUT",
			NetTemplate: " SIG",
			Placement:   RelativePlacement{XMM: math.NaN(), YMM: 0, Layer: "Nope.Cu"},
			Side:        "sideways",
			When:        RealizationWhen{Params: map[string]any{"missing": true}},
		},
	}

	issues := ValidatePCBRealization(definition)
	assertIssuePath(t, issues, "block.demo.pcb_realization.entry_anchors.1.id")
	assertIssuePath(t, issues, "block.demo.pcb_realization.entry_anchors.1.port")
	assertIssuePath(t, issues, "block.demo.pcb_realization.entry_anchors.1.net_template")
	assertIssuePath(t, issues, "block.demo.pcb_realization.entry_anchors.1.side")
	assertIssuePath(t, issues, "block.demo.pcb_realization.entry_anchors.1.placement")
	assertIssuePath(t, issues, "block.demo.pcb_realization.entry_anchors.1.placement.layer")
	assertIssuePath(t, issues, "block.demo.pcb_realization.entry_anchors.1.when.params.missing")
}

func TestValidatePCBRealizationRejectsInvalidRouteEndpointModes(t *testing.T) {
	definition := minimalRealizationDefinition()
	definition.PCBRealization.EntryAnchors = []PCBEntryAnchor{{
		ID:        "input_entry",
		Port:      "IN",
		Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"},
	}}
	definition.PCBRealization.LocalRoutes = append(definition.PCBRealization.LocalRoutes,
		PCBLocalRoute{
			ID:          "missing_mode",
			NetTemplate: "SIG",
			From:        RouteEndpoint{},
			To:          RouteEndpoint{ComponentRole: "resistor", Pin: "1"},
			Layer:       "F.Cu",
		},
		PCBLocalRoute{
			ID:          "conflicting_mode",
			NetTemplate: "SIG",
			From:        RouteEndpoint{ComponentRole: "resistor", Pin: "1", AnchorID: "input_entry"},
			To:          RouteEndpoint{ComponentRole: "led", Pin: "A"},
			Layer:       "F.Cu",
		},
		PCBLocalRoute{
			ID:          "unknown_anchor",
			NetTemplate: "SIG",
			From:        RouteEndpoint{AnchorID: "missing"},
			To:          RouteEndpoint{ComponentRole: "led", Pin: "A"},
			Layer:       "F.Cu",
		},
		PCBLocalRoute{
			ID:          "unknown_port",
			NetTemplate: "SIG",
			From:        RouteEndpoint{Port: "OUT"},
			To:          RouteEndpoint{ComponentRole: "led", Pin: "A"},
			Layer:       "F.Cu",
		},
	)

	issues := ValidatePCBRealization(definition)
	assertIssuePath(t, issues, "block.demo.pcb_realization.local_routes.1.from")
	assertIssuePath(t, issues, "block.demo.pcb_realization.local_routes.2.from")
	assertIssuePath(t, issues, "block.demo.pcb_realization.local_routes.3.from.anchor_id")
	assertIssuePath(t, issues, "block.demo.pcb_realization.local_routes.4.from.port")
}

func TestValidatePCBRealizationRejectsInvalidZoneCondition(t *testing.T) {
	definition := minimalRealizationDefinition()
	definition.PCBRealization.Zones[0].When = RealizationWhen{Params: map[string]any{"missing": true}}

	issues := ValidatePCBRealization(definition)
	assertIssuePath(t, issues, "block.demo.pcb_realization.zones.0.when.params.missing")
}

func TestValidatePCBRealizationRejectsAnchorOutsideGroup(t *testing.T) {
	definition := minimalRealizationDefinition()
	definition.PCBRealization.PlacementGroups[0].AnchorRole = "led"
	definition.PCBRealization.PlacementGroups[0].ComponentRoles = []string{"resistor"}

	issues := ValidatePCBRealization(definition)
	assertIssuePath(t, issues, "block.demo.pcb_realization.placement_groups.0.anchor_role")
}

func TestCloneBlockDefinitionClonesPCBRealization(t *testing.T) {
	definition := timingRealizationDefinition()
	definition.PCBRealization.EntryAnchors = []PCBEntryAnchor{{
		ID:        "input_entry",
		Port:      "IN",
		Placement: RelativePlacement{XMM: 0, YMM: 0, Layer: "F.Cu"},
		When:      RealizationWhen{Params: map[string]any{"enabled": true}},
	}}
	definition.PCBRealization.LocalRoutes[0].When = RealizationWhen{Params: map[string]any{"enabled": true}}
	definition.PCBRealization.Zones[0].When = RealizationWhen{Params: map[string]any{"enabled": true}}
	clone := cloneBlockDefinition(definition)
	clone.PCBRealization.Components[0].Properties["k"] = "changed"
	clone.PCBRealization.EntryAnchors[0].When.Params["enabled"] = false
	clone.PCBRealization.LocalRoutes[0].Waypoints[0].XMM = 99
	clone.PCBRealization.LocalRoutes[0].When.Params["enabled"] = false
	clone.PCBRealization.Zones[0].When.Params["enabled"] = false
	fixture := &clone.PCBRealization.TimingFixtures[0]
	if len(fixture.LoadCapacitorRoles) == 0 || len(fixture.DecouplingRoles) == 0 || len(fixture.EnableControlRoles) == 0 {
		t.Fatalf("test fixture missing timing roles: %#v", fixture)
	}
	if fixture.MaxLoadCapDistanceMM == nil || fixture.MaxDecouplingDistanceMM == nil {
		t.Fatalf("test fixture missing timing thresholds: %#v", fixture)
	}
	fixture.LoadCapacitorRoles[0] = "changed"
	fixture.DecouplingRoles[0] = "changed"
	fixture.EnableControlRoles[0] = "changed"
	*fixture.MaxLoadCapDistanceMM = 99
	*fixture.MaxDecouplingDistanceMM = 99
	fixture.Roles["crystal"] = PCBTimingRoleOscillator

	if definition.PCBRealization.Components[0].Properties["k"] != "v" {
		t.Fatalf("component properties were not cloned: %#v", definition.PCBRealization.Components[0].Properties)
	}
	if definition.PCBRealization.EntryAnchors[0].When.Params["enabled"] != true {
		t.Fatalf("entry anchor condition params were not cloned")
	}
	if definition.PCBRealization.LocalRoutes[0].Waypoints[0].XMM == 99 {
		t.Fatalf("route waypoints were not cloned")
	}
	if definition.PCBRealization.LocalRoutes[0].When.Params["enabled"] != true {
		t.Fatalf("route condition params were not cloned")
	}
	if definition.PCBRealization.Zones[0].When.Params["enabled"] != true {
		t.Fatalf("zone condition params were not cloned")
	}
	originalFixture := definition.PCBRealization.TimingFixtures[0]
	if originalFixture.LoadCapacitorRoles[0] == "changed" {
		t.Fatalf("timing load capacitor roles were not cloned")
	}
	if originalFixture.DecouplingRoles[0] == "changed" {
		t.Fatalf("timing decoupling roles were not cloned")
	}
	if originalFixture.EnableControlRoles[0] == "changed" {
		t.Fatalf("timing enable control roles were not cloned")
	}
	if *originalFixture.MaxLoadCapDistanceMM == 99 {
		t.Fatalf("timing threshold pointer was not cloned")
	}
	if *originalFixture.MaxDecouplingDistanceMM == 99 {
		t.Fatalf("timing decoupling threshold pointer was not cloned")
	}
	if originalFixture.Roles["crystal"] != PCBTimingRoleCrystal {
		t.Fatalf("timing role map was not cloned: %#v", originalFixture.Roles)
	}
}

func TestValidatePCBRealizationRejectsInvalidTimingFixture(t *testing.T) {
	definition := timingRealizationDefinition()
	definition.PCBRealization.TimingFixtures = append(definition.PCBRealization.TimingFixtures, PCBTimingFixture{
		ID:                      "bad",
		TimingGroupID:           " bad_group",
		Kind:                    "timer",
		SourceRole:              "missing",
		LoadCapacitorRoles:      []string{" missing_cap"},
		DecouplingRoles:         []string{" missing_decoupling"},
		EnableControlRoles:      []string{" missing_enable"},
		GroundNetTemplate:       " gnd",
		ClockNetTemplates:       []string{"xtal1", ""},
		LocalRouteIDs:           []string{" missing_route"},
		MaxLoadCapDistanceMM:    floatPtr(math.NaN()),
		MaxDecouplingDistanceMM: floatPtr(math.NaN()),
		PreferredLayer:          "Nope.Cu",
		Roles:                   map[string]PCBTimingRole{"crystal": "bad_role"},
	})

	issues := ValidatePCBRealization(definition)
	assertIssuePath(t, issues, "block.demo.pcb_realization.timing.1.kind")
	assertIssuePath(t, issues, "block.demo.pcb_realization.timing.1.timing_group_id")
	assertIssuePath(t, issues, "block.demo.pcb_realization.timing.1.source_role")
	assertIssuePath(t, issues, "block.demo.pcb_realization.timing.1.load_capacitor_roles.0")
	assertIssuePath(t, issues, "block.demo.pcb_realization.timing.1.decoupling_roles.0")
	assertIssuePath(t, issues, "block.demo.pcb_realization.timing.1.enable_control_roles.0")
	assertIssuePath(t, issues, "block.demo.pcb_realization.timing.1.ground_net_template")
	assertIssuePath(t, issues, "block.demo.pcb_realization.timing.1.clock_net_templates.1")
	assertIssuePath(t, issues, "block.demo.pcb_realization.timing.1.local_route_ids.0")
	assertIssuePath(t, issues, "block.demo.pcb_realization.timing.1.max_load_cap_distance_mm")
	assertIssuePath(t, issues, "block.demo.pcb_realization.timing.1.max_decoupling_distance_mm")
	assertIssuePath(t, issues, "block.demo.pcb_realization.timing.1.preferred_layer")
	assertIssuePath(t, issues, "block.demo.pcb_realization.timing.1.roles.crystal")
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

func timingRealizationDefinition() BlockDefinition {
	definition := minimalRealizationDefinition()
	definition.Components = append(definition.Components,
		BlockComponent{Role: "crystal", RefPrefix: "Y", SymbolID: "Device:Crystal", FootprintID: "Crystal:Crystal_SMD_5032-2Pin_5.0x3.2mm"},
		BlockComponent{Role: "load_capacitor", RefPrefix: "C", SymbolID: "Device:C", FootprintID: "Capacitor_SMD:C_0603_1608Metric"},
	)
	definition.PCBRealization.LocalRoutes = append(definition.PCBRealization.LocalRoutes, PCBLocalRoute{
		ID:          "xtal_load",
		NetTemplate: "XTAL1",
		From:        RouteEndpoint{ComponentRole: "crystal", Pin: "1"},
		To:          RouteEndpoint{ComponentRole: "load_capacitor", Pin: "1"},
		Layer:       "F.Cu",
		WidthMM:     0.2,
	})
	definition.PCBRealization.TimingFixtures = []PCBTimingFixture{{
		ID:                      "demo_timing",
		TimingGroupID:           "demo_clock",
		Kind:                    "crystal",
		SourceRole:              "crystal",
		LoadCapacitorRoles:      []string{"load_capacitor"},
		DecouplingRoles:         []string{"resistor"},
		EnableControlRoles:      []string{"led"},
		GroundNetTemplate:       "GND",
		ClockNetTemplates:       []string{"XTAL1"},
		LocalRouteIDs:           []string{"xtal_load"},
		MaxLoadCapDistanceMM:    floatPtr(8),
		MaxDecouplingDistanceMM: floatPtr(8),
		MaxLoadCapAsymmetryMM:   floatPtr(1),
		PreferredLayer:          "F.Cu",
		Roles:                   map[string]PCBTimingRole{"crystal": PCBTimingRoleCrystal, "load_capacitor": PCBTimingRoleLoadCapacitor, "resistor": PCBTimingRoleDecoupling, "led": PCBTimingRoleEnableControl},
	}}
	return definition
}

func floatPtr(value float64) *float64 {
	return &value
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
