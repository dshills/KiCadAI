package capabilityevaluation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

const (
	CorpusSchema  = "kicadai.open-world-behavior-corpus.v1"
	CorpusVersion = 1
)

type CorpusRole string

const (
	CorpusDiscovery CorpusRole = "discovery"
	CorpusHeldOut   CorpusRole = "held_out"
)

type CorpusCase struct {
	ID           string       `json:"id"`
	Domain       Domain       `json:"domain"`
	SafetyImpact SafetyImpact `json:"safety_impact"`
	Prompt       string       `json:"prompt"`
	SourceSHA256 string       `json:"source_sha256"`
}

type Corpus struct {
	Schema  string       `json:"schema"`
	Version int          `json:"version"`
	Role    CorpusRole   `json:"role"`
	Cases   []CorpusCase `json:"cases"`
}

func LoadCorpus(path string) (Corpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Corpus{}, fmt.Errorf("read open-world corpus: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var corpus Corpus
	if err := decoder.Decode(&corpus); err != nil {
		return Corpus{}, fmt.Errorf("decode open-world corpus: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Corpus{}, fmt.Errorf("decode open-world corpus: trailing JSON value")
		}
		return Corpus{}, fmt.Errorf("decode open-world corpus trailing data: %w", err)
	}
	if err := ValidateCorpus(corpus); err != nil {
		return Corpus{}, err
	}
	return corpus, nil
}

func ValidateCorpus(corpus Corpus) error {
	if corpus.Schema != CorpusSchema || corpus.Version != CorpusVersion {
		return fmt.Errorf("open-world corpus schema/version = %q/%d", corpus.Schema, corpus.Version)
	}
	if corpus.Role != CorpusDiscovery && corpus.Role != CorpusHeldOut {
		return fmt.Errorf("open-world corpus role %q is invalid", corpus.Role)
	}
	if len(corpus.Cases) == 0 {
		return fmt.Errorf("open-world corpus is empty")
	}
	ids := map[string]bool{}
	sources := map[string]bool{}
	domains := map[Domain]bool{}
	priorID := ""
	for index, current := range corpus.Cases {
		if !semanticIDPattern.MatchString(current.ID) {
			return fmt.Errorf("open-world corpus case %d has invalid id %q", index, current.ID)
		}
		if ids[current.ID] {
			return fmt.Errorf("open-world corpus case id %q is duplicated", current.ID)
		}
		if priorID != "" && current.ID <= priorID {
			return fmt.Errorf("open-world corpus membership is not sorted by id at %q", current.ID)
		}
		priorID = current.ID
		ids[current.ID] = true
		if !validDomain(current.Domain) {
			return fmt.Errorf("open-world corpus case %q has invalid domain %q", current.ID, current.Domain)
		}
		domains[current.Domain] = true
		if !validSafetyImpact(current.SafetyImpact) {
			return fmt.Errorf("open-world corpus case %q has invalid safety impact %q", current.ID, current.SafetyImpact)
		}
		if current.Prompt != strings.TrimSpace(current.Prompt) || current.Prompt == "" {
			return fmt.Errorf("open-world corpus case %q has empty or untrimmed source", current.ID)
		}
		sum := sha256.Sum256([]byte(current.Prompt))
		want := hex.EncodeToString(sum[:])
		if current.SourceSHA256 != want {
			return fmt.Errorf("open-world corpus case %q source hash = %q, want %q", current.ID, current.SourceSHA256, want)
		}
		if sources[current.SourceSHA256] {
			return fmt.Errorf("open-world corpus source %q is duplicated", current.SourceSHA256)
		}
		sources[current.SourceSHA256] = true
	}
	for _, domain := range allDomains() {
		if !domains[domain] {
			return fmt.Errorf("open-world corpus is missing domain %q", domain)
		}
	}
	return nil
}

func ValidateCorpusPair(discovery, heldOut Corpus) error {
	if err := ValidateCorpus(discovery); err != nil {
		return err
	}
	if err := ValidateCorpus(heldOut); err != nil {
		return err
	}
	if discovery.Role != CorpusDiscovery || heldOut.Role != CorpusHeldOut {
		return fmt.Errorf("open-world corpus pair roles = %q/%q", discovery.Role, heldOut.Role)
	}
	discoveryIDs := map[string]bool{}
	discoverySources := map[string]bool{}
	for _, current := range discovery.Cases {
		discoveryIDs[current.ID] = true
		discoverySources[current.SourceSHA256] = true
	}
	for _, current := range heldOut.Cases {
		if discoveryIDs[current.ID] {
			return fmt.Errorf("open-world corpus case %q appears in discovery and held-out sets", current.ID)
		}
		if discoverySources[current.SourceSHA256] {
			return fmt.Errorf("open-world corpus source %q appears in discovery and held-out sets", current.SourceSHA256)
		}
	}
	return nil
}

func CorpusSHA256(corpus Corpus) (string, error) {
	normalized := corpus
	normalized.Cases = slices.Clone(corpus.Cases)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal open-world corpus: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
