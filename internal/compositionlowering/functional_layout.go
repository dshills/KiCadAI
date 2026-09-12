package compositionlowering

import (
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
	"kicadai/internal/schematiclayout"
)

// applyFunctionalLayout uses the exact selected fragment instances and the
// synthesis support-parent records. No component ID prefix is interpreted as
// ownership. The naming operation matches Lower; the source payload is required.
func applyFunctionalLayout(request *designworkflow.Request, candidate architecturesearch.CandidateResult, synthesis circuitgraph.SynthesisReport, profile string) error {
	if (profile != schematiclayout.FunctionalOwnershipV1 && profile != schematiclayout.FunctionalOwnershipV2) || request.ExplicitCircuit == nil {
		return fmt.Errorf("unsupported functional layout profile or absent explicit circuit")
	}
	document := request.ExplicitCircuit.Schematic
	if document.Layout.NativeProfile != schematiclayout.NativeAnnotationV2 {
		return fmt.Errorf("functional layout requires annotation-v2")
	}
	owners := map[string]schematicir.FunctionalOwner{}
	groups := map[string]schematicir.Group{}
	for _, selection := range candidate.Selections {
		realization, err := architecturesearch.DecodeFragmentRealization(selection.Payload)
		if err != nil {
			return err
		}
		group := "functional_" + safeID(selection.ObligationPath)
		if _, exists := groups[group]; exists {
			return fmt.Errorf("duplicate functional group %s", group)
		}
		groups[group] = schematicir.Group{ID: group, Label: selection.ObligationPath, Role: schematicir.GroupRoleProcessingStage, Rank: 2}
		for _, instance := range realization.Instances {
			id := safeID(safeID(selection.ObligationPath) + "__" + instance.ID)
			if _, exists := owners[id]; exists {
				return fmt.Errorf("duplicate functional owner for %s", id)
			}
			owners[id] = schematicir.FunctionalOwner{Component: id, Group: group, Source: "fragment:" + selection.ObligationPath}
		}
	}
	parents := map[string]string{}
	components := map[string]bool{}
	for _, component := range document.Circuit.Components {
		components[component.ID] = true
	}
	for _, selection := range synthesis.Selections {
		if selection.ParentID != "" {
			if !components[selection.IntentID] || !components[selection.ParentID] {
				return fmt.Errorf("unknown component in support parent record")
			}
			if _, primary := owners[selection.IntentID]; primary {
				return fmt.Errorf("conflicting fragment and support ownership for %s", selection.IntentID)
			}
			if _, exists := parents[selection.IntentID]; exists {
				return fmt.Errorf("duplicate support parent")
			}
			parents[selection.IntentID] = selection.ParentID
		}
	}
	var resolveOwner func(string, map[string]bool) (schematicir.FunctionalOwner, bool)
	resolveOwner = func(id string, seen map[string]bool) (schematicir.FunctionalOwner, bool) {
		if owner, ok := owners[id]; ok {
			return owner, true
		}
		if seen[id] || parents[id] == "" {
			return schematicir.FunctionalOwner{}, false
		}
		seen[id] = true
		parent, ok := resolveOwner(parents[id], seen)
		if !ok {
			return schematicir.FunctionalOwner{}, false
		}
		return schematicir.FunctionalOwner{Component: id, Group: parent.Group, Parent: parents[id], Source: "synthesis-support-parent"}, true
	}
	layout := schematicir.CloneLayout(document.Layout)
	layout.FunctionalProfile = profile
	layout.Groups, layout.FunctionalOwners = nil, nil
	for _, component := range document.Circuit.Components {
		owner, ok := resolveOwner(component.ID, map[string]bool{})
		if !ok {
			if _, support := parents[component.ID]; support {
				return fmt.Errorf("unresolvable explicit support parent for %s", component.ID)
			}
			switch component.Role {
			case schematicir.ComponentRoleConnector, schematicir.ComponentRoleInputConnector, schematicir.ComponentRoleOutputConnector, schematicir.ComponentRolePowerSymbol, schematicir.ComponentRoleGroundSymbol:
				owner = schematicir.FunctionalOwner{Component: component.ID, Group: "functional_boundaries", Source: "explicit-boundary-role"}
				groups[owner.Group] = schematicir.Group{ID: owner.Group, Label: "External interfaces and rail flags", Role: schematicir.GroupRoleConnectorStage, Rank: 0, Members: groups[owner.Group].Members}
			default:
				return fmt.Errorf("no explicit functional owner for %s", component.ID)
			}
		}
		group := groups[owner.Group]
		if component.Role == schematicir.ComponentRoleRegulator {
			group.Role, group.Rank = schematicir.GroupRolePowerStage, 1
		}
		group.Members = append(group.Members, component.ID)
		groups[owner.Group] = group
		layout.FunctionalOwners = append(layout.FunctionalOwners, owner)
	}
	for _, group := range groups {
		if len(group.Members) == 0 {
			return fmt.Errorf("empty functional group %s", group.ID)
		}
		slices.Sort(group.Members)
		layout.Groups = append(layout.Groups, group)
	}
	slices.SortFunc(layout.Groups, func(a, b schematicir.Group) int {
		if a.Rank != b.Rank {
			return a.Rank - b.Rank
		}
		return strings.Compare(a.ID, b.ID)
	})
	slices.SortFunc(layout.FunctionalOwners, func(a, b schematicir.FunctionalOwner) int { return strings.Compare(a.Component, b.Component) })
	for i := range layout.Groups {
		layout.Groups[i].Rank = i
	}
	// Existing orientations remain; old inferred relationships cannot impose
	// a cross-block layout. Membership and geometry are regenerated from intent.
	orientations := map[string]schematicir.Placement{}
	for _, p := range layout.Placements {
		orientations[p.Target] = p
	}
	layout.Placements = nil
	for _, owner := range layout.FunctionalOwners {
		p := orientations[owner.Component]
		layout.Placements = append(layout.Placements, schematicir.Placement{Target: owner.Component, Group: owner.Group, Orientation: p.Orientation, Mirror: p.Mirror})
	}
	document.Layout = layout
	if validation := schematicir.Validate(document); reports.HasBlockingIssue(validation) {
		return fmt.Errorf("functional layout validation: %v", validation)
	}
	request.ExplicitCircuit.Schematic = document
	return nil
}
