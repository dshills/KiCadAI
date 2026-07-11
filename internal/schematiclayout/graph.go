package schematiclayout

import "sort"

type placementCell struct {
	rank   int
	order  int
	island int
}

type placementGraph struct {
	refs       []string
	neighbors  map[string]map[string]struct{}
	directed   map[string]map[string]struct{}
	components map[string]Component
}

func buildPlacementGraph(request Request) placementGraph {
	graph := placementGraph{
		neighbors:  map[string]map[string]struct{}{},
		directed:   map[string]map[string]struct{}{},
		components: map[string]Component{},
	}
	for _, component := range request.Components {
		graph.refs = append(graph.refs, component.Ref)
		graph.components[component.Ref] = component
		graph.neighbors[component.Ref] = map[string]struct{}{}
		graph.directed[component.Ref] = map[string]struct{}{}
	}
	for _, component := range request.Components {
		for _, near := range component.Near {
			graph.addUndirected(component.Ref, near)
		}
		for _, above := range component.Above {
			graph.addUndirected(component.Ref, above)
		}
		for _, left := range component.RightOf {
			graph.addUndirected(component.Ref, left)
			graph.addDirected(left, component.Ref)
		}
	}
	for _, net := range request.Nets {
		if containsRole(net.Role, "no_connect") {
			continue
		}
		refs := uniqueEndpointRefs(net.Endpoints, graph.components)
		powerLike := containsRole(net.Role, "power", "ground", "return", "shield", "negative_rail")
		// Two-terminal power paths carry useful stage topology (for example a
		// connector feeding a regulator). High-fanout rails and returns must not
		// collapse otherwise independent functional islands into one graph.
		if powerLike && (len(refs) > 2 || containsRole(net.Role, "ground", "return", "shield")) {
			continue
		}
		for index := 0; index < len(refs); index++ {
			for other := index + 1; other < len(refs); other++ {
				graph.addUndirected(refs[index], refs[other])
				if containsRole(net.Role, "feedback") || powerLike {
					continue
				}
				from, to, ok := graph.direction(refs[index], refs[other], net)
				if ok {
					graph.directed[from][to] = struct{}{}
				}
			}
		}
	}
	return graph
}

func (graph placementGraph) addDirected(first, second string) {
	if first == "" || second == "" || first == second {
		return
	}
	if _, ok := graph.components[first]; !ok {
		return
	}
	if _, ok := graph.components[second]; !ok {
		return
	}
	graph.directed[first][second] = struct{}{}
}

func (graph placementGraph) addUndirected(first, second string) {
	if first == "" || second == "" || first == second {
		return
	}
	if _, ok := graph.components[first]; !ok {
		return
	}
	if _, ok := graph.components[second]; !ok {
		return
	}
	graph.neighbors[first][second] = struct{}{}
	graph.neighbors[second][first] = struct{}{}
}

func (graph placementGraph) direction(first, second string, net Net) (string, string, bool) {
	firstPin := endpointPinRole(net.Endpoints, first, graph.components[first])
	secondPin := endpointPinRole(net.Endpoints, second, graph.components[second])
	if isSourcePin(firstPin) && isSinkPin(secondPin) {
		return first, second, true
	}
	if isSourcePin(secondPin) && isSinkPin(firstPin) {
		return second, first, true
	}
	firstStage := graph.components[first].Stage
	secondStage := graph.components[second].Stage
	if firstStage != secondStage {
		if firstStage < secondStage {
			return first, second, true
		}
		return second, first, true
	}
	return "", "", false
}

func uniqueEndpointRefs(endpoints []Endpoint, components map[string]Component) []string {
	seen := map[string]struct{}{}
	var refs []string
	for _, endpoint := range endpoints {
		if _, ok := components[endpoint.Ref]; !ok {
			continue
		}
		if _, ok := seen[endpoint.Ref]; ok {
			continue
		}
		seen[endpoint.Ref] = struct{}{}
		refs = append(refs, endpoint.Ref)
	}
	sort.Strings(refs)
	return refs
}

func endpointPinRole(endpoints []Endpoint, ref string, component Component) string {
	for _, endpoint := range endpoints {
		if endpoint.Ref != ref {
			continue
		}
		for _, pin := range component.Pins {
			if pin.Number == endpoint.Pin {
				return normalizeRole(pin.Role)
			}
		}
	}
	return ""
}

func isSourcePin(role string) bool {
	return containsNormalizedRole(role, "output", "power_output", "open_collector", "open_emitter")
}

func isSinkPin(role string) bool {
	return containsNormalizedRole(role, "input", "power_input")
}

func planPlacement(request Request) (map[string]placementCell, int, int) {
	graph := buildPlacementGraph(request)
	islands := graph.islands()
	cells := map[string]placementCell{}
	maxRanks := 0
	for islandIndex, refs := range islands {
		ranks := graph.ranks(refs)
		orders := graph.orders(refs, ranks)
		seenRanks := map[int]struct{}{}
		for _, ref := range refs {
			cells[ref] = placementCell{rank: ranks[ref], order: orders[ref], island: islandIndex}
			seenRanks[ranks[ref]] = struct{}{}
		}
		if len(seenRanks) > maxRanks {
			maxRanks = len(seenRanks)
		}
	}
	return cells, len(islands), maxRanks
}

