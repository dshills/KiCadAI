package circuitgraph

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

const (
	CodeSchemaInvalid             reports.Code = "GRAPH_SCHEMA_INVALID"
	CodeLimitExceeded             reports.Code = "GRAPH_LIMIT_EXCEEDED"
	CodeComponentDuplicate        reports.Code = "GRAPH_COMPONENT_DUPLICATE"
	CodeComponentSelectionInvalid reports.Code = "GRAPH_COMPONENT_SELECTION_INVALID"
	CodeUnitInvalid               reports.Code = "GRAPH_UNIT_INVALID"
	CodeNetInvalid                reports.Code = "GRAPH_NET_INVALID"
	CodePowerFlagInvalid          reports.Code = "GRAPH_POWER_FLAG_INVALID"
	CodeEndpointDuplicate         reports.Code = "GRAPH_ENDPOINT_DUPLICATE"
	CodeLayoutUnsupported         reports.Code = "GRAPH_LAYOUT_UNSUPPORTED"
	CodePCBConstraintInvalid      reports.Code = "GRAPH_PCB_CONSTRAINT_INVALID"
	CodeRepairForbidden           reports.Code = "GRAPH_REPAIR_FORBIDDEN"
	CodeLegacyLaneInferred        reports.Code = "GRAPH_LEGACY_LANE_INFERRED"
)

var (
	graphIDPattern     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,62}$`)
	projectNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
	referencePattern   = regexp.MustCompile(`^[A-Za-z#][A-Za-z0-9_#-]*[0-9][A-Za-z0-9_-]*$`)
)

func Validate(document Document) []reports.Issue {
	validator := graphValidator{document: document}
	validator.header()
	componentsByID := validator.components()
	netsByName, connected := validator.nets(componentsByID)
	validator.powerFlags(netsByName)
	validator.noConnects(componentsByID, connected)
	validator.buses(netsByName)
	validator.schematic(componentsByID)
	validator.pcb(componentsByID, netsByName)
	validator.policy()
	return finalizeGraphIssues(validator.issues)
}

type graphValidator struct {
	document         Document
	issues           []reports.Issue
	unitsByComponent map[string]map[string]struct{}
}

func (validator *graphValidator) add(code reports.Code, path, message string) {
	validator.issues = append(validator.issues, graphIssue(code, path, message))
}

func (validator *graphValidator) header() {
	if validator.document.Schema != SchemaID {
		validator.add(CodeSchemaInvalid, "schema", "schema must be "+SchemaID)
	}
	if validator.document.Version != Version {
		validator.add(CodeSchemaInvalid, "version", fmt.Sprintf("version must be %d", Version))
	}
	project := validator.document.Project
	if !projectNamePattern.MatchString(project.Name) || strings.Contains(project.Name, "..") {
		validator.add(CodeSchemaInvalid, "project.name", "project name must be a safe basename")
	}
	validator.boundedString("project.title", project.Title, MaxStringBytes)
	validator.boundedString("project.description", project.Description, MaxDescriptionBytes)
	if !validAcceptance(project.Acceptance) {
		validator.add(CodeSchemaInvalid, "project.acceptance", "unsupported acceptance level")
	}
	if !finiteInRange(project.Board.WidthMM, 0, MaxBoardDimensionMM, false) {
		validator.add(CodeSchemaInvalid, "project.board.width_mm", "board width must be finite, positive, and at most 1000 mm")
	}
	if !finiteInRange(project.Board.HeightMM, 0, MaxBoardDimensionMM, false) {
		validator.add(CodeSchemaInvalid, "project.board.height_mm", "board height must be finite, positive, and at most 1000 mm")
	}
	if project.Board.Layers != 2 && project.Board.Layers != 4 {
		validator.add(CodeSchemaInvalid, "project.board.layers", "board layers must be 2 or 4")
	}
	if !finiteInRange(project.Board.EdgeClearanceMM, 0, MaxBoardDimensionMM, true) {
		validator.add(CodeSchemaInvalid, "project.board.edge_clearance_mm", "edge clearance must be finite and non-negative")
	}
}

