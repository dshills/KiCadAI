package pcb

import (
	"cmp"
	"io"
	"math/big"
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
	TrackArcs            []TrackArc
	Vias                 []Via
	Drawings             []Drawing
	Zones                []Zone
	Dimensions           []Dimension
	Preserved            []PreservedNode
	TitleBlock           kicadfiles.TitleBlock
	EmbeddedFonts        *bool
	RequireClosedOutline bool
}

type PreservedNode struct {
	Family string
	Raw    string
}

type PCBGeneral struct {
	Thickness       kicadfiles.IU
	LegacyTeardrops bool
}

type PCBSetup struct {
	HasStackup                         bool
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
	kicad10LayerUserBase = 39

	defaultTextSizeMM         = 1.0
	defaultTextThicknessMM    = 0.15
	outlineClosureToleranceIU = kicadfiles.IU(100)

	zoneIslandRemovalAlways = 0
	zoneIslandRemovalNever  = 1
	zoneIslandRemovalArea   = 2
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
	UUID               kicadfiles.UUID
	Path               string
	LibraryID          string
	Reference          string
	Value              string
	Description        string
	Tags               string
	SheetName          string
	SheetFile          string
	Attributes         []string
	Position           kicadfiles.Point
	Rotation           kicadfiles.Angle
	Layer              kicadfiles.BoardLayer
	Locked             bool
	Properties         []FootprintProperty
	MetadataProperties []FootprintMetadataProperty
	Units              []FootprintUnit
	NetTiePadGroups    []string
	Texts              []FootprintText
	Pads               []Pad
	Graphics           []FootprintGraphic
	Models             []Model3D
	EmbeddedFonts      *bool
	// KiCad 10 writes this flag explicitly on saved footprints.
	DuplicatePadNumbersAreJumpers *bool
}

type FootprintText struct {
	UUID     kicadfiles.UUID
	Kind     string
	Text     string
	Position kicadfiles.Point
	Rotation kicadfiles.Angle
	Layer    kicadfiles.BoardLayer
}

type FootprintProperty struct {
	UUID     kicadfiles.UUID
	Name     string
	Value    string
	Position kicadfiles.Point
	Rotation kicadfiles.Angle
	Layer    kicadfiles.BoardLayer
	Hide     bool
	Unlocked bool
	Effects  TextEffects
}

type FootprintMetadataProperty struct {
	Name  string
	Value string
}

type FootprintUnit struct {
	Name string
	Pins []string
}

type TextEffects struct {
	FontSize          kicadfiles.Point
	FontThickness     kicadfiles.IU
	OmitFontThickness bool
	Justify           []string
}

type Model3D struct {
	Path   string
	Offset XYZ
	Scale  XYZ
	Rotate XYZ
}

type XYZ struct {
	X float64
	Y float64
	Z float64
}

type Pad struct {
	UUID               kicadfiles.UUID
	Name               string
	Type               string
	NetCode            int
	NetName            string
	Shape              string
	RoundRectRRatio    float64
	PinFunction        string
	PinType            string
	Position           kicadfiles.Point
	Rotation           kicadfiles.Angle
	Size               kicadfiles.Point
	Drill              kicadfiles.IU
	Layers             []kicadfiles.BoardLayer
	RemoveUnusedLayers *bool
	ThermalBridgeAngle *float64
	Teardrops          *TeardropSettings
}

type TeardropSettings struct {
	BestLengthRatio      float64
	MaxLength            kicadfiles.IU
	BestWidthRatio       float64
	MaxWidth             kicadfiles.IU
	CurvedEdges          bool
	FilterRatio          float64
	Enabled              bool
	AllowTwoSegments     bool
	PreferZoneConnection bool
}

type Drawing struct {
	UUID       kicadfiles.UUID
	Layer      kicadfiles.BoardLayer
	Kind       string
	StrokeType string
	Fill       string
	NetCode    int
	NetName    string
	Line       *LineDrawing
	Rect       *RectDrawing
	Circle     *CircleDrawing
	Arc        *ArcDrawing
	Poly       *PolylineDrawing
	Curve      *PolylineDrawing
	Text       *TextDrawing
}

type LineDrawing struct {
	Start kicadfiles.Point
	End   kicadfiles.Point
	Width kicadfiles.IU
}

