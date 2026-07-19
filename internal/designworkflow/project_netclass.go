package designworkflow

import (
	"math"
	"strings"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

const projectClearancePrecisionMM = 1e-6

func fitRoutingClearanceToIntrinsicPads(request *routing.Request, components []placement.Component, explicitlyRequired bool) []reports.Issue {
	if request == nil {
		return nil
	}
	intrinsic, ok := minimumIntrinsicPadClearanceMM(components)
	if !ok || intrinsic <= 0 || intrinsic >= request.Rules.ClearanceMM {
		return nil
	}
	intrinsic = math.Floor((intrinsic+1e-9)/projectClearancePrecisionMM) * projectClearancePrecisionMM
	if explicitlyRequired {
		return []reports.Issue{{
			Code:       reports.CodeValidationFailed,
			Severity:   reports.SeverityBlocked,
			Path:       "routing.rules.clearance_mm",
			Message:    "required routing clearance exceeds the intrinsic different-net pad gap of a selected footprint",
			Suggestion: "select a footprint with wider pad spacing or reduce the explicit clearance requirement",
		}}
	}
	request.Rules.ClearanceMM = intrinsic
	if request.Rules.ViaClearanceMM > intrinsic {
		request.Rules.ViaClearanceMM = intrinsic
	}
	for name, class := range request.Rules.NetClasses {
		if class.ClearanceMM > intrinsic {
			class.ClearanceMM = intrinsic
		}
		if class.ViaClearanceMM > intrinsic {
			class.ViaClearanceMM = intrinsic
		}
		request.Rules.NetClasses[name] = class
	}
	for name, override := range request.Rules.NetOverrides {
		if override.ClearanceMM > intrinsic {
			override.ClearanceMM = intrinsic
		}
		if override.ViaClearanceMM > intrinsic {
			override.ViaClearanceMM = intrinsic
		}
		request.Rules.NetOverrides[name] = override
	}
	return nil
}

// projectNetClassClearanceMM keeps KiCad's default net class aligned with the
// geometry that was actually routed. The router still enforces its requested
// clearance everywhere it has freedom to move copper; the only permitted
// reduction is an intrinsic, different-net pad gap inside one verified
// footprint, where no board-level router can increase the spacing.
func projectNetClassClearanceMM(routed *RoutingStageResult, placed *PlacementStageResult) float64 {
	clearance := routing.DefaultRules().ClearanceMM
	if routed != nil && routed.Request.Rules.ClearanceMM > 0 {
		clearance = routed.Request.Rules.ClearanceMM
	}
	if placed == nil {
		return clearance
	}
	if intrinsic, ok := minimumIntrinsicPadClearanceMM(placed.Request.Components); ok && intrinsic > 0 && intrinsic < clearance {
		return math.Floor((intrinsic+1e-9)/projectClearancePrecisionMM) * projectClearancePrecisionMM
	}
	return clearance
}

func projectMinimumThroughHoleDiameterMM(placed *PlacementStageResult) float64 {
	if placed == nil {
		return 0
	}
	const kicadDefaultMinimumHoleMM = 0.3
	minimum := kicadDefaultMinimumHoleMM
	foundSmaller := false
	for _, component := range placed.Request.Components {
		for _, pad := range component.Pads {
			if pad.DrillMM > 0 && pad.DrillMM < minimum {
				minimum = pad.DrillMM
				foundSmaller = true
			}
		}
	}
	if !foundSmaller {
		return 0
	}
	return math.Floor((minimum+1e-9)/projectClearancePrecisionMM) * projectClearancePrecisionMM
}

func minimumIntrinsicPadClearanceMM(components []placement.Component) (float64, bool) {
	minimum := math.Inf(1)
	found := false
	for _, component := range components {
		for leftIndex := 0; leftIndex < len(component.Pads); leftIndex++ {
			left := component.Pads[leftIndex]
			if strings.TrimSpace(left.Net) == "" || left.WidthMM <= 0 || left.HeightMM <= 0 {
				continue
			}
			for rightIndex := leftIndex + 1; rightIndex < len(component.Pads); rightIndex++ {
				right := component.Pads[rightIndex]
				if strings.TrimSpace(right.Net) == "" || strings.EqualFold(strings.TrimSpace(left.Net), strings.TrimSpace(right.Net)) || right.WidthMM <= 0 || right.HeightMM <= 0 || !padSummariesShareCopper(left, right) {
					continue
				}
				gap := padSummaryClearanceMM(left, right)
				if gap < minimum {
					minimum = gap
					found = true
				}
			}
		}
	}
	return minimum, found
}

func padSummariesShareCopper(left, right placement.PadSummary) bool {
	leftLayers, leftAll := padSummaryCopperLayers(left)
	rightLayers, rightAll := padSummaryCopperLayers(right)
	if leftAll || rightAll {
		return true
	}
	for layer := range leftLayers {
		if _, ok := rightLayers[layer]; ok {
			return true
		}
	}
	return false
}

func padSummaryCopperLayers(pad placement.PadSummary) (map[string]struct{}, bool) {
	if pad.DrillMM > 0 || len(pad.Layers) == 0 {
		return nil, true
	}
	layers := map[string]struct{}{}
	for _, layer := range pad.Layers {
		layer = strings.ToUpper(strings.TrimSpace(layer))
		if layer == "*.CU" {
			return nil, true
		}
		if strings.HasSuffix(layer, ".CU") {
			layers[layer] = struct{}{}
		}
	}
	return layers, len(layers) == 0
}

type projectClearancePoint struct {
	x float64
	y float64
}

func padSummaryClearanceMM(left, right placement.PadSummary) float64 {
	leftPolygon := padSummaryPolygon(left)
	rightPolygon := padSummaryPolygon(right)
	if projectPolygonsOverlap(leftPolygon, rightPolygon) {
		return 0
	}
	minimum := math.Inf(1)
	for leftIndex := range leftPolygon {
		leftNext := (leftIndex + 1) % len(leftPolygon)
		for rightIndex := range rightPolygon {
			rightNext := (rightIndex + 1) % len(rightPolygon)
			minimum = math.Min(minimum, projectSegmentDistance(leftPolygon[leftIndex], leftPolygon[leftNext], rightPolygon[rightIndex], rightPolygon[rightNext]))
		}
	}
	return minimum
}

func padSummaryPolygon(pad placement.PadSummary) []projectClearancePoint {
	halfWidth := pad.WidthMM / 2
	halfHeight := pad.HeightMM / 2
	angle := pad.RotationDeg * math.Pi / 180
	cosine := math.Cos(angle)
	sine := math.Sin(angle)
	local := []projectClearancePoint{{-halfWidth, -halfHeight}, {halfWidth, -halfHeight}, {halfWidth, halfHeight}, {-halfWidth, halfHeight}}
	for index := range local {
		x := local[index].x
		y := local[index].y
		local[index] = projectClearancePoint{x: pad.XMM + x*cosine - y*sine, y: pad.YMM + x*sine + y*cosine}
	}
	return local
}

func projectPolygonsOverlap(left, right []projectClearancePoint) bool {
	for leftIndex := range left {
		leftNext := (leftIndex + 1) % len(left)
		for rightIndex := range right {
			rightNext := (rightIndex + 1) % len(right)
			if projectSegmentsIntersect(left[leftIndex], left[leftNext], right[rightIndex], right[rightNext]) {
				return true
			}
		}
	}
	return projectPointInPolygon(left[0], right) || projectPointInPolygon(right[0], left)
}

func projectPointInPolygon(point projectClearancePoint, polygon []projectClearancePoint) bool {
	inside := false
	for index, previous := 0, len(polygon)-1; index < len(polygon); previous, index = index, index+1 {
		currentPoint := polygon[index]
		previousPoint := polygon[previous]
		if (currentPoint.y > point.y) != (previousPoint.y > point.y) && point.x < (previousPoint.x-currentPoint.x)*(point.y-currentPoint.y)/(previousPoint.y-currentPoint.y)+currentPoint.x {
			inside = !inside
		}
	}
	return inside
}

func projectSegmentsIntersect(a, b, c, d projectClearancePoint) bool {
	return projectOrientation(a, b, c)*projectOrientation(a, b, d) <= 0 && projectOrientation(c, d, a)*projectOrientation(c, d, b) <= 0 &&
		math.Max(math.Min(a.x, b.x), math.Min(c.x, d.x)) <= math.Min(math.Max(a.x, b.x), math.Max(c.x, d.x))+1e-12 &&
		math.Max(math.Min(a.y, b.y), math.Min(c.y, d.y)) <= math.Min(math.Max(a.y, b.y), math.Max(c.y, d.y))+1e-12
}

func projectOrientation(a, b, c projectClearancePoint) float64 {
	return (b.x-a.x)*(c.y-a.y) - (b.y-a.y)*(c.x-a.x)
}

func projectSegmentDistance(a, b, c, d projectClearancePoint) float64 {
	if projectSegmentsIntersect(a, b, c, d) {
		return 0
	}
	return math.Min(math.Min(projectPointSegmentDistance(a, c, d), projectPointSegmentDistance(b, c, d)), math.Min(projectPointSegmentDistance(c, a, b), projectPointSegmentDistance(d, a, b)))
}

func projectPointSegmentDistance(point, start, end projectClearancePoint) float64 {
	dx := end.x - start.x
	dy := end.y - start.y
	if dx == 0 && dy == 0 {
		return math.Hypot(point.x-start.x, point.y-start.y)
	}
	t := ((point.x-start.x)*dx + (point.y-start.y)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(point.x-(start.x+t*dx), point.y-(start.y+t*dy))
}
