package designworkflow

import (
	"fmt"
	"sort"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
)

const (
	inferredInputStagePriority = iota * 20
	inferredRegulatorStagePriority
	inferredProcessingStagePriority
	inferredOutputStagePriority
)

type inferredSchematicInstance struct {
	id         string
	components []schematicir.Component
	anchor     schematicir.Component
	priority   int
	groupRole  schematicir.GroupRole
	side       schematicir.Side
}

type inferredSchematicGroup struct {
	group    schematicir.Group
	order    int
	anchor   string
	previous string
	relation inferredSchematicGroupRelation
}

type inferredSchematicGroupRelation string

const (
	inferredRelationMain          inferredSchematicGroupRelation = "main"
	inferredRelationInputSupport  inferredSchematicGroupRelation = "input_support"
	inferredRelationLocalSupport  inferredSchematicGroupRelation = "local_support"
	inferredRelationOutputSupport inferredSchematicGroupRelation = "output_support"
)

func inferSchematicLayout(output blocks.CompositionOutput) (schematicir.Layout, []reports.Issue) {
	document, _, err := schematicLayoutDocument(output, schematicir.Layout{})
	if err != nil {
		return schematicir.Layout{}, []reports.Issue{schematicLayoutInferenceIssue(
			"schematic_layout.inference",
			"build topology model: "+err.Error(),
			"provide component instance and role metadata or an explicit schematic_layout",
		)}
	}
	instances, issues := inferredSchematicInstances(document.Circuit.Components)
	if reports.HasBlockingIssue(issues) {
		return schematicir.Layout{}, issues
	}
	issues = append(issues, validateInferredSchematicTopology(document.Circuit.Nets, instances)...)
	if reports.HasBlockingIssue(issues) {
		return schematicir.Layout{}, issues
	}

	trueValue := true
	groupSpacing := 30.48
	componentSpacing := 12.7
	layout := schematicir.Layout{
		Flow:   schematicir.FlowLeftToRight,
		Origin: schematicir.OriginCentered,
		Lanes: schematicir.Lanes{
			Power:   schematicir.LanePositionTop,
			Ground:  schematicir.LanePositionBottom,
			Signals: schematicir.LanePositionMiddle,
		},
		Rules: schematicir.LayoutRules{
			PositivePowerTop:        &trueValue,
			GroundBottom:            &trueValue,
			CenterOnPage:            &trueValue,
			PreferLabelsForLongNets: &trueValue,
			AvoidWireCrossings:      &trueValue,
			MinGroupSpacingMM:       &groupSpacing,
			MinComponentSpacingMM:   &componentSpacing,
		},
	}

	groups := inferredSchematicGroups(instances)
	for index := range groups {
		groups[index].group.Rank = index
		layout.Groups = append(layout.Groups, groups[index].group)
		layout.Placements = append(layout.Placements, inferredSchematicPlacements(groups[index])...)
	}
	return layout, issues
}

