package capabilityevaluation

import (
	"fmt"
	"slices"
)

type AffectedCases struct {
	Capability string   `json:"capability"`
	Cases      []string `json:"cases"`
}

func ExplainCluster(report Report, capability string, rank int) (Cluster, error) {
	if capability == "" && rank <= 0 {
		return Cluster{}, fmt.Errorf("cluster explanation requires a capability or positive rank")
	}
	for _, cluster := range report.RankedClusters {
		if capability != "" && cluster.Capability != capability {
			continue
		}
		if rank > 0 && cluster.Rank != rank {
			continue
		}
		return cluster, nil
	}
	return Cluster{}, fmt.Errorf("cluster explanation not found")
}

func ListCasesAffectedByCapability(report Report, capability string) (AffectedCases, error) {
	if !semanticIDPattern.MatchString(capability) {
		return AffectedCases{}, fmt.Errorf("capability %q is invalid", capability)
	}
	seen := map[string]bool{}
	for _, current := range report.Cases {
		for _, observation := range current.Observations {
			if observation.Capability == capability {
				seen[current.ID] = true
			}
		}
	}
	cases := sortedStringKeys(seen)
	return AffectedCases{Capability: capability, Cases: slices.Clone(cases)}, nil
}
