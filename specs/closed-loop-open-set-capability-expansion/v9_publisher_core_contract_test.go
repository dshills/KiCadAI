package closedloopopensetcontract

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestVersionNinePublisherCoreIsOutcomeNeutralAndFrozen(t *testing.T) {
	directory := v7ContractDirectory(t)
	var freeze struct {
		Schema                      string `json:"schema"`
		Version                     int    `json:"version"`
		FreezeCommitParent          string `json:"freeze_commit_parent"`
		PublisherCoreManifest       string `json:"publisher_core_manifest"`
		PublisherCoreManifestSHA256 string `json:"publisher_core_manifest_sha256"`
		CaseCount                   int    `json:"case_count"`
		DiscoveryCount              int    `json:"discovery_count"`
		HeldOutCount                int    `json:"held_out_count"`
		RealKeyOpened               bool   `json:"real_key_opened"`
		RealCorpusEvaluated         bool   `json:"real_corpus_evaluated"`
	}
	if err := json.Unmarshal(v7ReadFile(t, filepath.Join(directory, "V9_PUBLISHER_CORE_FREEZE.json")), &freeze); err != nil {
		t.Fatal(err)
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-publisher-core-freeze.v9" || freeze.Version != 9 {
		t.Fatalf("V9 publisher core schema/version = %q/%d", freeze.Schema, freeze.Version)
	}
	if freeze.FreezeCommitParent != "36492bb0609c77a4f789fc30552549fdd1cf83cb" {
		t.Fatalf("V9 publisher core parent = %q", freeze.FreezeCommitParent)
	}
	if freeze.PublisherCoreManifest != "V9_PUBLISHER_CORE.sha256" ||
		freeze.PublisherCoreManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.PublisherCoreManifest)) {
		t.Fatal("V9 publisher core manifest binding is invalid")
	}
	if freeze.CaseCount != 48 || freeze.DiscoveryCount != 24 || freeze.HeldOutCount != 24 {
		t.Fatalf("V9 publisher core counts = %d/%d/%d", freeze.CaseCount, freeze.DiscoveryCount, freeze.HeldOutCount)
	}
	if freeze.RealKeyOpened || freeze.RealCorpusEvaluated {
		t.Fatal("V9 publisher core preparation claims real key or corpus access")
	}
	v8VerifyManifest(t, directory, freeze.PublisherCoreManifest)
	manifest := string(v7ReadFile(t, filepath.Join(directory, freeze.PublisherCoreManifest)))
	v9Sources := []string{}
	for _, line := range strings.Split(strings.TrimSpace(manifest), "\n") {
		if len(line) < 67 {
			t.Fatalf("malformed V9 publisher core manifest line %q", line)
		}
		relative := line[66:]
		if strings.HasPrefix(relative, "../../internal/corpuspublication/v9") && strings.HasSuffix(relative, ".go") {
			v9Sources = append(v9Sources, relative)
		}
	}
	if len(v9Sources) != 7 {
		t.Fatalf("V9 publisher core manifest names %d V9 Go sources, want 7", len(v9Sources))
	}
	for _, name := range v9Sources {
		assertV9PublisherSourceOutcomeNeutral(t, name, v7ReadFile(t, filepath.Join(directory, filepath.FromSlash(name))))
	}
}

func assertV9PublisherSourceOutcomeNeutral(t *testing.T, name string, source []byte) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, source, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse frozen V9 publisher source %s: %v", name, err)
	}
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatalf("decode import in %s: %v", name, err)
		}
		for _, forbidden := range []string{"kicadai/internal/closedloopsynthesis", "kicadai/internal/capabilityfeedback", "kicadai/internal/capabilityrounds", "kicadai/internal/capabilitybundles"} {
			if path == forbidden || strings.HasPrefix(path, forbidden+"/") {
				t.Fatalf("V9 publisher core %s imports forbidden outcome package %q", name, path)
			}
		}
	}
	forbiddenCalls := map[string]bool{"synthesize": true, "simulate": true, "classify": true, "rank": true}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		called := ""
		switch function := call.Fun.(type) {
		case *ast.Ident:
			called = function.Name
		case *ast.SelectorExpr:
			called = function.Sel.Name
		}
		if forbiddenCalls[strings.ToLower(called)] {
			t.Errorf("V9 publisher core %s calls forbidden outcome function %q", name, called)
		}
		return true
	})
}
