package blocks

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
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
	Timing      []TimingFixtureEvidence      `json:"timing,omitempty"`
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
	ID       string                `json:"id"`
	NetName  string                `json:"net_name"`
	From     transactions.Endpoint `json:"from"`
	To       transactions.Endpoint `json:"to"`
	Layer    string                `json:"layer,omitempty"`
	WidthMM  float64               `json:"width_mm,omitempty"`
	LengthMM float64               `json:"length_mm,omitempty"`
}

type TimingFixtureEvidence struct {
	ID                            string                   `json:"id"`
	TimingGroupID                 string                   `json:"timing_group_id,omitempty"`
	Kind                          string                   `json:"kind"`
	SourceRef                     string                   `json:"source_ref,omitempty"`
	ConsumerRef                   string                   `json:"consumer_ref,omitempty"`
	LoadCapacitorRefs             []string                 `json:"load_capacitor_refs,omitempty"`
	DecouplingRefs                []string                 `json:"decoupling_refs,omitempty"`
	EnableControlRefs             []string                 `json:"enable_control_refs,omitempty"`
	ClockNets                     []string                 `json:"clock_nets,omitempty"`
	GroundNet                     string                   `json:"ground_net,omitempty"`
	SourceToConsumerDistanceMM    *float64                 `json:"source_to_consumer_distance_mm,omitempty"`
	MaxSourceToConsumerDistanceMM *float64                 `json:"max_source_to_consumer_distance_mm,omitempty"`
	LoadCapacitorDistancesMM      map[string]float64       `json:"load_capacitor_distances_mm,omitempty"`
	DecouplingDistancesMM         map[string]float64       `json:"decoupling_distances_mm,omitempty"`
	LoadCapacitorAsymmetryMM      *float64                 `json:"load_capacitor_asymmetry_mm,omitempty"`
	MaxLoadCapDistanceMM          *float64                 `json:"max_load_cap_distance_mm,omitempty"`
	MaxDecouplingDistanceMM       *float64                 `json:"max_decoupling_distance_mm,omitempty"`
	MaxLoadCapAsymmetryMM         *float64                 `json:"max_load_cap_asymmetry_mm,omitempty"`
	ClockRouteLengthsMM           map[string]float64       `json:"clock_route_lengths_mm,omitempty"`
	MaxClockRouteLengthMM         *float64                 `json:"max_clock_route_length_mm,omitempty"`
	GroundReturnPresent           bool                     `json:"ground_return_present"`
	EnableControlPresent          bool                     `json:"enable_control_present"`
	Satisfied                     bool                     `json:"satisfied"`
	Findings                      []TimingFixtureFinding   `json:"findings,omitempty"`
	Roles                         map[string]PCBTimingRole `json:"roles,omitempty"`
}

type TimingFixtureFinding struct {
	ID          string           `json:"id"`
	Severity    reports.Severity `json:"severity"`
	Message     string           `json:"message"`
	MeasuredMM  *float64         `json:"measured_mm,omitempty"`
	ThresholdMM *float64         `json:"threshold_mm,omitempty"`
	Refs        []string         `json:"refs,omitempty"`
	Nets        []string         `json:"nets,omitempty"`
}

