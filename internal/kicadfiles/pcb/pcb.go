package pcb

import (
	"cmp"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

type PCBFile struct {
	Version              kicadfiles.KiCadFormatVersion
	Generator            string
	GeneratorVersion     string
	General              PCBGeneral
	Paper                kicadfiles.Paper
	Layers               []LayerDefinition
	Setup                PCBSetup
	Nets                 []Net
	Footprints           []Footprint
	Tracks               []Track
	Vias                 []Via
	Drawings             []Drawing
	Zones                []Zone
	Dimensions           []Dimension
	TitleBlock           kicadfiles.TitleBlock
	RequireClosedOutline bool
}

type PCBGeneral struct {
	Thickness       kicadfiles.IU
	LegacyTeardrops bool
}

type PCBSetup struct {
	Stackup                            PCBStackup
	SolderMaskMinWidth                 kicadfiles.IU
	PadToMaskClearance                 kicadfiles.IU
	AllowSoldermaskBridgesInFootprints bool
	TentingFront                       bool
	TentingBack                        bool
	CoveringFront                      bool
	CoveringBack                       bool
	PluggingFront                      bool
	PluggingBack                       bool
	Capping                            bool
	Filling                            bool
	PlotParams                         PCBPlotParams
}

type PCBStackup struct {
	Thickness kicadfiles.IU
}

type LayerDefinition struct {
	Number      int
	Name        kicadfiles.BoardLayer
	Kind        string
	DisplayName string
}

type PCBPlotParams struct {
	LayerSelection              string
	PlotOnAllLayersSelection    string
	DisableApertureMacros       bool
	UseGerberExtensions         bool
	UseGerberAttributes         bool
	UseGerberAdvancedAttributes bool
	CreateGerberJobFile         bool
	DashedLineDashRatio         int
	DashedLineGapRatio          int
	SVGPrecision                int
	PlotFrameRef                bool
	Mode                        int
	UseAuxOrigin                bool
	PDFFrontFPPropertyPopups    bool
	PDFBackFPPropertyPopups     bool
	PDFMetadata                 bool
	PDFSingleDocument           bool
	DXFPolygonMode              bool
	DXFImperialUnits            bool
	DXFUsePcbNewFont            bool
	PSNegative                  bool
	PSA4Output                  bool
	PlotBlackAndWhite           bool
	SketchPadsOnFab             bool
	PlotPadNumbers              bool
	HideDNPOnFab                bool
	SketchDNPOnFab              bool
	CrossoutDNPOnFab            bool
	SubtractMaskFromSilk        bool
	OutputFormat                int
	Mirror                      bool
	DrillShape                  int
	ScaleSelection              int
	OutputDirectory             string
}

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
	kicad10LayerUserBase = 37
)

type Net struct {
	Code int
	Name string
}

type NetRegistry struct {
	mu     sync.Mutex
	nets   []Net
	byName map[string]int
}

type Footprint struct {
	UUID      kicadfiles.UUID
	Path      string
	LibraryID string
	Reference string
	Value     string
	Position  kicadfiles.Point
	Rotation  kicadfiles.Angle
	Layer     kicadfiles.BoardLayer
	Texts     []FootprintText
	Pads      []Pad
	Graphics  []FootprintGraphic
}

type FootprintText struct {
	Kind     string
	Text     string
	Position kicadfiles.Point
	Rotation kicadfiles.Angle
	Layer    kicadfiles.BoardLayer
}

type Pad struct {
	Name            string
	NetCode         int
	Shape           string
	RoundRectRRatio float64
	Position        kicadfiles.Point
	Rotation        kicadfiles.Angle
	Size            kicadfiles.Point
	Drill           kicadfiles.IU
	Layers          []kicadfiles.BoardLayer
}

type Drawing struct {
	UUID   kicadfiles.UUID
	Layer  kicadfiles.BoardLayer
	Kind   string
	Line   *LineDrawing
	Circle *CircleDrawing
	Arc    *ArcDrawing
	Poly   *PolylineDrawing
}

type LineDrawing struct {
	Start kicadfiles.Point
	End   kicadfiles.Point
	Width kicadfiles.IU
}

type CircleDrawing struct {
	Center kicadfiles.Point
	End    kicadfiles.Point
	Width  kicadfiles.IU
}

type ArcDrawing struct {
	Start kicadfiles.Point
	Mid   kicadfiles.Point
	End   kicadfiles.Point
	Width kicadfiles.IU
}

type PolylineDrawing struct {
	Points []kicadfiles.Point
	Width  kicadfiles.IU
}

type FootprintGraphic Drawing

type Track struct {
	UUID    kicadfiles.UUID
	Start   kicadfiles.Point
	End     kicadfiles.Point
	Width   kicadfiles.IU
	Layer   kicadfiles.BoardLayer
	NetCode int
}

type Via struct {
	UUID     kicadfiles.UUID
	Position kicadfiles.Point
	Size     kicadfiles.IU
	Drill    kicadfiles.IU
	NetCode  int
	Layers   []kicadfiles.BoardLayer
}

type Zone struct {
	UUID     kicadfiles.UUID
	NetCode  int
	Layers   []kicadfiles.BoardLayer
	Polygons [][]kicadfiles.Point
	Priority int
}

