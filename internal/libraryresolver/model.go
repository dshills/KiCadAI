package libraryresolver

import (
	"time"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/reports"
)

const (
	EnvKLCRoot        = "KICADAI_KLC_ROOT"
	EnvSymbolsRoot    = "KICADAI_SYMBOLS_ROOT"
	EnvFootprintsRoot = "KICADAI_FOOTPRINTS_ROOT"
	EnvTemplatesRoot  = "KICADAI_TEMPLATES_ROOT"
	EnvLibraryCache   = "KICADAI_LIBRARY_CACHE"
)

type LibraryRoots struct {
	KLCRoot        string `json:"klc_root,omitempty"`
	SymbolsRoot    string `json:"symbols_root,omitempty"`
	FootprintsRoot string `json:"footprints_root,omitempty"`
	TemplatesRoot  string `json:"templates_root,omitempty"`
}

type LibraryIndex struct {
	GeneratedAt time.Time                  `json:"generated_at"`
	Roots       LibraryRoots               `json:"roots"`
	Inventory   LibraryInventory           `json:"inventory"`
	Symbols     map[string]SymbolRecord    `json:"symbols"`
	Footprints  map[string]FootprintRecord `json:"footprints"`
	Diagnostics []reports.Issue            `json:"diagnostics"`
}

type LibraryInventory struct {
	SymbolFiles           []LibraryFile   `json:"symbol_files"`
	FootprintFiles        []LibraryFile   `json:"footprint_files"`
	SymbolLibraryCount    int             `json:"symbol_library_count"`
	FootprintLibraryCount int             `json:"footprint_library_count"`
	Diagnostics           []reports.Issue `json:"diagnostics"`
}

type LibraryFileKind string

const (
	LibraryFileSymbol    LibraryFileKind = "symbol"
	LibraryFileFootprint LibraryFileKind = "footprint"
)

type LibraryFile struct {
	Kind            LibraryFileKind `json:"kind"`
	Path            string          `json:"path"`
	LibraryNickname string          `json:"library_nickname"`
	Name            string          `json:"name,omitempty"`
	IDPrefix        string          `json:"id_prefix"`
}

type SymbolRecord struct {
	LibraryID       string            `json:"library_id"`
	LibraryNickname string            `json:"library_nickname"`
	Name            string            `json:"name"`
	Path            string            `json:"path"`
	Description     string            `json:"description,omitempty"`
	Keywords        []string          `json:"keywords,omitempty"`
	Datasheet       string            `json:"datasheet,omitempty"`
	FootprintFilter []string          `json:"footprint_filter,omitempty"`
	Properties      map[string]string `json:"properties,omitempty"`
	Units           []SymbolUnit      `json:"units,omitempty"`
	Pins            []SymbolPin       `json:"pins,omitempty"`
	Raw             string            `json:"raw,omitempty"`
}

type SymbolUnit struct {
	Unit      int `json:"unit"`
	BodyStyle int `json:"body_style"`
}

type SymbolPin struct {
	Number       string           `json:"number"`
	Name         string           `json:"name,omitempty"`
	Electrical   string           `json:"electrical,omitempty"`
	Unit         int              `json:"unit"`
	BodyStyle    int              `json:"body_style"`
	Position     kicadfiles.Point `json:"position"`
	Orientation  string           `json:"orientation,omitempty"`
	Length       kicadfiles.IU    `json:"length,omitempty"`
	Hidden       bool             `json:"hidden"`
	FunctionHint string           `json:"function_hint,omitempty"`
}

type FootprintRecord struct {
	FootprintID     string            `json:"footprint_id"`
	LibraryNickname string            `json:"library_nickname"`
	Name            string            `json:"name"`
	Path            string            `json:"path"`
	Description     string            `json:"description,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	Attributes      []string          `json:"attributes,omitempty"`
	Properties      map[string]string `json:"properties,omitempty"`
	Pads            []FootprintPad    `json:"pads,omitempty"`
	Texts           []FootprintText   `json:"texts,omitempty"`
	GraphicsSummary GraphicsSummary   `json:"graphics_summary"`
	Models          []string          `json:"models,omitempty"`
	BoundingBox     BoundingBox       `json:"bounding_box"`
	Raw             string            `json:"raw,omitempty"`
}

type FootprintPad struct {
	Name        string                  `json:"name"`
	Type        string                  `json:"type,omitempty"`
	Shape       string                  `json:"shape,omitempty"`
	Position    kicadfiles.Point        `json:"position"`
	Rotation    float64                 `json:"rotation,omitempty"`
	Size        kicadfiles.Point        `json:"size"`
	Drill       kicadfiles.IU           `json:"drill,omitempty"`
	Layers      []kicadfiles.BoardLayer `json:"layers,omitempty"`
	PinFunction string                  `json:"pin_function,omitempty"`
	PinType     string                  `json:"pin_type,omitempty"`
	RoundRectR  float64                 `json:"round_rect_r,omitempty"`
}

type FootprintText struct {
	Kind     string           `json:"kind,omitempty"`
	Text     string           `json:"text"`
	Position kicadfiles.Point `json:"position"`
	Layer    string           `json:"layer,omitempty"`
}

type GraphicsSummary struct {
	LineCount     int  `json:"line_count,omitempty"`
	ArcCount      int  `json:"arc_count,omitempty"`
	CircleCount   int  `json:"circle_count,omitempty"`
	PolygonCount  int  `json:"polygon_count,omitempty"`
	TextCount     int  `json:"text_count,omitempty"`
	HasCourtyard  bool `json:"has_courtyard"`
	HasFabOutline bool `json:"has_fab_outline"`
	HasSilk       bool `json:"has_silk"`
}

type BoundingBox struct {
	Min kicadfiles.Point `json:"min"`
	Max kicadfiles.Point `json:"max"`
}

type LoadOptions struct {
	CachePath string `json:"cache_path,omitempty"`
	Refresh   bool   `json:"refresh,omitempty"`
}

type Query struct {
	Text  string `json:"text,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type MatchOptions struct {
	Limit              int      `json:"limit,omitempty"`
	RequiredAttributes []string `json:"required_attributes,omitempty"`
}

type CompatibilityStatus string

const (
	CompatibilityCompatible        CompatibilityStatus = "compatible"
	CompatibilityCandidate         CompatibilityStatus = "candidate"
	CompatibilityNeedsVerification CompatibilityStatus = "needs_verification"
	CompatibilityIncompatible      CompatibilityStatus = "incompatible"
	CompatibilityUnknown           CompatibilityStatus = "unknown"
)

type CompatibilityResult struct {
	SymbolID        string                  `json:"symbol_id"`
	FootprintID     string                  `json:"footprint_id"`
	Status          CompatibilityStatus     `json:"status"`
	Score           float64                 `json:"score,omitempty"`
	PinmapCandidate []PinmapCandidate       `json:"pinmap_candidate,omitempty"`
	Issues          []reports.Issue         `json:"issues"`
	Evidence        []CompatibilityEvidence `json:"evidence,omitempty"`
}

type PinmapCandidate struct {
	SymbolPin    string  `json:"symbol_pin"`
	SymbolName   string  `json:"symbol_name,omitempty"`
	Function     string  `json:"function,omitempty"`
	FootprintPad string  `json:"footprint_pad"`
	Confidence   float64 `json:"confidence"`
	Reason       string  `json:"reason"`
}

type CompatibilityEvidence struct {
	Kind    string  `json:"kind"`
	Message string  `json:"message"`
	Score   float64 `json:"score,omitempty"`
}
