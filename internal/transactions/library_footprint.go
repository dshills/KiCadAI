package transactions

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/designapi"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/libraryresolver"
)

func footprintRecordPadSpecs(record libraryresolver.FootprintRecord, placementLayer kicadfiles.BoardLayer) []designapi.PadSpec {
	pads := make([]designapi.PadSpec, 0, len(record.Pads))
	for _, pad := range record.Pads {
		pads = append(pads, designapi.PadSpec{
			Name:     pad.Name,
			Type:     pad.Type,
			Shape:    pad.Shape,
			Offset:   pad.Position,
			Rotation: kicadfiles.Angle(pad.Rotation),
			Size:     pad.Size,
			Drill:    pad.Drill,
			Layers:   placementLayers(pad.Layers, placementLayer),
		})
	}
	return pads
}

func enrichPlaceFootprintOptionsWithRecord(options *designapi.PlaceFootprintOptions, record libraryresolver.FootprintRecord, placementLayer kicadfiles.BoardLayer) error {
	options.Description = record.Description
	options.Tags = strings.Join(record.Tags, " ")
	options.Attributes = append([]string(nil), record.Attributes...)
	options.MetadataProperties = importedMetadataProperties(record.Properties)
	options.Properties = footprintPropertiesFromRecord(record.CustomProperties, placementLayer)
	options.Texts = footprintTextsFromRecord(record.Texts, placementLayer)
	options.Graphics = footprintGraphicsFromRecord(record.Graphics, record.Pads, placementLayer)
	options.Models = importedModels(record.Models)
	if len(options.Pads) == 0 {
		options.Pads = footprintRecordPadSpecs(record, placementLayer)
		return nil
	}
	if !netOnlyDesignAPIPadSpecs(options.Pads) {
		return nil
	}
	nets := make(map[string]string, len(options.Pads))
	for _, pad := range options.Pads {
		nets[pad.Name] = pad.Net
	}
	hydrated := footprintRecordPadSpecs(record, placementLayer)
	seen := make(map[string]struct{}, len(hydrated))
	for index := range hydrated {
		if net, exists := nets[hydrated[index].Name]; exists {
			hydrated[index].Net = net
			seen[hydrated[index].Name] = struct{}{}
		}
	}
	for name := range nets {
		if _, exists := seen[name]; !exists {
			return fmt.Errorf("footprint %s has no pad %s", record.FootprintID, name)
		}
	}
	options.Pads = hydrated
	return nil
}

func netOnlyDesignAPIPadSpecs(specs []designapi.PadSpec) bool {
	for _, spec := range specs {
		if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Type) != "" || strings.TrimSpace(spec.Shape) != "" ||
			spec.Offset != (kicadfiles.Point{}) || spec.Rotation != 0 || spec.Size != (kicadfiles.Point{}) || spec.Drill != 0 || len(spec.Layers) != 0 {
			return false
		}
	}
	return len(specs) > 0
}

func upsertImportedFootprintWithLibrary(board *pcb.PCBFile, generator kicadfiles.IDGenerator, payload PlaceFootprintOperation, index *libraryresolver.LibraryIndex) error {
	if index == nil || (len(payload.Pads) > 0 && !netOnlyPadSpecs(payload.Pads)) {
		upsertImportedFootprint(board, generator, payload)
		return nil
	}
	record, ok := libraryresolver.ResolveFootprint(*index, payload.FootprintID)
	if !ok || len(record.Pads) == 0 {
		if netOnlyPadSpecs(payload.Pads) {
			return fmt.Errorf("footprint library record %s with pad geometry is required for net-only pad assignments", payload.FootprintID)
		}
		upsertImportedFootprint(board, generator, payload)
		return nil
	}
	for i := range board.Footprints {
		if board.Footprints[i].Reference == payload.Ref {
			placement := payload
			placement.Pads = nil
			updateImportedFootprint(&board.Footprints[i], generator, placement)
			if len(board.Footprints[i].Pads) == 0 {
				board.Footprints[i].Pads = importedPadsFromRecord(generator, payload.Ref, record, board.Footprints[i].Layer)
			}
			return applyImportedPadNets(&board.Footprints[i], payload.Pads)
		}
	}
	placement := payload
	placement.Pads = nil
	board.Footprints = append(board.Footprints, importedFootprintFromRecord(generator, placement, record))
	return applyImportedPadNets(&board.Footprints[len(board.Footprints)-1], payload.Pads)
}

