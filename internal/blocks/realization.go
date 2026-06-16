package blocks

import (
	"fmt"
	"math"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/reports"
)

type PCBVerificationLevel string

const (
	PCBVerificationUnrealized           PCBVerificationLevel = "pcb_unrealized"
	PCBVerificationPlacementVerified    PCBVerificationLevel = "pcb_placement_verified"
	PCBVerificationConnectivityVerified PCBVerificationLevel = "pcb_connectivity_verified"
	PCBVerificationDRCVerified          PCBVerificationLevel = "pcb_drc_verified"
	PCBVerificationReferenceVerified    PCBVerificationLevel = "pcb_reference_verified"
)

type PCBRealization struct {
	Version              string                    `json:"version"`
	VerificationLevel    PCBVerificationLevel      `json:"verification_level"`
	Components           []PCBComponentRealization `json:"components,omitempty"`
	PlacementGroups      []PCBPlacementGroup       `json:"placement_groups,omitempty"`
	LocalRoutes          []PCBLocalRoute           `json:"local_routes,omitempty"`
	Zones                []PCBZoneRealization      `json:"zones,omitempty"`
	Keepouts             []PCBKeepout              `json:"keepouts,omitempty"`
	Constraints          []PCBConstraint           `json:"constraints,omitempty"`
	Validation           PCBValidationExpectations `json:"validation,omitempty"`
	FabricationReadiness bool                      `json:"fabrication_readiness,omitempty"`
	UnsupportedBehaviors []string                  `json:"unsupported_behaviors,omitempty"`
}

type PCBComponentRealization struct {
	ComponentRole  string            `json:"component_role"`
	FootprintParam string            `json:"footprint_param,omitempty"`
	FootprintID    string            `json:"footprint_id,omitempty"`
	Placement      RelativePlacement `json:"placement"`
	Side           string            `json:"side,omitempty"`
	Properties     map[string]string `json:"properties,omitempty"`
}

type RelativePlacement struct {
	XMM         float64 `json:"x_mm"`
	YMM         float64 `json:"y_mm"`
	RotationDeg float64 `json:"rotation_deg,omitempty"`
	Layer       string  `json:"layer,omitempty"`
	Fixed       bool    `json:"fixed,omitempty"`
}

type PCBPlacementGroup struct {
	ID             string          `json:"id"`
	ComponentRoles []string        `json:"component_roles"`
	AnchorRole     string          `json:"anchor_role,omitempty"`
	Bounds         *RelativeBounds `json:"bounds,omitempty"`
	Description    string          `json:"description,omitempty"`
}

type RelativeBounds struct {
	MinXMM float64 `json:"min_x_mm"`
	MinYMM float64 `json:"min_y_mm"`
	MaxXMM float64 `json:"max_x_mm"`
	MaxYMM float64 `json:"max_y_mm"`
}

type PCBLocalRoute struct {
	ID          string          `json:"id"`
	NetTemplate string          `json:"net_template"`
	From        RouteEndpoint   `json:"from"`
	To          RouteEndpoint   `json:"to"`
	Waypoints   []RelativePoint `json:"waypoints,omitempty"`
	Layer       string          `json:"layer,omitempty"`
	WidthMM     float64         `json:"width_mm,omitempty"`
	Required    bool            `json:"required,omitempty"`
	Description string          `json:"description,omitempty"`
}

type RouteEndpoint struct {
	ComponentRole string `json:"component_role"`
	Pin           string `json:"pin"`
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
}

type PCBKeepout struct {
	ID          string         `json:"id"`
	Layer       string         `json:"layer"`
	Bounds      RelativeBounds `json:"bounds"`
	AppliesTo   []string       `json:"applies_to,omitempty"`
	Description string         `json:"description,omitempty"`
}

type PCBConstraint struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	NetTemplate string  `json:"net_template,omitempty"`
	MinWidthMM  float64 `json:"min_width_mm,omitempty"`
	ClearanceMM float64 `json:"clearance_mm,omitempty"`
	MaxLengthMM float64 `json:"max_length_mm,omitempty"`
	Description string  `json:"description,omitempty"`
}

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
		issues = append(issues, validateRouteEndpoint(routePath+".from", route.From, roles)...)
		issues = append(issues, validateRouteEndpoint(routePath+".to", route.To, roles)...)
		issues = append(issues, validateLayer(routePath+".layer", route.Layer, true)...)
		if route.WidthMM < 0 || !finite(route.WidthMM) {
			issues = append(issues, blockIssue(routePath+".width_mm", "route width must be finite and non-negative"))
		}
		for waypointIndex, point := range route.Waypoints {
			issues = append(issues, validatePoint(fmt.Sprintf("%s.waypoints.%d", routePath, waypointIndex), point)...)
		}
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
		if constraint.MinWidthMM < 0 || constraint.ClearanceMM < 0 || constraint.MaxLengthMM < 0 ||
			!finite(constraint.MinWidthMM) || !finite(constraint.ClearanceMM) || !finite(constraint.MaxLengthMM) {
			issues = append(issues, blockIssue(constraintPath, "constraint dimensions must be finite and non-negative"))
		}
	}
	return issues
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

func validateRouteEndpoint(path string, endpoint RouteEndpoint, roles map[string]struct{}) []reports.Issue {
	var issues []reports.Issue
	issues = append(issues, validateKnownRole(path+".component_role", endpoint.ComponentRole, roles)...)
	if strings.TrimSpace(endpoint.Pin) == "" {
		issues = append(issues, blockIssue(path+".pin", "route endpoint pin is required"))
	}
	return issues
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
		clone.Components[i].Properties = cloneStringMap(realization.Components[i].Properties)
	}
	clone.PlacementGroups = append([]PCBPlacementGroup(nil), realization.PlacementGroups...)
	for i := range clone.PlacementGroups {
		clone.PlacementGroups[i].ComponentRoles = append([]string(nil), realization.PlacementGroups[i].ComponentRoles...)
		if realization.PlacementGroups[i].Bounds != nil {
			bounds := *realization.PlacementGroups[i].Bounds
			clone.PlacementGroups[i].Bounds = &bounds
		}
	}
	clone.LocalRoutes = append([]PCBLocalRoute(nil), realization.LocalRoutes...)
	for i := range clone.LocalRoutes {
		clone.LocalRoutes[i].Waypoints = append([]RelativePoint(nil), realization.LocalRoutes[i].Waypoints...)
	}
	clone.Zones = append([]PCBZoneRealization(nil), realization.Zones...)
	for i := range clone.Zones {
		clone.Zones[i].Points = append([]RelativePoint(nil), realization.Zones[i].Points...)
	}
	clone.Keepouts = append([]PCBKeepout(nil), realization.Keepouts...)
	for i := range clone.Keepouts {
		clone.Keepouts[i].AppliesTo = append([]string(nil), realization.Keepouts[i].AppliesTo...)
	}
	clone.Constraints = append([]PCBConstraint(nil), realization.Constraints...)
	clone.Validation.RequiredNets = append([]string(nil), realization.Validation.RequiredNets...)
	clone.Validation.RequiredRoutes = append([]string(nil), realization.Validation.RequiredRoutes...)
	clone.Validation.RequiredZones = append([]string(nil), realization.Validation.RequiredZones...)
	clone.Validation.AllowedUnroutedNets = append([]string(nil), realization.Validation.AllowedUnroutedNets...)
	clone.UnsupportedBehaviors = append([]string(nil), realization.UnsupportedBehaviors...)
	return &clone
}