type RectDrawing struct {
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

type TextDrawing struct {
	Text     string
	Position kicadfiles.Point
	Rotation kicadfiles.Angle
	Effects  TextEffects
}

type FootprintGraphic Drawing

type Track struct {
	UUID    kicadfiles.UUID
	Start   kicadfiles.Point
	End     kicadfiles.Point
	Width   kicadfiles.IU
	Layer   kicadfiles.BoardLayer
	NetCode int
	NetName string
}

type TrackArc struct {
	UUID    kicadfiles.UUID
	Start   kicadfiles.Point
	Mid     kicadfiles.Point
	End     kicadfiles.Point
	Width   kicadfiles.IU
	Layer   kicadfiles.BoardLayer
	NetCode int
	NetName string
}

type Via struct {
	UUID         kicadfiles.UUID
	Position     kicadfiles.Point
	Size         kicadfiles.IU
	Drill        kicadfiles.IU
	NetCode      int
	NetName      string
	Layers       []kicadfiles.BoardLayer
	TentingFront bool
	TentingBack  bool
}

type Zone struct {
	UUID           kicadfiles.UUID
	NetCode        int
	NetName        string
	Name           string
	Layers         []kicadfiles.BoardLayer
	Polygons       [][]kicadfiles.Point
	FilledPolygons []ZoneFilledPolygon
	HatchStyle     string
	HatchPitch     kicadfiles.IU
	Priority       int
	ConnectPads    bool
	// ConnectPadsMode takes precedence over ConnectPads when set.
	ConnectPadsMode      string
	Clearance            kicadfiles.IU
	MinThickness         kicadfiles.IU
	FilledAreasThickness bool
	Fill                 ZoneFillSettings
	Keepout              *ZoneKeepout
	Attributes           []ZoneAttribute
}

type ZoneKeepout struct {
	Tracks     string
	Vias       string
	Pads       string
	CopperPour string
	Footprints string
}

type ZoneFillSettings struct {
	Enabled            bool
	ThermalGap         kicadfiles.IU
	ThermalBridgeWidth kicadfiles.IU
	IslandRemovalMode  int
	IslandAreaMin      float64
}

type ZoneFilledPolygon struct {
	Layer  kicadfiles.BoardLayer
	Points []kicadfiles.Point
}

type ZoneAttribute struct {
	Name   string
	Values map[string]string
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
	Effects  TextEffects
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
	if board.Setup.HasStackup && board.Setup.Stackup.Thickness <= 0 {
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
	validNetNames := netNameMap(board.Nets)
	for i, footprint := range board.Footprints {
		errs = append(errs, validateFootprint(i, footprint, validNetCodes, validNetNames)...)
	}
	for i, drawing := range board.Drawings {
		errs = append(errs, validateDrawing(i, drawing, validNetCodes, validNetNames)...)
	}
	if board.RequireClosedOutline {
		errs = append(errs, validateClosedOutline(board.Drawings)...)
	}
	for i, track := range board.Tracks {
		errs = append(errs, validateTrack(i, track, validNetCodes, validNetNames)...)
	}
	for i, arc := range board.TrackArcs {
		errs = append(errs, validateTrackArc(i, arc, validNetCodes, validNetNames)...)
	}
	for i, via := range board.Vias {
		errs = append(errs, validateVia(i, via, validNetCodes, validNetNames)...)
	}
	for i, zone := range board.Zones {
		errs = append(errs, validateZone(i, zone, validNetCodes, validNetNames)...)
	}
	for i, dimension := range board.Dimensions {
		errs = append(errs, validateDimension(i, dimension)...)
	}
	for i, preserved := range board.Preserved {
		raw := strings.TrimSpace(preserved.Raw)
		if raw == "" {
			errs = append(errs, fieldError(indexed("preserved", i, "raw"), "required"))
		} else if !sexpr.ValidRaw(raw) {
			errs = append(errs, fieldError(indexed("preserved", i, "raw"), "invalid s-expression syntax"))
		} else if strings.TrimSpace(preserved.Family) != "" {
			family := strings.TrimSpace(preserved.Family)
			if family != preserved.Family {
				errs = append(errs, fieldError(indexed("preserved", i, "family"), "trimmed value required"))
			} else if !isPreservationOnlyObject(family) {
				errs = append(errs, fieldError(indexed("preserved", i, "family"), "unknown preservation-only object family"))
			} else if rawRootToken(raw) != family {
				errs = append(errs, fieldError(indexed("preserved", i, "family"), "must match preserved raw node"))
			}
		}
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

func rawRootToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '(' {
		return ""
	}
	rest := strings.TrimLeft(raw[1:], " \t\r\n")
	end := strings.IndexAny(rest, " \t\r\n()")
	if end == -1 {
		return ""
	}
	return rest[:end]
}

func isPreservationOnlyObject(token string) bool {
	switch token {
	case "embedded_fonts", "teardrops", "group", "image", "table", "target", "embedded_files", "component_classes":
		return true
	default:
		return false
	}
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
	netNames := netNameMap(board.Nets)
	layerNumbers := layerNumberMap(board.Layers)
	footprints := sortedFootprints(board.Footprints)
	routeNetCodes := routedNetSortCodes(footprints, netNames)
	for _, footprint := range footprints {
		nodes = append(nodes, renderFootprint(footprint, netNames))
	}
	for _, drawing := range sortedDrawings(board.Drawings, layerNumbers) {
		nodes = append(nodes, renderDrawing(drawing))
	}
	for _, item := range sortedRoutedItems(board.Tracks, board.TrackArcs, board.Vias, layerNumbers, netNames, routeNetCodes) {
		nodes = append(nodes, item.render(netNames))
	}
	for _, zone := range board.Zones {
		nodes = append(nodes, renderZone(zone, netNames))
	}
	for _, dimension := range board.Dimensions {
		nodes = append(nodes, renderDimension(dimension))
	}
	if board.EmbeddedFonts != nil {
		nodes = append(nodes, sexpr.L(sexpr.A("embedded_fonts"), yesNo(*board.EmbeddedFonts)))
	}
	for _, preserved := range board.Preserved {
		nodes = append(nodes, sexpr.R(preserved.Raw))
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
	for _, layer := range layers {
		layerNodes := []sexpr.Node{sexpr.I(int64(layer.Number)), sexpr.S(string(layer.Name)), sexpr.A(layer.Kind)}
		if strings.TrimSpace(layer.DisplayName) != "" {
			layerNodes = append(layerNodes, sexpr.S(layer.DisplayName))
		}
		nodes = append(nodes, sexpr.L(layerNodes...))
	}
	return sexpr.L(nodes...)
}

func renderSetup(setup PCBSetup) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("setup")}
	if setup.HasStackup {
		nodes = append(nodes, sexpr.L(sexpr.A("stackup"), sexpr.L(sexpr.A("thickness"), fixed(setup.Stackup.Thickness))))
	}
	nodes = append(nodes,
		sexpr.L(sexpr.A("pad_to_mask_clearance"), fixed(setup.PadToMaskClearance)),
	)
	if setup.SolderMaskMinWidth > 0 {
		nodes = append(nodes, sexpr.L(sexpr.A("solder_mask_min_width"), fixed(setup.SolderMaskMinWidth)))
	}
	nodes = append(nodes,
		sexpr.L(sexpr.A("allow_soldermask_bridges_in_footprints"), yesNo(setup.AllowSoldermaskBridgesInFootprints)),
		renderSidePair("tenting", setup.TentingFront, setup.TentingBack),
		renderSidePair("covering", setup.CoveringFront, setup.CoveringBack),
		renderSidePair("plugging", setup.PluggingFront, setup.PluggingBack),
		sexpr.L(sexpr.A("capping"), yesNo(setup.Capping)),
		sexpr.L(sexpr.A("filling"), yesNo(setup.Filling)),
		renderPlotParams(setup.PlotParams),
	)
	return sexpr.L(nodes...)
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
	}
	if footprint.Locked {
		nodes = append(nodes, sexpr.L(sexpr.A("locked"), sexpr.A("yes")))
	}
	nodes = append(nodes,
		sexpr.L(sexpr.A("layer"), sexpr.S(string(footprint.Layer))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(footprint.UUID))),
		renderAt(footprint.Position, footprint.Rotation),
	)
	if strings.TrimSpace(footprint.Description) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("descr"), sexpr.S(footprint.Description)))
	}
	if strings.TrimSpace(footprint.Tags) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("tags"), sexpr.S(footprint.Tags)))
	}
	for _, property := range footprint.Properties {
		nodes = append(nodes, renderFootprintProperty(property))
	}
	for _, property := range footprint.MetadataProperties {
		nodes = append(nodes, renderFootprintMetadataProperty(property))
	}
	if strings.TrimSpace(footprint.Path) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("path"), sexpr.S(footprint.Path)))
	}
	if strings.TrimSpace(footprint.SheetName) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("sheetname"), sexpr.S(footprint.SheetName)))
	}
	if strings.TrimSpace(footprint.SheetFile) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("sheetfile"), sexpr.S(footprint.SheetFile)))
	}
	if len(footprint.Units) > 0 {
		nodes = append(nodes, renderFootprintUnits(footprint.Units))
	}
	if len(footprint.Attributes) > 0 {
		attrNodes := []sexpr.Node{sexpr.A("attr")}
		for _, attr := range footprint.Attributes {
			attrNodes = append(attrNodes, sexpr.A(attr))
		}
		nodes = append(nodes, sexpr.L(attrNodes...))
	}
	if len(footprint.NetTiePadGroups) > 0 {
		groupNodes := []sexpr.Node{sexpr.A("net_tie_pad_groups")}
		for _, group := range footprint.NetTiePadGroups {
			groupNodes = append(groupNodes, sexpr.S(group))
		}
		nodes = append(nodes, sexpr.L(groupNodes...))
	}
	if footprint.DuplicatePadNumbersAreJumpers != nil {
		nodes = append(nodes, sexpr.L(sexpr.A("duplicate_pad_numbers_are_jumpers"), yesNo(*footprint.DuplicatePadNumbersAreJumpers)))
	}
	for _, text := range footprint.Texts {
		nodes = append(nodes, renderFootprintText(text))
	}
	for _, graphic := range footprint.Graphics {
		nodes = append(nodes, renderFootprintGraphic(graphic))
	}
	for _, pad := range footprint.Pads {
		nodes = append(nodes, renderPad(pad, netNames[pad.NetCode]))
	}
	if footprint.EmbeddedFonts != nil {
		nodes = append(nodes, sexpr.L(sexpr.A("embedded_fonts"), yesNo(*footprint.EmbeddedFonts)))
	}
	for _, model := range footprint.Models {
		nodes = append(nodes, renderModel3D(model))
	}
	return sexpr.L(nodes...)
}

func renderFootprintMetadataProperty(property FootprintMetadataProperty) sexpr.List {
	return sexpr.L(sexpr.A("property"), sexpr.S(property.Name), sexpr.S(property.Value))
}

func renderFootprintUnits(units []FootprintUnit) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("units")}
	for _, unit := range units {
		unitNodes := []sexpr.Node{
			sexpr.A("unit"),
			sexpr.L(sexpr.A("name"), sexpr.S(unit.Name)),
		}
		pinNodes := []sexpr.Node{sexpr.A("pins")}
		for _, pin := range unit.Pins {
			pinNodes = append(pinNodes, sexpr.S(pin))
		}
		unitNodes = append(unitNodes, sexpr.L(pinNodes...))
		nodes = append(nodes, sexpr.L(unitNodes...))
	}
	return sexpr.L(nodes...)
}

func renderFootprintProperty(property FootprintProperty) sexpr.List {
	nodes := []sexpr.Node{
		sexpr.A("property"),
		sexpr.S(property.Name),
		sexpr.S(property.Value),
		renderAtWithRotation(property.Position, property.Rotation),
	}
	if property.Unlocked {
		nodes = append(nodes, sexpr.L(sexpr.A("unlocked"), sexpr.A("yes")))
	}
	nodes = append(nodes, sexpr.L(sexpr.A("layer"), sexpr.S(string(property.Layer))))
	if property.Hide {
		nodes = append(nodes, sexpr.L(sexpr.A("hide"), sexpr.A("yes")))
	}
	nodes = append(nodes, sexpr.L(sexpr.A("uuid"), sexpr.S(string(property.UUID))), renderEffects(property.Effects))
	return sexpr.L(nodes...)
}

func renderEffects(effects TextEffects) sexpr.List {
	size := effects.FontSize
	if size.X == 0 {
		size.X = kicadfiles.MM(defaultTextSizeMM)
	}
	if size.Y == 0 {
		size.Y = kicadfiles.MM(defaultTextSizeMM)
	}
	fontNodes := []sexpr.Node{
		sexpr.A("font"),
		sexpr.L(sexpr.A("size"), fixed(size.X), fixed(size.Y)),
	}
	if !effects.OmitFontThickness {
		thickness := effects.FontThickness
		if thickness == 0 {
			thickness = kicadfiles.MM(defaultTextThicknessMM)
		}
		fontNodes = append(fontNodes, sexpr.L(sexpr.A("thickness"), fixed(thickness)))
	}
	font := sexpr.L(fontNodes...)
	nodes := []sexpr.Node{sexpr.A("effects"), font}
	if len(effects.Justify) > 0 {
		justify := []sexpr.Node{sexpr.A("justify")}
		for _, value := range effects.Justify {
			justify = append(justify, sexpr.A(value))
		}
		nodes = append(nodes, sexpr.L(justify...))
	}
	return sexpr.L(nodes...)
}

