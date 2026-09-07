package capabilityfeedback

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/corpuspublication"
)

// This read-only audit verifier must never be passed to an evaluation updater.
// Only old dependency and test-harness bytes can come from the archive.
// Production source, manifests, protocols, and corpus content stay live.
func verifyHistoricalAuditManifest(root, manifestPath string) ([]byte, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if len(line) < 67 || line[64:66] != "  " {
			return nil, fmt.Errorf("malformed historical checksum entry")
		}
		digest, relative := line[:64], line[66:]
		if !filepath.IsLocal(relative) || filepath.Clean(relative) != relative || strings.Contains(relative, "\\") {
			return nil, fmt.Errorf("unsafe historical checksum path")
		}
		path := filepath.Join(root, relative)
		if relative == "go.mod" || relative == "go.sum" || strings.HasSuffix(relative, "_test.go") {
			archive := filepath.Join(root, "specs", "closed-loop-open-set-capability-expansion", "historical-source", relative+".snapshot")
			if info, statErr := os.Lstat(archive); statErr == nil {
				if !info.Mode().IsRegular() {
					return nil, fmt.Errorf("historical snapshot is not regular")
				}
				path = archive
			} else if !os.IsNotExist(statErr) {
				return nil, statErr
			}
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, readErr
		}
		if corpusHash(source) != digest {
			return nil, fmt.Errorf("historical checksum mismatch: %s", relative)
		}
	}
	return data, nil
}

// Keep this in the existing V8 round-one CI lane, which materializes the
// frozen manifest's historical uppercase aliases on case-sensitive systems.
func TestClosedLoopV8Round1HistoricalAuditDoesNotAdmitCurrentBuild(t *testing.T) {
	root := closedLoopModuleRoot(t)
	for _, name := range []string{"V8_EVALUATOR.sha256", closedLoopV8Round1RunnerManifest} {
		path := filepath.Join(closedLoopSpecDirectory(t), name)
		if _, err := verifyHistoricalAuditManifest(root, path); err != nil {
			t.Fatal(err)
		}
		if _, err := corpuspublication.VerifyChecksumManifest(root, path); err == nil {
			t.Fatal("maintained source must not impersonate the frozen V8 environment")
		}
	}
}
