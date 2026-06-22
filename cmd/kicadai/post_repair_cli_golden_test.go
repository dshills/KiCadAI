package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/repair"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const postRepairCLIFixtureRoot = "../../internal/designworkflow/testdata/post_repair_cli"

type postRepairCLIResult struct {
	OK        bool               `json:"ok"`
	Command   string             `json:"command"`
	Data      postRepairCLIData  `json:"data"`
	Issues    []reports.Issue    `json:"issues,omitempty"`
	Artifacts []reports.Artifact `json:"artifacts,omitempty"`
}

type postRepairCLIData struct {
	Project struct {
		OutputDir string `json:"output_dir"`
	} `json:"project,omitempty"`
	Stages     []postRepairCLIStage      `json:"stages,omitempty"`
	Status     repair.Status             `json:"status,omitempty"`
	Summary    map[string]any            `json:"summary,omitempty"`
	Delta      map[string]any            `json:"delta,omitempty"`
	Validation []postRepairCLIValidation `json:"validation,omitempty"`
}

type postRepairCLIValidation struct {
	Name      string             `json:"name"`
	Skipped   bool               `json:"skipped,omitempty"`
	Issues    []reports.Issue    `json:"issues,omitempty"`
	Artifacts []reports.Artifact `json:"artifacts,omitempty"`
}

type postRepairCLIStage struct {
	Name      string             `json:"name"`
	Status    string             `json:"status"`
	Summary   map[string]any     `json:"summary,omitempty"`
	Issues    []reports.Issue    `json:"issues,omitempty"`
	Artifacts []reports.Artifact `json:"artifacts,omitempty"`
}

type postRepairCLIFixture struct {
	Name    string `json:"name"`
	Request string `json:"request,omitempty"`
	Bundle  string `json:"bundle,omitempty"`
	Intent  string `json:"intent,omitempty"`
}

func loadPostRepairCLIFixture(t *testing.T, name string) postRepairCLIFixture {
	t.Helper()
	if err := validatePostRepairCLIFixtureName(name); err != nil {
		t.Fatalf("invalid post-repair CLI fixture name %q: %v", name, err)
	}
	path := postRepairCLIFixtureFilePath(t, name, "metadata.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read post-repair CLI fixture metadata %q: %v", path, err)
	}
	var fixture postRepairCLIFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode post-repair CLI fixture metadata %q: %v", path, err)
	}
	if fixture.Name != name {
		t.Fatalf("fixture name = %q, want %q", fixture.Name, name)
	}
	return fixture
}

func postRepairCLIFixtureFilePath(t *testing.T, fixtureName string, fileName string) string {
	t.Helper()
	if err := validatePostRepairCLIFixtureName(fixtureName); err != nil {
		t.Fatalf("invalid post-repair CLI fixture name %q: %v", fixtureName, err)
	}
	if strings.TrimSpace(fileName) == "" {
		t.Fatalf("fixture %q has empty file path", fixtureName)
	}
	if filepath.IsAbs(fileName) {
		t.Fatalf("fixture %q file path must be relative: %q", fixtureName, fileName)
	}
	base := filepath.Join(postRepairCLIFixtureRoot, fixtureName)
	path := filepath.Join(base, filepath.Clean(fileName))
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("fixture %q file path escapes fixture directory: %q", fixtureName, fileName)
	}
	return path
}

func validatePostRepairCLIFixtureName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("empty fixture name")
	}
	// filepath.Base(".") and filepath.Base("..") return the original value,
	// so reject them explicitly before using the name as a fixture directory.
	if name == "." || name == ".." {
		return fmt.Errorf("must not be %q", name)
	}
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("must be a single fixture directory name")
	}
	return nil
}

