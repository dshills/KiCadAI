package blocks

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

var realizationPathSegmentReplacer = strings.NewReplacer(".", "_", " ", "_", "/", "_", "\\", "_", ":", "_")

type PCBVerificationLevel string

const (
	PCBVerificationUnrealized           PCBVerificationLevel = "pcb_unrealized"
	PCBVerificationPlacementVerified    PCBVerificationLevel = "pcb_placement_verified"
	PCBVerificationConnectivityVerified PCBVerificationLevel = "pcb_connectivity_verified"
	PCBVerificationDRCVerified          PCBVerificationLevel = "pcb_drc_verified"
	PCBVerificationReferenceVerified    PCBVerificationLevel = "pcb_reference_verified"
)

type PCBRealization struct {
	Version                 string                    `json:"version"`
	VerificationLevel       PCBVerificationLevel      `json:"verification_level"`
	Components              []PCBComponentRealization `json:"components,omitempty"`
	EntryAnchors            []PCBEntryAnchor          `json:"entry_anchors,omitempty"`
	PlacementGroups         []PCBPlacementGroup       `json:"placement_groups,omitempty"`
	LocalRoutes             []PCBLocalRoute           `json:"local_routes,omitempty"`
	TimingFixtures          []PCBTimingFixture        `json:"timing,omitempty"`
	Zones                   []PCBZoneRealization      `json:"zones,omitempty"`
	Keepouts                []PCBKeepout              `json:"keepouts,omitempty"`
	Constraints             []PCBConstraint           `json:"constraints,omitempty"`
	InterBlockObstaclePorts []string                  `json:"inter_block_obstacle_ports,omitempty"`
	Validation              PCBValidationExpectations `json:"validation,omitempty"`
	FabricationReadiness    bool                      `json:"fabrication_readiness,omitempty"`
	UnsupportedBehaviors    []string                  `json:"unsupported_behaviors,omitempty"`
}

type PCBComponentRealization struct {
	ComponentRole  string                 `json:"component_role"`
	FootprintParam string                 `json:"footprint_param,omitempty"`
	FootprintID    string                 `json:"footprint_id,omitempty"`
	PhysicalPins   []transactions.PinSpec `json:"physical_pins,omitempty"`
	Placement      RelativePlacement      `json:"placement"`
	Side           string                 `json:"side,omitempty"`
	Properties     map[string]string      `json:"properties,omitempty"`
	When           RealizationWhen        `json:"when,omitempty"`
}

type RealizationWhen struct {
	Params map[string]any `json:"params,omitempty"`
}

type RelativePlacement struct {
	XMM         float64 `json:"x_mm"`
	YMM         float64 `json:"y_mm"`
	RotationDeg float64 `json:"rotation_deg,omitempty"`
	Layer       string  `json:"layer,omitempty"`
	Fixed       bool    `json:"fixed,omitempty"`
}

type PCBPlacementGroup struct {
	ID              string          `json:"id"`
	ComponentRoles  []string        `json:"component_roles"`
	AnchorRole      string          `json:"anchor_role,omitempty"`
	Bounds          *RelativeBounds `json:"bounds,omitempty"`
	TranslateAsUnit bool            `json:"translate_as_unit,omitempty"`
	Description     string          `json:"description,omitempty"`
}

type PCBEntryAnchor struct {
	ID          string                      `json:"id"`
	Port        string                      `json:"port"`
	NetTemplate string                      `json:"net_template,omitempty"`
	Placement   RelativePlacement           `json:"placement"`
	Variants    []PCBAnchorPlacementVariant `json:"placement_variants,omitempty"`
	Side        string                      `json:"side,omitempty"`
	Description string                      `json:"description,omitempty"`
	When        RealizationWhen             `json:"when,omitempty"`
	OmitWhen    []RealizationWhen           `json:"omit_when,omitempty"`
}

type PCBAnchorPlacementVariant struct {
	Placement RelativePlacement `json:"placement"`
	When      RealizationWhen   `json:"when"`
}

type RelativeBounds struct {
	MinXMM float64 `json:"min_x_mm"`
	MinYMM float64 `json:"min_y_mm"`
	MaxXMM float64 `json:"max_x_mm"`
	MaxYMM float64 `json:"max_y_mm"`
}

type PCBLocalRoute struct {
	ID                    string                    `json:"id"`
	NetTemplate           string                    `json:"net_template"`
	From                  RouteEndpoint             `json:"from"`
	To                    RouteEndpoint             `json:"to"`
	Waypoints             []RelativePoint           `json:"waypoints,omitempty"`
	WaypointVariants      []PCBWaypointVariant      `json:"waypoint_variants,omitempty"`
	EndpointVariants      []PCBEndpointVariant      `json:"endpoint_variants,omitempty"`
	GeometryVariants      []PCBRouteGeometryVariant `json:"geometry_variants,omitempty"`
	FromEndpointDogbone   bool                      `json:"from_endpoint_dogbone,omitempty"`
	ToEndpointDogbone     bool                      `json:"to_endpoint_dogbone,omitempty"`
	Layer                 string                    `json:"layer,omitempty"`
	WidthMM               float64                   `json:"width_mm,omitempty"`
	Required              bool                      `json:"required,omitempty"`
	EntryAnchorDogbone    *PCBEntryAnchorDogbone    `json:"entry_anchor_dogbone,omitempty"`
	DisableEntryAnchorVia bool                      `json:"disable_entry_anchor_via,omitempty"`
	Disabled              bool                      `json:"disabled,omitempty"`
	Description           string                    `json:"description,omitempty"`
	When                  RealizationWhen           `json:"when,omitempty"`
}

type PCBWaypointVariant struct {
	Waypoints []RelativePoint `json:"waypoints"`
	When      RealizationWhen `json:"when"`
}

type PCBEndpointVariant struct {
	FromEndpointDogbone bool            `json:"from_endpoint_dogbone,omitempty"`
	ToEndpointDogbone   bool            `json:"to_endpoint_dogbone,omitempty"`
	When                RealizationWhen `json:"when"`
}

type PCBRouteGeometryVariant struct {
	Waypoints                 []RelativePoint `json:"waypoints,omitempty"`
	ClearWaypoints            bool            `json:"clear_waypoints,omitempty"`
	Layer                     string          `json:"layer,omitempty"`
	DisableEntryAnchorDogbone bool            `json:"disable_entry_anchor_dogbone,omitempty"`
	DisableEntryAnchorVia     bool            `json:"disable_entry_anchor_via,omitempty"`
	DisableRoute              bool            `json:"disable_route,omitempty"`
	When                      RealizationWhen `json:"when"`
}

