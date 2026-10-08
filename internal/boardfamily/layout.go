package boardfamily

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"kicadai/internal/kicadfiles/sexpr"
	"kicadai/internal/routing"
)

// LayoutSpec selects mechanical constraints without changing the circuit/BOM.
// Nil keeps the historical reference bytes and specification fingerprints.
type LayoutSpec struct {
	Version string `json:"version"`
	Size    string `json:"size"`
	Power   string `json:"power_connector"`
	UART    string `json:"uart_connector"`
	GPIO    string `json:"gpio_connector"`
}

type ConnectorPlacement struct {
	Reference string  `json:"reference"`
	XMM       float64 `json:"x_mm"`
	YMM       float64 `json:"y_mm"`
}

type LayoutReview struct {
	Specification LayoutSpec           `json:"specification"`
	WidthMM       float64              `json:"width_mm"`
	HeightMM      float64              `json:"height_mm"`
	Connectors    []ConnectorPlacement `json:"connectors"`
	Conditions    []string             `json:"conditions"`
}

func LayoutOptions() map[string]any {
	return map[string]any{"version": "board-layout-1", "sizes": []string{"120x70", "140x80"},
		"power_connector": []string{"inboard", "west"}, "uart_connector": []string{"inboard", "south"},
		"gpio_connector": []string{"inboard", "east"}, "construction": "two layers, nominal 1.6 mm FR4 / 35 um copper",
		"placement": "reviewed controller/sensor groups retained; connector boundary constraints placed automatically"}
}

func ReviewLayout(s LayoutSpec) (LayoutReview, error) {
	r := LayoutReview{Specification: s, WidthMM: 120, HeightMM: 70,
		Connectors: []ConnectorPlacement{{"J1", 20, 22.5}, {"J2", 65, 50}, {"J3", 78.02, 32}},
		Conditions: []string{"Circuit, BOM, sensor and controller groups retain reviewed geometry; no component substitutions or rotation changes.",
			"Connector tails are automatically routed around retained copper and pads; bounded routing or native validation may reject a configuration.",
			"Declared total bus capacitance still includes all board traces, headers and probes; it is not extracted or measured.",
			"No physical assembly, power integrity, RF, EMC or sensor accuracy claim. Radios remain disabled."}}
	if s.Version != "board-layout-1" {
		return r, errors.New("unsupported layout version")
	}
	switch s.Size {
	case "120x70":
	case "140x80":
		r.WidthMM = 140
		r.HeightMM = 80
	default:
		return r, errors.New("unsupported board size; choose 120x70 or 140x80")
	}
	switch s.Power {
	case "inboard":
	case "west":
		r.Connectors[0].XMM = 5
	default:
		return r, errors.New("unsupported power connector position")
	}
	switch s.UART {
	case "inboard":
	case "south":
		r.Connectors[1].YMM = r.HeightMM - 10
	default:
		return r, errors.New("unsupported UART connector position")
	}
	switch s.GPIO {
	case "inboard":
	case "east":
		r.Connectors[2].XMM = r.WidthMM - 5
	default:
		return r, errors.New("unsupported GPIO connector position")
	}
	return r, nil
}

func DecodeLayout(r io.Reader) (LayoutSpec, error) {
	var s LayoutSpec
	if err := decodeSpecificationJSON(r, &s); err != nil {
		return s, err
	}
	_, err := ReviewLayout(s)
	return s, err
}

