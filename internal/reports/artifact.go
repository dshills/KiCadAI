package reports

type ArtifactKind string

const (
	ArtifactKiCadProject          ArtifactKind = "kicad_project"
	ArtifactSchematic             ArtifactKind = "schematic"
	ArtifactPCB                   ArtifactKind = "pcb"
	ArtifactSymbolLibraryTable    ArtifactKind = "symbol_library_table"
	ArtifactFootprintLibraryTable ArtifactKind = "footprint_library_table"
	ArtifactValidationReport      ArtifactKind = "validation_report"
	ArtifactDiagnosticsReport     ArtifactKind = "diagnostics_report"
	ArtifactSimulationReport      ArtifactKind = "simulation_report"
	ArtifactPromotionReport       ArtifactKind = "promotion_report"
	ArtifactRoundTripReport       ArtifactKind = "roundtrip_report"
	ArtifactDRCReport             ArtifactKind = "drc_report"
	ArtifactERCReport             ArtifactKind = "erc_report"
	ArtifactPreview               ArtifactKind = "preview"
	ArtifactBOM                   ArtifactKind = "bom"
	ArtifactCPL                   ArtifactKind = "cpl"
	ArtifactGerber                ArtifactKind = "gerber"
	ArtifactDrill                 ArtifactKind = "drill"
	ArtifactFabricationPackage    ArtifactKind = "fabrication_package"
)

type Artifact struct {
	Kind        ArtifactKind `json:"kind"`
	Path        string       `json:"path"`
	Description string       `json:"description,omitempty"`
}
