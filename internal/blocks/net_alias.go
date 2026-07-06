package blocks

import (
	"fmt"
	"strings"

	"kicadai/internal/reports"
)

type CompositionNetAliasResolution struct {
	LocalNet     string `json:"local_net"`
	CanonicalNet string `json:"canonical_net"`
	InstanceID   string `json:"instance_id"`
	Port         string `json:"port"`
}

func ResolveCompositionNetAliases(request CompositionRequest) ([]CompositionNetAliasResolution, []reports.Issue) {
	netGroups := newPortDisjointSet()
	for _, connection := range request.Connections {
		netGroups.union(connection.From, connection.To)
	}
	aliasesByRoot := map[PortRef]string{}
	var issues []reports.Issue
	for index, connection := range request.Connections {
		if connection.NetAlias == "" {
			continue
		}
		alias := connection.NetAlias
		if mapped, ok := request.NetAliases[alias]; ok {
			alias = mapped
		}
		alias = sanitizeNetPart(alias)
		root := netGroups.find(connection.From)
		if existing := aliasesByRoot[root]; existing != "" && existing != alias {
			issues = append(issues, blockIssue(fmt.Sprintf("connections[%d].net_alias", index), fmt.Sprintf("conflicting net aliases for connected ports: %s and %s", existing, alias)))
			continue
		}
		aliasesByRoot[root] = alias
	}
	if len(issues) != 0 {
		return nil, issues
	}
	resolutionByLocal := map[string]CompositionNetAliasResolution{}
	seenPorts := map[PortRef]struct{}{}
	for _, connection := range request.Connections {
		root := netGroups.find(connection.From)
		canonical := aliasesByRoot[root]
		if canonical == "" {
			continue
		}
		for _, ref := range []PortRef{connection.From, connection.To} {
			if _, seen := seenPorts[ref]; seen {
				continue
			}
			seenPorts[ref] = struct{}{}
			for _, local := range compositionPortLocalNetCandidates(ref) {
				if local == "" || local == canonical {
					continue
				}
				resolution := CompositionNetAliasResolution{
					LocalNet:     local,
					CanonicalNet: canonical,
					InstanceID:   ref.InstanceID,
					Port:         ref.Port,
				}
				if existing := resolutionByLocal[local]; existing.CanonicalNet != "" && existing.CanonicalNet != canonical {
					issues = append(issues, reports.Issue{
						Code:       reports.CodeValidationFailed,
						Severity:   reports.SeverityError,
						Path:       "net_aliases." + local,
						Message:    "conflicting canonical net aliases for " + local + ": " + existing.CanonicalNet + " and " + canonical,
						Nets:       []string{existing.CanonicalNet, canonical},
						Suggestion: "use one net_alias for each connected instance port",
					})
					continue
				}
				resolutionByLocal[local] = resolution
			}
		}
	}
	if len(issues) != 0 {
		return nil, issues
	}
	resolutions := make([]CompositionNetAliasResolution, 0, len(resolutionByLocal))
	for _, resolution := range resolutionByLocal {
		resolutions = append(resolutions, resolution)
	}
	return resolutions, nil
}

func compositionPortLocalNetCandidates(ref PortRef) []string {
	port := strings.TrimSpace(ref.Port)
	candidates := []string{InstanceNetName(ref.InstanceID, port)}
	if lower := strings.ToLower(port); lower != "" && lower != port {
		candidates = append(candidates, InstanceNetName(ref.InstanceID, lower))
	}
	return candidates
}
