package corpusfreezev9

import (
	"encoding/json"
	"fmt"
	"strings"

	"kicadai/internal/atomicfile"
)

func (report Report) MarshalJSONStable() ([]byte, error) {
	if report.Schema != "kicadai.behavior-corpus-validation-report.v9" || report.Version != 9 ||
		!validSHA256(report.PolicySHA256) || !validSHA256(report.PacketSetSHA256) ||
		!validSHA256(report.ContractBindingSHA256) || !validSHA256(report.HistoricalCommitmentsSHA256) || len(report.Entries) != CorpusCaseCount {
		return nil, fmt.Errorf("V9 validation report is incomplete")
	}
	for _, entry := range report.Entries {
		if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.CircuitRole) == "" ||
			strings.TrimSpace(entry.PrimaryClass) == "" || strings.TrimSpace(entry.RequiredPrimaryAnalysis) == "" ||
			strings.TrimSpace(entry.OutputMultiplicity) == "" ||
			!validSHA256(entry.RequirementSHA256) || !validSHA256(entry.NeutralSemanticSHA256) || !validSHA256(entry.NormalizedSemanticSHA256) {
			return nil, fmt.Errorf("V9 validation report entry is invalid")
		}
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func WriteReport(path string, report Report) error {
	data, err := report.MarshalJSONStable()
	if err != nil {
		return err
	}
	return atomicfile.Write(path, data, 0o600)
}
