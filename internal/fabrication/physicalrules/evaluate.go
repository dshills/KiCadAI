package physicalrules

import (
	"math"
	"slices"
	"strings"

	"kicadai/internal/kicadfiles"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
	projectfiles "kicadai/internal/kicadfiles/project"
	"kicadai/internal/reports"
)

const outlineToleranceMM = 0.001
const maxReportedObjects = 100

var metadataKeyReplacer = strings.NewReplacer("-", "_", " ", "_")

func EvaluateBoard(board *pcbfiles.PCBFile, project *projectfiles.ProjectFile, opts Options) Report {
	opts = NormalizeOptions(opts)
	if board == nil {
		report := NewReport(opts.ProfileID, BoardRef{}, []Check{{
			ID:         "physical.board.present",
			Category:   CategoryStackup,
			Status:     StatusBlocked,
			Message:    "PCB board data is required for physical fabrication checks",
			Suggestion: "load a .kicad_pcb file before running physical fabrication checks",
			IssuePath:  "physical.board",
			Source:     SourceParser,
		}})
		report.ProfileDetails = opts.ProfileDetails
		report.Evidence = appendProfileEvidence(report.Evidence, opts.ProfileDetails)
		return Normalize(report)
	}
	checks := []Check{}
	checks = append(checks, evaluateStackup(board)...)
	checks = append(checks, evaluateNetClasses(board, project)...)
	checks = append(checks, evaluateImpedanceEvidence(board, project, opts)...)
	checks = append(checks, evaluateAnnularRings(board, opts)...)
	outline := evaluateEdgeCuts(board)
	checks = append(checks, evaluateCopperSlivers(board, opts, outline.Bounds)...)
	checks = append(checks, evaluateMaskPaste(board, opts)...)
	checks = append(checks, outline.Checks...)
	edgePlating := edgePlatingFeatures(board, outline.Bounds)
	checks = append(checks, evaluateEdgePlating(edgePlating, opts)...)
	checks = append(checks, evaluateFabricationMetadata(project, edgePlating, opts)...)
	checks = append(checks, evaluateBoardContainment(board, outline.Bounds)...)
	checks = append(checks, evaluateCourtyardSilkscreen(board, outline.Bounds)...)
	checks = append(checks, evaluateMountingHoles(board, outline.Bounds, opts)...)
	report := NewReport(opts.ProfileID, BoardRef{LayerCount: copperLayerCount(board)}, checks)
	report.ProfileDetails = opts.ProfileDetails
	report.Evidence = appendProfileEvidence(report.Evidence, opts.ProfileDetails)
	return Normalize(report)
}

func appendProfileEvidence(evidence []Evidence, profile *ProfileInfo) []Evidence {
	if profile == nil || strings.TrimSpace(profile.ID) == "" {
		return evidence
	}
	return append(evidence, Evidence{
		Kind: "manufacturer_profile",
		Path: profile.SourcePath,
		Note: profile.ID + "@" + profile.Version,
	})
}

func evaluateMaskPaste(board *pcbfiles.PCBFile, opts Options) []Check {
	var maskObjects []string
	var maskRefs = map[string]struct{}{}
	var pasteObjects []string
	var pasteRefs = map[string]struct{}{}
	maskViolations := 0
	pasteViolations := 0
	for _, footprint := range board.Footprints {
		for padIndex := range footprint.Pads {
			pad := &footprint.Pads[padIndex]
			layers := summarizePadLayers(pad)
			if layers.requiresMask() && !layers.hasRequiredMask() {
				maskViolations++
				addRef(maskRefs, footprint.Reference)
				maskObjects = appendLimited(maskObjects, string(pad.UUID))
			}
			if padRequiresPaste(pad) {
				if !layers.hasRequiredPaste() {
					pasteViolations++
					addRef(pasteRefs, footprint.Reference)
					pasteObjects = appendLimited(pasteObjects, string(pad.UUID))
				}
			} else if layers.hasAnyPaste() {
				pasteViolations++
				addRef(pasteRefs, footprint.Reference)
				pasteObjects = appendLimited(pasteObjects, string(pad.UUID))
			}
		}
	}
	checks := []Check{
		{
			ID:       CheckSolderMaskArtifacts,
			Category: CategorySolderMask,
			Status:   StatusSkipped,
			Message:  "solder mask artifact presence is checked during fabrication package validation",
			Source:   SourceParser,
		},
		{
			ID:       CheckSolderPasteArtifacts,
			Category: CategorySolderPaste,
			Status:   StatusSkipped,
			Message:  "solder paste artifact presence is checked during fabrication package validation",
			Source:   SourceParser,
		},
	}
	if maskViolations == 0 {
		checks = append(checks, Check{
			ID:       CheckSolderMaskPadLayers,
			Category: CategorySolderMask,
			Status:   StatusPass,
			Message:  "pad solder mask layers are consistent with pad copper layers",
			Source:   SourceParser,
		})
	} else {
		checks = append(checks, Check{
			ID:         CheckSolderMaskPadLayers,
			Category:   CategorySolderMask,
			Status:     StatusBlocked,
			Message:    "one or more pads are missing required solder mask layers",
			Suggestion: "add matching F.Mask/B.Mask layers for assembly pads or use an explicit pad policy before fabrication export",
			IssuePath:  "physical.solder_mask.pad_layers",
			References: sortedMapKeys(maskRefs),
			Objects:    maskObjects,
			Measurements: []Measurement{
				{Name: "violation_count", Value: float64(maskViolations), Unit: "count"},
			},
			Source: SourceParser,
		})
	}
	if pasteViolations == 0 {
		checks = append(checks, Check{
			ID:       CheckSolderPastePadLayers,
			Category: CategorySolderPaste,
			Status:   StatusPass,
			Message:  "pad solder paste layers are consistent with pad type and copper side",
			Source:   SourceParser,
		})
	} else {
		checks = append(checks, Check{
			ID:         CheckSolderPastePadLayers,
			Category:   CategorySolderPaste,
			Status:     StatusBlocked,
			Message:    "one or more pads have inconsistent solder paste layers",
			Suggestion: "add paste only to SMD assembly pads on the matching side, or remove paste from THT and mechanical pads",
			IssuePath:  "physical.solder_paste.pad_layers",
			References: sortedMapKeys(pasteRefs),
			Objects:    pasteObjects,
			Measurements: []Measurement{
				{Name: "violation_count", Value: float64(pasteViolations), Unit: "count"},
			},
			Source: SourceParser,
		})
	}
	checks = append(checks, evaluateSolderMaskWeb(board, opts))
	checks = append(checks, evaluateSolderMaskPolygonWeb(board, opts))
	return checks
}

func evaluateSolderMaskPolygonWeb(board *pcbfiles.PCBFile, opts Options) Check {
	expansion := board.Setup.PadToMaskClearance
	openings := dfmMaskOpenings(board, expansion)
	if len(openings) < 2 {
		return Check{ID: CheckSolderMaskPolygonWebWidth, Category: CategorySolderMask, Status: StatusSkipped, Message: "not enough mask opening polygons were found for polygon web checks", Source: SourceParser}
	}
	unsupported := 0
	candidates := 0
	compared := 0
	violations := 0
	minObserved := math.MaxFloat64
	var objects []string
	objectSet := map[string]struct{}{}
	addObject := func(objectID string) {
		if strings.TrimSpace(objectID) == "" {
			return
		}
		if _, ok := objectSet[objectID]; ok {
			return
		}
		objectSet[objectID] = struct{}{}
		objects = appendLimited(objects, objectID)
	}
	refs := map[string]struct{}{}
	type openingRecord struct {
		Polygon dfmPolygon
		Bounds  dfmRect
		Object  string
		Layer   string
	}
	records := make([]openingRecord, 0, len(openings))
	for _, opening := range openings {
		if !opening.supported() {
			unsupported++
			addObject(polygonObjectID(opening))
			addRef(refs, opening.Reference)
			continue
		}
		candidates++
		records = append(records, openingRecord{Polygon: opening, Bounds: dfmPolygonBounds(opening.Points), Object: polygonObjectID(opening), Layer: string(opening.Layer)})
	}
	slices.SortFunc(records, func(a, b openingRecord) int {
		if a.Layer != b.Layer {
			return strings.Compare(a.Layer, b.Layer)
		}
		if a.Bounds.MinX < b.Bounds.MinX {
			return -1
		}
		if a.Bounds.MinX > b.Bounds.MinX {
			return 1
		}
		return strings.Compare(a.Object, b.Object)
	})
	sameSidePairs := false
	for index, opening := range records {
		for otherIndex := index + 1; otherIndex < len(records); otherIndex++ {
			other := records[otherIndex]
			if opening.Polygon.Layer != other.Polygon.Layer {
				break
			}
			sameSidePairs = true
			if opening.Bounds.Valid && other.Bounds.Valid && other.Bounds.MinX-opening.Bounds.MaxX >= opts.MinSolderMaskWebMM {
				break
			}
			if opening.Bounds.Valid && other.Bounds.Valid && dfmRectDistanceMM(opening.Bounds, other.Bounds) >= opts.MinSolderMaskWebMM {
				continue
			}
			compared++
			web := dfmPolygonDistanceMM(opening.Polygon.Points, other.Polygon.Points)
			if web > 1e-6 && web < minObserved {
				minObserved = web
			}
			if web > 1e-6 && web < opts.MinSolderMaskWebMM {
				violations++
				addObject(opening.Object)
				addObject(other.Object)
				addRef(refs, opening.Polygon.Reference)
				addRef(refs, other.Polygon.Reference)
			}
		}
	}
	measurements := []Measurement{
		{Name: "candidate_opening_count", Value: float64(candidates), Unit: "count"},
		{Name: "compared_opening_pair_count", Value: float64(compared), Unit: "count"},
		{Name: "unsupported_opening_count", Value: float64(unsupported), Unit: "count"},
		{Name: "minimum_required_solder_mask_web", Value: opts.MinSolderMaskWebMM, Unit: "mm"},
		{Name: "pad_to_mask_clearance", Value: iuToMM(expansion), Unit: "mm"},
	}
	if minObserved != math.MaxFloat64 {
		measurements = append(measurements, Measurement{Name: "minimum_observed_solder_mask_web", Value: minObserved, Unit: "mm"})
	}
	if candidates < 2 || (compared == 0 && !sameSidePairs) {
		status := StatusSkipped
		message := "not enough same-side supported mask opening polygons were found for polygon web checks"
		suggestion := ""
		if unsupported > 0 {
			status = unsupportedGeometryStatus(opts)
			message = strictEvidenceMessage("mask opening geometry was present but some openings could not be measured for polygon web checks", opts)
			suggestion = "run KiCad DRC or expand pad geometry support before treating solder-mask web checks as complete"
		}
		return Check{ID: CheckSolderMaskPolygonWebWidth, Category: CategorySolderMask, Status: status, Message: message, Suggestion: suggestion, IssuePath: "physical.solder_mask.polygon_web_width", Objects: objects, References: sortedMapKeys(refs), Measurements: measurements, Source: SourceParser}
	}
	if compared == 0 && unsupported == 0 {
		return Check{ID: CheckSolderMaskPolygonWebWidth, Category: CategorySolderMask, Status: StatusPass, Message: "mask opening polygon bounding boxes are farther apart than the active profile threshold", Measurements: measurements, Source: SourceParser}
	}
	if violations > 0 {
		measurements = append(measurements, Measurement{Name: "violation_count", Value: float64(violations), Unit: "count"})
		return Check{
			ID:           CheckSolderMaskPolygonWebWidth,
			Category:     CategorySolderMask,
			Status:       StatusBlocked,
			Message:      "one or more mask opening polygons leave less solder-mask web than the active profile threshold",
			Suggestion:   "increase pad spacing, reduce pad-to-mask expansion, or select a profile that supports the modeled solder-mask web",
			IssuePath:    "physical.solder_mask.polygon_web_width",
			Objects:      objects,
			References:   sortedMapKeys(refs),
			Measurements: measurements,
			Source:       SourceParser,
		}
	}
	if unsupported > 0 {
		status := unsupportedGeometryStatus(opts)
		return Check{
			ID:           CheckSolderMaskPolygonWebWidth,
			Category:     CategorySolderMask,
			Status:       status,
			Message:      strictEvidenceMessage("supported mask opening polygons meet the active profile threshold, but some openings could not be measured", opts),
			Suggestion:   "run KiCad DRC or expand pad geometry support before treating solder-mask web checks as complete",
			IssuePath:    "physical.solder_mask.polygon_web_width",
			Objects:      objects,
			References:   sortedMapKeys(refs),
			Measurements: measurements,
			Source:       SourceParser,
		}
	}
	return Check{ID: CheckSolderMaskPolygonWebWidth, Category: CategorySolderMask, Status: StatusPass, Message: "mask opening polygon webs meet the active profile threshold", Measurements: measurements, Source: SourceParser}
}

func evaluateSolderMaskWeb(board *pcbfiles.PCBFile, opts Options) Check {
	pads := maskWebPads(board)
	if len(pads) < 2 {
		return Check{ID: CheckSolderMaskWebWidth, Category: CategorySolderMask, Status: StatusSkipped, Message: "not enough same-side exposed pads were found for solder mask web checks", Source: SourceParser}
	}
	expansion := board.Setup.PadToMaskClearance
	if expansion < 0 {
		expansion = 0
	}
	threshold := opts.MinSolderMaskWebMM
	expansionMM := iuToMM(expansion) * 2
	searchRadius := kicadfiles.MM(threshold + expansionMM)
	slices.SortFunc(pads, func(a, b maskWebPad) int {
		if a.Side != b.Side {
			return strings.Compare(string(a.Side), string(b.Side))
		}
		if a.Bounds.MinX < b.Bounds.MinX {
			return -1
		}
		if a.Bounds.MinX > b.Bounds.MinX {
			return 1
		}
		return strings.Compare(a.UUID, b.UUID)
	})
	violations := 0
	unknown := 0
	compared := 0
	var objects []string
	refs := map[string]struct{}{}
	minObserved := math.MaxFloat64
	for i := 0; i < len(pads); i++ {
		for j := i + 1; j < len(pads); j++ {
			a := pads[i]
			b := pads[j]
			if a.Side != b.Side {
				break
			}
			if a.Bounds.Valid && b.Bounds.Valid && b.Bounds.MinX-a.Bounds.MaxX > searchRadius {
				break
			}
			if !a.Bounds.Valid || !b.Bounds.Valid || a.Rotated || b.Rotated {
				unknown++
				addRef(refs, a.Reference)
				addRef(refs, b.Reference)
				objects = appendLimited(objects, a.UUID)
				objects = appendLimited(objects, b.UUID)
				continue
			}
			compared++
			spacing := rectSpacingMM(a.Bounds, b.Bounds)
			web := spacing - expansionMM
			if web < minObserved {
				minObserved = web
			}
			if web < threshold {
				violations++
				addRef(refs, a.Reference)
				addRef(refs, b.Reference)
				objects = appendLimited(objects, a.UUID)
				objects = appendLimited(objects, b.UUID)
			}
		}
	}
	measurements := []Measurement{
		{Name: "candidate_pad_count", Value: float64(len(pads)), Unit: "count"},
		{Name: "compared_pair_count", Value: float64(compared), Unit: "count"},
		{Name: "min_required_solder_mask_web", Value: threshold, Unit: "mm"},
		{Name: "pad_to_mask_clearance", Value: iuToMM(expansion), Unit: "mm"},
	}
	if minObserved != math.MaxFloat64 {
		measurements = append(measurements, Measurement{Name: "minimum_observed_solder_mask_web", Value: minObserved, Unit: "mm"})
	}
	if violations > 0 {
		measurements = append(measurements, Measurement{Name: "violation_count", Value: float64(violations), Unit: "count"})
		return Check{
			ID:           CheckSolderMaskWebWidth,
			Category:     CategorySolderMask,
			Status:       StatusBlocked,
			Message:      "one or more same-side exposed pads leave less solder mask web than the active profile threshold",
			Suggestion:   "increase pad spacing, reduce mask expansion where appropriate, or select a manufacturer profile that supports the modeled mask web",
			IssuePath:    "physical.solder_mask.web_width",
			References:   sortedMapKeys(refs),
			Objects:      objects,
			Measurements: measurements,
			Source:       SourceHeuristic,
		}
	}
	if unknown > 0 {
		measurements = append(measurements, Measurement{Name: "unknown_geometry_count", Value: float64(unknown), Unit: "count"})
		return Check{
			ID:           CheckSolderMaskUnsupported,
			Category:     CategorySolderMask,
			Status:       StatusWarning,
			Message:      "one or more exposed pads lack enough geometry for deterministic solder mask web checks",
			Suggestion:   "hydrate pad geometry or use KiCad/manufacturer DFM evidence before treating mask web checks as complete",
			IssuePath:    "physical.solder_mask.unsupported_geometry",
			References:   sortedMapKeys(refs),
			Objects:      objects,
			Measurements: measurements,
			Source:       SourceHeuristic,
		}
	}
	return Check{ID: CheckSolderMaskWebWidth, Category: CategorySolderMask, Status: StatusPass, Message: "same-side exposed pad spacing leaves enough estimated solder mask web", Measurements: measurements, Source: SourceHeuristic}
}

