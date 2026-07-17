package libraryresolver

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
	"kicadai/internal/reports"
)

const maxFootprintLibraryBytes int64 = 64 << 20
const maxFootprintNestedNumericDepth = 500

func IndexFootprints(inventory LibraryInventory) (map[string]FootprintRecord, []reports.Issue) {
	return IndexFootprintsContext(context.Background(), inventory)
}

func IndexFootprintsContext(ctx context.Context, inventory LibraryInventory) (map[string]FootprintRecord, []reports.Issue) {
	records := make(map[string]FootprintRecord, len(inventory.FootprintFiles))
	var issues []reports.Issue
	if issue, ok := contextIssue(ctx); ok {
		return records, []reports.Issue{issue}
	}
	results := parseFootprintFiles(ctx, inventory.FootprintFiles)
	for _, result := range results {
		issues = append(issues, result.issues...)
		if !result.ok {
			continue
		}
		if _, exists := records[result.record.FootprintID]; exists {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityWarning,
				Path:     result.record.Path,
				Message:  "duplicate footprint ID " + result.record.FootprintID,
			})
			continue
		}
		records[result.record.FootprintID] = result.record
	}
	if issue, ok := contextIssue(ctx); ok {
		issues = append(issues, issue)
	}
	return records, issues
}

type footprintParseResult struct {
	record FootprintRecord
	issues []reports.Issue
	ok     bool
}

func parseFootprintFiles(ctx context.Context, files []LibraryFile) []footprintParseResult {
	return parallelMap(ctx, len(files), func(index int) footprintParseResult {
		record, issues, ok := parseFootprintFile(files[index])
		return footprintParseResult{record: record, issues: issues, ok: ok}
	})
}

func ResolveFootprint(index LibraryIndex, footprintID string) (FootprintRecord, bool) {
	if index.Footprints == nil {
		return FootprintRecord{}, false
	}
	record, ok := index.Footprints[footprintID]
	return record, ok
}

func parseFootprintFile(file LibraryFile) (FootprintRecord, []reports.Issue, bool) {
	sourcePath := filepath.FromSlash(file.Path)
	info, err := os.Stat(sourcePath)
	if err != nil {
		return FootprintRecord{}, []reports.Issue{parseIssue(file.Path, err.Error())}, false
	}
	if info.Size() > maxFootprintLibraryBytes {
		return FootprintRecord{}, []reports.Issue{parseIssue(file.Path, "footprint library exceeds 64 MiB parser limit")}, false
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return FootprintRecord{}, []reports.Issue{parseIssue(file.Path, err.Error())}, false
	}
	root, err := sexpr.Parse(data)
	if err != nil {
		return FootprintRecord{}, []reports.Issue{parseIssue(file.Path, err.Error())}, false
	}
	if root.Head() != "footprint" {
		return FootprintRecord{}, []reports.Issue{parseIssue(file.Path, "expected footprint root, got "+root.Head())}, false
	}
	if len(root.Children) < 2 || strings.TrimSpace(root.ListValue(1)) == "" {
		return FootprintRecord{}, []reports.Issue{parseIssue(file.Path, "footprint without name")}, false
	}
	record, issues := readLibraryFootprint(file, root, root.ListValue(1))
	return record, issues, true
}