const (
	TimingFindingFixtureSourcePresent        = "timing.fixture.source_present"
	TimingFindingFixtureConsumerPresent      = "timing.fixture.consumer_present"
	TimingFindingClockSourceProximity        = "timing.clock_source.proximity"
	TimingFindingLoadCapsPresent             = "timing.load_caps.present"
	TimingFindingLoadCapsProximity           = "timing.load_caps.proximity"
	TimingFindingLoadCapsSymmetry            = "timing.load_caps.symmetry"
	TimingFindingClockRoutesPresent          = "timing.clock_routes.present"
	TimingFindingClockRoutesLength           = "timing.clock_routes.length"
	TimingFindingGroundReturnPresent         = "timing.ground_return.present"
	TimingFindingDecouplingPresent           = "timing.decoupling.present"
	TimingFindingDecouplingProximity         = "timing.decoupling.proximity"
	TimingFindingEnableControlPresent        = "timing.enable_control.present"
	TimingFindingResetProgrammingRouteLength = "timing.reset_programming.route_length"
	TimingFindingProgrammingGroundReference  = "timing.programming.ground_reference"
)

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
		if !realizationWhenMatches(component.When, output.Instance.Params) {
			continue
		}
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
		if !realizationWhenMatches(route.When, output.Instance.Params) {
			continue
		}
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
		realizedRoute.LengthMM = routePointLength(points)
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
	result.Timing = buildTimingFixtureEvidence(activeTimingFixtures(definition.PCBRealization.TimingFixtures, output.Instance.Params), output, result.Components, result.LocalRoutes)
	result.Metadata["components"] = RealizationMetric{Count: len(result.Components)}
	result.Metadata["local_routes"] = RealizationMetric{Count: len(result.LocalRoutes)}
	result.Metadata["timing"] = RealizationMetric{Count: len(result.Timing)}
	return result
}

func activeTimingFixtures(fixtures []PCBTimingFixture, params map[string]any) []PCBTimingFixture {
	if len(fixtures) == 0 {
		return nil
	}
	active := make([]PCBTimingFixture, 0, len(fixtures))
	for _, fixture := range fixtures {
		if realizationWhenMatches(fixture.When, params) {
			active = append(active, fixture)
		}
	}
	return active
}

func realizationWhenMatches(condition RealizationWhen, params map[string]any) bool {
	for key, want := range condition.Params {
		if !parameterValuesEqual(params[key], want) {
			return false
		}
	}
	return true
}

func parameterValuesEqual(got any, want any) bool {
	switch expected := want.(type) {
	case bool:
		value, ok := got.(bool)
		return ok && value == expected
	case string:
		value, ok := got.(string)
		return ok && strings.TrimSpace(value) == expected
	default:
		return reflect.DeepEqual(got, want)
	}
}