func (graph placementGraph) islands() [][]string {
	seen := map[string]struct{}{}
	var islands [][]string
	for _, root := range graph.refs {
		if _, ok := seen[root]; ok {
			continue
		}
		queue := []string{root}
		seen[root] = struct{}{}
		var refs []string
		for len(queue) != 0 {
			ref := queue[0]
			queue = queue[1:]
			refs = append(refs, ref)
			for _, next := range sortedSet(graph.neighbors[ref]) {
				if _, ok := seen[next]; ok {
					continue
				}
				seen[next] = struct{}{}
				queue = append(queue, next)
			}
		}
		sort.Strings(refs)
		islands = append(islands, refs)
	}
	return islands
}

func (graph placementGraph) ranks(refs []string) map[string]int {
	inIsland := stringSet(refs)
	ranks := map[string]int{}
	fixed := map[string]bool{}
	if len(refs) == 1 {
		component := graph.components[refs[0]]
		if component.RankFixed {
			return map[string]int{refs[0]: component.FlowRank}
		}
		return map[string]int{refs[0]: stageRank(component.Stage)}
	}
	var roots []string
	for _, ref := range refs {
		component := graph.components[ref]
		if component.RankFixed {
			ranks[ref] = component.FlowRank
			fixed[ref] = true
			roots = append(roots, ref)
			continue
		}
		if component.Stage == StageBoundaryInput || containsRole(component.Role, "input_connector", "power_connector") {
			roots = append(roots, ref)
		}
	}
	if len(roots) == 0 {
		for _, ref := range refs {
			if graph.inDegree(ref, inIsland) == 0 && len(graph.directed[ref]) != 0 {
				roots = append(roots, ref)
			}
		}
	}
	if len(roots) == 0 && len(refs) != 0 {
		roots = []string{refs[0]}
	}
	sort.Strings(roots)
	distance := graph.distances(roots, inIsland)
	for _, ref := range refs {
		if fixed[ref] {
			continue
		}
		if value, ok := distance[ref]; ok {
			ranks[ref] = value
		} else {
			ranks[ref] = stageRank(graph.components[ref].Stage)
		}
	}
	graph.relaxCondensedRanks(refs, ranks, fixed)
	return compactRanks(ranks, fixed)
}

func (graph placementGraph) relaxCondensedRanks(refs []string, ranks map[string]int, fixed map[string]bool) {
	components, componentByRef := graph.stronglyConnectedComponents(refs)
	componentRank := map[int]int{}
	componentFixed := map[int]bool{}
	for index, members := range components {
		componentRank[index] = ranks[members[0]]
		for _, ref := range members {
			if ranks[ref] < componentRank[index] {
				componentRank[index] = ranks[ref]
			}
			if fixed[ref] {
				componentRank[index] = ranks[ref]
				componentFixed[index] = true
			}
		}
	}
	edges := map[int]map[int]struct{}{}
	indegree := map[int]int{}
	for index := range components {
		edges[index] = map[int]struct{}{}
	}
	for _, from := range refs {
		fromComponent := componentByRef[from]
		for _, to := range sortedSet(graph.directed[from]) {
			toComponent, ok := componentByRef[to]
			if !ok || fromComponent == toComponent {
				continue
			}
			if _, exists := edges[fromComponent][toComponent]; exists {
				continue
			}
			edges[fromComponent][toComponent] = struct{}{}
			indegree[toComponent]++
		}
	}
	var ready []int
	for index := range components {
		if indegree[index] == 0 {
			ready = append(ready, index)
		}
	}
	sort.Ints(ready)
	for len(ready) != 0 {
		current := ready[0]
		ready = ready[1:]
		for _, next := range sortedIntSet(edges[current]) {
			if !componentFixed[next] && componentRank[current]+1 > componentRank[next] {
				componentRank[next] = componentRank[current] + 1
			}
			indegree[next]--
			if indegree[next] == 0 {
				ready = append(ready, next)
				sort.Ints(ready)
			}
		}
	}
	for index, members := range components {
		for _, ref := range members {
			if !fixed[ref] {
				ranks[ref] = componentRank[index]
			}
		}
	}
}

