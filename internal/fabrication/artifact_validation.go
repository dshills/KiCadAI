package fabrication

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/kicadfiles"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/reports"
)

type FabricationArtifactValidation struct {
	Gerber EvidenceStatus  `json:"gerber"`
	Drill  EvidenceStatus  `json:"drill"`
	Issues []reports.Issue `json:"issues,omitempty"`
}

func ValidateFabricationArtifacts(ctx context.Context, request PlotRequest) FabricationArtifactValidation {
	if err := ctx.Err(); err != nil {
		issue := reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityError, Path: "fabrication/artifacts", Message: err.Error()}
		return FabricationArtifactValidation{Gerber: EvidenceFail, Drill: EvidenceFail, Issues: []reports.Issue{issue}}
	}
	board, err := pcbfiles.ReadFile(request.PCBPath)
	if err != nil {
		issue := reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "fabrication/pcb", Message: err.Error()}
		return FabricationArtifactValidation{Gerber: EvidenceFail, Drill: EvidenceFail, Issues: []reports.Issue{issue}}
	}
	var issues []reports.Issue
	gerber := validateGerberFiles(ctx, request.GerberDir, board, &issues)
	drill := validateDrillFiles(ctx, request.DrillDir, board, &issues)
	issues = dedupeIssues(issues)
	slices.SortFunc(issues, compareIssues)
	return FabricationArtifactValidation{Gerber: gerber, Drill: drill, Issues: issues}
}

func validateGerberFiles(ctx context.Context, dir string, board pcbfiles.PCBFile, issues *[]reports.Issue) EvidenceStatus {
	files, err := scanNonEmptyFiles(ctx, dir)
	if os.IsNotExist(err) {
		*issues = append(*issues, missingArtifactIssue("fabrication/gerbers", "Gerber output directory is missing"))
		return EvidenceMissing
	}
	if err != nil {
		*issues = append(*issues, failedArtifactIssue("fabrication/gerbers", err.Error()))
		return EvidenceFail
	}
	required := requiredGerberTokens(board)
	status := EvidencePass
	for _, token := range required {
		if hasLayerFile(files, token) {
			continue
		}
		status = EvidenceFail
		path := "fabrication/gerbers/" + token
		message := "required Gerber output is missing for " + token
		if token == "Edge_Cuts" {
			message = "Edge.Cuts Gerber output is missing"
		}
		*issues = append(*issues, failedArtifactIssue(path, message))
	}
	if len(files.Empty) > 0 {
		status = EvidenceFail
		for _, path := range files.Empty {
			*issues = append(*issues, failedArtifactIssue(path, "fabrication artifact file is empty"))
		}
	}
	if len(required) == 0 && len(files.NonEmpty) == 0 {
		*issues = append(*issues, missingArtifactIssue("fabrication/gerbers", "Gerber files have not been generated"))
		return EvidenceMissing
	}
	return status
}

func validateDrillFiles(ctx context.Context, dir string, board pcbfiles.PCBFile, issues *[]reports.Issue) EvidenceStatus {
	needsDrill := boardHasDrilledFeatures(board)
	files, err := scanNonEmptyFiles(ctx, dir)
	if os.IsNotExist(err) {
		if !needsDrill {
			return EvidenceSkipped
		}
		*issues = append(*issues, missingArtifactIssue("fabrication/drill", "drill output directory is missing"))
		return EvidenceMissing
	}
	if err != nil {
		*issues = append(*issues, failedArtifactIssue("fabrication/drill", err.Error()))
		return EvidenceFail
	}
	status := EvidencePass
	if len(files.Empty) > 0 {
		status = EvidenceFail
		for _, path := range files.Empty {
			*issues = append(*issues, failedArtifactIssue(path, "fabrication artifact file is empty"))
		}
	}
	if !needsDrill {
		return EvidenceSkipped
	}
	if !hasAnyDrillFile(files) {
		*issues = append(*issues, failedArtifactIssue("fabrication/drill", "drill files have not been generated"))
		return EvidenceFail
	}
	return status
}

type scannedFiles struct {
	NonEmpty []string
	Empty    []string
}

func scanNonEmptyFiles(ctx context.Context, dir string) (scannedFiles, error) {
	if err := ctx.Err(); err != nil {
		return scannedFiles{}, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return scannedFiles{}, err
	}
	if !info.IsDir() {
		return scannedFiles{}, os.ErrInvalid
	}
	var files scannedFiles
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		cleaned := filepath.ToSlash(path)
		if info.Size() == 0 {
			files.Empty = append(files.Empty, cleaned)
			return nil
		}
		files.NonEmpty = append(files.NonEmpty, cleaned)
		return nil
	})
	slices.Sort(files.NonEmpty)
	slices.Sort(files.Empty)
	return files, err
}

func requiredGerberTokens(board pcbfiles.PCBFile) []string {
	var tokens []string
	for _, layer := range board.Layers {
		name := string(layer.Name)
		switch {
		case strings.HasSuffix(name, ".Cu"):
			tokens = append(tokens, normalizeFabricationLayerToken(name))
		case name == string(kicadfiles.LayerFMask), name == string(kicadfiles.LayerBMask), name == string(kicadfiles.LayerFSilkS), name == string(kicadfiles.LayerBSilkS):
			tokens = append(tokens, normalizeFabricationLayerToken(name))
		}
	}
	if boardHasEdgeCuts(board) {
		tokens = append(tokens, "Edge_Cuts")
	}
	slices.Sort(tokens)
	return tokens
}

func boardHasEdgeCuts(board pcbfiles.PCBFile) bool {
	for _, drawing := range board.Drawings {
		if drawing.Layer == kicadfiles.LayerEdge {
			return true
		}
	}
	for _, footprint := range board.Footprints {
		for _, graphic := range footprint.Graphics {
			if graphic.Layer == kicadfiles.LayerEdge {
				return true
			}
		}
	}
	return false
}

func boardHasDrilledFeatures(board pcbfiles.PCBFile) bool {
	for _, via := range board.Vias {
		if via.Drill > 0 {
			return true
		}
	}
	for _, footprint := range board.Footprints {
		for _, pad := range footprint.Pads {
			if pad.Drill > 0 || strings.EqualFold(pad.Type, "thru_hole") || strings.EqualFold(pad.Type, "np_thru_hole") {
				return true
			}
		}
	}
	return false
}

func hasLayerFile(files scannedFiles, token string) bool {
	tokenLower := canonicalFabricationLayerToken(strings.ToLower(token))
	for _, path := range files.NonEmpty {
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		normalized := canonicalFabricationLayerToken(strings.ToLower(normalizeFabricationLayerToken(base)))
		if normalized == tokenLower || strings.HasSuffix(normalized, "_"+tokenLower) {
			return true
		}
	}
	return false
}

func canonicalFabricationLayerToken(token string) string {
	token = strings.ReplaceAll(token, "f_silkscreen", "f_silks")
	token = strings.ReplaceAll(token, "b_silkscreen", "b_silks")
	return token
}

func hasAnyDrillFile(files scannedFiles) bool {
	for _, path := range files.NonEmpty {
		if isDrillArtifactPath(path) {
			return true
		}
	}
	for _, path := range files.Empty {
		if isDrillArtifactPath(path) {
			return true
		}
	}
	return false
}

func isDrillArtifactPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".drl", ".gbr", ".xln", ".txt":
		return true
	default:
		return false
	}
}

func normalizeFabricationLayerToken(value string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '.', '-', ' ':
			return '_'
		default:
			return r
		}
	}, value)
}

func missingArtifactIssue(path string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: path, Message: message}
}

func failedArtifactIssue(path string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: path, Message: message}
}