type PCBEntryAnchorDogbone struct {
	TieOffset   RelativePoint `json:"tie_offset"`
	Description string        `json:"description,omitempty"`
}

type PCBTimingRole string

const (
	PCBTimingKindCrystal    = "crystal"
	PCBTimingKindOscillator = "oscillator"
	PCBTimingKindReset      = "reset_programming"

	PCBTimingRoleCrystal           PCBTimingRole = "crystal"
	PCBTimingRoleOscillator        PCBTimingRole = "oscillator"
	PCBTimingRoleConsumer          PCBTimingRole = "clock_consumer"
	PCBTimingRoleLoadCapacitor     PCBTimingRole = "load_capacitor"
	PCBTimingRoleDecoupling        PCBTimingRole = "decoupling"
	PCBTimingRoleEnableControl     PCBTimingRole = "enable_control"
	PCBTimingRoleGroundReturn      PCBTimingRole = "ground_return"
	PCBTimingRoleResetPullup       PCBTimingRole = "reset_pullup"
	PCBTimingRoleProgrammingHeader PCBTimingRole = "programming_header"
)

type PCBTimingFixture struct {
	ID                            string                   `json:"id"`
	TimingGroupID                 string                   `json:"timing_group_id,omitempty"`
	Kind                          string                   `json:"kind"`
	SourceRole                    string                   `json:"source_role"`
	ConsumerRole                  string                   `json:"consumer_role,omitempty"`
	LoadCapacitorRoles            []string                 `json:"load_capacitor_roles,omitempty"`
	DecouplingRoles               []string                 `json:"decoupling_roles,omitempty"`
	EnableControlRoles            []string                 `json:"enable_control_roles,omitempty"`
	GroundNetTemplate             string                   `json:"ground_net_template,omitempty"`
	ClockNetTemplates             []string                 `json:"clock_net_templates,omitempty"`
	LocalRouteIDs                 []string                 `json:"local_route_ids,omitempty"`
	MaxSourceToConsumerDistanceMM *float64                 `json:"max_source_to_consumer_distance_mm,omitempty"`
	MaxLoadCapDistanceMM          *float64                 `json:"max_load_cap_distance_mm,omitempty"`
	MaxDecouplingDistanceMM       *float64                 `json:"max_decoupling_distance_mm,omitempty"`
	MaxLoadCapAsymmetryMM         *float64                 `json:"max_load_cap_asymmetry_mm,omitempty"`
	MaxClockRouteLengthMM         *float64                 `json:"max_clock_route_length_mm,omitempty"`
	MinNoiseKeepoutMM             *float64                 `json:"min_noise_keepout_mm,omitempty"`
	PreferredLayer                string                   `json:"preferred_layer,omitempty"`
	Roles                         map[string]PCBTimingRole `json:"roles,omitempty"`
	Description                   string                   `json:"description,omitempty"`
	When                          RealizationWhen          `json:"when,omitempty"`
}

type RouteEndpoint struct {
	ComponentRole string `json:"component_role,omitempty"`
	Pin           string `json:"pin,omitempty"`
	AnchorID      string `json:"anchor_id,omitempty"`
	Port          string `json:"port,omitempty"`
}

type RelativePoint struct {
	XMM float64 `json:"x_mm"`
	YMM float64 `json:"y_mm"`
}

type PCBZoneRealization struct {
	ID           string          `json:"id"`
	NetTemplate  string          `json:"net_template"`
	Layer        string          `json:"layer"`
	Priority     int             `json:"priority,omitempty"`
	Points       []RelativePoint `json:"points"`
	ThermalGapMM float64         `json:"thermal_gap_mm,omitempty"`
	Description  string          `json:"description,omitempty"`
	When         RealizationWhen `json:"when,omitempty"`
}

type PCBKeepout struct {
	ID               string         `json:"id"`
	Layer            string         `json:"layer"`
	Bounds           RelativeBounds `json:"bounds"`
	PlacementGroupID string         `json:"placement_group_id,omitempty"`
	AppliesTo        []string       `json:"applies_to,omitempty"`
	BlocksRoute      *bool          `json:"blocks_route,omitempty"`
	Description      string         `json:"description,omitempty"`
}

type PCBConstraint struct {
	ID          string                `json:"id"`
	Kind        string                `json:"kind"`
	Category    PCBConstraintCategory `json:"category,omitempty"`
	NetTemplate string                `json:"net_template,omitempty"`
	AppliesTo   []string              `json:"applies_to,omitempty"`
	MinWidthMM  float64               `json:"min_width_mm,omitempty"`
	ClearanceMM float64               `json:"clearance_mm,omitempty"`
	MaxLengthMM float64               `json:"max_length_mm,omitempty"`
	Description string                `json:"description,omitempty"`
	When        RealizationWhen       `json:"when,omitempty"`
}

type PCBConstraintCategory string

const (
	PCBConstraintAnalogInputSeparation PCBConstraintCategory = "analog_input_separation"
	PCBConstraintReturnTopology        PCBConstraintCategory = "return_topology"
	PCBConstraintFeedbackSense         PCBConstraintCategory = "feedback_sense"
	PCBConstraintDecouplingProximity   PCBConstraintCategory = "decoupling_proximity"
	PCBConstraintCurrentPath           PCBConstraintCategory = "current_path"
	PCBConstraintKelvinSense           PCBConstraintCategory = "kelvin_sense"
	PCBConstraintThermalCoupling       PCBConstraintCategory = "thermal_coupling"
	PCBConstraintDeviceSymmetry        PCBConstraintCategory = "device_symmetry"
	PCBConstraintThermalKeepout        PCBConstraintCategory = "thermal_keepout"
	PCBConstraintPolarizedOrientation  PCBConstraintCategory = "polarized_orientation"
)

type PCBValidationExpectations struct {
	RequiredNets        []string `json:"required_nets,omitempty"`
	RequiredRoutes      []string `json:"required_routes,omitempty"`
	RequiredZones       []string `json:"required_zones,omitempty"`
	AllowedUnroutedNets []string `json:"allowed_unrouted_nets,omitempty"`
	RequiresDRC         bool     `json:"requires_drc,omitempty"`
}