type Dimension struct {
	UUID     kicadfiles.UUID
	Type     string
	Layer    kicadfiles.BoardLayer
	Points   []kicadfiles.Point
	Height   kicadfiles.IU
	Text     string
	Position kicadfiles.Point
	Rotation kicadfiles.Angle
}

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
		layers = append(layers, LayerDefinition{Number: kicad10LayerUserBase + i*2, Name: kicadfiles.BoardLayer("User." + strconv.Itoa(i)), Kind: "user"})
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

func Validate(board PCBFile) error {
	var errs kicadfiles.ValidationErrors
	if board.Version == "" {
		errs = append(errs, fieldError("version", "required"))
	} else if _, err := strconv.ParseInt(string(board.Version), 10, 64); err != nil {
		errs = append(errs, fieldError("version", "must be numeric"))
	}
	if strings.TrimSpace(board.Generator) == "" {
		errs = append(errs, fieldError("generator", "required"))
	}
	if strings.TrimSpace(board.GeneratorVersion) == "" {
		errs = append(errs, fieldError("generator_version", "required"))
	}
	if strings.TrimSpace(board.Paper.Name) == "" {
		errs = append(errs, fieldError("paper", "required"))
	}
	if board.General.Thickness <= 0 {
		errs = append(errs, fieldError("general.thickness", "must be positive"))
	}
	if board.Setup.Stackup.Thickness <= 0 {
		errs = append(errs, fieldError("setup.stackup.thickness", "must be positive"))
	}
	if board.Setup.SolderMaskMinWidth < 0 {
		errs = append(errs, fieldError("setup.solder_mask_min_width", "must be non-negative"))
	}
	if board.Setup.PadToMaskClearance < 0 {
		errs = append(errs, fieldError("setup.pad_to_mask_clearance", "must be non-negative"))
	}
	if len(board.TitleBlock.Comments) > 9 {
		errs = append(errs, fieldError("title_block.comments", "at most 9 comments allowed"))
	}
	if len(board.Layers) == 0 {
		errs = append(errs, fieldError("layers", "at least one layer required"))
	}
	layerNumbers := make(map[int]struct{}, len(board.Layers))
	layerNames := make(map[kicadfiles.BoardLayer]struct{}, len(board.Layers))
	for i, layer := range board.Layers {
		if _, ok := layerNumbers[layer.Number]; ok {
			errs = append(errs, fieldError(indexed("layers", i, "number"), "duplicate"))
		}
		layerNumbers[layer.Number] = struct{}{}
		if _, ok := layerNames[layer.Name]; ok {
			errs = append(errs, fieldError(indexed("layers", i, "name"), "duplicate"))
		}
		layerNames[layer.Name] = struct{}{}
		if !kicadfiles.IsValidBoardLayer(layer.Name) {
			errs = append(errs, fieldError(indexed("layers", i, "name"), "invalid"))
		}
		if strings.TrimSpace(layer.Kind) == "" {
			errs = append(errs, fieldError(indexed("layers", i, "kind"), "required"))
		}
	}
	netCodes := make(map[int]struct{}, len(board.Nets))
	netNames := make(map[string]struct{}, len(board.Nets))
	for i, net := range board.Nets {
		if net.Code < 0 {
			errs = append(errs, fieldError(indexed("nets", i, "code"), "must be non-negative"))
		}
		if net.Code == 0 && net.Name != "" {
			errs = append(errs, fieldError(indexed("nets", i, "name"), "must be empty for net 0"))
		}
		if net.Code > 0 && strings.TrimSpace(net.Name) == "" {
			errs = append(errs, fieldError(indexed("nets", i, "name"), "required"))
		}
		if _, ok := netCodes[net.Code]; ok {
			errs = append(errs, fieldError(indexed("nets", i, "code"), "duplicate"))
		}
		netCodes[net.Code] = struct{}{}
		if _, ok := netNames[net.Name]; ok {
			errs = append(errs, fieldError(indexed("nets", i, "name"), "duplicate"))
		}
		netNames[net.Name] = struct{}{}
	}
	validNetCodes := netCodeSet(board.Nets)
	for i, footprint := range board.Footprints {
		errs = append(errs, validateFootprint(i, footprint, validNetCodes)...)
	}
	for i, drawing := range board.Drawings {
		errs = append(errs, validateDrawing(i, drawing)...)
	}
	if board.RequireClosedOutline {
		errs = append(errs, validateClosedOutline(board.Drawings)...)
	}
	for i, track := range board.Tracks {
		errs = append(errs, validateTrack(i, track, validNetCodes)...)
	}
	for i, via := range board.Vias {
		errs = append(errs, validateVia(i, via, validNetCodes)...)
	}
	for i, zone := range board.Zones {
		errs = append(errs, validateZone(i, zone, validNetCodes)...)
	}
	for i, dimension := range board.Dimensions {
		errs = append(errs, validateDimension(i, dimension)...)
	}
	return errs.Err()
}

func Write(w io.Writer, board PCBFile) error {
	if err := validateNetZeroForNormalization(board.Nets); err != nil {
		return err
	}
	board.Nets = NormalizeNets(board.Nets)
	if err := Validate(board); err != nil {
		return err
	}
	node, err := render(board)
	if err != nil {
		return err
	}
	return sexpr.Render(w, node)
}

func validateNetZeroForNormalization(nets []Net) error {
	for i, net := range nets {
		if net.Code == 0 && net.Name != "" {
			return fieldError(indexed("nets", i, "name"), "must be empty for net 0")
		}
	}
	return nil
}

