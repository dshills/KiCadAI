package closedloopopensetcontract

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/capabilityroundsv9"
)

func TestVersionNineEffectExposureEngineIsFrozenAndCorpusBlind(t *testing.T) {
	directory := v7ContractDirectory(t)
	var freeze struct {
		Schema               string `json:"schema"`
		Version              int    `json:"version"`
		FreezeCommitParent   string `json:"freeze_commit_parent"`
		EngineManifest       string `json:"engine_manifest"`
		EngineManifestSHA256 string `json:"engine_manifest_sha256"`
		DiscoveryCount       int    `json:"discovery_count"`
		MaximumRounds        int    `json:"maximum_rounds"`
		MaximumTotalAtoms    int    `json:"maximum_total_atoms"`
		MaximumTotalMembers  int    `json:"maximum_total_members"`
		RealFrontierLoaded   bool   `json:"real_frontier_loaded"`
		HeldOutOpened        bool   `json:"held_out_opened"`
	}
	if err := json.Unmarshal(v7ReadFile(t, filepath.Join(directory, "V9_EFFECT_EXPOSURE_ENGINE_FREEZE.json")), &freeze); err != nil {
		t.Fatal(err)
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-effect-exposure-freeze.v9" || freeze.Version != 9 {
		t.Fatalf("V9 exposure freeze schema/version = %q/%d", freeze.Schema, freeze.Version)
	}
	if freeze.FreezeCommitParent != "5a09283f69ead9915a08ee32d9db0da234cf6149" {
		t.Fatalf("V9 exposure freeze parent = %q", freeze.FreezeCommitParent)
	}
	if freeze.EngineManifest != "V9_EFFECT_EXPOSURE_ENGINE.sha256" ||
		freeze.EngineManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.EngineManifest)) {
		t.Fatal("V9 exposure engine manifest binding is invalid")
	}
	if freeze.DiscoveryCount != 24 || freeze.MaximumRounds != 2 || freeze.MaximumTotalAtoms != 6 || freeze.MaximumTotalMembers != 18 {
		t.Fatalf("V9 exposure freeze bounds = %d/%d/%d/%d", freeze.DiscoveryCount, freeze.MaximumRounds, freeze.MaximumTotalAtoms, freeze.MaximumTotalMembers)
	}
	if freeze.RealFrontierLoaded || freeze.HeldOutOpened {
		t.Fatal("V9 exposure preparation claims real frontier or held-out access")
	}
	v8VerifyManifest(t, directory, freeze.EngineManifest)
	policy := capabilityroundsv9.FrozenPolicy()
	if policy.ExpectedDiscoveryCases != 24 || policy.MaximumRounds != 2 || policy.MaximumTotalAtoms != 6 || policy.MaximumTotalMembers != 18 ||
		policy.MaximumRoundAtoms != 3 || policy.MaximumRoundMembers != 9 || policy.MinimumCaseSupport != 2 ||
		policy.MinimumAdvancedCases != 2 || policy.MinimumDomains != 2 || policy.MinimumRoles != 2 || policy.MaximumSuccessors != 4 {
		t.Fatalf("V9 exposure runtime policy differs from freeze: %+v", policy)
	}
	assertV9ExposureManifestImportsAreBlind(t, directory, freeze.EngineManifest)
}

func assertV9ExposureManifestImportsAreBlind(t *testing.T, directory, manifestName string) {
	t.Helper()
	manifest := string(v7ReadFile(t, filepath.Join(directory, manifestName)))
	productionSources := 0
	for _, line := range strings.Split(strings.TrimSpace(manifest), "\n") {
		if len(line) < 67 {
			t.Fatalf("malformed V9 exposure manifest line %q", line)
		}
		relative := line[66:]
		if !strings.HasPrefix(relative, "../../internal/capabilityroundsv9/") || !strings.HasSuffix(relative, ".go") || strings.HasSuffix(relative, "_test.go") {
			continue
		}
		productionSources++
		source := v7ReadFile(t, filepath.Join(directory, filepath.FromSlash(relative)))
		file, err := parser.ParseFile(token.NewFileSet(), relative, source, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse V9 exposure source %s: %v", relative, err)
		}
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("decode V9 exposure import in %s: %v", relative, err)
			}
			if strings.HasPrefix(path, "kicadai/") {
				t.Fatalf("V9 exposure engine %s imports non-stdlib package %q", relative, path)
			}
		}
	}
	if productionSources != 4 {
		t.Fatalf("V9 exposure manifest names %d production Go sources, want 4", productionSources)
	}
}
