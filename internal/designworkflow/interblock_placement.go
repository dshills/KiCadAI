package designworkflow

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type blockPortEndpointIndex map[blockPortKey][]placement.Endpoint

const interBlockPlacementNetWeight = 12

type blockPortKey struct {
	instance string
	port     string
}

func addPlacementConnectionNets(request *placement.Request, indexes map[string]int, design Request, fragments PCBFragmentResult) []reports.Issue {
	if request == nil || len(design.Connections) == 0 {
		return nil
	}
	endpoints := buildBlockPortEndpointIndex(fragments)
	var issues []reports.Issue
	for index, connection := range design.Connections {
		fromRef, fromOK := ParseEndpoint(connection.From)
		toRef, toOK := ParseEndpoint(connection.To)
		path := fmt.Sprintf("design.inter_block_routing.connections[%d]", index)
		if !fromOK {
			issues = append(issues, interBlockPlacementIssue(path+".from", "connection from endpoint must be instance.port", nil))
			continue
		}
		if !toOK {
			issues = append(issues, interBlockPlacementIssue(path+".to", "connection to endpoint must be instance.port", nil))
			continue
		}
		fromEndpoints := endpoints.lookup(fromRef.InstanceID, fromRef.Port)
		toEndpoints := endpoints.lookup(toRef.InstanceID, toRef.Port)
		if len(fromEndpoints) == 0 {
			issues = append(issues, interBlockPlacementIssue(path+".from", "connection endpoint does not resolve to a generated PCB pad", []string{connection.From}))
			continue
		}
		if len(toEndpoints) == 0 {
			issues = append(issues, interBlockPlacementIssue(path+".to", "connection endpoint does not resolve to a generated PCB pad", []string{connection.To}))
			continue
		}
		netName := canonicalInterBlockNetName(connection, fromRef, toRef)
		addPlacementNetWithEndpointMerges(request, indexes, netName, netRoleFromName(netName), interBlockPlacementNetWeight, append(fromEndpoints, toEndpoints...)...)
	}
	return issues
}

func buildBlockPortEndpointIndex(fragments PCBFragmentResult) blockPortEndpointIndex {
	index := blockPortEndpointIndex{}
	for _, fragment := range fragments.Fragments {
		instanceID := strings.TrimSpace(fragment.InstanceID)
		if instanceID == "" {
			continue
		}
		realizedRefs := realizedFragmentRefs(fragment)
		for port, endpoints := range fragment.PortEndpoints {
			key := newBlockPortKey(instanceID, port)
			index[key] = appendUniquePlacementEndpoints(index[key], placementEndpointsFromTransactionEndpointsForRefs(endpoints, realizedRefs)...)
		}
	}
	return index
}

func fragmentPortEndpoints(instanceID string, operations []transactions.Operation) map[string][]transactions.Endpoint {
	out := map[string][]transactions.Endpoint{}
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return nil
	}
	for _, operation := range operations {
		if operation.Op != transactions.OpConnect || len(operation.Raw) == 0 {
			continue
		}
		var connect transactions.ConnectOperation
		if err := json.Unmarshal(operation.Raw, &connect); err != nil {
			continue
		}
		addFragmentPortEndpoint(out, instanceID, connect.From, connect.To)
		addFragmentPortEndpoint(out, instanceID, connect.To, connect.From)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func addFragmentPortEndpoint(out map[string][]transactions.Endpoint, instanceID string, port transactions.Endpoint, physical transactions.Endpoint) {
	if !strings.EqualFold(strings.TrimSpace(port.Ref), instanceID) {
		return
	}
	if strings.EqualFold(strings.TrimSpace(physical.Ref), instanceID) {
		return
	}
	pin := strings.TrimSpace(port.Pin)
	endpoint := transactions.Endpoint{Ref: strings.TrimSpace(physical.Ref), Pin: strings.TrimSpace(physical.Pin)}
	if pin == "" || endpoint.Ref == "" || endpoint.Pin == "" {
		return
	}
	key := strings.ToUpper(pin)
	for _, existing := range out[key] {
		if strings.EqualFold(existing.Ref, endpoint.Ref) && strings.EqualFold(existing.Pin, endpoint.Pin) {
			return
		}
	}
	out[key] = append(out[key], endpoint)
}

func placementEndpointsFromTransactionEndpointsForRefs(endpoints []transactions.Endpoint, realizedRefs map[string]struct{}) []placement.Endpoint {
	out := make([]placement.Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if realizedRefs != nil {
			if _, ok := realizedRefs[strings.ToUpper(strings.TrimSpace(endpoint.Ref))]; !ok {
				continue
			}
		}
		out = append(out, placement.Endpoint{Ref: endpoint.Ref, Pin: endpoint.Pin})
	}
	return out
}

func realizedFragmentRefs(fragment BlockFragment) map[string]struct{} {
	refs := map[string]struct{}{}
	for _, component := range fragment.Realization.Components {
		ref := strings.ToUpper(strings.TrimSpace(component.Ref))
		if ref != "" {
			refs[ref] = struct{}{}
		}
	}
	return refs
}

func (index blockPortEndpointIndex) lookup(instanceID string, port string) []placement.Endpoint {
	return append([]placement.Endpoint(nil), index[newBlockPortKey(instanceID, port)]...)
}

func newBlockPortKey(instanceID string, port string) blockPortKey {
	return blockPortKey{
		instance: strings.ToUpper(strings.TrimSpace(instanceID)),
		port:     strings.ToUpper(strings.TrimSpace(port)),
	}
}

func canonicalInterBlockNetName(connection ConnectionSpec, fromRef, toRef blocks.PortRef) string {
	if alias := strings.TrimSpace(connection.NetAlias); alias != "" {
		return sanitizeGeneratedNetName(alias)
	}
	return sanitizeGeneratedNetName(fromRef.InstanceID + "_" + fromRef.Port + "_" + toRef.InstanceID + "_" + toRef.Port)
}

func sanitizeGeneratedNetName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "NET"
	}
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range value {
		keep := r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if keep {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(builder.String(), "_")
	if out == "" {
		return "NET"
	}
	if len(out) > 128 {
		sum := sha1.Sum([]byte(out))
		out = strings.TrimRight(out[:111], "_") + "_" + hex.EncodeToString(sum[:8])
	}
	return out
}

func interBlockPlacementIssue(path string, message string, refs []string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityWarning,
		Path:     path,
		Message:  message,
		Refs:     append([]string(nil), refs...),
	}
}
