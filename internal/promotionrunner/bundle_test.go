package promotionrunner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/promotiontoolchain"
)

func TestBundleBuildIsDeterministicAndVerifiesOffline(t *testing.T) {
	root := t.TempDir()
	matrix, toolchain, firstResults, firstPromotionRoot := runFakeBundlePromotion(t, root, "promotion-1")
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for index := range firstResults {
		relative, err := filepath.Rel(workingDirectory, firstResults[index].Project)
		if err != nil {
			t.Fatal(err)
		}
		firstResults[index].Project = relative
	}
	first, err := BuildBundle(BundleBuildOptions{
		RepositoryRoot: root, PromotionRoot: firstPromotionRoot,
		DestinationParent: filepath.Join(root, "bundles-1"), RepositoryRevision: strings.Repeat("a", 40),
		Matrix: matrix, Toolchain: toolchain, Results: firstResults,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, secondResults, secondPromotionRoot := runFakeBundlePromotion(t, root, "promotion-2")
	second, err := BuildBundle(BundleBuildOptions{
		RepositoryRoot: root, PromotionRoot: secondPromotionRoot,
		DestinationParent: filepath.Join(root, "bundles-2"), RepositoryRevision: strings.Repeat("a", 40),
		Matrix: matrix, Toolchain: toolchain, Results: secondResults,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ManifestSHA256 != second.ManifestSHA256 || filepath.Base(first.Path) != filepath.Base(second.Path) {
		t.Fatalf("bundle digests differ: %#v != %#v", first, second)
	}
	verification, err := VerifyBundle(first.Path, true)
	if err != nil {
		t.Fatal(err)
	}
	if verification.Status != "pass" || verification.ManifestSHA256 != first.ManifestSHA256 {
		t.Fatalf("verification = %#v", verification)
	}
	if _, err := VerifyBundle(first.Path, false); err != nil {
		t.Fatalf("verification receipt affected verification: %v", err)
	}
}

func TestBundleVerifierRejectsByteTampering(t *testing.T) {
	root := t.TempDir()
	bundle := buildFakeBundle(t, root)
	tests := []struct {
		name   string
		mutate func(t *testing.T, path string)
	}{
		{name: "manifest", mutate: func(t *testing.T, path string) {
			appendFile(t, filepath.Join(path, "manifest.json"), " ")
		}},
		{name: "checksum", mutate: func(t *testing.T, path string) {
			appendFile(t, filepath.Join(path, "manifest.sha256"), "x")
		}},
		{name: "request", mutate: func(t *testing.T, path string) {
			appendFile(t, filepath.Join(path, "scenarios", "case", "request.json"), " ")
		}},
		{name: "project", mutate: func(t *testing.T, path string) {
			appendFile(t, filepath.Join(path, "scenarios", "case", "run-1", "project", ".kicadai", "workflow-result.json"), " ")
		}},
		{name: "report", mutate: func(t *testing.T, path string) {
			appendFile(t, filepath.Join(path, "scenarios", "case", "comparison.json"), " ")
		}},
		{name: "extra", mutate: func(t *testing.T, path string) {
			if err := os.WriteFile(filepath.Join(path, "extra.txt"), []byte("extra"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "hidden", mutate: func(t *testing.T, path string) {
			if err := os.WriteFile(filepath.Join(path, ".DS_Store"), []byte("hidden"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			copyRoot := filepath.Join(t.TempDir(), filepath.Base(bundle.Path))
			copyDirectory(t, bundle.Path, copyRoot)
			test.mutate(t, copyRoot)
			if _, err := VerifyBundle(copyRoot, false); err == nil {
				t.Fatal("expected tamper rejection")
			}
		})
	}
}

func TestBundleVerifierRejectsSymlinkAndAddressMismatch(t *testing.T) {
	if filepath.Separator != '/' {
		t.Skip("symbolic-link test requires POSIX semantics")
	}
	root := t.TempDir()
	bundle := buildFakeBundle(t, root)
	symlinkRoot := filepath.Join(t.TempDir(), filepath.Base(bundle.Path))
	copyDirectory(t, bundle.Path, symlinkRoot)
	if err := os.Symlink("manifest.json", filepath.Join(symlinkRoot, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyBundle(symlinkRoot, false); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("expected symbolic-link rejection, got %v", err)
	}
	wrongAddress := filepath.Join(t.TempDir(), "sha256-"+strings.Repeat("0", 64))
	copyDirectory(t, bundle.Path, wrongAddress)
	if _, err := VerifyBundle(wrongAddress, false); err == nil || !strings.Contains(err.Error(), "content-address") {
		t.Fatalf("expected address rejection, got %v", err)
	}
}

func TestBundleVerifierRejectsSelfConsistentSkippedGate(t *testing.T) {
	root := t.TempDir()
	bundle := buildFakeBundle(t, root)
	copyRoot := filepath.Join(t.TempDir(), filepath.Base(bundle.Path))
	copyDirectory(t, bundle.Path, copyRoot)
	for _, run := range []string{"run-1", "run-2"} {
		mutatePromotionGate(t, filepath.Join(
			copyRoot, "scenarios", "case", run, "project", ".kicadai", "design-promotion.json",
		))
	}
	refreshComparison(t, filepath.Join(copyRoot, "scenarios", "case"))
	readdressed := readdressBundle(t, copyRoot)
	if _, err := VerifyBundle(readdressed, false); err == nil || !strings.Contains(err.Error(), "kicad_checks") {
		t.Fatalf("expected skipped-gate rejection, got %v", err)
	}
}

func TestBundleVerifierRejectsSelfConsistentMissingKiCadArgument(t *testing.T) {
	root := t.TempDir()
	bundle := buildFakeBundle(t, root)
	copyRoot := filepath.Join(t.TempDir(), filepath.Base(bundle.Path))
	copyDirectory(t, bundle.Path, copyRoot)
	commandsPath := filepath.Join(copyRoot, "commands.json")
	raw, err := os.ReadFile(commandsPath)
	if err != nil {
		t.Fatal(err)
	}
	var commands BundleCommands
	if err := json.Unmarshal(raw, &commands); err != nil {
		t.Fatal(err)
	}
	commands.Records[0].Args = removeString(commands.Records[0].Args, "--require-drc")
	writeCanonicalTestJSON(t, commandsPath, commands)
	readdressed := readdressBundle(t, copyRoot)
	if _, err := VerifyBundle(readdressed, false); err == nil || !strings.Contains(err.Error(), "--require-drc") {
		t.Fatalf("expected required-argument rejection, got %v", err)
	}
}

func TestBundleBuildRejectsResultOutsidePromotionRoot(t *testing.T) {
	root := t.TempDir()
	matrix, toolchain, results, promotionRoot := runFakeBundlePromotion(t, root, "promotion")
	results[0].Project = filepath.Join(root, "unrelated", "project")
	_, err := BuildBundle(BundleBuildOptions{
		RepositoryRoot: root, PromotionRoot: promotionRoot,
		DestinationParent: filepath.Join(root, "bundles"), RepositoryRevision: strings.Repeat("a", 40),
		Matrix: matrix, Toolchain: toolchain, Results: results,
	})
	if err == nil || !strings.Contains(err.Error(), "outside the declared promotion run") {
		t.Fatalf("expected promotion-root rejection, got %v", err)
	}
}

func TestBundleVerifierRejectsSelfConsistentFalseProjectDigest(t *testing.T) {
	root := t.TempDir()
	bundle := buildFakeBundle(t, root)
	copyRoot := filepath.Join(t.TempDir(), filepath.Base(bundle.Path))
	copyDirectory(t, bundle.Path, copyRoot)
	comparisonPath := filepath.Join(copyRoot, "scenarios", "case", "comparison.json")
	raw, err := os.ReadFile(comparisonPath)
	if err != nil {
		t.Fatal(err)
	}
	var comparison Comparison
	if err := json.Unmarshal(raw, &comparison); err != nil {
		t.Fatal(err)
	}
	comparison.Run1SHA256 = strings.Repeat("2", 64)
	comparison.Run2SHA256 = comparison.Run1SHA256
	writeCanonicalTestJSON(t, comparisonPath, comparison)
	readdressed := readdressBundle(t, copyRoot)
	if _, err := VerifyBundle(readdressed, false); err == nil ||
		!strings.Contains(err.Error(), "inventory digest") {
		t.Fatalf("expected false project-digest rejection, got %v", err)
	}
}

func TestAbsoluteEvidencePathRecognizesHostForms(t *testing.T) {
	for _, value := range []string{"/tmp/file", `\temp\file`, `\\server\share`, `C:\temp\file`, "C:/temp/file"} {
		if !absoluteEvidencePath(value) {
			t.Errorf("absoluteEvidencePath(%q) = false", value)
		}
	}
	for _, value := range []string{"${PROJECT}/file", "relative/file", `relative\file`} {
		if absoluteEvidencePath(value) {
			t.Errorf("absoluteEvidencePath(%q) = true", value)
		}
	}
}

func buildFakeBundle(t *testing.T, root string) BundleResult {
	t.Helper()
	matrix, toolchain, results, promotionRoot := runFakeBundlePromotion(t, root, "promotion")
	bundle, err := BuildBundle(BundleBuildOptions{
		RepositoryRoot: root, PromotionRoot: promotionRoot,
		DestinationParent: filepath.Join(root, "bundles"), RepositoryRevision: strings.Repeat("a", 40),
		Matrix: matrix, Toolchain: toolchain, Results: results,
	})
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func runFakeBundlePromotion(t *testing.T, root, outputName string) (MatrixDocument, promotiontoolchain.Evidence, []RunResult, string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "request.json"), []byte(`{"request":"stable"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	matrixPath := filepath.Join(root, "matrix.json")
	if _, err := os.Stat(matrixPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(matrixPath, []byte(testMatrixJSON("intent", "request.json")), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	matrix, err := LoadMatrix(matrixPath, root)
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("1", 64)
	toolchain := promotiontoolchain.Evidence{
		Schema: promotiontoolchain.EvidenceSchema, Version: 1, LockSHA256: hash,
		OS: "test", Arch: "test", KiCadVersion: "10.0.3", KiCadCLI: "/locked/kicad-cli",
		SymbolsRoot: "/locked/symbols", FootprintsRoot: "/locked/footprints",
		SymbolTable: "/locked/template/sym-lib-table", FootprintTable: "/locked/template/fp-lib-table",
		SymbolTableSHA256: hash, FootprintTableSHA256: hash,
		SymbolsIdentity:    promotiontoolchain.LibraryIdentity{SHA256: hash, FileCount: 1, ByteCount: 1},
		FootprintsIdentity: promotiontoolchain.LibraryIdentity{SHA256: hash, FileCount: 1, ByteCount: 1},
		Resolution:         "test",
	}
	promotion := validPromotionDocument(t)
	var document map[string]any
	if err := json.Unmarshal(promotion, &document); err != nil {
		t.Fatal(err)
	}
	document["kicad_version"] = toolchain.KiCadVersion
	promotion, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, outputName)
	results, err := Run(context.Background(), matrix, toolchain, Options{
		RepositoryRoot: root, KiCadAI: fakePromotionCLI(t, promotion), OutputRoot: output,
	})
	if err != nil {
		t.Fatal(err)
	}
	return matrix, toolchain, results, output
}

func copyDirectory(t *testing.T, source, destination string) {
	t.Helper()
	if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			input.Close()
			return err
		}
		if _, err := io.Copy(output, input); err != nil {
			input.Close()
			output.Close()
			return err
		}
		if err := input.Close(); err != nil {
			output.Close()
			return err
		}
		return output.Close()
	}); err != nil {
		t.Fatal(err)
	}
}

func mutatePromotionGate(t *testing.T, promotionPath string) {
	t.Helper()
	raw, err := os.ReadFile(promotionPath)
	if err != nil {
		t.Fatal(err)
	}
	var promotion map[string]any
	if err := json.Unmarshal(raw, &promotion); err != nil {
		t.Fatal(err)
	}
	for _, gate := range promotion["gates"].([]any) {
		typed := gate.(map[string]any)
		if typed["id"] == "kicad_checks" {
			typed["status"] = "skipped"
		}
	}
	writeCanonicalTestJSON(t, promotionPath, promotion)
}

func refreshComparison(t *testing.T, scenarioRoot string) {
	t.Helper()
	comparisonPath := filepath.Join(scenarioRoot, "comparison.json")
	raw, err := os.ReadFile(comparisonPath)
	if err != nil {
		t.Fatal(err)
	}
	var comparison Comparison
	if err := json.Unmarshal(raw, &comparison); err != nil {
		t.Fatal(err)
	}
	const promotionRelative = ".kicadai/design-promotion.json"
	for index := range comparison.Files {
		if comparison.Files[index].Path != promotionRelative {
			continue
		}
		projectPath := filepath.Join(scenarioRoot, "run-1", "project", filepath.FromSlash(promotionRelative))
		projectRaw, err := os.ReadFile(projectPath)
		if err != nil {
			t.Fatal(err)
		}
		comparison.Files[index].SHA256 = hashBytes(projectRaw)
		comparison.Files[index].Bytes = int64(len(projectRaw))
	}
	comparison.Run1SHA256 = inventorySHA256(comparison.Files)
	comparison.Run2SHA256 = comparison.Run1SHA256
	writeCanonicalTestJSON(t, comparisonPath, comparison)
}

func appendFile(t *testing.T, path, value string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(value); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeCanonicalTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := canonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readdressBundle(t *testing.T, root string) string {
	t.Helper()
	manifestPath := filepath.Join(root, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest BundleManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	files, err := verifyBundleTree(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Files = files
	inventory := make(map[string]BundleFile, len(files))
	for _, file := range files {
		inventory[file.Path] = file
	}
	manifest.Toolchain = BundleReference(inventory[manifest.Toolchain.Path])
	manifest.Commands = BundleReference(inventory[manifest.Commands.Path])
	for index := range manifest.Scenarios {
		scenario := &manifest.Scenarios[index]
		scenario.Request = BundleReference(inventory[scenario.Request.Path])
		scenario.Comparison = BundleReference(inventory[scenario.Comparison.Path])
		comparisonRaw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(scenario.Comparison.Path)))
		if err != nil {
			t.Fatal(err)
		}
		var comparison Comparison
		if err := json.Unmarshal(comparisonRaw, &comparison); err != nil {
			t.Fatal(err)
		}
		if len(scenario.Runs) != 2 {
			t.Fatalf("scenario %q has %d runs", scenario.ID, len(scenario.Runs))
		}
		scenario.Runs[0].ProjectSHA256 = comparison.Run1SHA256
		scenario.Runs[1].ProjectSHA256 = comparison.Run2SHA256
	}
	raw, err = canonicalJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	digest := hashBytes(raw)
	if err := os.WriteFile(manifestPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.sha256"), []byte(digest+"  manifest.json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(filepath.Dir(root), "sha256-"+digest)
	if err := os.Rename(root, destination); err != nil {
		t.Fatal(err)
	}
	return destination
}

func removeString(values []string, target string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}
