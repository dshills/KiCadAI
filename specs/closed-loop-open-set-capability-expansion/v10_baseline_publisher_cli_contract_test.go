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

func TestVersionTenBaselinePublisherCLIIsFrozenAndPublicOnly(t *testing.T) {
	directory := v7ContractDirectory(t)
	var freeze struct {
		Schema                  string `json:"schema"`
		Version                 int    `json:"version"`
		FreezeParentCommit      string `json:"freeze_parent_commit"`
		CLIManifest             string `json:"cli_manifest"`
		CLIManifestSHA256       string `json:"cli_manifest_sha256"`
		PublisherManifestSHA256 string `json:"publisher_manifest_sha256"`
		DiscoveryCaseCount      int    `json:"discovery_case_count"`
		ProductionPath          bool   `json:"production_path"`
		HeldOutAccess           bool   `json:"held_out_access_surface"`
		RealReportLoaded        bool   `json:"real_report_loaded"`
	}
	if err := json.Unmarshal(v7ReadFile(t, filepath.Join(directory, "V10_BASELINE_PUBLISHER_CLI_FREEZE.json")), &freeze); err != nil {
		t.Fatal(err)
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-baseline-publisher-cli-freeze.v10" || freeze.Version != 10 || freeze.FreezeParentCommit != "592ce44b630a1fcd9375c9604b0e1d24f4f6c4a4" || freeze.CLIManifest != "V10_BASELINE_PUBLISHER_CLI.sha256" || freeze.CLIManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.CLIManifest)) || freeze.PublisherManifestSHA256 != "ac59be26ba83f8f8e8e3268ca732a3646c73d761446d0bd5c39bbba96029eab9" || freeze.DiscoveryCaseCount != 24 || !freeze.ProductionPath || freeze.HeldOutAccess || freeze.RealReportLoaded {
		t.Fatalf("invalid V10 baseline publisher CLI freeze: %+v", freeze)
	}
	v8VerifyManifest(t, directory, freeze.CLIManifest)
	manifest := string(v7ReadFile(t, filepath.Join(directory, freeze.CLIManifest)))
	production := 0
	for _, line := range strings.Split(strings.TrimSpace(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !strings.HasSuffix(fields[1], ".go") || strings.HasSuffix(fields[1], "_test.go") {
			continue
		}
		production++
		relative := fields[1]
		if !strings.Contains(relative, "cmd/kicadai-baseline-publish-v10/main.go") {
			t.Fatalf("unexpected baseline CLI production source %q", relative)
		}
		source := v7ReadFile(t, filepath.Join(directory, filepath.FromSlash(relative)))
		for _, prohibited := range []string{"OpenHeldOutV10", "VerifyPublicationV10WithKey", "blindbaseline", "externalkey"} {
			if strings.Contains(string(source), prohibited) {
				t.Fatalf("baseline CLI contains %q", prohibited)
			}
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), relative, source, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			name, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if name == "kicadai/internal/blindbaseline" || name == "kicadai/internal/externalkey" {
				t.Fatalf("forbidden baseline CLI import %q", name)
			}
		}
	}
	if production != 1 {
		t.Fatalf("baseline CLI production sources=%d", production)
	}
}
