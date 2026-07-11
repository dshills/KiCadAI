package schematicir

import (
	"math"
	"sort"
	"strings"
)

// CloneLayout returns an independent copy suitable for carrying layout intent
// across parser, planner, and workflow boundaries.
func CloneLayout(layout Layout) Layout {
	clone := layout
	clone.Groups = make([]Group, len(layout.Groups))
	for index, group := range layout.Groups {
		clone.Groups[index] = group
		clone.Groups[index].Members = append([]string(nil), group.Members...)
	}
	clone.Placements = make([]Placement, len(layout.Placements))
	for index, placement := range layout.Placements {
		clone.Placements[index] = placement
		clone.Placements[index].Near = append([]string(nil), placement.Near...)
		clone.Placements[index].Above = append([]string(nil), placement.Above...)
		clone.Placements[index].RightOf = append([]string(nil), placement.RightOf...)
	}
	clone.Buses = make([]BusLayout, len(layout.Buses))
	for index, bus := range layout.Buses {
		clone.Buses[index] = bus
		clone.Buses[index].Points = append([]LayoutPoint(nil), bus.Points...)
		clone.Buses[index].Entries = append([]BusEntryLayout(nil), bus.Entries...)
	}
	clone.Rules.PositivePowerTop = cloneBool(layout.Rules.PositivePowerTop)
	clone.Rules.GroundBottom = cloneBool(layout.Rules.GroundBottom)
	clone.Rules.CenterOnPage = cloneBool(layout.Rules.CenterOnPage)
	clone.Rules.PreferLabelsForLongNets = cloneBool(layout.Rules.PreferLabelsForLongNets)
	clone.Rules.AvoidWireCrossings = cloneBool(layout.Rules.AvoidWireCrossings)
	clone.Rules.MinGroupSpacingMM = cloneFloat(layout.Rules.MinGroupSpacingMM)
	clone.Rules.MinComponentSpacingMM = cloneFloat(layout.Rules.MinComponentSpacingMM)
	return clone
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// PlacementRelationCycle returns one deterministic cycle for a directed
// placement relation, or nil when the relation graph is acyclic.
func PlacementRelationCycle(placements []Placement, relation string) []string {
	graph := map[string][]string{}
	for _, placement := range placements {
		var targets []string
		switch relation {
		case "above":
			targets = placement.Above
		case "right_of":
			targets = placement.RightOf
		default:
			return nil
		}
		graph[placement.Target] = append(graph[placement.Target], targets...)
	}
	nodes := make([]string, 0, len(graph))
	for node := range graph {
		nodes = append(nodes, node)
		sort.Strings(graph[node])
	}
	sort.Strings(nodes)
	state := map[string]uint8{}
	stack := []string{}
	stackIndex := map[string]int{}
	var visit func(string) []string
	visit = func(node string) []string {
		state[node] = 1
		stackIndex[node] = len(stack)
		stack = append(stack, node)
		for _, next := range graph[node] {
			switch state[next] {
			case 0:
				if cycle := visit(next); len(cycle) != 0 {
					return cycle
				}
			case 1:
				start := stackIndex[next]
				cycle := append([]string(nil), stack[start:]...)
				return append(cycle, next)
			}
		}
		stack = stack[:len(stack)-1]
		delete(stackIndex, node)
		state[node] = 2
		return nil
	}
	for _, node := range nodes {
		if state[node] == 0 {
			if cycle := visit(node); len(cycle) != 0 {
				return cycle
			}
		}
	}
	return nil
}

func FormatPlacementRelationCycle(cycle []string) string {
	return strings.Join(cycle, " -> ")
}

// NormalizeLayoutIntent fills in deterministic schematic layout hints that are
// useful for AI-generated schematics but tedious for an AI to specify exactly.
func NormalizeLayoutIntent(document Document) Document {
	document = Normalize(document)
	if len(document.Circuit.Components) == 0 {
		return document
	}

	knownComponents := map[string]ComponentRole{}
	componentGroup := map[string]string{}
	for _, component := range document.Circuit.Components {
		knownComponents[component.ID] = component.Role
	}

	groups := make([]Group, len(document.Layout.Groups))
	for index, group := range document.Layout.Groups {
		groups[index] = group
		groups[index].Members = append([]string(nil), group.Members...)
	}
	groupIndexes := map[string]int{}
	for index := range groups {
		group := &groups[index]
		groupIndexes[group.ID] = index
		for _, member := range group.Members {
			if _, known := knownComponents[member]; known {
				if _, assigned := componentGroup[member]; !assigned {
					componentGroup[member] = group.ID
				}
			}
		}
	}

	generatedGroups := map[string]*Group{}
	for _, component := range document.Circuit.Components {
		if _, assigned := componentGroup[component.ID]; assigned {
			continue
		}
		template := inferredGroupTemplate(component.Role)
		if index, exists := groupIndexes[template.ID]; exists {
			groups[index].Members = append(groups[index].Members, component.ID)
			componentGroup[component.ID] = groups[index].ID
			continue
		}
		group := generatedGroupForTemplate(template, generatedGroups)
		group.Members = append(group.Members, component.ID)
		componentGroup[component.ID] = group.ID
	}

	if len(generatedGroups) != 0 {
		generated := make([]Group, 0, len(generatedGroups))
		for _, group := range generatedGroups {
			generated = append(generated, *group)
		}
		sort.SliceStable(generated, func(left int, right int) bool {
			if generated[left].Rank != generated[right].Rank {
				return generated[left].Rank < generated[right].Rank
			}
			return generated[left].ID < generated[right].ID
		})
		groups = append(groups, generated...)
	}
	document.Layout.Groups = groups
	document.Layout.Placements = normalizePlacements(document.Layout.Placements, document.Circuit.Components, componentGroup)
	document.Layout.Buses = normalizeBusGeometry(document.Layout.Buses)
	return document
}

const schematicIRGridMM = 2.54

func normalizeBusGeometry(layouts []BusLayout) []BusLayout {
	if len(layouts) == 0 {
		return layouts
	}
	normalized := make([]BusLayout, len(layouts))
	for layoutIndex, source := range layouts {
		normalized[layoutIndex] = source
		layout := &normalized[layoutIndex]
		if source.Points != nil {
			layout.Points = make([]LayoutPoint, len(source.Points))
			copy(layout.Points, source.Points)
		}
		if source.Entries != nil {
			layout.Entries = make([]BusEntryLayout, len(source.Entries))
			copy(layout.Entries, source.Entries)
		}
		for pointIndex := range layout.Points {
			layout.Points[pointIndex] = snapBusPoint(layout.Points[pointIndex])
		}
		for entryIndex := range layout.Entries {
			entry := &layout.Entries[entryIndex]
			entry.At = snapBusPoint(entry.At)
			entry.Size.XMM = snapBusSize(entry.Size.XMM)
			entry.Size.YMM = snapBusSize(entry.Size.YMM)
		}
	}
	return normalized
}

func snapBusPoint(point LayoutPoint) LayoutPoint {
	return LayoutPoint{XMM: snapBusCoordinate(point.XMM), YMM: snapBusCoordinate(point.YMM)}
}

func snapBusCoordinate(value float64) float64 {
	return math.Round(value/schematicIRGridMM) * schematicIRGridMM
}

func snapBusSize(value float64) float64 {
	if value == 0 {
		return 0
	}
	snapped := snapBusCoordinate(value)
	if snapped == 0 {
		return math.Copysign(schematicIRGridMM, value)
	}
	return snapped
}

type groupTemplate struct {
	ID    string
	Label string
	Role  GroupRole
	Rank  int
	Side  Side
}

func inferredGroupTemplate(role ComponentRole) groupTemplate {
	switch role {
	case ComponentRoleInputConnector:
		return groupTemplate{ID: "inputs", Label: "Inputs", Role: GroupRoleInputStage, Rank: defaultLayoutInputRank, Side: SideLeft}
	case ComponentRoleOutputConnector:
		return groupTemplate{ID: "outputs", Label: "Outputs", Role: GroupRoleOutputStage, Rank: defaultLayoutOutputRank, Side: SideRight}
	case ComponentRoleRegulator, ComponentRoleProtection, ComponentRoleFuse, ComponentRoleTVS, ComponentRolePowerSymbol:
		return groupTemplate{ID: "power", Label: "Power", Role: GroupRolePowerStage, Rank: defaultLayoutPowerRank, Side: SideTop}
	case ComponentRoleDecouplingCapacitor, ComponentRoleBulkCapacitor, ComponentRoleGroundSymbol:
		return groupTemplate{ID: "decoupling", Label: "Decoupling", Role: GroupRoleDecouplingStage, Rank: defaultLayoutPowerRank, Side: SideBottom}
	case ComponentRoleSensor, ComponentRoleIC:
		return groupTemplate{ID: "processing", Label: "Processing", Role: GroupRoleProcessingStage, Rank: defaultLayoutProcessingRank}
	default:
		return groupTemplate{ID: "signal", Label: "Signal", Role: GroupRoleProcessingStage, Rank: defaultLayoutFallbackRank}
	}
}

func generatedGroupForTemplate(template groupTemplate, generatedGroups map[string]*Group) *Group {
	if group, exists := generatedGroups[template.ID]; exists {
		return group
	}
	group := &Group{
		ID:       template.ID,
		Label:    template.Label,
		Role:     template.Role,
		Rank:     template.Rank,
		Side:     template.Side,
		Inferred: true,
	}
	generatedGroups[template.ID] = group
	return group
}

func normalizePlacements(existing []Placement, components []Component, componentGroup map[string]string) []Placement {
	placements := append([]Placement(nil), existing...)
	seen := map[string]struct{}{}
	for index := range placements {
		placement := &placements[index]
		placement.Near = append([]string(nil), placement.Near...)
		placement.Above = append([]string(nil), placement.Above...)
		placement.RightOf = append([]string(nil), placement.RightOf...)
		sort.Strings(placement.Near)
		sort.Strings(placement.Above)
		sort.Strings(placement.RightOf)
		if placement.Target == "" {
			continue
		}
		seen[placement.Target] = struct{}{}
		if placement.Group == "" {
			placement.Group = componentGroup[placement.Target]
		}
		if placement.Orientation == "" {
			placement.Orientation = OrientationNormal
		}
	}
	for _, component := range components {
		if _, exists := seen[component.ID]; exists {
			continue
		}
		placements = append(placements, Placement{
			Target:      component.ID,
			Group:       componentGroup[component.ID],
			Orientation: orientationForRole(component.Role),
		})
	}
	return placements
}

func orientationForRole(role ComponentRole) Orientation {
	switch role {
	case ComponentRoleResistor, ComponentRoleCurrentLimiter, ComponentRolePullup,
		ComponentRoleCapacitor, ComponentRoleDecouplingCapacitor, ComponentRoleBulkCapacitor,
		ComponentRoleDiode, ComponentRoleIndicatorLED, ComponentRoleFuse, ComponentRoleTVS:
		return OrientationRotated
	default:
		return OrientationNormal
	}
}