func runPostRepairDesignCreateCLI(t *testing.T, fixtureName string, globalArgs ...string) (postRepairCLIResult, string) {
	t.Helper()
	fixture := loadPostRepairCLIFixture(t, fixtureName)
	if strings.TrimSpace(fixture.Request) == "" {
		t.Fatalf("fixture %q missing request", fixtureName)
	}
	outputDir := filepath.Join(t.TempDir(), fixtureName)
	requestPath := postRepairCLIFixtureFilePath(t, fixtureName, fixture.Request)
	args := []string{
		"--json",
		"--request", requestPath,
		"--output", outputDir,
		"--overwrite",
	}
	args = append(args, globalArgs...)
	args = append(args, "design", "create")
	return runPostRepairCLI(t, args...), outputDir
}

func runPostRepairTargetApplyCLI(t *testing.T, target string, bundlePath string, globalArgs ...string) postRepairCLIResult {
	t.Helper()
	// This CLI uses one global flag set, so all flags must precede "repair apply".
	args := []string{
		"--json",
		"--execute",
		"--overwrite",
		"--target", target,
		"--request", bundlePath,
	}
	args = append(args, globalArgs...)
	args = append(args, "repair", "apply")
	return runPostRepairCLI(t, args...)
}

func runPostRepairCLI(t *testing.T, args ...string) postRepairCLIResult {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run(args, &stdout, &stderr)
	var result postRepairCLIResult
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(stdout.Bytes())))
	if decodeErr := decoder.Decode(&result); decodeErr != nil {
		t.Fatalf("decode CLI JSON: %v\nerr=%v\nstdout=%s\nstderr=%s", decodeErr, err, stdout.String(), stderr.String())
	}
	var trailing json.RawMessage
	if decodeErr := decoder.Decode(&trailing); decodeErr != io.EOF {
		t.Fatalf("CLI JSON has trailing stdout: %v\nerr=%v\nstdout=%s\nstderr=%s", decodeErr, err, stdout.String(), stderr.String())
	}
	if err != nil && len(result.Issues) == 0 {
		t.Fatalf("CLI returned error without JSON issues: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	return result
}

func postRepairStageByName(t *testing.T, stages []postRepairCLIStage, name string) postRepairCLIStage {
	t.Helper()
	for _, stage := range stages {
		if stage.Name == name {
			return stage
		}
	}
	names := make([]string, 0, len(stages))
	for _, stage := range stages {
		names = append(names, stage.Name)
	}
	t.Fatalf("missing stage %q; got stages %v", name, names)
	return postRepairCLIStage{}
}

func postRepairSummaryMap(t *testing.T, summary map[string]any, key string) map[string]any {
	t.Helper()
	if summary == nil {
		t.Fatalf("summary missing; need key %q", key)
	}
	value, ok := summary[key]
	if !ok {
		t.Fatalf("summary missing %q: %#v", key, summary)
	}
	typed, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("summary[%s] has type %T: %#v", key, value, value)
	}
	return typed
}

func postRepairSummaryNumber(t *testing.T, summary map[string]any, key string) float64 {
	t.Helper()
	if summary == nil {
		t.Fatalf("summary missing; need key %q", key)
	}
	value, ok := summary[key]
	if !ok {
		t.Fatalf("summary missing %q: %#v", key, summary)
	}
	number, ok := value.(float64)
	if !ok {
		t.Fatalf("summary[%s] has type %T: %#v", key, value, value)
	}
	return number
}

func postRepairBundleArtifact(t *testing.T, outputDir string, artifacts []reports.Artifact) reports.Artifact {
	t.Helper()
	for _, artifact := range artifacts {
		if normalizePostRepairPath(outputDir, artifact.Path) == ".kicadai/repair-bundle.json" {
			assertPostRepairPathInside(t, outputDir, artifact.Path)
			return artifact
		}
	}
	t.Fatalf("missing repair bundle artifact under %q: %#v", outputDir, artifacts)
	return reports.Artifact{}
}

