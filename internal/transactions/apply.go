package transactions

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/designapi"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/manifest"
	"kicadai/internal/reports"
)

const applyLockFileName = ".kicadai.apply.lock"

type ApplyOptions struct {
	OutputDir string
	Overwrite bool
	Seed      string
}

type ApplyResult struct {
	Plan      Plan               `json:"plan"`
	Artifacts []reports.Artifact `json:"artifacts"`
	Issues    []reports.Issue    `json:"issues"`
}

func Apply(tx Transaction, opts ApplyOptions) ApplyResult {
	plan := PlanTransaction(opts.OutputDir, tx)
	result := ApplyResult{Plan: plan, Artifacts: []reports.Artifact{}, Issues: append([]reports.Issue{}, plan.Issues...)}
	if reports.HasBlockingIssue(result.Issues) {
		return result
	}
	if existingProjectTarget(opts.OutputDir) {
		return applyImported(tx, opts, result)
	}
	if len(tx.Operations) == 0 || tx.Operations[0].Op != OpCreateProject {
		result.Issues = append(result.Issues, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "operations[0].op",
			Message:  "create_project must be the first operation for apply",
		})
		return result
	}
	builder, err := builderFromTransaction(tx, opts)
	if err != nil {
		result.Issues = append(result.Issues, applyIssue(0, err))
		return result
	}
	for i, op := range tx.Operations {
		if op.Op == OpCreateProject {
			continue
		}
		artifacts, err := applyOperation(builder, op, opts)
		if err != nil {
			result.Issues = append(result.Issues, applyIssue(i, err))
			return result
		}
		result.Artifacts = append(result.Artifacts, artifacts...)
	}
	if len(result.Artifacts) > 0 {
		manifestArtifact, err := writeManifestForApply(opts.OutputDir, tx, result.Artifacts)
		if err != nil {
			result.Issues = append(result.Issues, applyIssue(len(tx.Operations)-1, err))
			return result
		}
		result.Artifacts = append(result.Artifacts, manifestArtifact)
	}
	return result
}