func ValidatePCBRealization(definition BlockDefinition) []reports.Issue {
	realization := definition.PCBRealization
	if realization == nil {
		return nil
	}
	path := "block." + definition.ID + ".pcb_realization"
	var issues []reports.Issue
	if strings.TrimSpace(realization.Version) == "" {
		issues = append(issues, blockIssue(path+".version", "PCB realization version is required"))
	}
	if realization.VerificationLevel == "" {
		issues = append(issues, blockIssue(path+".verification_level", "PCB verification level is required"))
	} else if !validPCBVerificationLevel(realization.VerificationLevel) {
		issues = append(issues, blockIssue(path+".verification_level", "unsupported PCB verification level "+string(realization.VerificationLevel)))
	}
	roles := componentRoleSet(definition.Components)
	ports := portNameSet(definition.Ports)
	parameters := parameterNameSet(definition.Parameters)
	for index, component := range realization.Components {
		componentPath := fmt.Sprintf("%s.components.%d", path, index)
		issues = append(issues, validateKnownRole(componentPath+".component_role", component.ComponentRole, roles)...)
		if strings.TrimSpace(component.FootprintParam) == "" && strings.TrimSpace(component.FootprintID) == "" {
			issues = append(issues, blockIssue(componentPath+".footprint", "component realization requires footprint_param or footprint_id"))
		}
		if component.FootprintParam != "" {
			if _, ok := parameters[strings.TrimSpace(component.FootprintParam)]; !ok {
				issues = append(issues, blockIssue(componentPath+".footprint_param", "unknown footprint parameter "+component.FootprintParam))
			}
		}
		issues = append(issues, validateRealizationWhen(componentPath+".when", component.When, parameters)...)
		issues = append(issues, validateRelativePlacement(componentPath+".placement", component.Placement)...)
	}
	groupIDs := map[string]struct{}{}
	for index, group := range realization.PlacementGroups {
		groupPath := fmt.Sprintf("%s.placement_groups.%d", path, index)
		id := strings.TrimSpace(group.ID)
		if id == "" {
			issues = append(issues, blockIssue(groupPath+".id", "placement group ID is required"))
		} else if _, exists := groupIDs[id]; exists {
			issues = append(issues, blockIssue(groupPath+".id", "duplicate placement group ID "+id))
		}
		groupIDs[id] = struct{}{}
		for roleIndex, role := range group.ComponentRoles {
			issues = append(issues, validateKnownRole(fmt.Sprintf("%s.component_roles.%d", groupPath, roleIndex), role, roles)...)
		}
		if group.AnchorRole != "" {
			issues = append(issues, validateKnownRole(groupPath+".anchor_role", group.AnchorRole, roles)...)
			if !stringInSlice(group.AnchorRole, group.ComponentRoles) {
				issues = append(issues, blockIssue(groupPath+".anchor_role", "anchor role must be included in component_roles"))
			}
		}
		if group.Bounds != nil {
			issues = append(issues, validateBounds(groupPath+".bounds", *group.Bounds)...)
		}
	}
	anchorIDs := map[string]struct{}{}
	for index, anchor := range realization.EntryAnchors {
		anchorPath := fmt.Sprintf("%s.entry_anchors.%d", path, index)
		id := strings.TrimSpace(anchor.ID)
		validID := false
		if id == "" {
			issues = append(issues, blockIssue(anchorPath+".id", "entry anchor ID is required"))
		} else if id != anchor.ID {
			issues = append(issues, blockIssue(anchorPath+".id", "entry anchor ID must not contain leading or trailing whitespace"))
		} else if _, exists := anchorIDs[id]; exists {
			issues = append(issues, blockIssue(anchorPath+".id", "duplicate entry anchor ID "+id))
		} else {
			validID = true
		}
		if validID {
			anchorIDs[id] = struct{}{}
		}
		port := strings.TrimSpace(anchor.Port)
		if port == "" {
			issues = append(issues, blockIssue(anchorPath+".port", "entry anchor port is required"))
		} else if port != anchor.Port {
			issues = append(issues, blockIssue(anchorPath+".port", "entry anchor port must not contain leading or trailing whitespace"))
		} else if _, ok := ports[port]; !ok {
			issues = append(issues, blockIssue(anchorPath+".port", "unknown entry anchor port "+port))
		}
		if strings.TrimSpace(anchor.NetTemplate) != anchor.NetTemplate {
			issues = append(issues, blockIssue(anchorPath+".net_template", "entry anchor net template must not contain leading or trailing whitespace"))
		}
		issues = append(issues, validatePCBRealizationSide(anchorPath+".side", anchor.Side)...)
		issues = append(issues, validateRealizationWhen(anchorPath+".when", anchor.When, parameters)...)
		for omitIndex, condition := range anchor.OmitWhen {
			issues = append(issues, validateRealizationWhen(fmt.Sprintf("%s.omit_when.%d", anchorPath, omitIndex), condition, parameters)...)
		}
		issues = append(issues, validateRelativePlacement(anchorPath+".placement", anchor.Placement)...)
		for variantIndex, variant := range anchor.Variants {
			variantPath := fmt.Sprintf("%s.placement_variants.%d", anchorPath, variantIndex)
			issues = append(issues, validateRelativePlacement(variantPath+".placement", variant.Placement)...)
			if len(variant.When.Params) == 0 {
				issues = append(issues, blockIssue(variantPath+".when", "anchor placement variant requires a non-empty condition"))
			}
			issues = append(issues, validateRealizationWhen(variantPath+".when", variant.When, parameters)...)
		}
	}
	routeIDs := map[string]struct{}{}
	for index, route := range realization.LocalRoutes {
		routePath := fmt.Sprintf("%s.local_routes.%d", path, index)
		id := strings.TrimSpace(route.ID)
		if id == "" {
			issues = append(issues, blockIssue(routePath+".id", "local route ID is required"))
		} else if _, exists := routeIDs[id]; exists {
			issues = append(issues, blockIssue(routePath+".id", "duplicate local route ID "+id))
		}
		routeIDs[id] = struct{}{}
		if strings.TrimSpace(route.NetTemplate) == "" {
			issues = append(issues, blockIssue(routePath+".net_template", "local route net template is required"))
		}
		issues = append(issues, validateRouteEndpoint(routePath+".from", route.From, roles, ports, anchorIDs)...)
		issues = append(issues, validateRouteEndpoint(routePath+".to", route.To, roles, ports, anchorIDs)...)
		issues = append(issues, validateLayer(routePath+".layer", route.Layer, true)...)
		if route.WidthMM < 0 || !finite(route.WidthMM) {
			issues = append(issues, blockIssue(routePath+".width_mm", "route width must be finite and non-negative"))
		}
		if route.EntryAnchorDogbone != nil {
			issues = append(issues, validatePoint(routePath+".entry_anchor_dogbone.tie_offset", route.EntryAnchorDogbone.TieOffset)...)
			if zeroNear(route.EntryAnchorDogbone.TieOffset.XMM) == 0 && zeroNear(route.EntryAnchorDogbone.TieOffset.YMM) == 0 {
				issues = append(issues, blockIssue(routePath+".entry_anchor_dogbone.tie_offset", "entry anchor dogbone tie offset must be non-zero"))
			}
		}
		if (route.FromEndpointDogbone || route.ToEndpointDogbone) && len(route.Waypoints) == 0 {
			issues = append(issues, blockIssue(routePath, "endpoint dogbone requires at least one route waypoint"))
		}
		if route.FromEndpointDogbone && route.ToEndpointDogbone && len(route.Waypoints) < 2 {
			issues = append(issues, blockIssue(routePath, "source and destination endpoint dogbones require at least two route waypoints"))
		}
		issues = append(issues, validateRealizationWhen(routePath+".when", route.When, parameters)...)
		for waypointIndex, point := range route.Waypoints {
			issues = append(issues, validatePoint(fmt.Sprintf("%s.waypoints.%d", routePath, waypointIndex), point)...)
		}
		for variantIndex, variant := range route.WaypointVariants {
			variantPath := fmt.Sprintf("%s.waypoint_variants.%d", routePath, variantIndex)
			if len(variant.Waypoints) == 0 {
				issues = append(issues, blockIssue(variantPath+".waypoints", "waypoint variant requires at least one point"))
			}
			if len(variant.When.Params) == 0 {
				issues = append(issues, blockIssue(variantPath+".when", "waypoint variant requires a non-empty condition"))
			}
			for waypointIndex, point := range variant.Waypoints {
				issues = append(issues, validatePoint(fmt.Sprintf("%s.waypoints.%d", variantPath, waypointIndex), point)...)
			}
			issues = append(issues, validateRealizationWhen(variantPath+".when", variant.When, parameters)...)
		}
		for variantIndex, variant := range route.EndpointVariants {
			variantPath := fmt.Sprintf("%s.endpoint_variants.%d", routePath, variantIndex)
			if !variant.FromEndpointDogbone && !variant.ToEndpointDogbone {
				issues = append(issues, blockIssue(variantPath, "endpoint variant must enable a supported behavior"))
			}
			if len(route.Waypoints) == 0 {
				issues = append(issues, blockIssue(variantPath+".to_endpoint_dogbone", "destination endpoint dogbone requires at least one route waypoint"))
			}
			if len(variant.When.Params) == 0 {
				issues = append(issues, blockIssue(variantPath+".when", "endpoint variant requires a non-empty condition"))
			}
			issues = append(issues, validateRealizationWhen(variantPath+".when", variant.When, parameters)...)
		}
		for variantIndex, variant := range route.GeometryVariants {
			variantPath := fmt.Sprintf("%s.geometry_variants.%d", routePath, variantIndex)
			if len(variant.Waypoints) > 0 && variant.ClearWaypoints {
				issues = append(issues, blockIssue(variantPath+".clear_waypoints", "route geometry variant cannot replace and clear waypoints"))
			}
			if len(variant.Waypoints) == 0 && !variant.ClearWaypoints && strings.TrimSpace(variant.Layer) == "" && !variant.DisableEntryAnchorDogbone && !variant.DisableEntryAnchorVia && !variant.DisableRoute {
				issues = append(issues, blockIssue(variantPath, "route geometry variant requires at least one geometry change"))
			}
			issues = append(issues, validateLayer(variantPath+".layer", variant.Layer, true)...)
			for waypointIndex, point := range variant.Waypoints {
				issues = append(issues, validatePoint(fmt.Sprintf("%s.waypoints.%d", variantPath, waypointIndex), point)...)
			}
			if len(variant.When.Params) == 0 {
				issues = append(issues, blockIssue(variantPath+".when", "route geometry variant requires a non-empty condition"))
			}
			issues = append(issues, validateRealizationWhen(variantPath+".when", variant.When, parameters)...)
		}
	}
	timingIDs := make(map[string]struct{})
	for index, timing := range realization.TimingFixtures {
		timingPath := fmt.Sprintf("%s.timing.%d", path, index)
		id := strings.TrimSpace(timing.ID)
		if id == "" {
			issues = append(issues, blockIssue(timingPath+".id", "timing fixture ID is required"))
		} else if id != timing.ID {
			issues = append(issues, blockIssue(timingPath+".id", "timing fixture ID must not contain leading or trailing whitespace"))
		} else if _, exists := timingIDs[id]; exists {
			issues = append(issues, blockIssue(timingPath+".id", fmt.Sprintf("duplicate timing fixture ID %q", id)))
		} else {
			timingIDs[id] = struct{}{}
		}
		kind := strings.TrimSpace(timing.Kind)
		if kind == "" {
			issues = append(issues, blockIssue(timingPath+".kind", "timing fixture kind is required"))
		} else if kind != timing.Kind {
			issues = append(issues, blockIssue(timingPath+".kind", "timing fixture kind must not contain leading or trailing whitespace"))
		} else if !validPCBTimingKind(kind) {
			issues = append(issues, blockIssue(timingPath+".kind", "unsupported timing fixture kind "+kind))
		}
		if groupID := strings.TrimSpace(timing.TimingGroupID); groupID != timing.TimingGroupID {
			issues = append(issues, blockIssue(timingPath+".timing_group_id", "timing group ID must not contain leading or trailing whitespace"))
		}
		sourceRole := strings.TrimSpace(timing.SourceRole)
		if sourceRole == "" {
			issues = append(issues, blockIssue(timingPath+".source_role", "timing fixture source role is required"))
		} else if sourceRole != timing.SourceRole {
			issues = append(issues, blockIssue(timingPath+".source_role", "source role must not contain leading or trailing whitespace"))
		}
		if sourceRole != "" {
			issues = append(issues, validateKnownRole(timingPath+".source_role", sourceRole, roles)...)
		}
		consumerRole := strings.TrimSpace(timing.ConsumerRole)
		if timing.ConsumerRole != "" && consumerRole != timing.ConsumerRole {
			issues = append(issues, blockIssue(timingPath+".consumer_role", "consumer role must not contain leading or trailing whitespace"))
		}
		if consumerRole != "" {
			issues = append(issues, validateKnownRole(timingPath+".consumer_role", consumerRole, roles)...)
		}
		if strings.TrimSpace(timing.GroundNetTemplate) != timing.GroundNetTemplate {
			issues = append(issues, blockIssue(timingPath+".ground_net_template", "ground net template must not contain leading or trailing whitespace"))
		}
		for netIndex, netTemplate := range timing.ClockNetTemplates {
			if strings.TrimSpace(netTemplate) == "" {
				issues = append(issues, blockIssue(fmt.Sprintf("%s.clock_net_templates.%d", timingPath, netIndex), "clock net template is required"))
			} else if strings.TrimSpace(netTemplate) != netTemplate {
				issues = append(issues, blockIssue(fmt.Sprintf("%s.clock_net_templates.%d", timingPath, netIndex), "clock net template must not contain leading or trailing whitespace"))
			}
		}
		for roleIndex, role := range timing.LoadCapacitorRoles {
			rolePath := fmt.Sprintf("%s.load_capacitor_roles.%d", timingPath, roleIndex)
			trimmedRole := strings.TrimSpace(role)
			if trimmedRole != role {
				issues = append(issues, blockIssue(rolePath, "load capacitor role must not contain leading or trailing whitespace"))
			}
			issues = append(issues, validateKnownRole(rolePath, trimmedRole, roles)...)
		}
		for roleIndex, role := range timing.DecouplingRoles {
			rolePath := fmt.Sprintf("%s.decoupling_roles.%d", timingPath, roleIndex)
			trimmedRole := strings.TrimSpace(role)
			if trimmedRole != role {
				issues = append(issues, blockIssue(rolePath, "decoupling role must not contain leading or trailing whitespace"))
			}
			issues = append(issues, validateKnownRole(rolePath, trimmedRole, roles)...)
		}
		for roleIndex, role := range timing.EnableControlRoles {
			rolePath := fmt.Sprintf("%s.enable_control_roles.%d", timingPath, roleIndex)
			trimmedRole := strings.TrimSpace(role)
			if trimmedRole != role {
				issues = append(issues, blockIssue(rolePath, "enable control role must not contain leading or trailing whitespace"))
			}
			issues = append(issues, validateKnownRole(rolePath, trimmedRole, roles)...)
		}
		for roleName, timingRole := range timing.Roles {
			issues = append(issues, validateKnownRole(timingPath+".roles."+roleName, roleName, roles)...)
			if !validPCBTimingRole(timingRole) {
				issues = append(issues, blockIssue(timingPath+".roles."+roleName, "unsupported timing role "+string(timingRole)))
			}
		}
		for routeIndex, routeID := range timing.LocalRouteIDs {
			routePath := fmt.Sprintf("%s.local_route_ids.%d", timingPath, routeIndex)
			trimmedRouteID := strings.TrimSpace(routeID)
			if trimmedRouteID == "" {
				issues = append(issues, blockIssue(fmt.Sprintf("%s.local_route_ids.%d", timingPath, routeIndex), "local route ID is required"))
				continue
			}
			if trimmedRouteID != routeID {
				issues = append(issues, blockIssue(routePath, "local route ID must not contain leading or trailing whitespace"))
			}
			if _, ok := routeIDs[trimmedRouteID]; !ok {
				issues = append(issues, blockIssue(routePath, "unknown local route ID "+trimmedRouteID))
			}
		}
		issues = append(issues, validateOptionalNonNegativeMM(timingPath+".max_source_to_consumer_distance_mm", timing.MaxSourceToConsumerDistanceMM)...)
		issues = append(issues, validateOptionalNonNegativeMM(timingPath+".max_load_cap_distance_mm", timing.MaxLoadCapDistanceMM)...)
		issues = append(issues, validateOptionalNonNegativeMM(timingPath+".max_decoupling_distance_mm", timing.MaxDecouplingDistanceMM)...)
		issues = append(issues, validateOptionalNonNegativeMM(timingPath+".max_load_cap_asymmetry_mm", timing.MaxLoadCapAsymmetryMM)...)
		issues = append(issues, validateOptionalNonNegativeMM(timingPath+".max_clock_route_length_mm", timing.MaxClockRouteLengthMM)...)
		issues = append(issues, validateOptionalNonNegativeMM(timingPath+".min_noise_keepout_mm", timing.MinNoiseKeepoutMM)...)
		issues = append(issues, validateLayer(timingPath+".preferred_layer", timing.PreferredLayer, true)...)
		issues = append(issues, validateRealizationWhen(timingPath+".when", timing.When, parameters)...)
	}
	zoneIDs := map[string]struct{}{}
	for index, zone := range realization.Zones {
		zonePath := fmt.Sprintf("%s.zones.%d", path, index)
		id := strings.TrimSpace(zone.ID)
		if id == "" {
			issues = append(issues, blockIssue(zonePath+".id", "zone ID is required"))
		} else if _, exists := zoneIDs[id]; exists {
			issues = append(issues, blockIssue(zonePath+".id", "duplicate zone ID "+id))
		}
		zoneIDs[id] = struct{}{}
		if strings.TrimSpace(zone.NetTemplate) == "" {
			issues = append(issues, blockIssue(zonePath+".net_template", "zone net template is required"))
		}
		issues = append(issues, validateLayer(zonePath+".layer", zone.Layer, false)...)
		if len(zone.Points) < 3 {
			issues = append(issues, blockIssue(zonePath+".points", "zone polygon requires at least three points"))
		}
		for pointIndex, point := range zone.Points {
			issues = append(issues, validatePoint(fmt.Sprintf("%s.points.%d", zonePath, pointIndex), point)...)
		}
		if zone.ThermalGapMM < 0 || !finite(zone.ThermalGapMM) {
			issues = append(issues, blockIssue(zonePath+".thermal_gap_mm", "thermal gap must be finite and non-negative"))
		}
		issues = append(issues, validateRealizationWhen(zonePath+".when", zone.When, parameters)...)
	}
	keepoutIDs := map[string]struct{}{}
	for index, keepout := range realization.Keepouts {
		keepoutPath := fmt.Sprintf("%s.keepouts.%d", path, index)
		id := strings.TrimSpace(keepout.ID)
		if id == "" {
			issues = append(issues, blockIssue(keepoutPath+".id", "keepout ID is required"))
		} else if _, exists := keepoutIDs[id]; exists {
			issues = append(issues, blockIssue(keepoutPath+".id", "duplicate keepout ID "+id))
		}
		keepoutIDs[id] = struct{}{}
		issues = append(issues, validateLayer(keepoutPath+".layer", keepout.Layer, false)...)
		issues = append(issues, validateBounds(keepoutPath+".bounds", keepout.Bounds)...)
		if groupID := strings.TrimSpace(keepout.PlacementGroupID); groupID != "" {
			if groupID != keepout.PlacementGroupID {
				issues = append(issues, blockIssue(keepoutPath+".placement_group_id", "placement group ID must not contain leading or trailing whitespace"))
			} else if _, exists := groupIDs[groupID]; !exists {
				issues = append(issues, blockIssue(keepoutPath+".placement_group_id", "unknown placement group "+groupID))
			}
		}
		for appliesIndex, target := range keepout.AppliesTo {
			issues = append(issues, validateKnownKeepoutTarget(fmt.Sprintf("%s.applies_to.%d", keepoutPath, appliesIndex), target, roles)...)
		}
	}
	constraintIDs := map[string]struct{}{}
	for index, constraint := range realization.Constraints {
		constraintPath := fmt.Sprintf("%s.constraints.%d", path, index)
		id := strings.TrimSpace(constraint.ID)
		if id == "" {
			issues = append(issues, blockIssue(constraintPath+".id", "constraint ID is required"))
		} else if _, exists := constraintIDs[id]; exists {
			issues = append(issues, blockIssue(constraintPath+".id", "duplicate constraint ID "+id))
		}
		constraintIDs[id] = struct{}{}
		if strings.TrimSpace(constraint.Kind) == "" {
			issues = append(issues, blockIssue(constraintPath+".kind", "constraint kind is required"))
		}
		if constraint.Category != "" && !validPCBConstraintCategory(constraint.Category) {
			issues = append(issues, blockIssue(constraintPath+".category", "unsupported PCB constraint category "+string(constraint.Category)))
		}
		for appliesIndex, role := range constraint.AppliesTo {
			issues = append(issues, validateKnownRole(fmt.Sprintf("%s.applies_to.%d", constraintPath, appliesIndex), role, roles)...)
		}
		if constraint.MinWidthMM < 0 || constraint.ClearanceMM < 0 || constraint.MaxLengthMM < 0 ||
			!finite(constraint.MinWidthMM) || !finite(constraint.ClearanceMM) || !finite(constraint.MaxLengthMM) {
			issues = append(issues, blockIssue(constraintPath, "constraint dimensions must be finite and non-negative"))
		}
		issues = append(issues, validateRealizationWhen(constraintPath+".when", constraint.When, parameters)...)
	}
	return issues
}