func netOnlyPadSpecs(specs []PadSpec) bool {
	for _, spec := range specs {
		if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Type) != "" || strings.TrimSpace(spec.Shape) != "" ||
			spec.XMM != 0 || spec.YMM != 0 || spec.RotationDeg != 0 || spec.WidthMM != 0 || spec.HeightMM != 0 || spec.DrillMM != 0 {
			return false
		}
	}
	return len(specs) > 0
}

func applyImportedPadNets(footprint *pcb.Footprint, specs []PadSpec) error {
	if len(specs) == 0 {
		return nil
	}
	pads := make(map[string][]*pcb.Pad, len(footprint.Pads))
	for i := range footprint.Pads {
		pads[footprint.Pads[i].Name] = append(pads[footprint.Pads[i].Name], &footprint.Pads[i])
	}
	for _, spec := range specs {
		matches := pads[spec.Name]
		if len(matches) == 0 {
			return fmt.Errorf("footprint %s has no pad %s", footprint.LibraryID, spec.Name)
		}
		if spec.Net != nil {
			for _, pad := range matches {
				pad.NetName = *spec.Net
			}
		}
	}
	return nil
}

func importedFootprintFromRecord(generator kicadfiles.IDGenerator, payload PlaceFootprintOperation, record libraryresolver.FootprintRecord) pcb.Footprint {
	value := firstNonEmpty(payload.Value, payload.Ref)
	layer := boardLayer(payload.Layer)
	if layer == "" {
		layer = kicadfiles.LayerFCu
	}
	footprint := pcb.Footprint{
		UUID:               generator.New("imported.pcb.footprint", payload.Ref),
		LibraryID:          strings.TrimSpace(payload.FootprintID),
		Reference:          payload.Ref,
		Value:              value,
		Description:        record.Description,
		Tags:               strings.Join(record.Tags, " "),
		Attributes:         append([]string(nil), record.Attributes...),
		Position:           point(payload.At.XMM, payload.At.YMM),
		Rotation:           kicadfiles.Angle(payload.Rotation),
		Layer:              layer,
		MetadataProperties: importedMetadataProperties(record.Properties),
		Properties:         importedDefaultFootprintProperties(generator, payload.Ref, value, layer, payload.HideDefaultFootprintText),
		Texts:              importedFootprintTexts(generator, payload.Ref, record.Texts, layer),
		Graphics:           importedFootprintGraphics(generator, payload.Ref, record.Graphics, record.Pads, layer),
		Pads:               importedPadsFromRecord(generator, payload.Ref, record, layer),
		Models:             importedModels(record.Models),
	}
	footprint.Properties = append(footprint.Properties, importedFootprintProperties(generator, payload.Ref, record.CustomProperties, layer)...)
	return footprint
}

func footprintPropertiesFromRecord(properties []libraryresolver.FootprintProperty, placementLayer kicadfiles.BoardLayer) []pcb.FootprintProperty {
	result := make([]pcb.FootprintProperty, 0, len(properties))
	for _, property := range properties {
		if isDefaultFootprintPropertyName(property.Name) {
			continue
		}
		result = append(result, pcb.FootprintProperty{
			Name: property.Name, Value: property.Value, Position: property.Position,
			Layer: kicadfiles.BoardLayerForPlacement(kicadfiles.BoardLayer(property.Layer), placementLayer), Hide: property.Hide,
		})
	}
	return result
}

func importedFootprintProperties(generator kicadfiles.IDGenerator, ref string, properties []libraryresolver.FootprintProperty, placementLayer kicadfiles.BoardLayer) []pcb.FootprintProperty {
	result := footprintPropertiesFromRecord(properties, placementLayer)
	for index := range result {
		result[index].UUID = generator.New("imported.pcb.footprint.property", ref, result[index].Name, strconv.Itoa(index))
	}
	return result
}

func importedDefaultFootprintProperties(generator kicadfiles.IDGenerator, ref string, value string, layer kicadfiles.BoardLayer, hideDefaultFootprintText bool) []pcb.FootprintProperty {
	return []pcb.FootprintProperty{
		{Name: "Reference", Value: ref, Position: kicadfiles.DefaultFootprintPropertyPosition("Reference"), Layer: kicadfiles.BoardLayerForPlacement(kicadfiles.LayerFSilkS, layer), Hide: hideDefaultFootprintText, UUID: generator.New("imported.pcb.footprint.property", ref, "Reference")},
		{Name: "Value", Value: value, Position: kicadfiles.DefaultFootprintPropertyPosition("Value"), Layer: kicadfiles.BoardLayerForPlacement(kicadfiles.LayerFSilkS, layer), Hide: hideDefaultFootprintText, UUID: generator.New("imported.pcb.footprint.property", ref, "Value")},
	}
}

