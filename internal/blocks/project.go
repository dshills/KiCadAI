package blocks

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"kicadai/internal/transactions"
)

const DefaultGeneratedProjectName = "generated_blocks"
const projectSchematicGridMM = 1.27

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
	operations, err = normalizeProjectSchematicSymbolPositions(operations)
	if err != nil {
		return transactions.Transaction{}, err
	}
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

func normalizeProjectSchematicSymbolPositions(operations []transactions.Operation) ([]transactions.Operation, error) {
	normalized := append([]transactions.Operation(nil), operations...)
	occupied := map[projectPointKey]struct{}{}
	for index, operation := range normalized {
		if operation.Op != transactions.OpAddSymbol {
			continue
		}
		var payload transactions.AddSymbolOperation
		if err := decodeBlockOperation(operation, &payload); err != nil {
			return nil, fmt.Errorf("decode add_symbol operation %d: %w", index, err)
		}
		position := safeProjectSchematicSymbolPosition(payload.At, payload.Pins, payload.Rotation, occupied)
		payload.At = position
		updated, err := wrapOperation(transactions.OpAddSymbol, payload)
		if err != nil {
			return nil, fmt.Errorf("encode normalized add_symbol operation %d: %w", index, err)
		}
		normalized[index] = updated
		for _, pin := range payload.Pins {
			if strings.TrimSpace(pin.Number) == "" {
				continue
			}
			xMM, yMM := rotatedProjectPinOffset(pin.XMM, pin.YMM, payload.Rotation)
			occupied[projectPointKeyFromMM(position.XMM+xMM, position.YMM+yMM)] = struct{}{}
		}
	}
	return normalized, nil
}

type projectEndpointKey struct {
	ref string
	pin string
}

type projectEndpointSet struct {
	parent map[projectEndpointKey]projectEndpointKey
}

type projectPoint struct {
	xMM  float64
	yMM  float64
	role string
}

