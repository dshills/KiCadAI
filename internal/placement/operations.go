package placement

import (
	"encoding/json"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func PlacementOperation(component Component, placement PlacementResult) (transactions.Operation, error) {
	pads := padSpecs(component.Pads)
	payload := transactions.PlaceFootprintOperation{
		Op:                            transactions.OpPlaceFootprint,
		Ref:                           firstNonEmpty(component.Ref, placement.Ref),
		FootprintID:                   firstNonEmpty(placement.FootprintID, component.FootprintID),
		Value:                         component.Value,
		At:                            transactions.Point{XMM: placement.Position.XMM, YMM: placement.Position.YMM},
		Rotation:                      placement.Position.RotationDeg,
		Layer:                         placement.Position.Layer,
		Pads:                          pads,
		AllowUnmatchedUnconnectedPads: component.AllowUnmatchedUnconnectedPads,
		HideDefaultFootprintText:      true,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return transactions.Operation{}, err
	}
	return transactions.NewOperationWithMetadata(transactions.OpPlaceFootprint, raw, payload.Ref, ""), nil
}

func PlacementOperations(request Request, placements []PlacementResult) ([]transactions.Operation, []reports.Issue) {
	components := map[string]Component{}
	var issues []reports.Issue
	for _, component := range request.Components {
		key := strings.ToUpper(strings.TrimSpace(component.Ref))
		if _, exists := components[key]; exists {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeDuplicateReference,
				Severity: reports.SeverityError,
				Path:     "components." + component.Ref,
				Message:  "duplicate component reference " + component.Ref,
			})
			continue
		}
		components[key] = component
	}
	operations := make([]transactions.Operation, 0, len(placements))
	processed := map[string]struct{}{}
	for _, placement := range placements {
		if placement.Reason != "" {
			continue
		}
		placementKey := strings.ToUpper(strings.TrimSpace(placement.Ref))
		if _, exists := processed[placementKey]; exists {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeDuplicateReference,
				Severity: reports.SeverityError,
				Path:     "placements." + placement.Ref,
				Message:  "duplicate placement reference " + placement.Ref,
			})
			continue
		}
		processed[placementKey] = struct{}{}
		component, ok := components[placementKey]
		if !ok {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityError,
				Path:     "placements." + placement.Ref,
				Message:  "placement component not found in request",
			})
			continue
		}
		if firstNonEmpty(placement.FootprintID, component.FootprintID) == "" {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityError,
				Path:     "placements." + placement.Ref,
				Message:  "placement component missing footprint ID",
			})
			continue
		}
		operation, err := PlacementOperation(component, placement)
		if err != nil {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityError,
				Path:     "placements." + placement.Ref,
				Message:  err.Error(),
			})
			continue
		}
		operations = append(operations, operation)
	}
	return operations, issues
}

func padSpecs(pads []PadSummary) []transactions.PadSpec {
	if len(pads) == 0 {
		return nil
	}
	specs := make([]transactions.PadSpec, 0, len(pads))
	for _, pad := range pads {
		var net *string
		if value := strings.TrimSpace(pad.Net); value != "" {
			net = &value
		}
		specs = append(specs, transactions.PadSpec{
			Name:     pad.Name,
			XMM:      pad.XMM,
			YMM:      pad.YMM,
			WidthMM:  pad.WidthMM,
			HeightMM: pad.HeightMM,
			Net:      net,
		})
	}
	return specs
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
