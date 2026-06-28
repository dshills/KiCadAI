package amplifiers

import (
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/kicadfiles/design"
)

func TestExistingClassABHeadphoneAmpFixtureParses(t *testing.T) {
	root := repoPath(t, "examples", "06_class_ab_headphone_amp")
	project, err := design.ReadProjectDirectory(root)
	if err != nil {
		t.Fatalf("read amplifier project: %v", err)
	}
	schematicFile := project.Schematic
	if schematicFile == nil {
		t.Fatalf("schematic missing")
	}
	if validation := ValidateSchematicLandmarks(schematicFile, ClassABHeadphoneAmpLandmarks()); !validation.OK() {
		t.Fatalf("%s", validation)
	}
}

func repoPath(t *testing.T, parts ...string) string {
	t.Helper()
	root := os.Getenv("KICADAI_REPO_ROOT")
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("get working directory: %v", err)
		}
		root = wd
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("resolve repository search root %s: %v", root, err)
	}
	root = absRoot
	start := root
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		} else if !os.IsNotExist(err) {
			t.Fatalf("check repository marker %s: %v", filepath.Join(root, "go.mod"), err)
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatalf("repository root (go.mod) not found starting from %s", start)
		}
		root = next
	}
	items := append([]string{root}, parts...)
	return filepath.Join(items...)
}
