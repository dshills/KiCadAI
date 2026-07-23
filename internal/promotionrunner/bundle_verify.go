package promotionrunner

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"kicadai/internal/creationevidence"
)

var bundleAddressPattern = regexp.MustCompile(`^sha256-([0-9a-f]{64})$`)
var bundleSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var windowsAbsolutePattern = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
var kicadEnvironmentPattern = regexp.MustCompile(`^KICAD[0-9]+_(?:SYMBOL|FOOTPRINT)_DIR$`)

func VerifyBundle(bundleRoot string, writeReceipt bool) (BundleVerification, error) {
	absolute, err := filepath.Abs(bundleRoot)
	if err != nil {
		return BundleVerification{}, err
	}
	rootInfo, err := os.Lstat(absolute)
	if err != nil {
		return BundleVerification{}, err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return BundleVerification{}, errors.New("bundle root must be a non-symlink directory")
	}
	addressMatch := bundleAddressPattern.FindStringSubmatch(filepath.Base(absolute))
	if addressMatch == nil {
		return BundleVerification{}, errors.New("bundle directory is not content-addressed")
	}
	manifestRaw, err := readRegularFile(filepath.Join(absolute, "manifest.json"), maxManifestBytes)
	if err != nil {
		return BundleVerification{}, fmt.Errorf("read manifest: %w", err)
	}
	manifestDigest := hashBytes(manifestRaw)
	if manifestDigest != addressMatch[1] {
		return BundleVerification{}, errors.New("content-address directory does not match manifest digest")
	}
	checksum, err := readRegularFile(filepath.Join(absolute, "manifest.sha256"), 256)
	if err != nil {
		return BundleVerification{}, fmt.Errorf("read manifest checksum: %w", err)
	}
	if string(checksum) != manifestDigest+"  manifest.json\n" {
		return BundleVerification{}, errors.New("manifest checksum mismatch")
	}
	var manifest BundleManifest
	if err := decodeStrictBundleJSON(manifestRaw, &manifest); err != nil {
		return BundleVerification{}, fmt.Errorf("decode manifest: %w", err)
	}
	canonical, err := canonicalJSON(manifest)
	if err != nil {
		return BundleVerification{}, err
	}
	if !bytes.Equal(canonical, manifestRaw) {
		return BundleVerification{}, errors.New("manifest is not canonical JSON")
	}
	if err := validateBundleManifest(manifest); err != nil {
		return BundleVerification{}, err
	}

	actualFiles, err := verifyBundleTree(absolute)
	if err != nil {
		return BundleVerification{}, err
	}
	if err := compareBundleInventory(manifest.Files, actualFiles); err != nil {
		return BundleVerification{}, err
	}
	inventory := make(map[string]BundleFile, len(manifest.Files))
	for _, file := range manifest.Files {
		inventory[file.Path] = file
	}
	if err := verifyReference(manifest.Toolchain, inventory); err != nil {
		return BundleVerification{}, fmt.Errorf("toolchain reference: %w", err)
	}
	if err := verifyReference(manifest.Commands, inventory); err != nil {
		return BundleVerification{}, fmt.Errorf("commands reference: %w", err)
	}
	toolchain, err := verifyBundleToolchain(absolute, manifest.Toolchain)
	if err != nil {
		return BundleVerification{}, err
	}
	_, err = verifyBundleCommands(absolute, manifest.Commands, manifest.Scenarios)
	if err != nil {
		return BundleVerification{}, err
	}
	if err := verifyBundleScenarios(absolute, manifest, inventory, toolchain); err != nil {
		return BundleVerification{}, err
	}

	verification := BundleVerification{
		Schema: BundleVerificationSchema, Status: "pass", Bundle: filepath.Base(absolute),
		ManifestSHA256: manifestDigest, FileCount: len(manifest.Files),
	}
	if writeReceipt {
		receipt, err := canonicalJSON(verification)
		if err != nil {
			return BundleVerification{}, err
		}
		if err := writeVerificationReceipt(absolute, receipt); err != nil {
			return BundleVerification{}, err
		}
	}
	return verification, nil
}

