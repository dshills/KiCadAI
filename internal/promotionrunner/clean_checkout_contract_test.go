package promotionrunner

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var pinnedActionPattern = regexp.MustCompile(`uses:\s*[^@\s]+@([^\s#]+)`)
var fullCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func TestCleanCheckoutPromotionCommandContract(t *testing.T) {
	root := repositoryRootForContractTest(t)
	makefile := readContractFile(t, filepath.Join(root, "Makefile"))
	for _, required := range []string{
		"promotion-bundle:", "held-out-promotion-bundle:", "./scripts/clean-checkout-promotion.sh",
		`PROMOTION_ROOT="$(PROMOTION_ROOT)"`, `PROMOTION_CACHE_DIR="$(PROMOTION_CACHE_DIR)"`,
		`PROMOTION_MATRIX="$(HELD_OUT_PROMOTION_MATRIX)"`,
	} {
		if !strings.Contains(makefile, required) {
			t.Errorf("Makefile is missing %q", required)
		}
	}

	scriptPath := filepath.Join(root, "scripts", "clean-checkout-promotion.sh")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("clean-checkout promotion wrapper is not executable")
	}
	if output, err := exec.Command("sh", "-n", scriptPath).CombinedOutput(); err != nil {
		t.Fatalf("wrapper shell syntax: %v: %s", err, output)
	}
	script := readContractFile(t, scriptPath)
	for _, required := range []string{
		"status --porcelain --untracked-files=normal",
		"go build -o \"$kicadai_cli\" ./cmd/kicadai",
		"go build -o \"$promotion_cli\" ./cmd/kicadai-promotion",
		"matrix_path=${PROMOTION_MATRIX:-\"$repo_root/testdata/external-review-mitigation/matrix.json\"}",
		"--matrix \"$matrix_path\"",
		"--bundle-output \"$bundle_root\"",
		"--revision \"$revision\"",
		"--bootstrap",
		"--cache-dir \"$cache_root\"",
		"checkout changed during promotion; refusing to publish its bundle",
		"verify --bundle \"$bundle_path\" --receipt",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("promotion wrapper is missing %q", required)
		}
	}
	for _, forbidden := range []string{"rm -rf", "git clean", "git reset", "|| true"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("promotion wrapper contains unsafe or fail-open command %q", forbidden)
		}
	}
}

func TestPromotionWorkflowContractAndPinnedActions(t *testing.T) {
	root := repositoryRootForContractTest(t)
	workflowRoot := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(workflowRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || (filepath.Ext(entry.Name()) != ".yml" && filepath.Ext(entry.Name()) != ".yaml") {
			continue
		}
		path := filepath.Join(workflowRoot, entry.Name())
		workflow := readContractFile(t, path)
		matches := pinnedActionPattern.FindAllStringSubmatch(workflow, -1)
		for _, match := range matches {
			if !fullCommitPattern.MatchString(match[1]) {
				t.Errorf("%s contains action reference that is not a full commit SHA: %q", entry.Name(), match[1])
			}
		}
	}

	promotion := readContractFile(t, filepath.Join(workflowRoot, "kicad-promotion.yml"))
	for _, required := range []string{
		"workflow_dispatch:",
		"runs-on: macos-15",
		"timeout-minutes: 60",
		"run: make promotion-bundle",
		"kicadai-promotion' verify --bundle \"$bundle_path\"",
		"actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02",
		"name: kicadai-promotion-${{ github.sha }}",
		"path: ${{ runner.temp }}/kicadai-promotion/bundles/sha256-*",
		"if-no-files-found: error",
		"include-hidden-files: true",
	} {
		if !strings.Contains(promotion, required) {
			t.Errorf("installed promotion workflow is missing %q", required)
		}
	}
	for _, forbidden := range []string{"continue-on-error:", "if: false", "KICADAI_SKIP"} {
		if strings.Contains(promotion, forbidden) {
			t.Errorf("installed promotion workflow contains skip mechanism %q", forbidden)
		}
	}
}

func repositoryRootForContractTest(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func readContractFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
