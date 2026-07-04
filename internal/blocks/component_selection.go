package blocks

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

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
	params := ApplyParameterDefaults(definition, nil)
	for _, component := range definition.Components {
		if err := ctx.Err(); err != nil {
			report.Issues = append(report.Issues, reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityBlocked, Path: "block." + definition.ID, Message: err.Error()})
			return report
		}
		component = componentWithParamDrivenPins(component, params)
		if !ComponentActiveForParams(component, params) {
			continue
		}
		request, ok := SelectionRequestForComponentWithParams(component, acceptance, params)
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

func SelectionRequestForComponent(component BlockComponent, acceptance components.AcceptanceLevel) (components.SelectionRequest, bool) {
	return SelectionRequestForComponentWithParams(component, acceptance, nil)
}

func SelectionRequestForComponentWithParams(component BlockComponent, acceptance components.AcceptanceLevel, params map[string]any) (components.SelectionRequest, bool) {
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
		defaultPackage := packageQueryFromFootprint(component.FootprintID)
		if query.MinimumConfidence == "" {
			query.MinimumConfidence = component.MinimumConfidence
		}
		if query.Package == "" {
			query.Package = component.ComponentVariant
		}
		if component.ComponentPackageParam != "" {
			if value := stringParam(params, component.ComponentPackageParam); value != "" {
				query.Package = packageQueryFromFootprint(value)
			}
		}
		if query.Value == "" {
			query.Value = component.Value
		}
		if component.ComponentValueParam != "" {
			if value := selectionValueParam(params, component.ComponentValueParam); value != "" {
				query.Value = value
			}
		}
		if query.ValueKind == "pin_count" && query.Value != "" && (query.Package == "" || query.Package == defaultPackage) {
			if packageQuery := connectorPinCountPackage(query.Value, component.ComponentPackageTemplate); packageQuery != "" {
				query.Package = packageQuery
			}
		}
		return components.SelectionRequest{Query: query, Acceptance: acceptance}, true
	}
	return components.SelectionRequest{}, false
}

func selectionValueParam(params map[string]any, name string) string {
	if value := stringParam(params, name); value != "" {
		return value
	}
	if value, ok := numericValue(params[name]); ok {
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	return ""
}

func componentWithParamDrivenPins(component BlockComponent, params map[string]any) BlockComponent {
	if component.ComponentPinsParam == "" {
		return component
	}
	count, ok := numericValue(params[component.ComponentPinsParam])
	if !ok || count <= 0 || math.Trunc(count) != count {
		return component
	}
	countInt := int(count)
	component.Pins = connectorSymbolPins(countInt)
	if symbolID := formattedPinCountTemplate(component.ComponentSymbolTemplate, countInt); symbolID != "" {
		component.SymbolID = symbolID
	}
	return component
}

func connectorPinCountPackage(value string, template string) string {
	template = strings.TrimSpace(template)
	if template == "" {
		return ""
	}
	count, err := strconv.ParseFloat(value, 64)
	if err != nil || count <= 0 || math.Trunc(count) != count {
		return ""
	}
	return formattedPinCountTemplate(template, int(count))
}

func formattedPinCountTemplate(template string, count int) string {
	template = strings.TrimSpace(template)
	if template == "" || count <= 0 {
		return ""
	}
	result := strings.ReplaceAll(template, "%02d", fmt.Sprintf("%02d", count))
	result = strings.ReplaceAll(result, "%d", strconv.Itoa(count))
	return result
}

func ComponentActiveForParams(component BlockComponent, params map[string]any) bool {
	return realizationWhenMatches(component.When, params)
}