func writeVerificationReceipt(root string, receipt []byte) error {
	temporary, err := os.CreateTemp(root, ".verification-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(receipt); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, filepath.Join(root, "verification.json"))
}

func validateBundleManifest(manifest BundleManifest) error {
	if manifest.Schema != BundleSchema || manifest.Status != "pass" {
		return errors.New("bundle manifest schema or status is invalid")
	}
	if !revisionPattern.MatchString(manifest.RepositoryRevision) {
		return errors.New("bundle repository revision is invalid")
	}
	if manifest.Matrix.Schema != MatrixSchema || !bundleSHA256Pattern.MatchString(manifest.Matrix.SHA256) ||
		manifest.LaneRegistry.Schema != LaneSchema || !bundleSHA256Pattern.MatchString(manifest.LaneRegistry.SHA256) {
		return errors.New("bundle matrix or lane identity is invalid")
	}
	if manifest.LaneRegistry.SHA256 != LaneRegistrySHA256() {
		return errors.New("bundle lane registry identity is unsupported")
	}
	if len(manifest.Scenarios) == 0 || len(manifest.Files) == 0 {
		return errors.New("bundle manifest has no scenarios or files")
	}
	seenScenarios := map[string]struct{}{}
	for _, scenario := range manifest.Scenarios {
		if !identifierPattern.MatchString(scenario.ID) || scenario.Status != "pass" {
			return fmt.Errorf("scenario %q is malformed or not passing", scenario.ID)
		}
		if _, duplicate := seenScenarios[scenario.ID]; duplicate {
			return fmt.Errorf("duplicate bundle scenario %q", scenario.ID)
		}
		seenScenarios[scenario.ID] = struct{}{}
		if _, err := LaneFor(scenario.Lane); err != nil {
			return err
		}
		if len(scenario.Runs) != 2 || scenario.Runs[0].Run != 1 || scenario.Runs[1].Run != 2 {
			return fmt.Errorf("%s: bundle must contain ordered run 1 and run 2", scenario.ID)
		}
		for _, run := range scenario.Runs {
			if run.Status != "pass" || !bundleSHA256Pattern.MatchString(run.ProjectSHA256) {
				return fmt.Errorf("%s run %d is not a passing content-addressed run", scenario.ID, run.Run)
			}
			if err := validateBundlePath(run.PromotionPath); err != nil {
				return err
			}
		}
	}
	previous := ""
	seenFiles := map[string]struct{}{}
	for _, file := range manifest.Files {
		if err := validateBundlePath(file.Path); err != nil {
			return err
		}
		if file.Path <= previous {
			return errors.New("bundle inventory is not strictly sorted")
		}
		previous = file.Path
		if _, duplicate := seenFiles[file.Path]; duplicate {
			return fmt.Errorf("duplicate bundle inventory path %q", file.Path)
		}
		seenFiles[file.Path] = struct{}{}
		if file.Bytes < 0 || !bundleSHA256Pattern.MatchString(file.SHA256) {
			return fmt.Errorf("invalid bundle inventory record %q", file.Path)
		}
	}
	return nil
}

func verifyBundleTree(root string) ([]BundleFile, error) {
	var files []BundleFile
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("bundle contains symbolic link %q", path)
		}
		if path == root {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") && !(entry.IsDir() && name == ".kicadai") {
			return fmt.Errorf("bundle contains unrecognized hidden entry %q", path)
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
		switch relative {
		case "manifest.json", "manifest.sha256", "verification.json":
			return nil
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

func compareBundleInventory(expected, actual []BundleFile) error {
	if len(expected) != len(actual) {
		return fmt.Errorf("bundle file count is %d, manifest records %d", len(actual), len(expected))
	}
	for index := range expected {
		if expected[index] != actual[index] {
			return fmt.Errorf("bundle file mismatch at %q", expected[index].Path)
		}
	}
	return nil
}

func verifyReference(reference BundleReference, inventory map[string]BundleFile) error {
	if err := validateBundlePath(reference.Path); err != nil {
		return err
	}
	file, exists := inventory[reference.Path]
	if !exists || file.SHA256 != reference.SHA256 || file.Bytes != reference.Bytes {
		return fmt.Errorf("reference %q does not match inventory", reference.Path)
	}
	return nil
}

func verifyBundleToolchain(root string, reference BundleReference) (BundleToolchain, error) {
	raw, err := readRegularFile(filepath.Join(root, filepath.FromSlash(reference.Path)), maxManifestBytes)
	if err != nil {
		return BundleToolchain{}, err
	}
	var toolchain BundleToolchain
	if err := decodeStrictBundleJSON(raw, &toolchain); err != nil {
		return BundleToolchain{}, fmt.Errorf("decode bundle toolchain: %w", err)
	}
	canonical, err := canonicalJSON(toolchain)
	if err != nil || !bytes.Equal(canonical, raw) {
		return BundleToolchain{}, errors.New("bundle toolchain is not canonical JSON")
	}
	if toolchain.Schema != BundleToolchainSchema || toolchain.LockSHA256 == "" ||
		!bundleSHA256Pattern.MatchString(toolchain.LockSHA256) || toolchain.OS == "" || toolchain.Arch == "" ||
		toolchain.KiCadVersion == "" || !bundleSHA256Pattern.MatchString(toolchain.SymbolTableSHA256) ||
		!bundleSHA256Pattern.MatchString(toolchain.FootprintTableSHA256) ||
		!bundleSHA256Pattern.MatchString(toolchain.SymbolsIdentity.SHA256) ||
		!bundleSHA256Pattern.MatchString(toolchain.FootprintsIdentity.SHA256) {
		return BundleToolchain{}, errors.New("bundle toolchain identity is incomplete")
	}
	return toolchain, nil
}

func verifyBundleCommands(root string, reference BundleReference, scenarios []BundleScenario) (BundleCommands, error) {
	raw, err := readRegularFile(filepath.Join(root, filepath.FromSlash(reference.Path)), maxManifestBytes)
	if err != nil {
		return BundleCommands{}, err
	}
	var commands BundleCommands
	if err := decodeStrictBundleJSON(raw, &commands); err != nil {
		return BundleCommands{}, fmt.Errorf("decode bundle commands: %w", err)
	}
	canonical, err := canonicalJSON(commands)
	if err != nil || !bytes.Equal(canonical, raw) {
		return BundleCommands{}, errors.New("bundle commands are not canonical JSON")
	}
	if commands.Schema != BundleCommandsSchema || len(commands.Records) != len(scenarios)*2 {
		return BundleCommands{}, errors.New("bundle command schema or count is invalid")
	}
	allowedEnvironment := map[string]struct{}{
		"HOME": {}, "KICAD_CONFIG_HOME": {}, "KICADAI_KICAD_CLI": {},
		"KICADAI_SYMBOLS_ROOT": {}, "KICADAI_FOOTPRINTS_ROOT": {},
	}
	requiredArgs := []string{"--require-erc", "--require-drc", "--require-kicad-roundtrip", "--strict-diffs"}
	for index, record := range commands.Records {
		scenario := scenarios[index/2]
		run := index%2 + 1
		if record.Scenario != scenario.ID || record.Run != run || record.Status != "pass" {
			return BundleCommands{}, errors.New("bundle commands are not in scenario/run order")
		}
		for _, required := range requiredArgs {
			if !slices.Contains(record.Args, required) {
				return BundleCommands{}, fmt.Errorf("%s run %d is missing %s", scenario.ID, run, required)
			}
		}
		for _, argument := range record.Args {
			if absoluteEvidencePath(argument) {
				return BundleCommands{}, fmt.Errorf("%s run %d command contains absolute path", scenario.ID, run)
			}
		}
		for name, value := range record.Environment {
			if _, allowed := allowedEnvironment[name]; !allowed && !kicadEnvironmentPattern.MatchString(name) {
				return BundleCommands{}, fmt.Errorf("%s run %d has unrecognized environment key %q", scenario.ID, run, name)
			}
			if absoluteEvidencePath(value) {
				return BundleCommands{}, fmt.Errorf("%s run %d environment contains absolute path", scenario.ID, run)
			}
		}
	}
	return commands, nil
}

func absoluteEvidencePath(value string) bool {
	return strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\`) || windowsAbsolutePattern.MatchString(value)
}

func verifyBundleScenarios(
	root string,
	manifest BundleManifest,
	inventory map[string]BundleFile,
	toolchain BundleToolchain,
) error {
	for _, scenario := range manifest.Scenarios {
		if err := verifyReference(scenario.Request, inventory); err != nil {
			return fmt.Errorf("%s request: %w", scenario.ID, err)
		}
		if err := verifyReference(scenario.Comparison, inventory); err != nil {
			return fmt.Errorf("%s comparison: %w", scenario.ID, err)
		}
		comparisonRaw, err := readRegularFile(
			filepath.Join(root, filepath.FromSlash(scenario.Comparison.Path)), maxManifestBytes,
		)
		if err != nil {
			return err
		}
		var comparison Comparison
		if err := decodeStrictBundleJSON(comparisonRaw, &comparison); err != nil {
			return fmt.Errorf("%s comparison: %w", scenario.ID, err)
		}
		canonical, err := canonicalJSON(comparison)
		if err != nil || !bytes.Equal(canonical, comparisonRaw) {
			return fmt.Errorf("%s comparison is not canonical JSON", scenario.ID)
		}
		if comparison.Schema != ComparisonSchema || comparison.Scenario != scenario.ID ||
			comparison.Status != "pass" || comparison.Run1SHA256 == "" ||
			comparison.Run1SHA256 != comparison.Run2SHA256 || len(comparison.Differences) != 0 ||
			len(comparison.Files) == 0 {
			return fmt.Errorf("%s deterministic comparison is not a pass", scenario.ID)
		}
		if err := validateComparisonInventory(comparison); err != nil {
			return fmt.Errorf("%s comparison: %w", scenario.ID, err)
		}
		if scenario.Runs[0].ProjectSHA256 != comparison.Run1SHA256 ||
			scenario.Runs[1].ProjectSHA256 != comparison.Run2SHA256 {
			return fmt.Errorf("%s project addresses do not match comparison", scenario.ID)
		}
		for _, run := range scenario.Runs {
			projectPrefix := filepath.ToSlash(
				filepath.Join("scenarios", scenario.ID, fmt.Sprintf("run-%d", run.Run), "project"),
			) + "/"
			expectedPromotionPath := projectPrefix + filepath.ToSlash(creationevidence.DesignPromotionPath)
			if run.PromotionPath != expectedPromotionPath {
				return fmt.Errorf("%s run %d promotion path is not canonical", scenario.ID, run.Run)
			}
			if err := verifyNormalizedProjectInventory(projectPrefix, comparison.Files, inventory); err != nil {
				return fmt.Errorf("%s run %d: %w", scenario.ID, run.Run, err)
			}
			promotionRaw, err := readRegularFile(
				filepath.Join(root, filepath.FromSlash(run.PromotionPath)), maxManifestBytes,
			)
			if err != nil {
				return err
			}
			if err := validatePromotionEvidence(promotionRaw, toolchain.KiCadVersion); err != nil {
				return fmt.Errorf("%s run %d promotion: %w", scenario.ID, run.Run, err)
			}
		}
	}
	return nil
}

func validateComparisonInventory(comparison Comparison) error {
	previous := ""
	for _, file := range comparison.Files {
		if err := validateBundlePath(file.Path); err != nil {
			return err
		}
		if file.Path <= previous {
			return errors.New("normalized project inventory is not strictly sorted")
		}
		previous = file.Path
		if file.Kind != "bytes" && file.Kind != "json" && file.Kind != "kicad" {
			return fmt.Errorf("normalized project file %q has invalid kind %q", file.Path, file.Kind)
		}
		if file.Bytes < 0 || !bundleSHA256Pattern.MatchString(file.SHA256) {
			return fmt.Errorf("normalized project file %q has invalid content identity", file.Path)
		}
	}
	if inventorySHA256(comparison.Files) != comparison.Run1SHA256 {
		return errors.New("normalized project inventory digest does not match comparison")
	}
	return nil
}

func verifyNormalizedProjectInventory(prefix string, expected []NormalizedFile, inventory map[string]BundleFile) error {
	expectedPaths := make(map[string]NormalizedFile, len(expected))
	for _, file := range expected {
		if err := validateBundlePath(file.Path); err != nil {
			return err
		}
		if _, duplicate := expectedPaths[file.Path]; duplicate {
			return fmt.Errorf("duplicate normalized project path %q", file.Path)
		}
		expectedPaths[file.Path] = file
		bundlePath := prefix + file.Path
		actual, exists := inventory[bundlePath]
		if !exists || actual.SHA256 != file.SHA256 || actual.Bytes != file.Bytes {
			return fmt.Errorf("normalized project file %q does not match comparison", file.Path)
		}
	}
	actualCount := 0
	for path := range inventory {
		if strings.HasPrefix(path, prefix) {
			actualCount++
			relative := strings.TrimPrefix(path, prefix)
			if _, exists := expectedPaths[relative]; !exists {
				return fmt.Errorf("normalized project has extra file %q", relative)
			}
		}
	}
	if actualCount != len(expected) {
		return fmt.Errorf("normalized project file count is %d, want %d", actualCount, len(expected))
	}
	return nil
}