func readLayout(dir string) (*LayoutSpec, error) {
	f, err := os.Open(filepath.Join(dir, "layout.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s, err := DecodeLayout(f)
	return &s, errors.Join(err, f.Close())
}

type layoutBranch struct {
	Ref, Pin, Net string
	Old           routing.Point
}
type LayoutReport struct {
	Review          LayoutReview `json:"review"`
	Branches        int          `json:"routed_connector_pads"`
	Segments        int          `json:"added_segments"`
	RetiredSegments int          `json:"retired_anchor_segments"`
	Vias            int          `json:"added_vias"`
	SearchNodes     int          `json:"search_nodes"`
	ManualRepair    bool         `json:"manual_repair"`
}

func coord(v float64) nativeNode {
	return nativeNode{Atom: strconv.FormatFloat(math.Round(v*1e6)/1e6, 'f', -1, 64)}
}
func setXY(n *nativeNode, head string, x, y float64) error {
	for i := range n.Children {
		p := &n.Children[i]
		if p.Head() == head && len(p.Children) >= 3 {
			p.Children[1] = coord(x)
			p.Children[2] = coord(y)
			return nil
		}
	}
	return fmt.Errorf("missing %s coordinate", head)
}

func padPoint(fp, pad nativeNode) (routing.Point, error) {
	x, y, a, err := nativePosition(fp)
	if err != nil {
		return routing.Point{}, err
	}
	px, py, _, err := nativePosition(pad)
	if err != nil {
		return routing.Point{}, err
	}
	t := a * math.Pi / 180
	return routing.Point{XMM: x + px*math.Cos(t) + py*math.Sin(t), YMM: y - px*math.Sin(t) + py*math.Cos(t)}, nil
}

// applyLayout places connector footprints from independent mechanical choices,
// keeps existing local circuit copper, and routes every displaced pad back into
// that copper. No cached successful layout or repaired artifact is substituted.
func applyLayout(root *nativeNode, s LayoutSpec) (LayoutReport, error) {
	r, err := ReviewLayout(s)
	report := LayoutReport{Review: r}
	if err != nil {
		return report, err
	}
	originalTracks := map[string]bool{}
	for _, n := range root.Children {
		if n.Head() == "segment" {
			originalTracks[field(n, "uuid")] = true
		}
	}
	targets := map[string]ConnectorPlacement{}
	for _, c := range r.Connectors {
		targets[c.Reference] = c
	}
	var branches []layoutBranch
	for i := range root.Children {
		fp := &root.Children[i]
		if fp.Head() != "footprint" {
			continue
		}
		ref := property(*fp, "Reference")
		target, ok := targets[ref]
		if !ok {
			continue
		}
		x, y, rotation, err := nativePosition(*fp)
		if err != nil {
			return report, err
		}
		if rotation != 0 {
			return report, errors.New("connector reference must be unrotated")
		}
		if x == target.XMM && y == target.YMM {
			continue
		}
		for j, pad := range fp.Children {
			if pad.Head() != "pad" {
				continue
			}
			net := field(pad, "net")
			if net == "" {
				return report, errors.New("connector pad has no net")
			}
			point, err := padPoint(*fp, pad)
			if err != nil {
				return report, err
			}
			branches = append(branches, layoutBranch{ref, fmt.Sprintf("%s_%d", pad.ListValue(1), j), net, point})
		}
		if err = setXY(fp, "at", target.XMM, target.YMM); err != nil {
			return report, err
		}
	}
	for i := range root.Children {
		n := &root.Children[i]
		if n.Head() != "gr_line" || field(*n, "layer") != "Edge.Cuts" {
			continue
		}
		for _, key := range []string{"start", "end"} {
			p, _ := n.Child(key)
			x, e := number(p, 1)
			if e != nil {
				return report, e
			}
			y, e := number(p, 2)
			if e != nil {
				return report, e
			}
			if x == 120 {
				x = r.WidthMM
			}
			if y == 80 {
				y = r.HeightMM
			}
			if e = setXY(n, key, x, y); e != nil {
				return report, e
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	anchors := map[string]routing.Point{}
	for index, b := range branches {
		id := nativeUUID(fmt.Sprintf("layout/%v/anchor/%d", s, index))
		anchors[id] = b.Old
		n, err := sexpr.Parse([]byte(fmt.Sprintf("(via (at %.6f %.6f) (size 0.6) (drill 0.3) (layers \"F.Cu\" \"B.Cu\") (net %q) (uuid %q))", b.Old.XMM, b.Old.YMM, b.Net, id)))
		if err != nil {
			return report, err
		}
		root.Children = append(root.Children, n)
		report.Vias++
	}
	for index, b := range branches {
		request, err := layoutRoutingRequest(*root, r)
		if err != nil {
			return report, err
		}
		request.Components = append(request.Components, routing.Component{Ref: "ANCHOR", Position: routing.Placement{XMM: b.Old.XMM, YMM: b.Old.YMM, Layer: "F.Cu"}, Pads: []routing.Pad{{Name: "1", Net: b.Net, Type: routing.PadThroughHole, Shape: routing.PadCircle, Size: routing.Size{WidthMM: .6, HeightMM: .6}, Drill: &routing.Drill{DiameterMM: .3}, Layers: []string{"*.Cu"}}}})
		request.Nets = []routing.Net{{Name: b.Net, Endpoints: []routing.Endpoint{{Ref: "ANCHOR", Pin: "1"}, {Ref: b.Ref, Pin: b.Pin}}, Role: routing.NetSignal}}
		if b.Ref == "J1" {
			request.Rules.TraceWidthMM = 1
			request.Nets[0].Role = routing.NetPower
		}
		result := routing.RouteRequestContext(ctx, request)
		if result.Status != routing.StatusRouted {
			return report, fmt.Errorf("bounded connector routing rejected %s.%s: %v", b.Ref, b.Pin, result.Issues)
		}
		for _, route := range result.Routes {
			for j, segment := range route.Segments {
				if strings.EqualFold(segment.Layer, "F.Cu") {
					segment.Layer = "F.Cu"
				} else if strings.EqualFold(segment.Layer, "B.Cu") {
					segment.Layer = "B.Cu"
				} else {
					return report, fmt.Errorf("unexpected routed layer %s", segment.Layer)
				}
				if math.Hypot(segment.Start.XMM-segment.End.XMM, segment.Start.YMM-segment.End.YMM) < .000001 {
					continue
				}
				n, err := sexpr.Parse([]byte(fmt.Sprintf("(segment (start %.6f %.6f) (end %.6f %.6f) (width %.6f) (layer %q) (net %q) (uuid %q))", segment.Start.XMM, segment.Start.YMM, segment.End.XMM, segment.End.YMM, segment.WidthMM, segment.Layer, b.Net, nativeUUID(fmt.Sprintf("layout/%v/%d/segment/%d", s, index, j)))))
				if err != nil {
					return report, err
				}
				root.Children = append(root.Children, n)
				report.Segments++
			}
			for j, v := range route.Vias {
				n, err := sexpr.Parse([]byte(fmt.Sprintf("(via (at %.6f %.6f) (size %.6f) (drill %.6f) (layers \"F.Cu\" \"B.Cu\") (net %q) (uuid %q))", v.At.XMM, v.At.YMM, v.DiameterMM, v.DrillMM, b.Net, nativeUUID(fmt.Sprintf("layout/%v/%d/via/%d", s, index, j)))))
				if err != nil {
					return report, err
				}
				root.Children = append(root.Children, n)
				report.Vias++
			}
			report.SearchNodes += route.SearchNodes
		}
		report.Branches++
	}
	// A connector's old PTH location needs a via only when its retained and new
	// copper use different layers. Omit single-layer anchors in source generation.
	kept := make([]nativeNode, 0, len(root.Children))
	var singleLayerAnchors []routing.Point
	for _, n := range root.Children {
		point, isAnchor := anchors[field(n, "uuid")]
		if n.Head() == "via" && isAnchor {
			layers := map[string]bool{}
			for _, track := range root.Children {
				if track.Head() != "segment" || field(track, "net") != field(n, "net") {
					continue
				}
				a, _ := track.Child("start")
				b, _ := track.Child("end")
				x, _ := number(a, 1)
				y, _ := number(a, 2)
				xx, _ := number(b, 1)
				yy, _ := number(b, 2)
				width, _ := track.Child("width")
				w, _ := number(width, 1)
				if pointSegmentDistance(point, routing.Point{XMM: x, YMM: y}, routing.Point{XMM: xx, YMM: yy}) < .3+w/2-1e-6 {
					layers[field(track, "layer")] = true
				}
			}
			if len(layers) < 2 {
				singleLayerAnchors = append(singleLayerAnchors, point)
				report.Vias--
				continue
			}
		}
		kept = append(kept, n)
	}
	root.Children = kept
	// Existing-copper access can return a connector escape segment already in
	// the reference. Keep one copy and remove only obsolete open anchor stubs.
	seen := map[string]bool{}
	kept = make([]nativeNode, 0, len(root.Children))
	for _, n := range root.Children {
		if n.Head() == "segment" {
			key := trackKey(n)
			if seen[key] {
				report.Segments--
				continue
			}
			seen[key] = true
		}
		kept = append(kept, n)
	}
	root.Children = kept
	for _, point := range singleLayerAnchors {
		var indexes []int
		neighbors := map[string]bool{}
		interior := false
		for i, n := range root.Children {
			if n.Head() != "segment" {
				continue
			}
			a, b := trackPoints(n)
			if pointSegmentDistance(point, a, b) < 1e-6 {
				indexes = append(indexes, i)
				other := b
				if math.Hypot(a.XMM-point.XMM, a.YMM-point.YMM) < 1e-6 {
				} else if math.Hypot(b.XMM-point.XMM, b.YMM-point.YMM) < 1e-6 {
					other = a
				} else {
					interior = true
				}
				neighbors[fmt.Sprintf("%.6f/%.6f", other.XMM, other.YMM)] = true
			}
		}
		if !interior && len(neighbors) == 1 {
			for j := len(indexes) - 1; j >= 0; j-- {
				index := indexes[j]
				root.Children = append(root.Children[:index], root.Children[index+1:]...)
			}
		}
	}
	// Match KiCad's coordinate spelling at generation time. Decimal-only changes
	// must not hide geometry changes in the round-trip gate.
	walkNative(root, func(n *nativeNode) {
		if n.Quoted || n.IsList {
			return
		}
		if x, err := strconv.ParseFloat(n.Atom, 64); err == nil {
			n.Atom = coord(x).Atom
		}
	})
	report.Segments = 0
	for _, n := range root.Children {
		if n.Head() == "segment" {
			id := field(n, "uuid")
			if originalTracks[id] {
				delete(originalTracks, id)
			} else {
				report.Segments++
			}
		}
	}
	report.RetiredSegments = len(originalTracks)
	return report, nil
}

func trackPoints(n nativeNode) (routing.Point, routing.Point) {
	a, _ := n.Child("start")
	b, _ := n.Child("end")
	x, _ := number(a, 1)
	y, _ := number(a, 2)
	xx, _ := number(b, 1)
	yy, _ := number(b, 2)
	return routing.Point{XMM: x, YMM: y}, routing.Point{XMM: xx, YMM: yy}
}
func trackKey(n nativeNode) string {
	a, b := trackPoints(n)
	aa := fmt.Sprintf("%.6f/%.6f", a.XMM, a.YMM)
	bb := fmt.Sprintf("%.6f/%.6f", b.XMM, b.YMM)
	if aa > bb {
		aa, bb = bb, aa
	}
	w, _ := n.Child("width")
	width, _ := number(w, 1)
	return fmt.Sprintf("%s/%s/%.6f/%s/%s", field(n, "net"), field(n, "layer"), width, aa, bb)
}

func pointSegmentDistance(p, a, b routing.Point) float64 {
	x, y := b.XMM-a.XMM, b.YMM-a.YMM
	d := x*x + y*y
	if d == 0 {
		return math.Hypot(p.XMM-a.XMM, p.YMM-a.YMM)
	}
	t := math.Max(0, math.Min(1, ((p.XMM-a.XMM)*x+(p.YMM-a.YMM)*y)/d))
	return math.Hypot(p.XMM-a.XMM-t*x, p.YMM-a.YMM-t*y)
}

func layoutRoutingRequest(root nativeNode, r LayoutReview) (routing.Request, error) {
	request := routing.Request{Board: routing.Board{WidthMM: r.WidthMM, HeightMM: r.HeightMM, MarginMM: .5, Layers: []routing.Layer{{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true}, {Name: "B.Cu", Kind: routing.LayerCopper, Routable: true}}},
		Rules:    routing.Rules{GridMM: .25, TraceWidthMM: .25, ClearanceMM: .2, ViaDiameterMM: .6, ViaDrillMM: .3, ViaClearanceMM: .2, EdgeClearanceMM: .5, MaxSearchNodes: 150000, MaxViasPerNet: 4, PreferLayer: "B.Cu"},
		Strategy: routing.Strategy{Mode: routing.ModeTwoLayer, NetOrder: "input", PreserveExisting: true, TreatZonesAs: routing.ZoneObstacle}}
	for _, fp := range root.ChildrenByHead("footprint") {
		x, y, a, err := nativePosition(fp)
		if err != nil {
			return request, err
		}
		component := routing.Component{Ref: property(fp, "Reference"), Position: routing.Placement{XMM: x, YMM: y, RotationDeg: a, Layer: "F.Cu"}, Fixed: true}
		for index, pad := range fp.Children {
			if pad.Head() != "pad" || pad.ListValue(1) == "" {
				continue
			}
			px, py, pa, err := nativePosition(pad)
			if err != nil {
				return request, err
			}
			size, _ := pad.Child("size")
			w, err := number(size, 1)
			if err != nil {
				return request, err
			}
			h, err := number(size, 2)
			if err != nil {
				return request, err
			}
			if pad.ListValue(3) == "custom" {
				// Conservatively bound the reviewed sensor exposed-pad polygon,
				// whose anchor size alone does not describe all its copper.
				primitives, ok := pad.Child("primitives")
				if !ok {
					return request, errors.New("custom copper pad missing primitives")
				}
				walkNative(&primitives, func(n *nativeNode) {
					if n.Head() == "xy" {
						x, e := number(*n, 1)
						if e != nil {
							err = e
							return
						}
						y, e := number(*n, 2)
						if e != nil {
							err = e
							return
						}
						w = math.Max(w, 2*math.Abs(x))
						h = math.Max(h, 2*math.Abs(y))
					}
				})
				if err != nil {
					return request, err
				}
			}
			p := routing.Pad{Name: fmt.Sprintf("%s_%d", pad.ListValue(1), index), Net: field(pad, "net"), Position: routing.Point{XMM: px, YMM: py}, RotationDeg: pa - a, Type: routing.PadSMD, Shape: routing.PadRect, Size: routing.Size{WidthMM: w, HeightMM: h}}
			if pad.ListValue(2) == "thru_hole" {
				p.Type = routing.PadThroughHole
				drill, _ := pad.Child("drill")
				d, err := number(drill, 1)
				if err != nil {
					return request, err
				}
				p.Drill = &routing.Drill{DiameterMM: d}
			}
			switch pad.ListValue(3) {
			case "circle":
				p.Shape = routing.PadCircle
			case "oval":
				p.Shape = routing.PadOval
			case "roundrect":
				p.Shape = routing.PadRoundedRect
			}
			layers, _ := pad.Child("layers")
			for _, layer := range layers.Children[1:] {
				if layer.String == "*.Cu" || layer.String == "F.Cu" || layer.String == "B.Cu" {
					p.Layers = append(p.Layers, layer.String)
				}
			}
			component.Pads = append(component.Pads, p)
		}
		request.Components = append(request.Components, component)
	}
	for _, n := range root.Children {
		switch n.Head() {
		case "segment":
			a, _ := n.Child("start")
			b, _ := n.Child("end")
			x, err := number(a, 1)
			if err != nil {
				return request, err
			}
			y, err := number(a, 2)
			if err != nil {
				return request, err
			}
			xx, err := number(b, 1)
			if err != nil {
				return request, err
			}
			yy, err := number(b, 2)
			if err != nil {
				return request, err
			}
			width, _ := n.Child("width")
			w, err := number(width, 1)
			if err != nil {
				return request, err
			}
			request.Existing = append(request.Existing, routing.ExistingCopper{Kind: routing.CopperSegment, Net: field(n, "net"), Layer: field(n, "layer"), Fixed: true, Centerline: []routing.Point{{XMM: x, YMM: y}, {XMM: xx, YMM: yy}}, Geometry: routing.Shape{Rect: &routing.Rect{Min: routing.Point{XMM: math.Min(x, xx) - w/2, YMM: math.Min(y, yy) - w/2}, Max: routing.Point{XMM: math.Max(x, xx) + w/2, YMM: math.Max(y, yy) + w/2}}}})
		case "via":
			x, y, _, err := nativePosition(n)
			if err != nil {
				return request, err
			}
			size, _ := n.Child("size")
			d, err := number(size, 1)
			if err != nil {
				return request, err
			}
			request.Existing = append(request.Existing, routing.ExistingCopper{Kind: routing.CopperVia, Net: field(n, "net"), Layer: "F.Cu", Fixed: true, Geometry: routing.Shape{Rect: &routing.Rect{Min: routing.Point{XMM: x - d/2, YMM: y - d/2}, Max: routing.Point{XMM: x + d/2, YMM: y + d/2}}}})
		}
	}
	return request, nil
}
