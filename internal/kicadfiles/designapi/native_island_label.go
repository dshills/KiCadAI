package designapi

import (
	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
)

// labelNativeConnectedIsland preserves an electrical label when a requested
// stub meets an already-wired pin. An existing wire alone does not prove that
// its island has a label connecting it to the other label-only endpoints.
// The local-block profile opts in; previous drawing profiles are unchanged.
func (builder *Builder) labelNativeConnectedIsland(netName string, anchor kicadfiles.Point) {
	probe := schematic.Label{Text: netName, Position: anchor}
	island := builder.nativeLabelIsland(probe)
	if len(island) == 0 {
		return // Never attach a label to an unproven/foreign conductor.
	}
	for _, label := range builder.design.Schematic.Labels {
		if builder.canonicalNet(label.Text) != builder.canonicalNet(netName) {
			continue
		}
		for _, wire := range island {
			for i := 1; i < len(wire.Points); i++ {
				if pointOnSchematicSegment(label.Position, wire.Points[i-1], wire.Points[i]) {
					return
				}
			}
		}
	}
	// Finalization moves this seed along this exact island to a clear non-pin
	// glyph position, or fails explicitly. It does not add or alter any wire.
	_ = builder.AddLabelWithOptions(netName, anchor, schematic.LabelLocal, LabelOptions{})
}