func importedPadsFromRecord(generator kicadfiles.IDGenerator, ref string, record libraryresolver.FootprintRecord, layer kicadfiles.BoardLayer) []pcb.Pad {
	pads := make([]pcb.Pad, 0, len(record.Pads))
	for i, pad := range record.Pads {
		pads = append(pads, pcb.Pad{
			UUID:        generator.New("imported.pcb.footprint.pad", ref, pad.Name, strconv.Itoa(i)),
			Name:        pad.Name,
			Type:        pad.Type,
			Shape:       pad.Shape,
			Position:    pad.Position,
			Rotation:    kicadfiles.Angle(pad.Rotation),
			Size:        pad.Size,
			Drill:       pad.Drill,
			Layers:      placementLayers(pad.Layers, layer),
			PinFunction: pad.PinFunction,
			PinType:     pad.PinType,
		})
	}
	return pads
}

func importedMetadataProperties(properties map[string]string) []pcb.FootprintMetadataProperty {
	if len(properties) == 0 {
		return nil
	}
	keys := sortedMapKeys(properties)
	metadata := make([]pcb.FootprintMetadataProperty, 0, len(keys))
	for _, key := range keys {
		if isDefaultFootprintPropertyName(key) {
			continue
		}
		metadata = append(metadata, pcb.FootprintMetadataProperty{Name: key, Value: properties[key]})
	}
	return metadata
}

func isDefaultFootprintPropertyName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "reference", "value", "datasheet", "description":
		return true
	default:
		return false
	}
}

func importedFootprintTexts(generator kicadfiles.IDGenerator, ref string, texts []libraryresolver.FootprintText, placementLayer kicadfiles.BoardLayer) []pcb.FootprintText {
	result := footprintTextsFromRecord(texts, placementLayer)
	for i := range result {
		if !result[i].UUID.Valid() {
			result[i].UUID = generator.New("imported.pcb.footprint.text", ref, result[i].Kind, strconv.Itoa(i))
		}
	}
	return result
}

func footprintTextsFromRecord(texts []libraryresolver.FootprintText, placementLayer kicadfiles.BoardLayer) []pcb.FootprintText {
	result := make([]pcb.FootprintText, 0, len(texts))
	for _, text := range texts {
		result = append(result, pcb.FootprintText{
			Kind:     text.Kind,
			Text:     text.Text,
			Position: text.Position,
			Layer:    kicadfiles.BoardLayerForPlacement(kicadfiles.BoardLayer(text.Layer), placementLayer),
		})
	}
	return result
}

func importedFootprintGraphics(generator kicadfiles.IDGenerator, ref string, graphics []libraryresolver.FootprintGraphic, pads []libraryresolver.FootprintPad, placementLayer kicadfiles.BoardLayer) []pcb.FootprintGraphic {
	result := make([]pcb.FootprintGraphic, 0, len(graphics))
	seen := map[string]int{}
	seedScratch := make([]byte, 0, 256)
	maskObstacles := footprintMaskObstacles(pads, placementLayer)
	for _, graphic := range graphics {
		if silkscreenLineOverlapsPadMask(graphic, maskObstacles, placementLayer) {
			continue
		}
		converted, ok := footprintGraphicFromRecord(graphic, placementLayer)
		if !ok {
			continue
		}
		var seed string
		seed, seedScratch = footprintGraphicSeed(graphic, seedScratch)
		occurrence := seen[seed]
		seen[seed]++
		drawing := pcb.Drawing(converted)
		if !drawing.UUID.Valid() {
			drawing.UUID = generator.New("imported.pcb.footprint.graphic", ref, seed, strconv.Itoa(occurrence))
		}
		result = append(result, pcb.FootprintGraphic(drawing))
	}
	return result
}

func footprintGraphicsFromRecord(graphics []libraryresolver.FootprintGraphic, pads []libraryresolver.FootprintPad, placementLayer kicadfiles.BoardLayer) []pcb.FootprintGraphic {
	result := make([]pcb.FootprintGraphic, 0, len(graphics))
	maskObstacles := footprintMaskObstacles(pads, placementLayer)
	for _, graphic := range graphics {
		if silkscreenLineOverlapsPadMask(graphic, maskObstacles, placementLayer) {
			continue
		}
		if converted, ok := footprintGraphicFromRecord(graphic, placementLayer); ok {
			result = append(result, converted)
		}
	}
	return result
}

