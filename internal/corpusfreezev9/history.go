package corpusfreezev9

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"kicadai/internal/corpusfreeze"
	"kicadai/internal/corpusfreezev6"
)

const PredecessorHistoricalCommitmentsSHA256 = "f56d30c27b30e90f4c8568e06870718bac7e9db7d29ed24dac6c768ad163cebf"

const (
	HistoricalRawCount        = 240
	HistoricalNeutralCount    = 168
	HistoricalNormalizedCount = 144
)

type HistoricalCommitments = corpusfreezev6.HistoricalCommitments

func LoadHistoricalCommitments(path string) (HistoricalCommitments, error) {
	return corpusfreezev6.LoadHistoricalCommitments(path)
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
	var source corpusfreezev6.HistoricalCommitmentFile
	decoder := json.NewDecoder(bytes.NewReader(previous))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&source); err != nil {
		return nil, fmt.Errorf("decode predecessor history: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("decode predecessor history: trailing JSON value")
	}
	if source.Schema != corpusfreezev6.HistoricalCommitmentSchema || source.Version != corpusfreezev6.HistoricalCommitmentVersion ||
		source.RetiredSourceOpened || hashBytes(previous) != PredecessorHistoricalCommitmentsSHA256 ||
		len(source.Raw) != 204 || len(source.NeutralSemantic) != 132 || len(source.NormalizedSemantic) != 108 {
		return nil, fmt.Errorf("predecessor history boundary is invalid")
	}
	if len(entries) != 36 {
		return nil, fmt.Errorf("V8 commitment count = %d, want 36", len(entries))
	}

	result := corpusfreezev6.HistoricalCommitmentFile{
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
		result.Raw = append(result.Raw, corpusfreeze.CommitmentRecord{SHA256: entry.RequirementSHA256, ID: id})
		result.NeutralSemantic = append(result.NeutralSemantic, corpusfreeze.CommitmentRecord{SHA256: entry.NeutralSemanticSHA256, ID: id})
		result.NormalizedSemantic = append(result.NormalizedSemantic, corpusfreeze.CommitmentRecord{SHA256: entry.NormalizedSemanticSHA256, ID: id})
	}
	for index := 1; index <= 36; index++ {
		if !seenSource[fmt.Sprintf("v8_source_%03d", index)] {
			return nil, fmt.Errorf("V8 commitment source set is incomplete")
		}
	}
	for name, records := range map[string][]corpusfreeze.CommitmentRecord{
		"raw": result.Raw, "neutral semantic": result.NeutralSemantic, "normalized semantic": result.NormalizedSemantic,
	} {
		sort.Slice(records, func(i, j int) bool {
			if records[i].SHA256 != records[j].SHA256 {
				return records[i].SHA256 < records[j].SHA256
			}
			return records[i].ID < records[j].ID
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
