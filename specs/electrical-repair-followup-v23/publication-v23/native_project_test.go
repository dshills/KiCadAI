package v23publication

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"kicadai/internal/capabilitybaselinev10"
	ot "kicadai/internal/opentopologysynthesis"
)

// Mirrors the frozen physicalPromotionProjectHash byte framing. Unlike the
// producer, this post-run verifier explicitly rejects nonregular entries.
func retainedProjectHash(root string) (string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".evidence" {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("nonregular retained project entry: %s", path)
		}
		switch filepath.Ext(path) {
		case ".kicad_pro", ".kicad_sch", ".kicad_pcb":
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(paths) < 3 {
		return "", fmt.Errorf("incomplete retained project")
	}
	slices.Sort(paths)
	sum := sha256.New()
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		sum.Write([]byte(filepath.ToSlash(relative)))
		sum.Write([]byte{0})
		sum.Write(data)
		sum.Write([]byte{0})
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func TestRetainedNativeV23Projects(t *testing.T) {
	root := os.Getenv("KICADAI_V23_AUDIT_SCRATCH")
	if root == "" {
		t.Skip("optional original native project byte verification")
	}
	var predecessor capabilitybaselinev10.Report
	readJSON(t, repository+"/internal/capabilityfeedback/testdata/closed_loop_open_set_v22_public_1/report.json", &predecessor)
	if err := capabilitybaselinev10.Validate(predecessor); err != nil {
		t.Fatal(err)
	}
	projects := 0
	for i, prior := range predecessor.Cases {
		id := fmt.Sprintf("v10_case_%03d", i+1)
		if id != prior.Case.ID {
			t.Fatal("invalid public identity")
		}
		checkpoint := filepath.Join(root, "checkpoints", id+".json")
		if _, err := os.Stat(checkpoint); os.IsNotExist(err) {
			continue
		}
		var record capabilitybaselinev10.CaseEvidence
		readJSON(t, checkpoint, &record)
		validated, err := capabilitybaselinev10.ValidateCase(record)
		if err != nil || validated.Hash != record.Hash || record.Case.ID != id {
			t.Fatal("invalid native-project checkpoint")
		}
		if record.Case.Outcome != "pass" {
			continue
		}
		for replay := 1; replay <= 2; replay++ {
			replayRoot := filepath.Join(root, id, fmt.Sprintf("replay-%d", replay))
			var promotion ot.PhysicalPromotionResult
			readJSON(t, filepath.Join(replayRoot, "PROMOTION.json"), &promotion)
			var electrical sidecar
			readJSON(t, filepath.Join(replayRoot, "ELECTRICAL_REPAIR.json"), &electrical)
			var metrics replayMetrics
			readJSON(t, filepath.Join(replayRoot, "METRICS.json"), &metrics)
			if err := authenticatePromotion(promotion, electrical.SynthesisHash, &record.Promotions[replay-1], metrics.PromotionNanoseconds); err != nil {
				t.Fatal(err)
			}
			for _, run := range promotion.Runs {
				projectRoot := filepath.Join(replayRoot, "promotion", fmt.Sprintf("run-%d", run.Number))
				actual, err := retainedProjectHash(projectRoot)
				if err != nil || actual != run.ProjectHash {
					t.Fatalf("native project bytes differ in %s: %v", projectRoot, err)
				}
				projects++
			}
		}
	}
	if projects == 0 {
		t.Fatal("no completed native projects authenticated")
	}
	t.Logf("authenticated_native_projects=%d", projects)
}

func TestRetainedProjectHashRejectsChanges(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.kicad_pro", "a.kicad_sch", "a.kicad_pcb"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := retainedProjectHash(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.kicad_sch"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := retainedProjectHash(root); err != nil || got == want {
		t.Fatal("native file mutation was not detected")
	}
	if err := os.Symlink(filepath.Join(root, "a.kicad_sch"), filepath.Join(root, "linked.kicad_sch")); err != nil {
		t.Fatal(err)
	}
	if _, err := retainedProjectHash(root); err == nil {
		t.Fatal("linked project file was accepted")
	}
}