func inferredSchematicInstances(components []schematicir.Component) ([]inferredSchematicInstance, []reports.Issue) {
	byInstance := map[string][]schematicir.Component{}
	for _, component := range components {
		instanceID, _, ok := strings.Cut(component.ID, schematicLayoutTargetDelimiter)
		if !ok || strings.TrimSpace(instanceID) == "" {
			return nil, []reports.Issue{schematicLayoutInferenceIssue(
				"schematic_layout.inference.components",
				"component "+component.ID+" does not have a stable instance__role target",
				"use stable block instance and component role metadata",
			)}
		}
		byInstance[instanceID] = append(byInstance[instanceID], component)
	}
	if len(byInstance) == 0 {
		return nil, []reports.Issue{schematicLayoutInferenceIssue(
			"schematic_layout.inference.components",
			"cannot infer schematic layout without components",
			"add supported circuit blocks or provide explicit schematic_layout intent",
		)}
	}

	ids := make([]string, 0, len(byInstance))
	for id := range byInstance {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	instances := make([]inferredSchematicInstance, 0, len(ids))
	var issues []reports.Issue
	priorityOwners := map[int]string{}
	for _, id := range ids {
		instance := inferredSchematicInstance{id: id, components: byInstance[id]}
		for _, component := range instance.components {
			priority, groupRole, side, anchor := inferredSchematicAnchor(component.Role)
			if !anchor {
				continue
			}
			if instance.anchor.ID != "" {
				issues = append(issues, schematicLayoutInferenceIssue(
					"schematic_layout.inference.instances."+id,
					fmt.Sprintf("component roles %s and %s are both stage anchors", instance.anchor.ID, component.ID),
					"provide explicit schematic_layout intent for a multi-anchor block",
				))
				continue
			}
			instance.anchor = component
			instance.priority = priority
			instance.groupRole = groupRole
			instance.side = side
		}
		if instance.anchor.ID == "" {
			issues = append(issues, schematicLayoutInferenceIssue(
				"schematic_layout.inference.instances."+id,
				"no supported input, regulator, processing, or output anchor role was found",
				"provide explicit schematic_layout intent for this block topology",
			))
			continue
		}
		if owner := priorityOwners[instance.priority]; owner != "" {
			issues = append(issues, schematicLayoutInferenceIssue(
				"schematic_layout.inference.instances."+id,
				fmt.Sprintf("stage order is ambiguous with instance %s at priority %d", owner, instance.priority),
				"provide explicit schematic_layout intent for repeated parallel stages",
			))
		} else {
			priorityOwners[instance.priority] = id
		}
		instances = append(instances, instance)
	}
	sort.SliceStable(instances, func(i, j int) bool {
		if instances[i].priority != instances[j].priority {
			return instances[i].priority < instances[j].priority
		}
		return instances[i].id < instances[j].id
	})
	return instances, issues
}

func inferredSchematicAnchor(role schematicir.ComponentRole) (int, schematicir.GroupRole, schematicir.Side, bool) {
	switch role {
	case schematicir.ComponentRoleInputConnector:
		return inferredInputStagePriority, schematicir.GroupRoleInputStage, schematicir.SideLeft, true
	case schematicir.ComponentRoleRegulator:
		return inferredRegulatorStagePriority, schematicir.GroupRoleRegulatorStage, "", true
	case schematicir.ComponentRoleSensor:
		return inferredProcessingStagePriority, schematicir.GroupRoleProcessingStage, "", true
	case schematicir.ComponentRoleOutputConnector:
		return inferredOutputStagePriority, schematicir.GroupRoleOutputStage, schematicir.SideRight, true
	default:
		return 0, "", "", false
	}
}

func validateInferredSchematicTopology(nets []schematicir.Net, instances []inferredSchematicInstance) []reports.Issue {
	adjacent := map[string]map[string]struct{}{}
	for _, net := range nets {
		if net.Role == schematicir.NetRoleGround {
			continue
		}
		members := map[string]struct{}{}
		for _, endpoint := range net.Connect {
			componentID, _, ok := strings.Cut(string(endpoint), ".")
			if !ok {
				continue
			}
			instanceID, _, ok := strings.Cut(componentID, schematicLayoutTargetDelimiter)
			if ok {
				members[instanceID] = struct{}{}
			}
		}
		ids := make([]string, 0, len(members))
		for id := range members {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for i, from := range ids {
			if adjacent[from] == nil {
				adjacent[from] = map[string]struct{}{}
			}
			for _, to := range ids[i+1:] {
				adjacent[from][to] = struct{}{}
				if adjacent[to] == nil {
					adjacent[to] = map[string]struct{}{}
				}
				adjacent[to][from] = struct{}{}
			}
		}
	}
	if len(instances) == 0 {
		return nil
	}
	root := instances[0].id
	reachable := inferredSchematicReachable(adjacent, root)
	for _, instance := range instances[1:] {
		if _, ok := reachable[instance.id]; !ok {
			return []reports.Issue{schematicLayoutInferenceIssue(
				"schematic_layout.inference.topology",
				fmt.Sprintf("cannot derive a connected stage topology from %s to %s", root, instance.id),
				"connect the functional stages or provide explicit schematic_layout intent",
			)}
		}
	}
	return nil
}

func inferredSchematicReachable(adjacent map[string]map[string]struct{}, root string) map[string]struct{} {
	queue := []string{root}
	seen := map[string]struct{}{root: {}}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for neighbor := range adjacent[current] {
			if _, exists := seen[neighbor]; exists {
				continue
			}
			seen[neighbor] = struct{}{}
			queue = append(queue, neighbor)
		}
	}
	return seen
}

func inferredSchematicGroups(instances []inferredSchematicInstance) []inferredSchematicGroup {
	groups := make([]inferredSchematicGroup, 0, len(instances)*2)
	previousAnchor := ""
	for _, instance := range instances {
		main := inferredSchematicGroup{
			group: schematicir.Group{
				ID:    instance.id + "_stage",
				Label: instance.id + " stage",
				Role:  instance.groupRole,
				Side:  instance.side,
			},
			order:    instance.priority * 10,
			anchor:   instance.anchor.ID,
			previous: previousAnchor,
			relation: inferredRelationMain,
		}
		inputSupport := inferredSchematicGroup{
			group: schematicir.Group{ID: instance.id + "_input_support", Label: instance.id + " input support", Role: schematicir.GroupRoleDecouplingStage, Side: schematicir.SideTop},
			order: instance.priority*10 - 2, anchor: instance.anchor.ID, previous: previousAnchor, relation: inferredRelationInputSupport,
		}
		localSupport := inferredSchematicGroup{
			group: schematicir.Group{ID: instance.id + "_support", Label: instance.id + " support", Role: schematicir.GroupRoleDecouplingStage, Side: schematicir.SideTop},
			order: instance.priority*10 - 1, anchor: instance.anchor.ID, previous: previousAnchor, relation: inferredRelationLocalSupport,
		}
		outputSupport := inferredSchematicGroup{
			group: schematicir.Group{ID: instance.id + "_output_support", Label: instance.id + " output support", Role: schematicir.GroupRoleDecouplingStage, Side: schematicir.SideTop},
			order: instance.priority*10 + 1, anchor: instance.anchor.ID, previous: previousAnchor, relation: inferredRelationOutputSupport,
		}
		for _, component := range instance.components {
			_, rawRole, _ := strings.Cut(component.ID, schematicLayoutTargetDelimiter)
			switch {
			case rawRole == "input_capacitor":
				inputSupport.group.Members = append(inputSupport.group.Members, component.ID)
			case rawRole == "output_capacitor":
				outputSupport.group.Members = append(outputSupport.group.Members, component.ID)
			case component.Role == schematicir.ComponentRoleDecouplingCapacitor || component.Role == schematicir.ComponentRolePullup:
				localSupport.group.Members = append(localSupport.group.Members, component.ID)
			default:
				main.group.Members = append(main.group.Members, component.ID)
			}
		}
		for _, group := range []inferredSchematicGroup{inputSupport, localSupport, main, outputSupport} {
			if len(group.group.Members) != 0 {
				groups = append(groups, group)
			}
		}
		previousAnchor = instance.anchor.ID
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].order != groups[j].order {
			return groups[i].order < groups[j].order
		}
		return groups[i].group.ID < groups[j].group.ID
	})
	return groups
}

func inferredSchematicPlacements(group inferredSchematicGroup) []schematicir.Placement {
	placements := make([]schematicir.Placement, 0, len(group.group.Members))
	for _, target := range group.group.Members {
		placement := schematicir.Placement{Target: target, Group: group.group.ID}
		switch group.relation {
		case inferredRelationMain:
			if target == group.anchor {
				if group.previous != "" {
					placement.RightOf = []string{group.previous}
				}
			} else {
				placement.Near = []string{group.anchor}
			}
		case inferredRelationInputSupport, inferredRelationLocalSupport:
			placement.Near = []string{group.anchor}
			placement.Above = []string{group.anchor}
			if group.previous != "" {
				placement.RightOf = []string{group.previous}
			}
		case inferredRelationOutputSupport:
			placement.Near = []string{group.anchor}
			placement.Above = []string{group.anchor}
			placement.RightOf = []string{group.anchor}
		}
		placements = append(placements, placement)
	}
	return placements
}

func schematicLayoutInferenceIssue(path, message, suggestion string) reports.Issue {
	return reports.Issue{
		Code:       reports.CodeInvalidArgument,
		Severity:   reports.SeverityBlocked,
		Path:       path,
		Message:    message,
		Suggestion: suggestion,
	}
}
