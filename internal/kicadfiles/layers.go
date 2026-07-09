package kicadfiles

// DefaultFootprintPropertyPosition returns side-independent footprint-local positions for KiCad's generated properties.
func DefaultFootprintPropertyPosition(name string) Point {
	switch name {
	case "Reference":
		return Point{Y: MM(-1.5)}
	case "Value":
		return Point{Y: MM(1.5)}
	default:
		return Point{}
	}
}

// BoardLayerForPlacement returns the equivalent board layer for a footprint placement side.
func BoardLayerForPlacement(layer BoardLayer, placementLayer BoardLayer) BoardLayer {
	if placementLayer != LayerBCu {
		return layer
	}
	switch layer {
	case LayerFCu:
		return LayerBCu
	case LayerFMask:
		return LayerBMask
	case LayerFPaste:
		return LayerBPaste
	case LayerFAdhes:
		return LayerBAdhes
	case LayerFSilkS:
		return LayerBSilkS
	case LayerFFab:
		return LayerBFab
	case LayerFCrtYd:
		return LayerBCrtYd
	default:
		return layer
	}
}