type projectPseudoEdge struct {
	pseudo   projectEndpointKey
	endpoint projectEndpointKey
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
	var pseudoEdges []projectPseudoEdge
	anchors, err := projectEndpointAnchors(operations)
	if err != nil {
		return nil, err
	}
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
		fromPseudo := isProjectPseudoRef(from.ref, pseudoRefs)
		toPseudo := isProjectPseudoRef(to.ref, pseudoRefs)
		switch {
		case fromPseudo && !toPseudo:
			pseudoEdges = append(pseudoEdges, projectPseudoEdge{pseudo: from, endpoint: to})
		case toPseudo && !fromPseudo:
			pseudoEdges = append(pseudoEdges, projectPseudoEdge{pseudo: to, endpoint: from})
		}
		netName := strings.TrimSpace(payload.NetName)
		if netName != "" {
			netNames[from] = netName
			netNames[to] = netName
		}
	}
	groups := map[projectEndpointKey][]projectEndpointKey{}
	groupHasGenerated := map[projectEndpointKey]bool{}
	groupNetNames := map[projectEndpointKey]string{}
	groupPseudoEndpoints := map[projectEndpointKey][]projectEndpointKey{}
	groupPseudoLabelEndpoints := map[projectEndpointKey][]projectEndpointKey{}
	for endpoint := range set.parent {
		if isProjectPseudoRef(endpoint.ref, pseudoRefs) {
			root := set.find(endpoint)
			groupPseudoEndpoints[root] = append(groupPseudoEndpoints[root], endpoint)
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
	for _, edge := range pseudoEdges {
		root := set.find(edge.endpoint)
		groupPseudoLabelEndpoints[root] = append(groupPseudoLabelEndpoints[root], edge.endpoint)
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
		pseudoEndpoints := groupPseudoEndpoints[root]
		sort.Slice(endpoints, func(i, j int) bool {
			return projectEndpointLess(endpoints[i], endpoints[j])
		})
		if len(endpoints) >= 1 && len(pseudoEndpoints) >= 1 {
			sort.Slice(pseudoEndpoints, func(i, j int) bool {
				return projectEndpointLess(pseudoEndpoints[i], pseudoEndpoints[j])
			})
			if !projectGroupSuppressesMaterializedLabel(endpoints, anchors) {
				labelEndpoint := preferredProjectLabelEndpoint(endpoints, groupPseudoLabelEndpoints[root])
				labelEndpoint = preferredProjectMaterializedLabelEndpoint(labelEndpoint, endpoints, len(pseudoEndpoints), len(endpoints), anchors)
				label, err := materializedPortLabel(labelEndpoint, pseudoEndpoints[0], groupNetNames[root], len(pseudoEndpoints), anchors)
				if err != nil {
					return nil, err
				}
				if label.Op != "" {
					out = append(out, label)
				}
			}
		}
		netName := groupNetNames[root]
		if netName == "" && len(endpoints) > 0 {
			netName = "NET_" + endpoints[0].ref + "_" + endpoints[0].pin
		}
		if pseudoNetName, ok := projectPseudoEndpointNetName(pseudoEndpoints, netName); ok {
			netName = pseudoNetName
		}
		if len(endpoints) < 2 {
			continue
		}
		first := endpoints[0]
		for _, endpoint := range endpoints[1:] {
			operation, issues := ConnectOperation(first.ref, first.pin, endpoint.ref, endpoint.pin, netName)
			if len(issues) != 0 {
				errs := make([]error, 0, len(issues))
				for _, issue := range issues {
					errs = append(errs, errors.New(issue.Message))
				}
				return nil, fmt.Errorf("failed to connect %s.%s to %s.%s on %s: %w", first.ref, first.pin, endpoint.ref, endpoint.pin, netName, errors.Join(errs...))
			}
			out = append(out, operation)
		}
	}
	return out, nil
}

func projectPseudoEndpointNetName(pseudoEndpoints []projectEndpointKey, fallback string) (string, bool) {
	if len(pseudoEndpoints) == 0 {
		return "", false
	}
	netName := terminalValue(pseudoEndpoints[0], fallback)
	for _, endpoint := range pseudoEndpoints[1:] {
		if terminalValue(endpoint, fallback) != netName {
			return "", false
		}
	}
	return netName, true
}

func isProjectPseudoRef(ref string, pseudoRefs map[string]struct{}) bool {
	_, ok := pseudoRefs[ref]
	return ok
}

func preferredProjectLabelEndpoint(endpoints []projectEndpointKey, candidates []projectEndpointKey) projectEndpointKey {
	if len(candidates) == 0 {
		return endpoints[0]
	}
	candidateSet := make(map[projectEndpointKey]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidateSet[candidate] = struct{}{}
	}
	var valid []projectEndpointKey
	for _, endpoint := range endpoints {
		if _, ok := candidateSet[endpoint]; ok {
			valid = append(valid, endpoint)
		}
	}
	if len(valid) == 0 {
		return endpoints[0]
	}
	sort.Slice(valid, func(i, j int) bool {
		return projectEndpointLess(valid[i], valid[j])
	})
	return valid[0]
}

func projectEndpointAnchors(operations []transactions.Operation) (map[projectEndpointKey]projectPoint, error) {
	anchors := map[projectEndpointKey]projectPoint{}
	for index, operation := range operations {
		if operation.Op != transactions.OpAddSymbol {
			continue
		}
		var payload transactions.AddSymbolOperation
		if err := decodeBlockOperation(operation, &payload); err != nil {
			return nil, fmt.Errorf("decode add_symbol operation %d: %w", index, err)
		}
		for _, pin := range payload.Pins {
			number := strings.TrimSpace(pin.Number)
			if number == "" {
				continue
			}
			xMM, yMM := rotatedProjectPinOffset(pin.XMM, pin.YMM, payload.Rotation)
			anchor := projectPoint{
				xMM:  payload.At.XMM + xMM,
				yMM:  payload.At.YMM + yMM,
				role: strings.TrimSpace(payload.Role),
			}
			anchors[projectEndpointKey{ref: strings.TrimSpace(payload.Ref), pin: number}] = anchor
		}
	}
	return anchors, nil
}

type projectPointKey struct {
	X int64
	Y int64
}

func projectPointKeyFromMM(xMM float64, yMM float64) projectPointKey {
	const scale = 1_000_000
	return projectPointKey{X: int64(math.Round(xMM * scale)), Y: int64(math.Round(yMM * scale))}
}

func safeProjectSchematicSymbolPosition(requested transactions.Point, pins []transactions.PinSpec, rotationDeg float64, occupied map[projectPointKey]struct{}) transactions.Point {
	position := transactions.Point{XMM: snapProjectSchematicMM(requested.XMM), YMM: snapProjectSchematicMM(requested.YMM)}
	if !projectSymbolPinAnchorsCollide(position, pins, rotationDeg, occupied) {
		return position
	}
	for radius := 1; radius <= 8; radius++ {
		for _, offset := range projectGridPerimeterOffsets(radius) {
			candidate := transactions.Point{XMM: position.XMM + offset.XMM, YMM: position.YMM + offset.YMM}
			if !projectSymbolPinAnchorsCollide(candidate, pins, rotationDeg, occupied) {
				return candidate
			}
		}
	}
	return position
}

func projectSymbolPinAnchorsCollide(position transactions.Point, pins []transactions.PinSpec, rotationDeg float64, occupied map[projectPointKey]struct{}) bool {
	if len(pins) == 0 || len(occupied) == 0 {
		return false
	}
	for _, pin := range pins {
		if strings.TrimSpace(pin.Number) == "" {
			continue
		}
		xMM, yMM := rotatedProjectPinOffset(pin.XMM, pin.YMM, rotationDeg)
		if _, ok := occupied[projectPointKeyFromMM(position.XMM+xMM, position.YMM+yMM)]; ok {
			return true
		}
	}
	return false
}

func projectGridPerimeterOffsets(radius int) []transactions.Point {
	if radius <= 0 {
		return nil
	}
	offsets := make([]transactions.Point, 0, radius*8)
	for dx := -radius; dx <= radius; dx++ {
		offsets = append(offsets, transactions.Point{XMM: float64(dx) * projectSchematicGridMM, YMM: float64(-radius) * projectSchematicGridMM})
		offsets = append(offsets, transactions.Point{XMM: float64(dx) * projectSchematicGridMM, YMM: float64(radius) * projectSchematicGridMM})
	}
	for dy := -radius + 1; dy <= radius-1; dy++ {
		offsets = append(offsets, transactions.Point{XMM: float64(-radius) * projectSchematicGridMM, YMM: float64(dy) * projectSchematicGridMM})
		offsets = append(offsets, transactions.Point{XMM: float64(radius) * projectSchematicGridMM, YMM: float64(dy) * projectSchematicGridMM})
	}
	return offsets
}

func snapProjectSchematicMM(value float64) float64 {
	return math.Round(value/projectSchematicGridMM) * projectSchematicGridMM
}

func rotatedProjectPinOffset(xMM float64, yMM float64, rotationDeg float64) (float64, float64) {
	if rotationDeg == 0 {
		return xMM, yMM
	}
	radians := rotationDeg * math.Pi / 180
	cosine := math.Cos(radians)
	sine := math.Sin(radians)
	return xMM*cosine - yMM*sine, xMM*sine + yMM*cosine
}

func materializedPortLabel(endpoint projectEndpointKey, pseudoEndpoint projectEndpointKey, netName string, pseudoEndpointCount int, anchors map[projectEndpointKey]projectPoint) (transactions.Operation, error) {
	anchor, ok := anchors[endpoint]
	if !ok || projectEndpointIsExternalTerminal(anchor.role) {
		return transactions.Operation{}, nil
	}
	netName = strings.TrimSpace(netName)
	if netName == "" {
		netName = "NET_" + endpoint.ref + "_" + endpoint.pin
	}
	text := terminalValue(pseudoEndpoint, netName)
	if pseudoEndpointCount > 1 {
		text = netName
	}
	return wrapOperation(transactions.OpAddLabel, transactions.AddLabelOperation{
		Op:   transactions.OpAddLabel,
		Text: text,
		At:   transactions.Point{XMM: anchor.xMM, YMM: anchor.yMM},
		Kind: "local",
	})
}

func preferredProjectMaterializedLabelEndpoint(preferred projectEndpointKey, endpoints []projectEndpointKey, pseudoEndpointCount int, concreteEndpointCount int, anchors map[projectEndpointKey]projectPoint) projectEndpointKey {
	anchor, ok := anchors[preferred]
	if !ok || !projectEndpointSuppressesMaterializedLabel(anchor.role, pseudoEndpointCount, concreteEndpointCount) {
		return preferred
	}
	for _, endpoint := range endpoints {
		candidate, ok := anchors[endpoint]
		if !ok || projectEndpointIsExternalTerminal(candidate.role) || projectEndpointSuppressesMaterializedLabel(candidate.role, pseudoEndpointCount, concreteEndpointCount) {
			continue
		}
		return endpoint
	}
	return preferred
}

func projectEndpointSuppressesMaterializedLabel(role string, pseudoEndpointCount int, concreteEndpointCount int) bool {
	if pseudoEndpointCount < 2 && concreteEndpointCount < 2 {
		return false
	}
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "dc_blocking_capacitor", "decoupling_capacitor", "input_coupling", "output_coupling":
		return true
	default:
		return false
	}
}

func projectGroupSuppressesMaterializedLabel(endpoints []projectEndpointKey, anchors map[projectEndpointKey]projectPoint) bool {
	if len(endpoints) < 2 {
		return false
	}
	for _, endpoint := range endpoints {
		anchor, ok := anchors[endpoint]
		if !ok {
			continue
		}
		if projectEndpointSuppressesMaterializedLabel(anchor.role, 0, len(endpoints)) {
			return true
		}
	}
	return false
}

func projectEndpointIsExternalTerminal(role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return false
	}
	return role == "connector" ||
		role == "generated_terminal" ||
		strings.Contains(role, "header") ||
		strings.Contains(role, "receptacle") ||
		strings.Contains(role, "terminal")
}

func terminalValue(endpoint projectEndpointKey, netName string) string {
	port := strings.TrimSpace(endpoint.pin)
	if port == "" {
		return netName
	}
	return strings.ToUpper(port)
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