func (validator *graphValidator) components() map[string]Component {
	if len(validator.document.Components) == 0 {
		validator.add(CodeSchemaInvalid, "components", "at least one component is required")
	}
	if len(validator.document.Components) > MaxComponents {
		validator.add(CodeLimitExceeded, "components", "component count exceeds limit")
	}
	byID := make(map[string]Component, len(validator.document.Components))
	validator.unitsByComponent = make(map[string]map[string]struct{}, len(validator.document.Components))
	references := map[string]int{}
	for index, component := range validator.document.Components {
		path := fmt.Sprintf("components[%d]", index)
		if !graphIDPattern.MatchString(component.ID) {
			validator.add(CodeSchemaInvalid, path+".id", "component id must be a safe identifier")
		} else if _, exists := byID[component.ID]; exists {
			validator.add(CodeComponentDuplicate, path+".id", "duplicate component id "+component.ID)
		}
		byID[component.ID] = component
		if component.Reference != "" {
			if !referencePattern.MatchString(component.Reference) {
				validator.add(CodeSchemaInvalid, path+".reference", "reference must be a safe KiCad designator containing a numeric index")
			} else if previous, exists := references[component.Reference]; exists {
				validator.add(CodeComponentDuplicate, path+".reference", fmt.Sprintf("reference duplicates components[%d]", previous))
			}
			references[component.Reference] = index
			if strings.HasPrefix(strings.ToUpper(component.Reference), "#FLG") {
				validator.add(CodePowerFlagInvalid, path+".reference", "#FLG reference prefix is reserved for generated power flags")
			}
		}
		if !validComponentRole(component.Role) {
			validator.add(CodeSchemaInvalid, path+".role", "unsupported component role")
		}
		if len(component.Units) > MaxUnitsPerComponent {
			validator.add(CodeLimitExceeded, path+".units", "component unit count exceeds limit")
		}
		unitIDs := map[string]struct{}{}
		validator.unitsByComponent[component.ID] = unitIDs
		for unitIndex, unit := range component.Units {
			unitPath := fmt.Sprintf("%s.units[%d]", path, unitIndex)
			unitID := canonicalUnitID(unit.ID)
			if !graphIDPattern.MatchString(unitID) {
				validator.add(CodeUnitInvalid, unitPath+".id", "unit id must be a safe identifier")
			}
			if _, exists := unitIDs[unitID]; exists {
				validator.add(CodeUnitInvalid, unitPath+".id", "duplicate component unit "+unitID)
			}
			unitIDs[unitID] = struct{}{}
			if !graphIDPattern.MatchString(unit.Role) {
				validator.add(CodeUnitInvalid, unitPath+".role", "unit role must be a safe identifier")
			}
		}
		hasID := strings.TrimSpace(component.ComponentID) != ""
		hasQuery := component.Query != nil
		if hasID == hasQuery {
			validator.add(CodeComponentSelectionInvalid, path, "declare exactly one of component_id or query")
		}
		if !hasID && component.VariantID != "" {
			validator.add(CodeComponentSelectionInvalid, path+".variant_id", "variant_id requires component_id")
		}
		if hasQuery {
			validator.query(path+".query", *component.Query)
		}
		validator.boundedString(path+".value", component.Value, MaxStringBytes)
		validator.libraryConstraint(path+".symbol", component.Symbol)
		validator.libraryConstraint(path+".footprint", component.Footprint)
		if component.Population != PopulationPopulate && component.Population != PopulationDoNotPopulate {
			validator.add(CodeSchemaInvalid, path+".population", "population must be populate or do_not_populate")
		}
		parameterNames := map[string]struct{}{}
		for parameterIndex, parameter := range component.Parameters {
			parameterPath := fmt.Sprintf("%s.parameters[%d]", path, parameterIndex)
			if !graphIDPattern.MatchString(parameter.Name) {
				validator.add(CodeSchemaInvalid, parameterPath+".name", "parameter name must be a safe identifier")
			}
			if _, exists := parameterNames[parameter.Name]; exists {
				validator.add(CodeSchemaInvalid, parameterPath+".name", "duplicate parameter name")
			}
			parameterNames[parameter.Name] = struct{}{}
			validator.parameterValue(parameterPath+".value", parameter.Value)
		}
		for ratingIndex, rating := range component.RequiredRatings {
			ratingPath := fmt.Sprintf("%s.required_ratings[%d]", path, ratingIndex)
			if strings.TrimSpace(rating.Kind) == "" || strings.TrimSpace(rating.Value) == "" || strings.TrimSpace(rating.Unit) == "" {
				validator.add(CodeSchemaInvalid, ratingPath, "rating kind, value, and unit are required")
			}
		}
		for functionIndex, function := range component.RequiredFunctions {
			if strings.TrimSpace(function) == "" {
				validator.add(CodeSchemaInvalid, fmt.Sprintf("%s.required_functions[%d]", path, functionIndex), "required function cannot be empty")
			}
		}
		propertyNames := map[string]struct{}{}
		for propertyIndex, property := range component.Properties {
			propertyPath := fmt.Sprintf("%s.properties[%d]", path, propertyIndex)
			if !graphIDPattern.MatchString(property.Name) {
				validator.add(CodeSchemaInvalid, propertyPath+".name", "property name must be a safe identifier")
			}
			if _, exists := propertyNames[property.Name]; exists {
				validator.add(CodeSchemaInvalid, propertyPath+".name", "duplicate property name")
			}
			propertyNames[property.Name] = struct{}{}
			validator.boundedString(propertyPath+".value", property.Value, MaxStringBytes)
		}
	}
	return byID
}

