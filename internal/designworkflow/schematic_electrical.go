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
	return schematicElectricalStageFromTransaction(tx)
}

func schematicElectricalStageFromTransaction(tx transactions.Transaction) StageResult {
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
	var wireCandidates []schematicElectricalWireCandidate
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
			netName := strings.TrimSpace(payload.NetName)
			if netName != "" {
				labelOnly := payload.UseLabels != nil && *payload.UseLabels
				if !labelOnly || !payload.SkipFromLabel {
					file.Labels = schematicElectricalAppendLabel(file.Labels, labels, netName, from)
				}
				if !labelOnly || !payload.SkipToLabel {
					file.Labels = schematicElectricalAppendLabel(file.Labels, labels, netName, to)
				}
				if !labelOnly {
					wireCandidates = append(wireCandidates, schematicElectricalWireCandidate{NetName: netName, From: from, To: to})
				}
			} else {
				file.Wires = append(file.Wires, schematic.Wire{Points: []kicadfiles.Point{from, to}})
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
	file.Wires = append(file.Wires, schematicElectricalSafeWires(wireCandidates, file.Labels, schematicElectricalObstacles(file))...)
	return file, opts, nil
}

type schematicElectricalWireCandidate struct {
	NetName string
	From    kicadfiles.Point
	To      kicadfiles.Point
}

func schematicElectricalAppendLabel(existing []schematic.Label, labels map[string]struct{}, netName string, point kicadfiles.Point) []schematic.Label {
	labelKey := schematicElectricalLabelKey(netName, point)
	if _, exists := labels[labelKey]; exists {
		return existing
	}
	labels[labelKey] = struct{}{}
	return append(existing, schematic.Label{Text: netName, Kind: schematic.LabelLocal, Position: point})
}

func schematicElectricalSafeWires(candidates []schematicElectricalWireCandidate, labels []schematic.Label, obstacles []kicadfiles.Point) []schematic.Wire {
	wires := make([]schematic.Wire, 0, len(candidates))
	for _, candidate := range candidates {
		wires = append(wires, schematicElectricalWireForCandidate(candidate, labels, obstacles))
	}
	return wires
}

func schematicElectricalWireForCandidate(candidate schematicElectricalWireCandidate, labels []schematic.Label, obstacles []kicadfiles.Point) schematic.Wire {
	direct := []kicadfiles.Point{candidate.From, candidate.To}
	if !schematicElectricalPolylineBlocked(candidate, direct, labels, obstacles) {
		return schematic.Wire{Points: direct}
	}
	for _, offset := range []kicadfiles.IU{kicadfiles.MM(2.54), -kicadfiles.MM(2.54), kicadfiles.MM(5.08), -kicadfiles.MM(5.08), kicadfiles.MM(7.62), -kicadfiles.MM(7.62)} {
		yDogleg := []kicadfiles.Point{
			candidate.From,
			{X: candidate.From.X, Y: candidate.From.Y + offset},
			{X: candidate.To.X, Y: candidate.From.Y + offset},
			candidate.To,
		}
		if !schematicElectricalPolylineBlocked(candidate, yDogleg, labels, obstacles) {
			return schematic.Wire{Points: yDogleg}
		}
		xDogleg := []kicadfiles.Point{
			candidate.From,
			{X: candidate.From.X + offset, Y: candidate.From.Y},
			{X: candidate.From.X + offset, Y: candidate.To.Y},
			candidate.To,
		}
		if !schematicElectricalPolylineBlocked(candidate, xDogleg, labels, obstacles) {
			return schematic.Wire{Points: xDogleg}
		}
	}
	return schematic.Wire{Points: direct}
}

func schematicElectricalObstacles(file schematic.SchematicFile) []kicadfiles.Point {
	seen := map[kicadfiles.Point]struct{}{}
	for _, symbol := range file.Symbols {
		for _, anchor := range symbol.PinAnchors {
			seen[anchor] = struct{}{}
		}
	}
	for _, noConnect := range file.NoConnects {
		seen[noConnect.Position] = struct{}{}
	}
	obstacles := make([]kicadfiles.Point, 0, len(seen))
	for obstacle := range seen {
		obstacles = append(obstacles, obstacle)
	}
	return obstacles
}

func schematicElectricalPolylineBlocked(candidate schematicElectricalWireCandidate, points []kicadfiles.Point, labels []schematic.Label, obstacles []kicadfiles.Point) bool {
	if schematicElectricalPolylineCrossesOtherNetLabel(candidate.NetName, points, labels) {
		return true
	}
	for index := 1; index < len(points); index++ {
		for _, obstacle := range obstacles {
			if obstacle == candidate.From || obstacle == candidate.To {
				continue
			}
			if schematicElectricalPointOnSegment(obstacle, points[index-1], points[index]) {
				return true
			}
		}
	}
	return false
}

func schematicElectricalPolylineCrossesOtherNetLabel(netName string, points []kicadfiles.Point, labels []schematic.Label) bool {
	for index := 1; index < len(points); index++ {
		if schematicElectricalSegmentCrossesOtherNetLabel(netName, points[index-1], points[index], labels) {
			return true
		}
	}
	return false
}

func schematicElectricalSegmentCrossesOtherNetLabel(netName string, start, end kicadfiles.Point, labels []schematic.Label) bool {
	for _, label := range labels {
		if label.Text == netName {
			continue
		}
		if schematicElectricalPointStrictlyInsideSegment(label.Position, start, end) {
			return true
		}
	}
	return false
}

func schematicElectricalPointStrictlyInsideSegment(point, start, end kicadfiles.Point) bool {
	if point == start || point == end {
		return false
	}
	dx1 := int64(end.X) - int64(start.X)
	dy1 := int64(end.Y) - int64(start.Y)
	dx2 := int64(point.X) - int64(start.X)
	dy2 := int64(point.Y) - int64(start.Y)
	if dx1*dy2 != dy1*dx2 {
		return false
	}
	return schematicElectricalBetween(point.X, start.X, end.X) && schematicElectricalBetween(point.Y, start.Y, end.Y)
}

func schematicElectricalPointOnSegment(point, start, end kicadfiles.Point) bool {
	if !schematicElectricalBetween(point.X, start.X, end.X) || !schematicElectricalBetween(point.Y, start.Y, end.Y) {
		return false
	}
	dx1 := int64(end.X) - int64(start.X)
	dy1 := int64(end.Y) - int64(start.Y)
	dx2 := int64(point.X) - int64(start.X)
	dy2 := int64(point.Y) - int64(start.Y)
	return dx1*dy2 == dy1*dx2
}

func schematicElectricalBetween(value, first, second kicadfiles.IU) bool {
	if first > second {
		first, second = second, first
	}
	return value >= first && value <= second
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