type footprintMaskObstacle struct {
	layer                 kicadfiles.BoardLayer
	center                kicadfiles.Point
	cosine, sine          float64
	halfWidth, halfHeight float64
}

func footprintMaskObstacles(pads []libraryresolver.FootprintPad, placementLayer kicadfiles.BoardLayer) []footprintMaskObstacle {
	obstacles := make([]footprintMaskObstacle, 0, len(pads))
	for _, pad := range pads {
		if !supportedMaskPadShape(pad.Shape) {
			continue
		}
		radians := -float64(pad.Rotation) * math.Pi / 180
		cosine, sine := math.Cos(radians), math.Sin(radians)
		for _, layer := range pad.Layers {
			maskLayer := kicadfiles.BoardLayerForPlacement(layer, placementLayer)
			if maskLayer != kicadfiles.LayerFMask && maskLayer != kicadfiles.LayerBMask {
				continue
			}
			obstacles = append(obstacles, footprintMaskObstacle{
				layer: maskLayer, center: pad.Position, cosine: cosine, sine: sine,
				halfWidth: float64(pad.Size.X) / 2, halfHeight: float64(pad.Size.Y) / 2,
			})
		}
	}
	return obstacles
}

func silkscreenLineOverlapsPadMask(graphic libraryresolver.FootprintGraphic, obstacles []footprintMaskObstacle, placementLayer kicadfiles.BoardLayer) bool {
	if graphic.Kind != "line" || graphic.Start == nil || graphic.End == nil {
		return false
	}
	graphicLayer := kicadfiles.BoardLayerForPlacement(kicadfiles.BoardLayer(graphic.Layer), placementLayer)
	if graphicLayer != kicadfiles.LayerFSilkS && graphicLayer != kicadfiles.LayerBSilkS {
		return false
	}
	maskLayer := kicadfiles.LayerFMask
	if graphicLayer == kicadfiles.LayerBSilkS {
		maskLayer = kicadfiles.LayerBMask
	}
	strokeRadius := float64(footprintGraphicStrokeWidth(graphic.Width)) / 2
	for _, obstacle := range obstacles {
		if obstacle.layer != maskLayer {
			continue
		}
		startX, startY := pointInMaskObstacleCoordinates(*graphic.Start, obstacle)
		endX, endY := pointInMaskObstacleCoordinates(*graphic.End, obstacle)
		halfWidth := obstacle.halfWidth + strokeRadius
		halfHeight := obstacle.halfHeight + strokeRadius
		if segmentIntersectsCenteredRect(startX, startY, endX, endY, halfWidth, halfHeight) {
			return true
		}
	}
	return false
}

func supportedMaskPadShape(shape string) bool {
	switch strings.ToLower(strings.TrimSpace(shape)) {
	case "rect", "roundrect", "oval", "circle":
		return true
	default:
		return false
	}
}

func pointInMaskObstacleCoordinates(point kicadfiles.Point, obstacle footprintMaskObstacle) (float64, float64) {
	dx := float64(point.X - obstacle.center.X)
	dy := float64(point.Y - obstacle.center.Y)
	return dx*obstacle.cosine - dy*obstacle.sine, dx*obstacle.sine + dy*obstacle.cosine
}

func segmentIntersectsCenteredRect(startX, startY, endX, endY, halfWidth, halfHeight float64) bool {
	deltaX, deltaY := endX-startX, endY-startY
	minimum, maximum := 0.0, 1.0
	for _, boundary := range [4][2]float64{
		{-deltaX, startX + halfWidth},
		{deltaX, halfWidth - startX},
		{-deltaY, startY + halfHeight},
		{deltaY, halfHeight - startY},
	} {
		if boundary[0] == 0 {
			if boundary[1] < 0 {
				return false
			}
			continue
		}
		ratio := boundary[1] / boundary[0]
		if boundary[0] < 0 {
			if ratio > maximum {
				return false
			}
			minimum = max(minimum, ratio)
		} else {
			if ratio < minimum {
				return false
			}
			maximum = min(maximum, ratio)
		}
	}
	return true
}

