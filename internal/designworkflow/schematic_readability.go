package designworkflow

import (
	"encoding/json"
	"fmt"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/schematiclayout"
	"kicadai/internal/transactions"
)

const (
	schematicReadabilityCodeDiagonalWire   = "diagonal_wire"
	schematicReadabilityCodeStageOrder     = "stage_order"
	schematicReadabilityCodePowerPlacement = "power_placement"
	bmp280BreakoutProjectName              = "sensor_bmp280_breakout"
)

func requiresGeneratedSchematicLayout(projectName string) bool {
	return projectName == bmp280BreakoutProjectName
}

func schematicReadabilitySummary(operations []transactions.Operation) map[string]any {
	result, refRoles, decodeErrors := schematicReadabilityLayout(operations)
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

func schematicReadabilityLayout(operations []transactions.Operation) (schematiclayout.Result, map[string]string, int) {
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
	return result, refRoles, decodeErrors
}

func layoutSchematicOperations(operations []transactions.Operation) ([]transactions.Operation, string, error) {
	positions := bmp280SchematicRolePositions()
	seenRoles := make(map[string]struct{}, len(positions))
	normalized := append([]transactions.Operation(nil), operations...)
	for index, operation := range normalized {
		if operation.Op != transactions.OpAddSymbol {
			continue
		}
		var payload transactions.AddSymbolOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			return nil, "", fmt.Errorf("decode add_symbol operation %d: %w", index, err)
		}
		at, ok := positions[payload.Role]
		if !ok {
			return nil, "", fmt.Errorf("BMP280 schematic layout has no position for role %s", payload.Role)
		}
		if _, duplicate := seenRoles[payload.Role]; duplicate {
			return nil, "", fmt.Errorf("BMP280 schematic layout role %s is not unique", payload.Role)
		}
		seenRoles[payload.Role] = struct{}{}
		deltaX := at.XMM - payload.At.XMM
		deltaY := at.YMM - payload.At.YMM
		payload.At = at
		for propertyIndex := range payload.Properties {
			property := &payload.Properties[propertyIndex]
			if property.At == nil {
				continue
			}
			moved := transactions.Point{XMM: property.At.XMM + deltaX, YMM: property.At.YMM + deltaY}
			property.At = &moved
		}
		payload.Properties = bmp280SchematicTextProperties(payload)
		updated, err := workflowOperation(transactions.OpAddSymbol, payload)
		if err != nil {
			return nil, "", fmt.Errorf("encode laid-out add_symbol operation %d: %w", index, err)
		}
		normalized[index] = updated
	}
	if len(seenRoles) != len(positions) {
		return nil, "", fmt.Errorf("BMP280 schematic layout placed %d of %d required roles", len(seenRoles), len(positions))
	}
	return normalized, "A4", nil
}

func bmp280SchematicTextProperties(payload transactions.AddSymbolOperation) []transactions.SymbolProperty {
	referenceOffsetY := -7.62
	valueOffsetY := 7.62
	if payload.Role == "usb_c_receptacle" {
		referenceOffsetY = -20.32
		valueOffsetY = 20.32
	}
	showName := false
	doNotAutoplace := true
	rotation := 0.0
	properties := append([]transactions.SymbolProperty(nil), payload.Properties...)
	desired := []transactions.SymbolProperty{
		transactions.SymbolProperty{
			Name:           "Reference",
			Value:          payload.Ref,
			ShowName:       &showName,
			DoNotAutoplace: &doNotAutoplace,
			At:             &transactions.Point{XMM: payload.At.XMM, YMM: payload.At.YMM + referenceOffsetY},
			Rotation:       &rotation,
		},
		transactions.SymbolProperty{
			Name:           "Value",
			Value:          payload.Value,
			ShowName:       &showName,
			DoNotAutoplace: &doNotAutoplace,
			At:             &transactions.Point{XMM: payload.At.XMM, YMM: payload.At.YMM + valueOffsetY},
			Rotation:       &rotation,
		},
	}
	for _, property := range desired {
		replaced := false
		for index := range properties {
			if strings.EqualFold(strings.TrimSpace(properties[index].Name), property.Name) {
				properties[index] = property
				replaced = true
				break
			}
		}
		if !replaced {
			properties = append(properties, property)
		}
	}
	return properties
}

func bmp280SchematicRolePositions() map[string]transactions.Point {
	return map[string]transactions.Point{
		"usb_c_receptacle":     {XMM: 38.10, YMM: 88.90},
		"cc1_rd":               {XMM: 63.50, YMM: 106.68},
		"cc2_rd":               {XMM: 63.50, YMM: 124.46},
		"input_capacitor":      {XMM: 88.90, YMM: 68.58},
		"regulator":            {XMM: 116.84, YMM: 88.90},
		"output_capacitor":     {XMM: 142.24, YMM: 68.58},
		"decoupling_capacitor": {XMM: 170.18, YMM: 58.42},
		"sensor":               {XMM: 182.88, YMM: 88.90},
		"sda_pullup":           {XMM: 208.28, YMM: 58.42},
		"scl_pullup":           {XMM: 226.06, YMM: 58.42},
		"connector":            {XMM: 254.00, YMM: 88.90},
	}
}

func applySchematicPaper(tx transactions.Transaction, paper string) (transactions.Transaction, error) {
	if paper == "" {
		return tx, nil
	}
	for index, operation := range tx.Operations {
		if operation.Op != transactions.OpCreateProject {
			continue
		}
		var payload transactions.CreateProjectOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			return tx, fmt.Errorf("decode create_project operation: %w", err)
		}
		payload.Paper = paper
		updated, err := workflowOperation(transactions.OpCreateProject, payload)
		if err != nil {
			return tx, fmt.Errorf("encode create_project operation: %w", err)
		}
		tx.Operations[index] = updated
		return tx, nil
	}
	return tx, fmt.Errorf("schematic transaction is missing create_project operation")
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