func evaluateAnnularRings(board *pcbfiles.PCBFile, opts Options) []Check {
	padCheck := evaluatePlatedPadAnnularRings(board, opts)
	viaCheck := evaluateViaAnnularRings(board, opts)
	return []Check{
		{
			ID:       CheckAnnularRingProfile,
			Category: CategoryAnnularRing,
			Status:   StatusPass,
			Message:  "annular ring thresholds are defined by the active physical-rule profile",
			Measurements: []Measurement{
				{Name: "min_plated_pad_annular_ring", Value: opts.MinPlatedPadAnnularRingMM, Unit: "mm"},
				{Name: "min_via_annular_ring", Value: opts.MinViaRingMM, Unit: "mm"},
			},
			Source: SourceProfile,
		},
		padCheck,
		viaCheck,
	}
}

func evaluateCopperSlivers(board *pcbfiles.PCBFile, opts Options, outlineBounds boardBounds) []Check {
	filledPolygons := dfmFilledZonePolygons(board)
	copperPolygons := dfmCopperGraphicPolygons(board)
	return []Check{
		evaluateCopperTrackWidths(board, opts),
		evaluateCopperZoneWidths(board, opts),
		evaluateCopperPolygonWidths(CheckCopperSliverFilledPolygon, "physical.copper_sliver.filled_polygon_width", "filled zone polygon", filledPolygons, opts),
		evaluateCopperPolygonWidths(CheckCopperSliverCopperPolygon, "physical.copper_sliver.copper_polygon_width", "copper graphic polygon", copperPolygons, opts),
		evaluateCopperPolygonEdgeClearance(board, opts, outlineBounds, filledPolygons, copperPolygons),
		evaluateUnsupportedCopperGeometry(board),
	}
}

func dfmFilledZonePolygons(board *pcbfiles.PCBFile) []dfmPolygon {
	observations := dfmZonePolygons(board)
	out := make([]dfmPolygon, 0, len(observations))
	for _, observation := range observations {
		if observation.Kind == dfmGeometryFilledZonePolygon {
			out = append(out, observation)
		}
	}
	return out
}

func evaluateCopperPolygonWidths(id, issuePath, label string, polygons []dfmPolygon, opts Options) Check {
	if len(polygons) == 0 {
		return Check{ID: id, Category: CategoryCopperSliver, Status: StatusSkipped, Message: "no " + label + " geometry was found for polygon width checks", Source: SourceParser}
	}
	checked := 0
	unsupported := 0
	violations := 0
	minObserved := math.MaxFloat64
	var objects []string
	nets := map[string]struct{}{}
	for _, polygon := range polygons {
		width := dfmEstimatePolygonWidth(polygon)
		if !width.Measured {
			unsupported++
			objects = appendLimited(objects, polygonObjectID(polygon))
			addRef(nets, polygon.NetName)
			continue
		}
		checked++
		if width.WidthMM < minObserved {
			minObserved = width.WidthMM
		}
		if width.WidthMM < opts.MinCopperFeatureMM {
			violations++
			objects = appendLimited(objects, polygonObjectID(polygon))
			addRef(nets, polygon.NetName)
		}
	}
	measurements := []Measurement{
		{Name: "checked_polygon_count", Value: float64(checked), Unit: "count"},
		{Name: "unsupported_polygon_count", Value: float64(unsupported), Unit: "count"},
		{Name: "minimum_required_copper_feature", Value: opts.MinCopperFeatureMM, Unit: "mm"},
	}
	if minObserved != math.MaxFloat64 {
		measurements = append(measurements, Measurement{Name: "minimum_observed_polygon_width", Value: minObserved, Unit: "mm"})
	}
	if violations > 0 {
		measurements = append(measurements, Measurement{Name: "violation_count", Value: float64(violations), Unit: "count"})
		return Check{
			ID:           id,
			Category:     CategoryCopperSliver,
			Status:       StatusBlocked,
			Message:      "one or more " + label + " features are narrower than the active profile threshold",
			Suggestion:   "increase polygon feature width or verify the geometry with KiCad DRC/manufacturer DFM evidence",
			IssuePath:    issuePath,
			Objects:      objects,
			Nets:         sortedMapKeys(nets),
			Measurements: measurements,
			Source:       SourceParser,
		}
	}
	if unsupported > 0 {
		status := unsupportedGeometryStatus(opts)
		return Check{
			ID:           id,
			Category:     CategoryCopperSliver,
			Status:       status,
			Message:      strictEvidenceMessage("some "+label+" geometry could not be measured for polygon width checks", opts),
			Suggestion:   "run KiCad DRC or expand polygon parser support before treating copper sliver checks as complete",
			IssuePath:    issuePath,
			Objects:      objects,
			Nets:         sortedMapKeys(nets),
			Measurements: measurements,
			Source:       SourceParser,
		}
	}
	return Check{ID: id, Category: CategoryCopperSliver, Status: StatusPass, Message: label + " widths meet the active profile threshold", Measurements: measurements, Source: SourceParser}
}

func evaluateCopperPolygonEdgeClearance(board *pcbfiles.PCBFile, opts Options, outlineBounds boardBounds, filledPolygons, copperPolygons []dfmPolygon) Check {
	if opts.MinCopperEdgeMM <= 0 {
		return Check{ID: CheckCopperSliverPolygonEdge, Category: CategoryCopperSliver, Status: StatusSkipped, Message: "no copper-to-edge threshold is active for polygon edge-clearance checks", Source: SourceProfile}
	}
	outlines := dfmBoardOutlinePolygons(board, outlineBounds)
	if len(outlines) == 0 {
		return Check{ID: CheckCopperSliverPolygonEdge, Category: CategoryCopperSliver, Status: StatusSkipped, Message: "no usable board outline polygon was found for copper polygon edge-clearance checks", Source: SourceParser}
	}
	type outlineGeometry struct {
		Points []kicadfiles.Point
		Bounds []dfmRect
	}
	outlineGeometries := make([]outlineGeometry, 0, len(outlines))
	for _, outline := range outlines {
		if outline.supported() {
			points := dfmNormalizePolygon(outline.Points)
			outlineGeometries = append(outlineGeometries, outlineGeometry{Points: points, Bounds: dfmSegmentBoundsForNormalizedPolygon(points)})
		}
	}
	if len(outlineGeometries) == 0 {
		return Check{ID: CheckCopperSliverPolygonEdge, Category: CategoryCopperSliver, Status: StatusSkipped, Message: "no supported board outline polygon was found for copper polygon edge-clearance checks", Source: SourceParser}
	}
	if len(filledPolygons)+len(copperPolygons) == 0 {
		return Check{ID: CheckCopperSliverPolygonEdge, Category: CategoryCopperSliver, Status: StatusSkipped, Message: "no copper polygon geometry was found for edge-clearance checks", Source: SourceParser}
	}
	checked := 0
	unsupported := 0
	partialUnsupported := 0
	violations := 0
	minObserved := math.MaxFloat64
	var objects []string
	objectSet := map[string]struct{}{}
	addObject := func(polygon dfmPolygon) {
		objectID := polygonObjectID(polygon)
		if strings.TrimSpace(objectID) == "" {
			return
		}
		if _, ok := objectSet[objectID]; ok {
			return
		}
		objectSet[objectID] = struct{}{}
		objects = appendLimited(objects, objectID)
	}
	nets := map[string]struct{}{}
	measurePolygon := func(polygon dfmPolygon) {
		if !polygon.supported() {
			unsupported++
			addObject(polygon)
			addRef(nets, polygon.NetName)
			return
		}
		polygonPoints := dfmNormalizePolygon(polygon.Points)
		polygonBounds := dfmSegmentBoundsForNormalizedPolygon(polygonPoints)
		distance := math.Inf(1)
		hasPartialUnsupported := false
		for _, outline := range outlineGeometries {
			observed := dfmPolygonEdgeDistanceWithBoundsMM(polygonPoints, polygonBounds, outline.Points, outline.Bounds)
			if math.IsInf(observed, 1) {
				hasPartialUnsupported = true
				continue
			}
			if observed < distance {
				distance = observed
			}
		}
		if math.IsInf(distance, 1) {
			unsupported++
			addObject(polygon)
			addRef(nets, polygon.NetName)
			return
		}
		if hasPartialUnsupported {
			partialUnsupported++
			addObject(polygon)
			addRef(nets, polygon.NetName)
		}
		checked++
		if distance < minObserved {
			minObserved = distance
		}
		if distance < opts.MinCopperEdgeMM {
			violations++
			addObject(polygon)
			addRef(nets, polygon.NetName)
		}
	}
	for _, polygon := range filledPolygons {
		measurePolygon(polygon)
	}
	for _, polygon := range copperPolygons {
		measurePolygon(polygon)
	}
	measurements := []Measurement{
		{Name: "checked_polygon_count", Value: float64(checked), Unit: "count"},
		{Name: "unsupported_polygon_count", Value: float64(unsupported), Unit: "count"},
		{Name: "partial_unsupported_polygon_count", Value: float64(partialUnsupported), Unit: "count"},
		{Name: "minimum_required_copper_edge_clearance", Value: opts.MinCopperEdgeMM, Unit: "mm"},
	}
	if minObserved != math.MaxFloat64 {
		measurements = append(measurements, Measurement{Name: "minimum_observed_copper_edge_clearance", Value: minObserved, Unit: "mm"})
	}
	if violations > 0 {
		measurements = append(measurements, Measurement{Name: "violation_count", Value: float64(violations), Unit: "count"})
		return Check{
			ID:           CheckCopperSliverPolygonEdge,
			Category:     CategoryCopperSliver,
			Status:       StatusBlocked,
			Message:      "one or more copper polygons are closer to the board edge than the active profile allows",
			Suggestion:   "move polygon copper away from Edge.Cuts or select a profile that supports the modeled clearance",
			IssuePath:    "physical.copper_sliver.polygon_edge_clearance",
			Objects:      objects,
			Nets:         sortedMapKeys(nets),
			Measurements: measurements,
			Source:       SourceParser,
		}
	}
	if unsupported+partialUnsupported > 0 {
		status := unsupportedGeometryStatus(opts)
		return Check{
			ID:           CheckCopperSliverPolygonEdge,
			Category:     CategoryCopperSliver,
			Status:       status,
			Message:      strictEvidenceMessage("some copper polygon geometry could not be measured for board-edge clearance", opts),
			Suggestion:   "run KiCad DRC or expand polygon parser support before treating copper edge-clearance checks as complete",
			IssuePath:    "physical.copper_sliver.polygon_edge_clearance",
			Objects:      objects,
			Nets:         sortedMapKeys(nets),
			Measurements: measurements,
			Source:       SourceParser,
		}
	}
	return Check{ID: CheckCopperSliverPolygonEdge, Category: CategoryCopperSliver, Status: StatusPass, Message: "copper polygon edge clearances meet the active profile threshold", Measurements: measurements, Source: SourceParser}
}

func polygonObjectID(polygon dfmPolygon) string {
	if objectID := strings.TrimSpace(polygon.ObjectID); objectID != "" {
		return objectID
	}
	return polygon.SourcePath
}

func unsupportedGeometryStatus(opts Options) Status {
	if opts.Strict {
		return StatusBlocked
	}
	return StatusWarning
}

func strictEvidenceMessage(message string, opts Options) string {
	if opts.Strict {
		return message + " under strict evidence policy"
	}
	return message
}

func evaluateCopperTrackWidths(board *pcbfiles.PCBFile, opts Options) Check {
	checked := 0
	violations := 0
	var objects []string
	nets := map[string]struct{}{}
	minObserved := math.MaxFloat64
	checkWidth := func(uuid kicadfiles.UUID, width kicadfiles.IU, netName string) {
		checked++
		widthMM := iuToMM(width)
		if widthMM < minObserved {
			minObserved = widthMM
		}
		if width <= 0 || widthMM < opts.MinCopperFeatureMM {
			violations++
			objects = appendLimited(objects, string(uuid))
			if strings.TrimSpace(netName) != "" {
				nets[netName] = struct{}{}
			}
		}
	}
	for _, track := range board.Tracks {
		checkWidth(track.UUID, track.Width, track.NetName)
	}
	for _, arc := range board.TrackArcs {
		checkWidth(arc.UUID, arc.Width, arc.NetName)
	}
	for _, drawing := range board.Drawings {
		if !isCopperLayer(drawing.Layer) {
			continue
		}
		width, ok := drawingStrokeWidth(drawing)
		if !ok {
			continue
		}
		checkWidth(drawing.UUID, width, drawing.NetName)
	}
	if checked == 0 {
		return Check{ID: CheckCopperSliverTrackWidth, Category: CategoryCopperSliver, Status: StatusSkipped, Message: "no explicit copper feature widths were found for sliver checks", Source: SourceParser}
	}
	measurements := []Measurement{
		{Name: "checked_count", Value: float64(checked), Unit: "count"},
		{Name: "min_required_copper_feature", Value: opts.MinCopperFeatureMM, Unit: "mm"},
	}
	if minObserved != math.MaxFloat64 {
		measurements = append(measurements, Measurement{Name: "minimum_observed_copper_width", Value: minObserved, Unit: "mm"})
	}
	if violations == 0 {
		return Check{ID: CheckCopperSliverTrackWidth, Category: CategoryCopperSliver, Status: StatusPass, Message: "explicit copper feature widths meet the active profile threshold", Measurements: measurements, Source: SourceParser}
	}
	measurements = append(measurements, Measurement{Name: "violation_count", Value: float64(violations), Unit: "count"})
	return Check{
		ID:           CheckCopperSliverTrackWidth,
		Category:     CategoryCopperSliver,
		Status:       StatusBlocked,
		Message:      "one or more explicit copper features are narrower than the active profile threshold",
		Suggestion:   "increase track/arc/copper drawing width or select a manufacturer profile that supports the modeled feature width",
		IssuePath:    "physical.copper_sliver.track_width",
		Objects:      objects,
		Nets:         sortedMapKeys(nets),
		Measurements: measurements,
		Source:       SourceParser,
	}
}

