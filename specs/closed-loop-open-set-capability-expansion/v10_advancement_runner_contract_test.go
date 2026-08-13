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

func TestVersionTenAdvancementRunnerIsFrozenAndPublicOnly(t *testing.T) {
	directory := v7ContractDirectory(t)
	var freeze struct {
		Schema               string `json:"schema"`
		Version              int    `json:"version"`
		FreezeParentCommit   string `json:"freeze_parent_commit"`
		RunnerManifest       string `json:"runner_manifest"`
		RunnerManifestSHA256 string `json:"runner_manifest_sha256"`
		DiscoveryCases       int    `json:"discovery_case_count"`
		MinimumCases         int    `json:"minimum_advanced_cases"`
		MinimumDomains       int    `json:"minimum_advanced_domains"`
		MinimumRoles         int    `json:"minimum_advanced_roles"`
		MaximumRounds        int    `json:"maximum_rounds"`
		ProductionPath       bool   `json:"production_path"`
		ExactConfinement     bool   `json:"exact_effect_confinement"`
		RealSuccessorLoaded  bool   `json:"real_successor_loaded"`
		HeldOutAccess        bool   `json:"held_out_access_surface"`
	}
	if err := json.Unmarshal(v7ReadFile(t, filepath.Join(directory, "V10_ADVANCEMENT_RUNNER_FREEZE.json")), &freeze); err != nil {
		t.Fatal(err)
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-advancement-runner-freeze.v10" || freeze.Version != 10 || freeze.FreezeParentCommit != "5fa84f9ae8af0d9f127ab530d83d6969ad775c0a" || freeze.RunnerManifest != "V10_ADVANCEMENT_RUNNER.sha256" || freeze.RunnerManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.RunnerManifest)) {
		t.Fatalf("invalid V10 advancement freeze: %+v", freeze)
	}
	if freeze.DiscoveryCases != 24 || freeze.MinimumCases != 2 || freeze.MinimumDomains != 2 || freeze.MinimumRoles != 2 || freeze.MaximumRounds != 2 || !freeze.ProductionPath || !freeze.ExactConfinement || freeze.RealSuccessorLoaded || freeze.HeldOutAccess {
		t.Fatal("V10 advancement freeze crosses its public pre-round boundary")
	}
	v8VerifyManifest(t, directory, freeze.RunnerManifest)
	manifest := string(v7ReadFile(t, filepath.Join(directory, freeze.RunnerManifest)))
	production := 0
	commandFound := false
	for _, line := range strings.Split(strings.TrimSpace(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !strings.HasSuffix(fields[1], ".go") || strings.HasSuffix(fields[1], "_test.go") {
			continue
		}
		relative := fields[1]
		if strings.Contains(relative, "cmd/kicadai-capability-advance-v10/main.go") {
			commandFound = true
		}
		if !strings.Contains(relative, "capabilityadvancementv10/") && !strings.Contains(relative, "cmd/kicadai-capability-advance-v10/") {
			t.Fatalf("unexpected advancement production source %q", relative)
		}
		production++
		source := v7ReadFile(t, filepath.Join(directory, filepath.FromSlash(relative)))
		for _, token := range []string{"OpenHeldOutV10", "VerifyPublicationV10WithKey", "blindbaseline", "externalkey"} {
			if strings.Contains(string(source), token) {
				t.Fatalf("advancement source %s contains %q", relative, token)
			}
		}
		parsed, err := parser.ParseFile(tokenpkg(), relative, source, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			name, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if name == "kicadai/internal/blindbaseline" || name == "kicadai/internal/externalkey" {
				t.Fatalf("forbidden advancement import %q", name)
			}
		}
	}
	if production != 3 || !commandFound {
		t.Fatalf("advancement production sources=%d command=%t", production, commandFound)
	}
}

func tokenpkg() *token.FileSet { return token.NewFileSet() }
