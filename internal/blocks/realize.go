package blocks

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type PCBRealizationOptions struct {
	OriginXMM float64 `json:"origin_x_mm,omitempty"`
	OriginYMM float64 `json:"origin_y_mm,omitempty"`
	Layer     string  `json:"layer,omitempty"`
}

type BlockPCBRealizationResult struct {
	Definition  BlockSummary                 `json:"definition"`
	Instance    BlockInstance                `json:"instance"`
	Components  []RealizedPCBComponent       `json:"components,omitempty"`
	LocalRoutes []RealizedPCBLocalRoute      `json:"local_routes,omitempty"`
	Operations  []transactions.Operation     `json:"operations,omitempty"`
	Validation  PCBValidationExpectations    `json:"validation,omitempty"`
	Issues      []reports.Issue              `json:"issues,omitempty"`
	Unsupported []string                     `json:"unsupported,omitempty"`
	RoleRefs    map[string]string            `json:"role_refs,omitempty"`
	Metadata    map[string]RealizationMetric `json:"metadata,omitempty"`
}

type RealizationMetric struct {
	Count int `json:"count,omitempty"`
}

type RealizedPCBComponent struct {
	ComponentRole string            `json:"component_role"`
	Ref           string            `json:"ref"`
	FootprintID   string            `json:"footprint_id"`
	Value         string            `json:"value,omitempty"`
	Placement     RelativePlacement `json:"placement"`
}

type RealizedPCBLocalRoute struct {
	ID      string                `json:"id"`
	NetName string                `json:"net_name"`
	From    transactions.Endpoint `json:"from"`
	To      transactions.Endpoint `json:"to"`
	Layer   string                `json:"layer,omitempty"`
	WidthMM float64               `json:"width_mm,omitempty"`
}

const (
	degreesToRadians             = math.Pi / 180
	routeCoordinateSnapEpsilonMM = 1e-9
)