func evaluateCopperZoneWidths(board *pcbfiles.PCBFile, opts Options) Check {
	checked := 0
	violations := 0
	var objects []string
	nets := map[string]struct{}{}
	minObserved := math.MaxFloat64
	for _, zone := range board.Zones {
		if !zoneTouchesCopper(zone) {
			continue
		}
		checked++
		widthMM := iuToMM(zone.MinThickness)
		if widthMM < minObserved {
			minObserved = widthMM
		}
		if widthMM < opts.MinCopperFeatureMM {
			violations++
			objects = appendLimited(objects, string(zone.UUID))
			if strings.TrimSpace(zone.NetName) != "" {
				nets[zone.NetName] = struct{}{}
			}
		}
	}
	if checked == 0 {
		return Check{ID: CheckCopperSliverZoneMinWidth, Category: CategoryCopperSliver, Status: StatusSkipped, Message: "no zone minimum-thickness evidence was found for copper sliver checks", Source: SourceParser}
	}
	measurements := []Measurement{
		{Name: "checked_count", Value: float64(checked), Unit: "count"},
		{Name: "min_required_copper_feature", Value: opts.MinCopperFeatureMM, Unit: "mm"},
	}
	if minObserved != math.MaxFloat64 {
		measurements = append(measurements, Measurement{Name: "minimum_observed_zone_width", Value: minObserved, Unit: "mm"})
	}
	if violations == 0 {
		return Check{ID: CheckCopperSliverZoneMinWidth, Category: CategoryCopperSliver, Status: StatusPass, Message: "zone minimum-thickness evidence meets the active profile threshold", Measurements: measurements, Source: SourceParser}
	}
	measurements = append(measurements, Measurement{Name: "violation_count", Value: float64(violations), Unit: "count"})
	return Check{
		ID:           CheckCopperSliverZoneMinWidth,
		Category:     CategoryCopperSliver,
		Status:       StatusBlocked,
		Message:      "one or more zones allow copper narrower than the active profile threshold",
		Suggestion:   "increase zone minimum thickness or verify the zone with KiCad/manufacturer DFM before fabrication export",
		IssuePath:    "physical.copper_sliver.zone_min_width",
		Objects:      objects,
		Nets:         sortedMapKeys(nets),
		Measurements: measurements,
		Source:       SourceParser,
	}
}

func evaluateUnsupportedCopperGeometry(board *pcbfiles.PCBFile) Check {
	unsupported := 0
	var objects []string
	for _, zone := range board.Zones {
		if !zoneTouchesCopper(zone) {
			continue
		}
		if strings.TrimSpace(zone.Raw) != "" && len(zone.Polygons) == 0 && len(zone.FilledPolygons) == 0 {
			unsupported++
			objects = appendLimited(objects, string(zone.UUID))
		}
	}
	if unsupported == 0 {
		return Check{ID: CheckCopperSliverUnsupported, Category: CategoryCopperSliver, Status: StatusPass, Message: "no unsupported copper geometry prevented sliver checks", Source: SourceParser}
	}
	return Check{
		ID:         CheckCopperSliverUnsupported,
		Category:   CategoryCopperSliver,
		Status:     StatusWarning,
		Message:    "one or more copper zones lack minimum-width evidence for deterministic sliver checks",
		Suggestion: "run KiCad DRC or add parsed zone minimum-thickness evidence before treating copper sliver checks as complete",
		IssuePath:  "physical.copper_sliver.unsupported_geometry",
		Objects:    objects,
		Measurements: []Measurement{
			{Name: "unsupported_count", Value: float64(unsupported), Unit: "count"},
		},
		Source: SourceHeuristic,
	}
}

func zoneTouchesCopper(zone pcbfiles.Zone) bool {
	for _, layer := range zone.Layers {
		if isCopperLayer(layer) {
			return true
		}
	}
	return false
}

func evaluatePlatedPadAnnularRings(board *pcbfiles.PCBFile, opts Options) Check {
	checked := 0
	violations := 0
	missing := 0
	var objects []string
	refs := map[string]struct{}{}
	minRing := math.MaxFloat64
	for footprintIndex := range board.Footprints {
		footprint := &board.Footprints[footprintIndex]
		footprintSuggestsHole := mountingHoleFootprintCandidate(footprint)
		for padIndex := range footprint.Pads {
			pad := &footprint.Pads[padIndex]
			if !isPlatedThroughHolePad(pad, footprintSuggestsHole) {
				continue
			}
			checked++
			outer := minPositiveIU(pad.Size.X, pad.Size.Y)
			if outer <= 0 || pad.Drill <= 0 {
				missing++
				addRef(refs, footprint.Reference)
				objects = appendLimited(objects, string(pad.UUID))
				continue
			}
			ring := iuToMM(outer-pad.Drill) / 2
			if ring < minRing {
				minRing = ring
			}
			if pad.Drill >= outer || ring < opts.MinPlatedPadAnnularRingMM {
				violations++
				addRef(refs, footprint.Reference)
				objects = appendLimited(objects, string(pad.UUID))
			}
		}
	}
	if checked == 0 {
		return Check{ID: CheckAnnularRingPlatedPad, Category: CategoryAnnularRing, Status: StatusSkipped, Message: "no plated through-hole pads were found for annular ring checks", Source: SourceParser}
	}
	measurements := []Measurement{
		{Name: "checked_count", Value: float64(checked), Unit: "count"},
		{Name: "min_required_annular_ring", Value: opts.MinPlatedPadAnnularRingMM, Unit: "mm"},
	}
	if minRing != math.MaxFloat64 {
		measurements = append(measurements, Measurement{Name: "minimum_observed_annular_ring", Value: minRing, Unit: "mm"})
	}
	if violations > 0 {
		measurements = append(measurements, Measurement{Name: "violation_count", Value: float64(violations), Unit: "count"})
		return Check{
			ID:           CheckAnnularRingPlatedPad,
			Category:     CategoryAnnularRing,
			Status:       StatusBlocked,
			Message:      "one or more plated through-hole pads have insufficient annular ring",
			Suggestion:   "increase pad diameter, reduce drill diameter, or select a manufacturer profile that supports the modeled geometry",
			IssuePath:    "physical.annular_ring.plated_pad",
			References:   sortedMapKeys(refs),
			Objects:      objects,
			Measurements: measurements,
			Source:       SourceParser,
		}
	}
	if missing > 0 {
		measurements = append(measurements, Measurement{Name: "missing_geometry_count", Value: float64(missing), Unit: "count"})
		return Check{
			ID:           CheckAnnularRingPlatedPad,
			Category:     CategoryAnnularRing,
			Status:       StatusWarning,
			Message:      "one or more likely plated pads are missing drill or outer diameter evidence",
			Suggestion:   "hydrate pad geometry before treating annular ring evidence as fabrication-ready",
			IssuePath:    "physical.annular_ring.plated_pad",
			References:   sortedMapKeys(refs),
			Objects:      objects,
			Measurements: measurements,
			Source:       SourceParser,
		}
	}
	return Check{ID: CheckAnnularRingPlatedPad, Category: CategoryAnnularRing, Status: StatusPass, Message: "plated through-hole pad annular rings meet the active profile threshold", Measurements: measurements, Source: SourceParser}
}

func evaluateViaAnnularRings(board *pcbfiles.PCBFile, opts Options) Check {
	if len(board.Vias) == 0 {
		return Check{ID: CheckAnnularRingVia, Category: CategoryAnnularRing, Status: StatusSkipped, Message: "no vias were found for annular ring checks", Source: SourceParser}
	}
	violations := 0
	missing := 0
	var objects []string
	nets := map[string]struct{}{}
	minRing := math.MaxFloat64
	for _, via := range board.Vias {
		if via.Size <= 0 || via.Drill <= 0 {
			missing++
			objects = appendLimited(objects, string(via.UUID))
			if strings.TrimSpace(via.NetName) != "" {
				nets[via.NetName] = struct{}{}
			}
			continue
		}
		ring := iuToMM(via.Size-via.Drill) / 2
		if ring < minRing {
			minRing = ring
		}
		if via.Drill >= via.Size || ring < opts.MinViaRingMM {
			violations++
			objects = appendLimited(objects, string(via.UUID))
			if strings.TrimSpace(via.NetName) != "" {
				nets[via.NetName] = struct{}{}
			}
		}
	}
	measurements := []Measurement{
		{Name: "checked_count", Value: float64(len(board.Vias)), Unit: "count"},
		{Name: "min_required_annular_ring", Value: opts.MinViaRingMM, Unit: "mm"},
	}
	if minRing != math.MaxFloat64 {
		measurements = append(measurements, Measurement{Name: "minimum_observed_annular_ring", Value: minRing, Unit: "mm"})
	}
	if violations > 0 {
		measurements = append(measurements, Measurement{Name: "violation_count", Value: float64(violations), Unit: "count"})
		return Check{
			ID:           CheckAnnularRingVia,
			Category:     CategoryAnnularRing,
			Status:       StatusBlocked,
			Message:      "one or more vias have insufficient annular ring",
			Suggestion:   "increase via diameter, reduce via drill, or select a manufacturer profile that supports the modeled geometry",
			IssuePath:    "physical.annular_ring.via",
			Objects:      objects,
			Nets:         sortedMapKeys(nets),
			Measurements: measurements,
			Source:       SourceParser,
		}
	}
	if missing > 0 {
		measurements = append(measurements, Measurement{Name: "missing_geometry_count", Value: float64(missing), Unit: "count"})
		return Check{
			ID:           CheckAnnularRingVia,
			Category:     CategoryAnnularRing,
			Status:       StatusWarning,
			Message:      "one or more vias are missing drill or diameter evidence",
			Suggestion:   "hydrate via geometry before treating annular ring evidence as fabrication-ready",
			IssuePath:    "physical.annular_ring.via",
			Objects:      objects,
			Nets:         sortedMapKeys(nets),
			Measurements: measurements,
			Source:       SourceParser,
		}
	}
	return Check{ID: CheckAnnularRingVia, Category: CategoryAnnularRing, Status: StatusPass, Message: "via annular rings meet the active profile threshold", Measurements: measurements, Source: SourceParser}
}

func evaluateCourtyardSilkscreen(board *pcbfiles.PCBFile, bounds boardBounds) []Check {
	var courtyards []courtyardBounds
	var missingCourtyardRefs = map[string]struct{}{}
	var missingCourtyardObjects []string
	var silkOutsideRefs = map[string]struct{}{}
	var silkOutsideObjects []string
	missingCourtyardCount := 0
	silkOutsideCount := 0
	for footprintIndex := range board.Footprints {
		footprint := &board.Footprints[footprintIndex]
		courtyard, ok := footprintCourtyardBounds(footprint)
		if ok {
			courtyards = append(courtyards, courtyardBounds{Reference: footprint.Reference, Bounds: courtyard})
		} else if footprintNeedsCourtyard(footprint) {
			missingCourtyardCount++
			addRef(missingCourtyardRefs, footprint.Reference)
			missingCourtyardObjects = appendLimited(missingCourtyardObjects, string(footprint.UUID))
		}
		if bounds.Valid {
			for _, text := range footprint.Texts {
				if isSilkscreenLayer(text.Layer) && !silkscreenTextInsideBoard(bounds, footprint, text) {
					silkOutsideCount++
					addRef(silkOutsideRefs, footprint.Reference)
					silkOutsideObjects = appendLimited(silkOutsideObjects, string(text.UUID))
				}
			}
			for _, graphic := range footprint.Graphics {
				drawing := pcbfiles.Drawing(graphic)
				if isSilkscreenLayer(drawing.Layer) && !allPointsInsideBoard(bounds, transformFootprintPoints(footprint, drawingPoints(drawing))) {
					silkOutsideCount++
					addRef(silkOutsideRefs, footprint.Reference)
					silkOutsideObjects = appendLimited(silkOutsideObjects, string(drawing.UUID))
				}
			}
		}
	}
	checks := []Check{}
	if missingCourtyardCount == 0 {
		checks = append(checks, Check{ID: CheckCourtyardPresence, Category: CategoryCourtyard, Status: StatusPass, Message: "assembly footprints have courtyard evidence or do not require it", Source: SourceParser})
	} else {
		checks = append(checks, Check{
			ID:         CheckCourtyardPresence,
			Category:   CategoryCourtyard,
			Status:     StatusWarning,
			Message:    "one or more assembly footprints are missing courtyard graphics",
			Suggestion: "hydrate footprints from KiCad libraries or add courtyard graphics before fabrication release",
			IssuePath:  "physical.courtyard.presence",
			References: sortedMapKeys(missingCourtyardRefs),
			Objects:    missingCourtyardObjects,
			Measurements: []Measurement{
				{Name: "violation_count", Value: float64(missingCourtyardCount), Unit: "count"},
			},
			Source: SourceParser,
		})
	}
	overlapRefs, overlapCount := courtyardOverlaps(courtyards)
	if overlapCount == 0 {
		checks = append(checks, Check{ID: CheckCourtyardOverlap, Category: CategoryCourtyard, Status: StatusPass, Message: "footprint courtyard bounds do not overlap", Source: SourceParser})
	} else {
		checks = append(checks, Check{
			ID:         CheckCourtyardOverlap,
			Category:   CategoryCourtyard,
			Status:     StatusBlocked,
			Message:    "one or more footprint courtyard bounds overlap",
			Suggestion: "move overlapping footprints apart before fabrication export",
			IssuePath:  "physical.courtyard.overlap",
			References: overlapRefs,
			Measurements: []Measurement{
				{Name: "violation_count", Value: float64(overlapCount), Unit: "count"},
			},
			Source: SourceParser,
		})
	}
	checks = append(checks, Check{
		ID:       CheckSilkscreenPadClearance,
		Category: CategorySilkscreen,
		Status:   StatusSkipped,
		Message:  "silkscreen-to-pad clearance requires rendered text and stroke geometry and is deferred to KiCad DRC evidence",
		Source:   SourceHeuristic,
	})
	if silkOutsideCount == 0 {
		status := StatusPass
		message := "silkscreen reference points and graphics are inside board bounds"
		if !bounds.Valid {
			status = StatusSkipped
			message = "silkscreen board-clearance check skipped because no usable Edge.Cuts bounds were found"
		}
		checks = append(checks, Check{ID: CheckSilkscreenBoardClearance, Category: CategorySilkscreen, Status: status, Message: message, Source: SourceParser})
	} else {
		checks = append(checks, Check{
			ID:         CheckSilkscreenBoardClearance,
			Category:   CategorySilkscreen,
			Status:     StatusBlocked,
			Message:    "one or more silkscreen objects are outside board bounds",
			Suggestion: "move, hide, or clip silkscreen so it stays inside Edge.Cuts",
			IssuePath:  "physical.silkscreen.board_clearance",
			References: sortedMapKeys(silkOutsideRefs),
			Objects:    silkOutsideObjects,
			Measurements: []Measurement{
				{Name: "violation_count", Value: float64(silkOutsideCount), Unit: "count"},
			},
			Source: SourceParser,
		})
	}
	checks = append(checks, Check{ID: CheckSilkscreenReference, Category: CategorySilkscreen, Status: StatusPass, Message: "silkscreen reference text presence is covered by existing footprint text data", Source: SourceParser})
	return checks
}