func render(board PCBFile) (sexpr.List, error) {
	version, err := strconv.ParseInt(string(board.Version), 10, 64)
	if err != nil {
		return nil, err
	}
	nodes := []sexpr.Node{
		sexpr.A("kicad_pcb"),
		sexpr.L(sexpr.A("version"), sexpr.I(version)),
		sexpr.L(sexpr.A("generator"), sexpr.S(strings.TrimSpace(board.Generator))),
		sexpr.L(sexpr.A("generator_version"), sexpr.S(strings.TrimSpace(board.GeneratorVersion))),
		renderGeneral(board.General),
		sexpr.L(sexpr.A("paper"), sexpr.S(strings.TrimSpace(board.Paper.Name))),
	}
	if title := renderTitleBlock(board.TitleBlock); len(title) > 1 {
		nodes = append(nodes, title)
	}
	nodes = append(nodes, renderLayers(board.Layers), renderSetup(board.Setup))
	for _, net := range sortedNets(board.Nets) {
		nodes = append(nodes, sexpr.L(sexpr.A("net"), sexpr.I(int64(net.Code)), sexpr.S(net.Name)))
	}
	netNames := netNameMap(board.Nets)
	for _, footprint := range sortedFootprints(board.Footprints) {
		nodes = append(nodes, renderFootprint(footprint, netNames))
	}
	for _, drawing := range board.Drawings {
		nodes = append(nodes, renderDrawing(drawing))
	}
	for _, track := range board.Tracks {
		nodes = append(nodes, renderTrack(track))
	}
	for _, via := range board.Vias {
		nodes = append(nodes, renderVia(via))
	}
	for _, zone := range board.Zones {
		nodes = append(nodes, renderZone(zone, netNames))
	}
	for _, dimension := range board.Dimensions {
		nodes = append(nodes, renderDimension(dimension))
	}
	return sexpr.L(nodes...), nil
}

func renderGeneral(general PCBGeneral) sexpr.List {
	return sexpr.L(
		sexpr.A("general"),
		sexpr.L(sexpr.A("thickness"), fixed(general.Thickness)),
		sexpr.L(sexpr.A("legacy_teardrops"), yesNo(general.LegacyTeardrops)),
	)
}

func renderLayers(layers []LayerDefinition) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("layers")}
	ordered := slices.Clone(layers)
	slices.SortFunc(ordered, func(a, b LayerDefinition) int { return cmp.Compare(a.Number, b.Number) })
	for _, layer := range ordered {
		layerNodes := []sexpr.Node{sexpr.I(int64(layer.Number)), sexpr.S(string(layer.Name)), sexpr.A(layer.Kind)}
		if strings.TrimSpace(layer.DisplayName) != "" {
			layerNodes = append(layerNodes, sexpr.S(layer.DisplayName))
		}
		nodes = append(nodes, sexpr.L(layerNodes...))
	}
	return sexpr.L(nodes...)
}

func renderSetup(setup PCBSetup) sexpr.List {
	return sexpr.L(
		sexpr.A("setup"),
		sexpr.L(sexpr.A("stackup"), sexpr.L(sexpr.A("thickness"), fixed(setup.Stackup.Thickness))),
		sexpr.L(sexpr.A("solder_mask_min_width"), fixed(setup.SolderMaskMinWidth)),
		sexpr.L(sexpr.A("pad_to_mask_clearance"), fixed(setup.PadToMaskClearance)),
		sexpr.L(sexpr.A("allow_soldermask_bridges_in_footprints"), yesNo(setup.AllowSoldermaskBridgesInFootprints)),
		renderSidePair("tenting", setup.TentingFront, setup.TentingBack),
		renderSidePair("covering", setup.CoveringFront, setup.CoveringBack),
		renderSidePair("plugging", setup.PluggingFront, setup.PluggingBack),
		sexpr.L(sexpr.A("capping"), yesNo(setup.Capping)),
		sexpr.L(sexpr.A("filling"), yesNo(setup.Filling)),
		renderPlotParams(setup.PlotParams),
	)
}

func renderSidePair(name string, front, back bool) sexpr.List {
	return sexpr.L(
		sexpr.A(name),
		sexpr.L(sexpr.A("front"), yesNo(front)),
		sexpr.L(sexpr.A("back"), yesNo(back)),
	)
}

