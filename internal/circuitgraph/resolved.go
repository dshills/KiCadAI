package circuitgraph

import (
	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

const (
	CodeComponentUnresolved reports.Code = "GRAPH_COMPONENT_UNRESOLVED"
	CodeComponentAmbiguous  reports.Code = "GRAPH_COMPONENT_AMBIGUOUS"
	CodeSymbolMismatch      reports.Code = "GRAPH_SYMBOL_MISMATCH"
	CodeFootprintMismatch   reports.Code = "GRAPH_FOOTPRINT_MISMATCH"
	CodePinUnresolved       reports.Code = "GRAPH_PIN_UNRESOLVED"
	CodePadUnresolved       reports.Code = "GRAPH_PAD_UNRESOLVED"
	CodePinmapConflict      reports.Code = "GRAPH_PINMAP_CONFLICT"
	CodeRequiredPinOpen     reports.Code = "GRAPH_REQUIRED_PIN_UNCONNECTED"
	CodeSchematicLowering   reports.Code = "GRAPH_SCHEMATIC_LOWERING_INVALID"
	CodeSimulationInvalid   reports.Code = "GRAPH_SIMULATION_INVALID"
)

type ResolvedDocument struct {
	Schema         string              `json:"schema"`
	Version        int                 `json:"version"`
	Source         Document            `json:"source"`
	Components     []ResolvedComponent `json:"components"`
	Nets           []ResolvedNet       `json:"nets"`
	NoConnects     []ResolvedEndpoint  `json:"no_connects"`
	Synthesis      *SynthesisReport    `json:"synthesis,omitempty"`
	Simulation     *simmodel.Plan      `json:"simulation,omitempty"`
	CatalogID      string              `json:"catalog_id"`
	CatalogHash    string              `json:"catalog_hash"`
	LibraryHash    string              `json:"library_hash,omitempty"`
	ResolutionHash string              `json:"resolution_hash"`
}

type ResolvedComponent struct {
	Instance         Component                  `json:"instance"`
	ComponentID      string                     `json:"component_id"`
	VariantID        string                     `json:"variant_id"`
	Family           string                     `json:"family"`
	Manufacturer     string                     `json:"manufacturer,omitempty"`
	MPN              string                     `json:"mpn,omitempty"`
	Confidence       components.ConfidenceLevel `json:"confidence"`
	SymbolID         string                     `json:"symbol_id"`
	FootprintID      string                     `json:"footprint_id"`
	PinMapID         string                     `json:"pinmap_id,omitempty"`
	Functions        []ResolvedFunction         `json:"functions"`
	Units            []ResolvedUnit             `json:"units,omitempty"`
	CatalogSources   []string                   `json:"catalog_sources,omitempty"`
	SymbolSources    []string                   `json:"symbol_sources,omitempty"`
	FootprintSources []string                   `json:"footprint_sources,omitempty"`
	Warnings         []reports.Issue            `json:"warnings,omitempty"`
	Record           components.ComponentRecord `json:"-"`
	Variant          components.PackageVariant  `json:"-"`
	Symbols          []components.SymbolBinding `json:"-"`
}

type ResolvedUnit struct {
	ID       string                    `json:"id"`
	Role     string                    `json:"role"`
	Type     components.SymbolUnitType `json:"type"`
	Required bool                      `json:"required"`
	Unit     int                       `json:"unit"`
	SymbolID string                    `json:"symbol_id"`
}

type ResolvedFunction struct {
	Function   string   `json:"function"`
	Aliases    []string `json:"aliases,omitempty"`
	SymbolID   string   `json:"symbol_id"`
	Unit       int      `json:"unit"`
	UnitID     string   `json:"unit_id,omitempty"`
	SymbolPin  string   `json:"symbol_pin"`
	Pad        string   `json:"pad"`
	Electrical string   `json:"electrical,omitempty"`
	Polarity   string   `json:"polarity,omitempty"`
	Required   bool     `json:"required,omitempty"`
}

type ResolvedNet struct {
	Intent    Net                `json:"intent"`
	Endpoints []ResolvedEndpoint `json:"endpoints"`
}

type ResolvedEndpoint struct {
	Intent   Endpoint          `json:"intent"`
	Function string            `json:"function"`
	Bindings []ResolvedBinding `json:"bindings"`
}

type ResolvedBinding struct {
	SymbolID   string `json:"symbol_id"`
	Unit       int    `json:"unit"`
	UnitID     string `json:"unit_id,omitempty"`
	SymbolPin  string `json:"symbol_pin"`
	Pad        string `json:"pad"`
	Electrical string `json:"electrical,omitempty"`
	Polarity   string `json:"polarity,omitempty"`
}

type ResolveOptions struct {
	Catalog                *components.Catalog
	CatalogID              string
	CatalogHash            string
	LibrarySymbols         map[string]LibrarySymbolEvidence
	LibraryFootprints      map[string]LibraryFootprintEvidence
	RequireLibraryEvidence bool
}

type LibrarySymbolEvidence struct {
	LibraryID string
	Pins      map[string]struct{}
	Source    string
}

type LibraryFootprintEvidence struct {
	LibraryID string
	Pads      map[string]struct{}
	Source    string
}