func evaluateMountingHoles(board *pcbfiles.PCBFile, bounds boardBounds, opts Options) []Check {
	var holes []mountingHole
	for footprintIndex := range board.Footprints {
		footprint := &board.Footprints[footprintIndex]
		footprintSuggestsHole := mountingHoleFootprintCandidate(footprint)
		for padIndex := range footprint.Pads {
			pad := &footprint.Pads[padIndex]
			if isMountingHole(pad, footprintSuggestsHole) {
				center := transformFootprintPoint(footprint, pad.Position)
				holes = append(holes, mountingHole{Reference: footprint.Reference, UUID: string(pad.UUID), Center: center, Drill: pad.Drill})
			}
		}
	}
	checks := []Check{}
	if opts.RequireMountingHoles && len(holes) == 0 {
		checks = append(checks, Check{
			ID:         CheckMountingHolePresence,
			Category:   CategoryMountingHole,
			Status:     StatusBlocked,
			Message:    "mounting holes are required but none were found",
			Suggestion: "add NPTH mounting holes with edge clearance and keepout evidence",
			IssuePath:  "physical.mounting_hole.presence",
			Source:     SourceProfile,
		})
	} else if len(holes) == 0 {
		checks = append(checks, Check{ID: CheckMountingHolePresence, Category: CategoryMountingHole, Status: StatusSkipped, Message: "no mounting holes were required or detected", Source: SourceProfile})
	} else {
		checks = append(checks, Check{ID: CheckMountingHolePresence, Category: CategoryMountingHole, Status: StatusPass, Message: "mounting holes were detected", Measurements: []Measurement{{Name: "hole_count", Value: float64(len(holes)), Unit: "count"}}, Source: SourceParser})
	}
	var geometryObjects []string
	var geometryRefs = map[string]struct{}{}
	geometryViolations := 0
	var edgeObjects []string
	var edgeRefs = map[string]struct{}{}
	edgeViolations := 0
	minEdge := opts.MinHoleEdgeMM
	minObservedEdge := math.Inf(1)
	for _, hole := range holes {
		if hole.Drill <= 0 {
			geometryViolations++
			addRef(geometryRefs, hole.Reference)
			geometryObjects = appendLimited(geometryObjects, hole.UUID)
		}
		if bounds.Valid && minEdge > 0 && len(bounds.Polygons) > 0 {
			clearance := pointPolygonDistance(hole.Center, bounds.Polygons) - math.Max(0, iuToMM(hole.Drill))/2
			if clearance < minObservedEdge {
				minObservedEdge = clearance
			}
			if clearance < minEdge {
				edgeViolations++
				addRef(edgeRefs, hole.Reference)
				edgeObjects = appendLimited(edgeObjects, hole.UUID)
			}
		}
	}
	if len(holes) == 0 {
		checks = append(checks, Check{ID: CheckMountingHoleGeometry, Category: CategoryMountingHole, Status: StatusSkipped, Message: "mounting-hole geometry check skipped because no mounting holes were detected", Source: SourceParser})
	} else if geometryViolations == 0 {
		checks = append(checks, Check{ID: CheckMountingHoleGeometry, Category: CategoryMountingHole, Status: StatusPass, Message: "detected mounting holes have positive drill sizes", Source: SourceParser})
	} else {
		checks = append(checks, Check{
			ID:         CheckMountingHoleGeometry,
			Category:   CategoryMountingHole,
			Status:     StatusBlocked,
			Message:    "one or more mounting holes have invalid drill geometry",
			Suggestion: "set a positive drill diameter for each mounting hole",
			IssuePath:  "physical.mounting_hole.geometry",
			References: sortedMapKeys(geometryRefs),
			Objects:    geometryObjects,
			Measurements: []Measurement{
				{Name: "violation_count", Value: float64(geometryViolations), Unit: "count"},
			},
			Source: SourceParser,
		})
	}
	if len(holes) == 0 || minEdge <= 0 || !bounds.Valid || len(bounds.Polygons) == 0 {
		checks = append(checks, Check{ID: CheckMountingHoleEdgeClearance, Category: CategoryMountingHole, Status: StatusSkipped, Message: "mounting-hole edge clearance requires detected holes, board polygon bounds, and a minimum edge-clearance policy", Source: SourceProfile})
	} else if edgeViolations == 0 {
		checks = append(checks, Check{ID: CheckMountingHoleEdgeClearance, Category: CategoryMountingHole, Status: StatusPass, Message: "mounting holes satisfy minimum edge clearance", Measurements: []Measurement{{Name: "min_hole_edge_clearance", Value: minEdge, Unit: "mm"}, {Name: "observed_min_hole_edge_clearance", Value: minObservedEdge, Unit: "mm"}}, Source: SourceProfile})
	} else {
		checks = append(checks, Check{
			ID:         CheckMountingHoleEdgeClearance,
			Category:   CategoryMountingHole,
			Status:     StatusBlocked,
			Message:    "one or more mounting holes violate minimum edge clearance",
			Suggestion: "move mounting holes farther from Edge.Cuts or lower the profile threshold after review",
			IssuePath:  "physical.mounting_hole.edge_clearance",
			References: sortedMapKeys(edgeRefs),
			Objects:    edgeObjects,
			Measurements: []Measurement{
				{Name: "min_hole_edge_clearance", Value: minEdge, Unit: "mm"},
				{Name: "observed_min_hole_edge_clearance", Value: minObservedEdge, Unit: "mm"},
				{Name: "violation_count", Value: float64(edgeViolations), Unit: "count"},
			},
			Source: SourceProfile,
		})
	}
	return checks
}

type padLayerSummary struct {
	FCu     bool
	BCu     bool
	AllCu   bool
	FMask   bool
	BMask   bool
	AllMask bool
	FPaste  bool
	BPaste  bool
}

type maskWebPad struct {
	Reference string
	UUID      string
	Side      kicadfiles.BoardLayer
	Bounds    rectBounds
	Rotated   bool
}

func summarizePadLayers(pad *pcbfiles.Pad) padLayerSummary {
	var summary padLayerSummary
	for _, layer := range pad.Layers {
		switch layer {
		case kicadfiles.LayerFCu:
			summary.FCu = true
		case kicadfiles.LayerBCu:
			summary.BCu = true
		case kicadfiles.LayerAllCu:
			summary.AllCu = true
			summary.FCu = true
			summary.BCu = true
		case kicadfiles.LayerFMask:
			summary.FMask = true
		case kicadfiles.LayerBMask:
			summary.BMask = true
		case kicadfiles.LayerAllMask:
			summary.AllMask = true
		case kicadfiles.LayerFPaste:
			summary.FPaste = true
		case kicadfiles.LayerBPaste:
			summary.BPaste = true
		}
	}
	return summary
}

func (summary padLayerSummary) requiresMask() bool {
	return summary.FCu || summary.BCu || summary.AllCu
}

func (summary padLayerSummary) hasRequiredMask() bool {
	if summary.AllMask {
		return true
	}
	if summary.AllCu {
		return summary.FMask && summary.BMask
	}
	if summary.FCu && !summary.FMask {
		return false
	}
	if summary.BCu && !summary.BMask {
		return false
	}
	return true
}

func padRequiresPaste(pad *pcbfiles.Pad) bool {
	return strings.EqualFold(pad.Type, "smd")
}

func maskWebPads(board *pcbfiles.PCBFile) []maskWebPad {
	var pads []maskWebPad
	for footprintIndex := range board.Footprints {
		footprint := &board.Footprints[footprintIndex]
		transform := footprintTransform(footprint)
		for padIndex := range footprint.Pads {
			pad := &footprint.Pads[padIndex]
			layers := summarizePadLayers(pad)
			bounds := transformedPadBounds(footprint, transform, pad)
			rotated := nonOrthogonalPadRotation(footprint, pad)
			if layers.FCu && layers.FMask {
				pads = append(pads, maskWebPad{Reference: footprint.Reference, UUID: string(pad.UUID), Side: kicadfiles.LayerFCu, Bounds: bounds, Rotated: rotated})
			}
			if layers.BCu && layers.BMask {
				pads = append(pads, maskWebPad{Reference: footprint.Reference, UUID: string(pad.UUID), Side: kicadfiles.LayerBCu, Bounds: bounds, Rotated: rotated})
			}
		}
	}
	return pads
}

func nonOrthogonalPadRotation(footprint *pcbfiles.Footprint, pad *pcbfiles.Pad) bool {
	rotation := int(math.Round(float64(footprint.Rotation + pad.Rotation)))
	rotation %= 90
	if rotation < 0 {
		rotation += 90
	}
	return rotation != 0
}

func transformedPadBounds(footprint *pcbfiles.Footprint, transform transform2D, pad *pcbfiles.Pad) rectBounds {
	var bounds rectBounds
	if pad.Size.X <= 0 || pad.Size.Y <= 0 {
		return bounds
	}
	halfX := float64(pad.Size.X) / 2
	halfY := float64(pad.Size.Y) / 2
	padRadians := float64(pad.Rotation) * math.Pi / 180
	padCosine := math.Cos(padRadians)
	padSine := math.Sin(padRadians)
	for _, corner := range []struct{ x, y float64 }{
		{-halfX, -halfY},
		{-halfX, halfY},
		{halfX, -halfY},
		{halfX, halfY},
	} {
		rotatedX := corner.x*padCosine - corner.y*padSine
		rotatedY := corner.x*padSine + corner.y*padCosine
		local := kicadfiles.Point{
			X: pad.Position.X + kicadfiles.IU(math.Round(rotatedX)),
			Y: pad.Position.Y + kicadfiles.IU(math.Round(rotatedY)),
		}
		bounds = includeRectPoint(bounds, transformFootprintPointWith(footprint, transform, local))
	}
	return bounds
}

func rectSpacingMM(a, b rectBounds) float64 {
	dx := kicadfiles.IU(0)
	switch {
	case a.MaxX < b.MinX:
		dx = b.MinX - a.MaxX
	case b.MaxX < a.MinX:
		dx = a.MinX - b.MaxX
	}
	dy := kicadfiles.IU(0)
	switch {
	case a.MaxY < b.MinY:
		dy = b.MinY - a.MaxY
	case b.MaxY < a.MinY:
		dy = a.MinY - b.MaxY
	}
	if dx == 0 {
		return iuToMM(dy)
	}
	if dy == 0 {
		return iuToMM(dx)
	}
	return math.Hypot(iuToMM(dx), iuToMM(dy))
}

func rectDistanceToBoardEdgeMM(bounds boardBounds, rect rectBounds) float64 {
	if !bounds.Valid || !rect.Valid {
		return math.MaxFloat64
	}
	if rect.MaxX < bounds.MinX || rect.MinX > bounds.MaxX || rect.MaxY < bounds.MinY || rect.MinY > bounds.MaxY {
		return math.MaxFloat64
	}
	distances := []kicadfiles.IU{
		rect.MinX - bounds.MinX,
		bounds.MaxX - rect.MaxX,
		rect.MinY - bounds.MinY,
		bounds.MaxY - rect.MaxY,
	}
	minDistance := distances[0]
	for _, distance := range distances[1:] {
		if absIU(distance) < absIU(minDistance) {
			minDistance = distance
		}
	}
	return math.Abs(iuToMM(minDistance))
}

func footprintSuggestsEdgePlating(footprint *pcbfiles.Footprint) bool {
	text := strings.ToLower(strings.Join([]string{
		footprint.LibraryID,
		footprint.Value,
		footprint.Description,
		footprint.Tags,
	}, " "))
	return strings.Contains(text, "castell") ||
		strings.Contains(text, "edge_plat") ||
		strings.Contains(text, "edge plat")
}

func isPotentialEdgePlatedPad(pad *pcbfiles.Pad, footprintSuggestsEdgePlating bool) bool {
	if !footprintSuggestsEdgePlating && !strings.EqualFold(pad.Type, "thru_hole") {
		return false
	}
	if strings.EqualFold(pad.Type, "np_thru_hole") {
		return false
	}
	layers := summarizePadLayers(pad)
	return layers.FCu || layers.BCu || layers.AllCu
}

func isPlatedThroughHolePad(pad *pcbfiles.Pad, footprintSuggestsHole bool) bool {
	if strings.EqualFold(pad.Type, "np_thru_hole") || isMountingHole(pad, footprintSuggestsHole) {
		return false
	}
	return strings.EqualFold(pad.Type, "thru_hole")
}

func minPositiveIU(a, b kicadfiles.IU) kicadfiles.IU {
	switch {
	case a > 0 && b > 0 && a < b:
		return a
	case a > 0 && b > 0:
		return b
	case a > 0:
		return a
	default:
		return b
	}
}

func (summary padLayerSummary) hasRequiredPaste() bool {
	return summary.FCu == summary.FPaste && summary.BCu == summary.BPaste
}

func (summary padLayerSummary) hasAnyPaste() bool {
	return summary.FPaste || summary.BPaste
}

type boardBounds struct {
	Valid       bool
	Rectangular bool
	Polygons    [][]kicadfiles.Point
	MinX        kicadfiles.IU
	MinY        kicadfiles.IU
	MaxX        kicadfiles.IU
	MaxY        kicadfiles.IU
}

type rectBounds struct {
	Valid bool
	MinX  kicadfiles.IU
	MinY  kicadfiles.IU
	MaxX  kicadfiles.IU
	MaxY  kicadfiles.IU
}

type courtyardBounds struct {
	Reference string
	Bounds    rectBounds
}

type mountingHole struct {
	Reference string
	UUID      string
	Center    kicadfiles.Point
	Drill     kicadfiles.IU
}

type edgeCutResult struct {
	Checks []Check
	Bounds boardBounds
}

func evaluateStackup(board *pcbfiles.PCBFile) []Check {
	var checks []Check
	copperLayers := copperLayers(board)
	layerNames := boardLayerNames(copperLayers)
	switch {
	case len(copperLayers) == 0:
		checks = append(checks, Check{
			ID:         CheckStackupCopperLayers,
			Category:   CategoryStackup,
			Status:     StatusBlocked,
			Message:    "PCB has no enabled copper layers",
			Suggestion: "enable at least F.Cu and B.Cu before fabrication export",
			IssuePath:  "physical.stackup.copper_layers",
			Layers:     layerNames,
			Source:     SourceParser,
		})
	case !hasLayer(copperLayers, kicadfiles.LayerFCu) || !hasLayer(copperLayers, kicadfiles.LayerBCu):
		checks = append(checks, Check{
			ID:         CheckStackupCopperLayers,
			Category:   CategoryStackup,
			Status:     StatusWarning,
			Message:    "PCB copper stackup does not include both F.Cu and B.Cu",
			Suggestion: "review the stackup before fabrication; generated two-layer boards should normally include both outer copper layers",
			IssuePath:  "physical.stackup.copper_layers",
			Layers:     layerNames,
			Source:     SourceParser,
		})
	default:
		checks = append(checks, Check{
			ID:       CheckStackupCopperLayers,
			Category: CategoryStackup,
			Status:   StatusPass,
			Message:  "PCB copper layers are valid",
			Layers:   layerNames,
			Source:   SourceParser,
		})
	}

	thickness := board.Setup.Stackup.Thickness
	if thickness <= 0 {
		thickness = board.General.Thickness
	}
	if thickness <= 0 {
		checks = append(checks, Check{
			ID:         CheckStackupThickness,
			Category:   CategoryStackup,
			Status:     StatusBlocked,
			Message:    "PCB stackup thickness is missing or non-positive",
			Suggestion: "set a positive board thickness before fabrication export",
			IssuePath:  "physical.stackup.thickness",
			Source:     SourceParser,
		})
	} else {
		checks = append(checks, Check{
			ID:           CheckStackupThickness,
			Category:     CategoryStackup,
			Status:       StatusPass,
			Message:      "PCB stackup thickness is positive",
			Measurements: []Measurement{{Name: "thickness", Value: iuToMM(thickness), Unit: "mm"}},
			Source:       SourceParser,
		})
	}
	if board.Setup.SolderMaskMinWidth < 0 {
		checks = append(checks, Check{
			ID:         CheckStackupSolderMask,
			Category:   CategoryStackup,
			Status:     StatusBlocked,
			Message:    "PCB solder mask setup contains a negative minimum width",
			Suggestion: "use a non-negative solder mask minimum width; pad-to-mask clearance may be negative for solder-mask-defined pads",
			IssuePath:  "physical.stackup.solder_mask",
			Source:     SourceParser,
		})
	} else {
		checks = append(checks, Check{
			ID:           CheckStackupSolderMask,
			Category:     CategoryStackup,
			Status:       StatusPass,
			Message:      "PCB solder mask setup is non-negative",
			Measurements: []Measurement{{Name: "solder_mask_min_width", Value: iuToMM(board.Setup.SolderMaskMinWidth), Unit: "mm"}, {Name: "pad_to_mask_clearance", Value: iuToMM(board.Setup.PadToMaskClearance), Unit: "mm"}},
			Source:       SourceParser,
		})
	}
	return checks
}