func (graph placementGraph) stronglyConnectedComponents(refs []string) ([][]string, map[string]int) {
	allowed := stringSet(refs)
	index := 0
	indexes := map[string]int{}
	lowlinks := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	var components [][]string
	var visit func(string)
	visit = func(ref string) {
		indexes[ref] = index
		lowlinks[ref] = index
		index++
		stack = append(stack, ref)
		onStack[ref] = true
		for _, next := range sortedSet(graph.directed[ref]) {
			if _, ok := allowed[next]; !ok {
				continue
			}
			if _, seen := indexes[next]; !seen {
				visit(next)
				if lowlinks[next] < lowlinks[ref] {
					lowlinks[ref] = lowlinks[next]
				}
			} else if onStack[next] && indexes[next] < lowlinks[ref] {
				lowlinks[ref] = indexes[next]
			}
		}
		if lowlinks[ref] != indexes[ref] {
			return
		}
		var members []string
		for len(stack) != 0 {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[last] = false
			members = append(members, last)
			if last == ref {
				break
			}
		}
		sort.Strings(members)
		components = append(components, members)
	}
	for _, ref := range refs {
		if _, seen := indexes[ref]; !seen {
			visit(ref)
		}
	}
	sort.SliceStable(components, func(i, j int) bool { return components[i][0] < components[j][0] })
	componentByRef := map[string]int{}
	for componentIndex, members := range components {
		for _, ref := range members {
			componentByRef[ref] = componentIndex
		}
	}
	return components, componentByRef
}

func (graph placementGraph) distances(roots []string, allowed map[string]struct{}) map[string]int {
	distance := map[string]int{}
	queue := append([]string(nil), roots...)
	for _, root := range roots {
		distance[root] = 0
	}
	for len(queue) != 0 {
		ref := queue[0]
		queue = queue[1:]
		for _, next := range sortedSet(graph.neighbors[ref]) {
			if _, ok := allowed[next]; !ok {
				continue
			}
			candidate := distance[ref] + 1
			if previous, ok := distance[next]; ok && previous <= candidate {
				continue
			}
			distance[next] = candidate
			queue = append(queue, next)
		}
	}
	return distance
}

func (graph placementGraph) inDegree(ref string, allowed map[string]struct{}) int {
	count := 0
	for from, targets := range graph.directed {
		if _, ok := allowed[from]; !ok {
			continue
		}
		if _, ok := targets[ref]; ok {
			count++
		}
	}
	return count
}

func (graph placementGraph) orders(refs []string, ranks map[string]int) map[string]int {
	byRank := map[int][]string{}
	for _, ref := range refs {
		byRank[ranks[ref]] = append(byRank[ranks[ref]], ref)
	}
	rankValues := make([]int, 0, len(byRank))
	for rank := range byRank {
		rankValues = append(rankValues, rank)
		sort.SliceStable(byRank[rank], func(i, j int) bool { return graph.refLess(byRank[rank][i], byRank[rank][j]) })
	}
	sort.Ints(rankValues)
	positions := map[string]int{}
	for _, rank := range rankValues {
		for index, ref := range byRank[rank] {
			positions[ref] = index
		}
	}
	for sweep := 0; sweep < 4; sweep++ {
		values := rankValues
		if sweep%2 == 1 {
			values = reversedInts(rankValues)
		}
		for _, rank := range values {
			sort.SliceStable(byRank[rank], func(i, j int) bool {
				left := graph.barycenter(byRank[rank][i], positions, ranks, rank, sweep%2 == 0)
				right := graph.barycenter(byRank[rank][j], positions, ranks, rank, sweep%2 == 0)
				if left != right {
					return left < right
				}
				return graph.refLess(byRank[rank][i], byRank[rank][j])
			})
			for index, ref := range byRank[rank] {
				positions[ref] = index
			}
		}
	}
	return positions
}

func (graph placementGraph) refLess(first, second string) bool {
	firstComponent := graph.components[first]
	secondComponent := graph.components[second]
	if firstComponent.OriginalOrdinal != secondComponent.OriginalOrdinal {
		return firstComponent.OriginalOrdinal < secondComponent.OriginalOrdinal
	}
	return first < second
}

func (graph placementGraph) barycenter(ref string, positions, ranks map[string]int, rank int, fromLeft bool) float64 {
	total := 0
	count := 0
	for neighbor := range graph.neighbors[ref] {
		neighborRank := ranks[neighbor]
		if (fromLeft && neighborRank >= rank) || (!fromLeft && neighborRank <= rank) {
			continue
		}
		total += positions[neighbor]
		count++
	}
	if count == 0 {
		return float64(positions[ref])
	}
	return float64(total) / float64(count)
}

func compactRanks(ranks map[string]int, fixed map[string]bool) map[string]int {
	if len(fixed) != 0 {
		return ranks
	}
	values := make([]int, 0, len(ranks))
	seen := map[int]struct{}{}
	for _, rank := range ranks {
		if _, ok := seen[rank]; ok {
			continue
		}
		seen[rank] = struct{}{}
		values = append(values, rank)
	}
	sort.Ints(values)
	remap := map[int]int{}
	for index, value := range values {
		remap[value] = index
	}
	for ref, rank := range ranks {
		ranks[ref] = remap[rank]
	}
	return ranks
}

func stageRank(stage Stage) int {
	if stage <= StageUnknown {
		return int(StageProcessing) - 1
	}
	return int(stage) - 1
}

func sortedSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortedIntSet(values map[int]struct{}) []int {
	out := make([]int, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func reversedInts(values []int) []int {
	out := append([]int(nil), values...)
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	return out
}
