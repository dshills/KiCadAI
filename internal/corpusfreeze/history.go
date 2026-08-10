package corpusfreeze

import (
	"fmt"
	"strings"
)

const (
	HistoricalCommitmentSchema  = "kicadai.behavior-corpus-historical-commitments.v1"
	HistoricalCommitmentVersion = 1
)

type HistoricalCommitmentFile struct {
	Schema              string             `json:"schema"`
	Version             int                `json:"version"`
	Raw                 []CommitmentRecord `json:"raw"`
	NeutralSemantic     []CommitmentRecord `json:"neutral_semantic"`
	RetiredSourceOpened bool               `json:"retired_source_opened"`
}

type CommitmentRecord struct {
	SHA256 string `json:"sha256"`
	ID     string `json:"id"`
}

func LoadHistoricalCommitments(path string) (HistoricalCommitments, error) {
	data, err := readRegularFile(path, maxAuthorshipBytes)
	if err != nil {
		return HistoricalCommitments{}, fmt.Errorf("read historical commitments: %w", err)
	}
	var source HistoricalCommitmentFile
	if err := decodeStrict(data, &source); err != nil {
		return HistoricalCommitments{}, fmt.Errorf("decode historical commitments: %w", err)
	}
	if source.Schema != HistoricalCommitmentSchema || source.Version != HistoricalCommitmentVersion || source.RetiredSourceOpened {
		return HistoricalCommitments{}, fmt.Errorf("historical commitment header or retirement boundary is invalid")
	}
	result := HistoricalCommitments{
		RawSHA256: map[string]string{}, NeutralSemanticSHA256: map[string]string{}, SourceSHA256: hashBytes(data),
	}
	for _, group := range []struct {
		name    string
		records []CommitmentRecord
		target  map[string]string
	}{
		{"raw", source.Raw, result.RawSHA256},
		{"neutral semantic", source.NeutralSemantic, result.NeutralSemanticSHA256},
	} {
		previous := ""
		for _, record := range group.records {
			if !validSHA256(record.SHA256) || strings.TrimSpace(record.ID) == "" || group.target[record.SHA256] != "" {
				return HistoricalCommitments{}, fmt.Errorf("historical %s commitment is invalid or duplicated", group.name)
			}
			key := record.SHA256 + "\x00" + record.ID
			if previous != "" && key <= previous {
				return HistoricalCommitments{}, fmt.Errorf("historical %s commitments are not in canonical order", group.name)
			}
			previous = key
			group.target[record.SHA256] = record.ID
		}
	}
	return result, nil
}