func renderPlotParams(params PCBPlotParams) sexpr.List {
	params = normalizePlotParams(params)
	return sexpr.L(
		sexpr.A("pcbplotparams"),
		sexpr.L(sexpr.A("layerselection"), sexpr.A(params.LayerSelection)),
		sexpr.L(sexpr.A("plot_on_all_layers_selection"), sexpr.A(params.PlotOnAllLayersSelection)),
		sexpr.L(sexpr.A("disableapertmacros"), yesNo(params.DisableApertureMacros)),
		sexpr.L(sexpr.A("usegerberextensions"), yesNo(params.UseGerberExtensions)),
		sexpr.L(sexpr.A("usegerberattributes"), yesNo(params.UseGerberAttributes)),
		sexpr.L(sexpr.A("usegerberadvancedattributes"), yesNo(params.UseGerberAdvancedAttributes)),
		sexpr.L(sexpr.A("creategerberjobfile"), yesNo(params.CreateGerberJobFile)),
		sexpr.L(sexpr.A("dashed_line_dash_ratio"), sexpr.I(int64(params.DashedLineDashRatio))),
		sexpr.L(sexpr.A("dashed_line_gap_ratio"), sexpr.I(int64(params.DashedLineGapRatio))),
		sexpr.L(sexpr.A("svgprecision"), sexpr.I(int64(params.SVGPrecision))),
		sexpr.L(sexpr.A("plotframeref"), yesNo(params.PlotFrameRef)),
		sexpr.L(sexpr.A("mode"), sexpr.I(int64(params.Mode))),
		sexpr.L(sexpr.A("useauxorigin"), yesNo(params.UseAuxOrigin)),
		sexpr.L(sexpr.A("pdf_front_fp_property_popups"), yesNo(params.PDFFrontFPPropertyPopups)),
		sexpr.L(sexpr.A("pdf_back_fp_property_popups"), yesNo(params.PDFBackFPPropertyPopups)),
		sexpr.L(sexpr.A("pdf_metadata"), yesNo(params.PDFMetadata)),
		sexpr.L(sexpr.A("pdf_single_document"), yesNo(params.PDFSingleDocument)),
		sexpr.L(sexpr.A("dxfpolygonmode"), yesNo(params.DXFPolygonMode)),
		sexpr.L(sexpr.A("dxfimperialunits"), yesNo(params.DXFImperialUnits)),
		sexpr.L(sexpr.A("dxfusepcbnewfont"), yesNo(params.DXFUsePcbNewFont)),
		sexpr.L(sexpr.A("psnegative"), yesNo(params.PSNegative)),
		sexpr.L(sexpr.A("psa4output"), yesNo(params.PSA4Output)),
		sexpr.L(sexpr.A("plot_black_and_white"), yesNo(params.PlotBlackAndWhite)),
		sexpr.L(sexpr.A("sketchpadsonfab"), yesNo(params.SketchPadsOnFab)),
		sexpr.L(sexpr.A("plotpadnumbers"), yesNo(params.PlotPadNumbers)),
		sexpr.L(sexpr.A("hidednponfab"), yesNo(params.HideDNPOnFab)),
		sexpr.L(sexpr.A("sketchdnponfab"), yesNo(params.SketchDNPOnFab)),
		sexpr.L(sexpr.A("crossoutdnponfab"), yesNo(params.CrossoutDNPOnFab)),
		sexpr.L(sexpr.A("subtractmaskfromsilk"), yesNo(params.SubtractMaskFromSilk)),
		sexpr.L(sexpr.A("outputformat"), sexpr.I(int64(params.OutputFormat))),
		sexpr.L(sexpr.A("mirror"), yesNo(params.Mirror)),
		sexpr.L(sexpr.A("drillshape"), sexpr.I(int64(params.DrillShape))),
		sexpr.L(sexpr.A("scaleselection"), sexpr.I(int64(params.ScaleSelection))),
		sexpr.L(sexpr.A("outputdirectory"), sexpr.S(params.OutputDirectory)),
	)
}

func normalizePlotParams(params PCBPlotParams) PCBPlotParams {
	if params == (PCBPlotParams{}) {
		return DefaultPlotParams()
	}
	if strings.TrimSpace(params.LayerSelection) == "" {
		params.LayerSelection = "0x00000000_00000000_00000000_00000000"
	}
	if strings.TrimSpace(params.PlotOnAllLayersSelection) == "" {
		params.PlotOnAllLayersSelection = "0x00000000_00000000_00000000_00000000"
	}
	return params
}

func renderTitleBlock(title kicadfiles.TitleBlock) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("title_block")}
	if title.Title != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("title"), sexpr.S(title.Title)))
	}
	if title.Date != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("date"), sexpr.S(title.Date)))
	}
	if title.Revision != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("rev"), sexpr.S(title.Revision)))
	}
	if title.Company != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("company"), sexpr.S(title.Company)))
	}
	for i, comment := range title.Comments {
		nodes = append(nodes, sexpr.L(sexpr.A("comment"), sexpr.I(int64(i+1)), sexpr.S(comment)))
	}
	return sexpr.L(nodes...)
}

func renderFootprint(footprint Footprint, netNames map[int]string) sexpr.List {
	nodes := []sexpr.Node{
		sexpr.A("footprint"),
		sexpr.S(footprint.LibraryID),
		sexpr.L(sexpr.A("layer"), sexpr.S(string(footprint.Layer))),
		renderAt(footprint.Position, footprint.Rotation),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(footprint.UUID))),
		sexpr.L(sexpr.A("path"), sexpr.S(footprint.Path)),
	}
	for _, text := range footprint.Texts {
		nodes = append(nodes, renderFootprintText(text))
	}
	for _, pad := range footprint.Pads {
		nodes = append(nodes, renderPad(pad, netNames[pad.NetCode]))
	}
	for _, graphic := range footprint.Graphics {
		nodes = append(nodes, renderFootprintGraphic(graphic))
	}
	return sexpr.L(nodes...)
}

func renderFootprintText(text FootprintText) sexpr.List {
	return sexpr.L(
		sexpr.A("fp_text"),
		sexpr.A(text.Kind),
		sexpr.S(text.Text),
		renderAt(text.Position, text.Rotation),
		sexpr.L(sexpr.A("layer"), sexpr.S(string(text.Layer))),
	)
}

