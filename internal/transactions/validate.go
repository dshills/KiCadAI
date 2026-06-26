package transactions

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

	"kicadai/internal/reports"
)

type ValidationResult struct {
	OperationCount int                  `json:"operation_count"`
	Operations     []ValidatedOperation `json:"operations,omitempty"`
	Issues         []reports.Issue      `json:"issues"`
}

type ValidatedOperation struct {
	ID    string        `json:"id"`
	Index int           `json:"index"`
	Op    OperationKind `json:"op"`
	Refs  []string      `json:"refs,omitempty"`
	Nets  []string      `json:"nets,omitempty"`
}

func LoadFile(path string) (Transaction, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Transaction{}, err
	}
	return Parse(data)
}

func Parse(data []byte) (Transaction, error) {
	var tx Transaction
	if err := json.Unmarshal(data, &tx); err != nil {
		return Transaction{}, err
	}
	for i := range tx.Operations {
		tx.Operations[i].Index = i
	}
	return tx, nil
}

func Validate(tx Transaction) ValidationResult {
	result := ValidationResult{OperationCount: len(tx.Operations), Issues: []reports.Issue{}}
	if len(tx.Operations) == 0 {
		result.Issues = append(result.Issues, issue(reports.CodeInvalidArgument, "operations", "transaction requires at least one operation"))
		return result
	}
	result.Operations = make([]ValidatedOperation, 0, len(tx.Operations))
	plannedOperations := make([]PlannedOperation, 0, len(tx.Operations))
	operationIDs := map[string]struct{}{}
	operationIDCounts := map[string]int{}
	for i, op := range tx.Operations {
		op.Index = i
		planned := plannedOperationIdentity(op)
		planned.ID = uniquePlannedOperationID(plannedOperationID(planned, op), operationIDs, operationIDCounts)
		plannedOperations = append(plannedOperations, planned)
		result.Operations = append(result.Operations, ValidatedOperation{
			ID:    planned.ID,
			Index: planned.Index,
			Op:    planned.Op,
			Refs:  append([]string(nil), planned.Refs...),
			Nets:  append([]string(nil), planned.Nets...),
		})
		result.Issues = append(result.Issues, validateOperation(op)...)
	}
	AnnotateIssueOperationIDs(result.Issues, plannedOperations)
	return result
}

func plannedOperationIdentity(op Operation) PlannedOperation {
	planned := PlannedOperation{Index: op.Index, Op: op.Op}
	switch op.Op {
	case OpAddSymbol:
		var payload AddSymbolOperation
		if decodeRaw(op, &payload) == nil {
			addRef(&planned, payload.Ref)
		}
	case OpConnect:
		var payload ConnectOperation
		if decodeRaw(op, &payload) == nil {
			addRef(&planned, payload.From.Ref)
			addRef(&planned, payload.To.Ref)
			addNet(&planned, payload.NetName)
		}
	case OpAddNoConnect:
		var payload AddNoConnectOperation
		if decodeRaw(op, &payload) == nil {
			addRef(&planned, payload.Endpoint.Ref)
		}
	case OpAssignFootprint:
		var payload AssignFootprintOperation
		if decodeRaw(op, &payload) == nil {
			addRef(&planned, payload.Ref)
		}
	case OpPlaceFootprint:
		var payload PlaceFootprintOperation
		if decodeRaw(op, &payload) == nil {
			addRef(&planned, payload.Ref)
			for _, pad := range payload.Pads {
				if pad.Net != nil {
					addNet(&planned, *pad.Net)
				}
			}
		}
	case OpRoute:
		var payload RouteOperation
		if decodeRaw(op, &payload) == nil {
			addNet(&planned, payload.NetName)
		}
	case OpAddZone:
		var payload AddZoneOperation
		if decodeRaw(op, &payload) == nil && payload.NetName != nil {
			addNet(&planned, *payload.NetName)
		}
	}
	return planned
}

