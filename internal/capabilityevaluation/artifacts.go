package capabilityevaluation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const (
	EvidenceSchema          = "kicadai.open-world-terminal-evidence.v1"
	PromotionEvidenceSchema = "kicadai.open-world-promotion-evidence.v1"
)

type EvidenceSet struct {
	Schema       string       `json:"schema"`
	Version      int          `json:"version"`
	CorpusRole   CorpusRole   `json:"corpus_role"`
	CorpusSHA256 string       `json:"corpus_sha256"`
	Cases        []CaseResult `json:"cases"`
}

type PromotionEvidenceSet struct {
	Schema       string              `json:"schema"`
	Version      int                 `json:"version"`
	CorpusRole   CorpusRole          `json:"corpus_role"`
	CorpusSHA256 string              `json:"corpus_sha256"`
	Cases        []PromotionEvidence `json:"cases"`
}

func LoadEvidenceSet(path string) (EvidenceSet, error) {
	var result EvidenceSet
	if err := decodeStrictFile(path, &result); err != nil {
		return EvidenceSet{}, fmt.Errorf("load terminal evidence: %w", err)
	}
	if result.Schema != EvidenceSchema || result.Version != 1 {
		return EvidenceSet{}, fmt.Errorf("terminal evidence schema/version = %q/%d", result.Schema, result.Version)
	}
	return result, nil
}

func LoadImpactRegistry(path string) (ImpactRegistry, error) {
	var result ImpactRegistry
	if err := decodeStrictFile(path, &result); err != nil {
		return ImpactRegistry{}, fmt.Errorf("load capability-impact registry: %w", err)
	}
	if _, _, _, err := normalizeRegistry(result); err != nil {
		return ImpactRegistry{}, err
	}
	return result, nil
}

func LoadReport(path string) (Report, error) {
	var result Report
	if err := decodeStrictFile(path, &result); err != nil {
		return Report{}, fmt.Errorf("load capability report: %w", err)
	}
	if result.Schema != ReportSchema || result.PolicyVersion == "" ||
		result.CorpusRole == "" || result.CorpusSHA256 == "" ||
		result.RegistryVersion == "" || result.RegistrySHA256 == "" {
		return Report{}, fmt.Errorf("capability report identity is incomplete or unsupported")
	}
	if result.CaseCount != len(result.Cases) {
		return Report{}, fmt.Errorf("capability report case count = %d, cases = %d", result.CaseCount, len(result.Cases))
	}
	return result, nil
}

func LoadPromotionEvidenceSet(path string) (PromotionEvidenceSet, error) {
	var result PromotionEvidenceSet
	if err := decodeStrictFile(path, &result); err != nil {
		return PromotionEvidenceSet{}, fmt.Errorf("load promotion evidence: %w", err)
	}
	if result.Schema != PromotionEvidenceSchema || result.Version != 1 ||
		result.CorpusRole == "" || result.CorpusSHA256 == "" {
		return PromotionEvidenceSet{}, fmt.Errorf("promotion evidence identity is incomplete or unsupported")
	}
	if _, err := normalizePromotions(result.Cases); err != nil {
		return PromotionEvidenceSet{}, err
	}
	return result, nil
}

func EvaluateEvidenceSet(corpus Corpus, evidence EvidenceSet, registry ImpactRegistry, policy RankingPolicy) (Report, error) {
	corpusHash, err := CorpusSHA256(corpus)
	if err != nil {
		return Report{}, err
	}
	if evidence.CorpusRole != corpus.Role || evidence.CorpusSHA256 != corpusHash {
		return Report{}, fmt.Errorf(
			"terminal evidence corpus binding = %q/%q, want %q/%q",
			evidence.CorpusRole, evidence.CorpusSHA256, corpus.Role, corpusHash,
		)
	}
	return EvaluateCorpus(corpus, evidence.Cases, registry, policy)
}

func decodeStrictFile(path string, destination any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return fmt.Errorf("trailing JSON data: %w", err)
	}
	return nil
}
