package blocks

import (
	"context"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

type ComponentSelectionReport struct {
	BlockID    string                    `json:"block_id"`
	Selections []BlockComponentSelection `json:"selections"`
	Issues     []reports.Issue           `json:"issues,omitempty"`
}

type BlockComponentSelection struct {
	Role      string               `json:"role"`
	Selection components.Selection `json:"selection"`
}

func SelectDefinitionComponents(ctx context.Context, definition BlockDefinition, catalog *components.Catalog, acceptance components.AcceptanceLevel) ComponentSelectionReport {
	report := ComponentSelectionReport{BlockID: definition.ID, Selections: make([]BlockComponentSelection, 0, len(definition.Components))}
	for _, component := range definition.Components {
		if err := ctx.Err(); err != nil {
			report.Issues = append(report.Issues, reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityBlocked, Path: "block." + definition.ID, Message: err.Error()})
			return report
		}
		request, ok := componentSelectionRequest(component, acceptance)
		if !ok {
			report.Issues = append(report.Issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityWarning,
				Path:     "block." + definition.ID + ".components." + component.Role,
				Message:  "block component has no component_id or component_query",
			})
			continue
		}
		selection, result := components.Select(ctx, catalog, request)
		report.Issues = append(report.Issues, result.Issues...)
		if result.OK {
			report.Selections = append(report.Selections, BlockComponentSelection{Role: component.Role, Selection: selection})
		}
	}
	return report
}

func componentSelectionRequest(component BlockComponent, acceptance components.AcceptanceLevel) (components.SelectionRequest, bool) {
	if acceptance == "" {
		acceptance = component.Acceptance
	}
	if acceptance == "" {
		acceptance = components.AcceptanceDraft
	}
	if component.ComponentID != "" {
		return components.SelectionRequest{
			Query: components.Query{
				Text:              component.ComponentID,
				Package:           component.ComponentVariant,
				MinimumConfidence: component.MinimumConfidence,
			},
			Acceptance: acceptance,
		}, true
	}
	if component.ComponentQuery != nil {
		query := *component.ComponentQuery
		if query.MinimumConfidence == "" {
			query.MinimumConfidence = component.MinimumConfidence
		}
		if query.Package == "" {
			query.Package = component.ComponentVariant
		}
		if query.Value == "" {
			query.Value = component.Value
		}
		return components.SelectionRequest{Query: query, Acceptance: acceptance}, true
	}
	return components.SelectionRequest{}, false
}
