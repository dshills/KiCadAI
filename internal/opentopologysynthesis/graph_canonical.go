package opentopologysynthesis

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const maxCanonicalBranches = 200_000

type graphHashMode int

const (
	graphHashFull graphHashMode = iota
	graphHashTopology
)

type canonicalVertex struct {
	kind      string
	original  string
	label     string
	neighbors []canonicalEdge
}

type canonicalEdge struct {
	vertex int
	label  string
}

type canonicalResult struct {
	encoding string
	order    []int
}

func GraphHash(graph CandidateGraph) (string, error) {
	result, err := canonicalizeGraph(graph, graphHashFull)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(result.encoding))
	return hex.EncodeToString(sum[:]), nil
}

func TopologyHash(graph CandidateGraph) (string, error) {
	result, err := canonicalizeGraph(graph, graphHashTopology)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(result.encoding))
	return hex.EncodeToString(sum[:]), nil
}

func NormalizeGraph(graph CandidateGraph) (CandidateGraph, error) {
	if graphUsesCanonicalIDs(graph) {
		result := CloneGraph(graph)
		for index := range result.Instances {
			canonicalizeSymmetricTerminals(&result.Instances[index])
			slices.SortFunc(result.Instances[index].Terminals, compareTerminalConnections)
		}
		slices.SortFunc(result.Nodes, compareGraphNodes)
		slices.SortFunc(result.Instances, func(left, right GraphInstance) int {
			return cmp.Compare(left.ID, right.ID)
		})
		result.Schema = CandidateGraphSchema
		result.Version = CandidateGraphVersion
		return result, nil
	}
	result, err := canonicalizeGraph(graph, graphHashFull)
	if err != nil {
		return CandidateGraph{}, err
	}
	vertices, nodeIndex, instanceIndex, err := graphCanonicalVertices(graph, graphHashFull)
	if err != nil {
		return CandidateGraph{}, err
	}
	_ = vertices

	nodeIDs := map[string]string{}
	internalIndex := 0
	for _, vertexIndex := range result.order {
		if vertexIndex >= len(graph.Nodes) {
			continue
		}
		node := graph.Nodes[nodeIndex[vertexIndex]]
		if node.Scope == "external" {
			nodeIDs[node.ID] = "port_" + node.SemanticID
		} else {
			nodeIDs[node.ID] = fmt.Sprintf("internal_%03d", internalIndex)
			internalIndex++
		}
	}
	_ = instanceIndex

	normalized := CandidateGraph{Schema: CandidateGraphSchema, Version: CandidateGraphVersion}
	for _, node := range graph.Nodes {
		node.ID = nodeIDs[node.ID]
		normalized.Nodes = append(normalized.Nodes, node)
	}
	for _, source := range graph.Instances {
		instance := source
		instance.Terminals = slices.Clone(source.Terminals)
		for index := range instance.Terminals {
			instance.Terminals[index].Node = nodeIDs[instance.Terminals[index].Node]
		}
		canonicalizeSymmetricTerminals(&instance)
		slices.SortFunc(instance.Terminals, compareTerminalConnections)
		normalized.Instances = append(normalized.Instances, instance)
	}
	slices.SortFunc(normalized.Nodes, compareGraphNodes)
	slices.SortFunc(normalized.Instances, compareNormalizedGraphInstances)
	for index := range normalized.Instances {
		normalized.Instances[index].ID = fmt.Sprintf("primitive_%03d", index)
	}
	return normalized, nil
}

func graphUsesCanonicalIDs(graph CandidateGraph) bool {
	internal := []string{}
	for _, node := range graph.Nodes {
		if node.Scope == "external" {
			if node.ID != "port_"+node.SemanticID {
				return false
			}
			continue
		}
		if node.Scope != "internal" {
			return false
		}
		internal = append(internal, node.ID)
	}
	slices.Sort(internal)
	for index, id := range internal {
		if id != fmt.Sprintf("internal_%03d", index) {
			return false
		}
	}
	instances := make([]string, 0, len(graph.Instances))
	for _, instance := range graph.Instances {
		instances = append(instances, instance.ID)
	}
	slices.Sort(instances)
	for index, id := range instances {
		if id != fmt.Sprintf("primitive_%03d", index) {
			return false
		}
	}
	return true
}

