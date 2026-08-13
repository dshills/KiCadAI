package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/corpusfreeze"
	"kicadai/internal/corpusfreezev8"
	"kicadai/internal/corpushistoryv9"
	"kicadai/internal/corpuspublication"
)

func TestUsageFailsClosed(t *testing.T) {
	for _, arguments := range [][]string{nil, {"-repository-root", "x"}, {"extra"}} {
		if err := run(arguments, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Fatalf("arguments %v did not fail with usage", arguments)
		}
	}
}

func TestCombineCommitmentsRequiresCompletePartitions(t *testing.T) {
	discovery := make([]corpuspublication.EntryV8, 18)
	heldOut := make([]corpuspublication.EntryV8, 18)
	for index := 0; index < 36; index++ {
		entry := corpuspublication.EntryV8{SourceID: fmt.Sprintf("v8_source_%03d", index+1),
			RequirementSHA256: digest("raw", index), NeutralSemanticSHA256: digest("neutral", index), NormalizedSemanticSHA256: digest("normalized", index)}
		if index < 18 {
			entry.Role = "discovery"
			discovery[index] = entry
		} else {
			entry.Role, entry.Sealed = "held_out", true
			heldOut[index-18] = entry
		}
	}
	got, err := combineCommitments(discovery, heldOut)
	if err != nil || len(got) != 36 || got[0].SourceID != "v8_source_001" || got[35].SourceID != "v8_source_036" {
		t.Fatalf("combine = %d entries, %v", len(got), err)
	}
	if _, err := combineCommitments(discovery[:17], heldOut); err == nil {
		t.Fatal("incomplete discovery partition was accepted")
	}
	heldOut[0].Sealed = false
	if _, err := combineCommitments(discovery, heldOut); err == nil {
		t.Fatal("unsealed held-out metadata was accepted")
	}
}

func TestPublishExclusiveNeverReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	validate := func(stage string) error {
		data, err := os.ReadFile(stage)
		if err != nil || string(data) != "first\n" {
			return fmt.Errorf("unexpected staged bytes")
		}
		return nil
	}
	if err := publishExclusive(path, []byte("first\n"), 0o644, validate); err != nil {
		t.Fatal(err)
	}
	if err := publishExclusive(path, []byte("second\n"), 0o644, func(string) error { return nil }); err == nil {
		t.Fatal("occupied history destination was replaced")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "first\n" {
		t.Fatalf("history bytes = %q, %v", got, err)
	}
}

func TestPublishExclusiveValidatesBeforeVisibility(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	if err := publishExclusive(path, []byte("invalid\n"), 0o644, func(string) error { return fmt.Errorf("invalid") }); err == nil {
		t.Fatal("invalid staged history was published")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("failed validation made history visible")
	}
}

