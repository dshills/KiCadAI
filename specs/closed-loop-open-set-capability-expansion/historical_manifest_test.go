package closedloopopensetcontract

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path"
	"strings"
	"testing"
)

// historicalManifestNames validates a manifest whose own bytes are pinned by
// a later contract or publisher seal. It intentionally does not compare the
// recorded hashes with today's production files: later capability versions
// are allowed to evolve those files without rewriting historical evidence.
func historicalManifestNames(t *testing.T, manifestPath string) []string {
	t.Helper()
	manifest, err := os.Open(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	defer manifest.Close()

	var names []string
	seen := map[string]bool{}
	scanner := bufio.NewScanner(manifest)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) <= sha256.Size*2+2 || line[sha256.Size*2:sha256.Size*2+2] != "  " {
			t.Fatalf("invalid historical manifest entry %q", line)
		}
		digest, name := line[:sha256.Size*2], line[sha256.Size*2+2:]
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size || strings.ToLower(digest) != digest {
			t.Fatalf("invalid historical manifest digest %q", digest)
		}
		if strings.TrimSpace(name) != name || strings.ContainsAny(name, `\:`) || path.IsAbs(name) ||
			path.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") || seen[name] {
			t.Fatalf("unsafe or duplicate historical manifest path %q", name)
		}
		seen[name] = true
		names = append(names, name)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("historical manifest is empty")
	}
	return names
}
