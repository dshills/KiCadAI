package blocks

import (
	"fmt"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func instantiateConnectorBreakout(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	pinNames := stringListParam(params, "pin_names")
	if len(pinNames) == 0 {
		issues = append(issues, blockIssue("params.pin_names", "pin_names must not be empty"))
	}
	seen := map[string]struct{}{}
	for _, pinName := range pinNames {
		if pinName == "" {
			issues = append(issues, blockIssue("params.pin_names", "pin_names must not contain empty names"))
			continue
		}
		if _, exists := seen[pinName]; exists {
			issues = append(issues, blockIssue("params.pin_names", "duplicate pin name "+pinName))
		}
		seen[pinName] = struct{}{}
	}
	if count, ok := numericValue(params["pin_count"]); ok && int(count) != len(pinNames) {
		issues = append(issues, blockIssue("params.pin_count", "pin_count must match pin_names length"))
	}
	_, connectorSymbolProvided := request.Params["connector_symbol"]
	connectorSymbol := connectorSymbolParam(params, len(pinNames), connectorSymbolProvided)
	if connectorSymbol == "" {
		issues = append(issues, blockIssue("params.connector_symbol", "connector_symbol is required"))
	}
	_, connectorFootprintProvided := request.Params["connector_footprint"]
	connectorFootprint := connectorFootprintParam(params, len(pinNames), connectorFootprintProvided)
	if connectorFootprint == "" {
		issues = append(issues, blockIssue("params.connector_footprint", "connector_footprint is required"))
	}
	if includeMountingHoles, _ := params["include_mounting_holes"].(bool); includeMountingHoles {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeUnsupportedOperation,
			Severity: reports.SeverityBlocked,
			Path:     "params.include_mounting_holes",
			Message:  "connector mounting holes are not implemented in schematic-only block instantiation",
		})
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	params["connector_symbol"] = connectorSymbol
	params["connector_footprint"] = connectorFootprint
	if _, ok := params["pin_count"]; !ok {
		params["pin_count"] = len(pinNames)
	}

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	connectorRef := allocator.Next("J")
	component := BlockComponent{
		Role:        "connector",
		RefPrefix:   "J",
		Value:       fmt.Sprintf("Conn_01x%02d", len(pinNames)),
		SymbolID:    connectorSymbol,
		FootprintID: connectorFootprint,
	}
	operations, componentIssues := ComponentOperations(component, connectorRef, transactions.Point{XMM: 0, YMM: 0})
	issues = append(issues, componentIssues...)
	var ports []BlockPort
	var nets []string
	for index, pinName := range pinNames {
		netName := InstanceNetName(request.InstanceID, pinName)
		ports = append(ports, BlockPort{Name: pinName, Direction: PortPassive, Description: "Connector pin " + fmt.Sprint(index+1)})
		nets = append(nets, netName)
		appendConnectOperation(&operations, &issues, request.InstanceID, pinName, connectorRef, fmt.Sprint(index+1), netName)
	}
	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	output.Instance.Ports = ports
	output.Instance.Refs = []string{connectorRef}
	output.Instance.Nets = nets
	return output
}

func stringListParam(params map[string]any, name string) []string {
	switch values := params[name].(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil
			}
			result = append(result, text)
		}
		return result
	default:
		return nil
	}
}

func connectorSymbolParam(params map[string]any, pinCount int, explicit bool) string {
	symbol := stringParam(params, "connector_symbol")
	if !explicit && pinCount > 0 && symbol == defaultConnectorSymbol && pinCount != 2 {
		return fmt.Sprintf("Connector_Generic:Conn_01x%02d", pinCount)
	}
	return symbol
}

func connectorFootprintParam(params map[string]any, pinCount int, explicit bool) string {
	footprint := stringParam(params, "connector_footprint")
	if !explicit && pinCount > 0 && footprint == defaultConnectorFootprint && pinCount != 2 {
		return fmt.Sprintf("Connector_PinHeader_2.54mm:PinHeader_1x%02d_P2.54mm_Vertical", pinCount)
	}
	return footprint
}
