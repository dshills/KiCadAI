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
	"strings"
	"testing"

	"kicadai/internal/components"
)

func TestMCUPowerIntegritySynthesisCorpusSelectsAndFailsClosedDeterministically(t *testing.T) {
	type expectation struct {
		mcuID             string
		localCalculations int
		bulkCalculations  int
	}
	expectations := map[string]expectation{
		"atmega_mixed_domain_transient.json": {
			mcuID:             "mcu.microchip.atmega328p_a.tqfp32",
			localCalculations: 2,
			bulkCalculations:  1,
		},
		"esp32_wireless_transient.json": {
			mcuID:             "mcu.espressif.esp32_wroom_32e",
			localCalculations: 1,
			bulkCalculations:  1,
		},
		"stm32_debug_transient.json": {
			mcuID:             "mcu.st.stm32g031k8t6.lqfp32",
			localCalculations: 1,
			bulkCalculations:  1,
		},
	}
	root := filepath.Join("testdata", "mcu_power_integrity_synthesis_corpus")
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
		Mutations []struct {
			ID                string  `json:"id"`
			BaseCase          string  `json:"base_case"`
			Constraint        string  `json:"constraint"`
			Value             float64 `json:"value"`
			ExpectedRejection string  `json:"expected_rejection"`
		} `json:"adversarial_mutations"`
	}
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != "kicadai.mcu-power-integrity-synthesis-corpus.v1" ||
		manifest.Version != 1 || manifest.BaseCommit == "" ||
		len(manifest.Cases) != len(expectations) || len(manifest.Mutations) != 3 {
		t.Fatalf("MCU power-integrity manifest identity = %#v", manifest)
	}
	hashes := map[string]string{}
	for _, entry := range manifest.Cases {
		if entry.ID == "" || entry.File == "" || entry.SHA256 == "" ||
			len(entry.Features) == 0 || entry.ExpectedStatus != string(SearchSelected) {
			t.Fatalf("invalid MCU power-integrity manifest case: %#v", entry)
		}
		hashes[entry.File] = entry.SHA256
	}
	registry := mustMCUCorpusRegistry(t)
	catalog := loadArchitectureCatalog(t)
	for file, expect := range expectations {
		t.Run(file, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join(root, file))
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(contents)); got != hashes[file] {
				t.Fatalf("requirement sha256 = %s, want %s", got, hashes[file])
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
			if first.Status != SearchSelected || first.Selected == nil {
				t.Fatalf("search status = %s; issues=%#v rejections=%#v", first.Status, first.Issues, first.Rejections)
			}
			if !selectedCandidateHasComponent(*first.Selected, expect.mcuID) {
				t.Fatalf("selected candidate lacks %s: %#v", expect.mcuID, first.Selected.Selections)
			}
			if got := selectedCalculationCount(*first.Selected, "mcu_power_local_"); got != expect.localCalculations {
				t.Fatalf("local calculation count = %d, want %d", got, expect.localCalculations)
			}
			if got := selectedCalculationCount(*first.Selected, "mcu_power_bulk_"); got != expect.bulkCalculations {
				t.Fatalf("bulk calculation count = %d, want %d", got, expect.bulkCalculations)
			}
			assertSelectedMCUPowerInstancesAreConcrete(t, *first.Selected, catalog)
		})
	}
	for _, mutation := range manifest.Mutations {
		t.Run(mutation.ID, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join(root, mutation.BaseCase))
			if err != nil {
				t.Fatal(err)
			}
			requirement, issues := DecodeStrict(bytes.NewReader(contents))
			if len(issues) != 0 {
				t.Fatalf("strict decode issues = %#v", issues)
			}
			if !replaceParticipantNumericConstraint(&requirement, mutation.Constraint, mutation.Value) {
				t.Fatalf("base case lacks constraint %q", mutation.Constraint)
			}
			result := Search(context.Background(), requirement, registry, SearchOptions{})
			if result.Status != SearchUnsupported {
				t.Fatalf("mutation status = %s, want %s; issues=%#v rejections=%#v", result.Status, SearchUnsupported, result.Issues, result.Rejections)
			}
			if !slices.ContainsFunc(result.Rejections, func(summary RejectionSummary) bool {
				return string(summary.Code) == mutation.ExpectedRejection
			}) {
				t.Fatalf("mutation lacks %s rejection: %#v", mutation.ExpectedRejection, result.Rejections)
			}
		})
	}
}

func assertSelectedMCUPowerInstancesAreConcrete(t *testing.T, candidate CandidateResult, catalog *components.Catalog) {
	t.Helper()
	records := make(map[string]components.ComponentRecord, len(catalog.Records))
	for _, record := range catalog.Records {
		records[record.ID] = record
	}
	count := 0
	for _, selection := range candidate.Selections {
		realization, err := DecodeFragmentRealization(selection.Payload)
		if err != nil {
			t.Fatal(err)
		}
		for _, instance := range realization.Instances {
			if !strings.Contains(instance.ID, "mcu_power_") {
				continue
			}
			count++
			record, exists := records[instance.CatalogID]
			if !exists || record.Generic || strings.TrimSpace(record.MPN) == "" ||
				record.Verification.Confidence != components.ConfidenceVerified {
				t.Fatalf("MCU power instance is not concrete and verified: %#v", instance)
			}
		}
	}
	if count == 0 {
		t.Fatal("selected candidate has no MCU power-integrity instances")
	}
}

func selectedCalculationCount(candidate CandidateResult, prefix string) int {
	count := 0
	for _, selection := range candidate.Selections {
		for _, calculation := range selection.Calculations {
			if strings.HasPrefix(calculation.ID, prefix) && calculation.Pass && calculation.Hash != "" {
				count++
			}
		}
	}
	return count
}

func replaceParticipantNumericConstraint(requirement *Requirement, name string, value float64) bool {
	replacement, err := json.Marshal(value)
	if err != nil {
		return false
	}
	for participantIndex := range requirement.Requirements.Participants {
		for constraintIndex := range requirement.Requirements.Participants[participantIndex].Constraints {
			constraint := &requirement.Requirements.Participants[participantIndex].Constraints[constraintIndex]
			if constraint.Name == name {
				constraint.Value = replacement
				return true
			}
		}
	}
	return false
}
