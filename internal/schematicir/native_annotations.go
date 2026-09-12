package schematicir

import (
	"fmt"
	"kicadai/internal/schematiclayout"
	"strings"
)

func nativeAnnotationGeometry(document Document) bool {
	return document.Layout.Rules.OrientEndpointLabels || document.Layout.NativeProfile == schematiclayout.NativeAnnotationV2
}

// Reading-guide strings are non-electrical annotations derived only from the
// explicit IR. Source IDs preserve functional provenance without guessing a
// connector function from its reference, or renaming an electrical net.
func nativeSchematicNotes(document Document) []string {
	if document.Layout.NativeProfile != schematiclayout.NativeAnnotationV2 {
		return nil
	}
	lines := []string{"READING GUIDE - explicit circuit intent", "Source IDs identify functional provenance.", "Connections below are not signal-direction arrows."}
	byID := indexComponentsByID(document.Circuit.Components)
	functional := document.Layout.FunctionalProfile == schematiclayout.FunctionalOwnershipV1 || document.Layout.FunctionalProfile == schematiclayout.FunctionalOwnershipV2
	if functional {
		lines = []string{"FUNCTIONAL BLOCKS - explicit fragment provenance", "Connections are not signal-direction arrows."}
		for _, group := range document.Layout.Groups {
			var refs []string
			for _, id := range group.Members {
				if c, ok := byID[id]; ok && !schematicComponentIsPowerFlag(c) {
					refs = append(refs, c.Ref)
				}
			}
			lines = append(lines, group.Label+": "+strings.Join(refs, ", "))
		}
	}
	for _, c := range document.Circuit.Components {
		if schematicComponentIsPowerFlag(c) {
			continue
		}
		if !functional {
			lines = append(lines, fmt.Sprintf("%s [%s]: %s", c.Ref, c.Role, c.ID))
		}
		if c.Role != ComponentRoleConnector && c.Role != ComponentRoleInputConnector && c.Role != ComponentRoleOutputConnector {
			continue
		}
		for _, n := range document.Circuit.Nets {
			for _, endpoint := range n.Connect {
				id, pin, ok := endpoint.Split()
				if ok && id == c.ID {
					lines = append(lines, fmt.Sprintf("  %s pin %s = %s [%s]", c.Ref, pin, n.Name, n.Role))
				}
			}
		}
	}
	lines = append(lines, "NET / REFERENCE AND SIGNAL CONNECTIONS")
	for _, n := range document.Circuit.Nets {
		var endpoints []string
		for _, endpoint := range n.Connect {
			id, pin, ok := endpoint.Split()
			c, found := byID[id]
			if !ok || !found || schematicComponentIsPowerFlag(c) {
				continue
			}
			value := c.Ref + "." + pin
			for _, p := range c.Pins {
				if p.Number == pin && p.Name != "" {
					value += "(" + p.Name + ")"
					break
				}
			}
			endpoints = append(endpoints, value)
		}
		lines = append(lines, fmt.Sprintf("%s [%s]: %s", n.Name, n.Role, strings.Join(endpoints, ", ")))
	}
	return lines
}