func writePostRepairCleanBundle(t *testing.T, dir string, outputDir string) string {
	t.Helper()
	tx := mustPostRepairTransaction(t, `{"operations":[
	  {"op":"create_project","name":"post_repair_clean_apply"},
	  {"op":"set_board_outline","board":{"width_mm":30,"height_mm":20}},
	  {"op":"write_project","overwrite":true}
	]}`)
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir, Overwrite: true})
	if len(apply.Issues) != 0 {
		t.Fatalf("materialize clean repair target issues: %#v", apply.Issues)
	}
	path := filepath.Join(dir, "clean-repair-bundle.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create clean repair bundle directory: %v", err)
	}
	if err := repair.SaveBundle(path, repair.Bundle{
		Schema:        repair.BundleSchemaV1,
		ProjectRoot:   outputDir,
		ProjectName:   "post_repair_clean_apply",
		Generated:     true,
		Transaction:   &tx,
		RepairOptions: repair.Options{Enabled: true, Apply: true},
	}); err != nil {
		t.Fatalf("write clean repair bundle: %v", err)
	}
	return path
}

func mustPostRepairTransaction(t *testing.T, input string) transactions.Transaction {
	t.Helper()
	tx, err := transactions.Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse transaction fixture: %v", err)
	}
	return tx
}

func postRepairStageExists(stages []postRepairCLIStage, name string) bool {
	for _, stage := range stages {
		if stage.Name == name {
			return true
		}
	}
	return false
}

func postRepairValidationByName(t *testing.T, validations []postRepairCLIValidation, name string) postRepairCLIValidation {
	t.Helper()
	for _, validation := range validations {
		if validation.Name == name {
			return validation
		}
	}
	names := make([]string, 0, len(validations))
	for _, validation := range validations {
		names = append(names, validation.Name)
	}
	t.Fatalf("missing validation %q; got validations %q", name, names)
	// t.Fatalf aborts the test; this zero value satisfies the compiler.
	return postRepairCLIValidation{}
}

func normalizePostRepairPath(root string, path string) string {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		if rel == "." {
			return "."
		}
		return filepath.ToSlash(rel)
	}
	if filepath.IsAbs(path) {
		return "<outside-root>"
	}
	return filepath.ToSlash(path)
}

func assertPostRepairPathInside(t *testing.T, root string, path string) {
	t.Helper()
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	// The root itself is not an artifact path inside the root.
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		t.Fatalf("path %q is not inside root %q", path, root)
	}
}

func TestPostRepairCLIGoldenHarnessFixtureLoader(t *testing.T) {
	fixture := loadPostRepairCLIFixture(t, "bundle_basic")
	if fixture.Request != "request.json" {
		t.Fatalf("fixture = %#v", fixture)
	}
}

func TestPostRepairCLIGoldenHarnessNormalizesPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	path := filepath.Join(root, ".kicadai", "repair-bundle.json")
	if got := normalizePostRepairPath(root, root); got != "." {
		t.Fatalf("normalized root path = %q", got)
	}
	if got := normalizePostRepairPath(root, path); got != ".kicadai/repair-bundle.json" {
		t.Fatalf("normalized path = %q", got)
	}
	if got := normalizePostRepairPath(root, filepath.Dir(root)); got != "<outside-root>" {
		t.Fatalf("normalized outside path = %q", got)
	}
	assertPostRepairPathInside(t, root, path)
}

