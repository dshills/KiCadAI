package capabilityfeedback

import (
	"testing"

	"kicadai/internal/opentopologysynthesis"
)

func TestV21TopologyDiagnosticsHaveStableSpecificCapabilities(t *testing.T) {
	tests := map[string]string{
		string(opentopologysynthesis.CodeTopologyNoApplicableV21):  "generic_topology_operation",
		string(opentopologysynthesis.CodeTopologyContradictoryV21): "causal_path_consistency",
		string(opentopologysynthesis.CodeTopologyCycleV21):         "topology_cycle_resolution",
		string(opentopologysynthesis.CodeTopologyBoundV21):         "bounded_topology_completion",
		string(opentopologysynthesis.CodeTopologyInvalidRepairV21): "topology_invariant_preservation",
	}
	for code, capability := range tests {
		gap, found := topologyCompletionGapV21(Gap{Code: code})
		if !found || gap.Stage != "topology_completion" || gap.Scope != ScopeTopology || gap.Capability != capability || len(gap.RequiredEvidence) == 0 {
			t.Errorf("code %s = found=%t gap=%+v", code, found, gap)
		}
	}
	if _, found := topologyCompletionGapV21(Gap{Code: "OPEN_TOPOLOGY_REPAIR_EXHAUSTED"}); found {
		t.Fatal("historical topology diagnostic was reclassified")
	}
}