func applyImported(tx Transaction, opts ApplyOptions, result ApplyResult) ApplyResult {
	for i, op := range tx.Operations {
		if op.Op != OpWriteProject {
			continue
		}
		for next := i + 1; next < len(tx.Operations); next++ {
			if touchesDesign(tx.Operations[next].Op) {
				result.Issues = append(result.Issues, reports.Issue{
					Code:     reports.CodeInvalidArgument,
					Severity: reports.SeverityError,
					Path:     fmt.Sprintf("operations[%d]", next),
					Message:  "write_project must be the final imported project mutation",
				})
				return result
			}
		}
	}
	releaseLock, err := acquireProjectApplyLock(opts.OutputDir)
	if err != nil {
		result.Issues = append(result.Issues, applyIssue(0, err))
		return result
	}
	defer releaseLock()
	design, err := kicaddesign.ReadProjectDirectory(opts.OutputDir)
	if err != nil {
		result.Issues = append(result.Issues, applyIssue(0, err))
		return result
	}
	projectBase, err := projectBaseName(opts.OutputDir)
	if err != nil {
		result.Issues = append(result.Issues, applyIssue(0, err))
		return result
	}
	generator, err := kicadfiles.NewDeterministicIDGenerator(deterministicDesignUUID(projectBase, opts.Seed), firstNonEmpty(opts.Seed, projectBase))
	if err != nil {
		result.Issues = append(result.Issues, applyIssue(0, err))
		return result
	}
	schematicDirty := false
	pcbDirty := false
	for i, op := range tx.Operations {
		switch op.Op {
		case OpWriteProject:
			artifacts, err := writeImportedProject(opts.OutputDir, projectBase, design, schematicDirty, pcbDirty)
			if err != nil {
				result.Issues = append(result.Issues, applyIssue(i, err))
				return result
			}
			result.Artifacts = append(result.Artifacts, artifacts...)
			schematicDirty = false
			pcbDirty = false
		case OpAddSymbol:
			if design.Schematic == nil {
				result.Issues = append(result.Issues, applyIssue(i, fmt.Errorf("root schematic required")))
				return result
			}
			var payload AddSymbolOperation
			if err := decodeRaw(op, &payload); err != nil {
				result.Issues = append(result.Issues, applyIssue(i, err))
				return result
			}
			opIndex := fmt.Sprintf("%d", i)
			symbol := schematic.NewSymbol(generator.New("imported.schematic.symbol", payload.Ref, opIndex), payload.LibraryID, payload.Ref, firstNonEmpty(payload.Value, payload.Ref), point(payload.At.XMM, payload.At.YMM))
			for _, pin := range payload.Pins {
				symbol.Pins = append(symbol.Pins, schematic.SymbolPin{Number: pin.Number, UUID: generator.New("imported.schematic.symbol.pin", payload.Ref, opIndex, pin.Number)})
			}
			design.Schematic.Symbols = append(design.Schematic.Symbols, symbol)
			schematicDirty = true
		case OpAssignFootprint:
			var payload AssignFootprintOperation
			if err := decodeRaw(op, &payload); err != nil {
				result.Issues = append(result.Issues, applyIssue(i, err))
				return result
			}
			if err := assignImportedFootprint(design.Schematic, payload.Ref, payload.FootprintID); err != nil {
				result.Issues = append(result.Issues, applyIssue(i, err))
				return result
			}
			schematicDirty = true
		case OpPlaceFootprint:
			if design.PCB == nil {
				result.Issues = append(result.Issues, applyIssue(i, fmt.Errorf("PCB required")))
				return result
			}
			var payload PlaceFootprintOperation
			if err := decodeRaw(op, &payload); err != nil {
				result.Issues = append(result.Issues, applyIssue(i, err))
				return result
			}
			if strings.TrimSpace(payload.FootprintID) == "" {
				result.Issues = append(result.Issues, applyIssue(i, fmt.Errorf("place_footprint requires footprint_id for imported apply")))
				return result
			}
			upsertImportedFootprint(design.PCB, generator, payload)
			pcbDirty = true
		case OpRoute:
			if design.PCB == nil {
				result.Issues = append(result.Issues, applyIssue(i, fmt.Errorf("PCB required")))
				return result
			}
			var payload RouteOperation
			if err := decodeRaw(op, &payload); err != nil {
				result.Issues = append(result.Issues, applyIssue(i, err))
				return result
			}
			points := make([]kicadfiles.Point, 0, len(payload.Points))
			for _, p := range payload.Points {
				points = append(points, point(p.XMM, p.YMM))
			}
			for p := 0; p < len(points)-1; p++ {
				design.PCB.Tracks = append(design.PCB.Tracks, pcb.Track{
					UUID:    generator.New("imported.pcb.route", payload.NetName, pointSeed(points[p]), pointSeed(points[p+1])),
					Start:   points[p],
					End:     points[p+1],
					Width:   kicadfiles.MM(payload.WidthMM),
					Layer:   boardLayer(payload.Layer),
					NetName: payload.NetName,
				})
			}
			pcbDirty = true
		case OpAddZone:
			if design.PCB == nil {
				result.Issues = append(result.Issues, applyIssue(i, fmt.Errorf("PCB required")))
				return result
			}
			var payload AddZoneOperation
			if err := decodeRaw(op, &payload); err != nil {
				result.Issues = append(result.Issues, applyIssue(i, err))
				return result
			}
			if payload.NetName == nil {
				result.Issues = append(result.Issues, applyIssue(i, fmt.Errorf("add_zone requires net_name for imported apply")))
				return result
			}
			zone := pcb.Zone{UUID: generator.New("imported.pcb.zone", *payload.NetName, pointsSeed(payload.Polygon)), NetName: *payload.NetName, Name: firstNonEmpty(payload.Name, *payload.NetName), MinThickness: kicadfiles.MM(0.25)}
			for _, layer := range payload.Layers {
				zone.Layers = append(zone.Layers, boardLayer(layer))
			}
			if len(zone.Layers) == 0 {
				zone.Layers = []kicadfiles.BoardLayer{kicadfiles.LayerFCu}
			}
			polygon := make([]kicadfiles.Point, 0, len(payload.Polygon))
			for _, p := range payload.Polygon {
				polygon = append(polygon, point(p.XMM, p.YMM))
			}
			zone.Polygons = [][]kicadfiles.Point{polygon}
			design.PCB.Zones = append(design.PCB.Zones, zone)
			pcbDirty = true
		default:
			result.Issues = append(result.Issues, applyIssue(i, fmt.Errorf("operation %s is not supported by imported apply", op.Op)))
			return result
		}
	}
	if schematicDirty || pcbDirty {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "operations", Message: "write_project operation is required to write imported project changes"})
	}
	return result
}