func (validator *graphValidator) powerFlags(netsByName map[string]Net) {
	if len(validator.document.PowerFlags) > MaxPowerFlags {
		validator.add(CodeLimitExceeded, "power_flags", "power flag count exceeds limit")
	}
	seen := map[string]struct{}{}
	for index, flag := range validator.document.PowerFlags {
		path := fmt.Sprintf("power_flags[%d].net", index)
		if strings.TrimSpace(flag.Net) == "" {
			validator.add(CodePowerFlagInvalid, path, "power flag net is required")
			continue
		}
		validator.boundedString(path, flag.Net, MaxStringBytes)
		net, exists := netsByName[flag.Net]
		if !exists {
			validator.add(CodePowerFlagInvalid, path, "power flag references unknown net "+flag.Net)
			continue
		}
		if _, duplicate := seen[flag.Net]; duplicate {
			validator.add(CodePowerFlagInvalid, path, "duplicate power flag for net "+flag.Net)
		}
		seen[flag.Net] = struct{}{}
		if !validPowerFlagNetRole(net.Role) {
			validator.add(CodePowerFlagInvalid, path, "power flag requires a power, ground, or return net")
		}
	}
}

func (validator *graphValidator) query(path string, query ComponentQuery) {
	if strings.TrimSpace(query.Text) == "" && strings.TrimSpace(query.Family) == "" {
		validator.add(CodeComponentSelectionInvalid, path, "query requires text or family")
	}
	if query.MinimumConfidence != "" && !components.ValidConfidence(query.MinimumConfidence) {
		validator.add(CodeComponentSelectionInvalid, path+".minimum_confidence", "invalid confidence level")
	}
	if query.MinimumConfidence == components.ConfidencePlaceholder || query.MinimumConfidence == components.ConfidenceBlocked {
		validator.add(CodeComponentSelectionInvalid, path+".minimum_confidence", "placeholder and blocked confidence are not selectable")
	}
	if !finiteInRange(query.MinVoltageV, 0, math.MaxFloat64, true) {
		validator.add(CodeComponentSelectionInvalid, path+".min_voltage_v", "minimum voltage must be finite and non-negative")
	}
}

func (validator *graphValidator) libraryConstraint(path string, constraint *LibraryConstraint) {
	if constraint == nil {
		return
	}
	if strings.TrimSpace(constraint.LibraryID) == "" || strings.ContainsAny(constraint.LibraryID, "\x00\r\n") {
		validator.add(CodeComponentSelectionInvalid, path+".library_id", "library id is invalid")
	}
}

func (validator *graphValidator) parameterValue(path string, value ParameterValue) {
	set := 0
	if value.String != nil {
		set++
		validator.boundedString(path, *value.String, MaxStringBytes)
	}
	if value.Number != nil {
		set++
		if math.IsNaN(*value.Number) || math.IsInf(*value.Number, 0) {
			validator.add(CodeSchemaInvalid, path, "parameter number must be finite")
		}
	}
	if value.Bool != nil {
		set++
	}
	if value.List != nil {
		set++
		if len(value.List) > 256 {
			validator.add(CodeLimitExceeded, path, "parameter list exceeds limit")
		}
		for index, item := range value.List {
			validator.boundedString(fmt.Sprintf("%s[%d]", path, index), item, MaxStringBytes)
		}
	}
	if set != 1 {
		validator.add(CodeSchemaInvalid, path, "parameter value must contain exactly one supported value")
	}
}

