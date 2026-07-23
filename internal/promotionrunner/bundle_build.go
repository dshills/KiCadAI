package promotionrunner

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"kicadai/internal/creationevidence"
	"kicadai/internal/designworkflow"
	"kicadai/internal/promotiontoolchain"
)

var revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

const maxTextLogBytes int64 = 256 << 20

func BuildBundle(options BundleBuildOptions) (BundleResult, error) {
	if err := validateBundleBuildOptions(options); err != nil {
		return BundleResult{}, err
	}
	if err := os.MkdirAll(options.DestinationParent, 0o755); err != nil {
		return BundleResult{}, err
	}
	temporary, err := os.MkdirTemp(options.DestinationParent, ".promotion-bundle-")
	if err != nil {
		return BundleResult{}, err
	}
	keepTemporary := false
	defer func() {
		if !keepTemporary {
			_ = os.RemoveAll(temporary)
		}
	}()

	toolchain := BundleToolchain{
		Schema: BundleToolchainSchema, LockSHA256: options.Toolchain.LockSHA256,
		OS: options.Toolchain.OS, Arch: options.Toolchain.Arch, KiCadVersion: options.Toolchain.KiCadVersion,
		SymbolTableSHA256:    options.Toolchain.SymbolTableSHA256,
		FootprintTableSHA256: options.Toolchain.FootprintTableSHA256,
		SymbolsIdentity:      options.Toolchain.SymbolsIdentity, FootprintsIdentity: options.Toolchain.FootprintsIdentity,
	}
	if _, err := writeCanonicalJSON(filepath.Join(temporary, "toolchain.json"), toolchain); err != nil {
		return BundleResult{}, err
	}

	resultByScenarioRun, err := indexRunResults(
		options.Matrix, options.Results, options.Toolchain.KiCadVersion, options.PromotionRoot,
	)
	if err != nil {
		return BundleResult{}, err
	}
	commands := BundleCommands{Schema: BundleCommandsSchema, Records: []BundleCommand{}}
	for _, scenario := range options.Matrix.Matrix.Scenarios {
		for run := 1; run <= 2; run++ {
			result := resultByScenarioRun[scenarioRunKey(scenario.ID, run)]
			commands.Records = append(commands.Records, normalizeBundleCommand(result, options.RepositoryRoot, options.Toolchain))
		}
	}
	if _, err := writeCanonicalJSON(filepath.Join(temporary, "commands.json"), commands); err != nil {
		return BundleResult{}, err
	}

	scenarios := make([]BundleScenario, 0, len(options.Matrix.Matrix.Scenarios))
	for _, scenario := range options.Matrix.Matrix.Scenarios {
		scenarioRoot := filepath.Join(temporary, "scenarios", scenario.ID)
		requestSource := filepath.Join(options.RepositoryRoot, filepath.FromSlash(scenario.Fixture))
		requestPath := filepath.Join(scenarioRoot, "request.json")
		if err := copyRegularFile(requestSource, requestPath); err != nil {
			return BundleResult{}, fmt.Errorf("%s request: %w", scenario.ID, err)
		}
		requestReference, err := bundleReference(temporary, requestPath)
		if err != nil {
			return BundleResult{}, err
		}

		bundleScenario := BundleScenario{
			ID: scenario.ID, Lane: scenario.Lane, Status: "pass", Request: requestReference,
			Runs: []BundleRun{},
		}
		var comparison *Comparison
		for run := 1; run <= 2; run++ {
			result := resultByScenarioRun[scenarioRunKey(scenario.ID, run)]
			runRoot := filepath.Join(scenarioRoot, fmt.Sprintf("run-%d", run))
			projectDestination := filepath.Join(runRoot, "project")
			if err := os.MkdirAll(projectDestination, 0o755); err != nil {
				return BundleResult{}, err
			}
			if _, err := normalizedInventoryAt(
				result.Project, options.RepositoryRoot, options.Toolchain, projectDestination,
			); err != nil {
				return BundleResult{}, fmt.Errorf("%s run %d normalized project: %w", scenario.ID, run, err)
			}
			sourceRunRoot := filepath.Dir(result.Project)
			context := newNormalizationContext(result.Project, options.RepositoryRoot, options.Toolchain)
			if err := copyNormalizedJSON(
				filepath.Join(sourceRunRoot, "stdout.json"), filepath.Join(runRoot, "stdout.json"), context,
			); err != nil {
				return BundleResult{}, fmt.Errorf("%s run %d stdout: %w", scenario.ID, run, err)
			}
			if err := copyNormalizedText(
				filepath.Join(sourceRunRoot, "stderr.txt"), filepath.Join(runRoot, "stderr.txt"), context,
			); err != nil {
				return BundleResult{}, fmt.Errorf("%s run %d stderr: %w", scenario.ID, run, err)
			}
			projectHash := ""
			if run == 2 {
				comparison = result.Comparison
			}
			if result.Comparison != nil {
				if run == 1 {
					projectHash = result.Comparison.Run1SHA256
				} else {
					projectHash = result.Comparison.Run2SHA256
				}
			}
			if run == 1 {
				second := resultByScenarioRun[scenarioRunKey(scenario.ID, 2)]
				if second.Comparison != nil {
					projectHash = second.Comparison.Run1SHA256
				}
			}
			bundleScenario.Runs = append(bundleScenario.Runs, BundleRun{
				Run: run, Status: "pass", ProjectSHA256: projectHash,
				PromotionPath: filepath.ToSlash(filepath.Join(
					"scenarios", scenario.ID, fmt.Sprintf("run-%d", run), "project", creationevidence.DesignPromotionPath,
				)),
			})
		}
		if comparison == nil {
			return BundleResult{}, fmt.Errorf("%s: missing deterministic comparison", scenario.ID)
		}
		comparisonPath := filepath.Join(scenarioRoot, "comparison.json")
		if _, err := writeCanonicalJSON(comparisonPath, comparison); err != nil {
			return BundleResult{}, err
		}
		comparisonReference, err := bundleReference(temporary, comparisonPath)
		if err != nil {
			return BundleResult{}, err
		}
		bundleScenario.Comparison = comparisonReference
		scenarios = append(scenarios, bundleScenario)
	}

	files, err := bundleInventory(temporary)
	if err != nil {
		return BundleResult{}, err
	}
	toolchainReference, err := referenceFromInventory(files, "toolchain.json")
	if err != nil {
		return BundleResult{}, err
	}
	commandsReference, err := referenceFromInventory(files, "commands.json")
	if err != nil {
		return BundleResult{}, err
	}
	manifest := BundleManifest{
		Schema: BundleSchema, Status: "pass", RepositoryRevision: options.RepositoryRevision,
		Matrix:       BundleIdentity{Schema: MatrixSchema, SHA256: options.Matrix.SHA256},
		LaneRegistry: BundleIdentity{Schema: LaneSchema, SHA256: LaneRegistrySHA256()},
		Toolchain:    toolchainReference, Commands: commandsReference, Scenarios: scenarios, Files: files,
	}
	manifestBytes, err := canonicalJSON(manifest)
	if err != nil {
		return BundleResult{}, err
	}
	manifestDigest := hashBytes(manifestBytes)
	if err := writeExclusiveFile(filepath.Join(temporary, "manifest.json"), manifestBytes); err != nil {
		return BundleResult{}, err
	}
	checksum := []byte(manifestDigest + "  manifest.json\n")
	if err := writeExclusiveFile(filepath.Join(temporary, "manifest.sha256"), checksum); err != nil {
		return BundleResult{}, err
	}

	finalPath := filepath.Join(options.DestinationParent, "sha256-"+manifestDigest)
	if _, err := os.Lstat(finalPath); err == nil {
		verification, verifyErr := VerifyBundle(finalPath, false)
		if verifyErr != nil {
			return BundleResult{}, fmt.Errorf("existing bundle address is invalid: %w", verifyErr)
		}
		return BundleResult{
			Schema: BundleSchema, Status: "pass", Path: finalPath,
			ManifestSHA256: verification.ManifestSHA256, FileCount: verification.FileCount,
		}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return BundleResult{}, err
	}
	if err := os.Rename(temporary, finalPath); err != nil {
		return BundleResult{}, err
	}
	verification, err := VerifyBundle(finalPath, false)
	if err != nil {
		if removeErr := os.RemoveAll(finalPath); removeErr != nil {
			return BundleResult{}, errors.Join(fmt.Errorf("verify completed bundle: %w", err), removeErr)
		}
		return BundleResult{}, fmt.Errorf("verify completed bundle: %w", err)
	}
	keepTemporary = true
	return BundleResult{
		Schema: BundleSchema, Status: "pass", Path: finalPath,
		ManifestSHA256: manifestDigest, FileCount: verification.FileCount,
	}, nil
}