func renderModel3D(model Model3D) sexpr.List {
	return sexpr.L(
		sexpr.A("model"),
		sexpr.S(model.Path),
		sexpr.L(sexpr.A("offset"), renderXYZ(model.Offset)),
		sexpr.L(sexpr.A("scale"), renderXYZ(defaultScale(model.Scale))),
		sexpr.L(sexpr.A("rotate"), renderXYZ(model.Rotate)),
	)
}

func renderXYZ(value XYZ) sexpr.List {
	return sexpr.L(sexpr.A("xyz"), sexpr.F(value.X), sexpr.F(value.Y), sexpr.F(value.Z))
}

func defaultScale(value XYZ) XYZ {
	if value.X == 0 {
		value.X = 1
	}
	if value.Y == 0 {
		value.Y = 1
	}
	if value.Z == 0 {
		value.Z = 1
	}
	return value
}

func renderFootprintText(text FootprintText) sexpr.List {
	nodes := []sexpr.Node{
		sexpr.A("fp_text"),
		sexpr.A(text.Kind),
		sexpr.S(text.Text),
		renderAt(text.Position, text.Rotation),
		sexpr.L(sexpr.A("layer"), sexpr.S(string(text.Layer))),
	}
	if text.UUID.Valid() {
		nodes = append(nodes, sexpr.L(sexpr.A("uuid"), sexpr.S(string(text.UUID))))
	}
	return sexpr.L(nodes...)
}

func renderPad(pad Pad, netName string) sexpr.List {
	if strings.TrimSpace(pad.NetName) != "" {
		netName = pad.NetName
	}
	nodes := []sexpr.Node{
		sexpr.A("pad"),
		sexpr.S(pad.Name),
		sexpr.A(padType(pad)),
		sexpr.A(pad.Shape),
		renderAt(pad.Position, pad.Rotation),
		sexpr.L(sexpr.A("size"), fixed(pad.Size.X), fixed(pad.Size.Y)),
		renderLayerList("layers", pad.Layers),
	}
	if pad.Drill > 0 {
		nodes = append(nodes, sexpr.L(sexpr.A("drill"), fixed(pad.Drill)))
	}
	if pad.RemoveUnusedLayers != nil {
		nodes = append(nodes, sexpr.L(sexpr.A("remove_unused_layers"), yesNo(*pad.RemoveUnusedLayers)))
	}
	if strings.TrimSpace(pad.PinFunction) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("pinfunction"), sexpr.S(pad.PinFunction)))
	}
	if strings.TrimSpace(pad.PinType) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("pintype"), sexpr.S(pad.PinType)))
	}
	if pad.ThermalBridgeAngle != nil {
		nodes = append(nodes, sexpr.L(sexpr.A("thermal_bridge_angle"), sexpr.F(*pad.ThermalBridgeAngle)))
	}
	if pad.Shape == "roundrect" {
		nodes = append(nodes, sexpr.L(sexpr.A("roundrect_rratio"), sexpr.F(roundRectRRatio(pad))))
	}
	if pad.NetCode > 0 || strings.TrimSpace(pad.NetName) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("net"), sexpr.S(netName)))
	}
	if pad.UUID.Valid() {
		nodes = append(nodes, sexpr.L(sexpr.A("uuid"), sexpr.S(string(pad.UUID))))
	}
	if pad.Teardrops != nil {
		nodes = append(nodes, renderTeardrops(*pad.Teardrops))
	}
	return sexpr.L(nodes...)
}

func renderTeardrops(teardrops TeardropSettings) sexpr.List {
	return sexpr.L(
		sexpr.A("teardrops"),
		sexpr.L(sexpr.A("best_length_ratio"), sexpr.F(teardrops.BestLengthRatio)),
		sexpr.L(sexpr.A("max_length"), fixed(teardrops.MaxLength)),
		sexpr.L(sexpr.A("best_width_ratio"), sexpr.F(teardrops.BestWidthRatio)),
		sexpr.L(sexpr.A("max_width"), fixed(teardrops.MaxWidth)),
		sexpr.L(sexpr.A("curved_edges"), yesNo(teardrops.CurvedEdges)),
		sexpr.L(sexpr.A("filter_ratio"), sexpr.F(teardrops.FilterRatio)),
		sexpr.L(sexpr.A("enabled"), yesNo(teardrops.Enabled)),
		sexpr.L(sexpr.A("allow_two_segments"), yesNo(teardrops.AllowTwoSegments)),
		sexpr.L(sexpr.A("prefer_zone_connections"), yesNo(teardrops.PreferZoneConnection)),
	)
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
			renderStroke(drawing.Line.Width, drawing.StrokeType),
		)
	case drawing.Rect != nil:
		nodes = append(nodes,
			sexpr.L(sexpr.A("start"), fixed(drawing.Rect.Start.X), fixed(drawing.Rect.Start.Y)),
			sexpr.L(sexpr.A("end"), fixed(drawing.Rect.End.X), fixed(drawing.Rect.End.Y)),
			renderStroke(drawing.Rect.Width, drawing.StrokeType),
			sexpr.L(sexpr.A("fill"), sexpr.A(fillMode(drawing.Fill))),
		)
	case drawing.Circle != nil:
		nodes = append(nodes,
			sexpr.L(sexpr.A("center"), fixed(drawing.Circle.Center.X), fixed(drawing.Circle.Center.Y)),
			sexpr.L(sexpr.A("end"), fixed(drawing.Circle.End.X), fixed(drawing.Circle.End.Y)),
			renderStroke(drawing.Circle.Width, drawing.StrokeType),
			sexpr.L(sexpr.A("fill"), sexpr.A(fillMode(drawing.Fill))),
		)
	case drawing.Arc != nil:
		nodes = append(nodes,
			sexpr.L(sexpr.A("start"), fixed(drawing.Arc.Start.X), fixed(drawing.Arc.Start.Y)),
			sexpr.L(sexpr.A("mid"), fixed(drawing.Arc.Mid.X), fixed(drawing.Arc.Mid.Y)),
			sexpr.L(sexpr.A("end"), fixed(drawing.Arc.End.X), fixed(drawing.Arc.End.Y)),
			renderStroke(drawing.Arc.Width, drawing.StrokeType),
			sexpr.L(sexpr.A("fill"), sexpr.A(fillMode(drawing.Fill))),
		)
	case drawing.Poly != nil:
		nodes = append(nodes, renderPoints(drawing.Poly.Points), renderStroke(drawing.Poly.Width, drawing.StrokeType), sexpr.L(sexpr.A("fill"), sexpr.A(fillMode(drawing.Fill))))
	case drawing.Curve != nil:
		nodes = append(nodes, renderPoints(drawing.Curve.Points), renderStroke(drawing.Curve.Width, drawing.StrokeType))
		if strings.TrimSpace(drawing.Fill) != "" {
			nodes = append(nodes, sexpr.L(sexpr.A("fill"), sexpr.A(fillMode(drawing.Fill))))
		}
	case drawing.Text != nil:
		nodes = append(nodes,
			sexpr.S(drawing.Text.Text),
			renderAt(drawing.Text.Position, drawing.Text.Rotation),
		)
	}
	nodes = append(nodes,
		sexpr.L(sexpr.A("layer"), sexpr.S(string(drawing.Layer))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(drawing.UUID))),
	)
	if drawing.Text != nil {
		nodes = append(nodes, renderEffects(drawing.Text.Effects))
	}
	if drawing.NetCode > 0 && drawing.Poly != nil {
		nodes = append(nodes, sexpr.L(sexpr.A("net"), sexpr.I(int64(drawing.NetCode))))
	}
	return sexpr.L(nodes...)
}

func renderStroke(width kicadfiles.IU, strokeType string) sexpr.List {
	if strings.TrimSpace(strokeType) == "" {
		strokeType = "solid"
	}
	return sexpr.L(sexpr.A("stroke"), sexpr.L(sexpr.A("width"), fixed(width)), sexpr.L(sexpr.A("type"), sexpr.A(strokeType)))
}

func fillMode(fill string) string {
	if strings.TrimSpace(fill) == "" {
		return "none"
	}
	return fill
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultIU(value, fallback kicadfiles.IU) kicadfiles.IU {
	if value == 0 {
		return fallback
	}
	return value
}

func renderTrack(track Track, netNames map[int]string) sexpr.List {
	return sexpr.L(
		sexpr.A("segment"),
		sexpr.L(sexpr.A("start"), fixed(track.Start.X), fixed(track.Start.Y)),
		sexpr.L(sexpr.A("end"), fixed(track.End.X), fixed(track.End.Y)),
		sexpr.L(sexpr.A("width"), fixed(track.Width)),
		sexpr.L(sexpr.A("layer"), sexpr.S(string(track.Layer))),
		sexpr.L(sexpr.A("net"), sexpr.S(routedNetName(track.NetCode, track.NetName, netNames))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(track.UUID))),
	)
}