func (validator *graphValidator) nets(componentsByID map[string]Component) (map[string]Net, map[Endpoint]string) {
	if len(validator.document.Nets) == 0 {
		validator.add(CodeNetInvalid, "nets", "at least one net is required")
	}
	if len(validator.document.Nets) > MaxNets {
		validator.add(CodeLimitExceeded, "nets", "net count exceeds limit")
	}
	byName := make(map[string]Net, len(validator.document.Nets))
	// Endpoint is comparable and includes Component, Unit, SelectorKind, and
	// Selector, so identical selectors on distinct multi-unit symbol units do
	// not collide.
	connected := map[Endpoint]string{}
	differentialPairs := map[string]int{}
	totalEndpoints := 0
	for index, net := range validator.document.Nets {
		path := fmt.Sprintf("nets[%d]", index)
		if strings.TrimSpace(net.Name) == "" {
			validator.add(CodeNetInvalid, path+".name", "net name is required")
		} else if _, exists := byName[net.Name]; exists {
			validator.add(CodeNetInvalid, path+".name", "duplicate net name "+net.Name)
		}
		byName[net.Name] = net
		if !validNetRole(net.Role) {
			validator.add(CodeNetInvalid, path+".role", "unsupported net role")
		}
		if net.Required == nil {
			validator.add(CodeNetInvalid, path+".required", "required policy must be explicit")
		}
		if len(net.Endpoints) < 2 {
			validator.add(CodeNetInvalid, path+".endpoints", "ordinary net requires at least two endpoints")
		}
		if len(net.Endpoints) > MaxEndpointsPerNet {
			validator.add(CodeLimitExceeded, path+".endpoints", "endpoint count exceeds per-net limit")
		}
		totalEndpoints += len(net.Endpoints)
		if !finiteInRange(net.CurrentMA, 0, math.MaxFloat64, true) || !finiteInRange(net.WidthMM, 0, MaxBoardDimensionMM, true) || !finiteInRange(net.ClearanceMM, 0, MaxBoardDimensionMM, true) {
			validator.add(CodeNetInvalid, path, "net numeric constraints must be finite and non-negative")
		}
		if net.DifferentialPair != "" {
			differentialPairs[net.DifferentialPair]++
		}
		for endpointIndex, endpoint := range net.Endpoints {
			endpointPath := fmt.Sprintf("%s.endpoints[%d]", path, endpointIndex)
			validator.endpoint(endpointPath, endpoint, componentsByID)
			if previous, exists := connected[endpoint]; exists {
				validator.add(CodeEndpointDuplicate, endpointPath, "endpoint is already connected to net "+previous)
			} else {
				connected[endpoint] = net.Name
			}
		}
	}
	if totalEndpoints > MaxTotalEndpoints {
		validator.add(CodeLimitExceeded, "nets", "total endpoint count exceeds limit")
	}
	for pair, count := range differentialPairs {
		if count != 2 {
			validator.add(CodeNetInvalid, "nets", fmt.Sprintf("differential pair %s must contain exactly two nets", pair))
		}
	}
	return byName, connected
}

func (validator *graphValidator) endpoint(path string, endpoint Endpoint, componentsByID map[string]Component) {
	component, exists := componentsByID[endpoint.Component]
	if !exists {
		validator.add(CodeNetInvalid, path+".component", "endpoint references unknown component "+endpoint.Component)
	}
	if endpoint.Unit != "" && !graphIDPattern.MatchString(endpoint.Unit) {
		validator.add(CodeNetInvalid, path+".unit", "endpoint unit must be a safe identifier")
	}
	if exists && len(component.Units) != 0 {
		if endpoint.Unit == "" {
			validator.add(CodeUnitInvalid, path+".unit", "endpoint unit is required for a named multi-unit component")
		} else if !validator.componentHasUnit(component.ID, endpoint.Unit) {
			validator.add(CodeUnitInvalid, path+".unit", "endpoint references undeclared component unit "+endpoint.Unit)
		}
	}
	if endpoint.SelectorKind != SelectorFunction && endpoint.SelectorKind != SelectorAlias && endpoint.SelectorKind != SelectorSymbolPin {
		validator.add(CodeNetInvalid, path+".selector_kind", "unsupported endpoint selector kind")
	}
	if strings.TrimSpace(endpoint.Selector) == "" {
		validator.add(CodeNetInvalid, path+".selector", "endpoint selector is required")
	}
	validator.boundedString(path+".selector", endpoint.Selector, MaxStringBytes)
}

