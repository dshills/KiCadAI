package pcb

import (
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
)

const (
	kicad10LayerFCu      = 0
	kicad10LayerFMask    = 1
	kicad10LayerBCu      = 2
	kicad10LayerBMask    = 3
	kicad10LayerFSilkS   = 5
	kicad10LayerBSilkS   = 7
	kicad10LayerFAdhes   = 9
	kicad10LayerBAdhes   = 11
	kicad10LayerFPaste   = 13
	kicad10LayerBPaste   = 15
	kicad10LayerDwgs     = 17
	kicad10LayerCmts     = 19
	kicad10LayerEco1     = 21
	kicad10LayerEco2     = 23
	kicad10LayerEdge     = 25
	kicad10LayerMargin   = 27
	kicad10LayerBCrtYd   = 29
	kicad10LayerFCrtYd   = 31
	kicad10LayerBFab     = 33
	kicad10LayerFFab     = 35
	kicad10LayerUserBase = 39

	defaultTextSizeMM         = 1.0
	defaultTextThicknessMM    = 0.15
	outlineClosureToleranceIU = kicadfiles.IU(100)

	zoneIslandRemovalAlways = 0
	zoneIslandRemovalNever  = 1
	zoneIslandRemovalArea   = 2
)

func DefaultTwoLayerStack() []LayerDefinition {
	layers := []LayerDefinition{
		{Number: kicad10LayerFCu, Name: kicadfiles.LayerFCu, Kind: "signal"},
		{Number: kicad10LayerBCu, Name: kicadfiles.LayerBCu, Kind: "signal"},
		{Number: kicad10LayerFAdhes, Name: kicadfiles.LayerFAdhes, Kind: "user", DisplayName: "F.Adhesive"},
		{Number: kicad10LayerBAdhes, Name: kicadfiles.LayerBAdhes, Kind: "user", DisplayName: "B.Adhesive"},
		{Number: kicad10LayerFPaste, Name: kicadfiles.LayerFPaste, Kind: "user"},
		{Number: kicad10LayerBPaste, Name: kicadfiles.LayerBPaste, Kind: "user"},
		{Number: kicad10LayerFSilkS, Name: kicadfiles.LayerFSilkS, Kind: "user", DisplayName: "F.Silkscreen"},
		{Number: kicad10LayerBSilkS, Name: kicadfiles.LayerBSilkS, Kind: "user", DisplayName: "B.Silkscreen"},
		{Number: kicad10LayerFMask, Name: kicadfiles.LayerFMask, Kind: "user"},
		{Number: kicad10LayerBMask, Name: kicadfiles.LayerBMask, Kind: "user"},
		{Number: kicad10LayerDwgs, Name: kicadfiles.LayerDwgs, Kind: "user", DisplayName: "User.Drawings"},
		{Number: kicad10LayerCmts, Name: kicadfiles.LayerCmts, Kind: "user", DisplayName: "User.Comments"},
		{Number: kicad10LayerEco1, Name: kicadfiles.LayerEco1, Kind: "user", DisplayName: "User.Eco1"},
		{Number: kicad10LayerEco2, Name: kicadfiles.LayerEco2, Kind: "user", DisplayName: "User.Eco2"},
		{Number: kicad10LayerEdge, Name: kicadfiles.LayerEdge, Kind: "user"},
		{Number: kicad10LayerMargin, Name: kicadfiles.LayerMargin, Kind: "user"},
		{Number: kicad10LayerFCrtYd, Name: kicadfiles.LayerFCrtYd, Kind: "user", DisplayName: "F.Courtyard"},
		{Number: kicad10LayerBCrtYd, Name: kicadfiles.LayerBCrtYd, Kind: "user", DisplayName: "B.Courtyard"},
		{Number: kicad10LayerFFab, Name: kicadfiles.LayerFFab, Kind: "user"},
		{Number: kicad10LayerBFab, Name: kicadfiles.LayerBFab, Kind: "user"},
	}
	for i := 1; i <= 45; i++ {
		layers = append(layers, LayerDefinition{Number: kicad10LayerUserBase + (i-1)*2, Name: kicadfiles.BoardLayer("User." + strconv.Itoa(i)), Kind: "user"})
	}
	return layers
}

func DefaultGeneral() PCBGeneral {
	return PCBGeneral{Thickness: kicadfiles.MM(1.6)}
}

func DefaultSetup() PCBSetup {
	return PCBSetup{
		Stackup:                            PCBStackup{Thickness: kicadfiles.MM(1.6)},
		AllowSoldermaskBridgesInFootprints: false,
		TentingFront:                       true,
		TentingBack:                        true,
		PlotParams:                         DefaultPlotParams(),
	}
}

func DefaultPlotParams() PCBPlotParams {
	return PCBPlotParams{
		LayerSelection:              "0x00000000_00000000_55555555_5755f5ff",
		PlotOnAllLayersSelection:    "0x00000000_00000000_00000000_00000000",
		UseGerberAttributes:         true,
		UseGerberAdvancedAttributes: true,
		CreateGerberJobFile:         true,
		DashedLineDashRatio:         12,
		DashedLineGapRatio:          3,
		SVGPrecision:                4,
		Mode:                        1,
		PDFFrontFPPropertyPopups:    true,
		PDFBackFPPropertyPopups:     true,
		PDFMetadata:                 true,
		DXFPolygonMode:              true,
		DXFImperialUnits:            true,
		DXFUsePcbNewFont:            true,
		PlotBlackAndWhite:           true,
		SketchDNPOnFab:              true,
		CrossoutDNPOnFab:            true,
		OutputFormat:                1,
		DrillShape:                  1,
		ScaleSelection:              1,
	}
}

func NewNetRegistry(names ...string) *NetRegistry {
	registry := &NetRegistry{
		nets:   []Net{{Code: 0, Name: ""}},
		byName: map[string]int{"": 0},
	}
	for _, name := range names {
		registry.EnsureNet(name)
	}
	return registry
}

func (registry *NetRegistry) EnsureNet(name string) Net {
	registry.mustBeUsable()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.ensureInitializedLocked()
	name = strings.TrimSpace(name)
	if code, ok := registry.byName[name]; ok {
		return Net{Code: code, Name: name}
	}
	code := len(registry.nets)
	net := Net{Code: code, Name: name}
	registry.nets = append(registry.nets, net)
	registry.byName[name] = code
	return net
}

func (registry *NetRegistry) NetCode(name string) (int, bool) {
	registry.mustBeUsable()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.ensureInitializedLocked()
	code, ok := registry.byName[strings.TrimSpace(name)]
	return code, ok
}

func (registry *NetRegistry) Nets() []Net {
	registry.mustBeUsable()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.ensureInitializedLocked()
	return slices.Clone(registry.nets)
}

func (registry *NetRegistry) mustBeUsable() {
	if registry == nil {
		panic("nil PCB net registry")
	}
}

func (registry *NetRegistry) ensureInitializedLocked() {
	if registry.byName != nil {
		return
	}
	registry.nets = []Net{{Code: 0, Name: ""}}
	registry.byName = map[string]int{"": 0}
}

func NormalizeNets(nets []Net) []Net {
	ordered := sortedNets(slices.Clone(nets))
	if len(ordered) > 0 && ordered[0].Code == 0 {
		return ordered
	}
	normalized := make([]Net, 0, len(ordered)+1)
	normalized = append(normalized, Net{Code: 0, Name: ""})
	return append(normalized, ordered...)
}