func renderPad(pad Pad, netName string) sexpr.List {
	nodes := []sexpr.Node{
		sexpr.A("pad"),
		sexpr.S(pad.Name),
		sexpr.A(padType(pad)),
		sexpr.A(pad.Shape),
		renderAt(pad.Position, pad.Rotation),
		sexpr.L(sexpr.A("size"), fixed(pad.Size.X), fixed(pad.Size.Y)),
		renderLayerList("layers", pad.Layers),
		sexpr.L(sexpr.A("net"), sexpr.I(int64(pad.NetCode)), sexpr.S(netName)),
	}
	if pad.Drill > 0 {
		nodes = append(nodes, sexpr.L(sexpr.A("drill"), fixed(pad.Drill)))
	}
	if pad.Shape == "roundrect" {
		nodes = append(nodes, sexpr.L(sexpr.A("roundrect_rratio"), sexpr.F(roundRectRRatio(pad))))
	}
	return sexpr.L(nodes...)
}

func renderDrawing(drawing Drawing) sexpr.List {
	return renderGraphic("gr", Drawing(drawing))
}

func renderFootprintGraphic(graphic FootprintGraphic) sexpr.List {
	return renderGraphic("fp", Drawing(graphic))
}

func renderGraphic(prefix string, drawing Drawing) sexpr.List {
	nodes := []sexpr.Node{sexpr.A(prefix + "_" + drawingKind(drawing))}
	switch {
	case drawing.Line != nil:
		nodes = append(nodes,
			sexpr.L(sexpr.A("start"), fixed(drawing.Line.Start.X), fixed(drawing.Line.Start.Y)),
			sexpr.L(sexpr.A("end"), fixed(drawing.Line.End.X), fixed(drawing.Line.End.Y)),
			renderStroke(drawing.Line.Width),
		)
	case drawing.Circle != nil:
		nodes = append(nodes,
			sexpr.L(sexpr.A("center"), fixed(drawing.Circle.Center.X), fixed(drawing.Circle.Center.Y)),
			sexpr.L(sexpr.A("end"), fixed(drawing.Circle.End.X), fixed(drawing.Circle.End.Y)),
			renderStroke(drawing.Circle.Width),
		)
	case drawing.Arc != nil:
		nodes = append(nodes,
			sexpr.L(sexpr.A("start"), fixed(drawing.Arc.Start.X), fixed(drawing.Arc.Start.Y)),
			sexpr.L(sexpr.A("mid"), fixed(drawing.Arc.Mid.X), fixed(drawing.Arc.Mid.Y)),
			sexpr.L(sexpr.A("end"), fixed(drawing.Arc.End.X), fixed(drawing.Arc.End.Y)),
			renderStroke(drawing.Arc.Width),
		)
	case drawing.Poly != nil:
		nodes = append(nodes, renderPoints(drawing.Poly.Points), renderStroke(drawing.Poly.Width), sexpr.L(sexpr.A("fill"), sexpr.A("none")))
	}
	nodes = append(nodes,
		sexpr.L(sexpr.A("layer"), sexpr.S(string(drawing.Layer))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(drawing.UUID))),
	)
	return sexpr.L(nodes...)
}

func renderStroke(width kicadfiles.IU) sexpr.List {
	return sexpr.L(sexpr.A("stroke"), sexpr.L(sexpr.A("width"), fixed(width)), sexpr.L(sexpr.A("type"), sexpr.A("solid")))
}

func renderTrack(track Track) sexpr.List {
	return sexpr.L(
		sexpr.A("segment"),
		sexpr.L(sexpr.A("start"), fixed(track.Start.X), fixed(track.Start.Y)),
		sexpr.L(sexpr.A("end"), fixed(track.End.X), fixed(track.End.Y)),
		sexpr.L(sexpr.A("width"), fixed(track.Width)),
		sexpr.L(sexpr.A("layer"), sexpr.S(string(track.Layer))),
		sexpr.L(sexpr.A("net"), sexpr.I(int64(track.NetCode))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(track.UUID))),
	)
}

func renderVia(via Via) sexpr.List {
	return sexpr.L(
		sexpr.A("via"),
		sexpr.L(sexpr.A("at"), fixed(via.Position.X), fixed(via.Position.Y)),
		sexpr.L(sexpr.A("size"), fixed(via.Size)),
		sexpr.L(sexpr.A("drill"), fixed(via.Drill)),
		renderLayerList("layers", via.Layers),
		sexpr.L(sexpr.A("net"), sexpr.I(int64(via.NetCode))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(via.UUID))),
	)
}

func renderZone(zone Zone, netNames map[int]string) sexpr.List {
	nodes := []sexpr.Node{
		sexpr.A("zone"),
		sexpr.L(sexpr.A("net"), sexpr.I(int64(zone.NetCode))),
		sexpr.L(sexpr.A("net_name"), sexpr.S(netNames[zone.NetCode])),
		renderLayerList("layers", zone.Layers),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(zone.UUID))),
		sexpr.L(sexpr.A("priority"), sexpr.I(int64(zone.Priority))),
	}
	for _, polygon := range zone.Polygons {
		nodes = append(nodes, sexpr.L(sexpr.A("polygon"), renderPoints(polygon)))
	}
	return sexpr.L(nodes...)
}

func renderDimension(dimension Dimension) sexpr.List {
	return sexpr.L(
		sexpr.A("dimension"),
		sexpr.L(sexpr.A("type"), sexpr.A(dimension.Type)),
		sexpr.L(sexpr.A("layer"), sexpr.S(string(dimension.Layer))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(dimension.UUID))),
		renderPoints(dimension.Points),
		sexpr.L(sexpr.A("height"), fixed(dimension.Height)),
		sexpr.L(sexpr.A("gr_text"), sexpr.S(dimension.Text), renderAt(dimension.Position, dimension.Rotation)),
	)
}