func (validator *graphValidator) noConnects(componentsByID map[string]Component, connected map[Endpoint]string) {
	if len(validator.document.NoConnects) > MaxNoConnects {
		validator.add(CodeLimitExceeded, "no_connects", "no-connect count exceeds limit")
	}
	seen := map[Endpoint]struct{}{}
	for index, endpoint := range validator.document.NoConnects {
		path := fmt.Sprintf("no_connects[%d]", index)
		validator.endpoint(path, endpoint, componentsByID)
		if netName, exists := connected[endpoint]; exists {
			validator.add(CodeEndpointDuplicate, path, "no-connect endpoint is already on net "+netName)
		}
		if _, exists := seen[endpoint]; exists {
			validator.add(CodeEndpointDuplicate, path, "duplicate no-connect endpoint")
		}
		seen[endpoint] = struct{}{}
	}
}

func (validator *graphValidator) buses(netsByName map[string]Net) {
	if len(validator.document.Buses) > MaxBuses {
		validator.add(CodeLimitExceeded, "buses", "bus count exceeds limit")
	}
	seen := map[string]struct{}{}
	ownedNets := map[string]string{}
	for index, bus := range validator.document.Buses {
		path := fmt.Sprintf("buses[%d]", index)
		if !graphIDPattern.MatchString(bus.ID) {
			validator.add(CodeSchemaInvalid, path+".id", "bus id must be a safe identifier")
		}
		if _, exists := seen[bus.ID]; exists {
			validator.add(CodeSchemaInvalid, path+".id", "duplicate bus id")
		}
		seen[bus.ID] = struct{}{}
		if strings.TrimSpace(bus.Name) == "" || len(bus.Members) == 0 {
			validator.add(CodeSchemaInvalid, path, "bus name and members are required")
		}
		for memberIndex, member := range bus.Members {
			memberPath := fmt.Sprintf("%s.members[%d]", path, memberIndex)
			if _, exists := netsByName[member.Net]; !exists {
				validator.add(CodeNetInvalid, memberPath+".net", "bus member references unknown net "+member.Net)
			}
			if owner, exists := ownedNets[member.Net]; exists {
				validator.add(CodeNetInvalid, memberPath+".net", "net already belongs to bus "+owner)
			}
			ownedNets[member.Net] = bus.ID
			if strings.TrimSpace(member.Label) == "" {
				validator.add(CodeSchemaInvalid, memberPath+".label", "bus member label is required")
			}
		}
	}
}