func readLibraryFootprint(file LibraryFile, node sexpr.ParsedNode, name string) (FootprintRecord, []reports.Issue) {
	record := FootprintRecord{
		FootprintID:     file.LibraryNickname + ":" + name,
		LibraryNickname: file.LibraryNickname,
		Name:            name,
		Path:            file.Path,
		Properties:      map[string]string{},
		Raw:             strings.Clone(strings.TrimSpace(node.Raw)),
	}
	var issues []reports.Issue
	if descr, ok := node.Child("descr"); ok && len(descr.Children) > 1 {
		record.Description = descr.ListValue(1)
	}
	if tags, ok := node.Child("tags"); ok && len(tags.Children) > 1 {
		record.Tags = strings.Fields(tags.ListValue(1))
	}
	if attr, ok := node.Child("attr"); ok {
		record.Attributes = listValues(attr, 1)
	}
	for _, property := range node.ChildrenByHead("property") {
		if len(property.Children) >= 3 {
			if _, hasAt := property.Child("at"); hasAt {
				position, _ := readNamedPointOK(property, "at")
				layer := ""
				if layerNode, ok := property.Child("layer"); ok {
					layer = layerNode.ListValue(1)
				}
				_, hidden := property.Child("hide")
				record.CustomProperties = append(record.CustomProperties, FootprintProperty{Name: property.ListValue(1), Value: property.ListValue(2), Position: position, Layer: layer, Hide: hidden})
			} else {
				record.Properties[property.ListValue(1)] = property.ListValue(2)
			}
		}
	}
	bounds := newBounds()
	courtyardBounds := newBounds()
	for _, child := range node.Children {
		switch child.Head() {
		case "pad":
			pad, padIssues := readLibraryPad(file.Path, child)
			issues = append(issues, padIssues...)
			record.Pads = append(record.Pads, pad)
			bounds.includePad(pad)
		case "fp_text":
			text := readLibraryFootprintText(child)
			record.Texts = append(record.Texts, text)
			record.GraphicsSummary.TextCount++
			record.GraphicsSummary.markLayer(text.Layer)
			if point, ok := readNamedPointOK(child, "at"); ok {
				bounds.includePoint(point)
			}
		case "fp_line":
			courtyard := markGraphicLayer(&record.GraphicsSummary, child)
			start, startOK := readNamedPointOK(child, "start")
			end, endOK := readNamedPointOK(child, "end")
			if startOK && endOK {
				if appendLibraryFootprintGraphic(&record, "line", child, func(graphic *FootprintGraphic) {
					graphic.Start = pointPtr(start)
					graphic.End = pointPtr(end)
				}) {
					record.GraphicsSummary.LineCount++
				}
			}
			if startOK {
				bounds.includePoint(start)
			}
			if endOK {
				bounds.includePoint(end)
			}
			if courtyard {
				if startOK {
					courtyardBounds.includePoint(start)
				}
				if endOK {
					courtyardBounds.includePoint(end)
				}
			}
		case "fp_rect":
			courtyard := markGraphicLayer(&record.GraphicsSummary, child)
			start, startOK := readNamedPointOK(child, "start")
			end, endOK := readNamedPointOK(child, "end")
			if startOK && endOK {
				if appendLibraryFootprintGraphic(&record, "rect", child, func(graphic *FootprintGraphic) {
					graphic.Start = pointPtr(start)
					graphic.End = pointPtr(end)
				}) {
					record.GraphicsSummary.PolygonCount++
				}
			}
			if startOK {
				bounds.includePoint(start)
			}
			if endOK {
				bounds.includePoint(end)
			}
			if courtyard {
				if startOK {
					courtyardBounds.includePoint(start)
				}
				if endOK {
					courtyardBounds.includePoint(end)
				}
			}
		case "fp_circle":
			courtyard := markGraphicLayer(&record.GraphicsSummary, child)
			center, centerOK := readNamedPointOK(child, "center")
			end, endOK := readNamedPointOK(child, "end")
			if centerOK && endOK {
				if appendLibraryFootprintGraphic(&record, "circle", child, func(graphic *FootprintGraphic) {
					graphic.Center = pointPtr(center)
					graphic.End = pointPtr(end)
				}) {
					record.GraphicsSummary.CircleCount++
				}
			}
			if centerOK && endOK {
				bounds.includeCircle(center, end)
				if courtyard {
					courtyardBounds.includeCircle(center, end)
				}
			}
		case "fp_arc":
			courtyard := markGraphicLayer(&record.GraphicsSummary, child)
			start, mid, end, arcOK := readLibraryArcPointsOK(child)
			if arcOK {
				if appendLibraryFootprintGraphic(&record, "arc", child, func(graphic *FootprintGraphic) {
					graphic.Start = pointPtr(start)
					graphic.Mid = pointPtr(mid)
					graphic.End = pointPtr(end)
				}) {
					record.GraphicsSummary.ArcCount++
				}
			}
			if arcOK {
				bounds.includeArc(start, mid, end)
				if courtyard {
					courtyardBounds.includeArc(start, mid, end)
				}
			}
		case "fp_poly":
			courtyard := markGraphicLayer(&record.GraphicsSummary, child)
			points, pointIssues := readPolyPoints(file.Path, child)
			issues = append(issues, pointIssues...)
			if len(points) >= 3 {
				if appendLibraryFootprintGraphic(&record, "poly", child, func(graphic *FootprintGraphic) {
					graphic.Points = points
				}) {
					record.GraphicsSummary.PolygonCount++
				}
			}
			includeFootprintGraphicPoints(&bounds, &courtyardBounds, points, courtyard)
		case "fp_curve":
			courtyard := markGraphicLayer(&record.GraphicsSummary, child)
			points, pointIssues := readPolyPoints(file.Path, child)
			issues = append(issues, pointIssues...)
			if len(points) >= 2 {
				if appendLibraryFootprintGraphic(&record, "curve", child, func(graphic *FootprintGraphic) {
					graphic.Points = points
				}) {
					record.GraphicsSummary.CurveCount++
				}
			}
			includeFootprintGraphicPoints(&bounds, &courtyardBounds, points, courtyard)
		case "model":
			if len(child.Children) > 1 {
				record.Models = append(record.Models, child.ListValue(1))
			}
		}
	}
	sort.Strings(record.Attributes)
	sort.Strings(record.Models)
	record.BoundingBox = bounds.box()
	record.CourtyardBox = courtyardBounds.box()
	record.SearchText = buildFootprintSearchText(record)
	return record, issues
}