func renderTrackArc(arc TrackArc, netNames map[int]string) sexpr.List {
	return sexpr.L(
		sexpr.A("arc"),
		sexpr.L(sexpr.A("start"), fixed(arc.Start.X), fixed(arc.Start.Y)),
		sexpr.L(sexpr.A("mid"), fixed(arc.Mid.X), fixed(arc.Mid.Y)),
		sexpr.L(sexpr.A("end"), fixed(arc.End.X), fixed(arc.End.Y)),
		sexpr.L(sexpr.A("width"), fixed(arc.Width)),
		sexpr.L(sexpr.A("layer"), sexpr.S(string(arc.Layer))),
		sexpr.L(sexpr.A("net"), sexpr.S(routedNetName(arc.NetCode, arc.NetName, netNames))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(arc.UUID))),
	)
}

func renderVia(via Via, netNames map[int]string) sexpr.List {
	nodes := []sexpr.Node{
		sexpr.A("via"),
		sexpr.L(sexpr.A("at"), fixed(via.Position.X), fixed(via.Position.Y)),
		sexpr.L(sexpr.A("size"), fixed(via.Size)),
		sexpr.L(sexpr.A("drill"), fixed(via.Drill)),
		renderLayerList("layers", via.Layers),
	}
	if via.TentingFront || via.TentingBack {
		nodes = append(nodes, renderSidePair("tenting", via.TentingFront, via.TentingBack))
	}
	nodes = append(nodes,
		sexpr.L(sexpr.A("net"), sexpr.S(routedNetName(via.NetCode, via.NetName, netNames))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(via.UUID))),
	)
	return sexpr.L(nodes...)
}

func routedNetName(code int, explicit string, netNames map[int]string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	return netNames[code]
}

func renderZone(zone Zone, netNames map[int]string) sexpr.List {
	netName := netNames[zone.NetCode]
	if strings.TrimSpace(zone.NetName) != "" {
		netName = zone.NetName
	}
	nodes := []sexpr.Node{
		sexpr.A("zone"),
		sexpr.L(sexpr.A("net"), sexpr.I(int64(zone.NetCode))),
		sexpr.L(sexpr.A("net_name"), sexpr.S(netName)),
		renderLayerList("layers", zone.Layers),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(zone.UUID))),
	}
	if strings.TrimSpace(zone.Name) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("name"), sexpr.S(zone.Name)))
	}
	nodes = append(nodes, sexpr.L(sexpr.A("hatch"), sexpr.A(defaultString(zone.HatchStyle, "edge")), fixed(defaultIU(zone.HatchPitch, kicadfiles.MM(0.5)))))
	if zone.Priority > 0 {
		nodes = append(nodes, sexpr.L(sexpr.A("priority"), sexpr.I(int64(zone.Priority))))
	}
	for _, attr := range zone.Attributes {
		nodes = append(nodes, renderZoneAttribute(attr))
	}
	nodes = append(nodes,
		sexpr.L(sexpr.A("connect_pads"), sexpr.A(zoneConnectMode(zone)), sexpr.L(sexpr.A("clearance"), fixed(zone.Clearance))),
		sexpr.L(sexpr.A("min_thickness"), fixed(defaultIU(zone.MinThickness, kicadfiles.MM(0.25)))),
		sexpr.L(sexpr.A("filled_areas_thickness"), yesNo(zone.FilledAreasThickness)),
	)
	if zone.Keepout != nil {
		nodes = append(nodes, renderZoneKeepout(*zone.Keepout))
	}
	nodes = append(nodes, renderZoneFill(zone.Fill))
	for _, polygon := range zone.Polygons {
		nodes = append(nodes, sexpr.L(sexpr.A("polygon"), renderPoints(polygon)))
	}
	for _, polygon := range zone.FilledPolygons {
		nodes = append(nodes, sexpr.L(sexpr.A("filled_polygon"), sexpr.L(sexpr.A("layer"), sexpr.S(string(polygon.Layer))), renderPoints(polygon.Points)))
	}
	return sexpr.L(nodes...)
}

func renderZoneKeepout(keepout ZoneKeepout) sexpr.List {
	return sexpr.L(
		sexpr.A("keepout"),
		sexpr.L(sexpr.A("tracks"), sexpr.A(defaultString(keepout.Tracks, "allowed"))),
		sexpr.L(sexpr.A("vias"), sexpr.A(defaultString(keepout.Vias, "allowed"))),
		sexpr.L(sexpr.A("pads"), sexpr.A(defaultString(keepout.Pads, "allowed"))),
		sexpr.L(sexpr.A("copperpour"), sexpr.A(defaultString(keepout.CopperPour, "allowed"))),
		sexpr.L(sexpr.A("footprints"), sexpr.A(defaultString(keepout.Footprints, "allowed"))),
	)
}

func renderZoneAttribute(attr ZoneAttribute) sexpr.List {
	children := []sexpr.Node{sexpr.A(attr.Name)}
	keys := make([]string, 0, len(attr.Values))
	for key := range attr.Values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		children = append(children, sexpr.L(sexpr.A(key), sexpr.S(attr.Values[key])))
	}
	return sexpr.L(sexpr.A("attr"), sexpr.L(children...))
}

func zoneConnectMode(zone Zone) string {
	if strings.TrimSpace(zone.ConnectPadsMode) != "" {
		return zone.ConnectPadsMode
	}
	if zone.ConnectPads {
		return "yes"
	}
	return "no"
}

func renderZoneFill(fill ZoneFillSettings) sexpr.List {
	return sexpr.L(
		sexpr.A("fill"),
		yesNo(fill.Enabled),
		sexpr.L(sexpr.A("thermal_gap"), fixed(fill.ThermalGap)),
		sexpr.L(sexpr.A("thermal_bridge_width"), fixed(fill.ThermalBridgeWidth)),
		sexpr.L(sexpr.A("island_removal_mode"), sexpr.I(int64(fill.IslandRemovalMode))),
		sexpr.L(sexpr.A("island_area_min"), sexpr.F(fill.IslandAreaMin)),
	)
}

func renderDimension(dimension Dimension) sexpr.List {
	return sexpr.L(
		sexpr.A("dimension"),
		sexpr.L(sexpr.A("type"), sexpr.A(dimension.Type)),
		sexpr.L(sexpr.A("layer"), sexpr.S(string(dimension.Layer))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(dimension.UUID))),
		renderPoints(dimension.Points),
		sexpr.L(sexpr.A("height"), fixed(dimension.Height)),
		sexpr.L(
			sexpr.A("gr_text"),
			sexpr.S(dimension.Text),
			renderAt(dimension.Position, dimension.Rotation),
			sexpr.L(sexpr.A("layer"), sexpr.S(string(dimension.Layer))),
			sexpr.L(sexpr.A("uuid"), sexpr.S(string(dimension.UUID))),
			renderEffects(dimension.Effects),
		),
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

func renderAtWithRotation(point kicadfiles.Point, rotation kicadfiles.Angle) sexpr.List {
	return sexpr.L(sexpr.A("at"), fixed(point.X), fixed(point.Y), sexpr.F(float64(rotation)))
}

func renderLayerList(name string, layers []kicadfiles.BoardLayer) sexpr.List {
	nodes := []sexpr.Node{sexpr.A(name)}
	for _, layer := range layers {
		nodes = append(nodes, sexpr.S(string(layer)))
	}
	return sexpr.L(nodes...)
}

func fixed(value kicadfiles.IU) sexpr.Fixed {
	formatted := kicadfiles.ToMMString(value)
	if strings.Contains(formatted, ".") {
		formatted = strings.TrimRight(formatted, "0")
		formatted = strings.TrimSuffix(formatted, ".")
	}
	return sexpr.X(formatted)
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
		return cmp.Compare(string(a.UUID), string(b.UUID))
	})
	return ordered
}

func routedNetSortCodes(footprints []Footprint, netNames map[int]string) map[string]int {
	codes := map[string]int{}
	next := 1
	for _, footprint := range footprints {
		for _, pad := range footprint.Pads {
			netName := routedNetName(pad.NetCode, pad.NetName, netNames)
			if strings.TrimSpace(netName) == "" {
				continue
			}
			if _, ok := codes[netName]; ok {
				continue
			}
			codes[netName] = next
			next++
		}
	}
	return codes
}