func evaluateNetClasses(board *pcbfiles.PCBFile, project *projectfiles.ProjectFile) []Check {
	var checks []Check
	routedNets := routedNetNames(board)
	if project == nil {
		status := StatusSkipped
		message := "project net-class settings were not provided"
		if len(routedNets) > 0 {
			status = StatusWarning
			message = "routed PCB nets cannot be matched to project net-class settings"
		}
		return []Check{{
			ID:         CheckNetClassDefault,
			Category:   CategoryNetClass,
			Status:     status,
			Message:    message,
			Suggestion: "load the .kicad_pro file so fabrication checks can verify effective net classes",
			IssuePath:  "physical.net_class.default",
			Nets:       routedNets,
			Source:     SourceParser,
		}}
	}
	defaultClass, ok := defaultNetClass(project.NetClasses)
	if !ok {
		checks = append(checks, Check{
			ID:         CheckNetClassDefault,
			Category:   CategoryNetClass,
			Status:     StatusBlocked,
			Message:    "project has no Default net class",
			Suggestion: "add a Default net class with trace, clearance, via diameter, and via drill values",
			IssuePath:  "physical.net_class.default",
			Source:     SourceParser,
		})
	} else if invalidNetClass(defaultClass) {
		checks = append(checks, Check{
			ID:         CheckNetClassDefault,
			Category:   CategoryNetClass,
			Status:     StatusBlocked,
			Message:    "Default net class has invalid trace, clearance, or via dimensions",
			Suggestion: "use positive trace width, via diameter, via drill, and clearance values; via drill must be smaller than via diameter",
			IssuePath:  "physical.net_class.default",
			Source:     SourceParser,
		})
	} else {
		checks = append(checks, Check{
			ID:           CheckNetClassDefault,
			Category:     CategoryNetClass,
			Status:       StatusPass,
			Message:      "Default net class has valid fabrication dimensions",
			Measurements: netClassMeasurements(defaultClass),
			Source:       SourceParser,
		})
	}
	if len(routedNets) == 0 {
		checks = append(checks, Check{
			ID:       CheckNetClassEffectiveRules,
			Category: CategoryNetClass,
			Status:   StatusSkipped,
			Message:  "PCB has no routed nets requiring effective net-class checks",
			Source:   SourceParser,
		})
		return checks
	}
	if ok && !invalidNetClass(defaultClass) {
		checks = append(checks, Check{
			ID:       CheckNetClassEffectiveRules,
			Category: CategoryNetClass,
			Status:   StatusPass,
			Message:  "routed PCB nets have Default net-class fallback fabrication rules",
			Nets:     routedNets,
			Source:   SourceParser,
		})
		if len(project.NetClasses) > 1 {
			checks = append(checks, evaluateTrackWidths(board, defaultClass, false)...)
			checks = append(checks, evaluateViaDimensions(board, defaultClass, false)...)
			checks = append(checks, Check{
				ID:         CheckNetClassAssignmentCoverage,
				Category:   CategoryNetClass,
				Status:     StatusWarning,
				Message:    "project has multiple net classes but per-net class assignments are not modeled by the current checker",
				Suggestion: "use KiCad DRC evidence or extend project parsing before relying on non-Default class-specific physical checks",
				IssuePath:  "physical.net_class.effective_rules",
				Nets:       routedNets,
				Source:     SourceParser,
			})
		} else {
			checks = append(checks, evaluateTrackWidths(board, defaultClass, true)...)
			checks = append(checks, evaluateViaDimensions(board, defaultClass, true)...)
		}
	}
	return checks
}

func evaluateImpedanceEvidence(board *pcbfiles.PCBFile, project *projectfiles.ProjectFile, opts Options) []Check {
	if project == nil {
		return []Check{
			{ID: CheckImpedanceStackupEvidence, Category: CategoryImpedance, Status: StatusSkipped, Message: "controlled-impedance evidence skipped because project net classes are unavailable", Source: SourceParser},
			{ID: CheckImpedanceWidthGapEvidence, Category: CategoryImpedance, Status: StatusSkipped, Message: "controlled-impedance width/gap evidence skipped because project net classes are unavailable", Source: SourceParser},
			{ID: CheckDiffPairFabrication, Category: CategoryDifferentialPair, Status: StatusSkipped, Message: "differential-pair fabrication evidence skipped because project net classes are unavailable", Source: SourceParser},
		}
	}
	impedanceClasses, diffClasses := impedanceIntentClasses(project.NetClasses)
	if len(impedanceClasses) == 0 && len(diffClasses) == 0 {
		return []Check{
			{ID: CheckImpedanceStackupEvidence, Category: CategoryImpedance, Status: StatusSkipped, Message: "no controlled-impedance net-class intent was detected", Source: SourceParser},
			{ID: CheckImpedanceWidthGapEvidence, Category: CategoryImpedance, Status: StatusSkipped, Message: "no controlled-impedance width/gap intent was detected", Source: SourceParser},
			{ID: CheckDiffPairFabrication, Category: CategoryDifferentialPair, Status: StatusSkipped, Message: "no differential-pair net-class intent was detected", Source: SourceParser},
		}
	}
	status := policyStatus(opts.ImpedancePolicy)
	severity := reports.SeverityWarning
	if status == StatusBlocked {
		severity = reports.SeverityError
	}
	stackupStatus := status
	stackupMessage := "controlled-impedance intent lacks solver-grade stackup/material evidence"
	if opts.ImpedancePolicy == PolicyIgnore {
		stackupStatus = StatusSkipped
		stackupMessage = "controlled-impedance stackup evidence ignored by active profile policy"
		severity = ""
	}
	widthStatus := status
	widthMessage := "controlled-impedance intent has trace-width evidence but no solver-grade width/gap proof"
	if opts.ImpedancePolicy == PolicyIgnore {
		widthStatus = StatusSkipped
		widthMessage = "controlled-impedance width/gap evidence ignored by active profile policy"
	}
	diffStatus := status
	diffMessage := "differential-pair intent lacks solver-grade pair spacing and length-match fabrication evidence"
	if opts.ImpedancePolicy == PolicyIgnore {
		diffStatus = StatusSkipped
		diffMessage = "differential-pair fabrication evidence ignored by active profile policy"
	}
	measurements := []Measurement{
		{Name: "copper_layer_count", Value: float64(copperLayerCount(board)), Unit: "count"},
		{Name: "controlled_impedance_class_count", Value: float64(len(impedanceClasses)), Unit: "count"},
		{Name: "differential_pair_class_count", Value: float64(len(diffClasses)), Unit: "count"},
	}
	return []Check{
		{
			ID:           CheckImpedanceStackupEvidence,
			Category:     CategoryImpedance,
			Status:       stackupStatus,
			Severity:     severity,
			Message:      stackupMessage,
			Suggestion:   "add explicit stackup/material/impedance proof or keep the board below fabrication-ready status",
			IssuePath:    "physical.impedance.stackup_evidence",
			Objects:      impedanceClasses,
			Measurements: measurements,
			Source:       SourceHeuristic,
		},
		{
			ID:           CheckImpedanceWidthGapEvidence,
			Category:     CategoryImpedance,
			Status:       widthStatus,
			Severity:     severity,
			Message:      widthMessage,
			Suggestion:   "provide modeled impedance width/gap evidence or defer to manufacturer/KiCad DRC evidence",
			IssuePath:    "physical.impedance.width_gap_evidence",
			Objects:      impedanceClasses,
			Measurements: measurements,
			Source:       SourceHeuristic,
		},
		{
			ID:           CheckDiffPairFabrication,
			Category:     CategoryDifferentialPair,
			Status:       diffStatus,
			Severity:     severity,
			Message:      diffMessage,
			Suggestion:   "provide explicit pair width, gap, skew, and stackup evidence before claiming differential-pair fabrication readiness",
			IssuePath:    "physical.differential_pair.fabrication_evidence",
			Objects:      diffClasses,
			Measurements: measurements,
			Source:       SourceHeuristic,
		},
	}
}

func impedanceIntentClasses(classes []projectfiles.NetClass) ([]string, []string) {
	var impedance []string
	var differential []string
	for _, class := range classes {
		name := strings.ToLower(strings.TrimSpace(class.Name))
		switch {
		case strings.Contains(name, "diff") || strings.Contains(name, "differential"):
			differential = append(differential, class.Name)
			impedance = append(impedance, class.Name)
		case strings.Contains(name, "impedance") || strings.Contains(name, "controlled") || strings.Contains(name, "usb") || strings.Contains(name, "clock"):
			impedance = append(impedance, class.Name)
		}
	}
	return cleanStrings(impedance), cleanStrings(differential)
}

func policyStatus(policy Policy) Status {
	switch policy {
	case PolicyBlock:
		return StatusBlocked
	case PolicyIgnore:
		return StatusSkipped
	default:
		return StatusWarning
	}
}

func evaluateTrackWidths(board *pcbfiles.PCBFile, class projectfiles.NetClass, blocking bool) []Check {
	var low []string
	violationCount := 0
	nets := map[string]struct{}{}
	for _, track := range board.Tracks {
		if class.TrackWidth > 0 && track.Width < class.TrackWidth {
			violationCount++
			low = appendLimited(low, string(track.UUID))
			if strings.TrimSpace(track.NetName) != "" {
				nets[track.NetName] = struct{}{}
			}
		}
	}
	if len(low) == 0 {
		return []Check{{
			ID:           CheckNetClassRoutedWidth,
			Category:     CategoryNetClass,
			Status:       StatusPass,
			Message:      "routed track widths meet the Default net-class width",
			Measurements: []Measurement{{Name: "default_track_width", Value: iuToMM(class.TrackWidth), Unit: "mm"}},
			Source:       SourceParser,
		}}
	}
	status := StatusBlocked
	severity := reports.SeverityError
	suggestion := "reroute narrow tracks or assign a verified narrower net class before fabrication export"
	if !blocking {
		status = StatusWarning
		severity = reports.SeverityWarning
		suggestion = "verify net-specific track widths with KiCad DRC until net-class assignment parsing is modeled"
	}
	return []Check{{
		ID:         CheckNetClassRoutedWidth,
		Category:   CategoryNetClass,
		Status:     status,
		Severity:   severity,
		Message:    "one or more routed tracks are narrower than the Default net-class width",
		Suggestion: suggestion,
		IssuePath:  "physical.net_class.routed_width",
		Objects:    low,
		Nets:       sortedMapKeys(nets),
		Measurements: []Measurement{
			{Name: "default_track_width", Value: iuToMM(class.TrackWidth), Unit: "mm"},
			{Name: "violation_count", Value: float64(violationCount), Unit: "count"},
		},
		Source: SourceParser,
	}}
}

func evaluateViaDimensions(board *pcbfiles.PCBFile, class projectfiles.NetClass, blocking bool) []Check {
	var low []string
	violationCount := 0
	nets := map[string]struct{}{}
	for _, via := range board.Vias {
		if (class.ViaDiameter > 0 && via.Size < class.ViaDiameter) || (class.ViaDrill > 0 && via.Drill < class.ViaDrill) {
			violationCount++
			low = appendLimited(low, string(via.UUID))
			if strings.TrimSpace(via.NetName) != "" {
				nets[via.NetName] = struct{}{}
			}
		}
	}
	if len(low) == 0 {
		return []Check{{
			ID:           CheckNetClassViaDimensions,
			Category:     CategoryNetClass,
			Status:       StatusPass,
			Message:      "routed via dimensions meet the Default net-class via rules",
			Measurements: []Measurement{{Name: "default_via_diameter", Value: iuToMM(class.ViaDiameter), Unit: "mm"}, {Name: "default_via_drill", Value: iuToMM(class.ViaDrill), Unit: "mm"}},
			Source:       SourceParser,
		}}
	}
	status := StatusBlocked
	severity := reports.SeverityError
	suggestion := "increase via diameter/drill or assign a verified smaller net class before fabrication export"
	if !blocking {
		status = StatusWarning
		severity = reports.SeverityWarning
		suggestion = "verify net-specific via dimensions with KiCad DRC until net-class assignment parsing is modeled"
	}
	return []Check{{
		ID:         CheckNetClassViaDimensions,
		Category:   CategoryNetClass,
		Status:     status,
		Severity:   severity,
		Message:    "one or more vias are smaller than the Default net-class via dimensions",
		Suggestion: suggestion,
		IssuePath:  "physical.net_class.effective_rules",
		Objects:    low,
		Nets:       sortedMapKeys(nets),
		Measurements: []Measurement{
			{Name: "default_via_diameter", Value: iuToMM(class.ViaDiameter), Unit: "mm"},
			{Name: "default_via_drill", Value: iuToMM(class.ViaDrill), Unit: "mm"},
			{Name: "violation_count", Value: float64(violationCount), Unit: "count"},
		},
		Source: SourceParser,
	}}
}

func evaluateEdgeCuts(board *pcbfiles.PCBFile) edgeCutResult {
	bounds := edgeBounds(board)
	status := StatusPass
	message := "PCB has a closed Edge.Cuts outline"
	suggestion := ""
	issuePath := ""
	code := reports.Code("")
	switch {
	case !bounds.Valid:
		status = StatusBlocked
		message = "PCB has no Edge.Cuts outline"
		suggestion = "add a closed board outline on Edge.Cuts before fabrication export"
		issuePath = "physical.edge_cuts.outline"
		code = reports.CodeMissingBoardOutline
	case !edgeOutlineClosed(board):
		status = StatusWarning
		message = "PCB Edge.Cuts outline could not be proven closed by the internal checker"
		suggestion = "use KiCad DRC or simplify the outline so closure can be verified"
		issuePath = "physical.edge_cuts.outline"
	}
	check := Check{
		ID:         CheckEdgeCutsOutline,
		Category:   CategoryEdgeCuts,
		Status:     status,
		Message:    message,
		Suggestion: suggestion,
		IssueCode:  code,
		IssuePath:  issuePath,
		Source:     SourceParser,
	}
	if bounds.Valid {
		check.Measurements = []Measurement{
			{Name: "min_x", Value: iuToMM(bounds.MinX), Unit: "mm"},
			{Name: "min_y", Value: iuToMM(bounds.MinY), Unit: "mm"},
			{Name: "max_x", Value: iuToMM(bounds.MaxX), Unit: "mm"},
			{Name: "max_y", Value: iuToMM(bounds.MaxY), Unit: "mm"},
		}
	}
	return edgeCutResult{Checks: []Check{check}, Bounds: bounds}
}

func evaluateEdgePlating(features edgePlatingFeatureSet, opts Options) []Check {
	if len(features.Objects) == 0 {
		return []Check{
			{ID: CheckEdgePlatingCastellation, Category: CategoryEdgePlating, Status: StatusSkipped, Message: "no likely castellated or edge-plated features were detected", Source: SourceHeuristic},
			{ID: CheckEdgePlatingProfile, Category: CategoryEdgePlating, Status: StatusSkipped, Message: "edge-plating profile support not required because no edge-plated features were detected", Source: SourceProfile},
			{ID: CheckEdgePlatingContact, Category: CategoryEdgePlating, Status: StatusSkipped, Message: "edge-contact evidence not required because no edge-plated features were detected", Source: SourceHeuristic},
		}
	}
	status := StatusWarning
	severity := reports.SeverityWarning
	message := "likely castellated or edge-plated features require manufacturer profile confirmation"
	suggestion := "confirm that the selected manufacturer profile allows castellations or edge plating before fabrication export"
	switch opts.EdgePlatingPolicy {
	case PolicyAllow:
		status = StatusPass
		severity = ""
		message = "likely castellated or edge-plated features are allowed by the active profile"
		suggestion = ""
	case PolicyBlock:
		status = StatusBlocked
		severity = reports.SeverityError
		message = "likely castellated or edge-plated features are not allowed by the active profile"
		suggestion = "remove edge-plated features or select a profile that explicitly allows castellations"
	}
	measurements := []Measurement{{Name: "feature_count", Value: float64(len(features.Objects)), Unit: "count"}}
	if features.MinEdgeDistanceMM != math.MaxFloat64 {
		measurements = append(measurements, Measurement{Name: "minimum_edge_distance", Value: features.MinEdgeDistanceMM, Unit: "mm"})
	}
	checks := []Check{
		{
			ID:           CheckEdgePlatingCastellation,
			Category:     CategoryEdgePlating,
			Status:       StatusWarning,
			Message:      "likely castellated or edge-plated features were detected",
			Suggestion:   "review edge-plating requirements and fabrication notes before release",
			IssuePath:    "physical.edge_plating.castellation_detected",
			References:   sortedMapKeys(features.Refs),
			Objects:      features.Objects,
			Measurements: measurements,
			Source:       SourceHeuristic,
		},
		{
			ID:           CheckEdgePlatingProfile,
			Category:     CategoryEdgePlating,
			Status:       status,
			Severity:     severity,
			Message:      message,
			Suggestion:   suggestion,
			IssuePath:    "physical.edge_plating.profile_support",
			References:   sortedMapKeys(features.Refs),
			Objects:      features.Objects,
			Measurements: measurements,
			Source:       SourceProfile,
		},
		{
			ID:           CheckEdgePlatingContact,
			Category:     CategoryEdgePlating,
			Status:       StatusWarning,
			Message:      "edge contact was inferred from conservative board-bound proximity",
			Suggestion:   "use KiCad/manufacturer DFM evidence for exact plated-edge geometry",
			IssuePath:    "physical.edge_plating.edge_contact",
			References:   sortedMapKeys(features.Refs),
			Objects:      features.Objects,
			Measurements: measurements,
			Source:       SourceHeuristic,
		},
	}
	if opts.EdgePlatingPolicy == PolicyAllow {
		checks[0].Status = StatusPass
		checks[2].Status = StatusPass
		checks[2].Message = "edge contact evidence is accepted by the active profile"
		checks[2].Suggestion = ""
	}
	return checks
}