func writeManifestForApply(outputDir string, tx Transaction, artifacts []reports.Artifact) (reports.Artifact, error) {
	projectName := projectNameFromTransaction(tx)
	if projectName == "" {
		projectName = "generated_design"
	}
	ops := make([]manifest.OperationSummary, 0, len(tx.Operations))
	for i, op := range tx.Operations {
		ops = append(ops, manifest.OperationSummary{Index: i, Op: string(op.Op)})
	}
	return manifest.Write(outputDir, manifest.Manifest{
		ProjectName:      projectName,
		GeneratorVersion: reports.Version,
		Operations:       ops,
		Artifacts:        artifacts,
	})
}

func builderFromTransaction(tx Transaction, opts ApplyOptions) (*designapi.Builder, error) {
	for _, op := range tx.Operations {
		if op.Op != OpCreateProject {
			continue
		}
		var payload CreateProjectOperation
		if err := json.Unmarshal(op.Raw, &payload); err != nil {
			return nil, err
		}
		return designapi.New(designapi.Options{
			Name:     payload.Name,
			Seed:     firstNonEmpty(opts.Seed, payload.Name),
			DesignID: deterministicDesignUUID(payload.Name, opts.Seed),
		})
	}
	return nil, fmt.Errorf("create_project operation is required")
}

func deterministicDesignUUID(name string, seed string) kicadfiles.UUID {
	source := firstNonEmpty(seed, name, "generated_design")
	sum := sha1.Sum([]byte("kicadai-transaction-design:" + source))
	bytes := sum[:16]
	bytes[6] = (bytes[6] & 0x0f) | 0x50
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return kicadfiles.UUID(fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16]))
}

func writeOutputDir(op Operation, fallback string) string {
	output := fallback
	var payload WriteProjectOperation
	if decodeRaw(op, &payload) == nil && strings.TrimSpace(payload.OutputDir) != "" {
		output = payload.OutputDir
	}
	return output
}

