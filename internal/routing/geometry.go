package routing

import (
	"fmt"
	"math"
)

type Point struct {
	XMM float64 `json:"x_mm"`
	YMM float64 `json:"y_mm"`
}

type Size struct {
	WidthMM  float64 `json:"width_mm"`
	HeightMM float64 `json:"height_mm"`
}

type Rect struct {
	Min Point `json:"min"`
	Max Point `json:"max"`
}

type Shape struct {
	Rect    *Rect   `json:"rect,omitempty"`
	Polygon []Point `json:"polygon,omitempty"`
}

type GridCoord struct {
	X     int
	Y     int
	Layer int
}

type Grid struct {
	Origin Point
	GridMM float64
}

func NewGrid(origin Point, gridMM float64) Grid {
	if gridMM <= 0 || math.IsNaN(gridMM) || math.IsInf(gridMM, 0) {
		gridMM = DefaultRules().GridMM
	}
	return Grid{Origin: origin, GridMM: gridMM}
}

func (grid Grid) ToGrid(point Point, layer int) GridCoord {
	gridMM := grid.spacingMM()
	if gridMM <= 0 {
		return GridCoord{Layer: layer}
	}
	return GridCoord{
		X:     int(math.Round((point.XMM - grid.Origin.XMM) / gridMM)),
		Y:     int(math.Round((point.YMM - grid.Origin.YMM) / gridMM)),
		Layer: layer,
	}
}

func (grid Grid) ToPoint(coord GridCoord) Point {
	gridMM := grid.spacingMM()
	return Point{
		XMM: roundMM(grid.Origin.XMM + float64(coord.X)*gridMM),
		YMM: roundMM(grid.Origin.YMM + float64(coord.Y)*gridMM),
	}
}

func (grid Grid) spacingMM() float64 {
	if grid.GridMM <= 0 || math.IsNaN(grid.GridMM) || math.IsInf(grid.GridMM, 0) {
		defaultGrid := DefaultRules().GridMM
		if defaultGrid <= 0 || math.IsNaN(defaultGrid) || math.IsInf(defaultGrid, 0) {
			return 0.25
		}
		return defaultGrid
	}
	return grid.GridMM
}

func LocalToGlobal(board Board, local Point) Point {
	return Point{XMM: roundMM(board.Origin.XMM + local.XMM), YMM: roundMM(board.Origin.YMM + local.YMM)}
}

func GlobalToLocal(board Board, global Point) Point {
	return Point{XMM: roundMM(global.XMM - board.Origin.XMM), YMM: roundMM(global.YMM - board.Origin.YMM)}
}

func (rect Rect) WidthMM() float64 {
	rect = normalizeRect(rect)
	return rect.Max.XMM - rect.Min.XMM
}

func (rect Rect) HeightMM() float64 {
	rect = normalizeRect(rect)
	return rect.Max.YMM - rect.Min.YMM
}

func (rect Rect) ContainsPoint(point Point) bool {
	rect = normalizeRect(rect)
	return rect.containsPoint(point)
}

func (rect Rect) Contains(other Rect) bool {
	rect = normalizeRect(rect)
	other = normalizeRect(other)
	return rect.containsPoint(other.Min) && rect.containsPoint(other.Max)
}

func (rect Rect) containsPoint(point Point) bool {
	return point.XMM >= rect.Min.XMM && point.XMM <= rect.Max.XMM && point.YMM >= rect.Min.YMM && point.YMM <= rect.Max.YMM
}

func (rect Rect) Intersects(other Rect) bool {
	rect = normalizeRect(rect)
	other = normalizeRect(other)
	return rect.Min.XMM <= other.Max.XMM &&
		rect.Max.XMM >= other.Min.XMM &&
		rect.Min.YMM <= other.Max.YMM &&
		rect.Max.YMM >= other.Min.YMM
}

func (rect Rect) Expand(deltaMM float64) Rect {
	expanded := Rect{
		Min: Point{XMM: rect.Min.XMM - deltaMM, YMM: rect.Min.YMM - deltaMM},
		Max: Point{XMM: rect.Max.XMM + deltaMM, YMM: rect.Max.YMM + deltaMM},
	}
	if deltaMM < 0 {
		if expanded.Min.XMM > expanded.Max.XMM {
			center := (rect.Min.XMM + rect.Max.XMM) / 2
			expanded.Min.XMM = center
			expanded.Max.XMM = center
		}
		if expanded.Min.YMM > expanded.Max.YMM {
			center := (rect.Min.YMM + rect.Max.YMM) / 2
			expanded.Min.YMM = center
			expanded.Max.YMM = center
		}
	}
	return normalizeRect(expanded)
}

func BoardRect(board Board) Rect {
	return Rect{Min: Point{}, Max: Point{XMM: board.WidthMM, YMM: board.HeightMM}}
}

func UsableBoardRect(board Board, rules Rules) Rect {
	margin := board.MarginMM + rules.EdgeClearanceMM
	minX := margin
	maxX := board.WidthMM - margin
	if minX > maxX {
		center := board.WidthMM / 2
		minX = center
		maxX = center
	}
	minY := margin
	maxY := board.HeightMM - margin
	if minY > maxY {
		center := board.HeightMM / 2
		minY = center
		maxY = center
	}
	return normalizeRect(Rect{
		Min: Point{XMM: minX, YMM: minY},
		Max: Point{XMM: maxX, YMM: maxY},
	})
}

func LayerIndexes(layers []Layer) (map[string]int, error) {
	indexes := make(map[string]int, len(layers))
	for index, layer := range layers {
		name := normalizeLayer(layer.Name)
		if name != "" {
			if existing, exists := indexes[name]; exists {
				return nil, fmt.Errorf("duplicate layer name %q at indexes %d and %d", name, existing, index)
			}
			indexes[name] = index
		}
	}
	return indexes, nil
}

func normalizeRect(rect Rect) Rect {
	if rect.Min.XMM > rect.Max.XMM {
		rect.Min.XMM, rect.Max.XMM = rect.Max.XMM, rect.Min.XMM
	}
	if rect.Min.YMM > rect.Max.YMM {
		rect.Min.YMM, rect.Max.YMM = rect.Max.YMM, rect.Min.YMM
	}
	return rect
}

func roundMM(value float64) float64 {
	return math.Round(value*1_000_000) / 1_000_000
}

func normalizeLayer(layer string) string {
	return normalizeKey(layer)
}
