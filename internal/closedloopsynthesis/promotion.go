package closedloopsynthesis

import (
	"fmt"
	"slices"

	"kicadai/internal/simmodel"
)

// ValidatePromotionReport verifies that persisted closed-loop evidence is
// current, internally complete, and passing before a physical workflow may use
// it as a promotion gate.
func ValidatePromotionReport(report Report, catalogHash string) []Diagnostic {
	var diagnostics []Diagnostic
	if report.Schema != ReportSchema || report.PolicyVersion != PolicyVersion {
		diagnostics = append(diagnostics, Diagnostic{Path: "schema", Message: "closed-loop report schema or policy version is unsupported"})
	}
	if report.PolicyHash != hashJSON(report.Policy) {
		diagnostics = append(diagnostics, Diagnostic{Path: "policy_hash", Message: "closed-loop policy hash does not match the persisted policy"})
	}
	for path, value := range map[string]string{
		"requirement_hash": report.RequirementHash, "registry_hash": report.RegistryHash, "catalog_hash": report.CatalogHash,
		"formula_library_hash": report.FormulaLibraryHash, "model_registry_hash": report.ModelRegistryHash,
	} {
		if !validHash(value) {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "closed-loop report requires a lowercase SHA-256 hash"})
		}
	}
	if report.RegistryHash != simmodel.RegistryHash() {
		diagnostics = append(diagnostics, Diagnostic{Path: "registry_hash", Message: "closed-loop report uses a stale trusted simulation registry"})
	}
	if report.CatalogHash != catalogHash {
		diagnostics = append(diagnostics, Diagnostic{Path: "catalog_hash", Message: "closed-loop report catalog hash does not match the resolved circuit"})
	}
	if report.Status != "pass" || report.StopReason != StopPassed || report.Selected == nil {
		diagnostics = append(diagnostics, Diagnostic{Path: "status", Message: "closed-loop report is not a passing selected result"})
	}
	if report.Consumption.CandidatesEvaluated < 1 || report.Consumption.Evaluations < 1 || report.Consumption.BudgetExhausted {
		diagnostics = append(diagnostics, Diagnostic{Path: "consumption", Message: "closed-loop report has invalid or exhausted work accounting"})
	}
	if len(report.Diagnostics) != 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "diagnostics", Message: "passing closed-loop report cannot retain top-level blocking diagnostics"})
	}

	selectedMatches := 0
	for candidateIndex, candidate := range report.Candidates {
		path := fmt.Sprintf("candidates[%d]", candidateIndex)
		if !validHash(candidate.Fingerprint) || candidate.Fingerprint != candidate.FinalState.Fingerprint {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".fingerprint", Message: "candidate fingerprint is invalid or disagrees with final state"})
		}
		if len(candidate.Attempts) == 0 {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".attempts", Message: "candidate has no complete evaluation attempt"})
			continue
		}
		if report.Selected != nil && candidate.Fingerprint == report.Selected.Fingerprint {
			selectedMatches++
			last := candidate.Attempts[len(candidate.Attempts)-1]
			if candidate.Status != "pass" || candidate.StopReason != StopPassed || last.Status != "pass" || candidate.FinalScore.Failures != 0 || candidate.FinalScore.CriticalFailures != 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "selected candidate is not a complete passing evaluation"})
			}
			if !validHash(last.EvidenceHash) || last.StateHash != stateHash(last.State) {
				diagnostics = append(diagnostics, Diagnostic{Path: path + ".attempts", Message: "selected candidate has invalid simulation evidence or state hash"})
			}
			if stateHash(report.Selected.State) != stateHash(candidate.FinalState) || report.Selected.Score != candidate.FinalScore || report.Selected.Repairs != len(candidate.Repairs) {
				diagnostics = append(diagnostics, Diagnostic{Path: "selected", Message: "selected result disagrees with its candidate transcript"})
			}
		}
	}
	if selectedMatches != 1 {
		diagnostics = append(diagnostics, Diagnostic{Path: "selected", Message: "selected fingerprint must identify exactly one candidate transcript"})
	}
	slices.SortStableFunc(diagnostics, compareDiagnostics)
	return diagnostics
}