func applyOperation(builder *designapi.Builder, op Operation, opts ApplyOptions) ([]reports.Artifact, error) {
	switch op.Op {
	case OpSetBoardOutline:
		var payload SetBoardOutlineOperation
		if err := decodeRaw(op, &payload); err != nil {
			return nil, err
		}
		if payload.Board != nil {
			_, err := builder.SetRectangularBoardOutline(kicadfiles.MM(payload.Board.WidthMM), kicadfiles.MM(payload.Board.HeightMM))
			return nil, err
		}
		points := make([]kicadfiles.Point, 0, len(payload.Points))
		for _, p := range payload.Points {
			points = append(points, point(p.XMM, p.YMM))
		}
		_, err := builder.SetBoardOutline(points)
		return nil, err
	case OpAddSymbol:
		var payload AddSymbolOperation
		if err := decodeRaw(op, &payload); err != nil {
			return nil, err
		}
		pins := make([]designapi.PinSpec, 0, len(payload.Pins))
		for _, pin := range payload.Pins {
			pins = append(pins, designapi.PinSpec{Number: pin.Number, Offset: point(pin.XMM, pin.YMM)})
		}
		_, err := builder.AddSymbol(designapi.SymbolOptions{
			Reference: payload.Ref,
			Value:     payload.Value,
			LibraryID: payload.LibraryID,
			Position:  point(payload.At.XMM, payload.At.YMM),
			Pins:      pins,
		})
		return nil, err
	case OpConnect:
		var payload ConnectOperation
		if err := decodeRaw(op, &payload); err != nil {
			return nil, err
		}
		return nil, builder.Connect(endpoint(payload.From), endpoint(payload.To), payload.NetName)
	case OpAssignFootprint:
		var payload AssignFootprintOperation
		if err := decodeRaw(op, &payload); err != nil {
			return nil, err
		}
		return nil, builder.AssignFootprint(payload.Ref, payload.FootprintID)
	case OpPlaceFootprint:
		var payload PlaceFootprintOperation
		if err := decodeRaw(op, &payload); err != nil {
			return nil, err
		}
		pads := make([]designapi.PadSpec, 0, len(payload.Pads))
		for _, pad := range payload.Pads {
			net := ""
			if pad.Net != nil {
				net = *pad.Net
			}
			pads = append(pads, designapi.PadSpec{Name: pad.Name, Type: pad.Type, Shape: pad.Shape, Offset: point(pad.XMM, pad.YMM), Size: padSize(pad), Drill: kicadfiles.MM(pad.DrillMM), Net: net})
		}
		_, err := builder.PlaceFootprint(payload.Ref, designapi.PlaceFootprintOptions{
			Position: point(payload.At.XMM, payload.At.YMM),
			Rotation: kicadfiles.Angle(payload.Rotation),
			Layer:    boardLayer(payload.Layer),
			Pads:     pads,
		})
		return nil, err
	case OpRoute:
		var payload RouteOperation
		if err := decodeRaw(op, &payload); err != nil {
			return nil, err
		}
		points := make([]kicadfiles.Point, 0, len(payload.Points))
		for _, p := range payload.Points {
			points = append(points, point(p.XMM, p.YMM))
		}
		_, err := builder.Route(payload.NetName, points, designapi.RouteOptions{Layer: boardLayer(payload.Layer), Width: kicadfiles.MM(payload.WidthMM)})
		return nil, err
	case OpAddZone:
		var payload AddZoneOperation
		if err := decodeRaw(op, &payload); err != nil {
			return nil, err
		}
		if payload.NetName == nil {
			return nil, fmt.Errorf("add_zone requires net_name for apply")
		}
		polygon := make([]kicadfiles.Point, 0, len(payload.Polygon))
		for _, p := range payload.Polygon {
			polygon = append(polygon, point(p.XMM, p.YMM))
		}
		layers := make([]kicadfiles.BoardLayer, 0, len(payload.Layers))
		for _, layer := range payload.Layers {
			if strings.TrimSpace(layer) != "" {
				layers = append(layers, boardLayer(layer))
			}
		}
		_, err := builder.AddZone(*payload.NetName, polygon, designapi.ZoneOptions{Name: payload.Name, Layers: layers})
		return nil, err
	case OpWriteProject:
		var payload WriteProjectOperation
		if err := decodeRaw(op, &payload); err != nil {
			return nil, err
		}
		if strings.TrimSpace(payload.OutputDir) != "" {
			return nil, fmt.Errorf("write_project.output_dir override is not allowed; use CLI output directory")
		}
		output := opts.OutputDir
		if strings.TrimSpace(output) == "" {
			return nil, fmt.Errorf("output directory required")
		}
		writeResult, err := builder.WriteProject(output, kicaddesign.WriteOptions{Overwrite: opts.Overwrite || payload.Overwrite})
		if err != nil {
			return nil, err
		}
		return artifactsFromWrittenFiles(writeResult.WrittenFiles), nil
	default:
		return nil, fmt.Errorf("operation %s is not supported by apply", op.Op)
	}
}