func canonicalizeSymmetricTerminals(instance *GraphInstance) {
	if !slices.Contains([]string{"capacitor", "inductor", "resistor"}, instance.Kind) {
		return
	}
	left, right := -1, -1
	for index, terminal := range instance.Terminals {
		switch terminal.Terminal {
		case "A":
			left = index
		case "B":
			right = index
		}
	}
	if left >= 0 && right >= 0 && instance.Terminals[left].Node > instance.Terminals[right].Node {
		instance.Terminals[left].Node, instance.Terminals[right].Node = instance.Terminals[right].Node, instance.Terminals[left].Node
	}
}

func compareNormalizedGraphInstances(left, right GraphInstance) int {
	if comparison := cmp.Or(
		cmp.Compare(left.Kind, right.Kind),
		cmp.Compare(left.PrimitiveKey, right.PrimitiveKey),
		cmp.Compare(canonicalOptionalFloat(left.ValueSI), canonicalOptionalFloat(right.ValueSI)),
		cmp.Compare(len(left.Terminals), len(right.Terminals)),
	); comparison != 0 {
		return comparison
	}
	for index := range left.Terminals {
		if comparison := compareTerminalConnections(left.Terminals[index], right.Terminals[index]); comparison != 0 {
			return comparison
		}
	}
	return cmp.Compare(left.ID, right.ID)
}

func CanonicalGraphJSON(graph CandidateGraph) ([]byte, error) {
	normalized, err := NormalizeGraph(graph)
	if err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}

func canonicalizeGraph(graph CandidateGraph, mode graphHashMode) (canonicalResult, error) {
	vertices, _, _, err := graphCanonicalVertices(graph, mode)
	if err != nil {
		return canonicalResult{}, err
	}
	colors := initialCanonicalColors(vertices)
	branches := 0
	return canonicalSearch(vertices, colors, &branches)
}

func graphCanonicalVertices(graph CandidateGraph, mode graphHashMode) ([]canonicalVertex, []int, []int, error) {
	if graph.Schema != CandidateGraphSchema || graph.Version != CandidateGraphVersion {
		return nil, nil, nil, errors.New("candidate graph schema/version is invalid")
	}
	nodes := append([]GraphNode(nil), graph.Nodes...)
	nodeIndex := make([]int, len(nodes))
	for index := range nodeIndex {
		nodeIndex[index] = index
	}
	instances := append([]GraphInstance(nil), graph.Instances...)
	instanceIndex := make([]int, len(instances))
	for index := range instanceIndex {
		instanceIndex[index] = index
	}
	nodeByID := map[string]int{}
	vertices := make([]canonicalVertex, 0, len(nodes)+len(instances))
	for index, node := range nodes {
		if _, duplicate := nodeByID[node.ID]; duplicate || node.ID == "" {
			return nil, nil, nil, errors.New("candidate graph node IDs must be nonempty and unique")
		}
		nodeByID[node.ID] = index
		label := "node:internal:" + node.Role
		if node.Scope == "external" {
			label = strings.Join([]string{"node", "external", node.SemanticKind, node.SemanticID, node.Domain, node.Role}, ":")
		} else if node.Scope != "internal" {
			return nil, nil, nil, errors.New("candidate graph node scope is invalid")
		}
		vertices = append(vertices, canonicalVertex{kind: "node", original: node.ID, label: label})
	}
	instanceByID := map[string]bool{}
	for index, instance := range instances {
		if instance.ID == "" || instanceByID[instance.ID] {
			return nil, nil, nil, errors.New("candidate graph instance IDs must be nonempty and unique")
		}
		instanceByID[instance.ID] = true
		label := "instance:" + instance.Kind
		if mode == graphHashFull {
			label += ":" + instance.PrimitiveKey + ":" + canonicalOptionalFloat(instance.ValueSI)
		}
		vertexIndex := len(nodes) + index
		vertices = append(vertices, canonicalVertex{kind: "instance", original: instance.ID, label: label})
		seenTerminals := map[string]bool{}
		for _, terminal := range instance.Terminals {
			nodeVertex, exists := nodeByID[terminal.Node]
			if !exists {
				return nil, nil, nil, errors.New("candidate graph terminal refers to an unknown node")
			}
			if terminal.Terminal == "" || seenTerminals[terminal.Terminal] {
				return nil, nil, nil, errors.New("candidate graph terminal names must be nonempty and unique per instance")
			}
			seenTerminals[terminal.Terminal] = true
			edgeLabel := canonicalTerminalLabel(instance.Kind, terminal.Terminal)
			vertices[vertexIndex].neighbors = append(vertices[vertexIndex].neighbors, canonicalEdge{vertex: nodeVertex, label: edgeLabel})
			vertices[nodeVertex].neighbors = append(vertices[nodeVertex].neighbors, canonicalEdge{vertex: vertexIndex, label: edgeLabel})
		}
	}
	for index := range vertices {
		slices.SortFunc(vertices[index].neighbors, func(left, right canonicalEdge) int {
			return cmp.Or(cmp.Compare(left.label, right.label), cmp.Compare(left.vertex, right.vertex))
		})
	}
	return vertices, nodeIndex, instanceIndex, nil
}