func validPCBConstraintCategory(category PCBConstraintCategory) bool {
	switch category {
	case PCBConstraintAnalogInputSeparation,
		PCBConstraintReturnTopology,
		PCBConstraintFeedbackSense,
		PCBConstraintDecouplingProximity,
		PCBConstraintCurrentPath,
		PCBConstraintKelvinSense,
		PCBConstraintThermalCoupling,
		PCBConstraintDeviceSymmetry,
		PCBConstraintThermalKeepout,
		PCBConstraintPolarizedOrientation:
		return true
	default:
		return false
	}
}

func validPCBTimingKind(kind string) bool {
	switch kind {
	case PCBTimingKindCrystal, PCBTimingKindOscillator, PCBTimingKindReset:
		return true
	default:
		return false
	}
}

func validPCBTimingRole(role PCBTimingRole) bool {
	switch role {
	case PCBTimingRoleCrystal,
		PCBTimingRoleOscillator,
		PCBTimingRoleConsumer,
		PCBTimingRoleLoadCapacitor,
		PCBTimingRoleDecoupling,
		PCBTimingRoleEnableControl,
		PCBTimingRoleGroundReturn,
		PCBTimingRoleResetPullup,
		PCBTimingRoleProgrammingHeader:
		return true
	default:
		return false
	}
}

