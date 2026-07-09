package designworkflow

import (
	"encoding/json"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/schematiclayout"
	"kicadai/internal/transactions"
)

const (
	schematicReadabilityCodeDiagonalWire   = "diagonal_wire"
	schematicReadabilityCodeStageOrder     = "stage_order"
	schematicReadabilityCodePowerPlacement = "power_placement"
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
		case transactions.OpCreateProject:
			var payload transactions.CreateProjectOperation
			if err := json.Unmarshal(operation.Raw, &payload); err != nil {
				decodeErrors++
				continue
			}
			request.Sheet = schematiclayout.SheetForPaper(payload.Paper)
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
	diagnostics := result.Diagnostics
	if schematicReadabilityHasAmplifierRoles(refRoles) {
		ruleProfile = schematiclayout.RuleProfileAmplifier
		diagnostics = schematiclayout.AmplifierLayoutDiagnostics(result)
	}
	ruleCount := schematiclayout.RuleCountForProfile(ruleProfile)
	counts := schematicReadabilityDiagnosticCounts(diagnostics)
	return map[string]any{
		"profile":                         result.Report.Profile,
		"selected_paper":                  result.Report.SelectedPaper,
		"page_escalation_count":           result.Report.PageEscalationCount,
		"rule_profile":                    ruleProfile,
		"rule_count":                      ruleCount,
		"repair_guidance_available":       ruleCount > 0,
		"repair_guidance_count":           counts.repairGuidance,
		"passed":                          counts.errors == 0,
		"component_count":                 result.Report.ComponentCount,
		"routed_net_count":                result.Report.RoutedNetCount,
		"label_fallback_count":            result.Report.LabelFallbackCount,
		"diagonal_wire_count":             counts.diagonalWires,
		"stage_order_violation_count":     counts.stageOrderViolations,
		"power_placement_violation_count": counts.powerPlacementViolations,
		"diagnostic_count":                len(diagnostics),
		"error_count":                     counts.errors,
		"warning_count":                   counts.warnings,
		"decode_error_count":              decodeErrors,
		"roles":                           refRoles,
	}
}

func schematicReadabilityHasAmplifierRoles(refRoles map[string]string) bool {
	for _, role := range refRoles {
		switch role {
		case "opamp", "input_coupling", "input_stopper", "bias_top", "bias_bottom", "upper_bias_feed", "bias_upper", "bias_lower", "lower_bias_feed", "amp_out_anchor", "upper_output", "lower_output", "upper_emitter_resistor", "lower_emitter_resistor", "dc_blocking_capacitor", "headphone_connector":
			return true
		}
	}
	return false
}

type schematicReadabilityCounts struct {
	repairGuidance           int
	errors                   int
	warnings                 int
	diagonalWires            int
	stageOrderViolations     int
	powerPlacementViolations int
}

func schematicReadabilityDiagnosticCounts(diagnostics []schematiclayout.Diagnostic) schematicReadabilityCounts {
	var counts schematicReadabilityCounts
	for _, diagnostic := range diagnostics {
		if diagnostic.Repair != "" {
			counts.repairGuidance++
		}
		switch diagnostic.Severity {
		case schematiclayout.SeverityError:
			counts.errors++
		case schematiclayout.SeverityWarning:
			counts.warnings++
		}
		switch diagnostic.Code {
		case schematicReadabilityCodeDiagonalWire:
			counts.diagonalWires++
		case schematicReadabilityCodeStageOrder:
			counts.stageOrderViolations++
		case schematicReadabilityCodePowerPlacement:
			counts.powerPlacementViolations++
		}
	}
	return counts
}

func pointFromTransaction(point transactions.Point) kicadfiles.Point {
	return kicadfiles.Point{X: kicadfiles.MM(point.XMM), Y: kicadfiles.MM(point.YMM)}
}
