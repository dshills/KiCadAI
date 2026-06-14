package placement

import (
	"fmt"
	"math"
	"strings"

	"kicadai/internal/reports"
)

const defaultPlacementLayer = "F.Cu"

func BoardUsableRect(board BoardPlacementArea, rules Rules) Rect {
	margin := board.MarginMM
	if rules.BoardEdgeClearanceMM > margin {
		margin = rules.BoardEdgeClearanceMM
	}
	minX := board.Origin.XMM + margin
	minY := board.Origin.YMM + margin
	maxX := board.Origin.XMM + board.WidthMM - margin
	maxY := board.Origin.YMM + board.HeightMM - margin
	if minX > maxX {
		center := board.Origin.XMM + board.WidthMM/2
		minX = center
		maxX = center
	}
	if minY > maxY {
		center := board.Origin.YMM + board.HeightMM/2
		minY = center
		maxY = center
	}
	return Rect{
		Min: Point{XMM: minX, YMM: minY},
		Max: Point{XMM: maxX, YMM: maxY},
	}
}

func ComponentPlacementBounds(component Component, placement Placement, rules Rules) (Rect, bool) {
	return componentPlacementBounds(component, placement, rules.ComponentSpacingMM/2+component.Bounds.CourtyardMM)
}

func ComponentPhysicalBounds(component Component, placement Placement) (Rect, bool) {
	return componentPlacementBounds(component, placement, 0)
}

func componentPlacementBounds(component Component, placement Placement, spacing float64) (Rect, bool) {
	if component.Bounds.WidthMM <= 0 || component.Bounds.HeightMM <= 0 {
		return Rect{}, false
	}
	if !validRotation(placement.RotationDeg) {
		return Rect{}, false
	}
	return rotatedBounds(component.Bounds, placement, spacing), true
}

func ValidateGeometry(request Request, placements []PlacementResult) []reports.Issue {
	request = NormalizeRequest(request)
	var issues []reports.Issue
	usable := BoardUsableRect(request.Board, request.Rules)
	components := map[string]Component{}
	for _, component := range request.Components {
		components[strings.ToUpper(component.Ref)] = component
	}
	occupancy := newOccupancy(request)
	for i := range placements {
		placement := &placements[i]
		path := fmt.Sprintf("placements[%d]", i)
		component, ok := components[strings.ToUpper(strings.TrimSpace(placement.Ref))]
		if !ok {
			issues = append(issues, issue(path+".ref", "placement references unknown component "+placement.Ref))
			continue
		}
		candidate := *placement
		physicalBounds, ok := ComponentPhysicalBounds(component, placement.Position)
		if !ok {
			issues = append(issues, issue(path+".bounds", "placement bounds unavailable for component "+component.Ref))
			continue
		}
		if !usable.Contains(physicalBounds) {
			issues = append(issues, geometryIssue(reports.CodePlacementOutsideBoard, path+".bounds", "placement is outside usable board area"))
		}
		collisionBounds := placement.Bounds
		if collisionBounds.IsZero() {
			var ok bool
			collisionBounds, ok = ComponentPlacementBounds(component, placement.Position, request.Rules)
			if !ok {
				issues = append(issues, issue(path+".bounds", "placement bounds unavailable for component "+component.Ref))
				continue
			}
		}
		candidate.Bounds = collisionBounds
		if conflict, ok := occupancy.FirstConflict(candidate); ok {
			issues = append(issues, geometryIssue(reports.CodePlacementCollision, path+".bounds", "placement conflicts with "+conflict))
		}
		occupancy.Add(candidate)
	}
	return issues
}

func geometryIssue(code reports.Code, path string, message string) reports.Issue {
	return reports.Issue{
		Code:     code,
		Severity: reports.SeverityError,
		Path:     path,
		Message:  message,
	}
}

func NewPlacementResult(component Component, placement Placement, rules Rules) (PlacementResult, bool) {
	bounds, ok := ComponentPlacementBounds(component, placement, rules)
	if !ok {
		return PlacementResult{}, false
	}
	return PlacementResult{
		Ref:         component.Ref,
		FootprintID: component.FootprintID,
		Position:    normalizePlacementLayer(placement),
		Bounds:      bounds,
		Fixed:       component.Fixed,
		GroupID:     component.GroupID,
	}, true
}

type occupancy struct {
	placements []PlacementResult
	keepouts   []Keepout
}

func newOccupancy(request Request) *occupancy {
	return &occupancy{keepouts: request.Keepouts}
}

func (o *occupancy) Add(placement PlacementResult) {
	placement.Position = normalizePlacementLayer(placement.Position)
	o.placements = append(o.placements, placement)
}

