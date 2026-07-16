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
		if component.SchematicOnly {
			continue
		}
		request, ok := SelectionRequestForComponentWithParams(component, acceptance, params)
		if !ok {
			if component.ComponentIDParam != "" && stringParam(params, component.ComponentIDParam) == "" {
				continue
			}
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
	if component.ComponentIDParam != "" {
		if componentID := stringParam(params, component.ComponentIDParam); componentID != "" {
			return components.SelectionRequest{
				Query: components.Query{
					Text:              componentID,
					MinimumConfidence: component.MinimumConfidence,
				},
				Acceptance:        acceptance,
				RequiredRatings:   sensorSupplyRating(component, params),
				RequiredFunctions: []string{"SDA", "SCL"},
				RequireConcrete:   true,
				RequireCompanions: true,
			}, true
		}
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
		queryCopy := *component.ComponentQuery
		defaultPackage := packageQueryFromFootprint(component.FootprintID)
		if queryCopy.MinimumConfidence == "" {
			queryCopy.MinimumConfidence = component.MinimumConfidence
		}
		if queryCopy.Package == "" {
			queryCopy.Package = component.ComponentVariant
		}
		if component.ComponentPackageParam != "" {
			if value := stringParam(params, component.ComponentPackageParam); value != "" {
				queryCopy.Package = packageQueryFromFootprint(value)
			}
		}
		if queryCopy.Value == "" {
			queryCopy.Value = component.Value
		}
		params = paramsWithInferredConnectorPinCount(component, params)
		if component.ComponentValueParam != "" {
			if value := selectionValueParam(params, component.ComponentValueParam); value != "" {
				queryCopy.Value = value
			}
		}
		if component.ComponentVoltageParam != "" {
			if value, ok := parseUnit(params[component.ComponentVoltageParam], "V", voltageMultipliers()); ok {
				queryCopy.MinVoltageV = value
			}
		}
		if queryCopy.ValueKind == "pin_count" && queryCopy.Value != "" && (queryCopy.Package == "" || queryCopy.Package == defaultPackage) {
			if packageQuery := connectorPinCountPackage(queryCopy.Value, component.ComponentPackageTemplate); packageQuery != "" {
				queryCopy.Package = packageQuery
			}
		}
		return components.SelectionRequest{Query: queryCopy, Acceptance: acceptance}, true
	}
	return components.SelectionRequest{}, false
}

func sensorSupplyRating(component BlockComponent, params map[string]any) []components.RequiredRating {
	if component.ComponentIDParam != "sensor_component_id" {
		return nil
	}
	value, ok := parseUnit(params["supply_voltage"], "V", voltageMultipliers())
	if !ok {
		return nil
	}
	return []components.RequiredRating{{Kind: "supply_voltage", Value: strconv.FormatFloat(value, 'f', -1, 64), Unit: "V"}}
}

func paramsWithInferredConnectorPinCount(component BlockComponent, params map[string]any) map[string]any {
	if component.ComponentValueParam != "pin_count" && component.ComponentPinsParam != "pin_count" {
		return params
	}
	if _, ok := params["pin_count"]; ok {
		return params
	}
	pinNames := stringListParam(params, "pin_names")
	if len(pinNames) == 0 {
		return params
	}
	out := make(map[string]any, len(params)+1)
	for key, value := range params {
		out[key] = value
	}
	out["pin_count"] = strconv.Itoa(len(pinNames))
	return out
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
	if component.ComponentPinsParam == "regulator_symbol" && component.ComponentPackageParam != "" {
		symbol := stringParam(params, component.ComponentPinsParam)
		footprint := stringParam(params, component.ComponentPackageParam)
		if profile, ok := regulatorProfileFor(symbol, footprint); ok {
			component.SymbolID = profile.Symbol
			component.FootprintID = profile.Footprint
			component.Pins = profile.Pins
		} else if symbol != "" || footprint != "" {
			component.Pins = nil
		}
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