func readLibraryPad(path string, node sexpr.ParsedNode) (FootprintPad, []reports.Issue) {
	var issues []reports.Issue
	pad := FootprintPad{Raw: strings.TrimSpace(node.Raw)}
	if len(node.Children) < 2 || strings.TrimSpace(node.ListValue(1)) == "" {
		issues = append(issues, parseIssue(path, "pad without name"))
	} else {
		pad.Name = node.ListValue(1)
	}
	if len(node.Children) > 2 {
		pad.Type = node.ListValue(2)
	}
	if len(node.Children) > 3 {
		pad.Shape = node.ListValue(3)
	}
	if at, ok := node.Child("at"); ok {
		var pointOK bool
		pad.Position, pointOK = readPointValuesOK(at, 1)
		if !pointOK {
			issues = append(issues, parseIssue(path, "at requires numeric x and y coordinates"))
		}
		if len(at.Children) > 3 {
			var rotationOK bool
			pad.Rotation, rotationOK = at.FloatValue(3)
			if !rotationOK {
				issues = append(issues, parseIssue(path, "at requires numeric rotation"))
			}
		}
	}
	if size, ok := node.Child("size"); ok {
		var sizeOK bool
		pad.Size, sizeOK = readPointValuesOK(size, 1)
		if !sizeOK {
			issues = append(issues, parseIssue(path, "size requires numeric x and y coordinates"))
		}
	}
	if drill, ok := node.Child("drill"); ok {
		var drillOK bool
		pad.Drill, drillOK = firstNumericMM(drill, 1)
		if !drillOK {
			issues = append(issues, parseIssue(path, "drill requires numeric size"))
		}
	}
	if layers, ok := node.Child("layers"); ok {
		for _, layer := range listValues(layers, 1) {
			pad.Layers = append(pad.Layers, kicadfiles.BoardLayer(layer))
		}
	}
	if pinFunction, ok := node.Child("pinfunction"); ok && len(pinFunction.Children) > 1 {
		pad.PinFunction = pinFunction.ListValue(1)
	}
	if pinType, ok := node.Child("pintype"); ok && len(pinType.Children) > 1 {
		pad.PinType = pinType.ListValue(1)
	}
	if ratio, ok := node.Child("roundrect_rratio"); ok {
		var ratioOK bool
		pad.RoundRectR, ratioOK = ratio.FloatValue(1)
		if !ratioOK {
			issues = append(issues, parseIssue(path, "roundrect_rratio requires numeric value"))
		}
	}
	return pad, issues
}

func readLibraryFootprintText(node sexpr.ParsedNode) FootprintText {
	text := FootprintText{}
	if len(node.Children) > 1 {
		text.Kind = node.ListValue(1)
	}
	if len(node.Children) > 2 {
		text.Text = node.ListValue(2)
	}
	text.Position = readNamedPoint(node, "at")
	if layer, ok := node.Child("layer"); ok && len(layer.Children) > 1 {
		text.Layer = layer.ListValue(1)
	}
	return text
}

func readLibraryFootprintGraphicBase(kind string, node sexpr.ParsedNode) FootprintGraphic {
	return FootprintGraphic{
		Kind:       kind,
		Layer:      graphicLayer(node),
		StrokeType: readStrokeType(node),
		Width:      readStrokeWidth(node),
		Fill:       readFill(node),
	}
}

func appendLibraryFootprintGraphic(record *FootprintRecord, kind string, node sexpr.ParsedNode, configure func(*FootprintGraphic)) bool {
	graphic := readLibraryFootprintGraphicBase(kind, node)
	if strings.TrimSpace(graphic.Layer) == "" {
		return false
	}
	configure(&graphic)
	record.Graphics = append(record.Graphics, graphic)
	return true
}