func (o *occupancy) FirstConflict(candidate PlacementResult) (string, bool) {
	candidate.Position = normalizePlacementLayer(candidate.Position)
	candidateLayer := candidate.Position.Layer
	for _, keepout := range o.keepouts {
		if keepoutAppliesToLayer(keepout, candidateLayer) && keepout.Bounds.Intersects(candidate.Bounds) {
			if keepout.ID != "" {
				return "keepout " + keepout.ID, true
			}
			return "keepout", true
		}
	}
	for _, existing := range o.placements {
		if !strings.EqualFold(existing.Position.Layer, candidateLayer) {
			continue
		}
		if existing.Bounds.Intersects(candidate.Bounds) {
			return "component " + existing.Ref, true
		}
	}
	return "", false
}

func rotatedBounds(bounds Bounds, placement Placement, spacing float64) Rect {
	left := -bounds.AnchorOffset.XMM
	top := -bounds.AnchorOffset.YMM
	right := left + bounds.WidthMM
	bottom := top + bounds.HeightMM
	corners := []Point{
		{XMM: left, YMM: top},
		{XMM: right, YMM: top},
		{XMM: right, YMM: bottom},
		{XMM: left, YMM: bottom},
	}
	minX := math.Inf(1)
	minY := math.Inf(1)
	maxX := math.Inf(-1)
	maxY := math.Inf(-1)
	for _, corner := range corners {
		rotated := rotatePoint(corner, placement.RotationDeg)
		x := placement.XMM + rotated.XMM
		y := placement.YMM + rotated.YMM
		minX = math.Min(minX, x)
		minY = math.Min(minY, y)
		maxX = math.Max(maxX, x)
		maxY = math.Max(maxY, y)
	}
	return Rect{
		Min: Point{XMM: minX - spacing, YMM: minY - spacing},
		Max: Point{XMM: maxX + spacing, YMM: maxY + spacing},
	}
}

func rotatePoint(point Point, degrees float64) Point {
	normalized := math.Mod(degrees, 360)
	if normalized < 0 {
		normalized += 360
	}
	switch {
	case nearlyEqual(normalized, 0) || nearlyEqual(normalized, 360):
		return point
	case nearlyEqual(normalized, 90):
		return Point{XMM: -point.YMM, YMM: point.XMM}
	case nearlyEqual(normalized, 180):
		return Point{XMM: -point.XMM, YMM: -point.YMM}
	case nearlyEqual(normalized, 270):
		return Point{XMM: point.YMM, YMM: -point.XMM}
	default:
		radians := normalized * math.Pi / 180
		sin, cos := math.Sin(radians), math.Cos(radians)
		return Point{
			XMM: point.XMM*cos - point.YMM*sin,
			YMM: point.XMM*sin + point.YMM*cos,
		}
	}
}

func (r Rect) WidthMM() float64 {
	return r.Max.XMM - r.Min.XMM
}

func (r Rect) HeightMM() float64 {
	return r.Max.YMM - r.Min.YMM
}

func (r Rect) IsZero() bool {
	return r.Min == (Point{}) && r.Max == (Point{})
}

func (r Rect) Contains(other Rect) bool {
	return other.Min.XMM >= r.Min.XMM &&
		other.Min.YMM >= r.Min.YMM &&
		other.Max.XMM <= r.Max.XMM &&
		other.Max.YMM <= r.Max.YMM
}

func (r Rect) Intersects(other Rect) bool {
	return r.Min.XMM < other.Max.XMM &&
		r.Max.XMM > other.Min.XMM &&
		r.Min.YMM < other.Max.YMM &&
		r.Max.YMM > other.Min.YMM
}

func (r Rect) Center() Point {
	return Point{
		XMM: (r.Min.XMM + r.Max.XMM) / 2,
		YMM: (r.Min.YMM + r.Max.YMM) / 2,
	}
}

func normalizePlacementLayer(placement Placement) Placement {
	placement.Layer = normalizeLayer(placement.Layer)
	return placement
}

func normalizeLayer(layer string) string {
	layer = strings.TrimSpace(layer)
	if layer == "" {
		return defaultPlacementLayer
	}
	return layer
}

func keepoutAppliesToLayer(keepout Keepout, layer string) bool {
	if len(keepout.Layers) == 0 {
		return true
	}
	layer = normalizeLayer(layer)
	for _, keepoutLayer := range keepout.Layers {
		if strings.EqualFold(strings.TrimSpace(keepoutLayer), layer) {
			return true
		}
	}
	return false
}

func nearlyEqual(a float64, b float64) bool {
	const epsilon = 1e-7
	return math.Abs(a-b) < epsilon
}