func sortedDrawings(drawings []Drawing, layers map[kicadfiles.BoardLayer]int) []Drawing {
	ordered := slices.Clone(drawings)
	slices.SortFunc(ordered, func(a, b Drawing) int {
		if byType := cmp.Compare(drawingTypeOrder(a), drawingTypeOrder(b)); byType != 0 {
			return byType
		}
		if byLayer := cmp.Compare(layerNumber(a.Layer, layers), layerNumber(b.Layer, layers)); byLayer != 0 {
			return byLayer
		}
		if drawingTypeOrder(a) == kicadTypePCBShape {
			if byShape := compareShape(a, b); byShape != 0 {
				return byShape
			}
		}
		return cmp.Compare(string(a.UUID), string(b.UUID))
	})
	return ordered
}

type routedItem struct {
	kind    int
	netCode int
	layer   kicadfiles.BoardLayer
	uuid    kicadfiles.UUID
	track   *Track
	arc     *TrackArc
	via     *Via
}

func (item routedItem) render(netNames map[int]string) sexpr.List {
	switch {
	case item.track != nil:
		return renderTrack(*item.track, netNames)
	case item.arc != nil:
		return renderTrackArc(*item.arc, netNames)
	case item.via != nil:
		return renderVia(*item.via, netNames)
	default:
		return nil
	}
}

func sortedRoutedItems(tracks []Track, arcs []TrackArc, vias []Via, layers map[kicadfiles.BoardLayer]int, netNames map[int]string, netSortCodes map[string]int) []routedItem {
	items := make([]routedItem, 0, len(tracks)+len(arcs)+len(vias))
	for i := range tracks {
		items = append(items, routedItem{
			kind:    kicadTypePCBTrace,
			netCode: routedItemNetSortCode(tracks[i].NetCode, tracks[i].NetName, netNames, netSortCodes),
			layer:   tracks[i].Layer,
			uuid:    tracks[i].UUID,
			track:   &tracks[i],
		})
	}
	for i := range arcs {
		items = append(items, routedItem{
			kind:    kicadTypePCBArc,
			netCode: routedItemNetSortCode(arcs[i].NetCode, arcs[i].NetName, netNames, netSortCodes),
			layer:   arcs[i].Layer,
			uuid:    arcs[i].UUID,
			arc:     &arcs[i],
		})
	}
	for i := range vias {
		items = append(items, routedItem{
			kind:    kicadTypePCBVia,
			netCode: routedItemNetSortCode(vias[i].NetCode, vias[i].NetName, netNames, netSortCodes),
			layer:   viaTopLayer(vias[i]),
			uuid:    vias[i].UUID,
			via:     &vias[i],
		})
	}
	slices.SortFunc(items, func(a, b routedItem) int {
		if byNet := cmp.Compare(a.netCode, b.netCode); byNet != 0 {
			return byNet
		}
		if byLayer := cmp.Compare(layerNumber(a.layer, layers), layerNumber(b.layer, layers)); byLayer != 0 {
			return byLayer
		}
		if byType := cmp.Compare(a.kind, b.kind); byType != 0 {
			return byType
		}
		return cmp.Compare(string(a.uuid), string(b.uuid))
	})
	return items
}

func routedItemNetSortCode(code int, explicit string, netNames map[int]string, netSortCodes map[string]int) int {
	name := routedNetName(code, explicit, netNames)
	if sortCode, ok := netSortCodes[name]; ok {
		return sortCode
	}
	return code
}

const (
	kicadTypePCBShape = 5
	kicadTypePCBText  = 9
	kicadTypePCBTrace = 13
	kicadTypePCBVia   = 14
	kicadTypePCBArc   = 15

	kicadShapeSegment = 0
	kicadShapeRect    = 1
	kicadShapeArc     = 2
	kicadShapeCircle  = 3
	kicadShapePoly    = 4
)

func drawingTypeOrder(drawing Drawing) int {
	if drawing.Text != nil {
		return kicadTypePCBText
	}
	return kicadTypePCBShape
}

func compareShape(a, b Drawing) int {
	aStart, aEnd := shapeStartEnd(a)
	bStart, bEnd := shapeStartEnd(b)
	if byStart := comparePoint(aStart, bStart); byStart != 0 {
		return byStart
	}
	if byEnd := comparePoint(aEnd, bEnd); byEnd != 0 {
		return byEnd
	}
	if byShape := cmp.Compare(shapeOrder(a), shapeOrder(b)); byShape != 0 {
		return byShape
	}
	if shapeOrder(a) == kicadShapeArc {
		if byMid := comparePoint(a.Arc.Mid, b.Arc.Mid); byMid != 0 {
			return byMid
		}
	}
	if shapeOrder(a) == kicadShapePoly {
		if byVertices := cmp.Compare(len(a.Poly.Points), len(b.Poly.Points)); byVertices != 0 {
			return byVertices
		}
		for i := range a.Poly.Points {
			if byPoint := comparePoint(a.Poly.Points[i], b.Poly.Points[i]); byPoint != 0 {
				return byPoint
			}
		}
	}
	if byWidth := cmp.Compare(shapeWidth(a), shapeWidth(b)); byWidth != 0 {
		return byWidth
	}
	return cmp.Compare(fillOrder(a.Fill), fillOrder(b.Fill))
}

func shapeStartEnd(drawing Drawing) (kicadfiles.Point, kicadfiles.Point) {
	switch {
	case drawing.Line != nil:
		return drawing.Line.Start, drawing.Line.End
	case drawing.Rect != nil:
		return drawing.Rect.Start, drawing.Rect.End
	case drawing.Arc != nil:
		return drawing.Arc.Start, drawing.Arc.End
	case drawing.Circle != nil:
		return drawing.Circle.Center, drawing.Circle.End
	case drawing.Poly != nil && len(drawing.Poly.Points) > 0:
		return drawing.Poly.Points[0], drawing.Poly.Points[len(drawing.Poly.Points)-1]
	default:
		return kicadfiles.Point{}, kicadfiles.Point{}
	}
}

func shapeOrder(drawing Drawing) int {
	switch {
	case drawing.Line != nil:
		return kicadShapeSegment
	case drawing.Rect != nil:
		return kicadShapeRect
	case drawing.Arc != nil:
		return kicadShapeArc
	case drawing.Circle != nil:
		return kicadShapeCircle
	case drawing.Poly != nil:
		return kicadShapePoly
	default:
		return kicadShapeSegment
	}
}

func shapeWidth(drawing Drawing) kicadfiles.IU {
	switch {
	case drawing.Line != nil:
		return drawing.Line.Width
	case drawing.Rect != nil:
		return drawing.Rect.Width
	case drawing.Arc != nil:
		return drawing.Arc.Width
	case drawing.Circle != nil:
		return drawing.Circle.Width
	case drawing.Poly != nil:
		return drawing.Poly.Width
	default:
		return 0
	}
}

func comparePoint(a, b kicadfiles.Point) int {
	if byX := cmp.Compare(a.X, b.X); byX != 0 {
		return byX
	}
	return cmp.Compare(a.Y, b.Y)
}

func fillOrder(fill string) int {
	switch fillMode(fill) {
	case "solid", "yes":
		return 2
	case "background":
		return 3
	case "none":
		return 1
	default:
		return 1
	}
}

func layerNumberMap(layers []LayerDefinition) map[kicadfiles.BoardLayer]int {
	numbers := make(map[kicadfiles.BoardLayer]int, len(layers))
	for _, layer := range layers {
		numbers[layer.Name] = layer.Number
	}
	return numbers
}

func layerNumber(layer kicadfiles.BoardLayer, layers map[kicadfiles.BoardLayer]int) int {
	if number, ok := layers[layer]; ok {
		return number
	}
	if strings.HasPrefix(string(layer), "In") && strings.HasSuffix(string(layer), ".Cu") {
		index, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(string(layer), "In"), ".Cu"))
		if err == nil {
			return index*2 + 2
		}
	}
	return int(^uint(0) >> 1)
}