func footprintGraphicFromRecord(graphic libraryresolver.FootprintGraphic, placementLayer kicadfiles.BoardLayer) (pcb.FootprintGraphic, bool) {
	width := footprintGraphicStrokeWidth(graphic.Width)
	drawing := pcb.Drawing{
		Kind:       graphic.Kind,
		Layer:      kicadfiles.BoardLayerForPlacement(kicadfiles.BoardLayer(graphic.Layer), placementLayer),
		StrokeType: graphic.StrokeType,
		Fill:       graphic.Fill,
	}
	switch graphic.Kind {
	case "line":
		if graphic.Start != nil && graphic.End != nil {
			drawing.Line = &pcb.LineDrawing{Start: *graphic.Start, End: *graphic.End, Width: width}
		}
	case "rect":
		if graphic.Start != nil && graphic.End != nil {
			drawing.Rect = &pcb.RectDrawing{Start: *graphic.Start, End: *graphic.End, Width: width}
		}
	case "circle":
		if graphic.Center != nil && graphic.End != nil {
			drawing.Circle = &pcb.CircleDrawing{Center: *graphic.Center, End: *graphic.End, Width: width}
		}
	case "arc":
		if graphic.Start != nil && graphic.Mid != nil && graphic.End != nil {
			drawing.Arc = &pcb.ArcDrawing{Start: *graphic.Start, Mid: *graphic.Mid, End: *graphic.End, Width: width}
		}
	case "poly":
		if len(graphic.Points) > 0 {
			drawing.Poly = &pcb.PolylineDrawing{Points: append([]kicadfiles.Point(nil), graphic.Points...), Width: width}
		}
	case "curve":
		if len(graphic.Points) > 0 {
			drawing.Curve = &pcb.PolylineDrawing{Points: append([]kicadfiles.Point(nil), graphic.Points...), Width: width}
		}
	}
	if drawing.Line == nil && drawing.Rect == nil && drawing.Circle == nil && drawing.Arc == nil && drawing.Poly == nil && drawing.Curve == nil {
		return pcb.FootprintGraphic{}, false
	}
	return pcb.FootprintGraphic(drawing), true
}

func footprintGraphicStrokeWidth(width kicadfiles.IU) kicadfiles.IU {
	if width > 0 {
		return width
	}
	return pcb.DefaultFootprintGraphicStrokeWidth
}

func footprintGraphicSeed(graphic libraryresolver.FootprintGraphic, scratch []byte) (string, []byte) {
	scratch = scratch[:0]
	scratch = appendSeedString(scratch, graphic.Kind)
	scratch = appendSeedString(scratch, graphic.Layer)
	scratch = appendSeedString(scratch, graphic.StrokeType)
	scratch = appendSeedString(scratch, graphic.Fill)
	scratch = strconv.AppendInt(scratch, int64(graphic.Width), 10)
	scratch = append(scratch, '|')
	scratch = appendSeedPoint(scratch, graphic.Start)
	scratch = appendSeedPoint(scratch, graphic.End)
	scratch = appendSeedPoint(scratch, graphic.Mid)
	scratch = appendSeedPoint(scratch, graphic.Center)
	for i, point := range graphic.Points {
		scratch = strconv.AppendInt(scratch, int64(i), 10)
		scratch = append(scratch, ':')
		scratch = strconv.AppendInt(scratch, int64(point.X), 10)
		scratch = append(scratch, ',')
		scratch = strconv.AppendInt(scratch, int64(point.Y), 10)
		scratch = append(scratch, '|')
	}
	sum := sha1.Sum(scratch)
	return hex.EncodeToString(sum[:8]), scratch
}

func appendSeedString(buffer []byte, value string) []byte {
	buffer = append(buffer, value...)
	return append(buffer, '|')
}

func appendSeedPoint(buffer []byte, point *kicadfiles.Point) []byte {
	if point == nil {
		buffer = append(buffer, "nil"...)
		return append(buffer, '|')
	}
	buffer = strconv.AppendInt(buffer, int64(point.X), 10)
	buffer = append(buffer, ',')
	buffer = strconv.AppendInt(buffer, int64(point.Y), 10)
	return append(buffer, '|')
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func importedModels(paths []string) []pcb.Model3D {
	models := make([]pcb.Model3D, 0, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) != "" {
			models = append(models, pcb.Model3D{Path: strings.TrimSpace(path)})
		}
	}
	return models
}

func placementLayers(layers []kicadfiles.BoardLayer, placementLayer kicadfiles.BoardLayer) []kicadfiles.BoardLayer {
	if len(layers) == 0 {
		return nil
	}
	mapped := make([]kicadfiles.BoardLayer, 0, len(layers))
	for _, layer := range layers {
		mapped = append(mapped, kicadfiles.BoardLayerForPlacement(layer, placementLayer))
	}
	return mapped
}
