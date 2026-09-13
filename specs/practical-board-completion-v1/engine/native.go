package main

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kicadai/internal/designworkflow"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/writercorrectness"
)

const nativeCLI = "/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli"

var requiredStages = []designworkflow.StageName{
	designworkflow.StageSchematic, designworkflow.StageSchematicElectrical,
	designworkflow.StagePlacement, designworkflow.StageRouting,
	designworkflow.StageProjectWrite, designworkflow.StageWriterCorrect,
	designworkflow.StageValidation, designworkflow.StageKiCadChecks,
	designworkflow.StageSimulation,
}

func firstFailedStage(result designworkflow.WorkflowResult) string {
	for _, name := range requiredStages {
		count := 0
		for _, stage := range result.Stages {
			if stage.Name != name {
				continue
			}
			count++
			if stage.Status != designworkflow.StageStatusOK || reports.HasBlockingIssue(stage.Issues) {
				return string(name)
			}
		}
		if count != 1 {
			return string(name)
		}
	}
	if result.Feedback.Summary.BlockingCount != 0 || result.Feedback.Summary.ErrorCount != 0 {
		return "workflow_feedback"
	}
	return ""
}

func writeGzip(dir, name string, value any) error {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w := gzip.NewWriter(f)
	encodeErr := json.NewEncoder(w).Encode(value)
	return errors.Join(encodeErr, w.Close(), f.Close())
}

// Capture every native project file (including hierarchical sheets) before any
// rendering. Previews must later be made from separately inventoried copies.
func projectIdentity(root string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in project tree")
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("nonregular project file")
		}
		suffix := filepath.Ext(path)
		if suffix != ".kicad_sch" && suffix != ".kicad_pcb" && suffix != ".kicad_pro" && suffix != ".kicad_dru" {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, f)
		if err := errors.Join(copyErr, f.Close()); err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	return files, err
}

func nativeWorkflow(ctx context.Context, output string, request designworkflow.Request) (string, error) {
	if request.Name == "" || filepath.Base(request.Name) != request.Name || strings.ContainsAny(request.Name, `/\`) {
		return "project_identity", fmt.Errorf("unsafe project name")
	}
	if info, err := os.Stat(nativeCLI); err != nil || !info.Mode().IsRegular() {
		return "native_environment", fmt.Errorf("pinned installed KiCad executable unavailable")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if err := writeNew(output, "library-root-issues.json", rootIssues); err != nil {
		return "native_environment", err
	}
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" || reports.HasBlockingIssue(rootIssues) {
		return "native_environment", fmt.Errorf("installed library roots unavailable")
	}
	index, issues := libraryresolver.Load(ctx, roots, libraryresolver.LoadOptions{})
	if err := writeNew(output, "library-load-issues.json", issues); err != nil {
		return "native_environment", err
	}
	// Retain unrelated installed-library diagnostics. Selected references must
	// still pass the resolver, writer, native checks, and full result gates.
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		return "native_environment", fmt.Errorf("empty installed library index")
	}
	if err := writeGzip(output, "library-index.json.gz", index); err != nil {
		return "native_environment", err
	}
	index.GeneratedAt = time.Time{}
	indexBytes, err := json.Marshal(index)
	if err != nil {
		return "native_environment", err
	}
	if err := writeNew(output, "library-identity.json", map[string]string{"sha256": digest(indexBytes), "normalization": "only LibraryIndex.GeneratedAt collection time"}); err != nil {
		return "native_environment", err
	}
	project := filepath.Join(output, "project")
	result := designworkflow.Create(ctx, request, designworkflow.CreateOptions{
		OutputDir: project, LibraryIndex: &index,
		Validation:  designworkflow.ValidationOptions{StrictUnrouted: true, RequireDRC: true, KiCadCLI: nativeCLI, KeepArtifacts: true, ArtifactDir: filepath.Join(project, ".kicadai", "validation")},
		KiCadChecks: designworkflow.KiCadCheckOptions{KiCadCLI: nativeCLI, RequireERC: true, RequireDRC: true, EnforceRequirements: true, KeepArtifacts: true, ArtifactDir: filepath.Join(project, ".kicadai", "checks")},
		Writer:      writercorrectness.Options{RequireKiCadRoundTrip: true, StrictDiffs: true, KiCadCLI: nativeCLI, KeepArtifacts: true, ArtifactDir: filepath.Join(project, ".kicadai", "roundtrip"), LibraryIndex: index, HasLibraryIndex: true, LibraryResolutionUsed: true},
	})
	if err := writeNew(output, "workflow.json", result); err != nil {
		return "physical_workflow", err
	}
	if gate := firstFailedStage(result); gate != "" {
		return gate, nil
	}
	for _, suffix := range []string{".kicad_sch", ".kicad_pcb", ".kicad_pro"} {
		if info, err := os.Lstat(filepath.Join(project, request.Name+suffix)); err != nil || !info.Mode().IsRegular() {
			return "project_identity", fmt.Errorf("missing/nonregular native project file %s", suffix)
		}
	}
	files, err := projectIdentity(project)
	if err != nil {
		return "project_identity", err
	}
	return "", writeNew(output, "project-identity.json", files)
}