func validateOperation(op Operation) []reports.Issue {
	path := fmt.Sprintf("operations[%d]", op.Index)
	if op.Op == "" {
		return []reports.Issue{issue(reports.CodeInvalidArgument, path+".op", "operation kind is required")}
	}
	switch op.Op {
	case OpCreateProject:
		var payload CreateProjectOperation
		return validateDecoded(op, &payload, func() []reports.Issue {
			return requireNonEmpty(path+".name", "project name", payload.Name)
		})
	case OpSetBoardOutline:
		var payload SetBoardOutlineOperation
		return validateDecoded(op, &payload, func() []reports.Issue {
			if payload.Board != nil && len(payload.Points) > 0 {
				return []reports.Issue{issue(reports.CodeInvalidArgument, path, "set_board_outline must provide either board or points, not both")}
			}
			if len(payload.Points) > 0 {
				return validatePolygon(path+".points", payload.Points)
			}
			if payload.Board == nil {
				return []reports.Issue{issue(reports.CodeInvalidArgument, path+".board", "board size or outline points are required")}
			}
			var issues []reports.Issue
			if payload.Board.WidthMM <= 0 || !finite(payload.Board.WidthMM) {
				issues = append(issues, issue(reports.CodeInvalidArgument, path+".board.width_mm", "board width must be positive and finite"))
			}
			if payload.Board.HeightMM <= 0 || !finite(payload.Board.HeightMM) {
				issues = append(issues, issue(reports.CodeInvalidArgument, path+".board.height_mm", "board height must be positive and finite"))
			}
			return issues
		})
	case OpAddSymbol:
		var payload AddSymbolOperation
		return validateDecoded(op, &payload, func() []reports.Issue {
			var issues []reports.Issue
			issues = append(issues, requireNonEmpty(path+".ref", "reference", payload.Ref)...)
			issues = append(issues, requireLibraryID(path+".library_id", payload.LibraryID)...)
			issues = append(issues, validatePoint(path+".at", payload.At)...)
			for pinIndex, pin := range payload.Pins {
				if strings.TrimSpace(pin.Number) == "" {
					issues = append(issues, issue(reports.CodeInvalidArgument, fmt.Sprintf("%s.pins[%d].number", path, pinIndex), "pin number is required"))
				}
				issues = append(issues, validatePoint(fmt.Sprintf("%s.pins[%d]", path, pinIndex), Point{XMM: pin.XMM, YMM: pin.YMM})...)
			}
			return issues
		})
	case OpConnect:
		var payload ConnectOperation
		return validateDecoded(op, &payload, func() []reports.Issue {
			var issues []reports.Issue
			issues = append(issues, validateEndpoint(path+".from", payload.From)...)
			issues = append(issues, validateEndpoint(path+".to", payload.To)...)
			issues = append(issues, requireNonEmpty(path+".net_name", "net_name", payload.NetName)...)
			return issues
		})
	case OpAddNoConnect:
		var payload AddNoConnectOperation
		return validateDecoded(op, &payload, func() []reports.Issue {
			return validateEndpoint(path+".endpoint", payload.Endpoint)
		})
	case OpAssignFootprint:
		var payload AssignFootprintOperation
		return validateDecoded(op, &payload, func() []reports.Issue {
			var issues []reports.Issue
			issues = append(issues, requireNonEmpty(path+".ref", "reference", payload.Ref)...)
			issues = append(issues, requireLibraryID(path+".footprint_id", payload.FootprintID)...)
			return issues
		})
	case OpPlaceFootprint:
		var payload PlaceFootprintOperation
		return validateDecoded(op, &payload, func() []reports.Issue {
			var issues []reports.Issue
			issues = append(issues, requireNonEmpty(path+".ref", "reference", payload.Ref)...)
			issues = append(issues, validatePoint(path+".at", payload.At)...)
			if !finite(payload.Rotation) {
				issues = append(issues, issue(reports.CodeInvalidArgument, path+".rotation_deg", "rotation must be finite"))
			}
			for padIndex, pad := range payload.Pads {
				if strings.TrimSpace(pad.Name) == "" {
					issues = append(issues, issue(reports.CodeInvalidArgument, fmt.Sprintf("%s.pads[%d].name", path, padIndex), "pad name is required"))
				}
				issues = append(issues, validatePoint(fmt.Sprintf("%s.pads[%d]", path, padIndex), Point{XMM: pad.XMM, YMM: pad.YMM})...)
				issues = append(issues, validatePadGeometry(fmt.Sprintf("%s.pads[%d]", path, padIndex), pad)...)
			}
			return issues
		})
	case OpRoute:
		var payload RouteOperation
		return validateDecoded(op, &payload, func() []reports.Issue {
			var issues []reports.Issue
			issues = append(issues, requireNonEmpty(path+".net_name", "net name", payload.NetName)...)
			if len(payload.Points) < 2 && len(payload.Vias) == 0 {
				issues = append(issues, issue(reports.CodeInvalidArgument, path+".points", "route requires at least two points"))
			}
			for pointIndex, point := range payload.Points {
				issues = append(issues, validatePoint(fmt.Sprintf("%s.points[%d]", path, pointIndex), point)...)
				if pointIndex > 0 && samePoint(payload.Points[pointIndex-1], point) {
					issues = append(issues, issue(reports.CodeInvalidArgument, fmt.Sprintf("%s.points[%d]", path, pointIndex), "route contains a zero-length segment"))
				}
			}
			for viaIndex, via := range payload.Vias {
				prefix := fmt.Sprintf("%s.vias[%d]", path, viaIndex)
				issues = append(issues, validatePoint(prefix+".at", via.At)...)
				if via.DiameterMM <= 0 {
					issues = append(issues, issue(reports.CodeInvalidArgument, prefix+".diameter_mm", "via diameter must be positive"))
				}
				if via.DrillMM <= 0 || via.DrillMM >= via.DiameterMM {
					issues = append(issues, issue(reports.CodeInvalidArgument, prefix+".drill_mm", "via drill must be positive and smaller than diameter"))
				}
				if len(via.Layers) < 2 {
					issues = append(issues, issue(reports.CodeInvalidArgument, prefix+".layers", "via requires at least two layers"))
				}
			}
			return issues
		})
	case OpAddZone:
		var payload AddZoneOperation
		return validateDecoded(op, &payload, func() []reports.Issue {
			var issues []reports.Issue
			if payload.NetName != nil && strings.TrimSpace(*payload.NetName) == "" {
				issues = append(issues, issue(reports.CodeInvalidArgument, path+".net_name", "net name must be null or non-empty"))
			}
			issues = append(issues, validatePolygon(path+".polygon", payload.Polygon)...)
			return issues
		})
	case OpWriteProject:
		var payload WriteProjectOperation
		return validateDecoded(op, &payload, func() []reports.Issue { return nil })
	default:
		return []reports.Issue{issue(reports.CodeUnsupportedOperation, path+".op", "unsupported operation "+string(op.Op))}
	}
}

