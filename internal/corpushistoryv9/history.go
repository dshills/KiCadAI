package corpushistoryv9

import (
	"bytes"
	"cmp"
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
	HistoricalCommitmentSchema             = "kicadai.behavior-corpus-historical-commitments.v2"
	HistoricalCommitmentVersion            = 2
	PredecessorHistoricalCommitmentsSHA256 = "f56d30c27b30e90f4c8568e06870718bac7e9db7d29ed24dac6c768ad163cebf"
	maximumHistoryBytes                    = 4 << 20
)

const (
	HistoricalRawCount         = 240
	HistoricalNeutralCount     = 168
	HistoricalNormalizedCount  = 144
	predecessorRawCount        = HistoricalRawCount - 36
	predecessorNeutralCount    = HistoricalNeutralCount - 36
	predecessorNormalizedCount = HistoricalNormalizedCount - 36
)

type CommitmentRecord struct {
	SHA256 string `json:"sha256"`
	ID     string `json:"id"`
}

type HistoricalCommitmentFile struct {
	Schema              string             `json:"schema"`
	Version             int                `json:"version"`
	Raw                 []CommitmentRecord `json:"raw"`
	NeutralSemantic     []CommitmentRecord `json:"neutral_semantic"`
	NormalizedSemantic  []CommitmentRecord `json:"normalized_semantic"`
	RetiredSourceOpened bool               `json:"retired_source_opened"`
}

type BaseCommitments struct {
	RawSHA256             map[string]string
	NeutralSemanticSHA256 map[string]string
	SourceSHA256          string
}

type HistoricalCommitments struct {
	Base                     BaseCommitments
	NormalizedSemanticSHA256 map[string]string
}

func LoadHistoricalCommitments(path string) (HistoricalCommitments, error) {
	data, err := readRegularFile(path)
	if err != nil {
		return HistoricalCommitments{}, fmt.Errorf("read V9 historical commitments: %w", err)
	}
	var source HistoricalCommitmentFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&source); err != nil {
		return HistoricalCommitments{}, fmt.Errorf("decode V9 historical commitments: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return HistoricalCommitments{}, fmt.Errorf("decode V9 historical commitments: trailing JSON value")
	}
	if source.Schema != HistoricalCommitmentSchema || source.Version != HistoricalCommitmentVersion ||
		source.RetiredSourceOpened || len(source.Raw) == 0 || len(source.NeutralSemantic) == 0 || len(source.NormalizedSemantic) == 0 {
		return HistoricalCommitments{}, fmt.Errorf("V9 historical commitment header or retirement boundary is invalid")
	}
	result := HistoricalCommitments{Base: BaseCommitments{RawSHA256: map[string]string{}, NeutralSemanticSHA256: map[string]string{}, SourceSHA256: hashBytes(data)}, NormalizedSemanticSHA256: map[string]string{}}
	for _, group := range []struct {
		name    string
		records []CommitmentRecord
		target  map[string]string
	}{
		{"raw", source.Raw, result.Base.RawSHA256},
		{"neutral semantic", source.NeutralSemantic, result.Base.NeutralSemanticSHA256},
		{"normalized semantic", source.NormalizedSemantic, result.NormalizedSemanticSHA256},
	} {
		previous := ""
		seenID := map[string]bool{}
		for _, record := range group.records {
			key := record.SHA256 + "\x00" + record.ID
			if !validSHA256(record.SHA256) || strings.TrimSpace(record.ID) == "" || group.target[record.SHA256] != "" ||
				seenID[record.ID] || (previous != "" && key <= previous) {
				return HistoricalCommitments{}, fmt.Errorf("V9 historical %s commitment is invalid, duplicated, or unordered", group.name)
			}
			previous = key
			seenID[record.ID] = true
			group.target[record.SHA256] = record.ID
		}
	}
	return result, nil
}

// ValidateHistoricalBoundary rejects the predecessor-only history and any
// partial V8 extension before an author bundle is read.
func ValidateHistoricalBoundary(value HistoricalCommitments) error {
	if !validSHA256(value.Base.SourceSHA256) || len(value.Base.RawSHA256) != HistoricalRawCount ||
		len(value.Base.NeutralSemanticSHA256) != HistoricalNeutralCount ||
		len(value.NormalizedSemanticSHA256) != HistoricalNormalizedCount {
		return fmt.Errorf("V9 historical commitment boundary is incomplete")
	}
	return nil
}

