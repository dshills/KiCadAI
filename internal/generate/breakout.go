package generate

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type BreakoutOptions struct {
	OutputDir string
	Overwrite bool
	Seed      string
}

func LoadBreakoutRequest(path string) (BreakoutRequest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return BreakoutRequest{}, err
	}
	var req BreakoutRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return BreakoutRequest{}, err
	}
	return req, nil
}

func GenerateBreakout(req BreakoutRequest, opts BreakoutOptions) BreakoutResult {
	issues := ValidateBreakoutRequest(req)
	if issues == nil {
		issues = []reports.Issue{}
	}
	result := BreakoutResult{
		ProjectName: strings.TrimSpace(req.Name),
		Board:       req.Board,
		Connectors:  req.Connectors,
		Artifacts:   []reports.Artifact{},
		Issues:      issues,
	}
	if reports.HasBlockingIssue(result.Issues) {
		return result
	}
	tx, err := BreakoutTransaction(req)
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "transaction",
			Message:  err.Error(),
		})
		return result
	}
	result.TransactionOperations = len(tx.Operations)
	apply := transactions.Apply(tx, transactions.ApplyOptions{
		OutputDir: opts.OutputDir,
		Overwrite: opts.Overwrite,
		Seed:      opts.Seed,
	})
	result.Artifacts = apply.Artifacts
	result.Issues = append(result.Issues, apply.Issues...)
	return result
}

func BreakoutTransaction(req BreakoutRequest) (transactions.Transaction, error) {
	name := strings.TrimSpace(req.Name)
	ops, err := operations(
		transactions.CreateProjectOperation{Op: transactions.OpCreateProject, Name: name},
		transactions.SetBoardOutlineOperation{Op: transactions.OpSetBoardOutline, Board: &transactions.BoardSize{WidthMM: req.Board.WidthMM, HeightMM: req.Board.HeightMM}},
	)
	if err != nil {
		return transactions.Transaction{}, err
	}
	for i, connector := range req.Connectors {
		ref := strings.TrimSpace(connector.Ref)
		pins := connectorPins(connector, i == 0)
		x := req.Board.WidthMM * 0.2
		rotation := 0.0
		if i%2 == 1 {
			x = req.Board.WidthMM * 0.8
			rotation = 180
		}
		y := req.Board.HeightMM / 2
		next, err := operations(
			transactions.AddSymbolOperation{
				Op:        transactions.OpAddSymbol,
				Ref:       ref,
				Value:     ref,
				LibraryID: fmt.Sprintf("Connector_Generic:Conn_01x%02d", len(connector.Pins)),
				At:        transactions.Point{XMM: 25 + float64(i)*35, YMM: 25},
				Pins:      pins,
			},
			transactions.AssignFootprintOperation{
				Op:          transactions.OpAssignFootprint,
				Ref:         ref,
				FootprintID: fmt.Sprintf("Connector_PinHeader_2.54mm:PinHeader_1x%02d_P2.54mm_Vertical", len(connector.Pins)),
			},
			transactions.PlaceFootprintOperation{
				Op:       transactions.OpPlaceFootprint,
				Ref:      ref,
				At:       transactions.Point{XMM: x, YMM: y},
				Rotation: rotation,
				Pads:     connectorPads(connector),
			},
		)
		if err != nil {
			return transactions.Transaction{}, err
		}
		ops = append(ops, next...)
	}
	if len(req.Connectors) >= 2 {
		left := req.Connectors[0]
		right := req.Connectors[1]
		for i, pin := range left.Pins {
			netName := strings.TrimSpace(pin)
			pinNumber := fmt.Sprintf("%d", i+1)
			next, err := operations(
				transactions.ConnectOperation{
					Op:      transactions.OpConnect,
					From:    transactions.Endpoint{Ref: strings.TrimSpace(left.Ref), Pin: pinNumber},
					To:      transactions.Endpoint{Ref: strings.TrimSpace(right.Ref), Pin: pinNumber},
					NetName: netName,
				},
				transactions.RouteOperation{
					Op:      transactions.OpRoute,
					NetName: netName,
					Layer:   "F.Cu",
					WidthMM: 0.25,
					Points: []transactions.Point{
						{XMM: req.Board.WidthMM * 0.2, YMM: pinY(req.Board.HeightMM, len(left.Pins), i, false)},
						{XMM: req.Board.WidthMM * 0.8, YMM: pinY(req.Board.HeightMM, len(left.Pins), i, true)},
					},
				},
			)
			if err != nil {
				return transactions.Transaction{}, err
			}
			ops = append(ops, next...)
		}
	}
	if req.GroundZone && len(req.Connectors) > 0 && hasPin(req.Connectors[0].Pins, "GND") {
		gnd := pinNetName(req.Connectors[0].Pins, "GND")
		op, err := operation(transactions.AddZoneOperation{
			Op:      transactions.OpAddZone,
			Name:    "GND",
			NetName: &gnd,
			Layers:  []string{"B.Cu"},
			Polygon: []transactions.Point{
				{XMM: 1, YMM: 1},
				{XMM: req.Board.WidthMM - 1, YMM: 1},
				{XMM: req.Board.WidthMM - 1, YMM: req.Board.HeightMM - 1},
				{XMM: 1, YMM: req.Board.HeightMM - 1},
			},
		})
		if err != nil {
			return transactions.Transaction{}, err
		}
		ops = append(ops, op)
	}
	op, err := operation(transactions.WriteProjectOperation{Op: transactions.OpWriteProject})
	if err != nil {
		return transactions.Transaction{}, err
	}
	ops = append(ops, op)
	return transactions.Transaction{Name: name, Project: name, Operations: ops}, nil
}