func validateDecoded[T any](op Operation, target *T, validate func() []reports.Issue) []reports.Issue {
	if err := json.Unmarshal(op.Raw, target); err != nil {
		return []reports.Issue{issue(reports.CodeInvalidArgument, fmt.Sprintf("operations[%d]", op.Index), err.Error())}
	}
	return validate()
}

func validateEndpoint(path string, endpoint Endpoint) []reports.Issue {
	var issues []reports.Issue
	issues = append(issues, requireNonEmpty(path+".ref", "endpoint reference", endpoint.Ref)...)
	issues = append(issues, requireNonEmpty(path+".pin", "endpoint pin", endpoint.Pin)...)
	return issues
}

func validatePolygon(path string, points []Point) []reports.Issue {
	var issues []reports.Issue
	if len(points) < 3 {
		issues = append(issues, issue(reports.CodeInvalidArgument, path, "polygon requires at least three points"))
	}
	for i, point := range points {
		issues = append(issues, validatePoint(fmt.Sprintf("%s[%d]", path, i), point)...)
		if i > 0 && samePoint(points[i-1], point) {
			issues = append(issues, issue(reports.CodeInvalidArgument, fmt.Sprintf("%s[%d]", path, i), "polygon contains a zero-length segment"))
		}
	}
	return issues
}

func validatePadGeometry(path string, pad PadSpec) []reports.Issue {
	var issues []reports.Issue
	for _, field := range []struct {
		name  string
		value float64
	}{
		{name: "width_mm", value: pad.WidthMM},
		{name: "height_mm", value: pad.HeightMM},
		{name: "drill_mm", value: pad.DrillMM},
	} {
		if !finite(field.value) {
			issues = append(issues, issue(reports.CodeInvalidArgument, path+"."+field.name, field.name+" must be finite"))
		} else if field.value < 0 {
			issues = append(issues, issue(reports.CodeInvalidArgument, path+"."+field.name, field.name+" must be non-negative"))
		}
	}
	return issues
}

func validatePoint(path string, point Point) []reports.Issue {
	var issues []reports.Issue
	if !finite(point.XMM) {
		issues = append(issues, issue(reports.CodeInvalidArgument, path+".x_mm", "x coordinate must be finite"))
	}
	if !finite(point.YMM) {
		issues = append(issues, issue(reports.CodeInvalidArgument, path+".y_mm", "y coordinate must be finite"))
	}
	return issues
}

func requireLibraryID(path, value string) []reports.Issue {
	issues := requireNonEmpty(path, "library id", value)
	if len(issues) > 0 {
		return issues
	}
	colon := strings.Index(value, ":")
	if colon <= 0 || colon == len(value)-1 || strings.TrimSpace(value[:colon]) == "" || strings.TrimSpace(value[colon+1:]) == "" {
		return []reports.Issue{issue(reports.CodeInvalidArgument, path, "library id must use Library:Name syntax")}
	}
	return nil
}

func requireNonEmpty(path, label, value string) []reports.Issue {
	if strings.TrimSpace(value) == "" {
		return []reports.Issue{issue(reports.CodeInvalidArgument, path, label+" is required")}
	}
	return nil
}

func issue(code reports.Code, path string, message string) reports.Issue {
	return reports.Issue{Code: code, Severity: reports.SeverityError, Path: path, Message: message}
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func samePoint(a, b Point) bool {
	return a.XMM == b.XMM && a.YMM == b.YMM
}