// CommitmentEntry is the outcome-neutral digest surface needed to retire one
// V8 requirement. It intentionally contains no requirement content, outcome,
// feasibility, obligation, or selection data.
type CommitmentEntry struct {
	SourceID                 string
	RequirementSHA256        string
	NeutralSemanticSHA256    string
	NormalizedSemanticSHA256 string
}

// ExtendHistoricalCommitments appends the complete authenticated V8 digest
// set to the sanitized V1-V7 history and returns canonical V1-V8 JSON. An
// isolated custodian supplies the 36 entries after authenticating the V8
// encrypted source; this function neither reads a key nor opens ciphertext.
func ExtendHistoricalCommitments(previous []byte, entries []CommitmentEntry) ([]byte, error) {
	var source HistoricalCommitmentFile
	decoder := json.NewDecoder(bytes.NewReader(previous))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&source); err != nil {
		return nil, fmt.Errorf("decode predecessor history: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("decode predecessor history: trailing JSON value")
	}
	if source.Schema != HistoricalCommitmentSchema || source.Version != HistoricalCommitmentVersion ||
		source.RetiredSourceOpened || hashBytes(previous) != PredecessorHistoricalCommitmentsSHA256 ||
		len(source.Raw) != predecessorRawCount || len(source.NeutralSemantic) != predecessorNeutralCount ||
		len(source.NormalizedSemantic) != predecessorNormalizedCount {
		return nil, fmt.Errorf("predecessor history boundary is invalid")
	}
	if len(entries) != 36 {
		return nil, fmt.Errorf("V8 commitment count = %d, want 36", len(entries))
	}

	result := HistoricalCommitmentFile{
		Schema:              source.Schema,
		Version:             source.Version,
		Raw:                 slices.Clone(source.Raw),
		NeutralSemantic:     slices.Clone(source.NeutralSemantic),
		NormalizedSemantic:  slices.Clone(source.NormalizedSemantic),
		RetiredSourceOpened: false,
	}
	seenSource := map[string]bool{}
	for _, entry := range entries {
		if seenSource[entry.SourceID] ||
			!validSHA256(entry.RequirementSHA256) || !validSHA256(entry.NeutralSemanticSHA256) || !validSHA256(entry.NormalizedSemanticSHA256) {
			return nil, fmt.Errorf("V8 commitment entry is invalid or duplicated")
		}
		seenSource[entry.SourceID] = true
		id := "v8:" + entry.SourceID
		result.Raw = append(result.Raw, CommitmentRecord{SHA256: entry.RequirementSHA256, ID: id})
		result.NeutralSemantic = append(result.NeutralSemantic, CommitmentRecord{SHA256: entry.NeutralSemanticSHA256, ID: id})
		result.NormalizedSemantic = append(result.NormalizedSemantic, CommitmentRecord{SHA256: entry.NormalizedSemanticSHA256, ID: id})
	}
	// Source identities are part of the frozen V8 assignment, not author
	// content. Requiring the exact set proves that the custodian did not accept
	// an arbitrary collection of 36 valid-looking digests or omit one retired
	// source while duplicating another under a different name.
	for index := 1; index <= 36; index++ {
		if !seenSource[fmt.Sprintf("v8_source_%03d", index)] {
			return nil, fmt.Errorf("V8 commitment source set is incomplete")
		}
	}
	for name, records := range map[string][]CommitmentRecord{
		"raw": result.Raw, "neutral semantic": result.NeutralSemantic, "normalized semantic": result.NormalizedSemantic,
	} {
		slices.SortFunc(records, func(left, right CommitmentRecord) int {
			if order := cmp.Compare(left.SHA256, right.SHA256); order != 0 {
				return order
			}
			return cmp.Compare(left.ID, right.ID)
		})
		seenID := map[string]bool{}
		for index, record := range records {
			if !validSHA256(record.SHA256) || strings.TrimSpace(record.ID) == "" ||
				seenID[record.ID] || (index > 0 && records[index-1].SHA256 == record.SHA256) {
				return nil, fmt.Errorf("%s commitment is invalid or duplicated", name)
			}
			seenID[record.ID] = true
		}
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > maximumHistoryBytes {
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
	if err != nil || len(data) > maximumHistoryBytes {
		return nil, fmt.Errorf("read bounded history: %w", err)
	}
	return data, nil
}

func validSHA256(value string) bool {
	// Lowercase is part of the canonical artifact grammar. Accepting uppercase
	// would permit multiple byte representations of the same commitment.
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func hashBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
