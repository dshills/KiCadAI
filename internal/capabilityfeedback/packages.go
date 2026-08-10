package capabilityfeedback

import (
	"fmt"

	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilitypackages"
)

// PackageObservations adapts a validated discovery aggregate into the frozen
// V5 identity-neutral package-ranking boundary.
func PackageObservations(report AggregateReport, registry capabilityevaluation.ImpactRegistry) ([]capabilitypackages.Observation, error) {
	if report.CorpusRole != RoleDiscovery {
		return nil, fmt.Errorf("capability packages require discovery-only evidence")
	}
	if err := ValidateAggregateReport(report, registry); err != nil {
		return nil, fmt.Errorf("validate discovery evidence: %w", err)
	}
	cases := make(map[string]CaseMeta, len(report.Cases))
	for _, current := range report.Cases {
		cases[current.Case.ID] = current.Case
	}
	observationCount := 0
	for _, cluster := range report.Clusters {
		observationCount += len(cluster.Cases)
	}
	observations := make([]capabilitypackages.Observation, 0, observationCount)
	for _, cluster := range report.Clusters {
		requiredEvidence := append([]string(nil), cluster.RequiredEvidence...)
		for _, caseID := range cluster.Cases {
			meta, found := cases[caseID]
			if !found {
				return nil, fmt.Errorf("cluster references unknown discovery case %q", caseID)
			}
			observations = append(observations, capabilitypackages.Observation{
				Role: string(RoleDiscovery), CaseID: caseID, ReportingDomain: string(meta.Domain), SafetyWeight: int64(safetyWeight(meta.SafetyImpact)),
				Stage: cluster.Stage, Scope: string(cluster.Scope), Capability: cluster.Capability, Code: cluster.Code,
				RequiredEvidence: requiredEvidence,
			})
		}
	}
	return observations, nil
}
