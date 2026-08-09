package closedloopopensetcontract

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVersionOneContractIsFrozen(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate contract test source")
	}
	directory := filepath.Dir(sourceFile)
	manifest, err := os.Open(filepath.Join(directory, "CONTRACT.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	defer manifest.Close()

	seen := map[string]bool{}
	scanner := bufio.NewScanner(manifest)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			t.Fatalf("invalid frozen contract checksum line %q", scanner.Text())
		}
		want, name := fields[0], fields[1]
		if len(want) != sha256.Size*2 || seen[name] || filepath.Base(name) != name {
			t.Fatalf("invalid frozen contract entry %q", scanner.Text())
		}
		data, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Fatalf("%s hash = %s, want frozen %s", name, got, want)
		}
		seen[name] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"SPEC.md", "PLAN.md", "CORPUS_RULES.md", "BASELINE_PROTOCOL.md"} {
		if !seen[required] {
			t.Fatalf("frozen contract omits %s", required)
		}
	}
}