func artifactsFromWrittenFiles(paths []string) []reports.Artifact {
	artifacts := make([]reports.Artifact, 0, len(paths))
	for _, path := range paths {
		artifacts = append(artifacts, reports.Artifact{Kind: artifactKindForPath(path), Path: filepath.ToSlash(path)})
	}
	return artifacts
}

func artifactKindForPath(path string) reports.ArtifactKind {
	switch filepath.Ext(path) {
	case ".kicad_pro":
		return reports.ArtifactKiCadProject
	case ".kicad_sch":
		return reports.ArtifactSchematic
	case ".kicad_pcb":
		return reports.ArtifactPCB
	default:
		return reports.ArtifactValidationReport
	}
}

func projectBaseName(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	var base string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".kicad_pro" {
			continue
		}
		if base != "" {
			return "", fmt.Errorf("multiple .kicad_pro files in %s", root)
		}
		base = strings.TrimSuffix(entry.Name(), ".kicad_pro")
	}
	if base == "" {
		return "", fmt.Errorf("no .kicad_pro file in %s", root)
	}
	return base, nil
}

func assignImportedFootprint(file *schematic.SchematicFile, ref string, footprintID string) error {
	if file == nil {
		return fmt.Errorf("root schematic required")
	}
	matched := false
	for i := range file.Symbols {
		if file.Symbols[i].Reference != ref {
			continue
		}
		matched = true
		assignImportedSymbolFootprint(&file.Symbols[i], footprintID)
	}
	if !matched {
		return fmt.Errorf("reference %s not found", ref)
	}
	return nil
}

func assignImportedSymbolFootprint(symbol *schematic.SchematicSymbol, footprintID string) {
	for i := range symbol.Properties {
		if strings.EqualFold(strings.TrimSpace(symbol.Properties[i].Name), "Footprint") {
			symbol.Properties[i].Name = "Footprint"
			symbol.Properties[i].Value = footprintID
			return
		}
	}
	symbol.Properties = append(symbol.Properties, schematic.Property{Name: "Footprint", Value: footprintID, Hidden: true, Position: symbol.Position, Rotation: symbol.Rotation})
}

func upsertImportedFootprint(board *pcb.PCBFile, generator kicadfiles.IDGenerator, payload PlaceFootprintOperation) {
	for i := range board.Footprints {
		if board.Footprints[i].Reference == payload.Ref {
			updateImportedFootprint(&board.Footprints[i], generator, payload)
			return
		}
	}
	value := firstNonEmpty(payload.Value, payload.Ref)
	footprint := pcb.Footprint{
		UUID:      generator.New("imported.pcb.footprint", payload.Ref),
		LibraryID: strings.TrimSpace(payload.FootprintID),
		Reference: payload.Ref,
		Value:     value,
		Position:  point(payload.At.XMM, payload.At.YMM),
		Rotation:  kicadfiles.Angle(payload.Rotation),
		Layer:     boardLayer(payload.Layer),
		Properties: []pcb.FootprintProperty{
			{Name: "Reference", Value: payload.Ref, Position: kicadfiles.Point{Y: kicadfiles.MM(-1.5)}, Layer: kicadfiles.LayerFSilkS, UUID: generator.New("imported.pcb.footprint.property", payload.Ref, "Reference")},
			{Name: "Value", Value: value, Position: kicadfiles.Point{Y: kicadfiles.MM(1.5)}, Layer: kicadfiles.LayerFSilkS, UUID: generator.New("imported.pcb.footprint.property", payload.Ref, "Value")},
		},
	}
	for i, padSpec := range payload.Pads {
		net := ""
		if padSpec.Net != nil {
			net = *padSpec.Net
		}
		footprint.Pads = append(footprint.Pads, pcb.Pad{
			UUID:     generator.New("imported.pcb.footprint.pad", payload.Ref, padSpec.Name, fmt.Sprintf("%d", i)),
			Name:     padSpec.Name,
			Type:     firstNonEmpty(padSpec.Type, "smd"),
			Shape:    firstNonEmpty(padSpec.Shape, "rect"),
			Position: point(padSpec.XMM, padSpec.YMM),
			Size:     padSizeOrDefault(padSpec),
			Drill:    kicadfiles.MM(padSpec.DrillMM),
			Layers:   padLayersFor(firstNonEmpty(padSpec.Type, "smd"), footprint.Layer),
			NetName:  net,
		})
	}
	board.Footprints = append(board.Footprints, footprint)
}