func (validator *graphValidator) schematic(componentsByID map[string]Component) {
	intent := validator.document.Schematic
	if intent.Flow != FlowLeftToRight || intent.Origin != OriginCentered {
		validator.add(CodeLayoutUnsupported, "schematic", "v1 requires left_to_right flow and centered origin")
	}
	if intent.Lanes.Power != LaneTop || intent.Lanes.Signals != LaneMiddle || intent.Lanes.Ground != LaneBottom {
		validator.add(CodeLayoutUnsupported, "schematic.lanes", "v1 requires power top, signals middle, and ground bottom")
	}
	negativeRails := map[string]struct{}{}
	for _, net := range validator.document.Nets {
		if net.Role == NetRolePowerNeg {
			negativeRails[net.Name] = struct{}{}
		}
	}
	if len(negativeRails) == 0 && intent.Lanes.PowerNegative != nil {
		validator.add(CodeLayoutUnsupported, "schematic.lanes.power_negative", "power_negative must be null when the graph has no power_neg net")
	}
	if len(negativeRails) != 0 && (intent.Lanes.PowerNegative == nil || *intent.Lanes.PowerNegative != LaneLower) {
		validator.add(CodeLayoutUnsupported, "schematic.lanes.power_negative", "power_negative must be lower when the graph has a power_neg net")
	}
	if intent.Rules.PositivePowerTop == nil || intent.Rules.GroundBottom == nil || intent.Rules.CenterOnPage == nil || intent.Rules.PreferLabelsForLongNets == nil || intent.Rules.AvoidWireCrossings == nil {
		validator.add(CodeLayoutUnsupported, "schematic.rules", "all schematic rule booleans are required")
	}
	if !finiteInRange(intent.Rules.MinGroupSpacingMM, 0, MaxBoardDimensionMM, false) || !finiteInRange(intent.Rules.MinComponentSpacingMM, 0, MaxBoardDimensionMM, false) {
		validator.add(CodeLayoutUnsupported, "schematic.rules", "schematic spacing must be finite and positive")
	}
	groups := map[string]struct{}{}
	memberOwner := map[string]string{}
	for index, group := range intent.Groups {
		path := fmt.Sprintf("schematic.groups[%d]", index)
		if !graphIDPattern.MatchString(group.ID) {
			validator.add(CodeLayoutUnsupported, path+".id", "group id must be a safe identifier")
		}
		if group.Side != "" && !validSide(group.Side) {
			validator.add(CodeLayoutUnsupported, path+".side", "unsupported group side")
		}
		if _, exists := groups[group.ID]; exists {
			validator.add(CodeLayoutUnsupported, path+".id", "duplicate group id")
		}
		groups[group.ID] = struct{}{}
		for memberIndex, member := range group.Members {
			if _, exists := componentsByID[member]; !exists {
				validator.add(CodeLayoutUnsupported, fmt.Sprintf("%s.members[%d]", path, memberIndex), "group references unknown component "+member)
			}
			if owner, exists := memberOwner[member]; exists {
				validator.add(CodeLayoutUnsupported, fmt.Sprintf("%s.members[%d]", path, memberIndex), "component already belongs to group "+owner)
			}
			memberOwner[member] = group.ID
		}
	}
	placed := map[string]struct{}{}
	for index, placement := range intent.Placements {
		path := fmt.Sprintf("schematic.placements[%d]", index)
		component, exists := componentsByID[placement.Component]
		if !exists {
			validator.add(CodeLayoutUnsupported, path+".component", "placement references unknown component")
		}
		unitID := canonicalUnitID(placement.Unit)
		placementID := placement.Component + "\x00" + unitID
		if _, exists := placed[placementID]; exists {
			validator.add(CodeLayoutUnsupported, path+".unit", "duplicate component unit placement")
		}
		placed[placementID] = struct{}{}
		if unitID != "" {
			if !graphIDPattern.MatchString(unitID) {
				validator.add(CodeLayoutUnsupported, path+".unit", "placement unit must be a safe identifier")
			} else if exists && !validator.componentHasUnit(component.ID, unitID) {
				validator.add(CodeLayoutUnsupported, path+".unit", "placement references undeclared component unit "+unitID)
			}
		}
		if placement.Group != "" {
			if _, exists := groups[placement.Group]; !exists {
				validator.add(CodeLayoutUnsupported, path+".group", "placement references unknown group")
			}
		}
		for field, target := range map[string]string{"near": placement.Near, "above": placement.Above, "right_of": placement.RightOf} {
			if target != "" {
				if _, exists := componentsByID[target]; !exists {
					validator.add(CodeLayoutUnsupported, path+"."+field, "placement references unknown component "+target)
				}
			}
		}
		for _, relation := range []struct {
			field, target, unitField, unit string
		}{
			{field: "near", target: placement.Near, unitField: "near_unit", unit: placement.NearUnit},
			{field: "above", target: placement.Above, unitField: "above_unit", unit: placement.AboveUnit},
			{field: "right_of", target: placement.RightOf, unitField: "right_of_unit", unit: placement.RightOfUnit},
		} {
			unit := canonicalUnitID(relation.unit)
			if unit == "" {
				continue
			}
			if relation.target == "" {
				validator.add(CodeLayoutUnsupported, path+"."+relation.unitField, relation.unitField+" requires "+relation.field)
				continue
			}
			target, targetExists := componentsByID[relation.target]
			if !graphIDPattern.MatchString(unit) {
				validator.add(CodeLayoutUnsupported, path+"."+relation.unitField, "relationship unit must be a safe identifier")
			} else if targetExists && !validator.componentHasUnit(target.ID, unit) {
				validator.add(CodeLayoutUnsupported, path+"."+relation.unitField, "relationship references undeclared target unit "+unit)
			}
		}
		if placement.Orientation != "" && placement.Orientation != "normal" && placement.Orientation != "rotated_90" && placement.Orientation != "rotated_180" && placement.Orientation != "rotated_270" {
			validator.add(CodeLayoutUnsupported, path+".orientation", "unsupported orientation")
		}
		if placement.Mirror != "" && placement.Mirror != "none" && placement.Mirror != "x" && placement.Mirror != "y" {
			validator.add(CodeLayoutUnsupported, path+".mirror", "unsupported mirror")
		}
	}
	if intent.Hierarchy.Mode != "flat" && intent.Hierarchy.Mode != "auto" {
		validator.add(CodeLayoutUnsupported, "schematic.hierarchy.mode", "hierarchy mode must be flat or auto")
	}
	if intent.Hierarchy.MaxComponentsPerSheet < 0 || intent.Hierarchy.MaxComponentsPerSheet > MaxComponents {
		validator.add(CodeLayoutUnsupported, "schematic.hierarchy.max_components_per_sheet", "hierarchy component limit is invalid")
	}
}

