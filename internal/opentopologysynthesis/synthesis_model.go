package opentopologysynthesis

const (
	SynthesisRunSchema  = "kicadai.open-topology-synthesis-run.v1"
	SynthesisRunVersion = 1
)

// SynthesisRun retains the complete, hash-bound evidence needed to explain why
// a primitive topology was selected or why the bounded search failed closed.
// SelectedGraph and Physical are populated only when Report.Status is
// StatusPassed; their absence is the explicit found=false contract for every
// invalid, canceled, exhausted, or failed run.
type SynthesisRun struct {
	Schema         string                       `json:"schema"`
	Version        int                          `json:"version"`
	Report         Report                       `json:"report"`
	Search         TopologySearchResult         `json:"search"`
	Candidates     []SynthesisCandidateEvidence `json:"candidates"`
	SelectedGraph  *CandidateGraph              `json:"selected_graph,omitempty"`
	SelectedTrial  *ValueTrial                  `json:"selected_trial,omitempty"`
	SelectedRepair *RepairSearchResult          `json:"selected_repair,omitempty"`
	Physical       *PhysicalLoweringResult      `json:"physical,omitempty"`
	Hash           string                       `json:"hash"`
}

type SynthesisCandidateEvidence struct {
	Fingerprint         string                   `json:"fingerprint"`
	TopologyHash        string                   `json:"topology_hash"`
	ActiveStructureHash string                   `json:"active_structure_hash"`
	ValuePlan           ValueSearchPlan          `json:"value_plan"`
	Evaluations         []SimulationEvaluation   `json:"evaluations"`
	Repair              *RepairSearchResult      `json:"repair,omitempty"`
	Physical            []PhysicalLoweringResult `json:"physical"`
}