func updateImportedFootprint(footprint *pcb.Footprint, generator kicadfiles.IDGenerator, payload PlaceFootprintOperation) {
	footprint.Position = point(payload.At.XMM, payload.At.YMM)
	footprint.Rotation = kicadfiles.Angle(payload.Rotation)
	if layer := boardLayer(payload.Layer); layer != "" {
		footprint.Layer = layer
	}
	if strings.TrimSpace(footprint.LibraryID) == "" {
		footprint.LibraryID = strings.TrimSpace(payload.FootprintID)
	}
	if strings.TrimSpace(payload.Value) != "" {
		footprint.Value = strings.TrimSpace(payload.Value)
		for i := range footprint.Properties {
			if footprint.Properties[i].Name == "Value" {
				footprint.Properties[i].Value = footprint.Value
			}
		}
	}
	for i, padSpec := range payload.Pads {
		updateImportedPad(footprint, generator, payload.Ref, padSpec, i)
	}
}

func updateImportedPad(footprint *pcb.Footprint, generator kicadfiles.IDGenerator, ref string, padSpec PadSpec, index int) {
	net := ""
	if padSpec.Net != nil {
		net = *padSpec.Net
	}
	matched := false
	for i := range footprint.Pads {
		if footprint.Pads[i].Name != padSpec.Name {
			continue
		}
		matched = true
		pad := &footprint.Pads[i]
		if strings.TrimSpace(padSpec.Type) != "" {
			pad.Type = strings.TrimSpace(padSpec.Type)
			pad.Layers = padLayersFor(pad.Type, footprint.Layer)
		}
		if strings.TrimSpace(padSpec.Shape) != "" {
			pad.Shape = strings.TrimSpace(padSpec.Shape)
		}
		pad.Position = point(padSpec.XMM, padSpec.YMM)
		if padSpec.WidthMM > 0 || padSpec.HeightMM > 0 {
			pad.Size = padSizeOrDefault(padSpec)
		}
		if padSpec.DrillMM > 0 {
			pad.Drill = kicadfiles.MM(padSpec.DrillMM)
		}
		if padSpec.Net != nil {
			pad.NetName = net
		}
	}
	if matched {
		return
	}
	footprint.Pads = append(footprint.Pads, pcb.Pad{
		UUID:     generator.New("imported.pcb.footprint.pad", ref, padSpec.Name, fmt.Sprintf("%d", index)),
		Name:     padSpec.Name,
		Type:     firstNonEmpty(padSpec.Type, "smd"),
		Shape:    firstNonEmpty(padSpec.Shape, "rect"),
		Position: point(padSpec.XMM, padSpec.YMM),
		Size:     padSizeOrDefault(padSpec),
		Drill:    kicadfiles.MM(padSpec.DrillMM),
		Layers:   padLayersFor(firstNonEmpty(padSpec.Type, "smd"), footprint.Layer),
		NetName:  net,
	})
}

func padSize(pad PadSpec) kicadfiles.Point {
	return kicadfiles.Point{X: kicadfiles.MM(pad.WidthMM), Y: kicadfiles.MM(pad.HeightMM)}
}

func padSizeOrDefault(pad PadSpec) kicadfiles.Point {
	size := padSize(pad)
	if size.X <= 0 {
		size.X = kicadfiles.MM(1.6)
	}
	if size.Y <= 0 {
		size.Y = size.X
	}
	return size
}

