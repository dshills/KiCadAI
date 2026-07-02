package pcb

import (
	"sync"

	"kicadai/internal/kicadfiles"
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
	After  string
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
	Raw                string
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
	Raw                string
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
	DrillShape         string
	DrillSize          kicadfiles.Point
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
	Raw            string
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