func validateKnownKeepoutTarget(path string, target string, roles map[string]struct{}) []reports.Issue {
	trimmed := strings.TrimSpace(target)
	switch trimmed {
	case "copper", "tracks", "vias", "pads", "zones", "footprints":
		return nil
	default:
		return validateKnownRole(path, trimmed, roles)
	}
}

func componentRoleSet(components []BlockComponent) map[string]struct{} {
	roles := map[string]struct{}{}
	for _, component := range components {
		role := strings.TrimSpace(component.Role)
		if role != "" {
			roles[role] = struct{}{}
		}
	}
	return roles
}

func parameterNameSet(parameters []BlockParameter) map[string]struct{} {
	names := map[string]struct{}{}
	for _, parameter := range parameters {
		name := strings.TrimSpace(parameter.Name)
		if name != "" {
			names[name] = struct{}{}
		}
	}
	return names
}

func portNameSet(ports []BlockPort) map[string]struct{} {
	names := map[string]struct{}{}
	for _, port := range ports {
		name := strings.TrimSpace(port.Name)
		if name != "" {
			names[name] = struct{}{}
		}
	}
	return names
}

func validateKnownRole(path string, role string, roles map[string]struct{}) []reports.Issue {
	role = strings.TrimSpace(role)
	if role == "" {
		return []reports.Issue{blockIssue(path, "component role is required")}
	}
	if _, ok := roles[role]; !ok {
		return []reports.Issue{blockIssue(path, "unknown component role "+role)}
	}
	return nil
}