func (validator *graphValidator) componentHasUnit(componentID, unitID string) bool {
	_, exists := validator.unitsByComponent[componentID][canonicalUnitID(unitID)]
	return exists
}

func (validator *graphValidator) pcb(componentsByID map[string]Component, netsByName map[string]Net) {
	regions := map[string]PCBRegion{}
	for index, region := range validator.document.PCB.Regions {
		path := fmt.Sprintf("pcb.regions[%d]", index)
		if !graphIDPattern.MatchString(region.ID) {
			validator.add(CodePCBConstraintInvalid, path+".id", "region id must be a safe identifier")
		}
		if _, exists := regions[region.ID]; exists {
			validator.add(CodePCBConstraintInvalid, path+".id", "duplicate region id")
		}
		regions[region.ID] = region
		validator.bounds(path+".bounds", region.Bounds)
	}
	placements := map[string]struct{}{}
	for index, placement := range validator.document.PCB.Placements {
		path := fmt.Sprintf("pcb.placements[%d]", index)
		if _, exists := componentsByID[placement.Component]; !exists {
			validator.add(CodePCBConstraintInvalid, path+".component", "placement references unknown component")
		}
		if _, exists := placements[placement.Component]; exists {
			validator.add(CodePCBConstraintInvalid, path+".component", "duplicate PCB placement")
		}
		placements[placement.Component] = struct{}{}
		if placement.Region != "" {
			if _, exists := regions[placement.Region]; !exists {
				validator.add(CodePCBConstraintInvalid, path+".region", "placement references unknown region")
			}
		}
		if placement.Near != "" {
			if _, exists := componentsByID[placement.Near]; !exists {
				validator.add(CodePCBConstraintInvalid, path+".near", "placement references unknown component")
			}
		}
		if !finiteInRange(placement.MaxDistanceMM, 0, MaxBoardDimensionMM, true) {
			validator.add(CodePCBConstraintInvalid, path+".max_distance_mm", "maximum distance must be finite and non-negative")
		}
		if placement.Edge != "" && !validSide(placement.Edge) {
			validator.add(CodePCBConstraintInvalid, path+".edge", "unsupported board edge")
		}
	}
	keepouts := map[string]struct{}{}
	for index, keepout := range validator.document.PCB.Keepouts {
		path := fmt.Sprintf("pcb.keepouts[%d]", index)
		if !graphIDPattern.MatchString(keepout.ID) {
			validator.add(CodePCBConstraintInvalid, path+".id", "keepout id must be a safe identifier")
		}
		if _, exists := keepouts[keepout.ID]; exists {
			validator.add(CodePCBConstraintInvalid, path+".id", "duplicate keepout id")
		}
		keepouts[keepout.ID] = struct{}{}
		validator.bounds(path+".bounds", keepout.Bounds)
		if len(keepout.Layers) == 0 {
			validator.add(CodePCBConstraintInvalid, path+".layers", "keepout layers are required")
		}
	}
	for index, zone := range validator.document.PCB.Zones {
		path := fmt.Sprintf("pcb.zones[%d]", index)
		if _, exists := netsByName[zone.Net]; !exists {
			validator.add(CodePCBConstraintInvalid, path+".net", "zone references unknown net")
		}
		if len(zone.Layers) == 0 {
			validator.add(CodePCBConstraintInvalid, path+".layers", "zone layers are required")
		}
		if !finiteInRange(zone.ClearanceMM, 0, MaxBoardDimensionMM, true) {
			validator.add(CodePCBConstraintInvalid, path+".clearance_mm", "zone clearance must be finite and non-negative")
		}
	}
}

