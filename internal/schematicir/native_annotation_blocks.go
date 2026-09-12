package schematicir

import (
	"slices"
	"strings"

	"kicadai/internal/schematiclayout"
)

// nativeSchematicBlocks is a local explanation, not a second netlist. Pin names
// come from the explicit circuit. No component-ID substring is interpreted as a
// voltage, ADC assignment, reset function or programming protocol.
func nativeSchematicBlocks(document Document) []schematiclayout.NativeAnnotationBlock {
	if document.Layout.FunctionalProfile != schematiclayout.FunctionalOwnershipV3 || document.Layout.NativeProfile != schematiclayout.NativeAnnotationV2 {
		return nil
	}
	components := indexComponentsByID(document.Circuit.Components)
	var blocks []schematiclayout.NativeAnnotationBlock
	for _, group := range document.Layout.Groups {
		label := group.Label
		if strings.TrimSpace(label) == "" {
			label = group.ID
		}
		if _, suffix, ok := strings.Cut(label, ":"); ok {
			label = suffix
		}
		block := schematiclayout.NativeAnnotationBlock{ID: group.ID, Lines: []string{strings.ToUpper(humanizeHierarchyName(label))}}
		members := append([]string(nil), group.Members...)
		slices.Sort(members)
		for _, id := range members {
			c := components[id]
			if !schematicComponentIsPowerFlag(c) {
				block.References = append(block.References, c.Ref)
			}
		}
		block.Lines = append(block.Lines, "Parts: "+strings.Join(block.References, ", "))
		for _, id := range members {
			c := components[id]
			switch c.Role {
			case ComponentRoleIC, ComponentRoleRegulator, ComponentRoleSensor, ComponentRoleConnector, ComponentRoleInputConnector, ComponentRoleOutputConnector:
			default:
				continue
			}
			for _, pin := range c.Pins {
				if peers := nativeBlockPinPeers(document, c, pin.Number); peers != "" {
					block.Lines = append(block.Lines, nativeBlockPinName(c, pin.Number)+": "+peers)
				}
			}
		}
		if len(block.References) != 0 {
			blocks = append(blocks, block)
		}
	}
	slices.SortFunc(blocks, func(a, b schematiclayout.NativeAnnotationBlock) int { return strings.Compare(a.ID, b.ID) })
	return blocks
}

func nativeBlockPinName(c Component, number string) string {
	for _, pin := range c.Pins {
		if pin.Number == number && pin.Name != "" && pin.Name != "PIN_"+number {
			return c.Ref + "." + pin.Name + "(" + number + ")"
		}
	}
	return c.Ref + "." + number
}

func nativeBlockPinPeers(document Document, component Component, number string) string {
	components := indexComponentsByID(document.Circuit.Components)
	for _, net := range document.Circuit.Nets {
		if !slices.Contains(net.Connect, EndpointRef(component.ID+"."+number)) || net.Role == NetRoleNoConnect {
			continue
		}
		if net.Role == NetRoleGround {
			return "Ground (net role)"
		}
		var peers, active []string
		for _, endpoint := range net.Connect {
			id, pin, ok := endpoint.Split()
			c := components[id]
			if !ok || (id == component.ID && pin == number) || schematicComponentIsPowerFlag(c) {
				continue
			}
			name := nativeBlockPinName(c, pin)
			peers = append(peers, name)
			switch c.Role {
			case ComponentRoleIC, ComponentRoleRegulator, ComponentRoleSensor:
				active = append(active, name)
			}
		}
		if len(active) != 0 {
			slices.Sort(active)
			return "active pins " + strings.Join(active, ", ")
		}
		slices.Sort(peers)
		return strings.Join(peers, ", ")
	}
	return ""
}