func evaluateFabricationMetadata(project *projectfiles.ProjectFile, features edgePlatingFeatureSet, opts Options) []Check {
	metadata := projectFabricationMetadata(project)
	boardFinish := strings.TrimSpace(firstMetadataValue(metadata, "board_finish", "finish", "surface_finish"))
	panelization := strings.TrimSpace(firstMetadataValue(metadata, "panelization", "panel"))
	notes := strings.TrimSpace(firstMetadataValue(metadata, "fabrication_notes", "fab_notes", "fabrication_note", "notes"))
	checks := []Check{
		fabricationMetadataCheck(
			CheckFabMetadataBoardFinish,
			"physical.fabrication_metadata.board_finish",
			"board finish",
			boardFinish,
			opts.RequireBoardFinish,
			opts.Strict,
		),
	}
	panelRequired := opts.PanelizationPolicy == PolicyWarn || opts.PanelizationPolicy == PolicyBlock
	checks = append(checks, fabricationMetadataCheck(
		CheckFabMetadataPanelization,
		"physical.fabrication_metadata.panelization",
		"panelization",
		panelization,
		panelRequired,
		opts.PanelizationPolicy == PolicyBlock,
	))
	notesRequired := opts.RequireFabricationNotes || len(features.Objects) > 0 || opts.ImpedancePolicy == PolicyBlock
	noteCheck := fabricationMetadataCheck(
		CheckFabMetadataNotes,
		"physical.fabrication_metadata.fabrication_notes",
		"fabrication notes",
		notes,
		notesRequired,
		opts.Strict || opts.RequireFabricationNotes && opts.EdgePlatingPolicy == PolicyBlock,
	)
	if len(features.Objects) > 0 {
		noteCheck.Objects = features.Objects
		noteCheck.References = sortedMapKeys(features.Refs)
		noteCheck.Measurements = append(noteCheck.Measurements, Measurement{Name: "edge_plating_feature_count", Value: float64(len(features.Objects)), Unit: "count"})
	}
	checks = append(checks, noteCheck)
	return checks
}

func projectFabricationMetadata(project *projectfiles.ProjectFile) map[string]string {
	metadata := map[string]string{}
	if project == nil {
		return metadata
	}
	for key, value := range project.TextVariables {
		metadata[normalizeMetadataKey(key)] = strings.TrimSpace(value)
	}
	return metadata
}

func normalizeMetadataKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	return metadataKeyReplacer.Replace(key)
}

func firstMetadataValue(metadata map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(metadata[normalizeMetadataKey(key)]); value != "" {
			return value
		}
	}
	return ""
}

func fabricationMetadataCheck(id, path, label, value string, required, blockMissing bool) Check {
	if strings.TrimSpace(value) != "" {
		return Check{
			ID:        id,
			Category:  CategoryFabMetadata,
			Status:    StatusPass,
			Message:   label + " metadata is present",
			IssuePath: path,
			Evidence:  []Evidence{{Kind: "project_text_variable", Note: value}},
			Source:    SourceParser,
			Measurements: []Measurement{
				{Name: "metadata_present", Value: 1, Unit: "bool"},
			},
		}
	}
	if !required {
		return Check{ID: id, Category: CategoryFabMetadata, Status: StatusSkipped, Message: label + " metadata is not required by the active profile", IssuePath: path, Source: SourceProfile}
	}
	status := StatusWarning
	severity := reports.SeverityWarning
	if blockMissing {
		status = StatusBlocked
		severity = reports.SeverityError
	}
	return Check{
		ID:         id,
		Category:   CategoryFabMetadata,
		Status:     status,
		Severity:   severity,
		Message:    label + " metadata is required but missing",
		Suggestion: "add KiCadAI-managed project metadata or project text variables before claiming fabrication readiness",
		IssuePath:  path,
		Source:     SourceProfile,
		Measurements: []Measurement{
			{Name: "metadata_present", Value: 0, Unit: "bool"},
		},
	}
}

type edgePlatingFeatureSet struct {
	Objects           []string
	Refs              map[string]struct{}
	MinEdgeDistanceMM float64
}

func edgePlatingFeatures(board *pcbfiles.PCBFile, bounds boardBounds) edgePlatingFeatureSet {
	features := edgePlatingFeatureSet{Refs: map[string]struct{}{}, MinEdgeDistanceMM: math.MaxFloat64}
	if !bounds.Valid {
		return features
	}
	const edgeContactToleranceMM = 0.25
	for footprintIndex := range board.Footprints {
		footprint := &board.Footprints[footprintIndex]
		nameSuggestsEdgePlating := footprintSuggestsEdgePlating(footprint)
		transform := footprintTransform(footprint)
		for padIndex := range footprint.Pads {
			pad := &footprint.Pads[padIndex]
			if !isPotentialEdgePlatedPad(pad, nameSuggestsEdgePlating) {
				continue
			}
			padBounds := transformedPadBounds(footprint, transform, pad)
			if !padBounds.Valid {
				continue
			}
			distanceMM := rectDistanceToBoardEdgeMM(bounds, padBounds)
			if nameSuggestsEdgePlating || distanceMM <= edgeContactToleranceMM {
				features.Objects = appendLimited(features.Objects, string(pad.UUID))
				addRef(features.Refs, footprint.Reference)
				if distanceMM < features.MinEdgeDistanceMM {
					features.MinEdgeDistanceMM = distanceMM
				}
			}
		}
	}
	return features
}

func evaluateBoardContainment(board *pcbfiles.PCBFile, bounds boardBounds) []Check {
	if !bounds.Valid {
		return []Check{{
			ID:         CheckEdgeCutsContainment,
			Category:   CategoryEdgeCuts,
			Status:     StatusSkipped,
			Message:    "board containment was skipped because no usable Edge.Cuts bounds were found",
			IssuePath:  "physical.edge_cuts.containment",
			Suggestion: "add a closed board outline before checking object containment",
			Source:     SourceParser,
		}}
	}
	refs := map[string]struct{}{}
	var objects []string
	violationCount := 0
	for footprintIndex := range board.Footprints {
		footprint := &board.Footprints[footprintIndex]
		if !pointInsideBoard(bounds, footprint.Position) {
			addRef(refs, footprint.Reference)
			violationCount++
			objects = appendLimited(objects, string(footprint.UUID))
		}
		transform := footprintTransform(footprint)
		for padIndex := range footprint.Pads {
			pad := &footprint.Pads[padIndex]
			if !padInside(bounds, transform, footprint, pad) {
				addRef(refs, footprint.Reference)
				violationCount++
				objects = appendLimited(objects, string(pad.UUID))
			}
		}
	}
	for _, track := range board.Tracks {
		if !trackInside(bounds, track) {
			violationCount++
			objects = appendLimited(objects, string(track.UUID))
		}
	}
	for _, via := range board.Vias {
		if !objectInside(bounds, via.Position, via.Size/2) {
			violationCount++
			objects = appendLimited(objects, string(via.UUID))
		}
	}
	for _, zone := range board.Zones {
		for _, polygon := range zone.Polygons {
			if !polygonInsideBoard(bounds, polygon) {
				violationCount++
				objects = appendLimited(objects, string(zone.UUID))
				break
			}
		}
		for _, filled := range zone.FilledPolygons {
			if !polygonInsideBoard(bounds, filled.Points) {
				violationCount++
				objects = appendLimited(objects, string(zone.UUID))
				break
			}
		}
	}
	for _, drawing := range board.Drawings {
		if !isCopperLayer(drawing.Layer) {
			continue
		}
		if !drawingInsideBoard(bounds, drawing) {
			violationCount++
			objects = appendLimited(objects, string(drawing.UUID))
		}
	}
	if len(objects) == 0 {
		if !bounds.Rectangular && len(bounds.Polygons) == 0 {
			return []Check{{
				ID:         CheckEdgeCutsContainment,
				Category:   CategoryEdgeCuts,
				Status:     StatusWarning,
				Message:    "PCB generated objects are inside Edge.Cuts bounds, but non-rectangular outline containment is conservatively approximated",
				Suggestion: "use KiCad DRC evidence for non-rectangular outlines until polygon containment is implemented",
				IssuePath:  "physical.edge_cuts.containment",
				Source:     SourceHeuristic,
			}}
		}
		return []Check{{
			ID:       CheckEdgeCutsContainment,
			Category: CategoryEdgeCuts,
			Status:   StatusPass,
			Message:  "PCB generated objects are inside Edge.Cuts bounds",
			Source:   SourceParser,
		}}
	}
	return []Check{{
		ID:         CheckEdgeCutsContainment,
		Category:   CategoryEdgeCuts,
		Status:     StatusBlocked,
		Message:    "one or more PCB objects are outside Edge.Cuts bounds",
		Suggestion: "move the objects inside the board outline or enlarge the outline before fabrication export",
		IssuePath:  "physical.edge_cuts.containment",
		References: sortedMapKeys(refs),
		Objects:    objects,
		Measurements: []Measurement{
			{Name: "violation_count", Value: float64(violationCount), Unit: "count"},
		},
		Source: SourceParser,
	}}
}

func copperLayers(board *pcbfiles.PCBFile) []kicadfiles.BoardLayer {
	var layers []kicadfiles.BoardLayer
	for _, layer := range board.Layers {
		if strings.HasSuffix(string(layer.Name), ".Cu") {
			layers = append(layers, layer.Name)
		}
	}
	return layers
}

func copperLayerCount(board *pcbfiles.PCBFile) int {
	return len(copperLayers(board))
}

func boardLayerNames(layers []kicadfiles.BoardLayer) []string {
	names := make([]string, 0, len(layers))
	for _, layer := range layers {
		names = append(names, string(layer))
	}
	slices.Sort(names)
	return names
}

func hasLayer(layers []kicadfiles.BoardLayer, want kicadfiles.BoardLayer) bool {
	for _, layer := range layers {
		if layer == want {
			return true
		}
	}
	return false
}

func isCopperLayer(layer kicadfiles.BoardLayer) bool {
	return strings.HasSuffix(string(layer), ".Cu") || layer == kicadfiles.LayerAllCu
}

func isSilkscreenLayer(layer kicadfiles.BoardLayer) bool {
	return layer == kicadfiles.LayerFSilkS || layer == kicadfiles.LayerBSilkS
}

func defaultNetClass(classes []projectfiles.NetClass) (projectfiles.NetClass, bool) {
	for _, class := range classes {
		if strings.TrimSpace(class.Name) == "Default" {
			return class, true
		}
	}
	return projectfiles.NetClass{}, false
}

func invalidNetClass(class projectfiles.NetClass) bool {
	return class.Clearance < 0 || class.TrackWidth <= 0 || class.ViaDiameter <= 0 || class.ViaDrill <= 0 || class.ViaDrill >= class.ViaDiameter
}

func netClassMeasurements(class projectfiles.NetClass) []Measurement {
	return []Measurement{
		{Name: "clearance", Value: iuToMM(class.Clearance), Unit: "mm"},
		{Name: "track_width", Value: iuToMM(class.TrackWidth), Unit: "mm"},
		{Name: "via_diameter", Value: iuToMM(class.ViaDiameter), Unit: "mm"},
		{Name: "via_drill", Value: iuToMM(class.ViaDrill), Unit: "mm"},
	}
}

func routedNetNames(board *pcbfiles.PCBFile) []string {
	seen := map[string]struct{}{}
	for _, track := range board.Tracks {
		if name := strings.TrimSpace(track.NetName); name != "" {
			seen[name] = struct{}{}
		}
	}
	for _, via := range board.Vias {
		if name := strings.TrimSpace(via.NetName); name != "" {
			seen[name] = struct{}{}
		}
	}
	return sortedMapKeys(seen)
}

func edgeBounds(board *pcbfiles.PCBFile) boardBounds {
	var bounds boardBounds
	rectCount := 0
	edgeDrawingCount := 0
	for _, drawing := range board.Drawings {
		if drawing.Layer != kicadfiles.LayerEdge {
			continue
		}
		edgeDrawingCount++
		if drawing.Rect != nil {
			rectCount++
		}
		for _, point := range drawingPoints(drawing) {
			bounds = includePoint(bounds, point)
		}
	}
	if bounds.Valid {
		bounds.Rectangular = (edgeDrawingCount == 1 && rectCount == 1) || lineOutlineIsAxisAlignedRectangle(board)
		bounds.Polygons = outlinePolygons(board, bounds)
	}
	return bounds
}

func drawingPoints(drawing pcbfiles.Drawing) []kicadfiles.Point {
	switch {
	case drawing.Line != nil:
		return []kicadfiles.Point{drawing.Line.Start, drawing.Line.End}
	case drawing.Rect != nil:
		return []kicadfiles.Point{drawing.Rect.Start, drawing.Rect.End}
	case drawing.Circle != nil:
		radius := distanceIU(drawing.Circle.Center, drawing.Circle.End)
		return []kicadfiles.Point{
			{X: drawing.Circle.Center.X - radius, Y: drawing.Circle.Center.Y - radius},
			{X: drawing.Circle.Center.X + radius, Y: drawing.Circle.Center.Y + radius},
		}
	case drawing.Arc != nil:
		return arcBoundsPoints(*drawing.Arc)
	case drawing.Poly != nil:
		return slices.Clone(drawing.Poly.Points)
	default:
		return nil
	}
}

func drawingStrokeWidth(drawing pcbfiles.Drawing) (kicadfiles.IU, bool) {
	switch {
	case drawing.Line != nil:
		return drawing.Line.Width, true
	case drawing.Rect != nil:
		return drawing.Rect.Width, true
	case drawing.Circle != nil:
		return drawing.Circle.Width, true
	case drawing.Arc != nil:
		return drawing.Arc.Width, true
	case drawing.Poly != nil:
		return drawing.Poly.Width, true
	case drawing.Curve != nil:
		return drawing.Curve.Width, true
	default:
		return 0, false
	}
}

func includePoint(bounds boardBounds, point kicadfiles.Point) boardBounds {
	if !bounds.Valid {
		return boardBounds{Valid: true, MinX: point.X, MinY: point.Y, MaxX: point.X, MaxY: point.Y}
	}
	if point.X < bounds.MinX {
		bounds.MinX = point.X
	}
	if point.Y < bounds.MinY {
		bounds.MinY = point.Y
	}
	if point.X > bounds.MaxX {
		bounds.MaxX = point.X
	}
	if point.Y > bounds.MaxY {
		bounds.MaxY = point.Y
	}
	return bounds
}

func edgeOutlineClosed(board *pcbfiles.PCBFile) bool {
	edgeDrawingCount := 0
	closedPrimitiveCount := 0
	for _, drawing := range board.Drawings {
		if drawing.Layer != kicadfiles.LayerEdge {
			continue
		}
		edgeDrawingCount++
		if drawing.Circle != nil || drawing.Rect != nil || (drawing.Poly != nil && len(drawing.Poly.Points) >= 4 && closePoint(drawing.Poly.Points[0], drawing.Poly.Points[len(drawing.Poly.Points)-1])) {
			closedPrimitiveCount++
		}
	}
	if edgeDrawingCount > 0 && edgeDrawingCount == closedPrimitiveCount {
		return true
	}
	for _, drawing := range board.Drawings {
		if drawing.Layer == kicadfiles.LayerEdge && drawing.Circle != nil {
			// Circles mixed with other Edge.Cuts objects require KiCad DRC or
			// future curve-aware loop classification.
			return false
		}
	}
	lines := edgeLines(board)
	if len(lines) == 0 {
		return false
	}
	degree := map[pointKey]int{}
	seen := map[quantizedPointKey]pointKey{}
	for _, line := range lines {
		start := canonicalPointKey(seen, line.Start)
		end := canonicalPointKey(seen, line.End)
		degree[start]++
		degree[end]++
	}
	for _, count := range degree {
		if count != 2 {
			return false
		}
	}
	return len(degree) >= 3
}

