package schematiclayout

import "kicadai/internal/kicadfiles"

func Route(request Request, result Result) Result {
	request = Classify(request)
	rules := normalizeRules(request.Rules)
	anchors := pinAnchors(result.Components)
	for _, net := range request.Nets {
		if len(net.Endpoints) < 2 {
			continue
		}
		if shouldUseLabels(net, anchors, rules) {
			for _, endpoint := range net.Endpoints {
				if anchor, ok := anchors[endpoint]; ok {
					result.Labels = append(result.Labels, Label{NetName: net.Name, Text: net.Name, Position: anchor})
				}
			}
			continue
		}
		startIndex, start, ok := firstRoutableEndpoint(net, anchors)
		if !ok {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityWarning, Code: "missing_pin_anchor", NetName: net.Name, Message: "net has no routable endpoint anchors"})
			continue
		}
		for _, endpoint := range net.Endpoints[startIndex+1:] {
			end, ok := anchors[endpoint]
			if !ok {
				result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityWarning, Code: "missing_pin_anchor", NetName: net.Name, Ref: endpoint.Ref, Message: "net endpoint has no pin anchor"})
				continue
			}
			result.Wires = append(result.Wires, orthogonalSegments(net.Name, start, end)...)
			start = end
		}
	}
	result = Validate(result, request)
	return NormalizeResult(result, rules)
}

func firstRoutableEndpoint(net Net, anchors map[Endpoint]kicadfiles.Point) (int, kicadfiles.Point, bool) {
	for index, endpoint := range net.Endpoints {
		if anchor, ok := anchors[endpoint]; ok {
			return index, anchor, true
		}
	}
	return -1, kicadfiles.Point{}, false
}

func Layout(request Request) Result {
	placed := Place(request)
	return Route(request, placed)
}

func pinAnchors(components []PlacedComponent) map[Endpoint]kicadfiles.Point {
	anchors := map[Endpoint]kicadfiles.Point{}
	for _, component := range components {
		if len(component.Pins) == 0 {
			anchors[Endpoint{Ref: component.Ref, Pin: "1"}] = component.PlacedAt
			continue
		}
		for _, pin := range component.Pins {
			anchors[Endpoint{Ref: component.Ref, Pin: pin.Number}] = kicadfiles.Point{
				X: component.PlacedAt.X + pin.At.X,
				Y: component.PlacedAt.Y + pin.At.Y,
			}
		}
	}
	return anchors
}

func shouldUseLabels(net Net, anchors map[Endpoint]kicadfiles.Point, rules Rules) bool {
	if !rules.LabelFallbackEnabled || len(net.Endpoints) < 2 {
		return false
	}
	role := normalizeRole(net.Role)
	if net.PreferredLabels || len(net.Endpoints) > 2 || containsNormalizedRole(role, "power", "ground", "bus", "negative_rail") {
		return true
	}
	startIndex, start, ok := firstRoutableEndpoint(net, anchors)
	if !ok {
		return false
	}
	for _, endpoint := range net.Endpoints[startIndex+1:] {
		end, ok := anchors[endpoint]
		if !ok {
			continue
		}
		if manhattan(start, end) > rules.LongWireThreshold {
			return true
		}
	}
	return false
}

func orthogonalSegments(netName string, start, end kicadfiles.Point) []WireSegment {
	if start == end {
		return nil
	}
	if start.X == end.X || start.Y == end.Y {
		return []WireSegment{{NetName: netName, From: start, To: end}}
	}
	mid := kicadfiles.Point{X: start.X + (end.X-start.X)/2, Y: start.Y}
	midDrop := kicadfiles.Point{X: mid.X, Y: end.Y}
	return nonZeroSegments([]WireSegment{
		{NetName: netName, From: start, To: mid},
		{NetName: netName, From: mid, To: midDrop},
		{NetName: netName, From: midDrop, To: end},
	})
}

func manhattan(first, second kicadfiles.Point) kicadfiles.IU {
	dx := first.X - second.X
	if dx < 0 {
		dx = -dx
	}
	dy := first.Y - second.Y
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}

func nonZeroSegments(segments []WireSegment) []WireSegment {
	filtered := segments[:0]
	for _, segment := range segments {
		if segment.From != segment.To {
			filtered = append(filtered, segment)
		}
	}
	return filtered
}
