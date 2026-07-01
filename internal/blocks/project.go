package blocks

import (
	"fmt"
	"sort"
	"strings"

	"kicadai/internal/transactions"
)

const DefaultGeneratedProjectName = "generated_blocks"

func ProjectTransactionForBlockOutput(projectName string, output BlockOutput, overwrite bool) (transactions.Transaction, error) {
	return ProjectTransactionForBlockOutputPtr(projectName, &output, overwrite)
}

func ProjectTransactionForBlockOutputPtr(projectName string, output *BlockOutput, overwrite bool) (transactions.Transaction, error) {
	if output == nil {
		return transactions.Transaction{}, fmt.Errorf("block output is required")
	}
	refs := make(map[string]struct{}, len(output.Instance.Refs))
	for _, ref := range output.Instance.Refs {
		if _, exists := refs[ref]; exists {
			return transactions.Transaction{}, fmt.Errorf("duplicate generated reference %s in instance %s", ref, output.Instance.InstanceID)
		}
		refs[ref] = struct{}{}
	}
	pseudoRefs := map[string]struct{}{}
	if instanceID := strings.TrimSpace(output.Instance.InstanceID); instanceID != "" {
		pseudoRefs[instanceID] = struct{}{}
	}
	return projectTransaction(projectName, output.Operations, refs, pseudoRefs, overwrite)
}

func ProjectTransactionForCompositionOutput(projectName string, output CompositionOutput, overwrite bool) (transactions.Transaction, error) {
	refCount := 0
	for _, instance := range output.Instances {
		refCount += len(instance.Refs)
	}
	refs := make(map[string]struct{}, refCount)
	owners := make(map[string]string, refCount)
	pseudoRefs := make(map[string]struct{}, len(output.Instances))
	for _, instance := range output.Instances {
		if instanceID := strings.TrimSpace(instance.InstanceID); instanceID != "" {
			pseudoRefs[instanceID] = struct{}{}
		}
		for _, ref := range instance.Refs {
			if owner, exists := owners[ref]; exists {
				if owner == instance.InstanceID {
					return transactions.Transaction{}, fmt.Errorf("duplicate generated reference %s within instance %s", ref, owner)
				}
				return transactions.Transaction{}, fmt.Errorf("duplicate generated reference %s in instances %s and %s", ref, owner, instance.InstanceID)
			}
			owners[ref] = instance.InstanceID
			refs[ref] = struct{}{}
		}
	}
	return projectTransaction(projectName, output.Operations, refs, pseudoRefs, overwrite)
}

func projectTransaction(projectName string, operations []transactions.Operation, generatedRefs map[string]struct{}, pseudoRefs map[string]struct{}, overwrite bool) (transactions.Transaction, error) {
	if projectName == "" {
		projectName = DefaultGeneratedProjectName
	}
	create, err := wrapOperation(transactions.OpCreateProject, transactions.CreateProjectOperation{Op: transactions.OpCreateProject, Name: projectName})
	if err != nil {
		return transactions.Transaction{}, err
	}
	write, err := wrapOperation(transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject, Overwrite: overwrite})
	if err != nil {
		return transactions.Transaction{}, err
	}
	tx := transactions.Transaction{Name: projectName, Project: projectName, Operations: []transactions.Operation{create}}
	materializedConnects, err := materializedGeneratedConnects(operations, generatedRefs, pseudoRefs)
	if err != nil {
		return transactions.Transaction{}, err
	}
	for _, operation := range operations {
		if operation.Op == transactions.OpConnect {
			continue
		}
		tx.Operations = append(tx.Operations, operation)
	}
	tx.Operations = append(tx.Operations, materializedConnects...)
	tx.Operations = append(tx.Operations, write)
	return tx, nil
}

type projectEndpointKey struct {
	ref string
	pin string
}

type projectEndpointSet struct {
	parent map[projectEndpointKey]projectEndpointKey
}

func newProjectEndpointSet() projectEndpointSet {
	return projectEndpointSet{parent: map[projectEndpointKey]projectEndpointKey{}}
}

