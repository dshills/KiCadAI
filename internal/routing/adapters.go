package routing

import (
	"fmt"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
)

func RequestFromPlacement(placementRequest placement.Request, placementResult placement.Result) (Request, []reports.Issue) {
	issues := []reports.Issue{}
	placements := map[string]placement.PlacementResult{}
	for _, placed := range placementResult.Placements {
		if placed.Ref != "" && placed.Reason == "" {
			placements[normalizeKey(placed.Ref)] = placed
		}
	}
	request := Request{
		Board: Board{
			Origin:   Point{XMM: placementRequest.Board.Origin.XMM, YMM: placementRequest.Board.Origin.YMM},
			WidthMM:  placementRequest.Board.WidthMM,
			HeightMM: placementRequest.Board.HeightMM,
			MarginMM: placementRequest.Board.MarginMM,
			Layers: []Layer{
				{Name: "F.Cu", Kind: LayerCopper, Routable: true},
				{Name: "B.Cu", Kind: LayerCopper, Routable: true},
			},
		},
		Rules:    DefaultRules(),
		Strategy: Strategy{Mode: ModeTwoLayer, TreatZonesAs: ZoneObstacle, AllowPartial: true},
		Seed:     placementRequest.Seed,
	}
	if !placementRequest.Rules.AllowBackLayer {
		request.Rules.AllowBackLayer = boolPtr(false)
	}
	for _, component := range placementRequest.Components {
		placed, ok := placements[normalizeKey(component.Ref)]
		if !ok && component.Position == nil {
			issues = append(issues, reports.Issue{
				Code:     reports.CodePlacementOutsideBoard,
				Severity: reports.SeverityBlocked,
				Path:     "components." + component.Ref,
				Message:  "component has no placement for routing",
				Refs:     []string{component.Ref},
			})
			continue
		}
		position := placement.Placement{}
		if ok {
			position = placed.Position
		} else {
			position = *component.Position
		}
		if len(component.Pads) == 0 {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityBlocked,
				Path:     "components." + component.Ref + ".pads",
				Message:  "component has no footprint pad summaries for routing",
				Refs:     []string{component.Ref},
			})
			continue
		}
		request.Components = append(request.Components, Component{
			Ref:       component.Ref,
			Footprint: firstNonEmpty(placed.FootprintID, component.FootprintID),
			Position:  Placement{XMM: position.XMM, YMM: position.YMM, RotationDeg: position.RotationDeg, Layer: firstNonEmpty(position.Layer, "F.Cu")},
			Pads:      routingPadsFromPlacement(component, firstNonEmpty(position.Layer, "F.Cu")),
			Fixed:     component.Fixed || placed.Fixed,
		})
	}
	for _, net := range placementRequest.Nets {
		request.Nets = append(request.Nets, Net{
			Name:      net.Name,
			Endpoints: routingEndpointsFromPlacement(net.Endpoints),
			Role:      routingNetRole(net.Role),
			Priority:  net.Weight,
		})
	}
	for _, keepout := range placementRequest.Keepouts {
		layers := keepout.Layers
		if len(layers) == 0 {
			layers = []string{"F.Cu", "B.Cu"}
		}
		for _, layer := range layers {
			rect := Rect{
				Min: Point{XMM: keepout.Bounds.Min.XMM, YMM: keepout.Bounds.Min.YMM},
				Max: Point{XMM: keepout.Bounds.Max.XMM, YMM: keepout.Bounds.Max.YMM},
			}
			request.Obstacles = append(request.Obstacles, Obstacle{
				Kind:     ObstacleKeepout,
				Layer:    layer,
				Geometry: Shape{Rect: &rect},
				Source:   fmt.Sprintf("keepout:%s", keepout.ID),
			})
		}
	}
	return request, issues
}

func routingPadsFromPlacement(component placement.Component, layer string) []Pad {
	pads := make([]Pad, 0, len(component.Pads))
	layer = firstNonEmpty(layer, "F.Cu")
	for _, pad := range component.Pads {
		pads = append(pads, Pad{
			Ref:      component.Ref,
			Name:     pad.Name,
			Net:      pad.Net,
			Position: Point{XMM: pad.XMM, YMM: pad.YMM},
			Shape:    PadRect,
			Type:     PadSMD,
			Size:     Size{WidthMM: positiveOrDefault(pad.WidthMM, 1), HeightMM: positiveOrDefault(pad.HeightMM, 1)},
			Layers:   []string{layer},
		})
	}
	return pads
}

func routingEndpointsFromPlacement(endpoints []placement.Endpoint) []Endpoint {
	routed := make([]Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		routed = append(routed, Endpoint{Ref: endpoint.Ref, Pin: endpoint.Pin})
	}
	return routed
}

func routingNetRole(role placement.NetRole) NetRole {
	switch role {
	case placement.NetPower:
		return NetPower
	case placement.NetGround:
		return NetGround
	case placement.NetSignal:
		return NetSignal
	default:
		return NetUnknown
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func positiveOrDefault(value float64, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}
