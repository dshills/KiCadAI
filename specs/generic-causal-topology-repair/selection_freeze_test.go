package genericcausaltopologyrepair

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const populationFreezeSHA256V21 = "5809ba88980d3745727f3c0a3324cc987105c94a2a2a816baf54382d25ab70d1"

type publicTopologyFreezeV21 struct {
	Schema             string `json:"schema"`
	Version            int    `json:"version"`
	SourceReport       string `json:"source_report"`
	SourceReportSHA256 string `json:"source_report_sha256"`
	SourceReportHash   string `json:"source_report_hash"`
	Population         struct {
		SelectedCaseCount    int `json:"selected_case_count"`
		ReportingDomainCount int `json:"reporting_domain_count"`
		Causal               int `json:"causal_topology_repair_occurrences"`
		Complete             int `json:"complete_topology_occurrences"`
		Total                int `json:"total_topology_occurrences"`
	} `json:"population"`
	SelectedCases []struct {
		ReportingDomain string `json:"reporting_domain"`
		Causal          int    `json:"causal_topology_repair"`
		Complete        int    `json:"complete_topology"`
	} `json:"selected_cases"`
}

func TestV21SelectedPublicPopulationFreeze(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve selection-freeze test path")
	}
	testDirectory := filepath.Dir(testFile)
	data, err := os.ReadFile(filepath.Join(testDirectory, "V21_PUBLIC_TOPOLOGY_POPULATION.json"))
	if err != nil {
		t.Fatal(err)
	}
	// The protocol freezes exact repository bytes, including serialization.
	// Git's checked-in LF representation is therefore deliberately significant.
	digest := sha256.Sum256(data)
	if got := hex.EncodeToString(digest[:]); got != populationFreezeSHA256V21 {
		t.Fatalf("population freeze sha256 = %s, want %s", got, populationFreezeSHA256V21)
	}
	var freeze publicTopologyFreezeV21
	if err := json.Unmarshal(data, &freeze); err != nil {
		t.Fatal(err)
	}
	if freeze.Schema != "kicadai.public-causal-topology-population.v21" {
		t.Errorf("schema = %q, want %q", freeze.Schema, "kicadai.public-causal-topology-population.v21")
	}
	if freeze.Version != 21 {
		t.Errorf("version = %d, want 21", freeze.Version)
	}
	domains := map[string]bool{}
	causal, complete := 0, 0
	for _, selected := range freeze.SelectedCases {
		domains[selected.ReportingDomain] = true
		causal += selected.Causal
		complete += selected.Complete
	}
	if got := len(freeze.SelectedCases); got != freeze.Population.SelectedCaseCount {
		t.Errorf("selected cases = %d, want summary %d", got, freeze.Population.SelectedCaseCount)
	}
	if got := len(domains); got != freeze.Population.ReportingDomainCount {
		t.Errorf("reporting domains = %d, want summary %d", got, freeze.Population.ReportingDomainCount)
	}
	if causal != freeze.Population.Causal {
		t.Errorf("causal occurrences = %d, want summary %d", causal, freeze.Population.Causal)
	}
	if complete != freeze.Population.Complete {
		t.Errorf("complete occurrences = %d, want summary %d", complete, freeze.Population.Complete)
	}
	if causal+complete != freeze.Population.Total {
		t.Errorf("total occurrences = %d, want summary %d", causal+complete, freeze.Population.Total)
	}

	repositoryRoot := filepath.Clean(filepath.Join(testDirectory, "..", ".."))
	report := filepath.Join(repositoryRoot, filepath.FromSlash(freeze.SourceReport))
	reportFile, err := os.Open(report)
	if err != nil {
		t.Fatal(err)
	}
	defer reportFile.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reportFile); err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); got != freeze.SourceReportSHA256 {
		t.Fatalf("V20 report sha256 = %s, want %s", got, freeze.SourceReportSHA256)
	}
	if _, err := reportFile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	var reportEnvelope struct {
		Hash string `json:"hash"`
	}
	if err := json.NewDecoder(reportFile).Decode(&reportEnvelope); err != nil {
		t.Fatal(err)
	}
	if reportEnvelope.Hash != freeze.SourceReportHash {
		t.Errorf("V20 report content hash = %s, want %s", reportEnvelope.Hash, freeze.SourceReportHash)
	}
}
