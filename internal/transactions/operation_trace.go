package transactions

import "kicadai/internal/reports"

type OperationTrace struct {
	OperationID string             `json:"operation_id"`
	Index       int                `json:"index"`
	Op          OperationKind      `json:"op"`
	Refs        []string           `json:"refs,omitempty"`
	Nets        []string           `json:"nets,omitempty"`
	Paths       []string           `json:"paths,omitempty"`
	Artifacts   []reports.Artifact `json:"artifacts,omitempty"`
}

type OperationTraceMap struct {
	traces     []OperationTrace
	byID       map[string]int
	byIndex    map[int]int
	byRef      map[string][]int
	byNet      map[string][]int
	byArtifact map[string][]int
}

func NewOperationTraceMapFromPlan(plan Plan) OperationTraceMap {
	traces := make([]OperationTrace, 0, len(plan.Operations))
	for _, operation := range plan.Operations {
		trace := OperationTrace{
			OperationID: operation.ID,
			Index:       operation.Index,
			Op:          operation.Op,
			Refs:        append([]string(nil), operation.Refs...),
			Nets:        append([]string(nil), operation.Nets...),
			Artifacts:   append([]reports.Artifact(nil), operation.Artifacts...),
		}
		traces = append(traces, trace)
	}
	return NewOperationTraceMap(traces)
}

func NewOperationTraceMap(traces []OperationTrace) OperationTraceMap {
	traceMap := OperationTraceMap{
		traces:     traces,
		byID:       map[string]int{},
		byIndex:    map[int]int{},
		byRef:      map[string][]int{},
		byNet:      map[string][]int{},
		byArtifact: map[string][]int{},
	}
	for i, trace := range traces {
		if trace.OperationID != "" {
			traceMap.byID[trace.OperationID] = i
		}
		traceMap.byIndex[trace.Index] = i
		for _, ref := range trace.Refs {
			if ref != "" {
				traceMap.byRef[ref] = appendUniqueTraceIndex(traceMap.byRef[ref], i)
			}
		}
		for _, net := range trace.Nets {
			if net != "" {
				traceMap.byNet[net] = appendUniqueTraceIndex(traceMap.byNet[net], i)
			}
		}
		for _, path := range trace.Paths {
			if path != "" {
				traceMap.byArtifact[path] = appendUniqueTraceIndex(traceMap.byArtifact[path], i)
			}
		}
		for _, artifact := range trace.Artifacts {
			if artifact.Path != "" {
				traceMap.byArtifact[artifact.Path] = appendUniqueTraceIndex(traceMap.byArtifact[artifact.Path], i)
			}
		}
	}
	return traceMap
}

func appendUniqueTraceIndex(indices []int, index int) []int {
	if len(indices) > 0 && indices[len(indices)-1] == index {
		return indices
	}
	return append(indices, index)
}

func (traceMap OperationTraceMap) ByOperationID(id string) (OperationTrace, bool) {
	index, ok := traceMap.byID[id]
	if !ok {
		return OperationTrace{}, false
	}
	return traceMap.traces[index], true
}

func (traceMap OperationTraceMap) ByIndex(index int) (OperationTrace, bool) {
	traceIndex, ok := traceMap.byIndex[index]
	if !ok {
		return OperationTrace{}, false
	}
	return traceMap.traces[traceIndex], true
}

func (traceMap OperationTraceMap) UniqueByRef(ref string) (OperationTrace, bool) {
	return traceMap.unique(traceMap.byRef[ref])
}

func (traceMap OperationTraceMap) UniqueByNet(net string) (OperationTrace, bool) {
	return traceMap.unique(traceMap.byNet[net])
}

func (traceMap OperationTraceMap) ByArtifact(path string) (OperationTrace, bool) {
	return traceMap.unique(traceMap.byArtifact[path])
}

func (traceMap OperationTraceMap) AnnotateIssues(issues []reports.Issue) {
	for i := range issues {
		if issues[i].OperationID != "" {
			continue
		}
		if trace, ok := traceMap.ByArtifact(issues[i].Path); ok {
			issues[i].OperationID = trace.OperationID
			continue
		}
		if len(issues[i].Refs) == 1 {
			if trace, ok := traceMap.UniqueByRef(issues[i].Refs[0]); ok {
				issues[i].OperationID = trace.OperationID
				continue
			}
		}
		if len(issues[i].Nets) == 1 {
			if trace, ok := traceMap.UniqueByNet(issues[i].Nets[0]); ok {
				issues[i].OperationID = trace.OperationID
			}
		}
	}
}

func (traceMap OperationTraceMap) unique(indices []int) (OperationTrace, bool) {
	if len(indices) != 1 {
		return OperationTrace{}, false
	}
	return traceMap.traces[indices[0]], true
}
