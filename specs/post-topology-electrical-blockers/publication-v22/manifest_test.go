package v22publication

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/capabilitybaselinev10"
)

const publicationManifestSHA256 = "12f5e026bda420fc6a663f2d464986282aeff25eb8df356d9dee1ffecbcf612a"

// verifyManifest authenticates the complete expected inventory, not merely the
// files listed by a potentially incomplete checksum manifest. It rejects links
// before reading any retained file and never follows manifest-provided paths.
func verifyManifest(root string, expected []string, expectedHash string) error {
	want := slices.Clone(expected)
	slices.Sort(want)
	for i, name := range want {
		if name == "SHA256SUMS" || !fs.ValidPath(name) || strings.ContainsAny(name, "\\\r\n") || i > 0 && name == want[i-1] {
			return fmt.Errorf("invalid expected inventory")
		}
	}
	var actual []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("nonregular publication entry: %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative != "SHA256SUMS" {
			actual = append(actual, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		return err
	}
	slices.Sort(actual)
	if !slices.Equal(actual, want) {
		var missing, extra []string
		for _, name := range want {
			if !slices.Contains(actual, name) {
				missing = append(missing, name)
			}
		}
		for _, name := range actual {
			if !slices.Contains(want, name) {
				extra = append(extra, name)
			}
		}
		return fmt.Errorf("publication inventory differs: missing=%q extra=%q", missing, extra)
	}
	var canonical strings.Builder
	for _, name := range want {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		fmt.Fprintf(&canonical, "%s  %s\n", digest(data), name)
	}
	manifest, err := os.ReadFile(filepath.Join(root, "SHA256SUMS"))
	if err != nil {
		return err
	}
	if digest(manifest) != expectedHash {
		return fmt.Errorf("publication manifest commitment differs")
	}
	if string(manifest) != canonical.String() {
		return fmt.Errorf("publication checksum manifest differs")
	}
	return nil
}

func TestPublishedV22Inventory(t *testing.T) {
	if _, err := os.Stat(filepath.Join(publication, "report.json")); os.IsNotExist(err) {
		t.Skip("V22 public report not published yet")
	}
	var report capabilitybaselinev10.Report
	readJSON(t, filepath.Join(publication, "report.json"), &report)
	if err := capabilitybaselinev10.Validate(report); err != nil || report.CaseCount != 24 {
		t.Fatalf("invalid complete report: %v", err)
	}
	var predecessor capabilitybaselinev10.Report
	readJSON(t, repository+"/internal/capabilityfeedback/testdata/closed_loop_open_set_v21_maintenance_1/report.json", &predecessor)
	if err := capabilitybaselinev10.Validate(predecessor); err != nil || len(predecessor.Cases) != len(report.Cases) {
		t.Fatal("invalid predecessor inventory")
	}
	expected := []string{"report.json", "ASSESSMENT.json", "EVALUATION_SUMMARY.json", "SOURCE_COMMIT.txt", "GO_ENVIRONMENT.txt", "EVALUATOR.sha256", "PROCESS_METRICS.txt"}
	for i, record := range report.Cases {
		if record.Case.ID != predecessor.Cases[i].Case.ID {
			t.Fatal("public case identity differs from predecessor")
		}
		for replay := 1; replay <= 2; replay++ {
			prefix := fmt.Sprintf("replays/%s/replay-%d/", record.Case.ID, replay)
			for _, name := range []string{"CLEAN_ROOT.json", "ELECTRICAL_REPAIR.json", "METRICS.json"} {
				expected = append(expected, prefix+name)
			}
			if record.Case.Outcome == "pass" {
				expected = append(expected, prefix+"PROMOTION.json")
			}
		}
	}
	if err := verifyManifest(publication, expected, publicationManifestSHA256); err != nil {
		t.Fatal(err)
	}
}

func TestManifestRejectsIncompleteOrChangedEvidence(t *testing.T) {
	for _, mutation := range []string{"none", "changed", "rehashed", "missing", "extra", "duplicate", "traversal", "symlink", "manifest_symlink"} {
		t.Run(mutation, func(t *testing.T) {
			root := t.TempDir()
			write := func(name, value string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(root, name), []byte(value), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			remove := func(name string) {
				t.Helper()
				if err := os.Remove(filepath.Join(root, name)); err != nil {
					t.Fatal(err)
				}
			}
			link := func(name string) {
				t.Helper()
				remove(name)
				if err := os.Symlink(filepath.Join(root, "a.json"), filepath.Join(root, name)); err != nil {
					t.Fatal(err)
				}
			}
			manifest := digest([]byte("a")) + "  a.json\n" + digest([]byte("b")) + "  b.json\n"
			write("a.json", "a")
			write("b.json", "b")
			write("SHA256SUMS", manifest)
			switch mutation {
			case "changed":
				write("b.json", "changed")
			case "rehashed":
				write("b.json", "changed")
				write("SHA256SUMS", strings.ReplaceAll(manifest, digest([]byte("b")), digest([]byte("changed"))))
			case "missing":
				remove("b.json")
			case "extra":
				write("extra.json", "extra")
			case "duplicate":
				write("SHA256SUMS", manifest+manifest)
			case "traversal":
				write("SHA256SUMS", strings.ReplaceAll(manifest, "a.json", "../a.json"))
			case "symlink":
				link("b.json")
			case "manifest_symlink":
				link("SHA256SUMS")
			}
			err := verifyManifest(root, []string{"b.json", "a.json"}, digest([]byte(manifest)))
			if (err == nil) != (mutation == "none") {
				t.Fatalf("mutation=%s err=%v", mutation, err)
			}
		})
	}
}
