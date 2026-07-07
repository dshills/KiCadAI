package designworkflow

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
	"kicadai/internal/schematicrules"
	"kicadai/internal/transactions"
)

func SchematicElectricalStage(plan BlockPlanResult) StageResult {
	if plan.Stage.Status == StageStatusBlocked || reports.HasBlockingIssue(plan.Stage.Issues) {
		return StageResult{Name: StageSchematicElectrical, Status: StageStatusSkipped, Summary: map[string]any{"reason": "block planning did not complete"}}
	}
	projectName := plan.Composition.ProjectName
	if projectName == "" {
		projectName = plan.Request.Name
	}
	tx, err := blocks.ProjectTransactionForCompositionOutput(projectName, plan.Output, false)
	if err != nil {
		stage := NewStageResult(StageSchematicElectrical, []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "schematic_electrical.transaction",
			Message:  err.Error(),
		}})
		stage.Summary = map[string]any{"status": string(schematicrules.StatusBlocked)}
		return stage
	}
	file, opts, issues := schematicElectricalInputsFromTransaction(tx)
	if len(issues) != 0 {
		stage := NewStageResult(StageSchematicElectrical, issues)
		stage.Summary = map[string]any{"status": string(schematicrules.StatusBlocked)}
		return stage
	}
	report := schematicrules.Inspect(file, opts)
	stage := NewStageResult(StageSchematicElectrical, schematicElectricalIssues(report))
	stage.Summary = map[string]any{
		"status":                          string(report.Status),
		"finding_count":                   report.FindingCount,
		"checked_symbols":                 report.CheckedSymbols,
		"checked_nets":                    report.CheckedNets,
		"checked_required_pins":           report.CheckedRequiredPins,
		"checked_decoupling_requirements": report.CheckedDecouplingRequirements,
		"checked_value_checks":            report.CheckedValueChecks,
		"checked_rating_checks":           report.CheckedRatingChecks,
	}
	return stage
}

