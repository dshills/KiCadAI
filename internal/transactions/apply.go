package transactions

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/designapi"
	"kicadai/internal/manifest"
	"kicadai/internal/reports"
)

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
		return nil, fmt.Errorf("set_board_outline apply is not implemented yet")
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
			pads = append(pads, designapi.PadSpec{Name: pad.Name, Type: pad.Type, Net: net})
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

func boardLayer(value string) kicadfiles.BoardLayer {
	switch strings.TrimSpace(value) {
	case "", "F.Cu":
		return kicadfiles.LayerFCu
	case "B.Cu":
		return kicadfiles.LayerBCu
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
