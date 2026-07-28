package architecturesearch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestClockProgrammingSynthesisCorpusSelectsAndFailsClosedDeterministically(t *testing.T) {
	type expectation struct {
		status       SearchStatus
		componentIDs []string
		calculations []string
		rejection    string
	}
	expectations := map[string]expectation{
		"external_crystal_isp.json": {
			status: SearchSelected,
			componentIDs: []string{
				"crystal.abracon.abm3_16mhz.5032_2pin",
				"mcu.microchip.atmega328p_a.tqfp32",
			},
			calculations: []string{"mcu_external_crystal_worst_case", "mcu_programming_interface_worst_case"},
		},
		"uart_bootloader.json": {
			status:       SearchSelected,
			componentIDs: []string{"mcu.espressif.esp32_wroom_32e"},
			calculations: []string{"mcu_programming_interface_worst_case"},
		},
		"unsupported_unpowered_swd.json": {
			status:    SearchUnsupported,
			rejection: string(CodeMCUProgrammingLoad),
		},
	}
	root := filepath.Join("testdata", "clock_programming_synthesis_corpus")
	var manifest struct {
		Schema     string `json:"schema"`
		Version    int    `json:"version"`
		BaseCommit string `json:"base_commit"`
		Cases      []struct {
			ID             string   `json:"id"`
			File           string   `json:"file"`
			SHA256         string   `json:"sha256"`
			Features       []string `json:"features"`
			ExpectedStatus string   `json:"expected_status"`
		} `json:"cases"`
		Inherited []struct {
			Feature     string `json:"feature"`
			Requirement string `json:"requirement"`
		} `json:"inherited_frozen_evidence"`
	}
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != "kicadai.clock-programming-synthesis-corpus.v1" ||
		manifest.Version != 1 || manifest.BaseCommit == "" ||
		len(manifest.Cases) != len(expectations) || len(manifest.Inherited) != 3 {
		t.Fatalf("clock/programming manifest identity = %#v", manifest)
	}
	manifestHashes := map[string]string{}
	for _, entry := range manifest.Cases {
		if entry.ID == "" || entry.File == "" || entry.SHA256 == "" ||
			len(entry.Features) == 0 || entry.ExpectedStatus == "" {
			t.Fatalf("invalid clock/programming manifest case: %#v", entry)
		}
		manifestHashes[entry.File] = entry.SHA256
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(expectations)+1 {
		t.Fatalf("clock/programming corpus entries = %d, want %d", len(entries), len(expectations)+1)
	}
	registry := mustMCUCorpusRegistry(t)
	for _, entry := range entries {
		if entry.Name() == "manifest.json" {
			continue
		}
		expect, exists := expectations[entry.Name()]
		if !exists || entry.IsDir() {
			t.Fatalf("unexpected clock/programming corpus entry %q", entry.Name())
		}
		t.Run(entry.Name(), func(t *testing.T) {
			contents, readErr := os.ReadFile(filepath.Join(root, entry.Name()))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(contents)); got != manifestHashes[entry.Name()] {
				t.Fatalf("requirement sha256 = %s, want %s", got, manifestHashes[entry.Name()])
			}
			requirement, issues := DecodeStrict(bytes.NewReader(contents))
			if len(issues) != 0 {
				t.Fatalf("strict decode issues = %#v", issues)
			}
			first := Search(context.Background(), requirement, registry, SearchOptions{})
			second := Search(context.Background(), requirement, registry, SearchOptions{})
			firstJSON, firstErr := json.Marshal(first)
			secondJSON, secondErr := json.Marshal(second)
			if firstErr != nil || secondErr != nil || !bytes.Equal(firstJSON, secondJSON) {
				t.Fatalf("search replay differs: first=%v second=%v equal=%t", firstErr, secondErr, bytes.Equal(firstJSON, secondJSON))
			}
			if first.Status != expect.status {
				t.Fatalf("search status = %s, want %s; issues=%#v rejections=%#v", first.Status, expect.status, first.Issues, first.Rejections)
			}
			if expect.status == SearchUnsupported {
				if !slices.ContainsFunc(first.Rejections, func(summary RejectionSummary) bool {
					return string(summary.Code) == expect.rejection
				}) {
					t.Fatalf("unsupported case lacks %s rejection: %#v", expect.rejection, first.Rejections)
				}
				return
			}
			if first.Selected == nil {
				t.Fatal("selected search result has no candidate")
			}
			for _, componentID := range expect.componentIDs {
				if !selectedCandidateHasComponent(*first.Selected, componentID) {
					t.Fatalf("selected candidate lacks %s: %#v", componentID, first.Selected.Selections)
				}
			}
			for _, calculationID := range expect.calculations {
				if !selectedCandidateHasCalculation(*first.Selected, calculationID) {
					t.Fatalf("selected candidate lacks finalized %s evidence: %#v", calculationID, first.Selected.Selections)
				}
			}
		})
	}
}

func selectedCandidateHasComponent(candidate CandidateResult, componentID string) bool {
	for _, selection := range candidate.Selections {
		if slices.ContainsFunc(selection.Components, func(component SelectedComponent) bool {
			return component.CatalogID == componentID
		}) {
			return true
		}
	}
	return false
}

func selectedCandidateHasCalculation(candidate CandidateResult, calculationID string) bool {
	for _, selection := range candidate.Selections {
		if slices.ContainsFunc(selection.Calculations, func(calculation CalculationEvidence) bool {
			return calculation.ID == calculationID && calculation.Pass && calculation.Hash != ""
		}) {
			return true
		}
	}
	return false
}
