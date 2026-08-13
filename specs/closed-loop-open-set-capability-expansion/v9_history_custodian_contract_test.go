package closedloopopensetcontract

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestV9HistoryCustodianIsFrozenAndOutcomeNeutral(t *testing.T) {
	directory := v7ContractDirectory(t)
	data := v7ReadFile(t, filepath.Join(directory, "V9_HISTORY_CUSTODIAN_FREEZE.json"))
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var freeze struct {
		Schema                    string `json:"schema"`
		Version                   int    `json:"version"`
		CustodianManifestSHA256   string `json:"custodian_manifest_sha256"`
		PredecessorHistorySHA256  string `json:"predecessor_history_sha256"`
		V8CorpusChecksumsSHA256   string `json:"v8_corpus_checksums_sha256"`
		V8PublisherManifestSHA256 string `json:"v8_publisher_manifest_sha256"`
		V9ContractManifestSHA256  string `json:"v9_contract_manifest_sha256"`
		HeldOutOpened             bool   `json:"held_out_opened"`
	}
	if err := decoder.Decode(&freeze); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatal("V9 history custodian freeze has trailing JSON")
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-history-custodian-freeze.v9" || freeze.Version != 9 || freeze.HeldOutOpened ||
		freeze.CustodianManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, "V9_HISTORY_CUSTODIAN.sha256")) ||
		freeze.PredecessorHistorySHA256 != v7FileSHA256(t, filepath.Join(directory, "V8_HISTORICAL_COMMITMENTS.json")) ||
		freeze.V8CorpusChecksumsSHA256 != v7FileSHA256(t, filepath.Join(directory, "..", "..", "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v8_corpus", "CHECKSUMS.sha256")) ||
		freeze.V8PublisherManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, "V8_PUBLISHER.sha256")) ||
		freeze.V9ContractManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, "V9_CONTRACT.sha256")) {
		t.Fatalf("V9 history custodian freeze is invalid: %+v", freeze)
	}
	v8VerifyManifest(t, directory, "V9_HISTORY_CUSTODIAN.sha256")

	want := []string{
		"../../.gitattributes",
		"../../cmd/kicadai-v9-history-custodian/main.go",
		"../../cmd/kicadai-v9-history-custodian/main_test.go",
		"../../internal/capabilityfeedback/testdata/closed_loop_open_set_v8_corpus/CHECKSUMS.sha256",
		"../../internal/corpushistoryv9/history.go",
		"../../internal/corpushistoryv9/history_test.go",
		"../../internal/corpuspublication/v9_history_bridge.go",
		"../../internal/corpuspublication/v9_history_bridge_test.go",
		"../../internal/externalkey/key.go",
		"V8_HISTORICAL_COMMITMENTS.json",
		"V8_PUBLISHER.sha256",
		"V9_CONTRACT.sha256",
		"V9_HISTORY_CUSTODIAN_FREEZE.md",
		"v9_history_custodian_contract_test.go",
	}
	manifest, err := os.Open(filepath.Join(directory, "V9_HISTORY_CUSTODIAN.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	defer manifest.Close()
	var got []string
	scanner := bufio.NewScanner(manifest)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 67 {
			t.Fatalf("short custodian manifest line %q", line)
		}
		got = append(got, line[66:])
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("V9 custodian dependency set = %v, want %v", got, want)
	}

	for _, name := range []string{
		"../../cmd/kicadai-v9-history-custodian/main.go",
		"../../internal/corpushistoryv9/history.go",
		"../../internal/corpuspublication/v9_history_bridge.go",
	} {
		source := bytes.ToLower(v7ReadFile(t, filepath.Join(directory, filepath.FromSlash(name))))
		for _, forbidden := range []string{"closedloopsynthesis", "capabilityfeedback", "capabilityrounds", "synthesize(", "simulate(", "rankgap", "frontier"} {
			if bytes.Contains(source, []byte(forbidden)) {
				t.Fatalf("V9 history custodian source %s names forbidden outcome path %q", name, forbidden)
			}
		}
	}
	command := strings.ToLower(string(v7ReadFile(t, filepath.Join(directory, "..", "..", "cmd", "kicadai-v9-history-custodian", "main.go"))))
	if strings.Contains(command, "v8_case_") || strings.Contains(command, "held_out/request_") {
		t.Fatal("V9 history custodian contains an unsafe direct identity reporting path")
	}
}