func validateBundleBuildOptions(options BundleBuildOptions) error {
	for name, value := range map[string]string{
		"repository root": options.RepositoryRoot, "promotion root": options.PromotionRoot,
		"destination parent": options.DestinationParent,
	} {
		if value == "" || !filepath.IsAbs(value) {
			return fmt.Errorf("%s must be an absolute path", name)
		}
	}
	if !revisionPattern.MatchString(options.RepositoryRevision) {
		return errors.New("repository revision must be a lowercase 40-character Git SHA")
	}
	if options.Matrix.Matrix.SchemaVersion != MatrixSchema || options.Matrix.SHA256 == "" {
		return errors.New("validated promotion matrix is required")
	}
	if options.Toolchain.Schema == "" || options.Toolchain.KiCadVersion == "" ||
		options.Toolchain.LockSHA256 == "" || options.Toolchain.SymbolsIdentity.SHA256 == "" ||
		options.Toolchain.FootprintsIdentity.SHA256 == "" {
		return errors.New("complete resolved toolchain evidence is required")
	}
	return nil
}

func indexRunResults(
	matrix MatrixDocument,
	results []RunResult,
	kicadVersion string,
	promotionRoot string,
) (map[string]RunResult, error) {
	expected := len(matrix.Matrix.Scenarios) * 2
	if len(results) != expected {
		return nil, fmt.Errorf("promotion result count is %d, want %d", len(results), expected)
	}
	indexed := make(map[string]RunResult, expected)
	for _, result := range results {
		key := scenarioRunKey(result.Scenario, result.Run)
		if result.Run < 1 || result.Run > 2 || result.Command.Status != "pass" || result.Project == "" {
			return nil, fmt.Errorf("%s: incomplete passing run result", key)
		}
		absoluteProject, err := filepath.Abs(result.Project)
		if err != nil {
			return nil, fmt.Errorf("%s: resolve project path: %w", key, err)
		}
		expectedProject := filepath.Join(
			promotionRoot, "scenarios", result.Scenario, fmt.Sprintf("run-%d", result.Run), "project",
		)
		if filepath.Clean(absoluteProject) != filepath.Clean(expectedProject) {
			return nil, fmt.Errorf("%s: project is outside the declared promotion run", key)
		}
		result.Project = absoluteProject
		if _, exists := indexed[key]; exists {
			return nil, fmt.Errorf("duplicate run result %s", key)
		}
		if err := validatePromotionEvidence(result.Promotion, kicadVersion); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		indexed[key] = result
	}
	for _, scenario := range matrix.Matrix.Scenarios {
		for run := 1; run <= 2; run++ {
			if _, exists := indexed[scenarioRunKey(scenario.ID, run)]; !exists {
				return nil, fmt.Errorf("%s run %d: missing result", scenario.ID, run)
			}
		}
		second := indexed[scenarioRunKey(scenario.ID, 2)]
		if second.Comparison == nil || second.Comparison.Status != "pass" ||
			second.Comparison.Run1SHA256 == "" ||
			second.Comparison.Run1SHA256 != second.Comparison.Run2SHA256 ||
			len(second.Comparison.Differences) != 0 {
			return nil, fmt.Errorf("%s: deterministic comparison did not pass", scenario.ID)
		}
	}
	return indexed, nil
}

