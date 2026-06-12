package inspect

import (
	"kicadai/internal/manifest"
	"kicadai/internal/reports"
)

type FileSummary struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

type UnsupportedNode struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

type ProjectSummary struct {
	Root             string            `json:"root"`
	Name             string            `json:"name,omitempty"`
	Files            []FileSummary     `json:"files"`
	Schematic        *SchematicSummary `json:"schematic,omitempty"`
	PCB              *PCBSummary       `json:"pcb,omitempty"`
	Unsupported      []UnsupportedNode `json:"unsupported,omitempty"`
	PreservationOnly []UnsupportedNode `json:"preservation_only,omitempty"`
	Manifest         manifest.Status   `json:"manifest"`
	Issues           []reports.Issue   `json:"issues"`
}

type SchematicSummary struct {
	Path             string            `json:"path"`
	FormatVersion    string            `json:"format_version,omitempty"`
	Generator        string            `json:"generator,omitempty"`
	SymbolCount      int               `json:"symbol_count"`
	WireCount        int               `json:"wire_count"`
	LabelCount       int               `json:"label_count"`
	JunctionCount    int               `json:"junction_count"`
	NoConnectCount   int               `json:"no_connect_count"`
	SheetCount       int               `json:"sheet_count"`
	Symbols          []string          `json:"symbols,omitempty"`
	Truncated        bool              `json:"truncated,omitempty"`
	ObjectCounts     map[string]int    `json:"object_counts,omitempty"`
	InspectionDepth  string            `json:"inspection_depth"`
	Unsupported      []UnsupportedNode `json:"unsupported,omitempty"`
	PreservationOnly []UnsupportedNode `json:"preservation_only,omitempty"`
	Issues           []reports.Issue   `json:"issues"`
}

type PCBSummary struct {
	Path             string            `json:"path"`
	FilesScanned     int               `json:"files_scanned"`
	NetCount         int               `json:"net_count"`
	FootprintCount   int               `json:"footprint_count"`
	PadCount         int               `json:"pad_count"`
	TrackCount       int               `json:"track_count"`
	ViaCount         int               `json:"via_count"`
	ZoneCount        int               `json:"zone_count"`
	DrawingCount     int               `json:"drawing_count"`
	DimensionCount   int               `json:"dimension_count"`
	HasBoardOutline  bool              `json:"has_board_outline"`
	Nets             []string          `json:"nets,omitempty"`
	Footprints       []string          `json:"footprints,omitempty"`
	Truncated        bool              `json:"truncated,omitempty"`
	ObjectCounts     map[string]int    `json:"object_counts,omitempty"`
	LayerUsage       map[string]int    `json:"layer_usage,omitempty"`
	Unsupported      []UnsupportedNode `json:"unsupported,omitempty"`
	PreservationOnly []UnsupportedNode `json:"preservation_only,omitempty"`
	Issues           []reports.Issue   `json:"issues"`
}
