package transactions

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/designapi"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/manifest"
	"kicadai/internal/reports"
)

const applyLockFileName = ".kicadai.apply.lock"
const transactionProvenanceSchema = "kicadai.transaction.provenance.v1"
const transactionProvenancePath = ".kicadai/transaction.json"

type ApplyOptions struct {
	OutputDir             string
	Overwrite             bool
	Seed                  string
	AllowImportedMutation bool
	LibraryIndex          *libraryresolver.LibraryIndex
	LibraryIssues         []reports.Issue
}

type ApplyResult struct {
	Plan      Plan               `json:"plan"`
	Artifacts []reports.Artifact `json:"artifacts"`
	Issues    []reports.Issue    `json:"issues"`
}

type transactionProvenance struct {
	Schema             string                      `json:"schema"`
	ProjectName        string                      `json:"project_name"`
	GeneratorVersion   string                      `json:"generator_version,omitempty"`
	CreatedBy          string                      `json:"created_by,omitempty"`
	Transaction        Transaction                 `json:"transaction"`
	OperationCount     int                         `json:"operation_count"`
	OperationSummaries []manifest.OperationSummary `json:"operation_summaries,omitempty"`
	Source             transactionProvenanceSource `json:"source,omitempty"`
}

type transactionProvenanceSource struct {
	Kind string `json:"kind,omitempty"`
	Seed string `json:"seed,omitempty"`
}

func Apply(tx Transaction, opts ApplyOptions) (result ApplyResult) {
	plan := PlanTransactionWithOptions(opts.OutputDir, tx, PlanOptions{
		LibraryIndex:            opts.LibraryIndex,
		LibraryIssues:           opts.LibraryIssues,
		AllowGeneratedOverwrite: generatedOverwriteApply(tx, opts),
	})
	result = ApplyResult{Plan: plan, Artifacts: []reports.Artifact{}, Issues: append([]reports.Issue{}, plan.Issues...)}
	// Keep every early return annotated with operation IDs from the plan.
	defer annotateApplyResultIssueOperationIDs(&result)
	result.Issues = append(result.Issues, opts.LibraryIssues...)
	if reports.HasBlockingIssue(result.Issues) {
		return result
	}
	if existingProjectTarget(opts.OutputDir) && !generatedOverwriteApply(tx, opts) {
		if !opts.AllowImportedMutation {
			result.Issues = append(result.Issues, reports.Issue{
				Code:       reports.CodePreservationConflict,
				Severity:   reports.SeverityBlocked,
				Path:       "transaction.apply.imported",
				Message:    "imported project apply is disabled by default",
				Suggestion: "rerun with explicit imported mutation approval after reviewing the transaction preservation report",
			})
			return result
		}
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
		metadataArtifacts, err := writeManifestForApply(opts.OutputDir, tx, result.Artifacts)
		if err != nil {
			result.Issues = append(result.Issues, applyIssue(lastOperationIndex(tx), err))
			return result
		}
		result.Artifacts = append(result.Artifacts, metadataArtifacts...)
	}
	return result
}

func generatedOverwriteApply(tx Transaction, opts ApplyOptions) bool {
	return opts.Overwrite && firstOperationKind(tx) == OpCreateProject
}