func buildTimingFixtureEvidence(fixtures []PCBTimingFixture, output BlockOutput, components []RealizedPCBComponent, routes []RealizedPCBLocalRoute) []TimingFixtureEvidence {
	if len(fixtures) == 0 {
		return nil
	}
	componentsByRole := realizedComponentsByRole(components)
	routesByID := realizedRoutesByID(routes)
	evidence := make([]TimingFixtureEvidence, 0, len(fixtures))
	for _, fixture := range fixtures {
		item := TimingFixtureEvidence{
			ID:                            fixture.ID,
			TimingGroupID:                 fixture.TimingGroupID,
			Kind:                          fixture.Kind,
			MaxSourceToConsumerDistanceMM: cloneFloat64Ptr(fixture.MaxSourceToConsumerDistanceMM),
			MaxLoadCapDistanceMM:          cloneFloat64Ptr(fixture.MaxLoadCapDistanceMM),
			MaxDecouplingDistanceMM:       cloneFloat64Ptr(fixture.MaxDecouplingDistanceMM),
			MaxLoadCapAsymmetryMM:         cloneFloat64Ptr(fixture.MaxLoadCapAsymmetryMM),
			MaxClockRouteLengthMM:         cloneFloat64Ptr(fixture.MaxClockRouteLengthMM),
			GroundNet:                     InstanceNetName(output.Instance.InstanceID, fixture.GroundNetTemplate),
			Roles:                         cloneTimingRoleMap(fixture.Roles),
			Satisfied:                     true,
		}
		source, hasSource := componentsByRole[fixture.SourceRole]
		if hasSource {
			item.SourceRef = source.Ref
		} else {
			item.Findings = append(item.Findings, timingFinding(TimingFindingFixtureSourcePresent, reports.SeverityError, "timing fixture source component is missing", nil, nil, nil, nil))
		}
		if fixture.ConsumerRole != "" {
			if consumer, ok := componentsByRole[fixture.ConsumerRole]; ok {
				item.ConsumerRef = consumer.Ref
				if hasSource {
					distance := placementDistance(source.Placement, consumer.Placement)
					item.SourceToConsumerDistanceMM = &distance
					item.Findings = appendThresholdFinding(item.Findings, TimingFindingClockSourceProximity, "clock source exceeds maximum source-to-consumer distance", distance, fixture.MaxSourceToConsumerDistanceMM, []string{source.Ref, consumer.Ref}, item.ClockNets)
				}
			} else {
				item.Findings = append(item.Findings, timingFinding(TimingFindingFixtureConsumerPresent, reports.SeverityWarning, "timing fixture consumer component is missing", nil, nil, nil, nil))
			}
		}
		item.LoadCapacitorDistancesMM = map[string]float64{}
		loadDistances := []float64{}
		for _, role := range fixture.LoadCapacitorRoles {
			load, ok := componentsByRole[role]
			if !ok {
				item.Findings = append(item.Findings, timingFinding(TimingFindingLoadCapsPresent, reports.SeverityError, "timing load capacitor component is missing", nil, nil, nil, nil))
				continue
			}
			item.LoadCapacitorRefs = append(item.LoadCapacitorRefs, load.Ref)
			if hasSource {
				distance := placementDistance(source.Placement, load.Placement)
				item.LoadCapacitorDistancesMM[load.Ref] = distance
				loadDistances = append(loadDistances, distance)
				item.Findings = appendThresholdFinding(item.Findings, TimingFindingLoadCapsProximity, "load capacitor exceeds maximum source distance", distance, fixture.MaxLoadCapDistanceMM, []string{source.Ref, load.Ref}, nil)
			}
		}
		if len(item.LoadCapacitorDistancesMM) == 0 {
			item.LoadCapacitorDistancesMM = nil
		}
		if len(fixture.DecouplingRoles) != 0 {
			item.DecouplingDistancesMM = map[string]float64{}
		}
		for _, role := range fixture.DecouplingRoles {
			decoupling, ok := componentsByRole[role]
			if !ok {
				item.Findings = append(item.Findings, timingFinding(TimingFindingDecouplingPresent, reports.SeverityError, fmt.Sprintf("timing decoupling component for role %q is missing", role), nil, nil, nil, nil))
				continue
			}
			item.DecouplingRefs = append(item.DecouplingRefs, decoupling.Ref)
			if hasSource {
				distance := placementDistance(source.Placement, decoupling.Placement)
				item.DecouplingDistancesMM[decoupling.Ref] = distance
				item.Findings = appendThresholdFinding(item.Findings, TimingFindingDecouplingProximity, "decoupling component exceeds maximum source distance", distance, fixture.MaxDecouplingDistanceMM, []string{source.Ref, decoupling.Ref}, nil)
			}
		}
		if len(item.DecouplingDistancesMM) == 0 {
			item.DecouplingDistancesMM = nil
		}
		for _, role := range fixture.EnableControlRoles {
			control, ok := componentsByRole[role]
			if !ok {
				item.Findings = append(item.Findings, timingFinding(TimingFindingEnableControlPresent, reports.SeverityError, fmt.Sprintf("timing enable/control component for role %q is missing", role), nil, nil, nil, nil))
				continue
			}
			item.EnableControlRefs = append(item.EnableControlRefs, control.Ref)
		}
		item.EnableControlPresent = len(fixture.EnableControlRoles) != 0 && len(item.EnableControlRefs) == len(fixture.EnableControlRoles)
		if len(loadDistances) >= 2 {
			asymmetry := math.Abs(loadDistances[0] - loadDistances[1])
			item.LoadCapacitorAsymmetryMM = &asymmetry
			item.Findings = appendThresholdFinding(item.Findings, TimingFindingLoadCapsSymmetry, "load capacitor placement exceeds symmetry tolerance", asymmetry, fixture.MaxLoadCapAsymmetryMM, item.LoadCapacitorRefs, nil)
		}
		item.ClockRouteLengthsMM = map[string]float64{}
		clockNetSet := map[string]struct{}{}
		for _, netTemplate := range fixture.ClockNetTemplates {
			netName := InstanceNetName(output.Instance.InstanceID, netTemplate)
			clockNetSet[netName] = struct{}{}
			item.ClockNets = append(item.ClockNets, netName)
		}
		for _, routeID := range fixture.LocalRouteIDs {
			route, ok := routesByID[routeID]
			if !ok {
				item.Findings = append(item.Findings, timingFinding(TimingFindingClockRoutesPresent, reports.SeverityError, "timing local route is missing", nil, nil, nil, nil))
				continue
			}
			if _, ok := clockNetSet[route.NetName]; ok {
				item.ClockRouteLengthsMM[route.ID] = route.LengthMM
				findingID := TimingFindingClockRoutesLength
				message := "timing route exceeds maximum length"
				if fixture.Kind == PCBTimingKindReset {
					findingID = TimingFindingResetProgrammingRouteLength
					message = "reset/programming route exceeds maximum length"
				}
				item.Findings = appendThresholdFinding(item.Findings, findingID, message, route.LengthMM, fixture.MaxClockRouteLengthMM, []string{route.From.Ref, route.To.Ref}, []string{route.NetName})
			}
			if route.NetName == item.GroundNet {
				item.GroundReturnPresent = true
			}
		}
		if len(item.ClockRouteLengthsMM) == 0 {
			item.ClockRouteLengthsMM = nil
		}
		if fixture.GroundNetTemplate != "" && !item.GroundReturnPresent {
			findingID := TimingFindingGroundReturnPresent
			message := "timing fixture has no local ground return evidence"
			refs := item.LoadCapacitorRefs
			if len(refs) == 0 {
				refs = compactStrings(item.SourceRef, item.ConsumerRef)
			}
			if fixture.Kind == PCBTimingKindReset {
				findingID = TimingFindingProgrammingGroundReference
				message = "programming header has no local ground reference evidence"
			}
			item.Findings = append(item.Findings, timingFinding(findingID, reports.SeverityError, message, nil, nil, refs, []string{item.GroundNet}))
		}
		item.Satisfied = timingFindingsSatisfied(item.Findings)
		evidence = append(evidence, item)
	}
	return evidence
}

