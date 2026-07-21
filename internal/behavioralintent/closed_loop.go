package behavioralintent

import (
	"strings"

	"kicadai/internal/closedloopsynthesis"
)

// ApplyClosedLoopEvidence is the final behavioral qualification gate. A
// selected architecture remains executable only when trusted models prove all
// declared analyses and operating cases for the exact bound evidence hashes.
func ApplyClosedLoopEvidence(compilation Result, report closedloopsynthesis.Report) Result {
	result := compilation
	result.ClosedLoop = &ClosedLoopEvidence{
		Status: report.Status, StopReason: report.StopReason, RequirementHash: report.RequirementHash,
		RegistryHash: report.RegistryHash, CatalogHash: report.CatalogHash, ModelRegistryHash: report.ModelRegistryHash,
		SelectedCircuitHash: report.SelectedCircuitHash,
	}
	if result.Status != StatusReady {
		return result
	}
	if closedLoopEvidenceMatches(result, report) && report.Status == "pass" && report.StopReason == closedloopsynthesis.StopPassed && report.Selected != nil && sha256Pattern.MatchString(report.SelectedCircuitHash) {
		return result
	}
	result.Status = StatusUnsupported
	result.Requirement = nil
	capability := closedLoopGapCapability(report.StopReason)
	gap := CapabilityGap{
		ID: stableSearchGapID(capability, "requirements.behavioral_requirements"), Capability: capability,
		Path:             "requirements.behavioral_requirements",
		Reason:           "the installed trusted simulation and model registry did not prove every declared behavior and operating corner",
		RequiredEvidence: []string{"complete trusted graph simulation plan", "reviewed model provenance for every selected component and required analysis", "passing all-corner behavioral assertions"},
	}
	result.CapabilityGaps = normalizeCapabilityGaps(append(result.CapabilityGaps, gap))
	references := []Reference{{Kind: "capability_gap", ID: gap.ID}}
	result.Coverage = normalizeCoverage(rewriteCompiledCoverage(result.Coverage, DispositionCapabilityGap, references, "trusted closed-loop qualification did not prove the compiled behavior"))
	return result
}

func closedLoopEvidenceMatches(compilation Result, report closedloopsynthesis.Report) bool {
	if compilation.Architecture == nil || report.Schema != closedloopsynthesis.ReportSchema {
		return false
	}
	return report.RequirementHash == compilation.Architecture.RequirementHash &&
		report.CatalogHash == compilation.Architecture.CatalogHash &&
		sha256Pattern.MatchString(report.RegistryHash) &&
		sha256Pattern.MatchString(report.ModelRegistryHash) &&
		sha256Pattern.MatchString(report.FormulaLibraryHash) &&
		strings.TrimSpace(report.PolicyVersion) != ""
}

func closedLoopGapCapability(reason closedloopsynthesis.StopReason) string {
	switch reason {
	case closedloopsynthesis.StopModelTrustFailed:
		return "trusted_model_coverage"
	case closedloopsynthesis.StopAssertionIncomplete:
		return "behavioral_assertion_coverage"
	default:
		return "closed_loop_behavioral_verification"
	}
}