func viaTopLayer(via Via) kicadfiles.BoardLayer {
	if len(via.Layers) == 0 {
		return ""
	}
	return via.Layers[0]
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

func validateFootprint(index int, footprint Footprint, netCodes map[int]struct{}, netNames map[int]string) kicadfiles.ValidationErrors {
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
	referenceProperty := ""
	valueProperty := ""
	hasReferenceProperty := false
	hasValueProperty := false
	propertyNames := map[string]struct{}{}
	for propertyIndex, property := range footprint.Properties {
		errs = append(errs, validateFootprintProperty(prefix("properties"), propertyIndex, property)...)
		propertyName := strings.TrimSpace(property.Name)
		if _, exists := propertyNames[propertyName]; exists {
			errs = append(errs, fieldError(indexed(prefix("properties"), propertyIndex, "name"), "duplicate"))
		}
		propertyNames[propertyName] = struct{}{}
		switch property.Name {
		case "Reference":
			referenceProperty = property.Value
			hasReferenceProperty = true
		case "Value":
			valueProperty = property.Value
			hasValueProperty = true
		}
	}
	for propertyIndex, property := range footprint.MetadataProperties {
		errs = append(errs, validateFootprintMetadataProperty(prefix("metadata_properties"), propertyIndex, property)...)
		propertyName := strings.TrimSpace(property.Name)
		if _, exists := propertyNames[propertyName]; exists {
			errs = append(errs, fieldError(indexed(prefix("metadata_properties"), propertyIndex, "name"), "duplicate"))
		}
		propertyNames[propertyName] = struct{}{}
	}
	attributes := map[string]struct{}{}
	for attrIndex, attr := range footprint.Attributes {
		trimmedAttr := strings.TrimSpace(attr)
		if trimmedAttr == "" {
			errs = append(errs, fieldError(indexedValue(prefix("attributes"), attrIndex), "required"))
		}
		if attr != trimmedAttr {
			errs = append(errs, fieldError(indexedValue(prefix("attributes"), attrIndex), "trimmed value required"))
		}
		if _, exists := attributes[trimmedAttr]; exists {
			errs = append(errs, fieldError(indexedValue(prefix("attributes"), attrIndex), "duplicate"))
		}
		attributes[trimmedAttr] = struct{}{}
	}
	unitNames := map[string]struct{}{}
	for unitIndex, unit := range footprint.Units {
		errs = append(errs, validateFootprintUnit(prefix("units"), unitIndex, unit)...)
		unitName := strings.TrimSpace(unit.Name)
		if _, exists := unitNames[unitName]; exists {
			errs = append(errs, fieldError(indexed(prefix("units"), unitIndex, "name"), "duplicate"))
		}
		unitNames[unitName] = struct{}{}
	}
	netTiePadGroups := map[string]struct{}{}
	for groupIndex, group := range footprint.NetTiePadGroups {
		trimmedGroup := strings.TrimSpace(group)
		if trimmedGroup == "" {
			errs = append(errs, fieldError(indexedValue(prefix("net_tie_pad_groups"), groupIndex), "required"))
		}
		if group != trimmedGroup {
			errs = append(errs, fieldError(indexedValue(prefix("net_tie_pad_groups"), groupIndex), "trimmed value required"))
		}
		if _, exists := netTiePadGroups[trimmedGroup]; exists {
			errs = append(errs, fieldError(indexedValue(prefix("net_tie_pad_groups"), groupIndex), "duplicate"))
		}
		netTiePadGroups[trimmedGroup] = struct{}{}
	}
	textKinds := map[string]string{}
	for textIndex, text := range footprint.Texts {
		errs = append(errs, validateFootprintText(prefix("texts"), textIndex, text)...)
		textKinds[text.Kind] = text.Text
	}
	if len(footprint.Properties) > 0 {
		if !hasReferenceProperty {
			referenceProperty = textKinds["reference"]
		}
		if !hasValueProperty {
			valueProperty = textKinds["value"]
		}
		if referenceProperty != footprint.Reference {
			errs = append(errs, fieldError(prefix("properties.Reference"), "must match footprint reference"))
		}
		if valueProperty != footprint.Value {
			errs = append(errs, fieldError(prefix("properties.Value"), "must match footprint value"))
		}
	} else {
		referenceText, hasReferenceText := textKinds["reference"]
		if !hasReferenceText || referenceText != footprint.Reference {
			errs = append(errs, fieldError(prefix("texts.reference"), "must match footprint reference"))
		}
		valueText, hasValueText := textKinds["value"]
		if !hasValueText || valueText != footprint.Value {
			errs = append(errs, fieldError(prefix("texts.value"), "must match footprint value"))
		}
	}
	padNames := make(map[string]struct{}, len(footprint.Pads))
	for padIndex, pad := range footprint.Pads {
		errs = append(errs, validatePad(prefix("pads"), padIndex, pad, netCodes, netNames)...)
		if _, ok := padNames[pad.Name]; ok {
			errs = append(errs, fieldError(indexed(prefix("pads"), padIndex, "name"), "duplicate"))
		}
		padNames[pad.Name] = struct{}{}
	}
	for graphicIndex, graphic := range footprint.Graphics {
		errs = append(errs, validateGraphic(indexedValue(prefix("graphics"), graphicIndex), Drawing(graphic))...)
	}
	for modelIndex, model := range footprint.Models {
		if strings.TrimSpace(model.Path) == "" {
			errs = append(errs, fieldError(indexed(prefix("models"), modelIndex, "path"), "required"))
		}
	}
	return errs
}

func validateFootprintMetadataProperty(collection string, index int, property FootprintMetadataProperty) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	trimmedName := strings.TrimSpace(property.Name)
	if trimmedName == "" {
		errs = append(errs, fieldError(indexed(collection, index, "name"), "required"))
	}
	if property.Name != trimmedName {
		errs = append(errs, fieldError(indexed(collection, index, "name"), "trimmed value required"))
	}
	return errs
}

func validateFootprintUnit(collection string, index int, unit FootprintUnit) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	trimmedName := strings.TrimSpace(unit.Name)
	if trimmedName == "" {
		errs = append(errs, fieldError(indexed(collection, index, "name"), "required"))
	}
	if unit.Name != trimmedName {
		errs = append(errs, fieldError(indexed(collection, index, "name"), "trimmed value required"))
	}
	if len(unit.Pins) == 0 {
		errs = append(errs, fieldError(indexed(collection, index, "pins"), "required"))
	}
	pins := map[string]struct{}{}
	for pinIndex, pin := range unit.Pins {
		trimmedPin := strings.TrimSpace(pin)
		if trimmedPin == "" {
			errs = append(errs, fieldError(indexedValue(indexed(collection, index, "pins"), pinIndex), "required"))
		}
		if pin != trimmedPin {
			errs = append(errs, fieldError(indexedValue(indexed(collection, index, "pins"), pinIndex), "trimmed value required"))
		}
		if _, exists := pins[trimmedPin]; exists {
			errs = append(errs, fieldError(indexedValue(indexed(collection, index, "pins"), pinIndex), "duplicate"))
		}
		pins[trimmedPin] = struct{}{}
	}
	return errs
}

func validateFootprintProperty(collection string, index int, property FootprintProperty) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if !property.UUID.Valid() {
		errs = append(errs, fieldError(indexed(collection, index, "uuid"), "valid UUID required"))
	}
	trimmedName := strings.TrimSpace(property.Name)
	if trimmedName == "" {
		errs = append(errs, fieldError(indexed(collection, index, "name"), "required"))
	}
	if property.Name != trimmedName {
		errs = append(errs, fieldError(indexed(collection, index, "name"), "trimmed value required"))
	}
	if !kicadfiles.IsValidBoardLayer(property.Layer) {
		errs = append(errs, fieldError(indexed(collection, index, "layer"), "invalid"))
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
	if !text.UUID.Valid() {
		errs = append(errs, fieldError(indexed(collection, index, "uuid"), "valid UUID required"))
	}
	if !kicadfiles.IsValidBoardLayer(text.Layer) {
		errs = append(errs, fieldError(indexed(collection, index, "layer"), "invalid"))
	}
	return errs
}

func validatePad(collection string, index int, pad Pad, netCodes map[int]struct{}, netNames map[int]string) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if strings.TrimSpace(pad.Name) == "" {
		errs = append(errs, fieldError(indexed(collection, index, "name"), "required"))
	}
	padType := padType(pad)
	if !isValidPadType(padType) {
		errs = append(errs, fieldError(indexed(collection, index, "type"), "invalid"))
	}
	if strings.TrimSpace(pad.Shape) == "" {
		errs = append(errs, fieldError(indexed(collection, index, "shape"), "required"))
	}
	if !isValidPadShape(pad.Shape) {
		errs = append(errs, fieldError(indexed(collection, index, "shape"), "invalid"))
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
	padLayers := map[kicadfiles.BoardLayer]struct{}{}
	for layerIndex, layer := range pad.Layers {
		if !kicadfiles.IsValidBoardLayer(layer) {
			errs = append(errs, fieldError(indexedValue(indexed(collection, index, "layers"), layerIndex), "invalid"))
		}
		if _, exists := padLayers[layer]; exists {
			errs = append(errs, fieldError(indexedValue(indexed(collection, index, "layers"), layerIndex), "duplicate"))
		}
		padLayers[layer] = struct{}{}
	}
	if (padType == "thru_hole" || padType == "np_thru_hole") && pad.Drill <= 0 {
		errs = append(errs, fieldError(indexed(collection, index, "drill"), "required for through-hole pads"))
	}
	if padType == "smd" && pad.Drill > 0 {
		errs = append(errs, fieldError(indexed(collection, index, "drill"), "not allowed for SMD pads"))
	}
	if padType == "smd" && !validSMDPadLayers(pad.Layers) {
		errs = append(errs, fieldError(indexed(collection, index, "layers"), "SMD pads require a single copper side with matching mask/paste side"))
	}
	if pad.Drill > 0 && !validDrilledPadLayers(pad.Layers) {
		errs = append(errs, fieldError(indexed(collection, index, "layers"), "drilled pads require through copper and mask layers"))
	}
	if _, ok := netCodes[pad.NetCode]; !ok {
		errs = append(errs, fieldError(indexed(collection, index, "net_code"), "unknown"))
	}
	if strings.TrimSpace(pad.NetName) != "" && pad.NetName != netNames[pad.NetCode] {
		errs = append(errs, fieldError(indexed(collection, index, "net_name"), "must match net code"))
	}
	return errs
}