func TestPostRepairDesignCreateEmitsGeneratedRepairBundle(t *testing.T) {
	result, outputDir := runPostRepairDesignCreateCLI(t, "bundle_basic", "--repair-apply", "--skip-routing")
	stage := postRepairStageByName(t, result.Data.Stages, "validation_repair")
	validationStage := postRepairStageByName(t, result.Data.Stages, "validation")
	kiCadStage := postRepairStageByName(t, result.Data.Stages, "kicad_checks")
	if validationStage.Status != "skipped" || kiCadStage.Status != "skipped" {
		t.Fatalf("downstream stages should be explicit skips: validation=%#v kicad=%#v", validationStage, kiCadStage)
	}
	if stage.Status == "" {
		t.Fatalf("validation_repair status is empty: %#v", stage)
	}
	bundleArtifact := postRepairBundleArtifact(t, outputDir, stage.Artifacts)
	if bundleArtifact.Kind != reports.ArtifactValidationReport {
		t.Fatalf("bundle artifact kind = %q", bundleArtifact.Kind)
	}
	bundle, err := repair.LoadBundle(bundleArtifact.Path)
	if err != nil {
		t.Fatalf("load generated repair bundle: %v", err)
	}
	if bundle.Schema != repair.BundleSchemaV1 || !bundle.Generated || bundle.ProjectName != "post_repair_bundle_basic" {
		t.Fatalf("bundle metadata = %#v", bundle)
	}
	if bundle.Transaction == nil || len(bundle.Transaction.Operations) == 0 {
		t.Fatalf("bundle transaction missing: %#v", bundle.Transaction)
	}
	if len(bundle.StageIssues) == 0 {
		t.Fatalf("bundle stage issues missing: %#v", bundle.StageIssues)
	}
	if !bundle.RepairOptions.Enabled || !bundle.RepairOptions.Apply {
		t.Fatalf("bundle repair options = %#v", bundle.RepairOptions)
	}
	if got := normalizePostRepairPath(outputDir, bundle.ProjectRoot); got != "." {
		t.Fatalf("bundle project root = %q", got)
	}
}

func TestPostRepairDesignCreateDisabledOmitsRepairBundle(t *testing.T) {
	result, _ := runPostRepairDesignCreateCLI(t, "bundle_basic", "--skip-routing")
	if postRepairStageExists(result.Data.Stages, "validation_repair") {
		t.Fatalf("validation_repair stage present without repair enabled: %#v", result.Data.Stages)
	}
	for _, artifact := range result.Artifacts {
		if normalizePostRepairPath(result.Data.Project.OutputDir, artifact.Path) == ".kicadai/repair-bundle.json" {
			t.Fatalf("repair bundle artifact present without repair enabled: %#v", artifact)
		}
	}
}

func TestPostRepairApplyValidationSummary(t *testing.T) {
	root := t.TempDir()
	outputDir := filepath.Join(root, "target")
	bundlePath := writePostRepairCleanBundle(t, root, outputDir)
	applied := runPostRepairTargetApplyCLI(t, outputDir, bundlePath, "--strict-unrouted")
	if applied.Data.Status == "" {
		t.Fatalf("repair apply status missing: %#v", applied.Data)
	}
	transaction := postRepairValidationByName(t, applied.Data.Validation, "transaction")
	writer := postRepairValidationByName(t, applied.Data.Validation, "writer_correctness")
	board := postRepairValidationByName(t, applied.Data.Validation, "board_validation")
	if transaction.Skipped || writer.Skipped || board.Skipped {
		t.Fatalf("built-in validators should not be skipped: transaction=%#v writer=%#v board=%#v", transaction, writer, board)
	}
	if len(writer.Issues) == 0 {
		t.Fatalf("writer correctness evidence missing issues: %#v", writer)
	}
	if applied.Data.Delta == nil {
		t.Fatalf("validation delta missing from repair apply: %#v", applied.Data)
	}
	before := postRepairSummaryMap(t, applied.Data.Delta, "before")
	after := postRepairSummaryMap(t, applied.Data.Delta, "after")
	beforeIssues := postRepairSummaryNumber(t, before, "issue_count")
	afterIssues := postRepairSummaryNumber(t, after, "issue_count")
	if beforeIssues != 0 || afterIssues < 1 {
		t.Fatalf("unexpected delta issue counts: before=%v (want 0), after=%v (want >= 1)", beforeIssues, afterIssues)
	}
}

