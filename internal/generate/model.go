package generate

import "kicadai/internal/reports"

type BreakoutRequest struct {
	Kind       string             `json:"kind"`
	Name       string             `json:"name"`
	Board      BoardRequest       `json:"board"`
	Connectors []ConnectorRequest `json:"connectors"`
	GroundZone bool               `json:"ground_zone,omitempty"`
}

type BoardRequest struct {
	WidthMM  float64 `json:"width_mm"`
	HeightMM float64 `json:"height_mm"`
}

type ConnectorRequest struct {
	Ref  string   `json:"ref"`
	Pins []string `json:"pins"`
}

type BreakoutResult struct {
	TransactionOperations int                `json:"transaction_operations"`
	ProjectName           string             `json:"project_name"`
	Board                 BoardRequest       `json:"board"`
	Connectors            []ConnectorRequest `json:"connectors"`
	Artifacts             []reports.Artifact `json:"artifacts"`
	Issues                []reports.Issue    `json:"issues"`
}
