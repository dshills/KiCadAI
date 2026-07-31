package opentopologysynthesis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kicadai/internal/designworkflow"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/writercorrectness"
)

// PromoteSynthesisRun executes the normal project-generation path twice from
// the exact simulation-selected graph. It fails closed unless both clean-root
// runs pass routing, connectivity, writer, installed-KiCad ERC/strict DRC,
// zero-difference round trip, and raw project replay.
func PromoteSynthesisRun(
	ctx context.Context,
	run SynthesisRun,
	environment SimulationEnvironment,
	opts PhysicalPromotionOptions,
) PhysicalPromotionResult {
	result := PhysicalPromotionResult{
		Schema:        PhysicalPromotionSchema,
		Version:       PhysicalPromotionVersion,
		PolicyVersion: PolicyVersion,
		SynthesisHash: run.Hash,
		Status:        PhysicalPromotionInvalid,
		Runs:          []PhysicalPromotionRun{},
		Issues:        []reports.Issue{},
	}
	if run.Report.Status != StatusPassed ||
		run.Report.Selected == nil ||
		run.Physical == nil ||
		run.Physical.Status != PhysicalLoweringReady ||
		run.Physical.Hash == "" ||
		run.SelectedGraph == nil {
		result.Issues = []reports.Issue{physicalPromotionIssue(
			"run",
			"physical promotion requires a fully passing, physically lowered synthesis run",
			"run bounded topology and simulation synthesis before promotion",
		)}
		return finalizePhysicalPromotion(result)
	}
	result.RequirementHash = run.Report.RequirementHash
	result.InventoryHash = run.Report.PrimitiveInventoryHash
	result.PhysicalHash = run.Physical.Hash
	if strings.TrimSpace(opts.OutputRoot) == "" {
		result.Issues = []reports.Issue{physicalPromotionIssue(
			"options.output_root",
			"physical promotion output root is required",
			"provide a clean writable output root",
		)}
		return finalizePhysicalPromotion(result)
	}
	if strings.TrimSpace(opts.KiCadCLI) == "" {
		result.Issues = []reports.Issue{physicalPromotionIssue(
			"options.kicad_cli",
			"installed KiCad CLI path is required",
			"configure the installed kicad-cli executable",
		)}
		return finalizePhysicalPromotion(result)
	}
	if opts.LibraryIndex == nil {
		result.Issues = []reports.Issue{physicalPromotionIssue(
			"options.library_index",
			"resolved symbol and footprint library index is required",
			"load the configured KiCad library roots",
		)}
		return finalizePhysicalPromotion(result)
	}
	if environment.Catalog == nil ||
		environment.CatalogHash != run.Report.CatalogHash {
		result.Issues = []reports.Issue{physicalPromotionIssue(
			"environment.catalog",
			"physical promotion catalog does not match the synthesis run",
			"reuse the immutable catalog bound during synthesis",
		)}
		return finalizePhysicalPromotion(result)
	}
	if err := preparePhysicalPromotionRoot(opts.OutputRoot); err != nil {
		result.Issues = []reports.Issue{physicalPromotionIssue(
			"options.output_root",
			"prepare physical promotion output root: "+err.Error(),
			"provide a writable directory path",
		)}
		return finalizePhysicalPromotion(result)
	}

	request := run.Physical.DesignRequest
	request.Validation.Acceptance =
		designworkflow.AcceptanceFabricationCandidate
	request.Validation.RequireERC = true
	request.Validation.RequireDRC = true
	request.Validation.StrictUnrouted = true
	request.Validation.StrictZones = true
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	for number := 1; number <= 2; number++ {
		if err := ctx.Err(); err != nil {
			result.Status = PhysicalPromotionFailed
			result.Issues = []reports.Issue{physicalPromotionIssue(
				"context",
				"physical promotion canceled: "+err.Error(),
				"retry with an active context",
			)}
			return finalizePhysicalPromotion(result)
		}
		output := filepath.Join(opts.OutputRoot, fmt.Sprintf("run-%d", number))
		workflow := designworkflow.Create(
			ctx,
			request,
			physicalPromotionCreateOptions(
				output,
				run.Report.RequirementHash,
				opts.KiCadCLI,
				environment,
				*opts.LibraryIndex,
				timeout,
				opts.KeepArtifacts,
				opts.Overwrite,
			),
		)
		promotionRun := PhysicalPromotionRun{
			Number:      number,
			ProjectRoot: output,
			Workflow:    workflow,
			Artifacts:   designworkflow.WorkflowArtifacts(workflow),
		}
		stageIssues := physicalPromotionWorkflowIssues(workflow)
		if len(stageIssues) == 0 {
			projectHash, err := physicalPromotionProjectHash(output)
			if err != nil {
				stageIssues = append(
					stageIssues,
					physicalPromotionIssue(
						fmt.Sprintf("runs.%d.project", number-1),
						"hash generated KiCad project: "+err.Error(),
						"inspect the project-write artifacts",
					),
				)
			} else {
				promotionRun.ProjectHash = projectHash
			}
		}
		result.Runs = append(result.Runs, promotionRun)
		if len(stageIssues) != 0 {
			result.Status = PhysicalPromotionFailed
			result.Issues = reports.SortedIssues(stageIssues)
			return finalizePhysicalPromotion(result)
		}
	}
	result.ReplayIdentical =
		result.Runs[0].ProjectHash != "" &&
			result.Runs[0].ProjectHash == result.Runs[1].ProjectHash
	if !result.ReplayIdentical {
		result.Status = PhysicalPromotionFailed
		result.Issues = []reports.Issue{physicalPromotionIssue(
			"replay",
			"clean-root generated KiCad projects are not byte-identical",
			"inspect deterministic generation inputs and writer output",
		)}
		return finalizePhysicalPromotion(result)
	}
	result.ProjectHash = result.Runs[0].ProjectHash
	result.Status = PhysicalPromotionPassed
	return finalizePhysicalPromotion(result)
}

