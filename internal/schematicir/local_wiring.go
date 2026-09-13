package schematicir

import (
	"kicadai/internal/libraryresolver"
	"kicadai/internal/schematiclayout"
)

// A boundary connector is not by itself a reason to hide all local conductors.
// Explicit routing choices, global ports/buses and uncertain geometry retain
// their existing semantics. Every participating pin must be resolver-backed.
func functionalLocalWiringEligible(document Document, net Net, index *libraryresolver.LibraryIndex, components map[string]Component) bool {
	if net.UseLabel != nil || index == nil || len(net.Connect) < 2 ||
		stateDocumentHasPortNet(document, net.Name) || net.Role == NetRoleBus ||
		schematicNetHasUnsafeTransform(document, net, index) {
		return false
	}
	for _, endpoint := range net.Connect {
		id, number, ok := endpoint.Split()
		component, exists := components[id]
		if !ok || !exists {
			return false
		}
		geometry := schematicLayoutGeometry(component, index)
		if geometry.Source != schematiclayout.GeometrySourceResolverGraphics && geometry.Source != schematiclayout.GeometrySourceResolverPinEnvelope {
			return false
		}
		found := false
		for _, pin := range schematicLayoutPins(component, index) {
			found = found || pin.Number == number
		}
		if !found {
			return false
		}
	}
	return true
}
