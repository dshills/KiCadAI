package generate

import (
	"fmt"
	"math"
	"strings"

	"kicadai/internal/reports"
)

func ValidateBreakoutRequest(req BreakoutRequest) []reports.Issue {
	var issues []reports.Issue
	if strings.TrimSpace(req.Kind) != "breakout_board" {
		issues = append(issues, invalid("kind", "kind must be breakout_board"))
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		issues = append(issues, invalid("name", "name is required"))
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		issues = append(issues, invalid("name", "name must be a project name, not a path"))
	}
	if req.Board.WidthMM <= 0 || !finite(req.Board.WidthMM) {
		issues = append(issues, invalid("board.width_mm", "board width must be positive and finite"))
	}
	if req.Board.HeightMM <= 0 || !finite(req.Board.HeightMM) {
		issues = append(issues, invalid("board.height_mm", "board height must be positive and finite"))
	}
	if req.GroundZone && (req.Board.WidthMM <= 2 || req.Board.HeightMM <= 2) {
		issues = append(issues, invalid("board", "ground-zone boards must be larger than the 1mm zone margin on each side"))
	}
	if len(req.Connectors) != 2 {
		issues = append(issues, invalid("connectors", "the initial breakout generator requires exactly two connectors"))
	}
	refs := map[string]struct{}{}
	for i, connector := range req.Connectors {
		path := fmt.Sprintf("connectors[%d]", i)
		ref := strings.TrimSpace(connector.Ref)
		if ref == "" {
			issues = append(issues, invalid(path+".ref", "connector reference is required"))
		} else if _, ok := refs[ref]; ok {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeDuplicateReference,
				Severity: reports.SeverityError,
				Path:     path + ".ref",
				Message:  "duplicate connector reference " + ref,
				Refs:     []string{ref},
			})
		}
		refs[ref] = struct{}{}
		if len(connector.Pins) == 0 {
			issues = append(issues, invalid(path+".pins", "connector requires at least one pin"))
		}
		if len(connector.Pins) > 40 {
			issues = append(issues, invalid(path+".pins", "connector pin count must be 40 or fewer for standard KiCad pin-header libraries"))
		}
		pins := map[string]struct{}{}
		for pinIndex, pin := range connector.Pins {
			pin = strings.TrimSpace(pin)
			pinPath := fmt.Sprintf("%s.pins[%d]", path, pinIndex)
			if pin == "" {
				issues = append(issues, invalid(pinPath, "pin name is required"))
				continue
			}
			if _, ok := pins[pin]; ok {
				issues = append(issues, invalid(pinPath, "duplicate pin name "+pin))
			}
			pins[pin] = struct{}{}
		}
	}
	if len(req.Connectors) == 2 {
		base := normalizedPins(req.Connectors[0].Pins)
		for i := 1; i < len(req.Connectors); i++ {
			if !samePins(base, normalizedPins(req.Connectors[i].Pins)) || !sameOrderedPins(req.Connectors[0].Pins, req.Connectors[i].Pins) {
				issues = append(issues, invalid(fmt.Sprintf("connectors[%d].pins", i), "both connectors must expose the same pin names in the same order"))
			}
		}
	}
	return issues
}

func sameOrderedPins(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}

func invalid(path, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: path, Message: message}
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func normalizedPins(pins []string) map[string]struct{} {
	result := make(map[string]struct{}, len(pins))
	for _, pin := range pins {
		pin = strings.TrimSpace(pin)
		if pin != "" {
			result[pin] = struct{}{}
		}
	}
	return result
}

func samePins(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for key := range a {
		if _, ok := b[key]; !ok {
			return false
		}
	}
	return true
}