func preparePhysicalPromotionRoot(root string) error {
	return os.MkdirAll(filepath.Clean(root), 0o755)
}

func physicalPromotionCreateOptions(
	output string,
	seed string,
	kicadCLI string,
	environment SimulationEnvironment,
	index libraryresolver.LibraryIndex,
	timeout time.Duration,
	keepArtifacts bool,
	overwrite bool,
) designworkflow.CreateOptions {
	return designworkflow.CreateOptions{
		OutputDir: output,
		Overwrite: overwrite,
		Seed:      "open-topology-" + seed,
		Components: designworkflow.ComponentSelectionOptions{
			Catalog: environment.Catalog,
		},
		LibraryIndex: &index,
		Routing: designworkflow.RoutingOptions{
			Mode:         routing.ModeTwoLayer,
			GridMM:       0.25,
			TraceWidthMM: 0.2,
			ClearanceMM:  0.1,
		},
		Validation: designworkflow.ValidationOptions{
			StrictZones:    true,
			StrictUnrouted: true,
			RequireDRC:     true,
			KiCadCLI:       kicadCLI,
			KeepArtifacts:  keepArtifacts,
			ArtifactDir: filepath.Join(
				output,
				".evidence",
				"validation",
			),
		},
		KiCadChecks: designworkflow.KiCadCheckOptions{
			KiCadCLI:      kicadCLI,
			Timeout:       timeout,
			RequireERC:    true,
			RequireDRC:    true,
			KeepArtifacts: keepArtifacts,
			ArtifactDir: filepath.Join(
				output,
				".evidence",
				"kicad",
			),
		},
		Writer: writercorrectness.Options{
			KiCadCLI:              kicadCLI,
			RequireKiCadRoundTrip: true,
			StrictDiffs:           true,
			KeepArtifacts:         keepArtifacts,
			ArtifactDir: filepath.Join(
				output,
				".evidence",
				"writer",
			),
			LibraryIndex:    index,
			HasLibraryIndex: true,
		},
	}
}

func physicalPromotionWorkflowIssues(
	workflow designworkflow.WorkflowResult,
) []reports.Issue {
	issues := []reports.Issue{}
	for _, issue := range designworkflow.WorkflowIssues(workflow) {
		if issue.Blocking() {
			issues = append(issues, issue)
		}
	}
	required := []designworkflow.StageName{
		designworkflow.StageSchematic,
		designworkflow.StageSchematicElectrical,
		designworkflow.StagePlacement,
		designworkflow.StageRouting,
		designworkflow.StageProjectWrite,
		designworkflow.StageWriterCorrect,
		designworkflow.StageValidation,
		designworkflow.StageKiCadChecks,
	}
	for _, name := range required {
		status := designworkflow.StageStatusSkipped
		for _, stage := range workflow.Stages {
			if stage.Name == name {
				status = stage.Status
				break
			}
		}
		if status == designworkflow.StageStatusOK {
			continue
		}
		issues = append(issues, physicalPromotionIssue(
			"workflow."+string(name),
			fmt.Sprintf(
				"required production stage %s completed with status %s",
				name,
				status,
			),
			"inspect the stage evidence and correct the generated design",
		))
	}
	return reports.SortedIssues(issues)
}

func physicalPromotionProjectHash(root string) (string, error) {
	paths := []string{}
	err := filepath.WalkDir(
		root,
		func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == ".evidence" {
					return filepath.SkipDir
				}
				return nil
			}
			switch filepath.Ext(path) {
			case ".kicad_pro", ".kicad_sch", ".kicad_pcb":
				paths = append(paths, path)
			}
			return nil
		},
	)
	if err != nil {
		return "", err
	}
	if len(paths) < 3 {
		return "", fmt.Errorf(
			"generated project files = %d, want at least 3",
			len(paths),
		)
	}
	slices.Sort(paths)
	sum := sha256.New()
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		_, _ = sum.Write([]byte(filepath.ToSlash(relative)))
		_, _ = sum.Write([]byte{0})
		_, copyErr := io.Copy(sum, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		_, _ = sum.Write([]byte{0})
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func physicalPromotionIssue(
	path string,
	message string,
	suggestion string,
) reports.Issue {
	return reports.Issue{
		Code:       CodePhysicalPromotionFailed,
		Severity:   reports.SeverityBlocked,
		Stage:      "open_topology_physical_promotion",
		Path:       path,
		Message:    message,
		Suggestion: suggestion,
	}
}

func finalizePhysicalPromotion(
	result PhysicalPromotionResult,
) PhysicalPromotionResult {
	copy := result
	copy.Hash = ""
	result.Hash = hashJSON(copy)
	return result
}