func (validator *graphValidator) bounds(path string, bounds Bounds) {
	board := validator.document.Project.Board
	if !finiteInRange(bounds.XMM, 0, board.WidthMM, true) || !finiteInRange(bounds.YMM, 0, board.HeightMM, true) || !finiteInRange(bounds.WidthMM, 0, board.WidthMM, false) || !finiteInRange(bounds.HeightMM, 0, board.HeightMM, false) || bounds.XMM+bounds.WidthMM > board.WidthMM || bounds.YMM+bounds.HeightMM > board.HeightMM {
		validator.add(CodePCBConstraintInvalid, path, "bounds must be finite, positive, and inside the board")
	}
}

func (validator *graphValidator) policy() {
	policy := validator.document.Policy
	for path, value := range map[string]*bool{
		"allow_reference_assignment": policy.AllowReferenceAssignment,
		"allow_value_normalization":  policy.AllowValueNormalization,
		"allow_layout_inference":     policy.AllowLayoutInference,
		"allow_spacing_adjustment":   policy.AllowSpacingAdjustment,
		"allow_label_insertion":      policy.AllowLabelInsertion,
		"allow_placement_adjustment": policy.AllowPlacementAdjustment,
		"allow_route_retry":          policy.AllowRouteRetry,
	} {
		if value == nil {
			validator.add(CodeRepairForbidden, "policy."+path, "policy decision must be explicit")
		}
	}
}

func (validator *graphValidator) boundedString(path, value string, limit int) {
	if !utf8.ValidString(value) || len(value) > limit || strings.ContainsRune(value, '\x00') {
		validator.add(CodeSchemaInvalid, path, "string is invalid or exceeds its size limit")
	}
}

func graphIssue(code reports.Code, path, message string) reports.Issue {
	return annotateGraphIssue(reports.Issue{Code: code, Severity: reports.SeverityError, Path: path, Message: message})
}

func finiteInRange(value, minimum, maximum float64, allowMinimum bool) bool {
	if math.IsNaN(value) || math.IsInf(value, 0) || value > maximum {
		return false
	}
	if allowMinimum {
		return value >= minimum
	}
	return value > minimum
}

func validAcceptance(value AcceptanceLevel) bool {
	switch value {
	case AcceptanceStructural, AcceptanceConnectivity, AcceptanceERCDRC, AcceptanceFabricationCandidate:
		return true
	default:
		return false
	}
}

func validComponentRole(value ComponentRole) bool {
	switch value {
	case RoleConnector, RoleInputConnector, RoleOutputConnector, RoleResistor, RoleCurrentLimiter, RolePullup, RoleCapacitor, RoleDecouplingCapacitor, RoleBulkCapacitor, RoleInductor, RoleDiode, RoleIndicatorLED, RoleIC, RoleSensor, RoleRegulator, RoleTransistor, RoleBJT, RoleMOSFET, RoleSwitch, RoleCrystal, RoleOscillator, RoleProtection, RoleFuse, RoleTVS, RolePowerSymbol, RoleGroundSymbol, RoleTestpoint, RoleGeneric:
		return true
	default:
		return false
	}
}

func validNetRole(value NetRole) bool {
	switch value {
	case NetRoleSignal, NetRolePower, NetRolePowerPos, NetRolePowerNeg, NetRoleGround, NetRoleReturn, NetRoleFeedback, NetRoleBias, NetRoleShield:
		return true
	default:
		return false
	}
}

func validPowerFlagNetRole(value NetRole) bool {
	switch value {
	case NetRolePower, NetRolePowerPos, NetRolePowerNeg, NetRoleGround, NetRoleReturn:
		return true
	default:
		return false
	}
}

func validSide(value Side) bool {
	switch value {
	case SideLeft, SideRight, SideTop, SideBottom:
		return true
	default:
		return false
	}
}