func TestRunAuthenticatesSyntheticV8AndPublishesDigestOnlyHistory(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	if err := os.Mkdir(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	sourceTemplate, err := os.ReadFile(filepath.Join("..", "..", "internal", "opentopologysynthesis", "testdata", "architecture_generalization_corpus", "regulated_low_voltage_output.json"))
	if err != nil {
		t.Fatal(err)
	}
	var template map[string]any
	if err := json.Unmarshal(sourceTemplate, &template); err != nil {
		t.Fatal(err)
	}
	report := corpusfreezev8.Report{Schema: "kicadai.behavior-corpus-validation-report.v8", Version: 8,
		PolicySHA256: digest("policy", 0), PacketSetSHA256: digest("packet", 0), ContractBindingSHA256: digest("binding", 0), HistoricalCommitmentsSHA256: digest("history", 0),
		AuthorPacketSHA256: map[string]string{}, AssignmentSHA256: map[string]string{}, AuthorshipSHA256: map[string]string{},
		Counts: map[string]map[string]int{"discovery": {"analog_signal_path": 18}, "held_out": {"analog_signal_path": 18}}}
	bundles := map[string]corpusfreeze.Bundle{}
	for authorIndex := 1; authorIndex <= 6; authorIndex++ {
		author := fmt.Sprintf("author_%d", authorIndex)
		authorship := []byte(fmt.Sprintf("{\"author_slot\":%q}\n", author))
		report.AuthorPacketSHA256[author] = digest("packet-"+author, 0)
		report.AssignmentSHA256[author] = digest("assignment-"+author, 0)
		report.AuthorshipSHA256[author] = byteDigest(authorship)
		bundles[author] = corpusfreeze.Bundle{AuthorshipJSON: authorship, Requirements: map[string][]byte{}}
	}
	for index := 1; index <= 36; index++ {
		project := template["project"].(map[string]any)
		project["name"] = fmt.Sprintf("v9_history_synthetic_%03d", index)
		source, err := json.MarshalIndent(template, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		source = append(source, '\n')
		id := fmt.Sprintf("v8_case_%03d", index)
		author := fmt.Sprintf("author_%d", ((index-1)/6)+1)
		role := "discovery"
		if index > 18 {
			role = "held_out"
		}
		requestPath := fmt.Sprintf("%s/request_%03d.json", role, index)
		bundle := bundles[author]
		bundle.Requirements[requestPath] = source
		bundles[author] = bundle
		report.Entries = append(report.Entries, corpusfreezev8.EntryEvidence{ID: id, AuthorSlot: author, Role: role,
			Domain: "analog_signal_path", CircuitRole: "conversion_regulation", SafetyImpact: "non_safety",
			SourceID: fmt.Sprintf("v8_source_%03d", index), RequirementFile: requestPath, RequirementSHA256: byteDigest(source),
			NeutralSemanticSHA256: digest("neutral", index), NormalizedSemanticSHA256: digest("normalized", index)})
	}
	entropy := make([]byte, 2048)
	for index := range entropy {
		entropy[index] = byte(index)
	}
	corpusRoot := filepath.Join(repository, "v8-corpus")
	keyPath := filepath.Join(root, "keys", "v8-source.key")
	_, err = corpuspublication.PublishV8(corpuspublication.RequestV8{RepositoryRoot: repository, DestinationRoot: corpusRoot, KeyPath: keyPath,
		ContractManifestSHA256: digest("contract", 0), ValidatorManifest: []byte("validator\n"), PublisherManifest: []byte("publisher\n"),
		Commits: corpuspublication.Commits{StartingCommit: strings.Repeat("1", 40), ContractFreezeCommit: strings.Repeat("2", 40), AuthoringPacketCommit: strings.Repeat("3", 40), ValidatorCommit: strings.Repeat("4", 40), FreezeParentCommit: strings.Repeat("5", 40)},
		Report:  report, Bundles: bundles, Random: bytes.NewReader(entropy)})
	if err != nil {
		t.Fatal(err)
	}
	predecessorData, err := os.ReadFile(filepath.Join("..", "..", "specs", "closed-loop-open-set-capability-expansion", "V8_HISTORICAL_COMMITMENTS.json"))
	if err != nil {
		t.Fatal(err)
	}
	predecessor := filepath.Join(repository, "V8_HISTORICAL_COMMITMENTS.json")
	if err := os.WriteFile(predecessor, predecessorData, 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(repository, "V9_HISTORICAL_COMMITMENTS.json")
	var stdout bytes.Buffer
	err = run([]string{"-repository-root", repository, "-v8-corpus-root", corpusRoot, "-v8-source-key", keyPath,
		"-predecessor-history", predecessor, "-output", output}, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), "v8_case_") || strings.Contains(stdout.String(), "held_out") {
		t.Fatalf("custodian output disclosed held-out identity: %q", stdout.String())
	}
	history, err := corpushistoryv9.LoadHistoricalCommitments(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := corpushistoryv9.ValidateHistoricalBoundary(history); err != nil {
		t.Fatal(err)
	}
}

func digest(kind string, index int) string {
	value := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", kind, index)))
	return hex.EncodeToString(value[:])
}

func byteDigest(data []byte) string {
	value := sha256.Sum256(data)
	return hex.EncodeToString(value[:])
}
