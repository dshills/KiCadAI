package designworkflow

import (
	"encoding/json"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/schematiclayout"
	"kicadai/internal/transactions"
)

func schematicReadabilitySummary(operations []transactions.Operation) map[string]any {
	request := schematiclayout.Request{
		Sheet: schematiclayout.Sheet{Width: kicadfiles.MM(297), Height: kicadfiles.MM(210), Margin: kicadfiles.MM(10.16)},
		Rules: schematiclayout.DefaultRules(schematiclayout.ProfileStandard),
	}
	refRoles := map[string]string{}
	decodeErrors := 0
	for index, operation := range operations {
		switch operation.Op {
		case transactions.OpAddSymbol:
			var payload transactions.AddSymbolOperation
			if err := json.Unmarshal(operation.Raw, &payload); err != nil {
				decodeErrors++
				continue
			}
			refRoles[payload.Ref] = payload.Role
			component := schematiclayout.Component{
				Ref:             payload.Ref,
				Value:           payload.Value,
				LibraryID:       payload.LibraryID,
				Role:            payload.Role,
				Position:        pointFromTransaction(payload.At),
				OriginalOrdinal: index,
			}
			for _, pin := range payload.Pins {
				component.Pins = append(component.Pins, schematiclayout.Pin{Number: pin.Number, At: kicadfiles.Point{X: kicadfiles.MM(pin.XMM), Y: kicadfiles.MM(pin.YMM)}})
			}
			request.Components = append(request.Components, component)
		case transactions.OpConnect:
			var payload transactions.ConnectOperation
			if err := json.Unmarshal(operation.Raw, &payload); err != nil {
				decodeErrors++
				continue
			}
			request.Nets = append(request.Nets, schematiclayout.Net{
				Name:            payload.NetName,
				Endpoints:       []schematiclayout.Endpoint{{Ref: payload.From.Ref, Pin: payload.From.Pin}, {Ref: payload.To.Ref, Pin: payload.To.Pin}},
				OriginalOrdinal: index,
			})
		}
	}
	result := schematiclayout.Layout(request)
	ruleProfile := schematiclayout.RuleProfileForLayoutProfile(result.Report.Profile)
	ruleCount := schematiclayout.RuleCountForProfile(ruleProfile)
	repairGuidanceCount := schematicReadabilityRepairGuidanceCount(result.Diagnostics)
	return map[string]any{
		"profile":                         result.Report.Profile,
		"rule_profile":                    ruleProfile,
		"rule_count":                      ruleCount,
		"repair_guidance_available":       ruleCount > 0,
		"repair_guidance_count":           repairGuidanceCount,
		"passed":                          result.Report.Passed,
		"component_count":                 result.Report.ComponentCount,
		"routed_net_count":                result.Report.RoutedNetCount,
		"label_fallback_count":            result.Report.LabelFallbackCount,
		"diagonal_wire_count":             result.Report.DiagonalWireCount,
		"stage_order_violation_count":     result.Report.StageOrderViolationCount,
		"power_placement_violation_count": result.Report.PowerPlacementViolations,
		"diagnostic_count":                result.Report.DiagnosticCount,
		"error_count":                     result.Report.ErrorCount,
		"warning_count":                   result.Report.WarningCount,
		"decode_error_count":              decodeErrors,
		"roles":                           refRoles,
	}
}

func schematicReadabilityRepairGuidanceCount(diagnostics []schematiclayout.Diagnostic) int {
	count := 0
	for _, diagnostic := range diagnostics {
		if diagnostic.Repair != "" {
			count++
		}
	}
	return count
}

func pointFromTransaction(point transactions.Point) kicadfiles.Point {
	return kicadfiles.Point{X: kicadfiles.MM(point.XMM), Y: kicadfiles.MM(point.YMM)}
}