func initialCanonicalColors(vertices []canonicalVertex) []int {
	labels := make([]string, len(vertices))
	for index, vertex := range vertices {
		labels[index] = vertex.label
	}
	return ranksForCanonicalSignatures(labels)
}

func refineCanonicalColors(vertices []canonicalVertex, colors []int) []int {
	current := append([]int(nil), colors...)
	for {
		signatures := make([]string, len(vertices))
		for index, vertex := range vertices {
			neighbors := make([]string, 0, len(vertex.neighbors))
			for _, edge := range vertex.neighbors {
				neighbors = append(neighbors, edge.label+":"+strconv.Itoa(current[edge.vertex]))
			}
			slices.Sort(neighbors)
			signatures[index] = strconv.Itoa(current[index]) + "|" + strings.Join(neighbors, ",")
		}
		next := ranksForCanonicalSignatures(signatures)
		if sameCanonicalPartition(current, next) {
			return next
		}
		current = next
	}
}

// sameCanonicalPartition compares color classes rather than their numeric
// labels. Rank compression can deterministically rename an already-stable
// partition on successive refinement rounds; requiring byte-for-byte color
// equality can therefore oscillate forever even though no class is being
// split. Weisfeiler-Lehman refinement is complete for this round as soon as
// the equivalence relation between vertices stops changing.
func sameCanonicalPartition(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	forward := make(map[int]int, len(left))
	reverse := make(map[int]int, len(right))
	for index := range left {
		if mapped, ok := forward[left[index]]; ok && mapped != right[index] {
			return false
		}
		if mapped, ok := reverse[right[index]]; ok && mapped != left[index] {
			return false
		}
		forward[left[index]] = right[index]
		reverse[right[index]] = left[index]
	}
	return true
}

func ranksForCanonicalSignatures(signatures []string) []int {
	unique := append([]string(nil), signatures...)
	slices.Sort(unique)
	unique = slices.Compact(unique)
	ranks := make(map[string]int, len(unique))
	for index, signature := range unique {
		ranks[signature] = index
	}
	result := make([]int, len(signatures))
	for index, signature := range signatures {
		result[index] = ranks[signature]
	}
	return result
}

func canonicalSearch(vertices []canonicalVertex, colors []int, branches *int) (canonicalResult, error) {
	*branches++
	if *branches > maxCanonicalBranches {
		return canonicalResult{}, fmt.Errorf("canonical labeling exceeded %d branches", maxCanonicalBranches)
	}
	colors = refineCanonicalColors(vertices, colors)
	class := firstAmbiguousCanonicalClass(colors)
	if len(class) == 0 {
		order := canonicalOrder(colors)
		return canonicalResult{encoding: encodeCanonicalGraph(vertices, order), order: order}, nil
	}
	maxColor := 0
	for _, color := range colors {
		if color > maxColor {
			maxColor = color
		}
	}
	best := canonicalResult{}
	twinClasses := map[string]struct{}{}
	for _, vertex := range class {
		// Vertices with identical labels and exact labeled neighbors are true
		// twins: transposing them is a graph automorphism. Exploring every order
		// of a parallel component bank is factorial work but cannot produce a
		// different canonical encoding, so keep one proven representative.
		twinKey := canonicalExactTwinKey(vertices, vertex)
		if _, duplicate := twinClasses[twinKey]; duplicate {
			continue
		}
		twinClasses[twinKey] = struct{}{}
		next := append([]int(nil), colors...)
		next[vertex] = maxColor + 1
		candidate, err := canonicalSearch(vertices, next, branches)
		if err != nil {
			return canonicalResult{}, err
		}
		if best.encoding == "" || candidate.encoding < best.encoding {
			best = candidate
		}
	}
	return best, nil
}