func connectorPins(connector ConnectorRequest, leftSide bool) []transactions.PinSpec {
	pins := make([]transactions.PinSpec, 0, len(connector.Pins))
	x := -2.54
	if leftSide {
		x = 2.54
	}
	for i := range connector.Pins {
		pins = append(pins, transactions.PinSpec{
			Number: fmt.Sprintf("%d", i+1),
			XMM:    x,
			YMM:    (float64(i) - float64(len(connector.Pins)-1)/2.0) * 2.54,
		})
	}
	return pins
}

func connectorPads(connector ConnectorRequest) []transactions.PadSpec {
	pads := make([]transactions.PadSpec, 0, len(connector.Pins))
	for i, pin := range connector.Pins {
		net := strings.TrimSpace(pin)
		pads = append(pads, transactions.PadSpec{
			Name: fmt.Sprintf("%d", i+1),
			Type: "thru_hole",
			YMM:  pinOffsetY(len(connector.Pins), i),
			Net:  &net,
		})
	}
	return pads
}

func pinY(height float64, count int, index int, rotated bool) float64 {
	center := height / 2
	offset := pinOffsetY(count, index)
	if rotated {
		offset = -offset
	}
	return center + offset
}

func pinOffsetY(count int, index int) float64 {
	return (float64(index) - float64(count-1)/2.0) * 2.54
}

func hasPin(pins []string, want string) bool {
	for _, pin := range pins {
		if strings.EqualFold(strings.TrimSpace(pin), want) {
			return true
		}
	}
	return false
}

func pinNetName(pins []string, want string) string {
	for _, pin := range pins {
		if strings.EqualFold(strings.TrimSpace(pin), want) {
			return strings.TrimSpace(pin)
		}
	}
	return want
}

func operations(payloads ...any) ([]transactions.Operation, error) {
	result := make([]transactions.Operation, 0, len(payloads))
	for _, payload := range payloads {
		op, err := operation(payload)
		if err != nil {
			return nil, err
		}
		result = append(result, op)
	}
	return result, nil
}

func operation(payload any) (transactions.Operation, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return transactions.Operation{}, err
	}
	kind, err := operationKind(payload)
	if err != nil {
		return transactions.Operation{}, err
	}
	return transactions.NewOperationWithRef(kind, data, operationRef(payload)), nil
}

func operationRef(payload any) string {
	switch value := payload.(type) {
	case transactions.AddSymbolOperation:
		return value.Ref
	case transactions.AssignFootprintOperation:
		return value.Ref
	case transactions.PlaceFootprintOperation:
		return value.Ref
	default:
		return ""
	}
}

func operationKind(payload any) (transactions.OperationKind, error) {
	switch value := payload.(type) {
	case transactions.CreateProjectOperation:
		return value.Op, nil
	case transactions.SetBoardOutlineOperation:
		return value.Op, nil
	case transactions.AddSymbolOperation:
		return value.Op, nil
	case transactions.AssignFootprintOperation:
		return value.Op, nil
	case transactions.PlaceFootprintOperation:
		return value.Op, nil
	case transactions.ConnectOperation:
		return value.Op, nil
	case transactions.RouteOperation:
		return value.Op, nil
	case transactions.AddZoneOperation:
		return value.Op, nil
	case transactions.WriteProjectOperation:
		return value.Op, nil
	default:
		return "", fmt.Errorf("unsupported operation payload %T", payload)
	}
}