func renderPoints(points []kicadfiles.Point) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("pts")}
	for _, point := range points {
		nodes = append(nodes, sexpr.L(sexpr.A("xy"), fixed(point.X), fixed(point.Y)))
	}
	return sexpr.L(nodes...)
}

func renderAt(point kicadfiles.Point, rotation kicadfiles.Angle) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("at"), fixed(point.X), fixed(point.Y)}
	if rotation != 0 {
		nodes = append(nodes, sexpr.F(float64(rotation)))
	}
	return sexpr.L(nodes...)
}

func renderLayerList(name string, layers []kicadfiles.BoardLayer) sexpr.List {
	nodes := []sexpr.Node{sexpr.A(name)}
	for _, layer := range layers {
		nodes = append(nodes, sexpr.S(string(layer)))
	}
	return sexpr.L(nodes...)
}

func fixed(value kicadfiles.IU) sexpr.Fixed {
	return sexpr.X(kicadfiles.ToMMString(value))
}

func yesNo(value bool) sexpr.Atom {
	if value {
		return sexpr.A("yes")
	}
	return sexpr.A("no")
}

func sortedNets(nets []Net) []Net {
	ordered := slices.Clone(nets)
	if !hasNetZero(ordered) {
		ordered = append(ordered, Net{Code: 0, Name: ""})
	}
	slices.SortFunc(ordered, func(a, b Net) int { return cmp.Compare(a.Code, b.Code) })
	return ordered
}

func sortedFootprints(footprints []Footprint) []Footprint {
	ordered := slices.Clone(footprints)
	slices.SortFunc(ordered, func(a, b Footprint) int {
		if byReference := cmp.Compare(a.Reference, b.Reference); byReference != 0 {
			return byReference
		}
		return cmp.Compare(string(a.UUID), string(b.UUID))
	})
	return ordered
}

func netCodeSet(nets []Net) map[int]struct{} {
	codes := map[int]struct{}{0: struct{}{}}
	for _, net := range nets {
		codes[net.Code] = struct{}{}
	}
	return codes
}

func netNameMap(nets []Net) map[int]string {
	names := map[int]string{0: ""}
	for _, net := range nets {
		names[net.Code] = net.Name
	}
	return names
}

func hasNetZero(nets []Net) bool {
	for _, net := range nets {
		if net.Code == 0 {
			return true
		}
	}
	return false
}

func validateFootprint(index int, footprint Footprint, netCodes map[int]struct{}) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := func(field string) string { return indexed("footprints", index, field) }
	if !footprint.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if strings.TrimSpace(footprint.Path) == "" {
		errs = append(errs, fieldError(prefix("path"), "required"))
	}
	if strings.TrimSpace(footprint.LibraryID) == "" {
		errs = append(errs, fieldError(prefix("library_id"), "required"))
	}
	if strings.TrimSpace(footprint.Reference) == "" {
		errs = append(errs, fieldError(prefix("reference"), "required"))
	}
	if !kicadfiles.IsValidBoardLayer(footprint.Layer) {
		errs = append(errs, fieldError(prefix("layer"), "invalid"))
	}
	textKinds := map[string]string{}
	for textIndex, text := range footprint.Texts {
		errs = append(errs, validateFootprintText(prefix("texts"), textIndex, text)...)
		textKinds[text.Kind] = text.Text
	}
	referenceText, hasReferenceText := textKinds["reference"]
	if !hasReferenceText || referenceText != footprint.Reference {
		errs = append(errs, fieldError(prefix("texts.reference"), "must match footprint reference"))
	}
	valueText, hasValueText := textKinds["value"]
	if !hasValueText || valueText != footprint.Value {
		errs = append(errs, fieldError(prefix("texts.value"), "must match footprint value"))
	}
	padNames := make(map[string]struct{}, len(footprint.Pads))
	for padIndex, pad := range footprint.Pads {
		errs = append(errs, validatePad(prefix("pads"), padIndex, pad, netCodes)...)
		if _, ok := padNames[pad.Name]; ok {
			errs = append(errs, fieldError(indexed(prefix("pads"), padIndex, "name"), "duplicate"))
		}
		padNames[pad.Name] = struct{}{}
	}
	for graphicIndex, graphic := range footprint.Graphics {
		errs = append(errs, validateGraphic(indexedValue(prefix("graphics"), graphicIndex), Drawing(graphic))...)
	}
	return errs
}

func validateFootprintText(collection string, index int, text FootprintText) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if text.Kind != "reference" && text.Kind != "value" && text.Kind != "user" {
		errs = append(errs, fieldError(indexed(collection, index, "kind"), "invalid"))
	}
	if strings.TrimSpace(text.Text) == "" {
		errs = append(errs, fieldError(indexed(collection, index, "text"), "required"))
	}
	if !kicadfiles.IsValidBoardLayer(text.Layer) {
		errs = append(errs, fieldError(indexed(collection, index, "layer"), "invalid"))
	}
	return errs
}

