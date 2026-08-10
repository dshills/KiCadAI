package corpusfreeze

import (
	"encoding/json"
	"fmt"
	"strings"

	"kicadai/internal/atomicfile"
)

func (report Report) MarshalJSONStable() ([]byte, error) {
	if report.Schema != "kicadai.behavior-corpus-validation-report.v1" || report.Version != 1 || !validSHA256(report.PolicySHA256) || !validSHA256(report.PacketSetSHA256) || !validSHA256(report.ContractBindingSHA256) || !validSHA256(report.HistoricalCommitmentsSHA256) {
		return nil, fmt.Errorf("corpus validation report header is invalid")
	}
	if len(report.Entries) == 0 || len(report.AuthorPacketSHA256) == 0 || len(report.AssignmentSHA256) == 0 || len(report.AuthorshipSHA256) == 0 || len(report.Counts) == 0 {
		return nil, fmt.Errorf("corpus validation report is incomplete")
	}
	for author, digest := range report.AuthorPacketSHA256 {
		if strings.TrimSpace(author) == "" || !validSHA256(digest) || !validSHA256(report.AssignmentSHA256[author]) || !validSHA256(report.AuthorshipSHA256[author]) {
			return nil, fmt.Errorf("corpus validation report author commitments are invalid")
		}
	}
	if len(report.AuthorPacketSHA256) != len(report.AssignmentSHA256) || len(report.AuthorPacketSHA256) != len(report.AuthorshipSHA256) {
		return nil, fmt.Errorf("corpus validation report author commitments are incomplete")
	}
	for _, entry := range report.Entries {
		if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.AuthorSlot) == "" || !validSHA256(entry.RequirementSHA256) || !validSHA256(entry.NeutralSemanticSHA256) || !validSHA256(entry.NormalizedSemanticSHA256) {
			return nil, fmt.Errorf("corpus validation report entry is invalid")
		}
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal corpus validation report: %w", err)
	}
	return append(data, '\n'), nil
}

func WriteReport(path string, report Report) error {
	data, err := report.MarshalJSONStable()
	if err != nil {
		return err
	}
	if err := atomicfile.Write(path, data, 0o600); err != nil {
		return fmt.Errorf("write corpus validation report: %w", err)
	}
	return nil
}