func validateRouteEndpoint(path string, endpoint RouteEndpoint, roles map[string]struct{}, ports map[string]struct{}, anchors map[string]struct{}) []reports.Issue {
	var issues []reports.Issue
	componentRole := strings.TrimSpace(endpoint.ComponentRole)
	pin := strings.TrimSpace(endpoint.Pin)
	anchorID := strings.TrimSpace(endpoint.AnchorID)
	port := strings.TrimSpace(endpoint.Port)
	componentMode := componentRole != "" || pin != ""
	anchorMode := anchorID != ""
	portMode := port != ""
	modeCount := 0
	if componentMode {
		modeCount++
	}
	if anchorMode {
		modeCount++
	}
	if portMode {
		modeCount++
	}
	if modeCount == 0 {
		return []reports.Issue{blockIssue(path, "route endpoint requires component_role/pin, anchor_id, or port")}
	}
	if modeCount > 1 {
		return []reports.Issue{blockIssue(path, "route endpoint must use exactly one endpoint mode")}
	}
	if componentMode {
		issues = append(issues, validateKnownRole(path+".component_role", componentRole, roles)...)
		if componentRole != endpoint.ComponentRole {
			issues = append(issues, blockIssue(path+".component_role", "route endpoint component role must not contain leading or trailing whitespace"))
		}
		if pin == "" {
			issues = append(issues, blockIssue(path+".pin", "route endpoint pin is required"))
		} else if pin != endpoint.Pin {
			issues = append(issues, blockIssue(path+".pin", "route endpoint pin must not contain leading or trailing whitespace"))
		}
		return issues
	}
	if anchorMode {
		if anchorID != endpoint.AnchorID {
			issues = append(issues, blockIssue(path+".anchor_id", "route endpoint anchor ID must not contain leading or trailing whitespace"))
		}
		if _, ok := anchors[anchorID]; !ok {
			issues = append(issues, blockIssue(path+".anchor_id", "unknown route endpoint anchor "+anchorID))
		}
		return issues
	}
	if portMode {
		if port != endpoint.Port {
			issues = append(issues, blockIssue(path+".port", "route endpoint port must not contain leading or trailing whitespace"))
		}
		if _, ok := ports[port]; !ok {
			issues = append(issues, blockIssue(path+".port", "unknown route endpoint port "+port))
		}
		return issues
	}
	return nil
}