func validateDrawing(index int, drawing Drawing, netCodes map[int]struct{}, netNames map[int]string) kicadfiles.ValidationErrors {
	errs := validateGraphic(indexedValue("drawings", index), drawing)
	if drawing.NetCode > 0 {
		if drawing.Poly == nil {
			errs = append(errs, fieldError(indexed("drawings", index, "net_code"), "only supported for copper polygons"))
		}
		if _, ok := netCodes[drawing.NetCode]; !ok {
			errs = append(errs, fieldError(indexed("drawings", index, "net_code"), "unknown"))
		}
		if strings.TrimSpace(drawing.NetName) != "" && drawing.NetName != netNames[drawing.NetCode] {
			errs = append(errs, fieldError(indexed("drawings", index, "net_name"), "must match net code"))
		}
	}
	return errs
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
	case drawing.Rect != nil:
		if drawing.Rect.Width < 0 {
			errs = append(errs, fieldError(prefix+".rect.width", "must be non-negative"))
		}
		if drawing.Rect.Start.X == drawing.Rect.End.X || drawing.Rect.Start.Y == drawing.Rect.End.Y {
			errs = append(errs, fieldError(prefix+".rect", "must have non-zero area"))
		}
	case drawing.Circle != nil:
		if drawing.Circle.Width < 0 {
			errs = append(errs, fieldError(prefix+".circle.width", "must be non-negative"))
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
		if drawing.Poly.Width < 0 {
			errs = append(errs, fieldError(prefix+".poly.width", "must be non-negative"))
		}
		if countDistinctPoints(drawing.Poly.Points) < 2 {
			errs = append(errs, fieldError(prefix+".poly.points", "at least two distinct points required"))
		}
	case drawing.Curve != nil:
		if drawing.Curve.Width < 0 {
			errs = append(errs, fieldError(prefix+".curve.width", "must be non-negative"))
		}
		if len(drawing.Curve.Points) != 4 {
			errs = append(errs, fieldError(prefix+".curve.points", "exactly four points required"))
		}
		if countDistinctPoints(drawing.Curve.Points) < 2 {
			errs = append(errs, fieldError(prefix+".curve.points", "at least two distinct points required"))
		}
	case drawing.Text != nil:
		if strings.TrimSpace(drawing.Text.Text) == "" {
			errs = append(errs, fieldError(prefix+".text", "required"))
		}
	}
	return errs
}

func validateClosedOutline(drawings []Drawing) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	degrees := map[kicadfiles.Point]int{}
	outlinePoints := map[outlineCell][]kicadfiles.Point{}
	hasClosedShape := false
	for _, drawing := range drawings {
		if drawing.Layer == kicadfiles.LayerEdge && (drawing.Rect != nil || drawing.Circle != nil || drawing.Poly != nil) {
			hasClosedShape = true
			continue
		}
		if drawing.Layer != kicadfiles.LayerEdge || drawing.Line == nil {
			continue
		}
		start := canonicalOutlinePoint(drawing.Line.Start, outlinePoints)
		end := canonicalOutlinePoint(drawing.Line.End, outlinePoints)
		degrees[start]++
		degrees[end]++
	}
	if len(degrees) == 0 {
		if hasClosedShape {
			return nil
		}
		return append(errs, fieldError("drawings.edge_cuts", "closed outline required"))
	}
	for point, degree := range degrees {
		if degree != 2 {
			errs = append(errs, fieldError("drawings.edge_cuts", "outline endpoint "+kicadfiles.ToMMString(point.X)+","+kicadfiles.ToMMString(point.Y)+" is not closed"))
		}
	}
	return errs
}

type outlineCell struct {
	x int64
	y int64
}

func canonicalOutlinePoint(point kicadfiles.Point, grid map[outlineCell][]kicadfiles.Point) kicadfiles.Point {
	cell := pointOutlineCell(point)
	for dx := int64(-1); dx <= 1; dx++ {
		for dy := int64(-1); dy <= 1; dy++ {
			for _, existing := range grid[outlineCell{x: cell.x + dx, y: cell.y + dy}] {
				xDelta := absIU(point.X - existing.X)
				if xDelta > outlineClosureToleranceIU {
					continue
				}
				yDelta := absIU(point.Y - existing.Y)
				if yDelta > outlineClosureToleranceIU {
					continue
				}
				if xDelta*xDelta+yDelta*yDelta <= outlineClosureToleranceIU*outlineClosureToleranceIU {
					return existing
				}
			}
		}
	}
	grid[cell] = append(grid[cell], point)
	return point
}

func pointOutlineCell(point kicadfiles.Point) outlineCell {
	return outlineCell{
		x: floorDivIU(point.X, outlineClosureToleranceIU),
		y: floorDivIU(point.Y, outlineClosureToleranceIU),
	}
}

func floorDivIU(value, divisor kicadfiles.IU) int64 {
	quotient := int64(value / divisor)
	if value < 0 && value%divisor != 0 {
		quotient--
	}
	return quotient
}

func absIU(value kicadfiles.IU) kicadfiles.IU {
	if value < 0 {
		return -value
	}
	return value
}

func validateTrack(index int, track Track, netCodes map[int]struct{}, netNames map[int]string) kicadfiles.ValidationErrors {
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
	errs = append(errs, validateRoutedNet(prefix, track.NetCode, track.NetName, netCodes, netNames)...)
	return errs
}

func validateTrackArc(index int, arc TrackArc, netCodes map[int]struct{}, netNames map[int]string) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := func(field string) string { return indexed("track_arcs", index, field) }
	if !arc.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if arc.Start == arc.Mid || arc.Mid == arc.End || arc.Start == arc.End {
		errs = append(errs, fieldError(prefix("points"), "start, mid, and end must be distinct"))
	}
	if collinear(arc.Start, arc.Mid, arc.End) {
		errs = append(errs, fieldError(prefix("points"), "start, mid, and end must not be collinear"))
	}
	if arc.Width <= 0 {
		errs = append(errs, fieldError(prefix("width"), "must be positive"))
	}
	if !isCopperLayer(arc.Layer) {
		errs = append(errs, fieldError(prefix("layer"), "must be copper"))
	}
	errs = append(errs, validateRoutedNet(prefix, arc.NetCode, arc.NetName, netCodes, netNames)...)
	return errs
}

func collinear(a, b, c kicadfiles.Point) bool {
	left := new(big.Int).Mul(big.NewInt(int64(b.Y-a.Y)), big.NewInt(int64(c.X-b.X)))
	right := new(big.Int).Mul(big.NewInt(int64(c.Y-b.Y)), big.NewInt(int64(b.X-a.X)))
	return left.Cmp(right) == 0
}

func validateVia(index int, via Via, netCodes map[int]struct{}, netNames map[int]string) kicadfiles.ValidationErrors {
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
	errs = append(errs, validateRoutedNet(prefix, via.NetCode, via.NetName, netCodes, netNames)...)
	copperLayerCount := 0
	for layerIndex, layer := range via.Layers {
		duplicateLayer := false
		for previousIndex := 0; previousIndex < layerIndex; previousIndex++ {
			if via.Layers[previousIndex] == layer {
				errs = append(errs, fieldError(indexedValue(prefix("layers"), layerIndex), "duplicate"))
				duplicateLayer = true
				break
			}
		}
		if !isCopperLayer(layer) {
			errs = append(errs, fieldError(indexedValue(prefix("layers"), layerIndex), "must be copper"))
		} else if !duplicateLayer {
			copperLayerCount++
		}
	}
	if copperLayerCount < 2 {
		errs = append(errs, fieldError(prefix("layers"), "at least two copper layers required"))
	}
	return errs
}

func validateRoutedNet(prefix func(string) string, code int, name string, netCodes map[int]struct{}, netNames map[int]string) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if _, ok := netCodes[code]; !ok {
		return append(errs, fieldError(prefix("net_code"), "unknown"))
	}
	return validateExplicitNetName(prefix("net_name"), code, name, netNames)
}

func validateExplicitNetName(field string, code int, name string, netNames map[int]string) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	trimmed := strings.TrimSpace(name)
	if name != trimmed {
		errs = append(errs, fieldError(field, "trimmed value required"))
		return errs
	}
	if name != "" && name != netNames[code] {
		errs = append(errs, fieldError(field, "must match net code"))
	}
	return errs
}