func (set projectEndpointSet) find(endpoint projectEndpointKey) projectEndpointKey {
	if endpoint.ref == "" || endpoint.pin == "" {
		return endpoint
	}
	parent, ok := set.parent[endpoint]
	if !ok {
		set.parent[endpoint] = endpoint
		return endpoint
	}
	root := endpoint
	for {
		parent = set.parent[root]
		if parent == root {
			break
		}
		root = parent
	}
	for endpoint != root {
		next := set.parent[endpoint]
		set.parent[endpoint] = root
		endpoint = next
	}
	return root
}

func (set projectEndpointSet) union(a, b projectEndpointKey) {
	rootA := set.find(a)
	rootB := set.find(b)
	if rootA == rootB {
		return
	}
	if projectEndpointLess(rootB, rootA) {
		rootA, rootB = rootB, rootA
	}
	set.parent[rootB] = rootA
}

func materializedGeneratedConnects(operations []transactions.Operation, generatedRefs map[string]struct{}, pseudoRefs map[string]struct{}) ([]transactions.Operation, error) {
	set := newProjectEndpointSet()
	netNames := map[projectEndpointKey]string{}
	for _, operation := range operations {
		if operation.Op != transactions.OpConnect {
			continue
		}
		var payload transactions.ConnectOperation
		if err := decodeBlockOperation(operation, &payload); err != nil {
			return nil, fmt.Errorf("decode connect operation: %w", err)
		}
		from := projectEndpoint(payload.From)
		to := projectEndpoint(payload.To)
		set.union(from, to)
		netName := strings.TrimSpace(payload.NetName)
		if netName != "" {
			netNames[from] = netName
			netNames[to] = netName
		}
	}
	groups := map[projectEndpointKey][]projectEndpointKey{}
	groupHasGenerated := map[projectEndpointKey]bool{}
	groupNetNames := map[projectEndpointKey]string{}
	for endpoint := range set.parent {
		if _, pseudo := pseudoRefs[endpoint.ref]; pseudo {
			continue
		}
		root := set.find(endpoint)
		groups[root] = append(groups[root], endpoint)
		if _, ok := generatedRefs[endpoint.ref]; ok {
			groupHasGenerated[root] = true
		}
		if netName := netNames[endpoint]; netName != "" {
			groupNetNames[root] = preferredProjectNetName(groupNetNames[root], netName)
		}
	}
	var roots []projectEndpointKey
	for root := range groups {
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool {
		return projectEndpointLess(roots[i], roots[j])
	})
	var out []transactions.Operation
	for _, root := range roots {
		if !groupHasGenerated[root] {
			continue
		}
		endpoints := groups[root]
		if len(endpoints) < 2 {
			continue
		}
		sort.Slice(endpoints, func(i, j int) bool {
			return projectEndpointLess(endpoints[i], endpoints[j])
		})
		netName := groupNetNames[root]
		if netName == "" {
			netName = "NET_" + endpoints[0].ref + "_" + endpoints[0].pin
		}
		first := endpoints[0]
		for _, endpoint := range endpoints[1:] {
			operation, issues := ConnectOperation(first.ref, first.pin, endpoint.ref, endpoint.pin, netName)
			if len(issues) != 0 {
				return nil, fmt.Errorf(issues[0].Message)
			}
			out = append(out, operation)
		}
	}
	return out, nil
}

func projectEndpoint(endpoint transactions.Endpoint) projectEndpointKey {
	return projectEndpointKey{
		ref: strings.TrimSpace(endpoint.Ref),
		pin: strings.TrimSpace(endpoint.Pin),
	}
}

func projectEndpointLess(a, b projectEndpointKey) bool {
	if a.ref != b.ref {
		return a.ref < b.ref
	}
	return a.pin < b.pin
}

func preferredProjectNetName(existing, candidate string) string {
	existing = strings.TrimSpace(existing)
	candidate = strings.TrimSpace(candidate)
	if existing == "" {
		return candidate
	}
	if candidate == "" {
		return existing
	}
	existingGenerated := strings.Contains(strings.ToLower(existing), "_")
	candidateGenerated := strings.Contains(strings.ToLower(candidate), "_")
	if existingGenerated != candidateGenerated && !candidateGenerated {
		return candidate
	}
	if len(candidate) < len(existing) {
		return candidate
	}
	return existing
}