func validatePad(collection string, index int, pad Pad, netCodes map[int]struct{}) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if strings.TrimSpace(pad.Name) == "" {
		errs = append(errs, fieldError(indexed(collection, index, "name"), "required"))
	}
	if strings.TrimSpace(pad.Shape) == "" {
		errs = append(errs, fieldError(indexed(collection, index, "shape"), "required"))
	}
	if pad.Shape == "roundrect" && (pad.RoundRectRRatio < 0 || pad.RoundRectRRatio > 1) {
		errs = append(errs, fieldError(indexed(collection, index, "roundrect_rratio"), "must be between 0 and 1"))
	}
	if pad.Size.X <= 0 || pad.Size.Y <= 0 {
		errs = append(errs, fieldError(indexed(collection, index, "size"), "must be positive"))
	}
	if pad.Drill < 0 {
		errs = append(errs, fieldError(indexed(collection, index, "drill"), "must be non-negative"))
	}
	if len(pad.Layers) == 0 {
		errs = append(errs, fieldError(indexed(collection, index, "layers"), "required"))
	}
	for layerIndex, layer := range pad.Layers {
		if !kicadfiles.IsValidBoardLayer(layer) {
			errs = append(errs, fieldError(indexedValue(indexed(collection, index, "layers"), layerIndex), "invalid"))
		}
	}
	if pad.Drill > 0 && !validDrilledPadLayers(pad.Layers) {
		errs = append(errs, fieldError(indexed(collection, index, "layers"), "drilled pads require through copper and mask layers"))
	}
	if _, ok := netCodes[pad.NetCode]; !ok {
		errs = append(errs, fieldError(indexed(collection, index, "net_code"), "unknown"))
	}
	return errs
}

func validateDrawing(index int, drawing Drawing) kicadfiles.ValidationErrors {
	return validateGraphic(indexedValue("drawings", index), drawing)
}

func validateGraphic(prefix string, drawing Drawing) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if !drawing.UUID.Valid() {
		errs = append(errs, fieldError(prefix+".uuid", "valid UUID required"))
	}
	if !kicadfiles.IsValidBoardLayer(drawing.Layer) {
		errs = append(errs, fieldError(prefix+".layer", "invalid"))
	}
	shapes := countShapes(drawing)
	if shapes != 1 {
		errs = append(errs, fieldError(prefix, "exactly one shape required"))
		return errs
	}
	switch {
	case drawing.Line != nil:
		if drawing.Line.Width <= 0 {
			errs = append(errs, fieldError(prefix+".line.width", "must be positive"))
		}
		if drawing.Line.Start == drawing.Line.End {
			errs = append(errs, fieldError(prefix+".line", "must have non-zero length"))
		}
	case drawing.Circle != nil:
		if drawing.Circle.Width <= 0 {
			errs = append(errs, fieldError(prefix+".circle.width", "must be positive"))
		}
		if drawing.Circle.Center == drawing.Circle.End {
			errs = append(errs, fieldError(prefix+".circle", "must have non-zero radius"))
		}
	case drawing.Arc != nil:
		if drawing.Arc.Width <= 0 {
			errs = append(errs, fieldError(prefix+".arc.width", "must be positive"))
		}
		if drawing.Arc.Start == drawing.Arc.Mid || drawing.Arc.Mid == drawing.Arc.End || drawing.Arc.Start == drawing.Arc.End {
			errs = append(errs, fieldError(prefix+".arc", "points must be distinct"))
		}
	case drawing.Poly != nil:
		if drawing.Poly.Width <= 0 {
			errs = append(errs, fieldError(prefix+".poly.width", "must be positive"))
		}
		if countDistinctPoints(drawing.Poly.Points) < 2 {
			errs = append(errs, fieldError(prefix+".poly.points", "at least two distinct points required"))
		}
	}
	return errs
}

func validateClosedOutline(drawings []Drawing) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	degrees := map[kicadfiles.Point]int{}
	for _, drawing := range drawings {
		if drawing.Layer != kicadfiles.LayerEdge || drawing.Line == nil {
			continue
		}
		degrees[drawing.Line.Start]++
		degrees[drawing.Line.End]++
	}
	if len(degrees) == 0 {
		return append(errs, fieldError("drawings.edge_cuts", "closed outline required"))
	}
	for point, degree := range degrees {
		if degree != 2 {
			errs = append(errs, fieldError("drawings.edge_cuts", "outline endpoint "+kicadfiles.ToMMString(point.X)+","+kicadfiles.ToMMString(point.Y)+" is not closed"))
		}
	}
	return errs
}

func validateTrack(index int, track Track, netCodes map[int]struct{}) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := func(field string) string { return indexed("tracks", index, field) }
	if !track.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if track.Start == track.End {
		errs = append(errs, fieldError(prefix("end"), "must differ from start"))
	}
	if track.Width <= 0 {
		errs = append(errs, fieldError(prefix("width"), "must be positive"))
	}
	if !isCopperLayer(track.Layer) {
		errs = append(errs, fieldError(prefix("layer"), "must be copper"))
	}
	if _, ok := netCodes[track.NetCode]; !ok {
		errs = append(errs, fieldError(prefix("net_code"), "unknown"))
	}
	return errs
}

func validateVia(index int, via Via, netCodes map[int]struct{}) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := func(field string) string { return indexed("vias", index, field) }
	if !via.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if via.Size <= 0 {
		errs = append(errs, fieldError(prefix("size"), "must be positive"))
	}
	if via.Drill <= 0 {
		errs = append(errs, fieldError(prefix("drill"), "must be positive"))
	}
	if via.Drill >= via.Size {
		errs = append(errs, fieldError(prefix("drill"), "must be less than size"))
	}
	if _, ok := netCodes[via.NetCode]; !ok {
		errs = append(errs, fieldError(prefix("net_code"), "unknown"))
	}
	if countDistinctCopperLayers(via.Layers) < 2 {
		errs = append(errs, fieldError(prefix("layers"), "at least two copper layers required"))
	}
	return errs
}