func validateRealizationWhen(path string, condition RealizationWhen, parameters map[string]struct{}) []reports.Issue {
	if len(condition.Params) == 0 {
		return nil
	}
	var issues []reports.Issue
	names := make([]string, 0, len(condition.Params))
	for name := range condition.Params {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			issues = append(issues, blockIssue(path+".params", "condition parameter name is required"))
			continue
		}
		if trimmed != name {
			issues = append(issues, blockIssue(path+".params."+realizationPathSegment(trimmed), "condition parameter name must not contain leading or trailing whitespace"))
			continue
		}
		if _, ok := parameters[trimmed]; !ok {
			issues = append(issues, blockIssue(path+".params."+realizationPathSegment(trimmed), "unknown condition parameter "+trimmed))
		}
	}
	return issues
}

func realizationPathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return realizationPathSegmentReplacer.Replace(value)
}

func validatePCBRealizationSide(path string, side string) []reports.Issue {
	trimmed := strings.TrimSpace(side)
	if trimmed == "" {
		return nil
	}
	if trimmed != side {
		return []reports.Issue{blockIssue(path, "side must not contain leading or trailing whitespace")}
	}
	switch strings.ToLower(trimmed) {
	case "front", "top", "back", "bottom":
		return nil
	default:
		return []reports.Issue{blockIssue(path, "side must be front/top or back/bottom")}
	}
}

func validateRelativePlacement(path string, placement RelativePlacement) []reports.Issue {
	var issues []reports.Issue
	if !finite(placement.XMM) || !finite(placement.YMM) || !finite(placement.RotationDeg) {
		issues = append(issues, blockIssue(path, "placement coordinates and rotation must be finite"))
	}
	issues = append(issues, validateLayer(path+".layer", placement.Layer, true)...)
	return issues
}

func validatePoint(path string, point RelativePoint) []reports.Issue {
	if !finite(point.XMM) || !finite(point.YMM) {
		return []reports.Issue{blockIssue(path, "point coordinates must be finite")}
	}
	return nil
}

func validateBounds(path string, bounds RelativeBounds) []reports.Issue {
	if !finite(bounds.MinXMM) || !finite(bounds.MinYMM) || !finite(bounds.MaxXMM) || !finite(bounds.MaxYMM) {
		return []reports.Issue{blockIssue(path, "bounds coordinates must be finite")}
	}
	if bounds.MaxXMM <= bounds.MinXMM || bounds.MaxYMM <= bounds.MinYMM {
		return []reports.Issue{blockIssue(path, "bounds max values must exceed min values")}
	}
	return nil
}

func validateOptionalNonNegativeMM(path string, value *float64) []reports.Issue {
	if value == nil {
		return nil
	}
	if *value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return []reports.Issue{blockIssue(path, "timing dimension must be finite and non-negative")}
	}
	return nil
}