func canonicalExactTwinKey(vertices []canonicalVertex, vertex int) string {
	var builder strings.Builder
	builder.WriteString(vertices[vertex].kind)
	builder.WriteByte('|')
	builder.WriteString(vertices[vertex].label)
	// graphCanonicalVertices normalizes neighbor order, but keep the pruning
	// key independently order-invariant so future callers cannot make exact
	// twin detection depend on construction order.
	neighbors := slices.Clone(vertices[vertex].neighbors)
	slices.SortFunc(neighbors, func(left, right canonicalEdge) int {
		return cmp.Or(cmp.Compare(left.label, right.label), cmp.Compare(left.vertex, right.vertex))
	})
	for _, edge := range neighbors {
		builder.WriteByte('|')
		builder.WriteString(edge.label)
		builder.WriteByte(':')
		builder.WriteString(strconv.Itoa(edge.vertex))
	}
	return builder.String()
}

func firstAmbiguousCanonicalClass(colors []int) []int {
	classes := map[int][]int{}
	for vertex, color := range colors {
		classes[color] = append(classes[color], vertex)
	}
	selectedColor := -1
	selectedSize := 0
	for color, vertices := range classes {
		if len(vertices) < 2 {
			continue
		}
		if selectedColor < 0 || len(vertices) < selectedSize || len(vertices) == selectedSize && color < selectedColor {
			selectedColor = color
			selectedSize = len(vertices)
		}
	}
	if selectedColor < 0 {
		return nil
	}
	result := append([]int(nil), classes[selectedColor]...)
	slices.Sort(result)
	return result
}

func canonicalOrder(colors []int) []int {
	order := make([]int, len(colors))
	for index := range order {
		order[index] = index
	}
	slices.SortFunc(order, func(left, right int) int {
		return cmp.Or(cmp.Compare(colors[left], colors[right]), cmp.Compare(left, right))
	})
	return order
}

func encodeCanonicalGraph(vertices []canonicalVertex, order []int) string {
	position := make([]int, len(order))
	for index, vertex := range order {
		position[vertex] = index
	}
	parts := make([]string, 0, len(vertices)*2)
	for _, vertex := range order {
		parts = append(parts, "V:"+vertices[vertex].label)
	}
	for leftPosition, leftVertex := range order {
		edgeLabels := map[int][]string{}
		for _, edge := range vertices[leftVertex].neighbors {
			rightPosition := position[edge.vertex]
			if rightPosition < leftPosition {
				continue
			}
			edgeLabels[rightPosition] = append(edgeLabels[rightPosition], edge.label)
		}
		rightPositions := make([]int, 0, len(edgeLabels))
		for right := range edgeLabels {
			rightPositions = append(rightPositions, right)
		}
		slices.Sort(rightPositions)
		for _, right := range rightPositions {
			labels := edgeLabels[right]
			slices.Sort(labels)
			parts = append(parts, fmt.Sprintf("E:%d:%d:%s", leftPosition, right, strings.Join(labels, "+")))
		}
	}
	return strings.Join(parts, "\n")
}

func canonicalTerminalLabel(kind, terminal string) string {
	if slices.Contains([]string{"capacitor", "inductor", "resistor"}, kind) &&
		(terminal == "A" || terminal == "B") {
		return "PASSIVE"
	}
	return terminal
}

func canonicalOptionalFloat(value *float64) string {
	if value == nil {
		return "-"
	}
	return strconv.FormatFloat(*value, 'g', -1, 64)
}