func RealizeBlockPCB(definition BlockDefinition, output BlockOutput, opts PCBRealizationOptions) BlockPCBRealizationResult {
	result := BlockPCBRealizationResult{
		Definition:  Summary(definition),
		Instance:    output.Instance,
		Issues:      append([]reports.Issue(nil), output.Issues...),
		RoleRefs:    map[string]string{},
		Metadata:    map[string]RealizationMetric{},
		Unsupported: append([]string(nil), unsupportedPCBBehaviors(definition)...),
	}
	if definition.PCBRealization == nil {
		result.Issues = append(result.Issues, reports.Issue{
			Code:     reports.CodeUnsupportedOperation,
			Severity: reports.SeverityBlocked,
			Path:     "pcb_realization",
			Message:  "block does not define PCB realization metadata",
		})
		return result
	}
	result.Validation = definition.PCBRealization.Validation
	if issues := ValidatePCBRealization(definition); len(issues) != 0 {
		result.Issues = append(result.Issues, issues...)
		return result
	}
	componentFacts, factIssues := componentFactsFromOperations(output.Operations)
	result.Issues = append(result.Issues, factIssues...)
	roleRefs := roleRefsFromOutput(definition, output, componentFacts)
	for role, ref := range roleRefs {
		result.RoleRefs[role] = ref
	}
	for _, component := range definition.PCBRealization.Components {
		ref := roleRefs[component.ComponentRole]
		if ref == "" {
			result.Issues = append(result.Issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityWarning,
				Path:     "pcb_realization.components." + component.ComponentRole,
				Message:  "component role was not emitted by block instantiation",
			})
			continue
		}
		footprintID := resolveRealizationFootprint(component, output.Instance.Params)
		if footprintID == "" {
			footprintID = componentFacts[ref].FootprintID
		}
		if footprintID == "" {
			result.Issues = append(result.Issues, reports.Issue{
				Code:     reports.CodeMissingFootprint,
				Severity: reports.SeverityError,
				Path:     "pcb_realization.components." + component.ComponentRole + ".footprint_id",
				Message:  "component role " + component.ComponentRole + " has no resolved footprint",
				Refs:     []string{ref},
			})
			continue
		}
		placement := component.Placement
		placement.XMM += opts.OriginXMM
		placement.YMM += opts.OriginYMM
		if placement.Layer == "" {
			placement.Layer = firstNonEmptyString(opts.Layer, "F.Cu")
		}
		value := componentFacts[ref].Value
		operation, err := wrapOperation(transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{
			Op:          transactions.OpPlaceFootprint,
			Ref:         ref,
			FootprintID: footprintID,
			Value:       value,
			At:          transactions.Point{XMM: placement.XMM, YMM: placement.YMM},
			Rotation:    placement.RotationDeg,
			Layer:       placement.Layer,
		})
		if err != nil {
			result.Issues = append(result.Issues, blockIssue("pcb_realization.components."+component.ComponentRole, err.Error()))
			continue
		}
		realized := RealizedPCBComponent{
			ComponentRole: component.ComponentRole,
			Ref:           ref,
			FootprintID:   footprintID,
			Value:         value,
			Placement:     placement,
		}
		result.Components = append(result.Components, realized)
		result.Operations = append(result.Operations, operation)
	}
	placements := realizedPlacementMap(result.Components)
	componentByRole := blockComponentByRole(definition.Components)
	for _, route := range definition.PCBRealization.LocalRoutes {
		fromRef := roleRefs[route.From.ComponentRole]
		toRef := roleRefs[route.To.ComponentRole]
		if fromRef == "" || toRef == "" {
			result.Issues = append(result.Issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityWarning,
				Path:     "pcb_realization.local_routes." + route.ID,
				Message:  "local route endpoint component was not emitted by block instantiation",
			})
			continue
		}
		if _, ok := placements[route.From.ComponentRole]; !ok {
			result.Issues = append(result.Issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityWarning,
				Path:     "pcb_realization.local_routes." + route.ID + ".from",
				Message:  "local route source component was not placed",
				Refs:     []string{fromRef},
			})
			continue
		}
		if _, ok := placements[route.To.ComponentRole]; !ok {
			result.Issues = append(result.Issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityWarning,
				Path:     "pcb_realization.local_routes." + route.ID + ".to",
				Message:  "local route target component was not placed",
				Refs:     []string{toRef},
			})
			continue
		}
		netName := InstanceNetName(output.Instance.InstanceID, route.NetTemplate)
		realizedRoute := RealizedPCBLocalRoute{
			ID:      route.ID,
			NetName: netName,
			From:    transactions.Endpoint{Ref: fromRef, Pin: route.From.Pin},
			To:      transactions.Endpoint{Ref: toRef, Pin: route.To.Pin},
			Layer:   firstNonEmptyString(route.Layer, opts.Layer, "F.Cu"),
			WidthMM: route.WidthMM,
		}
		points, pointIssues := routePoints(route, placements, componentByRole, opts)
		if len(pointIssues) != 0 {
			result.Issues = append(result.Issues, pointIssues...)
			continue
		}
		operation, err := wrapOperation(transactions.OpRoute, transactions.RouteOperation{
			Op:      transactions.OpRoute,
			NetName: netName,
			Layer:   realizedRoute.Layer,
			WidthMM: realizedRoute.WidthMM,
			Points:  points,
		})
		if err != nil {
			result.Issues = append(result.Issues, blockIssue("pcb_realization.local_routes."+route.ID, err.Error()))
			continue
		}
		result.LocalRoutes = append(result.LocalRoutes, realizedRoute)
		result.Operations = append(result.Operations, operation)
	}
	result.Metadata["components"] = RealizationMetric{Count: len(result.Components)}
	result.Metadata["local_routes"] = RealizationMetric{Count: len(result.LocalRoutes)}
	return result
}

type emittedComponentFact struct {
	Value       string
	SymbolID    string
	FootprintID string
}

func componentFactsFromOperations(operations []transactions.Operation) (map[string]emittedComponentFact, []reports.Issue) {
	facts := map[string]emittedComponentFact{}
	var issues []reports.Issue
	for index, operation := range operations {
		switch operation.Op {
		case transactions.OpAddSymbol:
			var payload transactions.AddSymbolOperation
			if err := decodeOperation(operation, &payload); err != nil {
				issues = append(issues, realizationDecodeIssue(index, err))
				continue
			}
			fact := facts[payload.Ref]
			fact.Value = payload.Value
			fact.SymbolID = payload.LibraryID
			facts[payload.Ref] = fact
		case transactions.OpAssignFootprint, transactions.OpPlaceFootprint:
			var payload struct {
				Ref         string `json:"ref"`
				FootprintID string `json:"footprint_id"`
			}
			if err := decodeOperation(operation, &payload); err != nil {
				issues = append(issues, realizationDecodeIssue(index, err))
				continue
			}
			if payload.FootprintID != "" {
				fact := facts[payload.Ref]
				fact.FootprintID = payload.FootprintID
				facts[payload.Ref] = fact
			}
		}
	}
	return facts, issues
}

func realizationDecodeIssue(index int, err error) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     fmt.Sprintf("operations.%d", index),
		Message:  "could not decode operation for PCB realization: " + err.Error(),
	}
}

func decodeOperation(operation transactions.Operation, payload any) error {
	if len(operation.Raw) == 0 {
		return fmt.Errorf("operation payload missing raw JSON")
	}
	return json.Unmarshal(operation.Raw, payload)
}

