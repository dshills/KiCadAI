package promotionrunner

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestLintCoversHistoricalSourcesWithoutChangingFrozenBytes(t *testing.T) {
	root := repositoryRootForContractTest(t)
	makefile := readContractFile(t, filepath.Join(root, "Makefile"))
	if !strings.Contains(makefile, "golangci-lint run ./...;") {
		t.Fatal("local lint must cover the same packages as CI")
	}
	const paths = `^specs/(practical-board-completion-v1/engine/main\.go|ai-requirement-contract-integration/publication-live-v1/replay-audit/main\.go)$`
	const finding = "^Error return value of `f\\.Close` is not checked$"
	config := readContractFile(t, filepath.Join(root, ".golangci.yml"))
	if !strings.Contains(config, "- path: "+paths+"\n        text: "+finding+"\n        linters:\n          - errcheck") {
		t.Fatal("frozen audit exception must be limited to the two exact paths and f.Close errcheck finding")
	}
	for path, want := range map[string]string{
		"specs/practical-board-completion-v1/engine/main.go":                                 "a6f9bcfb270e59aebff2d16aec1013c875e131cb28e08715033fd9080ce46cc8",
		"specs/ai-requirement-contract-integration/publication-live-v1/replay-audit/main.go": "63cbbf0aa480596d828ef84ef155fb98ff4fdb53913425a0042620b284e40090",
	} {
		if got := fmt.Sprintf("%x", sha256.Sum256([]byte(readContractFile(t, filepath.Join(root, path))))); got != want {
			t.Errorf("frozen source %s changed: %s", path, got)
		}
		if !regexp.MustCompile(paths).MatchString(path) {
			t.Errorf("frozen source %s is not matched", path)
		}
		if regexp.MustCompile(paths).MatchString(path + ".successor") {
			t.Errorf("exception matches a successor of %s", path)
		}
	}
	if regexp.MustCompile(finding).MatchString("Error return value of `f.Write` is not checked") {
		t.Fatal("exception must not hide write errors")
	}
}