func validateLayer(path string, layer string, allowEmpty bool) []reports.Issue {
	layer = strings.TrimSpace(layer)
	if layer == "" && allowEmpty {
		return nil
	}
	if layer == "" {
		return []reports.Issue{blockIssue(path, "layer is required")}
	}
	if !kicadfiles.IsValidBoardLayer(kicadfiles.BoardLayer(layer)) {
		return []reports.Issue{blockIssue(path, "invalid KiCad board layer "+layer)}
	}
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func validPCBVerificationLevel(level PCBVerificationLevel) bool {
	switch level {
	case PCBVerificationUnrealized,
		PCBVerificationPlacementVerified,
		PCBVerificationConnectivityVerified,
		PCBVerificationDRCVerified,
		PCBVerificationReferenceVerified:
		return true
	default:
		return false
	}
}

func stringInSlice(value string, values []string) bool {
	value = strings.TrimSpace(value)
	for _, candidate := range values {
		if strings.TrimSpace(candidate) == value {
			return true
		}
	}
	return false
}

func clonePCBRealization(realization *PCBRealization) *PCBRealization {
	if realization == nil {
		return nil
	}
	clone := *realization
	clone.Components = append([]PCBComponentRealization(nil), realization.Components...)
	for i := range clone.Components {
		clone.Components[i].PhysicalPins = append([]transactions.PinSpec(nil), realization.Components[i].PhysicalPins...)
		clone.Components[i].Properties = cloneStringMap(realization.Components[i].Properties)
		clone.Components[i].When = cloneRealizationWhen(realization.Components[i].When)
	}
	clone.PlacementGroups = append([]PCBPlacementGroup(nil), realization.PlacementGroups...)
	for i := range clone.PlacementGroups {
		clone.PlacementGroups[i].ComponentRoles = append([]string(nil), realization.PlacementGroups[i].ComponentRoles...)
		if realization.PlacementGroups[i].Bounds != nil {
			bounds := *realization.PlacementGroups[i].Bounds
			clone.PlacementGroups[i].Bounds = &bounds
		}
	}
	clone.EntryAnchors = append([]PCBEntryAnchor(nil), realization.EntryAnchors...)
	for i := range clone.EntryAnchors {
		clone.EntryAnchors[i].When = cloneRealizationWhen(realization.EntryAnchors[i].When)
		clone.EntryAnchors[i].OmitWhen = append([]RealizationWhen(nil), realization.EntryAnchors[i].OmitWhen...)
		for omitIndex := range clone.EntryAnchors[i].OmitWhen {
			clone.EntryAnchors[i].OmitWhen[omitIndex] = cloneRealizationWhen(clone.EntryAnchors[i].OmitWhen[omitIndex])
		}
		clone.EntryAnchors[i].Variants = append([]PCBAnchorPlacementVariant(nil), realization.EntryAnchors[i].Variants...)
		for variantIndex := range clone.EntryAnchors[i].Variants {
			clone.EntryAnchors[i].Variants[variantIndex].When = cloneRealizationWhen(clone.EntryAnchors[i].Variants[variantIndex].When)
		}
	}
	clone.LocalRoutes = append([]PCBLocalRoute(nil), realization.LocalRoutes...)
	for i := range clone.LocalRoutes {
		clone.LocalRoutes[i].Waypoints = append([]RelativePoint(nil), realization.LocalRoutes[i].Waypoints...)
		clone.LocalRoutes[i].WaypointVariants = append([]PCBWaypointVariant(nil), realization.LocalRoutes[i].WaypointVariants...)
		clone.LocalRoutes[i].EndpointVariants = append([]PCBEndpointVariant(nil), realization.LocalRoutes[i].EndpointVariants...)
		for variantIndex := range clone.LocalRoutes[i].EndpointVariants {
			clone.LocalRoutes[i].EndpointVariants[variantIndex].When = cloneRealizationWhen(clone.LocalRoutes[i].EndpointVariants[variantIndex].When)
		}
		clone.LocalRoutes[i].GeometryVariants = append([]PCBRouteGeometryVariant(nil), realization.LocalRoutes[i].GeometryVariants...)
		for variantIndex := range clone.LocalRoutes[i].GeometryVariants {
			variant := &clone.LocalRoutes[i].GeometryVariants[variantIndex]
			variant.Waypoints = append([]RelativePoint(nil), variant.Waypoints...)
			variant.When = cloneRealizationWhen(variant.When)
		}
		for variantIndex := range clone.LocalRoutes[i].WaypointVariants {
			variant := &clone.LocalRoutes[i].WaypointVariants[variantIndex]
			variant.Waypoints = append([]RelativePoint(nil), variant.Waypoints...)
			variant.When = cloneRealizationWhen(variant.When)
		}
		if realization.LocalRoutes[i].EntryAnchorDogbone != nil {
			dogbone := *realization.LocalRoutes[i].EntryAnchorDogbone
			clone.LocalRoutes[i].EntryAnchorDogbone = &dogbone
		}
		clone.LocalRoutes[i].When = cloneRealizationWhen(realization.LocalRoutes[i].When)
	}
	if len(realization.TimingFixtures) > 0 {
		clone.TimingFixtures = make([]PCBTimingFixture, len(realization.TimingFixtures))
	}
	for i, timing := range realization.TimingFixtures {
		timing.LoadCapacitorRoles = append([]string(nil), timing.LoadCapacitorRoles...)
		timing.DecouplingRoles = append([]string(nil), timing.DecouplingRoles...)
		timing.EnableControlRoles = append([]string(nil), timing.EnableControlRoles...)
		timing.ClockNetTemplates = append([]string(nil), timing.ClockNetTemplates...)
		timing.LocalRouteIDs = append([]string(nil), timing.LocalRouteIDs...)
		timing.MaxSourceToConsumerDistanceMM = cloneFloat64Ptr(timing.MaxSourceToConsumerDistanceMM)
		timing.MaxLoadCapDistanceMM = cloneFloat64Ptr(timing.MaxLoadCapDistanceMM)
		timing.MaxDecouplingDistanceMM = cloneFloat64Ptr(timing.MaxDecouplingDistanceMM)
		timing.MaxLoadCapAsymmetryMM = cloneFloat64Ptr(timing.MaxLoadCapAsymmetryMM)
		timing.MaxClockRouteLengthMM = cloneFloat64Ptr(timing.MaxClockRouteLengthMM)
		timing.MinNoiseKeepoutMM = cloneFloat64Ptr(timing.MinNoiseKeepoutMM)
		timing.Roles = cloneTimingRoleMap(timing.Roles)
		timing.When = cloneRealizationWhen(timing.When)
		clone.TimingFixtures[i] = timing
	}
	clone.Zones = append([]PCBZoneRealization(nil), realization.Zones...)
	for i := range clone.Zones {
		clone.Zones[i].Points = append([]RelativePoint(nil), realization.Zones[i].Points...)
		clone.Zones[i].When = cloneRealizationWhen(realization.Zones[i].When)
	}
	clone.Keepouts = append([]PCBKeepout(nil), realization.Keepouts...)
	for i := range clone.Keepouts {
		clone.Keepouts[i].AppliesTo = append([]string(nil), realization.Keepouts[i].AppliesTo...)
		if clone.Keepouts[i].BlocksRoute != nil {
			value := *clone.Keepouts[i].BlocksRoute
			clone.Keepouts[i].BlocksRoute = &value
		}
	}
	clone.Constraints = append([]PCBConstraint(nil), realization.Constraints...)
	for i := range clone.Constraints {
		clone.Constraints[i].AppliesTo = append([]string(nil), realization.Constraints[i].AppliesTo...)
		clone.Constraints[i].When = cloneRealizationWhen(realization.Constraints[i].When)
	}
	clone.Validation.RequiredNets = append([]string(nil), realization.Validation.RequiredNets...)
	clone.Validation.RequiredRoutes = append([]string(nil), realization.Validation.RequiredRoutes...)
	clone.Validation.RequiredZones = append([]string(nil), realization.Validation.RequiredZones...)
	clone.Validation.AllowedUnroutedNets = append([]string(nil), realization.Validation.AllowedUnroutedNets...)
	clone.UnsupportedBehaviors = append([]string(nil), realization.UnsupportedBehaviors...)
	return &clone
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneRealizationWhen(condition RealizationWhen) RealizationWhen {
	condition.Params = cloneAnyParams(condition.Params)
	return condition
}

func cloneTimingRoleMap(values map[string]PCBTimingRole) map[string]PCBTimingRole {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]PCBTimingRole, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