func validatePromotionEvidence(raw json.RawMessage, kicadVersion string) error {
	var document creationevidence.DesignPromotionDocument
	if err := decodeStrictJSON(raw, &document); err != nil {
		return fmt.Errorf("decode promotion evidence: %w", err)
	}
	if document.SchemaVersion != creationevidence.DesignPromotionSchema ||
		document.Applicability.Status != "applicable" ||
		document.Status != designworkflow.PromotionStatusPass ||
		document.AchievedReadiness != designworkflow.PromotionReadinessPass {
		return errors.New("promotion evidence is not an applicable pass")
	}
	if document.KiCadVersion != kicadVersion {
		return fmt.Errorf("promotion KiCad version %q does not match %q", document.KiCadVersion, kicadVersion)
	}
	gates := make(map[string]designworkflow.PromotionGateStatus, len(document.Gates))
	for _, gate := range document.Gates {
		gates[gate.ID] = gate.Status
	}
	for _, required := range strictGateContract.Promotion {
		if gates[required] != designworkflow.PromotionGateStatusPass {
			return fmt.Errorf("required promotion gate %q is %q", required, gates[required])
		}
	}
	return nil
}

func scenarioRunKey(scenario string, run int) string {
	return fmt.Sprintf("%s/%d", scenario, run)
}