func padLayersFor(padType string, footprintLayer kicadfiles.BoardLayer) []kicadfiles.BoardLayer {
	switch strings.TrimSpace(padType) {
	case "thru_hole", "np_thru_hole":
		return []kicadfiles.BoardLayer{kicadfiles.LayerAllCu, kicadfiles.LayerAllMask}
	}
	if footprintLayer == kicadfiles.LayerBCu {
		return []kicadfiles.BoardLayer{kicadfiles.LayerBCu, kicadfiles.LayerBMask}
	}
	return []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFMask}
}

func writeImportedProject(root string, base string, design kicaddesign.Design, schematicDirty bool, pcbDirty bool) ([]reports.Artifact, error) {
	var artifacts []reports.Artifact
	if schematicDirty {
		if design.Schematic == nil {
			return nil, fmt.Errorf("root schematic required")
		}
		normalizeImportedSchematic(design.Schematic)
		path := filepath.Join(root, base+".kicad_sch")
		if err := writeSchematicAtomic(path, *design.Schematic); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, reports.Artifact{Kind: reports.ArtifactSchematic, Path: filepath.ToSlash(path)})
	}
	if pcbDirty {
		if design.PCB == nil {
			return nil, fmt.Errorf("PCB required")
		}
		normalizeImportedPCB(design.PCB)
		path := filepath.Join(root, base+".kicad_pcb")
		if err := writePCBAtomic(path, *design.PCB); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, reports.Artifact{Kind: reports.ArtifactPCB, Path: filepath.ToSlash(path)})
	}
	return artifacts, nil
}

func normalizeImportedSchematic(file *schematic.SchematicFile) {
	if strings.TrimSpace(file.GeneratorVersion) == "" {
		file.GeneratorVersion = "10.0"
	}
}

func normalizeImportedPCB(board *pcb.PCBFile) {
	if strings.TrimSpace(board.GeneratorVersion) == "" {
		board.GeneratorVersion = "10.0"
	}
	if board.General.Thickness <= 0 {
		board.General = pcb.DefaultGeneral()
	}
	if len(board.Layers) == 0 {
		board.Layers = pcb.DefaultTwoLayerStack()
	}
	existingNetNames := make([]string, 0, len(board.Nets))
	for _, net := range board.Nets {
		if strings.TrimSpace(net.Name) != "" {
			existingNetNames = append(existingNetNames, net.Name)
		}
	}
	registry := pcb.NewNetRegistry(existingNetNames...)
	for footprintIndex := range board.Footprints {
		footprint := &board.Footprints[footprintIndex]
		if strings.TrimSpace(footprint.Path) == "" && strings.TrimSpace(footprint.Reference) != "" {
			footprint.Path = "/" + strings.TrimSpace(footprint.Reference)
		}
		for propertyIndex := range footprint.Properties {
			property := &footprint.Properties[propertyIndex]
			if !kicadfiles.IsValidBoardLayer(property.Layer) {
				property.Layer = kicadfiles.LayerFSilkS
			}
		}
		for padIndex := range footprint.Pads {
			pad := &footprint.Pads[padIndex]
			if net := registry.EnsureNet(pad.NetName); net.Name != "" {
				pad.NetCode = net.Code
				pad.NetName = net.Name
			}
		}
	}
	for drawingIndex := range board.Drawings {
		drawing := &board.Drawings[drawingIndex]
		if drawing.Layer == "" {
			drawing.Layer = kicadfiles.LayerEdge
		}
		if drawing.Line != nil && drawing.Line.Width <= 0 {
			drawing.Line.Width = kicadfiles.MM(0.1)
		}
	}
	for trackIndex := range board.Tracks {
		track := &board.Tracks[trackIndex]
		if track.Width <= 0 {
			track.Width = kicadfiles.MM(0.25)
		}
		if !kicadfiles.IsValidBoardLayer(track.Layer) {
			track.Layer = kicadfiles.LayerFCu
		}
		if net := registry.EnsureNet(track.NetName); net.Name != "" {
			track.NetCode = net.Code
			track.NetName = net.Name
		}
	}
	for zoneIndex := range board.Zones {
		zone := &board.Zones[zoneIndex]
		if zone.MinThickness <= 0 {
			zone.MinThickness = kicadfiles.MM(0.25)
		}
		if net := registry.EnsureNet(zone.NetName); net.Name != "" {
			zone.NetCode = net.Code
			zone.NetName = net.Name
		}
	}
	board.Nets = registry.Nets()
}