func annotateApplyResultIssueOperationIDs(result *ApplyResult) {
	if len(result.Issues) == 0 {
		return
	}
	AnnotateIssueOperationIDs(result.Issues, result.Plan.Operations)
	NewOperationTraceMapFromPlan(result.Plan).AnnotateIssues(result.Issues)
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
			if err := validateSymbolPropertyPayload(payload.Properties); err != nil {
				result.Issues = append(result.Issues, applyIssue(i, err))
				return result
			}
			opIndex := fmt.Sprintf("%d", i)
			pins, err := resolveSymbolPins(payload.Pins, opts.LibraryIndex, payload.LibraryID)
			if err != nil {
				result.Issues = append(result.Issues, applyIssue(i, err))
				return result
			}
			payload.Pins = pins
			symbol := schematic.NewSymbol(generator.New("imported.schematic.symbol", payload.Ref, opIndex), payload.LibraryID, payload.Ref, firstNonEmpty(payload.Value, payload.Ref), point(payload.At.XMM, payload.At.YMM))
			symbol.Rotation = kicadfiles.Angle(payload.Rotation)
			symbol.Properties = schematic.MergeProperties(symbol.Properties, schematicPropertiesFromPayload(payload.Properties, symbol.Position, symbol.Rotation, 2))
			for _, pin := range payload.Pins {
				symbol.Pins = append(symbol.Pins, schematic.SymbolPin{Number: pin.Number, UUID: generator.New("imported.schematic.symbol.pin", payload.Ref, opIndex, pin.Number)})
				symbol.PinAnchors = append(symbol.PinAnchors, addPoints(symbol.Position, point(pin.XMM, pin.YMM)))
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
		case OpAddNoConnect:
			if design.Schematic == nil {
				result.Issues = append(result.Issues, applyIssue(i, fmt.Errorf("root schematic required")))
				return result
			}
			var payload AddNoConnectOperation
			if err := decodeRaw(op, &payload); err != nil {
				result.Issues = append(result.Issues, applyIssue(i, err))
				return result
			}
			anchor, err := symbolPinAnchor(design.Schematic, payload.Endpoint.Ref, payload.Endpoint.Pin)
			if err != nil {
				result.Issues = append(result.Issues, applyIssue(i, err))
				return result
			}
			if !schematicHasNoConnect(design.Schematic.NoConnects, anchor) {
				design.Schematic.NoConnects = append(design.Schematic.NoConnects, schematic.NewNoConnect(generator.New("imported.schematic.no_connect", payload.Endpoint.Ref, payload.Endpoint.Pin, fmt.Sprintf("%d", i)), anchor))
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
			upsertImportedFootprintWithLibrary(design.PCB, generator, payload, opts.LibraryIndex)
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
			for _, via := range payload.Vias {
				layers := make([]kicadfiles.BoardLayer, 0, len(via.Layers))
				for _, layer := range via.Layers {
					layers = append(layers, boardLayer(layer))
				}
				design.PCB.Vias = append(design.PCB.Vias, pcb.Via{
					UUID:     generator.New("imported.pcb.via", payload.NetName, pointSeed(point(via.At.XMM, via.At.YMM)), strings.Join(via.Layers, ",")),
					Position: point(via.At.XMM, via.At.YMM),
					Size:     kicadfiles.MM(via.DiameterMM),
					Drill:    kicadfiles.MM(via.DrillMM),
					Layers:   layers,
					NetName:  payload.NetName,
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

func writeManifestForApply(outputDir string, tx Transaction, artifacts []reports.Artifact) ([]reports.Artifact, error) {
	projectName := projectNameFromTransaction(tx)
	if projectName == "" {
		projectName = "generated_design"
	}
	ops := operationSummariesForApply(tx)
	provenanceArtifact, err := writeTransactionProvenanceForApply(outputDir, projectName, tx)
	if err != nil {
		return nil, err
	}
	manifestArtifacts := append([]reports.Artifact(nil), artifacts...)
	manifestArtifacts = append(manifestArtifacts, provenanceArtifact)
	manifestArtifact, err := manifest.Write(outputDir, manifest.Manifest{
		ProjectName:      projectName,
		GeneratorVersion: reports.Version,
		Operations:       ops,
		Artifacts:        manifestArtifacts,
		Provenance: &manifest.ProvenanceRef{
			TransactionPath: transactionProvenancePath,
			Schema:          transactionProvenanceSchema,
			OperationCount:  len(tx.Operations),
		},
	})
	if err != nil {
		return nil, err
	}
	return []reports.Artifact{provenanceArtifact, manifestArtifact}, nil
}

func lastOperationIndex(tx Transaction) int {
	if len(tx.Operations) == 0 {
		return -1
	}
	return len(tx.Operations) - 1
}

func writeTransactionProvenanceForApply(outputDir string, projectName string, tx Transaction) (reports.Artifact, error) {
	data := transactionProvenance{
		Schema:             transactionProvenanceSchema,
		ProjectName:        projectName,
		GeneratorVersion:   reports.Version,
		CreatedBy:          "kicadai",
		Transaction:        tx,
		OperationCount:     len(tx.Operations),
		OperationSummaries: operationSummariesForApply(tx),
		Source:             transactionProvenanceSource{Kind: "transaction_apply"},
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return reports.Artifact{}, err
	}
	raw = append(raw, '\n')
	path := filepath.Join(outputDir, filepath.FromSlash(transactionProvenancePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return reports.Artifact{}, err
	}
	if err := writeFileAtomic(path, raw, 0o644); err != nil {
		return reports.Artifact{}, err
	}
	return reports.Artifact{Kind: reports.ArtifactValidationReport, Path: transactionProvenancePath, Description: "KiCadAI generated transaction provenance"}, nil
}

func operationSummariesForApply(tx Transaction) []manifest.OperationSummary {
	summaries := make([]manifest.OperationSummary, 0, len(tx.Operations))
	for index, op := range tx.Operations {
		summaries = append(summaries, manifest.OperationSummary{Index: index, Op: string(op.Op)})
	}
	return summaries
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if runes := []rune(base); len(runes) > 128 {
		base = string(runes[:128])
	}
	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	closed = true
	return os.Rename(tmpPath, path)
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
		if err := validateSymbolPropertyPayload(payload.Properties); err != nil {
			return nil, err
		}
		resolverPins, err := resolveSymbolPins(payload.Pins, opts.LibraryIndex, payload.LibraryID)
		if err != nil {
			return nil, err
		}
		payload.Pins = resolverPins
		pins := make([]designapi.PinSpec, 0, len(payload.Pins))
		for _, pin := range payload.Pins {
			pins = append(pins, designapi.PinSpec{Number: pin.Number, Offset: point(pin.XMM, pin.YMM)})
		}
		_, err = builder.AddSymbol(designapi.SymbolOptions{
			Reference:  payload.Ref,
			Value:      payload.Value,
			LibraryID:  payload.LibraryID,
			Position:   point(payload.At.XMM, payload.At.YMM),
			Rotation:   kicadfiles.Angle(payload.Rotation),
			Pins:       pins,
			Properties: schematicPropertiesFromPayload(payload.Properties, point(payload.At.XMM, payload.At.YMM), kicadfiles.Angle(payload.Rotation), 2),
		})
		return nil, err
	case OpConnect:
		var payload ConnectOperation
		if err := decodeRaw(op, &payload); err != nil {
			return nil, err
		}
		return nil, builder.Connect(endpoint(payload.From), endpoint(payload.To), payload.NetName)
	case OpAddNoConnect:
		var payload AddNoConnectOperation
		if err := decodeRaw(op, &payload); err != nil {
			return nil, err
		}
		return nil, builder.AddNoConnect(endpoint(payload.Endpoint))
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
		placeOptions := designapi.PlaceFootprintOptions{
			Position: point(payload.At.XMM, payload.At.YMM),
			Rotation: kicadfiles.Angle(payload.Rotation),
			Layer:    boardLayer(payload.Layer),
			Pads:     pads,
		}
		if opts.LibraryIndex != nil && strings.TrimSpace(payload.FootprintID) != "" {
			if record, ok := libraryresolver.ResolveFootprint(*opts.LibraryIndex, payload.FootprintID); ok {
				enrichPlaceFootprintOptionsWithRecord(&placeOptions, record, boardLayer(payload.Layer))
			}
		}
		_, err := builder.PlaceFootprint(payload.Ref, placeOptions)
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
	if len(payload.Pads) > 0 {
		reconcileImportedPads(footprint, generator, payload.Ref, payload.Pads)
	}
}

func reconcileImportedPads(footprint *pcb.Footprint, generator kicadfiles.IDGenerator, ref string, specs []PadSpec) {
	existing := map[string][]pcb.Pad{}
	for _, pad := range footprint.Pads {
		existing[pad.Name] = append(existing[pad.Name], pad)
	}
	pads := make([]pcb.Pad, 0, len(specs))
	for i, spec := range specs {
		var pad pcb.Pad
		if matches := existing[spec.Name]; len(matches) > 0 {
			pad = matches[0]
			existing[spec.Name] = matches[1:]
		} else {
			pad = pcb.Pad{
				UUID:   generator.New("imported.pcb.footprint.pad", ref, spec.Name, fmt.Sprintf("%d", i)),
				Type:   "smd",
				Shape:  "rect",
				Size:   kicadfiles.Point{X: kicadfiles.MM(1.6), Y: kicadfiles.MM(1.6)},
				Layers: padLayersFor("smd", footprint.Layer),
			}
		}
		applyImportedPadSpec(&pad, footprint.Layer, spec)
		pads = append(pads, pad)
	}
	footprint.Pads = pads
}

func applyImportedPadSpec(pad *pcb.Pad, footprintLayer kicadfiles.BoardLayer, spec PadSpec) {
	pad.Name = spec.Name
	if strings.TrimSpace(spec.Type) != "" {
		pad.Type = strings.TrimSpace(spec.Type)
	} else if strings.TrimSpace(pad.Type) == "" {
		pad.Type = "smd"
	}
	pad.Layers = padLayersFor(pad.Type, footprintLayer)
	if strings.TrimSpace(spec.Shape) != "" {
		pad.Shape = strings.TrimSpace(spec.Shape)
	} else if strings.TrimSpace(pad.Shape) == "" {
		pad.Shape = "rect"
	}
	pad.Position = point(spec.XMM, spec.YMM)
	if spec.WidthMM > 0 || spec.HeightMM > 0 {
		pad.Size = padSizeOrDefault(spec)
	} else if pad.Size == (kicadfiles.Point{}) {
		pad.Size = padSizeOrDefault(spec)
	}
	if spec.DrillMM > 0 {
		pad.Drill = kicadfiles.MM(spec.DrillMM)
	}
	if spec.Net != nil {
		pad.NetName = *spec.Net
	}
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
	var writes []importedProjectWrite
	if schematicDirty {
		if design.Schematic == nil {
			return nil, fmt.Errorf("root schematic required")
		}
		normalizeImportedSchematic(design.Schematic)
		path := filepath.Join(root, base+".kicad_sch")
		file := design.Schematic
		writes = append(writes, importedProjectWrite{path: path, write: func(f *os.File) error { return schematic.Write(f, *file) }})
		artifacts = append(artifacts, reports.Artifact{Kind: reports.ArtifactSchematic, Path: filepath.ToSlash(path)})
	}
	if pcbDirty {
		if design.PCB == nil {
			return nil, fmt.Errorf("PCB required")
		}
		normalizeImportedPCB(design.PCB)
		path := filepath.Join(root, base+".kicad_pcb")
		file := design.PCB
		writes = append(writes, importedProjectWrite{path: path, write: func(f *os.File) error { return pcb.Write(f, *file) }})
		artifacts = append(artifacts, reports.Artifact{Kind: reports.ArtifactPCB, Path: filepath.ToSlash(path)})
	}
	if err := writeImportedProjectFilesAtomic(writes); err != nil {
		return nil, err
	}
	return artifacts, nil
}

type importedProjectWrite struct {
	path  string
	write func(*os.File) error
}

type stagedImportedProjectWrite struct {
	target      string
	temp        string
	backup      string
	hadOriginal bool
}

func writeImportedProjectFilesAtomic(writes []importedProjectWrite) error {
	if len(writes) == 0 {
		return nil
	}
	staged := make([]stagedImportedProjectWrite, 0, len(writes))
	cleanup := true
	defer func() {
		if cleanup {
			for _, file := range staged {
				_ = os.Remove(file.temp)
			}
		}
	}()
	for _, write := range writes {
		temp, err := renderImportedProjectTempFile(write.path, write.write)
		if err != nil {
			return err
		}
		staged = append(staged, stagedImportedProjectWrite{target: write.path, temp: temp})
	}
	for i := range staged {
		if err := backupImportedProjectTarget(&staged[i]); err != nil {
			restoreImportedProjectBackups(staged[:i])
			return err
		}
	}
	for i, file := range staged {
		if err := os.Rename(file.temp, file.target); err != nil {
			rollbackImportedProjectWrites(staged[:i])
			restoreImportedProjectBackups(staged[i:])
			return err
		}
	}
	if err := syncImportedProjectDirs(staged); err != nil {
		rollbackImportedProjectWrites(staged)
		return err
	}
	for _, file := range staged {
		if file.backup != "" {
			_ = os.Remove(file.backup)
		}
	}
	cleanup = false
	return nil
}

func backupImportedProjectTarget(file *stagedImportedProjectWrite) error {
	if _, err := os.Stat(file.target); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file.target), "."+filepath.Base(file.target)+".backup-*")
	if err != nil {
		return err
	}
	backupPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(backupPath)
		return err
	}
	if err := os.Rename(file.target, backupPath); err != nil {
		_ = os.Remove(backupPath)
		return err
	}
	file.backup = backupPath
	file.hadOriginal = true
	return nil
}

func rollbackImportedProjectWrites(files []stagedImportedProjectWrite) {
	for i := len(files) - 1; i >= 0; i-- {
		file := files[i]
		if file.hadOriginal {
			_ = os.Rename(file.backup, file.target)
		} else {
			_ = os.Remove(file.target)
		}
	}
}

func restoreImportedProjectBackups(files []stagedImportedProjectWrite) {
	for i := len(files) - 1; i >= 0; i-- {
		file := files[i]
		if file.hadOriginal {
			_ = os.Rename(file.backup, file.target)
		}
	}
}

func syncImportedProjectDirs(files []stagedImportedProjectWrite) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	seen := map[string]struct{}{}
	for _, file := range files {
		dir := filepath.Dir(file.target)
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		handle, err := os.Open(dir)
		if err != nil {
			return err
		}
		if err := handle.Sync(); err != nil {
			_ = handle.Close()
			return err
		}
		if err := handle.Close(); err != nil {
			return err
		}
	}
	return nil
}

func renderImportedProjectTempFile(path string, write func(*os.File) error) (string, error) {
	dir := filepath.Dir(path)
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return "", err
	}
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
	}()
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := write(tmp); err != nil {
		return "", err
	}
	if err := tmp.Chmod(mode); err != nil {
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	closed = true
	cleanup = false
	return tmpPath, nil
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
	if _, err := fmt.Fprintf(file, "pid=%d\n", os.Getpid()); err != nil {
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

func schematicPropertiesFromPayload(properties []SymbolProperty, defaultPosition kicadfiles.Point, defaultRotation kicadfiles.Angle, visibleBaseIndex int) []schematic.Property {
	if len(properties) == 0 {
		return nil
	}
	out := make([]schematic.Property, 0, len(properties))
	visibleIndex := 0
	for _, property := range properties {
		position := defaultPosition
		if property.At != nil {
			position = point(property.At.XMM, property.At.YMM)
		} else if !property.Hidden {
			position = visiblePropertyDefaultPosition(defaultPosition, visibleBaseIndex+visibleIndex)
			visibleIndex++
		}
		rotation := defaultRotation
		if property.Rotation != nil {
			rotation = kicadfiles.Angle(*property.Rotation)
		}
		out = append(out, schematic.Property{
			Name:           strings.TrimSpace(property.Name),
			Value:          property.Value,
			Private:        property.Private,
			Hidden:         property.Hidden,
			ShowName:       schematic.CloneBool(property.ShowName),
			DoNotAutoplace: schematic.CloneBool(property.DoNotAutoplace),
			Position:       position,
			Rotation:       rotation,
		})
	}
	return out
}

func visiblePropertyDefaultPosition(symbolPosition kicadfiles.Point, index int) kicadfiles.Point {
	return kicadfiles.Point{
		X: symbolPosition.X,
		Y: symbolPosition.Y + kicadfiles.MM(2.54*float64(index+1)),
	}
}

func validateSymbolPropertyPayload(properties []SymbolProperty) error {
	issues := validateSymbolProperties("properties", properties)
	for _, issue := range issues {
		if issue.Blocking() {
			return fmt.Errorf("%s: %s", issue.Path, issue.Message)
		}
	}
	return nil
}

func point(xMM, yMM float64) kicadfiles.Point {
	return kicadfiles.Point{X: kicadfiles.MM(xMM), Y: kicadfiles.MM(yMM)}
}

func addPoints(a kicadfiles.Point, b kicadfiles.Point) kicadfiles.Point {
	return kicadfiles.Point{X: a.X + b.X, Y: a.Y + b.Y}
}

func symbolPinAnchor(schematicFile *schematic.SchematicFile, ref string, pin string) (kicadfiles.Point, error) {
	if schematicFile == nil {
		return kicadfiles.Point{}, fmt.Errorf("root schematic required")
	}
	ref = strings.TrimSpace(ref)
	pin = strings.TrimSpace(pin)
	for _, symbol := range schematicFile.Symbols {
		if symbol.Reference != ref {
			continue
		}
		if len(symbol.Pins) != len(symbol.PinAnchors) {
			return kicadfiles.Point{}, fmt.Errorf("symbol %s has mismatched pins and pin anchors", ref)
		}
		for index, symbolPin := range symbol.Pins {
			if symbolPin.Number == pin {
				return symbol.PinAnchors[index], nil
			}
		}
		return kicadfiles.Point{}, fmt.Errorf("symbol %s has no pin %s", ref, pin)
	}
	return kicadfiles.Point{}, fmt.Errorf("symbol %s not found", ref)
}

func schematicHasNoConnect(noConnects []schematic.NoConnect, position kicadfiles.Point) bool {
	for _, noConnect := range noConnects {
		if noConnect.Position == position {
			return true
		}
	}
	return false
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