func pointPtr(point kicadfiles.Point) *kicadfiles.Point {
	return &point
}

func readLibraryArcPointsOK(node sexpr.ParsedNode) (kicadfiles.Point, kicadfiles.Point, kicadfiles.Point, bool) {
	start, startOK := readNamedPointOK(node, "start")
	mid, midOK := readNamedPointOK(node, "mid")
	end, endOK := readNamedPointOK(node, "end")
	if startOK && midOK && endOK {
		return start, mid, end, true
	}
	angleNode, angleOK := node.Child("angle")
	if !angleOK {
		return kicadfiles.Point{}, kicadfiles.Point{}, kicadfiles.Point{}, false
	}
	center, centerOK := readNamedPointOK(node, "start")
	legacyStart, legacyStartOK := readNamedPointOK(node, "end")
	if !centerOK || !legacyStartOK {
		return kicadfiles.Point{}, kicadfiles.Point{}, kicadfiles.Point{}, false
	}
	angle, angleValueOK := angleNode.FloatValue(1)
	if !angleValueOK {
		return kicadfiles.Point{}, kicadfiles.Point{}, kicadfiles.Point{}, false
	}
	// Legacy KiCad arcs store center in start and arc start in end. Convert to
	// the modern start/mid/end shape used by pcb.FootprintGraphic rendering.
	legacyMid := rotatePointAround(center, legacyStart, -angle/2)
	legacyEnd := rotatePointAround(center, legacyStart, -angle)
	return legacyStart, legacyMid, legacyEnd, true
}

func rotatePointAround(center kicadfiles.Point, point kicadfiles.Point, degrees float64) kicadfiles.Point {
	radians := degrees * math.Pi / 180
	sin, cos := math.Sin(radians), math.Cos(radians)
	dx := float64(point.X - center.X)
	dy := float64(point.Y - center.Y)
	return kicadfiles.Point{
		X: center.X + kicadfiles.IU(math.Round(dx*cos-dy*sin)),
		Y: center.Y + kicadfiles.IU(math.Round(dx*sin+dy*cos)),
	}
}

func readStrokeWidth(node sexpr.ParsedNode) kicadfiles.IU {
	if stroke, ok := node.Child("stroke"); ok {
		if width, ok := stroke.Child("width"); ok && len(width.Children) > 1 {
			if value, ok := width.FloatValue(1); ok {
				return kicadfiles.MM(value)
			}
		}
	}
	return 0
}

func readStrokeType(node sexpr.ParsedNode) string {
	if stroke, ok := node.Child("stroke"); ok {
		if kind, ok := stroke.Child("type"); ok && len(kind.Children) > 1 {
			return kind.ListValue(1)
		}
	}
	return ""
}

func readFill(node sexpr.ParsedNode) string {
	if fill, ok := node.Child("fill"); ok && len(fill.Children) > 1 {
		return fill.ListValue(1)
	}
	return ""
}

func readNamedPoint(node sexpr.ParsedNode, name string) kicadfiles.Point {
	point, _ := readNamedPointOK(node, name)
	return point
}

func readNamedPointOK(node sexpr.ParsedNode, name string) (kicadfiles.Point, bool) {
	child, ok := node.Child(name)
	if !ok {
		return kicadfiles.Point{}, false
	}
	return readPointValuesOK(child, 1)
}

func readPointValues(node sexpr.ParsedNode, offset int) kicadfiles.Point {
	point, _ := readPointValuesOK(node, offset)
	return point
}

func readPointValuesOK(node sexpr.ParsedNode, offset int) (kicadfiles.Point, bool) {
	var point kicadfiles.Point
	x, xOK := node.FloatValue(offset)
	y, yOK := node.FloatValue(offset + 1)
	if xOK {
		point.X = kicadfiles.MM(x)
	}
	if yOK {
		point.Y = kicadfiles.MM(y)
	}
	return point, xOK && yOK
}

func firstNumericMM(node sexpr.ParsedNode, offset int) (kicadfiles.IU, bool) {
	return firstNumericMMAtDepth(node, offset, 0)
}

func firstNumericMMAtDepth(node sexpr.ParsedNode, offset int, depth int) (kicadfiles.IU, bool) {
	if depth > maxFootprintNestedNumericDepth {
		return 0, false
	}
	for i := offset; i < len(node.Children); i++ {
		if value, ok := node.FloatValue(i); ok {
			return kicadfiles.MM(value), true
		}
		if node.Children[i].IsList {
			if value, ok := firstNumericMMAtDepth(node.Children[i], 1, depth+1); ok {
				return value, true
			}
		}
	}
	return 0, false
}