func schematicElectricalInputsFromTransaction(tx transactions.Transaction) (schematic.SchematicFile, schematicrules.Options, []reports.Issue) {
	var file schematic.SchematicFile
	opts := schematicrules.Options{Scope: schematicrules.ScopeGenerated, Acceptance: schematicrules.AcceptanceConnectivity}
	symbolPins := map[string]map[string]kicadfiles.Point{}
	labels := map[string]struct{}{}
	var issues []reports.Issue
	for index, operation := range tx.Operations {
		switch operation.Op {
		case transactions.OpAddSymbol:
			var payload transactions.AddSymbolOperation
			if err := decodeSchematicElectricalOperation(operation, &payload); err != nil {
				return file, opts, []reports.Issue{schematicElectricalDecodeIssue(index, err)}
			}
			position := schematicElectricalPoint(payload.At)
			symbol := schematic.SchematicSymbol{
				LibraryID:  payload.LibraryID,
				Reference:  payload.Ref,
				Value:      payload.Value,
				Position:   position,
				PinAnchors: schematicElectricalPinAnchors(position, payload.Pins),
			}
			symbolPins[payload.Ref] = schematicElectricalPinMap(position, payload.Pins)
			file.Symbols = append(file.Symbols, symbol)
			if strings.TrimSpace(payload.Value) != "" || !strings.HasPrefix(strings.TrimSpace(payload.Ref), "#") {
				opts.ValueChecks = append(opts.ValueChecks, schematicrules.ValueCheck{
					Reference: payload.Ref,
					Value:     payload.Value,
					Required:  !strings.HasPrefix(strings.TrimSpace(payload.Ref), "#"),
					ParseOK:   strings.TrimSpace(payload.Value) != "",
				})
			}
		case transactions.OpConnect:
			var payload transactions.ConnectOperation
			if err := decodeSchematicElectricalOperation(operation, &payload); err != nil {
				return file, opts, []reports.Issue{schematicElectricalDecodeIssue(index, err)}
			}
			from, fromOK := schematicElectricalEndpointPoint(symbolPins, payload.From)
			to, toOK := schematicElectricalEndpointPoint(symbolPins, payload.To)
			if !fromOK {
				issues = append(issues, schematicElectricalEndpointIssue(index, "from", payload.From))
			}
			if !toOK {
				issues = append(issues, schematicElectricalEndpointIssue(index, "to", payload.To))
			}
			if !fromOK || !toOK {
				continue
			}
			file.Wires = append(file.Wires, schematic.Wire{Points: []kicadfiles.Point{from, to}})
			if strings.TrimSpace(payload.NetName) != "" {
				labelKey := schematicElectricalLabelKey(payload.NetName, from)
				if _, exists := labels[labelKey]; !exists {
					file.Labels = append(file.Labels, schematic.Label{Text: payload.NetName, Kind: schematic.LabelLocal, Position: from})
					labels[labelKey] = struct{}{}
				}
			}
			opts.PinIntents = append(opts.PinIntents,
				schematicrules.PinIntent{Reference: payload.From.Ref, Pin: payload.From.Pin, Net: payload.NetName, Position: schematicElectricalIntentPointIU(from), Kind: schematicrules.PinIntentRequired},
				schematicrules.PinIntent{Reference: payload.To.Ref, Pin: payload.To.Pin, Net: payload.NetName, Position: schematicElectricalIntentPointIU(to), Kind: schematicrules.PinIntentRequired},
			)
		case transactions.OpAddNoConnect:
			var payload transactions.AddNoConnectOperation
			if err := decodeSchematicElectricalOperation(operation, &payload); err != nil {
				return file, opts, []reports.Issue{schematicElectricalDecodeIssue(index, err)}
			}
			point, ok := schematicElectricalEndpointPoint(symbolPins, payload.Endpoint)
			if !ok {
				issues = append(issues, schematicElectricalEndpointIssue(index, "endpoint", payload.Endpoint))
				continue
			}
			file.NoConnects = append(file.NoConnects, schematic.NoConnect{Position: point})
			opts.PinIntents = append(opts.PinIntents, schematicrules.PinIntent{Reference: payload.Endpoint.Ref, Pin: payload.Endpoint.Pin, Position: schematicElectricalIntentPointIU(point), Kind: schematicrules.PinIntentNoConnect})
		case transactions.OpAddLabel:
			var payload transactions.AddLabelOperation
			if err := decodeSchematicElectricalOperation(operation, &payload); err != nil {
				return file, opts, []reports.Issue{schematicElectricalDecodeIssue(index, err)}
			}
			file.Labels = append(file.Labels, schematic.Label{Text: payload.Text, Kind: schematicElectricalLabelKind(payload.Kind), Position: schematicElectricalPoint(payload.At)})
		}
	}
	if len(issues) != 0 {
		return file, opts, issues
	}
	return file, opts, nil
}

func schematicElectricalLabelKind(value string) schematic.LabelKind {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "global":
		return schematic.LabelGlobal
	case "hierarchical":
		return schematic.LabelHierarchical
	default:
		return schematic.LabelLocal
	}
}

func decodeSchematicElectricalOperation(operation transactions.Operation, target any) error {
	data := operation.Raw
	if len(data) == 0 {
		return errSchematicElectricalMissingRaw
	}
	return json.Unmarshal(data, target)
}

var errSchematicElectricalMissingRaw = errors.New("operation raw payload is required")

func schematicElectricalDecodeIssue(index int, err error) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     "operations[" + strconv.Itoa(index) + "]",
		Message:  "decode schematic electrical operation: " + err.Error(),
	}
}