func normalizeBundleCommand(result RunResult, repositoryRoot string, toolchain promotiontoolchain.Evidence) BundleCommand {
	context := newNormalizationContext(result.Project, repositoryRoot, toolchain)
	args := make([]string, len(result.Command.Args))
	for index, argument := range result.Command.Args {
		args[index] = normalizeString(argument, context)
	}
	environment := make(map[string]string, len(result.Command.Environment))
	for name, value := range result.Command.Environment {
		environment[name] = normalizeString(value, context)
	}
	return BundleCommand{
		Scenario: result.Scenario, Run: result.Run, Args: args, Environment: environment, Status: result.Command.Status,
	}
}

func copyNormalizedJSON(source, destination string, context normalizationContext) error {
	raw, err := readRegularFile(source, maxJSONBytes)
	if err != nil {
		return err
	}
	normalized, err := normalizeJSON(raw, context)
	if err != nil {
		return err
	}
	return writeExclusiveFile(destination, normalized)
}

func copyNormalizedText(source, destination string, context normalizationContext) error {
	raw, err := readRegularFile(source, maxTextLogBytes)
	if err != nil {
		return err
	}
	normalized, _ := maskKnownPaths(string(raw), context)
	return writeExclusiveFile(destination, []byte(normalized))
}

func copyRegularFile(source, destination string) error {
	input, err := openRegularFile(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := createExclusiveFile(destination)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}

func openRegularFile(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a regular non-symlink file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		file.Close()
		return nil, fmt.Errorf("%q changed while it was being opened", path)
	}
	return file, nil
}

func readRegularFile(path string, limit int64) ([]byte, error) {
	file, err := openRegularFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	limited := &io.LimitedReader{R: file, N: limit + 1}
	value, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if limited.N == 0 {
		return nil, fmt.Errorf("%q exceeds size limit %d", path, limit)
	}
	return value, nil
}

func canonicalJSON(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func writeCanonicalJSON(path string, value any) ([]byte, error) {
	encoded, err := canonicalJSON(value)
	if err != nil {
		return nil, err
	}
	return encoded, writeExclusiveFile(path, encoded)
}

func bundleInventory(root string) ([]BundleFile, error) {
	var files []BundleFile
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("bundle contains symbolic link %q", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("bundle contains non-regular file %q", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if err := validateBundlePath(relative); err != nil {
			return err
		}
		if relative == "manifest.json" || relative == "manifest.sha256" || relative == "verification.json" {
			return fmt.Errorf("reserved bundle file %q exists before manifest finalization", relative)
		}
		reference, err := bundleReference(root, path)
		if err != nil {
			return err
		}
		files = append(files, BundleFile(reference))
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(files, func(left, right BundleFile) int { return strings.Compare(left.Path, right.Path) })
	return files, nil
}

func bundleReference(root, path string) (BundleReference, error) {
	file, err := openRegularFile(path)
	if err != nil {
		return BundleReference{}, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return BundleReference{}, err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return BundleReference{}, err
	}
	return BundleReference{
		Path: filepath.ToSlash(relative), SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: size,
	}, nil
}

func referenceFromInventory(files []BundleFile, path string) (BundleReference, error) {
	for _, file := range files {
		if file.Path == path {
			return BundleReference(file), nil
		}
	}
	return BundleReference{}, fmt.Errorf("bundle inventory is missing %q", path)
}

func validateBundlePath(path string) error {
	if path == "" || path != filepath.ToSlash(path) || strings.Contains(path, `\`) ||
		strings.HasPrefix(path, "/") || filepath.IsAbs(path) || filepath.ToSlash(filepath.Clean(path)) != path {
		return fmt.Errorf("unsafe bundle path %q", path)
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("unsafe bundle path %q", path)
		}
	}
	return nil
}

func decodeStrictBundleJSON(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