func listValues(node sexpr.ParsedNode, offset int) []string {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(node.Children) {
		return nil
	}
	values := make([]string, 0, len(node.Children)-offset)
	for _, child := range node.Children[offset:] {
		if value := strings.TrimSpace(child.Value()); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func graphicLayer(node sexpr.ParsedNode) string {
	if layer, ok := node.Child("layer"); ok && len(layer.Children) > 1 {
		return layer.ListValue(1)
	}
	return ""
}

func markGraphicLayer(summary *GraphicsSummary, node sexpr.ParsedNode) bool {
	layer := graphicLayer(node)
	summary.markLayer(layer)
	return isCourtyardLayer(layer)
}

func isCourtyardLayer(layer string) bool {
	switch layer {
	case "F.CrtYd", "B.CrtYd":
		return true
	default:
		return false
	}
}

func (summary *GraphicsSummary) markLayer(layer string) {
	if isCourtyardLayer(layer) {
		summary.HasCourtyard = true
		return
	}
	switch layer {
	case "F.Fab", "B.Fab":
		summary.HasFabOutline = true
	case "F.SilkS", "B.SilkS":
		summary.HasSilk = true
	}
}

func readPolyPoints(path string, node sexpr.ParsedNode) ([]kicadfiles.Point, []reports.Issue) {
	pts, ok := node.Child("pts")
	if !ok {
		return nil, nil
	}
	var points []kicadfiles.Point
	var issues []reports.Issue
	for _, xy := range pts.ChildrenByHead("xy") {
		point, ok := readPointValuesOK(xy, 1)
		if !ok {
			issues = append(issues, parseIssue(path, "xy requires numeric x and y coordinates"))
			continue
		}
		points = append(points, point)
	}
	return points, issues
}

func includeFootprintGraphicPoints(bounds *footprintBounds, courtyardBounds *footprintBounds, points []kicadfiles.Point, courtyard bool) {
	for _, point := range points {
		bounds.includePoint(point)
		if courtyard {
			courtyardBounds.includePoint(point)
		}
	}
}

type footprintBounds struct {
	initialized bool
	min         kicadfiles.Point
	max         kicadfiles.Point
}

func newBounds() footprintBounds {
	return footprintBounds{}
}

func (bounds *footprintBounds) includePad(pad FootprintPad) {
	if pad.Shape == "circle" || pad.Shape == "oval" {
		bounds.includeEllipsePad(pad)
		return
	}
	minX := pad.Position.X - pad.Size.X/2
	minY := pad.Position.Y - pad.Size.Y/2
	maxX := minX + pad.Size.X
	maxY := minY + pad.Size.Y
	if pad.Rotation == 0 {
		bounds.includePoint(kicadfiles.Point{X: minX, Y: minY})
		bounds.includePoint(kicadfiles.Point{X: maxX, Y: maxY})
		return
	}
	radians := pad.Rotation * math.Pi / 180
	sin, cos := math.Sin(radians), math.Cos(radians)
	for _, corner := range []kicadfiles.Point{
		{X: minX, Y: minY},
		{X: minX, Y: maxY},
		{X: maxX, Y: minY},
		{X: maxX, Y: maxY},
	} {
		localX := float64(corner.X - pad.Position.X)
		localY := float64(corner.Y - pad.Position.Y)
		bounds.includePoint(kicadfiles.Point{
			X: pad.Position.X + kicadfiles.IU(math.Round(localX*cos-localY*sin)),
			Y: pad.Position.Y + kicadfiles.IU(math.Round(localX*sin+localY*cos)),
		})
	}
}

func (bounds *footprintBounds) includeEllipsePad(pad FootprintPad) {
	a := float64(pad.Size.X) / 2
	b := float64(pad.Size.Y) / 2
	radians := pad.Rotation * math.Pi / 180
	sin, cos := math.Sin(radians), math.Cos(radians)
	xRadius := kicadfiles.IU(math.Round(math.Hypot(a*cos, b*sin)))
	yRadius := kicadfiles.IU(math.Round(math.Hypot(a*sin, b*cos)))
	bounds.includePoint(kicadfiles.Point{X: pad.Position.X - xRadius, Y: pad.Position.Y - yRadius})
	bounds.includePoint(kicadfiles.Point{X: pad.Position.X + xRadius, Y: pad.Position.Y + yRadius})
}

func (bounds *footprintBounds) includePoint(point kicadfiles.Point) {
	if !bounds.initialized {
		bounds.initialized = true
		bounds.min = point
		bounds.max = point
		return
	}
	if point.X < bounds.min.X {
		bounds.min.X = point.X
	}
	if point.Y < bounds.min.Y {
		bounds.min.Y = point.Y
	}
	if point.X > bounds.max.X {
		bounds.max.X = point.X
	}
	if point.Y > bounds.max.Y {
		bounds.max.Y = point.Y
	}
}

func (bounds *footprintBounds) includeNamedPoint(node sexpr.ParsedNode, name string) {
	if point, ok := readNamedPointOK(node, name); ok {
		bounds.includePoint(point)
	}
}

func (bounds *footprintBounds) includeCircle(center kicadfiles.Point, end kicadfiles.Point) {
	radius := kicadfiles.IU(math.Round(math.Hypot(float64(end.X-center.X), float64(end.Y-center.Y))))
	bounds.includePoint(kicadfiles.Point{X: center.X - radius, Y: center.Y - radius})
	bounds.includePoint(kicadfiles.Point{X: center.X + radius, Y: center.Y + radius})
}

func (bounds *footprintBounds) includeArc(start kicadfiles.Point, mid kicadfiles.Point, end kicadfiles.Point) {
	center, ok := circleCenter(start, mid, end)
	if !ok {
		bounds.includePoint(start)
		bounds.includePoint(mid)
		bounds.includePoint(end)
		return
	}
	bounds.includePoint(start)
	bounds.includePoint(mid)
	bounds.includePoint(end)
	radius := math.Hypot(float64(start.X-center.X), float64(start.Y-center.Y))
	startAngle := normalizedAngle(start, center)
	midAngle := normalizedAngle(mid, center)
	endAngle := normalizedAngle(end, center)
	ccw := angleOnCCWArc(startAngle, endAngle, midAngle)
	for _, angle := range []float64{0, math.Pi / 2, math.Pi, 3 * math.Pi / 2} {
		onArc := angleOnCCWArc(startAngle, endAngle, angle)
		if !ccw {
			onArc = angleOnCCWArc(endAngle, startAngle, angle)
		}
		if onArc {
			bounds.includePoint(kicadfiles.Point{
				X: center.X + kicadfiles.IU(math.Round(math.Cos(angle)*radius)),
				Y: center.Y + kicadfiles.IU(math.Round(math.Sin(angle)*radius)),
			})
		}
	}
}

func circleCenter(a kicadfiles.Point, b kicadfiles.Point, c kicadfiles.Point) (kicadfiles.Point, bool) {
	ax, ay := float64(a.X), float64(a.Y)
	bx, by := float64(b.X), float64(b.Y)
	cx, cy := float64(c.X), float64(c.Y)
	d := 2 * (ax*(by-cy) + bx*(cy-ay) + cx*(ay-by))
	if math.Abs(d) < 1e-9 {
		return kicadfiles.Point{}, false
	}
	ax2ay2 := ax*ax + ay*ay
	bx2by2 := bx*bx + by*by
	cx2cy2 := cx*cx + cy*cy
	ux := (ax2ay2*(by-cy) + bx2by2*(cy-ay) + cx2cy2*(ay-by)) / d
	uy := (ax2ay2*(cx-bx) + bx2by2*(ax-cx) + cx2cy2*(bx-ax)) / d
	return kicadfiles.Point{X: kicadfiles.IU(math.Round(ux)), Y: kicadfiles.IU(math.Round(uy))}, true
}

func normalizedAngle(point kicadfiles.Point, center kicadfiles.Point) float64 {
	angle := math.Atan2(float64(point.Y-center.Y), float64(point.X-center.X))
	if angle < 0 {
		angle += 2 * math.Pi
	}
	return angle
}

func angleOnCCWArc(start float64, end float64, target float64) bool {
	const epsilon = 1e-9
	diff := end - start
	if diff < 0 {
		diff += 2 * math.Pi
	}
	targetDiff := target - start
	if targetDiff < 0 {
		targetDiff += 2 * math.Pi
	}
	return targetDiff <= diff+epsilon || targetDiff >= 2*math.Pi-epsilon
}

func (bounds footprintBounds) box() BoundingBox {
	if !bounds.initialized {
		return BoundingBox{}
	}
	return BoundingBox{Min: bounds.min, Max: bounds.max}
}
