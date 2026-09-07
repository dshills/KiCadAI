package closedloopopensetcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Historical dependency and test-harness snapshots are only used by audit
// tests. Production verification must continue reading the live source and
// refusing a build whose environment differs from its frozen manifest.
func historicalSourcePath(t *testing.T, path string) string {
	t.Helper()
	root := filepath.Clean(filepath.Join(v7ContractDirectory(t), "..", ".."))
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return path
	}
	// Never substitute production source, a protocol, a manifest, or a corpus.
	if relative != "go.mod" && relative != "go.sum" && !strings.HasSuffix(relative, "_test.go") {
		return path
	}
	archived := filepath.Join(v7ContractDirectory(t), "historical-source", relative+".snapshot")
	info, err := os.Lstat(archived)
	if os.IsNotExist(err) {
		return path
	}
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("historical source must be a regular file: %s", archived)
	}
	return archived
}

func TestHistoricalDependenciesAndAuditHarness(t *testing.T) {
	root := filepath.Clean(filepath.Join(v7ContractDirectory(t), "..", ".."))
	want := map[string]string{
		"go.mod": "a5a21e337278bd0c2f0b8f299d2eb561507ad3e2e7794b295b07a8e2c5290939",
		"go.sum": "6eee5c8722b41ef5d1a156f9814ac865807af4486a9a46268635d1991cc6c3a6",
	}
	for path, digest := range want {
		if actual := v7FileSHA256(t, filepath.Join(root, path)); actual != digest {
			t.Fatalf("historical %s changed: %s", path, actual)
		}
	}
	for _, name := range []string{"V10_VALIDATOR.sha256", "V10_PUBLISHER.sha256"} {
		data := v7ReadFile(t, filepath.Join(v7ContractDirectory(t), name))
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if len(line) < 67 || line[64:66] != "  " {
				t.Fatalf("malformed historical manifest %s", name)
			}
			if actual := v7FileSHA256(t, filepath.Join(root, filepath.FromSlash(line[66:]))); actual != line[:64] {
				t.Fatalf("historical source drift in %s: %s", name, line[66:])
			}
		}
	}
	// Live verification still sees the new dependency bytes, not the archive.
	for path, old := range want {
		data := v7ReadFile(t, filepath.Join(root, path))
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) == old {
			t.Fatalf("security maintenance must update live %s", path)
		}
	}
	for _, path := range []string{"internal/corpuspublication/checksum.go", "specs/closed-loop-open-set-capability-expansion/V10_VALIDATOR.sha256"} {
		live := filepath.Join(root, filepath.FromSlash(path))
		if historicalSourcePath(t, live) != live {
			t.Fatal("production source or manifest substituted")
		}
	}
}