func writeSchematicAtomic(path string, file schematic.SchematicFile) error {
	return writeAtomic(path, func(f *os.File) error { return schematic.Write(f, file) })
}

func writePCBAtomic(path string, file pcb.PCBFile) error {
	return writeAtomic(path, func(f *os.File) error { return pcb.Write(f, file) })
}

func writeAtomic(path string, write func(*os.File) error) error {
	dir := filepath.Dir(path)
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := write(tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	_ = tmp.Chmod(mode)
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func acquireProjectApplyLock(root string) (func(), error) {
	lockPath := filepath.Join(root, applyLockFileName)
	file, err := createApplyLock(lockPath)
	if err != nil {
		if !os.IsExist(err) {
			return nil, err
		}
		if stale, staleErr := removeStaleApplyLock(lockPath); staleErr != nil {
			return nil, staleErr
		} else if !stale {
			return nil, fmt.Errorf("project apply lock already exists: %s", lockPath)
		}
		file, err = createApplyLock(lockPath)
		if err != nil {
			if os.IsExist(err) {
				return nil, fmt.Errorf("project apply lock already exists: %s", lockPath)
			}
			return nil, err
		}
	}
	if _, err := file.WriteString(fmt.Sprintf("pid=%d\n", os.Getpid())); err != nil {
		_ = file.Close()
		_ = os.Remove(lockPath)
		return nil, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(lockPath)
		return nil, err
	}
	return func() {
		_ = os.Remove(lockPath)
	}, nil
}

func createApplyLock(lockPath string) (*os.File, error) {
	return os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
}

func removeStaleApplyLock(lockPath string) (bool, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false, err
	}
	pid, ok := parseApplyLockPID(string(data))
	if !ok || processAlive(pid) {
		return false, nil
	}
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return true, nil
}

func parseApplyLockPID(contents string) (int, bool) {
	for _, line := range strings.Split(contents, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || key != "pid" {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || pid <= 0 {
			return 0, false
		}
		return pid, true
	}
	return 0, false
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func applyIssue(index int, err error) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     "operations[" + fmt.Sprintf("%d", index) + "]",
		Message:  err.Error(),
	}
}

func endpoint(value Endpoint) designapi.Endpoint {
	return designapi.Endpoint{Reference: value.Ref, Pin: value.Pin}
}

func point(xMM, yMM float64) kicadfiles.Point {
	return kicadfiles.Point{X: kicadfiles.MM(xMM), Y: kicadfiles.MM(yMM)}
}

func pointSeed(point kicadfiles.Point) string {
	return fmt.Sprintf("%d,%d", point.X, point.Y)
}

func pointsSeed(points []Point) string {
	parts := make([]string, 0, len(points))
	for _, point := range points {
		parts = append(parts, fmt.Sprintf("%g,%g", point.XMM, point.YMM))
	}
	return strings.Join(parts, ";")
}

func boardLayer(value string) kicadfiles.BoardLayer {
	switch strings.TrimSpace(value) {
	case "", "F.Cu":
		return kicadfiles.LayerFCu
	case "B.Cu":
		return kicadfiles.LayerBCu
	case "F.Mask":
		return kicadfiles.LayerFMask
	case "B.Mask":
		return kicadfiles.LayerBMask
	case "F.Paste":
		return kicadfiles.LayerFPaste
	case "B.Paste":
		return kicadfiles.LayerBPaste
	case "F.SilkS":
		return kicadfiles.LayerFSilkS
	case "B.SilkS":
		return kicadfiles.LayerBSilkS
	default:
		return kicadfiles.BoardLayer(value)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