func TestPostRepairApplyValidationDeltaStatuses(t *testing.T) {
	t.Run("clean", func(t *testing.T) {
		cleanRoot := t.TempDir()
		cleanOutput := filepath.Join(cleanRoot, "target")
		cleanBundle := writePostRepairCleanBundle(t, cleanRoot, cleanOutput)
		clean := runPostRepairTargetApplyCLI(t, cleanOutput, cleanBundle)
		if !clean.OK {
			t.Fatalf("clean apply ok=false, want true; issues=%#v result=%#v", clean.Issues, clean)
		}
		if clean.Data.Status != repair.StatusRepaired {
			t.Fatalf("clean apply status = %q, want %q", clean.Data.Status, repair.StatusRepaired)
		}
		cleanAfter := postRepairSummaryMap(t, clean.Data.Delta, "after")
		if got := postRepairSummaryNumber(t, cleanAfter, "issue_count"); got != 0 {
			t.Fatalf("clean after issue_count = %v, want 0", got)
		}
	})

	t.Run("strict", func(t *testing.T) {
		// The bundle is structurally clean; strict validation intentionally
		// surfaces non-blocking evidence such as skipped external KiCad checks.
		partialRoot := t.TempDir()
		partialOutput := filepath.Join(partialRoot, "target")
		partialBundle := writePostRepairCleanBundle(t, partialRoot, partialOutput)
		partial := runPostRepairTargetApplyCLI(t, partialOutput, partialBundle, "--strict-unrouted")
		if !partial.OK {
			t.Fatalf("strict apply ok=false, want true; issues=%#v result=%#v", partial.Issues, partial)
		}
		if partial.Data.Status != repair.StatusPartial {
			t.Fatalf("strict apply status = %q, want %q", partial.Data.Status, repair.StatusPartial)
		}
		partialAfter := postRepairSummaryMap(t, partial.Data.Delta, "after")
		if got := postRepairSummaryNumber(t, partialAfter, "blocking_count"); got != 0 {
			t.Fatalf("strict after blocking_count = %v, want 0", got)
		}
		if got := postRepairSummaryNumber(t, partialAfter, "warning_count"); got < 1 {
			t.Fatalf("strict after warning_count = %v, want >= 1", got)
		}
	})
}

func TestPostRepairApplyKiCadValidationPolicy(t *testing.T) {
	t.Run("optional missing DRC", func(t *testing.T) {
		root := t.TempDir()
		outputDir := filepath.Join(root, "target")
		bundlePath := writePostRepairCleanBundle(t, root, outputDir)
		missingCLI := filepath.Join(root, "missing-kicad-cli")
		result := runPostRepairTargetApplyCLI(t, outputDir, bundlePath, "--allow-missing-drc", "--kicad-cli", missingCLI)
		if !result.OK || result.Data.Status != repair.StatusPartial {
			t.Fatalf("optional missing DRC ok/status = %v/%q, want true/%q; issues=%#v", result.OK, result.Data.Status, repair.StatusPartial, result.Issues)
		}
		drc := postRepairValidationByName(t, result.Data.Validation, "kicad_drc")
		if len(drc.Issues) != 1 || drc.Issues[0].Severity != reports.SeverityWarning {
			t.Fatalf("optional DRC issues = %#v, want one warning", drc.Issues)
		}
	})

	t.Run("required missing DRC", func(t *testing.T) {
		root := t.TempDir()
		outputDir := filepath.Join(root, "target")
		bundlePath := writePostRepairCleanBundle(t, root, outputDir)
		missingCLI := filepath.Join(root, "missing-kicad-cli")
		result := runPostRepairTargetApplyCLI(t, outputDir, bundlePath, "--require-drc", "--kicad-cli", missingCLI)
		if result.OK || result.Data.Status != repair.StatusBlocked {
			t.Fatalf("required missing DRC ok/status = %v/%q, want false/%q; issues=%#v", result.OK, result.Data.Status, repair.StatusBlocked, result.Issues)
		}
		drc := postRepairValidationByName(t, result.Data.Validation, "kicad_drc")
		if len(drc.Issues) != 1 || drc.Issues[0].Severity != reports.SeverityError {
			t.Fatalf("required DRC issues = %#v, want one error", drc.Issues)
		}
	})
}
