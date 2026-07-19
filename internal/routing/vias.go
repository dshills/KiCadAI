package routing

import (
	"fmt"
	"math"
)

func BuildViasFromPath(path GridPath, rules Rules) []Via {
	rules = normalizedSearchRules(rules)
	diameter := roundMM(rules.ViaDiameterMM)
	drill := roundMM(rules.ViaDrillMM)
	if diameter <= 0 || math.IsNaN(diameter) || math.IsInf(diameter, 0) {
		diameter = roundMM(DefaultRules().ViaDiameterMM)
	}
	if drill <= 0 || math.IsNaN(drill) || math.IsInf(drill, 0) || drill >= diameter {
		drill = roundMM(DefaultRules().ViaDrillMM)
	}
	if drill <= 0 || drill >= diameter {
		drill = roundMM(diameter / 2)
	}
	vias := []Via{}
	seen := map[string]struct{}{}
	for index := 1; index < len(path.Coordinates) && index < len(path.Points); index++ {
		previous := path.Coordinates[index-1]
		current := path.Coordinates[index]
		if previous.Layer == current.Layer {
			continue
		}
		point := viaPointForTransition(path, index)
		key := viaKey(point, previous.Layer, current.Layer)
		if _, exists := seen[key]; exists {
			continue
		}
		layers := viaLayers(path, previous.Layer, current.Layer)
		if len(layers) != 2 || layers[0] == layers[1] {
			continue
		}
		seen[key] = struct{}{}
		vias = append(vias, Via{
			Net:        path.Net,
			At:         point,
			DiameterMM: diameter,
			DrillMM:    drill,
			Layers:     layers,
		})
	}
	return vias
}

func viaPointForTransition(path GridPath, index int) Point {
	// Endpoint alignment may move the terminal path point from the searched
	// grid coordinate onto off-grid pad copper. Keep the physical via at the
	// clearance-checked grid point; the segment builder emits a short connector
	// on the terminal pad layer.
	if index == len(path.Points)-1 && index > 0 {
		return roundPoint(path.Points[index-1])
	}
	return roundPoint(path.Points[index])
}

func viaLayers(path GridPath, first int, second int) []string {
	firstName := path.LayerNames[first]
	secondName := path.LayerNames[second]
	if firstName == "" {
		return nil
	}
	if secondName == "" {
		return nil
	}
	if firstName <= secondName {
		return []string{firstName, secondName}
	}
	return []string{secondName, firstName}
}

func viaKey(point Point, first int, second int) string {
	if first > second {
		first, second = second, first
	}
	return fmt.Sprintf("%.6f,%.6f,%d,%d", point.XMM, point.YMM, first, second)
}