func realizedComponentsByRole(components []RealizedPCBComponent) map[string]RealizedPCBComponent {
	byRole := map[string]RealizedPCBComponent{}
	for _, component := range components {
		if role := strings.TrimSpace(component.ComponentRole); role != "" {
			byRole[role] = component
		}
	}
	return byRole
}

func realizedRoutesByID(routes []RealizedPCBLocalRoute) map[string]RealizedPCBLocalRoute {
	byID := map[string]RealizedPCBLocalRoute{}
	for _, route := range routes {
		if id := strings.TrimSpace(route.ID); id != "" {
			byID[id] = route
		}
	}
	return byID
}

func placementDistance(first RelativePlacement, second RelativePlacement) float64 {
	return math.Hypot(first.XMM-second.XMM, first.YMM-second.YMM)
}

func appendThresholdFinding(findings []TimingFixtureFinding, id string, message string, measured float64, threshold *float64, refs []string, nets []string) []TimingFixtureFinding {
	if threshold == nil || measured <= *threshold {
		return findings
	}
	return append(findings, timingFinding(id, reports.SeverityError, message, &measured, threshold, refs, nets))
}

func timingFinding(id string, severity reports.Severity, message string, measured *float64, threshold *float64, refs []string, nets []string) TimingFixtureFinding {
	return TimingFixtureFinding{
		ID:          id,
		Severity:    severity,
		Message:     message,
		MeasuredMM:  cloneFloat64Ptr(measured),
		ThresholdMM: cloneFloat64Ptr(threshold),
		Refs:        append([]string(nil), refs...),
		Nets:        append([]string(nil), nets...),
	}
}

func timingFindingsSatisfied(findings []TimingFixtureFinding) bool {
	for _, finding := range findings {
		if finding.Severity == reports.SeverityError || finding.Severity == reports.SeverityBlocked {
			return false
		}
	}
	return true
}

func compactStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
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

func routePointLength(points []transactions.Point) float64 {
	if len(points) < 2 {
		return 0
	}
	length := 0.0
	for index := 1; index < len(points); index++ {
		length += math.Hypot(points[index].XMM-points[index-1].XMM, points[index].YMM-points[index-1].YMM)
	}
	return length
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
