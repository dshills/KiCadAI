package behavioralcontractrealizability

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

func TestContractIsFrozen(t *testing.T) {
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
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 || seen[fields[1]] || filepath.Base(fields[1]) != fields[1] {
			t.Fatalf("invalid frozen contract entry %q", scanner.Text())
		}
		data, err := os.ReadFile(filepath.Join(directory, fields[1]))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != fields[0] {
			t.Fatalf("%s hash = %s, want frozen %s", fields[1], got, fields[0])
		}
		seen[fields[1]] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"SPEC.md", "PLAN.md"} {
		if !seen[required] {
			t.Fatalf("frozen contract omits %s", required)
		}
	}
}
