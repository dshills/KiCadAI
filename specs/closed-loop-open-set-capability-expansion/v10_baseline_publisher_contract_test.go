package closedloopopensetcontract

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/capabilitybaselinepublicationv10"
)

func TestVersionTenBaselinePublisherIsFrozenAndPublicOnly(t *testing.T) {
	directory := v7ContractDirectory(t)
	var freeze struct {
		Schema               string `json:"schema"`
		Version              int    `json:"version"`
		FreezeParentCommit   string `json:"freeze_parent_commit"`
		PublisherManifest    string `json:"publisher_manifest"`
		PublisherSHA256      string `json:"publisher_manifest_sha256"`
		DiscoveryCount       int    `json:"discovery_count"`
		HeldOutAccessSurface bool   `json:"held_out_access_surface"`
		RealCorpusEvaluated  bool   `json:"real_corpus_evaluated"`
		ExternalKeyOpened    bool   `json:"external_key_opened"`
	}
	if err := json.Unmarshal(v7ReadFile(t, filepath.Join(directory, "V10_BASELINE_PUBLISHER_FREEZE.json")), &freeze); err != nil {
		t.Fatal(err)
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-baseline-publisher-freeze.v10" || freeze.Version != 10 ||
		freeze.FreezeParentCommit != "aa39fae8a5adcc4a040f72469dcc401bf4a5c71e" {
		t.Fatalf("invalid V10 baseline publisher freeze: %+v", freeze)
	}
	if freeze.PublisherManifest != "V10_BASELINE_PUBLISHER.sha256" ||
		freeze.PublisherSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.PublisherManifest)) {
		t.Fatal("V10 baseline publisher manifest binding is invalid")
	}
	if freeze.DiscoveryCount != 24 || capabilitybaselinepublicationv10.ExpectedCases != 24 ||
		freeze.HeldOutAccessSurface || freeze.RealCorpusEvaluated || freeze.ExternalKeyOpened {
		t.Fatal("V10 baseline publisher freeze crosses its public-only preparation boundary")
	}
	v8VerifyManifest(t, directory, freeze.PublisherManifest)
	assertV10BaselinePublisherImportsArePublicOnly(t, directory, freeze.PublisherManifest)
}

func assertV10BaselinePublisherImportsArePublicOnly(t *testing.T, directory, manifestName string) {
	t.Helper()
	manifest := string(v7ReadFile(t, filepath.Join(directory, manifestName)))
	productionSources := 0
	for _, line := range strings.Split(strings.TrimSpace(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("malformed V10 baseline publisher manifest line %q", line)
		}
		relative := fields[1]
		if !strings.HasSuffix(relative, ".go") {
			continue
		}
		if !strings.HasPrefix(relative, "../../internal/capabilitybaselinepublicationv10/") {
			t.Fatalf("V10 baseline publisher manifest contains out-of-package Go source %q", relative)
		}
		if strings.HasSuffix(relative, "_test.go") {
			continue
		}
		productionSources++
		source := v7ReadFile(t, filepath.Join(directory, filepath.FromSlash(relative)))
		file, err := parser.ParseFile(token.NewFileSet(), relative, source, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(path, "kicadai/") && path != "kicadai/internal/atomicdir" && path != "kicadai/internal/capabilitybaselinev10" {
				t.Fatalf("V10 public baseline publisher %s imports forbidden package %q", relative, path)
			}
		}
	}
	if productionSources != 4 {
		t.Fatalf("V10 baseline publisher manifest names %d production Go sources, want 4", productionSources)
	}
}
