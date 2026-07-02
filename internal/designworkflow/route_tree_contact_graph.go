package designworkflow

import (
	"slices"

	"kicadai/internal/transactions"
)

type RouteTreeContactGraphSummary struct {
	Nets              []string `json:"nets,omitempty"`
	RequiredEndpoints int      `json:"required_endpoints"`
	ProvenEndpoints   int      `json:"proven_endpoints"`
	Components        int      `json:"components"`
	CompleteGroups    int      `json:"complete_groups"`
	PartialGroups     int      `json:"partial_groups"`
	BlockedGroups     int      `json:"blocked_groups"`
	SameNetMerges     int      `json:"same_net_merges"`
	LocalRouteMerges  int      `json:"local_route_merges"`
}

func SummarizeRouteTreeContactGraph(targetEvidence InterBlockContactEvidence, operations []transactions.Operation, access []RouteTreeEndpointAccess) RouteTreeContactGraphSummary {
	targetsByNet := normalizedContactGraphTargetsByNet(targetEvidence.Targets)
	decodedOperationsByNet, operationIssues := decodeInterBlockRouteOperations(operations)
	operationsByNet := normalizedDecodedContactOperationsByNet(decodedOperationsByNet)
	componentCounts := interBlockGraphComponentCountsFromDecoded(targetsByNet, operationsByNet, operationIssues)
	summary := RouteTreeContactGraphSummary{}
	netNames := make([]string, 0, len(targetsByNet))
	for netName := range targetsByNet {
		netNames = append(netNames, netName)
	}
	slices.Sort(netNames)
	for _, netName := range netNames {
		targets := targetsByNet[netName]
		summary.RequiredEndpoints += len(targets)
		graph := newInterBlockContactGraph(operationsByNet[netName])
		proven := 0
		for _, target := range targets {
			if _, ok := graph.findTargetNode(target); ok {
				proven++
			}
		}
		summary.ProvenEndpoints += proven
		components := componentCounts[netName]
		summary.Components += components
		switch {
		case len(targets) > 0 && proven == len(targets) && components == 1:
			summary.CompleteGroups++
		case proven > 0:
			summary.PartialGroups++
		default:
			summary.BlockedGroups++
		}
	}
	for _, item := range access {
		switch item.Role {
		case RouteTreeAccessSameNetCopper:
			summary.SameNetMerges++
		case RouteTreeAccessLocalRouteAnchor:
			summary.LocalRouteMerges++
		}
	}
	summary.Nets = netNames
	return summary
}

func normalizedContactGraphTargetsByNet(targets []InterBlockContactTarget) map[string][]InterBlockContactTarget {
	byNet := map[string][]InterBlockContactTarget{}
	for _, target := range targets {
		netName := interBlockSummaryNetKey(target.NetName)
		if netName == "" {
			continue
		}
		byNet[netName] = append(byNet[netName], target)
	}
	return byNet
}

func normalizedDecodedContactOperationsByNet(operationsByNet map[string][]decodedContactRouteOperation) map[string][]decodedContactRouteOperation {
	normalized := map[string][]decodedContactRouteOperation{}
	for rawNetName, operations := range operationsByNet {
		netName := interBlockSummaryNetKey(rawNetName)
		if netName == "" {
			continue
		}
		normalized[netName] = append(normalized[netName], operations...)
	}
	return normalized
}
