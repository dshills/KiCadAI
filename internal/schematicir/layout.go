package schematicir

import "sort"

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
	return document
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
