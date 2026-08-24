package simmodel

import "sync"

const maxResistorAdjacencyCacheEntries = 128

type resistorAdjacencyCacheState struct {
	sync.Mutex
	entries map[string]map[string][]string
	order   []string
}

var resistorAdjacencyCache = resistorAdjacencyCacheState{
	entries: make(map[string]map[string][]string, maxResistorAdjacencyCacheEntries),
}

func resistorAdjacencyForPlan(plan Plan) map[string][]string {
	if plan.TopologyHash == "" {
		return buildResistorAdjacency(plan.Devices)
	}
	resistorAdjacencyCache.Lock()
	if adjacency, found := resistorAdjacencyCache.entries[plan.TopologyHash]; found {
		resistorAdjacencyCache.Unlock()
		return adjacency
	}
	resistorAdjacencyCache.Unlock()

	adjacency := buildResistorAdjacency(plan.Devices)
	resistorAdjacencyCache.Lock()
	defer resistorAdjacencyCache.Unlock()
	if existing, found := resistorAdjacencyCache.entries[plan.TopologyHash]; found {
		return existing
	}
	if len(resistorAdjacencyCache.order) == maxResistorAdjacencyCacheEntries {
		oldest := resistorAdjacencyCache.order[0]
		delete(resistorAdjacencyCache.entries, oldest)
		copy(resistorAdjacencyCache.order, resistorAdjacencyCache.order[1:])
		resistorAdjacencyCache.order = resistorAdjacencyCache.order[:len(resistorAdjacencyCache.order)-1]
	}
	resistorAdjacencyCache.entries[plan.TopologyHash] = adjacency
	resistorAdjacencyCache.order = append(resistorAdjacencyCache.order, plan.TopologyHash)
	return adjacency
}