type edgeLine struct {
	Start kicadfiles.Point
	End   kicadfiles.Point
}

func edgeLines(board *pcbfiles.PCBFile) []edgeLine {
	var lines []edgeLine
	for _, drawing := range board.Drawings {
		if drawing.Layer != kicadfiles.LayerEdge {
			continue
		}
		if drawing.Line != nil {
			lines = append(lines, edgeLine{Start: drawing.Line.Start, End: drawing.Line.End})
		}
		if drawing.Rect != nil {
			start := drawing.Rect.Start
			end := drawing.Rect.End
			p1 := kicadfiles.Point{X: start.X, Y: start.Y}
			p2 := kicadfiles.Point{X: end.X, Y: start.Y}
			p3 := kicadfiles.Point{X: end.X, Y: end.Y}
			p4 := kicadfiles.Point{X: start.X, Y: end.Y}
			lines = append(lines,
				edgeLine{Start: p1, End: p2},
				edgeLine{Start: p2, End: p3},
				edgeLine{Start: p3, End: p4},
				edgeLine{Start: p4, End: p1},
			)
		}
		if drawing.Poly != nil {
			for i := 0; i+1 < len(drawing.Poly.Points); i++ {
				lines = append(lines, edgeLine{Start: drawing.Poly.Points[i], End: drawing.Poly.Points[i+1]})
			}
		}
		if drawing.Arc != nil {
			lines = append(lines, edgeLine{Start: drawing.Arc.Start, End: drawing.Arc.End})
		}
	}
	return lines
}

func closePoint(a, b kicadfiles.Point) bool {
	tolerance := kicadfiles.MM(outlineToleranceMM)
	return absIU(a.X-b.X) <= tolerance && absIU(a.Y-b.Y) <= tolerance
}

type pointKey struct {
	X kicadfiles.IU
	Y kicadfiles.IU
}

type quantizedPointKey struct {
	X int64
	Y int64
}

func pointKeyFor(point kicadfiles.Point) pointKey {
	return pointKey{X: point.X, Y: point.Y}
}

func canonicalPointKey(seen map[quantizedPointKey]pointKey, point kicadfiles.Point) pointKey {
	candidate := pointKeyFor(point)
	tolerance := kicadfiles.MM(outlineToleranceMM)
	quantized := quantizePoint(point, tolerance)
	for dx := int64(-1); dx <= 1; dx++ {
		for dy := int64(-1); dy <= 1; dy++ {
			key := quantizedPointKey{X: quantized.X + dx, Y: quantized.Y + dy}
			if existing, ok := seen[key]; ok && absIU(existing.X-candidate.X) <= tolerance && absIU(existing.Y-candidate.Y) <= tolerance {
				return existing
			}
		}
	}
	seen[quantized] = candidate
	return candidate
}

func quantizePoint(point kicadfiles.Point, tolerance kicadfiles.IU) quantizedPointKey {
	if tolerance <= 0 {
		tolerance = 1
	}
	return quantizedPointKey{X: floorDiv(int64(point.X), int64(tolerance)), Y: floorDiv(int64(point.Y), int64(tolerance))}
}

func floorDiv(value, divisor int64) int64 {
	quotient := value / divisor
	remainder := value % divisor
	if remainder != 0 && ((remainder < 0) != (divisor < 0)) {
		quotient--
	}
	return quotient
}

func pointInside(bounds boardBounds, point kicadfiles.Point) bool {
	return point.X >= bounds.MinX && point.X <= bounds.MaxX && point.Y >= bounds.MinY && point.Y <= bounds.MaxY
}

func pointInsideBoard(bounds boardBounds, point kicadfiles.Point) bool {
	if !pointInside(bounds, point) {
		return false
	}
	if len(bounds.Polygons) > 0 {
		inside := false
		for _, polygon := range bounds.Polygons {
			if pointInPolygon(polygon, point) {
				inside = !inside
			}
		}
		return inside
	}
	return true
}

func allPointsInsideBoard(bounds boardBounds, points []kicadfiles.Point) bool {
	for _, point := range points {
		if !pointInsideBoard(bounds, point) {
			return false
		}
	}
	return true
}

func polygonInsideBoard(bounds boardBounds, polygon []kicadfiles.Point) bool {
	if !allPointsInsideBoard(bounds, polygon) {
		return false
	}
	if len(bounds.Polygons) == 0 || len(polygon) < 2 {
		return true
	}
	for i := range polygon {
		next := (i + 1) % len(polygon)
		if segmentIntersectsPolygonBoundary(polygon[i], polygon[next], bounds.Polygons) {
			return false
		}
	}
	return true
}

func drawingInsideBoard(bounds boardBounds, drawing pcbfiles.Drawing) bool {
	points := drawingPoints(drawing)
	if !allPointsInsideBoard(bounds, points) {
		return false
	}
	if len(bounds.Polygons) == 0 {
		return true
	}
	switch {
	case drawing.Line != nil:
		return !segmentIntersectsPolygonBoundary(drawing.Line.Start, drawing.Line.End, bounds.Polygons)
	case drawing.Rect != nil:
		return polygonInsideBoard(bounds, []kicadfiles.Point{
			drawing.Rect.Start,
			{X: drawing.Rect.End.X, Y: drawing.Rect.Start.Y},
			drawing.Rect.End,
			{X: drawing.Rect.Start.X, Y: drawing.Rect.End.Y},
		})
	case drawing.Poly != nil:
		return polygonInsideBoard(bounds, drawing.Poly.Points)
	default:
		return true
	}
}

func trackInside(bounds boardBounds, track pcbfiles.Track) bool {
	radius := track.Width / 2
	if !objectInside(bounds, track.Start, radius) || !objectInside(bounds, track.End, radius) {
		return false
	}
	if len(bounds.Polygons) > 0 {
		if segmentIntersectsPolygonBoundary(track.Start, track.End, bounds.Polygons) {
			return false
		}
		if track.Width > 0 && segmentPolygonDistance(track.Start, track.End, bounds.Polygons) < iuToMM(track.Width)/2 {
			return false
		}
	}
	return true
}

func objectInside(bounds boardBounds, point kicadfiles.Point, radius kicadfiles.IU) bool {
	if radius < 0 {
		radius = 0
	}
	if len(bounds.Polygons) > 0 {
		return pointInsideBoard(bounds, point) && pointPolygonDistance(point, bounds.Polygons) >= iuToMM(radius)
	}
	return pointInsideBoard(bounds, point) &&
		pointInsideBoard(bounds, kicadfiles.Point{X: point.X - radius, Y: point.Y - radius}) &&
		pointInsideBoard(bounds, kicadfiles.Point{X: point.X - radius, Y: point.Y + radius}) &&
		pointInsideBoard(bounds, kicadfiles.Point{X: point.X + radius, Y: point.Y - radius}) &&
		pointInsideBoard(bounds, kicadfiles.Point{X: point.X + radius, Y: point.Y + radius})
}

func distanceIU(a, b kicadfiles.Point) kicadfiles.IU {
	dx := float64(a.X - b.X)
	dy := float64(a.Y - b.Y)
	return kicadfiles.IU(math.Round(math.Sqrt(dx*dx + dy*dy)))
}

func iuToMM(value kicadfiles.IU) float64 {
	return float64(value) / 1_000_000
}

type transform2D struct {
	Cosine  float64
	Sine    float64
	MirrorX bool
}

func footprintTransform(footprint *pcbfiles.Footprint) transform2D {
	radians := float64(footprint.Rotation) * math.Pi / 180
	return transform2D{
		Cosine:  math.Cos(radians),
		Sine:    math.Sin(radians),
		MirrorX: footprint.Layer == kicadfiles.LayerBCu,
	}
}

func transformFootprintPoint(footprint *pcbfiles.Footprint, point kicadfiles.Point) kicadfiles.Point {
	return transformFootprintPointWith(footprint, footprintTransform(footprint), point)
}

func transformFootprintPointWith(footprint *pcbfiles.Footprint, transform transform2D, point kicadfiles.Point) kicadfiles.Point {
	offset := transformedOffset(transform, point)
	return kicadfiles.Point{X: footprint.Position.X + offset.X, Y: footprint.Position.Y + offset.Y}
}

func transformFootprintPoints(footprint *pcbfiles.Footprint, points []kicadfiles.Point) []kicadfiles.Point {
	transform := footprintTransform(footprint)
	return transformFootprintPointsWith(footprint, transform, points)
}

func transformFootprintPointsWith(footprint *pcbfiles.Footprint, transform transform2D, points []kicadfiles.Point) []kicadfiles.Point {
	out := make([]kicadfiles.Point, 0, len(points))
	for _, point := range points {
		out = append(out, transformFootprintPointWith(footprint, transform, point))
	}
	return out
}

func padInside(bounds boardBounds, transform transform2D, footprint *pcbfiles.Footprint, pad *pcbfiles.Pad) bool {
	center := transformedOffset(transform, pad.Position)
	centerPoint := kicadfiles.Point{X: footprint.Position.X + center.X, Y: footprint.Position.Y + center.Y}
	if !pointInsideBoard(bounds, centerPoint) {
		return false
	}
	if strings.EqualFold(pad.Shape, "circle") {
		return objectInside(bounds, centerPoint, maxIU(pad.Size.X, pad.Size.Y)/2)
	}
	if strings.EqualFold(pad.Shape, "oval") || strings.EqualFold(pad.Shape, "oblong") {
		return ovalPadInside(bounds, transform, footprint, pad)
	}
	halfX := float64(pad.Size.X) / 2
	halfY := float64(pad.Size.Y) / 2
	padRadians := float64(pad.Rotation) * math.Pi / 180
	padCosine := math.Cos(padRadians)
	padSine := math.Sin(padRadians)
	for _, corner := range []struct{ x, y float64 }{
		{-halfX, -halfY},
		{-halfX, halfY},
		{halfX, -halfY},
		{halfX, halfY},
	} {
		rotatedX := corner.x*padCosine - corner.y*padSine
		rotatedY := corner.x*padSine + corner.y*padCosine
		local := kicadfiles.Point{
			X: pad.Position.X + kicadfiles.IU(math.Round(rotatedX)),
			Y: pad.Position.Y + kicadfiles.IU(math.Round(rotatedY)),
		}
		offset := transformedOffset(transform, local)
		if !pointInsideBoard(bounds, kicadfiles.Point{X: footprint.Position.X + offset.X, Y: footprint.Position.Y + offset.Y}) {
			return false
		}
	}
	return true
}

func ovalPadInside(bounds boardBounds, transform transform2D, footprint *pcbfiles.Footprint, pad *pcbfiles.Pad) bool {
	halfX := float64(pad.Size.X) / 2
	halfY := float64(pad.Size.Y) / 2
	padRadians := float64(pad.Rotation) * math.Pi / 180
	padCosine := math.Cos(padRadians)
	padSine := math.Sin(padRadians)
	const samples = 16
	for index := 0; index < samples; index++ {
		angle := 2 * math.Pi * float64(index) / samples
		x := halfX * math.Cos(angle)
		y := halfY * math.Sin(angle)
		rotatedX := x*padCosine - y*padSine
		rotatedY := x*padSine + y*padCosine
		local := kicadfiles.Point{
			X: pad.Position.X + kicadfiles.IU(math.Round(rotatedX)),
			Y: pad.Position.Y + kicadfiles.IU(math.Round(rotatedY)),
		}
		offset := transformedOffset(transform, local)
		if !pointInsideBoard(bounds, kicadfiles.Point{X: footprint.Position.X + offset.X, Y: footprint.Position.Y + offset.Y}) {
			return false
		}
	}
	return true
}

func transformedOffset(transform transform2D, point kicadfiles.Point) kicadfiles.Point {
	x := float64(point.X)
	y := float64(point.Y)
	if transform.MirrorX {
		x = -x
	}
	return kicadfiles.Point{
		X: kicadfiles.IU(math.Round(x*transform.Cosine - y*transform.Sine)),
		Y: kicadfiles.IU(math.Round(x*transform.Sine + y*transform.Cosine)),
	}
}

func arcBoundsPoints(arc pcbfiles.ArcDrawing) []kicadfiles.Point {
	center, radius, ok := circleFromThreePoints(arc.Start, arc.Mid, arc.End)
	if !ok {
		return []kicadfiles.Point{arc.Start, arc.Mid, arc.End}
	}
	points := []kicadfiles.Point{
		arc.Start,
		arc.Mid,
		arc.End,
	}
	start := angleFor(center, arc.Start)
	mid := angleFor(center, arc.Mid)
	end := angleFor(center, arc.End)
	for _, candidate := range []float64{0, math.Pi / 2, math.Pi, 3 * math.Pi / 2} {
		if angleOnArc(start, mid, end, candidate) {
			points = append(points, kicadfiles.Point{
				X: center.X + kicadfiles.IU(math.Round(float64(radius)*math.Cos(candidate))),
				Y: center.Y + kicadfiles.IU(math.Round(float64(radius)*math.Sin(candidate))),
			})
		}
	}
	return points
}

func circleFromThreePoints(a, b, c kicadfiles.Point) (kicadfiles.Point, kicadfiles.IU, bool) {
	bx, by := iuToMM(b.X-a.X), iuToMM(b.Y-a.Y)
	cx, cy := iuToMM(c.X-a.X), iuToMM(c.Y-a.Y)
	d := 2 * (bx*cy - by*cx)
	scale := math.Max(math.Max(math.Abs(bx), math.Abs(by)), math.Max(math.Abs(cx), math.Abs(cy)))
	epsilon := math.Max(1e-9, scale*scale*1e-12)
	if math.Abs(d) < epsilon {
		return kicadfiles.Point{}, 0, false
	}
	bLen := bx*bx + by*by
	cLen := cx*cx + cy*cy
	ux := (bLen*cy - cLen*by) / d
	uy := (bx*cLen - cx*bLen) / d
	center := kicadfiles.Point{X: a.X + kicadfiles.MM(ux), Y: a.Y + kicadfiles.MM(uy)}
	return center, distanceIU(center, a), true
}

func angleFor(center, point kicadfiles.Point) float64 {
	angle := math.Atan2(float64(point.Y-center.Y), float64(point.X-center.X))
	if angle < 0 {
		angle += 2 * math.Pi
	}
	return angle
}

func angleOnArc(start, mid, end, candidate float64) bool {
	if angleBetweenCCW(start, end, mid) {
		return angleBetweenCCW(start, end, candidate)
	}
	return angleBetweenCCW(end, start, candidate)
}

func angleBetweenCCW(start, end, candidate float64) bool {
	if end < start {
		end += 2 * math.Pi
	}
	if candidate < start {
		candidate += 2 * math.Pi
	}
	return candidate >= start && candidate <= end
}

func lineOutlineIsAxisAlignedRectangle(board *pcbfiles.PCBFile) bool {
	lines := edgeLines(board)
	if len(lines) != 4 || !edgeOutlineClosed(board) {
		return false
	}
	for _, line := range lines {
		if line.Start.X != line.End.X && line.Start.Y != line.End.Y {
			return false
		}
	}
	return true
}

func maxIU(a, b kicadfiles.IU) kicadfiles.IU {
	if a > b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func absIU(value kicadfiles.IU) kicadfiles.IU {
	if value < 0 {
		return -value
	}
	return value
}

func sortedMapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		if strings.TrimSpace(value) != "" {
			keys = append(keys, value)
		}
	}
	slices.Sort(keys)
	return keys
}

func addRef(refs map[string]struct{}, ref string) {
	ref = strings.TrimSpace(ref)
	if ref != "" {
		refs[ref] = struct{}{}
	}
}