func schematicElectricalEndpointIssue(index int, field string, endpoint transactions.Endpoint) reports.Issue {
	message := "schematic electrical endpoint " + strings.TrimSpace(endpoint.Ref) + "." + strings.TrimSpace(endpoint.Pin) + " could not be resolved"
	if strings.TrimSpace(endpoint.Ref) == "" {
		message = "schematic electrical endpoint reference is empty"
	}
	return reports.Issue{
		Code:       reports.CodeValidationFailed,
		Severity:   reports.SeverityError,
		Path:       "operations[" + strconv.Itoa(index) + "]." + field,
		Message:    message,
		Refs:       schematicElectricalEndpointRefs(endpoint),
		Suggestion: "ensure connect and no-connect operations reference generated symbols and valid symbol pins",
	}
}

func schematicElectricalPoint(point transactions.Point) kicadfiles.Point {
	return kicadfiles.Point{X: kicadfiles.MM(point.XMM), Y: kicadfiles.MM(point.YMM)}
}

func schematicElectricalIntentPointIU(point kicadfiles.Point) schematicrules.Point {
	return schematicrules.Point{X: int64(point.X), Y: int64(point.Y)}
}

func schematicElectricalPinAnchors(position kicadfiles.Point, pins []transactions.PinSpec) []kicadfiles.Point {
	if len(pins) == 0 {
		return []kicadfiles.Point{position}
	}
	anchors := make([]kicadfiles.Point, 0, len(pins))
	for _, pin := range pins {
		anchors = append(anchors, kicadfiles.Point{X: position.X + kicadfiles.MM(pin.XMM), Y: position.Y + kicadfiles.MM(pin.YMM)})
	}
	return anchors
}

func schematicElectricalPinMap(position kicadfiles.Point, pins []transactions.PinSpec) map[string]kicadfiles.Point {
	anchors := map[string]kicadfiles.Point{}
	if len(pins) == 0 {
		anchors[""] = position
		return anchors
	}
	for _, pin := range pins {
		anchors[strings.TrimSpace(pin.Number)] = kicadfiles.Point{X: position.X + kicadfiles.MM(pin.XMM), Y: position.Y + kicadfiles.MM(pin.YMM)}
	}
	return anchors
}

func schematicElectricalEndpointPoint(symbolPins map[string]map[string]kicadfiles.Point, endpoint transactions.Endpoint) (kicadfiles.Point, bool) {
	pins, ok := symbolPins[endpoint.Ref]
	if !ok {
		return kicadfiles.Point{}, false
	}
	point, ok := pins[strings.TrimSpace(endpoint.Pin)]
	if !ok {
		return kicadfiles.Point{}, false
	}
	return point, true
}

func schematicElectricalIssues(report schematicrules.Report) []reports.Issue {
	issues := make([]reports.Issue, 0, len(report.Findings))
	for _, finding := range report.Findings {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   finding.Severity,
			Path:       finding.Path,
			Message:    string(finding.RuleID) + ": " + finding.Message,
			Refs:       schematicElectricalIssueRefs(finding),
			Nets:       schematicElectricalIssueNets(finding),
			Suggestion: finding.Repair,
		})
	}
	return issues
}

func schematicElectricalIssueRefs(finding schematicrules.Finding) []string {
	if strings.TrimSpace(finding.Reference) == "" {
		return nil
	}
	return []string{strings.TrimSpace(finding.Reference)}
}

func schematicElectricalEndpointRefs(endpoint transactions.Endpoint) []string {
	if strings.TrimSpace(endpoint.Ref) == "" {
		return nil
	}
	return []string{strings.TrimSpace(endpoint.Ref)}
}

func schematicElectricalLabelKey(netName string, point kicadfiles.Point) string {
	return strings.TrimSpace(netName) + "@" + strconv.FormatInt(int64(point.X), 10) + "," + strconv.FormatInt(int64(point.Y), 10)
}

func schematicElectricalIssueNets(finding schematicrules.Finding) []string {
	if strings.TrimSpace(finding.Net) == "" {
		return nil
	}
	return []string{strings.TrimSpace(finding.Net)}
}