func validateZone(index int, zone Zone, netCodes map[int]struct{}) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := func(field string) string { return indexed("zones", index, field) }
	if !zone.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if _, ok := netCodes[zone.NetCode]; !ok {
		errs = append(errs, fieldError(prefix("net_code"), "unknown"))
	}
	if len(zone.Layers) == 0 {
		errs = append(errs, fieldError(prefix("layers"), "required"))
	}
	for layerIndex, layer := range zone.Layers {
		if !kicadfiles.IsValidBoardLayer(layer) {
			errs = append(errs, fieldError(indexedValue(prefix("layers"), layerIndex), "invalid"))
		}
	}
	if len(zone.Polygons) == 0 {
		errs = append(errs, fieldError(prefix("polygons"), "required"))
	}
	for polygonIndex, polygon := range zone.Polygons {
		if countDistinctPoints(polygon) < 3 {
			errs = append(errs, fieldError(indexed(prefix("polygons"), polygonIndex, "points"), "at least three distinct points required"))
		}
	}
	return errs
}

func validateDimension(index int, dimension Dimension) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := func(field string) string { return indexed("dimensions", index, field) }
	if !dimension.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if strings.TrimSpace(dimension.Type) == "" {
		errs = append(errs, fieldError(prefix("type"), "required"))
	}
	if !kicadfiles.IsValidBoardLayer(dimension.Layer) {
		errs = append(errs, fieldError(prefix("layer"), "invalid"))
	}
	if countDistinctPoints(dimension.Points) < 2 {
		errs = append(errs, fieldError(prefix("points"), "at least two distinct points required"))
	}
	if dimension.Height <= 0 {
		errs = append(errs, fieldError(prefix("height"), "must be positive"))
	}
	if strings.TrimSpace(dimension.Text) == "" {
		errs = append(errs, fieldError(prefix("text"), "required"))
	}
	return errs
}

func countShapes(drawing Drawing) int {
	count := 0
	if drawing.Line != nil {
		count++
	}
	if drawing.Circle != nil {
		count++
	}
	if drawing.Arc != nil {
		count++
	}
	if drawing.Poly != nil {
		count++
	}
	return count
}

func drawingKind(drawing Drawing) string {
	switch {
	case drawing.Line != nil:
		return "line"
	case drawing.Circle != nil:
		return "circle"
	case drawing.Arc != nil:
		return "arc"
	case drawing.Poly != nil:
		return "poly"
	default:
		return drawing.Kind
	}
}

func countDistinctPoints(points []kicadfiles.Point) int {
	seen := make(map[kicadfiles.Point]struct{}, len(points))
	for _, point := range points {
		seen[point] = struct{}{}
	}
	return len(seen)
}

func countDistinctCopperLayers(layers []kicadfiles.BoardLayer) int {
	seen := map[kicadfiles.BoardLayer]struct{}{}
	for _, layer := range layers {
		if isCopperLayer(layer) {
			seen[layer] = struct{}{}
		}
	}
	return len(seen)
}

func isCopperLayer(layer kicadfiles.BoardLayer) bool {
	name := string(layer)
	return layer == kicadfiles.LayerFCu || layer == kicadfiles.LayerBCu || strings.HasPrefix(name, "In") && strings.HasSuffix(name, ".Cu") && kicadfiles.IsValidBoardLayer(layer)
}

func padType(pad Pad) string {
	if pad.Drill > 0 {
		return "thru_hole"
	}
	return "smd"
}

func roundRectRRatio(pad Pad) float64 {
	if pad.RoundRectRRatio == 0 {
		return 0.25
	}
	return pad.RoundRectRRatio
}

func hasPadLayerSet(layers []kicadfiles.BoardLayer, required ...kicadfiles.BoardLayer) bool {
	seen := make(map[kicadfiles.BoardLayer]struct{}, len(layers))
	for _, layer := range layers {
		seen[layer] = struct{}{}
	}
	for _, layer := range required {
		if _, ok := seen[layer]; !ok {
			return false
		}
	}
	return true
}

func validDrilledPadLayers(layers []kicadfiles.BoardLayer) bool {
	if hasPadLayerSet(layers, kicadfiles.LayerAllCu, kicadfiles.LayerAllMask) {
		return true
	}
	return countDistinctCopperLayers(layers) >= 2 && hasAnyMaskLayer(layers)
}

func hasAnyMaskLayer(layers []kicadfiles.BoardLayer) bool {
	for _, layer := range layers {
		if layer == kicadfiles.LayerAllMask || layer == kicadfiles.LayerFMask || layer == kicadfiles.LayerBMask {
			return true
		}
	}
	return false
}

func fieldError(field, message string) kicadfiles.ValidationError {
	return kicadfiles.ValidationError{Section: "pcb", Field: field, Message: message}
}

func indexed(collection string, index int, field string) string {
	return collection + "[" + strconv.Itoa(index) + "]." + field
}

func indexedValue(collection string, index int) string {
	return collection + "[" + strconv.Itoa(index) + "]"
}
