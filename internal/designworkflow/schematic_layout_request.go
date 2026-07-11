package designworkflow

import (
	"fmt"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
)

func validateSchematicLayoutRequest(layout *schematicir.Layout, blockInstances map[string]struct{}) []reports.Issue {
	if layout == nil {
		return nil
	}
	var issues []reports.Issue
	add := func(path, message string) {
		issues = append(issues, issue("schematic_layout."+path, message))
	}
	if layout.Flow != schematicir.FlowLeftToRight {
		add("flow", "schematic flow must be left_to_right")
	}
	if layout.Origin != schematicir.OriginCentered && layout.Origin != schematicir.OriginPageUpperLeft {
		add("origin", "schematic origin must be centered or page_upper_left")
	}
	if layout.Lanes.Power != schematicir.LanePositionTop {
		add("lanes.power", "positive power lane must be top")
	}
	if layout.Lanes.Ground != schematicir.LanePositionBottom {
		add("lanes.ground", "ground lane must be bottom")
	}
	if layout.Lanes.Signals != schematicir.LanePositionMiddle {
		add("lanes.signals", "signal lane must be middle")
	}
	if layout.Rules.MinGroupSpacingMM != nil && *layout.Rules.MinGroupSpacingMM < 0 {
		add("rules.min_group_spacing_mm", "group spacing must be non-negative")
	}
	if layout.Rules.MinComponentSpacingMM != nil && *layout.Rules.MinComponentSpacingMM < 0 {
		add("rules.min_component_spacing_mm", "component spacing must be non-negative")
	}
	groups := map[string]struct{}{}
	knownTargets := map[string]struct{}{}
	for index, group := range layout.Groups {
		path := fmt.Sprintf("groups[%d]", index)
		id := strings.TrimSpace(group.ID)
		if id == "" {
			add(path+".id", "layout group ID is required")
		} else if _, duplicate := groups[id]; duplicate {
			add(path+".id", "duplicate layout group "+id)
		} else {
			groups[id] = struct{}{}
		}
		for memberIndex, target := range group.Members {
			targetPath := fmt.Sprintf("%s.members[%d]", path, memberIndex)
			if message := validateSchematicLayoutTarget(target, blockInstances); message != "" {
				add(targetPath, message)
			}
			knownTargets[target] = struct{}{}
		}
	}
	placements := map[string]struct{}{}
	for index, placement := range layout.Placements {
		path := fmt.Sprintf("placements[%d]", index)
		if message := validateSchematicLayoutTarget(placement.Target, blockInstances); message != "" {
			add(path+".target", message)
		}
		if _, duplicate := placements[placement.Target]; duplicate {
			add(path+".target", "duplicate placement target "+placement.Target)
		}
		placements[placement.Target] = struct{}{}
		knownTargets[placement.Target] = struct{}{}
		if placement.Group != "" {
			if _, exists := groups[placement.Group]; !exists {
				add(path+".group", "placement references unknown group "+placement.Group)
			}
		}
	}
	for index, placement := range layout.Placements {
		path := fmt.Sprintf("placements[%d]", index)
		relations := []struct {
			field   string
			targets []string
		}{{"near", placement.Near}, {"above", placement.Above}, {"right_of", placement.RightOf}}
		for _, relation := range relations {
			seen := map[string]struct{}{}
			for targetIndex, target := range relation.targets {
				targetPath := fmt.Sprintf("%s.%s[%d]", path, relation.field, targetIndex)
				if target == placement.Target {
					add(targetPath, "placement relation cannot reference its own target")
				} else if _, exists := knownTargets[target]; !exists {
					add(targetPath, "placement relation references unknown target "+target)
				} else if _, duplicate := seen[target]; duplicate {
					add(targetPath, "placement relation contains duplicate target "+target)
				}
				seen[target] = struct{}{}
			}
		}
	}
	for _, relation := range []string{"above", "right_of"} {
		if cycle := schematicir.PlacementRelationCycle(layout.Placements, relation); len(cycle) != 0 {
			add("placements", relation+" relation contains a cycle: "+schematicir.FormatPlacementRelationCycle(cycle))
		}
	}
	return issues
}

func validateSchematicLayoutTarget(target string, blockInstances map[string]struct{}) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return "layout target is required"
	}
	for instanceID := range blockInstances {
		prefix := instanceID + schematicLayoutTargetDelimiter
		if strings.HasPrefix(target, prefix) && strings.TrimPrefix(target, prefix) != "" {
			return ""
		}
	}
	return "layout target must use known instance__role syntax: " + target
}
