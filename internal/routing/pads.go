package routing

import (
	"fmt"
	"sort"
	"strings"

	"kicadai/internal/reports"
)

type PadAccess struct {
	Pads         map[endpointID]Pad
	AccessPoints map[endpointID][]AccessPoint
	Issues       []reports.Issue
}

type AccessPoint struct {
	Endpoint Endpoint `json:"endpoint"`
	Point    Point    `json:"point"`
	Layer    string   `json:"layer"`
}

// BuildPadAccess builds normalized pad and access-point lookup tables for a
// routing request without mutating the caller's request.
func BuildPadAccess(request Request) PadAccess {
	request = cloneRequest(request)
	NormalizeRequest(&request)
	access := PadAccess{
		Pads:         map[endpointID]Pad{},
		AccessPoints: map[endpointID][]AccessPoint{},
	}
	routableLayers := routableLayerNames(request.Board.Layers)
	for _, component := range request.Components {
		ref := normalizeKey(component.Ref)
		for _, pad := range component.Pads {
			pin := normalizeKey(pad.Name)
			if ref == "" || pin == "" {
				access.Issues = append(access.Issues, reports.Issue{
					Code:     reports.CodeInvalidArgument,
					Severity: reports.SeverityBlocked,
					Path:     fmt.Sprintf("components[%s].pads[%s]", component.Ref, pad.Name),
					Message:  "component reference and pad name are required for routing access",
					Refs:     []string{component.Ref},
				})
				continue
			}
			key := endpointKey(ref, pin)
			if existing, exists := access.Pads[key]; exists {
				access.Issues = append(access.Issues, reports.Issue{
					Code:     reports.CodeInvalidArgument,
					Severity: reports.SeverityBlocked,
					Path:     fmt.Sprintf("components[%s].pads[%s]", component.Ref, pad.Name),
					Message:  fmt.Sprintf("duplicate normalized pad endpoint conflicts with %s.%s", existing.Ref, existing.Name),
					Refs:     []string{component.Ref, existing.Ref},
				})
				continue
			}
			pad.Ref = component.Ref
			absolutePad := pad
			absolutePad.Position = absolutePadPoint(component, pad.Position)
			access.Pads[key] = absolutePad
			points, issues := accessPointsForPad(component.Ref, absolutePad, routableLayers)
			access.AccessPoints[key] = points
			access.Issues = append(access.Issues, issues...)
		}
	}
	sortIssues(access.Issues)
	return access
}

// AccessPointsForEndpoint returns a defensive copy of access points for the
// requested endpoint using the same normalization as route validation.
func AccessPointsForEndpoint(access PadAccess, endpoint Endpoint) ([]AccessPoint, bool) {
	points, ok := access.AccessPoints[endpointKey(normalizeKey(endpoint.Ref), normalizeKey(endpoint.Pin))]
	if !ok || len(points) == 0 {
		return nil, false
	}
	return append([]AccessPoint(nil), points...), true
}

func accessPointsForPad(ref string, pad Pad, routableLayers []string) ([]AccessPoint, []reports.Issue) {
	var issues []reports.Issue
	shape := PadShape(strings.ToLower(strings.TrimSpace(string(pad.Shape))))
	if shape != "" && shape != PadCircle && shape != PadOval && shape != PadRect && shape != PadRoundedRect {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityWarning,
			Path:       fmt.Sprintf("components[%s].pads[%s].shape", ref, pad.Name),
			Message:    "unsupported pad shape approximated by center access point",
			Refs:       []string{ref},
			Suggestion: "add explicit supported pad geometry before fabrication routing",
		})
	}
	layers := padAccessLayers(pad, routableLayers)
	points := make([]AccessPoint, 0, len(layers))
	for _, layer := range layers {
		points = append(points, AccessPoint{
			Endpoint: Endpoint{Ref: ref, Pin: pad.Name},
			Point:    pad.Position,
			Layer:    layer,
		})
	}
	if len(points) == 0 {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     fmt.Sprintf("components[%s].pads[%s].layers", ref, pad.Name),
			Message:  "pad has no routable copper access layer",
			Refs:     []string{ref},
			Nets:     []string{pad.Net},
		})
	}
	return points, issues
}

func padAccessLayers(pad Pad, routableLayers []string) []string {
	layers := make([]string, 0, len(routableLayers))
	for _, layer := range routableLayers {
		if padAllowsLayer(pad, layer) {
			layers = append(layers, layer)
		}
	}
	return layers
}

func padAllowsLayer(pad Pad, routableLayer string) bool {
	if pad.Type == PadThroughHole {
		return true
	}
	for _, layer := range pad.Layers {
		layer = normalizeLayer(layer)
		if layer == "*.CU" || layer == routableLayer {
			return true
		}
	}
	return false
}

func routableLayerNames(layers []Layer) []string {
	names := []string{}
	seen := map[string]struct{}{}
	for _, layer := range layers {
		if layer.Routable {
			name := normalizeLayer(layer.Name)
			if name != "" {
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				names = append(names, name)
			}
		}
	}
	return names
}

func sortIssues(issues []reports.Issue) {
	sort.SliceStable(issues, func(i int, j int) bool {
		if issues[i].Path != issues[j].Path {
			return issues[i].Path < issues[j].Path
		}
		return issues[i].Message < issues[j].Message
	})
}

func cloneRequest(request Request) Request {
	request.Board.Layers = append([]Layer(nil), request.Board.Layers...)
	request.Board.Outline = append([]Shape(nil), request.Board.Outline...)
	request.Components = append([]Component(nil), request.Components...)
	for index := range request.Components {
		request.Components[index].Pads = append([]Pad(nil), request.Components[index].Pads...)
		for padIndex := range request.Components[index].Pads {
			request.Components[index].Pads[padIndex].Layers = append([]string(nil), request.Components[index].Pads[padIndex].Layers...)
		}
	}
	request.Nets = append([]Net(nil), request.Nets...)
	for index := range request.Nets {
		request.Nets[index].Endpoints = append([]Endpoint(nil), request.Nets[index].Endpoints...)
	}
	request.Obstacles = append([]Obstacle(nil), request.Obstacles...)
	request.Existing = append([]ExistingCopper(nil), request.Existing...)
	if request.Rules.NetClasses != nil {
		classes := make(map[string]NetClass, len(request.Rules.NetClasses))
		for name, netClass := range request.Rules.NetClasses {
			classes[name] = netClass
		}
		request.Rules.NetClasses = classes
	}
	return request
}
