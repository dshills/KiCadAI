package closedloopopensetcontract

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestVersionTenProductionEvaluatorIsFrozenAndPublicOnly(t *testing.T) {
	directory := v7ContractDirectory(t)
	var freeze struct {
		Schema                  string `json:"schema"`
		Version                 int    `json:"version"`
		FreezeParentCommit      string `json:"freeze_parent_commit"`
		EvaluatorManifest       string `json:"evaluator_manifest"`
		EvaluatorManifestSHA256 string `json:"evaluator_manifest_sha256"`
		DiscoveryCaseCount      int    `json:"discovery_case_count"`
		ReplaysPerCase          int    `json:"replays_per_case"`
		MaximumParallelCases    int    `json:"maximum_parallel_cases"`
		OuterPromotionsPerPass  int    `json:"outer_promotions_per_pass"`
		ProductionPath          bool   `json:"production_path"`
		HeldOutAccessSurface    bool   `json:"held_out_access_surface"`
		RealCorpusEvaluated     bool   `json:"real_corpus_evaluated"`
		ExternalKeyOpened       bool   `json:"external_key_opened"`
	}
	if err := json.Unmarshal(v7ReadFile(t, filepath.Join(directory, "V10_EVALUATOR_FREEZE.json")), &freeze); err != nil {
		t.Fatal(err)
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-evaluator-freeze.v10" || freeze.Version != 10 ||
		freeze.FreezeParentCommit != "cb887a8146d26922a1ebe7e4f3d2cb9ea51faf2c" ||
		freeze.EvaluatorManifest != "V10_EVALUATOR.sha256" ||
		freeze.EvaluatorManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.EvaluatorManifest)) {
		t.Fatalf("invalid V10 evaluator freeze: %+v", freeze)
	}
	if freeze.DiscoveryCaseCount != 24 || freeze.ReplaysPerCase != 2 || freeze.MaximumParallelCases != 4 || freeze.OuterPromotionsPerPass != 2 ||
		!freeze.ProductionPath || freeze.HeldOutAccessSurface || freeze.RealCorpusEvaluated || freeze.ExternalKeyOpened {
		t.Fatal("V10 evaluator freeze crosses its public pre-evaluation boundary")
	}
	v8VerifyManifest(t, directory, freeze.EvaluatorManifest)
	assertV10EvaluatorHasNoHeldOutAccess(t, directory, freeze.EvaluatorManifest)
}

func assertV10EvaluatorHasNoHeldOutAccess(t *testing.T, directory, manifestName string) {
	t.Helper()
	manifest := string(v7ReadFile(t, filepath.Join(directory, manifestName)))
	productionSources := 0
	commandFound := false
	for _, line := range strings.Split(strings.TrimSpace(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !strings.HasSuffix(fields[1], ".go") || strings.HasSuffix(fields[1], "_test.go") {
			continue
		}
		relative := fields[1]
		if strings.Contains(relative, "cmd/kicadai-discovery-baseline-v10/main.go") {
			commandFound = true
		}
		if !strings.Contains(relative, "capabilityexecutorv10/") && !strings.Contains(relative, "cmd/kicadai-discovery-baseline-v10/") {
			t.Fatalf("V10 evaluator manifest contains unexpected production source %q", relative)
		}
		productionSources++
		source := v7ReadFile(t, filepath.Join(directory, filepath.FromSlash(relative)))
		if strings.Contains(string(source), "OpenHeldOutV10") || strings.Contains(string(source), "VerifyPublicationV10WithKey") {
			t.Fatalf("V10 public evaluator %s contains a held-out opening surface", relative)
		}
		file, err := parser.ParseFile(token.NewFileSet(), relative, source, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if path == "kicadai/internal/blindbaseline" || path == "kicadai/internal/externalkey" {
				t.Fatalf("V10 public evaluator %s imports forbidden package %q", relative, path)
			}
		}
	}
	if productionSources != 6 || !commandFound {
		t.Fatalf("V10 evaluator production sources = %d, command found = %t", productionSources, commandFound)
	}
}
