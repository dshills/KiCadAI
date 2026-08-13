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

func TestVersionTenProductionSelectionRunnerIsFrozenAndPublicOnly(t *testing.T) {
	directory := v7ContractDirectory(t)
	var freeze struct {
		Schema                       string `json:"schema"`
		Version                      int    `json:"version"`
		FreezeParentCommit           string `json:"freeze_parent_commit"`
		RunnerManifest               string `json:"runner_manifest"`
		RunnerManifestSHA256         string `json:"runner_manifest_sha256"`
		EffectExposureEngineSHA256   string `json:"effect_exposure_engine_manifest_sha256"`
		DiscoveryCaseCount           int    `json:"discovery_case_count"`
		MaximumPlanSetBytes          int    `json:"maximum_plan_set_bytes"`
		ProductionPath               bool   `json:"production_path"`
		StaticEffectEvidenceRequired bool   `json:"static_effect_evidence_required"`
		RealFrontierLoaded           bool   `json:"real_frontier_loaded"`
		HeldOutAccessSurface         bool   `json:"held_out_access_surface"`
	}
	if err := json.Unmarshal(v7ReadFile(t, filepath.Join(directory, "V10_SELECTION_RUNNER_FREEZE.json")), &freeze); err != nil {
		t.Fatal(err)
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-selection-runner-freeze.v10" || freeze.Version != 10 ||
		freeze.FreezeParentCommit != "5a39fadd319961d2e4eb911b24965765f27d1294" ||
		freeze.RunnerManifest != "V10_SELECTION_RUNNER.sha256" ||
		freeze.RunnerManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.RunnerManifest)) ||
		freeze.EffectExposureEngineSHA256 != "e8f2e796efe5a10a6c2d88039685112b9e4efa2a43dd607bf7b3174b8dce6c2e" {
		t.Fatalf("invalid V10 selection-runner freeze: %+v", freeze)
	}
	if freeze.DiscoveryCaseCount != 24 || freeze.MaximumPlanSetBytes != 32<<20 || !freeze.ProductionPath ||
		!freeze.StaticEffectEvidenceRequired || freeze.RealFrontierLoaded || freeze.HeldOutAccessSurface {
		t.Fatal("V10 selection-runner freeze crosses its public pre-selection boundary")
	}
	v8VerifyManifest(t, directory, freeze.RunnerManifest)
	assertV10SelectionRunnerHasNoHeldOutAccess(t, directory, freeze.RunnerManifest)
}

func assertV10SelectionRunnerHasNoHeldOutAccess(t *testing.T, directory, manifestName string) {
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
		if strings.Contains(relative, "cmd/kicadai-capability-rank-v10/main.go") {
			commandFound = true
		}
		if !strings.Contains(relative, "capabilityselectionv10/") && !strings.Contains(relative, "cmd/kicadai-capability-rank-v10/") {
			t.Fatalf("V10 selection manifest contains unexpected production source %q", relative)
		}
		productionSources++
		source := v7ReadFile(t, filepath.Join(directory, filepath.FromSlash(relative)))
		for _, prohibited := range []string{"OpenHeldOutV10", "VerifyPublicationV10WithKey", "blindbaseline", "externalkey"} {
			if strings.Contains(string(source), prohibited) {
				t.Fatalf("V10 public selection source %s contains prohibited token %q", relative, prohibited)
			}
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
				t.Fatalf("V10 public selection source %s imports forbidden package %q", relative, path)
			}
		}
	}
	if productionSources != 3 || !commandFound {
		t.Fatalf("V10 selection production sources = %d, command found = %t", productionSources, commandFound)
	}
}