func validateZone(index int, zone Zone, netCodes map[int]struct{}, netNames map[int]string) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := func(field string) string { return indexed("zones", index, field) }
	if !zone.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if _, ok := netCodes[zone.NetCode]; !ok {
		errs = append(errs, fieldError(prefix("net_code"), "unknown"))
	}
	if expected, ok := netNames[zone.NetCode]; ok && strings.TrimSpace(zone.NetName) != "" && zone.NetName != expected {
		errs = append(errs, fieldError(prefix("net_name"), "must match net code"))
	}
	if zone.Keepout != nil {
		if zone.NetCode != 0 {
			errs = append(errs, fieldError(prefix("net_code"), "must be 0 for keepout zones"))
		}
		if strings.TrimSpace(zone.NetName) != "" {
			errs = append(errs, fieldError(prefix("net_name"), "must be empty for keepout zones"))
		}
		errs = append(errs, validateZoneKeepout(prefix, *zone.Keepout)...)
	}
	if len(zone.Layers) == 0 {
		errs = append(errs, fieldError(prefix("layers"), "required"))
	}
	zoneLayers := make(map[kicadfiles.BoardLayer]struct{}, len(zone.Layers))
	for layerIndex, layer := range zone.Layers {
		if zone.Keepout != nil {
			if !kicadfiles.IsValidBoardLayer(layer) {
				errs = append(errs, fieldError(indexedValue(prefix("layers"), layerIndex), "invalid"))
			}
		} else if !isCopperLayer(layer) {
			errs = append(errs, fieldError(indexedValue(prefix("layers"), layerIndex), "must be copper"))
		}
		if _, ok := zoneLayers[layer]; ok {
			errs = append(errs, fieldError(indexedValue(prefix("layers"), layerIndex), "duplicate"))
		}
		zoneLayers[layer] = struct{}{}
	}
	if len(zone.Polygons) == 0 {
		errs = append(errs, fieldError(prefix("polygons"), "required"))
	}
	for polygonIndex, polygon := range zone.Polygons {
		if countDistinctPoints(polygon) < 3 {
			errs = append(errs, fieldError(indexed(prefix("polygons"), polygonIndex, "points"), "at least three distinct points required"))
		}
	}
	if zone.Keepout != nil && len(zone.FilledPolygons) > 0 {
		errs = append(errs, fieldError(prefix("filled_polygons"), "not allowed for keepout zones"))
	}
	for polygonIndex, polygon := range zone.FilledPolygons {
		if !isCopperLayer(polygon.Layer) {
			errs = append(errs, fieldError(indexed(prefix("filled_polygons"), polygonIndex, "layer"), "must be copper"))
		}
		if _, ok := zoneLayers[polygon.Layer]; !ok {
			errs = append(errs, fieldError(indexed(prefix("filled_polygons"), polygonIndex, "layer"), "must be declared in zone layers"))
		}
		if countDistinctPoints(polygon.Points) < 3 {
			errs = append(errs, fieldError(indexed(prefix("filled_polygons"), polygonIndex, "points"), "at least three distinct points required"))
		}
	}
	if zone.Priority < 0 {
		errs = append(errs, fieldError(prefix("priority"), "must be non-negative"))
	}
	if !isValidZoneConnectMode(zoneConnectMode(zone)) {
		errs = append(errs, fieldError(prefix("connect_pads"), "invalid"))
	}
	if zone.MinThickness < 0 {
		errs = append(errs, fieldError(prefix("min_thickness"), "must be non-negative"))
	}
	if zone.Clearance < 0 {
		errs = append(errs, fieldError(prefix("clearance"), "must be non-negative"))
	}
	if zone.HatchPitch < 0 {
		errs = append(errs, fieldError(prefix("hatch_pitch"), "must be non-negative"))
	}
	if zone.Fill.ThermalGap < 0 {
		errs = append(errs, fieldError(prefix("fill.thermal_gap"), "must be non-negative"))
	}
	if zone.Fill.ThermalBridgeWidth < 0 {
		errs = append(errs, fieldError(prefix("fill.thermal_bridge_width"), "must be non-negative"))
	}
	if zone.Fill.IslandRemovalMode < zoneIslandRemovalAlways || zone.Fill.IslandRemovalMode > zoneIslandRemovalArea {
		errs = append(errs, fieldError(prefix("fill.island_removal_mode"), "must be 0, 1, or 2"))
	}
	if zone.Fill.IslandAreaMin < 0 {
		errs = append(errs, fieldError(prefix("fill.island_area_min"), "must be non-negative"))
	}
	return errs
}

func validateZoneKeepout(prefix func(string) string, keepout ZoneKeepout) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	for _, permission := range []struct {
		field string
		value string
	}{
		{field: "tracks", value: keepout.Tracks},
		{field: "vias", value: keepout.Vias},
		{field: "pads", value: keepout.Pads},
		{field: "copperpour", value: keepout.CopperPour},
		{field: "footprints", value: keepout.Footprints},
	} {
		if strings.TrimSpace(permission.value) != permission.value {
			errs = append(errs, fieldError(prefix("keepout."+permission.field), "trimmed value required"))
			continue
		}
		if !isValidKeepoutPermission(defaultString(permission.value, "allowed")) {
			errs = append(errs, fieldError(prefix("keepout."+permission.field), "invalid"))
		}
	}
	return errs
}

func isValidKeepoutPermission(value string) bool {
	switch value {
	case "allowed", "not_allowed":
		return true
	default:
		return false
	}
}

func isValidZoneConnectMode(value string) bool {
	switch value {
	case "yes", "no", "thru_hole_only":
		return true
	default:
		return false
	}
}

func validateDimension(index int, dimension Dimension) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := func(field string) string { return indexed("dimensions", index, field) }
	if !dimension.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if strings.TrimSpace(dimension.Type) != dimension.Type {
		errs = append(errs, fieldError(prefix("type"), "trimmed value required"))
	} else if !isValidDimensionType(dimension.Type) {
		errs = append(errs, fieldError(prefix("type"), "invalid"))
	}
	if !kicadfiles.IsValidBoardLayer(dimension.Layer) {
		errs = append(errs, fieldError(prefix("layer"), "invalid"))
	}
	if countDistinctPoints(dimension.Points) < 2 {
		errs = append(errs, fieldError(prefix("points"), "at least two distinct points required"))
	}
	if dimension.Height == 0 {
		errs = append(errs, fieldError(prefix("height"), "must be non-zero"))
	}
	if strings.TrimSpace(dimension.Text) == "" {
		errs = append(errs, fieldError(prefix("text"), "required"))
	}
	return errs
}

func isValidDimensionType(value string) bool {
	switch value {
	case "aligned", "orthogonal", "radial", "leader":
		return true
	default:
		return false
	}
}

func countShapes(drawing Drawing) int {
	count := 0
	if drawing.Line != nil {
		count++
	}
	if drawing.Rect != nil {
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
	if drawing.Curve != nil {
		count++
	}
	if drawing.Text != nil {
		count++
	}
	return count
}

func drawingKind(drawing Drawing) string {
	switch {
	case drawing.Line != nil:
		return "line"
	case drawing.Rect != nil:
		return "rect"
	case drawing.Circle != nil:
		return "circle"
	case drawing.Arc != nil:
		return "arc"
	case drawing.Poly != nil:
		return "poly"
	case drawing.Curve != nil:
		return "curve"
	case drawing.Text != nil:
		return "text"
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
	if strings.TrimSpace(pad.Type) != "" {
		return pad.Type
	}
	if pad.Drill > 0 {
		return "thru_hole"
	}
	return "smd"
}

func isValidPadType(value string) bool {
	switch value {
	case "smd", "thru_hole", "np_thru_hole", "connect":
		return true
	default:
		return false
	}
}

func isValidPadShape(value string) bool {
	switch value {
	case "rect", "circle", "oval", "trapezoid", "roundrect", "custom":
		return true
	default:
		return false
	}
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

func validSMDPadLayers(layers []kicadfiles.BoardLayer) bool {
	if hasPadLayerSet(layers, kicadfiles.LayerAllCu) || hasPadLayerSet(layers, kicadfiles.LayerAllMask) {
		return false
	}
	hasFrontCopper := hasPadLayerSet(layers, kicadfiles.LayerFCu)
	hasBackCopper := hasPadLayerSet(layers, kicadfiles.LayerBCu)
	if hasFrontCopper == hasBackCopper {
		return false
	}
	if countDistinctCopperLayers(layers) != 1 {
		return false
	}
	if hasFrontCopper {
		return !hasPadLayerSet(layers, kicadfiles.LayerBMask) && !hasPadLayerSet(layers, kicadfiles.LayerBPaste)
	}
	return !hasPadLayerSet(layers, kicadfiles.LayerFMask) && !hasPadLayerSet(layers, kicadfiles.LayerFPaste)
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