func roleRefsFromOutput(definition BlockDefinition, output BlockOutput, facts map[string]emittedComponentFact) map[string]string {
	refs := append([]string(nil), output.Instance.Refs...)
	roleRefs := map[string]string{}
	used := map[string]struct{}{}
	for _, component := range definition.Components {
		role := strings.TrimSpace(component.Role)
		if role == "" {
			continue
		}
		if _, exists := roleRefs[role]; exists {
			continue
		}
		if ref := matchingRefForComponent(component, refs, facts, used); ref != "" {
			roleRefs[role] = ref
			used[ref] = struct{}{}
			continue
		}
	}
	return roleRefs
}

func matchingRefForComponent(component BlockComponent, refs []string, facts map[string]emittedComponentFact, used map[string]struct{}) string {
	for _, ref := range refs {
		if _, exists := used[ref]; exists {
			continue
		}
		fact := facts[ref]
		if component.SymbolID != "" && fact.SymbolID != component.SymbolID {
			continue
		}
		if component.FootprintID != "" && fact.FootprintID != component.FootprintID {
			continue
		}
		return ref
	}
	return ""
}

func resolveRealizationFootprint(component PCBComponentRealization, params map[string]any) string {
	if component.FootprintParam != "" {
		if value, ok := params[component.FootprintParam].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return strings.TrimSpace(component.FootprintID)
}

func unsupportedPCBBehaviors(definition BlockDefinition) []string {
	if definition.PCBRealization == nil {
		return nil
	}
	return definition.PCBRealization.UnsupportedBehaviors
}

func realizedPlacementMap(components []RealizedPCBComponent) map[string]RelativePlacement {
	placements := map[string]RelativePlacement{}
	for _, component := range components {
		placements[component.ComponentRole] = component.Placement
	}
	return placements
}

func blockComponentByRole(components []BlockComponent) map[string]BlockComponent {
	byRole := map[string]BlockComponent{}
	for _, component := range components {
		role := strings.TrimSpace(component.Role)
		if role != "" {
			if _, exists := byRole[role]; exists {
				continue
			}
			byRole[role] = component
		}
	}
	return byRole
}

func routePoints(route PCBLocalRoute, placements map[string]RelativePlacement, components map[string]BlockComponent, opts PCBRealizationOptions) ([]transactions.Point, []reports.Issue) {
	from, ok := routeEndpointPoint(route.From, placements, components)
	if !ok {
		return nil, []reports.Issue{routePinIssue(route.ID, "from", route.From)}
	}
	to, ok := routeEndpointPoint(route.To, placements, components)
	if !ok {
		return nil, []reports.Issue{routePinIssue(route.ID, "to", route.To)}
	}
	points := []transactions.Point{from}
	for _, waypoint := range route.Waypoints {
		points = append(points, transactions.Point{XMM: waypoint.XMM + opts.OriginXMM, YMM: waypoint.YMM + opts.OriginYMM})
	}
	points = append(points, to)
	return points, nil
}

func routeEndpointPoint(endpoint RouteEndpoint, placements map[string]RelativePlacement, components map[string]BlockComponent) (transactions.Point, bool) {
	placement := placements[endpoint.ComponentRole]
	pin, ok := componentPin(components[endpoint.ComponentRole], endpoint.Pin)
	if !ok {
		return transactions.Point{}, false
	}
	x, y := rotatePoint(pin.XMM, pin.YMM, placement.RotationDeg)
	return transactions.Point{XMM: placement.XMM + x, YMM: placement.YMM + y}, true
}

func componentPin(component BlockComponent, pinNumber string) (transactions.PinSpec, bool) {
	for _, pin := range component.Pins {
		if strings.TrimSpace(pin.Number) == strings.TrimSpace(pinNumber) {
			return pin, true
		}
	}
	return transactions.PinSpec{}, false
}

func routePinIssue(routeID string, side string, endpoint RouteEndpoint) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityWarning,
		Path:     "pcb_realization.local_routes." + routeID + "." + side + ".pin",
		Message:  "route endpoint pin " + endpoint.Pin + " was not found on component role " + endpoint.ComponentRole,
	}
}

func rotatePoint(x float64, y float64, rotationDeg float64) (float64, float64) {
	if rotationDeg == 0 {
		return x, y
	}
	radians := rotationDeg * degreesToRadians
	sine, cosine := math.Sincos(radians)
	// KiCad PCB file coordinates are Y-down. This transform preserves KiCad's
	// positive-angle direction in that coordinate frame.
	return zeroNear(x*cosine + y*sine), zeroNear(-x*sine + y*cosine)
}

func zeroNear(value float64) float64 {
	if math.Abs(value) < routeCoordinateSnapEpsilonMM {
		return 0
	}
	return value
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