func footprintNeedsCourtyard(footprint *pcbfiles.Footprint) bool {
	if len(footprint.Pads) == 0 {
		return false
	}
	for _, attribute := range footprint.Attributes {
		if strings.EqualFold(attribute, "board_only") || strings.EqualFold(attribute, "exclude_from_pos_files") {
			return false
		}
	}
	return true
}

func mountingHoleFootprintCandidate(footprint *pcbfiles.Footprint) bool {
	ref := strings.ToUpper(strings.TrimSpace(footprint.Reference))
	if strings.HasPrefix(ref, "MH") {
		return true
	}
	libraryID := strings.ToLower(strings.TrimSpace(footprint.LibraryID))
	value := strings.ToLower(strings.TrimSpace(footprint.Value))
	return strings.Contains(libraryID, "mountinghole") ||
		strings.Contains(libraryID, "mounting_hole") ||
		strings.Contains(value, "mountinghole") ||
		strings.Contains(value, "mounting hole")
}

func isMountingHole(pad *pcbfiles.Pad, footprintSuggestsHole bool) bool {
	if strings.EqualFold(pad.Type, "np_thru_hole") {
		return true
	}
	if !strings.EqualFold(pad.Type, "thru_hole") {
		return false
	}
	return footprintSuggestsHole
}

func footprintCourtyardBounds(footprint *pcbfiles.Footprint) (rectBounds, bool) {
	var bounds rectBounds
	for _, graphic := range footprint.Graphics {
		drawing := pcbfiles.Drawing(graphic)
		if drawing.Layer != kicadfiles.LayerFCrtYd && drawing.Layer != kicadfiles.LayerBCrtYd {
			continue
		}
		for _, point := range transformFootprintPoints(footprint, drawingPoints(drawing)) {
			bounds = includeRectPoint(bounds, point)
		}
	}
	return bounds, bounds.Valid
}

func includeRectPoint(bounds rectBounds, point kicadfiles.Point) rectBounds {
	if !bounds.Valid {
		return rectBounds{Valid: true, MinX: point.X, MinY: point.Y, MaxX: point.X, MaxY: point.Y}
	}
	if point.X < bounds.MinX {
		bounds.MinX = point.X
	}
	if point.Y < bounds.MinY {
		bounds.MinY = point.Y
	}
	if point.X > bounds.MaxX {
		bounds.MaxX = point.X
	}
	if point.Y > bounds.MaxY {
		bounds.MaxY = point.Y
	}
	return bounds
}

func courtyardOverlaps(courtyards []courtyardBounds) ([]string, int) {
	overlapRefs := map[string]struct{}{}
	count := 0
	for i := 0; i < len(courtyards); i++ {
		for j := i + 1; j < len(courtyards); j++ {
			if rectsOverlap(courtyards[i].Bounds, courtyards[j].Bounds) {
				count++
				overlapRefs[courtyards[i].Reference] = struct{}{}
				overlapRefs[courtyards[j].Reference] = struct{}{}
			}
		}
	}
	return sortedMapKeys(overlapRefs), count
}

func rectsOverlap(a, b rectBounds) bool {
	if !a.Valid || !b.Valid {
		return false
	}
	return a.MinX < b.MaxX && a.MaxX > b.MinX && a.MinY < b.MaxY && a.MaxY > b.MinY
}

func silkscreenTextInsideBoard(bounds boardBounds, footprint *pcbfiles.Footprint, text pcbfiles.FootprintText) bool {
	width := kicadfiles.MM(math.Max(0.6, float64(len(text.Text))*0.6))
	height := kicadfiles.MM(1.0)
	halfWidth := float64(width) / 2
	halfHeight := float64(height) / 2
	radians := float64(text.Rotation) * math.Pi / 180
	cosine := math.Cos(radians)
	sine := math.Sin(radians)
	for _, corner := range []struct{ x, y float64 }{
		{-halfWidth, -halfHeight},
		{-halfWidth, halfHeight},
		{halfWidth, -halfHeight},
		{halfWidth, halfHeight},
	} {
		local := kicadfiles.Point{
			X: text.Position.X + kicadfiles.IU(math.Round(corner.x*cosine-corner.y*sine)),
			Y: text.Position.Y + kicadfiles.IU(math.Round(corner.x*sine+corner.y*cosine)),
		}
		if !pointInsideBoard(bounds, transformFootprintPoint(footprint, local)) {
			return false
		}
	}
	return pointInsideBoard(bounds, transformFootprintPoint(footprint, text.Position))
}

func appendLimited(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || len(values) >= maxReportedObjects {
		return values
	}
	return append(values, value)
}

func outlinePolygons(board *pcbfiles.PCBFile, bounds boardBounds) [][]kicadfiles.Point {
	var polygons [][]kicadfiles.Point
	for _, drawing := range board.Drawings {
		if drawing.Layer != kicadfiles.LayerEdge {
			continue
		}
		if drawing.Rect != nil {
			start := drawing.Rect.Start
			end := drawing.Rect.End
			polygons = append(polygons, []kicadfiles.Point{
				{X: start.X, Y: start.Y},
				{X: end.X, Y: start.Y},
				{X: end.X, Y: end.Y},
				{X: start.X, Y: end.Y},
			})
		}
		if drawing.Poly != nil && len(drawing.Poly.Points) >= 4 && closePoint(drawing.Poly.Points[0], drawing.Poly.Points[len(drawing.Poly.Points)-1]) {
			polygons = append(polygons, slices.Clone(drawing.Poly.Points[:len(drawing.Poly.Points)-1]))
		}
		if drawing.Circle != nil {
			polygons = append(polygons, circlePolygon(*drawing.Circle))
		}
	}
	if len(polygons) > 0 {
		return polygons
	}
	if linePolygons := lineOutlinePolygons(board); len(linePolygons) > 0 {
		return linePolygons
	}
	return nil
}

func lineOutlinePolygons(board *pcbfiles.PCBFile) [][]kicadfiles.Point {
	for _, drawing := range board.Drawings {
		if drawing.Layer == kicadfiles.LayerEdge && drawing.Circle != nil {
			return nil
		}
	}
	lines := polygonEdgeLines(board)
	if len(lines) < 3 || !edgeOutlineClosed(board) {
		return nil
	}
	seen := map[quantizedPointKey]pointKey{}
	points := map[pointKey]kicadfiles.Point{}
	adjacency := map[pointKey][]pointKey{}
	for _, line := range lines {
		start := canonicalPointKey(seen, line.Start)
		end := canonicalPointKey(seen, line.End)
		points[start] = line.Start
		points[end] = line.End
		adjacency[start] = append(adjacency[start], end)
		adjacency[end] = append(adjacency[end], start)
	}
	var polygons [][]kicadfiles.Point
	visited := map[pointKey]struct{}{}
	for start := range adjacency {
		if _, ok := visited[start]; ok {
			continue
		}
		var polygon []kicadfiles.Point
		var previous pointKey
		hasPrevious := false
		current := start
		for {
			polygon = append(polygon, points[current])
			visited[current] = struct{}{}
			nextCandidates := adjacency[current]
			var next pointKey
			found := false
			for _, candidate := range nextCandidates {
				if !hasPrevious || candidate != previous {
					next = candidate
					found = true
					break
				}
			}
			if !found {
				return nil
			}
			if next == start {
				break
			}
			previous, current = current, next
			hasPrevious = true
			if len(polygon) > len(lines) {
				return nil
			}
		}
		if len(polygon) >= 3 {
			polygons = append(polygons, polygon)
		}
	}
	return polygons
}

func polygonEdgeLines(board *pcbfiles.PCBFile) []edgeLine {
	var lines []edgeLine
	for _, drawing := range board.Drawings {
		if drawing.Layer != kicadfiles.LayerEdge {
			continue
		}
		if drawing.Line != nil {
			lines = append(lines, edgeLine{Start: drawing.Line.Start, End: drawing.Line.End})
		}
		if drawing.Rect != nil {
			start := drawing.Rect.Start
			end := drawing.Rect.End
			p1 := kicadfiles.Point{X: start.X, Y: start.Y}
			p2 := kicadfiles.Point{X: end.X, Y: start.Y}
			p3 := kicadfiles.Point{X: end.X, Y: end.Y}
			p4 := kicadfiles.Point{X: start.X, Y: end.Y}
			lines = append(lines,
				edgeLine{Start: p1, End: p2},
				edgeLine{Start: p2, End: p3},
				edgeLine{Start: p3, End: p4},
				edgeLine{Start: p4, End: p1},
			)
		}
		if drawing.Poly != nil {
			for i := 0; i+1 < len(drawing.Poly.Points); i++ {
				lines = append(lines, edgeLine{Start: drawing.Poly.Points[i], End: drawing.Poly.Points[i+1]})
			}
		}
		if drawing.Arc != nil {
			points := arcPolyline(*drawing.Arc)
			for i := 0; i+1 < len(points); i++ {
				lines = append(lines, edgeLine{Start: points[i], End: points[i+1]})
			}
		}
	}
	return lines
}

func arcPolyline(arc pcbfiles.ArcDrawing) []kicadfiles.Point {
	center, radius, ok := circleFromThreePoints(arc.Start, arc.Mid, arc.End)
	if !ok || radius <= 0 {
		return []kicadfiles.Point{arc.Start, arc.End}
	}
	start := angleFor(center, arc.Start)
	mid := angleFor(center, arc.Mid)
	end := angleFor(center, arc.End)
	ccw := angleBetweenCCW(start, end, mid)
	sweep := end - start
	if ccw {
		if sweep < 0 {
			sweep += 2 * math.Pi
		}
	} else {
		sweep = start - end
		if sweep < 0 {
			sweep += 2 * math.Pi
		}
	}
	segments := maxInt(8, int(math.Ceil(sweep/(math.Pi/16))))
	points := make([]kicadfiles.Point, 0, segments+1)
	for index := 0; index <= segments; index++ {
		delta := sweep * float64(index) / float64(segments)
		angle := start + delta
		if !ccw {
			angle = start - delta
		}
		points = append(points, kicadfiles.Point{
			X: center.X + kicadfiles.IU(math.Round(float64(radius)*math.Cos(angle))),
			Y: center.Y + kicadfiles.IU(math.Round(float64(radius)*math.Sin(angle))),
		})
	}
	points[0] = arc.Start
	points[len(points)-1] = arc.End
	return points
}

func circlePolygon(circle pcbfiles.CircleDrawing) []kicadfiles.Point {
	radius := distanceIU(circle.Center, circle.End)
	const segments = 64
	points := make([]kicadfiles.Point, 0, segments)
	for index := 0; index < segments; index++ {
		angle := 2 * math.Pi * float64(index) / segments
		points = append(points, kicadfiles.Point{
			X: circle.Center.X + kicadfiles.IU(math.Round(float64(radius)*math.Cos(angle))),
			Y: circle.Center.Y + kicadfiles.IU(math.Round(float64(radius)*math.Sin(angle))),
		})
	}
	return points
}

func pointInPolygon(polygon []kicadfiles.Point, point kicadfiles.Point) bool {
	inside := false
	x := float64(point.X)
	y := float64(point.Y)
	for i, j := 0, len(polygon)-1; i < len(polygon); j, i = i, i+1 {
		xi, yi := float64(polygon[i].X), float64(polygon[i].Y)
		xj, yj := float64(polygon[j].X), float64(polygon[j].Y)
		if pointOnSegment(polygon[j], polygon[i], point) {
			return true
		}
		intersects := (yi > y) != (yj > y)
		if intersects {
			xAtY := (xj-xi)*(y-yi)/(yj-yi) + xi
			if x <= xAtY {
				inside = !inside
			}
		}
	}
	return inside
}

func pointOnSegment(a, b, point kicadfiles.Point) bool {
	cross := iuToMM(point.Y-a.Y)*iuToMM(b.X-a.X) - iuToMM(point.X-a.X)*iuToMM(b.Y-a.Y)
	tolerance := outlineToleranceMM * iuToMM(distanceIU(a, b))
	if math.Abs(cross) > tolerance {
		return false
	}
	return point.X >= minIU(a.X, b.X) && point.X <= maxIU(a.X, b.X) && point.Y >= minIU(a.Y, b.Y) && point.Y <= maxIU(a.Y, b.Y)
}

func minIU(a, b kicadfiles.IU) kicadfiles.IU {
	if a < b {
		return a
	}
	return b
}

func segmentIntersectsPolygonBoundary(start, end kicadfiles.Point, polygons [][]kicadfiles.Point) bool {
	for _, polygon := range polygons {
		for i, j := 0, len(polygon)-1; i < len(polygon); j, i = i, i+1 {
			a := polygon[j]
			b := polygon[i]
			if sharesEndpoint(start, end, a, b) {
				continue
			}
			if segmentsIntersect(start, end, a, b) {
				return true
			}
		}
	}
	return false
}

func sharesEndpoint(a1, a2, b1, b2 kicadfiles.Point) bool {
	return closePoint(a1, b1) || closePoint(a1, b2) || closePoint(a2, b1) || closePoint(a2, b2)
}

func segmentsIntersect(a1, a2, b1, b2 kicadfiles.Point) bool {
	o1 := orientation(a1, a2, b1)
	o2 := orientation(a1, a2, b2)
	o3 := orientation(b1, b2, a1)
	o4 := orientation(b1, b2, a2)
	if o1 == 0 && pointOnSegment(a1, a2, b1) {
		return true
	}
	if o2 == 0 && pointOnSegment(a1, a2, b2) {
		return true
	}
	if o3 == 0 && pointOnSegment(b1, b2, a1) {
		return true
	}
	if o4 == 0 && pointOnSegment(b1, b2, a2) {
		return true
	}
	return o1 != o2 && o3 != o4
}

func orientation(a, b, c kicadfiles.Point) int {
	value := iuToMM(b.Y-a.Y)*iuToMM(c.X-b.X) - iuToMM(b.X-a.X)*iuToMM(c.Y-b.Y)
	tolerance := outlineToleranceMM * iuToMM(distanceIU(a, b)) * iuToMM(distanceIU(b, c))
	switch {
	case math.Abs(value) <= tolerance:
		return 0
	case value > 0:
		return 1
	default:
		return -1
	}
}

func segmentPolygonDistance(start, end kicadfiles.Point, polygons [][]kicadfiles.Point) float64 {
	minDistance := math.Inf(1)
	for _, polygon := range polygons {
		for i, j := 0, len(polygon)-1; i < len(polygon); j, i = i, i+1 {
			distance := segmentSegmentDistanceMM(start, end, polygon[j], polygon[i])
			if distance < minDistance {
				minDistance = distance
			}
		}
	}
	return minDistance
}

func pointPolygonDistance(point kicadfiles.Point, polygons [][]kicadfiles.Point) float64 {
	minDistance := math.Inf(1)
	for _, polygon := range polygons {
		for i, j := 0, len(polygon)-1; i < len(polygon); j, i = i, i+1 {
			distance := pointSegmentDistanceMM(point, polygon[j], polygon[i])
			if distance < minDistance {
				minDistance = distance
			}
		}
	}
	return minDistance
}

func segmentSegmentDistanceMM(a1, a2, b1, b2 kicadfiles.Point) float64 {
	if segmentsIntersect(a1, a2, b1, b2) {
		return 0
	}
	return math.Min(
		math.Min(pointSegmentDistanceMM(a1, b1, b2), pointSegmentDistanceMM(a2, b1, b2)),
		math.Min(pointSegmentDistanceMM(b1, a1, a2), pointSegmentDistanceMM(b2, a1, a2)),
	)
}

func pointSegmentDistanceMM(point, start, end kicadfiles.Point) float64 {
	px, py := iuToMM(point.X), iuToMM(point.Y)
	x1, y1 := iuToMM(start.X), iuToMM(start.Y)
	x2, y2 := iuToMM(end.X), iuToMM(end.Y)
	dx := x2 - x1
	dy := y2 - y1
	lengthSquared := dx*dx + dy*dy
	if lengthSquared == 0 {
		return math.Hypot(px-x1, py-y1)
	}
	t := ((px-x1)*dx + (py-y1)*dy) / lengthSquared
	t = math.Max(0, math.Min(1, t))
	projX := x1 + t*dx
	projY := y1 + t*dy
	return math.Hypot(px-projX, py-projY)
}
