// Package corpusfreezev6 adds the V6 historical-exclusion boundary without
// changing the byte-frozen V5 validator implementation.
package corpusfreezev6

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"kicadai/internal/corpusfreeze"
)

const (
	HistoricalCommitmentSchema  = "kicadai.behavior-corpus-historical-commitments.v2"
	HistoricalCommitmentVersion = 2
	maximumHistoryBytes         = 4 << 20
)

type HistoricalCommitmentFile struct {
	Schema              string                          `json:"schema"`
	Version             int                             `json:"version"`
	Raw                 []corpusfreeze.CommitmentRecord `json:"raw"`
	NeutralSemantic     []corpusfreeze.CommitmentRecord `json:"neutral_semantic"`
	NormalizedSemantic  []corpusfreeze.CommitmentRecord `json:"normalized_semantic"`
	RetiredSourceOpened bool                            `json:"retired_source_opened"`
}

type HistoricalCommitments struct {
	Base                     corpusfreeze.HistoricalCommitments
	NormalizedSemanticSHA256 map[string]string
}

func LoadHistoricalCommitments(path string) (HistoricalCommitments, error) {
	data, err := readRegularFile(path)
	if err != nil {
		return HistoricalCommitments{}, fmt.Errorf("read V6 historical commitments: %w", err)
	}
	var source HistoricalCommitmentFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&source); err != nil {
		return HistoricalCommitments{}, fmt.Errorf("decode V6 historical commitments: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return HistoricalCommitments{}, fmt.Errorf("decode V6 historical commitments: trailing JSON value")
	}
	if source.Schema != HistoricalCommitmentSchema || source.Version != HistoricalCommitmentVersion ||
		source.RetiredSourceOpened || len(source.Raw) == 0 || len(source.NeutralSemantic) == 0 || len(source.NormalizedSemantic) == 0 {
		return HistoricalCommitments{}, fmt.Errorf("V6 historical commitment header or retirement boundary is invalid")
	}
	result := HistoricalCommitments{
		Base: corpusfreeze.HistoricalCommitments{
			RawSHA256: map[string]string{}, NeutralSemanticSHA256: map[string]string{}, SourceSHA256: hashBytes(data),
		},
		NormalizedSemanticSHA256: map[string]string{},
	}
	for _, group := range []struct {
		name    string
		records []corpusfreeze.CommitmentRecord
		target  map[string]string
	}{
		{"raw", source.Raw, result.Base.RawSHA256},
		{"neutral semantic", source.NeutralSemantic, result.Base.NeutralSemanticSHA256},
		{"normalized semantic", source.NormalizedSemantic, result.NormalizedSemanticSHA256},
	} {
		previous := ""
		for _, record := range group.records {
			if !validSHA256(record.SHA256) || strings.TrimSpace(record.ID) == "" || group.target[record.SHA256] != "" {
				return HistoricalCommitments{}, fmt.Errorf("V6 historical %s commitment is invalid or duplicated", group.name)
			}
			key := record.SHA256 + "\x00" + record.ID
			if previous != "" && key <= previous {
				return HistoricalCommitments{}, fmt.Errorf("V6 historical %s commitments are not in canonical order", group.name)
			}
			previous = key
			group.target[record.SHA256] = record.ID
		}
	}
	return result, nil
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximumHistoryBytes {
		return nil, fmt.Errorf("history path is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("history path changed during open")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximumHistoryBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maximumHistoryBytes {
		return nil, fmt.Errorf("history file exceeds size limit")
	}
	return data, nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
